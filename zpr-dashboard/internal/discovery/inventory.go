package discovery

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"time"
)

const SchemaVersion = 1
const ReportSchemaVersion = 2

// AdapterInventory is imported evidence, not an authenticated live adapter API.
type AdapterInventory struct {
	SchemaVersion int           `json:"schema_version"`
	Hosts         []AdapterHost `json:"hosts"`
}

type AdapterHost struct {
	Address    netip.Addr `json:"address"`
	AdapterID  string     `json:"adapter_id"`
	ObservedAt time.Time  `json:"observed_at"`
	Listeners  []Listener `json:"listeners"`
}

type Listener struct {
	Transport   string     `json:"transport"`
	BindAddress netip.Addr `json:"bind_address"`
	Port        uint16     `json:"port"`
	Process     string     `json:"process,omitempty"`
	Service     string     `json:"service,omitempty"`
}

func ReadAdapterInventory(reader io.Reader) (*AdapterInventory, error) {
	const maxBytes = 2 << 20
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read adapter inventory: %w", err)
	}
	if len(data) > maxBytes {
		return nil, errors.New("adapter inventory exceeds 2 MiB")
	}
	var inventory AdapterInventory
	// Reject unknown fields rather than silently losing potentially important evidence.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inventory); err != nil {
		return nil, fmt.Errorf("invalid adapter inventory: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("adapter inventory contains trailing data")
	}
	if err := inventory.Validate(); err != nil {
		return nil, err
	}
	return &inventory, nil
}

func (inventory *AdapterInventory) Validate() error {
	if inventory.SchemaVersion != SchemaVersion || inventory.Hosts == nil {
		return errors.New("adapter inventory requires schema_version 1 and a hosts array")
	}
	seen := map[netip.Addr]bool{}
	for _, host := range inventory.Hosts {
		if !validAddress(host.Address) || seen[host.Address] || strings.TrimSpace(host.AdapterID) == "" ||
			len(host.AdapterID) > 256 || host.ObservedAt.IsZero() || host.Listeners == nil || len(host.Listeners) > 4096 {
			return fmt.Errorf("invalid or duplicate adapter host %q: require address, adapter_id, observed_at and bounded listeners", host.Address)
		}
		seen[host.Address] = true
		for _, listener := range host.Listeners {
			if (listener.Transport != "tcp" && listener.Transport != "udp") || listener.Port == 0 ||
				!listener.BindAddress.IsValid() || listener.BindAddress.IsMulticast() ||
				listener.BindAddress == netip.MustParseAddr("255.255.255.255") ||
				listener.BindAddress.Zone() != "" || listener.BindAddress.Is4In6() ||
				len(listener.Process) > 256 || len(listener.Service) > 256 {
				return fmt.Errorf("invalid listener for adapter host %s", host.Address)
			}
		}
	}
	return nil
}
