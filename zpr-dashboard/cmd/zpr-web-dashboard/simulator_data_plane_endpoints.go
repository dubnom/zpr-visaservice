package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// EVALUATION ONLY: the Simulator runs the endpoints that an evaluation
// install admits by bootstrap key (shortcut X15). The install hands them over
// in data-plane-endpoints.json; the dependency is Simulator -> install only.
const (
	simulatorDataPlaneEndpointsEnv   = "SIMULATOR_DATA_PLANE_ENDPOINTS_FILE"
	simulatorDataPlaneImageEnv       = "SIMULATOR_DATA_PLANE_IMAGE"
	simulatorDataPlaneDefaultImage   = "zpr-evaluation-data-plane:local"
	simulatorDataPlaneReconcileEvery = 10 * time.Second
	simulatorDataPlaneEndpointLabel  = "org.zpr.evaluation-endpoint"
)

var (
	simulatorDataPlaneEndpointName = regexp.MustCompile(`^endpoint-[0-9]{2}$`)
	// Additional install nodes are <network>-node-<name> (D6b).
	simulatorDataPlaneNodeSuffix = regexp.MustCompile(`^-node-[a-z][a-z0-9-]{0,30}[a-z0-9]$`)
)

type simulatorDataPlaneEndpoint struct {
	Name         string `json:"name"`
	BootstrapKey string `json:"bootstrap_key"`
	ZPRAddress   string `json:"zpr_address"`
	// NodeContainer optionally docks this endpoint with an additional node of
	// the same install instead of the handoff's node_container.
	NodeContainer string `json:"node_container,omitempty"`
}

type simulatorDataPlaneHandoff struct {
	Version        int                          `json:"version"`
	Kind           string                       `json:"kind"`
	EvaluationOnly bool                         `json:"evaluation_only"`
	InstallID      string                       `json:"install_id"`
	Network        string                       `json:"network"`
	NodeContainer  string                       `json:"node_container"`
	CAFile         string                       `json:"ca_file"`
	Endpoints      []simulatorDataPlaneEndpoint `json:"endpoints"`
}

// loadSimulatorDataPlaneHandoff reads and strictly validates an install
// handoff; referenced files must be regular files beside it.
func loadSimulatorDataPlaneHandoff(path string) (simulatorDataPlaneHandoff, string, error) {
	var handoff simulatorDataPlaneHandoff
	absolute, err := filepath.Abs(path)
	if err != nil {
		return handoff, "", err
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return handoff, "", fmt.Errorf("read data-plane endpoint handoff: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&handoff); err != nil {
		return handoff, "", fmt.Errorf("parse data-plane endpoint handoff: %w", err)
	}
	dir := filepath.Dir(absolute)
	if strings.ContainsAny(dir, ",\"=") {
		return handoff, "", errors.New("data-plane endpoint handoff directory must not contain commas, quotes or '='")
	}
	if handoff.Version != 1 || handoff.Kind != "zpr-evaluation-data-plane-endpoints" || !handoff.EvaluationOnly {
		return handoff, "", errors.New("not an EVALUATION ONLY data-plane endpoint handoff")
	}
	if _, err := hex.DecodeString(handoff.InstallID); err != nil || len(handoff.InstallID) != 24 {
		return handoff, "", errors.New("data-plane endpoint handoff has an invalid install id")
	}
	network := "zpr-evaluation-" + handoff.InstallID + "-data-plane"
	if handoff.Network != network || handoff.NodeContainer != network {
		return handoff, "", errors.New("data-plane endpoint handoff network does not belong to its install")
	}
	regular := func(name string) error {
		if name == "" || filepath.Base(name) != name {
			return fmt.Errorf("handoff file %q must be a plain name beside the handoff", name)
		}
		info, err := os.Lstat(filepath.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("handoff file %s is not a regular file", name)
		}
		return nil
	}
	if err := regular(handoff.CAFile); err != nil {
		return handoff, "", err
	}
	if len(handoff.Endpoints) == 0 || len(handoff.Endpoints) > 16 {
		return handoff, "", errors.New("data-plane endpoint handoff must list 1-16 endpoints")
	}
	seen := map[string]bool{}
	for _, endpoint := range handoff.Endpoints {
		address := net.ParseIP(endpoint.ZPRAddress)
		if node := endpoint.NodeContainer; node != "" && (!strings.HasPrefix(node, network) || !simulatorDataPlaneNodeSuffix.MatchString(strings.TrimPrefix(node, network))) {
			return handoff, "", fmt.Errorf("data-plane endpoint %q docks with a node outside its install", endpoint.Name)
		}
		if !simulatorDataPlaneEndpointName.MatchString(endpoint.Name) || seen[endpoint.Name] || seen[endpoint.ZPRAddress] ||
			endpoint.BootstrapKey != "dp-"+endpoint.Name+"-auth.key" || address == nil || address.To4() != nil || address.String() != endpoint.ZPRAddress {
			return handoff, "", fmt.Errorf("invalid data-plane endpoint %q", endpoint.Name)
		}
		if err := regular(endpoint.BootstrapKey); err != nil {
			return handoff, "", err
		}
		seen[endpoint.Name], seen[endpoint.ZPRAddress] = true, true
	}
	return handoff, dir, nil
}

type simulatorDataPlaneEndpointStatus struct {
	Name       string `json:"name"`
	ZPRAddress string `json:"zpr_address"`
	Container  string `json:"container"`
	Running    bool   `json:"running"`
	Error      string `json:"error,omitempty"`
}

type simulatorDataPlaneEndpoints struct {
	handoff simulatorDataPlaneHandoff
	dir     string
	image   string
	docker  func(ctx context.Context, args ...string) (string, error)

	mu        sync.Mutex
	status    map[string]simulatorDataPlaneEndpointStatus
	planeUp   bool
	checkedAt time.Time
}

func newSimulatorDataPlaneEndpointsFromEnv() (*simulatorDataPlaneEndpoints, error) {
	path := strings.TrimSpace(os.Getenv(simulatorDataPlaneEndpointsEnv))
	if path == "" {
		return nil, nil
	}
	return newSimulatorDataPlaneEndpoints(path)
}

func newSimulatorDataPlaneEndpoints(path string) (*simulatorDataPlaneEndpoints, error) {
	handoff, dir, err := loadSimulatorDataPlaneHandoff(path)
	if err != nil {
		return nil, err
	}
	image := strings.TrimSpace(os.Getenv(simulatorDataPlaneImageEnv))
	if image == "" {
		image = simulatorDataPlaneDefaultImage
	}
	if strings.ContainsAny(image, " \t\n") {
		return nil, errors.New(simulatorDataPlaneImageEnv + " must name a local image")
	}
	return &simulatorDataPlaneEndpoints{handoff: handoff, dir: dir, image: image, docker: simulatorDocker, status: map[string]simulatorDataPlaneEndpointStatus{}}, nil
}

func simulatorDocker(ctx context.Context, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func (plane *simulatorDataPlaneEndpoints) containerName(endpoint simulatorDataPlaneEndpoint) string {
	return plane.handoff.Network + "-" + endpoint.Name
}

func (plane *simulatorDataPlaneEndpoints) nodeContainer(endpoint simulatorDataPlaneEndpoint) string {
	if endpoint.NodeContainer != "" {
		return endpoint.NodeContainer
	}
	return plane.handoff.NodeContainer
}

// runArguments mounts only this endpoint's bootstrap key and the ZPR CA.
func (plane *simulatorDataPlaneEndpoints) runArguments(endpoint simulatorDataPlaneEndpoint) []string {
	const keyPath, caPath = "/run/zpr-endpoint/bootstrap.key", "/run/zpr-endpoint/ca.crt"
	return []string{"run", "-d", "--rm", "--privileged", "--init", "--pull", "never",
		"--network", plane.handoff.Network, "--name", plane.containerName(endpoint), "--hostname", endpoint.Name,
		"--label", "org.zpr.evaluation-install=" + plane.handoff.InstallID,
		"--label", simulatorDataPlaneEndpointLabel + "=" + endpoint.Name,
		"-e", "ZPR_EP_NAME=" + endpoint.Name, "-e", "ZPR_EP_KEY=" + keyPath, "-e", "ZPR_EP_CA=" + caPath,
		"-e", "ZPR_EP_NODE=" + plane.nodeContainer(endpoint), "-e", "ZPR_EP_ZPR_ADDR=" + endpoint.ZPRAddress,
		"--mount", "type=bind,source=" + filepath.Join(plane.dir, endpoint.BootstrapKey) + ",target=" + keyPath + ",readonly",
		"--mount", "type=bind,source=" + filepath.Join(plane.dir, plane.handoff.CAFile) + ",target=" + caPath + ",readonly",
		"--entrypoint", "/usr/local/bin/zpr-evaluation-endpoint", plane.image}
}

// reconcile adopts running endpoint containers and starts missing ones while
// the install's data plane is up; it never touches the install itself.
func (plane *simulatorDataPlaneEndpoints) reconcile(ctx context.Context) {
	_, networkErr := plane.docker(ctx, "network", "inspect", plane.handoff.Network)
	status := map[string]simulatorDataPlaneEndpointStatus{}
	for _, endpoint := range plane.handoff.Endpoints {
		current := simulatorDataPlaneEndpointStatus{Name: endpoint.Name, ZPRAddress: endpoint.ZPRAddress, Container: plane.containerName(endpoint)}
		if networkErr != nil {
			current.Error = "install data plane is not running"
			status[endpoint.Name] = current
			continue
		}
		running, err := plane.docker(ctx, "inspect", "--format", "{{.State.Running}}", current.Container)
		if err == nil && running == "true" {
			current.Running = true
			status[endpoint.Name] = current
			continue
		}
		_, _ = plane.docker(ctx, "rm", "-f", current.Container)
		if output, err := plane.docker(ctx, plane.runArguments(endpoint)...); err != nil {
			current.Error = "endpoint container failed to start"
			log.Printf("data-plane endpoint %s failed to start: %v: %s", endpoint.Name, err, output)
		} else {
			current.Running = true
		}
		status[endpoint.Name] = current
	}
	plane.mu.Lock()
	plane.status, plane.planeUp, plane.checkedAt = status, networkErr == nil, time.Now().UTC()
	plane.mu.Unlock()
}

func (plane *simulatorDataPlaneEndpoints) run(ctx context.Context) {
	ticker := time.NewTicker(simulatorDataPlaneReconcileEvery)
	defer ticker.Stop()
	for {
		reconcileContext, cancel := context.WithTimeout(ctx, time.Minute)
		plane.reconcile(reconcileContext)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type simulatorDataPlaneEndpointsResponse struct {
	Configured     bool                               `json:"configured"`
	EvaluationOnly bool                               `json:"evaluation_only,omitempty"`
	InstallID      string                             `json:"install_id,omitempty"`
	Network        string                             `json:"network,omitempty"`
	DataPlaneUp    bool                               `json:"data_plane_up"`
	CheckedAt      string                             `json:"checked_at,omitempty"`
	Endpoints      []simulatorDataPlaneEndpointStatus `json:"endpoints"`
}

func (plane *simulatorDataPlaneEndpoints) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	response := simulatorDataPlaneEndpointsResponse{Endpoints: []simulatorDataPlaneEndpointStatus{}}
	if plane != nil {
		plane.mu.Lock()
		response.Configured, response.EvaluationOnly = true, true
		response.InstallID, response.Network, response.DataPlaneUp = plane.handoff.InstallID, plane.handoff.Network, plane.planeUp
		if !plane.checkedAt.IsZero() {
			response.CheckedAt = plane.checkedAt.Format(time.RFC3339)
		}
		for _, endpoint := range plane.handoff.Endpoints {
			current, ok := plane.status[endpoint.Name]
			if !ok {
				current = simulatorDataPlaneEndpointStatus{Name: endpoint.Name, ZPRAddress: endpoint.ZPRAddress, Container: plane.containerName(endpoint), Error: "not yet checked"}
			}
			response.Endpoints = append(response.Endpoints, current)
		}
		plane.mu.Unlock()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(response)
}
