package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
)

type dnsConfig struct {
	BootstrapIP      string `json:"bootstrap_ip"`
	RoomIP           string `json:"room_ip"`
	ZPRDNSIP         string `json:"zpr_dns_ip"`
	ZPRClientPrefix  string `json:"zpr_client_prefix"`
	HostBootstrapDNS bool   `json:"host_bootstrap_dns"`
}

func (config dnsConfig) validate() error {
	underlay := config.BootstrapIP != "" || config.RoomIP != "" || config.HostBootstrapDNS
	protected := config.ZPRDNSIP != "" || config.ZPRClientPrefix != ""
	if !underlay && !protected {
		return errors.New("DNS generation requires underlay record addresses or protected ZPR DNS inputs")
	}
	if underlay {
		for _, value := range []string{config.BootstrapIP, config.RoomIP} {
			ip, err := netip.ParseAddr(value)
			if err != nil || !ip.Is4() || !ip.IsPrivate() || ip.String() != value {
				return errors.New("DNS generation requires explicit canonical private IPv4 bootstrap-ip and room-ip")
			}
		}
		if !protected {
			return nil
		}
	}
	ip, err := netip.ParseAddr(config.ZPRDNSIP)
	if err != nil || !ip.Is6() || ip.Is4In6() || ip.Zone() != "" || !ip.IsPrivate() || ip.String() != config.ZPRDNSIP {
		return errors.New("zpr-dns-ip must be a canonical ULA IPv6 address on the DNS adapter")
	}
	prefix, err := netip.ParsePrefix(config.ZPRClientPrefix)
	if err != nil || !prefix.Addr().Is6() || !prefix.Addr().IsPrivate() ||
		prefix.Bits() < 7 || prefix != prefix.Masked() ||
		prefix.String() != config.ZPRClientPrefix || !prefix.Contains(ip) {
		return errors.New("zpr-client-prefix must be a canonical ULA IPv6 network containing zpr-dns-ip")
	}
	return nil
}

func (config dnsConfig) render(domain string) (map[string][]byte, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	references := map[string]any{
		"schema_version": 1,
		"kind":           "simulator-lab-dns-files",
	}
	if config.BootstrapIP != "" {
		files["underlay-records.txt"] = fmt.Appendf(nil, `room.%s. 30 IN A %s
simulator.%s. 30 IN A %s
ldap.%s. 30 IN A %s
oidc.%s. 30 IN A %s
`, domain, config.RoomIP, domain, config.BootstrapIP, domain, config.BootstrapIP, domain, config.BootstrapIP)
		references["underlay_records"] = "underlay-records.txt"
	}
	if config.HostBootstrapDNS {
		renderBootstrapDNS(files, config, domain)
		references["bootstrap_config"] = "bootstrap-named.conf"
		references["bootstrap_zone"] = "bootstrap.zone"
	}
	if config.ZPRDNSIP != "" {
		if err := renderProtectedDNS(files, config); err != nil {
			return nil, err
		}
		for key, filename := range map[string]string{
			"zpr_config":           "zpr-named.conf",
			"zpr_zone":             "db.svc.zpr",
			"publisher_key":        "zpr-vs-publisher.key",
			"viewer_key":           "zpr-dns-viewer.key",
			"visa_config_fragment": "zpr-dns-update.toml",
			"policy_fragment":      "zpr-dns.zpl",
		} {
			references[key] = filename
		}
	}
	data, err := json.MarshalIndent(references, "", "  ")
	if err != nil {
		return nil, err
	}
	files["dns-files.json"] = append(data, '\n')
	return files, nil
}

func renderProtectedDNS(files map[string][]byte, config dnsConfig) error {
	for _, name := range []string{"zpr-vs-publisher", "zpr-dns-viewer"} {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return errors.New("cannot generate DNS TSIG material")
		}
		files[name+".key"] = fmt.Appendf(nil, "key \"%s\" {\n    algorithm hmac-sha256;\n    secret \"%s\";\n};\n", name, base64.StdEncoding.EncodeToString(secret))
	}
	files["zpr-named.conf"] = fmt.Appendf(nil, `acl zpr_clients { %s; };
include "/etc/zpr/lab/zpr-vs-publisher.key";
include "/etc/zpr/lab/zpr-dns-viewer.key";
options {
    directory "/var/cache/bind";
    recursion no;
    allow-recursion { none; };
    allow-query-cache { none; };
    allow-query { zpr_clients; };
    allow-transfer { none; };
    listen-on { none; };
    listen-on-v6 port 53 { %s; };
};
controls {};
zone "svc.zpr" {
    type primary;
    file "/var/lib/bind/db.svc.zpr";
    allow-transfer { key "zpr-dns-viewer"; };
    update-policy { grant zpr-vs-publisher zonesub A AAAA; };
};
`, config.ZPRClientPrefix, config.ZPRDNSIP)
	files["db.svc.zpr"] = fmt.Appendf(nil, `$ORIGIN svc.zpr.
$TTL 30
@ IN SOA dns.svc.zpr. hostmaster.svc.zpr. (1 60 60 86400 30)
  IN NS dns.svc.zpr.
dns IN AAAA %s
`, config.ZPRDNSIP)
	files["zpr-dns-update.toml"] = fmt.Appendf(nil, `[dns_update]
server = "%s"
port = 53
zone = "svc.zpr."
adapter_state_file = "/var/lib/zpr/dns-adapters.json"
tsig_key_file = "/etc/zpr/dns-publisher.key"
ttl_seconds = 30
nsupdate_bin = "/usr/bin/nsupdate"
`, config.ZPRDNSIP)
	files["zpr-dns.zpl"] = fmt.Appendf(nil, `define ZprDNS as a service with device.zpr.adapter.cn:zpr-dns and zpr.addr:'%s'.
define VisaDnsPublisher as an adapter with zpr.adapter.cn:'vs.zpr'.
provide ZprDNS at dns.svc.zpr over TCP 53.
  allow VisaDnsPublisher.
`, config.ZPRDNSIP)
	return nil
}

func renderBootstrapDNS(files map[string][]byte, config dnsConfig, domain string) {
	files["bootstrap-named.conf"] = fmt.Appendf(nil, `options {
    directory "/var/cache/bind";
    recursion no;
    allow-recursion { none; };
    allow-query-cache { none; };
    allow-query { localnets; localhost; };
    allow-transfer { none; };
    listen-on port 53 { %s; };
    listen-on-v6 { none; };
};
controls {};
zone "%s" {
    type primary;
    file "/etc/zpr/lab/bootstrap.zone";
    allow-update { none; };
};
`, config.BootstrapIP, domain)
	files["bootstrap.zone"] = fmt.Appendf(nil, `$ORIGIN %s.
$TTL 30
@ IN SOA bootstrap.%s. hostmaster.%s. (1 60 60 86400 30)
  IN NS bootstrap.%s.
bootstrap IN A %s
room IN A %s
simulator IN A %s
ldap IN A %s
oidc IN A %s
`, domain, domain, domain, domain, config.BootstrapIP, config.RoomIP, config.BootstrapIP, config.BootstrapIP, config.BootstrapIP)
}
