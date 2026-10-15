package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"

	"gopkg.in/yaml.v3"
)

type identityStackConfig struct {
	BindIP    string `json:"bind_ip"`
	LDAPImage string `json:"ldap_image"`
	DexImage  string `json:"dex_image"`
	Platform  string `json:"platform"`
	UID       int    `json:"uid"`
	GID       int    `json:"gid"`
}

var immutableImage = regexp.MustCompile(`^(sha256:[0-9a-f]{64}|([a-z0-9][a-z0-9.-]*(:[0-9]+)?/)?[a-z0-9]+([._-][a-z0-9]+)*(/[a-z0-9]+([._-][a-z0-9]+)*)*@sha256:[0-9a-f]{64})$`)

func (config identityStackConfig) validate(providers providerConfig) error {
	ip, err := netip.ParseAddr(config.BindIP)
	if err != nil || !ip.Is4() || !ip.IsPrivate() || ip.String() != config.BindIP {
		return errors.New("identity Compose requires an explicit canonical private IPv4 bind-ip")
	}
	if !immutableImage.MatchString(config.LDAPImage) || !immutableImage.MatchString(config.DexImage) {
		return errors.New("identity Compose requires both immutable image IDs or repository@sha256 digests; tags and substitutions are not accepted")
	}
	if config.Platform != "linux/amd64" && config.Platform != "linux/arm64" {
		return errors.New("identity platform must be linux/amd64 or engineering-only linux/arm64")
	}
	if config.UID < 0 || config.GID < 0 {
		return errors.New("identity Compose requires a valid invoking Unix UID/GID")
	}
	ports := map[string]bool{"1636": true}
	for _, origin := range []string{providers.RoomOrigin, providers.SimulatorOrigin, providers.OIDCOrigin} {
		address, err := url.Parse(origin)
		if err != nil {
			return errors.New("identity Compose requires valid Room, Simulator and OIDC origins")
		}
		port := address.Port()
		if port == "" {
			port = "443"
		}
		if ports[port] {
			return errors.New("same-host Room, Simulator, LDAP and OIDC require distinct ports")
		}
		ports[port] = true
	}
	return nil
}

func (config identityStackConfig) render(domain string, providers providerConfig, files map[string][]byte) error {
	if err := config.validate(providers); err != nil {
		return err
	}
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return errors.New("cannot generate independent companion Compose project identity")
	}
	project := "zpr-lab-identity-" + hex.EncodeToString(random)
	bind := func(source, target string, readonly bool) map[string]any {
		return map[string]any{
			"type": "bind", "source": "./" + source, "target": target, "read_only": readonly,
			"bind": map[string]bool{"create_host_path": false},
		}
	}
	service := func(image string, names []string, state, target string) map[string]any {
		mounts := []map[string]any{bind(state, target, false)}
		for _, name := range names {
			mounts = append(mounts, bind(name, "/etc/zpr/lab/"+name, true))
		}
		return map[string]any{
			"image": image, "platform": config.Platform, "pull_policy": "never",
			"user":      fmt.Sprintf("%d:%d", config.UID, config.GID),
			"read_only": true, "cap_drop": []string{"ALL"},
			"security_opt": []string{"no-new-privileges:true"},
			"tmpfs":        []string{"/tmp:rw,noexec,nosuid,size=16777216,mode=1777"},
			"volumes":      mounts, "pids_limit": 128,
			"logging": map[string]any{"driver": "json-file", "options": map[string]string{"max-size": "10m", "max-file": "3"}},
		}
	}
	initializer := service(config.LDAPImage,
		[]string{"slapd.conf", "directory.ldif", "initialize-directory.sh", "ldap-chain.pem", "ldap-key.pem", "lab-ca.pem"},
		"ldap-state", "/var/lib/ldap")
	initializer["profiles"] = []string{"initialize"}
	initializer["network_mode"] = "none"
	initializer["entrypoint"] = []string{"/bin/sh", "/etc/zpr/lab/initialize-directory.sh"}
	initializer["restart"] = "no"

	files["check-directory.sh"] = fmt.Appendf(nil, `#!/bin/sh
set -eu
umask 077
password_file=$(mktemp /tmp/zpr-ldap-health.XXXXXXXX)
trap 'rm -f "$password_file"' EXIT
trap 'exit 1' HUP INT TERM
password=$(cat /etc/zpr/lab/ldap-reader.secret)
printf '%%s' "$password" > "$password_file"
unset password
env LDAPTLS_REQCERT=demand LDAPTLS_CACERT=/etc/zpr/lab/lab-ca.pem ldapsearch -LLL -x -o nettimeout=2 -l 2 \
    -H ldaps://ldap.%s:1636 -D cn=zpr-reader,ou=Services,dc=lab,dc=zpr \
    -y "$password_file" -b dc=lab,dc=zpr -s base '(objectClass=*)' dn
`, domain)
	ldap := service(config.LDAPImage,
		[]string{"slapd.conf", "serve-directory.sh", "check-directory.sh", "ldap-reader.secret", "ldap-chain.pem", "ldap-key.pem", "lab-ca.pem"},
		"ldap-state", "/var/lib/ldap")
	ldap["entrypoint"] = []string{"/bin/sh", "/etc/zpr/lab/serve-directory.sh"}
	ldap["restart"] = "unless-stopped"
	ldap["ports"] = []map[string]any{{"target": 1636, "published": "1636", "host_ip": config.BindIP, "protocol": "tcp"}}
	ldap["networks"] = map[string]any{"identity": map[string]any{"aliases": []string{"ldap." + domain}}}
	ldap["healthcheck"] = map[string]any{
		"test":     []string{"CMD", "/bin/sh", "/etc/zpr/lab/check-directory.sh"},
		"interval": "5s", "timeout": "4s", "retries": 12, "start_period": "10s",
	}
	dex := service(config.DexImage, []string{"dex.yaml", "oidc-chain.pem", "oidc-key.pem", "lab-ca.pem"},
		"dex-state", "/var/lib/dex")
	dex["entrypoint"] = []string{"dex", "serve", "/etc/zpr/lab/dex.yaml"}
	dex["restart"] = "unless-stopped"
	issuer, _ := url.Parse(providers.OIDCOrigin)
	port := issuer.Port()
	if port == "" {
		port = "443"
	}
	dex["ports"] = []map[string]any{{"target": port, "published": port, "host_ip": config.BindIP, "protocol": "tcp"}}
	dex["networks"] = []string{"identity"}
	dex["depends_on"] = map[string]any{"ldap": map[string]string{"condition": "service_healthy"}}
	document := map[string]any{
		"name": project, "services": map[string]any{"ldap-initialize": initializer, "ldap": ldap, "oidc": dex},
		"networks": map[string]any{"identity": map[string]string{"driver": "bridge"}},
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return errors.New("cannot encode companion identity Compose")
	}
	files["identity-compose.yaml"] = data
	return nil
}
