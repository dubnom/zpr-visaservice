package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"
)

type testSyncResponse struct {
	entry    *ldap.Entry
	controls []ldap.Control
	err      error
}

func (response testSyncResponse) Entry() *ldap.Entry       { return response.entry }
func (response testSyncResponse) Controls() []ldap.Control { return response.controls }
func (response testSyncResponse) Referral() string         { return "" }
func (response testSyncResponse) Err() error               { return response.err }
func (response testSyncResponse) Next() bool               { return false }

func TestLDAPSyncEntryMetadataAndCookie(t *testing.T) {
	for _, state := range []ldap.ControlSyncStateState{ldap.SyncStatePresent, ldap.SyncStateAdd, ldap.SyncStateModify, ldap.SyncStateDelete} {
		response := testSyncResponse{
			entry:    ldap.NewEntry("uid=alice,dc=test", map[string][]string{"userPassword": {"secret"}}),
			controls: []ldap.Control{&ldap.ControlSyncState{State: state, EntryUUID: uuid.MustParse("12345678-1234-1234-1234-123456789012"), Cookie: []byte("checkpoint")}},
		}
		var events []ldapChangeEvent
		cookie, err := consumeLDAPSyncResponse(response, func(event ldapChangeEvent) error { events = append(events, event); return nil })
		if err != nil || string(cookie) != "checkpoint" || len(events) != 1 || events[0].DN != "uid=alice,dc=test" || events[0].UUID == "" {
			t.Fatalf("unexpected sync result: %v %v %v", cookie, events, err)
		}
	}
}

func TestLDAPSyncOutputFailureStopsConsumer(t *testing.T) {
	expected := errors.New("sink failed")
	err := consumeLDAPChanges(context.Background(), func([]byte) (ldap.Response, func(), error) {
		t.Fatal("must not dial when sink failed")
		return nil, nil, nil
	}, func(ldapChangeEvent) error { return expected })
	if !errors.Is(err, expected) {
		t.Fatalf("expected sink error, got %v", err)
	}
}

func TestLDAPSyncCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var events []ldapChangeEvent
	err := consumeLDAPChanges(ctx, func([]byte) (ldap.Response, func(), error) {
		return testSyncResponse{}, func() { cancel() }, nil
	}, func(event ldapChangeEvent) error { events = append(events, event); return nil })
	if err != nil || len(events) != 1 || events[0].Type != "refresh_start" {
		t.Fatalf("unexpected cancellation result: %v %v", events, err)
	}
}

type testSyncStream struct {
	testSyncResponse
	remaining int
}

func (stream *testSyncStream) Next() bool {
	if stream.remaining == 0 {
		return false
	}
	stream.remaining--
	return true
}

func TestLDAPSyncReconnectResumesCookie(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	attempts := 0
	err := consumeLDAPChanges(ctx, func(cookie []byte) (ldap.Response, func(), error) {
		attempts++
		if attempts == 1 {
			return &testSyncStream{testSyncResponse: testSyncResponse{controls: []ldap.Control{&ldap.ControlSyncInfo{Value: ldap.SyncInfoNewcookie, NewCookie: &ldap.ControlSyncInfoNewCookie{Cookie: []byte("resume")}}}}, remaining: 1}, func() {}, nil
		}
		if string(cookie) != "resume" {
			t.Errorf("reconnect lost cookie: %q", cookie)
		}
		cancel()
		return testSyncResponse{}, func() {}, nil
	}, func(ldapChangeEvent) error { return nil })
	if err != nil || attempts != 2 {
		t.Fatalf("reconnect: attempts=%d error=%v", attempts, err)
	}
}

func TestLDAPSyncRejectedCookieStartsNewRefresh(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	attempts := 0
	resync := false
	err := consumeLDAPChanges(ctx, func(cookie []byte) (ldap.Response, func(), error) {
		attempts++
		switch attempts {
		case 1:
			return &testSyncStream{testSyncResponse: testSyncResponse{controls: []ldap.Control{&ldap.ControlSyncDone{Cookie: []byte("old")}}}, remaining: 1}, func() {}, nil
		case 2:
			return nil, nil, ldap.NewError(4096, errors.New("refresh required"))
		default:
			if len(cookie) != 0 {
				t.Error("rejected cookie reused")
			}
			cancel()
			return testSyncResponse{}, func() {}, nil
		}
	}, func(event ldapChangeEvent) error { resync = resync || event.Type == "resync_required"; return nil })
	if err != nil || attempts != 3 || !resync {
		t.Fatalf("resync: attempts=%d marker=%v error=%v", attempts, resync, err)
	}
}

func TestLDAPSyncRefreshAndIdentitySets(t *testing.T) {
	for _, deletes := range []bool{false, true} {
		entryUUID := uuid.New()
		response := testSyncResponse{controls: []ldap.Control{
			&ldap.ControlSyncInfo{Value: ldap.SyncInfoSyncIdSet, SyncIdSet: &ldap.ControlSyncInfoSyncIdSet{Cookie: []byte("set"), RefreshDeletes: deletes, SyncUUIDs: []uuid.UUID{entryUUID}}},
			&ldap.ControlSyncInfo{Value: ldap.SyncInfoRefreshPresent, RefreshPresent: &ldap.ControlSyncInfoRefreshPresent{RefreshDone: true, Cookie: []byte("ready")}},
		}}
		var events []ldapChangeEvent
		cookie, err := consumeLDAPSyncResponse(response, func(event ldapChangeEvent) error { events = append(events, event); return nil })
		if err != nil || string(cookie) != "ready" || len(events) != 2 || events[1].Type != "refresh_complete" {
			t.Fatalf("refresh result: %v %v %v", cookie, events, err)
		}
		expected := "present"
		if deletes {
			expected = "delete"
		}
		if events[0].Type != expected || events[0].UUID != entryUUID.String() {
			t.Fatalf("identity set: %v", events[0])
		}
	}
}

type testLDAPEventWriter struct {
	events chan ldapChangeEvent
}

func (writer testLDAPEventWriter) Write(data []byte) (int, error) {
	var event ldapChangeEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return 0, err
	}
	writer.events <- event
	return len(data), nil
}

func TestLDAPSyncLiveOpenLDAP(t *testing.T) {
	if os.Getenv("ZPR_TEST_LDAP_SYNC") != "1" {
		t.Skip("set ZPR_TEST_LDAP_SYNC=1 on Linux with OpenLDAP and openssl installed")
	}
	directory := t.TempDir()
	run := func(name string, arguments ...string) []byte {
		t.Helper()
		output, err := exec.Command(name, arguments...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed: %v", name, err)
		}
		return output
	}
	caFile := filepath.Join(directory, "ldap.crt")
	keyFile := filepath.Join(directory, "ldap.key")
	run("openssl", "req", "-new", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-subj", "/CN=localhost", "-addext", "subjectAltName=IP:127.0.0.1", "-keyout", keyFile, "-out", caFile)
	password := uuid.NewString()
	passwordFile := filepath.Join(directory, "password")
	if err := os.WriteFile(passwordFile, []byte(password), 0600); err != nil {
		t.Fatal(err)
	}
	hash := strings.TrimSpace(string(run("slappasswd", "-T", passwordFile)))
	dataDir := filepath.Join(directory, "data")
	if err := os.Mkdir(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(directory, "slapd.conf")
	config := fmt.Sprintf("include /etc/ldap/schema/core.schema\ninclude /etc/ldap/schema/cosine.schema\ninclude /etc/ldap/schema/inetorgperson.schema\npidfile %s/slapd.pid\nmodulepath /usr/lib/ldap\nmoduleload back_mdb\nmoduleload syncprov\nTLSCertificateFile %s\nTLSCertificateKeyFile %s\ndatabase mdb\nmaxsize 10485760\nsuffix dc=feed,dc=test\nrootdn cn=admin,dc=feed,dc=test\nrootpw %s\ndirectory %s\nindex entryUUID eq\nindex entryCSN eq\noverlay syncprov\nsyncprov-sessionlog 100\n", directory, caFile, keyFile, hash, dataDir)
	if err := os.WriteFile(configFile, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	seed := exec.Command("slapadd", "-f", configFile)
	seed.Stdin = strings.NewReader("dn: dc=feed,dc=test\nobjectClass: domain\ndc: feed\n")
	if output, err := seed.CombinedOutput(); err != nil {
		t.Fatalf("slapadd: %v %s", err, output)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	uri := "ldaps://" + listener.Addr().String()
	listener.Close()
	server := exec.Command("slapd", "-f", configFile, "-h", uri, "-d", "0")
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Process.Kill(); server.Wait() })
	roots := x509.NewCertPool()
	cert, err := os.ReadFile(caFile)
	if err != nil || !roots.AppendCertsFromPEM(cert) {
		t.Fatal("cannot load test CA")
	}
	var conn *ldap.Conn
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err = ldap.DialURL(uri, ldap.DialWithTLSConfig(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}), ldap.DialWithDialer(&net.Dialer{Timeout: time.Second}))
		if err == nil {
			break
		}
		<-time.After(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal("test LDAP did not start")
	}
	defer conn.Close()
	conn.SetTimeout(5 * time.Second)
	if err := conn.Bind("cn=admin,dc=feed,dc=test", password); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	events := make(chan ldapChangeEvent, 64)
	done := make(chan error, 1)
	go func() {
		done <- watchLDAP(ctx, uri, caFile, "dc=feed,dc=test", "cn=admin,dc=feed,dc=test", passwordFile, testLDAPEventWriter{events: events})
	}()
	await := func(eventType, dn string) ldapChangeEvent {
		t.Helper()
		for {
			select {
			case event := <-events:
				if event.Type == eventType && (dn == "" || event.DN == dn) {
					if event.Version != 1 || event.BaseDN != "dc=feed,dc=test" {
						t.Fatalf("missing event contract fields: %+v", event)
					}
					return event
				}
			case err := <-done:
				t.Fatalf("consumer ended: %v", err)
			case <-ctx.Done():
				t.Fatalf("timed out waiting for %s", eventType)
			}
		}
	}
	await("refresh_complete", "")
	dn := "uid=feed-probe,dc=feed,dc=test"
	add := ldap.NewAddRequest(dn, nil)
	add.Attribute("objectClass", []string{"inetOrgPerson"})
	add.Attribute("cn", []string{"Feed probe"})
	add.Attribute("sn", []string{"Probe"})
	add.Attribute("uid", []string{"feed-probe"})
	if err := conn.Add(add); err != nil {
		t.Fatal(err)
	}
	added := await("add", dn)
	modify := ldap.NewModifyRequest(dn, nil)
	modify.Replace("description", []string{"value-not-in-feed"})
	if err := conn.Modify(modify); err != nil {
		t.Fatal(err)
	}
	modified := await("modify", dn)
	if err := conn.Del(ldap.NewDelRequest(dn, nil)); err != nil {
		t.Fatal(err)
	}
	deleted := await("delete", dn)
	if added.UUID == "" || added.UUID != modified.UUID || added.UUID != deleted.UUID {
		t.Fatal("entry UUID did not remain stable through change events")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not stop after cancellation")
	}
}
