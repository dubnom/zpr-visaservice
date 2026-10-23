package adapterlogs

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenObserveOrdinaryVerifiedSourceCoexists(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "admin@example.test" || password != "private-secret" {
			w.WriteHeader(401)
			return
		}
		if r.Method != http.MethodPost {
			t.Error("unexpected method")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/default/events/_json":
			w.Write([]byte(`{"code":200,"status":[{"successful":1,"failed":0}]}`))
		case "/api/default/_search":
			w.Write([]byte(`{"hits":[{"event":"ready","label":"EVALUATION ONLY"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	password := filepath.Join(root, "password")
	ca := filepath.Join(root, "ca.pem")
	os.WriteFile(password, []byte("private-secret"), 0600)
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	source := SourceConfig{Name: "Evaluation logs", Kind: "controller", URL: server.URL, CAFile: ca, Username: "admin@example.test", PasswordFile: password, Organization: "default", Stream: "events"}
	config := Inventory{Adapters: []Target{{ID: "starter", Name: "Evaluation", Sources: []SourceConfig{source}}, {ID: "company", Name: "Company", Sources: []SourceConfig{{Name: "Company log", Kind: "adapter", File: "/company/log"}}}}}
	if _, err := ReadInventory(inventoryFile(t, encodedInventory(t, config))); err != nil {
		t.Fatal(err)
	}
	if err := Append(context.Background(), source, []byte(`[{"event":"ready"}]`)); err != nil {
		t.Fatal(err)
	}
	output, err := Query(context.Background(), source)
	if err != nil || !strings.Contains(output, "EVALUATION ONLY") {
		t.Fatalf("query %q %v", output, err)
	}
	untrusted := source
	untrusted.CAFile = ""
	if _, err := Query(context.Background(), untrusted); err == nil {
		t.Fatal("untrusted TLS accepted")
	}
	os.Chmod(password, 0644)
	if _, err := Query(context.Background(), source); err == nil || strings.Contains(err.Error(), "private-secret") {
		t.Fatal("unprotected credential accepted or leaked")
	}
	os.Chmod(password, 0600)
	server.Close()
	if _, err := Query(context.Background(), source); err == nil {
		t.Fatal("unavailable logs appear healthy")
	}
}

func TestOpenObserveRejectsUnsafeConfiguration(t *testing.T) {
	base := SourceConfig{Name: "logs", Kind: "controller", URL: "https://logs.example.test", Username: "admin", PasswordFile: "/private/password", Organization: "default", Stream: "events"}
	for _, change := range []func(*SourceConfig){func(s *SourceConfig) { s.URL = "http://logs.example.test" }, func(s *SourceConfig) { s.URL = "https://user:secret@logs.example.test" }, func(s *SourceConfig) { s.Stream = "invalid stream" }, func(s *SourceConfig) { s.Container = "company" }, func(s *SourceConfig) { s.PasswordFile = "relative" }} {
		source := base
		change(&source)
		config := Inventory{Adapters: []Target{{ID: "logs", Name: "Logs", Sources: []SourceConfig{source}}}}
		if _, err := ReadInventory(inventoryFile(t, encodedInventory(t, config))); err == nil {
			t.Fatalf("unsafe config accepted: %+v", source)
		}
	}
}
