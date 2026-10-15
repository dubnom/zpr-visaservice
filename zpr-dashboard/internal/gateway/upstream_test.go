package gateway

import (
	"strings"
	"testing"
)

func TestParseUpstreamCompatibility(t *testing.T) {
	for _, raw := range []string{"https://example.com", " https://example.com:8443/path ", "https://intranet", "https://example.com./path"} {
		got, err := ParseUpstream(raw)
		if err != nil || got.String() != strings.TrimSpace(raw) {
			t.Fatalf("accepted upstream %q: %v %v", raw, got, err)
		}
	}
	for _, raw := range []string{"http://example.com", "https://user@example.com", "https://example.com?q=1",
		"https://example.com#fragment", "https://127.0.0.1", "https://[::1]", "https://LOCALHOST.",
		"https://host.localhost", "https://host.local", "https://%invalid"} {
		if _, err := ParseUpstream(raw); err == nil {
			t.Fatalf("invalid upstream %q accepted", raw)
		}
	}
}
