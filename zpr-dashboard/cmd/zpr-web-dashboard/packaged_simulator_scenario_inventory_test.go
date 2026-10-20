package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestPackagedSimulatorScenarioInventory(t *testing.T) {
	scenarioDirectory := filepath.Join("..", "..", "packaging", "demo-runtime", "scenarios")
	tests := []struct {
		id string
	}{
		{id: "packaged-admission-acceptance"},
		{id: "packaged-demo-acceptance"},
		{id: "packaged-workload-acceptance"},
	}

	entries, err := os.ReadDir(scenarioDirectory)
	if err != nil {
		t.Fatal(err)
	}
	var scenarioFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			scenarioFiles = append(scenarioFiles, entry.Name())
		}
	}
	sort.Strings(scenarioFiles)
	wantFiles := make([]string, 0, len(tests))
	for _, test := range tests {
		wantFiles = append(wantFiles, test.id+".json")
	}
	sort.Strings(wantFiles)
	if !reflect.DeepEqual(scenarioFiles, wantFiles) {
		t.Fatalf("packaged scenario files = %v, want exactly %v", scenarioFiles, wantFiles)
	}

	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			path := filepath.Join(scenarioDirectory, test.id+".json")
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()

			decoder := json.NewDecoder(file)
			decoder.DisallowUnknownFields()
			var scenario simulatorScenario
			if err := decoder.Decode(&scenario); err != nil {
				t.Fatalf("decode scenario: %v", err)
			}
			var trailing any
			if err := decoder.Decode(&trailing); err != io.EOF {
				if err == nil {
					t.Fatal("scenario contains trailing JSON data")
				}
				t.Fatalf("read trailing scenario data: %v", err)
			}

			if scenario.ID != test.id {
				t.Errorf("scenario id = %q, want %q", scenario.ID, test.id)
			}
			if scenario.OrganizationID != "demo" {
				t.Errorf("organization_id = %q, want explicit binding to demo", scenario.OrganizationID)
			}
			if scenario.Name == "" || scenario.Description == "" {
				t.Error("scenario name and description must be declared")
			}
			if len(scenario.Steps) != 1 || scenario.Steps[0].Action != "verify_packaged_demo" ||
				scenario.Steps[0].TimeoutSeconds != 90 {
				t.Errorf("declared steps = %+v, want one verify_packaged_demo step with 90-second timeout",
					scenario.Steps)
			}
			if len(scenario.Cleanup) != 0 {
				t.Errorf("declared cleanup actions = %+v, want none", scenario.Cleanup)
			}

			var machineRefs []string
			for _, phase := range []struct {
				name  string
				steps []simulatorScenarioStep
			}{
				{name: "steps", steps: scenario.Steps},
				{name: "cleanup", steps: scenario.Cleanup},
			} {
				for index, step := range phase.steps {
					if step.Machine != "" {
						machineRefs = append(machineRefs, step.Machine)
					}
					if step.Action == "" {
						t.Errorf("%s[%d] has no declared action", phase.name, index)
					}
				}
			}
			sort.Strings(machineRefs)
			if len(machineRefs) != 0 {
				t.Errorf("explicit step+cleanup machine refs = %v, want none (do not infer organization machines)", machineRefs)
			}
		})
	}
}
