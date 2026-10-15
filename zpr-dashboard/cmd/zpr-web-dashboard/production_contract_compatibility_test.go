package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/controlservice/gatewaydraft"
	"neboagency.com/zpr-dashborad/internal/enrollment"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type productionContractAuthorizer func(*http.Request, string, string) (operatorauth.Identity, error)

func (authorize productionContractAuthorizer) Authorize(r *http.Request, organization, permission string) (operatorauth.Identity, error) {
	return authorize(r, organization, permission)
}

func unavailableSimulatorForProductionContract(t *testing.T) {
	t.Helper()
	for _, key := range []string{"SIMULATION_MANIFEST", "SIMULATION_ORGANIZATIONS_DIR", "SIMULATION_WORKSPACE_DB_DIR"} {
		t.Setenv(key, filepath.Join(t.TempDir(), "absent"))
	}
	for _, key := range []string{"SIMULATOR_URL", "ZPR_SIMULATOR_URL"} {
		t.Setenv(key, "http://127.0.0.1:1")
	}
	t.Setenv("ZPR_CONTROL_ROOM_PROXY_IP", "")
}

func TestProductionContractGatewayAuthorizationWiring(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	for _, test := range []struct {
		method, path, permission string
	}{
		{"GET", "/api/gateways/contracts", "gateway.read"},
		{"HEAD", "/api/gateways/contracts", "gateway.read"},
		{"GET", "/api/gateways/configs", "gateway.read"},
		{"GET", "/api/gateways/configs/public-egress", "gateway.read"},
		{"POST", "/api/gateways/config/check", "gateway.analyze"},
		{"POST", "/api/gateways/configs/public-egress/revisions", "gateway.edit"},
		{"GET", "/api/gateways/assistant", "gateway.read"},
		{"POST", "/api/gateways/assistant", "gateway.analyze"},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			allowed, called, checks := false, false, 0
			auth := productionContractAuthorizer(func(r *http.Request, organization, permission string) (operatorauth.Identity, error) {
				checks++
				if organization != "production" || permission != test.permission || r.Method != test.method {
					t.Fatalf("authorization = %s %s %s", organization, permission, r.Method)
				}
				if !allowed {
					return operatorauth.Identity{}, errors.New("denied")
				}
				return operatorauth.Identity{Subject: "operator"}, nil
			})
			handler := controlRoomAPIProxy(auth, " production ", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			}))
			for _, status := range []int{http.StatusForbidden, http.StatusNoContent} {
				request := httptest.NewRequest(test.method, test.path, nil)
				request.RemoteAddr, request.Host = "127.0.0.1:43210", "localhost"
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != status || called != allowed || response.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("status=%d called=%t headers=%v", response.Code, called, response.Header())
				}
				allowed = true
			}
			if checks != 2 {
				t.Fatalf("authorization calls = %d", checks)
			}
		})
	}
}

func TestProductionContractGatewayPersistenceAndInventoryFailure(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	root := t.TempDir()
	store, err := newGatewayConfigStore(root)
	if err != nil {
		t.Fatal(err)
	}
	data := snapshot{APIStatus: "connected",
		Actors:   []actor{{CN: "gateway-public-egress"}},
		Services: []service{{Name: "public-egress.svc.zpr", ActorCN: "gateway-public-egress", Kind: "Gateway"}}}
	read := func(context.Context) (gatewaydraft.Inventory, error) { return gatewayInventoryFromSnapshot(data) }
	config := gatewayInstanceConfigJSON(t, nil)
	path := "/api/gateways/configs/public-egress/revisions"
	response := gatewayAPIRequest(newGatewayAPI("northstar", read, store), "POST", path, gatewayConfigSaveBody(t, 0, config))
	if response.Code != http.StatusOK {
		t.Fatalf("save = %d %s", response.Code, response.Body)
	}
	reopened, err := newGatewayConfigStore(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := reopened.Get("northstar", "public-egress")
	if err != nil || record.CurrentRevision != 1 || len(record.Revisions) != 1 || !bytes.Equal(record.Revisions[0].Config, config) {
		t.Fatalf("reopened = %+v %v", record, err)
	}
	if _, err := reopened.Get("other", "public-egress"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cross-organization read = %v", err)
	}
	data.Errors = []string{"inventory unavailable"}
	handler := newGatewayAPI("northstar", read, reopened)
	response = gatewayAPIRequest(handler, "POST", path, gatewayConfigSaveBody(t, 1, config))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unhealthy save = %d %s", response.Code, response.Body)
	}
	response = gatewayAPIRequest(handler, "GET", "/api/gateways/configs/public-egress", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("offline draft read = %d %s", response.Code, response.Body)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &record); err != nil || record.CurrentRevision != 1 || len(record.Revisions) != 1 {
		t.Fatalf("failed save changed draft = %+v %v", record, err)
	}
}

func TestProductionContractEnrollmentConfigurationAndReopen(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	t.Setenv("ZPR_OPERATOR_DELEGATION_TRUST_FILE", "")
	ca, key, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	certificate, _, err := createTestPolicyLeaf(ca, key, "contract-admin", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(leaf.Raw)
	config := enrollment.Config{Version: 1, InvitationLifetimeSeconds: 600,
		Principals: []enrollment.Principal{{Name: "contract-admin", CertificateSHA256: hex.EncodeToString(pin[:]),
			Organizations: []string{"production"}, Permissions: []string{"read", "create"}}},
		Organizations: map[string]enrollment.Organization{"production": {Profiles: []string{"managed"}, Types: []string{"laptop"}}}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	configPath := filepath.Join(directory, "enrollment.json")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_ENROLLMENT_CONFIG_FILE", configPath)
	t.Setenv("ZPR_ENROLLMENT_DATABASE_FILE", filepath.Join(directory, "registry.sqlite"))
	open := func() (http.Handler, func()) {
		t.Helper()
		handler, closeStore, err := configuredEnrollmentHandler()
		if err != nil {
			t.Fatal(err)
		}
		closed := false
		close := func() {
			if !closed {
				closed = true
				if err := closeStore(); err != nil {
					t.Error(err)
				}
			}
		}
		t.Cleanup(close)
		return handler, close
	}
	call := func(handler http.Handler, method, path string, body any, verified bool) *httptest.ResponseRecorder {
		t.Helper()
		var data []byte
		if body != nil {
			data, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		request := httptest.NewRequest(method, path, bytes.NewReader(data))
		request.Header.Set("Content-Type", "application/json")
		request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}
		if verified {
			request.TLS.VerifiedChains = [][]*x509.Certificate{{leaf, ca}}
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	handler, closeStore := open()
	asset := enrollment.Asset{Organization: "production", AssetID: "contract-laptop", Name: "Contract laptop",
		Owner: "Operations", Type: "laptop", Profile: "managed", Recipient: "operator@example.test"}
	if response := call(handler, "POST", enrollment.APIPrefix+"invitations", asset, true); response.Code != http.StatusConflict {
		t.Fatalf("missing readiness = %d %s", response.Code, response.Body)
	}
	preflight := enrollment.PreflightAttestation{Organization: "production", Mode: "production",
		HostLabel: "operator-services", HostAddress: "10.20.30.40", DeliveryChannel: "production-portal",
		DNSVerified: true, ACMEVerified: true, TLSVerified: true, NetworkVerified: true, DockerVerified: true,
		ListenersVerified: true, PackageVerified: true, RecoveryVerified: true, BackupVerified: true}
	if response := call(handler, "POST", enrollment.APIPrefix+"preflight", preflight, true); response.Code != http.StatusOK {
		t.Fatalf("preflight = %d %s", response.Code, response.Body)
	}
	if response := call(handler, "POST", enrollment.APIPrefix+"invitations", asset, true); response.Code != http.StatusCreated {
		t.Fatalf("invitation = %d %s", response.Code, response.Body)
	}
	closeStore()
	handler, _ = open()
	for _, test := range []struct {
		path     string
		verified bool
		status   int
		contains string
	}{
		{"invitations?organization=production", false, 403, "verified client certificate"},
		{"invitations?organization=other", true, 403, "not authorized"},
		{"invitations?organization=production", true, 200, `"asset_id":"contract-laptop"`},
		{"preflight?organization=production", true, 200, `"ready":true`},
	} {
		response := call(handler, "GET", enrollment.APIPrefix+test.path, nil, test.verified)
		if response.Code != test.status || !strings.Contains(response.Body.String(), test.contains) || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s = %d %s", test.path, response.Code, response.Body)
		}
	}
}

func TestProductionContractReviewCaptureResponseLimit(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	contract := gatewayInstanceContract{OrganizationID: "northstar", InstanceID: "public-egress",
		AdapterCN: "gateway-public-egress", ServiceName: "public-egress.svc.zpr"}
	contracts, err := json.Marshal(gatewayContractsResponse{OrganizationID: "northstar", Contracts: []gatewayInstanceContract{contract}})
	if err != nil {
		t.Fatal(err)
	}
	config := gatewayInstanceConfigJSON(t, nil)
	for _, test := range []struct {
		name    string
		size    int
		status  int
		invalid bool
		wantOK  bool
	}{
		{"exact limit", 512 << 10, 200, false, true},
		{"one byte over", (512 << 10) + 1, 200, false, false},
		{"upstream unavailable", len(contracts), 503, false, false},
		{"invalid JSON", len(contracts), 200, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			reads := 0
			capture := changeReviewDomainCapture(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, ok := r.Context().Deadline(); !ok {
					t.Fatal("capture omitted deadline")
				}
				switch r.URL.Path {
				case "/api/gateways/contracts":
					payload := append(append([]byte(nil), contracts...), bytes.Repeat([]byte(" "), test.size-len(contracts))...)
					if test.invalid {
						payload[0] = '!'
					}
					w.WriteHeader(test.status)
					_, _ = w.Write(payload)
				case "/api/gateways/configs/public-egress":
					reads++
					writeJSON(w, 200, gatewayConfigRecord{OrganizationID: "northstar", InstanceID: "public-egress", CurrentRevision: 1,
						Revisions: []gatewayConfigRevision{{Revision: 1, Config: config}}})
				default:
					t.Fatalf("unexpected capture route %s", r.URL.Path)
				}
			}))
			proposal, err := capture(context.Background(), "northstar", "gateway", "public-egress", 1)
			if (err == nil) != test.wantOK || (reads == 1) != test.wantOK {
				t.Fatalf("capture reads=%d err=%v", reads, err)
			}
			if test.wantOK && proposal.After != string(config) {
				t.Fatal("capture changed saved source")
			}
		})
	}
}
