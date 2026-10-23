package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/controlservice/changewatcher"
)

const watcherTestUUID = "12345678-1234-1234-1234-123456789012"

func watchedSource(name string) assertionSourceConfig {
	return assertionSourceConfig{Name: name, Kind: "ldap", WatchChanges: true, URL: "ldaps://production.invalid:636", CAFile: "operator-ca.pem", Organizations: map[string]assertionSourceScope{
		"org-a": {BaseDN: "dc=a", BindDN: "cn=reader,dc=a", BindPasswordFile: "private-a", VisaServiceID: name + "_a"},
		"org-b": {BaseDN: "dc=b", BindDN: "cn=reader,dc=b", BindPasswordFile: "private-b", VisaServiceID: name + "_b"},
	}}
}

func TestTrustedWatcherProductionRestartIsolationNoSimulator(t *testing.T) {
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	t.Setenv("ZPR_SIMULATOR_MANIFEST", "unavailable")
	var mu sync.Mutex
	calls := map[string]int{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.HasPrefix(r.URL.Path, "/admin/services/") || !strings.HasSuffix(r.URL.Path, "/cache") || r.Header.Get("X-API-Key") != "write-key" {
			t.Errorf("unexpected production invalidation %s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		calls[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	admin := &adminClient{baseURL: server.URL, http: server.Client(), apiKey: "write-key"}
	dir := filepath.Join(t.TempDir(), "watchers")
	for cycle := 0; cycle < 2; cycle++ {
		configs := []assertionSourceConfig{watchedSource("ldap_one"), watchedSource("ldap_two")}
		runtime := &assertionRuntime{organizationID: "org-a", sourcesConfig: assertionSourcesConfig{Sources: configs}, sources: map[string]assertionNamedSource{}}
		for _, config := range configs {
			runtime.sources[config.Name] = assertionNamedSource{read: func(_ context.Context, org, base, bind string) (assertionDirectory, error) {
				expected := config.Organizations[org]
				if base != expected.BaseDN || bind != expected.BindDN {
					t.Errorf("cross-source organization scope %s %s", base, bind)
				}
				return assertionDirectory{PersonAttributes: map[string]map[string][]string{"alice": {"title": {"updated-private-value"}}}}, nil
			}}
		}
		done := make(chan struct{}, 4)
		stop, err := runtime.startTrustedWatchersUsing(context.Background(), dir, admin, func(config assertionSourceConfig, scope assertionSourceScope) changewatcher.Stream {
			return func(ctx context.Context, initial []byte, sink changewatcher.Sink, save func([]byte) error) error {
				cookie := config.Name + ":" + scope.BaseDN
				kind := "refresh_start"
				if cycle == 0 && len(initial) != 0 {
					t.Error("new source resumed a different cookie")
				}
				if cycle == 1 {
					kind = "resume_start"
					if string(initial) != cookie {
						t.Errorf("restart lost source isolation: %q want %q", initial, cookie)
					}
				}
				if err := sink(changewatcher.Event{Version: 1, BaseDN: scope.BaseDN, Type: kind}); err != nil {
					return err
				}
				for _, eventType := range []string{"present", "add", "modify", "delete", "refresh_complete"} {
					event := changewatcher.Event{Version: 1, BaseDN: scope.BaseDN, Type: eventType}
					if eventType != "refresh_complete" {
						event.UUID = watcherTestUUID
						event.DN = "uid=alice," + scope.BaseDN
					}
					if err := sink(event); err != nil {
						return err
					}
				}
				if err := save([]byte(cookie)); err != nil {
					return err
				}
				done <- struct{}{}
				<-ctx.Done()
				return nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 4; i++ {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				stop()
				t.Fatal("production watcher did not refresh")
			}
		}
		changes, _ := newTrustedSourceChanges("")
		changes.watcherStatus = runtime.allWatcherStatus
		response := httptest.NewRecorder()
		changes.listFeeds(response, httptest.NewRequest(http.MethodGet, "/api/trusted-sources/change-feeds", nil))
		var body struct {
			Watchers []struct {
				Organization string               `json:"organization_id"`
				Source       string               `json:"source_id"`
				Health       changewatcher.Status `json:"health"`
			}
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Watchers) != 4 {
			t.Errorf("health missing independent sources: %s", response.Body.String())
		}
		for _, watcher := range body.Watchers {
			if watcher.Health.State != "healthy" {
				t.Errorf("unhealthy: %+v", watcher)
			}
		}
		if strings.Contains(response.Body.String(), "private") || strings.Contains(response.Body.String(), "cookie") || strings.Contains(response.Body.String(), "dc=") {
			t.Error("health leaked private metadata")
		}
		if len(runtime.watcherStatus()) != 2 {
			t.Error("assertion status crossed organization boundary")
		}
		stop()
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 4 {
		t.Fatalf("wrong cache consumers: %v", calls)
	}
	for path, count := range calls {
		if count != 10 {
			t.Errorf("%s invalidations=%d want 10", path, count)
		}
	}
}

func TestTrustedWatcherFailureStopsWithoutCheckpoint(t *testing.T) {
	for _, failure := range []string{"source", "admin", "checkpoint"} {
		t.Run(failure, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if failure == "source" {
					t.Error("failed source must not invalidate a different cache")
				}
				if failure == "admin" {
					w.WriteHeader(http.StatusForbidden)
				} else {
					w.WriteHeader(http.StatusAccepted)
				}
			}))
			defer server.Close()
			config := watchedSource("ldap")
			delete(config.Organizations, "org-b")
			runtime := &assertionRuntime{sourcesConfig: assertionSourcesConfig{Sources: []assertionSourceConfig{config}}, sources: map[string]assertionNamedSource{"ldap": {read: func(context.Context, string, string, string) (assertionDirectory, error) {
				if failure == "source" {
					return assertionDirectory{}, errors.New("sensitive source error")
				}
				return assertionDirectory{}, nil
			}}}}
			completed := make(chan struct{})
			dir := filepath.Join(t.TempDir(), "state")
			stop, err := runtime.startTrustedWatchersUsing(context.Background(), dir, &adminClient{baseURL: server.URL, http: server.Client()}, func(_ assertionSourceConfig, scope assertionSourceScope) changewatcher.Stream {
				return func(ctx context.Context, initial []byte, sink changewatcher.Sink, save func([]byte) error) error {
					defer close(completed)
					if err := sink(changewatcher.Event{Version: 1, Type: "refresh_start"}); err != nil {
						return err
					}
					if err := sink(changewatcher.Event{Version: 1, Type: "modify", UUID: watcherTestUUID, DN: "uid=alice," + scope.BaseDN}); err != nil {
						return err
					}
					if failure == "checkpoint" {
						return save(make([]byte, (64<<10)+1))
					}
					return save([]byte("must-not-commit"))
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			<-completed
			stop()
			status := runtime.allWatcherStatus()[0]["health"].(changewatcher.Status)
			if status.State != "failed" || strings.Contains(status.Error, "sensitive") {
				t.Fatalf("unsafe failure status %+v", status)
			}
			files, err := filepath.Glob(filepath.Join(dir, "*.json"))
			if err != nil || len(files) != 0 {
				t.Fatalf("failed event committed checkpoint %v %v", files, err)
			}
		})
	}
}

func TestTrustedWatcherConfigurationFailsClosed(t *testing.T) {
	config := watchedSource("ldap")
	for _, mutate := range []func(*assertionSourceConfig){
		func(c *assertionSourceConfig) { c.Kind = "https-json" }, func(c *assertionSourceConfig) { c.Container = "demo" }, func(c *assertionSourceConfig) { c.CAFile = "" }, func(c *assertionSourceConfig) { c.Organizations = nil }, func(c *assertionSourceConfig) {
			scope := c.Organizations["org-a"]
			scope.VisaServiceID = ""
			c.Organizations["org-a"] = scope
		},
	} {
		copy := watchedSource("ldap")
		mutate(&copy)
		if validateWatchedSource(copy) == nil {
			t.Fatalf("invalid watcher config accepted %+v", copy)
		}
	}
	if editableAssertionSource(config, "org-a") {
		t.Fatal("running watcher can be rebound through registry")
	}
	runtime := &assertionRuntime{sourcesConfig: assertionSourcesConfig{Sources: []assertionSourceConfig{config}}}
	if stop, err := runtime.startTrustedWatchers(context.Background(), t.TempDir(), nil); err == nil {
		stop()
		t.Fatal("watcher without production invalidation API started")
	}
	runtime = &assertionRuntime{}
	stop, err := runtime.startTrustedWatchers(context.Background(), "", nil)
	if err != nil {
		t.Fatal("unconfigured watchers must preserve starter behavior")
	}
	stop()
}

func TestTrustedWatcherAdminRedirectAndFailure(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("Admin write credential followed redirect") }))
	defer target.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	admin := &adminClient{baseURL: server.URL, http: server.Client(), apiKey: "secret"}
	if err := admin.refreshTrustedService(context.Background(), "ldap"); err == nil {
		t.Fatal("redirect counted as successful cache invalidation")
	}
}
