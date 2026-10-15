package diagnostics

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/logview"
)

// OpenObserve receives an operator-validated endpoint, streams and credentials.
// It performs no environment or credential-file discovery.
type OpenObserve struct {
	Client            *http.Client
	Endpoint          *url.URL
	Organization      string
	ZPROrganizationID string
	LogsStream        string
	MetricsStream     string
	Username          string
	Token             string
}

type SearchRequest struct {
	Query struct {
		SQL       string `json:"sql"`
		StartTime int64  `json:"start_time"`
		EndTime   int64  `json:"end_time"`
		From      int    `json:"from"`
		Size      int    `json:"size"`
	} `json:"query"`
}

type searchResponse struct {
	Hits []map[string]any `json:"hits"`
}

func (provider *OpenObserve) Query(ctx context.Context, source Source, limit int) ([]Log, []Metric, error) {
	end := time.Now()
	start := end.Add(-time.Hour)
	logHits, logsErr := provider.Search(ctx, provider.LogsStream, source, start, end, limit, "logs")
	parsedLogs := make([]Log, 0, len(logHits))
	for _, hit := range logHits {
		timestamp := hitTime(hit)
		body := hitString(hit, "body", "message", "log", "_all")
		if body == "" {
			encoded, _ := json.Marshal(hit)
			body = string(encoded)
		}
		if len(body) > 2048 {
			body = body[:2048] + "…"
		}
		parsedLogs = append(parsedLogs, Log{Timestamp: timestamp, Severity: hitString(hit, "severity_text", "severity", "level"), Body: logview.Redact(body), Attributes: safeAttributes(hit)})
	}
	metricLimit := MaxMetrics
	if source.Kind == "ZPR node" {
		metricLimit = 1024
	}
	metricHits, metricsErr := provider.Search(ctx, provider.MetricsStream, source, start, end, metricLimit, "metrics")
	parsedMetrics := make([]Metric, 0, len(metricHits))
	for _, hit := range metricHits {
		name := hitString(hit, "name", "metric_name", "metric.name")
		value := hitString(hit, "value", "as_double", "as_int", "metric_value")
		if name == "" || value == "" {
			continue
		}
		parsedMetrics = append(parsedMetrics, Metric{Name: name, Value: value, Unit: hitString(hit, "unit"), Timestamp: hitTime(hit)})
	}
	sort.Slice(parsedLogs, func(i, j int) bool { return parsedLogs[i].Timestamp.After(parsedLogs[j].Timestamp) })
	sort.Slice(parsedMetrics, func(i, j int) bool { return parsedMetrics[i].Timestamp.After(parsedMetrics[j].Timestamp) })
	if len(parsedLogs) > limit {
		parsedLogs = parsedLogs[:limit]
	}
	return parsedLogs, parsedMetrics, errors.Join(logsErr, metricsErr)
}

func (provider *OpenObserve) Search(ctx context.Context, stream string, source Source, start, end time.Time, limit int, signal string) ([]map[string]any, error) {
	quote := func(value string) string { return strings.ReplaceAll(value, "'", "''") }
	query := SearchRequest{}
	signalFilter := "body IS NOT NULL"
	if signal == "metrics" {
		signalFilter = "name IS NOT NULL"
	}
	query.Query.SQL = fmt.Sprintf(`SELECT * FROM "%s" WHERE zpr_organization_id = '%s' AND service_name = '%s' AND service_instance_id = '%s' AND %s ORDER BY _timestamp DESC`, stream, quote(provider.ZPROrganizationID), quote(source.ServiceName), quote(source.InstanceID), signalFilter)
	query.Query.StartTime = start.UnixMilli()
	query.Query.EndTime = end.UnixMilli()
	query.Query.Size = limit
	body, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	target := *provider.Endpoint
	target.Path = "/api/" + url.PathEscape(provider.Organization) + "/" + url.PathEscape(stream) + "/_search"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(provider.Username+":"+provider.Token)))
	response, err := provider.Client.Do(request)
	if err != nil {
		return nil, errors.New("telemetry provider is unavailable")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxQueryBytes+1))
	if err != nil || len(data) > MaxQueryBytes {
		return nil, errors.New("telemetry provider response exceeded limits")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("telemetry provider returned HTTP %d", response.StatusCode)
	}
	var result searchResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errors.New("telemetry provider returned invalid query data")
	}
	return result.Hits, nil
}

func hitString(hit map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := hit[key]; ok && value != nil {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func hitTime(hit map[string]any) time.Time {
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

func Truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

func safeAttributes(hit map[string]any) map[string]any {
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
			attributes[key] = Truncate(typed, 128)
		default:
			encoded, err := json.Marshal(value)
			if err == nil {
				attributes[key] = Truncate(string(encoded), 128)
			}
		}
	}
	return attributes
}
