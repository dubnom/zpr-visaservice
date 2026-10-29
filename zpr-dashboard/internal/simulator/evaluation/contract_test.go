package evaluation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadPolicyDigestBindsLiveBundle(t *testing.T) {
	body := `{"config_id":0,"version":"","format":"base64;zip;0.16.0","container":"first"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/policies/curr" || r.Header.Get("X-API-Key") != "test-key" {
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	first, err := ReadPolicyDigest(context.Background(), server.Client(), server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	body = `{"container":"first", "format":"base64;zip;0.16.0", "version":"", "config_id":0}`
	reordered, err := ReadPolicyDigest(context.Background(), server.Client(), server.URL, "test-key")
	if err != nil || reordered != first {
		t.Fatal("wire formatting changed canonical policy identity", err)
	}
	body = `{"config_id":0,"version":"","format":"base64;zip;0.16.0","container":"second"}`
	changed, err := ReadPolicyDigest(context.Background(), server.Client(), server.URL, "test-key")
	if err != nil || changed == first {
		t.Fatal("policy change did not alter identity", err)
	}
	for _, invalid := range []string{`{}`, `{"container":"missing-format"}`, `invalid`} {
		body = invalid
		if _, err := ReadPolicyDigest(context.Background(), server.Client(), server.URL, "test-key"); err == nil {
			t.Fatal("invalid live bundle accepted", invalid)
		}
	}
	if _, err := ReadPolicyDigest(context.Background(), server.Client(), server.URL, "wrong"); err == nil {
		t.Fatal("unauthorized policy read accepted")
	}
}
