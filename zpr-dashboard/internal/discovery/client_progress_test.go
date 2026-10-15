package discovery

import (
	"net/netip"
	"testing"
	"time"
)

func TestProgressClientClassification(t *testing.T) {
	for _, test := range []struct {
		name       string
		states     []string
		hostChecks []bool
		clients    []int
		hosts      []int
	}{
		{"ping-first", []string{"reply", "closed", "timeout"}, []bool{true, false, false}, []int{0, 0, 1}, []int{0, 0, 0}},
		{"ping-last", []string{"closed", "timeout", "reply"}, []bool{false, false, true}, []int{0, 0, 1}, []int{0, 0, 0}},
		{"open-after-ping", []string{"reply", "closed", "open"}, []bool{true, false, false}, []int{0, 0, 0}, []int{0, 0, 1}},
		{"open-before-ping", []string{"open", "closed", "reply"}, []bool{false, false, true}, []int{0, 0, 0}, []int{1, 1, 1}},
		{"cancelled", []string{"reply", "closed", "cancelled"}, []bool{true, false, false}, []int{0, 0, 0}, []int{0, 0, 0}},
		{"error", []string{"reply", "closed", "error"}, []bool{true, false, false}, []int{0, 0, 0}, []int{0, 0, 0}},
		{"dns-only", []string{"resolved", "closed", "closed"}, []bool{true, false, false}, []int{0, 0, 0}, []int{0, 0, 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			progress := newScanProgress(Options{Ports: []uint16{22, 80}}, time.Now())
			for i, state := range test.states {
				progress.begin()
				progress.finish(netip.MustParseAddr("127.0.0.1"), test.hostChecks[i], state)
				if progress.value.DiscoveredClients != test.clients[i] ||
					progress.value.DiscoveredHosts != test.hosts[i] {
					t.Fatalf("step %d (%s): %+v", i, state, progress.value)
				}
			}
		})
	}
}
