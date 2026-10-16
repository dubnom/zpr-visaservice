# ZPR Dashboard TUI Application

This repository contains the Zero-Trust Packet Routing (ZPR) Terminal User Interface application. The dashboard application allows you to manage your ZPR infrastructure quickly through an easy-to-understand and easy-to-use interface.

Production appliance work is tracked in the
[system packaging foundation](packaging/system/README.md). The new
`zpr-system-config` command validates only company management-bootstrap input;
`-check-files` opts into read-only local TLS/OIDC credential-file checks.
It is not an installer or runtime activation command.

The separately planned Simulator/LDAP/OIDC/DNS companion now has an initial
[`zpr-lab-bootstrap` trust-generation tool](packaging/simulator/README.md).
It creates protected per-install lab TLS identities and service-secret files;
optional explicit network inputs also generate separate bootstrap/ZPR BIND
configuration, zones and TSIG credentials. Existing network DNS is the default:
underlay addresses generate record requests, while a bootstrap BIND server
requires explicit `-host-bootstrap-dns`; protected ZPR DNS is independent.
It does not apply configuration or
start services, and no companion bundle is ready.
Optional `-identity-services` now also renders protected OpenLDAP/Dex config,
per-install lab users/groups, separate safe initialization/startup scripts and
an integration inventory. Real LDAP-backed OIDC/PKCE was verified in isolated
engineering containers; automatic named operator grants and the complete
installer remain unfinished.
Optional `-identity-compose` also generates a separate pinned-image LDAP/Dex
stack with private-IP host bindings, one-time initialization and independent
persistent state. Generated-stack LDAPS health, live OIDC login and preserved
identity after container recreation passed arm64 engineering tests without
Simulator. See the [same-host startup instructions](packaging/simulator/README.md#same-host-independent-identity-stack);
Ubuntu amd64 release images and complete paired packages remain unfinished.

# Development

To setup your development environment, run `go mod download` to install the local dependencies. Finally, run `make run` to start the TUI.

## Guarded policy composition (contract-verified; disabled by default)

Change Review captures an optional immutable staged candidate alongside the saved
source revision. Candidate-bearing reviews bind the source/config hashes and exact
compiled-container SHA-256; draft-only reviews remain compatible but cannot
authorize application. The producer derives the named reviewer from the stored
approval event, rechecks source/stage identity, and enforces the current self-review
policy (distinct reviewer by default).

The policy-specific Ed25519 contract in
[`internal/operatordelegation`](internal/operatordelegation/delegation.go) is
separate from enrollment delegation. An explicitly trusted Control Room signer
creates two standard-base64 `{payload, signature}` proofs:

- Policy-Service audience, purpose `policy-service-policy-operation`, binding
  the exact POST URI and SHA-256 of the complete command bytes.
- Distinct Visa Service audience, purpose `visa-service-policy-operation`,
  binding POST `/admin/policy-operations` and the exact operation. This proof,
  not the Policy-Service proof, is included in the VS command.

Both preserve the same signed reviewer issuer/subject, organization, action,
target, operation ID, candidate digest/source revision and expected digest/version.
Proofs last at most 30 seconds when produced here; verifiers reject lifetimes
over 300 seconds. Policy trust requires explicit `audience`,
`visa_service_audience`, `organization`, `target_id`, `keys` (unique `key_id`,
HTTPS signer `issuer`, standard-base64 raw Ed25519 `public_key`, `expires_unix`)
and `grants` (named-user `issuer`, `subject`, exact `organization`, explicit
`actions`: `install`/`rollback`). No wildcard or inherited API-key approval exists.
They are loaded from an explicitly configured protected trust file; no trust or
grants are installed automatically.

[`policy_apply.go`](cmd/zpr-web-dashboard/policy_apply.go) provides a gated,
injected-transport client for the receiver's
`GET /admin/policy-operations/current` and `POST /admin/policy-operations`
contracts. It requires an explicit matching HTTPS origin and separate API key.
Before POST, Policy-Service atomically consumes the PS nonce and records the
immutable semantic command/approver binding in its separate protected SQLite
operation ledger. Control Room calls only Policy-Service over verified mTLS;
Policy-Service alone reads the Visa Service CAS state and dispatches operations
over verified TLS with a separate API credential.
Retries use the same operation and expected state with newly signed
proofs/nonces; reusing an operation ID for changed bindings fails. Success
requires a matching complete receipt/fingerprint and independent authoritative
digest/version readback. Transport loss, malformed receipts and mismatches
persist `uncertain`, not success or implicit rollback. Nonces currently have no
cleanup; operation and approval records must survive reconciliation.

**Apply remains disabled without explicit protected configuration.** Policy-Service
loads `ZPR_POLICY_APPLY_CONFIG_FILE` only when supplied and requires
`ZPR_POLICY_ORGANIZATION_ID` to match its trust organization. The strict version-1
JSON config requires `operation_database`, `trust_file`, `visa_service`,
`api_credential_file`, `client_certificate_file`, `client_key_file` and
`server_ca_file`. Invalid supplied configuration fails startup; absent config
leaves `/api/policy/operations` and `/api/policy/operations/candidate` unregistered.

Change Review additionally requires `policy_apply_config_file` in its configuration.
That protected version-1 JSON requires `trust_file`, `signing_key_file`, `key_id`,
`issuer`, `policy_service`, `client_certificate_file`, `client_key_file` and
`server_ca_file`. The signing key must be PKCS#8 Ed25519 matching the explicit
trusted signer. All resource paths must be distinct canonical absolute paths;
protected files must pass the existing ownership, permission and safe-open checks.
No example private key, credential or production grant is supplied.

With the full chain configured, policy application requires explicit org-scoped
`policy.apply`, normal session/origin/CSRF checks and the separately stored
candidate-bound reviewer approval. Neither generic edit permission nor mTLS/API
credentials imply named approval. `GET /api/policy/operations/candidate` returns
the verified staged artifact and authoritative VS current state through PS;
`POST /api/policy/operations` requires the exact PS-audience proof in
`X-ZPR-Policy-Approval` plus the VS command/proof body. Original artifact/CAS/
operation identity is immutable in the review store before dispatch; retries use
fresh nonces and reconcile the durable PS record, including after response loss
or later stage/source replacement. Rollback remains unavailable. This is a
synthetic integration milestone, not deployment or live capability verification.

The isolated real-receiver fixture below now verifies the Go Policy-Service
client against the actual guarded Rust router. Remaining production qualification
includes crash-window durability, deployment topology and operator-provisioned
trust/TLS configuration. No deployment keys, credentials or
target grants are assumed, and no production activation/readiness is claimed.
Existing enrollment routes and delegation remain unchanged.

### Evidence boundaries for the migration integration batch

These checkpoints are distinct; one does not imply the others:

| Evidence | What it establishes | What it does not establish |
| --- | --- | --- |
| Go Control Room/Policy-Service contract fixtures | Approval binding, independent org/action checks, durable operation reconciliation and status projection against synthetic receiver responses | Actual Rust transport interoperability or production readiness |
| Rust guarded router fixtures | Real receiver proof/API authorization, CAS and policy application using test state | Go wire/TLS compatibility or real-store restart durability |
| Executed Go-to-Rust HTTPS fixture | Actual policy client/router interoperability, verified TLS, authorization, apply/readback, replay/CAS and response-suppression reconciliation after PS store reopen | Full production bootstrap, Redis durability, Rust restart, packet loss or live topology |
| Executed persistent recovery HTTPS fixture | Actual RedisDb, receiver SIGKILL/restart, graceful AOF-backed Valkey restart and PS SQLite reopen retain the same operation/receipt/CAS and exact readback without a second install | Database crash/power loss, in-flight commit-window recovery, upstream Redis qualification or production startup |
| Isolated Valkey-compatible Lua tests | Six operation/nonce/CAS cases against a private disposable store | Upstream Redis qualification, restart persistence or nonce cleanup |
| Simulator browser fixtures | 72 verified desktop/tablet status/navigation cases | Live PS/VS, native identity or connected-adapter admission |
| Endpoint producer and public binding journal fixtures | Exact identity/reference resume, exclusive creation reservation and fail-closed disk-state handling | Native Keychain nonexportability, authenticated helper IPC or PH/BAS use |

The bounded source batch implemented a native producer and executed an
opt-in real Go-to-Rust HTTPS fixture. Neither is a live deployment or a grant of
apply authority. Native validation must remain compile/fake-backed only: do not
invoke Keychain tests, install a helper or use host credentials for this batch.
The receiver fixture may use only its own temporary keys, state, loopback
listener and bounded children; it must verify TLS and exercise the actual
receiver rather than a duplicate mock. Implementation and execution results
are separate from the earlier checkpoints.

The bounded source/test batch adds a trusted helper source boundary and
persistent receiver recovery. The recovery fixture below has now executed
independently under the coordinator's race run. The original FakeDb interop
test still does not establish restart durability, and source/fake-backed
verification does not establish an installed protected helper. No PH wire
contract or native runtime authority is implied by this work.

Independent approval/ledger/authority regression coverage can run without
either worker's source or external services:

```sh
GOPROXY=off GOSUMDB=off go test -race -count=1 \
  ./internal/changereview ./internal/operatordelegation ./internal/operatorauth
```

### Opt-in actual Rust guarded receiver interop

From this directory, with the existing Rust/Go dependency caches populated:

```sh
CARGO_NET_OFFLINE=true GOPROXY=off GOSUMDB=off ZPR_RUST_POLICY_INTEROP=1 \
  go test ./cmd/zpr-web-dashboard -run '^TestPolicyApplyRustInterop$' \
  -count=1 -v -timeout=8m
```

The [Go test](cmd/zpr-web-dashboard/policy_apply_rust_interop_test.go) launches
the [Rust in-binary fixture](../vs/src/policy_interop_fixture.rs) with the actual
guarded admin router, coherent FakeDb and valid policy containers. This Unix-only
test owns its prebound loopback listener, temporary CA/server/approval keys,
API keys and SQLite state. TLS verifies the generated CA and IP SAN; it uses
server-authenticated TLS plus API key and signed approval, **not mTLS**.
No environment proxy, insecure verification, production bootstrap, Redis or
Simulator is used.

The opt-in run passed independently in the coordinator and again with the
combined Go race tests. It verifies reviewed application and exact independent
digest/version readback, missing/insufficient API-key rejection, wrong org/action
and bad-proof refusal, exact-proof replay and stale-CAS HTTP 409, and identical
receipt reconciliation with a fresh proof. One completed successful application
reply is deliberately replaced with HTTP 503; reopening the PS store and retrying
with a fresh proof reconciles without a second version increment. This is
**application-response suppression, not physical packet loss**. Rust connection
tasks are tracked/drained; the exact owned child is stopped and reaped and its
private files removed. Compilation and normal skipped runs are not interop proof.

The worker's full `cargo test -p vs` passed 517 tests with 7 ignored; the opt-in
fixture was separately executed, not counted as a normal-suite pass.
The offline full Go suite, run with `CGO_ENABLED=0` to avoid existing native
Keychain tests, retains exactly the five known dashboard failures:
`TestAssistantSkillsEmbeddedAndTaskSpecific`,
`TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator`,
`TestOrganizationPolicyInstallNeverUploadsStaleBundle`,
`TestScenarioPlacementPreflightMatchesLauncher`, and
`TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact`.
No existing failing test was edited. This is not production acceptance.

### Opt-in persistent receiver recovery

With the existing dependency caches and `/opt/homebrew/bin/valkey-server`
already present, run:

```sh
CARGO_NET_OFFLINE=true GOPROXY=off GOSUMDB=off \
  ZPR_RUST_POLICY_INTEROP=1 ZPR_RUST_POLICY_RECOVERY_INTEROP=1 \
  go test -race ./cmd/zpr-web-dashboard \
  -run '^TestPolicyApplyRust(Interop|PersistentRecoveryInterop)$' \
  -count=1 -v -timeout=8m
```

The [recovery test](cmd/zpr-web-dashboard/policy_apply_recovery_interop_test.go)
reuses the actual client/router harness with a
[test-only persistent assembly](../vs/src/policy_recovery_fixture.rs) whose
repositories share the actual `RedisDb`. It owns a private loopback Valkey
process and temporary AOF directory (`appendfsync=always`); it never uses a
configured shared database or installs dependencies.

After a completed receiver application whose reply was suppressed, the test
kills/reaps the actual Rust receiver, gracefully stops/reaps and restarts
Valkey, then starts a new receiver process loading the persisted policy.
It separately closes/reopens PS SQLite, verifies the unchanged uncertain
operation binding, reconciles with fresh proofs to the identical receipt,
and checks exact digest/version readback without a second install. A new stale
CAS request remains HTTP 409 after restart. The coordinator independently
executed both interop modes under the policy regression race suite (7.694s).
Owned processes are stopped/reaped and their exact temporary directory removed.

This proves completed-receipt recovery across **receiver SIGKILL and graceful
database restart**, not database crash/power-loss durability or interruption
between pending/commit/receipt transitions. It is Valkey-compatible evidence,
not upstream Redis or production bootstrap qualification. Nonce records still
have no TTL; separately approved rollback, production trust, and live topology
remain outside this fixture. The transport remains server-authenticated TLS plus
API key and signed reviewer proof, not mTLS.

## Read-only guarded policy operation lookup

With protected PS guarded resources configured, optional `status_readers` entries
in `ZPR_POLICY_APPLY_CONFIG_FILE` enable
`GET /api/policy/operations/status/{organization}/{target_id}/{operation_id}`.
Each entry requires `certificate_sha256` (64 lowercase hex SHA-256 of the client
certificate DER), `organization` and `target_id`, exactly matching PS trust scope.
Pins must be unique. A valid verified mTLS client-auth certificate AND its explicit
scoped pin are required; mTLS alone, caller actor headers and human-supplied scope
are not authorization. This identity needs neither a signing key nor `policy.apply`.
Absent readers leave the endpoint unregistered; invalid readers fail startup.
No reader credentials, trust or runtime configuration are provisioned here.

Segments are literal `[A-Za-z0-9_-]{1,128}`; queries, escapes and bodies are rejected.
The response contains `operation` (existing exact operation ID/target/source
revision/candidate digest/expected digest/version), `organization`,
`approver: {issuer, subject}`, `action`, `durable_status`, `record_version` and
derived `status`. Optional `receipt` is previously recorded operation-specific VS
evidence, with diagnostic `error` redacted to null; `recorded_readback` is historical
verified evidence, while `current_readback` is separately fresh read-only VS state.
No bundles, proofs, credentials or raw errors are exposed.

Exact `succeeded` requires the durable successful operation, its full matching
receipt/fingerprint/applied version, recorded readback and fresh exact state.
`superseded` preserves verified historical success when fresh current version is
higher, including the same digest at a later version. Other mismatches or failed
fresh reads produce `uncertain`; pending/failed/uncertain records are never promoted
by digest equality alone. `durable_status` remains unchanged. Readback errors are
bounded codes `unavailable` or `invalid_evidence`. The lookup never applies, retries,
consumes a nonce or changes the ledger. Concurrent durable snapshot changes return
503 rather than mixed-version evidence.

Known records return 200; unauthorized scope/identity returns 403 before lookup,
unknown authorized operation returns 404, malformed request 400, non-GET 405 and
unavailable/corrupt storage 503. Error JSON has a bounded `status` code; absent
endpoint configuration uses ordinary 404. Handled responses are `no-store`, capped
at 16 KiB with a 10-second context deadline. Existing protected VS TLS/API-key
transport performs only its authoritative current-state GET. This is synthetic
verification, not deployment readiness or live qualification.

## Simulator read-only policy status

The Simulator Organizations page can reconcile a human-supplied organization,
target and operation ID through
`GET /api/simulator/policy-operations/status/{organization}/{target_id}/{operation_id}`.
It requires an authenticated named Simulator operator with `organization.read`
for that organization. The browser never receives Policy-Service credentials.

Configure all four values explicitly before starting Simulator:

- `SIMULATION_POLICY_STATUS_URL`: exact HTTPS Policy-Service origin with no path.
- `SIMULATION_POLICY_STATUS_CLIENT_CERT_FILE`: protected scoped read-identity
  certificate.
- `SIMULATION_POLICY_STATUS_CLIENT_KEY_FILE`: its protected private key.
- `SIMULATION_POLICY_STATUS_SERVER_CA_FILE`: protected CA bundle for the
  Policy-Service TLS endpoint.

All values absent leaves the status client unavailable; a partial, malformed or
unprotected configuration fails closed at startup. Simulator accepts no URL or
credential from the browser, sends only GET requests, follows no redirects and
uses verified TLS with the explicit client certificate and CA. Policy-Service
must independently pin that certificate to the exact organization and target
using `status_readers`; the request selectors are not authority. The client
requires `no-store`, enforces the 16 KiB response cap and 10-second deadline,
and rejects malformed or contradictory evidence. It never applies, signs,
retries a mutation or falls back to local policy installation. `succeeded`
means this exact operation's recorded receipt and fresh current readback agree;
`superseded` remains distinct even when the current digest is identical at a
higher version. This is a read-only fixture-verified client, not provisioned
runtime identity or live-service qualification.

Browser fixture verification (2026-10-16): 34 policy-status cases plus the 38
existing D2 navigation cases pass across desktop and tablet (72 total), using
the existing VS Code Electron Node runtime without installing Node or packages.
The form clears obsolete evidence when inputs change or the page deactivates,
ignores delayed results for obsolete inputs, rejects response identity
mismatches, and never renders an HTTP error as success. Identifier patterns
escape the hyphen for modern HTML validation. Tests also cover bounded error
display, unavailable/malformed responses and safe text rendering. These are
mocked browser fixtures, not real Policy-Service/VS interoperability, native
Keychain, connected-adapter admission or production acceptance.

## Migration discovery CLI

[`zpr-discover`](cmd/zpr-discover/README.md) is a standalone, production-independent
TCP inventory command for the first migration-discovery slice. Run it on the
operator-selected scanner machine with explicit authorized IP/CIDR targets and
ports. It provides a no-network dry run, bounded concurrency/rate/timeouts,
optional conservative protocol checks and private versioned JSON exports.
Ping and reverse DNS are enabled by default without gating TCP checks; both
can be disabled. JSON config files supply options with explicit CLI overrides.
Ping currently requires the macOS/Linux system utility; other platforms can
use `-ping=false`.
Config `targets` is a string array of IPs, CIDRs or inclusive dash ranges.
There is no host-count cap: scopes are compact and iterated lazily. Version-2
exports retain the full scope and exact planned counts, with host records only
for reached/imported targets. Rate, worker and time limits remain enforced.
Imported adapter listener inventory stays separate and explicitly unverified.
There is no live adapter collector, remote scanner control, Control Room page,
UDP scan, automatic installation or policy generation in this first version.

## Backend package reorganization

The existing web command remains the compatibility entry point for all modes.
The first staged extraction moves neutral forwarding and the allowlisted
HTTP/CONNECT runtime into [`internal/gateway`](internal/gateway/). It accepts
explicit host configuration and depends only on the Go standard library; a
dependency test enforces that boundary. Simulator profile selection, test
workload logging, and the fixed-upstream scenario handler remain in the command
layer and call the extracted helpers.

The second extraction puts the separate browser-access host router and mTLS
configuration in [`internal/browsergateway`](internal/browsergateway/).
It also has a standard-library-only dependency check. Its command file now
contains only environment/default resolution, existing security-header wrapping,
and HTTP server startup. Real certificate integration remains at the command
boundary; package tests cover validation, credential stripping, error responses,
and Control Room routing with the Simulator upstream unavailable.

The third extraction moves service CA loading and mTLS client transport into
[`internal/servicehttp`](internal/servicehttp/). Control Room, Policy-Service
clients, trusted change feeds, and Simulator callers reuse explicit TLS
primitives without importing each other's application configuration.
Policy-specific certificate diagnostics remain in the Policy-Service adapter.
The package has a standard-library-only dependency check and local TLS
accept/reject tests; service routes and authorization remain command-owned.

The fourth extraction puts Control Room's upstream proxy into
[`internal/controlroom`](internal/controlroom/). It accepts an explicit service
origin and configured mTLS transport, preserves browser-credential stripping,
signed enrollment delegation and slow-read transport selection, and has no
Simulator dependency. Its transitive in-module allowlist permits only existing
production authentication/delegation packages. Environment configuration,
authorization, signing, JSON errors and startup remain command-owned.

The fifth extraction moves Control Room API permission mapping and the
named-user authorization gate into that same production package. It accepts an
explicit authorizer, organization, next handler and application error formatter.
The command retains absent/typed-nil local-development fallback, peer/host/origin
checks, login/session configuration and JSON errors. Existing permission names,
route matching, organization binding, denial ordering and CSRF handling remain
unchanged. No Simulator data or configuration is used by the package.

The sixth extraction moves the shared local-request check into
[`internal/servicehttp/local_request.go`](internal/servicehttp/local_request.go).
It accepts an explicit trusted proxy IP and preserves verified-client TLS,
loopback peer/host, exact same-origin and rejection-order behavior. The command
retains per-request environment lookup and exact JSON error formatting through
the existing adapter, so all current callers remain wired consistently.
The package remains standard-library-only and has real TLS plus IPv4/IPv6,
proxy, origin and forwarding-header compatibility tests.

The seventh extraction puts DNS statistics and stylesheet forwarding in
[`internal/dnsstats`](internal/dnsstats/). It takes an explicit upstream URL,
preserves validation, paths, credential stripping, response/error contracts and
default HTTP/HTTPS transport behavior, and has a standard-library-only dependency
check. Control-Service retains environment lookup and route registration.
It neither reads Simulator configuration nor requires Simulator availability.
DNS record transfers and independent relay scripts are unchanged.

The eighth extraction moves signed AXFR record viewing and BIND TSIG parsing to
[`internal/dnsrecords`](internal/dnsrecords/). It accepts an explicit transfer
address and key-file path; the command retains per-request environment lookup
and authenticated route registration. The fixed zone, signing/timeouts,
key reload, sorted/deduplicated JSON and exact errors are unchanged. Real signed
transfer tests and a dependency allowlist enforce the independent production
boundary, reusing the existing DNS library without adding dependencies.

The ninth extraction moves Policy-Service forwarding to
[`internal/policyclient`](internal/policyclient/), accepting a validated origin
and configured mTLS transport. It preserves credential removal, upstream Host,
forwarding-header behavior, paths/queries and the 100ms flush interval.
The command retains environment/TLS validation, authorization, assistant-status
enrichment and application error hooks. It intentionally differs from the
Control Room proxy's forwarding policy. Standard-library-only dependency tests,
forwarding tests and existing real-mTLS integration enforce the boundary.

The tenth extraction adds the bounded assistant attribute fetch and its
two-field attribute contract to that same package. The command retains
local-workspace selection/locking, environment/TLS setup and transport cleanup.
The client preserves its 12-second timeout, redirect refusal, exact 4 MiB
response limit, error messages and attributes-only decoding. Local consumers use
a type alias, keeping their JSON schema unchanged. Tests cover exact size bounds,
redirects, cancellation/read failures, unrelated-data exclusion and local copies.

The eleventh extraction puts the shared Claude HTTP completion transport in
[`internal/assistant`](internal/assistant/), using explicit credentials, endpoint,
HTTP client and caller-provided prompts. Message/reply aliases preserve the JSON
contracts at every command caller. The command retains key discovery, model
selection, the 45-second timeout and redirect refusal, permissions, task skills,
request validation and production-versus-Simulator context selection. The neutral
transport reads no environment or files and has a standard-library-only dependency
check. The same package now owns `ReadAPIKeyFile`, which reads only a
caller-selected path and preserves the protected regular-file/link/size/text
checks; it performs no environment lookup or path discovery. Request
headers/JSON, text-block joining, token usage, errors and the
existing 64 KiB response-reader truncation behavior are preserved.

Production assistant configuration now lives in command-local
`assistant_config.go`, independent of Simulator discovery. Control-Service
startup and policy-status enrichment use only `ANTHROPIC_API_KEY`, then
`ZPR_ANTHROPIC_API_KEY_FILE`, then the existing `assistant/api-key` sibling of
`ZPR_GATEWAY_CONFIG_STORE_DIR`. Production no longer derives a key path from
`SIMULATION_MANIFEST`: configure the explicit file instead. Simulator status
and all design/editor/profile/definition routes use a separate constructor,
preserving manifest-sibling discovery and request-time key reload. Invalid
selected files leave the assistant unavailable and log the error type without
key text or file contents. Shared model/HTTP policy and task prompts are unchanged.
This closes the audited assistant fallback leak, not the full production
application/package boundary.

The twelfth extraction moves production adapter-log inventory types and parsing
into [`internal/controlservice/adapterlogs`](internal/controlservice/adapterlogs/).
It reads only an explicit operator configuration file and uses only the standard
library, enforced by a dependency test. Command aliases preserve the JSON schema;
configuration selection, HTTP handling, log collection, runtime-state lookup and
redaction stay command-owned. Validation, errors, 100-target/8-source limits and
the existing single-decode/65,537-byte-reader behavior are unchanged. Package
tests cover exact bounds, invalid sources, decoder compatibility, reload and
operation without Simulator; existing production log integrations remain intact.

The thirteenth extraction puts shared text formatting/redaction in
[`internal/logview`](internal/logview/). Production adapter logs and diagnostics
now call that neutral package directly rather than a Simulator machine-log
helper or regex. Simulator retains a compatibility adapter. The exact existing
byte-tail, line-count, whitespace and redaction behavior is preserved; diagnostics
keep their separate text bounds. Tests cover exact boundaries and credential
formats, and a standard-library-only dependency test enforces isolation. The
helper receives only text and performs no configuration lookup or collection.

The fourteenth extraction puts gateway draft validation, source-owned JSON
diagnostics, installed-identity selection and versioned file storage in
[`internal/controlservice/gatewaydraft`](internal/controlservice/gatewaydraft/).
The package receives explicit organization/inventory data and paths, with no
Simulator or Admin snapshot types. HTTP composition remains in the command;
an Admin adapter preserves existing snapshot health checks and projects only
the required identity fields. Draft reads work without live inventory;
checks/saves still require it. Neutral fixed-upstream parsing now belongs to
`internal/gateway`, shared with Simulator through its existing adapter.
JSON schema, storage encoding/permissions/revision retention, diagnostics and
review-only behavior are unchanged. Dependency and compatibility race checks
pass locally; this extraction was published in `990d54a`, without expanding
dated deployment evidence. See the
[gateway-owner verification and remaining scope](../REDIRECTORY.md#fourteenth-extraction-production-gateway-draft-owner-2026-10-15).

The fifteenth extraction puts the explicit-config production Admin read
transport in
[`internal/controlservice/adminapi`](internal/controlservice/adminapi/).
The command retains environment configuration, existing error messages and
compatibility adapters; the package owns credential-file loading, TLS and
bounded JSON GETs. It has only standard-library dependencies and no environment,
Simulator or TUI discovery. The audited TLS/redirect/timeout/status/decoder
behavior is unchanged. This slice is locally verified, not deployed or published;
see the [Admin read-owner record](../REDIRECTORY.md#fifteenth-extraction-explicit-config-production-admin-read-client-2026-10-15).

The sixteenth extraction puts snapshot DTOs, concurrent Admin acquisition and
pure service/trusted assembly in
[`internal/controlservice/inventory`](internal/controlservice/inventory/).
An explicit reader and callbacks preserve trusted-before-platform and
enrichment-before-health ordering; the command keeps environment/provider
selection, HTTP routes and compatibility aliases. Existing wire shapes,
health/error gates, detail concurrency, recent/active visas and identity
semantics are unchanged. Package and consumer race checks pass, along with
vet and both Linux builds; the full suite retains four known baseline groups.
This is locally verified, not deployed or published. See the
[snapshot-owner record](../REDIRECTORY.md#sixteenth-extraction-production-snapshot-inventory-owner-2026-10-15)
for remaining provider and application ownership.

The seventeenth extraction adds explicit display-name loading/application and
node telemetry enrichment to the inventory package. The command supplies the
refresh-time file path, original capped diagnostics catalog and query callback;
provider discovery/OTLP/OpenObserve collection stay outside the owner.
Existing freshness, uint64/counter limits, node-only scope, error text and
health behavior are preserved. New exact-bound and catalog-adapter tests plus
unchanged consumer races pass; full tests retain four baseline failure groups.
See the [node-enrichment record](../REDIRECTORY.md#seventeenth-extraction-explicit-node-inventory-enrichment-2026-10-15).
This is locally verified, not deployed or published.

The eighteenth extraction puts diagnostics catalog/wire/provider contracts and
explicit OpenObserve/OTLP acquisition in
[`internal/controlservice/diagnostics`](internal/controlservice/diagnostics/).
The command retains operator configuration validation/selection and authorized
HTTP aggregation, with compatibility aliases/read adapters. Catalog selection/
caps, query scoping, response bounds, resource binding, parsing/redaction and
partial signals are preserved. No environment or Simulator discovery belongs to
the package. New owner/adapter and unchanged consumer tests pass; this is locally
verified, not deployed or published. See the
[diagnostics-acquisition record](../REDIRECTORY.md#eighteenth-extraction-production-diagnostics-acquisition-owner-2026-10-15).

Diagnostics result aggregation now also belongs to the production diagnostics
package. `Aggregate` takes an explicit catalog/provider and caller context,
retaining four-query concurrency, source/overall states, latest timestamps,
ordered 500-log/1,000-metric budgets, field truncation and post-budget search.
The command keeps HTTP validation, authorization composition, request deadlines
and operator configuration. The five-minute default is per call, not a shared
handler write. New bounds/concurrency/HTTP tests and unchanged consumers pass
under race; vet and both Linux builds pass. Full tests retain four known baseline
groups. This is locally verified, not deployed or published; all seven milestone
gates remain open. See the
[aggregation boundary and next audit](../REDIRECTORY.md#production-diagnostics-aggregation-boundary-2026-10-15).

The subsequent
[production startup/composition audit](../REDIRECTORY.md#m1-production-startup-and-http-composition-audit-2026-10-15)
records initialization/failure order, configuration read timing, exact nested
route bindings and distinct server defaults. Five new source/runtime tests
(21 editor-runner cases) and focused race suites pass; no runtime code changed.
Full tests retain the four baseline failure groups. Next is an explicit-input
production Control-Service HTTP composition owner for its fifteen direct and
eight delegated registrations, without moving environment resolution,
persistence, scheduler lifetime or TLS/listener configuration in that slice.
All seven milestone gates remain open; no publication/deployment in this audit.

The subsequent [Control-Service HTTP composition owner](../REDIRECTORY.md#m2-explicit-handler-control-service-http-composition-2026-10-15)
is [`internal/controlservice/httpapi`](internal/controlservice/httpapi/): an
explicit-handler, standard-library-only mux for all 23 production registrations.
The command still supplies domain handlers and owns startup/configuration,
security headers, persistence/scheduler lifetime and TLS/listener setup.
Method specificity, broad-route fallthrough, path values and error/header
behavior are covered by new mux/integration tests and updated source ownership
guards (83 editor-runner cases). Races, vet and both Linux builds pass; full
tests retain four known baseline groups. Next is the explicit-input operator
web-security loader/boundary, with independent production/Simulator environment
adapters. All seven gates remain open; this slice is not deployed or published.

The subsequent [operator web-security owner](../REDIRECTORY.md#m2-explicit-input-operator-web-security-owner-2026-10-15)
is [`internal/operatorweb`](internal/operatorweb/). It loads explicit protected
TLS/OIDC files and owns request protection/auth routes/cleanup, reusing existing
production helpers. The command retains separate Control Room/Simulator
environment adapters and server/permission/store lifecycle. Existing management,
direct HTTPS and development behavior is preserved; CA-path normalization is
outside the reusable factory closure to avoid concurrent writes. Owner/dependency
and unchanged consumer races, 38 editor-runner cases, vet and both Linux builds
pass; full tests retain the four baseline groups. Next is Control Room root HTTP
composition. All seven gates remain open; no publication/deployment in this slice.

The subsequent [Control Room root HTTP composition owner](../REDIRECTORY.md#m2-explicit-handler-control-room-root-http-composition-2026-10-15)
is [`internal/controlroom/httpapi`](internal/controlroom/httpapi/). Its seven
registrations receive explicit prebuilt handlers, preserving auth-config/login,
review/enrollment/API precedence and stylesheet/static fallthrough. The command
still selects management/local authorization wrappers and owns configuration,
signer/review stores, embedded files, headers and server lifetime. Operator-web
auth handlers are exposed individually and reused by its compatibility
registration method. Served-mux/real-boundary regressions report 91 passing
editor-runner cases; owner/consumer races, vet and both Linux builds pass.
Full tests retain the four baseline groups. Next is Policy-Service root HTTP
composition. All seven gates remain open; this slice is not published/deployed.

Mode names, flags, API behavior, embedded resources, scripts, and container
build targets are unchanged. This is not full production/Simulator separation,
production gateway activation, or deployment. See the
[migration evidence and remaining stages](../REDIRECTORY.md#first-backend-extraction-2026-10-14).

Focused validation:

```sh
go test -race ./internal/gateway ./cmd/zpr-web-dashboard -run 'Gateway|WebGateway|ValidateWebGateway' -count=1
go test -race ./internal/controlservice/gatewaydraft ./internal/gateway ./cmd/zpr-web-dashboard -run '^Test(Gateway|ParseGateway|ParseUpstream|ConfigSize|DiagnosticLine|InstalledContracts|Store|ProductionContract|ChangeReview|InternetGateway)' -count=1 -timeout=180s
go test -race ./internal/controlservice/adminapi
go test -race ./internal/controlservice/inventory
go test -race ./internal/controlservice/diagnostics
go test -race ./internal/controlservice/httpapi -count=3 -timeout=120s
go test -race ./internal/controlroom/... ./cmd/zpr-web-dashboard -run '^Test(Root|ControlRoom|ControlHTTP|ProductionComposition|Management|Operator)' -count=1 -timeout=180s
go test -race ./internal/operatorweb ./internal/operatorauth ./internal/servicehttp ./internal/systemconfig -count=3 -timeout=180s
go test -race ./cmd/zpr-web-dashboard -run '^TestProductionComposition' -count=3 -timeout=120s
go test -race ./cmd/zpr-web-dashboard -run '^Test(AdminClientConfiguration|InventoryCompatibility|ProductionContract|CommandCompatibility)' -count=1 -timeout=180s
go build -o /dev/null ./cmd/zpr-web-dashboard
go test -race ./internal/browsergateway ./cmd/zpr-web-dashboard -run 'Browser' -count=1
go test -race ./internal/servicehttp ./cmd/zpr-web-dashboard -run 'ServiceHTTP|MutualTLSClient|CertificateErrors|PolicyService|ControlService|ControlRoom|Trusted.*Change|Simulator.*(Snapshot|Operator)' -count=1
go test -race ./internal/servicehttp ./cmd/zpr-web-dashboard -run 'LocalRequest|LocalEditor|PolicyEditorRejects|ControlRoom' -count=1
go test -race ./internal/controlroom ./cmd/zpr-web-dashboard -run 'ControlRoom|ControlService|OperatorAuthorization|OperatorConfig|OperatorEnrollment|OperatorDelegation|SimulatorAPI|ChangeReview|ConfigurationForm|GatewayAssistantPermissions|AdapterLog|LogComponents' -count=1
go test -race ./internal/dnsstats ./cmd/zpr-web-dashboard -run 'DNSStats|ControlServiceDNS|OperatorDNS|ControlRoom' -count=1
go test -race ./internal/dnsrecords ./cmd/zpr-web-dashboard -run 'DNSRecord|ParseBINDTSIGKey|DNSStats|ControlServiceDNS|OperatorDNS' -count=1
go test -race ./internal/policyclient ./cmd/zpr-web-dashboard -run 'PolicyAttributes|PolicyAssistantAttributes|PolicyProxy|PolicyService|ControlServiceProxiesPolicy|ControlServiceAssistant|PolicyAndAssertionAssistantContext|PolicyAttribute' -count=1
go test -race ./internal/assistant ./cmd/zpr-web-dashboard -run 'Assistant|Claude|DesignReply|DesignProposal|Complete' -count=1 -timeout=90s
go test -race ./internal/controlservice/adapterlogs ./cmd/zpr-web-dashboard -run 'AdapterLog|AdapterFile|FileLogSources|ControlService.*Log|OperatorLogComponents|LogComponents' -count=1 -timeout=90s
go test -race ./internal/logview ./cmd/zpr-web-dashboard -run 'LogLines|LogRedaction|MachineLog|AdapterLog|FileLogSources|OperatorLogComponents|LogComponents|Diagnostics' -count=1 -timeout=90s
```

## Tests

`make test` runs the Go unit and component suites. Browser regressions run in
the pinned Playwright Docker image; Docker is the only additional local
prerequisite. Node/npm and Chromium stay inside the container:

```sh
make test-browser
make test-all
```

When changing browser dependencies, regenerate the lockfile and embedded
browser assets with `make vendor-browser-assets`. This runs npm in the
multi-architecture Node container. CI runs both the Go and browser suites;
failed browser tests retain screenshots and traces as artifacts.

The browser suite runs regressions on desktop and iPad-like tablet Chromium inside
the container, matching the supported web-GUI device scope. It serves the real
static assets with the dashboard's CSP and fixture APIs, without live services,
certificates, or private credentials. Coverage includes
radio log selection, per-source scroll/follow memory, errors and polling lifecycle,
ANSI/HTML safety, policy completions, map/table colors and gateway clouds, visa
refresh/DNS labels, LDAP popups, attributes, memberships, branch collapse, and
explicit organization-switch approval/cancellation and stale-approval handling.
These are deterministic UI regressions, not live network or multi-node acceptance
tests; unfinished protocol, teardown, and recovery work remains separate.

### Simulator top banner

Every Simulator page shares a top banner showing backend connectivity, the active
organization, and the current scenario's name and run state (including cleanup
and terminal outcomes). With no selected run it shows an idle state. The banner
refreshes every three seconds from the Simulator scenario API and remains active
across in-app navigation. Failed or invalid responses clear organization/scenario
values instead of presenting stale data as current; the connection indicator
shows unavailable and its tooltip gives the error.

The operator user section sits before Help, following the Control Room header
layout. When authentication is disabled it shows `User: Not signed in`, rather
than the login-configuration message. Configured sign-in gates and sign-out
behavior are unchanged. The Control Room does not use this Simulator banner or
its APIs.

### Simulator organization-profile editor

Organizations places Browse, File, the organization name, and activation controls
above the organization description and a horizontal navigation bar ordered
Scenarios, Identity, Trusted Sources, Policy, and Services. Browse and File
controls match the live Control Room: dark ink background, lime text,
16px regular sans-serif labels and chevrons, and 31px minimum height in
catalogs and editors.
The same functional
organization header and description persist across all five tabs, including
direct visits to Scenarios and Trusted Sources. Identity no longer includes a
separate Scenarios summary; use the Scenarios tab instead. The sidebar groups
Simulate, Activity, and Workers under MONITORING first, followed by
Organizations under CONFIGURATION, separated by the same divider used in
Control Room. Identity omits the redundant departments/people/groups count
summary above the directory graph. Policy and Services rely on their selected tabs
instead of repeated panel headings,
and Simulator Trusted Sources omits the source title and aggregate
people/groups/attributes summary while retaining records and tab counts.
Scenario browsing and editing remain in the organization's Scenarios tab.
Execution controls, live runner, and results are available only under
MONITORING > Simulate (`/scenarios.html#simulate`). Selecting a scenario or
opening Simulate never starts a run; Run requires a published scenario in the
active organization and is disabled while another run is busy.
Scenarios and Trusted Sources are no longer
separate sidebar entries; their existing URLs remain available. Identity,
Policy, and Services select separate organization detail panels. The standalone
organization "Design with Claude" action has been removed; the Scenario design
assistant remains available. Zoom, branch-toggle, and dismiss glyphs use larger,
heavier symbols, with keyboard focus indicators and labeled buttons.

Organization profile drafts use mutually exclusive Form and Raw JSON views.
The Raw/Form button switches modes without saving; invalid JSON remains in Raw
with an explicit error instead of replacing the draft. Structured sections and
raw source edit the same draft, including unfamiliar fields.

File... contains Save, Discard and Close, plus supporting-definition editing
and saved-definition export for private drafts. Browse... opens runnable
organization profiles or private drafts without activation and confirms before
discarding local edits. History sits beside the editor identity. Analyze checks
the profile contract, not runtime readiness; Format reformats raw JSON without
saving. Wrap defaults off, and Ctrl/Command-S uses the same draft-save action.
Save creates a protected draft revision and never changes running machines,
bundled source files or the active organization.

### Config form editor

Control Room's **Config** page keeps raw ZPLC TOML and a **Form** view of the
same draft. The form covers resolver order/hosts, nodes and substrate interfaces,
links, Visa Service docking, trusted services and attribute mappings, protocols
and ICMP/port settings, service refinements, and bootstrap public-key paths.
Use the add/remove controls for named entries, provider pairs, peers and maps.
String arrays use one entry per line; ICMP codes use comma-separated integers.
Blank optional fields are omitted. Leave certificate/public-key paths here,
not private keys or credentials.

Form changes apply when a field is committed (blur/Tab). The independent
Policy-Service `POST /api/policy/config/form` endpoint parses the supplied
draft and patches only the requested field; it does not read runtime or
Simulator configuration. It reparses the result and verifies that no other
settings changed. Comments, unfamiliar fields and untouched formatting are
preserved. Comments inside a replaced array move above its assignment.
Inline-table or array-of-table layouts that cannot be safely patched, and
values outside the form's supported types, remain available in **Raw TOML**.
Failures leave the source unchanged and appear in the form. Incomplete input
blocks Analyze/Save; switching to raw mode prompts before discarding unapplied
input. Browser requests retain normal operator authorization (`policy.edit`).

Browse, History, New, local import/download, Discard and Ctrl/Command-S Save
operate on the same draft in either view. **Analyze checks TOML syntax only**;
it is not a complete compiler-semantic check. **Save creates a configuration
revision with optimistic revision protection; runtime remains unchanged.**
No installation, organization activation or Simulator API is involved.
The form requires the corresponding Policy-Service endpoint alongside the UI;
deploying only the static assets is insufficient.

Focused validation:

```sh
go test ./cmd/zpr-web-dashboard -run '^TestConfigurationForm'
make test-browser # includes desktop/tablet form tests using the real TOML endpoint
```

### Opt-in organization/scenario acceptance matrix

`scripts/simulator-matrix.mjs` is a long-running Simulator-only acceptance
runner. It is deliberately excluded from `make test`, `make test-all` and the
normal Playwright suite. It needs Node.js 22+ and no additional packages.

Review the plan first:

```sh
node scripts/simulator-matrix.mjs --url http://127.0.0.1:8788
```

Planning discovers each visible organization's published scenarios without
activating organizations or starting scenarios. Catalog GETs may cause the
Simulator's existing workspace seed importer to initialize its catalog.
Unpublished scenarios fail preflight rather than being silently skipped or
automatically published. Archived scenarios are not in the catalog. An
organization with no scenarios is recorded with an empty list; an entirely
empty matrix fails.

Run only after explicitly reserving the Simulator/runtime for this test:

```sh
node scripts/simulator-matrix.mjs \
  --url http://127.0.0.1:8788 \
  --execute --exclusive-runtime \
  --report /absolute/path/to/new-simulator-matrix-report.json
```

The report must be a new file in an existing directory. The runner creates it
with owner-only permissions and atomically checkpoints progress. It records
organization/scenario IDs, published revisions, timing, step action/status
evidence, failures and restoration outcome, not raw log output or authentication
headers. Exit status is nonzero for any scenario, preflight or restoration
failure. No later scenario is started after the first error.

For an authenticated Simulator, use `--headers-file /private/headers.json`
containing the current authorized session's `Cookie` and `X-ZPR-CSRF` header
values. Keep this file outside source control with owner-only permissions.
The runner supplies the exact Simulator Origin on mutations, does not log
headers, does not bypass authorization and does not automatically sign in.
The session must cover the selected organizations and activation/run/cancel
permissions for the duration of the test. Session expiry stops the test;
recovery that cannot authenticate is reported for operator action.
For trusted local HTTPS certificates, use the existing Node trust mechanism
`NODE_EXTRA_CA_CERTS`; do not disable TLS verification.

Execution behavior:

Multi-node activation requires every configured node to be synchronized, not
just a connected Admin endpoint or Active substrate peers. Readiness failures
include each reported node's identity, last contact and pending visa operations.
The PH Visa API client bounds each TCP dial to ten seconds and retries through
its existing reconnect loop; activation does not bypass readiness or extend
its wait to accommodate the operating system's much longer SYN timeout.
Runtime-verification scenarios resolve supporting service addresses from the
selected organization's actual Docker network rather than assuming a shared
`172.30.0.0/24` subnet.

- Require no running/cleaning scenario or ongoing activation before starting.
  Require machine users logged out, workload selections empty, and all reported
  scenario-machine containers stopped/exited/missing. A normal empty session's
  `workloads: null` is accepted; unknown runtime states are not.
- Sequentially activate each organization, wait for completed activation and
  matching runtime/manifest identity, then run every published scenario.
  Check that its published definition/revision has not changed before running.
- Verify run identity, revision and complete step results, including cleanup.
  Actual traffic success/denial checks are the scenario executor's assertions;
  this runner does not invent extra traffic tests absent from a scenario.
- Confirm logged-out sessions, empty workload selections and stopped scenario
  machines after every run. This is state-leak/cleanup verification, not a
  blanket assertion that every production service is stopped.
  Completed steps with leaked runtime state are reported as a serious failure,
  not a passing case. Bundled machine scenarios stop every machine they start
  in their cleanup, including the managed-laptop policy-denial case.
  Failed machine-control readiness records bounded controller/adapter logs,
  kernel UDP receive/drop counters, socket queues and fastpath thread wait
  states before removing the probe. Multi-node diagnostics cover every configured
  node and the Visa adapter; a diagnostic failure is reported and never turns
  failed readiness into success.
- Stop on the first failed scenario, including an isolated traffic-step failure;
  verify cleanup before attempting safe restoration and do not start later
  scenarios or organizations. Also stop on any infrastructure
  or cleanup-step failure, timeout, malformed/missing evidence, changed revision,
  unexpected organization/run replacement, or API failure. Infrastructure
  includes machine startup, controller readiness, login, DNS and other
  non-traffic actions.
- On timeout or SIGINT/SIGTERM, cancel only a run whose start identity the
  runner owns and wait up to five minutes for cleanup. Never cancel an
  unrelated run. Lost/ambiguous start acknowledgements require operator
  recovery if a run remains active.
- Restore the original **organization selection** through normal activation
  when the runtime is idle and clean. This resets that organization's runtime;
  it does not reconstruct prior containers, sessions or process state.
  If cleanup, activation or ownership is unresolved, stop and report failed
  restoration rather than resetting over evidence or another operator's work.
- Use a local per-origin lock to prevent two copies of this runner overlapping.
  The lock cannot exclude browser users, another host or other scripts:
  `--exclusive-runtime` is an operator commitment, not a distributed lock.
  SIGKILL/crash can leave a lock and an active run; inspect both and recover
  the runtime before removing the specifically named stale lock.

Optional controls: `--organizations alpha,beta` selects a subset and order;
without it every organization visible to the authorized catalog is selected
in ID order. Defaults are 20 minutes per activation, two hours per scenario
and two seconds between polls. Override with `--activation-timeout-seconds`,
`--scenario-timeout-seconds`, and `--poll-seconds`. The overall duration grows
with the number and length of scenarios; no automatic schedule is enabled.

**When to use it:** reserve a maintenance/acceptance window after organization
activation, runtime lifecycle, bootstrap/session recovery, routing/policy or
scenario-executor changes, and before a Simulator release. A manually approved
overnight run is appropriate. Do not run it during Great Lakes recovery,
provisioning pilots, demonstrations or unrelated GUI work. For a small change,
use the focused tests first; choose a subset for diagnosis and the complete
matrix for release certification. Decide the actual window before execution.

Fast runner regression tests use fake APIs and a disposable loopback HTTP
server; they never touch the live Simulator:

```sh
node --test scripts/simulator-matrix.test.mjs
```

World Map keeps nodes anchored at their configured latitude/longitude, but
attached adapters do not have to use equal-length dock connections. Distances
account for each adapter's service ring and gateway cloud; angular spacing keeps
components separated and avoids inter-node link corridors. Fan-outs prefer
positions inside the basemap, with outside positions retained when the cluster
cannot fit. Both maps use the graph viewport's dimensions for layout: Topology
chooses landscape/portrait node rows, free hosts use aspect-aware columns, and
World Map adapter fan-outs favor the available orientation without moving
geographic anchors. Significant aspect changes trigger layout recalculation;
manual cameras and disabled Auto-fit remain preserved. Fit uses the actual
viewport aspect in both views. Refreshes are deterministic
for unchanged inputs and viewport dimensions; this is a size-aware layout, not a global collision solver
for different nodes' clusters.
Unconnected adapters are placed alongside the occupied component bounds with
a 40-unit clearance instead of a distant fixed staging offset. Their grid
chooses the side and column count that best fits the viewport, reserving room
for service rings and gateway clouds. When no nodes or docked adapters exist,
World Map centers the unconnected-only group on the basemap; this is display
placement, not inferred geography or an implied network connection.

Both Control Room maps offer a shared **Traffic: Off / Packets** control.
Node halos show measured substrate RX + TX packets/sec in fixed bands; **Scale**
explains the bands and dashed waiting/unavailable/stale outlines. Hover or select
a node for separate RX/TX rates, sample age, and recent history. These counters
include control traffic, not just application packets. No per-adapter, per-link,
or map-wide throughput is inferred. Rates expire after ten seconds even when
updates are paused; use the three- or five-second refresh for live traffic.
Counter resets and inventory/session changes establish a new baseline.
See [World Map layout heuristics](../World%20Map%20layout%20heuristics.md) for the
measurement and display rules. Rate arithmetic regressions run with
`node --test scripts/map-traffic.test.mjs`; both map views are covered by
`tests/browser/geography.spec.mjs` with Simulator unavailable.

## Project layout

```
config.toml.example  Endpoint, timeout and credential file paths
cmd/zpr-dashboard/   Program entry point (thin main)
internal/
  app/               Bubble Tea model: Init/Update/View, input handling
  pages/             Top-level tab views (Dashboard, Visas, Actors)
  components/        Reusable panels, tables, charts and their data
  charts/            Braille dot chart and bar chart primitives
  styles/            Color palette and shared lipgloss styles
  config/            Reads config.toml into the environment
  dataplane/         HTTP client for the vs-admin API
  enrollment/        SQLite registry, admin API, and device HTTPS service
```

## Device enrollment foundation

`internal/enrollment` implements the first storage slice of production device
provisioning without Simulator dependencies or additional module dependencies.
It persists organization-scoped asset invitations, hashed enrollment codes,
expiry/cancellation, single-key claims, and transactional audit events.
The database is owner-readable/writable only. The caller supplies the approved
invitation expiration; this library does not select a deployment lifetime.

This package is not a device authentication or credential-issuance service.
The device service now verifies fresh proof of key possession and atomically
claims an invitation. Durable approval/rejection is implemented;
authoritative inventory integration,
issuance, device-facing rate limits, and the signed Debian/Ubuntu installer
remain pending. Named-user Control Room authorization and opt-in invitation
creation are described below.

### Certificate-authorized invitation administration

Control-Service optionally serves `/api/enrollment/v1/` on its existing private
TLS listener. No additional listener or public device enrollment endpoint is
opened. With both settings absent, these routes return `503`; supplying only
one setting or an invalid configuration prevents service startup:

```sh
ZPR_ENROLLMENT_CONFIG_FILE=/etc/zpr/enrollment-admin.json
ZPR_ENROLLMENT_DATABASE_FILE=/var/lib/zpr/enrollment.sqlite
```

The configuration must explicitly define the invitation lifetime, approved
organization/profile/type catalogs, and administrator certificate permissions.
Review permissions additionally require an explicit approval lifetime.
For example (both lifetimes are examples, not defaults):

```json
{
  "version": 1,
  "invitation_lifetime_seconds": 3600,
  "approval_lifetime_seconds": 3600,
  "organizations": {
    "example-company": {
      "profiles": ["managed-linux"],
      "types": ["laptop", "desktop", "server"]
    }
  },
  "principals": [
    {
      "name": "enrollment-operator",
      "certificate_sha256": "REPLACE_WITH_LOWERCASE_SHA256_OF_CLIENT_CERTIFICATE_DER",
      "organizations": ["example-company"],
      "permissions": ["read", "create", "cancel", "approve", "reject"]
    }
  ]
}
```

Use a dedicated administrator client certificate issued by the configured
Control-Service client CA. Its verified leaf certificate must also match the
explicit SHA-256 pin; a matching CN or request header is not sufficient.
Readers can be assigned only `read`. Unlisted certificates, organizations,
and permissions fail closed. Do not authorize the shared Control Room service
certificate as an administrator. Control Room proxy requests to enrollment
are blocked unless HTTPS named-user authorization and independently verified
backend delegation are explicitly configured.

Obtain the pin from the **public** administrator certificate:

```sh
openssl x509 -in admin-client.crt -outform DER | openssl dgst -sha256
```

Configuration is validated and loaded at startup; restart Control-Service
after changing grants, removing a certificate pin, or rotating a certificate.
Both configured lifetimes must be between 1 second and 30 days (a validation
bound, not a recommended duration). Existing invitation-only configurations
may omit `approval_lifetime_seconds` if they grant neither `approve` nor
`reject`. The pending asset
owner, recipient, and inventory reference are administrative claims, not
verified device attributes or granted policy roles.

| Method | Path | Permission | Input/result |
| --- | --- | --- | --- |
| GET | `/api/enrollment/v1/catalog` | read | Scoped catalogs/lifetimes; general GUI mutations disabled; separate opt-in creator organization capability on delegated route |
| POST | `/api/enrollment/v1/invitations` | create | JSON asset; returns invitation and one-time-visible enrollment code |
| GET | `/api/enrollment/v1/invitations?organization=...` | read | Paginated invitations, no codes or hashes |
| GET | `/api/enrollment/v1/invitations/{id}?organization=...` | read | Invitation detail, no code or hash |
| POST | `/api/enrollment/v1/invitations/{id}/cancel` | cancel | JSON `{"organization":"..."}`; cancels the invitation |
| POST | `/api/enrollment/v1/invitations/{id}/approve` | approve | Organization, reviewed revision, key fingerprint, and verification reason |
| POST | `/api/enrollment/v1/invitations/{id}/reject` | reject | Organization, reviewed revision, key fingerprint, and rejection reason |

Creation accepts `organization`, `asset_id`, `name`, `owner`, `type`, `profile`,
and `recipient`. All are required bounded strings; type/profile must be in the
configured catalog. The backend derives the audited principal from the verified
certificate and the expiration from server configuration. Creating an asset
does not create an adapter, issue a credential, or authorize network access.

JSON mutation bodies are limited to 8192 bytes and reject unknown fields,
trailing JSON values, non-JSON content types, and query parameters. Responses
use `Cache-Control: no-store`. Browser-origin requests are rejected, even if
they present an authorized certificate. Use the APIs directly with an mTLS
client, not through the browser/shared-service proxy.

Listing defaults to 50 invitations; `limit` is bounded to 1–100. Pass the
returned `next_after` as `after` to continue in stable invitation-ID order.
Cancellation is idempotent. Duplicate active asset invitations return `409`.
If a create response is lost, inspect the asset's invitation and cancel/replace
it; codes cannot be retrieved afterward. Delivery retries/idempotency for
creation are not yet implemented. Transfer the code through the approved
separate secure channel, never an email, URL, command argument, or log.

### Reviewing a claimed device

The device proof-of-possession service sets an explicit approval deadline
from its deployment configuration. The claim records `claimed_at`, `approval_expires_at`, and the
verified key fingerprint, and increments `revision`. Same-key retries preserve
that deadline and revision; they cannot keep a request alive indefinitely.
At the deadline, the request reports `approval_expired` and cannot be approved
or resumed. A fresh invitation is required. Legacy pending records without a
deadline fail closed rather than acquiring an unlimited approval window.

Read the invitation before review, verify the asset and fingerprint through
the trusted procedure, then submit:

```json
{
  "organization": "example-company",
  "revision": 2,
  "key_fingerprint": "REPLACE_WITH_THE_REVIEWED_DEVICE_KEY_SHA256",
  "reason": "Asset and device verification code checked through trusted channel"
}
```

Reasons are required and bounded to 256 bytes; never include enrollment secrets
or unnecessary personal information. A missing/mismatched fingerprint is rejected.
Stale revisions and competing decisions return `409`; reload and inspect the
record instead of blindly retrying. Approval also rechecks the current approved
profile/type catalog. Rejection remains available if a profile was removed.
Decision principal, reason, timestamp, and revision are persisted atomically
with the state and audit event. Approved and rejected decisions cannot be
reopened by these endpoints.

An `approved` state means **approved for future credential delivery**, not
enrolled, issued, or connected. Approved assets stay reserved against duplicate
invitations. Cancellation is not revocation and cannot cancel an approved
record; future issuance/revocation services must handle that lifecycle.
Rejected, cancelled, or expired requests may be replaced by a fresh invitation.
Expiration is evaluated on reads; replacing an expired invitation materializes
and audits its expired state transactionally.

These administrative APIs do not provide a way to manufacture a claimed request.
The separate device protocol below verifies key possession first. Neither its
listener nor the installer has been deployed, and the GUI remains blocked.

### Device challenge and proof protocol

The proposed production ownership split and versioned lifecycle APIs are
documented in the [Enrollment Service API contract](ENROLLMENT_SERVICE_CONTRACT.md).
The full lifecycle contract is not implemented; details below describe the
current prototype and its opt-in separate administration listener.

`NewDeviceService` and `NewDeviceHandler` implement the device-facing protocol
independently of the administration handler. They are **not registered on
Control-Service**. The opt-in standalone `zpr-enrollment-service` command
serves them on a separate TLS listener.
The selected first-release boundary is private corporate underlay/VPN access
only; direct public-Internet exposure is not approved. Keep the device listener
separate from Control-Service, Policy-Service, Visa Service Admin, and operator
APIs. The handler requires end-to-end TLS, so a private TCP/L4 gateway can
forward the TLS connection; TLS termination and trusted-proxy identity handling
are not implemented. Do not expose the administrative listener to devices. No
service is automatically started by the dashboard stack or this change.

The invitation's asset fields are currently a snapshot, not an authoritative
inventory. The first release will use a separate Enrollment Service-owned,
organization-scoped asset registry; external ITAM/MDM integration is out of
scope. In production, the Enrollment Service must be the sole writer for
inventory and device identity lifecycle; Control-Service will use an
independently authenticated administrative API and must not share its database.
The current local prototype instead mounts the administrative handler in
Control-Service and uses the enrollment package/SQLite registry for device
proofs. The production ownership split, inventory API/storage,
revision-bound invitation contract, credential issuer, trusted-attribute
publication/revocation, and private network deployment have not been
implemented. Do not treat this protocol as production device admission.
For the initial single-operator deployment, the invitation creator may also
approve the request. The named administrator's permission, revision/key binding,
and audit checks remain required. Multi-person separation of duties is deferred.

The service requires an explicit trusted HTTPS origin (`Audience`), challenge
lifetime (1 second to 5 minutes), and approval lifetime (1 second to 30 days).
These are protocol bounds, not selected deployment defaults. Challenges are
durable in the same SQLite registry and bind version, origin/audience,
organization, invitation ID, purpose, key fingerprint, expiry, and a random nonce.

The initial proof scheme uses RSA-2048 through RSA-4096 with exponent 65537,
SHA-256, and RSASSA-PKCS1-v1_5, matching the existing adapter's RSA signing
family. This is an **enrollment proof**, not the adapter's Noise key or a
replacement for trusted-provider authentication. Hardware-backed key storage,
attestation, and link/authentication credential binding remain installer/provider
integration work. Possession of a key does not prove device posture.

1. Generate the key locally. Encode its public key as standard Base64 of DER
   SubjectPublicKeyInfo. Retain the private key on the device.
2. POST JSON to `/enrollment/v1/challenges` with `organization`,
   `invitation_id`, `purpose: "claim"`, `enrollment_code`, and `public_key`.
   The administrator must transfer the non-secret invitation ID and organization
   with the instructions as well as the code through the separate secure channel.
   The invitation must still be unclaimed and unexpired.
3. The response contains `challenge_id`, `payload` (Base64 of the exact bytes
   to sign), and `expires_at`. The installer must verify the payload's version,
   audience, purpose, invitation/organization, and public-key fingerprint against
   its trusted configuration and local request before signing. Do not sign
   arbitrary unvalidated payloads or reserialize the JSON before signing.
4. Sign SHA-256 of the decoded payload using RSASSA-PKCS1-v1_5, then POST
   `challenge_id` and Base64 `signature` to `/enrollment/v1/proofs`.
   The server verifies the signature against the stored key and payload, then
   claims the invitation, records its approval deadline/audit, and consumes
   the challenge in one transaction. Cancelled/expired invitations fail closed.
5. Persist the local key and non-secret request metadata. To check status,
   request a fresh challenge with `purpose: "status"` and the same public key,
   **without** `enrollment_code`, then sign/submit it identically. Status proofs
   require the claimed key; the invitation code cannot authenticate this operation.
   After a lost proof response, try key-authenticated status first. If the request
   remains unclaimed, obtain a fresh claim challenge; never replay the old proof.

Each proof is single-use, including status proofs. An audience change makes
previous challenges unusable. Replay state persists across service restarts.
Challenge validity ends at its deadline; claim challenges also end no later
than invitation expiry. A status proof returns only invitation ID, state,
revision, key fingerprint, approval deadline, and `credentials_issued: false`.
It does not return owner/recipient, administrative review reasons, credential
material, or network authorization. Approved/rejected/cancelled and
approval-expired requests can be inspected only by their already-bound key.

The device HTTP handler requires TLS, rejects browser-origin requests, uses
the same strict 8192-byte JSON limits, and never accepts secrets in mutation
URLs. It limits requests to 30 per source IP and 600 globally per minute,
without trusting forwarded IP headers, and bounds its source map to 1024.
An authenticated TLS reverse proxy is not yet integrated: simply forwarding
HTTP to the handler fails its TLS requirement.

The registry holds at most 1024 unexpired challenges globally and eight per
invitation, including consumed challenges until expiry. Expired challenges are
removed when another is requested. `429` responses include `Retry-After`;
the installer must back off instead of polling aggressively. These bounded
single-instance controls are not a substitute for production gateway/distributed
abuse protection or deployment-specific capacity sizing.

Tests cover signed claim/status round trips, wrong key/code/organization,
tampered payloads, expiry, cancellation races, persistent concurrent replay
rejection, audit-failure rollback, audience separation, HTTP/TLS guards, private
status shape, and measured challenge/request caps.

### Running the separate device HTTPS service

Build the standalone command from the dashboard module:

```sh
go build -o bin/zpr-enrollment-service ./cmd/zpr-enrollment-service
```

Create an operator-owned configuration file. Example for **local development
only**, using a certificate valid for `localhost` (lifetimes are examples):

```json
{
  "version": 1,
  "listen": "127.0.0.1:9443",
  "allow_non_loopback": false,
  "audience": "https://localhost:9443",
  "database_file": "/var/lib/zpr/enrollment.sqlite",
  "certificate_file": "/etc/zpr/enrollment-server.crt",
  "key_file": "/etc/zpr/enrollment-server.key",
  "challenge_lifetime_seconds": 60,
  "approval_lifetime_seconds": 3600
}
```

Start only when explicitly required:

```sh
bin/zpr-enrollment-service -config /etc/zpr/device-enrollment.json
```

There are no listener, certificate, lifetime, or database defaults.
Configuration rejects unknown fields, trailing JSON, and files over 65536 bytes.
Relative file paths resolve against the configuration file's directory, not
the process working directory. The listener must use a literal IP and explicit
port. Non-loopback addresses require `allow_non_loopback: true`; that flag is
a technical opt-in, **not approval for internet deployment**.

The server validates the certificate/key pair, certificate validity period,
server-authentication usage, and audience hostname before opening the database.
It serves TLS 1.3 or later and requires the HTTP Host to match the configured
audience authority exactly, including any explicit port. The audience is the
stable HTTPS origin trusted by the device, not necessarily the private bind
address. Clients must verify the hostname and trust chain; never use an
insecure TLS override.

Only `/enrollment/v1/challenges` and `/enrollment/v1/proofs` are exposed.
There are no administrative, static GUI, proxy, metrics, or inventory routes
on this listener. The service accepts devices without client certificates
because initial identity is established through the invitation and signed
challenge, not through a credential they do not yet possess.

In the **legacy prototype mode**, the device service and private Control-Service
administration process point to the **same enrollment database on the same host**, with owner-only
permissions under an operator-controlled parent directory. The current SQLite
design is not a network-filesystem or multi-host deployment contract.
Immediate transactions serialize claims/reviews across database connections.
Configure the same approval lifetime in both services; the device service
sets and persists the actual claim deadline. Changes apply after restart and
do not extend existing deadlines. Keep TLS private keys and writable registry
files out of package-download directories.

Read headers are limited to 16 KiB; header/read/write/idle timeouts are
5/10/15/30 seconds. Requests have a 10-second application deadline and a
32-request concurrency cap. Busy responses return `503` with `Retry-After`.
SIGINT/SIGTERM stops acceptance and allows up to 15 seconds for graceful
shutdown before closing remaining connections and the database.

An approved production gateway is still required before remote rollout.
Prefer TCP/TLS pass-through for this initial server. HTTP forwarding is not
supported, and proxy-supplied source/identity headers are not trusted. Behind
a pass-through gateway, connections may share the gateway's source-IP rate
budget; enforce remote-client abuse limits there as well. Certificate renewal,
gateway policy, service supervision, operational monitoring, and remote
deployment approval remain operator work. This change does not configure or
expose any running service.

### Enrollment-owned administration API

The standalone Enrollment Service can now host both the existing device protocol
and a **separate private mTLS named-user administration listener**, sharing one
Enrollment-owned `Store` within that process. Control-Service selects an explicit
remote relay instead of opening the registry. This is a compatibility bridge for
the existing invitation APIs, **not** completion of the proposed `/v1` lifecycle
contract, fresh-company bootstrap or an installable appliance.

Add this optional object to the device service JSON above:

```json
{
  "administration": {
    "listen": "10.20.30.40:9444",
    "audience": "https://enrollment-admin.example.com:9444",
    "certificate_file": "/etc/zpr/enrollment/admin-chain.pem",
    "key_file": "/etc/zpr/enrollment/admin-key.pem",
    "client_ca_file": "/etc/zpr/enrollment/control-service-ca.pem",
    "catalog_file": "/etc/zpr/enrollment/catalog.json",
    "delegation_trust_file": "/etc/zpr/enrollment/delegation-trust.json"
  }
}
```

The listener requires a fixed, explicit loopback/private IP and its own audience
on the same fixed port, separate from the device listener. No wildcard or public
bind is supported for administration. Both listeners must bind successfully before
either starts serving; failure or shutdown of one stops both. Only the device
listener accepts clients without certificates. Administration requires TLS 1.3
and verified client certificates, exact Host, a valid signed delegation, and
independent issuer/subject/organization/operation grants. It exposes only the
existing `/api/operator-enrollment/v1/` routes; certificate-only administration,
device, inventory and GUI routes are not added to this listener.

The catalog uses the existing enrollment JSON contract (organization/profile/type,
invitation/approval lifetimes and GUI-creation flag). `principals` may be empty
or omitted **for this delegated listener only**; direct-certificate administration
still requires explicit principals. Independent named-user rights come from the
delegation trust file, not this optional list. Keep the device and catalog approval
lifetimes aligned. Certificate/key, client CA, catalog and trust files use the
bounded protected operator-file reader (64 KiB, root/current-user-owned,
single-link regular files; private key owner-only). Protect parent directories
and runtime mounts separately. No keys, catalogs or grants are generated.

Configure **Control-Service**, not Control Room, with:

```sh
export ZPR_ENROLLMENT_SERVICE_URL=https://enrollment-admin.example.com:9444
export ZPR_ENROLLMENT_SERVICE_CA_FILE=/etc/zpr/control-service/enrollment-ca.pem
export ZPR_ENROLLMENT_CLIENT_CERT_FILE=/etc/zpr/control-service/enrollment-client.pem
export ZPR_ENROLLMENT_CLIENT_KEY_FILE=/etc/zpr/control-service/enrollment-client-key.pem
```

An optional `ZPR_ENROLLMENT_SERVICE_TLS_SERVER_NAME` explicitly selects the
verified service hostname; otherwise the URL hostname is used. A partial or
invalid remote configuration aborts startup. Remote mode rejects any simultaneous
`ZPR_ENROLLMENT_DATABASE_FILE`, `ZPR_ENROLLMENT_CONFIG_FILE` or
`ZPR_OPERATOR_DELEGATION_TRUST_FILE`; there is no fallback to a local registry.
It relays only the signed operator route, not `/api/enrollment/v1/` direct
administration, and rejects browser credentials, unsigned requests and oversized
bodies. The incoming Control Room connection must also be verified mTLS.
Unavailable transport returns an explicit `502` warning that a mutation may
have an uncertain outcome; reconcile with a fresh read rather than blindly retry.

In **Control Room**, set `ZPR_OPERATOR_DELEGATION_AUDIENCE` to the Enrollment
administration audience and use that exact audience in its signer JSON.
Control Room still sends requests to its existing `ZPR_CONTROL_SERVICE_URL`;
the original short-lived assertion and body pass unchanged through the relay.
No relay signing key is introduced. Without the new audience variable, legacy
signer validation still requires the Control-Service URL.

At the **Enrollment Service**, the existing delegation trust format now pins the
**Control-Service-to-Enrollment client certificate**, together with the trusted
Control Room Ed25519 public signing key. It does not pin the original Room
transport certificate, which is not the peer on this hop. Only accept relays
authorized for this purpose; never trust a browser-supplied client-certificate
header. Enrollment independently verifies the original signature, audience,
method/path/body, named-user scope and durable replay nonce. Its trust audience,
the Room signer audience and the service URL must match exactly.

This opt-in does not migrate data, start containers, change the local stack or
make BAS a network client. The standalone BAS command still opens its configured
registry: **do not share that volume in a production package** or claim complete
sole-writer lifecycle ownership until BAS access is redesigned. Asset lifecycle,
idempotency/reconciliation, complete credential delivery, supervision, trust
bootstrap and package acceptance remain open. No deployment is implied.

Live TLS integration tests cover device claim, approval from a separate
registry connection, key-authenticated status, admin route isolation, exact
Host checks, certificate trust, launch readiness, and graceful shutdown with
Simulator configuration unavailable.

### Private enrollment-backed BAS (local implementation, not deployed)

`cmd/zpr-enrollment-bas` implements the adapter's `/preauthorize` and
`/authorize` exchange and a separate, mTLS-only `/token` listener for Visa
Service `validation/2`. It uses the same local enrollment registry as the
device and review services. It does not read Simulator configuration or expose
administration, enrollment, GUI, or proxy routes.

The adapter client ID and JWT subject are the **inventory asset ID**, not the
display name, invitation ID, or a device-submitted identity. Each BAS process
is explicitly scoped to one organization, so identically named asset IDs in
another organization cannot be used through this issuer.
The adapter's configured authentication CN/name (including its Noise
certificate CN when present) must therefore equal that inventory asset ID.
Installer configuration delivery must establish this binding; setting only
the asset's display name is insufficient.

Only an approved invitation with a stored, distinct BAS runtime public key
and recorded review can authenticate. Enrollment keys cannot sign BAS
nonces. Every authorization/redemption transaction rechecks the approved
state, invitation, revision, and runtime fingerprint. This prevents pending,
rejected, cancelled, changed-key, or changed-revision records from redeeming
an outstanding exchange. It is not a complete revocation service: an issued
JWT remains valid until expiry, and active-link termination still needs the
production Visa Service revocation contract.

Nonces are 32 random bytes, signed using RSA PKCS#1 v1.5/SHA-256. Nonces and
authorization codes have explicit short lifetimes, are stored only as
SHA-256 verifiers, and are consumed atomically once across restarts and
registry connections. A lost authorization/redemption response is not
replayed; begin a fresh exchange. The service caps outstanding exchanges at
8 per organization/client and 1024 globally, and each listener applies the
existing source/global request rate policy and a 32-request concurrency limit.

JWTs use HS384, `iss=zpr/bas`, `aud=zpr`, `sub=<asset ID>`, `iat`, `exp`, and a
random `jti`. Only operator-configured `z/` mappings from trusted `organization`,
`asset_id`, `owner`, `type`, and `profile` fields are emitted. No claims come
from adapter requests, and enrollment recipient/display fields are not
claim sources.

Build without starting or deploying:

```sh
go build -o bin/zpr-enrollment-bas ./cmd/zpr-enrollment-bas
```

Example operator configuration (values are illustrative, not a live Great
Lakes configuration):

```json
{
  "version": 1,
  "organization": "company",
  "allow_non_loopback": false,
  "database_file": "/etc/zpr/enrollment/registry.sqlite",
  "signing_key_file": "/etc/zpr/bas-private/token-signing.key",
  "visa_service_ca_file": "/etc/zpr/bas/visa-service-ca.crt",
  "visa_service_peers": [
    {
      "name": "visa-service",
      "certificate_sha256": "<lowercase SHA-256 of authorized client certificate DER>"
    }
  ],
  "adapter": {
    "listen": "127.0.0.1:4000",
    "origin": "https://auth.zpr:4000",
    "certificate_file": "/etc/zpr/bas/server.crt",
    "key_file": "/etc/zpr/bas/server.key"
  },
  "token": {
    "listen": "127.0.0.1:3999",
    "origin": "https://auth.zpr:3999",
    "certificate_file": "/etc/zpr/bas/server.crt",
    "key_file": "/etc/zpr/bas/server.key"
  },
  "nonce_lifetime_seconds": 60,
  "code_lifetime_seconds": 60,
  "token_lifetime_seconds": 300,
  "claim_sources": {
    "asset_id": "asset_id",
    "organization": "organization",
    "profile": "profile"
  }
}
```

Both origins must use `auth.zpr` and their explicit listener port; the TLS
certificates must currently be valid for that hostname and server usage.
TLS 1.3 and exact Host matching are required. The token listener requires
both a CA-verified client certificate at the TLS handshake and an explicitly
listed certificate fingerprint at the application boundary. Browser requests
are rejected. Pin rotation requires explicit configuration and restart.

Provision at least 48 random bytes as the raw binary HMAC signing key.
The key and its containing directory must be owned by the service account
and owner-only; symlinks and additional hard links to the key are rejected.
Provision the same secret independently in the Visa Service `validation/2`
verification-key configuration. **Never distribute it to the Mac, browser,
package download server, or privileged helper.** Relative configuration file
paths resolve against the configuration directory. Missing settings, unknown
JSON fields, untrusted CA data, and unsafe signing-key files fail explicitly.

Start only after separate rollout authorization:

```sh
bin/zpr-enrollment-bas -config /etc/zpr/private-bas.json
```

Nonce/code lifetimes are 1-300 seconds; token lifetime is 1-3600 seconds.
SIGINT/SIGTERM gracefully shuts down both listeners. If either listener
cannot start or subsequently fails, the other is shut down too.

Tests cover real TLS adapter-to-token exchanges, required mTLS, rejection
of CA-trusted but unapproved peers, route isolation, durable replay
protection, concurrent redemption, expiry boundaries, trusted claims, and
server startup/shutdown with Simulator unavailable. This implements the
BAS exchange only: installer credential/configuration issuance and signed
approval delivery, privileged Keychain handoff, adapter installation, live
Authentication policy/provider activation, and end-to-end traffic remain
open. The wizard still correctly reports credentials not installed.

### Signed Mac runtime-key import approval (local implementation)

The device service can now issue a short-lived, Ed25519-signed authorization
for **importing the separate BAS runtime key**. This is not an adapter
configuration, Noise certificate, BAS JWT, installation result, or proof of
connectivity. The approval issuer is independent of the BAS HS384 signing key;
only its public key belongs on the Mac.

This feature is opt-in. Add both fields to the device-service configuration:

```json
{
  "runtime_import_seed_file": "/etc/zpr/enrollment-private/runtime-import.seed",
  "runtime_import_lifetime_seconds": 60
}
```

The file is exactly 32 raw random Ed25519 seed bytes, held in an owner-only
directory and owner-only regular file owned by the service account. The
existing private-file checks reject symlinks and extra hard links. Relative
paths resolve against the service configuration file. The seed must not be
published to the Mac, helper, browser, or download server.

Independently provision the corresponding 32-byte public key in trusted
desktop setup configuration as canonical standard base64:

```json
{
  "runtime_import_public_key": "<base64 Ed25519 public key>"
}
```

When that pin is configured, approved setup status triggers a fresh
`runtime-key-import` challenge using the existing device challenge/proof
routes. Both the enrollment key and runtime key must sign. The service
rechecks approval and both key bindings in the same transaction as receipt
persistence, audit, and challenge consumption. Ordinary claim/status never
returns this receipt, and unconfigured services cannot issue one.

The receipt signs the exact decoded JSON bytes, with version, HTTPS issuer,
audience `com.zpr.enrollment.helper`, purpose `runtime-key-import`, platform
`macos`, random operation ID, organization, invitation, trusted asset ID,
approval revision, both fingerprints, issue time, and expiry. Lifetimes are
explicitly 1-300 seconds, with no future-time/expiry skew allowance. The
native client verifies signature and context against its trusted issuer pin
and saved identity before reporting authorization. The receipt stays in
native memory; the loopback browser gets only `runtime_import_authorized`,
never the signed receipt or private keys.

The `macos` field scopes the target helper; it is not platform attestation or
proof that the requesting device is running macOS. Native platform and signed
application identity checks belong to the helper's IPC boundary.

Fresh dual-key proofs reconcile the same unexpired receipt after an
uncertain response or restart. Once expired, a fresh proof can request a new
short-lived operation. A future helper must durably enforce **at most one
import for the approved organization/invitation/runtime-key binding**, not
merely one use of an operation ID; refreshed receipts must not permit a
second independent installation or key replacement. It must independently
pin the issuer/audience/organization, verify the actual transferred key
fingerprint, authenticate the signed IPC peer, and reconcile uncertain
Keychain imports without retrying blindly.

Local Mac helper primitives now implement the native runtime vault and durable
receipt-to-import reconciliation described below. The authenticated signed IPC
helper and adapter integration are not implemented; setup still has no import
action and no runtime key is transferred by the wizard.

Offline receipts cannot reflect a revocation made after signing until they
expire. They authorize only the narrow import operation; BAS independently
rechecks current approval before authentication. Complete revocation and
active-link termination remain separate release gates.

Tests cover real TLS receipt delivery, wrong issuer pins, tampering, wrong
purpose/platform/organization/revision/key context, expiry, durable
reconciliation, dual-key proof/replay checks, approval changes between
challenge and proof, and setup restart/browser exclusion. Neither a helper
nor adapter Keychain signing is implemented yet, so the wizard explicitly
reports that no key has been transferred and runtime installation remains
unimplemented.

### Enrollment client core

`internal/enrollment.NewClient` implements the installer's claim/status
exchange. It accepts a trusted HTTPS audience, an optional CA pool (nil uses
OS trust roots), and a `crypto.Signer` exposing an approved RSA public key.
It does not itself select or persist a key store, generate credentials, or change
network settings. The separate development wizard is described below. The signer boundary allows a future
hardware-backed signer without exporting its private key; actual TPM support
is not implemented.

The client verifies server certificates and hostnames using TLS 1.3 or later.
It never follows redirects or inherits environment-controlled HTTP proxies.
Trusted deployments needing a forward proxy require a separate explicit proxy
contract; disabling TLS verification is not supported.

Before signing, it verifies challenge version, audience, organization,
invitation, purpose, local public-key fingerprint, challenge ID, nonce, and
matching expiry. Challenges must still be valid and expire within five minutes.
It signs the exact decoded payload bytes and checks the resulting RSA signature
locally before sending it. Enrollment service and device clocks must be correct;
there is no silent clock-skew override.

`Claim` takes the code in memory; `Status` never accepts a code. `Fingerprint`
supplies the verification value to show in the future wizard. Call `Close` to
close idle connections. The caller retains its signer and non-secret request
metadata to resume status after restart. This core does not persist the code,
log it, or accept it through a command-line interface.

Responses are bounded to 16 KiB and strictly decoded. Wrong identity/key,
invalid revision/state, missing pending approval deadline, and unexpected
credential issuance are rejected. An approved response still means approval
only, not successful credential installation or ZPR connectivity.

HTTP failures return a `RemoteError` with status and bounded `RetryAfter`,
without reflecting server error bodies or request secrets. No requests or
proofs are automatically replayed. After uncertain proof delivery, use a fresh
key-authenticated status request; the wizard must surface uncertainty and
honor backoff rather than silently declaring success or starting a new asset.

Client tests cover a real TLS claim/approval/resumed-status exchange, rejected
challenge bindings before signing, redirect prevention, unknown trust roots,
response limits/error redaction, retry timing, context cancellation,
environment-proxy isolation, and rejection of false issued/connected results.

### Development software enrollment identity

The initial Linux development release uses a **non-hardware-backed software
key**, as approved in the [Provisioning plan](../../Provisioning%20plan.md).
`CreateSoftwareIdentity` and `LoadSoftwareIdentity` in
[identity.go](internal/enrollment/identity.go) provide the local persistence
core. They are available on Linux and macOS (for development tests); this
does not expand the approved installer platform beyond Debian/Ubuntu.

Creation generates RSA-3072 and stores PKCS#8 key material with versioned
`software-development` metadata in `identity.json`. Metadata includes only
the trusted HTTPS audience, organization, and invitation ID. It does not
include the enrollment code, issued credentials, or cached approval status.
The returned identity implements `crypto.Signer` and exposes `Metadata()`;
pass it to `NewClient` and use a fresh `Status` request after restart.

The caller must select an absolute, clean, dedicated state directory under a
trusted parent on a local filesystem that supports hard links and file/directory
sync. Creation makes the leaf directory with mode `0700` and the file with
mode `0600`; existing state must belong to the effective user and have no
group/other permissions. Symlink directories/files, nonregular files, additional
hard links, unexpected fields, oversized/corrupt records, and incompatible
versions/protection/key formats are rejected rather than repaired silently.
The caller must separately validate installation configuration and CA trust;
stored metadata is not an authoritative source of new trust roots.

Creation syncs a complete temporary file, atomically links it to the final
name without replacement, removes the temporary name, and syncs the directory.
Concurrent creation has exactly one winner. An I/O failure after publication
can leave a complete identity even though creation returned an error: surface
the error and inspect/load the state, never assume failure means no key exists.
A crash can leave a private temporary file; there is no automatic deletion
or recovery of temporary key files. Loading missing/corrupt state never
creates a replacement. Losing the key requires administrator-led recovery,
not reuse of the invitation code to impersonate the previous identity.

This is permission-protected storage, **not encrypted-at-rest or non-exportable
storage**. Root/the owning account and backups containing this file can access
the key; copying the state can impersonate the enrolled request. Protect backup
and support bundles accordingly and never include the state file in logs.
TPM storage, attestation, system service-account handoff,
rotation, and production key-recovery policy remain unimplemented.

Tests cover persistence, signing after reload, unsafe filesystem entries,
strict bounded record validation, no-overwrite concurrent creation, and an
actual TLS claim/approval/status exchange using a key reloaded from disk
without the enrollment code. The identity store does not issue credentials
or provide live adapter admission; the development setup command below uses it.

### Fixture-only endpoint runtime-key producer

`internal/enrollment/endpoint_key_fixture.go` joins a local-policy producer
seam to `NewClientWithRuntimeKey`. Its policy checks the caller identity,
code requirement, UID, and scope returned by an injected trusted-verifier
interface; the endpoint key reference comes only from that policy. A bound
reference resumes the exact signer and fingerprint. Missing keys, unbound
existing keys, uncertain creation, and fingerprint changes fail closed rather
than replacing a key. The client continues to use separate enrollment and
runtime signers, and the seam exposes only the public key, fingerprint,
reference, and opaque `crypto.Signer`—never private-key serialization.

This is an **unexported fixture seam**, not a production helper or endpoint.
The original producer tests use in-memory fake vault and binding stores. The
Linux/macOS `endpoint_binding_store_unix.go` adapter now implements durable
public reference/fingerprint storage using the existing enrollment safe-file
patterns, without new dependencies. It remains unexported and has no helper
route, native IPC verifier, or production wiring. The separate native source
adapter described below adds generation without activating any helper.

Trusted operator configuration selects a dedicated absolute, clean directory,
an immutable reference, and trusted ancestors on a local filesystem honoring
file/directory sync and advisory locking. Explicit first provisioning creates
a **new directory only**; opening never creates directories or files. An
existing empty directory, missing established journal, wrong reference,
corrupt/truncated journal, symlink, extra hard link, foreign owner, or
group/other-accessible state fails closed. The directory is owner-only and the
single-link journal is owner-only and owned by the effective UID, matching
the repository's enrollment identity storage rules. The journal contains
version/reference/state/public fingerprint only, never a signer or key bytes.

Cross-process exclusive locking serializes the append-only
`empty → reserved → bound` journal. A reservation is synced **before** vault
creation, so concurrent producers cannot both attempt creation. A surviving
incomplete reservation blocks automatic creation even if the vault has no key.
A failed binding write/sync returns no signer; reopening may recover only a
complete, validated binding after syncing it again and verifying the exact
vault fingerprint. Partial writes remain errors. No repair, deletion,
replacement, rotation, reset, or adoption path exists. Do not delete/recreate
state or fall back from open to create on error. Privileged tampering,
owner-authorized rollback of valid historical state, storage ignoring sync,
and real power-loss durability are not established by these tests.

Offline tests cover disk reopen (including an independent reader process),
cross-process reservation races, concurrent producer creation, immutable
conflicts, missing/mismatched keys, corrupt/unsafe files, partial append, and
injected sync failures. They reuse the existing opaque-signer client seam
with an **in-memory software vault**; the signer itself is not disk-persisted.
These tests prove the local adapter's behavior, not real Keychain protection,
native caller authentication, per-signature authorization, admission, or
production acceptance. Native helper ownership/ACL integration remains gated;
D5 reset/revoke/re-enrollment remains undecided.

### Native endpoint producer source (not runtime-qualified)

The unexported
[native vault](internal/enrollment/endpoint_native_vault.go) and
[Darwin composition](internal/enrollment/endpoint_native_vault_darwin.go) join
that same producer and durable reservation store. The native backend generates
RSA-3072 inside an explicitly configured Keychain using `SecKeyCreatePair`,
then inspects actual native key metadata: permanent, sensitive, always-sensitive,
never-extractable and nonextractable. Unsupported policy metadata fails closed;
there is no import, software or extractable fallback. Existing runtime-import
behavior is unchanged.

Only the public key and opaque SHA-256 PKCS#1-v1.5 signer are exposed. The
configured immutable reference determines the key label; resume must match the
durable public fingerprint. Each returned signature is checked against the
pinned public key. Missing/replaced keys and uncertain creation cannot trigger
replacement. A crash may leave an orphan native key; the durable reservation
blocks another create rather than automatically deleting, repairing or adopting.

Earlier native producer validation passed fake-backed race tests, vet,
formatting, macOS CGo compile/link and Linux portable compile checks.
**No Keychain operation was executed.** Deprecated native API runtime
compatibility and reported protection metadata still need separately authorized
qualification. Default local creator access is not an approved helper/caller
ACL. The helper source boundary below explicitly disables native endpoint
generation, public lookup and signing until an enforceable helper-only ACL is represented;
existing explicit-import runtime behavior is unchanged.

### Trusted helper source boundary (not installed)

The unexported [helper seam](internal/enrollment/endpoint_helper.go) binds the
existing producer to immutable trusted caller/code requirement/UID/scope/key
reference configuration. Its
[Darwin adapter](internal/enrollment/endpoint_helper_darwin_cgo.go) accepts a
Unix connection and reuses kernel audit-token, effective UID and strict code
requirement verification. Caller identity is never populated from request
fields. The returned opaque signer re-verifies that same peer before every
signature, rather than relying only on initial key resolution.

The helper constructor requires protected owner-only
path validation, no symlink or extra hard link, and consistent open-file
identity. This source composition installs no listener, defines no wire
protocol, provisions no UID/code requirement, and grants no runtime authority.
Native endpoint generation, public lookup and signing return an explicit ACL-unavailable
error; they cannot silently inherit default creator access. Fake tests cover
trusted-policy authorization and per-signature denial, not actual kernel peer,
installed ownership/ACL or native protection enforcement.

The installed legacy `SecAccess` API documents trusted applications as allowed
access without confirmation; that is not an exclusive helper-only allowlist.
Inspecting those application lists therefore cannot establish the required
deny-other-applications property. A separately trusted helper executable identity
and an OS access-control primitive qualified to enforce exclusive helper use
are still required before enabling the native path.

The offline [authenticated signing-transport prerequisite](internal/enrollment/endpoint_helper_ipc.go)
implements a frozen v1 contract: bounded strict JSON frames on an already connected
Unix stream, reciprocal kernel UID/code verification, trusted reference/SPKI
pins and locally verified RSA-3072 SHA-256 signatures. This is not an installed
listener or a PH client. PH currently uses a concrete private-key object for BAS;
its opaque signer and macOS peer authentication remain unwired.
PH/AuthCode/BAS integration is therefore still blocked. D5 reset/revoke/re-enroll
and D6 live acceptance remain separate decisions.

The client revalidates the configured helper before each round trip; the server
retains per-sign caller verification. Reference and public-key pins must come
from independent trusted enrollment state, never the socket. Strict schema and
frame bounds, configured deadlines, cancellation and serialized requests fail
closed. `public` retains the existing producer's reserve-before-create/resume
semantics; it introduces no creation retry or fallback. Ambiguous sign failures
close the connection without retry. Authorized operation failures return a
sanitized wire error and preserve the original error for the server owner.

The coordinator independently passed fake-only helper/producer/journal and
policy race selectors (enrollment 6.474s, dashboard 5.748s), scoped vet,
formatting and Darwin compile/link without executing the binary. IPC fixtures
also cover reconnect with unchanged pins, peer revocation, malformed responses,
concurrent signing and lost responses without duplicate signing. The once-only
full `CGO_ENABLED=0 make test` retained the five previously known dashboard
failure groups; enrollment passed (11.650s). Reduced non-CGo coverage does not
qualify actual kernel-peer enforcement, native ACLs or Keychain behavior.

Final combined coordinator validation passed fake-only helper/producer/journal
race selectors in the prior helper/recovery batch (enrollment 5.611s) alongside both actual receiver modes and policy
regressions (dashboard 7.413s). Scoped vet, formatting and cleanup checks passed.
The legacy non-operating producer constructor remains compatible; strict path
checks apply to the helper constructor; native public lookup is now closed. This does not
weaken the closed native generation/signing ACL gate. No existing test was
changed to obtain the pass.

To run only fake-backed producer/journal tests on macOS:

```sh
GOPROXY=off GOSUMDB=off go test -race -count=1 ./internal/enrollment \
  -run '^Test(EndpointHelper|NativeEndpoint|FixtureEndpoint|EndpointBinding)'
```

Existing macOS CGo Keychain tests run native operations without an opt-in.
When such operations are prohibited, use `CGO_ENABLED=0 GOPROXY=off GOSUMDB=off
make test` for the full offline suite; this deliberately excludes native runtime
coverage. Do not run the entire CGo enrollment package as a supposedly fake-only
check. Use a short trusted workspace-local `TMPDIR` if Unix socket path limits
apply to other selected local fixtures.
The complete
workflow and release decisions are in the
[Provisioning plan](../../Provisioning%20plan.md).

### Local browser setup wizard (development)

The [setup command](cmd/zpr-enrollment-setup/main.go) is a runnable development
GUI, not a signed installer or production adapter. Build it explicitly:

```sh
go build -o /tmp/zpr-enrollment-setup ./cmd/zpr-enrollment-setup
/tmp/zpr-enrollment-setup -config /path/to/trusted-setup.json
```

Example operator configuration (replace all paths and the audience):

```json
{
  "version": 1,
  "audience": "https://enroll.example.com:9443",
  "ca_file": "/etc/zpr/enrollment-ca.pem",
  "state_directory": "/var/lib/zpr/enrollment-development",
  "allow_software_development": true
}
```

The state directory is absolute and must be a dedicated owner-only leaf under
an existing trusted parent. Run as its intended owner; do not launch a browser
as root or indiscriminately run the wizard with sudo. The development desktop
package uses the logged-in user as described below; system service-account
handoff is not implemented. Configuration and CA files must be regular,
non-symlink files owned by root or the effective user, not writable by group
or others, and at most 64 KiB. Their parent directories must also be trusted.
Relative `ca_file` paths resolve against the configuration directory. Omit
`ca_file` to use OS trust roots; a configured file supplies a separate CA pool.
An invalid configuration, corrupt identity, or changed stored audience fails
startup. The GUI cannot override the audience, CA roots, or saved invitation.

The command listens only on `127.0.0.1`, chooses an ephemeral port, and prints
a private URL with a one-hour session capability in the fragment. Open it
manually in this machine's normal browser and keep the command running.
Do not share/capture the URL or process output in support logs. No browser is
launched automatically. The page removes the fragment using `replaceState`;
it keeps the capability only in memory and sends it in API headers. Reloading
requires reopening the printed complete URL; after one hour restart the
command for a new session. Ctrl-C/SIGTERM shuts down the local listener.
There is no remote binding option, headless flow, or SSH/browser tunnel contract.

Every API call requires the capability; mutations require the exact local
Origin. Exact Host, Fetch Metadata, method, body size, and strict JSON checks
reject cross-origin or malformed requests. Responses are no-store, scripts
and styles are embedded local assets, and CSP disallows external resources,
inline scripts, framing, and form navigation. These controls do not protect
against root/the same local account or a compromised browser.

Workflow:

1. Enter the administrator-supplied organization and invitation ID. Preparing
   creates/persists the software key locally, without sending a claim.
2. Verify those details and enter the separately delivered code. It is cleared
   from the password field when submitted, never logged or stored, and sent
   only over the verified enrollment HTTPS exchange.
3. Share the displayed fingerprint through an authenticated administrator
   verification channel. Pending approval polls at ten-second intervals;
   longer service `Retry-After` is enforced on the server as well as in the UI.
4. After restart use **Check fresh status** with the saved key, without a code.
   A successful approval explicitly states that credentials are **not issued**
   and ZPR connectivity is **not established**.
5. On a transport/invalid response failure, the outcome remains uncertain.
   Automatic polling stops and code replay is blocked; wait and request fresh
   status. A denied status does not prove the invitation is unclaimed. Only
   explicit administrator-confirmed recovery can submit the code again with
   the same key. Rejected/cancelled/expired requests require administrator
   action, not deletion or replacement of local identity state.

Requests are serialized; concurrent operations are rejected rather than
queued. The ten-second floor also limits manual polling. Retry timing is
in-memory per process, not a distributed rate-control guarantee. Opening a
new command does not restore a cached approval decision.

Go tests cover session/host/origin isolation, strict configuration, no key
replacement, live TLS claim/approval/status after restart, administrator-confirmed
recovery, uncertain responses, server backoff, and graceful shutdown:

```sh
go test -race ./internal/enrollment
sh scripts/test-browser-container.sh \
  tests/browser/enrollment-setup.spec.mjs tests/browser/enrollment-live.spec.mjs
```

The isolated desktop/tablet browser tests start their own temporary loopback
wizard with an unavailable remote service, never Simulator. They verify key
preparation, code-field clearing, uncertain failure, and missing-session
blocking. All four desktop/tablet tests passed in the pinned Playwright
container; host Node/npm is not required. The runner cross-builds the Linux
setup fixture for the Docker server architecture and supplies it through
`ZPR_SETUP_TEST_BINARY`. Direct npm runs can instead build it with local Go.
These tests do not certify a full graphical clean-machine installation or
production credential issuance.

The [real-service browser suite](tests/browser/enrollment-live.spec.mjs) adds
eight desktop/tablet cases, bringing enrollment coverage to twelve passing
browser cases. It uses the actual wizard, TLS device handler, SQLite registry,
and an independent reviewer connection, with invalid Simulator configuration.
It verifies:

- A browser-submitted claim reaches pending approval and automatic polling
  observes a fingerprint-bound approval; only one code submission occurs.
- Wizard restart retains the identical stored key/metadata and requires a
  fresh signed status before showing approval.
- Rejection remains terminal across restart; no code form reappears.
- An unclaimed saved identity requires a status denial, administrator-confirmed
  recovery checkbox, and the server retry delay before code submission.
- A successfully delivered claim whose local browser response is lost remains
  uncertain, then recovers through signed status without another claim or key.

The [opt-in test fixture](internal/enrollment/browser_fixture_test.go) is
compiled into an enrollment Go test executable, not the shipped wizard.
Review/restart commands use the test process's stdin, never an HTTP bypass.
The container runner builds this executable for the Docker architecture and
sets `ZPR_ENROLLMENT_TEST_BINARY`. Direct npm execution can build it with
local Go. Test startup/teardown bounds process waits and closes temporary
listeners and state. Reviewer actions exercise the durable review state
machine, not certificate-authorized administration or Control Room login.
Thus these tests are device-flow acceptance, not named-user authorization,
graphical desktop package certification, credential issuance, or ZPR admission.

### Development Debian/Ubuntu package

Approved initial targets are **Ubuntu 24.04 LTS and Debian 12, amd64 desktop**.
The [package builder](packaging/enrollment/build-deb.sh) requires `dpkg-deb`
and standard Debian/Ubuntu shell utilities. Build a static Linux executable
from the module root, then package it:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -o /tmp/zpr-enrollment-setup ./cmd/zpr-enrollment-setup
sh packaging/enrollment/build-deb.sh /tmp/zpr-enrollment-setup \
  0.1.0~dev1 /tmp/zpr-enrollment-setup_0.1.0~dev1_amd64.deb
```

The output path must not exist. The builder checks version characters and ELF
amd64 format and sets package ownership to root. It creates an **unsigned
development artifact**, not an authenticated installer. A checksum is not a
signature; do not distribute it as the signed package required by the plan.
Detached manifest signing/verification is now implemented below. Production
signing authority, authenticated download hosting, revocation of old versions,
and release automation remain pending.

Contents:

- `/usr/bin/zpr-enrollment-setup`: static wizard binary.
- `/usr/share/applications/zpr-enrollment-setup.desktop`: terminal-based
  **ZPR Machine Setup (Development)** launcher.
- `/usr/share/zpr-enrollment-setup/setup.example.json`: intentionally invalid
  endpoint example; no active configuration or invitation.
- `/usr/share/doc/zpr-enrollment-setup/README`: operator/user instructions.

There are no installation hooks, system services, auto-start listeners,
private keys, codes, or adapter binaries. Installation does not create a
device identity. A modern browser, graphical desktop, and terminal emulator
are prerequisites; the package does not install a desktop environment.

For controlled development testing, install using the target distribution's
package manager. An administrator must then create a root-owned, non-writable
by others `/etc/zpr` directory and `/etc/zpr/enrollment-setup.json`, replacing
the example audience with the independently trusted HTTPS endpoint and
optionally adding `ca_file`. **Omit `state_directory`** in desktop configuration.
Keep the development software-key opt-in explicit. Do not use an invitation,
email, or browser form to select CA trust.

Launch the desktop entry as the logged-in user, or run:

```sh
zpr-enrollment-setup -config /etc/zpr/enrollment-setup.json -user-state
```

The desktop entry opens a terminal; open the printed private URL in the same
user's normal browser. It does not run a browser as root. `-user-state` refuses
root and derives state from `$HOME/.local/state/zpr-enrollment-development`
(it intentionally does not use `XDG_STATE_HOME`). The home and existing
`.local/state` parents must be owned by that user, not symlinks or group/other
writable. Missing parents are created with `0700`; the identity store creates
the private leaf. Existing configuration cannot override the per-user path.
The explicit-state command remains available for development/service testing,
not for the desktop package launcher.

Upgrades, removal, and purge preserve per-user state and administrator-created
configuration: the package owns neither. Losing/deleting the key requires
administrator-led recovery; uninstall is not server revocation. The eventual
machine adapter must not silently consume another user's key. System-service
identity handoff is deliberately not implemented.

The [package lifecycle checks](packaging/enrollment/test-deb.sh) are only for
fresh disposable Docker containers, never a real workstation. They install
the package, run setup tests as an unprivileged user, verify launch readiness,
reinstall it, and purge while checking preserved user state/configuration.
They passed on Debian 12 and Ubuntu 24.04 amd64, with network disabled.
The retained-state fixture checks file preservation, not adapter revocation.
Graphical launcher/desktop installation and end-to-end signed provisioning
still require clean-machine certification.

### Detached release signatures

The approved first distribution contract uses a detached OpenPGP signature
over a canonical text manifest, not an embedded `.deb` signature or an APT
repository. **Installing the `.deb` directly does not verify this signature.**
The retained development artifact is still unsigned; no production signing
identity has been created.

The [signer](packaging/enrollment/sign-release.sh) and
[verifier](packaging/enrollment/verify-release.sh) require GnuPG (`gpg`, `gpgv`),
`sha256sum`, `dpkg-deb`, and standard Debian/Ubuntu shell utilities.
An operator supplies an existing private key through their managed GnuPG
agent/hardware signer. Scripts never generate/import private keys or accept
a passphrase argument. Example operator workflow:

```sh
# PRIMARY_FINGERPRINT is the full uppercase fingerprint of the approved
# existing signer. EXPIRES is an explicit Unix timestamp, within 30 days.
sh packaging/enrollment/sign-release.sh /trusted/build/package.deb \
  0.1.0~dev1 "$PRIMARY_FINGERPRINT" "$EXPIRES" /trusted/releases/new-dev1
```

The new output directory must not exist. Signer input is copied into private
temporary storage; package metadata is checked before signing. The signature
is verified against the requested primary key before publishing. Distribute
only after a successful exit; an I/O failure can leave an incomplete output
directory that must not be served. The output contains exactly:

- `zpr-enrollment-setup_0.1.0~dev1_amd64.deb`
- `release.manifest`
- `release.manifest.asc`

The five-line, newline-terminated ASCII manifest binds format version 1,
release version, expiry epoch, canonical package filename, and SHA-256.
The signer chooses SHA-256 signatures; verification accepts SHA-256/384/512.
No codes, machine keys, CA trust, or device invitations belong in this bundle.
Publish the three files through operator-managed HTTPS without mutable local
write access. Hosting/email delivery is not implemented.

**Before downloading**, provision the trusted verifier scripts (including
`release-common.sh`), binary OpenPGP public keyring, full primary fingerprint,
and exact approved release version through an independent authenticated
operator channel. Do not trust a keyring, fingerprint, script, or version just
because it accompanies the download or appears in its email. The verifier
does not contact keyservers, import keys into the user's keyring, or fall back
to default trust. Example recipient check:

```sh
sh /trusted/tools/verify-release.sh \
  /trusted/download/zpr-enrollment-setup_0.1.0~dev1_amd64.deb \
  /trusted/download/release.manifest /trusted/download/release.manifest.asc \
  /trusted/keys/release-signers.gpg "$PRIMARY_FINGERPRINT" 0.1.0~dev1
```

Verification requires exactly one valid signature from the pinned primary key
or its signing subkey and rejects expired/revoked keys present in the supplied
keyring. It validates canonical manifest bytes, version, lifetime, SHA-256,
and package name/version/architecture. Expected-version pinning rejects
substitution of an older release; it does not automatically discover the
latest release or record a monotonic version history. Correct system clocks
are required. Expiry limits initial download verification, not invitation
lifetime, installed-package execution, or server revocation.

The command copies inputs into private temporary storage and never installs
anything. Preserve the exact original bytes in a trusted, non-writable-by-others
directory before and after verification, then install through the approved
package manager as a separate administrator action. Changing the file after
verification invalidates that result; the scripts are not a privileged
verification-to-install transaction. Signature authenticity is not evidence
of hardware protection, credential issuance, or live ZPR admission.

Operators must independently distribute refreshed keyrings/revocation data
and fingerprints during key rotation. Offline verification cannot discover
a revocation absent from its trusted keyring. No production keys are bundled,
and signed test keys must never become production trust roots.

[Disposable release tests](packaging/enrollment/test-release.sh) generate
short-lived test keys only inside Docker containers and leave no retained
signed release. Valid primary/subkey checks and rejection of tampering,
wrong trust/version, expired keys/releases, excessive lifetime, weak digests,
unexpected paths/fields/bytes, and symlink inputs pass on Debian 12 and an
Ubuntu 24.04-based image. They install no packages and do not certify release
operations or graphical clean-machine provisioning.

### Trusted HTTPS release staging

[download-release.sh](packaging/enrollment/download-release.sh) combines bounded
HTTPS download with the existing detached-signature verifier. It never installs
anything and does not download trust roots, signing keys, or verifier scripts.
Provision this tool and its sibling verification scripts, trusted keyring,
primary fingerprint, exact expected version, and approved download base URL
through an independent authenticated operator channel first.

On Debian 12/Ubuntu 24.04, install the prerequisite tools (`curl`, GnuPG,
`dpkg`, coreutils) through trusted OS package management. Create an existing
private output parent owned by the invoking user with **mode 0700**. Example:

```sh
mkdir -m 0700 "$HOME/zpr-release-downloads"
sh /trusted/tools/download-release.sh \
  https://downloads.example.com/zpr/dev1 \
  /trusted/keys/release-signers.gpg "$PRIMARY_FINGERPRINT" 0.1.0~dev1 \
  "$HOME/zpr-release-downloads/dev1" system
```

The final argument is `system` for OS TLS roots or the path to an independently
trusted CA PEM file. Environment CA overrides (`CURL_CA_BUNDLE`, `SSL_CERT_FILE`,
`SSL_CERT_DIR`), HTTP proxies, and user curl configuration are ignored.
Certificate and hostname verification remain enabled; TLS 1.2 or later is
required for the static download endpoint (the enrollment device service itself
still requires TLS 1.3). Initial URL support is intentionally narrow: HTTPS
DNS/IPv4 authority, optional port, and plain path; no credentials, percent
escapes, queries, fragments, IPv6 literals, or redirect-based CDN links.
Provide the final HTTPS base URL directly rather than weakening TLS checks.

The tool fetches exactly `release.manifest`, `release.manifest.asc`, and the
canonical versioned amd64 package. Only HTTP 200 is accepted; redirects are
not followed. Each transfer has a ten-second connect timeout and 120-second
total timeout. Accepted content limits are 1 KiB manifest, 64 KiB signature,
and 32 MiB package. Curl size checks plus an OS file-size limit bound chunked
transfers even on older curl: temporary data may reach the OS cap (up to
64 MiB) before rejection, but oversized files are never accepted/published.
There is no silent retry, HTTP downgrade, or automatic proxy fallback.

The existing parent must be an absolute canonical path without symlink
components, owned by the current user, mode 0700. Its ancestors, CA, keyring,
and tools must also be independently trusted; this is not protection from
root/the owning account. A private sibling temporary directory holds downloads
and copies of trust material. All signatures, hashes, version, expiry, and
package metadata must verify before publication. GNU `mv -T -n` atomically
publishes the directory without replacing/merging into an existing destination,
including a destination created during download. On failure, no completed
bundle is published and temporary files are removed. Existing output is left
unchanged. Only the package, manifest, and signature remain in successful output.

Staging is a separate unprivileged step, not a privileged verification-to-install
transaction. Preserve the private output, reverify if anything changes or the
release expires, and install the exact verified package through a separate
approved administrator action. The tools do not auto-select newer versions,
send emails, host downloads, manage production keys, or expose an installer GUI.

The [HTTPS download tests](packaging/enrollment/test-download.mjs) run through
`ZPR_TEST_DOWNLOAD=1 sh packaging/enrollment/test-release.sh package.deb` inside
a disposable Docker container with Node, OpenSSL, curl, and GnuPG installed.
They use a local HTTPS server and disposable signed releases, not an external
download host or Simulator. Tests pass with Debian 12 tooling and an Ubuntu
24.04-based image, covering valid staging, curl/proxy override isolation,
untrusted TLS, plaintext/redirect rejection, missing/truncated/oversized transfers,
signature/hash failures, unsafe/symlink parents, concurrent destination creation,
and cleanup. Test keys, TLS identities, and bundles are removed at teardown.

## Mac enrollment app and adapter validation (development)

The Apple Silicon development image contains a per-user **ZPR Machine Setup.app**,
the shared loopback browser wizard, Terminal/default-browser launcher, instructions
and configuration example. Copy the app to the intended user's `~/Applications`.
No privileged installer, launch daemon, network adapter, route/DNS changes,
automatic CA import or enrollment request is performed by copying the app.
Other accounts require their own asset/invitation and enrollment identity.

**This app is enrollment-only, not a connected Mac adapter.** It is ad-hoc signed
for local integrity, **not Developer ID signed or notarized**. Do not remove
quarantine or disable Gatekeeper to distribute it. The Mach-O deployment floor
and bundle minimum are explicitly macOS 13.0, with native arm64 only; testing
on this host does not certify every macOS release, Intel, Finder/Automation
permissions or remote clean-machine installation.

An administrator must provision
`~/Library/Application Support/ZPR/EnrollmentSetup/setup.json` from the bundled
example, supply the reachable HTTPS device service and optional PEM `ca_file`,
and retain `"key_protection":"macos-keychain"`. The invalid example cannot enroll
anything. Configuration/CA files must be regular user/root-owned files, not
shared-writable; they and their non-symlinked parents must be inside the trusted
real desktop home. Trust/ownership of the home and its external ancestors is an
operator prerequisite. Omit `state_directory`, keys and codes. Relative CA paths
are resolved beside the configuration. A remote Mac cannot use localhost on
the operator's machine.

Opening the app requests Terminal automation to run setup as the desktop user.
The wizard opens the default browser to a private one-hour loopback capability
URL. Keep Terminal running; Ctrl-C stops it. If configuration is absent, the app
offers instructions instead of sending a request. If browser launch fails, an
explicit warning retains the URL for manual opening. The launcher requires the
Keychain mode exactly; omission or unavailable native support fails rather than
falling back to plaintext. The existing key-fingerprint verification, separately
delivered enrollment code, claim/status and uncertain-response flow is unchanged.
Approval remains only an administrative decision. Before using the selected Mac
pilot invitation, confirm the reachable device listener, active Authentication/
BAS policy and secure runtime installation path; do not consume a real invitation
for a local package smoke test. For signed runtime-import authorization, supply
the independently trusted `runtime_import_public_key` described in
[signed import approval](#signed-mac-runtime-key-import-approval-local-implementation).
No pin is accepted from the invitation or device response. A pinned receipt does
not install a helper, import a runtime key or configure the adapter.

The separate RSA-3072 enrollment-proof/runtime keys and enrollment metadata are stored in the
current user's configured default **macOS Keychain** as a generic-password item
with service `com.zpr.enrollment.development` and label
`ZPR development enrollment identity`. Only an owner-private, single-link
reference file is published on disk:
`~/Library/Application Support/ZPR/EnrollmentDevelopment/identity.keychain.json`.
Keychain encryption/access control protects storage; the authorized wizard still
loads the software keys in memory. This is not Secure Enclave-backed, TPM
attestation or a production adapter credential.

The native implementation uses Apple's Security/CoreFoundation frameworks via
CGO, not a private-key command-line argument or a third-party Keychain library.
It explicitly scopes lookup to the selected default Keychain and exact persistent
reference. Denial/locked Keychain, deleted item, invalid reference or corruption
fails closed and never produces a replacement key. Publication never overwrites
an existing reference; competing publishers remove only their unused new items.
Changing the default Keychain or app signature can require deliberate recovery/
authorization. Do not automatically grant other applications Keychain access.

Replacing/removing the app preserves the Keychain item, reference and operator
configuration. Removing the app does not revoke enrollment. Legacy unencrypted
development state is not automatically migrated/imported. Administrators must
avoid inviting the same asset again merely because the new Mac profile uses
a separate state path. Lost references/items, account migration and retirement
require deliberate recovery/revocation. A future root adapter must not receive
the user identity by exporting its private key to a plaintext service file.

Native build from this dashboard directory (the explicit deployment flags prevent
the host SDK from silently producing a newer minimum OS than the bundle declares):

```sh
MACOSX_DEPLOYMENT_TARGET=13.0 \
CGO_CFLAGS='-mmacosx-version-min=13.0' \
CGO_LDFLAGS='-mmacosx-version-min=13.0' \
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -trimpath \
  -o /absolute/output/zpr-enrollment-setup-macos-arm64 ./cmd/zpr-enrollment-setup
sh packaging/enrollment/build-macos.sh \
  /absolute/output/zpr-enrollment-setup-macos-arm64 0.1.0 \
  /absolute/output/zpr-enrollment-macos.dmg
sh packaging/enrollment/test-macos.sh \
  /absolute/output/zpr-enrollment-macos.dmg \
  /absolute/output/zpr-enrollment-setup-macos-arm64
```

The builder rejects existing output, wrong architecture, missing Security.framework
or a mismatched Mach-O deployment floor. It compiles the AppleScript launcher,
signs the executable/app ad-hoc, verifies the sealed bundle and creates a read-only
compressed DMG. The test mounts it read-only, checks signatures/plist/resources,
compares the packaged executable against equivalently signed source bytes and
starts the actual packaged wizard under a disposable home. Authorized session
reads succeed and anonymous reads fail; no login Keychain entry, browser, live
claim or adapter is created by that package check.

```sh
go test -race ./internal/enrollment
CGO_ENABLED=0 go test ./internal/enrollment
```

Mac-specific tests use disposable, explicitly scoped test Keychains—not the login
Keychain—to verify key persistence/proof signing, fresh-process reload, competing
publishers, deletion recovery, corrupt/unsafe references, legacy rejection,
configuration-parent checks and protection labels. The native
[Mac pilot test](internal/enrollment/keychain_pilot_darwin_test.go) also performs
a dual-key claim and named review against a disposable TLS device service, then
starts a fresh process that reloads both keys from an isolated Keychain, proves
possession without the code, and reconciles the same signed import receipt.
It verifies browser receipt isolation, wrong issuer rejection and a reference-only
state directory. No real invitation, login/System Keychain, helper, tunnel, route,
DNS, Simulator or production service is used. The no-CGO suite proves that
Keychain configuration fails at startup without native support. Shared browser
flows and Linux/Windows builds remain separate regression gates.

Local pilot refresh verified 2026-10-10 UTC: native enrollment/wizard race tests,
no-CGO enrollment tests, vet, and the fresh-process Mac pilot test pass. The
rebuilt `0.1.1` arm64 development DMG passes packaged signature/resource/binary
comparison and authenticated/anonymous loopback startup checks. Its SHA-256 is
`c2a8997f56b810aed5d802cf84c3e4fe06977a4827905883931bb5795b2ed1d2`.
It is still ad-hoc signed and not notarized; no installation, real invitation
claim, default/System Keychain import or privileged runtime operation occurred.
The required `make test` passes enrollment and all other package suites except
the existing `TestRedwoodLDIFMatchesOrganizationPeopleSitesAndGroups` mismatch
(53 LDAP identities instead of 53 people plus three machines), left unchanged.

### Local native runtime handoff primitives (not installed or wired)

#### Developer ID package preparation

`packaging/enrollment/build-macos.sh` accepts an optional fourth argument:
the full `Developer ID Application: ...` identity name. With that argument it
signs the native wizard and app using hardened runtime and secure timestamps,
then signs the DMG. Without it, existing ad-hoc development behavior remains.
The package is still enrollment-only: no helper, adapter, invitation, private
key, service secret, route or DNS installation is included.

`packaging/enrollment/test-macos.sh` verifies the signed app before comparing
disposable copies with normalized ad-hoc signatures. This accounts for
Developer ID signature metadata without changing the distributed payload.
It checks packaged resources and authenticated/anonymous loopback startup
without creating a default-Keychain identity or contacting enrollment services.

Notarization is a separate explicitly approved upload to Apple. Store its
credentials through `xcrun notarytool store-credentials` with a hidden password
prompt; never put passwords in scripts or repository files. Submit with
`xcrun notarytool submit IMAGE --keychain-profile zpr-notary --wait`, inspect
the final status, then staple/validate the accepted image and assess Gatekeeper.
Neither a signing certificate nor a pending submission is notarization success.

The enrollment-only Apple Silicon `0.1.2` DMG was accepted by Apple under
submission `5dbef621-a6bb-408c-9976-c5124a527852`. Its ticket was stapled and
validated; Gatekeeper reports `accepted`, source `Notarized Developer ID`.
Final stapled-image SHA-256:
`9dbb67a2db60b6e4cf42dffc0eb0ad54271030b52e955d3dbb510e1467d4c17b`.
Packaged signature/content and native loopback startup checks pass after
stapling. This is local distribution verification, not clean-laptop
certification, helper installation, invitation claim, or adapter connectivity.

[runtime_vault_darwin_cgo.go](internal/enrollment/runtime_vault_darwin_cgo.go)
uses native `SecItemImport` to store only the separate RSA-3072 runtime key as
a permanent, sensitive, non-extractable Keychain key. It requires an explicit
Keychain path, uses a deterministic binding label, refuses duplicate imports,
verifies the stored non-extractable attribute and retrieves only the public key.
Nonce signatures use native `SecKeyCreateSignature` with SHA-256 PKCS#1 v1.5;
private-key files, command-line keys and plaintext exports are not used.
Non-extractability does not claim hardware protection or protection from an
authorized process that held the original software key.

[runtime_import_darwin.go](internal/enrollment/runtime_import_darwin.go) is an
unexposed helper primitive. It independently verifies the issuer pin and
organization, signed receipt, expiry and actual transferred runtime key.
A separate helper-owned ledger records only non-secret binding metadata and
intent before the native side effect. The issuer/organization/invitation binding,
not the refreshable operation ID, identifies one import. Asset, revision and
either fingerprint changes are rejected rather than replacing an existing key.
After restart or a lost response, an existing matching native key is proven by
signature and reconciled without another import. Missing/unavailable native state
after persisted intent remains explicitly unresolved; no automatic retry,
replacement or success-shaped fallback is allowed.

Native tests use explicitly scoped disposable Keychains and test ledgers, never
the login/System Keychain or the live invitation. The Mac pilot TLS regression
also imports the approved runtime key locally and uses its native Keychain
signature to complete the isolated enrollment-backed BAS authorization/token
exchange. Runtime vault/import/pilot race tests, all enrollment race tests,
no-CGO tests and vet pass. This verifies compatible signing, not a running PH
adapter or Visa Service admission.

The local [IPC peer verifier](internal/enrollment/runtime_peer_darwin_cgo.go)
checks the kernel-provided Unix-socket `LOCAL_PEERTOKEN`, effective UID and an
independently pinned code-signing requirement via `SecCodeCopyGuestWithAttributes`
and strict `SecCodeCheckValidityWithErrors`. It does not trust a claimed PID,
process path or peer-supplied signing identifier. Isolated tests authenticate
both directions using their own exact ad-hoc code requirement and reject a
different executable, wrong user, malformed requirement and missing connection.
The requirement and expected UID must be fixed by trusted installation/config,
not a browser or IPC request.

These primitives are **not a privileged helper or an end-to-end installer**.
Before exposing them, integrate the tested signed-peer/audit-token checks, trusted
System Keychain/ledger configuration, bounded one-time IPC, the helper's signed
installation/authorization lifecycle, PH runtime signing integration, approved
adapter configuration delivery, and explicit installation-result/revocation
contracts. No production command, network listener, root operation, or wizard
import action has been added. The user selected isolated ad-hoc development
testing because no Developer ID identity is available; laptop distribution and
production helper installation remain blocked. No Gatekeeper bypass is allowed.

### Existing Mac adapter runtime

The separate `zpr-core` packet handler already has a native `utun` backend.
On this Apple Silicon host, native `cargo test --locked -p ph` passes (246 library
and 267 binary tests, two ignored in each runner); `cargo build --locked -p ph
--bin ph` and non-networking help startup pass. Six new Mac-specific tests cover
interface-name/unit bounds, pre-kernel prefix/MTU rejection, IPv6 masks,
scoped/global address recognition and the single-queue restriction. Socket errors
are checked before constructing an owned descriptor; address add/clear operations
are serialized. Those unprivileged checks did not change networking.

The explicitly administrator-approved
[isolated Mac smoke runner](../../zpr-core/integration-test/macos-utun-smoke.sh)
now also passes on this host: kernel-assigned temporary utun creation, MTU 1400
then 1280, exact IPv6 /128 aliases, duplicate add/remove and two teardown cycles.
Cargo builds unprivileged; only the exact ignored test is elevated via cached
sudo or the macOS administrator dialog. Before/after existing interface, IPv4/
IPv6 default-route and DNS snapshots match; no running organization was changed.
This is **interface lifecycle validation**, not packet-flow certification.
Current high-level Mac address lifecycle is IPv6-only and single-queue. Full
route/DNS rollback, reconnect/sleep/wake, allowed/denied traffic,
credential issuance/Keychain-to-runtime handoff and revocation remain
required. Strict Clippy is currently blocked by unrelated existing dependency
lints and an existing capture-worker partial-write lint; these were not changed.
See the [adapter platform plan](../../zpr-core/ADAPTER_PLATFORM_SUPPORT_PLAN.md).

## Windows 11 x64 enrollment installer (development)

Windows is an additional first-stage enrollment target; the existing Linux
desktop package remains supported. The per-user NSIS setup EXE installs the
same loopback browser wizard, a console/browser launcher, instructions, a
configuration example, a current-user Start menu shortcut and an HKCU uninstall
entry. It requires a native AMD64 Windows 11 desktop (build 22000 or newer),
not ARM64, Windows 10 or Server. Run without elevation. Install does not start
a service/browser, import a CA, change networking/firewall rules or contact
the enrollment service.

**It does not install a network adapter/driver or Windows service, issue
credentials or connect traffic.** This development artifact is unsigned and
not yet certified on a real Windows machine. Authenticode/release verification,
trusted release delivery, credential issuance and adapter-runtime integration
are separate unfinished work. Do not bypass Windows security warnings as an
ordinary distribution procedure.

Installed files are under `%LOCALAPPDATA%\ZPR\EnrollmentSetup`. An administrator
must copy `setup.example.json` to `setup.json`, supply the actual reachable
HTTPS device enrollment audience and, if required, a trusted PEM `ca_file`
(relative to the config or an absolute local path). No trust is auto-discovered;
the invalid example cannot enroll a device. A remote Windows machine cannot
use the operator's localhost endpoint. Never put a code, key or `state_directory`
in this configuration. The wizard rejects unsafe writers/owners, reparse
points, hard-linked config/key files, UNC/device paths and alternate streams.

Launch **ZPR Machine Setup (Development)** from the current user's Start menu.
The console opens the default browser to a private one-hour loopback capability
URL. Keep it running; Ctrl-C stops setup. Failure to open a browser is explicit
and retains the URL for manual opening. The existing invitation/key verification,
separately transmitted code and uncertain-response recovery flow is unchanged.
Control Room review controls are available only with matching named-user and
independent Control-Service grants for that organization plus a configured
approval deadline. A reviewer must confirm that the exact key fingerprint was
verified through a trusted channel and record a reason; the backend binds the
decision to the fresh invitation revision and key. Uncertain responses require
fresh-detail reconciliation without automatic retry. This review does not issue
credentials, admit an adapter, or establish connectivity.

Windows private state is
`%LOCALAPPDATA%\ZPR\EnrollmentDevelopment\identity.dpapi`. The RSA-3072 identity
and metadata are encrypted with **current-user DPAPI**, never machine-wide DPAPI,
and stored under user/SYSTEM-only ACLs. Creation publishes complete ciphertext
without overwriting an existing identity; invalid protection, unsafe ACLs,
corruption and decryption failure never generate a replacement key. There is
no plaintext import/fallback. This is software development protection, not TPM
attestation; the logged-in user, compromised processes or privileged recovery
administrators can still impersonate the identity.

Each Windows user has separate enrollment state and needs a separate asset/
invitation. This does not yet mean multiple working network adapters. Neither
upgrade nor uninstall deletes the user's key or operator-created config/CA files;
uninstall does not revoke enrollment. Plan DPAPI recovery before account resets
or profile migration; lost state requires administrator-led replacement/revocation.
The Linux/macOS file protection and state paths remain unchanged.

Build from this dashboard directory with the existing Go toolchain and NSIS:

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -o /absolute/output/zpr-enrollment-setup.exe ./cmd/zpr-enrollment-setup
sh packaging/enrollment/build-windows.sh /absolute/output/zpr-enrollment-setup.exe \
  0.1.0.0 /absolute/output/zpr-enrollment-installer.exe
```

The builder refuses an existing output, non-AMD64 PE payload or invalid four-part
version. For macOS/Linux hosts without NSIS, the provided
`packaging/enrollment/Dockerfile.windows-builder` installs NSIS and the extraction
checker in an isolated, base-digest-pinned image:

```sh
docker build -f packaging/enrollment/Dockerfile.windows-builder \
  -t zpr-enrollment-windows-builder:local packaging/enrollment
docker run --rm -v "$PWD/packaging/enrollment:/packaging:ro" \
  -v /absolute/output:/artifacts zpr-enrollment-windows-builder:local \
  /packaging/build-windows.sh /artifacts/zpr-enrollment-setup.exe \
  0.1.0.0 /artifacts/zpr-enrollment-installer.exe
```

Cross-build, Windows-target vet/test compilation, existing enrollment race
tests and installer extraction checks pass. The extraction runner compares the
packaged EXE/config/launcher/instructions byte-for-byte and rejects invalid inputs:

```sh
docker run --rm -v "$PWD/packaging/enrollment:/packaging:ro" \
  -v /absolute/output:/artifacts zpr-enrollment-windows-builder:local \
  /packaging/test-windows-build.sh /artifacts/zpr-enrollment-setup.exe
```

**Native Windows certification remains required**, without elevation in a clean
Windows 11 x64 account. Run from the dashboard source directory:

```powershell
go test ./internal/enrollment -run Windows -count=1
powershell -NoProfile -File packaging/enrollment/test-windows.ps1 `
  -Installer C:\test\zpr-enrollment-installer.exe
```

The Go suite checks persistence/proof signing, DPAPI tampering, concurrent
publication, private ACLs, config overrides, unsafe paths, loopback authorization
and protection labels. The installer test refuses existing installation/state,
checks silent install, starts the actual wizard, prepares a test-only key without
claiming an invitation, and verifies upgrade/uninstall preservation. It removes
only its newly created test state/config. Also certify interactive Start menu/
default-browser behavior, two-user isolation and DPAPI decryption rejection
under another account, junction/symlink denial, Windows 10/ARM64/Server rejection,
and the actual reachable HTTPS claim/status/restart flow. Never present successful
cross-compilation or extraction as a substitute for those runtime checks.

## Named operator OIDC and opt-in direct HTTPS

[internal/operatorauth](internal/operatorauth/auth.go) provides the approved
OIDC login/session boundary for Control Room. Direct HTTPS and login are
explicitly opt-in; the default HTTP listener does not mount authentication.
Existing certificate-authorized administration remains unchanged. Control Room
GUI invitation/review controls remain disconnected. Versioned enrollment APIs
can now be explicitly enabled with signed named-user delegation and independently
configured Control-Service grants; sign-in alone is never sufficient.

The library uses `github.com/coreos/go-oidc/v3` and `golang.org/x/oauth2`, with
`go-jose` for signed-token integration tests. This concrete authentication
feature needs maintained protocol/crypto libraries; hand-written JWT validation
is intentionally avoided. Existing unrelated module dependencies are preserved.

`operatorauth.New(ctx, config, clientSecret)` validates typed operator
configuration and performs HTTPS discovery. The secret is supplied separately,
not included in public JSON, requests, logs, or sessions. The Control Room now
loads strict bounded configuration and a separate protected secret file.
Actual IdP registration/deployment remains the operator's responsibility. Example typed
configuration's JSON shape:

```json
{
  "version": 1,
  "issuer": "https://identity.example.com",
  "client_id": "zpr-control-room",
  "redirect_url": "https://localhost:8787/auth/operator/callback",
  "session_lifetime_seconds": 600,
  "grants": [{
    "issuer": "https://identity.example.com",
    "subject": "stable-admin-subject",
    "organizations": ["production"],
    "permissions": ["read", "create", "approve", "reject", "cancel"]
  }]
}
```

Issuer/subject pairs are exact and explicit; email, display name, groups,
arbitrary browser headers, and shared backend certificates are never used
as authorization. Grants are copied at initialization, so caller mutation
cannot expand scopes. Updates require a new instance, invalidating old sessions;
there is no hot reload or distributed session store.

When enabled on direct HTTPS, the handler supports:

- `POST /auth/operator/login`: exact same-origin initiation; creates a
  five-minute browser-bound state/nonce and redirects to authorization with
  S256 PKCE and only the `openid` scope.
- `GET /auth/operator/callback`: one-time state/browser cookie, code exchange,
  signed ID-token validation, explicit grant lookup, and opaque session cookie.
  Expired, missing-browser, lost-state and replayed login attempts clear the
  stale flow cookie and return an HTTP 303 to the fixed recovery target
  `/?operator_login=expired`. There is no interim HTML or meta-refresh page.
  Recovery never exchanges the rejected code or reflects callback parameters.
  The five-minute login-attempt timeout and authorization checks are unchanged.
  Failed/expired attempts retain an explicit retry rather than an automatic
  loop; the validated same-origin return location survives until successful
  sign-in.
- `GET /auth/operator/session`: same-origin identity and CSRF token only,
  `no-store`; no provider tokens or client secret.
- `POST /auth/operator/logout`: same-origin CSRF-protected local session
  invalidation. It does not log the user out of the identity provider.
  Control Room then opens provider sign-in directly, without the interim
  operator-access page; the provider's existing SSO policy still applies.
  Simulator retains its explicit sign-in button.

All routes require actual TLS and the exact configured callback authority.
Forwarded headers cannot turn plaintext into trusted HTTPS. Cookies use
`__Host-` names, Secure, HttpOnly, Path `/`, no Domain, and SameSite=Lax.
Discovery authorization/token/JWKS endpoints must also be HTTPS.
The owned IdP HTTP client enforces timeouts, rejects redirects, and does not
inherit environment proxies. Provider token validation uses RS256/ES256,
issuer/client audience, authorized party, nonce, issued-at, signature, and expiry.

Sessions store only grants, CSRF proof, and expiration in memory. They expire
at the earlier of token expiry and an explicit 60-3600 second lifetime.
There is no sliding renewal, refresh-token storage, or offline token acceptance.
Restarts clear all sessions. Login/session capacity is bounded at 256/1024;
expired entries are removed on access. Gateway/source rate controls and
multi-process sessions remain deployment work.

`Session` returns a copied identity/CSRF token for a same-origin reader.
`Authorize` checks the active session plus explicit organization/permission;
mutations additionally require exact Origin and `X-ZPR-CSRF`. Only local logout
uses empty scope arguments. An enrollment integration must always pass the
actual organization and required permission and must never trust an identity
header or serialize a session as an unsigned backend assertion. Request denial
is surfaced without reflecting raw tokens, authorization codes, or provider errors.

Tests use a disposable HTTPS provider with real JWKS/signatures and real code
exchange/PKCE. They cover grant isolation, callback replay, initiating-browser
binding, invalid token claims/signatures, IdP failure, plaintext/cross-origin
rejection, forged headers, CSRF, logout, token-bounded session expiry, and
capacity limits without Simulator:

```sh
go test -race ./internal/operatorauth ./internal/enrollment
```

### Direct HTTPS configuration

Control Room terminates TLS itself. With no management-IP opt-in, it preserves
the existing loopback-only browser/API exposure: use a literal
loopback `-listen` address and a `localhost`/loopback origin with the same fixed
port. A container may listen on `0.0.0.0` only with an explicit
`ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP`; the Docker host port must still bind
to `127.0.0.1`, and the handler requires actual TLS, the exact loopback Host,
and a loopback or exact configured peer address. Other wildcard/remote binding
is rejected unless the explicit management mode below is selected.
Forwarded headers and the existing browser gateway cannot establish TLS or a
named-user identity for these handlers.

For `-mode control-room`, keep the existing Control-Service mTLS environment
configuration and explicitly add:

```sh
export ZPR_CONTROL_ROOM_ORIGIN=https://localhost:8787
export ZPR_CONTROL_ROOM_CERT_FILE=/etc/zpr/control-room/server.crt
export ZPR_CONTROL_ROOM_KEY_FILE=/etc/zpr/control-room/server.key
export ZPR_OPERATOR_OIDC_CONFIG_FILE=/etc/zpr/control-room/oidc.json
export ZPR_OPERATOR_OIDC_SECRET_FILE=/etc/zpr/control-room/oidc-client.secret
go run ./cmd/zpr-web-dashboard -mode control-room -listen 127.0.0.1:8787
```

Provision a current server-auth certificate covering the origin hostname; the
browser must independently trust its issuer. TLS minimum is 1.3. All three TLS
variables are required together; omitting both OIDC variables enables HTTPS
without login **in loopback mode only**. A partial configuration or failed discovery aborts startup,
never falls back to HTTP. No automatic HTTP redirect/listener is started.
Register the exact callback URI in the operator-selected confidential OIDC client,
which must support S256 PKCE and the library's RS256/ES256 signed-token contract.
The identity provider is verified against system CA roots. For an independently
trusted private IdP CA, optionally set `ZPR_OPERATOR_OIDC_CA_FILE` to a protected
PEM file (at most 64 KiB); its certificates are added to system roots. The same
owned client verifies discovery, token exchange, and JWKS. No insecure TLS option,
environment proxy, or redirect bypass is used. A CA file without OIDC configuration
is a startup error.

Files must be regular, single-link, root/current-user-owned, and not writable by
group/others. Key and secret require no group/other permissions (normally 0600).
Final symlinks and special files are rejected. Operators must protect the containing
directories and their ancestors; file permission checks are not an adversary boundary
against root/the owning user or a writable ancestor. Certificate/key/config are
bounded to 64 KiB each; secret to 4 KiB, one nonempty line with optional final newline.
OIDC JSON rejects unknown fields and trailing documents. Configuration/grants,
secret, and certificate are startup snapshots; rotate them with a deliberate restart.

`GET /auth/operator/config` reveals only whether login is configured, not provider
configuration or secrets. The GUI shows Sign in, a verified subject/scope after
callback, and CSRF-protected Sign out. Session/CSRF data remain in memory, never
browser storage. Session expiry is rechecked on page load/window focus; logout is
not reported successful without HTTP 204. A login/config failure is shown explicitly.
The signing/trust configuration below covers only enrollment; it does not
delegate unrelated monitoring/editor operations or replace their separate
operator-policy requirements.

#### Direct management HTTPS

The first production topology is direct HTTPS on a company-selected private
management IP, reachable only through the corporate network/VPN, not a reverse
proxy or public-Internet deployment. Opt in explicitly:

```sh
export ZPR_CONTROL_ROOM_MANAGEMENT_IP=10.20.30.40
export ZPR_CONTROL_ROOM_ORGANIZATION_ID=example-company
export ZPR_CONTROL_ROOM_ORIGIN=https://control.example.com:8787
# Set the TLS, OIDC and independent Control-Service mTLS variables described above.
go run ./cmd/zpr-web-dashboard -mode control-room -listen 10.20.30.40:8787
```

This is an engineering invocation, not a signed appliance installation command.
The management IP must be a canonical private IPv4 or IPv6 address (RFC1918 or
ULA), present in the process's network namespace. The listener must bind that
exact literal IP and the origin's exact port (443 if omitted). Wildcards,
loopback/public management IPs, hostname listeners, ephemeral ports and a
simultaneously configured `ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP` are rejected.
The origin must use a lowercase dotted company DNS hostname, not an IP,
localhost or `.local`. Company DNS, routing and firewall rules must be configured
separately. Bind failure is a startup error, never a wildcard fallback.

**The application is not a firewall/VPN verifier.** Direct unicast clients may
connect; their address does not establish identity. The company must restrict
the listener to authorized corporate/VPN networks and avoid public NAT/port
forwarding. No firewall rules, DNS, containers or listeners are created by the
configuration validator. Container networking/published-port integration is not
implemented: do not relax the exact-IP listener check to fit a bridge container.

Management mode requires both OIDC inputs and initialized authentication; no
anonymous HTTPS fallback exists. The organization ID and every issuer/subject
grant must satisfy the same single-company production scope rules as the
[company bootstrap configuration](packaging/system/README.md#company-management-bootstrap-configuration):
no wildcard/multiple-company or Simulator permissions, and at least one explicit
`monitor.read` grant. Register the exact company callback with the company IdP.
Discovery/trust failure aborts startup. No local Dex or Simulator input is used.
The example's fictional organization, hostname, IP and subject are not credentials.

The handler requires actual TLS and exact Host; foreign Origin and nonempty
`Forwarded`/`X-Forwarded-For`/`X-Forwarded-Host`/`X-Forwarded-Proto` headers are
rejected, not trusted. A reverse proxy is not a supported management topology.
Static/login resources remain available to reach Sign in, but all upstream API
access requires a named session and explicit route/company permissions; unknown
routes fail closed. DNS's `bind9.xsl` asset also requires `monitor.read`.
Enrollment still needs separate verified delegation and backend grants; change
review still needs its independent configuration. Neither becomes deployment.
Existing exact-origin CSRF, state/nonce/PKCE, secure host-only cookies, session
expiry and logout checks remain in force. Browser credentials are stripped
before forwarding over the independent Control-Service mTLS connection.

Startup also checks the supplied leaf-first TLS chain's validity, hostname,
signatures/path constraints, server authentication and TLS 1.3 signing use,
using the offline validator's checks. It does not prove browser/company trust
or check revocation. Company renewal remains company-managed; files are startup
snapshots, so replacement requires a controlled restart, not a hot-reload claim.
No running service is changed by adding this opt-in mode.

### Local development operator login

The local stack can now run a real Dex identity provider, **not** the test
login fixture or the certificate-only browser gateway. Both listener ports are
published only on host loopback. The dedicated development CA is independent of
the machine-control and backend-service CAs. Do not deploy this local static-user
profile remotely or treat it as a production IdP with MFA, account lockout or
managed password recovery.

First prepare the ordinary local stack, including its existing service/client
certificates and runtime directory. Then, for a fresh operator profile, run from
this dashboard directory:

```sh
sh scripts/local-operator-login.sh init dubnom great-lakes
sh scripts/local-operator-login.sh start
sh scripts/local-operator-login.sh trust  # macOS user login keychain; explicit trust change
```

`init` creates protected files under `../../.local-runtime/operator-login`,
refuses to overwrite an existing identity/configuration, and never prints
passwords, client secrets or private keys. `operator-password` contains the
random initial password; open it locally and keep it private. The IdP stores a
bcrypt hash and its signing/session state in persistent SQLite storage.
The local image built with `scripts/Dockerfile.operator-idp` extends the pinned
Dex 2.44.0 image and uses HTTPS at `https://zpr-id.localhost:5556`;
Docker maps that hostname explicitly for the Control Room OIDC client.
The browser resolves `.localhost` locally. Certificates last 90 days and the
development CA 365 days; arrange deliberate renewal rather than disabling TLS
verification when they expire.

The local error template replaces Dex's "Requested resource does not exist."
error for expired login pages (including refresh) with **Sign-in timed out**
and a one-time return to the fixed trusted Control Room sign-in screen, with a
**Timed out. Try again.** fallback action. A new attempt starts only when the
operator selects **Sign in** on that screen.
No stale state/code or browser-supplied return URL is reused. Other provider
errors remain visible. The recovery target is Control Room, not Simulator.
New configurations select `frontend.dir: "/srv/dex/web"` and new IdP containers
build the custom image. Existing containers/configurations are not silently
replaced by `start`: adopting this template requires an explicit IdP image/
configuration rollout with the persistent SQLite database and identities
preserved. The local IdP and Control Room recovery handlers were explicitly
deployed on 2026-10-09; subsequent source edits still require a separate rollout.

The local admin is deliberately granted `organizations:["*"]` at Control Room
and independently at Control-Service: all existing/future organizations, but
**only the explicitly listed permissions**. There is no permission wildcard.
An exact issuer/subject still identifies the user; Dex encodes the stable
`zpr-local-admin` ID plus its local connector into the subject. The GUI displays
this opaque verified subject; the login username is `dubnom`.
General Control Room organization context is explicitly `*`, requiring a global
grant. An organization-specific deployment should use an exact organization ID
and exact grant instead.

Enrollment remains limited to the separate Control-Service `enrollment.json`
organization/type/profile catalog. Initialization seeds `great-lakes`, laptop/
workstation/server and standard profile as explicit local operator configuration,
not from Simulator manifests. Add future enrollment organizations to that file
and deliberately restart Control-Service; global user grants automatically cover
configured organizations without allowing arbitrary unconfigured organizations.
Creation remains **default off** (`gui_invitation_creation_enabled:false`);
cancel capability requires the independent named cancel grant. No review grants,
credential issuance or adapter admission are enabled by setup.

The presence of `operator-login/stack.json` opts the launcher into this profile.
Restart Control-Service using its existing operator environment (retain configured
LDAP/admin/diagnostics settings), then run:

```sh
sh scripts/dashboard-stack.sh restart-control-room
```

The launcher mounts only the Room certificate/key, OIDC config/client secret,
public CA and delegation signer files into Control Room; it does not mount the
initial password, CA signing key or IdP signing database there. The IdP has
`unless-stopped` restart policy and startup checks trusted discovery before
Control Room switches. Configuration, IdP database and enrollment registry
persist across service/container restarts. Control Room's in-memory sessions do
not: sign in again after a Room restart.

When direct HTTPS operator login is configured, a signed-out user is sent
straight to the identity provider's login page through the same-origin native
POST login form. Failed or denied callbacks retain a retryable sign-in screen
instead of automatically redirecting again. Explicit sign-out leaves the user
signed out with a Sign in button, avoiding immediate single-sign-on reentry.
Protected API authorization responses trigger a session recheck; only an
expired session starts sign-in automatically. Permission denials and Visa
Service availability errors do not redirect to the identity provider.
Control Room no longer shows a refreshing Operator Access/checking screen.
Successful focus rechecks leave the signed-in UI visible. Session loss on a
protected API recheck, focus, or reload displays **Session expired. Redirecting
to sign in...** for 1.5 seconds before opening the provider. Automatic initiation
is limited to one attempt until a successful session is established; manual
retry remains available after failed or interrupted sign-in.
Unconfigured local login retains its existing behavior; HTTP never starts OIDC.

Visit **https://localhost:8787** and enter `dubnom` in the
IdP's login field and the locally saved password. Do not use the old HTTP or
`https://127.0.0.1:8787` URL: the configured origin/callback is exactly localhost.
No HTTP redirect listener is provided. Login-bearing documents use
`Referrer-Policy:same-origin` so Chromium supplies Origin on native login POSTs;
callback/API responses retain `no-referrer`, and cross-origin referrers remain
suppressed. Editor mutations now use a shared memory-only CSRF request helper;
logout/session checks erase its token and no mutation is automatically retried.

Simulator remains at **http://127.0.0.1:8788** without named login in this local
profile. Activity and activation checks use private mTLS Control-Service access,
not Control Room browser sessions. Its new `SIMULATOR_OPERATOR_SERVICE_*` URL,
CA/server name and `SIMULATOR_OPERATOR_CLIENT_*` certificate/key are supplied by
the launcher. Explicit incomplete/non-HTTPS service settings fail rather than
falling back to unauthenticated browser access. Legacy `SIMULATOR_CONTROL_ROOM_*`
settings remain only for backward-compatible local callers.
`sh scripts/dashboard-stack.sh restart-simulator-ui` updates only the UI/container without stopping DNS/rig
containers; it refuses if the UI container hosts live socat relays that need a
planned relay restart. This does not switch the active organization.

Validate an explicitly deployed profile with:

```sh
sh scripts/test-local-operator-browser.sh
```

This checks actual Dex password rejection/login, global grants, authenticated
monitoring/catalog reads, native Origin, missing-CSRF denial, GUI configuration
Analyze, reload and logout in desktop/tablet Chromium. Container Chromium pins
the exact local TLS public keys instead of disabling certificate checking
globally; credential traces/screenshots are disabled. The user's browser uses
the installed dedicated CA. The integrated browser fixture also now exercises
the actual production security headers.

To remove user CA trust, run `security remove-trusted-cert` with the exact
`operator-login/ca.crt` path and remove that specific certificate from the login
keychain. Do not delete the identity database/keys or repeatedly rerun `init`
to reset a password; take a protected backup, deliberately replace the bcrypt
entry and protected password file, restart the IdP, then restart Room to invalidate
existing sessions. No credentials belong in source control, chat or deployment logs.

### Route-level Control Room and Simulator policy

Simulator organization activation prepares its Linux/amd64 trusted-service
helper before stopping the previous runtime. A stale or missing helper is built
atomically using local Go, or the pinned `golang:1.26-alpine3.22` Docker builder
when the Simulator runtime has no Go compiler. Builder failures leave the old
artifact and current rig intact. The fallback requires Docker access, the builder
image (or permission to pull it), and dependency download access on a cold cache.
Use `sh scripts/prepare-trusted-service.sh` to prepare it ahead of switching.

Without OIDC configured, both applications retain their existing local-stack
API behavior. No named operator session is required and
`ZPR_CONTROL_ROOM_ORGANIZATION_ID` may remain unset. Control Room still applies
its existing local/proxy origin checks and blocks public/private enrollment
routes; disabled login does not grant enrollment authority. Simulator retains
its existing handler boundaries and does not resolve organization context for
operator authorization in this mode. This is local development behavior, not
approval for unauthenticated remote exposure.

The Control Room API classifies monitoring, policy, and Gateway routes as
`monitor.read`, `policy.read`/`policy.analyze`/`policy.edit`, and
`gateway.read`/`gateway.analyze`/`gateway.edit`. With OIDC enabled, every API
route must have an explicit classification, the operator grant must include the
configured `ZPR_CONTROL_ROOM_ORGANIZATION_ID`, and mutations require the
session's same-origin CSRF proof. Unmapped routes fail closed. A missing
organization with OIDC enabled returns HTTP 503; set
`ZPR_CONTROL_ROOM_ORGANIZATION_ID` to the production organization authorized by
the operator grant rather than disabling authorization or using Simulator state.

Simulator accepts separate `ZPR_SIMULATOR_OPERATOR_*` TLS/OIDC settings and
requires its own confidential OIDC client with callback
`https://localhost:8788/auth/operator/callback`. Its cookie namespace is derived
from issuer, client ID, and callback, so signing into either app does not replace
the other app's browser session. Simulator routes use distinct organization,
scenario, directory, device-lifecycle, simulated-device-session, workload, and
simulator-control permissions. Organization IDs in route paths are checked
against grants; the organization catalog is filtered to granted IDs. Global
Simulator views are checked against the selected active organization. Simulated
device-user login is not operator login.

To enable Simulator OIDC through `scripts/dashboard-stack.sh`, set
`ZPR_SIMULATOR_OPERATOR_ORIGIN`, `ZPR_SIMULATOR_OPERATOR_CERT_FILE`,
`ZPR_SIMULATOR_OPERATOR_KEY_FILE`, `ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE`,
`ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE`, and
`ZPR_SIMULATOR_OPERATOR_TLS_CA_FILE`. The certificate, key, config, secret, and
optional `ZPR_SIMULATOR_OPERATOR_OIDC_CA_FILE` must be under the mounted
`.local-runtime` directory. The launcher defaults the trusted peer to the Docker
bridge gateway; override with `ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP` only when
the direct TLS peer differs. Readiness uses the supplied TLS CA, not an insecure
certificate bypass. Leaving the Simulator origin unset preserves the existing
loopback HTTP development mode.

These route checks are enforced at the web-app edge. General monitoring,
policy-editor, and Simulator operations do not yet carry signed user delegation
to their backend services; mTLS still identifies the service, not the named
operator. Enrollment has its separate signed delegation contract. Do not treat
the current edge policy as backend authorization for direct service callers.

Before deploying, complete operator-selected IdP registration and configure the
independent delegation keys, certificate pins, and grants below. Live GUI forms
still need registry-backed wiring and real-service browser acceptance. No running
service was restarted or identity provider deployed by this work.

### Independently verified named-user delegation

[internal/operatordelegation](internal/operatordelegation/delegation.go) defines
a deliberately narrow, non-JWT signed request format using standard-library
Ed25519. It is not a transferable session, OIDC access token, or unsigned
identity header. Keep the OIDC client secret, delegation signing key, TLS server
key, and Control Room mTLS client key separate.

On Control Room, configure a protected strict JSON signing file:

```json
{
  "version": 1,
  "audience": "https://control-service.example:8790",
  "key_id": "control-room-production-1"
}
```

Set `ZPR_OPERATOR_DELEGATION_SIGNER_FILE` to that file and
`ZPR_OPERATOR_DELEGATION_KEY_FILE` to one protected PKCS#8 Ed25519 PEM private
key. Both are required together and require direct HTTPS/OIDC already enabled.
Audience must exactly equal `ZPR_CONTROL_SERVICE_URL`, including explicit port;
no implicit origin aliases are accepted. Operators provision these keys
independently; no production signing keys are generated by this feature.

On Control-Service, set `ZPR_OPERATOR_DELEGATION_TRUST_FILE` to a protected
strict JSON trust configuration. Existing `ZPR_ENROLLMENT_CONFIG_FILE` and
`ZPR_ENROLLMENT_DATABASE_FILE` are also required:

```json
{
  "version": 1,
  "audience": "https://control-service.example:8790",
  "keys": [{
    "key_id": "control-room-production-1",
    "public_key": "REPLACE_WITH_BASE64URL_NO_PADDING_ED25519_PUBLIC_KEY",
    "certificate_sha256": "REPLACE_WITH_LOWERCASE_LEAF_CERTIFICATE_SHA256"
  }],
  "grants": [{
    "issuer": "https://identity.example.com",
    "subject": "stable-admin-subject",
    "organizations": ["production"],
    "permissions": ["read", "create", "cancel", "approve", "reject"]
  }]
}
```

The placeholder key/pin deliberately fail validation. Public key is the raw
32-byte Ed25519 key, base64url encoded without padding; the pin is the SHA-256
of the Control Room mTLS leaf certificate's DER bytes (not the TLS server
certificate or a CA). Control-Service verifies its existing mTLS chain *and*
the exact configured leaf pin for the asserted key ID. CA trust alone cannot
delegate a user. Independent exact issuer/subject grants determine backend
organizations/permissions; browser-side grants cannot expand these. Unknown
organizations and review permissions without an approval lifetime fail startup.
Issuer/subject audit encoding must fit the existing 256-byte principal limit.
Files follow the same ownership, permission, regular-file, and 64 KiB limits
as OIDC configuration. Grants/trust are startup snapshots; rotate by deliberate
restart. Operator administration remains independent of Simulator.

Browser callers use the existing versioned `/api/enrollment/v1/` contract,
an active same-origin OIDC session, and `X-ZPR-CSRF` for mutations. Control Room
checks its organization/permission grants, bounds request bodies to 8 KiB,
discards caller-supplied delegation headers, and signs only allowlisted enrollment
operations. It rewrites to a separate private
`/api/operator-enrollment/v1/` route; normal proxy access to that route is blocked.
Direct `/api/enrollment/v1/` certificate administration stays separate and never
accepts delegated headers as certificate authority.

The single canonical assertion binds version, key ID, exact audience, verified
issuer/subject, HTTP method, exact path/query, SHA-256 of exact body bytes,
issued/expiry nanoseconds, and a random nonce. Lifetime is at most 30 seconds;
issued-at may be at most two seconds ahead for clock skew. Keep both hosts'
clocks synchronized. The private service validates signature and bindings before
dispatch and consumes a hashed nonce in the registry's SQLite immediate
transaction. Replay tracking persists across restart and concurrent connections,
expires with the assertion, and is capped at 8192 live entries. Database errors,
capacity exhaustion, or duplicate consumption fail closed; there is no replay
cache fallback. A denied or failed operation still consumes its accepted assertion.

Creation/cancellation/approval/rejection reuse the existing registry/audit
transactions, attributing actions to `oidc:["exact-issuer","exact-subject"]`.
Review retains current-revision/key-fingerprint/deadline checks. Signing is not
credential issuance or adapter admission. Once signed, an in-flight assertion can
finish within its short validity even if the browser logs out; logout blocks new
delegation but does not retroactively cancel accepted requests. No automatic
mutation retries are introduced. If a response is lost, inspect registry state
before requesting another mutation; a token cannot be replayed.

The catalog retains `gui_mutations_enabled:false`: general GUI mutations/review
are not enabled. Invitation creation has a separate, explicitly configured
capability described below. A successful catalog read alone never authorizes
creation.

Tests include tampered method/path/query/body/signature, wrong certificate,
absent verification, wrong issuer/subject, expiry/future issue time, replay and
concurrent consumption, persisted replay after reopen, server-grant isolation,
direct-route separation, named audit attribution, and an actual HTTPS OIDC
login → mTLS delegated catalog/create/cancel → logout path:

```sh
go test -race ./internal/operatordelegation ./internal/enrollment ./internal/operatorauth
go test -race ./cmd/zpr-web-dashboard -run 'Operator|Delegation|ControlRoom|Enrollment'
```

### Control Room provisioning invitations and read-only registry

Provisioning > Adapters now offers a memory-only invitation worksheet and
local review dialog for name, owner, organization/type/profile, inventory
reference, and instruction recipient. **Review worksheet** submits nothing.
**Clear worksheet** and reload clear form values; no form values, CSRF proof, or
codes are saved in browser storage. Never paste an enrollment code or private key
into this form.

#### Local deployment readiness preflight

Before preparing an invitation, an operator can complete the **Service
readiness preflight** in the same page. It collects a Docker host label and
private IPv4 address, then asks the operator to attest to the pilot/production
mode's private dnsmasq answer (with no public service address record), Route 53
DNS-01 IAM scope limited to
`_acme-challenge.enrollment.zpr.net`, direct TLS 1.3 certificate, LAN/VPN firewall,
isolated Compose/data volume, separate admin/device listeners, package
readiness, recovery/audit, and code-delivery checks. The canonical service name
is now `enrollment.zpr.net`, following the purchase of `zpr.net`. Registration
does not itself configure DNS delegation, the private dnsmasq answer, the
Route 53 validation record, or a TLS certificate; those remain deployment
steps. Configure the public DNS authority for DNS-01 validation without
publishing a public service A/AAAA record, and configure dnsmasq to resolve the
service hostname privately for target clients. Update the Route 53 DNS-01 IAM
policy to the exact `_acme-challenge.enrollment.zpr.net` record before enabling
automation. The address is validated as a private IPv4 address, including the
ZeroTier CGNAT range.

The pilot is explicitly disposable and does not require backups. Production
requires the encrypted backup/recovery attestation and an authenticated SSO/MFA
code-delivery portal. Pilot code delivery must use a password-manager secure
share or a verified phone/in-person handoff, always separate from the installer
link. No AWS credentials, passwords, invitation codes, keys, or other secrets
belong in the form.

The readiness checks are **operator attestations, not network probes**. On
submission, the Enrollment Service stores the organization-scoped values and
operator identity in its SQLite registry, appends an audit record, and requires
a valid record for invitation creation through both direct administration and
the Control Room operator flow. Records expire 30 days after the last explicit
submission; every submission, including an unchanged re-attestation, advances
the revision and audit history. The service still cannot independently verify
DNS, TLS, firewall, Docker, package, recovery, or delivery-channel claims.
Clearing the form or signing out only clears the page; it does not revoke the
persisted record. Re-attest after relevant infrastructure changes and at least
every 30 days. Do not mark an item ready unless it has actually been verified.
The service contract and macOS package are still prototypes: the notarized
macOS package is enrollment-only and does not install the adapter, so it cannot
currently satisfy the package-readiness check or provide end-to-end
provisioning.

Without an authorized catalog, fields are explicitly unvalidated requests and
creation is locked. With configured HTTPS/OIDC login and independently verified
delegation, the form uses approved organization/type/profile selects; changing
organization clears incompatible choices. Catalog selections are administrative
claims, not verified device attributes. The page also loads organization-scoped
registry pages of up to 50 records.
**Next page** follows the service cursor; **Check again** reloads the catalog and
first page for the selected organization. **Details** reads a fresh record showing
revision, fingerprint, deadlines, and audit identity/reason; it never retrieves
an enrollment code. Navigation, session checks/loss, and logout clear registry
data and details. Stale reads are discarded. Invalid, denied, and unavailable
responses are distinguished from a legitimately empty page.

#### Opt-in invitation creation

Keep creation disabled in existing deployments (the default). To deliberately
enable it, add `"gui_invitation_creation_enabled": true` to the operator-owned
Control-Service `ZPR_ENROLLMENT_CONFIG_FILE`. Configure HTTPS/OIDC, signing and
independent backend trust as above, and give the exact issuer/subject `read` and
`create` grants for the intended organizations at **both** Control Room and
Control-Service. Apply the configuration through your normal deployment process;
no running service or configuration was changed by this implementation.

Control-Service returns `gui_create_organizations` containing only independently
authorized creator organizations when this flag is enabled. Older catalogs or
absent capabilities keep creation locked. The service also enforces this opt-in
on delegated POST creation, so changing the browser UI cannot bypass it.
Direct certificate-authorized creation remains unchanged by this flag.
The legacy general-mutation flag does not unlock anything.

1. Choose approved organization/type/profile and fill the remaining asset fields.
   Asset values must fit the backend limit of 256 UTF-8 bytes without line breaks.
2. Select **Create invitation**, review the exact asset and lifetime, and
   acknowledge separate authenticated code delivery. **Confirm creation** sends
   one same-origin CSRF-protected POST; double submission is blocked.
3. A validated HTTP 201 response shows the invitation ID, audited named principal,
   expiry, and code once in a dialog. Record the ID and deliver the code through a
   separate authenticated secure channel, never email or the installer URL.
4. Closing, navigation, session checks/loss, or logout erases the code from the
   page. Read APIs cannot recover it. No code is automatically copied, downloaded,
   emailed, or saved to browser storage. If it was not securely delivered, use
   authorized cancellation from fresh **Details**, certificate administration,
   or expiry before replacing it.
5. A timeout (20 seconds), lost response, unexpected status, or malformed/mismatched
   creation result is **uncertain**, not success or failure. No automatic retry
   occurs. Creation stays locked in this page until explicit registry
   reconciliation is acknowledged. Clearing the form, refreshing the catalog,
   or rechecking the session does not remove this uncertainty.
6. Check **all registry pages** for the organization and inventory reference.
   Resolve an existing unusable invitation by authorized GUI cancellation,
   certificate administration, or expiry before requesting a replacement. Reload loses the in-memory guard;
   it does not prove failure or make a retry safe. The registry's active-asset
   uniqueness remains the durable duplicate protection.

Approval and rejection GUI controls remain disabled. Creating an
invitation sends no email, reserves no admitted/live adapter, and issues no
credentials. No production signed package host or installer link is configured;
the retained package remains unsigned development tooling.

#### Authorized invitation cancellation

An administrator with `read` and `cancel` grants for the organization at **both**
Control Room and Control-Service can cancel an `invited` or `pending_approval`
request whenever it remains active. No additional feature flag or creation
opt-in is required. The catalog independently advertises
`gui_cancel_organizations`; missing/older capabilities keep the GUI locked.
HTTPS/OIDC, CSRF and independently verified delegation remain mandatory.

1. Open Provisioning > Adapters, select the organization, and find the invitation
   across all registry pages. Open **Details** for a fresh service read.
2. Review the exact ID, inventory reference, owner, state, revision, key
   fingerprint (empty before claim), and invitation/approval deadline.
3. Enter a reason of 1–256 UTF-8 bytes without line breaks, acknowledge the exact
   invitation/revision, and select **Confirm cancellation**. The GUI sends one
   CSRF-protected POST with organization, revision, fingerprint and reason.
4. Control-Service independently authorizes the named user and checks current
   state, deadline, revision and fingerprint in one SQLite transaction. A
   concurrent claim/decision cannot cancel an unseen revision. Successful
   cancellation increments the revision and durably records the named decision
   principal, reason, time and audit event. The code and pending submission can
   no longer be used.
5. Confirm the returned cancellation summary or reopen **Details** to read back
   its audit fields. Approved, rejected, cancelled and expired records cannot be
   cancelled through this workflow. This is not credential revocation.
6. Explicit rejection/conflict requires reopening fresh **Details** before
   another decision. A 20-second timeout, lost/malformed/mismatched response,
   navigation, dialog closure, or session loss during a request means
   **outcome uncertain**, not failure. No automatic retry occurs.
7. Reopen fresh **Details** and inspect state/revision/decision fields. Select
   **Acknowledge refreshed outcome**, then reopen again before a new cancellation
   if still active. An in-flight service operation may finish after a read; a new
   operation remains revision-bound. Refresh/session checks preserve the
   page-memory uncertainty marker. Reload loses it but does not prove failure.

Delegated cancellation now requires the checked body:
`{"organization":"…","revision":2,"key_fingerprint":"…","reason":"…"}`.
Direct certificate-authorized cancellation retains its existing
`{"organization":"…"}` body and idempotent behavior. Neither path reads or calls
Simulator. Real HTTPS/OIDC/mTLS/SQLite browser tests cover invited and pending
cancellation, named audit and committed-but-lost response readback with Simulator
unavailable.

Browser acceptance checks run without Simulator:

```sh
sh scripts/test-browser-container.sh dashboard.spec.mjs provisioning-live.spec.mjs \
  --grep 'GUI operator|GUI [Pp]rovisioning|live provisioning'
```

The real HTTPS browser fixture verifies opt-in creation through OIDC, CSRF, mTLS
delegation, and SQLite, named audit, secret-free readback, and a committed but
lost response without a retry. It does not certify device installation or
credential issuance.

# Compiling the binary

To compile the final binary, run `make all`. The binaries will be located at bin/zpr-dashboard and bin/zpr-sim.


## Running it

Copy `config.toml.example` to `config.toml` and edit it.


```sh
make run                                # Start admin panel
```

The same lifecycle starts a separate simulation operator site at
`http://127.0.0.1:8788`. It is intentionally simulation-only and provides
manifest-backed agent/service inventory, a fixed fleet of 20 machine profiles,
workload placement, and recent managed-service logs. The fleet has 12 laptops
and 8 desktops; machine type, model, location, owner, and secure posture are
seeded into the local demo LDAP directory when the disposable Linux rig starts.
Its Activity page is a live view of recent Control Room visa and denial data.
It shows request/approval/denial totals, sortable recent-visa and denial tables,
and a manual Refresh beside the stream status. It is an operational feed, not a
durable audit archive. **Workers** merges passive device/runtime inventory and
workload logs, with device-type filtering. Control Room remains the read-only network and policy monitor at
`http://127.0.0.1:8787`.

The Control Room sidebar lists Map, Status, Security, Services, Nodes, Trusted
Sources, Adapter Logs, and the external Log Manager (marked with a green ↗
arrow). Policy/Assertions, Gateways, and Config follow under a small
**Configuration** label; the label is hidden in condensed and mobile layouts.

The top header uses concise **Control Room** and **Visa Service** titles.
Visa Service uptime is on its secondary line; healthy states do not repeat
"Available" or "connected". Independent green/red lamps distinguish Control Room
transport from Visa Service source health. Partial, unavailable, unconfigured
and snapshot-transport failures use the single secondary label **Unavailable**;
unavailable source uptime is hidden until a healthy snapshot restores it.
Detailed failures remain in the alert strip rather than the top banner.

Control Room groups Adapters, Actors, Services, Visas, Denials, and DNS under
counted Status tabs. The summary metrics stay on Map rather than repeating on
each status page. Map updates animate retained topology components as bounds
move; Fit centers the rendered topology with padding while preserving glyph
proportions, and reduced-motion preferences suppress movement animation.

Selecting an adapter in the Control Room map shows its current unexpired visas,
including inbound and outbound flows, protocol, expiry, node, and policy ID.
The panel refreshes with polling and queries the complete Visa Service list,
not the recent-ten snapshot. These are current grants involving the adapter's
ZPR address, not confirmations that its PH has installed each grant. Loading
and upstream failures are distinguished from an empty current-visa list.
Zero visa and buffered-denial badges are white-filled circles; `?` means the
count is unavailable. Click, right-click, or press Enter on a red node denial
badge to show the node's buffered and local denial telemetry plus recent Visa
Service denials whose source is an adapter docked on that node. The node exports
only a count, so these records are context and may not match it one-to-one.
Refreshes keep the last successful visa list visible, including after a failed
request; switching adapters still loads that adapter's own list. Visa flows in
the inspector and Visas table show matching DNS names alongside the original
addresses, including aliases from the configured `svc.zpr.` zone. The zone
records are cached and refreshed once per minute during normal polling;
unmatched addresses remain numeric, and DNS failures retain cached names.

The policy editor offers compact completions beside the cursor. Inside a
`define` statement's `with` clause, the same menu includes configured attributes
and their sources, including multiline clauses. Suggestions are suppressed
after terminating periods and inside comments, quoted strings, and attribute
values. Control-Space requests suggestions, arrow keys select, Tab accepts,
and Escape dismisses; Enter remains a newline and Shift-Tab moves focus out.

Source editors offer a **Find & Replace** button immediately before History:
policy/assertion, ZPR Config, Simulator scenario source, and directory LDIF. It
turns dark green while its dialog is open and closes the dialog when clicked
again. The dialog follows Microsoft Word's Find and Replace layout: **Find** and
**Replace** tabs, *Find what* and *Replace with* fields, and Replace, Replace
All, Find Next, and Cancel buttons. **More >>** reveals Search Options: search
direction (All wraps; Down and Up stop at the end or beginning), Match case,
and **Use regular expressions** (Word's wildcards are replaced by JavaScript
regular expressions). Control/Command-F opens the Find tab in the focused source;
Control/Command-H opens the Replace tab. Enter is Find Next, Shift-Enter
searches backward, and Escape closes the dialog.

Searches are literal unless regular expressions are enabled. Regular-expression
mode reports invalid patterns, ignores empty matches, and expands `$&`,
`$1`–`$99`, `$<name>`, and `$$` in replacements; literal mode keeps replacement
text literal, including `$`. The status shows only match counts and position
(for example, `Match 2 of 5`). Replace or Replace All is applied as one native
edit, so the editor's Undo/Redo (Control/Command-Z and Shift-Control/Command-Z)
restores it. Replacements change only the unsaved source, preserving each
editor's normal modified state and analysis invalidation. Nothing is saved,
published, or applied automatically; replacements exceeding an editor's
character limit are rejected.

ZPR Config uses the policy editor's syntax colors and toolbar styling. Its narrow
gutter contains only source-local error markers, not line numbers. Analyze shows
success/error state on its button, and File contains Save draft and local file
commands. An empty new draft has no redundant footer; actionable diagnostics and
save/load status remain visible. Saving configuration still never applies it to
the runtime.

The Simulator scenario dialog switches between its structured form and a
top-right raw-source editor toggle. Raw mode edits either JSON or YAML, using the
same dark syntax colors, error-only gutter, File/Analyze/Format controls, and
synchronized scrolling as Control Room's source editors, with a line/column
indicator and a responsive assistant column. Save and Publish store the same
canonical scenario definition regardless of the chosen raw format.
Analyze calls the read-only, organization-scoped
`POST /api/simulator/organizations/{organization}/scenario-check` endpoint. It
accepts `{format:"json"|"yaml", source}` and returns the canonical JSON/YAML
rendering when valid. It uses the normal scenario validator but never opens a
workspace, saves, publishes, or starts anything. Syntax/type errors with exact
source offsets appear in the gutter; definition errors without source locations
remain in the status area. Form and raw-source changes round-trip, and invalid
JSON/YAML blocks returning to the form rather than discarding the draft. File
import/download remains local and unsaved.

The same Claude assistant is available in both modes and uses the current form
or exact raw draft. Its disclosure explains what is sent to Anthropic; the
server still requires `ANTHROPIC_API_KEY`. Run
`scripts/configure-assistant.sh` from an interactive terminal to store it in a
0600 file under `.local-runtime/dashboard-stack/assistant/`; the existing
Control-Service container is restarted in place to load it, preserving its
LDAP, Admin API, diagnostics, enrollment and organization configuration.
Setup refuses to reload a service configured for a different key-file path.
Simulator assistant requests read the protected
file at request time. This avoids shell exports that disappear after a
terminal closes. Applying a proposal updates only the unsaved editor.
Responses/proposals for a changed draft or reset conversation cannot overwrite
newer work. Save, Publish, and Run remain separate explicit operations.

All editor and design assistants share `assistant-core.js`: ready-to-use
controls without an opt-in checkbox, initially collapsed **Model and max
tokens** preferences, the **How can I help?** prompt, bounded conversations,
usage counters, pending/errors, safe text rendering and pane collapse.
Text suggestions are undoable with Ctrl/Command+Z (with redo and a fallback
edit history when native editing is unavailable). Structured Scenario and
organization-directory Apply offer **Undo AI change**/**Redo AI change**,
restore unsaved drafts only, and reject history actions after conflicting
source, record or revision changes. Applying never saves, publishes, activates
or runs anything.

Policy/Assertion context consists of the selected source/group definitions,
configured trusted-attribute definitions and operator-written conversation.
It does not load a People catalog, individual user records, memberships or
Simulator state. A regression exercises both endpoints with unavailable
Simulator configuration. Directory and Scenario request payloads are unchanged.
In production, Control-Service obtains Policy/Assertion attribute definitions
from Policy-Service over its configured mutual-TLS connection rather than
requiring an in-process policy workspace. Only attribute definitions enter the
assistant prompt; unrelated repository records are not forwarded. Context
failures are reported explicitly and prevent the model request. ZPR Config
does not need an attribute catalog lookup.

Rescan LDAP, Format, Discard, Evaluate & Test, Save, Save As, and Compile & Stage
share a responsive control strip directly above the source editor. Evaluate &
Test runs ZPLC before the identity simulation. Save and Save As automatically
rerun the simulation; if it fails, the user must confirm Save anyway. Compiler-invalid
source can still be preserved, but cannot be staged. Compile & Stage recompiles
the selected saved revision on the server before creating a candidate.

The Control Room Services page uses distinct type colors for BuiltIn, Regular,
Visa, Gateway, trusted file, trusted REST, and services not in the current
policy. The upper-right **Type colors** dropdown shows the mapping and includes
stable colors for any additional reported types. Labels remain visible alongside
colors; the key stays open during polling and closes on outside click or Escape.
Map service badges use the same background, border, and text colors, including
while hovered or highlighted as arriving or exiting.

The IPv6 simulator policy also defines a ZPR `internet-gateway` service at
`[fd00:1:9::1]:8082`. Its identity uses a dedicated pre-generated bootstrap key,
and the trusted file service assigns its `public-internet` network label. The
Finance workload alone is allowed to use the service. The bundled
`internet-gateway-egress` scenario sends an HTTP health probe through ZPR to a
fixed `https://example.com/` upstream; it is not an arbitrary-host proxy. Rebuild
and restart the disposable Linux rig to load the updated signed policy and
gateway key before running this scenario.

In the Control Room map, each gateway is connected to a dark gray cloud
representing its external network. Hover over the cloud to see the declared
network label.

### Gateway draft API

Control-Service exposes `GET /api/gateways/contracts` to list live registered
Gateway services with a matching actor, and `POST /api/gateways/config/check`
to validate a JSON draft against one of those contracts. `GET /api/gateways/configs`
lists the active organization's saved drafts;
`GET /api/gateways/configs/{instance}` reads a draft and its revisions; and
`POST /api/gateways/configs/{instance}/revisions` saves a new revision with an
`expected_revision` compare-and-swap. Draft files are organization-partitioned,
private, and versioned. The Control Room **Gateways** page provides fields for
destinations, path prefixes, GET/HEAD methods, timeout, and response size, with
server validation before saving. All these operations leave runtime unchanged.
There is intentionally no Activate action yet.

The draft cannot set the external-network classification; that remains
policy-owned.

The Visa Service Admin API reports service kind from installed policy but does
not return the ZPL `external-network-connection` claim. Contract discovery uses
the policy-derived `Gateway` kind and a matching live actor; any response
external-network label is optional `ZPR_PLATFORM_SERVICES` display metadata,
not a verified ZPL claim. Do not use draft validation as an activation/egress
security boundary until Control-Service can verify the complete installed ZPL
contract and runtime DNS/egress checks are implemented.

## Policy Layers

Runtime policy is composed from three ordered source tiers:

1. `examples/policy-layers/bootstrap.zpl`: node, Visa Service, and essential
   bootstrap/administration grants.
2. `examples/policy-layers/platform.zpl`: shared simulator control, DNS,
   observability, and baseline fixture services/grants.
3. `examples/organizations/<id>/runtime-policy.zpl`: company application policy.

The composer always uses the same first two tiers and selects one company tier.
It rejects missing/empty tiers, duplicate sources, path traversal, symlink
escapes, and oversized bundles. This is source composition, not a new precedence
or authorization model: the normal compiler and evaluator still determine
policy semantics. The company runtime layer is separate from its Policy Studio
example catalog; Redwood's illustrative business endpoints are not deployed by
its login-only runtime scenario.

Generate a candidate without changing the running network:

```sh
go run ./cmd/zpr-web-dashboard -mode compose-policy \
  -policy-root cmd/zpr-web-dashboard/examples \
  -policy-organization velocity \
  -policy-output /tmp/velocity-runtime.zpl
```

Compile/sign the generated source with the complete runtime `.zplc` configuration
and the normal protected signing key, not the company's editor-only config.
Northstar, Redwood, and Velocity candidates have been compiler-checked. The
running monolithic rig policy is not replaced by this command. Layer-aware
activation must still coordinate actor/visa reset, company LDAP, Policy-Service
workspace selection, health checks, and DNS-data retention before live rollout.
Control Room consumes generic Policy-Service APIs; it must not call simulator
APIs to select a company or orchestrate reset.

## Simulator Policy Source

Control Room's Policy tree includes **Simulator / Runtime / Simulator runtime
policy** when the local stack configures `ZPR_POLICY_SOURCE_FILE`. This is the
simulator ZPL source, separate from the editable Northstar demonstration catalog
and from acknowledgement of a signed policy installed in Visa Service.

The configured source is imported even when the repository already has records.
Importer-owned records gain a revision when the file changes; identical reads
are idempotent. Manual edits to the imported record and unrelated policies are
never overwritten. `ZPR_POLICY_SEED_CATEGORY` and `ZPR_POLICY_SEED_NAME` control
its location. This view does not install or activate a policy.

## Log Views

Log panels retain each source's last successful bounded tail when an adapter,
controller, application, or service disconnects. The selected source remains
available, its panel turns white, and a disconnect/read-error status appears
without replacing the log text. Reconnection replaces the tail with fresh data
and restores the running style. Retained tails are browser-memory only, scoped
to the active organization, and cleared when organizations change or the page
is reloaded; they are not a durable archive.

Both maps show **Detached** and **Lost** text-button overlays in the map's
bottom-right corner, styled like Fit and zoom controls, each with an adapter count.
Click or keyboard-activate either group to inspect its members, addresses and
registered services, then open individual actor details. The overlays remain
fixed while panning/zooming and do not affect layout, SVG bounds or Fit.
Individual unattached adapters and their service shapes also remain outside the map.
Both groups remain visible with zero counts when inventory exists. Truly empty
inventory retains the empty state. Isolated nodes remain visible. Observed
attachments move adapters onto the map; detachments move them into **Lost**.
**Detached** means no attachment has been observed in this browser. **Lost**
means one was previously observed. History is browser-local and scoped to the
organization; an unobserved attachment is not proof of never having connected.
Actor details show the reported ZPR address, attachment status/node, registered
services and snapshot time. Repeated Admin inventory entries for the same common
name are resolved once by Control-Service, matching the Admin actor lookup key.
Control Room's **Adapter Logs** page shows independently configured production
adapter/controller log sources. Its read-only API passes through the
authenticated Control Service to the production collector, without reading
Simulator manifests or calling Simulator APIs.
Locally generated log panel titles use only the source name (for example,
`web0-adapter`); organization prefixes remain in internal IDs, not display titles.
File-backed sources may include a `runtime_container` identity for runtime-state checks;
logs still come from the configured file. **Running only** uses that container's
state, not the presence of retained log history. Files without an associated
runtime remain unknown and are excluded; unknown is never treated as running.
The **Adapter logs** and **Controller logs** buttons at the top switch log types
using the same horizontal panels. Each panel remembers its adapter/controller
selection and source scroll/follow state; changing types needs no additional
collection request. The machine/adapter selectors and add/remove panel controls
apply to the selected type.
The Simulator's **Workers** page shows passive device, controller, session and
selected workload status together with assigned application/service
event logs, not Controller or adapter logs. Running machines with no supported
workload logs have an explicit empty state.

Adapter/Controller Logs and Workers offer **Format JSON**, initially
unchecked. Enable it to indent complete JSON objects/arrays; disable it to restore
the exact raw log text. This is a local display change, including while paused,
and does not collect or modify logs. Plain text, prefixed messages, and malformed
or truncated JSON remain unchanged. Entries nested beyond 64 levels stay raw to
bound indentation growth. Formatting retains number tokens (including
large integers), duplicate keys, and escaped string contents. Wrap is independent.

Both views poll two seconds after each collection completes, without overlapping
requests. Pause/resume, manual refresh, search, running-only filtering, and
follow-tail controls operate independently of machine lifecycle actions.
Each machine window follows the newest entries by default. Scrolling back pauses
following for that window without affecting the others; scrolling to the bottom
resumes it. Re-enabling **Follow logs** resumes all windows. The Simulator's **Log**
radio controls switch between its workload sources, showing one at a time.
Selection survives polling, and
each source remembers its scroll position and follow state when switching back.
The selector also works in maximized windows; a disappeared source falls back
to the first available source.
Polling is cancelled when navigating away from the page. ANSI foreground and
background colors (standard, bright, 256-color, and RGB), bold, faint, italic,
and underline are rendered using the vendored MIT-licensed `ansi_up` 6.0.6
browser module. Log text is HTML-escaped, terminal hyperlinks stay inert, and
only approved color and text-emphasis styles are applied without relaxing CSP.

Trusted Sources **Manage** links are configured per trusted-service name, not
as one shared LDAP editor. The local launcher merges the operator registry
`dashboard-stack/provider-managers.json` with `ZPR_PROVIDER_MANAGER_URLS`;
explicit environment entries take precedence. Override the registry path with
`ZPR_PROVIDER_MANAGER_URLS_FILE` and restart Control-Service after updates.
Organization deployments provision their own loopback-only directory managers
and register links for their actual Visa-configured source names. Control Room
uses only the resulting operator configuration and does not query Simulator.
The browser authenticates directly to the manager with a separate LDAP admin
login; Control Room does not receive the LDAP admin password.

Control Room's **Nodes** page includes the selected node's production source
health, sortable source metrics and logs below its statistics. Health shows
source identity/address, state, last update and whether the displayed metric
sample is current, stale, partial or unavailable. Source metrics and Diagnostics
service metrics share `source-metrics.js`, including exact integer sorting
without rounding large counter strings and change pulses on refreshed values.
Select nodes from the horizontal tab navigation; each tab switches the node
summary, counters, source health/metrics and logs together. Arrow keys, Home
and End move between node tabs.
When the node tabs exceed the available selector width, they are replaced by a
node pulldown. Resizing or updating node names/counts reevaluates the fit.
Both selector modes keep the selected node and the second navigation's active
data area unchanged.
Within the selected node, a second horizontal navigation row separates
**Overview**, **Counters**, **Health & metrics**, and **Logs**. Only the selected
area is visible. Area selection survives node changes and snapshot refreshes;
returning to Logs restores its scroll/follow state without an additional request.
Nodes, Adapter/Controller Logs and Simulator Workers use the shared
`log-panel.js` component for panel construction, safe rendering, retained scroll
position, follow-tail behavior and standard Maximize/Restore icons.
Nodes uses the same black terminal surface, with a 360px scrollport in the normal
view and an expanded scrollport when maximized. Escape restores the panel and
returns focus to its window control; navigation restores and detaches the panel.
Each visited node remembers its scroll/follow state. Scrolling back suspends
following; scrolling to the bottom resumes it.

Node log bodies remain raw, with no Format JSON control or synthetic date/time/
severity columns. Provider ANSI styling resets independently for each record;
HTML and terminal links remain inert. **Wrap lines** is initially off.
The viewer loads on entering Nodes or selecting another node, then refreshes
with Control Room's successful snapshot refreshes. Its local **Pause/Resume**
controls the source health/metrics/log request, not node counters; **Refresh**
can query the source while locally paused. Leaving Nodes or changing selection cancels the pending request,
and obsolete successes/failures cannot affect the new view.
Initial Loading, empty logs, stale/partial/unavailable sources and request errors
remain distinct. Failed reads retain the selected node's last observed logs with
an explicit last-known warning, never another node's content. Healthy/stale
metric samples are retained independently from partial log samples, so a log
refresh cannot discard the last usable metrics or mislabel them as current.
Session loss clears retained node source data and cancels pending requests.

Nodes calls the independent production
`GET /api/diagnostics?source=node%3A<encoded-node-CN>` contract through the
authenticated Control Room/Control-Service boundary. The optional `source`
parameter accepts one inventory ID (at most 512 bytes). Unknown IDs return 404;
empty, repeated, oversized or malformed query parameters return 400. The
server resolves the source from production inventory before querying its
provider, rather than accepting caller-supplied provider identities. A selected
source is resolved before aggregate source/log caps, so unrelated sources cannot
exhaust its budget; the existing per-source bounds still apply.
The unfiltered Diagnostics API remains unchanged. Its Control Room page is
named **Services**, retaining `#diagnostics` for existing bookmarks (`#services`
already identifies the Status service-descriptor tab). Services shows only
non-node service/trusted-source telemetry: logs, freshness and errors.
The main Services page has a service selector and the standard shared
`log-panel.js` viewer, rather than separate log lists for every source.
Selecting a service displays its health and logs; the service
count and telemetry summary still cover the non-node inventory. The provisional
filter and metrics display have been removed pending a future real filter.
The API may return multiple service/trusted-source entries, but only the
selected entry's health and logs are displayed. Metrics remain in the backend
contract and Nodes; Services neither renders nor validates unused metrics.
Selection survives reordered
refreshes, and each service retains its own scroll/follow position.
The viewer provides Pause/Resume, explicit Refresh even while paused, Wrap lines
(initially off), and standard maximize/restore controls with Escape support.
Failed reads retain the last usable logs with explicit last-known warnings;
navigation cancels requests and session loss clears retained data.
This frontend-only selector change is verified locally and awaiting deployment.
It uses the existing aggregate Diagnostics API and its bounds, not a new
selected-service request or an expanded log budget. The Status service-descriptor
tab remains unchanged.
Node rows, logs and metrics remain exclusively in Nodes. Service counts and
the telemetry summary exclude nodes, even when node health changes the API's
overall aggregate state. Adapter Logs and Trusted Sources retain their own
dedicated log and provider-management workflows.
Rollout of this feature requires the Control-Service source filter as well as
the Control Room assets; changing Simulator or deploying unrelated enrollment,
review or gateway backend work is not required.
The health/metrics merger is a frontend-only change, deployed to Control Room
on 2026-10-10 at 00:50 UTC. Scoped rollouts must include the new
`source-metrics.js` asset together with the updated Diagnostics/Nodes assets
and index cache references. The node-source backend filter is already deployed;
this merger needs no additional backend or Simulator change.

Services log bodies use the shared viewer's `colored-log.js` renderer, like
Nodes, Adapter/Controller Logs and Workers, preserving provider-supplied ANSI colors
and emphasis instead of showing escape sequences as plain text. Each Diagnostics
record starts with fresh terminal styling, so color cannot leak into the next
record. Diagnostics displays provider text without JSON reformatting and has no
Format JSON control. Provider HTML
and terminal hyperlinks remain inert. No severity colors are invented and
plain messages use the standard black-background, white-text terminal display.
Diagnostics log entries show only the full-width body, without separate
timestamp or severity columns. Dates, times and levels already included in
provider text remain intact; no prefix is synthesized or stripped.
Diagnostics uses the available application width, including wide monitors.
Metrics span their selected-service detail panel. The standard log panel is
360 pixels tall normally, scrolls internally, and can be maximized.
The page scrolls vertically; narrow source tables can still scroll horizontally.
An initial **Loading** indicator is shown until the first request finishes.
Diagnostics does not display a recurring Querying/Updated status line;
background refresh retains existing content, with failures shown explicitly.

Each source returns at most 100 lines and 64 KiB. Collection runs for at most
12 seconds with four concurrent machines; empty and unavailable sources are
shown separately. Adapter Logs reads Docker controller output and PH log files;
Workers reads application/service JSON event files. Neither view generates
traffic or starts log-producing
workloads. Recognized credentials are redacted before sending logs to the
browser. This is a bounded live tail, not a durable audit archive.

## Trusted Data Assertions

Control Room's **Assertions** page edits a per-organization, report-only assertion
record in the Policy Repository, independent of ZPL compilation and access policy.
It evaluates live trusted LDAP membership data with
group cardinality, exact-one membership, mutual exclusion, and approved LDAP
attribute presence/value/integer checks. Manual draft
evaluation and opt-in periodic checks are available; source failures and missing
groups are errors, never successful checks. See [ASSERTIONS.md](ASSERTIONS.md)
for syntax, source configuration, API contracts, and current limits.
Source-located assertion lint warnings appear as clickable yellow `WARN`
markers on their source lines; clicking a marker opens its details. The Analyze
button turns yellow when a successful evaluation includes warnings. Warnings
without a valid source line remain visible below the editor controls rather
than being assigned an invented line.

## Organization Profile Drafts

The Simulator Organizations page's **IDENTITY**, **POLICY**, and **SERVICES**
links jump to the selected organization's directory, policies, and services
sections, respectively, with keyboard-accessible in-page navigation.

The Simulator Organizations page offers **Edit organization draft** for existing
bundled organizations. Name/description fields and advanced JSON edit the same
profile. **Save draft** persists an unpublished profile revision in the private
per-organization SQLite workspace; it never rewrites bundled profiles, edits
linked LDAP/policy/scenario documents, switches the active organization, or
changes running machines. **History** loads an immutable revision for review;
saving it creates a new revision rather than rewriting history.

**Validate profile** checks the accepted profile schema and structural rules.
It does not certify linked-document consistency or runtime readiness. This first
slice supports valid existing profiles, not incomplete new-organization drafts.
Unknown fields are rejected rather than silently stripped; the stable ID cannot
change. Save conflicts retain local edits; after an uncertain response, reload
and reconcile the saved revision before retrying.

The AI pane sends the unsaved fictional profile and conversation, with a visible
disclosure. Linked files and live runtime state are not included. A validated
proposal opens a before/after field review; only **Insert reviewed draft** changes
the editor. Undo/Redo remain local. AI never saves, publishes or activates, and
changed editor/revision context invalidates old proposals.
Small top-level `profile_patch` proposals are merged into the submitted draft
and validated as a full profile; unchanged definitions need not consume the
assistant's bounded output budget.

Organization-scoped routes:

| Method | Route suffix under `/api/simulator/organizations/{id}` | Permission |
| --- | --- | --- |
| GET | `/profile`, `/profile/revisions`, `/profile/revisions/{revision}` | `organization.read` |
| PUT | `/profile` | `organization.edit` |
| POST | `/profile/check`, `/profile/assistant` | `organization.analyze` |

PUT takes `profile`, a required positive `expected_revision`, and `summary`.
Revision author records the authenticated operator subject. Operators require
explicit new analyze/edit grants; existing read/activate grants are not expanded.
These routes are Simulator-only and are not exposed through Control Room.
These draft/editor slices were deployed to Simulator on 2026-10-11 20:18 UTC.
No source/runtime activation or production Control Room change is implied.
New/Import, bundled-definition duplication and coherent document/AI editing are
described below. Structured section editors, full reference/compiler checks and
reviewed activation of edited drafts remain planned in
[Organization editor plan](../Organization%20editor%20plan.md).

### New and Import organization drafts

Choose **New organization**, enter a stable ID, name and description, and
optionally a directory base DN. **Review proposed draft** displays the profile
and missing definition sections; confirm before **Create draft**. New drafts
have no fabricated sites, identities, addresses, directory or policy documents.
They are private, unpublished bundles, not runnable catalog entries.

Choose **Import organization** to load a local JSON file or paste a portable
definition bundle:

```json
{
  "version": 1,
  "profile": {
    "id": "fictional-org",
    "name": "Fictional organization",
    "description": "Private incomplete draft",
    "directory": { "base_dn": "dc=fictional,dc=test" }
  },
  "documents": []
}
```

The target ID must match `profile.id` and each scenario's `organization_id`.
IDs/base DNs/namespaces are not silently rewritten. Documents contain `path`,
`kind` and `content`: directory (`.ldif`), policy (`.zpl`), policy-config
(`.zplc`), policy-catalog (`.json`), or scenario (`.json`). Supplied profile
references must resolve to matching bundle documents. Paths must be safe,
unique and relative; import never extracts an archive or writes these paths.

Limits: 256 KiB per profile/document, 128 documents, 8 MiB encoded bundle.
Unknown schema fields, internal creation metadata and runtime-state document
kinds are rejected. Imported LDAP passwords/auth-passwords, certificates and
PKCS12 attributes are rejected rather than removed. Detected secret assignments
and private-key markers also reject imports. Catalog records must be accepted
policy/configuration/assertion definitions with matching content types;
metadata allows only `demo` and `live` flags. These checks cannot guarantee
arbitrary free-form text is secret-free: use fictional definitions only and
review all content before confirmation or explicit AI disclosure.
Loading/importing does not send content to an AI provider.

Incomplete private profiles remain saveable and editable with reviewed AI
proposals. **Validate profile** checks supplied schema, local references and
directory containment, returns `readiness_gaps`, and always reports
`runnable: false`. Missing sections are gaps; malformed supplied sections are
errors. Compiler certification, full scenario-reference checks and
activation remain separate future work. Existing bundled-profile validation
has not been relaxed.

**Export saved definitions** downloads this portable format without operation
IDs, request hashes or creation receipts. Save unsaved edits first; export
validates the saved document schema and rejects unsupported content explicitly.

Preview and creation require target `organization.create`, independently of
later read/analyze/edit grants. Confirmed requests include `operation_id`,
`reviewed_hash` and `confirmed`. Browser-tab retained operations support exact
retry and **Check creation result** after uncertain responses, including reload.
Changed retries and conflicting IDs never overwrite the original bundle.

| Method | Route | Permission |
| --- | --- | --- |
| POST | `/api/simulator/organizations/{id}/create/check` | target `organization.create` |
| POST | `/api/simulator/organizations/{id}/create` | target `organization.create` |
| GET | `/api/simulator/organizations/{id}/draft/export` | target `organization.read` |

This slice is deployed to Simulator (2026-10-11 20:18 UTC). No source/runtime
activation, production Control Room dependency, or live permission-grant change
is implied. See [deployment evidence](../GUI%20deployments.md) for the exact
release, approved read-only supporting definitions, persistent-draft checks and
stopped rollback. Structured editors and runtime certification remain pending.

### Coherent supporting definitions and AI editing

Open a private organization profile, then choose **Edit supporting definitions**.
Save any profile-only edits first or explicitly discard them when switching.
The full-definition workspace loads the saved profile and documents together;
it never silently saves the profile on navigation.

Use **Definition file** to select the profile JSON or directory/policy/compiler/
catalog/scenario source. **Add document** creates an unsaved local entry;
set its path, kind and content. **Remove document** requires confirmation.
Update related profile references before saving. Invalid local drafts remain
editable, but **Validate all definitions** and **Save all definitions** reject
unsafe, malformed, credential-bearing or inconsistent supplied definitions.
These use the same bounded portable-definition contract as Import.

**Save all definitions** appends one immutable profile/document bundle revision,
requires the loaded `expected_revision`, and preserves initial creation receipts
and provenance server-side. Conflicts do not overwrite newer revisions or erase
local edits. A failed/uncertain save instructs **Reload definitions** to reconcile
before another attempt. Draft checks never save and always return
`runnable: false` with explicit readiness gaps.

History loads the entire matching snapshot. Private profile history also opens
this workspace with the corresponding complete bundle; it does not mix an old
profile with current directory/policy content. Saving reviewed history appends a
new revision using the current expected revision, not the historical number.
Profile-only saves still preserve the current supporting documents.

The AI pane explicitly discloses that **Send** sends the entire unsaved fictional
profile, every supporting document and conversation to Anthropic. Check for
credentials first; free-form text cannot be guaranteed secret-free. No internal
creation receipts or observed runtime data are included. The existing 2 MiB
assistant request/context limit is enforced with an error, not silent omission.
Section-only context for larger organizations is still planned.

AI returns top-level profile patches plus complete document upserts/deletions.
The server merges these into the submitted unsaved bundle, preserving untouched
fields/documents, then validates the combined result. Incomplete sections remain
explicit gaps; it does not fabricate configuration to imply readiness.
**Review definition proposal** shows changed fields/documents before/after.
**Insert reviewed definitions** changes only local state. Undo/Redo restores the
combined change; source/revision changes invalidate stale responses or undo
attempts. **Save all definitions** remains a separate operator action.

| Method | Route suffix under `/api/simulator/organizations/{id}/draft/definitions` | Permission |
| --- | --- | --- |
| GET | root, `/revisions`, `/revisions/{revision}` | target `organization.read` |
| PUT | root | target `organization.edit` |
| POST | `/check`, `/assistant` | target `organization.analyze` |

PUT/check take `bundle` (portable version/profile/documents),
`expected_revision` (required for PUT) and `summary`.
Assistant requests use `scope: "organization-definitions"`, matching
`organization_id`, `bundle`, `messages`, `model` and `max_tokens`.
Responses expose portable definitions, not private operation receipts.
Scoped permissions remain independent; no live grants are automatically changed.

This is locally verified source editing and reviewed cross-document assistance,
not complete structured section editors, policy compiler certification, full
scenario/service/group reference checks or activation. Draft operations never
write bundled files, publish to other workspaces, run scenarios, provision
machines or depend on Simulator runtime availability. Production Control Room
and Control-Service remain independent.

### Duplicate organization definitions

On Simulator Organizations, select a bundled organization and choose
**Duplicate organization**. Enter a new stable ID, name and description, then
**Review proposed draft**. The preview displays the profile and sanitized
supporting definitions. Confirm review before **Create draft**.

The source is the bundled definitions, not unsaved profile/directory/policy
workspace edits. LDAP passwords/auth-passwords and user certificate/PKCS12
attributes are removed (including folded/base64 values). Unsafe paths,
symlinks, oversized files and detected credential-bearing configuration fail
explicitly. No enrollment credentials, approvals, live inventory, sessions or
run history are collected. Only explicitly organization-scoped bundled
scenario definitions are copied; unscoped legacy scenarios are not inferred.

The new profile and copied documents form a single unpublished revision in
`organization-drafts.db` beside the configured Simulator workspace databases.
This private draft catalog is separate from the runnable organization catalog.
Open a private draft for profile/AI editing, Save, Validate and History.
Each profile save preserves supporting documents in a new whole-bundle revision.
Supporting document editing is available through **Edit supporting definitions**,
described below. Policy compilation, full cross-document semantic checks and
activation of these bundles are not yet implemented.

The source base DN, desired node addresses, device/service identifiers and
policy namespace remain unchanged and are disclosed for review. Typed profile
file references become bundle-local references; copied scenarios bind to the
new organization ID. No textual namespace replacement occurs. Changing base DN
without matching directory edits fails validation. Never treat this copied
bundle as ready to provision or activate.

The creation operation and reviewed sanitized source hash are frozen and
retained in browser-tab session storage before sending. The initial immutable
bundle revision records the operation/request hash. A matching retry returns
the current bundle without re-copying source files or overwriting later edits.
Changed retries or conflicting organization IDs return 409. After a lost
response, **Check creation result** looks up the retained operation. A source
change requires a fresh preview; an unresolved response never silently generates
a new operation. Operation IDs are scoped to the target organization.
Creation and retry responses contain only operation/revision receipt metadata;
retrieving draft content requires a separate target `organization.read` request.
Logout clears browser data and ignores delayed responses; persisted server
drafts remain discoverable after authorized login and catalog refresh.

The Simulator organization viewer exposes **New organization** with New,
Import and Duplicate modes. Its **File…** actions keep editing scoped to the
selected profile or private draft: Rename and confirmed Delete are available
only for private drafts, while Restore is available only for bundled profiles.
Deleting sends the draft's current revision, archives that private bundle and
retains its revision history; bundled profiles, active organizations and
runtime state are not changed.

| Method | Route | Permission |
| --- | --- | --- |
| GET | `/api/simulator/organization-drafts` | `organization.read`; target-scope filtered |
| POST | `/api/simulator/organizations/{source}/duplicates/{target}/check` | source `organization.analyze` and `organization.read` |
| POST | `/api/simulator/organizations/{source}/duplicates/{target}` | target `organization.create` and source `organization.read` |
| GET | `/api/simulator/organizations/{target}/draft` | target `organization.read` |
| DELETE | `/api/simulator/organizations/{target}/draft` | target `organization.archive`; requires current `expected_revision` |
| GET/PUT/POST | `/api/simulator/organizations/{target}/draft/profile` and its `/check`, `/assistant`, `/revisions`, `/revisions/{revision}` routes | same read/edit/analyze split as the existing profile routes |

Preview takes `name` and `description`. Creation also requires `confirmed: true`,
`operation_id` and `expected_source_hash`. Preview/creation never writes bundled
files or invokes runtime operations. Draft profile AI sends only the unsaved
profile, not copied documents or runtime state, and validates proposed
bundle-local file references before offering insertion. Operators need explicit
target create/read/analyze/edit/archive grants; no live authorization
configuration is changed by this implementation.

## Organization Activation

Selecting a profile does not activate it. **Activate organization** opens a
warning naming the current and target organizations: activation resets the
simulated ZPR environment and can interrupt connections and workloads. Cancel
is focused by default; Cancel or Escape sends no activation request. Only
**Switch organization** approves the reset. If the active organization changes
while the dialog is open, cancel and review the new state before trying again.
The backend still requires scenarios to be finished/cancelled and machine users
to be logged out.

For `docker-multinode` profiles, switching/redeploying preserves generated node
identities, API credentials and the active Visa policy in an organization-scoped
persistent Redis directory. Legacy in-memory databases are checkpointed before
their container is removed. First activation installs the composed policy with
the deployed keys; later activations do not replace reviewed policy with the
base policy. Missing database state or partial/mismatched identities fail
explicitly. **Restore base**, not ordinary activation, authorizes a backed-up
policy/directory reset. Validation uses a temporary runtime rather than rewriting
live identity/configuration files. DNS is rebound to the current node container
on redeploy and its service records are checked before readiness.

The Simulator Organizations page shows Restore progress in a blocking modal,
including the current phase, rollback progress and **See Logs**. Background
controls are unavailable and Escape does not dismiss it; it closes automatically
on completion or failure. Activation uses the status above the catalog.
Both work for the already-active organization. Completion and failure remain
visible above the catalog until the next operation or
Simulator restart. Restore shows an immediate submitting status and disables
duplicate Restore actions.

**See Logs** is available while an operation runs or after it fails. It opens the
local reset log in a separate tab. The link disappears after completion or a return
to idle; a catalog refresh failure retains access to the last known operation.
Operation status is currently in memory: restarting Simulator clears it, but does
not delete the local reset log.

## LDAP Organization Graph

### Independent policy-test fixture configuration

Policy-Service's `GET /api/policy/test/fixtures` now requires explicit
`ZPR_POLICY_TEST_FIXTURES_FILE` metadata plus its configured live LDAP reader.
The strict version-1 JSON contract supplies directory mappings, identity
authority, device inventory/ownership and service test inputs. Missing, invalid
or context-mismatched configuration produces an unavailable error; production
never reads Simulator manifests or profiles. Direct `POST /api/policy/test`
remains independent and accepts caller-supplied fixtures.

Simulator provisioning exports its fictional metadata to an atomic protected
file in the launcher's `policy-test-fixtures/` directory before stopping the
previous service. Export failure preserves the previous file and running service.
Deploy the updated Policy-Service executable and regenerate its context to use
this contract; a source change alone does not update the running service.
See the [fixture input contract](cmd/zpr-web-dashboard/README.md#independent-fixture-configuration).

### Independent assertion seed configuration

Policy-Service accepts optional `ZPR_POLICY_ASSERTIONS_SOURCE_FILE`, an explicit
UTF-8 assertion source file (up to 1 MiB, with at least one valid rule). Missing,
empty or invalid explicitly configured files fail workspace initialization.
Without this setting, a new production workspace starts with empty assertions;
it never discovers defaults through Simulator profiles or manifests.
Configured defaults fill an empty assertion record, preserving its schedule and
all nonempty operator-authored rules.

The Simulator provisioning launcher exports its selected fictional defaults to
an atomic file under `policy-assertion-seeds/` and supplies this production input
contract. Policy-Service does not receive Simulator environment variables.
Organization context restarts retain the currently deployed Policy-Service image
instead of reverting to an older Simulator default. Stopping the service saves
its image reference in `policy-service-image` under the launcher state directory,
so separate Restore stop/start commands retain it after container removal. Set
`ZPR_POLICY_SERVICE_IMAGE` to override that image explicitly.

The simulator's Organizations page uses the reusable `<ldap-org-graph>` custom
element from `cmd/zpr-web-dashboard/static/ldap-org-graph.js` and its scoped
stylesheet `ldap-org-graph.css`. Load both assets and assign a directory profile:

```javascript
const graph = document.createElement("ldap-org-graph");
graph.directory = {
  base_dn: "dc=example,dc=test",
  departments: [{ name: "Engineering" }, { name: "Platform", parent: "Engineering" }],
  people: [{ uid: "alex", name: "Alex", department: "Platform", title: "Engineer" }],
  groups: [{ name: "Operators", members: ["alex"] }],
};
container.append(graph);
graph.addEventListener("ldap-node-select", (event) => console.log(event.detail));
```

Each instance owns its camera, search, and selection. Reassign `directory` to
replace the data. Department names are unique parent keys and group members use
person UIDs. Missing parents and cyclic department references attach to the
directory root. The +/- control on a directory, department, or Groups branch
collapses or expands its descendants and compacts the layout. Expand all and
Collapse all operate across branches; Left/Right arrow keys collapse or expand
the focused branch. Selection and search survive branch changes.
Clicking a node opens a component-info popup with its profile attributes,
including structured values. Close it with the close button, Escape, or a
click outside the popup; focus returns to the selected node.
People list their groups, and groups list member names and UIDs. Selecting a
person or group displays dashed membership edges to visible nodes; memberships
remain listed when related branches are collapsed. Node labels and attributes
are text, not interpreted HTML. The graph
shows organizational relationships from profile data, not a live LDAP query or
an LDIF parser, and does not imply that people are physically stored beneath
their department DN. It supports keyboard node selection, pan, zoom, and fit.

## Browser Access Gateway Prototype

The optional gateway is a loopback-only development prototype. It routes
`control.localhost` to `127.0.0.1:8787` and `simulator.localhost` to
`127.0.0.1:8788`; upstreams must remain loopback origins. LDAP and OpenObserve
remain separate local-only tools.

With the default hostnames and port, use `https://control.localhost:8443` for
Control Room and `https://simulator.localhost:8443` for the Simulator.

The prototype requires a server certificate/key and a dedicated browser-client
CA. Do not reuse the machine-control CA. Start it on loopback for controlled
development without restarting the rest of the stack:

```sh
ZPR_ACCESS_GATEWAY_TLS_CERT_FILE=/path/to/gateway.crt \
ZPR_ACCESS_GATEWAY_TLS_KEY_FILE=/path/to/gateway.key \
ZPR_ACCESS_GATEWAY_CLIENT_CA_FILE=/path/to/browser-client-ca.crt \
scripts/dashboard-stack.sh start-browser-gateway
```

The listener defaults to `127.0.0.1:8443`. Browsers must trust the server
certificate and present a client certificate issued by the configured client
CA. This authenticates a client certificate, not a named user; the prototype
has no OIDC, role authorization, user sessions, or per-certificate revocation.
Do not set `ZPR_ACCESS_GATEWAY_LISTEN` to a non-loopback address or publish an
administrative UI. Public access is deferred until the [Remote Browser Access
Contract](BROWSER_ACCESS_CONTRACT.md) is implemented and its release gates pass.
Use `scripts/dashboard-stack.sh stop-browser-gateway` to stop the local
prototype independently.

The Simulator opens on Agents. Shared ZPR actors, links, and service inventory
remain in Control Room; Agents provides per-machine controls without stack-wide
lifecycle buttons.

The local stack also starts the BIND 9 DNS container from `../dns/bind9` using
the adapter1 ZPR network namespace. It expects the publisher TSIG key and
`svc.zpr` zone file under `.local-runtime/dns-bind/`. BIND's HTTP statistics
channel is loopback-only in the adapter namespace. Simulator's internal relay
is not a Control Room statistics origin.

Control-Service requires an explicit `ZPR_DNS_STATS_URL` for an authorized
production statistics origin. For this local Docker deployment, an independent
operator-owned relay can instead read BIND's loopback listener. Create
`.local-runtime/dashboard-stack/dns-stats-relay.json` with an explicit operator
source, for example `{"source_container":"operator-dns-node"}`. That container
must share BIND's network namespace and contain `socat`; do not derive its name
from Simulator's active organization or profiles.

Run `sh scripts/control-dns-stats.sh start /path/to/dns-stats-relay.json`.
The relay has its own image, restart policy and internal Docker network
(`zpr-dns-stats-read-only`), with no published ports or Simulator mounts.
On the next Control-Service start, `dashboard-stack.sh` reads this operator
configuration (or `ZPR_DNS_STATS_RELAY_CONFIG_FILE`), ensures the relay is ready,
connects Control-Service to the private network, and uses
`http://zpr-dns-stats-relay:8054`. An explicit `ZPR_DNS_STATS_URL` takes precedence;
without either configuration, statistics remain explicitly unconfigured.
The relay persists independently of Simulator UI shutdown/replacement.
Use `sh scripts/control-dns-stats.sh stop` before changing its source.

This local administrative transport uses the Docker socket, which grants
host-level Docker privileges despite the read-only container filesystem.
It is not an authenticated ZPR monitoring adapter and is not a production
deployment recommendation. Production should use an authorized ZPR-connected
origin as described in [BIND deployment requirements](../dns/bind9/README.md).
BIND TCP/8053 must never be published on the host/underlay.

The standard simulator's Exercise action resolves each component's FQDN target
through the bootstrap DNS server in the manifest, then pings the returned ZPR
address. Provider aliases are reconciled into `svc.zpr` by Visa Service using
the configured TSIG publisher key.
Use `scripts/dashboard-stack.sh start-dns` or `stop-dns` to manage DNS without
restarting the simulator's machine fleet.

The simulator defines 20 Docker machine profiles but initially creates only
`zpr-machine-01`, from the `debian:trixie` image. Starting another machine in
a scenario creates it on demand. Each running container has its own
Linux/ARM64 machine controller and PH adapter with a unique bootstrapped ZPR
identity. Machine adapters use dynamic `fd5a:5052::/32` addresses. In multinode
organizations the simulator-control provider also receives a dynamic address;
its policy provider matches the authenticated adapter identity, not a requested
fixed address. Controllers reach TCP `SimulatorControlService` on port `8792`
at the address actually assigned to the control adapter, over ZPR.
The service is published by the dedicated `simulator-control` actor and
only the 20 machine identities are authorized to access it. The service
gateway forwards the mutually authenticated TLS protocol to the simulator's
local listener; the ZPR adapter link and per-machine TLS client certificate
are both required.

Organization switching stops and verifies all running managed machine containers
before removing the previous network. Machine software/credential assets are
prepared under `dashboard-stack/machine-runtimes/<organization>/<generation>`,
not overwritten in one shared workload directory. Running containers carry both
scope labels and cannot be reused for another organization or generation.
Controller heartbeats, command polling and results must carry the current
organization/generation; stale controllers are rejected before changing state.
Manual machine mutations are blocked during activation, and provisioning scripts
share an exclusive runtime lock. A lock left by a forcibly killed operation is
not automatically broken: inspect `dashboard-stack/machine-runtime.lock` and
confirm no mutation is running before removing its token/pid files and directory.

Activation requires a disposable machine at every configured node to send a
heartbeat and acknowledge an actual command over the ZPR/mTLS path. Each probe
is stopped and removed; success is not inferred from synchronized nodes alone.
Readiness and machine launch share the same placement resolver, including
single-node profiles without explicit machine ownership mappings. Candidate
coverage is checked before stopping the previous runtime; uncovered nodes or
invalid placement fail explicitly instead of selecting an arbitrary node.
The machine controller retains its reserved `tun5` interface; Internet Gateway
uses `zgateway0`, independently of controller and telemetry tunnels. Workload
startup must not reuse the controller's interface or disrupt its command path.
Workload routing tables are explicit numeric IDs (100-105), not derived from
interface names; Gateway retains table 105 with its distinct `zgateway0` interface.
Simulator machine images include the system CA bundle for Gateway HTTPS upstream
verification; missing trust roots must not be worked around by disabling TLS checks.
Velocity's 100 warmed requests and exact 16 MiB transfer share a bounded
60-second benchmark deadline (inside the scenario's 75-second step deadline).
There is no separate 10-second body deadline that would prematurely reject a
valid slower transfer. This does not change transport keepalive timers or
introduce a minimum throughput requirement.
Simulator machine controller/workload adapters explicitly use the supported
`posix_unbatched` packet-I/O engine. Native ARM64 Docker testing reproduced an
`io_uring` receive-worker stall during the benchmark: its substrate receive ring
filled, kernel UDP drops increased and dock keepalive failed despite zero
application backpressure counters. Selecting POSIX for these adapters completed
the exact transfer without changing queues, policy or transport deadlines.
This is a Simulator launch choice, not a change to production PH defaults.
Rollback performs the same controller check. Trusted-feed scope is
recalculated for the restored organization so
Control-Service does not join a removed, other-organization feed network.
The one-time
`dynamic-control-policy-v1` migration recompiles and installs the composed runtime
policy to remove the historical fixed-address provider constraint. Directory
data and editor drafts are retained, but separately installed runtime-only policy
changes must be exported before this migration. Later switches preserve installed
policies rather than repeating that migration.

On Apple Silicon, multi-node Simulator infrastructure can run entirely as
Linux/ARM64 instead of relying on Docker Desktop's x86 emulator. See the
[native build instructions](../../zpr-demo/multinode-demo/README.md#how-to-deploy--run-zpr-local-docker-env)
for the separate ARM64 artifacts and images. Configure
`ZPR_MULTINODE_PLATFORM=linux/arm64`, `ZPR_MULTINODE_IMAGE`,
`ZPR_MULTINODE_DIRECTORY_IMAGE` and `ZPR_MULTINODE_DIRECTORY_MANAGER_IMAGE`
in the Simulator container environment so switches and rollback consistently
use native images. Simulator recreation/restart preserves those settings unless
explicit host environment overrides are supplied. The trusted-service helper
and control proxy follow that
platform; a cached helper with the wrong ELF architecture is rejected.
For a complete stop/start that removes the Simulator container, save the
same `ZPR_MULTINODE_*` settings (and any explicit trusted-service binary path)
as a JSON object in `dashboard-stack/multinode-platform.json`. Startup loads
this operator configuration without executing it; unknown keys, non-string
values and corrupted JSON are rejected. Explicit environment settings take
precedence. Production services do not load this Simulator configuration.
The existing amd64 configuration remains the default. Control Room has no
dependency on this Simulator configuration.

Simulator mode runs in the Linux `zpr-simulator` container, built from source
with a containerized Go toolchain. Its UI is published on `127.0.0.1:8788` and
its mTLS machine-control listener on `8791`; the Docker socket is mounted so it
can manage machine containers. Use
`scripts/dashboard-stack.sh restart-simulator` to rebuild and replace only
Simulator mode without resetting the active ZPR rig or organization.

On each machine start, the Simulator installs a host route for that machine's
assigned ZPR address through the control adapter. This avoids ambiguous return
routing when multiple adapters advertise the same overlay prefix. Startup
also removes stale link-down ZPR routes. The simulator-control adapter uses the
Visa Service's default four-hour adapter authentication lifetime; if its actor
expires, refresh only that adapter with
`scripts/dashboard-stack.sh restart-simulator-control`.

Machine Login/Logout writes/removes the selected simulated user in that
machine container at `/run/zpr-simulator/user`. It does not start or stop
workloads. User choices come from the active Organization's people and
machine-owner assignments; organizations without overrides retain the
manifest-owner behavior. The same rule applies to scenario logins. Sessions
are simulator state and reset when the stack restarts. This is not a
password check or ZPR user authentication: controller mTLS is a separate
machine identity, and the existing BAS auth-code flow authenticates an adapter
key rather than a human.

Workers is read-only: its expandable device details show owner, posture,
container/controller state, authenticated user and selected workload identities.
The former Agents page redirects to Workers at `/machine-logs.html`, and the
navigation has one Workers entry rather than separate Agents and Workload Logs.
Word wrap is initially disabled and can be enabled in editors and log viewers.

All six source editors register a shared `editor-page.js` controller with
explicit load/save/render adapters and Analyze adapters where supported.
Scenario form/raw modes share one controller; Assertions reuse Policy's
File/History controls without duplicating their listeners. Controllers own
identity/status, scoped analysis ownership, command and Save-shortcut bindings,
and responsive source-frame setup. Domain-specific service contracts,
cancellation, diagnostics, confirmations and mutation handling remain separate.
Disposal invalidates requests, removes owned listeners and releases layout
observers; obsolete request cleanup cannot release a newer operation.
All six source editors share Ctrl+S / Cmd+S handling through `editor-page.js`.
Their title rows also use its shared identity renderer for Untitled, safe title
text, path tooltips, and Modified visibility. Each editor retains its own dirty
calculation and draft naming; version information remains in History.
Discard, record/history switching, and editor-close checks use a shared
confirmation guard. Clean source does not prompt; cancellation retains the
current draft. Warning text and reset actions remain specific to each editor.
File menus also share one controller: opening focuses the first visible, enabled
action; arrow keys wrap through available actions and Home/End select the first
or last. Hidden and disabled actions are skipped. Escape closes the menu and
restores focus to File; selecting an action or clicking outside closes it.
Menu arrow keys do not intercept typing/navigation elsewhere in the editor.
History uses the same shared dropdown and safe version renderer in all six
editors, including singular/plural counts, empty histories, current-version
markers, outside-click dismissal and Escape focus restoration. Each editor owns
its revision-loading callback and discard checks; selecting another editor's
version never also invokes the Policy revision loader.
Policy, Assertions, Config, Gateways and Scenario also share explicit Analyze
button-state transitions (idle, pending, success and error). The helper does not
change readiness classes, enable actions, create diagnostics or interpret
service failures. Request ownership and stale-response rejection remain with
each editor. Scenario's pending state can mean edits need analysis, not a request
in flight. Directory has no Analyze action.
Non-line editor errors appear below the control buttons and above the source
surface in all six editors, not below the document. Source-local diagnostics
remain in the gutter. Shared line validation accepts only positive integer
locations within the current source; missing, fractional or out-of-range
locations are never clamped onto a real line. Assertion service failures stay
visible in the error status instead of acquiring a line-1 marker.
Config Analyze/validation-before-Format, Gateway and raw Scenario analysis use
a shared generation/context guard.
Editing and then restoring the same text still invalidates the old request.
Late successes and failures cannot update validity, status or gutter markers
after the source or domain-owned record/revision/organization/format context
changes. The guard has no service dependencies and never retries mutations.
Config Save and other mutations retain their existing error handling; analysis
guarding does not hide mutation failures or imply a failed save succeeded.
Assertion evaluation also uses this guard alongside its existing abort
controller. Source edits (including Tab and catalog insertion), record changes
and organization/revision context changes prevent late results from populating
the current gutter, result table or cached record result.
Assertion Format uses the same source/context guard and a separately owned
abort controller. Late formatted text, warnings and errors cannot replace or
decorate another draft, including after edit-then-restore. Switching records
or leaving the editor cancels Format; obsolete cleanup cannot unlock a newer
operation. Current formatting is an undoable unsaved edit and dispatches normal
source input so analysis and assistant suggestions are invalidated.
Policy compiler checks also use the shared source/context guard, covering
record kind/draft identity, current and browsing revisions, and organization.
Source edits and context replacement cancel the compiler request; only its
own controller can clear pending state. Obsolete successes, warnings and
failures never attach to the current source or start runtime testing. Save
and Save As stop on cancelled checks while retaining the existing explicit
save-with-errors flow for current compiler failures. Runtime testing uses the
same context adapter with its own shared scope and abort controller. Both
fixture loading and evaluation reject obsolete successes/failures before
starting another request or updating dimensions, results, warnings and errors.
Source/record/revision/organization changes cancel both analysis phases;
obsolete pre-save cleanup cannot unlock a newer Save test or cache cancelled
tests as current failures. Cancelled tests stop Save, Save As and Stage
continuations. Current test failures retain explicit warnings and the existing
save-anyway confirmation. Production service contracts remain unchanged.
Structured Scenario analysis uses the shared generation/context guard with
the exact serialized form, scenario identity/revision, organization, viewed
revision and editor mode/open state. Form edits and replacement rendering
invalidate earlier requests, including edit-then-restore and identical
replacement forms. Only the newest Analyze may update status; closing and
reopening the editor cannot resurrect an old response. Raw Scenario checks
also include the viewed revision and editor open state. These adapters retain
Simulator-only endpoints and do not change Save, Publish or Run contracts.
Shared viewport sizing across all six editors returns an idempotent binding per source
container. Its `schedule()` coalesces layout requests into one animation frame
and skips hidden containers; `dispose()` cancels queued work, disconnects the
resize observer, removes window listeners and restores the original inline
height and priority. A disposed container may be bound again. The existing
viewport inset and minimum-height rules are unchanged; the binding has no
service or editor-domain dependencies. Controllers reschedule when frames open,
mode changes alter visibility or the viewport resizes; maximized frames use
their full-page sizing instead. Scenario and Directory remain nonmodal pages.
The shared editor-page foundation supplies Maximize/Restore to Policy,
Assertions, Config, Gateways, Scenario, and Directory editors. It fills the
viewport, keeps focus within non-modal editor pages, and locks background
scrolling. Scenario and Directory dialogs remain open when Escape restores
their normal layout. The maximized editor exposes modal semantics and makes
surrounding content inert; restoring returns focus to the toolbar control and
preserves any pre-existing inert states.
All editor and adapter/controller/Worker log window controls use the same
dependency-free SVG renderer in `safe-display.js`: an outlined window for
Maximize and overlapping windows for Restore, without visible button text.
The renderer also marks each window button for shared styling in `app.css`:
dark icons on a white 32px square, with identical hover, active, disabled and
keyboard-focus treatment across editor and log toolbars.
Action-specific accessible names, tooltips and `aria-pressed` remain available;
keyboard and focus behavior are unchanged. This applies to window sizing only,
not restoring archived records, revisions or organization base state.
All six source editors share `bindSourceLayout` for highlight-overlay scroll,
native gutter scrolling or translated result rows, resize observation and
horizontal-overflow detection where supported. Policy and Assertion retain
their own gutter bounds and diagnostic/highlight rendering; Policy retains
completion positioning on scroll. Assertion scroll/resize no longer rebuilds
highlighted token nodes. The returned binding (also exposed through source
surfaces) can disconnect scroll/resize wiring with `dispose()`; disposal
leaves rendered content and diagnostics intact.
Policy and Assertion gutters retain one row per current logical source line
after edits, including newline insertion/deletion, undo/redo and wrapped lines;
editing clears obsolete analysis markers, not the gutter layout.
The shared `ZPREditorPage.editText` helper owns undo-aware programmatic edits.
Trusted Source assertion insertion, policy completions, Tab indentation,
Find & Replace and AI text insertion use the same native input path (with the
existing undo/redo fallback when native editing is unavailable). These edits
invalidate stale analysis and remain unsaved; editor read-only and size limits
are enforced before changing text.
The dependency-free `page-runtime.js` supplies neutral JSON transport and page
polling/lifecycle helpers. It has no endpoints, authentication defaults,
organization lookup or Simulator dependencies. Load it before editor-page,
assistant and page scripts. The existing editor JSON and Simulator navigation
poller entrypoints delegate to this runtime, retaining their public contracts.

`createPoller` requires `run` and an explicit `onError` callback. It shares an
in-flight Promise between refreshes, aborts on stop/pause/pagehide, and supplies
`signal`/`isCurrent` so late successes or failures cannot update another request
generation. Only the owning operation releases pending controls. Timer-based
reads wait the configured interval after completion; paused pages still permit
manual Refresh. `dispose` removes the pagehide listener and prevents restart.
Control Room snapshots, active-page DNS statistics, Diagnostics, Security scans,
adapter/Worker logs and manual Trusted Sources reads use this lifecycle.
DNS names remain a separately cached, page-independent production read so other
operator pages can resolve addresses without depending on the DNS route.
Security retains background inventory alerts while telemetry reads remain
active-page-only. Policy context checks use a separate navigation-owned scope.
Sample-age displays and animation timers are not network pollers.

Failed snapshot, DNS, Diagnostics and log reads retain last-good data with
explicit unavailable/error states; logs retain disconnected tails. Directory
reads retain last-known records, and normal snapshot polling/navigation does
not automatically reread LDAP or collapse its expanded tree. Change-feed
cursors remain source-owned. Failed/expired feed reads hide unconfirmed history
and keep the feed identity/error visible, rather than presenting it as current.

`requestJSON` receives the caller's fetch function and request options. Production
reads inject authenticated operator fetch; Simulator callers inject their own
fetch and endpoints. It preserves headers, bodies and abort identity, reports
malformed JSON with HTTP status, retains structured service errors, and makes
one attempt only. Assistant, Activity, policy browsing and policy File operations
also use it. Enrollment keeps its stricter content-type, status and uncertainty
rules, reusing only JSON decoding after those checks. Scenario Delete retains
its potentially empty success response. Operator login/session handling and
HTML navigation fetches retain their specialized contracts. No write is retried
or implicitly saved, published, activated or run by these helpers.

Config, Assertions and Gateways share `requestJSON`, with each adapter injecting
the existing authenticated operator fetch function and its own endpoint/options.
The helper preserves request bodies, headers and abort signals, never retries,
and retains structured service-error details and source locations. Malformed
JSON reports an explicit HTTP-status error rather than becoming an empty success
response; network and abort errors retain their identity. Gateway validation
explicitly accepts `valid: false` analysis responses, but failed saves remain
errors and leave the unsaved draft intact. Authentication, CSRF and mutation
uncertainty handling remain owned by the existing transport and editor adapters.
Simulator Directory now uses the same injected JSON helper for opening artifacts,
listing/loading revisions, saving drafts and publishing. Its adapter injects
Simulator fetch and keeps Simulator directory endpoints local to that page;
Control Room never imports those contracts. Malformed responses leave the
existing editor source/revision state intact and are shown through the existing
operation-specific error surfaces. Save and Publish remain single attempts,
and publishing still only affects the next explicit LDAP reseed or rig restart.
Scenario form/raw Analyze, artifact/history/revision reads and Create/Save/Publish
also use the shared injected JSON transport through a Simulator-local adapter.
Raw analysis maps structured service errors to validated source-line markers
only after checking request ownership; malformed JSON remains a visible,
unlocated error. Existing form/raw stale-context guards remain independent of
transport decoding. Failed writes leave the draft and revision identity intact,
never retry automatically, and never publish or run implicitly. Catalog polling,
Delete and Run retain their separate response/lifecycle contracts.
Policy compiler checks and runtime fixture/evaluation requests use the shared
JSON helper through authenticated operator fetch with their existing abort
signals. Compiler failures preserve diagnostics-before-error message priority
and source-owned warnings even on non-success HTTP responses; a non-success
response cannot become valid merely because its body says `valid: true`.
Runtime fixture and evaluation failures retain distinct error surfaces.
Malformed responses create explicit unlocated errors without invented markers.
Context guards and request-owned pending cleanup still prevent obsolete results
or cancelled Save/Save As/Stage continuations from changing current state.
Policy workspace, record, History and revision reads also use this authenticated
transport. Load errors appear in a dedicated status above source, separate from
compiler/runtime results; structured load errors never receive source markers.
Failed reads do not replace source text, and successful recovery or a new draft
clears the load status. Existing History selection and dirty-confirmation rules
are unchanged. Load-request lifecycle consolidation remains separate.
Policy revision Save, draft creation through Save and confirmed Stage also use
the authenticated JSON helper. Existing expected-revision bodies, Save error
dialogs and explicit Stage confirmation remain unchanged. Undecodable mutation
responses never advance local revision/draft state or trigger automatic retries;
they do not prove that the server did not apply the operation. Other record
management mutations retain their separate transport contracts.
The shortcut invokes the editor's existing enabled Save action, including its
name/revision checks and confirmations; it never publishes, activates, or runs.
Disabled Save still suppresses the browser's Save Page action. Held-key repeats,
IME composition, already-handled keys, and Alt/Shift-modified shortcuts do not
trigger Save. The Scenario shortcut also works in its structured form.
Start/stop, login/logout and workload assignment are controlled
by scenarios rather than page buttons. Selections are simulator control state
and a workload can be selected on only one logged-in machine at a time. Login/logout
and workload start/stop commands are queued by the simulator and executed by
the machine controller over its ZPR service connection; PH workload adapters
run inside the selected machine container. The selected workload remains
stopped until a scenario starts it. Docker container Start/Stop
remains a local host operation because an offline container cannot receive a
ZPR command. Stopping a machine stops its workloads and clears its
simulated login session; starting it restores its route but does not log a user
in or restart workloads. Controller connectivity is shown separately from
Docker state.

For local LDAP schema and seed changes, the runtime consumes
`../../.local-runtime/linux-integration/pregen/zpr-machine.schema` and
`demo-machines.ldif`; the 20 profile definitions are in
`../../.local-runtime/simulation-environment.json`.
## Control Room and Policy Service

The browser Control Room lives in `cmd/zpr-web-dashboard` and calls only
same-origin `/api/*` endpoints. The Control-Room process proxies those requests
to Control-Service using mutual TLS and holds no upstream API credentials.
Its topology draws only actors returned by the live admin API; service
registrations remain service badges attached to their provider actor, and a
missing provider does not create a synthetic adapter vertex.
Control-Service aggregates Visa Service Admin data and proxies Policy
Repository requests to a separate Policy-Service process using mutual TLS.
The Policy-Service owns its SQLite journal and ZPLC configuration. It stores
generic categorized records with immutable revisions; ZPL records are
compiler-checked before a version is written, and are not automatically
installed into the running Visa Service. Per-user policy-record authorization
is not implemented yet.

In the ZPL service model, attribute providers, authentication, logging/audit,
and the Policy Repository are distinct trusted-service classes with separate
REST contracts. The trusted-source view associates providers with actors and
reports the latest real lookup outcome when available; it does not browse
provider records.

Foundational services are registered during the extended node boot through the
Visa Service adapter. The boot passes their names in `ZPR_ADAPTER_SERVICES`;
the adapter includes repeated `zpr.services` claims in its real
`authorize_connect` request. Visa Service then persists those services against
the adapter actor and the Control Room reads them from `/admin/services`.

The simulation environment declares machine and runtime components in
`.local-runtime/simulation-environment.json`. Organization profiles under
`cmd/zpr-web-dashboard/examples/organizations/` own the LDAP base DN and seed
files, people, groups, machine ownership, policy compiler config/catalog, and
service catalog. Scenarios reference an `organization_id` and may declare an
independent `topology` of nodes, links, and component overrides. The active
profile is selected at stack startup with `SIMULATION_ORGANIZATION_ID` (or the
manifest's `organization_id`); it defaults to `northstar`. The Organizations
page includes an organization-scoped LDAP seed editor. Scenario definitions,
directory LDIF drafts, their immutable revisions, and existing policy records
share the per-organization SQLite file under
`.local-runtime/dashboard-stack/policy-private/`. The scenario editor provides
form-based metadata, repeatable step and cleanup controls, optional parallel
dependencies, advanced JSON for topology-specific fields, and virtual folders
grouped in the catalog; scenarios without a folder appear under **Unfiled**.
The run panel offers **Clear** only for a terminal run, and clearing removes
that run history without deleting its scenario or changing machine logs.
Drafts are validated and versioned; only published
scenarios can run, and each run records the organization and published revision
it used. Deleting a scenario archives it from the catalog while retaining its
immutable revision history. Directory edits are drafts until explicitly
published. Publishing stages a private LDIF at
`.local-runtime/published-directories/<organization>.ldif`; the running
directory is unchanged, and the new seed applies on the next explicit LDAP
reseed or runtime restart. All bundled organization profiles now use isolated
Docker Compose projects; single-node profiles keep one node, while Great Lakes
and Redwood retain their multi-node topologies. Activate another profile
through the Organizations page so its directory, policy, and services are
reseeded together. Set `SIMULATION_MANIFEST` to use a different machine/runtime
manifest.

Multi-node activation, rollback, stack startup, policy upload, and observability
discover the Visa admin host port from the running `<organization>-vs`
container's published `8183/tcp` binding. They do not assume every organization
uses host port 8185: the runtime may publish Great Lakes or other profiles on
8186 or a custom port. Missing, stopped, ambiguous, or invalid local bindings
fail explicitly instead of falling back to another organization's endpoint.
Run `node --test scripts/multinode-admin.test.mjs` for endpoint regressions.
This discovery belongs to Simulator provisioning only; production operator
services still receive an explicit independent admin URL.

The Simulator's Claude design assistant can review scenario drafts and
organization profiles. It keeps `ANTHROPIC_API_KEY` server-side and only applies
validated scenario or LDIF proposals to the open draft; saving and publishing
remain explicit operator actions. Scenario structured responses require every
schema key; optional scenario values use empty strings, arrays, `false`, or `0`
to stay below Anthropic's union-type limit. New scenarios use the full schema;
populated scenarios use a compact one-step patch that preserves and validates
the existing scenario server-side. The assistant can propose simulated
business-service access probes from existing expected-denial examples; it does
not perform raw network scans or claim runtime outcomes.
`scripts/configure-assistant.sh` stores the key in a protected runtime file
readable by both assistant services. Simulator
status and requests check that file dynamically; the existing Control-Service
container is restarted in place by the setup command, not recreated from shell
defaults. The stack forwards only the key-file path to
containers, not the key as a command-line argument.

Assistant setup requires a running Control-Service using the standard persistent
key path. `sh scripts/dashboard-stack.sh reload-assistant` reloads an existing
stored key without replacing the container configuration. Keep the private
`.local-runtime/dashboard-stack/assistant/` directory when cleaning runtime
artifacts; if its key file is deleted, the GUI cannot recover it.

The local stack's Policy-Service startup/restart selects an explicit
`SIMULATION_ORGANIZATION_ID` first, then the saved active organization, then
the bootstrap manifest. This keeps policy attributes aligned with the selected
local directory rather than reverting to the bootstrap organization on restart.

The reusable base contract lives in `.local-runtime/generic-zpr-base.json` and
is validated separately by the installer.

The manifest extends the generic ZPR base and describes the complete bootstrap:
required packages, Rust toolchain, ZPR artifacts, base trusted services, and
boot agents. A scenario adds services and actors on top of that base rather
than replacing the installation contract.

The containerized simulated environment installs the ZPR binaries with
`.local-runtime/build-and-run-linux-node.sh`; it validates and passes the same
manifest into the extended boot. A generic simulation uses the default
manifest, while a scenario-specific simulation sets `SIMULATION_MANIFEST`.

For the local runtime, configure and start the complete stack with
`cmd/zpr-web-dashboard/scripts/dashboard-stack.sh`. It builds one binary,
starts the services in dependency order, records their PIDs and logs, and
supports `start`, `stop`, `restart`, and `status`.

The individual process environment is documented in
[`cmd/zpr-web-dashboard/README.md`](cmd/zpr-web-dashboard/README.md) for
non-local deployments.

Open `http://127.0.0.1:8787` for the Control Room UI.

### Great Lakes business-service simulations

Great Lakes includes seven stateful simulated business APIs and two 42-pair
access-matrix scenarios. They use a dedicated provider host, authenticated
employee machine adapters, persistent records, and fresh Visa denial evidence.
They are not commercial applications or human/group-claim authentication tests.
See the [Great Lakes guide](cmd/zpr-web-dashboard/examples/organizations/great-lakes/README.md)
for the service/policy matrix, persistence limits, and deployment prerequisites.
This source addition does not deploy providers into existing running containers.
