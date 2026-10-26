//go:build !linux && !darwin && !production

package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func configureSimulatorReset(_ operatorRequestAuthorizer, _ *simulatorScenarioManager) (http.Handler, func() error, error) {
	organization := os.Getenv("SIMULATOR_RESET_ORGANIZATION")
	databasePath := os.Getenv("SIMULATOR_RESET_DB_PATH")
	if organization == "" && databasePath == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Simulator reset is unavailable on this operating system.", http.StatusServiceUnavailable)
		}), func() error { return nil }, nil
	}
	if organization == "" || databasePath == "" {
		return nil, nil, errors.New("Simulator reset configuration requires both SIMULATOR_RESET_ORGANIZATION and SIMULATOR_RESET_DB_PATH")
	}
	if organization != strings.TrimSpace(organization) || !validSimulatorResetOrganizationUnsupported(organization) {
		return nil, nil, errors.New("SIMULATOR_RESET_ORGANIZATION must be an explicit organization identifier")
	}
	if !filepath.IsAbs(databasePath) || filepath.Clean(databasePath) != databasePath || filepath.Base(databasePath) == "." {
		return nil, nil, errors.New("SIMULATOR_RESET_DB_PATH must be an absolute, clean path")
	}
	return nil, nil, fmt.Errorf("Simulator reset is unavailable on %s", runtime.GOOS)
}

func validSimulatorResetOrganizationUnsupported(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}
