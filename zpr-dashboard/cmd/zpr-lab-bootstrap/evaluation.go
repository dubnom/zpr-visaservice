package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"neboagency.com/zpr-dashborad/internal/controlservice/adapterlogs"
)

const evaluationBanner = "EVALUATION ONLY — NOT PRODUCTION SUPPORTED. Local administrators only; no Simulator dependency."
const evaluationLDAPImage = "osixia/openldap@sha256:0de3850635911109f0daf8f5957808ec4f4eb827fbaaa339100b5f1bab925745"
const evaluationDexImage = "ghcr.io/dexidp/dex@sha256:5d0656fce7d453c0e3b2706abf40c0d0ce5b371fb0b73b3cf714d05f35fa5f86"
const evaluationLogImage = "public.ecr.aws/zinclabs/openobserve@sha256:d4a878fac1f6c56003764f7f2a1625668917388f167e222c8c810de3f54c56ba"

type evaluationConfig struct {
	DexImage    string `json:"oidc_image"`
	DexPlatform string `json:"oidc_platform"`
	OIDCPort    int    `json:"oidc_port"`
	LDAPImage   string `json:"ldap_image"`
	LogImage    string `json:"logging_image"`
	LogPlatform string `json:"logging_platform"`
	RoomPort    int    `json:"room_port"`
	ServicePort int    `json:"service_port"`
	LDAPPort    int    `json:"ldap_port"`
	LogPort     int    `json:"logging_port"`
}

type evaluationInventory struct {
	Version   int               `json:"version"`
	Kind      string            `json:"kind"`
	InstallID string            `json:"install_id"`
	Config    evaluationConfig  `json:"config"`
	Files     map[string]string `json:"files_sha256"`
}

func runEvaluation(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-lab-bootstrap -evaluation", flag.ContinueOnError)
	flags.SetOutput(stderr)
	state := flags.String("state", "", "protected persistent install directory (required)")
	renderOnly := flags.Bool("render-only", false, "render/validate protected files only; no builds, Docker or services")
	config := evaluationConfig{}
	flags.StringVar(&config.LDAPImage, "ldap-image", evaluationLDAPImage, "current packaged OpenLDAP immutable image")
	flags.StringVar(&config.LogImage, "logging-image", evaluationLogImage, "current packaged OpenObserve immutable image")
	flags.StringVar(&config.LogPlatform, "logging-platform", "linux/arm64", "platform of the current packaged OpenObserve image (linux/amd64 requires a matching image override)")
	flags.StringVar(&config.DexImage, "oidc-image", evaluationDexImage, "current packaged Dex immutable image")
	flags.StringVar(&config.DexPlatform, "oidc-platform", "linux/arm64", "platform of the current packaged Dex image")
	flags.IntVar(&config.OIDCPort, "oidc-port", 15556, "loopback named-operator OIDC HTTPS port")
	flags.IntVar(&config.RoomPort, "room-port", 18787, "loopback Control Room HTTPS port")
	flags.IntVar(&config.ServicePort, "service-port", 18790, "loopback Control-Service mTLS port")
	flags.IntVar(&config.LDAPPort, "ldap-port", 11636, "loopback starter LDAPS port")
	flags.IntVar(&config.LogPort, "logging-port", 15080, "loopback OpenObserve HTTPS port")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *state == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: zpr-lab-bootstrap -evaluation -state DIRECTORY [-render-only]")
		return 2
	}
	fmt.Fprintln(stdout, evaluationBanner)
	absolute, err := filepath.Abs(*state)
	if err == nil {
		err = config.validate()
	}
	var inventory evaluationInventory
	if err == nil {
		inventory, err = prepareEvaluation(absolute, config, time.Now().UTC())
	}
	if err != nil {
		fmt.Fprintf(stderr, "Evaluation bootstrap: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Install %s: protected state validated; secrets are in private files, never printed.\n", inventory.InstallID)
	if *renderOnly {
		fmt.Fprintln(stdout, "RENDER ONLY: no services started; protocol readiness NOT verified.")
		return 0
	}
	if err := startEvaluation(absolute, inventory, stdout); err != nil {
		fmt.Fprintf(stderr, "Evaluation bootstrap: %v\n", err)
		return 1
	}
	return 0
}

func (config evaluationConfig) validate() error {
	if !immutableImage.MatchString(config.DexImage) || !immutableImage.MatchString(config.LDAPImage) || !immutableImage.MatchString(config.LogImage) {
		return errors.New("evaluation images must be immutable image IDs or repository@sha256 digests")
	}
	if config.DexPlatform != "linux/amd64" && config.DexPlatform != "linux/arm64" {
		return errors.New("OIDC platform must be linux/amd64 or linux/arm64")
	}
	if config.LogPlatform != "linux/amd64" && config.LogPlatform != "linux/arm64" {
		return errors.New("logging platform must be linux/amd64 or linux/arm64")
	}
	ports := map[int]bool{}
	for _, port := range []int{config.OIDCPort, config.RoomPort, config.ServicePort, config.LDAPPort, config.LogPort} {
		if port < 1024 || port > 65535 || ports[port] {
			return errors.New("evaluation ports must be distinct unprivileged TCP ports")
		}
		ports[port] = true
	}
	return nil
}

func prepareEvaluation(output string, config evaluationConfig, now time.Time) (evaluationInventory, error) {
	empty := evaluationInventory{}
	if err := config.validate(); err != nil {
		return empty, err
	}
	if output == string(filepath.Separator) || !filepath.IsAbs(output) || filepath.Clean(output) != output {
		return empty, errors.New("state must be a canonical absolute non-root path")
	}
	parent, err := os.OpenRoot(filepath.Dir(output))
	if err != nil {
		return empty, errors.New("create an existing protected state parent first")
	}
	defer parent.Close()
	info, err := parent.Stat(".")
	if err != nil || info.Mode().Perm()&0o022 != 0 {
		return empty, errors.New("state parent must not be group/other writable")
	}
	if info, err := os.Lstat(output); err == nil {
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			return empty, errors.New("existing state must be a real private (0700) directory, not a symlink")
		}
		return readEvaluation(output, config)
	} else if !errors.Is(err, os.ErrNotExist) {
		return empty, errors.New("cannot inspect state directory")
	}
	files, inventory, err := generateEvaluation(output, config, now)
	if err != nil {
		return empty, err
	}
	if err := parent.Mkdir(filepath.Base(output), 0o700); err != nil {
		return empty, errors.New("cannot exclusively create state directory; inspect before retrying")
	}
	root, err := parent.OpenRoot(filepath.Base(output))
	if err != nil {
		return empty, err
	}
	defer root.Close()
	for _, name := range []string{"ldap-state", "logging-state", "oidc-state", "logs", "build"} {
		if err := root.Mkdir(name, 0o700); err != nil {
			return empty, err
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if err := writePrivate(root, name, files[name]); err != nil {
			return empty, errors.New("incomplete evaluation state; inspect it before retrying, never reseed")
		}
	}
	data, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return empty, err
	}
	if err := writePrivate(root, "evaluation.json", append(data, '\n')); err != nil {
		return empty, err
	}
	// Mutable operator registries are deliberately not frozen in the inventory.
	mutable := map[string]any{
		"assertion-sources.json": map[string]any{"default_source": "", "sources": []any{}},
		"adapter-logs.json": adapterlogs.Inventory{Adapters: []adapterlogs.Target{{
			ID: "logs_" + inventory.InstallID, Name: "OpenObserve (EVALUATION ONLY)",
			Sources: []adapterlogs.SourceConfig{evaluationLogSource(output, inventory)},
		}}},
	}
	for name, value := range mutable {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return empty, err
		}
		if err := writePrivate(root, name, append(data, '\n')); err != nil {
			return empty, err
		}
	}
	return inventory, nil
}

func readEvaluation(output string, config evaluationConfig) (evaluationInventory, error) {
	inventory := evaluationInventory{}
	if err := filepath.WalkDir(output, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || (!entry.IsDir() && !info.Mode().IsRegular()) {
			return errors.New("state contains a symlink or special file; refuse unsafe reuse")
		}
		return nil
	}); err != nil {
		return inventory, err
	}
	data, err := os.ReadFile(filepath.Join(output, "evaluation.json"))
	if err != nil || json.Unmarshal(data, &inventory) != nil || inventory.Version != 1 || inventory.Kind != "zpr-evaluation-only" || len(inventory.InstallID) != 24 {
		return inventory, errors.New("incomplete/invalid evaluation inventory; inspect state, never automatically reset it")
	}
	if _, err := hex.DecodeString(inventory.InstallID); err != nil {
		return inventory, errors.New("invalid install identity")
	}
	if !reflect.DeepEqual(inventory.Config, config) {
		return inventory, errors.New("existing install configuration differs; reuse original image/port flags or choose a NEW state directory")
	}
	for _, name := range []string{"assertion-sources.json", "adapter-logs.json"} {
		info, err := os.Lstat(filepath.Join(output, name))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return inventory, errors.New("operator registry missing or unprotected; refuse reseed")
		}
	}
	required := []string{"ca.pem", "ca-key.pem", "server-chain.pem", "server-key.pem", "client-chain.pem", "client-key.pem", "ldap-admin.secret", "ldap-reader.secret", "logging-admin.secret", "openobserve.env", "slapd.conf", "directory.ldif", "serve-directory.sh", "compose.json", "dex.json", "operator-admin.secret", "oidc-room-client.secret", "EVALUATION.txt", "source-registration.json"}
	if len(inventory.Files) != len(required) {
		return inventory, errors.New("evaluation inventory is missing required files")
	}
	for _, name := range required {
		expected, ok := inventory.Files[name]
		if !ok {
			return inventory, errors.New("evaluation inventory is missing required files")
		}
		info, err := os.Stat(filepath.Join(output, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			return inventory, fmt.Errorf("protected file %s must remain private (0600)", name)
		}
		data, err := os.ReadFile(filepath.Join(output, name))
		sum := sha256.Sum256(data)
		if err != nil || hex.EncodeToString(sum[:]) != expected {
			return inventory, fmt.Errorf("protected file %s missing or changed; refuse regeneration", name)
		}
	}
	return inventory, nil
}

func generateEvaluation(state string, config evaluationConfig, now time.Time) (map[string][]byte, evaluationInventory, error) {
	files := map[string][]byte{}
	inventory := evaluationInventory{Version: 1, Kind: "zpr-evaluation-only", Config: config, Files: map[string]string{}}
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return nil, inventory, err
	}
	inventory.InstallID = hex.EncodeToString(random)
	ca, key, caPEM, caKey, err := certificateWithOrganization("ZPR evaluation-only CA", "localhost", "ZPR EVALUATION ONLY", now, nil, nil, x509.ExtKeyUsageServerAuth)
	if err != nil {
		return nil, inventory, err
	}
	defer key.D.SetInt64(0)
	files["ca.pem"], files["ca-key.pem"] = caPEM, caKey
	for name, usage := range map[string]x509.ExtKeyUsage{"server": x509.ExtKeyUsageServerAuth, "client": x509.ExtKeyUsageClientAuth} {
		_, leafKey, leaf, secret, err := certificateWithOrganization("ZPR evaluation-only "+name, "localhost", "ZPR EVALUATION ONLY", now, ca, key, usage)
		if err != nil {
			return nil, inventory, err
		}
		leafKey.D.SetInt64(0)
		files[name+"-chain.pem"], files[name+"-key.pem"] = append(leaf, caPEM...), secret
	}
	for _, name := range []string{"ldap-admin", "ldap-reader", "logging-admin", "operator-admin", "oidc-room-client"} {
		random := make([]byte, 32)
		if _, err := rand.Read(random); err != nil {
			return nil, inventory, err
		}
		files[name+".secret"] = []byte(base64.RawURLEncoding.EncodeToString(random) + "!Aa1")
	}
	admin, err := ldapPasswordHash(strings.TrimSpace(string(files["ldap-admin.secret"])))
	if err != nil {
		return nil, inventory, err
	}
	reader, err := ldapPasswordHash(strings.TrimSpace(string(files["ldap-reader.secret"])))
	if err != nil {
		return nil, inventory, err
	}
	files["slapd.conf"] = fmt.Appendf(nil, `include /etc/ldap/schema/core.schema
include /etc/ldap/schema/cosine.schema
include /etc/ldap/schema/inetorgperson.schema
pidfile /var/lib/ldap/slapd.pid
argsfile /var/lib/ldap/slapd.args
modulepath /usr/lib/ldap
moduleload back_mdb
TLSCertificateFile /etc/zpr/evaluation/server-chain.pem
TLSCertificateKeyFile /etc/zpr/evaluation/server-key.pem
TLSCACertificateFile /etc/zpr/evaluation/ca.pem
TLSProtocolMin 3.3
disallow bind_anon
require authc
security simple_bind=128
database mdb
maxsize 1073741824
suffix "dc=evaluation,dc=zpr"
rootdn "cn=admin,dc=evaluation,dc=zpr"
rootpw %s
directory /var/lib/ldap
index objectClass eq
access to attrs=userPassword
    by anonymous auth
    by * none
access to *
    by dn.exact="cn=zpr-reader,dc=evaluation,dc=zpr" read
    by * none
`, admin)
	files["directory.ldif"] = fmt.Appendf(nil, `dn: dc=evaluation,dc=zpr
objectClass: top
objectClass: dcObject
objectClass: organization
dc: evaluation
o: ZPR EVALUATION ONLY %s

dn: cn=zpr-reader,dc=evaluation,dc=zpr
objectClass: organizationalRole
objectClass: simpleSecurityObject
cn: zpr-reader
userPassword: %s

dn: ou=People,dc=evaluation,dc=zpr
objectClass: organizationalUnit
ou: People

dn: ou=Groups,dc=evaluation,dc=zpr
objectClass: organizationalUnit
ou: Groups
`, inventory.InstallID, reader)
	files["serve-directory.sh"] = []byte(`#!/bin/sh
set -eu
umask 077
if [ ! -e /var/lib/ldap/evaluation-initialized ]; then
    if [ -n "$(find /var/lib/ldap -mindepth 1 -maxdepth 1 -print -quit)" ]; then
        echo 'Partial LDAP state: refusing reseed; inspect private state.' >&2
        exit 1
    fi
    slaptest -u -f /etc/zpr/evaluation/slapd.conf
    slapadd -f /etc/zpr/evaluation/slapd.conf -l /etc/zpr/evaluation/directory.ldif
    test -s /var/lib/ldap/data.mdb
    : > /var/lib/ldap/evaluation-initialized
fi
test -s /var/lib/ldap/data.mdb
exec slapd -f /etc/zpr/evaluation/slapd.conf -h ldaps://0.0.0.0:1636 -d 0
`)
	files["openobserve.env"] = fmt.Appendf(nil, `ZO_ROOT_USER_EMAIL=admin@evaluation.invalid
ZO_ROOT_USER_PASSWORD=%s
ZO_DATA_DIR=/data
ZO_HTTP_ADDR=0.0.0.0
ZO_HTTP_PORT=5080
ZO_HTTP_TLS_ENABLED=true
ZO_HTTP_TLS_CERT_PATH=/etc/zpr/evaluation/server-chain.pem
ZO_HTTP_TLS_KEY_PATH=/etc/zpr/evaluation/server-key.pem
ZO_COMPACT_DATA_RETENTION_DAYS=7
ZO_TELEMETRY=false
ZO_LOG_LEVEL=warn
`, strings.TrimSpace(string(files["logging-admin.secret"])))
	mount := func(source, target string, readonly bool) map[string]any {
		return map[string]any{"type": "bind", "source": "./" + source, "target": target, "read_only": readonly, "bind": map[string]bool{"create_host_path": false}}
	}
	service := func(image, state, target string, port int, containerPort int, names ...string) map[string]any {
		mounts := []map[string]any{mount(state, target, false)}
		for _, name := range names {
			mounts = append(mounts, mount(name, "/etc/zpr/evaluation/"+name, true))
		}
		return map[string]any{"image": image, "platform": "linux/amd64", "pull_policy": "never", "user": fmt.Sprintf("%d:%d", os.Geteuid(), os.Getegid()), "read_only": true, "cap_drop": []string{"ALL"}, "security_opt": []string{"no-new-privileges:true"}, "volumes": mounts,
			"ports": []string{fmt.Sprintf("127.0.0.1:%d:%d", port, containerPort)}, "pids_limit": 256, "mem_limit": "1g", "cpus": 2,
			"logging": map[string]any{"driver": "none"},
		}
	}
	ldap := service(config.LDAPImage, "ldap-state", "/var/lib/ldap", config.LDAPPort, 1636, "server-chain.pem", "server-key.pem", "ca.pem", "slapd.conf", "directory.ldif", "serve-directory.sh", "ldap-reader.secret")
	ldap["entrypoint"] = []string{"/bin/sh", "/etc/zpr/evaluation/serve-directory.sh"}
	logging := service(config.LogImage, "logging-state", "/data", config.LogPort, 5080, "server-chain.pem", "server-key.pem")
	logging["platform"] = config.LogPlatform
	logging["env_file"] = []string{"./openobserve.env"}
	logging["environment"] = map[string]string{"RUST_LOG": "warn"}
	hash, err := bcrypt.GenerateFromPassword(files["operator-admin.secret"], bcrypt.DefaultCost)
	if err != nil {
		return nil, inventory, errors.New("cannot protect bootstrap administrator credential")
	}
	dexConfig := map[string]any{
		"issuer":           fmt.Sprintf("https://localhost:%d", config.OIDCPort),
		"storage":          map[string]any{"type": "sqlite3", "config": map[string]string{"file": "/var/lib/dex/dex.db"}},
		"web":              map[string]string{"https": "0.0.0.0:5556", "tlsCert": "/etc/zpr/evaluation/server-chain.pem", "tlsKey": "/etc/zpr/evaluation/server-key.pem"},
		"oauth2":           map[string]any{"responseTypes": []string{"code"}, "passwordConnector": "local"},
		"enablePasswordDB": true,
		"staticClients":    []map[string]any{{"id": "zpr-control-room", "name": "ZPR Evaluation Control Room", "secret": string(files["oidc-room-client.secret"]), "redirectURIs": []string{fmt.Sprintf("https://localhost:%d/auth/operator/callback", config.RoomPort)}}},
		"staticPasswords":  []map[string]string{{"email": "admin@evaluation.invalid", "hash": string(hash), "username": "Evaluation Administrator", "userID": inventory.InstallID}},
		"logger":           map[string]string{"level": "error", "format": "json"},
	}
	data, err := json.MarshalIndent(dexConfig, "", "  ")
	if err != nil {
		return nil, inventory, err
	}
	files["dex.json"] = append(data, '\n')
	dex := service(config.DexImage, "oidc-state", "/var/lib/dex", config.OIDCPort, 5556, "server-chain.pem", "server-key.pem", "dex.json")
	dex["platform"] = config.DexPlatform
	dex["entrypoint"] = []string{"dex", "serve", "/etc/zpr/evaluation/dex.json"}
	compose := map[string]any{"name": "zpr-evaluation-" + inventory.InstallID, "services": map[string]any{"ldap": ldap, "logging": logging, "oidc": dex}}
	data, err = json.MarshalIndent(compose, "", "  ")
	if err != nil {
		return nil, inventory, err
	}
	files["compose.json"] = append(data, '\n')
	files["EVALUATION.txt"] = []byte(evaluationBanner + "\nOpenObserve AGPL-3.0 accepted for evaluation; 7-day retention; admin-only loopback access.\nRegistration and lifecycle logging use ordinary production contracts. No policy enforcement or production install is claimed.\n")
	registration := evaluationSourceRegistration(state, inventory)
	data, err = json.MarshalIndent(registration, "", "  ")
	if err != nil {
		return nil, inventory, err
	}
	files["source-registration.json"] = append(data, '\n')
	for name, data := range files {
		sum := sha256.Sum256(data)
		inventory.Files[name] = hex.EncodeToString(sum[:])
	}
	return files, inventory, nil
}
