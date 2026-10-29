package enrollment

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	auditcontract "neboagency.com/zpr-dashborad/internal/audit"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

type operatorPrincipalKey struct{}

func (s *Store) ConsumeOperatorDelegation(ctx context.Context, hash string, expires, now time.Time) error {
	if len(hash) != 64 || now.IsZero() || !expires.After(now) || expires.Sub(now) > 32*time.Second {
		return ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM operator_delegation_replay WHERE expires_at<=?`, now.UnixNano()); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operator_delegation_replay`).Scan(&count); err != nil {
		return err
	}
	if count >= 8192 {
		return ErrUnavailable
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO operator_delegation_replay(token_hash,expires_at) VALUES(?,?)`, hash, expires.UnixNano())
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted != 1 {
		return ErrConflict
	}
	return tx.Commit()
}

func NewOperatorAdminHandler(store *Store, config Config, trust operatordelegation.Config, auditRepositories ...auditcontract.Repository) (http.Handler, error) {
	if len(auditRepositories) > 1 {
		return nil, errors.New("at most one audit repository may be configured")
	}
	var auditRepository auditcontract.Repository
	if len(auditRepositories) == 1 {
		auditRepository = auditRepositories[0]
	}
	if err := trust.Validate(); err != nil {
		return nil, err
	}
	for _, grant := range trust.Grants {
		for _, organization := range grant.Organizations {
			if organization == "*" {
				continue
			}
			if _, ok := config.Organizations[organization]; !ok {
				return nil, errors.New("operator delegation grant references unknown enrollment organization")
			}
		}
		for _, permission := range grant.Permissions {
			if (permission == "approve" || permission == "reject") && config.ApprovalLifetimeSeconds == 0 {
				return nil, errors.New("operator review grants require configured approval lifetime")
			}
		}
	}
	verifier, err := operatordelegation.NewVerifier(trust, store)
	if err != nil {
		return nil, err
	}
	handler, err := newAdminHandler(store, config, true)
	if err != nil {
		return nil, err
	}
	organizations := make([]string, 0, len(config.Organizations))
	for name := range config.Organizations {
		organizations = append(organizations, name)
	}
	slices.Sort(organizations)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" ||
			r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("X-ZPR-CSRF") != "" {
			log.Print("Operator enrollment denied: browser credentials reached private API")
			writeOperatorError(w, http.StatusForbidden)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, operatordelegation.MaxBody))
		if err != nil {
			log.Print("Operator enrollment denied: oversized or unreadable body")
			apiError(w, http.StatusRequestEntityTooLarge, "Enrollment request exceeds 8192 bytes or could not be read.")
			return
		}
		grant, err := verifier.Verify(r, body, time.Now())
		if err != nil {
			log.Printf("Operator enrollment delegation denied: %T", err)
			writeOperatorError(w, http.StatusForbidden)
			return
		}
		principal := Principal{Name: operatordelegation.AuditIdentity(grant.Issuer, grant.Subject),
			Organizations: grant.Organizations, Permissions: grant.Permissions}
		if slices.Contains(grant.Organizations, "*") {
			principal.Organizations = organizations
		}
		request := r.Clone(context.WithValue(r.Context(), operatorPrincipalKey{}, principal))
		request.URL.Path = APIPrefix + strings.TrimPrefix(r.URL.Path, operatordelegation.Prefix)
		request.URL.RawPath = ""
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.Header.Del(operatordelegation.Header)
		if auditRepository != nil {
			event, ok, err := operatorMutationEvent(r, body, grant, principal.Organizations, time.Now())
			if err != nil {
				log.Printf("Operator enrollment audit event could not be prepared: %T", err)
				apiError(w, http.StatusServiceUnavailable, "Enrollment audit history is unavailable.")
				return
			}
			if ok {
				if _, err := auditRepository.Append(r.Context(), event); err != nil {
					log.Printf("Operator enrollment audit event could not be recorded: %T", err)
					apiError(w, http.StatusServiceUnavailable, "Enrollment audit history is unavailable.")
					return
				}
			}
		}
		handler.ServeHTTP(w, request)
	}), nil
}

func operatorMutationEvent(r *http.Request, body []byte, grant operatorauth.Grant, organizations []string, now time.Time) (auditcontract.Event, bool, error) {
	if r.Method == http.MethodGet {
		return operatorReadEvent(r, body, grant, organizations, now)
	}
	if r.Method != http.MethodPost {
		return auditcontract.Event{}, false, nil
	}
	path := strings.TrimPrefix(r.URL.Path, operatordelegation.Prefix)
	action, permission, target := "", "", ""
	switch {
	case path == "invitations":
		action, permission, target = "enrollment.invitation.create", "create", "/api/enrollment/v1/invitations"
	case path == "preflight":
		action, permission, target = "enrollment.preflight.record", "create", "/api/enrollment/v1/preflight"
	case strings.HasPrefix(path, "invitations/"):
		parts := strings.Split(path, "/")
		if len(parts) != 3 || parts[1] == "" {
			return auditcontract.Event{}, false, nil
		}
		switch parts[2] {
		case "cancel":
			action, permission = "enrollment.invitation.cancel", "cancel"
		case "approve":
			action, permission = "enrollment.invitation.approve", "approve"
		case "reject":
			action, permission = "enrollment.invitation.reject", "reject"
		default:
			return auditcontract.Event{}, false, nil
		}
		target = "/api/enrollment/v1/invitations/{id}/" + parts[2]
	default:
		return auditcontract.Event{}, false, nil
	}
	var input struct {
		Organization string `json:"organization"`
	}
	if err := json.Unmarshal(body, &input); err != nil || input.Organization == "" ||
		!slices.Contains(organizations, input.Organization) ||
		!slices.Contains(grant.Permissions, permission) {
		return auditcontract.Event{}, false, nil
	}
	var correlation [16]byte
	if _, err := rand.Read(correlation[:]); err != nil {
		return auditcontract.Event{}, false, err
	}
	return auditcontract.Event{
		OrganizationID: input.Organization,
		OccurredAt:     now.UTC(),
		ActorIssuer:    grant.Issuer,
		ActorSubject:   grant.Subject,
		Action:         action,
		Target:         target,
		Outcome:        "requested",
		Source:         "control-room",
		CorrelationID:  hex.EncodeToString(correlation[:]),
	}, true, nil
}

func operatorReadEvent(r *http.Request, body []byte, grant operatorauth.Grant, organizations []string, now time.Time) (auditcontract.Event, bool, error) {
	if len(body) != 0 {
		return auditcontract.Event{}, false, nil
	}
	path := strings.TrimPrefix(r.URL.Path, operatordelegation.Prefix)
	action, target := "", ""
	switch {
	case path == "invitations":
		action, target = "enrollment.invitations.read", "/api/enrollment/v1/invitations"
	case path == "preflight":
		action, target = "enrollment.preflight.read", "/api/enrollment/v1/preflight"
	case strings.HasPrefix(path, "invitations/") && !strings.Contains(strings.TrimPrefix(path, "invitations/"), "/"):
		action, target = "enrollment.invitation.read", "/api/enrollment/v1/invitations/{id}"
	default:
		return auditcontract.Event{}, false, nil
	}
	values := r.URL.Query()
	organizationValues := values["organization"]
	if len(organizationValues) != 1 || organizationValues[0] == "" {
		return auditcontract.Event{}, false, nil
	}
	for key, entries := range values {
		if len(entries) != 1 || (key != "organization" &&
			(path != "invitations" || (key != "limit" && key != "after"))) {
			return auditcontract.Event{}, false, nil
		}
	}
	organization := organizationValues[0]
	if !slices.Contains(organizations, organization) || !slices.Contains(grant.Permissions, "read") {
		return auditcontract.Event{}, false, nil
	}
	var correlation [16]byte
	if _, err := rand.Read(correlation[:]); err != nil {
		return auditcontract.Event{}, false, err
	}
	return auditcontract.Event{
		OrganizationID: organization,
		OccurredAt:     now.UTC(),
		ActorIssuer:    grant.Issuer,
		ActorSubject:   grant.Subject,
		Action:         action,
		Target:         target,
		Outcome:        "requested",
		Source:         "control-room",
		CorrelationID:  hex.EncodeToString(correlation[:]),
	}, true, nil
}

func writeOperatorError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	apiError(w, status, "Verified named-user delegation and independently authorized scope are required.")
}
