//go:build linux || darwin

package enrollment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupConfig(t *testing.T) SetupConfig {
	t.Helper()
	return SetupConfig{Version: 1, Audience: "https://enroll.example.test",
		StateDirectory: filepath.Join(t.TempDir(), "state"), AllowSoftwareDevelopment: true}
}

func setupRequest(s *SetupServer, path, body string) *httptest.ResponseRecorder {
	method := http.MethodPost
	if body == "" {
		method = http.MethodGet
	}
	r := httptest.NewRequest(method, s.origin+path, strings.NewReader(body))
	r.Header.Set("Origin", s.origin)
	r.Header.Set("X-ZPR-Setup-Session", s.token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.serveHTTP(w, r)
	return w
}

func setupResult(t *testing.T, w *httptest.ResponseRecorder) setupView {
	t.Helper()
	var view setupView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func TestSetupConfigAndStartupFailClosed(t *testing.T) {
	base := setupConfig(t)
	for _, scenario := range []string{"version", "http", "software", "relative", "root", "corrupt-ca"} {
		t.Run(scenario, func(t *testing.T) {
			config := base
			switch scenario {
			case "version":
				config.Version = 2
			case "http":
				config.Audience = "http://unsafe.test"
			case "software":
				config.AllowSoftwareDevelopment = false
			case "relative":
				config.StateDirectory = "state"
			case "root":
				config.StateDirectory = "/"
			case "corrupt-ca":
				config.CAFile = filepath.Join(t.TempDir(), "ca.pem")
				if err := os.WriteFile(config.CAFile, []byte("not a certificate"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if server, err := NewSetupServer(config); err == nil {
				server.Close()
				t.Fatal("unsafe config accepted")
			}
			if _, err := os.Stat(config.StateDirectory); scenario != "root" && !errors.Is(err, os.ErrNotExist) {
				t.Fatal("startup generated state")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "setup.json")
	data, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSetupConfig(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSetupConfig(path); err == nil {
		t.Fatal("writable config accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{`{"secret":"unrecognized"}`, strings.Repeat("x", 65537)} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSetupConfig(path); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
	if _, err := CreateSoftwareIdentity(base.StateDirectory, localMetadata()); err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.Audience = "https://changed.example.test"
	if server, err := NewSetupServer(changed); err == nil {
		server.Close()
		t.Fatal("stored audience override accepted")
	}
}

func TestSetupSessionBoundaryAndValidation(t *testing.T) {
	s, err := NewSetupServer(setupConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	host, _, err := net.SplitHostPort(s.listener.Addr().String())
	if err != nil || host != "127.0.0.1" || !strings.Contains(s.URL(), "/#session=") {
		t.Fatal("listener or capability not loopback/fragment only")
	}
	for _, scenario := range []string{"token", "host", "origin", "no-origin", "fetch-site", "query", "expired", "method"} {
		t.Run(scenario, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, s.origin+"/api/prepare", strings.NewReader(`{"organization":"company","invitation_id":"invitation"}`))
			r.Header.Set("Origin", s.origin)
			r.Header.Set("X-ZPR-Setup-Session", s.token)
			r.Header.Set("Content-Type", "application/json")
			switch scenario {
			case "token":
				r.Header.Set("X-ZPR-Setup-Session", "wrong")
			case "host":
				r.Host = "evil.test"
			case "origin":
				r.Header.Set("Origin", "https://evil.test")
			case "no-origin":
				r.Header.Del("Origin")
			case "fetch-site":
				r.Header.Set("Sec-Fetch-Site", "cross-site")
			case "query":
				r.URL.RawQuery = "code=secret"
			case "expired":
				s.expires = time.Now().Add(-time.Second)
				defer func() { s.expires = time.Now().Add(time.Hour) }()
			case "method":
				r.Method = http.MethodGet
			}
			w := httptest.NewRecorder()
			s.serveHTTP(w, r)
			if w.Code < 400 || s.identity != nil {
				t.Fatalf("boundary accepted: %d %s", w.Code, w.Body.String())
			}
		})
	}
	for _, body := range []string{`{"organization":"company","invitation_id":"invitation","unknown":true}`,
		`{} {}`, `{"code":"secret"}`, strings.Repeat("x", 1025)} {
		if w := setupRequest(s, "/api/prepare", body); w.Code < 400 || s.identity != nil {
			t.Fatalf("invalid prepare accepted: %d", w.Code)
		}
	}
	if w := setupRequest(s, "/api/session", ""); w.Code != 200 || strings.Contains(w.Body.String(), s.token) {
		t.Fatal("session response failed or leaked capability")
	}
	r := httptest.NewRequest(http.MethodGet, s.origin+"/", nil)
	w := httptest.NewRecorder()
	s.serveHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") ||
		!strings.Contains(w.Body.String(), "Development release") || strings.Contains(w.Body.String(), s.token) {
		t.Fatal("page missing isolation or leaks session")
	}
}

func TestSetupTLSClaimRestartApprovalAndBackoff(t *testing.T) {
	config, _ := serverConfig(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	config.Listen, config.Audience = listener.Addr().String(), "https://localhost:"+port
	device, store, err := newDeviceServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	done := make(chan error, 1)
	go func() { done <- device.ServeTLS(listener, "", "") }()
	defer func() { device.Close(); <-done }()
	invitation, code := createInvitation(t, store, time.Now().UTC())
	local := setupConfig(t)
	local.Audience, local.CAFile = config.Audience, config.CertificateFile
	s, err := NewSetupServer(local)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	body, err := json.Marshal(map[string]string{"organization": "company", "invitation_id": invitation.ID})
	if err != nil {
		t.Fatal(err)
	}
	w := setupRequest(s, "/api/prepare", string(body))
	if w.Code != 200 || !setupResult(t, w).CanClaim {
		t.Fatalf("prepare: %d %s", w.Code, w.Body.String())
	}
	if w := setupRequest(s, "/api/prepare", string(body)); w.Code != 409 {
		t.Fatal("prepare replaced identity")
	}
	w = setupRequest(s, "/api/claim", `{"code":"`+code+`"}`)
	view := setupResult(t, w)
	if w.Code != 200 || view.Status.State != "pending_approval" || view.CanClaim || view.RetryAfter < 9 ||
		strings.Contains(w.Body.String(), code) {
		t.Fatalf("claim: %d %s", w.Code, w.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(local.StateDirectory, identityFile))
	if err != nil || bytes.Contains(data, []byte(code)) {
		t.Fatal("code persisted")
	}
	if w := setupRequest(s, "/api/status", `{}`); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("backoff not enforced")
	}
	s.next = time.Time{}
	if w := setupRequest(s, "/api/claim", `{"code":"`+code+`"}`); w.Code != 409 {
		t.Fatal("claim replay accepted")
	}
	if _, err := store.DecideWithRuntimeKey(context.Background(), "company", invitation.ID, "admin", "approved",
		"Verified development setup", view.Fingerprint, view.Status.RuntimeKeyFingerprint, view.Status.Revision, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewSetupServer(local)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if w := setupRequest(restarted, "/api/claim", `{"code":"`+code+`"}`); w.Code != 409 {
		t.Fatal("restart permitted claim before fresh status")
	}
	w = setupRequest(restarted, "/api/status", `{}`)
	view = setupResult(t, w)
	if w.Code != 200 || view.Status.State != "approved" || view.Status.CredentialsIssued || view.Uncertain || view.CanClaim ||
		!strings.Contains(view.Message, "not issued") {
		t.Fatalf("resumed approval: %d %s", w.Code, w.Body.String())
	}
}

func TestSetupUnclaimedRestartRequiresExplicitRecovery(t *testing.T) {
	config, _ := serverConfig(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	config.Listen, config.Audience = listener.Addr().String(), "https://localhost:"+port
	device, store, err := newDeviceServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	done := make(chan error, 1)
	go func() { done <- device.ServeTLS(listener, "", "") }()
	defer func() { device.Close(); <-done }()
	invitation, code := createInvitation(t, store, time.Now().UTC())
	local := setupConfig(t)
	local.Audience, local.CAFile = config.Audience, config.CertificateFile
	if _, err := CreateSoftwareIdentity(local.StateDirectory, LocalEnrollment{
		Audience: local.Audience, Organization: "company", InvitationID: invitation.ID}); err != nil {
		t.Fatal(err)
	}
	s, err := NewSetupServer(local)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w := setupRequest(s, "/api/status", `{}`)
	view := setupResult(t, w)
	if w.Code != 403 || !view.Uncertain || !view.ConfirmRecovery || !view.CanClaim {
		t.Fatalf("status denial: %d %s", w.Code, w.Body.String())
	}
	s.next = time.Time{}
	if w := setupRequest(s, "/api/claim", `{"code":"`+code+`"}`); w.Code != 409 {
		t.Fatal("unconfirmed recovery accepted")
	}
	w = setupRequest(s, "/api/claim", `{"code":"`+code+`","confirm_recovery":true}`)
	if w.Code != 200 || setupResult(t, w).Status.State != "pending_approval" {
		t.Fatalf("explicit recovery: %d %s", w.Code, w.Body.String())
	}
}

func TestSetupRunServesAndShutsDown(t *testing.T) {
	s, err := NewSetupServer(setupConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(s.origin + "/")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || !bytes.Contains(data, []byte("ZPR machine setup")) {
		t.Fatal("setup listener not responsive")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("setup did not shut down")
	}
}

func TestSetupRemoteFailureIsUncertainAndHonorsBackoff(t *testing.T) {
	var calls int
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "120")
		http.Error(w, "remote secret or supplied code", http.StatusServiceUnavailable)
	}))
	defer remote.Close()
	local := setupConfig(t)
	local.Audience = remote.URL
	s, err := NewSetupServer(local)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w := setupRequest(s, "/api/prepare", `{"organization":"company","invitation_id":"invitation"}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	s.client.Close()
	s.client = trustedClient(t, remote, s.identity)
	w = setupRequest(s, "/api/claim", `{"code":"`+strings.Repeat("c", 26)+`"}`)
	view := setupResult(t, w)
	if w.Code != 502 || !view.Uncertain || view.CanClaim || view.RetryAfter < 119 ||
		strings.Contains(w.Body.String(), "remote secret") || calls != 1 {
		t.Fatalf("failure/backoff not surfaced safely: %d %s", w.Code, w.Body.String())
	}
	if w := setupRequest(s, "/api/status", `{}`); w.Code != 429 || calls != 1 {
		t.Fatal("server backoff bypassed")
	}
	s.next = time.Time{}
	if w := setupRequest(s, "/api/claim", `{"code":"`+strings.Repeat("c", 26)+`"}`); w.Code != 409 || calls != 1 {
		t.Fatal("uncertain claim replayed")
	}
}

func TestSetupBusyOperationDoesNotQueueMutations(t *testing.T) {
	s, err := NewSetupServer(setupConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.mu.Lock()
	w := setupRequest(s, "/api/prepare", `{"organization":"company","invitation_id":"invitation"}`)
	s.mu.Unlock()
	if w.Code != 409 || w.Header().Get("Retry-After") != "2" || s.identity != nil {
		t.Fatal("busy operation was not rejected")
	}
}
