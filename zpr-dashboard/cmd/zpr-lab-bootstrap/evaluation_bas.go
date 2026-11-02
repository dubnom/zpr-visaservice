package main

// EVALUATION ONLY: resumable add-later provisioning of the evaluation
// authentication authority (BAS) for an existing protected install, per the
// approved 2026-11-01 decision that an endpoint-authentication authority is
// part of the evaluation-install minimum. The command generates the dedicated
// constrained BAS key material, extends the Enrollment Service configuration
// with the single-store BAS section, and emits the reviewed root-adapter
// scripts. Existing matching material is adopted and never regenerated or
// overwritten; mismatched material fails closed. The separate
// authentication-root policy/configuration change stays with the maintained
// Policy-Service change-review pipeline under the recorded self-approval
// decision; this command only prints the exact root identity and policy
// stanza for it and never grants anything by itself.

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const (
	evaluationBASCAFile         = "bas-ca.pem"
	evaluationBASCAKeyFile      = "bas-ca.key"
	evaluationBASServerCert     = "auth.zpr-runtime.pem"
	evaluationBASServerKey      = "auth.zpr-runtime.key"
	evaluationBASTokenKeyFile   = "token-signing.key"
	evaluationBASRootKeyFile    = "root-adapter.key"
	evaluationBASRootPubFile    = "root-adapter.pub"
	evaluationBASConfigFragment = "bas-compiler-config-fragment.toml"
	evaluationBASHostScript     = "bas-adapter-host.sh"
	evaluationBASInnerScript    = "bas-adapter-container.sh"
	evaluationBASTokenKeyBytes  = 48
	evaluationBASCertDays       = 730
)

func runEvaluationBAS(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-lab-bootstrap -evaluation-bas", flag.ContinueOnError)
	flags.SetOutput(stderr)
	state := flags.String("state", "", "existing protected evaluation install directory (required)")
	enrollmentConfig := flags.String("enrollment-config", "", "existing Enrollment Service configuration to extend with the BAS section (required)")
	basDir := flags.String("bas-dir", "", "private directory for the BAS key material and adapter scripts (required)")
	adapterListen := flags.String("adapter-listen", "", "explicit BAS adapter HTTPS listener IP:port (required)")
	tokenListen := flags.String("token-listen", "", "explicit BAS mTLS token HTTPS listener IP:port (required)")
	adapterImage := flags.String("adapter-image", "", "exact immutable container image ID for the BAS root adapter (required)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *state == "" || *enrollmentConfig == "" || *basDir == "" ||
		*adapterListen == "" || *tokenListen == "" || *adapterImage == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: zpr-lab-bootstrap -evaluation-bas -state DIR -enrollment-config FILE -bas-dir DIR -adapter-listen IP:PORT -token-listen IP:PORT -adapter-image sha256:HEX")
		return 2
	}
	fmt.Fprintln(stdout, evaluationBanner)
	if err := provisionEvaluationBAS(*state, *enrollmentConfig, *basDir, *adapterListen, *tokenListen, *adapterImage, stdout); err != nil {
		fmt.Fprintf(stderr, "evaluation BAS: %v\n", err)
		return 1
	}
	return 0
}

func provisionEvaluationBAS(state, enrollmentConfig, basDir, adapterListen, tokenListen, adapterImage string, stdout io.Writer) error {
	for _, path := range []string{state, enrollmentConfig, basDir} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, ",\"\r\n=") {
			return errors.New("state, enrollment-config and bas-dir require canonical absolute paths")
		}
	}
	for _, listen := range []string{adapterListen, tokenListen} {
		host, port, err := net.SplitHostPort(listen)
		if err != nil || net.ParseIP(host) == nil || port == "" {
			return errors.New("listeners require a literal IP and port")
		}
	}
	if adapterListen == tokenListen {
		return errors.New("adapter and token listeners must be separate")
	}
	if !strings.HasPrefix(adapterImage, "sha256:") || len(adapterImage) != len("sha256:")+64 {
		return errors.New("adapter-image requires an exact immutable sha256 image ID")
	}
	data, err := os.ReadFile(filepath.Join(state, "evaluation.json"))
	var inventory evaluationInventory
	if err != nil || json.Unmarshal(data, &inventory) != nil {
		return errors.New("evaluation inventory unavailable")
	}
	if err := inventory.Config.validate(); err != nil {
		return fmt.Errorf("invalid evaluation inventory: %w", err)
	}
	if !inventory.Config.DataPlane {
		return errors.New("the BAS root adapter requires the install-owned data plane")
	}
	organization := evaluationOrganization(inventory)
	if err := os.MkdirAll(basDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(basDir, 0o700); err != nil {
		return err
	}
	material, err := ensureEvaluationBASMaterial(basDir, organization)
	if err != nil {
		return err
	}
	if err := extendEvaluationBASEnrollmentConfig(enrollmentConfig, basDir, organization, adapterListen, tokenListen, material); err != nil {
		return err
	}
	if err := writeEvaluationBASAdapterScripts(state, basDir, organization, inventory, adapterImage, adapterListen, tokenListen, material); err != nil {
		return err
	}
	if err := writeEvaluationBASConfigFragment(state, basDir, organization, adapterListen, tokenListen); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Evaluation BAS provisioned for %s.\n", organization)
	fmt.Fprintf(stdout, "Root adapter public key SHA-256: %s\n", material.rootPublicKeySHA256)
	fmt.Fprintf(stdout, "Visa Service peer certificate SHA-256: %s\n", material.visaServicePeerSHA256)
	fmt.Fprintf(stdout, "Restart only the Enrollment Service to serve the BAS listeners, then run the adapter host script (%s) under its own unit.\n",
		filepath.Join(basDir, evaluationBASHostScript))
	fmt.Fprintf(stdout, "The authentication root still requires the separate self-approved review of the emitted compiler-config fragment (%s) and the policy stanza below. Both are merged through the maintained change-review pipeline; this command grants nothing.\n\n"+
		"# Explicit authentication-root proposal only; not workload access policy.\n"+
		"define %sEnrollmentAuth as a service with device.zpr.adapter.cn:'%s-enrollment-bas'.\n"+
		"service %sEnrollmentAuth as json {\"service_class\":\"%sEnrollmentAuth\"}.\n"+
		"  allow zpr.adapter.cn: devices.\n",
		filepath.Join(basDir, evaluationBASConfigFragment),
		evaluationBASTitle(organization), organization, evaluationBASTitle(organization), evaluationBASTitle(organization))
	return nil
}

// ensureEvaluationBASRootPublicKey writes the root adapter SPKI public key
// PEM beside its private key for the reviewed compiler-config bootstrap pin.
func ensureEvaluationBASRootPublicKey(path string, keyPEM []byte) error {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return errors.New("invalid root adapter key PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	signer, ok := key.(interface{ Public() crypto.PublicKey })
	if !ok {
		return errors.New("unsupported root adapter key type")
	}
	der, err := x509.MarshalPKIXPublicKey(signer.Public())
	if err != nil {
		return err
	}
	return writeEvaluationBASFile(path, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// writeEvaluationBASConfigFragment emits the exact reviewed compiler-config
// stanzas an operator merges (through the self-approved configuration review)
// so the data plane bootstraps the BAS root adapter, trusts its validation
// authority, and exposes its OAuth and mTLS validation services. Paths are
// absolute so the fragment is unambiguous when merged.
func writeEvaluationBASConfigFragment(state, basDir, organization, adapterListen, tokenListen string) error {
	_, adapterPort, _ := net.SplitHostPort(adapterListen)
	_, tokenPort, _ := net.SplitHostPort(tokenListen)
	adapterCN := organization + "-enrollment-bas"
	fragment := fmt.Sprintf(`# EVALUATION ONLY BAS authentication-root compiler-config fragment for %[1]s.
# Merge these stanzas into the data-plane compiler configuration through the
# self-approved configuration review; they admit only the authentication root.

[bootstrap]
%[2]q = %[3]q

[trusted_services.%[2]s]
expiration_seconds = 300
api = "validation/2"
client = "%[4]sEnrollmentAuth"
cert_path = %[5]q
returns_attributes = [
  "organization -> device.enrollment.organization",
  "asset_id -> device.enrollment.asset_id",
  "profile -> device.enrollment.profile",
  "type -> device.enrollment.type",
]
identity_attributes = ["asset_id"]
provider = [["device.zpr.adapter.cn", %[2]q]]

[services.%[2]s-vs]
protocol = "zpr-validation2"
port = %[6]s

[services.%[4]sEnrollmentAuth]
protocol = "zpr-oauthrsa"
port = %[7]s
`, organization, adapterCN, filepath.Join(basDir, evaluationBASRootPubFile),
		evaluationBASTitle(organization), filepath.Join(basDir, evaluationBASCAFile),
		tokenPort, adapterPort)
	return writeEvaluationBASFile(filepath.Join(basDir, evaluationBASConfigFragment), []byte(fragment))
}

type evaluationBASMaterial struct {
	rootPublicKeySHA256   string
	visaServicePeerSHA256 string
}

func evaluationBASTitle(organization string) string {
	parts := strings.Split(organization, "-")
	for i, part := range parts {
		if part != "" {
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			parts[i] = string(runes)
		}
	}
	return strings.Join(parts, "")
}

// ensureEvaluationBASMaterial generates missing key material and adopts
// existing matching material without ever overwriting it. Partially present
// or unparsable material fails closed for explicit operator inspection.
func ensureEvaluationBASMaterial(basDir, organization string) (evaluationBASMaterial, error) {
	none := evaluationBASMaterial{}
	caCert, caKey, err := ensureEvaluationBASCA(basDir, organization)
	if err != nil {
		return none, err
	}
	serverName := "auth.zpr-runtime"
	if _, err := ensureEvaluationBASIdentity(basDir, evaluationBASServerCert, evaluationBASServerKey,
		serverName, []string{"auth.zpr"}, x509.ExtKeyUsageServerAuth, caCert, caKey); err != nil {
		return none, err
	}
	peerCertificate, err := ensureEvaluationBASIdentity(basDir,
		organization+"-visa-service-runtime.pem", organization+"-visa-service-runtime.key",
		organization+"-visa-service-runtime", nil, x509.ExtKeyUsageClientAuth, caCert, caKey)
	if err != nil {
		return none, err
	}
	tokenPath := filepath.Join(basDir, evaluationBASTokenKeyFile)
	if info, err := os.Lstat(tokenPath); err == nil {
		if !info.Mode().IsRegular() || info.Size() != evaluationBASTokenKeyBytes {
			return none, errors.New("existing token signing key does not match the expected shape; inspect it")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		secret := make([]byte, evaluationBASTokenKeyBytes)
		if _, err := rand.Read(secret); err != nil {
			return none, err
		}
		if err := writeEvaluationBASFile(tokenPath, secret); err != nil {
			return none, err
		}
	} else {
		return none, err
	}
	rootPath := filepath.Join(basDir, evaluationBASRootKeyFile)
	rootPEM, err := os.ReadFile(rootPath)
	if errors.Is(err, os.ErrNotExist) {
		key, err := rsa.GenerateKey(rand.Reader, 3072)
		if err != nil {
			return none, err
		}
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return none, err
		}
		rootPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
		if err := writeEvaluationBASFile(rootPath, rootPEM); err != nil {
			return none, err
		}
	} else if err != nil {
		return none, err
	}
	rootDigest, err := evaluationBASPublicKeySHA256(rootPEM)
	if err != nil {
		return none, fmt.Errorf("root adapter key: %w", err)
	}
	if err := ensureEvaluationBASRootPublicKey(filepath.Join(basDir, evaluationBASRootPubFile), rootPEM); err != nil {
		return none, err
	}
	peerDigest := sha256.Sum256(peerCertificate.Raw)
	return evaluationBASMaterial{
		rootPublicKeySHA256:   rootDigest,
		visaServicePeerSHA256: hex.EncodeToString(peerDigest[:]),
	}, nil
}

func evaluationBASPublicKeySHA256(keyPEM []byte) (string, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return "", errors.New("invalid private key PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}
	signer, ok := key.(interface{ Public() crypto.PublicKey })
	if !ok {
		return "", errors.New("unsupported private key type")
	}
	der, err := x509.MarshalPKIXPublicKey(signer.Public())
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(der)
	return hex.EncodeToString(digest[:]), nil
}

func writeEvaluationBASFile(path string, content []byte) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func ensureEvaluationBASCA(basDir, organization string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPath := filepath.Join(basDir, evaluationBASCAFile)
	keyPath := filepath.Join(basDir, evaluationBASCAKeyFile)
	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	switch {
	case certErr == nil && keyErr == nil:
		certificate, err := parseEvaluationBASCertificate(certPEM)
		if err != nil || !certificate.IsCA {
			return nil, nil, errors.New("existing BAS CA certificate is invalid; inspect it")
		}
		key, err := parseEvaluationBASECKey(keyPEM)
		if err != nil {
			return nil, nil, errors.New("existing BAS CA key is invalid; inspect it")
		}
		return certificate, key, nil
	case errors.Is(certErr, os.ErrNotExist) && errors.Is(keyErr, os.ErrNotExist):
	default:
		return nil, nil, errors.New("partial BAS CA material; inspect the BAS directory")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: evaluationBASTitle(organization) + " BAS runtime CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(0, 0, evaluationBASCertDays),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	if err := writeEvaluationBASFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})); err != nil {
		return nil, nil, err
	}
	if err := writeEvaluationBASFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})); err != nil {
		return nil, nil, err
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return certificate, key, nil
}

func parseEvaluationBASCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("invalid certificate PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

func parseEvaluationBASECKey(keyPEM []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("invalid key PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("not an EC key")
	}
	return ecKey, nil
}

// ensureEvaluationBASIdentity adopts or issues one CA-signed TLS identity and
// returns its certificate.
func ensureEvaluationBASIdentity(basDir, certName, keyName, commonName string, dnsNames []string,
	usage x509.ExtKeyUsage, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) (*x509.Certificate, error) {
	certPath := filepath.Join(basDir, certName)
	keyPath := filepath.Join(basDir, keyName)
	certPEM, certErr := os.ReadFile(certPath)
	_, keyErr := os.Stat(keyPath)
	switch {
	case certErr == nil && keyErr == nil:
		certificate, err := parseEvaluationBASCertificate(certPEM)
		if err != nil || certificate.Subject.CommonName != commonName {
			return nil, fmt.Errorf("existing %s identity is invalid; inspect it", commonName)
		}
		return certificate, nil
	case errors.Is(certErr, os.ErrNotExist) && errors.Is(keyErr, os.ErrNotExist):
	default:
		return nil, fmt.Errorf("partial %s identity material; inspect the BAS directory", commonName)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(0, 0, evaluationBASCertDays),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
		DNSNames:     dnsNames,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := writeEvaluationBASFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})); err != nil {
		return nil, err
	}
	if err := writeEvaluationBASFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})); err != nil {
		return nil, err
	}
	return x509.ParseCertificate(der)
}

// extendEvaluationBASEnrollmentConfig adds the single-store BAS section to
// the Enrollment Service configuration, preserving every other field. An
// existing equal section is adopted; a different one fails closed.
func extendEvaluationBASEnrollmentConfig(path, basDir, organization, adapterListen, tokenListen string, material evaluationBASMaterial) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read enrollment configuration: %w", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("parse enrollment configuration: %w", err)
	}
	database, _ := document["database_file"].(string)
	if database == "" {
		return errors.New("enrollment configuration lacks its database_file; the BAS is single-store by design")
	}
	_, adapterPort, _ := net.SplitHostPort(adapterListen)
	_, tokenPort, _ := net.SplitHostPort(tokenListen)
	section := map[string]any{
		"version":              1,
		"organization":         organization,
		"allow_non_loopback":   true,
		"database_file":        database,
		"signing_key_file":     filepath.Join(basDir, evaluationBASTokenKeyFile),
		"visa_service_ca_file": filepath.Join(basDir, evaluationBASCAFile),
		"visa_service_peers": []any{map[string]any{
			"name":               organization + "-visa-service",
			"certificate_sha256": material.visaServicePeerSHA256,
		}},
		"adapter": map[string]any{
			"listen": adapterListen, "origin": "https://auth.zpr:" + adapterPort,
			"certificate_file": filepath.Join(basDir, evaluationBASServerCert),
			"key_file":         filepath.Join(basDir, evaluationBASServerKey),
		},
		"token": map[string]any{
			"listen": tokenListen, "origin": "https://auth.zpr:" + tokenPort,
			"certificate_file": filepath.Join(basDir, evaluationBASServerCert),
			"key_file":         filepath.Join(basDir, evaluationBASServerKey),
		},
		"nonce_lifetime_seconds": 60,
		"code_lifetime_seconds":  60,
		"token_lifetime_seconds": 300,
		"claim_sources": map[string]any{
			"asset_id": "asset_id", "organization": "organization",
			"profile": "profile", "type": "type",
		},
	}
	if existing, present := document["bas"]; present {
		existingJSON, _ := json.Marshal(existing)
		desiredJSON, _ := json.Marshal(section)
		if string(existingJSON) == string(desiredJSON) {
			return nil
		}
		return errors.New("enrollment configuration already carries a different BAS section; reconcile it explicitly")
	}
	backup := path + ".before-evaluation-bas"
	if _, err := os.Lstat(backup); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(backup, data, 0o600); err != nil {
			return err
		}
	}
	document["bas"] = section
	updated, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return writeEvaluationBASFile(path, append(updated, '\n'))
}

// writeEvaluationBASAdapterScripts emits the host and container scripts for
// the reviewed BAS root adapter, mirroring the qualified Velocity pattern.
// Existing scripts are replaced only through their temporary-rename write.
func writeEvaluationBASAdapterScripts(state, basDir, organization string, inventory evaluationInventory,
	adapterImage, adapterListen, tokenListen string, material evaluationBASMaterial) error {
	node := "zpr-evaluation-" + inventory.InstallID + "-data-plane"
	adapterHost, adapterPort, _ := net.SplitHostPort(adapterListen)
	_, tokenPort, _ := net.SplitHostPort(tokenListen)
	containerName := "zpr-enrollment-bas-" + organization
	inner := fmt.Sprintf(`#!/bin/sh
# EVALUATION ONLY: reviewed BAS root adapter for %[1]s (emitted by -evaluation-bas).
set -eu
umask 077
address=fd5a:5052:adda:1:ffff:ffff:ffff:fffd
pids=""
cleanup() {
    trap - EXIT INT TERM
    for pid in $pids; do
        if kill -0 "$pid" 2>/dev/null; then kill -TERM "$pid"; fi
    done
    for pid in $pids; do
        if wait "$pid"; then :; else
            status=$?
            case $status in 143) ;; *) printf 'BAS child %%s exited with status %%s\n' "$pid" "$status" >&2 ;; esac
        fi
    done
}
trap cleanup EXIT
trap 'exit 0' INT TERM
mkdir -p /run/zpr
ip tuntap add name tunbas mode tun multi_queue
ip link set tunbas up
ZPR_ADAPTER_SERVICES=%[2]sEnrollmentAuth,%[1]s-enrollment-bas \
ph adapter --logging all=INFO --control-path /run/zpr/bas.sock \
    --capture-path /run/zpr/bas-cap.sock --self-addr 0.0.0.0:0 \
    --ca-file %[3]q \
    --bootstrap-key %[4]q \
    --name %[1]s-enrollment-bas --km-impl noise --tun-if tunbas \
    --io-engine posix_unbatched --node-addr "${ZPR_REVIEWED_NODE:?}:5000" --zpr-addr "$address" &
adapter=$!
pids="$adapter"
ready=false
for attempt in $(seq 1 45); do
    if ! kill -0 "$adapter" 2>/dev/null; then
        printf 'Reviewed BAS root adapter exited before admission\n' >&2
        exit 1
    fi
    if ip -6 addr show dev tunbas | grep -q "inet6 $address/"; then
        ready=true
        break
    fi
    sleep 1
done
[ "$ready" = true ] || { printf 'Reviewed BAS root admission timed out\n' >&2; exit 1; }
for port in %[5]s %[6]s; do
    socat -T 15 "TCP6-LISTEN:$port,bind=[$address],reuseaddr,fork" "TCP4:%[7]s:$port" &
    pids="$pids $!"
done
printf 'Reviewed BAS root relays ready; TLS remains end-to-end to the Enrollment Service\n'
while :; do
    for pid in $pids; do
        if ! kill -0 "$pid" 2>/dev/null; then
            printf 'Required BAS root child %%s exited\n' "$pid" >&2
            exit 1
        fi
    done
    sleep 1
done
`, organization, evaluationBASTitle(organization),
		filepath.Join(state, "dp-ca.crt"), filepath.Join(basDir, evaluationBASRootKeyFile),
		adapterPort, tokenPort, adapterHost)
	host := fmt.Sprintf(`#!/bin/sh
# EVALUATION ONLY: reviewed BAS root adapter host for %[1]s (emitted by -evaluation-bas).
set -eu
umask 077
install_id=%[2]s
name=%[3]s
node=%[4]s
image=%[5]s
fail() { printf '%[1]s-bas-adapter: %%s\n' "$*" >&2; exit 1; }
owned() {
    [ "$(docker inspect --format '{{index .Config.Labels "org.zpr.evaluation-install"}}' "$name")" = "$install_id" ] &&
    [ "$(docker inspect --format '{{index .Config.Labels "org.zpr.component"}}' "$name")" = enrollment-bas-adapter ] &&
    [ "$(docker inspect --format '{{.Config.Image}}' "$name")" = "$image" ]
}
[ "$#" -eq 1 ] || fail "expected start or stop"
ids=$(docker ps -a --filter "name=^/$name$" --format '{{.ID}}')
case $1 in
    stop)
        if [ -n "$ids" ]; then
            owned || fail "refusing to stop a foreign container"
            docker stop --time 20 "$name"
        fi
        exit 0
        ;;
    start) [ -z "$ids" ] || fail "existing container requires ownership/status reconciliation" ;;
    *) fail "expected start or stop" ;;
esac
[ "$(openssl pkey -in %[6]q -pubout -outform DER | sha256sum | cut -d' ' -f1)" = %[7]s ] ||
    fail "reviewed root identity changed"
ready=false
for attempt in $(seq 1 45); do
    if docker inspect "$node" >/dev/null 2>&1; then
        [ "$(docker inspect --format '{{index .Config.Labels "org.zpr.evaluation-install"}}' "$node")" = "$install_id" ] ||
            fail "foreign docking container"
        if [ "$(docker inspect --format '{{.State.Running}}' "$node")" = true ]; then
            ready=true
            break
        fi
    fi
    sleep 1
done
[ "$ready" = true ] || fail "owning %[1]s data plane unavailable"
node_ip=$(docker inspect --format '{{with index .NetworkSettings.Networks %[8]q}}{{.IPAddress}}{{end}}' "$node")
[ -n "$node_ip" ] || fail "owning docking network unavailable"
# Older data-plane images lack the ZPR-wide route in the Visa Service
# namespace; the VS needs it to reach this authentication root for the
# trusted-service token exchange. The replace is idempotent.
docker exec "$node" ip netns exec zpr-vs ip -6 route replace fd5a:5052::/32 dev tun0 ||
    fail "cannot ensure the Visa Service ZPR route"
# Container root needs DAC_OVERRIDE to read the operator-owned, read-only key.
exec docker run --rm --init --name "$name" --network "$node" --user 0:0 \
    --read-only --cap-drop ALL --cap-add NET_ADMIN --cap-add NET_RAW --cap-add DAC_OVERRIDE \
    --security-opt no-new-privileges --device /dev/net/tun \
    --tmpfs /run:rw,nosuid,nodev,size=16m --tmpfs /tmp:rw,nosuid,nodev,size=16m \
    --label "org.zpr.evaluation-install=$install_id" \
    --label org.zpr.component=enrollment-bas-adapter \
    --label org.zpr.organization=%[1]s --label org.zpr.root-proposal=evaluation-bas \
    --mount "type=bind,source=%[9]s,target=%[9]s,readonly" \
    --mount "type=bind,source=%[6]s,target=%[6]s,readonly" \
    --mount "type=bind,source=%[10]s,target=/usr/local/bin/%[1]s-bas-adapter,readonly" \
    -e "ZPR_REVIEWED_NODE=$node_ip" --entrypoint /bin/sh "$image" /usr/local/bin/%[1]s-bas-adapter
`, organization, inventory.InstallID, containerName, node, adapterImage,
		filepath.Join(basDir, evaluationBASRootKeyFile), material.rootPublicKeySHA256, node,
		filepath.Join(state, "dp-ca.crt"), filepath.Join(basDir, evaluationBASInnerScript))
	for name, content := range map[string]string{
		evaluationBASInnerScript: inner,
		evaluationBASHostScript:  host,
	} {
		path := filepath.Join(basDir, name)
		if err := writeEvaluationBASFile(path, []byte(content)); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return err
		}
	}
	return nil
}
