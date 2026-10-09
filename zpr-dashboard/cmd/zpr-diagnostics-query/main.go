package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const maxBody = 512 << 10

type configuration struct {
	Upstream         string   `json:"upstream"`
	Tenant           string   `json:"tenant"`
	Organization     string   `json:"organization"`
	Streams          []string `json:"streams"`
	MetricStreams    []string `json:"metric_streams"`
	QueryUsername    string   `json:"query_username"`
	QueryPassword    string   `json:"query_password"`
	UpstreamUsername string   `json:"upstream_username"`
	UpstreamPassword string   `json:"upstream_password"`
}

type searchRequest struct {
	Query struct {
		SQL       string `json:"sql"`
		StartTime int64  `json:"start_time"`
		EndTime   int64  `json:"end_time"`
		From      int    `json:"from"`
		Size      int    `json:"size"`
	} `json:"query"`
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func queryHandler(config configuration, client *http.Client) (http.Handler, error) {
	upstream, err := url.Parse(config.Upstream)
	if err != nil || upstream.Hostname() == "" || upstream.User != nil || upstream.Path != "" ||
		upstream.RawQuery != "" || upstream.Fragment != "" ||
		(upstream.Scheme != "https" && !(upstream.Scheme == "http" && upstream.Hostname() == "zpr-diagnostics-store" && upstream.Port() == "5080")) {
		return nil, errors.New("query proxy requires HTTPS or its isolated private telemetry store")
	}
	if !identifier.MatchString(config.Tenant) || !identifier.MatchString(config.Organization) ||
		len(config.Streams) == 0 || len(config.Streams) > 16 || config.QueryUsername == "" ||
		config.QueryPassword == "" || config.UpstreamUsername == "" || config.UpstreamPassword == "" ||
		config.QueryUsername == config.UpstreamUsername || strings.Contains(config.QueryUsername, ":") {
		return nil, errors.New("query proxy requires separate credentials and valid tenant, organization and streams")
	}
	allowed := map[string]*regexp.Regexp{}
	if len(config.MetricStreams) == 0 || len(config.MetricStreams) > 256 {
		return nil, errors.New("query proxy requires bounded native metric streams")
	}
	for _, stream := range config.MetricStreams {
		if !identifier.MatchString(stream) {
			return nil, errors.New("query proxy metric stream is invalid")
		}
	}
	for _, stream := range config.Streams {
		if !identifier.MatchString(stream) {
			return nil, errors.New("query proxy stream is invalid")
		}
		// Only the Diagnostics source-scoped SELECT contract is accepted.
		sql := `^SELECT \* FROM "` + regexp.QuoteMeta(stream) + `" WHERE (zpr_organization_id = '` +
			regexp.QuoteMeta(config.Organization) + `' AND service_name = '(?:[^'\r\n]|'')+' AND service_instance_id = '(?:[^'\r\n]|'')+') AND (body|name) IS NOT NULL ORDER BY _timestamp DESC$`
		allowed["/api/"+config.Tenant+"/"+stream+"/_search"] = regexp.MustCompile(sql)
	}
	wantUser := sha256.Sum256([]byte(config.QueryUsername))
	wantPassword := sha256.Sum256([]byte(config.QueryPassword))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user, password, ok := r.BasicAuth()
		gotUser := sha256.Sum256([]byte(user))
		gotPassword := sha256.Sum256([]byte(password))
		if !ok || subtle.ConstantTimeCompare(wantUser[:], gotUser[:])&subtle.ConstantTimeCompare(wantPassword[:], gotPassword[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Diagnostics query"`)
			http.Error(w, "query authentication required", http.StatusUnauthorized)
			return
		}
		pattern := allowed[r.URL.Path]
		if r.Method != http.MethodPost || pattern == nil || r.URL.RawQuery != "" || r.URL.EscapedPath() != r.URL.Path {
			http.Error(w, "only configured Diagnostics searches are permitted", http.StatusForbidden)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		var query searchRequest
		if decoder.Decode(&query) != nil || decoder.Decode(new(any)) != io.EOF || !pattern.MatchString(query.Query.SQL) ||
			query.Query.From != 0 || query.Query.Size < 1 || query.Query.Size > 1024 || query.Query.StartTime < 0 ||
			query.Query.EndTime <= query.Query.StartTime || query.Query.EndTime > time.Now().Add(time.Minute).UnixMilli() ||
			query.Query.EndTime-query.Query.StartTime > (24*time.Hour).Milliseconds() {
			http.Error(w, "invalid bounded Diagnostics search", http.StatusBadRequest)
			return
		}
		match := pattern.FindStringSubmatch(query.Query.SQL)
		signal := "logs"
		if match[2] == "name" {
			signal = "metrics"
			selects := make([]string, 0, len(config.MetricStreams))
			for _, stream := range config.MetricStreams {
				selects = append(selects, fmt.Sprintf(`SELECT __name__ AS name, value, _timestamp FROM "%s" WHERE %s`, stream, match[1]))
			}
			query.Query.SQL = strings.Join(selects, " UNION ALL ") + " ORDER BY _timestamp DESC"
		}
		query.Query.StartTime *= 1000
		query.Query.EndTime *= 1000
		body, err := json.Marshal(query)
		if err != nil {
			http.Error(w, "cannot encode query", http.StatusInternalServerError)
			return
		}
		target := *upstream
		target.Path = "/api/" + config.Tenant + "/_search"
		target.RawQuery = "type=" + signal
		request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target.String(), bytes.NewReader(body))
		if err != nil {
			http.Error(w, "cannot construct provider query", http.StatusBadGateway)
			return
		}
		request.SetBasicAuth(config.UpstreamUsername, config.UpstreamPassword)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json")
		response, err := client.Do(request)
		if err != nil {
			log.Print("Diagnostics store query failed")
			http.Error(w, "telemetry store unavailable", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		content, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
		if err != nil || len(content) > maxBody {
			http.Error(w, "telemetry response exceeds limits", http.StatusBadGateway)
			return
		}
		if response.StatusCode != http.StatusOK {
			log.Printf("Diagnostics store returned HTTP %d", response.StatusCode)
			http.Error(w, "telemetry store rejected query", http.StatusBadGateway)
			return
		}
		if !json.Valid(content) {
			http.Error(w, "invalid telemetry response", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}), nil
}

func run() error {
	file := os.Getenv("ZPR_DIAGNOSTICS_QUERY_CONFIG")
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("query configuration must be a private regular file")
	}
	content, err := os.ReadFile(file)
	if err != nil || len(content) > 64<<10 {
		return errors.New("query configuration is unavailable or too large")
	}
	var config configuration
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&config) != nil || decoder.Decode(new(any)) != io.EOF {
		return errors.New("invalid query configuration")
	}
	client := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("telemetry redirects are forbidden")
	}}
	handler, err := queryHandler(config, client)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: ":5080", Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	log.Print("Private Diagnostics query proxy listening on :5080")
	return server.ListenAndServe()
}

func main() {
	if err := run(); err != nil {
		log.Fatal(fmt.Errorf("Diagnostics query proxy: %w", err))
	}
}
