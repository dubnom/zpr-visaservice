package operatorauth

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOIDCExpiredOrMissingLoginRecoversWithoutAuthenticating(t *testing.T) {
	for _, scenario := range []string{"expired", "missing-cookie", "lost-state", "replayed"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
			f := newProvider(t)
			a, err := newAuth(context.Background(), testConfig(f), "test-secret", f.server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			now := time.Now()
			a.now = func() time.Time { return now }
			state, browser := begin(t, a, f)
			switch scenario {
			case "expired":
				now = now.Add(5 * time.Minute)
			case "missing-cookie":
				browser = nil
			case "lost-state":
				delete(a.pending, state)
			case "replayed":
				if response := callback(a, state, browser); response.Code != http.StatusSeeOther {
					t.Fatalf("initial callback failed: %d", response.Code)
				}
			}
			exchanges, sessions := f.exchanges, len(a.sessions)
			response := callback(a, state, browser)
			if response.Code != http.StatusForbidden || f.exchanges != exchanges || len(a.sessions) != sessions {
				t.Fatal("invalid callback exchanged a code or authenticated a session")
			}
			if response.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
				response.Header().Get("Cache-Control") != "no-store" ||
				response.Header().Get("Referrer-Policy") != "no-referrer" ||
				!strings.Contains(response.Header().Get("Content-Security-Policy"), "form-action 'self'") {
				t.Fatal("recovery page lacks HTML type or privacy/security headers")
			}
			body := response.Body.String()
			for _, content := range []string{`method="get" action="/"`, `<button type="submit">Timed out. Try again.</button>`} {
				if !strings.Contains(body, content) {
					t.Fatalf("recovery action missing: %s", content)
				}
				if strings.Contains(body, "<a ") || strings.Contains(body, "Sign in again") {
					t.Fatal("recovery page includes an unwanted alternative action")
				}
			}
			if strings.Contains(body, state) || strings.Contains(body, "test-code") {
				t.Fatal("callback secrets reflected in recovery page")
			}
			if strings.Contains(body, `action="/auth/operator/login"`) {
				t.Fatal("recovery bypasses the application's same-origin login initiation")
			}
			for _, cookie := range response.Result().Cookies() {
				if cookie.Name == a.cookie && cookie.MaxAge >= 0 {
					t.Fatal("rejected callback issued a session cookie")
				}
			}
			newState, newBrowser := begin(t, a, f)
			if newState == state || (browser != nil && newBrowser.Value == browser.Value) {
				t.Fatal("retry reused the rejected login state or browser binding")
			}
			retry := callback(a, newState, newBrowser)
			if retry.Code != http.StatusSeeOther || retry.Header().Get("Location") != "/" || f.exchanges != exchanges+1 {
				t.Fatalf("fresh sign-in did not recover: status=%d", retry.Code)
			}
		})
	}
}
