package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"neboagency.com/zpr-dashborad/internal/audit"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

var ErrMissingDependency = errors.New("audit HTTP handler requires a repository and trusted scope resolver")

type AuditReadVerifier interface {
	VerifyAuditRead(*http.Request, []byte, time.Time) (operatorauth.Grant, error)
}

type RequestScope struct {
	OrganizationID string
	ActorIssuer    string
	ActorSubject   string
}

type ScopeResolver func(*http.Request) (RequestScope, error)

type Handler struct {
	repository audit.Repository
	resolve    ScopeResolver
}

func NewHandler(repository audit.Repository, resolve ScopeResolver) (*Handler, error) {
	if repository == nil || resolve == nil {
		return nil, ErrMissingDependency
	}
	return &Handler{repository: repository, resolve: resolve}, nil
}

// NewDelegatedHandler requires a signed, organization- and permission-scoped
// operator assertion from Control Room before querying the repository.
func NewDelegatedHandler(repository audit.Repository, verifier AuditReadVerifier, now func() time.Time) (*Handler, error) {
	if repository == nil || verifier == nil {
		return nil, ErrMissingDependency
	}
	if now == nil {
		now = time.Now
	}
	return NewHandler(repository, func(r *http.Request) (RequestScope, error) {
		grant, err := verifier.VerifyAuditRead(r, nil, now())
		if err != nil {
			return RequestScope{}, err
		}
		organization := r.URL.Query().Get("organization_id")
		if organization == "" || !operatorauth.OrganizationAllowed(grant.Organizations, organization) ||
			!slices.Contains(grant.Permissions, "audit.read") {
			return RequestScope{}, errors.New("audit scope is not authorized")
		}
		return RequestScope{
			OrganizationID: organization,
			ActorIssuer:    grant.Issuer,
			ActorSubject:   grant.Subject,
		}, nil
	})
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.ContentLength > 0 {
		writeError(w, http.StatusBadRequest, "audit reads do not accept a request body")
		return
	}
	if r.Body != nil {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1))
		if err != nil || len(body) != 0 {
			writeError(w, http.StatusBadRequest, "audit reads do not accept a request body")
			return
		}
	}
	scope, err := handler.resolve(r)
	if err != nil || scope.OrganizationID == "" {
		writeError(w, http.StatusForbidden, "audit access is not authorized")
		return
	}
	query, err := parseQuery(r, scope.OrganizationID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid audit query")
		return
	}
	if scope.ActorIssuer != "" || scope.ActorSubject != "" {
		correlationID, err := newCorrelationID()
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "audit history is unavailable")
			return
		}
		if _, err := handler.repository.Append(r.Context(), audit.Event{
			OrganizationID: scope.OrganizationID,
			OccurredAt:     time.Now().UTC(),
			ActorIssuer:    scope.ActorIssuer,
			ActorSubject:   scope.ActorSubject,
			Action:         "audit.history.read",
			Target:         "/api/audit/events",
			Outcome:        "requested",
			Source:         "control-room",
			CorrelationID:  correlationID,
		}); err != nil {
			writeError(w, http.StatusServiceUnavailable, "audit history is unavailable")
			return
		}
	}
	page, err := handler.repository.List(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "audit history is unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(page); err != nil {
		return
	}
}

func newCorrelationID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func parseQuery(r *http.Request, organization string) (audit.Query, error) {
	values := r.URL.Query()
	allowed := map[string]bool{
		"organization_id": true, "from": true, "until": true, "source": true, "action": true,
		"outcome": true, "cursor": true, "limit": true,
	}
	for key, entries := range values {
		if !allowed[key] || len(entries) != 1 {
			return audit.Query{}, audit.ErrInvalidQuery
		}
	}
	query := audit.Query{OrganizationID: organization, Limit: 50}
	if supplied := values.Get("organization_id"); supplied != "" && supplied != organization {
		return audit.Query{}, audit.ErrInvalidQuery
	}
	var err error
	if query.From, err = parseTime(values.Get("from")); err != nil {
		return audit.Query{}, err
	}
	if query.Until, err = parseTime(values.Get("until")); err != nil {
		return audit.Query{}, err
	}
	query.Source = values.Get("source")
	query.Action = values.Get("action")
	query.Outcome = values.Get("outcome")
	query.Cursor = values.Get("cursor")
	if limit := values.Get("limit"); limit != "" {
		query.Limit, err = strconv.Atoi(limit)
		if err != nil {
			return audit.Query{}, audit.ErrInvalidQuery
		}
	}
	if err := audit.ValidateQuery(query); err != nil {
		return audit.Query{}, err
	}
	return query, nil
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, audit.ErrInvalidQuery
	}
	return parsed.UTC(), nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{Error: message})
}
