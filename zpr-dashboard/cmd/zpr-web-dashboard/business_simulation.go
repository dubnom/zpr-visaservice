package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/simulator/business"
)

func businessWorkload(agent string) (machineWorkloadConfig, bool) {
	for index, spec := range business.Specs() {
		if spec.Workload == agent {
			return machineWorkloadConfig{
				address: fmt.Sprintf("fd00:1:%x::1", 40+index),
				key:     agent + ".key", tun: fmt.Sprintf("zbiz%d", index),
				services: spec.Service, routeTable: 120 + index,
			}, true
		}
	}
	return machineWorkloadConfig{}, false
}

func businessComponent(organization simulatorOrganization, name string) (simulatorComponent, bool) {
	spec, ok := business.Lookup(name)
	if !ok || !organization.BusinessSimulation {
		return simulatorComponent{}, false
	}
	return simulatorComponent{Name: name, Agent: name, Kind: "service", Machine: spec.Machine}, true
}

func validateBusinessSimulationOrganization(organization simulatorOrganization) error {
	if !organization.BusinessSimulation {
		return nil
	}
	if organization.Runtime.Driver != "docker-multinode" {
		return errors.New("business simulations require the docker-multinode runtime")
	}
	for _, spec := range business.Specs() {
		services := organization.WorkloadServices[spec.Workload]
		if len(services) != 1 || services[0] != spec.Service || len(organization.MachineOwners[spec.Machine]) != 1 {
			return fmt.Errorf("business simulation %s requires its service registration and dedicated host owner", spec.Workload)
		}
	}
	return nil
}

func runBusinessService(address, workload, stateDirectory string) error {
	if _, ok := business.Lookup(workload); !ok {
		return errors.New("unknown business simulation")
	}
	listener, err := net.Listen("tcp6", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	handler, err := business.New(workload, filepath.Join(stateDirectory, workload+".json"), func(event business.Event) error {
		return appendTestEvent(workload, testAppEvent{
			Time: time.Now().UTC(), Role: "service", Direction: "received", Service: workload,
			Remote: event.Remote, Path: event.Method + " " + event.Path, Status: event.Status,
		})
	})
	if err != nil {
		return err
	}
	return (&http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}).Serve(listener)
}

func runBusinessClient(ctx context.Context, address, source, workload string) error {
	ip := net.ParseIP(source)
	if ip == nil || ip.To4() != nil {
		return errors.New("business simulation client requires an IPv6 source")
	}

	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{
		Timeout: 3 * time.Second, LocalAddr: &net.TCPAddr{IP: ip},
	}).DialContext}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	record, err := business.Exercise(ctx, client, "http://"+address, workload)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(record)
}

func checkBusinessService(ctx context.Context, address, workload string) error {
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+"/health", nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			var result struct {
				Service    string `json:"service"`
				Status     string `json:"status"`
				Simulation bool   `json:"simulation"`
			}
			decoder := json.NewDecoder(io.LimitReader(response.Body, 4097))
			decodeErr := decoder.Decode(&result)
			var trailing any
			trailingErr := decoder.Decode(&trailing)
			response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && trailingErr == io.EOF &&
				result.Service == workload && result.Status == "ok" && result.Simulation {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("business service did not become ready: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func businessDenyCount(denies []deny, source, destination string, port int) uint64 {
	var count uint64
	for _, entry := range denies {
		if net.ParseIP(entry.Source).Equal(net.ParseIP(source)) &&
			net.ParseIP(entry.Destination).Equal(net.ParseIP(destination)) &&
			entry.Protocol == 6 && int(entry.Port) == port {
			count += entry.Count
		}
	}
	return count
}

func executeBusinessRequest(ctx context.Context, manifest simulatorManifest, step simulatorScenarioStep) (string, error) {
	return executeBusinessRequestWithCommand(ctx, manifest, step, scenarioCommand)
}

func executeBusinessRequestWithCommand(ctx context.Context, manifest simulatorManifest, step simulatorScenarioStep, command scenarioCommandFunc) (string, error) {
	spec, ok := business.Lookup(step.Component)
	if !ok {
		return "", errors.New("unknown business simulation")
	}
	if step.Machine == spec.Machine || (step.Expected != "allow" && step.Expected != "deny") {
		return "", errors.New("business requests require a separate employee machine and explicit allow/deny expectation")
	}
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), activeSimulatorOrganizationID(manifest))
	if err != nil {
		return "", err
	}
	session := simulatorSessions.snapshot([]string{step.Machine})[step.Machine]
	if !organization.BusinessSimulation || !session.Authenticated ||
		!simulatorOrganizationAllowsUser(manifest, organization, step.Machine, session.User) {
		return "", errors.New("business request requires a logged-in owner on the configured organization machine")
	}
	// Employee traffic uses the machine's own authenticated ZPR adapter, not
	// finance-client or an identity supplied in an HTTP header.
	output, err := command(ctx, "docker", "exec", machineContainerName(step.Machine), "ip", "-j", "-6", "addr", "show", "dev", machineControlTun)
	if err != nil {
		return "", fmt.Errorf("read employee machine address: %w", err)
	}
	source := firstGrantedMachineAddress(output)
	if source == "" {
		return "", errors.New("employee machine has no Visa-assigned ZPR address")
	}
	if ip := net.ParseIP(manifest.DNSServer); ip == nil || ip.To4() != nil {
		return "", errors.New("business scenario requires an IPv6 DNS server")
	}
	if _, err := command(ctx, "docker", "exec", machineContainerName(step.Machine), "ip", "-6", "route", "replace", manifest.DNSServer+"/128", "dev", machineControlTun); err != nil {
		return "", err
	}
	output, err = command(ctx, "docker", "exec", machineContainerName(step.Machine), "dig", "+tcp", "+time=2", "+tries=1", "+short", "AAAA", "@"+manifest.DNSServer, spec.DNS)
	if err != nil {
		return "", fmt.Errorf("business DNS lookup failed: %w", err)
	}
	destination := firstIPv6Address(output)
	if destination == "" {
		return "", errors.New("business DNS lookup returned no IPv6 address")
	}
	if _, err := command(ctx, "docker", "exec", machineContainerName(step.Machine), "ip", "-6", "route", "replace", destination+"/128", "dev", machineControlTun); err != nil {
		return "", err
	}
	var admin *adminClient
	var before uint64
	if step.Expected == "deny" {
		var message string
		admin, message = newAdminClient()
		if admin == nil {
			return "", fmt.Errorf("denial verification requires Visa Admin API: %s", message)
		}
		var entries []deny
		if err := admin.getJSON(ctx, "/admin/visas/denies?limit=500", &entries); err != nil {
			return "", err
		}
		before = businessDenyCount(entries, source, destination, spec.Port)
	}
	output, requestErr := command(ctx, "docker", "exec", machineContainerName(step.Machine),
		"/usr/local/bin/zpr-machine-controller", "-mode", "business-client",
		"-listen", net.JoinHostPort(destination, strconv.Itoa(spec.Port)), "-zpr-addr", source, "-log-workload", spec.Workload)
	if step.Expected != "deny" {
		return output, requestErr
	}
	if requestErr == nil {
		return output, errors.New("business request succeeded but denial was required")
	}
	for {
		var entries []deny
		if err := admin.getJSON(ctx, "/admin/visas/denies?limit=500", &entries); err != nil {
			return output, fmt.Errorf("verify business denial: %w", err)
		}
		if businessDenyCount(entries, source, destination, spec.Port) > before {
			return output + "\nVisa Service recorded the expected TCP denial", nil
		}
		select {
		case <-ctx.Done():
			return output, fmt.Errorf("request failed without matching Visa denial evidence: %w", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func firstGrantedMachineAddress(output string) string {
	var interfaces []struct {
		Addresses []struct {
			Local string `json:"local"`
			Scope string `json:"scope"`
		} `json:"addr_info"`
	}
	if json.Unmarshal([]byte(output), &interfaces) != nil {
		return ""
	}
	for _, item := range interfaces {
		for _, address := range item.Addresses {
			ip := net.ParseIP(address.Local)
			if address.Scope == "global" && ip != nil && ip.To4() == nil && strings.HasPrefix(ip.String(), "fd5a:5052:") {
				return ip.String()
			}
		}
	}
	return ""
}
