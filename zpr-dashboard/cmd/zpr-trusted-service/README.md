# Trusted-service REST provider

This command exposes one read-only, mutually authenticated HTTPS endpoint:

`POST /v1/attributes` with `Content-Type: application/json`:

```json
{"identities":[{"key":"device.zpr.adapter.cn","value":"alice"}]}
```

Response: `{"attributes":{"color":["red"]}}`. Names in `attributes` are **source-side**
names; Visa Service applies the policy's `returns_attributes` mappings. Unknown identities
return an empty object. All matching identities contribute attributes; conflicting values
fail the lookup rather than silently choosing one. Unavailable sources return non-2xx;
Visa Service must treat these as indeterminate. Neither request nor response is cached by
the provider API. The provider accepts at most 64 identities and a 64 KiB request.

Changes to the file or LDAP directory are visible at the provider's next lookup.
Visa Service refreshes an actor when its trusted attributes expire, or after an
admin cache flush changes the source revision. The lookup API has no push
invalidation or background polling: a change is not immediately reflected in
an unexpired actor record. On a failed refresh, expired attributes cannot
authorize access; a revision-stale lookup is treated as indeterminate.

Run a file-backed provider using the same identity-keyed JSON format as the built-in
Visa Service file source:

```sh
go run ./cmd/zpr-trusted-service -listen 127.0.0.1:8443 \
  -cert /secure/provider.crt -key /secure/provider.key -client-ca /secure/visa-ca.crt \
  -file /secure/actors.json
```

For example, `actors.json` can contain:

```json
{"device.zpr.adapter.cn":{"alice":{"color":["red"]}}}
```

For LDAPS, replace `-file` with `-ldap-uri ldaps://directory.example:636`,
`-ldap-ca /secure/ldap-ca.crt`, `-ldap-base 'dc=example,dc=org'`,
`-ldap-bind 'cn=reader,dc=example,dc=org'`, `-ldap-password-file /secure/bind-password`,
`-ldap-identities '{"user.sub":"uid"}'`, and `-ldap-attributes 'department,role'`.
Only listed person attributes are returned. To resolve `groupOfNames` membership, also set
`-ldap-groups-base 'ou=Roles,dc=example,dc=org'`. The provider searches that subtree for
groups whose `member` matches the person's DN and returns each matching group's `cn` in the
multivalued source-side `role` attribute. Map it with `role -> device.demo.role` in the
Visa Service policy. Omitting the flag disables group lookups. LDAP identity values are
escaped in equality filters; attribute names are restricted to LDAP-safe names. The bind
account should have read-only access, and its password file must be readable only by the
service account.
LDAP connectivity requires a reachable TLS-enabled directory; the automated
default tests cover the filter and provider contract. An opt-in change-stream
test starts an isolated OpenLDAP directory; see below.

Visa Service policy declares `api = "rest/1"`, mapped `returns_attributes`, and a TTL
over 60 seconds. The Visa Service config separately supplies the URL and TLS material:

```toml
[trusted_service_http.example]
url = "https://127.0.0.1:8443"
ca_cert = "/secure/provider-ca.crt"
client_cert = "/secure/visa-client.crt"
client_key = "/secure/visa-client.key"
```

Server verification is pinned to `ca_cert`; a valid Visa Service client certificate is
required. Do not expose this endpoint publicly or store private keys in the repository.

## LDAP Change Consumer

`-ldap-watch` is a separate read-only RFC 4533 consumer, not an HTTPS server:

```sh
go run ./cmd/zpr-trusted-service -ldap-watch \
  -ldap-uri ldaps://directory.example:636 -ldap-ca /secure/ldap-ca.crt \
  -ldap-base 'dc=example,dc=org' -ldap-bind 'cn=reader,dc=example,dc=org' \
  -ldap-password-file /secure/bind-password
```

On OpenLDAP, load `syncprov` before configuring the database, then include
`scripts/ldap-syncprov.conf` in that database's configuration. Validate with
`slaptest` before restarting LDAP. Enable it without recreating or reseeding
the database. The bind account must remain read-only and scoped to the intended
directory; TLS server identity is verified against the configured CA.

For an existing single-database `slapd.conf`, run
`sh scripts/enable-ldap-sync.sh /path/to/slapd.conf` on the LDAP host. It validates
the new configuration and preserves a private backup; it does not restart LDAP
or alter directory contents. The next LDAP-only restart applies the overlay.

The consumer emits version-1 JSON lines on stdout containing `base_dn`, `type`,
and, for entry events, `uuid` and sometimes `dn`. It requests no attribute
values (`1.1`); passwords, attribute contents, bind credentials, and sync
cookies are not published. DNs are identifying data: keep output private.
This is a source-side metadata stream, not yet a shared ZPR trusted-source API.

Events include `refresh_start`, `resume_start`, `present`, `add`, `modify`,
`delete`, `refresh_complete`, `disconnected`, and `resync_required`. Initial
entries can arrive as `add` or `present`; do not treat them as new live writes.
Refresh-present UUID sets are not deletion sets. A refresh-complete event's
`refresh_deletes` flag identifies the RFC refresh strategy. Delete sets may
have a UUID without a DN. Consumers must follow RFC 4533 refresh reconciliation
and deduplicate by UUID; this is not an exactly-once audit log.

Connections resume with an in-memory cookie after disconnects, with retries
backing off from one to 30 seconds. Rejected cookies trigger a new refresh.
Process restart also starts a full refresh; there is no durable checkpoint or
retained event queue. Cookies advance after output succeeds; an output error
stops the process. SIGINT/SIGTERM cancels the stream. A blocked output pipe
requires its reader to drain or close it; supervisors must bound shutdown.

The simulator bootstrap starts a separate watcher alongside the lookup APIs
and writes private `demo-directory/changes.jsonl` and `change-consumer.log`
files. These files are demo diagnostics without rotation, not a production
event journal. This phase does not refresh cached ZPR attributes, evaluate
assertions, or revoke existing visas. Those consumers are a subsequent
trusted-source integration step.

Run unit tests with `go test ./cmd/zpr-trusted-service -run TestLDAPSync`.
On Linux with OpenLDAP and openssl installed, the following creates an isolated
temporary LDAPS server and verifies refresh plus add/modify/delete:

```sh
ZPR_TEST_LDAP_SYNC=1 go test ./cmd/zpr-trusted-service -run TestLDAPSyncLiveOpenLDAP -count=1
```