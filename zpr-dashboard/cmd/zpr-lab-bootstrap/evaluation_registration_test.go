package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvaluationRegistrationPublicContractAndRestart(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install")
	inventory, err := prepareEvaluation(state, testEvaluationConfig(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	expected := evaluationSourceRegistration(state, inventory)
	saved := false
	creates := 0
	tests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/assertions/sources":
			sources := []any{map[string]any{"source_id": "company", "editable": true}}
			if saved {
				sources = append(sources, map[string]any{"source_id": expected.SourceID, "editable": true, "config": expected.Config})
			}
			json.NewEncoder(w).Encode(map[string]any{"organization_id": expected.OrganizationID, "revision": "opaque", "durable": true, "sources": sources})
		case r.Method == http.MethodPost:
			var body evaluationRegistration
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.SourceID != expected.SourceID || body.OrganizationID != expected.OrganizationID || body.Revision != "opaque" {
				t.Error("registration identity/revision lost")
			}
			if r.URL.Path == "/api/assertions/sources/test" {
				tests++
			} else if r.URL.Path == "/api/assertions/sources" {
				creates++
				saved = true
			} else {
				t.Error("unexpected mutation")
			}
			w.Write([]byte(`{"status":"success"}`))
		case r.URL.Path == "/api/assertions/source":
			if r.URL.Query().Get("source") != expected.SourceID {
				t.Error("implicit default read")
			}
			json.NewEncoder(w).Encode(map[string]string{"organization_id": expected.OrganizationID, "source_id": expected.SourceID})
		default:
			t.Error("unexpected contract")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	for range 2 {
		if err := registerEvaluationSourceWithClient(context.Background(), state, inventory, server.Client(), server.URL); err != nil {
			t.Fatal(err)
		}
	}
	if creates != 1 || tests != 2 {
		t.Fatalf("creates=%d tests=%d", creates, tests)
	}
	os.WriteFile(filepath.Join(state, "assertion-sources.json"), []byte(`{"default_source":"company","sources":[]}`), 0600)
	if _, err := prepareEvaluation(state, testEvaluationConfig(), time.Now()); err != nil {
		t.Fatal("mutable registry frozen", err)
	}
	data, _ := os.ReadFile(filepath.Join(state, "adapter-logs.json"))
	if !strings.Contains(string(data), "EVALUATION ONLY") || !strings.Contains(string(data), "logging-admin.secret") {
		t.Fatal("missing ordinary log identity/secret reference")
	}
}
