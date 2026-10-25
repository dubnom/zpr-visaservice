package main

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"net"
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

const subjectLookupUUID = "b6a21ab4-5554-4cde-ae65-41a406a3c07f"

type subjectLDAPFixture struct {
	config   assertionSourceConfig
	mode     atomic.Int32
	searches atomic.Int64
	entered  chan struct{}
	release  chan struct{}
}

func subjectLDAP(t *testing.T) *subjectLDAPFixture {
	t.Helper()
	t.Setenv("SIMULATOR_URL", "http://127.0.0.1:0")
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	certServer := httptest.NewTLSServer(nil)
	certs, cert := certServer.TLS.Certificates, certServer.Certificate().Raw
	certServer.Close()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: certs, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	password, ca := filepath.Join(dir, "password"), filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(password, []byte("fixture-password"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := &subjectLDAPFixture{entered: make(chan struct{}, 1), release: make(chan struct{}),
		config: assertionSourceConfig{Name: "company", Version: 1, Kind: "ldap", SubjectSelector: "ldap_entry_uuid",
			URL: "ldaps://" + listener.Addr().String(), CAFile: ca, Attributes: []string{"cn", "mail"},
			Organizations: map[string]assertionSourceScope{"production": {
				BaseDN: "dc=company", BindDN: "cn=reader,dc=company", BindPasswordFile: password,
			}}}}
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
						if fixture.mode.Load() == 8 || op.Children[1].Value != "cn=reader,dc=company" ||
							op.Children[2].Data.String() != "fixture-password" {
							code = ldap.LDAPResultInvalidCredentials
						}
						packetAResult(conn, id, ldap.ApplicationBindResponse, code)
					case ldap.ApplicationSearchRequest:
						fixture.searches.Add(1)
						filter, err := ldap.DecompileFilter(op.Children[6])
						if err != nil || filter != "(entryUUID="+subjectLookupUUID+")" ||
							op.Children[0].Value != "dc=company" ||
							op.Children[1].Value != int64(ldap.ScopeWholeSubtree) ||
							op.Children[2].Value != int64(ldap.NeverDerefAliases) ||
							op.Children[3].Value != int64(2) || op.Children[4].Value != int64(8) {
							t.Error("immutable lookup escaped exact filter, scope or limits", filter, err)
						}
						for _, a := range op.Children[7].Children {
							if a.Value != "entryUUID" && a.Value != "mail" && a.Value != "cn" {
								t.Error("lookup requested unapproved attributes", a.Value)
							}
						}
						mode := fixture.mode.Load()
						if mode == 9 {
							select {
							case fixture.entered <- struct{}{}:
							default:
							}
							select {
							case <-fixture.release:
							case <-time.After(3 * time.Second):
								return
							}
						}
						if mode == 7 {
							packetAResult(conn, id, ldap.ApplicationSearchResultDone, ldap.LDAPResultSizeLimitExceeded)
							continue
						}
						if mode == 6 {
							reference := ber.Encode(ber.ClassApplication, ber.TypeConstructed, ldap.ApplicationSearchResultReference, nil, "")
							reference.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "ldaps://other.example", ""))
							packetASend(conn, id, reference)
						}
						if mode != 1 {
							count := 1
							if mode == 2 {
								count = 2
							}
							for i := 0; i < count; i++ {
								dn := "uid=renamed-person,dc=company"
								if mode == 3 {
									dn = "uid=person,dc=other"
								}
								attrs := map[string][]string{"entryUUID": {subjectLookupUUID}, "mail": {"person@example.test"},
									"userPassword": {"must-not-escape"}}
								if mode == 4 {
									attrs["entryUUID"] = []string{"00000000-0000-0000-0000-000000000001"}
								}
								if mode == 5 {
									delete(attrs, "entryUUID")
								}
								if mode == 10 {
									attrs["entryUUID"] = []string{subjectLookupUUID, subjectLookupUUID}
								}
								if mode == 11 {
									attrs["ENTRYUUID"] = []string{subjectLookupUUID}
								}
								if mode == 12 {
									attrs["entryUUID"] = []string{strings.ToUpper(subjectLookupUUID)}
								}
								subjectLDAPEntry(conn, id, dn, attrs)
							}
						}
						packetAResult(conn, id, ldap.ApplicationSearchResultDone, 0)
					case ldap.ApplicationUnbindRequest:
						return
					default:
						t.Error("unexpected LDAP operation; writes are forbidden", op.Tag)
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); workers.Wait() })
	return fixture
}

func subjectLDAPEntry(conn net.Conn, id int64, dn string, attributes map[string][]string) {
	op := ber.Encode(ber.ClassApplication, ber.TypeConstructed, ldap.ApplicationSearchResultEntry, nil, "")
	op.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, dn, ""))
	attrs := ber.NewSequence("")
	for name, values := range attributes {
		item := ber.NewSequence("")
		item.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, name, ""))
		set := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSet, nil, "")
		for _, value := range values {
			set.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, value, ""))
		}
		item.AppendChild(set)
		attrs.AppendChild(item)
	}
	op.AppendChild(attrs)
	packetASend(conn, id, op)
}

func TestImmutableLDAPSubjectExactLookupAndFailureModes(t *testing.T) {
	fixture := subjectLDAP(t)
	read := func() (map[string][]string, error) {
		return lookupAssertionLDAPSubject(context.Background(), fixture.config, "production", subjectLookupUUID, []string{"mail"})
	}
	values, err := read()
	if err != nil || !reflect.DeepEqual(values, map[string][]string{"mail": {"person@example.test"}}) {
		t.Fatal("exact UUID response leaked structural/unapproved attributes or failed", values, err)
	}
	for _, mode := range []int32{1, 2, 3, 4, 5, 6, 7, 8, 10, 11, 12} {
		fixture.mode.Store(mode)
		if values, err := read(); err == nil || values != nil ||
			strings.Contains(err.Error(), "fixture-password") || strings.Contains(err.Error(), "fixture-bind-secret") {
			t.Fatal("invalid response accepted or diagnostic leaked", mode, values, err)
		}
	}
	fixture.mode.Store(0)
	for _, id := range []string{"person", "*)(uid=*)", strings.ToUpper(subjectLookupUUID), "00000000-0000-0000-0000-000000000000"} {
		before := fixture.searches.Load()
		if _, err := lookupAssertionLDAPSubject(context.Background(), fixture.config, "production", id, []string{"mail"}); err == nil ||
			fixture.searches.Load() != before {
			t.Fatal("invalid ID performed LDAP lookup", id, err)
		}
	}
	for _, attributes := range [][]string{nil, {"uid"}, {"userPassword"}, {"mail", "mail"}} {
		if _, err := lookupAssertionLDAPSubject(context.Background(), fixture.config, "production", subjectLookupUUID, attributes); err == nil {
			t.Fatal("unapproved attributes accepted", attributes)
		}
	}
	if _, err := lookupAssertionLDAPSubject(context.Background(), fixture.config, "foreign", subjectLookupUUID, []string{"mail"}); err == nil {
		t.Fatal("foreign scope accepted")
	}
	config := fixture.config
	config.SubjectSelector = ""
	if _, err := lookupAssertionLDAPSubject(context.Background(), config, "production", subjectLookupUUID, []string{"mail"}); err == nil {
		t.Fatal("undeclared selector accepted")
	}
	config = fixture.config
	config.CAFile = ""
	if _, err := lookupAssertionLDAPSubject(context.Background(), config, "production", subjectLookupUUID, []string{"mail"}); err == nil {
		t.Fatal("untrusted LDAP certificate accepted")
	}
}

func TestImmutableLDAPSubjectCancellationInterruptsSearch(t *testing.T) {
	fixture := subjectLDAP(t)
	fixture.mode.Store(9)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := lookupAssertionLDAPSubject(ctx, fixture.config, "production", subjectLookupUUID, []string{"mail"})
		result <- err
	}()
	select {
	case <-fixture.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("lookup did not enter search")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancelled lookup succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation failed to interrupt lookup")
	}
	close(fixture.release)
}
