package main

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

func configuredEnrollmentServiceProxy() (http.Handler, func() error, bool, error) {
	endpoint := os.Getenv("ZPR_ENROLLMENT_SERVICE_URL")
	cert := os.Getenv("ZPR_ENROLLMENT_CLIENT_CERT_FILE")
	key := os.Getenv("ZPR_ENROLLMENT_CLIENT_KEY_FILE")
	ca := os.Getenv("ZPR_ENROLLMENT_SERVICE_CA_FILE")
	serverName := os.Getenv("ZPR_ENROLLMENT_SERVICE_TLS_SERVER_NAME")
	delegationAudience := os.Getenv("ZPR_ENROLLMENT_DELEGATION_AUDIENCE")
	if endpoint == "" && cert == "" && key == "" && ca == "" && serverName == "" && delegationAudience == "" {
		return nil, nil, false, nil
	}
	fail := func(err error) (http.Handler, func() error, bool, error) { return nil, nil, true, err }
	for _, name := range []string{"ZPR_ENROLLMENT_CONFIG_FILE", "ZPR_ENROLLMENT_DATABASE_FILE"} {
		if os.Getenv(name) != "" {
			return fail(errors.New("remote enrollment cannot be combined with a local registry, catalog or delegation trust"))
		}
		trustPath := os.Getenv("ZPR_OPERATOR_DELEGATION_TRUST_FILE")
		if trustPath != "" && delegationAudience == "" {
			return fail(errors.New("remote enrollment cannot be combined with implicit local delegation trust; configure its explicit logical delegation audience"))
		}
		if delegationAudience != "" {
			audience, err := url.Parse(delegationAudience)
			if err != nil || audience.Scheme != "https" || audience.Hostname() == "" || audience.User != nil ||
				delegationAudience != "https://"+audience.Host || trustPath == "" {
				return fail(errors.New("remote enrollment logical delegation requires an exact HTTPS operator audience and explicit shared trust"))
			}
			data, err := readOperatorFile(trustPath, 65536, false)
			if err != nil {
				return fail(err)
			}
			var trust operatordelegation.Config
			if err := decodeOperatorConfig(data, &trust); err != nil {
				return fail(err)
			}
			if err := trust.Validate(); err != nil {
				return fail(err)
			}
			if trust.Audience != delegationAudience {
				return fail(errors.New("remote enrollment logical audience differs from the shared operator delegation trust"))
			}
		}
	}
	if endpoint == "" || cert == "" || key == "" || ca == "" {
		return fail(errors.New("remote enrollment requires service URL, client certificate/key and service CA"))
	}
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.User != nil || endpoint != "https://"+base.Host {
		return fail(errors.New("ZPR_ENROLLMENT_SERVICE_URL must be an exact HTTPS origin"))
	}
	if serverName == "" {
		serverName = base.Hostname()
	}
	transport, err := servicehttp.MutualTLSClientTransport(cert, key, ca, serverName)
	if err != nil {
		return fail(errors.New("remote enrollment mutual TLS configuration is invalid or unavailable"))
	}
	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(base)
			request.Out.Host = base.Host
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			log.Printf("Enrollment Service request failed: %T", err)
			writePolicyError(w, http.StatusBadGateway, "Enrollment Service is unavailable; mutation outcome may be uncertain. Reconcile before retrying.")
		},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		deny := func(message string) {
			log.Print("Enrollment Service relay denied a request")
			writePolicyError(w, http.StatusForbidden, message)
		}
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			deny("Enrollment relay requires a verified service client.")
			return
		}
		if !strings.HasPrefix(r.URL.Path, operatordelegation.Prefix) ||
			r.URL.RawPath != "" || r.URL.ForceQuery ||
			(r.Method != http.MethodGet && r.Method != http.MethodPost) {
			deny("Remote enrollment permits only canonical named-user delegation routes.")
			return
		}
		if values := r.Header.Values(operatordelegation.Header); len(values) != 1 || values[0] == "" || len(values[0]) > 8192 {
			deny("Remote enrollment requires a signed named-user delegation assertion.")
			return
		}
		for _, header := range []string{"Origin", "Cookie", "Authorization", "X-ZPR-CSRF", "Sec-Fetch-Site"} {
			if r.Header.Get(header) != "" {
				deny("Browser credentials must not reach the private enrollment relay.")
				return
			}
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, operatordelegation.MaxBody))
		if err != nil || (r.Method == http.MethodGet && len(body) != 0) {
			log.Print("Enrollment relay denied an unreadable or oversized request body")
			writePolicyError(w, http.StatusRequestEntityTooLarge, "Enrollment request body is invalid or exceeds 8192 bytes.")
			return
		}
		request := r.Clone(r.Context())
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.ContentLength = int64(len(body))
		proxy.ServeHTTP(w, request)
	})
	return handler, func() error { transport.CloseIdleConnections(); return nil }, true, nil
}
