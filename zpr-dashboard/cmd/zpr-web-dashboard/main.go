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
	"log"
	"net"
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
	policyAPI  http.Handler
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
	ActiveVisas  []visa            `json:"active_visas"`
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
	Latitude          *float64      `json:"latitude,omitempty"`
	Longitude         *float64      `json:"longitude,omitempty"`
	LastContact       *int64        `json:"last_contact"`
	InSync            bool          `json:"in_sync"`
	PendingInstall    int           `json:"pending_install"`
	PendingRevocation int           `json:"pending_revocation"`
	Adapters          []string      `json:"adapters"`
	Links             []string      `json:"links"`
	Visas             []int64       `json:"visas"`
	VisaRequests      int           `json:"visa_requests"`
	ApprovedRequests  int           `json:"approved_vreqs"`
	DeniedRequests    int           `json:"denied_vreqs"`
	BufferedDenials   *uint64       `json:"buffered_denials"`
	LocalDenials      *uint64       `json:"local_denials"`
	DenialStatsError  string        `json:"denial_stats_error,omitempty"`
	Counters          []nodeCounter `json:"counters"`
	CounterStatsError string        `json:"counter_stats_error,omitempty"`
	CountersUpdatedAt *time.Time    `json:"counters_updated_at,omitempty"`
}

type nodeCounter struct {
	Group string `json:"group"`
	Name  string `json:"name"`
	Value string `json:"value"`
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
	Name                      string `json:"service_name"`
	ActorCN                   string `json:"actor_cn"`
	Address                   string `json:"zpr_addr"`
	Kind                      string `json:"service_kind"`
	Endpoints                 string `json:"service_endpoints"`
	ExternalNetworkConnection string `json:"external_network_connection,omitempty"`
}

func platformServices() []service {
	raw := strings.TrimSpace(os.Getenv("ZPR_PLATFORM_SERVICES"))
	if raw == "" {
		return nil
	}
	var services []service
	if err := json.Unmarshal([]byte(raw), &services); err != nil {
		log.Printf("invalid ZPR_PLATFORM_SERVICES: %v", err)
		return nil
	}
	return services
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

func providerManagerURL(name string) string {
	var managers map[string]string
	if json.Unmarshal([]byte(os.Getenv("ZPR_PROVIDER_MANAGER_URLS")), &managers) != nil {
		return ""
	}
	parsed, err := url.Parse(managers[name])
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost" || parsed.Hostname() == "::1")) {
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
	ID              int64    `json:"id"`
	Created         int64    `json:"created"`
	Expires         int64    `json:"expires"`
	Source          string   `json:"source_addr"`
	Destination     string   `json:"dest_addr"`
	SourcePort      *int     `json:"source_port"`
	DestinationPort *int     `json:"dest_port"`
	Protocol        string   `json:"proto"`
	Direction       string   `json:"direction"`
	RequestingNode  string   `json:"requesting_node"`
	Path            []string `json:"path"`
	PolicyID        string   `json:"policy_id"`
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
	mode := flag.String("mode", envOr("ZPR_WEB_MODE", "control-room"), "Run mode: control-room, simulator, browser-gateway, control-service, or policy-service")
	listen := flag.String("listen", envOr("ZPR_WEB_LISTEN", defaultListen), "HTTP listen address")
	browserGatewayListen := flag.String("gateway-listen", envOr("ZPR_ACCESS_GATEWAY_LISTEN", defaultBrowserGatewayListen), "Listen address for browser-gateway mode")
	machineID := flag.String("machine-id", "", "Machine identity for machine-controller mode")
	clientID := flag.String("client-id", "", "Client identifier for test-client mode")
	logWorkload := flag.String("log-workload", "", "Workload for test log reader mode")
	testService := flag.String("test-service-name", "", "Expected service for test-client mode")
	gatewayProxy := flag.String("gateway-proxy", "", "IPv6 HTTP proxy address for web-gateway-client mode")
	gatewayURL := flag.String("gateway-url", "", "Page URL for web-gateway-client mode")
	gatewayExpected := flag.String("gateway-expected", "", "Expected web gateway result: allow or deny")
	stressServiceCount := flag.Int("service-count", 0, "Number of services for stress-test modes")
	stressBasePort := flag.Int("base-port", 0, "First TCP port for stress-test services")
	stressClientCount := flag.Int("client-count", 0, "Number of logical clients for stress-client mode")
	stressDurationSeconds := flag.Int("duration-seconds", 0, "Stress-client run duration")
	stressRequestDelayMinMS := flag.Int("request-delay-min-ms", 0, "Minimum delay between logical-client requests")
	stressRequestDelayMaxMS := flag.Int("request-delay-max-ms", 0, "Maximum delay between logical-client requests")
	stressRestartMinSeconds := flag.Int("restart-interval-min-seconds", 0, "Minimum time between logical-client restart pauses")
	stressRestartMaxSeconds := flag.Int("restart-interval-max-seconds", 0, "Maximum time between logical-client restart pauses")
	stressRestartPauseMinSeconds := flag.Int("restart-pause-min-seconds", 0, "Minimum logical-client offline duration")
	stressRestartPauseMaxSeconds := flag.Int("restart-pause-max-seconds", 0, "Maximum logical-client offline duration")
	gatewayUpstream := flag.String("gateway-upstream", "", "Fixed HTTPS upstream for the internet-gateway test service")
	gatewayAllowedHosts := flag.String("gateway-allowed-hosts", "", "Comma-separated allowed hosts for the Simulator web gateway")
	controlURL := flag.String("control-url", "", "Simulator mTLS heartbeat URL")
	controlCA := flag.String("control-ca", "", "Simulator control CA certificate")
	clientCert := flag.String("client-cert", "", "Machine controller client certificate")
	clientKey := flag.String("client-key", "", "Machine controller client key")
	zprPH := flag.String("zpr-ph", "", "PH adapter executable for machine controller ZPR link")
	zprBootstrapKey := flag.String("zpr-bootstrap-key", "", "Machine controller ZPR bootstrap private key")
	zprNodeAddress := flag.String("zpr-node-addr", "", "ZPR node substrate address")
	zprAddress := flag.String("zpr-addr", "", "Machine controller ZPR address")
	proxyListen := flag.String("proxy-listen", "", "Listen address for the ZPR machine-control TCP proxy")
	proxyUpstream := flag.String("proxy-upstream", "", "Upstream machine-control TLS listener address")
	policyRoot := flag.String("policy-root", "", "Root directory of policy layer sources and organization profiles")
	policyOrganization := flag.String("policy-organization", "", "Organization policy layer to compose")
	policyOutput := flag.String("policy-output", "", "Output file for the composed policy source")
	policyConfigBase := flag.String("policy-config-base", "", "Organization ZPLC config to merge")
	policyConfigRuntime := flag.String("policy-config-runtime", "", "Generated runtime ZPLC config to overlay")
	policyConfigBootstrapDir := flag.String("policy-config-bootstrap-dir", "", "Directory containing runtime bootstrap public keys")
	assertionsDatabase := flag.String("assertions-db", "", "Organization policy database to populate with default assertions")
	flag.Parse()
	switch *mode {
	case "seed-assertions":
		if err := populateOrganizationAssertions(*policyRoot, *policyOrganization, *assertionsDatabase); err != nil {
			log.Fatal(err)
		}
	case "compose-policy":
		if err := writeOrganizationPolicy(*policyRoot, *policyOrganization, *policyOutput); err != nil {
			log.Fatal(err)
		}
	case "merge-policy-config":
		if err := writeMergedPolicyConfig(*policyConfigBase, *policyConfigRuntime, *policyConfigBootstrapDir, *policyOutput); err != nil {
			log.Fatal(err)
		}
	case "policy-service":
		if err := runPolicyService(); err != nil {
			log.Fatal(err)
		}
	case "control-service":
		if err := runControlService(); err != nil {
			log.Fatal(err)
		}
	case "control-room":
		if err := runControlRoom(*listen); err != nil {
			log.Fatal(err)
		}
	case "simulator":
		if err := runSimulator(*listen); err != nil {
			log.Fatal(err)
		}
	case "browser-gateway":
		if err := runBrowserAccessGateway(*browserGatewayListen); err != nil {
			log.Fatal(err)
		}
	case "machine-controller":
		if err := runMachineController(*machineID, *controlURL, *controlCA, *clientCert, *clientKey, *zprPH, *zprBootstrapKey, *zprNodeAddress, *zprAddress); err != nil {
			log.Fatal(err)
		}
	case "test-service":
		if err := runTestService(*listen, *logWorkload); err != nil {
			log.Fatal(err)
		}
	case "stress-service-fleet":
		if err := runStressServiceFleet(*listen, *logWorkload, *stressServiceCount, *stressBasePort); err != nil {
			log.Fatal(err)
		}
	case "stress-service-health":
		if err := checkStressServiceFleet(*listen, *stressServiceCount, *stressBasePort); err != nil {
			log.Fatal(err)
		}
	case "stress-client":
		if *stressDurationSeconds < 1 || *stressDurationSeconds > 180 {
			log.Fatal("stress-client duration must be between 1 and 180 seconds")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*stressDurationSeconds)*time.Second)
		defer cancel()
		if err := runStressClientFleet(ctx, *listen, *zprAddress, *logWorkload, *stressServiceCount, *stressBasePort, *stressClientCount, *stressRequestDelayMinMS, *stressRequestDelayMaxMS, *stressRestartMinSeconds, *stressRestartMaxSeconds, *stressRestartPauseMinSeconds, *stressRestartPauseMaxSeconds); err != nil {
			log.Fatal(err)
		}
	case "gateway-service":
		if err := runInternetGatewayService(*listen, *logWorkload, *gatewayUpstream); err != nil {
			log.Fatal(err)
		}
	case "web-gateway-service":
		if err := runInternetWebGatewayService(*listen, *logWorkload, strings.Split(*gatewayAllowedHosts, ",")); err != nil {
			log.Fatal(err)
		}
	case "test-client":
		if err := runTestClient(*listen, *zprAddress, *clientID, *logWorkload, *testService); err != nil {
			log.Fatal(err)
		}
	case "web-gateway-client":
		if err := runWebGatewayClient(*gatewayProxy, *gatewayURL, *zprAddress, *clientID, *logWorkload, *gatewayExpected); err != nil {
			log.Fatal(err)
		}
	case "benchmark-client":
		if err := runBenchmarkClient(*listen, *zprAddress); err != nil {
			log.Fatal(err)
		}
	case "test-log-read":
		if err := readTestLogService(*logWorkload); err != nil {
			log.Fatal(err)
		}
	case "machine-control-proxy":
		if err := runMachineControlProxy(*proxyListen, *proxyUpstream); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown run mode %q", *mode)
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

// revalidateStatic makes browsers check embedded UI files on every load. Embedded
// files have no modification time, so without this a deployed build can stay hidden
// behind a cached page that still references old asset versions.
func revalidateStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
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
			items, complete := a.fetchVisaInventory(ctx, entries, &out.Errors, &mu)
			out.RecentVisas = items[:min(len(items), maxRecentVisas)]
			if complete {
				out.ActiveVisas = []visa{}
				now := time.Now().Unix()
				for _, item := range items {
					if item.Expires > now {
						out.ActiveVisas = append(out.ActiveVisas, item)
					}
				}
			}
		case "denies":
			out.RecentDenies = res.value.([]deny)
		}
	}
	if haveStatuses {
		out.Trusted = trustedSourcesFromStatus(statuses, out.Services)
	} else {
		out.Trusted = trustedSourcesFrom(out.Services)
	}
	mergePlatformServices(&out)
	provider, staleAfter, providerError := newNodeDenialProvider()
	populateNodeDenialStats(ctx, &out, provider, staleAfter, providerError, diagnosticsSourceMappings())
	if successfulEndpoints == 0 {
		out.APIStatus = "disconnected"
	} else if len(out.Errors) > 0 {
		out.APIStatus = "partial"
	}
	sort.Slice(out.Errors, func(i, j int) bool { return out.Errors[i] < out.Errors[j] })
	return out
}

func mergePlatformServices(out *snapshot) {
	serviceIndexes := make(map[string]int, len(out.Services))
	for index, item := range out.Services {
		serviceIndexes[item.Name] = index
	}
	for _, item := range platformServices() {
		if item.Name == "" || item.ActorCN == "" {
			continue
		}
		if index, exists := serviceIndexes[item.Name]; exists {
			if strings.EqualFold(item.Kind, "Gateway") || item.ExternalNetworkConnection != "" {
				out.Services[index].Kind = "Gateway"
				out.Services[index].ExternalNetworkConnection = item.ExternalNetworkConnection
			}
			continue
		}
		out.Services = append(out.Services, item)
		serviceIndexes[item.Name] = len(out.Services) - 1
	}
	sort.Slice(out.Services, func(i, j int) bool { return out.Services[i].Name < out.Services[j].Name })
	sort.Slice(out.Actors, func(i, j int) bool { return out.Actors[i].CN < out.Actors[j].CN })
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
				if manager := providerManagerURL(status.Name); manager != "" {
					return manager
				}
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

func (a *application) fetchVisaInventory(ctx context.Context, entries []visaEntry, errs *[]string, mu *sync.Mutex) ([]visa, bool) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID > entries[j].ID })
	items := make([]visa, len(entries))
	valid := make([]bool, len(entries))
	complete := true
	var wg sync.WaitGroup
	workers := make(chan struct{}, 16)
	for i, entry := range entries {
		wg.Add(1)
		go func(i int, id int64) {
			defer wg.Done()
			select {
			case workers <- struct{}{}:
				defer func() { <-workers }()
			case <-ctx.Done():
				mu.Lock()
				complete = false
				*errs = append(*errs, fmt.Sprintf("visa %d: %v", id, ctx.Err()))
				mu.Unlock()
				return
			}
			path := "/admin/visas/" + fmt.Sprint(id)
			if err := a.admin.getJSON(ctx, path, &items[i]); err != nil {
				var statusError adminStatusError
				if errors.As(err, &statusError) && statusError.code == http.StatusNotFound {
					return
				}
				mu.Lock()
				complete = false
				*errs = append(*errs, fmt.Sprintf("visa %d: %v", id, err))
				mu.Unlock()
			} else {
				valid[i] = true
			}
		}(i, entry.ID)
	}
	wg.Wait()
	result := make([]visa, 0, len(items))
	for index, item := range items {
		if valid[index] {
			result = append(result, item)
		}
	}
	return result, complete
}

func (a *application) handleActorVisas(w http.ResponseWriter, r *http.Request) {
	if a.admin == nil {
		http.Error(w, "Visa Service unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var selected actor
	if err := a.admin.getJSON(ctx, "/admin/actors/"+url.PathEscape(r.PathValue("actor")), &selected); err != nil {
		http.Error(w, "adapter details unavailable", http.StatusBadGateway)
		return
	}
	address := net.ParseIP(selected.ZPRAddress)
	if address == nil {
		http.Error(w, "adapter has no valid ZPR address", http.StatusBadGateway)
		return
	}
	var entries []visaEntry
	if err := a.admin.getJSON(ctx, "/admin/visas", &entries); err != nil {
		http.Error(w, "current visas unavailable", http.StatusBadGateway)
		return
	}
	items := make([]visa, 0)
	for _, entry := range entries {
		var item visa
		if err := a.admin.getJSON(ctx, "/admin/visas/"+fmt.Sprint(entry.ID), &item); err != nil {
			var statusError adminStatusError
			if errors.As(err, &statusError) && statusError.code == http.StatusNotFound {
				continue
			}
			http.Error(w, "current visa details unavailable", http.StatusBadGateway)
			return
		}
		if item.Expires > time.Now().Unix() && (address.Equal(net.ParseIP(item.Source)) || address.Equal(net.ParseIP(item.Destination))) {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(left, right int) bool { return items[left].ID > items[right].ID })
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(items)
}
