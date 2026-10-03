package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserAndDevicePolicyExamplesParseWithZPLC(t *testing.T) {
	compiler := os.Getenv("ZPR_ZPLC_BIN")
	if compiler == "" {
		t.Skip("set ZPR_ZPLC_BIN to run example policy compiler checks")
	}
	for _, organizationID := range []string{"northstar", "redwood"} {
		catalogPath := filepath.Join("examples", organizationID, "demo-policy-catalog.json")
		catalogBytes, err := os.ReadFile(catalogPath)
		if err != nil {
			t.Fatal(err)
		}
		var catalog demoPolicyCatalog
		if err := json.Unmarshal(catalogBytes, &catalog); err != nil {
			t.Fatal(err)
		}
		configPath, err := filepath.Abs(filepath.Join("examples", organizationID, "policy-demo.zplc"))
		if err != nil {
			t.Fatal(err)
		}
		workspace := &policyWorkspace{configPath: configPath, compiler: compiler}
		for _, record := range catalog.Records {
			record := record
			t.Run(organizationID+"/"+record.Name, func(t *testing.T) {
				result := workspace.checkUnlocked(context.Background(), record.Content)
				if !result.Valid {
					t.Fatalf("ZPLC rejected policy: %s", result.Diagnostics)
				}
			})
		}
	}
}

func TestFinancePolicyExampleKeepsOriginalServiceBindings(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("examples", "northstar", "demo-policy-catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog demoPolicyCatalog
	if err := json.Unmarshal(contents, &catalog); err != nil {
		t.Fatal(err)
	}
	var source string
	for _, record := range catalog.Records {
		if record.Name == "Finance department access" {
			source = record.Content
			break
		}
	}
	if source == "" {
		t.Fatal("finance policy example is missing")
	}
	bindings := make(map[string]string)
	service := ""
	for _, line := range strings.Split(source, "\n") {
		words := strings.Fields(line)
		if len(words) < 2 {
			continue
		}
		switch words[0] {
		case "service", "provide":
			service = words[1]
		case "define":
			service = ""
		case "allow":
			bindings[strings.TrimSuffix(words[1], ".")] = service
		}
	}
	for subject, expected := range map[string]string{"AccountingStaff": "FinanceWorkspace", "FinanceStaff": "FinanceWorkspace", "PayrollStaff": "PayrollRecords"} {
		if bindings[subject] != expected {
			t.Errorf("%s targets %q, want %q", subject, bindings[subject], expected)
		}
	}
}
