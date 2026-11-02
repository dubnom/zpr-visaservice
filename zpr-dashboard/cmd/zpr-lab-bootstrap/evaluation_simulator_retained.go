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

// evaluationSimulatorRetainedControl carries the operator-declared
// machine-control listener configuration from the retained runtime binding so
// the launcher can start the Simulator's mutually authenticated control
// listener at exactly the upstream the retained callback proxy relays to.
type evaluationSimulatorRetainedControl struct {
	TLSDir string
	Listen string
}

// loadEvaluationSimulatorRetainedBindings reads an explicit operator-owned
// retained machine-runtime binding for the paired Simulator launch. The file
// must contain exactly the retained topology binding and retained machine
// runtime for this installation; the Simulator revalidates both authoritative
// contracts at startup and the retained lifecycle verifies the referenced
// files and containers before any use. The launcher never fabricates,
// migrates or widens a binding.
func loadEvaluationSimulatorRetainedBindings(path string, inventory evaluationInventory, catalogNodeID string) (json.RawMessage, json.RawMessage, evaluationSimulatorRetainedControl, error) {
	none := evaluationSimulatorRetainedControl{}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, ",\"\r\n=") {
		return nil, nil, none, errors.New("retained-machine-runtime-file requires a canonical absolute bind-mount-safe path")
	}
	data, err := servicehttp.ReadOperatorFile(path, 65536, true)
	if err != nil {
		return nil, nil, none, fmt.Errorf("read protected retained machine-runtime bindings: %w", err)
	}
	var document struct {
		Binding json.RawMessage `json:"retained_topology_binding"`
		Runtime json.RawMessage `json:"retained_machine_runtime"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil || decoder.Decode(new(any)) != io.EOF ||
		len(document.Binding) == 0 || len(document.Runtime) == 0 {
		return nil, nil, none, errors.New("retained machine-runtime bindings require one JSON object with exactly retained_topology_binding and retained_machine_runtime")
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
		// Nodes is the multi-node form (G6): the first entry is the docking
		// data-plane node, additional entries are the install's own
		// -node-<name> containers at their deterministic addresses.
		Nodes []struct {
			LogicalNodeID    string `json:"logical_node_id"`
			Container        string `json:"container"`
			SubstrateAddress string `json:"substrate_address"`
			ZPRAddress       string `json:"zpr_address"`
		} `json:"nodes"`
	}
	strict := json.NewDecoder(bytes.NewReader(document.Binding))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&binding); err != nil || strict.Decode(new(any)) != io.EOF {
		return nil, nil, none, errors.New("retained topology binding must be strict JSON with known fields")
	}
	if binding.Version != 1 || binding.InstallID != inventory.InstallID {
		return nil, nil, none, errors.New("retained topology binding must be version 1 for exactly this installation")
	}
	expectedContainer := "zpr-evaluation-" + inventory.InstallID + "-data-plane"
	nodes := evaluationDataPlaneTopology(inventory)
	if len(binding.Nodes) == 0 {
		// The published profile's node is authoritative: the launcher's own
		// minimal profile publishes the data-plane node name, and a packaged
		// catalog profile publishes the organization's original node.
		nodeMatches := len(nodes) == 1 && binding.LogicalNodeID == nodes[0].Name ||
			catalogNodeID != "" && binding.LogicalNodeID == catalogNodeID
		// Single-node installs run on dynamic Docker subnets: the binding
		// pins identity only and must not pin a substrate address.
		if !inventory.Config.DataPlane || len(nodes) != 1 || !nodeMatches ||
			binding.NodeContainer != expectedContainer || binding.Network != expectedContainer ||
			binding.SubstrateAddress != "" || binding.ZPRAddress != nodes[0].ZPR {
			return nil, nil, none, errors.New("retained topology binding requires this install-owned single-node data plane and its published node, pinning identity only (no substrate address)")
		}
	} else {
		// Multi-node form: all or nothing — the scalar fields stay empty and
		// every entry must match this install's own data-plane topology in
		// order, at its deterministic containers and addresses.
		if binding.LogicalNodeID != "" || binding.NodeContainer != "" ||
			binding.SubstrateAddress != "" || binding.ZPRAddress != "" {
			return nil, nil, none, errors.New("retained multi-node topology binding must declare all nodes in nodes with empty single-node fields")
		}
		if !inventory.Config.DataPlane || len(nodes) < 2 || len(binding.Nodes) != len(nodes) ||
			binding.Network != expectedContainer {
			return nil, nil, none, errors.New("retained multi-node topology binding requires this install-owned multi-node data plane, one entry per install node")
		}
		seenNodeIDs := map[string]bool{}
		for index, node := range binding.Nodes {
			dockingMatches := index == 0 && (node.LogicalNodeID == nodes[0].Name ||
				catalogNodeID != "" && node.LogicalNodeID == catalogNodeID)
			additionalMatches := index > 0 && node.LogicalNodeID == nodes[index].Name
			if node.LogicalNodeID == "" || seenNodeIDs[node.LogicalNodeID] ||
				!(dockingMatches || additionalMatches) ||
				node.Container != evaluationDataPlaneNodeContainer(inventory, index) ||
				node.SubstrateAddress != nodes[index].Substrate ||
				node.ZPRAddress != nodes[index].ZPR {
				return nil, nil, none, fmt.Errorf("retained multi-node topology entry %d must bind the install's own node %q at its deterministic container and addresses", index, nodes[index].Name)
			}
			seenNodeIDs[node.LogicalNodeID] = true
		}
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
		// Optional production DNS name controllers resolve for the
		// machine-control callback; the Simulator revalidates the full
		// contract, the launcher only enforces the bounded zone shape.
		ControlDNSName string `json:"control_dns_name"`
		// Optional explicit per-organization machine inventory; the
		// Simulator revalidates the full contract, the launcher only
		// enforces the all-or-nothing shape.
		ControlAsset string `json:"control_asset"`
		Machines     []struct {
			MachineID       string `json:"machine_id"`
			ControllerAsset string `json:"controller_asset"`
			Workload        string `json:"workload"`
			WorkloadAsset   string `json:"workload_asset"`
		} `json:"machines"`
	}
	strict = json.NewDecoder(bytes.NewReader(document.Runtime))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&runtime); err != nil || strict.Decode(new(any)) != io.EOF {
		return nil, nil, none, errors.New("retained machine runtime must be strict JSON with known fields")
	}
	if runtime.Version != 1 || runtime.CallbackUpstream == "" {
		return nil, nil, none, errors.New("retained machine runtime must be version 1 with an explicit callback upstream")
	}
	if name := runtime.ControlDNSName; name != "" &&
		(len(name) > 253 || !strings.HasSuffix(name, ".svc.zpr") || strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789.-") != "") {
		return nil, nil, none, errors.New("retained control DNS name must be a bounded name in the production svc.zpr zone")
	}
	if len(runtime.Machines) == 0 != (runtime.ControlAsset == "") {
		return nil, nil, none, errors.New("an explicit retained machine inventory requires both its control asset and machines")
	}
	for _, machine := range runtime.Machines {
		if machine.MachineID == "" || machine.ControllerAsset == "" || machine.Workload == "" || machine.WorkloadAsset == "" {
			return nil, nil, none, errors.New("every retained machine requires complete machine, controller, workload and asset identities")
		}
	}
	for _, reference := range []string{runtime.ControllerFile, runtime.EndpointRoot, runtime.DataPlaneCAFile, runtime.ControlTLSDir} {
		if !filepath.IsAbs(reference) || filepath.Clean(reference) != reference || strings.ContainsAny(reference, ",:\x00\r\n") {
			return nil, nil, none, errors.New("retained machine runtime requires canonical absolute bind paths without commas, colons or line breaks")
		}
	}
	control := evaluationSimulatorRetainedControl{TLSDir: runtime.ControlTLSDir, Listen: runtime.CallbackUpstream}
	return document.Binding, document.Runtime, control, nil
}
