package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-ldap/ldap/v3"
	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

type subjectLinkProviderConfig struct {
	Organization     string `json:"organization_id"`
	SourceID         string `json:"source_id"`
	SourceVersion    string `json:"source_version"`
	ProviderID       string `json:"provider_id"`
	URL              string `json:"url"`
	CAFile           string `json:"ca_file"`
	BaseDN           string `json:"base_dn"`
	BindDN           string `json:"bind_dn"`
	BindPasswordFile string `json:"bind_password_file"`
}

type subjectLinkDirectoryProvider struct {
	config  subjectLinkProviderConfig
	address string
	tls     *tls.Config
}

type subjectLinkProviders struct {
	entries []subjectLinkDirectoryProvider
}

var subjectLinkProviderUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// The caller explicitly selects this file; no existing assertion registry or
// Simulator environment can enable review providers or supply their identities.
func loadSubjectLinkProviders(path string) (*subjectLinkProviders, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("subject-link providers require an absolute protected configuration path")
	}
	data, err := servicehttp.ReadOperatorFile(path, 64<<10, true)
	if err != nil {
		return nil, err
	}
	var config struct {
		Version   int                         `json:"version"`
		Providers []subjectLinkProviderConfig `json:"providers"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if !utf8.Valid(data) || validateDocumentJSON(decoder, 0) != nil {
		return nil, errors.New("subject-link provider configuration requires unambiguous UTF-8 JSON")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("subject-link provider configuration has trailing data")
	}
	if err := decodeOperatorConfig(data, &config); err != nil {
		return nil, err
	}
	// Exact names avoid encoding/json's case-insensitive field aliases.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if len(fields) != 2 || fields["version"] == nil || fields["providers"] == nil ||
		config.Version != 1 || len(config.Providers) == 0 || len(config.Providers) > 64 {
		return nil, errors.New("subject-link providers require version 1 and 1 to 64 explicit providers")
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(fields["providers"], &entries); err != nil {
		return nil, err
	}
	providers := &subjectLinkProviders{}
	seen := map[string]bool{}
	for i, entry := range config.Providers {
		for _, key := range []string{"organization_id", "source_id", "source_version", "provider_id", "url", "ca_file", "base_dn", "bind_dn", "bind_password_file"} {
			if entries[i][key] == nil || bytes.Equal(bytes.TrimSpace(entries[i][key]), []byte("null")) {
				return nil, errors.New("subject-link provider fields must be explicit and canonically named")
			}
		}
		if len(entries[i]) != 9 || !reviewIdentifier.MatchString(entry.Organization) || !reviewIdentifier.MatchString(entry.SourceID) ||
			!reviewIdentifier.MatchString(entry.ProviderID) || !filepath.IsAbs(entry.CAFile) || !filepath.IsAbs(entry.BindPasswordFile) {
			return nil, errors.New("subject-link provider requires exact identifiers and absolute CA/password references")
		}
		version, err := strconv.ParseUint(entry.SourceVersion, 10, 64)
		if err != nil || version == 0 || strconv.FormatUint(version, 10) != entry.SourceVersion {
			return nil, errors.New("subject-link provider source version must be a positive canonical decimal string")
		}
		key := entry.Organization + "/" + entry.SourceID + "/" + entry.SourceVersion
		if seen[key] {
			return nil, errors.New("duplicate subject-link provider source version")
		}
		seen[key] = true
		if _, err := subjectLinkProviderPassword(entry.BindPasswordFile); err != nil {
			return nil, err
		}
		scope := assertionSourceScope{BaseDN: entry.BaseDN, BindDN: entry.BindDN, BindPasswordFile: entry.BindPasswordFile}
		address, tlsConfig, err := networkAssertionLDAPConnection(assertionSourceConfig{
			URL: entry.URL, Attributes: []string{"entryUUID"},
			Organizations: map[string]assertionSourceScope{entry.Organization: scope},
		})
		if err != nil {
			return nil, err
		}
		ca, err := servicehttp.ReadOperatorFile(entry.CAFile, 1<<20, false)
		if err != nil {
			return nil, err
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(ca) {
			return nil, errors.New("subject-link provider CA file has no certificates")
		}
		tlsConfig.RootCAs = roots
		providers.entries = append(providers.entries, subjectLinkDirectoryProvider{entry, address, tlsConfig})
	}
	return providers, nil
}

func subjectLinkProviderPassword(path string) (string, error) {
	data, err := servicehttp.ReadOperatorFile(path, 4096, true)
	if err != nil {
		return "", err
	}
	defer clear(data)
	password := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if password == "" || strings.ContainsAny(password, "\x00\r\n") {
		return "", errors.New("subject-link provider password file is invalid")
	}
	return password, nil
}

// validator binds independently provisioned transport to an exact stored
// registration. The resulting callback never re-enters the locked review Store.
func (providers *subjectLinkProviders) validator(capture changereview.SubjectLinkRegistryCapture) (changereview.SubjectLinkSourceCheck, error) {
	if providers == nil || capture.Revision == "" || !capture.Source.Config.Enabled || !capture.Authority.Config.Enabled ||
		capture.Source.Config.SelectorKind != "ldap_entry_uuid" || capture.Source.Organization != capture.Authority.Organization {
		return nil, errors.New("subject-link provider requires a current enabled registry capture")
	}
	for _, provider := range providers.entries {
		config := provider.config
		if config.Organization != capture.Source.Organization || config.SourceID != capture.Source.ID ||
			config.SourceVersion != capture.Source.Version || config.ProviderID != capture.Source.Config.ProviderID {
			continue
		}
		return func(ctx context.Context, proposal changereview.SubjectLinkProposal) error {
			if proposal.RegistryRevision != capture.Revision || proposal.Organization != config.Organization ||
				proposal.Root.AuthorityID != capture.Authority.ID || proposal.Root.AuthorityVersion != capture.Authority.Version ||
				proposal.Root.SubjectKind != "device" || proposal.Target.SourceID != config.SourceID ||
				proposal.Target.SourceVersion != config.SourceVersion || proposal.Target.SelectorKind != "ldap_entry_uuid" ||
				!subjectLinkProviderUUID.MatchString(proposal.Target.SubjectID) ||
				proposal.Target.SubjectID == "00000000-0000-0000-0000-000000000000" {
				return changereview.ErrConflict
			}
			password, err := subjectLinkProviderPassword(config.BindPasswordFile)
			if err != nil {
				return err
			}
			result, err := searchNetworkAssertionLDAPWithPassword(ctx, provider.address, provider.tls,
				assertionSourceScope{BaseDN: config.BaseDN, BindDN: config.BindDN, BindPasswordFile: config.BindPasswordFile},
				password, "(entryUUID="+ldap.EscapeFilter(proposal.Target.SubjectID)+")", []string{"entryUUID"}, 2)
			if err != nil {
				return err
			}
			if len(result.Entries) != 1 || result.Entries[0] == nil {
				return errors.New("subject-link provider requires exactly one immutable subject")
			}
			entry := result.Entries[0]
			if len(entry.Attributes) != 1 || entry.Attributes[0] == nil || !strings.EqualFold(entry.Attributes[0].Name, "entryUUID") ||
				len(entry.Attributes[0].Values) != 1 || entry.Attributes[0].Values[0] != proposal.Target.SubjectID {
				return errors.New("subject-link provider lacks exact immutable UUID evidence")
			}
			return nil
		}, nil
	}
	return nil, errors.New("subject-link provider is not configured for this exact registry source/version")
}
