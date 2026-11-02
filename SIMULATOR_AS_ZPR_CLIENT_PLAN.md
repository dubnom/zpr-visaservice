# Simulator as an Ordinary ZPR Client

**Document role (reconciled 2026-10-29):** functional relationship and boundaries,
with dated implementation records below. The
[administrator migration plan](SIMULATOR_ADMIN_MIGRATION_PLAN.md) owns its
specific execution/dependency work; the
[evaluation runbook](zpr-dashboard/packaging/evaluation/README.md) owns current
bootstrap/package usage. Do not use an older proposed milestone here to negate
newer narrowly scoped source evidence or to claim full acceptance.

Date: 2026-10-23

Status (2026-10-30): turn-key evaluation delivery direction; implementation and
canonical UI/package acceptance remain partial. The active delivery sequence
below governs the next work. Earlier packet records retain their dated scope;
production boundary gates remain open. This document does not claim deployment
or production readiness.

## Active turn-key delivery plan (2026-10-30)

**Goal (clarified user decision, 2026-10-31):** in every Simulator
installation, after authentication, explicitly activating an organization
starts only that organization's authorized setup; selecting or browsing an
organization is passive. Selecting a scenario separately prepares and runs
only that scenario; it must not activate or reset an organization. Automatic
named self-approval remains part of each authorized workflow, without a
separate approval prompt. Simulator orchestrates ordinary ZPR contracts;
production services must work without it.

**Baseline:** the unchanged Velocity scenario passes sequential direct-action
diagnostics, repeats and an independent base restart. That is not canonical
Simulator UI, parallel-scheduler or matched-package acceptance. Its five
pre-enrolled identities and exact-ID policy grants are diagnostic scaffolding,
not the general organization onboarding contract. See the
[current delivery evidence and gaps](GUI%20work.md#current-status).

### Preparation and ownership

| Prepared once | Automated by Simulator through ordinary contracts |
| --- | --- |
| Independent ZPR services, reachable nodes, trusted enrollment/authentication authorities and a trusted bootstrap endpoint/CA | Check readiness and report the exact missing prerequisite |
| Organization directory/source references, operator issuer/subject grants including scoped domain approvals and self-approval, permitted workload/service policy | Compare required inventory with retained state; automatically submit and self-approve exact authorized setup |
| Separate Simulator operator client, transport identity and delegation signer | Generate endpoint-owned keys for genuinely new machines/adapters; request enrollment and securely deliver approved credentials |
| Reviewed infrastructure topology; adding nodes uses the separate infrastructure join model | Authenticate retained endpoints with fresh short-lived tokens, start workloads and observe production-owned DNS publication |

Organization profiles describe people, topology, workloads and expected results.
They do not contain private keys or runtime tokens. Runtime paths and artifact
pins may remain installation-owned implementation details, not manual profile
authoring requirements. Each organization has isolated retained state.

- Restart reuses endpoint identities and obtains fresh authentication; it does
  not re-enroll or regenerate keys. New endpoints receive unique identities.
- **Superseded for Simulator:** the earlier separate deliberate self-approval
  click and run acknowledgment. Login and passive organization browsing alone
  do not execute a change; explicit organization activation and scenario
  selection start their separate authorized workflows, with automatic named
  self-approval recorded as a distinct operation. Production Control Room
  approval UX remains unchanged.
- Every installation must configure its intended Simulator operator with
  `approval.self` plus the required domain/action and organization grants.
  Credentials identify the operator; they do not bypass backend authorization.
  Unauthorized or expired sessions fail closed with an actionable error, not
  an extra approval prompt or a fallback identity. No blanket grant expansion.
- Bind automatic approval to the exact inventory, candidate/configuration
  digest, target, generation and current verified operator. Revalidate grants
  and current state before each mutation; invalidate stale approvals. Unexpected
  trust/policy changes or destructive identity resets outside the authorized
  selection scope stop rather than silently expanding that scope.
- Record requester/approver identity, self-approval, scope, operation and outcome
  through production-owned evidence contracts. Audit should warn on risky
  self-approval role combinations or assertion findings, with the rule,
  affected role/identity and evidence references. Warnings are advisory and
  do not replace authorization or add a mandatory run confirmation. The
  budget request-time advisory does not establish an observed self-approval;
  a separate observed subject-link-review advisory covers only that event.
  Role-risk analysis and broader coverage remain open. See the
  [Audit owner](ENTITY_HISTORY_PLAN.md#self-approval-role-risk-warnings).
- Passive organization selection and explicit organization activation are
  separate actions. The current organization browser selects without mutation;
  its explicit Activate action requires confirmation. With
  `SIMULATION_SETUP_MODE=ordinary-client` (since 2026-11-01), confirmed web
  activation performs the ordinary authorized setup instead of the legacy
  lifecycle: the exact required retained endpoint inventory is compared
  through the authorized enrollment contracts, activation records the
  organization selection only when every requirement is retained and
  approved, and otherwise every missing prerequisite is reported explicitly
  without starting any setup; the legacy activation machinery is never
  invoked and unconfigured named delegation fails closed (`U01-ACTIVATION`).
  Restore, legacy packaged `approve-and-provision` and
  `verify`, global legacy stack/agent lifecycle, and unbound legacy machine
  start/stop still fail explicitly. Packaged status and existing-machine start/stop
  remain available; status can update local inventory and issue readiness
  traffic. Scenario preflight reports the unbound machine fallback before
  approval. The paired evaluation launcher
  forwards the explicit setting to the Simulator process, while the legacy
  default remains available for compatibility. The setting is process-local
  and must be repeated on restart; it does not cover every Simulator
  entrypoint or separately invoked scripts.
  Automatic onboarding of genuinely new profile-declared endpoints at
  activation remains a separate explicit operation. Scenario selection
  independently drives the
  scoped preflight/approval/run flow and does not activate the organization.
  Focused desktop/tablet browser fixtures assert that passive organization
  selection submits no setup or scenario mutation, scenario selection submits
  no activation/reset, and confirmed activation submits only the activation
  request. Changing the selected organization also invalidates pending reset
  intent immediately and ignores stale catalog responses. These fixtures do
  not establish live authorized setup acceptance.
- The separate Velocity Simulator OIDC client/signer has already been approved
  at port 24788 using the existing issuer/operator. A source-only, resumable
  add-later registration slice is implemented and tested, including an
  explicitly supplied operator trust override; the retained Velocity install
  has not been paired or deployed. A separate isolated workbench install
  verified add-later pairing and named login, and another source-built
  starter-directory install verified the bounded automatic scenario, including
  a repeat after Simulator restart. Neither is Velocity or package acceptance.
  New-install Simulator grants now include scoped
  `approval.self`, and scenario/budget approval routes require the active
  session permission. Control-Service distinguishes signed self-approval
  requests in its audit outcome. Existing backend grants are not silently
  expanded. Scenario selection now drives scoped preflight, eligible bounded
  budget self-approval, plan self-approval and run submission in source; focused
  browser coverage passes for the automatic sequence, blockers, stale selections
  and uncertain run responses. The in-memory plan permit is bound to the named
  operator issuer/subject (or the explicitly unauthenticated local mode), and
  issuance and run admission recheck `scenario.run` and `approval.self` in the
  current session. Session grants are snapshots from login, not a dynamic
  revocation feed. For automatic bounded budget approval, Simulator sends
  explicit self-approval intent through the independent budget contract; the
  current session and backend delegation grant must both include
  `budget.approve` and `approval.self`. Ordinary budget approvals without that
  intent retain their existing contract. This intent does not establish a
  recorded requester or prove an observed self-approval. Run namespace
  preparation precedes permit consumption. Since 2026-10-31 run admission
  durably records allocation before permit consumption and the locally
  observed terminal state at finish, and a scoped readback endpoint reports
  `uncertain` for an allocation without a terminal record, never retrying
  automatically; since 2026-11-01 (`97c1d2e`) issued and consumed plan
  approvals are also journaled durably per organization, so permit recovery
  is restart-safe under the contract below.
  After the plan, budget and session are rechecked, the same named operator (or
  explicit local mode) can recover an unconsumed permit for the exact
  organization, scenario and plan digest, including after a Simulator restart:
  re-approval through the full recheck returns the original approval
  identifier from the durable journal, while direct consumption of a
  pre-restart identifier without that explicit re-approval is refused.
  Another operator cannot recover it
  and remains blocked while that scope is pending, across restarts. Consumed
  approvals are never resumed; durable consumption precedes in-memory
  consumption, and an unavailable or corrupt permit journal fails closed
  (a failed durable append keeps the one-use approval intact).
  A domain-separated,
  non-secret permit fingerprint links an approval response to the current
  in-memory run. After an ambiguous run response, the UI makes one bounded
  catalog read and recognizes only a matching fingerprint, plan and scope; it
  never retries the run automatically. An absent, replaced or unavailable
  snapshot is still unknown, not proof that no run started. This is neither
  durable run reconciliation nor production Audit evidence. Velocity pairing
  and named login and installed-package
  acceptance remain open. Do not reuse
  Room secrets/signing keys or ask for the approved pairing again.
- Existing exact-ID traffic grants remain effective. Broader role/profile-based
  policy is a separately reviewed proposal, not permission granted by this plan.
- **Approved decision (2026-11-01):** per-change user approval is no longer
  required for policy changes: the submitter and approver may be the same
  named operator through the existing organization-scoped `approval.self`
  review path, with the Audit role-risk warning recording the self-approval.
  The Simulator and Control Room may therefore submit and approve policy
  changes without separate user input. Audit attribution, the named-session
  and delegation-trust gates, and guarded readback evidence remain required.
- **Approved decision (2026-11-01):** an endpoint-authentication authority
  (the evaluation BAS) is part of the evaluation-install minimum: the
  documented minimal network already requires a configured authentication
  source, and an install without one cannot admit any endpoint. New installs
  should provision it at creation with the BAS root in the initial reviewed
  policy; existing installs use a maintained resumable add-later command
  (replacing the per-organization scripts), with the production contract
  staying a pluggable authentication/attribute-source interface.
- DNS locates services; it does not establish identity, trust or authorization.
  Pre-admission discovery needs an independently trusted bootstrap address.
  Nodes follow the [infrastructure join model](System%20release%20plan.md#multi-host-role-based-deployment),
  not generic adapter enrollment. Endpoint issuance follows the
  [provisioning contract](../Provisioning%20plan.md).

### Ordered milestones and exit gates

Execute in order; keep Velocity the only live qualification target until G5
passes. Each implementation slice includes focused tests and maintained
instructions. No full platform rewrite is a prerequisite.

| Gate | Deliverable | Required proof before proceeding |
| --- | --- | --- |
| **G0: Freeze contracts and acceptance** | Trace existing enrollment/issuance/readback, operator pairing and DNS contracts. Record any genuinely missing public operation before adding orchestration. Pin original Velocity scenario, runtime artifacts and immutable approval scope. | Each step has a supported operation, state owner and authoritative result. Full runtime configuration changes invalidate resource approval. No private DB writes, shared bootstrap keys or undocumented shell fallback. |
| **G1: Establish operator access and automatic self-approval** | Implement resumable add-later registration of the approved separate Velocity Simulator client/signer and scenario-selection-triggered named self-approval. Define the same operator grant configuration contract for every Simulator installation; preserve Room and retained install recovery/integrity. | Real named OIDC login at 24788; authorized scenario selection submits, self-approves and proceeds with no extra approval prompt. Reviewer/missing domain grant/expired session/cross-org cases fail closed. Login and passive organization browsing make no changes. Restart retains pairing. Room approval UX and availability remain unchanged with Simulator stopped. |
| **G2: Automate endpoint onboarding** | On explicit organization activation, wire inventory comparison -> endpoint key generation -> enrollment request -> authorized automatic self-approval -> issuance/readback -> protected runtime handoff. Persist operation IDs and endpoint identity references. Reuse existing helpers. | One genuinely new endpoint enrolls from explicit activation without credential preparation, token retrieval or approval dialogs beyond any separately retained activation-safety confirmation. Retry/restart produces no duplicate identity; interrupted issuance reconciles safely. Unapproved/revoked endpoints cannot join. Missing or expired authorization is explicit, never a fallback. |
| **G3: Replace brittle discovery** | Use independently configured bootstrap endpoints before admission; use published production DNS/service contracts afterward. Remove unnecessary fixed workload/service addresses. Use only discovery mechanisms actually supported by those contracts. | After service restart/address change, the client resolves the published service and reaches it through ZPR. Missing/stale DNS, wrong certificate and unreachable enrollment report distinct failures; no legacy DNS-container edits. |
| **G4: Qualify the actual Simulator path and evidence** | Run the unchanged original Velocity scenario through named login, explicit organization activation, scenario-selection-triggered preflight/self-approval and the real scenario manager, including parallel scheduling. Verify the separate triggers and production-owned self-approval evidence; connect Audit warning rules/assertions. | No post-login input except explicit organization activation and scenario selection. Passive organization selection submits no activation or scenario mutation; activation does not launch a scenario; scenario selection does not activate/reset an organization. Original owner sessions, controllers/workloads, HTTP 200, exactly 100 warmed samples and 16,777,216 bytes within the original 75-second deadline; full cleanup. Repeat, restart, cancellation and partial-start failures leave no owned runtime/listener leaks. Foreign/stale resources are not silently removed. A risky-role fixture yields an evidence-linked Audit warning without a blocking confirmation; missing evidence is labeled, never reported as safe. |
| **G5: Package and deploy dev** | Pin matching package/config/catalog/registry, verify authoritative policy readback, qualify installation/recovery, then deploy on the existing dev ports. Keep stable unchanged. | Installed artifacts match tested hashes; UI acceptance reruns from the package without source/toolchain or shell repair. Production health works with Simulator unavailable. Maintained environment page reports actual versions/status and development credentials only. Record verified rollback procedure. |
| **G6: Expand the proven path** | Apply the same orchestration to Northstar, Great Lakes, Load Lab and Redwood; do not create per-organization enrollment scripts or generic replacement scenarios. | All five original organizations and all 17 original scenarios pass their own expected outcomes, organization switching, isolation and retained restart checks. Update dev status only with the measured acceptance matrix. |

G2 first uses an exact test endpoint within the operator's authorized scope,
automatically self-approved without broadening existing Velocity traffic
permissions. Its onboarding proof and the original retained
scenario proof are distinct. G4/G6 must exercise the maintained onboarding path
when a scenario needs a genuinely new endpoint, not rely solely on the original
pre-enrolled diagnostic inventory.

### G1 registration implementation record (2026-10-30)

Source implementation with isolated source-built evidence; the G1 gate remains
open. The maintained resumable add-later
registration is implemented and unit-tested in the bootstrap:
`zpr-lab-bootstrap -evaluation-simulator-pairing` (and `./demo.sh pair` in the
installed package) registers the separate `zpr-simulator` OIDC client with a
new independent secret and an independent Simulator delegation signer pinned
to its own CA-issued mTLS client certificate on an existing stopped install,
through a recorded, resumable, fail-closed transaction that preserves the Room
client/secret/signing key, retained operator identities, frozen inventory
integrity and admin recovery. Recorded preparation and commit recovery verify
owned staging, retained targets, directly referenced operator inputs and
stopped ports; unrelated suffix-matching files are never discarded. Changed or
unrecorded files and older plans without preservation fingerprints fail closed.
New installs with `-simulator-port` now freeze
the same independent signer, and the paired companion uses the Simulator-owned
signer and transport identity instead of the Room's; the runtime-written
delegation trust carries both pinned keys. Usage and override implications are
documented in the
[evaluation runbook](zpr-dashboard/packaging/evaluation/README.md#add-later-simulator-pairing-for-an-existing-installation).
Not done: no deployment or live Velocity registration at 24788 occurred.
Scenario selection now performs
scoped preflight, eligible bounded budget self-approval, plan self-approval and
run submission in source, with focused browser tests in
[`evaluation-scenario-budget.spec.mjs`](zpr-dashboard/tests/browser/evaluation-scenario-budget.spec.mjs),
[`scenario-plan-preflight.spec.mjs`](zpr-dashboard/tests/browser/scenario-plan-preflight.spec.mjs)
and
[`scenario-scoped-preflight-blockers.spec.mjs`](zpr-dashboard/tests/browser/scenario-scoped-preflight-blockers.spec.mjs).
These tests alone do not establish Velocity pairing, package acceptance or the complete
reviewer/expiry/cross-organization gate. The source-only
[`simulator_scenario_principal_test.go`](zpr-dashboard/cmd/zpr-web-dashboard/simulator_scenario_principal_test.go)
also exercises issuer/subject and local/named isolation, organization scope,
permission/session denial, revalidation after budget resolution, cancellation,
replay and concurrent consumption through the middleware and scenario manager.
It also exercises real operator-auth sessions against a test OIDC provider,
including CSRF, another operator and logout; its mid-request permission changes
use a mutable authorizer fixture. The real session implementation rechecks the
grant snapshot captured at login, not live grant revocation. A separate
[`simulator_budget_self_approval_test.go`](zpr-dashboard/cmd/zpr-web-dashboard/simulator_budget_self_approval_test.go)
exercises the actual Simulator budget route, mTLS and signed delegation to a
local independent authority, including missing backend permission and changed
session/actor. It does not prove Velocity integration or that ordinary approvals
have a recorded requester. No durable production-owned scenario approval or
run evidence is established by these tests. The per-installation operator
grant contract remains open; durable scenario-run permit reconciliation was
closed on 2026-11-01 (`97c1d2e`, see the scenario-execution record below). A
local TLS browser/service fixture also tests same-operator permit recovery and
matching-run readback after synthetic lost responses.

**Isolated workbench evidence (2026-10-31):** On Ubuntu, a fresh source-built
installation was paired add-later with a protected operator trust override.
The original named grants and Room identity were retained; real Room and
Simulator logins used the same named operator and exact test organization.
With Simulator stopped, the paired Room and issuer remained healthy; after
Simulator restart, login and the ordinary-client activation guard were
rechecked. A separate fresh `-scenario-demo` installation then completed
preflight, eligible budget self-approval, plan self-approval and one run
automatically from scenario selection. Its fifteen fresh peer probes matched,
including six Visa-denial checks. After a Simulator-only restart, a new run
used the retained approved budget and again verified all fifteen probes; CA,
client, signer and endpoint identities were unchanged. The temporary installs
were stopped and removed. This is starter-directory evaluation evidence, not
Velocity, onboarding, durable recovery, matched-package or gate acceptance.

**Launcher install-shape support (2026-10-31, source only):** the paired
Simulator launcher now starts against an operator-directory install whose
subject links seed the reserved `evaluation-subject` health identity, and
against an install without bootstrap-admitted evaluation endpoints when the
install-owned data plane exists — the two shapes of the retained Velocity
installation. Without admitted endpoints the published install omits the
endpoints file, and the organization profile and manifest say no
bootstrap-admitted endpoints exist; nothing claims live endpoint traffic.
Unsupported shapes (operator directory without subject links, no data plane
and no admitted endpoints, unpaired install) fail closed with specific errors.
Focused launcher tests cover the supported and rejected shapes.

**Velocity pairing deployment (2026-10-31):** the retained Velocity
installation (`0422ecb887a5106caea70975`, operator directory
`dc=velocity,dc=test`, no bootstrap-admitted endpoints) was paired live on the
workbench through the recorded add-later transaction: BAS adapter and install
stopped cleanly, offline state backup retained, pairing registered the
separate `zpr-simulator` client and independent signer for port 24788 against
issuer 24556, and the install restarted fully healthy with the recorded port,
the derived paired service bindings and the pairing-capable
`bin/zpr-lab-bootstrap.simulator-r10` selected by the maintained
`organization-runtime.sh` r10 rule (unpaired organizations keep the shared r9
binary; SHA256SUMS updated with dated backups). The paired Simulator runs as
user unit `zpr-simulator-dev-velocity` in `ordinary-client` mode with the
2026-10-31 source-built `zpr-simulator`. Verified: Room 200 and issuer
discovery 200 with Simulator stopped, Control-Service answering over mTLS,
Simulator 302 with named operator login configured, and a clean
Simulator-only restart. Still open for G1: human named UI login on Velocity,
scenario selection against the retained machine runtime, Audit role-risk
warnings, and the per-installation operator grant contract. At that
deployment the installed package bootstrap still lacked signer support and
this deployment used
source-built binaries from a task-owned directory; the later
installed-package qualification record (2026-11-01, `5cfed89`) supersedes
that gap. The retained install's
independent downstream trust (Enrollment Service) is unchanged.

**Enrollment Service trust preparation (2026-10-31, source only):** toward G2,
the new resumable add-later command
`zpr-lab-bootstrap -evaluation-simulator-enrollment-trust` derives
`simulator-paired-enrollment-trust.json` for the separate Enrollment Service
administration listener from an existing completed pairing. It adds only the
already-registered independent Simulator signing key, pinned to the exact
enrollment relay transport certificate that presents forwarded delegations,
and preserves the audience, the retained Room key entry and every named grant
byte for byte; no permission is added or widened. It requires recorded paired
operator service bindings with an Enrollment Service relay and fails closed on
unpaired installs, foreign audiences or keys, differing existing targets or
staging, and inputs that are its own output; the operator-owned input file is
never modified and an exact rerun verifies instead of rewriting. Focused tests
cover derivation, resume after an interrupted rename and the refusal cases.
**Deployed on Velocity the same day:** with all Velocity units stopped, the
task-owned `zpr-lab-bootstrap-r11` derived the trust from the staged
`delegation-trust-self-approval.json`, the Enrollment Service configuration
now points at the derived file (dated backup retained, SHA256SUMS unaffected),
and the install restarted fully healthy: all four enrollment listeners up,
Room 200 and issuer discovery 200 with Simulator stopped, administration mTLS
still refusing unverified clients, Simulator 302 after its transient unit was
recreated. See the
[dev environment record](environments/dev.json). This registers signer
acceptance only: no Simulator-signed enrollment administration call has been
exercised, and G2 inventory comparison, endpoint key generation, enrollment
requests and issuance/readback wiring remain open.

**Simulator enrollment read and session-grant override (2026-10-31, source
only):** the Simulator web backend now serves
`GET /api/simulator/organizations/{organization}/enrollment/invitations`: the
named operator session is authorized for the enrollment `read` permission at
the route policy and again before signing, the request is bound to the exact
initiating operator, a delegation is signed with the Simulator's independent
signer and relayed by the paired install's Control-Service to the Enrollment
Service. It performs no mutation and fails closed without a named session, a
missing enrollment read grant, a changed operator, an unconfigured signer or
an unknown paired install; focused tests verify the forwarded delegation
against the Simulator key behind a synthetic mTLS relay and all refusal
paths. The paired launcher now passes the independent Simulator delegation
signer to every paired install (previously scenario-demo only) and accepts an
explicit operator-owned `-simulator-oidc-file` session override, validated
against the frozen verified configuration: identity, issuer, client,
callback, lifetime and organizations are preserved exactly, retained
permissions cannot be removed, and only explicit enrollment administration
permissions plus bounded `budget.approve` may be added — never
`approval.self`, which keeps its own earlier explicit mechanism. The frozen
file is never modified. **Deployed and exercised live on Velocity the same
day:** with user approval, the explicit override
`enrollment/velocity/simulator-oidc-enrollment-read.json` adds exactly the
enrollment `read` and bounded `budget.approve` permissions to the existing
named administrator; the task-owned `zpr-simulator-r3`/`zpr-lab-bootstrap-r12`
replaced only the Simulator transient unit. A real named OIDC login through
the separate `zpr-simulator` client then performed the first live
Simulator-signed enrollment contract call: the invitations read returned
HTTP 200 with the five retained approved machine-runtime invitations after
Control-Service relayed the delegation and the Enrollment Service verified it
against the derived paired trust; unauthenticated and pre-override reads
failed closed. See the
[dev environment record](environments/dev.json). Still open: enrollment
mutations from the Simulator, a Simulator UI surface for the read, the
backend budget-authority grant (live scenario budget self-approval still
fails closed), and G2 endpoint key generation and
issuance/readback wiring.

**Activation readiness inventory comparison (2026-10-31, source only):**
`GET /api/simulator/organizations/{organization}/enrollment/readiness` now
implements the non-mutating first step of G2 activation wiring: it compares
the exact reviewed retained Velocity endpoint inventory (the shared
five-asset list also enforced by the retained machine lifecycle) with the
organization's current enrollment state read through the same authorized
operator-bound relay, and reports per-endpoint status
(`approved-retained`/`not-approved`/`approval-expired`/`missing`) plus every
exact missing prerequisite. An organization without a retained
machine-runtime inventory contract truthfully reports that single
prerequisite without calling enrollment; an upstream denial or invalid
response reports unknown-not-ready explicitly; nothing starts setup, and
explicit activation itself remains blocked in ordinary-client mode
(U01-ACTIVATION). Focused tests cover the ready and mixed comparisons and the
fail-closed paths. **Live on Velocity the same day** with `zpr-simulator-r4`:
after real named login, the readiness route returned HTTP 200 with
`ready=false` and exactly the truthful missing prerequisite — the paired
Simulator state has no retained machine-runtime inventory contract wired —
and the invitations read still passed. The activation-triggered onboarding
mutations remain open.

**Retained-binding launch support (2026-10-31, source only):** the paired
Simulator launcher now accepts an explicit operator-owned
`-retained-machine-runtime-file` containing exactly this installation's
retained topology binding and retained machine runtime (the contracts the
isolated r21/r22 consumer evidence already exercised). The launcher checks
the canonical protected path, strict JSON, version 1, the exact install ID,
the install-owned single-node data-plane container/network, the published
logical node and canonical absolute runtime paths; the Simulator revalidates
both authoritative contracts at startup, and the retained machine lifecycle
still verifies the referenced executable, identities and containers before
any use. The bindings are written into a separate frozen
`installs-retained.json` registry selected by the launch environment, so the
original verified registry is never rewritten; an exact repeat verifies and a
changed binding is refused rather than silently rebound. Focused tests cover
the frozen-registry selection, exact repeat, changed-binding refusal and the
unsafe-input refusals. **Deployed and exercised live on Velocity the same
day:** `zpr-lab-bootstrap-r13` launches the paired Simulator with the
operator-owned `simulator-retained-machine-runtime.json` derived from the
reviewed isolated r22 consumer configuration (bound to the paired install's
published `node`); startup validation accepted both contracts, and after real
named login the readiness route performed the genuine five-asset live
comparison: HTTP 200, `ready=true`, every retained endpoint
`approved-retained`, no missing prerequisites and no container drift. This is
inventory comparison only — no machine lifecycle operation, scenario run or
enrollment mutation was performed, and the G2 onboarding mutations
(key generation, invitation create, automatic self-approval,
issuance/readback, runtime handoff) remain open.

**Simulator enrollment mutations — invitation create/cancel (2026-10-31,
source only):** the Simulator web backend now relays exactly one explicit
operator-bound invitation mutation per request through the paired install's
ordinary contracts:
`POST /api/simulator/organizations/{organization}/enrollment/invitations`
(session `create` grant) and
`POST .../enrollment/invitations/{id}/cancel` (session `cancel` grant with
`expected` revision, fingerprints and a bounded reason). The organization is
always injected from the authorized path scope, bodies are strict JSON,
invitation references are validated, mutations are bound to the exact
initiating operator and re-signed per request, and the Enrollment Service
remains the sole lifecycle authority — the catalog's approved profiles/types,
preflight gate and idempotent store semantics all still apply upstream. No
endpoint key is generated and no approval is issued by these routes. Focused
tests assert the exact relayed mutation bodies behind the synthetic mTLS
relay and the fail-closed paths (missing specific grant, changed operator,
local mode, invalid references, unknown body fields).
**Deployed and exercised live on Velocity the same day:** with the explicitly
user-approved session override `simulator-oidc-enrollment.json`
(read/create/cancel/budget.approve added to the existing named administrator;
`approval.self` untouched) and `zpr-simulator-r5`, a real named login
performed the first live Simulator-signed enrollment mutations: invitation
create returned HTTP 201 for the new bounded test asset
`velocity-sim-onboarding-check` (catalog profile/type and preflight gates
enforced upstream), readiness remained `ready=true` and correctly scoped to
the required five retained endpoints, and the explicit cancel with the exact
revision and a bounded reason returned the invitation in state `cancelled`
with the named decision identity recorded. The five retained approvals were
untouched and no device claim, key issuance or approval occurred. G2 still
open: endpoint key generation, device claim/issuance, automatic
self-approval of a claimed invitation, protected runtime handoff, and
persisted operation IDs/endpoint identity references.

**Simulator new-endpoint onboarding operation (2026-10-31, source only):**
`POST /api/simulator/organizations/{organization}/enrollment/onboarding`
performs one explicit authorized workflow through ordinary contracts:
invitation creation via the authorized relay, fresh endpoint-owned RSA
enrollment and runtime key generation, the proof-of-possession device claim
against the separately configured Enrollment Service device listener
(launcher flag `-enrollment-device-url`, CA pinned to the install), a local
check that the service recorded exactly the locally generated fingerprints,
and automatic named self-approval bound to the exact claimed fingerprints and
revision — the backend independently requires the approve permission and
`approval.self` for the same named operator and its catalog/preflight gates
still apply. A persisted operation record (no secret material: the one-time
code is never returned and private keys exist only in process memory) covers
success and every failed stage; failures name the stage and the invitation
reference and are never retried or cleaned up silently. Focused tests cover
the exact fingerprint/revision binding, per-stage fail-closed behavior and
the unconfigured device contract. Protected runtime credential handoff and
issuance readback remain open.
**Exercised live on Velocity the same day (G2 core chain):** with
`zpr-simulator-r7`, the launcher's `-enrollment-device-url` and the
user-approved onboarding session override (adds `approve`), one explicit POST
onboarded the genuinely new endpoint `velocity-sim-onboarded-endpoint`:
invitation created, fresh endpoint-owned keys generated, proof-of-possession
device claim at the production device listener, exact local/recorded
fingerprint binding verified, and automatic named self-approval — final state
`approved` with the persisted operation record and no prompt, manual
credential or shell step after the explicit request. The first attempt's
state-name defect (`claimed` vs the service's `pending_approval`) failed
closed exactly as designed, and the stranded claim whose keys had been
discarded was cancelled through the explicit review path with the exact
fingerprints, revision and a recorded reason before the successful rerun.
The approved identity has no issued credentials, no runtime handoff and no
traffic grants. Still open for the G2 gate: issuance/readback, protected
runtime credential handoff, retry/restart duplicate-identity proof beyond the
store's active-invitation uniqueness, and wiring onboarding to explicit
organization activation under `U01-ACTIVATION`.

**Issuance readback, protected local handoff and duplicate-retry proof
(2026-11-01):** after automatic named self-approval, onboarding now performs
a proof-of-possession status readback with the same endpoint-owned keys
(requiring exactly the approved fingerprints and no credential envelope) and
writes a protected local handoff bundle — a 0700 directory with 0600 files
holding the endpoint-owned runtime and enrollment private keys plus a
non-secret identity manifest — under the Simulator workspace. A failure after
approval reports the approved endpoint's exact failed stage and is never
rolled back or silently retried; no secret reaches the response or the
operation record. This is the evaluation client's local handoff, not the
separate privileged installer or Keychain import contract. **Exercised live
on Velocity with `zpr-simulator-r8`:** `velocity-sim-onboarded-endpoint-2`
completed the whole chain in one explicit request — approved, readback
`approved`, protected bundle on disk — and an immediate duplicate retry for
the same asset failed closed at invitation creation (HTTP 409 from the paired
operator service), leaving exactly one approved identity per asset. Still
open for the G2 gate: adapter/BAS credential issuance for onboarded
endpoints (the local bundle is not an admitted runtime), wiring onboarding to
explicit organization activation under `U01-ACTIVATION`, and the Simulator UI
surface.

**Explicit BAS runtime credential check (2026-11-01, source only):**
`POST /api/simulator/organizations/{organization}/enrollment/credential-check`
exercises the endpoint side of the production BAS runtime credential contract
for one onboarded endpoint: the handoff bundle's runtime key (verified
against the recorded approved fingerprint before any network call) requests a
one-time nonce from the separately configured BAS adapter listener (launcher
flags `-enrollment-bas-url`/`-enrollment-bas-authority`/
`-enrollment-bas-ca-file`) and proves possession with an RSA signature; the
resulting one-time authorization code is deliberately discarded unread, since
redemption belongs exclusively to the pinned Visa Service peer and the code
expires on its configured short lifetime. Success updates the persisted
operation record (`runtime_credential_check: bas-authorized`); a denied
proof, mismatched bundle, missing bundle or unconfigured contract fails
closed with a distinct status. This proves the approved runtime key is
accepted by the production issuer; it does not admit the endpoint, issue a
visa or grant any traffic. **Exercised live on Velocity the same day:** with
`zpr-simulator-r10` and the BAS contract flags, the onboarded endpoint's
runtime key completed the live nonce/proof exchange against the production
adapter listener (`bas-authorized`, code discarded unread) and an endpoint
without a bundle failed closed. The live duplicate-retry evidence also
exposed and fixed a record-persistence defect: attempts that never reach
approval now persist to a separate per-asset attempt file and can no longer
clobber the canonical approved record, whose full lifecycle on Velocity now
reads approved → readback approved → local-bundle handoff → bas-authorized. **Ordinary-client activation wiring (2026-11-01, source
only at first):** with `SIMULATION_SETUP_MODE=ordinary-client`, confirmed
explicit web activation now performs the ordinary authorized setup instead of
rejecting: the required retained inventory comparison runs through the
authorized contracts, activation records the organization selection only when
every requirement is retained and approved (never invoking the legacy
activation lifecycle), and otherwise returns HTTP 409 with every exact
missing prerequisite and no setup started. Restore stays blocked
(`U01-ACTIVATION`), unconfigured delegation or a non-named session fails
closed, and the scenario-running and machine-session conflicts still apply.
Focused tests cover the verified activation with the recorded selection and
the fail-closed missing-prerequisite refusal; the existing guard tests and
browser fixtures (which mock the backend denial) continue to pass.
**Exercised live on Velocity the same day** with `zpr-simulator-r11`: after
real named login, explicit activation returned HTTP 200 `ready-verified` with
the full five-asset readiness report and recorded the organization selection;
Restore remained blocked (the live session grant also lacks
`organization.restore`, so the route policy denies before the handler's own
guard); production stayed untouched. Still open
for the G2 gate after the same-day declared-endpoint record below: the
Simulator UI surface; token redemption
remains exclusively the pinned Visa Service peer's, and no onboarded endpoint
has a visa or traffic grant.

**Profile-declared endpoint onboarding at activation (2026-11-01,
`544a335`):** organization profiles may declare required endpoints
(`declared_endpoints`: complete bounded asset fields, unique identifiers, no
overlap with the reviewed retained inventory, at most 32). The authorized
readiness comparison includes declared endpoints in the required inventory,
and explicit ordinary-client activation onboards exactly the genuinely new
declared endpoints — those the readiness readback reports with no invitation
at all — through the maintained explicit onboarding chain, then re-verifies
readiness before recording the selection; onboarding evidence alone never
substitutes for the readiness readback. Required endpoints in any other
unready state fail closed to the ordinary prerequisite report, so unapproved
or revoked endpoints never join and interrupted issuance reconciles through
the explicit review path, never an automatic retry. Focused tests cover the
onboarded activation, the fail-closed unapproved and stopped-onboarding
paths and the profile contract refusals. **Proven live on Velocity** with
`zpr-simulator-r15` and an operator-owned catalog variant declaring
`velocity-declared-probe`: readiness reported the declared endpoint missing;
one explicit activation created the invitation, performed the device claim
with fresh endpoint-owned keys, automatic named self-approval (revision 3),
proof-of-possession readback and the protected local key bundle, then
re-verified readiness (all six approved-retained) and returned
`ready-verified`; an immediate repeat activation returned HTTP 200 with no
duplicate identity, and scenario preflight afterwards remained plan-only
with no blockers. This completes the G2 automatic-onboarding acceptance for
the Simulator-side chain; the declared endpoint has no visa or traffic
grant, and custom-organization launch additionally needs its own paired
production install.

**Installed-package paired qualification (2026-11-01, `5cfed89`):** the
packaged Simulator launch (`simulator.sh`) accepts the operator-owned
paired-operation options (`--simulator-oidc-file`,
`--retained-machine-runtime-file`, `--enrollment-device-url`,
`--enrollment-bas-url`, `--enrollment-bas-authority`,
`--enrollment-bas-ca-file`) and forwards them verbatim to the packaged
bootstrap, which revalidates every file and contract; operator files must be
absolute existing paths, enrollment URLs HTTPS, and unrecognized options are
refused before the bootstrap starts. The maintained packaging test covers
option forwarding and the fail-closed validations. **Qualified live on
Velocity:** `package.sh velocity-paired-20261101` built matching ZPR and
Simulator archives on the workbench from clean source `5cfed89`
(`source_dirty=false`, user-local Go 1.27.1, live data-plane image
`c820f183…` pinned); both archives verified against their SHA256SUMS and
paired build identity; the Simulator then ran purely from the extracted
package with a fresh private state (`SIMULATION_SETUP_MODE=ordinary-client`,
`--full-catalog` plus the paired-operation options referencing the
operator-owned Velocity configuration) and completed the whole authorized
chain: named login, explicit activation `ready-verified`, scenario catalog
load (which seeds the published workspace revisions), bounded budget
self-approval HTTP 201, plan self-approval, run accepted HTTP 202 and
`velocity-single-node` completed 20/20 against the retained install. Honest
limits: the archives were built on the same workbench host, not a separate
clean build host; browser-UI acceptance from the package was not rerun (UI
verification is deferred); the package retains its
`status=engineering-unqualified` label, and G5's dev-port deployment,
rollback record and maintained environment page remain open.

**Per-organization retained machine inventory (2026-11-01, `d7239a4`):**
toward G6, the retained machine-runtime binding may declare its exact
inventory (`control_asset` plus `machines`, each with `machine_id`,
`controller_asset`, `workload` and `workload_asset`); a binding without an
explicit inventory keeps the exact reviewed retained Velocity contract, so
existing deployments are unchanged. The retained lifecycle (container
identities, control TLS verification, peer teardown, machine recognition),
the enrollment readiness comparison and the declared-endpoint collision rule
all use the binding's effective inventory instead of the previously
hardcoded Velocity list; validation is fail-closed (all-or-nothing
declaration, complete bounded identities, unique machines and assets, only
workloads with a maintained runtime configuration), and the launcher
preserves an explicit inventory in retained bindings while enforcing the
all-or-nothing shape. Focused tests cover the Velocity default, a declared
non-Velocity inventory, container-argument derivation and every refusal.
**No-regression proof:** the package was rebuilt from clean `d7239a4`
(`velocity-paired-20261101-r2`) and the full authorized chain re-passed live
from the installed package on the default-inventory path (activation 200,
catalog 200, budget 201, run 202, `velocity-single-node` completed 20/20).
This unblocks non-Velocity single-node organizations Simulator-side; still
open for G6: multi-node retained topology (the binding still requires a
single-node data plane), per-organization pairing/enrollment/control-TLS
provisioning for each remaining organization, and the full five-organization
seventeen-scenario acceptance matrix.

**Northstar paired live (2026-11-01, user-approved):** the recorded add-later
pairing registered a separate Northstar Simulator client and independent
delegation signer at port 20788 against the stopped install. Because the
original Northstar operator services lacked a delegation trust override, the
operator-owned `enrollment/northstar/control-delegation-trust-self-approval.json`
was derived from the install's canonical `operator-delegation-trust.json`
(admin grant plus `approval.self` and `budget.approve`, mirroring the
reviewed Velocity pattern; dated backup of the operator services). The
derived paired Control-Service trust therefore already carries both
permissions, with the Room key and viewer grant unchanged. The Simulator
enrollment trust was derived and the Enrollment Service administration
listener repointed (the setting belongs under `administration`, unlike the
Velocity layout with its BAS section). The install restarted paired and
healthy (Room 200, issuer discovery 200, scenario-capable Control Room via
the generic paired runtime branch), and a second Simulator instance launched
at 20788 **from the same installed `velocity-paired-20261101-r2` package**
with a Northstar session override (onboarding permissions plus
`budget.approve`). Live evidence: named login against Northstar’s own
issuer; the first Simulator-signed Northstar enrollment read returned the
eight pre-existing claimed invitations (`pending_approval`, including exactly
the `client-service` scenario assets for machine-03/machine-05); readiness
truthfully reports that no retained inventory or declared endpoints are
configured; explicit activation fails closed with that exact prerequisite;
the Velocity Simulator at 24788 was unaffected throughout. Still open for a
Northstar scenario run: resolution of the eight stranded `pending_approval`
claims through the explicit review path, a `northstar-simulator-control`
identity, issued runtime credentials (the install has no BAS section;
admission credential provisioning is unresolved), Northstar control-TLS
identities and the operator retained-bindings file (callback upstream
172.28.0.1), and verification that Northstar’s applied policy grants its
original scenario flows. The eight stranded pre-pairing `pending_approval`
claims were cancelled the same day through the explicit Simulator-signed
review path (exact revisions, fingerprints, recorded reason), leaving the
enrollment inventory clean.

**Maintained BAS provisioning design (approved 2026-11-01, next executable
phase):** per the recorded decisions, a resumable
`zpr-lab-bootstrap -evaluation-bas` add-later command replaces the
per-organization scripts and provisions the evaluation authentication
authority for an existing install, mirroring the reviewed Velocity artifacts
exactly:
1. *Key material* (new `enrollment/<org>/bas/`, 0700/0600): dedicated
   constrained CA (EC P-256, `CN=<Org> BAS runtime CA`, critical
   keyUsage digitalSignature+keyCertSign, basicConstraints critical CA:TRUE
   pathlen:0); `auth.zpr-runtime` TLS server identity (SAN `DNS:auth.zpr`,
   EKU serverAuth, CA-signed); `<org>-visa-service-runtime` TLS client
   identity (EKU clientAuth); 48-byte random `token-signing.key`; RSA-3072
   `root-adapter.key` (the BAS root adapter ZPR identity, public-key SHA-256
   pinned in the adapter script and recorded).
2. *Enrollment service `bas` section* (ports from the install inventory,
   Velocity pattern 24996/24997 -> Northstar 20996/20997): adapter and token
   listeners with the runtime identity, `claim_sources`
   asset_id/organization/profile/type, single-store `database_file` =
   enrollment `registry.sqlite`, `signing_key_file`, `visa_service_ca_file`
   = the new CA, `visa_service_peers` pinned to the install VS runtime
   certificate SHA-256; dated backup and Enrollment-Service-only restart.
3. *BAS root adapter* host script and user unit (per-install container on
   the install data-plane network, `ph` as adapter CN
   `<org>-enrollment-bas`, reviewed-image/evidence/ownership checks exactly
   as `velocity-bas-adapter-host-r12.sh`).
4. *Authentication-root policy and configuration change*, driven by the
   Simulator/Control Room named session through the maintained
   Policy-Service change-review pipeline under the recorded self-approval
   decision (submit and approve as the same named operator; Audit role-risk
   warning records it): policy record + revision appending only the
   `<Org>EnrollmentAuth` trusted-service definition for adapter CN
   `<org>-enrollment-bas` (never workload access), staged with the updated
   trusted-services configuration carrying the root adapter public key,
   applied with guarded readback evidence (Velocity precedent:
   `operator-inputs/velocity/bas-root-r13` +
   `policy/velocity/bas-root-r13-evidence`).
5. *Operator services* gain `visa_service_read_only_files` for the BAS
   materials the Visa Service reads, mirroring Velocity.
Acceptance: enrollment-issued runtime credentials for a Northstar endpoint
complete the live nonce/proof exchange (`bas-authorized`) against the new
authority, the adapter container joins only the install-owned network, the
policy diff admits exactly the authentication root, and Velocity remains
untouched.

**Delivered and deployed (2026-11-01, `e317b4d`):** the maintained
`-evaluation-bas` command exists with focused tests (material shapes and CA
chain, single-store configuration extension with the pinned Visa Service
peer, emitted script contracts, resumable adoption, and fail-closed partial
material/differing section/missing data-plane/identical listener refusals).
Run live against Northstar: key material generated under
`enrollment/northstar/bas/`, the Enrollment Service configuration extended
(dated `.before-evaluation-bas` backup) and restarted serving both BAS
listeners at 10.0.1.160:20996/20997, adapter scripts emitted, root adapter
public key `0a66bc2494965ee2af00f72ec02e12f2d742b087af1b024f0c20d90e3246b578`
recorded. The command additionally emits (`05243cb`) the root adapter SPKI public key
`root-adapter.pub` and the reviewed compiler-config fragment
`bas-compiler-config-fragment.toml`, closing a gap found during apply: the
Velocity authentication root was both a compiler-configuration change (the
data-plane `dp-policy.zplc` gained the `[bootstrap]` root-adapter pin, the
`[trusted_services.<org>-enrollment-bas]` validation/2 authority with the
enrollment attribute mapping, and the `[services.<Org>EnrollmentAuth]` /
`[services.<org>-enrollment-bas-vs]` service ports) AND a policy-source
change (the `<Org>EnrollmentAuth` stanza). The maintained apply procedure,
reverse-engineered from the Velocity `bas-root-r13` evidence, is: (1) merge
the emitted fragment into the install’s `state/<org>/dp-policy.zplc`
(the fixed `ZPR_POLICY_STAGE_CONFIG_FILE` the Policy-Service compiles
against; dated backup); (2) append the emitted policy stanza to the
organization’s applied data-plane policy record through the Room/Policy
change-review APIs; (3) stage (compiles against the now-BAS-aware config),
self-approve the review as the same named operator under the recorded
self-approval decision, and apply with guarded readback. All of that completed live the same
day (`06e717c` and operator transactions):
- *Northstar authentication root applied through the first fully unattended
  self-approved policy change*: the operator-owned BAS-aware stage
  configuration lives beside the frozen verified one
  (`state/northstar/dp-policy-stage.zplc`; `organization-policy-service.sh`
  prefers it and the qualified `zplc-validation-companion-r13` compiler,
  with SUMS updates and dated backups, because the frozen `dp-policy.zplc`
  refuses edits at startup verification — the transient-swap approach the
  Velocity r13 evidence implies is replaced by this maintained override).
  The Room pipeline ran record → revision → stage → submit → self-approve
  (the recorded AUDIT WARNING path; session and enrollment trusts gained
  `approval.self` following the exact Velocity convention with dated
  backups) → apply: operation `LWU5P5HGWCDTTG2NSJ7KFL7ISE` succeeded,
  candidate `631ecb83…` applied to `northstar-vs` (version 2).
- *BAS root admitted and accepted*: the emitted adapter unit joined the
  install-owned network, the dock link became ACTIVE at the reviewed ZPR
  address, and after explicit activation onboarded the declared probe
  (`ready-verified`), the live nonce/proof exchange returned
  `runtime_credential_check: bas-authorized` (HTTP 200) — the full
  acceptance for the maintained provisioning design.
- *Cancelled invitations never block genuinely new onboarding* (`06e717c`):
  found live when a claim stranded by the missing backend `approval.self`
  grant was cancelled through explicit review but then permanently blocked
  declared-endpoint onboarding; the readiness comparison now reports a
  cancelled (explicitly reconciled, terminal) invitation as missing while
  rejected/pending/expired states keep failing closed.
- *Velocity migrated to the maintained command*: `-evaluation-bas` adopted
  Velocity’s existing material byte-for-byte (root `6f1ab5db…`, peer
  `3ecd72ea…`, configuration untouched), the BAS unit switched from the
  artisanal `velocity-bas-adapter-host-r12.sh` to the emitted maintained
  script (unit backup retained), and after a clean install restart the
  maintained adapter went ACTIVE and `velocity-single-node` completed 20/20
  from the installed package — retained machine admission authenticating
  against the maintained BAS stack.

**Scenario stack for paired installs (2026-11-01, source only):** four
changes prepare live scenario selection on the paired retained install.
Paired installs (recorded `simulator_port`) now host the bounded
Control-Service budget authority — pairing is the explicit opt-in — and the
paired Simulator always carries the budget-authority client; mutations remain
gated by the delegation trust grants. The operator delegation override
validation now also accepts bounded `budget.approve` added to an existing
named grant, only alongside `approval.self` in the same override grant.
The packaged-catalog launch integrates with a paired catalog organization:
the launcher skips its own minimal profile when the catalog supplies the
organization's original profile, the retained-binding validation accepts the
catalog profile's published node, and `catalog-oidc.json` is built from the
login configuration the prepared environment actually selects (preserving an
explicit operator-approved session override) with duplicate organization
scopes deduplicated. Focused tests cover the authority opt-in matrix, the
bounded budget.approve override rules and the catalog integration. At commit
time the live Velocity install still ran the r10 start binary without a
budget authority; the same-day scenario-execution record below supersedes
that deployment gap.

**Scenario execution completed live on Velocity (2026-11-01, `d86eb2e`):**
the full authorized scenario chain was proven on the paired retained install.
Deployed: the install's `bin/zpr-lab-bootstrap.simulator-r11` start binary
hosts the bounded Control-Service budget authority, both delegation trusts
carry `budget.approve` for the Simulator key (dated backups retained), the
packaged scenario catalog is installed, and paired installs run a
scenario-capable Control Room/Service build
(`bin/zpr-control-room.scenario-r12`, selected by `organization-runtime.sh`
only for paired installs) whose budget-approval decoder accepts the bounded
self-approval request. Source fixes completing the chain: scenario preflight
and bounded budget self-approval accept the reviewed
`retained-machine-bas-v1` contract alongside `evaluation-endpoints-v1`; the
retained path resolves the authoritative policy digest through the paired
install's guarded Visa Service admin readback (an unavailable readback keeps
the explicit policy blocker); the paired launcher serves the Simulator
machine-control listener at the retained binding's declared callback
upstream with the operator-generated control identity, so retained machine
controllers connect through the callback proxy; and paired manifests publish
the install's protected DNS (`fd5a:5052::53`) so scenario service
publication is read back from production-owned DNS. Live evidence
(2026-11-01, operator session at 24788): bounded budget self-approval 201
with persisted evidence, preflight blockers fully drained, plan self-approval
200, run accepted 202, and `velocity-single-node` completed all 20 steps —
retained machine containers, controller connections, logins, workload
start, production-DNS publication of `echo-web.svc.zpr.`, the 100-sample
HTTP round-trip and 16 MiB download assertions, and clean teardown.
Still open: scenario launch from the Simulator UI has not been operator-
verified end to end in the browser, and the remaining G2 UI surface stream is
unchanged.

**Restart-safe permit reconciliation (2026-11-01, `97c1d2e`):** issued and
consumed scenario plan approvals are journaled durably per organization in an
append-only fsynced JSONL file beside the run evidence journal (records carry
scope, approval identifier and the exact principal; no plans or secrets
beyond the one-use identifier). After a restart, the same principal resumes
an unconsumed approval with its original identifier only through the full
approval recheck; other principals stay blocked for that scope; consumed
approvals are never resumed; direct consumption of a pre-restart identifier
is refused until re-approval; the journal fails closed when unavailable,
corrupt or unwritable, and a failed durable append keeps the one-use
approval intact. Focused tests cover restart resume, cross-principal
blocking, local mode, consume-after-restart refusal, corrupt-journal
fail-closed, failed-append preservation and the memory-only store contract.
Live evidence on Velocity (zpr-simulator-r14): approval issued, durable
journal written, Simulator-only restart, direct run with the old permit
refused HTTP 409, re-approval returned the identical approval identifier,
and the resumed permit ran `velocity-single-node` to completion (20/20); the
journal records exactly issued then consumed for the named operator.

**Dynamic machine-control rendezvous through production DNS (2026-11-01,
`69e0e1b`, source only):** the retained machine-control contract no longer
assumes any fixed ZPR address (user-approved decision, 2026-11-01,
superseding the reviewed `fd5a:5052:adda:1:ffff:ffff:ffff:fffe` rendezvous
and the hard dependency on a Visa Service that honors `--zpr-addr`). The
callback adapter docks without requesting an address, discovers the address
the production Visa Service actually granted on its TUN interface, binds the
control proxy to exactly that address and records it; the host then fails
closed unless production-owned DNS publishes the control service name at
that granted address (readback through the node's `zpr-dns` namespace, with
explicit evidence naming the missing publication instead of a silent
`wait_controller` timeout). Machine controllers resolve the operator-declared
name — `control_dns_name` in the retained machine-runtime binding, default
`simulator-control.svc.zpr`, validated to the production `svc.zpr` zone —
through the install's protected DNS on every connection (new
`-control-dns`), so a re-granted address self-heals without restarts. Trust
in the control channel is unchanged and exclusively its pinned mTLS
identities (pinned CA, pinned server name, per-machine client certificates);
the DNS answer is only a dial hint. The retained runtime digest carries a
rendezvous-contract revision, so containers started under the superseded
fixed-address arguments are refused until explicitly stopped and recreated.
Evidence: focused tests for granted-address selection, per-connection
protected-DNS dialing, publication-readback fail-closed evidence and the
refused configured listener; full `cmd/zpr-web-dashboard` and
`cmd/zpr-lab-bootstrap` suites, vet and production build pass. The launcher's
strict retained-bindings schema accepts the optional `control_dns_name` field
(`ab1db7a`). Both applied Velocity and Northstar policies already carried
`provide SimulatorControlService at simulator-control.svc.zpr over TCP 8792`.
Residual: the legacy simulated-fabric `dashboard-stack.sh` single-node path
still binds the fixed address and its adapter launches still request it as a
hint; the multinode path already discovers the granted address.

**Northstar client-service completed live on the dynamic rendezvous
(2026-11-02):** deployed to the Northstar dev install (launcher r22,
simulator r18 from `ab1db7a`; Simulator state rotated to
`simulator-state-northstar-dns3` preserving `workspace/`). The readback
immediately surfaced, with explicit evidence, that the installed `9f2cb68c…`
image's Visa Service never published the service alias and — after policy
vinst 5 removed the superseded vinst-4 address pin (record `fbde61ce`
revision 3, self-approved review `JD7XE72V7MJOPCUHTV7O2QPP56`, apply
operation `LSNMC5SIDCBZGB4EKL3UHS2GOH` succeeded) — that the same image
denied controller flows outright (`eval denied (no match)`), because it
cannot evaluate the BAS-sourced attribute policy; the old image did publish
per-adapter identity records (`<asset>.adapters.svc.zpr`), so Northstar's
retained bindings declare `control_dns_name
northstar-simulator-control.adapters.svc.zpr`. The data plane was then
upgraded to `c820f183…` through the maintained `organization-runtime.sh`
image pin (SHA256SUMS updated, dated backups; binding `image_id` updated;
BAS adapter re-docked). Live result at 01:47Z: controllers resolved the
production-published control name to the dynamically granted callback
address per connection and `client-service` completed all 23 steps —
onboarded identities, BAS-authorized controllers, machine-control
heartbeats over ZPR, workloads, publication, traffic and verified clean
teardown. Velocity's Visa Service DNS publication had been failing since
23:51Z (stale noise session on its `vs-adapter`, `tun0` NO-CARRIER,
predating this work); one install restart plus a maintained BAS-adapter
restart restored publication (zero reconcile failures, records resolving).
Still open: the Northstar VS subject-snapshot checkpoint is uninitialized
(the subject-link publication cycle was never run for this install;
periodic fetch errors are logged and did not gate the machine scenario);
browser-UI work remains deferred.

**Packages rebuilt and Velocity requalified on the dynamic rendezvous
(2026-11-02):** matching `velocity-paired-20261102` ZPR and Simulator
archives were built on the workbench from clean `d1336b3` (user-local Go
toolchain, live `c820f183…` data-plane image pinned, both archives verified
against their SHA256SUMS), installed beside the retained r2 packages, and
the Velocity transient unit switched to the new installed Simulator with its
retained bindings moved to the shared r23 controller (dated backup) and the
default production control name `simulator-control.svc.zpr`; private
Simulator state rotated to `simulator-state-r4` preserving `workspace/`.
Live result: the full authorized chain passed and `velocity-single-node`
completed all 20 steps from the installed 20261102 package with verified
container cleanup — both paired organizations now run the dynamic
rendezvous from package-built binaries of current main, closing the
mixed-version gap. The 20261102 archives keep the engineering-unqualified
label: they were built on the same workbench host, not a separate clean
build host, and browser-UI package acceptance was not rerun.

**Load-lab paired foundation on the maintained paths (2026-11-02):** load-lab
was brought up as the third paired organization entirely through maintained
and scripted paths, deliberately stopping before any net-new hand-built
provisioning. Done: maintained Simulator pairing
(`-evaluation-simulator-pairing` against the stopped install, port 22788);
the self-approval trust derivations mirroring the reviewed Velocity/Northstar
pattern (control delegation trust with `approval.self`+`budget.approve`,
enrollment trust and operator-oidc gaining `approval.self`, administration
listener repointed, operator services pointing at the derived trust, all with
dated backups); data-plane image pinned to `c820f183…` through the maintained
`organization-runtime.sh` branch (SHA256SUMS updated, dated backups); the
three org services started healthy (Room 22787 HTTP 200, issuer 22556 HTTP
200); maintained BAS provisioning (`-evaluation-bas`, listeners
22996/22997); and the BAS authentication root applied through the fully
self-approved policy pipeline (record `176f5749…` revision 2, review
`FXKM6A6LMHDXN6BQKELZYOAODH`, apply `OYA2NOLPBS56IRPDCOBFGL7OVO` succeeded;
the compiler-config fragment was merged into an operator-owned
`dp-policy-stage.zplc` with the bootstrap key folded into the existing table,
not duplicated) — the BAS adapter then docked ACTIVE. Control-TLS identities
were generated with the generic `prepare-retained-machine-control.sh`.
**Deliberately not started** (recorded as the maintained-provisioning gap,
not a hand-build task for tonight): load-lab has no workload-access or
`SimulatorControlService` provider policy, no declared-endpoint catalog
variant, no onboarded retained identities and no retained bindings. Unlike
Northstar — which carried those from prior sessions — load-lab would require
building its entire scenario workload policy and retained identity set from
scratch; that is exactly the per-organization provisioning that the design
backlog item below says should become a maintained command before scaling to
the remaining organizations. `load-lab-fanout` has not run.

**Load-lab completed the on-the-fly way through the authorized chain
(2026-11-02, evening):** continuing under the approved on-the-fly decision,
load-lab's five scenario identities (simulator-control service-adapter, two
machine controllers, two workloads) were onboarded entirely through the
maintained single-call onboarding endpoint — invitation, endpoint-owned key
claim, named self-approval and protected bundle handoff, five HTTP 201s — with
no hand-built key material; endpoint identity directories were assembled
mechanically from the delivered bundles (CA = the BAS CA). Prerequisites
found and satisfied through maintained paths: the enrollment administration
trust must be the pairing-derived
`state/<org>/simulator-paired-enrollment-trust.json`
(`-evaluation-simulator-enrollment-trust` against the stopped install — the
hand-derived trust lacks the Simulator signing key and fails closed 403), and
invitation creation requires a current organization-scoped enrollment
readiness preflight attestation (saved through the Room, mirroring the
recorded Northstar attestation with load-lab's development CA fingerprint;
absent preflight fails closed 409). The retained bindings use the shared r23
controller, the granted-address contract and the default
`simulator-control.svc.zpr` control name; the enrollment catalog gained the
simulator-control profile and service-adapter type (dated backup); the VS
gained the `trusted_service_http.load-lab-enrollment-bas` section and both
operator service bindings the BAS read-only files (dated backups). **The
first fully class-based workload policy was then applied** (record `176f5749`
revision 3, self-approved, apply operation `MQOXBMBBPA4XPABSNWJB7G4FA4`
succeeded, vinst 3): endpoint classes match only BAS-verified
organization/profile/type claims — no per-asset identity pins anywhere — with
`SimulatorControlService` provided at `simulator-control.svc.zpr` and 200
generated fleet provides (`load-service-001..200`, TCP 10000-10199) with the
matching generated protocol/service compiler-config tables. Live evidence:
the post-apply visa sweep resolved every BAS-sourced attribute
(`allowed=14, skipped_unresolved=0`) — the class-based shape works on the
current image. **Remaining blocker, deliberately fail-closed:** the runtime
plan refuses `start_service_fleet`/`stress_traffic` ("load-test fanout is not
executable from a runtime plan", pinned by tests with fanout/service counts
recorded as separate unapproved budget inputs), and load-lab publishes only
fanout scenarios, so no load-lab scenario has run. Enabling fleet actions on
the paired runtime plan — workload declarations, fanout budget approval
semantics and live qualification — is the next bounded implementation slice;
it is a reviewed product gate, not a provisioning gap, and was not bypassed.

**Fleet actions qualified and load-lab completed live (2026-11-02, `6488d7e`):**
the runtime plan now executes `start_service_fleet` and `stress_traffic`
steps that exactly match the organization's configured load-test profile
(wrong machines, workloads or out-of-range client counts still fail closed;
fanout and service counts remain separate budget inputs bound to the
authorization scope; reworked focused tests pass with the full suite). The
load-lab Simulator moved to the task-owned launcher r22 with simulator r19
(built from `6488d7e`). One live blocker was diagnosed on the way: the
load-lab `vs-adapter` entered the same stale-noise HandshakeError loop seen
on Velocity the same evening (no egress link to the Visa Service fabric
address, BAS relay and fleet DNS reconciliation stalled); one install restart
plus a maintained BAS-adapter restart healed it. Live result: the full
authorized chain passed (budget 201, preflight no blockers, plan approval,
run 202) and `load-lab-fanout` **completed all 19 steps** — on-the-fly
onboarded identities, class-based visas, dynamic control rendezvous,
controllers connected, the 200-service fleet started on ports 10000-10199 and
200 randomized stress clients ran the two-minute fan-out with clean teardown
and verified container cleanup. Three organizations (Velocity, Northstar,
load-lab) now run live scenarios on the granted-address DNS rendezvous;
load-lab is additionally the first organization provisioned end to end under
the approved on-the-fly decision with a fully class-based workload policy.
Still open: `vs-adapter` stale-noise recovery is manual (an install restart)
and recurred twice in one evening — a maintained detection/recovery belongs
on the design backlog; load-lab's subject-link cycle, like Northstar's, has
not been run; offboard-at-teardown remains the next on-the-fly design piece.

**Load-lab scenario set complete (2026-11-02, late evening):** all four
published load-lab scenarios ran to completion through the full authorized
chain on the on-the-fly stack — `load-lab-fanout` (200 clients),
`load-lab-fanout-50`, `load-lab-fanout-100` and `load-lab-fanout-150`, each
19/19 with verified container cleanup. The `vs-adapter` stale-noise wedge
recurred once between the first fan-out and the variant batch (the node loses
its egress link to the Visa Service fabric address; BAS relay and DNS
reconciliation stall; the new callback readback surfaces it within two
minutes) and was healed by the one-restart remedy; the subsequent three runs
completed back-to-back without recurrence, so the wedge is intermittent and
correlates with heavy stress teardown. Recorded as a data-plane defect
needing maintained detection/recovery (same class as the DP-K03 adapter
re-dock work), tracked on the design hardening backlog.

**Offboarding design (proposed 2026-11-02, next executable on-the-fly
phase):** a maintained single-call offboarding endpoint mirrors onboarding —
`POST /api/simulator/organizations/{organization}/enrollment/offboarding`
with the asset ID and an explicit reason — and executes fail-closed recorded
stages: resolve the asset's enrollment record through the authorized relay
(exact revision and key fingerprints), revoke it in the registry, then remove
the Simulator workspace bundle only after the registry revocation succeeds;
operator-owned endpoint identity directories are reported for explicit
operator removal, never deleted by the Simulator; a partial failure persists
the operation record and stops. **Verified design constraint (2026-11-02):**
the Enrollment Service has no revocation path for approved enrollments —
`Cancel` accepts only `invited`/`pending_approval` and decisions are only
approved/rejected — so offboarding requires a new explicit, audited
registry operation (approved → revoked) before the Simulator endpoint can
exist, BAS token issuance must refuse revoked assets (the
300-second token lifetime bounds residual validity), and the Northstar
stranded-claim cancel path is NOT sufficient for approved assets.
Acceptance: an offboarded asset can no longer
BAS-authenticate or re-dock, its flows lose visas at the next sweep, and the
enrollment inventory and workspace are verifiably clean.
**Approved decisions (user, 2026-11-02):** revocation is gated by a new
distinct `revoke` permission (not a reuse of `approve`), and "teardown" means
explicit organization deactivation/reset — identities persist across scenario
runs and the bounded registry comes from deactivation-time offboarding.
Per-run offboarding remains the eventual full on-the-fly contract once
activation onboarding feeds the machine runtime directly, and would
additionally require automatic per-run onboarding and runtime bundle wiring
before it could hold.

**Registry revocation and the Simulator offboarding endpoint implemented in
source (2026-11-02):** the audited approved → revoked registry operation and
the single-call offboarding endpoint above exist in source with the full
suite, vet and dev/production builds passing; nothing has been exercised
live. The Enrollment Service store gained `Revoke` — the only exit from the
approved state, bound to the exact reviewed revision and both recorded key
fingerprints, revision-bumping and audited as `revoked` — exposed as
`POST /api/enrollment/v1/invitations/{id}/revoke` and gated everywhere
(enrollment principal configuration, operator-delegation grants, the
Control-Service relay proxy) by the approved distinct `revoke` permission.
A revoked asset leaves the active-asset uniqueness set, so a genuinely new
invitation can be created later, and Simulator enrollment readiness treats
`revoked` like `cancelled`: terminal, never blocking new onboarding. BAS
refusal is inherent and now test-pinned: every exchange stage re-reads the
registry requiring the approved state and the revocation revision bump
invalidates outstanding nonces and codes, leaving only already-issued tokens
for their bounded 300-second lifetime. The Simulator endpoint
(`POST /api/simulator/organizations/{organization}/enrollment/offboarding`,
session permission `revoke`) executes the fail-closed recorded stages exactly
as designed: inventory-read resolves the single approved record,
registry-revoke relays the bound revocation, and only then is the
Simulator-owned workspace bundle (key bundle plus canonical onboarding
record) removed; operator-owned identity directories are reported for
explicit operator removal, and every partial failure persists an attempt
record and stops. **Deactivation wiring (2026-11-02, same session):** the
approved deactivation-time contract is implemented in source as
`POST /api/simulator/organizations/{organization}/deactivate` (session
permission `revoke`, ordinary-client mode only — legacy-launcher is refused so
reviewed retained identities are never offboarded): with no running scenario
and no authenticated machine sessions, every profile-declared endpoint with an
approved enrollment record is offboarded through the maintained single-call
workflow (retained inventory is never in the set), assets without an approved
record are reported already-clean, and the active-organization selection is
cleared only after every declared endpoint is offboarded. Live status is in
the acceptance record below: load-lab carries the `revoke` grants and passed
the acceptance; the qualified package archives initially predated these
commits (closed by the 2026-11-02 `velocity-paired-20261102-r2` rebuild
recorded below).

**Revocation and offboarding accepted live on load-lab (2026-11-02,
`8355ecb`):** the upgraded load-lab stack ran the full acceptance from
binaries built on the workbench toolchain at `8355ecb`: Simulator
`zpr-simulator-r20` with launcher `zpr-lab-bootstrap-r23` (the launcher's
Simulator OIDC override allow-list had to learn the distinct `revoke`
permission — it refused the override fail-closed until `8355ecb`), and the
shared `bin/zpr-enrollment-service` replaced with a dated backup and an
updated `SHA256SUMS` pin (the enrollment unit refused the unpinned binary
fail-closed first, as designed; Velocity/Northstar enrollment units keep
their old running processes and pick up the compatible binary on their next
restart). Operator configuration gained `revoke` with dated backups in the
operator-owned enrollment trust and the Simulator OIDC override, and the
pairing-derived `simulator-paired-enrollment-trust.json` was re-derived
through the maintained command against the stopped install (the superseded
derived file kept as a dated backup); changing the OIDC override identity
digest froze out the old private Simulator state, which was rotated to
`simulator-state-load-lab-r2` preserving `workspace/`. **Acceptance evidence,
all through the authorized named-operator chain:** the on-the-fly workload
`load-lab-machine-03-finance-client` BAS-authorized with its exact runtime
key (credential-check) before the operation; the single-call offboarding
returned 200 with the registry record revoked at revision 4 bound to both
recorded fingerprints and the workspace bundle removed; the direct BAS
preauthorize then refused the revoked client (HTTP 403) while an approved
control asset still received a nonce (HTTP 200); credential-check failed
closed 404 with the bundle gone, the canonical onboarding record removed and
the offboarding operation record persisted; re-onboarding the same asset
succeeded (new invitation approved — the revoked record leaves the
active-asset uniqueness set and is preserved for audit), the operator
identity directory was explicitly replaced from the newly delivered bundle
(dated backup), and credential-check re-authorized with the new runtime key.
The estate was then requalified: `load-lab-fanout` completed all 19 steps
from the upgraded stack with verified container cleanup. Not separately
exercised: the visas-lost-at-next-sweep clause (the asset had no active
flows; no scenario was running during revocation) and the live `deactivate`
endpoint (would offboard all five identities; deferred to an explicit
deactivation occasion). The 20261102 Velocity/Northstar packages initially
predated these commits (closed by the `velocity-paired-20261102-r2` rebuild
recorded below).

**Maintained vs-adapter wedge detection/recovery built and used live
(2026-11-02, `2f4472b`):** `zpr-lab-bootstrap -evaluation-adapter-health`
replaces the manual stale-noise remedy. Detection is evidence-first from the
install's node-container log window with a test-pinned contract: only a
sustained, current loop of the recorded signature (flow_mgmt "no egress
link" visa-matching errors, noise `HandshakeError` retries) on a
long-running container is the wedge — startup transients and stale lines are
explicitly classified as not-the-wedge. Detection alone exits 3 with bounded
evidence and the proven remedy; `-recover` with explicit install and
BAS-adapter unit names performs exactly that remedy (install restart, settle,
signature-free current readback, BAS adapter restart to active), refuses
blind restarts of a healthy install, and fails closed with defect evidence
when the wedge persists. **First live use on its first run:** the command
detected a real recurrence on Northstar (190+ egress-loss lines and 8
HandshakeErrors sustained 04:58–05:03Z while every unit showed active) and
`-recover` healed it end to end; post-recovery all Northstar units are
active, the Room answers 200 and the detector reports no current signature.
Velocity and load-lab checked healthy. This closes the interim-remedy
backlog item only: the recurrence itself remains an open data-plane defect
(DP-K03 class) owned by the data plane.

**Maintained per-organization provisioning command (2026-11-02, `503f731`,
verified live):** `zpr-lab-bootstrap -evaluation-provision-organization`
wraps the proven load-lab sequence behind one resumable fail-closed entry
point driven by a small operator-owned manifest: the base operator tree, the
organization, its single pinned data-plane image and the declared on-the-fly
assets. BAS is part of the minimum ZPR stack, so nothing BAS-specific is
declared — the adapter runs the pinned image and its listeners derive from
the install's own recorded browser IP and port block, exactly as every path
derives from the base-tree convention (explicit overrides exist for
deviating estates). Stage model: pairing, the pairing-derived enrollment
administration trust and BAS execute in-process through their existing
resume-or-verify semantics; onboarded endpoint identity bundles, control-TLS
material and the machine-runtime contract (pinned controller digest,
declared-assets-only references) are verified as file evidence; the
enrollment readiness preflight and the applied class-based workload policy
are never inferred and require explicit operator attestation pointers in the
manifest. Each run writes a stage record beside the manifest; incomplete
runs exit 3 naming the exact bounded next action, and retained inventory is
never provisioned. **Live verification:** against the stopped, already
provisioned load-lab install the command completed all eight stages
(maintained stages resumed and verified, five identity bundles, control-TLS
and the controller digest verified, both attestations recorded) and exited
0 with a complete record; the restarted stack came back healthy (Room 200,
BAS nonce issuance 200, Simulator up) and the adapter-health detector
correctly classified the restart's three egress-loss lines as startup
transients. Remaining scope, not regression: the running-service stages
(preflight save, the onboarding loop, control-TLS generation, bindings
emission, class-based policy generation/apply) are verified or attested, not
yet executed, by this command; converting them into executed stages is the
follow-on increment. A genuinely new organization has not yet been
provisioned end to end with it.

**Multi-node retained topology binding (2026-11-02, G6 increment, source
only):** the retained topology binding now has an all-or-nothing multi-node
form: `nodes` declares the docking data-plane container first and then the
install's own `-node-<name>` containers, each with its logical node ID and
addresses; the reviewed single-node scalar form is byte-for-byte unchanged,
so Velocity/Northstar/load-lab bindings keep their exact contracts. The
Simulator validates the multi-node form against the organization topology
(ordered logical-node match, unique containers and addresses, docking
container first, additional containers restricted to the install's node
naming) and verifies every node container live (ownership label, image,
network address, running state and tun0 ZPR address per node); retained
machine containers and the DNS readback dock at the first (docking) node.
The launcher's strict bindings schema accepts the multi-node form only when
it binds exactly this install's own data-plane topology at its deterministic
containers and addresses, and now refuses a single-node binding against a
multi-node install. Test-pinned on both sides with the full suites, vet and
both builds passing. Live status is in the great-lakes record below; still
open for G6: per-machine docking-node selection in the retained machine
runtime (machines currently dock at the docking node; endpoints already
choose nodes via `node_host`) and closing the acceptance matrix — its first
full measured baseline (9/17 completed) is recorded below (redwood bring-up
itself is done — see its record below).

**Design hardening backlog (recorded 2026-11-01):** recorded for scheduling:
the multi-node retained topology binding (G6) and the maintained
per-organization provisioning command are now implemented — see their dated
records above (the provisioning command provisions pairing, BAS and declared
endpoints, not retained inventory, per the approved on-the-fly decision
below). Still unstarted: a maintained data-plane image upgrade path for
frozen installs, race-free delegation-trust derivation (today the install
must be stopped to avoid racing the state walk), and fail-fast capability
preflight that verifies installed-image contract assumptions with explicit
evidence before scenario execution.

**Great-lakes provisioned as the first new organization through the
maintained command, multi-node live (2026-11-02, through `bae94eb`):**
great-lakes came up as the fourth paired organization — and the first
provisioned end to end by `-evaluation-provision-organization` (exit 0, all
eight stages: pairing, derived enrollment trust and BAS executed; preflight
and the class-based policy operator-attested; identity bundles, control-TLS
and the multi-node machine-runtime contract file-verified) — on the approved
on-the-fly model with the G6 multi-node retained topology binding verified
live. Maintained-path sequence with dated backups throughout: the
`organization-runtime.sh` image pin added great-lakes to the current
`c820f183…` data-plane image; the reviewed self-approval derivations
(enrollment and control trusts, operator-oidc, enrollment catalog gaining
simulator-control/service-adapter); pairing at 21788; the paired enrollment
trust; BAS at 21996/21997 with its auth root and class-based endpoint policy
(controller/echo-service/simulator-control classes, zero asset pins) applied
through the self-approved pipeline (record `2227f3fe…` revision 2, review
`62G3XZHODMXY47EG25RF66BQP2`, apply `BE4A6FY26KWS5UJJGOP364DZUQ`, vinst 2);
BAS adapter docked ACTIVE; preflight saved through the Room; three on-the-fly
assets onboarded via the single-call endpoint (3×201) with assembled identity
directories, each `bas-authorized`; control-TLS via the generic prepare
script; and the first multi-node retained bindings (docking `milwaukee-hq`
plus `shenzhen-office`/`tijuana-plant` at the install's deterministic
addresses, machine-03 inventory). Four fail-closed contracts were found and
fixed live, each pushed with tests: the provisioning plan resolves the
operator services file for pairing (`896d552`), pairing derives paired
service bindings post-hoc for already-paired installs without key rotation
(`b5c4d77`), multi-node catalog profiles publish their docking node
(`55504bc`), the runtime plan matches the scenario topology to every binding
node (`5924c68`), and `verify_multinode_runtime` verifies retained bound
nodes and their mesh instead of legacy demo containers (`bae94eb`). **Live
result:** `great-lakes-runtime-verification` completed through the full
authorized chain (budget 201, preflight zero blockers, approve, run 202) with
"retained multi-node runtime verified: 3 bound nodes running with their exact
addresses and a fully active mesh" — the first multi-node scenario on the
paired on-the-fly model. All four organizations are healthy at session end.
Open: great-lakes' business scenarios need their 13-machine identity set and
workload policy (not tonight's scope) and the acceptance matrix; redwood
followed the same night (record below).

**Redwood provisioned as the fifth paired organization — the maintained
command is now repeatable (2026-11-02, through `ed5dbe3`):** redwood came up
on the same approved on-the-fly model with a two-node retained topology
(docking `north-hub` plus `regional-yard` at the install's deterministic
addresses) and, unlike great-lakes, required **zero new provisioning-command
fixes**: all eight stages of `-evaluation-provision-organization` completed
on the completion run (exit 0) using the contracts proven the first time.
Same maintained-path sequence with dated backups: image pin to the current
`c820f183…` data-plane image; the self-approval derivations; the enrollment
catalog gaining the simulator-control/echo-service profiles and
service-adapter/workload endpoint types redwood's declared assets need;
pairing and derived trusts at the 23xxx block; BAS at 23996/23997 with the
class-based policy applied through the self-approved pipeline (record
`a1a75a16…` revision 2, apply `2VO7WMEJ2H5C2PSGHSJ4BYGFQD`, vinst 2); BAS
adapter docked; preflight saved through the Room; three on-the-fly assets
(redwood-simulator-control, machine-01 controller and echo-service) onboarded
via the single-call endpoint (3×201), assembled, each `bas-authorized`;
control-TLS and the two-node bindings; both node containers verified with an
active substrate mesh (`10.205.202.2↔.3:5000 Active`). One generic gap was
found live and fixed in the repo: `prepare-retained-machine-control.sh`
hardcoded machine-03/machine-05 while redwood's inventory uses machine-01 —
the script now takes machine identities as arguments and issues missing
machine certificates additively from the existing control CA without
touching existing identities (`ed5dbe3`); after the additive machine-01
issuance the scenario preflight's machine-runtime check passed. Redwood's
own scenarios initially declared the legacy demo topology and had no
approved budget evidence, so no scenario ran that morning — both were ported
and completed the same day (see the scenario-enablement record below).
All five organizations (velocity, northstar, load-lab, great-lakes, redwood)
are healthy at session end.

**Estate consolidated on a single package rebuild (2026-11-02,
`velocity-paired-20261102-r2` from `01b6d1e`):** the mixed-version live
stack (operator-tree binaries from the r8–r13 era, hand-deployed
r18/r20/r21/r22/r23 simulator and launcher builds, and an installed package
from a pre-revocation revision) was collapsed onto one engineering package
pair built by the maintained `packaging/evaluation/package.sh` at `01b6d1e`
(`source_dirty=false`), carrying the estate's pinned `c820f183…` data-plane
image. Source qualification before the build: the authoritative workstation
suite (build, vet, full tests) and the offline packaging-script test both
passed at `01b6d1e`. Deployment kept every existing operator contract: the
archives were checksum-verified and extracted next to the prior installs;
the operator tree's binaries — including the pinned paired-install role
names `zpr-lab-bootstrap.simulator-r11` and `zpr-control-room.scenario-r12`,
which organization-runtime.sh still requires by name — were repointed to the
package build with dated `.before-20261102-package` backups and refreshed
`SHA256SUMS` entries (strict check passing); velocity's simulator now runs
from the new package's own `simulator.sh`, and the other four simulators run
the package launcher and simulator binary with their existing states,
catalogs and arguments. Catalog staging inputs were verified byte-identical
since the prior package revision, so no frozen simulator state needed
rotation and none objected on restart. **Result:** all 25 units (five per
organization) active, Room 200/Simulator 302 on every organization, redwood's
multi-node mesh active, and no error or freeze lines in any simulator
journal. `zplc`/`zpr-policy-service` come from a different source tree and
were not part of this rebuild. The package remains engineering-grade
(`status=engineering-unqualified`); clean-host qualification is unchanged
separate work.

**Approved decision — bundled scenarios adopt the retained topology binding
instead of declaring addresses (user-approved 2026-11-02):** scenario
catalogs must not pin fabric addresses; ZPR addresses are VS-granted and
dynamic. Scenarios that declared legacy fixed node addresses are ported to
declare no topology and adopt the organization's retained topology binding at
plan time, exactly like the other bundled scenarios (the alternative — a
logical IDs-only declared topology — was considered and not chosen).

**Redwood scenarios enabled and completed; Visa-Service BAS wiring made a
fail-closed provisioning stage (2026-11-02, through `07c0948`, packages
rebuilt at `velocity-paired-20261102-r3` from `5a3ec3e` and redeployed
estate-wide):** both redwood scenarios were ported to the adopted retained
topology (`d93d24d`) and completed live through the full authorized chain
(budget 201, preflight zero blockers, approve, run 202):
`redwood-regional-operations` verified the two bound nodes and their active
mesh, and `redwood-dispatch-login` completed all 7 steps — machine-01
started, its controller docked, dispatcher marisol.vega logged in and out
with clean teardown — the **first business machine scenario on a newly
provisioned on-the-fly organization**. Three gaps were found live, each
closed on the maintained path: (1) paired operator baselines carry no
workspace scenario-authoring grant, so seeded scenarios could never be
updated through the API — the launcher's explicit OIDC-override allow-list
now accepts the Simulator-private `scenario.edit`/`scenario.publish`
additions (`5a3ec3e`; redwood's override gained them with a dated backup and
its frozen state rotated to `simulator-state-redwood-r2` preserving
`workspace/`; velocity's state rotated to `simulator-state-r5` when the r3
package's changed catalog froze `simulator-state-r4`). (2) The first machine
run failed with explicit VS evidence — "missing HTTPS and token-key
configuration for authentication service redwood-enrollment-bas": endpoint
admission requires the operator Visa configuration to wire the
organization's BAS as an HTTPS trusted service
(`[trusted_service_http.<org>-enrollment-bas]` plus the BAS runtime files
mounted read-only), wiring load-lab had by hand and the provisioning path
never covered; redwood and (latently) great-lakes were wired with dated
backups, and because paired installs consume the pairing-derived
`simulator-paired-operator-services.json`, both were re-derived through the
maintained pairing command against stopped installs (no key rotation;
great-lakes first needed container-written root-owned
`data-plane/dns/adapters.json` ownership restored). (3) The provisioning
command now verifies that exact wiring as a ninth fail-closed stage,
`visa-bas-trust` (`07c0948`), test-pinned; both organizations re-ran
provisioning to exit 0 with all nine stages. All five organizations healthy
at session end. Redwood's acceptance-matrix cell is now 2/2 scenarios; still
open: great-lakes business scenarios (13-machine identity set, workload
policy), the full five-organization matrix, and per-machine docking-node
selection.

**Measured seventeen-scenario acceptance matrix baseline (2026-11-02, on the
`velocity-paired-20261102-r3` estate):** every one of the seventeen original
organization scenarios was driven (or explicitly preflighted) through the
full authorized chain for the first time since the paired migration. **9/17
completed:** velocity `velocity-single-node` (20/20), northstar
`client-service` (23/23), all four load-lab fanouts (`load-lab-fanout`,
`-50`, `-100`, `-150`, each 19/19), great-lakes
`great-lakes-runtime-verification` (re-measured on the current package), and
both redwood scenarios (recorded above). **1/17 fails as an unported
legacy-runtime scenario:** northstar `default-deny` reproducibly fails at
its single machine-less `traffic` step, which execs into the legacy
`zpr-local-linux-node` demo container that does not exist on the paired
retained estate — porting machine-less traffic steps to the retained
runtime is new enablement work. **7/17 are blocked on missing retained
machine identities (explicit preflight evidence, "retained machine
lifecycle does not support <machine>"):** northstar `machine-policy-deny`
(machine-01), `internet-gateway-egress` (machine-07), `active-team-cycle`
(machine-04), and the four great-lakes business scenarios (machines 01–13
plus the echo/metrics/internet-gateway workloads; `finance-web-gateway`
additionally needs real internet egress). Operational findings while
measuring, each resolved on maintained paths: velocity and load-lab were
both found carrying live vs-adapter wedges and were healed by
`-evaluation-adapter-health -recover` (its second and third real uses);
velocity's and northstar's single-node installs had drifted off their
pinned binding substrate addresses because single-node installs use dynamic
Docker subnets (bindings updated with dated backups; their frozen states —
which digest the binding file — rotated to `simulator-state-r6` and
`simulator-state-northstar-dns4` preserving `workspace/`), recording a
durable-fix backlog item (closed the same day — see the identity-only
binding record below): single-node retained bindings pin an inherently
dynamic substrate address and should verify container identity while
resolving the address live, or single-node installs should gain the
deterministic subnet multi-node installs already have; and the first one or
two scenario runs immediately after an install restart can fail on the
control-callback rendezvous before the stack settles (both such failures
completed on retry; recurrence under a settled stack remains defect
evidence). All five organizations healthy at session end.

**Single-node retained bindings pin identity only; dock address resolved
live (user-approved direction, 2026-11-02, `e0f848d`; estate on
`velocity-paired-20261102-r4`):** acting on the approved addressing design
(addresses are dynamic and never pinned; trust comes from pinned
identities), the single-node retained topology binding no longer declares a
substrate address — the launcher and the Simulator both refuse one
fail-closed — and verification pins identity (container name, ownership
label, image digest, running state, attachment to the install's own network
and the tun0 ZPR address) while the machine and control-callback dock
address is resolved live from the verified docking container at start. The
multi-node form keeps the install's deterministic addresses because
compiled policy embeds them. Scenario topology nodes may omit the substrate
(set values are still validated); the retained runtime digest revision was
bumped (`resolved-dock-address-v3`) so containers started under the
superseded fixed-address contract are refused. Test-pinned on both sides
with full suites green. Rolled out estate-wide: packages rebuilt at
`e0f848d` as `velocity-paired-20261102-r4` and redeployed through the
maintained swap, the three single-node bindings (velocity, northstar,
load-lab) migrated to identity-only with dated backups, and their frozen
states — which digest the binding file — rotated to `simulator-state-r7`,
`simulator-state-northstar-dns5` and `simulator-state-load-lab-r3`
preserving `workspace/`. **Proof on the new contract, all first-try:**
`velocity-single-node` 20/20 (live-resolved dock address end to end),
`load-lab-fanout` 19/19, and `redwood-regional-operations` (multi-node
unchanged). Network recreation can no longer strand single-node bindings;
this closes the substrate-drift class found during the acceptance-matrix
baseline. All five organizations healthy at session end.

**Approved decision — on-the-fly provisioning replaces retained inventory for
new organizations (user-approved 2026-11-01):** new organizations are not
provisioned with a pre-enrolled retained machine inventory. Instead the
scenario's endpoints are declared in the organization catalog, onboarded at
explicit activation through the existing proven declared-endpoint chain
(invitation, device claim, named self-approval, bundle handoff), used by the
run, and offboarded at teardown; policy matches endpoint classes by
BAS-sourced `profile`/`type` claims instead of per-asset identity pins, and
control-TLS is issued per activation. The accepted trade is slower scenario
starts for the production-shaped provisioning contract with near-zero per-org
pre-provisioning. Velocity and Northstar are grandfathered on their reviewed
retained contracts until explicitly migrated; their retained records above
remain the authority for those installs. Load-lab is finished the new way,
not by hand-building retained inventory. Open design work before
implementation: the offboard-at-teardown contract (revocation/cancellation so
the enrollment registry stays bounded, fail-closed on partial teardown), the
class-based workload policy shape replacing per-asset defines, and per-run or
per-activation control-TLS issuance. The dynamic machine-control rendezvous
(granted addresses, production DNS names, mTLS-only trust) is a prerequisite
and is already live.

### Delivery discipline and completion

- One coordinator owns live changes. Parallel chats may take bounded source-only
  work on independent contracts/tests; they do not change shared runtime state.
- At each 60-90 minute engineering checkpoint, report the gate, passing evidence,
  exact blocker and next bounded action. If the gate makes no measurable
  progress, stop expanding configuration and reproduce the failing contract.
- Diagnose with a focused test, fix the owning layer, then rerun the failed gate.
  A manual repair is diagnosis, not accepted delivery; replace it with a tested,
  maintained path before recording success.
- Do not deploy other organizations or main dev from sequential diagnostic
  success. Do not change policy/trust scope or reset retained identities without
  separate review.
- **Done means:** a new permitted endpoint can be enrolled from the UI without
  manual credentials or additional prompts after selection, named self-approval
  is recorded and Audit role-risk warnings are verified, retained identities
  survive restart, all original
  scenarios pass from the installed dev package, cleanup/recovery are proven,
  and production remains independent. Source tests alone do not satisfy this.

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

**2026-10-24 source ownership update:** policy declarations, not operator
directory registration, authorize the VS trusted-source set. A durable default-on
operator toggle now gates declared sources through provisional production
`GET`/`PUT /admin/trusted-services/{id}/control` JSON. Off survives policy
removal/re-addition and state-preserving restart; lookups fail closed and visa
reconciliation remains asynchronous. This is a VS runtime/API increment, not
the completed Control Room workflow, LDAP provider delivery, multi-document
declaration merger or M1 acceptance. Earlier registration/wizard tasks below
describe existing evaluation scaffolding and must not be treated as a competing
source authorization authority. See the
[ownership, API and lifecycle evidence](MULTISOURCE_POLICY_SUPPORT.md#policy-declared-sources-and-durable-default-on-controls-2026-10-24).

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
- **B1-RECOVERY (evaluation-only implementation):** the stopped-host CLI reset
  rotates the Dex static password, client secret and subject, preserves provider
  data, audits without credentials and delivers a fresh 256-bit password once to
  a new protected file or terminal. Focused tests cover local ownership, lifecycle
  locking, safe delivery and fail-closed interruption. This is one-time disclosure,
  not enforced single-use: Dex does not enforce replacement, expiry or replay, old
  signed JWTs remain valid until expiry, and OpenObserve's independent admin is not
  reset. Live IdP/browser recovery and production qualification remain open.
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

#### B05 targeted test baseline record (2026-10-25)

Recorded on `maturing/integration` at `0b8a8c3` (macOS arm64; Go 1.27.1, Rust
and Cargo 1.98.1, Node 26.10.0). The sibling `zpr-common` checkout is `2756e9a`
(`main`). This is local source/test evidence, not a deployment version. The
local evaluation stack was bootstrapped from this worktree at `08391e8`, before
the internal signing gate; that gate is not called by any running service.

| Suite | Result in a clean worktree |
| --- | --- |
| Operator-stack checks (`make test`, Python) | 11 passed |
| Go (`make test` → `go test ./...`) | exit 2; exactly 13 failures, all in `cmd/zpr-web-dashboard` |
| Go race + vet, `internal/changereview` and `internal/subjectsnapshot` | pass |
| Rust (`cargo test --locked --offline`) | 1,008 passed, 23 ignored (needs the `zpr-common` sibling below) |

Root causes of the 13 failures (diagnosis only; no tests were changed):

| Cause | Failing tests | Kind |
| --- | --- | --- |
| Gitignored generated `policy-demo.zplc` files (`*.zplc` in `zpr-dashboard/.gitignore`) exist only in the main checkout: `packaging/.../northstar` and the `great-lakes`/`load-lab` examples. | Northstar LDIF/runtime-asset evidence, organization portable/duplicate/bundle/profile/draft-archive (8) | Local generated input |
| Tests resolve `../../../../.local-runtime` (Northstar pregen LDIF, simulation manifest) and the Cargo workspace resolves `../zpr-common`; in a worktree these sit one directory too deep. | Bundled seed locations, Simulator design assistant, Simulator organizations catalog, Rust policy-apply crash-window interop (4) | Worktree layout |
| Since `4627e4c`, `start_machine_container` calls `machine_docker_network_for_org` (`scripts/machine-runtime.sh`), which the isolated launcher harness neither sources nor stubs. | `TestBusinessMachineStateBindIsPersistentAndGenerationScoped` (1) | **Real test drift** |

Verification: temporarily providing the three gitignored `.zplc` files (copied
from the main checkout) and sibling `zpr-common`/`.local-runtime` links made 12
of the 13 pass. Full `make test` then failed only the launcher harness test. All
temporary inputs were removed afterward. Browser (Playwright) suites were not
run for this record.

**Update (2026-10-25, operator-approved):** the launcher harness now stubs
`machine_docker_network_for_org` and asserts that the multi-node container is
attached to `zpr-<organization>`. The clean-worktree baseline is therefore
**12** failures, all missing local inputs. The operator **confirmed the
10-second observation budget** in section 6. B05 stays open only until
deployment versions are recorded for a qualified integrated stack.

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
  Source-policy completion now persists explicit per-org policy defaults and
  proves default binding with the source-built compiler; dependency-aware visa
  outage handling remains open, so S06 is not marked complete.
- [ ] **S07:** Test source/organization isolation, duplicate IDs, invalid
  mappings, revision conflicts, missing secrets, wrong credentials/certificates,
  source disable/removal and restart persistence. Block or explicitly resolve
  references before removing a source used by a policy/assertion.

**Exit:** two sources can be registered and read concurrently, including the
approved LDAP/provider path, with Simulator never started. LDAP receives no
write requests. Visual tests V02-V07 and V13 pass.

#### M1 acceptance reconciliation (2026-10-24)

M1 remains **open**. The unchecked items above are whole acceptance gates,
not a claim that their implementation has not started. Use this reconciliation
to choose the next increment; additional database/visa race hardening is not a
substitute for the missing M1 functionality or integrated acceptance.

| Gate | Current evidence | Remaining before closure |
| --- | --- | --- |
| S01: durable registration | Protected single-writer registry, stable source/org identity, revision conflicts, protected secret references, edits and restart persistence have backend tests; registration/default controls have desktop/tablet fixture tests. | Reconcile enabled-state and optional-capability requirements with the actual registration contract; verify the approved configuration in the integrated stack. Single-writer provisioning is not HA qualification. |
| S02: independent LDAP | Verified LDAPS bind/search fixtures exercise two independently configured network providers, approved attributes, scoped DNs and credential/certificate failures with invalid Simulator inputs. The client fixture rejects LDAP write operations. | Approved read-only directory account/ACL and live LDAP/provider acceptance; local protocol fixtures are not evidence of deployed directory access or least-privilege ACLs. |
| S03: two concurrent sources | Backend registration/read tests and desktop/tablet catalogs retain separate stable IDs and different values for the same username. | Register/read both approved sources through the integrated deployment without Simulator. |
| S04: tests and health/capabilities | Bounded read-only connection tests, selected-source failure/recovery, last-success UI evidence and explicit unsupported Updates have backend/browser fixture coverage. Saved configuration does not certify connectivity. | Reconcile registration, runtime inventory and feed identities/capabilities; qualify unavailable/stale/last-success behavior and the observation budget in the integrated stack. Browser-session connection evidence is not durable health certification. |
| S05: provenance and identity isolation | Source-qualified catalogs/compiler/runtime keys avoid overwrites; scoped assertion expressions and UI selection preserve source identity. | **Functional blocker:** VS lookup still fans out matching live identity strings. Approved issuer-qualified subject links and their revocation enforcement are not implemented. Separate keys/catalogs do not establish separate principals. |
| S06: defaults and dependency-aware outages | Explicit per-org policy defaults persist and bind signed staging. Required-provider failure is fail-closed; an independent selected-source read remains usable. | **Functional blocker:** VS whole-actor closure is conservative, not dependency-aware. Complete negative/deny/admission dependencies and cached-visa enforcement must precede selective availability. Do not skip failed providers or substitute defaults. |
| S07: lifecycle and reference safety | Scoped/revisioned removal, default/watched/shared-source restrictions, duplicate IDs, invalid configuration, restart and fault coverage exist. Staged policy bindings detect source/default changes. | Verify policy/assertion reference handling before removal and enabled-state lifecycle; installed-policy/visa version invalidation remains open. Existing source retirement restrictions are not proof of complete reference protection. |

Fresh validation for this reconciliation:

- Focused source registry, network LDAP, policy-default/version-binding and
  source-ID tests pass with `go test -race`. These tests use local protocol
  fixtures, not a running Simulator or an external company directory.
- **28 browser fixture cases pass** across desktop and tablet: 12 registration/
  multi-source catalog cases and 16 Trusted Sources GUI cases. They cover
  delayed responses, source/org switching, unsupported Updates, independent
  browsing during outage, recovery, explicit defaults and registration errors.
  Routes/data are mocked; these are not integrated V02-V07/V13 sign-off.
- Added `TestSourceRegistryRestartOutageAndRecoveryPreserveSourceIdentity`
  in [the registry tests](zpr-dashboard/cmd/zpr-web-dashboard/assertion_source_registry_test.go).
  It registers both LDAPS sources, persists the policy default, recreates the
  runtime twice during a provider bind failure, proves no cached-success or
  peer fallback, recovers, restarts again and verifies unchanged registry bytes,
  revision and identities. The existing fixture permits only bind/search/unbind.
  This is combined backend restart/recovery evidence, **not** a live LDAP
  process restart or browser/service deployment test.

Reproduction from `zpr-dashboard`:

```sh
go test -race ./cmd/zpr-web-dashboard \
  -run 'Test(SourceRegistry|NetworkLDAP|PolicySourceDefault|PolicySourceBindingsRequire|PolicySourceID)' -count=1
npm run test:browser -- tests/browser/source-registration.spec.mjs tests/browser/multisource-policy-contract.spec.mjs
npm run test:browser -- tests/browser/dashboard.spec.mjs --grep 'Trusted Sources GUI'
```

**Remaining work order:** resolve the S05 approved subject-link/authenticated
identity contract and S06 complete dependency contract documented in
[multi-source support](MULTISOURCE_POLICY_SUPPORT.md#explicit-subject-links-required-follow-up)
before implementation; reconcile the S01/S04/S07 contract gaps above; then run
V02-V07/V13 against an isolated, approved two-source stack with Simulator absent.
Do not introduce the deferred principal-envelope/shared-schema design merely
to close a checkbox, and do not claim acceptance from Rust test totals.
M0's still-open production boundary prerequisites also remain prerequisites.

The next S05 design packet is now drafted for review in
[the minimal subject-link proposal](MULTISOURCE_POLICY_SUPPORT.md#s05-minimal-subject-link-contract-review-proposal-2026-10-24).
It specifies direct approved root-to-provider bindings, no identity-string
fan-out, actor/visa persistence and revocation/restart acceptance. It explicitly
leaves authority/immutable-ID ownership, source-version delivery, approval
storage/API, enforcement timing and compatibility decisions unapproved. No
subject-link runtime behavior or shared schema was changed by the proposal.

A separately approved
[verified-device foundation](MULTISOURCE_POLICY_SUPPORT.md#s05-foundation-increment-preserved-device-authentication-2026-10-24)
now preserves authenticated adapter/device identity outside policy claims and
through actor storage/refresh. It does not authorize links or change provider
lookups. Organization/trust-version ownership, directory selectors, binding
approval/storage and generation/revocation enforcement remain open; S05 remains
unchecked.

A subsequent approved
[session-guarded write packet](MULTISOURCE_POLICY_SUPPORT.md#s05-foundation-increment-session-guarded-actor-writes-2026-10-24)
atomically rejects stale actor updates after authentication-session replacement
and preserves explicit reconnect behavior. It is not complete actor-generation
or subject-link/visa enforcement; the remaining S05 gates above still apply.

The operator has now selected **Control-Service ownership of approved links**,
VS enforcement through authenticated versioned snapshots, and **immutable
provider subject IDs only**, with no UID/DN/mail fallback. The
[production binding contract review](MULTISOURCE_POLICY_SUPPORT.md#s05-production-binding-contract-ownership-and-api-review-2026-10-24)
specifies proposed records/API shapes, approval boundaries, delivery/restart
rules and an acceptance matrix. This is a documentation-only contract packet:
routes, storage, transport/schema and link enforcement are not implemented.
Operator authorization, authority registration/version mapping, authenticated
delivery format, atomic publication and revocation timing still need approval.

The next approved
[operator authorization foundation](MULTISOURCE_POLICY_SUPPORT.md#s05-operator-authorization-foundation-2026-10-24)
adds dedicated subject-link permissions and exact Control Room route mapping
using existing organization-scoped OIDC authorization, plus the independent
issuer/subject reviewer-check primitive. Approval storage/handlers and the
durable two-person commit check are still absent; enrollment delegation is not
broadened. This is not S05 sign-off or a running subject-link workflow.

The approved
[durable subject-link review foundation](MULTISOURCE_POLICY_SUPPORT.md#s05-durable-subject-link-review-foundation-2026-10-24)
now persists isolated immutable proposals and transactional independent review
decisions in the existing SQLite store. Its `review-approved` state is not an
active binding and is excluded from the policy/gateway queue. HTTP authorization
integration, atomic source/authority validation, publication, delivery and
revocation remain unimplemented; this does not close S05.

The [durable review retry packet](MULTISOURCE_POLICY_SUPPORT.md#s05-durable-review-retry-receipts-2026-10-24)
adds principal/organization-scoped retry receipts committed with the review and
audit event. Identical keyed retries replay the original response; changed
content conflicts. This does not publish active links or expose HTTP handlers,
and does not close S05.

The documentation-only
[Control-Service review integration proposal](MULTISOURCE_POLICY_SUPPORT.md#s05-control-service-review-integration-contract-2026-10-24)
locates future subject-link review APIs/state in Control-Service, leaves the
generic policy/gateway queue in Control Room and separates review-only approval
from active publication. It specifies end-to-end operator-proof requirements,
retry/capture ordering and acceptance with Simulator absent. Concrete proof
scope, authoritative authority capture, configuration and HTTP limits still
require approval/implementation; no endpoints or active links are added.

The approved
[isolated review-proof groundwork](MULTISOURCE_POLICY_SUPPORT.md#s05-isolated-review-request-proof-foundation-2026-10-24)
adds internal signing/verification for the three review-only routes, with a
distinct purpose, exact organization/permission/request/retry-key binding,
independent backend grants, pinned mTLS caller and single-use nonces. Targeted
race/compatibility tests pass; enrollment scope is unchanged. No HTTP handler,
trust provisioning, authority registration or active binding is enabled.

The approved
[durable proof/review composition batch](MULTISOURCE_POLICY_SUPPORT.md#s05-durable-proof-and-review-composition-groundwork-2026-10-24)
adds a bounded persistent proof-nonce store, exact review-only permission
mappings and signer/verifier/review composition tests across reopen, concurrent
retries, grant denial and audit failure. Proof consumption and semantic retry
receipts remain separate. HTTP handlers, service trust/configuration,
authoritative authority capture and active publication remain unimplemented;
S05 is not closed.

The approved
[internal BAS authority batch](MULTISOURCE_POLICY_SUPPORT.md#s05-internal-bas-authority-registration-and-capture-2026-10-24)
adds organization-scoped service/issuer registration, server-owned version-CAS
edits, immutable audit history and exact enabled-version device-root capture.
Tests cover restart, isolation, concurrent edits and rollback. No production
authority configuration/HTTP API, trust provisioning, combined review capture
or active-link enforcement is enabled; S05 remains open.

The [authority-guarded review integration](MULTISOURCE_POLICY_SUPPORT.md#s05-transaction-guarded-authority-review-integration-2026-10-24)
now checks current BAS registration in opt-in review transactions, with distinct
retry fingerprints and legacy/guarded proof-composition tests. Separate source
capture/selector capability, production HTTP composition and active publication
are still missing. This narrows the authority capture race, not the S05 gate.

The approved
[LDAP selector declaration increment](MULTISOURCE_POLICY_SUPPORT.md#s05-explicit-ldap-selector-declaration-2026-10-24)
adds optional network-LDAPS-only `ldap_entry_uuid` configuration, identity-version
invalidation and an explicitly unverified editor declaration. Backend tests and
12 desktop/tablet registration cases pass. Actual UUID lookup, provider support
validation and combined review capture remain unimplemented; S05 stays open.

The [exact LDAP/source-review batch](MULTISOURCE_POLICY_SUPPORT.md#s05-exact-ldap-subject-lookup-and-source-guarded-review-2026-10-24)
now provides internal exact UUID lookup and source/authority-guarded review
composition, with API-managed source edits fenced through commit and historical
retries independent of provider availability. Local LDAPS/race fixtures pass.
External-file atomic publication and configured HTTP service composition remain
open. Dynamic source binding into VS explicitly needs source-to-policy version/
identity mapping, authenticated delivery, credential activation and installed
readback plus disable/removal invalidation; existing policy-driven store rebuilds
do not establish that contract. No dynamic VS installation is enabled here.

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
  endpoint identities. The fixed pre-enrolled demo set is a diagnostic stepping
  stone, not the turn-key completion gate. Automate new-endpoint enrollment and
  retained-identity reuse through existing supported tools/contracts under G2
  above; native platform-helper certification is not a prerequisite for the
  selected Linux evaluation runtime. Do not clone endpoint keys.
- [x] **U05:** Run normal allow/deny workloads against services. Compare one
  generated client and one non-Simulator client under equivalent policy.
  Scenario intent/expected outcomes stay in Simulator; Control Room reports
  only independently observed production events.
- [ ] **U06:** Reconcile setup/apply retries by durable operation identity and
  readback. Scenario reset preserves registrations, policy, directory data and
  enrollment; destructive cleanup is separately scoped, confirmed and audited.

**Exit:** V09-V12 and V14 pass. Simulator can exercise an independently prepared
ZPR installation; stopping it does not remove services or their configuration.

#### M3 data-plane increments (2026-10-25)

The native loopback VS cannot host nodes: nodes reach the VS only at its fixed
ZPR address `fd5a:5052::1` through its own `vs.zpr` adapter. The legacy
organization launcher stays blocked (`U01-ACTIVATION`). The Simulator therefore
drives an evaluation install's own data plane instead.

- [x] **D1:** `-visa-service -data-plane` runs the VS, private Valkey, one node
  and the `vs.zpr` adapter in a privileged Linux container that the evaluation
  install owns. Identities and the policy are unique per install, readiness
  requires both actors to be admitted, and Control Room shows them through
  Control-Service. See the
  [evaluation runbook](zpr-dashboard/packaging/evaluation/README.md).
- [ ] **D2:** ordinary endpoint enrollment (deferred; X15 stands in).
- [x] **D3:** Simulator endpoint adapters join the install's data-plane network
  with keys from X15. `-data-plane-endpoints N` generates per-install endpoint
  keys, ping services and the handoff `data-plane-endpoints.json`. The
  Simulator reconciles endpoint containers from
  `SIMULATOR_DATA_PLANE_ENDPOINTS_FILE`, and install teardown removes them.
  This was verified live: three endpoints were admitted, ping worked between
  endpoints over ZPR, running endpoints were adopted, and teardown cleaned up.
- [x] **D4:** allow/deny workloads observed independently by Control Room
  (U05). The Simulator probes from every endpoint: a ping to the next endpoint
  (allowed) and a ping to the VS (denied by policy, `NoMatch`). It reports
  expected vs observed outcomes at `/api/simulator/data-plane-workloads`.
  Control-Service independently reports the resulting visas, denies and
  counters. This was verified live with zero mismatches.
- [x] **U05 completion:**
  - [x] **U05a:** TCP and UDP service workloads. Endpoints run echo listeners
    on their ZPR address: `tcp/7007` and `udp/7008` are published to peers by
    the install policy, `tcp/7009` and `udp/7010` are not. The handoff lists
    the listeners, and the Simulator probes each one on the next endpoint.
    Verified in a trial install: published ports echo, unpublished ports are
    dropped by ZPR while answering locally. Live on the northstar,
    great-lakes and redwood installs: 18 probes per install with no
    mismatches or errors, including across nodes. The VS admin API shows TCP
    visas for port 7007 and `NoMatch` denials for port 7009.
  - [x] **U05b:** compare with a client the Simulator does not run, under
    equivalent policy. The install generates operator client `client-01`
    (own key, `fd5a:5052:0:1::c1`) with exactly an endpoint's policy in both
    directions. The Simulator handoff excludes it, and the operator runs it
    with `data-plane/client.sh`. Live on northstar: client-01 and endpoint-02
    got identical results probing endpoint-01 (ping, `tcp/7007` and `udp/7008`
    reply; `tcp/7009` and `udp/7010` dropped), and so did the reverse
    direction. The VS and Control-Service report the client's visas and
    `NoMatch` denials like an endpoint's, while the Simulator reports 0
    mismatches and never names the client.
- [x] **D5:** one evaluation install per organization for Simulator
  organization switching. This replaces the legacy reset script.
  - [x] **D5a:** `-node-location LAT,LON` writes the data-plane node's map
    position into the frozen `vs.toml`, so Control Room can place it.
  - [x] **D5b:** `SIMULATOR_EVALUATION_INSTALLS_FILE` registers independently
    started installs per organization. Activation selects and checks the
    install, never manages it; endpoints and workloads run for all installs.
  - [x] **D5c:** live with northstar and great-lakes installs, each with its
    node on the map, three endpoints and clean workloads. Switching between
    them works, and an organization without an install is refused. The single
    maturity install was retired. Per-organization directory people and
    multi-node topology come later.
- [x] **D6:** organizations' own directories and node topology.
  - [x] **D6a:** `-directory-base-dn DN -directory-ldif FILE...` serves an
    organization's own LDIF under its own base DN instead of the starter
    directory. It is operator configuration frozen at creation; entries
    carrying `userPassword` are not imported, and an evaluation-only
    `zprMachine` schema is included. Verified with slapadd in the pinned image
    for northstar, great-lakes and redwood.
  - [x] **D6b:** multiple data-plane nodes per install, with peer links, map
    positions and endpoints spread across nodes. `-data-plane-node
    NAME@LAT,LON` (repeatable, up to 6) adds full-mesh linked node containers
    on a per-install Docker subnet; the policy declares the nodes, links and
    bootstrap keys, and endpoints are assigned round-robin. Verified live with
    three nodes: all admitted, in sync, linked to each other and at their
    configured positions.
  - [x] **D6c:** live northstar, great-lakes and redwood installs with their
    directories and topologies. Northstar has one node; great-lakes docks at
    Milwaukee with shenzhen-office and tijuana-plant; redwood docks at the
    north hub with regional-yard. All three are registered with the
    Simulator. Endpoints on additional nodes dock through the node's network
    alias `node-<name>` (handoff `node_host`), because full container names
    exceed the 63-byte DNS label limit. Verified live: every node is in sync
    at its position, and cross-node workloads match policy with no
    mismatches.

#### Known data-plane defects

Open defects found while running the live installs. They are outside the
Simulator/Control Room boundary work, but they affect long-running
environments, so each must be fixed or explicitly accepted before M5.

- [ ] **DP-K01 (fixed and running live; zpr-core merge pending; found
  2026-10-26, owner: zpr-core adapter/node):** an endpoint adapter whose dock
  link fails keep-alive never recovers.
  - **Seen:** once, on northstar endpoint-02, about 4 minutes after start
    while `make test` loaded the host. Both the node (link 5) and the adapter
    logged "failed to respond to keep-alive messages" at 15:27:00Z. The node
    removed the link and revoked its visas. The adapter then retried every
    ~25 s with "noise: handshake timeout - node-adapter connection failed"
    and never re-tethered. The Simulator reported every allowed probe to or
    from endpoint-02 as `unreachable` (6 mismatches per round).
  - **Second symptom:** after the stuck container was replaced, the new
    adapter linked and TCP/UDP visas worked again. But ICMP between
    re-connected adapters (endpoint-01/03 and client-01, which were also
    recycled) stayed broken for at least 7 minutes. No ICMP visa requests
    reached the VS for those flows, so the node dropped them before asking
    the VS. The node's denied-flow backoff is capped at 60 s, so it is not
    the deny cache.
  - **Workaround:** restart the install with the same command (identity,
    CA and passwords are kept). After that, all 18 Simulator probes matched.
  - **Evidence:** `zpr-dashboard/.evaluation-build/stuck-endpoint-02-adapter.log`
    and `stuck-endpoint-02-node.log` (local, not committed).
  - **Reproduced** on a two-endpoint trial install by pausing endpoint-02 for
    10 s (`docker pause`): TCP recovered (each connection is a new 5-tuple)
    but ICMP to and from endpoint-02 stayed broken.
  - **Root causes (three, all in `zpr-core/adapter/ph`):**
    1. Revoking a visa (VS revocation, expiry, or ejection for a removed
       link) silently removed the node's PFT entries on *surviving* adapter
       links. Those adapters kept sending on the stale stream ID and the node
       dropped it ("Unknown Stream ID"). ICMP echo always reuses one 5-tuple,
       so it never re-bound.
    2. Tether IDs are scoped to the dock link, but the adapter kept its ELT
       across a dock-link restart, so the restarted adapter also sent on
       stale IDs.
    3. **DP-K02:** when a replacement adapter re-authorized an address before
       the stale link timed out, the stale link's deregistration made the VS
       delete the new actor (`SourceNotFound`/`DestNotFound` for good). This
       is the likely cause of the "never recovers" symptom after the
       container replacement above.
  - **Fix:** zpr-core branch `dp-k01/link-recovery`, commit `9e7fc00` (from
    `3c7ef60`; local only, not merged into the default zpr-core checkout):
    1. Revocation paths return the removed forwarding entries, and the node
       sends a per-flow `StreamIdWithdrawal` to each affected adapter, which
       drops the matching ELT entry so its next packet re-binds.
    2. The adapter clears its ELT during dock-link cleanup.
    3. Deregistration skips addresses now held by another live link.
  - **Verification:**
    - `ph` unit tests: 321 pass, including new regression tests for the
      withdrawn entries and ELT removal/clear.
    - Clippy warnings are identical to the `3c7ef60` baseline.
    - On the trial install (image built with `ZPR_CORE_DIR`), these all
      recovered every allow probe in every direction for ping and TCP: three
      10 s and one 30 s endpoint pauses, a 10 s node pause, and replacing all
      endpoint containers in place.
  - **Not reproduced:** the adapter retrying "noise: handshake timeout"
    forever. Node and endpoint pauses both re-tethered cleanly. Reopen this
    if it recurs with the fixed image.
  - **Follow-up (not a correctness issue):** a restarted adapter leaks its
    stale DLT entries; the node binds fresh egress IDs, so they are not
    reused. The node's `Pft::clear` is also still a TODO.
  - **Rolled out live:** the `:local` image was rebuilt with
    `ZPR_CORE_DIR=<zpr-core worktree> sh packaging/evaluation/data-plane/build.sh`
    (the previous image is kept as `:pre-dpk01`). Each install was restarted
    with its same command and the Simulator relaunched the endpoints. Redwood
    and great-lakes each passed a 12/12 endpoint ping/TCP matrix, the
    northstar client matched policy, and the Simulator reported 0 mismatches.
  - **Remaining:** merge the zpr-core branch upstream; then tick this item.
    The Simulator's workload mismatches remain the detector for a recurrence.

- [ ] **DP-K03 (found 2026-10-28, owner: zpr-core adapter; Visa side
  accepted for now):** an endpoint adapter stops getting visas about 4 hours
  after it enrolls, because it never re-authenticates.
  - **Seen:** on the r5 paired demo, Alice's
    `device.zpr.authority=zpr-bootstrap` attribute (source `zpr`, 4 h
    lifetime, `DEFAULT_AUTH_EXPIRATION`) expired at 05:46:53Z. Visa then logged
    `trusted service not found: ID=zpr` and `attribute refresh indeterminate`
    and denied every visa request. Alice, then web0, lost HTTP to OciWeb, so
    the packaged demo verifier failed its first check. Nodes got
    `must reconnect to renew authentication` at the 4 h mark and renewed.
  - **Root causes (two):**
    1. Visa can only refresh a claim through a trusted service with the same
       source ID. Built-in issuers such as `zpr` have none, so
       `refresh_expired_attributes` (`vs/src/actor_attributes.rs`) can never
       extend them; only re-authentication can.
    2. The endpoint adapter (`zpr-core/adapter/ph`) has no credential-expiry
       handling: it never reconnects and re-enrolls before the claim lapses.
  - **Workaround:** restart the adapter process (restarting the container
    also kills docker-exec'd adapters, which must be relaunched by hand). If
    the same address re-registers before the old session clears, the VS
    evicts the new actor (DP-K02-like); wait about 20 s after stopping the
    old adapter.
  - **Decision:** fix in the adapter (reconnect and re-enroll before the
    claim expires). Making built-in claims refreshable in Visa is a security
    design change and is deferred.
  - **Fix (written, not yet rolled out):** zpr-core branch
    `dp-k03/adapter-reauth`, commit `3aa7af8` (stacked on
    `dp-k01/link-recovery`, because an orderly re-dock depends on the DP-K02
    fix; local only). When an adapter dock link becomes Active it arms a
    timer (default 3 h, `adapter.reauth_interval_secs` in the adapter TOML;
    0 disables, minimum 60). On expiry it closes the link with the normal
    terminate/ack path and the existing restart holddown re-docks and
    re-authenticates. The timer is cleared on any state change. `ph` unit
    tests: 324 pass, including config validation and timer arm/clear tests;
    clippy warnings match the baseline.
  - **Live check (2026-10-28, r5 paired demo, no image rebuild):** the branch
    was built in an `ubuntu:24.04` container on the workbench (matches the
    runtime image's glibc 2.39) and copied into the alice and web0 containers
    as `/app/bin/ph-dpk03` (the shipped `/app/bin/ph` is untouched). With
    `reauth_interval_secs = 120` for both adapters, an 18-minute soak ran an
    Alice→OciWeb HTTP probe every 15 s across 9-11 re-docks per adapter:
    57 probes passed and 10 failed. Every failure fell inside a 5-6 s re-dock
    window of Alice (new address each time) or web0 (same fixed address, no
    DP-K02 eviction), and each recovered on its own. The soak also crossed
    Alice's 16:04:40Z attribute-file TTL: Visa refreshed it (new expiry
    17:04:23Z), so that source is not a second expiry problem. Both adapters
    now run `ph-dpk03` with the default 3 h interval.
  - **Cost:** each re-dock is a brief (about 5-6 s) outage for that endpoint
    and gives it a new ZPR address unless one is pinned. A longer
    `reauth_interval_secs` (up to just under the 4 h lifetime) reduces
    churn; make-before-break re-docking is a possible follow-up.
  - **Remaining:** a real 4 h soak with the default interval (first re-dock at
    about 19:10Z); bake the fix into the packaged image (the demo deploy
    script launches adapters from `/app/bin/ph` and relaunches nothing after a
    container restart); confirm interactive (AuthCode) adapters accept
    periodic re-auth or set the interval to 0; merge zpr-core; then tick this
    item.

### M4 - Enforce separation in builds, configuration and lifecycle

Depends on M2 and M3. This closes deployable boundaries, not just code naming.

- [x] **P01:** Give production and Simulator separate entrypoints/configuration,
  startup and shutdown ownership. Finish the minimal package/import extraction
  needed to prevent production importing Simulator code; reuse neutral helpers.
  Production builds use `main_production.go` with `-tags production`; the
  default build uses `main_lab.go`. Production-tagged package tests verify the
  selected Go source/import graph, reject non-production modes, and exercise the
  production composition with Simulator unavailable. Default and production
  package tests, builds, and vet pass. This closes the compile-time/entrypoint
  boundary only; P02-P05 and the M4 exit evidence remain open.
- [ ] **P02:** Produce a production deployment with no Simulator executable,
  routes, static pages, manifests, scenario mounts or lab defaults. Reject
  missing explicit operator configuration rather than discovering demo values.
  The production package candidate now contains only the production binary and
  its two manifests; package tests verify Simulator mode is rejected and
  Control Room refuses to start without explicit TLS, OIDC and organization
  configuration. Production source/import and asset tests cover the compiled
  and served boundaries. The installed-deployment/cold-start acceptance remains
  open; this is not deployment evidence.
- [ ] **P03:** Independently supervise lab LDAP/OIDC. Document which disposable
  resources belong to the lab and which services are shared; keep bootstrap
  names on existing network DNS and integrate protected DNS with the production
  package lifecycle.
  The companion LDAP/Dex Compose project has independent per-install identity
  and persistent state, with generated stop/restart instructions and a
  container-recreation identity test. Ownership is now documented in the
  Simulator packaging guide. Per the user's 2026-10-26 direction, the companion
  no longer generates an optional bootstrap-BIND role; bootstrap names use
  existing network DNS, while required protected BIND is production-owned.
  P03 remains open until protected DNS has production package
  namespace/state/supervisor integration and restart evidence. The envelope
  model is deferred and is not an implemented namespace boundary.
- [ ] **P04:** Add CI import/configuration/route checks and runtime boundary
  tests. The dashboard workflow now has explicit production-tagged build/test/
  vet and production browser-boundary jobs covering compiler-selected imports,
  configuration rejection, production routes/assets, process startup, browser
  navigation and reverse-request interception. CI also builds the protected
  BIND image and validates both standalone and Simulator configurations,
  including their configured zones, with disposable owner-only key/zone inputs.
  The checker requires a local Docker Unix socket and disables networking for
  validation containers. Capture browser and server egress plus file-access
  evidence; browser request interception alone cannot detect server-side
  reverse dependencies. BIND configuration checks do not establish service
  lifecycle or restart evidence.
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

Demo observation budget (**operator-confirmed 2026-10-25**): changed health/activity is visible within
**10 seconds** after a manual refresh or normal polling, measured from the
injected event. Tests must not hide failures with
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
| V17 | Exercise default-source selection, an approved/revoked cross-source subject link, source outage during positive and absence/negated checks, and identity-changing source edits. | Explicit default/link/version states are visible; only dependent grants deny on outage; changed source versions visibly invalidate dependent grants. | Default persistence and signed staging version binding are locally verified; installed-visa invalidation, selective outage and link lifecycle remain open. |
| V18 | In trusted-source registration, replace the org policy default, reload, then explicitly clear it; edit source identity and attempt to apply an old stage. | Exactly one explicit default is restored; clearing restores unqualified ambiguity errors; stale stage requires restage/new approval. | Desktop/tablet `source-registration.spec.mjs` V03 screenshots verify default replacement/clearing with Simulator unavailable. Go race + actual signed ZPLC stage tests verify identity-version invalidation; full live-visa/browser invalidation remains pending. |

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

| X14 | Bind staged policies conservatively to all configured source identities/versions and pin the canonical binding into existing signed policy metadata. | Active source-policy completion; owner: Policy-Service/VS maintainers. Protected registry, optimistic revisions, identity fingerprints, explicit org defaults and apply-time restage checks prevent stale staged artifacts from rebinding. Display-only changes preserve identity versions; no Simulator dependency. | Authenticate and enforce source version manifests at VS runtime, invalidate installed policies/issued visas, qualify approved issuer/subject links and minimal full-condition dependency sets. Metadata-only restaging is not live-grant invalidation; `validation/2` issuer-qualified ingestion remains a separate gate. |
| X15 | Admit evaluation endpoints by RSA bootstrap keys compiled into the per-install data-plane policy instead of enrollment. | Active from D3 for `-data-plane` evaluation installs only; owner: evaluation-bootstrap maintainers. Each endpoint has a unique per-install key (`-data-plane-endpoints`, 2–16). Private keys are mounted only into that endpoint's Simulator-owned container. The ZPR CA key is discarded, and the policy is compiled in-container with a per-install signing key. | Replace with ordinary enrollment/approval (U04/D2), revocation and re-enrollment; never ship keys compiled into policy. |

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
