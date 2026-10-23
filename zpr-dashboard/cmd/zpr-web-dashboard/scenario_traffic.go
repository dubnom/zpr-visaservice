package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

const scenarioDenialEvidenceTimeout = 3 * time.Second

type scenarioDenialReader func(context.Context) ([]deny, error)

func executeScenarioTrafficStep(
	ctx context.Context,
	manifest simulatorManifest,
	step simulatorScenarioStep,
	command scenarioCommandFunc,
	readDenials scenarioDenialReader,
) (string, error) {
	component, err := readSimulatorComponent(manifest, step.Component)
	if err != nil {
		return "", err
	}
	target := component.Target
	if step.Target != "" {
		target = step.Target
	}

	if step.Machine != "" {
		session := simulatorSessions.snapshot([]string{step.Machine})[step.Machine]
		if !session.Authenticated {
			return "", errors.New("log in before sending traffic from a machine")
		}
		selected := false
		for _, name := range session.Workloads {
			selected = selected || name == step.Component
		}
		if !selected {
			return "", fmt.Errorf("traffic workload %q is not selected on %s", step.Component, step.Machine)
		}
	}

	machineContainer := ""
	if step.Machine != "" {
		machineContainer, _, err = simulatorScenarioMachineContainerName(manifest, step.Machine)
		if err != nil {
			return "", err
		}
	}

	var source string
	var since uint64
	if step.Expected == "deny" {
		source, err = scenarioTrafficSourceAddress(ctx, manifest, step, machineContainer, command)
		if err != nil {
			return "", err
		}
		targetIP := net.ParseIP(target)
		if targetIP == nil || targetIP.To4() != nil {
			return "", errors.New("policy-denial traffic evidence requires an IPv6 target address")
		}
		since = uint64(time.Now().UnixMilli())
	}

	var output string
	if step.Machine != "" {
		output, err = command(ctx, "docker", "exec", machineContainer, "ping6", "-c", "2", "-W", "1", target)
	} else {
		container := strings.TrimSpace(os.Getenv("SIMULATION_CONTAINER"))
		if container == "" {
			container = "zpr-local-linux-node"
		}
		output, err = command(ctx, "docker", "exec", container, "ip", "netns", "exec", component.Namespace, "ping6", "-c", "2", "-W", "1", target)
	}

	if err != nil {
		if step.Expected != "deny" {
			return output, err
		}
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) || exitError.ExitCode() != 1 {
			return output, err
		}
		evidence, err := waitForScenarioPolicyDenial(ctx, source, target, since, readDenials)
		if err != nil {
			return output, err
		}
		return fmt.Sprintf(
			"%s\nVisa Service recorded fresh %s policy-denial evidence: source=%s destination=%s protocol=%d icmp_code=%d last_deny_ms=%d (ICMP type is not included in the service record)",
			output, evidence.Code, evidence.Source, evidence.Destination, evidence.Protocol, evidence.Port, evidence.LastDenyMS,
		), nil
	}
	if step.Expected == "deny" {
		return output, errors.New("traffic was allowed, but the scenario expected denial")
	}
	return output + "\ntraffic allowed as expected", nil
}

func scenarioTrafficSourceAddress(
	ctx context.Context,
	manifest simulatorManifest,
	step simulatorScenarioStep,
	machineContainer string,
	command scenarioCommandFunc,
) (string, error) {
	var output string
	var err error
	if step.Machine != "" {
		workload, configured := machineWorkload(step.Component)
		if !configured {
			return "", fmt.Errorf("workload %q has no TUN configuration", step.Component)
		}
		output, err = command(ctx, "docker", "exec", machineContainer, "ip", "-j", "-6", "addr", "show", "dev", workload.tun)
	} else {
		component, componentErr := readSimulatorComponent(manifest, step.Component)
		if componentErr != nil {
			return "", componentErr
		}
		container := strings.TrimSpace(os.Getenv("SIMULATION_CONTAINER"))
		if container == "" {
			container = "zpr-local-linux-node"
		}
		output, err = command(ctx, "docker", "exec", container, "ip", "netns", "exec", component.Namespace, "ip", "-j", "-6", "addr", "show")
	}
	if err != nil {
		return "", fmt.Errorf("read traffic source ZPR address: %w", err)
	}
	return grantedScenarioClientAddress(output, step.Component)
}

func waitForScenarioPolicyDenial(
	ctx context.Context,
	source, destination string,
	since uint64,
	readDenials scenarioDenialReader,
) (deny, error) {
	if readDenials == nil {
		return deny{}, errors.New("Visa Service denial evidence reader is unavailable")
	}
	evidenceContext, cancel := context.WithTimeout(ctx, scenarioDenialEvidenceTimeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		entries, err := readDenials(evidenceContext)
		if err != nil {
			return deny{}, fmt.Errorf("read Visa Service policy-denial evidence: %w", err)
		}
		if evidence, ok := findFreshScenarioPolicyDenial(entries, source, destination, since); ok {
			return evidence, nil
		}
		select {
		case <-evidenceContext.Done():
			return deny{}, fmt.Errorf("traffic failed without fresh matching Visa Service policy-denial evidence: %w", evidenceContext.Err())
		case <-ticker.C:
		}
	}
}

func hasFreshScenarioPolicyDenial(entries []deny, source, destination string, since uint64) bool {
	_, ok := findFreshScenarioPolicyDenial(entries, source, destination, since)
	return ok
}

func findFreshScenarioPolicyDenial(entries []deny, source, destination string, since uint64) (deny, bool) {
	sourceIP := net.ParseIP(source)
	destinationIP := net.ParseIP(destination)
	if sourceIP == nil || destinationIP == nil {
		return deny{}, false
	}
	for _, entry := range entries {
		entrySource := net.ParseIP(entry.Source)
		entryDestination := net.ParseIP(entry.Destination)
		if entrySource != nil && entrySource.Equal(sourceIP) &&
			entryDestination != nil && entryDestination.Equal(destinationIP) &&
			entry.Protocol == 58 && entry.Port == 0 && entry.Count > 0 && entry.LastDenyMS >= since &&
			(entry.Code == "Denied" || entry.Code == "NoMatch") {
			return entry, true
		}
	}
	return deny{}, false
}
