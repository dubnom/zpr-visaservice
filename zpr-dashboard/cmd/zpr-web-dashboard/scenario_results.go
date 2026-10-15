package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"
)

// Observations are independent of the step's optional allow/deny expectation.
type scenarioHTTPResult struct {
	SchemaVersion  int                 `json:"schema_version"`
	Kind           string              `json:"kind"`
	Method         string              `json:"method"`
	RequestedURL   string              `json:"requested_url"`
	FinalURL       string              `json:"final_url,omitempty"`
	StatusCode     int                 `json:"status_code,omitempty"`
	StatusText     string              `json:"status_text,omitempty"`
	DurationMS     float64             `json:"duration_ms"`
	BytesRead      int                 `json:"bytes_read"`
	ReadLimited    bool                `json:"read_limited"`
	ContentType    string              `json:"content_type,omitempty"`
	Headers        map[string][]string `json:"headers,omitempty"`
	HeadersLimited bool                `json:"headers_limited"`
	Body           string              `json:"body"`
	BodyEncoding   string              `json:"body_encoding"`
	BodyTruncated  bool                `json:"body_truncated"`
	Error          string              `json:"error,omitempty"`
}

func gatewayResultHeaders(header http.Header) (map[string][]string, bool) {
	safe := map[string]bool{
		"Content-Type": true, "Content-Length": true, "Content-Encoding": true,
		"Date": true, "Server": true, "Cache-Control": true, "Etag": true,
		"Last-Modified": true, "Allow": true, "Retry-After": true,
	}
	keys := make([]string, 0, len(header))
	for name := range header {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	result := make(map[string][]string)
	remaining := 16 << 10
	limited := false
	for index, name := range keys {
		if index >= 64 || len(name) > remaining {
			limited = true
			break
		}
		remaining -= len(name)
		if !safe[http.CanonicalHeaderKey(name)] {
			if remaining < len("[redacted]") {
				limited = true
				break
			}
			result[name] = []string{"[redacted]"}
			remaining -= len("[redacted]")
			continue
		}
		for valueIndex, value := range header[name] {
			if valueIndex >= 16 {
				limited = true
				break
			}
			size := min(len(value), 2048, remaining)
			result[name] = append(result[name], strings.ToValidUTF8(value[:size], "\uFFFD"))
			remaining -= size
			if size < len(value) {
				limited = true
			}
			if remaining == 0 {
				limited = true
				break
			}
		}
	}
	return result, limited
}

func setGatewayResultBody(result *scenarioHTTPResult, body []byte) {
	result.BodyEncoding = "utf-8"
	if utf8.Valid(body) {
		result.Body = string(body)
	} else {
		result.BodyEncoding = "base64"
		result.Body = base64.StdEncoding.EncodeToString(body)
	}
}

func parseScenarioHTTPResult(output, target string) (*scenarioHTTPResult, error) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 4096), 256<<10)
	var result *scenarioHTTPResult
	for scanner.Scan() {
		var event testAppEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Result == nil {
			continue
		}
		if result != nil {
			return nil, errors.New("multiple gateway results received")
		}
		result = event.Result
		if event.Service != "internet-gateway" || event.Role != "client" ||
			result.SchemaVersion != 1 || result.Kind != "http" ||
			result.Method != http.MethodGet || result.RequestedURL != target ||
			(result.StatusCode != 0 && (result.StatusCode < 100 || result.StatusCode > 599)) ||
			(result.StatusCode == 0 && result.Error == "") ||
			result.DurationMS < 0 || result.BytesRead < 0 || result.BytesRead > webGatewayPageBodyLimit+1 ||
			len(result.Body) > 2*webGatewayPagePreviewLimit ||
			(result.BodyEncoding != "utf-8" && result.BodyEncoding != "base64") {
			return nil, errors.New("invalid gateway result contract")
		}
		// Apply the same redaction at the receiving boundary, including old or
		// independently built test clients.
		clean, limited := gatewayResultHeaders(http.Header(result.Headers))
		result.Headers = clean
		result.HeadersLimited = result.HeadersLimited || limited
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read gateway result: %w", err)
	}
	if result == nil {
		return nil, errors.New("structured gateway result unavailable; rebuild the machine controller and rerun")
	}
	return result, nil
}
