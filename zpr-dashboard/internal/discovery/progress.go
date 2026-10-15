package discovery

import (
	"net/netip"
	"sync"
	"time"
)

// Progress counts completed operations, including failed or cancelled checks.
// Planned counts remain exact decimal strings even for very large scopes.
type Progress struct {
	Elapsed           time.Duration
	TCPCompleted      int
	TCPPlanned        string
	HostCompleted     int
	HostPlanned       string
	InFlight          int
	OpenEndpoints     int
	DiscoveredHosts   int
	DiscoveredClients int
	UnexpectedErrors  int
	Done              bool
}

type scanProgress struct {
	mu         sync.Mutex
	value      Progress
	started    time.Time
	discovered map[netip.Addr]hostProgress
	ports      int
}

type hostProgress struct {
	pingReply     bool
	open          bool
	portsFinished int
}

func newScanProgress(options Options, started time.Time) *scanProgress {
	return &scanProgress{
		started:    started,
		discovered: make(map[netip.Addr]hostProgress),
		ports:      len(options.Ports),
		value: Progress{
			TCPPlanned:  options.PlannedChecks(len(options.Ports)),
			HostPlanned: options.HostChecksPlanned(),
		},
	}
}

func (p *scanProgress) begin() {
	p.mu.Lock()
	p.value.InFlight++
	p.mu.Unlock()
}

func (p *scanProgress) finish(address netip.Addr, hostCheck bool, state string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.value.InFlight--
	if hostCheck {
		p.value.HostCompleted++
	} else {
		p.value.TCPCompleted++
	}
	host := p.discovered[address]
	if hostCheck && state == "reply" {
		host.pingReply = true
	}
	if !hostCheck && state != "cancelled" && state != "error" {
		host.portsFinished++
	}
	if !hostCheck && state == "open" {
		p.value.OpenEndpoints++
		if !host.open {
			host.open = true
			p.value.DiscoveredHosts++
		}
	}
	if p.isClient(p.discovered[address]) {
		p.value.DiscoveredClients--
	}
	if p.isClient(host) {
		p.value.DiscoveredClients++
	}
	p.discovered[address] = host
	if state == "error" {
		p.value.UnexpectedErrors++
	}
}

func (p *scanProgress) isClient(host hostProgress) bool {
	return host.pingReply && !host.open && host.portsFinished == p.ports
}

// Callbacks are serialized and finish before Scan returns. They must return
// promptly so progress output does not delay scanning or cancellation.
func (p *scanProgress) start(notify func(Progress)) func() {
	if notify == nil {
		return func() {}
	}
	emit := func(done bool) {
		p.mu.Lock()
		value := p.value
		p.mu.Unlock()
		value.Elapsed = time.Since(p.started)
		value.Done = done
		notify(value)
	}
	emit(false)
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				emit(false)
			case <-stop:
				emit(true)
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-stopped
	}
}
