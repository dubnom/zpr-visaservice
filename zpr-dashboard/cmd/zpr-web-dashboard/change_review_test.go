package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type reviewAuthFixture struct {
	subject string
	denied  string
}

func (f *reviewAuthFixture) Authorize(r *http.Request, org, permission string) (operatorauth.Identity, error) {
	if org != "great-lakes" || permission == f.denied || (r.Method == "POST" && r.Header.Get("X-ZPR-CSRF") != "proof") {
		return operatorauth.Identity{}, errors.New("denied")
	}
	return operatorauth.Identity{Issuer: "https://idp.example", Subject: f.subject}, nil
}
func TestChangeReviewAPIRevisionBindingPermissionsDriftAndNoApplication(t *testing.T) {
	t.Setenv("SIMULATOR_URL", "http://127.0.0.1:1")
	store, err := changereview.Open(filepath.Join(t.TempDir(), "private", "reviews.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	auth := &reviewAuthFixture{subject: "author"}
	current := "saved source"
	captures := 0
	api := &changeReviewAPI{store: store, auth: auth, capture: func(_ context.Context, org, kind, target string, revision int) (changereview.Proposal, error) {
		captures++
		return changereview.Proposal{Organization: org, Kind: kind, Target: target, Revision: revision,
			Before: "previous source", BaseRevision: 1, After: current, Validation: "Validated", Impact: "Draft only"}, nil
	}}
	call := func(method, path, body, csrf string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-ZPR-CSRF", csrf)
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		return w
	}
	input := `{"organization":"great-lakes","kind":"gateway","target":"internet-gateway","revision":2,"reason":"Restrict access"}`
	if w := call("POST", changeReviewPrefix+"requests", input, ""); w.Code != 403 || captures != 0 {
		t.Fatal("missing CSRF reached source")
	}
	auth.denied = "gateway.edit"
	if w := call("POST", changeReviewPrefix+"requests", input, "proof"); w.Code != 403 || captures != 0 {
		t.Fatal("missing domain permission reached source")
	}
	auth.denied = ""
	w := call("POST", changeReviewPrefix+"requests", input, "proof")
	if w.Code != 201 {
		t.Fatalf("submit %d %s", w.Code, w.Body)
	}
	var record changereview.Record
	if err = json.Unmarshal(w.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Proposal.After != current || record.Proposal.Author.Subject != "author" {
		t.Fatal("authoritative source/author not captured")
	}
	decision := func(version int, digest string) string {
		data, _ := json.Marshal(changeReviewInput{Organization: "great-lakes", ExpectedVersion: version, Digest: digest, Reason: "Reviewed exact diff"})
		return string(data)
	}
	path := changeReviewPrefix + "requests/" + record.ID
	if w := call("POST", path+"/approve", decision(1, record.Digest), "proof"); w.Code != 409 {
		t.Fatal("default self approval accepted")
	}
	auth.subject = "reviewer"
	current = "changed source"
	if w := call("POST", path+"/approve", decision(1, record.Digest), "proof"); w.Code != 409 {
		t.Fatal("stale source approved")
	}
	current = "saved source"
	if w := call("POST", path+"/approve", decision(1, "wrong"), "proof"); w.Code != 409 {
		t.Fatal("wrong digest approved")
	}
	if w := call("POST", path+"/approve", decision(1, record.Digest), "proof"); w.Code != 200 {
		t.Fatalf("reviewer approve %d %s", w.Code, w.Body)
	}
	if w := call("POST", path+"/apply", decision(2, record.Digest), "proof"); w.Code != 409 || !strings.Contains(w.Body.String(), "Application is disabled") {
		t.Fatal("application not fail-closed")
	}
	auth.denied = "policy.read"
	if w := call("GET", changeReviewPrefix+"requests?organization=great-lakes", "", ""); w.Code != 403 {
		t.Fatal("list exposed domain source without domain permission")
	}
	auth.denied = ""
	if w := call("GET", path+"?organization=other", "", ""); w.Code != 403 {
		t.Fatal("wrong organization read accepted")
	}
	if w := call("POST", changeReviewPrefix+"requests?unexpected=1", input, "proof"); w.Code != 400 {
		t.Fatal("query mutation accepted")
	}
	if w := call("POST", changeReviewPrefix+"requests", strings.TrimSuffix(input, "}")+`,"after":"browser source"}`, "proof"); w.Code != 400 {
		t.Fatal("browser content injection accepted")
	}
}

func TestChangeReviewExplicitSingleOperatorApproval(t *testing.T) {
	store, err := changereview.Open(filepath.Join(t.TempDir(), "private", "reviews.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	p := changereview.Proposal{Organization: "great-lakes", Kind: "policy", Target: "record", Revision: 1, After: "source",
		Validation: "Validated", Impact: "Draft only", Reason: "Change", Author: changereview.Principal{Issuer: "https://idp.example", Subject: "author"}}
	record, err := store.Submit(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	api := &changeReviewAPI{store: store, auth: &reviewAuthFixture{subject: "author"}, allowSelf: true,
		capture: func(context.Context, string, string, string, int) (changereview.Proposal, error) { return p, nil }}
	body, _ := json.Marshal(changeReviewInput{Organization: "great-lakes", Digest: record.Digest, ExpectedVersion: 1, Reason: "Single operator reviewed"})
	r := httptest.NewRequest("POST", changeReviewPrefix+"requests/"+record.ID+"/approve", strings.NewReader(string(body)))
	r.Header.Set("X-ZPR-CSRF", "proof")
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("opt-in not honored: %d %s", w.Code, w.Body)
	}
}

func TestChangeReviewGatewayCapturesOnlyCurrentInstalledSavedRevision(t *testing.T) {
	contract := gatewayInstanceContract{OrganizationID: "great-lakes", InstanceID: "public-egress", AdapterCN: "gateway-public-egress", ServiceName: "public-egress.svc.zpr"}
	data := gatewayInstanceConfigJSON(t, map[string]any{"organization_id": "great-lakes"})
	revision := 2
	proxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/gateways/contracts":
			writeJSON(w, 200, gatewayContractsResponse{OrganizationID: "great-lakes", Contracts: []gatewayInstanceContract{contract}})
		case "/api/gateways/configs/" + contract.InstanceID:
			writeJSON(w, 200, gatewayConfigRecord{OrganizationID: "great-lakes", InstanceID: contract.InstanceID, CurrentRevision: revision,
				Revisions: []gatewayConfigRevision{{Revision: 1, Config: data}, {Revision: 2, Config: data}}})
		default:
			t.Fatalf("unexpected production request: %s", r.URL.Path)
		}
	})
	capture := changeReviewDomainCapture(proxy)
	p, err := capture(context.Background(), "great-lakes", "gateway", contract.InstanceID, 2)
	if err != nil || p.After != string(data) || p.Before != string(data) || p.BaseRevision != 1 {
		t.Fatalf("capture failed: %+v %v", p, err)
	}
	revision = 3
	if _, err = capture(context.Background(), "great-lakes", "gateway", contract.InstanceID, 2); err == nil {
		t.Fatal("old gateway revision captured")
	}
	if _, err = capture(context.Background(), "other", "gateway", contract.InstanceID, 2); err == nil {
		t.Fatal("cross-org contract captured")
	}
	revision = 2
	contract.OrganizationID = "other"
	if _, err = capture(context.Background(), "great-lakes", "gateway", contract.InstanceID, 2); err == nil {
		t.Fatal("mixed-organization installed contract captured")
	}
}

func TestChangeReviewPolicyUsesProductionValidationAndRejectsConcurrentEdits(t *testing.T) {
	t.Setenv("SIMULATOR_URL", "http://127.0.0.1:1")
	changed := false
	checks := 0
	target := "0123456789abcdef"
	record := policyRecord{ID: target, Kind: "policy", Name: "Access policy", CurrentRevision: 2, Content: "allow users to services;"}
	proxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/policy":
			writeJSON(w, 200, policyStatus{OrganizationID: "great-lakes", Configured: true})
		case "/api/policy/records/" + target:
			current := record
			if changed && checks > 0 {
				current.CurrentRevision = 3
			}
			writeJSON(w, 200, current)
		case "/api/policy/records/" + target + "/revisions/1":
			writeJSON(w, 200, policyRevision{RecordID: target, Number: 1, Content: "old source"})
		case "/api/policy/check":
			if r.Method != http.MethodPost {
				t.Fatal("compiler check is not POST")
			}
			var request policySourceRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Source != record.Content {
				t.Fatal("compiler was not given authoritative saved source")
			}
			checks++
			writeJSON(w, 200, policyCheckResponse{Valid: true})
		default:
			t.Fatalf("unexpected service request: %s", r.URL.Path)
		}
	})
	capture := changeReviewDomainCapture(proxy)
	p, err := capture(context.Background(), "great-lakes", "policy", target, 2)
	if err != nil || p.Before != "old source" || p.After != record.Content || p.BaseRevision != 1 || checks != 1 {
		t.Fatalf("policy capture failed: %+v %v", p, err)
	}
	changed = true
	checks = 0
	if _, err := capture(context.Background(), "great-lakes", "policy", target, 2); err == nil {
		t.Fatal("policy changed during validation but submission succeeded")
	}
	if _, err := capture(context.Background(), "other", "policy", target, 2); err == nil {
		t.Fatal("different production organization accepted")
	}
	changed = false
	for _, kind := range []string{"assertion", "definitions"} {
		record.Kind = kind
		if _, err := capture(context.Background(), "great-lakes", "policy", target, 2); err == nil {
			t.Fatal("non-policy record accepted")
		}
	}
	record.Kind = "policy"
	record.Archived = true
	if _, err := capture(context.Background(), "great-lakes", "policy", target, 2); err == nil {
		t.Fatal("archived policy accepted")
	}
	record.Archived = false
	for _, invalidPath := range []string{"/api/policy/check", "/api/policy/records/" + target + "/revisions/1"} {
		rejecting := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == invalidPath {
				writeJSON(w, 200, map[string]any{})
				return
			}
			proxy.ServeHTTP(w, r)
		})
		if _, err := changeReviewDomainCapture(rejecting)(context.Background(), "great-lakes", "policy", target, 2); err == nil {
			t.Fatalf("invalid production response accepted: %s", invalidPath)
		}
	}
}

func TestChangeReviewRequiresExplicitConfigurationAndNamedUser(t *testing.T) {
	t.Setenv("ZPR_CHANGE_REVIEW_CONFIG_FILE", "")
	handler, close, err := configuredChangeReview(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", changeReviewPrefix+"capabilities?organization=great-lakes", nil))
	if w.Code != 503 {
		t.Fatal("unconfigured review looks enabled")
	}
	t.Setenv("ZPR_CHANGE_REVIEW_CONFIG_FILE", "/unavailable/config.json")
	if _, _, err := configuredChangeReview(nil, nil); err == nil {
		t.Fatal("review enabled without named-user auth")
	}
}

func TestChangeReviewConfigurationDefaultsAndStrictOptIn(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.json")
	t.Setenv("ZPR_CHANGE_REVIEW_CONFIG_FILE", path)
	config := map[string]any{"version": 1, "database": filepath.Join(directory, "private", "reviews.sqlite")}
	writeConfig := func() {
		t.Helper()
		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, allow := range []bool{false, true} {
		if allow {
			config["allow_self_approval"] = true
		}
		writeConfig()
		handler, close, err := configuredChangeReview(&operatorauth.Auth{}, http.NotFoundHandler())
		if err != nil {
			t.Fatal(err)
		}
		if api := handler.(*changeReviewAPI); api.allowSelf != allow {
			close()
			t.Fatal("single-operator opt-in default changed")
		}
		close()
	}
	config["allow_self_approval"] = "true"
	writeConfig()
	if _, _, err := configuredChangeReview(&operatorauth.Auth{}, nil); err == nil {
		t.Fatal("string self-approval option accepted")
	}
	config["allow_self_approval"] = false
	config["unexpected"] = true
	writeConfig()
	if _, _, err := configuredChangeReview(&operatorauth.Auth{}, nil); err == nil {
		t.Fatal("unknown configuration option accepted")
	}
}

func TestChangeReviewLauncherPreflightKeepsOperatorSecretsOutsideDatabaseMount(t *testing.T) {
	source, err := os.ReadFile("../../scripts/dashboard-stack.sh")
	if err != nil {
		t.Fatal(err)
	}
	_, helper, found := strings.Cut(string(source), "configure_change_review() {")
	if !found {
		t.Fatal("launcher preflight is missing")
	}
	helper, _, found = strings.Cut(helper, "\nstart_control_room() {")
	if !found {
		t.Fatal("launcher preflight boundary is missing")
	}
	directory := t.TempDir()
	operator := filepath.Join(directory, "operator")
	if err := os.Mkdir(operator, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(operator, "stack.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.json")
	scriptPath := filepath.Join(directory, "preflight.sh")
	script := "set -eu\nOPERATOR_DIR=$1\nZPR_CHANGE_REVIEW_CONFIG_FILE=$2\nconfigure_change_review() {" + helper + "\nconfigure_change_review\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		database string
		allow    any
		valid    bool
	}{
		{"dedicated directory", filepath.Join(directory, "private", "reviews.sqlite"), false, true},
		{"explicit single operator", filepath.Join(directory, "private", "reviews.sqlite"), true, true},
		{"operator secrets directory", filepath.Join(operator, "reviews.sqlite"), false, false},
		{"public ancestor", filepath.Join(directory, "reviews.sqlite"), false, false},
		{"relative database", "reviews.sqlite", false, false},
		{"non-boolean approval", filepath.Join(directory, "private", "reviews.sqlite"), "false", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{"version": 1, "database": test.database, "allow_self_approval": test.allow})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(configPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command("sh", scriptPath, operator, configPath).CombinedOutput()
			if (err == nil) != test.valid {
				t.Fatalf("preflight validity: %v, output: %s", err, out)
			}
		})
	}
}
