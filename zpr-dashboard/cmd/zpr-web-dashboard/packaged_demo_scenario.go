//go:build !production

package main

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func runPackagedDemoProbe(ctx context.Context, script string) (string, error) {
	command := exec.CommandContext(ctx, "sh", script, "verify-packaged-demo")
	// Let the shell trap remove its temporary listener on cancellation.
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = 20 * time.Second
	output, err := command.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}
