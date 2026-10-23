package main

import (
	"crypto/tls"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	roomhttp "neboagency.com/zpr-dashborad/internal/controlroom/httpapi"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestControlRoomHTTPRealHandlersWithoutSimulator(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	t.Setenv("ZPR_CONTROL_ROOM_PROXY_IP", "")
	t.Setenv("ZPR_CHANGE_REVIEW_CONFIG_FILE", "")
	staticRoot, err := fs.Sub(controlRoomFiles, "static")
	if err != nil {
		t.Fatal(err)
	}
	for _, management := range []bool{false, true} {
		name := "development"
		if management {
			name = "management"
		}
		t.Run(name, func(t *testing.T) {
			security := &controlRoomSecurity{}
			host, remote := "localhost:8787", "127.0.0.1:1234"
			if management {
				security = &controlRoomSecurity{
					origin: "https://room.company.example", tls: &tls.Config{MinVersion: tls.VersionTLS13},
					auth: &operatorauth.Auth{}, management: true,
				}
				host, remote = "room.company.example", "192.0.2.10:1234"
			}
			proxyCalls := 0
			proxy := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				proxyCalls++
				w.WriteHeader(http.StatusNoContent)
			})
			review, closeReview, err := configuredChangeReview(security.auth, proxy)
			if err != nil {
				t.Fatal(err)
			}
			defer closeReview()
			web := security.boundary()
			mux := roomhttp.NewMux(roomhttp.Handlers{
				AuthConfig: web.ConfigHandler(writeJSON), Auth: web.LoginHandler(writePolicyError),
				ChangeReview: review, Enrollment: operatorEnrollmentProxy(security.auth, nil, proxy),
				API: security.apiProxy("production", proxy), DNSStylesheet: security.assetProxy("production", proxy),
				Static: revalidateStatic(http.FileServer(http.FS(staticRoot))),
			})
			handler := securityHeaders(security.protect(mux))
			configBody, apiStatus, stylesheetStatus, referrer, apiCache := `{"enabled":false}`, 204, 204, "no-referrer", ""
			if management {
				configBody, apiStatus, stylesheetStatus, referrer, apiCache = `{"enabled":true}`, 403, 403, "same-origin", "no-store"
			}
			for _, test := range []struct {
				method, path, body, cache string
				status                    int
			}{
				{"GET", "/auth/operator/config", configBody, "no-store", 200},
				{"GET", "/api/change-review/proposals", "Change review is not configured.", "no-store", 503},
				{"POST", "/api/enrollment/v1/invitations", "requires configured named-user authorization", "no-store", 403},
				{"GET", "/api/snapshot", "", apiCache, apiStatus},
				{"GET", "/bind9.xsl", "", apiCache, stylesheetStatus},
				{"GET", "/", "<!doctype html>", "no-cache", 200},
				{"GET", "/missing-static-file", "404 page not found", "", 404},
			} {
				t.Run(test.path, func(t *testing.T) {
					request := httptest.NewRequest(test.method, "https://"+host+test.path, nil)
					request.Host, request.RemoteAddr = host, remote
					response := httptest.NewRecorder()
					before := proxyCalls
					handler.ServeHTTP(response, request)
					if response.Code != test.status || !strings.Contains(strings.ToLower(response.Body.String()), strings.ToLower(test.body)) ||
						response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Content-Security-Policy") == "" {
						t.Fatalf("status=%d body=%s headers=%v", response.Code, response.Body, response.Header())
					}
					if response.Header().Get("Cache-Control") != test.cache {
						t.Fatalf("cache=%q want %q", response.Header().Get("Cache-Control"), test.cache)
					}
					if test.path == "/" && response.Header().Get("Referrer-Policy") != referrer {
						t.Fatalf("referrer=%q", response.Header().Get("Referrer-Policy"))
					}
					expectedCalls := 0
					if test.status == 204 {
						expectedCalls = 1
					}
					if proxyCalls-before != expectedCalls {
						t.Fatalf("proxy calls=%d want %d", proxyCalls-before, expectedCalls)
					}
				})
			}
			for _, foreign := range []bool{false, true} {
				request := httptest.NewRequest("GET", "https://"+host+"/api/snapshot", nil)
				request.Host, request.RemoteAddr = host, "192.0.2.10:1234"
				if foreign {
					request.Header.Set("Origin", "https://foreign.example")
				} else if management {
					request.Header.Set("X-Forwarded-For", "127.0.0.1")
				}
				response := httptest.NewRecorder()
				before := proxyCalls
				handler.ServeHTTP(response, request)
				if response.Code != 403 || proxyCalls != before {
					t.Fatalf("composed boundary bypass: status=%d calls=%d", response.Code, proxyCalls-before)
				}
			}
		})
	}
}
