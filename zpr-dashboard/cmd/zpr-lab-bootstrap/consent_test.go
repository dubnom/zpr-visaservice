package main

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDedicatedIdentityProvidersSkipConsent(t *testing.T) {
	providers := testProviderConfig()
	labFiles, err := generateConfigured("lab.example.test", time.Now(), nil, &providers)
	if err != nil {
		t.Fatal(err)
	}
	var lab struct {
		OAuth2 struct {
			SkipApprovalScreen bool     `yaml:"skipApprovalScreen"`
			ResponseTypes      []string `yaml:"responseTypes"`
		} `yaml:"oauth2"`
	}
	if err := yaml.Unmarshal(labFiles["dex.yaml"], &lab); err != nil {
		t.Fatal(err)
	}
	if !lab.OAuth2.SkipApprovalScreen || len(lab.OAuth2.ResponseTypes) != 1 || lab.OAuth2.ResponseTypes[0] != "code" {
		t.Fatal("lab provider must skip consent while retaining authorization-code sign-in")
	}
	for _, simulatorPort := range []int{0, 18788} {
		config := testEvaluationConfig()
		config.SimulatorPort = simulatorPort
		files, _, err := generateEvaluation(t.TempDir(), config, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		var evaluation struct {
			OAuth2 struct {
				SkipApprovalScreen bool     `json:"skipApprovalScreen"`
				ResponseTypes      []string `json:"responseTypes"`
				PasswordConnector  string   `json:"passwordConnector"`
			} `json:"oauth2"`
		}
		if err := json.Unmarshal(files["dex.json"], &evaluation); err != nil {
			t.Fatal(err)
		}
		if !evaluation.OAuth2.SkipApprovalScreen || evaluation.OAuth2.PasswordConnector != "local" ||
			len(evaluation.OAuth2.ResponseTypes) != 1 || evaluation.OAuth2.ResponseTypes[0] != "code" {
			t.Fatalf("evaluation provider with Simulator port %d changed authentication or retained consent", simulatorPort)
		}
	}
}
