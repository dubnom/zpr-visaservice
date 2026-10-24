package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (string, inventory) {
	t.Helper()
	state := filepath.Join(t.TempDir(), "companion")
	inv := inventory{Organization: "evaluation_install", Source: "synthetic_source", StarterSource: "directory_install", Port: 19446}
	if err := initialize(state, inv); err != nil {
		t.Fatal(err)
	}
	return state, inv
}
func TestCompanionPrivateStateNeverReseedsStarter(t *testing.T) {
	state, inv := fixture(t)
	before, err := os.ReadFile(filepath.Join(state, "directory.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := initialize(state, inv); err == nil {
		t.Fatal("existing state reseeded")
	}
	after, _ := os.ReadFile(filepath.Join(state, "directory.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("existing state changed")
	}
	for _, name := range []string{"companion.json", "directory.json", "source-key.pem", "source-token.secret", "source-ca.pem"} {
		info, err := os.Stat(filepath.Join(state, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("unprotected %s", name)
		}
	}
	collision := inv
	collision.Source = inv.StarterSource
	if err := validate(collision); err == nil {
		t.Fatal("starter source collision accepted")
	}
	if _, err := load(state); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(filepath.Dir(state), "link")
	if err := os.Symlink(state, link); err != nil {
		t.Fatal(err)
	}
	if _, err := load(link); err == nil {
		t.Fatal("symlink state accepted")
	}
}
func TestCompanionPresentAbsentAndExplicitRemovalSmoke(t *testing.T) {
	state, inv := fixture(t)
	inv.Port = 0
	directory, listener, err := directoryServer(state, inv)
	if err != nil {
		t.Fatal(err)
	}
	inv.Port = listener.Addr().(*net.TCPAddr).Port
	done := make(chan error, 1)
	go func() {
		done <- directory.ServeTLS(listener, filepath.Join(state, "source-ca.pem"), filepath.Join(state, "source-key.pem"))
	}()
	t.Cleanup(func() { directory.Close(); <-done })
	roots := x509.NewCertPool()
	ca, _ := os.ReadFile(filepath.Join(state, "source-ca.pem"))
	roots.AppendCertsFromPEM(ca)
	reader := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}
	defer reader.CloseIdleConnections()
	sourceURL := fmt.Sprintf("https://127.0.0.1:%d/directory/%s", inv.Port, inv.Organization)
	unauthorized, err := reader.Get(sourceURL)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != 401 {
		t.Fatal("directory permits unauthenticated read")
	}
	token, _ := os.ReadFile(filepath.Join(state, "source-token.secret"))
	readDirectory := func() error {
		request, _ := http.NewRequest(http.MethodGet, sourceURL, nil)
		request.Header.Set("Authorization", "Bearer "+string(token))
		response, err := reader.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		if response.StatusCode != 200 || !bytes.Contains(data, []byte("synthetic-companion-user")) {
			return fmt.Errorf("invalid directory: %d", response.StatusCode)
		}
		return nil
	}
	starter := map[string]any{"name": inv.StarterSource, "kind": "ldap"}
	current := registry{Organization: inv.Organization, Revision: "r1", Durable: true}
	current.Sources = append(current.Sources, struct {
		Source   string         `json:"source_id"`
		Editable bool           `json:"editable"`
		Config   map[string]any `json:"config"`
	}{inv.StarterSource, false, starter})
	var methods []string
	service := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(current)
			return
		}
		var payload registration
		if json.NewDecoder(r.Body).Decode(&payload) != nil {
			w.WriteHeader(400)
			return
		}
		if payload.Organization != inv.Organization || payload.Source != inv.Source || payload.Revision != current.Revision {
			w.WriteHeader(409)
			return
		}
		if r.URL.Path == "/api/assertions/sources/test" {
			if err := readDirectory(); err != nil {
				w.WriteHeader(502)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"organization_id": inv.Organization, "source_id": inv.Source, "status": "success"})
			return
		}
		switch r.Method {
		case http.MethodPost:
			current.Sources = append(current.Sources, struct {
				Source   string         `json:"source_id"`
				Editable bool           `json:"editable"`
				Config   map[string]any `json:"config"`
			}{payload.Source, true, payload.Config})
		case http.MethodDelete:
			if len(current.Sources) != 2 || current.Sources[1].Source != inv.Source {
				w.WriteHeader(409)
				return
			}
			current.Sources = current.Sources[:1]
		default:
			w.WriteHeader(405)
			return
		}
		current.Revision += "x"
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}))
	defer service.Close()
	api := contract{service.Client(), service.URL}
	ctx := context.Background()
	if err := reconcile(ctx, api, state, inv, false); err != nil {
		t.Fatal(err)
	}
	if len(current.Sources) != 2 {
		t.Fatal("companion not registered alongside starter")
	}
	if err := reconcile(ctx, api, state, inv, false); err != nil {
		t.Fatal("retry not reconciled", err)
	}
	directory.Close()
	if err := reconcile(ctx, api, state, inv, false); err == nil {
		t.Fatal("absent companion passed connection test")
	}
	if len(current.Sources) != 2 || current.Sources[0].Config["kind"] != "ldap" {
		t.Fatal("companion absence mutated starter/registrations")
	}
	if err := reconcile(ctx, api, state, inv, true); err != nil {
		t.Fatal(err)
	}
	if len(current.Sources) != 1 || current.Sources[0].Source != inv.StarterSource {
		t.Fatal("removal touched starter")
	}
	if err := reconcile(ctx, api, state, inv, true); err == nil {
		t.Fatal("absent source silently removed")
	}
	for _, method := range methods {
		if strings.Contains(method, "simulator") || strings.Contains(method, "organization") {
			t.Fatal("private setup API used", method)
		}
	}
}
func TestCompanionFailsClosedWithoutPublicContract(t *testing.T) {
	state, inv := fixture(t)
	cases := []struct {
		name     string
		status   int
		response string
	}{
		{"unauthorized", 401, `{}`}, {"unavailable", 503, `{}`}, {"missing", 404, `{}`},
		{"wrong organization", 200, `{"organization_id":"other_org","revision":"r1","durable":true}`},
		{"nondurable", 200, `{"organization_id":"evaluation_install","revision":"r1","durable":false}`},
		{"missing revision", 200, `{"organization_id":"evaluation_install","durable":true}`},
		{"identity collision", 200, `{"organization_id":"evaluation_install","revision":"r1","durable":true,"sources":[{"source_id":"synthetic_source","editable":true,"config":{"kind":"ldap"}}]}`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet {
					t.Error("mutated after failed preflight")
				}
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.response)
			}))
			defer server.Close()
			if err := reconcile(context.Background(), contract{server.Client(), server.URL}, state, inv, false); err == nil {
				t.Fatal("preflight succeeded")
			}
			if calls != 1 {
				t.Fatal("fallback or retry after incompatible contract")
			}
		})
	}
	if _, err := newContract("http://localhost:1", "", "", ""); err == nil {
		t.Fatal("cleartext contract accepted")
	}
	if _, err := newContract("https://user:secret@localhost:1", "", "", ""); err == nil {
		t.Fatal("URL credentials accepted")
	}
	if _, err := newContract("https://localhost:1", "missing", "missing", "missing"); err == nil {
		t.Fatal("missing trust/identity accepted")
	}
}

func TestCompanionInitUsesOnlyRuntimeOrganization(t *testing.T) {
	for _, tt := range []struct {
		name, source, starter string
		status                int
		body                  string
		want                  bool
	}{
		{"same evaluation organization", "synthetic_source", "directory_install", 200, `{"organization_id":"evaluation_install","revision":"r1","durable":true,"sources":[{"source_id":"directory_install"},{"source_id":"company_directory"}]}`, true},
		{"company collision", "company_directory", "directory_install", 200, `{"organization_id":"evaluation_install","revision":"r1","durable":true,"sources":[{"source_id":"directory_install"},{"source_id":"company_directory"}]}`, false},
		{"starter collision", "directory_install", "directory_install", 200, `{"organization_id":"evaluation_install","revision":"r1","durable":true,"sources":[{"source_id":"directory_install"}]}`, false},
		{"starter missing", "synthetic_source", "directory_install", 200, `{"organization_id":"evaluation_install","revision":"r1","durable":true,"sources":[]}`, false},
		{"contract unavailable", "synthetic_source", "directory_install", 503, `{}`, false},
		{"not authenticated", "synthetic_source", "directory_install", 401, `{}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := filepath.Join(t.TempDir(), "separate")
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/assertions/sources" {
					t.Error("private/mutation contract during init")
				}
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			defer server.Close()
			err := initializeFromContract(context.Background(), contract{server.Client(), server.URL}, state, tt.source, tt.starter, 19446)
			if (err == nil) != tt.want {
				t.Fatalf("init: %v", err)
			}
			if tt.want {
				inv, err := load(state)
				if err != nil || inv.Organization != "evaluation_install" || inv.Source != tt.source {
					t.Fatalf("runtime organization not retained: %+v %v", inv, err)
				}
			} else {
				if _, err := os.Stat(state); !os.IsNotExist(err) {
					t.Fatal("failed public preflight provisioned local state")
				}
			}
		})
	}
}

func TestCompanionRemovalRequiresPublicDeleteWithoutFallback(t *testing.T) {
	state, inv := fixture(t)
	payload := sourceRegistration(state, inv)
	methods := []string{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{"organization_id": inv.Organization, "revision": "r1", "durable": true, "sources": []any{map[string]any{"source_id": inv.Source, "editable": true, "config": payload.Config}}})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()
	if err := reconcile(context.Background(), contract{server.Client(), server.URL}, state, inv, true); err == nil {
		t.Fatal("missing public DELETE reported success")
	}
	if strings.Join(methods, ",") != "GET,DELETE" {
		t.Fatal("removal attempted fallback", methods)
	}
	if _, err := load(state); err != nil {
		t.Fatal("failed unregister removed private state", err)
	}
}
