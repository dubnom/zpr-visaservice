// zpr-evaluation-supervisor controls an independently configured user service.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var unitName = regexp.MustCompile(`^zpr-evaluation-[a-z0-9][a-z0-9-]{0,63}\.service$`)

type systemctlRunner func(context.Context, ...string) (string, error)

func control(ctx context.Context, args []string, run systemctlRunner) error {
	if len(args) != 2 || !unitName.MatchString(args[0]) || strings.HasSuffix(args[0], "-.service") || (args[1] != "start" && args[1] != "stop") {
		return errors.New("usage: zpr-evaluation-supervisor zpr-evaluation-NAME.service start|stop")
	}
	unit, action := args[0], args[1]
	if _, err := run(ctx, "--user", action, unit); err != nil {
		if action == "stop" && ctx.Err() == nil {
			// systemd unloads stopped transient units; stop then exits 5 even
			// though the requested inactive state has already been reached.
			state, readErr := run(ctx, "--user", "show", "--property=LoadState", "--property=ActiveState", unit)
			if readErr == nil && unloadedInactive(state) {
				return nil
			}
			err = errors.Join(err, readErr)
		}
		return fmt.Errorf("%s %s failed; inspect the user service journal: %w", action, unit, err)
	}
	state, err := run(ctx, "--user", "show", "--property=ActiveState", "--value", unit)
	if err != nil {
		return fmt.Errorf("read %s state failed: %w", unit, err)
	}
	state = strings.TrimSpace(state)
	if action == "start" && state == "active" || action == "stop" && state == "inactive" {
		return nil
	}
	return fmt.Errorf("%s %s returned unexpected service state %q; inspect the user service journal", action, unit, state)
}

func unloadedInactive(value string) bool {
	properties := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || (key != "LoadState" && key != "ActiveState") {
			return false
		}
		if _, exists := properties[key]; exists {
			return false
		}
		properties[key] = value
	}
	return properties["LoadState"] == "not-found" && properties["ActiveState"] == "inactive"
}

func systemctl(ctx context.Context, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "/usr/bin/systemctl", args...)
	command.Env = []string{}
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		if !strings.HasPrefix(name, "SIMULATION_") && !strings.HasPrefix(name, "SIMULATOR_") && !strings.HasPrefix(name, "ZPR_SIMULATOR_") {
			command.Env = append(command.Env, variable)
		}
	}
	command.Stderr = io.Discard
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = 5 * time.Second
	output, err := command.Output()
	return string(output), err
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "render-unit" {
		if err := renderUnit(os.Stdout, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 110*time.Second)
	defer timeout()
	if err := control(ctx, os.Args[1:], systemctl); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
