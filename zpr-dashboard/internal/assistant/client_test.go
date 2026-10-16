package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCompleteExplicitConfigurationWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/profiles")
	t.Setenv("ZPR_SIMULATOR_URL", "://invalid")
	t.Setenv("ANTHROPIC_API_KEY", "unrelated-environment-key")
	t.Setenv("ANTHROPIC_MODEL", "unrelated-environment-model")
	messages := []Message{{Role: "user", Content: "draft <data>"}, {Role: "assistant", Content: "prior answer"}, {Role: "user", Content: "review"}}
	original := append([]Message(nil), messages...)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.RequestURI() != "/v1/messages?version=1" {
			t.Errorf("request=%s %s", r.Method, r.URL)
		}
		for name, want := range map[string]string{
			"Content-Type": "application/json", "X-Api-Key": "explicit-key", "Anthropic-Version": "2023-06-01",
		} {
			if r.Header.Get(name) != want {
				t.Errorf("%s=%q, want %q", name, r.Header.Get(name), want)
			}
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		want := map[string]any{"model": "explicit-model", "max_tokens": 600, "system": "caller-owned system", "messages": messages}
		if len(body) != len(want) {
			t.Errorf("unexpected request fields: %v", body)
		}
		for key, value := range want {
			expected, _ := json.Marshal(value)
			if string(body[key]) != string(expected) {
				t.Errorf("%s=%s, want %s", key, body[key], expected)
			}
		}
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"first"},{"type":"tool_use","text":"ignored"},{"type":"text","text":"second"}],"usage":{"input_tokens":123,"output_tokens":17},"extra":true}`)
	}))
	defer upstream.Close()
	client := Client{APIKey: "explicit-key", URL: upstream.URL + "/v1/messages?version=1", HTTP: upstream.Client()}
	reply, err := client.Complete(context.Background(), "caller-owned system", messages, "explicit-model", 600)
	if err != nil || reply != (Reply{Answer: "first\nsecond", InputTokens: 123, OutputTokens: 17}) {
		t.Fatalf("reply=%+v error=%v", reply, err)
	}
	if !reflect.DeepEqual(messages, original) {
		t.Fatal("caller messages changed")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type observedBody struct {
	io.Reader
	closed bool
}

func (b *observedBody) Close() error {
	b.closed = true
	return nil
}

type failedReader struct{ err error }

func (r failedReader) Read([]byte) (int, error) { return 0, r.err }

func TestCompleteResponseContracts(t *testing.T) {
	valid := `{"content":[{"type":"text","text":"ok"}]}`
	for _, tc := range []struct {
		name   string
		status int
		body   string
		error  string
	}{
		{"text", 200, valid, ""},
		{"exact limit", 200, valid + strings.Repeat(" ", (64<<10)-len(valid)), ""},
		{"existing truncation behavior", 200, valid + strings.Repeat(" ", (64<<10)-len(valid)) + "ignored beyond limit", ""},
		{"truncated JSON", 200, `{"content":[{"type":"text","text":"` + strings.Repeat("x", 64<<10) + `"}]}`, "unexpected end"},
		{"malformed JSON", 200, `{`, "unexpected end"},
		{"wrong type", 200, `{"usage":{"input_tokens":"wrong"}}`, "cannot unmarshal"},
		{"trailing JSON", 200, valid + `{}`, "after top-level value"},
		{"no text", 200, `{"content":[{"type":"tool_use","text":"ignored"}]}`, "Claude response contained no text"},
		{"empty text", 200, `{"content":[{"type":"text","text":""}]}`, "Claude response contained no text"},
		{"missing content", 200, `{}`, "Claude response contained no text"},
		{"invalid structured request", 400, `{"type":"error","error":{"type":"invalid_request_error","message":"Invalid schema at output_config.format.schema.properties.proposal"}}`, "Anthropic returned HTTP 400: invalid_request_error: Invalid schema at output_config.format.schema.properties.proposal"},
		{"unauthorized", 401, `{"error":"private upstream detail"}`, "Anthropic returned HTTP 401"},
		{"rate limited", 429, `{}`, "Anthropic returned HTTP 429"},
		{"unavailable", 503, `{}`, "Anthropic returned HTTP 503"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &observedBody{Reader: strings.NewReader(tc.body)}
			client := Client{URL: "https://explicit.invalid/messages", HTTP: &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					content, err := io.ReadAll(r.Body)
					if err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(string(content), `"messages":[]`) {
						t.Errorf("nil message list wire shape changed: %s", content)
					}
					return &http.Response{StatusCode: tc.status, Body: body, Header: make(http.Header)}, nil
				}),
			}}
			reply, err := client.Complete(context.Background(), "", nil, "", 0)
			if tc.error == "" {
				if err != nil || reply != (Reply{Answer: "ok"}) {
					t.Fatalf("reply=%+v error=%v", reply, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.error) || reply != (Reply{}) {
				t.Fatalf("reply=%+v error=%v, want %q", reply, err, tc.error)
			}
			if !body.closed {
				t.Fatal("response body not closed")
			}
		})
	}
}

func TestProviderErrorSummaryIsBoundedAndSingleLine(t *testing.T) {
	message := strings.Repeat("schema error ", 100)
	body, err := json.Marshal(map[string]any{"error": map[string]string{"type": "invalid_request_error", "message": "schema\n" + message}})
	if err != nil {
		t.Fatal(err)
	}
	summary := providerErrorSummary(body)
	if strings.Contains(summary, "\n") || len([]rune(summary)) > 550 || !strings.HasPrefix(summary, "invalid_request_error: schema ") {
		t.Fatalf("provider error summary is not bounded/single-line: %q", summary)
	}
	if got := providerErrorSummary([]byte(`{"error":"unknown"}`)); got != "no provider error detail" {
		t.Fatalf("malformed provider error summary=%q", got)
	}
}

func TestCompleteTransportFailures(t *testing.T) {
	failure := errors.New("test read failure")
	body := &observedBody{Reader: failedReader{failure}}
	client := Client{URL: "https://explicit.invalid", HTTP: &http.Client{Transport: roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
		},
	)}}
	if reply, err := client.Complete(context.Background(), "", nil, "", 0); !errors.Is(err, failure) || reply != (Reply{}) || !body.closed {
		t.Fatalf("read error=%v reply=%+v closed=%v", err, reply, body.closed)
	}
	client.HTTP.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, failure
	})
	if reply, err := client.Complete(context.Background(), "", nil, "", 0); !errors.Is(err, failure) || reply != (Reply{}) {
		t.Fatalf("transport error=%v reply=%+v", err, reply)
	}
	client.URL = "://invalid"
	if _, err := client.Complete(context.Background(), "", nil, "", 0); err == nil {
		t.Fatal("invalid endpoint accepted")
	}

	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer upstream.Close()
	defer close(release)
	client = Client{URL: upstream.URL, HTTP: &http.Client{Timeout: 30 * time.Millisecond}}
	if _, err := client.Complete(context.Background(), "", nil, "", 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("client timeout=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Complete(ctx, "", nil, "", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
}

func TestCompleteTextBlockJoining(t *testing.T) {
	body := `{"content":[{"type":"text","text":""},{"type":"text","text":"first"},{"type":"image","text":"ignored"},{"type":"text","text":""},{"type":"text","text":"second"},{"type":"text","text":""}]}`
	client := Client{URL: "https://explicit.invalid", HTTP: &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
		}),
	}}
	reply, err := client.Complete(context.Background(), "", nil, "", 0)
	if err != nil || reply.Answer != "first\n\nsecond\n" {
		t.Fatalf("text joining=%q error=%v", reply.Answer, err)
	}
}

func TestCompleteRedirectRefusal(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("redirect target must not receive credentials or prompt")
	}))
	defer target.Close()
	for _, status := range []int{301, 302, 303, 307, 308} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, status)
		}))
		client := Client{APIKey: "test-only", URL: upstream.URL, HTTP: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}}
		_, err := client.Complete(context.Background(), "private prompt", nil, "model", 600)
		upstream.Close()
		if err == nil || !strings.HasPrefix(err.Error(), "Anthropic returned HTTP ") {
			t.Fatalf("redirect %d error=%v", status, err)
		}
	}
}

func TestAssistantDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect assistant dependencies: %v\n%s", err, output)
	}
	packages := strings.Fields(string(output))
	if len(packages) != 1 || packages[0] != "neboagency.com/zpr-dashborad/internal/assistant" {
		t.Fatalf("assistant transport must use only the standard library, got %v", packages)
	}
}
