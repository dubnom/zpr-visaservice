package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func renderUnit(output io.Writer, command []string) error {
	if len(command) == 0 || len(command) > 128 {
		return errors.New("usage: zpr-evaluation-supervisor render-unit /absolute/operator-launcher [arguments...]")
	}
	path := command[0]
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("operator launcher requires a canonical absolute path")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect operator launcher: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 || info.Mode().Perm()&0022 != 0 {
		return errors.New("operator launcher must be a regular executable not writable by group or others")
	}
	quoted := make([]string, len(command))
	total := 0
	for index, argument := range command {
		total += len(argument)
		if len(argument) > 4096 || total > 32768 || strings.IndexFunc(argument, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			return errors.New("operator launch arguments must be bounded and contain no control characters")
		}
		// systemd expands specifiers and environment variables even in quoted
		// ExecStart arguments; escape both, without involving a shell.
		argument = strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`, `$`, `$$`).Replace(argument)
		quoted[index] = `"` + argument + `"`
	}
	_, err = fmt.Fprintf(output, `[Unit]
Description=Independent ZPR evaluation install

[Service]
Type=exec
ExecStart=%s
Restart=on-failure
RestartSec=5s
UMask=0077
KillMode=mixed
KillSignal=SIGTERM
TimeoutStopSec=90s

[Install]
WantedBy=default.target
`, strings.Join(quoted, " "))
	if err != nil {
		return fmt.Errorf("write operator unit: %w", err)
	}
	return nil
}
