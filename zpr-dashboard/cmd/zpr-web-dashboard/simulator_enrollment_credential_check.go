//go:build !production

package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

// simulatorEnrollmentCredentialCheck exercises the endpoint side of the
// production BAS runtime credential contract for one onboarded endpoint: the
// handoff bundle's runtime key requests a one-time nonce from the adapter
// listener and proves possession to receive an authorization code. The code
// is never redeemed or revealed — token redemption belongs exclusively to the
// pinned Visa Service peer — and it expires on its own short lifetime. This
// proves the approved runtime key is accepted by the production issuer; it
// does not admit the endpoint, issue a visa or grant any traffic.
type simulatorEnrollmentCredentialCheckHandler struct {
	basURL       string
	basAuthority string
	basCAFile    string
	workspace    string
	authorize    func(ctx context.Context, assetID string, key *rsa.PrivateKey) error
	now          func() time.Time
}

func newSimulatorEnrollmentCredentialCheckHandler() *simulatorEnrollmentCredentialCheckHandler {
	handler := &simulatorEnrollmentCredentialCheckHandler{
		basURL:       strings.TrimSpace(os.Getenv("SIMULATOR_ENROLLMENT_BAS_URL")),
		basAuthority: strings.TrimSpace(os.Getenv("SIMULATOR_ENROLLMENT_BAS_AUTHORITY")),
		basCAFile:    strings.TrimSpace(os.Getenv("SIMULATOR_ENROLLMENT_BAS_CA_FILE")),
		workspace:    strings.TrimSpace(os.Getenv("SIMULATION_WORKSPACE_DB_DIR")),
		now:          time.Now,
	}
	handler.authorize = func(ctx context.Context, assetID string, key *rsa.PrivateKey) error {
		return simulatorEnrollmentBASAuthorize(ctx, handler.basURL, handler.basAuthority, handler.basCAFile, assetID, key)
	}
	return handler
}

// simulatorEnrollmentBASAuthorize performs the nonce/proof exchange with the
// BAS adapter listener and discards the resulting one-time code unread.
func simulatorEnrollmentBASAuthorize(ctx context.Context, basURL, authority, caFile, assetID string, key *rsa.PrivateKey) error {
	caPEM, err := readOperatorFile(caFile, 65536, false)
	if err != nil {
		return fmt.Errorf("read BAS adapter CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return errors.New("BAS adapter CA contains no certificates")
	}
	host := authority
	if name, _, err := net.SplitHostPort(authority); err == nil {
		host = name
	}
	transport := &http.Transport{
		TLSClientConfig:     &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13, ServerName: host},
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		MaxResponseHeaderBytes: 16 * 1024,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		basURL+"/preauthorize?response_type=code&client_id="+url.QueryEscape(assetID), nil)
	if err != nil {
		return err
	}
	request.Host = authority
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("BAS preauthorize: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("BAS preauthorize denied (HTTP %d)", response.StatusCode)
	}
	var challenge struct {
		Nonce string `json:"nonce"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, response.Body, 4096)).Decode(&challenge); err != nil {
		return errors.New("BAS preauthorize response is invalid")
	}
	nonce, err := base64.StdEncoding.Strict().DecodeString(challenge.Nonce)
	if err != nil || len(nonce) != 32 {
		return errors.New("BAS nonce is invalid")
	}
	digest := sha256.Sum256(nonce)
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return errors.New("cannot sign BAS proof of possession")
	}
	body, err := json.Marshal(struct {
		Nonce    string `json:"nonce"`
		ClientID string `json:"client_id"`
		Payload  string `json:"payload"`
	}{challenge.Nonce, assetID, base64.StdEncoding.EncodeToString(signature)})
	if err != nil {
		return err
	}
	request, err = http.NewRequestWithContext(ctx, http.MethodPost, basURL+"/authorize", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Host = authority
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		return fmt.Errorf("BAS authorize: %w", err)
	}
	defer response.Body.Close()
	location := response.Header.Get("Location")
	if response.StatusCode != http.StatusFound || !strings.HasPrefix(location, "https://auth.zpr?code=") {
		return fmt.Errorf("BAS authorize denied (HTTP %d)", response.StatusCode)
	}
	// The one-time authorization code is deliberately discarded: redemption
	// belongs exclusively to the pinned Visa Service peer, and the code
	// expires on its configured short lifetime.
	return nil
}

func (h *simulatorEnrollmentCredentialCheckHandler) serve(auth operatorEnrollmentAuthorizer, signer *operatordelegation.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			AssetID string `json:"asset_id"`
		}
		organization, _, ok := simulatorEnrollmentMutationContext(w, r, auth, signer, &input)
		if !ok {
			return
		}
		if input.AssetID == "" || strings.ContainsAny(input.AssetID, "/\\.\x00") {
			http.Error(w, "asset_id must be a plain asset identifier", http.StatusBadRequest)
			return
		}
		if h.basURL == "" || h.basAuthority == "" || h.basCAFile == "" {
			http.Error(w, "BAS runtime credential contract is not configured", http.StatusServiceUnavailable)
			return
		}
		if h.workspace == "" {
			http.Error(w, "simulator workspace is not configured", http.StatusServiceUnavailable)
			return
		}
		bundle := filepath.Join(h.workspace, "enrollment-identities", organization, input.AssetID)
		manifestData, err := os.ReadFile(filepath.Join(bundle, "identity.json"))
		if err != nil {
			writeWorkspaceError(w, http.StatusNotFound,
				"no onboarded endpoint key bundle exists for this asset; run explicit onboarding first")
			return
		}
		var manifest struct {
			InvitationID          string `json:"invitation_id"`
			RuntimeKeyFingerprint string `json:"runtime_key_fingerprint"`
		}
		if err := json.Unmarshal(manifestData, &manifest); err != nil || manifest.RuntimeKeyFingerprint == "" {
			writeWorkspaceError(w, http.StatusConflict, "the endpoint key bundle manifest is invalid; reconcile before retrying")
			return
		}
		keyPEM, err := os.ReadFile(filepath.Join(bundle, "runtime-auth.key"))
		if err != nil {
			writeWorkspaceError(w, http.StatusConflict, "the endpoint runtime key is missing from the bundle; reconcile before retrying")
			return
		}
		block, rest := pem.Decode(keyPEM)
		if block == nil || block.Type != "PRIVATE KEY" || len(bytes.TrimSpace(rest)) != 0 {
			writeWorkspaceError(w, http.StatusConflict, "the endpoint runtime key is not one PKCS#8 PEM private key")
			return
		}
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		key, isRSA := parsed.(*rsa.PrivateKey)
		if err != nil || !isRSA {
			writeWorkspaceError(w, http.StatusConflict, "the endpoint runtime key is not an RSA PKCS#8 key")
			return
		}
		fingerprint, err := simulatorEnrollmentKeyFingerprint(key)
		if err != nil || fingerprint != manifest.RuntimeKeyFingerprint {
			writeWorkspaceError(w, http.StatusConflict,
				"the bundled runtime key does not match the recorded approved fingerprint; reconcile before retrying")
			return
		}
		checkContext, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		err = h.authorize(checkContext, input.AssetID, key)
		cancel()
		if err != nil {
			writeWorkspaceError(w, http.StatusBadGateway, fmt.Sprintf(
				"BAS runtime credential check failed for asset %s: %v", input.AssetID, err))
			return
		}
		checkedAt := h.now().UTC()
		recordPath := filepath.Join(h.workspace, "enrollment-onboarding", organization+"-"+input.AssetID+".json")
		if data, err := os.ReadFile(recordPath); err == nil {
			var record simulatorEnrollmentOnboardingRecord
			if json.Unmarshal(data, &record) == nil {
				record.RuntimeCredentialCheck = "bas-authorized"
				record.RuntimeCredentialCheckedAt = &checkedAt
				if updated, err := json.MarshalIndent(record, "", "  "); err == nil {
					temporary := recordPath + ".tmp"
					if os.WriteFile(temporary, append(updated, '\n'), 0o600) == nil {
						_ = os.Rename(temporary, recordPath)
					}
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Organization          string    `json:"organization"`
			AssetID               string    `json:"asset_id"`
			InvitationID          string    `json:"invitation_id"`
			RuntimeKeyFingerprint string    `json:"runtime_key_fingerprint"`
			Check                 string    `json:"runtime_credential_check"`
			CheckedAt             time.Time `json:"checked_at"`
		}{organization, input.AssetID, manifest.InvitationID, fingerprint, "bas-authorized", checkedAt})
	}
}
