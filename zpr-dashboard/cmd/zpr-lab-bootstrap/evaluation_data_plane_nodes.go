package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// EVALUATION ONLY multi-node data plane (D6b). The docking node "node" stays
// in the data-plane container with the Visa Service; each additional node runs
// in its own container on the install's Docker network at a fixed address in
// a per-install /24, and every pair of nodes is linked (full mesh).

const evaluationDataPlaneMaxNodes = 7

var evaluationDataPlaneNodeName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}[a-z0-9]$`)

type evaluationDataPlaneNode struct {
	Name     string                 `json:"name"`
	Location evaluationNodeLocation `json:"location"`
}

func parseEvaluationDataPlaneNode(value string) (evaluationDataPlaneNode, error) {
	name, location, ok := strings.Cut(value, "@")
	if !ok {
		return evaluationDataPlaneNode{}, errors.New("-data-plane-node must be NAME@LATITUDE,LONGITUDE")
	}
	parsed, err := parseEvaluationNodeLocation(location)
	if err != nil {
		return evaluationDataPlaneNode{}, errors.New("-data-plane-node location must be finite LATITUDE,LONGITUDE within [-90,90],[-180,180]")
	}
	return evaluationDataPlaneNode{Name: name, Location: *parsed}, nil
}

func validateEvaluationDataPlaneNodes(config evaluationConfig) error {
	if len(config.DataPlaneNodes) == 0 {
		return nil
	}
	if !config.DataPlane {
		return errors.New("additional data-plane nodes require -data-plane")
	}
	if len(config.DataPlaneNodes) > evaluationDataPlaneMaxNodes {
		return fmt.Errorf("at most %d additional data-plane nodes", evaluationDataPlaneMaxNodes)
	}
	seen := map[string]bool{}
	for _, node := range config.DataPlaneNodes {
		// "node" is the docking node; endpoint-NN names belong to Simulator endpoints.
		if !evaluationDataPlaneNodeName.MatchString(node.Name) || node.Name == "node" || strings.HasPrefix(node.Name, "endpoint-") || seen[node.Name] {
			return fmt.Errorf("data-plane node name %q must be a distinct lowercase DNS label other than node and endpoint-*", node.Name)
		}
		if _, err := parseEvaluationNodeLocation(node.Location.String()); err != nil {
			return err
		}
		seen[node.Name] = true
	}
	return nil
}

type evaluationDataPlaneTopologyNode struct {
	Name      string
	Substrate string // IPv4 on the install's data-plane network (multi-node only)
	ZPR       string
}

// evaluationDataPlaneSubnetPrefix derives the install's fixed /24 (first three
// octets) from its install ID, so compiled policy survives network recreation.
func evaluationDataPlaneSubnetPrefix(installID string) string {
	sum := sha256.Sum256([]byte("zpr-evaluation-data-plane-subnet:" + installID))
	return fmt.Sprintf("10.%d.%d", 200+int(sum[0])%50, sum[1])
}

// evaluationDataPlaneTopology lists the docking node first. A single-node
// install keeps the D1 veth substrate and no Docker address.
func evaluationDataPlaneTopology(inventory evaluationInventory) []evaluationDataPlaneTopologyNode {
	nodes := []evaluationDataPlaneTopologyNode{{Name: "node", ZPR: "fd5a:5052::2"}}
	if len(inventory.Config.DataPlaneNodes) == 0 {
		return nodes
	}
	prefix := evaluationDataPlaneSubnetPrefix(inventory.InstallID)
	nodes[0].Substrate = prefix + ".2"
	for index, node := range inventory.Config.DataPlaneNodes {
		nodes = append(nodes, evaluationDataPlaneTopologyNode{
			Name: node.Name, Substrate: fmt.Sprintf("%s.%d", prefix, 3+index), ZPR: fmt.Sprintf("fd5a:5052::%x", 3+index),
		})
	}
	return nodes
}

// evaluationDataPlaneNodeFile names node index's files: index 0 keeps the D1
// dp-node* names; additional nodes use dp-node<index>* so names never collide.
func evaluationDataPlaneNodeFile(index int, suffix string) string {
	if index == 0 {
		return "dp-node" + suffix
	}
	return fmt.Sprintf("dp-node%d%s", index, suffix)
}

func evaluationDataPlaneLinkID(a, b int) string {
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("l%dl%d", a, b)
}

func evaluationDataPlaneNodeContainer(inventory evaluationInventory, index int) string {
	if index == 0 {
		return evaluationDataPlaneName(inventory)
	}
	return evaluationDataPlaneName(inventory) + "-node-" + inventory.Config.DataPlaneNodes[index-1].Name
}

// evaluationDataPlaneNodeHost is additional node index's alias on the install
// network. Endpoints resolve it instead of the container name, which can
// exceed the 63-byte DNS label limit.
func evaluationDataPlaneNodeHost(inventory evaluationInventory, index int) string {
	return "node-" + inventory.Config.DataPlaneNodes[index-1].Name
}

// renderEvaluationDataPlaneTopology adds the additional nodes, their links and
// bootstrap keys to the D1 compiler configuration.
func renderEvaluationDataPlaneTopology(compiler string, nodes []evaluationDataPlaneTopologyNode) string {
	if len(nodes) < 2 {
		return compiler
	}
	var hosts, sections, keys strings.Builder
	for index, node := range nodes {
		fmt.Fprintf(&hosts, "\"n%d.zpr\" = %q\n\"n%d.overlay\" = %q\n", index, node.ZPR, index, node.Substrate)
		if index == 0 {
			continue
		}
		fmt.Fprintf(&sections, "\n[nodes.%q]\nprovider = [ [ \"device.zpr.adapter.cn\", %q ] ]\nzpr_address = \"n%d.zpr\"\n\n[nodes.%q.substrate_addrs]\nin1 = \"n%d.overlay:5000\"\n",
			node.Name, node.Name, index, node.Name, index)
		fmt.Fprintf(&keys, "%q = %q\n", node.Name, evaluationDataPlaneNodeFile(index, "-auth.pub"))
	}
	for a := range nodes {
		for b := a + 1; b < len(nodes); b++ {
			fmt.Fprintf(&sections, "\n[links.%s]\npeers = [ { node = %q }, { node = %q } ]\n", evaluationDataPlaneLinkID(a, b), nodes[a].Name, nodes[b].Name)
		}
	}
	const hostsAnchor, nodeAnchor, keyAnchor = "\"n0.zpr\" = \"fd5a:5052::2\"\n\"n0.overlay\" = \"10.0.0.1\"\n", "in1 = \"n0.overlay:5000\"\n", "\"node\" = \"dp-node-auth.pub\"\n"
	compiler = strings.Replace(compiler, hostsAnchor, hosts.String(), 1)
	compiler = strings.Replace(compiler, nodeAnchor, nodeAnchor+sections.String(), 1)
	return strings.Replace(compiler, keyAnchor, keyAnchor+keys.String(), 1)
}

// renderEvaluationDataPlaneNodeConfig is node index's ph node configuration:
// its advertised Docker substrate and every other node as a Noise-pinned peer.
// Paths are absolute because state files are mounted at their host paths.
func renderEvaluationDataPlaneNodeConfig(state string, nodes []evaluationDataPlaneTopologyNode, index int) []byte {
	quoted := func(name string) string {
		data, _ := json.Marshal(filepath.Join(state, name))
		return string(data)
	}
	var config strings.Builder
	// Node links take their source from self_addr, which must not be a wildcard.
	fmt.Fprintf(&config, "# EVALUATION ONLY node %q of a multi-node data plane.\n[global]\nself_addr = \"%[2]s:5000\"\n\n[node]\nadvertised_substrate_addr = \"%[2]s:5000\"\nlocal_vs_dock = %[3]t\n\n[node.peer_noise_certificates]\n",
		nodes[index].Name, nodes[index].Substrate, index == 0)
	for peer, node := range nodes {
		if peer != index {
			fmt.Fprintf(&config, "%q = %s\n", node.ZPR, quoted(evaluationDataPlaneNodeFile(peer, ".crt")))
		}
	}
	for peer, node := range nodes {
		if peer != index {
			fmt.Fprintf(&config, "\n[[node.peers]]\nzpr_addr = %q\nsubstrate_addr = \"%s:5000\"\nlink_id = %q\n", node.ZPR, node.Substrate, evaluationDataPlaneLinkID(index, peer))
		}
	}
	return []byte(config.String())
}

// evaluationDataPlaneNodeFiles lists the protected files of the additional
// nodes and the docking node's peer configuration.
func evaluationDataPlaneNodeFiles(config evaluationConfig) []string {
	if len(config.DataPlaneNodes) == 0 {
		return nil
	}
	files := []string{evaluationDataPlaneNodeFile(0, ".toml")}
	for index := 1; index <= len(config.DataPlaneNodes); index++ {
		for _, suffix := range []string{".crt", ".key", "-auth.key", "-auth.pub", ".toml"} {
			files = append(files, evaluationDataPlaneNodeFile(index, suffix))
		}
	}
	return files
}

// evaluationDataPlaneNodeArguments runs additional node index with only its
// own identity, the CA and its peers' certificates mounted read-only.
func evaluationDataPlaneNodeArguments(state string, inventory evaluationInventory, image string, index int) []string {
	nodes := evaluationDataPlaneTopology(inventory)
	node := nodes[index]
	args := []string{"run", "--rm", "--name", evaluationDataPlaneNodeContainer(inventory, index), "--hostname", node.Name,
		"--network", evaluationDataPlaneName(inventory), "--network-alias", evaluationDataPlaneNodeHost(inventory, index), "--ip", node.Substrate,
		"--privileged", "--init", "--pull", "never",
		"--label", "org.zpr.evaluation-install=" + inventory.InstallID, "--label", "org.zpr.evaluation-node=" + node.Name,
		"-e", "ZPR_NODE_STATE=" + state, "-e", fmt.Sprintf("ZPR_NODE_INDEX=%d", index), "-e", "ZPR_NODE_ZPR_ADDR=" + node.ZPR,
		"--entrypoint", "/usr/local/bin/zpr-evaluation-node"}
	files := []string{"dp-ca.crt"}
	for _, suffix := range []string{".key", "-auth.key", ".toml"} {
		files = append(files, evaluationDataPlaneNodeFile(index, suffix))
	}
	for peer := range nodes {
		files = append(files, evaluationDataPlaneNodeFile(peer, ".crt"))
	}
	for _, name := range files {
		path := filepath.Join(state, name)
		args = append(args, "--mount", "type=bind,source="+path+",target="+path+",readonly")
	}
	return append(args, image)
}

// prepareEvaluationDataPlaneNetwork gives a multi-node install its fixed
// subnet (dynamic endpoint addresses stay in the upper half) and removes
// stale node containers; a stale network with another subnet is replaced.
func prepareEvaluationDataPlaneNetwork(ctx context.Context, inventory evaluationInventory) error {
	name := evaluationDataPlaneName(inventory)
	for index := 1; index <= len(inventory.Config.DataPlaneNodes); index++ {
		_ = evaluationDockerCommand(ctx, "rm", "-f", evaluationDataPlaneNodeContainer(inventory, index))
	}
	prefix := evaluationDataPlaneSubnetPrefix(inventory.InstallID)
	inspect := exec.CommandContext(ctx, "docker", "network", "inspect", "-f", "{{range .IPAM.Config}}{{.Subnet}} {{end}}", name)
	inspect.Env = evaluationDockerEnvironment()
	if output, err := inspect.Output(); err == nil {
		if strings.TrimSpace(string(output)) == prefix+".0/24" {
			return nil
		}
		if err := removeEvaluationDataPlane(ctx, inventory); err != nil {
			return errors.New("cannot replace the stale data-plane Docker network")
		}
	}
	if err := evaluationDockerCommand(ctx, "network", "create", "--label", "org.zpr.evaluation-install="+inventory.InstallID,
		"--subnet", prefix+".0/24", "--gateway", prefix+".1", "--ip-range", prefix+".128/25", name); err != nil {
		return fmt.Errorf("cannot create the data-plane Docker network %s.0/24; check for an overlapping Docker network", prefix)
	}
	return nil
}
