package discovery

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestScanProgressFinalCounts(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	targets, err := ParseTargets("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	var updates []Progress
	options := Options{
		ScannerID: "test", Organization: "test", Targets: targets,
		Ports:   []uint16{uint16(listener.Addr().(*net.TCPAddr).Port)},
		Workers: 3, Rate: 100, Timeout: time.Second, Ping: true, ReverseDNS: true,
		OnProgress: func(value Progress) { updates = append(updates, value) },
	}
	report, err := scan(context.Background(), options, hostProbes{
		ping: func(context.Context, netip.Addr, time.Duration) HostCheck {
			return HostCheck{State: "error", Error: "fixture"}
		},
		reverseDNS: func(context.Context, netip.Addr, time.Duration) HostCheck {
			return HostCheck{State: "resolved", Names: []string{"localhost"}}
		},
	})
	if err != nil || !report.Complete || len(updates) < 2 {
		t.Fatalf("report=%+v error=%v updates=%+v", report, err, updates)
	}
	first, last := updates[0], updates[len(updates)-1]
	if first.Done || first.TCPCompleted != 0 || first.HostCompleted != 0 ||
		first.TCPPlanned != "1" || first.HostPlanned != "2" {
		t.Fatalf("initial progress: %+v", first)
	}
	if !last.Done || last.InFlight != 0 || last.OpenEndpoints != 1 || last.DiscoveredHosts != 1 ||
		last.TCPCompleted != report.Attempted || last.HostCompleted != report.HostChecksAttempted ||
		last.UnexpectedErrors != report.EndpointErrors+report.HostCheckErrors {
		t.Fatalf("final progress=%+v report=%+v", last, report)
	}
}

func TestProgressDiscoveredHostsAndServices(t *testing.T) {
	progress := newScanProgress(Options{Ports: []uint16{22, 80}}, time.Now())
	for _, check := range []struct {
		address         string
		hostCheck       bool
		state           string
		hosts, services int
	}{
		{"127.0.0.1", true, "resolved", 0, 0},
		{"127.0.0.1", true, "reply", 0, 0},
		{"127.0.0.1", false, "open", 1, 1},
		{"127.0.0.1", false, "open", 1, 2},
		{"127.0.0.2", false, "open", 2, 3},
		{"127.0.0.2", true, "reply", 2, 3},
		{"127.0.0.3", false, "closed", 2, 3},
		{"127.0.0.3", true, "no_reply", 2, 3},
		{"127.0.0.3", false, "timeout", 2, 3},
		{"127.0.0.3", true, "resolved", 2, 3},
		{"127.0.0.3", true, "error", 2, 3},
		{"127.0.0.4", true, "reply", 2, 3},
	} {
		progress.begin()
		progress.finish(netip.MustParseAddr(check.address), check.hostCheck, check.state)
		if progress.value.DiscoveredHosts != check.hosts || progress.value.OpenEndpoints != check.services {
			t.Fatalf("after %+v: progress=%+v", check, progress.value)
		}
	}
}

func TestScanProgressDuringBlockedCheckAndCancellation(t *testing.T) {
	targets, err := ParseTargets("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan Progress, 16)
	type result struct {
		report *Report
		err    error
	}
	finished := make(chan result, 1)
	go func() {
		report, err := scan(ctx, Options{
			ScannerID: "test", Organization: "test", Targets: targets, Ports: []uint16{22},
			Workers: 1, Rate: 100, Timeout: 5 * time.Second, Ping: true,
			OnProgress: func(value Progress) { updates <- value },
		}, hostProbes{
			ping: func(ctx context.Context, _ netip.Addr, _ time.Duration) HostCheck {
				<-ctx.Done()
				return HostCheck{State: "cancelled"}
			},
		})
		finished <- result{report, err}
	}()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case value := <-updates:
			if value.Done {
				t.Fatal("scan ended before cancellation")
			}
			if value.Elapsed >= time.Second && value.InFlight == 1 {
				cancel()
				select {
				case got := <-finished:
					if !errors.Is(got.err, context.Canceled) || got.report.Complete ||
						got.report.HostChecksAttempted != 1 {
						t.Fatalf("cancelled scan: %+v %v", got.report, got.err)
					}
					var last Progress
					for len(updates) > 0 {
						last = <-updates
					}
					if !last.Done || last.InFlight != 0 || last.HostCompleted != 1 ||
						last.TCPCompleted != 0 || last.UnexpectedErrors != 0 {
						t.Fatalf("cancelled progress: %+v", last)
					}
					return
				case <-deadline:
					t.Fatal("scan did not stop after cancellation")
				}
			}
		case <-deadline:
			t.Fatal("no periodic progress while a check was blocked")
		}
	}
}

func TestProgressLargeScopeCounts(t *testing.T) {
	targets, err := ParseTargets("2001:db8::/32")
	if err != nil {
		t.Fatal(err)
	}
	progress := newScanProgress(Options{
		Targets: targets, Ports: []uint16{22, 80}, Ping: true, ReverseDNS: true,
	}, time.Now())
	want := "158456325028528675187087900672"
	if progress.value.TCPPlanned != want || progress.value.HostPlanned != want {
		t.Fatalf("large scope counts lost precision: %+v", progress.value)
	}
}
