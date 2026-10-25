# visa service

ZPR visa service implementation (under active development).

For the B1 **evaluation-only**, Simulator-independent local operator/LDAP/logging
bootstrap, see [the one-command evaluation runbook](zpr-dashboard/packaging/evaluation/README.md).
This is not the complete production appliance or source-registration milestone.

On a state-preserving restart, retained node identities are not evidence of a
live node session. Policy-authorized peer bootstrap visas remain available until
the peer authenticates in the current Visa Service process, allowing remote
nodes to reconnect without clearing the organization database.
Live-session tracking follows the RPC handle lifetime, including soft reconnects.
A failed VSS connection also invalidates its associated session, so half-open
VSAPI sockets cannot suppress restart bootstrap. Failure of an older VSS worker
or dropping an older RPC handle cannot clear a newer session. Bootstrap requests
check session liveness even when both endpoint identities remain in the database.
Bootstrap visas do not reuse
retained ephemeral A2A keys: those keys must be learned again through authentication.
Bootstrap transport retains authenticated hop encryption and the VSAPI TLS session.
A fresh node reset revokes that node's previous non-bootstrap visas on the live relays before
issuing replacements; clearing only the restarting node's references would leave
relays matching obsolete VSS visas and ephemeral keys.
Key-free bootstrap copies remain on live relays to preserve the VSAPI connection
used to authenticate the restarting node; its own copies are queued for reinstall.
Cached bootstrap visas missing any relay's installation reference are replaced,
so a relay reset cannot strand a future peer with an undeliverable visa.
Nodes may also pull visas by ID while their own delivery state is
`PendingInstall`, not just `Installed`. This lets a relay bind a VSS flow before
the reverse VSS channel exists, rather than requiring that channel to deliver
its own prerequisite. Pulled copies are actualized for the requesting node;
off-path, expired, pending-revoke and revoked visas are not returned. A successful
reply uses the existing installation-state transition, which cannot override a
concurrent revocation. The installed-visa inventory remains installed-only.

Node RPC pings request a reconnect when the session's authenticated identity
has less than one minute remaining. The node's existing ping-failure/reconnect
path performs a fresh signed challenge with retained-state semantics. An idle,
healthy RPC socket must not keep expired node authentication alive while Visa
deliveries to that node fail. Expired and near-expiry sessions receive
`AuthError`; this does not extend credentials without reauthentication.

See [Features Added Since the Upstream Fork](FEATURES_SINCE_ZPR_ORG.md) for a
high-level overview of the operator, policy, trusted-service, DNS, observability,
and simulator capabilities added in this fork.

Policy test fixture generation uses explicit operator configuration and live
LDAP, without loading Simulator manifests or profiles. Simulator provisioning
exports its fictional metadata into the same production input contract. See the
[fixture configuration contract](zpr-dashboard/cmd/zpr-web-dashboard/README.md#independent-fixture-configuration);
this incremental input boundary does not complete the application/package split.

For engineering coordination, see the workspace
[component work/status index](../COMPONENT_WORK.md) and
[agent team charter](../AGENT_TEAM_STARTER.md). The
[source reorganization plan](REDIRECTORY.md#inventory-and-target-layout-2026-10-14)
records the current inventory, verified incremental extractions and remaining
milestone gates; the full application/package split is not complete.

See [NATTY: session-scoped address indirection](NATTY.md) for a proposed,
not-yet-implemented design for adapter-local address aliases and their session,
visa, and replay-protection requirements.

See the [system packaging and release plan](System%20release%20plan.md) for
planned clean-company installation milestones and the recurring qualification,
signing, publication, rollout and recovery process for real system releases.
The plan is not a claim that a production installer is currently available.

See the [Simulator as an ordinary ZPR client plan](SIMULATOR_AS_ZPR_CLIENT_PLAN.md)
for a proposed demo-first task list, milestones, visual acceptance tests,
multi-source/organization LDAP work and tracked pre-production shortcuts.

Trusted-source cache refresh (`DELETE /admin/services/{id}/cache`) returns 202
only after reconciliation is recorded in the state database. This acknowledges
acceptance, not completion or relay revocation delivery. Pending generations
survive Visa Service worker/process restarts and are retried every five seconds;
duplicate refreshes coalesce and repeat the existing convergent reconciliation.
`GET` on the same authenticated resource reports each source's pending/failed
state and requested/completed generations. Database write failures return 503;
operators must configure durable Redis persistence for the state database.

Trusted-source TTL-refresh failures, like revision-refresh failures, block visa
issuance until the source can be refreshed. The evaluator cannot interpret an
unavailable source as proof that a different, uncached attribute is absent.
Existing-visa rechecks also refresh both actors and queue revocation on
indeterminate attributes; relay delivery remains asynchronous. This is the
conservative whole-actor behavior, not selective per-policy outage handling.
See [multi-source policy support](MULTISOURCE_POLICY_SUPPORT.md) for approved
subject-link and source-dependency enforcement work that remains open.

Policy declarations own the Visa Service's trusted-source set. Each source is
default-on unless its ID has a stored operational override. The provisional
admin JSON interface is `GET`/`PUT /admin/trusted-services/{id}/control`;
PUT accepts exactly `{"enabled":false}` or `{"enabled":true}` and requires a
read/write admin key. GET requires a read-capable key. Unknown, never-declared
IDs return 404; retained overrides remain readable/editable after policy removal.
Disable keeps the declaration and source namespace but makes lookups
indeterminate, denies new dependent requests and queues existing-visa revocation
through the reconciliation worker. Re-enable reloads before lifting the gate;
failed reloads cannot expose old cached claims. Off survives policy replacement,
source removal/re-addition and state-preserving VS restart.

Control readback separates desired `enabled` from `declared`,
`effective_enabled`, an opaque `revision`, and durable `pending` work. A source
with no override reports `revision: ""`. PUT returns 202 only after desired state
and pending work are stored together and the local gate is applied. GET on
`/admin/trusted-services` also includes each declared source's effective
`enabled` flag and `disabled` health. Storage/reload failures return 503, may
leave a conservative local gate off, and require readback/retry; they are not
successful toggle acknowledgments. Pending work retries every five seconds
without relying on the in-memory notification. Neither 202 nor `pending:false`
proves gateway delivery. With live actors depending on an off source, the
conservative unresolved refresh can keep `pending:true` even after revocation
is queued.

These controls use the existing per-deployment VS state database/instance lease,
not a global or cross-organization ID namespace. Redis persistence must be
configured; malformed/unreadable controls fail VS startup before policy
activation. Existing policy/store validation still applies even when off
(for example, a missing file snapshot can still prevent policy installation).
Credentials/CA/REST transport settings remain independently provisioned; they
are not supplied by the toggle. Control Room now exposes runtime controls through
its authenticated production Control-Service bridge, with explicit
`trusted-source.read` and `trusted-source.control` grants. Policy-Service supports
provisional JSON documents with identical shared-ID declarations and rejects
conflicts. The reference provider accepts independent protected JSON config for
exact immutable LDAP UUID lookup over the existing mTLS REST contract.
The opt-in [source lifecycle acceptance runner](zpr-dashboard/scripts/test-source-lifecycle.sh)
now connects saved shared-ID JSON policy, the real compiler, the VS REST consumer
and an mTLS provider backed by a TLS LDAP protocol fixture. It proves compiled
allow/deny, durable source-off and queued revocation across real database/process
restarts, changed-data denial, outage handling and fresh recovery, with Simulator
unavailable. Toggles and readback cross the production authenticated Admin HTTPS
listener; the real event worker handles reconciliation. Rejected credentials,
malformed toggles and an unrelated TLS trust root are checked. Lifecycle
mutations and readback now also cross the actual Control-Service bridge using
its production Admin transport and server-owned credentials; bad upstream
credentials/trust fail explicitly and correct configuration recovers.
This uses
synthetic actor identities inside test processes, not the full daemon/node
authentication/Control-Service ingress/Control Room flow. The bridge's inbound
test listener now uses CA-signed HTTPS and real OIDC-derived operator sessions
with production permission, organization and same-origin CSRF checks, including
logged-out-session rejection. OIDC uses an isolated signed-token provider and
handler-driven login/callback requests in the default browser-free run.
Runtime requests now cross the production Control Room reverse proxy and
file-configured mTLS transport to a separate TLS 1.3 service listener.
Missing/foreign client certificates fail TLS, browser credentials are stripped,
and unrelated service trust fails before the bridge. This is local transport
acceptance, not full Control-Service startup or deployed ingress acceptance.
With `ZPR_TEST_SOURCE_BROWSER=1`, real Chromium also runs the embedded production
login/request/source-control assets against those live APIs: cancel confirmation,
disable/readback and re-enable precede fresh VS recovery. Chromium starts
without a session, navigates real OIDC login/fixture consent/callback, logs out,
proves the old cookie/CSRF cannot mutate state, then signs in with a new session
and unchanged source revision. The IdP selects a named synthetic identity; it
does not test production password/MFA authentication. The browser uses a minimal
component page rather than the full Control Room. Source-only operators are
recognized by the login widget without unrelated monitor/policy/gateway grants.
It also confirms a service-TLS failure displays an error and disables stale
controls; restoring trust and refreshing recovers the same source revision.
Both service hops now use production route composition and the Room's HTTPS/
security-header wrapper. A separate one-shot test drops the service response
only after Admin stores a disable: the browser reports 502 without retrying,
then explicit refresh reveals the changed off revision before confirmed
re-enable. Unrelated handlers are explicitly unavailable; this is not full
service startup or a deployed service crash/restart test.
Reload and a second same-session tab read the exact stored off revision without
another mutation or login. Logout also rejects the already-open tab's cached
CSRF mutation. A superseded concurrent sign-in attempt shows the existing
expired-attempt UI; normal Sign in recovers a new session without changing
source state. This is shared-session logout evidence, not independent-session
revocation or concurrent-write arbitration.
It does not activate subject links, provision production transport/credentials,
prove gateway acknowledgement or establish deployed-directory acceptance.
Final envelope syntax remains open. See the
[runner prerequisites and evidence](MULTISOURCE_POLICY_SUPPORT.md#repeatable-source-lifecycle-acceptance)
and the
[implementation contract and evidence](MULTISOURCE_POLICY_SUPPORT.md#policy-declared-sources-and-durable-default-on-controls-2026-10-24).

The initial subject-link direction is now approved: Control-Service ownership,
verified device roots, exact LDAP `entryUUID` targets, independent scoped
reviewers and no strict-mode legacy fallback. Its protected review store now
has versioned source registrations with immutable audit evidence and coherent
authority/source capture. Registry-bound submission/approval atomically check
versions and enrichment scope while retaining mandatory exact-subject validation
and historical retry semantics. An internal protected provider loader now binds
the exact organization/source/version/provider reference to independently
provisioned LDAPS trust and credentials. Its read-only validator requires one
exact canonical `entryUUID`, and live TLS fixture tests compose it with durable
proposal/independent approval and historical retries after database reopen.
The split Control-Service now has explicitly configured review-only HTTP routes
and a dedicated Control Room named-user signing proxy. They are disabled by
default and require independent backend grants, mTLS, a protected review database
and version-bound provider configuration. OIDC-to-LDAPS tests cover independent
approval, lost replies, historical retries, source disable and logout. There is
no review UI, registry provisioning workflow, active-link publication or VS
enforcement. Snapshot format, timing bounds and rollout still require approval. See
[current implementation and gates](MULTISOURCE_POLICY_SUPPORT.md#approved-initial-direction-and-transactional-source-registry-2026-10-25).

Expired lookup identities are indeterminate even if another identity on the
actor is still live. They cannot fetch fresh enrichment or be silently dropped
to permit absence-based rules; reauthentication is required. The same guard
applies to admission, actor refresh and existing-visa rechecks. This does not
provide issuer-qualified cross-source subject links.

REST trusted sources can optionally bound lookup observation validity with
`expires_unix`, including empty results. Updated VS retains a source-qualified
freshness record, bounds new visas by it, refreshes expired observations, and
queues revocation when a new source deadline is earlier than an existing visa's
expiry. The provider's `-attribute-validity` flag is opt-in for compatibility;
see the [provider contract](zpr-dashboard/cmd/zpr-trusted-service/README.md).
Only omission preserves legacy deadline behavior: explicit `null` or malformed
validity data fails the lookup as indeterminate, with no partial attributes.
Repeated JSON attribute names also fail before mapping instead of silently
overwriting an earlier claim.
File-backed source lookups also require exactly one value for a single-valued
mapping. Missing or multiple values fail the whole lookup as indeterminate;
they are not converted to an empty string or silently truncated. Multi-valued
claims and valueless tags retain their existing behavior.
File JSON also rejects repeated identity keys, subject entries and attribute
names, using the same unique-name decoder as REST. Invalid or unreadable file
reloads are source-labelled indeterminate failures and do not advance the
snapshot revision. Failed explicit flushes preserve a still-valid snapshot;
automatic reload failures do not serve an expired snapshot.
File snapshots refresh when less than 60 seconds remain; a failed automatic
reload also rejects a still-unexpired snapshot inside that refresh window.
Queued automatic readers reuse a successful reload, while each explicit flush
publishes its own revision. Cache hits do not extend snapshot validity.
JSON decoding and matched-claim validation are separate: a decoded file can
advance revision yet fail a lookup on scalar cardinality. Repair then requires
an explicit flush or the next automatic reload, not merely another cache hit.
The reference REST provider also rejects duplicate/null file data and malformed
identity requests before lookup, with bounded input sizes. Corrected provider
files recover on the next lookup; canonical claim cardinality remains VS-owned.
Request fields use their exact documented casing. Invalid UTF-8 or unpaired
Unicode escapes in requests or provider files fail rather than being silently
rewritten into different identity data.
LDAP lookups reject incomplete referral/partial results, duplicate requested
attributes and conflicting direct/group roles. Canceled search results do not
become successful observations; group names must be unambiguous.
Provider responses are validated and encoded before publication, rejecting
null/invalid-text claims or encoded bodies over the VS 1 MiB limit. Cancellation
and source validity are checked before success is written; write failures are
logged and lookup responses are not cacheable.
VS requires a complete HTTP 200 observation from the configured provider
endpoint and never follows lookup redirects. Other statuses, including partial
content, are indeterminate rather than valid attribute snapshots.
HTTPS store initialization rejects empty/malformed CA bundles and invalid
client identity material. Isolated consumer tests verify mutual TLS, including
peer-certificate enforcement, hostname verification and credential failures.
TLS file-read errors include source, purpose and path while retaining the
underlying I/O error. Lookup tests also verify the existing five-second timeout
across stalled handshakes, response headers and partial bodies.
Consumer mTLS tests cover not-yet-valid certificates, complete versus missing
intermediate chains on both sides, and IP-address SAN verification. Repeated
lookups verify exact identity serialization and recovery after HTTP, response
validation and interrupted-body failures using the same client without a flush
or restart. Lookup outcomes preserve the source revision; explicit flush
advances it. These are isolated contract tests, not deployment or PKI lifecycle
acceptance.
Trusted-source health is scoped to the active service-list snapshot: completion
of a retired lookup cannot mark a replacement source working or failed. Full
and targeted lookups use the same lifecycle rule, and failures retain the last
success timestamp within their own snapshot. Health is observation evidence,
not a change to attribute authorization or a downstream revocation guarantee.
Actor-refresh regressions cover changed mappings, refreshed single/multi-source
alias transitions, bounded empty observations, mixed source failures and a flush
during lookup. Database readback verifies claim removal and shorter supporting
deadlines at both endpoints. These tests do not establish complete retired-source
cleanup or policy-generation isolation for in-flight authorization.
Removed-source cleanup now runs before lookup identity selection. It removes
persisted source-qualified claims and their same-source legacy aliases,
including authority/freshness claims and associated identity keys, even before
TTL expiry. Matching key qualification and source provenance identify ownership
after manager restart without deleting unrelated authenticated claims by issuer
name alone. Revision records are forgotten only after successful persistence.
An explicitly removed source means known absence under the current policy;
an unavailable active source remains indeterminate. Existing-visa sweeps persist
cleanup before rechecking visas; actors without live visas refresh lazily.
Legacy-only claims without qualified ownership evidence are not inferred to be
source-owned. Concurrent policy/source-generation isolation remains open.
Refresh now rejects results if policy generation or the source-list publication
changed during a lookup, before writing or committing revisions. Republishing
the same store after removal is detected too. A change observed after persistence
invalidates actor revisions and requires a reload/retry; it does not roll back
the write. Deterministic tests pause the actor attribute pipeline after lookup
and before mutation: source replacement/removal/republication and policy
publication require retry after the write, clear only that actor's revision
records, and preserve a simultaneous storage error. Reloaded retries fetch
current claims even when replacement revisions reuse earlier values.
These tests validate the existing post-persistence guard, not atomic rollback.
If a refresh future is dropped while awaiting actor persistence, a synchronous
drop guard invalidates that actor's source revisions and logs the required
reload. Cancellation before persistence leaves existing revisions intact.
Actor updates replace attributes, identity-key metadata and both service-index
directions in the same atomic pipeline. This prevents cancellation between
those writes from leaving identity keys pointing at removed claims or service
registrations contradicting the applied claims. Replacement/removal affects
only this actor's memberships, preserving other providers of shared services.
Timestamps remain separate writes. Actor updates now compare the exact
previously read service set inside a Redis Lua batch before applying mutations.
A mismatch returns an explicit concurrent-update conflict with no batch writes;
the caller must reload and retry. This prevents stale service-index cleanup
from leaving memberships introduced by an intervening update. Equal service
sets (including change-away/change-back), concurrent attribute-only writes,
disconnect and address reuse are not fenced. Submitted database work is not
rolled back.
The Redis backend now explicitly uses `MULTI`/`EXEC` for atomic pipelines;
previously it only batched commands, despite FakeDb modeling atomic execution.
Private Redis regressions cover queue-time batch aborts, supported operations
and TTL, empty batches, and the distinction between isolation and rollback:
Redis execution-time errors can still leave other commands applied.
The conditional Lua batch stops at its first command error without rolling
back earlier mutations, unlike MULTI/EXEC, which continues later commands.
Full actor loads read the metadata and attribute hashes together in one
`MULTI`/`EXEC` snapshot, preventing old attributes from being paired with new
identity keys during a concurrent update or removal. Missing actor metadata
returns not-found; missing identity-key metadata and malformed records remain
explicit errors. This snapshot is consistent at read time, not a lease on actor
liveness, and it does not snapshot service-index queries.
CN-only lookups reuse the same two-hash snapshot, decoding only the CN claim.
They do not decode unrelated claims or identity-key JSON, preserving targeted
lookup behavior even when unrelated records are malformed. This fetches both
hashes rather than a single field; no claim-size performance bound is asserted.
Normal visa requests retain the policy captured before endpoint
refresh and deny if configuration changes are observed after refresh or policy
evaluation. These are optimistic guards, not atomic publication/write/issuance
isolation; changes after the last check and concurrent same-actor writes remain
open boundaries.
Normal issuance initially persists every node in a non-installable `Staged`
state, checks configuration, then activates the whole path with a guarded
write before distribution. Interrupted requests remain staged across repository
restart until expiry; policy verdicts do not activate them. An observed change
after initial storage queues revocation before returning deny; if that write
fails, the record remains staged rather than pending install. Activation write
errors attempt deletion under the same backing-write lock: successful cleanup
removes the durable record and memory indexes, requiring a fresh request rather
than retrying that visa ID. The original activation error still propagates.
Failed cleanup is explicitly logged and leaves memory staged for repair; if
activation applied but deletion was rejected, restart can still expose a pending
install. Configuration is checked again after activation. Cancellation during
activation/cleanup and changes during/after activation remain open boundaries.
This is not atomic configuration/storage/distribution isolation.
`Staged` is a new local persistence state: older binaries do not understand
records containing it; mixed-version access/downgrade is not supported.
Actor-persistence fault tests verify rejected attribute writes preserve stored
claims and identity bookkeeping without committing source revisions. Failed
pruning and mixed-source updates remain retryable. A late repository failure
can occur after new claims have been stored: the overall actor update is not
atomic, but it still returns an error and leaves source revisions uncommitted.
Recovery reloads and retries rather than trusting a partially completed update.
These are injected test-database failures, not live Redis durability evidence.
Disconnect releases managed addresses only after actor cleanup succeeds and
source revisions are purged. Rejected or ambiguous cleanup failures retain the
allocation, including cascaded adapters, so a new actor cannot inherit a failed
cleanup's revision records through pool reuse. A later successful cleanup can
release the address even if an earlier partial deletion removed the actor.
Failures remain logged under the existing best-effort disconnect contract;
automatic retry scheduling and restart-persistent quarantine are not implemented.
Visa deadline tests exercise competing supports, reverse-direction roles,
selected-policy isolation, expired bounds and exact recheck comparisons at
equality and one nanosecond either side. Repeated attribute sweeps retain queued
revocation after validity recovers and never extend the stored visa; relay
delivery remains asynchronous and is not established by these tests.
Trusted-source declarations reject blank provider field names, repeated raw
fields mapped to different targets, and aliases that assign different
cardinalities to one canonical key. This validation applies to file and REST
sources before store construction/policy publication. Identical repeats,
same-shape aliases and case-sensitive raw field names remain supported.
File/verified-HTTPS parity tests check exact canonical keys, provenance and
value arrays across cardinality, tags, aliases, Unicode and strict malformed
input handling. Unknown fields are filtered only after their JSON values are
validated. File alias conflicts retain their existing parameter-error contract;
REST conflicts are indeterminate. A failed explicit file reload preserves its
still-valid snapshot, unlike a failed REST lookup, which returns no observation.
Derived user-authority tests cover exact source qualification, supporting user
deadlines, explicit authority precedence and real single/multi-source stores.
Under the existing presence-based contract, user tags and present empty
multi-value user claims support derived authority; omitted user fields,
device-only claims and freshness metadata do not. Explicit provider authority
is preserved rather than shortened or replaced by derivation.
Policy/source lifecycle tests check rejected updates against both active and
persisted containers/generations, source revisions and working lookups. Accepted
mapping, TTL and identity-declaration changes reconstruct stores; unchanged
declarations reuse the current snapshot without rereading the backing file.
Test-database restart restores the accepted policy generation and reconstructs
sources with new revisions. This is not live Redis crash-recovery or atomic
policy/source publication evidence for in-flight authorization.
Lookup-identity tests verify exact keys/text, expiration rejection and the
existing non-scalar skip rule. Real file-source tests cover identity-key
isolation, order-independent unions/conflicts, duplicate identities, malformed
matched versus unmatched records, and repair through flush. Lookup pairs still
do not carry issuer provenance; these checks do not implement subject links.
This does not implement delegated-source credentials or selective invalidation.
Durable reconciliation tests check generation coalescing, exact source-name
isolation, malformed-state errors, flush/lookup failure retries and requests
arriving during a pass. Recovery acknowledges completed work without canceling
queued revocation or extending issued visas. These use the test database; they
do not establish live Redis crash recovery, write-failure durability, concurrent
worker safety or relay acknowledgement.

Successful trusted-source refreshes replace the complete observation from that
source, whether triggered by TTL or revision changes. Claims no longer returned
are removed even when their old individual TTL has not expired. Other sources'
claims remain separate; lookup failures remain indeterminate rather than being
treated as successful empty results. Namespace-envelope identity and delegation
work is explicitly deferred beyond the current maturation scope.

Existing-visa rechecks reuse issuance lifetime bounds. If the newly selected
policy conditions, authentication, provided-services claim or source observation
expires before a stored visa, reconciliation queues revocation even when the
flow is still allowed now. Longer validity does not extend an issued visa.

Control-Service watchers retain their previous durable LDAP checkpoint on rejected
acceptance and replay on restart. Watcher health reports `accepted_pending`
separately from source-stream health; use the Visa Service resource for definitive
completion/failure, not the watcher's last refresh timestamp.

## Crates / Packages / Libraries

Policy evaluations use a fresh Cap'n Proto reader traversal budget for each
request. Repeated evaluation of an unchanged policy cannot exhaust a shared
reader's lifetime budget; parsing errors remain explicit.

- `vs` - The ZPR Visa Service. Aka "v2vs" -- the "v2" used to differentiate it
  from the "v1", prototype visa service.
- `libeval` - The "evaluator" library used by the `vs` and `zpt`. Includes the
  code that compares a description of network traffic to policy to determine if
  a visa should be issued.
- `zpt` - ZPR Policy Tester is a command line tool for testing how `libeval`
  evaluates policy.
- `vs-admin` - CLI based administration client for the visa service. Exercises
  the HTTPS admin api of `vs`.
- `zpr-dashboard` - CLI based "GUI" dashboard for monitoring visa
  service activity.
- `admin-api-types` - Library crate for data structures used by `vs` and
  `vs-admin`.
- `integration-test` - Shell-based integration tests. Includes evaluation tests
  of `libeval` using `zpt`.
- `tools` - Helper scripts, including `zpr-pki` for PKI operations.

Most of the visa service code depends on the
[zpr-common](https://github.com/org-zpr/zpr-common.git) repository, which
defines data structures used in the NODE-VS API and the policy binary format.
This dependency is pulled automatically via git in `Cargo.toml`, so no manual 
setup is required.


## Prerequisites

- **Rust** - Edition 2024. Install via [rustup](https://rustup.rs/).
- **Make** - The build is driven by a root Cargo workspace.
- **OpenSSL** - Via the `openssl` crate.
- **Redis/Valkey** - Required at runtime by `vs`.


## To build

Run `make` or `cargo build` in this top-level directory.

Run `make test` or `cargo test` in this top-level directory to run all
the unit tests.


## Release build (prototype vs only)

Run `make release` to produce a release tarball of the visa service tools. This
builds everything, then packages the `vs`, `vs-admin`, and `zpt` binaries into
`build-release/`.


## Admin HTTPS API

The visa service (`vs`) exposes an HTTPS admin API on port 8182 by default.
The `vs-admin` command line tool consumes this API.

See [admin-http-api.txt](admin-http-api.txt) for full endpoint documentation.

## ZPR DNS service

The BIND 9 deployment profile runs DNS on a ZPR service address, uses ZPL to
authorize authenticated query clients, and accepts service-record updates from
the Visa Service only through TSIG over ZPR. BIND is not exposed as a public or
underlay DNS service. See the [BIND 9 ZPR DNS guide](dns/bind9/README.md).

## ZPR observability service

The [OpenObserve deployment profile](observability/openobserve/README.md)
packages a third-party open-source log and metrics server behind a dedicated
ZPR adapter. It specifies OTLP/HTTP ingestion, authenticated reader access,
and the required Visa Service exporter interfaces. The profile does not
provision an adapter, install policy, or export existing metrics automatically.


## Multiple trusted sources and policy limitations

Multiple trusted-service attributes are now exposed as source-qualified policy
keys (`attribute__source__source-id`); the source-built ZPLC resolves them against
raw provider mappings, with a real compiler → VS stores → libeval grant/deny test.
Single-source policies retain unqualified aliases; multiple sources require an
explicit organization policy default or qualified references. Control Room can
persist that default. Identity-changing registry edits advance source versions;
staging pins source versions/defaults into signed metadata and guarded apply
requires restaging when they change. This does not yet provide installed-visa
version enforcement, approved cross-source subject links, dependency-aware outage
handling or selective per-grant revalidation. See the
[evidence trace and seven-scenario support matrix](MULTISOURCE_POLICY_SUPPORT.md)
before relying on those guarantees.

The S05 foundation now preserves verified adapter/device authentication as
internal actor metadata, separately from enriched claims, through persistence
and refresh. Ordinary updates now atomically reject a changed authentication
session alongside the service-set check; verified reconnect has an explicit
replacement path. This does not establish a human identity, an approved source
link, an organization/trust-version binding or complete actor-generation
fencing; provider lookup behavior is unchanged. See the
[foundation scope and limitations](MULTISOURCE_POLICY_SUPPORT.md#s05-foundation-increment-preserved-device-authentication-2026-10-24).
The [session-guarded write packet](MULTISOURCE_POLICY_SUPPORT.md#s05-foundation-increment-session-guarded-actor-writes-2026-10-24)
documents conflict handling and remaining race boundaries.

The [production subject-link contract proposal](MULTISOURCE_POLICY_SUPPORT.md#s05-production-binding-contract-ownership-and-api-review-2026-10-24)
records the selected Control-Service ownership and immutable-provider-ID-only
direction, with proposed approval/delivery APIs and acceptance requirements.
It is documentation only; those APIs and subject-link enforcement do not exist
yet.
Internal [durable proof/review groundwork](MULTISOURCE_POLICY_SUPPORT.md#s05-durable-proof-and-review-composition-groundwork-2026-10-24)
now combines scoped request proofs, bounded persistent nonce consumption,
retry-safe review evidence and exact permission mappings in tests. No review
HTTP handlers, production trust provisioning, authority registration or active
subject-link enforcement are enabled by this groundwork.
The [internal BAS authority registry](MULTISOURCE_POLICY_SUPPORT.md#s05-internal-bas-authority-registration-and-capture-2026-10-24)
adds persistent organization-scoped mappings, versioned edits, audit history and
root capture. It is not yet composed into a production service or review HTTP
workflow, and does not provision trust or change subject-link enforcement.
Opt-in [guarded review APIs](MULTISOURCE_POLICY_SUPPORT.md#s05-transaction-guarded-authority-review-integration-2026-10-24)
now validate current authority registration inside review transactions.
Separate source validation, HTTP composition and active-link publication remain
unimplemented; successful review evidence is not lookup authorization.
The [exact LDAP/source-review batch](MULTISOURCE_POLICY_SUPPORT.md#s05-exact-ldap-subject-lookup-and-source-guarded-review-2026-10-24)
adds an internal exact UUID reader and single-writer guarded review composition.
No review endpoint, active link or dynamic VS source installation is enabled;
production policy/source delivery and credential activation remain unresolved.
