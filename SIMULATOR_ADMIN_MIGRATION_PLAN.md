# Simulator Administrator Workflow Migration

Date: 2026-10-16<br>
Prior milestone status: Stopped at a verified bounded helper/recovery
milestone. Both workers completed and released their source reservations. The
coordinator independently verified the combined fake-only helper/native
selectors and both actual HTTPS
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
| Lab LDAP/OIDC lifecycle | Independently supervised companion services; bootstrap DNS uses existing network DNS and protected ZPR DNS is production-owned |

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
| D5 | Approved 2026-10-16: reset clears scenario state only and retains endpoint identity; revoke/re-enroll is a separate action requiring named human approval. Reset remains scoped to the explicitly configured disposable lab organization and requires named approval under D1. | W2 + W4, C0 | Any identity-changing reset/re-enrollment migration; live acceptance remains gated by D6 |
| D6 | Approved 2026-10-16 by C0 with user delegation: first acceptance is one isolated disposable Linux lab stack and one separate Apple Silicon Mac running macOS 13+ with a single PH/AuthCode/BAS adapter. Include the required Authentication actor/`validation/2` and one configured trusted attribute provider, each operated independently of Simulator. Prove production operator services remain available with Simulator stopped. No Great Lakes/production target. | W5, C0 | Selection complete; qualification still requires D1/D3 prerequisites and separate execution approval |

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

For the D5 scenario-only reset, the user selected an authenticated named human
session with explicit organization binding and per-reset confirmation. Before
W4 changes the handler, W1/Control-Service must verify that an existing
supported identity/authorization contract can enforce this. Do not share
Control Room tokens or sessions with Simulator. Until that source check passes,
the reset handler remains unchanged.

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

### D5 decision record (2026-10-16)

**Approved reset boundary:** reset may clear Simulator scenario state only and
must retain the endpoint's existing identity. It must not implicitly revoke,
re-enroll, erase enrollment/repository state, or restart production services.
Revocation and re-enrollment are separate actions requiring named human
approval. Reset remains restricted to the explicitly configured disposable
lab organization and requires named human approval under D1. This selects
source/test behavior only; it does not authorize service mutation or live
execution.

### D6 decision record (2026-10-16)

**Selected first acceptance topology:** one isolated disposable Linux lab
stack for the Visa Service, Policy-Service, Control-Service/Control Room and
Simulator, plus one separate Apple Silicon Mac running macOS 13 or newer with
one PH/AuthCode/BAS adapter. The lab must include the Authentication actor
and its advertised `validation/2` service, plus one configured trusted
attribute provider. The provider and endpoint lifecycles must be independently
supervised and remain usable without Simulator; acceptance must verify
production operator services while Simulator is stopped. Use only the
explicitly configured disposable lab organization, not Great Lakes or another
production target. This selects scope only: it does not approve deployment,
live changes, credential use, native Keychain operations or privileged host
changes. This matches the existing local Linux one-node test runtime and the
provisioning plan's first-release Mac target; broader fleet and multi-node
acceptance are deferred. D1 authority and D3 native/helper qualification gates
remain in force.

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

## 12. Coordinator Continuation (2026-10-16)

**Current status:** C0 has resumed coordination with five visible specialist
chats. W1 and W5 completed two disjoint test-only changes and released their
reservations. W1's read-only D1 check concluded that the existing reset path
is insufficient. W2's repository/owner check concluded blocked; W3/W4
completed blocked recovery. Initial recovery has concluded; W4 completed the
user-requested read-only durable audit/approval contract addendum and stopped
pending review.
The subsequent generic continuation does not approve reset implementation.
W4's corrected store reservation is released and independently verified.
The next server-only integration reservation below is active. The
Visa Service worktree is `main` ahead of `origin/main` by one commit, with
pre-existing untracked/modified discovery-example files that are outside this
work; they must be preserved. No production/configuration mutation, privileged
host operation, Keychain operation, deployment or publication is authorized.

| Chat | Workstream | Current state | Chat |
|---|---|---|---|
| W1 | Production Configuration & Apply | Verified: auth matrix passed worker and C0 focused/race checks; single-file reservation released | [Open W1](agent-host-session://ahp-session/9f3ba312-b177-4a44-9bde-a87fb262392b?chat=48fe60db-adca-4e2b-b528-9698c88e7356) |
| W2 | Endpoint Provisioning & Trusted Helper | Blocked: PH adapter location documented, but authoritative AuthCode/BAS client files and named owner unestablished; no source reservation | [Open W2](agent-host-session://ahp-session/9f3ba312-b177-4a44-9bde-a87fb262392b?chat=c87009be-310e-41c6-8262-87142a456b7a) |
| W3 | Control Room Editors & Operator UX | Verified: corrected Simulator reset UI passed C0 90/90 repeated desktop/tablet fixtures; reservation released; D2 editor scope remains blocked | [Open W3](agent-host-session://ahp-session/9f3ba312-b177-4a44-9bde-a87fb262392b?chat=e3463d35-38ea-462b-b50e-aab6cb368d08) |
| W4 | Simulator Orchestration | Verified source/test milestone: server reservation released; C0 backend/auth/store checks passed; UI/live qualification pending | [Open W4](agent-host-session://ahp-session/9f3ba312-b177-4a44-9bde-a87fb262392b?chat=a595039b-c2fd-44ff-81d5-73dca1000a03) |
| W5 | Packaging & Acceptance Evidence | Verified: HTTP handler restart acceptance plus combined C0 reset tests/race/vet passed; reservation released | [Open W5](agent-host-session://ahp-session/9f3ba312-b177-4a44-9bde-a87fb262392b?chat=44243e25-2e51-4e13-91e7-df02352f36cb) |

**Verified W1 test:** `TestPolicyStatusRejectsPinnedVerifiedCertificateWithoutClientAuth`
proves a pinned, chain-verified serverAuth-only certificate receives 403 before
any upstream status lookup. W1's first attempt overlapped W5's incomplete Go
test file and did not reach execution; after W5 completed, C0's combined
focused test passed. This first W1 reservation was released before the separate
D1 authorization-contract check.

**Verified W5 fixtures:** `vs/src/policy_recovery_fixture.rs` seeds actual
RedisDb-backed pending-before-commit and committed-with-pending-receipt states;
`zpr-dashboard/cmd/zpr-web-dashboard/policy_apply_crash_window_interop_test.go`
restarts an owned Visa Service receiver and asserts binding, exact-once
readback/replay, and stale-CAS rejection. The fixture uses an owned private
loopback Valkey with a dynamic port and temporary AOF directory. Both source
reservations were released. This is synthetic intermediate-state recovery,
not power-loss or upstream Redis qualification.

**Combined verification:** With `CGO_ENABLED=0`, the focused Go command
`go test ./cmd/zpr-web-dashboard -run '^(TestPolicyStatusRejectsPinnedVerifiedCertificateWithoutClientAuth|TestPolicyApplyRustCrashWindowRecoveryInterop)$' -count=1 -v`
passed (both crash-window subtests and the ClientAuth rejection test). Offline
`cargo test -p vs` passed 6 library tests and 511 service tests; 8 were ignored,
including the Go-owned fixture which the Go integration selector executed
separately. The new Rust fixture target also compiled via `cargo test --no-run`;
Go/Rust formatting and `git diff --check` passed. The same two Go selectors
passed with `-race` under `CGO_ENABLED=1`; no enrollment/Keychain test was
selected or run.

After both Go runs, the exact owned receiver PIDs were absent and no
`zpr-policy-crash-window-*` temp directories remained.

**Open boundaries:** Native generation/lookup/signing remain closed until an
actual exclusive helper-only key-use ACL, protected helper identity and
installation policy are approved and represented. PH/BAS wiring remains
blocked until its reviewed authenticated opaque-signer transport is ready.
D5 is resolved: scenario reset retains endpoint identity, while
revoke/re-enroll remains a separate named-human-approved action under D1.
D1's approval requirements still apply. D6's initial topology is selected,
but integrated/live acceptance is not authorized or qualified until D1/D3
prerequisites are met and execution is separately approved. No operation
against a production or lab service is inferred from source/test work.

**W3 scope gate:** the next editor acceptance slice needs explicit production
organization/target context. D2 forbids Simulator from selecting or handing off
that context, but permits independent selection and confirmation inside
independently authenticated Control Room. W3's read-only report identifies
that D2-compatible route; a D2 change is needed only if Simulator context
handoff is required. No UI/source change or browser test was authorized.
Before reserving implementation, bound one production-owned authoring/revision
fixture slice and its ownership. Keep draft validation distinct from scenarios,
retain optimistic conflicts/drafts and stale-response guards, and test with
Simulator unavailable. The full apply/readback/scenario roundtrip remains
subject to the separate approval and execution gates.

**W2 blocker:** the frozen Go helper IPC v1 is only a transport prerequisite;
W2 found no PH consumer path in the inspected isolated Visa Service tree.
Its follow-up found `../zpr-core/README.md:53` documents the Rust `ph` adapter
under `adapter/ph`; `../Provisioning plan.md:30` describes the connected,
policy-authorized Authentication actor prerequisite and line 39 records a
local PH AuthCode client prerequisite. These references do not establish that
the adapter tree implements or owns that client. W2 found no `AuthCode`,
`auth.zpr`, `validation/2` or `BAS` references in that tree and no documented
named PH/AuthCode/BAS owner or exact client/test file pair. The inspected Core
checkout was on `work/macos-tun-address`, tracking
`personal/work/macos-tun-address`; it was not edited or tested.
Before PH fake-stream implementation, C0 must establish the authoritative PH
repository/files and obtain PH/BAS owner confirmation of the v1 mapping. No
native ACL, helper installation, Keychain operation or PH integration is
reserved.

**W4 blocker:** D5 permits only scenario-state reset while retaining endpoint
identity. For D1, the user selected named-session confirmation with explicit
organization binding and per-reset confirmation, without token/session
sharing. W1's completed read-only check found no sufficient existing end-to-end
contract. Simulator has independent named OIDC sessions (`operator_login.go:55`)
and session/CSRF/org/permission authorization (`internal/operatorauth/auth.go:474`).
However, `simulator_policy.go:127-128,150-204` authorizes cancel with an implicit
manifest organization. Clear uses the same generic permission and implicit
organization. Cancel only stops an active run (`scenario.go:1664-1671`); clear
only clears inactive state. Neither establishes distinct per-reset human
confirmation; CSRF is not that confirmation. Control Room authorization is
independent, its proxy strips credentials/CSRF, and it exposes no Simulator
route. Control-Service route composition leaves caller authentication to its
owner and supplies no missing Simulator approval contract. Existing sessions
are reusable building blocks, not approval to invent or enable a new reset
contract. W4 proposed only local scenario-state changes in `scenario.go`,
`simulator_policy.go` and their tests; no source reservation was granted.

**W4 reset contract proposal (unapproved):** the user authorized contract
drafting only. W4 completed the read-only proposal and stopped without edits,
tests or processes. Proposed action: an explicit-org POST reset route, exact
`scenario.reset` permission, independently authenticated Simulator OIDC session,
same-origin CSRF, and explicit human confirmation binding configured disposable
lab org and expected run generation. Immutable actor is verified issuer/subject,
not caller-supplied identity or display name. Under one manager lock, terminal
scenario state clears and generation increments; active/cleaning runs conflict.
No definitions, endpoint identity, machine/controller/session state, service
clients, activation scripts or production state may be changed.

The proposal uses an operation ID and actor/org/generation binding for exact
in-process retries, conflicts on stale generation or distinct concurrent resets,
and makes no durable receipt claim after process restart. Proposed audit fields
include actor, action, org, operation, expected/result generation, timestamp and
outcome. No audit sink, retention policy, durable approval or idempotency store
has been established. User review must choose volatile/process-local evidence
with explicitly accepted restart limitations or an approved durable audit/
approval contract before implementation. Proposed denials never mutate state;
unavailable authorization/audit prerequisites fail closed.

C0 review also requires resolving the existing `/clear` entry point: a new
guarded reset route must not leave an equivalent generic-permission bypass.
Preserve cancel as a distinct active-run action. Specify confirmation expiry,
bounded dedup retention, same-ID/different-payload conflicts, and audit/mutation
failure ordering before calling the contract complete. Existing OIDC/CSRF
helpers do not provide these additions automatically.

Candidate source/test scope remains `simulator_policy.go`,
`simulator_policy_test.go`, `scenario.go`, and `scenario_test.go`; permission
registration, startup/config validation, route registration and UI callers must
also be mapped before an exact reservation can be complete. UI confirmation
and its browser coverage are not implicitly reserved. Only explicit offline
test selectors may run after approval; no unrestricted package/CGo enrollment
tests. No implementation or native/live authorization is inferred.

**Reset audit decision:** the user selected durable audit/approval evidence;
continue contract design only. Process-local-only approval/audit evidence is
not accepted. No storage, owner, protocol or runtime enablement is approved by
this choice.

**W4 durable addendum (proposed, not approved):** no existing generic Simulator
audit sink was found. Control Room's `internal/changereview.Store` and
Policy-Service's policy-operation tables are private, purpose-specific contracts,
not available for direct Simulator database access. W4 recommends a dedicated
Simulator-owned private SQLite store using the existing driver, one writer,
with explicit configuration and protected file/parent ownership. This is local
durable evidence, not independent or tamper-evident evidence. An independent
operator-service approval/audit API is an alternative requiring a new contract;
it must not introduce an operator-service dependency on Simulator.

Proposed workflow: explicit-org reset intent and named confirmation, two-minute
expiry, immutable issuer/subject binding, durable operation ID, durable startup
epoch and run generation. Persist consumption/applying before volatile clear;
persist terminal outcome afterward. There is no transaction spanning SQLite
and in-memory scenario state. Startup reconciles old-epoch unfinished operations
as interrupted/indeterminate, never resumes mutation or invents success.
Audit unavailability prevents mutation; final-write failure yields unknown/
unavailable outcome requiring status reconciliation. Same-ID changed binding
conflicts; exact retries read stored status without another clear. Prior epochs
and stale generations cannot mutate state. Detailed transaction/lock ordering,
clock/expiry behavior and status authorization require implementation review.

Proposed retention: 365 days for operation/audit evidence, seven days for
expired-unconfirmed intent detail, permanent epoch/replay protection. User must
approve or replace this policy, including legal hold/backup requirements.
Any intent-detail purge must preserve required audit evidence for its full
retention period; append-only within retention is not indefinite retention.
Legacy `/clear` should return 410 without mutation, even with auth disabled;
cancel remains separate. A temporary alias is an alternative only if it uses
the entire explicit target/approval guard.

Affected surfaces exceed the initial four-file candidate: permission registry
`internal/operatorauth/auth.go`; reset routes and scope tests; `simulator.go`
startup/config/store lifecycle; dedicated store and transaction tests; scenario
manager/handlers/tests; `static/scenarios.js` and `static/scenarios.html`,
matching browser tests; dashboard command documentation/config guidance.
No grants, deployment mount, service configuration or new dependency is approved.
W4 ran no tests/processes and made no edits.

**User contract decisions (2026-10-16):** approved dedicated Simulator-owned
SQLite evidence (not independently tamper-evident); two-step same-actor intent/
confirm, two-minute expiry, epoch/generation binding, exact retries and
indeterminate restart semantics; 365-day operation/audit retention and seven-day
expired-intent detail purge preserving audit, with permanent replay protection;
legacy clear permanently returns 410 without mutation. These decisions
supersede the corresponding unapproved options above, not the live-operation
gates. Backup/legal-hold/deployment policy remains a release prerequisite.

**Exclusive W4 reservation:** only new
`zpr-dashboard/internal/simulatorreset/store.go` and `store_test.go`, implementing
the approved durable-store primitives and offline SQLite tests using the existing
driver. No route, UI, permission, grant, startup/config or service wiring yet.
No other worker may edit these files. W4 reports actual tests and releases the
reservation before C0 integration verification. C0 alone edits shared trackers.

**First store review/verification:** W4 released the initial increment; C0
independently passed focused no-CGo tests, focused CGo race tests (only this
package, no enrollment) and no-CGo vet. The store remains unwired.
C0 requested corrections within the same exclusive two-file reservation:
align `scenario.state.reset` to the approved `scenario.reset` action; verify
filesystem ownership, hardlinks, ancestor/lock/SQLite sidecar safety rather than
claim owner protection from mode checks alone; add concurrent exactly-one
consumption and failed-final-audit/reopen-indeterminate tests. W4 must stop for
scope approval if more files/platform separation is required. Linux/macOS-only
`Flock` support is an explicit integration limitation, not portable qualification.

C0 ran `CGO_ENABLED=0 GOPROXY=off GOSUMDB=off make test` in the dashboard module:
exit 2 with exactly the five documented pre-existing dashboard failure groups
(`TestAssistantSkillsEmbeddedAndTaskSpecific`,
`TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator`,
`TestOrganizationPolicyInstallNeverUploadsStaleBundle`,
`TestScenarioPlacementPreflightMatchesLauncher`,
`TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact`).
The new store package passed; enrollment passed. No failing test was changed.
An initial invocation at workspace root had no `test` target and was corrected
to the module directory. The complete module run is in C0's session artifact
`files/reset-store-make-test.log`, not a new repository tracking file.

**Corrected store acceptance:** W4 released both files after corrections.
C0 directly reviewed the corrected path/action implementation and added tests;
independent no-CGo package tests, focused package race tests, vet and formatting
passed. Exact action is now `scenario.reset`; Linux/macOS build constraints and
no-follow directory-relative file opens, UID/mode/link/identity checks and
SQLite sidecar checks are present. Concurrent confirmation consumes exactly once;
failed final audit rolls back and reopen reconciles as indeterminate.
This is local durable evidence, not defense against a compromised same-user
process, native qualification or atomic memory/SQLite commit.
C0 reran the safe full Go baseline after corrections: the same five known
dashboard groups failed; store and enrollment passed. Log:
`files/reset-store-corrected-make-test.log` in C0 session artifacts.

**Next exclusive W4 reservation (server-only):**
`zpr-dashboard/internal/operatorauth/auth.go` and `auth_test.go`;
`zpr-dashboard/cmd/zpr-web-dashboard/simulator.go`,
`simulator_policy.go`, `simulator_policy_test.go`, `scenario.go`,
`scenario_test.go`, new `simulator_reset.go`, new `simulator_reset_test.go`,
and `cmd/zpr-web-dashboard/README.md`. The store files are released and not
reserved for further changes. Stop for additional paths if required.
Implement opt-in explicit disposable-org/private-store configuration with
startup validation/closure, exact reset permission and explicit-org intent/
confirm/status APIs, verified identity/CSRF enforcement, manager epoch/run
generation and safe mutation/outcome ordering, and legacy clear 410 without
mutation (including auth-disabled mode). Preserve cancel as a separate action.
No implicit permission grants or default store/org; disabled/missing auth/store
must never permit reset. This is offline source/test wiring only, not permission
to configure or invoke any running service.

**Parallel work activation:** user requested existing workers advance disjoint
work while W4 implements. W4 retains all server reservations and must confirm
exact routes/bodies/responses/config/identity interface once for consumers.
W1 prepares a bounded auth acceptance matrix and helper mapping, read-only;
proposed new `cmd/zpr-web-dashboard/simulator_reset_authorization_test.go`
is not reserved until that interface is confirmed.
W3 prepares the Simulator-only confirmation state machine/caller and browser
file scope, read-only pending the same interface. This is not the blocked D2
production-editor handoff. W2 remains blocked; no native/PH workaround.

**Released W5 reservation:** only new
`zpr-dashboard/internal/simulatorreset/store_acceptance_test.go`, external test
package using the accepted public API. Implement owned subprocess restart/
reopen evidence for pending/consumed operations without graceful store closure,
exact retry and changed-binding rejection. Use private temporary SQLite and
bounded owned/reaped test children; no service/listener/shared resource.
Run only explicit `TestResetStoreAcceptance` selectors offline, including race.
This is process-restart evidence, not power-loss/database-crash qualification.
Store implementation and W4 server files remain outside W5 ownership.

**W5 acceptance verified:** W5 completed and released the file; C0 read it and
independently passed all store unit plus acceptance tests without CGo, all store
tests under race, vet and formatting. Three owned subprocess modes cover pending,
consumed and 24 concurrent confirmations before `os.Exit` without `Close`, then
reopen validates epoch advancement, interrupted/indeterminate state, immutable
exact retries, changed-binding conflicts and system-attributed reconciliation.
Each child is waited/reaped, with exact PID kill/wait on timeout and private
temporary storage. This is application subprocess exit/reopen, not SIGKILL,
kernel/SQLite crash, power loss or production acceptance. Store API/backend
integration remains separately owned by W4.

**Frozen server interface:** C0 accepted W4's source-only wire contract.
Configuration is explicit `SIMULATOR_RESET_ORGANIZATION` plus
`SIMULATOR_RESET_DB_PATH`; both absent disables reset, partial configuration
or configured reset without named auth fails startup. No defaults or grants.
All routes under `/api/simulator/organizations/{organization}/scenario-reset`
require exact `scenario.reset`, explicit org and verified identity:
GET `/status` supplies org/epoch/generation/run_state;
POST `/intents` accepts operation_id/org/epoch/generation (201 new, 200 retry);
POST `/operations/{operation_id}/confirm` accepts org/epoch/generation
(200 completion/retry, 410 expiry, 409 active/stale/conflict, 503 unknown outcome);
GET `/operations/{operation_id}` returns evidence for original actor/org only
(404 unknown, 410 purged). Mutations use existing same-origin CSRF.
Wire fields use `organization`, not abbreviated `org`, and `operation_id`.
No actor is accepted from client headers/body. Legacy clear always returns 410.
For the exact retired POST `/api/simulator/scenarios/clear`, return the
non-mutating 410 tombstone before proxy authorization/organization resolution,
and independently in the direct handler. It must not call the authorizer,
manifest resolver, store or manager. C0 rejected a proposed empty-org
`scenario.reset` authorization exception: no such grant or relaxed test fixture
is needed for a route that cannot execute. New reset routes remain fully
authenticated and explicitly org-scoped; cancel retains its separate behavior.
Manager starts at nonzero generation and advances on each run replacement/start/
reset, guarding cross-org and stale approval. Detailed status evidence preserves
honest consumed/indeterminate state, never authorizing repeat mutation.

**Extended W4 reservation:** C0 explicitly adds only
`cmd/zpr-web-dashboard/simulator_reset_unsupported.go` for unsupported-platform
fail-closed/build-compatible behavior; no Unix-only store import may silently
break existing other-platform builds. Existing server reservation otherwise
unchanged. No platform runtime qualification is claimed.

**Exclusive W1 reservation:** only new
`cmd/zpr-web-dashboard/simulator_reset_authorization_test.go`, implementing its
matrix against actual handlers/proxy/store and verified fake identity. Prefix
`TestSimulatorResetAuthorization`; explicit offline/race selectors only.
W4 supplies exact test constructor seam; W1 does not edit backend files.

**Exclusive W3 reservation:** only `cmd/zpr-web-dashboard/static/scenarios.js`,
`static/scenarios.html`, and new
`tests/browser/simulator-reset-confirmation.spec.mjs`. Implement explicit
org/run/named-actor confirmation, exact operation retry/reconciliation, expiry/
unknown/conflict UX and desktop/tablet fixtures. Do not call legacy clear,
auto-confirm, transfer context to Control Room or repeat D2 navigation tests.
Coordinate owned browser fixture resources before starting; no live operations.

**W3 runner decision:** local Node and Playwright shebang execution failed with
exit 127; no installation was attempted. C0 found the matching cached
`mcr.microsoft.com/playwright:v1.63.0-noble` image. The user approved a temporary
offline container only: `--pull=never`, `--network=none`, no host ports/socket/
credentials, read-only dashboard mount, existing node_modules, temporary output,
owned exact-name cleanup. JS syntax and the new reset browser fixture only;
no npm install, container-script dependency restore, deployment or live service
access. W3 may use an owned fixture listener inside that isolated container.
Browser results remain pending; container availability is not acceptance.

**W3 first independent browser result:** W3 reported 28/28 and released its
three files; C0 reran the approved offline cached container: JS syntax passed,
27/28 browser cases passed, but tablet success confirmation lost its dialog
before showing completion (spec line 239). The container exited and was removed.
Review identifies unconditional run-signature invalidation during the reset's
own successful refresh as a likely race. C0 renewed only W3's same three-file
reservation for a root-cause fix and deterministic post-success-refresh
regression, retaining unrelated run/org/session stale-intent protection.
Repeated desktop/tablet verification is required; UI is not yet accepted.

**Corrected W3 acceptance:** W3 fixed the reset's own completion-refresh race
within its three files and released the reservation. The narrow in-flight
confirmation guard preserves the operation while its response is pending;
expected subsequent idle transition requires same org and operation epoch,
current generation exactly original+1 and no scenario ID. Pending intents and
unrelated org/session/run changes remain invalidated. A deterministic fixture
holds the confirm response until post-reset run refresh occurs, then checks
visible completion and exactly one confirm.
C0 reviewed the guard/regression and independently reran the approved isolated
cached browser container: JS syntax passed; 90/90 cases passed (15 cases,
three repetitions, desktop and tablet). Exact container cleanup was verified.
Log: `files/reset-ui-corrected-browser.log` in C0 session artifacts.

The final safe offline no-CGo `make test` after UI correction still has exactly
the five known dashboard failure groups, with enrollment/operatorauth/
simulatorreset passing. Log: `files/reset-integrated-final-make-test.log`.
All tracked whitespace checks passed; pre-existing discovery changes remain.
No failing test was edited to hide a result.

**Integrated milestone/handoff:** approved scenario-only durable reset now has
source wiring, explicit named-session/org permission/CSRF guards, legacy clear
tombstone, confirmed UI and independently verified offline store/auth/HTTP/
browser fixtures. This is NOT deployment, publication, real OIDC browser-to-
backend acceptance, live lab reset, database power-loss or native qualification.
The new browser suite mocks HTTP; the backend fixtures use synthetic verified
identity and HTTP recorders. Existing legacy-clear browser assertions were not
changed or rerun; deliberate migration of those previously passing assertions
and a single real-browser-to-owned-backend fixture remain next acceptance work,
with exact file/resource reservations required before writes.
No permission grants, running-service config, credentials or endpoint identities
were changed. All W1/W3/W4/W5 reservations are released; W2 remains blocked on
PH/AuthCode/BAS ownership and native prerequisites.

**Next action:** integrate the disjoint backend/auth-test/UI changes after each
reservation is released, run combined safe selectors and baseline, and validate
fixture-only desktop/tablet and HTTP restart acceptance. No new worker or
repeated discovery is needed.

**Backend integration verification:** W4 released all server/auth/docs/stub
files. C0 reviewed configuration/route composition, handler binding and durable
consumption/outcome ordering, manager generation and retired-route protection.
Combined explicit `TestSimulatorReset`, policy-map/proxy and manager-clear
selectors passed without CGo and under race; all operatorauth and simulatorreset
package tests passed without CGo. The full safe offline `make test` again has
exactly the same five documented dashboard failure groups; enrollment,
operatorauth and simulatorreset passed. Evidence log:
`files/reset-backend-make-test.log` in C0 session artifacts.
Windows cross-build is not qualified: W4 reports pre-existing unreserved
`internal/assistant/key.go` syscall errors; the new unsupported-platform stub
does not import the Unix-only store.

W1's auth matrix passed C0's combined selector and W1's exact offline no-CGo/
race selectors; formatting passed and its single-file reservation is released.
W3 retains UI reservation to fix three concrete
wire mismatches: `intent` not `pending`, 32 lowercase-hex operation IDs not
hyphenated UUIDs, and HTTP 200 `expired` operation status. Required browser
fixtures must assert those exact shapes. Browser acceptance remains pending.

**Exclusive W5 HTTP acceptance reservation:** only new
`cmd/zpr-web-dashboard/simulator_reset_restart_test.go`, Linux/macOS.
Use actual handlers/proxy, verified synthetic identity and private SQLite with
manager/store reopen to check seeded pending/consumed and completed receipt
reconciliation, actor/binding preservation and no repeated mutation across epoch
change. Explicit `TestSimulatorResetRestartAcceptance` selectors only, offline
and race; no listener or runtime service. This is seeded boundary/graceful
reopen evidence, not a process crash or live acceptance claim.

**W5 HTTP acceptance released/verified:** three cases seed consumed-before-
outcome, pending intent and completed outcome through the public store API,
close/reopen, then exercise actual handler/proxy via HTTP recorders. They assert
indeterminate/interrupted/completed evidence, exact retry without mutation of
a distinct retained terminal manager state, unchanged audit, changed-binding
409 and new stale-epoch operation rejection. C0 read the fixture and passed
all `TestSimulatorReset` tests without CGo and under race, plus vet for dashboard,
operatorauth and simulatorreset. W5's own exact selectors passed; reservation
released. This is internal `package main` integration coverage, not an external
Go test package. Seeded boundaries and orderly Close/reopen do not establish
API-server crash, power-loss, real OIDC login or live acceptance.
Also confirm the authoritative
PH/BAS repository and v1 owner mapping before PH fake-stream work; ask for a
separate D2 decision only if production organization/target handoff is
required. For W3, bound the independent Control Room organization-selection/
authoring fixture slice before granting any source/browser reservation; do not
repeat the completed 72-case navigation milestone. Native ACL/installation,
PH wiring and live acceptance remain
blocked. The next acceptance reservations below supersede the released milestone.
This is chat-based coordination,
not a daemon or continuous-monitoring guarantee.

### Next offline integration acceptance (2026-10-16 continuation)

The current ledger distinguishes verified mocked browser fixtures, real
handler/proxy/SQLite recorder tests and store subprocess restart evidence.
No browser has yet exercised the real reset routes/store end to end.
Production/Control Room independence remains unchanged; no reverse dependency
is added. The next increment closes that specific gap rather than repeating
completed discovery or claiming real OIDC/live qualification.

Exclusive reservations:

| Worker | Exact files | Deliverable |
|---|---|---|
| W4 | New `cmd/zpr-web-dashboard/simulator_reset_browser_fixture_test.go` | Owned loopback Go test fixture serving embedded UI and real reset proxy/handlers/store, synthetic named auth and private temp SQLite; compile before execution |
| W5 | New `tests/browser/simulator-reset-live-fixture.spec.mjs` | Actual browser-to-fixture HTTP acceptance, no reset-response mocks; owned subprocess lifecycle, completion/retry/denial/reopen evidence |
| W3 | `tests/browser/scenario-browser.mjs`, `tests/browser/dashboard.spec.mjs` | Deliberate migration of directly affected previously passing legacy-clear assertions, preserving cancel and unrelated coverage |

W4/W5 must synchronize exact test-process protocol before coupling code; no
production route/helper changes are reserved. W1 remains verified/unreserved;
W2 remains blocked. No new workers. Known failing tests must not be altered.
C0 owns combined verification, shared tracking and directly related handoff.

The user approved extending the cached offline Playwright runner to these
fixtures/selectors: compile the Go fixture with no CGo/offline modules, mount
the test binary read-only, start only an owned ephemeral loopback test backend
inside the network-disabled container. Source read-only, state/output temporary,
no host ports/socket/credentials/installs, bounded/reaped children and exact
container cleanup. This is not a lab/production service or live reset.

Next action: receive the synchronized fixture protocol and released files,
compile and run the targeted actual-HTTP/browser plus migrated regression cases,
review results and reconcile the ledger. Do not label mocked or synthetic-auth
evidence as real OIDC, live deployment, abrupt database crash or power loss.
No commit/push/tag/publication, native Keychain work or PH ownership decision
is authorized by continuation.

### Parent-coordinated browser acceptance (2026-10-16)

Cross-chat messages are blocked by the reported 100-message service limit,
including in the replacement supervisor. Reading worker results still works.
The old supervisor's last turn and both fixture workers' latest turns were
complete when the parent continued verification; no worker message or explicit
reservation-release acknowledgement was delivered. The user explicitly
authorized the parent to take over W5's browser spec for harness corrections
and verification. No Go fixture or production source was changed.

The first actual run rejected the cookie because Playwright accepts a cookie
URL or domain/path, not URL and path together. The spec now uses URL only.
The next run exposed an incorrect post-reopen visibility expectation: the
fixture recreates an idle scenario manager, so the reset button must be hidden,
not visible. The spec now asserts both idle status and the hidden button while
retaining the durable interrupted-operation and no-confirmation-replay checks.

Independent parent verification passed all **6/6** cases, covering three cases
on desktop and tablet: cancelled confirmation with interrupted evidence after
reopen, completion with exact idempotent HTTP retry, and missing identity/CSRF
plus legacy-clear denial without mutation. The Linux arm64 fixture binary was
compiled using `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOPROXY=off GOSUMDB=off
go test -c ./cmd/zpr-web-dashboard`. The cached Playwright 1.63.0 container ran
only `simulator-reset-live-fixture.spec.mjs` with one worker, no external
network, read-only source and no published ports. The container was removed
and tracked whitespace validation passed.

This closes the real embedded-browser/reset-route/private-SQLite gap for
these six checks. Identity/session endpoints remain synthetic; real client
login/session/CSRF fetch code runs, but real OIDC is not qualified. Restart
means orderly store close/reopen with manager recreation inside the fixture,
not listener/process crash, database power loss, or live service recovery.
The migrated legacy regression selectors and final combined acceptance ledger
still require reconciliation before declaring the whole source milestone done.
No deployment, live reset, publication, native qualification or full migration
completion is claimed.

### Fresh-chat checkpoint handoff (2026-10-16)

The user requested a local checkpoint commit and retirement of the old chats.
This is an engineering checkpoint, not full acceptance or a deployment.
Discovery example port-list changes are included after inspection; the
untracked `zpr-dashboard/cmd/zpr-discover/foo` is an interrupted discovery
result artifact and is deliberately excluded, preserved locally.

Checkpoint verification: offline `cargo test -p vs` passed 6 library tests
and 511 service tests, with 8 ignored. The safe
`CGO_ENABLED=0 GOPROXY=off GOSUMDB=off make test` rerun retained these five
previously recorded dashboard failure groups:

- `TestAssistantSkillsEmbeddedAndTaskSpecific/scenario`
- `TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator`
- `TestOrganizationPolicyInstallNeverUploadsStaleBundle`
- `TestScenarioPlacementPreflightMatchesLauncher`
- `TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact`

All other Go packages passed or had no tests. The six real-fixture browser
cases passed as recorded above; the earlier 90-case UI evidence remains
separate. Opt-in receiver/crash-window fixtures were not re-executed for this
checkpoint. No existing failing test was modified to hide these failures.

Start the next fresh session here, inspect the checkpoint commit and working
tree, then reconcile the migrated legacy browser selectors and combined
acceptance ledger. Preserve the five known failure groups as explicit open
items. Do not repeat completed reset discovery or mistake fixture evidence
for real OIDC, crash/power-loss recovery, deployment or full migration
acceptance. PH/AuthCode/BAS ownership and native helper qualification remain
separate blocked workstreams. No live operations are authorized.

Old chat messaging has reached its service limit. The deletion tool refused
to delete the encompassing current session; no old chat is claimed deleted.
Retire the old team through the VS Code UI before starting a new team, rather
than creating another sibling replacement that may share the same limit.

### Fresh-session regression reconciliation (2026-10-16 evening)

Two background workers launched for this step stalled (no tool progress for
~110 minutes, no test processes or containers). They were told to stop and
left no repository changes; C0 ran both tasks directly at `eb17e3c`.

**Browser (cached Playwright 1.63.0 image `sha256:eff16c30e6f3`, arm64,
`--pull=never --network=none`, read-only source, temporary fixture binary):**
JS syntax passed for `scenario-browser.mjs` and `static/scenarios.js`. A
targeted `--grep` run gave 46 passed / 12 failed. Both
`simulator-reset-confirmation.spec.mjs` and
`simulator-reset-live-fixture.spec.mjs` passed. All three migrated
legacy-clear assertions in `dashboard.spec.mjs` fail on desktop and tablet:

- `Simulator Scenarios groups unfiled entries and keeps terminal run history`
  (line 5001: `Run` button not found).
- `Scenario cancel reports malformed JSON without retrying` (line 5084:
  `#scenario-cancel` click times out).
- `GUI scenario failure progress preserves the original step through cleanup`
  (line 8390: `#scenario-reset` expected visible, is hidden).

Unrelated matching tests also fail on missing current UI elements
(`#scenario-organization`, `#organization-connection`, `#policy-test-gutter`).
An attempted full `dashboard.spec.mjs` run (1,284 cases) timed out on most
early editor/assertion cases and was stopped. `dashboard.spec.mjs` appears
broadly stale against the current UI; this is not attributed to the reset
migration alone. No test was modified.

**Five known Go failure groups (individually rerun, no CGo, offline):**

| Test | Current failure |
|---|---|
| `TestAssistantSkillsEmbeddedAndTaskSpecific/scenario` | missing task instruction `"proposal to null"` |
| `TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator` | 409: organization-scoped service readiness preflight required before invitation creation |
| `TestOrganizationPolicyInstallNeverUploadsStaleBundle/success` | install script: `ZPR_MULTINODE_PLATFORM: unbound variable` |
| `TestScenarioPlacementPreflightMatchesLauncher` (6 cases) | stack function `resolve_machine_placement` missing |
| `TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact` | helper is not a Linux amd64 ELF executable (host/arch dependent) |

These remain open; root-cause classification and fixes are not yet done.
Next: decide whether to repair the stale `dashboard.spec.mjs` assertions
(requires explicit approval to edit pre-existing tests) and fix or
re-baseline the five Go groups. No commit, deployment or live operation.

### Browser test reconciliation (2026-10-16 night)

User approved updating the stale browser tests. Only `zpr-dashboard/tests/browser/`
and one stale README sentence changed; no product code, commit, deployment or
live operation. `diagnostics-guidelines.mjs` was removed (superseded by
`service-logs.mjs`). Every edited module passes `node --check`.

**Results (offline Playwright 1.63 container, `--network=none`):**

| Project | Before | After |
|---|---|---|
| desktop | 326 passed / 289 failed | 731 passed / 11 failed / 27 skipped |
| tablet | not run | 730 passed / 12 failed / 27 skipped |

**Main causes of the stale tests:**
- CodeMirror editor migration (hidden textareas, token classes, forwarded keys).
- Picker popover and row menus.
- Icon-only window controls and dialog close labels.
- Simulate split to `/scenarios.html#simulate`, with run views and gateway
  results moving there.
- New Simulator primary navigation plus organization section tabs.
- Unattached adapters moved to the Detached tray.
- The inspector now closes on outside pointerdown.
- Trusted Sources title and feed changes; Security Review markup and DNS probe.
- Removed connection strip and `#organization-active-name` (the active org is
  now shown by catalog cards and the banner).
- Activity metric cards removed.
- Provisioning copy and the `runtime_key_fingerprint` contract changed.
- Sign-out now opens provider sign-in once (`GUI deployments.md`, 2026-10-16),
  and the README was updated to match.
- Test timing fixes: the banner poll needs more time, and the tablet map width
  transition must settle before measuring.

**Remaining failures, left failing because they are product issues (all
reproduce on HEAD):**
1. Scenario editors get a duplicate Wrap toggle. `simulator-nav.js` re-runs
   `editor-search.js` without a guard (dashboard 2130,
   `remaining-code-editors` 74).
2. `.node-stats-count` is 10px, below the 11px Control Room minimum
   (`node-stats.css`).
3. The poll-change pulse makes inspector markup differ on every render, so
   all rows are rebuilt and focus is lost (`app.js`; visa refresh test).
4. The Directory editor stays inset 16px when maximized. The
   `organization-profile.css` `[open]` rule overrides maximize (shared
   maximize test).
5. Adding topology parents moves existing map nodes by about 8px with no
   viewport animation.
6. The new read-only "Policy operation status" form on `organizations.html`
   (from `6ab9bc9`) pushes the directory graph to y≈420, which breaks the
   graph-height layout test. Move it below the detail or collapse it.
7. Recovering from a login configuration outage shows "Sign-in failed. Check
   your credentials" because the catch path reuses `loginFailure = "failed"`.
   The no-loop guard is correct; only the message is misleading.
8. Rare, unreproduced (0 of 355 stress runs): an intermittent
   `Cannot read properties of null (reading 'dataset')` page error during
   Simulator page switching on tablet.

**Environment-only failures:**
- `enrollment-live` 115 and `enrollment-setup` 68 need `go` inside the
  browser container.
- `stable-login` 174 needs a live callback listener on `localhost:8787`.

The five Go failure groups above are unchanged and still open.

## 13. Packaging Continuation (2026-10-17)

**Scope:** User Control clarified the current request as making separate ZPR
production and Simulator companion packages. This section records bounded
engineering evidence only. It does not approve live service/host/network
operations, credential use, privileged changes, signing, publication,
deployment, or release qualification. The pre-existing untracked
`zpr-dashboard/cmd/zpr-discover/foo` remains outside scope and must be
preserved.

### PKG-CLI-TOOLS-01 — W5

**State:** Verified by C0 at bounded engineering-staging scope; no integration
or release acceptance. W5 owns only new Linux/amd64 CLI build/test scripts
and corresponding README updates in
`zpr-dashboard/packaging/system/` and `zpr-dashboard/packaging/simulator/`.
The reserved paths are `build-cli-linux-amd64.sh`,
`test-cli-linux-amd64.sh`, and `README.md` in each directory. C0 reviewed
these additions and independently ran `sh -n` on all four scripts and both
staging regression scripts. Both regression suites passed their missing/old
toolchain, argument, existing-output, build-failure/no-publish and successful
ELF metadata cases; generated binaries were not executed. `git diff --check`
passed and repository status showed only this plan edit, W5's four new scripts
and two README edits, and the preserved discovery example. This slice stages
existing `zpr-system-config` and `zpr-lab-bootstrap` commands; it is not a
complete production or companion package, compatible pair, signed release, or
fresh-host acceptance.

The non-executing package vet and Linux/amd64 cross-builds also passed as
recorded under PKG-CLI-ACCEPT-02. W5's final handoff confirms its unfiltered Go
package tests executed fixtures that generated ephemeral private keys/secrets,
contrary to the explicit assignment prohibition. Record this as an
out-of-scope validation execution, not acceptable test or release evidence.
Do not rerun any such tests without a newly scoped, explicit authorization.
Exact commands/output remain pending. User Control confirms W5 released both
packaging-directory reservations.
The duplicate helper in the Simulator staging regression script was removed;
C0 reran syntax and both script regressions against the corrected version.

### PKG-CLI-ACCEPT-02 — W4

**State:** Handoff received; no file reservation or edits. Under the explicit
restriction against generating fixture keys/secrets, Go tests were skipped.
C0 independently ran the two permitted non-executing checks from
`zpr-dashboard/`:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS= go vet ./cmd/zpr-system-config ./cmd/zpr-lab-bootstrap
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS= GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o <private-temp>/zpr-system-config ./cmd/zpr-system-config
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS= GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o <private-temp>/zpr-lab-bootstrap ./cmd/zpr-lab-bootstrap
```

Vet and both builds passed with Go 1.27.1 on darwin/arm64 and Visa Service
HEAD `29b4b13dc85dec9538f19012a7fa7e6e54ac8857`. C0 confirmed the outputs were
statically linked x86-64 ELF binaries and removed them without execution.
No Go tests ran; this is not test-suite, package-handoff, Ubuntu install, or
runtime acceptance evidence.

### PKG-GAP-MAP-03 — W3

**State:** Read-only handoff complete; no reservation or edits. The map used
only `git show 29b4b13:` snapshots of the system and Simulator packaging
READMEs and System release plan, plus the current packaging file listing
excluding W5-reserved files. It identifies source/build inputs and generated
engineering configurations, but no installable production/companion bundle,
production Compose appliance, signed manifest, compatible release lock, or
qualified pinned image set. Runtime/provider/engine versions and named
component/release owners remain unspecified in the inspected evidence.
The map's component-level acceptance gaps include supervised production
service composition, persistent state, endpoint admission/credential delivery,
protected DNS integration, provider/logging qualification, and paired fresh-host
acceptance with Simulator unavailable. W1/W2 dependencies are not assigned
owners by these documents; do not infer them from role labels. The gap map is
planning evidence, not a release manifest or acceptance report.

### PKG-PROD-SERVICES-INV-04 — W1

**State:** Withdrawn after a routing error. C0 sent W3's already-completed
`PKG-GAP-MAP-03` prompt to W1's chat. The W1 transcript confirms W1 completed
that read-only duplicate; its result is not counted separately from the
canonical map already supplied to C0. C0 also misrouted W3's next source task
to W1. W1 initiated five `git show` reads at the specified commit but reported
that their outputs were moved to the background. W1 later read full output for
`zpr-dashboard/cmd/zpr-lab-bootstrap/identity_stack.go` and
`zpr-dashboard/cmd/zpr-system-config/main.go`; it saw only oversized-output
previews for `providers.go` and `zpr-lab-bootstrap/main.go`, and the
`zpr-system-config/config.go` read failed because that path was absent at the
requested commit. No further source was read after those outputs. This was
out-of-scope inspection after the stop, not a W3 handoff. W1 confirms no edits
or reservation. C0 preserved the stop and did not accept the duplicate
separately. No W1 source edits were visible in the shared Visa/Core worktrees.

### PKG-PROD-ENTRYPOINTS-06 — W1

**State:** Read-only source handoff complete; no reservation or edits. Base is
Visa Service `main`, HEAD `29b4b13dc85dec9538f19012a7fa7e6e54ac8857`. W1
reports its permitted reads were non-test entrypoints/build files, relevant
config source, and a static asset filename listing. One exploratory pinned
`git grep` used a `*.go` glob that matched test Go files; its offloaded output
was not inspected or used and is excluded. No Go tests ran.

The exact scope-slip command was:

```sh
git -C 'ZPR Docs/zpr-visaservice' grep -n -E 'ZPR_(ENROLLMENT|POLICY|ASSERTION|OPENOBSERVE|CONTROL_SERVICE|GATEWAY|TRUSTED|CONTROL_ROOM)' 29b4b13dc85dec9538f19012a7fa7e6e54ac8857 -- 'zpr-dashboard/cmd/zpr-web-dashboard/*.go'
```

The glob matched test files. Its 46.8 KB offloaded output at
`/var/folders/3k/m6scq8tj6hb39xx8rwzmpshh0000gn/T/1792275976914-copilot-tool-output-64251-5db31f3f-a27d-4851-b580-8a79a95b9bea.txt`
has not been opened or inspected and is excluded entirely. W1 states that its
production report used only the pinned `go.mod`, `Makefile`,
`cmd/zpr-web-dashboard/{main.go,control_service.go,policy_service.go,operator_login.go,admin_client.go}`,
`cmd/zpr-enrollment-service/main.go`, and static asset filenames from
`git ls-tree`; `git status --short` was used only to preserve state. No result
from the wildcard grep contributes to the report. No tests/build/execution or
edits occurred for this inventory, and W1 stopped without further reads.

- `cmd/zpr-web-dashboard/main.go:165` selects `control-room` by default and
  accepts `-mode`/`ZPR_WEB_MODE` and `-listen`/`ZPR_WEB_LISTEN`.
  `control_service.go:21` composes Control Room; `operator_login.go:36`
  reads OIDC/TLS settings. Its browser assets are embedded with `//go:embed
  static/*` in `main.go:25`. Runtime inputs include TLS certificate/key,
  OIDC config/secret/optional CA, organization/management settings, and an
  mTLS client connection to Control-Service.
- The same source binary has `control-service` and `policy-service` mode
  branches at `main.go:224` and `main.go:220`. `runControlService` at
  `control_service.go:109` uses mTLS and configures Visa Admin, Policy-Service,
  enrollment handling, assertions/diagnostics, and optional gateway state.
  Its entry code can continue to start the listener when some downstream
  configuration is unavailable; listener start is not readiness evidence.
  `runPolicyService` at `policy_service.go:166` requires mTLS listener files
  and policy workspace/apply resources. No strict startup/readiness sequence
  is encoded in these entrypoints.
- `cmd/zpr-enrollment-service/main.go:14` is a separate `-config` driven
  process that loads server config and runs the device HTTPS server. Its
  config/registry/credential contract and its relationship to the
  Control-Service enrollment handler remain unresolved.
- `go.mod` declares Go 1.26.1 and module dependency versions. The Makefile's
  `build` target produces `bin/zpr-dashboard` from
  `./cmd/zpr-dashboard`, not `cmd/zpr-web-dashboard`; there is no named
  Enrollment Service target. W1 did not build; C0 independently cross-built
  `./cmd/zpr-web-dashboard` and `./cmd/zpr-enrollment-service` with Go 1.27.1,
  `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`, `-mod=readonly -trimpath
  -buildvcs=false`. Both produced static x86-64 ELF files, were not executed,
  and were removed from a temporary directory. This proves source buildability
  at that toolchain/base only, not supported release artifacts or installability.

The scope slip and its excluded output are recorded, not used as evidence.
Remaining decisions include the release owner/source revision for each
service, the production binary/image layout for the three dashboard modes,
runtime config/secrets and downstream readiness contract, Enrollment Service
ownership/schema, and a release build target. No supported tags, image pins,
Compose composition, or ownership were inferred.

### PKG-DNS-ADAPTER-INV-05 — W2

**State:** Read-only handoff complete; no scoped source edits or reservations.
W2 reports bases match the expected Core `work/macos-tun-address` HEAD
`561fa7351795a723b74b5d1e57761d870722c77c` and Visa Service `main` HEAD
`29b4b13dc85dec9538f19012a7fa7e6e54ac8857`. Unrelated pre-existing Visa
packaging/docs changes were preserved. No builds or tests were run.

- Artifacts in scope are the PH (`ph`) Cargo binary in workspace version
  `0.7.0`, the Visa (`vs`) Cargo binary version `0.19.0` with Rust `1.98.1`,
  and `dns/bind9`'s Dockerfile based on `debian:trixie-slim` with unversioned
  apt packages. No `ph`/`vs` image or build handoff exists; BIND has no
  immutable image digest or apt package pin.
- PH's optional adapter loopback DNS stub is a client-side feature: it forwards
  to a configured literal upstream over TCP through the adapter TUN. It is
  distinct from the protected authoritative BIND service.
- BIND's entrypoint requires the matching PH adapter process to be visible
  through a shared PID namespace and enters its network namespace. Visa's DNS
  publisher performs RFC2136 updates through external `nsupdate` (default
  binary name), and requires the DNS service and adapter to be reachable first.
  `dns_update` is optional; join/leave plus 30-second periodic reconciliation
  is gated on policy-authorized registrations. BIND requires zone config/file
  input; its entrypoint seeds reverse-zone files only.
- PH inputs include a BAS RSA key and authentication TLS CA. Visa BAS
  validation requires configured HTTPS mTLS CA/client certificate/key and a
  token-verification key. The Visa DNS publisher's TSIG key file must match
  BIND's included key; BIND copies the mounted key into its runtime path.
- Open decisions: immutable artifact/version/platform pins; whether the Visa
  runtime artifact includes `nsupdate`; adapter process/name and services
  configuration alignment; namespace, startup/readiness, zone/config and
  secret-mount handoffs; and the accountable package/integration owner.
  Existing RFC2136/TSIG and BAS-validation contracts are source evidence, not
  a proposal for a new protocol/API.

The handoff text received ends mid-clause after `BAS validation/2.`; no
continuation was available, so no meaning is inferred from that suffix.

### PKG-COMPANION-CONFIG-INV-07 — W3

**State:** Read-only source handoff complete; no reservation or edits. At Visa
Service `main` HEAD `29b4b13dc85dec9538f19012a7fa7e6e54ac8857`, W3 traced the
identity Compose path through `cmd/zpr-lab-bootstrap/main.go`,
`providers.go`, and `identity_stack.go`; no binary ran and no secret values
were read or reported.

- Optional lab identity generation emits LDAP/Dex configs and handoff files;
  it does not install or start services. `-identity-services` requires exact
  Room/Simulator/OIDC HTTPS origins. `-identity-compose` also requires explicit
  bind IP, LDAP/Dex immutable image IDs or digests, and platform; no actual
  image names/digests or qualified provider versions are selected.
- Generated Compose project/services are per-install `zpr-lab-identity-*`,
  `ldap-initialize`, `ldap`, and `oidc`, with an identity bridge and private
  LDAP/Dex state directories. Initialization is separate and only for empty
  LDAP state; OIDC waits for LDAP health. This is configuration generation,
  not activation or installer integration.
- The generator emits per-lab TLS and service-secret material. Seeded lab
  LDAP users/groups do not grant ZPR operator permissions; the handoff marks
  grants unconfigured and requires verified live issuer/subject configuration.
- `cmd/zpr-system-config/main.go` accepts company JSON and optional offline
  credential-file checks; it performs no network/readiness check, config
  rendering, or installation. W3 subsequently inspected the top-level schema
  in `internal/systemconfig`; nested OIDC validation and the underlying
  protected-file reader remain separate source dependencies.

### PKG-COMPANION-SCHEMA-INV-08 — W3

**State:** Read-only handoff complete; no edits/builds/tests. At Visa Service
`main` HEAD `29b4b13dc85dec9538f19012a7fa7e6e54ac8857`, W3 inspected only the
permitted `internal/systemconfig` production source.

- `config.go:22-45` defines schema version 1 and top-level `organization`,
  `control_room`, and `oidc` objects. Input is bounded to 64 KiB, exactly one
  JSON document, no nulls, nesting <=16, lowercase unique keys, and no unknown
  fields or wrong types (`config.go:19,51-76`). Organization ID/name and HTTPS
  origin/private management-IP constraints are source-validated
  (`config.go:128-156`).
- Cert/key/secret references are required; CA is optional. Names must be
  distinct canonical absolute Linux paths, not `/`, with shell expansion
  characters rejected. The OIDC redirect must equal the Control Room origin
  plus `/auth/operator/callback`; issuer must be HTTPS; grants are restricted
  to exactly the configured organization, source-allowlisted permissions,
  with at least one `monitor.read` grant (`config.go:174-209`).
- `CheckFiles` applies size/content/TLS checks to referenced cert/key/secret/CA
  files (`files.go:19-69`) and validates the server certificate against the supplied chain tip
  as an inspection anchor. This does not prove browser trust, revocation,
  provider discovery/login, network reachability, config application or
  runtime readiness.
- Nested `oidc.config` struct/validation is owned by `internal/operatorauth`;
  actual filesystem permission/ownership/symlink/hardlink behavior is owned by
  `internal/servicehttp.ReadOperatorFile`. W3 stopped at those dependencies
  as required and did not inspect their source.

### PKG-COMPANION-CONFIG-DEPS-09 — W3

**State:** Read-only source handoff complete; no reservation or edits. W3
confirmed Visa Service `main` HEAD
`29b4b13dc85dec9538f19012a7fa7e6e54ac8857`; W5 modifications, packaging
scripts, and the pre-existing `zpr-dashboard/cmd/zpr-discover/foo` remain
unchanged. No builds, tests, or execution.

- `internal/operatorauth/auth.go:26-30,47-54` defines nested OIDC `config` as
  `version`, `issuer`, `client_id`, `redirect_url`,
  `session_lifetime_seconds`, and `grants[]`; each grant has `issuer`,
  `subject`, `organizations[]`, and `permissions[]`.
- `auth.go:33-45,57-107` validates version 1; HTTPS issuer/redirect URLs with
  hostname and no user/query/fragment/opaque components; the exact callback
  path `/auth/operator/callback`; nonempty plain-text client ID of at most 256
  bytes; session lifetime 60–3600 seconds; and at least one grant. Grant
  issuer must match the configured issuer; subjects must be nonempty plain
  text, at most 256 bytes, and unique. Organization and permission lists must
  be nonempty and unique, with nonempty plain-text entries of at most 256
  bytes and no controls or edge whitespace. Permissions must be in the
  source-defined allowlist. The allowlist includes simulator/admin scopes;
  that does not mean a company configuration grants them or that they are
  appropriate to select. These checks validate config strings/scopes, not
  actual identity, OIDC discovery/login, or applied authorization grants.
- `internal/servicehttp/operator_file_unix.go:12-43` (`linux || darwin`)
  `ReadOperatorFile` requires an available regular file with one hard link,
  owned by root or the current effective UID, and disallows group/other write
  bits; `private=true` also disallows all group/other permission bits. It
  opens with `O_RDONLY|O_NOFOLLOW|O_NONBLOCK`, checks the opened file against
  pre-open metadata, and performs a bounded `limit+1` read, rejecting
  over-limit input and read errors. Final-component symlinks are rejected.
  The helper does not check parent-directory ownership/modes or prohibit
  intermediate-component symlinks.
- `internal/servicehttp/operator_file_other.go:1-8` returns an explicit
  unsupported protected-file-check error on platforms other than Linux/macOS.

These checks do not establish parent-directory trust, immutability beyond the
stated metadata comparison and bounded read, OIDC discovery/login, TLS/browser
trust, revocation, service readiness, runtime configuration, installation, or
active grants. The top-level system-config path validation still requires
canonical absolute Linux paths. This closes the nested OIDC schema and
protected-file helper inventory only; it does not provide provider ownership,
runtime rendering, or an installable artifact.

**Ranked M2 critical path toward the first company install:**

1. **Release ownership and artifact inventory.** W1 has identified buildable
   source entrypoints for the combined dashboard modes and standalone
   Enrollment Service; C0 cross-built both for Linux/amd64 at Go 1.27.1 from
   this checkout. The Makefile still targets a different dashboard command,
   and no release artifact/pin/owner follows from those builds. W2's
   protected-DNS/adapter source inventory is complete but identifies no
   immutable `ph`/`vs`/BIND release artifacts or package owner; W3's external
   config-dependency inventory is complete but does not establish release
   ownership or runtime readiness. Decision needed: who owns each required production/companion
   artifact and which exact source revision/toolchain/image version is the
   supported candidate? Do not guess image tags or call a repo HEAD a release.
2. **Production runtime composition.** W1 found Control Room depends on
   Control-Service mTLS; Control-Service configures Visa Admin, Policy-Service,
   enrollment handling, assertions/diagnostics and optional gateway state.
   Services expose mode-specific listeners, but no strict startup/readiness
   sequence is encoded, and Control-Service can start despite missing downstream
   configuration. Obtain owner contracts for entrypoint/arguments, config and
   secret references, readiness/failure, listener/network boundaries,
   persistence, backup and recovery. Decision needed: who supplies and supports
   the full Ubuntu Compose/runtime definition and upgrade/removal lifecycle?
   No Compose file or installer can be responsibly authored without these
   contracts.
3. **Enrollment and endpoint admission.** W1 found a standalone Enrollment
   Service `-config` entrypoint but not its config/registry/credential schema
   or its relationship to Control-Service's separate enrollment handler.
   Resolve Enrollment's sole-writer/API ownership, supervised node entrypoint,
   endpoint-specific key/credential delivery, selected adapter support, and
   revocation/restart behavior. These are prerequisites to first-endpoint
   acceptance, not substitute services packaging may invent.
4. **Provider, bootstrap and protected DNS handoffs.** Existing-vs-bundled
   identity/logging modes are approved as installer choices, but provider
   versions, identity/attribute mappings, first-admin creation/recovery,
   logging persistence/authorization remain unspecified. W2 identified the
   protected-DNS RFC2136/TSIG publisher path, BIND namespace/readiness
   dependency, and PH/BAS inputs, but artifact pins, `nsupdate` inclusion,
   process/config/secret-mount alignment, and the package/integration owner
   remain unresolved. Company OIDC JSON and protected-file validation rules
   are source-inventoried, but do not establish provider versions, active
   grants, login, or runtime behavior. Company TLS and underlay DNS remain
   explicit inputs. Decision needed: accountable owners for each interface
   and its supported artifact/configuration contract.
5. **Artifact assembly and fresh-host acceptance.** Only after the above pins
   and contracts are agreed can an installer/package assemble release
   artifacts and test a complete from-scratch Ubuntu host with named operator
   login, endpoint admission, loaded policy, real allow/deny traffic, restart
   and recovery. Signing/release qualification and any live installation still
   require separate authorization.

The approved target is already Ubuntu 24.04 LTS x86-64, a dedicated
server/VM and a Docker Engine/Compose v2 appliance; provider modes are already
approved. W1's inventory and non-executing cross-build evidence, W2's
protected-DNS/adapter source inventory, and W3's identity-generator, schema,
and config-dependency inventories are complete. These inventories confirm
missing release/install handoffs and runtime readiness contracts. W2's
received handoff text ends mid-clause after `BAS validation/2.`; no remainder
is inferred. No additional user choice is presently required
before those reports. The closest M2 implementation remains blocked on
artifact owners and service contracts; do not create a source-dependent or
fake installer.
Fresh-host and D6 live acceptance remain separate gates with their own
prerequisites and explicit execution authorization.
