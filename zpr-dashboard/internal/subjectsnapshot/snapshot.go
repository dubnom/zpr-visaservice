// Package subjectsnapshot implements an offline, complete-snapshot contract.
// It does not publish links, authenticate device roots or enable enforcement.
package subjectsnapshot

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	Domain      = "ZPR-SUBJECT-LINK-SNAPSHOT-V1\x00"
	MaxPayload  = 1 << 20
	MaxEnvelope = 1400000
	MaxRecords  = 1024
)

var (
	ErrInvalid = errors.New("invalid subject-link snapshot")
	ErrTrust   = errors.New("subject-link snapshot authentication failed")
	ErrReplay  = errors.New("subject-link snapshot rollback, equivocation or epoch recovery required")
	idPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,255}$`)
	uuid       = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type Authority struct {
	ID        string `json:"authority_id"`
	Version   string `json:"version"`
	Kind      string `json:"kind"`
	ServiceID string `json:"service_id"`
	Issuer    string `json:"issuer"`
	Enabled   bool   `json:"enabled"`
}

type Source struct {
	ID             string   `json:"source_id"`
	Version        string   `json:"version"`
	ProviderID     string   `json:"provider_id"`
	SelectorKind   string   `json:"selector_kind"`
	EnrichmentKeys []string `json:"enrichment_keys"`
	Enabled        bool     `json:"enabled"`
}

type Root struct {
	AuthorityID      string `json:"authority_id"`
	AuthorityVersion string `json:"authority_version"`
	SubjectKind      string `json:"subject_kind"`
	SubjectID        string `json:"subject_id"`
}

type Target struct {
	SourceID      string `json:"source_id"`
	SourceVersion string `json:"source_version"`
	SelectorKind  string `json:"selector_kind"`
	SubjectID     string `json:"subject_id"`
}

type Binding struct {
	ID             string   `json:"binding_id"`
	Version        string   `json:"version"`
	Root           Root     `json:"root"`
	Target         Target   `json:"target"`
	EnrichmentKeys []string `json:"enrichment_keys"`
	NotBefore      string   `json:"not_before"`
	ExpiresAt      string   `json:"expires_at"`
}

type Tombstone struct {
	ID      string `json:"binding_id"`
	Version string `json:"version"`
}

// Times are canonical positive decimal Unix seconds. No clock skew or lease
// duration is supplied by this package. Array order is preserved and signed.
type Payload struct {
	Version      int         `json:"version"`
	Organization string      `json:"organization_id"`
	Recipient    string      `json:"vs_id"`
	Epoch        string      `json:"epoch"`
	Sequence     string      `json:"sequence"`
	IssuedAt     string      `json:"issued_at"`
	ExpiresAt    string      `json:"expires_at"`
	Authorities  []Authority `json:"authorities"`
	Sources      []Source    `json:"sources"`
	Bindings     []Binding   `json:"bindings"`
	Tombstones   []Tombstone `json:"tombstones"`
}

type envelope struct {
	Version   int    `json:"version"`
	KeyID     string `json:"key_id"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

type Scope struct {
	Organization string
	Recipient    string
	Epoch        string
}

// Verified is returned only after complete authentication and validation.
// Callers must persist its exact envelope and ordering state atomically before
// future acknowledgement. This package performs no persistence or activation.
type Verified struct {
	payload Payload
	digest  string
}

func (v Verified) Digest() string { return v.digest }

type Decision string

const (
	Advance Decision = "advance"
	Replay  Decision = "historical-replay"
)

func decimal(value string, limit uint64) (uint64, bool) {
	n, err := strconv.ParseUint(value, 10, 64)
	return n, err == nil && n > 0 && n <= limit && strconv.FormatUint(n, 10) == value
}

func text(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, c := range value {
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}

func keys(values []string) bool {
	if len(values) == 0 || len(values) > 128 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !keyPattern.MatchString(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func (p Payload) validate() error {
	issued, validIssued := decimal(p.IssuedAt, math.MaxInt64)
	expires, validExpires := decimal(p.ExpiresAt, math.MaxInt64)
	if p.Version != 1 || !idPattern.MatchString(p.Organization) || !idPattern.MatchString(p.Recipient) ||
		!idPattern.MatchString(p.Epoch) || !validIssued || !validExpires || expires <= issued {
		return ErrInvalid
	}
	if _, ok := decimal(p.Sequence, math.MaxUint64); !ok {
		return ErrInvalid
	}
	if p.Authorities == nil || p.Sources == nil || p.Bindings == nil || p.Tombstones == nil ||
		len(p.Authorities) > MaxRecords || len(p.Sources) > MaxRecords || len(p.Bindings) > MaxRecords || len(p.Tombstones) > MaxRecords {
		return ErrInvalid
	}
	authorities := map[string]Authority{}
	authorityTuples := map[[3]string]bool{}
	for _, a := range p.Authorities {
		_, version := decimal(a.Version, math.MaxUint64)
		tuple := [3]string{a.Kind, a.ServiceID, a.Issuer}
		if !idPattern.MatchString(a.ID) || !version || a.Kind != "bas_device" || !text(a.ServiceID) || !text(a.Issuer) ||
			authorities[a.ID].ID != "" || authorityTuples[tuple] {
			return ErrInvalid
		}
		authorities[a.ID], authorityTuples[tuple] = a, true
	}
	sources := map[string]Source{}
	for _, s := range p.Sources {
		_, version := decimal(s.Version, math.MaxUint64)
		if !idPattern.MatchString(s.ID) || !version || !idPattern.MatchString(s.ProviderID) ||
			s.SelectorKind != "ldap_entry_uuid" || !keys(s.EnrichmentKeys) || sources[s.ID].ID != "" {
			return ErrInvalid
		}
		sources[s.ID] = s
	}
	seen := map[string]bool{}
	for _, b := range p.Bindings {
		_, version := decimal(b.Version, math.MaxUint64)
		start, validStart := decimal(b.NotBefore, math.MaxInt64)
		end, validEnd := decimal(b.ExpiresAt, math.MaxInt64)
		a, authority := authorities[b.Root.AuthorityID]
		s, source := sources[b.Target.SourceID]
		if !idPattern.MatchString(b.ID) || !version || seen[b.ID] || !validStart || !validEnd || end <= start || end <= issued ||
			!authority || !a.Enabled || a.Version != b.Root.AuthorityVersion || b.Root.SubjectKind != "device" || !text(b.Root.SubjectID) ||
			!source || !s.Enabled || s.Version != b.Target.SourceVersion || b.Target.SelectorKind != s.SelectorKind ||
			!uuid.MatchString(b.Target.SubjectID) || b.Target.SubjectID == "00000000-0000-0000-0000-000000000000" || !keys(b.EnrichmentKeys) {
			return ErrInvalid
		}
		for _, key := range b.EnrichmentKeys {
			found := false
			for _, allowed := range s.EnrichmentKeys {
				found = found || key == allowed
			}
			if !found {
				return ErrInvalid
			}
		}
		seen[b.ID] = true
	}
	for _, t := range p.Tombstones {
		_, version := decimal(t.Version, math.MaxUint64)
		if !idPattern.MatchString(t.ID) || !version || seen[t.ID] {
			return ErrInvalid
		}
		seen[t.ID] = true
	}
	return nil
}

// strictValue requires every schema field exactly once, including empty arrays
// and false booleans. It does not normalize signed bytes or accept JSON aliases.
func strictValue(decoder *json.Decoder, shape any) error {
	token, err := decoder.Token()
	if err != nil {
		return ErrInvalid
	}
	switch s := shape.(type) {
	case map[string]any:
		if token != json.Delim('{') {
			return ErrInvalid
		}
		seen := map[string]bool{}
		for decoder.More() {
			name, err := decoder.Token()
			if err != nil {
				return ErrInvalid
			}
			key, ok := name.(string)
			child, found := s[key]
			if !ok || !found || seen[key] {
				return ErrInvalid
			}
			seen[key] = true
			if err := strictValue(decoder, child); err != nil {
				return err
			}
		}
		if len(seen) != len(s) {
			return ErrInvalid
		}
		if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
			return ErrInvalid
		}
	case []any:
		if token != json.Delim('[') {
			return ErrInvalid
		}
		count := 0
		for decoder.More() {
			count++
			if count > MaxRecords {
				return ErrInvalid
			}
			if err := strictValue(decoder, s[0]); err != nil {
				return err
			}
		}
		if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
			return ErrInvalid
		}
	case string:
		if _, ok := token.(string); !ok {
			return ErrInvalid
		}
	case bool:
		if _, ok := token.(bool); !ok {
			return ErrInvalid
		}
	case int:
		if token != json.Number("1") {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

var rootShape = map[string]any{"authority_id": "", "authority_version": "", "subject_kind": "", "subject_id": ""}
var targetShape = map[string]any{"source_id": "", "source_version": "", "selector_kind": "", "subject_id": ""}
var payloadShape = map[string]any{
	"version": 1, "organization_id": "", "vs_id": "", "epoch": "", "sequence": "", "issued_at": "", "expires_at": "",
	"authorities": []any{map[string]any{"authority_id": "", "version": "", "kind": "", "service_id": "", "issuer": "", "enabled": false}},
	"sources":     []any{map[string]any{"source_id": "", "version": "", "provider_id": "", "selector_kind": "", "enrichment_keys": []any{""}, "enabled": false}},
	"bindings": []any{map[string]any{"binding_id": "", "version": "", "root": rootShape, "target": targetShape,
		"enrichment_keys": []any{""}, "not_before": "", "expires_at": ""}},
	"tombstones": []any{map[string]any{"binding_id": "", "version": ""}},
}

func decode(data []byte, shape any, out any) error {
	if !utf8.Valid(data) {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := strictValue(decoder, shape); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrInvalid
	}
	if err := json.Unmarshal(data, out); err != nil {
		return ErrInvalid
	}
	return nil
}

func Sign(p Payload, keyID string, key ed25519.PrivateKey) ([]byte, error) {
	if !idPattern.MatchString(keyID) || len(key) != ed25519.PrivateKeySize || p.validate() != nil {
		return nil, ErrInvalid
	}
	data, err := json.Marshal(p)
	if err != nil || len(data) > MaxPayload {
		return nil, ErrInvalid
	}
	signature := ed25519.Sign(key, append([]byte(Domain), data...))
	return json.Marshal(envelope{1, keyID, base64.RawURLEncoding.EncodeToString(data), base64.RawURLEncoding.EncodeToString(signature)})
}

// Verify uses only independently provisioned pins and an explicit current time.
// key_id is an untrusted lookup hint; a payload never supplies trust material.
func Verify(data []byte, pins map[string]ed25519.PublicKey, scope Scope, now uint64) (Verified, error) {
	if len(data) > MaxEnvelope || len(pins) == 0 || len(pins) > 64 ||
		!idPattern.MatchString(scope.Organization) || !idPattern.MatchString(scope.Recipient) || !idPattern.MatchString(scope.Epoch) ||
		now == 0 || now > math.MaxInt64 {
		return Verified{}, ErrInvalid
	}
	for id, key := range pins {
		if !idPattern.MatchString(id) || len(key) != ed25519.PublicKeySize {
			return Verified{}, ErrTrust
		}
	}
	var e envelope
	if err := decode(data, map[string]any{"version": 1, "key_id": "", "payload": "", "signature": ""}, &e); err != nil {
		return Verified{}, err
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(e.Payload)
	if err != nil || len(raw) > MaxPayload || base64.RawURLEncoding.EncodeToString(raw) != e.Payload {
		return Verified{}, ErrInvalid
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(e.Signature)
	key, trusted := pins[e.KeyID]
	if err != nil || len(signature) != ed25519.SignatureSize || base64.RawURLEncoding.EncodeToString(signature) != e.Signature ||
		!trusted || !ed25519.Verify(key, append([]byte(Domain), raw...), signature) {
		return Verified{}, ErrTrust
	}
	var p Payload
	if err := decode(raw, payloadShape, &p); err != nil || p.validate() != nil {
		return Verified{}, ErrInvalid
	}
	issued, _ := decimal(p.IssuedAt, math.MaxInt64)
	expires, _ := decimal(p.ExpiresAt, math.MaxInt64)
	if p.Organization != scope.Organization || p.Recipient != scope.Recipient || p.Epoch != scope.Epoch || now < issued || now >= expires {
		return Verified{}, ErrInvalid
	}
	hash := sha256.Sum256(raw)
	return Verified{p, hex.EncodeToString(hash[:])}, nil
}

// Compare accepts only authenticated complete snapshots. A higher sequence must
// not lower retained record versions, revive a tombstoned ID, or forget prior
// tombstones. This conservative v1 contract has no tombstone compaction.
// It does not write state: persistence and acknowledgement are a future gate.
func Compare(prior, next Verified) (Decision, error) {
	a, b := prior.payload, next.payload
	if prior.digest == "" || next.digest == "" || a.Organization != b.Organization || a.Recipient != b.Recipient || a.Epoch != b.Epoch {
		return "", ErrReplay
	}
	oldSeq, _ := decimal(a.Sequence, math.MaxUint64)
	newSeq, _ := decimal(b.Sequence, math.MaxUint64)
	if newSeq < oldSeq || (newSeq == oldSeq && prior.digest != next.digest) {
		return "", ErrReplay
	}
	if newSeq == oldSeq {
		return Replay, nil
	}
	oldIssued, _ := decimal(a.IssuedAt, math.MaxInt64)
	newIssued, _ := decimal(b.IssuedAt, math.MaxInt64)
	if newIssued < oldIssued {
		return "", ErrReplay
	}
	tombstones := map[string]string{}
	active := map[string]string{}
	for _, t := range b.Tombstones {
		tombstones[t.ID] = t.Version
	}
	for _, link := range b.Bindings {
		active[link.ID] = link.Version
	}
	for _, t := range a.Tombstones {
		if active[t.ID] != "" || !atLeast(tombstones[t.ID], t.Version) {
			return "", ErrReplay
		}
	}
	for _, link := range a.Bindings {
		if active[link.ID] == "" && !atLeast(tombstones[link.ID], link.Version) {
			return "", ErrReplay
		}
	}
	if !recordsAdvance(a.Authorities, b.Authorities, true, func(a Authority) (string, string) { return a.ID, a.Version }) ||
		!recordsAdvance(a.Sources, b.Sources, true, func(s Source) (string, string) { return s.ID, s.Version }) ||
		!recordsAdvance(a.Bindings, b.Bindings, false, func(b Binding) (string, string) { return b.ID, b.Version }) {
		return "", ErrReplay
	}
	return Advance, nil
}

func atLeast(next, prior string) bool {
	n, ok := decimal(next, math.MaxUint64)
	p, valid := decimal(prior, math.MaxUint64)
	return ok && valid && n >= p
}

func recordsAdvance[T any](prior, next []T, retain bool, identity func(T) (string, string)) bool {
	old := map[string]T{}
	for _, item := range prior {
		id, _ := identity(item)
		old[id] = item
	}
	for _, item := range next {
		id, version := identity(item)
		if p, found := old[id]; found {
			_, priorVersion := identity(p)
			if !atLeast(version, priorVersion) {
				return false
			}
			if version == priorVersion {
				before, err := json.Marshal(p)
				after, nextErr := json.Marshal(item)
				if err != nil || nextErr != nil || !bytes.Equal(before, after) {
					return false
				}
			}
		}
		delete(old, id)
	}
	return !retain || len(old) == 0
}
