package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var simulatorProvisionIdentifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var simulatorProvisionReleaseIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

type simulatorPackagedProvisioning struct {
	Version          int      `json:"version"`
	Driver           string   `json:"driver"`
	PrimaryMachineID string   `json:"primary_machine_id"`
	Machines         []string `json:"machines"`
	Service          string   `json:"service"`
}

func validatePackagedProvisioning(config *simulatorPackagedProvisioning) error {
	if config == nil {
		return nil
	}
	if config.Version != 2 || config.Driver != "shared-approved-bootstrap" ||
		!simulatorProvisionIdentifier.MatchString(config.PrimaryMachineID) ||
		len(config.Machines) == 0 || len(config.Machines) > 20 ||
		config.Machines[0] != config.PrimaryMachineID || config.Service != "OciWeb" {
		return errors.New("unsupported packaged provisioning configuration")
	}
	seen := make(map[string]struct{}, len(config.Machines))
	for _, machine := range config.Machines {
		if !simulatorProvisionIdentifier.MatchString(machine) {
			return errors.New("unsupported packaged provisioning machine identifier")
		}
		if _, exists := seen[machine]; exists {
			return errors.New("duplicate packaged provisioning machine identifier")
		}
		seen[machine] = struct{}{}
	}
	return nil
}

func simulatorPackagedContainerName(organization, primaryMachine, machine, releaseID string) string {
	digest := sha256.Sum256([]byte(organization + "/" + primaryMachine + "/" + releaseID))
	suffix := "endpoint"
	if machine != primaryMachine {
		suffix += "-" + machine
	}
	return fmt.Sprintf("zpr-org-%x-%s", digest[:10], suffix)
}

func simulatorUsesPackagedMachine(manifest simulatorManifest, machine string) (bool, error) {
	if os.Getenv("SIMULATION_PROVISION_ROOT") == "" {
		return false, nil
	}
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), manifest.OrganizationID)
	if err != nil {
		return false, err
	}
	if organization.PackagedProvisioning == nil {
		return false, nil
	}
	for _, configuredMachine := range organization.PackagedProvisioning.Machines {
		if configuredMachine == machine {
			return true, nil
		}
	}
	return false, nil
}

type simulatorProvisionRunner func(context.Context, string, string, string, string) ([]byte, error)

type simulatorProvisionHandler struct {
	run  simulatorProvisionRunner
	gate chan struct{}
}

func newSimulatorProvisionHandler() http.Handler {
	return &simulatorProvisionHandler{
		run: runSimulatorProvisionCommand, gate: make(chan struct{}, 1),
	}
}

func runSimulatorProvisionCommand(ctx context.Context, script, action, organization, machine string) ([]byte, error) {
	command := exec.CommandContext(ctx, "python3", script, action, organization, machine)
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = 20 * time.Second
	var output, diagnostic bytes.Buffer
	command.Stdout, command.Stderr = &output, &diagnostic
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(diagnostic.String())
		if len(detail) > 2000 {
			detail = detail[len(detail)-2000:]
		}
		return nil, fmt.Errorf("packaged provisioning failed: %w: %s", err, detail)
	}
	return output.Bytes(), nil
}

func (h *simulatorProvisionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	organization, machine := r.PathValue("organization"), r.PathValue("machine")
	if !simulatorProvisionIdentifier.MatchString(organization) || !simulatorProvisionIdentifier.MatchString(machine) {
		http.Error(w, "invalid organization or machine identifier", http.StatusBadRequest)
		return
	}
	action := r.PathValue("action")
	switch {
	case r.Method == http.MethodGet && action == "":
		action = "status"
	case r.Method == http.MethodPost &&
		(action == "approve-and-provision" || action == "start" || action == "stop" || action == "verify"):
	default:
		http.Error(w, "unsupported provisioning operation", http.StatusNotFound)
		return
	}
	if r.URL.RawQuery != "" {
		http.Error(w, "query parameters are not accepted", http.StatusBadRequest)
		return
	}
	if r.Body != nil {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1))
		if len(body) != 0 || (err != nil && !errors.Is(err, io.EOF)) {
			http.Error(w, "request bodies are not accepted", http.StatusBadRequest)
			return
		}
	}
	script := os.Getenv("SIMULATION_PROVISION_SCRIPT")
	if !filepath.IsAbs(script) || os.Getenv("SIMULATION_PROVISION_PACKAGE") == "" || os.Getenv("SIMULATION_PROVISION_ROOT") == "" {
		http.Error(w, "packaged provisioning is unavailable: installed package and persistent host-path configuration are required", http.StatusServiceUnavailable)
		return
	}
	select {
	case h.gate <- struct{}{}:
		defer func() { <-h.gate }()
	default:
		http.Error(w, "another provisioning operation is in progress", http.StatusConflict)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	output, err := h.run(ctx, script, action, organization, machine)
	if err != nil {
		log.Printf("Simulator provisioning %s/%s %s: %v", organization, machine, action, err)
		if ctx.Err() != nil {
			http.Error(w, "provisioning interrupted or deadline exceeded; inspect state before retrying", http.StatusGatewayTimeout)
		} else {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}
	var response struct {
		Version      int    `json:"version"`
		Organization string `json:"organization_id"`
		Machine      string `json:"machine_id"`
		State        string `json:"state"`
	}
	if err := json.Unmarshal(output, &response); err != nil || response.Version != 1 ||
		response.Organization != organization || response.Machine != machine || response.State == "" {
		log.Printf("Simulator provisioning returned an invalid response for %s/%s", organization, machine)
		http.Error(w, "provisioning runtime returned an invalid or mismatched response", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(output)
}
