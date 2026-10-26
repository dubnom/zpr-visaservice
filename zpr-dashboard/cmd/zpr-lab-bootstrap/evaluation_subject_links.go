package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Generated only with -visa-service: the EVALUATION-ONLY subject-link review,
// signing and pinned delivery configuration for the loopback VS fetcher.
var evaluationSubjectLinkFiles = []string{
	"subject-link-providers.json", "operator-delegation-signer.json", "operator-delegation-key.pem",
	"subject-snapshot-signing.json", "subject-snapshot-signing-key.pem", "vs-client-chain.pem", "vs-client-key.pem",
	"subject-snapshot-recipients.json", "operator-reviewer.secret", "SUBJECT-LINKS.txt",
}

const (
	evaluationSubjectSourceID   = "evaluation-directory"
	evaluationSubjectProviderID = "evaluation-ldap"
	evaluationSubjectVSID       = "evaluation-vs"
	evaluationSubjectEpoch      = "evaluation"
	evaluationDelegationKeyID   = "evaluation-room"
	evaluationSnapshotKeyID     = "evaluation-snapshot"
	evaluationReviewerEmail     = "reviewer@evaluation.invalid"
	// Runtime-written after both Dex subjects are known; refused if it changes.
	evaluationDelegationTrust = "operator-delegation-trust.json"
)

var evaluationAdminSubjectLinkPermissions = []string{
	"subject-link.read", "subject-link.propose", "subject-link.registry.manage", "subject-link.publish",
	"subject-link.snapshot.capture", "subject-link.snapshot.sign",
}

var evaluationReviewerPermissions = []string{"monitor.read", "subject-link.read", "subject-link.approve"}

func evaluationOrganization(inventory evaluationInventory) string {
	return "evaluation-" + inventory.InstallID
}

// evaluationSubjectUUID is a random lowercase RFC 4122 v4 UUID given to the
// synthetic LDAP subject as an explicit entryUUID so operators can link it.
func evaluationSubjectUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6], value[8] = value[6]&0x0f|0x40, value[8]&0x3f|0x80
	text := hex.EncodeToString(value)
	return text[:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:], nil
}

func evaluationSubjectLDIF(baseDN, uuid string) string {
	return fmt.Sprintf(`
dn: uid=evaluation-subject,ou=People,%s
objectClass: inetOrgPerson
uid: evaluation-subject
cn: Evaluation Subject
sn: Subject
mail: evaluation-subject@evaluation.invalid
entryUUID: %s
`, baseDN, uuid)
}

func evaluationReviewerPassword(secret []byte) (map[string]string, error) {
	hash, err := bcrypt.GenerateFromPassword(secret, bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("cannot protect evaluation reviewer credential")
	}
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	return map[string]string{"email": evaluationReviewerEmail, "hash": string(hash), "username": "Evaluation Reviewer", "userID": hex.EncodeToString(id)}, nil
}

func evaluationEd25519PEM() (ed25519.PublicKey, []byte, error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return nil, nil, err
	}
	return public, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func evaluationJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	return append(data, '\n'), err
}

func generateEvaluationSubjectLinks(files map[string][]byte, state string, inventory evaluationInventory, now time.Time, subjectUUID string, ca *x509.Certificate, caKey *ecdsa.PrivateKey) error {
	config := inventory.Config
	organization := evaluationOrganization(inventory)
	path := func(name string) string { return filepath.Join(state, name) }
	var err error
	if files["subject-link-providers.json"], err = evaluationJSON(map[string]any{"version": 1, "providers": []map[string]string{{
		"organization_id": organization, "source_id": evaluationSubjectSourceID, "source_version": "1", "provider_id": evaluationSubjectProviderID,
		"url": fmt.Sprintf("ldaps://localhost:%d", config.LDAPPort), "ca_file": path("ca.pem"), "base_dn": config.directoryBaseDN(),
		"bind_dn": config.directoryReaderDN(), "bind_password_file": path("ldap-reader.secret"),
	}}}); err != nil {
		return err
	}
	if files["operator-delegation-signer.json"], err = evaluationJSON(map[string]any{"version": 1, "audience": evaluationServiceAudience(config), "key_id": evaluationDelegationKeyID}); err != nil {
		return err
	}
	if _, files["operator-delegation-key.pem"], err = evaluationEd25519PEM(); err != nil {
		return err
	}
	public, signingKey, err := evaluationEd25519PEM()
	if err != nil {
		return err
	}
	files["subject-snapshot-signing-key.pem"] = signingKey
	if files["subject-snapshot-signing.json"], err = evaluationJSON(map[string]any{"version": 1, "key_id": evaluationSnapshotKeyID,
		"pins": []map[string]string{{"key_id": evaluationSnapshotKeyID, "public_key": base64.RawURLEncoding.EncodeToString(public)}}}); err != nil {
		return err
	}
	client, clientKey, clientPEM, clientKeyPEM, err := certificateWithOrganization("ZPR evaluation-only vs-client", "localhost", "ZPR EVALUATION ONLY", now, ca, caKey, x509.ExtKeyUsageClientAuth)
	if err != nil {
		return err
	}
	clientKey.D.SetInt64(0)
	files["vs-client-chain.pem"], files["vs-client-key.pem"] = clientPEM, clientKeyPEM
	spki := sha256.Sum256(client.RawSubjectPublicKeyInfo)
	if files["subject-snapshot-recipients.json"], err = evaluationJSON(map[string]any{"version": 1, "recipients": []map[string]string{{
		"organization_id": organization, "vs_id": evaluationSubjectVSID, "client_spki_sha256": hex.EncodeToString(spki[:]),
	}}}); err != nil {
		return err
	}
	quoted := func(name string) string {
		data, _ := json.Marshal(path(name))
		return string(data)
	}
	files["vs.toml"] = fmt.Appendf(files["vs.toml"], `
# EVALUATION ONLY pinned subject snapshot fetch. Fetched snapshots are
# verified and checkpointed only; nothing is enforced.
[subject_snapshot_fetch]
url = "https://localhost:%d"
ca_cert = %s
client_cert = %s
client_key = %s
organization_id = %q
vs_id = %q
epoch = %q
signing_pins_file = %s
checkpoint_dir = %s
poll_seconds = 15
`, config.SnapshotDeliveryPort, quoted("ca.pem"), quoted("vs-client-chain.pem"), quoted("vs-client-key.pem"), organization,
		evaluationSubjectVSID, evaluationSubjectEpoch, quoted("subject-snapshot-signing.json"), quoted(evaluationSubjectCheckpointDir(inventory.Config)))
	files["SUBJECT-LINKS.txt"] = fmt.Appendf(nil, `ZPR EVALUATION ONLY subject-link publication cycle

Organization ID:        %s
LDAP source ID:         %s (version 1, the only version the provider binds)
LDAP provider ID:       %s
Selector kind:          ldap_entry_uuid
Sample LDAP subject:    uid=evaluation-subject,ou=People,%s
Sample entryUUID:       %s
VS recipient ID:        %s
Snapshot epoch:         %s
Reviewer login:         %s (password in operator-reviewer.secret)

Administrator registers the BAS authority and the LDAP source, proposes a
link and captures/signs snapshots; the independent reviewer approves. After
the first signed snapshot, initialize the VS checkpoint once:

  <vs-binary> --initialize-subject-snapshot -c %s

The running VS then polls every 15 seconds and posts delivery receipts.
Editing the registered LDAP source advances its version and no longer
matches the provider; keep version 1 for this evaluation.
`, organization, evaluationSubjectSourceID, evaluationSubjectProviderID, config.directoryBaseDN(), subjectUUID, evaluationSubjectVSID, evaluationSubjectEpoch,
		evaluationReviewerEmail, path("vs.toml"))
	return nil
}

func evaluationServiceAudience(config evaluationConfig) string {
	return fmt.Sprintf("https://localhost:%d", config.ServicePort)
}

// evaluationDelegationTrustFile binds the Room signer key and its mTLS client
// certificate to the administrator and independent reviewer grants.
func evaluationDelegationTrustFile(config evaluationConfig, issuer, organization string, signerKey, clientChain []byte, admin, reviewer operatorGrantIdentity) ([]byte, error) {
	block, _ := pem.Decode(signerKey)
	if block == nil {
		return nil, errors.New("invalid evaluation delegation key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	private, ok := key.(ed25519.PrivateKey)
	if err != nil || !ok {
		return nil, errors.New("invalid evaluation delegation key")
	}
	leaf, _ := pem.Decode(clientChain)
	if leaf == nil || leaf.Type != "CERTIFICATE" {
		return nil, errors.New("invalid evaluation Room client certificate")
	}
	pin := sha256.Sum256(leaf.Bytes)
	grants := []map[string]any{
		{"issuer": issuer, "subject": admin.Subject, "organizations": []string{organization}, "permissions": admin.Permissions},
		{"issuer": issuer, "subject": reviewer.Subject, "organizations": []string{organization}, "permissions": reviewer.Permissions},
	}
	return evaluationJSON(map[string]any{"version": 1, "audience": evaluationServiceAudience(config), "keys": []map[string]string{{
		"key_id": evaluationDelegationKeyID, "public_key": base64.RawURLEncoding.EncodeToString(private.Public().(ed25519.PublicKey)),
		"certificate_sha256": hex.EncodeToString(pin[:]),
	}}, "grants": grants})
}

type operatorGrantIdentity struct {
	Subject     string
	Permissions []string
}

func evaluationSubjectEnvironment(state string, config evaluationConfig, room bool) []string {
	path := func(name string) string { return filepath.Join(state, name) }
	if room {
		return []string{"ZPR_OPERATOR_DELEGATION_SIGNER_FILE=" + path("operator-delegation-signer.json"), "ZPR_OPERATOR_DELEGATION_KEY_FILE=" + path("operator-delegation-key.pem")}
	}
	return []string{
		"ZPR_SUBJECT_LINK_REVIEW_DB_PATH=" + path("subject-link-review.db"),
		"ZPR_SUBJECT_LINK_PROVIDERS_FILE=" + path("subject-link-providers.json"),
		"ZPR_OPERATOR_DELEGATION_TRUST_FILE=" + path(evaluationDelegationTrust),
		"ZPR_SUBJECT_SNAPSHOT_SIGNING_FILE=" + path("subject-snapshot-signing.json"),
		"ZPR_SUBJECT_SNAPSHOT_SIGNING_KEY_FILE=" + path("subject-snapshot-signing-key.pem"),
		fmt.Sprintf("ZPR_SUBJECT_SNAPSHOT_DELIVERY_LISTEN=127.0.0.1:%d", config.SnapshotDeliveryPort),
		"ZPR_SUBJECT_SNAPSHOT_DELIVERY_CLIENT_CA_FILE=" + path("ca.pem"),
		"ZPR_SUBJECT_SNAPSHOT_RECIPIENTS_FILE=" + path("subject-snapshot-recipients.json"),
	}
}

func evaluationVSInitCommand(binary, state string) string {
	return fmt.Sprintf("%q --initialize-subject-snapshot -c %q", binary, filepath.Join(state, "vs.toml"))
}

// checkEvaluationDelivery completes a TLS 1.3 handshake with the VS client
// identity; the listener rejects unpinned certificates during the handshake.
func checkEvaluationDelivery(state string, port int) error {
	config, err := evaluationTLS(state, false)
	if err != nil {
		return err
	}
	certificate, err := tls.LoadX509KeyPair(filepath.Join(state, "vs-client-chain.pem"), filepath.Join(state, "vs-client-key.pem"))
	if err != nil {
		return errors.New("invalid VS delivery client identity")
	}
	config.MinVersion, config.Certificates = tls.VersionTLS13, []tls.Certificate{certificate}
	connection, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", fmt.Sprintf("127.0.0.1:%d", port), config)
	if err != nil {
		return errors.New("subject snapshot delivery handshake failed")
	}
	defer connection.Close()
	// TLS 1.3 reports client-certificate rejection after the handshake.
	_ = connection.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, err := connection.Read(make([]byte, 1)); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil
		}
		return errors.New("subject snapshot delivery rejected the pinned VS client")
	}
	return nil
}

// evaluationSubjectCheckpointDir is relative to the state directory. The VS
// creates it at initialization, so the data plane keeps it inside its
// writable data-plane mount rather than binding a not-yet-existing path.
func evaluationSubjectCheckpointDir(config evaluationConfig) string {
	if config.DataPlane {
		return filepath.Join("data-plane", "vs-subject-snapshot")
	}
	return "vs-subject-snapshot"
}
