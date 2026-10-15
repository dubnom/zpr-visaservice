package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/gateway"
)

//go:embed static/*
var staticFiles embed.FS

const (
	defaultListen       = "127.0.0.1:8787"
	defaultPollInterval = 5 * time.Second
)

type application struct {
	admin      *adminClient
	configErr  string
	policy     *policyWorkspace
	policyErr  string
	policyAPI  http.Handler
	assistant  *claudeAssistant
	staticRoot http.Handler
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

func main() {
	mode := flag.String("mode", envOr("ZPR_WEB_MODE", "control-room"), "Run mode: control-room, simulator, browser-gateway, control-service, or policy-service")
	listen := flag.String("listen", envOr("ZPR_WEB_LISTEN", defaultListen), "HTTP listen address")
	browserGatewayListen := flag.String("gateway-listen", envOr("ZPR_ACCESS_GATEWAY_LISTEN", defaultBrowserGatewayListen), "Listen address for browser-gateway mode")
	machineID := flag.String("machine-id", "", "Machine identity for machine-controller mode")
	clientID := flag.String("client-id", "", "Client identifier for test-client mode")
	logWorkload := flag.String("log-workload", "", "Workload for test log reader mode")
	businessState := flag.String("business-state-dir", "/var/lib/zpr-business", "Persistent directory for simulated business records")
	testService := flag.String("test-service-name", "", "Expected service for test-client mode")
	gatewayProxy := flag.String("gateway-proxy", "", "IPv6 HTTP proxy address for web-gateway-client mode")
	gatewayURL := flag.String("gateway-url", "", "Page URL for web-gateway-client mode")
	gatewayExpected := flag.String("gateway-expected", "", "Optional web gateway expectation: allow or deny; empty records observations only")
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
		if err := gateway.RunWebService(*listen, *logWorkload, strings.Split(*gatewayAllowedHosts, ",")); err != nil {
			log.Fatal(err)
		}
	case "business-service":
		if err := runBusinessService(*listen, *logWorkload, *businessState); err != nil {
			log.Fatal(err)
		}
	case "business-client":
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := runBusinessClient(ctx, *listen, *zprAddress, *logWorkload); err != nil {
			log.Fatal(err)
		}
	case "business-health":
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := checkBusinessService(ctx, *listen, *logWorkload); err != nil {
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

// Chromium preserves token colors as inline styles while editing contenteditable text.
// Hash only these color declarations, not arbitrary inline styles or scripts.
var codeEditorStyleAttributes = []string{
	"color: rgb(229, 166, 108);",
	"color: rgb(215, 167, 207);",
	"color: rgb(130, 145, 136);",
	"color: rgb(216, 189, 128);",
	"color: rgb(146, 197, 223);",
	"color: rgb(194, 216, 138);",
	"color: rgb(184, 215, 139);",
	"color: rgb(186, 201, 191);",
	"color: rgb(228, 235, 229);",
}

func codeEditorStylePolicy() string {
	policy := "style-src-attr 'unsafe-hashes'"
	for _, declaration := range codeEditorStyleAttributes {
		hash := sha256.Sum256([]byte(declaration))
		policy += " 'sha256-" + base64.StdEncoding.EncodeToString(hash[:]) + "'"
	}
	return policy
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; "+codeEditorStylePolicy()+"; script-src 'self'; connect-src 'self'; img-src 'self' data:")
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
