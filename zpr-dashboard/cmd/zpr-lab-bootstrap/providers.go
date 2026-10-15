package main

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"

	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

type providerConfig struct {
	RoomOrigin      string               `json:"room_origin"`
	SimulatorOrigin string               `json:"simulator_origin"`
	OIDCOrigin      string               `json:"oidc_origin"`
	Deployment      *identityStackConfig `json:"deployment,omitempty"`
}

func (config providerConfig) validate(domain string) error {
	for host, origin := range map[string]string{
		"room": config.RoomOrigin, "simulator": config.SimulatorOrigin, "oidc": config.OIDCOrigin,
	} {
		address, err := url.Parse(origin)
		if err != nil || systemconfig.ValidateManagementEndpoint(origin, "") != nil ||
			address.Hostname() != host+"."+domain {
			return errors.New("identity services require exact HTTPS room/simulator/oidc origins matching generated lab hostnames")
		}
	}
	if config.Deployment != nil {
		return config.Deployment.validate(config)
	}
	return nil
}

func (config providerConfig) render(domain string, files map[string][]byte) error {
	if err := config.validate(domain); err != nil {
		return err
	}
	secret := func(name string) string {
		return strings.TrimSuffix(string(files[name+".secret"]), "\n")
	}
	hashes := map[string]string{}
	for _, name := range []string{"ldap-admin", "ldap-reader", "lab-operator", "lab-observer"} {
		value := secret(name)
		if value == "" {
			return errors.New("identity rendering requires generated per-install LDAP and user secrets")
		}
		hash, err := ldapPasswordHash(value)
		if err != nil {
			return err
		}
		hashes[name] = hash
	}
	for _, name := range []string{"oidc-room-client", "oidc-simulator-client"} {
		if secret(name) == "" {
			return errors.New("identity rendering requires generated per-install OIDC client secrets")
		}
	}
	files["slapd.conf"] = fmt.Appendf(nil, `include /etc/ldap/schema/core.schema
include /etc/ldap/schema/cosine.schema
include /etc/ldap/schema/inetorgperson.schema
pidfile /var/lib/ldap/slapd.pid
argsfile /var/lib/ldap/slapd.args
modulepath /usr/lib/ldap
moduleload back_mdb
TLSCertificateFile /etc/zpr/lab/ldap-chain.pem
TLSCertificateKeyFile /etc/zpr/lab/ldap-key.pem
TLSCACertificateFile /etc/zpr/lab/lab-ca.pem
TLSProtocolMin 3.3
disallow bind_anon
require authc
security simple_bind=128
database mdb
maxsize 1073741824
suffix "dc=lab,dc=zpr"
rootdn "cn=admin,dc=lab,dc=zpr"
rootpw %s
directory /var/lib/ldap
index objectClass eq
index uid,member eq
access to attrs=userPassword
    by anonymous auth
    by * none
access to *
    by dn.exact="cn=zpr-reader,ou=Services,dc=lab,dc=zpr" read
    by self read
    by * none
`, hashes["ldap-admin"])
	files["directory.ldif"] = fmt.Appendf(nil, `dn: dc=lab,dc=zpr
objectClass: top
objectClass: dcObject
objectClass: organization
dc: lab
o: ZPR companion lab only

dn: ou=People,dc=lab,dc=zpr
objectClass: organizationalUnit
ou: People

dn: ou=Groups,dc=lab,dc=zpr
objectClass: organizationalUnit
ou: Groups

dn: ou=Services,dc=lab,dc=zpr
objectClass: organizationalUnit
ou: Services

dn: cn=zpr-reader,ou=Services,dc=lab,dc=zpr
objectClass: organizationalRole
objectClass: simpleSecurityObject
cn: zpr-reader
userPassword: %s

dn: uid=lab-operator,ou=People,dc=lab,dc=zpr
objectClass: inetOrgPerson
uid: lab-operator
cn: Lab Operator
sn: Operator
mail: lab-operator@%s
userPassword: %s

dn: uid=lab-observer,ou=People,dc=lab,dc=zpr
objectClass: inetOrgPerson
uid: lab-observer
cn: Lab Observer
sn: Observer
mail: lab-observer@%s
userPassword: %s

dn: cn=lab-operators,ou=Groups,dc=lab,dc=zpr
objectClass: groupOfNames
cn: lab-operators
member: uid=lab-operator,ou=People,dc=lab,dc=zpr

dn: cn=lab-observers,ou=Groups,dc=lab,dc=zpr
objectClass: groupOfNames
cn: lab-observers
member: uid=lab-observer,ou=People,dc=lab,dc=zpr
`, hashes["ldap-reader"], domain, hashes["lab-operator"], domain, hashes["lab-observer"])
	files["initialize-directory.sh"] = []byte(`#!/bin/sh
set -eu
umask 077
if [ ! -d /var/lib/ldap ] || [ -n "$(find /var/lib/ldap -mindepth 1 -maxdepth 1 -print -quit)" ]; then
    echo "Lab LDAP initialization requires an existing empty state directory; existing or partial state is never reset." >&2
    exit 1
fi
slaptest -u -f /etc/zpr/lab/slapd.conf
slapadd -f /etc/zpr/lab/slapd.conf -l /etc/zpr/lab/directory.ldif
test -s /var/lib/ldap/data.mdb
set -C
: > /var/lib/ldap/lab-initialized
`)
	files["serve-directory.sh"] = []byte(`#!/bin/sh
set -eu
umask 077
if [ ! -f /var/lib/ldap/lab-initialized ] || [ ! -s /var/lib/ldap/data.mdb ]; then
    echo "Lab LDAP is not initialized; inspect state and run the separate initializer on a new empty state directory." >&2
    exit 1
fi
slaptest -u -f /etc/zpr/lab/slapd.conf
exec slapd -f /etc/zpr/lab/slapd.conf -h ldaps://0.0.0.0:1636 -d 0
`)
	issuer, _ := url.Parse(config.OIDCOrigin)
	port := issuer.Port()
	if port == "" {
		port = "443"
	}
	dex := map[string]any{
		"issuer":  config.OIDCOrigin,
		"storage": map[string]any{"type": "sqlite3", "config": map[string]string{"file": "/var/lib/dex/dex.db"}},
		"web": map[string]string{
			"https":   "0.0.0.0:" + port,
			"tlsCert": "/etc/zpr/lab/oidc-chain.pem", "tlsKey": "/etc/zpr/lab/oidc-key.pem",
		},
		"oauth2":           map[string]any{"responseTypes": []string{"code"}},
		"enablePasswordDB": false,
		"staticClients": []map[string]any{
			{"id": "zpr-control-room", "name": "ZPR Lab Control Room", "secret": secret("oidc-room-client"),
				"redirectURIs": []string{config.RoomOrigin + "/auth/operator/callback"}},
			{"id": "zpr-simulator", "name": "ZPR Lab Simulator", "secret": secret("oidc-simulator-client"),
				"redirectURIs": []string{config.SimulatorOrigin + "/auth/operator/callback"}},
		},
		"connectors": []map[string]any{{
			"type": "ldap", "id": "lab-ldap", "name": "Lab LDAP",
			"config": map[string]any{
				"host": "ldap." + domain + ":1636", "rootCA": "/etc/zpr/lab/lab-ca.pem",
				"insecureNoSSL": false, "insecureSkipVerify": false, "startTLS": false,
				"bindDN": "cn=zpr-reader,ou=Services,dc=lab,dc=zpr", "bindPW": secret("ldap-reader"),
				"userSearch": map[string]string{
					"baseDN": "ou=People,dc=lab,dc=zpr", "filter": "(objectClass=inetOrgPerson)",
					"username": "uid", "idAttr": "entryUUID", "emailAttr": "mail",
					"nameAttr": "cn", "preferredUsernameAttr": "uid",
				},
				"groupSearch": map[string]any{
					"baseDN": "ou=Groups,dc=lab,dc=zpr", "filter": "(objectClass=groupOfNames)", "nameAttr": "cn",
					"userMatchers": []map[string]string{{"userAttr": "DN", "groupAttr": "member"}},
				},
			},
		}},
	}
	data, err := yaml.Marshal(dex)
	if err != nil {
		return errors.New("cannot encode Dex lab configuration")
	}
	files["dex.yaml"] = data
	handoff := map[string]any{
		"schema_version": 1, "kind": "simulator-lab-identity-inputs",
		"issuer": config.OIDCOrigin, "ca_file": "lab-ca.pem",
		"ldap": map[string]string{
			"url": "ldaps://ldap." + domain + ":1636", "base_dn": "dc=lab,dc=zpr",
			"users_base_dn": "ou=People,dc=lab,dc=zpr", "groups_base_dn": "ou=Groups,dc=lab,dc=zpr",
			"bind_dn": "cn=zpr-reader,ou=Services,dc=lab,dc=zpr", "password_file": "ldap-reader.secret",
		},
		"clients": map[string]any{
			"room": map[string]string{"client_id": "zpr-control-room", "client_secret_file": "oidc-room-client.secret",
				"redirect_url": config.RoomOrigin + "/auth/operator/callback"},
			"simulator": map[string]string{"client_id": "zpr-simulator", "client_secret_file": "oidc-simulator-client.secret",
				"redirect_url": config.SimulatorOrigin + "/auth/operator/callback"},
		},
		"files": map[string]string{
			"directory_config": "slapd.conf", "directory_seed": "directory.ldif", "oidc_config": "dex.yaml",
			"directory_initializer": "initialize-directory.sh", "directory_server": "serve-directory.sh",
		},
		"grant_status": "not-configured: obtain verified issuer/sub from live OIDC; group membership does not grant ZPR operator permissions",
	}
	if config.Deployment != nil {
		handoff["deployment_files"] = map[string]string{
			"compose": "identity-compose.yaml", "directory_healthcheck": "check-directory.sh",
			"directory_state": "ldap-state", "oidc_state": "dex-state",
		}
	}
	data, err = json.MarshalIndent(handoff, "", "  ")
	if err != nil {
		return errors.New("cannot encode lab identity input inventory")
	}
	files["identity-inputs.json"] = append(data, '\n')
	if config.Deployment != nil {
		return config.Deployment.render(domain, config, files)
	}
	return nil
}

func ldapPasswordHash(password string) (string, error) {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return "", errors.New("cannot generate LDAP password salt")
	}
	// OpenLDAP's portable SSHA format protects randomly generated 256-bit secrets,
	// not operator-chosen low-entropy passwords.
	hash := sha1.New()
	hash.Write([]byte(password))
	hash.Write(salt)
	return "{SSHA}" + base64.StdEncoding.EncodeToString(append(hash.Sum(nil), salt...)), nil
}
