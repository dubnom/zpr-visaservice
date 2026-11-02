package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

// Add-later registration of the separate Simulator OIDC client and its
// independent delegation signer on an existing stopped evaluation install.
// It preserves the frozen Room client, secrets, operators and recovery
// invariants; it never reseeds state, rebinds identities or starts Simulator.
const (
	evaluationSimulatorPairingPreparing = "simulator-pairing.preparing"
	evaluationSimulatorPairingPending   = "simulator-pairing.pending"
	evaluationSimulatorPairingSuffix    = ".simulator-pairing-new"
	evaluationSimulatorServicesFile     = "simulator-paired-operator-services.json"
	evaluationSimulatorTrustFile        = "simulator-paired-delegation-trust.json"
)

type evaluationSimulatorPairingPlan struct {
	Version              int               `json:"version"`
	SimulatorPort        int               `json:"simulator_port"`
	OperatorServicesFile string            `json:"operator_services_file"`
	SessionGrantsFile    string            `json:"session_grants_file,omitempty"`
	Files                map[string]string `json:"files"`
	OriginalFiles        map[string]string `json:"original_files"`
	OperatorInputs       map[string]string `json:"operator_inputs,omitempty"`
}

// evaluationSimulatorPairingFiles are the per-install Simulator registration
// artifacts frozen in the inventory whenever a Simulator port is configured,
// at creation or through add-later pairing.
var evaluationSimulatorPairingFiles = []string{
	"oidc-simulator-client.secret", "simulator-delegation-signer.json", "simulator-delegation-key.pem",
	"simulator-client-chain.pem", "simulator-client-key.pem",
}

func runEvaluationSimulatorPairing(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-lab-bootstrap -evaluation-simulator-pairing", flag.ContinueOnError)
	flags.SetOutput(stderr)
	state := flags.String("state", "", "existing stopped protected evaluation install directory (required)")
	port := flags.Int("simulator-port", 0, "separate Simulator HTTPS callback port to register (required)")
	services := flags.String("operator-services-file", "", "existing protected operator service bindings with a delegation trust override")
	sessionGrants := flags.String("session-grants-file", "", "optional protected composed Simulator session configuration whose named identities gain this installation's organization grant in the derived paired trust only (requires -operator-services-file)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *state == "" || *port == 0 || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: zpr-lab-bootstrap -evaluation-simulator-pairing -state /existing/install -simulator-port PORT")
		return 2
	}
	fmt.Fprintln(stdout, evaluationBanner)
	absolute, err := filepath.Abs(*state)
	if err != nil {
		fmt.Fprintln(stderr, "Invalid local state path.")
		return 1
	}
	if err := pairEvaluationSimulatorWithServices(absolute, *port, *services, *sessionGrants, time.Now().UTC()); err != nil {
		fmt.Fprintf(stderr, "Simulator pairing: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Separate Simulator OIDC client and independent delegation signer are registered for port %d.\nRestart the install with its original flags plus -simulator-port %d so OIDC serves the new client. Login alone changes nothing; production services never require Simulator.\n", *port, *port)
	if *services != "" {
		fmt.Fprintf(stdout, "Use -operator-services-file %s for the validated paired service bindings; the original bindings remain unchanged.\n", filepath.Join(absolute, evaluationSimulatorServicesFile))
	}
	return 0
}

// pairEvaluationSimulator registers the Simulator client/signer exactly once.
// The replacement is staged, recorded in a pending plan and then committed
// with atomic renames, so an interrupted run resumes instead of reseeding.
func pairEvaluationSimulator(state string, port int, now time.Time) error {
	return pairEvaluationSimulatorWithServices(state, port, "", "", now)
}

func pairEvaluationSimulatorWithServices(state string, port int, servicesFile, sessionFile string, now time.Time) error {
	if port == 0 {
		return errors.New("simulator-port is required for pairing")
	}
	if servicesFile != "" && (!filepath.IsAbs(servicesFile) || filepath.Clean(servicesFile) != servicesFile ||
		strings.ContainsAny(servicesFile, ",\"\r\n=")) {
		return errors.New("operator-services-file requires a canonical absolute bind-mount-safe path")
	}
	if sessionFile != "" {
		if servicesFile == "" {
			return errors.New("session-grants-file extends only the derived paired trust; it requires -operator-services-file")
		}
		if !filepath.IsAbs(sessionFile) || filepath.Clean(sessionFile) != sessionFile ||
			strings.ContainsAny(sessionFile, ",\"\r\n=") {
			return errors.New("session-grants-file requires a canonical absolute bind-mount-safe path")
		}
	}
	lock, err := lockEvaluation(state)
	if err != nil {
		return err
	}
	defer lock.Close()
	simulatorLock, err := lockEvaluationSimulator(state)
	if err != nil {
		return err
	}
	defer simulatorLock.Close()
	root, err := os.OpenRoot(state)
	if err != nil {
		return errors.New("cannot open protected state")
	}
	defer root.Close()
	if _, err := root.Lstat(evaluationSimulatorPairingPending); err == nil {
		return resumeEvaluationSimulatorPairing(root, port, servicesFile, sessionFile)
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot inspect pending pairing plan")
	}
	if _, err := root.Lstat(evaluationSimulatorPairingPreparing); err == nil {
		if err := recoverEvaluationSimulatorPreparation(root, port, servicesFile, sessionFile); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot inspect Simulator pairing preparation")
	}
	data, err := os.ReadFile(filepath.Join(state, "evaluation.json"))
	var inventory evaluationInventory
	if err != nil || json.Unmarshal(data, &inventory) != nil || inventory.Config.validate() != nil {
		return errors.New("invalid evaluation state; pairing never creates or reseeds an install")
	}
	inventory, err = readEvaluation(state, inventory.Config)
	if err != nil {
		return err
	}
	if inventory.Config.SimulatorPort == port {
		// readEvaluation already verified every registration artifact.
		if servicesFile != "" {
			return ensureEvaluationSimulatorServices(root, state, inventory, servicesFile, sessionFile)
		}
		return nil
	}
	if inventory.Config.SimulatorPort != 0 {
		return errors.New("install already registers a Simulator client on another port; pairing never rebinds or replaces it")
	}
	updated := inventory.Config
	updated.SimulatorPort = port
	if err := updated.validate(); err != nil {
		return err
	}
	release, err := reserveEvaluationSimulatorPairingPorts(inventory.Config, port)
	if err != nil {
		return err
	}
	defer release()
	files, err := planEvaluationSimulatorPairing(state, inventory, updated, now)
	if err != nil {
		return err
	}
	var operatorInputs map[string]string
	if servicesFile != "" {
		operatorInputs, err = snapshotEvaluationSimulatorOperatorInputs(servicesFile)
		if err != nil {
			return fmt.Errorf("snapshot original operator inputs: %w", err)
		}
		services, err := loadEvaluationOperatorServices(servicesFile, state, inventory)
		if err != nil {
			return fmt.Errorf("validate existing operator services: %w", err)
		}
		if services.OperatorDelegationTrustFile == "" {
			return errors.New("operator services have no delegation trust override; omit -operator-services-file")
		}
		sessionGrants, err := loadEvaluationSimulatorSessionGrants(sessionFile, services.Organization)
		if err != nil {
			return err
		}
		for _, name := range []string{evaluationSimulatorServicesFile, evaluationSimulatorTrustFile} {
			if _, err := root.Lstat(name); err == nil {
				return fmt.Errorf("paired operator services target %s already exists; inspect it before pairing", name)
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("cannot inspect paired operator services target %s", name)
			}
		}
		paired, err := renderEvaluationSimulatorServices(state, services,
			updated, files["simulator-delegation-key.pem"], files["simulator-client-chain.pem"], sessionGrants)
		if err != nil {
			return err
		}
		var base, override operatordelegation.Config
		if err := decodeEvaluationDelegationTrust(files[evaluationDelegationTrust], &base); err != nil {
			return fmt.Errorf("validate paired base delegation trust: %w", err)
		}
		if err := decodeEvaluationDelegationTrust(paired[evaluationSimulatorTrustFile], &override); err != nil {
			return fmt.Errorf("validate paired delegation override: %w", err)
		}
		if err := validateEvaluationDelegationOverride(override, base, services.Organization); err != nil {
			return fmt.Errorf("validate paired delegation override: %w", err)
		}
		for name, contents := range paired {
			files[name] = contents
		}
		currentInputs, err := snapshotEvaluationSimulatorOperatorInputs(servicesFile)
		if err != nil || !maps.Equal(operatorInputs, currentInputs) {
			return errors.New("operator inputs changed or became unavailable while preparing pairing; keep stopped and inspect them")
		}
		if err := validateEvaluationSimulatorPairingInputTargets(state, operatorInputs, files); err != nil {
			return err
		}
	}
	digests := map[string]string{}
	names := make([]string, 0, len(files))
	for name, contents := range files {
		sum := sha256.Sum256(contents)
		digests[name] = hex.EncodeToString(sum[:])
		names = append(names, name)
	}
	slices.Sort(names)
	original, err := captureEvaluationSimulatorPairingTargets(root, names)
	if err != nil {
		return err
	}
	plan := evaluationSimulatorPairingPlan{
		Version: 2, SimulatorPort: port, OperatorServicesFile: servicesFile, SessionGrantsFile: sessionFile,
		Files: digests, OriginalFiles: original, OperatorInputs: operatorInputs,
	}
	marker, err := evaluationJSON(plan)
	if err != nil {
		return errors.New("cannot render pairing plan")
	}
	if err := writePrivate(root, evaluationSimulatorPairingPreparing, marker); err != nil {
		return errors.New("cannot record pairing preparation; keep stopped and inspect state")
	}
	if err := syncRecoveryDirectory(root); err != nil {
		return err
	}
	for _, name := range names {
		if err := writePrivate(root, name+evaluationSimulatorPairingSuffix, files[name]); err != nil {
			return errors.New("pairing staging failed; keep stopped and rerun pairing with the same arguments")
		}
	}
	if err := syncRecoveryDirectory(root); err != nil {
		return err
	}
	if err := writePrivate(root, evaluationSimulatorPairingPending, marker); err != nil {
		return errors.New("cannot record pairing plan; inspect staged state before retrying")
	}
	if err := syncRecoveryDirectory(root); err != nil {
		return err
	}
	staged, _, err := validateEvaluationSimulatorPairingCommit(root, plan)
	if err != nil {
		return err
	}
	return commitEvaluationSimulatorPairing(root, plan, staged)
}

func reserveEvaluationSimulatorPairingPorts(config evaluationConfig, port int) (func(), error) {
	release, err := reserveRecoveryPorts(config)
	if err != nil {
		return nil, err
	}
	callback, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		release()
		return nil, errors.New("requested Simulator port is occupied; stop its process or choose another port")
	}
	return func() {
		callback.Close()
		release()
	}, nil
}

func snapshotEvaluationSimulatorOperatorInputs(servicesFile string) (map[string]string, error) {
	read := func(path string) ([]byte, error) {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, ",\"\r\n=") {
			return nil, errors.New("operator inputs require canonical absolute bind-mount-safe paths")
		}
		data, err := servicehttp.ReadOperatorFile(path, 1<<20, true)
		if err != nil {
			return nil, fmt.Errorf("read protected operator input %s: %w", path, err)
		}
		return data, nil
	}
	data, err := read(servicesFile)
	if err != nil {
		return nil, err
	}
	var services evaluationOperatorServices
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&services); err != nil || decoder.Decode(new(any)) != io.EOF ||
		services.Version != 1 || services.OperatorDelegationTrustFile == "" {
		return nil, errors.New("original operator services are invalid or lack a delegation trust override")
	}
	inputs := map[string]string{servicesFile: evaluationPairingDigest(data)}
	paths := []string{
		services.OperatorDelegationTrustFile, services.OperatorOIDCConfigFile,
		services.ChangeReviewConfigFile, services.VisaServiceConfigFile,
	}
	paths = append(paths, services.VisaServiceReadOnlyFiles...)
	for _, binding := range []*evaluationMutualTLSServiceBinding{services.PolicyService, services.EnrollmentService} {
		if binding != nil {
			paths = append(paths, binding.ClientCertificateFile, binding.ClientKeyFile, binding.CAFile)
		}
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, ok := inputs[path]; ok {
			continue
		}
		data, err := read(path)
		if err != nil {
			return nil, err
		}
		inputs[path] = evaluationPairingDigest(data)
	}
	return inputs, nil
}

func validateEvaluationSimulatorPairingInputTargets(state string, inputs map[string]string, files map[string][]byte) error {
	targets := make(map[string]bool, len(files))
	for name := range files {
		targets[filepath.Join(state, name)] = true
	}
	for path := range inputs {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("cannot resolve protected operator input %s", path)
		}
		if targets[resolved] {
			return fmt.Errorf("operator input %s is also a pairing output; supply independent protected inputs", path)
		}
	}
	return nil
}

func evaluationSimulatorPairingReplacement(name string) bool {
	return name == "dex.json" || name == "evaluation.json" || name == evaluationDelegationTrust
}

func captureEvaluationSimulatorPairingTargets(root *os.Root, names []string) (map[string]string, error) {
	original := map[string]string{}
	for _, name := range names {
		if _, err := root.Lstat(name + evaluationSimulatorPairingSuffix); err == nil {
			return nil, fmt.Errorf("unrecorded staged pairing file %s exists; inspect it without removing unrelated state", name+evaluationSimulatorPairingSuffix)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("cannot inspect staged pairing target %s", name)
		}
		_, err := root.Lstat(name)
		if !evaluationSimulatorPairingReplacement(name) {
			if err == nil {
				return nil, fmt.Errorf("pairing target %s already exists; inspect it before pairing", name)
			}
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("cannot inspect pairing target %s", name)
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("retained pairing target %s is unavailable", name)
		}
		data, err := root.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("cannot read retained pairing target %s", name)
		}
		original[name] = evaluationPairingDigest(data)
	}
	return original, nil
}

// renderEvaluationSimulatorServices snapshots the explicitly supplied
// operator-owned bindings without changing their source files or grants; the
// optional composed-session grants exist only in the derived paired trust.
func renderEvaluationSimulatorServices(state string, original *evaluationOperatorServices, config evaluationConfig, simulatorKey, simulatorChain []byte, sessionGrants []operatorauth.Grant) (map[string][]byte, error) {
	trust, err := servicehttp.ReadOperatorFile(original.OperatorDelegationTrustFile, 1<<20, true)
	if err != nil {
		return nil, fmt.Errorf("read operator delegation override for pairing: %w", err)
	}
	extended, err := extendEvaluationDelegationTrust(trust, config, simulatorKey, simulatorChain, sessionGrants)
	if err != nil {
		return nil, fmt.Errorf("extend operator delegation override: %w", err)
	}
	paired := *original
	paired.OperatorDelegationTrustFile = filepath.Join(state, evaluationSimulatorTrustFile)
	services, err := json.MarshalIndent(paired, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		evaluationSimulatorServicesFile: append(services, '\n'),
		evaluationSimulatorTrustFile:    extended,
	}, nil
}

// ensureEvaluationSimulatorServices verifies the derived paired service
// bindings on an already-paired install, deriving them first when the install
// registered its Simulator client without them. Derivation uses only the
// registered pairing artifacts — no key is created, rotated or rebound — and
// writes exactly the content the verification re-derives, through staged
// private files; partial presence fails closed for inspection.
func ensureEvaluationSimulatorServices(root *os.Root, state string, inventory evaluationInventory, servicesFile, sessionFile string) error {
	_, servicesErr := root.Lstat(evaluationSimulatorServicesFile)
	_, trustErr := root.Lstat(evaluationSimulatorTrustFile)
	servicesMissing := errors.Is(servicesErr, os.ErrNotExist)
	trustMissing := errors.Is(trustErr, os.ErrNotExist)
	if (servicesErr != nil && !servicesMissing) || (trustErr != nil && !trustMissing) {
		return errors.New("cannot inspect paired operator service targets")
	}
	if servicesMissing != trustMissing {
		return errors.New("paired operator service targets are partially present; inspect them before retrying")
	}
	if !servicesMissing {
		return verifyEvaluationSimulatorServices(state, inventory, servicesFile, sessionFile)
	}
	data, err := servicehttp.ReadOperatorFile(servicesFile, 1<<20, true)
	if err != nil {
		return fmt.Errorf("read original operator services: %w", err)
	}
	var original evaluationOperatorServices
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&original); err != nil || decoder.Decode(new(any)) != io.EOF ||
		original.OperatorDelegationTrustFile == "" {
		return errors.New("original operator services are invalid or lack a delegation trust override")
	}
	key, err := servicehttp.ReadOperatorFile(filepath.Join(state, "simulator-delegation-key.pem"), 1<<20, true)
	if err != nil {
		return fmt.Errorf("read registered Simulator delegation key: %w", err)
	}
	chain, err := servicehttp.ReadOperatorFile(filepath.Join(state, "simulator-client-chain.pem"), 1<<20, true)
	if err != nil {
		return fmt.Errorf("read registered Simulator client certificate: %w", err)
	}
	sessionGrants, err := loadEvaluationSimulatorSessionGrants(sessionFile, original.Organization)
	if err != nil {
		return err
	}
	paired, err := renderEvaluationSimulatorServices(state, &original, inventory.Config, key, chain, sessionGrants)
	if err != nil {
		return err
	}
	baseData, err := servicehttp.ReadOperatorFile(filepath.Join(state, evaluationDelegationTrust), 1<<20, true)
	if err != nil {
		return fmt.Errorf("read registered base delegation trust: %w", err)
	}
	var base, override operatordelegation.Config
	if err := decodeEvaluationDelegationTrust(baseData, &base); err != nil {
		return fmt.Errorf("validate paired base delegation trust: %w", err)
	}
	if err := decodeEvaluationDelegationTrust(paired[evaluationSimulatorTrustFile], &override); err != nil {
		return fmt.Errorf("validate paired delegation override: %w", err)
	}
	if err := validateEvaluationDelegationOverride(override, base, original.Organization); err != nil {
		return fmt.Errorf("validate paired delegation override: %w", err)
	}
	names := []string{evaluationSimulatorTrustFile, evaluationSimulatorServicesFile}
	for _, name := range names {
		if err := writePrivate(root, name+".services-new", paired[name]); err != nil {
			return errors.New("paired operator services staging failed; rerun with the same arguments")
		}
	}
	if err := syncRecoveryDirectory(root); err != nil {
		return err
	}
	for _, name := range names {
		if err := root.Rename(name+".services-new", name); err != nil {
			return errors.New("paired operator services replacement failed; rerun pairing with the same state and port to complete it")
		}
	}
	if err := syncRecoveryDirectory(root); err != nil {
		return err
	}
	return verifyEvaluationSimulatorServices(state, inventory, servicesFile, sessionFile)
}

func verifyEvaluationSimulatorServices(state string, inventory evaluationInventory, servicesFile, sessionFile string) error {
	pairedPath := filepath.Join(state, evaluationSimulatorServicesFile)
	paired, err := loadEvaluationOperatorServices(pairedPath, state, inventory)
	if err != nil {
		return fmt.Errorf("validate paired operator services: %w", err)
	}
	if paired.OperatorDelegationTrustFile != filepath.Join(state, evaluationSimulatorTrustFile) {
		return errors.New("paired operator services do not reference the recorded trust file")
	}
	data, err := servicehttp.ReadOperatorFile(servicesFile, 1<<20, true)
	if err != nil {
		return fmt.Errorf("read original operator services: %w", err)
	}
	var original evaluationOperatorServices
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&original); err != nil || decoder.Decode(new(any)) != io.EOF ||
		original.OperatorDelegationTrustFile == "" {
		return errors.New("original operator services are invalid or lack a delegation trust override")
	}
	key, err := servicehttp.ReadOperatorFile(filepath.Join(state, "simulator-delegation-key.pem"), 1<<20, true)
	if err != nil {
		return fmt.Errorf("read paired Simulator delegation key: %w", err)
	}
	chain, err := servicehttp.ReadOperatorFile(filepath.Join(state, "simulator-client-chain.pem"), 1<<20, true)
	if err != nil {
		return fmt.Errorf("read paired Simulator client certificate: %w", err)
	}
	sessionGrants, err := loadEvaluationSimulatorSessionGrants(sessionFile, original.Organization)
	if err != nil {
		return err
	}
	expected, err := renderEvaluationSimulatorServices(state, &original, inventory.Config, key, chain, sessionGrants)
	if err != nil {
		return err
	}
	for name, contents := range expected {
		actual, err := servicehttp.ReadOperatorFile(filepath.Join(state, name), 1<<20, true)
		if err != nil || !bytes.Equal(actual, contents) {
			return fmt.Errorf("paired operator services %s differ from the original bindings; inspect before retrying", name)
		}
	}
	return nil
}

// planEvaluationSimulatorPairing renders every replaced or added protected
// file, including the updated frozen inventory, without touching the state.
func planEvaluationSimulatorPairing(state string, inventory evaluationInventory, updated evaluationConfig, now time.Time) (map[string][]byte, error) {
	files := map[string][]byte{}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	files["oidc-simulator-client.secret"] = []byte(base64.RawURLEncoding.EncodeToString(secret))
	dexData, err := os.ReadFile(filepath.Join(state, "dex.json"))
	var dex struct {
		Passwords []map[string]string `json:"staticPasswords"`
		Clients   []map[string]any    `json:"staticClients"`
	}
	var document map[string]json.RawMessage
	passwords := 1
	if updated.subjectLinks() {
		passwords = 2
	}
	if err != nil || json.Unmarshal(dexData, &dex) != nil || json.Unmarshal(dexData, &document) != nil ||
		len(dex.Passwords) != passwords || len(dex.Clients) != 1 ||
		dex.Clients[0]["id"] != "zpr-control-room" || dex.Passwords[0]["email"] != "admin@evaluation.invalid" ||
		(passwords == 2 && dex.Passwords[1]["email"] != evaluationReviewerEmail) {
		return nil, errors.New("pairing supports only the bundled Control Room Dex configuration")
	}
	dex.Clients = append(dex.Clients, map[string]any{
		"id": "zpr-simulator", "name": "ZPR Evaluation Simulator",
		"secret":       string(files["oidc-simulator-client.secret"]),
		"redirectURIs": []string{updated.browserOrigin(updated.SimulatorPort) + "/auth/operator/callback"},
	})
	if document["staticClients"], err = json.Marshal(dex.Clients); err != nil {
		return nil, err
	}
	rendered, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	files["dex.json"] = append(rendered, '\n')
	caPEM, err := os.ReadFile(filepath.Join(state, "ca.pem"))
	if err != nil {
		return nil, errors.New("cannot read install CA certificate")
	}
	caKeyPEM, err := os.ReadFile(filepath.Join(state, "ca-key.pem"))
	if err != nil {
		return nil, errors.New("cannot read private install CA key")
	}
	block := firstPEMCertificate(caPEM)
	if block == nil {
		return nil, errors.New("invalid install CA certificate")
	}
	ca, err := x509.ParseCertificate(block)
	if err != nil {
		return nil, errors.New("invalid install CA certificate")
	}
	caKey, err := parseCAKey(caKeyPEM)
	if err != nil {
		return nil, errors.New("invalid install CA key")
	}
	addresses := []net.IP{net.ParseIP("127.0.0.1")}
	if updated.BrowserIP != "" {
		addresses = append(addresses, net.ParseIP(updated.BrowserIP))
	}
	if err := generateEvaluationSimulatorSigner(files, updated, now, caPEM, ca, caKey, addresses); err != nil {
		return nil, err
	}
	// Extend the runtime-written delegation trust, when present, with only the
	// new independent key so the next verified-identity rendering matches it.
	trustData, err := os.ReadFile(filepath.Join(state, evaluationDelegationTrust))
	if err == nil {
		extended, err := extendEvaluationDelegationTrust(trustData, updated,
			files["simulator-delegation-key.pem"], files["simulator-client-chain.pem"], nil)
		if err != nil {
			return nil, err
		}
		files[evaluationDelegationTrust] = extended
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("cannot read operator delegation trust")
	}
	inventory.Config = updated
	frozen := map[string]string{}
	for name, digest := range inventory.Files {
		frozen[name] = digest
	}
	inventory.Files = frozen
	for name, contents := range files {
		if name == evaluationDelegationTrust {
			continue // runtime-written grants stay out of the frozen inventory
		}
		sum := sha256.Sum256(contents)
		inventory.Files[name] = hex.EncodeToString(sum[:])
	}
	data, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return nil, errors.New("cannot render pairing inventory")
	}
	files["evaluation.json"] = append(data, '\n')
	return files, nil
}

// generateEvaluationSimulatorSigner creates the independent Simulator
// delegation signer and its own CA-issued mTLS transport identity. Neither
// the Room signing key nor the Room client certificate is reused.
func generateEvaluationSimulatorSigner(files map[string][]byte, config evaluationConfig, now time.Time, caPEM []byte, ca *x509.Certificate, caKey *ecdsa.PrivateKey, addresses []net.IP) error {
	signer, err := evaluationJSON(map[string]any{"version": 1, "audience": evaluationServiceAudience(config), "key_id": evaluationSimulatorDelegationKeyID})
	if err != nil {
		return err
	}
	files["simulator-delegation-signer.json"] = signer
	if _, files["simulator-delegation-key.pem"], err = evaluationEd25519PEM(); err != nil {
		return err
	}
	_, leafKey, leaf, keyPEM, err := certificateWithOrganization("ZPR evaluation-only simulator-client", "localhost", "ZPR EVALUATION ONLY", now, ca, caKey, x509.ExtKeyUsageClientAuth, addresses...)
	if err != nil {
		return err
	}
	leafKey.D.SetInt64(0)
	files["simulator-client-chain.pem"], files["simulator-client-key.pem"] = append(leaf, caPEM...), keyPEM
	return nil
}

// extendEvaluationDelegationTrust appends the independent Simulator key to an
// existing runtime-written trust document while preserving the retained
// audience, Room key and exact named grants byte for byte. Optional
// composed-session grants are appended after the retained grants; they never
// replace or reorder a retained identity.
func extendEvaluationDelegationTrust(trustData []byte, config evaluationConfig, simulatorKey, simulatorChain []byte, sessionGrants []operatorauth.Grant) ([]byte, error) {
	var document map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(trustData))
	if err := decoder.Decode(&document); err != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("operator delegation trust requires one JSON object")
	}
	var audience string
	if json.Unmarshal(document["audience"], &audience) != nil || audience != evaluationServiceAudience(config) {
		return nil, errors.New("operator delegation trust audience differs from this installation")
	}
	var keys []map[string]string
	if json.Unmarshal(document["keys"], &keys) != nil || len(keys) != 1 || keys[0]["key_id"] != evaluationDelegationKeyID {
		return nil, errors.New("operator delegation trust must contain exactly the retained Room key before pairing")
	}
	entry, err := evaluationDelegationKeyEntry(evaluationSimulatorDelegationKeyID, simulatorKey, simulatorChain)
	if err != nil {
		return nil, err
	}
	keys = append(keys, entry)
	if document["keys"], err = json.Marshal(keys); err != nil {
		return nil, err
	}
	if len(sessionGrants) != 0 {
		var grants []json.RawMessage
		if json.Unmarshal(document["grants"], &grants) != nil || len(grants) == 0 {
			return nil, errors.New("operator delegation trust must contain its retained named grants")
		}
		retained := map[string]bool{}
		for _, raw := range grants {
			var grant operatorauth.Grant
			if json.Unmarshal(raw, &grant) != nil || grant.Issuer == "" || grant.Subject == "" {
				return nil, errors.New("operator delegation trust contains an invalid retained grant")
			}
			retained[grant.Issuer+"\x00"+grant.Subject] = true
		}
		for _, grant := range sessionGrants {
			if retained[grant.Issuer+"\x00"+grant.Subject] {
				return nil, errors.New("composed session identity already holds a retained named grant; refuse the ambiguous extension")
			}
			retained[grant.Issuer+"\x00"+grant.Subject] = true
			rendered, err := json.Marshal(grant)
			if err != nil {
				return nil, err
			}
			grants = append(grants, rendered)
		}
		if document["grants"], err = json.Marshal(grants); err != nil {
			return nil, err
		}
	}
	rendered, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(rendered, '\n'), nil
}

// loadEvaluationSimulatorSessionGrants derives the delegation-trust grants a
// composed multi-organization Simulator session needs on this installation:
// each named identity in the verified session configuration is granted
// exactly this installation's organization with its session permissions.
// The grants exist only in the derived paired trust; the installation's own
// runtime-written trust and frozen identities never change, and the load-time
// override validation independently bounds the added permissions.
func loadEvaluationSimulatorSessionGrants(path, organization string) ([]operatorauth.Grant, error) {
	if path == "" {
		return nil, nil
	}
	data, err := servicehttp.ReadOperatorFile(path, 65536, true)
	if err != nil {
		return nil, fmt.Errorf("read protected composed session configuration: %w", err)
	}
	var config operatorauth.Config
	if err := decodeEvaluationSimulatorOIDC(data, &config); err != nil {
		return nil, fmt.Errorf("composed session configuration: %w", err)
	}
	if organization == "" {
		return nil, errors.New("composed session grants require the operator services organization")
	}
	grants := make([]operatorauth.Grant, 0, len(config.Grants))
	for _, grant := range config.Grants {
		if !slices.Contains(grant.Organizations, organization) {
			return nil, fmt.Errorf("composed session grant for %q does not span organization %q; compose the members session first", grant.Subject, organization)
		}
		grants = append(grants, operatorauth.Grant{
			Issuer: grant.Issuer, Subject: grant.Subject,
			Organizations: []string{organization}, Permissions: slices.Clone(grant.Permissions),
		})
	}
	return grants, nil
}

func readEvaluationSimulatorPairingPlan(root *os.Root, name string, port int, servicesFile, sessionFile string) (evaluationSimulatorPairingPlan, error) {
	var marker evaluationSimulatorPairingPlan
	data, err := root.ReadFile(name)
	if err != nil {
		return marker, errors.New("cannot read pairing plan; keep stopped and inspect the offline backup")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&marker); err != nil || decoder.Decode(new(any)) != io.EOF ||
		marker.Version != 2 || len(marker.Files) == 0 {
		return marker, errors.New("invalid or legacy pairing plan without preservation evidence; keep stopped and inspect the offline backup")
	}
	if marker.SimulatorPort != port {
		return marker, errors.New("pending pairing records a different simulator-port; rerun with that exact port or restore the offline backup")
	}
	if marker.OperatorServicesFile != servicesFile {
		return marker, errors.New("pending pairing records a different operator-services-file; rerun with the original flag or restore the offline backup")
	}
	if marker.SessionGrantsFile != sessionFile {
		return marker, errors.New("pending pairing records a different session-grants-file; rerun with the original flag or restore the offline backup")
	}
	allowed := append([]string{"dex.json", "evaluation.json", evaluationDelegationTrust}, evaluationSimulatorPairingFiles...)
	required := append([]string{"dex.json", "evaluation.json"}, evaluationSimulatorPairingFiles...)
	if servicesFile != "" {
		allowed = append(allowed, evaluationSimulatorServicesFile, evaluationSimulatorTrustFile)
		required = append(required, evaluationDelegationTrust, evaluationSimulatorServicesFile, evaluationSimulatorTrustFile)
		if len(marker.OperatorInputs) == 0 {
			return marker, errors.New("pairing plan lacks recorded operator inputs; restore the complete offline backup")
		}
	} else if len(marker.OperatorInputs) != 0 {
		return marker, errors.New("pairing plan contains unexpected operator inputs; restore the complete offline backup")
	}
	for name, digest := range marker.Files {
		if !slices.Contains(allowed, name) || !validEvaluationPairingDigest(digest) {
			return marker, errors.New("pairing plan contains an invalid target or digest; restore the complete offline backup")
		}
	}
	for _, name := range required {
		if _, ok := marker.Files[name]; !ok {
			return marker, errors.New("pairing plan is missing a required target; restore the complete offline backup")
		}
	}
	for name, digest := range marker.OriginalFiles {
		if _, ok := marker.Files[name]; !ok || !evaluationSimulatorPairingReplacement(name) || !validEvaluationPairingDigest(digest) {
			return marker, errors.New("pairing plan contains an invalid retained target; restore the complete offline backup")
		}
	}
	for name := range marker.Files {
		if evaluationSimulatorPairingReplacement(name) && marker.OriginalFiles[name] == "" {
			return marker, errors.New("pairing plan lacks a retained target fingerprint; restore the complete offline backup")
		}
	}
	return marker, nil
}

func validEvaluationPairingDigest(digest string) bool {
	if len(digest) != sha256.Size*2 || digest != strings.ToLower(digest) {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func evaluationSimulatorPairingNames(plan evaluationSimulatorPairingPlan) []string {
	names := make([]string, 0, len(plan.Files))
	for name := range plan.Files {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func verifyEvaluationSimulatorPairingInputs(plan evaluationSimulatorPairingPlan) error {
	if plan.OperatorServicesFile == "" {
		return nil
	}
	current, err := snapshotEvaluationSimulatorOperatorInputs(plan.OperatorServicesFile)
	if err != nil {
		return fmt.Errorf("cannot verify original operator inputs; keep stopped: %w", err)
	}
	if !maps.Equal(current, plan.OperatorInputs) {
		return errors.New("original operator inputs changed during pairing; keep stopped and restore the recorded inputs before retrying")
	}
	return nil
}

func verifyEvaluationSimulatorPairingTargets(root *os.Root, plan evaluationSimulatorPairingPlan, names []string) error {
	for _, name := range names {
		original, existed := plan.OriginalFiles[name]
		_, err := root.Lstat(name)
		if !existed {
			if err == nil {
				return fmt.Errorf("pairing target %s appeared after staging; keep stopped and inspect it", name)
			}
			if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("cannot inspect pairing target %s", name)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("retained pairing target %s is unavailable; keep stopped", name)
		}
		data, err := root.ReadFile(name)
		if err != nil || evaluationPairingDigest(data) != original {
			return fmt.Errorf("retained pairing target %s changed after staging; keep stopped and inspect the offline backup", name)
		}
	}
	return nil
}

func recoverEvaluationSimulatorPreparation(root *os.Root, port int, servicesFile, sessionFile string) error {
	plan, err := readEvaluationSimulatorPairingPlan(root, evaluationSimulatorPairingPreparing, port, servicesFile, sessionFile)
	if err != nil {
		return err
	}
	if _, err := root.Lstat("admin-recovery.pending"); !errors.Is(err, os.ErrNotExist) {
		return errors.New("admin recovery is interrupted; keep stopped and inspect the offline backup")
	}
	data, err := root.ReadFile("evaluation.json")
	var inventory evaluationInventory
	if err != nil || json.Unmarshal(data, &inventory) != nil || inventory.Config.SimulatorPort != 0 ||
		inventory.Config.validate() != nil {
		return errors.New("original evaluation inventory is unavailable during pairing preparation; keep stopped")
	}
	if _, err := validateEvaluationInventory(root.Name(), inventory.Config); err != nil {
		return fmt.Errorf("verify original installation before pairing recovery: %w", err)
	}
	updated := inventory.Config
	updated.SimulatorPort = port
	if err := updated.validate(); err != nil {
		return err
	}
	release, err := reserveEvaluationSimulatorPairingPorts(inventory.Config, port)
	if err != nil {
		return err
	}
	defer release()
	if err := verifyEvaluationSimulatorPairingInputs(plan); err != nil {
		return err
	}
	names := evaluationSimulatorPairingNames(plan)
	if err := verifyEvaluationSimulatorPairingTargets(root, plan, names); err != nil {
		return err
	}
	staged := []string{}
	for _, name := range names {
		contents, err := root.ReadFile(name + evaluationSimulatorPairingSuffix)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || evaluationPairingDigest(contents) != plan.Files[name] {
			return fmt.Errorf("staged pairing file %s differs from its preparation plan; keep stopped and inspect it", name)
		}
		staged = append(staged, name)
	}
	for _, name := range staged {
		if err := root.Remove(name + evaluationSimulatorPairingSuffix); err != nil {
			return errors.New("cannot retire recorded pairing preparation files; keep stopped and retry")
		}
	}
	if err := syncRecoveryDirectory(root); err != nil {
		return err
	}
	if err := root.Remove(evaluationSimulatorPairingPreparing); err != nil {
		return errors.New("cannot retire pairing preparation; keep stopped and retry")
	}
	return syncRecoveryDirectory(root)
}

func validateEvaluationSimulatorPairingCommit(root *os.Root, plan evaluationSimulatorPairingPlan) ([]string, evaluationConfig, error) {
	var empty evaluationConfig
	contents := make(map[string][]byte, len(plan.Files))
	staged := []string{}
	for _, name := range evaluationSimulatorPairingNames(plan) {
		data, err := root.ReadFile(name + evaluationSimulatorPairingSuffix)
		if err == nil {
			staged = append(staged, name)
		} else if errors.Is(err, os.ErrNotExist) {
			data, err = root.ReadFile(name)
		}
		if err != nil || evaluationPairingDigest(data) != plan.Files[name] {
			return nil, empty, fmt.Errorf("pairing target %s lost or differs from its recorded content; keep stopped and inspect the offline backup", name)
		}
		contents[name] = data
	}
	var inventory evaluationInventory
	if json.Unmarshal(contents["evaluation.json"], &inventory) != nil || inventory.Version != 1 ||
		inventory.Kind != "zpr-evaluation-only" || len(inventory.InstallID) != 24 ||
		inventory.Config.SimulatorPort != plan.SimulatorPort || inventory.Config.validate() != nil {
		return nil, empty, errors.New("invalid planned pairing inventory; keep stopped and inspect the offline backup")
	}
	if _, err := hex.DecodeString(inventory.InstallID); err != nil {
		return nil, empty, errors.New("invalid planned pairing installation identity")
	}
	required := evaluationRequiredFiles(inventory.Config)
	if len(inventory.Files) != len(required) {
		return nil, empty, errors.New("planned pairing inventory is missing required files")
	}
	for _, name := range required {
		expected, ok := inventory.Files[name]
		if !ok {
			return nil, empty, errors.New("planned pairing inventory is missing required files")
		}
		data, planned := contents[name]
		if !planned {
			var err error
			data, err = root.ReadFile(name)
			if err != nil {
				return nil, empty, fmt.Errorf("retained protected file %s is unavailable; keep stopped", name)
			}
		}
		if evaluationPairingDigest(data) != expected {
			return nil, empty, fmt.Errorf("protected file %s differs from the planned inventory; keep stopped", name)
		}
	}
	for _, name := range []string{"assertion-sources.json", "adapter-logs.json"} {
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return nil, empty, errors.New("operator registry missing or unprotected; keep stopped")
		}
	}
	if err := verifyEvaluationSimulatorPairingInputs(plan); err != nil {
		return nil, empty, err
	}
	if err := verifyEvaluationSimulatorPairingTargets(root, plan, staged); err != nil {
		return nil, empty, err
	}
	return staged, inventory.Config, nil
}

// resumeEvaluationSimulatorPairing finishes only the recorded, validated plan.
func resumeEvaluationSimulatorPairing(root *os.Root, port int, servicesFile, sessionFile string) error {
	plan, err := readEvaluationSimulatorPairingPlan(root, evaluationSimulatorPairingPending, port, servicesFile, sessionFile)
	if err != nil {
		return err
	}
	_, config, err := validateEvaluationSimulatorPairingCommit(root, plan)
	if err != nil {
		return err
	}
	release, err := reserveEvaluationSimulatorPairingPorts(config, port)
	if err != nil {
		return err
	}
	defer release()
	staged, _, err := validateEvaluationSimulatorPairingCommit(root, plan)
	if err != nil {
		return err
	}
	return commitEvaluationSimulatorPairing(root, plan, staged)
}

func commitEvaluationSimulatorPairing(root *os.Root, plan evaluationSimulatorPairingPlan, staged []string) error {
	pending, err := root.ReadFile(evaluationSimulatorPairingPending)
	if err != nil {
		return errors.New("cannot read pending pairing plan; keep stopped")
	}
	preparing, err := root.ReadFile(evaluationSimulatorPairingPreparing)
	hasPreparation := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot inspect pairing preparation; keep stopped")
	}
	if hasPreparation && !bytes.Equal(preparing, pending) {
		return errors.New("pairing preparation and commit plans differ; keep stopped and inspect the offline backup")
	}
	if err := verifyEvaluationSimulatorPairingInputs(plan); err != nil {
		return err
	}
	if err := verifyEvaluationSimulatorPairingTargets(root, plan, staged); err != nil {
		return err
	}
	for _, name := range staged {
		if err := root.Rename(name+evaluationSimulatorPairingSuffix, name); err != nil {
			return errors.New("pairing replacement failed; rerun pairing with the same state and port to complete it")
		}
	}
	if err := syncRecoveryDirectory(root); err != nil {
		return err
	}
	if hasPreparation {
		if err := root.Remove(evaluationSimulatorPairingPreparing); err != nil {
			return errors.New("cannot retire pairing preparation; keep stopped and rerun pairing")
		}
		if err := syncRecoveryDirectory(root); err != nil {
			return err
		}
	}
	if err := root.Remove(evaluationSimulatorPairingPending); err != nil {
		return errors.New("cannot retire the pairing plan; rerun pairing with the same state and port")
	}
	return syncRecoveryDirectory(root)
}

func evaluationPairingDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
