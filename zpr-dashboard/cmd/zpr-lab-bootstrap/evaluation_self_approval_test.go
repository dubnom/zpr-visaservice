package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestEvaluationSelfApprovalTrustPreservesAuthority(t *testing.T) {
	config := testEvaluationDataPlaneConfig()
	config.OrganizationID = "velocity"
	state := filepath.Join(t.TempDir(), "install")
	inventory, err := prepareEvaluation(state, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	original := operatordelegation.Config{Version: 1, Audience: "https://localhost:24790",
		Keys: []operatordelegation.TrustedKey{{KeyID: "room", PublicKey: base64.RawURLEncoding.EncodeToString(public),
			CertificateSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		Grants: []operatorauth.Grant{
			{Issuer: "https://identity.example", Subject: "admin", Organizations: []string{"velocity"}, Permissions: []string{"subject-link.read", "subject-link.approve"}},
			{Issuer: "https://identity.example", Subject: "reader", Organizations: []string{"velocity"}, Permissions: []string{"subject-link.read"}},
		}}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, evaluationDelegationTrust), data, 0600); err != nil {
		t.Fatal(err)
	}
	trustPath := filepath.Join(t.TempDir(), "delegation.json")
	servicesPath := filepath.Join(t.TempDir(), "services.json")
	services := evaluationOperatorServices{Version: 1, Organization: "velocity", OperatorDelegationTrustFile: trustPath}
	data, err = json.Marshal(services)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(servicesPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"explicit", "audience", "key", "subject", "organization", "permission", "removed", "reader"} {
		t.Run(mode, func(t *testing.T) {
			data, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			var trust operatordelegation.Config
			if err := json.Unmarshal(data, &trust); err != nil {
				t.Fatal(err)
			}
			grant := &trust.Grants[0]
			grant.Permissions = append(grant.Permissions, "approval.self")
			switch mode {
			case "audience":
				trust.Audience = "https://other.example"
			case "key":
				trust.Keys[0].CertificateSHA256 = string(make([]byte, 64))
			case "subject":
				grant.Subject = "another-user"
			case "organization":
				grant.Organizations = []string{"northstar"}
			case "permission":
				grant.Permissions = append(grant.Permissions, "organization.archive")
			case "removed":
				grant.Permissions = []string{"approval.self"}
			case "reader":
				grant.Permissions = slices.DeleteFunc(grant.Permissions, func(permission string) bool { return permission == "approval.self" })
				trust.Grants[1].Permissions = append(trust.Grants[1].Permissions, "approval.self")
				trust.Grants[1].Permissions = []string{"subject-link.read", "approval.self"}
			}
			data, err = json.Marshal(trust)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(trustPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := loadEvaluationOperatorServices(servicesPath, state, inventory)
			if mode != "explicit" {
				if err == nil {
					t.Fatal("authority changed by self-approval configuration")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(loaded.environment("control-service"), "ZPR_OPERATOR_DELEGATION_TRUST_FILE="+trustPath) ||
				len(loaded.environment("control-room")) != 0 {
				t.Fatal("backend trust override has incorrect ownership")
			}
		})
	}
}
