package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

type budgetAuthorityTestAuthorizer struct {
	identity operatorauth.Identity
	wantOrg  string
	wantPerm string
	gotOrg   string
	gotPerm  string
	calls    int
	deny     bool
}

func (authorizer *budgetAuthorityTestAuthorizer) Authorize(
	request *http.Request,
	organization string,
	permission string,
) (operatorauth.Identity, error) {
	authorizer.calls++
	authorizer.gotOrg = organization
	authorizer.gotPerm = permission
	if organization != authorizer.wantOrg || permission != authorizer.wantPerm ||
		request.Header.Get("Origin") != "https://room.example" ||
		request.Header.Get("X-ZPR-CSRF") != "csrf-proof" ||
		request.Header.Get("Sec-Fetch-Site") != "same-origin" || authorizer.deny {
		return operatorauth.Identity{}, errors.New("operator request denied")
	}
	return authorizer.identity, nil
}

func TestConfiguredBudgetAuthorityDisabledWithoutExplicitDatabasePath(t *testing.T) {
	t.Setenv("ZPR_BUDGET_AUTHORITY_DB_PATH", "")
	t.Setenv("ZPR_OPERATOR_DELEGATION_TRUST_FILE", "")
	handler, closeStore := configuredBudgetAuthority()
	defer func() {
		if err := closeStore(); err != nil {
			t.Fatal(err)
		}
	}()
	body, err := json.Marshal(budgetauthority.ScopeRequest{
		OrganizationID: "org-a", ScenarioID: "scenario-a", PlanDigest: "sha256:plan-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"https://control-service.example/api/budget-authority/current",
		bytes.NewReader(body),
	)
	request.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{}},
		VerifiedChains:   [][]*x509.Certificate{{{}}},
	}
	response := httptest.NewRecorder()
	handler.Current(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("disabled budget authority status=%d cache=%q body=%s",
			response.Code, response.Header().Get("Cache-Control"), response.Body)
	}
	operatorRequest := httptest.NewRequest(
		http.MethodPost,
		"https://control-service.example"+operatordelegation.Prefix+"budget-authority/approvals",
		nil,
	)
	operatorRequest.TLS = request.TLS
	response = httptest.NewRecorder()
	handler.Operator(response, operatorRequest)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled budget approval status=%d body=%s", response.Code, response.Body)
	}
}

func TestBudgetAuthorityControlRoomProxyRequiresScopedApproval(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := operatordelegation.NewSigner(
		operatordelegation.SignerConfig{Version: 1, Audience: "https://control-service.example", KeyID: "room"},
		privateKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	authorizer := &budgetAuthorityTestAuthorizer{
		identity: operatorauth.Identity{
			Issuer: "https://idp.example", Subject: "operator-1",
			Organizations: []string{"org-a"}, Permissions: []string{"budget.approve"},
		},
		wantOrg: "org-a", wantPerm: "budget.approve",
	}
	var forwarded bool
	proxy := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		forwarded = true
		if request.URL.Path != operatordelegation.Prefix+"budget-authority/approvals" {
			t.Errorf("delegated path = %q", request.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	})
	handler := budgetAuthorityOperatorProxy(authorizer, signer, proxy)
	requestBody, err := json.Marshal(struct {
		Scope     budgetauthority.ScopeRequest `json:"scope"`
		Decision  budgetauthority.Decision     `json:"decision"`
		ExpiresAt time.Time                    `json:"expires_at"`
	}{
		Scope: budgetauthority.ScopeRequest{
			OrganizationID: "org-a", ScenarioID: "scenario-a", PlanDigest: "sha256:plan-a",
		},
		Decision: budgetauthority.DecisionApproved, ExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPost, "https://room.example/api/budget-authority/approvals",
		bytes.NewReader(requestBody),
	)
	request.Header.Set("Origin", "https://room.example")
	request.Header.Set("X-ZPR-CSRF", "csrf-proof")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !forwarded ||
		authorizer.calls != 1 || authorizer.gotOrg != "org-a" ||
		authorizer.gotPerm != "budget.approve" {
		t.Fatalf("proxy status=%d forwarded=%t auth=%q/%q calls=%d body=%s",
			response.Code, forwarded, authorizer.gotOrg, authorizer.gotPerm, authorizer.calls, response.Body)
	}

	authorizer.deny = true
	forwarded = false
	request = httptest.NewRequest(
		http.MethodPost, "https://room.example/api/budget-authority/approvals",
		bytes.NewReader(requestBody),
	)
	request.Header.Set("Origin", "https://room.example")
	request.Header.Set("X-ZPR-CSRF", "csrf-proof")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || forwarded {
		t.Fatalf("denied proxy status=%d forwarded=%t body=%s", response.Code, forwarded, response.Body)
	}

	authorizer.deny = false
	authorizer.identity.Organizations = []string{"other-org"}
	request = httptest.NewRequest(
		http.MethodPost, "https://room.example/api/budget-authority/approvals",
		bytes.NewReader(requestBody),
	)
	request.Header.Set("Origin", "https://room.example")
	request.Header.Set("X-ZPR-CSRF", "csrf-proof")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	forwarded = false
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || forwarded {
		t.Fatalf("unscoped identity status=%d forwarded=%t body=%s", response.Code, forwarded, response.Body)
	}

	authorizer.identity.Organizations = []string{"org-a"}
	authorizer.identity.Permissions = []string{"read"}
	request = httptest.NewRequest(
		http.MethodPost, "https://room.example/api/budget-authority/approvals",
		bytes.NewReader(requestBody),
	)
	request.Header.Set("Origin", "https://room.example")
	request.Header.Set("X-ZPR-CSRF", "csrf-proof")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	forwarded = false
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || forwarded {
		t.Fatalf("missing budget.approve status=%d forwarded=%t body=%s", response.Code, forwarded, response.Body)
	}

	authorizer.identity.Permissions = []string{"budget.approve"}
	request = httptest.NewRequest(
		http.MethodPost, "https://room.example/api/budget-authority/approvals",
		bytes.NewReader(requestBody),
	)
	request.Header.Set("Origin", "https://room.example")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	forwarded = false
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || forwarded {
		t.Fatalf("missing CSRF status=%d forwarded=%t body=%s", response.Code, forwarded, response.Body)
	}
}
