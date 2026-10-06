package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAdapterLogRuntimeStateIsIndependentOfReadableHistory(t *testing.T) {
	directory := t.TempDir()
	docker := `#!/bin/sh
case "$1" in
  inspect)
    case "$4" in
      live) printf 'running\n' ;;
      stopped) printf 'exited\n' ;;
      paused) printf 'paused\n' ;;
      *) exit 1 ;;
    esac ;;
  logs) printf 'retained log entry\n' ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(directory, "docker"), []byte(docker), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	file := filepath.Join(directory, "history.log")
	if err := os.WriteFile(file, []byte("old entry\n"), 0600); err != nil {
		t.Fatal(err)
	}
	inventory := adapterLogInventory{Adapters: []adapterLogTarget{
		{ID: "mixed", Name: "Mixed", Sources: []adapterLogSourceConfig{
			{Name: "Live", Kind: "controller", Container: "live"},
			{Name: "Stopped", Kind: "adapter", Container: "stopped"},
		}},
		{ID: "paused", Name: "Paused", Sources: []adapterLogSourceConfig{{Name: "Paused", Kind: "adapter", Container: "paused"}}},
		{ID: "missing", Name: "Missing", Sources: []adapterLogSourceConfig{{Name: "Missing", Kind: "adapter", Container: "missing"}}},
		{ID: "file", Name: "File", Sources: []adapterLogSourceConfig{{Name: "File", Kind: "adapter", File: file}}},
	}}
	content, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(directory, "inventory.json")
	if err := os.WriteFile(config, content, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_ADAPTER_LOG_CONFIG_FILE", config)
	response := httptest.NewRecorder()
	newAdapterLogsHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/adapter-logs", nil))
	var result struct {
		Adapters []adapterLogEntry `json:"adapters"`
	}
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d: %s", response.Code, response.Body)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Adapters) != 4 {
		t.Fatalf("got %d adapters, want 4", len(result.Adapters))
	}
	for index, want := range []string{"running", "paused", "unavailable", "unknown"} {
		if result.Adapters[index].State != want {
			t.Errorf("adapter %d state=%s, want %s", index, result.Adapters[index].State, want)
		}
	}
	stopped := result.Adapters[0].Sources[1]
	if stopped.State != "exited" || len(stopped.Lines) == 0 {
		t.Fatalf("stopped container must retain history without reporting running: %+v", stopped)
	}
	if result.Adapters[3].Sources[0].State != "unknown" {
		t.Fatal("a readable file does not prove a running adapter")
	}
}
