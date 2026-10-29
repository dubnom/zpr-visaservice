package main

import (
	"context"
	"crypto/tls"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/controlroom"
	roomhttp "neboagency.com/zpr-dashborad/internal/controlroom/httpapi"
	"neboagency.com/zpr-dashborad/internal/controlservice/httpapi"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

func runControlRoom(listen string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	security, err := configuredControlRoomSecurity(ctx, listen)
	if err != nil {
		return err
	}
	defer security.close()
	signer, err := configuredOperatorSigner(security)
	if err != nil {
		return err
	}
	proxy, message := newControlServiceProxy()
	if proxy == nil {
		return errors.New(message)
	}
	staticRoot, err := fs.Sub(controlRoomFiles, "static")
	if err != nil {
		return err
	}
	web := security.boundary()
	authConfig, authLogin := web.ConfigHandler(writeJSON), web.LoginHandler(writePolicyError)
	changeReview, closeReview, err := configuredChangeReview(security.auth, proxy)
	if err != nil {
		return err
	}
	defer closeReview()
	mux := roomhttp.NewMux(roomhttp.Handlers{
		AuthConfig: authConfig, Auth: authLogin, ChangeReview: changeReview,
		Enrollment:    operatorEnrollmentProxy(security.auth, signer, proxy),
		API:           security.apiProxy(os.Getenv("ZPR_CONTROL_ROOM_ORGANIZATION_ID"), proxy),
		DNSStylesheet: security.assetProxy(os.Getenv("ZPR_CONTROL_ROOM_ORGANIZATION_ID"), proxy),
		Static:        revalidateStatic(http.FileServer(http.FS(staticRoot))),
	})
	controlRoomMux := http.NewServeMux()
	controlRoomMux.Handle(subjectReviewPrefix, subjectLinkReviewProxy(security.auth, signer, proxy))
	controlRoomMux.Handle(subjectRegistryPrefix, subjectLinkReviewProxy(security.auth, signer, proxy))
	controlRoomMux.Handle(subjectPublicationPrefix, subjectLinkReviewProxy(security.auth, signer, proxy))
	controlRoomMux.Handle(auditEventsRoute, auditEventsProxy(security.auth, signer, os.Getenv("ZPR_CONTROL_ROOM_ORGANIZATION_ID"), proxy))
	controlRoomMux.Handle("/api/budget-authority/", budgetAuthorityOperatorProxy(security.auth, signer, proxy))
	controlRoomMux.Handle("/", mux)
	server := &http.Server{
		Addr: listen, Handler: securityHeaders(security.protect(controlRoomMux)), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
		TLSConfig: security.tls, MaxHeaderBytes: 32768,
	}
	if security.tls != nil {
		log.Printf("ZPR Control Room listening at %s (named operator login configured: %t; delegation signing configured: %t)", security.origin, security.auth != nil, signer != nil)
		return server.ListenAndServeTLS("", "")
	}
	log.Printf("ZPR Control Room listening at http://%s", listen)
	return server.ListenAndServe()
}

func localControlRoomProxy(proxy http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localEditorRequest(w, r) {
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/enrollment/") || strings.HasPrefix(r.URL.Path, "/api/operator-enrollment/") {
			w.Header().Set("Cache-Control", "no-store")
			writePolicyError(w, http.StatusForbidden, "Control Room enrollment requires named-user authorization; use the certificate-authorized administration API directly.")
			return
		}
		proxy.ServeHTTP(w, r)
	})
}

func newControlServiceProxy() (http.Handler, string) {
	endpoint := strings.TrimRight(strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_URL")), "/")
	clientCert := strings.TrimSpace(os.Getenv("ZPR_CONTROL_CLIENT_CERT_FILE"))
	clientKey := strings.TrimSpace(os.Getenv("ZPR_CONTROL_CLIENT_KEY_FILE"))
	caFile := strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_CA_FILE"))
	if endpoint == "" || clientCert == "" || clientKey == "" || caFile == "" {
		return nil, "Configure ZPR_CONTROL_SERVICE_URL, ZPR_CONTROL_CLIENT_CERT_FILE, ZPR_CONTROL_CLIENT_KEY_FILE, and ZPR_CONTROL_SERVICE_CA_FILE."
	}
	baseURL, err := url.Parse(endpoint)
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" || baseURL.Path != "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, "ZPR_CONTROL_SERVICE_URL must be an HTTPS origin without a path."
	}
	serverName := strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_TLS_SERVER_NAME"))
	if serverName == "" {
		serverName = baseURL.Hostname()
	}
	transport, err := servicehttp.MutualTLSClientTransport(clientCert, clientKey, caFile, serverName)
	if err != nil {
		return nil, err.Error()
	}
	proxy := controlroom.NewProxy(baseURL, transport)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("Control Service request failed: %T", err)
		writePolicyError(w, http.StatusBadGateway, "Control Service is unavailable.")
	}
	return proxy, ""
}

func runControlService() error {
	listen := envOr("ZPR_CONTROL_SERVICE_LISTEN", "127.0.0.1:8790")
	certFile := strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_CLIENT_CA_FILE"))
	if certFile == "" || keyFile == "" || clientCAFile == "" {
		return errors.New("control-service mode requires ZPR_CONTROL_SERVICE_CERT_FILE, ZPR_CONTROL_SERVICE_KEY_FILE, and ZPR_CONTROL_SERVICE_CLIENT_CA_FILE")
	}
	clientCAs, err := servicehttp.LoadCertificateAuthorities(clientCAFile, "Control Service client")
	if err != nil {
		return err
	}
	subjectReviews, closeSubjectReviews, err := configuredSubjectLinkReviews()
	if err != nil {
		return err
	}
	defer func() {
		if err := closeSubjectReviews(); err != nil {
			log.Printf("Close subject-link review store failed: %T", err)
		}
	}()
	auditEvents, auditRepository, closeAuditEvents, err := configuredAuditEvents()
	if err != nil {
		return err
	}
	defer func() {
		if err := closeAuditEvents(); err != nil {
			log.Printf("Close audit event store failed: %T", err)
		}
	}()
	enrollmentAPI, closeEnrollment, err := configuredEnrollmentHandler(auditRepository)
	if err != nil {
		return err
	}
	defer func() {
		if err := closeEnrollment(); err != nil {
			log.Printf("Close enrollment registry failed: %T", err)
		}
	}()
	budgetAuthorityAPI, closeBudgetAuthority := configuredBudgetAuthority(auditRepository)
	defer func() {
		if err := closeBudgetAuthority(); err != nil {
			log.Printf("Close budget authority store failed: %T", err)
		}
	}()
	admin, configErr := newAdminClient()
	trustedChanges, err := newTrustedSourceChanges(strings.TrimSpace(os.Getenv("ZPR_TRUSTED_CHANGE_FEEDS_FILE")))
	if err != nil {
		return err
	}
	assertions, err := newAssertionRuntime()
	if err != nil {
		return err
	}
	assertionStore, assertionStoreErr := newPolicyServiceAssertionSettingsStore()
	if assertionStoreErr != "" {
		log.Printf("Policy Repository assertion settings are unavailable: %s", assertionStoreErr)
	} else {
		assertions.settingsStore = assertionStore
		refreshCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		if err := assertions.refreshSettings(refreshCtx); err != nil {
			log.Printf("Policy Repository assertion settings are unavailable: %T", err)
		} else if err := assertions.migrateLegacySettings(refreshCtx); err != nil {
			log.Printf("Legacy assertions were not migrated: %T", err)
		}
		cancel()
	}
	assertionContext, stopAssertions := context.WithCancel(context.Background())
	defer stopAssertions()
	stopWatchers, err := assertions.startTrustedWatchers(assertionContext, strings.TrimSpace(os.Getenv("ZPR_TRUSTED_WATCH_STATE_DIR")), admin)
	if err != nil {
		return err
	}
	defer stopWatchers()
	trustedChanges.watcherStatus = assertions.allWatcherStatus
	assertions.start(assertionContext)
	policyAPI, policyErr := newPolicyServiceProxy()
	diagnostics, diagnosticsStaleAfter, diagnosticsErr := newOpenObserveDiagnosticsProvider()
	app := &application{
		admin: admin, configErr: configErr, policyAPI: policyAPI, policyErr: policyErr, assistant: newClaudeAssistant(),
	}
	var gatewayStore *gatewayConfigStore
	if storePath := strings.TrimSpace(os.Getenv("ZPR_GATEWAY_CONFIG_STORE_DIR")); storePath != "" {
		gatewayStore, err = newGatewayConfigStore(storePath)
		if err != nil {
			log.Printf("Gateway draft store is unavailable: %T", err)
		}
	}
	gatewayAPI := newGatewayAPI(strings.TrimSpace(os.Getenv("ZPR_GATEWAY_ORGANIZATION_ID")), gatewayInventoryReaderFromApplication(app), gatewayStore)
	readDiagnosticsSnapshot := func(ctx context.Context) snapshot {
		if app.admin == nil {
			return snapshot{APIStatus: "disconnected", Errors: []string{app.configErr}}
		}
		return app.fetchSnapshot(ctx)
	}
	mux := httpapi.NewMux(httpapi.Handlers{
		Enrollment: enrollmentAPI, Gateways: gatewayAPI,
		SubjectLinkReviews:      subjectReviews,
		Audit:                   httpapi.Audit{Events: auditEvents},
		BudgetAuthorityCurrent:  http.HandlerFunc(budgetAuthorityAPI.Current),
		BudgetAuthorityOperator: http.HandlerFunc(budgetAuthorityAPI.Operator),
		Assertions:              assertions.handlers(), TrustedChanges: trustedChanges.handlers(),
		TrustedSourceControl: trustedSourceControlHandler(admin),
		Snapshot:             http.HandlerFunc(app.handleSnapshot),
		Diagnostics:          newDiagnosticsHandler(readDiagnosticsSnapshot, diagnostics, diagnosticsErr, diagnosticsStaleAfter),
		ActorVisas:           http.HandlerFunc(app.handleActorVisas), AdapterLogs: newAdapterLogsHandler(),
		DNSRecords: newDNSRecordsHandler(), DNSStats: newDNSStatsProxy(), DNSStylesheet: newDNSStatsAssetProxy(),
		PolicyAssistant:        http.HandlerFunc(app.handlePolicyAssistant),
		GatewayAssistantStatus: http.HandlerFunc(app.handleGatewayAssistantStatus),
		GatewayAssistant:       http.HandlerFunc(app.handleGatewayAssistant), Policy: app.policyProxyHandler(),
	})
	server := &http.Server{
		Addr: listen, Handler: securityHeaders(mux), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientCAs},
	}
	reviewAPI, _ := subjectReviews.(*subjectReviewAPI)
	delivery, err := configuredSubjectSnapshotDelivery(reviewAPI, certFile, keyFile)
	if err != nil {
		return err
	}
	log.Printf("ZPR Control Service listening with mutual TLS at %s", listen)
	if configErr != "" {
		log.Printf("Visa Service Admin API is not configured: %s", configErr)
	}
	if policyErr != "" {
		log.Printf("Policy Service is not configured: %s", policyErr)
	}
	if delivery == nil {
		return server.ListenAndServeTLS(certFile, keyFile)
	}
	errs := make(chan error, 2)
	go func() { errs <- server.ListenAndServeTLS(certFile, keyFile) }()
	go func() { errs <- runSubjectSnapshotDelivery(delivery) }()
	return <-errs
}
