package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/miekg/dns"
)

func TestParseBINDTSIGKey(t *testing.T) {
	keyName, secret, err := parseBINDTSIGKey([]byte(`key "zpr-dns-viewer" {
	algorithm hmac-sha256;
	secret "c2VjcmV0";
};`))
	if err != nil {
		t.Fatal(err)
	}
	if keyName != "zpr-dns-viewer." || secret != "c2VjcmV0" {
		t.Fatalf("key = (%q, %q), want (%q, %q)", keyName, secret, "zpr-dns-viewer.", "c2VjcmV0")
	}
}

func TestParseBINDTSIGKeyRejectsUnsupportedConfig(t *testing.T) {
	for _, input := range []string{
		`key "first" { algorithm hmac-sha256; secret "one"; }; key "second" { algorithm hmac-sha256; secret "two"; };`,
		`key "zpr-dns-viewer" { algorithm hmac-md5; secret "c2VjcmV0"; };`,
	} {
		if _, _, err := parseBINDTSIGKey([]byte(input)); err == nil {
			t.Fatalf("parseBINDTSIGKey(%q) succeeded; want error", input)
		}
	}
}

func TestDNSRecordFromRR(t *testing.T) {
	rr, err := dns.NewRR("adapter2.svc.zpr. 30 IN AAAA fd00:1::2")
	if err != nil {
		t.Fatal(err)
	}
	record, ok := dnsRecordFromRR(rr)
	if !ok {
		t.Fatal("dnsRecordFromRR returned false")
	}
	if record.Name != "adapter2.svc.zpr." || record.TTL != 30 || record.Type != "AAAA" || record.Value != "fd00:1::2" {
		t.Fatalf("record = %#v", record)
	}
}

func TestDNSRecordsHandlerRequiresConfiguration(t *testing.T) {
	t.Setenv("ZPR_DNS_TRANSFER_ADDR", "")
	t.Setenv("ZPR_DNS_TRANSFER_TSIG_KEY_FILE", "")
	response := httptest.NewRecorder()
	newDNSRecordsHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/dns/records", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
