package enrollment

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

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

func NewOperatorAdminHandler(store *Store, config Config, trust operatordelegation.Config) (http.Handler, error) {
	if err := trust.Validate(); err != nil {
		return nil, err
	}
	for _, grant := range trust.Grants {
		for _, organization := range grant.Organizations {
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
		request := r.Clone(context.WithValue(r.Context(), operatorPrincipalKey{}, principal))
		request.URL.Path = APIPrefix + strings.TrimPrefix(r.URL.Path, operatordelegation.Prefix)
		request.URL.RawPath = ""
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.Header.Del(operatordelegation.Header)
		handler.ServeHTTP(w, request)
	}), nil
}

func writeOperatorError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	apiError(w, status, "Verified named-user delegation and independently authorized scope are required.")
}
