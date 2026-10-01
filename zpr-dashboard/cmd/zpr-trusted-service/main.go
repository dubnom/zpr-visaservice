package main

import (
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
	"sort"
	"strings"
	"time"

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
	Attributes map[string][]string `json:"attributes"`
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

func (p fileProvider) lookup(_ context.Context, identities []identity) (map[string][]string, error) {
	file, err := os.Open(p.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var records map[string]map[string]map[string][]string
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	if err := decoder.Decode(&records); err != nil {
		return nil, err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("invalid attribute file: trailing data or oversized snapshot")
	}
	var tail [1]byte
	if n, err := file.Read(tail[:]); err != io.EOF || n != 0 {
		return nil, errors.New("attribute file exceeds 1 MiB")
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

func (p ldapProvider) lookup(ctx context.Context, identities []identity) (map[string][]string, error) {
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
	defer conn.Close()
	conn.SetTimeout(5 * time.Second)
	if err := conn.Bind(p.bindDN, p.password); err != nil {
		return nil, err
	}
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

func ldapIdentityAttributes(ctx context.Context, searcher ldapSearcher, peopleBaseDN, groupsBaseDN, identityAttribute, identityValue string, attributes []string) (map[string][]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	peopleSearch := ldap.NewSearchRequest(peopleBaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 100, 5, false, ldapFilter(identityAttribute, identityValue), attributes, nil)
	people, err := searcher.Search(peopleSearch)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string)
	for _, person := range people.Entries {
		values := make(map[string][]string)
		for _, name := range attributes {
			if attribute := person.GetAttributeValues(name); len(attribute) > 0 {
				values[name] = attribute
			}
		}
		if groupsBaseDN != "" {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			groupsSearch := ldap.NewSearchRequest(groupsBaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 100, 5, false, ldapFilter("member", person.DN), []string{"cn"}, nil)
			groups, err := searcher.Search(groupsSearch)
			if err != nil {
				return nil, err
			}
			roles := make([]string, 0, len(groups.Entries))
			for _, group := range groups.Entries {
				if name := group.GetAttributeValue("cn"); name != "" {
					roles = append(roles, name)
				}
			}
			sort.Strings(roles)
			if len(roles) > 0 {
				values["role"] = roles
			}
		}
		if err := merge(result, values); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func handler(store provider) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/attributes", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "expected application/json", http.StatusUnsupportedMediaType)
			return
		}
		var request lookupRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil || len(request.Identities) > 64 {
			http.Error(w, "invalid lookup request", http.StatusBadRequest)
			return
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			http.Error(w, "invalid lookup request", http.StatusBadRequest)
			return
		}
		for _, ident := range request.Identities {
			if ident.Key == "" || ident.Value == "" {
				http.Error(w, "empty identity", http.StatusBadRequest)
				return
			}
		}
		attributes, err := store.lookup(r.Context(), request.Identities)
		if err != nil {
			log.Printf("trusted service lookup failed: %T", err)
			http.Error(w, "lookup unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(lookupResponse{Attributes: attributes})
	})
	return mux
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8443", "HTTPS listen address")
	cert := flag.String("cert", "", "server certificate PEM")
	key := flag.String("key", "", "server private key PEM")
	clientCA := flag.String("client-ca", "", "CA for Visa Service client certificates")
	file := flag.String("file", "", "file-backed attribute JSON path")
	ldapURI := flag.String("ldap-uri", "", "LDAPS URI (alternative to -file)")
	ldapCA := flag.String("ldap-ca", "", "LDAP server CA PEM")
	ldapBase := flag.String("ldap-base", "", "LDAP search base DN")
	ldapBind := flag.String("ldap-bind", "", "LDAP bind DN")
	ldapPasswordFile := flag.String("ldap-password-file", "", "path to LDAP bind password")
	ldapIdentities := flag.String("ldap-identities", "", "JSON map of ZPR identity key to LDAP search attribute")
	ldapAttributes := flag.String("ldap-attributes", "", "comma-separated LDAP attributes to return")
	ldapGroupsBase := flag.String("ldap-groups-base", "", "optional LDAP group search base DN for groupOfNames role membership")
	flag.Parse()
	if *cert == "" || *key == "" || *clientCA == "" || (*file == "") == (*ldapURI == "") {
		log.Fatal("require -cert, -key, -client-ca, and exactly one of -file or -ldap-uri")
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
	if *file != "" {
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
		store = ldapProvider{
			uri: *ldapURI, baseDN: *ldapBase, bindDN: *ldapBind, password: strings.TrimSuffix(string(password), "\n"),
			ca: roots, identityKeys: keys, attributes: attributes, groupsBaseDN: *ldapGroupsBase,
		}
	}
	server := http.Server{Addr: *listen, Handler: handler(store), ReadHeaderTimeout: 5 * time.Second, TLSConfig: &tls.Config{
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
