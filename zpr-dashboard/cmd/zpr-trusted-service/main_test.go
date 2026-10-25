package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
)

func TestSourceValidityIncludesEmptyObservationsAndPreservesLegacyShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actors.json")
	if err := os.WriteFile(path, []byte(`{"user.sub":{"alice":{"team":["eng"]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{"alice", "missing"} {
		for _, validity := range []time.Duration{0, 120 * time.Second} {
			start := time.Now().Unix()
			request := httptest.NewRequest(http.MethodPost, "/v1/attributes",
				strings.NewReader(`{"identities":[{"key":"user.sub","value":"`+subject+`"}]}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handlerWithValidity(fileProvider{path}, nil, validity).ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("lookup failed: %s", response.Body.String())
			}
			var result lookupResponse
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if validity == 0 {
				if strings.Contains(response.Body.String(), "expires_unix") {
					t.Fatal("legacy response shape changed")
				}
			} else if result.ExpiresUnix < start+120 || result.ExpiresUnix > time.Now().Unix()+120 {
				t.Fatalf("invalid source deadline: %d", result.ExpiresUnix)
			}
			if subject == "missing" && len(result.Attributes) != 0 {
				t.Fatalf("unexpected attributes: %v", result.Attributes)
			}
		}
	}
}

type delayedEmptyProvider struct{}

func (delayedEmptyProvider) lookup(context.Context, []identity) (map[string][]string, error) {
	time.Sleep(time.Millisecond)
	return map[string][]string{}, nil
}

func TestExpiredLookupValidityDoesNotReturnSuccessfulObservation(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/attributes",
		strings.NewReader(`{"identities":[]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	// Exercise an already elapsed bound without waiting for the production
	// minimum one-second validity interval.
	handlerWithValidity(delayedEmptyProvider{}, nil, time.Nanosecond).ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expired observation returned status %d", response.Code)
	}
}

func TestFileLookupAndConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actors.json")
	data := `{"device.zpr.adapter.cn":{"alice":{"color":["red"]},"bob":{"color":["blue"]}},"user.sub":{"alice":{"team":["eng"]}}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	store := fileProvider{path}
	attributes, err := store.lookup(context.Background(), []identity{{"device.zpr.adapter.cn", "alice"}, {"user.sub", "alice"}})
	if err != nil || attributes["color"][0] != "red" || attributes["team"][0] != "eng" {
		t.Fatalf("lookup: %v %v", attributes, err)
	}
	attributes, err = store.lookup(context.Background(), []identity{{"device.zpr.adapter.cn", "missing"}})
	if err != nil || len(attributes) != 0 {
		t.Fatalf("unknown identity: %v %v", attributes, err)
	}
	_, err = store.lookup(context.Background(), []identity{{"device.zpr.adapter.cn", "alice"}, {"device.zpr.adapter.cn", "bob"}})
	if err == nil {
		t.Fatal("conflicting identities must fail closed")
	}
	if err := os.WriteFile(path, []byte(`{"device.zpr.adapter.cn":{"alice":{"color":["green"]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	attributes, err = store.lookup(context.Background(), []identity{{"device.zpr.adapter.cn", "alice"}})
	if err != nil || attributes["color"][0] != "green" {
		t.Fatalf("file update was not visible: %v %v", attributes, err)
	}
}

func TestFileProviderRejectsAmbiguousAndNullSnapshotsAndRecovers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actors.json")
	store := fileProvider{path}
	identities := []identity{{"user.sub", "alice"}}
	for _, data := range []string{
		`{"user.sub":{"alice":{"team":["sales"],"team":["eng"]}}}`,
		`{"user.sub":{"alice":{"team":["eng"],"team":["eng"]}}}`,
		`{"user.sub":{"alice":{"team":["sales"],"te\u0061m":["eng"]}}}`,
		`{"user.sub":{"alice":{},"alice":{"team":["eng"]}}}`,
		`{"user.sub":{"alice":{}},"user.sub":{"alice":{"team":["eng"]}}}`,
		`null`, `{"user.sub":null}`, `{"user.sub":{"alice":null}}`,
		`{"user.sub":{"alice":{"team":null}}}`,
		`{"user.sub":{"alice":{"team":[null]}}}`,
		`{"user.sub":{"alice":{"team":["eng"]}}} {}`,
		`{"user.sub":{}}` + strings.Repeat(" ", 1<<20),
	} {
		t.Run(data[:min(len(data), 80)], func(t *testing.T) {
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			attrs, err := store.lookup(context.Background(), identities)
			if err == nil || attrs != nil {
				t.Fatalf("invalid snapshot returned attributes: %v, %v", attrs, err)
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/attributes",
				strings.NewReader(`{"identities":[{"key":"user.sub","value":"alice"}]}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler(store, nil).ServeHTTP(response, request)
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("invalid file returned %d", response.Code)
			}
			if err := os.WriteFile(path, []byte(`{"user.sub":{"alice":{"team":["eng"],"tag":[]}}}`), 0600); err != nil {
				t.Fatal(err)
			}
			attrs, err = store.lookup(context.Background(), identities)
			if err != nil || len(attrs["team"]) != 1 || attrs["team"][0] != "eng" || attrs["tag"] == nil {
				t.Fatalf("repair did not recover valid/empty-array claims: %v, %v", attrs, err)
			}
		})
	}
}

type countingProvider struct{ calls int }

func TestLDAPLookupHTTPFailuresHaveNoPartialObservationAndRecover(t *testing.T) {
	for _, failure := range []string{"people-referral", "groups-referral", "size-limit", "nil-result", "group-error", "duplicate", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			broken := true
			store := providerFunc(func(ctx context.Context, _ []identity) (map[string][]string, error) {
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()
				searcher := ldapSearcherFunc(func(request *ldap.SearchRequest) (*ldap.SearchResult, error) {
					if request.BaseDN == "people" {
						result := &ldap.SearchResult{Entries: []*ldap.Entry{
							ldap.NewEntry("uid=alice,dc=example", map[string][]string{"team": {"eng"}}),
						}}
						if broken {
							switch failure {
							case "people-referral":
								result.Referrals = []string{"ldaps://other.example"}
							case "size-limit":
								return result, ldap.NewError(ldap.LDAPResultSizeLimitExceeded, errors.New("partial"))
							case "nil-result":
								return nil, nil
							case "duplicate":
								result.Entries[0].Attributes = append(result.Entries[0].Attributes,
									&ldap.EntryAttribute{Name: "TEAM", Values: []string{"sales"}})
							case "cancel":
								cancel()
							}
						}
						return result, nil
					}
					if broken && failure == "group-error" {
						return &ldap.SearchResult{Entries: []*ldap.Entry{
							ldap.NewEntry("cn=Approver,dc=example", map[string][]string{"cn": {"Approver"}}),
						}}, errors.New("group search unavailable")
					}
					if broken && failure == "groups-referral" {
						return &ldap.SearchResult{Referrals: []string{"ldaps://other.example"}}, nil
					}
					return &ldap.SearchResult{}, nil
				})
				return ldapIdentityAttributes(ctx, searcher, "people", "groups", "uid", "alice", []string{"team"})
			})
			for _, unavailable := range []bool{true, false} {
				broken = unavailable
				request := httptest.NewRequest(http.MethodPost, "/v1/attributes",
					strings.NewReader(`{"identities":[{"key":"user.sub","value":"alice"}]}`))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				handlerWithValidity(store, nil, time.Minute).ServeHTTP(response, request)
				if unavailable {
					if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "expires_unix") || strings.Contains(response.Body.String(), `"attributes"`) {
						t.Fatalf("incomplete LDAP lookup produced observation: %d %s", response.Code, response.Body)
					}
				} else if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"team":["eng"]`) {
					t.Fatalf("LDAP recovery failed: %d %s", response.Code, response.Body)
				}
			}
		})
	}
}

func TestLDAPUnknownIdentityAndAttributeCasingRemainSupported(t *testing.T) {
	for _, known := range []bool{false, true} {
		searcher := ldapSearcherFunc(func(*ldap.SearchRequest) (*ldap.SearchResult, error) {
			if !known {
				return &ldap.SearchResult{}, nil
			}
			return &ldap.SearchResult{Entries: []*ldap.Entry{
				ldap.NewEntry("uid=alice,dc=example", map[string][]string{"TEAM": {"eng"}}),
			}}, nil
		})
		attrs, err := ldapIdentityAttributes(context.Background(), searcher, "people", "", "uid", "alice", []string{"team"})
		if err != nil || attrs == nil {
			t.Fatalf("valid result rejected: %v %v", attrs, err)
		}
		if known && strings.Join(attrs["team"], ",") != "eng" {
			t.Fatalf("LDAP attribute casing lost: %v", attrs)
		}
		if !known && len(attrs) != 0 {
			t.Fatalf("unknown identity vended claims: %v", attrs)
		}
	}
}

type providerFunc func(context.Context, []identity) (map[string][]string, error)

func (f providerFunc) lookup(ctx context.Context, identities []identity) (map[string][]string, error) {
	return f(ctx, identities)
}

func (p *countingProvider) lookup(context.Context, []identity) (map[string][]string, error) {
	p.calls++
	return map[string][]string{}, nil
}

func TestLookupRequestFieldNamesAreExact(t *testing.T) {
	for _, body := range []string{
		`{"IDENTITIES":[]}`,
		`{"identities":[],"Identities":[]}`,
		`{"identities":[{"Key":"user.sub","value":"alice"}]}`,
		`{"identities":[{"key":"user.sub","Value":"alice"}]}`,
		`{"identities":[{"key":"user.sub","KEY":"other","value":"alice"}]}`,
		`{"identities":[{"key":"user.sub","value":"alice","VALUE":"bob"}]}`,
	} {
		store := &countingProvider{}
		request := httptest.NewRequest(http.MethodPost, "/v1/attributes", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler(store, nil).ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || store.calls != 0 {
			t.Errorf("noncanonical fields reached provider: %s: %d %d", body, response.Code, store.calls)
		}
	}
}

func TestInvalidUTF8CannotAliasAFileSubject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actors.json")
	// Valid replacement characters remain valid identity data, not a decoding fallback.
	valid := `{"user.sub":{"\ufffd":{"team":["eng"]}}}`
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	body := append([]byte(`{"identities":[{"key":"user.sub","value":"`), 0xff)
	body = append(body, []byte(`"}]}`)...)
	request := httptest.NewRequest(http.MethodPost, "/v1/attributes", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler(fileProvider{path}, nil).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Errorf("invalid request UTF-8 matched a different subject: %d %s", response.Code, response.Body)
	}
	invalid := append([]byte(`{"user.sub":{"`), 0xff)
	invalid = append(invalid, []byte(`":{"team":["eng"]}}}`)...)
	if err := os.WriteFile(path, invalid, 0600); err != nil {
		t.Fatal(err)
	}
	attrs, err := (fileProvider{path}).lookup(context.Background(), []identity{{"user.sub", "\ufffd"}})
	if err == nil || attrs != nil {
		t.Errorf("invalid file UTF-8 became a replacement-character subject: %v %v", attrs, err)
	}
}

func TestLookupJSONRejectsUnpairedUnicodeEscapes(t *testing.T) {
	for _, escape := range []string{`\ud800`, `\udfff`, `\ud800x`, `\ud800\u0041`, `\ud800\ud800`} {
		body := `{"identities":[{"key":"user.sub","value":"` + escape + `"}]}`
		store := &countingProvider{}
		request := httptest.NewRequest(http.MethodPost, "/v1/attributes", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler(store, nil).ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || store.calls != 0 {
			t.Errorf("unpaired escape reached provider: %s: %d %d", escape, response.Code, store.calls)
		}
	}
	for _, data := range []string{
		`{"user.sub":{"\ud800":{"team":["eng"]}}}`,
		`{"user.sub":{"alice":{"team":["\udfff"]}}}`,
	} {
		var records map[string]map[string]map[string][]string
		if err := decodeLookupJSON([]byte(data), &records); err == nil {
			t.Errorf("unpaired file escape accepted: %s", data)
		}
	}
	for _, value := range []string{`\ud83d\ude00`, `\\ud800`, `\\`, `\"`, `\ufffd`} {
		body := []byte(`{"identities":[{"key":"user.sub","value":"` + value + `"}]}`)
		if _, err := decodeLookupRequest(body); err != nil {
			t.Errorf("valid escapes rejected: %s: %v", value, err)
		}
	}
}

func TestLookupRequestExactByteLimitAndUnicodePreservation(t *testing.T) {
	const minimal = `{"identities":[]}`
	for _, size := range []int{(64 << 10) - 1, 64 << 10, (64 << 10) + 1} {
		store := &countingProvider{}
		body := minimal + strings.Repeat(" ", size-len(minimal))
		request := httptest.NewRequest(http.MethodPost, "/v1/attributes", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler(store, nil).ServeHTTP(response, request)
		if size <= 64<<10 {
			if response.Code != http.StatusOK || store.calls != 1 {
				t.Fatalf("request of %d bytes rejected: %d %d", size, response.Code, store.calls)
			}
		} else if response.Code != http.StatusBadRequest || store.calls != 0 {
			t.Fatalf("request of %d bytes accepted: %d %d", size, response.Code, store.calls)
		}
	}
	path := filepath.Join(t.TempDir(), "actors.json")
	if err := os.WriteFile(path, []byte(`{"user.sub":{"\u00e9\ud83d\ude00\ufffd":{"team":["eng"]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`\u00e9\ud83d\ude00\ufffd`, "\u00e9\U0001f600\ufffd"} {
		request := httptest.NewRequest(http.MethodPost, "/v1/attributes",
			strings.NewReader(`{"ident\u0069ties":[{"k\u0065y":"user.sub","value":"`+value+`"}]}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler(fileProvider{path}, nil).ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"team":["eng"]`) {
			t.Fatalf("valid Unicode/escaped field names changed: %d %s", response.Code, response.Body)
		}
	}
}

func TestMutualTLSStrictLookupAndFileRecovery(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(100),
		Subject:      pkix.Name{CommonName: "lookup boundary test"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	certificate := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	path := filepath.Join(t.TempDir(), "actors.json")
	const validFile = `{"user.sub":{"alice":{"team":["eng"]}}}`
	if err := os.WriteFile(path, []byte(validFile), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handlerWithValidity(fileProvider{path}, nil, time.Minute))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{certificate}, ClientCAs: roots,
		ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13,
	}
	server.StartTLS()
	defer server.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: roots, Certificates: []tls.Certificate{certificate},
		ServerName: "localhost", MinVersion: tls.VersionTLS13,
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	send := func(body string, status int) {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/attributes", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != status {
			t.Fatalf("TLS request returned %d, want %d: %s", response.StatusCode, status, data)
		}
		if status != http.StatusOK && (bytes.Contains(data, []byte(`"attributes"`)) || bytes.Contains(data, []byte("expires_unix"))) {
			t.Fatalf("failed TLS lookup returned success-shaped fields: %s", data)
		}
		if status == http.StatusOK {
			var result lookupResponse
			if err := json.Unmarshal(data, &result); err != nil || result.Attributes == nil || result.ExpiresUnix <= time.Now().Unix() {
				t.Fatalf("valid bounded TLS observation failed: %s %v", data, err)
			}
		}
	}
	const validRequest = `{"identities":[{"key":"user.sub","value":"alice"}]}`
	send(validRequest, http.StatusOK)
	for _, body := range []string{
		`{"identities":[],"Identities":[]}`,
		`{"identities":[{"key":"user.sub","VALUE":"alice"}]}`,
		`{"identities":[{"key":"user.sub","value":"` + string([]byte{0xff}) + `"}]}`,
		`{"identities":[{"key":"user.sub","value":"\ud800"}]}`,
	} {
		send(body, http.StatusBadRequest)
		send(validRequest, http.StatusOK)
	}
	const empty = `{"identities":[]}`
	send(empty+strings.Repeat(" ", (64<<10)-len(empty)), http.StatusOK)
	send(empty+strings.Repeat(" ", (64<<10)+1-len(empty)), http.StatusBadRequest)
	for _, invalid := range []string{
		`{"user.sub":{"alice":{"team":["eng"],"team":["sales"]}}}`,
		`{"user.sub":{"alice":{"team":[null]}}}`,
		`{"user.sub":{"alice":{"team":["` + string([]byte{0xff}) + `"]}}}`,
	} {
		if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		send(validRequest, http.StatusServiceUnavailable)
		if err := os.WriteFile(path, []byte(validFile), 0600); err != nil {
			t.Fatal(err)
		}
		send(validRequest, http.StatusOK)
	}
}

func TestMalformedLookupRequestsNeverReachProvider(t *testing.T) {
	for _, body := range []string{
		`null`, `{}`, `{"identities":null}`, `{"identities":[null]}`,
		`{"identities":[],"identities":[]}`,
		`{"identities":[{"key":"user.sub","key":"device.zpr.adapter.cn","value":"alice"}]}`,
		`{"identities":[{"key":"user.sub","value":"alice","val\u0075e":"bob"}]}`,
		`{"identities":[{"key":null,"value":"alice"}]}`,
		`{"identities":[]} {}`,
		`{"identities":[],"unknown":true}`,
		`{"identities":[{"key":"user.sub","value":"` + strings.Repeat("a", 64<<10) + `"}]}`,
	} {
		t.Run(body[:min(len(body), 80)], func(t *testing.T) {
			store := &countingProvider{}
			request := httptest.NewRequest(http.MethodPost, "/v1/attributes", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler(store, nil).ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || store.calls != 0 {
				t.Fatalf("malformed request: status=%d provider calls=%d", response.Code, store.calls)
			}
		})
	}
	store := &countingProvider{}
	request := httptest.NewRequest(http.MethodPost, "/v1/attributes", strings.NewReader(`{"identities":[]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler(store, nil).ServeHTTP(response, request)
	if response.Code != http.StatusOK || store.calls != 1 {
		t.Fatalf("explicit empty identity list must remain valid: %d, %d", response.Code, store.calls)
	}
}

func TestLookupInputLimitsAndCaseSensitiveFileNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actors.json")
	const snapshot = `{"user.sub":{"Alice":{"team":["upper"]},"alice":{"team":["lower"]}}}`
	if err := os.WriteFile(path, []byte(snapshot+strings.Repeat(" ", (1<<20)-len(snapshot))), 0600); err != nil {
		t.Fatal(err)
	}
	store := fileProvider{path}
	attrs, err := store.lookup(context.Background(), []identity{{"user.sub", "Alice"}})
	if err != nil || len(attrs["team"]) != 1 || attrs["team"][0] != "upper" {
		t.Fatalf("exact-limit snapshot or case-sensitive subject failed: %v %v", attrs, err)
	}
	if err := os.WriteFile(path, []byte(snapshot+strings.Repeat(" ", (1<<20)+1-len(snapshot))), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.lookup(context.Background(), nil); err == nil {
		t.Fatal("snapshot one byte over limit accepted")
	}
	for _, count := range []int{64, 65} {
		identities := make([]identity, count)
		for i := range identities {
			identities[i] = identity{"user.sub", "alice"}
		}
		body, err := json.Marshal(lookupRequest{identities})
		if err != nil {
			t.Fatal(err)
		}
		provider := &countingProvider{}
		request := httptest.NewRequest(http.MethodPost, "/v1/attributes", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler(provider, nil).ServeHTTP(response, request)
		if count == 64 {
			if response.Code != http.StatusOK || provider.calls != 1 {
				t.Fatalf("64 identities rejected: %d %d", response.Code, provider.calls)
			}
		} else if response.Code != http.StatusBadRequest || provider.calls != 0 {
			t.Fatalf("65 identities reached provider: %d %d", response.Code, provider.calls)
		}
	}
	var target any
	if err := decodeLookupJSON([]byte(strings.Repeat("[", 66)+"0"+strings.Repeat("]", 66)), &target); err == nil {
		t.Fatal("excessive nesting accepted")
	}
}

func TestMutualTLSLookup(t *testing.T) {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test root"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	rootCert, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	makeCert := func(serial int64, server bool) tls.Certificate {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		cert := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "test"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour)}
		if server {
			cert.DNSNames = []string{"localhost"}
			cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		} else {
			cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
		}
		der, err := x509.CreateCertificate(rand.Reader, cert, rootCert, &key.PublicKey, rootKey)
		if err != nil {
			t.Fatal(err)
		}
		privateKey, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		certificate, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateKey}))
		if err != nil {
			t.Fatal(err)
		}
		return certificate
	}
	path := filepath.Join(t.TempDir(), "actors.json")
	if err := os.WriteFile(path, []byte(`{"user.sub":{"alice":{"team":["eng"]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(rootCert)
	server := httptest.NewUnstartedServer(handler(fileProvider{path}, nil))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{makeCert(2, true)}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13}
	server.StartTLS()
	defer server.Close()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, Certificates: []tls.Certificate{makeCert(3, false)}, ServerName: "localhost", MinVersion: tls.VersionTLS13}}}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/attributes", bytes.NewBufferString(`{"identities":[{"key":"user.sub","value":"alice"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("unexpected status: %s", response.Status)
	}
	unauthenticated := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "localhost"}}}
	request, err = http.NewRequest(http.MethodPost, server.URL+"/v1/attributes", bytes.NewBufferString(`{"identities":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if response, err := unauthenticated.Do(request); err == nil {
		response.Body.Close()
		t.Fatal("missing client certificate accepted")
	}
}

func TestHandler(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actors.json")
	if err := os.WriteFile(path, []byte(`{"user.sub":{"alice":{"team":["eng"]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	server := handler(fileProvider{path}, nil)
	for _, test := range []struct {
		body string
		code int
	}{
		{`{"identities":[{"key":"user.sub","value":"alice"}]}`, 200},
		{`{"identities":[{"key":"user.sub","value":""}]}`, 400},
		{`{"identities":[],"unexpected":true}`, 400},
		{`{"identities":[]} {}`, 400},
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/attributes", bytes.NewBufferString(test.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != test.code {
			t.Fatalf("%s: %d, want %d", test.body, response.Code, test.code)
		}
		if test.code == 200 && !strings.Contains(response.Body.String(), `"team":["eng"]`) {
			t.Fatal(response.Body.String())
		}
	}
}

func TestLDAPFilterEscapesIdentityAndRejectsUnsafeNames(t *testing.T) {
	if safeLDAPAttribute("cn)(userPassword=*") {
		t.Fatal("unsafe LDAP name accepted")
	}
	if !safeLDAPAttribute("uid") {
		t.Fatal("valid LDAP name rejected")
	}
	if got := ldapFilter("uid", "a*)(uid=*)"); got != `(uid=a\2a\29\28uid=\2a\29)` {
		t.Fatalf("unsafe filter: %s", got)
	}
}

func TestLDAPIdentityAttributesIncludeGroupNamesAsRoles(t *testing.T) {
	const personDN = "uid=alice,ou=People,dc=example,dc=org"
	searcher := ldapSearcherFunc(func(request *ldap.SearchRequest) (*ldap.SearchResult, error) {
		switch request.BaseDN {
		case "ou=People,dc=example,dc=org":
			if request.Filter != "(uid=alice)" {
				t.Fatalf("person filter = %q", request.Filter)
			}
			return &ldap.SearchResult{Entries: []*ldap.Entry{
				ldap.NewEntry(personDN, map[string][]string{"ou": {"Platform"}, "title": {"Engineer"}}),
			}}, nil
		case "ou=Roles,dc=example,dc=org":
			if request.Filter != "(member="+personDN+")" {
				t.Fatalf("group filter = %q", request.Filter)
			}
			return &ldap.SearchResult{Entries: []*ldap.Entry{
				ldap.NewEntry("cn=Security Reviewer,ou=Roles,dc=example,dc=org", map[string][]string{"cn": {"Security Reviewer"}}),
				ldap.NewEntry("cn=Approver,ou=Roles,dc=example,dc=org", map[string][]string{"cn": {"Approver"}}),
			}}, nil
		default:
			t.Fatalf("unexpected LDAP base DN %q", request.BaseDN)
			return nil, nil
		}
	})

	attributes, err := ldapIdentityAttributes(context.Background(), searcher, "ou=People,dc=example,dc=org", "ou=Roles,dc=example,dc=org", "uid", "alice", []string{"ou", "title"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(attributes["role"], ",") != "Approver,Security Reviewer" {
		t.Fatalf("role attributes = %v", attributes["role"])
	}
	if strings.Join(attributes["ou"], ",") != "Platform" || strings.Join(attributes["title"], ",") != "Engineer" {
		t.Fatalf("direct attributes = %v", attributes)
	}
}

func TestLDAPLookupRejectsIncompleteReferralResults(t *testing.T) {
	for _, groupReferral := range []bool{false, true} {
		searcher := ldapSearcherFunc(func(request *ldap.SearchRequest) (*ldap.SearchResult, error) {
			if request.BaseDN == "people" {
				result := &ldap.SearchResult{Entries: []*ldap.Entry{
					ldap.NewEntry("uid=alice,dc=example", map[string][]string{"team": {"eng"}}),
				}}
				if !groupReferral {
					result.Referrals = []string{"ldaps://other.example/dc=other"}
				}
				return result, nil
			}
			return &ldap.SearchResult{Referrals: []string{"ldaps://other.example/dc=roles"}}, nil
		})
		attrs, err := ldapIdentityAttributes(context.Background(), searcher, "people", "groups", "uid", "alice", []string{"team"})
		if err == nil || attrs != nil {
			t.Errorf("incomplete referral result returned claims (groups=%v): %v %v", groupReferral, attrs, err)
		}
	}
}

func TestLDAPGroupRolesCannotOverwriteDirectRoles(t *testing.T) {
	for _, direct := range [][]string{{"Approver"}, {"Reader"}, nil} {
		searcher := ldapSearcherFunc(func(request *ldap.SearchRequest) (*ldap.SearchResult, error) {
			if request.BaseDN == "people" {
				attributes := map[string][]string{"team": {"eng"}}
				if direct != nil {
					attributes["role"] = direct
				}
				return &ldap.SearchResult{Entries: []*ldap.Entry{
					ldap.NewEntry("uid=alice,dc=example", attributes),
				}}, nil
			}
			return &ldap.SearchResult{Entries: []*ldap.Entry{
				ldap.NewEntry("cn=Approver,dc=example", map[string][]string{"cn": {"Approver"}}),
			}}, nil
		})
		attrs, err := ldapIdentityAttributes(context.Background(), searcher, "people", "groups", "uid", "alice", []string{"role", "team"})
		if len(direct) > 0 && direct[0] == "Reader" {
			if err == nil || attrs != nil {
				t.Fatalf("group roles overwrote conflicting direct role: %v %v", attrs, err)
			}
		} else if err != nil || strings.Join(attrs["role"], ",") != "Approver" {
			t.Fatalf("valid roles rejected: %v %v", attrs, err)
		}
	}
}

func TestLDAPLookupCancellationAfterSearchDiscardsClaims(t *testing.T) {
	for _, cancelAt := range []string{"before", "people", "groups"} {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		if cancelAt == "before" {
			cancel()
		}
		searcher := ldapSearcherFunc(func(request *ldap.SearchRequest) (*ldap.SearchResult, error) {
			calls++
			if request.BaseDN == "people" {
				if cancelAt == "people" {
					cancel()
				}
				return &ldap.SearchResult{Entries: []*ldap.Entry{
					ldap.NewEntry("uid=alice,dc=example", map[string][]string{"team": {"eng"}}),
				}}, nil
			}
			cancel()
			return &ldap.SearchResult{}, nil
		})
		groupsBase := ""
		if cancelAt == "groups" {
			groupsBase = "groups"
		}
		attrs, err := ldapIdentityAttributes(ctx, searcher, "people", groupsBase, "uid", "alice", []string{"team"})
		if !errors.Is(err, context.Canceled) || attrs != nil {
			t.Errorf("canceled %s lookup returned claims: %v %v", cancelAt, attrs, err)
		}
		if cancelAt == "before" && calls != 0 {
			t.Fatal("already canceled lookup searched LDAP")
		}
	}
}

func TestLDAPDuplicateAttributesAndMalformedGroupNamesFail(t *testing.T) {
	for _, test := range []struct {
		name   string
		person *ldap.Entry
		group  *ldap.Entry
	}{
		{"duplicate person attribute", &ldap.Entry{DN: "uid=alice,dc=example", Attributes: []*ldap.EntryAttribute{
			{Name: "team", Values: []string{"sales"}}, {Name: "TEAM", Values: []string{"eng"}},
		}}, nil},
		{"duplicate group name", ldap.NewEntry("uid=alice,dc=example", nil), &ldap.Entry{DN: "cn=group,dc=example", Attributes: []*ldap.EntryAttribute{
			{Name: "cn", Values: []string{"Reader"}}, {Name: "CN", Values: []string{"Approver"}},
		}}},
		{"multiple group names", ldap.NewEntry("uid=alice,dc=example", nil), ldap.NewEntry("cn=group,dc=example", map[string][]string{"cn": {"Reader", "Approver"}})},
		{"missing group name", ldap.NewEntry("uid=alice,dc=example", nil), ldap.NewEntry("cn=group,dc=example", nil)},
		{"empty group name", ldap.NewEntry("uid=alice,dc=example", nil), ldap.NewEntry("cn=group,dc=example", map[string][]string{"cn": {""}})},
	} {
		t.Run(test.name, func(t *testing.T) {
			searcher := ldapSearcherFunc(func(request *ldap.SearchRequest) (*ldap.SearchResult, error) {
				if request.BaseDN == "people" {
					return &ldap.SearchResult{Entries: []*ldap.Entry{test.person}}, nil
				}
				if test.group == nil {
					return &ldap.SearchResult{}, nil
				}
				return &ldap.SearchResult{Entries: []*ldap.Entry{test.group}}, nil
			})
			attrs, err := ldapIdentityAttributes(context.Background(), searcher, "people", "groups", "uid", "alice", []string{"team"})
			if err == nil || attrs != nil {
				t.Fatalf("ambiguous LDAP data returned claims: %v %v", attrs, err)
			}
		})
	}
}
