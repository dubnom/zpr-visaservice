package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestControlServiceProxiesPolicyOverTLSAndKeepsAssistantInControlLayer(t *testing.T) {
	var sawClientCertificate bool
	var sawBrowserSecrets bool
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawClientCertificate = r.TLS != nil && len(r.TLS.PeerCertificates) > 0
		sawBrowserSecrets = r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != ""
		if r.URL.Path == "/api/policy" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"configured":true,"categories":[],"records":[],"compiler_ready":true,"assistant_ready":false}`))
			return
		}
		if r.URL.Path == "/api/policy/categories" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"created"}`))
			return
		}
		http.NotFound(w, r)
	}))
	upstream.TLS = &tls.Config{ClientAuth: tls.RequestClientCert}
	upstream.StartTLS()
	defer upstream.Close()

	certificate := upstream.TLS.Certificates[0]
	certFile := filepath.Join(t.TempDir(), "client.crt")
	keyFile := filepath.Join(t.TempDir(), "client.key")
	caFile := filepath.Join(t.TempDir(), "service-ca.crt")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]})
	keyDER, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_POLICY_SERVICE_URL", upstream.URL)
	t.Setenv("ZPR_POLICY_CLIENT_CERT_FILE", certFile)
	t.Setenv("ZPR_POLICY_CLIENT_KEY_FILE", keyFile)
	t.Setenv("ZPR_POLICY_SERVICE_CA_FILE", caFile)
	t.Setenv("ANTHROPIC_API_KEY", "configured-for-control-service")
	proxy, message := newPolicyServiceProxy()
	if proxy == nil {
		t.Fatalf("newPolicyServiceProxy: %s", message)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/policy", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("Origin", "http://127.0.0.1:8787")
	request.Header.Set("Cookie", "session=browser")
	request.Header.Set("Authorization", "browser-token")
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET proxy status=%d body=%s", response.Code, response.Body)
	}
	var status policyStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.AssistantReady {
		t.Fatal("control service did not augment status with its optional assistant state")
	}
	if !sawClientCertificate {
		t.Fatal("Policy Repository did not receive the Control Service client certificate")
	}
	if sawBrowserSecrets {
		t.Fatal("browser origin/cookie/authorization headers were forwarded to Policy Repository")
	}
	post := httptest.NewRecorder()
	create := httptest.NewRequest(http.MethodPost, "/api/policy/categories", strings.NewReader(`{"name":"Test"}`))
	proxy.ServeHTTP(post, create)
	if post.Code != http.StatusCreated || !strings.Contains(post.Body.String(), "created") {
		t.Fatalf("POST proxy status=%d body=%s", post.Code, post.Body)
	}
}

func TestNorthstarDemoCatalogImportsIdempotently(t *testing.T) {
	databasePath := filepath.Join(privatePolicyTestDir(t), "northstar.db")
	store, err := openSQLitePolicyRepository(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	catalogPath, err := filepath.Abs("examples/northstar/demo-policy-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_POLICY_DEMO_CATALOG_FILE", catalogPath)
	workspace := &policyWorkspace{
		store: store,
		checkSource: func(context.Context, string) policyCheckResponse {
			return policyCheckResponse{Valid: true, Diagnostics: "stub compiler accepted source"}
		},
	}
	if err := seedDemoPolicyCatalog(context.Background(), workspace); err != nil {
		t.Fatal(err)
	}
	if err := seedDemoPolicyCatalog(context.Background(), workspace); err != nil {
		t.Fatal(err)
	}
	categories, records, err := store.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(categories) != 15 || len(records) != 13 {
		t.Fatalf("catalog contains %d categories and %d records, want 15 and 13", len(categories), len(records))
	}
}

func TestNorthstarDemoPoliciesPassConfiguredZPLC(t *testing.T) {
	configPath := os.Getenv("ZPR_POLICY_CONFIG_FILE")
	if configPath == "" {
		t.Skip("set ZPR_POLICY_CONFIG_FILE to run the Northstar ZPLC fixture check")
	}
	compiler := os.Getenv("ZPR_ZPLC_BIN")
	if compiler == "" {
		var err error
		compiler, err = exec.LookPath("zplc")
		if err != nil {
			t.Skip("zplc is not on PATH; set ZPR_ZPLC_BIN to run the Northstar fixture check")
		}
	}
	configPath, err := filepath.EvalSymlinks(configPath)
	if err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join("examples", "northstar", "demo-policy-catalog.json")
	contents, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	var catalog demoPolicyCatalog
	if err := json.Unmarshal(contents, &catalog); err != nil {
		t.Fatal(err)
	}
	workspace := &policyWorkspace{configPath: configPath, compiler: compiler}
	for _, record := range catalog.Records {
		if record.Kind != "policy" {
			continue
		}
		result := workspace.check(context.Background(), record.Content)
		if !result.Valid {
			t.Errorf("policy %q did not pass ZPLC: %s", record.Name, result.Diagnostics)
		}
	}
}

func TestPolicyCategoriesVersionHistoryAndPersistence(t *testing.T) {
	databasePath := filepath.Join(privatePolicyTestDir(t), "policy.db")
	store, err := openSQLitePolicyRepository(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	workspace := &policyWorkspace{
		store: store, compiler: "test-compiler",
		checkSource: func(_ context.Context, source string) policyCheckResponse {
			if strings.Contains(source, "invalid") {
				return policyCheckResponse{Diagnostics: "invalid policy"}
			}
			return policyCheckResponse{Valid: true, Diagnostics: "parse-only check passed"}
		},
	}
	app := &application{policy: workspace, assistant: &claudeAssistant{}}

	rootResponse := httptest.NewRecorder()
	app.handleCreatePolicyCategory(rootResponse, localPolicyRequest(http.MethodPost, "/api/policy/categories", `{"name":"Network"}`))
	if rootResponse.Code != http.StatusCreated {
		t.Fatalf("root category status = %d, body %s", rootResponse.Code, rootResponse.Body)
	}
	var root policyCategory
	if err := json.Unmarshal(rootResponse.Body.Bytes(), &root); err != nil {
		t.Fatal(err)
	}
	childRequest := localPolicyRequest(http.MethodPost, "/api/policy/categories", `{"parent_id":"`+root.ID+`","name":"Lab"}`)
	childResponse := httptest.NewRecorder()
	app.handleCreatePolicyCategory(childResponse, childRequest)
	var child policyCategory
	if childResponse.Code != http.StatusCreated || json.Unmarshal(childResponse.Body.Bytes(), &child) != nil || child.Path != "Network/Lab" {
		t.Fatalf("child category = %d %s", childResponse.Code, childResponse.Body)
	}

	createRecord := `{"category_id":"` + child.ID + `","name":"allow-lab","kind":"policy","content_type":"text/vnd.zpr.zpl","metadata":{"language":"zpl"},"content":"allow team to access service.","summary":"Initial rule"}`
	created := httptest.NewRecorder()
	app.handleCreatePolicyRecord(created, localPolicyRequest(http.MethodPost, "/api/policy/records", createRecord))
	if created.Code != http.StatusCreated {
		t.Fatalf("create record status = %d, body %s", created.Code, created.Body)
	}
	var record policyRecord
	if err := json.Unmarshal(created.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.CurrentRevision != 1 || record.Kind != "policy" {
		t.Fatalf("created record = %+v", record)
	}

	invalidCheck := httptest.NewRecorder()
	app.handleCheckPolicy(invalidCheck, localPolicyRequest(http.MethodPost, "/api/policy/check", `{"source":"invalid policy"}`))
	if invalidCheck.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid check status = %d, want 422", invalidCheck.Code)
	}

	appendRequest := func(expected int, content string) *http.Request {
		body, _ := json.Marshal(policySaveRequest{Content: content, ExpectedRevision: expected, Summary: "Update rule"})
		request := localPolicyRequest(http.MethodPost, "/api/policy/records/"+record.ID+"/revisions", string(body))
		request.SetPathValue("id", record.ID)
		return request
	}
	updatedContent := "allow team to access another-service."
	updated := httptest.NewRecorder()
	app.handlePolicyRecordRevisions(updated, appendRequest(1, updatedContent))
	if updated.Code != http.StatusCreated {
		t.Fatalf("append revision status = %d, body %s", updated.Code, updated.Body)
	}
	stale := httptest.NewRecorder()
	app.handlePolicyRecordRevisions(stale, appendRequest(1, "allow everyone."))
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale revision status = %d, want 409", stale.Code)
	}

	revisionList := httptest.NewRecorder()
	listRequest := localPolicyRequest(http.MethodGet, "/api/policy/records/"+record.ID+"/revisions", "")
	listRequest.SetPathValue("id", record.ID)
	app.handlePolicyRecordRevisions(revisionList, listRequest)
	var revisions []policyRevisionSummary
	if err := json.Unmarshal(revisionList.Body.Bytes(), &revisions); err != nil || len(revisions) != 2 || revisions[0].Number != 2 || revisions[1].Number != 1 {
		t.Fatalf("revision history = %+v, err = %v", revisions, err)
	}
	firstRevision, err := store.GetRevision(context.Background(), record.ID, 1)
	if err != nil || firstRevision.Content != "allow team to access service." {
		t.Fatalf("first revision changed: %+v, err = %v", firstRevision, err)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openSQLitePolicyRepository(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	reopened, err := store.GetRecord(context.Background(), record.ID)
	if err != nil || reopened.CurrentRevision != 2 || reopened.Content != updatedContent {
		t.Fatalf("persisted record = %+v, err = %v", reopened, err)
	}
}

func TestPolicyEditorRejectsNonLoopbackAndCrossOriginRequests(t *testing.T) {
	app := &application{}
	remote := httptest.NewRequest(http.MethodGet, "/api/policy", nil)
	remote.RemoteAddr = "192.0.2.10:4321"
	response := httptest.NewRecorder()
	app.handleGetPolicy(response, remote)
	if response.Code != http.StatusForbidden {
		t.Fatalf("remote request status = %d, want 403", response.Code)
	}

	crossOrigin := localPolicyRequest(http.MethodGet, "/api/policy", "")
	crossOrigin.Header.Set("Origin", "http://evil.example")
	response = httptest.NewRecorder()
	app.handleGetPolicy(response, crossOrigin)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin request status = %d, want 403", response.Code)
	}

	rebinding := localPolicyRequest(http.MethodGet, "/api/policy", "")
	rebinding.Host = "attacker.example"
	rebinding.Header.Set("Origin", "http://attacker.example")
	response = httptest.NewRecorder()
	app.handleGetPolicy(response, rebinding)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-loopback host status = %d, want 403", response.Code)
	}
}

func TestClaudeAssistantKeepsCredentialsServerSide(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
			t.Errorf("upstream request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-secret" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("missing required Anthropic headers")
		}
		var body struct {
			Model    string `json:"model"`
			System   string `json:"system"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode Claude request: %v", err)
		}
		if body.Model != "test-model" || !strings.Contains(body.System, "allow team.") || len(body.Messages) != 1 || body.Messages[0].Content != "Explain this rule" {
			t.Errorf("unexpected Claude request body: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"It grants access."}]}`))
	}))
	defer server.Close()

	assistant := &claudeAssistant{apiKey: "test-secret", model: "test-model", url: server.URL + "/v1/messages", http: server.Client()}
	answer, err := assistant.reply(context.Background(), "allow team.", []assistantMessage{{Role: "user", Content: "Explain this rule"}})
	if err != nil || answer != "It grants access." {
		t.Fatalf("reply = %q, err = %v", answer, err)
	}
}

func TestClaudeAssistantDoesNotFollowRedirects(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-secret")
	assistant := newClaudeAssistant()
	if assistant == nil || assistant.http.CheckRedirect == nil {
		t.Fatal("Claude client must disable redirects")
	}
}

func localPolicyRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:4321"
	request.Host = "127.0.0.1:8787"
	request.Header.Set("Origin", "http://127.0.0.1:8787")
	request.Header.Set("Content-Type", "application/json")
	return request
}
