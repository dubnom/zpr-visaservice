package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxDiagnosticsSources      = 100
	maxDiagnosticsLogs         = 50
	maxDiagnosticsMetrics      = 50
	maxDiagnosticsTotalLogs    = 500
	maxDiagnosticsTotalMetrics = 1000
	maxDiagnosticsQueryBytes   = 512 << 10
)

type diagnosticsProviderConfig struct {
	Endpoint          string `json:"endpoint"`
	Organization      string `json:"organization"`
	LogsStream        string `json:"logs_stream"`
	MetricsStream     string `json:"metrics_stream"`
	StaleAfterSeconds int    `json:"stale_after_seconds"`
}

type diagnosticsOTelIdentity struct {
	ServiceName string `json:"service_name"`
	InstanceID  string `json:"instance_id"`
}

type diagnosticsProvider interface {
	query(context.Context, diagnosticsSource, int) ([]diagnosticsLog, []diagnosticsMetric, error)
}

type openObserveDiagnosticsProvider struct {
	client        *http.Client
	endpoint      *url.URL
	organization  string
	logsStream    string
	metricsStream string
	username      string
	token         string
	staleAfter    time.Duration
}

type diagnosticsSource struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Kind        string              `json:"kind"`
	Identity    string              `json:"identity"`
	Address     string              `json:"address,omitempty"`
	ServiceName string              `json:"service_name"`
	InstanceID  string              `json:"service_instance_id"`
	State       string              `json:"state"`
	LastUpdated *time.Time          `json:"last_updated,omitempty"`
	Logs        []diagnosticsLog    `json:"logs"`
	Metrics     []diagnosticsMetric `json:"metrics"`
	Error       string              `json:"error,omitempty"`
}

type diagnosticsLog struct {
	Timestamp  time.Time      `json:"timestamp"`
	Severity   string         `json:"severity,omitempty"`
	Body       string         `json:"body"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type diagnosticsMetric struct {
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	Unit      string    `json:"unit,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type diagnosticsResponse struct {
	GeneratedAt time.Time           `json:"generated_at"`
	State       string              `json:"state"`
	Error       string              `json:"error,omitempty"`
	Sources     []diagnosticsSource `json:"sources"`
}

type openObserveSearchRequest struct {
	Query struct {
		SQL       string `json:"sql"`
		StartTime int64  `json:"start_time"`
		EndTime   int64  `json:"end_time"`
		From      int    `json:"from"`
		Size      int    `json:"size"`
	} `json:"query"`
}

type openObserveSearchResponse struct {
	Hits []map[string]any `json:"hits"`
}

var diagnosticsStreamPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var diagnosticsOrganizationPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func diagnosticsSourceMappings() map[string]diagnosticsOTelIdentity {
	path := strings.TrimSpace(os.Getenv("ZPR_DIAGNOSTICS_SOURCE_MAP_FILE"))
	if path == "" {
		return nil
	}
	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil || len(content) > 128<<10 {
		return nil
	}
	var mappings map[string]diagnosticsOTelIdentity
	if json.Unmarshal(content, &mappings) != nil {
		return nil
	}
	return mappings
}

func newOpenObserveDiagnosticsProvider() (diagnosticsProvider, time.Duration, string) {
	configPath := strings.TrimSpace(os.Getenv("ZPR_DIAGNOSTICS_CONFIG_FILE"))
	if configPath == "" {
		return nil, 5 * time.Minute, "Diagnostics provider is not configured."
	}
	content, err := os.ReadFile(filepath.Clean(configPath))
	if err != nil || len(content) > 64<<10 {
		return nil, 5 * time.Minute, "Diagnostics provider configuration is unavailable."
	}
	var config diagnosticsProviderConfig
	if json.Unmarshal(content, &config) != nil {
		return nil, 5 * time.Minute, "Diagnostics provider configuration is invalid."
	}
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, 5 * time.Minute, "Diagnostics provider endpoint must be an origin without credentials or path."
	}
	if endpoint.Scheme != "https" {
		address := net.ParseIP(endpoint.Hostname())
		if endpoint.Scheme != "http" || address == nil || !address.IsLoopback() {
			return nil, 5 * time.Minute, "Diagnostics provider endpoint must use HTTPS or HTTP loopback."
		}
	}
	if !diagnosticsOrganizationPattern.MatchString(config.Organization) || !diagnosticsStreamPattern.MatchString(config.LogsStream) || !diagnosticsStreamPattern.MatchString(config.MetricsStream) {
		return nil, 5 * time.Minute, "Diagnostics provider organization and valid stream names are required."
	}
	username := strings.TrimSpace(os.Getenv("ZPR_DIAGNOSTICS_USERNAME"))
	tokenPath := strings.TrimSpace(os.Getenv("ZPR_DIAGNOSTICS_TOKEN_FILE"))
	if username == "" || tokenPath == "" {
		return nil, 5 * time.Minute, "Diagnostics provider credentials are unavailable."
	}
	info, err := os.Stat(filepath.Clean(tokenPath))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, 5 * time.Minute, "Diagnostics provider token file must be a private regular file."
	}
	token, err := os.ReadFile(filepath.Clean(tokenPath))
	if err != nil || strings.TrimSpace(string(token)) == "" || len(token) > 16<<10 {
		return nil, 5 * time.Minute, "Diagnostics provider credentials are unavailable."
	}
	staleAfter := config.StaleAfterSeconds
	if staleAfter < 30 || staleAfter > 86400 {
		staleAfter = 300
	}
	return &openObserveDiagnosticsProvider{
		client: &http.Client{Timeout: 12 * time.Second}, endpoint: endpoint,
		organization: config.Organization, logsStream: config.LogsStream, metricsStream: config.MetricsStream,
		username: username, token: strings.TrimSpace(string(token)), staleAfter: time.Duration(staleAfter) * time.Second,
	}, time.Duration(staleAfter) * time.Second, ""
}

func diagnosticsSourcesFromSnapshot(data snapshot, mappings map[string]diagnosticsOTelIdentity) []diagnosticsSource {
	sources := make([]diagnosticsSource, 0, len(data.Actors)+len(data.Services)+len(data.Trusted))
	appendSource := func(id, name, kind, identity, address, serviceName string) {
		mapping := mappings[id]
		serviceName = mapping.ServiceName
		instanceID := mapping.InstanceID
		if serviceName == "" {
			serviceName = identity
		}
		if instanceID == "" {
			instanceID = identity
		}
		sources = append(sources, diagnosticsSource{ID: id, Name: name, Kind: kind, Identity: identity, Address: address,
			ServiceName: serviceName, InstanceID: instanceID, State: "unavailable", Logs: []diagnosticsLog{}, Metrics: []diagnosticsMetric{}})
	}
	for _, actor := range data.Actors {
		if actor.Node {
			appendSource("node:"+actor.CN, actor.CN, "ZPR node", actor.CN, actor.ZPRAddress, "zpr-core")
		}
	}
	for _, service := range data.Services {
		if service.Name == "" {
			continue
		}
		appendSource("service:"+service.Name, service.Name, "Required service · "+service.Kind, service.ActorCN, service.Address, service.ActorCN)
	}
	for _, trusted := range data.Trusted {
		identity := trusted.ActorCN
		if identity == "" {
			identity = trusted.Name
		}
		appendSource("trusted:"+trusted.Name, trusted.Name, "Trusted service · "+trusted.Provider, identity, trusted.ZPRAddress, trusted.Name)
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Kind != sources[j].Kind {
			return sources[i].Kind < sources[j].Kind
		}
		return sources[i].Name < sources[j].Name
	})
	if len(sources) > maxDiagnosticsSources {
		sources = sources[:maxDiagnosticsSources]
	}
	return sources
}

func (provider *openObserveDiagnosticsProvider) query(ctx context.Context, source diagnosticsSource, limit int) ([]diagnosticsLog, []diagnosticsMetric, error) {
	end := time.Now()
	start := end.Add(-time.Hour)
	logHits, logsErr := provider.search(ctx, provider.logsStream, source, start, end, limit, "logs")
	parsedLogs := make([]diagnosticsLog, 0, len(logHits))
	for _, hit := range logHits {
		timestamp := diagnosticsHitTime(hit)
		body := diagnosticsHitString(hit, "body", "message", "log", "_all")
		if body == "" {
			encoded, _ := json.Marshal(hit)
			body = string(encoded)
		}
		if len(body) > 2048 {
			body = body[:2048] + "…"
		}
		parsedLogs = append(parsedLogs, diagnosticsLog{Timestamp: timestamp, Severity: diagnosticsHitString(hit, "severity_text", "severity", "level"), Body: machineLogSecrets.ReplaceAllString(body, "[REDACTED]"), Attributes: diagnosticsSafeAttributes(hit)})
	}
	metricHits, metricsErr := provider.search(ctx, provider.metricsStream, source, start, end, maxDiagnosticsMetrics, "metrics")
	parsedMetrics := make([]diagnosticsMetric, 0, len(metricHits))
	for _, hit := range metricHits {
		name := diagnosticsHitString(hit, "name", "metric_name", "metric.name")
		value := diagnosticsHitString(hit, "value", "as_double", "as_int", "metric_value")
		if name == "" || value == "" {
			continue
		}
		parsedMetrics = append(parsedMetrics, diagnosticsMetric{Name: name, Value: value, Unit: diagnosticsHitString(hit, "unit"), Timestamp: diagnosticsHitTime(hit)})
	}
	sort.Slice(parsedLogs, func(i, j int) bool { return parsedLogs[i].Timestamp.After(parsedLogs[j].Timestamp) })
	sort.Slice(parsedMetrics, func(i, j int) bool { return parsedMetrics[i].Timestamp.After(parsedMetrics[j].Timestamp) })
	if len(parsedLogs) > limit {
		parsedLogs = parsedLogs[:limit]
	}
	return parsedLogs, parsedMetrics, errors.Join(logsErr, metricsErr)
}

func (provider *openObserveDiagnosticsProvider) search(ctx context.Context, stream string, source diagnosticsSource, start, end time.Time, limit int, signal string) ([]map[string]any, error) {
	quote := func(value string) string { return strings.ReplaceAll(value, "'", "''") }
	query := openObserveSearchRequest{}
	signalFilter := "body IS NOT NULL"
	if signal == "metrics" {
		signalFilter = "name IS NOT NULL"
	}
	query.Query.SQL = fmt.Sprintf(`SELECT * FROM "%s" WHERE service_name = '%s' AND service_instance_id = '%s' AND %s ORDER BY _timestamp DESC`, stream, quote(source.ServiceName), quote(source.InstanceID), signalFilter)
	query.Query.StartTime = start.UnixMilli()
	query.Query.EndTime = end.UnixMilli()
	query.Query.Size = limit
	body, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	target := *provider.endpoint
	target.Path = "/api/" + url.PathEscape(provider.organization) + "/" + url.PathEscape(stream) + "/_search"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(provider.username+":"+provider.token)))
	response, err := provider.client.Do(request)
	if err != nil {
		return nil, errors.New("telemetry provider is unavailable")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxDiagnosticsQueryBytes+1))
	if err != nil || len(data) > maxDiagnosticsQueryBytes {
		return nil, errors.New("telemetry provider response exceeded limits")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("telemetry provider returned HTTP %d", response.StatusCode)
	}
	var result openObserveSearchResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errors.New("telemetry provider returned invalid query data")
	}
	return result.Hits, nil
}

func diagnosticsHitString(hit map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := hit[key]; ok && value != nil {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func diagnosticsHitTime(hit map[string]any) time.Time {
	for _, key := range []string{"_timestamp", "time_unix_nano", "time", "timestamp"} {
		value, ok := hit[key]
		if !ok || value == nil {
			continue
		}
		if text, ok := value.(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
				return parsed.UTC()
			}
		}
		if number, err := strconv.ParseFloat(fmt.Sprint(value), 64); err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) {
			switch {
			case number > 1e17:
				return time.Unix(0, int64(number)).UTC()
			case number > 1e14:
				return time.UnixMicro(int64(number)).UTC()
			case number > 1e11:
				return time.UnixMilli(int64(number)).UTC()
			default:
				return time.Unix(int64(number), 0).UTC()
			}
		}
	}
	return time.Time{}
}

func truncateDiagnostics(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

func diagnosticsSafeAttributes(hit map[string]any) map[string]any {
	attributes := map[string]any{}
	for key, value := range hit {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "credential") || strings.Contains(lower, "private_key") || strings.Contains(lower, "authorization") {
			continue
		}
		if len(attributes) >= 8 {
			break
		}
		switch typed := value.(type) {
		case nil, bool, float64:
			attributes[key] = value
		case string:
			attributes[key] = truncateDiagnostics(typed, 128)
		default:
			encoded, err := json.Marshal(value)
			if err == nil {
				attributes[key] = truncateDiagnostics(string(encoded), 128)
			}
		}
	}
	return attributes
}

func newDiagnosticsHandler(snapshotFn func(context.Context) snapshot, provider diagnosticsProvider, providerError string, staleAfter time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writePolicyError(w, http.StatusMethodNotAllowed, "Diagnostics are read-only.")
			return
		}
		search := strings.TrimSpace(r.URL.Query().Get("search"))
		if len(search) > 200 {
			writePolicyError(w, http.StatusBadRequest, "Diagnostics search is limited to 200 characters.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		snapshot := snapshotFn(ctx)
		mappings := diagnosticsSourceMappings()
		sources := diagnosticsSourcesFromSnapshot(snapshot, mappings)
		response := diagnosticsResponse{GeneratedAt: time.Now().UTC(), State: "available", Sources: sources}
		if len(response.Sources) == 0 {
			response.State = "unavailable"
			response.Error = "No nodes or configured services were returned by the Visa Service."
		}
		if provider == nil {
			response.State = "unavailable"
			response.Error = providerError
			for index := range response.Sources {
				response.Sources[index].Error = providerError
			}
			writeJSON(w, http.StatusOK, response)
			return
		}
		if staleAfter <= 0 {
			staleAfter = 5 * time.Minute
		}
		workers := make(chan struct{}, 4)
		var wait sync.WaitGroup
		for index := range response.Sources {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				select {
				case workers <- struct{}{}:
					defer func() { <-workers }()
				case <-ctx.Done():
					response.Sources[index].Error = "Diagnostics query timed out"
					return
				}
				logs, metrics, err := provider.query(ctx, response.Sources[index], maxDiagnosticsLogs)
				response.Sources[index].Logs = logs
				response.Sources[index].Metrics = metrics
				if err != nil {
					response.Sources[index].Error = err.Error()
				}
				latest := time.Time{}
				for _, item := range logs {
					if item.Timestamp.After(latest) {
						latest = item.Timestamp
					}
				}
				for _, item := range metrics {
					if item.Timestamp.After(latest) {
						latest = item.Timestamp
					}
				}
				if !latest.IsZero() {
					response.Sources[index].LastUpdated = &latest
				}
				if latest.IsZero() {
					response.Sources[index].State = "unavailable"
					if response.Sources[index].Error == "" {
						response.Sources[index].Error = "No OpenTelemetry signals received for this source"
					}
				} else if response.Sources[index].Error != "" {
					response.Sources[index].State = "partial"
				} else {
					if time.Since(latest) > staleAfter {
						response.Sources[index].State = "stale"
					} else {
						response.Sources[index].State = "available"
					}
				}
			}(index)
		}
		wait.Wait()
		remainingLogs := maxDiagnosticsTotalLogs
		for index := range response.Sources {
			if len(response.Sources[index].Logs) > remainingLogs {
				response.Sources[index].Logs = response.Sources[index].Logs[:remainingLogs]
			}
			remainingLogs -= len(response.Sources[index].Logs)
			if remainingLogs == 0 {
				for later := index + 1; later < len(response.Sources); later++ {
					response.Sources[later].Logs = []diagnosticsLog{}
				}
				break
			}
		}
		remainingMetrics := maxDiagnosticsTotalMetrics
		for index := range response.Sources {
			if len(response.Sources[index].Metrics) > remainingMetrics {
				response.Sources[index].Metrics = response.Sources[index].Metrics[:remainingMetrics]
			}
			remainingMetrics -= len(response.Sources[index].Metrics)
			if remainingMetrics == 0 {
				for later := index + 1; later < len(response.Sources); later++ {
					response.Sources[later].Metrics = []diagnosticsMetric{}
				}
				break
			}
		}
		for index := range response.Sources {
			for metricIndex := range response.Sources[index].Metrics {
				metric := &response.Sources[index].Metrics[metricIndex]
				metric.Name = truncateDiagnostics(metric.Name, 128)
				metric.Value = truncateDiagnostics(metric.Value, 128)
				metric.Unit = truncateDiagnostics(metric.Unit, 32)
			}
		}
		if search != "" {
			needle := strings.ToLower(search)
			for index := range response.Sources {
				filtered := response.Sources[index].Logs[:0]
				for _, item := range response.Sources[index].Logs {
					if strings.Contains(strings.ToLower(item.Body), needle) || strings.Contains(strings.ToLower(response.Sources[index].Name), needle) || strings.Contains(strings.ToLower(response.Sources[index].Identity), needle) {
						filtered = append(filtered, item)
					}
				}
				response.Sources[index].Logs = filtered
			}
		}
		for _, source := range response.Sources {
			if source.Error != "" {
				response.State = "partial"
				break
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, response)
	})
}
