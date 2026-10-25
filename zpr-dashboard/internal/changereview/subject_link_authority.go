package changereview

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

const subjectLinkAuthoritySchema = `
CREATE TABLE IF NOT EXISTS subject_link_authorities (
 organization TEXT NOT NULL, id TEXT NOT NULL, version TEXT NOT NULL,
 kind TEXT NOT NULL, service_id TEXT NOT NULL, issuer TEXT NOT NULL,
 enabled INTEGER NOT NULL, PRIMARY KEY(organization,id),
 UNIQUE(organization,kind,service_id,issuer));
CREATE TABLE IF NOT EXISTS subject_link_authority_events (
 organization TEXT NOT NULL, id TEXT NOT NULL, version TEXT NOT NULL,
 evidence TEXT NOT NULL, PRIMARY KEY(organization,id,version));
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_authority_identity
BEFORE UPDATE OF organization,id ON subject_link_authorities
BEGIN SELECT RAISE(ABORT,'authority identity is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_authority_delete
BEFORE DELETE ON subject_link_authorities
BEGIN SELECT RAISE(ABORT,'authority retirement requires a versioned disable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_authority_event_update
BEFORE UPDATE ON subject_link_authority_events
BEGIN SELECT RAISE(ABORT,'authority evidence is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_authority_event_delete
BEFORE DELETE ON subject_link_authority_events
BEGIN SELECT RAISE(ABORT,'authority evidence is immutable'); END;`

type SubjectLinkAuthorityConfig struct {
	Kind      string `json:"kind"`
	ServiceID string `json:"service_id"`
	Issuer    string `json:"issuer"`
	Enabled   bool   `json:"enabled"`
}

type SubjectLinkAuthority struct {
	Organization string                     `json:"organization_id"`
	ID           string                     `json:"authority_id"`
	Version      string                     `json:"version"`
	Config       SubjectLinkAuthorityConfig `json:"config"`
}

type SubjectLinkAuthorityEvent struct {
	Authority SubjectLinkAuthority `json:"authority"`
	Actor     Principal            `json:"actor"`
	Reason    string               `json:"reason"`
	At        time.Time            `json:"at"`
}

func readSubjectLinkAuthority(ctx context.Context, q queryer, org, id string) (SubjectLinkAuthority, error) {
	var a SubjectLinkAuthority
	err := q.QueryRowContext(ctx, `SELECT organization,id,version,kind,service_id,issuer,enabled
FROM subject_link_authorities WHERE organization=? AND id=?`, org, id).
		Scan(&a.Organization, &a.ID, &a.Version, &a.Config.Kind, &a.Config.ServiceID, &a.Config.Issuer, &a.Config.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// PutSubjectLinkAuthority requires an independently authorized operator.
// Empty expectedVersion creates; all edits, including disable, require exact CAS.
// Registration records configuration intent, not trust provisioning or activation.
func (s *Store) PutSubjectLinkAuthority(ctx context.Context, org, id, expectedVersion string, config SubjectLinkAuthorityConfig, actor Principal, reason string) (SubjectLinkAuthority, error) {
	if !identifier.MatchString(org) || !identifier.MatchString(id) ||
		(expectedVersion != "" && !subjectLinkVersion(expectedVersion)) ||
		config.Kind != "bas_device" || !cleanText(config.ServiceID, 256) || !cleanText(config.Issuer, 256) ||
		!subjectLinkPrincipal(actor) || !cleanText(reason, 1024) {
		return SubjectLinkAuthority{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubjectLinkAuthority{}, err
	}
	defer tx.Rollback()
	current, err := readSubjectLinkAuthority(ctx, tx, org, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return SubjectLinkAuthority{}, err
	}
	missing := errors.Is(err, ErrNotFound)
	if (missing && expectedVersion != "") || (!missing && current.Version != expectedVersion) {
		return SubjectLinkAuthority{}, ErrConflict
	}
	if !missing && current.Config == config {
		return current, tx.Commit()
	}
	version := uint64(1)
	if !missing {
		prior, err := strconv.ParseUint(current.Version, 10, 64)
		if err != nil || !subjectLinkVersion(current.Version) {
			return SubjectLinkAuthority{}, errors.New("invalid stored authority version")
		}
		if prior == ^uint64(0) {
			return SubjectLinkAuthority{}, ErrConflict
		}
		version = prior + 1
	}
	var duplicate int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM subject_link_authorities
WHERE organization=? AND kind=? AND service_id=? AND issuer=? AND id<>?`,
		org, config.Kind, config.ServiceID, config.Issuer, id).Scan(&duplicate); err != nil {
		return SubjectLinkAuthority{}, err
	}
	if duplicate != 0 {
		return SubjectLinkAuthority{}, ErrConflict
	}
	a := SubjectLinkAuthority{org, id, strconv.FormatUint(version, 10), config}
	if missing {
		_, err = tx.ExecContext(ctx, "INSERT INTO subject_link_authorities VALUES(?,?,?,?,?,?,?)",
			org, id, a.Version, config.Kind, config.ServiceID, config.Issuer, config.Enabled)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE subject_link_authorities
SET version=?,kind=?,service_id=?,issuer=?,enabled=? WHERE organization=? AND id=? AND version=?`,
			a.Version, config.Kind, config.ServiceID, config.Issuer, config.Enabled, org, id, expectedVersion)
	}
	if err != nil {
		return SubjectLinkAuthority{}, err
	}
	evidence, err := json.Marshal(SubjectLinkAuthorityEvent{a, actor, reason, time.Now().UTC()})
	if err != nil {
		return SubjectLinkAuthority{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO subject_link_authority_events VALUES(?,?,?,?)",
		org, id, a.Version, string(evidence)); err != nil {
		return SubjectLinkAuthority{}, err
	}
	if err := invalidateSubjectLinkPublications(ctx, tx, org, "authority", id, actor, reason); err != nil {
		return SubjectLinkAuthority{}, err
	}
	return a, tx.Commit()
}

func (s *Store) GetSubjectLinkAuthority(ctx context.Context, org, id string) (SubjectLinkAuthority, error) {
	if !identifier.MatchString(org) || !identifier.MatchString(id) {
		return SubjectLinkAuthority{}, ErrInvalid
	}
	return readSubjectLinkAuthority(ctx, s.db, org, id)
}

// CaptureSubjectLinkAuthority reads one coherent current registration for review.
// Its result is not an atomic guard for a later review/publication transaction.
func (s *Store) CaptureSubjectLinkAuthority(ctx context.Context, org string, root SubjectLinkRoot) (SubjectLinkAuthority, error) {
	return captureSubjectLinkAuthority(ctx, s.db, org, root)
}

func captureSubjectLinkAuthority(ctx context.Context, q queryer, org string, root SubjectLinkRoot) (SubjectLinkAuthority, error) {
	if !identifier.MatchString(org) || !identifier.MatchString(root.AuthorityID) ||
		!subjectLinkVersion(root.AuthorityVersion) || root.SubjectKind != "device" || !cleanText(root.SubjectID, 256) {
		return SubjectLinkAuthority{}, ErrInvalid
	}
	a, err := readSubjectLinkAuthority(ctx, q, org, root.AuthorityID)
	if err != nil {
		return SubjectLinkAuthority{}, err
	}
	if a.Version != root.AuthorityVersion || !a.Config.Enabled {
		return SubjectLinkAuthority{}, ErrConflict
	}
	if a.Config.Kind != "bas_device" || !cleanText(a.Config.ServiceID, 256) || !cleanText(a.Config.Issuer, 256) {
		return SubjectLinkAuthority{}, errors.New("invalid stored authority configuration")
	}
	return a, nil
}

// GetSubjectLinkAuthorityEvent returns immutable historical configuration evidence,
// never a substitute for checking the current enabled version.
func (s *Store) GetSubjectLinkAuthorityEvent(ctx context.Context, org, id, version string) (SubjectLinkAuthorityEvent, error) {
	var event SubjectLinkAuthorityEvent
	if !identifier.MatchString(org) || !identifier.MatchString(id) || !subjectLinkVersion(version) {
		return event, ErrInvalid
	}
	var evidence string
	err := s.db.QueryRowContext(ctx, `SELECT evidence FROM subject_link_authority_events
WHERE organization=? AND id=? AND version=?`, org, id, version).Scan(&evidence)
	if errors.Is(err, sql.ErrNoRows) {
		return event, ErrNotFound
	}
	if err != nil {
		return event, err
	}
	if err := json.Unmarshal([]byte(evidence), &event); err != nil {
		return SubjectLinkAuthorityEvent{}, err
	}
	if event.Authority.Organization != org || event.Authority.ID != id || event.Authority.Version != version {
		return SubjectLinkAuthorityEvent{}, errors.New("authority evidence scope mismatch")
	}
	return event, nil
}
