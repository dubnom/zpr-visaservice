//go:build !production

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EVALUATION ONLY: allow/deny workloads between the Simulator's data-plane
// endpoints. Expected outcomes are Simulator scenario intent; Control Room
// observes the resulting visas and denies independently through
// Control-Service and never reads these results.
const (
	simulatorDataPlaneWorkloadIntervalEnv = "SIMULATOR_DATA_PLANE_WORKLOAD_INTERVAL"
	simulatorDataPlaneWorkloadDefault     = 30 * time.Second
	simulatorDataPlaneWorkloadMinimum     = 5 * time.Second
	// The Visa Service's fixed ZPR address. The data-plane policy lets only
	// the node ping it, so an endpoint ping is denied by policy.
	simulatorDataPlaneVisaServiceAddress = "fd5a:5052::1"
)

var simulatorPingReceived = regexp.MustCompile(`(\d+) packets transmitted, (\d+) (?:packets )?received`)

type simulatorDataPlaneProbe struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Source        string `json:"source"`
	Target        string `json:"target"`
	TargetAddress string `json:"target_address"`
	Port          int    `json:"port,omitempty"`
	Expected      string `json:"expected"`
}

type simulatorDataPlaneProbeResult struct {
	simulatorDataPlaneProbe
	Observed  string `json:"observed"`
	Replies   int    `json:"replies"`
	Matches   bool   `json:"matches_expectation"`
	CheckedAt string `json:"checked_at,omitempty"`
	Error     string `json:"error,omitempty"`
}

// simulatorDataPlaneProbes pings the next endpoint (allowed by the
// per-install policy) and the Visa Service (denied) from every endpoint, and
// sends to each of the next endpoint's listeners: allowed only when the
// install policy publishes the port.
func simulatorDataPlaneProbes(endpoints []simulatorDataPlaneEndpoint, listeners []simulatorDataPlaneListener) []simulatorDataPlaneProbe {
	probes := []simulatorDataPlaneProbe{}
	for index, endpoint := range endpoints {
		if len(endpoints) > 1 {
			peer := endpoints[(index+1)%len(endpoints)]
			probes = append(probes, simulatorDataPlaneProbe{
				Name: endpoint.Name + " ping " + peer.Name, Kind: "ping", Source: endpoint.Name,
				Target: peer.Name, TargetAddress: peer.ZPRAddress, Expected: "allow",
			})
			for _, listener := range listeners {
				expected := "deny"
				if listener.InPolicy {
					expected = "allow"
				}
				probes = append(probes, simulatorDataPlaneProbe{
					Name: fmt.Sprintf("%s %s/%d %s", endpoint.Name, listener.Protocol, listener.Port, peer.Name), Kind: listener.Protocol,
					Source: endpoint.Name, Target: peer.Name, TargetAddress: peer.ZPRAddress, Port: listener.Port, Expected: expected,
				})
			}
		}
		probes = append(probes, simulatorDataPlaneProbe{
			Name: endpoint.Name + " ping visa service", Kind: "ping", Source: endpoint.Name,
			Target: "vs.zpr", TargetAddress: simulatorDataPlaneVisaServiceAddress, Expected: "deny",
		})
	}
	return probes
}

func simulatorDataPlaneWorkloadInterval() (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(simulatorDataPlaneWorkloadIntervalEnv))
	if value == "" {
		return simulatorDataPlaneWorkloadDefault, nil
	}
	if value == "0" || value == "off" {
		return 0, nil
	}
	interval, err := time.ParseDuration(value)
	if err != nil || interval < simulatorDataPlaneWorkloadMinimum {
		return 0, fmt.Errorf("%s must be a duration of at least %s, or 0/off", simulatorDataPlaneWorkloadIntervalEnv, simulatorDataPlaneWorkloadMinimum)
	}
	return interval, nil
}

type simulatorDataPlaneWorkloads struct {
	plane    *simulatorDataPlaneEndpoints
	interval time.Duration
	probes   []simulatorDataPlaneProbe

	mu      sync.Mutex
	results map[string]simulatorDataPlaneProbeResult
	rounds  int
}

func newSimulatorDataPlaneWorkloads(plane *simulatorDataPlaneEndpoints) (*simulatorDataPlaneWorkloads, error) {
	if plane == nil {
		return nil, nil
	}
	interval, err := simulatorDataPlaneWorkloadInterval()
	if err != nil {
		return nil, err
	}
	return &simulatorDataPlaneWorkloads{plane: plane, interval: interval, probes: simulatorDataPlaneProbes(plane.handoff.Endpoints, plane.handoff.Listeners), results: map[string]simulatorDataPlaneProbeResult{}}, nil
}

func (plane *simulatorDataPlaneEndpoints) runningEndpoints() map[string]bool {
	plane.mu.Lock()
	defer plane.mu.Unlock()
	running := map[string]bool{}
	for name, status := range plane.status {
		running[name] = plane.planeUp && status.Running
	}
	return running
}

// probe runs once per probe from inside the source endpoint container. A
// first packet may be lost while the node requests a visa, so any reply
// counts as reachable.
func (workloads *simulatorDataPlaneWorkloads) probe(ctx context.Context, probe simulatorDataPlaneProbe, running bool) simulatorDataPlaneProbeResult {
	result := simulatorDataPlaneProbeResult{simulatorDataPlaneProbe: probe, Observed: "error", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	if !running {
		result.Error = "source endpoint is not running"
		return result
	}
	container := workloads.plane.handoff.Network + "-" + probe.Source
	if probe.Kind == "tcp" || probe.Kind == "udp" {
		output, err := workloads.plane.docker(ctx, "exec", container, "zpr-evaluation-probe", probe.Kind, probe.TargetAddress, strconv.Itoa(probe.Port))
		switch strings.TrimSpace(output) {
		case "probe reply":
			result.Replies = 1
		case "probe no-reply":
		default:
			result.Error = "probe could not run"
			log.Printf("data-plane workload %q failed: %v: %s", probe.Name, err, output)
			return result
		}
		return workloads.observed(result)
	}
	output, err := workloads.plane.docker(ctx, "exec", container, "ping", "-6", "-n", "-q", "-c", "3", "-i", "0.5", "-W", "2", probe.TargetAddress)
	match := simulatorPingReceived.FindStringSubmatch(output)
	if match == nil {
		result.Error = "probe could not run"
		if err != nil {
			log.Printf("data-plane workload %q failed: %v: %s", probe.Name, err, output)
		}
		return result
	}
	result.Replies, _ = strconv.Atoi(match[2])
	return workloads.observed(result)
}

func (workloads *simulatorDataPlaneWorkloads) observed(result simulatorDataPlaneProbeResult) simulatorDataPlaneProbeResult {
	result.Observed = "unreachable"
	if result.Replies > 0 {
		result.Observed = "reachable"
	}
	result.Matches = (result.Expected == "allow") == (result.Replies > 0)
	if !result.Matches {
		log.Printf("data-plane workload %q expected %s but observed %s", result.Name, result.Expected, result.Observed)
	}
	return result
}

func (workloads *simulatorDataPlaneWorkloads) round(ctx context.Context) {
	running := workloads.plane.runningEndpoints()
	results := make([]simulatorDataPlaneProbeResult, len(workloads.probes))
	var wait sync.WaitGroup
	for index, probe := range workloads.probes {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index] = workloads.probe(ctx, probe, running[probe.Source])
		}()
	}
	wait.Wait()
	workloads.mu.Lock()
	for _, result := range results {
		workloads.results[result.Name] = result
	}
	workloads.rounds++
	workloads.mu.Unlock()
}

func (workloads *simulatorDataPlaneWorkloads) run(ctx context.Context) {
	if workloads.interval == 0 {
		return
	}
	ticker := time.NewTicker(workloads.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		roundContext, cancel := context.WithTimeout(ctx, time.Minute)
		workloads.round(roundContext)
		cancel()
	}
}

type simulatorDataPlaneWorkloadsResponse struct {
	Configured      bool                            `json:"configured"`
	EvaluationOnly  bool                            `json:"evaluation_only,omitempty"`
	Enabled         bool                            `json:"enabled"`
	IntervalSeconds int                             `json:"interval_seconds,omitempty"`
	Rounds          int                             `json:"rounds"`
	Mismatches      int                             `json:"mismatches"`
	Errors          int                             `json:"errors"`
	Probes          []simulatorDataPlaneProbeResult `json:"probes"`
}

func (workloads *simulatorDataPlaneWorkloads) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	response := simulatorDataPlaneWorkloadsResponse{Probes: []simulatorDataPlaneProbeResult{}}
	if workloads != nil {
		workloads.mu.Lock()
		response.Configured, response.EvaluationOnly = true, true
		response.Enabled, response.IntervalSeconds, response.Rounds = workloads.interval > 0, int(workloads.interval/time.Second), workloads.rounds
		for _, probe := range workloads.probes {
			result, ok := workloads.results[probe.Name]
			if !ok {
				result = simulatorDataPlaneProbeResult{simulatorDataPlaneProbe: probe, Observed: "pending"}
			} else if result.Observed == "error" {
				response.Errors++
			} else if !result.Matches {
				response.Mismatches++
			}
			response.Probes = append(response.Probes, result)
		}
		workloads.mu.Unlock()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(response)
}
