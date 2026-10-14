package dnsrecords

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

const testKeyName = "zpr-dns-viewer."
const testKeySecret = "c2VjcmV0"
const testKey = `key "zpr-dns-viewer" { algorithm hmac-sha256; secret "c2VjcmV0"; };`

func writeTestKey(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "viewer.key")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func startTransferServer(t *testing.T, handler dns.HandlerFunc) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	server := &dns.Server{
		Listener:          listener,
		Handler:           handler,
		TsigSecret:        map[string]string{testKeyName: testKeySecret},
		NotifyStartedFunc: func() { close(started) },
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.ActivateAndServe() }()
	select {
	case <-started:
	case err := <-stopped:
		_ = listener.Close()
		t.Fatalf("start DNS server: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Shutdown(); err != nil {
			t.Errorf("stop DNS server: %v", err)
		}
		if err := <-stopped; err != nil {
			t.Errorf("DNS server: %v", err)
		}
	})
	return listener.Addr().String()
}

func transferRecords(t *testing.T, text ...string) []dns.RR {
	t.Helper()
	records := make([]dns.RR, 0, len(text))
	for _, value := range text {
		rr, err := dns.NewRR(value)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, rr)
	}
	return records
}

func TestDNSRecordsSignedTransferWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/simulation.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/organizations")
	t.Setenv("ZPR_SIMULATOR_URL", "://invalid")
	t.Setenv("ZPR_DNS_TRANSFER_ADDR", "invalid environment address")
	t.Setenv("ZPR_DNS_TRANSFER_TSIG_KEY_FILE", "/unavailable/environment.key")
	records := transferRecords(t,
		"svc.zpr. 30 IN SOA dns.svc.zpr. hostmaster.svc.zpr. 1 60 60 86400 30",
		`z.svc.zpr. 30 IN TXT "second value"`,
		"adapter.svc.zpr. 30 IN AAAA fd00:1::2",
		"adapter.svc.zpr. 30 IN A 192.0.2.2",
		"adapter.svc.zpr. 30 IN A 192.0.2.1",
		"adapter.svc.zpr. 30 IN A 192.0.2.1",
		`z.svc.zpr. 30 IN TXT "first value"`,
	)
	queries := make(chan *dns.Msg, 1)
	address := startTransferServer(t, func(w dns.ResponseWriter, query *dns.Msg) {
		queries <- query.Copy()
		if err := w.TsigStatus(); err != nil {
			t.Errorf("AXFR TSIG verification: %v", err)
			return
		}
		envelopes := make(chan *dns.Envelope, 2)
		envelopes <- &dns.Envelope{RR: records}
		envelopes <- &dns.Envelope{RR: []dns.RR{records[0]}}
		close(envelopes)
		transfer := &dns.Transfer{}
		if err := transfer.Out(w, query, envelopes); err != nil {
			t.Errorf("send signed transfer: %v", err)
		}
	})
	handler := NewHandler(" \t"+address+"\n", " "+writeTestKey(t, testKey)+" ")
	request := httptest.NewRequest(http.MethodGet, "/api/dns/records", nil)
	request.Header.Set("Authorization", "Bearer browser-only")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	want := `{"zone":"svc.zpr.","records":[{"name":"adapter.svc.zpr.","ttl":30,"type":"A","value":"192.0.2.1"},{"name":"adapter.svc.zpr.","ttl":30,"type":"A","value":"192.0.2.2"},{"name":"adapter.svc.zpr.","ttl":30,"type":"AAAA","value":"fd00:1::2"},{"name":"svc.zpr.","ttl":30,"type":"SOA","value":"dns.svc.zpr. hostmaster.svc.zpr. 1 60 60 86400 30"},{"name":"z.svc.zpr.","ttl":30,"type":"TXT","value":"\"first value\""},{"name":"z.svc.zpr.","ttl":30,"type":"TXT","value":"\"second value\""}]}` + "\n"
	if response.Code != http.StatusOK || response.Body.String() != want {
		t.Fatalf("status=%d body=%s, want 200 %s", response.Code, response.Body.String(), want)
	}
	if response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers=%v", response.Header())
	}
	query := <-queries
	signature := query.IsTsig()
	if len(query.Question) != 1 || query.Question[0].Name != "svc.zpr." || query.Question[0].Qtype != dns.TypeAXFR ||
		signature == nil || signature.Hdr.Name != testKeyName || signature.Algorithm != dns.HmacSHA256 || signature.Fudge != 300 {
		t.Fatalf("AXFR question or TSIG contract changed")
	}
	if request.Header.Get("Authorization") != "Bearer browser-only" || request.URL.Path != "/api/dns/records" {
		t.Fatal("incoming request was mutated")
	}
}

func TestDNSRecordsConfigurationErrorsAndKeyReload(t *testing.T) {
	path := writeTestKey(t, testKey)
	handler := NewHandler("invalid-address", path)
	for _, contents := range []string{testKey, "invalid key", testKey} {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/dns/records", nil))
		code, body := http.StatusBadGateway, "DNS zone transfer failed\n"
		if contents != testKey {
			code, body = http.StatusServiceUnavailable, "DNS record viewer key is invalid\n"
		}
		assertDNSError(t, response, code, body)
	}
	for _, test := range []struct {
		address string
		keyPath string
		body    string
	}{
		{"", path, "DNS record viewer is not configured\n"},
		{" \t", path, "DNS record viewer is not configured\n"},
		{"invalid-address", "", "DNS record viewer is not configured\n"},
		{"invalid-address", path + ".missing", "DNS record viewer is not configured\n"},
		{"invalid-address", writeTestKey(t, `key "viewer" { algorithm hmac-md5; secret "c2VjcmV0"; };`), "DNS record viewer key is invalid\n"},
	} {
		response := httptest.NewRecorder()
		NewHandler(test.address, test.keyPath).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/dns/records", nil))
		assertDNSError(t, response, http.StatusServiceUnavailable, test.body)
	}
}

func assertDNSError(t *testing.T, response *httptest.ResponseRecorder, code int, body string) {
	t.Helper()
	if response.Code != code || response.Body.String() != body {
		t.Fatalf("status=%d body=%q, want %d %q", response.Code, response.Body.String(), code, body)
	}
	if response.Header().Get("Content-Type") != "text/plain; charset=utf-8" || response.Header().Get("Cache-Control") != "" {
		t.Fatalf("error response headers=%v", response.Header())
	}
}

func TestDNSRecordsTransferFailures(t *testing.T) {
	soa := transferRecords(t, "svc.zpr. 30 IN SOA dns.svc.zpr. hostmaster.svc.zpr. 1 60 60 86400 30")[0]
	for _, mode := range []string{"refused", "incomplete", "bad-signature"} {
		t.Run(mode, func(t *testing.T) {
			address := startTransferServer(t, func(w dns.ResponseWriter, query *dns.Msg) {
				if err := w.TsigStatus(); err != nil {
					t.Errorf("verify AXFR request: %v", err)
					return
				}
				reply := new(dns.Msg)
				reply.SetReply(query)
				reply.Authoritative = true
				switch mode {
				case "refused":
					reply.Rcode = dns.RcodeRefused
				case "incomplete":
					reply.Answer = []dns.RR{soa}
				case "bad-signature":
					reply.Answer = []dns.RR{soa, soa}
					reply.SetTsig(testKeyName, dns.HmacSHA256, 300, 1)
				}
				if err := w.WriteMsg(reply); err != nil {
					t.Errorf("send failure response: %v", err)
				}
				if err := w.Close(); err != nil {
					t.Errorf("close failure response: %v", err)
				}
			})
			response := httptest.NewRecorder()
			NewHandler(address, writeTestKey(t, testKey)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/dns/records", nil))
			assertDNSError(t, response, http.StatusBadGateway, "DNS zone transfer failed\n")
		})
	}
}

func TestDNSRecordsDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect DNS record dependencies: %v\n%s", err, output)
	}
	allowed := map[string]bool{
		"neboagency.com/zpr-dashborad/internal/dnsrecords": true,
		"github.com/miekg/dns":                             true,
		"golang.org/x/sys/unix":                            true,
		"golang.org/x/sys/windows":                         true,
	}
	for _, dependency := range strings.Fields(string(output)) {
		if !allowed[dependency] {
			t.Errorf("DNS record viewer must depend only on the standard library and existing DNS dependencies, got %s", dependency)
		}
	}
}
