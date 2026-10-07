package enrollment

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func apiConfig() Config {
	return Config{Version: 1, InvitationLifetimeSeconds: 3600,
		Organizations: map[string]Organization{
			"company":       {Profiles: []string{"standard"}, Types: []string{"laptop"}},
			"other-company": {Profiles: []string{"server"}, Types: []string{"server"}},
		},
		Principals: []Principal{
			{Name: "admin", CertificateSHA256: digest("admin-cert"), Organizations: []string{"company"}, Permissions: []string{"read", "create", "cancel"}},
			{Name: "reader", CertificateSHA256: digest("reader-cert"), Organizations: []string{"company"}, Permissions: []string{"read"}},
		},
	}
}

func requestAPI(handler http.Handler, method, path string, body []byte, cert string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if cert != "" {
		leaf := &x509.Certificate{Raw: []byte(cert)}
		request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	}
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func testAPI(t *testing.T) (*Store, http.Handler) {
	t.Helper()
	store := testStore(t)
	handler, err := NewAdminHandler(store, apiConfig())
	if err != nil {
		t.Fatal(err)
	}
	return store, handler
}

func assetJSON(t *testing.T, asset Asset) []byte {
	t.Helper()
	data, err := json.Marshal(asset)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAdminAPICreateReadCancel(t *testing.T) {
	store, handler := testAPI(t)
	response := requestAPI(handler, "POST", APIPrefix+"invitations", assetJSON(t, testAsset()), "admin-cert")
	if response.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("secret response may be cached")
	}
	var created struct {
		Invitation Invitation `json:"invitation"`
		Code       string     `json:"enrollment_code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if len(created.Code) != 26 || created.Invitation.CreatedBy != "admin" ||
		created.Invitation.ExpiresAt.Sub(created.Invitation.CreatedAt) != time.Hour {
		t.Fatalf("incorrect creation result: %+v", created.Invitation)
	}
	for _, path := range []string{APIPrefix + "invitations?organization=company", APIPrefix + "invitations/" + created.Invitation.ID + "?organization=company"} {
		response := requestAPI(handler, "GET", path, nil, "reader-cert")
		if response.Code != http.StatusOK {
			t.Fatalf("read: %d %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), created.Code) || strings.Contains(response.Body.String(), digest(created.Code)) {
			t.Fatal("read leaked invitation secret or verifier")
		}
	}
	response = requestAPI(handler, "POST", APIPrefix+"invitations", assetJSON(t, testAsset()), "admin-cert")
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d %s", response.Code, response.Body.String())
	}
	response = requestAPI(handler, "POST", APIPrefix+"invitations/"+created.Invitation.ID+"/cancel", []byte(`{"organization":"company"}`), "admin-cert")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"cancelled"`) {
		t.Fatalf("cancel = %d %s", response.Code, response.Body.String())
	}
	var count int
	if err := store.db.QueryRow(`SELECT count(*) FROM audit WHERE action='cancelled' AND principal='admin'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("cancellation audit count = %d", count)
	}
}

func TestAdminAPIDeniesUnverifiedAndSpoofedPrincipals(t *testing.T) {
	_, handler := testAPI(t)
	for _, scenario := range []string{"missing", "unverified", "unknown", "mismatched-chain", "browser"} {
		t.Run(scenario, func(t *testing.T) {
			r := httptest.NewRequest("GET", APIPrefix+"catalog", nil)
			r.Header.Set("X-Enrollment-Principal", "admin")
			r.Header.Set("X-Forwarded-Client-Cert", "admin-cert")
			r.Header.Set("Authorization", "admin-cert")
			leaf := &x509.Certificate{Raw: []byte("admin-cert")}
			switch scenario {
			case "unverified":
				r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}
			case "unknown":
				other := &x509.Certificate{Raw: []byte("control-room-service-cert")}
				r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{other}, VerifiedChains: [][]*x509.Certificate{{other}}}
			case "mismatched-chain":
				r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{{Raw: []byte("other")}}}}
			case "browser":
				r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
				r.Header.Set("Origin", "https://control.example.test")
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAdminAPIOrganizationAndPermissionIsolation(t *testing.T) {
	_, handler := testAPI(t)
	asset := testAsset()
	asset.Organization = "other-company"
	tests := []struct {
		method, path, cert string
		body               []byte
	}{
		{"POST", APIPrefix + "invitations", "admin-cert", assetJSON(t, asset)},
		{"POST", APIPrefix + "invitations", "reader-cert", assetJSON(t, testAsset())},
		{"GET", APIPrefix + "invitations?organization=other-company", "admin-cert", nil},
		{"GET", APIPrefix + "invitations/unknown?organization=other-company", "admin-cert", nil},
		{"POST", APIPrefix + "invitations/unknown/cancel", "admin-cert", []byte(`{"organization":"other-company"}`)},
		{"POST", APIPrefix + "invitations/unknown/cancel", "reader-cert", []byte(`{"organization":"company"}`)},
	}
	for _, item := range tests {
		response := requestAPI(handler, item.method, item.path, item.body, item.cert)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s %s = %d", item.method, item.path, response.Code)
		}
	}
	response := requestAPI(handler, "GET", APIPrefix+"catalog", nil, "reader-cert")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "other-company") ||
		!strings.Contains(response.Body.String(), `"gui_mutations_enabled":false`) {
		t.Fatalf("catalog = %d %s", response.Code, response.Body.String())
	}
}

func TestAdminAPIRequestValidation(t *testing.T) {
	_, handler := testAPI(t)
	badProfile := testAsset()
	badProfile.Profile = "super-admin"
	tests := []struct {
		body []byte
		want int
	}{
		{[]byte(`{"organization":"company","created_by":"admin"}`), http.StatusBadRequest},
		{append(assetJSON(t, testAsset()), []byte(` {}`)...), http.StatusBadRequest},
		{[]byte(`{"organization":`), http.StatusBadRequest},
		{[]byte(`null`), http.StatusForbidden},
		{assetJSON(t, badProfile), http.StatusBadRequest},
		{[]byte(`{"name":"` + strings.Repeat("x", 9000) + `"}`), http.StatusRequestEntityTooLarge},
	}
	for _, item := range tests {
		response := requestAPI(handler, "POST", APIPrefix+"invitations", item.body, "admin-cert")
		if response.Code != item.want {
			t.Fatalf("invalid request = %d %s, want %d", response.Code, response.Body.String(), item.want)
		}
	}
	response := requestAPI(handler, "POST", APIPrefix+"invitations?code=secret", assetJSON(t, testAsset()), "admin-cert")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("mutation query = %d", response.Code)
	}
	response = requestAPI(handler, "GET", APIPrefix+"invitations?organization=company&limit=101", nil, "admin-cert")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unbounded listing = %d", response.Code)
	}
	r := httptest.NewRequest("POST", APIPrefix+"invitations", bytes.NewReader(assetJSON(t, testAsset())))
	leaf := &x509.Certificate{Raw: []byte("admin-cert")}
	r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	r.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("content type = %d", w.Code)
	}
}

func TestAdminAPIPaginationAndNoDeviceClaimEndpoint(t *testing.T) {
	_, handler := testAPI(t)
	for _, id := range []string{"a", "b", "c"} {
		asset := testAsset()
		asset.AssetID = id
		response := requestAPI(handler, "POST", APIPrefix+"invitations", assetJSON(t, asset), "admin-cert")
		if response.Code != http.StatusCreated {
			t.Fatal(response.Body.String())
		}
	}
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < 3; page++ {
		response := requestAPI(handler, "GET", APIPrefix+"invitations?organization=company&limit=1&after="+cursor, nil, "admin-cert")
		if response.Code != http.StatusOK {
			t.Fatal(response.Body.String())
		}
		var result struct {
			Invitations []Invitation `json:"invitations"`
			Next        string       `json:"next_after"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Invitations) != 1 || seen[result.Invitations[0].ID] {
			t.Fatalf("invalid page: %+v", result)
		}
		seen[result.Invitations[0].ID] = true
		cursor = result.Next
	}
	if cursor != "" {
		t.Fatal("last page has a cursor")
	}
	response := requestAPI(handler, "POST", APIPrefix+"claim", []byte(`{}`), "admin-cert")
	if response.Code != http.StatusNotFound {
		t.Fatalf("unexpected device claim route = %d", response.Code)
	}
}

func TestAdminConfigValidation(t *testing.T) {
	for _, scenario := range []string{"version", "lifetime", "pin", "unknown-org", "permission", "duplicate", "profile"} {
		t.Run(scenario, func(t *testing.T) {
			config := apiConfig()
			switch scenario {
			case "version":
				config.Version = 2
			case "lifetime":
				config.InvitationLifetimeSeconds = 0
			case "pin":
				config.Principals[0].CertificateSHA256 = "CN=admin"
			case "unknown-org":
				config.Principals[0].Organizations = []string{"unknown"}
			case "permission":
				config.Principals[0].Permissions = []string{"approve"}
			case "duplicate":
				config.Principals = append(config.Principals, config.Principals[0])
			case "profile":
				config.Organizations["company"] = Organization{Types: []string{"laptop"}}
			}
			if config.Validate() == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
	config := apiConfig()
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeConfig(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeConfig(strings.NewReader(`{"version":1,"unknown":true}`)); err == nil {
		t.Fatal("unknown config field accepted")
	}
}
