//go:build !production

package main

import (
	"context"
	"flag"
	"log"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/gateway"
)

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
