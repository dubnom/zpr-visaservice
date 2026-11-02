package main

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func simulatorPairingTestPlan(t *testing.T, root *os.Root, port int, services string, files map[string][]byte) (evaluationSimulatorPairingPlan, []string) {
	t.Helper()
	digests := make(map[string]string, len(files))
	names := make([]string, 0, len(files))
	for name, contents := range files {
		digests[name] = evaluationPairingDigest(contents)
		names = append(names, name)
	}
	slices.Sort(names)
	original, err := captureEvaluationSimulatorPairingTargets(root, names)
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]string
	if services != "" {
		inputs, err = snapshotEvaluationSimulatorOperatorInputs(services)
		if err != nil {
			t.Fatal(err)
		}
	}
	return evaluationSimulatorPairingPlan{
		Version: 2, SimulatorPort: port, OperatorServicesFile: services,
		Files: digests, OriginalFiles: original, OperatorInputs: inputs,
	}, names
}

func stageSimulatorPairingPreparation(t *testing.T, state string, port int, services string, files map[string][]byte, count int, pending bool) []string {
	t.Helper()
	root, err := os.OpenRoot(state)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	plan, names := simulatorPairingTestPlan(t, root, port, services, files)
	marker, err := evaluationJSON(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := writePrivate(root, evaluationSimulatorPairingPreparing, marker); err != nil {
		t.Fatal(err)
	}
	if count > len(names) {
		count = len(names)
	}
	for _, name := range names[:count] {
		if err := writePrivate(root, name+evaluationSimulatorPairingSuffix, files[name]); err != nil {
			t.Fatal(err)
		}
	}
	if pending {
		if count != len(names) {
			t.Fatal("a pending commit requires every target to be staged")
		}
		if err := writePrivate(root, evaluationSimulatorPairingPending, marker); err != nil {
			t.Fatal(err)
		}
	}
	return names
}

func stagePendingSimulatorPairing(t *testing.T, state string, port int, services string, files map[string][]byte) []string {
	t.Helper()
	return stageSimulatorPairingPreparation(t, state, port, services, files, len(files), true)
}

func TestSimulatorPairingPreservesUnrelatedStagingSuffix(t *testing.T) {
	state, _ := recoveryFixture(t)
	name := "operator-notes" + evaluationSimulatorPairingSuffix
	want := []byte("unrelated operator data")
	if err := os.WriteFile(filepath.Join(state, name), want, 0o600); err != nil {
		t.Fatal(err)
	}
	port := pairingFreePort(t)
	if err := pairEvaluationSimulator(state, port, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(state, name)); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("pairing removed or changed unrelated state: %q, %v", got, err)
	}
	if err := pairEvaluationSimulator(state, port, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(state, name)); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("completed retry removed or changed unrelated state: %q, %v", got, err)
	}
}

func TestSimulatorPairingRefusesUnrecordedTargetCollisions(t *testing.T) {
	for _, name := range []string{
		"oidc-simulator-client.secret",
		"simulator-delegation-key.pem",
		"simulator-client-chain.pem",
		"dex.json" + evaluationSimulatorPairingSuffix,
	} {
		t.Run(name, func(t *testing.T) {
			state, _ := recoveryFixture(t)
			beforeDex := readSubjectFile(t, state, "dex.json")
			beforeInventory := readSubjectFile(t, state, "evaluation.json")
			want := []byte("unrelated existing data")
			if err := os.WriteFile(filepath.Join(state, name), want, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := pairEvaluationSimulator(state, pairingFreePort(t), time.Now()); err == nil {
				t.Fatal("pairing accepted an unrecorded existing output")
			}
			if got, err := os.ReadFile(filepath.Join(state, name)); err != nil || !bytes.Equal(got, want) {
				t.Fatalf("pairing removed or changed the existing file: %q, %v", got, err)
			}
			if !bytes.Equal(readSubjectFile(t, state, "dex.json"), beforeDex) ||
				!bytes.Equal(readSubjectFile(t, state, "evaluation.json"), beforeInventory) {
				t.Fatal("rejected pairing changed the existing installation")
			}
		})
	}
}

func TestSimulatorPairingResumeRequiresStoppedServices(t *testing.T) {
	state, inventory := recoveryFixture(t)
	port := pairingFreePort(t)
	updated := inventory.Config
	updated.SimulatorPort = port
	files, err := planEvaluationSimulatorPairing(state, inventory, updated, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	stagePendingSimulatorPairing(t, state, port, "", files)
	listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(inventory.Config.RoomPort))
	if err != nil {
		t.Fatal(err)
	}
	err = pairEvaluationSimulator(state, port, time.Now())
	listener.Close()
	if err == nil || !strings.Contains(err.Error(), "occupied") {
		t.Fatalf("resume must refuse running installation services: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state, evaluationSimulatorPairingPending)); err != nil {
		t.Fatalf("refused resume must retain its plan: %v", err)
	}
	listener, err = net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	err = pairEvaluationSimulator(state, port, time.Now())
	listener.Close()
	if err == nil || !strings.Contains(err.Error(), "Simulator port is occupied") {
		t.Fatalf("resume must refuse a running Simulator: %v", err)
	}
	if err := pairEvaluationSimulator(state, port, time.Now()); err != nil {
		t.Fatalf("resume after stopping services: %v", err)
	}
	if _, err := readEvaluation(state, updated); err != nil {
		t.Fatalf("resumed install failed integrity verification: %v", err)
	}
}

func TestSimulatorPairingResumeRejectsChangedOperatorInputs(t *testing.T) {
	for _, changed := range []string{"bindings", "trust", "referenced-ca"} {
		t.Run(changed, func(t *testing.T) {
			f := newPairingServicesFixture(t)
			var externalCA string
			if changed == "referenced-ca" {
				externalCA = filepath.Join(filepath.Dir(f.services), "external-ca.pem")
				if err := os.WriteFile(externalCA, readSubjectFile(t, f.state, "ca.pem"), 0o600); err != nil {
					t.Fatal(err)
				}
				var bindings evaluationOperatorServices
				if err := json.Unmarshal(f.serviceData, &bindings); err != nil {
					t.Fatal(err)
				}
				bindings.EnrollmentService.CAFile = externalCA
				data, err := json.Marshal(bindings)
				if err != nil {
					t.Fatal(err)
				}
				f.serviceData = data
				if err := os.WriteFile(f.services, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			port := pairingFreePort(t)
			updated := f.inventory.Config
			updated.SimulatorPort = port
			files, err := planEvaluationSimulatorPairing(f.state, f.inventory, updated, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			services, err := loadEvaluationOperatorServices(f.services, f.state, f.inventory)
			if err != nil {
				t.Fatal(err)
			}
			paired, err := renderEvaluationSimulatorServices(f.state, services, updated,
				files["simulator-delegation-key.pem"], files["simulator-client-chain.pem"], nil)
			if err != nil {
				t.Fatal(err)
			}
			for name, contents := range paired {
				files[name] = contents
			}
			names := stagePendingSimulatorPairing(t, f.state, port, f.services, files)
			if err := os.Rename(filepath.Join(f.state, names[0]+evaluationSimulatorPairingSuffix),
				filepath.Join(f.state, names[0])); err != nil {
				t.Fatal(err)
			}
			path, original := f.trust, f.original
			replacement := []byte("{}")
			if changed == "bindings" {
				path, original = f.services, f.serviceData
				var bindings evaluationOperatorServices
				if err := json.Unmarshal(original, &bindings); err != nil {
					t.Fatal(err)
				}
				bindings.EnrollmentService.URL = "https://localhost:24995"
				replacement, err = json.Marshal(bindings)
				if err != nil {
					t.Fatal(err)
				}
			} else if changed == "referenced-ca" {
				path = externalCA
				original, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(path, replacement, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, "", time.Now()); err == nil {
				t.Fatal("resume accepted changed operator inputs")
			}
			if _, err := os.Stat(filepath.Join(f.state, evaluationSimulatorPairingPending)); err != nil {
				t.Fatalf("refused resume must retain its plan: %v", err)
			}
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, "", time.Now()); err != nil {
				t.Fatalf("resume after restoring operator input: %v", err)
			}
			if _, err := readEvaluation(f.state, updated); err != nil {
				t.Fatalf("resumed install failed integrity verification: %v", err)
			}
		})
	}
}

func TestSimulatorPairingRecoversRecordedPreparation(t *testing.T) {
	for _, count := range []int{0, 2, 100} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			state, inventory := recoveryFixture(t)
			port := pairingFreePort(t)
			updated := inventory.Config
			updated.SimulatorPort = port
			files, err := planEvaluationSimulatorPairing(state, inventory, updated, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			unrelated := "notes" + evaluationSimulatorPairingSuffix
			if err := os.WriteFile(filepath.Join(state, unrelated), []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			names := stageSimulatorPairingPreparation(t, state, port, "", files, count, false)
			if _, err := readEvaluation(state, inventory.Config); err == nil || !strings.Contains(err.Error(), "interrupted Simulator pairing") {
				t.Fatalf("preparation must block startup: %v", err)
			}
			if err := pairEvaluationSimulator(state, port+1, time.Now()); err == nil || !strings.Contains(err.Error(), "different simulator-port") {
				t.Fatalf("preparation recovery must require the recorded port: %v", err)
			}
			if err := pairEvaluationSimulator(state, port, time.Now()); err != nil {
				t.Fatalf("recorded preparation should recover automatically: %v", err)
			}
			if _, err := readEvaluation(state, updated); err != nil {
				t.Fatalf("recovered installation failed integrity validation: %v", err)
			}
			for _, name := range append(names, evaluationSimulatorPairingPreparing, evaluationSimulatorPairingPending) {
				path := filepath.Join(state, name)
				if slices.Contains(names, name) {
					path += evaluationSimulatorPairingSuffix
				}
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("recorded preparation was not retired: %s: %v", name, err)
				}
			}
			if got, err := os.ReadFile(filepath.Join(state, unrelated)); err != nil || string(got) != "keep" {
				t.Fatalf("preparation recovery changed unrelated data: %q, %v", got, err)
			}
		})
	}
}

func TestSimulatorPairingResumesAfterPreparationWasRetired(t *testing.T) {
	state, inventory := recoveryFixture(t)
	port := pairingFreePort(t)
	updated := inventory.Config
	updated.SimulatorPort = port
	files, err := planEvaluationSimulatorPairing(state, inventory, updated, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	names := stagePendingSimulatorPairing(t, state, port, "", files)
	for _, name := range names {
		if err := os.Rename(filepath.Join(state, name+evaluationSimulatorPairingSuffix), filepath.Join(state, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(state, evaluationSimulatorPairingPreparing)); err != nil {
		t.Fatal(err)
	}
	if err := pairEvaluationSimulator(state, port, time.Now()); err != nil {
		t.Fatalf("fully committed pairing should retire its pending record: %v", err)
	}
	if _, err := readEvaluation(state, updated); err != nil {
		t.Fatalf("retired plan did not leave a valid installation: %v", err)
	}
}

func TestSimulatorPairingPreparationRefusesChangedStagedFile(t *testing.T) {
	state, inventory := recoveryFixture(t)
	port := pairingFreePort(t)
	updated := inventory.Config
	updated.SimulatorPort = port
	files, err := planEvaluationSimulatorPairing(state, inventory, updated, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	names := stageSimulatorPairingPreparation(t, state, port, "", files, 2, false)
	first := filepath.Join(state, names[0]+evaluationSimulatorPairingSuffix)
	changed := filepath.Join(state, names[1]+evaluationSimulatorPairingSuffix)
	if err := os.WriteFile(changed, []byte("foreign or partial data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pairEvaluationSimulator(state, port, time.Now()); err == nil || !strings.Contains(err.Error(), "differs from its preparation plan") {
		t.Fatalf("changed staged data must fail closed: %v", err)
	}
	if got, err := os.ReadFile(first); err != nil || !bytes.Equal(got, files[names[0]]) {
		t.Fatalf("failed recovery removed another staged file: %v", err)
	}
	if got, err := os.ReadFile(changed); err != nil || string(got) != "foreign or partial data" {
		t.Fatalf("failed recovery removed changed staged data: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(state, evaluationSimulatorPairingPreparing)); err != nil {
		t.Fatalf("failed recovery must retain its preparation plan: %v", err)
	}
}

func TestSimulatorPairingPreparationRefusesChangedOperatorInputs(t *testing.T) {
	f := newPairingServicesFixture(t)
	port := pairingFreePort(t)
	updated := f.inventory.Config
	updated.SimulatorPort = port
	files, err := planEvaluationSimulatorPairing(f.state, f.inventory, updated, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	services, err := loadEvaluationOperatorServices(f.services, f.state, f.inventory)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := renderEvaluationSimulatorServices(f.state, services, updated,
		files["simulator-delegation-key.pem"], files["simulator-client-chain.pem"], nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range paired {
		files[name] = contents
	}
	names := stageSimulatorPairingPreparation(t, f.state, port, f.services, files, 2, false)
	if err := os.WriteFile(f.trust, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, "", time.Now()); err == nil {
		t.Fatal("preparation recovery accepted changed operator inputs")
	}
	if got, err := os.ReadFile(filepath.Join(f.state, names[0]+evaluationSimulatorPairingSuffix)); err != nil || !bytes.Equal(got, files[names[0]]) {
		t.Fatalf("rejected recovery changed staged data: %v", err)
	}
	if err := os.WriteFile(f.trust, f.original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, "", time.Now()); err != nil {
		t.Fatalf("preparation recovery after restoring inputs: %v", err)
	}
	if _, err := readEvaluation(f.state, updated); err != nil {
		t.Fatal(err)
	}
}

func TestSimulatorPairingResumePreservesNewAndChangedTargets(t *testing.T) {
	for _, changed := range []string{"new-target", "retained-file", "replacement-target"} {
		t.Run(changed, func(t *testing.T) {
			state, inventory := recoveryFixture(t)
			port := pairingFreePort(t)
			updated := inventory.Config
			updated.SimulatorPort = port
			files, err := planEvaluationSimulatorPairing(state, inventory, updated, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			stagePendingSimulatorPairing(t, state, port, "", files)
			name := "simulator-delegation-key.pem"
			if changed == "retained-file" {
				name = "oidc-room-client.secret"
			} else if changed == "replacement-target" {
				name = "dex.json"
			}
			path := filepath.Join(state, name)
			original, err := os.ReadFile(path)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("preserve this change"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := pairEvaluationSimulator(state, port, time.Now()); err == nil {
				t.Fatal("resume accepted a changed or unexpected target")
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != "preserve this change" {
				t.Fatalf("resume overwrote or removed the changed target: %q, %v", got, err)
			}
			if original == nil {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := pairEvaluationSimulator(state, port, time.Now()); err != nil {
				t.Fatalf("resume after restoring the original target: %v", err)
			}
			if _, err := readEvaluation(state, updated); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSimulatorPairingRefusesOperatorInputThatWouldBeReplaced(t *testing.T) {
	f := newPairingServicesFixture(t)
	var bindings evaluationOperatorServices
	if err := json.Unmarshal(f.serviceData, &bindings); err != nil {
		t.Fatal(err)
	}
	bindings.OperatorDelegationTrustFile = filepath.Join(f.state, evaluationDelegationTrust)
	data, err := json.Marshal(bindings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.services, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pairEvaluationSimulatorWithServices(f.state, pairingFreePort(t), f.services, "", time.Now()); err == nil ||
		!strings.Contains(err.Error(), "also a pairing output") {
		t.Fatalf("pairing must not overwrite an original operator input: %v", err)
	}
	if got := readSubjectFile(t, f.state, evaluationDelegationTrust); !bytes.Equal(got, f.base) {
		t.Fatal("rejected pairing changed the original trust")
	}
}

func TestSimulatorPairingRejectsMismatchedOrLegacyPlans(t *testing.T) {
	for _, mode := range []string{"mismatched", "legacy", "unknown-target"} {
		t.Run(mode, func(t *testing.T) {
			state, inventory := recoveryFixture(t)
			port := pairingFreePort(t)
			updated := inventory.Config
			updated.SimulatorPort = port
			files, err := planEvaluationSimulatorPairing(state, inventory, updated, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			stagePendingSimulatorPairing(t, state, port, "", files)
			pendingPath := filepath.Join(state, evaluationSimulatorPairingPending)
			preparingPath := filepath.Join(state, evaluationSimulatorPairingPreparing)
			pending, err := os.ReadFile(pendingPath)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "mismatched" {
				if err := os.WriteFile(preparingPath, append(pending, ' '), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				var plan evaluationSimulatorPairingPlan
				if err := json.Unmarshal(pending, &plan); err != nil {
					t.Fatal(err)
				}
				if mode == "legacy" {
					plan.Version = 1
				} else {
					plan.Files["unrelated-operator-file"] = evaluationPairingDigest([]byte("keep"))
				}
				changed, err := evaluationJSON(plan)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(pendingPath, changed, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			beforeDex := readSubjectFile(t, state, "dex.json")
			if err := pairEvaluationSimulator(state, port, time.Now()); err == nil {
				t.Fatal("unsafe or mismatched pairing plans must fail closed")
			}
			if !bytes.Equal(readSubjectFile(t, state, "dex.json"), beforeDex) {
				t.Fatal("invalid plan changed an existing target")
			}
			if err := os.WriteFile(pendingPath, pending, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(preparingPath, pending, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := pairEvaluationSimulator(state, port, time.Now()); err != nil {
				t.Fatalf("restoring matching recorded plans should allow resume: %v", err)
			}
		})
	}
}

func TestSimulatorPairingRequiresPort(t *testing.T) {
	state, _ := recoveryFixture(t)
	if err := pairEvaluationSimulator(state, 0, time.Now()); err == nil {
		t.Fatal("pairing without a Simulator port must not report success")
	}
}

func TestSimulatorLauncherLocksBeforePreparingState(t *testing.T) {
	config := testEvaluationDataPlaneConfig()
	config.DataPlaneEndpoints = 3
	config.SimulatorPort = pairingFreePort(t)
	parent := recoveryTempDir(t)
	evaluation := filepath.Join(parent, "evaluation")
	inventory, err := prepareEvaluation(evaluation, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	login := operatorauth.Config{
		Version: 1, Issuer: config.browserOrigin(config.OIDCPort), ClientID: "zpr-simulator",
		RedirectURL: config.browserOrigin(config.SimulatorPort) + "/auth/operator/callback", SessionLifetimeSeconds: 3600,
		Grants: []operatorauth.Grant{{
			Issuer: config.browserOrigin(config.OIDCPort), Subject: "verified-test-subject",
			Organizations: []string{evaluationOrganization(inventory)}, Permissions: []string{"simulator.read"},
		}},
	}
	data, err := json.Marshal(login)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evaluation, "simulator-oidc.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(parent, "simulator")
	if err := os.WriteFile(binary, []byte("prebuilt"), 0o700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(parent, "simulator-state")
	lock, err := lockEvaluationSimulator(evaluation)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	var stdout, stderr bytes.Buffer
	if code := runEvaluationSimulator([]string{
		"-evaluation-state", evaluation, "-state", state, "-binary", binary,
		"-data-plane-image", "sha256:" + strings.Repeat("a", 64),
	}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "paired Simulator is running") {
		t.Fatalf("launcher must refuse a held lifecycle lock: exit=%d, error=%s", code, stderr.String())
	}
	if _, err := os.Lstat(state); !os.IsNotExist(err) {
		t.Fatalf("launcher prepared state before acquiring the lifecycle lock: %v", err)
	}
}

func TestSimulatorLauncherRejectsSymlinkBeforeLocking(t *testing.T) {
	parent := recoveryTempDir(t)
	realState := filepath.Join(parent, "real")
	if err := os.Mkdir(realState, 0o700); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(parent, "evaluation")
	if err := os.Symlink(realState, symlink); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(parent, "simulator")
	if err := os.WriteFile(binary, []byte("prebuilt"), 0o700); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runEvaluationSimulator([]string{
		"-evaluation-state", symlink, "-state", filepath.Join(parent, "simulator-state"), "-binary", binary,
		"-data-plane-image", "sha256:" + strings.Repeat("a", 64),
	}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "real private directory") {
		t.Fatalf("symlinked evaluation state was accepted: exit=%d, error=%s", code, stderr.String())
	}
	if _, err := os.Lstat(filepath.Join(realState, "simulator-runtime.lock")); !os.IsNotExist(err) {
		t.Fatalf("launcher wrote a lock through a rejected symlink: %v", err)
	}
}
