package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLabMachineOwnerPlacement(t *testing.T) {
	profilePath := filepath.Join("examples", "organizations", "load-lab.json")
	organization, err := loadSimulatorOrganization(filepath.Dir(profilePath), "load-lab")
	if err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(filepath.Join("examples", "organizations", "load-lab", "directory.ldif"))
	if err != nil {
		t.Fatal(err)
	}
	directory, err := parseAssertionLDAPAttributes(string(seed), []string{"l", "zprMachineLocation"})
	if err != nil {
		t.Fatal(err)
	}
	for _, machineID := range []string{"machine-03", "machine-05"} {
		t.Run(machineID, func(t *testing.T) {
			owners := organization.MachineOwners[machineID]
			if len(owners) != 1 {
				t.Fatalf("machine owners = %v; want one owner", owners)
			}
			output, err := exec.Command("jq", "-er", "--arg", "owner", owners[0],
				".directory.people[] | select(.uid == $owner) | .location", profilePath).CombinedOutput()
			if err != nil {
				t.Fatalf("startup owner-location lookup failed: %v: %s", err, output)
			}
			location := strings.TrimSpace(string(output))
			matchingNodes := 0
			for _, node := range organization.Runtime.Nodes {
				if node.Location == location {
					matchingNodes++
				}
			}
			if location != "Load Test Bench" || matchingNodes != 1 {
				t.Fatalf("owner location %q matches %d nodes; want Load Test Bench and one node", location, matchingNodes)
			}
			for _, identity := range []string{owners[0], machineID} {
				attribute := "l"
				if identity == machineID {
					attribute = "zprMachineLocation"
				}
				values := directory.PersonAttributes[identity][attribute]
				if len(values) != 1 || values[0] != location {
					t.Errorf("%s LDAP %s = %v; want [%s]", identity, attribute, values, location)
				}
			}
		})
	}
}
