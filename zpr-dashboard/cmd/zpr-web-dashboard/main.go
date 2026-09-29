package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

const (
	defaultListen       = "127.0.0.1:8787"
	defaultPollInterval = 5 * time.Second
	maxRecentVisas      = 10
	maxRecentDenies     = 20
)

type adminClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type application struct {
	admin      *adminClient
	configErr  string
	policy     *policyWorkspace
	policyErr  string
	assistant  *claudeAssistant
	staticRoot http.Handler
}

type snapshot struct {
	GeneratedAt  time.Time         `json:"generated_at"`
	APIStatus    string            `json:"api_status"`
	ConfigError  string            `json:"config_error,omitempty"`
	Errors       []string          `json:"errors"`
	Stats        map[string]string `json:"stats"`
	Actors       []actor           `json:"actors"`
	Network      []link            `json:"network"`
	Services     []service         `json:"services"`
	Trusted      []trustedSource   `json:"trusted_sources"`
	VisaCount    int               `json:"visa_count"`
	RecentVisas  []visa            `json:"recent_visas"`
	RecentDenies []deny            `json:"recent_denies"`
}

type actorEntry struct {
	CN string `json:"cn"`
}

type actor struct {
	CN          string      `json:"cn"`
	Node        bool        `json:"node"`
	ZPRAddress  string      `json:"zpr_addr"`
	AuthExpires *int64      `json:"auth_exp"`
	NodeDetails *nodeDetail `json:"node_details"`
}

type nodeDetail struct {
	LastContact       *int64   `json:"last_contact"`
	InSync            bool     `json:"in_sync"`
	PendingInstall    int      `json:"pending_install"`
	PendingRevocation int      `json:"pending_revocation"`
	Adapters          []string `json:"adapters"`
	Links             []string `json:"links"`
	Visas             []int64  `json:"visas"`
	VisaRequests      int      `json:"visa_requests"`
	ApprovedRequests  int      `json:"approved_vreqs"`
	DeniedRequests    int      `json:"denied_vreqs"`
}

type link struct {
	NodeA      string `json:"node_a_addr"`
	NodeB      string `json:"node_b_addr"`
	State      string `json:"ctype"`
	SubstrateA string `json:"node_a_substrate"`
	SubstrateB string `json:"node_b_substrate"`
	LinkID     string `json:"link_id"`
	Cost       int    `json:"link_cost"`
}

type serviceEntry struct {
	ID string `json:"id"`
}

type service struct {
	Name      string `json:"service_name"`
	ActorCN   string `json:"actor_cn"`
	Address   string `json:"zpr_addr"`
	Kind      string `json:"service_kind"`
	Endpoints string `json:"service_endpoints"`
}

type trustedSource struct {
	Name          string  `json:"name"`
	ActorCN       string  `json:"actor_cn"`
	Provider      string  `json:"provider"`
	ZPRAddress    string  `json:"zpr_addr,omitempty"`
	Endpoints     string  `json:"service_endpoints,omitempty"`
	Health        string  `json:"health"`
	HealthNote    string  `json:"health_note"`
	LastLookupMS  *uint64 `json:"last_lookup_ms"`
	LastSuccessMS *uint64 `json:"last_success_ms"`
	EditorURL     string  `json:"editor_url,omitempty"`
}

func localLDAPEditorURL() string {
	raw := os.Getenv("ZPR_DEMO_LDAP_EDITOR_URL")
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.Port() == "" || parsed.User != nil || parsed.Fragment != "" {
		return ""
	}
	return parsed.String()
}

type trustedStatus struct {
	Name          string  `json:"name"`
	Health        string  `json:"health"`
	LastLookupMS  *uint64 `json:"last_lookup_ms"`
	LastSuccessMS *uint64 `json:"last_success_ms"`
}

type adminStatusError struct {
	code   int
	status string
}

func (e adminStatusError) Error() string { return "admin API returned " + e.status }

type visaEntry struct {
	ID int64 `json:"id"`
}

type visa struct {
	ID              int64  `json:"id"`
	Created         int64  `json:"created"`
	Expires         int64  `json:"expires"`
	Source          string `json:"source_addr"`
	Destination     string `json:"dest_addr"`
	SourcePort      *int   `json:"source_port"`
	DestinationPort *int   `json:"dest_port"`
	Protocol        string `json:"proto"`
	Direction       string `json:"direction"`
	RequestingNode  string `json:"requesting_node"`
	PolicyID        string `json:"policy_id"`
}

type deny struct {
	Source      string `json:"source_addr"`
	Destination string `json:"dest_addr"`
	Protocol    uint8  `json:"protocol"`
	Port        uint16 `json:"dest_port"`
	Count       uint64 `json:"count"`
	LastDenyMS  uint64 `json:"last_deny_ms"`
	Code        string `json:"deny_code"`
}

type statsResponse struct {
	Stats map[string]string `json:"stats"`
}

func main() {
	listen := flag.String("listen", envOr("ZPR_WEB_LISTEN", defaultListen), "HTTP listen address")
	flag.Parse()

	admin, configErr := newAdminClient()
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	policy, policyErr := newPolicyWorkspace()
	app := &application{
		admin:      admin,
		configErr:  configErr,
		policy:     policy,
		policyErr:  policyErr,
		assistant:  newClaudeAssistant(),
		staticRoot: http.FileServer(http.FS(staticRoot)),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/snapshot", app.handleSnapshot)
	mux.HandleFunc("GET /api/policy", app.handleGetPolicy)
	mux.HandleFunc("POST /api/policy/check", app.handleCheckPolicy)
	mux.HandleFunc("POST /api/policy/categories", app.handleCreatePolicyCategory)
	mux.HandleFunc("POST /api/policy/records", app.handleCreatePolicyRecord)
	mux.HandleFunc("GET /api/policy/records/{id}", app.handleGetPolicyRecord)
	mux.HandleFunc("GET /api/policy/records/{id}/revisions", app.handlePolicyRecordRevisions)
	mux.HandleFunc("POST /api/policy/records/{id}/revisions", app.handlePolicyRecordRevisions)
	mux.HandleFunc("GET /api/policy/records/{id}/revisions/{revision}", app.handleGetPolicyRevision)
	mux.HandleFunc("POST /api/policy/assistant", app.handlePolicyAssistant)
	mux.Handle("GET /", app.staticRoot)

	server := &http.Server{
		Addr:              *listen,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("ZPR web monitor listening at http://%s", *listen)
	if configErr != "" {
		log.Printf("admin API is not configured: %s", configErr)
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func newAdminClient() (*adminClient, string) {
	baseURL := strings.TrimRight(os.Getenv("ZPR_ADMIN_URL"), "/")
	key := strings.TrimSpace(os.Getenv("ZPR_ADMIN_API_KEY"))
	keyFile := os.Getenv("ZPR_ADMIN_KEY_FILE")
	caFile := os.Getenv("ZPR_ADMIN_CA_FILE")
	if baseURL == "" || caFile == "" || (key == "" && keyFile == "") {
		return nil, "set ZPR_ADMIN_URL, ZPR_ADMIN_CA_FILE, and either ZPR_ADMIN_API_KEY or ZPR_ADMIN_KEY_FILE"
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return nil, "ZPR_ADMIN_URL must be a valid https URL"
	}
	if parsedURL.Path != "" && parsedURL.Path != "/" {
		return nil, "ZPR_ADMIN_URL must contain only the scheme and host"
	}
	if key == "" {
		contents, readErr := os.ReadFile(filepath.Clean(keyFile))
		if readErr != nil {
			return nil, fmt.Sprintf("read admin API key file: %v", readErr)
		}
		key = strings.TrimSpace(string(contents))
	}
	if key == "" {
		return nil, "admin API key is empty"
	}

	caPEM, err := os.ReadFile(filepath.Clean(caFile))
	if err != nil {
		return nil, fmt.Sprintf("read admin CA file: %v", err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, "admin CA file contains no certificates"
	}
	serverName := os.Getenv("ZPR_ADMIN_SERVER_NAME")
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		RootCAs:            roots,
		InsecureSkipVerify: true,
	}
	tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("admin server sent no TLS certificate")
		}
		intermediates := x509.NewCertPool()
		for _, cert := range state.PeerCertificates[1:] {
			intermediates.AddCert(cert)
		}
		_, verifyErr := state.PeerCertificates[0].Verify(x509.VerifyOptions{
			Roots:         roots,
			Intermediates: intermediates,
			DNSName:       serverName,
		})
		return verifyErr
	}

	return &adminClient{
		baseURL: baseURL,
		apiKey:  key,
		http: &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, ""
}

func (c *adminClient) getJSON(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return adminStatusError{resp.StatusCode, resp.Status}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(target); err != nil {
		return fmt.Errorf("decode admin response: %w", err)
	}
	return nil
}

func (a *application) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if a.admin == nil {
		_ = json.NewEncoder(w).Encode(snapshot{
			GeneratedAt:  time.Now().UTC(),
			APIStatus:    "not configured",
			ConfigError:  a.configErr,
			Errors:       []string{},
			Stats:        map[string]string{},
			Actors:       []actor{},
			Network:      []link{},
			Services:     []service{},
			Trusted:      []trustedSource{},
			RecentVisas:  []visa{},
			RecentDenies: []deny{},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	data := a.fetchSnapshot(ctx)
	_ = json.NewEncoder(w).Encode(data)
}

func (a *application) fetchSnapshot(ctx context.Context) snapshot {
	out := snapshot{
		GeneratedAt:  time.Now().UTC(),
		APIStatus:    "connected",
		Errors:       []string{},
		Stats:        map[string]string{},
		Actors:       []actor{},
		Network:      []link{},
		Services:     []service{},
		Trusted:      []trustedSource{},
		RecentVisas:  []visa{},
		RecentDenies: []deny{},
	}

	type result struct {
		name  string
		value any
		err   error
	}
	results := make(chan result, 7)
	go func() {
		var value statsResponse
		err := a.admin.getJSON(ctx, "/admin/stats", &value)
		results <- result{"stats", value.Stats, err}
	}()
	go func() {
		var value []actorEntry
		err := a.admin.getJSON(ctx, "/admin/actors", &value)
		results <- result{"actors", value, err}
	}()
	go func() {
		var value struct {
			Network []link `json:"network"`
		}
		err := a.admin.getJSON(ctx, "/admin/network", &value)
		results <- result{"network", value.Network, err}
	}()
	go func() {
		var value []serviceEntry
		err := a.admin.getJSON(ctx, "/admin/services", &value)
		results <- result{"services", value, err}
	}()
	go func() {
		var value []trustedStatus
		err := a.admin.getJSON(ctx, "/admin/trusted-services", &value)
		results <- result{"trusted", value, err}
	}()
	go func() {
		var value []visaEntry
		err := a.admin.getJSON(ctx, "/admin/visas", &value)
		results <- result{"visas", value, err}
	}()
	go func() {
		var value []deny
		err := a.admin.getJSON(ctx, "/admin/visas/denies?limit="+fmt.Sprint(maxRecentDenies), &value)
		results <- result{"denies", value, err}
	}()
	var mu sync.Mutex
	successfulEndpoints := 0
	var statuses []trustedStatus
	haveStatuses := false
	for i := 0; i < 7; i++ {
		res := <-results
		if res.err != nil {
			var statusErr adminStatusError
			if res.name == "trusted" && errors.As(res.err, &statusErr) && statusErr.code == http.StatusNotFound {
				continue
			}
			mu.Lock()
			out.Errors = append(out.Errors, res.name+": "+res.err.Error())
			mu.Unlock()
			continue
		}
		successfulEndpoints++
		switch res.name {
		case "stats":
			out.Stats = res.value.(map[string]string)
		case "actors":
			entries := res.value.([]actorEntry)
			out.Actors = a.fetchActors(ctx, entries, &out.Errors, &mu)
		case "network":
			out.Network = res.value.([]link)
		case "services":
			entries := res.value.([]serviceEntry)
			out.Services = a.fetchServices(ctx, entries, &out.Errors, &mu)
		case "trusted":
			statuses, haveStatuses = res.value.([]trustedStatus), true
		case "visas":
			entries := res.value.([]visaEntry)
			out.VisaCount = len(entries)
			out.RecentVisas = a.fetchRecentVisas(ctx, entries, &out.Errors, &mu)
		case "denies":
			out.RecentDenies = res.value.([]deny)
		}
	}
	if haveStatuses {
		out.Trusted = trustedSourcesFromStatus(statuses, out.Services)
	} else {
		out.Trusted = trustedSourcesFrom(out.Services)
	}
	if successfulEndpoints == 0 {
		out.APIStatus = "disconnected"
	} else if len(out.Errors) > 0 {
		out.APIStatus = "partial"
	}
	sort.Slice(out.Errors, func(i, j int) bool { return out.Errors[i] < out.Errors[j] })
	return out
}

func (a *application) fetchActors(ctx context.Context, entries []actorEntry, errs *[]string, mu *sync.Mutex) []actor {
	items := make([]actor, len(entries))
	var wg sync.WaitGroup
	for i, entry := range entries {
		wg.Add(1)
		go func(i int, cn string) {
			defer wg.Done()
			var value actor
			path := "/admin/actors/" + url.PathEscape(cn)
			if err := a.admin.getJSON(ctx, path, &value); err != nil {
				mu.Lock()
				*errs = append(*errs, "actor "+cn+": "+err.Error())
				mu.Unlock()
				items[i] = actor{CN: cn}
				return
			}
			items[i] = value
		}(i, entry.CN)
	}
	wg.Wait()
	sort.Slice(items, func(i, j int) bool { return items[i].CN < items[j].CN })
	return items
}

func (a *application) fetchServices(ctx context.Context, entries []serviceEntry, errs *[]string, mu *sync.Mutex) []service {
	items := make([]service, len(entries))
	var wg sync.WaitGroup
	for i, entry := range entries {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			path := "/admin/services/" + url.PathEscape(name)
			if err := a.admin.getJSON(ctx, path, &items[i]); err != nil {
				mu.Lock()
				*errs = append(*errs, "service "+name+": "+err.Error())
				mu.Unlock()
			}
		}(i, entry.ID)
	}
	wg.Wait()
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func trustedSourcesFrom(services []service) []trustedSource {
	var trusted []trustedSource
	for _, item := range services {
		if !strings.HasPrefix(item.Kind, "Trusted(") {
			continue
		}
		provider := strings.TrimSuffix(strings.TrimPrefix(item.Kind, "Trusted(\""), "\")")
		if provider == item.Kind {
			provider = "trusted"
		}
		trusted = append(trusted, trustedSource{
			Name: item.Name, ActorCN: item.ActorCN, Provider: provider,
			ZPRAddress: item.Address, Endpoints: item.Endpoints,
			Health: "unreported", HealthNote: "The admin API exposes no live connection or source-health signal.",
		})
	}
	return trusted
}

func trustedSourcesFromStatus(statuses []trustedStatus, services []service) []trustedSource {
	byName := make(map[string]service, len(services))
	for _, item := range services {
		byName[item.Name] = item
	}
	trusted := make([]trustedSource, 0, len(statuses))
	for _, status := range statuses {
		provider := "trusted source"
		actor := ""
		address := ""
		endpoints := ""
		if descriptor, ok := byName[status.Name]; ok {
			actor = descriptor.ActorCN
			address = descriptor.Address
			endpoints = descriptor.Endpoints
			provider = strings.TrimSuffix(strings.TrimPrefix(descriptor.Kind, "Trusted(\""), "\")")
			if provider == descriptor.Kind {
				provider = "trusted source"
			}
		}
		health := status.Health
		if health != "working" && health != "failed" {
			health = "unverified"
		}
		trusted = append(trusted, trustedSource{
			Name: status.Name, ActorCN: actor, Provider: provider, Health: health,
			ZPRAddress: address, Endpoints: endpoints,
			HealthNote:   "Based on the most recent real attribute lookup, not a live probe.",
			LastLookupMS: status.LastLookupMS, LastSuccessMS: status.LastSuccessMS,
			EditorURL: func() string {
				if status.Name == "demo_ldap" && provider == "rest/1" {
					return localLDAPEditorURL()
				}
				return ""
			}(),
		})
	}
	sort.Slice(trusted, func(i, j int) bool { return trusted[i].Name < trusted[j].Name })
	return trusted
}

func (a *application) fetchRecentVisas(ctx context.Context, entries []visaEntry, errs *[]string, mu *sync.Mutex) []visa {
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID > entries[j].ID })
	if len(entries) > maxRecentVisas {
		entries = entries[:maxRecentVisas]
	}
	items := make([]visa, len(entries))
	var wg sync.WaitGroup
	for i, entry := range entries {
		wg.Add(1)
		go func(i int, id int64) {
			defer wg.Done()
			path := "/admin/visas/" + fmt.Sprint(id)
			if err := a.admin.getJSON(ctx, path, &items[i]); err != nil {
				mu.Lock()
				*errs = append(*errs, fmt.Sprintf("visa %d: %v", id, err))
				mu.Unlock()
			}
		}(i, entry.ID)
	}
	wg.Wait()
	return items
}
