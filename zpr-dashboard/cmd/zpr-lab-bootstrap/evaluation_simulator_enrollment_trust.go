package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

const (
	// evaluationSimulatorEnrollmentTrustFile is the derived delegation trust
	// for the separate Enrollment Service administration listener. It adds
	// only the already-registered independent Simulator signing key; the
	// operator-owned original trust file is never modified.
	evaluationSimulatorEnrollmentTrustFile    = "simulator-paired-enrollment-trust.json"
	evaluationSimulatorEnrollmentTrustStaging = evaluationSimulatorEnrollmentTrustFile + ".enrollment-trust-new"
)

func runEvaluationSimulatorEnrollmentTrust(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-lab-bootstrap -evaluation-simulator-enrollment-trust", flag.ContinueOnError)
	flags.SetOutput(stderr)
	state := flags.String("state", "", "existing stopped Simulator-paired evaluation install directory (required)")
	trustFile := flags.String("enrollment-trust-file", "", "protected delegation trust currently loaded by the Enrollment Service administration listener (required)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *state == "" || *trustFile == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: zpr-lab-bootstrap -evaluation-simulator-enrollment-trust -state /existing/install -enrollment-trust-file /absolute/enrollment-delegation-trust.json")
		return 2
	}
	fmt.Fprintln(stdout, evaluationBanner)
	absolute, err := filepath.Abs(*state)
	if err != nil {
		fmt.Fprintln(stderr, "Invalid local state path.")
		return 1
	}
	if err := deriveEvaluationSimulatorEnrollmentTrust(absolute, *trustFile); err != nil {
		fmt.Fprintf(stderr, "Simulator enrollment trust: %v\n", err)
		return 1
	}
	target := filepath.Join(absolute, evaluationSimulatorEnrollmentTrustFile)
	fmt.Fprintf(stdout, "The registered independent Simulator signing key is included for forwarded enrollment administration in %s.\nPoint the Enrollment Service administration delegation_trust_file at that derived file and restart only the Enrollment Service. The original trust file, its named grants, the Room key and every retained identity are unchanged; production services never require Simulator.\n", target)
	return 0
}

// deriveEvaluationSimulatorEnrollmentTrust writes a derived Enrollment Service
// administration delegation trust that adds only the install's registered
// independent Simulator signing key, pinned to the exact relay transport
// certificate that presents forwarded delegations. It requires a completed
// Simulator pairing with recorded operator service bindings that include an
// Enrollment Service relay, never modifies the operator-owned input, never
// rotates or rebinds keys, and resumes or verifies instead of overwriting.
func deriveEvaluationSimulatorEnrollmentTrust(state, trustFile string) error {
	if !filepath.IsAbs(trustFile) || filepath.Clean(trustFile) != trustFile || strings.ContainsAny(trustFile, ",\"\r\n=") {
		return errors.New("enrollment-trust-file requires a canonical absolute bind-mount-safe path")
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
	data, err := os.ReadFile(filepath.Join(state, "evaluation.json"))
	var inventory evaluationInventory
	if err != nil || json.Unmarshal(data, &inventory) != nil || inventory.Config.validate() != nil {
		return errors.New("invalid evaluation state; enrollment trust derivation never creates or reseeds an install")
	}
	if inventory.Config.SimulatorPort == 0 {
		return errors.New("install has no registered Simulator client; run -evaluation-simulator-pairing first")
	}
	inventory, err = readEvaluation(state, inventory.Config)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(trustFile)
	if err != nil {
		return errors.New("cannot resolve protected enrollment trust input")
	}
	target := filepath.Join(state, evaluationSimulatorEnrollmentTrustFile)
	if resolved == target {
		return errors.New("enrollment-trust-file is the derived output; supply the Enrollment Service's current operator-owned trust instead")
	}
	services, err := loadEvaluationOperatorServices(filepath.Join(state, evaluationSimulatorServicesFile), state, inventory)
	if err != nil {
		return fmt.Errorf("validate paired operator services: %w", err)
	}
	if services.EnrollmentService == nil {
		return errors.New("paired operator services have no Enrollment Service relay binding; nothing presents forwarded delegations to extend")
	}
	relayChain, err := servicehttp.ReadOperatorFile(services.EnrollmentService.ClientCertificateFile, 1<<20, true)
	if err != nil {
		return fmt.Errorf("read enrollment relay client certificate: %w", err)
	}
	roomKey, err := servicehttp.ReadOperatorFile(filepath.Join(state, "operator-delegation-key.pem"), 1<<20, true)
	if err != nil {
		return fmt.Errorf("read retained Room delegation key: %w", err)
	}
	simulatorKey, err := servicehttp.ReadOperatorFile(filepath.Join(state, "simulator-delegation-key.pem"), 1<<20, true)
	if err != nil {
		return fmt.Errorf("read registered Simulator delegation key: %w", err)
	}
	trustData, err := servicehttp.ReadOperatorFile(trustFile, 1<<20, true)
	if err != nil {
		return fmt.Errorf("read protected enrollment trust input: %w", err)
	}
	expected, err := renderEvaluationSimulatorEnrollmentTrust(trustData, inventory.Config, roomKey, simulatorKey, relayChain)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(state)
	if err != nil {
		return errors.New("cannot open protected state")
	}
	defer root.Close()
	return commitEvaluationSimulatorEnrollmentTrust(root, expected)
}

// renderEvaluationSimulatorEnrollmentTrust extends the Enrollment Service's
// delegation trust with only the independent Simulator key. The audience,
// retained Room key entry and every named grant are preserved byte for byte.
// Both keys are pinned to the relay transport certificate because the
// Control-Service relay presents forwarded delegations to the Enrollment
// Service administration listener.
func renderEvaluationSimulatorEnrollmentTrust(trustData []byte, config evaluationConfig, roomKey, simulatorKey, relayChain []byte) ([]byte, error) {
	var document map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(trustData))
	if err := decoder.Decode(&document); err != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("enrollment delegation trust requires one JSON object")
	}
	var audience string
	if json.Unmarshal(document["audience"], &audience) != nil || audience != evaluationServiceAudience(config) {
		return nil, errors.New("enrollment delegation trust audience differs from this installation's Control-Service delegation audience")
	}
	room, err := evaluationDelegationKeyEntry(evaluationDelegationKeyID, roomKey, relayChain)
	if err != nil {
		return nil, fmt.Errorf("derive retained Room key entry: %w", err)
	}
	simulator, err := evaluationDelegationKeyEntry(evaluationSimulatorDelegationKeyID, simulatorKey, relayChain)
	if err != nil {
		return nil, fmt.Errorf("derive registered Simulator key entry: %w", err)
	}
	var keys []map[string]string
	if json.Unmarshal(document["keys"], &keys) != nil || len(keys) == 0 || len(keys) > 2 ||
		!maps.Equal(keys[0], room) {
		return nil, errors.New("enrollment delegation trust must contain exactly this installation's retained Room key pinned to the enrollment relay transport")
	}
	if len(keys) == 2 && !maps.Equal(keys[1], simulator) {
		return nil, errors.New("enrollment delegation trust already contains a different second key; derivation never rotates or rebinds keys")
	}
	keys = []map[string]string{room, simulator}
	if document["keys"], err = json.Marshal(keys); err != nil {
		return nil, err
	}
	rendered, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	rendered = append(rendered, '\n')
	var validated operatordelegation.Config
	if err := decodeEvaluationDelegationTrust(rendered, &validated); err != nil {
		return nil, fmt.Errorf("derived enrollment delegation trust is invalid: %w", err)
	}
	return rendered, nil
}

// commitEvaluationSimulatorEnrollmentTrust verifies an existing target,
// resumes exact recorded staging, or stages and atomically renames the new
// derived trust. A differing existing target or staging fails closed.
func commitEvaluationSimulatorEnrollmentTrust(root *os.Root, expected []byte) error {
	staged, stagedErr := root.ReadFile(evaluationSimulatorEnrollmentTrustStaging)
	if stagedErr != nil && !errors.Is(stagedErr, os.ErrNotExist) {
		return errors.New("cannot inspect staged enrollment trust")
	}
	if stagedErr == nil && !bytes.Equal(staged, expected) {
		return errors.New("staged enrollment trust differs from the current derivation; inspect it before retrying")
	}
	existing, err := root.ReadFile(evaluationSimulatorEnrollmentTrustFile)
	if err == nil {
		if !bytes.Equal(existing, expected) {
			return errors.New("derived enrollment trust target already exists with different content; inspect it before retrying")
		}
		if stagedErr == nil {
			if err := root.Remove(evaluationSimulatorEnrollmentTrustStaging); err != nil {
				return errors.New("cannot retire completed enrollment trust staging")
			}
			return syncRecoveryDirectory(root)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot inspect derived enrollment trust target")
	}
	if stagedErr != nil {
		if err := writePrivate(root, evaluationSimulatorEnrollmentTrustStaging, expected); err != nil {
			return errors.New("enrollment trust staging failed; rerun with the same arguments")
		}
		if err := syncRecoveryDirectory(root); err != nil {
			return err
		}
	}
	if err := root.Rename(evaluationSimulatorEnrollmentTrustStaging, evaluationSimulatorEnrollmentTrustFile); err != nil {
		return errors.New("enrollment trust replacement failed; rerun with the same arguments to complete it")
	}
	return syncRecoveryDirectory(root)
}
