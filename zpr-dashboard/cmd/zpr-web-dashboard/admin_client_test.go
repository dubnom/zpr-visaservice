package main

import (
	"path/filepath"
	"testing"
)

func TestAdminClientConfigurationErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	for _, test := range []struct {
		name, origin, key, keyFile, caFile, want string
	}{
		{"missing", "", "", "", "", "set ZPR_ADMIN_URL, ZPR_ADMIN_CA_FILE, and either ZPR_ADMIN_API_KEY or ZPR_ADMIN_KEY_FILE"},
		{"invalid URL", "http://admin.example", "key", "", missing, "ZPR_ADMIN_URL must be a valid https URL"},
		{"URL path", "https://admin.example/admin", "key", "", missing, "ZPR_ADMIN_URL must contain only the scheme and host"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ZPR_ADMIN_URL", test.origin)
			t.Setenv("ZPR_ADMIN_API_KEY", test.key)
			t.Setenv("ZPR_ADMIN_KEY_FILE", test.keyFile)
			t.Setenv("ZPR_ADMIN_CA_FILE", test.caFile)
			t.Setenv("ZPR_ADMIN_SERVER_NAME", "")
			client, message := newAdminClient()
			if client != nil || message != test.want {
				t.Fatalf("configuration response: client=%v message=%q, want %q", client, message, test.want)
			}
		})
	}
}
