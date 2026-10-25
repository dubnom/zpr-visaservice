package changereview

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"time"
)

const subjectLinkSourceSchema = `
CREATE TABLE IF NOT EXISTS subject_link_sources (
 organization TEXT NOT NULL, id TEXT NOT NULL, version TEXT NOT NULL,
 config TEXT NOT NULL, PRIMARY KEY(organization,id));
CREATE TABLE IF NOT EXISTS subject_link_source_events (
 organization TEXT NOT NULL, id TEXT NOT NULL, version TEXT NOT NULL,
 evidence TEXT NOT NULL, PRIMARY KEY(organization,id,version));
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_source_identity
BEFORE UPDATE OF organization,id ON subject_link_sources
BEGIN SELECT RAISE(ABORT,'source identity is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_source_delete
BEFORE DELETE ON subject_link_sources
BEGIN SELECT RAISE(ABORT,'source retirement requires a versioned disable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_source_event_update
BEFORE UPDATE ON subject_link_source_events
BEGIN SELECT RAISE(ABORT,'source evidence is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_source_event_delete
BEFORE DELETE ON subject_link_source_events
BEGIN SELECT RAISE(ABORT,'source evidence is immutable'); END;`

// ProviderID references independently provisioned provider configuration. This
// registration never contains credentials or certifies directory ACLs/identity.
type SubjectLinkSourceConfig struct {
	ProviderID     string   `json:"provider_id"`
	SelectorKind   string   `json:"selector_kind"`
	EnrichmentKeys []string `json:"enrichment_keys"`
	Enabled        bool     `json:"enabled"`
}

type SubjectLinkSource struct {
	Organization string                  `json:"organization_id"`
	ID           string                  `json:"source_id"`
	Version      string                  `json:"version"`
	Config       SubjectLinkSourceConfig `json:"config"`
}

type SubjectLinkSourceEvent struct {
	Source SubjectLinkSource `json:"source"`
	Actor  Principal         `json:"actor"`
	Reason string            `json:"reason"`
	At     time.Time         `json:"at"`
}

func (s *Store) GetSubjectLinkSource(ctx context.Context, org, id string) (SubjectLinkSource, error) {
	if !identifier.MatchString(org) || !identifier.MatchString(id) {
		return SubjectLinkSource{}, ErrInvalid
	}
	return readSubjectLinkSource(ctx, s.db, org, id)
}

func validSubjectLinkSourceConfig(config SubjectLinkSourceConfig) bool {
	if !identifier.MatchString(config.ProviderID) || config.SelectorKind != "ldap_entry_uuid" ||
		len(config.EnrichmentKeys) == 0 || len(config.EnrichmentKeys) > 128 {
		return false
	}
	seen := make(map[string]bool, len(config.EnrichmentKeys))
	for _, key := range config.EnrichmentKeys {
		if !subjectLinkKey.MatchString(key) || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func readSubjectLinkSource(ctx context.Context, q queryer, org, id string) (SubjectLinkSource, error) {
	var source SubjectLinkSource
	var config string
	err := q.QueryRowContext(ctx, `SELECT organization,id,version,config FROM subject_link_sources
WHERE organization=? AND id=?`, org, id).Scan(&source.Organization, &source.ID, &source.Version, &config)
	if errors.Is(err, sql.ErrNoRows) {
		return source, ErrNotFound
	}
	if err != nil {
		return source, err
	}
	if err := json.Unmarshal([]byte(config), &source.Config); err != nil {
		return SubjectLinkSource{}, err
	}
	if !subjectLinkVersion(source.Version) || !validSubjectLinkSourceConfig(source.Config) {
		return SubjectLinkSource{}, errors.New("invalid stored subject-link source registration")
	}
	return source, nil
}

// PutSubjectLinkSource requires an independently authorized operator. Every
// changed registration advances its version and commits immutable audit evidence.
func (s *Store) PutSubjectLinkSource(ctx context.Context, org, id, expectedVersion string, config SubjectLinkSourceConfig, actor Principal, reason string) (SubjectLinkSource, error) {
	if !identifier.MatchString(org) || !identifier.MatchString(id) ||
		(expectedVersion != "" && !subjectLinkVersion(expectedVersion)) ||
		!validSubjectLinkSourceConfig(config) || !subjectLinkPrincipal(actor) || !cleanText(reason, 1024) {
		return SubjectLinkSource{}, ErrInvalid
	}
	config.EnrichmentKeys = slices.Clone(config.EnrichmentKeys)
	slices.Sort(config.EnrichmentKeys)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubjectLinkSource{}, err
	}
	defer tx.Rollback()
	current, err := readSubjectLinkSource(ctx, tx, org, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return SubjectLinkSource{}, err
	}
	missing := errors.Is(err, ErrNotFound)
	if (missing && expectedVersion != "") || (!missing && current.Version != expectedVersion) {
		return SubjectLinkSource{}, ErrConflict
	}
	if !missing && current.Config.ProviderID == config.ProviderID && current.Config.SelectorKind == config.SelectorKind &&
		current.Config.Enabled == config.Enabled && slices.Equal(current.Config.EnrichmentKeys, config.EnrichmentKeys) {
		return current, tx.Commit()
	}
	version := uint64(1)
	if !missing {
		prior, err := strconv.ParseUint(current.Version, 10, 64)
		if err != nil {
			return SubjectLinkSource{}, err
		}
		if prior == ^uint64(0) {
			return SubjectLinkSource{}, ErrConflict
		}
		version = prior + 1
	}
	source := SubjectLinkSource{org, id, strconv.FormatUint(version, 10), config}
	encoded, err := json.Marshal(config)
	if err != nil {
		return SubjectLinkSource{}, err
	}
	if missing {
		_, err = tx.ExecContext(ctx, "INSERT INTO subject_link_sources VALUES(?,?,?,?)", org, id, source.Version, string(encoded))
	} else {
		_, err = tx.ExecContext(ctx, "UPDATE subject_link_sources SET version=?,config=? WHERE organization=? AND id=? AND version=?",
			source.Version, string(encoded), org, id, expectedVersion)
	}
	if err != nil {
		return SubjectLinkSource{}, err
	}
	evidence, err := json.Marshal(SubjectLinkSourceEvent{source, actor, reason, time.Now().UTC()})
	if err != nil {
		return SubjectLinkSource{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO subject_link_source_events VALUES(?,?,?,?)",
		org, id, source.Version, string(evidence)); err != nil {
		return SubjectLinkSource{}, err
	}
	if err := invalidateSubjectLinkPublications(ctx, tx, org, "source", id, actor, reason); err != nil {
		return SubjectLinkSource{}, err
	}
	return source, tx.Commit()
}

func (s *Store) GetSubjectLinkSourceEvent(ctx context.Context, org, id, version string) (SubjectLinkSourceEvent, error) {
	var event SubjectLinkSourceEvent
	if !identifier.MatchString(org) || !identifier.MatchString(id) || !subjectLinkVersion(version) {
		return event, ErrInvalid
	}
	var evidence string
	err := s.db.QueryRowContext(ctx, `SELECT evidence FROM subject_link_source_events
WHERE organization=? AND id=? AND version=?`, org, id, version).Scan(&evidence)
	if errors.Is(err, sql.ErrNoRows) {
		return event, ErrNotFound
	}
	if err != nil {
		return event, err
	}
	if err := json.Unmarshal([]byte(evidence), &event); err != nil {
		return SubjectLinkSourceEvent{}, err
	}
	if event.Source.Organization != org || event.Source.ID != id || event.Source.Version != version {
		return SubjectLinkSourceEvent{}, errors.New("source evidence scope mismatch")
	}
	return event, nil
}

type SubjectLinkRegistryCapture struct {
	Revision  string               `json:"revision"`
	Authority SubjectLinkAuthority `json:"authority"`
	Source    SubjectLinkSource    `json:"source"`
}

// CaptureSubjectLinkRegistry returns a coherent review context, not a signed VS
// snapshot or a publication epoch. Its digest covers the exact referenced pair.
func (s *Store) CaptureSubjectLinkRegistry(ctx context.Context, org string, root SubjectLinkRoot, target SubjectLinkTarget) (SubjectLinkRegistryCapture, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubjectLinkRegistryCapture{}, err
	}
	defer tx.Rollback()
	capture, err := captureSubjectLinkRegistry(ctx, tx, org, root, target)
	if err != nil {
		return capture, err
	}
	return capture, tx.Commit()
}

func captureSubjectLinkRegistry(ctx context.Context, q queryer, org string, root SubjectLinkRoot, target SubjectLinkTarget) (SubjectLinkRegistryCapture, error) {
	authority, err := captureSubjectLinkAuthority(ctx, q, org, root)
	if err != nil {
		return SubjectLinkRegistryCapture{}, err
	}
	if !identifier.MatchString(target.SourceID) || !subjectLinkVersion(target.SourceVersion) ||
		target.SelectorKind != "ldap_entry_uuid" || !subjectLinkUUID.MatchString(target.SubjectID) ||
		target.SubjectID == "00000000-0000-0000-0000-000000000000" {
		return SubjectLinkRegistryCapture{}, ErrInvalid
	}
	source, err := readSubjectLinkSource(ctx, q, org, target.SourceID)
	if err != nil {
		return SubjectLinkRegistryCapture{}, err
	}
	if source.Version != target.SourceVersion || source.Config.SelectorKind != target.SelectorKind || !source.Config.Enabled {
		return SubjectLinkRegistryCapture{}, ErrConflict
	}
	capture := SubjectLinkRegistryCapture{Authority: authority, Source: source}
	data, err := json.Marshal(capture)
	if err != nil {
		return SubjectLinkRegistryCapture{}, err
	}
	hash := sha256.Sum256(data)
	capture.Revision = hex.EncodeToString(hash[:])
	return capture, nil
}

func checkSubjectLinkRegistry(ctx context.Context, q queryer, p SubjectLinkProposal) error {
	capture, err := captureSubjectLinkRegistry(ctx, q, p.Organization, p.Root, p.Target)
	if err != nil {
		return err
	}
	if p.RegistryRevision != capture.Revision {
		return ErrConflict
	}
	for _, key := range p.EnrichmentKeys {
		if !slices.Contains(capture.Source.Config.EnrichmentKeys, key) {
			return ErrConflict
		}
	}
	return nil
}
