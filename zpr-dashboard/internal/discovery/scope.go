// Package discovery inventories explicitly authorized TCP endpoints without
// depending on Control Room, Simulator, or adapter runtime configuration.
package discovery

import (
	"fmt"
	"iter"
	"math/big"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

const MaxPorts = 128

// AddressRange includes both endpoints. TargetScope ranges are sorted, merged
// and non-overlapping, so even very large scopes need no host-sized allocation.
type AddressRange struct {
	First netip.Addr `json:"first"`
	Last  netip.Addr `json:"last"`
}

type TargetScope struct {
	ranges []AddressRange
}

func (scope TargetScope) Ranges() []AddressRange {
	return slices.Clone(scope.ranges)
}

func (scope TargetScope) Count() *big.Int {
	count := new(big.Int)
	for _, span := range scope.ranges {
		first := new(big.Int).SetBytes(span.First.AsSlice())
		last := new(big.Int).SetBytes(span.Last.AsSlice())
		width := new(big.Int).Sub(last, first)
		width.Add(width, big.NewInt(1))
		count.Add(count, width)
	}
	return count
}

func (scope TargetScope) Contains(address netip.Addr) bool {
	index, _ := slices.BinarySearchFunc(scope.ranges, address, func(span AddressRange, address netip.Addr) int {
		return span.Last.Compare(address)
	})
	return index < len(scope.ranges) && scope.ranges[index].First.Compare(address) <= 0
}

func (scope TargetScope) Addresses() iter.Seq[netip.Addr] {
	return func(yield func(netip.Addr) bool) {
		for _, span := range scope.ranges {
			for address := span.First; ; address = address.Next() {
				if !yield(address) {
					return
				}
				if address == span.Last {
					break
				}
			}
		}
	}
}

func validAddress(address netip.Addr) bool {
	return address.IsValid() && address.Zone() == "" && !address.IsUnspecified() &&
		!address.IsMulticast() && address != netip.MustParseAddr("255.255.255.255") &&
		!address.Is4In6() && !address.IsLinkLocalUnicast()
}

// ParseTargets accepts comma-separated literal IPs, CIDRs and dash ranges.
// IPv4 network/broadcast addresses are excluded for prefixes shorter than /31.
func ParseTargets(value string) (TargetScope, error) {
	return ParseTargetEntries(strings.Split(value, ","))
}

// ParseTargetEntries requires one IP, CIDR or range per entry.
func ParseTargetEntries(entries []string) (TargetScope, error) {
	var scope TargetScope
	size := 0
	for index, entry := range entries {
		size += len(entry)
		if index > 0 {
			size++
		}
		if size > 65536 {
			return TargetScope{}, fmt.Errorf("target specification exceeds 64 KiB")
		}
		entry = strings.TrimSpace(entry)
		span, err := parseTargetEntry(entry)
		if err != nil {
			return TargetScope{}, err
		}
		if err := validateRange(span); err != nil {
			return TargetScope{}, fmt.Errorf("unsupported target %q: %w", entry, err)
		}
		scope.ranges = append(scope.ranges, span)
	}
	if len(scope.ranges) == 0 {
		return TargetScope{}, fmt.Errorf("at least one target is required")
	}
	slices.SortFunc(scope.ranges, func(a, b AddressRange) int {
		if order := a.First.Compare(b.First); order != 0 {
			return order
		}
		return a.Last.Compare(b.Last)
	})
	merged := scope.ranges[:0]
	for _, span := range scope.ranges {
		if len(merged) > 0 {
			previous := &merged[len(merged)-1]
			if previous.Last.Is4() == span.First.Is4() &&
				(span.First.Compare(previous.Last) <= 0 || previous.Last.Next() == span.First) {
				if span.Last.Compare(previous.Last) > 0 {
					previous.Last = span.Last
				}
				continue
			}
		}
		merged = append(merged, span)
	}
	scope.ranges = merged
	return scope, nil
}

func parseTargetEntry(entry string) (AddressRange, error) {
	if address, err := netip.ParseAddr(entry); err == nil {
		return AddressRange{address, address}, nil
	}
	if strings.Contains(entry, "-") {
		bounds := strings.Split(entry, "-")
		if len(bounds) == 2 {
			first, firstErr := netip.ParseAddr(strings.TrimSpace(bounds[0]))
			lastText := strings.TrimSpace(bounds[1])
			if firstErr == nil && first.Is4() && !strings.ContainsAny(lastText, ".:") {
				octet, err := strconv.Atoi(lastText)
				if err == nil && octet >= 0 && octet <= 255 && strconv.Itoa(octet) == lastText {
					bytes := first.As4()
					bytes[3] = byte(octet)
					lastText = netip.AddrFrom4(bytes).String()
				}
			}
			last, lastErr := netip.ParseAddr(lastText)
			if firstErr == nil && lastErr == nil {
				return AddressRange{first, last}, nil
			}
		}
		return AddressRange{}, fmt.Errorf("invalid target range %q: use IP-IP or IPv4-finalOctet", entry)
	}
	prefix, err := netip.ParsePrefix(entry)
	if err != nil {
		return AddressRange{}, fmt.Errorf("invalid target %q: expected a literal IP, CIDR or dash range", entry)
	}
	prefix = prefix.Masked()
	span := AddressRange{prefix.Addr(), prefixLast(prefix)}
	if span.First.Is4() && prefix.Bits() < 31 {
		span.First, span.Last = span.First.Next(), span.Last.Prev()
	}
	return span, nil
}

func prefixLast(prefix netip.Prefix) netip.Addr {
	bytes := prefix.Masked().Addr().AsSlice()
	for bit := prefix.Bits(); bit < len(bytes)*8; bit++ {
		bytes[bit/8] |= 1 << (7 - bit%8)
	}
	address, _ := netip.AddrFromSlice(bytes)
	return address
}

var excludedTargets = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/32"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("255.255.255.255/32"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::ffff:0:0/96"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func validateRange(span AddressRange) error {
	if !validAddress(span.First) || !validAddress(span.Last) ||
		span.First.Is4() != span.Last.Is4() || span.First.Compare(span.Last) > 0 {
		return fmt.Errorf("require ascending unicast endpoints of the same IP family, without zones or link-local addresses")
	}
	for _, prefix := range excludedTargets {
		if span.First.Is4() != prefix.Addr().Is4() {
			continue
		}
		if span.First.Compare(prefixLast(prefix)) <= 0 && span.Last.Compare(prefix.Addr()) >= 0 {
			return fmt.Errorf("range intersects unsupported addresses in %s", prefix)
		}
	}
	return nil
}

func ParsePorts(value string) ([]uint16, error) {
	if len(value) > 4096 {
		return nil, fmt.Errorf("port specification exceeds 4 KiB")
	}
	seen := map[uint16]bool{}
	parse := func(value string) (int, error) {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return 0, fmt.Errorf("invalid TCP port %q", value)
		}
		return port, nil
	}
	for _, entry := range strings.Split(value, ",") {
		bounds := strings.Split(strings.TrimSpace(entry), "-")
		if len(bounds) > 2 {
			return nil, fmt.Errorf("invalid port range %q", entry)
		}
		first, err := parse(bounds[0])
		if err != nil {
			return nil, err
		}
		last := first
		if len(bounds) == 2 {
			last, err = parse(bounds[1])
			if err != nil {
				return nil, err
			}
		}
		if last < first || last-first+1 > MaxPorts {
			return nil, fmt.Errorf("invalid or oversized port range %q (maximum %d ports)", entry, MaxPorts)
		}
		for port := first; port <= last; port++ {
			seen[uint16(port)] = true
			if len(seen) > MaxPorts {
				return nil, fmt.Errorf("port scope exceeds %d ports", MaxPorts)
			}
		}
	}
	ports := make([]uint16, 0, len(seen))
	for port := range seen {
		ports = append(ports, port)
	}
	slices.Sort(ports)
	return ports, nil
}
