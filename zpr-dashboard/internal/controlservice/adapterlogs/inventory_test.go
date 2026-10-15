package adapterlogs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func inventoryFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inventory.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func encodedInventory(t *testing.T, inventory Inventory) string {
	t.Helper()
	content, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestAdapterLogInventoryExplicitConfigurationWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/profiles")
	t.Setenv("ZPR_SIMULATOR_URL", "://invalid")
	t.Setenv("ZPR_ADAPTER_LOG_CONFIG_FILE", "/unavailable/not-selected.json")
	expected := Inventory{Adapters: []Target{
		{ID: "file", Name: "File history", Sources: []SourceConfig{
			{Name: "Adapter", Kind: "adapter", File: "/operator/logs/adapter.log", RuntimeContainer: "adapter-1"},
		}},
		{ID: "container", Name: "Container history", Sources: []SourceConfig{
			{Name: "Adapter", Kind: "adapter", Container: "adapter-1"},
			{Name: "Controller", Kind: "controller", Container: "controller.1", Path: "/var/log/controller.log", RuntimeContainer: "runtime_1"},
		}},
		{ID: "empty", Name: "No configured sources", Sources: []SourceConfig{}},
	}}
	path := inventoryFile(t, encodedInventory(t, expected))
	got, err := ReadInventory(path)
	if err != nil || !reflect.DeepEqual(got, expected) {
		t.Fatalf("inventory=%+v error=%v, want %+v", got, err, expected)
	}
	replacement := Inventory{Adapters: []Target{}}
	if err := os.WriteFile(path, []byte(encodedInventory(t, replacement)), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = ReadInventory(path)
	if err != nil || !reflect.DeepEqual(got, replacement) {
		t.Fatalf("inventory not reloaded: %+v error=%v", got, err)
	}
}

func TestAdapterLogInventoryValidation(t *testing.T) {
	valid := Target{ID: "adapter", Name: "Adapter", Sources: []SourceConfig{{Name: "Log", Kind: "adapter", Container: "adapter-1"}}}
	for _, tc := range []struct {
		name   string
		target Target
		error  string
	}{
		{"missing id", Target{Name: "Adapter"}, "invalid adapter log target"},
		{"missing name", Target{ID: "adapter"}, "invalid adapter log target"},
		{"missing source name", Target{ID: "adapter", Name: "Adapter", Sources: []SourceConfig{{Kind: "adapter", File: "/tmp/log"}}}, "invalid adapter log source"},
		{"unsupported kind", Target{ID: "adapter", Name: "Adapter", Sources: []SourceConfig{{Name: "Log", Kind: "workload", File: "/tmp/log"}}}, "invalid adapter log source"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReadInventory(inventoryFile(t, encodedInventory(t, Inventory{Adapters: []Target{tc.target}})))
			if err == nil || err.Error() != tc.error {
				t.Fatalf("error=%v, want %q", err, tc.error)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		source SourceConfig
		error  string
	}{
		{"relative file", SourceConfig{File: "relative.log"}, "invalid file log source"},
		{"mixed file container", SourceConfig{File: "/tmp/log", Container: "adapter"}, "invalid file log source"},
		{"mixed file path", SourceConfig{File: "/tmp/log", Path: "/other"}, "invalid file log source"},
		{"missing location", SourceConfig{}, "invalid container log source"},
		{"option container", SourceConfig{Container: "--privileged"}, "invalid container log source"},
		{"slash container", SourceConfig{Container: "invalid/container"}, "invalid container log source"},
		{"relative container path", SourceConfig{Container: "adapter", Path: "relative.log"}, "invalid container log source"},
		{"invalid runtime container", SourceConfig{File: "/tmp/log", RuntimeContainer: "invalid/container"}, "invalid runtime container"},
		{"long container", SourceConfig{Container: strings.Repeat("a", 129)}, "invalid container log source"},
		{"long runtime container", SourceConfig{File: "/tmp/log", RuntimeContainer: strings.Repeat("a", 129)}, "invalid runtime container"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := valid
			tc.source.Name, tc.source.Kind = "Log", "adapter"
			target.Sources = []SourceConfig{tc.source}
			_, err := ReadInventory(inventoryFile(t, encodedInventory(t, Inventory{Adapters: []Target{target}})))
			if err == nil || err.Error() != tc.error {
				t.Fatalf("error=%v, want %q", err, tc.error)
			}
		})
	}
}

func TestAdapterLogInventoryCountLimits(t *testing.T) {
	for _, count := range []int{100, 101} {
		var targets []Target
		for index := 0; index < count; index++ {
			targets = append(targets, Target{ID: fmt.Sprint(index), Name: "Adapter"})
		}
		_, err := ReadInventory(inventoryFile(t, encodedInventory(t, Inventory{Adapters: targets})))
		if count == 100 && err != nil || count == 101 && (err == nil || err.Error() != "too many adapter log targets") {
			t.Fatalf("%d targets: %v", count, err)
		}
	}
	for _, count := range []int{8, 9} {
		target := Target{ID: "adapter", Name: "Adapter"}
		for index := 0; index < count; index++ {
			target.Sources = append(target.Sources, SourceConfig{Name: "Log", Kind: "adapter", Container: strings.Repeat("a", 128), RuntimeContainer: strings.Repeat("b", 128)})
		}
		_, err := ReadInventory(inventoryFile(t, encodedInventory(t, Inventory{Adapters: []Target{target}})))
		if count == 8 && err != nil || count == 9 && (err == nil || err.Error() != "invalid adapter log target") {
			t.Fatalf("%d sources: %v", count, err)
		}
	}
	_, err := ReadInventory(inventoryFile(t, `{"adapters":[{"id":"same","name":"One"},{"id":"same","name":"Two"}]}`))
	if err == nil || err.Error() != "invalid adapter log target" {
		t.Fatalf("duplicate id error=%v", err)
	}
}

func TestAdapterLogInventoryDecoderCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    Inventory
		error   string
	}{
		{"missing adapters", `{}`, Inventory{}, ""},
		{"null inventory", `null`, Inventory{}, ""},
		{"null adapters", `{"adapters":null}`, Inventory{}, ""},
		{"empty adapters", `{"adapters":[]}`, Inventory{Adapters: []Target{}}, ""},
		{"trailing object", `{"adapters":[]}{"ignored":true}`, Inventory{Adapters: []Target{}}, ""},
		{"trailing garbage", `{"adapters":[]}garbage`, Inventory{Adapters: []Target{}}, ""},
		{"duplicate JSON field", `{"adapters":null,"adapters":[]}`, Inventory{Adapters: []Target{}}, ""},
		{"unknown inventory field", `{"unknown":true}`, Inventory{}, `unknown field "unknown"`},
		{"unknown target field", `{"adapters":[{"unknown":true}]}`, Inventory{}, `unknown field "unknown"`},
		{"unknown source field", `{"adapters":[{"sources":[{"unknown":true}]}]}`, Inventory{}, `unknown field "unknown"`},
		{"wrong field type", `{"adapters":42}`, Inventory{}, "cannot unmarshal"},
		{"empty file", ``, Inventory{}, "EOF"},
		{"truncated JSON", `{"adapters":[`, Inventory{}, "unexpected EOF"},
		{"exact reader limit", strings.Repeat(" ", 65535) + `{}`, Inventory{}, ""},
		{"over reader limit", strings.Repeat(" ", 65536) + `{}`, Inventory{}, "unexpected EOF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadInventory(inventoryFile(t, tc.content))
			if tc.error != "" {
				if err == nil || !strings.Contains(err.Error(), tc.error) {
					t.Fatalf("error=%v, want %q", err, tc.error)
				}
			} else if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("inventory=%+v error=%v, want %+v", got, err, tc.want)
			}
		})
	}
	for _, path := range []string{"", filepath.Join(t.TempDir(), "missing.json")} {
		if _, err := ReadInventory(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing inventory %q error=%v", path, err)
		}
	}
}

func TestAdapterLogInventoryDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect adapter log inventory dependencies: %v\n%s", err, output)
	}
	packages := strings.Fields(string(output))
	if len(packages) != 1 || packages[0] != "neboagency.com/zpr-dashborad/internal/controlservice/adapterlogs" {
		t.Fatalf("adapter log inventory must use only the standard library, got %v", packages)
	}
}
