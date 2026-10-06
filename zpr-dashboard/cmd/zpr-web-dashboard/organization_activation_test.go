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
	manager := &organizationActivationManager{run: organizationActivation{State: "idle"}, reset: func(context.Context, string, string) error {
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

func TestOrganizationActivationPublishesResetProgress(t *testing.T) {
	selection := filepath.Join(t.TempDir(), "active.txt")
	phase := "Waiting for ZPR node and company LDAP"
	started, release := make(chan struct{}), make(chan struct{})
	manager := &organizationActivationManager{run: organizationActivation{State: "idle"}, reset: func(context.Context, string, string) error {
		if err := os.WriteFile(selection+".progress", []byte(phase+"\n"), 0600); err != nil {
			return err
		}
		close(started)
		<-release
		return nil
	}}
	if !manager.start("velocity", selection) {
		t.Fatal("activation was not started")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("reset did not start")
	}
	deadline := time.After(time.Second)
	for manager.snapshot().Progress != phase {
		select {
		case <-deadline:
			t.Fatalf("activation progress = %q; want %q", manager.snapshot().Progress, phase)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if manager.snapshot().State != "resetting" {
		t.Fatalf("activation state = %q while reset is blocked", manager.snapshot().State)
	}
	close(release)
	select {
	case <-manager.done:
	case <-time.After(time.Second):
		t.Fatal("activation did not finish")
	}
	status := manager.snapshot()
	if status.State != "completed" || status.Progress != "Organization ready" {
		t.Fatalf("activation completion status = %+v", status)
	}
}
