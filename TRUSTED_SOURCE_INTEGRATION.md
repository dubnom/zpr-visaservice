# Trusted-source integration guide

This guide is for provider implementers and operators connecting an external
attribute source to Visa Service (VS). It describes the implemented `rest/1`
lookup contract and current configuration boundaries. Examples target this
repository's current consumer, not an independently standardized public API.

**Status:** the lookup protocol is implemented and contract-tested. Source
controls, policy-document composition and immutable LDAP configuration are
implemented but provisional. Local acceptance is not production deployment
qualification. Envelope-based delegated source access is future work.

The implementation baseline for this guide is `1bd69c9` (2026-10-25), including
the opt-in subject-link registry and review-only operator workflow. Maturation
continues independently; uncommitted work is not a contract described here.
These registries are not provider self-registration or a new lookup protocol.

## 1. Ownership and trust boundaries

| Surface | Owner | Purpose |
|---|---|---|
| Effective policy source declaration | Policy publisher | Source ID, API, TTL and source-to-canonical attribute mappings |
| VS HTTPS transport configuration | Deployment operator | Provider origin, server trust and VS client credentials |
| Provider configuration | Source operator | Backing store, approved attributes, directory trust and read-only credentials |
| Runtime source control | Authorized VS operator | Durable enable/disable of a policy-declared source |
| Directory/subject-link registration | Separate operator/review contracts | Inventory and identity-link authority; not a replacement lookup declaration |

Effective usability requires both an effective policy declaration and an enabled
source. A registration, enabled toggle, reachable endpoint or TLS certificate
alone does not create policy authority. A provider does not self-register or
publish policy through `rest/1`.

Control Room and Control-Service use independently configured production
contracts and must work with Simulator unavailable. Simulator may act as a
client of these services; it is not an integration prerequisite.

Keep these identities separate:

- **Source ID:** the stable policy reference, such as `company`; provision the
  corresponding transport under the exact same ID.
- **Lookup subject:** the actor identity whose attributes are requested.
- **VS transport client:** the service authenticated by the provider's mTLS
  listener. This is not the subject or a human policy author's credential.
- **Future policy-associated principal:** delegated access context not present
  in the current request. Do not invent an unsigned field to stand in for it.

The lookup wire format contains no source ID, issuer, policy ID, requested-field
projection or delegation proof. An endpoint's association with a source ID is
therefore an independently approved configuration binding, not wire negotiation.
The current runtime control ID scope is a VS deployment/state database, not a
new global or cross-tenant identity namespace.

## 2. Implementing the lookup endpoint

### Transport

Serve `POST /v1/attributes` over HTTPS with mandatory client-certificate
authentication. The reference provider trusts the configured client CA; issue
and scope those client certificates appropriately. A trusted CA certificate is
not itself a per-policy authorization system.

VS configuration accepts an HTTPS **origin** only: no user information,
non-root path, query or fragment. It appends `v1/attributes` itself. Configure the
final origin; redirects, including same-origin redirects, are not followed.
Server chain, validity, purpose and hostname/IP SAN must verify against the
explicit configured CA bundle. VS uses its configured certificate/key pair,
not browser credentials or a system-root fallback.

The current consumer has a five-second request timeout, covering TLS negotiation,
headers and body. Do not depend on an automatic retry: the next actor refresh
or explicit operation is separate from this request.

### Request

```http
POST /v1/attributes HTTP/1.1
Content-Type: application/json

{"identities":[{"key":"device.zpr.adapter.cn","value":"alice"}]}
```

Reference-provider rules:

- Require exactly `Content-Type: application/json`; parameters such as
  `charset=utf-8` are not accepted by this implementation.
- Require an explicit `identities` array. `[]` is structurally valid; a provider
  mode may require a particular identity.
- Accept at most 64 entries and 64 KiB of request bytes.
- Each entry contains exactly nonempty string fields `key` and `value`.
  Identity text is case-sensitive; do not normalize it into a different subject.
- Reject unknown/case-variant fields, duplicate JSON member names, nulls,
  trailing data, invalid UTF-8 and unpaired Unicode surrogate escapes.
  The reference JSON validator permits at most 64 levels of nesting.

VS constructs this set from policy-designated identity keys intersected with
unexpired actor attributes; arbitrary returned claims are not lookup identities.
Do not interpret a CN, email or display name as an issuer-qualified person link.

### Successful observation

Return exactly HTTP **200**, with a JSON object:

```json
{"attributes":{"department":["Finance"],"role":["approver","employee"]}}
```

Keys are **provider-side names**, not canonical policy names. Values are arrays
of strings. Unmapped names are ignored by VS, but still count toward body limits.
Return only attributes the provider is authorized to disclose.

The response may additionally include `expires_unix`; no other top-level
fields are accepted by the current consumer. `attributes` is required. Reject
duplicate member names, including escape-equivalent attribute names. Null maps
or value arrays, wrong types, malformed Unicode and truncated JSON are invalid.
Return one complete observation, not streaming partial claims.

The consumer accepts at most **1 MiB of body bytes**, checking Content-Length
when available and accumulated chunks regardless of that header. The reference
provider bounds encoded JSON including its trailing newline.

Different source-side names may map to one canonical attribute only when their
value arrays agree. Conflicting mappings fail the lookup. A single-valued
canonical attribute requires exactly one value; an empty or multiple-value array
is not valid for that mapping. Multivalued and tag mappings have their own
compiler/consumer semantics; do not infer cardinality from sample JSON.

### Absence versus failure

| Outcome | Wire result | Meaning to VS |
|---|---|---|
| Complete, known empty observation | `200 {"attributes":{}}` | Successful absence, subject to mode and freshness constraints |
| Complete nonempty observation | 200 with valid attributes | Install mapped claims only after validating the whole response |
| Source unavailable, ambiguous or incomplete | Non-200, typically 503 | Indeterminate; not successful absence |
| Invalid request to reference provider | 400; 415 for content type | No valid observation |
| Redirect, 202, 204, 206, 207 or other non-200 | Any body | Indeterminate, even with success-looking JSON |
| TLS/timeout/body/decode/mapping failure | No accepted observation | Indeterminate; no partial attributes installed |

Legacy file/generic LDAP modes union matching identities and return empty
attributes for unknown identities; conflicting values fail. Immutable LDAP mode
instead requires positive exact-UUID evidence and returns failure for missing
or ambiguous subjects. Never substitute empty attributes for a directory outage.
Reference lookup responses, including failures, carry `Cache-Control: no-store`;
do not add intermediary response caches.

### Source-controlled validity

An updated provider may return:

```json
{"attributes":{"department":["Finance"]},"expires_unix":2000000000}
```

The timestamp is illustrative. Calculate a future UTC Unix-seconds deadline
for the whole observation; do not copy a fixed example into a live provider.
It must be a non-null unsigned integer, future and representable at VS.
Explicit null, expired values, overflow and wrong types fail.

VS caps validity at the earlier of this deadline and the configured policy TTL.
With this field, empty observations also carry freshness metadata: absence
cannot outlive the observation. Newly issued visas are bounded by the endpoint
dependencies; earlier validity discovered on recheck can queue revocation.
Revocation delivery is asynchronous, and too-short validity can prevent issuance
under the minimum usable visa lifetime. Synchronize clocks.

Omitting the field retains legacy TTL behavior, including legacy empty-result
freshness gaps. The reference provider defaults to omission; enable
`-attribute-validity 2m` only with a consumer supporting this field. It calculates
the deadline at lookup start and rejects a result that finishes too late.
This is not a credential, directory-entry expiration or push-invalidation proof.

## 3. Declaring and provisioning a source

### Policy declaration

An existing compiler configuration can declare:

```toml
[trusted_services.company]
api = "rest/1"
expiration_seconds = 600
returns_attributes = ["department -> user.department", "role -> user.role{}"]
```

Use mappings valid for the deployment's attribute catalog and cardinality.
The REST consumer requires TTL **greater than 60 seconds**. Declaration defaults
that produce a zero TTL do not make a usable REST integration.

Policy-Service also supports provisional JSON documents inside **one saved
policy revision**:

```json
{
  "documents": [
    {
      "id": "directory-definitions",
      "source": "define Finance as user with user.department__source__company:'Finance'.",
      "trusted_sources": {
        "company": {
          "api": "rest/1",
          "expiration_seconds": 600,
          "returns_attributes": ["department -> user.department"]
        }
      }
    }
  ]
}
```

This is authoring input, not the provider HTTP request or final envelope syntax.
Exact repeated declarations share a source ID; different API, TTL or ordered
mappings conflict with other documents or the independent base config.
There is no last-writer-wins override. Limits include 1-64 unique documents,
1 MiB JSON and 16 effective nondefault sources. This does not compose arbitrary
catalog records or fetch remote includes. JSON-specific formatting, completion
and assistant editing are not implemented; see the
[composition contract](MULTISOURCE_POLICY_SUPPORT.md#operator-controls-shared-json-documents-and-immutable-ldap-provider).

Use source-qualified attributes where policy must name a particular authority.
Unqualified resolution must not silently choose between competing sources.
Declaration/composition does not authorize a person/device association.

### Independent VS transport

```toml
[trusted_service_http.company]
url = "https://attributes.example.test:8443"
ca_cert = "/secure/provider-ca.pem"
client_cert = "/secure/vs-client.pem"
client_key = "/secure/vs-client-key.pem"
```

Provision these files independently of editable policy. Use real certificates
whose SAN matches the origin, a CA bundle with at least one valid certificate,
and a matching client key. Do not expose secrets in policy, browser state,
logs or source control. TLS configuration is loaded when constructing the
store; do not assume editing a file hot-reloads an existing TLS client.
Certificate rotation/revocation lifecycle is not established by lookup tests.

### Reference provider

From `zpr-dashboard`, a file-backed integration fixture can run as:

```sh
go run ./cmd/zpr-trusted-service -listen 127.0.0.1:8443 \
  -cert /secure/provider.pem -key /secure/provider-key.pem \
  -client-ca /secure/vs-client-ca.pem -file /secure/actors.json \
  -attribute-validity 2m
```

Example backing data:

```json
{"device.zpr.adapter.cn":{"alice":{"department":["Finance"]}}}
```

For independently provisioned exact-UUID LDAP use `-source-config`, not the
legacy generic lookup flags:

```json
{
  "source_id": "company",
  "subject_selector": "ldap_entry_uuid",
  "identity_key": "user.directory_uuid",
  "url": "ldaps://directory.example.test:636",
  "ca_file": "/secure/directory-ca.pem",
  "base_dn": "ou=People,dc=example,dc=test",
  "bind_dn": "cn=reader,dc=example,dc=test",
  "bind_password_file": "/secure/directory-password",
  "attributes": ["mail", "department"]
}
```

This requires exactly one selected identity containing a canonical lowercase
nonzero UUID, exactly one matching entry inside the configured base and
allowlisted returned attributes. It does not derive that identity from a
username/CN/DN or establish an active subject link. Config is pinned at startup;
protected password contents are reread before binds. The source ID must match
the independently approved VS binding; the protocol does not verify that match.
Protected-file, allowlist, search and failure details are in the
[provider runbook](zpr-dashboard/cmd/zpr-trusted-service/README.md#immutable-ldap-source-configuration).

## 4. Operational controls and changes

VS offers `GET /admin/trusted-services` and
`GET`/`PUT /admin/trusted-services/{id}/control`, using its Admin API keys.
PUT accepts exactly `{"enabled":false}` or `{"enabled":true}` and requires
read/write permission; GET requires read permission. A successful PUT is 202
after persistence and local application, not proof of completed reconciliation
or gateway delivery.

Control Room proxies runtime inventory/control through
`/api/trusted-sources/runtime` and
`/api/trusted-sources/runtime/{id}/control`. Named operators need explicit
`trusted-source.read` and, for mutation, `trusted-source.control`, plus normal
organization and CSRF checks. The browser never receives the server-owned
Admin credential. These controls are deployment-wide, not tenant-local toggles.

Read back `enabled`, `declared`, `effective_enabled`, `pending` and `revision`.
Revision is an opaque acceptance ID, not a monotonic policy version or approval
receipt. Disabled sources fail indeterminate rather than supplying absence.
Overrides survive declaration removal/re-add and ordinary state-preserving
restart; database clearing destroys them. Failed enable can remain desired-on,
effective-off and pending. Retry/read back explicitly; never report delivery
from acceptance alone. See the
[durable-control contract](MULTISOURCE_POLICY_SUPPORT.md#provisional-production-api).

The lookup endpoint does not push invalidation. Ordinary source changes are
observed on the next lookup after expiration or revision invalidation.
Generic LDAP mode can separately expose metadata-only `GET /v1/changes` using
OpenLDAP accesslog, but immutable `-source-config` mode does not expose this feed.
VS lookup does not automatically subscribe to it.

For feed consumers: obtain a baseline cursor, perform a full sync, then poll
from that cursor; preserve the latest cursor and drain `more:true` pages.
`limit` is 1-500 (default 100). `since` is an alternative bounded-history
bootstrap, not compatible with `cursor`. Expired history returns
`410 {"error":"cursor_expired"}` and requires resync. Metadata can contain DNs
and UUIDs: it is not public data. RFC 4533 watch mode is another separate
source-side stream, not this HTTP protocol. See the
[change-feed runbook](zpr-dashboard/cmd/zpr-trusted-service/README.md#ldap-change-feed).

## 5. Integration qualification and troubleshooting

### Separate subject-link registry and review workflow

The opt-in Control-Service review store has organization-scoped authority and
source registrations. They describe approved review context, not the effective
VS policy source set or provider transport. Their version strings and coherent
registry-context digest are not the runtime source-control acceptance revision.

| Operation | Endpoint | Required permission |
|---|---|---|
| Create/edit/disable authority or source registration | `PUT /api/subject-link-registry/{authorities\|sources}/{id}` | `subject-link.registry.manage` |
| Read current or immutable historical registration | `GET /api/subject-link-registry/{authorities\|sources}/{id}` or `.../{id}/versions/{version}`, with `organization_id` query | `subject-link.read` |
| Capture exact enabled authority/source versions | `POST /api/subject-link-registry/context` | `subject-link.read` |
| Propose a candidate link | `POST /api/subject-link-reviews/proposals` | `subject-link.propose` |
| Read a scoped review | `GET /api/subject-link-reviews/{id}?organization_id=...` | `subject-link.read` |
| Independently approve a review | `POST /api/subject-link-reviews/{id}/approve` | `subject-link.approve` |

Grant permissions separately; registry management does not imply proposal,
approval or read access. Requests require a current named Room session,
organization authorization and, where applicable, Origin/CSRF checks. The
backend independently checks fresh exact-request delegation proofs, pinned Room
mTLS identity and its own grants. This operator proof is **not** the future
policy-principal proof for `/v1/attributes`.

Registration writes use an exact `expected_version`: empty for create, current
decimal version for edit. They reject `Idempotency-Key`. After response loss,
read current state and immutable audit history before deciding whether another
edit is needed; a stale CAS returns 409. No deletion or renaming is provided:
retire through an audited disable. Registering a source does not install its
protected provider configuration; exact source-version provider mappings must
be provisioned separately and the service reconstructed.

Review mutations instead require a semantic `Idempotency-Key`, an exact body
and fresh authorization on every attempt. An approver must differ from the
persisted proposer by verified issuer/subject. Historical receipts do not
refresh authority. The operator page `/subject-links.html` provides the
registration/context/proposal/independent-review workflow with explicit
readback and retry handling.

**Approval remains review-only:** it neither activates a person/device binding
nor publishes a signed VS snapshot. Active publication, snapshot timing and
VS enforcement are separate gates. The full request fields, configuration,
limits and acceptance evidence are in the
[operator review contract](zpr-dashboard/README.md#subject-link-registry-administration)
and [operator page runbook](zpr-dashboard/README.md#subject-link-review-operator-page).

### Qualification checklist

Before enabling traffic:

1. Approve the source ID, identity provenance, mappings/cardinality, TTL, endpoint
   and disclosed fields. Verify that transport trust has not rebound an existing
   identity to another authority.
2. Verify server SAN/chain/validity and client authentication without disabling
   TLS verification. Confirm unauthorized clients never reach the application.
3. Test known, unknown, empty, ambiguous and conflicting observations for the
   chosen mode. Test malformed JSON, duplicate names, wrong status, redirects,
   oversize/truncated bodies, timeout and fresh recovery after failure.
4. Test expiry of positive and negative observations, expired lookup identities,
   disable/re-enable readback and persistence across restart.
5. Check policy composition conflict rejection and source qualification.
   Test with Simulator unavailable and its configuration unset/invalid.
6. Verify reconciliation and downstream delivery separately; keep production
   directory, certificate rotation, restart and deployment qualification explicit.

| Symptom | First checks |
|---|---|
| Store initialization fails | HTTPS origin, TTL >60s, source-ID transport entry, readable valid CA/cert/key |
| TLS lookup fails | SAN, trust chain, certificate validity/purpose, client CA, network reachability |
| HTTP 400/415 | Exact content type, identity fields, limits and encoding |
| HTTP 503 in immutable LDAP mode | Protected config/password, bind/search trust, exact UUID and one in-scope entry |
| Indeterminate mapping | Canonical cardinality, duplicate names, conflicting values, reserved names |
| Data appears unchanged | Unexpired actor observation, configured validity, revision refresh; no automatic push |
| Disabled or pending after enable | Desired/effective readback, durable store and worker errors; do not clear state to hide the failure |

Useful evidence and commands:

- [Rust consumer](vs/src/trusted_services/http_attribute_store.rs) and its
  adjacent TLS/recovery/parity tests define actual consumer behavior.
- [Provider implementation](zpr-dashboard/cmd/zpr-trusted-service/main.go) and
  [runbook](zpr-dashboard/cmd/zpr-trusted-service/README.md) define reference modes.
- From `zpr-dashboard`, `go test ./cmd/zpr-trusted-service` checks the provider;
  `cargo test -p vs http_attribute_store` from the repository root checks the
  consumer (confirm package/toolchain prerequisites in the workspace manifest).
- The opt-in [lifecycle runner](zpr-dashboard/scripts/test-source-lifecycle.sh)
  builds and exercises actual signed policy, VS, mTLS provider and a TLS LDAP
  fixture with isolated durable state. Its browser mode also tests real
  OIDC/operator/control transport. Read its prerequisites before running;
  it is local acceptance, not a production deployment test.

## 6. Explicitly not provided by this protocol

No per-policy/principal delegation proof, source-side per-principal field
visibility, generic subscription protocol, selective dependency invalidation,
global source registry, automatic registration or envelope inheritance is
defined here. An mTLS-authenticated VS is not proof that an arbitrary policy
principal may read every source field. Future changes must authenticate both
service and delegated access context, with source-side authorization and
context-safe caching. See the
[deferred requirements](MULTISOURCE_POLICY_SUPPORT.md#future-requirement-policy-associated-trusted-source-access-identity).
