package business

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func discardEvent(Event) error { return nil }

func TestBusinessWorkflowPersistsForEveryService(t *testing.T) {
	for _, spec := range Specs() {
		t.Run(spec.Workload, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "records.json")
			var events []Event
			handler, err := New(spec.Workload, path, func(event Event) error { events = append(events, event); return nil })
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(handler)
			record, err := Exercise(context.Background(), server.Client(), server.URL, spec.Workload)
			server.Close()
			if err != nil || record.State != spec.Final {
				t.Fatalf("workflow: %+v, %v", record, err)
			}
			if len(events) != 6 || events[0].Method != "POST" || events[5].Status != http.StatusOK {
				t.Fatalf("workflow events: %+v", events)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("persistent state mode: %v, %v", info, err)
			}
			restarted, err := New(spec.Workload, path, discardEvent)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			restarted.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/records/"+record.ID, nil))
			var stored Record
			if err := json.Unmarshal(response.Body.Bytes(), &stored); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || stored != record {
				t.Fatalf("restart lost state: %+v, HTTP %d", stored, response.Code)
			}
			response = httptest.NewRecorder()
			restarted.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/records/"+record.ID+"/advance", nil))
			if response.Code != http.StatusConflict {
				t.Fatalf("repeated transition accepted: %d", response.Code)
			}
		})
	}
}

func TestBusinessValidationFailuresDoNotMutateState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.json")
	handler, err := New(Specs()[0].Workload, path, discardEvent)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{`, `{}`, `{"id":"../x","summary":"invalid"}`, `{"id":"valid","summary":""}`,
		`{"id":"valid","summary":"ok","state":"resolved"}`,
		`{"id":"valid","summary":"ok"} {}`,
		`{"id":"valid","summary":"` + strings.Repeat("x", 257) + `"}`,
		strings.Repeat("x", 2049),
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/records", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid input returned %d", response.Code)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("invalid request created state")
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/records", strings.NewReader(`{"id":"case-1","summary":"test"}`)))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Body.String())
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/records", strings.NewReader(`{"id":"case-1","summary":"duplicate"}`)),
		httptest.NewRequest(http.MethodDelete, "/records/case-1", nil),
		httptest.NewRequest(http.MethodGet, "/records/unknown", nil),
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code < 400 {
			t.Fatalf("invalid operation returned %d", response.Code)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("rejected operation mutated persistent state")
		}
	}
}

func TestBusinessCorruptionAndIOErrorsAreExplicit(t *testing.T) {
	for _, content := range []string{
		`{`, `{"version":2,"service":"support-desk-app","records":[]}`,
		`{"version":1,"service":"finance-workspace","records":[]}`,
		`{"version":1,"service":"support-desk-app","records":[]} {}`,
		`{"version":1,"service":"support-desk-app","records":[{"id":"bad","summary":"x","state":"invented"}]}`,
		`{"version":1,"service":"support-desk-app","records":[{"id":"same","summary":"x","state":"open"},{"id":"same","summary":"x","state":"open"}]}`,
	} {
		path := filepath.Join(t.TempDir(), "records.json")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := New("support-desk-app", path, discardEvent); err == nil {
			t.Fatalf("corrupt state accepted: %s", content)
		}
	}
	for _, logFailure := range []bool{false, true} {
		directory := t.TempDir()
		path := filepath.Join(directory, "records.json")
		log := discardEvent
		if logFailure {
			log = func(Event) error { return errors.New("test audit failure") }
		}
		handler, err := New("support-desk-app", path, log)
		if err != nil {
			t.Fatal(err)
		}
		if !logFailure {
			if err := os.Remove(directory); err != nil {
				t.Fatal(err)
			}
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/records", strings.NewReader(`{"id":"case-1","summary":"test"}`)))
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("IO failure returned %d", response.Code)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("failed persistence or audit created state")
		}
		if len(handler.(*service).state.Records) != 0 {
			t.Fatal("failed persistence or audit changed in-memory state")
		}
	}
}

func TestBusinessConcurrentRequestsAndCapacity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.json")
	handler, err := New("support-desk-app", path, discardEvent)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if _, err := Exercise(context.Background(), server.Client(), server.URL, "support-desk-app"); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	s := handler.(*service)
	if len(s.state.Records) != 12 {
		t.Fatalf("records=%d; want 12", len(s.state.Records))
	}
	for len(s.state.Records) < maxRecords-1 {
		s.state.Records = append(s.state.Records, Record{ID: fmt.Sprintf("seed-%d", len(s.state.Records)), Summary: "seed", State: "open"})
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/records", strings.NewReader(`{"id":"at-limit","summary":"test"}`)))
	if response.Code != http.StatusCreated || len(s.state.Records) != maxRecords {
		t.Fatal("the thousandth record must be accepted")
	}
	if _, err := New("support-desk-app", path, discardEvent); err != nil {
		t.Fatalf("full persisted state rejected: %v", err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/records", strings.NewReader(`{"id":"overflow","summary":"test"}`)))
	if response.Code != http.StatusInsufficientStorage || len(s.state.Records) != maxRecords {
		t.Fatal("capacity must fail explicitly without evicting records")
	}
}

func TestBusinessFinalAuditFailureReportsCommittedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.json")
	handler, err := New("support-desk-app", path, func(event Event) error {
		if event.Status == http.StatusCreated {
			return errors.New("test completion audit unavailable")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/records", strings.NewReader(`{"id":"committed","summary":"test"}`)))
	if response.Code != http.StatusInternalServerError {
		t.Fatal("post-write audit failure must not return success")
	}
	restarted, err := New("support-desk-app", path, discardEvent)
	if err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	restarted.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/records/committed", nil))
	if response.Code != http.StatusOK {
		t.Fatal("record committed before final audit failure must remain queryable")
	}
}

func TestBusinessExerciseRejectsIncorrectResponses(t *testing.T) {
	for _, body := range []string{`{}`, `{"id":"wrong","state":"resolved"}`, `{`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(body))
		}))
		_, err := Exercise(context.Background(), server.Client(), server.URL, "support-desk-app")
		server.Close()
		if err == nil {
			t.Fatal("invalid business response accepted")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := Exercise(ctx, http.DefaultClient, "http://127.0.0.1:1", "support-desk-app"); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled workflow did not preserve cancellation: %v", err)
		}
	}
}
