//go:build linux || darwin

package simulatorreset_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/simulatorreset"
)

const (
	resetAcceptanceHelperEnv = "ZPR_RESET_STORE_ACCEPTANCE_HELPER"
	resetAcceptancePathEnv   = "ZPR_RESET_STORE_ACCEPTANCE_PATH"
	resetAcceptanceModeEnv   = "ZPR_RESET_STORE_ACCEPTANCE_MODE"
	resetAcceptanceOrg       = "acceptance-disposable-lab"
)

func TestResetStoreAcceptanceRestartReconcilesAbruptChildExit(t *testing.T) {
	for _, mode := range []string{"consumed", "pending", "concurrent"} {
		t.Run(mode, func(t *testing.T) {
			runResetStoreAcceptanceRestart(t, mode)
		})
	}
}

func TestResetStoreAcceptanceSubprocessHelper(t *testing.T) {
	if os.Getenv(resetAcceptanceHelperEnv) != "1" {
		return
	}

	path := os.Getenv(resetAcceptancePathEnv)
	root := filepath.Dir(filepath.Dir(path))
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil || path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path ||
		canonicalRoot != root || path != filepath.Join(root, "store", "reset.sqlite") {
		resetAcceptanceChildFail("database path is not canonical: %q (%v)", path, err)
	}
	mode := os.Getenv(resetAcceptanceModeEnv)
	if mode != "consumed" && mode != "pending" && mode != "concurrent" {
		resetAcceptanceChildFail("invalid helper mode %q", mode)
	}

	store, err := simulatorreset.Open(path, resetAcceptanceOrg, time.Now)
	if err != nil {
		resetAcceptanceChildFail("open owned acceptance store: %v", err)
	}
	ctx := context.Background()
	binding := simulatorreset.Binding{
		Organization: resetAcceptanceOrg,
		Actor: simulatorreset.Actor{
			Issuer:  "https://issuer.acceptance.invalid",
			Subject: "synthetic-operator",
		},
		Action:     simulatorreset.ActionScenarioStateReset,
		Epoch:      store.Epoch(),
		Generation: 41,
	}
	id := "a1111111111111111111111111111111"
	if mode == "pending" {
		id = "b2222222222222222222222222222222"
	}
	if mode == "concurrent" {
		id = "c3333333333333333333333333333333"
	}
	intent, err := store.BeginIntent(ctx, id, binding)
	if err != nil || !intent.Created || intent.Operation.State != "intent" {
		resetAcceptanceChildFail("create intent: result=%+v err=%v", intent, err)
	}

	if mode == "consumed" {
		confirmation, err := store.ConfirmIntent(ctx, id, binding)
		if err != nil || !confirmation.Consumed || confirmation.Operation.State != "consumed" {
			resetAcceptanceChildFail("consume intent: result=%+v err=%v", confirmation, err)
		}
	}
	if mode == "concurrent" {
		const confirmations = 24
		start := make(chan struct{})
		results := make(chan error, confirmations)
		var workers sync.WaitGroup
		for range confirmations {
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-start
				result, err := store.ConfirmIntent(ctx, id, binding)
				if err != nil {
					results <- err
					return
				}
				if result.Consumed {
					results <- nil
					return
				}
				results <- nil
			}()
		}
		close(start)
		workers.Wait()
		close(results)
		for err := range results {
			if err != nil {
				resetAcceptanceChildFail("concurrent confirmation: %v", err)
			}
		}
		events, err := store.Audit(ctx, id)
		if err != nil {
			resetAcceptanceChildFail("read child audit: %v", err)
		}
		confirmed := 0
		for _, event := range events {
			if event.Kind == "intent_confirmed" {
				confirmed++
			}
		}
		if confirmed != 1 {
			resetAcceptanceChildFail("child confirmation audit count = %d, want 1", confirmed)
		}
	}

	if mode == "pending" {
		op, err := store.Operation(ctx, id)
		if err != nil || op.State != "intent" {
			resetAcceptanceChildFail("pending intent before exit: operation=%+v err=%v", op, err)
		}
	}

	// Deliberately bypass Close: process teardown, not graceful store shutdown,
	// is the persistence boundary under test.
	os.Exit(0)
}

func runResetStoreAcceptanceRestart(t *testing.T, mode string) {
	t.Helper()
	path := filepath.Join(canonicalTempDir(t), "store", "reset.sqlite")
	child := startResetStoreAcceptanceChild(t, path, mode)
	initialEpoch := child.epoch

	store, err := simulatorreset.Open(path, resetAcceptanceOrg, time.Now)
	if err != nil {
		t.Fatalf("reopen retained child database: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Errorf("close parent acceptance store: %v", err)
		}
	}()
	if got, want := store.Epoch(), initialEpoch+1; got != want {
		t.Fatalf("reopened epoch = %d, want %d", got, want)
	}

	ctx := context.Background()
	before, err := store.Operation(ctx, child.id)
	if err != nil {
		t.Fatal(err)
	}
	wantState := "indeterminate"
	if mode == "pending" {
		wantState = "interrupted"
	}
	if before.State != wantState {
		t.Fatalf("reconciled state = %q, want %q", before.State, wantState)
	}
	if before.Binding != child.binding {
		t.Fatalf("restart changed exact operation binding: got %+v want %+v", before.Binding, child.binding)
	}

	auditBefore, err := store.Audit(ctx, child.id)
	if err != nil {
		t.Fatal(err)
	}
	exact, err := store.BeginIntent(ctx, child.id, child.binding)
	if err != nil || exact.Created || !reflect.DeepEqual(exact.Operation, before) {
		t.Fatalf("exact intent retry mutated or recreated operation: result=%+v err=%v", exact, err)
	}
	confirmation, err := store.ConfirmIntent(ctx, child.id, child.binding)
	if err != nil || confirmation.Consumed || !reflect.DeepEqual(confirmation.Operation, before) {
		t.Fatalf("exact confirmation retry consumed or mutated operation: result=%+v err=%v", confirmation, err)
	}
	auditAfter, err := store.Audit(ctx, child.id)
	if err != nil || !reflect.DeepEqual(auditAfter, auditBefore) {
		t.Fatalf("exact retries changed audit history: before=%+v after=%+v err=%v", auditBefore, auditAfter, err)
	}

	wrong := child.binding
	wrong.Generation++
	if _, err := store.BeginIntent(ctx, child.id, wrong); !errors.Is(err, simulatorreset.ErrConflict) {
		t.Fatalf("wrong-binding intent retry error = %v, want ErrConflict", err)
	}
	if _, err := store.ConfirmIntent(ctx, child.id, wrong); !errors.Is(err, simulatorreset.ErrConflict) {
		t.Fatalf("wrong-binding confirmation error = %v, want ErrConflict", err)
	}
	afterWrong, err := store.Operation(ctx, child.id)
	if err != nil || !reflect.DeepEqual(afterWrong, before) {
		t.Fatalf("wrong binding changed operation: before=%+v after=%+v err=%v", before, afterWrong, err)
	}

	events, err := store.Audit(ctx, child.id)
	if err != nil {
		t.Fatal(err)
	}
	wantRestartEvent := "consumed_operation_indeterminate_on_restart"
	wantAuditEvents := 3
	if mode == "pending" {
		wantRestartEvent = "intent_interrupted_on_restart"
		wantAuditEvents = 2
	}
	if len(events) != wantAuditEvents ||
		events[len(events)-1].Kind != wantRestartEvent || events[len(events)-1].ActorKind != "system" {
		t.Fatalf("restart audit evidence = %+v, want one system %q event", events, wantRestartEvent)
	}
	t.Logf("%s: owned child PID %d exited without Close; epoch %d -> %d; state %s; retries immutable",
		mode, child.pid, initialEpoch, store.Epoch(), before.State)
}

type resetAcceptanceChild struct {
	pid     int
	id      string
	epoch   uint64
	binding simulatorreset.Binding
}

func startResetStoreAcceptanceChild(t *testing.T, path, mode string) resetAcceptanceChild {
	t.Helper()
	parent := filepath.Dir(filepath.Dir(path))
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(parent)
	if err != nil || canonical != parent {
		t.Fatalf("test database parent is not canonical: %q (%v)", parent, err)
	}

	command := exec.Command(os.Args[0], "-test.run=^TestResetStoreAcceptanceSubprocessHelper$")
	command.Env = resetAcceptanceChildEnv(os.Environ(), path, mode)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatalf("start owned acceptance child: %v", err)
	}
	pid := command.Process.Pid
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("acceptance child PID %d: %v\n%s", pid, err, output.String())
		}
	case <-time.After(20 * time.Second):
		_ = command.Process.Kill()
		err := <-wait
		t.Fatalf("acceptance child PID %d exceeded deadline (wait=%v)\n%s", pid, err, output.String())
	}

	// The child writes this deterministic identity only by completing its
	// scenario. Its DB binding uses the initial epoch, which is 1 in a fresh DB.
	id := "a1111111111111111111111111111111"
	if mode == "pending" {
		id = "b2222222222222222222222222222222"
	}
	if mode == "concurrent" {
		id = "c3333333333333333333333333333333"
	}
	return resetAcceptanceChild{
		pid: pid, id: id, epoch: 1,
		binding: simulatorreset.Binding{
			Organization: resetAcceptanceOrg,
			Actor: simulatorreset.Actor{
				Issuer:  "https://issuer.acceptance.invalid",
				Subject: "synthetic-operator",
			},
			Action: simulatorreset.ActionScenarioStateReset, Epoch: 1, Generation: 41,
		},
	}
}

func resetAcceptanceChildEnv(base []string, path, mode string) []string {
	env := make([]string, 0, len(base)+3)
	for _, item := range base {
		if strings.HasPrefix(item, resetAcceptanceHelperEnv+"=") ||
			strings.HasPrefix(item, resetAcceptancePathEnv+"=") ||
			strings.HasPrefix(item, resetAcceptanceModeEnv+"=") {
			continue
		}
		env = append(env, item)
	}
	return append(env,
		resetAcceptanceHelperEnv+"=1",
		resetAcceptancePathEnv+"="+path,
		resetAcceptanceModeEnv+"="+mode,
	)
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	raw := t.TempDir()
	canonical, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(canonical) || filepath.Clean(canonical) != canonical {
		t.Fatalf("temporary directory is not a canonical absolute path: %q", canonical)
	}
	return canonical
}

func resetAcceptanceChildFail(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
