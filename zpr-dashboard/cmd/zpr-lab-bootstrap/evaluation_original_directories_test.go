package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvaluationOriginalOrganizationDirectoryState(t *testing.T) {
	// LDIF is explicit operator input. The production bootstrap must not load
	// organization profiles to discover its identity or directory configuration.
	source, err := filepath.Abs(filepath.Join("..", "..", "packaging", "demo-runtime", "organizations"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	installIDs := map[string]string{}
	for _, test := range []struct {
		id, baseDN, subject string
		files               []string
	}{
		{"northstar", "dc=northstar,dc=test", "elena.park", []string{"demo-company.ldif", "demo-employees.ldif", "demo-roles.ldif", "demo-machines.ldif"}},
		{"great-lakes", "dc=greatlakes,dc=test", "maya.brooks", []string{"directory.ldif"}},
		{"load-lab", "dc=loadlab,dc=test", "load.client", []string{"directory.ldif"}},
		{"redwood", "dc=redwood,dc=test", "north.dispatch.01", []string{"directory.ldif", "directory-people.ldif"}},
		{"velocity", "dc=velocity,dc=test", "alex.rivera", []string{"directory.ldif"}},
	} {
		t.Run(test.id, func(t *testing.T) {
			config := testEvaluationDataPlaneConfig()
			config.OrganizationID = test.id
			config.Directory = &evaluationDirectory{BaseDN: test.baseDN}
			for _, name := range test.files {
				config.Directory.LDIFFiles = append(config.Directory.LDIFFiles, filepath.Join(source, test.id, name))
			}
			state := filepath.Join(root, test.id)
			inventory, err := prepareEvaluation(state, config, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if other, exists := installIDs[inventory.InstallID]; exists {
				t.Fatalf("organizations %s and %s share an installation identity", other, test.id)
			}
			installIDs[inventory.InstallID] = test.id
			before, err := os.ReadFile(filepath.Join(state, "directory.ldif"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(before), "uid: "+test.subject+"\n") || !strings.Contains(string(before), "dn: "+test.baseDN+"\n") {
				t.Fatal("original directory identity or base was lost")
			}
			for _, name := range []string{"source-registration.json", "subject-link-providers.json"} {
				data, err := os.ReadFile(filepath.Join(state, name))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(data), test.baseDN) || !strings.Contains(string(data), test.id) {
					t.Fatalf("%s is not bound to the independent organization", name)
				}
			}
			restarted, err := prepareEvaluation(state, config, time.Now().Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(filepath.Join(state, "directory.ldif"))
			if err != nil || string(before) != string(after) || restarted.InstallID != inventory.InstallID {
				t.Fatal("restart changed isolated directory state or install identity", err)
			}
			if evaluationOrganization(restarted) != test.id {
				t.Fatal("restart changed organization identity")
			}
			wrong := config
			wrong.OrganizationID = "another-organization"
			if _, err := prepareEvaluation(state, wrong, time.Now()); err == nil {
				t.Fatal("existing directory rebound to another organization")
			}
		})
	}
}
