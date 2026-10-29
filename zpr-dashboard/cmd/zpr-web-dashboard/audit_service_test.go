package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredAuditEventsRequiresDatabaseAndKeepsStoreWithoutReadTrust(t *testing.T) {
	t.Setenv("ZPR_AUDIT_DB_PATH", "")
	t.Setenv("ZPR_OPERATOR_DELEGATION_TRUST_FILE", "")
	if _, _, _, err := configuredAuditEvents(); err == nil {
		t.Fatal("Control-Service accepted a missing required audit database")
	}
	directory := filepath.Join(t.TempDir(), "audit")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_AUDIT_DB_PATH", filepath.Join(directory, "audit.db"))
	handler, repository, closeStore, err := configuredAuditEvents()
	if err != nil {
		t.Fatal(err)
	}
	if repository == nil {
		t.Fatal("configured audit database was discarded when read trust was missing")
	}
	if err := closeStore(); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/operator-enrollment/v1/audit/events", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing-trust status=%d body=%s", response.Code, response.Body)
	}
}
