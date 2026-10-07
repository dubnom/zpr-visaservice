package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/enrollment"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

type signedOperatorRequestKey struct{}

func decodeOperatorConfig(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return errors.New("operator configuration must be strict JSON")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("operator configuration must contain exactly one JSON document")
	}
	return nil
}

func configuredOperatorSigner(security *controlRoomSecurity) (*operatordelegation.Signer, error) {
	configPath := strings.TrimSpace(os.Getenv("ZPR_OPERATOR_DELEGATION_SIGNER_FILE"))
	keyPath := strings.TrimSpace(os.Getenv("ZPR_OPERATOR_DELEGATION_KEY_FILE"))
	if configPath == "" && keyPath == "" {
		return nil, nil
	}
	if configPath == "" || keyPath == "" || security.auth == nil || security.tls == nil {
		return nil, errors.New("operator delegation requires direct HTTPS/OIDC plus ZPR_OPERATOR_DELEGATION_SIGNER_FILE and ZPR_OPERATOR_DELEGATION_KEY_FILE")
	}
	data, err := readOperatorFile(configPath, 65536, false)
	if err != nil {
		return nil, err
	}
	var config operatordelegation.SignerConfig
	if err := decodeOperatorConfig(data, &config); err != nil {
		return nil, err
	}
	if config.Audience != strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_URL")) {
		return nil, errors.New("operator delegation audience must exactly equal ZPR_CONTROL_SERVICE_URL")
	}
	keyData, err := readOperatorFile(keyPath, 65536, true)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(keyData)
	if block == nil || block.Type != "PRIVATE KEY" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("operator delegation key must be one PKCS#8 Ed25519 PEM private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid operator delegation PKCS#8 key")
	}
	private, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("operator delegation key must be Ed25519")
	}
	return operatordelegation.NewSigner(config, private)
}

func configuredOperatorEnrollment(store *enrollment.Store, config enrollment.Config) (http.Handler, error) {
	path := strings.TrimSpace(os.Getenv("ZPR_OPERATOR_DELEGATION_TRUST_FILE"))
	if path == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Print("Operator enrollment requested but delegation is not configured")
			writePolicyError(w, http.StatusServiceUnavailable, "Named-user delegation is not configured at Control-Service.")
		}), nil
	}
	data, err := readOperatorFile(path, 65536, false)
	if err != nil {
		return nil, err
	}
	var trust operatordelegation.Config
	if err := decodeOperatorConfig(data, &trust); err != nil {
		return nil, err
	}
	return enrollment.NewOperatorAdminHandler(store, config, trust)
}

func operatorEnrollmentProxy(auth *operatorauth.Auth, signer *operatordelegation.Signer, proxy http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		deny := func(status int, message string) {
			log.Printf("Control Room operator enrollment denied (HTTP %d)", status)
			writePolicyError(w, status, message)
		}
		if auth == nil || signer == nil {
			deny(http.StatusForbidden, "Control Room enrollment requires configured named-user authorization and verified backend delegation.")
			return
		}
		if !strings.HasPrefix(r.URL.Path, enrollment.APIPrefix) || r.URL.RawPath != "" || r.URL.ForceQuery {
			deny(http.StatusBadRequest, "Use the canonical versioned enrollment route.")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, enrollment.APIPrefix)
		parts := strings.Split(path, "/")
		permission := ""
		switch {
		case r.Method == "GET" && (path == "catalog" || path == "invitations" || (len(parts) == 2 && parts[0] == "invitations" && parts[1] != "")):
			permission = "read"
		case r.Method == "POST" && path == "invitations":
			permission = "create"
		case r.Method == "POST" && len(parts) == 3 && parts[0] == "invitations" && parts[1] != "" &&
			slices.Contains([]string{"cancel", "approve", "reject"}, parts[2]):
			permission = parts[2]
		default:
			deny(http.StatusNotFound, "Unknown enrollment operation.")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, operatordelegation.MaxBody))
		if err != nil || (r.Method == "GET" && len(body) != 0) {
			deny(http.StatusRequestEntityTooLarge, "Enrollment request body is invalid or exceeds 8192 bytes.")
			return
		}
		organization := r.URL.Query().Get("organization")
		if r.Method == "POST" {
			var input struct {
				Organization string `json:"organization"`
			}
			if r.URL.RawQuery != "" || json.Unmarshal(body, &input) != nil {
				deny(http.StatusBadRequest, "Enrollment mutation requires an organization in a JSON body without URL parameters.")
				return
			}
			organization = input.Organization
		}
		var identity operatorauth.Identity
		if path == "catalog" {
			identity, _, err = auth.Session(r)
			if err == nil && !slices.Contains(identity.Permissions, "read") {
				err = errors.New("read permission required")
			}
		} else if organization == "" {
			err = errors.New("explicit organization required")
		} else {
			identity, err = auth.Authorize(r, organization, permission)
		}
		if err != nil {
			deny(http.StatusForbidden, "An active same-origin named-user session, authorized scope, and mutation CSRF proof are required.")
			return
		}
		request := r.Clone(r.Context())
		request.URL.Path = operatordelegation.Prefix + path
		request.Body = io.NopCloser(bytes.NewReader(body))
		token, err := signer.Sign(request, body, identity, time.Now())
		if err != nil {
			deny(http.StatusBadRequest, "Enrollment delegation could not be created for this request.")
			return
		}
		request = request.WithContext(context.WithValue(request.Context(), signedOperatorRequestKey{}, token))
		request.Header.Del(operatordelegation.Header)
		proxy.ServeHTTP(w, request)
	})
}
