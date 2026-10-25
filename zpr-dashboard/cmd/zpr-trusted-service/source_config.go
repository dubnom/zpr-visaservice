package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

// One independently provisioned source per provider process. Policy supplies
// declarations/mappings, never bind credentials or directory-person associations.
type sourceConfig struct {
	SourceID         string   `json:"source_id"`
	SubjectSelector  string   `json:"subject_selector"`
	IdentityKey      string   `json:"identity_key"`
	URL              string   `json:"url"`
	CAFile           string   `json:"ca_file"`
	BaseDN           string   `json:"base_dn"`
	BindDN           string   `json:"bind_dn"`
	BindPasswordFile string   `json:"bind_password_file"`
	Attributes       []string `json:"attributes"`
}

var sourceIdentifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var sourceIdentityKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,127}$`)
var subjectUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type immutableLDAPProvider struct {
	config sourceConfig
	ldap   ldapProvider
}

func configuredSource(path string) (immutableLDAPProvider, error) {
	var store immutableLDAPProvider
	data, err := servicehttp.ReadOperatorFile(path, 64<<10, true)
	if err != nil {
		return store, err
	}
	if err := decodeLookupJSON(data, &store.config); err != nil {
		return store, err
	}
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(data, &properties); err != nil {
		return store, err
	}
	for key := range properties {
		switch key {
		case "source_id", "subject_selector", "identity_key", "url", "ca_file", "base_dn", "bind_dn", "bind_password_file", "attributes":
		default:
			return store, errors.New("unknown or noncanonical source configuration field")
		}
	}
	config := store.config
	if !filepath.IsAbs(config.CAFile) || !filepath.IsAbs(config.BindPasswordFile) {
		return store, errors.New("source CA and password references must be absolute protected file paths")
	}
	if !sourceIdentifier.MatchString(config.SourceID) || config.SourceID == "default" || strings.Contains(config.SourceID, "__source__") ||
		config.SubjectSelector != "ldap_entry_uuid" || !sourceIdentityKey.MatchString(config.IdentityKey) {
		return store, errors.New("source requires an exact source ID, ldap_entry_uuid selector and explicit identity key")
	}
	uri, err := url.Parse(config.URL)
	if err != nil || uri.Scheme != "ldaps" || uri.Hostname() == "" || uri.User != nil ||
		uri.Path != "" || uri.RawQuery != "" || uri.Fragment != "" {
		return store, errors.New("source requires an LDAPS origin")
	}
	for _, dn := range []string{config.BaseDN, config.BindDN} {
		parsed, err := ldap.ParseDN(dn)
		if err != nil || len(parsed.RDNs) == 0 {
			return store, errors.New("source requires nonempty base and bind DNs")
		}
	}
	if len(config.Attributes) == 0 || len(config.Attributes) > 64 {
		return store, errors.New("source requires 1 to 64 approved LDAP attributes")
	}
	seen := map[string]bool{}
	for _, name := range config.Attributes {
		key := strings.ToLower(name)
		if !safeLDAPAttribute(name) || key != name || seen[key] || sensitiveSourceAttribute(key) {
			return store, errors.New("source attributes must be unique lowercase approved LDAP names without credential fields")
		}
		seen[key] = true
	}
	pem, err := servicehttp.ReadOperatorFile(config.CAFile, 1<<20, false)
	if err != nil {
		return store, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return store, errors.New("invalid source LDAP CA")
	}
	if _, err := sourcePassword(config.BindPasswordFile); err != nil {
		return store, err
	}
	store.ldap = ldapProvider{uri: config.URL, baseDN: config.BaseDN, bindDN: config.BindDN, ca: roots, passwordFile: config.BindPasswordFile}
	return store, nil
}

func sensitiveSourceAttribute(name string) bool {
	for _, excluded := range []string{"password", "passwd", "pwd", "token", "secret", "credential", "private", "key", "auth", "certificate"} {
		if strings.Contains(name, excluded) {
			return true
		}
	}
	return false
}

func sourcePassword(path string) (string, error) {
	data, err := servicehttp.ReadOperatorFile(path, 64<<10, true)
	if err != nil {
		return "", err
	}
	defer clear(data)
	password := strings.TrimSuffix(string(data), "\n")
	if password == "" || strings.ContainsAny(password, "\x00\r\n") {
		return "", errors.New("source bind password file is invalid")
	}
	return password, nil
}

func (p immutableLDAPProvider) lookup(ctx context.Context, identities []identity) (map[string][]string, error) {
	var subject string
	for _, ident := range identities {
		if ident.Key != p.config.IdentityKey {
			continue
		}
		if subject != "" || !validSubjectUUID(ident.Value) {
			return nil, errors.New("immutable source requires exactly one lookup identity")
		}
		subject = ident.Value
	}
	if !validSubjectUUID(subject) {
		return nil, errors.New("immutable source requires a canonical UUID identity; no name fallback")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := p.ldap.dial()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	return immutableLDAPAttributes(ctx, conn, p.config.BaseDN, subject, p.config.Attributes)
}

func validSubjectUUID(subject string) bool {
	return subjectUUID.MatchString(subject) && subject != "00000000-0000-0000-0000-000000000000"
}

func immutableLDAPAttributes(ctx context.Context, searcher ldapSearcher, base, subject string, approved []string) (map[string][]string, error) {
	if !validSubjectUUID(subject) {
		return nil, errors.New("invalid immutable LDAP subject")
	}
	requested := slices.Clone(approved)
	if !slices.Contains(requested, "entryuuid") {
		requested = append(requested, "entryUUID")
	}
	request := ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 5, false, ldapFilter("entryUUID", subject), requested, nil)
	result, err := lookupLDAPSearch(ctx, searcher, request)
	if err != nil {
		return nil, err
	}
	if len(result.Entries) != 1 || result.Entries[0] == nil {
		return nil, errors.New("immutable LDAP source requires exactly one matching entry")
	}
	entry := result.Entries[0]
	baseDN, err := ldap.ParseDN(base)
	if err != nil {
		return nil, err
	}
	entryDN, err := ldap.ParseDN(entry.DN)
	if err != nil || !(baseDN.EqualFold(entryDN) || baseDN.AncestorOfFold(entryDN)) {
		return nil, errors.New("immutable LDAP entry is outside configured base")
	}
	seen := map[string]bool{}
	for _, attribute := range entry.Attributes {
		if attribute == nil || seen[strings.ToLower(attribute.Name)] {
			return nil, errors.New("invalid or duplicate immutable LDAP attribute")
		}
		seen[strings.ToLower(attribute.Name)] = true
	}
	uuids, err := lookupLDAPAttribute(entry, "entryUUID")
	if err != nil || len(uuids) != 1 || uuids[0] != subject {
		return nil, errors.New("immutable LDAP entry lacks exact UUID evidence")
	}
	values := make(map[string][]string)
	for _, name := range approved {
		attribute, err := lookupLDAPAttribute(entry, name)
		if err != nil {
			return nil, fmt.Errorf("immutable LDAP attribute %s: %w", name, err)
		}
		if attribute != nil {
			values[name] = slices.Clone(attribute)
		}
	}
	return values, nil
}
