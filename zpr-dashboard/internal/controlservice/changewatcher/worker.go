package changewatcher

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"
)

type Status struct {
	State       string    `json:"state"`
	Error       string    `json:"error,omitempty"`
	Events      uint64    `json:"events"`
	LastRefresh time.Time `json:"last_refresh,omitempty"`
}

type Stream func(context.Context, []byte, Sink, func([]byte) error) error

// Worker accepts only the ordered RFC 4533 lifecycle. Cookies have no sortable
// sequence number: replayed entry events must be handled idempotently by refresh.
type Worker struct {
	mu      sync.Mutex
	status  Status
	phase   string
	base    *ldap.DN
	refresh func(context.Context) error
}

func NewWorker(baseDN string, refresh func(context.Context) error) (*Worker, error) {
	base, err := ldap.ParseDN(baseDN)
	if err != nil || len(base.RDNs) == 0 || refresh == nil {
		return nil, errors.New("invalid trusted watcher scope")
	}
	return &Worker{base: base, refresh: refresh, status: Status{State: "starting"}}, nil
}

func (worker *Worker) Status() Status {
	worker.mu.Lock()
	defer worker.mu.Unlock()
	return worker.status
}

func (worker *Worker) Run(ctx context.Context, store *Store, initial []byte, stream Stream) {
	err := stream(ctx, initial, func(event Event) error { return worker.accept(ctx, event) }, store.Save)
	worker.mu.Lock()
	defer worker.mu.Unlock()
	if err != nil {
		worker.status.State, worker.status.Error = "failed", "watcher stopped; checkpoint was not advanced past failed processing"
	} else {
		worker.status.State = "stopped"
	}
}

func (worker *Worker) accept(ctx context.Context, event Event) error {
	worker.mu.Lock()
	defer worker.mu.Unlock()
	invalid := func() error { return errors.New("invalid trusted watcher event or lifecycle") }
	if event.Version != 1 || len(event.DN) > 4096 || len(event.UUID) > 64 {
		return invalid()
	}
	if event.BaseDN != "" {
		base, err := ldap.ParseDN(event.BaseDN)
		if err != nil || !worker.base.EqualFold(base) {
			return invalid()
		}
	}
	refresh := false
	switch event.Type {
	case "refresh_start", "resume_start":
		if worker.phase != "" && worker.phase != "disconnected" && worker.phase != "resync_required" {
			return invalid()
		}
		worker.phase = event.Type
		worker.status.State = "refreshing"
		if event.Type == "resume_start" {
			worker.status.State = "resuming"
		}
	case "present", "add", "modify", "delete":
		if worker.phase != "refresh_start" && worker.phase != "resume_start" && worker.phase != "live" {
			return invalid()
		}
		if event.Type == "present" && worker.phase == "live" {
			return invalid()
		}
		if id, err := uuid.Parse(event.UUID); err != nil || id == uuid.Nil {
			return invalid()
		}
		if event.DN != "" {
			dn, err := ldap.ParseDN(event.DN)
			if err != nil || !worker.base.EqualFold(dn) && !worker.base.AncestorOfFold(dn) {
				return invalid()
			}
		} else if event.Type != "present" && event.Type != "delete" {
			return invalid()
		}
		refresh = true
	case "refresh_complete":
		if worker.phase != "refresh_start" && worker.phase != "resume_start" {
			return invalid()
		}
		refresh = true
	case "resync_required":
		if worker.phase == "" || worker.phase == "resync_required" {
			return invalid()
		}
		worker.phase, worker.status.State = "resync_required", "resync_required"
	case "disconnected":
		if worker.phase == "" || worker.phase == "disconnected" {
			return invalid()
		}
		worker.phase, worker.status.State = "disconnected", "disconnected"
	default:
		return invalid()
	}
	if refresh {
		// Do not acknowledge even the opaque cookie until refresh and invalidation
		// both succeed. Duplicate/replayed events intentionally repeat the refresh.
		worker.mu.Unlock()
		err := worker.refresh(ctx)
		worker.mu.Lock()
		if err != nil {
			return errors.New("trusted source refresh or visa invalidation failed")
		}
		worker.status.LastRefresh = time.Now().UTC()
		if event.Type == "refresh_complete" {
			worker.phase = "live"
		}
		if worker.phase == "live" {
			worker.status.State = "healthy"
		}
	}
	worker.status.Events++
	return nil
}
