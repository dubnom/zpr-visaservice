package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func renderOrganizationGeography(t *testing.T, organization simulatorOrganization) ([]byte, error) {
	t.Helper()
	content, err := json.Marshal(organization)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("jq", "-r", "-f", filepath.Join("..", "..", "scripts", "render-node-geography.jq"))
	command.Stdin = strings.NewReader(string(content))
	return command.CombinedOutput()
}

func TestBundledOrganizationGeographyReachesRuntimeNodeCNs(t *testing.T) {
	organizations, err := loadSimulatorOrganizations(filepath.Join("examples", "organizations"))
	if err != nil {
		t.Fatal(err)
	}
	nodeCount := 0
	for _, organization := range organizations {
		t.Run(organization.ID, func(t *testing.T) {
			rendered, err := renderOrganizationGeography(t, organization)
			if err != nil {
				t.Fatalf("render geography: %v: %s", err, rendered)
			}
			var config struct {
				Nodes map[string]struct {
					Latitude  float64
					Longitude float64
				}
			}
			if _, err := toml.Decode(string(rendered), &config); err != nil {
				t.Fatal(err)
			}
			if len(config.Nodes) != len(organization.Runtime.Nodes) {
				t.Fatalf("rendered %d nodes; want %d", len(config.Nodes), len(organization.Runtime.Nodes))
			}
			for index, node := range organization.Runtime.Nodes {
				nodeCount++
				if node.Latitude == nil || node.Longitude == nil || !strings.HasPrefix(node.CoordinateNote, "Simulation ") {
					t.Fatalf("node %q lacks coordinates or approximation provenance", node.ID)
				}
				cn := fmt.Sprintf("node%d.demo", index)
				coordinates, ok := config.Nodes[cn]
				if !ok || coordinates.Latitude != *node.Latitude || coordinates.Longitude != *node.Longitude {
					t.Fatalf("node %q coordinates did not reach %q: %s", node.ID, cn, rendered)
				}
			}
		})
	}
	if len(organizations) != 5 || nodeCount != 8 {
		t.Fatalf("geography coverage: %d organizations, %d nodes; want 5 and 8", len(organizations), nodeCount)
	}
}

func TestOrganizationGeographyValidationAndRendering(t *testing.T) {
	number := func(value float64) *float64 { return &value }
	for _, test := range []struct {
		name      string
		latitude  *float64
		longitude *float64
		valid     bool
	}{
		{"absent", nil, nil, true},
		{"zero", number(0), number(0), true},
		{"northwest", number(90), number(-180), true},
		{"southeast", number(-90), number(180), true},
		{"latitude-only", number(1), nil, false},
		{"longitude-only", nil, number(1), false},
		{"latitude-range", number(91), number(1), false},
		{"longitude-range", number(1), number(-181), false},
		{"nan", number(math.NaN()), number(1), false},
		{"infinity", number(1), number(math.Inf(1)), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "northstar")
			if err != nil {
				t.Fatal(err)
			}
			organization.Runtime.Nodes[0].Latitude = test.latitude
			organization.Runtime.Nodes[0].Longitude = test.longitude
			err = validateSimulatorOrganization(organization)
			if (err == nil) != test.valid {
				t.Fatalf("validation error=%v; valid=%v", err, test.valid)
			}
			if test.name == "nan" || test.name == "infinity" {
				return // JSON rejects non-finite numbers before the provisioning renderer.
			}
			rendered, err := renderOrganizationGeography(t, organization)
			if (err == nil) != test.valid {
				t.Fatalf("render error=%v output=%s; valid=%v", err, rendered, test.valid)
			}
			if test.name == "absent" && strings.TrimSpace(string(rendered)) != "" {
				t.Fatalf("missing coordinates produced metadata: %s", rendered)
			}
			if test.name == "zero" && !strings.Contains(string(rendered), "latitude = 0\nlongitude = 0") {
				t.Fatalf("zero coordinates lost: %s", rendered)
			}
		})
	}
}
