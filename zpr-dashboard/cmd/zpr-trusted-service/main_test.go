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
	"encoding/pem"
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
