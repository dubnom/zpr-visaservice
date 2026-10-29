package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluationPrebuiltOperator(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "zpr-control-room")
	if err := os.WriteFile(binary, []byte("prebuilt executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(binary, link); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(root, "plain")
	if err := os.WriteFile(plain, []byte("not executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(root, "empty")
	if err := os.WriteFile(empty, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"relative", root, link, plain, empty, filepath.Join(root, "missing")} {
		t.Run(filepath.Base(invalid), func(t *testing.T) {
			if _, err := (evaluationRuntimeOptions{ControlRoomBinary: invalid}).resolve(testEvaluationConfig()); err == nil {
				t.Fatal("invalid prebuilt executable accepted", invalid)
			}
		})
	}
	options, err := (evaluationRuntimeOptions{ControlRoomBinary: binary}).resolve(testEvaluationConfig())
	if err != nil {
		t.Fatal(err)
	}
	// No Go executable, source checkout or output directory exists in this path.
	t.Setenv("PATH", root)
	var out bytes.Buffer
	got, err := prepareEvaluationOperator(context.Background(), filepath.Join(root, "absent"), options, &out)
	if err != nil || got != binary || !strings.Contains(out.String(), "no source build") {
		t.Fatalf("prebuilt startup: binary=%q err=%v output=%q", got, err, out.String())
	}
}
