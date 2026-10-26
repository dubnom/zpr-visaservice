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
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

// Per-install policy: only the node and the VS adapter are admitted. Endpoint
// bootstrap keys are added by later increments.
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

// generateEvaluationDataPlane renders unique per-install ZPR identities: an
// RSA policy-signing key, an RSA ZPR CA with X25519 Noise certificates for the
// node and vs.zpr, and RSA bootstrap authentication keys for both.
func generateEvaluationDataPlane(files map[string][]byte, now time.Time) error {
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
	for _, actor := range []struct{ name, cn string }{{"node", "node"}, {"vs.zpr", "vs.zpr"}} {
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
	for _, name := range []string{"node", "vs"} {
		_, private, public, err := rsaKey()
		if err != nil {
			return err
		}
		files["dp-"+name+"-auth.key"], files["dp-"+name+"-auth.pub"] = private, public
	}
	// The CA key is deliberately not retained: endpoints are admitted by bootstrap key.
	files["dp-policy.zpl"] = []byte(evaluationDataPlanePolicy)
	files["dp-policy.zplc"] = []byte(evaluationDataPlaneCompilerConfig)
	return nil
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
	if installPolicy {
		args = append(args, "-e", "ZPR_DP_INSTALL_POLICY=1")
	}
	if inventory.Config.subjectLinks() {
		args = append(args, "-e", fmt.Sprintf("ZPR_DP_DELIVERY_PORT=%d", inventory.Config.SnapshotDeliveryPort))
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
	return evaluationDockerCommand(ctx, "network", "rm", name)
}

func evaluationDataPlaneInitCommand(inventory evaluationInventory, state string) string {
	return fmt.Sprintf("docker exec %s ip netns exec zpr-vs vs --initialize-subject-snapshot -c %q", evaluationDataPlaneName(inventory), filepath.Join(state, "vs.toml"))
}

// checkEvaluationDataPlaneActors requires both the node and vs.zpr to be
// admitted actors, proving the node reached the VS over the ZPR overlay.
func checkEvaluationDataPlaneActors(state string, port int, config *tls.Config) error {
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
	if !evaluationDataPlaneActorsAdmitted(body) {
		return errors.New("node and vs.zpr adapter not yet admitted")
	}
	return nil
}

func evaluationDataPlaneActorsAdmitted(body []byte) bool {
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
			if typed == "node" || typed == "vs.zpr" {
				found[typed] = true
			}
		}
	}
	walk(actors)
	return found["node"] && found["vs.zpr"]
}
