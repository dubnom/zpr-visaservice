package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"neboagency.com/zpr-dashborad/internal/enrollment"
)

func configuredEnrollmentHandler() (http.Handler, func() error, error) {
	configPath := strings.TrimSpace(os.Getenv("ZPR_ENROLLMENT_CONFIG_FILE"))
	databasePath := strings.TrimSpace(os.Getenv("ZPR_ENROLLMENT_DATABASE_FILE"))
	if configPath == "" && databasePath == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			writePolicyError(w, http.StatusServiceUnavailable, "Device enrollment administration is not configured.")
		}), func() error { return nil }, nil
	}
	if configPath == "" || databasePath == "" {
		return nil, nil, errors.New("enrollment requires both ZPR_ENROLLMENT_CONFIG_FILE and ZPR_ENROLLMENT_DATABASE_FILE")
	}
	file, err := os.Open(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open enrollment configuration: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 65537))
	closeErr := file.Close()
	if readErr != nil {
		return nil, nil, readErr
	}
	if closeErr != nil {
		return nil, nil, closeErr
	}
	if len(data) > 65536 {
		return nil, nil, errors.New("enrollment configuration exceeds 65536 bytes")
	}
	config, err := enrollment.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	store, err := enrollment.Open(databasePath)
	if err != nil {
		return nil, nil, err
	}
	handler, err := enrollment.NewAdminHandler(store, config)
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	return handler, store.Close, nil
}
