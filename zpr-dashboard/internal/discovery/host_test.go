package discovery

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPingArguments(t *testing.T) {
	for _, test := range []struct {
		platform, address string
		want              []string
	}{
		{"darwin", "127.0.0.1", []string{"-n", "-c", "1", "127.0.0.1"}},
		{"darwin", "::1", []string{"-n", "-c", "1", "::1"}},
		{"linux", "::1", []string{"-n", "-c", "1", "-6", "::1"}},
	} {
		if got := pingArgs(test.platform, netip.MustParseAddr(test.address)); !slices.Equal(got, test.want) {
			t.Fatalf("arguments: %v, want %v", got, test.want)
		}
	}
}

func TestPingResultsAndDeadline(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("Unix ping backend")
	}
	noReply := "exit 1"
	if runtime.GOOS == "darwin" {
		noReply = "exit 2"
	}
	for _, test := range []struct {
		name, script, state string
	}{
		{"reply", "exit 0", "reply"},
		{"no-reply", noReply, "no_reply"},
		{"permission", "echo 'permission denied' >&2; exit 3", "error"},
		{"deadline", "exec sleep 2", "timeout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			start := time.Now()
			result := runPing(context.Background(), 100*time.Millisecond, "/bin/sh", []string{"-c", test.script})
			if result.State != test.state || result.ObservedAt == nil ||
				(result.State != "reply" && result.Error == "") || time.Since(start) > time.Second {
				t.Fatalf("unexpected ping: %+v", result)
			}
			if test.name == "permission" && !strings.Contains(result.Error, "permission denied") {
				t.Fatalf("lost ping stderr: %+v", result)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := pingHost(ctx, netip.MustParseAddr("127.0.0.1"), time.Second, "/missing/ping"); result.State != "cancelled" {
		t.Fatalf("cancelled ping: %+v", result)
	}
	var output boundedPingOutput
	n, err := output.Write([]byte(strings.Repeat("x", 8192)))
	if n != 8192 || err != nil || len(output) != 4096 {
		t.Fatalf("ping output not bounded: %d %v %d", n, err, len(output))
	}
}

func TestReverseLookupEvidence(t *testing.T) {
	for _, test := range []struct {
		name   string
		lookup func(context.Context, string) ([]string, error)
		state  string
	}{
		{"names", func(context.Context, string) ([]string, error) {
			return []string{"z.example.", "a.example.", "a.example."}, nil
		}, "resolved"},
		{"empty", func(context.Context, string) ([]string, error) { return nil, nil }, "no_records"},
		{"nxdomain", func(context.Context, string) ([]string, error) {
			return nil, &net.DNSError{IsNotFound: true, Err: "not found"}
		}, "no_records"},
		{"resolver-error", func(context.Context, string) ([]string, error) {
			return nil, errors.New("resolver failed")
		}, "error"},
		{"dns-timeout", func(context.Context, string) ([]string, error) {
			return nil, &net.DNSError{IsTimeout: true, Err: "DNS timeout"}
		}, "timeout"},
		{"deadline", func(ctx context.Context, _ string) ([]string, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}, "timeout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := lookupNames(context.Background(), netip.MustParseAddr("127.0.0.1"), 10*time.Millisecond, test.lookup)
			if result.State != test.state || result.ObservedAt == nil {
				t.Fatalf("lookup: %+v", result)
			}
			if test.name == "names" && !slices.Equal(result.Names, []string{"a.example.", "z.example."}) {
				t.Fatalf("lookup names: %v", result.Names)
			}
		})
	}
}

func TestHostChecksNeverGateTCP(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	options := optionsFor(uint16(listener.Addr().(*net.TCPAddr).Port))
	options.Ping, options.ReverseDNS = true, true
	for _, state := range []string{"no_reply", "error", "timeout"} {
		t.Run(state, func(t *testing.T) {
			probes := hostProbes{
				ping: func(context.Context, netip.Addr, time.Duration) HostCheck {
					return HostCheck{State: state, Error: "fixture ping result"}
				},
				reverseDNS: func(context.Context, netip.Addr, time.Duration) HostCheck {
					return HostCheck{State: "no_records"}
				},
			}
			report, err := scan(context.Background(), options, probes)
			if err != nil || !report.Complete || report.HostChecksPlanned != "2" ||
				report.HostChecksAttempted != 2 || report.Attempted != 1 || report.Hosts[0].TCP[0].State != "open" {
				t.Fatalf("host checks gated TCP: %+v, %v", report, err)
			}
			if (report.HostCheckErrors == 1) != (state == "error") {
				t.Fatalf("unexpected error count: %+v", report)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			partial, err := scan(ctx, options, probes)
			if !errors.Is(err, context.Canceled) || partial.Complete || partial.HostChecksAttempted != 0 ||
				len(partial.Hosts) != 0 || partial.HostCount != "1" || partial.HostChecksPlanned != "2" {
				t.Fatalf("partial host checks: %+v, %v", partial, err)
			}
		})
	}
}

func TestMissingPingFailsBeforeScan(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	options := optionsFor(80)
	options.Ping = true
	report, err := Scan(context.Background(), options)
	if err == nil || report != nil || !strings.Contains(err.Error(), "-ping=false") {
		t.Fatalf("missing ping silently ignored: %+v %v", report, err)
	}
}
