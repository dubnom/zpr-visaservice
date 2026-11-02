package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

type pairingServicesFixture struct {
	state       string
	inventory   evaluationInventory
	services    string
	trust       string
	base        []byte
	original    []byte
	serviceData []byte
}

func newPairingServicesFixture(t *testing.T) pairingServicesFixture {
	t.Helper()
	config := testEvaluationSubjectConfig()
	config.OrganizationID = "velocity"
	state, inventory := recoveryFixtureWith(t, config)
	config = inventory.Config
	issuer := config.browserOrigin(config.OIDCPort)
	admin := operatorGrantIdentity{Subject: "named-admin", Permissions: append([]string{"monitor.read"}, evaluationAdminSubjectLinkPermissions...)}
	reviewer := operatorGrantIdentity{Subject: "named-reviewer", Permissions: evaluationReviewerPermissions}
	base, err := evaluationDelegationTrustFile(config, issuer, "velocity",
		readSubjectFile(t, state, "operator-delegation-key.pem"), readSubjectFile(t, state, "client-chain.pem"),
		nil, nil, admin, reviewer)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, evaluationDelegationTrust), base, 0o600); err != nil {
		t.Fatal(err)
	}
	var override operatordelegation.Config
	if err := json.Unmarshal(base, &override); err != nil {
		t.Fatal(err)
	}
	override.Grants[0].Permissions = append(override.Grants[0].Permissions, "approval.self")
	original, err := json.MarshalIndent(override, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	directory := recoveryTempDir(t)
	trust := filepath.Join(directory, "operator-trust.json")
	if err := os.WriteFile(trust, original, 0o600); err != nil {
		t.Fatal(err)
	}
	bindings := evaluationOperatorServices{
		Version: 1, Organization: "velocity", OperatorDelegationTrustFile: trust,
		EnrollmentService: &evaluationMutualTLSServiceBinding{
			URL: "https://localhost:24994", ClientCertificateFile: filepath.Join(state, "client-chain.pem"),
			ClientKeyFile: filepath.Join(state, "client-key.pem"), CAFile: filepath.Join(state, "ca.pem"),
		},
	}
	serviceData, err := json.MarshalIndent(bindings, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	services := filepath.Join(directory, "operator-services.json")
	if err := os.WriteFile(services, serviceData, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadEvaluationOperatorServices(services, state, inventory); err != nil {
		t.Fatalf("invalid original fixture: %v", err)
	}
	return pairingServicesFixture{state, inventory, services, trust, base, original, serviceData}
}

func TestEvaluationSimulatorPairingCLIForwardsOperatorServices(t *testing.T) {
	f := newPairingServicesFixture(t)
	port := pairingFreePort(t)
	var stdout, stderr bytes.Buffer
	if code := runEvaluationSimulatorPairing([]string{
		"-state", f.state, "-simulator-port", strconv.Itoa(port),
		"-operator-services-file", f.services,
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("pairing CLI exit %d: %s", code, stderr.String())
	}
	pairedPath := filepath.Join(f.state, evaluationSimulatorServicesFile)
	if !strings.Contains(stdout.String(), "-operator-services-file "+pairedPath) {
		t.Fatalf("pairing CLI did not report the derived restart binding: %s", stdout.String())
	}
	config := f.inventory.Config
	config.SimulatorPort = port
	inventory, err := readEvaluation(f.state, config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadEvaluationOperatorServices(pairedPath, f.state, inventory); err != nil {
		t.Fatalf("CLI-derived operator services cannot start: %v", err)
	}
}

func TestEvaluationSimulatorPairingPreservesOperatorServicesOverride(t *testing.T) {
	f := newPairingServicesFixture(t)
	port := pairingFreePort(t)
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, time.Now()); err != nil {
		t.Fatal(err)
	}
	config := f.inventory.Config
	config.SimulatorPort = port
	inventory, err := readEvaluation(f.state, config)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.InstallID != f.inventory.InstallID {
		t.Fatal("pairing changed the install identity")
	}
	pairedPath := filepath.Join(f.state, evaluationSimulatorServicesFile)
	paired, err := loadEvaluationOperatorServices(pairedPath, f.state, inventory)
	if err != nil {
		t.Fatalf("paired operator services cannot start: %v", err)
	}
	if paired.OperatorDelegationTrustFile != filepath.Join(f.state, evaluationSimulatorTrustFile) ||
		paired.EnrollmentService == nil || paired.EnrollmentService.URL != "https://localhost:24994" {
		t.Fatal("paired bindings lost the exact independent service or trust path")
	}
	for path, want := range map[string][]byte{f.services: f.serviceData, f.trust: f.original} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("pairing changed original operator input %s: %v", filepath.Base(path), err)
		}
	}
	var before, after, base operatordelegation.Config
	if json.Unmarshal(f.original, &before) != nil ||
		json.Unmarshal(readSubjectFile(t, f.state, evaluationSimulatorTrustFile), &after) != nil ||
		json.Unmarshal(readSubjectFile(t, f.state, evaluationDelegationTrust), &base) != nil {
		t.Fatal("invalid delegation trust")
	}
	if !reflect.DeepEqual(before.Grants, after.Grants) || len(after.Keys) != 2 ||
		after.Keys[0] != before.Keys[0] || after.Keys[1].KeyID != evaluationSimulatorDelegationKeyID ||
		!reflect.DeepEqual(base.Keys, after.Keys) {
		t.Fatal("pairing changed grants, lost the Room key, or failed to add only the Simulator key")
	}
	if !slices.Contains(after.Grants[0].Permissions, "approval.self") ||
		slices.Contains(after.Grants[1].Permissions, "approval.self") {
		t.Fatal("pairing changed the named self-approval authority")
	}
	if _, err := loadEvaluationOperatorServices(f.services, f.state, inventory); err == nil {
		t.Fatal("stale original trust must not be accepted against the paired base trust")
	}
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, time.Now()); err != nil {
		t.Fatalf("exact pairing retry failed: %v", err)
	}
	pairedBefore := readSubjectFile(t, f.state, evaluationSimulatorTrustFile)
	if err := os.WriteFile(f.trust, append(f.original, []byte(" ")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, time.Now()); err != nil {
		t.Fatalf("format-only original trust change changed the bindings: %v", err)
	}
	if err := os.WriteFile(f.trust, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, time.Now()); err == nil {
		t.Fatal("changed original trust was silently accepted on retry")
	}
	if !bytes.Equal(readSubjectFile(t, f.state, evaluationSimulatorTrustFile), pairedBefore) {
		t.Fatal("failed retry changed the paired trust")
	}
}

func TestEvaluationSimulatorPairingRejectsUnsafeOperatorServices(t *testing.T) {
	for _, mode := range []string{"cross-organization", "extra-authority", "unprotected", "missing-trust", "existing-target", "relative-path"} {
		t.Run(mode, func(t *testing.T) {
			f := newPairingServicesFixture(t)
			beforeDex := readSubjectFile(t, f.state, "dex.json")
			beforeInventory := readSubjectFile(t, f.state, "evaluation.json")
			path := f.services
			switch mode {
			case "cross-organization":
				var bindings evaluationOperatorServices
				if err := json.Unmarshal(f.serviceData, &bindings); err != nil {
					t.Fatal(err)
				}
				bindings.Organization = "other"
				data, _ := json.Marshal(bindings)
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			case "extra-authority":
				var trust operatordelegation.Config
				if err := json.Unmarshal(f.original, &trust); err != nil {
					t.Fatal(err)
				}
				trust.Grants[0].Permissions = append(trust.Grants[0].Permissions, "organization.archive")
				data, _ := json.Marshal(trust)
				if err := os.WriteFile(f.trust, data, 0o600); err != nil {
					t.Fatal(err)
				}
			case "unprotected":
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
			case "missing-trust":
				if err := os.Remove(f.trust); err != nil {
					t.Fatal(err)
				}
			case "existing-target":
				if err := os.WriteFile(filepath.Join(f.state, evaluationSimulatorTrustFile), []byte("retain"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "relative-path":
				path = "relative-services.json"
			}
			if err := pairEvaluationSimulatorWithServices(f.state, pairingFreePort(t), path, time.Now()); err == nil {
				t.Fatal("unsafe operator services were accepted")
			}
			if !bytes.Equal(readSubjectFile(t, f.state, "dex.json"), beforeDex) ||
				!bytes.Equal(readSubjectFile(t, f.state, "evaluation.json"), beforeInventory) ||
				!bytes.Equal(readSubjectFile(t, f.state, evaluationDelegationTrust), f.base) {
				t.Fatal("rejected pairing changed retained install state")
			}
			if _, err := os.Lstat(filepath.Join(f.state, evaluationSimulatorPairingPending)); !os.IsNotExist(err) {
				t.Fatalf("rejected pairing recorded a commit plan: %v", err)
			}
			if mode == "existing-target" && string(readSubjectFile(t, f.state, evaluationSimulatorTrustFile)) != "retain" {
				t.Fatal("pairing overwrote an unrelated existing file")
			}
		})
	}
}

func TestEvaluationSimulatorPairingResumesExactOperatorServicesPlan(t *testing.T) {
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
		files["simulator-delegation-key.pem"], files["simulator-client-chain.pem"])
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range paired {
		files[name] = contents
	}
	names := stagePendingSimulatorPairing(t, f.state, port, f.services, files)
	if err := os.Rename(filepath.Join(f.state, names[0]+evaluationSimulatorPairingSuffix), filepath.Join(f.state, names[0])); err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []string{"", filepath.Join(filepath.Dir(f.services), "other.json")} {
		err := pairEvaluationSimulatorWithServices(f.state, port, wrong, time.Now())
		if err == nil || !strings.Contains(err.Error(), "different operator-services-file") {
			t.Fatalf("resume accepted different operator bindings: %v", err)
		}
	}
	if _, err := readEvaluation(f.state, updated); err == nil {
		t.Fatal("interrupted pairing must prevent startup")
	}
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, time.Now()); err != nil {
		t.Fatalf("resuming exact pairing failed: %v", err)
	}
	inventory, err := readEvaluation(f.state, updated)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadEvaluationOperatorServices(filepath.Join(f.state, evaluationSimulatorServicesFile), f.state, inventory); err != nil {
		t.Fatalf("resumed bindings failed validation: %v", err)
	}
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, time.Now()); err != nil {
		t.Fatalf("resumed pairing is not idempotent: %v", err)
	}
}

// An install paired without the services flag may add the derived paired
// bindings later from its registered artifacts: no key is created, rotated or
// rebound, and the result is exactly what verification re-derives.
func TestEvaluationSimulatorPairingDerivesServicesForAlreadyPairedInstall(t *testing.T) {
	f := newPairingServicesFixture(t)
	port := pairingFreePort(t)
	if err := pairEvaluationSimulatorWithServices(f.state, port, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{evaluationSimulatorServicesFile, evaluationSimulatorTrustFile} {
		if _, err := os.Lstat(filepath.Join(f.state, name)); !os.IsNotExist(err) {
			t.Fatalf("flagless pairing wrote paired services %s: %v", name, err)
		}
	}
	keyBefore := readSubjectFile(t, f.state, "simulator-delegation-key.pem")
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, time.Now()); err != nil {
		t.Fatalf("post-hoc paired services derivation failed: %v", err)
	}
	if !bytes.Equal(keyBefore, readSubjectFile(t, f.state, "simulator-delegation-key.pem")) {
		t.Fatal("post-hoc derivation rotated the registered Simulator key")
	}
	config := f.inventory.Config
	config.SimulatorPort = port
	inventory, err := readEvaluation(f.state, config)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := loadEvaluationOperatorServices(filepath.Join(f.state, evaluationSimulatorServicesFile), f.state, inventory)
	if err != nil || paired.OperatorDelegationTrustFile != filepath.Join(f.state, evaluationSimulatorTrustFile) {
		t.Fatalf("derived paired bindings are unusable: %v", err)
	}
	var before, after operatordelegation.Config
	if json.Unmarshal(f.original, &before) != nil ||
		json.Unmarshal(readSubjectFile(t, f.state, evaluationSimulatorTrustFile), &after) != nil {
		t.Fatal("invalid delegation trust")
	}
	if !reflect.DeepEqual(before.Grants, after.Grants) || len(after.Keys) != 2 ||
		after.Keys[1].KeyID != evaluationSimulatorDelegationKeyID {
		t.Fatal("post-hoc derivation changed grants or did not add only the Simulator key")
	}
	// Idempotent: the rerun verifies the exact derived content.
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, time.Now()); err != nil {
		t.Fatalf("verification rerun failed: %v", err)
	}
	// Partial presence fails closed for inspection.
	if err := os.Remove(filepath.Join(f.state, evaluationSimulatorServicesFile)); err != nil {
		t.Fatal(err)
	}
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, time.Now()); err == nil ||
		!strings.Contains(err.Error(), "partially present") {
		t.Fatalf("partial paired services were not refused: %v", err)
	}
}
