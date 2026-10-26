//go:build !production

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// EVALUATION ONLY (D5): each Simulator organization is backed by its own,
// independently started evaluation install. The Simulator only selects among
// registered installs; it never starts, stops or reconfigures them.
const simulatorEvaluationInstallsEnv = "SIMULATOR_EVALUATION_INSTALLS_FILE"

type simulatorEvaluationInstall struct {
	OrganizationID         string `json:"organization_id"`
	ControlRoomURL         string `json:"control_room_url,omitempty"`
	OperatorServiceURL     string `json:"operator_service_url"`
	ClientCertFile         string `json:"client_cert_file"`
	ClientKeyFile          string `json:"client_key_file"`
	ServiceCAFile          string `json:"service_ca_file"`
	DataPlaneEndpointsFile string `json:"data_plane_endpoints_file,omitempty"`
}

type simulatorEvaluationInstallsFile struct {
	Version        int                          `json:"version"`
	Kind           string                       `json:"kind"`
	EvaluationOnly bool                         `json:"evaluation_only"`
	Installs       []simulatorEvaluationInstall `json:"installs"`
}

type simulatorEvaluationInstalls struct {
	installs  map[string]simulatorEvaluationInstall
	endpoints map[string]*simulatorDataPlaneEndpoints
	workloads map[string]*simulatorDataPlaneWorkloads
	// selectOrganization reports the Simulator's selected organization.
	selectOrganization func() (string, error)
}

// simulatorEvaluationInstallRegistry is set once at Simulator startup.
var simulatorEvaluationInstallRegistry *simulatorEvaluationInstalls

type simulatorOperatorTarget struct {
	ServiceURL, CertFile, KeyFile, CAFile string
}

func exactHTTPSOrigin(value string) bool {
	origin, err := url.Parse(value)
	return err == nil && origin.Scheme == "https" && origin.Hostname() != "" && origin.User == nil && origin.Path == "" &&
		origin.RawQuery == "" && !origin.ForceQuery && origin.Fragment == "" && origin.Opaque == "" && value == "https://"+origin.Host
}

func loadSimulatorEvaluationInstalls(path, organizationsDirectory string) (*simulatorEvaluationInstalls, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read evaluation install registry: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var registry simulatorEvaluationInstallsFile
	if err := decoder.Decode(&registry); err != nil {
		return nil, fmt.Errorf("parse evaluation install registry: %w", err)
	}
	if registry.Version != 1 || registry.Kind != "zpr-simulator-evaluation-installs" || !registry.EvaluationOnly {
		return nil, errors.New("not an EVALUATION ONLY Simulator install registry")
	}
	if len(registry.Installs) == 0 {
		return nil, errors.New("evaluation install registry lists no installs")
	}
	result := &simulatorEvaluationInstalls{installs: map[string]simulatorEvaluationInstall{}, endpoints: map[string]*simulatorDataPlaneEndpoints{}, workloads: map[string]*simulatorDataPlaneWorkloads{}, selectOrganization: simulatorSelectedOrganizationID}
	networks := map[string]string{}
	for _, install := range registry.Installs {
		id := install.OrganizationID
		if _, err := loadSimulatorOrganization(organizationsDirectory, id); err != nil {
			return nil, fmt.Errorf("evaluation install for unknown organization %q", id)
		}
		if _, exists := result.installs[id]; exists {
			return nil, fmt.Errorf("organization %q has more than one evaluation install", id)
		}
		if !exactHTTPSOrigin(install.OperatorServiceURL) {
			return nil, fmt.Errorf("organization %q operator service URL must be an exact HTTPS origin", id)
		}
		if install.ControlRoomURL != "" {
			if _, err := simulatorControlRoomURL(install.ControlRoomURL); err != nil {
				return nil, fmt.Errorf("organization %q Control Room URL: %w", id, err)
			}
		}
		seen := map[string]bool{}
		for _, name := range []string{install.ClientCertFile, install.ClientKeyFile, install.ServiceCAFile} {
			if !filepath.IsAbs(name) || filepath.Clean(name) != name || seen[name] {
				return nil, fmt.Errorf("organization %q TLS files require distinct canonical absolute paths", id)
			}
			seen[name] = true
		}
		if install.DataPlaneEndpointsFile != "" {
			if !filepath.IsAbs(install.DataPlaneEndpointsFile) {
				return nil, fmt.Errorf("organization %q data-plane endpoint handoff must be an absolute path", id)
			}
			endpoints, err := newSimulatorDataPlaneEndpoints(install.DataPlaneEndpointsFile)
			if err != nil {
				return nil, fmt.Errorf("organization %q: %w", id, err)
			}
			if other, exists := networks[endpoints.handoff.Network]; exists {
				return nil, fmt.Errorf("organizations %q and %q share one data plane", other, id)
			}
			networks[endpoints.handoff.Network] = id
			workloads, err := newSimulatorDataPlaneWorkloads(endpoints)
			if err != nil {
				return nil, err
			}
			result.endpoints[id], result.workloads[id] = endpoints, workloads
		}
		result.installs[id] = install
	}
	return result, nil
}

// newSimulatorEvaluationInstallsFromEnv rejects mixing the registry with the
// single-install settings it replaces.
func newSimulatorEvaluationInstallsFromEnv() (*simulatorEvaluationInstalls, error) {
	path := strings.TrimSpace(os.Getenv(simulatorEvaluationInstallsEnv))
	if path == "" {
		return nil, nil
	}
	for _, name := range []string{"SIMULATOR_OPERATOR_SERVICE_URL", simulatorDataPlaneEndpointsEnv, "SIMULATION_ORGANIZATION_RESET_SCRIPT", "ZPR_SIMULATOR_CONTROL_ROOM_URL"} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			return nil, fmt.Errorf("%s replaces %s; unset it", simulatorEvaluationInstallsEnv, name)
		}
	}
	return loadSimulatorEvaluationInstalls(path, simulatorOrganizationsDirectory())
}

// run keeps every registered install's endpoints and workloads running, not
// only the selected organization's, so all installs stay exercised.
func (registry *simulatorEvaluationInstalls) run(ctx context.Context) {
	for id, endpoints := range registry.endpoints {
		go endpoints.run(ctx)
		go registry.workloads[id].run(ctx)
	}
}

func (registry *simulatorEvaluationInstalls) target(organizationID string) (simulatorOperatorTarget, error) {
	install, ok := registry.installs[organizationID]
	if !ok {
		return simulatorOperatorTarget{}, fmt.Errorf("organization %q has no evaluation install; start one and register it in %s", organizationID, simulatorEvaluationInstallsEnv)
	}
	return simulatorOperatorTarget{ServiceURL: install.OperatorServiceURL, CertFile: install.ClientCertFile, KeyFile: install.ClientKeyFile, CAFile: install.ServiceCAFile}, nil
}

func simulatorSelectedOrganizationID() (string, error) {
	manifest, err := readSimulatorManifest()
	if err != nil {
		return "", err
	}
	return activeSimulatorOrganizationID(manifest), nil
}

// activate selects an organization after confirming its independently started
// install answers; it never changes the install.
func (registry *simulatorEvaluationInstalls) activate(ctx context.Context, organizationID, operation string) error {
	if operation == "restore-base" {
		return errors.New("restore base is not available for evaluation installs; reset or recreate the organization's install independently")
	}
	target, err := registry.target(organizationID)
	if err != nil {
		return err
	}
	if _, err := readOperatorSnapshot(ctx, target); err != nil {
		return fmt.Errorf("organization %q evaluation install is not reachable; start it first", organizationID)
	}
	return nil
}

func (registry *simulatorEvaluationInstalls) selected() (string, bool) {
	id, err := registry.selectOrganization()
	if err != nil {
		return "", false
	}
	_, ok := registry.installs[id]
	return id, ok
}

func (registry *simulatorEvaluationInstalls) endpointsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := registry.selected()
		registry.endpoints[id].ServeHTTP(w, r)
	})
}

func (registry *simulatorEvaluationInstalls) workloadsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := registry.selected()
		registry.workloads[id].ServeHTTP(w, r)
	})
}

type simulatorEvaluationInstallStatus struct {
	OrganizationID     string `json:"organization_id"`
	Selected           bool   `json:"selected"`
	ControlRoomURL     string `json:"control_room_url,omitempty"`
	OperatorServiceURL string `json:"operator_service_url"`
	DataPlaneEndpoints bool   `json:"data_plane_endpoints"`
}

func (registry *simulatorEvaluationInstalls) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	response := struct {
		Configured     bool                               `json:"configured"`
		EvaluationOnly bool                               `json:"evaluation_only,omitempty"`
		Installs       []simulatorEvaluationInstallStatus `json:"installs"`
	}{Installs: []simulatorEvaluationInstallStatus{}}
	if registry != nil {
		response.Configured, response.EvaluationOnly = true, true
		selected, _ := registry.selected()
		for id, install := range registry.installs {
			response.Installs = append(response.Installs, simulatorEvaluationInstallStatus{
				OrganizationID: id, Selected: id == selected, ControlRoomURL: install.ControlRoomURL,
				OperatorServiceURL: install.OperatorServiceURL, DataPlaneEndpoints: registry.endpoints[id] != nil,
			})
		}
		sort.Slice(response.Installs, func(i, j int) bool { return response.Installs[i].OrganizationID < response.Installs[j].OrganizationID })
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(response)
}
