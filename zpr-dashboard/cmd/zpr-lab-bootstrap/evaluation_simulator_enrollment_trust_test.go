package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func enrollmentTrustFixtureInput(t *testing.T, f pairingServicesFixture) string {
	t.Helper()
	directory := recoveryTempDir(t)
	input := filepath.Join(directory, "enrollment-delegation-trust.json")
	if err := os.WriteFile(input, f.original, 0o600); err != nil {
		t.Fatal(err)
	}
	return input
}

func TestEvaluationSimulatorEnrollmentTrustDerivesPairedTrust(t *testing.T) {
	f := newPairingServicesFixture(t)
	port := pairingFreePort(t)
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	input := enrollmentTrustFixtureInput(t, f)
	if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, ""); err != nil {
		t.Fatalf("derivation failed: %v", err)
	}
	target := filepath.Join(f.state, evaluationSimulatorEnrollmentTrustFile)
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("derived enrollment trust is missing or unprotected: %v", err)
	}
	var before, derived, paired operatordelegation.Config
	if json.Unmarshal(f.original, &before) != nil ||
		json.Unmarshal(readSubjectFile(t, f.state, evaluationSimulatorEnrollmentTrustFile), &derived) != nil ||
		json.Unmarshal(readSubjectFile(t, f.state, evaluationSimulatorTrustFile), &paired) != nil {
		t.Fatal("invalid delegation trust")
	}
	config := f.inventory.Config
	config.SimulatorPort = port
	if derived.Audience != evaluationServiceAudience(config) || !reflect.DeepEqual(derived.Grants, before.Grants) {
		t.Fatal("derivation changed the audience or named grants")
	}
	if len(derived.Keys) != 2 || derived.Keys[0] != before.Keys[0] ||
		derived.Keys[1].KeyID != evaluationSimulatorDelegationKeyID ||
		derived.Keys[1].PublicKey != paired.Keys[1].PublicKey {
		t.Fatal("derivation lost the Room key or failed to add only the registered Simulator key")
	}
	if derived.Keys[1].CertificateSHA256 != before.Keys[0].CertificateSHA256 ||
		derived.Keys[1].CertificateSHA256 == paired.Keys[1].CertificateSHA256 {
		t.Fatal("the Simulator key must be pinned to the enrollment relay transport, not its own browser-facing client certificate")
	}
	if got, err := os.ReadFile(input); err != nil || !bytes.Equal(got, f.original) {
		t.Fatal("derivation changed the operator-owned enrollment trust input")
	}
	rendered := readSubjectFile(t, f.state, evaluationSimulatorEnrollmentTrustFile)
	var stdout, stderr bytes.Buffer
	if code := runEvaluationSimulatorEnrollmentTrust([]string{
		"-state", f.state, "-enrollment-trust-file", input,
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("exact retry exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), target) {
		t.Fatalf("CLI did not report the derived trust path: %s", stdout.String())
	}
	if !bytes.Equal(readSubjectFile(t, f.state, evaluationSimulatorEnrollmentTrustFile), rendered) {
		t.Fatal("exact retry changed the derived trust")
	}
	// Deriving again with the derived content as input verifies instead of extending twice.
	alreadyExtended := filepath.Join(recoveryTempDir(t), "already-extended.json")
	if err := os.WriteFile(alreadyExtended, rendered, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, alreadyExtended, ""); err != nil {
		t.Fatalf("already-extended input was not verified: %v", err)
	}
}

func TestEvaluationSimulatorEnrollmentTrustResumesInterruptedRename(t *testing.T) {
	f := newPairingServicesFixture(t)
	port := pairingFreePort(t)
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	input := enrollmentTrustFixtureInput(t, f)
	if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, ""); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(f.state, evaluationSimulatorEnrollmentTrustFile)
	rendered := readSubjectFile(t, f.state, evaluationSimulatorEnrollmentTrustFile)
	staging := filepath.Join(f.state, evaluationSimulatorEnrollmentTrustStaging)
	if err := os.Rename(target, staging); err != nil {
		t.Fatal(err)
	}
	if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, ""); err != nil {
		t.Fatalf("interrupted rename did not resume: %v", err)
	}
	if !bytes.Equal(readSubjectFile(t, f.state, evaluationSimulatorEnrollmentTrustFile), rendered) {
		t.Fatal("resume changed the derived trust")
	}
	if _, err := os.Lstat(staging); !os.IsNotExist(err) {
		t.Fatal("resume left staging behind")
	}
	if err := os.WriteFile(staging, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, ""); err == nil ||
		!strings.Contains(err.Error(), "staged enrollment trust differs") {
		t.Fatalf("mismatched staging was not refused: %v", err)
	}
}

func TestEvaluationSimulatorEnrollmentTrustRefusesUnsafeInputs(t *testing.T) {
	t.Run("unpaired", func(t *testing.T) {
		f := newPairingServicesFixture(t)
		input := enrollmentTrustFixtureInput(t, f)
		err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, "")
		if err == nil || !strings.Contains(err.Error(), "no registered Simulator client") {
			t.Fatalf("unpaired install was not refused: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(f.state, evaluationSimulatorEnrollmentTrustFile)); !os.IsNotExist(err) {
			t.Fatal("refusal still wrote a derived trust")
		}
	})
	t.Run("no-enrollment-binding", func(t *testing.T) {
		f := newPairingServicesFixture(t)
		port := pairingFreePort(t)
		if err := pairEvaluationSimulator(f.state, port, time.Now()); err != nil {
			t.Fatal(err)
		}
		input := enrollmentTrustFixtureInput(t, f)
		err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, "")
		if err == nil || !strings.Contains(err.Error(), "paired operator services") {
			t.Fatalf("missing relay binding was not refused: %v", err)
		}
	})
	t.Run("relative-path", func(t *testing.T) {
		f := newPairingServicesFixture(t)
		err := deriveEvaluationSimulatorEnrollmentTrust(f.state, "enrollment-trust.json", "")
		if err == nil || !strings.Contains(err.Error(), "canonical absolute") {
			t.Fatalf("relative input was not refused: %v", err)
		}
	})
	paired := func(t *testing.T) (pairingServicesFixture, string) {
		t.Helper()
		f := newPairingServicesFixture(t)
		if err := pairEvaluationSimulatorWithServices(f.state, pairingFreePort(t), f.services, "", time.Now()); err != nil {
			t.Fatal(err)
		}
		return f, enrollmentTrustFixtureInput(t, f)
	}
	t.Run("wrong-audience", func(t *testing.T) {
		f, input := paired(t)
		var document map[string]json.RawMessage
		if err := json.Unmarshal(f.original, &document); err != nil {
			t.Fatal(err)
		}
		document["audience"], _ = json.Marshal("https://localhost:1")
		changed, err := json.MarshalIndent(document, "", "  ")
		if err != nil || os.WriteFile(input, changed, 0o600) != nil {
			t.Fatal(err)
		}
		if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, ""); err == nil ||
			!strings.Contains(err.Error(), "audience differs") {
			t.Fatalf("foreign audience was not refused: %v", err)
		}
	})
	t.Run("foreign-room-key", func(t *testing.T) {
		f, input := paired(t)
		var trust operatordelegation.Config
		if err := json.Unmarshal(f.original, &trust); err != nil {
			t.Fatal(err)
		}
		trust.Keys[0].PublicKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		changed, err := json.MarshalIndent(trust, "", "  ")
		if err != nil || os.WriteFile(input, changed, 0o600) != nil {
			t.Fatal(err)
		}
		if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, ""); err == nil ||
			!strings.Contains(err.Error(), "retained Room key") {
			t.Fatalf("foreign Room key was not refused: %v", err)
		}
	})
	t.Run("foreign-second-key", func(t *testing.T) {
		f, input := paired(t)
		var trust operatordelegation.Config
		if err := json.Unmarshal(f.original, &trust); err != nil {
			t.Fatal(err)
		}
		foreign := trust.Keys[0]
		foreign.KeyID = evaluationSimulatorDelegationKeyID
		trust.Keys = append(trust.Keys, foreign)
		changed, err := json.MarshalIndent(trust, "", "  ")
		if err != nil || os.WriteFile(input, changed, 0o600) != nil {
			t.Fatal(err)
		}
		if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, ""); err == nil ||
			!strings.Contains(err.Error(), "different second key") {
			t.Fatalf("foreign second key was not refused: %v", err)
		}
	})
	t.Run("existing-different-target", func(t *testing.T) {
		f, input := paired(t)
		target := filepath.Join(f.state, evaluationSimulatorEnrollmentTrustFile)
		if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, ""); err == nil ||
			!strings.Contains(err.Error(), "different content") {
			t.Fatalf("existing different target was not refused: %v", err)
		}
		if got := readSubjectFile(t, f.state, evaluationSimulatorEnrollmentTrustFile); !bytes.Equal(got, []byte("{}")) {
			t.Fatal("refusal changed the existing target")
		}
	})
	t.Run("output-as-input", func(t *testing.T) {
		f, input := paired(t)
		if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, ""); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(f.state, evaluationSimulatorEnrollmentTrustFile)
		if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, target, ""); err == nil ||
			!strings.Contains(err.Error(), "derived output") {
			t.Fatalf("derived output as input was not refused: %v", err)
		}
	})
}

func TestEvaluationSimulatorEnrollmentTrustCLIRequiresArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runEvaluationSimulatorEnrollmentTrust(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("missing flags exit %d", code)
	}
	if code := runEvaluationSimulatorEnrollmentTrust([]string{"-state", "/tmp/x"}, &stdout, &stderr); code != 2 {
		t.Fatalf("missing trust flag exit %d", code)
	}
	if code := runEvaluationSimulatorEnrollmentTrust([]string{
		"-state", "/tmp/x", "-enrollment-trust-file", "/tmp/y", "extra" + strconv.Itoa(0),
	}, &stdout, &stderr); code != 2 {
		t.Fatalf("extra argument exit %d", code)
	}
}

// TestEvaluationSimulatorEnrollmentTrustComposedSessionGrants proves the
// composed home-session identity reaches the derived Enrollment Service
// trust with exactly this organization's scope, bounded to the Simulator
// session permissions, while the retained enrollment grants stay unchanged.
func TestEvaluationSimulatorEnrollmentTrustComposedSessionGrants(t *testing.T) {
	f := newPairingServicesFixture(t)
	port := pairingFreePort(t)
	permissions := evaluationSimulatorOperatorPermissions(true)
	session := writeTestComposedSessionConfig(t, "https://home.invalid:15556", "home-admin",
		[]string{"home-org", "velocity"}, permissions)
	if err := pairEvaluationSimulatorWithServices(f.state, port, f.services, session, time.Now()); err != nil {
		t.Fatal(err)
	}
	input := enrollmentTrustFixtureInput(t, f)
	if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, session); err != nil {
		t.Fatalf("composed enrollment trust derivation failed: %v", err)
	}
	var before, derived operatordelegation.Config
	if json.Unmarshal(f.original, &before) != nil ||
		json.Unmarshal(readSubjectFile(t, f.state, evaluationSimulatorEnrollmentTrustFile), &derived) != nil {
		t.Fatal("invalid delegation trust")
	}
	if len(derived.Grants) != len(before.Grants)+1 ||
		!reflect.DeepEqual(derived.Grants[:len(before.Grants)], before.Grants) {
		t.Fatalf("retained enrollment grants changed: %+v", derived.Grants)
	}
	composed := derived.Grants[len(derived.Grants)-1]
	if composed.Issuer != "https://home.invalid:15556" || composed.Subject != "home-admin" ||
		!reflect.DeepEqual(composed.Organizations, []string{"velocity"}) ||
		!reflect.DeepEqual(composed.Permissions, permissions) {
		t.Fatalf("composed enrollment session grant = %+v", composed)
	}
	// Re-deriving without the session grants must fail closed on the
	// differing existing target, never silently narrow the derived trust.
	if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, ""); err == nil ||
		!strings.Contains(err.Error(), "different content") {
		t.Fatalf("narrowed re-derivation accepted: %v", err)
	}
	// Out-of-bound session permissions are refused.
	bad := writeTestComposedSessionConfig(t, "https://home.invalid:15556", "home-admin",
		[]string{"velocity"}, []string{"monitor.read"})
	if err := deriveEvaluationSimulatorEnrollmentTrust(f.state, input, bad); err == nil ||
		!strings.Contains(err.Error(), "bounded to the Simulator session permissions") {
		t.Fatalf("out-of-bound enrollment session grant accepted: %v", err)
	}
}
