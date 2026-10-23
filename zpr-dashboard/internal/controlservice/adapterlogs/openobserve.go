package adapterlogs

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpenObserve uses the same operator-owned inventory as file/container logs.
// Credentials remain server-side file references; backend errors are redacted.
func openObserveRequest(ctx context.Context, source SourceConfig, path string, body []byte) ([]byte, error) {
	fail := errors.New("OpenObserve log source unavailable")
	info, err := os.Lstat(source.PasswordFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return nil, fail
	}
	secret, err := os.ReadFile(source.PasswordFile)
	if err != nil || len(strings.TrimSpace(string(secret))) == 0 {
		return nil, fail
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, fail
	}
	if source.CAFile != "" {
		ca, err := os.ReadFile(source.CAFile)
		if err != nil || !roots.AppendCertsFromPEM(ca) {
			return nil, fail
		}
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, source.URL+"/api/"+source.Organization+"/"+path, bytes.NewReader(body))
	if err != nil {
		return nil, fail
	}
	request.SetBasicAuth(source.Username, strings.TrimSpace(string(secret)))
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, fail
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(data) > 65536 || response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fail
	}
	return data, nil
}

// Append sends bounded structured events to one explicitly configured destination.
// Callers must not supply credentials or raw service output as event payloads.
func Append(ctx context.Context, source SourceConfig, events []byte) error {
	if len(events) > 65536 || !json.Valid(events) {
		return errors.New("invalid log events")
	}
	data, err := openObserveRequest(ctx, source, source.Stream+"/_json", events)
	if err != nil {
		return err
	}
	var result struct {
		Code   int `json:"code"`
		Status []struct {
			Successful int `json:"successful"`
			Failed     int `json:"failed"`
		} `json:"status"`
	}
	if json.Unmarshal(data, &result) != nil || result.Code != 200 || len(result.Status) == 0 {
		return errors.New("log ingestion was not acknowledged")
	}
	for _, status := range result.Status {
		if status.Successful < 1 || status.Failed != 0 {
			return errors.New("log ingestion was not acknowledged")
		}
	}
	return nil
}

func Query(ctx context.Context, source SourceConfig) (string, error) {
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]any{"query": map[string]any{"sql": `SELECT * FROM "` + source.Stream + `" ORDER BY _timestamp DESC`, "start_time": now.Add(-7 * 24 * time.Hour).UnixMicro(), "end_time": now.UnixMicro(), "from": 0, "size": 100}})
	data, err := openObserveRequest(ctx, source, "_search", body)
	if err != nil {
		return "", err
	}
	var result struct {
		Hits []json.RawMessage `json:"hits"`
	}
	if json.Unmarshal(data, &result) != nil || result.Hits == nil {
		return "", errors.New("invalid log query response")
	}
	lines := make([]string, 0, len(result.Hits))
	for _, hit := range result.Hits {
		lines = append(lines, string(hit))
	}
	return strings.Join(lines, "\n"), nil
}
