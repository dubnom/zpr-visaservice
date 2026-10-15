// Package logview formats bounded, redacted log text without knowing its source.
package logview

import (
	"regexp"
	"strings"
)

var secrets = regexp.MustCompile(`(?i)(\bBearer\s+[A-Za-z0-9._~+/=-]+|\bzpr_vsapi\.[0-9a-f]{8}\.[A-Za-z0-9_-]+|(?:authorization|x-api-key|password|secret|token)["']?\s*[:=]\s*["']?(?:bearer\s+)?[^\s,;"}]+)`)

// Lines retains the last 64 KiB and 100 lines, redacting recognized credentials.
// Byte truncation discards the first partial line when a newline is available.
func Lines(output string) []string {
	if len(output) > 65536 {
		output = output[len(output)-65536:]
		if boundary := strings.IndexByte(output, '\n'); boundary >= 0 {
			output = output[boundary+1:]
		}
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}
	}
	if len(lines) > 100 {
		lines = lines[len(lines)-100:]
	}
	for index, line := range lines {
		lines[index] = Redact(line)
	}
	return lines
}

// Redact replaces recognized credentials without trimming or truncating text.
func Redact(text string) string {
	return secrets.ReplaceAllString(text, "[REDACTED]")
}
