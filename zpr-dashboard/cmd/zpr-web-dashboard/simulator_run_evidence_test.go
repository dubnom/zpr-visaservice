//go:build !production

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testRunEvidenceRunID = "run-" + "0123456789abcdef0123456789abcdef"

// TestMain keeps every test in this binary out of the real user configuration
// directory: run admission and workspace stores default to a private
// temporary directory unless a test explicitly overrides it with t.Setenv.
func TestMain(m *testing.M) {
	code := func() int {
		if os.Getenv("SIMULATION_WORKSPACE_DB_DIR") == "" {
			directory, err := os.MkdirTemp("", "zpr-simulator-test-workspace-")
			if err != nil {
				return 1
			}
			defer os.RemoveAll(directory)
			if err := os.Chmod(directory, 0o700); err != nil {
				return 1
			}
			if err := os.Setenv("SIMULATION_WORKSPACE_DB_DIR", directory); err != nil {
				return 1
			}
		}
		return m.Run()
	}()
	os.Exit(code)
}

func testRunEvidenceDir(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", directory)
	return directory
}

func TestSimulatorRunEvidenceJournalRoundTrip(t *testing.T) {
	directory := testRunEvidenceDir(t)
	allocated := simulatorRunEvidenceRecord{
		Event: simulatorRunEvidenceAllocated, RunID: testRunEvidenceRunID,
		RunNamespace: "org/scn/" + testRunEvidenceRunID, OrganizationID: "velocity",
		ScenarioID: "scn", PlanDigest: "digest", ApprovalReference: "approval-ref",
		ActorIssuer: "https://issuer.test", ActorSubject: "operator",
	}
	if err := appendSimulatorRunEvidence(allocated); err != nil {
		t.Fatal(err)
	}
	if err := appendSimulatorRunEvidence(simulatorRunEvidenceRecord{
		Event: simulatorRunEvidenceFinished, RunID: testRunEvidenceRunID,
		OrganizationID: "velocity", ScenarioID: "scn", State: "passed",
	}); err != nil {
		t.Fatal(err)
	}
	// Records for other runs and organizations are not returned.
	other := allocated
	other.RunID = "run-" + strings.Repeat("f", 32)
	if err := appendSimulatorRunEvidence(other); err != nil {
		t.Fatal(err)
	}
	records, err := readSimulatorRunEvidence("velocity", testRunEvidenceRunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Event != simulatorRunEvidenceAllocated ||
		records[1].Event != simulatorRunEvidenceFinished || records[1].State != "passed" ||
		records[0].ActorSubject != "operator" || records[0].RecordedAt.IsZero() {
		t.Fatalf("unexpected journal records: %+v", records)
	}
	info, err := os.Stat(filepath.Join(directory, "velocity-run-evidence.jsonl"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("journal is not a private file: %v %v", info, err)
	}
	if records, err := readSimulatorRunEvidence("other-org", testRunEvidenceRunID); err != nil || len(records) != 0 {
		t.Fatalf("missing journal must yield no records: %v %v", records, err)
	}
}

func TestSimulatorRunEvidenceFailsClosed(t *testing.T) {
	directory := testRunEvidenceDir(t)
	if err := appendSimulatorRunEvidence(simulatorRunEvidenceRecord{
		Event: "invented", RunID: testRunEvidenceRunID, OrganizationID: "velocity", ScenarioID: "scn",
	}); err == nil {
		t.Fatal("unsupported event accepted")
	}
	if err := appendSimulatorRunEvidence(simulatorRunEvidenceRecord{
		Event: simulatorRunEvidenceFinished, RunID: testRunEvidenceRunID,
		OrganizationID: "velocity", ScenarioID: "scn",
	}); err == nil {
		t.Fatal("finished record without terminal state accepted")
	}
	path := filepath.Join(directory, "velocity-run-evidence.jsonl")
	if err := os.WriteFile(path, []byte("{\"version\":1,\"event\":\"allocated\"corrupt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSimulatorRunEvidence("velocity", testRunEvidenceRunID); err == nil ||
		!strings.Contains(err.Error(), "inspect") {
		t.Fatalf("corrupt journal did not fail closed: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := appendSimulatorRunEvidence(simulatorRunEvidenceRecord{
		Event: simulatorRunEvidenceAllocated, RunID: testRunEvidenceRunID,
		OrganizationID: "velocity", ScenarioID: "scn",
	}); err == nil {
		t.Fatal("group/other-readable journal accepted for append")
	}
}

func TestSimulatorRunEvidenceReadbackOutcomes(t *testing.T) {
	testRunEvidenceDir(t)
	request := func(organization, run string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet,
			"/api/simulator/organizations/"+organization+"/runs/"+run+"/evidence", nil)
		r.SetPathValue("organization", organization)
		r.SetPathValue("run", run)
		r = r.WithContext(context.WithValue(r.Context(), simulatorAuthorizedOrganizationContextKey{}, organization))
		handleSimulatorRunEvidence(recorder, r)
		return recorder
	}
	decode := func(t *testing.T, recorder *httptest.ResponseRecorder) simulatorRunEvidenceResponse {
		t.Helper()
		if recorder.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		var response simulatorRunEvidenceResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}

	if response := decode(t, request("velocity", testRunEvidenceRunID)); response.Outcome != "unknown" {
		t.Fatalf("missing evidence outcome=%q", response.Outcome)
	}

	if err := appendSimulatorRunEvidence(simulatorRunEvidenceRecord{
		Event: simulatorRunEvidenceAllocated, RunID: testRunEvidenceRunID,
		OrganizationID: "velocity", ScenarioID: "scn",
	}); err != nil {
		t.Fatal(err)
	}
	response := decode(t, request("velocity", testRunEvidenceRunID))
	if response.Outcome != "uncertain" || !strings.Contains(response.Advisory, "never automatically retried") {
		t.Fatalf("allocated-only readback=%+v", response)
	}

	if err := appendSimulatorRunEvidence(simulatorRunEvidenceRecord{
		Event: simulatorRunEvidenceFinished, RunID: testRunEvidenceRunID,
		OrganizationID: "velocity", ScenarioID: "scn", State: "failed", Error: "step 3 failed",
	}); err != nil {
		t.Fatal(err)
	}
	response = decode(t, request("velocity", testRunEvidenceRunID))
	if response.Outcome != "finished" || response.State != "failed" || len(response.Records) != 2 {
		t.Fatalf("finished readback=%+v", response)
	}
	if !strings.Contains(response.Scope, "not remote acceptance") {
		t.Fatalf("missing evidence scope statement: %+v", response)
	}

	notStarted := "run-" + strings.Repeat("a", 32)
	recordSimulatorRunNotStarted(notStarted, "velocity", "scn", errSimulatorScenarioApprovalPlanMismatch)
	response = decode(t, request("velocity", notStarted))
	if response.Outcome != "not-started" || !strings.Contains(response.Advisory, "does not match") {
		t.Fatalf("not-started readback=%+v", response)
	}

	if recorder := request("velocity", "run-invalid"); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid run ID status=%d", recorder.Code)
	}
	recorder := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/simulator/organizations/velocity/runs/"+testRunEvidenceRunID+"/evidence", nil)
	r.SetPathValue("organization", "velocity")
	r.SetPathValue("run", testRunEvidenceRunID)
	r = r.WithContext(context.WithValue(r.Context(), simulatorAuthorizedOrganizationContextKey{}, "other-org"))
	handleSimulatorRunEvidence(recorder, r)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-organization readback status=%d", recorder.Code)
	}
}
