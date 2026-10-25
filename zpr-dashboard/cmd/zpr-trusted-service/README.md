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
the provider API. These empty-result/union rules describe the legacy file/generic
LDAP modes; immutable LDAP mode below requires exact positive UUID evidence.
The provider accepts at most 64 identities and a 64 KiB request.
Requests require an explicit `identities` array (`[]` is valid). Duplicate JSON
names, null values, unknown fields and trailing data are rejected before calling
the provider. File snapshots are limited to 1 MiB and reject duplicate names at
every level and null objects, arrays or array elements. Empty attribute arrays
remain valid; identity keys and subject values remain case-sensitive. Lookup
JSON nesting is limited to 64 levels.
Request field names are exact and case-sensitive: `identities`, `key` and
`value`. Case variants are not aliases and cannot overwrite canonical fields.
Both requests and file snapshots require valid UTF-8 and paired Unicode
surrogate escapes. Invalid bytes or lone surrogates fail instead of being
silently replaced and matching a different identity. Valid Unicode, including
an explicitly supplied replacement character, and valid JSON escapes remain
unchanged.
Invalid file snapshots return 503 without partial attributes; a corrected file
is read on the next lookup. The reference provider does not know canonical
mapping cardinality: VS still validates single-valued claims after mapping.

Outgoing lookup observations are validated and fully encoded before success
headers are written. Null attribute maps/value arrays and invalid UTF-8 in
provider-returned names/values fail with 503 rather than being normalized by
Go's JSON encoder. Empty objects/arrays, empty strings and valid Unicode remain
valid. Responses are limited to 1 MiB of encoded JSON including the trailing
newline, matching the VS consumer; JSON escaping and all claims count toward
that bound. This is a wire-size limit, not a bound on provider allocation.
Cancellation is checked before provider lookup and again before publication;
source validity is rechecked after encoding. No in-flight lookup/write
interruption guarantee is implied. Lookup successes and failures carry
`Cache-Control: no-store`, and response-write errors are logged.

The VS REST consumer rejects repeated JSON attribute names before mapping,
including identical repeats and names that become equal after JSON escape
decoding. Such responses are indeterminate, not last-value-wins observations.
Distinct provider names may still map to the same canonical attribute when
their values agree; conflicting mapped values fail the lookup.

Changes to the file or LDAP directory are visible at the provider's next lookup.
Visa Service refreshes an actor when its trusted attributes expire, or after an
admin cache flush changes the source revision. The lookup API has no push
invalidation or background polling: a change is not immediately reflected in
an unexpired actor record. Failed TTL and revision refreshes are indeterminate,
not evidence that an uncached attribute is absent.

### Immutable LDAP source configuration

An opt-in production mode separates policy-owned declarations from independently
provisioned directory transport and secrets:

```sh
go run ./cmd/zpr-trusted-service -listen 127.0.0.1:8443 \
  -cert /secure/provider.crt -key /secure/provider.key \
  -client-ca /secure/visa-ca.crt -source-config /secure/company-source.json \
  -attribute-validity 2m
```

Example `/secure/company-source.json` (provisional JSON, not policy syntax):

```json
{
  "source_id": "company",
  "subject_selector": "ldap_entry_uuid",
  "identity_key": "user.directory_uuid",
  "url": "ldaps://directory.example:636",
  "ca_file": "/secure/ldap-ca.crt",
  "base_dn": "ou=People,dc=example,dc=org",
  "bind_dn": "cn=reader,dc=example,dc=org",
  "bind_password_file": "/secure/company-bind-password",
  "attributes": ["cn", "mail"]
}
```

Source IDs are exact `[A-Za-z0-9_-]{1,64}`, excluding `default` and
`__source__`. Configure the corresponding VS transport under
`[trusted_service_http.company]`; the existing REST protocol does **not**
negotiate or verify this source-ID match. The TLS endpoint/CA and client
configuration must be independently approved. The provider does not register
itself, modify policy, or query an operator registry.

Configuration is read and pinned at startup; edits require restart. Config and
password files must be private (for example mode 0600), regular, single-link,
root/current-user-owned files. CA files may be publicly readable but must not
be group/other-writable. Symlinks are rejected. CA/password references must be
absolute paths. Passwords are bounded and reread before every bind, permitting
protected file replacement without caching a former password; missing/unsafe/
empty files fail explicitly. No inline secret, fallback credentials or system-CA
fallback is accepted. JSON rejects duplicate/null/unknown/case-variant fields,
trailing data, malformed Unicode and oversized configuration. Only 1-64 unique
lowercase LDAP attribute names are approved; credential-like names are rejected.
Legacy file/LDAP/watch modes remain unchanged and cannot be mixed with
`-source-config`. This mode does not add a directory watcher/change feed.

The request must contain exactly one identity with the configured, case-sensitive
key and a canonical lowercase nonzero UUID. Other identity keys are ignored,
never converted from CN, email, username or DN. Duplicate selected identities,
missing identities and malformed UUIDs fail before dialing. The provider issues
only an escaped equality filter `(entryUUID=...)`, subtree scope, no alias
dereference, size limit two and five-second LDAP bounds. Exactly one entry,
inside the configured base and with exactly one matching UUID, is required.
Missing/ambiguous subjects, duplicate attributes, referrals and partial results
return 503, never successful empty absence. No group expansion is performed.
Only approved fields are returned; structural UUID evidence is omitted unless
`entryuuid` is explicitly approved. Canceling an active search closes its LDAP
connection; dial/bind retain the existing five-second bounds.

**Identity boundary:** this mode expects an already authorized lookup identity.
It does not create a BAS-device/person association or activate reviewed subject
links. A raw configured identity key is not issuer-qualified proof. End-to-end
production use still requires approved active linking and its VS enforcement.

Tests use a local TLS LDAP protocol fixture behind a real mTLS HTTPS lookup,
plus controlled directory-result failures. They exercise bind failure, CA
failure, password rotation, cancellation, exact UUID evidence and response
allowlisting with Simulator unavailable. They are not real-directory or
VS-to-gateway deployment acceptance.

The opt-in [source lifecycle runner](../../scripts/test-source-lifecycle.sh)
also drives this provider with the actual Rust VS REST consumer and a real
signed policy compiled from two saved JSON documents sharing a source ID.
It tests allow/deny, durable disable and queued revocation, a private AOF
database/process restart, changed-data denial, bind failure and fresh recovery.
Simulator is unavailable throughout. Synthetic UUID-valued actor CNs are
explicitly provisioned as fixture lookup identities; this does not approve or
implement a device/person subject link. Source controls use the actual Admin
HTTPS listener and API-key checks, and the real event worker performs queued
reconciliation. Unauthorized/malformed mutations and an unrelated TLS trust
root are rejected. The VS runs production components in test processes, not
the full daemon or gateway transport. Lifecycle controls/readback also traverse
the actual Control-Service bridge and production Admin transport, with
server-owned credentials and explicit upstream-credential/trust failure recovery.
Its inbound CA-signed HTTPS fixture now applies real OIDC session, permission,
organization and same-origin/CSRF authorization, including logout rejection.
OIDC uses a signed-token fixture with actual TLS token/JWKS exchange, not a
production IdP or browser login. This is not deployed Control-Service mTLS
or full Control Room proxy acceptance. See
[prerequisites, reproduction and evidence boundaries](../../../MULTISOURCE_POLICY_SUPPORT.md#repeatable-source-lifecycle-acceptance).

### Optional source validity bound

With an updated VS consumer, start the provider with `-attribute-validity 2m` to
emit an absolute, whole-observation deadline:

```json
{"attributes":{},"expires_unix":1792879200}
```

`expires_unix` is UTC Unix seconds, not a duration. The example is illustrative;
the provider calculates the deadline at lookup start, including empty results.
If supplied, the field must be a non-null unsigned integer representing a future,
representable deadline. Explicit `null`, wrong types, overflow and expired
deadlines are indeterminate lookup failures, not a request for legacy behavior.
Malformed response JSON is also indeterminate and includes the source ID in the
VS error; no partial response attributes are installed.
A lookup that finishes after its deadline returns 503. The flag must be zero
(the default, preserving the legacy response shape) or at least one second.
Do not enable it against an older VS that rejects additional response fields.

Updated VS caps returned attributes and the whole observation at the earlier
of the source deadline and configured policy TTL. Empty observations retain
source-qualified freshness metadata, so absence-based decisions cannot outlive
the observation and expiry triggers another lookup. New visas are bounded by
both endpoints' observation deadlines; rechecks queue revocation when an
updated deadline is earlier than an existing visa's expiry. Delivery of that
revocation is asynchronous. Too-short validity can cause denial under VS's
minimum usable visa lifetime.

This is a freshness bound, not an authorization credential, snapshot revision,
proof of LDAP entry expiration, or push-invalidation guarantee. It assumes
synchronized clocks. When the field is omitted, the legacy configured TTL
behavior remains; this does not close legacy empty-result freshness gaps.
The two-identity policy-principal/VS authorization contract, source-side
per-principal field visibility and selective dependency invalidation are future
work, not implemented by this flag.

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
Lookup searches reject unresolved LDAP referrals and partial/error results;
they do not treat an incomplete search as known absence. Repeated requested
attributes (including LDAP case variants) also fail rather than picking one.
With group lookups enabled, each matching group must expose exactly one
nonempty `cn`. If a listed direct `role` and derived group roles disagree,
the lookup fails instead of overwriting the direct claim. Agreeing roles and
group-only roles remain valid. Cancellation is checked before and after
searches; this discards canceled results but does not interrupt an in-flight
LDAP search (the existing five-second LDAP timeout still applies).
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
The VS consumer requires HTTP 200 for a complete observation and does not follow
redirects, even to another path on the same origin. Configure the final provider
origin directly; proxy redirects are not a supported lookup contract. Accepted,
partial-content, multi-status, redirect and error responses are indeterminate,
even if their body resembles a valid observation. Empty successful results must
use HTTP 200 with `{"attributes":{}}`, not HTTP 204. The consumer checks the
1 MiB decoded-body bound with or without Content-Length, and rejects truncated
bodies and malformed Unicode instead of installing partial claims.
CA files may contain a PEM certificate bundle but must contain at least one
valid certificate. Empty or malformed CA material and malformed/mismatched
client certificates/keys fail VS store initialization. Isolated consumer tests
exercise a server that requires client certificates, checking the actual peer
certificate and lookup body. Wrong server roots/hostnames and missing,
wrong-root, expired or wrong-purpose client certificates fail before a lookup
reaches the server application. These tests do not establish certificate
rotation, revocation checking or a production CA lifecycle.
VS also rejects expired server certificates and certificates without the server
authentication purpose. TLS file-read failures identify the configured source,
file purpose and path. The existing five-second consumer request timeout covers
stalled TLS negotiation, response headers and incomplete response bodies; these
phases are exercised by isolated transport tests without shortening that timeout.
Additional consumer tests reject not-yet-valid server/client certificates,
accept complete intermediate chains and reject missing intermediates on either
side. IP-address origins require a matching IP SAN. Same-client request sequences
verify exact identity text, empty identity arrays and recovery from rejected,
invalid or interrupted responses without a flush/restart. These remain consumer
contract tests, not live directory, credential rotation or relay-delivery evidence.

## LDAP Change Feed

With `-ldap-uri`, add `-ldap-changes-base cn=accesslog` to serve `GET /v1/changes` on the
same mTLS listener. Callers poll it to learn which directory entries changed since their
last call. The feed reads the OpenLDAP `accesslog` overlay and returns metadata only:
attribute names, never values.

```text
GET /v1/changes                    -> {"changes":[],"cursor":"<head>","more":false}
GET /v1/changes?cursor=<c>&limit=N -> {"changes":[{"cursor","time","type","dn","new_dn"?,
                                        "entry_uuid"?,"attributes"?}],"cursor":"<next>","more":bool}
GET /v1/changes?since=<RFC3339|duration>&limit=N -> the first page from the inclusive lookback time
```

- With no cursor, the response is a baseline cursor. Do a full sync through `/v1/attributes`,
  then poll with the cursor you received. Always pass back the latest response `cursor`.
- A `since` bootstrap returns changes starting at that time, including other writes with the
  same accesslog second stamp. It is mutually exclusive with `cursor` and is intended for
  bounded history views that do not have a saved cursor. Duration lookbacks such as `24h`
  are calculated using the trusted-service clock and cannot exceed configured retention.
- `type` is `add`, `delete`, `modify`, or `modrdn`. `modrdn` includes `new_dn`.
  Operational attributes are omitted from `attributes`.
- `limit` is 1–500 (default 100). When `more` is true, poll again immediately.
- Writes become visible after `-ldap-changes-settle` (default 2s) plus up to one second.
- A cursor or `since` time older than `-ldap-changes-retention` (default 24h) whose entry was
  purged returns `410 {"error":"cursor_expired"}`. The caller must full-sync and request a
  new baseline, or restart a bounded history view from a time within retention.
  Keep the retention at or below the slapd `logpurge` age.
- Responses are `400` for bad parameters and `503` when the change log is unavailable.

The slapd configuration needs a separate accesslog database. Define it before the main
database and make it readable by the bind account:

```text
moduleload accesslog
database mdb
suffix "cn=accesslog"
directory /runtime/accesslog
index reqStart eq
access to * by dn.exact="<bind DN>" read by * none
database mdb
suffix "<base DN>"
...
overlay accesslog
logdb "cn=accesslog"
logops writes
logsuccess TRUE
logpurge 01+00:00 01+00:00
```

Imports done with `slapadd` are not logged. A directory reseed therefore needs a caller full
sync, not a change poll. The multinode demo directory (`zpr-demo/multinode-demo`) enables
this configuration.

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
Standalone process restart also starts a full refresh; the stdout mode has no
durable checkpoint or retained event queue. Control-Service now embeds this same
consumer with private per-organization/source durable checkpoints; see
[production LDAP push watcher](../../ASSERTIONS.md#production-ldap-push-watcher-b4).
Cookies advance after output succeeds; an output error
stops the process. SIGINT/SIGTERM cancels the stream. A blocked output pipe
requires its reader to drain or close it; supervisors must bound shutdown.

The simulator bootstrap starts a separate watcher alongside the lookup APIs
and writes private `demo-directory/changes.jsonl` and `change-consumer.log`
files. These files are demo diagnostics without rotation, not a production
event journal. The standalone metadata stream does not refresh cached ZPR
attributes, evaluate
assertions, or revoke existing visas. The independent Control-Service supervisor
can now opt into approved-attribute refresh and the existing Visa Service cache
invalidation/reconciliation API; the stdout/demo stream remains diagnostic only.

Run unit tests with `go test ./cmd/zpr-trusted-service -run TestLDAPSync`.
On Linux with OpenLDAP and openssl installed, the following creates an isolated
temporary LDAPS server and verifies refresh plus add/modify/delete:

```sh
ZPR_TEST_LDAP_SYNC=1 go test ./cmd/zpr-trusted-service -run TestLDAPSyncLiveOpenLDAP -count=1
```
## Evaluation bootstrap boundary (B1)

The [local evaluation launcher](../../packaging/evaluation/README.md) starts an
independent, empty starter OpenLDAP directory. B1 does not register it with this
provider or start its change watcher; those are B2/B4 tasks, not evidence that
attributes refresh or visas revoke. Control-Service owns opt-in durable
per-organization/source watcher checkpoints
and resync health independently of bootstrap; B4 does not add registration or
start a watcher without explicit production source configuration. Simulator is
neither required nor started
and must keep its synthetic directory separate.
