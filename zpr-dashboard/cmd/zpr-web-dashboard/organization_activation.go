package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type organizationActivation struct {
	OrganizationID string `json:"organization_id,omitempty"`
	State          string `json:"state"`
	Progress       string `json:"progress,omitempty"`
	Error          string `json:"error,omitempty"`
}

type organizationActivationManager struct {
	mu    sync.RWMutex
	run   organizationActivation
	done  chan struct{}
	reset func(context.Context, string) error
}

var activeOrganizationActivation = &organizationActivationManager{
	run: organizationActivation{State: "idle"},
	reset: func(ctx context.Context, organizationID string) error {
		script := strings.TrimSpace(os.Getenv("SIMULATION_ORGANIZATION_RESET_SCRIPT"))
		if script == "" {
			return errors.New("organization reset is not configured")
		}
		command := exec.CommandContext(ctx, "sh", script, "reset-organization", organizationID)
		command.Env = append(os.Environ(), "SIMULATION_ACTIVATION_STATUS_FILE="+simulatorActiveOrganizationPath()+".progress")
		command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
		command.WaitDelay = 2 * time.Minute
		if err := command.Run(); err != nil {
			return errors.New("organization reset failed; check the local reset log")
		}
		return nil
	},
}

func (manager *organizationActivationManager) snapshot() organizationActivation {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.run
}

func (manager *organizationActivationManager) start(organizationID, selectionFile string) bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.run.State == "resetting" {
		return false
	}
	manager.run = organizationActivation{OrganizationID: organizationID, State: "resetting", Progress: "Preparing organization switch"}
	manager.done = make(chan struct{})
	go manager.execute(organizationID, selectionFile, manager.done)
	return true
}

func (manager *organizationActivationManager) execute(organizationID, selectionFile string, done chan struct{}) {
	defer close(done)
	progressFile := selectionFile + ".progress"
	_ = os.Remove(progressFile)
	progressDone := make(chan struct{})
	go manager.watchProgress(progressFile, progressDone)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	err := manager.reset(ctx, organizationID)
	close(progressDone)
	if err == nil {
		manager.setProgress("Saving active organization")
		err = saveActiveOrganization(selectionFile, organizationID)
	}
	_ = os.Remove(progressFile)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.run.State = "completed"
	if err != nil {
		manager.run.State = "failed"
		manager.run.Progress = "Organization switch failed"
		manager.run.Error = err.Error()
	} else {
		manager.run.Progress = "Organization ready"
	}
}

func (manager *organizationActivationManager) setProgress(progress string) {
	progress = strings.TrimSpace(progress)
	if progress == "" {
		return
	}
	manager.mu.Lock()
	if manager.run.State == "resetting" {
		manager.run.Progress = progress
	}
	manager.mu.Unlock()
}

func (manager *organizationActivationManager) watchProgress(progressFile string, done <-chan struct{}) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if progress, err := os.ReadFile(progressFile); err == nil {
			manager.setProgress(string(progress))
		}
		select {
		case <-done:
			return
		case <-ticker.C:
		}
	}
}

func saveActiveOrganization(selectionFile, organizationID string) error {
	file, err := os.CreateTemp(filepath.Dir(selectionFile), ".active-organization-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.WriteString(organizationID + "\n")
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), selectionFile)
}
