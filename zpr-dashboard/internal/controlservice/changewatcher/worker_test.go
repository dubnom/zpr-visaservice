package changewatcher

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-ldap/ldap/v3"
)

const testUUID = "12345678-1234-1234-1234-123456789012"

func event(kind string) Event { return Event{Version: 1, BaseDN: "dc=test", Type: kind} }
func entryEvent(kind string) Event {
	e := event(kind)
	e.UUID = testUUID
	e.DN = "uid=alice,dc=test"
	return e
}

func TestWorkerRefreshDuplicateDeleteAndHealth(t *testing.T) {
	count := 0
	worker, err := NewWorker("dc=test", func(context.Context) error { count++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []Event{event("refresh_start"), entryEvent("present"), entryEvent("add"), event("refresh_complete"), entryEvent("modify"), entryEvent("modify"), {Version: 1, Type: "delete", UUID: testUUID}} {
		if err := worker.accept(context.Background(), e); err != nil {
			t.Fatalf("%s: %v", e.Type, err)
		}
	}
	if count != 6 || worker.Status().State != "healthy" || worker.Status().LastRefresh.IsZero() {
		t.Fatalf("refresh count=%d state=%+v", count, worker.Status())
	}
	for _, kind := range []string{"disconnected", "resume_start", "refresh_complete", "resync_required", "disconnected", "refresh_start", "refresh_complete"} {
		if err := worker.accept(context.Background(), event(kind)); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
}

func TestWorkerRejectsMalformedAndOutOfOrderEvents(t *testing.T) {
	invalid := []Event{entryEvent("modify"), event("refresh_complete"), event("unknown"), {Version: 2, Type: "refresh_start"}}
	for _, e := range invalid {
		worker, _ := NewWorker("dc=test", func(context.Context) error { t.Fatal("invalid event refreshed source"); return nil })
		if err := worker.accept(context.Background(), e); err == nil {
			t.Fatalf("accepted %+v", e)
		}
	}
	for _, mutate := range []func(*Event){
		func(e *Event) { e.UUID = "malformed" }, func(e *Event) { e.UUID = "00000000-0000-0000-0000-000000000000" }, func(e *Event) { e.DN = "uid=alice,dc=other" }, func(e *Event) { e.DN = "broken" }, func(e *Event) { e.BaseDN = "dc=other" },
	} {
		worker, _ := NewWorker("dc=test", func(context.Context) error { t.Fatal("invalid event refreshed source"); return nil })
		_ = worker.accept(context.Background(), event("refresh_start"))
		e := entryEvent("modify")
		mutate(&e)
		if err := worker.accept(context.Background(), e); err == nil {
			t.Fatalf("accepted %+v", e)
		}
	}
	worker, _ := NewWorker("dc=test", func(context.Context) error { return nil })
	_ = worker.accept(context.Background(), event("refresh_start"))
	_ = worker.accept(context.Background(), event("disconnected"))
	if worker.accept(context.Background(), entryEvent("modify")) == nil {
		t.Fatal("accepted event after disconnect")
	}
}

func TestWorkerRestartResumeAndFailurePreservesCheckpoint(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	store, _, err := OpenStore(dir, "org", "ldap", "binding")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Save([]byte("safe")); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, initial, err := OpenStore(dir, "org", "ldap", "binding")
	if err != nil {
		t.Fatal(err)
	}
	worker, _ := NewWorker("dc=test", func(context.Context) error { return errors.New("private upstream error") })
	worker.Run(context.Background(), store, initial, func(ctx context.Context, cookie []byte, sink Sink, save func([]byte) error) error {
		if string(cookie) != "safe" {
			t.Fatal("restart cookie not passed to stream")
		}
		if err := sink(event("resume_start")); err != nil {
			return err
		}
		if err := sink(entryEvent("modify")); err != nil {
			return err
		}
		return save([]byte("unsafe"))
	})
	if worker.Status().State != "failed" {
		t.Fatalf("failure not visible: %+v", worker.Status())
	}
	store.Close()
	store, cookie, err := OpenStore(dir, "org", "ldap", "binding")
	if err != nil || string(cookie) != "safe" {
		t.Fatalf("failed refresh advanced checkpoint: %q %v", cookie, err)
	}
	store.Close()
}

func TestWorkerConcurrentStatusDuringRefresh(t *testing.T) {
	var worker *Worker
	var wg sync.WaitGroup
	worker, _ = NewWorker("dc=test", func(context.Context) error { _ = worker.Status(); return nil })
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = worker.Status()
		}
	}()
	_ = worker.accept(context.Background(), event("refresh_start"))
	for i := 0; i < 100; i++ {
		if err := worker.accept(context.Background(), entryEvent("modify")); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}

type syncResponse struct{ controls []ldap.Control }

func (r syncResponse) Entry() *ldap.Entry       { return nil }
func (r syncResponse) Controls() []ldap.Control { return r.controls }
func (r syncResponse) Referral() string         { return "" }
func (r syncResponse) Err() error               { return nil }
func (r syncResponse) Next() bool               { return false }

func TestMalformedSyncControlsFailClosed(t *testing.T) {
	for _, control := range []ldap.Control{(*ldap.ControlSyncState)(nil), (*ldap.ControlSyncDone)(nil), (*ldap.ControlSyncInfo)(nil), &ldap.ControlSyncInfo{Value: ldap.SyncInfoNewcookie}, &ldap.ControlSyncInfo{Value: ldap.SyncInfoSyncIdSet}, &ldap.ControlSyncInfo{Value: ldap.SyncInfoRefreshPresent}, &ldap.ControlSyncInfo{Value: ldap.SyncInfoRefreshDelete}} {
		if _, err := ConsumeLDAPSyncResponse(syncResponse{[]ldap.Control{control}}, func(Event) error { t.Fatal("malformed control emitted an event"); return nil }); err == nil {
			t.Fatalf("accepted %T", control)
		}
	}
}

type syncStream struct {
	syncResponse
	remaining int
}

func (r *syncStream) Next() bool {
	if r.remaining == 0 {
		return false
	}
	r.remaining--
	return true
}

func TestConsumerResumesDurableCookieOnFirstDial(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	store, _, err := OpenStore(dir, "org", "ldap", "binding")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save([]byte("restart-cookie")); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, cookie, err := OpenStore(dir, "org", "ldap", "binding")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = ConsumeLDAPChanges(ctx, func(initial []byte) (ldap.Response, func(), error) {
		if string(initial) != "restart-cookie" {
			t.Errorf("first LDAP dial got %q", initial)
		}
		cancel()
		return syncResponse{}, func() {}, nil
	}, func(e Event) error {
		if e.Type != "resume_start" {
			t.Errorf("expected resume, got %s", e.Type)
		}
		return nil
	}, cookie, store.Save)
	if err != nil {
		t.Fatal(err)
	}
}

func TestConsumerCheckpointFailureStopsBeforeReconnect(t *testing.T) {
	expected := errors.New("checkpoint failed")
	calls := 0
	err := ConsumeLDAPChanges(context.Background(), func([]byte) (ldap.Response, func(), error) {
		calls++
		return &syncStream{syncResponse{[]ldap.Control{&ldap.ControlSyncInfo{Value: ldap.SyncInfoNewcookie, NewCookie: &ldap.ControlSyncInfoNewCookie{Cookie: []byte("next")}}}}, 1}, func() {}, nil
	}, func(Event) error { return nil }, []byte("safe"), func([]byte) error { return expected })
	if !errors.Is(err, expected) || calls != 1 {
		t.Fatalf("checkpoint failure: calls=%d err=%v", calls, err)
	}
}

func TestConsumerRejectedCookieDurablyResets(t *testing.T) {
	store, _, err := OpenStore(filepath.Join(t.TempDir(), "state"), "org", "ldap", "binding")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Save([]byte("rejected")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reset := false
	err = ConsumeLDAPChanges(ctx, func([]byte) (ldap.Response, func(), error) {
		return nil, nil, ldap.NewError(4096, errors.New("cookie rejected"))
	}, func(e Event) error {
		if e.Type == "resync_required" && !reset {
			t.Error("resync reported before durable reset")
		}
		if e.Type == "disconnected" {
			cancel()
		}
		return nil
	}, []byte("rejected"), func(cookie []byte) error {
		if len(cookie) != 0 {
			t.Error("rejected cookie retained")
		}
		reset = true
		return store.Save(cookie)
	})
	if err != nil || !reset {
		t.Fatalf("reset=%v err=%v", reset, err)
	}
}

func TestWorkerRejectedAcceptanceRestartRetryAndDuplicate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	store, _, err := OpenStore(dir, "org", "ldap", "binding")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save([]byte("before")); err != nil {
		t.Fatal(err)
	}
	store.Close()
	for _, accepted := range []bool{false, true} {
		store, cookie, err := OpenStore(dir, "org", "ldap", "binding")
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		worker, _ := NewWorker("dc=test", func(context.Context) error {
			calls++
			if !accepted {
				return errors.New("HTTP 503: enqueue rejected")
			}
			return nil
		})
		worker.Run(context.Background(), store, cookie, func(ctx context.Context, initial []byte, sink Sink, save func([]byte) error) error {
			if string(initial) != "before" {
				t.Fatalf("unsafe restart checkpoint: %q", initial)
			}
			for _, e := range []Event{event("resume_start"), entryEvent("modify"), entryEvent("modify")} {
				if err := sink(e); err != nil {
					return err
				}
			}
			return save([]byte("after"))
		})
		status := worker.Status()
		if accepted {
			if calls != 2 || status.Reconciliation != "accepted_pending" {
				t.Fatalf("duplicate acceptance: %d %+v", calls, status)
			}
		} else if calls != 1 || status.State != "failed" || status.Reconciliation != "failed" {
			t.Fatalf("rejection not visible: %d %+v", calls, status)
		}
		store.Close()
	}
	store, cookie, err := OpenStore(dir, "org", "ldap", "binding")
	if err != nil || string(cookie) != "after" {
		t.Fatalf("successful retry not checkpointed: %q %v", cookie, err)
	}
	store.Close()
}
