package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/controlservice/inventory"
)

func TestCatalogSelectionMappingsAndWireShapes(t *testing.T) {
	data := inventory.Snapshot{
		Actors: []inventory.Actor{{CN: "adapter"}, {CN: "z", Node: true}, {CN: "a", Node: true}},
		Services: []inventory.Service{
			{}, {Name: "app", ActorCN: "service-actor", Address: "fd00::1", Kind: "Application"},
			{Name: "app", ActorCN: "duplicate", Kind: "Application"},
		},
		Trusted: []inventory.TrustedSource{{Name: "directory", Provider: "file"}, {Name: "ldap", ActorCN: "ldap-actor", Provider: "rest/1"}},
	}
	mappings := map[string]OTelIdentity{
		"node:a":      {ServiceName: "production-node"},
		"service:app": {InstanceID: "instance"},
	}
	sources := Sources(data, mappings, "")
	if len(sources) != 6 || sources[0].Kind != "Required service · Application" || sources[2].Kind != "Trusted service · file" || sources[4].ID != "node:a" {
		t.Fatalf("catalog classes/order/non-dedup changed: %+v", sources)
	}
	for _, source := range sources {
		switch source.ID {
		case "node:a":
			if source.ServiceName != "production-node" || source.InstanceID != "a" {
				t.Fatalf("partial node mapping changed: %+v", source)
			}
		case "node:z":
			if source.ServiceName != "z" || source.InstanceID != "z" {
				t.Fatalf("node fallback is identity, not invented zpr-core: %+v", source)
			}
		case "service:app":
			if source.ServiceName != source.Identity || source.InstanceID != "instance" {
				t.Fatalf("partial service mapping changed: %+v", source)
			}
		case "trusted:directory":
			if source.Identity != "directory" || source.ServiceName != "directory" {
				t.Fatalf("trusted fallback changed: %+v", source)
			}
		}
		encoded, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		if string(fields["logs"]) != "[]" || string(fields["metrics"]) != "[]" || source.State != "unavailable" || fields["last_updated"] != nil || fields["error"] != nil {
			t.Fatalf("initial source wire shape changed: %s", encoded)
		}
	}
	for i := 0; i < 120; i++ {
		data.Actors = append(data.Actors, inventory.Actor{CN: fmt.Sprintf("n-%03d", i), Node: true})
	}
	if got := Sources(data, nil, ""); len(got) != 100 {
		t.Fatalf("aggregate cap=%d", len(got))
	}
	if got := Sources(data, nil, "node:z"); len(got) != 1 || got[0].ID != "node:z" {
		t.Fatalf("selected source lost to aggregate cap: %+v", got)
	}
	if got := Sources(data, nil, "node:unknown"); got == nil || len(got) != 0 {
		t.Fatalf("unknown source must be empty, not invented: %+v", got)
	}
}

func TestOpenObserveQueryScopeLimitsAndPartialSignals(t *testing.T) {
	var calls []string
	var mu sync.Mutex
	var failedSignal atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.URL.Path)
		mu.Unlock()
		user, token, ok := r.BasicAuth()
		if !ok || user != "reader" || token != "test-token" || r.Method != http.MethodPost ||
			r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "application/json" {
			t.Error("provider method/credentials/media types changed")
		}
		var query SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			t.Error(err)
			return
		}
		metrics := strings.HasSuffix(r.URL.Path, "/metrics/_search")
		wantLimit, predicate := 2, "body IS NOT NULL"
		if metrics {
			wantLimit, predicate = 1024, "name IS NOT NULL"
		}
		for _, text := range []string{"zpr_organization_id = 'org''id'", "service_name = 'node''name'", "service_instance_id = 'instance''id'", predicate} {
			if !strings.Contains(query.Query.SQL, text) {
				t.Errorf("query scope/escaping lost: %s", query.Query.SQL)
			}
		}
		if query.Query.Size != wantLimit || query.Query.From != 0 || query.Query.EndTime-query.Query.StartTime != time.Hour.Milliseconds() {
			t.Errorf("provider bounds changed: %+v", query.Query)
		}
		if metrics {
			if failedSignal.Load() == 1 {
				http.Error(w, "sensitive response must not be returned", http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(w, `{"hits":[{"_timestamp":1700000000000,"name":"packets","value":"18446744073709551615"},{"_timestamp":1700000001000,"name":"newer","as_int":"9007199254740993"},{"name":"no value"},{"value":"no name"}]}`)
			return
		}
		if failedSignal.Load() == 2 {
			http.Error(w, "logs unavailable", http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, `{"hits":[{"_timestamp":1700000000000,"body":"old"},{"_timestamp":1700000002000,"body":"authorization: Bearer test-private-value"},{"_timestamp":1700000001000,"body":"middle"}]}`)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	provider := OpenObserve{Client: server.Client(), Endpoint: endpoint, Organization: "tenant", ZPROrganizationID: "org'id",
		LogsStream: "logs", MetricsStream: "metrics", Username: "reader", Token: "test-token"}
	source := Source{Kind: "ZPR node", ServiceName: "node'name", InstanceID: "instance'id"}
	logs, metrics, err := provider.Query(context.Background(), source, 2)
	if err != nil || len(logs) != 2 || len(metrics) != 2 || logs[1].Body != "middle" ||
		strings.Contains(logs[0].Body, "test-private-value") || metrics[0].Name != "newer" || metrics[1].Value != "18446744073709551615" {
		t.Fatalf("parse/sort/precision/redaction changed: logs=%+v metrics=%+v err=%v", logs, metrics, err)
	}
	mu.Lock()
	gotCalls := slices.Clone(calls)
	mu.Unlock()
	if !slices.Equal(gotCalls, []string{"/api/tenant/logs/_search", "/api/tenant/metrics/_search"}) || endpoint.Path != "" {
		t.Fatalf("sequential query paths or endpoint mutation: %v %+v", gotCalls, endpoint)
	}
	failedSignal.Store(1)
	logs, metrics, err = provider.Query(context.Background(), source, 2)
	if len(logs) != 2 || len(metrics) != 0 || err == nil || err.Error() != "telemetry provider returned HTTP 503" {
		t.Fatalf("partial logs lost or error hidden: logs=%d metrics=%d err=%v", len(logs), len(metrics), err)
	}
	failedSignal.Store(2)
	logs, metrics, err = provider.Query(context.Background(), source, 2)
	if len(logs) != 0 || len(metrics) != 2 || err == nil || err.Error() != "telemetry provider returned HTTP 502" {
		t.Fatalf("partial metrics lost or log error hidden: logs=%d metrics=%d err=%v", len(logs), len(metrics), err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}

func TestOpenObserveResponseBoundsContextAndErrors(t *testing.T) {
	exact := `{"hits":[]}`
	exact += strings.Repeat(" ", MaxQueryBytes-len(exact))
	for _, test := range []struct {
		name, body, want string
		status           int
	}{
		{"exact", exact, "", 200},
		{"over", exact + " ", "telemetry provider response exceeded limits", 200},
		{"bad status", `{"detail":"must not leak"}`, "telemetry provider returned HTTP 401", 401},
		{"bad json", `{`, "telemetry provider returned invalid query data", 200},
		{"trailing json", `{"hits":[]} {}`, "telemetry provider returned invalid query data", 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(test.body)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			endpoint, err := url.Parse("https://telemetry.example")
			if err != nil {
				t.Fatal(err)
			}
			var calls int
			provider := OpenObserve{Endpoint: endpoint, Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.Context() != ctx {
					t.Error("caller context replaced")
				}
				return &http.Response{StatusCode: test.status, Body: body, Header: make(http.Header)}, nil
			})}}
			_, err = provider.Search(ctx, "logs", Source{}, time.Now().Add(-time.Hour), time.Now(), 1, "logs")
			if (err == nil) != (test.want == "") || (err != nil && err.Error() != test.want) || !body.closed || calls != 1 {
				t.Fatalf("response error/close/retry: err=%v closed=%t calls=%d", err, body.closed, calls)
			}
			cancel()
			provider.Client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if !errors.Is(request.Context().Err(), context.Canceled) {
					t.Error("cancellation not forwarded")
				}
				return nil, request.Context().Err()
			})
			if _, err := provider.Search(ctx, "logs", Source{}, time.Time{}, time.Time{}, 1, "logs"); err == nil || err.Error() != "telemetry provider is unavailable" {
				t.Fatalf("existing transport error contract changed: %v", err)
			}
		})
	}
}

func TestSignalParsingTimestampAndAttributeBounds(t *testing.T) {
	want := time.Unix(1700000000, 0).UTC()
	for _, value := range []any{"2023-11-14T22:13:20Z", "1700000000", float64(1700000000000), "1700000000000000", "1700000000000000000"} {
		if got := hitTime(map[string]any{"_timestamp": value}); !got.Equal(want) {
			t.Fatalf("timestamp %v = %v, want %v", value, got, want)
		}
	}
	if got := hitTime(map[string]any{"_timestamp": "NaN", "timestamp": "invalid"}); !got.IsZero() {
		t.Fatal("invalid timestamp accepted")
	}
	attributes := safeAttributes(map[string]any{
		"password": "hidden", "private_key": "hidden", "Authorization": "hidden", "api_token": "hidden",
		"long": strings.Repeat("x", 129), "nested": []string{"value"}, "flag": true,
	})
	if len(attributes) != 3 || attributes["long"] != strings.Repeat("x", 128)+"…" || attributes["nested"] != `["value"]` {
		t.Fatalf("attribute filtering/truncation changed: %+v", attributes)
	}
	many := map[string]any{}
	for i := 0; i < 20; i++ {
		many[fmt.Sprint(i)] = i
	}
	if len(safeAttributes(many)) != 8 {
		t.Fatal("attribute count limit changed")
	}
	if Truncate("exact", 5) != "exact" || Truncate("over", 3) != "ove…" {
		t.Fatal("byte truncation contract changed")
	}
}

func TestOTLPExplicitFileIdentityAndBounds(t *testing.T) {
	const payload = `{"resourceMetrics":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"production-node"}},{"key":"service.instance.id","value":{"stringValue":"instance"}}]},"scopeMetrics":[{"metrics":[{"name":"counter","gauge":{"dataPoints":[{"timeUnixNano":"1700000000000000000","asInt":"18446744073709551615"}]},"sum":{"dataPoints":[{"timeUnixNano":"1700000000000000001","asInt":"2"}]}}]}]}]}`
	path := filepath.Join(t.TempDir(), "metrics.json")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	source := Source{Identity: "node", ServiceName: "production-node", InstanceID: "instance"}
	provider := OTLPFiles{"node": path}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	write(payload + strings.Repeat(" ", (64<<10)-len(payload)))
	logs, metrics, err := provider.Query(ctx, source, 1)
	if err != nil || logs != nil || len(metrics) != 2 || metrics[0].Value != "18446744073709551615" || metrics[1].Value != "2" ||
		metrics[1].Timestamp.UnixNano() != 1700000000000000001 {
		t.Fatalf("explicit file/gauge+sum/string precision contract changed: %+v %+v %v", logs, metrics, err)
	}
	for _, test := range []struct {
		name, body, want string
		source           Source
	}{
		{"over", payload + strings.Repeat(" ", (64<<10)+1-len(payload)), "invalid or oversized node OTLP metrics", source},
		{"bad timestamp", strings.ReplaceAll(payload, "1700000000000000000", "invalid"), "invalid node metric timestamp", source},
		{"wrong instance", payload, "node OTLP resource identity or metrics missing", Source{Identity: "node", ServiceName: "production-node", InstanceID: "other"}},
		{"wrong service", payload, "node OTLP resource identity or metrics missing", Source{Identity: "node", ServiceName: "other", InstanceID: "instance"}},
		{"missing mapping", payload, "no operator OTLP file for node other", Source{Identity: "other"}},
		{"trailing", payload + `{}`, "invalid or oversized node OTLP metrics", source},
	} {
		t.Run(test.name, func(t *testing.T) {
			write(test.body)
			_, _, err := provider.Query(context.Background(), test.source, 1)
			if err == nil || err.Error() != test.want {
				t.Fatalf("file error=%v want %q", err, test.want)
			}
		})
	}
	write(payload)
	if _, metrics, err := provider.Query(context.Background(), source, 1); err != nil || len(metrics) != 2 {
		t.Fatalf("file must be reread after replacement: %v", err)
	}
	if _, _, err := (OTLPFiles{"node": path + ".absent"}).Query(context.Background(), source, 1); err == nil || err.Error() != "node OTLP metrics file unavailable" {
		t.Fatalf("file availability error changed: %v", err)
	}
}
