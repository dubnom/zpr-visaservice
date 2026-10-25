package main

import (
	"context"
	"go/ast"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/controlservice/httpapi"
)

func TestControlHTTPCommandHandlerBindings(t *testing.T) {
	for _, test := range []struct {
		file, function, literal string
		fields                  map[string]string
	}{
		{"control_service.go", "runControlRoom", "roomhttp.Handlers", map[string]string{
			"AuthConfig": "authConfig", "Auth": "authLogin", "ChangeReview": "changeReview",
			"Enrollment":    "operatorEnrollmentProxy(security.auth, signer, proxy)",
			"API":           `security.apiProxy(os.Getenv("ZPR_CONTROL_ROOM_ORGANIZATION_ID"), proxy)`,
			"DNSStylesheet": `security.assetProxy(os.Getenv("ZPR_CONTROL_ROOM_ORGANIZATION_ID"), proxy)`,
			"Static":        "revalidateStatic(http.FileServer(http.FS(staticRoot)))",
		}},
		{"control_service.go", "runControlService", "httpapi.Handlers", map[string]string{
			"Enrollment": "enrollmentAPI", "Gateways": "gatewayAPI",
			"SubjectLinkReviews":      "subjectReviews",
			"BudgetAuthorityCurrent":  "http.HandlerFunc(budgetAuthorityAPI.Current)",
			"BudgetAuthorityOperator": "http.HandlerFunc(budgetAuthorityAPI.Operator)",
			"Assertions":              "assertions.handlers()", "TrustedChanges": "trustedChanges.handlers()",
			"TrustedSourceControl": "trustedSourceControlHandler(admin)",
			"Snapshot":             "http.HandlerFunc(app.handleSnapshot)",
			"Diagnostics":          "newDiagnosticsHandler(readDiagnosticsSnapshot, diagnostics, diagnosticsErr, diagnosticsStaleAfter)",
			"ActorVisas":           "http.HandlerFunc(app.handleActorVisas)", "AdapterLogs": "newAdapterLogsHandler()",
			"DNSRecords": "newDNSRecordsHandler()", "DNSStats": "newDNSStatsProxy()", "DNSStylesheet": "newDNSStatsAssetProxy()",
			"PolicyAssistant": "http.HandlerFunc(app.handlePolicyAssistant)", "GatewayAssistantStatus": "http.HandlerFunc(app.handleGatewayAssistantStatus)",
			"GatewayAssistant": "http.HandlerFunc(app.handleGatewayAssistant)", "Policy": "app.policyProxyHandler()",
		}},
		{"assertion_runtime.go", "handlers", "httpapi.Assertions", map[string]string{
			"Analyze": "http.HandlerFunc(handleAssertionSourceTools)", "Format": "http.HandlerFunc(handleAssertionSourceTools)",
			"Source": "http.HandlerFunc(runtime.handleSource)", "Status": "http.HandlerFunc(runtime.handleStatus)",
			"Save": "http.HandlerFunc(runtime.handleSave)", "Evaluate": "http.HandlerFunc(runtime.handleEvaluate)",
			"Registry": "http.HandlerFunc(runtime.handleSourceRegistry)",
		}},
		{"trusted_changes.go", "handlers", "httpapi.TrustedChanges", map[string]string{
			"List": "http.HandlerFunc(service.listFeeds)", "Read": "http.HandlerFunc(service.readChanges)",
		}},
	} {
		t.Run(test.literal, func(t *testing.T) {
			set, function := productionCompositionFunction(t, test.file, test.function)
			var got map[string]string
			ast.Inspect(function.Body, func(node ast.Node) bool {
				literal, ok := node.(*ast.CompositeLit)
				if !ok || compatibilityExpression(t, set, literal.Type) != test.literal {
					return true
				}
				if got != nil {
					t.Fatal("multiple handler compositions")
				}
				got = map[string]string{}
				for _, element := range literal.Elts {
					entry := element.(*ast.KeyValueExpr)
					got[compatibilityExpression(t, set, entry.Key)] = compatibilityExpression(t, set, entry.Value)
				}
				return true
			})
			if !reflect.DeepEqual(got, test.fields) {
				t.Fatalf("handler bindings=%v, want %v", got, test.fields)
			}
		})
	}
}

func TestControlHTTPRealDomainHandlersWithoutSimulator(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	t.Setenv("ZPR_CONTROL_ROOM_PROXY_IP", "")
	t.Setenv("ZPR_DIAGNOSTICS_SOURCE_MAP_FILE", "")
	app := &application{configErr: "Admin not configured", policyErr: "Policy not configured"}
	assertions := &assertionRuntime{}
	changes := &trustedSourceChanges{feeds: map[string]trustedChangeFeed{}}
	unavailable := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writePolicyError(w, http.StatusServiceUnavailable, "Not configured")
	})
	mux := securityHeaders(httpapi.NewMux(httpapi.Handlers{
		Enrollment: unavailable, Gateways: unavailable,
		Snapshot: http.HandlerFunc(app.handleSnapshot),
		Diagnostics: newDiagnosticsHandler(func(context.Context) snapshot {
			return snapshot{Actors: []actor{{CN: "production-node", Node: true}}}
		}, nil, "Diagnostics not configured", 0),
		ActorVisas: http.HandlerFunc(app.handleActorVisas), AdapterLogs: unavailable,
		DNSRecords: unavailable, DNSStats: unavailable, DNSStylesheet: unavailable,
		PolicyAssistant: unavailable, GatewayAssistantStatus: unavailable, GatewayAssistant: unavailable,
		Policy: app.policyProxyHandler(), Assertions: assertions.handlers(), TrustedChanges: changes.handlers(),
	}))
	for _, test := range []struct {
		method, path, contains string
		status                 int
	}{
		{"GET", "/api/snapshot", `"api_status":"not configured"`, 200},
		{"GET", "/api/diagnostics", "Diagnostics not configured", 200},
		{"GET", "/api/actors/production-node/visas", "Visa Service unavailable", 503},
		{"GET", "/api/assertions", "Policy Repository assertion settings are unavailable", 502},
		{"GET", "/api/trusted-sources/change-feeds", `"sources":[]`, 200},
		{"GET", "/api/policy", "Policy not configured", 503},
		{"GET", "/api/enrollment/catalog", "Not configured", 503},
		{"GET", "/api/operator-enrollment/catalog", "Not configured", 503},
	} {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			request.Host, request.RemoteAddr = "localhost", "127.0.0.1:54321"
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.contains) || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status=%d body=%s headers=%v", response.Code, response.Body, response.Header())
			}
		})
	}
	request := httptest.NewRequest("GET", "/api/policy", nil)
	request.Host, request.RemoteAddr = "foreign.example", "192.0.2.10:1234"
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("composition bypassed local boundary: %d %s", response.Code, response.Body)
	}
}
