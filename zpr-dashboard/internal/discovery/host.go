package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"
)

type HostCheck struct {
	State      string     `json:"state"`
	ObservedAt *time.Time `json:"observed_at,omitempty"`
	Names      []string   `json:"names,omitempty"`
	Error      string     `json:"error,omitempty"`
}

type hostProbe func(context.Context, netip.Addr, time.Duration) HostCheck

type hostProbes struct {
	ping       hostProbe
	reverseDNS hostProbe
}

func newHostProbes(options Options) (hostProbes, error) {
	probes := hostProbes{reverseDNS: reverseLookup}
	if options.Ping {
		if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
			return probes, errors.New("ping is supported on macOS and Linux; use -ping=false on this platform")
		}
		path, err := exec.LookPath("ping")
		if err != nil {
			return probes, fmt.Errorf("ping utility unavailable (install it or use -ping=false): %w", err)
		}
		probes.ping = func(ctx context.Context, address netip.Addr, timeout time.Duration) HostCheck {
			return pingHost(ctx, address, timeout, path)
		}
	}
	return probes, nil
}

func pingArgs(platform string, address netip.Addr) []string {
	args := []string{"-n", "-c", "1"}
	if address.Is6() && platform == "linux" {
		args = append(args, "-6")
	}
	return append(args, address.String())
}

func pingHost(ctx context.Context, address netip.Addr, timeout time.Duration, path string) HostCheck {
	// macOS ships a separate IPv6 utility; it uses the same one-packet flags.
	if address.Is6() && runtime.GOOS == "darwin" {
		var err error
		path, err = exec.LookPath("ping6")
		if err != nil {
			now := time.Now().UTC()
			return HostCheck{State: "error", ObservedAt: &now, Error: fmt.Sprintf("IPv6 ping utility unavailable: %v", err)}
		}
	}
	return runPing(ctx, timeout, path, pingArgs(runtime.GOOS, address))
}

func runPing(ctx context.Context, timeout time.Duration, path string, args []string) HostCheck {
	now := time.Now().UTC()
	result := HostCheck{ObservedAt: &now}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, path, args...)
	var stderr boundedPingOutput
	command.Stderr = &stderr
	err := command.Run()
	switch {
	case ctx.Err() != nil:
		result.State, result.Error = connectionState(ctx.Err()), ctx.Err().Error()
	case err == nil:
		result.State = "reply"
	default:
		var exit *exec.ExitError
		noReplyCode := 1
		if runtime.GOOS == "darwin" {
			noReplyCode = 2
		}
		result.State = "error"
		if errors.As(err, &exit) && exit.ExitCode() == noReplyCode {
			result.State = "no_reply"
		}
		result.Error = fmt.Sprintf("ping: %v", err)
		if len(stderr) > 0 {
			result.Error += ": " + strings.TrimSpace(string(stderr))
		}
	}
	return result
}

type boundedPingOutput []byte

func (output *boundedPingOutput) Write(data []byte) (int, error) {
	*output = append(*output, data[:min(len(data), 4096-len(*output))]...)
	return len(data), nil
}

func reverseLookup(ctx context.Context, address netip.Addr, timeout time.Duration) HostCheck {
	return lookupNames(ctx, address, timeout, net.DefaultResolver.LookupAddr)
}

func lookupNames(ctx context.Context, address netip.Addr, timeout time.Duration,
	lookup func(context.Context, string) ([]string, error)) HostCheck {
	now := time.Now().UTC()
	result := HostCheck{ObservedAt: &now}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	names, err := lookup(ctx, address.String())
	var dnsError *net.DNSError
	switch {
	case ctx.Err() != nil:
		result.State, result.Error = connectionState(ctx.Err()), ctx.Err().Error()
	case err != nil:
		result.State, result.Error = "error", err.Error()
		if errors.As(err, &dnsError) {
			if dnsError.IsNotFound {
				result.State = "no_records"
			} else if dnsError.IsTimeout {
				result.State = "timeout"
			}
		}
	default:
		result.State = "resolved"
		if len(names) == 0 {
			result.State = "no_records"
		}
		slices.Sort(names)
		result.Names = slices.Compact(names)
	}
	return result
}
