package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFileLogSourcesUseConfiguredContainerState(t *testing.T) {
	directory := t.TempDir()
	docker := "#!/bin/sh\ncase \"$4\" in live) echo running ;; stopped) echo exited ;; *) exit 1 ;; esac\n"
	if err := os.WriteFile(filepath.Join(directory, "docker"), []byte(docker), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	history := filepath.Join(directory, "history.log")
	if err := os.WriteFile(history, []byte("retained history\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(directory, "inventory.json")
	var targets []adapterLogTarget
	for _, container := range []string{"live", "stopped", "missing", ""} {
		targets = append(targets, adapterLogTarget{
			ID: "file-" + container, Name: "File " + container,
			Sources: []adapterLogSourceConfig{{Name: "History", Kind: "adapter", File: history, RuntimeContainer: container}},
		})
	}
	content, err := json.Marshal(adapterLogInventory{Adapters: targets})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, content, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_ADAPTER_LOG_CONFIG_FILE", config)
	response := httptest.NewRecorder()
	newAdapterLogsHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/adapter-logs", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d: %s", response.Code, response.Body)
	}
	var result struct {
		Adapters []adapterLogEntry `json:"adapters"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Adapters) != 4 {
		t.Fatalf("got %d adapters", len(result.Adapters))
	}
	for index, state := range []string{"running", "exited", "unavailable", "unknown"} {
		source := result.Adapters[index].Sources[0]
		if source.State != state || len(source.Lines) != 1 || source.Lines[0] != "retained history" {
			t.Errorf("file source %d: %+v, want state %s and retained history", index, source, state)
		}
	}
	targets[0].Sources[0].RuntimeContainer = "invalid/container"
	content, err = json.Marshal(adapterLogInventory{Adapters: targets})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readAdapterLogInventory(config); err == nil {
		t.Fatal("unsafe container identity accepted for a file source")
	}
}
