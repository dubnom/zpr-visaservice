package policyclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPolicyAttributesResponseContractsWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/profiles")
	t.Setenv("ZPR_SIMULATOR_URL", "://invalid")
	t.Setenv("ZPR_POLICY_SERVICE_URL", "://unrelated-environment")
	body := `{"configured":true,"attributes":[{"source":"ou","attribute":"device.department"}],"people":[{"uid":"private-person"}],"records":[{"content":"private-source"}]}`
	const limit = 4 << 20
	for _, test := range []struct {
		name   string
		status int
		body   string
		want   []Attribute
		err    string
	}{
		{"attributes only", 200, body, []Attribute{{Source: "ou", Attribute: "device.department"}}, ""},
		{"empty", 200, `{"configured":true,"attributes":[]}`, []Attribute{}, ""},
		{"null", 200, `{"configured":true,"attributes":null}`, nil, ""},
		{"missing attributes", 200, `{"configured":true}`, nil, ""},
		{"exact limit", 200, body + strings.Repeat(" ", limit-len(body)), []Attribute{{Source: "ou", Attribute: "device.department"}}, ""},
		{"one byte over", 200, body + strings.Repeat(" ", limit+1-len(body)), nil, "Policy Service context exceeds the size limit"},
		{"malformed", 200, `{`, nil, "Policy Service returned invalid assistant context"},
		{"wrong attribute type", 200, `{"configured":true,"attributes":[{"source":7}]}`, nil, "Policy Service returned invalid assistant context"},
		{"trailing object", 200, body + `{}`, nil, "Policy Service returned invalid assistant context"},
		{"unconfigured", 200, `{"configured":false,"attributes":[{"source":"private"}]}`, nil, "Policy Service workspace is unavailable"},
		{"missing configured", 200, `{}`, nil, "Policy Service workspace is unavailable"},
		{"HTTP error before decoding", 503, body, nil, "Policy Service returned HTTP 503"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.RequestURI() != "/api/policy" || r.Header.Get("Accept") != "application/json" {
					t.Errorf("unexpected attribute request: %s %s %v", r.Method, r.URL, r.Header)
				}
				for _, header := range []string{"Cookie", "Authorization", "Origin", "X-ZPR-CSRF"} {
					if r.Header.Get(header) != "" {
						t.Errorf("browser credential forwarded: %s", header)
					}
				}
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			baseURL, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			originalURL := *baseURL
			transport := http.DefaultTransport.(*http.Transport).Clone()
			defer transport.CloseIdleConnections()
			got, err := FetchAttributes(context.Background(), baseURL, transport)
			if test.err != "" {
				if err == nil || err.Error() != test.err || got != nil {
					t.Fatalf("attributes=%v error=%v, want nil %q", got, err, test.err)
				}
			} else if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("attributes=%v error=%v, want %v", got, err, test.want)
			}
			if *baseURL != originalURL {
				t.Fatal("configured origin mutated")
			}
		})
	}
}

func TestPolicyAttributesDoesNotFollowRedirects(t *testing.T) {
	var followed atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		followed.Add(1)
		_, _ = io.WriteString(w, `{"configured":true}`)
	}))
	defer destination.Close()
	for _, status := range []int{301, 302, 303, 307, 308} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, destination.URL, status)
		}))
		baseURL, err := url.Parse(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		attributes, err := FetchAttributes(context.Background(), baseURL, transport)
		transport.CloseIdleConnections()
		server.Close()
		want := "Policy Service returned HTTP " + strconv.Itoa(status)
		if err == nil || err.Error() != want || attributes != nil {
			t.Fatalf("redirect %d: attributes=%v error=%v", status, attributes, err)
		}
	}
	if followed.Load() != 0 {
		t.Fatal("attribute client followed a redirect")
	}
}

func TestPolicyAttributesCancellationAndReadFailure(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("canceled request reached repository")
		}))
		defer server.Close()
		baseURL, err := url.Parse(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		defer transport.CloseIdleConnections()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		attributes, err := FetchAttributes(ctx, baseURL, transport)
		if !errors.Is(err, context.Canceled) || attributes != nil {
			t.Fatalf("attributes=%v error=%v", attributes, err)
		}
	})
	t.Run("deadline during read", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}))
		defer server.Close()
		baseURL, err := url.Parse(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		defer transport.CloseIdleConnections()
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		attributes, err := FetchAttributes(ctx, baseURL, transport)
		if !errors.Is(err, context.DeadlineExceeded) || attributes != nil {
			t.Fatalf("attributes=%v error=%v", attributes, err)
		}
	})
	t.Run("truncated response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100")
			_, _ = io.WriteString(w, `{"configured":true}`)
		}))
		defer server.Close()
		baseURL, err := url.Parse(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		defer transport.CloseIdleConnections()
		attributes, err := FetchAttributes(context.Background(), baseURL, transport)
		if !errors.Is(err, io.ErrUnexpectedEOF) || attributes != nil {
			t.Fatalf("attributes=%v error=%v", attributes, err)
		}
	})
}
