package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

// loadEvaluationSimulatorRetainedBindings reads an explicit operator-owned
// retained machine-runtime binding for the paired Simulator launch. The file
// must contain exactly the retained topology binding and retained machine
// runtime for this installation; the Simulator revalidates both authoritative
// contracts at startup and the retained lifecycle verifies the referenced
// files and containers before any use. The launcher never fabricates,
// migrates or widens a binding.
func loadEvaluationSimulatorRetainedBindings(path string, inventory evaluationInventory, catalogNodeID string) (json.RawMessage, json.RawMessage, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, ",\"\r\n=") {
		return nil, nil, errors.New("retained-machine-runtime-file requires a canonical absolute bind-mount-safe path")
	}
	data, err := servicehttp.ReadOperatorFile(path, 65536, true)
	if err != nil {
		return nil, nil, fmt.Errorf("read protected retained machine-runtime bindings: %w", err)
	}
	var document struct {
		Binding json.RawMessage `json:"retained_topology_binding"`
		Runtime json.RawMessage `json:"retained_machine_runtime"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil || decoder.Decode(new(any)) != io.EOF ||
		len(document.Binding) == 0 || len(document.Runtime) == 0 {
		return nil, nil, errors.New("retained machine-runtime bindings require one JSON object with exactly retained_topology_binding and retained_machine_runtime")
	}
	var binding struct {
		Version          int    `json:"version"`
		InstallID        string `json:"install_id"`
		LogicalNodeID    string `json:"logical_node_id"`
		NodeContainer    string `json:"node_container"`
		Network          string `json:"network"`
		ImageID          string `json:"image_id"`
		SubstrateAddress string `json:"substrate_address"`
		ZPRAddress       string `json:"zpr_address"`
	}
	strict := json.NewDecoder(bytes.NewReader(document.Binding))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&binding); err != nil || strict.Decode(new(any)) != io.EOF {
		return nil, nil, errors.New("retained topology binding must be strict JSON with known fields")
	}
	if binding.Version != 1 || binding.InstallID != inventory.InstallID {
		return nil, nil, errors.New("retained topology binding must be version 1 for exactly this installation")
	}
	expectedContainer := "zpr-evaluation-" + inventory.InstallID + "-data-plane"
	nodes := evaluationDataPlaneTopology(inventory)
	// The published profile's node is authoritative: the launcher's own
	// minimal profile publishes the data-plane node name, and a packaged
	// catalog profile publishes the organization's original node.
	nodeMatches := len(nodes) == 1 && binding.LogicalNodeID == nodes[0].Name ||
		catalogNodeID != "" && binding.LogicalNodeID == catalogNodeID
	if !inventory.Config.DataPlane || len(nodes) != 1 || !nodeMatches ||
		binding.NodeContainer != expectedContainer || binding.Network != expectedContainer {
		return nil, nil, errors.New("retained topology binding requires this install-owned single-node data plane and its published node")
	}
	var runtime struct {
		Version          int    `json:"version"`
		ImageID          string `json:"image_id"`
		ControllerFile   string `json:"controller_file"`
		ControllerSHA256 string `json:"controller_sha256"`
		EndpointRoot     string `json:"endpoint_root"`
		DataPlaneCAFile  string `json:"data_plane_ca_file"`
		ControlTLSDir    string `json:"control_tls_dir"`
		CallbackUpstream string `json:"callback_upstream"`
	}
	strict = json.NewDecoder(bytes.NewReader(document.Runtime))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&runtime); err != nil || strict.Decode(new(any)) != io.EOF {
		return nil, nil, errors.New("retained machine runtime must be strict JSON with known fields")
	}
	if runtime.Version != 1 || runtime.CallbackUpstream == "" {
		return nil, nil, errors.New("retained machine runtime must be version 1 with an explicit callback upstream")
	}
	for _, reference := range []string{runtime.ControllerFile, runtime.EndpointRoot, runtime.DataPlaneCAFile, runtime.ControlTLSDir} {
		if !filepath.IsAbs(reference) || filepath.Clean(reference) != reference || strings.ContainsAny(reference, ",:\x00\r\n") {
			return nil, nil, errors.New("retained machine runtime requires canonical absolute bind paths without commas, colons or line breaks")
		}
	}
	return document.Binding, document.Runtime, nil
}
