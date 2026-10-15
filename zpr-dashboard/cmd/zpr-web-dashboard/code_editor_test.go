package main

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCodeEditorColorOnlyCSP(t *testing.T) {
	response := httptest.NewRecorder()
	securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	policy := response.Header().Get("Content-Security-Policy")
	for _, required := range []string{"default-src 'self'", "style-src 'self';", "script-src 'self';", "style-src-attr 'unsafe-hashes'"} {
		if !strings.Contains(policy, required) {
			t.Fatalf("CSP missing %q: %s", required, policy)
		}
	}
	for _, forbidden := range []string{"'unsafe-inline'", "'unsafe-eval'", "https:", "data:;"} {
		if strings.Contains(policy, forbidden) {
			t.Fatalf("CSP unexpectedly permits %q: %s", forbidden, policy)
		}
	}
	for _, color := range []string{"229, 166, 108", "215, 167, 207", "130, 145, 136", "216, 189, 128", "146, 197, 223", "194, 216, 138", "184, 215, 139", "186, 201, 191", "228, 235, 229"} {
		hash := sha256.Sum256([]byte("color: rgb(" + color + ");"))
		if !strings.Contains(policy, "'sha256-"+base64.StdEncoding.EncodeToString(hash[:])+"'") {
			t.Fatalf("missing editor color hash for %s", color)
		}
	}
	if strings.Count(policy, "'sha256-") != 9 {
		t.Fatalf("unexpected inline style allowance: %s", policy)
	}
}
