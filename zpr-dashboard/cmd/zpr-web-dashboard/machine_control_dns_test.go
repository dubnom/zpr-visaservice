//go:build !production

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestGrantedZPRAddressSelectsOnlyTheGrantedGlobalAddress(t *testing.T) {
	granted := net.ParseIP("fd5a:5052:adda:1::1234")
	addresses := []net.Addr{
		&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
		&net.IPNet{IP: net.ParseIP("192.0.2.10"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("fd00:1:1::1"), Mask: net.CIDRMask(64, 128)},
		&net.IPNet{IP: granted, Mask: net.CIDRMask(32, 128)},
	}
	address, err := grantedZPRAddress(addresses)
	if err != nil || address != granted.String() {
		t.Fatalf("granted address = %q, %v", address, err)
	}
	if _, err := grantedZPRAddress(addresses[:3]); err == nil {
		t.Fatal("a missing granted address must be explicit")
	}
}

func TestPreferredGrantedAddressPrefersTheZPRPrefix(t *testing.T) {
	loopback, granted := netip.MustParseAddr("::1"), netip.MustParseAddr("fd5a:5052::9")
	if preferredGrantedAddress([]netip.Addr{loopback, granted}) != granted {
		t.Fatal("granted ZPR address was not preferred")
	}
	if preferredGrantedAddress([]netip.Addr{loopback}) != loopback {
		t.Fatal("the only answer must be used")
	}
}

func TestMachineControlDNSDialContextRequiresIPv6Server(t *testing.T) {
	for _, server := range []string{"", "203.0.113.5", "dns.example"} {
		if _, err := machineControlDNSDialContext(server); err == nil {
			t.Fatalf("server %q accepted", server)
		}
	}
	if _, err := machineControlDNSDialContext("fd5a:5052::53"); err != nil {
		t.Fatal(err)
	}
}

// serveSingleAAAATCPDNS answers every TCP DNS query with one AAAA record so
// the controller's per-connection resolution path can be exercised without a
// production data plane.
func serveSingleAAAATCPDNS(t *testing.T, answer netip.Addr) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func(connection net.Conn) {
				defer connection.Close()
				for {
					length := make([]byte, 2)
					if _, err := io.ReadFull(connection, length); err != nil {
						return
					}
					query := make([]byte, binary.BigEndian.Uint16(length))
					if _, err := io.ReadFull(connection, query); err != nil || len(query) < 12 {
						return
					}
					// Parse the question name to copy exactly one question.
					offset := 12
					for offset < len(query) && query[offset] != 0 {
						offset += int(query[offset]) + 1
					}
					offset += 5 // terminating zero plus QTYPE and QCLASS
					if offset > len(query) {
						return
					}
					response := make([]byte, 0, offset+28)
					response = append(response, query[0], query[1], 0x81, 0x80, 0, 1, 0, 1, 0, 0, 0, 0)
					response = append(response, query[12:offset]...)
					response = append(response, 0xC0, 0x0C, 0, 28, 0, 1, 0, 0, 0, 30, 0, 16)
					raw := answer.As16()
					response = append(response, raw[:]...)
					framed := append([]byte{byte(len(response) >> 8), byte(len(response))}, response...)
					if _, err := connection.Write(framed); err != nil {
						return
					}
				}
			}(connection)
		}
	}()
	return listener.Addr().String()
}

func TestMachineControlResolverDialContextResolvesPerConnection(t *testing.T) {
	target, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 loopback unavailable", err)
	}
	defer target.Close()
	accepted := make(chan struct{}, 4)
	go func() {
		for {
			connection, err := target.Accept()
			if err != nil {
				return
			}
			accepted <- struct{}{}
			connection.Close()
		}
	}()
	dnsAddress := serveSingleAAAATCPDNS(t, netip.MustParseAddr("::1"))
	dial := machineControlResolverDialContext(dnsAddress)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, port, err := net.SplitHostPort(target.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	connection, err := dial(ctx, "tcp", net.JoinHostPort("simulator-control.svc.zpr", port))
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
	select {
	case <-accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("resolved connection never reached the control listener")
	}
	// IP literals bypass resolution entirely.
	connection, err = dial(ctx, "tcp", net.JoinHostPort("::1", port))
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
}

func TestWaitForRetainedControlPublicationFailsClosedWithEvidence(t *testing.T) {
	organization := retainedRuntimeTestOrganization(t)
	granted := "fd5a:5052:adda:1::77"
	run := func(recorded, published string) func(context.Context, string, ...string) (string, error) {
		return func(_ context.Context, executable string, args ...string) (string, error) {
			if executable != "docker" {
				t.Fatalf("unexpected executable %q", executable)
			}
			joined := strings.Join(args, " ")
			if strings.Contains(joined, "cat "+retainedControlAddressFile) {
				if recorded == "" {
					return "", fmt.Errorf("no such file")
				}
				return recorded + "\n", nil
			}
			if strings.Contains(joined, "dig") {
				if !strings.Contains(joined, "simulator-control.svc.zpr.") {
					t.Fatalf("publication readback must use the control DNS name: %s", joined)
				}
				return published + "\n", nil
			}
			t.Fatalf("unexpected command: %s", joined)
			return "", nil
		}
	}
	if err := waitForRetainedControlPublication(context.Background(), organization, "fd5a:5052::53", run(granted, granted)); err != nil {
		t.Fatal(err)
	}
	if err := waitForRetainedControlPublication(context.Background(), organization, "fd5a:5052::53", run("203.0.113.9", granted)); err == nil ||
		!strings.Contains(err.Error(), "invalid granted control address") {
		t.Fatal("an address outside the ZPR prefix must be refused", err)
	}
	short, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := waitForRetainedControlPublication(short, organization, "fd5a:5052::53", run(granted, "fd5a:5052::1")); err == nil ||
		!strings.Contains(err.Error(), "has not published") {
		t.Fatal("missing production publication must be explicit", err)
	}
	unrecorded, cancelUnrecorded := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelUnrecorded()
	if err := waitForRetainedControlPublication(unrecorded, organization, "fd5a:5052::53", run("", "")); err == nil ||
		!strings.Contains(err.Error(), "has not recorded") {
		t.Fatal("a silent callback must be explicit", err)
	}
}

func TestRetainedCallbackProxyRefusesConfiguredListener(t *testing.T) {
	t.Setenv("SIMULATION_RETAINED_CALLBACK_NODE", "172.26.0.2:5000")
	err := runMachineControlProxy("[fd5a:5052::1]:8792", "172.26.0.1:24995")
	if err == nil || !strings.Contains(err.Error(), "granted dynamic ZPR address") {
		t.Fatal("a configured retained listener must be refused", err)
	}
}
