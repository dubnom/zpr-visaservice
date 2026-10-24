// zpr-simulator-companion is an optional ordinary client, never an operator dependency.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type inventory struct {
	Organization  string `json:"organization_id"`
	Source        string `json:"source_id"`
	StarterSource string `json:"starter_source_id"`
	Port          int    `json:"port"`
}
type registration struct {
	Organization string         `json:"organization_id"`
	Source       string         `json:"source_id"`
	Revision     string         `json:"revision"`
	Config       map[string]any `json:"config"`
}
type registry struct {
	Organization string `json:"organization_id"`
	Revision     string `json:"revision"`
	Durable      bool   `json:"durable"`
	Sources      []struct {
		Source   string         `json:"source_id"`
		Editable bool           `json:"editable"`
		Config   map[string]any `json:"config"`
	} `json:"sources"`
}
type contract struct {
	client   *http.Client
	endpoint string
}

var identity = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	action := flag.String("action", "", "init, serve, or remove")
	mode := flag.String("mode", "", "must explicitly be ordinary-client")
	state := flag.String("state", "", "separate protected companion directory (not bootstrap state)")
	source := flag.String("source", "", "unique companion source ID")
	starterSource := flag.String("starter-source", "", "starter source ID (collision guard)")
	port := flag.Int("port", 19446, "loopback synthetic HTTPS directory port")
	endpoint := flag.String("endpoint", "", "authenticated production Control-Service HTTPS endpoint")
	ca := flag.String("api-ca", "", "production API CA PEM")
	cert := flag.String("client-cert", "", "ordinary setup client's mTLS certificate PEM")
	key := flag.String("client-key", "", "ordinary setup client's mTLS key PEM")
	confirm := flag.String("confirm-source", "", "required exact source ID for explicit removal")
	flag.Parse()
	if *mode != "ordinary-client" || flag.NArg() != 0 || *state == "" {
		return errors.New("explicit -mode ordinary-client and separate -state are required; no legacy fallback")
	}
	absolute, err := filepath.Abs(*state)
	if err != nil {
		return err
	}
	if *action != "init" && *action != "serve" && *action != "remove" {
		return errors.New("action must be init, serve, or remove")
	}
	api, err := newContract(*endpoint, *ca, *cert, *key)
	if err != nil {
		return err
	}
	defer api.client.CloseIdleConnections()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *action == "init" {
		return initializeFromContract(ctx, api, absolute, *source, *starterSource, *port)
	}
	inv, err := load(absolute)
	if err != nil {
		return err
	}
	if *action == "remove" {
		if *confirm != inv.Source {
			return errors.New("removal requires -confirm-source with this companion's exact source ID")
		}
		if err := reconcile(ctx, api, absolute, inv, true); err != nil {
			return err
		}
		fmt.Println("Only the companion source was unregistered. Stop its foreground server; private files are retained.")
		return nil
	}
	// Preflight the ordinary contract before opening the synthetic service.
	if _, _, err := inspect(ctx, api, absolute, inv); err != nil {
		return err
	}
	server, listener, err := directoryServer(absolute, inv)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer server.Close()
	served := make(chan error, 1)
	go func() {
		served <- server.ServeTLS(listener, filepath.Join(absolute, "source-ca.pem"), filepath.Join(absolute, "source-key.pem"))
	}()
	if err := reconcile(ctx, api, absolute, inv, false); err != nil {
		return err
	}
	fmt.Println("EVALUATION ONLY: companion registered through ordinary authenticated source contracts. No starter LDAP or production policy was changed.")
	fmt.Println("Stopping leaves registration intact (source unavailable, no fallback). Use explicit remove before deleting private state.")
	select {
	case <-ctx.Done():
		return nil
	case err := <-served:
		return err
	}
}
func validate(inv inventory) error {
	if !identity.MatchString(inv.Organization) || !identity.MatchString(inv.Source) || !identity.MatchString(inv.StarterSource) || inv.Source == inv.StarterSource || inv.Port < 1024 || inv.Port > 65535 {
		return errors.New("companion requires a valid runtime organization, a distinct valid source ID and an unprivileged port")
	}
	return nil
}
func write(path string, value []byte) error { return os.WriteFile(path, value, 0600) }
func initializeFromContract(ctx context.Context, api contract, state, source, starter string, port int) error {
	var current registry
	if err := api.call(ctx, http.MethodGet, "/api/assertions/sources", nil, &current); err != nil {
		return err
	}
	if !current.Durable || current.Revision == "" || !identity.MatchString(current.Organization) {
		return errors.New("durable public source contract and authorized runtime organization required; nothing provisioned")
	}
	foundStarter := false
	for _, entry := range current.Sources {
		if entry.Source == source {
			return errors.New("source ID already registered; choose a separate stable synthetic source ID")
		}
		if entry.Source == starter {
			foundStarter = true
		}
	}
	if !foundStarter {
		return errors.New("explicit starter source is not registered in this runtime organization; nothing provisioned")
	}
	return initialize(state, inventory{Organization: current.Organization, Source: source, StarterSource: starter, Port: port})
}
func initialize(state string, inv inventory) (err error) {
	if err = validate(inv); err != nil {
		return err
	}
	// Never accept/reseed an existing directory, including bootstrap state.
	if err = os.Mkdir(state, 0700); err != nil {
		return errors.New("init requires a new separate state directory; existing state is never reseeded")
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(state)
		}
	}()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Synthetic companion EVALUATION ONLY"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(30 * 24 * time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return err
	}
	files := map[string][]byte{
		"source-ca.pem":       pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		"source-key.pem":      pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
		"source-token.secret": []byte(hex.EncodeToString(secret)),
		"directory.json":      []byte(`{"people":["synthetic-companion-user"],"groups":{"synthetic-companion-users":["synthetic-companion-user"]},"person_attributes":{"synthetic-companion-user":{"uid":["synthetic-companion-user"]}}}`),
	}
	files["companion.json"], err = json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return err
	}
	for name, data := range files {
		if err = write(filepath.Join(state, name), data); err != nil {
			return err
		}
	}
	fmt.Println("Created separate synthetic directory state; nothing was registered or written to starter LDAP.")
	return nil
}
func protected(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("required protected regular companion file unavailable")
	}
	return os.ReadFile(path)
}
func load(state string) (inventory, error) {
	var inv inventory
	info, err := os.Lstat(state)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return inv, errors.New("state must be a private directory, not a symlink")
	}
	data, err := protected(filepath.Join(state, "companion.json"))
	if err != nil {
		return inv, err
	}
	if err = json.Unmarshal(data, &inv); err != nil {
		return inv, err
	}
	return inv, validate(inv)
}
func sourceRegistration(state string, inv inventory) registration {
	return registration{Organization: inv.Organization, Source: inv.Source, Config: map[string]any{
		"name": inv.Source, "display_name": "Synthetic companion (EVALUATION ONLY)", "kind": "https-json",
		"ca_file": filepath.Join(state, "source-ca.pem"), "token_file": filepath.Join(state, "source-token.secret"), "attributes": []any{"uid"},
		"organizations": map[string]any{inv.Organization: map[string]any{"url": fmt.Sprintf("https://127.0.0.1:%d/directory/%s", inv.Port, inv.Organization)}},
	}}
}
func newContract(endpoint, ca, cert, key string) (contract, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return contract{}, errors.New("explicit HTTPS production endpoint required; no discovery or fallback")
	}
	roots := x509.NewCertPool()
	data, err := os.ReadFile(ca)
	if err != nil || !roots.AppendCertsFromPEM(data) {
		return contract{}, errors.New("production CA unavailable")
	}
	if _, err := protected(key); err != nil {
		return contract{}, errors.New("ordinary setup mTLS key must be a protected regular file")
	}
	pair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return contract{}, errors.New("ordinary setup mTLS identity unavailable")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{pair}}}
	return contract{&http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, strings.TrimRight(endpoint, "/")}, nil
}
func (api contract) call(ctx context.Context, method, path string, value, result any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, api.endpoint+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := api.client.Do(request)
	if err != nil {
		return errors.New("ordinary production contract unavailable; no fallback attempted")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("ordinary source contract %s %s returned HTTP %d; no fallback attempted", method, path, response.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(data) > 65536 {
		return errors.New("invalid source contract response")
	}
	if result != nil {
		if err = json.Unmarshal(data, result); err != nil {
			return errors.New("invalid source contract response")
		}
	}
	return nil
}
func inspect(ctx context.Context, api contract, state string, inv inventory) (registration, bool, error) {
	payload := sourceRegistration(state, inv)
	var current registry
	if err := api.call(ctx, http.MethodGet, "/api/assertions/sources", nil, &current); err != nil {
		return payload, false, err
	}
	if !current.Durable || current.Revision == "" || current.Organization != inv.Organization {
		return payload, false, errors.New("durable public contract must still authorize the saved evaluation organization; no org switching or private bootstrap attempted")
	}
	payload.Revision = current.Revision
	for _, entry := range current.Sources {
		if entry.Source == inv.Source {
			if !entry.Editable || !reflect.DeepEqual(withoutServerVersion(entry.Config), withoutServerVersion(payload.Config)) {
				return payload, false, errors.New("source identity collision or changed ownership/configuration; refusing mutation")
			}
			return payload, true, nil
		}
	}
	return payload, false, nil
}

// withoutServerVersion drops the registry-owned identity version so readback
// compares only operator-supplied configuration.
func withoutServerVersion(config map[string]any) map[string]any {
	out := make(map[string]any, len(config))
	for key, value := range config {
		if key != "version" {
			out[key] = value
		}
	}
	return out
}

func reconcile(ctx context.Context, api contract, state string, inv inventory, remove bool) error {
	payload, exists, err := inspect(ctx, api, state, inv)
	if err != nil {
		return err
	}
	if remove {
		if !exists {
			return errors.New("companion source absent; nothing removed")
		}
		if err = api.call(ctx, http.MethodDelete, "/api/assertions/sources", payload, nil); err != nil {
			return err
		}
	} else {
		var tested struct {
			Organization string `json:"organization_id"`
			Source       string `json:"source_id"`
			Status       string `json:"status"`
		}
		if err = api.call(ctx, http.MethodPost, "/api/assertions/sources/test", payload, &tested); err != nil {
			return err
		}
		if tested.Organization != inv.Organization || tested.Source != inv.Source || tested.Status != "success" {
			return errors.New("source test identity/status mismatch; not registered")
		}
		if !exists {
			if err = api.call(ctx, http.MethodPost, "/api/assertions/sources", payload, nil); err != nil {
				return err
			}
		}
	}
	_, present, err := inspect(ctx, api, state, inv)
	if err != nil {
		return err
	}
	if present == remove {
		return errors.New("source registration/removal readback failed; inspect registry before retry; no fallback attempted")
	}
	return nil
}
func directoryServer(state string, inv inventory) (*http.Server, net.Listener, error) {
	if _, err := protected(filepath.Join(state, "source-key.pem")); err != nil {
		return nil, nil, err
	}
	token, err := protected(filepath.Join(state, "source-token.secret"))
	if err != nil {
		return nil, nil, err
	}
	expected := sha256.Sum256([]byte("Bearer " + strings.TrimSpace(string(token))))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /directory/"+inv.Organization, func(w http.ResponseWriter, r *http.Request) {
		actual := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		data, err := protected(filepath.Join(state, "directory.json"))
		if err != nil {
			http.Error(w, "directory unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	})
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", inv.Port))
	if err != nil {
		return nil, nil, err
	}
	return &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}, listener, nil
}
