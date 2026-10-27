package main

import (
	"context"
	"crypto"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// The EVALUATION ONLY data plane hosts the Visa Service, Valkey, one node and
// the vs.zpr adapter in a Linux container (packaging/evaluation/data-plane), so
// nodes and adapters can reach the VS at its fixed ZPR address. The native
// loopback VS has no ZPR overlay and therefore can never admit a node.
const evaluationDataPlaneImage = "zpr-evaluation-data-plane:local"

// Inside the data plane the VS admin API is relayed on this port and
// published only to host loopback at the configured -vs-admin-port.
const evaluationDataPlaneAdminPort = 8182

var evaluationDataPlaneFiles = []string{
	"dp-policy.zpl", "dp-policy.zplc", "dp-policy-signing.key", "dp-ca.crt",
	"dp-node.crt", "dp-node.key", "dp-node-auth.key", "dp-node-auth.pub",
	"dp-vs.zpr.crt", "dp-vs.zpr.key", "dp-vs-auth.key", "dp-vs-auth.pub",
}

var evaluationProtectedDNSFiles = []string{
	"dp-dns.crt", "dp-dns.key", "dp-dns-auth.key", "dp-dns-auth.pub",
	"dp-dns-publisher.key", "dp-dns-named.conf",
}

const (
	evaluationProtectedDNSAddress     = "fd5a:5052::53"
	evaluationProtectedDNSVisaAddress = "fd5a:5052::1"
	evaluationProtectedDNSBootstrap   = "\"vs.zpr\" = \"dp-vs-auth.pub\"\n"
)

// Per-install policy: the node, the VS adapter and optional evaluation
// endpoints (bootstrap keys, shortcut X15) that may ping one another.
const evaluationDataPlanePolicy = `# EVALUATION ONLY data-plane policy: one node and the Visa Service.
define adapter as a device with zpr.adapter.cn.

define Node as adapter with zpr.adapter.cn:node.
define Vs as adapter with zpr.adapter.cn:'vs.zpr'.

define PingableVs as a service with device.zpr.adapter.cn:'vs.zpr'.
define PingableNode as a service with device.zpr.adapter.cn:node.

service PingableVs as json {"service_class":"PingableVs"}.
  allow Node.

service PingableNode as json {"service_class":"PingableNode"}.
  allow Vs.
`

const evaluationDataPlaneCompilerConfig = `# -*- mode: toml -*-
# EVALUATION ONLY data-plane compiler configuration.

[resolver]
order = ["hosts", "dns"]

[resolver.hosts]
"n0.zpr" = "fd5a:5052::2"
"n0.overlay" = "10.0.0.1"

[nodes."node"]
provider = [ [ "device.zpr.adapter.cn", "node" ] ]
zpr_address = "n0.zpr"

[nodes."node".substrate_addrs]
in1 = "n0.overlay:5000"

[trusted_services.default]
cert_path = "dp-ca.crt"

[visa_service]
dock_node = "node"

[bootstrap]
"node" = "dp-node-auth.pub"
"vs.zpr" = "dp-vs-auth.pub"

[protocols.ping]
l4protocol = "iana.ICMP6"
icmp_type = "request-response"
icmp_codes = [ 128, 129 ]

[services.PingableVs]
protocol = "ping"

[services.PingableNode]
protocol = "ping"
`

const (
	evaluationDataPlaneMinEndpoints = 2
	evaluationDataPlaneMaxEndpoints = 16
	// evaluationDataPlaneEndpointsFile is the install-to-Simulator handoff.
	evaluationDataPlaneEndpointsFile = "data-plane-endpoints.json"
	// evaluationDataPlaneClientFile describes the operator client (U05b) for
	// client.sh; it is deliberately not part of the Simulator handoff.
	evaluationDataPlaneClientFile    = "data-plane-client.env"
	evaluationDataPlaneClientName    = "client-01"
	evaluationDataPlaneClientActor   = "Client01"
	evaluationDataPlaneClientAddress = "fd5a:5052:0:1::c1"
	// Endpoint bootstrap keys (X15) join the existing [bootstrap] table here.
	evaluationDataPlaneBootstrapAnchor = "\"vs.zpr\" = \"dp-vs-auth.pub\"\n"
)

// evaluationDataPlaneListener is a service port every endpoint listens on.
// InPolicy listeners are published to peer endpoints by the install policy;
// the others listen without any policy service, so ZPR must deny them.
type evaluationDataPlaneListener struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	InPolicy bool   `json:"in_policy"`
}

// Endpoint echo listeners (U05): TCP and UDP services the policy allows
// between peers, and one unpublished port of each protocol.
var evaluationDataPlaneListeners = []evaluationDataPlaneListener{
	{Protocol: "tcp", Port: 7007, InPolicy: true},
	{Protocol: "udp", Port: 7008, InPolicy: true},
	{Protocol: "tcp", Port: 7009},
	{Protocol: "udp", Port: 7010},
}

type evaluationDataPlaneEndpoint struct {
	Name         string `json:"name"`
	BootstrapKey string `json:"bootstrap_key"`
	ZPRAddress   string `json:"zpr_address"`
	// NodeHost, when set, is the network alias of the additional node this
	// endpoint docks with; otherwise it docks with the handoff's node_container.
	NodeHost string `json:"node_host,omitempty"`
}

// evaluationDataPlaneEndpointsHandoff is what an install hands to the
// Simulator: file names are relative to the install directory.
type evaluationDataPlaneEndpointsHandoff struct {
	Version        int                           `json:"version"`
	Kind           string                        `json:"kind"`
	EvaluationOnly bool                          `json:"evaluation_only"`
	InstallID      string                        `json:"install_id"`
	Network        string                        `json:"network"`
	NodeContainer  string                        `json:"node_container"`
	CAFile         string                        `json:"ca_file"`
	Endpoints      []evaluationDataPlaneEndpoint `json:"endpoints"`
	Listeners      []evaluationDataPlaneListener `json:"listeners,omitempty"`
}

func evaluationDataPlaneEndpointName(index int) string {
	return fmt.Sprintf("endpoint-%02d", index)
}

func evaluationDataPlaneEndpointPolicyName(index int) string {
	return fmt.Sprintf("Endpoint%02d", index)
}

// evaluationDataPlaneListenerService is the policy service suffix of a
// published listener, e.g. EchoTCP; its compiler protocol is echotcp.
func evaluationDataPlaneListenerService(listener evaluationDataPlaneListener) string {
	return "Echo" + strings.ToUpper(listener.Protocol)
}

func evaluationDataPlaneListenerProtocol(listener evaluationDataPlaneListener) string {
	return strings.ToLower(evaluationDataPlaneListenerService(listener))
}

// evaluationDataPlaneRequired lists the protected data-plane files for config.
func evaluationDataPlaneRequired(config evaluationConfig) []string {
	files := slices.Clone(evaluationDataPlaneFiles)
	if config.ProtectedDNS {
		files = append(files, evaluationProtectedDNSFiles...)
	}
	for index := 1; index <= config.DataPlaneEndpoints; index++ {
		name := evaluationDataPlaneEndpointName(index)
		files = append(files, "dp-"+name+"-auth.key", "dp-"+name+"-auth.pub")
	}
	if config.DataPlaneEndpoints > 0 {
		files = append(files, evaluationDataPlaneEndpointsFile, "dp-"+evaluationDataPlaneClientName+"-auth.key", "dp-"+evaluationDataPlaneClientName+"-auth.pub", evaluationDataPlaneClientFile)
	}
	return append(files, evaluationDataPlaneNodeFiles(config)...)
}

func renderEvaluationDataPlanePolicy(endpoints int, nodes []evaluationDataPlaneTopologyNode, protectedDNS bool) (policy, compiler string) {
	if endpoints == 0 {
		policy, compiler = evaluationDataPlanePolicy, renderEvaluationDataPlaneTopology(evaluationDataPlaneCompilerConfig, nodes)
		return addEvaluationProtectedDNSPolicy(policy, compiler, protectedDNS)
	}
	var zpl, keys, services strings.Builder
	zpl.WriteString(evaluationDataPlanePolicy)
	zpl.WriteString("\n# EVALUATION ONLY endpoints admitted by per-install bootstrap keys (X15).\n")
	keys.WriteString(evaluationDataPlaneBootstrapAnchor)
	kinds, protocols := []string{"Ping"}, map[string]string{"Ping": "ping"}
	for _, listener := range evaluationDataPlaneListeners {
		if listener.InPolicy {
			kind, protocol := evaluationDataPlaneListenerService(listener), evaluationDataPlaneListenerProtocol(listener)
			kinds, protocols[kind] = append(kinds, kind), protocol
			fmt.Fprintf(&services, "\n[protocols.%s]\nl4protocol = \"iana.%s\"\nport = %d\n", protocol, strings.ToUpper(listener.Protocol), listener.Port)
		}
	}
	// The operator client (U05b) is a peer with exactly an endpoint's policy.
	type peer struct{ name, actor string }
	peers := []peer{}
	for index := 1; index <= endpoints; index++ {
		peers = append(peers, peer{evaluationDataPlaneEndpointName(index), evaluationDataPlaneEndpointPolicyName(index)})
	}
	peers = append(peers, peer{evaluationDataPlaneClientName, evaluationDataPlaneClientActor})
	for _, current := range peers {
		fmt.Fprintf(&zpl, "define %s as adapter with zpr.adapter.cn:'%s'.\n", current.actor, current.name)
		fmt.Fprintf(&keys, "%q = %q\n", current.name, "dp-"+current.name+"-auth.pub")
		for _, kind := range kinds {
			fmt.Fprintf(&zpl, "define %s%s as a service with device.zpr.adapter.cn:'%s'.\n", current.actor, kind, current.name)
			fmt.Fprintf(&services, "\n[services.%s%s]\nprotocol = %q\n", current.actor, kind, protocols[kind])
		}
	}
	for _, current := range peers {
		for _, kind := range kinds {
			service := current.actor + kind
			fmt.Fprintf(&zpl, "\nservice %s as json {\"service_class\":\"%s\"}.\n", service, service)
			for _, other := range peers {
				if other != current {
					fmt.Fprintf(&zpl, "  allow %s.\n", other.actor)
				}
			}
		}
	}
	policy, compiler = zpl.String(), strings.Replace(renderEvaluationDataPlaneTopology(evaluationDataPlaneCompilerConfig, nodes), evaluationDataPlaneBootstrapAnchor, keys.String(), 1)+services.String()
	return addEvaluationProtectedDNSPolicy(policy, compiler, protectedDNS)
}

func addEvaluationProtectedDNSPolicy(policy, compiler string, enabled bool) (string, string) {
	if !enabled {
		return policy, compiler
	}
	policy += fmt.Sprintf(`

# EVALUATION ONLY protected authoritative DNS service.
define ZprDNS as a service with device.zpr.adapter.cn:'zpr-dns' and zpr.addr:'%s'.
define AuthenticatedZprDevice as a device with device.zpr.adapter.cn.
define VisaDnsPublisher as an adapter with zpr.adapter.cn:'vs.zpr'.

provide ZprDNS at dns.svc.zpr over TCP 53.
  allow AuthenticatedZprDevice.
  allow VisaDnsPublisher.
`, evaluationProtectedDNSAddress)
	compiler = strings.Replace(compiler, evaluationProtectedDNSBootstrap,
		evaluationProtectedDNSBootstrap+`"zpr-dns" = "dp-dns-auth.pub"
`, 1)
	compiler += `
[protocols.dns_tcp]
l4protocol = "iana.TCP"
port = 53

[services.ZprDNS]
protocol = "dns_tcp"
`
	return policy, compiler
}

func renderEvaluationProtectedDNSConfig() []byte {
	return fmt.Appendf(nil, `acl zpr_clients { fd5a:5052::/32; };

include "/run/zpr/zpr-vs-publisher.key";

options {
    directory "/var/cache/bind";
    recursion no;
    dnssec-validation no;
    allow-recursion { none; };
    allow-query-cache { none; };
    allow-query { zpr_clients; };
    allow-transfer { none; };
    listen-on port 53 { none; };
    listen-on-v6 port 53 { %s; };
};

controls { };

zone "svc.zpr" {
    type primary;
    file "/var/lib/bind/db.svc.zpr";
    allow-transfer { none; };
    update-policy { grant zpr-vs-publisher zonesub A AAAA; };
};
`, evaluationProtectedDNSAddress)
}

// generateEvaluationDataPlane renders unique per-install ZPR identities: an
// RSA policy-signing key, an RSA ZPR CA with X25519 Noise certificates for the
// node and vs.zpr, and RSA bootstrap authentication keys for both and for any
// evaluation endpoints, plus the endpoint handoff for the Simulator.
func generateEvaluationDataPlane(files map[string][]byte, state string, inventory evaluationInventory, now time.Time) error {
	rsaKey := func() (*rsa.PrivateKey, []byte, []byte, error) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, nil, nil, err
		}
		private, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, nil, nil, err
		}
		public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			return nil, nil, nil, err
		}
		return key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public}), nil
	}
	serial := func() (*big.Int, error) {
		return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	}
	_, signing, _, err := rsaKey()
	if err != nil {
		return err
	}
	files["dp-policy-signing.key"] = signing
	caKey, _, _, err := rsaKey()
	if err != nil {
		return err
	}
	caSerial, err := serial()
	if err != nil {
		return err
	}
	notBefore, notAfter := now.Add(-time.Hour), now.AddDate(5, 0, 0)
	caTemplate := &x509.Certificate{
		SerialNumber: caSerial, Subject: pkix.Name{CommonName: "authority.zpr", Organization: []string{"ZPR EVALUATION ONLY"}},
		NotBefore: notBefore, NotAfter: notAfter, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		return err
	}
	files["dp-ca.crt"] = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	nodes := evaluationDataPlaneTopology(inventory)
	actors := []struct{ name, cn string }{{"node", "node"}, {"vs.zpr", "vs.zpr"}}
	if inventory.Config.ProtectedDNS {
		actors = append(actors, struct{ name, cn string }{"dns", "zpr-dns"})
	}
	for index := 1; index < len(nodes); index++ {
		actors = append(actors, struct{ name, cn string }{fmt.Sprintf("node%d", index), nodes[index].Name})
	}
	for _, actor := range actors {
		noise, err := ecdh.X25519().GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		private, err := x509.MarshalPKCS8PrivateKey(noise)
		if err != nil {
			return err
		}
		leafSerial, err := serial()
		if err != nil {
			return err
		}
		der, err := evaluationNoiseCertificate(ca, caKey, noise.PublicKey(), actor.cn, leafSerial, notBefore, notAfter)
		if err != nil {
			return err
		}
		files["dp-"+actor.name+".crt"] = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		files["dp-"+actor.name+".key"] = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})
	}
	authNames := []string{"node", "vs"}
	if inventory.Config.ProtectedDNS {
		authNames = append(authNames, "dns")
	}
	for index := 1; index < len(nodes); index++ {
		authNames = append(authNames, fmt.Sprintf("node%d", index))
	}
	for _, name := range authNames {
		_, private, public, err := rsaKey()
		if err != nil {
			return err
		}
		files["dp-"+name+"-auth.key"], files["dp-"+name+"-auth.pub"] = private, public
	}
	// The CA key is deliberately not retained: endpoints are admitted by bootstrap key.
	endpoints := inventory.Config.DataPlaneEndpoints
	policy, compiler := renderEvaluationDataPlanePolicy(endpoints, nodes, inventory.Config.ProtectedDNS)
	files["dp-policy.zpl"], files["dp-policy.zplc"] = []byte(policy), []byte(compiler)
	if inventory.Config.ProtectedDNS {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return errors.New("cannot generate protected DNS publisher key")
		}
		files["dp-dns-publisher.key"] = fmt.Appendf(nil, "key \"zpr-vs-publisher\" {\n    algorithm hmac-sha256;\n    secret \"%s\";\n};\n", base64.StdEncoding.EncodeToString(secret))
		files["dp-dns-named.conf"] = renderEvaluationProtectedDNSConfig()
	}
	if len(nodes) > 1 {
		for index := range nodes {
			files[evaluationDataPlaneNodeFile(index, ".toml")] = renderEvaluationDataPlaneNodeConfig(state, nodes, index)
		}
	}
	if endpoints == 0 {
		return nil
	}
	handoff := evaluationDataPlaneEndpointsHandoff{
		Version: 1, Kind: "zpr-evaluation-data-plane-endpoints", EvaluationOnly: true,
		InstallID: inventory.InstallID, Network: evaluationDataPlaneName(inventory),
		NodeContainer: evaluationDataPlaneName(inventory), CAFile: "dp-ca.crt",
		Listeners: slices.Clone(evaluationDataPlaneListeners),
	}
	for index := 1; index <= endpoints; index++ {
		name := evaluationDataPlaneEndpointName(index)
		_, private, public, err := rsaKey()
		if err != nil {
			return err
		}
		files["dp-"+name+"-auth.key"], files["dp-"+name+"-auth.pub"] = private, public
		endpoint := evaluationDataPlaneEndpoint{
			Name: name, BootstrapKey: "dp-" + name + "-auth.key", ZPRAddress: fmt.Sprintf("fd5a:5052:0:1::%x", index),
		}
		// Endpoints are spread round-robin over the nodes, starting with the docking node.
		if node := (index - 1) % len(nodes); node > 0 {
			endpoint.NodeHost = evaluationDataPlaneNodeHost(inventory, node)
		}
		handoff.Endpoints = append(handoff.Endpoints, endpoint)
	}
	data, err := json.MarshalIndent(handoff, "", "  ")
	if err != nil {
		return err
	}
	files[evaluationDataPlaneEndpointsFile] = append(data, '\n')
	_, private, public, err := rsaKey()
	if err != nil {
		return err
	}
	client := evaluationDataPlaneClientName
	files["dp-"+client+"-auth.key"], files["dp-"+client+"-auth.pub"] = private, public
	files[evaluationDataPlaneClientFile] = renderEvaluationDataPlaneClient(inventory)
	return nil
}

// renderEvaluationDataPlaneClient describes the operator client for
// packaging/evaluation/data-plane/client.sh as plain KEY=value lines; the
// script parses them and never executes the file.
func renderEvaluationDataPlaneClient(inventory evaluationInventory) []byte {
	listen := make([]string, 0, len(evaluationDataPlaneListeners))
	for _, listener := range evaluationDataPlaneListeners {
		listen = append(listen, fmt.Sprintf("%s:%d", listener.Protocol, listener.Port))
	}
	name := evaluationDataPlaneName(inventory)
	return fmt.Appendf(nil, "# EVALUATION ONLY operator client (U05b); not run by the Simulator.\n"+
		"ZPR_CLIENT_INSTALL_ID=%s\nZPR_CLIENT_NETWORK=%s\nZPR_CLIENT_NODE=%s\nZPR_CLIENT_NAME=%s\n"+
		"ZPR_CLIENT_ZPR_ADDR=%s\nZPR_CLIENT_KEY=dp-%s-auth.key\nZPR_CLIENT_CA=dp-ca.crt\nZPR_CLIENT_LISTEN=%s\n",
		inventory.InstallID, name, name, evaluationDataPlaneClientName, evaluationDataPlaneClientAddress,
		evaluationDataPlaneClientName, strings.Join(listen, " "))
}

// evaluationNoiseCertificate issues an X25519 key-agreement leaf signed by the
// RSA CA. crypto/x509 cannot certify X25519 keys, so the TBS is built directly.
func evaluationNoiseCertificate(ca *x509.Certificate, caKey *rsa.PrivateKey, public *ecdh.PublicKey, commonName string, serial *big.Int, notBefore, notAfter time.Time) ([]byte, error) {
	spki, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return nil, err
	}
	// ZPR matches well-known peers (vs.zpr) by exact DER: a lone UTF8String CN.
	subject, err := asn1.Marshal([]pkix.RelativeDistinguishedNameSET{{{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: asn1.RawValue{Tag: asn1.TagUTF8String, Class: asn1.ClassUniversal, Bytes: []byte(commonName)}}}})
	if err != nil {
		return nil, err
	}
	keyUsage, err := asn1.Marshal(asn1.BitString{Bytes: []byte{0x08}, BitLength: 5})
	if err != nil {
		return nil, err
	}
	basic, err := asn1.Marshal(struct{}{})
	if err != nil {
		return nil, err
	}
	algorithm := pkix.AlgorithmIdentifier{Algorithm: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}, Parameters: asn1.NullRawValue}
	tbs, err := asn1.Marshal(struct {
		Version    int `asn1:"optional,explicit,default:0,tag:0"`
		Serial     *big.Int
		Algorithm  pkix.AlgorithmIdentifier
		Issuer     asn1.RawValue
		Validity   struct{ NotBefore, NotAfter time.Time }
		Subject    asn1.RawValue
		PublicKey  asn1.RawValue
		Extensions []pkix.Extension `asn1:"optional,explicit,tag:3"`
	}{
		Version: 2, Serial: serial, Algorithm: algorithm,
		Issuer:   asn1.RawValue{FullBytes: ca.RawSubject},
		Validity: struct{ NotBefore, NotAfter time.Time }{notBefore.UTC(), notAfter.UTC()},
		Subject:  asn1.RawValue{FullBytes: subject}, PublicKey: asn1.RawValue{FullBytes: spki},
		Extensions: []pkix.Extension{
			{Id: asn1.ObjectIdentifier{2, 5, 29, 15}, Critical: true, Value: keyUsage},
			{Id: asn1.ObjectIdentifier{2, 5, 29, 19}, Critical: true, Value: basic},
		},
	})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(tbs)
	signature, err := rsa.SignPKCS1v15(rand.Reader, caKey, crypto.SHA256, digest[:])
	if err != nil {
		return nil, err
	}
	der, err := asn1.Marshal(struct {
		TBS       asn1.RawValue
		Algorithm pkix.AlgorithmIdentifier
		Signature asn1.BitString
	}{asn1.RawValue{FullBytes: tbs}, algorithm, asn1.BitString{Bytes: signature, BitLength: 8 * len(signature)}})
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	if err := leaf.CheckSignatureFrom(ca); err != nil {
		return nil, err
	}
	return der, nil
}

func evaluationDataPlaneName(inventory evaluationInventory) string {
	return "zpr-evaluation-" + inventory.InstallID + "-data-plane"
}

// evaluationDataPlaneMounts lists the only host paths the privileged data
// plane sees, each at its original absolute path so vs.toml stays valid.
func evaluationDataPlaneMounts(state string, config evaluationConfig) (readOnly, writable []string) {
	readOnly = append([]string{"vs.toml", "valkey.conf", "vs-admin-keys.toml", "server-chain.pem", "server-key.pem", "vs-trusted-services"}, evaluationDataPlaneFiles...)
	if config.ProtectedDNS {
		readOnly = append(readOnly, evaluationProtectedDNSFiles...)
	}
	// Endpoint and operator client public keys only: their private keys stay
	// with the Simulator or the operator.
	for _, name := range evaluationDataPlaneRequired(config) {
		if (strings.HasPrefix(name, "dp-endpoint-") || strings.HasPrefix(name, "dp-"+evaluationDataPlaneClientName+"-")) && strings.HasSuffix(name, ".pub") {
			readOnly = append(readOnly, name)
		}
	}
	// Multi-node: the docking node's peer configuration, peer certificates and
	// the additional nodes' bootstrap public keys (compiled into the policy).
	for _, name := range evaluationDataPlaneNodeFiles(config) {
		if name == "dp-node.toml" || strings.HasSuffix(name, ".crt") || strings.HasSuffix(name, "-auth.pub") {
			readOnly = append(readOnly, name)
		}
	}
	writable = []string{"valkey-state", "data-plane"}
	if config.subjectLinks() {
		readOnly = append(readOnly, "ca.pem", "vs-client-chain.pem", "vs-client-key.pem", "subject-snapshot-signing.json")
	}
	for index, name := range readOnly {
		readOnly[index] = filepath.Join(state, name)
	}
	for index, name := range writable {
		writable[index] = filepath.Join(state, name)
	}
	return readOnly, writable
}

func evaluationDataPlaneArguments(state string, inventory evaluationInventory, image string, installPolicy bool) []string {
	name := evaluationDataPlaneName(inventory)
	args := []string{"run", "--rm", "--name", name, "--hostname", "data-plane", "--network", name,
		"--privileged", "--init", "--pull", "never",
		"--label", "org.zpr.evaluation-install=" + inventory.InstallID,
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", inventory.Config.VSAdminPort, evaluationDataPlaneAdminPort),
		"-e", "ZPR_DP_STATE=" + state}
	if dock := evaluationDataPlaneTopology(inventory)[0]; dock.Substrate != "" {
		args = append(args, "--ip", dock.Substrate, "-e", "ZPR_DP_NODE_SUBSTRATE="+dock.Substrate)
	}
	if installPolicy {
		args = append(args, "-e", "ZPR_DP_INSTALL_POLICY=1")
	}
	if inventory.Config.subjectLinks() {
		args = append(args, "-e", fmt.Sprintf("ZPR_DP_DELIVERY_PORT=%d", inventory.Config.SnapshotDeliveryPort))
	}
	if inventory.Config.ProtectedDNS {
		args = append(args, "-e", "ZPR_DP_PROTECTED_DNS=1")
	}
	readOnly, writable := evaluationDataPlaneMounts(state, inventory.Config)
	for _, path := range readOnly {
		args = append(args, "--mount", "type=bind,source="+path+",target="+path+",readonly")
	}
	for _, path := range writable {
		args = append(args, "--mount", "type=bind,source="+path+",target="+path)
	}
	return append(args, image)
}

// evaluationDataPlaneMountsSafe rejects paths that would break the
// comma-separated --mount syntax.
func evaluationDataPlaneMountsSafe(state string) error {
	if strings.ContainsAny(state, ",\"=") {
		return errors.New("-data-plane state path must not contain commas, quotes or '='")
	}
	return nil
}

func evaluationDockerCommand(ctx context.Context, args ...string) error {
	command := exec.CommandContext(ctx, "docker", args...)
	command.Env = evaluationDockerEnvironment()
	command.Stdout, command.Stderr = io.Discard, io.Discard
	return command.Run()
}

// createEvaluationDataPlaneNetwork gives this install a private Docker network
// that later endpoint containers join; stale data-plane containers are removed.
func createEvaluationDataPlaneNetwork(ctx context.Context, inventory evaluationInventory) error {
	name := evaluationDataPlaneName(inventory)
	_ = evaluationDockerCommand(ctx, "rm", "-f", name)
	if len(inventory.Config.DataPlaneNodes) > 0 {
		return prepareEvaluationDataPlaneNetwork(ctx, inventory)
	}
	if evaluationDockerCommand(ctx, "network", "inspect", name) == nil {
		return nil
	}
	if err := evaluationDockerCommand(ctx, "network", "create", "--label", "org.zpr.evaluation-install="+inventory.InstallID, name); err != nil {
		return errors.New("cannot create the data-plane Docker network")
	}
	return nil
}

func removeEvaluationDataPlane(ctx context.Context, inventory evaluationInventory) error {
	name := evaluationDataPlaneName(inventory)
	_ = evaluationDockerCommand(ctx, "rm", "-f", name)
	if evaluationDockerCommand(ctx, "network", "inspect", name) != nil {
		return nil
	}
	// Endpoint containers (run by the Simulator) are members of this install's
	// data plane; they cannot outlive it and would block network removal.
	members := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", "network="+name)
	members.Env = evaluationDockerEnvironment()
	if output, err := members.Output(); err == nil {
		if ids := strings.Fields(string(output)); len(ids) > 0 {
			_ = evaluationDockerCommand(ctx, append([]string{"rm", "-f"}, ids...)...)
		}
	}
	return evaluationDockerCommand(ctx, "network", "rm", name)
}

func evaluationDataPlaneInitCommand(inventory evaluationInventory, state string) string {
	return fmt.Sprintf("docker exec %s ip netns exec zpr-vs vs --initialize-subject-snapshot -c %q", evaluationDataPlaneName(inventory), filepath.Join(state, "vs.toml"))
}

// checkEvaluationDataPlaneActors requires vs.zpr and every named node to be
// admitted actors, proving each node reached the VS over the ZPR overlay.
func checkEvaluationDataPlaneActors(state string, port int, config *tls.Config, nodes ...string) error {
	key, err := os.ReadFile(filepath.Join(state, "vs-admin-api.key"))
	if err != nil {
		return errors.New("cannot read private Visa Service admin key")
	}
	transport := &http.Transport{TLSClientConfig: config, Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://localhost:%d/admin/actors", port), nil)
	if err != nil {
		return err
	}
	request.Header.Set("X-API-Key", strings.TrimSpace(string(key)))
	response, err := client.Do(request)
	if err != nil {
		return errors.New("Visa Service admin HTTPS not ready")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode != http.StatusOK {
		return errors.New("Visa Service actor list unavailable")
	}
	if !evaluationDataPlaneActorsAdmitted(body, nodes...) {
		return errors.New("nodes and vs.zpr adapter not yet admitted")
	}
	return nil
}

func evaluationDataPlaneActorsAdmitted(body []byte, nodes ...string) bool {
	if len(nodes) == 0 {
		nodes = []string{"node"}
	}
	required := append([]string{"vs.zpr"}, nodes...)
	var actors any
	if json.Unmarshal(body, &actors) != nil {
		return false
	}
	found := map[string]bool{}
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for _, item := range typed {
				walk(item)
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case string:
			if slices.Contains(required, typed) {
				found[typed] = true
			}
		}
	}
	walk(actors)
	return len(found) == len(required)
}

func checkEvaluationProtectedDNS(ctx context.Context, inventory evaluationInventory) error {
	if err := checkEvaluationDNSRecord(ctx, inventory, "dns.svc.zpr.", evaluationProtectedDNSAddress); err != nil {
		return fmt.Errorf("protected DNS TCP query over the ZPR tunnel failed: %w", err)
	}
	if err := checkEvaluationDNSRecord(ctx, inventory, "vs.zpr.adapters.svc.zpr.", evaluationProtectedDNSVisaAddress); err != nil {
		return fmt.Errorf("Visa Service dynamic DNS publication was not queryable over the ZPR tunnel: %w", err)
	}
	return nil
}

func checkEvaluationDNSRecord(ctx context.Context, inventory evaluationInventory, name, expected string) error {
	args := append([]string{"exec", evaluationDataPlaneName(inventory)}, evaluationProtectedDNSQueryArguments(name)...)
	command := exec.CommandContext(ctx, "docker", args...)
	command.Env = evaluationDockerEnvironment()
	output, err := command.Output()
	if err != nil {
		return err
	}
	if evaluationProtectedDNSAnswerMatches(output, expected) {
		return nil
	}
	return fmt.Errorf("DNS record %s did not return expected address", name)
}

func evaluationProtectedDNSQueryArguments(name string) []string {
	return []string{"ip", "netns", "exec", "zpr-vs", "dig", "+tcp", "+time=2", "+tries=1", "+short",
		"AAAA", "@" + evaluationProtectedDNSAddress, name}
}

func evaluationProtectedDNSAnswerMatches(output []byte, expected string) bool {
	expectedAddress, expectedErr := netip.ParseAddr(expected)
	if expectedErr != nil {
		return false
	}
	for _, answer := range strings.Fields(string(output)) {
		address, err := netip.ParseAddr(answer)
		if err == nil && address == expectedAddress {
			return true
		}
	}
	return false
}
