//go:build !production

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type simulatorInstallWorkers struct {
	cancel context.CancelFunc
	done   <-chan struct{}
}

func validateSimulatorInstallLifecycle(registryPath string, command []string) error {
	if len(command) > 16 || len(command) == 0 || !filepath.IsAbs(command[0]) || filepath.Clean(command[0]) != command[0] {
		return errors.New("lifecycle command requires a canonical absolute executable and at most 16 elements")
	}
	for _, arg := range command {
		if arg == "" || len(arg) > 4096 || strings.ContainsAny(arg, "\x00\r\n") {
			return errors.New("lifecycle command contains an invalid argument")
		}
	}
	for _, path := range []string{registryPath, command[0]} {
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect lifecycle file: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
			return errors.New("lifecycle registry and executable must be regular files not writable by group or others")
		}
		if path == registryPath && info.Mode().Perm()&0077 != 0 {
			return errors.New("lifecycle registry must be private")
		}
		if path == command[0] && info.Mode().Perm()&0111 == 0 {
			return errors.New("lifecycle command is not executable")
		}
	}
	return nil
}

func (registry *simulatorEvaluationInstalls) lifecycle(ctx context.Context, id, action string) error {
	command := registry.installs[id].LifecycleCommand
	if len(command) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var err error
	if registry.runLifecycle != nil {
		err = registry.runLifecycle(ctx, command, action)
	} else {
		args := append(append([]string{}, command[1:]...), action)
		process := exec.CommandContext(ctx, command[0], args...)
		process.Env = []string{}
		for _, variable := range os.Environ() {
			name, _, _ := strings.Cut(variable, "=")
			if !strings.HasPrefix(name, "SIMULATION_") && !strings.HasPrefix(name, "SIMULATOR_") && !strings.HasPrefix(name, "ZPR_SIMULATOR_") {
				process.Env = append(process.Env, variable)
			}
		}
		process.Cancel = func() error { return process.Process.Signal(syscall.SIGTERM) }
		process.WaitDelay = 10 * time.Second
		err = process.Run()
	}
	if err != nil {
		log.Printf("organization %q lifecycle %s failed: %v", id, action, err)
		return fmt.Errorf("organization %q lifecycle %s failed; check the operator supervisor log: %w", id, action, err)
	}
	return nil
}

func (registry *simulatorEvaluationInstalls) ready(ctx context.Context, id string, target simulatorOperatorTarget) error {
	if len(registry.installs[id].LifecycleCommand) == 0 {
		if err := registry.checkSnapshot(ctx, target); err != nil {
			return fmt.Errorf("organization %q evaluation install is not reachable; start it first: %w", id, err)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for {
		err := registry.checkSnapshot(ctx, target)
		if err == nil {
			return ctx.Err()
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("organization %q is not ready: %w", id, errors.Join(err, ctx.Err()))
		case <-timer.C:
		}
	}
}

// switchInstall runs under lifecycleMu. Selection is committed only after the
// previous managed runtime stops; failure restarts it without changing selection.
func (registry *simulatorEvaluationInstalls) switchInstall(ctx context.Context, id string, target simulatorOperatorTarget, commit func() error) error {
	previous := ""
	for _, install := range registry.installs {
		if len(install.LifecycleCommand) != 0 {
			var err error
			previous, err = registry.selectOrganization()
			if err != nil {
				return fmt.Errorf("read active organization: %w", err)
			}
			break
		}
	}
	cleanupNext := func(cause error) error {
		if id == previous || len(registry.installs[id].LifecycleCommand) == 0 {
			return cause
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return errors.Join(cause, registry.lifecycle(cleanup, id, "stop"))
	}
	if err := registry.lifecycle(ctx, id, "start"); err != nil {
		return cleanupNext(err)
	}
	if err := registry.ready(ctx, id, target); err != nil {
		return cleanupNext(err)
	}
	previousManaged := id != previous && len(registry.installs[previous].LifecycleCommand) != 0
	rollback := func(cause error) error {
		if previousManaged {
			recovery, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			restartErr := registry.lifecycle(recovery, previous, "start")
			if restartErr == nil {
				previousTarget, targetErr := registry.target(previous)
				restartErr = targetErr
				if restartErr == nil {
					restartErr = registry.ready(recovery, previous, previousTarget)
				}
			}
			if restartErr == nil {
				registry.startWorkers(previous)
			} else {
				cause = errors.Join(cause, fmt.Errorf("previous organization recovery failed: %w", restartErr))
			}
		}
		return cleanupNext(cause)
	}
	if previousManaged {
		registry.stopWorkers(previous)
		if err := registry.lifecycle(ctx, previous, "stop"); err != nil {
			return rollback(err)
		}
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	if err := commit(); err != nil {
		return rollback(fmt.Errorf("save active organization: %w", err))
	}
	registry.startWorkers(id)
	return nil
}

// run adopts the selected managed install after a Simulator restart. Independently
// managed installs retain the existing always-exercised behavior.
func (registry *simulatorEvaluationInstalls) run(ctx context.Context) error {
	registry.lifecycleMu.Lock()
	defer registry.lifecycleMu.Unlock()
	registry.workersContext = ctx
	registry.workers = map[string]simulatorInstallWorkers{}
	hasManaged := false
	for _, install := range registry.installs {
		hasManaged = hasManaged || len(install.LifecycleCommand) != 0
	}
	selected := ""
	if hasManaged {
		var err error
		selected, err = registry.selectOrganization()
		if err != nil {
			return fmt.Errorf("read active organization: %w", err)
		}
		target, err := registry.target(selected)
		if err != nil {
			return err
		}
		if len(registry.installs[selected].LifecycleCommand) != 0 {
			if err := registry.lifecycle(ctx, selected, "start"); err != nil {
				return err
			}
			if err := registry.ready(ctx, selected, target); err != nil {
				return err
			}
		}
		// Selection is the recovery record if a process exited mid-switch.
		// Stop only explicitly managed inactive installs, never independent ones.
		ids := make([]string, 0, len(registry.installs))
		for id := range registry.installs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if id != selected {
				if err := registry.lifecycle(ctx, id, "stop"); err != nil {
					return err
				}
			}
		}
	}
	for id, install := range registry.installs {
		if len(install.LifecycleCommand) == 0 || id == selected {
			registry.startWorkers(id)
		}
	}
	return nil
}

func (registry *simulatorEvaluationInstalls) startWorkers(id string) {
	if registry.workersContext == nil || registry.workersContext.Err() != nil {
		return
	}
	if _, exists := registry.workers[id]; exists || registry.endpoints[id] == nil {
		return
	}
	ctx, cancel := context.WithCancel(registry.workersContext)
	done := make(chan struct{})
	registry.workers[id] = simulatorInstallWorkers{cancel: cancel, done: done}
	go func() {
		endpointsDone := make(chan struct{})
		go func() {
			defer close(endpointsDone)
			registry.endpoints[id].run(ctx)
		}()
		if workloads := registry.workloads[id]; workloads != nil {
			workloads.run(ctx)
		} else {
			<-ctx.Done()
		}
		<-endpointsDone
		close(done)
	}()
}

func (registry *simulatorEvaluationInstalls) stopWorkers(id string) {
	if workers, ok := registry.workers[id]; ok {
		workers.cancel()
		<-workers.done
		delete(registry.workers, id)
		if plane := registry.endpoints[id]; plane != nil {
			plane.mu.Lock()
			plane.status = map[string]simulatorDataPlaneEndpointStatus{}
			plane.planeUp = false
			plane.mu.Unlock()
		}
		if workloads := registry.workloads[id]; workloads != nil {
			workloads.mu.Lock()
			workloads.results = map[string]simulatorDataPlaneProbeResult{}
			workloads.mu.Unlock()
		}
	}
}

// Simulator shutdown stops its workers, not the independent production runtime.
func (registry *simulatorEvaluationInstalls) close() {
	registry.lifecycleMu.Lock()
	defer registry.lifecycleMu.Unlock()
	for id := range registry.workers {
		registry.stopWorkers(id)
	}
	registry.workersContext = nil
}
