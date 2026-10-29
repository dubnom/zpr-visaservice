//go:build (linux || darwin) && !production

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/simulatorreset"
)

func TestSimulatorResetRestartAcceptance(t *testing.T) {
	for _, test := range []struct {
		name      string
		operation string
		state     string
	}{
		{name: "consumed-before-outcome", operation: "a1111111111111111111111111111111", state: "indeterminate"},
		{name: "pending-intent", operation: "b2222222222222222222222222222222", state: "interrupted"},
		{name: "completed-retry", operation: "c3333333333333333333333333333333", state: "completed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runSimulatorResetRestartAcceptance(t, test.operation, test.state)
		})
	}
}

func runSimulatorResetRestartAcceptance(t *testing.T, operationID, wantState string) {
	t.Helper()
	ctx := context.Background()
	const organization = "restart-acceptance-lab"
	actor := simulatorResetRestartIdentity()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		t.Fatalf("temporary database root is not canonical: %q", directory)
	}
	databasePath := filepath.Join(directory, "private", "reset.sqlite")
	store, err := simulatorreset.Open(databasePath, organization, time.Now)
	if err != nil {
		t.Fatalf("open isolated reset database: %v", err)
	}
	initialEpoch := store.Epoch()
	binding := simulatorreset.Binding{
		Organization: organization,
		Actor:        simulatorreset.Actor{Issuer: actor.Issuer, Subject: actor.Subject},
		Action:       simulatorreset.ActionScenarioStateReset,
		Epoch:        initialEpoch,
		Generation:   1,
	}
	intent, err := store.BeginIntent(ctx, operationID, binding)
	if err != nil || !intent.Created || intent.Operation.State != "intent" {
		_ = store.Close()
		t.Fatalf("seed reset intent: result=%+v err=%v", intent, err)
	}
	switch wantState {
	case "indeterminate":
		confirmed, confirmErr := store.ConfirmIntent(ctx, operationID, binding)
		if confirmErr != nil || !confirmed.Consumed || confirmed.Operation.State != "consumed" {
			_ = store.Close()
			t.Fatalf("seed consumed-before-outcome boundary: result=%+v err=%v", confirmed, confirmErr)
		}
	case "completed":
		confirmed, confirmErr := store.ConfirmIntent(ctx, operationID, binding)
		if confirmErr != nil || !confirmed.Consumed {
			_ = store.Close()
			t.Fatalf("seed completed boundary confirmation: result=%+v err=%v", confirmed, confirmErr)
		}
		if _, err := store.RecordOutcome(ctx, operationID, binding, simulatorreset.Outcome{State: "completed"}); err != nil {
			_ = store.Close()
			t.Fatalf("seed completed outcome: %v", err)
		}
	case "interrupted":
	default:
		_ = store.Close()
		t.Fatalf("unsupported expected state %q", wantState)
	}
	seeded, err := store.Operation(ctx, operationID)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	seededAudit, err := store.Audit(ctx, operationID)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seeded reset store: %v", err)
	}

	reopened, err := simulatorreset.Open(databasePath, organization, time.Now)
	if err != nil {
		t.Fatalf("reopen isolated reset database: %v", err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("close reopened reset store: %v", err)
		}
	}()
	if reopened.Epoch() != initialEpoch+1 {
		t.Fatalf("reopened epoch = %d, want %d", reopened.Epoch(), initialEpoch+1)
	}
	reconciled, err := reopened.Operation(ctx, operationID)
	if err != nil || reconciled.Binding != binding || reconciled.State != wantState {
		t.Fatalf("restart reconciliation: operation=%+v err=%v, want state %q and original binding", reconciled, err, wantState)
	}
	reconciledAudit, err := reopened.Audit(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if wantState == "completed" {
		if !reflect.DeepEqual(reconciledAudit, seededAudit) {
			t.Fatalf("completed receipt audit changed on reopen: before=%+v after=%+v", seededAudit, reconciledAudit)
		}
	} else {
		wantKind := "consumed_operation_indeterminate_on_restart"
		if wantState == "interrupted" {
			wantKind = "intent_interrupted_on_restart"
		}
		if len(reconciledAudit) != len(seededAudit)+1 ||
			reconciledAudit[len(reconciledAudit)-1].Kind != wantKind ||
			reconciledAudit[len(reconciledAudit)-1].ActorKind != "system" {
			t.Fatalf("restart audit did not record exactly the seeded boundary: %+v", reconciledAudit)
		}
	}

	manager := newSimulatorScenarioManager()
	manager.run = simulatorScenarioRun{
		ScenarioID:     "retained-terminal-scenario",
		ScenarioName:   "Synthetic retained terminal scenario",
		OrganizationID: organization,
		State:          "completed",
		Generation:     manager.generation,
		Steps:          []simulatorScenarioStepResult{{Number: 1, Phase: "scenario", Action: "synthetic", Status: "completed"}},
	}
	beforeSnapshot := manager.snapshot()
	beforeGeneration := manager.generation
	handler := simulatorResetRestartAuthorizedHandler(
		simulatorResetTestMux(newSimulatorResetHandler(reopened, organization, manager)), actor)
	operationPath := "/api/simulator/organizations/" + organization + "/scenario-reset/operations/" + operationID
	operationResponse := simulatorResetRestartServe(handler, simulatorResetRequest(http.MethodGet, operationPath, ""))
	var status simulatorResetOperationResponse
	if operationResponse.Code != http.StatusOK ||
		json.Unmarshal(operationResponse.Body.Bytes(), &status) != nil ||
		status.OperationID != operationID || status.State != wantState ||
		status.Actor.Issuer != actor.Issuer || status.Actor.Subject != actor.Subject ||
		status.Epoch != initialEpoch || status.CurrentEpoch != initialEpoch+1 ||
		status.Generation != binding.Generation {
		t.Fatalf("reopened HTTP operation status=%d body=%s", operationResponse.Code, operationResponse.Body.String())
	}

	confirmPath := operationPath + "/confirm"
	confirmBody, err := json.Marshal(simulatorResetConfirmationRequest{
		Organization: organization, Epoch: binding.Epoch, Generation: binding.Generation,
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed := simulatorResetRestartServe(handler,
		simulatorResetRequest(http.MethodPost, confirmPath, string(confirmBody)))
	var confirmedStatus simulatorResetOperationResponse
	if confirmed.Code != http.StatusOK ||
		json.Unmarshal(confirmed.Body.Bytes(), &confirmedStatus) != nil ||
		confirmedStatus.State != wantState || confirmedStatus.OperationID != operationID {
		t.Fatalf("repeat HTTP confirmation status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}
	if got := manager.snapshot(); !reflect.DeepEqual(got, beforeSnapshot) || manager.generation != beforeGeneration {
		t.Fatalf("repeat confirmation mutated re-composed manager: snapshot=%+v generation=%d", got, manager.generation)
	}
	afterRetry, err := reopened.Operation(ctx, operationID)
	if err != nil || !reflect.DeepEqual(afterRetry, reconciled) {
		t.Fatalf("repeat HTTP confirmation changed durable operation: before=%+v after=%+v err=%v", reconciled, afterRetry, err)
	}
	afterRetryAudit, err := reopened.Audit(ctx, operationID)
	if err != nil || !reflect.DeepEqual(afterRetryAudit, reconciledAudit) {
		t.Fatalf("repeat HTTP confirmation changed audit: before=%+v after=%+v err=%v", reconciledAudit, afterRetryAudit, err)
	}

	var changed simulatorResetConfirmationRequest
	if err := json.Unmarshal(confirmBody, &changed); err != nil {
		t.Fatal(err)
	}
	changed.Generation++
	wrongBinding, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	wrongConfirm := simulatorResetRestartServe(handler,
		simulatorResetRequest(http.MethodPost, confirmPath, string(wrongBinding)))
	if wrongConfirm.Code != http.StatusConflict {
		t.Fatalf("wrong-binding HTTP confirmation status=%d body=%s, want 409", wrongConfirm.Code, wrongConfirm.Body.String())
	}

	staleID := "d4444444444444444444444444444444"
	intentPath := "/api/simulator/organizations/" + organization + "/scenario-reset/intents"
	staleBody, err := json.Marshal(simulatorResetIntentRequest{
		OperationID: staleID, Organization: organization, Epoch: initialEpoch, Generation: manager.generation,
	})
	if err != nil {
		t.Fatal(err)
	}
	staleIntent := simulatorResetRestartServe(handler,
		simulatorResetRequest(http.MethodPost, intentPath, string(staleBody)))
	if staleIntent.Code != http.StatusConflict {
		t.Fatalf("new stale-epoch HTTP intent status=%d body=%s, want 409", staleIntent.Code, staleIntent.Body.String())
	}
	if _, err := reopened.Operation(ctx, staleID); !errors.Is(err, simulatorreset.ErrNotFound) {
		t.Fatalf("stale-epoch operation persisted after conflict: %v", err)
	}
	if got := manager.snapshot(); !reflect.DeepEqual(got, beforeSnapshot) || manager.generation != beforeGeneration {
		t.Fatalf("stale-epoch request mutated retained scenario: snapshot=%+v generation=%d", got, manager.generation)
	}
	t.Logf("seeded %s boundary; graceful store close/reopen epoch %d -> %d; HTTP status and retries retained; stale epoch rejected",
		seeded.State, initialEpoch, reopened.Epoch())
}

func simulatorResetRestartIdentity() operatorauth.Identity {
	return operatorauth.Identity{Issuer: "https://issuer.restart-acceptance.invalid", Subject: "synthetic-reviewer"}
}

func simulatorResetRestartAuthorizedHandler(next http.Handler, identity operatorauth.Identity) http.Handler {
	authorize := productionContractAuthorizer(func(r *http.Request, organization, permission string) (operatorauth.Identity, error) {
		if organization != "restart-acceptance-lab" || permission != simulatorreset.ActionScenarioStateReset {
			return operatorauth.Identity{}, errors.New("unexpected reset authorization scope")
		}
		if r.Method != http.MethodGet &&
			(r.Header.Get("Origin") != "https://simulator.example" || r.Header.Get("X-ZPR-CSRF") != "reset-test-csrf") {
			return operatorauth.Identity{}, errors.New("reset mutation lacks synthetic CSRF proof")
		}
		return identity, nil
	})
	return simulatorAPIProxy(authorize, func(*http.Request) (string, error) {
		return "", errors.New("explicit reset route must not resolve active organization")
	}, next)
}

func simulatorResetRestartServe(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
