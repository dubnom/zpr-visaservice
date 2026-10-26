package main

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

func testEvaluationMultiNodeConfig() evaluationConfig {
	config := testEvaluationDataPlaneConfig()
	config.NodeLocation = &evaluationNodeLocation{Latitude: 43.04, Longitude: -87.91}
	config.DataPlaneNodes = []evaluationDataPlaneNode{
		{Name: "shenzhen-office", Location: evaluationNodeLocation{Latitude: 22.54, Longitude: 114.06}},
		{Name: "tijuana", Location: evaluationNodeLocation{Latitude: 32.51, Longitude: -117.04}},
	}
	config.DataPlaneEndpoints = 4
	return config
}

func TestEvaluationDataPlaneMultiNodeRender(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install with space")
	config := testEvaluationMultiNodeConfig()
	inventory, err := prepareEvaluation(state, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readEvaluation(state, config); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dp-node.toml", "dp-node1.crt", "dp-node1.key", "dp-node1-auth.key", "dp-node1-auth.pub", "dp-node1.toml", "dp-node2.toml"} {
		if info, err := os.Stat(filepath.Join(state, name)); err != nil || info.Mode().Perm() != 0o600 || inventory.Files[name] == "" {
			t.Fatalf("%s is not a protected inventory file", name)
		}
	}
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(state, name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	prefix := evaluationDataPlaneSubnetPrefix(inventory.InstallID)
	topology := evaluationDataPlaneTopology(inventory)
	if len(topology) != 3 || topology[0].Substrate != prefix+".2" || topology[2].Substrate != prefix+".4" || topology[2].ZPR != "fd5a:5052::4" {
		t.Fatalf("topology: %+v", topology)
	}

	var compiler struct {
		Resolver struct {
			Hosts map[string]string `toml:"hosts"`
		} `toml:"resolver"`
		Nodes map[string]struct {
			Provider       [][]string        `toml:"provider"`
			ZPRAddress     string            `toml:"zpr_address"`
			SubstrateAddrs map[string]string `toml:"substrate_addrs"`
		} `toml:"nodes"`
		Links map[string]struct {
			Peers []map[string]string `toml:"peers"`
		} `toml:"links"`
		VisaService struct {
			DockNode string `toml:"dock_node"`
		} `toml:"visa_service"`
		Bootstrap map[string]string `toml:"bootstrap"`
	}
	if _, err := toml.Decode(string(read("dp-policy.zplc")), &compiler); err != nil {
		t.Fatalf("compiler configuration is not TOML: %v", err)
	}
	if compiler.Resolver.Hosts["n0.overlay"] != prefix+".2" || compiler.Resolver.Hosts["n2.zpr"] != "fd5a:5052::4" || compiler.VisaService.DockNode != "node" {
		t.Fatalf("resolver hosts: %v", compiler.Resolver.Hosts)
	}
	if node := compiler.Nodes["shenzhen-office"]; len(compiler.Nodes) != 3 || node.ZPRAddress != "n1.zpr" || node.SubstrateAddrs["in1"] != "n1.overlay:5000" ||
		!slices.Equal(node.Provider[0], []string{"device.zpr.adapter.cn", "shenzhen-office"}) {
		t.Fatalf("nodes: %+v", compiler.Nodes)
	}
	if len(compiler.Links) != 3 || compiler.Links["l1l2"].Peers[0]["node"] != "shenzhen-office" || compiler.Links["l0l2"].Peers[1]["node"] != "tijuana" {
		t.Fatalf("links must be a full mesh: %+v", compiler.Links)
	}
	if compiler.Bootstrap["tijuana"] != "dp-node2-auth.pub" || compiler.Bootstrap["node"] != "dp-node-auth.pub" || compiler.Bootstrap["endpoint-04"] != "dp-endpoint-04-auth.pub" {
		t.Fatalf("bootstrap: %v", compiler.Bootstrap)
	}

	type nodeConfig struct {
		Global struct {
			SelfAddr string `toml:"self_addr"`
		} `toml:"global"`
		Node struct {
			Advertised string            `toml:"advertised_substrate_addr"`
			LocalDock  bool              `toml:"local_vs_dock"`
			PeerCerts  map[string]string `toml:"peer_noise_certificates"`
			Peers      []struct {
				ZPRAddr       string `toml:"zpr_addr"`
				SubstrateAddr string `toml:"substrate_addr"`
				LinkID        string `toml:"link_id"`
			} `toml:"peers"`
		} `toml:"node"`
	}
	for index, name := range []string{"dp-node.toml", "dp-node1.toml", "dp-node2.toml"} {
		var node nodeConfig
		if _, err := toml.Decode(string(read(name)), &node); err != nil {
			t.Fatalf("%s is not TOML: %v", name, err)
		}
		if node.Node.Advertised != topology[index].Substrate+":5000" || node.Global.SelfAddr != node.Node.Advertised || node.Node.LocalDock != (index == 0) || len(node.Node.Peers) != 2 || len(node.Node.PeerCerts) != 2 {
			t.Fatalf("%s: %+v", name, node)
		}
		for _, peer := range node.Node.Peers {
			if peer.ZPRAddr == topology[index].ZPR || !strings.HasPrefix(peer.SubstrateAddr, prefix+".") || node.Node.PeerCerts[peer.ZPRAddr] == "" {
				t.Fatalf("%s peer: %+v", name, peer)
			}
		}
		if index == 1 && (node.Node.PeerCerts["fd5a:5052::2"] != filepath.Join(state, "dp-node.crt") || node.Node.Peers[1].LinkID != "l1l2") {
			t.Fatalf("%s peers: %+v", name, node.Node)
		}
	}

	block, _ := pem.Decode(read("dp-node2.crt"))
	if leaf, err := x509.ParseCertificate(block.Bytes); err != nil || leaf.Subject.CommonName != "tijuana" {
		t.Fatal("additional node Noise certificate must carry the node name")
	}
	if bytes.Equal(read("dp-node1-auth.key"), read("dp-node2-auth.key")) || bytes.Equal(read("dp-node1.key"), read("dp-node.key")) {
		t.Fatal("node identities must be unique")
	}
	if vs := string(read("vs.toml")); !strings.Contains(vs, "[nodes.\"node\"]\nlatitude = 43.04\n") || !strings.Contains(vs, "[nodes.\"tijuana\"]\nlatitude = 32.51\nlongitude = -117.04\n") {
		t.Fatalf("vs.toml node locations: %s", vs)
	}

	var handoff evaluationDataPlaneEndpointsHandoff
	if err := json.Unmarshal(read(evaluationDataPlaneEndpointsFile), &handoff); err != nil {
		t.Fatal(err)
	}
	want := []string{"", "node-shenzhen-office", "node-tijuana", ""}
	for index, endpoint := range handoff.Endpoints {
		if endpoint.NodeHost != want[index] {
			t.Fatalf("endpoint %s docks with %q, want %q", endpoint.Name, endpoint.NodeHost, want[index])
		}
	}
	if args := strings.Join(evaluationDataPlaneNodeArguments(state, inventory, "image", 2), " "); !strings.Contains(args, "--network "+evaluationDataPlaneName(inventory)+" --network-alias node-tijuana ") {
		t.Fatalf("additional node must carry its endpoint alias: %s", args)
	}
}

func TestEvaluationDataPlaneMultiNodeMounts(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install")
	inventory := evaluationInventory{InstallID: "0123456789abcdef01234567", Config: testEvaluationMultiNodeConfig()}
	prefix := evaluationDataPlaneSubnetPrefix(inventory.InstallID)
	mounted := func(args []string) []string {
		names := []string{}
		for index, arg := range args {
			if arg == "--mount" {
				fields := strings.Split(args[index+1], ",")
				if len(fields) != 4 || fields[3] != "readonly" {
					if base := filepath.Base(strings.TrimPrefix(fields[1], "source=")); base != "valkey-state" && base != "data-plane" {
						t.Fatalf("unexpected writable mount %s", args[index+1])
					}
				}
				names = append(names, filepath.Base(strings.TrimPrefix(fields[1], "source=")))
			}
		}
		return names
	}
	dock := evaluationDataPlaneArguments(state, inventory, "image", true)
	if !strings.Contains(strings.Join(dock, "\n"), "--ip\n"+prefix+".2\n-e\nZPR_DP_NODE_SUBSTRATE="+prefix+".2\n") {
		t.Fatal("docking node needs its fixed substrate address")
	}
	dockMounts := mounted(dock)
	for _, want := range []string{"dp-node.toml", "dp-node1.crt", "dp-node2-auth.pub"} {
		if !slices.Contains(dockMounts, want) {
			t.Fatalf("docking container missing %s", want)
		}
	}
	for _, name := range dockMounts {
		if strings.HasPrefix(name, "dp-node1") || strings.HasPrefix(name, "dp-node2") {
			if strings.HasSuffix(name, ".key") || strings.HasSuffix(name, ".toml") {
				t.Fatalf("docking container must not see %s", name)
			}
		}
	}
	args := evaluationDataPlaneNodeArguments(state, inventory, "image", 2)
	joined := strings.Join(args, "\n")
	for _, want := range []string{"--name\nzpr-evaluation-0123456789abcdef01234567-data-plane-node-tijuana", "--network\nzpr-evaluation-0123456789abcdef01234567-data-plane",
		"--ip\n" + prefix + ".4", "ZPR_NODE_INDEX=2", "ZPR_NODE_ZPR_ADDR=fd5a:5052::4", "--entrypoint\n/usr/local/bin/zpr-evaluation-node", "--pull\nnever"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("node arguments missing %q", want)
		}
	}
	if args[len(args)-1] != "image" || strings.Contains(joined, "-p\n") {
		t.Fatal("node containers publish nothing and end with the image")
	}
	nodeMounts := mounted(args)
	slices.Sort(nodeMounts)
	if want := []string{"dp-ca.crt", "dp-node.crt", "dp-node1.crt", "dp-node2-auth.key", "dp-node2.crt", "dp-node2.key", "dp-node2.toml"}; !slices.Equal(nodeMounts, want) {
		t.Fatalf("node mounts %v, want %v", nodeMounts, want)
	}
	single := evaluationInventory{InstallID: inventory.InstallID, Config: testEvaluationDataPlaneConfig()}
	if strings.Contains(strings.Join(evaluationDataPlaneArguments(state, single, "image", true), "\n"), "--ip") || len(evaluationDataPlaneNodeFiles(single.Config)) != 0 {
		t.Fatal("single-node installs keep the D1 container and file set")
	}
	if other := evaluationDataPlaneSubnetPrefix("ffffffffffffffffffffffff"); other == prefix || !strings.HasPrefix(prefix, "10.") {
		t.Fatalf("subnet prefixes %s %s", prefix, other)
	}
}

func TestEvaluationDataPlaneNodeValidation(t *testing.T) {
	if node, err := parseEvaluationDataPlaneNode("north-hub@40.8,-124.16"); err != nil || node.Name != "north-hub" || node.Location.Longitude != -124.16 {
		t.Fatalf("parse: %+v %v", node, err)
	}
	for _, value := range []string{"north-hub", "north-hub@91,0", "north-hub@x,y"} {
		if _, err := parseEvaluationDataPlaneNode(value); err == nil {
			t.Fatalf("%s accepted", value)
		}
	}
	for _, name := range []string{"node", "endpoint-01", "Upper", "-x", "x", "a.b", "trailing-", strings.Repeat("a", 33)} {
		config := testEvaluationDataPlaneConfig()
		config.DataPlaneNodes = []evaluationDataPlaneNode{{Name: name}}
		if config.validate() == nil {
			t.Fatalf("node name %q accepted", name)
		}
	}
	config := testEvaluationDataPlaneConfig()
	config.DataPlaneNodes = []evaluationDataPlaneNode{{Name: "a1"}, {Name: "a1"}}
	if config.validate() == nil {
		t.Fatal("duplicate node accepted")
	}
	config.DataPlaneNodes = nil
	for index := 0; index <= evaluationDataPlaneMaxNodes; index++ {
		config.DataPlaneNodes = append(config.DataPlaneNodes, evaluationDataPlaneNode{Name: "n" + string(rune('a'+index))})
	}
	if config.validate() == nil {
		t.Fatal("too many nodes accepted")
	}
	config.DataPlaneNodes = config.DataPlaneNodes[:evaluationDataPlaneMaxNodes]
	if err := config.validate(); err != nil {
		t.Fatalf("%d nodes rejected: %v", evaluationDataPlaneMaxNodes, err)
	}
	plain := testEvaluationVisaConfig()
	plain.DataPlaneNodes = []evaluationDataPlaneNode{{Name: "a1"}}
	if plain.validate() == nil {
		t.Fatal("nodes without a data plane accepted")
	}

	parent := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-evaluation", "-state", filepath.Join(parent, "x"), "-render-only", "-visa-service", "-data-plane-node", "a1@1,2"}, &stdout, &stderr); code != 2 {
		t.Fatalf("-data-plane-node without -data-plane: code %d", code)
	}
	state := filepath.Join(parent, "plane")
	args := []string{"-evaluation", "-state", state, "-render-only", "-visa-service", "-data-plane", "-data-plane-node", "north-hub@40.8,-124.16"}
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("render with a node: code %d: %s", code, stderr.String())
	}
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("same command must restart: %s", stderr.String())
	}
	if code := run([]string{"-evaluation", "-state", state, "-render-only", "-visa-service", "-data-plane"}, &stdout, &stderr); code != 1 {
		t.Fatal("restart without -data-plane-node must not silently change the frozen install")
	}
}

func TestEvaluationDataPlaneAllNodesAdmitted(t *testing.T) {
	body := []byte(`["node","vs.zpr","tijuana"]`)
	if !evaluationDataPlaneActorsAdmitted(body, "node", "tijuana") || evaluationDataPlaneActorsAdmitted(body, "node", "tijuana", "shenzhen-office") {
		t.Fatal("every named node must be admitted")
	}
}
