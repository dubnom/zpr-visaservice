# Simulator Administrator Workflow Migration

Date: 2026-10-16<br>
Status: Stopped at a verified bounded helper/recovery milestone. Both workers
completed and released their source reservations. The coordinator independently
verified the combined fake-only helper/native selectors and both actual HTTPS
receiver modes under race tests, scoped vet/formatting and the safe no-CGo full
Go suite. No daemon or independent worker-chat links are claimed.
Native generation is implemented source with compile/fake-backed evidence only;
no Keychain operation was executed. The verified 72-case browser milestone and
independently verified endpoint producer/binding-store fixtures remain prior
evidence. Native runtime qualification and production acceptance remain gated.
W1's producer and contract are
complete and verified. C0 completed and verified the disjoint Simulator consumer after W4
remained blocked waiting for the shared artifact; W4 made no consumer edits.
W2's original D3 implementation stopped before edits on an endpoint private-key
ownership blocker; the approved non-import producer seam is now the native
source task's boundary. W5's completed actual-Rust fixture assessment guides
the completed test-only implementation. W3's D2 navigation and W5's isolated Redis-compatible tests
remain complete and independently verified. No live credentials/configuration,
apply, deployment or mutation is authorized.
D1/D2/D3/D4 and first-target/ownership decisions are approved. No live
changes are authorized.

Baseline: `checkpoint-2026-10-16-pre-simulator-admin`
(`b49842bded73fe581b5a67383fd18734bad55b58`).
The tag is a reproducible engineering checkpoint, not a qualified release.
Recheck branch state and changes made since the tag before beginning work.

## 1. Outcome

Simulator becomes a customer of the real ZPR installation, operating through
the configuration and administration workflows available to a real sysadmin.
It no longer establishes production state through private launcher shortcuts.

Operators use Control Room editors to create, maintain and test environment
configuration. The supported production workflow saves revisions, validates/
tests, compiles/signs/stages candidates, explicitly applies them and reports
actual applied state. Simulator then controls simulated machines, adapters,
user activity, workloads, faults and scenario execution.

The same versioned production package must work with either company-provided
integrations or the separate lab companion. Simulator availability must never
be a requirement for production configuration, admission or normal operation.

This plan implements the direction in the
[system release plan](System%20release%20plan.md#simulator-methodology-act-as-a-zpr-administrator).
It does not replace its release gates or the broader
[source reorganization plan](REDIRECTORY.md).

## 2. Ownership and Non-Negotiable Boundaries

| Responsibility | Authority |
| --- | --- |
| Initial service installation, company trust and first operator access | Production installer/bootstrap |
| Configuration records, revisions and domain validation | Production services |
| Policy compilation/signing/staging, authorized push, readback and rollback | Production services |
| Enrollment approval, issuance, revocation and admission decisions | Production enrollment/authentication services |
| Endpoint key generation and protected runtime identity | Individual endpoint |
| Production editor workflows | Control Room / production-owned UI modules |
| Simulated machine creation, activity, faults and workload control | Simulator |
| Scenario definitions and execution timelines | Simulator |
| Lab LDAP/OIDC/DNS lifecycle | Independently supervised companion services |

- Production never reads Simulator manifests, profiles, sessions or workloads.
- Simulator uses supported authenticated APIs, not production databases or
  direct internal package calls as an alternative administration interface.
- No shared cloned adapter identities, fabricated credentials, copied browser
  cookies, disabled TLS checks or blanket administrator service certificates.
- Save, test, stage, compile, apply and runtime acceptance are distinguishable
  states. Existing compile-and-stage behavior need not be split artificially.
- Push, approval, revocation, reset and rollback require explicit authority.
  A worker assignment is not permission to mutate the running environment.
- A scenario reset cannot implicitly erase enrollment/repository state or
  restart company services. Lost responses require reconciliation.
- Missing production functionality is a production backlog item, not a reason
  to retain a hidden Simulator-only fallback.

## 3. Chat Structure

Use **one coordinator chat and five bounded specialist workstreams**. These
are assignments, not five continuously running agents. Start only work whose
inputs and exclusive file ownership are ready.

**This chat is the proposed coordinator and sole routine user-facing status
channel.** It is responsible for asking for decisions, integrating reports
and obtaining deployment approval. The Wave 0 activation below records chats
explicitly launched after user authorization; no continuous monitoring is claimed.

| Chat | Accountable deliverable | Initial independent task |
| --- | --- | --- |
| C0: Coordinator / Integration | Contracts, sequencing, decisions, integration and user communication | Baseline and assignment ledger; resolve cross-stream questions |
| W1: Production Configuration and Apply | Real operator configuration-to-applied-state contract | Inventory existing stage/compile/install/readback paths |
| W2: Endpoint Provisioning | Real enrollment and secure runtime delivery for simulated adapters | Trace current adapter types, identity generation and admission paths |
| W3: Operator Editors | Reuse production editing/test workflows from the lab context | Inventory editors, target context and navigation/session constraints |
| W4: Simulator Orchestration | Replace setup bypasses with operator-service clients | Inventory activation/Restore steps and separate endpoint control |
| W5: Packaging and Acceptance | Independent packages and reproducible boundary acceptance | Record build/test baseline and current package/service inventory |

### Coordinator duties

- **User cadence (2026-10-16): stop only at major milestones.** Discovery
  reports and worker handoffs are internal checkpoints, not requests for the
  user to say "continue". C0 proceeds through contract-ready implementation
  and verification within the authorized scope. Interrupt only for a material
  user decision, unsafe/conflicting state or a blocker that cannot be resolved
  within scope; group related findings into milestone reports. An active C0
  session is not an OS daemon, restart-persistent process, or promise of
  infinite supervision; record its last and next actions when it stops.
- Maintain the assignments, dependencies, decisions and acceptance evidence in
  this plan. Own shared tracker updates rather than asking every worker to edit
  the same status documents.
- Give each worker a bounded task, exact base commit, file/repository scope,
  required tests, exclusions and stopping condition.
- Read available worker reports at integration points; do not claim automatic
  continuous supervision. Request missing evidence before marking work verified.
- Present one concise user update per meaningful milestone or blocker:
  completed, in progress, blocked, decisions needed, next, publication/deployment.
- Route questions from workers to the user as focused decisions. Only the
  coordinator changes the agreed cross-stream contract after consulting owners.
- Integrate dependency changes before dependent implementations, run combined
  tests and reconcile failures. Do not accept a worker's green subset as a
  passing combined system.
- Own coordinated commit/push/tag and deployment requests. Existing tags remain
  immutable; integration and deployment are separately authorized activities.

## 4. Decisions Required Before Dependent Implementation

Discovery can proceed without deciding these. Each decision must record the
chosen option, rationale, affected workstreams and acceptance changes here.

| ID | Decision | Owner prepares options | Blocks |
| --- | --- | --- | --- |
| D1 | Approved 2026-10-16: restricted identity for one explicitly configured disposable lab organization; read/save/test/compile/stage allowed; apply, enrollment approval, revocation, reset and rollback require named human approval. The user later approved independent organization/action enforcement at Policy-Service and Visa Service and signed named-user approval bound to the candidate. Credential acquisition and exact permission/config mapping remain implementation details; no credential or grant is assumed. | W1 + W2, C0 | Implementation of automation authority and privileged orchestration |
| D2 | Approved 2026-10-16: separate-tab navigation to existing Control Room through a deployment-administrator-configured trusted URL. Simulator configuration reads, validates and exposes only that navigation value; Control Room retains its own login/auth/editors. Shared modules deferred. | W3 | Navigation/configuration implementation |
| D3 | Approved: one macOS PH adapter on AuthCode/BAS with an endpoint-generated protected key, no private-key copy/export, trusted locally authenticated helper policy and immutable pre-claim reference. Native runtime qualification remains gated. | W2 + W4 | Provisioning vertical slice |
| D4 | Approved 2026-10-16: durable operation identity for one explicitly selected Visa Service current-policy target; named approving actor; candidate digest/source revision and expected current state; retry/lost-response reconciliation by operation ID; independent applied-state readback; explicit failure state; rollback separately approved and never implicit on timeout. Visa Service owns guarded install/readback; Policy-Service owns durable operation/approval record and invokes Visa Service through an authenticated production API. | W1, C0 | Production apply implementation and client/UI integration |
| D5 | Retain identity, revoke/re-enroll and reset behavior; disposable lab targets and allowed mutations | W2 + W4 | Reset/rebuild migration and live tests |
| D6 | First acceptance host/topology and independently supervised endpoints/providers | W5 | Package/live acceptance |

Do not assume a new authentication grant type, automatic approval, iframe
embedding, shared-origin sessions or direct browser token access. Prefer existing
supported mechanisms where they satisfy the approved authority model.

### D1 decision record (2026-10-16)

**Approved authority model:** restricted lab automation identity, scoped to
**one explicitly configured disposable lab organization**. It may read, save,
test, compile and stage. Apply, enrollment approval, revocation, reset and
rollback require named human approval. This approves an authority boundary
only; it is not permission to change live systems, use credentials, or perform
any production mutation.

At D1 approval time, acquisition/authentication of the automation identity,
action-to-permission mapping, organization enforcement and named-approval
representation remained open. The user subsequently approved independent
organization/action enforcement at both Policy-Service and Visa Service, with
Policy-Service verifying signed named-user delegation bound to the candidate;
generic API keys are not approval. Exact configuration, permission mapping,
key provisioning and runtime trust remain unconfigured implementation/deploy
details. No new grant type, service credential, approval shortcut or weakened
existing authorization is approved. Use compatible existing source patterns
and fail closed when required trust is absent.

### D2 decision record (2026-10-16)

**Approved navigation boundary:** Simulator may expose a separate-tab link to
the existing production Control Room using a trusted URL configured by the
deployment administrator. The administrator owns the URL value; Simulator
configuration owns reading, validating and exposing that value for navigation.
Production must never read Simulator configuration. Control Room retains its
independent login, authorization and editors. Shared UI modules are deferred;
there is no cookie/token sharing, iframe embedding, reverse dependency, or
Simulator-selected organization/target handoff. Do not use a guessed URL or
localhost fallback, or place user/query/manifest-controlled destination,
credentials, organization, target or token values in the link. Missing or
invalid configuration must be reported honestly and safely. This decision
authorizes navigation/configuration code only, not production service changes.

### D4 decision record (2026-10-16)

**Approved desired semantics:** each operation is durably identified per
explicit target and records the named approving actor, candidate digest and
source revision, and expected currently-applied state. Retries and lost
responses are reconciled by operation ID, not by blind mutation replay.
Success requires independent readback of applied state. Partial failures must
be explicit. Rollback is a separate explicitly approved operation and is never
implicitly triggered by timeout or cancellation.

The selected first target is one Visa Service's current policy. Visa Service
owns guarded installation and authoritative current-state readback;
Policy-Service owns the durable operation/approval record and invokes Visa
Service over an authenticated production API. Fleet/downstream enforcement is
explicitly deferred to a later acceptance phase. This is a software ownership
decision, not authorization to apply, rollback or otherwise mutate live state.
Wire shapes, persistence, target trust and the exact existing authorization
context remain implementation details for W1 to derive from source patterns;
do not add self-asserted actor identity or a new grant type. If named approval
cannot be enforced safely, keep the apply client gated while completing only a
safe receiver producer slice. Fixture tests are synthetic and do not establish
live capability.

## 5. Common Handoff Contracts

Agree wire schemas and error semantics before producers and consumers diverge.
These are required information, not invented endpoint names or final schemas.

1. **Target context:** production service origin, organization, repository/
   document identity, deployment target and expected revision. Never derive
   authority from a Simulator display name or manifest path.
2. **Candidate identity:** source/configuration revision, relevant input hashes,
   compiler version, artifact digest, validation results and staging outcome.
3. **Mutation receipt:** operation ID, authorized actor, target, candidate,
   state and actionable error. Define repeat submission and reconciliation
   semantics; cancellation is not automatically rollback.
4. **Applied state:** independently read production revision/digest per target.
   Submission, process health or an old active policy is not proof of success.
   Define partial/multi-target results rather than collapsing them to success.
5. **Enrollment state:** intended asset, endpoint key binding, claim, approval,
   issuance, delivery and admission as separate observations. No secret values
   in audit, UI telemetry or operation receipts.
6. **Scenario evidence:** applied configuration identity, admitted endpoint
   identities, action/result and correlated enforcement observations. A timeout
   alone cannot prove denial.

Version or explicitly lock the agreed contracts before parallel implementation.
Domain owners maintain their API documentation/tests; C0 records agreement and
coordinates changes across all consumers.

## 6. Workstream Assignments

### W1: Production configuration, compilation, apply and recovery

**Own:** production policy/configuration lifecycle and APIs, independent of
Simulator. Start with one existing policy record, not all configuration types.

Starting points:
[policy staging](zpr-dashboard/cmd/zpr-web-dashboard/policy_staging.go),
[Policy-Service](zpr-dashboard/cmd/zpr-web-dashboard/policy_service.go),
[policy forwarding](zpr-dashboard/cmd/zpr-web-dashboard/policy.go), and
[Control-Service](zpr-dashboard/cmd/zpr-web-dashboard/control_service.go).

- [ ] Map supported record/save/test/stage operations and actual install paths.
  Distinguish production-owned behavior from launcher-only behavior.
- [ ] Propose D1/D4 contracts with existing capability, missing capability,
  owner, error ordering, concurrency and authorization requirements.
- [ ] Implement the smallest production apply/readback/reconciliation slice.
  Preserve artifact integrity, explicit target/revision and audit.
- [ ] Add rollback semantics that preserve the last known applied state;
  expose partial/failing deployment explicitly.
- [ ] Test forbidden scope, stale revision, invalid artifact, duplicate request,
  lost response, service restart and failure without replacing good state.
- [ ] Verify all production paths with Simulator stopped/unavailable.

**Handoff:** versioned API/authorization contract, synthetic fixtures, commands
and results. Stop before expanding into unrelated service extraction or
deploying to a live target.

### W2: Simulated adapter provisioning through production enrollment

**Own:** enrollment/issuance/runtime handoff, not Simulator scenario control.

Starting points:
[provisioning plan](../Provisioning%20plan.md),
[enrollment](zpr-dashboard/internal/enrollment/),
[delegation](zpr-dashboard/internal/operatordelegation/), and
[Enrollment relay](zpr-dashboard/cmd/zpr-web-dashboard/enrollment_service_proxy.go).
Changes in BAS or adapter repositories require an explicit extended assignment.

- [ ] Inventory current credential creation, adapter clients and admission.
  Select a proposed first client and identify missing production dependencies.
- [ ] Resolve D3 and prepare D1/D5 options; specify secure headless invitation
  delivery rather than assuming the loopback wizard is headless-capable.
- [ ] Close the required Enrollment/BAS ownership and issuance/handoff gaps
  for the selected client without direct registry access from Simulator.
- [ ] Generate keys on each endpoint; securely persist state and deliver
  credentials without placing them in manifests, logs or checked-in fixtures.
- [ ] Prove two distinct identities, unauthorized/unapproved refusal, duplicate
  claim/replay handling, interrupted delivery reconciliation and restart reuse.
- [ ] Test renewal, revocation and explicit re-enrollment/reset semantics.
  Document software-key protection limits; make no hardware-attestation claim.

**Handoff:** supported enrollment client flow, provisioning state machine,
non-secret fixtures, storage/cleanup requirements and denied-path evidence.
Stop at unsupported platform or protocol decisions rather than inventing a
lab-only issuer.

### W3: Production editor reuse and environment context

**Own:** user-facing production editor access and state presentation.

Starting points:
[operator guide](zpr-dashboard/cmd/zpr-web-dashboard/README.md),
[static UI](zpr-dashboard/cmd/zpr-web-dashboard/static/) and
[browser tests](zpr-dashboard/tests/browser/).

- [ ] Map policy, assertions, ZPR configuration and Gateway editors to current
  production APIs and identify missing target/context support.
- [ ] Prepare D2 options, including login/origin/CSRF implications, navigation
  with unsaved edits and code ownership. No duplicate editor fork.
- [ ] Implement the agreed access/context slice after D2; use W1 fixtures until
  real endpoints are available, with tests explicitly labeled as mocked.
- [ ] Preserve source-local diagnostics, test source identity, History,
  revision conflicts, permission checks, explicit AI review and draft retention.
- [ ] Show saved/tested/staged/applied state accurately; errors must not appear
  as successful setup. Keep live scenario execution separate from draft tests.
- [ ] Test wrong-target requests, stale responses, cancellation and production
  editor operation with Simulator unavailable.

**Handoff:** desktop/tablet browser evidence, API bindings and navigation/
authorization behavior. W3 must not edit Simulator activation scripts or
implement its own deployment/issuance authority.

### W4: Simulator administrator orchestration and endpoint control

**Own:** the Simulator client of W1/W2 contracts and retained machine control.

Starting points:
[activation script](zpr-dashboard/scripts/activate-organization.sh),
[activation manager](zpr-dashboard/cmd/zpr-web-dashboard/organization_activation.go),
and the scenario/machine-control code identified during inventory.

- [ ] Produce a step-by-step activation/Restore inventory: production setup,
  companion-provider setup, endpoint lifecycle, activity and destructive reset.
- [ ] Mark direct compile/install, database writes, privileged file injection
  and service restarts requiring migration; preserve preflight until replaced.
- [ ] Implement explicit production target/configuration inputs and operation
  tracking against agreed W1/W2 fixtures, then the real services.
- [ ] Model setup progress with reconcilable receipts and applied-state checks;
  no retries of uncertain mutations without their approved idempotency contract.
- [ ] Keep endpoints/workload control independent of the web/orchestration
  process so stopping Simulator does not stop production or test endpoints.
- [ ] Run scenarios only against an explicitly selected applied revision and
  admitted identities. Reject wrong-target/stale setup, not silently repair it.
- [ ] Replace one old bypass at a time after equivalent production-path tests
  pass. Legacy selection must be explicit during transition; the new path must
  never fall back silently. Remove redundant paths after acceptance.
- [ ] Separate scenario reset from environment reconfiguration, production
  rollback, credential revocation and company-state removal.

**Handoff:** bypass inventory, client/operation tests, one real environment
setup trace and remaining legacy list. No changes to production compiler/
issuer internals or unapproved runtime resets.

### W5: Package boundary and acceptance

**Own:** reproducible artifacts, independent supervision and acceptance harness.

Starting points:
[production packaging](zpr-dashboard/packaging/system/),
[companion packaging](zpr-dashboard/packaging/simulator/),
[lab bootstrap](zpr-dashboard/cmd/zpr-lab-bootstrap/) and
[release process](System%20release%20plan.md).

- [ ] Record current test/build baseline and package gaps. Capture environment,
  exact commands and failures without changing unrelated tests.
- [ ] Define compatible production/companion artifact manifests and explicit
  integration handoff, with pinned versions and protected persistent state.
- [ ] Establish service/process ownership: installer bootstrap, independent
  providers, production services and simulated endpoints. Identify stop/reset
  commands whose side effects currently violate this boundary.
- [ ] Build the synthetic acceptance harness while contracts stabilize.
  Mock success is not live network or package acceptance.
- [ ] Assemble the first candidate from integrated pinned revisions and test
  from a fresh supported Ubuntu host without a source checkout or toolchain.
- [ ] Exercise browser/manual and automated operator paths, enrollment,
  applied-state readback, real allowed/denied traffic, interruption/restart,
  rollback and Simulator-unavailable operation.
- [ ] Publish evidence and limitations against exact artifact IDs. Unsigned
  engineering candidates must be labeled; signing/qualification is separate.

**Handoff:** reproducible commands, candidate manifest, boundary assertions,
failure evidence and acceptance report. Only C0 coordinates live shared-host
execution after user approval.

## 7. Parallelism and Dependency Gates

| Wave | Work that may run concurrently | Exit gate |
| --- | --- | --- |
| 0: Baseline/discovery | W1 lifecycle inventory; W2 enrollment trace; W3 editor inventory; W4 bypass inventory; W5 test/package baseline | C0 reconciles existing/missing capability and exact ownership |
| 1: Contract decisions | W1/W2 authority and operations; W3 editor options; W4 state/client design; W5 harness design | D1-D4 recorded before affected implementation; D5-D6 before reset/live tests |
| 2: Isolated implementation | W1 backend; W2 provisioning; W3 editor shell with fixtures; W4 clients with fixtures; W5 harness/package assembly | Contract tests and scoped suites pass independently |
| 3: Integration | C0 integrates producers first, then consumers; W5 executes isolated integrated tests | One end-to-end vertical slice passes against real services |
| 4: Migration | W4 replaces setup bypasses; W3 completes editor lifecycle; W2 expands lifecycle tests; W5 qualifies candidate | No hidden setup authority; production independent of Simulator |
| 5: Release acceptance | Coordinated fresh-host, failure/recovery and Simulator-off tests | Exact candidate evidence, limitations and explicit release decision |

Do not run all five workers by default. Start the concrete discovery tasks in
small batches; implement only contract-ready work. A blocked worker should
report the dependency and stop, not modify another owner's domain.

### First integrated vertical slice

1. Use the standard bootstrap to establish services, trust and operator access.
2. Select a disposable, explicitly approved organization/target.
3. Author and test one policy/configuration revision in the production editor.
4. Explicitly compile/stage/apply through the authorized production workflow;
   verify exact applied revision/digest through independent readback.
5. Enroll two simulated adapters with distinct endpoint-generated keys and
   production-issued credentials; verify admission.
6. Drive one allowed and one denied request over the actual adapter/node path,
   retaining bounded responses and correlated enforcement observations.
7. Stop Simulator orchestration only. Keep endpoints/providers running and
   independently drive traffic; production login/configuration reads,
   admission and policy behavior must remain available.
8. Exercise a scoped failed apply/lost response and approved rollback, proving
   no duplicate mutation or silent replacement of the last good configuration.

The vertical slice is complete only when evidence crosses real service and
packet boundaries, not when fixture APIs return the expected JSON.

## 8. File Ownership and Integration Rules

- Prefer isolated Git worktrees/branches per implementation chat, with a common
  recorded base. Never run multiple writes in the same working directory.
- Worktrees do not isolate container names, ports, mounted databases, Go/browser
  output or live state. Allocate unique fixtures/output directories and reserve
  shared runtime access through C0.
- W1 owns production policy/config backend files; W2 owns enrollment/identity
  files; W3 owns production editor UI; W4 owns Simulator orchestration.
  W5 owns packaging and acceptance harness additions.
- Shared files such as `main.go`, `control_service.go`, static entry HTML,
  shared navigation, manifests and `dashboard.spec.mjs` require an explicit
  single-writer assignment. Consumers submit a bounded patch/request; C0
  schedules the edit rather than permitting overlapping writes.
- Existing authentication work belongs to its current owner. D1 changes must
  be coordinated with that owner, not developed over their uncommitted edits.
- New package paths and API names are proposals until reviewed. Prefer cohesive
  existing owners; do not create parallel abstractions to avoid coordination.
- C0 alone updates this plan, the GUI backlog, priorities, release ledger and
  canonical progress summaries during a batch. Workers update domain docs
  within assigned scope and return proposed summary text.
- Preserve unrelated work, private discovery artifacts and the existing
  `zpr-common` exclusion. Shared-schema changes require a separately agreed
  cross-repository task and compatibility evidence.

## 9. Baseline, Verification and Release Gates

The checkpoint records five full Go failure groups: Scenario skill-text
expectation, enrollment readiness, stale-bundle platform fixture, placement
shell extraction and trusted-service ELF fixture. The checkpoint login browser
selection recorded 20 passes and two callback connection-refused failures.
The earlier Organization selection recorded 22 failures; their current state
must be rerun, not assumed unchanged or fixed.

- Establish fresh per-worktree baselines before edits. Do not patch existing
  failing tests without the permission required by repository instructions.
- Run focused tests/races for changed behavior, required `make test` for Go
  changes, formatting assertions, vet and relevant target builds.
- Browser tests must cover actual assets/CSP and desktop/tablet behavior.
  Record fixture versus real authentication/service/network evidence.
- New or tightly coupled regressions block integration; historical unrelated
  failures remain visible and need an explicit disposition before qualification.
- Test authorization, stale revision, partial apply, malformed responses,
  interrupted provisioning, lost receipts, restart, expiry and rollback.
- No live configuration, credential, service, resolver or network mutation
  without explicit approval of target/scope and a recovery plan.
- A passing engineering slice does not close all seven refactoring gates or
  certify either complete installation bundle.

## 10. Assignment and Handoff Protocol

Copy this task envelope into each specialist chat:

> Workstream/task ID:<br>
> Objective and explicit stopping point:<br>
> Base commit, branch/worktree and repositories:<br>
> Exclusive files; shared-file reservations; forbidden surfaces:<br>
> Approved contracts/decision IDs and prerequisite commits:<br>
> Required tests and baseline failures:<br>
> Allowed fixture/runtime actions; prohibited live actions:<br>
> Deliver code, domain documentation, test evidence, limitations and a concise
> handoff to C0. Do not commit/push/deploy unless explicitly assigned.

Workers report:

- State: queued / working / blocked / ready for integration / verified.
- Exact changes and contracts used; any new decision needed.
- Test commands/results, baseline differences and evidence locations.
- Integration prerequisites, remaining work and safe next task.
- Commit/branch if authorized; no secrets, private inspect data or live codes.

### Initial assignment ledger

| Chat | Assignment | State | Dependencies |
| --- | --- | --- | --- |
| C0 | Plan, user decisions and bounded intake | D1/D4 and ownership recorded; verified receiver-producer milestone; coordinating next contract-ready work | Discovery reports + D4 follow-up |
| W1 | Production policy target/API owner | Receiver-producer implementation verified in fixtures; Policy-Service approval/auth integration remains gated | D1/D4 and target/ownership choices approved |
| W2 | Trace provisioning and propose D1/D3/D5 | Discovery complete; no current assignment | D3/D5 for dependent provisioning/reset |
| W3 | Inventory editor ownership and propose D2 | Report received; discovery complete | None |
| W4 | Inventory activation/Restore bypasses and endpoint supervision | Report received; discovery complete | None |
| W5 | Inventory baseline evidence/package/supervision gaps and propose D6 | Report received; discovery complete; fresh tests deferred | None |

W1 completed a bounded read-only D4 target-receiver follow-up, then the
fixture-only receiver-producer assignment in an exclusive single-writer code
scope. Its source changes and verification are recorded in §12. The discovery
follow-up evidence is in `migration-w1-d4-target-followup.txt`.

### Wave 0 activation (2026-10-16)

The user authorized beginning the plan. C0 launched the following bounded
read-only discovery chats. Each must stop with an evidence-backed report,
proposed exclusive implementation scope, prerequisites and tests. No worker
may edit source, inspect live credentials/state, start services or deploy.
All workers share the checkout read-only; future write assignments require
isolated worktrees or an explicitly reserved single-writer scope.

## 12. Wave 0 Reconciliation and Decision Gate Update (2026-10-16)

The five full discovery reports are in the coordinator session artifacts
`migration-w1-discovery.txt` through `migration-w5-discovery.txt`. Their
findings reconcile as follows; discovery is not implementation or runtime
acceptance evidence.

| Workstream | Established capability | Missing contract/capability | Implementation gate |
| --- | --- | --- | --- |
| W1 production policy lifecycle | Versioned record save/history, validation/test, content-addressed compile/sign/stage; Visa Service Admin API has a single-service install (`POST /admin/policies`) and current-bundle content readback (`GET /admin/policies/curr`) | Existing receiver has no expected-state precondition, operation ID/receipt/status or retry reconciliation, named actor attribution, partial/fleet outcome or rollback. Its public readback omits internal vinst/phash. Generic Control Room policy write permission is not by itself named approval | Implement against the approved single-Visa-Service ownership split; derive trusted actor/auth context from existing source patterns. Resolve D1 credential, permission-mapping and organization-enforcement mechanics without inventing grants |
| W2 provisioning | Key-bound claim, named review, status/resume, and enrollment-backed BAS authorization primitives with local tests | No credential issuance/delivery in device status, signed helper/adapter key handoff, completed admission/revocation integration, or sole-writer service boundary end-to-end; loopback setup is not headless. BAS opens a configured SQLite registry, but no deployed shared-database configuration was established | D3 now selects one macOS PH/AuthCode/BAS target. W2 is assigned the protected key-handoff/client implementation after exact sibling-repo guidance/status/exclusion checks. Authentication actor and trusted validation/2 availability remain unproven prerequisites; D5 blocks reset/rebuild/revocation/re-enrollment behavior only. |
| W3 operator editors | Control Room owns production policy/assertion, ZPR Config and Gateway editors, with source/revision-aware save and test flows | No Simulator-selected target handoff or editor apply/readback; no trusted deployment-owned Control Room URL source identified for a navigation-only entry | D2 and an approved URL/configuration source and missing-URL behavior |
| W4 orchestration | Activation/reset still owns local profile-driven compile/install/restarts and runtime/provider/machine lifecycle; scenario machine control is a distinct Simulator responsibility | No production target/revision client or durable receipt reconciliation; Restore includes destructive local-state deletion; endpoint/provider survival after stopping orchestration is unproven | D4/target-owner contract and unresolved D1 enforcement mechanics for policy migration; D5 for reset/identity behavior; D3 for connected endpoint lifecycle; D6 for integrated topology acceptance |
| W5 packaging/acceptance | Partial production preflight fixtures, lab bootstrap and selected provider integration tests | No installable production or companion bundle, pinned combined package acceptance, or end-to-end harness against agreed public contracts; historical test results are not a fresh baseline | W1/W2 contract seams for meaningful harness assertions; D6 for integrated/live topology acceptance |

At initial reconciliation, no implementation task was contract-ready. The
subsequent D1/D4 and target/ownership decisions below make a bounded production
receiver and operation-record slice appropriate. D1 authentication and
permission mapping must follow existing source patterns and be tested; they
are not license to invent grants or trust actor-supplied fields. The existing
Visa Service receiver lacks D4 operation semantics. W1 implementation is
fixture-only; no tests or live services may mutate production targets.
Historical failures remain as recorded; unrelated tests are not to be
modified.

**D1 and D4 are recorded as approved**, and the user has also approved the
first target and software-owner split: one Visa Service current policy;
Visa Service owns guarded install/readback; Policy-Service owns durable
operation/approval records and calls Visa Service through an authenticated
production API. Simulator is only a consumer of the supported operator
workflow. Fleet/downstream enforcement remains later. These decisions do not
authorize live changes. D5/D6 remain independent gates and do not block this
W1 production receiver/operation slice. D3 is selected and its W2
implementation/prerequisite work is active; endpoint-dependent acceptance
remains gated until that work and trusted service prerequisites are verified.

The bounded W1 follow-up is complete; its full report is
`migration-w1-d4-target-followup.txt` in the coordinator session artifacts.
The follow-up report's statement that D1 is still unresolved predates/does not
reflect the approved D1 boundary recorded above; treat its named-actor
authentication and permission details as implementation mechanics to resolve
from existing source patterns, not as an undecided authority-model choice.
It establishes a concrete *single Visa Service* receiver in `vs/src/
admin_service.rs` and `vs/src/policy_mgr.rs`: `POST /admin/policies` installs
the current bundle and `GET /admin/policies/curr` returns current bundle
content. Internal state has vinst/phash, but the public readback does not
expose them. The receiver does not compare expected current state, persist or
reconcile operation IDs, attribute a named approving actor, report fleet
partial results, or provide a separately approved rollback. Reposting even
identical bytes increments vinst. The user-approved software owner split is
Visa Service for receiver install/readback and Policy-Service for durable
operation/approval plus the authenticated API client. This is source-level
evidence, not deployment or runtime verification.

The ownership decisions cleared the target-scope gate without asserting that
every capability exists. W1 completed the smallest safe receiver-producer
slice in the exclusive Visa Service code scope, with synthetic fixtures only.
It adds durable operation reservation/commit/receipt handling, expected-state
CAS and operation-ID replay, exact current-policy digest/version readback, and
honest reconciliation states. `Succeeded` finalization atomically checks
persisted digest/version and the exact prior receipt; a replay whose persisted
commit markers have been superseded is `Uncertain`, while stale pre-commit
state without applied markers is `Failed`.

Approval actor attribution and the Policy-Service client/route were not safely
wireable from the current generic API-key permission context. No HTTP route,
client, caller-supplied actor/header bypass, or success-shaped stub was added;
apply remains gated until trusted named approval can be carried and enforced
using existing production authorization patterns. D1 credential acquisition,
action-to-permission mapping, organization enforcement, and named-approval
auditing remain unresolved implementation contracts. At this receiver-only checkpoint, this slice did not provide fleet/downstream
acceptance, Redis execution, live apply evidence, or rollback. W5's later
isolated Redis-compatible test milestone is recorded below.

W1's source reservation covered Visa Service receiver/policy manager/database
files and tests only; actual changes are limited to `vs/src/db/mod.rs`,
`vs/src/db/db_fake.rs`, `vs/src/db/db_redis.rs`, `vs/src/db/policy.rs`, and
`vs/src/policy_mgr.rs`. No Policy-Service, HTTP, or Go files changed. The
reserved source files passed focused receiver/operation tests and the complete
Rust workspace `cargo test`; rustfmt check and `git diff --check` passed at
that checkpoint. Redis script logic was then covered by source review and
FakeDb fixtures; later isolated Redis-compatible tests are recorded below. No live service, network,
credential or production mutation; no commit/push/deploy. Unrelated dirty
worktree files and historical failures were left untouched.

**Coordinator state at receiver milestone:** the first fixture-only
receiver-producer milestone is verified. W1's implementation handoff is
complete; no shared source reservation remains active. C0 reviewed the
finalizer/replay state corrections and independently reran the workspace test
command, formatting check and diff whitespace check. The later auth provenance
follow-up and current blocker are recorded below. D2, D3, D5 and D6 remain
undecided and gate only their dependent work. No live operation is authorized.

### Named-approval provenance follow-up

The prior source trace found the existing named-user path was proven for
**Enrollment only**. The user has now approved extending enforcement at both
Policy-Service and Visa Service, because the production target-to-organization
binding is not established:

> Enforce organization/action authorization at both Policy-Service and Visa
> Service. Policy-Service verifies signed named-user delegation and binds
> approval to the exact candidate. A generic API key is not approval.

This selects enforcement placement; it does not create or provision production
signing keys, trust files, API credentials, or a live target binding, and it
does not authorize a live mutation. No caller-supplied actor field/header can
establish identity or approval.

- Control Room OIDC verifies the issuer/subject and maps that exact identity
  to configured organization and permission grants in
  `zpr-dashboard/internal/operatorauth/auth.go`. Mutations require same-origin
  CSRF. `operatorEnrollmentProxy` maps enrollment routes to action permissions,
  requires an explicit organization, and signs the verified identity and
  exact request method/URI/body using a short-lived audience-bound delegation.
- Enrollment Service independently verifies the signed assertion, mTLS peer
  certificate pin, audience, expiry, nonce/replay protection, exact
  issuer/subject grant, organization and action; it records the verified
  issuer/subject as the audit principal. This proves a reusable pattern, not
  an existing Policy-Service/ Visa Service trust path. The delegation prefix,
  signer and verifier are currently wired only for enrollment.
- The Change Review workflow records named submitter/reviewer and an immutable
  proposal/digest in a Control Room-owned SQLite store. It requires
  `change.review`, checks the proposal remains current, and disallows
  self-approval by default. Its `apply` route explicitly returns 409; its
  approval record is not in Policy-Service and is not a Visa Service operation
  receipt.
- Policy-Service currently receives mTLS-authenticated Control Room requests
  but has no named-user delegation verifier, independent organization/action
  grant check, or apply route. Visa Service Admin authentication accepts
  `X-API-Key` and returns only generic `Read`/`ReadWrite` permission; that does
  not identify a human approver or enforce organization/action scope. No
  verified source path currently connects an approval to a guarded policy
  install.

Source evidence: `operatorEnrollmentProxy` and signer setup in
`zpr-dashboard/cmd/zpr-web-dashboard/operator_delegation.go`;
`operatorauth.Auth.Authorize` in `zpr-dashboard/internal/operatorauth/auth.go`;
delegation verification in `zpr-dashboard/internal/operatordelegation/
delegation.go`; independent Enrollment checks in
`zpr-dashboard/internal/enrollment/operator.go`; Change Review behavior in
`zpr-dashboard/cmd/zpr-web-dashboard/change_review.go` and
`zpr-dashboard/CHANGE_REVIEW.md`; Policy-Service routes in
`zpr-dashboard/cmd/zpr-web-dashboard/policy_service.go`; generic Visa Service
key permission in `vs/src/admin_service.rs`.

**Implementation sequencing (approved boundary; implementation details are
owned by W1/C0):**

1. W1 owns the receiver-side Visa Service contract first. It must enforce the
   explicitly configured organization and action independently of generic
   API-key permission, validate trusted signed approval/operation proof bound
   to exact target, organization, action, candidate digest/source revision,
   expected current state, request method/route, operation ID and bounded
   expiry/replay controls using compatible repository-reviewed primitives.
   Configuration absence or invalid trust must fail closed for the guarded
   route. Existing generic `POST /admin/policies` must not bypass guarded
   operations when that mode is enabled; the old route's status outside the
   guarded mode must be reported honestly and is not evidence of named approval.
2. C0 then owns the non-overlapping Go Policy-Service/Control Room producer
   work: extend the existing signed named-user delegation pattern to a
   distinct Policy-Service audience; validate the approved Change Review
   record and current candidate; bind the signed request to exact issuer,
   subject, organization, action, target and candidate; enforce the same
   one-organization config; persist canonical approval/operation state before
   invoking Visa Service; and fail closed on missing/stale/wrong-org/
   wrong-action/untrusted/replayed/mismatched-candidate input. No route/client
   will be wired until W1's receiver contract is reviewed.
3. The guarded flow requires both layers. The trusted target organization must
   come from explicit operator configuration and must be validated against
   known policy/source ownership; never infer it from bundle metadata or trust
   caller input. The service credential is separate transport authentication,
   not approval.

W1's receiver implementation was carried out in an exclusive Visa Service
scope. W2 was not needed for another delegation trace: its discovery report
and the direct source trace establish the Enrollment-only verifier pattern.
No production signing key, verifier key, trust grant, or credential is assumed
to exist; deployment setup remains outside this coding authorization.

**Redis test milestone and remaining limits:** W5 added six ignored
`#[cfg(test)]` unit tests inside the existing private `RedisDb` module, without
changing production statements, public API or manifests. C0 independently
reran the focused tests against six private loopback Valkey 9.1.2 children
(all six reaped and temporary directories removed), then `cargo test -p vs`
(6 library and 511 service tests passed; six Redis tests ignored by default).
Owned-file `rustfmt --check` and `git diff --check` pass, and the original
462-line production prefix matches its pre-test SHA-256. These tests exercise
duplicate reservation, stale CAS, application-level lost-response replay,
receipt/finalizer races and no-TTL nonce/approval retention using the existing
Lua methods. The lost-response test does not inject a socket-level failure;
the run qualifies Redis-compatible Lua behavior on Valkey, not upstream Redis
distribution behavior, restart/persistence, HTTP authentication, proof
expiry, or live apply. TTL `-1` confirms nonce and approval keys do not expire;
no retention policy change was made. Crate-wide `cargo fmt -p vs -- --check`
still reports the untouched historical formatting issue in
`vs/src/connection_control.rs:938`; the owned file passes.

**Historical coordination state at dual-enforcement approval:** W1 was assigned
the bounded receiver implementation and C0 reserved the Go Policy-Service/
Control Room producer until the receiver contract was reviewed. That receiver
has since been verified. The loop was stopped at that milestone and is now
resumed for the user-approved Go producer/client slice. No production route,
credential, grant, dependency, live action or deployment is implied by the
approval.

**Go test baseline observed before C0 Go implementation:** `go test
./internal/operatorauth ./internal/operatordelegation ./cmd/zpr-web-dashboard`
passed the operator-auth and delegation packages, but the dashboard package
failed in existing unrelated tests: `TestAssistantSkillsEmbeddedAndTaskSpecific`
(missing scenario instruction), `TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator`
(readiness preflight returned 409), `TestOrganizationPolicyInstallNeverUploadsStaleBundle`
(`ZPR_MULTINODE_PLATFORM` unset), `TestScenarioPlacementPreflightMatchesLauncher`
(`resolve_machine_placement` missing in six cases), and
`TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact` (test helper
was not a Linux amd64 ELF on this host). These tests were not modified. The
local `redis-server` and `valkey-server` binaries are available, but no
repository integration harness for the new Redis scripts was found; neither
service was started, so Redis-backend atomicity remains unverified.

**Go producer seam evidence:** the current Control Room staging route compiles
the saved policy revision and returns its bundle digest, while Change Review
captures/revalidates saved source and validation context without recording that
staged bundle digest; its apply handler remains explicitly disabled. Any future
producer must durably bind the reviewer decision to the exact staged candidate
and reject a changed/replaced stage. Policy-Service must verify the named-user
delegation on its distinct audience, persist the canonical approval/operation
before calling Visa Service. The receiver must independently validate a
Visa-Service-audience proof from a repository-reviewed trust chain; do not
forward the Control Room-to-Policy-Service token across the audience boundary.
Prefer existing signer patterns and distinct audience-bound proofs; do not
add a new Policy-Service signing authority unless an existing compatible
pattern requires it and configuration fails closed.

**W1 guarded receiver/readback milestone verified:** the reserved VS files now
contain a guarded `POST /admin/policy-operations` requiring both a write API
key and a signed Ed25519 proof, exact configured target/organization/action/
candidate/expected-state/request binding, a 5-minute maximum proof lifetime,
durable nonce and operation-approval binding, and guarded-mode rejection of
the legacy policy POST even when trust is invalid. Configuration pins signer
issuer/key expiry and exact named-user organization/action grants.

W1 also added **`GET /admin/policy-operations/current`**, requiring a
read-capable API key and valid guarded configuration. It returns configured
target/organization and exact persisted SHA-256 digest/vinst after validating
the container hash and rechecking state. Missing, corrupt, unverifiable or
changed state fails closed, supplying the Policy-Service CAS input without
trusting its own cache.

Validation: W1's focused operation/auth tests passed (14), the Visa Service
package passed (517 tests), and C0 independently passed the full Rust
workspace suite (678 tests); reserved-file rustfmt and `git diff --check`
passed. No live Redis was contacted. Nonce replay keys currently persist
without TTL; proof lifetime is capped at 5 minutes, but Redis storage grows
with accepted proofs and retries.

**Coordinator state (2026-10-16): ACTIVE bounded session turn.** The user
directed the next slice without another approval gate. W1 is assigned the
exclusive Go producer/client contract below; C0 owns this plan/state document
and performs integration review only. No live credentials, services, apply,
rollback, commit or deploy are authorized.

**Fresh Go baseline before the producer assignment:** from
`zpr-dashboard/`, `go test ./internal/operatorauth
./internal/operatordelegation ./internal/changereview
./cmd/zpr-web-dashboard` passed operatorauth, operatordelegation and
changereview. The dashboard package retained its unrelated failures:
assistant skill content, enrollment readiness 409, unset
`ZPR_MULTINODE_PLATFORM`, missing `resolve_machine_placement` in six cases,
and the non-Linux ELF helper. These pre-existing failures are not to be
modified. No Go implementation test failure baseline exists for new paths.

**W1 producer/client assignment (completed, exclusive Go writer):** implement
candidate-bound Change Review approval, signed named-user delegation to
Policy-Service with a distinct audience, Policy-Service org/action verification
and durable operation/approval reconciliation, plus the authenticated Visa
Service client using the exact W1 receiver schema and routes. Bind to the
authoritative staged bundle digest/source revision and VS current
`target_id`/organization/digest/vinst before constructing the operation;
require an approved review event whose named reviewer is distinct from the
submitter under existing default policy; derive identity only from verified
session/store state, never caller fields. Sign audience-/purpose-separated,
method/URI/body/expiry/nonce-bound proofs compatible with the VS
`PolicyApprovalClaims`, and use the exact stable request body for retries.
Persist canonical operation/approval before invoking VS; reconcile transport
loss by operation ID through supported VS status/readback behavior; represent
uncertain/failed outcomes explicitly and never auto-rollback. Fail closed when
config/trust/current state/stage/approval is absent or stale. No route may
fall back to generic API key as approval.

Reserved Go paths: `cmd/zpr-web-dashboard/operator_delegation.go` and its
tests; `cmd/zpr-web-dashboard/change_review.go` and its tests;
`cmd/zpr-web-dashboard/policy_service.go` and its tests; the new
`cmd/zpr-web-dashboard/policy_apply.go` and focused tests; and
`internal/operatordelegation/delegation.go` plus tests only if needed for the
existing signer/verifier pattern. C0 will not edit these paths during the
assignment. Update the directly related dashboard/operator docs and README
with fixture-only/gated status, exact configuration requirements and limits.
No dependency unless existing reviewed primitives cannot satisfy the contract.
If complete trusted chain or durable reconciliation cannot be safely wired,
keep apply disabled; do not add a success-shaped stub. Scope completion
requires targeted tests, full `go test ./...` report retaining baseline
failures, `gofmt`, and `git diff --check`.

No Go apply producer or live acceptance is currently verified. W1 completed a
gated fixture-tested producer/client slice: candidate-bound review provenance,
distinct PS/VS proof pair, a SQLite operation/approval ledger helper using
`changereview.Store`, and a VS client with receipt validation plus independent
readback. This ledger is not yet composed as Policy-Service-owned persistence.
No apply route is wired or enabled. W1 reports focused auth/delegation/review
tests and six new top-level tests passing, with scoped race/vet/gofmt/diff
checks passing. Its full Go suite report is 35 packages passed, one failed,
six skipped/no tests; the five top-level failures match the captured
pre-existing baseline. C0 source review and independent reruns remain pending.
C0's review found and W1 corrected a cross-organization binding gap in the
unregistered producer helper: `PolicySigner.ValidateScope` now requires the
review organization and receiver target to match current signer trust before
review lookup/signing. W1 added a valid-other-org-approval regression and
reports 12 targeted producer/review tests passing. C0 has inspected the guard
and regression; independent reruns remain pending. The helper remains gated
and no route is registered.

**Concurrent work DAG (2026-10-16):** W1's Go producer/client implementation
has completed and is under C0 integration review. The live Git status showed
its edits in
`zpr-dashboard/cmd/zpr-web-dashboard/change_review.go`,
`zpr-dashboard/internal/operatordelegation/delegation.go`,
`zpr-dashboard/internal/operatordelegation/delegation_test.go`, and new
`zpr-dashboard/cmd/zpr-web-dashboard/policy_apply.go` plus its test. Its
exclusive reservation covered the Go producer paths; no other writer edited
them. W1 says production activation still requires protected configuration/
TLS loading, durable Control Room review-to-operation retry association and
Policy-Service lifecycle composition. C0 found and W1 corrected the producer
helper's review-org/signer-trust mismatch; C0 independently reran the
`operatordelegation` and `changereview` packages and focused `TestPolicyApply`
tests successfully, as recorded in the current checkpoint below. W5's first external-integration-test attempt stopped with no repository edits
because the DB API is private to the VS binary crate. The user then authorized
a narrowly scoped test-only unit module inside `vs/src/db/db_redis.rs`, without
production logic or public API changes. W1 released that Rust file reservation.
W5 completed the tests and reported the handoff ready for review; C0
independently reran and verified the focused and full crate suites, formatting,
diff and unchanged production prefix. No container/shared/live service was
used. No TTL change is authorized. At that earlier checkpoint W2 provisioning
was blocked by D3; D3 is now approved and W2 has a bounded implementation
task. W3's D2 navigation implementation is complete; its files are released.
At that earlier checkpoint W4 had no separate assignment because the W1
operation contract was not yet composed. This is superseded by the current
bounded W4 task recorded below.

W3's bounded navigation/configuration implementation is now unblocked by D2.
The source/configuration owner is decided: the deployment administrator owns
the value and Simulator configuration reads/validates/exposes it only for
navigation. W3 owns `cmd/zpr-web-dashboard/static/simulator-nav.js`,
`static/index.html`, `cmd/zpr-web-dashboard/main.go` only if needed to wire the
existing configuration pattern, a new focused browser regression, and
`packaging/system/README.md`. These paths were checked clean before assignment.
W3 must not touch W1's Go producer/auth paths or the already modified
`zpr-dashboard/README.md`; the latter remains W1's documentation reservation.
Shared modules, query semantics and target-aware handoff remain out of scope.

W1's Rust receiver source shows proof lifetime capped at five minutes while
Redis nonce/approval records have no TTL. W5's completed isolated tests now
verify atomic operation outcomes and observe TTL `-1` for nonce/approval keys
against Valkey 9.1.2. Proof expiry remains a separate receiver check; the tests
do not wait five minutes or simulate server restart. The test-only assignment
is released, and no retention change was made.

**Current concurrent checkpoint (2026-10-16):** W3 completed the
navigation/configuration slice. The deployment administrator configures
`ZPR_SIMULATOR_CONTROL_ROOM_URL`; W3 reports safe origin validation, an honest
unavailable state, and separate-tab navigation without context/token handoff.
W3 reports its focused Go tests and 38 desktop/tablet browser tests pass,
including the Control Room Simulator-unavailable contract. W3 reports a
full-suite Go failure; W3 reports the same failure reasons reproduced in a
W3-excluded source overlay (not a pristine repository baseline). W3 released
`main.go` and `simulator.go`. C0 independently reran the navigation browser
suite: 38 passed (19 Desktop Chrome, 19 iPad Pro 11 Chromium). C0 also
independently reran W1's cross-organization scope regression and producer
tests uncached: the `operatordelegation` and `changereview` packages and all
`TestPolicyApply` tests pass. W5's baseline `cargo test -p vs` passed (6
library tests and 511 service tests). W5 then added six ignored tests in the
test-only module in `vs/src/db/db_redis.rs`; C0 independently reran all six
against private loopback Valkey children (verified cleanup) and the full crate
suite (6 library and 511 service tests passed, six ignored). The original
production prefix is byte-identical; owned-file formatting and diff checks
pass. The worker report and logs are in its session artifacts. The Redis
backend test evidence is limited to Valkey-compatible behavior, application-
level response loss, and no-TTL observation; it does not qualify upstream
Redis, physical connection loss, persistence/restart or full service behavior.
**W1 synthetic Policy-Service composition milestone verified (2026-10-16):**
Control Room Change Review calls Policy-Service over configured verified mTLS;
Policy-Service alone calls the Visa Service contract over protected TLS with
a separate API credential. Explicit `policy.apply` and immutable named
reviewer approval are distinct; both service trust layers bind the named
issuer/subject, one configured organization, install action, target,
candidate/source revision, expected current digest/version, operation ID and
short-lived audience/method/URI/body-bound proofs. Policy-Service consumes the
nonce and durably stores the canonical operation before dispatch. Retries
retain the same operation ID/binding; an independent exact digest/version
readback is required for success. Missing protected configuration leaves the
PS routes unregistered and Change Review apply disabled; invalid supplied
configuration fails startup. No runtime keys, trust, grants or live target
configuration are assumed. Legacy VS install remains guarded only when its
guarded trust is configured; this does not claim the legacy API is globally
secured.

C0 independently ran the operatorauth/delegation/Change Review package tests
and 27 dashboard/composition tests under race detection; all pass. Scoped
`go vet`, `gofmt -l` and `git diff --check` pass. The full
`go test -json -count=1 ./...` exits 1: 35 packages pass and the dashboard
package contains exactly the five established failures, matching W1's
pre-edit baseline:
`TestAssistantSkillsEmbeddedAndTaskSpecific`,
`TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator`,
`TestOrganizationPolicyInstallNeverUploadsStaleBundle`,
`TestScenarioPlacementPreflightMatchesLauncher`, and
`TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact`.
No existing failing test was changed.

This is synthetic contract/transport-fixture verification, not real
Control-Room/Policy-Service/Visa-Service network interoperability or Redis
deployment qualification. Operator-provisioned protected signing/trust/TLS/API
credentials and deployment authorization are absent; no service, live apply,
rollback, fleet, enrollment or Simulator dependency was exercised. D3 is
selected but not yet implementation-verified; D5/D6 remain undecided and block
only their dependent work.

**D3 approved (2026-10-16):** the first connected adapter target is ONE macOS
PH adapter using its existing AuthCode/BAS path, endpoint-generated unique
keys, signed helper IPC, System Keychain storage, and adapter Keychain
signing. No copied/shared endpoint identity or headless browser shortcut is
approved. W2 is assigned a bounded source/fixture implementation after it
reads guidance and reports exact clean sibling-repository paths/status. The
trusted Authentication actor and trusted validation/2 service are
prerequisites, not proven available; code/tests must fail closed when absent
and no live availability/admission is claimed. Process restart must retain the
same endpoint identity. D5 remains undecided and blocks destructive reset,
revocation, and re-enrollment only.

**Bounded loop stopped (2026-10-16):** W4's earlier attempt to replace the local
production-policy compile/install/policy-context-restart bypass was blocked
and made no changes. The initial focused baseline reproduced the existing
`TestScenarioPlacementPreflightMatchesLauncher` failure; Node.js was
unavailable for shell tests. The approved read-only operation lookup resolved
the evidence gap without granting Simulator apply or signing authority.

**W1 producer verified:** the read-only route is
`GET /api/policy/operations/status/{organization}/{target_id}/{operation_id}`.
The compact frozen contract and final implementation report are persisted at
`migration-w1-policy-lookup-contract.txt` and
`migration-w1-policy-lookup-handoff.txt` in C0's session files. The route uses
the real six-field `PolicyOperation`, `PolicyReceipt` and
`PolicyCurrentState` types, immutable named approver binding and durable
operation record; its client obtains fresh VS current state through the
existing protected GET-only client. Explicit pinned mTLS client-auth plus
server-side exact org/target authorization is required. It enforces canonical
selectors, no-store, 16 KiB response cap and a 10-second context; no lookup
mutation, retry or enumeration is allowed. Success requires the exact durable
operation/receipt/recorded readback/fresh readback chain; higher `vinst` is
superseded even when the digest matches. W1's scoped race tests, internal
package tests, vet, formatting and diff checks passed. `make test` retained the
same five historical dashboard failures:
`TestAssistantSkillsEmbeddedAndTaskSpecific`,
`TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator`,
`TestOrganizationPolicyInstallNeverUploadsStaleBundle`,
`TestScenarioPlacementPreflightMatchesLauncher`, and
`TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact`.
The Go reservation is released; no production credentials or live configuration
were created.

**Simulator consumer verified (C0-owned after W4 handoff blockage):** W4's
consumer session remained idle awaiting the contract artifact, and both W1's
and C0's artifact notifications were refused by the session messaging limit.
W4 made no source edits. C0 confirmed the reserved Simulator paths unchanged
and adopted the disjoint consumer deliverable rather than waiting for producer
completion. The implementation uses W1's actual exported source types, a
named-operator `organization.read` gate, a configured HTTPS origin and
protected verified mTLS files, a read-only GET, strict response validation,
manual human-supplied organization/target/operation selectors, and an
Organizations-page status form. Missing configuration is unavailable; partial
or invalid configuration fails startup. No browser credential, apply/signing
path, automatic retry/polling or legacy policy-install fallback is introduced.
Focused consumer tests pass, including a private TLS/mTLS fixture, scoped
selectors, status/evidence validation, no-store and body bounds. The combined
W1/W4 status/apply/composition tests passed under `go test -race -count=1`;
`go vet` for the dashboard, Change Review and delegation packages passed.
`gofmt`, `git diff --check` and editor problems checks passed. `make test`
exited 2 with exactly the five historical dashboard failures listed above; no
new failing test group appeared. Browser behavior was not executed at that
earlier checkpoint; the later parent verification below closes that fixture gap.

**Browser milestone verified (2026-10-16):** the parent added
`tests/browser/simulator-policy-status.spec.mjs` and fixed the tightly coupled
Organizations form/script behavior: clear stale evidence on input changes and
page deactivation, ignore delayed obsolete results, reject response identity
mismatches, and avoid success display on HTTP errors. The HTML identifier
patterns escape hyphens for modern validation and the script cache version is
32. The parent's existing VS Code Electron Node runtime ran 34 status cases and
38 prior D2 navigation cases across desktop/tablet: **72 passed**. Its earlier
status run had 20 passes/10 failures before the fixes. Evidence:
`policy-status-browser-after.txt` and `policy-status-browser-before.txt` in the
parent session files. Parent editor problems and `git diff --check` passed.
No Go source changed in that follow-up. This bounded coordinator read the
passing artifact and current file anchors; it did not rerun the suite.
Browser responses are fixtures, not live PS/VS, native identity, admission or
production acceptance.

**W2 D3 implementation blocked before edits:** preflight was completed for
`zpr-visaservice` and clean sibling `zpr-core`; the proposed enrollment/helper
and PH paths were checked, and no source files were changed. Source review
found that the current BAS setup identity owns a runtime private key and the
existing import primitive accepts that private key as input. Merely adding IPC
would export/copy that key and violate the approved endpoint-generated,
System-Keychain-owned identity contract. The existing PH AuthCode/BAS flow also
requires a trusted Authentication actor and validation/2 service; availability
is unproven. W2 stopped rather than weaken the handoff or claim
`credentials_issued`/admission. D5 destructive reset, revoke and re-enroll
semantics remain undecided and out of scope.

**Bounded endpoint seam assessment (2026-10-16):** the W2 discovery report and
the later W2 summary in coordinator state were reviewed; no separate later W2
artifact exists in that coordinator's files. A narrow source check confirms:
`identity_common.go:newSoftwareIdentityRecord` generates and serializes both
private keys; `client.go:NewClientWithRuntimeKey` already accepts an opaque
`crypto.Signer` and exchanges public-key binding/proof; the native vault offers
lookup/signing but imports private bytes via `SecItemImport`;
`runtime_import_darwin.go:importApproved` accepts `*rsa.PrivateKey`, not a
generation request. The existing runtime-import approval binds the runtime
fingerprint after claim/review. It cannot authorize initial generation of a
not-yet-existing key. `runtime_peer_darwin_cgo.go:verifyRuntimeConnection`
provides audit-token/UID/designated-requirement checks, but requires trusted
helper configuration; it is not an installed IPC service.

The concrete non-import seam is an endpoint/helper-owned generate-or-lookup
operation returning only a public key and opaque signer/reference, supplied to
the existing enrollment client **before claim**. Resume must find and prove the
same key, never silently regenerate after uncertain creation or missing state.
Approval then binds that existing fingerprint; PH uses the same protected signer
for BAS. No private key may enter setup records, IPC, manifests or PH config.
**Pre-claim authority approved:** use a locally authenticated,
operator-configured helper policy, not an Enrollment Service generation grant
and not the existing post-review import receipt. Trusted configuration fixes
the caller identity, code requirement, UID, endpoint scope and immutable key
reference. Caller identity must come from native verification, never request
fields. Approval still binds the already-created runtime fingerprint after
claim/review; its binding is not weakened or reused as a pre-claim grant.

**Endpoint producer/client fixture independently verified (2026-10-16):**
`internal/enrollment/endpoint_key_fixture.go` and its adjacent test file add an
unexported, fixture-only producer joining the existing
`NewClientWithRuntimeKey` consumer. Source inspection and a fresh
`go test -race -count=1 ./internal/enrollment` passed, as did scoped vet,
formatting and diff whitespace checks. The fixture covers distinct enrollment/
runtime identity, public-only descriptors and an opaque signer, exact
caller/code-requirement/UID/scope matching, immutable reference/fingerprint
resume, approval fingerprint mismatch, uncertain creation, missing/mismatched
bound keys, and concurrent single creation without replacement. No concrete
coupled source defect was found; no production or native code was changed.

Authorization here is an injected verifier contract checked on each
`RuntimeKey` resolution (including `NewClient`), not OS authentication or
per-signature enforcement on the returned signer. Create-only vault behavior
and atomic durable write-once binding were required adapter contracts in that
original milestone. Its restart test shares in-memory fake stores
between producer instances; it does not prove process/crash durability.
An unbound pre-existing key fails rather than being adopted. An uncertain
create returns no key; retry cannot overwrite a key retained by the vault.
No private-key serialization/import/export path is added by the seam; fake
software keys and a serialization-rejection assertion do not establish native
non-exportability.

**Offline durable endpoint binding adapter implemented (2026-10-16):**
`internal/enrollment/endpoint_binding_store_unix.go` and its exclusive tests
implement an unexported Linux/macOS public-only journal using the existing
enrollment safe-file/ownership patterns, with no new dependency. A minimal
optional reservation seam in the producer durably reserves the trusted
reference before vault creation. Exclusive process locking serializes
`empty → reserved → bound`; the bound fingerprint cannot be replaced.
Explicit first provisioning requires a new dedicated directory; open never
initializes missing state. Owner-only effective-UID ownership, no symlink,
single-link regular file, exact reference and canonical bounded journal
validation are enforced. Trusted ancestors and a local filesystem honoring
sync/locking are deployment requirements, not caller authentication.

An incomplete reservation blocks creation after reopen even when the vault
key is absent. Partial writes fail closed. A complete binding left after an
uncertain sync can recover only after re-sync and exact vault-fingerprint
verification; the failed operation returns no signer. Tests exercise actual
disk reopen and independent reader/reservation processes, concurrent producer
creation, immutable conflicts, missing/mismatched keys, corrupt/unsafe state,
partial append, and injected sync failures. The opaque runtime-signer client
is reused with a software vault retained in memory; no private key is stored
in the journal. These are offline adapter tests, not native Keychain, caller
authentication, real power-loss, hardware-storage or admission proof.
Owner/privileged rollback of valid historical state is outside this local
storage threat model. No deletion/repair/reset/rotation/adoption API is added.

Focused uncached enrollment race tests, scoped vet and formatting passed.
`make test` retains exactly the five previously recorded dashboard failure
groups, without editing them: `TestAssistantSkillsEmbeddedAndTaskSpecific`,
`TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator`,
`TestOrganizationPolicyInstallNeverUploadsStaleBundle`,
`TestScenarioPlacementPreflightMatchesLauncher`, and
`TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact`.
An initial longer workspace-local TMPDIR exceeded macOS Unix-socket path
limits in an existing enrollment test; rerunning with a short workspace-local
TMPDIR restored the focused pass and exact five-failure full-suite baseline.

**Remaining gates:** authenticated native IPC/audit-token and signed-code
verification; native helper ownership/ACL integration of durable storage;
runtime qualification of create-only System Keychain generation, access controls
and opaque signing (the current batch adds source/compile/fake-backed evidence);
PH/AuthCode/BAS use of that same endpoint-owned key; trusted Authentication
actor and validation/2 availability; and separately authorized connected/live
acceptance. No installed helper, Keychain operation, PH wiring, admission,
deployment or production proof is claimed. D5 still gates destructive reset,
revocation and re-enrollment; D6 gates integrated topology acceptance.

**Independent adapter verification complete:** a separate validator reran the
uncached enrollment race tests (30.808s), vet and formatting successfully with
a short trusted workspace-local TMPDIR. An earlier longer TMPDIR exceeded an
existing macOS Unix-socket test's path limit; no test was changed. This evidence
does not qualify native protection or live operations.
The earlier fixture verification is in `migration-endpoint-key-verification.txt`;
adapter files, contracts, validation and limits are persisted in
`migration-endpoint-binding-store-handoff.txt` in the parent session files.
The producer, verifier and binding-store reservations were released before the
current batch; the user-facing coordinator owns the new reservations below.

**Historical W5 receiver-fixture assessment (superseded by execution below):** its report is persisted at
`migration-w5-rust-integration-feasibility.txt` in C0's session files. A
test-only Go-to-Rust HTTPS fixture is feasible with generated temporary keys,
a private loopback listener, a real guarded Rust router and `FakeDb`, but no
existing command runs it and no harness or process was started. It can verify
wire/TLS/proof interoperability, not Redis durability, production startup,
physical network loss or release readiness. Follow-up remains gated on an
explicit test-only implementation assignment. Preserve gaps: implicit rollback
is unavailable; Redis nonce/approval records have no TTL; current Redis test
evidence is Valkey-compatible and application-level response loss only; real
production trust, credentials and network remain unverified.

W1's producer reservation is released. C0 completed the disjoint Simulator consumer
files `policy_status_consumer.go`, its focused tests, the status route and
permission mapping, and the Organizations-page read-only form/scripts. The W1
README section is complete; C0 added the Simulator-specific configuration and
user-workflow documentation after W1 released that file. W4 made no consumer
edits and its prior activation/Restore reservation is released. W2 made no
source edits; its selected PH/helper paths were clean but are not reserved while
the key-ownership blocker is unresolved. W5's feasibility assessment is
complete; it has no source reservation. The W1 contract artifact remains the
frozen source of the exact wire types and fields.

### Completed bounded integration batch (2026-10-16)

The prior coordinator chat was the sole activity surface for this batch. Task workers have
no supported chat links; their tool progress and coordinator status describe
actual work, not archived chat state. No UI hide/archive action or persistent
daemon is claimed. Prior worker identifiers are historical, not active links.

| Assignment | State | Exclusive scope and acceptance |
| --- | --- | --- |
| A: Native endpoint producer | COMPLETE / RELEASED | Non-import native RSA-3072 generation, exact durable-reference lookup and opaque signing source. Fake-backed race, vet, formatting, macOS compile/link and Linux compile passed. No Keychain invocation or helper/PH installation. |
| B: Actual receiver interop | COMPLETE / RELEASED | Actual guarded Rust router and Go policy client over private verified HTTPS using temporary keys and coherent FakeDb; actual apply/readback, authorization, replay/CAS and response-suppression reconciliation passed. Owned child/tasks and scratch cleaned up. |
| Coordinator | VERIFIED / STOPPED | Independent interop execution, combined targeted race tests, vet/format/whitespace checks and safe offline full Go suite completed; shared docs and persistent handoff updated. No active reservations or monitoring loop remains. |
| C: Simulator activation/reset | BLOCKED | D5 remains unresolved; no legacy activation/reset behavior changes or apply authority assigned. |

Reservations were recorded before edits in the current coordinator's
`migration-active-integration-coordinator.txt` session artifact. No overlapping
source writers, nested coordinators or old-session messaging are assigned.
Workers may propose shared documentation but only the coordinator edits it.
No dependencies, production configs, live services, credentials, excluded
discovery artifacts, `zpr-common`, commit, push, tag or deployment are in scope.
The isolated fixture is explicitly authorized to use only its own temporary
loopback listener/children and generated test material. Native helper IPC and
PH remain later gates; caller assertions cannot replace trusted audit/code/UID
verification. Control Room and Control-Service remain Simulator-independent.

**Native source result:** the explicit Keychain backend uses `SecKeyCreatePair`,
requires actual permanent/sensitive/always-sensitive/never-extractable metadata
and `extractable=false`, and exposes only public material and signatures. It
never imports or exports the private key. Unsupported metadata fails closed.
The existing durable pre-create reservation prevents replacement after
uncertainty; missing/replaced bound keys do not regenerate. Existing import
behavior was preserved. Fake tests, native compile/link and Linux compile are
not native runtime or nonexportability proof. Deprecated API compatibility,
approved helper ACLs/ownership, trusted authenticated IPC and PH use of that
same key still need separately bounded work; no request-asserted identity adapter
was introduced.

**Real receiver result:** the Go opt-in test owns a prebound ephemeral loopback
listener inherited by the actual Rust in-binary guarded router. Temporary
CA/server/approval/API keys, verified server-authenticated TLS (not mTLS), valid
policy containers and private PS SQLite state are used. Readiness requires an
actual successful HTTPS current-state request. Apply and independent exact
digest/version readback passed. Missing API key returns 401; insufficient API
key, wrong org/action and bad proof return 403; exact-proof replay and stale CAS
return 409 without changing state. Fresh-proof receiver replay returns the same
receipt. Suppressing one completed success response as HTTP 503 produces
uncertain PS state; reopening the PS store and sending a fresh proof reconciles
without a second version increment. This is not physical packet loss or a Rust
restart. Rust listener/connection tasks and the exact child are closed/reaped;
coordinator residue checks found no fixture directories.

**Combined validation:** the coordinator independently executed the opt-in test
(1.585s package result), then ran both changed Go packages with targeted race
selectors and interop enabled (enrollment 6.026s, dashboard 6.855s). Scoped vet,
Go formatting, Rust fixture formatting and tracked whitespace checks passed.
Independent approval/ledger/authority and eight production operator package
race regressions also passed. The worker ran `cargo test -p vs`: 6 library plus
511 service tests passed, 7 ignored; the opt-in fixture separately executed.
The coordinator's `CGO_ENABLED=0 GOPROXY=off GOSUMDB=off make test` exited 2
with exactly the same five known dashboard failures listed above; all other
packages passed or had no tests. CGo was intentionally disabled because
existing macOS native tests invoke Keychain unconditionally. No existing
failing test was edited; no native runtime coverage is claimed. UI source was
unchanged, so the prior 72-case browser evidence was not rerun.

The current coordinator's persistent `migration-active-integration-coordinator.txt`
records stopped state, worker handoffs, command-output artifacts and released
reservations. The next coherent boundary is trusted helper IPC/code/UID and
ownership/ACL source integration without invoking Keychain, followed only under
separate authorization by native runtime qualification and PH/BAS wiring.
D5 and D6 continue to block only reset/re-enrollment and live acceptance.

C0 owns this tracker and user communication. W1's cross-org fix, synthetic
Policy-Service composition and read-only status producer are complete and
independently verified. W3's D2 navigation and W5's isolated Redis test-only
assignment are also complete, independently verified and released. The
guarded apply path is implemented but conditionally registered only with
protected configuration; no live apply, deployment or mutation is authorized.

### Verified bounded helper/recovery batch (2026-10-16)

The new visible coordinator is the primary user contact. Exact reservations
were established before dispatch and persisted in
`migration-helper-recovery-coordinator.txt` in its session artifacts. Task
workers have no independent chat links; the identifiers below are tool IDs.

| Assignment | Actual worker ID | State | Exclusive scope |
| --- | --- | --- | --- |
| A: Trusted helper source boundary | `042bb2cb-b4f2-4533-8c0a-a61762798bef` | VERIFIED / RELEASED | Trusted kernel-peer adapter and per-signature source boundary; native generation/signing closed pending approved ACL; fake-only combined regression passed |
| B: Durable receiver recovery | `2c6fac34-b271-432c-a418-93b726a44801` | VERIFIED / RELEASED | Actual RedisDb receiver restart, graceful AOF Valkey restart and PS SQLite reopen preserve receipt/CAS/readback; no production receiver/store changes |
| Coordinator | Current visible chat | VERIFIED / STOPPED | Shared plan/README integrated; combined race/vet/format, Rust suite, safe no-CGo full suite and cleanup checks complete |
| C: PH integration | Not dispatched | GATED | Requires a fixed actual helper/signing contract and sibling guidance/status checks; no speculative wire or crypto implementation |

The installed `valkey-server` executable is available for a private ephemeral
loopback persistence fixture only. A restart claim requires actual process
restart with retained operation, receipt and current-state evidence through the
existing receiver/store implementation; reconstructing FakeDb does not qualify.
No production Redis configuration change, install or shared service is allowed.
The earlier six Valkey cases do not themselves prove restart persistence.

Native work is source, compile/link and fake-test only. Existing default CGo
enrollment tests invoke Keychain and must not run in this batch. Trusted caller
identity must come from kernel audit/code/UID verification, never request
fields. Default creator ACLs and fake ACL checks are not deployed enforcement.
Generation remains gated where required protected ownership/ACL checks are
absent; there is no implicit repair, replacement or authority expansion.

Independent offline authority/ledger race regressions passed during worker
execution: `internal/changereview` (1.826s), `internal/operatordelegation`
(1.353s), and `internal/operatorauth` (3.103s). Combined source verification
was performed after both writers released their reservations. A's initial no-edit stop
used a mistyped prior handoff path; the coordinator corrected it and clarified
the approved trusted-config source-only boundary. A resumed the same task with
the same exclusive reservation; concrete native ACL qualification remains gated.
No native runtime,
PH/BAS, admission or live qualification is inferred. D5 and D6 remain unresolved
only for dependent reset/re-enrollment and live acceptance.

**Persistent recovery independently verified:** B's test-only assembly shares
the actual `RedisDb` across repositories and reloads `PolicyMgr` from persisted
state, not reconstructed FakeDb. The coordinator executed both opt-in HTTPS
interop modes under targeted policy/status/production-composition race tests
(7.694s). The actual receiver was killed/reaped and replaced (PID
53381 to 53396); private AOF-backed Valkey was gracefully stopped/reaped and
replaced (PID 53373 to 53395). PS SQLite was separately closed/reopened with
the exact uncertain operation binding retained. Fresh-proof reconciliation
returned the identical committed receipt and unchanged digest/version; a new
stale CAS remained HTTP 409 with no state change. Exact child and scratch
residue checks passed. The original FakeDb scenarios also still pass.

The coordinator's offline `cargo test -p vs` passed 6 library and 511 service
tests, with 7 ignored; both opt-in modes actually executed separately.
Rust/Go fixture formatting and dashboard vet passed. Additional production
Control-Service/Control Room subpackage race regressions passed (8 packages).
The recovery evidence is for a completed receipt, receiver SIGKILL and
**graceful** `appendfsync=always` Valkey AOF restart. It does not qualify database
crash/power loss, pending/commit/receipt crash windows, upstream Redis,
production startup or trust/topology. Server-auth TLS plus API key and reviewer
proof is not mTLS; response suppression is not packet loss. Nonce no-TTL,
separate rollback and native/live qualification gaps remain.

**Helper integration checkpoint:** A added an unexported trusted-config helper
source seam and Darwin Unix-peer adapter reusing the kernel audit/UID/code
verifier. Opaque signing re-verifies the peer for each signature. Native endpoint
generation and signing are now explicitly disabled pending an approved
helper-only Keychain ACL; default creator access cannot enable them. Existing
explicit-import behavior is preserved. No listener/protocol, installation,
deployment values or actual native operation was introduced.

The first combined parent race run caught a new constructor compatibility
failure in `TestNativeEndpointDarwinCompositionValidatesWithoutCallingKeychain`:
strict helper directory checks had been applied to the older non-operating
fixture composition constructor. A corrected only that source seam, keeping
strict helper/runtime validation and the closed generation/sign gate. A new
fake test proves that the helper rejects an unsafe parent while the old
non-operating composition remains compatible. No existing test was modified.
The final independent combined race run passed enrollment (5.611s) and
dashboard (7.413s), with both actual receiver modes enabled. Scoped vet,
Go/Rust formatting and tracked whitespace checks passed. No owned recovery
fixture or helper scratch directory remained.

The safe full `CGO_ENABLED=0 GOPROXY=off GOSUMDB=off make test` retained exactly
the five known dashboard failures, all other packages passing/no tests
(enrollment 11.265s). This intentionally excludes native CGo tests and does not
replace the fake-only Darwin selector required for A's changed source.

**Stopped at verified source/test milestone:** both workers are idle and all
reservations released. A's Darwin compile/link and portable fake tests passed;
the coordinator executed only fake-backed native selectors, never Keychain.
Native endpoint generation/signing are intentionally closed, not runtime
qualified. The next real boundary is approval and source representation of a
helper-only Keychain ACL plus protected helper installation/ownership policy
and a concrete signing transport contract. C PH wiring remains unassigned
until that contract exists and sibling guidance/status are checked. Do not
invent a protocol or treat compile/fake ACL evidence as installed enforcement.
No dependency install, live service/config/credential operation, existing
failing-test edit, commit/push/tag/deploy or excluded-source change was made.
The persistent coordinator artifact is
`migration-helper-recovery-coordinator.txt`; worker handoffs are
`migration-trusted-helper-handoff.txt` and
`migration-durable-recovery-handoff.txt` in the same session artifacts.

### Verified bounded native ACL / PH readiness batch (2026-10-16)

This source-only follow-up preserves the previous verified recovery milestone.
Native runtime authority is not granted by this batch.

| Scope | Worker | Reservation / state |
|---|---|---|
| Concrete native ACL source | `13495443-493e-40c8-bf8a-8d4a1aeea1a5` | Complete / released: concrete exclusive ACL blocked by actual API semantics; native lookup also closed |
| Authenticated signing IPC prerequisite / PH readiness | `462c1449-521c-46a0-9cc0-55e761adef2c` | Verified / released: three new helper IPC files implement frozen v1; PH remains unchanged and blocked |
| Shared contract, documentation and combined tests | Coordinator | Complete; all source reservations released |

Existing helper peer/import mechanisms are read-only for the IPC task. The
native task must represent and inspect actual helper-only access policy rather
than enable default creator access or simply remove the closed native gate.
PH cannot use an invented unauthenticated protocol or fall back to a file key.
Native source/compile and injected fake tests are allowed; actual Keychain,
installed helper, live configuration/credentials and deployment remain excluded.
The combined full Go suite must use `CGO_ENABLED=0` because existing CGo
enrollment tests invoke Keychain. Shared dependencies and known failing tests
remain untouched.

Independent coordinator race regressions passed for eleven operator
authority/Control Room/Control-Service packages while source workers were active.
Rust receiver regressions also passed (6 library, 511 service, 7 ignored), and
both actual receiver interoperability fixtures passed again with child cleanup.

**Material native blocker:** installed `SecAccess` trusted-application lists
describe access without confirmation, not an exclusive helper-only allowlist.
Current trusted configuration also lacks a separate helper executable identity.
A did not fabricate an ACL implementation or enable default creator access.
Generation, lookup and signing all remain closed. Fake race, vet and Darwin
compile/link passed; no Keychain call was made.

**Contract frozen once:** v1 uses connected Unix streams, bounded strict
length-prefixed JSON, reciprocal kernel UID/code verification, exact trusted
reference/SPKI pins, SHA-256 digests and RSA-3072 PKCS#1 v1.5 only. Each signature
requires peer revalidation on both sides and local signature verification.
Configured deadlines are bounded; errors must reach the connection owner.
Public resolution retains the existing immutable producer's durable
reserve-before-create/resume semantics without adding retries or fallback.
This prerequisite owns no socket listener or installation. PH's BAS consumer
still holds a concrete private-key object and has no reviewed Rust helper-peer
transport. No PH/AuthCode/BAS integration or native runtime qualification is
claimed.

**Final independent validation after release:** fake-only enrollment/helper IPC/
producer/journal race selectors passed (6.474s), alongside dashboard policy/
status/production-composition selectors (5.748s). Scoped vet, Go formatting and
Darwin arm64 CGo compile/link passed; compiled binary was not executed and was
removed. Worker IPC race tests also passed five repeated runs. Fixtures cover
strict schemas, reciprocal peer denial, pin/signature validation, cancellation,
concurrent calls, reconnect with unchanged pins and ambiguous-sign response
loss without retry.

The once-only offline `CGO_ENABLED=0 make test` retained exactly the five known
dashboard failure groups: `TestAssistantSkillsEmbeddedAndTaskSpecific`,
`TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator`,
`TestOrganizationPolicyInstallNeverUploadsStaleBundle`,
`TestScenarioPlacementPreflightMatchesLauncher`, and
`TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact`.
Enrollment passed (11.650s); all other packages passed/no tests. No existing
failing test was changed. This deliberately excludes actual native CGo tests.
The sibling PH tree remains clean and helper scratch directories are absent.

**Stopped at verified source milestone / material native blocker:** the Go
per-connection signer prerequisite exists, but there is no installed helper,
qualified exclusive native ACL or Rust PH opaque signer transport. Next work
must establish a real exclusive OS key-use boundary and separately trusted
helper identity before enabling native operations. PH needs its own reviewed
kernel-peer transport plus an explicitly configured fallible opaque BAS signer
with endpoint reference/SPKI binding and no file-key fallback. Source/fake
evidence cannot substitute for that qualification. No live Keychain, install,
configuration/credential operation, dependency install, excluded-source edit,
commit/push/tag/deploy or persistent daemon occurred.
Persistent final evidence: `migration-acl-ph-coordinator.txt`,
`migration-native-acl-handoff.txt`, `migration-helper-ipc-ph-handoff.txt` and
the frozen `migration-helper-ipc-v1-contract.txt` in session `91b84c37`.

## 11. Completion and User Handoff

C0 can recommend the migration complete only when:

- [ ] Production and companion artifacts have an explicit compatibility record.
- [ ] Production editor and automation paths use the agreed supported contracts.
- [ ] Simulated adapters enroll normally with distinct protected identities.
- [ ] No active new-path setup bypass writes production files/databases or
  silently invokes legacy compile/install/credential shortcuts.
- [ ] Real traffic proves the exact applied policy, including denial evidence.
- [ ] Production and independently supervised endpoints/providers work with
  Simulator stopped; reset/rebuild boundaries are tested.
- [ ] Operation reconciliation, rollback, restart, revocation and renewal have
  recorded acceptance evidence.
- [ ] Remaining test failures and release limitations have explicit disposition.
- [ ] User receives one integrated report: what changed, exact versions,
  verified behavior, unresolved limits and requested release/deployment decision.

Related sources:
[priorities](TOP_10_PRIORITIES.md),
[GUI backlog](GUI%20work.md),
[organization workflow](Organization%20editor%20plan.md),
[provisioning](../Provisioning%20plan.md),
[team charter](../AGENT_TEAM_STARTER.md), and
[implementation progress](../zpr-dev-context/docs/IMPLEMENTATION_PROGRESS.md).
