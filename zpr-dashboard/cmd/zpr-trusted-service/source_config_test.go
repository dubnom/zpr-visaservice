package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"
	"github.com/go-ldap/ldap/v3"
)

const providerUUID = "b6a21ab4-5554-4cde-ae65-41a406a3c07f"

func immutableEntry() *ldap.Entry {
	return ldap.NewEntry("uid=renamed,dc=company", map[string][]string{
		"entryUUID": {providerUUID}, "mail": {"person@example.test"}, "userPassword": {"never-return"},
	})
}

func TestImmutableSourceExactLookupAndFailures(t *testing.T) {
	cases := []struct {
		name   string
		result *ldap.SearchResult
		err    error
	}{
		{"valid", &ldap.SearchResult{Entries: []*ldap.Entry{immutableEntry()}}, nil},
		{"missing", &ldap.SearchResult{}, nil},
		{"ambiguous", &ldap.SearchResult{Entries: []*ldap.Entry{immutableEntry(), immutableEntry()}}, nil},
		{"outside-base", &ldap.SearchResult{Entries: []*ldap.Entry{ldap.NewEntry("uid=x,dc=other", map[string][]string{"entryUUID": {providerUUID}})}}, nil},
		{"missing-uuid", &ldap.SearchResult{Entries: []*ldap.Entry{ldap.NewEntry("uid=x,dc=company", map[string][]string{"mail": {"x"}})}}, nil},
		{"wrong-uuid", &ldap.SearchResult{Entries: []*ldap.Entry{ldap.NewEntry("uid=x,dc=company", map[string][]string{"entryUUID": {"b6a21ab4-5554-4cde-ae65-41a406a3c07e"}})}}, nil},
		{"multi-uuid", &ldap.SearchResult{Entries: []*ldap.Entry{ldap.NewEntry("uid=x,dc=company", map[string][]string{"entryUUID": {providerUUID, providerUUID}})}}, nil},
		{"duplicate-uuid", &ldap.SearchResult{Entries: []*ldap.Entry{ldap.NewEntry("uid=x,dc=company", map[string][]string{"entryUUID": {providerUUID}, "ENTRYUUID": {providerUUID}})}}, nil},
		{"duplicate-mail", &ldap.SearchResult{Entries: []*ldap.Entry{ldap.NewEntry("uid=x,dc=company", map[string][]string{"entryUUID": {providerUUID}, "mail": {"x"}, "MAIL": {"x"}})}}, nil},
		{"referral", &ldap.SearchResult{Entries: []*ldap.Entry{immutableEntry()}, Referrals: []string{"ldaps://other"}}, nil},
		{"partial", &ldap.SearchResult{Entries: []*ldap.Entry{immutableEntry()}}, ldap.NewError(ldap.LDAPResultSizeLimitExceeded, errors.New("partial"))},
		{"nil-result", nil, nil},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			search := ldapSearcherFunc(func(request *ldap.SearchRequest) (*ldap.SearchResult, error) {
				if request.Filter != "(entryUUID="+providerUUID+")" || request.SizeLimit != 2 ||
					request.TimeLimit != 5 || request.Scope != ldap.ScopeWholeSubtree ||
					request.DerefAliases != ldap.NeverDerefAliases || request.BaseDN != "dc=company" ||
					!reflect.DeepEqual(request.Attributes, []string{"mail", "entryUUID"}) {
					t.Fatalf("not an exact bounded allowlisted query: %+v", request)
				}
				return test.result, test.err
			})
			values, err := immutableLDAPAttributes(context.Background(), search, "dc=company", providerUUID, []string{"mail"})
			if test.name == "valid" {
				if err != nil || !reflect.DeepEqual(values, map[string][]string{"mail": {"person@example.test"}}) {
					t.Fatalf("%v %v", values, err)
				}
			} else if err == nil {
				t.Fatal("ambiguous/partial evidence became a successful observation")
			}
		})
	}
	called := false
	search := ldapSearcherFunc(func(*ldap.SearchRequest) (*ldap.SearchResult, error) { called = true; return nil, nil })
	for _, value := range []string{"", "alice", strings.ToUpper(providerUUID), "00000000-0000-0000-0000-000000000000", "*)(uid=*)"} {
		if _, err := immutableLDAPAttributes(context.Background(), search, "dc=company", value, []string{"mail"}); err == nil {
			t.Fatal("invalid UUID accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := immutableLDAPAttributes(ctx, search, "dc=company", providerUUID, []string{"mail"}); err == nil || called {
		t.Fatal("invalid/canceled lookup reached directory")
	}
}

type immutableDirectoryFixture struct {
	config   sourceConfig
	path     string
	searches atomic.Int64
	mode     atomic.Int32
	entered  chan struct{}
	release  chan struct{}
}

func immutableDirectory(t *testing.T) *immutableDirectoryFixture {
	t.Helper()
	certServer := httptest.NewTLSServer(nil)
	certs, certificate := certServer.TLS.Certificates, certServer.Certificate().Raw
	certServer.Close()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: certs, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	password, ca := filepath.Join(root, "password"), filepath.Join(root, "ca.pem")
	for name, content := range map[string][]byte{password: []byte("fixture-password\n"), ca: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})} {
		if err := os.WriteFile(name, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fixture := &immutableDirectoryFixture{config: sourceConfig{
		SourceID: "company", SubjectSelector: "ldap_entry_uuid", IdentityKey: "user.directory_uuid",
		URL: "ldaps://" + listener.Addr().String(), CAFile: ca, BaseDN: "dc=company", BindDN: "cn=reader,dc=company", BindPasswordFile: password,
		Attributes: []string{"mail"},
	}, path: filepath.Join(root, "source.json"), entered: make(chan struct{}, 1), release: make(chan struct{})}
	fixture.write(t, fixture.config)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				for {
					packet, err := ber.ReadPacket(conn)
					if err != nil {
						return
					}
					id, op := packet.Children[0].Value.(int64), packet.Children[1]
					switch op.Tag {
					case ldap.ApplicationBindRequest:
						code := int64(0)
						if fixture.mode.Load() == 1 || op.Children[1].Value != fixture.config.BindDN || op.Children[2].Data.String() != "fixture-password" {
							code = ldap.LDAPResultInvalidCredentials
						}
						immutableResult(conn, id, ldap.ApplicationBindResponse, code)
					case ldap.ApplicationSearchRequest:
						fixture.searches.Add(1)
						filter, err := ldap.DecompileFilter(op.Children[6])
						if err != nil || filter != "(entryUUID="+providerUUID+")" || op.Children[3].Value != int64(2) || op.Children[0].Value != "dc=company" {
							t.Error("provider violated exact UUID query", filter, err)
						}
						if fixture.mode.Load() == 2 {
							fixture.entered <- struct{}{}
							<-fixture.release
						}
						entry := immutableEntry()
						response := ber.Encode(ber.ClassApplication, ber.TypeConstructed, ldap.ApplicationSearchResultEntry, nil, "")
						response.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, entry.DN, ""))
						attributes := ber.NewSequence("")
						for _, attribute := range entry.Attributes {
							item := ber.NewSequence("")
							item.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, attribute.Name, ""))
							values := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSet, nil, "")
							for _, value := range attribute.Values {
								values.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, value, ""))
							}
							item.AppendChild(values)
							attributes.AppendChild(item)
						}
						response.AppendChild(attributes)
						immutableSend(conn, id, response)
						immutableResult(conn, id, ldap.ApplicationSearchResultDone, 0)
					case ldap.ApplicationUnbindRequest:
						return
					default:
						t.Error("unexpected directory operation", op.Tag)
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { close(fixture.release); _ = listener.Close(); workers.Wait() })
	return fixture
}

func (fixture *immutableDirectoryFixture) write(t *testing.T, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func immutableSend(conn net.Conn, id int64, op *ber.Packet) {
	packet := ber.NewSequence("")
	packet.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger, id, ""))
	packet.AppendChild(op)
	_, _ = conn.Write(packet.Bytes())
}

func immutableResult(conn net.Conn, id int64, tag ber.Tag, code int64) {
	result := ber.Encode(ber.ClassApplication, ber.TypeConstructed, tag, nil, "")
	result.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagEnumerated, code, ""))
	for range 2 {
		result.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", ""))
	}
	immutableSend(conn, id, result)
}

func TestConfiguredSourceRESTDeliveryAndDirectoryFailures(t *testing.T) {
	t.Setenv("SIMULATOR_URL", "http://127.0.0.1:0")
	t.Setenv("SIMULATION_MANIFEST", "/unavailable")
	fixture := immutableDirectory(t)
	store, err := configuredSource(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	// Use the existing mTLS contract with fixture-only certificates, not
	// production keys. The directory side independently verifies its TLS CA.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(101), DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	certificates := []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	server := httptest.NewUnstartedServer(handlerWithValidity(store, nil, time.Minute))
	server.TLS = &tls.Config{Certificates: certificates, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13}
	server.StartTLS()
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, Certificates: certificates, ServerName: "localhost", MinVersion: tls.VersionTLS13}}}
	defer client.CloseIdleConnections()
	request := func() *http.Response {
		t.Helper()
		response, err := client.Post(server.URL+"/v1/attributes", "application/json", strings.NewReader(`{"identities":[{"key":"user.directory_uuid","value":"`+providerUUID+`"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	response := request()
	var result lookupResponse
	err = json.NewDecoder(response.Body).Decode(&result)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || result.ExpiresUnix <= time.Now().Unix() ||
		!reflect.DeepEqual(result.Attributes, map[string][]string{"mail": {"person@example.test"}}) {
		t.Fatalf("delivery: %+v %v %d", result, err, response.StatusCode)
	}
	fixture.mode.Store(1)
	response = request()
	response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("bind failure became an empty successful observation")
	}
	fixture.mode.Store(0)
	if err := os.WriteFile(fixture.config.BindPasswordFile, []byte("wrong-password"), 0o600); err != nil {
		t.Fatal(err)
	}
	response = request()
	response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("provider cached old credential after rotation")
	}
	if err := os.WriteFile(fixture.config.BindPasswordFile, []byte("fixture-password"), 0o600); err != nil {
		t.Fatal(err)
	}
	store.ldap.ca = x509.NewCertPool()
	if _, err := store.lookup(context.Background(), []identity{{fixture.config.IdentityKey, providerUUID}}); err == nil {
		t.Fatal("untrusted LDAP server accepted")
	}
}

func TestConfiguredSourceCancellationAndIdentityBoundary(t *testing.T) {
	fixture := immutableDirectory(t)
	store, err := configuredSource(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, identities := range [][]identity{
		{{"device.zpr.adapter.cn", providerUUID}}, {{fixture.config.IdentityKey, "alice"}},
		{{fixture.config.IdentityKey, ""}, {fixture.config.IdentityKey, providerUUID}},
		{{fixture.config.IdentityKey, providerUUID}, {fixture.config.IdentityKey, providerUUID}},
	} {
		if _, err := store.lookup(context.Background(), identities); err == nil {
			t.Fatal("identity ambiguity/name fallback accepted")
		}
	}
	if fixture.searches.Load() != 0 {
		t.Fatal("invalid identities reached directory")
	}
	fixture.mode.Store(2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := store.lookup(ctx, []identity{{fixture.config.IdentityKey, providerUUID}})
		done <- err
	}()
	select {
	case <-fixture.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("search did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled lookup succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt LDAP socket")
	}
}

func TestConfiguredSourceRejectsUnsafeConfiguration(t *testing.T) {
	fixture := immutableDirectory(t)
	original, err := os.ReadFile(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(map[string]any){
		func(c map[string]any) { c["source_id"] = "company__source__other" },
		func(c map[string]any) { c["subject_selector"] = "uid" },
		func(c map[string]any) { c["url"] = "ldap://example.test" },
		func(c map[string]any) { c["url"] = "ldaps://user:secret@example.test" },
		func(c map[string]any) { c["base_dn"] = "" },
		func(c map[string]any) { c["attributes"] = []string{"mail", "MAIL"} },
		func(c map[string]any) { c["attributes"] = []string{"userpassword"} },
		func(c map[string]any) { c["attributes"] = []string{"accesstoken"} },
		func(c map[string]any) { c["bind_password_file"] = "relative" },
		func(c map[string]any) { c["SOURCE_ID"] = "other" },
		func(c map[string]any) { c["password"] = "inline-secret" },
	} {
		var value map[string]any
		if err := json.Unmarshal(original, &value); err != nil {
			t.Fatal(err)
		}
		edit(value)
		fixture.write(t, value)
		if _, err := configuredSource(fixture.path); err == nil {
			t.Fatalf("unsafe config accepted: %v", value)
		}
	}
	for _, data := range [][]byte{
		bytes.Replace(original, []byte(`"source_id":"company"`), []byte(`"source_id":"company","source_id":"other"`), 1),
		[]byte(`{"source_id":null}`),
		append(original, []byte("{}")...),
		bytes.Replace(original, []byte(`"company"`), []byte(`"\ud800"`), 1),
	} {
		if err := os.WriteFile(fixture.path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := configuredSource(fixture.path); err == nil {
			t.Fatal("ambiguous JSON accepted")
		}
	}
	if err := os.WriteFile(fixture.path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(fixture.config.BindPasswordFile, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := configuredSource(fixture.path); err == nil {
		t.Fatal("public credential file accepted")
	}
}
