package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type adapterLogInventory struct {
	Adapters []adapterLogTarget `json:"adapters"`
}

type adapterLogTarget struct {
	ID      string                   `json:"id"`
	Name    string                   `json:"name"`
	Sources []adapterLogSourceConfig `json:"sources"`
}

type adapterLogSourceConfig struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	File      string `json:"file,omitempty"`
	Container string `json:"container,omitempty"`
	Path      string `json:"path,omitempty"`
}

type adapterLogOutput struct {
	Name  string   `json:"name"`
	Kind  string   `json:"kind"`
	Lines []string `json:"lines"`
	Error string   `json:"error,omitempty"`
}

type adapterLogEntry struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	State   string             `json:"state"`
	Sources []adapterLogOutput `json:"sources"`
}

var adapterLogContainerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

func readAdapterLogInventory(path string) (adapterLogInventory, error) {
	var inventory adapterLogInventory
	file, err := os.Open(path)
	if err != nil {
		return inventory, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inventory); err != nil {
		return inventory, err
	}
	if len(inventory.Adapters) > 100 {
		return inventory, errors.New("too many adapter log targets")
	}
	seen := make(map[string]bool)
	for _, adapter := range inventory.Adapters {
		if adapter.ID == "" || adapter.Name == "" || seen[adapter.ID] || len(adapter.Sources) > 8 {
			return inventory, errors.New("invalid adapter log target")
		}
		seen[adapter.ID] = true
		for _, source := range adapter.Sources {
			if source.Name == "" || source.Kind != "adapter" && source.Kind != "controller" {
				return inventory, errors.New("invalid adapter log source")
			}
			if source.File != "" {
				if !filepath.IsAbs(source.File) || source.Container != "" || source.Path != "" {
					return inventory, errors.New("invalid file log source")
				}
			} else if !adapterLogContainerName.MatchString(source.Container) || source.Path != "" && !filepath.IsAbs(source.Path) {
				return inventory, errors.New("invalid container log source")
			}
		}
	}
	return inventory, nil
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
		for _, adapter := range inventory.Adapters {
			entry := adapterLogEntry{ID: adapter.ID, Name: adapter.Name, State: "running", Sources: []adapterLogOutput{}}
			available := false
			for _, source := range adapter.Sources {
				output, err := readAdapterLogSource(ctx, source)
				log := adapterLogOutput{Name: source.Name, Kind: source.Kind, Lines: []string{}}
				if err != nil {
					log.Error = "Log source unavailable"
				} else {
					log.Lines = machineLogLines(output)
					available = true
				}
				entry.Sources = append(entry.Sources, log)
			}
			if !available {
				entry.State = "unavailable"
			}
			entries = append(entries, entry)
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"updated_at": time.Now().UTC(), "adapters": entries})
	})
}
