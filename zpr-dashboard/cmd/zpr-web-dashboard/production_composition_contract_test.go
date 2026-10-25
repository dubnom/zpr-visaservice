package main

import (
	"context"
	"encoding/pem"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func productionCompositionFunction(t *testing.T, path, name string) (*token.FileSet, *ast.FuncDecl) {
	t.Helper()
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok {
			qualified := function.Name.Name
			if function.Recv != nil {
				qualified = compatibilityExpression(t, set, function.Recv.List[0].Type) + "." + qualified
			}
			if function.Name.Name == name || qualified == name {
				return set, function
			}
		}
	}
	t.Fatalf("%s: missing %s", path, name)
	return nil, nil
}

func TestProductionCompositionRouteRegistrations(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	for _, test := range []struct {
		file, function string
		routes         map[string]string
		delegates      []string
	}{
		{"../../internal/controlroom/httpapi/routes.go", "NewMux", map[string]string{
			"GET /auth/operator/config": "handlers.AuthConfig",
			"/auth/operator/":           "handlers.Auth",
			"/api/change-review/":       "handlers.ChangeReview",
			"/api/enrollment/":          "handlers.Enrollment",
			"/api/":                     "handlers.API",
			"GET /bind9.xsl":            "handlers.DNSStylesheet",
			"/":                         "handlers.Static",
		}, nil},
		{"../../internal/controlservice/httpapi/routes.go", "NewMux", map[string]string{
			"/api/enrollment/":                                  "handlers.Enrollment",
			"/api/operator-enrollment/":                         "handlers.Enrollment",
			"POST /api/budget-authority/current":                "budgetCurrent",
			"/api/operator-enrollment/v1/budget-authority/":     "budgetOperator",
			"/api/subject-link-reviews/":                        "reviews",
			"/api/subject-link-registry/":                       "reviews",
			"/api/subject-link-publications/":                   "reviews",
			"GET /api/trusted-sources/runtime":                  "handlers.TrustedSourceControl",
			"GET /api/trusted-sources/runtime/{source}/control": "handlers.TrustedSourceControl",
			"PUT /api/trusted-sources/runtime/{source}/control": "handlers.TrustedSourceControl",
			"/api/gateways/":                                    "handlers.Gateways",
			"GET /api/snapshot":                                 "handlers.Snapshot",
			"GET /api/diagnostics":                              "handlers.Diagnostics",
			"GET /api/actors/{actor}/visas":                     "handlers.ActorVisas",
			"GET /api/adapter-logs":                             "handlers.AdapterLogs",
			"GET /api/dns/records":                              "handlers.DNSRecords",
			"POST /api/policy/assistant":                        "handlers.PolicyAssistant",
			"GET /api/gateways/assistant":                       "handlers.GatewayAssistantStatus",
			"POST /api/gateways/assistant":                      "handlers.GatewayAssistant",
			"/api/dns/stats/":                                   "handlers.DNSStats",
			"GET /bind9.xsl":                                    "handlers.DNSStylesheet",
			"/api/policy":                                       "handlers.Policy",
			"/api/policy/":                                      "handlers.Policy",
		}, []string{"handlers.Assertions.Register(mux)", "handlers.TrustedChanges.Register(mux)"}},
		{"policy_service.go", "policyServiceMux", map[string]string{
			"policyApplyProducerRoute":                          "handler",
			"policyApplyCandidateRoute":                         "handler",
			"GET /api/policy":                                   "app.handleGetPolicy",
			"GET /api/policy/context":                           "inline",
			"GET /api/assertions/settings":                      "app.handleGetAssertionSettings",
			"PUT /api/assertions/settings":                      "app.handleSaveAssertionSettings",
			"POST /api/policy/attributes/rescan":                "app.handleRescanPolicyAttributes",
			"POST /api/policy/test":                             "app.handlePolicyTest",
			"GET /api/policy/test/fixtures":                     "app.handlePolicyTestFixtures",
			"POST /api/policy/records/{id}/stage":               "app.handleStagePolicyRecord",
			"POST /api/policy/check":                            "app.handleCheckPolicy",
			"POST /api/policy/config/check":                     "app.handleCheckConfiguration",
			"POST /api/policy/config/form":                      "app.handleConfigurationForm",
			"POST /api/policy/categories":                       "app.handleCreatePolicyCategory",
			"POST /api/policy/records":                          "app.handleCreatePolicyRecord",
			"POST /api/policy/records/{id}/duplicate":           "app.handleDuplicatePolicyRecord",
			"POST /api/policy/records/{id}/rename":              "app.handleRenamePolicyRecord",
			"DELETE /api/policy/records/{id}":                   "app.handleArchivePolicyRecord",
			"POST /api/policy/records/{id}/restore":             "app.handleArchivePolicyRecord",
			"GET /api/policy/records/{id}":                      "app.handleGetPolicyRecord",
			"GET /api/policy/records/{id}/revisions":            "app.handlePolicyRecordRevisions",
			"POST /api/policy/records/{id}/revisions":           "app.handlePolicyRecordRevisions",
			"GET /api/policy/records/{id}/revisions/{revision}": "app.handleGetPolicyRevision",
		}, nil},
		{"../../internal/controlservice/httpapi/routes.go", "Assertions.Register", map[string]string{
			"POST /api/assertions/analyze":      "handlers.Analyze",
			"POST /api/assertions/format":       "handlers.Format",
			"GET /api/assertions/source":        "handlers.Source",
			"GET /api/assertions/sources":       "handlers.Registry",
			"POST /api/assertions/sources":      "handlers.Registry",
			"PUT /api/assertions/sources":       "handlers.Registry",
			"DELETE /api/assertions/sources":    "handlers.Registry",
			"POST /api/assertions/sources/test": "handlers.Registry",
			"GET /api/assertions":               "handlers.Status",
			"PUT /api/assertions":               "handlers.Save",
			"POST /api/assertions/evaluate":     "handlers.Evaluate",
		}, nil},
		{"../../internal/controlservice/httpapi/routes.go", "TrustedChanges.Register", map[string]string{
			"GET /api/trusted-sources/change-feeds":                  "handlers.List",
			"GET /api/trusted-sources/change-feeds/{source}/changes": "handlers.Read",
		}, nil},
		{"enrollment.go", "configuredEnrollmentHandler", map[string]string{
			"enrollment.APIPrefix":      "handler",
			"operatordelegation.Prefix": "operator",
		}, nil},
	} {
		t.Run(test.function+"/"+test.file, func(t *testing.T) {
			set, function := productionCompositionFunction(t, test.file, test.function)
			routes := map[string]string{}
			var delegates []string
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				target, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if target.Sel.Name == "register" || target.Sel.Name == "Register" {
					delegates = append(delegates, compatibilityExpression(t, set, call))
				}
				if compatibilityExpression(t, set, target.X) != "mux" || (target.Sel.Name != "Handle" && target.Sel.Name != "HandleFunc") {
					return true
				}
				pattern := compatibilityExpression(t, set, call.Args[0])
				if literal, ok := call.Args[0].(*ast.BasicLit); ok && literal.Kind == token.STRING {
					var err error
					pattern, err = strconv.Unquote(literal.Value)
					if err != nil {
						t.Fatal(err)
					}
				}
				handler := "inline"
				if _, ok := call.Args[1].(*ast.FuncLit); !ok {
					handler = compatibilityExpression(t, set, call.Args[1])
				}
				if _, duplicate := routes[pattern]; duplicate {
					t.Errorf("duplicate registration %s", pattern)
				}
				routes[pattern] = handler
				return true
			})
			if !reflect.DeepEqual(routes, test.routes) || !slices.Equal(delegates, test.delegates) {
				t.Fatalf("review production route ownership before moving:\nroutes=%v\ndelegates=%v", routes, delegates)
			}
		})
	}
}

func TestProductionCompositionStartupFailFastWithoutSimulator(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	directory := t.TempDir()
	ca, _, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(directory, "clients.crt")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(directory, "missing")
	for _, key := range []string{
		"ZPR_CONTROL_SERVICE_CERT_FILE", "ZPR_CONTROL_SERVICE_KEY_FILE", "ZPR_CONTROL_SERVICE_CLIENT_CA_FILE",
		"ZPR_POLICY_SERVICE_CERT_FILE", "ZPR_POLICY_SERVICE_KEY_FILE", "ZPR_POLICY_SERVICE_CLIENT_CA_FILE",
		"ZPR_ENROLLMENT_SERVICE_URL", "ZPR_ENROLLMENT_CLIENT_CERT_FILE", "ZPR_ENROLLMENT_CLIENT_KEY_FILE", "ZPR_ENROLLMENT_SERVICE_CA_FILE", "ZPR_ENROLLMENT_SERVICE_TLS_SERVER_NAME",
		"ZPR_ENROLLMENT_CONFIG_FILE", "ZPR_ENROLLMENT_DATABASE_FILE", "ZPR_OPERATOR_DELEGATION_TRUST_FILE",
		"ZPR_TRUSTED_CHANGE_FEEDS_FILE", "ZPR_ASSERTION_STORE_FILE", "ZPR_ASSERTION_SOURCES_FILE", "ZPR_ASSERTION_LDAP_CONTAINER",
		"ZPR_ADMIN_URL", "ZPR_ADMIN_API_KEY", "ZPR_ADMIN_KEY_FILE", "ZPR_ADMIN_CA_FILE", "ZPR_ADMIN_SERVER_NAME",
	} {
		t.Setenv(key, "")
	}
	for _, test := range []struct {
		name string
		run  func() error
		env  map[string]string
		want string
	}{
		{"control credentials", runControlService, nil, "control-service mode requires ZPR_CONTROL_SERVICE_CERT_FILE, ZPR_CONTROL_SERVICE_KEY_FILE, and ZPR_CONTROL_SERVICE_CLIENT_CA_FILE"},
		{"policy credentials", runPolicyService, nil, "policy-service mode requires ZPR_POLICY_SERVICE_CERT_FILE, ZPR_POLICY_SERVICE_KEY_FILE, and ZPR_POLICY_SERVICE_CLIENT_CA_FILE"},
		{"control CA before registry", runControlService, map[string]string{
			"ZPR_CONTROL_SERVICE_CERT_FILE": missing, "ZPR_CONTROL_SERVICE_KEY_FILE": missing, "ZPR_CONTROL_SERVICE_CLIENT_CA_FILE": missing,
			"ZPR_ENROLLMENT_CONFIG_FILE": missing,
		}, "read Control Service client CA"},
		{"policy CA before workspace", runPolicyService, map[string]string{
			"ZPR_POLICY_SERVICE_CERT_FILE": missing, "ZPR_POLICY_SERVICE_KEY_FILE": missing, "ZPR_POLICY_SERVICE_CLIENT_CA_FILE": missing,
			"ZPR_POLICY_DB_FILE": filepath.Join(directory, "must-not-create.db"),
		}, "read Policy Service client CA"},
		{"registry before feeds", runControlService, map[string]string{
			"ZPR_CONTROL_SERVICE_CERT_FILE": missing, "ZPR_CONTROL_SERVICE_KEY_FILE": missing, "ZPR_CONTROL_SERVICE_CLIENT_CA_FILE": caPath,
			"ZPR_ENROLLMENT_CONFIG_FILE": missing, "ZPR_TRUSTED_CHANGE_FEEDS_FILE": missing,
		}, "enrollment requires both ZPR_ENROLLMENT_CONFIG_FILE and ZPR_ENROLLMENT_DATABASE_FILE"},
		{"feeds before assertions", runControlService, map[string]string{
			"ZPR_CONTROL_SERVICE_CERT_FILE": missing, "ZPR_CONTROL_SERVICE_KEY_FILE": missing, "ZPR_CONTROL_SERVICE_CLIENT_CA_FILE": caPath,
			"ZPR_TRUSTED_CHANGE_FEEDS_FILE": missing, "ZPR_ASSERTION_SOURCES_FILE": missing,
		}, "Cannot open trusted change-feed configuration"},
		{"assertions before scheduler", runControlService, map[string]string{
			"ZPR_CONTROL_SERVICE_CERT_FILE": missing, "ZPR_CONTROL_SERVICE_KEY_FILE": missing, "ZPR_CONTROL_SERVICE_CLIENT_CA_FILE": caPath,
			"ZPR_ASSERTION_SOURCES_FILE": missing,
		}, "assertion"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for key, value := range test.env {
				t.Setenv(key, value)
			}
			err := test.run()
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("startup error=%v, want %q", err, test.want)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(directory, "must-not-create.db")); !os.IsNotExist(err) {
		t.Fatalf("failed startup touched policy workspace: %v", err)
	}
}

func TestProductionCompositionPolicyContextRefreshWithoutSimulator(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	handler := policyServiceMux(&policyWorkspace{})
	for _, organization := range []string{"production-a", "production-b"} {
		t.Setenv("ZPR_POLICY_ORGANIZATION_ID", organization)
		t.Setenv("ZPR_POLICY_ORGANIZATION_NAME", "Operator configured")
		request := httptest.NewRequest(http.MethodGet, "/api/policy/context", nil)
		request.Host, request.RemoteAddr = "localhost", "127.0.0.1:23456"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || recorder.Body.String() != `{"organization_id":"`+organization+`","organization_name":"Operator configured"}`+"\n" || recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("context=%d %s headers=%v", recorder.Code, recorder.Body, recorder.Header())
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/policy/context", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method changed=%d", recorder.Code)
	}
}

func TestProductionCompositionControlRoomMissingTransportBeforeReview(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	for _, key := range []string{
		"ZPR_CONTROL_ROOM_CERT_FILE", "ZPR_CONTROL_ROOM_KEY_FILE", "ZPR_CONTROL_ROOM_ORIGIN",
		"ZPR_OPERATOR_OIDC_CONFIG_FILE", "ZPR_OPERATOR_OIDC_SECRET_FILE", "ZPR_OPERATOR_OIDC_CA_FILE",
		"ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP", "ZPR_CONTROL_ROOM_MANAGEMENT_IP", "ZPR_CONTROL_ROOM_ORGANIZATION_ID",
		"ZPR_OPERATOR_DELEGATION_SIGNER_FILE", "ZPR_OPERATOR_DELEGATION_KEY_FILE",
		"ZPR_CONTROL_SERVICE_URL", "ZPR_CONTROL_CLIENT_CERT_FILE", "ZPR_CONTROL_CLIENT_KEY_FILE", "ZPR_CONTROL_SERVICE_CA_FILE",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("ZPR_CHANGE_REVIEW_CONFIG_FILE", filepath.Join(t.TempDir(), "missing"))
	security, err := configuredControlRoomSecurity(context.Background(), "127.0.0.1:8787")
	if err != nil || security == nil {
		t.Fatalf("development security=%+v %v", security, err)
	}
	defer security.close()
	if security.auth != nil || security.tls != nil || security.management {
		t.Fatalf("Simulator configuration enabled production auth: %+v", security)
	}
	if err := runControlRoom("127.0.0.1:8787"); err == nil || err.Error() != "Configure ZPR_CONTROL_SERVICE_URL, ZPR_CONTROL_CLIENT_CERT_FILE, ZPR_CONTROL_CLIENT_KEY_FILE, and ZPR_CONTROL_SERVICE_CA_FILE." {
		t.Fatalf("transport must fail before review/start listener: %v", err)
	}
}

func TestProductionCompositionServerBoundaries(t *testing.T) {
	for _, test := range []struct {
		file, function string
		fields         map[string]string
		serve          []string
	}{
		{"control_service.go", "runControlRoom", map[string]string{
			"Addr": "listen", "Handler": "securityHeaders(security.protect(controlRoomMux))",
			"ReadHeaderTimeout": "5 * time.Second", "IdleTimeout": "60 * time.Second",
			"TLSConfig": "security.tls", "MaxHeaderBytes": "32768",
		}, []string{`server.ListenAndServeTLS("", "")`, "server.ListenAndServe()"}},
		{"control_service.go", "runControlService", map[string]string{
			"Addr": "listen", "Handler": "securityHeaders(mux)",
			"ReadHeaderTimeout": "5 * time.Second", "IdleTimeout": "60 * time.Second",
			"TLSConfig": "&tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientCAs}",
		}, []string{"server.ListenAndServeTLS(certFile, keyFile)"}},
		{"policy_service.go", "runPolicyService", map[string]string{
			"Addr": "listen", "Handler": "policyServiceMux(workspace)", "ReadHeaderTimeout": "5 * time.Second",
			"TLSConfig": "&tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientCAs}",
		}, []string{"server.ListenAndServeTLS(certFile, keyFile)"}},
	} {
		t.Run(test.function, func(t *testing.T) {
			set, function := productionCompositionFunction(t, test.file, test.function)
			var fields map[string]string
			var serve []string
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if literal, ok := node.(*ast.CompositeLit); ok && compatibilityExpression(t, set, literal.Type) == "http.Server" {
					if fields != nil {
						t.Fatal("multiple production servers need a reviewed lifecycle contract")
					}
					fields = map[string]string{}
					for _, element := range literal.Elts {
						entry := element.(*ast.KeyValueExpr)
						fields[compatibilityExpression(t, set, entry.Key)] = compatibilityExpression(t, set, entry.Value)
					}
				}
				if call, ok := node.(*ast.CallExpr); ok {
					target := compatibilityExpression(t, set, call.Fun)
					if target == "server.ListenAndServeTLS" || target == "server.ListenAndServe" {
						serve = append(serve, compatibilityExpression(t, set, call))
					}
				}
				return true
			})
			if !reflect.DeepEqual(fields, test.fields) || !slices.Equal(serve, test.serve) {
				t.Fatalf("server/lifecycle contract changed: fields=%v serve=%v", fields, serve)
			}
		})
	}
}
