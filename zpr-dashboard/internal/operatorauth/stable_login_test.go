package operatorauth

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestExpiredLoginReturnsToStableScreenAndClearsFlow(t *testing.T) {
	f := newProvider(t)
	a, err := newAuth(context.Background(), testConfig(f), "test-secret", f.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	now := time.Now()
	a.now = func() time.Time { return now }
	state, browser := begin(t, a, f)
	now = now.Add(5 * time.Minute)
	response := callback(a, state, browser)
	if response.Code != http.StatusSeeOther || f.exchanges != 0 || len(a.sessions) != 0 {
		t.Fatal("expired callback authenticated or exchanged its code")
	}
	if response.Header().Get("Location") != "/?operator_login=expired" || strings.Contains(response.Body.String(), "http-equiv") {
		t.Fatal("expired callback does not return to the fixed stable sign-in screen")
	}
	cleared := false
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == a.flow && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("expired callback retained the browser flow cookie")
	}
}
