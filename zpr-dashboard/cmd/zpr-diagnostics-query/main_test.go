package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQueryProxyRestrictsRequestsAndUsesSeparateCredentials(t *testing.T) {
	calls := 0
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		user, password, _ := r.BasicAuth()
		if user != "store-user" || password != "store-password" {
			t.Error("upstream did not receive its isolated credentials")
		}
		signal := r.URL.Query().Get("type")
		if r.URL.Path != "/api/tenant/_search" || (signal != "metrics" && signal != "logs") {
			t.Errorf("wrong native provider endpoint: %s", r.URL)
		}
		var query searchRequest
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			t.Fatal(err)
		}
		if query.Query.StartTime != 1000000 || query.Query.EndTime != 2000000 {
			t.Errorf("native timestamp adaptation failed: %+v", query)
		}
		if signal == "metrics" && (!strings.Contains(query.Query.SQL, `FROM "uptime"`) || !strings.Contains(query.Query.SQL, "__name__ AS name")) {
			t.Errorf("native metrics/timestamp adaptation failed: %+v", query)
		}
		if signal == "logs" && !strings.Contains(query.Query.SQL, `FROM "signals"`) {
			t.Errorf("log stream changed: %+v", query)
		}
		_, _ = w.Write([]byte(`{"hits":[]}`))
	}))
	defer store.Close()
	client := store.Client()
	originalTransport := client.Transport
	client.Transport = rewriteTransport{originalTransport, store.URL}
	handler, err := queryHandler(configuration{Upstream: "http://zpr-diagnostics-store:5080",
		Tenant: "tenant", Organization: "great-lakes", Streams: []string{"signals"},
		MetricStreams: []string{"uptime"},
		QueryUsername: "query", QueryPassword: "query-password", UpstreamUsername: "store-user", UpstreamPassword: "store-password"}, client)
	if err != nil {
		t.Fatal(err)
	}
	sql := `SELECT * FROM "signals" WHERE zpr_organization_id = 'great-lakes' AND service_name = 'node' AND service_instance_id = 'node0' AND name IS NOT NULL ORDER BY _timestamp DESC`
	requestBody := func(sql string, size int) string {
		query := searchRequest{}
		query.Query.SQL, query.Query.Size = sql, size
		query.Query.StartTime, query.Query.EndTime = 1000, 2000
		data, _ := json.Marshal(query)
		return string(data)
	}
	for _, item := range []struct {
		name, method, path, body, password string
		status                             int
	}{
		{"query", "POST", "/api/tenant/signals/_search", requestBody(sql, 50), "query-password", 200},
		{"node-metric-limit", "POST", "/api/tenant/signals/_search", requestBody(sql, 1024), "query-password", 200},
		{"logs", "POST", "/api/tenant/signals/_search", requestBody(strings.Replace(sql, "name IS", "body IS", 1), 50), "query-password", 200},
		{"unauthenticated", "POST", "/api/tenant/signals/_search", requestBody(sql, 50), "", 401},
		{"wrong-password", "POST", "/api/tenant/signals/_search", requestBody(sql, 50), "wrong", 401},
		{"ingest", "POST", "/api/tenant/signals/_json", `{}`, "query-password", 403},
		{"users", "POST", "/api/tenant/users", `{}`, "query-password", 403},
		{"delete", "DELETE", "/api/tenant/signals/_search", `{}`, "query-password", 403},
		{"other-tenant", "POST", "/api/other/signals/_search", requestBody(sql, 50), "query-password", 403},
		{"query-parameters", "POST", "/api/tenant/signals/_search?type=metrics", requestBody(sql, 50), "query-password", 403},
		{"encoded-path", "POST", "/api/tenant/%73ignals/_search", requestBody(sql, 50), "query-password", 403},
		{"unscoped", "POST", "/api/tenant/signals/_search", requestBody(`SELECT * FROM "signals"`, 50), "query-password", 400},
		{"cross-org", "POST", "/api/tenant/signals/_search", requestBody(strings.Replace(sql, "great-lakes", "other", 1), 50), "query-password", 400},
		{"extra-statement", "POST", "/api/tenant/signals/_search", requestBody(sql+"; DROP TABLE signals", 50), "query-password", 400},
		{"large-limit", "POST", "/api/tenant/signals/_search", requestBody(sql, 1025), "query-password", 400},
		{"extra-fields", "POST", "/api/tenant/signals/_search", `{"query":{},"password":"secret"}`, "query-password", 400},
		{"trailing-json", "POST", "/api/tenant/signals/_search", requestBody(sql, 50) + `{}`, "query-password", 400},
		{"body-limit", "POST", "/api/tenant/signals/_search", strings.Repeat(" ", 16<<10) + requestBody(sql, 50), "query-password", 400},
		{"long-range", "POST", "/api/tenant/signals/_search", strings.Replace(requestBody(sql, 50), `"end_time":2000`, `"end_time":86402000`, 1), "query-password", 400},
	} {
		t.Run(item.name, func(t *testing.T) {
			request := httptest.NewRequest(item.method, item.path, strings.NewReader(item.body))
			request.SetBasicAuth("query", item.password)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != item.status {
				t.Fatalf("status=%d, want %d: %s", response.Code, item.status, response.Body)
			}
			if strings.Contains(response.Body.String(), "store-password") {
				t.Fatal("upstream credentials leaked")
			}
		})
	}
	if calls != 3 {
		t.Fatalf("forbidden requests reached upstream: calls=%d", calls)
	}
}

type rewriteTransport struct {
	base http.RoundTripper
	url  string
}

func TestQueryProxyBoundsAndSanitizesProviderResponses(t *testing.T) {
	for _, item := range []struct {
		name, body string
		status     int
	}{
		{"provider-error", "private-upstream-error", http.StatusUnauthorized},
		{"invalid-json", "private-upstream-error", http.StatusOK},
		{"response-limit", strings.Repeat("x", maxBody+1), http.StatusOK},
	} {
		t.Run(item.name, func(t *testing.T) {
			store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(item.status)
				_, _ = w.Write([]byte(item.body))
			}))
			defer store.Close()
			client := store.Client()
			client.Transport = rewriteTransport{client.Transport, store.URL}
			handler, err := queryHandler(configuration{
				Upstream: "http://zpr-diagnostics-store:5080", Tenant: "tenant", Organization: "great-lakes",
				Streams: []string{"signals"}, MetricStreams: []string{"uptime"},
				QueryUsername: "query", QueryPassword: "query-password",
				UpstreamUsername: "store-user", UpstreamPassword: "store-password",
			}, client)
			if err != nil {
				t.Fatal(err)
			}
			query := searchRequest{}
			query.Query.SQL = `SELECT * FROM "signals" WHERE zpr_organization_id = 'great-lakes' AND service_name = 'node' AND service_instance_id = 'node0' AND body IS NOT NULL ORDER BY _timestamp DESC`
			query.Query.StartTime, query.Query.EndTime, query.Query.Size = 1000, 2000, 50
			body, err := json.Marshal(query)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/tenant/signals/_search", strings.NewReader(string(body)))
			request.SetBasicAuth("query", "query-password")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), "private-upstream-error") {
				t.Fatalf("provider response not bounded/sanitized: status=%d body=%s", response.Code, response.Body)
			}
		})
	}
}

func (transport rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	redirect := request.Clone(request.Context())
	parsed := httptest.NewRequest(http.MethodGet, transport.url, nil).URL
	redirect.URL.Scheme, redirect.URL.Host = parsed.Scheme, parsed.Host
	return transport.base.RoundTrip(redirect)
}
