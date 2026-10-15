package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScenarioHTTPResultCapturesObservationAndFailedExpectation(t *testing.T) {
	body := "<!doctype html><script>window.gatewayExecuted=true</script>\n{\"n\":9007199254740993}"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/page", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Set-Cookie", "secret-cookie")
		w.Header().Set("X-API-Key", "secret-key")
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, body)
	}))
	defer server.Close()
	for _, expected := range []string{"", "allow", "deny"} {
		t.Run("expected-"+expected, func(t *testing.T) {
			event, err := requestWebGatewayPage(t.Context(), server.Client(), server.URL+"/start", "machine", "finance-client", expected,
				func(string, testAppEvent) error { return nil })
			if (err == nil) != (expected == "") {
				t.Fatalf("expectation error: %v", err)
			}
			result := event.Result
			if result == nil || result.StatusCode != 502 || result.StatusText != "Bad Gateway" ||
				result.RequestedURL != server.URL+"/start" || result.FinalURL != server.URL+"/page" ||
				result.Body != body || result.BodyEncoding != "utf-8" || result.BytesRead != len(body) ||
				result.DurationMS <= 0 || result.BodyTruncated || result.ContentType != "text/html" ||
				result.Headers["Set-Cookie"][0] != "[redacted]" || result.Headers["X-Api-Key"][0] != "[redacted]" ||
				result.Error != "" {
				t.Fatalf("response observation: %+v", result)
			}
			var output bytes.Buffer
			if writeErr := writeWebGatewayResult(&output, event, err); !errors.Is(writeErr, err) {
				t.Fatalf("write lost request error: %v", writeErr)
			}
			if strings.Contains(output.String(), "secret-cookie") || strings.Contains(output.String(), "secret-key") {
				t.Fatal("sensitive headers escaped redaction")
			}
			manager := newSimulatorScenarioManager()
			step := simulatorScenarioStep{Action: "request_web_gateway", Target: server.URL + "/start", Expected: expected}
			execute := func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
				return output.String() + "\ncontroller diagnostic\n", err
			}
			stepErr := manager.executeStep(t.Context(), simulatorManifest{}, step, execute, "run", 1, false)
			snapshot := manager.snapshot()
			stored := snapshot.Steps[0]
			if !errors.Is(stepErr, err) || stored.Result == nil || stored.Result.Body != body ||
				stored.ResultError != "" || stored.Output != "" ||
				(stored.Status == "failed") != (expected != "") {
				t.Fatalf("stored observation/verdict: %+v %v", stored, stepErr)
			}
		})
	}
}

type scenarioBrokenBody struct{ read bool }

func (body *scenarioBrokenBody) Read(buffer []byte) (int, error) {
	if !body.read {
		body.read = true
		return copy(buffer, "partial response"), nil
	}
	return 0, errors.New("fixture response interrupted")
}
func (*scenarioBrokenBody) Close() error { return nil }

func TestScenarioHTTPResultPreservesErrorsAndPartialBody(t *testing.T) {
	for _, test := range []struct {
		name       string
		response   *http.Response
		err        error
		wantStatus int
		wantBody   string
	}{
		{"network", nil, errors.New("fixture connection failure"), 0, ""},
		{"body", &http.Response{StatusCode: 200, Header: http.Header{}, Body: &scenarioBrokenBody{}}, nil, 200, "partial response"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: webGatewayTestRoundTripper(func(*http.Request) (*http.Response, error) {
				return test.response, test.err
			})}
			event, err := requestWebGatewayPage(t.Context(), client, "http://fixture.invalid/", "machine", "finance-client", "",
				func(string, testAppEvent) error { return nil })
			if err == nil || event.Result == nil || event.Result.Error == "" ||
				event.Result.StatusCode != test.wantStatus || event.Result.Body != test.wantBody {
				t.Fatalf("lost partial/error evidence: %+v %v", event, err)
			}
			if test.name == "body" && !event.Result.BodyTruncated {
				t.Fatal("interrupted body was presented as complete")
			}
			var output bytes.Buffer
			if writeErr := writeWebGatewayResult(&output, event, err); writeErr == nil {
				t.Fatal("error became success")
			}
			result, parseErr := parseScenarioHTTPResult(output.String(), "http://fixture.invalid/")
			if parseErr != nil || result.Error == "" || result.Body != test.wantBody {
				t.Fatalf("failed result not exported: %+v %v", result, parseErr)
			}
		})
	}
}

func TestScenarioHTTPResultBoundsAndBinaryEvidence(t *testing.T) {
	for _, test := range []struct {
		name, body         string
		encoding           string
		truncated, limited bool
	}{
		{"preview", strings.Repeat("x", webGatewayPagePreviewLimit+1), "utf-8", true, false},
		{"read-limit", strings.Repeat("x", webGatewayPageBodyLimit+2), "utf-8", true, true},
		{"binary", "\xff\x00\xfe", "base64", false, false},
		{"empty", "", "utf-8", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: webGatewayTestRoundTripper(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(test.body)), Request: request}, nil
			})}
			event, err := requestWebGatewayPage(t.Context(), client, "http://fixture.invalid/", "machine", "finance-client", "",
				func(string, testAppEvent) error { return nil })
			result := event.Result
			if err != nil || result.BodyEncoding != test.encoding || result.BodyTruncated != test.truncated ||
				result.ReadLimited != test.limited || result.BytesRead != min(len(test.body), webGatewayPageBodyLimit+1) {
				t.Fatalf("bounded result: %+v %v", result, err)
			}
			if test.encoding == "base64" {
				decoded, err := base64.StdEncoding.DecodeString(result.Body)
				if err != nil || string(decoded) != test.body {
					t.Fatal("binary response bytes changed")
				}
			} else if len(result.Body) > webGatewayPagePreviewLimit {
				t.Fatal("unbounded preview")
			}
		})
	}
	headers, limited := gatewayResultHeaders(http.Header{"Content-Type": {strings.Repeat("x", 20000)}, "Authorization": {"secret"}})
	if !limited || len(headers["Content-Type"][0]) != 2048 || headers["Authorization"][0] != "[redacted]" {
		t.Fatalf("header limits: %+v %v", headers, limited)
	}
}

func TestScenarioHTTPResultContractAndMissingEvidence(t *testing.T) {
	event := testAppEvent{Service: "internet-gateway", Role: "client", Result: &scenarioHTTPResult{
		SchemaVersion: 1, Kind: "http", Method: "GET", RequestedURL: "http://fixture.invalid/",
		StatusCode: 200, BodyEncoding: "utf-8", Headers: map[string][]string{"Set-Cookie": {"secret"}},
	}}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseScenarioHTTPResult(string(data), event.Result.RequestedURL)
	if err != nil || result.Headers["Set-Cookie"][0] != "[redacted]" {
		t.Fatalf("receiving redaction failed: %+v %v", result, err)
	}
	for _, output := range []string{"", "legacy output", string(data) + "\n" + string(data), strings.Repeat("x", 256<<10)} {
		if _, err := parseScenarioHTTPResult(output, event.Result.RequestedURL); err == nil {
			t.Fatal("missing/duplicate/oversized result accepted")
		}
	}
	if _, err := parseScenarioHTTPResult(string(data), "http://different.invalid/"); err == nil {
		t.Fatal("result attached to a different target")
	}
	manager := newSimulatorScenarioManager()
	err = manager.executeStep(t.Context(), simulatorManifest{}, simulatorScenarioStep{Action: "request_web_gateway"},
		func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
			return "legacy output", nil
		}, "run", 1, false)
	if err != nil || manager.snapshot().Steps[0].ResultError == "" {
		t.Fatal("missing evidence was silently discarded")
	}
	if err := validateWebGatewayPageTarget("https://www.google.com/", "", []string{"google.com", "*.google.com"}); err != nil {
		t.Fatalf("observational allowed-host fetch rejected: %v", err)
	}
	if err := validateWebGatewayPageTarget("https://unlisted.invalid/", "", []string{"google.com"}); err == nil {
		t.Fatal("observational fetch bypassed destination scope")
	}
}
