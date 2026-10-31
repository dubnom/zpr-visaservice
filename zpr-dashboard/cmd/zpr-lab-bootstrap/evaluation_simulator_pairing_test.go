package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func pairingFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func readPairingDex(t *testing.T, state string) (map[string]json.RawMessage, []map[string]any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(state, "dex.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	var clients []map[string]any
	if err := json.Unmarshal(document["staticClients"], &clients); err != nil {
		t.Fatal(err)
	}
	return document, clients
}

func TestEvaluationSimulatorPairingRegistersSeparateClient(t *testing.T) {
	state, inventory := recoveryFixture(t)
	config := inventory.Config
	port := pairingFreePort(t)
	_, before := readPairingDex(t, state)
	roomSecret := readSubjectFile(t, state, "oidc-room-client.secret")
	if err := pairEvaluationSimulator(state, port, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	updated := config
	updated.SimulatorPort = port
	paired, err := readEvaluation(state, updated)
	if err != nil {
		t.Fatalf("paired install failed integrity verification: %v", err)
	}
	if paired.InstallID != inventory.InstallID {
		t.Fatal("pairing must retain the install identity")
	}
	if _, err := readEvaluation(state, config); err == nil {
		t.Fatal("original flags without -simulator-port must no longer match the frozen configuration")
	}
	_, clients := readPairingDex(t, state)
	if len(clients) != 2 || !reflect.DeepEqual(clients[0], before[0]) {
		t.Fatalf("pairing must append one client and keep the Room client identical: %v", clients)
	}
	if string(readSubjectFile(t, state, "oidc-room-client.secret")) != string(roomSecret) {
		t.Fatal("pairing must not rotate the Room client secret")
	}
	secret := readSubjectFile(t, state, "oidc-simulator-client.secret")
	if clients[1]["id"] != "zpr-simulator" || clients[1]["secret"] != string(secret) || string(secret) == string(roomSecret) {
		t.Fatal("Simulator client requires its own registered independent secret")
	}
	redirects, ok := clients[1]["redirectURIs"].([]any)
	if !ok || len(redirects) != 1 || redirects[0] != "https://localhost:"+strconv.Itoa(port)+"/auth/operator/callback" {
		t.Fatalf("Simulator callback must use the registered port: %v", clients[1]["redirectURIs"])
	}
	signer := readSubjectFile(t, state, "simulator-delegation-signer.json")
	if !strings.Contains(string(signer), evaluationSimulatorDelegationKeyID) {
		t.Fatal("Simulator delegation signer must use the independent key ID")
	}
	chain := readSubjectFile(t, state, "simulator-client-chain.pem")
	block, _ := pem.Decode(chain)
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil || leaf.Subject.CommonName != "ZPR evaluation-only simulator-client" {
		t.Fatal("Simulator transport identity must be its own CA-issued client certificate")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(readSubjectFile(t, state, "ca.pem")) {
		t.Fatal(err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatalf("Simulator client certificate must chain to the install CA: %v", err)
	}
	dexBefore := readSubjectFile(t, state, "dex.json")
	if err := pairEvaluationSimulator(state, port, time.Now().UTC()); err != nil {
		t.Fatalf("repeating the same registration must succeed without changes: %v", err)
	}
	if string(readSubjectFile(t, state, "dex.json")) != string(dexBefore) {
		t.Fatal("repeated pairing must not modify the registered configuration")
	}
	if err := pairEvaluationSimulator(state, pairingFreePort(t), time.Now().UTC()); err == nil ||
		!strings.Contains(err.Error(), "never rebinds") {
		t.Fatalf("pairing must refuse to rebind an existing registration: %v", err)
	}
}

func TestEvaluationSimulatorPairingExtendsSubjectLinkTrust(t *testing.T) {
	state, inventory := recoveryFixtureWith(t, testEvaluationSubjectConfig())
	config := inventory.Config
	issuer := config.browserOrigin(config.OIDCPort)
	organization := evaluationOrganization(inventory)
	admin := operatorGrantIdentity{Subject: "admin-subject", Permissions: append([]string{"monitor.read", "approval.self"}, evaluationAdminSubjectLinkPermissions...)}
	reviewer := operatorGrantIdentity{Subject: "reviewer-subject", Permissions: evaluationReviewerPermissions}
	roomKey := readSubjectFile(t, state, "operator-delegation-key.pem")
	roomChain := readSubjectFile(t, state, "client-chain.pem")
	trust, err := evaluationDelegationTrustFile(config, issuer, organization, roomKey, roomChain, nil, nil, admin, reviewer)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeEvaluationOnce(state, evaluationDelegationTrust, trust); err != nil {
		t.Fatal(err)
	}
	port := pairingFreePort(t)
	if err := pairEvaluationSimulator(state, port, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	updated := config
	updated.SimulatorPort = port
	if _, err := readEvaluation(state, updated); err != nil {
		t.Fatal(err)
	}
	extended := readSubjectFile(t, state, evaluationDelegationTrust)
	var parsed operatordelegation.Config
	if err := json.Unmarshal(extended, &parsed); err != nil || parsed.Validate() != nil {
		t.Fatalf("extended trust invalid: %s", extended)
	}
	if len(parsed.Keys) != 2 || parsed.Keys[0].KeyID != evaluationDelegationKeyID ||
		parsed.Keys[1].KeyID != evaluationSimulatorDelegationKeyID {
		t.Fatalf("trust must retain the Room key and add only the Simulator key: %s", extended)
	}
	simulatorKey := ed25519PrivateFromPEM(t, readSubjectFile(t, state, "simulator-delegation-key.pem"))
	block, _ := pem.Decode(readSubjectFile(t, state, "simulator-client-chain.pem"))
	pin := sha256.Sum256(block.Bytes)
	if parsed.Keys[1].PublicKey != base64.RawURLEncoding.EncodeToString(simulatorKey.Public().(ed25519.PublicKey)) ||
		parsed.Keys[1].CertificateSHA256 != hex.EncodeToString(pin[:]) {
		t.Fatal("Simulator key must be pinned to its own transport certificate")
	}
	if parsed.Keys[1].PublicKey == parsed.Keys[0].PublicKey || parsed.Keys[1].CertificateSHA256 == parsed.Keys[0].CertificateSHA256 {
		t.Fatal("Simulator signer must not reuse the Room key or certificate")
	}
	if len(parsed.Grants) != 2 || parsed.Grants[0].Subject != "admin-subject" || parsed.Grants[1].Subject != "reviewer-subject" {
		t.Fatal("pairing must not change named grants")
	}
	// The next verified-identity rendering must match the extended file so the
	// write-once contract does not refuse the retained binding after restart.
	regenerated, err := evaluationDelegationTrustFile(updated, issuer, organization, roomKey, roomChain,
		readSubjectFile(t, state, "simulator-delegation-key.pem"), readSubjectFile(t, state, "simulator-client-chain.pem"), admin, reviewer)
	if err != nil {
		t.Fatal(err)
	}
	if string(regenerated) != string(extended) {
		t.Fatalf("regenerated trust must match the extended file byte for byte:\n%s\n---\n%s", regenerated, extended)
	}
	if err := writeEvaluationOnce(state, evaluationDelegationTrust, regenerated); err != nil {
		t.Fatalf("write-once must accept the regenerated binding: %v", err)
	}
}

func TestEvaluationSimulatorPairingResumesInterruptedCommit(t *testing.T) {
	state, inventory := recoveryFixture(t)
	config := inventory.Config
	port := pairingFreePort(t)
	updated := config
	updated.SimulatorPort = port
	files, err := planEvaluationSimulatorPairing(state, inventory, updated, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	names := stagePendingSimulatorPairing(t, state, port, "", files)
	// Interrupted mid-commit: exactly one staged file was already renamed.
	if err := os.Rename(filepath.Join(state, names[0]+evaluationSimulatorPairingSuffix), filepath.Join(state, names[0])); err != nil {
		t.Fatal(err)
	}
	if _, err := readEvaluation(state, config); err == nil || !strings.Contains(err.Error(), "interrupted Simulator pairing") {
		t.Fatalf("interrupted pairing must fail closed, not start: %v", err)
	}
	if err := pairEvaluationSimulator(state, port+1, time.Now().UTC()); err == nil ||
		!strings.Contains(err.Error(), "different simulator-port") {
		t.Fatalf("resume must require the recorded port: %v", err)
	}
	if err := pairEvaluationSimulator(state, port, time.Now().UTC()); err != nil {
		t.Fatalf("resume must complete the recorded commit: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(state, evaluationSimulatorPairingPending)); !os.IsNotExist(err) {
		t.Fatal("completed pairing must retire the pending plan")
	}
	if _, err := readEvaluation(state, updated); err != nil {
		t.Fatalf("resumed install failed integrity verification: %v", err)
	}
}

func TestEvaluationSimulatorPairingRefusesUnsafeState(t *testing.T) {
	state, inventory := recoveryFixture(t)
	config := inventory.Config
	busy, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(config.RoomPort))
	if err != nil {
		t.Fatal(err)
	}
	if err := pairEvaluationSimulator(state, pairingFreePort(t), time.Now().UTC()); err == nil ||
		!strings.Contains(err.Error(), "occupied") {
		busy.Close()
		t.Fatalf("pairing must require stopped install services: %v", err)
	}
	busy.Close()
	simulator, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer simulator.Close()
	occupied := simulator.Addr().(*net.TCPAddr).Port
	if err := pairEvaluationSimulator(state, occupied, time.Now().UTC()); err == nil ||
		!strings.Contains(err.Error(), "Simulator port is occupied") {
		t.Fatalf("pairing must refuse an occupied Simulator port: %v", err)
	}
	if err := pairEvaluationSimulator(state, config.RoomPort, time.Now().UTC()); err == nil {
		t.Fatal("pairing must refuse a port already owned by the install")
	}
}

func TestEvaluationNewInstallSimulatorSignerIsIndependent(t *testing.T) {
	config := testEvaluationSubjectConfig()
	config.SimulatorPort = 28788
	state := filepath.Join(recoveryTempDir(t), "install")
	inventory, err := prepareEvaluation(state, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range evaluationSimulatorPairingFiles {
		if inventory.Files[name] == "" {
			t.Fatalf("new install with a Simulator port must freeze %s", name)
		}
	}
	room := ed25519PrivateFromPEM(t, readSubjectFile(t, state, "operator-delegation-key.pem"))
	simulator := ed25519PrivateFromPEM(t, readSubjectFile(t, state, "simulator-delegation-key.pem"))
	if room.Equal(simulator) {
		t.Fatal("Simulator delegation key must be independent of the Room key")
	}
	if string(readSubjectFile(t, state, "simulator-client-chain.pem")) == string(readSubjectFile(t, state, "client-chain.pem")) {
		t.Fatal("Simulator transport certificate must be independent of the Room client certificate")
	}
}
