// Package httpapi owns production Control-Service route composition.
package httpapi

import "net/http"

type Assertions struct {
	Analyze, Format, Source, Status, Save, Evaluate http.Handler
	Registry                                        http.Handler
}

func (handlers Assertions) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/assertions/analyze", handlers.Analyze)
	mux.Handle("POST /api/assertions/format", handlers.Format)
	mux.Handle("GET /api/assertions/source", handlers.Source)
	if handlers.Registry != nil {
		mux.Handle("GET /api/assertions/sources", handlers.Registry)
		mux.Handle("POST /api/assertions/sources", handlers.Registry)
		mux.Handle("PUT /api/assertions/sources", handlers.Registry)
		mux.Handle("DELETE /api/assertions/sources", handlers.Registry)
		mux.Handle("POST /api/assertions/sources/test", handlers.Registry)
	}
	mux.Handle("GET /api/assertions", handlers.Status)
	mux.Handle("PUT /api/assertions", handlers.Save)
	mux.Handle("POST /api/assertions/evaluate", handlers.Evaluate)
}

type TrustedChanges struct {
	List, Read http.Handler
}

func (handlers TrustedChanges) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/trusted-sources/change-feeds", handlers.List)
	mux.Handle("GET /api/trusted-sources/change-feeds/{source}/changes", handlers.Read)
}

type Audit struct {
	Events http.Handler
}

func (handlers Audit) Register(mux *http.ServeMux) {
	mux.Handle("/api/operator-enrollment/v1/audit/events", readOnlyGet(handlers.Events))
}

func readOnlyGet(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type Handlers struct {
	Enrollment, Gateways, Snapshot, Diagnostics, ActorVisas           http.Handler
	AdapterLogs, DNSRecords, DNSStats, DNSStylesheet                  http.Handler
	PolicyAssistant, GatewayAssistantStatus, GatewayAssistant, Policy http.Handler
	BudgetAuthorityCurrent, BudgetAuthorityOperator                   http.Handler
	SubjectLinkReviews                                                http.Handler
	Assertions                                                        Assertions
	TrustedChanges                                                    TrustedChanges
	Audit                                                             Audit
	TrustedSourceControl                                              http.Handler
}

// NewMux requires every handler, including explicit unavailable-feature handlers.
// Authentication, security headers and server lifetime remain caller-owned.
func NewMux(handlers Handlers) *http.ServeMux {
	mux := http.NewServeMux()
	budgetCurrent := handlers.BudgetAuthorityCurrent
	if budgetCurrent == nil {
		budgetCurrent = http.HandlerFunc(unavailableBudgetAuthority)
	}
	budgetOperator := handlers.BudgetAuthorityOperator
	if budgetOperator == nil {
		budgetOperator = http.HandlerFunc(unavailableBudgetAuthority)
	}
	mux.Handle("/api/enrollment/", handlers.Enrollment)
	mux.Handle("/api/operator-enrollment/", handlers.Enrollment)
	reviews := handlers.SubjectLinkReviews
	if reviews == nil {
		reviews = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "subject-link review is unavailable", http.StatusServiceUnavailable)
		})
	}
	mux.Handle("/api/subject-link-reviews/", reviews)
	mux.Handle("/api/subject-link-registry/", reviews)
	mux.Handle("/api/subject-link-publications/", reviews)
	mux.Handle("POST /api/budget-authority/current", budgetCurrent)
	mux.Handle("/api/operator-enrollment/v1/budget-authority/", budgetOperator)
	handlers.Assertions.Register(mux)
	handlers.TrustedChanges.Register(mux)
	auditEvents := handlers.Audit.Events
	if auditEvents == nil {
		auditEvents = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "audit history is unavailable", http.StatusServiceUnavailable)
		})
	}
	(Audit{Events: auditEvents}).Register(mux)
	if handlers.TrustedSourceControl != nil {
		mux.Handle("GET /api/trusted-sources/runtime", handlers.TrustedSourceControl)
		mux.Handle("GET /api/trusted-sources/runtime/{source}/control", handlers.TrustedSourceControl)
		mux.Handle("PUT /api/trusted-sources/runtime/{source}/control", handlers.TrustedSourceControl)
	}
	mux.Handle("/api/gateways/", handlers.Gateways)
	mux.Handle("GET /api/snapshot", handlers.Snapshot)
	mux.Handle("GET /api/diagnostics", handlers.Diagnostics)
	mux.Handle("GET /api/actors/{actor}/visas", handlers.ActorVisas)
	mux.Handle("GET /api/adapter-logs", handlers.AdapterLogs)
	mux.Handle("GET /api/dns/records", handlers.DNSRecords)
	mux.Handle("POST /api/policy/assistant", handlers.PolicyAssistant)
	mux.Handle("GET /api/gateways/assistant", handlers.GatewayAssistantStatus)
	mux.Handle("POST /api/gateways/assistant", handlers.GatewayAssistant)
	mux.Handle("/api/dns/stats/", handlers.DNSStats)
	mux.Handle("GET /bind9.xsl", handlers.DNSStylesheet)
	mux.Handle("/api/policy", handlers.Policy)
	mux.Handle("/api/policy/", handlers.Policy)
	return mux
}

// unavailableBudgetAuthority provides the explicit fail-closed default for budget routes.
func unavailableBudgetAuthority(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "budget authority is unavailable", http.StatusServiceUnavailable)
}
