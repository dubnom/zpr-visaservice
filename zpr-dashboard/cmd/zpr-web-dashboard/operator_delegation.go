package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/audit"
	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/controlroom"
	"neboagency.com/zpr-dashborad/internal/enrollment"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

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
	audience := strings.TrimSpace(os.Getenv("ZPR_OPERATOR_DELEGATION_AUDIENCE"))
	if audience == "" {
		audience = strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_URL"))
	}
	if config.Audience != audience {
		return nil, errors.New("operator delegation audience must exactly equal its configured service audience")
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

// configuredBudgetAuthority opens only the explicitly configured persistent authority store.
func configuredBudgetAuthority(auditRepository audit.Repository) (*budgetauthority.Handler, func() error) {
	unavailable := func() (*budgetauthority.Handler, func() error) {
		return budgetauthority.NewAuditedHandler(nil, nil, time.Now, auditRepository), func() error { return nil }
	}
	databasePath := strings.TrimSpace(os.Getenv("ZPR_BUDGET_AUTHORITY_DB_PATH"))
	if databasePath == "" {
		log.Print("Budget authority is disabled: ZPR_BUDGET_AUTHORITY_DB_PATH is not configured")
		return unavailable()
	}
	store, err := budgetauthority.Open(databasePath)
	if err != nil {
		log.Printf("Budget authority database is unavailable: %T", err)
		return unavailable()
	}
	trustPath := strings.TrimSpace(os.Getenv("ZPR_OPERATOR_DELEGATION_TRUST_FILE"))
	if trustPath == "" {
		log.Print("Budget authority mutations are disabled: operator delegation trust is not configured")
		return budgetauthority.NewAuditedHandler(store, nil, time.Now, auditRepository), store.Close
	}
	data, err := readOperatorFile(trustPath, 65536, false)
	if err != nil {
		log.Printf("Budget authority mutations are disabled: delegation trust is unavailable (%T)", err)
		return budgetauthority.NewAuditedHandler(store, nil, time.Now, auditRepository), store.Close
	}
	var trust operatordelegation.Config
	if err := decodeOperatorConfig(data, &trust); err != nil {
		log.Printf("Budget authority mutations are disabled: delegation trust is invalid (%T)", err)
		return budgetauthority.NewAuditedHandler(store, nil, time.Now, auditRepository), store.Close
	}
	verifier, err := operatordelegation.NewVerifier(trust, store)
	if err != nil {
		log.Printf("Budget authority mutations are disabled: delegation verifier is unavailable (%T)", err)
		return budgetauthority.NewAuditedHandler(store, nil, time.Now, auditRepository), store.Close
	}
	return budgetauthority.NewAuditedHandler(store, verifier, time.Now, auditRepository), store.Close
}

func configuredOperatorEnrollment(store *enrollment.Store, config enrollment.Config, auditRepositories ...audit.Repository) (http.Handler, error) {
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
	return enrollment.NewOperatorAdminHandler(store, config, trust, auditRepositories...)
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
		case r.Method == "GET" && (path == "catalog" || path == "invitations" || path == "preflight" ||
			(len(parts) == 2 && (parts[0] == "invitations" || parts[0] == "invitation-batches") && parts[1] != "")):
			permission = "read"
		case r.Method == "POST" && (path == "invitations" || path == "preflight"):
			permission = "create"
		case r.Method == "POST" && path == "invitation-batches/review":
			permission = "read"
		case r.Method == "POST" && path == "invitation-batches/approve":
			permission = "approve"
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
		request = controlroom.WithOperatorDelegation(request, token)
		request.Header.Del(operatordelegation.Header)
		proxy.ServeHTTP(w, request)
	})
}

// budgetAuthorityOperatorProxy authorizes and delegates Control Room budget mutations.
func budgetAuthorityOperatorProxy(
	auth operatorRequestAuthorizer,
	signer *operatordelegation.Signer,
	proxy http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		deny := func(status int, message string) {
			log.Printf("Control Room budget authority denied (HTTP %d)", status)
			writePolicyError(w, status, message)
		}
		if operatorAuthorizationDisabled(auth) || signer == nil || proxy == nil {
			deny(http.StatusForbidden, "Budget authority requires configured named-user authorization and backend delegation.")
			return
		}
		if r.Method != http.MethodPost || r.URL.RawPath != "" || r.URL.RawQuery != "" ||
			r.URL.ForceQuery || path.Clean(r.URL.Path) != r.URL.Path {
			deny(http.StatusBadRequest, "Use a canonical budget authority mutation route.")
			return
		}
		const prefix = "/api/budget-authority/"
		remainder := strings.TrimPrefix(r.URL.Path, prefix)
		parts := strings.Split(remainder, "/")
		approvalRoute := r.URL.Path == prefix+"approvals"
		revokeRoute := len(parts) == 3 && parts[0] == "approvals" && parts[1] != "" && parts[2] == "revoke"
		if !approvalRoute && !revokeRoute {
			deny(http.StatusNotFound, "Unknown budget authority operation.")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, operatordelegation.MaxBody))
		if err != nil {
			deny(http.StatusRequestEntityTooLarge, "Budget authority request exceeds 8192 bytes or could not be read.")
			return
		}
		organization := ""
		if approvalRoute {
			var input struct {
				Scope     budgetauthority.ScopeRequest `json:"scope"`
				Decision  budgetauthority.Decision     `json:"decision"`
				ExpiresAt time.Time                    `json:"expires_at"`
			}
			if err := decodeOperatorConfig(body, &input); err != nil {
				deny(http.StatusBadRequest, "Budget approval request must be strict JSON.")
				return
			}
			scope, err := input.Scope.Scope()
			if err != nil || input.Decision != budgetauthority.DecisionApproved || input.ExpiresAt.IsZero() {
				deny(http.StatusBadRequest, "Explicit approval, exact scope, and expiry are required.")
				return
			}
			organization = scope.OrganizationID
		} else {
			var input struct {
				OrganizationID string `json:"organization_id"`
			}
			if err := decodeOperatorConfig(body, &input); err != nil {
				deny(http.StatusBadRequest, "Budget revocation request must be strict JSON.")
				return
			}
			organization = input.OrganizationID
		}
		if organization == "" || strings.TrimSpace(organization) != organization {
			deny(http.StatusBadRequest, "An explicit organization is required.")
			return
		}
		identity, err := auth.Authorize(r, organization, "budget.approve")
		if err != nil || !slices.Contains(identity.Organizations, organization) ||
			!slices.Contains(identity.Permissions, "budget.approve") {
			deny(http.StatusForbidden, "An active same-origin session, organization scope, budget.approve permission, and CSRF proof are required.")
			return
		}
		request := r.Clone(r.Context())
		request.URL.Path = operatordelegation.Prefix + strings.TrimPrefix(r.URL.Path, "/api/")
		request.URL.RawPath = ""
		request.Body = io.NopCloser(bytes.NewReader(body))
		token, err := signer.Sign(request, body, identity, time.Now())
		if err != nil {
			deny(http.StatusBadRequest, "Budget authority delegation could not be created.")
			return
		}
		request = controlroom.WithOperatorDelegation(request, token)
		request.Header.Del(operatordelegation.Header)
		proxy.ServeHTTP(w, request)
	})
}
