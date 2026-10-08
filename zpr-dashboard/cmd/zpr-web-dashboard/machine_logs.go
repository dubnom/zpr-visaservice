package main

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

type machineLogSource struct {
	Name  string   `json:"name"`
	Lines []string `json:"lines"`
	Error string   `json:"error,omitempty"`
}

type machineLogView struct {
	Machine    simulatorMachine        `json:"machine"`
	State      string                  `json:"state"`
	User       string                  `json:"user,omitempty"`
	Controller machineControllerStatus `json:"controller"`
	Session    simulatorUserSession    `json:"session"`
	Workloads  []workerWorkload        `json:"workloads"`
	Sources    []machineLogSource      `json:"sources"`
}

type workerWorkload struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Agent   string `json:"agent"`
	Address string `json:"address"`
	State   string `json:"state"`
}

func workerWorkloads(components []simulatorComponent, selected []string, states map[string]string) []workerWorkload {
	workloads := make([]workerWorkload, 0, len(selected))
	for _, name := range selected {
		for _, component := range components {
			if component.Name != name {
				continue
			}
			state := states[name]
			if state == "" {
				state = "unknown"
			}
			workloads = append(workloads, workerWorkload{Name: name, Kind: component.Kind, Agent: component.Agent, Address: component.Address, State: state})
			break
		}
	}
	return workloads
}

var machineLogSecrets = regexp.MustCompile(`(?i)(\bBearer\s+[A-Za-z0-9._~+/=-]+|\bzpr_vsapi\.[0-9a-f]{8}\.[A-Za-z0-9_-]+|(?:authorization|x-api-key|password|secret|token)["']?\s*[:=]\s*["']?(?:bearer\s+)?[^\s,;"}]+)`)

func machineLogLines(output string) []string {
	if len(output) > 65536 {
		output = output[len(output)-65536:]
		if boundary := strings.IndexByte(output, '\n'); boundary >= 0 {
			output = output[boundary+1:]
		}
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}
	}
	if len(lines) > 100 {
		lines = lines[len(lines)-100:]
	}
	for index, line := range lines {
		lines[index] = machineLogSecrets.ReplaceAllString(line, "[REDACTED]")
	}
	return lines
}

func readMachineLogSources(ctx context.Context, machineID string, workloads []string, execute func(context.Context, string, ...string) (string, error)) []machineLogSource {
	return readSelectedMachineLogSources(ctx, machineID, workloads, execute, "all")
}

func readSelectedMachineLogSources(ctx context.Context, machineID string, workloads []string, execute func(context.Context, string, ...string) (string, error), category string) []machineLogSource {
	container := machineContainerName(machineID)
	sources := make([]machineLogSource, 0)
	read := func(name string, args ...string) {
		output, err := execute(ctx, "docker", args...)
		source := machineLogSource{Name: name, Lines: []string{}}
		if err != nil {
			source.Error = "Log source unavailable"
		} else {
			source.Lines = machineLogLines(output)
		}
		sources = append(sources, source)
	}
	if category != "workload" {
		read("Controller", "logs", "--tail", "100", "--timestamps", container)
		read("Control adapter", "exec", container, "tail", "-n", "100", "/tmp/machine-control-ph.log")
	}
	for _, workload := range workloads {
		if _, ok := machineWorkload(workload); !ok {
			continue
		}
		if category != "workload" {
			read(workload+" adapter", "exec", container, "tail", "-n", "100", "/tmp/"+workload+".log")
		}
		if _, ok := testLogPorts[workload]; ok && category != "adapter" {
			read(workload+" events", "exec", container, "tail", "-n", "100", testLogPath(workload))
		}
	}
	return sources
}

func readExitedMachineLogSources(ctx context.Context, machineID string, execute func(context.Context, string, ...string) (string, error), readFile func(context.Context, string, string) (string, error), category string) []machineLogSource {
	container := machineContainerName(machineID)
	sources := make([]machineLogSource, 0)
	readFileSource := func(name, path string) {
		output, err := readFile(ctx, container, path)
		if err == nil {
			sources = append(sources, machineLogSource{Name: name, Lines: machineLogLines(output)})
		}
	}
	if category != "workload" {
		output, err := execute(ctx, "docker", "logs", "--tail", "100", "--timestamps", container)
		if err == nil {
			sources = append(sources, machineLogSource{Name: "Controller", Lines: machineLogLines(output)})
		}
		readFileSource("Control adapter", "/tmp/machine-control-ph.log")
	}
	for _, workload := range []string{"echo-service", "finance-client", "internet-gateway", "metrics-service", "operations-client", "telemetry-client"} {
		if _, ok := machineWorkload(workload); !ok {
			continue
		}
		if category != "workload" {
			readFileSource(workload+" adapter", "/tmp/"+workload+".log")
		}
		if _, ok := testLogPorts[workload]; ok && category != "adapter" {
			readFileSource(workload+" events", testLogPath(workload))
		}
	}
	return sources
}

func readExitedContainerLogFile(ctx context.Context, container, path string) (string, error) {
	command := exec.CommandContext(ctx, "docker", "cp", container+":"+path, "-")
	stdout, err := command.StdoutPipe()
	if err != nil {
		return "", err
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return "", err
	}
	output, readErr := readMachineLogArchiveTail(stdout)
	if readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return "", readErr
	}
	if err := command.Wait(); err != nil {
		return "", err
	}
	return string(output), nil
}

func readMachineLogArchiveTail(source io.Reader) ([]byte, error) {
	archive := tar.NewReader(source)
	header, err := archive.Next()
	if err != nil {
		return nil, err
	}
	if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
		return nil, errors.New("container log archive did not contain a regular file")
	}
	if header.Size > 65536 {
		if _, err := io.CopyN(io.Discard, archive, header.Size-65536); err != nil {
			return nil, err
		}
	}
	output, err := io.ReadAll(io.LimitReader(archive, 65536))
	if err != nil {
		return nil, err
	}
	for {
		_, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(io.Discard, archive); err != nil {
			return nil, err
		}
	}
	return bytes.TrimSpace(output), nil
}

func handleSimulatorMachineLogs(w http.ResponseWriter, r *http.Request) {
	handleMachineLogs(w, r, "workload")
}

func handleSimulatorAdapterLogs(w http.ResponseWriter, r *http.Request) {
	handleMachineLogs(w, r, "adapter")
}

func handleMachineLogs(w http.ResponseWriter, r *http.Request, category string) {
	manifest, err := readSimulatorManifest()
	if err != nil {
		writeWorkspaceError(w, http.StatusServiceUnavailable, "simulation manifest unavailable")
		return
	}
	ids := make([]string, 0, len(manifest.Machines))
	for _, machine := range manifest.Machines {
		ids = append(ids, machine.ID)
	}
	states := simulatorMachineContainerStates(ids)
	sessions := simulatorSessions.snapshot(ids)
	controllers := simulatorControllers.snapshot(ids, time.Now())
	componentStates := simulatorRuntimeComponentStates(manifest, nil, simulatorSelectedAgentLinkStates(manifest, sessions))
	views := make([]machineLogView, len(manifest.Machines))
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	workers := make(chan struct{}, 4)
	var wait sync.WaitGroup
	for index, machine := range manifest.Machines {
		session := sessions[machine.ID]
		views[index] = machineLogView{
			Machine: machine, State: states[machine.ID], User: session.User,
			Controller: controllers[machine.ID], Session: session,
			Workloads: workerWorkloads(manifest.Components, session.Workloads, componentStates),
			Sources:   []machineLogSource{},
		}
		if states[machine.ID] == "exited" {
			wait.Add(1)
			go func(index int, machineID string) {
				defer wait.Done()
				select {
				case workers <- struct{}{}:
					defer func() { <-workers }()
				case <-ctx.Done():
					return
				}
				views[index].Sources = readExitedMachineLogSources(ctx, machineID, scenarioCommand, readExitedContainerLogFile, category)
			}(index, machine.ID)
			continue
		}
		if states[machine.ID] != "running" {
			continue
		}
		wait.Add(1)
		go func(index int, machineID string) {
			defer wait.Done()
			select {
			case workers <- struct{}{}:
				defer func() { <-workers }()
			case <-ctx.Done():
				views[index].Sources = []machineLogSource{{Name: "Machine", Lines: []string{}, Error: "Log collection timed out"}}
				return
			}
			views[index].Sources = readSelectedMachineLogSources(ctx, machineID, sessions[machineID].Workloads, scenarioCommand, category)
		}(index, machine.ID)
	}
	wait.Wait()
	w.Header().Set("Cache-Control", "no-store")
	writeSimulatorJSON(w, map[string]any{"organization_id": manifest.OrganizationID, "updated_at": time.Now().UTC(), "machines": views})
}
