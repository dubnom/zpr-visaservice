package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
