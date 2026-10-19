package discovery

import (
	"regexp"
	"strings"
	"testing"
)

func TestIANAServiceRegistryCoverageAndSafeLabels(t *testing.T) {
	if len(ianaTCPServices) != 5890 {
		t.Fatalf("expected all 5890 named TCP ports from the bundled snapshot, got %d", len(ianaTCPServices))
	}
	labelPattern := regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	for port, names := range ianaTCPServices {
		if port == 0 || port >= 49152 || names == "" {
			t.Fatalf("invalid registered TCP assignment %d: %q", port, names)
		}
		name := serviceHints[port]
		if name == "" || !labelPattern.MatchString(serviceDNSLabel(name)) {
			t.Fatalf("unsafe generated service label for TCP/%d: %q", port, name)
		}
	}
	for port, want := range map[uint16]string{
		22: "ssh", 53: "dns", 80: "http", 443: "https", 465: "submissions",
		5432: "postgresql", 4840: "opcua-tcp", 9092: "xmlipcregsvc",
	} {
		if serviceHints[port] != want {
			t.Fatalf("TCP/%d name=%q want=%q", port, serviceHints[port], want)
		}
	}
	if ianaTCPServices[53] != "domain" || !strings.Contains(ianaTCPServices[80], "www") {
		t.Fatal("original IANA names and aliases were not preserved")
	}
	if serviceHints[65000] != "" {
		t.Fatal("private/dynamic ports must not get assigned registry names")
	}
	if got := serviceDNSLabel("whois++"); got != "whois" {
		t.Fatalf("legacy punctuation normalization: %s", got)
	}
}
