package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

func TestConfiguredPolicyImportRefreshesWithoutOverwritingEdits(t *testing.T) {
	directory := privatePolicyTestDir(t)
	store, err := openSQLitePolicyRepository(filepath.Join(directory, "policies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := t.Context()
	category, err := store.CreateCategory(ctx, nil, "Operator")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := store.CreateRecord(ctx, category.ID, "Keep my edits", "policy", "text/vnd.zpr.zpl", json.RawMessage(`{}`), "operator content", "operator", "Local edit")
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(directory, "simulator.zpl")
	if err := os.WriteFile(sourcePath, []byte("first source"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_POLICY_SOURCE_FILE", sourcePath)
	t.Setenv("ZPR_POLICY_SEED_CATEGORY", "Simulator/Runtime")
	t.Setenv("ZPR_POLICY_SEED_NAME", "Simulator runtime policy")
	workspace := &policyWorkspace{store: store}
	for attempt := 0; attempt < 2; attempt++ {
		if err := seedPolicyDatabase(ctx, workspace); err != nil {
			t.Fatal(err)
		}
	}
	_, records, err := store.Catalog(ctx)
	if err != nil || len(records) != 2 {
		t.Fatalf("catalog: records=%d error=%v", len(records), err)
	}
	var imported policyRecord
	for _, record := range records {
		if record.Name == "Simulator runtime policy" {
			imported = record
		}
	}
	if imported.CurrentRevision != 1 {
		t.Fatalf("initial import revision = %d", imported.CurrentRevision)
	}
	if err := os.WriteFile(sourcePath, []byte("updated simulator source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := seedPolicyDatabase(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	refreshed, err := store.GetRecord(ctx, imported.ID)
	if err != nil || refreshed.CurrentRevision != 2 || refreshed.Content != "updated simulator source" {
		t.Fatalf("source was not refreshed: %+v %v", refreshed, err)
	}
	if _, err := store.AppendRevision(ctx, imported.ID, 2, "my runtime edit", "operator", "Manual edit"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("third simulator source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := seedPolicyDatabase(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	preserved, err := store.GetRecord(ctx, imported.ID)
	if err != nil || preserved.CurrentRevision != 3 || preserved.Content != "my runtime edit" {
		t.Fatalf("operator revision changed: %+v %v", preserved, err)
	}
	untouched, err := store.GetRecord(ctx, operator.ID)
	if err != nil || untouched.Content != "operator content" {
		t.Fatalf("unrelated record changed: %+v %v", untouched, err)
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
	if len(categories) != 10 || len(records) != 9 {
		t.Fatalf("catalog contains %d categories and %d records, want 10 and 9", len(categories), len(records))
	}
	for _, category := range categories {
		if strings.Contains(category.Path, "/Services") {
			t.Fatalf("obsolete service category remains: %s", category.Path)
		}
	}
	serviceCount := 0
	for _, record := range records {
		stored, err := store.GetRecord(context.Background(), record.ID)
		if err != nil {
			t.Fatal(err)
		}
		serviceCount += strings.Count(stored.Content, " as json {")
	}
	if serviceCount != 10 {
		t.Fatalf("policy records contain %d service definitions, want 10", serviceCount)
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
	invalidCreate := `{"category_id":"` + child.ID + `","name":"draft-with-errors","kind":"policy","content_type":"text/vnd.zpr.zpl","metadata":{"language":"zpl"},"content":"invalid policy","summary":"Preserve compiler errors"}`
	invalidCreated := httptest.NewRecorder()
	app.handleCreatePolicyRecord(invalidCreated, localPolicyRequest(http.MethodPost, "/api/policy/records", invalidCreate))
	if invalidCreated.Code != http.StatusCreated {
		t.Fatalf("invalid policy draft status = %d, body %s", invalidCreated.Code, invalidCreated.Body)
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
	invalidUpdated := httptest.NewRecorder()
	app.handlePolicyRecordRevisions(invalidUpdated, appendRequest(2, "invalid policy"))
	if invalidUpdated.Code != http.StatusCreated {
		t.Fatalf("invalid policy revision status = %d, body %s", invalidUpdated.Code, invalidUpdated.Body)
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
	if err := json.Unmarshal(revisionList.Body.Bytes(), &revisions); err != nil || len(revisions) != 3 || revisions[0].Number != 3 || revisions[1].Number != 2 || revisions[2].Number != 1 {
		t.Fatalf("revision history = %+v, err = %v", revisions, err)
	}
	latestRevision, err := store.GetRevision(context.Background(), record.ID, 3)
	if err != nil || latestRevision.Content != "invalid policy" {
		t.Fatalf("invalid revision was not persisted: %+v, err = %v", latestRevision, err)
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
	if err != nil || reopened.CurrentRevision != 3 || reopened.Content != "invalid policy" {
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

	t.Setenv("ZPR_CONTROL_ROOM_PROXY_IP", "172.17.0.1")
	containerRequest := httptest.NewRequest(http.MethodGet, "/api/policy", nil)
	containerRequest.RemoteAddr = "172.17.0.1:4321"
	containerRequest.Host = "127.0.0.1:8787"
	response = httptest.NewRecorder()
	if !localEditorRequest(response, containerRequest) || response.Code != http.StatusOK {
		t.Fatalf("configured Docker gateway request status = %d, want 200", response.Code)
	}

	untrustedDockerRequest := httptest.NewRequest(http.MethodGet, "/api/policy", nil)
	untrustedDockerRequest.RemoteAddr = "172.17.0.2:4321"
	untrustedDockerRequest.Host = "127.0.0.1:8787"
	untrustedDockerRequest.Header.Set("X-Forwarded-For", "127.0.0.1")
	response = httptest.NewRecorder()
	if localEditorRequest(response, untrustedDockerRequest) || response.Code != http.StatusForbidden {
		t.Fatalf("untrusted Docker source status = %d, want 403", response.Code)
	}

	badHostRequest := httptest.NewRequest(http.MethodGet, "/api/policy", nil)
	badHostRequest.RemoteAddr = "172.17.0.1:4321"
	badHostRequest.Host = "attacker.example"
	response = httptest.NewRecorder()
	if localEditorRequest(response, badHostRequest) || response.Code != http.StatusForbidden {
		t.Fatalf("trusted gateway with non-loopback Host status = %d, want 403", response.Code)
	}
}

func TestAssertionPolicyRecordLifecycleAndProtectsOrganizationSettings(t *testing.T) {
	store, err := openSQLitePolicyRepository(filepath.Join(privatePolicyTestDir(t), "assertion-records.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	category, err := store.CreateCategory(ctx, nil, "Policies")
	if err != nil {
		t.Fatal(err)
	}
	app := &application{policy: &policyWorkspace{store: store}}
	content := `group "Operators" members >= 2;`
	createBody, _ := json.Marshal(policyRecordRequest{
		CategoryID: category.ID, Name: "Operator assertions", Kind: organizationAssertionsKind,
		ContentType: assertionRecordSourceContentType, Metadata: json.RawMessage(`{"language":"assertions"}`), Content: content,
	})
	created := httptest.NewRecorder()
	app.handleCreatePolicyRecord(created, localPolicyRequest(http.MethodPost, "/api/policy/records", string(createBody)))
	if created.Code != http.StatusCreated {
		t.Fatalf("create assertion record status=%d body=%s", created.Code, created.Body)
	}
	var record policyRecord
	if err := json.Unmarshal(created.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Kind != organizationAssertionsKind || record.CurrentRevision != 1 || record.Content != content {
		t.Fatalf("created assertion record = %+v", record)
	}

	duplicateBody := `{"category_id":"` + category.ID + `","name":"Operator assertions copy"}`
	duplicateRequest := localPolicyRequest(http.MethodPost, "/api/policy/records/"+record.ID+"/duplicate", duplicateBody)
	duplicateRequest.SetPathValue("id", record.ID)
	duplicateResponse := httptest.NewRecorder()
	app.handleDuplicatePolicyRecord(duplicateResponse, duplicateRequest)
	if duplicateResponse.Code != http.StatusCreated {
		t.Fatalf("duplicate assertion record status=%d body=%s", duplicateResponse.Code, duplicateResponse.Body)
	}
	var duplicate policyRecord
	if err := json.Unmarshal(duplicateResponse.Body.Bytes(), &duplicate); err != nil || duplicate.Content != content || duplicate.ID == record.ID {
		t.Fatalf("duplicate assertion record = %+v, err=%v", duplicate, err)
	}

	archiveBody, _ := json.Marshal(policyRecordArchiveRequest{ExpectedRevision: record.CurrentRevision})
	archiveRequest := localPolicyRequest(http.MethodDelete, "/api/policy/records/"+record.ID, string(archiveBody))
	archiveRequest.SetPathValue("id", record.ID)
	archived := httptest.NewRecorder()
	app.handleArchivePolicyRecord(archived, archiveRequest)
	if archived.Code != http.StatusOK {
		t.Fatalf("archive assertion record status=%d body=%s", archived.Code, archived.Body)
	}
	restoredRequest := localPolicyRequest(http.MethodPost, "/api/policy/records/"+record.ID+"/restore", string(archiveBody))
	restoredRequest.SetPathValue("id", record.ID)
	restored := httptest.NewRecorder()
	app.handleArchivePolicyRecord(restored, restoredRequest)
	if restored.Code != http.StatusOK {
		t.Fatalf("restore assertion record status=%d body=%s", restored.Code, restored.Body)
	}
	restoredRecord, err := store.GetRecord(ctx, record.ID)
	if err != nil || restoredRecord.Archived || restoredRecord.CurrentRevision != 1 {
		t.Fatalf("restored assertion record=%+v err=%v", restoredRecord, err)
	}
	revisions, err := store.ListRevisions(ctx, record.ID)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("assertion revisions=%+v err=%v", revisions, err)
	}

	builtIn, err := store.CreateRecord(ctx, category.ID, organizationAssertionsName, organizationAssertionsKind, "application/vnd.zpr.assertions+json", json.RawMessage(`{"language":"json"}`), `{"source":"","enabled":false,"interval_seconds":60}`, "tester", "Built-in")
	if err != nil {
		t.Fatal(err)
	}
	protectedDuplicate := localPolicyRequest(http.MethodPost, "/api/policy/records/"+builtIn.ID+"/duplicate", `{}`)
	protectedDuplicate.SetPathValue("id", builtIn.ID)
	duplicateRejected := httptest.NewRecorder()
	app.handleDuplicatePolicyRecord(duplicateRejected, protectedDuplicate)
	if duplicateRejected.Code != http.StatusConflict {
		t.Fatalf("built-in duplicate status=%d body=%s", duplicateRejected.Code, duplicateRejected.Body)
	}
	protectedArchiveBody, _ := json.Marshal(policyRecordArchiveRequest{ExpectedRevision: builtIn.CurrentRevision})
	protectedArchive := localPolicyRequest(http.MethodDelete, "/api/policy/records/"+builtIn.ID, string(protectedArchiveBody))
	protectedArchive.SetPathValue("id", builtIn.ID)
	archiveRejected := httptest.NewRecorder()
	app.handleArchivePolicyRecord(archiveRejected, protectedArchive)
	if archiveRejected.Code != http.StatusConflict {
		t.Fatalf("built-in archive status=%d body=%s", archiveRejected.Code, archiveRejected.Body)
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
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			System    string `json:"system"`
			Messages  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode Claude request: %v", err)
		}
		if body.Model != "test-model" || body.MaxTokens != 600 || !strings.Contains(body.System, "allow team.") || !strings.Contains(body.System, "ou -> device.demo.department") || len(body.Messages) != 1 || body.Messages[0].Content != "Explain this rule" {
			t.Errorf("unexpected Claude request body: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"It grants access."}],"usage":{"input_tokens":123,"output_tokens":17}}`))
	}))
	defer server.Close()

	assistant := &claudeAssistant{apiKey: "test-secret", model: "test-model", url: server.URL + "/v1/messages", http: server.Client()}
	attributes := []policyAttribute{{Source: "ou", Attribute: "device.demo.department"}}
	app := &application{assistant: assistant, policy: &policyWorkspace{attributes: attributes}}
	request := localPolicyRequest(http.MethodPost, "/api/policy/assistant", `{"source":"allow team.","messages":[{"role":"user","content":"Explain this rule"}],"model":"test-model","max_tokens":600}`)
	response := httptest.NewRecorder()
	app.handlePolicyAssistant(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("assistant status=%d body=%s", response.Code, response.Body)
	}
	var result assistantReply
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Answer != "It grants access." {
		t.Fatalf("answer = %q", result.Answer)
	}
	if result.InputTokens != 123 || result.OutputTokens != 17 {
		t.Fatalf("assistant usage = %d/%d, want 123/17", result.InputTokens, result.OutputTokens)
	}
}

func TestClaudeAssistantRejectsUnapprovedSettings(t *testing.T) {
	app := &application{assistant: &claudeAssistant{model: "test-model"}, policy: &policyWorkspace{}}
	for _, requestBody := range []string{
		`{"source":"allow users to access services.","messages":[{"role":"user","content":"Check this"}],"model":"unknown","max_tokens":600}`,
		`{"source":"allow users to access services.","messages":[{"role":"user","content":"Check this"}],"model":"test-model","max_tokens":100000}`,
	} {
		response := httptest.NewRecorder()
		app.handlePolicyAssistant(response, localPolicyRequest(http.MethodPost, "/api/policy/assistant", requestBody))
		if response.Code != http.StatusBadRequest {
			t.Errorf("assistant settings status = %d, want 400: %s", response.Code, response.Body.String())
		}
	}
}

func TestLoadPolicyAttributesFromTrustedServiceMappings(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "policy.zplc")
	config := `[trusted_services.demo_ldap]
returns_attributes = ["ou -> device.demo.department", "title -> device.demo.title"]

[trusted_services.demo_rest]
returns_attributes = ["role -> device.demo.role", "ou -> device.demo.department"]
`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	attributes := loadPolicyAttributes(configPath)
	if len(attributes) != 3 {
		t.Fatalf("got %d attributes, want 3: %#v", len(attributes), attributes)
	}
	want := []policyAttribute{
		{Source: "ou", Attribute: "device.demo.department"},
		{Source: "title", Attribute: "device.demo.title"},
		{Source: "role", Attribute: "device.demo.role"},
	}
	for index, attribute := range want {
		if attributes[index] != attribute {
			t.Fatalf("attribute %d = %#v, want %#v", index, attributes[index], attribute)
		}
	}
}

func TestCompileAndStagePolicyRecordStagesSignedCandidateWithoutPush(t *testing.T) {
	root := t.TempDir()
	stageDirectory := filepath.Join(root, "private-stage")
	if err := os.Mkdir(stageDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "runtime.zplc")
	signingKeyPath := filepath.Join(root, "signing-key.pem")
	for path, content := range map[string]string{
		configPath:     "runtime config",
		signingKeyPath: "test signing key",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	compilerPath := filepath.Join(root, "fake-zplc")
	compiler := "#!/bin/sh\nset -eu\noutput=\nsource=\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = \"-o\" ]; then output=$2; shift 2; else source=$1; shift; fi\ndone\nif /usr/bin/grep -q 'invalid policy' \"$source\"; then printf 'ZPLC: invalid policy\\n' >&2; exit 1; fi\nprintf 'signed test candidate' > \"$output\"\n"
	if err := os.WriteFile(compilerPath, []byte(compiler), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace := &policyWorkspace{
		compiler:            compilerPath,
		stageDirectory:      stageDirectory,
		stageConfigPath:     configPath,
		stageSigningKeyPath: signingKeyPath,
	}
	record := policyRecord{ID: "record-1", Name: "Selected policy", Kind: "policy", CurrentRevision: 3, Content: "allow Finance to access InternetGatewayWeb."}
	candidate, err := workspace.compileAndStagePolicyRecord(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.RecordID != record.ID || candidate.RecordRevision != record.CurrentRevision || candidate.BundleSize == 0 {
		t.Fatalf("staged candidate = %+v", candidate)
	}
	sourceHash := sha256.Sum256([]byte(record.Content))
	if candidate.SourceSHA256 != hex.EncodeToString(sourceHash[:]) {
		t.Fatalf("staged source hash = %s", candidate.SourceSHA256)
	}
	bundle, err := os.ReadFile(candidate.bundlePath)
	if err != nil || string(bundle) != "signed test candidate" {
		t.Fatalf("staged bundle = %q, err=%v", bundle, err)
	}
	metadata, err := os.ReadFile(filepath.Join(stageDirectory, "latest.json"))
	if err != nil || !strings.Contains(string(metadata), candidate.BundleSHA256) {
		t.Fatalf("staged metadata = %s, err=%v", metadata, err)
	}
	record.Content = "invalid policy"
	if _, err := workspace.compileAndStagePolicyRecord(context.Background(), record); err == nil || !strings.Contains(err.Error(), "invalid policy") {
		t.Fatalf("invalid policy staging error = %v", err)
	}
}

func TestFilterPolicyAttributeMappingsUsesScannedLDAPNames(t *testing.T) {
	mappings := []policyAttributeMapping{
		{policyAttribute: policyAttribute{Source: "ou", Attribute: "device.demo.department"}, requiresLDAP: true},
		{policyAttribute: policyAttribute{Source: "title", Attribute: "device.demo.title"}, requiresLDAP: true},
		{policyAttribute: policyAttribute{Source: "sub", Attribute: "user.sub"}},
	}
	got := filterPolicyAttributeMappings(mappings, map[string]struct{}{"OU": {}})
	want := []policyAttribute{
		{Source: "ou", Attribute: "device.demo.department"},
		{Source: "sub", Attribute: "user.sub"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered attributes = %#v, want %#v", got, want)
	}
}

func TestParseLDAPAttributeNamesOmitsDNAndValues(t *testing.T) {
	output := []byte("dn: uid=marisol.vega,ou=People,dc=redwood,dc=test\nobjectClass:\nuid:\nou:\ncn:\ntitle:\n")
	got := parseLDAPAttributeNames(output)
	for _, name := range []string{"objectclass", "uid", "ou", "cn", "title"} {
		if _, ok := got[name]; !ok {
			t.Errorf("LDAP attribute %q missing from %#v", name, got)
		}
	}
	if _, ok := got["dn"]; ok {
		t.Fatalf("dn must not be treated as an LDAP attribute: %#v", got)
	}
	if len(got) != 5 {
		t.Fatalf("got %d attribute names, want 5: %#v", len(got), got)
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
