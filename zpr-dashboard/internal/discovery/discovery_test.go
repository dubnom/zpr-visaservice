package discovery

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestTargets(t *testing.T) {
	for _, test := range []struct {
		input string
		want  []string
	}{
		{"192.0.2.0/30,192.0.2.1", []string{"192.0.2.1", "192.0.2.2"}},
		{"192.0.2.0/31", []string{"192.0.2.0", "192.0.2.1"}},
		{"192.0.2.1/32", []string{"192.0.2.1"}},
		{"2001:db8::/127", []string{"2001:db8::", "2001:db8::1"}},
		{"127.0.0.1,::1,127.0.0.1", []string{"127.0.0.1", "::1"}},
		{"192.0.2.2-192.0.2.4,192.0.2.3-5", []string{"192.0.2.2", "192.0.2.3", "192.0.2.4", "192.0.2.5"}},
		{"2001:db8::1-2001:db8::3", []string{"2001:db8::1", "2001:db8::2", "2001:db8::3"}},
		{"192.0.2.255-192.0.3.1", []string{"192.0.2.255", "192.0.3.0", "192.0.3.1"}},
		{"192.0.2.0-0", []string{"192.0.2.0"}},
		{"192.0.2.0/30,192.0.2.0", []string{"192.0.2.0", "192.0.2.1", "192.0.2.2"}},
	} {
		t.Run(test.input, func(t *testing.T) {
			addresses, err := ParseTargets(test.input)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for address := range addresses.Addresses() {
				got = append(got, address.String())
			}
			if !slices.Equal(got, test.want) {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
	for _, input := range []string{
		"", "example.com", "127.0.0.1,", "0.0.0.0", "::", "::ffff:127.0.0.1",
		"224.0.0.1", "ff02::1", "255.255.255.255", "fe80::1%en0", "fe80::1",
		"::ffff:192.0.2.0/120", strings.Repeat(" ", 65537),
		"192.0.2.10-1", "192.0.2.1-256", "192.0.2.1-", "-192.0.2.1",
		"192.0.2.1-192.0.2.2-192.0.2.3", "192.0.2.1-::1",
		"2001:db8::1-3", "192.0.2.1-01", "192.0.2.1-host",
		"169.253.255.255-169.255.0.0", "223.255.255.255-240.0.0.1",
		"fe7f:ffff:ffff:ffff:ffff:ffff:ffff:ffff-fec0::",
		"::fffe:ffff:ffff-::1:0:0:0", "fe00::/7", "0.0.0.0/0",
	} {
		t.Run("reject-"+input[:min(len(input), 50)], func(t *testing.T) {
			if _, err := ParseTargets(input); err == nil {
				t.Fatalf("accepted %q", input)
			}
		})
	}
}

func TestPorts(t *testing.T) {
	ports, err := ParsePorts("443,22,80-82,80,65535")
	if err != nil || !slices.Equal(ports, []uint16{22, 80, 81, 82, 443, 65535}) {
		t.Fatalf("ports=%v error=%v", ports, err)
	}
	for _, input := range []string{"", "0", "-1", "65536", "22,", "80-79", "1-129", "1-128,443", "a", "1-2-3", strings.Repeat("1,", 3000)} {
		if _, err := ParsePorts(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
	ports, err = ParsePorts("1-128")
	if err != nil || len(ports) != MaxPorts {
		t.Fatalf("exact port cap: count=%d error=%v", len(ports), err)
	}
}

func TestImportedAdapterInventory(t *testing.T) {
	valid := `{"schema_version":1,"hosts":[{"address":"127.0.0.1","adapter_id":"adapter-one","observed_at":"2026-10-14T12:00:00Z","listeners":[{"transport":"tcp","bind_address":"127.0.0.1","port":5432,"process":"postgres"},{"transport":"udp","bind_address":"0.0.0.0","port":53,"service":"dns"}]}]}`
	inventory, err := ReadAdapterInventory(strings.NewReader(valid))
	if err != nil || inventory.Hosts[0].Listeners[1].Transport != "udp" {
		t.Fatalf("inventory=%+v error=%v", inventory, err)
	}
	for _, input := range []string{
		`{}`, `{"schema_version":1}`, valid + `{}`, strings.Replace(valid, `"schema_version":1`, `"schema_version":2`, 1),
		strings.Replace(valid, `"adapter_id":"adapter-one"`, `"adapter_id":""`, 1),
		strings.Replace(valid, `"observed_at":"2026-10-14T12:00:00Z"`, `"observed_at":null`, 1),
		strings.Replace(valid, `"transport":"tcp"`, `"transport":"other"`, 1),
		strings.Replace(valid, `"port":5432`, `"port":0`, 1),
		strings.Replace(valid, `"schema_version":1`, `"unexpected":true,"schema_version":1`, 1),
		strings.Replace(valid, `"bind_address":"127.0.0.1"`, `"bind_address":"224.0.0.1"`, 1),
		strings.Repeat(" ", (2<<20)+1),
	} {
		if _, err := ReadAdapterInventory(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted invalid adapter inventory %.100s", input)
		}
	}
	inventory.Hosts = append(inventory.Hosts, inventory.Hosts[0])
	if err := inventory.Validate(); err == nil {
		t.Fatal("accepted duplicate imported host")
	}
}

func optionsFor(port uint16) Options {
	targets, err := ParseTargets("127.0.0.1")
	if err != nil {
		panic(err)
	}
	return Options{
		ScannerID: "loopback-fixture", Organization: "test", Targets: targets,
		Ports: []uint16{port}, Workers: 2, Rate: 100, Timeout: 100 * time.Millisecond,
	}
}

func TestOptionValidation(t *testing.T) {
	for name, change := range map[string]func(*Options){
		"scanner":        func(o *Options) { o.ScannerID = "" },
		"organization":   func(o *Options) { o.Organization = " " },
		"workers":        func(o *Options) { o.Workers = 33 },
		"rate":           func(o *Options) { o.Rate = 0 },
		"rate-upper":     func(o *Options) { o.Rate = 101 },
		"timeout":        func(o *Options) { o.Timeout = time.Minute },
		"timeout-lower":  func(o *Options) { o.Timeout = time.Millisecond },
		"targets":        func(o *Options) { o.Targets = TargetScope{} },
		"ports":          func(o *Options) { o.Ports = nil },
		"zero-port":      func(o *Options) { o.Ports[0] = 0 },
		"duplicate-port": func(o *Options) { o.Ports = append(o.Ports, o.Ports[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			options := optionsFor(80)
			change(&options)
			if report, err := Scan(context.Background(), options); err == nil || report != nil {
				t.Fatalf("invalid options reached scan: report=%+v error=%v", report, err)
			}
		})
	}
}

func TestLoopbackScanAndImportedEvidence(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	options := optionsFor(uint16(listener.Addr().(*net.TCPAddr).Port))
	observed := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	options.Adapter = &AdapterInventory{SchemaVersion: 1, Hosts: []AdapterHost{{
		Address: netip.MustParseAddr("127.0.0.1"), AdapterID: "adapter-one", ObservedAt: observed,
		Listeners: []Listener{{Transport: "tcp", BindAddress: netip.MustParseAddr("127.0.0.1"), Port: 5432, Process: "postgres"}},
	}}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	report, err := Scan(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := report.Hosts[0].TCP[0]
	if !report.Complete || report.Planned != "1" || report.Attempted != 1 || len(report.Hosts) != 1 || endpoint.State != "open" ||
		endpoint.DetectedProtocol != "" || endpoint.ObservedAt == nil || endpoint.LocalAddress == "" || report.Deadline == nil {
		t.Fatalf("invalid report: %+v endpoint=%+v", report, endpoint)
	}
	adapter := report.Hosts[0].Adapter
	if adapter.Evidence != "imported_unverified" || adapter.ObservedAt != observed || len(report.Hosts[0].TCP) != 1 {
		t.Fatalf("host-reported listeners were conflated with remote observations: %+v", report.Hosts[0])
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.SchemaVersion != 2 || !options.Targets.Contains(decoded.Hosts[0].Address) {
		t.Fatalf("JSON contract roundtrip: %s, %v", data, err)
	}
	connection, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	// Default scanning opens and closes TCP without transmitting application data.
	connection.SetReadDeadline(time.Now().Add(time.Second))
	var received [1]byte
	n, readErr := connection.Read(received[:])
	connection.Close()
	if n != 0 || readErr != io.EOF {
		t.Fatalf("default scanner transmitted data: n=%d error=%v", n, readErr)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	report, err = Scan(context.Background(), options)
	if err != nil || report.Hosts[0].TCP[0].State != "closed" || report.Hosts[0].TCP[0].Error == "" {
		t.Fatalf("connection refusal not explicit: report=%+v err=%v", report, err)
	}
	options.Adapter.Hosts[0].Address = netip.MustParseAddr("192.0.2.1")
	if _, err := Scan(context.Background(), options); err == nil {
		t.Fatal("accepted imported host outside target scope")
	}
}

func TestCancellationAndPacing(t *testing.T) {
	options := optionsFor(22)
	options.Ports = []uint16{22, 80, 443}
	options.Rate = 10
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	report, err := Scan(ctx, options)
	if !errors.Is(err, context.DeadlineExceeded) || report.Complete || report.Error == "" ||
		report.Attempted != 0 || report.Planned != "3" {
		t.Fatalf("cancelled inventory not explicit: %+v, %v", report, err)
	}
	for _, endpoint := range report.Hosts[0].TCP {
		if endpoint.State != "not_attempted" {
			t.Fatalf("pacing allowed an early connection: %+v", endpoint)
		}
	}
}

func TestPacingAcrossWorkersAndPartialResults(t *testing.T) {
	options := optionsFor(1)
	options.Ports = nil
	options.Rate = 10
	options.Workers = 8
	accepted := make(chan time.Time, 4)
	for range 4 {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		options.Ports = append(options.Ports, uint16(listener.Addr().(*net.TCPAddr).Port))
		go func() {
			connection, err := listener.Accept()
			if err == nil {
				accepted <- time.Now()
				connection.Close()
			}
		}()
	}
	started := time.Now()
	report, err := Scan(context.Background(), options)
	if err != nil || !report.Complete || report.Attempted != 4 || len(report.Hosts) != 1 {
		t.Fatalf("paced loopback scan: %+v %v", report, err)
	}
	times := make([]time.Time, 0, 4)
	for range 4 {
		select {
		case stamp := <-accepted:
			times = append(times, stamp)
		case <-time.After(time.Second):
			t.Fatal("missing loopback accept")
		}
	}
	slices.SortFunc(times, func(a, b time.Time) int { return a.Compare(b) })
	if time.Since(started) < 350*time.Millisecond || times[3].Sub(times[0]) < 250*time.Millisecond {
		t.Fatal("multiple workers bypassed the shared 10 starts/second limit")
	}
	// The listeners remain open but do not accept further connections. The TCP
	// backlog allows local handshakes while cancellation leaves the rest explicit.
	options.Ports = append(options.Ports, 1, 2, 3, 4, 5)
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()
	report, err = Scan(ctx, options)
	if !errors.Is(err, context.DeadlineExceeded) || report.Complete ||
		report.Attempted == 0 || report.Attempted >= 9 {
		t.Fatalf("expected mixed attempted/not-attempted partial report: %+v %v", report, err)
	}
}

func TestConnectionStates(t *testing.T) {
	for _, test := range []struct {
		err   error
		state string
	}{
		{context.Canceled, "cancelled"}, {context.DeadlineExceeded, "timeout"},
		{fmt.Errorf("wrapped: %w", syscall.ECONNREFUSED), "closed"},
		{syscall.ENETUNREACH, "unreachable"}, {syscall.EHOSTUNREACH, "unreachable"},
		{syscall.EACCES, "error"},
	} {
		if got := connectionState(test.err); got != test.state {
			t.Fatalf("%v: got %s, want %s", test.err, got, test.state)
		}
	}
}

func TestProtocolChecks(t *testing.T) {
	for _, test := range []struct {
		name     string
		port     uint16
		protocol string
		respond  func(net.Conn) error
	}{
		{"ssh", 22, "ssh", func(connection net.Conn) error {
			_, err := io.WriteString(connection, "notice\r\nSSH-2.0-fixture\r\n")
			return err
		}},
		{"http", 80, "http", func(connection net.Conn) error {
			request, err := http.ReadRequest(bufio.NewReader(connection))
			if err != nil {
				return err
			}
			if request.Method != http.MethodHead || request.URL.Path != "/" || request.Header.Get("Authorization") != "" {
				return fmt.Errorf("unexpected probe request: %+v", request)
			}
			_, err = io.WriteString(connection, "HTTP/1.1 302 Found\r\nLocation: http://192.0.2.1/\r\nContent-Length: 0\r\n\r\n")
			return err
		}},
		{"invalid-ssh", 22, "", func(connection net.Conn) error {
			_, err := io.WriteString(connection, "not SSH\r\n")
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			done := make(chan error, 1)
			go func() {
				defer server.Close()
				done <- test.respond(server)
			}()
			result := Endpoint{Port: test.port, State: "open"}
			probe(context.Background(), client, test.port, time.Second, &result)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if result.State != "open" || result.DetectedProtocol != test.protocol {
				t.Fatalf("protocol evidence incorrect: %+v", result)
			}
			if test.protocol == "" && result.ProbeError == "" {
				t.Fatalf("failed protocol check was hidden: %+v", result)
			}
			if test.protocol != "" && (result.ProtocolEvidence == "" || result.ProbeError != "") {
				t.Fatalf("missing protocol evidence: %+v", result)
			}
		})
	}
}

func TestProbeBoundsCancellationAndTLSVerification(t *testing.T) {
	for _, cancelEarly := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelEarly), func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelEarly {
				cancel()
			}
			started := time.Now()
			result := Endpoint{State: "open"}
			probe(ctx, client, 22, 30*time.Millisecond, &result)
			if time.Since(started) > time.Second || result.ProbeError == "" || result.State != "open" {
				t.Fatalf("probe did not terminate explicitly within bound: %+v", result)
			}
		})
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("TLS probe sent an application request")
	}))
	defer server.Close()
	connection, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	result := Endpoint{State: "open"}
	probe(context.Background(), connection, 443, time.Second, &result)
	if result.State != "open" || result.DetectedProtocol != "" || result.ProbeError == "" {
		t.Fatalf("untrusted certificate silently accepted or open port lost: %+v", result)
	}
}

func TestPortHintsAreNotProtocolEvidence(t *testing.T) {
	if serviceHints[389] != "ldap" || serviceHints[5432] != "postgresql" {
		t.Fatal("missing well-known service hints")
	}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	result := Endpoint{Port: 389, State: "open", ServiceHint: serviceHints[389], HintBasis: "tcp_port_number_only"}
	probe(context.Background(), client, 389, time.Second, &result)
	if result.DetectedProtocol != "" || result.ProtocolEvidence != "" || result.ProbeError != "" {
		t.Fatalf("unsupported application protocol inferred from port: %+v", result)
	}
}

func TestProductionIndependentDependencies(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect dependencies: %v\n%s", err, output)
	}
	for _, dependency := range strings.Fields(string(output)) {
		if dependency != "neboagency.com/zpr-dashborad/internal/discovery" {
			t.Fatalf("discovery depends on nonstandard or application runtime package %s", dependency)
		}
	}
}
