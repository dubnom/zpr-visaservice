# Simulator as an Ordinary ZPR Client

Date: 2026-10-23

Status: proposed functional/demo delivery plan. Packet B completed the B01
inventory and added targeted C06 regression evidence; production boundary
violations remain open. Other tasks/acceptance gates remain pending; this
document does not claim deployment or production readiness.

## Packet A implementation record (2026-10-23)

B02's production ownership/name mapping and the smallest network LDAP/read-path
increment are documented in
[zpr-dashboard/ASSERTIONS.md](zpr-dashboard/ASSERTIONS.md#production-network-ldap-and-source-identity-packet-a).
The existing Control-Service assertion source registry now supports explicit
organization-scoped LDAPS without containers, protected bind-file references,
verified TLS, approved attributes and independent selected-source reads. Local
TLS protocol/consumer fixtures cover two simultaneous sources, isolation,
no fallback, credential/certificate failures and Simulator-unavailable inputs.
No live LDAP access, deployment or runtime registration change is implied.
This does **not** close M0/M1: review/approval, revisioned registration,
feed/inventory joins, capability/freshness lifecycle and integrated UI acceptance
remain pending. The milestone checkboxes below retain that distinction.

## 1. Target outcome

**Control Room does not know that Simulator exists.** Simulator is one more
client of the ZPR system, alongside real users, adapters and applications.
Control Room sees ordinary identities, services, topology, policies, trusted
sources and activity through production-owned contracts.

"Unknown" means no special Simulator dependency, configuration, API, data model,
authorization bypass or operational workflow. It does not mean hiding an actor's
real name from audit records. An ordinary client may have "Simulator" in its
display name; production must not branch on that name or require a client-type
flag. There is no production "simulation mode."

Simulator has two separate roles:

- **Workload client:** exercises ZPR with ordinary endpoint/user identities and
  permissions. It receives the same allow/deny decisions as another client.
- **Optional setup automation:** uses ordinary, explicitly granted operator
  APIs to prepare a disposable organization. Workloads do not inherit those
  operator permissions. A human can perform the same setup without Simulator.

The first functional demonstration must include **two simultaneously configured
trusted sources**, including a read-only organization LDAP connection, with
source-specific provenance and no dependency on a Simulator directory.

## 2. Wave 2 bootstrap: evaluation-only starter profile (2026-10-23)

This wave adds an **EVALUATION-ONLY** path for a bootstrappable out-of-box ZPR:
one supported command brings up the ZPR evaluation stack with a unique bundled
LDAP directory and logging service, using the current lab bootstrap, identity-
compose/OpenLDAP/Dex, OpenObserve and `zpr-trusted-service` packages. Every
installation gets distinct secrets and persistent state; there are no shared or
default passwords. The UI, package and operator instructions must label these
starter services **not production supported**. This narrows the 2026-10-17
bundled-or-existing production decision only for the beta/evaluation profile;
it does not erase or satisfy the separate production requirement below.

The starter LDAP source is a real, registered trusted source in the evaluation
organization and must support the existing LDAP push-change facility. The
RFC 4533 syncrepl watcher and OpenLDAP accesslog change feed run with the ZPR
stack whether or not Simulator is installed or running. Simulator's optional
add-on registers its **own separate synthetic directory** as another trusted
source; it never writes its directory into or populates the bundled starter.

A company can link its LDAP and logging while the starter services continue to
run side by side. A later, explicit retirement action must identify and rebind
affected source/policy references, preserve or deliberately migrate state, and
show the impact before applying. There is no automatic identity merge, default-
source switch, or silent fallback between starter, Simulator and company data.

The B1 local implementation is available through the
[evaluation bootstrap runbook](zpr-dashboard/packaging/evaluation/README.md).
B2/B3 now use ordinary source/log contracts; see the runbook verification matrix.
Its focused race and two-cycle disposable runtime evidence does not accept B4-B6 or a clean-host
production release. B milestones retain separate release/visual acceptance from
the existing M0-M6 ordinary-client and production-boundary gates:

| Milestone | Scope and exit evidence |
| --- | --- |
| **B0 — Inventory and profile contract** | Record current package/image/script behavior and gaps in the [system release plan](System%20release%20plan.md#evaluation-only-bootstrap-profile-2026-10-23); freeze the evaluation-only label, state/secret ownership and independent service lifecycle. |
| **B1 — One-command bootstrap** | Local implementation: `sh packaging/evaluation/bootstrap.sh -state "$PWD/.zpr-evaluation-local"` from `zpr-dashboard` (add `-render-only` for offline rendering). Starts production-tagged operator services plus current packaged LDAP/OpenObserve/Dex, foreground supervised with verified TLS/readiness and preserved install identity. Clean-host/platform and browser release acceptance remain separate. A clean supported host can run one documented command to initialize unique per-install protected secrets/state, render and start required ZPR plus LDAP/logging services, wait on protocol health, and report actionable status. Repeat/restart preserves identity and data; Simulator is unavailable. |
| **B2 — Register starter source** | Local implementation and automated V02/V06/isolation/restart evidence: `directory_<install-id>` via the public registration/test/read API, explicit server organization scope, no reserved source name. See evaluation runbook; browser/clean-host release acceptance remains separate. Bootstrap registers the bundled LDAP as a named, organization-scoped trusted source; Control Room can read and attribute results to it. Verify isolation, restart persistence and no implicit default/fallback behavior. |
| **B3 — Register starter logging** | Local implementation and focused race/disposable evidence: ordinary production log inventory, HTTPS OpenObserve, administrator-only `logs.read`, 7-day retention and queryable allowlisted lifecycle events. Raw application logs/lossless collection, live company backends and elapsed retention acceptance remain gaps. Bootstrap configures the OpenObserve-based evaluation logging source/collector and authorized query path. Verify actual ingestion, visible source identity, persistent storage, access-control boundaries and explicit unavailable/error state. |
| **B4 — Independent LDAP push feed** | Run the existing RFC 4533 syncrepl watcher and accesslog feed for the bundled LDAP outside Simulator lifecycle. **Control-Service owns durable per-source watcher checkpoints**, reconnect/resync health and private metadata retention. Demonstrate refresh/add/modify/delete and disconnect/reconnect behavior with private metadata and no attribute values/secrets. This does **not** claim that feed events refresh ZPR attribute caches or revoke visas; implement and accept those consumers separately unless independently verified already. |
| **B5 — Add Simulator companion** | Add Simulator without replacing/reseeding the starter source. Register its own isolated synthetic directory as a second source, exercise ordinary production contracts, then stop Simulator and prove starter LDAP/logging/Control Room remain available. |
| **B6 — Link company source/logging and retire starter explicitly** | Link company LDAP and logging alongside the running starter; show provenance and independent failures. An explicit wizard: **link company source → rebind policies → verify → disable bundled**; preview affected identity/source/policy/logging state and require authorized confirmation/recovery before stopping starter services; no automatic merge or fallback. |

### B1 decisions recorded (2026-10-23)

- Use the **current packaged OpenLDAP image**, evaluation-only:
  `osixia/openldap@sha256:0de3850635911109f0daf8f5957808ec4f4eb827fbaaa339100b5f1bab925745`
  (`linux/amd64`). This is not production image qualification. The current
  OpenObserve/Dex package pins and mixed-platform requirements are in the runbook.
- OpenObserve **AGPL-3.0 accepted for evaluation**, **7-day global retention**,
  **administrator-only access**; verify unauthenticated queries fail and preserve
  TLS verification. B3 now supplies ordinary named-source configuration,
  allowlisted lifecycle ingestion and authorized Control Room querying; raw
  application-log collection and release acceptance remain separate.
- **B1-RECOVERY (design/task, not implemented):** local CLI reset generates a
  unique one-time password into a new protected file, never stdout/logs; verify
  local ownership, lock, supported provider update, invalidate sessions and require
  replacement on first use. Include expiry/replay/rollback and independent logging
  admin tests. Dex static credentials cannot enforce this safely as a small reset
  script; do not claim editing a hash or deleting state is admin recovery.
- **B4 decision:** Control-Service owns durable **per-source** change-watcher
  checkpoints/resync health. B4 now embeds the existing RFC 4533 consumer with
  production-source opt-in, restart resume, metadata-only health and the existing
  Visa Service cache/reconciliation API. Deterministic race-tested acceptance is
  documented in [ASSERTIONS.md](zpr-dashboard/ASSERTIONS.md#production-ldap-push-watcher-b4);
  live LDAP-to-visa revocation and asynchronous completion receipts remain gaps.
  B1 neither registers nor starts these watchers.
- **B6 decision:** explicit retirement wizard **link company source → rebind
  policies → verify → disable bundled**, with authorized impact preview, recovery
  and no implicit identity merge/default switch/fallback.
- Optional Simulator is an **add-later, separately installed companion only**.
  Give it independent state/credentials and a separate synthetic source through
  production contracts; it is never a requirement or startup action of B1.

B1 engineering evidence (2026-10-23): focused race/unit tests, vet, shellcheck,
Python syntax, strict boundary audit and the two-cycle isolated mixed-image
startup/restart test passed. Broader `make test` remains non-green on unrelated
missing fixtures/Rust dependencies and existing launcher/macOS path assumptions;
see the runbook. No B-V1/B2-B6/production acceptance is implied.

### Wave 2 visual acceptance

Add browser and clean-host acceptance for each release of this profile. Exercise
desktop and tablet, keyboard-only navigation, readable failure states and no
horizontal overflow. Capture screenshots, but also assert source IDs, API
requests, service health and persistent state:

| ID | Setup/action | Required visible result and independent proof |
| --- | --- | --- |
| **B-V1** | Fresh host, one-command bootstrap, Simulator absent/unreachable. | Login, source, logging and health views are usable; LDAP and logging are healthy, unique install identity is retained after restart, and the evaluation-only/not-production label is visible. No Simulator request or dependency. |
| **B-V2** | Add a Simulator companion with a distinct synthetic directory. | Two named sources coexist in the same runtime evaluation organization with distinct source identities and provenance; synthetic records do not appear in starter LDAP. Stop Simulator; starter source, logging and operator UI remain healthy. |
| **B-V3** | Change a starter LDAP entry and disconnect/reconnect the directory. | Updates feed reflects metadata-only add/modify/delete and explicit reconnect/resync state; no attribute values, passwords or cookies leak. No visa/attribute refresh or revocation claim is shown. |
| **B-V4** | Link company LDAP and logging while starter services remain up; then preview explicit retirement. | Company and starter sources are visibly distinct; failures stay source-specific without fallback. Retirement lists affected bindings and requires explicit approval/rebinding; no silent merge, source switch or loss of state. |

## 3. Relationship to existing work

This is a demo-first task and acceptance list, not a replacement for the
historical implementation/approval ledger in the
[administrator migration plan](SIMULATOR_ADMIN_MIGRATION_PLAN.md).
It proposes deferring advanced credential lifecycle and platform qualification,
not weakening existing authorization, TLS verification or named-approval rules.
Any implementation that changes an earlier approved decision needs that decision
reconciled explicitly first. Nothing here authorizes live company mutations.

Relevant starting points:

| Existing evidence | Implication for this plan |
| --- | --- |
| [Control Room boundary](AGENTS.repo.md#control-room-boundary) requires production-owned data and Simulator-independent operation. | Preserve this as the architectural acceptance gate, not just a UI naming change. |
| [Source configuration](zpr-dashboard/cmd/zpr-web-dashboard/assertion_sources.go) supports named `ldap` and `https-json` assertion sources, organization scopes and a configured default, bounded to 16 total sources. | Reuse this work and its explicit-source semantics; do not build a competing source registry. Reconcile assertion, inventory and feed identifiers. |
| [Multi-source runtime tests](zpr-dashboard/cmd/zpr-web-dashboard/assertion_runtime_test.go) exercise two HTTPS sources with Simulator inputs unavailable. | Multi-source capability is partially present. This is not proof of external organization LDAP registration or end-to-end UI readiness. |
| The existing LDAP source configuration requires a container; [Simulator's source handler](zpr-dashboard/cmd/zpr-web-dashboard/trusted_source.go) reads the active simulation organization. | Implement a production-owned external LDAP/provider path. Do not reuse the Simulator handler as the production contract. |
| [Change feeds](zpr-dashboard/cmd/zpr-web-dashboard/trusted_changes.go) have separate named configuration, and [browser tests](zpr-dashboard/tests/browser/trusted-source-gui.mjs) cover multiple feed selection. | Preserve working feed behavior, but extend coherent source selection to People, Groups, Attributes and assertions. Multiple feeds alone do not establish multiple usable identity sources. |
| [Source reorganization](REDIRECTORY.md) records incremental extraction with remaining application/package boundaries. | Inventory current code and dirty worktree before implementation; do not treat historical milestones as current release acceptance. |

## 4. Ownership and dependency rules

```text
Control Room -> Control-Service -> production services / configured providers
Simulator ----------------------> supported production APIs
Simulated endpoints ------------> ordinary enrollment/authentication/ZPR traffic
Production provider ------------> organization LDAP or independent lab LDAP
```

- Visa Service owns authoritative runtime identity, topology and visa state.
- Policy-Service owns policy/configuration revisions, validation, apply
  operations and authoritative application status through its production APIs.
- The production provider/configuration owner owns trusted-source registration,
  scope, mappings, connection status and credentials. Control-Service exposes
  authorized operator access; settle the existing service owner in M0.
- Operator configuration owns log/inventory inputs. Real service telemetry,
  not scenario plans, supplies observed activity.
- Independent companion services may host lab LDAP/OIDC/DNS. Simulator may use
  them, but stopping Simulator must not stop them.
- Simulator alone owns scenarios, synthetic machine definitions, workloads,
  faults, expected outcomes and scenario execution state.
- Control Room/Control-Service must not call Simulator APIs, read its files or
  discover its containers. Production service configuration must not be loaded
  from a simulation manifest or exported into a Simulator-owned runtime path.
- A scenario reset stops/resets scenario work only. Revoke, re-enroll, delete
  an organization, change policy or restart shared services are separate,
  authorized operations.

## 5. Milestones and task list

Delivery order: **M0 -> M1 -> M2 -> M3 -> M4 -> M5**. M6 is the separate
pre-production gate, not a prerequisite for a clearly labeled lab demonstration.
UI work can proceed against agreed contracts after M0; a milestone closes only
when its integrated exit criteria pass.

### M0 - Agree and enforce the ordinary-client boundary

- [x] **B01:** Inventory production routes, transitive imports, configuration,
  startup scripts, filesystem mounts, inventory/log enrichment and browser
  requests for Simulator coupling. Record each occurrence and its replacement
  owner; distinguish Simulator-only code from production dependencies. See
  [packet B inventory and failing acceptance audit](CONTROL_ROOM_BOUNDARY_INVENTORY.md).
- [ ] **B02:** Map existing source names across provider registration, Visa
  Service inventory, assertion configuration, change feeds and browser DTOs.
  Select one stable organization-scoped source identity and an existing
  production owner; document API/configuration shapes before adding routes.
- [ ] **B03:** Define supported ordinary-client operations and permissions:
  source registration/read/test, configuration save/test/stage/apply/readback,
  enrollment, telemetry and workload traffic. Preserve named human approval
  for privileged operations; no new Simulator-only endpoint or grant.
- [ ] **B04:** Establish two disposable test organizations, two independent
  sources and an ordinary non-Simulator comparison client. Arrange explicit
  read-only consent and a test account before touching organization LDAP.
- [ ] **B05:** Capture the current targeted test baseline and deployment
  versions separately. Agree the measurable refresh/outage budget used below.

**Exit:** dependency/ownership map and public contracts are reviewed; executable
negative tests can detect reverse dependencies. No live credentials required.

### M1 - Multiple trusted sources and real organization LDAP

Depends on M0.

- [ ] **S01:** Extend the existing production source configuration into a
  durable, revisioned registration workflow: stable ID, organization, display
  name, provider kind/endpoint, base DN, read-only bind reference, approved
  attributes/mappings, enabled state and optional capabilities. Never persist
  a password in browser-visible configuration.
- [ ] **S02:** Support organization LDAP independently of Docker/Simulator.
  Reuse an existing production LDAP integration where suitable; otherwise add
  the smallest network LDAP adapter or independently deployed provider needed.
  Use verified LDAPS/StartTLS and protected credentials. A provider bridge
  using the existing HTTPS-source contract is acceptable if it preserves the
  required identity/attribute semantics; no new identity platform is required.
- [ ] **S03:** Register two concurrent sources, for example `company_directory`
  and `lab_directory`, using the same contract. Do not change the selected
  organization's production configuration when a scenario is selected.
- [ ] **S04:** Add independent connection tests, bounded timeouts, last-success
  timestamps and clear not-configured/unavailable/stale states per source.
  A read-only directory without a change-feed API remains a valid source;
  show Updates as unsupported, not broken or silently empty.
- [ ] **S05:** Carry source identity and provenance through people, groups,
  attributes, mappings, assertion results and applicable policy inputs.
  Preserve source-qualified references. Matching usernames/DNs across sources
  must not silently merge identities or overwrite attributes; any cross-source
  correlation must be explicit and reviewed.
- [ ] **S06:** Make defaults explicit; a default selects a source, not a
  failover authority. If a required source fails, report evaluation unavailable/
  error and do not grant on substituted or stale attributes. An unrelated
  source remains browsable and usable where it is not a dependency.
- [ ] **S07:** Test source/organization isolation, duplicate IDs, invalid
  mappings, revision conflicts, missing secrets, wrong credentials/certificates,
  source disable/removal and restart persistence. Block or explicitly resolve
  references before removing a source used by a policy/assertion.

**Exit:** two sources can be registered and read concurrently, including the
approved LDAP/provider path, with Simulator never started. LDAP receives no
write requests. Visual tests V02-V07 and V13 pass.

### M2 - Production-owned Control Room workflows

Depends on M1's source contract and M0's service ownership.

- [ ] **C01:** Provide source list/detail and authorized registration/edit/test/
  disable workflows through production APIs. Use the existing Trusted Sources
  presentation and permission patterns; do not embed Simulator editors.
- [ ] **C02:** Keep source selection coherent across People, Groups, Attributes,
  Updates and assertion tooling. Cancel/reject stale responses on source or
  organization changes; reset pagination and source-specific diagnostics.
- [ ] **C03:** Obtain topology/identity from Visa Service, policy/organization
  context from Policy-Service, attributes from registered providers and logs
  from operator-configured sources. Remove production manifest/profile,
  synthetic-label and machine-log discovery dependencies.
- [ ] **C04:** Ensure a human can configure, validate, stage and explicitly
  apply an ordinary organization without Simulator. Display saved, tested,
  staged, approved, applied, failed and unknown/reconciling states honestly;
  application success requires authoritative readback.
- [ ] **C05:** Retain independent Control Room login/navigation. No scenario
  controls, Simulator status lamps, shared browser sessions, Simulator target
  handoff or automatic lab defaults in production.
- [ ] **C06:** Verify direct Control Room startup and normal usage with
  Simulator absent, its URL unreachable, its environment unset or invalid,
  and its files/mounts inaccessible. Packet B adds passing isolated Go/browser
  guards, but this gate remains open: the strict launcher/shared-asset audit
  still fails. See [scope, results and remaining violations](CONTROL_ROOM_BOUNDARY_INVENTORY.md).

**Exit:** V01-V08, V13 and V15 pass; production request/file-access evidence
shows no reverse dependency. A screenshot without Simulator labels is not
sufficient evidence.

### M3 - Simulator uses the same ZPR contracts as another client

Depends on M1 and the required M2 operator contracts.

- [ ] **U01:** Replace direct database writes, private service calls, generated
  production runtime configuration and launcher-only setup with ordinary
  supported APIs. Remove hidden fallback paths, not merely their menu entries.
- [ ] **U02:** Separate optional setup automation identity/permissions from
  workload/endpoint identities. Use existing grants and approval paths; a
  separate-tab Control Room link may retain the earlier approved navigation
  contract without passing sessions or organization selection.
- [ ] **U03:** Let a scenario select existing permitted organization/source
  references without owning their lifecycle. Real LDAP is read-only; scenario
  changes/fault injection may mutate only explicitly owned disposable lab data.
  Missing source/setup becomes an actionable preflight error, not auto-create
  or silent selection of another directory.
- [ ] **U04:** Use ordinary endpoint enrollment/authentication and unique
  endpoint identities. For the demo, pre-enroll a small fixed set through
  existing supported tools rather than making native key/helper development a
  prerequisite. Persist each identity; do not clone endpoint keys.
- [ ] **U05:** Run normal allow/deny workloads against services. Compare one
  generated client and one non-Simulator client under equivalent policy.
  Scenario intent/expected outcomes stay in Simulator; Control Room reports
  only independently observed production events.
- [ ] **U06:** Reconcile setup/apply retries by durable operation identity and
  readback. Scenario reset preserves registrations, policy, directory data and
  enrollment; destructive cleanup is separately scoped, confirmed and audited.

**Exit:** V09-V12 and V14 pass. Simulator can exercise an independently prepared
ZPR installation; stopping it does not remove services or their configuration.

### M4 - Enforce separation in builds, configuration and lifecycle

Depends on M2 and M3. This closes deployable boundaries, not just code naming.

- [ ] **P01:** Give production and Simulator separate entrypoints/configuration,
  startup and shutdown ownership. Finish the minimal package/import extraction
  needed to prevent production importing Simulator code; reuse neutral helpers.
- [ ] **P02:** Produce a production deployment with no Simulator executable,
  routes, static pages, manifests, scenario mounts or lab defaults. Reject
  missing explicit operator configuration rather than discovering demo values.
- [ ] **P03:** Independently supervise lab LDAP/OIDC/DNS. Document which
  disposable resources belong to the lab and which services are shared.
- [ ] **P04:** Add CI import/configuration/route checks and runtime boundary
  tests. Capture browser and server egress plus file-access evidence; browser
  request interception alone cannot detect server-side reverse dependencies.
- [ ] **P05:** Test cold start, independent restart and client disconnects.
  Stopping Simulator may naturally stop its workload traffic and disconnect its
  endpoints, but must not cause production dependency errors or erase state.

**Exit:** V01, V11, V13 and V15 pass from the production-only deployment, then
again after adding/removing Simulator. Independent startup is reproducible.

### M5 - Reproducible functional demonstration

Depends on M1-M4.

- [ ] **D01:** Prepare a short runbook: production-only login; two-source
  registration/read; start ordinary and generated clients; allow/deny traffic;
  source outage/recovery; stop Simulator; continue operating Control Room.
- [ ] **D02:** Use synthetic identities first. Run approved organization LDAP
  acceptance separately with read-only scope; use redacted screenshots and no
  sensitive directory exports. Synthetic LDAP proves the adapter behavior, not
  actual company connectivity.
- [ ] **D03:** Execute the visual matrix below plus integration/boundary tests.
  Retain build IDs, configuration revisions, browser traces/screenshots,
  operation IDs and sanitized service/network evidence.
- [ ] **D04:** Document completed behavior, unsupported capabilities and every
  active shortcut. Record failures separately from passes; no blanket green
  status from fixture-only tests.

**Exit:** all V01-V15 pass in the agreed lab; the approved organization LDAP
connection passes its separate read-only test. If unavailable, label that
acceptance blocked rather than substituting a synthetic pass.

### M6 - Pre-production closure

Not required for the isolated functional demo; required before production use.

- [ ] **R01:** Close every active item in the shortcut register with an owner,
  replacement implementation and passing qualification evidence.
- [ ] **R02:** Complete credential rotation/revocation, native protected-key
  qualification, least-privilege operator lifecycle and recovery.
- [ ] **R03:** Qualify installation, upgrade/rollback, backup/restore, audit
  retention, source scaling, outage semantics and real deployment platforms
  under the existing [system release plan](System%20release%20plan.md).

**Exit:** production release gates pass; no demo exception is enabled by default.

## 6. Visual acceptance tests

Use the existing [Playwright configuration](zpr-dashboard/playwright.config.mjs):
desktop Chromium and tablet Chromium, reduced motion, traces and screenshots.
For each row, exercise both layouts, keyboard navigation, readable errors and
no page-level horizontal overflow. Fixture tests cover deterministic states;
live isolated-stack tests establish the architecture and real behavior.

Proposed demo observation budget: changed health/activity is visible within
**10 seconds** after a manual refresh or normal polling, measured from the
injected event. Confirm this budget at B05. Tests must not hide failures with
arbitrary sleeps or screenshot updates.

| ID | Setup/action | Required visible result | Additional proof / milestone |
| --- | --- | --- | --- |
| V01 | Cold-start production; Simulator not installed or reachable; open every operator page. | Login, topology, policies, sources, logs and available editors operate; no Simulator warning, scenario selector or setup prerequisite. Missing production integrations are named explicitly. | No Simulator browser/server requests or file reads; M2/M4. |
| V02 | Register company LDAP/provider and an independent lab source. | Both appear together with stable IDs/names and separate connection results. No "active simulation organization" is needed. | Production registrations survive restart; read-only LDAP access; M1. |
| V03 | Select each source across People, Groups, Attributes, Updates and assertions. Delay an old response while switching. | Every view/result identifies its source; late results never overwrite the new selection. Unsupported Updates is explicit. | Requests include correct source/organization; cursors and diagnostics do not bleed; M1/M2. |
| V04 | Put the same username in both sources with different memberships/attributes. | Source-qualified identities/values stay distinct; no accidental union or overwrite. | Assertion/policy evidence identifies the configured authority/correlation; M1. |
| V05 | Disconnect one source, then restore it. | Affected source shows unavailable/stale and last success within the budget; other source stays usable; recovery is visible. | Required-source evaluations error, never silently pass/fail over; M1. |
| V06 | Test wrong bind credential, untrusted certificate and invalid base DN/mapping. | Clear source-specific error; no healthy badge, leaked secret or silent fallback. | Failed test cannot be presented as verified connectivity; M1. |
| V07 | Switch organizations, including one unauthorized or without registered sources. | Lists, results, counts and errors belong only to the selected authorized organization; explicit empty/forbidden state. | Backend scope checks and stale-response rejection, not just hidden UI; M1/M2. |
| V08 | Human saves/tests/stages/applies a policy with Simulator absent; inject failed apply/lost response. | Revisions and approval are distinct from applied state; failure/reconciling is visible until readback resolves it. | Durable operation ID, named approval and authoritative policy readback; M2. |
| V09 | Start one pre-enrolled generated client and one ordinary comparison client. | Both appear as ordinary actors/adapters/services; no special Simulator panel or badges are required. | Same endpoint contracts; distinct identities and expected permissions; M3. |
| V10 | Generate allowed and denied traffic from both clients. | Actual visas/denials/logs identify the real actors and policy; scenario expectations do not populate observed activity. | Compare independent service telemetry and actual traffic outcome; M3. |
| V11 | Stop/restart Simulator while leaving clients/services independently controlled. | Control Room stays usable; production identities/policies/sources remain. Ordinary disconnects are acceptable, dependency errors are not. | No provider/service restart or erased state; production requests succeed; M3/M4. |
| V12 | Reset a scenario and retry a setup action after losing its response. | Reset confirmation states its scope; retained configuration is unchanged; retry resolves the same operation without duplicate resources. | Compare source, policy, enrollment and directory state before/after; M3. |
| V13 | Restart production services and independently restart LDAP/provider with Simulator off. | Registrations/defaults persist, outage is explicit and recovery returns to the same source identities. | Production storage and independent lifecycle, not regenerated manifests; M1/M4. |
| V14 | Use workload-only credentials to attempt source registration, apply, approval or cross-organization access. | Honest forbidden/read-only state; no privileged action succeeds. | Production enforcement and named audit evidence; M3. |
| V15 | Use a fresh browser and production-only package/configuration. Inspect routes and unavailable integrations. | No Simulator navigation, lab defaults, scenario status or credential fields in operator payloads; production errors remain accurate. | Package/import/route checks and sanitized browser/server/file-access traces; M4. |
| V16 | Open the policy editor with two trusted sources returning the same attribute name, then choose each completion on desktop and tablet. | Completion labels show stable source IDs and insert distinct `attribute__source__source-id` keys; unqualified mapped conditions show a clear ambiguity diagnostic. | Verify keyboard completion, exact submitted source, analysis/staging rejection, and no horizontal overflow; source keys must match runtime ingestion. |
| V17 | Exercise default-source selection, an approved/revoked cross-source subject link, source outage during positive and absence/negated checks, and identity-changing source edits. | Explicit default/link/version states are visible; only dependent grants deny on outage; changed source versions visibly invalidate dependent grants. | Compiler/runtime contract tests and per-grant audit evidence; currently pending external schema/compiler and link lifecycle work. |

Extend the existing
[Trusted Sources tests](zpr-dashboard/tests/browser/trusted-source-gui.mjs)
and ordinary-client fixtures rather than creating another unrelated browser
harness. Existing tests are prior art, not execution evidence for this plan.
Screenshots verify presentation; backend, traffic and lifecycle assertions verify
that the separation is real.

### Wave 5 V02/V06 production registration slice (2026-10-23)

Base `315c10650b510911ab1b1516b699ab5a72df6adf`, exclusive
`maturing/w5-source-registration`. **Implemented and locally verified**, not a
claim of live-company provider or whole-migration acceptance:

- The production Sources page registers multiple LDAPS/HTTPS sources, edits
  display/configuration without changing organization/source IDs, validates
  approved mappings/attributes and protected secret-file references, and shows
  separate read-only connection-test success/failure. Saving is explicitly not
  verified connectivity; edited/late/other-organization results cannot become
  success evidence. No insecure TLS, raw credentials, fallback or reverse runtime
  dependency was introduced.
- `GET/POST/PUT /api/assertions/sources` and
  `POST /api/assertions/sources/test` are ordinary production contracts. The
  authenticated Policy Repository organization is authoritative. The sole
  registry remains `ZPR_ASSERTION_SOURCES_FILE`, now atomically replaced/synced
  with revision guards; empty-file seeding is supported. Registrations survive
  service restart. There is no session-only configuration fallback. Existing
  container/shared-scope sources remain file-managed. Contract/runbook:
  `zpr-dashboard/ASSERTIONS.md` and `zpr-dashboard/README.md`.
- Focused Go `-race`: **33 top-level / 79 including subtests pass**, separately
  in default and `production` builds; includes **seven new registry tests**,
  real local verified-TLS LDAP/HTTPS protocol fixtures, rejected bad bind,
  untrusted TLS, invalid/out-of-scope mapping, unsafe credential permissions,
  revision/identity/organization guards, restart persistence and concurrent
  immutable reader snapshots. All **eight** extracted production package suites
  pass with `-race`.
- Playwright desktop + tablet: **34 passed** (eight new registration cases,
  16 existing Trusted Sources GUI cases, ten Simulator-absent production browser
  boundary cases). New screenshots capture edit, TLS/bind/mapping failure,
  separate source results and HTTPS/unconfigured persistence layouts, with
  overflow assertions. Browser APIs are deterministic contract fixtures; actual
  network and persistence behavior is exercised by Go protocol tests, not a live
  company directory. Simulator was not started.
- Strict boundary audit: **exit 0 / zero references**, unchanged checker and no
  allowlist. Go formatting, JavaScript syntax and `git diff --check` pass.
  Required `make test` remains **exit 2 / 38 known baseline top-level failures**
  (24 command, one enrollment, 13 reset), from missing lab/Rust/shell fixtures,
  observability readiness, macOS IPC path length and protected reset directories.
  New route/handler ownership expectations were extended, not weakened; unrelated
  failures were neither repaired nor skipped.

**Still pending:** authorized real-company LDAP/HTTPS acceptance, directory ACL
least-privilege/completeness proof, credential lifecycle/rotation and HA writer
coordination. Configuration is durable but connection-test observations are
session-only evidence, not persisted healthy certification. X02/X07 remain
active; X11 records the single-writer local-config provisioning limit. No source
retirement, default-source reassignment, live LDAP mutation, push or deployment
was performed. M1/M4/M5 and the full V01-V15 gate are not declared complete.

## 7. Demo shortcuts and pre-production register

Reduce **credential ceremony**, not the trust boundary. Prefer generating or
pre-provisioning valid credentials once over disabling checks. These are
proposed permitted approaches for an isolated lab, not claims they are active.
Every shortcut used must record its ID, owner, exact scope/configuration,
activation date, removal gate and verification evidence in the demo handoff.

| ID | Demo shortcut | Guardrail now | Required before production |
| --- | --- | --- | --- |
| X01 | Generate one dedicated per-install lab CA and valid service certificates with existing bootstrap tooling; install trust once. | Keep hostname/chain verification and existing mTLS authorization; protect private files; no public/shared private keys. | Managed trust issuance, expiry monitoring, rotation, revocation and recovery qualification. |
| X02 | Provision read-only bind passwords/service tokens once into protected local files. | Use references, restrictive permissions and log/UI redaction; no repository/browser secrets, default passwords or company admin binds. | Managed secret storage/delivery, rotation, access review and recovery. |
| X03 | Pre-create a few named demo users and explicitly grant a restricted automation identity. | Preserve ordinary authentication and organization/action checks; no blanket admin or shared Control Room session. | Full provisioning/deprovisioning, access review and operator lifecycle acceptance. |
| X04 | Pre-enroll a fixed small set of endpoints through existing supported tools; retain their unique credentials. | Each endpoint owns its key; no key cloning/export to Simulator, fabricated identity or bypassed approval. | Full enrollment/revocation/re-enrollment, platform protected-key/helper and recovery qualification. |
| X05 | Keep policy-signing/service credentials locally provisioned for the disposable lab rather than completing automated custody/rotation. | Preserve actual signing/verification, named approvals and applied-state readback; protect signing keys. | Qualified custody, rollover, revocation, audit and disaster recovery. |
| X06 | Use a single-host/small isolated stack and manual startup/runbook instead of a finished installer or HA. | Production, Simulator and providers still have independent configuration/storage/lifecycle. Demo is labeled non-production. | Installer/upgrades, backup/restore, HA/capacity and platform/network qualification. |
| X07 | Begin with synthetic LDAP and a small deterministic traffic set. | Report it as synthetic; actual organization LDAP remains a distinct acceptance gate; no load/scale claims. | Approved real integrations, representative volume, freshness and sustained outage testing. |
| X08 | Use the current bundled OpenLDAP/Dex and OpenObserve starter profile for beta/evaluation bootstrap. | Mark **EVALUATION-ONLY / NOT PRODUCTION SUPPORTED**; generate unique per-install secrets/state; never use shared/default passwords; keep Simulator's directory separate. | Company-owned trusted identity and logging integrations must be linked and verified before the evaluation starter is retired; production qualification of bundled providers remains governed by the 2026-10-17 requirement. |
| X09 | Run starter and company LDAP/logging in parallel during evaluation migration. | Independent source IDs, credentials, health and provenance; no automatic identity merge, fallback or default-source switch. | Explicit reviewed source/policy rebinding, retirement approval, state retention/removal plan and recovery test. |
| X11 | Use a dedicated static-password Dex administrator and local password grant only to discover/verify the bootstrap operator subject. | Active B1, evaluation bootstrap maintainers, 2026-10-23: unique per-install 256-bit credential, loopback verified TLS, signed ID-token verification, organization-scoped read-only grants; no tokens/passwords in output, no shared sessions; upstream container output disabled. | Replace static/password-grant bootstrap with qualified administrator enrollment, one-time local CLI recovery/session invalidation and managed credentials; prove real browser OIDC/PKCE and forced-change/expiry/replay recovery gates. |
| X10 | Keep push-feed consumption at metadata-only observation in this wave. | Syncrepl/accesslog produces private source-change metadata only; do not imply that visa attributes refresh or visas revoke on each change. | Separate implementation and acceptance for cache refresh, assertion reevaluation and visa revocation semantics. |
| X11 | Provision one protected, operator-owned `ZPR_ASSERTION_SOURCES_FILE` per Control-Service and use a single service writer for registration/editing. | Durable atomic replacement, revision/external-edit conflicts, stable organization/source IDs, verified TLS and secret file references; no session-only fallback or test-as-health claim. V02/V06 local acceptance uses protocol fixtures, not live company LDAP. | Managed configuration/secret lifecycle, cross-process/HA writer coordination and authorized real-provider outage/recovery qualification. |

| X12 | Recover the bundled evaluation login administrator through a stopped-host local CLI and Dex static configuration. | Active 2026-10-23; owner: evaluation-bootstrap/package maintainers. `-evaluation -reset-admin -state DIR` requires state ownership, private permissions, checksums, lifecycle lock and idle ports; generates 256-bit credentials, rotates password/client secret/admin subject, removes old grants and audits without secrets. One terminal disclosure or new 0600 file; provider data retained. Evidence: focused recovery race/unit tests and the evaluation runbook. | Qualify actual one-time login, forced change, expiry/replay enforcement, upstream token/session revocation, independent OpenObserve-admin recovery and crash-atomic recovery/rollback. Static Dex passwords remain reusable until another reset; issued JWTs remain signed until expiry. Interrupted multi-file changes require a complete protected offline backup restore and fresh reset; no production recovery claim. |
| X13 | Defer per-grant selective visa revalidation; use the existing conservative global visa sweep in the demo. | Global revalidation is intentionally retained and fail-closed; this is not a claim that only grants depending on the changed source were reevaluated. | Before production, persist positive and absence/negation source dependencies per grant, then qualify source-scoped refresh/revalidation and unknown-as-deny behavior. |

**B1 activation ledger (2026-10-23):** X01/X02/X06/X08/X11/X12 are active for
`-evaluation` only, owned by the evaluation-bootstrap/package maintainers. Exact
image/port/state/TLS/retention configuration and removal gaps are in the
[evaluation runbook](zpr-dashboard/packaging/evaluation/README.md); focused
`evaluation_test.go` and the opt-in isolated startup/restart test are evidence,
not production qualification. X03/X04/X05/X07/X09/X10/X13 are not newly activated
by B1; no source migration, synthetic users, watchers or traffic are provisioned.

**Not shortcuts:** disabling TLS/certificate verification, trusting self-asserted
actors, bypassing authorization/approval, treating a staged policy as applied,
granting from failed/stale sources, silently falling back to lab LDAP, exposing
secrets, direct database setup or allowing Simulator to own company services.

## 8. First deliverable and completion evidence

The smallest useful vertical slice is:

1. Start production without Simulator.
2. Register/read two independent trusted sources through production contracts.
3. Browse their source-qualified data and test an assertion.
4. Start a pre-enrolled generated client and an ordinary comparison client.
5. Observe policy-driven allowed/denied traffic in Control Room.
6. Stop Simulator and keep operating Control Room with both sources intact.

Do this before expanding scenario catalogs, polishing native key helpers or
building installers. Do not call the migration complete until M4's production
package/lifecycle boundary and M5's integrated evidence also pass.

The final handoff should report each task/test as pending, passed, failed or
blocked; link the exact build/revision and evidence; enumerate active X-items;
and state **functional demo accepted** separately from **production qualified**.

### B5 optional companion implementation note (2026-10-23)

Per the evaluation decision, the companion uses the **same runtime evaluation
organization**, discovered via authenticated public source registry, with its own
stable, distinct synthetic source ID (not a second organization).
`zpr-dashboard/packaging/simulator-companion/README.md` documents separate local
state and foreground lifecycle, ordinary-client-only setup/test/registration,
and explicit guarded own-source DELETE. No bundled LDAP, operator configuration,
default source, watcher acknowledgment or policy semantics are changed.
The disposable verified-mTLS real-command smoke and Simulator-absent operator
tests are isolated evidence only; full B-V2 deployment/browser and multi-source
policy/workload acceptance remain separate. This does not complete U01–U06 or
authorize the legacy Simulator activation paths.
