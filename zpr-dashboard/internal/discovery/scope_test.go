package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestUnlimitedCompactScopes(t *testing.T) {
	for _, test := range []struct {
		input, count string
		ranges       int
	}{
		{"192.0.2.1/23", "510", 1},
		{"10.0.100.0/22", "1022", 1},
		{"10.0.0.0/8", "16777214", 1},
		{"192.0.2.0/24,198.51.100.0/24", "508", 2},
		{"2001:db8::/64", "18446744073709551616", 1},
		{"2001:db8::/32", "79228162514264337593543950336", 1},
		{"2001:db8::-2001:db8:ffff:ffff:ffff:ffff:ffff:ffff", "79228162514264337593543950336", 1},
		{"192.0.2.0/30,192.0.2.0-4,192.0.2.4-5", "6", 1},
		{"192.0.2.1-3,192.0.2.4-6,192.0.2.2-5", "6", 1},
		{"255.255.255.254,2001:db8::/64", "18446744073709551617", 2},
	} {
		t.Run(test.input, func(t *testing.T) {
			scope, err := ParseTargets(test.input)
			if err != nil || scope.Count().String() != test.count || len(scope.Ranges()) != test.ranges {
				t.Fatalf("scope: %+v count=%s error=%v", scope, scope.Count(), err)
			}
			first := scope.Ranges()[0].First
			visited := 0
			for address := range scope.Addresses() {
				visited++
				if address != first || !scope.Contains(address) {
					t.Fatalf("unexpected first address: %s", address)
				}
				break
			}
			if visited != 1 {
				t.Fatalf("iterator did not stop on caller break: %d", visited)
			}
			clone := scope.Ranges()
			clone[0].First = netip.Addr{}
			if !scope.Contains(first) {
				t.Fatal("Ranges leaked mutable scope storage")
			}
		})
	}
}

func TestRangeNormalizationAndContainment(t *testing.T) {
	scope, err := ParseTargetEntries([]string{
		"2001:db8::3-2001:db8::5", "192.0.2.4-6", "192.0.2.2-4",
		" 192.0.2.1 ", "192.0.2.10", "2001:db8::1-2001:db8::2",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for address := range scope.Addresses() {
		got = append(got, address.String())
	}
	want := []string{"192.0.2.1", "192.0.2.2", "192.0.2.3", "192.0.2.4",
		"192.0.2.5", "192.0.2.6", "192.0.2.10",
		"2001:db8::1", "2001:db8::2", "2001:db8::3", "2001:db8::4", "2001:db8::5"}
	if !slices.Equal(got, want) || len(scope.Ranges()) != 3 || scope.Count().String() != "12" {
		t.Fatalf("normalized scope: %v, %+v", got, scope.Ranges())
	}
	for _, text := range want {
		if !scope.Contains(netip.MustParseAddr(text)) {
			t.Fatalf("missing %s", text)
		}
	}
	for _, text := range []string{"192.0.2.0", "192.0.2.7", "192.0.2.11", "::1", "2001:db8::", "2001:db8::6"} {
		if scope.Contains(netip.MustParseAddr(text)) {
			t.Fatalf("scope expanded to %s", text)
		}
	}
	for _, entries := range [][]string{nil, {}, {""}, {"127.0.0.1,::1"}} {
		if _, err := ParseTargetEntries(entries); err == nil {
			t.Fatalf("accepted invalid list: %v", entries)
		}
	}
	exact := "127.0.0.1" + strings.Repeat(" ", 65536-len("127.0.0.1"))
	if scope, err := ParseTargets(exact); err != nil || scope.Count().String() != "1" {
		t.Fatalf("exact specification-size limit: count=%s err=%v", scope.Count(), err)
	}
	if _, err := ParseTargets(exact + " "); err == nil {
		t.Fatal("accepted oversized specification")
	}
}

func TestHugeScopeCancellationKeepsCompactReport(t *testing.T) {
	options := optionsFor(22)
	var err error
	options.Targets, err = ParseTargets("2001:db8::/32")
	if err != nil {
		t.Fatal(err)
	}
	options.Ports = []uint16{22, 80, 443}
	options.Ping, options.ReverseDNS = true, true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	report, err := scan(ctx, options, hostProbes{})
	if !errors.Is(err, context.Canceled) || report.Complete || len(report.Hosts) != 0 ||
		report.Planned != "237684487542793012780631851008" ||
		report.HostChecksPlanned != "158456325028528675187087900672" ||
		time.Since(start) > time.Second {
		t.Fatalf("huge partial report: %+v %v", report, err)
	}
	data, err := json.Marshal(report)
	if err != nil || len(data) > 2048 || !strings.Contains(string(data), `"hosts":[]`) {
		t.Fatalf("report expanded huge scope: size=%d err=%v", len(data), err)
	}
}

func TestLargeLoopbackScopeAllocatesOnlyReachedHosts(t *testing.T) {
	options := optionsFor(1)
	var err error
	options.Targets, err = ParseTargets("127.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	report, err := Scan(ctx, options)
	if !errors.Is(err, context.DeadlineExceeded) || report.Complete ||
		report.HostCount != "16777214" || report.Planned != "16777214" ||
		len(report.Hosts) == 0 || len(report.Hosts) > 12 || report.Attempted > 10 {
		t.Fatalf("large scope was not lazy/bounded: %+v %v", report, err)
	}
	for index, host := range report.Hosts {
		if host.Address != netip.MustParseAddr(fmt.Sprintf("127.0.0.%d", index+1)) {
			t.Fatalf("unexpected reached target %s", host.Address)
		}
	}
}

func TestAdapterImportsHaveNoHostCapAndSurviveUnreachedScope(t *testing.T) {
	options := optionsFor(22)
	var err error
	options.Targets, err = ParseTargets("10.0.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	inventory := &AdapterInventory{SchemaVersion: 1, Hosts: []AdapterHost{}}
	address := netip.MustParseAddr("10.0.0.1")
	for range 300 {
		inventory.Hosts = append(inventory.Hosts, AdapterHost{
			Address: address, AdapterID: "fixture", ObservedAt: time.Now().UTC(), Listeners: []Listener{},
		})
		address = address.Next()
	}
	data, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	options.Adapter, err = ReadAdapterInventory(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := Scan(ctx, options)
	if !errors.Is(err, context.Canceled) || len(report.Hosts) != 300 || report.Attempted != 0 {
		t.Fatalf("imported evidence lost or limited: %+v %v", report, err)
	}
	for _, host := range report.Hosts {
		if host.Adapter == nil || host.Adapter.Evidence != "imported_unverified" || host.TCP[0].State != "not_attempted" {
			t.Fatalf("unreached imported evidence conflated: %+v", host)
		}
	}
}
