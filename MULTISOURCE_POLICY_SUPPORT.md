# Multiple trusted sources: source-qualified policy support

## Repeatable source lifecycle acceptance

The opt-in [runner](zpr-dashboard/scripts/test-source-lifecycle.sh) connects
Policy-Service staging, the source-built compiler, the real VS REST consumer,
the independent Go provider and durable VS state in one automated chain:

1. Save two JSON policy documents sharing `company` in the actual SQLite policy
   repository, check and compile them, then stage a signed bundle using an
   ephemeral fixture key. Assert the original saved JSON/revision and independent
   base-config hashes, one effective source, and conflicting-declaration rejection
   without replacement of the last-good staged candidate.
2. Start an actual mTLS HTTPS provider backed by a local TLS LDAP protocol
   fixture. Explicit synthetic actor identities select exact directory UUIDs.
   The compiled source-qualified policy permits TCP 80 and denies TCP 81.
   Assert persisted, source-owned attributes and a real issued visa.
3. Start the production Admin TLS listener/router on an ephemeral loopback
   socket. With fresh read/read-write fixture API keys, assert authenticated
   readback and a default-on source with an empty override revision. Missing,
   malformed and revoked keys cannot read or mutate; read-only keys cannot
   mutate. Malformed/duplicate/extra-field toggle JSON cannot change the stored
   control. A client trusting an unrelated CA cannot connect.
4. Connect the actual Go Control-Service trusted-source bridge and production
   Admin client transport to that listener. Inventory and control readback must
   agree with direct Admin readback. Deliberately invalid server-owned credentials
   and unrelated CA trust return 502 JSON errors, not successful readback; a
   read-only server key returns 403 on mutation. Correct configuration recovers
   without changing stored controls. Malformed toggles return 400 without
   mutation, unknown sources return 404, and responses are `no-store`.
   Browser-supplied API keys/cookies/bearer values cannot select or replace the
   server-owned Admin key; no Admin key appears in returned JSON.
5. Put that bridge on a separate TLS 1.3 listener requiring a trusted client
   certificate. Use the production Control Room proxy and its file-configured
   mTLS client transport, with independent ephemeral service CA/server/Room
   certificates. The fixture verifies the configured Room certificate and that
   browser cookies, bearer credentials, origin, CSRF, forwarding headers and
   forged operator delegation do not reach the service. Missing and foreign
   client certificates fail during TLS before any HTTP bridge call.
   The browser-facing ingress remains CA-verified HTTPS behind the real
   Control Room API authorization wrapper. The existing operator-auth implementation obtains
   fixture sessions through OIDC discovery, PKCE token exchange, signed RS256
   ID-token/JWKS verification and nonce binding. Explicit fixture grants separate
   a reader, a writer and a foreign-organization writer. Writer requests carry
   a secure session cookie, same-origin header and session CSRF proof.
   Anonymous reads/writes, read-only mutations, foreign-organization requests
   (including a forged organization query), missing/wrong CSRF, missing/foreign
   origin, forged identity headers and logged-out sessions must return 403 with
   `no-store` and never reach the bridge. Logout uses the real session revocation.
   A valid writer/CSRF request through a proxy with unrelated service CA trust
   returns 502 with `no-store`, without reaching the bridge or Admin API.
   Correct service trust is retained for subsequent lifecycle operations.
   Both hops use their production route composers, not hand-registered source
   routes. Control Room also uses its production direct-HTTPS/management
   protection and security headers, including same-origin referrer policy for
   HTML login documents. Unrelated service handlers are explicitly unavailable;
   Simulator routes are not installed.
6. Disable the source through the bridge's HTTPS route, crossing real Admin
   HTTPS with the server-owned write key, and require a 202 response
   with declared/effective/desired state, pending work and a fresh UUID revision.
   Deny new requests and let the actual event worker queue the existing visa
   for revocation at both fixture nodes; the test does not call the control
   reconciliation function directly.
   Stop and restart a private Valkey/Redis process from its actual AOF files,
   then start a separate VS test process. Assert that the exact signed artifact,
   off revision, pending work, actor docking and revoke queues survive, including
   authenticated HTTPS control readback from the new process.
7. Re-enable through the bridge against changed LDAP mail, wait for the actual event
   worker's persisted completion, and prove a compiled-policy denial rather
   than stale permission or a missing-route/docking failure. A further process
   with LDAP bind failure must deny, report failed source health rather than
   successful empty absence, and issue no new visa.
8. Restore LDAP data and start another process. Re-enable through the bridge, wait for
   persisted worker completion, and verify fresh lookup permits a new visa
   with a different ID; the old queued revoke remains. Assert exactly two
   directory searches per successful phase and none during bind failure; the
   disabled requests must not add directory searches.
9. Optionally set `ZPR_TEST_SOURCE_BROWSER=1` to run actual Chromium during the
   recovery phase. Serve the embedded production login widget, operator request
   helper and runtime-source control component on a minimal fixture page.
   Begin with an empty browser cookie jar, navigate the widget's real OIDC login
   redirect to the isolated HTTPS IdP, explicitly consent to the named fixture
   writer identity, and follow the real callback. Require a secure HttpOnly
   session cookie and the source-only permission scope. No session is imported
   for sign-in. The widget reads the real session endpoint and supplies mutation
   CSRF through the production helper.
   Cancel confirmation without any PUT, then confirm disable and verify the
   rendered desired/effective-off/pending state and stored revision. Confirm
   re-enable and require a different persisted revision. Switch to an unrelated
   service trust root through an owner-only, browser-opt-in fixture fault route.
   A confirmed UI disable must return 502 with `no-store`, show the failure and
   disable stale controls; refresh must also fail visibly. Restore correct trust,
   explicitly refresh, and require usable controls with the same enabled state
   and exact revision: the rejected mutation must not change durable state.
   Next arm a one-shot response-loss fault. Execute a real disable through
   Admin, wait for its valid 202 response inside the service handler, then abort
   the service HTTP response before delivering it to the proxy. The browser
   receives 502 even though the change is stored. Require visible failure and
   disabled stale controls with no automatic retry. An explicit refresh must
   show desired/effective off, reconciliation pending and a new stored revision.
   Refresh must not send another mutation. Reload the entire HTML document and
   open a second same-session tab; both must read exactly the same stored off
   control/revision without another OIDC callback or PUT. Explicitly confirm
   re-enable and require another revision, then refresh the second tab to on.
   Then use the real logout button,
   require 204, and leave the fresh IdP consent unaccepted. Replay the former
   cookie and CSRF proof in a separate browser context: session read must return
   401 and mutation must return 403 with `no-store`. A mutation from the already
   open second tab, using its cached session CSRF, must also return 403 with
   `no-store` and must not retry; the real request helper rechecks the session
   and the login widget navigates back to fixture consent. Leave that consent
   unaccepted and close the tab. Its newer browser-bound login flow supersedes
   the first tab's pending flow: submitting the older consent must show the real
   expired-attempt recovery UI with no authenticated scope. Click Sign in and
   complete fresh consent/callback without editing cookies or session storage.
   Complete that new browser login
   and require a different session cookie with the same stored source revision,
   proving the rejected replay did not mutate state. Browser requests must stay
   on the two independent loopback application/IdP origins without Simulator
   access; page errors fail. After the browser sequence returns, the real event
   worker completes its accepted change and VS grants a fresh visa from exactly
   two LDAP endpoint refreshes.

Run from this repository's root:

```sh
ZPR_TEST_ZPLC=/absolute/path/to/source-built/zplc \
  sh zpr-dashboard/scripts/test-source-lifecycle.sh
```

For real-browser component acceptance, use:

```sh
ZPR_TEST_SOURCE_BROWSER=1 ZPR_TEST_ZPLC=/absolute/path/to/source-built/zplc \
  sh zpr-dashboard/scripts/test-source-lifecycle.sh
```

The browser option additionally requires the dashboard's existing Node/
Playwright dependencies and its installed Chromium. It never installs them.
The default remains browser-free. Chromium permits the ephemeral ingress and
IdP certificates' exact SPKIs via a process-scoped test flag; this is not a global
certificate-verification bypass or host trust-store modification. Admin and
provider and Room-to-service TLS continue verifying their configured roots normally. Browser
contexts/processes close on success or failure; missing tools and missing
browser completion markers fail explicitly.

Prerequisites: Rust/Cargo, Go, Python 3, an existing source-qualified compiler
build, and an installed `valkey-server` or Redis 7+ `redis-server`. The runner
builds the VS test executable using the checkout's configured `zpr-common`
dependency path. For nested worktrees where that sibling path is unavailable,
supply `ZPR_TEST_VS_BINARY=/absolute/path/to/current/VS-test-executable`
alongside `ZPR_TEST_ZPLC`, built from the same current source in a resolvable
Cargo layout. The runner never rewrites dependency manifests, installs tools,
contacts an existing database or starts persistent services. Its private
database has no TCP listener and uses a protected temporary Unix socket;
processes, AOF files and ephemeral credentials are cleaned up after the test.
Missing prerequisites, failed compilation or skipped child phases fail explicitly.

Both Go tests skip in ordinary suites unless their opt-in environment is set;
the Rust child is ignored unless launched by the fixture orchestrator. Output
must contain each lifecycle marker and a passing Rust child, preventing a
successful skip from being counted as acceptance. Simulator URLs deliberately
point to unavailable loopback endpoints, and no simulation manifest is loaded.

**Evidence boundary:** these are real compiler, HTTPS/mTLS, LDAP protocol,
Redis AOF, production Admin TLS/router/authentication, real event worker,
manager/evaluator/visa-worker and OS-process boundaries. The Admin listener
shares the production connection-serving loop, not a substitute HTTP router.
They are not full Visa Service daemon/startup, node authentication, Control-Service
deployment or full deployed Control Room acceptance. The optional
component browser check uses real production assets and live APIs but a minimal
fixture page, not the complete navigation/deployment. The browser-facing
listener is private CA-signed loopback HTTPS with real named-user/organization/
CSRF authorization and an additional random test-owner token. Runtime API
requests now cross the production reverse proxy and configured mTLS transport
to a separate private listener with the production TLS 1.3/client-CA requirement.
This runs the real source-control handler and both production route composers,
with unrelated features explicitly unavailable. It does not run full service
startup commands or provision every production feature. It is local transport evidence,
not a deployed ingress or proxy acceptance. The browser-free path provisions sessions through the real auth handler
using HTTP test requests, with actual TLS discovery/token/JWKS traffic to a local
identity-provider fixture. The browser option instead navigates OIDC login,
consent and callback without importing a session. The IdP's explicit consent
button selects only the named synthetic writer: it is not password/MFA
authentication, federation or a production IdP. Logout's automatic fresh
sign-in is retained rather than changing production login UX.
A test-only bootstrap route supplies each ephemeral Admin
endpoint, key and CA; it is not source registration or a production configuration
API. The actual production bridge and Admin transport handle runtime requests.
The service-trust fault route is also owner-only, browser-opt-in and test-only;
it selects an unrelated trust root without weakening certificate verification
or changing production configuration.
The response-loss arming route has those same restrictions and consumes exactly
one accepted PUT response. It does not simulate a rejected write: real durable
Admin acceptance must precede the abort. This covers response ambiguity, not a
service crash/restart or gateway acknowledgement.
Actor authority and UUID-valued device CNs
are explicitly synthetic fixtures, not invented device/person subject links.
The LDAP server is a bounded protocol fixture, not deployed OpenLDAP. The signed
candidate is loaded directly by the test harness, not activated through the
guarded apply/reviewer protocol. Revocation is queued, not gateway-acknowledged.
No production configuration, keys, permission grants or deployment is created.
**M1 remains open** for approved-link VS enforcement and the independently
deployed operator-to-gateway flow.

Validation for this acceptance increment:

- The real four-VS-process lifecycle chain passes, including the database
  restart, and passes three consecutive reruns with `GOFLAGS=-race` covering
  both Go orchestrators.
- Rust workspace: **993 passed, 23 ignored**; the new ignored child is exercised
  explicitly by the acceptance runner. Rust formatting checks pass.
- Provider and policy-document race suites, Go vet, changed-file Go formatting
  and POSIX shell syntax checks pass.
- Operator-auth and Control Room race suites pass. An ordinary, non-opt-in
  OIDC authorization regression also proves that scoped readers and
  CSRF-authorized writers pass while rejected requests never reach downstream.
- Required full `make test`: **11 operator-stack checks pass**, with exactly
  the same **13 pre-existing dashboard fixture/environment failures**. No
  unrelated failing tests were changed; the complete Go suite is not green.
- The Admin connection-serving loop is extracted without changing production
  routing, TLS or authentication behavior, allowing a test-only entrypoint to
  use a prebound private listener without a port reservation race. Other changes
  are acceptance tests, fixture support, runner and docs; no dependency manifests
  or production configuration changed.
- The bridge increment adds only acceptance/fixture code and documentation;
  no new production endpoint, credentials or permission grants are installed.
- The operator-authorization increment is also test/fixture-only: fixture
  users, grants, signing material and sessions do not provision production
  identities or change production login/session behavior.
- The optional live-browser chain passes three consecutive race-enabled runs;
  the default browser-free chain still passes. Four desktop/tablet source-control
  tests pass, including a source-only operator regression. That browser check
  found and fixed the production login widget's missing `trusted-source.*`
  application-permission recognition: legitimate source-only operators now
  receive the session-ready event and mutation CSRF instead of being gated
  despite valid backend grants. Backend authorization rules are unchanged.
- Browser-driven OIDC sign-in, actual logout, old-session/CSRF replay denial and
  fresh sign-in also pass three consecutive race-enabled full-chain runs.
  This increment changes fixture/browser code and documentation only; no Rust,
  dependency manifests or production login/authentication logic changed.
- The real Room-to-service proxy/mTLS chain, including visible browser failure
  and recovery from an unrelated service trust root with unchanged source
  revision, passes three consecutive race-enabled full-chain runs. Missing and
  foreign client certificates fail TLS; an ordinary non-opt-in regression checks
  stripped browser credentials, exact method/path/query/body preservation and
  good-trust/bad-trust/good-trust mutation delivery. These changes are confined
  to acceptance fixtures, browser harness and documentation; Rust and production
  transport/authentication code are unchanged.
- Production Room/Service route composition and the Room HTTPS/header wrapper
  now participate in three consecutive race-enabled browser lifecycle runs.
  Each run drops exactly one response after real durable acceptance, proves
  no blind retry and reads the changed off revision before explicit re-enable.
  An ordinary non-opt-in regression verifies that an accepted PUT whose service
  response is aborted returns 502 exactly once, GET still works and the next
  explicit PUT succeeds. No production code or dependency manifests changed.
- Full document reload and a second same-session tab read the exact stored off
  control after response loss without a mutation or another callback. Logout
  rejects an already-open tab's cached-CSRF mutation and triggers real session
  recovery. Competing pending tab logins reject the superseded callback; native
  Sign in recovers a new session with unchanged source revision. Three
  consecutive race-enabled full browser chains pass. Only the browser harness,
  fixture HTML submit button and docs change; production login/session code,
  Rust and dependency manifests remain unchanged. This tests shared-session
  logout, not independent logins, tab synchronization or concurrent-write arbitration.

## Operator controls, shared JSON documents and immutable LDAP provider

This local increment builds on the durable VS controls described below:

1. **Control Room controls.** Trusted Sources now includes runtime controls
   separate from read-only directory registrations. Control-Service bridges
   inventory and GET/PUT control routes to the existing VS Admin API using only
   server-owned credentials; redirects and malformed readbacks fail explicitly.
   Named operators need explicit `trusted-source.read` and
   `trusted-source.control` grants and normal organization/CSRF authorization.
   Desired/effective/pending readback distinguishes persistence from delivery.
   Failures disable stale mutation buttons until a successful refresh.
2. **Shared IDs inside a reviewed revision.** Policy-Service accepts this
   provisional document JSON wherever a policy source is checked, saved, tested
   or staged. Documents compose in array order; declarations deduplicate only
   when API, TTL and ordered mappings agree. Conflicts with another document or
   the independent base compiler config fail; no last-writer-wins path exists.
3. **Independent provider delivery.** The reference Go REST provider accepts
   protected `-source-config` JSON for exact `ldap_entry_uuid` lookup. Its CA,
   bind identity/password reference, base DN and attribute allowlist are
   separately provisioned, never embedded in policy. A local real mTLS REST →
   TLS LDAP fixture proves delivery/failure semantics without Simulator.

Example policy record:

```json
{
  "documents": [
    {
      "id": "finance",
      "source": "define Finance as user with user.mail__source__company:'finance@example.test'.",
      "trusted_sources": {
        "company": {
          "api": "rest/1",
          "expiration_seconds": 600,
          "returns_attributes": ["mail -> user.mail"]
        }
      }
    },
    {
      "id": "engineering",
      "source": "define Engineers as user with user.mail__source__company:'engineer@example.test'.",
      "trusted_sources": {
        "company": {
          "api": "rest/1",
          "expiration_seconds": 600,
          "returns_attributes": ["mail -> user.mail"]
        }
      }
    }
  ]
}
```

The compiler receives one composed ZPL source and a private effective TOML
configuration. Bootstrap/certificate paths keep their original config-relative
meaning. Staging composes from the same captured base bytes whose hash is
reviewed, not a later reread of that file. Reviewed source hashes bind the **original JSON**, including all
declarations; base config hashes retain the independent original config.
Existing staged bundle/revision/config/source-version guards still apply.
New declaration content is bound by reviewed JSON and the compiled artifact,
not an invented mutable registry version. Check/test diagnostics identify
composed ZPL and do not assign its line numbers to JSON editor gutters.

Limits: one saved/reviewed revision, 1-64 uniquely identified documents, 1 MiB
JSON, and at most 16 effective nondefault source IDs across documents and base
config. Only `file` and `rest/1` declarations are supported, with
`api`, optional nonnegative u32 `expiration_seconds` (default zero), and nonempty
`returns_attributes`. Unknown/null/duplicate/case-variant fields fail. Compiler
validation still enforces mappings, reserved namespaces and API semantics.
Identical means exact normalized fields, including list order; semantically
similar but differently spelled base tables are not automatically reconciled.
There are no external includes, independent catalog-record composition or
automatic source registration. JSON-specific completion, formatting and
assistant edits remain unsupported; format/assistant operations refuse JSON
rather than accidentally dropping its declarations. Plain ZPL is unchanged.

See [Control Room permissions](zpr-dashboard/README.md#policy-owned-source-controls-and-provisional-documents)
and [provider configuration](zpr-dashboard/cmd/zpr-trusted-service/README.md#immutable-ldap-source-configuration).
No permission grants, transport configuration, real keys or deployment state
were provisioned. These controls are deployment-wide; they are not a new
per-organization source namespace or reviewer-proof authorization protocol.
Neither source declaration nor immutable directory presence activates a subject
link. **M1 remains open** pending approved-link VS consumption, live policy-driven
allow/deny and source-off/recovery/gateway evidence on the independent production
stack. Selective dependency reconciliation remains separate.

Validation for this increment:

- Focused Go/race suites pass for production source controls, declaration
  conflicts/limits, original-review hashes, revision validation, captured-base
  inputs, immutable LDAP evidence and the local REST-to-directory fixture.
- The source-built compiler passes real document checks and actual signed
  candidate staging with an ephemeral test-only key.
- Production-tagged asset/route/domain-handler tests pass. The two exact
  handler/route snapshots were updated with explicit user approval; no
  unrelated failing tests were changed.
- Desktop/tablet runtime-source and colliding-source catalog specs: **4 passed**,
  including refresh-in-flight fencing, pending readback, mutation/readback
  failures and explicit refresh recovery.
- Go vet, changed-file formatting, JavaScript syntax and diff whitespace checks
  pass. No Rust or dependency manifests changed in this increment.
- Required full `make test`: **11 operator-stack checks pass**; the same
  **13 pre-existing dashboard fixture/environment failures remain**. The new
  source-control route snapshots pass; the complete Go suite is not green.

## Verified result (source-policy completion)

The source-built ZPLC in the exclusive `maturing/source-qualified-attrs` compiler
worktree accepts `x__source__id` in tag, key, scalar-value, set/group-member and
class/group-name positions. No lexer/grammar relaxation was necessary. A missing
semantic step was fixed: qualified conditions now resolve against the named
source's **raw** `returns_attributes` declarations. The compiler keeps those raw
provider mappings in the binary, so VS ingestion qualifies the keys exactly once.

The VS ignored interoperability test
`test_source_built_compiler_to_stores_to_real_evaluation` actually invokes that
compiler, loads its Cap'n Proto output, constructs both real file stores through
the production factory, ingests colliding claims, and evaluates the resulting
actors with libeval: company-qualified Alice grants and lab-only Bob denies.
The same grant/deny proof runs again with unqualified tag/key authoring explicitly
bound to the company default, under strict compiler source-qualification mode.
This is not merely a hand-built condition test. The fixtures also compile tags,
source-qualified class names and multi-valued group/set references. Group-member
values are ordinary literal values: ZPLC does not invent/append group membership
values or authorize cross-source subject joins.

Source IDs consistently use `[A-Za-z0-9_-]{1,64}`, excluding the reserved
`__source__` marker. Base return mappings cannot contain the marker. The suffix
preserves the stable source ID; a display rename does not rename keys.

## Explicit organization policy defaults

The protected `ZPR_ASSERTION_SOURCES_FILE` registry now persists:

```json
{
  "default_source": "company_directory",
  "policy_defaults": {"northstar": "company_directory"},
  "sources": [{"name": "company_directory", "version": 1}]
}
```

This abbreviated example omits required provider/scope fields. `default_source`
retains its existing **assertion** semantics; `policy_defaults` is an explicit
organization-to-source designation, not an implicit first-source preference or
failover. Its single-valued map permits at most one policy default per org; each
selection must name a source scoped to that org. An org may deliberately have no
default, in which case ambiguous/unqualified multi-source policy input is rejected.
Exactly one configured source preserves legacy unqualified authoring.

Control Room's trusted-source registration/editor has a persistent default
checkbox. Saving it replaces that organization's prior designation; unchecking
it clears that designation only. Editing a different source does not clear it.
Requests retain organization identity and optimistic registry revision checks.
Default-source retirement is blocked until its designation is removed/replaced.

Policy-Service reads the same protected registry independently (no Control Room
or Simulator callback), uses `ZPR_POLICY_ORGANIZATION_ID`, refreshes catalog
aliases on catalog refresh, and supplies `--default-source ID` to ZPLC for
analysis, policy tests and staging. Those paths also always request
`--require-source-qualification`, which enforces the multi-source rule on parsed
attributes/tags/groups, including shorthand tags the catalog text precheck cannot
recognize. ZPLC emits **qualified condition keys** even
when authoring used an unqualified tag/key/group attribute. No runtime default
alias is required. Raw compiler callers retain legacy noncolliding-name behavior;
Control Room's multi-source authoring gate is deliberately stricter.

## Version binding and invalidation

New registrations start at version 1. Identity-bearing edits (endpoint, kind,
organization/base-DN/bind configuration, approved attributes, trust/file-reference
configuration) advance the server-owned version; display-only changes do not.
The registry revision also changes on default designation and persists over
restart. Assertions still support explicitly selected multiple sources.

Staging stores `source_versions`, `default_source`, and
`source_binding_sha256` beside the record/config/bundle digests in `latest.json`.
Bindings include source config fingerprints and registry identity fingerprints as
well as version counters, catching externally edited configurations even when an
operator fails to advance a counter. For safety this iteration binds **all**
configured sources rather than claiming minimal positive-only dependencies.

When a mutable registry is configured, Policy-Service supplies
`--source-binding SHA256` to the source-built compiler. ZPLC includes that digest
inside the existing **signed policy metadata**, so a version/default change
changes the artifact digest and cannot reuse an old artifact approval merely
because compilation happened in the same second. A real Go integration test
stages signed policies with the built compiler and verifies this distinction.

Guarded apply capture checks bindings again before prepare/commit. Registry
outage, identity/version/default mismatch or an old unbound artifact requires
restaging; no registry fallback is used. This is a Policy-Service metadata gate,
**not** live VS source-version enforcement. Already installed policies/issued
visas do not independently compare registry versions. Propagating an authenticated
version manifest into VS and revoking/revalidating existing grants remains a
production gate. No shared schema file was edited.

## Seven-scenario matrix

| # | Scenario | Outcome / evidence |
| --- | --- | --- |
| 1 | Colliding A/B attribute names; A-only policy | **Verified:** source-built ZPLC → production VS store factory → namespaced actors → real libeval grant/deny. Unqualified Control Room input requires one source or explicit org default. |
| 2 | Combine A/B attributes/groups | Both key namespaces survive; compiler accepts qualified tag/key/group/class/set spellings. **Not an approved subject correlation:** shared lookup identity behavior remains. |
| 3 | A unavailable; B-only policy works | **Not implemented:** current whole-actor closure remains conservative; no fail-open skipping introduced. See precise design below. |
| 4 | Rename/edit cannot silently rebind | Display-only rename preserves version/key; identity edit advances version and invalidates staged candidates, including signed artifact binding. **Installed policy/visa invalidation remains open.** |
| 5 | Same-text A/B subjects not implicitly merged | **Not implemented:** key namespacing prevents overwrites, not implicit subject fan-out. Explicit link enforcement remains mandatory before production. |
| 6 | Bundled/company providers coexist | Distinct qualified keys and default selections work without Simulator. Approved identity-link/bootstrap lifecycle remains open. |
| 7 | A change affects only A grants | **Not implemented:** global conservative demo revalidation is intentionally retained (X13); staging conservatively binds all configured sources. |

## Dependency-aware outage: precise stop/design

### Maturation increment: TTL outage guard (2026-10-24)

TTL-refresh failures now report `AttributesIndeterminate`, even when the source
revision is unchanged. The first failure retains the expired cache, but records
the retry sentinel; subsequent revision-stale failures remove cached claims.
The existing visa-request guard denies rather than evaluating missing keys as
known absence. Visa rechecks now also refresh both actors, and indeterminate
refreshes queue revocation through the existing sweep. Other persistence/errors
remain explicit errors, not successful verdicts.

Regression coverage exercises a source-qualified absence-based allow with two
configured sources, a healthy empty sibling, failures at either endpoint,
recovery and existing-visa reconciliation. It proves why the evaluator's
expired-key guard alone is insufficient: the unavailable source might now vend
a different key that was never in the actor cache.

This is conservative whole-actor denial, not selective outage support. It does
not close explicit subject-link isolation, authenticated source/version
propagation, concurrent-change enforcement, restart dependency manifests or
bounded relay revocation delivery. No schema or peer authentication change is
included.

Validation for this increment: both new request/reconciliation regressions
failed before the fix and pass afterward. `cargo test --workspace` passes
(689 tests passed, 9 ignored); the 14 actor-refresh tests also pass separately.
`cargo fmt --all -- --check` passes. Validation used the existing local
`zpr-common` checkout through a temporary worktree-relative Cargo path, restored
afterward; no dependency or shared-schema changes are part of this increment.
This is source/test evidence, not deployment or relay-delivery acceptance.

Qualified `AttrExpr.key` values can identify source dependencies without changing
Cap'n Proto, but extracting only positive conditions is unsafe. VS currently
returns a whole-actor lookup error, and an absent attribute is not an unavailable
source. Simply dropping failed-source claims can grant a negation/absence rule.
Expiration guards do not represent a source that never returned attributes.

A safe follow-up must:

1. Track per-actor `(source ID, version, availability)` independently of claims;
   revoke cached claims on failed refresh and record unknown even for empty stores.
2. Derive complete dependencies from compiled client/service/link conditions,
   join/admission requirements and authority identities; include negative/absence
   operators, deny policies and expanded classes. Mark legacy unqualified or
   otherwise unresolvable dependencies conservatively, never as dependency-free.
3. Before evaluating a condition/policy, treat an unavailable dependency as
   indeterminate: it cannot satisfy any allow (including NOT/absence); an
   indeterminate deny must continue vetoing overlapping traffic. Only unrelated
   policies with fully available dependencies may grant.
4. Change whole-actor admission/refresh error handling only together with those
   evaluator and cached-visa guards. Persist grant dependency/epoch information,
   then verify positive/negative/deny, empty-source, expiry, recovery and concurrent
   version-change cases before claiming selective revalidation.

No unsafe partial outage implementation was added. An in-memory evaluator model
could avoid a schema change initially, but durable visa/version dependencies and
restart enforcement still need an authenticated contract. `zpr-common` remains
read-only; any schema addition must be separately reviewed.

## Maturation increment: authoritative TTL replacement

A successful source lookup now removes all old claims from that source that
were not returned, for both TTL-triggered and revision-triggered refresh.
Previously the TTL path removed only expired claims, so a longer-lived cached
field could survive a newer observation that no longer returned it.

Regression tests cover empty and partially populated observations, removed
identity-key/derived-authority bookkeeping, preservation of other sources and
internal identity, real evaluator allow-to-deny for a removed permission, and
persisted request-path replacement at either endpoint. Failed lookups retain
their existing indeterminate/deny behavior; they are not interpreted as empty
successful observations. This is existing lookup-contract enforcement, not
the deferred namespace envelope or delegated-source authorization work.

Validation: both regressions failed before the fix and pass afterward. Full
Rust workspace tests pass: **703 passed, 9 ignored**. Rust formatting and diff
checks pass. The temporary worktree dependency-path adjustment was restored;
no dependency, shared-schema, deployment or production-setting change is made.

## Maturation increment: existing-visa lifetime bounds

Visa rechecks now reuse the issuance expiration calculation for the selected
allow hit, rather than checking only source-observation metadata. A policy
condition, authentication identity/authority, provided-services claim or source
observation that expires before the stored visa queues revocation, even when
the refreshed policy evaluation still allows the flow now. Longer validity
does not extend an issued visa. Unrelated ordinary fields do not become new
lifetime dependencies; source-observation bounds remain conservative across
both actors.

The regression first reproduced an unrevoked visa whose still-matching
source-qualified condition expired before its stored lifetime. Coverage starts
with real issued visas and exercises shorter/longer condition, identity,
authority and observation bounds at either endpoint, the provider's services
bound, unrelated-field isolation, queued revocation and unchanged stored expiry.
This does not promise revocation delivery before a deadline: relay delivery
remains asynchronous, and no new background expiry scheduler is introduced.

Validation: the regression failed before the fix and passes afterward. Full
Rust workspace tests pass: **704 passed, 9 ignored**; formatting and diff checks
pass. The temporary worktree dependency path was restored with no manifest or
lockfile changes. No deployment or shared-schema change is claimed.

## Maturation increment: strict source-response validity

The optional REST `expires_unix` field now distinguishes omission (legacy
configured-TTL behavior) from explicit `null` (invalid). A present value must
decode as an unsigned integer, then pass the existing future/representable
deadline checks. Previously `null` decoded as omission, including on empty
responses, silently dropping source-observation metadata.

Malformed response JSON now reports `AttributesIndeterminate` with the source
ID and decoding error instead of a generic serialization error. No partial
attributes are mapped. Existing request/reconciliation guards continue to
deny/queue revocation rather than evaluating this as known absence.

New regressions reproduce null acceptance before the fix, then cover strict
decoder types and duplicate deadlines plus actual verified-HTTPS responses:
null on empty/populated observations, wrong types, integer overflow, unknown
fields, invalid attributes and truncated JSON. Existing HTTPS and mapping tests
retain legacy omission and valid bounded-response behavior. No new dependency
or delegated-source/envelope protocol is introduced.

Validation: both new regressions failed before the fix and pass afterward.
Full Rust workspace tests pass: **706 passed, 9 ignored**. Workspace formatting
and diff checks pass; the temporary dependency-path adjustment was restored.
No deployment, manifest/lockfile or shared-schema change is made.

## Maturation increment: unambiguous REST response claims

The REST response decoder now rejects duplicate attribute names before mapping.
Previously its JSON object decoded directly into a map, silently discarding all
but the last repeated claim before canonical-key conflict detection could run.
Conflicting repeated claims therefore bypassed the intended rejection rule.

A verified-HTTPS regression reproduced this overwrite before the fix. Coverage
now rejects both conflicting orders, identical repeats, JSON-escaped equivalent
names, repeated unmapped names and a duplicate top-level attributes field.
Valid distinct names and identical canonical aliases remain accepted; existing
alias-conflict and freshness tests continue to pass. The whole response fails
as source-labelled indeterminate, with no partially installed claims. This
increment applies to REST responses, not file-source JSON parsing.

Validation: full Rust workspace tests pass: **708 passed, 9 ignored**.
Workspace formatting and diff checks pass. The temporary worktree dependency
path was restored; no manifest, lockfile, shared-schema or deployment change
is made.

## Maturation increment: file-source claim cardinality

File-source lookups now reject a matched single-valued mapped claim unless it
contains exactly one value, consistent with REST lookups. Previously zero
values manufactured an empty-string claim and multiple values silently chose
the first. The whole lookup now fails as `AttributesIndeterminate`, identifying
the source and canonical key, rather than returning partial or invented claims.

A new regression reproduced empty-list acceptance before the fix. It covers
zero values, both conflicting orders, identical repeated values, successful
reload/recovery after each invalid lookup, preserved multi-valued/tag semantics,
explicit empty-string values, and isolation from unmatched identity records.
Valid JSON with invalid claim cardinality may still be loaded/flushed; the
claim is rejected when its identity is looked up. This does not change file
snapshot/revision atomicity or implement duplicate-name rejection for file JSON.

Validation: full Rust workspace tests pass: **709 passed, 9 ignored**.
Workspace formatting and diff checks pass. The temporary dependency path was
restored with no manifest/lockfile changes. No deployment or shared-schema
change is made.

## Maturation batch: file decoding, reload lifecycle and enforcement

Three related steps now extend the file-source hardening:

1. **Unambiguous file JSON.** Duplicate identity keys, subject entries and
   attribute names are rejected during decoding, including identical repeats,
   escaped equivalent names and unmapped attributes. REST and file decoding
   reuse one unique-name map decoder. Valid runtime snapshot structures and
   existing canonical-alias rules are unchanged.
2. **Explicit reload failures.** Unreadable or malformed files report
   `AttributesIndeterminate` with source ID and file path. Initialization still
   wraps this as `TrustedServiceInit`. Neither failed flushes nor failed automatic
   reloads install partial data or advance the revision. A rejected explicit
   flush preserves the still-valid snapshot; expiry-driven lookups fail rather
   than return the expired snapshot. Correcting the file recovers normally.
3. **Real-provider enforcement proof.** Tests issue a visa through a real
   file store, reload malformed single-valued claims at either endpoint, verify
   new requests deny and the existing visa queues revocation, then repair the
   file and verify new requests recover without canceling the old revocation.

New decoder/reload regressions failed before the fixes. Lifecycle coverage
includes duplicate JSON, invalid field types, truncated JSON and missing files,
checks snapshot pointer/revision preservation, and exercises automatic recovery.
The existing lookup-identity contract for non-single-valued claims remains
unchanged; no new identity or namespace-envelope semantics are introduced.

Validation: trusted-source tests pass (**34 passed, 1 ignored**) and the full
Rust workspace passes (**712 passed, 9 ignored**). Workspace formatting and diff
checks pass. The temporary worktree dependency path was restored exactly; no
manifest/lockfile, shared-schema or deployment changes are made.

## Maturation batch: reference-provider input consistency

The Go reference provider now validates JSON before typed decoding so repeated
object names and nulls cannot disappear through Go's map/struct decoding.
This applies to file snapshots and lookup requests, including JSON-escaped
equivalent names. Requests must include an explicit identities array; empty
arrays remain valid. Unknown request fields, trailing data and existing
identity-count/body-size limits continue to reject before provider lookup.

File snapshots reject duplicate keys at each level, null objects/value arrays
and null array elements instead of manufacturing known absence or empty-string
claims. Invalid files return 503 without partial attributes; repair is visible
at the next lookup. Source-side field names and subjects remain case-sensitive.
Canonical single-valued mapping validation still belongs to VS.

Regressions first reproduced duplicate/null file acceptance and malformed
requests reaching the provider. Coverage includes HTTP failure/recovery,
zero provider calls on invalid requests, empty-array compatibility, exactly
1 MiB versus one byte over, 64 versus 65 identities and bounded JSON nesting.
The existing 64 KiB request and 1 MiB file bounds remain; JSON nesting is now
explicitly capped at 64 levels. No Simulator dependency or namespace-envelope
protocol is added.

Validation: provider/change-watcher `go test -race`, provider `go vet`, focused
boundary tests and changed-file formatting pass. Required `make test` was run:
all 11 operator-stack checks pass, but the full Go suite fails in the untouched
web-dashboard package (13 tests) on missing runtime/policy/LDIF/manifest fixtures,
a missing launcher shell helper and the nested worktree's unresolved default
`zpr-common` path during Rust interop. The full dashboard suite is **not green**;
these failures were not bypassed or repaired in this provider-only batch.
No Rust, dependency manifest/lockfile, shared-schema or deployment changes are
made by this batch.

## Five-step maturation batch: exact identity text at the provider boundary

1. **Exact request-field names.** Request decoding now uses exact map keys before
   constructing typed identities, rather than Go's case-insensitive struct-field
   matching. Only `identities`, `key` and `value` are accepted; mixed-case aliases
   cannot overwrite an earlier field. Escapes resolving to a canonical field
   name remain valid. The regression reproduced all six case-variant requests
   reaching the provider before the fix.
2. **Identity text preservation.** Requests and provider file snapshots reject
   invalid UTF-8 and unpaired UTF-16 surrogate escapes. Go previously replaced
   them silently with U+FFFD, allowing invalid bytes to match a valid but different
   replacement-character subject. A regression reproduced this on both request
   and file paths. Valid Unicode, surrogate pairs, explicit U+FFFD, escaped
   backslashes and literal backslash-u sequences remain valid.
3. **Exact byte-boundary proof.** Tests exercise request sizes of 65535, 65536
   and 65537 bytes, permitting the documented limit and rejecting one byte over
   without calling the provider. Valid raw/escaped Unicode identities resolve
   to the same file subject without changing canonical names.
4. **Authenticated transport proof.** A new verified mutual-TLS test exercises
   valid bounded responses, malformed case/text rejection, exact body limits,
   duplicate/null/invalid-text file failures and repair recovery. Failed
   requests return 400, unavailable file observations return 503, and neither
   returns success-shaped attributes or expiration fields.
5. **Validation and scope.** Provider/change-watcher race tests, provider vet,
   changed-file formatting and all 11 operator-stack tests pass. Required
   `make test` still fails in the same 13 untouched web-dashboard tests documented
   above; the full dashboard suite is not green. No failing tests were skipped
   or modified. Rust and dependency files are unchanged by this batch; the last
   Rust workspace validation remains 712 passed, 9 ignored. No deployment,
   shared-schema or namespace-envelope change is made.

## Five-step maturation batch: complete LDAP observations

1. **Search completeness.** Attribute lookups reject unresolved referrals at
   both person and group stages, nil results, and LDAP errors even when partial
   entries accompany them. Previously referrals were ignored, allowing an
   incomplete result to assert absence. Referrals are not followed with the
   provider's bind credentials.
2. **Role conflict consistency.** Derived group roles use the existing conflict
   checker instead of unconditionally overwriting a listed direct `role`.
   Agreeing and group-only roles remain valid. Conflicting observations fail
   without partial claims.
3. **Unambiguous attributes/groups.** Requested LDAP attributes are read using
   LDAP case-insensitive names with duplicate detection, rather than first-value
   selection across duplicate attribute records. Matching groups must expose
   exactly one nonempty `cn`; missing/empty/multiple group names fail instead
   of being silently skipped or truncated. Multi-entry identity lookup retains
   the existing union/conflict contract, not a new subject-uniqueness rule.
4. **Cancellation boundaries.** An already-canceled lookup does not dial LDAP;
   cancellation before or after either search rejects its results. This does
   not interrupt an in-flight LDAP search; the existing five-second operation
   timeout still bounds it. No cancellation-latency guarantee is claimed.
5. **Regression and HTTP recovery.** Four regression groups reproduced
   referral acceptance, role overwrite, duplicate/group-name acceptance and
   successful canceled lookups before the fixes. HTTP tests verify 503 with no
   success-shaped attributes/deadline on referrals, size-limit partial errors,
   nil results, group failure, duplicate attributes and cancellation, followed
   by valid recovery. Unknown subjects and case-insensitive LDAP attributes
   remain supported.

Validation: provider/change-watcher race tests, provider vet and formatting pass,
as do all 11 operator-stack checks. Required `make test` again reports the same
13 untouched web-dashboard failures; the full suite is not green. Tests use
controlled LDAP search results, not a live-directory deployment/acceptance run.
Rust/dependencies/shared schemas are unchanged by this batch; no deployment or
namespace-envelope identity change is made.

## Five-step maturation batch: atomic provider observations

1. **Outgoing claim validation.** The HTTP boundary rejects nil attribute maps,
   nil value arrays and invalid UTF-8 names/values returned by providers. This
   closes Go encoder replacement of malformed LDAP text, not merely malformed
   file input. Empty objects, empty arrays, empty strings and valid Unicode
   (including explicit U+FFFD) remain valid.
2. **Consumer-aligned response size.** Responses are fully encoded before
   success headers and limited to the VS consumer's 1 MiB wire bound, including
   the existing trailing newline. Tests measure exactly 1048575, 1048576 and
   1048577 bytes in legacy and source-deadline modes, plus JSON-escaping
   expansion and the combined size of multiple claims. Oversized observations
   return 503 without partial attributes. This does not bound provider memory
   allocation or the temporary encoding buffer.
3. **Publication guards.** Already-canceled requests do not call the provider;
   cancellation is checked again after encoding. Source validity is also
   rechecked after encoding, before writing. These checks do not interrupt
   in-flight I/O or eliminate the race with cancellation after the final check.
4. **Failure/recovery contract.** Invalid response data returns 503 with no
   attributes/deadline fields. A corrected observation recovers without losing
   empty/Unicode claims. Controlled LDAP search tests prove invalid returned
   text is rejected, not silently rewritten. All lookup responses, including
   failures, use no-store. Write failures are explicitly logged; once a write
   starts, delivery cannot be rolled back.
5. **Verification.** Four new response regression groups failed before the fix.
   Provider/change-watcher race tests, provider vet, formatting and all 11
   operator-stack checks pass. Required `make test` still reports the same 13
   untouched web-dashboard failures; the full suite is not green.

No live LDAP deployment, relay-delivery guarantee, Rust/dependency/schema change
or namespace-envelope change is claimed. The response test coverage lives in a
focused test file beside the provider.

## Five-step maturation batch: HTTPS consumer transport contract

1. **Fixed lookup endpoint.** VS disables automatic redirects. A verified-HTTPS
   regression reproduced a redirected second request before the fix; 301, 302,
   303, 307 and 308 now produce indeterminate lookup errors without contacting
   the redirect target. Configure the provider's final origin directly.
2. **Complete HTTP observations.** Only HTTP 200 can supply an authoritative
   observation. The previous status helper rejected 4xx/5xx but accepted 300
   bodies as claims. The consumer now rejects accepted/partial/multi-status
   responses as well as redirects/errors, before decoding the body. An empty
   observation still requires HTTP 200 plus the attributes object, not 204.
3. **Exact consumer wire limits.** Verified-HTTPS tests check 1048575, 1048576
   and 1048577 decoded-body bytes under Content-Length and multi-chunk transfer
   encoding. Both accepted boundaries and the over-limit rejection pass using
   the existing bounded-body implementation; no size-limit relaxation is added.
4. **Malformed transport data.** Verified-HTTPS tests confirm invalid UTF-8,
   lone high/low surrogate escapes and a body truncated before its declared
   Content-Length remain indeterminate. These are proof of existing decoder
   behavior, not a new text-normalization layer.
5. **Validation.** Redirect/status regressions failed before the fixes; focused
   HTTP consumer tests pass afterward. Full Rust workspace and formatting
   results are recorded below. These tests verify server TLS but do not claim
   a new consumer mTLS acceptance proof; existing Go provider mTLS tests remain
   the separate client-certificate enforcement evidence.

No provider deployment, identity envelope, shared schema or dependency change
is introduced. Transport rejection uses the existing indeterminate request and
revocation guards.

Validation: full Rust workspace tests pass: **716 passed, 9 ignored**.
Workspace formatting and diff checks pass; the temporary worktree dependency
path was restored with no manifest/lockfile changes. Go code is unchanged by
this batch; the last full dashboard result remains the 13 documented
web-dashboard failures, not a green full Go suite.

## Five-step maturation batch: consumer mutual-TLS evidence

1. **Actual client-certificate enforcement.** A new isolated TLS fixture requires
   a verified client certificate, inspects the actual peer certificate and
   decodes the lookup request. The production HTTP store constructor/client
   completes a valid lookup, replacing the earlier server-only TLS proof with
   direct consumer mTLS evidence. The missing-certificate negative control
   deliberately constructs a client without identity; all other cases use the
   production client unchanged.
2. **Server identity/trust rejection.** An unrelated server CA and a hostname
   absent from the server certificate fail as indeterminate before application
   lookup. Both single-root and multi-root PEM bundles work.
3. **Client identity rejection.** The server rejects missing, wrong-root,
   expired and wrong-purpose client credentials, with no application lookup.
   Malformed client certificate/key and mismatched key material fail store
   initialization.
4. **Explicit CA configuration errors.** Tests exposed that the reqwest
   single-PEM parser could accept non-certificate input as an empty trust set.
   VS now parses a bundle and explicitly rejects zero certificates. Empty,
   junk and invalid-DER CA files fail initialization with source context;
   a valid CA later in a bundle remains usable.
5. **Validation.** The new CA-initialization regression failed before the fix;
   all 15 focused HTTPS consumer tests pass afterward. Full Rust workspace and
   formatting results are recorded below. Certificates/keys exist only in
   isolated temporary test directories and are not committed.

This is transport acceptance evidence, not production deployment, CA rotation,
CRL/OCSP revocation enforcement or the deferred CA/envelope architecture.
Go code/dependencies/shared schemas are unchanged by this batch.

Validation: full Rust workspace passes (**718 passed, 9 ignored**); formatting
and diff checks pass. The first full run exposed an intermittent early-close
error in the previous raw HTTPS fixture. It now reads the complete request and
shuts TLS down cleanly; the wire-limit test passed three consecutive focused
runs and then the full suite. The temporary dependency path was restored with
no manifest/lockfile changes. The last full Go result remains the documented
13 web-dashboard failures.

## Five-step maturation batch: transport lifetime and diagnostics

1. **Server certificate expiry/purpose.** The production-client mTLS matrix now
   verifies expired server certificates and wrong server EKU are rejected before
   any application lookup, alongside the previous trust/hostname checks.
2. **Origin and TTL validation.** Tests verify plaintext origins, userinfo, paths,
   queries, fragments and malformed URLs fail before credential files are read.
   Zero/exactly-60-second TTLs remain rejected without touching credentials.
3. **TLS configuration diagnostics.** A new regression reproduced generic
   missing-file errors without source/path. TLS reads now use `IoContext` to
   identify source, CA/certificate/private-key purpose and path while preserving
   the original I/O error. Credential contents are never logged.
4. **Real timeout boundaries.** An isolated server stalls separately during TLS
   negotiation, before response headers and during an incomplete response body.
   Each production client lookup fails indeterminate under its unchanged
   five-second timeout. Tests use an eight-second outer watchdog and assert
   failure did not happen immediately; helper tasks are explicitly released
   and joined after each lookup. This is not an end-to-end relay latency bound.
5. **Regression validation.** The full Rust workspace passed with **721 passed,
   9 ignored**, and workspace formatting passed. No Go behavior,
   dependency, schema, deployment or deferred CA/envelope protocol is changed.

## Ten-step maturation batch: certificate chains and consumer recovery

This batch adds evidence for the existing consumer contract; the focused tests
did not expose a new production defect or require a production behavior change.

1. **Future server certificates.** A trusted, correctly named server certificate
   whose validity has not begun fails before an application lookup.
2. **Future client certificates.** The mTLS listener rejects a not-yet-valid
   client certificate even when its issuer and authentication purpose match.
3. **Server intermediate chain.** A leaf issued by an intermediate succeeds when
   the server supplies that intermediate and fails when it is omitted. Only the
   root is configured as the consumer trust anchor.
4. **Client intermediate chain.** A leaf-plus-intermediate client PEM succeeds
   against a root-only server trust store; the same issuance without the
   intermediate fails. Successful handshakes verify the actual peer leaf.
5. **IP SAN verification.** A matching IP SAN accepts an IP-address origin;
   the existing DNS-only certificate/IP-origin negative control remains.
6. **Exact request identities.** Complete mTLS requests are decoded and compared
   against the entire expected JSON object, including multiple identities,
   duplicate keys as separate array entries, case distinctions, quotes,
   backslashes, a newline, non-BMP text and a literal escaped surrogate sequence.
   Empty identities remain an explicit array. These are serialization checks,
   not a new issuer-qualified subject-link or delegated-access contract.
7. **HTTP rejection recovery.** Valid/rejected/repaired sequences exercise
   401, 403, 429 and 503 responses, even when the rejected body contains claims.
   Rejection returns source-labelled indeterminate, never the previous success.
8. **Invalid observation recovery.** Duplicate names, null/expired source
   deadlines and invalid single-value cardinality fail atomically. The next
   valid observation returns the repaired claim and exact source deadline.
9. **Interrupted response recovery.** Disconnects before headers, truncated
   Content-Length bodies and unterminated chunked bodies fail indeterminate;
   the same production client accepts the subsequent repaired observation.
   Recovery sequences do not flush or reconstruct the client.
10. **Revision lifecycle.** The initial source revision is nonzero. Successful,
    empty and failed lookups leave it unchanged; repeated explicit flushes
    advance it monotonically. Revision allocation is not provider data versioning
    or proof of downstream persistence, reconciliation or relay delivery.

The new fixture verifies peer credentials and all three complete request bodies,
uses bounded watchdogs, and joins/aborts its server task on completion/timeout.
No Go behavior, shared schema, dependency, deployment or deferred CA/envelope
protocol changes are included.

Validation: the focused consumer suite passed **22 tests**. The full Rust
workspace passed **725 tests, 9 ignored**; workspace formatting passed. Cargo
manifest/lockfile contents were restored unchanged after the worktree-only path
adjustment. No Go code changed or Go suite was rerun; the prior full dashboard
result remains the documented 13 unrelated failures.

## Ten-step maturation batch: manager lifecycle and source isolation

1. **Retired full-lookup success.** Reproduced a successful old lookup marking a
   failed replacement source working. Health now belongs to the service-list
   snapshot which performed the lookup, not a manager-global source-name map.
2. **Retired full-lookup failure.** Reproduced an old failure replacing a new
   source's working state. The same snapshot-scoping fix prevents this case.
3. **Targeted lookup replacement.** Reproduced the same defect through targeted
   lookup; both success/failure directions now preserve the replacement status.
4. **Removal and re-addition.** Old full/targeted completions cannot leak status
   into a removed/re-added source. Replacement status begins unverified with no
   inherited timestamps. Service list and its health map are swapped together.
5. **Targeted real-file flush.** Reloading company changes only its revision and
   data; lab retains its revision and claims. Actor staleness names only company.
6. **Partial flush-all failure.** A broken first source does not prevent reloading
   the second. The failed source preserves its revision and still-valid cached
   snapshot; the repaired peer advances and returns its new value.
7. **Unknown source operations.** Unknown targeted lookup/flush returns the
   existing explicit not-found error without changing configured source health
   or revision.
8. **Replacement/address lifecycle.** A replacement real-file store under the
   same source name is stale for an actor whose old revision is recorded; an
   unchanged peer stays current. Forgetting a disconnected actor's revisions
   makes both sources stale for reuse of the address.
9. **Qualification transitions.** Single/multi/single service-list transitions
   exercise full and targeted real-file lookups. Legacy aliases appear only in
   single-source mode; canonical qualified names, multi-values, valueless tags,
   provenance and alias deadlines remain consistent. This does not claim to
   rewrite attributes already persisted on actors during policy replacement.
10. **Independent health/recovery.** Empty success remains working; a failing
    peer does not change it. Failure preserves the source's last-success time
    and subsequent success recovers its status. Same-source health updates use
    one map entry operation so concurrent failures cannot erase a recorded
    success timestamp through a read/insert race.

The new deterministic race fixtures release old lookups only after replacement
lookups finish. Three regressions failed before the snapshot fix. This changes
health publication only: already-running lookups still return to their original
callers, and no cancellation, stale-policy authorization prevention, selective
invalidation or relay-delivery guarantee is introduced.

Validation: all **10 new lifecycle regressions** pass; the manager suite passed
**16 tests, 1 ignored**. Full Rust workspace validation passed **735 tests,
9 ignored**, with workspace formatting passing. Cargo manifests/lockfile were
restored unchanged after the temporary worktree path adjustment. No Go code was
changed or Go validation rerun; the previously documented 13 unrelated full
dashboard test failures remain unresolved.

## Ten-step maturation batch: actor refresh and persistence lifecycle

This batch adds application-level regression evidence; the ten focused cases
passed without changing production behavior.

1. **Changed source mapping.** A replacement source revision removes unexpired
   old canonical/qualified keys and their identity-key bookkeeping while adding
   the new mapping. A committed refresh then takes the no-round-trip hot path.
2. **Single-to-multi refresh.** Replacement revisions trigger refresh of both
   sources. Old unqualified user claims and derived authority disappear;
   source-qualified claims and authority remain separate for each source.
3. **Multi-to-single refresh.** Refreshing a replacement survivor restores legacy
   aliases with the same values/deadlines as their qualified counterparts. The
   removed source in this fixture vended nothing; this is not retired-claim
   cleanup evidence.
4. **Bounded empty replacement.** Replacing a user observation with freshness
   metadata removes all its user claims/authority, retains exact empty-observation
   deadlines under both aliases, and remains current after revision commit.
5. **Shortened source validity.** A targeted revision refresh shortens company
   claims and derived authority while lab's claims/authority retain their exact
   deadlines. Lab is not queried again.
6. **Mixed successful/failed sources.** A failed stale lab source is stripped and
   reported indeterminate while company refreshes. Only lab remains stale after
   commit; lab recovery rederives authority without requerying company.
7. **TTL failure/retry/recovery.** The first failed TTL refresh retains the expired
   cache and reports indeterminate. Committing its retry sentinel makes the next
   failed revision refresh strip it; later recovery installs the new observation
   and returns to the committed no-op path.
8. **Flush during lookup.** A controlled source flushes before its lookup returns.
   Committing the revision captured before the call cannot mark that newer
   revision current; another refresh is still required.
9. **Discarded local refresh.** Discarding a refreshed working clone leaves the
   original copy untouched and revisions stale. Retrying from that original
   re-fetches before revision commit. This models a discarded result, not an
   injected database-write failure.
10. **Both-endpoint database readback.** The real refresh-and-persist path runs
    against both actors in an assembly. After a source flush, reloading each
    actor proves old aliases are removed and new claims/derived authority
    deadlines are shortened exactly; revisions are current only after persistence.

Alias-transition evidence assumes replacement stores have new revisions (as
the policy manager constructs them when declarations change). These tests do
not establish cleanup of every cached claim belonging to a removed source,
policy-generation consistency across in-flight refreshes, transactional
cross-actor updates, downstream visa rechecks or relay delivery. The previously
documented envelope, explicit subject links and selective dependencies remain
deferred. No shared schema, dependency or Go behavior changes are included.

Validation: all **10 new actor lifecycle tests** pass. Full Rust workspace
validation passed **745 tests, 9 ignored**; workspace formatting passed. Cargo
manifest and lockfile were restored unchanged after the worktree-only path
adjustment. No Go code changed or Go suite was rerun; the documented 13 unrelated
dashboard test failures remain unresolved.

## Ten-step maturation batch: exact visa deadline dependencies

This batch adds regression evidence for the existing lifetime and recheck
contract. All ten focused cases passed without a production behavior change.

1. **Competing support minima.** Rotate the exact earliest deadline among client
   and service condition claims, both endpoint identity claims, user/device
   authentication authority, the provider's service set and both endpoints'
   observation markers. Each gate independently wins against all others.
2. **Reverse-direction role isolation.** Correct client/provider keys bound both
   hit directions; deliberately earlier copies on the wrong endpoint do not
   become condition or service-set dependencies.
3. **Freshness marker provenance.** A qualified marker must match its stamped
   source to contribute observation freshness. Similar ordinary names and
   mismatched qualified suffixes do not become reserved freshness metadata.
   This is an internal classifier test, not wire-level permission to forge keys.
4. **Discrepant alias deadlines.** If legacy and correctly qualified observation
   aliases have different bounds, the earlier wins in either ordering. One later
   alias cannot erase the other's bound.
5. **Selected policy index.** Two communication policies with different condition
   keys use only the selected hit's keys; another policy's shorter ordinary
   condition deadline does not become an unrelated dependency.
6. **Negative-condition support.** A present claim used by a not-equal condition
   still contributes its expiration. This tests lifetime calculation, not a
   new evaluation truth-table implementation.
7. **Already expired support.** Expired condition, observation and authentication
   deadlines remain exactly in the past; the lifetime calculation never clamps
   them upward into a usable authorization period.
8. **Tied observation minima.** Removing one of two equally short observations
   does not extend the other endpoint's remaining bound.
9. **Exact existing-visa recheck.** A real issued visa is rechecked after persisted
   observation changes at each endpoint. A deadline one nanosecond earlier
   revokes; equal and one nanosecond later allow on the unchanged route.
10. **Queued revoke survives recovery.** Real issuance followed by repeated
    short-validity attribute sweeps queues exactly that visa for revoke.
    Restoring longer validity and sweeping repeatedly does not cancel it,
    change its stored expiration or advance its policy-check generation.
    This exercises validity recovery, not recovery from a provider outage.

No new policy-principal protocol, selective dependency index, source removal
cleanup, stale-policy cancellation, deployment or shared-schema change is
introduced. These tests cover desired queued state, not relay acknowledgement
or a guaranteed revocation delivery deadline.

Validation: **10 new deadline regressions** pass. Full Rust workspace validation
passed **755 tests, 9 ignored**, and workspace formatting passed. Cargo manifest
and lockfile were restored unchanged after the temporary worktree path
adjustment. No Go code changed or Go tests were rerun; the documented 13
unrelated dashboard failures remain unresolved.

## Ten-step maturation batch: unambiguous mapping declarations

Five reproduce-first regressions failed before this batch's declaration fix.
The factory now validates mapping consistency before building stores or
publishing a new policy. Shared schemas and the shared mapping parser are
unchanged; validation/2 remains outside ordinary attribute-store construction.

1. **Repeated raw field/different target.** Both declaration orders previously
   passed and let the mapper select its first entry. File and REST declarations
   now reject a raw field assigned conflicting canonical targets.
2. **Repeated raw field/different cardinality.** Single/multi declarations for
   the same raw field previously depended on order; both orders now reject.
3. **Distinct aliases/different cardinality.** Distinct raw fields resolving to
   the same canonical key must agree on emission shape. This prevents the
   single-value contract changing according to which alias is returned.
4. **Blank wire-level raw names.** Directly decoded records with empty or
   whitespace-only service field names previously bypassed the text parser's
   nonempty-name rule. Both APIs reject them with a source-labelled parameter
   error before store construction.
5. **Exact repeats remain compatible.** Repeating the same raw-to-canonical
   mapping still validates for both APIs and emits one file-backed claim.
6. **Agreeing distinct aliases remain compatible.** Different raw names with
   the same canonical key and shape remain valid; agreeing file values emit
   one claim. Runtime value disagreements retain the existing fail-closed rule.
7. **Raw field casing remains significant.** Case-distinct file fields can map
   to different canonical targets without being conflated.
8. **Multi-values and tags remain intact.** Agreeing multi-value aliases preserve
   their full value list and an independent tag remains valueless.
9. **Existing namespace boundaries.** Exact source-ID length limits, ASCII
   source names, reserved separator rejection and qualified-key rejection stay
   enforced. Both file and REST reject forged qualified mapping targets.
10. **Rejected policy update is non-publishing.** Installing either target or
    alias-cardinality conflicts now fails while the current policy generation,
    exact current container, recorded actor revision and live store lookup
    remain unchanged.

Compatibility change: configurations with conflicting mapping declarations or
blank wire-level raw names are now rejected rather than choosing a declaration
or accepting a field name that text authoring cannot produce. Exact duplicates
are deliberately not rejected. No case folding, raw-name rewriting, value-set
normalization, identity cardinality change or policy-principal protocol is
introduced.

Validation: all **10 new declaration/mapping regressions** pass; the combined
factory suite passed **15 tests**. Full Rust workspace validation passed
**765 tests, 9 ignored**, with workspace formatting passing. Cargo manifest and
lockfile were restored unchanged after the temporary worktree path adjustment.
No Go code changed or Go suite was rerun; the documented 13 unrelated dashboard
failures remain unresolved.

## Fifteen-step maturation batch: file and verified-HTTPS semantic parity

This batch compares actual policy-built file stores against REST responses sent
through the production HTTPS constructor and lookup path. It reuses the existing
verified-server fixture with parameterized mappings; it does not replace network
decoding with a mapping-only proxy. All fifteen cases passed without a production
behavior change.

1. **Explicit empty single value.** One empty string remains a present scalar,
   not an absent claim or an empty value array.
2. **Zero scalar values.** An empty array for a single-value mapping rejects the
   entire observation even alongside an otherwise valid claim.
3. **Multiple scalar values.** Both value orders fail rather than choose a winner.
4. **Repeated identical scalar values.** Duplicate identical values still violate
   exactly-one cardinality; they are not silently deduplicated into validity.
5. **Empty multi-value versus omission.** An explicitly empty multi-valued field
   remains a canonical attribute; an omitted field produces none.
6. **Exact multi-values.** Order, duplicate entries and explicit empty strings
   survive unchanged; neither consumer introduces set normalization.
7. **Tags.** Presence emits the same valueless canonical tag whether the provider
   payload is empty or populated.
8. **Agreeing aliases.** Distinct agreeing raw fields collapse to exactly one
   canonical claim.
9. **Conflicting aliases.** Both value assignments reject atomically, including
   when another unrelated valid claim is present.
10. **Reordered multi-value aliases.** Arrays with equal members in different
    orders retain the existing conflict behavior, not implicit set equality.
11. **Exact text.** Case-distinct field names stay separate; quotes, backslashes,
    newline and a non-BMP surrogate-pair encoding round-trip exactly.
12. **Unmapped cardinality.** Well-formed unmapped arrays do not acquire mapped
    scalar cardinality rules and do not become canonical claims.
13. **Strict unmapped value shapes.** Null, booleans, numbers, scalar strings,
    objects and non-string array elements reject even on unmapped fields.
14. **Strict decoded names/text.** Escaped-equivalent duplicate names, repeated
    unmapped names and unpaired surrogates in unmapped values all reject.
15. **Failed reload and repair.** Repeated malformed explicit reloads preserve a
    valid file snapshot and its revision; equivalent REST observations fail
    indeterminate. Repair advances the file revision and both consumers return
    exactly the repaired canonical claim, not the previous value.

Successful comparisons check the complete canonical-key/value map, absence of
duplicate output keys, exact source provenance and expiration within the
configured TTL's before/after timing bounds. The existing HTTPS fixture uses a
3600-second TTL and file declarations use 600 seconds; deadlines are checked
against each setting, not incorrectly required to match across consumers.

Preserved differences: file alias conflicts use the existing parameter-error
variant; REST conflicts are source-labelled indeterminate. Explicit file flush
failure preserves a still-valid cache, whereas a REST lookup has no successful
observation to return. No claim is made that expired automatic file reloads may
use that cache. No new envelope/identity contract, dependency, schema, Go behavior,
live deployment or relay-delivery evidence is introduced.

Validation: **15 new parity tests** pass. Full Rust workspace validation passed
**780 tests, 9 ignored**; workspace formatting passed. Cargo manifest and lockfile
were restored unchanged after the temporary worktree dependency-path adjustment.
No Go code changed or Go suite was rerun; the previously documented 13 unrelated
dashboard test failures remain unresolved.

## Fifteen-step maturation batch: user-authority derivation evidence

All fifteen focused cases passed without production behavior changes. This
batch exercises the existing bootstrap-era authority convention; it does not
introduce a policy principal, subject binding or source-side visibility rule.

1. **Qualified-only support.** Source-qualified user claims derive only qualified
   authority, never a legacy alias.
2. **Legacy-only support.** Unqualified user claims derive only legacy authority,
   without manufacturing a qualified user record.
3. **Mixed aliases.** Both derived authority aliases share the earliest supporting
   user deadline regardless of input order.
4. **Device deadline isolation.** A shorter unrelated device claim does not
   shorten derived user authority.
5. **Freshness-only absence.** Observation freshness metadata alone never creates
   a user record or user authority.
6. **Other-source qualified claims.** A peer suffix does not establish or shorten
   authority for the current source.
7. **Exact namespace/suffix matching.** Similar user prefixes and source names,
   including case differences, do not become current-source support.
8. **Explicit legacy authority.** A source-provided legacy authority suppresses
   derivation without rewriting its custom value or deadline.
9. **Explicit qualified authority.** An explicitly supplied matching qualified
   authority likewise suppresses derivation without rewriting it.
10. **Expired support.** Expired user support yields exactly expired authority,
    never a renewed validity period.
11. **Real single-source store.** Factory-built file lookup plus manager
    qualification derives both authority aliases at the supporting claim's
    exact expiration.
12. **Real multi-source stores.** Two stores derive independent qualified
    authorities, each with its own source and exact supporting deadline, and
    no legacy authority.
13. **Tags, empty arrays and omission.** Present user tags and empty multi-valued
    user claims support authority under the existing presence-based contract;
    device tags and omitted user fields do not, in either source mode.
14. **Real explicit-authority mappings.** Source-owned custom authority values
    survive file mapping and single/multi-source qualification; no derived
    authority is added.
15. **Authentication/visa integration.** A derived legacy authority bounds actor
    authentication and visa calculation exactly even when the selected
    communication policy has no conditions referencing the supporting user
    claim, at either endpoint and in either hit direction.

Compatibility limits: explicit authority retains its own source-supplied
deadline even if another user claim expires earlier; derivation deliberately
does not override it. Qualified authority is not a new legacy authentication
alias. The internal derivation helper trusts the caller's source grouping;
suffix matching tests are not proof of issuer-qualified subject links or
validation of arbitrary mixed-source input. Real-store tests cover the
production manager grouping, not admission, durable persistence or relay
delivery. No shared schema, dependency, Go behavior or deferred envelope work
changes are included.

Validation: **15 new authority lifecycle tests** pass, including a focused rerun
after extending present-tag/empty-multivalue evidence to both source modes. Full
Rust workspace validation passed **795 tests, 9 ignored**; workspace formatting
passed. Cargo manifest and lockfile were restored unchanged after the temporary
worktree path adjustment. No Go code changed or Go suite was rerun; the documented
13 unrelated dashboard failures remain unresolved.

## Fifteen-step maturation batch: policy/source publication lifecycle

All fifteen focused cases passed without production changes. Tests use the
real PolicyMgr/PolicyRepo paths backed by FakeDb and temporary file sources.
Rejected updates check active and persisted container bytes/generation, a held
policy snapshot, actor source-revision currency, unchanged health before another
lookup, and working cached claim names/values/provenance after rejection.

1. **Unsupported API.** Rejection does not publish or persist the candidate.
2. **Missing trusted-service record.** A service declaration without its record
   leaves the current policy and source untouched.
3. **Invalid TTL.** Zero, one-second and exactly-60-second file TTLs all reject
   without moving live or persisted state.
4. **Invalid source name.** Path-like, overlength and reserved-separator names
   reject before they can become live file sources.
5. **Reserved target.** A forged source-qualified mapping target rejects without
   changing the current policy or revision record.
6. **Conflicting mappings.** Both duplicate-raw-target and canonical-cardinality
   conflicts reject non-publishing through the full policy manager.
7. **Missing new source file.** A candidate referencing an absent file cannot
   displace the functioning source.
8. **Malformed replacement file.** Invalid JSON, null values and duplicate
   decoded subjects reject a changed declaration while the old still-valid
   snapshot remains queryable.
9. **Missing REST configuration.** Switching the declaration to rest/1 without
   explicit endpoint credentials/configuration cannot displace the file store.
10. **Unchanged declarations.** Reinstalling the same container advances and
    persists the policy generation but retains the store/revision. Deleting
    its file before reinstall proves the snapshot is reused, not reread.
11. **Changed mapping.** New mappings reconstruct the store, make the actor
    revision stale and emit only the new mapped field. A held policy snapshot
    still carries its original generation, container and declaration.
12. **Changed TTL.** Reconstructed store output falls within exact before/after
    bounds for the new 120-second TTL; its revision is stale for the old actor.
13. **Changed identity declaration.** Adding a mapped identity declaration
    reconstructs the store and resolves the new canonical lookup identity key.
14. **Restart from persisted state.** A new PolicyMgr reads the accepted
    container/generation from FakeDb, reconstructs sources with new revisions,
    begins with unverified health and serves the new mapping.
15. **Jointly submitted valid/invalid updates.** Joining both update futures
    leaves only the accepted container persisted/live and advances the policy
    generation exactly once. This is a joint-submission test, not a forced
    mid-write race or database crash-window test.

Limits: a held PolicySnapshot coherently pins its own policy/container; these
checks do not prove atomicity between separate policy-manager and source-manager
loads by in-flight requests. File deletion remains visible on a later required
reload, not on an unchanged declaration's cached-store reuse. FakeDb readback
is not live Redis durability, transaction failure injection, filesystem atomic
replacement or relay-delivery acceptance. Removed-source cleanup and the
previously deferred envelope/subject-link work remain open.

Validation: **15 new policy/source lifecycle tests** pass. Full Rust workspace
validation passed **810 tests, 9 ignored**, and workspace formatting passed.
Cargo manifest and lockfile were restored unchanged after the temporary
worktree path adjustment. No Go code changed or Go suite was rerun; the
documented 13 unrelated dashboard failures remain unresolved.

## Fifteen-step maturation batch: identity selection and multi-identity lookup

All fifteen focused cases passed without production behavior changes. Seven
exercise the shared lookup-identity collector directly; eight exercise real file
stores using temporary data and explicit canonical mappings.

1. **Exact key selection.** Keys are case-sensitive and qualified names require
   exact policy selection; no source suffix is inferred or removed.
2. **Exact identity values.** Empty strings, surrounding spaces, case,
   composed/decomposed Unicode, escaped/control/non-BMP text and literal escape
   sequences remain unchanged.
3. **Non-scalar skip contract.** Zero/multiple-value identities are skipped even
   when expired; identical repeated values are not normalized into a scalar.
4. **Expired selected scalar.** An expired selected scalar makes the entire
   identity set indeterminate in both iteration orders, with key/source context.
5. **Unselected expiration isolation.** Expired claims outside the selected keys
   do not poison live identities; selecting no keys yields an empty set.
6. **Repeated policy keys.** Repeating a lookup key in the selector does not
   duplicate a selected attribute.
7. **Issuer-provenance limit.** Two scalar attributes with the same key/value but
   different issuer stamps still become identical plain lookup pairs; this
   evidence explicitly does not claim issuer-qualified subject linking.
8. **Identity-key isolation.** Equal subject strings under user/device keys
   match only the requested key; a differently cased key matches neither.
9. **Exact subject matching.** Case, spaces, empty subjects, Unicode normalization
   forms and a literal escape sequence identify separate file records.
10. **Disjoint union permutations.** All six orderings of three matched identities
    return the same complete scalar/multi-value/tag union.
11. **Conflict permutations.** All six orderings reject a canonical alias
    disagreement even when a third identity contributes a valid separate claim.
12. **Repeated identities/agreeing aliases.** Repeated lookup entries and matching
    aliases collapse to one canonical field without duplicate outputs.
13. **Unmatched semantic errors.** A syntactically valid but invalid-cardinality
    unmatched record does not poison a valid subject or an empty lookup.
14. **Matched malformed union.** Zero/multiple/identical-repeated scalar values on
    a matched identity reject the whole union in either order beside a valid peer.
15. **Conflict repair.** A conflicting lookup does not advance revision; replacing
    data and flushing advances it and recovers the complete repaired result with
    the same identity set.

Successful file results check the complete canonical-key/value map, no duplicate
canonical outputs, exact source stamp and unexpired attributes. Preserved limits:
blank subjects are not silently rejected or trimmed; issuer provenance remains
absent from lookup pairs; multi-value identities retain the intentional skip
contract. Semantic-cardinality isolation does not bypass strict whole-file JSON
decoding. No source-principal authorization, new uniqueness rule, shared schema,
dependency, Go behavior, deployment or relay-delivery acceptance is introduced.

Validation: **15 new identity lifecycle tests** pass. Full Rust workspace
validation passed **825 tests, 9 ignored**, with workspace formatting passing.
Cargo manifest and lockfile were restored unchanged after the temporary
worktree path adjustment. No Go code changed or Go suite was rerun; the documented
13 unrelated dashboard failures remain unresolved.

## Fifteen-step maturation batch: durable reconciliation lifecycle

Added fifteen regressions against the existing durable outbox and real
assembly/actor/visa reconciliation paths, using the test database:

1. **Pristine status.** An untouched source is idle at generation zero without
   an invented error.
2. **Source-name isolation.** Delimiters, percent escapes, casing, spaces and
   distinct Unicode forms retain separate request keys and counters.
3. **Repeated notifications.** Twenty enqueues advance requested generation
   without advancing completed generation.
4. **Failed acknowledgement.** Repeated failed passes retain the previously
   completed generation and an explicit retry error.
5. **Successful retry.** Success clears the stored error without another enqueue.
6. **Captured generation.** Finishing older captured work leaves a newer request
   pending.
7. **Malformed requested state.** Negative, nonnumeric, empty and overflowing
   generations return errors instead of silently defaulting to idle.
8. **Malformed completed state.** The same invalid forms cannot become successful
   completion evidence.
9. **Idle processing.** No requests means no source flush or lookup.
10. **Coalesced processing.** Five requests need one successful flush. With no
    live visas, no actor lookup is required; another idle pass does no work.
11. **Flush failure/recovery.** A failed flush does not start actor refresh or
    acknowledge work. Repair retries the same request and refreshes live actors.
12. **Lookup failure.** Failed live-actor refresh keeps work unacknowledged even
    when the visa sweep has queued revocation.
13. **Lookup recovery.** A repaired source completes the pending generation,
    clears its error and leaves both endpoints' source revisions current.
    The previously queued revoke remains queued and stored expiry is unchanged.
14. **Missing-source isolation.** Missing sources retain failed work while a
    separate healthy source can complete its request.
15. **Enqueue during flush.** A request injected inside the flush is not consumed
    by the earlier captured generation; a second successful pass is required.

No new production defect surfaced and no runtime behavior changed. This does
not establish Redis crash recovery, database write-failure handling, concurrent
worker safety, source-removal cleanup, policy-generation isolation, or relay
acknowledgement/delivery. The no-live-visa case is outbox completion evidence,
not evidence that disconnected actors were eagerly reconciled.

Validation: **15 new reconciliation lifecycle tests** pass; full Rust workspace
validation passed **840 tests, 9 ignored**. Workspace formatting passed.
Cargo manifest and lockfile were restored unchanged after the temporary
worktree path adjustment. No Go code changed or Go suite was rerun; the documented
13 unrelated dashboard failures remain unresolved.

## Fifteen-step maturation batch: actor persistence failure boundaries

Added fifteen tests through the real actor manager/repository with existing
test-database fault hooks; no new fault mechanism or runtime fix was needed:

1. **Rejected attribute write.** The exact serialized attribute hash remains
   unchanged and the captured source revision is not committed.
2. **Working versus stored actor.** Local refreshed claims do not imply database
   publication; rejected writes leave the persisted old observation intact.
3. **Reloaded retry.** Repair and reload fetch the source again, remove omitted
   old claims, persist new claims/authority and make the revision current.
4. **Repeated rejection.** Three failed writes each retry the provider without
   making the source current.
5. **Failed pruning write.** A failed lookup followed by a failed removal write
   does not commit even the failure sentinel. Restoring the previous source
   revision distinguishes the unchanged record from a sentinel commit.
6. **Pruning retry.** Once writes recover, failed-source removal persists,
   reports indeterminate and commits the retry sentinel, not a successful revision.
7. **Mixed outcomes.** A successful source beside a failed source cannot commit
   either revision when the combined actor write fails.
8. **Identity bookkeeping.** A rejected replacement preserves the old persisted
   identity key; the successful retry removes it with the old claim.
9. **Late delete rejection.** Failure after the attribute pipeline can leave new
   claims in storage, but returns a store error and leaves the revision stale.
10. **Lost delete reply.** Apply-then-error also cannot report refresh success or
    advance the recorded source revision.
11. **Late-failure recovery.** Even fresh claims already persisted by the partial
    update do not suppress the next lookup. Successful retry makes the following
    unchanged pass a no-op.
12. **Sweep accounting.** Failed writes and missing actors increment separate
    counts; repair refreshes the actor while the missing address stays unresolved.
13. **Malformed stored actor.** Invalid serialized attribute data fails the load
    without a provider lookup or source revision commit.
14. **Deleted actor.** Refreshing a held copy after deletion does not recreate the
    base record or mark the source current.
15. **Addressless actor.** No address means no lookup/write/revision commit, even
    when writes are configured to fail.

The attribute replacement pipeline and the complete actor update are distinct:
the latter also writes identity metadata and service registrations in subsequent
operations. These tests explicitly expose partial persistence on late failure;
they do not claim all actor records roll back together. The safety boundary
tested is an explicit error plus uncommitted revisions and a required retry,
not cross-record atomicity. The fixture substitutes the actor repository's test
database; it is not a live Redis, full outbox/write-failure, process-crash,
concurrent-reader, or relay-delivery acceptance test.

Validation: **15 new tests** pass alongside the prior 10 actor lifecycle tests.
Full Rust workspace validation passed **855 tests, 9 ignored**, and workspace
formatting passed. Cargo manifest/lockfile were restored unchanged after the
temporary worktree path adjustment. No Go code changed or Go suite was rerun;
the documented 13 unrelated dashboard failures remain unresolved.

## Fifteen-step maturation batch: disconnect revision and address lifecycle

Reproduced five failing regressions: rejected direct removal, rejected cascade
removal, failed cleanup for an already-absent actor, late cleanup delete
rejection and an applied delete whose reply reports failure. All returned the
address to its pool while retaining the old actor's source revision records.

Disconnect now gates managed address release on successful actor cleanup in
both the direct and cascaded-adapter paths. Successful cleanup still purges
revision records before release. Failures log that the allocation is retained;
other best-effort cleanup continues and the existing error-return contract is
unchanged.

Fifteen lifecycle regressions cover:

1. **Successful adapter removal.** The actor and source revision disappear
   before the address becomes reservable by another owner.
2. **Rejected direct removal.** The stored actor, revision and allocation remain
   owned together.
3. **Rejected cascade removal.** Both the node and attached adapter retain their
   allocations and revisions when actor cleanup fails.
4. **Repaired direct cleanup.** A later successful attempt clears revisions and
   releases the retained allocation.
5. **Absent actor cleanup.** Successful orphan cleanup clears revision records
   and releases the address.
6. **Absent actor cleanup failure.** An unconfirmed cleanup cannot recycle even
   an address whose actor record is already absent.
7. **Malformed actor load.** A load error propagates before revision purge or
   address release.
8. **Successful cascade.** Node and adapter actor records/revisions disappear
   and both addresses become reusable.
9. **Peer isolation.** Adapter removal leaves its node's actor, revision and
   allocation unchanged.
10. **Address reuse.** A new owner must fetch its own source observation, even
    when that observation is empty; the next unchanged refresh is a no-op.
11. **Repeated disconnect.** Cleanup remains safe when repeated without an
    intervening reassignment.
12. **Late deletion failure.** Even after partial actor deletion, rejected
    remaining cleanup retains allocation and revision records.
13. **Ambiguous deletion reply.** Apply-then-error cannot justify address reuse.
14. **Partial-cleanup repair.** A successful retry clears records and releases
    the address even when the actor is now absent.
15. **Unmanaged actor.** Cleanup purges its revision without disturbing a managed
    peer's allocation or revision.

Tests use real actor/connection/network managers with test databases. Actor and
node repositories use separate test databases so rejected actor operations do
not prevent the cascade test from reaching its address-release boundary.
Allocation assertions attempt to reserve the exact address, rather than relying
on random allocation collisions.

This is not automatic retry scheduling, durable quarantine across restart,
live Redis recovery, concurrent disconnect/reconnect isolation, or protection
against delayed work that repopulates revision records after disconnect.
If a failed node cleanup has already removed its connection list, retained
adapter allocations may require individual cleanup attempts; no durable orphan
cleanup queue is introduced here.

Validation: the initial new suite had **10 passing and 5 failing tests** before
the fix. Afterwards all **15 new tests** and all **43 connection-control tests**
passed. Full Rust workspace validation passed **870 tests, 9 ignored**.
Workspace formatting passed; Cargo manifest/lockfile were restored unchanged
after the temporary worktree path adjustment. No Go code changed or Go suite
was rerun; the documented 13 unrelated dashboard failures remain unresolved.

## Fifteen-step maturation batch: file snapshot reload lifecycle

Added fifteen tests around the existing file store's snapshot and refresh lock:

1. **Cached data isolation.** Backing-file changes do not silently alter a live
   snapshot or its revision.
2. **Cached availability.** Deleting the file does not invalidate a snapshot
   outside its refresh window or invent a revision change.
3. **Early reload.** A snapshot with 59 seconds remaining refreshes before its
   old deadline, returning the new observation.
4. **Expired reload.** Expired cached data is replaced with fresh, unexpired
   claims and a new revision.
5. **Early reload failure.** Invalid replacement JSON rejects even still-live
   data within the refresh window; repeated failures retain the exact snapshot,
   and repair recovers on the next lookup.
6. **Queued automatic readers.** Sixteen readers blocked behind the refresh
   lock share the exact same newly published snapshot after one reload.
7. **Queued explicit flushes.** Each explicit flush publishes a distinct revision,
   rather than being coalesced like automatic readers.
8. **Reader after successful flush.** A queued reader rechecks current state and
   uses the newly published snapshot even if the file has since disappeared.
9. **Reader after failed flush.** A failed flush preserves the old snapshot;
   repairing the file before the queued reader runs lets that reader recover.
10. **Held snapshot immutability.** Replacement does not mutate a previously
    held snapshot's data.
11. **Cache deadline stability.** Repeated cache hits leave the exact monotonic
    snapshot deadline unchanged and emit attributes near that same deadline.
12. **Reload deadline bounds.** The new monotonic expiration is bracketed by
    before/after reload timestamps plus configured TTL; emitted wall-clock
    expiration is checked separately.
13. **Semantic versus decoding failure.** Syntactically valid invalid-cardinality
    data publishes a snapshot/revision but fails lookup. A repaired backing file
    is not read on a cache hit; explicit flush restores valid observations.
14. **Empty replacement.** Reloading an empty document removes the old subject
    instead of serving the prior observation.
15. **Atomic file replacement.** A renamed complete replacement is loaded on
    flush; a live cache hit before flush retains the old complete observation.

No new production defect surfaced and no runtime behavior changed. Tests inject
snapshot deadlines directly rather than sleeping for TTL expiry. Queued-future
tests explicitly poll while the refresh lock is held, then release it; they do
not rely on scheduler delays or prove every multi-threaded interleaving.
Wall-clock/monotonic conversion checks allow 10 milliseconds of sampling skew;
the monotonic snapshot deadline assertions remain exact.
This does not validate external writers' crash safety, filesystem durability,
symlink policy, arbitrary concurrent file writes or in-flight actor/policy
generation isolation.

Validation: all **15 new tests** and all **24 file-store tests** pass.
Full Rust workspace validation passed **885 tests, 9 ignored**, and workspace
formatting passed. Cargo manifest/lockfile were restored unchanged after the
temporary worktree path adjustment. No Go code changed or Go suite was rerun;
the documented 13 unrelated dashboard failures remain unresolved.

## Development increment: removed-source cached-claim cleanup

Reproduced seven regressions before implementation: unexpired removed-source
claims remained on actors; removal after revision-memory loss did nothing;
retired identities were sent to remaining sources; an expired retired identity
prevented otherwise valid refresh; peer isolation had no removal to observe;
removal never reached the persistence failure boundary; and an expired
authenticated identity prevented even retired-claim cleanup.

Actor refresh now determines removed sources from recorded revisions plus
persisted source-qualified ownership evidence, excluding the active source list.
Before selecting lookup identities, it prunes the removed sources' qualified
keys and same-source legacy aliases. Actor removal APIs also remove their
identity bookkeeping. This includes derived authority and freshness metadata
when those qualified claims are present. Persisted evidence works with a newly
constructed manager that has no actor-revision history.

Ownership requires a nonempty canonical prefix and an exact match between the
qualified suffix and attribute source stamp. An issuer name alone is not
sufficient: it can overlap a built-in issuer, and unrelated authenticated
claims must survive. Legacy-only claims without qualified ownership evidence
are not retroactively classified or migrated.

The refresh outcome carries retired revision records to forget only after
successful actor persistence. A failed write retains revision history and the
stored old claims; reloading and retrying removes them. Empty observations can
have revision bookkeeping cleared without inventing an actor mutation.
Re-adding a removed source at the same numeric revision still requires lookup
after its previous removal has completed.

Fifteen new regressions cover:

1. Unexpired claim, authority, freshness and identity-key removal.
2. Removal with a new manager and no revision memory.
3. Retired identities excluded from remaining providers' requests.
4. Expired retired identities removed before expiry validation.
5. Active-source values/deadlines and authenticated claims preserved.
6. Rejected removal writes retain stored claims; repair reloads and retries.
7. Same-revision re-addition forces a new observation.
8. An expired authenticated identity still reports indeterminate, while retired
   claims are pruned from the working actor for persistence.
9. Removed-source ownership evidence does not erase unrelated built-in claims
   sharing the issuer name.
10. Mismatched key suffix/source stamps do not establish ownership.
11. Rejected removal does not prematurely clear the recorded source revision.
12. Empty-source removal clears bookkeeping without an unnecessary actor write.
13. Completed removal leaves the next refresh a no-op.
14. Loaded actor cleanup is persisted with a restarted manager.
15. A real issued visa requiring a qualified source claim is allowed initially;
    removing the source persists cleanup at both endpoints before the existing
    visa recheck queues revocation.

Explicit source removal is known absence, unlike an outage of an active source.
Policy conditions on missing claims therefore retain their normal semantics;
this does not force blanket denial merely because a source was removed.
The existing request and reconciliation paths both use this refresh helper.
Actors without live visas remain lazy-refresh, and relay delivery is not proved.

Still open: atomic policy/source-generation isolation for in-flight requests,
concurrent same-actor refresh/disconnect races, legacy-only provenance recovery,
live database crash durability, and cross-record actor-update atomicity.
No principal-envelope authorization, issuer-qualified subject links, shared
schema change, dependency, Go change or deployment is included.

Validation: **15 new regressions** pass; all **57 actor-refresh tests** pass.
Full Rust workspace validation passed **900 tests, 9 ignored**, with workspace
formatting passing. Cargo manifest/lockfile were restored unchanged after the
temporary worktree path adjustment. No Go suite was rerun; the documented
13 unrelated dashboard failures remain unresolved.

## Development increment: reject retired in-flight refresh results

Added six deterministic regressions; five failed before implementation and the
unchanged-configuration control passed. A blocked lookup previously continued
through a source replacement/removal, persisted retired claims, and committed
its captured revision. A normal visa request crossing source replacement could
also return allow rather than require a fresh attempt.

The trusted-source manager now exposes an opaque, owned publication token.
It pins the service-list snapshot and compares by Arc identity, detecting
removal/re-addition even when the very same store is republished. This is not a
source flush revision token: existing snapshot-revision refresh behavior remains
separate.

Before refreshing an actor, the application captures source publication and
policy generation. Observed changes after lookup return indeterminate before
the database write or revision commit. After persistence it checks again: an
observed change clears the actor's revision records so subsequent refresh must
fetch again, and returns indeterminate (or preserves the underlying store error
if persistence failed). The working actor must be discarded on failure; a
completed or partially completed write is not rolled back.

Normal visa requests retain the policy captured before refreshing their
endpoints, instead of selecting a potentially different policy afterwards.
They check policy generation/source publication after endpoint refresh and
after policy evaluation, log observed changes and deny for caller retry.

Regressions verify:

1. Replacement during lookup leaves persisted old claims and recorded revision
   unchanged; retry uses the replacement source's new claims.
2. Removal during lookup cannot restore retired claims; reloaded retry performs
   removed-source cleanup.
3. Policy update during lookup rejects the old refresh before persistence.
4. An unchanged configuration permits the blocked refresh to persist and commit.
5. Removal/republication of the identical store is still detected.
6. A request crossing source replacement denies without creating a visa;
   a fresh attempt with the current source succeeds.

Tests explicitly poll futures until lookup blocks, then publish changes and
release lookup; they do not rely on sleeps. They establish rejection of changes
across the lookup await, not every concurrent persistence/issuance interleaving.
There is no dedicated write-blocking fault injection for the post-persistence
invalidation branch in this increment; the full suite is regression evidence,
not proof that this branch was exercised.

Remaining boundaries: policy and source publication are still separate atomic
stores; these optimistic checks neither pin all refresh queries to one service
snapshot nor lock publication against actor writes or visa issuance. A change
after a final check, concurrent same-actor refresh, and changes during final
visa creation still require stronger coordination. Admission/bootstrap and
existing-visa sweep publication semantics are not redesigned here. Do not
interpret this increment as complete policy/source-generation isolation.

Validation: **6 new regressions** pass, alongside all **63 actor-refresh tests**.
Full Rust workspace validation passed **906 tests, 9 ignored**, and workspace
formatting passed. Cargo manifest/lockfile were restored unchanged after the
temporary worktree path adjustment. No Go code changed or Go suite was rerun;
the documented 13 unrelated dashboard failures remain unresolved.

## Development increment: configuration checks after visa storage

Reproduced two issuance regressions by holding the real visa repository's
backing-write lock: after the request reached storage, replacing source
configuration or updating policy still let it return the old allow decision.
Unchanged configuration and rejected-store controls passed before the fix.

Normal issuance now captures live policy generation and source publication
before creation and checks both after creation returns. On an observed change,
it queues the stored visa for revocation at every path node, then returns deny
without starting the request's background distribution task. A failed revocation
write is logged and propagated rather than returning a success-shaped denial.
The original store error remains unchanged if visa creation itself fails.

Five regressions cover:

1. Source configuration changes while visa storage is blocked: deny plus
   persisted `PendingRevoke` at both nodes, with no pending installs.
2. Policy update while storage is blocked: no old-generation allow; the stored
   visa retains its original created/checked generation and is queued to revoke.
3. Unchanged configuration: the blocked store completes with the existing
   allow/pending-install behavior.
4. Rejected visa storage: return the store error, without a memory visa or
   invented revocation evidence.
5. Rejected revocation write: explicit store error, not allow or successful deny.
   The pending-install record remains; a later successful revocation write
   repairs it.

Successful revocation checks inspect both in-memory per-node queues and the
serialized backing record, then reconstruct a new repository over the same test
database to verify pending revocation survives reconstruction.
Tests verify the visa ID counter advanced while the memory image is still empty
before changing configuration, proving the request reached the storage boundary.
They disable Tokio cooperative budgeting for the explicitly polled request so a
cooperative yield cannot masquerade as the intended lock wait. The test-only
repository lock accessor changes no production locking behavior.

Limits: this is not a transaction between publication, storage and distribution.
The visa is initially visible as pending install; concurrent housekeeping may
observe it before the corrective revoke, and failed revoke persistence can
leave it there until reconciliation succeeds. No durable new retry mechanism,
rollback, live Redis recovery proof or relay delivery acknowledgement is added.
Changes after the last guard, bootstrap/admission consistency and concurrent
same-actor refresh still require separate coordination.

Validation: both reproduced race regressions pass after the fix. All **5 new
tests** pass; the focused request-worker run passed **38 tests** before adding
the final revoke-write failure case. Full Rust workspace validation, including
that final case, passed **911 tests, 9 ignored**. Workspace formatting passed.
Cargo manifest/lockfile were restored unchanged after the temporary worktree
path adjustment. No Go code changed or Go suite was rerun; the documented
13 unrelated dashboard failures remain unresolved.

## Development increment: non-installable staged visa issuance

Normal request issuance now persists a `Staged` node state for the requesting
node and every other node on the path. Installation queues only select
`PendingInstall`, so initial storage no longer publishes unchecked installation
work. After the post-storage configuration guard, a new repository operation
activates all nodes in one write-through update. The operation requires a live
visa with every node still staged; missing, expired, removed, revoked and
already-activated records cannot activate.

The activation operation holds the existing backing-write lock, writes the
whole persisted node-state map first, and then updates the memory image.
It preserves metadata and visa expiration. Failed activation writes return
errors. Policy allow/deny verdict recorders skip staged records, so policy
sweeps cannot bypass activation or convert an abandoned staged record through
a deny/allow cycle into installation work.

The existing post-storage invalidation still attempts revocation on a detected
configuration change. Its failure now leaves a staged, non-installable record,
not the pending-install record documented in the preceding increment. That
preceding failure test was updated to assert this intentional behavior change
in both serialized storage and a reconstructed repository.

Configuration is checked again after activation. A detected change queues
revocation and denies before starting this request's distribution task.
Existing direct creation used by fixtures retains immediate-install behavior
as a test-only helper; bootstrap creation retains its existing production
behavior. Staging and activation are used by normal request issuance.

Ten additional regressions cover:

1. Canceled issuance remains staged at all path nodes across reconstruction.
2. Activation publishes the full path once, with unchanged expiry/generation.
3. Policy verdicts cannot promote an unactivated staged record.
4. Revoked staged records cannot activate; unstamped revoke is not undone by
   a later policy allow.
5. Rejected activation writes leave staging intact and can be retried.
6. Missing or removed records cannot activate.
7. A request with a rejected activation write returns an explicit error,
   never allow, and remains staged.
8. Expired staged records cannot activate (paused Tokio clock test).
9. An apply-then-error activation reports error while memory stays staged,
   but a reconstructed repository sees the durably applied pending install;
   explicit revocation repairs both views.
10. A configuration change while activation is queued is detected afterwards,
    denying the request and persisting revocation for every node.

These extend the existing five configuration/storage tests: all **15 issuance
configuration tests** now pass. Abandoned staged records are bounded by the
visa TTL and are not automatically activated on restart.

Limits: staging closes the initial-storage install window, not the activation
write's own uncertainty or concurrent policy-publication window. A lost
activation reply can leave a durable pending install despite an error. A
configuration change during activation can allow housekeeping to see pending
installation before corrective revocation; failed corrective writes still
require reconciliation. No durable cancellation/activation retry queue or
transaction with configuration publication is introduced.

`Staged` extends the local persisted state enum without changing shared API
schemas. Older binaries cannot decode records with this state; mixed-version
database access and downgrade are unsupported. This early-release change does
not add a migration. Live Redis crash durability, relay acknowledgements and
concurrent same-actor refresh isolation remain unverified.

Validation: full Rust workspace **921 passed, 9 ignored**. Workspace formatting
and the non-test `vs` build check pass. The build check retains one pre-existing
unused `DnsPublisher::add_provider` warning; no new unused-method warning is
left by the test-only creation helper. Cargo manifest/lockfile were restored
unchanged after the temporary worktree path adjustment. No Go code changed or
Go suite was rerun; the documented 13 unrelated dashboard failures remain.

## Development increment: uncertain activation cleanup

Activation errors now attempt a compensating deletion while holding the same
backing-write lock as activation. A lost SET reply no longer leaves a durable
pending install when that deletion succeeds. Confirmed deletion removes the
memory entry and both secondary indexes. The original activation error still
propagates; cleanup never turns a failed request into allow or successful deny.
A cleaned-up visa ID cannot be activated or revived by a later policy verdict;
the caller must submit a fresh request.

If deletion also fails, both failures are logged with an explicit durable-state
uncertainty warning. Memory remains staged for explicit repair. An applied
activation plus rejected deletion still reappears as pending install after
restart. An applied deletion with a lost reply leaves staged memory but no
durable record; an explicit successful removal converges the two. This follows
the repository's DB-first/memory-second discipline rather than pretending an
unacknowledged deletion succeeded.

Four new tests cover rejected activation/rejected cleanup with retry, ambiguous
activation/rejected cleanup with revocation repair, ambiguous activation and
ambiguous cleanup with removal repair, and the real request's lost activation
reply with successful cleanup across restart. Existing increment-owned tests
were deliberately updated: rejected activation no longer remains retryable
after confirmed cleanup, and ambiguous activation no longer remains durable
when cleanup succeeds. Five assertions failed before the runtime fix; all
**19 issuance configuration tests** now pass, including durable reconstruction,
whole-path queues and node/actor index removal.

Validation: full Rust workspace **925 passed, 9 ignored**; formatting and the
non-test `vs` build check pass. The production check retains the one pre-existing
unused `DnsPublisher::add_provider` warning. No dependency or shared schema
changes, Go changes or deployment are included.

This supersedes the preceding increment's unconditional restart-uncertainty
description, not its limits on atomic publication. Failed compensation,
cancellation/crash during activation or cleanup, concurrent configuration
publication, live Redis durability and automated durable repair remain outside
the guarantee. No retry queue or transaction with configuration is introduced.

## Development increment: actor write/publication boundary coverage

The existing post-persistence configuration guard now has deterministic
write-boundary coverage. A test-only FakeDb pipeline gate pauses the actor
attribute replacement after lookup and repository validation but before the
atomic mutation. Reads remain available while paused, so tests can verify the
old persisted claims, publish configuration, and release the actual write.
Explicitly polled futures use Tokio unconstrained mode to avoid confusing a
cooperative-budget yield with the intended persistence boundary. No sleeps or
production database locking changes are introduced.

Six tests cover:

1. Source replacement with the previously recorded revision value. The old
   write may persist but returns indeterminate; clearing the old revision forces
   lookup from the replacement, whose authoritative claims replace it on retry.
2. Policy publication invalidates every revision for the affected actor while
   preserving a peer actor's revision. A formerly current second source is also
   stale and consulted on reload.
3. Source removal during writing rejects the completed refresh; a storage reload
   and retry prune the retired claims without consulting the removed provider.
4. Removal/republication of the identical store during writing requires another
   lookup despite unchanged revision.
5. Concurrent write rejection and source replacement preserve the Store error,
   leave old persisted claims untouched and invalidate prior revision memory.
   A repaired, reloaded retry succeeds.
6. An unchanged-publication control commits the refreshed revision once; the next
   reload/refresh is a no-op and performs no additional lookup.

All six pass against the existing runtime implementation: this increment adds
coverage, not a production behavior fix. Full Rust workspace validation:
**931 passed, 9 ignored**. All **52 actor lifecycle tests** pass. Workspace
formatting and the non-test `vs` build check pass, retaining the one pre-existing
unused `DnsPublisher::add_provider` warning. No dependencies, shared schemas,
Go code or deployment changed.

The evidence does not establish atomic publication/write isolation: completed
or partial writes are not rolled back, the caller must discard its working
actor, and cancellation before the post-write check, concurrent same-actor
updates, live Redis failures and restart-persistent revision fencing remain
outside this coverage.

## Development increment: canceled actor persistence

A refresh dropped while awaiting actor persistence now synchronously clears
that actor's cached source revisions and logs that a storage reload is required.
A scoped drop guard surrounds the write await only. Normal completion disarms
it before the existing post-write configuration/error checks; cancellation in
lookup never arms it. Other actors' revision records are untouched. Even with
unchanged configuration, cancellation invalidates every source for the affected
actor because the complete repository update may not have finished.

Cancellation testing also reproduced an unreadable persisted actor: attribute
replacement applied, but identity metadata still referenced a removed claim
before its separate write ran. ActorRepo now writes identity-key metadata in
the same existing atomic pipeline as attribute replacement. No schema change
or dependency is needed. Timestamps and service registrations remain separate;
this is not a transaction for the entire actor update.

Five new tests cover:

1. Cancellation before mutation, source replacement reusing revision 1,
   peer-cache isolation, and authoritative retry.
2. Cancellation after mutation but before its reply, readable actor/identity
   state, reused revision invalidation, and replacement lookup on reload.
3. Cancellation without publication invalidates a previously current second
   source as well, forcing both providers to be consulted on retry.
4. Cancellation after mutation and source removal permits a readable reload,
   followed by retired-source pruning without another retired-provider lookup.
5. Cancellation during lookup preserves the earlier revision and actor image.

The test-only FakeDb reply gate pauses after applying a complete pipeline and
releasing its data lock, so readback verifies the applied state before dropping
the refresh. Tests before the fix reproduced stale revision certification and
unreadable identity metadata. A prior increment-owned request-race test was
also made deterministic using Tokio unconstrained polling plus an exact source
call assertion: the extra test gate otherwise exposed a cooperative-budget
yield before its intended lookup boundary.

Validation: full Rust workspace **936 passed, 9 ignored**, including all
**74 actor-attribute tests**. Formatting and the non-test `vs` build check pass;
the existing unused `DnsPublisher::add_provider` warning remains. No shared
schema, dependency, Go or deployment changes.

Limits: the guard is in-process cancellation invalidation, not a durable fence.
An already-submitted Redis operation may finish after cancellation; concurrent
retry/actor writes and recycled-address races are not serialized. Service-index
partial updates, atomic multi-read actor snapshots, process-crash handling and
live Redis cancellation semantics remain outside this guarantee. Reload and
retry are required; no rollback or automatic repair queue is claimed.

## Development increment: real Redis atomic pipeline correction

Tracing the remaining service-registration cancellation window uncovered a
backend mismatch: RedisDb's `atomic_pipeline` used a normal redis-rs pipeline
without calling `atomic()`. FakeDb serialized the entire operation, but the
production backend merely batched individual commands. The preceding
attribute/identity change alone therefore did not provide its claimed
non-interleaving guarantee on Redis until this correction.

RedisDb now enables MULTI/EXEC for every nonempty atomic pipeline. Empty batches
return success without issuing an empty command (previously a Redis client
error). The existing DbConnection contract documents transaction isolation,
queue-time aborts, runtime partial failures and ambiguous transport replies.
No operation variants, schema changes or new dependencies are needed. All
workspace DbOp producers and backend match arms were inspected; the change
restores the semantics requested by existing actor, node, visa, link and policy
repository callers rather than creating a new transaction API.

Three opt-in tests use the existing private Redis child harness:

1. A queue-time HSET arity error must abort the entire batch: a preceding DEL
   must not remove the original key and a subsequent SET must not create a key.
   Before the fix the original key was deleted despite the returned error.
2. Successful DEL, binary SET, SREM, HSET, multi-field HSET and EXPIRE are visible
   through an independent connection with an exact binary payload and a bounded
   positive TTL. An empty batch succeeds; it failed before the fix.
3. An execution-time WRONGTYPE error returns an error but both surrounding SETs
   apply. This deliberately proves Redis transactions do not provide rollback.

Validation: all **9 private Redis transaction/policy-operation tests** pass,
including the three additions. They were explicitly run with `--ignored`, not
merely compiled. Each harness verifies its own server identity/readiness via a
private Unix socket, disables persistence, and reaps the exact child/removes
its temporary directory. No existing Redis service or user database is used.
Full Rust workspace: **936 passed, 12 ignored** (the three new Redis tests are
opt-in). Formatting and production `vs` build checks pass; the existing unused
`DnsPublisher::add_provider` warning remains.

Limits: MULTI/EXEC prevents interleaving, not execution-error rollback,
ambiguous-commit recovery, durable crash guarantees, or consistent snapshots
across multiple separate reads. The remaining service-registration work was
not folded into this correction: timestamps/indexes still update separately
from actor attributes/identity metadata. Concurrent actor/recycled-address
fencing and service-index cancellation repair remain follow-ups. No Go code,
shared schemas, dependencies or deployment changed.

## Development increment: actor service-index transaction

ActorRepo updates now build one MULTI/EXEC pipeline for attributes, identity
metadata, the actor's service-name set, and service-provider memberships.
Previously the first batch could apply and the caller could be canceled before
service cleanup/addition, leaving fresh service claims paired with old indexes.
Two deterministic regression cases reproduced that mismatch before the fix.

The existing service names are read before writing. The pipeline removes only
this actor from those provider sets, replaces this actor's service-name set,
and adds the new memberships through a new DbOp::SAdd variant. Redis and FakeDb
implement the same variant; FakeDb reuses one set-add helper for standalone and
pipeline paths without reacquiring its data lock. Attribute serialization now
propagates errors while constructing the batch rather than writing empty JSON.
Timestamps remain separate writes, preserving create-time-only-if-absent
behavior without introducing another operation variant.

Four added actor repository tests cover cancellation before mutation, after
replacement, after removal of all services, and a rejected batch followed by
retry. Readback checks actor service claims, the per-actor service set and
provider lookups, with colon/percent service names and shared providers.
Post-application cancellation leaves the full indexed state consistent, and
repeating the update is idempotent. Pre-application cancellation/rejection leaves
the old state intact.

One added private Redis test exercises the real ActorRepo with shared providers,
replacement, independent-connection readback and complete service removal.
The supported-operation transaction test also now checks SADD. All **10 private
Redis tests** pass when explicitly invoked with `--ignored`; each uses and
reaps its own private server.

Three earlier increment-owned refresh tests were updated intentionally:
standalone DEL faults no longer interrupt this update because the standalone
service-set deletion was removed. Those two cases now verify successful batch
completion/revision commit despite the irrelevant fault hook. The former
late-delete recovery case now cancels after an applied pipeline reply and
verifies a fresh lookup on reload, retaining meaningful retry coverage.

Validation: **24 actor repository tests**, **74 actor-attribute tests**, full
Rust workspace **940 passed, 13 ignored**, workspace formatting and production
`vs` build check pass. The existing unused `DnsPublisher::add_provider` warning
remains. No dependency, shared schema, Go or deployment changes.

This supersedes the preceding service-index cancellation limitation for
well-typed, non-concurrent updates. It does not fence concurrent same-actor
updates or disconnect/address reuse: another writer can change the index after
the pre-transaction read. Redis execution errors can still partially apply
transactions, transport replies can be ambiguous, timestamps can fail after
the batch, and separate reads are not a snapshot. No rollback, durable retry
queue or full actor transaction is claimed.

## Development increment: service-set compare-and-write

Competing actor updates could both read the same old service set, then commit
different attribute/index batches. The later update removed only the original
memberships, leaving a provider registration introduced by the earlier update
even though the final actor no longer offered it. Two deterministic tests
reproduced this across separate repositories and with a final removal request.

Actor updates now pass their previously read service-name set to a conditional
database batch. Redis runs an exact set comparison and the mutations in one
Lua invocation; FakeDb compares and mutates under its existing data write lock.
A changed set returns false before any mutation. ActorRepo logs the conflict
and returns StoreError::Conflict with an explicit reload requirement; it does
not automatically retry an actor snapshot that may be stale. Repeating the
update after reloading/recomputing the desired actor state is the caller's
responsibility.

The conditional API supports existing DbOp variants with binary-safe length-
framed Redis arguments, without interpolating actor data into Lua. Ordinary
MULTI/EXEC pipelines and conditional batches share command argument generation;
FakeDb shares its operation executor. No new dependencies, stored fields,
schemas or per-process locking scheme are needed.

Eight added regressions cover:

- Three actor repository cases: competing replacements across repository
  instances, replacement versus removal, and a compatible same-service-set
  control. Conflicts preserve the winner's claims/indexes; subsequent retries
  remove its memberships without disturbing shared providers.
- Two FakeDb cases: exact same-cardinality mismatch/missing sets/wrong types
  with binary writes, and execution-error partial application.
- Three private Redis cases: exact mismatch/binary supported commands,
  one winner across independent connections plus a fresh-expectation retry,
  and execution-error partial application.

The conditional Lua path stops at its first execution error, leaving preceding
commands applied. This differs from ordinary MULTI/EXEC, where subsequent
commands also run. Both contracts are explicitly tested and neither claims
rollback. Conditional comparison failure itself performs no writes.

Validation: full Rust workspace **945 passed, 16 ignored**. All **13 private
Redis tests** were explicitly executed with `--ignored` and pass, including
the real ActorRepo service replacement/removal readback test. Formatting and
production `vs` build checks pass with the existing unused
`DnsPublisher::add_provider` warning. No Go, shared-schema, dependency or
deployment changes.

Limits: this is a service-set guard, not an actor generation fence. Attribute-
only updates with equal service sets can still overwrite each other, and a
service set that changes away and back is not distinguished. Disconnect,
address recycling, identity/principal changes, cancellation of submitted work,
ambiguous replies, timestamps and multi-read snapshot consistency remain
outside this guard. The existing refresh cancellation/revision invalidation
continues to apply; no automatic durable retry/repair queue is introduced.

## Development increment: consistent actor read snapshots

Full actor loading previously performed separate existence, attribute-hash and
identity-metadata reads. Atomic writes did not make those reads a snapshot:
an intervening update could pair old attributes with identity keys for new
claims, causing an AttributeError. Removal after attribute capture could turn
the subsequent metadata read into empty JSON and a serialization error.
Both races were reproduced deterministically before the runtime change.

DbConnection now exposes a two-hash snapshot read. Redis uses a read-only
MULTI/EXEC pipeline; FakeDb captures both maps under its data write lock, then
releases that lock before a test-only reply gate. ActorRepo reconstructs its
actor from these two maps and no longer performs separate existence/identity
reads. An empty metadata hash means not-found (Redis cannot retain an empty
hash); a present hash without identity_keys returns explicit InvalidData
instead of parsing a default empty string. Wrong types and malformed JSON
continue to propagate errors, not empty or successful actors.

Six added tests cover:

1. An actor read paused after capture survives an intervening update adding a
   new identity claim: the returned actor is wholly old, and a later load is
   wholly new.
2. Removal after capture returns the captured complete actor; a subsequent
   read correctly returns not-found.
3. Missing identity metadata returns a specific InvalidData error.
4. Wrong attribute-hash type and malformed attribute JSON retain explicit
   Redis/serialization error categories.
5. Real Redis missing/one-sided hashes and wrong types in either position.
6. Real Redis competing atomic writers/readers on independent connections:
   200 updates and 200 snapshot reads never mix hash generations, and final
   readback verifies generation 200 in both hashes.

Validation: full Rust workspace **949 passed, 18 ignored**. All **15 private
Redis tests** were explicitly executed with `--ignored` and pass. Workspace
formatting and production `vs` build checks pass, retaining the existing
unused `DnsPublisher::add_provider` warning. No dependency, shared schema,
Go or deployment changes.

Limits: the snapshot establishes consistency at capture, not current actor
liveness at authorization or write time. Separate CN/service-index queries,
initial actor insertion and partial execution-error states are not made
transactional by this read change. Concurrent actor-generation/address-reuse
fencing and durable repair remain follow-ups; a captured actor may legitimately
be stale immediately after capture.

## Development increment: CN lookup snapshots

The targeted CN lookup still checked actor existence and fetched the CN in
separate operations after the full actor-load snapshot fix. It now reuses
hash_pair_snapshot for the metadata and attribute hashes, so actor presence
and the returned CN refer to the same capture. Only the CN is decoded:
malformed unrelated attributes or identity-key JSON do not break this targeted
lookup. Existing not-found/missing-CN and malformed-CN error categories and
the non-scalar formatting fallback are preserved.

Five repository tests cover captured reads across removal and address
replacement, targeted decoding with unrelated malformed data, orphaned
attributes without metadata, and missing/malformed CN behavior. The two
interleaving tests explicitly enforce the snapshot capture/reply boundary;
before the change the old targeted path completed without entering that
boundary. They establish the new snapshot semantics rather than claiming a
reproduced old-path mixed-value read. A sixth, private Redis test confirms
targeted decoding and orphan rejection through an independent connection.

Validation: full Rust workspace **954 passed, 19 ignored**. All **16 private
Redis tests** were explicitly run with `--ignored` and pass. Workspace
formatting and production `vs` build checks pass with the existing unused
`DnsPublisher::add_provider` warning. No dependencies, schemas, Go code or
deployment changed.

The implementation intentionally reuses the existing snapshot API rather than
adding another backend-specific query. It transfers both hashes but decodes
only CN; this increment does not establish a large-claim performance bound.
Captured CNs can become stale immediately afterwards. Service queries,
concurrent actor-generation/address-reuse fencing, partial insertion and
durable repair remain outside this change.

## Explicit subject links: required follow-up

### Approved initial direction and transactional source registry (2026-10-25)

The operator explicitly approved the following initial implementation direction:

- Control-Service owns versioned authority/source registrations and links.
- Initial roots are devices established by verified authentication; no inferred
  human identity, field-match association or transitive link.
- Initial provider targets use exact immutable LDAP `entryUUID` selectors.
- Proposers and independent approvers require explicit organization-scoped
  permissions. The verified issuer/subject pair identifies the reviewer.
- Strict mode has no implicit legacy fallback.
- Snapshot delivery is to use independently provisioned pinned signing trust,
  mutual TLS, organization/VS-recipient binding and durable anti-rollback.
  A new shared cryptographic contract still requires separate approval.

This approves the initial direction, not every field or API in the historical
proposals below. Snapshot lease, clock skew, gateway revocation SLA and
production rollout remain **unset**. Strict runtime enforcement is not enabled;
neither S05 nor M1 is complete.

#### Implemented: authoritative registry-bound review persistence

The protected `changereview.Store` now provides an internal, Control-Service-owned
source registration alongside its existing BAS authority registration:

- `PutSubjectLinkSource` creates at decimal-string version `"1"` and requires
  exact version compare-and-swap for edits. Changes to provider identity,
  selector, permitted enrichment keys or enabled state advance the version.
  An unchanged key set, including reordered input, is a no-op. Disable/re-enable
  is versioned; deletion and identity renaming are forbidden.
- Configurations contain a bounded provider configuration reference, selector
  kind `ldap_entry_uuid`, 1-128 distinct permitted enrichment keys and an explicit
  enabled flag. Keys are stored in sorted order. No URI, bind credential, token,
  signing key or directory record is stored in this registration.
- Version change and immutable principal/reason/time audit evidence commit in
  the same SQLite transaction. `GetSubjectLinkSourceEvent` reads historical
  evidence; historical events are not proof that a source remains enabled.
- `CaptureSubjectLinkRegistry` reads an enabled, exact-version authority/source
  pair coherently and returns its SHA-256 review-context digest. This is a digest
  of the referenced registration pair, **not** a global registry sequence,
  authenticated snapshot, lease, trust assertion or proof of directory presence.
- `SubmitSubjectLinkWithRegistryOnce` and
  `ApproveSubjectLinkWithRegistryOnce` validate that pair, captured digest and
  permitted enrichment scope inside the review transaction. Both still require
  the exact-subject provider validator; an absent validator is rejected.
  New review approval rechecks the stored proposal, independent reviewer,
  authority/source versions and provider subject. Source edits through this
  single Store cannot interleave with the validation/commit boundary.
- Exact retry receipts are looked up before current registry/provider validation
  and replay original historical evidence, not renewed approval. Registry-bound
  receipts have separate fingerprints from older authority-only and externally
  source-checked methods; neither can impersonate the other.

These are the production persistence primitives used by the opt-in review-only
service composition below. No operator permission grant, registration file
loader, active-link publication, source transport change or VS binding
consumption is automatically enabled.
Provider references now have the internal protected resolver described below;
future Control-Service composition must explicitly select its configuration and
preserve version-bound transport intent. Registry-only store tests use a fixture
validator; the additional live TLS directory tests below exercise the real
resolver without certifying a deployed LDAP directory.
The registration does not automatically synchronize or replace the existing
assertion-source file registry or the policy-owned runtime-source toggle.
No registry-file/SQLite atomicity or HA guarantee is claimed.

#### Implemented: protected exact-subject provider resolution

`loadSubjectLinkProviders` is a command-local loader selected by opt-in split
Control-Service startup. It requires an explicit absolute path to a
private protected JSON file, at most 64 KiB, with exactly `version: 1` and
`providers` (1-64 mappings). Each mapping requires all nine canonically named
fields: `organization_id`, `source_id`, `source_version`, `provider_id`, `url`,
`ca_file`, `base_dn`, `bind_dn`, and `bind_password_file`. Unknown/duplicate
fields, trailing data, null fields, noncanonical versions, duplicate
organization/source/version mappings and plaintext LDAP are rejected.

The resolver matches the exact enabled registry capture and provider reference;
it does not discover identities, create registrations or reuse Simulator
configuration. CA and password references must be absolute protected files.
The explicit CA pool replaces system-root trust for this provider. Passwords
are read through the protected bounded file reader on each lookup, so credential
rotation is possible without reopening unsafe paths; symlinks and nonprivate
password files are rejected. Transport identity/configuration is captured when
the resolver is constructed, not hot-reloaded under an existing registration.

The callback requests only `entryUUID` with an exact escaped canonical nonzero
lowercase UUID filter, subtree search under the configured base, alias
dereferencing disabled and size limit two. The shared bounded, cancellable LDAP
transport retains TLS hostname verification, bind/search deadlines, referral
rejection and returned-DN scope checks. Exactly one entry, one `entryUUID`
attribute and one matching value are required. A same-label/different-UUID
record, missing or ambiguous evidence, transport error or failed bind cannot
authorize submission or approval. This is read-only subject-presence evidence,
not enrichment, authenticated root admission or certification of directory ACLs.

Live TLS LDAP protocol fixtures verify exact bind/filter/attribute/limit
requests, failure recovery, duplicate/missing/wrong UUID evidence, foreign DN
scope, credential rotation, cancellation, unrelated CA rejection and protected
credential replacement. Composition tests use actual durable authority/source
registrations and a coherent store capture: failed presence checks do not consume
the semantic retry key, independent approval checks the directory, source
disable fences a new approval before LDAP, and an identical historical approval
retry after database reopen does not query the directory again.

The review HTTP composition below authorizes every attempt before receipt
access and defers current-provider validation errors until after historical
receipt lookup; directory outage cannot prevent retrieving committed historical
evidence. No review UI, permission grant, active-link publication, snapshot
delivery or VS enforcement is enabled automatically. Production directory
uniqueness/ACL provisioning, signed snapshot contract and timing/rollout remain
open gates.

#### Implemented: opt-in authenticated review transport

Split Control-Service now composes the three isolated proof routes:
`POST /api/subject-link-reviews/proposals`, `GET
/api/subject-link-reviews/{id}?organization_id={organization}`, and `POST
/api/subject-link-reviews/{id}/approve`. All responses are no-store. The
[dashboard configuration and request contract](zpr-dashboard/README.md) describes
the exact fields, response codes and explicit environment configuration.
The database and providers are disabled by default; partial or invalid opt-in
configuration fails startup. Existing delegated trust is loaded through the
protected file reader. The service opens only its own review database, without
reading Simulator or inferring registrations from another source registry.

Strict bounded JSON rejects duplicate keys, field aliases, null/unknown fields
and client-supplied review actors/submission times. The verified proof grant
supplies the principal, not a browser field or header. Backend proof verification
checks mTLS identity/pin, exact method/path/query/body/retry key, organization,
permission and persistent one-use nonce before accessing review records or
receipts. New decisions still atomically fence exact authority/source versions,
captured pair digest and key scope through the required directory check.
Independent approval checks the persisted author and creates `review-approved`
evidence only.

Current registry/provider resolution is performed outside the Store transaction
to avoid reentering its single connection. Resolution failures are returned by
the mandatory callback, not before invoking the retry-aware method. Consequently
an authorized identical historical retry still returns its committed evidence
after source disable, provider unavailability or database reopen, while new work
fails closed. The store rechecks registry identity inside the transaction, so
an intervening source edit cannot authorize a stale capture.

Control Room has a dedicated named-user proxy for this prefix, with no local
editor fallback. It authorizes the actual request organization and exact
permission, requires mutation Origin/CSRF through the real session authorizer,
and signs a fresh isolated review proof. The production mTLS proxy removes
browser cookies, bearer credentials, Origin, CSRF and forged delegation before
attaching only the server-signed proof. Generic enrollment signing remains
unable to sign review routes. No automatic lost-response retry is introduced.

Race tests exercise actual TLS OIDC token exchange/session verification,
production review route composition, the production mTLS proxy, a real TLS LDAP
protocol fixture and durable SQLite review/receipts. They deny missing sessions,
wrong permission/organization, missing/cross-origin CSRF, self-review, missing
or altered proof, nonce replay after reopen and wrong pinned Room certificate.
The accepted-response-loss fixture aborts only after a successful backend commit:
Room sees 502 and an explicit fresh-proof/same-key retry returns the original
record without a second LDAP read. Logout denies the old cookie; reconstructed
backend grants deny removed principals even for historical receipts.

This is not a deployed workflow or M1 completion. Registration provisioning and
review-context discovery now have the approved operator HTTP path below; the
review UI remains absent.
Request root IDs are candidates for review, not verified authentication results
or VS admission evidence. Signed snapshots, active binding/revoke/delivery,
strict VS enforcement and gateway acknowledgements remain gated.

#### Implemented: separately authorized registry administration

The operator approved named-user administration with an explicit
organization-scoped `subject-link.registry.manage` permission. The new
`/api/subject-link-registry/` routes provide exact-CAS authority/source creation,
versioned edits/disable, current reads, immutable versioned audit reads and a
read-only coherent review-context POST. The
[dashboard contract](zpr-dashboard/README.md#subject-link-registry-administration)
defines their strict field shapes and recovery behavior.

The existing exact-request signed envelope/algorithm is reused with a distinct
registry purpose, not a new signed-snapshot or active-binding format. Registry
management is independently checked at Room and backend, never inferred from
`subject-link.propose`, `subject-link.approve`, generic policy editing or
organization membership. Read/history/context requires `subject-link.read`;
even an administrator needs an explicit read grant. Named proof identity supplies
the immutable audit principal. No registration, provider, trust or permission is
silently provisioned at startup.

Registry edits deliberately do not implement review semantic receipts:
`Idempotency-Key` is rejected. After a lost reply an operator must read current
state and historical version evidence; stale CAS conflicts. An unchanged config
at the exact current version remains a no-op. No LDAP call is made to edit/read
registrations or capture context, so these operations do not pretend to certify
directory subjects, uniqueness or ACLs. New reviews still require independently
configured exact-version provider mapping and an exact directory presence check.

Tests cover explicit management/read separation, foreign scopes, strict nested
configuration fields, no-op/stale CAS, versioned disable/re-enable, immutable
authorized audit evidence after reopen and changed context digests. The actual
OIDC/mTLS/LDAP review chain now begins with an empty registry: a separately
authorized administrator creates authority/source registrations, a reader obtains
coherent context, and the independent proposer/reviewer use that context. The
administrator cannot submit without a proposal grant. Missing version-bound
transport fails a new review without a directory query. Registry/enrollment/
review signing scopes remain isolated.

#### Verification and next gates

New race tests cover unsupported selectors/configurations, exact CAS and one
concurrent winner, audit rollback, immutable evidence, foreign organization and
unregistered-source isolation, out-of-scope keys, stale captures, independent
approval, provider-check ordering, source-edit fencing/cancellation, receipt-write
rollback and historical replay after source replacement and database reopen.
Existing signed operator proofs plus persistent nonce consumption compose with
the new review methods: old proof replay fails after reopen, fresh proof permits
the original semantic retry, and reconstructed backend grants deny removed
principals before receipt access. The additional review transport tests above
now cover HTTP/mTLS composition, not production identity admission.

All changereview race tests pass; the six new registry tests pass ten consecutive
race-enabled runs. Delegation, operator-auth and Control Room race tests and Go
vet pass. Required `make test` passes 11 operator-stack checks and retains exactly
the same 13 pre-existing Go failures. No dependency manifests or Rust changed.

Next: review UI and real-browser acceptance. Protected configuration, named
registry administration/context discovery, provider-reference resolution and
review HTTP/proxy composition are implemented, not deployed.
Active publication/revocation and VS delivery require a separately
approved shared signed-snapshot format, key rotation/recovery rules and the
still-unset timing/enforcement bounds. No numeric defaults are invented here.

### S05 minimal subject-link contract: review proposal (2026-10-24)

Status: **binding contract design only; not approved or implemented**. The M1 reconciliation
authorized this design review, not the previously deferred principal-envelope,
shared-schema or delegated-source credential implementation. No new lookup
mode or registration file is introduced by this document.

#### Observed implementation boundary

- `trusted_services::lookup_identities` retains a selected claim's key/value,
  but drops its issuer/source before returning the tuple.
- `TrustedServicesMgr::get_attributes_for_actor` forwards that same tuple set
  to every configured provider. The existing same-subject/two-store test
  explicitly proves this behavior; canonical key qualification occurs after
  lookup and therefore cannot establish subject separation.
- Connection admission collects lookup identities from authenticated claims;
  later actor refresh collects them from persisted actor attributes. A
  provider-enriched claim must not become a new authentication root merely
  because it is present at refresh time.
- The validation/2 authentication result already carries verified
  `auth_service_id`, `client_id` and expiration. Its verified token subject is
  bound to the adapter client, not automatically to a human user's directory
  UID. RSA bootstrap is a distinct root and must remain so. Derived user/device
  authority claims are not, by themselves, a durable subject-link record.
- Current provider interfaces accept plain identity pairs and current source
  revisions are snapshot revisions, not authenticated registry identity
  versions. Neither can be relabeled to claim the missing contract exists.

#### Proposed invariant and minimal records

An enrichment lookup is permitted only by an explicit binding from a verified
root to an exact provider subject. Matching CN, UID, DN, mail, policy alias,
display name or attribute value never creates a binding.

The proposed records contain:

| Record | Required fields and meaning |
| --- | --- |
| Authentication root | Organization; registered authentication authority ID and trust/config version; subject kind (device or user); issuer-qualified immutable subject ID; authentication expiry; actor/session incarnation. Set only by the successful authentication boundary, not caller claims or enrichment. If an immutable human subject is unavailable, do not manufacture one from a device client ID. |
| Provider subject | Organization; stable source ID and identity version; configured lookup-key kind; immutable provider subject ID. Bind the mapping/lookup key to the source version; directory UID/DN rename or reuse must not silently identify another principal. Providers lacking an immutable subject selector need a separately reviewed resolver or remain unsupported for strict links. |
| Approved binding | Stable binding ID and monotonically increasing version; one authentication root identity to one exact provider subject; permitted enrichment identity keys; approver identity/time; validity bounds; active/revoked state. Pin both authority/source versions. Store no reusable provider credential in the binding. |
| Actor binding snapshot | Verified root/session incarnation plus the exact binding IDs/versions and source identity versions used for the stored observations. Persist as service-owned bookkeeping, not policy-supplied actor claims or address-only cache entries. |

The binding direction is root-to-provider, not a general graph. Multiple
providers require multiple direct approved bindings. No transitive traversal,
reverse inference, wildcard subject, implicit self-link or field-derived
auto-approval is proposed. A provider can be the root authority only when an
explicit, verified authority/source registration says so; equal names are
insufficient. Keep organization and device/user subject kinds in every check.

This models explicit correlation to an enrichment record. It does **not**
delegate a principal's source-access permissions, provide per-principal field
visibility or authorize privileged directory writes. Those require the deferred
source-side/dual-principal contract below.

#### Lookup, refresh and evaluation behavior

1. At authentication, construct the root from the verified protocol result.
   Persist it with a new actor/session incarnation before trusting it on later
   refreshes. A reauthentication or address reuse cannot inherit prior links.
2. Resolve direct active bindings under the current organization, authority and
   source versions. Validate validity/revocation before querying any provider.
   Pass only the target source's bound selector to that provider; never broadcast
   the root's raw identity pairs to other sources.
3. Keep canonical claim qualification and the existing field-mapping checks.
   Provider claims can contribute only within the approved binding's scope;
   they cannot add roots, approve further bindings or widen lookup keys.
4. Persist observations together with the binding/version snapshot. A failed or
   canceled write must not certify the new binding state in revision memory.
   Recheck root/binding/source publication before accepting an admission,
   refresh or visa activation. Existing optimistic checks alone are not an
   atomic binding-publication/activation barrier.
5. Missing, expired, revoked, mismatched or unavailable binding evidence is
   indeterminate for the affected source, **not a successful empty directory
   result**. Absence/negation rules must not grant because a link disappeared.
   Provider unavailability remains indeterminate as well.
6. Until S06's complete dependency semantics are approved and implemented,
   retain conservative whole-actor admission/request denial and existing-visa
   revocation on indeterminate required observations. This proposal does not
   use missing links as a shortcut to selective availability.

#### Durable invalidation and restart

Revocation, expiry, source identity edit or authentication-root change must
invalidate cached observations and affected grants, not just future lookups.
Persist each visa's root/binding/source-version dependencies, or use a
reviewed conservative all-grants invalidation mechanism with equivalent restart
enforcement. A durable invalidation marker must precede acknowledging a link
revocation. Block new authorization on the retired version, queue existing-visa
revocation, and retain explicit pending/error state until authoritative readback
and the required relay acknowledgements satisfy the acceptance bound.

At restart, reconcile persisted actors and visas against current approved
binding/version records before exposing installation queues or permitting
authorization. Missing registry/version evidence is not permission to reactivate
old visas. Acknowledge that already installed network grants require a bounded
revocation-delivery contract; a successful API response alone cannot prove that
revocation has reached nodes. The bound is a review decision, not assumed here.

#### Implementation/approval gates

Before implementing this proposal, agree on:

1. Authentication authority registration/version ownership and the immutable
   subject selectors actually available for each authentication/LDAP provider.
   Choose an authenticated source-version delivery contract for VS; do not
   infer versions from local snapshot counters or Policy-Service stage metadata.
2. The production owner and protected durable storage/API for bindings,
   organization-scoped approver permissions, optimistic version conflicts and
   auditable approval/revocation. Control Room consumes this production contract;
   it must not read Simulator data or grant approval by editing browser state.
3. Service-owned persistence for roots, actor incarnations and visa dependencies,
   restart/install-queue enforcement, and publication/activation fencing. Review
   any required shared API/schema change separately.
4. Revocation acceptance timing and delivery evidence; operator-visible
   pending/reconciling/failure states when enforcement cannot be confirmed.
5. Rollout/compatibility: strict linked deployments must never fall back to
   current string fan-out for old/missing records. Legacy behavior, if retained
   for evaluation-only use, must be explicit, labeled and excluded from M1 S05
   sign-off. No implicit compatibility default is selected in this proposal.

#### Required S05 acceptance packet

- Two sources with the same username and different grants: a binding only to A
  performs no B lookup and cannot obtain B claims.
- Explicit A/B bindings to distinct provider subjects yield separately qualified
  claims without merging authority identities or granting a transitive link.
- A device root cannot masquerade as a human root; foreign organizations,
  equal display names and mismatched authority/source versions are rejected.
- Expired/revoked/missing bindings cannot satisfy positive, absence or negated
  admission/request conditions. No provider query leaks an unbound selector.
- Approval/revocation and config conflicts remain explicit; workload credentials
  cannot approve links. Browser/source selection never changes bindings.
- Provider failure/recovery does not recreate revoked links or widen scope.
- Reauthentication/address reuse, cancellation, concurrent refresh and
  activation crossing revocation cannot commit an old binding version.
- Source identity edits, link revocation and link expiry invalidate existing
  grants with measured delivery/reconciliation evidence.
- Restart during each pending transition cannot restore old installation work
  or trust a missing binding registry.
- Desktop/tablet V04/V17 show the exact authority/source/binding identities,
  version and enforcement status; fixture presentation and integrated runtime
  evidence are recorded separately.

This packet remains open until both the contract decisions and runtime
acceptance are approved. S05 is not closed by the records or this design alone.

### S05 production binding contract: ownership and API review (2026-10-24)

**Approved direction:** Control-Service owns approved subject-link records;
VS enforces authenticated, versioned snapshots. Initial provider support
requires a verified immutable subject identifier, with no UID/DN/mail fallback.
**Authorized scope is contract-first documentation only.** The following API
names, wire shapes, lifecycle and acceptance requirements are proposals, not
implemented endpoints or an approved shared schema.

#### Ownership and approval boundary

Reuse Control-Service's production organization/source registry ownership.
Keep one protected durable writer initially, consistent with current source
registration; do not claim HA. VS stores enforcement snapshots, dependencies and
reconciliation state, not an independently editable copy of approved links.
Control Room presents the production APIs and never reads Simulator state.

The current source configuration has server-owned source versions, a registry
content revision and per-organization scopes. It has no immutable-subject
selector or authority registration. Its local-request/proxy gate is not proof
of an approver's identity or organization-scoped approval permission. These
existing fields/checks cannot be relabeled as the missing approval contract.

Require a verified operator principal with explicit organization-scoped
link-review permission for approval/revocation. Derive the approver and audit
time server-side. A browser-submitted approver, workload/service credential,
organization selection or successful source connection test is not approval.
VS's delivery credential permits snapshot consumption/readback only, not link
creation. Exact operator authentication, permission provisioning and independent
reviewer requirements remain approval gates.

#### Proposed records and wire rules

The service generates record IDs and monotonically increasing record versions.
Use decimal **strings** for versions/sequences on the JSON wire to avoid
JavaScript integer truncation. Reject unknown fields, duplicate object keys,
invalid/empty IDs, unsupported subject kinds/selectors and invalid validity
intervals. Define bounded record counts/payload size before implementation.

An approved binding's logical shape is:

```json
{
  "binding_id": "link-opaque-id",
  "binding_version": "1",
  "organization_id": "org-opaque-id",
  "root": {
    "authority_id": "authority-opaque-id",
    "authority_version": "3",
    "subject_kind": "device",
    "subject_id": "verified-opaque-device-subject"
  },
  "target": {
    "source_id": "company_directory",
    "source_version": "4",
    "selector_kind": "ldap_entry_uuid",
    "subject_id": "b6a21ab4-5554-4cde-ae65-41a406a3c07f"
  },
  "enrichment_keys": ["user.department"],
  "not_before": "2026-10-24T00:00:00Z",
  "expires_at": "2026-10-25T00:00:00Z",
  "state": "active",
  "approval": {
    "principal_id": "verified-operator-id",
    "approved_at": "2026-10-24T00:00:00Z"
  }
}
```

This is a server response, not an approval request to be accepted verbatim.
The root reference excludes an ephemeral authentication session: a direct
approved link can be resolved for a new verified session of the same registered
root. VS binds every observation/grant to that actual session independently.
The current preserved device identity lacks authority registration/version and
organization, so it cannot yet satisfy this reference. Human subjects remain
unsupported until a verified immutable human-subject authentication path exists.

#### Proposed operator and delivery surfaces

Paths below are review labels; no route is added by this packet.

| Surface | Request/response contract |
| --- | --- |
| `GET /api/subject-links?organization_id=...` | Authorized organization-scoped records, registry epoch/sequence and enforcement status. Explicit pagination with a consistent snapshot token; switching organization invalidates prior browser responses. |
| `POST /api/subject-links/proposals` | Organization, expected registry revision, exact root/target references, enrichment scope and validity. Server validates registration/selector support and returns an immutable proposal ID/version/digest. No active lookup permission is created. |
| `POST /api/subject-links/{id}/approve` | Expected proposal version/digest, expected registry revision and idempotency key. Approver identity comes from verified authorization. Recheck source/authority versions and the reviewed proposal at commit; a mismatch conflicts without publishing a link. |
| `POST /api/subject-links/{id}/revoke` | Expected binding version, reason and idempotency key. Persist a new version, tombstone and delivery operation atomically before acknowledging durable acceptance. Never hard-delete the only revocation evidence. |
| `GET /api/subject-link-operations/{id}` | Durable mutation version, intended VS recipients and per-recipient received/applied/reconciling/enforced/error state. A committed edit is not network enforcement success. |
| `GET /api/subject-link-snapshots/current` | Authenticated VS receives its complete organization-scoped snapshot, targeted to that registered VS identity, including source/authority versions, active bindings and revocations. No bind secrets or source credentials. |
| `POST /api/subject-link-operations/{id}/receipts` | Authenticated VS reports the exact snapshot epoch/sequence/digest, reconciled dependency version and pending/revoked grant evidence. The server cannot accept another VS's receipt. |

Use explicit 401/403 for absent/unauthorized identity, 409 for version or
idempotency-content conflicts, 422 for unsupported/invalid bindings, and
503 for unavailable authoritative state. Successful approval/revocation commit
returns 202 with an operation ID until enforcement is confirmed. An identical
idempotent replay returns the same operation, never another approval/version.
Receipt transport retries cannot convert an error/pending operation to success.
Edits to endpoints/scope/validity require a new reviewed version; revocation is
terminal for that approved version, not a toggle that silently reactivates it.

#### Authenticated snapshot and restart contract

Each complete snapshot carries schema version, organization, intended VS ID,
registry epoch, monotonic sequence, exact source/authority versions, issue time,
lease expiry, bindings/tombstones and an authenticated payload digest.
Content revision is not an ordering sequence; source snapshot counters and
policy staging metadata cannot stand in for this contract.

Provision the trusted Control-Service issuer independently of the payload.
Approve the concrete transport/signature format, exact signed bytes and key
rotation rules separately; do not invent a new signing library or accept keys
supplied by the snapshot itself. Authenticate both service endpoints and bind
the payload to its organization and VS recipient. A bare digest is not proof.

VS validates the entire snapshot before publication: reject unsupported schema,
duplicates, invalid selectors, unknown authority/source versions, expired leases,
wrong recipient/organization and invalid authentication. Persist the accepted
epoch/sequence/digest with the enforcement snapshot before acknowledging it.
Reject older sequences and same-sequence/different-digest payloads. Treat
same-sequence/same-digest delivery as an idempotent replay, not a lease renewal.
A new lease requires a newly authenticated publication. Registry epochs cannot
reset anti-rollback state automatically; recovery/restore needs explicit
operator reconciliation, particularly if either service's durable state is lost.

A complete snapshot is all-or-nothing, not an accumulation of pages/deltas.
Omission of a formerly active binding retires it and invalidates dependencies;
it never means "keep the cached approval." Retain tombstones until all intended
recipients and the restart/retention contract make compaction safe. Source or
authority edits must atomically advance the binding publication sequence or
hold dependent approval unavailable; unrelated files cannot leave a mixed
source-version/link snapshot publishable.

On restart, block relevant admission, evaluation and installation queues until
the persisted snapshot, anti-rollback state and grant dependencies reconcile.
An unavailable delivery service does not produce a successful empty registry.
An expired lease makes affected observations indeterminate and requires the
existing conservative denial/revocation behavior until S06 is implemented.
Already installed grants require bounded revocation delivery or independently
enforced lifetime limits; lease expiry alone cannot prove node enforcement.
Revocation fan-out to multiple VS instances is pending until every intended
recipient is accounted for, not successful after the first receipt.

#### Immutable directory selectors

Register exactly one supported selector kind/encoding per source identity
version. For LDAP `entryUUID`, use validated UUID canonical text. For AD
`objectGUID`, define canonical UUID text and explicitly tested AD binary
byte-order conversion before supporting that kind; do not treat binary GUID
bytes as an arbitrary UTF-8 string. These identifiers are source-qualified,
not globally interchangeable principals.

Request the selector explicitly (including operational attributes where
required), validate read-only ACL visibility, and perform an exact escaped
filter within the configured organization base with existing TLS, referral,
time and size limits. Require one exact matching entry and verify the returned
immutable identifier; missing/duplicate/malformed selectors or denied reads are
indeterminate. UID/DN/mail may be shown as labels but cannot rescue a failed
selector lookup. Rename preserves identity; deletion/recreation with the same
UID but a different immutable ID cannot reuse a link. HTTPS providers need an
equivalent authenticated stable-ID contract before strict support.

Provider eligibility is not certified by a one-entry test alone: directory
uniqueness/reuse semantics and read ACLs need approved deployment evidence.
The existing LDAP catalog scan and plain-pair VS lookup interface do not
implement these exact-selector requirements.

#### Contract acceptance packet and remaining decisions

| Case | Required observable outcome |
| --- | --- |
| Browser changes organization during review | Stale response/approval rejected; no foreign records or diagnostics attach to the current organization. |
| Local browser request without verified approval permission | Cannot activate/revoke links, even when the current source editor would accept a local request. |
| Authority/source edit after proposal review | Approval conflicts; no automatically rewritten endpoint or version. |
| Same username in A/B; only A binding | No B lookup/claims, no inferred self/reverse/transitive link. |
| Rename and delete/recreate of directory entry | Rename retains immutable identity; reused label cannot acquire the retired subject's approval. |
| LDAP selector absent, duplicated, unreadable or malformed | Explicit indeterminate state; absence/negation cannot grant. Test AD binary vectors separately if supported. |
| Snapshot tamper, foreign recipient, rollback or equal-sequence equivocation | Reject without replacing accepted state or renewing its lease. |
| Lost approval/revoke reply and identical retry | One durable version/operation; same-key/different-content retry conflicts. |
| Source edit, snapshot delivery, refresh and activation interleave | No grant committed against an unaccepted/retired binding version; durable pending reconciliation is visible. |
| Snapshot outage/lease expiry and restart mid-revocation | No old installation queue revival; pending/error remains visible until authoritative enforcement evidence. |
| Two VS recipients, one unreachable | One recipient's success cannot complete the operation; measure the agreed delivery bound. |
| Simulator unavailable and directory writes rejected | Production approval/delivery/enforcement operates independently and issues no LDAP writes. |

Before runtime implementation, approve operator identity/permissions,
authority registration ownership and version mapping, snapshot authentication
and trust provisioning, durable publication storage/atomicity, lease/clock-skew
and revocation bounds, retention/recovery rules and any shared-schema changes.
The directions approved above do not settle these choices. No numeric timeout,
strict-mode rollout default or production enforcement guarantee is invented by
this packet. S05 and M1 remain open.

### S05 operator authorization foundation (2026-10-24)

The operator approved reusing verified OIDC sessions with dedicated
`subject-link.read`, `subject-link.propose`, `subject-link.approve` and
`subject-link.revoke` permissions. Approval requires an independent verified
issuer/subject pair; changing a display name or email does not make a proposer
an independent reviewer. This supersedes the undecided permission/reviewer
choices in the contract review above, not its other implementation gates.

The existing operator permission validator now accepts these explicit grants.
Control Room's authorization map reserves exact operator-facing routes from
the proposal for their corresponding permission. General `policy.edit`,
`change.review`, directory publishing and generic `approve` do not imply
subject-link permission. Unknown methods, malformed IDs, extra path segments,
snapshot delivery and VS receipts are not authorized by this operator map.
Existing organization-scoped session, origin and mutation-CSRF checks remain
in force.

`operatorauth.RequireIndependentReviewer` validates the exact issuer/subject
pair comparison. It is an explicit primitive for the future approval commit,
not a complete authorization decision: callers must first authorize the
reviewer and load the proposer from the authoritative reviewed proposal.
No active approval handler or proposal store exists yet, so the helper is not
currently an enforced two-person approval workflow. Proposal/version/digest
checks and independent-reviewer enforcement at the durable commit remain
required before enabling approval.

Enrollment delegation's independent grant validator accepts the dedicated
permission names through the shared validator, but its signed-request route
scope is unchanged. It cannot sign subject-link operator/delivery/receipt
requests. Reusing named identity does not authorize forwarding an enrollment
proof to these endpoints. The subject-link delegation/delivery audience and
request-binding contract still needs its own approved integration.

Acceptance coverage exercises all four exact session permissions, organization
isolation, absent/expired sessions, forged identity headers, same-origin/CSRF
checks, exact route/method mapping and denial before proxy forwarding. Reviewer
tests distinguish issuer-qualified principals from display names and reject
incomplete identities. Delegation tests keep subject-link routes outside the
enrollment signing scope. Route tests deliberately configure Simulator
unavailable.

Validation: the operatorauth, controlroom and operatordelegation package suites
pass with `go test -race`. The configured `make test` ran the operator-stack
checks successfully (11 cases), but the full dashboard command suite reports
13 failures in other fixture-dependent/Rust-interop tests; this is not a clean
full-dashboard result. No Rust code changed in this packet.

No subject-link endpoint, approval storage, service-delivery protocol, UI or VS
lookup enforcement was added. Dedicated permissions are not auto-granted.
Production provisioning allowlists and the future backend approval boundary
must be wired explicitly when those features are implemented. The existing
local-development proxy fallback is not approval authority and must never be
used to enable the future link API. S05/M1 remain open.

### S05 durable subject-link review foundation (2026-10-24)

The operator separately approved dedicated subject-link review tables in the
existing protected SQLite review store. These tables are isolated from the
policy/gateway queue and policy installation receipts. This narrows the
previous "no approval store" limitation to **no active binding/publication
store or approval handler**.

`SubmitSubjectLink`, `GetSubjectLink` and `ApproveSubjectLink` persist an
immutable proposal, server-generated ID/digest/submission time, decimal-string
review version and attributable audit events. Initial candidates support device
roots and canonical, nonzero LDAP `entryUUID` targets only. Missing/invalid
references, noncanonical or overflowing positive version strings, duplicate or
invalid enrichment keys, expired/reversed validity and mutable selectors are
rejected explicitly. AD GUID, HTTPS stable-ID and human-root support remain
unsupported. Shape validation is not proof that a registration or directory
subject exists, or that a configured field is approved by the provider.

The only decision transition is
`submitted -> review-approved` with review version `"1" -> "2"`. Approval reads
the immutable proposer inside the transaction, compares exact digest/version
and the supplied current registry revision, requires a different verified
issuer/subject, and writes the decision and event together. There is no
self-approval override, including the generic review feature's single-operator
setting. A changed display name or caller-side proposal copy cannot bypass
the persisted-author check. Failed/stale/conflicting decisions write no audit
event; an audit write failure rolls back the state transition. The immutable
proposal/event triggers reject updates/deletion of the review evidence.

This is an internal persistence API, not an HTTP authorization boundary.
Its caller must authenticate and authorize the proposer/reviewer with the
dedicated organization permissions and obtain organization/author/registry
context server-side. The principal-shape validator does not authenticate.
The registry revision comparison records the review context provided by that
caller; it is **not an atomic guard against changes to a separate registry
file**, nor validation of actual authority/source registrations. Those checks
must be integrated before approval can produce an active binding.

`review-approved` intentionally confers no lookup permission. There are no
subject-link HTTP handlers, UI, activation, registry publication, VS delivery or
revocation operations. Review submission/approval has no idempotency-key API:
submission creates a new record, and repeating an old decision conflicts.
Do not claim the proposal contract's lost-reply idempotency requirement is
implemented. Cancellation/rejection/expiry cleanup and bounded listing are
also future workflow work; this packet exposes exact-record reads only.

The tables are initialized when the existing review store opens, using the
same owner-private directory/database and single-writer deployment limits.
No new service configuration, database path, dependency, auto-grant or shared
wire schema is introduced. Existing policy/gateway tables/records and their
decision rules are unchanged; enabling the current change-review UI does not
expose subject-link records or let them authorize policy installation.

Validation: changereview, operatorauth, controlroom and operatordelegation pass
with `go test -race`. New store tests exercise independent approval, exact
review/registry matching, malformed inputs, organization/queue isolation,
input-mutation isolation, private file permissions, close/reopen persistence,
two concurrent reviewers, immutable evidence and transactional audit failure.
The configured `make test` passes its 11 operator-stack checks but reports the
same 13 unrelated dashboard command failures as the preceding packet. No Rust
code changed. These tests do not establish process-crash, directory/provider,
delivery or production UI acceptance. M1/S05 remain open.

### S05 durable review retry receipts (2026-10-24)

The review store now offers `SubmitSubjectLinkOnce` and
`ApproveSubjectLinkOnce` with a required, bounded retry key. They supplement
the original unkeyed methods, whose create/conflict behavior is unchanged.
This supersedes the preceding packet's no-idempotency limitation at the
**internal review persistence API only**, not the proposed HTTP workflow.

An immutable receipt is scoped by organization, exact verified issuer/subject
and retry key. The request fingerprint includes the operation and candidate or
decision inputs: changing the reviewed digest/version, captured registry
revision, target, enrichment scope, validity or reason conflicts without
mutation. The same principal cannot reuse a submission key for approval.
Other principals/organizations have independent key spaces and cannot retrieve
another principal's response through a receipt replay.

Submission fingerprints omit the server-owned submission timestamp and
author display name, and normalize validity timestamps to UTC. Display-name
changes or equivalent timezone representations do not create a second request;
the original author label and timestamp remain in the stored evidence.
Enrichment-key order is preserved as part of the candidate representation.
Approval fingerprints likewise exclude the reviewer's display label, but pin
the exact supplied registry observation. A newly captured different registry
revision therefore conflicts; it is not permission to reuse an old approval.

The review mutation, audit event and original response receipt commit in one
SQLite transaction. Failed validation, denied self-review, version conflicts
and audit/receipt persistence failures do not consume the key. Concurrent
identical attempts through the supported single-writer store return one
committed result; the receipt and mutation survive closing/reopening the store.
Receipt-update/delete triggers preserve the original replay evidence.

Replay returns the **original response**, not the current review record.
Replaying submission after approval still returns its original `submitted`
response and cannot revert the current `review-approved` record. Call the
exact-record read API to reconcile current state. Existing receipts are
historical evidence and are not reevaluated as a fresh approval against the
clock, registry or provider. Their existence never renews validity, grants
lookup access or substitutes for active-binding checks.

Every future HTTP attempt must authenticate and authorize again before calling
these APIs, including replay attempts; database principal-shape checks are not
authorization. No HTTP routes, active binding publication, delivery or
revocation are added. Receipt retention/compaction and multi-writer contention
remain outside this packet. Retry-key storage is durable and unbounded, so
production request/retention limits must be resolved before exposing it.

Validation covers original-response replay after close/reopen, proposal and
approval content conflicts, cross-action conflicts, principal/organization
isolation, invalid keys, concurrent identical submissions/approvals, rejected
self-review without key consumption, receipt-write rollback and immutable
receipts. The changereview, operatorauth, controlroom and operatordelegation
suites pass with the race detector. The configured `make test` passes its 11
operator-stack checks and retains the same 13 dashboard command failures
outside this work. No Rust code, manifests or dependencies changed.
M1/S05 remain open.

### S05 Control-Service review integration contract (2026-10-24)

**Scope:** documentation-only integration proposal requested by the operator.
No route, service configuration, delegation scope, database deployment or
active-binding authority changes in this packet. The ownership direction is
approved; the concrete integration below still requires review before enabling
HTTP access.

#### Observed composition and proposed ownership

The production modes already have distinct composition roots:

- `runControlRoom` in
  [control_service.go](zpr-dashboard/cmd/zpr-web-dashboard/control_service.go)
  opens the generic policy/gateway review store through `configuredChangeReview`.
  Its generic API is local to Control Room. Opening this store currently also
  initializes the isolated subject-link tables, but no handler exposes them.
- `runControlService` owns source-registry/runtime composition and the
  [production route mux](zpr-dashboard/internal/controlservice/httpapi/routes.go).
  It does not currently open or serve a subject-link review store.
- The Control Room mTLS proxy authenticates the calling service, not the
  human reviewer. `AuthorizeAPI` checks the human session at the UI boundary
  but does not convey a verified human principal to Control-Service.
  Existing enrollment/budget delegation is route-restricted; its signer is
  deliberately unable to sign subject-link requests.

Proposed initial topology:

| Responsibility | Owner and constraint |
| --- | --- |
| Operator login, CSRF/origin validation and presentation | Control Room; no direct subject-link SQLite reads/writes |
| Subject-link review API, immutable evidence, retry receipts and independent-review commit | One configured Control-Service writer |
| Source/authority registrations and authoritative review capture | Control-Service production contracts; no browser-supplied principal or provider-verification claims |
| Active bindings, publication sequence and delivery operations | Future Control-Service publication contract, distinct from review state |
| Installed snapshot, lease and dependent-visa enforcement | VS; no operator-editable link authority |
| Existing policy/gateway review queue | Remains in Control Room under its current configuration; not moved or broadened |

Reuse the tested subject-link persistence implementation, not the running
Control Room database. A future opt-in Control-Service configuration must name
its own protected absolute database path and independent lifecycle/backup
ownership. Never mount one SQLite file into both modes or let either process
claim it owns the other's writes. The current Control Room table initialization
does not authorize importing those rows as trusted Control-Service evidence.
If existing local review evidence needs promotion, design an explicit reviewed
transfer; do not silently discover or copy a UI database. No new environment
variable/configuration name is established here.

#### Separate review-only HTTP surface

The earlier `/api/subject-links/{id}/approve` proposal means active-binding
publication with a durable operation and 202 response. It must **not** be wired
to `ApproveSubjectLinkOnce`, which only records `review-approved`.
Reserve separate review-only routes instead:

| Proposed route | Required permission | Successful response |
| --- | --- | --- |
| `POST /api/subject-link-reviews/proposals` | `subject-link.propose` | 201, original immutable review response; required retry key |
| `GET /api/subject-link-reviews/{id}?organization_id=...` | `subject-link.read` | 200, current exact review and audit evidence |
| `POST /api/subject-link-reviews/{id}/approve` | `subject-link.approve` | 200, original `review-approved` response; required retry key |

These names are proposals, not currently mapped permissions or handlers.
The existing exact route map must be extended explicitly with tests; broad
prefix authorization is not sufficient. Review listing/pagination,
rejection/cancellation, publication, revocation, operation polling and
snapshot/receipt APIs remain separate future surfaces. Do not invent an
operation ID, `active`, `applied` or `enforced` status for a review response.
GET is the reconciliation path after uncertain mutation outcomes; it returns
current state, whereas keyed POST replay returns historical original evidence.

Submission contains organization, expected registry revision, root/target
references, enrichment keys, validity and reason. Approval contains organization,
expected review digest/version, expected registry revision and reason.
Require one `Idempotency-Key` header using the store's 1-128 ASCII
letter/digit/underscore/hyphen grammar. Reject duplicate keys/fields, unknown
fields, trailing JSON, noncanonical paths and unexpected/repeated query
parameters. Reject client-supplied author, reviewer, timestamps, state, digest
on submission or server ID. Use decimal strings for review/registration versions.
Future handlers must publish explicit body-size, header and rate limits before
exposure; the internal 128-enrichment-key/1,024-byte-reason bounds alone do not
provide HTTP resource limits.

Authorize before exact-record lookup or retry replay. Return no cross-org
record details; an authorized scoped lookup that finds no record is 404.
Use 400 for malformed request framing, 401 for absent/invalid operator proof,
403 for missing scope/permission or self-review, 409 for stale context or
retry-content conflict, 422 for invalid/unsupported candidate content and 503
for unavailable authoritative registration/store/trust state. Unexpected
persistence failures must be logged and surfaced as server errors, not
fabricated success or an empty record. All responses are `no-store`.
Existing UI auth helpers may currently return a coarser 403; implementing
these status distinctions requires deliberate handler wiring, not a claim
that the helpers already provide them.

#### End-to-end operator proof and commit ordering

Use direct HTTPS/OIDC at Control Room and mutually authenticated transport to
Control-Service. Add a separately approved subject-link request-proof scope
using the existing delegation machinery only where its contract fits; do not
broaden enrollment/budget route allowlists or trust raw identity headers.
The concrete proof encoding and configuration remain an implementation gate.
The required authenticated claims/bindings are:

- exact intended service audience and trusted signing key/issuer;
- verified operator issuer/subject, organization and requested permission;
- canonical method, path, query and exact body digest;
- the retry key (including absence for GET), short validity and unique nonce.

Control-Service must verify both the allowed mTLS caller and the operator
proof, enforce its own configured organization/permission grants, and reject
wrong audience, altered request/key, expired proof and replayed nonce before
accessing review evidence. Neither mTLS alone nor asserted grants from an
untrusted client suffice. New retry attempts obtain fresh proof/nonces after
current session/CSRF authorization, retaining the same request/retry key.
A consumed proof is not reused just because its original response was lost.
Operator proof nonce replay protection and semantic retry receipts have
different lifetimes and must not share a key space.

For a new review mutation, Control-Service captures registrations from its
authoritative organization-scoped source/authority view, verifies referenced
versions/selectors and compares the client's expected registry revision.
It derives principal/time server-side and calls the keyed store operation.
Approval independently loads the persisted author; no self-review override
is allowed. Required authority registration/version mapping is not implemented
yet, so a source-only capture is insufficient to enable this API.

Retry matching must use the original expected request inputs, not substitute
a newly captured revision into the fingerprint. After fresh authorization,
an existing exact receipt may be returned even if its review has expired or
registrations have since changed: it is historical evidence only. A missing
receipt requires current capture/validation before committing new evidence;
a changed request under the same key must conflict. Integration must preserve
this ordering transactionally rather than creating a second mutation during
a receipt-check race. The current store has no HTTP/capture transaction API;
design and test that integration before exposing the handlers.

Registry file capture plus SQLite commit is not atomic publication. Even
validated `review-approved` evidence grants no lookup access. Future publication
must recheck exact approved content and current source/authority versions
under one defined commit boundary, then atomically persist binding,
publication sequence, audit and delivery operation. An unavailable or changed
registry blocks publication; review evidence must never bypass that check.
No promise of immediate revocation, enforcement or multi-writer HA follows.

#### Implementation gates and acceptance

Before enabling review HTTP: approve the distinct review routes and limits,
Control-Service database configuration, subject-link proof audience/key
provisioning and authoritative authority/source capture. Receipt retention,
resource exhaustion controls and backup/restore identity must be explicit.
Keep disabled/unavailable behavior visible; no local-editor fallback,
Simulator fixture discovery or generic change-review configuration may enable it.

Required integration evidence, not yet executed:

1. Start Control Room and Control-Service with Simulator stopped, its URL
   invalid and its files/mounts unavailable; exercise only production APIs.
2. Verify Control-Service is the only review writer; UI restart/loss cannot
   delete or become authoritative for review state. Preserve generic
   policy/gateway review behavior without importing its rows/configuration.
3. Deny absent/forged operator proof even from an allowed mTLS caller; deny
   wrong caller/audience/org/permission, altered method/path/query/body/key,
   expired proof, reused nonce and enrollment/budget proof substitution.
4. Verify independent issuer/subject review, exact digest/version capture,
   missing authority registration and source/authority edits between capture
   and decision. No case creates an active binding or delivery operation.
5. Lose a successful reply, restart services and retry with fresh proof:
   return the original response once, with no extra record/audit event.
   Revoke the caller's grant and prove replay is denied before receipt read.
6. Replay submission after approval and approval after expiry/registry change;
   return historical evidence without reverting state, renewing validity or
   publishing. A changed input/key binding conflicts; GET reports current state.
7. Exercise concurrent retries, store/capture outages and audit/receipt-write
   faults; assert explicit failure or committed original evidence, never a
   success-shaped fallback or duplicate mutation.

This packet documents code inspection and proposed integration only. No new
tests were run for this documentation-only change, and no integration acceptance
is claimed. S05/M1 remain open.

### S05 isolated review request-proof foundation (2026-10-24)

The operator approved internal proof groundwork and tests, not enabled HTTP
routes or production trust provisioning. `operatordelegation.Signer` and
`Verifier` now expose explicit `SignSubjectLinkReview` and
`VerifySubjectLinkReview` methods. They reuse the existing Ed25519 signature,
canonical JSON/base64 envelope, trusted key/client-certificate pin validation,
independent backend grants and required replay-store interface. No new
dependency, shared schema, configuration loader or service handler is added.

The signed envelope uses purpose `subject-link-review-v1` and binds exact
organization, route-derived permission and retry key in addition to the
existing audience, key ID, operator issuer/subject, method, exact request
target/body digest, issue/expiry time and nonce. The existing enrollment
`Sign`/`Verify` methods remain enrollment-route restricted and require these
review-only fields to be absent. Their emitted envelope and replay-key format
are unchanged; review proofs use a separate purpose-prefixed nonce hash.
Existing policy/budget signing surfaces are not expanded.

Only the three proposed review routes in the integration contract are
accepted by the new proof methods: POST proposal, GET exact review and POST
independent approval. Active-link approval/revocation, snapshot delivery,
operation receipts, unknown methods and extra path segments are rejected.
This is a proof allowlist, **not route registration**: Control Room's current
HTTP permission map and Control-Service's mux still do not expose these routes.

The bound body limit is 8,192 bytes, using the existing delegation limit.
POST requires exactly one `Idempotency-Key` matching 1-128 ASCII letters,
digits, underscores or hyphens, and no query. GET requires an empty body,
no retry-key header and exactly canonical `organization_id=<organization>`
query text. Organization and record identifiers use the same bounded ASCII
grammar. Encoded paths, empty `?`, duplicate query parameters and unexpected
parameters are rejected. This establishes internal proof bounds, not complete
HTTP server limits, strict candidate JSON decoding or a retention policy.

The verifier requires a verified TLS leaf matching an explicitly trusted
certificate pin and signing key. It checks the exact principal's backend grant
contains both the organization and the route's dedicated permission **before**
consuming the nonce. Generic policy/change-review permissions are not substitutes.
Proofs live for 30 seconds with the existing maximum two-second future issue
skew. Reused nonces and unavailable replay persistence fail closed. A semantic
retry retains its request/key but obtains a newly signed proof with a fresh
nonce. Replay-protection consumption is distinct from review receipt mutation.
Returned grant slices do not alias the verifier's configuration.

Callers must still authorize the live OIDC session, origin/CSRF and selected
organization before signing. The verifier's organization argument must come
from strict request decoding; the proof API does not parse proposal JSON or
replace independent-review/source/authority validation. Backend grants are
captured at verifier construction, consistent with existing configuration
semantics: provisioning changes require reconstruction/restart. No dynamic
grant revocation or complete browser-to-store integration is claimed.

Tests exercise all three routes, exact body/key/scope binding, mTLS/trust and
permission denial, cross-enrollment rejection, signed wrong-purpose/scope/time
claims, malformed framing, exact body-size threshold, unsupported active/service
surfaces, fresh retry proofs, concurrent nonce consumption and replay-store
failure. Fixtures explicitly set Simulator unavailable and its manifest path
invalid. These are internal request fixtures, not live service TLS/deployment
acceptance or process-restart proof-store evidence.

Validation: race tests pass for operatordelegation, operatorauth, controlroom
and changereview, plus focused dashboard command compatibility covering
delegation/enrollment, budget authority, review and proxy authorization.
The configured `make test` passes 11 operator-stack checks and still reports
the same 13 unrelated dashboard command failures. No Rust code or dependency
manifests changed. S05/M1 remain open: authoritative authority registration,
review capture/receipt ordering, persistent proof-store composition, explicit
key/configuration provisioning and HTTP integration precede enabled review APIs.
Active publication/delivery/enforcement remain separate work.

### S05 durable proof and review composition groundwork (2026-10-24)

The operator approved a three-step implementation batch: persistent proof
nonces, exact review-only permission mappings, and composition tests joining
proof verification to retry-safe review persistence. This supersedes the
preceding packet's missing durable proof-store implementation and missing
review-route permission-map limitations, but **not** its missing production
service composition, authoritative capture or HTTP-handler limitations.

#### Persistent nonce consumption

The protected review `Store` now implements the existing delegation
`ReplayStore` interface using a separate `subject_link_review_nonces` table.
It does not reuse review retry keys, policy-operation nonces, generic policy
review records or enrollment/budget databases. The stored key is the verifier's
purpose-prefixed SHA-256 nonce hash; no proof payload or signing secret is
retained. Interface conformance is checked at compile time.

A transaction removes expired nonce rows, checks the fixed 8,192-live-entry
capacity and inserts the new hash exactly once. Valid hashes are canonical
lowercase SHA-256 hex. Clocks must be positive, representable Unix nanoseconds,
with expiry strictly later than now and at most 32 seconds away (30-second
proof lifetime plus existing two-second future skew). Invalid inputs, duplicate
nonces, capacity exhaustion, cancellation and persistence failure return explicit
errors. Capacity exhaustion is `ErrProofCapacity`; verifier callers see the
existing logged fail-closed delegation denial. There is no eviction of an
unexpired nonce to admit a new request. Cleanup and insertion roll back together
on failure. This follows the existing enrollment/budget bounded replay-store
transaction pattern while preserving this store's separate schema and errors.

Proof consumption commits **before and separately from** any review mutation.
An authenticated but stale/denied self-review or failed audit write can consume
its proof without consuming the semantic retry key. A new attempt must obtain
fresh proof and keep the same request/retry key. Do not move proof consumption
into the review transaction: rolling it back would make a previously accepted
proof reusable. The two transactions are deliberately not an atomic publication
protocol. A crash between them yields a consumed proof and no review mutation;
fresh-proof retry is the recovery path.

As with the other stores, cleanup assumes the operator's trusted service clock;
rollback/restore/clock correction and key/trust reconciliation still need a
deployment contract. A copied/restored database cannot be advertised as
retaining every nonce consumed after that backup. The capacity is an internal
resource bound, not permission to expose an unthrottled HTTP API. No running
Control-Service currently opens this store for subject-link proof verification.

#### Exact permission mapping

Control Room's authorization map now reserves precisely GET exact review,
POST proposal and POST review approval from the integration proposal, using
`subject-link.read`, `.propose` and `.approve`. HEAD, listing, deletion,
review-revocation, malformed IDs and extra path segments are not included.
Existing active-binding proposal-route mappings are unchanged. The proof
allowlist independently enforces canonical query/header/body framing; permission
mapping alone does not decode, sign or authenticate a forwarded human identity.

No handler, dedicated proxy, trust loader or mux registration is added. The
generic mTLS proxy is still not proof of a named human; a mapped route is not
an enabled review workflow. Local-development fallback remains unsuitable.
Permissions are not automatically granted, and the policy/gateway queue remains
where it was. Production database placement is still the separately configured
Control-Service ownership described in the integration proposal, not a shared
Control Room database.

#### Composition evidence and limits

New tests compose the actual signer, verifier, SQLite nonce store and review
retry APIs using test-owned candidate capture and pinned request TLS fixtures:

- Concurrent consumption has one durable winner; a consumed proof remains
  rejected after close/reopen. Exactly 8,192 live entries are allowed, the
  next entry is denied, and exact-expiry cleanup frees capacity. Invalid
  hashes/clocks, cancellation, closed-store and injected write failures fail
  explicitly; write failure also rolls back expired-row cleanup.
- A successful submission followed by lost reply/reopen rejects the old proof.
  Fresh proof with the same retry key returns the original review response;
  concurrent fresh proofs also share that response without another review.
- A verified author cannot self-review. An injected approval-audit failure
  writes no decision receipt, and a fresh proof can retry the same decision
  key successfully. A later identical approval returns the original evidence.
- A reconstructed verifier with the reviewer's grant removed denies the
  attempt before any receipt access; wrong-organization verification also
  denies without consuming the nonce. Existing verifier instances retain
  their captured grants until reconstructed, not dynamic grant revocation.
- Submission replay after approval remains historical; verified GET plus
  exact-record read reconciles the current `review-approved` state. SQL counts
  confirm one review, two events and two semantic receipts, with no generic
  policy review or policy operation created.

Tests explicitly configure Simulator unavailable and its manifest invalid.
These are composition tests, not enabled HTTP handlers, authoritative
source/authority lookup, live TLS handshakes or process-crash/deployment
acceptance. In particular they do not solve registry-file/SQLite atomicity
or new-receipt/capture ordering. Those remain requirements before enabling
review HTTP, and publication still needs its separate atomic version checks.

Validation: changereview, controlroom, operatordelegation and operatorauth pass
with the race detector; the durable-proof/composition tests also pass ten
repeated runs. Focused dashboard delegation/enrollment, budget authority,
change-review and proxy compatibility tests pass. Configured `make test` passes
11 operator-stack checks and retains the same 13 unrelated dashboard command
failures. No dependencies, manifests, Rust code, keys or deployments changed.
M1/S05 remain open. The next functional prerequisite is authoritative
organization-scoped authority registration/version capture, followed by
explicit Control-Service composition and review-only HTTP integration.

### S05 internal BAS authority registration and capture (2026-10-24)

The operator approved an internal BAS-only registry/capture batch. The protected
review store now provides `PutSubjectLinkAuthority`,
`CaptureSubjectLinkAuthority` and `GetSubjectLinkAuthorityEvent`. These are
internal persistence APIs intended for future Control-Service composition,
not exposed operator endpoints or running service configuration.

Registrations have an organization-scoped stable authority ID, server-owned
positive decimal-string version, kind `bas_device`, exact BAS service ID,
exact issuer and explicit enabled flag. Only BAS device authorities are
accepted; bootstrap/local-VS and human authority kinds remain unsupported.
Service ID and issuer are nonempty clean opaque strings bounded to 256 bytes.
In particular BAS token issuers need not be HTTPS/OIDC URLs, and are never
case-folded, trimmed or URL-normalized. This maps the existing verified
`AuthenticationService { service_id, issuer }` evidence shape; it does not
provision its trust key or make unverified claims authoritative.

`PutSubjectLinkAuthority` requires an authorized caller to supply organization,
stable ID, expected version, configuration, attributable verified principal
and reason. Empty expected version means create at `"1"`; all edits require
exact compare-and-swap against the existing version. Changed configuration,
including disable/re-enable or service/issuer replacement, advances the version
without wraparound. An identical configuration with the current version returns
the current record without another version/audit event; a stale expected
version still conflicts. A lost reply to an actual edit requires explicit
reconciliation using its versioned evidence, not an unguarded retry.

The same exact BAS service-ID/issuer tuple cannot be registered under two IDs
in one organization, even when disabled. Organizations have separate mapping
namespaces; no organization is inferred from an issuer or device subject.
An explicit remapping advances the existing authority's version. Deletion and
ID/organization mutation are prohibited; retirement is a versioned disable.
No default/wildcard authority, fallback or automatic registration exists.

Each actual mutation commits its complete configuration, version, actor,
reason and server time in an immutable history event in the same SQLite
transaction. Audit failure rolls back creation/update. The exact historical
event read is organization-scoped; historical enabled evidence is not current
permission. Tables remain separate from proposal/audit/retry/nonce and generic
policy queues. Single-writer deployment limits are unchanged. No authority
management permission or provisioning mechanism is invented by these internal
methods; future composition must independently authorize registry administration.

`CaptureSubjectLinkAuthority` validates a device root's organization, authority
ID/version and nonempty subject, then reads one coherent current registration.
Unknown scoped IDs are not found; obsolete or disabled versions conflict.
The returned value pins the mapping observed for that root, not device
possession, provider-subject existence or a registered human identity.
It is a point-in-time read, **not a lock across later review or publication**.
Subsequent edits invalidate a new capture of the old version without rewriting
immutable existing proposals or historical audit evidence.

The earlier review submit/approve APIs still do not automatically call this
capture API. Consequently a raw internal review remains shape-validated
evidence only. Combined authoritative source/authority capture, binding its
revision to retry semantics, commit-time revalidation and production HTTP
composition remain unimplemented. This registry does not close that atomicity
gap, change VS lookup fan-out, disable installed visas, publish a snapshot or
activate/revoke a subject link. Control Room does not become its authority
merely because the shared store schema initializes these tables.

Validation: targeted race suites pass for changereview, operatorauth,
operatordelegation and controlroom; authority tests pass ten repeated runs,
focused dashboard review/delegation/enrollment/budget/proxy compatibility passes,
and changereview `go vet` passes. Tests cover registration, coherent capture,
disable/re-enable/remap, reopen, exact historical evidence, organization and
duplicate-mapping isolation, stale/noncanonical/exhausted versions, concurrent
CAS, audit rollback, cancellation and unchanged proposal evidence after edits.
Simulator is unavailable in the lifecycle fixture; no external directory or
live BAS is used. Configured `make test` passes 11 operator-stack checks and
retains the same 13 unrelated dashboard command failures. No dependencies,
Rust code, keys, deployments or HTTP handlers changed. S05/M1 remain open.

### S05 transaction-guarded authority review integration (2026-10-24)

`SubmitSubjectLinkWithAuthorityOnce` and
`ApproveSubjectLinkWithAuthorityOnce` now validate the current enabled BAS
authority and exact root version **inside the review's SQLite transaction**.
Approval uses the persisted immutable proposal's organization/root, not a
caller-provided replacement. This closes the point-in-time authority-read gap
for these opt-in internal methods. It does not close the separate source
registry, authorization or publication gaps.

The original unkeyed/keyed APIs retain their prior shape-validation behavior.
The guarded methods require a valid retry key and use distinct operation
fingerprints in the same principal/organization/key namespace. Switching
between guarded and legacy methods with an existing key conflicts in both
directions: an old unchecked receipt cannot be relabeled as authority-checked.
Review records themselves retain their existing evidence-only schema/state;
they are not a publication certificate or durable attestation of capture mode.

After caller authorization, receipt lookup still precedes current registration
validation. An identical guarded replay returns its original response after
disable/remap without renewing evidence or approving a new mutation. New keys
must pass the current authority check. Missing authorities return not found;
disabled, obsolete or cross-version roots conflict. Denied new mutations write
no review event or retry receipt. Independent-review and audit/receipt rollback
remain enforced. Neither method authenticates the caller or verified device.

Authority edits and guarded review mutations use the same store transaction/
single-writer boundary. A decision ordered before disable can commit review
evidence; one ordered after disable fails. Existing reviews are not rewritten
or revoked by registration edits. Active-binding publication must independently
check the current authority again; `review-approved` never grants lookup access.

Tests cover missing/disabled/stale/foreign authorities, invalid keys, legacy/
guarded receipt isolation, historical replay after disable, concurrent identical
decisions, authority-edit/decision ordering, self-review denial and receipt-write
rollback. The existing signer/verifier/SQLite composition fixture now runs
both legacy and guarded paths through reopen, lost replies, fresh-proof retries,
grant denial and audit failure with Simulator unavailable.

Validation: affected four package race suites, focused dashboard command
compatibility, changereview vet and formatting pass. Guarded/composition tests
also pass ten repeated runs. Configured `make test` passes 11 operator-stack
checks and retains the same 13 unrelated dashboard command failures.

No HTTP endpoint, key provisioning, deployment, dependency or Rust change.
The production source configuration still lacks an approved immutable-selector
capability; reading its file revision is not atomic source validation.
Combined authoritative source capture, its retry/commit boundary and explicit
Control-Service composition remain next. Do not wire these methods as full
binding approval or declare M1/S05 complete.

### S05 explicit LDAP selector declaration (2026-10-24)

The operator approved a declaration-only increment after reviewing its limits.
Production source configuration now accepts optional
`"subject_selector": "ldap_entry_uuid"` for network LDAPS sources only.
Absent/empty means undeclared; unknown selectors, container LDAP and HTTPS
sources reject nonempty declarations. Runtime construction and policy registry
reads validate the declaration; full endpoint/credential checks remain in the
existing source builder. This is not directory capability verification.

The declaration participates in the existing source identity digest and thus
server-owned source-version changes. Adding or removing it advances the version;
display-only edits preserve it. `omitempty` preserves legacy serialized
configuration/digests when absent. Registry reads and restart retain it.
The registration editor exposes an optional LDAP-only select with an explicit
warning: connection tests do not verify it or enable links. Ordinary edits
preserve the declaration; selecting None removes it, and switching to HTTPS
does not send the LDAP declaration.

Tests cover invalid kinds/selectors, legacy digest compatibility, declaration/
removal version changes, stale registry conflicts, persistence/restart,
display edits, and no extra LDAP calls on registration edits. The local LDAP
connection fixture succeeds without supplying UUID semantics, intentionally
showing that connection success is not certification. Both desktop/tablet
browser fixtures exercise save, reload, edit preservation and removal.

Validation: focused source registry, network LDAP and policy-binding race tests
pass; all 12 registration browser cases pass. The browser run initially exposed
and then verified the fix for optional-select rendering in the new control.
Configured `make test` passes 11 operator-stack checks and retains the same 13
unrelated dashboard command failures. No dependencies or Rust code changed.

This supersedes the missing declaration limitation, not the missing immutable
lookup/source-validation contract. No UUID query, provider certification,
source/authority combined review capture, HTTP review endpoint or VS lookup/
revocation behavior is implemented. Existing name-based directory browsing
remains unchanged. S05/M1 remain open.

### S05 exact LDAP subject lookup and source-guarded review (2026-10-24)

The operator approved autonomous fixture-based lookup/review work and dynamic
binding groundwork, stopping at unresolved production decisions. This packet
implements exact LDAP lookup and an internal source/authority-guarded review
composition. No review HTTP endpoint, deployed service, real directory, key
provisioning, shared schema or VS lookup behavior changed.

#### Exact subject lookup

`lookupAssertionLDAPSubject` requires the explicit network-LDAPS-only selector
declaration, exact organization scope and canonical lowercase, nonzero UUID.
It sends only the escaped equality filter `(entryUUID=<uuid>)`, subtree scope
under the configured base, no alias dereferencing, a two-entry maximum
(to detect ambiguity) and the existing eight-second server search bound.
Exactly one entry must return exactly one matching UUID value. Missing,
different, uppercase/noncanonical or multi-valued evidence and duplicate
case-insensitive attribute names fail. Referrals, size-limit/partial results,
out-of-base DNs and canceled searches fail without name fallback.

The implementation reuses the existing verified LDAPS connection/search helper:
hostname/CA validation, protected credential reread, ten-second overall bound,
socket interruption on cancellation and four-MiB aggregate response limit.
The directory browsing reader retains its existing filter, 2,000-entry limit
and parser. No LDAP writes, group expansion or mutable subject identifiers are
introduced.

Requested enrichment uses exact lowercase approved LDAP attribute names for
this initial internal path. Unapproved/sensitive/duplicate names are rejected;
unrequested returned fields are discarded. UUID is requested as structural
evidence and returned only if separately approved/requested. Missing optional
attributes are not synthesized, and a found UUID is not proof of directory ACL
completeness or future entry existence. No implicit `user.*` policy mapping is
invented; policy-key mapping and group semantics remain a separate contract.

#### Review composition and commit boundary

New store methods `SubmitSubjectLinkWithSourceOnce` and
`ApproveSubjectLinkWithSourceOnce` require a source-check callback and the
existing transaction-scoped authority check. The callback receives copied
candidate data; approval uses the immutable persisted proposal. It runs only
for a new mutation, after receipt lookup, shape/state checks, authority capture
and (for approval) independent-review checks. Source failure consumes no review
receipt, writes no decision and rolls back the transaction. Validity is checked
again after the potentially slow provider read.

Source-checked requests have distinct retry fingerprints from legacy and
authority-only methods, so weaker receipts cannot bypass new checks.
Identical source-checked replays return original historical responses, without
fresh LDAP/file validation, validity renewal or activation. Authorization still
must occur before every attempt. The callback may not call the same Store:
its single connection is held by the review transaction.

Internal `assertionRuntime` composition methods hold the runtime mutex across
the source-checked store call and commit. For new mutations their callback
requires the exact active organization, source registry revision, source ID,
positive source version, selector and approved attributes, then performs the
UUID lookup. It reads the protected registry file before and after the lookup
and requires its bytes' revision to match the captured runtime revision.
This fences source edits made through this runtime's registration API until
review commit, while authority edits are serialized by the SQLite store.

**Boundary limitation:** this is a single-process/single-writer composition,
not an atomic transaction across arbitrary file writers and SQLite. External
edits observed before/after lookup conflict, but a writer can still race the
last file read and database commit or replace/restore bytes between checks.
Out-of-band writers/multiple runtime instances must not be permitted to treat
this as a publication guard. A source-versioned publication transaction and
explicit production ownership are still required before active bindings.

The initial review check deliberately holds a mutex and SQLite transaction
during the bounded LDAP read, blocking other review/registry operations. This
simple serialization is suitable for the current internal single-writer
groundwork, not an unthrottled production HTTP performance/availability claim.
No configured server currently invokes the composition methods. Dedicated
proof/session authorization, admission/rate limits and service configuration
remain prerequisites before exposing review routes.

#### Validation and evidence

Local TLS LDAP fixtures verify the exact filter, requested fields and search
bounds, and reject any write operation. Cases cover successful lookup despite
a mutable/renamed DN, zero/multiple entries, missing/wrong/multivalued/duplicate/
noncanonical UUID evidence, referrals, partial results, out-of-scope entries,
bad credentials, untrusted CA, invalid/filter-injection IDs, unapproved
attributes, wrong organization, absent declaration and mid-search cancellation.
Unrequested password/UUID fields do not escape the lookup result.

Composition tests cover successful source-checked proposal/approval, missing
subject and recovery using the same retry key, database reopen, historical
replay with unavailable LDAP and missing source file, changed retry content,
stale registry/source/authority versions, unapproved mappings, and foreign/
missing sources. Deterministic search gates show the managed-edit mutex is held,
a subsequent managed edit stales the next request, and an external file edit
during lookup rejects the current mutation. Callback tests verify invocation
order, copy isolation, failed-check rollback and receipt-mode isolation.
Simulator inputs are deliberately unavailable; no external directory is used.

Affected internal package race suites and focused dashboard source/LDAP/review
tests pass. Exact-lookup/composition tests pass ten repeated race runs;
guarded authority/source/proof tests pass five repeated runs. Vet passes for
the dashboard command and review package. Configured `make test` passes 11
operator-stack checks and retains the same 13 unrelated dashboard failures.
No Rust code/dependencies changed; no Rust validation is claimed in this packet.

#### Dynamic trusted-source binding: inspected boundary and stop point

Inspection confirms VS already rebuilds/publishes trusted-service stores on
policy changes (`PolicyMgr::build_state`/`publish`), preserving unchanged
declarations. The factory supports policy-declared `file` and `rest/1` stores;
its HTTP transport configuration is constructor-owned. `/admin/trusted-services`
exposes lookup health, not an authenticated operator-registry installation
receipt containing source version/selector/registry sequence.

The operator LDAPS registry therefore cannot simply be pushed into that
manager. The missing production contract must define:

1. How operator source IDs/versions and exact attribute mappings bind to
   policy-declared VS service identities, without silently rewriting policy
   or treating a saved directory registration as runtime authorization.
2. Whether delivery uses the existing reviewed policy installation path plus
   an explicitly configured provider, or a separately approved signed
   configuration contract. No parallel unsigned write API should be added.
3. Where LDAPS credentials/CA/provider configuration are provisioned, and how
   versioned changes are activated without assuming constructor-owned VS
   transport settings hot-reload. Secrets must not appear in subject snapshots.
4. Installed-version/recipient readback, ordering/restart protection, failure
   reconciliation and disable/removal invalidation for claims and cached visas.

Those decisions are not resolved by the existing review store or this UUID
reader. Dynamic VS binding implementation is deliberately stopped here, rather
than installing a second source authority or claiming source registration has
activated VS. Recommended next review is to select the production delivery/
policy-binding contract, then implement configured review HTTP composition and
versioned runtime activation/readback together with its negative lifecycle
tests. M1/S05 remain open.

### Policy-declared sources and durable default-on controls (2026-10-24)

**Ownership decision:** policy declares/provides the source; an operator does
not create a second authorization authority by registering a directory.
The effective rule is **usable = declared by effective policy AND enabled**.
A stored toggle, default-on when no override exists, replaces deletion/retirement
as the basic operational control. This supersedes the operator-registration
ownership assumption in the preceding dynamic-binding stop point, but does not
resolve credential provisioning, LDAP provider delivery or subject links.

This increment uses the existing policy trusted-service records (`file`,
`rest/1`) and their mappings/identity contracts. No new shared policy schema or
final envelope syntax is introduced. `PolicyMgr` remains the only publisher of
the effective source set. Runtime snapshots preserve independent disabled-ID
overrides across policy publication, changed declarations, removal and re-add.
Sources absent from effective policy are not queried. Overrides remain stored
when the source is absent, so removing/reintroducing a declaration cannot reset
an operator's off decision.

The ID scope is the current VS deployment/state database, not a new global
cross-tenant namespace. VS still consumes one effective policy. Sharing across
multiple input policy documents, identical-ID deduplication and conflicting-ID
rejection at composition remain unimplemented here; this packet must not be
used as evidence that a multi-document merger exists.

#### Provisional production API

`GET`/`PUT /admin/trusted-services/{id}/control` use existing VS admin API keys,
not delegated human-review proofs. GET requires read permission; PUT requires
read/write permission and accepts exactly:

```json
{"enabled": false}
```

Use `true` to re-enable. Missing/non-boolean/duplicate fields, unknown fields and
invalid source IDs fail. Unknown IDs with no declaration or stored override
return 404. Retained overrides remain readable/editable after policy removal.
GET and successful PUT return:

```json
{
  "enabled": false,
  "revision": "dfb250ab-dfe9-4a1b-adfe-88341f00f734",
  "pending": true,
  "declared": true,
  "effective_enabled": false
}
```

`enabled` is desired state. `effective_enabled` also requires a current
declaration and an open local gate, and can be false during a failed re-enable.
`revision` is an opaque acceptance identifier, not a monotonic policy/source
version, approval receipt or credential revision. No override reports enabled,
not pending, with `revision: ""`. Concurrent writes serialize within the VS;
this is a simple last-accepted operational toggle, not a CAS review workflow.
The declared-source inventory `GET /admin/trusted-services` now also reports
effective `enabled` and `health: "disabled"` for gated sources.

PUT returns 202 only after persistence and local application. State and pending
work are one serialized JSON value in one Redis hash field; there is no crash
gap between recording desired state and recording reconciliation work. The
event queue is only a hint: the existing five-second worker also scans the
durable controls. API mutation and completion share a manager-owned async lock,
under the existing exclusive VS database lease, so an older completion cannot
erase a newer in-process request. Failures are logged and other sources'
pending controls still get a processing attempt. Direct out-of-band database
writers are not a supported control interface.

Read/write/reload failures return 503, not success-shaped acknowledgments.
Before attempting a write the local source is gated off, including on enable;
ambiguous storage errors therefore cannot leave a new enable usable. A failed
or interrupted enable can leave desired-on/effective-off/pending state; the
worker retries it without another HTTP request. A write that did not persist
may leave a conservative local off gate until an explicit retry or restart
restores the last durable decision. Startup reads and validates all stored
controls before constructing policy stores or admitting traffic; unavailable,
wrong-type, malformed or invalid-ID/revision state fails startup, rather than
silently defaulting on. Database durability still depends on operator-configured
Redis persistence. Explicit database clearing destroys these controls too.

#### Enforcement and limits

Off retains the declared store and its place in source qualification. Queries
return a source-labelled indeterminate failure, not an empty successful result,
so neither absence/inequality conditions nor single-source unqualified fallback
can turn an off source into permission. Disabled sources are always revision
stale. Toggle publication invalidates configuration pins, fencing concurrent
actor refresh and visa issuance against old lookup replies.
Visa storage now keeps the exact configuration pinned before evaluation instead
of resampling at the allow-to-storage handoff. Even a rapid off/on cycle cannot
rebase an old allow decision onto the newly enabled generation.

Re-enable keeps the gate closed through the store's explicit flush: file
snapshots reload and REST stores invalidate their cached observations before
lookups resume. A failed reload keeps the gate off. Subsequent requests must
refresh stale actor observations; neither enabling nor completing reconciliation
revives an old queued-revoke visa or renews its expiry.

Reconciliation uses the existing whole-actor fail-closed refresh and visa sweep.
New requests deny when source evidence is indeterminate; existing visas queue
revocation. This is not selective S06 dependency-aware outage handling.
An off source can leave unresolved live actors and `pending:true` even after
the revoke has been queued. Neither 202 nor `pending:false` means gateways have
acknowledged removal; delivery/inventory remains a separate observation.

Store construction/validation remains policy-owned and eager. Off prevents
lookups, not declaration validation: missing/invalid file snapshots or absent
REST transport configuration can still block policy install/startup. Credentials,
CA and transports remain independently provisioned, and constructor-owned REST
settings are not made hot-reloadable by this toggle.

No Control Room UI/proxy, live directory contact, LDAP provider installation,
device-to-person link activation, deployment, new keys or Simulator dependency
is added. Simulator may eventually exercise this same authorized production
API, but cannot own its state. S05/S06 and end-to-end M1 remain open.

#### Validation

Focused tests cover default-on, strict JSON/auth/readback, malformed/wrong-type
storage, default-on suppression on restore failure, persisted off across actual
policy installation/replacement/removal/re-addition and reconstructed policy
manager startup, namespace retention, both lookup entry points, failed/repaired
file re-enable, durable pending consumption without an event, interrupted
enable recovery, and independent processing after another source's reload fails.
Visa-path tests prove disable prevents publication from an in-flight provider
reply, denies a previously allowed cached request and queues revocation of the
original visa. Re-enable does not cancel its queued revoke. A shared persistence
contract runs against FakeDb and an explicitly opt-in private Redis server.

Final validation for this increment: `cargo test --workspace` passes **993**
tests, with **22** explicitly ignored; the opt-in private Redis suite separately
passes **19** tests. `cargo fmt --all -- --check` and `git diff --check` pass.
Strict workspace Clippy is blocked by an existing redundant import in
`admin-api-types`; a targeted strict VS run is also blocked by existing
`libeval` warnings. Non-strict VS/all-targets Clippy completes; no warnings
identify the new control module or the changed generation handoff. Those
unrelated lint findings were not edited. No Go/dependency changes were made,
and the temporary worktree-only Cargo path adjustment was restored.

### S05 foundation increment: preserved device authentication (2026-10-24)

The operator separately approved the verified adapter/device identity foundation
only. This implements internal authentication bookkeeping, **not** the binding
contract above or the deferred principal-envelope protocol:

- `Actor` carries typed `DeviceAuthentication` metadata outside its attribute
  map. It records authority kind/ID, verified device subject, a fresh opaque
  authentication session ID and an expiry bound. It is omitted from actor JSON
  exports, policy identity keys and policy hashing. Cloning an actor retains it.
  This is an internal evaluator structure addition, not a shared ZPR wire-schema
  change or a new policy attribute.
- The BAS path constructs the record after token verification from the verified
  issuer and subject, the policy-authorized authentication service ID, and the
  existing token/policy/default expiry limit. Mapped user claims do not select
  the subject. RSA bootstrap records the signature-verified CN and VS authority,
  with an expiry no later than its server-issued device-authority claim. VS
  self-authentication has a distinct local-authority kind. None produces an
  authenticated human subject.
- The actor repository stores the optional record in the actor metadata hash.
  Initial attribute/identity-metadata writes share a transaction; update writes
  include the record in the existing attribute/identity/service-set batch. Full
  actor loads use the existing two-hash snapshot, so captured attributes and
  authentication metadata belong to the same snapshot. Source refresh/pruning
  retains the record without deriving it from provider values.
- Legacy or deliberately unannotated actors load with no record; neither CN,
  authority claims nor similarly named enrichment fields synthesize one.
  Malformed or incomplete present metadata returns an explicit repository error.
  Replacement/removal clears the old session with the actor metadata hash.

Limits remain important: authority labels are not immutable trust-registration
versions, and there is no organization binding or immutable directory selector.
The session ID is persisted evidence, **not a compare-and-write actor-generation
fence**. A stale actor update can still race reauthentication/address reuse; the
existing service-set guard does not solve that race. Repository transactions
do not provide rollback after Redis runtime errors, nor do they make actor
creation's indexes/timestamps atomic with the snapshot.

The metadata is not yet consumed for lookup authorization, expiry enforcement,
link approval, visa dependencies or revocation. Existing authentication and
policy behavior, including plain-identity provider fan-out, remains in place.
Do not use presence of this record as S05 sign-off or permission to enable
strict subject linking. The reviewed organization/version delivery and
generation/publication/activation enforcement are still implementation gates.

Regression coverage includes verified BAS subject versus mapped user identity,
authority separation and fresh sessions, RSA and local VS paths, claim-only
authorization without a root, full provider refresh/pruning, repository
reconstruction, legacy absence, malformed records, rejected writes, address
replacement/removal and deterministic read/update snapshot interleaving. A
private Redis roundtrip additionally exercises persistence through independent
connections, update/reauthentication and removal; it is not a Redis process
restart or production deployment test.

Validation for this increment: `cargo test` passes **968 tests, 20 ignored**;
the opt-in private Redis operation suite passes **17 tests**, including the new
identity roundtrip. `cargo fmt -- --check` and `cargo check -p vs --bin vs`
pass; the production check retains the pre-existing unused
`DnsPublisher::add_provider` warning. Go/browser fixtures were not rerun for
this Rust-only increment.

### S05 foundation increment: session-guarded actor writes (2026-10-24)

The next separately approved packet protects ordinary actor updates against
replacing a different persisted authentication session. It narrows the stale
actor-write limitation above; it does not implement subject-link authorization.

- An ordinary update compares the actor's complete serialized device
  authentication record with stored metadata. A mismatch returns an explicit
  `StoreError::Conflict`; callers must discard the stale actor and reload, not
  retry that actor automatically. Comparing the whole record also detects
  authority/subject/expiry changes, not only a different session ID.
- Redis Lua and FakeDb compare the existing metadata hash/expected field value
  and the previously read service set before applying any update operations,
  under the same atomic boundary. A changed/deleted record between preflight
  and application is a conflict with no writes. A missing hash is not an empty
  authenticated session.
- Verified node reconnect uses an explicit authenticated-replacement method,
  which requires device authentication and compares the old persisted record
  captured at the start of its update. A concurrent replacement after that
  capture conflicts. Fresh actor creation remains the existing replacement
  path for adapters. Ordinary attribute refresh cannot introduce, remove or
  replace device authentication.
- Legacy actors without a record remain refreshable when the stored field is
  missing or JSON `null`; the atomic comparison pins whichever representation
  was read. They cannot overwrite an annotated actor. A reconnect without
  device metadata remains an ordinary update and cannot strip an existing root.
- Update time is now part of the guarded batch, with no post-reply metadata
  writes. Creation time is left unchanged. Deletion while an applied update's
  reply is delayed cannot be undone by a later timestamp write.

This is a bounded authentication-session write fence, **not** complete actor
generation fencing. Replacement between two unannotated actors, same-session
attribute-only races and same-set service ABA remain outside this packet.
Deletion/creation themselves are not generation-guarded operations. A later
mutation can still occur after a successful update; returned snapshots do not
guarantee continuing liveness. Reauthentication is ordered at its persisted
record capture, not at the earlier authentication/admission boundary.
Policy/source/link publication, visa activation, installed grants, revision
caches and durable revocation still need their separate generation/version
contracts. Redis runtime errors still do not roll back earlier script commands;
transport/cancellation can still hide an applied batch.

Acceptance coverage includes same-subject/same-services session replacement
across repository reconstruction, a session change after preflight, deletion
before apply, deletion during a delayed reply, legacy absence/null compatibility,
explicit node reconnect and conflicting concurrent authentication replacement.
The refresh-path regression proves a conflict does not certify new source
revisions or persist refreshed claims. FakeDb and private Redis run the same
guard contract for absent hashes/fields, exact/binary-safe values, service-set
conflicts and wrong types. The independent-connection Redis identity roundtrip
also rejects the prior session after reauthentication.

Validation: `cargo test` passes **978 tests, 21 ignored**. The private Redis
operation suite, run explicitly with `--ignored`, passes **18 tests**.
`cargo fmt -- --check`, `cargo check -p vs --bin vs` and `git diff --check`
pass; the production check retains only the pre-existing unused
`DnsPublisher::add_provider` warning.
These are local repository/protocol regressions, not process-restart or
integrated deployment acceptance. S05/M1 remain open and provider lookup
behavior is unchanged.

### Maturation increment: source-controlled observation validity (2026-10-24)

The REST lookup response optionally carries `expires_unix` (absolute UTC Unix
seconds). The Go provider emits it only with an explicit `-attribute-validity`
flag, keeping the legacy wire shape unchanged by default. Upgrade consumers
before enabling it; older VS response decoders reject unknown fields.

VS caps attributes at the earlier source deadline/configured TTL and retains
reserved source-qualified freshness metadata for the observation, including an
empty result. That metadata participates in actor refresh, new-visa expiration,
and existing-visa rechecks. An absence-based grant cannot outlive an observation
that justifies it. A source-bound reduction queues revocation if an existing
visa would outlive the new bound; unrelated updates do not renew another
source's deadline. Invalid/expired supplied deadlines fail explicitly.

This increment does not implement policy-associated access credentials,
source-side per-principal visibility, explicit subject links, selective
dependency tracking or push-driven revocation guarantees. Legacy responses
without the deadline retain existing TTL behavior, including the legacy
empty-result freshness limitation. Clocks must be synchronized. The provider's
bound is lookup observation validity, not proof of an underlying LDAP snapshot
or its entry/authorization expiration.

Validation: `cargo test --workspace` passes (701 passed, 9 ignored), including
verified-HTTPS deadline parsing, empty-observation refresh, exact issuance
bounds/minimum lifetime at both endpoints, and shorter-versus-longer source
validity during reconciliation. Provider and change-watcher `go test -race`
pass, including legacy response shape and expired-lookup failure checks.
Rust formatting, changed Go formatting and diff checks pass. The worktree Cargo
path adjustment was restored after validation. No deployment, shared-schema
change, production setting change or dual-principal authorization is claimed.

### Future requirement: policy-associated trusted-source access identity

**Deferred envelope work, not part of the current maturation scope.** The
envelope represents constraints and permissions assigned to a service/DNS
namespace. Its later design must address where the access identity originates,
namespace/delegation semantics, and consistent editor and trusted-source-viewer
behavior before enabling policy-associated credentials. Do not introduce an
interim policy-identity protocol or broaden maturation into this work.

#### Realm authority and parent-envelope composition

Design requirements agreed on 2026-10-24; these do not introduce runtime
behavior or settle the envelope wire format.

Realm administration and namespace allocation follow explicit authority grants:
creation, administration/editing, envelope issuance, delegation, transfer,
revocation and recovery are distinct rights. Each grant must trace to an
explicitly accepted trust anchor and remain within the grantor's delegable
scope. Identity authentication or DNS control alone does not imply all realm
administration rights. Parent recovery or takeover powers must be explicit,
not inferred from namespace ancestry.

An envelope's canonical ID binds its immutable authority context and cannot be
overridden by policy, aliases or inherited content. Changes require a new
immutable envelope identity; expiration, revocation and explicitly authorized
succession remain separate lifecycle concerns. Existing trusted-source
identities cannot be shadowed or rebound by an envelope or policy.

An envelope may explicitly reuse assertions and defines, and potentially
trusted-source bindings, from one or more parent envelopes. Reusing content
does not inherit or replace the parents' IDs, principals or credentials: the
child retains its own canonical identity and authority constraints. Parent
references must bind exact immutable identities/revisions, not floating aliases.
The child's identity-bound scope remains an enforced ceiling; possessing an ID
or importing content is not itself an authorization grant.

Composition must not union parent permissions into a broader grant or drop
applicable inherited restrictions. Defines remain authoring conveniences;
imported assertions retain their declared diagnostic or enforcement role;
source reuse cannot expand source-authorized operations or field visibility.
Specify explicit import selection, provenance, conflict handling, cycle
rejection and revocation effects before enabling composition. Ambiguous
definitions or incompatible constraints must not be resolved by import order,
silent override or a more permissive fallback. Whether trusted-source bindings
are inheritable, and the exact multi-parent composition rules, remain design
decisions.

Policy authoring `define` names are conveniences, not security identities.
Compilation must resolve them to canonical references carrying the relevant
source and namespace identity. Runtime enforcement must not depend on display
names or alias spelling.

Trusted-source communication must eventually carry an authenticated access
identity associated with the policy or delegated policy portion. That identity
may represent a user or another principal. It is separate from the subject
whose attributes are being requested and from the transport identity of the
Visa Service. Do not infer it from a `define`, a subject name, or an unsigned
request parameter.

Delegating a policy or namespace portion must preserve the trusted-source
information boundary: a delegate cannot read or operate on information beyond
the access identity's approved source, namespace and operation scope. Local
policy approval alone must not widen permissions at the source. Bind the
canonical policy/delegation scope to the authenticated access identity and
enforce the source's authorization for each operation. Define attenuation,
expiration and revocation before enabling further delegation.

The trusted source owns authorization of its information, including field
visibility. VS is not the authority that grants visibility into source fields.
Requests carry two independently verifiable authorization contexts: the calling
Visa Service's authenticated service identity and the policy-associated
principal's credential or authenticated delegation proof. A policy ID is a
reference, not a credential by itself. The trusted source must validate that the
VS may act for that principal in the specified canonical policy/namespace scope,
and authorize the requested operation and returned fields under that combined
context. Neither identity alone grants the other's permissions.

VS may request a narrower field set, validate responses and enforce local
data-minimization restrictions, but these cannot expand source-granted access or
replace source-side authorization. Do not implement a privileged broad fetch
followed only by VS-side field filtering as delegated access enforcement.
The exact credential/delegation protocol remains a design decision; this does
not require forwarding a user's reusable secret to VS or to another service.

Future contract work must specify credential issuance and verification, how
these bindings are authenticated with the policy, and how policy/source/version
or delegation changes invalidate them. Cache keys and audit provenance must
include the relevant canonical authorization context so data obtained under a
broader principal cannot leak through reuse by a narrower delegate. Tests must
exercise allowed access, cross-namespace and cross-principal rejection,
revocation, field visibility enforced at the source, and attempted privilege
expansion through aliases or delegation. Include missing/invalid credentials for
either identity and a valid VS credential paired with an unauthorized policy
principal, as well as a valid principal proof presented by an unauthorized VS.

This is a future architecture requirement, not implemented delegated-source
authorization or permission to replace the existing access contracts.

### Future requirement: update, expiration and authorization dependencies

Trusted-source data, source-side visibility, delegated access and issued visas
have overlapping many-to-many lifecycles:

- One source record or field change may affect multiple policy principals,
  actors, cached views, policy evaluations and visas, with different visibility
  under each authorization context.
- One policy decision or visa may depend on multiple records, sources,
  identity bindings and delegated credentials. Multiple policies may consume
  the same authorized view without acquiring each other's permissions.
- Data expiration, identity/credential expiration, delegation revocation,
  source-visibility changes and update-stream freshness are distinct events.
  An unchanged data value does not preserve authority after access is revoked.

Define canonical dependency/provenance records and reverse invalidation indexes
before claiming selective updates. Track source and record identity/version,
the authorized field projection, policy/delegation authorization context and
the consuming decisions/grants. Include dependencies on absence and negative
conditions, not just fields returned or positive allow matches.

An update event is an invalidation hint, not authorization to disclose its
payload or fetch the changed data. Authorize subscriptions and delivered fields
at the source under the combined VS/principal context. Re-fetch and revalidate
under the affected context when necessary. Do not broadcast a privileged change
payload across all subscribers or let subscription existence extend expired
access. A visibility change must invalidate affected views even if data itself
did not change.

Specify separate freshness/validity bounds for source data, authorization,
identity links, stream continuity and policy artifacts; a decision cannot
outlive any dependency required to justify it. An unrelated update must not
renew other dependencies. Where sources cannot prove a coherent snapshot,
define conservative handling of mixed revisions instead of assuming atomicity
across sources.

Subscriptions may be multiplexed for efficiency only if per-context
authorization and delivery isolation remain intact. Handle duplicate and
out-of-order events, gaps, reconnect/resync, concurrent access revocation and
durable restart recovery. A gap or outage is unknown state, not evidence of
absence. Define when to deny new grants and when/how to revalidate or revoke
existing grants, including bounded relay-delivery acceptance.

Acceptance must cover one change affecting multiple principals/visas, one
decision depending on multiple sources, differing visible fields for the same
record, access revocation without data changes, expiration during an active
subscription, negative dependencies, unrelated-update TTL isolation, and
gap/restart recovery. Retain conservative whole-actor/global reconciliation
until this dependency-aware path is implemented and verified.

### Maturation increment: expired lookup identities (2026-10-24)

The common lookup-identity collector now rejects expired single-valued identities
as `AttributesIndeterminate`. It neither forwards them to enrichment providers
nor silently omits them: omission could turn unknown claims into absence-based
permission. Admission propagates that failure; actor refresh reports it before
provider calls, so visa requests deny and rechecks queue revocation. Cached
identity data is not renewed or removed by this guard; a newly authenticated
identity is required to resume lookups.

Tests cover mixed live/expired identities, non-identity and multi-valued claims,
two real file stores, request denial/recovery at either endpoint, and
reconciliation under an absence-based allow with the provider still available.
This prevents expired lookup identities being used to fetch fresh enrichment;
it does not establish issuer-qualified subject links. Matching live subject
strings still fan out through the existing lookup model and remain an explicit
production limitation.

Validation for the expired-identity increment: three new lookup/refresh tests
failed before the guard and pass afterward. Added admission, request recovery,
and expanded reconciliation coverage also passes. `cargo test --workspace`
passes (694 passed, 9 ignored), as do formatting and diff checks. The existing
worktree dependency-path workaround was temporary and restored; no dependency,
shared-schema, deployment or trust-store changes are included.

Do not infer an approved link from matching usernames, DNs, mail, or attribute
values. The minimal next configuration must identify organization, both immutable
source IDs/versions, issuer-qualified subject IDs, approval identity/time and
revocation state. VS lookup must start with the authenticated issuer/subject and
fan out **only** through approved active links, invalidating dependent visas when
links are revoked or sources change. A config file alone without that lookup and
visa enforcement would provide false assurance, so none was added. Existing
explicit cross-source assertions remain available and are not permission links.

Authentication `validation/2` claim mapping remains separate from manager-backed
file/REST enrichment; this new end-to-end proof covers the latter, not a claim
that every authentication ingestion path has source-qualified aliases. Full
issuer-qualified auth claim/identity handling belongs with the link contract.

## Validation and operator UI evidence

- Compiler: `cargo test` passes (296 unit + 14 integration tests); source-built
  binary used for both VS evaluation and signed Policy-Service staging tests.
- VS: `cargo test --workspace` passes (687 passed, 9 ignored); optional source-built interoperability
  test explicitly executed with `ZPR_TEST_ZPLC` and `--ignored` and passes.
- Focused Go `-race` tests cover registry defaults, durable identity versions,
  staging binding/default changes, actual signed compiler staging, existing
  assertions and colliding catalogs; pass with Simulator deliberately unavailable.
- Browser: source-registration and multi-source catalog specs pass on desktop and
  tablet (12 tests), including persisted default replacement/clearing and
  `explicit-policy-default.png` screenshots. Browser APIs are fixtures.
- Strict Control Room boundary audit: **0 findings**; assets: 5 tests pass.
- Both Cargo formatting checks, changed Go `gofmt`, and both diff checks pass.
- Optional compiler `cargo clippy --all-targets -- -D warnings` fails existing
  style-lint baselines (99 library / 136 library-test diagnostics); unrelated
  lint cleanup was not mixed into this feature.
- Broader `make test` was executed and fails in existing fixture/environment
  areas (organization seeds/LDIF/policy-demo assets, isolated launcher, protected
  reset ancestry, Rust sibling dependency path, route contract baseline and macOS
  enrollment IPC). The final native-compiler-enabled run also exposes absent
  organization `.zplc` fixtures and old unqualified multi-source example policies;
  those samples need an explicit default or qualified references before use.
  No unrelated tests were patched or hidden.
- Worktree Cargo dependency path temporarily points at the existing
  `../../zpr-common` only during Rust commands, then is restored. No dependency
  manifest/schema changes are committed. Test scratch stays inside this worktree.

Reproduce native compiler-to-VS proof from this worktree after resolving the local
Cargo sibling path:

```sh
ZPR_TEST_ZPLC=/absolute/path/to/source-built/zplc cargo test -p vs \
  test_source_built_compiler_to_stores_to_real_evaluation -- --ignored
```

`SIMULATOR_AS_ZPR_CLIENT_PLAN.md` tracks V16/V17 visual gates and X13/X14
pre-production limitations. Defaults/staging verification does not close the live
visa, selective-outage or approved-link production gates.

## Exclusive-worktree compiler reference

Compiler result: `maturing/source-qualified-attrs` commit `0834bc1`.
Only the two assigned worktrees were changed; no push/deployment or shared-schema
mutation was performed.
