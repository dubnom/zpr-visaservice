package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	trustedChangesLookback = "24h"
	maxTrustedChangesBody  = 1 << 20
)

var trustedChangeSourceName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)
var trustedChangeAttributeName = regexp.MustCompile(`^(?:[a-zA-Z][a-zA-Z0-9-]*|[0-9]+(?:\.[0-9]+)+)$`)

type trustedChangeFeedConfig struct {
	Name           string `json:"name"`
	DisplayName    string `json:"display_name,omitempty"`
	URL            string `json:"url"`
	ServerName     string `json:"server_name,omitempty"`
	CAFile         string `json:"ca_file"`
	ClientCertFile string `json:"client_cert_file"`
	ClientKeyFile  string `json:"client_key_file"`
}

type trustedChangeFeed struct {
	name        string
	displayName string
	endpoint    string
	client      *http.Client
}

type trustedSourceChanges struct {
	feeds map[string]trustedChangeFeed
}

type trustedChange struct {
	Time       time.Time `json:"time"`
	Type       string    `json:"type"`
	DN         string    `json:"dn"`
	NewDN      string    `json:"new_dn,omitempty"`
	Attributes []string  `json:"attributes,omitempty"`
}

type trustedChangesResponse struct {
	Changes []trustedChange `json:"changes"`
	Cursor  string          `json:"cursor"`
	More    bool            `json:"more"`
}

type trustedChangeFeedEntry struct {
	Cursor     string    `json:"cursor"`
	Time       time.Time `json:"time"`
	Type       string    `json:"type"`
	DN         string    `json:"dn"`
	NewDN      string    `json:"new_dn,omitempty"`
	EntryUUID  string    `json:"entry_uuid,omitempty"`
	Attributes []string  `json:"attributes,omitempty"`
}

type trustedChangeFeedResponse struct {
	Changes []trustedChangeFeedEntry `json:"changes"`
	Cursor  string                   `json:"cursor"`
	More    bool                     `json:"more"`
}

func newTrustedSourceChanges(filePath string) (*trustedSourceChanges, error) {
	service := &trustedSourceChanges{feeds: map[string]trustedChangeFeed{}}
	if filePath == "" {
		return service, nil
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, errors.New("Cannot open trusted change-feed configuration")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return nil, errors.New("Trusted change-feed configuration exceeds limits")
	}
	var config struct {
		Sources []trustedChangeFeedConfig `json:"sources"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil || decoder.Decode(new(any)) != io.EOF || len(config.Sources) > 16 {
		return nil, errors.New("Invalid trusted change-feed configuration")
	}
	for _, entry := range config.Sources {
		if !trustedChangeSourceName.MatchString(entry.Name) || entry.CAFile == "" || entry.ClientCertFile == "" || entry.ClientKeyFile == "" {
			return nil, errors.New("Trusted change feeds require a unique name and mTLS certificate files")
		}
		if _, exists := service.feeds[entry.Name]; exists {
			return nil, errors.New("Trusted change-feed names must be unique")
		}
		parsed, err := url.Parse(entry.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, errors.New("Trusted change-feed URL must be an HTTPS origin without a path")
		}
		serverName := strings.TrimSpace(entry.ServerName)
		if serverName == "" {
			serverName = parsed.Hostname()
		}
		certificate, err := tls.LoadX509KeyPair(entry.ClientCertFile, entry.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("read trusted change-feed client certificate for %s: %w", entry.Name, err)
		}
		roots, err := loadCertificateAuthorities(entry.CAFile, "trusted change-feed server")
		if err != nil {
			return nil, err
		}
		transport := &http.Transport{
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{certificate}, ServerName: serverName},
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			IdleConnTimeout:       60 * time.Second,
		}
		client := &http.Client{
			Timeout:   15 * time.Second,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("trusted change-feed redirects are not allowed")
			},
		}
		displayName := strings.TrimSpace(entry.DisplayName)
		if displayName == "" {
			displayName = entry.Name
		}
		service.feeds[entry.Name] = trustedChangeFeed{
			name: entry.Name, displayName: displayName, endpoint: strings.TrimRight(entry.URL, "/") + "/v1/changes", client: client,
		}
	}
	return service, nil
}

func (service *trustedSourceChanges) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/trusted-sources/change-feeds", service.listFeeds)
	mux.HandleFunc("GET /api/trusted-sources/change-feeds/{source}/changes", service.readChanges)
}

func (service *trustedSourceChanges) listFeeds(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	type feedSummary struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	names := make([]string, 0, len(service.feeds))
	for name := range service.feeds {
		names = append(names, name)
	}
	sort.Strings(names)
	summaries := make([]feedSummary, 0, len(names))
	for _, name := range names {
		feed := service.feeds[name]
		summaries = append(summaries, feedSummary{Name: feed.name, DisplayName: feed.displayName})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": summaries})
}

func (service *trustedSourceChanges) readChanges(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	sourceName := r.PathValue("source")
	feed, exists := service.feeds[sourceName]
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Trusted source change feed is not configured"})
		return
	}
	query := r.URL.Query()
	for name := range query {
		if name != "cursor" && name != "limit" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid change-feed request"})
			return
		}
	}
	if len(query["cursor"]) > 1 || len(query["limit"]) > 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid change-feed request"})
		return
	}
	limit := 100
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Change-feed limit must be between 1 and 500"})
			return
		}
		limit = parsed
	}
	upstreamQuery := url.Values{"limit": []string{strconv.Itoa(limit)}}
	if cursor := query.Get("cursor"); cursor != "" {
		if len(cursor) > 256 || strings.ContainsAny(cursor, "\r\n") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid change cursor"})
			return
		}
		upstreamQuery.Set("cursor", cursor)
	} else {
		upstreamQuery.Set("since", trustedChangesLookback)
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, feed.endpoint+"?"+upstreamQuery.Encode(), nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Trusted source change feed is unavailable"})
		return
	}
	request.Header.Set("Accept", "application/json")
	response, err := feed.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Trusted source change feed is unavailable"})
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusGone {
			writeJSON(w, http.StatusGone, map[string]string{"error": "cursor_expired"})
			return
		}
		if response.StatusCode == http.StatusBadRequest {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Trusted source rejected the change-feed request"})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Trusted source change feed is unavailable"})
		return
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxTrustedChangesBody+1))
	if err != nil || len(data) > maxTrustedChangesBody {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Trusted source change feed response exceeds limits"})
		return
	}
	var upstream trustedChangeFeedResponse
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&upstream); err != nil || decoder.Decode(new(any)) != io.EOF || !validTrustedChangesResponse(upstream) {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Trusted source change feed response is invalid"})
		return
	}
	result := trustedChangesResponse{Cursor: upstream.Cursor, More: upstream.More, Changes: make([]trustedChange, 0, len(upstream.Changes))}
	for _, change := range upstream.Changes {
		result.Changes = append(result.Changes, trustedChange{
			Time: change.Time, Type: change.Type, DN: change.DN,
			NewDN: change.NewDN, Attributes: append([]string(nil), change.Attributes...),
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func validTrustedChangesResponse(response trustedChangeFeedResponse) bool {
	if len(response.Cursor) == 0 || len(response.Cursor) > 256 || len(response.Changes) > 500 {
		return false
	}
	for _, change := range response.Changes {
		switch change.Type {
		case "add", "delete", "modify", "modrdn":
		default:
			return false
		}
		if change.Cursor == "" || len(change.Cursor) > 256 || change.Time.IsZero() || change.DN == "" || len(change.DN) > 4096 || len(change.NewDN) > 4096 {
			return false
		}
		if len(change.Attributes) > 256 {
			return false
		}
		for _, attribute := range change.Attributes {
			if len(attribute) > 128 || !trustedChangeAttributeName.MatchString(attribute) {
				return false
			}
		}
	}
	return true
}
