package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOrganizationActivationCommitsOnlyAfterReset(t *testing.T) {
	selection := filepath.Join(t.TempDir(), "active.txt")
	if err := os.WriteFile(selection, []byte("northstar\n"), 0600); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	manager := &organizationActivationManager{run: organizationActivation{State: "idle"}, reset: func(context.Context, string) error {
		close(started)
		<-release
		return errors.New("reset failed")
	}}
	if !manager.start("velocity", selection) {
		t.Fatal("activation was not started")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("reset did not start")
	}
	if manager.start("redwood", selection) {
		t.Fatal("overlapping activation was accepted")
	}
	before, err := os.ReadFile(selection)
	if err != nil || string(before) != "northstar\n" {
		t.Fatalf("selection changed before reset: %q %v", before, err)
	}
	close(release)
	select {
	case <-manager.done:
	case <-time.After(time.Second):
		t.Fatal("activation did not finish")
	}
	after, err := os.ReadFile(selection)
	if err != nil || string(after) != "northstar\n" || manager.snapshot().State != "failed" {
		t.Fatalf("failure changed active selection: %q %v", after, manager.snapshot())
	}
}
