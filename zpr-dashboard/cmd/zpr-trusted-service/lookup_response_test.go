package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
)

func lookupResponseRecorder(t *testing.T, store provider, validity time.Duration, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/attributes",
		strings.NewReader(`{"identities":[]}`)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handlerWithValidity(store, nil, validity).ServeHTTP(response, request)
	return response
}

func TestInvalidProviderClaimsCannotBecomeSuccessfulObservations(t *testing.T) {
	for _, attrs := range []map[string][]string{
		nil,
		{"team": nil},
		{string([]byte{0xff}): {"eng"}},
		{"team": {string([]byte{0xff})}},
	} {
		for _, validity := range []time.Duration{0, time.Minute} {
			store := providerFunc(func(context.Context, []identity) (map[string][]string, error) {
				return attrs, nil
			})
			response := lookupResponseRecorder(t, store, validity, context.Background())
			if response.Code != http.StatusServiceUnavailable {
				t.Errorf("invalid claims returned %d: %s", response.Code, response.Body)
			}
			if strings.Contains(response.Body.String(), `"attributes"`) || strings.Contains(response.Body.String(), "expires_unix") {
				t.Errorf("invalid claims returned success fields: %s", response.Body)
			}
		}
	}
}

func TestResponseWireSizeMatchesVSConsumerLimit(t *testing.T) {
	for _, validity := range []time.Duration{0, time.Minute} {
		var deadline int64
		if validity > 0 {
			deadline = time.Now().Add(validity).Unix()
		}
		empty, err := json.Marshal(lookupResponse{
			Attributes: map[string][]string{"team": {""}}, ExpiresUnix: deadline,
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, size := range []int{(1 << 20) - 1, 1 << 20, (1 << 20) + 1} {
			// The existing response format includes the encoder's trailing newline.
			value := strings.Repeat("a", size-len(empty)-1)
			store := providerFunc(func(context.Context, []identity) (map[string][]string, error) {
				return map[string][]string{"team": {value}}, nil
			})
			response := lookupResponseRecorder(t, store, validity, context.Background())
			if size <= 1<<20 {
				if response.Code != http.StatusOK || response.Body.Len() != size {
					t.Errorf("valid wire size %d: status=%d bytes=%d", size, response.Code, response.Body.Len())
				}
			} else if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), `"attributes"`) {
				t.Errorf("oversized response was not rejected atomically: status=%d bytes=%d", response.Code, response.Body.Len())
			}
		}
	}
}

func TestCanceledHTTPRequestsDoNotPublishProviderClaims(t *testing.T) {
	for _, before := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		store := providerFunc(func(context.Context, []identity) (map[string][]string, error) {
			calls++
			cancel()
			return map[string][]string{"team": {"eng"}}, nil
		})
		if before {
			cancel()
		}
		response := lookupResponseRecorder(t, store, time.Minute, ctx)
		cancel()
		if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), `"attributes"`) {
			t.Errorf("canceled request published claims: %d %s", response.Code, response.Body)
		}
		if before && calls != 0 {
			t.Errorf("already canceled request called provider %d times", calls)
		}
	}
}

func TestProviderResponseRejectionRecoversAndPreservesEmptyClaims(t *testing.T) {
	valid := false
	store := providerFunc(func(context.Context, []identity) (map[string][]string, error) {
		if !valid {
			return map[string][]string{"team": nil}, nil
		}
		return map[string][]string{"tag": {}, "team": {""}, "unicode": {"\u00e9\U0001f600\ufffd"}}, nil
	})
	response := lookupResponseRecorder(t, store, time.Minute, context.Background())
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("invalid observation status/cache: %d %v", response.Code, response.Header())
	}
	valid = true
	response = lookupResponseRecorder(t, store, time.Minute, context.Background())
	var result lookupResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
		t.Fatalf("recovery failed: %d %s", response.Code, response.Body)
	}
	if result.Attributes["tag"] == nil || len(result.Attributes["team"]) != 1 || result.Attributes["team"][0] != "" ||
		result.Attributes["unicode"][0] != "\u00e9\U0001f600\ufffd" || result.ExpiresUnix <= time.Now().Unix() {
		t.Fatalf("valid response changed: %+v", result)
	}
}

func TestResponseSizeCountsEscapingAndCombinedClaims(t *testing.T) {
	for _, attrs := range []map[string][]string{
		{"team": {strings.Repeat("<", 180000)}},
		{"team": {strings.Repeat("a", 600000)}, "role": {strings.Repeat("b", 600000)}},
	} {
		store := providerFunc(func(context.Context, []identity) (map[string][]string, error) {
			return attrs, nil
		})
		response := lookupResponseRecorder(t, store, time.Minute, context.Background())
		if response.Code != http.StatusServiceUnavailable || response.Body.Len() > 1024 {
			t.Fatalf("aggregate/encoded size bypassed bound: %d %d", response.Code, response.Body.Len())
		}
	}
}

func TestLDAPInvalidTextCannotBeRewrittenByResponseEncoder(t *testing.T) {
	healthy := false
	store := providerFunc(func(ctx context.Context, _ []identity) (map[string][]string, error) {
		searcher := ldapSearcherFunc(func(*ldap.SearchRequest) (*ldap.SearchResult, error) {
			value := string([]byte{0xff})
			if healthy {
				value = "\ufffd"
			}
			return &ldap.SearchResult{Entries: []*ldap.Entry{
				ldap.NewEntry("uid=alice,dc=example", map[string][]string{"team": {value}}),
			}}, nil
		})
		return ldapIdentityAttributes(ctx, searcher, "people", "", "uid", "alice", []string{"team"})
	})
	response := lookupResponseRecorder(t, store, time.Minute, context.Background())
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("invalid LDAP text was published: %d %s", response.Code, response.Body)
	}
	healthy = true
	response = lookupResponseRecorder(t, store, time.Minute, context.Background())
	if response.Code != http.StatusOK {
		t.Fatalf("valid LDAP Unicode did not recover: %d %s", response.Code, response.Body)
	}
}

type failingLookupWriter struct {
	header http.Header
}

func (w *failingLookupWriter) Header() http.Header { return w.header }
func (*failingLookupWriter) WriteHeader(int)       {}
func (*failingLookupWriter) Write([]byte) (int, error) {
	return 0, errors.New("test transport failure")
}

func TestLookupResponseWriteFailuresAreLogged(t *testing.T) {
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(old)
	request := httptest.NewRequest(http.MethodPost, "/v1/attributes", strings.NewReader(`{"identities":[]}`))
	request.Header.Set("Content-Type", "application/json")
	store := &countingProvider{}
	handler(store, nil).ServeHTTP(&failingLookupWriter{header: make(http.Header)}, request)
	if store.calls != 1 || !strings.Contains(output.String(), "response write failed") {
		t.Fatalf("response write failure was hidden: calls=%d log=%s", store.calls, output.String())
	}
}
