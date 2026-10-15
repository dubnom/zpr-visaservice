package logview

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestLogLinesBoundsAndWhitespace(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", []string{}},
		{"whitespace", " \t\r\n ", []string{}},
		{"trim outer whitespace", " \tfirst \n second \t\n", []string{"first ", " second"}},
		{"preserve inner blank", "one\n\nthree", []string{"one", "", "three"}},
		{"preserve inner CR", "one\r\ntwo\r\n", []string{"one\r", "two"}},
		{"exact byte limit", strings.Repeat("a", 65536), []string{strings.Repeat("a", 65536)}},
		{"one byte over without newline", "x" + strings.Repeat("a", 65536), []string{strings.Repeat("a", 65536)}},
		{"partial line removed", strings.Repeat("x", 65533) + "\nkept", []string{"kept"}},
		{"newline at truncation point", "x\n" + strings.Repeat("a", 65535), []string{strings.Repeat("a", 65535)}},
		{"newline at end of truncated text", strings.Repeat("a", 65536) + "\n", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Lines(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %d lines, lengths=%v, want lengths=%v", len(got), lineLengths(got), lineLengths(tc.want))
			}
			if got == nil {
				t.Fatal("empty results must serialize as [] rather than null")
			}
		})
	}
	for _, count := range []int{99, 100, 101, 10000} {
		var input []string
		for index := 0; index < count; index++ {
			input = append(input, fmt.Sprintf("line %05d", index))
		}
		got := Lines(strings.Join(input, "\n"))
		start := max(0, count-100)
		if !reflect.DeepEqual(got, input[start:]) {
			t.Fatalf("%d input lines: got %d, want last %d", count, len(got), count-start)
		}
	}
	encoded, err := json.Marshal(Lines(""))
	if err != nil || string(encoded) != "[]" {
		t.Fatalf("empty JSON=%s error=%v", encoded, err)
	}
}

func lineLengths(lines []string) []int {
	lengths := make([]int, len(lines))
	for index, line := range lines {
		lengths[index] = len(line)
	}
	return lengths
}

func TestLogLinesRedactionCompatibility(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"Bearer abc.DEF_~+/=-", "[REDACTED]"},
		{"prefix bearer abc suffix", "prefix [REDACTED] suffix"},
		{"zpr_vsapi.0123abcd.private-key", "[REDACTED]"},
		{"ZPR_VSAPI.ABCD0123.private_key", "[REDACTED]"},
		{"authorization: Bearer private-token", "[REDACTED]"},
		{"x-api-key=private-key", "[REDACTED]"},
		{`{"token":"private-token"}`, `{"[REDACTED]"}`},
		{"password='private-password'; status=ok", "[REDACTED]; status=ok"},
		{"SECRET = private-value, next=ok", "[REDACTED], next=ok"},
		{"token=one; password=two", "[REDACTED]; [REDACTED]"},
		{"ordinary text without credentials", "ordinary text without credentials"},
		{"tokenization is complete", "tokenization is complete"},
		{"zpr_vsapi.short.not-a-key", "zpr_vsapi.short.not-a-key"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if got := Lines(tc.input); !reflect.DeepEqual(got, []string{tc.want}) {
				t.Fatalf("redaction=%q, want %q", got, tc.want)
			}
			if got := Redact(tc.input); got != tc.want {
				t.Fatalf("unbounded redaction=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestLogRedactionPreservesUnboundedTextLayout(t *testing.T) {
	input := "\n  " + strings.Repeat("entry\n", 20000) + "password=private\n\n"
	want := "\n  " + strings.Repeat("entry\n", 20000) + "[REDACTED]\n\n"
	if got := Redact(input); got != want {
		t.Fatal("redaction changed whitespace, line count or text bounds")
	}
}

func TestLogLinesIndependentOfConfiguration(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("ZPR_SIMULATOR_URL", "://invalid")
	t.Setenv("ZPR_ADAPTER_LOG_CONFIG_FILE", "/unavailable/inventory.json")
	if got := Lines("connected\npassword=private"); !reflect.DeepEqual(got, []string{"connected", "[REDACTED]"}) {
		t.Fatalf("formatting depends on configuration: %q", got)
	}
}

func TestLogLinesDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect log formatting dependencies: %v\n%s", err, output)
	}
	packages := strings.Fields(string(output))
	if len(packages) != 1 || packages[0] != "neboagency.com/zpr-dashborad/internal/logview" {
		t.Fatalf("log formatting must use only the standard library, got %v", packages)
	}
}
