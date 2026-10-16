package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestPolicyApplyProtectedPSLedgerOwnsLostResponseReconciliation(t *testing.T) {
	path, config, now := policyApplyConfigFixture(t)
	review := newPolicyApplyFixture(t)
	review.now = now
	writePolicyApplyJSON(t, config.TrustFile, review.trust)
	resources, err := loadPolicyApplyResources(path, now)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	calls := 0
	transport := policyApplyTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			calls++
			if _, err := resources.store.PolicyOperation(ctx, review.trust.Organization, "stable-op"); err != nil {
				t.Fatal("PS-owned durable approval must precede dispatch", err)
			}
			if _, err := review.store.PolicyOperation(ctx, review.trust.Organization, "stable-op"); !errors.Is(err, changereview.ErrNotFound) {
				t.Fatal("operation was stored in the Control Room review DB", err)
			}
			if calls == 1 {
				return nil, errors.New("synthetic lost response")
			}
			var command operatordelegation.PolicyCommand
			if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
				t.Fatal(err)
			}
			return policyApplyResponse(t, policyApplyReceipt(command.Operation)), nil
		}
		return policyApplyResponse(t, operatordelegation.PolicyCurrentState{
			TargetID: review.trust.TargetID, Organization: review.trust.Organization,
			CurrentDigest: review.stage.BundleSHA256, CurrentVinst: 8,
		}), nil
	})
	// Replace only the already-validated transport with an in-memory fixture;
	// never dial a network or use the generated test TLS keys for a service.
	resources.client.http.Transport = transport
	body, proof := review.pair(t, review.current)
	first, err := resources.client.apply(ctx, resources.store, body, proof, now)
	if err != nil || first.Status != "uncertain" {
		t.Fatal("lost response did not persist uncertain in PS store", err)
	}
	if err = resources.Close(); err != nil {
		t.Fatal(err)
	}
	resources, err = loadPolicyApplyResources(path, now)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resources.Close(); err != nil {
			t.Error(err)
		}
	}()
	resources.client.http.Transport = transport
	if _, err = resources.client.apply(ctx, resources.store, body, proof, now); err == nil || calls != 1 {
		t.Fatal("PS replay protection did not survive resource reopen")
	}
	body, proof = review.pair(t, review.current)
	final, err := resources.client.apply(ctx, resources.store, body, proof, now)
	if err != nil || final.Status != "succeeded" || final.Binding != first.Binding || calls != 2 {
		t.Fatalf("PS-owned fresh-proof reconciliation: %+v %v calls=%d", final, err, calls)
	}
}

func policyApplyConfigFixture(t *testing.T) (string, policyApplyServiceConfig, time.Time) {
	t.Helper()
	f := newPolicyApplyFixture(t)
	directory := t.TempDir()
	config := policyApplyServiceConfig{
		Version: 1, OperationDB: filepath.Join(directory, "ps-operations", "operations.sqlite"),
		TrustFile: filepath.Join(directory, "trust.json"), VisaService: f.trust.VisaServiceAudience,
		APICredential: filepath.Join(directory, "api-key"), ClientCert: filepath.Join(directory, "client.pem"),
		ClientKey: filepath.Join(directory, "client.key"), ServerCA: filepath.Join(directory, "server-ca.pem"),
	}
	now := time.Now()
	f.trust.Keys[0].ExpiresUnix = now.Add(time.Hour).Unix()
	writePolicyApplyJSON(t, config.TrustFile, f.trust)
	if err := os.WriteFile(config.APICredential, []byte("synthetic-api-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := createTestPolicyLeaf(ca, caKey, "policy-service", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeTLSCertificate(config.ClientCert, cert); err != nil {
		t.Fatal(err)
	}
	if err = writeTLSPrivateKey(config.ClientKey, key); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(config.ServerCA, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "policy-apply.json")
	writePolicyApplyJSON(t, configPath, config)
	return configPath, config, now
}

func writePolicyApplyJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyApplyProtectedResourcesConstructWithoutNetworkOrRoute(t *testing.T) {
	path, config, now := policyApplyConfigFixture(t)
	resources, err := loadPolicyApplyResources(path, now)
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig := resources.transport.TLSClientConfig
	if tlsConfig == nil || tlsConfig.MinVersion != tls.VersionTLS13 || tlsConfig.InsecureSkipVerify ||
		tlsConfig.RootCAs == nil || tlsConfig.ServerName != "vs.example" || len(tlsConfig.Certificates) != 1 ||
		resources.client.apiKey != "synthetic-api-key" || resources.client.http.Transport != resources.transport {
		t.Fatal("protected verified TLS/API-key client was not composed")
	}
	if resources.store == nil || resources.client.http.CheckRedirect == nil {
		t.Fatal("durable store or redirect guard absent")
	}
	if err = resources.Close(); err != nil {
		t.Fatal(err)
	}
	// Explicit PS-owned store can be reopened without sharing the CR review DB.
	resources, err = loadPolicyApplyResources(path, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = resources.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(config.OperationDB)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatal("PS-owned operation DB is not private", err)
	}
	// Even a verified mTLS peer and API key do not create a registered apply route.
	app := policyServiceMux(&policyWorkspace{})
	request := httptest.NewRequest("POST", policyApplyProducerRoute, strings.NewReader(`{"actor":"self-asserted"}`))
	request.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{}}, VerifiedChains: [][]*x509.Certificate{{{}}},
	}
	request.Header.Set("X-API-Key", "synthetic-api-key")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unapproved apply route became available: %d", response.Code)
	}
}

func TestPolicyApplyProtectedConfigurationFailsClosed(t *testing.T) {
	cases := []string{
		"absent", "relative", "unknown-field", "wrong-version", "public-config", "config-symlink",
		"missing-db", "aliased-db", "relative-trust", "missing-trust", "public-trust", "stale-trust",
		"wrong-org-grant", "wrong-action-grant", "same-audience", "wrong-vs-origin",
		"missing-api-key", "public-api-key", "empty-api-key", "multiline-api-key",
		"missing-client-key", "public-client-key", "invalid-client-cert", "expired-client-cert",
		"missing-ca", "invalid-ca", "public-ca", "public-db-directory",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			path, config, now := policyApplyConfigFixture(t)
			writeConfig := true
			switch name {
			case "absent":
				path = filepath.Join(t.TempDir(), "absent")
				writeConfig = false
			case "relative":
				path = "relative.json"
				writeConfig = false
			case "unknown-field":
				writePolicyApplyJSON(t, path, map[string]any{"version": 1, "actor": "self-asserted"})
				writeConfig = false
			case "wrong-version":
				config.Version = 2
			case "public-config":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "config-symlink":
				link := filepath.Join(filepath.Dir(path), "link.json")
				if err := os.Symlink(path, link); err != nil {
					t.Fatal(err)
				}
				path, writeConfig = link, false
			case "missing-db":
				config.OperationDB = ""
			case "aliased-db":
				config.OperationDB = config.TrustFile
			case "relative-trust":
				config.TrustFile = "trust.json"
			case "missing-trust":
				if err := os.Remove(config.TrustFile); err != nil {
					t.Fatal(err)
				}
			case "public-trust":
				if err := os.Chmod(config.TrustFile, 0644); err != nil {
					t.Fatal(err)
				}
			case "stale-trust", "wrong-org-grant", "wrong-action-grant", "same-audience":
				data, err := os.ReadFile(config.TrustFile)
				if err != nil {
					t.Fatal(err)
				}
				var trust operatordelegation.PolicyTrust
				if err = json.Unmarshal(data, &trust); err != nil {
					t.Fatal(err)
				}
				switch name {
				case "stale-trust":
					trust.Keys[0].ExpiresUnix = now.Unix()
				case "wrong-org-grant":
					trust.Grants[0].Organization = "other-org"
				case "wrong-action-grant":
					trust.Grants[0].Actions = []string{"policy.edit"}
				case "same-audience":
					trust.Audience = trust.VisaServiceAudience
				}
				writePolicyApplyJSON(t, config.TrustFile, trust)
			case "wrong-vs-origin":
				config.VisaService = "https://other.example"
			case "missing-api-key":
				if err := os.Remove(config.APICredential); err != nil {
					t.Fatal(err)
				}
			case "public-api-key":
				if err := os.Chmod(config.APICredential, 0644); err != nil {
					t.Fatal(err)
				}
			case "empty-api-key", "multiline-api-key":
				value := ""
				if name == "multiline-api-key" {
					value = "synthetic\ninjected"
				}
				if err := os.WriteFile(config.APICredential, []byte(value), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-client-key":
				if err := os.Remove(config.ClientKey); err != nil {
					t.Fatal(err)
				}
			case "public-client-key":
				if err := os.Chmod(config.ClientKey, 0644); err != nil {
					t.Fatal(err)
				}
			case "invalid-client-cert":
				if err := os.WriteFile(config.ClientCert, []byte("invalid"), 0600); err != nil {
					t.Fatal(err)
				}
			case "expired-client-cert":
				now = now.AddDate(10, 0, 0)
				data, err := os.ReadFile(config.TrustFile)
				if err != nil {
					t.Fatal(err)
				}
				var trust operatordelegation.PolicyTrust
				if err = json.Unmarshal(data, &trust); err != nil {
					t.Fatal(err)
				}
				trust.Keys[0].ExpiresUnix = now.Add(time.Hour).Unix()
				writePolicyApplyJSON(t, config.TrustFile, trust)
			case "missing-ca":
				if err := os.Remove(config.ServerCA); err != nil {
					t.Fatal(err)
				}
			case "invalid-ca":
				if err := os.WriteFile(config.ServerCA, []byte("invalid"), 0600); err != nil {
					t.Fatal(err)
				}
			case "public-ca":
				if err := os.Chmod(config.ServerCA, 0644); err != nil {
					t.Fatal(err)
				}
			case "public-db-directory":
				if err := os.MkdirAll(filepath.Dir(config.OperationDB), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(filepath.Dir(config.OperationDB), 0755); err != nil {
					t.Fatal(err)
				}
			}
			if writeConfig {
				writePolicyApplyJSON(t, path, config)
			}
			resources, err := loadPolicyApplyResources(path, now)
			if resources != nil {
				_ = resources.Close()
				t.Fatal("invalid configuration constructed apply resources")
			}
			if err == nil {
				t.Fatal("invalid config did not fail explicitly")
			}
			if name != "aliased-db" {
				if _, err := os.Stat(config.OperationDB); err == nil {
					t.Fatal("invalid configuration created operation DB")
				}
			}
		})
	}
}
