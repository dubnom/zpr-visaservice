package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/controlservice/adapterlogs"
	"neboagency.com/zpr-dashborad/internal/logview"
)

type adapterLogInventory = adapterlogs.Inventory

type adapterLogTarget = adapterlogs.Target

type adapterLogSourceConfig = adapterlogs.SourceConfig

type adapterLogOutput struct {
	Name  string   `json:"name"`
	Kind  string   `json:"kind"`
	State string   `json:"state"`
	Lines []string `json:"lines"`
	Error string   `json:"error,omitempty"`
}

func adapterLogRuntimeState(ctx context.Context, source adapterLogSourceConfig) string {
	container := source.RuntimeContainer
	if container == "" {
		container = source.Container
	}
	if container == "" {
		return "unknown"
	}
	output, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Status}}", container).Output()
	if err != nil {
		return "unavailable"
	}
	switch state := strings.TrimSpace(string(output)); state {
	case "running", "paused", "restarting", "created", "exited", "dead", "removing":
		return state
	default:
		return "unknown"
	}
}

type adapterLogEntry struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	State   string             `json:"state"`
	Sources []adapterLogOutput `json:"sources"`
}

func readAdapterLogInventory(path string) (adapterLogInventory, error) {
	return adapterlogs.ReadInventory(path)
}

func readAdapterLogSource(ctx context.Context, source adapterLogSourceConfig) (string, error) {
	if source.File != "" {
		file, err := os.Open(source.File)
		if err != nil {
			return "", err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return "", errors.New("log source is not a regular file")
		}
		if info.Size() > 65536 {
			if _, err := file.Seek(-65536, io.SeekEnd); err != nil {
				return "", err
			}
		}
		data, err := io.ReadAll(io.LimitReader(file, 65536))
		return string(data), err
	}
	args := []string{"logs", "--tail", "100", "--timestamps", source.Container}
	if source.Path != "" {
		args = []string{"exec", source.Container, "tail", "-c", "65536", "--", source.Path}
	}
	output := &limitedBuffer{limit: 65536}
	command := exec.CommandContext(ctx, "docker", args...)
	command.Stdout, command.Stderr = output, output
	err := command.Run()
	return output.String(), err
}

func newAdapterLogsHandler() http.Handler {
	configPath := strings.TrimSpace(os.Getenv("ZPR_ADAPTER_LOG_CONFIG_FILE"))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writePolicyError(w, http.StatusMethodNotAllowed, "Adapter logs are read-only.")
			return
		}
		inventory, err := readAdapterLogInventory(configPath)
		if err != nil {
			writePolicyError(w, http.StatusServiceUnavailable, "Adapter log inventory is unavailable; configure ZPR_ADAPTER_LOG_CONFIG_FILE on Control-Service.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		entries := make([]adapterLogEntry, 0, len(inventory.Adapters))
		runtimeStates := make(map[string]string)
		for _, adapter := range inventory.Adapters {
			entry := adapterLogEntry{ID: adapter.ID, Name: adapter.Name, State: "unknown", Sources: []adapterLogOutput{}}
			for _, source := range adapter.Sources {
				container := source.RuntimeContainer
				if container == "" {
					container = source.Container
				}
				state, known := runtimeStates[container]
				if !known {
					state = adapterLogRuntimeState(ctx, source)
					runtimeStates[container] = state
				}
				output, err := readAdapterLogSource(ctx, source)
				log := adapterLogOutput{Name: source.Name, Kind: source.Kind, State: state, Lines: []string{}}
				if err != nil {
					log.Error = "Log source unavailable"
				} else {
					log.Lines = logview.Lines(output)
				}
				if state == "running" || entry.State == "unknown" {
					entry.State = state
				}
				entry.Sources = append(entry.Sources, log)
			}
			entries = append(entries, entry)
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"updated_at": time.Now().UTC(), "adapters": entries})
	})
}
