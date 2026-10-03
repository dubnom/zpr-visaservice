package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMachineLogLinesAreBoundedAndRedacted(t *testing.T) {
	output := strings.Repeat("old entry\n", 110) + "authorization: Bearer private-bearer\npassword=private-password\n{\"token\":\"private-token\"}\nzpr_vsapi.01234567.private-key\n"
	lines := machineLogLines(output)
	if len(lines) != 100 {
		t.Fatalf("log lines = %d, want 100", len(lines))
	}
	if strings.Contains(strings.Join(lines, "\n"), "private-") {
		t.Fatal("credential was exposed in log output")
	}
	if len(machineLogLines("")) != 0 {
		t.Fatal("empty output must produce an empty list")
	}
	if len(strings.Join(machineLogLines(strings.Repeat("long entry\n", 10000)), "\n")) > 65536 {
		t.Fatal("log output exceeded byte limit")
	}
}

func TestMachineLogSourcesUseKnownWorkloadsAndReportErrors(t *testing.T) {
	var calls [][]string
	execute := func(_ context.Context, command string, args ...string) (string, error) {
		if command != "docker" {
			t.Fatalf("unexpected executable %q", command)
		}
		calls = append(calls, args)
		if args[len(args)-1] == "/tmp/machine-control-ph.log" {
			return "password=private-error", errors.New("missing file")
		}
		return "hello\n", nil
	}
	sources := readMachineLogSources(t.Context(), "machine-01", []string{"finance-client", "../../secret"}, execute)
	if len(sources) != 4 || len(calls) != 4 {
		t.Fatalf("sources=%d calls=%d, want four known sources", len(sources), len(calls))
	}
	if sources[1].Error == "" || len(sources[1].Lines) != 0 {
		t.Fatal("failed source should report unavailable, not expose error output")
	}
	for _, call := range calls {
		if strings.Contains(strings.Join(call, " "), "../../secret") {
			t.Fatal("unknown workload was passed to the container")
		}
	}
}

func TestMachineLogSourcesAreSeparatedByCategory(t *testing.T) {
	for _, test := range []struct {
		category string
		want     []string
	}{
		{"adapter", []string{"Controller", "Control adapter", "finance-client adapter"}},
		{"workload", []string{"finance-client events"}},
	} {
		t.Run(test.category, func(t *testing.T) {
			var calls int
			execute := func(_ context.Context, command string, args ...string) (string, error) {
				calls++
				if command != "docker" || strings.Contains(strings.Join(args, " "), "../../secret") {
					t.Fatal("unexpected log collection command")
				}
				return "password=private-value\nentry\n", nil
			}
			sources := readSelectedMachineLogSources(t.Context(), "machine-01", []string{"finance-client", "../../secret"}, execute, test.category)
			if len(sources) != len(test.want) || calls != len(test.want) {
				t.Fatalf("sources=%d calls=%d, want %d", len(sources), calls, len(test.want))
			}
			for index, source := range sources {
				if source.Name != test.want[index] || strings.Contains(strings.Join(source.Lines, " "), "private-value") {
					t.Fatalf("unexpected source: %+v", source)
				}
			}
		})
	}
}

func TestAdapterLogsProxyIsReadOnlyAndDropsCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/simulator/adapter-logs" || r.URL.RawQuery != "" {
			t.Errorf("unexpected upstream URL: %s", r.URL)
		}
		for _, name := range []string{"Authorization", "Cookie", "Origin"} {
			if r.Header.Get(name) != "" {
				t.Errorf("forwarded private header %s", name)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"machines":[]}`))
	}))
	defer upstream.Close()
	t.Setenv("ZPR_SIMULATOR_URL", upstream.URL)
	handler := newAdapterLogsProxy()
	request := httptest.NewRequest(http.MethodGet, "/api/adapter-logs?ignored=yes", nil)
	request.Header.Set("Authorization", "Bearer private")
	request.Header.Set("Cookie", "private=value")
	request.Header.Set("Origin", "http://control.local")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"machines"`) {
		t.Fatalf("unexpected proxy response: %d %s", response.Code, response.Body)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/adapter-logs", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", response.Code)
	}
}

type logTestTransport func(*http.Request) (*http.Response, error)

func (transport logTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestControlRoomUsesSlowTransportForLogsAndAssertionReads(t *testing.T) {
	var selected string
	transport := controlRoomTransport{
		standard: logTestTransport(func(request *http.Request) (*http.Response, error) {
			selected = "standard"
			return &http.Response{StatusCode: http.StatusOK}, nil
		}),
		logs: logTestTransport(func(request *http.Request) (*http.Response, error) {
			selected = "logs"
			return &http.Response{StatusCode: http.StatusOK}, nil
		}),
	}
	for _, test := range []struct{ route, want string }{
		{"/api/adapter-logs", "logs"},
		{"/api/assertions/evaluate", "logs"},
		{"/api/assertions/source", "logs"},
		{"/api/assertions", "standard"},
		{"/api/snapshot", "standard"},
		{"/api/policy", "standard"},
	} {
		_, err := transport.RoundTrip(httptest.NewRequest(http.MethodGet, test.route, nil))
		if err != nil || selected != test.want {
			t.Fatalf("route %s used %s transport, want %s", test.route, selected, test.want)
		}
	}
}

func TestAdapterLogsProxyRejectsNonLoopbackAndPathOrigins(t *testing.T) {
	for _, endpoint := range []string{"http://example.com", "http://127.0.0.1/private", "https://127.0.0.1", "http://user:pass@127.0.0.1"} {
		t.Run(endpoint, func(t *testing.T) {
			t.Setenv("ZPR_SIMULATOR_URL", endpoint)
			response := httptest.NewRecorder()
			newAdapterLogsProxy().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/adapter-logs", nil))
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("unsafe origin status = %d", response.Code)
			}
		})
	}
}
