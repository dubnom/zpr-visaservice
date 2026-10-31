package enrollment

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnrollmentOwnedBASUsesOneStoreAndPreservesTLSRouteBoundaries(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable")
	t.Setenv("ZPR_SIMULATOR_URL", "https://127.0.0.1:1")
	config, _, _, _ := administrationFixture(t)
	bas, roots, identity, other := basServerFixture(t)
	bas.DatabaseFile = config.DatabaseFile
	config.BAS = &bas
	servers, store, err := newEnrollmentServers(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if len(servers) != 4 {
		t.Fatalf("want device, administration and two isolated BAS listeners, got %d", len(servers))
	}
	adapter := httptest.NewUnstartedServer(servers[2].Handler)
	adapter.TLS = servers[2].TLSConfig
	adapter.StartTLS()
	defer adapter.Close()
	token := httptest.NewUnstartedServer(servers[3].Handler)
	token.TLS = servers[3].TLSConfig
	token.StartTLS()
	defer token.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: roots, ServerName: "auth.zpr", MinVersion: tls.VersionTLS13,
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	call := func(origin, host, method, path, contentType string, body []byte) (int, []byte, http.Header) {
		t.Helper()
		request, err := http.NewRequest(method, origin+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Host = host
		request.Header.Set("Content-Type", contentType)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, data, response.Header
	}
	status, _, _ := call(adapter.URL, "auth.zpr:4000", "GET",
		"/preauthorize?response_type=code&client_id=not-enrolled", "", nil)
	if status != http.StatusForbidden {
		t.Fatalf("unapproved identity authenticated: %d", status)
	}
	// This fixture mutation uses the exact Store already owned by the service.
	item, _, runtimeKey := basApprovedDevice(t, store, time.Now().UTC())
	status, data, _ := call(adapter.URL, "auth.zpr:4000", "GET",
		"/preauthorize?response_type=code&client_id="+url.QueryEscape(item.Asset.AssetID), "", nil)
	var nonce struct {
		Nonce string `json:"nonce"`
	}
	if status != http.StatusOK || json.Unmarshal(data, &nonce) != nil || nonce.Nonce == "" {
		t.Fatalf("BAS did not observe owner-store approval: %d %s", status, data)
	}
	body, err := json.Marshal(basSignedAuthorization(t, runtimeKey, item.Asset.AssetID, nonce.Nonce))
	if err != nil {
		t.Fatal(err)
	}
	status, data, header := call(adapter.URL, "auth.zpr:4000", "POST", "/authorize", "application/json", body)
	location, err := url.Parse(header.Get("Location"))
	if status != http.StatusFound || err != nil || location.Query().Get("code") == "" {
		t.Fatalf("retained runtime-key authorization: %d %s %v", status, data, err)
	}
	form := url.Values{
		"grant_type": {"authorization_code"}, "client_id": {item.Asset.AssetID},
		"redirect_url": {"https://auth.zpr"}, "code": {location.Query().Get("code")},
	}
	withPeer := func(peer tls.Certificate) {
		t.Helper()
		next := transport.Clone()
		next.TLSClientConfig = transport.TLSClientConfig.Clone()
		next.TLSClientConfig.Certificates = []tls.Certificate{peer}
		t.Cleanup(next.CloseIdleConnections)
		client.Transport = next
	}
	withPeer(other)
	status, _, _ = call(token.URL, "auth.zpr:3999", "POST", "/token", "application/x-www-form-urlencoded", []byte(form.Encode()))
	if status != http.StatusForbidden {
		t.Fatalf("unapproved CA-trusted token peer accepted: %d", status)
	}
	withPeer(identity)
	status, data, _ = call(token.URL, "auth.zpr:3999", "POST", "/token", "application/x-www-form-urlencoded", []byte(form.Encode()))
	if status != http.StatusOK {
		t.Fatalf("pinned VS could not redeem owner-store code: %d %s", status, data)
	}
	status, _, _ = call(token.URL, "auth.zpr:3999", "POST", "/token", "application/x-www-form-urlencoded", []byte(form.Encode()))
	if status != http.StatusForbidden {
		t.Fatalf("code redeemed twice: %d", status)
	}
	for _, path := range []string{"/v1/invitations", DeviceAPIPrefix + "challenges", "/token"} {
		status, _, _ = call(adapter.URL, "auth.zpr:4000", "POST", path, "application/json", []byte(`{}`))
		if status != http.StatusNotFound {
			t.Fatalf("adapter listener exposed non-adapter route %s: %d", path, status)
		}
	}
	request := httptest.NewRequest("GET", "https://localhost:9443/preauthorize", nil)
	request.Host = "localhost:9443"
	response := httptest.NewRecorder()
	servers[0].Handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatal("device listener exposed BAS routes")
	}
}

func TestEnrollmentOwnedBASConfigurationIsExplicitAndFailsClosed(t *testing.T) {
	for _, name := range []string{"database", "device-listener", "admin-listener", "missing-signing-key"} {
		t.Run(name, func(t *testing.T) {
			config, _, _, _ := administrationFixture(t)
			bas, _, _, _ := basServerFixture(t)
			bas.DatabaseFile = config.DatabaseFile
			config.BAS = &bas
			switch name {
			case "database":
				bas.DatabaseFile = filepath.Join(t.TempDir(), "other.sqlite")
			case "device-listener":
				bas.Adapter.Listen = config.Listen
				_, port, _ := net.SplitHostPort(config.Listen)
				bas.Adapter.Origin = "https://auth.zpr:" + port
			case "admin-listener":
				bas.Token.Listen = config.Administration.Listen
				_, port, _ := net.SplitHostPort(config.Administration.Listen)
				bas.Token.Origin = "https://auth.zpr:" + port
			case "missing-signing-key":
				bas.SigningKeyFile = filepath.Join(t.TempDir(), "missing.key")
			}
			if _, store, err := newEnrollmentServers(config); err == nil {
				store.Close()
				t.Fatal("invalid enrollment-owned BAS configuration accepted")
			}
		})
	}
}

func TestEnrollmentOwnedBASResolvesPathsAndDoesNotCloseBorrowedStoreOnError(t *testing.T) {
	config, _ := serverConfig(t)
	bas, _, _, _ := basServerFixture(t)
	bas.DatabaseFile = config.DatabaseFile
	bas.SigningKeyFile = filepath.Join(t.TempDir(), "missing.key")
	store, err := Open(config.DatabaseFile)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, _, err := newBASServersWithStore(bas, store); err == nil {
		t.Fatal("missing signing key accepted")
	}
	if err := store.db.PingContext(context.Background()); err != nil {
		t.Fatal("BAS failure closed the caller-owned store", err)
	}
	bas.SigningKeyFile = "bas-signing.key"
	bas.DatabaseFile = filepath.Base(config.DatabaseFile)
	config.DatabaseFile = bas.DatabaseFile
	config.BAS = &bas
	path := filepath.Join(t.TempDir(), "service.json")
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadServerConfig(path)
	if err != nil || loaded.BAS.DatabaseFile != loaded.DatabaseFile ||
		!strings.HasPrefix(loaded.BAS.SigningKeyFile, filepath.Dir(path)+string(os.PathSeparator)) {
		t.Fatalf("relative owned BAS paths not resolved against owning config: %+v %v", loaded.BAS, err)
	}
}
