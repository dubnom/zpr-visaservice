package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/controlservice/diagnostics"
)

const (
	maxDiagnosticsSources      = diagnostics.MaxSources
	maxDiagnosticsLogs         = diagnostics.MaxLogs
	maxDiagnosticsMetrics      = diagnostics.MaxMetrics
	maxDiagnosticsTotalLogs    = diagnostics.MaxTotalLogs
	maxDiagnosticsTotalMetrics = diagnostics.MaxTotalMetrics
	maxDiagnosticsQueryBytes   = diagnostics.MaxQueryBytes
)

type diagnosticsProviderConfig struct {
	Endpoint          string `json:"endpoint"`
	Organization      string `json:"organization"`
	ZPROrganizationID string `json:"zpr_organization_id"`
	LogsStream        string `json:"logs_stream"`
	MetricsStream     string `json:"metrics_stream"`
	StaleAfterSeconds int    `json:"stale_after_seconds"`
}

type diagnosticsOTelIdentity = diagnostics.OTelIdentity

type diagnosticsProvider interface {
	query(context.Context, diagnosticsSource, int) ([]diagnosticsLog, []diagnosticsMetric, error)
}

type diagnosticsProviderAdapter struct {
	diagnosticsProvider
}

func (provider diagnosticsProviderAdapter) Query(ctx context.Context, source diagnostics.Source, limit int) ([]diagnostics.Log, []diagnostics.Metric, error) {
	return provider.query(ctx, source, limit)
}

type openObserveDiagnosticsProvider struct {
	client            *http.Client
	endpoint          *url.URL
	organization      string
	zprOrganizationID string
	logsStream        string
	metricsStream     string
	username          string
	token             string
	staleAfter        time.Duration
}

type diagnosticsSource = diagnostics.Source
type diagnosticsLog = diagnostics.Log
type diagnosticsMetric = diagnostics.Metric
type diagnosticsResponse = diagnostics.Response
type openObserveSearchRequest = diagnostics.SearchRequest

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
	if !validDiagnosticsEndpoint(endpoint) {
		return nil, 5 * time.Minute, "Diagnostics provider endpoint must use HTTPS, HTTP loopback, or the private logger network."
	}
	if !diagnosticsOrganizationPattern.MatchString(config.Organization) || !diagnosticsOrganizationPattern.MatchString(config.ZPROrganizationID) || !diagnosticsStreamPattern.MatchString(config.LogsStream) || !diagnosticsStreamPattern.MatchString(config.MetricsStream) {
		return nil, 5 * time.Minute, "Diagnostics provider organizations and valid stream names are required."
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
		organization: config.Organization, zprOrganizationID: config.ZPROrganizationID, logsStream: config.LogsStream, metricsStream: config.MetricsStream,
		username: username, token: strings.TrimSpace(string(token)), staleAfter: time.Duration(staleAfter) * time.Second,
	}, time.Duration(staleAfter) * time.Second, ""
}

func validDiagnosticsEndpoint(endpoint *url.URL) bool {
	if endpoint.Scheme == "https" {
		return true
	}
	if endpoint.Scheme != "http" {
		return false
	}
	hostname := strings.TrimSuffix(strings.ToLower(endpoint.Hostname()), ".")
	address := net.ParseIP(hostname)
	if address != nil && address.IsLoopback() {
		return true
	}
	return hostname == "zpr-observability-local" && endpoint.Port() == "5080"
}

func diagnosticsSourcesFromSnapshot(data snapshot, mappings map[string]diagnosticsOTelIdentity) []diagnosticsSource {
	return diagnosticsSourcesFromSnapshotForSource(data, mappings, "")
}

func diagnosticsSourcesFromSnapshotForSource(data snapshot, mappings map[string]diagnosticsOTelIdentity, sourceID string) []diagnosticsSource {
	return diagnostics.Sources(data, mappings, sourceID)
}

func (provider *openObserveDiagnosticsProvider) query(ctx context.Context, source diagnosticsSource, limit int) ([]diagnosticsLog, []diagnosticsMetric, error) {
	return provider.reader().Query(ctx, source, limit)
}

func (provider *openObserveDiagnosticsProvider) search(ctx context.Context, stream string, source diagnosticsSource, start, end time.Time, limit int, signal string) ([]map[string]any, error) {
	return provider.reader().Search(ctx, stream, source, start, end, limit, signal)
}

func (provider *openObserveDiagnosticsProvider) reader() *diagnostics.OpenObserve {
	return &diagnostics.OpenObserve{
		Client: provider.client, Endpoint: provider.endpoint,
		Organization: provider.organization, ZPROrganizationID: provider.zprOrganizationID,
		LogsStream: provider.logsStream, MetricsStream: provider.metricsStream,
		Username: provider.username, Token: provider.token,
	}
}

func newDiagnosticsHandler(snapshotFn func(context.Context) snapshot, provider diagnosticsProvider, providerError string, staleAfter time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writePolicyError(w, http.StatusMethodNotAllowed, "Diagnostics are read-only.")
			return
		}
		parameters, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			writePolicyError(w, http.StatusBadRequest, "Diagnostics query parameters are invalid.")
			return
		}
		search := strings.TrimSpace(parameters.Get("search"))
		if len(search) > 200 {
			writePolicyError(w, http.StatusBadRequest, "Diagnostics search is limited to 200 characters.")
			return
		}
		sourceID := ""
		if values, present := parameters["source"]; present {
			if len(values) != 1 || strings.TrimSpace(values[0]) == "" || len(values[0]) > 512 {
				writePolicyError(w, http.StatusBadRequest, "Diagnostics source must be one nonempty inventory ID of at most 512 bytes.")
				return
			}
			sourceID = values[0]
		}
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		snapshot := snapshotFn(ctx)
		mappings := diagnosticsSourceMappings()
		sources := diagnosticsSourcesFromSnapshotForSource(snapshot, mappings, sourceID)
		if sourceID != "" && len(sources) == 0 {
			writePolicyError(w, http.StatusNotFound, "Selected diagnostics source is absent from the production inventory.")
			return
		}
		options := diagnostics.AggregateOptions{ProviderError: providerError, StaleAfter: staleAfter, Search: search}
		if provider != nil {
			options.Provider = diagnosticsProviderAdapter{provider}
		}
		response := diagnostics.Aggregate(ctx, sources, options)
		writeJSON(w, http.StatusOK, response)
	})
}
