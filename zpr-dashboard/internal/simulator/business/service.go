// Package business implements deliberately small, stateful simulation workloads.
// Network authorization belongs to ZPR, not caller-supplied HTTP identity headers.
package business

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Spec struct {
	Workload string
	Service  string
	DNS      string
	Port     int
	Machine  string
	Initial  string
	Final    string
}

func Specs() []Spec {
	return []Spec{
		{"support-desk-app", "CustomerSupportDesk", "customer-support.svc.zpr", 8443, "machine-13", "open", "resolved"},
		{"engineering-build-farm", "EngineeringBuildFarm", "engineering-build.svc.zpr", 8444, "machine-13", "queued", "built"},
		{"finance-workspace", "FinanceWorkspace", "finance-workspace.svc.zpr", 8445, "machine-13", "draft", "approved"},
		{"shipping-portal", "ShippingPortal", "shipping-portal.svc.zpr", 8446, "machine-13", "queued", "dispatched"},
		{"receiving-portal", "ReceivingPortal", "receiving-portal.svc.zpr", 8447, "machine-13", "expected", "received"},
		{"assembly-mes", "AssemblyExecution", "assembly-mes.svc.zpr", 8448, "machine-13", "planned", "assembled"},
		{"quality-test-bench", "QualityTestBench", "quality-test.svc.zpr", 8449, "machine-13", "pending", "passed"},
	}
}

func Lookup(workload string) (Spec, bool) {
	for _, spec := range Specs() {
		if spec.Workload == workload {
			return spec, true
		}
	}
	return Spec{}, false
}

type Record struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	State   string `json:"state"`
}

type diskState struct {
	Version int      `json:"version"`
	Service string   `json:"service"`
	Records []Record `json:"records"`
}

type Event struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Remote string `json:"remote"`
	Status int    `json:"status"`
	Record string `json:"record,omitempty"`
}

type service struct {
	mu    sync.Mutex
	spec  Spec
	path  string
	state diskState
	log   func(Event) error
}

const maxRecords = 1000

func validID(id string) bool {
	if len(id) == 0 || len(id) > 80 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

func New(workload, path string, log func(Event) error) (http.Handler, error) {
	spec, ok := Lookup(workload)
	if !ok || path == "" || log == nil {
		return nil, errors.New("business simulation requires a known service, state path and event logger")
	}
	s := &service{spec: spec, path: path, state: diskState{Version: 1, Service: workload, Records: []Record{}}, log: log}
	file, err := os.Open(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("open business state: %w", err)
	}
	if err == nil {
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return nil, fmt.Errorf("stat business state: %w", err)
		}
		if info.Size() > 1<<20 {
			return nil, errors.New("business state exceeds the 1 MiB limit")
		}
		decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&s.state); err != nil {
			return nil, fmt.Errorf("decode business state: %w", err)
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			return nil, errors.New("business state contains trailing data")
		}
		if s.state.Version != 1 || s.state.Service != workload || len(s.state.Records) > maxRecords {
			return nil, errors.New("invalid business state identity, version or capacity")
		}
		seen := make(map[string]bool)
		for _, record := range s.state.Records {
			if !validID(record.ID) || seen[record.ID] || strings.TrimSpace(record.Summary) == "" ||
				len(record.Summary) > 256 || (record.State != spec.Initial && record.State != spec.Final) {
				return nil, errors.New("invalid persisted business record")
			}
			seen[record.ID] = true
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create business state directory: %w", err)
	}
	return s, nil
}

func (s *service) save(next diskState) error {
	file, err := os.CreateTemp(filepath.Dir(s.path), ".business-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := json.NewEncoder(file).Encode(next); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), s.path)
}

func (s *service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event := Event{Method: r.Method, Path: r.URL.Path, Remote: r.RemoteAddr}
	reply := func(status int, value any) {
		event.Status = status
		if err := s.log(event); err != nil {
			http.Error(w, "simulation event log unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(value)
	}
	fail := func(status int, message string) { reply(status, map[string]string{"error": message}) }
	if r.URL.Path == "/health" && r.Method == http.MethodGet {
		reply(http.StatusOK, map[string]any{"service": s.spec.Workload, "status": "ok", "simulation": true})
		return
	}
	if r.URL.Path == "/records" && r.Method == http.MethodGet {
		records := append([]Record{}, s.state.Records...)
		sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
		reply(http.StatusOK, records)
		return
	}
	if r.URL.Path == "/records" && r.Method == http.MethodPost {
		var input struct {
			ID      string `json:"id"`
			Summary string `json:"summary"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		decoder.DisallowUnknownFields()
		var trailing any
		if err := decoder.Decode(&input); err != nil || decoder.Decode(&trailing) != io.EOF ||
			!validID(input.ID) || strings.TrimSpace(input.Summary) == "" || len(input.Summary) > 256 {
			fail(http.StatusBadRequest, "expected a bounded record id and summary")
			return
		}
		for _, record := range s.state.Records {
			if record.ID == input.ID {
				fail(http.StatusConflict, "record already exists")
				return
			}
		}
		if len(s.state.Records) >= maxRecords {
			fail(http.StatusInsufficientStorage, "simulation record capacity reached")
			return
		}
		event.Record = input.ID
		// Audit the attempt before mutation; a failed audit must not create state.
		if err := s.log(event); err != nil {
			http.Error(w, "simulation event log unavailable", http.StatusInternalServerError)
			return
		}
		record := Record{ID: input.ID, Summary: input.Summary, State: s.spec.Initial}
		next := s.state
		next.Records = append(append([]Record{}, s.state.Records...), record)
		if err := s.save(next); err != nil {
			fail(http.StatusInternalServerError, "unable to persist simulation record")
			return
		}
		s.state = next
		reply(http.StatusCreated, record)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "records" && validID(parts[1]) {
		for index, record := range s.state.Records {
			if record.ID != parts[1] {
				continue
			}
			event.Record = record.ID
			if len(parts) == 2 && r.Method == http.MethodGet {
				reply(http.StatusOK, record)
				return
			}
			if len(parts) == 3 && parts[2] == "advance" && r.Method == http.MethodPost {
				if record.State != s.spec.Initial {
					fail(http.StatusConflict, "record is already complete")
					return
				}
				if err := s.log(event); err != nil {
					http.Error(w, "simulation event log unavailable", http.StatusInternalServerError)
					return
				}
				record.State = s.spec.Final
				next := s.state
				next.Records = append([]Record{}, s.state.Records...)
				next.Records[index] = record
				if err := s.save(next); err != nil {
					fail(http.StatusInternalServerError, "unable to persist simulation transition")
					return
				}
				s.state = next
				reply(http.StatusOK, record)
				return
			}
			fail(http.StatusMethodNotAllowed, "unsupported simulation operation")
			return
		}
	}
	fail(http.StatusNotFound, "simulation record or route not found")
}
