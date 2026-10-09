package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"
	"github.com/go-ldap/ldap/v3"
)

type fakeAccessLog struct {
	entries  []*ldap.Entry
	searches []string
}

func accessLogEntry(start time.Time, reqType, dn string, mods ...string) *ldap.Entry {
	attributes := map[string][]string{
		"reqStart": {start.UTC().Format(accessLogTimeLayout)}, "reqType": {reqType}, "reqDN": {dn},
		"reqEntryUUID": {"uuid-" + dn},
	}
	if len(mods) > 0 {
		attributes["reqMod"] = mods
	}
	return ldap.NewEntry("reqStart="+attributes["reqStart"][0]+",cn=accesslog", attributes)
}

func (f *fakeAccessLog) Search(request *ldap.SearchRequest) (*ldap.SearchResult, error) {
	f.searches = append(f.searches, request.Filter)
	if request.BaseDN != "cn=accesslog" || request.Scope != ldap.ScopeSingleLevel {
		return nil, ldap.NewError(ldap.LDAPResultNoSuchObject, nil)
	}
	packet, err := ldap.CompileFilter(request.Filter)
	if err != nil {
		return nil, err
	}
	matches := []*ldap.Entry{}
	for _, entry := range f.entries {
		if matchAccessLogFilter(packet.Children, entry) {
			matches = append(matches, entry)
		}
	}
	if request.SizeLimit > 0 && len(matches) > request.SizeLimit {
		return &ldap.SearchResult{Entries: matches[:request.SizeLimit]}, ldap.NewError(ldap.LDAPResultSizeLimitExceeded, nil)
	}
	return &ldap.SearchResult{Entries: matches}, nil
}

// matchAccessLogFilter evaluates the AND of simple reqStart/objectClass/reqResult terms used by the change log.
func matchAccessLogFilter(terms []*ber.Packet, entry *ldap.Entry) bool {
	start := entry.GetAttributeValue("reqStart")
	for _, term := range terms {
		negate := term.Tag == ldap.FilterNot
		if negate {
			term = term.Children[0]
		}
		if len(term.Children) < 2 {
			continue
		}
		name := term.Children[0].Value.(string)
		value := term.Children[1].Value.(string)
		ok := true
		if name == "reqStart" {
			switch term.Tag {
			case ldap.FilterEqualityMatch:
				ok = start == value
			case ldap.FilterGreaterOrEqual:
				ok = start >= value
			case ldap.FilterLessOrEqual:
				ok = start <= value
			}
		}
		if ok == negate {
			return false
		}
	}
	return true
}

func testAccessLog(now time.Time, log *fakeAccessLog) accessLogChanges {
	return accessLogChanges{
		logBase: "cn=accesslog", retention: time.Hour, settle: 2 * time.Second,
		now: func() time.Time { return now },
		connect: func(context.Context) (ldapSearcher, func(), error) {
			return log, func() {}, nil
		},
	}
}

func TestLDAPChangesReturnOrderedMetadataWithoutValues(t *testing.T) {
	now := time.Date(2026, 10, 7, 15, 0, 0, 0, time.UTC)
	base := now.Add(-time.Minute)
	log := &fakeAccessLog{entries: []*ldap.Entry{
		accessLogEntry(base.Add(3*time.Second), "delete", "uid=gone,dc=x"),
		accessLogEntry(base.Add(time.Second), "modify", "uid=a,dc=x", "title:= Secret Title", "mail:+ a@x", "modifyTimestamp:= 2026", "title:- Old"),
		accessLogEntry(base.Add(2*time.Second), "modrdn", "uid=b,ou=Old,dc=x"),
		accessLogEntry(now.Add(-time.Second), "add", "uid=unsettled,dc=x", "cn:+ U"),
	}}
	log.entries[2].Attributes = append(log.entries[2].Attributes, &ldap.EntryAttribute{Name: "reqNewRDN", Values: []string{"uid=c"}},
		&ldap.EntryAttribute{Name: "reqNewSuperior", Values: []string{"ou=New,dc=x"}})
	response, err := testAccessLog(now, log).changes(context.Background(), base, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Changes) != 3 || response.More {
		t.Fatalf("response: %+v", response)
	}
	if response.Changes[0].Type != "modify" || strings.Join(response.Changes[0].Attributes, ",") != "mail,title" {
		t.Fatalf("modify metadata: %+v", response.Changes[0])
	}
	if response.Changes[1].NewDN != "uid=c,ou=New,dc=x" || response.Changes[2].Type != "delete" {
		t.Fatalf("ordering or rename: %+v", response.Changes)
	}
	encoded, _ := json.Marshal(response)
	if strings.Contains(string(encoded), "Secret") || strings.Contains(string(encoded), "a@x") || strings.Contains(string(encoded), "unsettled") {
		t.Fatalf("response leaked values or unsettled writes: %s", encoded)
	}
	if got, _ := decodeChangeCursor(response.Cursor); !got.Equal(now.Add(-2*time.Second - time.Microsecond)) {
		t.Fatalf("cursor should advance to the settled head, got %v", got)
	}
	next, err := testAccessLog(now, log).changes(context.Background(), now.Add(-2*time.Second-time.Microsecond), 10)
	if err != nil || len(next.Changes) != 0 {
		t.Fatalf("repeat poll should be empty: %+v %v", next, err)
	}
}

func TestLDAPChangesLimitResumesAfterLastReturnedChange(t *testing.T) {
	now := time.Date(2026, 10, 7, 15, 0, 0, 0, time.UTC)
	base := now.Add(-time.Minute)
	log := &fakeAccessLog{}
	for index := 1; index <= 5; index++ {
		log.entries = append(log.entries, accessLogEntry(base.Add(time.Duration(index)*time.Second), "modify", "uid=u,dc=x", "title:= v"))
	}
	source := testAccessLog(now, log)
	first, err := source.changes(context.Background(), base, 2)
	if err != nil || len(first.Changes) != 2 || !first.More {
		t.Fatalf("first page: %+v %v", first, err)
	}
	cursor, _ := decodeChangeCursor(first.Cursor)
	second, err := source.changes(context.Background(), cursor, 10)
	if err != nil || len(second.Changes) != 3 || second.More || !second.Changes[0].Time.Equal(base.Add(3*time.Second)) {
		t.Fatalf("second page: %+v %v", second, err)
	}
}

func TestLDAPChangesNarrowWindowWhenServerSizeLimitIsHit(t *testing.T) {
	now := time.Date(2026, 10, 7, 15, 0, 0, 0, time.UTC)
	base := now.Add(-time.Hour / 2)
	log := &fakeAccessLog{}
	for index := 0; index < changeWindowSizeLimit+20; index++ {
		log.entries = append(log.entries, accessLogEntry(base.Add(time.Duration(index+1)*time.Second), "modify", "uid=u,dc=x", "title:= v"))
	}
	source := testAccessLog(now, log)
	cursor := base
	seen := 0
	for pages := 0; pages < 20; pages++ {
		response, err := source.changes(context.Background(), cursor, maxChangeLimit)
		if err != nil {
			t.Fatal(err)
		}
		for index, change := range response.Changes {
			if !change.Time.After(cursor) || (index > 0 && !change.Time.After(response.Changes[index-1].Time)) {
				t.Fatalf("changes out of order at page %d", pages)
			}
		}
		seen += len(response.Changes)
		cursor, _ = decodeChangeCursor(response.Cursor)
		if !response.More {
			break
		}
	}
	if seen != len(log.entries) {
		t.Fatalf("saw %d of %d changes", seen, len(log.entries))
	}
}

func TestLDAPChangesExpireOnlyUnanchoredOldCursors(t *testing.T) {
	now := time.Date(2026, 10, 7, 15, 0, 0, 0, time.UTC)
	old := now.Add(-2 * time.Hour)
	log := &fakeAccessLog{}
	if _, err := testAccessLog(now, log).changes(context.Background(), old, 10); err != errChangeCursorExpired {
		t.Fatalf("expected expiry, got %v", err)
	}
	log.entries = append(log.entries, accessLogEntry(old, "modify", "uid=u,dc=x", "title:= v"))
	if _, err := testAccessLog(now, log).changes(context.Background(), old, 10); err != nil {
		t.Fatalf("retained anchor should remain valid: %v", err)
	}
}

func TestChangesHandler(t *testing.T) {
	now := time.Date(2026, 10, 7, 15, 0, 0, 0, time.UTC)
	log := &fakeAccessLog{entries: []*ldap.Entry{accessLogEntry(now.Add(-time.Minute), "add", "uid=a,dc=x", "cn:+ A")}}
	server := handler(fileProvider{"unused"}, testAccessLog(now, log))
	request := func(target string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		return recorder
	}
	baseline := request("/v1/changes")
	var body changesResponse
	if baseline.Code != http.StatusOK || json.Unmarshal(baseline.Body.Bytes(), &body) != nil || len(body.Changes) != 0 || baseline.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("baseline: %d %s", baseline.Code, baseline.Body)
	}
	if head, _ := decodeChangeCursor(body.Cursor); !head.Equal(now.Add(-2*time.Second - time.Microsecond)) {
		t.Fatalf("baseline cursor %v", head)
	}
	poll := request("/v1/changes?limit=10&cursor=" + encodeChangeCursor(now.Add(-10*time.Minute)))
	if poll.Code != http.StatusOK || !strings.Contains(poll.Body.String(), `"type":"add"`) || strings.Contains(poll.Body.String(), "A\"") {
		t.Fatalf("poll: %d %s", poll.Code, poll.Body)
	}
	for target, status := range map[string]int{
		"/v1/changes?cursor=bogus":                                                      http.StatusBadRequest,
		"/v1/changes?limit=0&cursor=" + encodeChangeCursor(now):                         http.StatusBadRequest,
		"/v1/changes?limit=501&cursor=" + encodeChangeCursor(now):                       http.StatusBadRequest,
		"/v1/changes?since=1":                                                           http.StatusBadRequest,
		"/v1/changes?cursor=" + encodeChangeCursor(now) + "&since=2026-10-07T14:00:00Z": http.StatusBadRequest,
		"/v1/changes?cursor=" + encodeChangeCursor(now.Add(-3*time.Hour)):               http.StatusGone,
		"/v1/changes?since=2026-10-07T13:00:00Z":                                        http.StatusGone,
		"/v1/changes?since=2h":                                                          http.StatusGone,
		"/v1/changes?since=2026-10-07T16:00:00Z":                                        http.StatusBadRequest,
		"/v1/changes?since=1ns":                                                         http.StatusBadRequest,
	} {
		if got := request(target).Code; got != status {
			t.Fatalf("%s: got %d want %d", target, got, status)
		}
	}

	recorder := httptest.NewRecorder()
	handler(fileProvider{"unused"}, nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/changes", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("changes must be disabled without a change source: %d", recorder.Code)
	}
}

func TestChangesHandlerSupportsBoundedDurationLookback(t *testing.T) {
	now := time.Date(2026, 10, 7, 15, 0, 0, 0, time.UTC)
	log := &fakeAccessLog{entries: []*ldap.Entry{accessLogEntry(now.Add(-time.Minute), "add", "uid=a,dc=x", "cn:+ A")}}
	source := testAccessLog(now, log)
	source.retention = 48 * time.Hour
	recorder := httptest.NewRecorder()
	changesHandler(source).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/changes?since=24h", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"type":"add"`) {
		t.Fatalf("24-hour lookback: %d %s", recorder.Code, recorder.Body)
	}
}

func TestLDAPChangesSupportInclusiveSinceBootstrap(t *testing.T) {
	now := time.Date(2026, 10, 7, 15, 0, 0, 0, time.UTC)
	since := now.Add(-time.Minute).Add(250 * time.Millisecond)
	log := &fakeAccessLog{entries: []*ldap.Entry{
		accessLogEntry(since.Truncate(time.Second).Add(time.Microsecond), "modify", "uid=boundary,dc=x", "title:= v"),
		accessLogEntry(since.Add(time.Second), "modify", "uid=later,dc=x", "mail:= v"),
	}}
	handler := changesHandler(testAccessLog(now, log))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/changes?since="+url.QueryEscape(since.Format(time.RFC3339Nano))+"&limit=1", nil)
	handler.ServeHTTP(recorder, request)
	var response changesResponse
	if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &response) != nil {
		t.Fatalf("since response: %d %s", recorder.Code, recorder.Body)
	}
	if len(response.Changes) != 1 || response.Changes[0].DN != "uid=boundary,dc=x" || !response.More {
		t.Fatalf("first page omitted inclusive boundary change: %+v", response)
	}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/changes?cursor="+url.QueryEscape(response.Cursor)+"&limit=10", nil)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "uid=later,dc=x") {
		t.Fatalf("cursor continuation: %d %s", recorder.Code, recorder.Body)
	}
}

func TestLDAPChangesHeadDoesNotSkipSequenceStampsLaterInTheSameSecond(t *testing.T) {
	second := time.Date(2026, 10, 7, 14, 59, 58, 0, time.UTC)
	log := &fakeAccessLog{}
	first := testAccessLog(second.Add(2*time.Second+700*time.Millisecond), log)
	cursor := first.head()
	if !cursor.Before(second) {
		t.Fatalf("head %v must end before the in-progress second", cursor)
	}
	// slapd stamps a write late in the second with a small sequence fraction.
	log.entries = append(log.entries, accessLogEntry(second.Add(time.Microsecond), "modify", "uid=u,dc=x", "title:= v"))
	response, err := testAccessLog(second.Add(3*time.Second+500*time.Millisecond), log).changes(context.Background(), cursor, 10)
	if err != nil || len(response.Changes) != 1 {
		t.Fatalf("same-second write was skipped: %+v %v", response, err)
	}
}
