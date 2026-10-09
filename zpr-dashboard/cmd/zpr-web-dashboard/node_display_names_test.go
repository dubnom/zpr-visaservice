package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNodeDisplayNamesPreserveIdentityAndIgnoreAdapters(t *testing.T) {
	file := filepath.Join(t.TempDir(), "names.json")
	if err := os.WriteFile(file, []byte(`{"fd5a:5052:90de::10":"Milwaukee","fd5a:5052:90de::11":"Shenzhen","fd5a:5052:90de::12":"Tijuana"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_NODE_DISPLAY_NAMES_FILE", file)
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/nonexistent")
	t.Setenv("SIMULATION_MANIFEST", "/nonexistent")
	out := snapshot{Actors: []actor{
		{CN: "node0.demo", Node: true, ZPRAddress: "fd5a:5052:90de:0:0:0:0:10"},
		{CN: "node1.demo", Node: true, ZPRAddress: "fd5a:5052:90de::11"},
		{CN: "node2.demo", Node: true, ZPRAddress: "fd5a:5052:90de::12"},
		{CN: "other-organization", Node: true, ZPRAddress: "fd00::10"},
		{CN: "adapter", ZPRAddress: "fd5a:5052:90de::10"},
	}}
	populateNodeDisplayNames(&out)
	for index, name := range []string{"Milwaukee", "Shenzhen", "Tijuana", "", ""} {
		if out.Actors[index].DisplayName != name {
			t.Fatalf("actor %d: display name=%q, want %q", index, out.Actors[index].DisplayName, name)
		}
	}
	if out.Actors[0].CN != "node0.demo" || out.Actors[0].ZPRAddress != "fd5a:5052:90de:0:0:0:0:10" || len(out.Errors) != 0 {
		t.Fatalf("identity changed or unexpected error: %+v", out)
	}
}

func TestNodeDisplayNamesReportConfigurationErrors(t *testing.T) {
	for _, data := range []string{`null`, `{"bad":"City"}`, `{"127.0.0.1":"City"}`, `{"fd00::1":""}`,
		`{"fd00::1":"City\nName"}`, `{"fd00::1":" City"}`, `{"fd00::1":"A","fd00:0:0:0:0:0:0:1":"B"}`, `{ } { }`} {
		t.Run(data, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "names.json")
			if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("ZPR_NODE_DISPLAY_NAMES_FILE", file)
			out := snapshot{Actors: []actor{{CN: "node", Node: true, ZPRAddress: "fd00::1"}}}
			populateNodeDisplayNames(&out)
			if len(out.Errors) != 1 || out.Actors[0].DisplayName != "" {
				t.Fatalf("invalid config did not surface an error: %+v", out)
			}
		})
	}
}
