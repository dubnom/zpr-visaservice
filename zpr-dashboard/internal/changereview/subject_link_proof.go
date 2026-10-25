package changereview

import (
	"context"
	"errors"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

const maxSubjectLinkProofNonces = 8192

var ErrProofCapacity = errors.New("subject link review proof nonce capacity exhausted")

var _ operatordelegation.ReplayStore = (*Store)(nil)

// ConsumeOperatorDelegation commits proof consumption separately from a review.
// A failed or uncertain review must use fresh proof, retaining its retry key.
func (s *Store) ConsumeOperatorDelegation(ctx context.Context, hash string, expires, now time.Time) error {
	if !digest(hash) || now.UnixNano() <= 0 || !time.Unix(0, now.UnixNano()).Equal(now) ||
		!time.Unix(0, expires.UnixNano()).Equal(expires) || !expires.After(now) ||
		expires.Sub(now) > 32*time.Second {
		return ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM subject_link_review_nonces WHERE expires_at<=?", now.UnixNano()); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM subject_link_review_nonces").Scan(&count); err != nil {
		return err
	}
	if count >= maxSubjectLinkProofNonces {
		return ErrProofCapacity
	}
	result, err := tx.ExecContext(ctx,
		"INSERT INTO subject_link_review_nonces(token_hash,expires_at) VALUES(?,?) ON CONFLICT(token_hash) DO NOTHING",
		hash, expires.UnixNano())
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
