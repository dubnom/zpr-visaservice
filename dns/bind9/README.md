# ZPR DNS with BIND 9

This profile uses BIND 9 (ISC-licensed) as the DNS service implementation
behind a normal ZPR adapter. The adapter joins as the `zpr-dns` actor and
provides the DNS service; BIND listens on the adapter's ZPR interface address.
The Visa Service sends RFC 2136 updates with TSIG through `nsupdate` over a
separately policy-authorized ZPR flow. BIND never opens a public/underlay DNS
listener or accepts unauthenticated dynamic updates. Service identity and
client permissions are ordinary ZPL policy, not DNS ACLs alone.

## ZPR policy

Compile [`zpr-dns.zpl`](zpr-dns.zpl) with the network policy. It declares
`dns.svc.zpr` on TCP port 53 and permits authenticated device actors, identified
by the ZPR-provided `device.zpr.adapter.cn` attribute, to access it. The DNS
adapter binds the service actor's ZPR address, so the query socket is not
exposed on the substrate or host interfaces. Adapters can optionally run a
loopback-only DNS stub that accepts local UDP/TCP requests and forwards them to
the bootstrapped ZPR DNS address over TCP. Configure the host resolver to use
that loopback listener; the adapter does not edit system resolver settings.
Upstream TCP sockets are bound to the adapter TUN, so they cannot fall through
to the host's default interface. The stub uses only the configured server and
never falls back to an underlay resolver. The initial policy publishes TCP/53
only, so its upstream queries use DNS over TCP.
The example also permits the device whose authenticated adapter CN is `vs.zpr`
to access DNS; this is the policy gate for Visa Service's TSIG update flow.
TSIG remains a separate publisher credential and does not replace ZPR
authorization.

The DNS service's ZPR address and identity are bootstrap inputs: clients cannot
use DNS to discover the DNS server itself. Put its address in the initial file
policy/bootstrap data. Once the Visa Service and DNS adapter are connected, the
adapter's registered `dns.svc.zpr` service appears on the Control Room map under
the `zpr-dns` actor. Service records are then published from policy-authorized
actor registrations.

The `zpr.addr` pinned on the ZPL service class must exactly match the adapter's
ZPR interface address, BIND's `listen-on-v6` address, and the
`[dns_update].server` address.

## BIND

Install BIND 9 and its `nsupdate` utility on the DNS-service host/container.
Copy [`named.conf.example`](named.conf.example), replace the ZPR address,
client prefix, and zone file path. Generate one shared BIND-format TSIG key
file and include it in `named.conf` and the Visa Service config:

```sh
sudo install -d -m 700 /etc/bind/keys
sudo tsig-keygen -a hmac-sha256 zpr-vs-publisher \
  | sudo tee /etc/bind/keys/zpr-vs-publisher.key >/dev/null
sudo chmod 600 /etc/bind/keys/zpr-vs-publisher.key
```

Copy that key file to the Visa Service host using a protected channel and set
its owner/mode so only the Visa Service account can read it (`0600`). Do not
print or commit the key contents.

The BIND key name and secret must match the BIND-format TSIG key file configured for the Visa Service publisher.

The sample is authoritative-only: recursion and cache access are disabled.
Queries are allowed only from the ZPR client prefix, the socket binds only to
the ZPR service address, and the update policy grants the publisher A/AAAA
updates only within `svc.zpr`. Do not publish port 53 or TCP/UDP 953 on the
host/underlay. The sample zone is dedicated to dynamically registered service
names.

Create an initial primary zone file owned by the BIND user, for example:

```dns
$ORIGIN svc.zpr.
$TTL 30
@ IN SOA dns.svc.zpr. hostmaster.svc.zpr. (
  1 60 60 86400 30
)
  IN NS dns.svc.zpr.
```

Validate the BIND configuration with `named-checkconf` and the zone with
`named-checkzone svc.zpr /var/lib/bind/db.svc.zpr` before starting `named`.

## Visa Service

Add this section to the Visa Service `vs.toml` after installing the `nsupdate`
executable and the BIND-format TSIG key file. The configured server is BIND's
ZPR address, not an underlay address:

```toml
[dns_update]
server = "fd5a:5052:adda:1::53" # DNS service's ZPR address
port = 53
zone = "svc.zpr."
tsig_key_file = "/etc/zpr/dns-publisher.key"
ttl_seconds = 30
nsupdate_bin = "/usr/bin/nsupdate"
```

The publisher accepts only policy-declared service names inside the configured
zone. It derives records from actor ZPR addresses assigned during authenticated
admission; callers cannot choose record addresses or transport scopes. Startup,
actor join/leave, and policy-update events reconcile each owner RRset to the set
of currently connected, policy-authorized providers. Failed updates are logged;
they do not grant access or relax Visa Service decisions. Every flow to a
resolved provider still requires an allowed visa.

Use a dedicated, rotated TSIG key for this publisher. BIND `update-policy`
should grant only the needed record types and dedicated zone. TSIG protects
update authentication and integrity; DNS query confidentiality is supplied by
the ZPR flow, not by ordinary DNS.
