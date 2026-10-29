package audit

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidEvent      = errors.New("invalid audit event")
	ErrInvalidDelegation = errors.New("invalid audit delegation")
	ErrInvalidQuery      = errors.New("invalid audit query")
	ErrInvalidCursor     = errors.New("invalid audit cursor")
	ErrReplay            = errors.New("audit delegation replayed")
)

const MaxPageSize = 100

// Event is the shared, minimal record format for attributable audit activity.
// It deliberately excludes arbitrary payloads that could contain sensitive data.
type Event struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	OccurredAt     time.Time `json:"occurred_at"`
	ActorIssuer    string    `json:"actor_issuer"`
	ActorSubject   string    `json:"actor_subject"`
	Action         string    `json:"action"`
	Target         string    `json:"target"`
	Outcome        string    `json:"outcome"`
	Source         string    `json:"source"`
	CorrelationID  string    `json:"correlation_id"`
}

// Query scopes every read to one organization and advances with an opaque,
// stable cursor. Time bounds are inclusive at From and exclusive at Until.
type Query struct {
	OrganizationID string
	From           time.Time
	Until          time.Time
	Source         string
	Action         string
	Outcome        string
	Cursor         string
	Limit          int
}

type Page struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

// Repository is the reusable audit event persistence boundary.
type Repository interface {
	Append(context.Context, Event) (Event, error)
	List(context.Context, Query) (Page, error)
}

func ValidateEvent(event Event) error {
	if !validText(event.OrganizationID, 128) ||
		!validText(event.ActorIssuer, 512) ||
		!validText(event.ActorSubject, 256) ||
		!validText(event.Action, 128) ||
		!validText(event.Target, 512) ||
		!validText(event.Outcome, 64) ||
		!validText(event.Source, 128) ||
		!validText(event.CorrelationID, 128) ||
		event.OccurredAt.IsZero() ||
		(event.ID != "" && !validText(event.ID, 128)) {
		return ErrInvalidEvent
	}
	return nil
}

func ValidateQuery(query Query) error {
	if !validText(query.OrganizationID, 128) ||
		(query.Limit < 1 || query.Limit > MaxPageSize) ||
		(!query.From.IsZero() && !query.Until.IsZero() && !query.From.Before(query.Until)) ||
		!optionalText(query.Source, 128) ||
		!optionalText(query.Action, 128) ||
		!optionalText(query.Outcome, 64) {
		return ErrInvalidQuery
	}
	return nil
}

func validText(value string, max int) bool {
	if value == "" || len(value) > max || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 32 || character == 127 {
			return false
		}
	}
	return true
}

func optionalText(value string, max int) bool {
	return value == "" || validText(value, max)
}
