package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/controlservice/adminapi"
)

type readFunc func(context.Context, string, any) error

func (f readFunc) GetJSON(ctx context.Context, path string, target any) error {
	return f(ctx, path, target)
}

func emptyResponse(path string) string {
	switch path {
	case "/admin/stats":
		return `{"stats":{}}`
	case "/admin/network":
		return `{"network":[]}`
	default:
		return `[]`
	}
}

func TestFetchConcurrentTopLevelReads(t *testing.T) {
	started := make(chan string, 7)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	type contextKey struct{}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), contextKey{}, "explicit"), 5*time.Second)
	defer cancel()
	reader := readFunc(func(got context.Context, path string, target any) error {
		if got != ctx || got.Value(contextKey{}) != "explicit" {
			t.Error("caller context was replaced")
		}
		started <- path
		select {
		case <-release:
		case <-got.Done():
			return got.Err()
		}
		return json.Unmarshal([]byte(emptyResponse(path)), target)
	})
	done := make(chan Snapshot, 1)
	go func() { done <- Fetch(ctx, reader, Options{}) }()
	paths := make([]string, 0, 7)
	for range 7 {
		select {
		case path := <-started:
			paths = append(paths, path)
		case <-ctx.Done():
			t.Fatalf("top-level reads were serialized: received %v", paths)
		}
	}
	slices.Sort(paths)
	want := []string{
		"/admin/actors", "/admin/network", "/admin/services", "/admin/stats",
		"/admin/trusted-services", "/admin/visas", "/admin/visas/denies?limit=20",
	}
	if !slices.Equal(paths, want) {
		t.Fatalf("read surface changed: %v", paths)
	}
	unblock()
	select {
	case data := <-done:
		if data.APIStatus != "connected" || len(data.Errors) != 0 {
			t.Fatalf("empty healthy inventory: %+v", data)
		}
	case <-ctx.Done():
		t.Fatal("snapshot did not finish")
	}
}

func TestFetchHealthShapesAndEnrichment(t *testing.T) {
	for _, test := range []struct {
		name, status, trusted, active string
		allFail, optional, detailFail bool
		visaStatus                    int
		enrichError                   bool
	}{
		{name: "empty", status: "connected", trusted: "[]", active: "[]"},
		{name: "optional trusted", status: "connected", trusted: "null", active: "[]", optional: true},
		{name: "disconnected", status: "disconnected", trusted: "null", active: "null", allFail: true, enrichError: true},
		{name: "optional trusted is not reachability", status: "disconnected", trusted: "null", active: "null", allFail: true, optional: true},
		{name: "detail partial", status: "partial", trusted: "[]", active: "[]", detailFail: true},
		{name: "expired missing detail", status: "connected", trusted: "[]", active: "[]", visaStatus: 404},
		{name: "unavailable detail", status: "partial", trusted: "[]", active: "null", visaStatus: 503},
		{name: "enrichment error", status: "partial", trusted: "[]", active: "[]", enrichError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := readFunc(func(_ context.Context, path string, target any) error {
				if test.optional && path == "/admin/trusted-services" {
					return fmt.Errorf("wrapped: %w", adminapi.StatusError{Code: http.StatusNotFound, Status: "404 Not Found"})
				}
				if test.allFail {
					return errors.New("unavailable")
				}
				body := emptyResponse(path)
				if test.detailFail {
					switch path {
					case "/admin/actors":
						body = `[{"cn":"missing"}]`
					case "/admin/services":
						body = `[{"id":"missing"}]`
					case "/admin/actors/missing", "/admin/services/missing":
						return errors.New("detail unavailable")
					}
				}
				if test.visaStatus != 0 {
					switch path {
					case "/admin/visas":
						body = `[{"id":1}]`
					case "/admin/visas/1":
						return adminapi.StatusError{Code: test.visaStatus, Status: fmt.Sprint(test.visaStatus)}
					}
				}
				return json.Unmarshal([]byte(body), target)
			})
			var enriched int
			data := Fetch(context.Background(), reader, Options{Enrich: func(_ context.Context, out *Snapshot) {
				enriched++
				if out.APIStatus != "connected" {
					t.Error("health finalized before enrichment")
				}
				if test.enrichError {
					out.Errors = append(out.Errors, "z enrichment", "a enrichment")
				}
			}})
			if enriched != 1 || data.APIStatus != test.status || !slices.IsSorted(data.Errors) || data.GeneratedAt.IsZero() || data.GeneratedAt.Location() != time.UTC {
				t.Fatalf("health/finalization changed: %+v", data)
			}
			for name, value := range map[string]any{"trusted": data.Trusted, "active": data.ActiveVisas} {
				want := test.trusted
				if name == "active" {
					want = test.active
				}
				encoded, err := json.Marshal(value)
				if err != nil || string(encoded) != want {
					t.Fatalf("%s shape = %s err=%v want %s", name, encoded, err, want)
				}
			}
			if test.detailFail && (len(data.Actors) != 1 || data.Actors[0].CN != "missing" || len(data.Services) != 1 || data.Services[0] != (Service{})) {
				t.Fatalf("detail placeholders changed: actors=%+v services=%+v", data.Actors, data.Services)
			}
			if test.allFail {
				want := 7
				if test.optional {
					want--
				}
				if test.enrichError {
					want += 2
				}
				if len(data.Errors) != want {
					t.Fatalf("missing endpoint/enrichment errors: %v", data.Errors)
				}
			}
			if test.visaStatus != 0 && data.VisaCount != 1 {
				t.Fatal("visa count must reflect listing, not details")
			}
		})
	}
}

func TestFetchNullWireShapes(t *testing.T) {
	data := Fetch(context.Background(), readFunc(func(_ context.Context, path string, target any) error {
		body := "null"
		switch path {
		case "/admin/stats":
			body = `{"stats":null}`
		case "/admin/network":
			body = `{"network":null}`
		}
		return json.Unmarshal([]byte(body), target)
	}), Options{})
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"stats", "network", "recent_denies"} {
		if string(fields[field]) != "null" {
			t.Fatalf("%s must retain successful Admin null: %s", field, encoded)
		}
	}
	for _, field := range []string{"actors", "services", "trusted_sources", "recent_visas", "active_visas", "errors"} {
		if string(fields[field]) != "[]" {
			t.Fatalf("%s must remain a complete empty inventory: %s", field, encoded)
		}
	}
	if data.APIStatus != "connected" || len(fields) != 12 {
		t.Fatalf("snapshot envelope changed: %s", encoded)
	}
}

func TestFetchDetailOrderingAndCompleteActiveInventory(t *testing.T) {
	var actorCalls, serviceCalls atomic.Int32
	reader := readFunc(func(_ context.Context, path string, target any) error {
		body := emptyResponse(path)
		switch path {
		case "/admin/stats":
			body = `{"stats":{}}`
		case "/admin/actors":
			body = `[{"cn":"actor/a"},{"cn":"actor/a"},{"cn":"z"}]`
		case "/admin/actors/actor%2Fa":
			actorCalls.Add(1)
			body = `{"cn":"actor/a","node":true,"node_details":{"latitude":0,"longitude":1,"counters":[{"group":"management","name":"requests","value":"18446744073709551615"}]}}`
		case "/admin/actors/z":
			body = `{"cn":"z"}`
		case "/admin/services":
			body = `[{"id":"service/a"},{"id":"service/a"}]`
		case "/admin/services/service%2Fa":
			serviceCalls.Add(1)
			body = `{"service_name":"directory","actor_cn":"actor/a","service_kind":"Trusted(\"file\")","zpr_addr":"fd00::1","service_endpoints":"file:///operator"}`
		case "/admin/trusted-services":
			body = `[{"name":"remote","health":"unknown"},{"name":"directory","health":"working","last_lookup_ms":42,"last_success_ms":41}]`
		case "/admin/network":
			body = `{"network":[{"link_id":"z"},{"link_id":"a"}]}`
		case "/admin/visas/denies?limit=20":
			body = `[{"count":2},{"count":1}]`
		case "/admin/visas":
			entries := make([]VisaEntry, 14)
			for i := range entries {
				entries[i].ID = int64(i + 1)
			}
			encoded, err := json.Marshal(entries)
			if err != nil {
				return err
			}
			body = string(encoded)
		default:
			if !strings.HasPrefix(path, "/admin/visas/") {
				return fmt.Errorf("unexpected Admin path %q", path)
			}
			var id int64
			if _, err := fmt.Sscanf(path, "/admin/visas/%d", &id); err != nil {
				return err
			}
			if id == 13 {
				return adminapi.StatusError{Code: 404, Status: "404 Not Found"}
			}
			expires := time.Now().Add(time.Hour).Unix()
			if id == 14 {
				expires = 1
			}
			body = fmt.Sprintf(`{"id":%d,"expires":%d}`, id, expires)
		}
		return json.Unmarshal([]byte(body), target)
	})
	data := Fetch(context.Background(), reader, Options{
		TrustedEditorURL: func(name, provider string) string {
			if name == "directory" && provider != "file" {
				t.Errorf("provider metadata lost: %q", provider)
			}
			return "https://operator.example/" + name
		},
		Enrich: func(_ context.Context, out *Snapshot) {
			if len(out.Trusted) != 2 || len(out.Services) != 2 {
				t.Fatal("trusted data was not assembled before enrichment")
			}
			MergePlatformServices(out, []Service{
				{Name: "directory", ActorCN: "ignored", Kind: "Gateway", ExternalNetworkConnection: "public"},
				{Name: "new", ActorCN: "not-invented", Kind: `Trusted("file")`},
			})
		},
	})
	if data.APIStatus != "connected" || actorCalls.Load() != 1 || serviceCalls.Load() != 2 {
		t.Fatalf("deduplication/health changed: actors=%d services=%d errors=%v", actorCalls.Load(), serviceCalls.Load(), data.Errors)
	}
	if len(data.Actors) != 2 || data.Actors[0].CN != "actor/a" || data.Actors[0].NodeDetails.Latitude == nil || *data.Actors[0].NodeDetails.Latitude != 0 ||
		data.Actors[0].NodeDetails.Counters[0].Value != "18446744073709551615" {
		t.Fatalf("actor/node wire data changed: %+v", data.Actors)
	}
	var gateways int
	for _, item := range data.Services {
		if item.Kind == "Gateway" && item.ActorCN == "actor/a" && item.Address == "fd00::1" && item.ExternalNetworkConnection == "public" {
			gateways++
		}
	}
	if len(data.Services) != 3 || gateways != 1 {
		t.Fatalf("service merge changed: %+v", data.Services)
	}
	if len(data.Trusted) != 2 || data.Trusted[0].Name != "directory" || data.Trusted[0].Provider != "file" || data.Trusted[0].EditorURL != "https://operator.example/directory" ||
		*data.Trusted[0].LastLookupMS != 42 || data.Trusted[1].Health != "unverified" {
		t.Fatalf("trusted ordering/enrichment changed: %+v", data.Trusted)
	}
	if data.Network[0].LinkID != "z" || data.RecentDenies[0].Count != 2 {
		t.Fatal("network or deny ordering changed")
	}
	if data.VisaCount != 14 || len(data.RecentVisas) != 10 || data.RecentVisas[0].ID != 14 || len(data.ActiveVisas) != 12 || data.ActiveVisas[0].ID != 12 || data.ActiveVisas[11].ID != 1 {
		t.Fatalf("visa count/recent/active contract changed: %+v", data)
	}
}

func TestFetchVisaInventoryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	entries := make([]VisaEntry, 32)
	for i := range entries {
		entries[i].ID = int64(i + 1)
	}
	var messages []string
	var mu sync.Mutex
	items, complete := FetchVisaInventory(ctx, readFunc(func(ctx context.Context, _ string, _ any) error {
		return ctx.Err()
	}), entries, &messages, &mu)
	if complete || len(items) != 0 || len(messages) != 32 {
		t.Fatalf("cancellation must mark every detail incomplete: complete=%t items=%v messages=%v", complete, items, messages)
	}
}
