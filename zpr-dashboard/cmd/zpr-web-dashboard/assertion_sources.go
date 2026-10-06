package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

type assertionNamedSource struct {
	kind string
	read func(context.Context, string, string, string) (assertionDirectory, error)
}

type assertionSourceScope struct {
	BaseDN string `json:"base_dn,omitempty"`
	BindDN string `json:"bind_dn,omitempty"`
	URL    string `json:"url,omitempty"`
}

type assertionSourceConfig struct {
	Name          string                          `json:"name"`
	Kind          string                          `json:"kind"`
	Container     string                          `json:"container,omitempty"`
	URL           string                          `json:"url,omitempty"`
	CAFile        string                          `json:"ca_file,omitempty"`
	TokenFile     string                          `json:"token_file,omitempty"`
	Attributes    []string                        `json:"attributes,omitempty"`
	Organizations map[string]assertionSourceScope `json:"organizations,omitempty"`
}

var assertionSourceName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)

func (runtime *assertionRuntime) configureSources(filePath string) error {
	if filePath == "" {
		return nil
	}
	file, err := os.Open(filePath)
	if err != nil {
		return errors.New("Cannot open assertion source configuration")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return errors.New("Assertion source configuration exceeds limits")
	}
	var config struct {
		DefaultSource string                  `json:"default_source"`
		Sources       []assertionSourceConfig `json:"sources"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil || decoder.Decode(new(any)) != io.EOF || len(config.Sources) == 0 || len(config.Sources) > 16 {
		return errors.New("Invalid assertion source configuration; configure 1 to 16 sources")
	}
	sources := make(map[string]assertionNamedSource)
	if runtime.reader != nil {
		sources["ldap"] = assertionNamedSource{kind: "ldap", read: func(ctx context.Context, organizationID, baseDN, bindDN string) (assertionDirectory, error) {
			return runtime.reader(ctx, baseDN, bindDN)
		}}
	}
	for _, entry := range config.Sources {
		if !assertionSourceName.MatchString(entry.Name) || sources[entry.Name].read != nil {
			return errors.New("Assertion source names must be unique identifiers")
		}
		attributes, err := approvedAssertionAttributes(strings.Join(entry.Attributes, ","))
		if err != nil {
			return err
		}
		var read func(context.Context, string, string, string) (assertionDirectory, error)
		switch entry.Kind {
		case "ldap":
			if entry.Container == "" || entry.URL != "" || entry.TokenFile != "" || entry.CAFile != "" {
				return errors.New("LDAP assertion sources require a container and cannot contain HTTPS configuration")
			}
			read = func(ctx context.Context, organizationID, baseDN, bindDN string) (assertionDirectory, error) {
				if scope, exists := entry.Organizations[organizationID]; exists {
					baseDN, bindDN = scope.BaseDN, scope.BindDN
				} else if len(entry.Organizations) > 0 {
					return assertionDirectory{}, errors.New("Trusted source is not configured for this organization")
				}
				if baseDN == "" || bindDN == "" {
					return assertionDirectory{}, errors.New("Trusted source organization context is unavailable")
				}
				return readAssertionLDAP(ctx, entry.Container, bindDN, baseDN, attributes)
			}
		case "https-json":
			if entry.Container != "" || entry.TokenFile == "" || entry.URL == "" && len(entry.Organizations) == 0 {
				return errors.New("HTTPS assertion sources require an organization URL and a token file")
			}
			if entry.URL != "" && (!strings.Contains(entry.URL, "{organization}") || !validAssertionSourceURL(strings.ReplaceAll(entry.URL, "{organization}", "organization"))) {
				return errors.New("HTTPS source URL must be HTTPS and contain {organization}")
			}
			for _, scope := range entry.Organizations {
				if !validAssertionSourceURL(scope.URL) {
					return errors.New("HTTPS organization source URL is invalid")
				}
			}
			roots, err := x509.SystemCertPool()
			if err != nil {
				return errors.New("Cannot load trusted source CA certificates")
			}
			if entry.CAFile != "" {
				data, err := os.ReadFile(entry.CAFile)
				if err != nil || !roots.AppendCertsFromPEM(data) {
					return errors.New("Trusted source CA file is invalid")
				}
			}
			client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}, CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("Trusted source redirects are not allowed")
			}}
			read = func(ctx context.Context, organizationID, baseDN, bindDN string) (assertionDirectory, error) {
				endpoint := strings.ReplaceAll(entry.URL, "{organization}", url.PathEscape(organizationID))
				if scope, exists := entry.Organizations[organizationID]; exists {
					endpoint = scope.URL
				} else if len(entry.Organizations) > 0 {
					return assertionDirectory{}, errors.New("Trusted source is not configured for this organization")
				}
				if organizationID == "" || !validAssertionSourceURL(endpoint) {
					return assertionDirectory{}, errors.New("Trusted source organization context is unavailable")
				}
				return readAssertionHTTPS(ctx, client, endpoint, entry.TokenFile, attributes)
			}
		default:
			return errors.New("Assertion source kind must be ldap or https-json")
		}
		sources[entry.Name] = assertionNamedSource{kind: entry.Kind, read: read}
	}
	if len(sources) > 16 || sources[config.DefaultSource].read == nil {
		return errors.New("Default assertion source must name a configured source; at most 16 sources are allowed")
	}
	runtime.sources, runtime.defaultSource = sources, config.DefaultSource
	return nil
}

func validAssertionSourceURL(endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func readAssertionHTTPS(ctx context.Context, client *http.Client, endpoint, tokenFile string, attributes []string) (assertionDirectory, error) {
	file, err := os.Open(tokenFile)
	if err != nil {
		return assertionDirectory{}, errors.New("Trusted source credential is unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	_ = file.Close()
	token := strings.TrimSpace(string(data))
	if err != nil || len(data) > 4096 || token == "" || strings.ContainsAny(token, "\r\n") {
		return assertionDirectory{}, errors.New("Trusted source credential is invalid")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return assertionDirectory{}, errors.New("Trusted source URL is invalid")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return assertionDirectory{}, errors.New("Trusted HTTPS source read failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return assertionDirectory{}, errors.New("Trusted HTTPS source returned an unsuccessful response")
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return assertionDirectory{}, errors.New("Trusted source snapshot exceeds limits")
	}
	var directory assertionDirectory
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&directory); err != nil || decoder.Decode(new(any)) != io.EOF {
		return assertionDirectory{}, errors.New("Trusted source snapshot is malformed")
	}
	return validateAssertionDirectory(directory, attributes)
}

func validateAssertionDirectory(directory assertionDirectory, attributes []string) (assertionDirectory, error) {
	if directory.People == nil || directory.Groups == nil || len(directory.People)+len(directory.Groups) > 20000 {
		return assertionDirectory{}, errors.New("Trusted source must provide complete people and groups within entry limits")
	}
	people := make(map[string]bool)
	for _, person := range directory.People {
		if strings.TrimSpace(person) == "" || len(person) > 200 || people[person] {
			return assertionDirectory{}, errors.New("Trusted source contains invalid or duplicate people")
		}
		people[person] = true
	}
	for group, members := range directory.Groups {
		if strings.TrimSpace(group) == "" || len(group) > 200 {
			return assertionDirectory{}, errors.New("Trusted source contains an invalid group")
		}
		seen := make(map[string]bool)
		for _, member := range members {
			if !people[member] || seen[member] {
				return assertionDirectory{}, errors.New("Trusted source contains unresolved or duplicate group members")
			}
			seen[member] = true
		}
	}
	approved := make(map[string]bool)
	for _, name := range attributes {
		approved[name] = true
	}
	for _, set := range []struct {
		values map[string]map[string][]string
		group  bool
	}{{directory.PersonAttributes, false}, {directory.GroupAttributes, true}} {
		for subject, values := range set.values {
			_, groupExists := directory.Groups[subject]
			if set.group && !groupExists || !set.group && !people[subject] {
				return assertionDirectory{}, errors.New("Trusted source attributes refer to an unknown subject")
			}
			for name, entries := range values {
				if !approved[name] || len(entries) > 256 {
					return assertionDirectory{}, errors.New("Trusted source contains excluded attributes or too many values")
				}
				unique := make(map[string]bool)
				clean := make([]string, 0, len(entries))
				for _, value := range entries {
					if len(value) > 4096 {
						return assertionDirectory{}, errors.New("Trusted source attribute value exceeds limits")
					}
					if strings.TrimSpace(value) != "" && !unique[value] {
						unique[value] = true
						clean = append(clean, value)
					}
				}
				values[name] = clean
			}
		}
	}
	directory.Attributes = append([]string(nil), attributes...)
	sort.Strings(directory.People)
	return directory, nil
}

func (runtime *assertionRuntime) readDirectory(ctx context.Context, organizationID, baseDN, bindDN string) (assertionDirectory, error) {
	if len(runtime.sources) == 0 {
		return runtime.reader(ctx, baseDN, bindDN)
	}
	names := make([]string, 0, len(runtime.sources))
	for name := range runtime.sources {
		names = append(names, name)
	}
	sort.Strings(names)
	snapshots := make(map[string]assertionDirectory)
	for _, name := range names {
		directory, err := runtime.sources[name].read(ctx, organizationID, baseDN, bindDN)
		if err != nil {
			return assertionDirectory{}, fmt.Errorf("Trusted source %s is unavailable; no assertions were evaluated", name)
		}
		snapshots[name] = directory
	}
	directory := snapshots[runtime.defaultSource]
	directory.Sources = snapshots
	return directory, nil
}

func assertionSourceSummaries(directory assertionDirectory, observed time.Time) []map[string]any {
	names := make([]string, 0, len(directory.Sources))
	for name := range directory.Sources {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]map[string]any, 0, len(names))
	for _, name := range names {
		summary := assertionSourceSummary(directory.Sources[name], observed)
		summary["name"] = name
		result = append(result, summary)
	}
	return result
}

func (runtime *assertionRuntime) sourceMetadata() []map[string]string {
	names := make([]string, 0, len(runtime.sources))
	for name := range runtime.sources {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]map[string]string, 0, len(names))
	for _, name := range names {
		result = append(result, map[string]string{"name": name, "kind": runtime.sources[name].kind})
	}
	if len(result) == 0 && runtime.reader != nil {
		result = append(result, map[string]string{"name": "ldap", "kind": "ldap"})
	}
	return result
}
