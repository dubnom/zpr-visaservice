package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/go-ldap/ldap/v3"
)

type identity struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type lookupRequest struct {
	Identities []identity `json:"identities"`
}

type lookupResponse struct {
	Attributes  map[string][]string `json:"attributes"`
	ExpiresUnix int64               `json:"expires_unix,omitempty"`
}

type provider interface {
	lookup(context.Context, []identity) (map[string][]string, error)
}

func merge(target map[string][]string, attributes map[string][]string) error {
	for name, values := range attributes {
		if existing, ok := target[name]; ok {
			if len(existing) != len(values) {
				return fmt.Errorf("conflicting values for %s", name)
			}
			for index := range values {
				if existing[index] != values[index] {
					return fmt.Errorf("conflicting values for %s", name)
				}
			}
		} else {
			target[name] = values
		}
	}
	return nil
}

type fileProvider struct{ path string }

// Neither the lookup request nor the file snapshot contract permits JSON null.
func decodeLookupJSON(data []byte, target any) error {
	if !utf8.Valid(data) {
		return errors.New("lookup JSON is not valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := validateLookupJSONValue(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("invalid lookup JSON: trailing data")
	}
	if err := validateLookupJSONEscapes(data); err != nil {
		return err
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// Go's JSON decoder replaces lone UTF-16 surrogates instead of rejecting them.
// Syntax has already been checked; skip escaped backslashes and require pairs.
func validateLookupJSONEscapes(data []byte) error {
	for i := 0; i < len(data); i++ {
		if data[i] != '\\' {
			continue
		}
		i++
		if data[i] != 'u' {
			continue
		}
		code, err := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		if err != nil {
			return err
		}
		i += 4
		if code >= 0xdc00 && code <= 0xdfff {
			return errors.New("unpaired Unicode surrogate in lookup JSON")
		}
		if code >= 0xd800 && code <= 0xdbff {
			if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
				return errors.New("unpaired Unicode surrogate in lookup JSON")
			}
			low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return errors.New("unpaired Unicode surrogate in lookup JSON")
			}
			i += 6
		}
	}
	return nil
}

func decodeLookupRequest(data []byte) (lookupRequest, error) {
	var fields map[string]json.RawMessage
	if err := decodeLookupJSON(data, &fields); err != nil {
		return lookupRequest{}, err
	}
	raw, ok := fields["identities"]
	if !ok || len(fields) != 1 {
		return lookupRequest{}, errors.New("expected identities field")
	}
	var entries []map[string]string
	if err := json.Unmarshal(raw, &entries); err != nil {
		return lookupRequest{}, err
	}
	request := lookupRequest{Identities: make([]identity, 0, len(entries))}
	for _, fields := range entries {
		key, hasKey := fields["key"]
		value, hasValue := fields["value"]
		if !hasKey || !hasValue || len(fields) != 2 {
			return lookupRequest{}, errors.New("expected key and value identity fields")
		}
		request.Identities = append(request.Identities, identity{key, value})
	}
	return request, nil
}

func validateLookupJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("lookup JSON nesting exceeds limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return errors.New("null is not permitted in lookup JSON")
	}
	switch token {
	case json.Delim('{'):
		seen := make(map[string]bool)
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := token.(string)
			if !ok || seen[name] {
				return errors.New("duplicate or invalid lookup JSON name")
			}
			seen[name] = true
			if err := validateLookupJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		token, err = decoder.Token()
		if err != nil || token != json.Delim('}') {
			return errors.New("invalid lookup JSON object")
		}
	case json.Delim('['):
		for decoder.More() {
			if err := validateLookupJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		token, err = decoder.Token()
		if err != nil || token != json.Delim(']') {
			return errors.New("invalid lookup JSON array")
		}
	}
	return nil
}

func (p fileProvider) lookup(_ context.Context, identities []identity) (map[string][]string, error) {
	file, err := os.Open(p.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("attribute file exceeds 1 MiB")
	}
	var records map[string]map[string]map[string][]string
	if err := decodeLookupJSON(data, &records); err != nil {
		return nil, err
	}
	attributes := make(map[string][]string)
	for _, ident := range identities {
		if err := merge(attributes, records[ident.Key][ident.Value]); err != nil {
			return nil, err
		}
	}
	return attributes, nil
}

type ldapProvider struct {
	uri, baseDN, bindDN, password string
	passwordFile                  string
	ca                            *x509.CertPool
	identityKeys                  map[string]string
	attributes                    []string
	groupsBaseDN                  string
}

type ldapSearcher interface {
	Search(*ldap.SearchRequest) (*ldap.SearchResult, error)
}

type ldapSearcherFunc func(*ldap.SearchRequest) (*ldap.SearchResult, error)

func (searcher ldapSearcherFunc) Search(request *ldap.SearchRequest) (*ldap.SearchResult, error) {
	return searcher(request)
}

func (p ldapProvider) dial() (*ldap.Conn, error) {
	uri, err := url.Parse(p.uri)
	if err != nil || uri.Scheme != "ldaps" || uri.Hostname() == "" || uri.User != nil {
		return nil, errors.New("LDAP URI must be ldaps:// with a hostname")
	}
	conn, err := ldap.DialURL(p.uri, ldap.DialWithTLSConfig(&tls.Config{
		RootCAs: p.ca, ServerName: uri.Hostname(), MinVersion: tls.VersionTLS12,
	}), ldap.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}))
	if err != nil {
		return nil, err
	}
	conn.SetTimeout(5 * time.Second)
	password := p.password
	if p.passwordFile != "" {
		var err error
		password, err = sourcePassword(p.passwordFile)
		if err != nil {
			conn.Close()
			return nil, err
		}
	}
	if err := conn.Bind(p.bindDN, password); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (p ldapProvider) lookup(ctx context.Context, identities []identity) (map[string][]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := p.dial()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	result := make(map[string][]string)
	for _, ident := range identities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key, ok := p.identityKeys[ident.Key]
		if !ok {
			continue
		}
		attributes, err := ldapIdentityAttributes(ctx, conn, p.baseDN, p.groupsBaseDN, key, ident.Value, p.attributes)
		if err != nil {
			return nil, err
		}
		if err := merge(result, attributes); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func lookupLDAPSearch(ctx context.Context, searcher ldapSearcher, request *ldap.SearchRequest) (*ldap.SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result, err := searcher.Search(request)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if result == nil || len(result.Referrals) != 0 {
		return nil, errors.New("LDAP lookup returned an incomplete result")
	}
	return result, nil
}

func lookupLDAPAttribute(entry *ldap.Entry, name string) ([]string, error) {
	if entry == nil {
		return nil, errors.New("LDAP lookup returned an invalid entry")
	}
	var values []string
	found := false
	for _, attribute := range entry.Attributes {
		if attribute == nil {
			return nil, errors.New("LDAP lookup returned an invalid attribute")
		}
		if strings.EqualFold(attribute.Name, name) {
			if found {
				return nil, fmt.Errorf("LDAP lookup returned duplicate %s attributes", name)
			}
			values, found = attribute.Values, true
		}
	}
	return values, nil
}

func ldapIdentityAttributes(ctx context.Context, searcher ldapSearcher, peopleBaseDN, groupsBaseDN, identityAttribute, identityValue string, attributes []string) (map[string][]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	peopleSearch := ldap.NewSearchRequest(peopleBaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 100, 5, false, ldapFilter(identityAttribute, identityValue), attributes, nil)
	people, err := lookupLDAPSearch(ctx, searcher, peopleSearch)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string)
	for _, person := range people.Entries {
		if person == nil || (groupsBaseDN != "" && person.DN == "") {
			return nil, errors.New("LDAP lookup returned an invalid person entry")
		}
		values := make(map[string][]string)
		for _, name := range attributes {
			attribute, err := lookupLDAPAttribute(person, name)
			if err != nil {
				return nil, err
			}
			if len(attribute) > 0 {
				values[name] = attribute
			}
		}
		if groupsBaseDN != "" {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			groupsSearch := ldap.NewSearchRequest(groupsBaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 100, 5, false, ldapFilter("member", person.DN), []string{"cn"}, nil)
			groups, err := lookupLDAPSearch(ctx, searcher, groupsSearch)
			if err != nil {
				return nil, err
			}
			roles := make([]string, 0, len(groups.Entries))
			for _, group := range groups.Entries {
				names, err := lookupLDAPAttribute(group, "cn")
				if err != nil {
					return nil, err
				}
				if len(names) != 1 || names[0] == "" {
					return nil, errors.New("LDAP group lookup requires exactly one nonempty cn")
				}
				roles = append(roles, names[0])
			}
			sort.Strings(roles)
			if len(roles) > 0 {
				if err := merge(values, map[string][]string{"role": roles}); err != nil {
					return nil, err
				}
			}
		}
		if err := merge(result, values); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func handler(store provider, changes changeSource) http.Handler {
	return handlerWithValidity(store, changes, 0)
}

func encodeLookupResponse(attributes map[string][]string, expiresUnix int64) ([]byte, error) {
	if attributes == nil {
		return nil, errors.New("provider returned null attributes")
	}
	for name, values := range attributes {
		if !utf8.ValidString(name) || values == nil {
			return nil, errors.New("provider returned invalid attribute data")
		}
		for _, value := range values {
			if !utf8.ValidString(value) {
				return nil, errors.New("provider returned invalid attribute text")
			}
		}
	}
	body, err := json.Marshal(lookupResponse{Attributes: attributes, ExpiresUnix: expiresUnix})
	if err != nil {
		return nil, err
	}
	// Include the existing trailing newline in the VS consumer's 1 MiB wire bound.
	if len(body) >= 1<<20 {
		return nil, errors.New("lookup response exceeds 1 MiB")
	}
	return append(body, '\n'), nil
}

func handlerWithValidity(store provider, changes changeSource, validity time.Duration) http.Handler {
	mux := http.NewServeMux()
	if changes != nil {
		mux.Handle("GET /v1/changes", changesHandler(changes))
	}
	mux.HandleFunc("POST /v1/attributes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "expected application/json", http.StatusUnsupportedMediaType)
			return
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
		if err != nil {
			http.Error(w, "invalid lookup request", http.StatusBadRequest)
			return
		}
		request, err := decodeLookupRequest(data)
		if err != nil || len(request.Identities) > 64 {
			http.Error(w, "invalid lookup request", http.StatusBadRequest)
			return
		}
		for _, ident := range request.Identities {
			if ident.Key == "" || ident.Value == "" {
				http.Error(w, "empty identity", http.StatusBadRequest)
				return
			}
		}
		var expiresUnix int64
		if validity > 0 {
			expiresUnix = time.Now().Add(validity).Unix()
		}
		if err := r.Context().Err(); err != nil {
			log.Printf("trusted service lookup canceled before provider call: %T", err)
			http.Error(w, "lookup canceled", http.StatusServiceUnavailable)
			return
		}
		attributes, err := store.lookup(r.Context(), request.Identities)
		if err != nil {
			log.Printf("trusted service lookup failed: %T", err)
			http.Error(w, "lookup unavailable", http.StatusServiceUnavailable)
			return
		}
		body, err := encodeLookupResponse(attributes, expiresUnix)
		if err != nil {
			log.Printf("trusted service response rejected: %v", err)
			http.Error(w, "lookup unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := r.Context().Err(); err != nil {
			log.Printf("trusted service lookup canceled before publication: %T", err)
			http.Error(w, "lookup canceled", http.StatusServiceUnavailable)
			return
		}
		if expiresUnix != 0 && time.Now().Unix() >= expiresUnix {
			log.Printf("trusted service lookup exceeded its validity bound")
			http.Error(w, "lookup validity expired", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(body); err != nil {
			log.Printf("trusted service response write failed: %T", err)
		}
	})
	return mux
}

func writeChangesJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func logChangeFailure(err error) {
	log.Printf("trusted service change query failed: %T", err)
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8443", "HTTPS listen address")
	cert := flag.String("cert", "", "server certificate PEM")
	key := flag.String("key", "", "server private key PEM")
	clientCA := flag.String("client-ca", "", "CA for Visa Service client certificates")
	file := flag.String("file", "", "file-backed attribute JSON path")
	sourceConfigFile := flag.String("source-config", "", "protected provisional JSON configuration for one immutable LDAP source")
	attributeValidity := flag.Duration("attribute-validity", 0, "optional lookup validity bound (at least 1s); requires an expires_unix-aware VS")
	ldapURI := flag.String("ldap-uri", "", "LDAPS URI (alternative to -file)")
	ldapCA := flag.String("ldap-ca", "", "LDAP server CA PEM")
	ldapBase := flag.String("ldap-base", "", "LDAP search base DN")
	ldapBind := flag.String("ldap-bind", "", "LDAP bind DN")
	ldapPasswordFile := flag.String("ldap-password-file", "", "path to LDAP bind password")
	ldapIdentities := flag.String("ldap-identities", "", "JSON map of ZPR identity key to LDAP search attribute")
	ldapAttributes := flag.String("ldap-attributes", "", "comma-separated LDAP attributes to return")
	ldapGroupsBase := flag.String("ldap-groups-base", "", "optional LDAP group search base DN for groupOfNames role membership")
	ldapChangesBase := flag.String("ldap-changes-base", "", "optional OpenLDAP accesslog suffix (for example cn=accesslog) that enables GET /v1/changes")
	ldapChangesRetention := flag.Duration("ldap-changes-retention", 24*time.Hour, "age of changes guaranteed retained by the accesslog logpurge setting; older cursors expire")
	ldapChangesSettle := flag.Duration("ldap-changes-settle", 2*time.Second, "delay before a logged write becomes visible to pollers")
	ldapWatch := flag.Bool("ldap-watch", false, "consume LDAP sync changes as metadata-only JSON lines on stdout instead of serving HTTPS")
	flag.Parse()
	if *sourceConfigFile != "" && (*file != "" || *ldapURI != "" || *ldapWatch || *ldapChangesBase != "" ||
		*ldapCA != "" || *ldapBase != "" || *ldapBind != "" || *ldapPasswordFile != "" || *ldapIdentities != "" || *ldapAttributes != "" || *ldapGroupsBase != "") {
		log.Fatal("-source-config cannot be combined with legacy file/LDAP/watch options")
	}
	if *ldapWatch {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		if err := watchLDAP(ctx, *ldapURI, *ldapCA, *ldapBase, *ldapBind, *ldapPasswordFile, os.Stdout); err != nil {
			log.Fatal("LDAP change consumer stopped: ", err)
		}
		return
	}
	if *attributeValidity < 0 || (*attributeValidity > 0 && *attributeValidity < time.Second) {
		log.Fatal("-attribute-validity must be zero or at least 1s")
	}
	modes := 0
	for _, mode := range []string{*file, *ldapURI, *sourceConfigFile} {
		if mode != "" {
			modes++
		}
	}
	if *cert == "" || *key == "" || *clientCA == "" || modes != 1 {
		log.Fatal("require -cert, -key, -client-ca, and exactly one of -file, -ldap-uri or -source-config")
	}
	caPEM, err := os.ReadFile(*clientCA)
	if err != nil {
		log.Fatal(err)
	}
	clients := x509.NewCertPool()
	if !clients.AppendCertsFromPEM(caPEM) {
		log.Fatal("invalid client CA")
	}
	var store provider
	var changes changeSource
	if *sourceConfigFile != "" {
		configured, err := configuredSource(*sourceConfigFile)
		if err != nil {
			log.Fatal("trusted source configuration rejected: ", err)
		}
		store = configured
		log.Printf("trusted source %s configured with immutable LDAP selector", configured.config.SourceID)
	} else if *file != "" {
		if *ldapChangesBase != "" {
			log.Fatal("-ldap-changes-base requires -ldap-uri")
		}
		store = fileProvider{*file}
	} else {
		if *ldapCA == "" || *ldapBase == "" || *ldapPasswordFile == "" || *ldapIdentities == "" || *ldapAttributes == "" {
			log.Fatal("incomplete LDAP configuration")
		}
		parsed, err := url.Parse(*ldapURI)
		if err != nil || parsed.Scheme != "ldaps" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" {
			log.Fatal("LDAP URI must be ldaps://host")
		}
		pem, err := os.ReadFile(*ldapCA)
		if err != nil {
			log.Fatal(err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			log.Fatal("invalid LDAP CA")
		}
		password, err := os.ReadFile(*ldapPasswordFile)
		if err != nil {
			log.Fatal(err)
		}
		var keys map[string]string
		if err := json.Unmarshal([]byte(*ldapIdentities), &keys); err != nil || len(keys) == 0 {
			log.Fatal("invalid LDAP identity mappings")
		}
		attributes := strings.Split(*ldapAttributes, ",")
		for _, name := range append(attributes, mapValues(keys)...) {
			if !safeLDAPAttribute(name) {
				log.Fatal("invalid LDAP attribute name")
			}
		}
		ldapStore := ldapProvider{
			uri: *ldapURI, baseDN: *ldapBase, bindDN: *ldapBind, password: strings.TrimSuffix(string(password), "\n"),
			ca: roots, identityKeys: keys, attributes: attributes, groupsBaseDN: *ldapGroupsBase,
		}
		store = ldapStore
		if *ldapChangesBase != "" {
			if _, err := ldap.ParseDN(*ldapChangesBase); err != nil {
				log.Fatal("invalid -ldap-changes-base")
			}
			if *ldapChangesRetention <= 0 || *ldapChangesSettle < 0 {
				log.Fatal("invalid LDAP change retention or settle delay")
			}
			changes = accessLogChanges{
				logBase: *ldapChangesBase, retention: *ldapChangesRetention, settle: *ldapChangesSettle, now: time.Now,
				connect: func(context.Context) (ldapSearcher, func(), error) {
					conn, err := ldapStore.dial()
					if err != nil {
						return nil, nil, err
					}
					return conn, func() { conn.Close() }, nil
				},
			}
		}
	}
	server := http.Server{Addr: *listen, Handler: handlerWithValidity(store, changes, *attributeValidity), ReadHeaderTimeout: 5 * time.Second, TLSConfig: &tls.Config{
		MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clients,
	}}
	log.Printf("trusted service listening at %s", *listen)
	log.Fatal(server.ListenAndServeTLS(*cert, *key))
}

func mapValues(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func safeLDAPAttribute(name string) bool {
	if name == "" {
		return false
	}
	for _, char := range name {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
			return false
		}
	}
	return true
}

func ldapFilter(attribute, value string) string {
	return fmt.Sprintf("(%s=%s)", attribute, ldap.EscapeFilter(value))
}
