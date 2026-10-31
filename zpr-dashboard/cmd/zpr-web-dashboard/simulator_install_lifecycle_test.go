//go:build !production

package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func lifecycleTestRegistry(selected *string, calls *[]string) *simulatorEvaluationInstalls {
	return &simulatorEvaluationInstalls{
		installs: map[string]simulatorEvaluationInstall{
			"northstar":   {LifecycleCommand: []string{"/operator/supervisor", "northstar"}, OperatorServiceURL: "https://northstar"},
			"great-lakes": {LifecycleCommand: []string{"/operator/supervisor", "great-lakes"}, OperatorServiceURL: "https://great-lakes"},
			"redwood":     {OperatorServiceURL: "https://redwood"},
		},
		selectOrganization: func() (string, error) { return *selected, nil },
		runLifecycle: func(_ context.Context, command []string, action string) error {
			*calls = append(*calls, action+" "+command[1])
			return nil
		},
		readSnapshot: func(_ context.Context, target simulatorOperatorTarget) error {
			*calls = append(*calls, "ready "+strings.TrimPrefix(target.ServiceURL, "https://"))
			return nil
		},
	}
}

func TestSimulatorInstallLifecycleTransaction(t *testing.T) {
	for _, next := range []string{"northstar", "great-lakes", "redwood"} {
		t.Run(next, func(t *testing.T) {
			selected := "northstar"
			calls := []string{}
			registry := lifecycleTestRegistry(&selected, &calls)
			err := registry.activateAndCommit(context.Background(), next, "reset-organization", func() error {
				calls = append(calls, "commit "+next)
				selected = next
				return nil
			})
			want := []string{"start " + next, "ready " + next}
			if next == "redwood" {
				want = []string{"ready redwood"}
			}
			if next != "northstar" {
				want = append(want, "stop northstar")
			}
			want = append(want, "commit "+next)
			if err != nil || selected != next || !reflect.DeepEqual(calls, want) {
				t.Fatalf("switch: selected=%s calls=%v err=%v; want %v", selected, calls, err, want)
			}
		})
	}
}

func TestSimulatorInstallLifecycleFailures(t *testing.T) {
	for _, failure := range []string{"start", "ready", "stop", "commit", "cleanup", "recovery", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			selected := "northstar"
			calls := []string{}
			registry := lifecycleTestRegistry(&selected, &calls)
			run := registry.runLifecycle
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			registry.runLifecycle = func(ctx context.Context, command []string, action string) error {
				if (action == "start" && command[1] == "northstar") || (action == "stop" && command[1] == "great-lakes") {
					if ctx.Err() != nil {
						t.Error("recovery or cleanup inherited the cancelled activation context")
					}
				}
				if err := run(ctx, command, action); err != nil {
					return err
				}
				if failure == "cancel" && action == "stop" && command[1] == "northstar" {
					cancel()
				}
				if (failure == "start" && action == "start" && command[1] == "great-lakes") ||
					(failure == "stop" && action == "stop" && command[1] == "northstar") ||
					(failure == "cleanup" && action == "stop" && command[1] == "great-lakes") ||
					(failure == "recovery" && action == "start" && command[1] == "northstar") {
					return errors.New("injected " + failure)
				}
				return nil
			}
			if failure == "ready" {
				read := registry.readSnapshot
				registry.readSnapshot = func(ctx context.Context, target simulatorOperatorTarget) error {
					_ = read(ctx, target)
					cancel()
					return errors.New("not ready")
				}
			}
			err := registry.activateAndCommit(ctx, "great-lakes", "activate", func() error {
				calls = append(calls, "commit")
				return errors.New("injected commit")
			})
			want := []string{"start great-lakes"}
			if failure != "start" {
				want = append(want, "ready great-lakes")
				if failure != "ready" {
					want = append(want, "stop northstar")
					if failure != "stop" && failure != "cancel" {
						want = append(want, "commit")
					}
					want = append(want, "start northstar")
					if failure != "recovery" {
						want = append(want, "ready northstar")
					}
				}
			}
			want = append(want, "stop great-lakes")
			if err == nil || selected != "northstar" || !reflect.DeepEqual(calls, want) {
				t.Fatalf("rollback: selected=%s calls=%v err=%v; want %v", selected, calls, err, want)
			}
			if failure == "cleanup" && !strings.Contains(err.Error(), "injected cleanup") {
				t.Fatal("cleanup failure was hidden", err)
			}
			if failure == "recovery" && !strings.Contains(err.Error(), "recovery failed") {
				t.Fatal("recovery failure was hidden", err)
			}
		})
	}
}

func TestSimulatorInstallLifecycleSameOrganizationFailurePreservesRuntime(t *testing.T) {
	selected := "northstar"
	calls := []string{}
	registry := lifecycleTestRegistry(&selected, &calls)
	err := registry.activateAndCommit(context.Background(), selected, "activate", func() error { return errors.New("save failed") })
	want := []string{"start northstar", "ready northstar"}
	if err == nil || !reflect.DeepEqual(calls, want) {
		t.Fatalf("same-org runtime stopped: %v, %v", calls, err)
	}
}

func TestSimulatorInstallLifecycleSameOrganizationStartFailure(t *testing.T) {
	selected := "northstar"
	calls := []string{}
	registry := lifecycleTestRegistry(&selected, &calls)
	run := registry.runLifecycle
	registry.runLifecycle = func(ctx context.Context, command []string, action string) error {
		_ = run(ctx, command, action)
		return errors.New("already selected runtime unavailable")
	}
	if err := registry.activateAndCommit(context.Background(), selected, "activate", func() error { t.Fatal("failed start committed"); return nil }); err == nil {
		t.Fatal("start failure ignored")
	}
	if !reflect.DeepEqual(calls, []string{"start northstar"}) {
		t.Fatal("selected runtime was stopped after failed start", calls)
	}
}

func TestSimulatorInstallLifecycleLeavesIndependentRuntimeRunning(t *testing.T) {
	selected := "redwood"
	calls := []string{}
	registry := lifecycleTestRegistry(&selected, &calls)
	err := registry.activateAndCommit(context.Background(), "great-lakes", "activate", func() error { return nil })
	want := []string{"start great-lakes", "ready great-lakes"}
	if err != nil || !reflect.DeepEqual(calls, want) {
		t.Fatalf("independent runtime touched: %v, %v", calls, err)
	}
}

func TestSimulatorInstallLifecycleValidation(t *testing.T) {
	for name, mutate := range map[string]func(string, string, *[]string){
		"valid":    func(_, _ string, _ *[]string) {},
		"relative": func(_, _ string, command *[]string) { (*command)[0] = "supervisor" },
		"newline":  func(_, _ string, command *[]string) { *command = append(*command, "bad\nargument") },
		"empty":    func(_, _ string, command *[]string) { *command = append(*command, "") },
		"long":     func(_, _ string, command *[]string) { *command = append(*command, strings.Repeat("x", 4097)) },
		"many": func(_, _ string, command *[]string) {
			for range 17 {
				*command = append(*command, "arg")
			}
		},
		"public registry": func(path, _ string, _ *[]string) {
			if err := os.Chmod(path, 0644); err != nil {
				panic(err)
			}
		},
		"writable executable": func(_, executable string, _ *[]string) {
			if err := os.Chmod(executable, 0777); err != nil {
				panic(err)
			}
		},
		"not executable": func(_, executable string, _ *[]string) {
			if err := os.Chmod(executable, 0600); err != nil {
				panic(err)
			}
		},
		"symlink": func(path, _ string, _ *[]string) {
			if err := os.Rename(path, path+".real"); err != nil {
				panic(err)
			}
			if err := os.Symlink(path+".real", path); err != nil {
				panic(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "registry.json")
			executable := filepath.Join(t.TempDir(), "supervisor")
			if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(executable, []byte("fixture"), 0700); err != nil {
				t.Fatal(err)
			}
			command := []string{executable, "--config", "operator.json"}
			mutate(path, executable, &command)
			err := validateSimulatorInstallLifecycle(path, command)
			if (err == nil) != (name == "valid") {
				t.Fatalf("%s: %v", name, err)
			}
		})
	}
}

func TestSimulatorInstallLifecycleWaitsForWorkers(t *testing.T) {
	selected := "northstar"
	calls := []string{}
	registry := lifecycleTestRegistry(&selected, &calls)
	started, stopped := make(chan struct{}), make(chan struct{})
	var once sync.Once
	plane := &simulatorDataPlaneEndpoints{
		status: map[string]simulatorDataPlaneEndpointStatus{},
		docker: func(ctx context.Context, _ ...string) (string, error) {
			once.Do(func() { close(started) })
			<-ctx.Done()
			close(stopped)
			return "", ctx.Err()
		},
	}
	registry.endpoints = map[string]*simulatorDataPlaneEndpoints{"northstar": plane, "great-lakes": plane}
	if err := registry.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer registry.close()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("selected worker did not start")
	}
	if len(registry.workers) != 1 {
		t.Fatalf("inactive workers started: %v", registry.workers)
	}
	run := registry.runLifecycle
	registry.runLifecycle = func(ctx context.Context, command []string, action string) error {
		if action == "stop" && command[1] == "northstar" {
			select {
			case <-stopped:
			default:
				t.Error("runtime stopped before worker exited")
			}
		}
		return run(ctx, command, action)
	}
	// No new worker is needed for this test; the next runtime has no handoff.
	delete(registry.endpoints, "great-lakes")
	if err := registry.activateAndCommit(context.Background(), "great-lakes", "activate", func() error { selected = "great-lakes"; return nil }); err != nil {
		t.Fatal(err)
	}
	if len(registry.workers) != 0 {
		t.Fatal("inactive workers retained", registry.workers)
	}
}

func TestSimulatorInstallLifecycleManagerCommitFailure(t *testing.T) {
	selected := "northstar"
	calls := []string{}
	registry := lifecycleTestRegistry(&selected, &calls)
	manager := &organizationActivationManager{
		run:      organizationActivation{State: "idle"},
		activate: registry.activateAndCommit,
	}
	selection := filepath.Join(t.TempDir(), "missing-directory", "active")
	if !manager.start("great-lakes", selection) {
		t.Fatal("activation rejected")
	}
	select {
	case <-manager.done:
	case <-time.After(time.Second):
		t.Fatal("activation did not complete")
	}
	if state := manager.snapshot(); state.State != "failed" || !strings.Contains(state.Error, "save active organization") {
		t.Fatalf("status: %+v", state)
	}
	want := []string{"start great-lakes", "ready great-lakes", "stop northstar", "start northstar", "ready northstar", "stop great-lakes"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("selection save failure did not restore runtime: %v", calls)
	}
}

func TestSimulatorInstallLifecycleRestartRecovery(t *testing.T) {
	for _, selectedID := range []string{"northstar", "redwood"} {
		t.Run(selectedID, func(t *testing.T) {
			selected := selectedID
			calls := []string{}
			registry := lifecycleTestRegistry(&selected, &calls)
			if err := registry.run(context.Background()); err != nil {
				t.Fatal(err)
			}
			registry.close()
			want := []string{"start northstar", "ready northstar", "stop great-lakes"}
			if selected == "redwood" {
				want = []string{"stop great-lakes", "stop northstar"}
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("restart touched independent runtime or left inactive managed runtime: %v", calls)
			}
		})
	}
}

func TestSimulatorInstallLifecycleRetriesReadiness(t *testing.T) {
	selected := "northstar"
	calls := []string{}
	registry := lifecycleTestRegistry(&selected, &calls)
	attempts := 0
	registry.readSnapshot = func(context.Context, simulatorOperatorTarget) error {
		attempts++
		if attempts == 1 {
			return errors.New("starting")
		}
		return nil
	}
	if err := registry.activateAndCommit(context.Background(), "great-lakes", "activate", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("readiness attempts = %d", attempts)
	}
}

func TestSimulatorInstallLifecycleSerializesSwitches(t *testing.T) {
	selected := "northstar"
	calls := []string{}
	registry := lifecycleTestRegistry(&selected, &calls)
	started, release := make(chan struct{}), make(chan struct{})
	read := registry.readSnapshot
	registry.readSnapshot = func(ctx context.Context, target simulatorOperatorTarget) error {
		if target.ServiceURL == "https://great-lakes" {
			close(started)
			<-release
		}
		return read(ctx, target)
	}
	first, second := make(chan error, 1), make(chan error, 1)
	go func() {
		first <- registry.activateAndCommit(context.Background(), "great-lakes", "activate", func() error { selected = "great-lakes"; return nil })
	}()
	<-started
	go func() {
		second <- registry.activateAndCommit(context.Background(), "redwood", "activate", func() error { selected = "redwood"; return nil })
	}()
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	want := []string{"start great-lakes", "ready great-lakes", "stop northstar", "ready redwood", "stop great-lakes"}
	if selected != "redwood" || !reflect.DeepEqual(calls, want) {
		t.Fatalf("switches overlapped: %s %v", selected, calls)
	}
}

func TestSimulatorInstallLifecycleProcessHelper(t *testing.T) {
	if os.Getenv("ZPR_LIFECYCLE_TEST_HELPER") != "1" {
		return
	}
	args := os.Args[len(os.Args)-3:]
	path, literal, action := args[0], args[1], args[2]
	if literal != "literal;not-a-shell" || os.Getenv("SIMULATION_MANIFEST") != "" || os.Getenv("SIMULATOR_OPERATOR_SERVICE_URL") != "" || os.Getenv("ZPR_SIMULATOR_CONTROL_ROOM_URL") != "" {
		os.Exit(2)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	if err := os.WriteFile(path, []byte(action), 0600); err != nil {
		os.Exit(3)
	}
	if action == "fail" {
		_, _ = os.Stdout.WriteString("private-supervisor-output")
		_, _ = os.Stderr.WriteString("private-supervisor-output")
		os.Exit(5)
	}
	if action == "wait" {
		<-signals
		if err := os.WriteFile(path, []byte("terminated"), 0600); err != nil {
			os.Exit(4)
		}
	}
	os.Exit(0)
}

func TestSimulatorInstallLifecycleRealProcess(t *testing.T) {
	t.Setenv("ZPR_LIFECYCLE_TEST_HELPER", "1")
	t.Setenv("SIMULATION_MANIFEST", "must-not-reach-production")
	t.Setenv("SIMULATOR_OPERATOR_SERVICE_URL", "must-not-reach-production")
	t.Setenv("ZPR_SIMULATOR_CONTROL_ROOM_URL", "must-not-reach-production")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "actions")
	registry := &simulatorEvaluationInstalls{installs: map[string]simulatorEvaluationInstall{
		"northstar": {LifecycleCommand: []string{executable, "-test.run=^TestSimulatorInstallLifecycleProcessHelper$", "--", path, "literal;not-a-shell"}},
	}}
	if err := registry.lifecycle(context.Background(), "northstar", "start"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "start" {
		t.Fatalf("supervisor args: %q, %v", data, err)
	}
	if err := registry.lifecycle(context.Background(), "northstar", "fail"); err == nil || strings.Contains(err.Error(), "private-supervisor-output") {
		t.Fatalf("process failure ignored or supervisor output exposed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- registry.lifecycle(ctx, "northstar", "wait") }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) == "wait" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("supervisor process did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancellation was reported as success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not terminate")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "terminated" {
		t.Fatalf("SIGTERM was not handled: %q %v", data, err)
	}
}

func TestSimulatorInstallLifecycleRegistryOptIn(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	path := writeTestEvaluationInstalls(t, func(registry map[string]any) {
		testRegistryInstall(registry, 0)["lifecycle_command"] = []string{executable, "--operator-config", "/operator/northstar.json"}
	})
	registry, err := loadSimulatorEvaluationInstalls(path, "examples/organizations")
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.installs["northstar"].LifecycleCommand) != 3 || len(registry.installs["great-lakes"].LifecycleCommand) != 0 {
		t.Fatal("explicit lifecycle opt-in was not preserved")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSimulatorEvaluationInstalls(path, "examples/organizations"); err == nil {
		t.Fatal("public lifecycle registry was accepted")
	}
}

func TestSimulatorInstallLifecycleRejectsUnsafeOperations(t *testing.T) {
	selected := "northstar"
	calls := []string{}
	registry := lifecycleTestRegistry(&selected, &calls)
	for _, operation := range []string{"restore-base", "invalid"} {
		if err := registry.activateAndCommit(context.Background(), "great-lakes", operation, func() error { t.Fatal("invalid operation committed"); return nil }); err == nil {
			t.Fatalf("operation %q accepted", operation)
		}
	}
	if err := registry.activate(context.Background(), "great-lakes", "activate"); err == nil {
		t.Fatal("managed activation without persistence accepted")
	}
	registry.selectOrganization = func() (string, error) { return "", errors.New("selection unreadable") }
	if err := registry.activateAndCommit(context.Background(), "great-lakes", "activate", func() error { return nil }); err == nil {
		t.Fatal("activation without known selection accepted")
	}
	if len(calls) != 0 {
		t.Fatal("invalid operation changed runtime", calls)
	}
}

func TestSimulatorInstallLifecycleSelectionPersistsAcrossRestart(t *testing.T) {
	selection := filepath.Join(t.TempDir(), "active-organization")
	if err := os.WriteFile(selection, []byte("northstar\n"), 0600); err != nil {
		t.Fatal(err)
	}
	selectOrganization := func() (string, error) {
		data, err := os.ReadFile(selection)
		return strings.TrimSpace(string(data)), err
	}
	selected := "northstar"
	calls := []string{}
	registry := lifecycleTestRegistry(&selected, &calls)
	registry.selectOrganization = selectOrganization
	manager := &organizationActivationManager{run: organizationActivation{State: "idle"}, activate: registry.activateAndCommit}
	if !manager.start("great-lakes", selection) {
		t.Fatal("activation rejected")
	}
	select {
	case <-manager.done:
	case <-time.After(time.Second):
		t.Fatal("activation did not complete")
	}
	if state := manager.snapshot(); state.State != "completed" {
		t.Fatalf("activation failed: %+v", state)
	}
	if id, err := selectOrganization(); err != nil || id != "great-lakes" {
		t.Fatalf("selection was not saved: %q, %v", id, err)
	}
	calls = nil
	restarted := lifecycleTestRegistry(&selected, &calls)
	restarted.selectOrganization = selectOrganization
	if err := restarted.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	restarted.close()
	want := []string{"start great-lakes", "ready great-lakes", "stop northstar"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("restart did not use committed selection: %v", calls)
	}
}
