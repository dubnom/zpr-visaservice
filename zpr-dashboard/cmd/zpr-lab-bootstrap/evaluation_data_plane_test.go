package main

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func testEvaluationDataPlaneConfig() evaluationConfig {
	config := testEvaluationVisaConfig()
	config.DataPlane, config.VSAPIPort, config.ValkeyPort, config.SnapshotDeliveryPort = true, 0, 0, 18793
	return config
}

func TestEvaluationDataPlaneProtectedRender(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install")
	config := testEvaluationDataPlaneConfig()
	inventory, err := prepareEvaluation(state, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readEvaluation(state, config); err != nil {
		t.Fatal(err)
	}
	for _, name := range evaluationVisaRequired(config) {
		info, err := os.Stat(filepath.Join(state, name))
		if err != nil || info.Mode().Perm() != 0o600 || inventory.Files[name] == "" {
			t.Fatalf("%s is not a protected inventory file", name)
		}
	}
	if _, err := os.Stat(filepath.Join(state, "vs-policy.bin2")); err == nil || inventory.Files["vs-policy.bin2"] != "" {
		t.Fatal("data plane must compile its per-install policy, not ship the bundled one")
	}
	if info, err := os.Stat(filepath.Join(state, "data-plane")); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatal("data-plane is not a private directory")
	}
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(state, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	vs := read("vs.toml")
	for _, want := range []string{`admin_addr = "10.0.0.2"`, "admin_port = 8182", "@127.0.0.1:6379\"", "[subject_snapshot_fetch]",
		strings.ReplaceAll(filepath.Join(state, "data-plane", "vs-subject-snapshot"), `\`, `\\`)} {
		if !strings.Contains(vs, want) {
			t.Fatalf("vs.toml missing %q", want)
		}
	}
	if strings.Contains(vs, "vs_addr") || strings.Contains(vs, "vsapi_port") {
		t.Fatal("data-plane VS must keep its fixed ZPR VSAPI address")
	}
	if !strings.Contains(read("valkey.conf"), "port 6379\n") {
		t.Fatal("container Valkey port not rendered")
	}
	if strings.Contains(read("evaluation.json"), "vs_api_port") || strings.Contains(read("evaluation.json"), "valkey_port") || !strings.Contains(read("evaluation.json"), `"data_plane": true`) {
		t.Fatal("data-plane inventory must freeze data_plane without host VSAPI/Valkey ports")
	}
	ports := config.ports()
	if slices.Contains(ports, 0) || !slices.Contains(ports, config.VSAdminPort) || len(config.listenAddresses()) != len(testEvaluationConfig().listenAddresses())+2 {
		t.Fatalf("data-plane ports: %v", ports)
	}
	policy, compiler := read("dp-policy.zpl"), read("dp-policy.zplc")
	for _, name := range []string{"dp-ca.crt", "dp-node-auth.pub", "dp-vs-auth.pub"} {
		if !strings.Contains(compiler, name) {
			t.Fatalf("compiler configuration does not reference %s", name)
		}
	}
	if !strings.Contains(policy, "zpr.adapter.cn:node") || !strings.Contains(policy, "zpr.adapter.cn:'vs.zpr'") || !strings.Contains(compiler, "[nodes.\"node\".substrate_addrs]") {
		t.Fatal("policy does not admit exactly the node and vs.zpr adapter")
	}
}

func TestEvaluationDataPlaneIdentities(t *testing.T) {
	files := map[string][]byte{}
	now := time.Now()
	if err := generateEvaluationDataPlane(files, evaluationInventory{InstallID: "0123456789abcdef01234567", Config: testEvaluationDataPlaneConfig()}, now); err != nil {
		t.Fatal(err)
	}
	for _, name := range evaluationDataPlaneFiles {
		if len(files[name]) == 0 {
			t.Fatalf("%s not generated", name)
		}
	}
	if len(files) != len(evaluationDataPlaneFiles) {
		t.Fatal("unexpected data-plane file (the CA key must not be retained)")
	}
	parse := func(name string) *x509.Certificate {
		block, _ := pem.Decode(files[name])
		if block == nil || block.Type != "CERTIFICATE" {
			t.Fatalf("%s is not a certificate", name)
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		return certificate
	}
	ca := parse("dp-ca.crt")
	if !ca.IsCA || ca.Subject.CommonName != "authority.zpr" || ca.PublicKeyAlgorithm != x509.RSA {
		t.Fatal("ZPR CA is not an RSA authority")
	}
	// zpr-common VISA_SERVICE_DN: SEQUENCE { SET { SEQUENCE { commonName, UTF8String "vs.zpr" } } }.
	visaServiceDN := append([]byte{0x30, 0x11, 0x31, 0x0f, 0x30, 0x0d, 0x06, 0x03, 0x55, 0x04, 0x03, 0x0c, 0x06}, "vs.zpr"...)
	for _, actor := range []string{"node", "vs.zpr"} {
		leaf := parse("dp-" + actor + ".crt")
		if err := leaf.CheckSignatureFrom(ca); err != nil {
			t.Fatalf("%s not issued by the install CA: %v", actor, err)
		}
		if leaf.Subject.CommonName != actor || leaf.KeyUsage != x509.KeyUsageKeyAgreement || leaf.IsCA || leaf.NotAfter.Before(now.AddDate(4, 0, 0)) {
			t.Fatalf("%s certificate attributes", actor)
		}
		block, _ := pem.Decode(files["dp-"+actor+".key"])
		private, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		// crypto/x509 leaves X25519 certificate keys unparsed; decode the SPKI directly.
		spki, err := x509.ParsePKIXPublicKey(leaf.RawSubjectPublicKeyInfo)
		if err != nil {
			t.Fatal(err)
		}
		noise, ok := private.(*ecdh.PrivateKey)
		public, publicOK := spki.(*ecdh.PublicKey)
		if !ok || !publicOK || noise.Curve() != ecdh.X25519() || !noise.PublicKey().Equal(public) {
			t.Fatalf("%s is not an X25519 Noise identity", actor)
		}
		if actor == "vs.zpr" && !bytes.Equal(leaf.RawSubject, visaServiceDN) {
			t.Fatalf("vs.zpr subject %x does not match the well-known VS DN", leaf.RawSubject)
		}
	}
	for _, name := range []string{"node", "vs"} {
		block, _ := pem.Decode(files["dp-"+name+"-auth.key"])
		private, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		publicBlock, _ := pem.Decode(files["dp-"+name+"-auth.pub"])
		public, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)
		rsaPrivate, ok := private.(*rsa.PrivateKey)
		if err != nil || !ok || !rsaPrivate.PublicKey.Equal(public) {
			t.Fatalf("%s bootstrap key pair mismatch", name)
		}
	}
	other := map[string][]byte{}
	if err := generateEvaluationDataPlane(other, evaluationInventory{InstallID: "0123456789abcdef01234567", Config: testEvaluationDataPlaneConfig()}, now); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dp-policy-signing.key", "dp-node.key", "dp-vs.zpr.key", "dp-node-auth.key", "dp-vs-auth.key"} {
		if bytes.Equal(files[name], other[name]) {
			t.Fatalf("%s is not unique per install", name)
		}
	}
}

func TestEvaluationDataPlaneFlags(t *testing.T) {
	parent := t.TempDir()
	var stdout, stderr bytes.Buffer
	for name, args := range map[string][]string{
		"without visa service": {"-data-plane"},
		"host VSAPI port":      {"-visa-service", "-data-plane", "-vs-api-port", "15003"},
		"host Valkey port":     {"-visa-service", "-data-plane", "-valkey-port", "16380"},
		"native vs binary":     {"-visa-service", "-data-plane", "-vs-binary", "/bin/sh"},
		"image without plane":  {"-visa-service", "-data-plane-image", "other:local"},
	} {
		if code := run(append([]string{"-evaluation", "-state", filepath.Join(parent, "x"), "-render-only"}, args...), &stdout, &stderr); code != 2 {
			t.Fatalf("%s: code %d", name, code)
		}
	}
	state := filepath.Join(parent, "plane")
	if code := run([]string{"-evaluation", "-state", state, "-render-only", "-visa-service", "-data-plane", "-data-plane-image", "other:local"}, &stdout, &stderr); code != 0 {
		t.Fatalf("render with -data-plane: code %d: %s", code, stderr.String())
	}
	if _, err := readEvaluation(state, testEvaluationDataPlaneConfig()); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"-evaluation", "-state", state, "-render-only", "-visa-service"}, &stdout, &stderr); code != 1 {
		t.Fatal("restart without -data-plane must not silently change the frozen install")
	}
	unsafe := filepath.Join(parent, "a,b")
	if code := run([]string{"-evaluation", "-state", unsafe, "-render-only", "-visa-service", "-data-plane"}, &stdout, &stderr); code == 0 {
		t.Fatal("state paths breaking --mount syntax must be rejected")
	}
	invalid := testEvaluationDataPlaneConfig()
	invalid.VisaService, invalid.VSAdminPort, invalid.SnapshotDeliveryPort = false, 0, 0
	if invalid.validate() == nil {
		t.Fatal("frozen data plane without Visa Service accepted")
	}
	invalid = testEvaluationDataPlaneConfig()
	invalid.VSAPIPort = 15002
	if invalid.validate() == nil {
		t.Fatal("frozen data plane without Visa Service accepted")
	}
}

func TestEvaluationDataPlaneMountsAreMinimal(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install with space")
	inventory := evaluationInventory{InstallID: "0123456789abcdef01234567", Config: testEvaluationDataPlaneConfig()}
	args := evaluationDataPlaneArguments(state, inventory, "zpr-evaluation-data-plane:local", true)
	joined := strings.Join(args, "\n")
	name := "zpr-evaluation-0123456789abcdef01234567-data-plane"
	for _, want := range []string{"--name\n" + name, "--network\n" + name, "--pull\nnever", "-p\n127.0.0.1:18182:8182",
		"-e\nZPR_DP_STATE=" + state, "-e\nZPR_DP_INSTALL_POLICY=1", "-e\nZPR_DP_DELIVERY_PORT=18793"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("data-plane arguments missing %q", want)
		}
	}
	if args[len(args)-1] != "zpr-evaluation-data-plane:local" {
		t.Fatal("image must be the final argument (no command override)")
	}
	writable := []string{}
	for index, arg := range args {
		if arg != "--mount" {
			continue
		}
		spec := args[index+1]
		fields := strings.Split(spec, ",")
		source, target := strings.TrimPrefix(fields[1], "source="), strings.TrimPrefix(fields[2], "target=")
		if source != target || filepath.Dir(source) != state {
			t.Fatalf("mount %q is not a same-path state entry", spec)
		}
		base := filepath.Base(source)
		if strings.HasSuffix(base, ".secret") || base == "ca-key.pem" || base == "client-key.pem" || base == "vs-admin-api.key" || strings.HasPrefix(base, "operator-") {
			t.Fatalf("data plane must not see %s", base)
		}
		if len(fields) != 4 || fields[3] != "readonly" {
			writable = append(writable, base)
		}
	}
	if !slices.Equal(writable, []string{"valkey-state", "data-plane"}) {
		t.Fatalf("writable mounts: %v", writable)
	}
	again := strings.Join(evaluationDataPlaneArguments(state, inventory, "image", false), "\n")
	if strings.Contains(again, "ZPR_DP_INSTALL_POLICY") {
		t.Fatal("per-install policy must be installed only once")
	}
	inventory.Config.SnapshotDeliveryPort = 0
	if strings.Contains(strings.Join(evaluationDataPlaneArguments(state, inventory, "image", false), "\n"), "subject-snapshot-signing.json") {
		t.Fatal("subject-link files mounted without the publication cycle")
	}
	if command := evaluationDataPlaneInitCommand(inventory, state); !strings.HasPrefix(command, "docker exec "+name+" ip netns exec zpr-vs vs --initialize-subject-snapshot -c ") {
		t.Fatalf("init command: %s", command)
	}
}

func TestEvaluationDataPlaneActorsAdmitted(t *testing.T) {
	for body, want := range map[string]bool{
		`[{"cn":"node"},{"cn":"vs.zpr"}]`: true,
		`[{"cn":"vs.zpr"}]`:               false,
		`[{"cn":"node"}]`:                 false,
		`not json`:                        false,
	} {
		if evaluationDataPlaneActorsAdmitted([]byte(body)) != want {
			t.Fatalf("%s: want %v", body, want)
		}
	}
}

func TestEvaluationDataPlaneEndpoints(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install")
	config := testEvaluationDataPlaneConfig()
	config.DataPlaneEndpoints = 3
	inventory, err := prepareEvaluation(state, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readEvaluation(state, config); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dp-endpoint-01-auth.key", "dp-endpoint-03-auth.pub", evaluationDataPlaneEndpointsFile} {
		if info, err := os.Stat(filepath.Join(state, name)); err != nil || info.Mode().Perm() != 0o600 || inventory.Files[name] == "" {
			t.Fatalf("%s is not a protected inventory file", name)
		}
	}
	data, err := os.ReadFile(filepath.Join(state, evaluationDataPlaneEndpointsFile))
	if err != nil {
		t.Fatal(err)
	}
	var handoff evaluationDataPlaneEndpointsHandoff
	if err := json.Unmarshal(data, &handoff); err != nil {
		t.Fatal(err)
	}
	if !handoff.EvaluationOnly || handoff.InstallID != inventory.InstallID || handoff.Network != evaluationDataPlaneName(inventory) ||
		handoff.CAFile != "dp-ca.crt" || len(handoff.Endpoints) != 3 || handoff.Endpoints[2].Name != "endpoint-03" ||
		handoff.Endpoints[2].BootstrapKey != "dp-endpoint-03-auth.key" || handoff.Endpoints[2].ZPRAddress != "fd5a:5052:0:1::3" {
		t.Fatalf("handoff: %s", data)
	}
	policy, err := os.ReadFile(filepath.Join(state, "dp-policy.zpl"))
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := os.ReadFile(filepath.Join(state, "dp-policy.zplc"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(compiler), "[bootstrap]") != 1 || !strings.Contains(string(compiler), `"endpoint-02" = "dp-endpoint-02-auth.pub"`) ||
		!strings.Contains(string(compiler), "[services.Endpoint03Ping]") {
		t.Fatalf("compiler configuration: %s", compiler)
	}
	if !strings.Contains(string(policy), "define Endpoint01 as adapter with zpr.adapter.cn:'endpoint-01'.") ||
		!strings.Contains(string(policy), "service Endpoint02Ping as json {\"service_class\":\"Endpoint02Ping\"}.\n  allow Endpoint01.\n  allow Endpoint03.\n") {
		t.Fatalf("policy: %s", policy)
	}
	readOnly, _ := evaluationDataPlaneMounts(state, config)
	if !slices.Contains(readOnly, filepath.Join(state, "dp-endpoint-03-auth.pub")) {
		t.Fatal("data plane must see endpoint public keys to compile the policy")
	}
	for _, path := range readOnly {
		if strings.HasPrefix(filepath.Base(path), "dp-endpoint-") && strings.HasSuffix(path, ".key") || strings.HasSuffix(path, evaluationDataPlaneEndpointsFile) {
			t.Fatalf("data plane must not see endpoint private keys or the handoff: %s", path)
		}
	}
	plain := testEvaluationDataPlaneConfig()
	if slices.ContainsFunc(evaluationDataPlaneRequired(plain), func(name string) bool { return strings.Contains(name, "endpoint") }) {
		t.Fatal("installs without endpoints must keep the D1 file set")
	}
	if policy, compiler := renderEvaluationDataPlanePolicy(0); policy != evaluationDataPlanePolicy || compiler != evaluationDataPlaneCompilerConfig {
		t.Fatal("zero endpoints must render the D1 policy unchanged")
	}
}

func TestEvaluationDataPlaneEndpointFlags(t *testing.T) {
	parent := t.TempDir()
	var stdout, stderr bytes.Buffer
	for name, args := range map[string][]string{
		"without data plane": {"-visa-service", "-data-plane-endpoints", "3"},
		"without visa":       {"-data-plane-endpoints", "3"},
	} {
		if code := run(append([]string{"-evaluation", "-state", filepath.Join(parent, "x"), "-render-only"}, args...), &stdout, &stderr); code != 2 {
			t.Fatalf("%s: code %d", name, code)
		}
	}
	for _, count := range []int{-1, 1, 17} {
		config := testEvaluationDataPlaneConfig()
		config.DataPlaneEndpoints = count
		if config.validate() == nil {
			t.Fatalf("%d endpoints accepted", count)
		}
	}
	state := filepath.Join(parent, "plane")
	if code := run([]string{"-evaluation", "-state", state, "-render-only", "-visa-service", "-data-plane", "-data-plane-endpoints", "2"}, &stdout, &stderr); code != 0 {
		t.Fatalf("render with endpoints: code %d: %s", code, stderr.String())
	}
	if code := run([]string{"-evaluation", "-state", state, "-render-only", "-visa-service", "-data-plane"}, &stdout, &stderr); code != 1 {
		t.Fatal("restart without -data-plane-endpoints must not silently change the frozen install")
	}
}

func TestEvaluationDataPlaneNodeLocation(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install")
	config := testEvaluationDataPlaneConfig()
	config.NodeLocation = &evaluationNodeLocation{Latitude: 44.98, Longitude: -93}
	if _, err := prepareEvaluation(state, config, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := readEvaluation(state, config); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(state, "vs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\n[nodes.\"node\"]\nlatitude = 44.98\nlongitude = -93.0\n") {
		t.Fatalf("vs.toml: %s", data)
	}
	plain := filepath.Join(t.TempDir(), "plain")
	if _, err := prepareEvaluation(plain, testEvaluationDataPlaneConfig(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(plain, "vs.toml")); strings.Contains(string(data), "[nodes.") {
		t.Fatal("installs without a location must keep vs.toml unchanged")
	}
	for _, value := range []string{"44.98", "91,0", "0,181", "nan,0", "north,0", "1,inf"} {
		if _, err := parseEvaluationNodeLocation(value); err == nil {
			t.Fatalf("%q accepted", value)
		}
	}
	if location, err := parseEvaluationNodeLocation(" -33.9 , 151.2 "); err != nil || location.Latitude != -33.9 || location.Longitude != 151.2 {
		t.Fatalf("%v %v", location, err)
	}
	var stdout, stderr bytes.Buffer
	parent := t.TempDir()
	if code := run([]string{"-evaluation", "-state", filepath.Join(parent, "x"), "-render-only", "-visa-service", "-node-location", "1,2"}, &stdout, &stderr); code != 2 {
		t.Fatalf("without data plane: code %d", code)
	}
	if code := run([]string{"-evaluation", "-state", filepath.Join(parent, "y"), "-render-only", "-visa-service", "-data-plane", "-node-location", "95,2"}, &stdout, &stderr); code != 2 {
		t.Fatalf("invalid location: code %d", code)
	}
	located := filepath.Join(parent, "located")
	if code := run([]string{"-evaluation", "-state", located, "-render-only", "-visa-service", "-data-plane", "-node-location", "1,2"}, &stdout, &stderr); code != 0 {
		t.Fatalf("render with location: code %d: %s", code, stderr.String())
	}
	if code := run([]string{"-evaluation", "-state", located, "-render-only", "-visa-service", "-data-plane", "-node-location", "1,3"}, &stdout, &stderr); code != 1 {
		t.Fatal("a changed location must not silently change the frozen install")
	}
	config = testEvaluationConfig()
	config.NodeLocation = &evaluationNodeLocation{}
	if config.validate() == nil {
		t.Fatal("location without data plane accepted")
	}
}
