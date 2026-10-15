package adminapi

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExplicitConfigurationAndTLS(t *testing.T) {
	for _, name := range []string{
		"ZPR_ADMIN_URL", "ZPR_ADMIN_API_KEY", "ZPR_ADMIN_KEY_FILE", "ZPR_ADMIN_CA_FILE",
		"ZPR_ADMIN_SERVER_NAME", "ZPR_SIMULATOR_URL", "ZPR_SIMULATION_MANIFEST",
	} {
		t.Setenv(name, "unavailable")
	}
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.Header.Get("X-API-Key") != "explicit-key" || r.URL.RequestURI() != "/admin/stats?limit=20" {
			t.Errorf("unexpected request: %s %s credential=%t", r.Method, r.URL.RequestURI(), r.Header.Get("X-API-Key") == "explicit-key")
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()
	directory := t.TempDir()
	caFile := filepath.Join(directory, "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	config := Config{
		BaseURL: server.URL + "///", APIKey: " explicit-key\n", CAFile: caFile,
		KeyFile: filepath.Join(directory, "absent"), ServerName: server.Certificate().DNSNames[0],
	}
	open := func() *Client {
		t.Helper()
		client, err := New(config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(client.HTTP.CloseIdleConnections)
		return client
	}
	client := open()
	if client.BaseURL != server.URL || client.APIKey != "explicit-key" || client.HTTP.Timeout != 8*time.Second {
		t.Fatalf("configuration changed: origin=%q timeout=%v", client.BaseURL, client.HTTP.Timeout)
	}
	transport, ok := client.HTTP.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatal("TLS minimum changed")
	}
	if err := transport.TLSClientConfig.VerifyConnection(tls.ConnectionState{}); err == nil {
		t.Fatal("missing peer certificate accepted")
	}
	request := func(client *Client) error {
		var result map[string]bool
		err := client.GetJSON(context.Background(), "/admin/stats?limit=20", &result)
		if err == nil && !result["ok"] {
			t.Error("response not decoded")
		}
		return err
	}
	if err := request(client); err != nil {
		t.Fatal(err)
	}
	config.ServerName = "wrong.example"
	if err := request(open()); err == nil {
		t.Fatal("configured hostname mismatch accepted")
	}
	config.ServerName = ""
	if err := request(open()); err != nil {
		t.Fatalf("chain-only verification: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("unexpected successful requests/retries: %d", calls.Load())
	}
}

func TestConfigurationErrorsAndFileKey(t *testing.T) {
	directory := t.TempDir()
	keyFile := filepath.Join(directory, "key")
	caFile := filepath.Join(directory, "invalid-ca")
	if err := os.WriteFile(keyFile, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caFile, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		config Config
		want   string
	}{
		{"missing", Config{}, ErrMissingConfig.Error()},
		{"http", Config{BaseURL: "http://example.com", APIKey: "key", CAFile: caFile}, ErrInvalidURL.Error()},
		{"whitespace", Config{BaseURL: " https://example.com", APIKey: "key", CAFile: caFile}, ErrInvalidURL.Error()},
		{"path", Config{BaseURL: "https://example.com/admin", APIKey: "key", CAFile: caFile}, ErrURLPath.Error()},
		{"missing key", Config{BaseURL: "https://example.com", KeyFile: filepath.Join(directory, "absent"), CAFile: caFile}, "read admin API key file:"},
		{"empty key", Config{BaseURL: "https://example.com", KeyFile: keyFile, CAFile: caFile}, "admin API key is empty"},
		{"missing CA", Config{BaseURL: "https://example.com", APIKey: "key", CAFile: filepath.Join(directory, "absent")}, "read admin CA file:"},
		{"invalid CA", Config{BaseURL: "https://example.com", APIKey: "key", CAFile: caFile}, "admin CA file contains no certificates"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, err := New(test.config)
			if client != nil || err == nil || !strings.HasPrefix(err.Error(), test.want) {
				t.Fatalf("configuration error: client=%v err=%v, want %q", client, err, test.want)
			}
		})
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte(" file-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := New(Config{BaseURL: server.URL, APIKey: " \n", KeyFile: keyFile, CAFile: caFile})
	if err != nil {
		t.Fatal(err)
	}
	defer client.HTTP.CloseIdleConnections()
	if client.APIKey != "file-key" {
		t.Fatal("file key not trimmed or whitespace inline key did not fall through")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type trackedBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *trackedBody) Read(buffer []byte) (int, error) {
	count, err := b.Reader.Read(buffer)
	b.read += count
	return count, err
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}

func TestReadStatusDrainAndCancellation(t *testing.T) {
	body := &trackedBody{Reader: strings.NewReader(strings.Repeat("x", 8192))}
	var calls int
	client := Client{
		BaseURL: "https://admin.example", APIKey: "key",
		HTTP: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			if err := request.Context().Err(); err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: 503, Status: "503 Service Unavailable", Body: body, Header: make(http.Header)}, nil
		})},
	}
	var result any
	err := client.GetJSON(context.Background(), "/admin/stats", &result)
	var status StatusError
	if !errors.As(err, &status) || status.Code != 503 || err.Error() != "admin API returned 503 Service Unavailable" {
		t.Fatalf("status lost: %v", err)
	}
	if body.read != 4096 || !body.closed || calls != 1 {
		t.Fatalf("drain/close/retry: bytes=%d closed=%t calls=%d", body.read, body.closed, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.GetJSON(ctx, "/admin/stats", &result); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	before := calls
	if err := client.GetJSON(context.Background(), "/\x00", &result); err == nil || calls != before {
		t.Fatalf("invalid request reached transport: %v", err)
	}
}

func TestDecoderAndRedirectCompatibility(t *testing.T) {
	exact := `{"value":"` + strings.Repeat("x", (2<<20)-12) + `"}`
	for _, test := range []struct {
		name, body string
		wantOK     bool
	}{
		{"exact reader window", exact, true},
		{"over reader window", strings.TrimSuffix(exact, `"}`) + `x"}`, false},
		{"trailing value", `{"value":"first"} {"ignored":true}`, true},
		{"unknown field", `{"value":"first","unknown":true}`, true},
		{"malformed", `{`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(test.body)}
			client := Client{BaseURL: "https://admin.example", HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
			})}}
			var result struct {
				Value string `json:"value"`
			}
			err := client.GetJSON(context.Background(), "/admin/test", &result)
			if (err == nil) != test.wantOK || !body.closed {
				t.Fatalf("decode=%v closed=%t", err, body.closed)
			}
			if err != nil && !strings.HasPrefix(err.Error(), "decode admin response:") {
				t.Fatalf("decode error prefix changed: %v", err)
			}
		})
	}
	// Exercise New's redirect policy without requiring an external endpoint.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/redirect" {
			t.Errorf("redirect followed: %s", r.URL.Path)
		}
		http.Redirect(w, r, "/unexpected", http.StatusFound)
	}))
	defer server.Close()
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := New(Config{BaseURL: server.URL, APIKey: "key", CAFile: caFile})
	if err != nil {
		t.Fatal(err)
	}
	defer client.HTTP.CloseIdleConnections()
	var result any
	var status StatusError
	if err := client.GetJSON(context.Background(), "/redirect", &result); !errors.As(err, &status) || status.Code != 302 {
		t.Fatalf("redirect status lost: %v", err)
	}
}
