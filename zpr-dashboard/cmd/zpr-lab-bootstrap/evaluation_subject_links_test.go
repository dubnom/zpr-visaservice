package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func testEvaluationSubjectConfig() evaluationConfig {
	config := testEvaluationVisaConfig()
	config.SnapshotDeliveryPort = 18793
	return config
}

func readSubjectFile(t *testing.T, state, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(state, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestEvaluationSubjectLinksProtectedRender(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install")
	config := testEvaluationSubjectConfig()
	inventory, err := prepareEvaluation(state, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range evaluationSubjectLinkFiles {
		info, err := os.Stat(filepath.Join(state, name))
		if err != nil || info.Mode().Perm() != 0o600 || inventory.Files[name] == "" {
			t.Fatalf("%s is not a protected inventory file", name)
		}
	}
	if _, err := os.Stat(filepath.Join(state, "vs-subject-snapshot")); !os.IsNotExist(err) {
		t.Fatal("VS checkpoint must be created only by explicit initialization")
	}
	organization := "evaluation-" + inventory.InstallID

	var providers struct {
		Version   int                 `json:"version"`
		Providers []map[string]string `json:"providers"`
	}
	if err := json.Unmarshal(readSubjectFile(t, state, "subject-link-providers.json"), &providers); err != nil || providers.Version != 1 || len(providers.Providers) != 1 {
		t.Fatal("providers file is not one version 1 provider")
	}
	provider := providers.Providers[0]
	if len(provider) != 9 || provider["organization_id"] != organization || provider["source_id"] != evaluationSubjectSourceID || provider["source_version"] != "1" ||
		provider["url"] != "ldaps://localhost:11636" || provider["ca_file"] != filepath.Join(state, "ca.pem") || provider["bind_password_file"] != filepath.Join(state, "ldap-reader.secret") {
		t.Fatalf("unexpected provider: %v", provider)
	}

	var signing struct {
		Version int                 `json:"version"`
		KeyID   string              `json:"key_id"`
		Pins    []map[string]string `json:"pins"`
	}
	if err := json.Unmarshal(readSubjectFile(t, state, "subject-snapshot-signing.json"), &signing); err != nil || signing.Version != 1 || len(signing.Pins) != 1 || signing.Pins[0]["key_id"] != signing.KeyID {
		t.Fatal("signing configuration does not pin its active key")
	}
	public := ed25519PrivateFromPEM(t, readSubjectFile(t, state, "subject-snapshot-signing-key.pem")).Public().(ed25519.PublicKey)
	if signing.Pins[0]["public_key"] != base64.RawURLEncoding.EncodeToString(public) {
		t.Fatal("signing pin does not match the signing key")
	}
	if bytes.Contains(readSubjectFile(t, state, "subject-snapshot-signing.json"), readSubjectFile(t, state, "subject-snapshot-signing-key.pem")) {
		t.Fatal("public pins leaked the signing key")
	}

	var signer operatordelegation.SignerConfig
	if err := json.Unmarshal(readSubjectFile(t, state, "operator-delegation-signer.json"), &signer); err != nil || signer.Validate() != nil || signer.Audience != "https://localhost:18790" {
		t.Fatal("delegation signer audience must equal the Room Control-Service URL")
	}
	ed25519PrivateFromPEM(t, readSubjectFile(t, state, "operator-delegation-key.pem"))

	client, err := tls.LoadX509KeyPair(filepath.Join(state, "vs-client-chain.pem"), filepath.Join(state, "vs-client-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(client.Certificate[0])
	if err != nil || len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
		t.Fatal("VS delivery identity is not a client certificate")
	}
	spki := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	var recipients struct {
		Version    int                 `json:"version"`
		Recipients []map[string]string `json:"recipients"`
	}
	if err := json.Unmarshal(readSubjectFile(t, state, "subject-snapshot-recipients.json"), &recipients); err != nil || len(recipients.Recipients) != 1 ||
		recipients.Recipients[0]["client_spki_sha256"] != hex.EncodeToString(spki[:]) || recipients.Recipients[0]["organization_id"] != organization ||
		recipients.Recipients[0]["vs_id"] != evaluationSubjectVSID {
		t.Fatal("recipient pin does not bind the VS client certificate")
	}

	vs := string(readSubjectFile(t, state, "vs.toml"))
	for _, expected := range []string{"[subject_snapshot_fetch]", `url = "https://localhost:18793"`, `organization_id = "` + organization + `"`,
		`vs_id = "evaluation-vs"`, `epoch = "evaluation"`, filepath.Join(state, "vs-client-key.pem"), filepath.Join(state, "subject-snapshot-signing.json"),
		filepath.Join(state, "vs-subject-snapshot"), "poll_seconds = 15"} {
		if !strings.Contains(vs, expected) {
			t.Fatalf("vs.toml missing %q", expected)
		}
	}
	if strings.Contains(vs, "subject-snapshot-signing-key.pem") {
		t.Fatal("VS must receive only public signing pins")
	}

	var dex struct {
		Passwords []map[string]string `json:"staticPasswords"`
	}
	if err := json.Unmarshal(readSubjectFile(t, state, "dex.json"), &dex); err != nil || len(dex.Passwords) != 2 ||
		dex.Passwords[1]["email"] != evaluationReviewerEmail || dex.Passwords[0]["userID"] == dex.Passwords[1]["userID"] {
		t.Fatal("independent reviewer login missing")
	}
	guide := string(readSubjectFile(t, state, "SUBJECT-LINKS.txt"))
	ldif := string(readSubjectFile(t, state, "directory.ldif"))
	index := strings.Index(ldif, "entryUUID: ")
	if index < 0 {
		t.Fatal("synthetic LDAP subject has no explicit entryUUID")
	}
	uuid := ldif[index+len("entryUUID: ") : index+len("entryUUID: ")+36]
	if !strings.Contains(guide, uuid) || !strings.Contains(guide, organization) || !strings.Contains(guide, "--initialize-subject-snapshot") {
		t.Fatal("operator guide does not describe this install")
	}
	if !strings.Contains(string(readSubjectFile(t, state, "EVALUATION.txt")), "never enforced") {
		t.Fatal("evaluation notice does not disclaim enforcement")
	}

	again, err := prepareEvaluation(state, config, time.Now().Add(time.Hour))
	if err != nil || !reflect.DeepEqual(inventory, again) {
		t.Fatal("restart changed subject-link inventory")
	}
	if _, err := prepareEvaluation(state, testEvaluationVisaConfig(), time.Now()); err == nil {
		t.Fatal("restart without the delivery port accepted")
	}
	if err := os.WriteFile(filepath.Join(state, "subject-snapshot-recipients.json"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareEvaluation(state, config, time.Now()); err == nil {
		t.Fatal("changed recipients accepted")
	}
}

func ed25519PrivateFromPEM(t *testing.T, data []byte) ed25519.PrivateKey {
	t.Helper()
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" || len(bytes.TrimSpace(rest)) != 0 {
		t.Fatal("key is not one PKCS#8 PEM block")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	private, ok := key.(ed25519.PrivateKey)
	if err != nil || !ok {
		t.Fatal("key is not Ed25519")
	}
	return private
}

func TestEvaluationVisaServiceWithoutSubjectLinks(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install")
	inventory, err := prepareEvaluation(state, testEvaluationVisaConfig(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	data := readSubjectFile(t, state, "evaluation.json")
	if bytes.Contains(data, []byte("snapshot_delivery_port")) {
		t.Fatal("existing Visa Service inventories would change")
	}
	for _, name := range evaluationSubjectLinkFiles {
		if _, err := os.Stat(filepath.Join(state, name)); err == nil || inventory.Files[name] != "" {
			t.Fatalf("%s rendered without subject links", name)
		}
	}
	if strings.Contains(string(readSubjectFile(t, state, "vs.toml")), "subject_snapshot_fetch") || strings.Contains(string(readSubjectFile(t, state, "directory.ldif")), "entryUUID") {
		t.Fatal("subject-link configuration rendered without subject links")
	}
	for _, mode := range []string{"control-service", "control-room"} {
		if env := strings.Join(evaluationEnvironment(state, inventory, mode), "\n"); strings.Contains(env, "SUBJECT") || strings.Contains(env, "DELEGATION") {
			t.Fatalf("%s has subject-link wiring without subject links", mode)
		}
	}
	if len(testEvaluationVisaConfig().listenAddresses()) != 8 {
		t.Fatal("disabled delivery port reserved")
	}
}

func TestEvaluationSubjectLinksEnvironment(t *testing.T) {
	t.Setenv("ZPR_SUBJECT_SNAPSHOT_SIGNING_KEY_FILE", "/untrusted")
	inventory := evaluationInventory{InstallID: "installation", Config: testEvaluationSubjectConfig()}
	service := evaluationEnvironment("/protected/state", inventory, "control-service")
	for _, expected := range []string{"ZPR_SUBJECT_LINK_REVIEW_DB_PATH=/protected/state/subject-link-review.db",
		"ZPR_SUBJECT_LINK_PROVIDERS_FILE=/protected/state/subject-link-providers.json",
		"ZPR_OPERATOR_DELEGATION_TRUST_FILE=/protected/state/operator-delegation-trust.json",
		"ZPR_SUBJECT_SNAPSHOT_SIGNING_FILE=/protected/state/subject-snapshot-signing.json",
		"ZPR_SUBJECT_SNAPSHOT_SIGNING_KEY_FILE=/protected/state/subject-snapshot-signing-key.pem",
		"ZPR_SUBJECT_SNAPSHOT_DELIVERY_LISTEN=127.0.0.1:18793",
		"ZPR_SUBJECT_SNAPSHOT_DELIVERY_CLIENT_CA_FILE=/protected/state/ca.pem",
		"ZPR_SUBJECT_SNAPSHOT_RECIPIENTS_FILE=/protected/state/subject-snapshot-recipients.json",
		"ZPR_ADMIN_URL=https://localhost:18182"} {
		found := 0
		for _, entry := range service {
			if strings.HasPrefix(entry, strings.SplitN(expected, "=", 2)[0]+"=") {
				found++
				if entry != expected {
					t.Fatalf("Control-Service %s, want %s", entry, expected)
				}
			}
		}
		if found != 1 {
			t.Fatalf("Control-Service environment missing %s", expected)
		}
	}
	room := strings.Join(evaluationEnvironment("/protected/state", inventory, "control-room"), "\n")
	for _, expected := range []string{"ZPR_OPERATOR_DELEGATION_SIGNER_FILE=/protected/state/operator-delegation-signer.json",
		"ZPR_OPERATOR_DELEGATION_KEY_FILE=/protected/state/operator-delegation-key.pem", "ZPR_CONTROL_SERVICE_URL=https://localhost:18790"} {
		if !strings.Contains(room, expected) {
			t.Fatalf("Control Room environment missing %s", expected)
		}
	}
	for _, forbidden := range []string{"ZPR_ADMIN_", "ZPR_SUBJECT_", "ZPR_OPERATOR_DELEGATION_TRUST", "SIMULATOR"} {
		if strings.Contains(room, forbidden) {
			t.Fatalf("Control Room must not receive %s", forbidden)
		}
	}
	addresses := strings.Join(testEvaluationSubjectConfig().listenAddresses(), " ")
	if !strings.Contains(addresses, "127.0.0.1:18793") {
		t.Fatal("delivery port not reserved")
	}
}

func TestEvaluationDelegationTrustValidates(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install")
	config := testEvaluationSubjectConfig()
	inventory, err := prepareEvaluation(state, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	issuer := config.browserOrigin(config.OIDCPort)
	admin := operatorGrantIdentity{Subject: "admin-subject", Permissions: append([]string{"monitor.read"}, evaluationAdminSubjectLinkPermissions...)}
	reviewer := operatorGrantIdentity{Subject: "reviewer-subject", Permissions: evaluationReviewerPermissions}
	chain := readSubjectFile(t, state, "client-chain.pem")
	data, err := evaluationDelegationTrustFile(config, issuer, evaluationOrganization(inventory), readSubjectFile(t, state, "operator-delegation-key.pem"), chain, admin, reviewer)
	if err != nil {
		t.Fatal(err)
	}
	var trust operatordelegation.Config
	if err := json.Unmarshal(data, &trust); err != nil || trust.Validate() != nil || len(trust.Grants) != 2 || trust.Audience != "https://localhost:18790" {
		t.Fatalf("trust file invalid: %s", data)
	}
	block, _ := pem.Decode(chain)
	pin := sha256.Sum256(block.Bytes)
	public := ed25519PrivateFromPEM(t, readSubjectFile(t, state, "operator-delegation-key.pem")).Public().(ed25519.PublicKey)
	if trust.Keys[0].CertificateSHA256 != hex.EncodeToString(pin[:]) || trust.Keys[0].PublicKey != base64.RawURLEncoding.EncodeToString(public) {
		t.Fatal("trust does not bind the Room signer key to its client certificate")
	}
	for _, grant := range trust.Grants {
		if grant.Subject == "reviewer-subject" && (len(grant.Permissions) != 3 || strings.Contains(strings.Join(grant.Permissions, ","), "propose")) {
			t.Fatal("reviewer must hold only read and approve permissions")
		}
	}
	once := filepath.Join(state, "once.json")
	if err := writeEvaluationOnce(state, "once.json", data); err != nil || writeEvaluationOnce(state, "once.json", data) != nil {
		t.Fatal("identical identity binding refused")
	}
	if writeEvaluationOnce(state, "once.json", append(data, ' ')) == nil {
		t.Fatal("changed identity binding accepted")
	}
	if info, err := os.Stat(once); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("identity binding not private")
	}
}

func TestEvaluationDeliveryReadinessRequiresPin(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install")
	if _, err := prepareEvaluation(state, testEvaluationSubjectConfig(), time.Now()); err != nil {
		t.Fatal(err)
	}
	server, err := tls.LoadX509KeyPair(filepath.Join(state, "server-chain.pem"), filepath.Join(state, "server-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(readSubjectFile(t, state, "ca.pem"))
	serve := func(pinned bool) int {
		listener, err := tls.Listen("tcp4", "127.0.0.1:0", &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{server},
			ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool,
			VerifyConnection: func(state tls.ConnectionState) error {
				if !pinned {
					return errors.New("not pinned")
				}
				return nil
			}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { listener.Close() })
		go func() {
			for {
				connection, err := listener.Accept()
				if err != nil {
					return
				}
				go func() {
					defer connection.Close()
					_ = connection.(*tls.Conn).Handshake()
					time.Sleep(time.Second)
				}()
			}
		}()
		return listener.Addr().(*net.TCPAddr).Port
	}
	if err := checkEvaluationDelivery(state, serve(true)); err != nil {
		t.Fatalf("pinned VS client rejected: %v", err)
	}
	if err := checkEvaluationDelivery(state, serve(false)); err == nil {
		t.Fatal("unpinned delivery accepted as ready")
	}
}

func TestEvaluationSubjectLinkFlags(t *testing.T) {
	parent := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-evaluation", "-state", filepath.Join(parent, "a"), "-render-only", "-snapshot-delivery-port", "18794"}, &stdout, &stderr); code != 2 {
		t.Fatalf("delivery port without -visa-service: code %d", code)
	}
	if code := run([]string{"-evaluation", "-state", filepath.Join(parent, "b"), "-render-only", "-visa-service", "-snapshot-delivery-port", "18182"}, &stdout, &stderr); code != 1 {
		t.Fatalf("duplicate delivery port: code %d", code)
	}
	stderr.Reset()
	if code := run([]string{"-evaluation", "-state", filepath.Join(parent, "c"), "-render-only", "-visa-service"}, &stdout, &stderr); code != 0 {
		t.Fatalf("render with subject links: code %d: %s", code, stderr.String())
	}
	if _, err := readEvaluation(filepath.Join(parent, "c"), testEvaluationSubjectConfig()); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluationAdminRecoveryKeepsReviewer(t *testing.T) {
	config := testEvaluationSubjectConfig()
	state, _ := recoveryFixtureWith(t, config)
	before := readSubjectFile(t, state, "dex.json")
	var old struct {
		Passwords []map[string]string `json:"staticPasswords"`
	}
	if err := json.Unmarshal(before, &old); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"operator-oidc.json", evaluationDelegationTrust} {
		if err := os.WriteFile(filepath.Join(state, name), []byte("old grants"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := resetEvaluationAdmin(state, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var dex struct {
		Passwords []map[string]string `json:"staticPasswords"`
	}
	if err := json.Unmarshal(readSubjectFile(t, state, "dex.json"), &dex); err != nil || len(dex.Passwords) != 2 {
		t.Fatal("reviewer login lost")
	}
	if !reflect.DeepEqual(dex.Passwords[1], old.Passwords[1]) || dex.Passwords[0]["userID"] == old.Passwords[0]["userID"] {
		t.Fatal("recovery must rotate only the administrator")
	}
	for _, name := range []string{"operator-oidc.json", evaluationDelegationTrust} {
		if _, err := os.Stat(filepath.Join(state, name)); !os.IsNotExist(err) {
			t.Fatalf("old %s retained", name)
		}
	}
}
