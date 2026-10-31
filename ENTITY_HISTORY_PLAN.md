# Control Room Audit tool plan

Status: proposed implementation plan, 2026-10-26. No runtime changes or release
qualification are implied. Accessibility and broader UI redesign remain deferred.

GUI-first progress: a standalone, sample-only HTML prototype now includes
Access Explorer, Policy Changes, History and Reports. Reports offers group,
service, policy-impact and evidence-coverage templates, scoped pack assembly,
ordering/removal, shared reporting metadata, preview, HTML download and browser
printing. Dates are explicitly metadata only, not a historical query. Saved
packs, scheduling and live evidence remain Not Implemented. This prototype is
outside production code and has not been integrated into Control Room.

## Goal and boundaries

Build an **Audit** tool within Control Room with three linked workspaces:

1. **History:** slice observed operational events by entity, time and outcome.
2. **Policy evolution:** understand authored, published and activated policy
   changes over time, including their potential effect on access.
3. **Potential access:** evaluate which known services a selected user/machine
   could access, and which users/machines could access a selected service,
   under an explicitly selected policy and context.

Audit is an investigative operator tool, not an assertion that the underlying
evidence is a complete or tamper-proof audit ledger. Existing entity History
sections should deep-link into Audit with scoped filters rather than introduce
separate query/analysis implementations.

Give operators useful, authorized history for a visa, policy revision, adapter
and user: what was observed, when, by which component, with what outcome and
which related entities. Start with operational evidence, not a claim of a
complete, immutable audit ledger.

Authoritative state stays in VS, Policy-Service and other production services.
OpenObserve provides searchable historical evidence. Control Room accesses it
through Control-Service, never through browser-held backend credentials.
Neither operator component may depend on Simulator, its availability or its
manifests/profiles/sessions/workloads. Simulator may supply test traffic.

### Self-approval role-risk warnings

Approved requirement (2026-10-30); implementation and live acceptance pending.
All Simulator installations use the authenticated operator's scoped permissions
to self-approve selection-triggered setup without further approval prompts.
See the [ordinary-client decision and delivery gates](SIMULATOR_AS_ZPR_CLIENT_PLAN.md#active-turn-key-delivery-plan-2026-10-30).

Audit should detect and warn about risky self-approval role combinations through
assertions or other explicit analysis rules. Use independently configured
production identity/grant and approval-event contracts, never Simulator files
or availability. Identify the affected issuer/subject, organization, role/grant,
rule or assertion, exact operation/revision when available, and supporting
evidence. Distinguish an observed self-approved operation from potential risk
in a role assignment; incomplete evidence means unknown, not safe.

Warnings are advisory, do not add a mandatory Simulator confirmation, and
cannot confer permissions or excuse a failed backend authorization check.
Existing Control Room approval workflows remain unchanged. Qualify with a
known risky-role fixture and a nonmatching fixture, scoped access controls and
Simulator unavailable; do not claim a complete audit ledger or deployed warning
engine from these requirements.

## Audit workspaces

### History: what was observed?

Use the entity-history phases below. Link events to exact policy revisions and
analysis inputs when those references exist. Preserve the distinction between
an observed event and a later reconstruction. Saved investigations reference
evidence and its retention/availability; they do not imply logs will exist
forever.

### Policy evolution: what changed, and when did it take effect?

Provide a revision timeline and side-by-side comparison:

- Author, timestamp and reason/review reference where recorded.
- Source diff plus changes to definitions, assertions, source bindings and
  relevant configuration; do not call a text diff an effective-access diff.
- Separate draft/save, validation, publication and activation. A published
  revision is not proof that all enforcement points activated it.
- Show activation scope and acknowledgments where available, gaps otherwise.
- Select two immutable revisions for a bounded impact analysis over the same
  explicitly captured identities, services and context.

Impact output groups newly permitted, newly denied, unchanged and unresolved
cases, with available policy explanations. Pin compiler/evaluator versions and
dependency inputs. Hold context constant to isolate policy effects; a separate
context comparison may explore attribute/identity changes. Never explain a
historical access change solely from policy text if source data, identity links,
configuration or evaluator semantics also changed.

Historical replay requires retained exact inputs, source evidence and evaluator
compatibility. Without them, label results as a reconstruction using specified
inputs, not proof of the historical decision. Snapshot retention must follow
privacy/access policy; do not indiscriminately archive user attributes.

### Potential access: what could this user access?

Select a canonical, organization-scoped user and enumerate services from
independent production service-directory/inventory contracts. A service ID alone
may not define an evaluable request: include destination identity, service
instance, protocol/port, source device/session and other required context.
Do not treat a display name or directory entry as proof of a live endpoint.

Run bounded, read-only evaluations through the production policy engine or a
production analysis contract reusing its semantics. Never approximate policy
with a second JavaScript evaluator. Analysis must not issue/install a visa,
activate policy, create a session or mutate a trusted source.

For each evaluated service/request, distinguish:

| Result | Meaning |
|---|---|
| Permitted in selected context | Exact evaluated inputs satisfy policy; not a reachability or installed-visa claim |
| Denied in selected context | A valid completed evaluation denies that concrete request |
| Context required / conditional | Missing device, session, time or other context prevents an unconditional answer |
| Indeterminate / unavailable | Evaluation/dependency failure, stale evidence or unsupported analysis; not denial |
| Not evaluated | Outside selected inventory/scope or bounded execution budget |

Absence of an attribute is not permission to substitute false/default values
unless the actual policy/source contract defines that behavior. Use the same
source-side field/principal authorization as real evaluation; operator analysis
permission must not become privileged attribute disclosure.

The first implementation evaluates **concrete selected contexts** against a
finite inventory. "Permitted now" is not an existential proof that access is
possible under some device/time/location. Exhaustive possible-access analysis
or symbolic conditions require separate evaluator support and an explicit
scope/feasibility decision. Every summary states its inventory coverage and
context; never advertise "all services" from a partial catalog.

Each result records policy revision, evaluator version, subject/identity-link
provenance, service/request context, observation time and dependency freshness
without exposing prohibited underlying data. Where the engine supplies safe
explanations, link matching rules/assertions and bounded reason codes.

The UI should support:

- User -> potentially accessible services under selected context.
- Service -> users/machines, as a first-class access-analysis direction with
  an explicit bounded cohort and privacy authorization. Enumerate authorized
  production user and machine inventories; distinguish user-only,
  machine-only and combined user-on-machine requests according to actual
  evaluator semantics. Do not infer access for every machine from a user's
  result, or for every user from a machine's result.
- Policy revision A versus B -> access differences for the same pinned inputs.
- Potential access -> related observed history, keeping the two result types
  visibly distinct.

  For a selected service, offer user, machine and user-on-machine result views.
  Show canonical identities, evaluated context, outcome and safe explanation,
  with filters for permitted, denied, conditional and indeterminate results.
  State cohort coverage, inventory freshness and skipped candidates explicitly.
  Do not claim an exhaustive list from one inventory page or silently evaluate
  the entire user-by-machine Cartesian product. Bound and paginate candidate
  enumeration as well as results; cross-pair evaluation needs an explicit scope
  and execution budget. A machine's presence in inventory is not proof of a live
  session or of a particular user being authenticated on it.

### Shared authorization and execution controls

Define separate read-history, read-policy-history and run-access-analysis
capabilities using repository permission conventions. Existing monitoring or
policy-edit access must not silently confer all three. Authorize organization,
entity visibility, source operations and explanation details server-side.

Enforce bounded cohorts, service candidates, evaluation concurrency, duration,
result sizes and cancellation. Return partial coverage explicitly. Use request
IDs and reject stale results after user/revision/context selection changes.
Audit the investigation action with safe actor/scope metadata, not copied
passwords, attributes or unrestricted result payloads.

## Tailored reports and report packs

Audit must eventually produce a set of audience/purpose-specific reports, not
merely export the currently visible screen. The standalone prototype's print/
HTML export is an initial layout exercise, not the final reporting architecture.

Candidate report templates:

- User or machine access review: memberships, evaluated services, remaining
  constraints and related observed history.
- Service access review: authorized user/machine cohorts, request contexts,
  group contributions, outcomes and unresolved candidates.
- Group review: direct/nested members, source provenance and potentially
  available services without treating membership as a permission grant.
- Policy change/impact review: exact revisions, authors/reviews, publication/
  activation evidence and access differences under fixed inputs.
- Assertion review: versioned definitions, enforcing versus diagnostic role,
  referencing policy revisions/services, source dependencies and scoped
  evaluation evidence. Separate satisfied/not satisfied, context required and
  indeterminate results. A satisfied assertion is not an overall access grant;
  a diagnostic failure is not automatically an enforcing denial. Include
  change history when retained and never infer population-wide pass rates
  from selected observations.
- Visa/decision investigation: correlated lifecycle, available explanations and
  missing evidence.
- Evidence coverage: source health, retention boundaries, delivery gaps and
  unknown completeness.

Support operator-selected templates assembled into a named report pack with a
shared organization, reporting period, evidence cutoff, purpose and intended
audience. Tailoring selects authorized subjects, services, groups, revisions,
sections and permitted detail/redaction level. It cannot remove mandatory
coverage warnings or relabel conditional access as unconditional permission.

Distinguish event reporting period, membership/context snapshot time, policy
effective revision and generation time. Include template/schema version,
report/pack references, provenance, filters, coverage and limitations. Historical
claims require historical evidence, not current attributes substituted silently.

Preview before generation. Initial deliverables can be printable PDF and
standalone HTML; structured evidence/CSV exports, saved template configuration,
scheduled generation and signed/integrity-protected packages require separate
implementation/authorization decisions. Each format must preserve scope and
redaction; protect CSV consumers from formula injection when that export exists.

All reports and cached/generated files enforce the same entity/organization/
field authorization as interactive Audit. Define generation-time and download-
time authorization, artifact retention/expiry, secure storage and access audit.
Audience labels do not grant access or automatically authorize delivery.
Scheduling/email distribution and any completeness/compliance certification
are not implied by template support.

Add a future Reports workspace: template catalog -> scoped parameters ->
section preview -> generate/download a single report or pack. Start with a
small approved template set; validate usefulness with auditors before adding a
general report designer. Template and pack controls may remain explicitly
Not Implemented in the GUI-first prototype until their contracts are approved.

## Existing foundation and gaps

Inspected sources:

- [OpenObserve profile](observability/openobserve/README.md): OTLP/HTTP
  ingestion; counters, denials and redacted process logs; organization resource
  tagging and optional operator-configured diagnostic sources. Its local
  simulator collector path is not a production deployment contract.
- [OpenObserve reader](zpr-dashboard/internal/controlservice/adapterlogs/openobserve.go):
  fixed seven-day, latest-100 query against a configured stream.
- [Log handler](zpr-dashboard/cmd/zpr-web-dashboard/adapter_logs.go):
  groups configured network/file/container log sources into display lines.
- [Evaluation runbook](zpr-dashboard/packaging/evaluation/README.md):
  acknowledged/queryable bootstrap lifecycle events; not comprehensive entity
  history.

The denial endpoint supplies a bounded in-memory 500-entry window; polling can
lose events on bursts/restarts. Process text does not guarantee stable entity
references. Evaluator signals retained in visa metadata are not an implemented
outbound audit stream. OTLP exists in the observability profile; do not replace
it with another transport just because evaluation bootstrap uses native JSON.
Different deployments/streams/retention settings must remain distinguishable.

## Phase 0: evidence inventory and scope gate

Inspect the active maturity baseline before coding. For each candidate event,
record its producer, current fields, canonical entity identifiers, authorization,
retention, durability, delivery path and observed gaps.

Inventory at least:

- Visa decision/denial, issue, delivery, installation acknowledgment,
  revocation and expiry.
- Policy edit, analysis, publication and activation with exact revisions.
- Adapter authentication/session, readiness, disconnection and errors.
- User authentication/identity resolution and links to decisions.
- Policy revision storage, review/publication/activation history and retained
  evaluator artifacts; availability of exact historical dependency evidence.
- Production user/service inventories and read-only policy-evaluation APIs,
  including context requirements, source authorization, explanations and side
  effects.

Distinguish canonical ZPR user identity from provider-qualified identity and
operator identity. Never correlate by email/display name alone. Check whether
existing production audit stores already answer policy/admin-history questions
before duplicating records.

**Deliverable:** event/identifier coverage matrix, proposed first-slice events,
and dependencies on maturity APIs. Classify each as available, needs enrichment,
new producer required or not observable.
Include a capability matrix for all three Audit workspaces, identifying whether
policy comparison and potential-access analysis can reuse existing engine
contracts or need new production APIs.

**Exit gate:** demonstrate how one real visa can join to its exact policy
revision and participating adapters, or explicitly narrow the first slice if
those references do not exist. Do not estimate the remainder as query-only work.

## Phase 1: minimum event contract

Define a versioned structured event envelope carried using existing OTLP
ingestion where supported. Prefer a small shared vocabulary to a new framework.

Required common concepts:

| Field group | Meaning |
|---|---|
| Schema/event identity | Schema version, unique event ID, event type |
| Time | Producer event time and collector ingestion time, kept distinct |
| Scope | Authoritative organization/network scope |
| Producer | Service name, instance identity and restart/session context |
| References | Canonical entity types/IDs, exact policy revision when relevant |
| Correlation | Request/decision correlation and causal parent where available |
| Outcome | Observed action/result, bounded reason code and safe summary |
| Provenance | Producer versus inferred observation; redaction/schema version |

Do not fabricate unavailable identifiers or infer installation from delivery.
Distinguish actual events from lifecycle facts derived from timestamps (such as
expected expiry). Do not record credentials, tokens, raw directory attributes,
full packets or unrestricted policy/decision payloads.

**Exit gate:** representative event fixtures for success, denial, duplicate,
out-of-order delivery, missing correlation and redacted sensitive data. Approve
identifier semantics and organization authority before wiring ingestion.

## Phase 2: first vertical slice -- visa timeline

Follow one visa from its producing decision through the lifecycle events
available in Phase 0. Link policy revision and source/destination adapters.
Show explicit gaps rather than filling them with assumed success.

Implementation:

1. Enrich the smallest necessary production event producers with canonical
   references. Reuse existing logging/collector helpers.
2. Send structured events through configured production ingestion independent
   of Simulator. Retain existing ingestion for unrelated logs.
3. Add bounded buffering/retries and observable delivery/drop counters.
   Preserve event IDs across retries for deduplication.
4. Keep telemetry failures off the authorization/issuance critical path.
   Document crash-loss windows rather than claiming lossless delivery.
5. Add a typed Control-Service entity-history query and normalized response.

Proposed query inputs: authorized organization, entity type/ID, bounded time
range, allowlisted event types, page size and opaque continuation cursor.
No arbitrary SQL or caller-selected backend credentials/streams.

Proposed response: structured events, effective time window, continuation,
configured-source status, retention boundary and known delivery/coverage
limitations. A successful empty search means "no matching records observed,"
not "the action never happened." Return explicit unavailable/partial status.
An unknown coverage state must not become a claim of completeness.

Authorization must check both organization and entity visibility before
querying. Scope every backend query server-side. Related links cannot disclose
entities outside the caller's permissions. Validate identifiers; parameterize
queries if supported, otherwise use a narrowly tested query builder rather than
interpolating arbitrary filter text.

**Exit gate:** a real service-generated visa scenario yields a history response
linked to the exact revision/adapters; foreign-scope and unauthorized requests
are rejected; backend outage is distinguishable from an empty result.

## Phase 3: operator history view

Add a small History section to the visa detail surface, reusing current UI
components rather than redesigning the interface.

- Time range and event-type filters, newest/oldest ordering and pagination.
- Producer time versus received time where late delivery matters.
- Expandable bounded details and links to authorized related entities.
- Visible retention, partial-source, unavailable and missing-event indicators.
- Session-safe request cancellation/stale-response handling when selection
  changes.

Local display filtering is not authorization. Credentials and raw backend
queries never reach the browser. Test Control Room with Simulator unavailable.

**Exit gate:** browser acceptance covers normal timeline, empty result,
backend failure, pagination, permission denial and rapid entity switching.
Use existing accessibility conventions, but defer the separate accessibility
audit and broad theme/UI work.

## Phase 4: expand using the same contract

After the visa slice passes:

1. Establish the Audit navigation/workspace and deep-link existing history into it.
2. Policy revision timeline and source comparison, reusing authoritative
   audit/revision records; preserve publication versus activation distinctions.
3. Bidirectional potential-access analysis: user/machine -> bounded services,
   and service -> bounded users/machines or explicitly selected user-machine
   pairs, using concrete contexts and read-only production evaluator semantics.
4. Revision-to-revision access comparison over pinned identical inputs.
5. Adapter session/health and visa delivery/install/revoke history.
6. User history across decisions/visas using authorized canonical identity
   links; make historical identity-link changes explicit.

Add the following acceptance gates beyond entity history:

- Identical policies with different context are not mislabeled a policy change.
- A text-only policy change can produce no effective-access change.
- Different revisions can produce newly permitted/denied outcomes, with unknown
  cases reported separately.
- Publication without activation is not shown as an active authorization change.
- Missing attributes, unavailable providers and partial inventory do not produce
  false permitted/denied results or an exhaustive-service claim.
- Access analysis creates no visa and makes no enforcement/configuration writes.
- Forward and reverse analysis of the same concrete user/machine/service request
  at the same pinned inputs produce the same decision. Machine-only permission
  never implies permission for all users, and user-only results never imply
  permission from all machines.
- User/organization switches cancel or discard old results; revoked permissions
  do not preserve visibility through cached analyses or saved investigations.

User explanations are a separate capability from a timeline. "Why denied?"
requires recorded decision evidence, applicable policy revision and safe
explanation semantics; absence of a log line cannot answer it.

Each entity gets its own coverage/permission tests. Avoid one global free-text
search endpoint as a shortcut around those boundaries.

## Phase 5: qualify reliability and optional audit guarantees

Operational-history acceptance:

- Duplicate, delayed and out-of-order events; producer/collector restart.
- Clock skew, burst loads, buffer exhaustion and observable drops.
- Retention expiry, backend outage/recovery and backup/restore behavior.
- Stable pagination for identical timestamps and newly arriving records;
  cursor binding to scope, filter and fixed query window.
- Sensitive-field redaction, cross-organization isolation and permission
  revocation during browsing.
- Measured query latency/event volume/retention cost against an agreed workload.
  Set explicit limits and pass thresholds before qualification.

If immutable security audit is required, approve a separate work item for
durable producer outboxes, delivery guarantees, tamper evidence, restricted
administration, retention/legal hold and security-state recovery. Neither OTLP
acknowledgments nor OpenObserve persistence alone establish those guarantees.

## Sequencing, effort and coordination

| Work | Relative effort | Prerequisite |
|---|---|---|
| Inventory and schema fixtures | Small/medium | Source inspection and identifier agreement |
| Scoped backend query/typed response | Medium | Schema, permission and stream contracts |
| Real correlated visa producers | Medium/large, highest uncertainty | Maturity decision/visa lifecycle interfaces |
| Small History UI | Medium | Stable typed query contract |
| Policy evolution | Medium, higher if revision/activation evidence is missing | Authoritative revision/history contracts |
| Bidirectional concrete-context potential access | Medium/large | Production user/machine/service inventories, bounded candidate/pair enumeration, reusable side-effect-free evaluator, source authorization |
| Policy access-impact comparison | Medium/large | Pinned revisions and identical evaluable dependency inputs |
| Adapter/policy/user expansion | Medium per entity; user correlation potentially large | First slice plus entity-specific coverage |
| Durable audit guarantees | Separate substantial project | Explicit reliability/security requirements |

No calendar estimate until Phase 0 identifies missing producers and accepted
reliability requirements.

Safe parallel work now: inventory, schemas/fixtures, query/permission design and
UI response mocks. Coordinate production event emission, lifecycle hooks and
activation with maturity; use an isolated branch and review the integration
point before merging. No live deployment, retention change, credential change
or service restart is authorized by this plan.

**Recommended next action:** Phase 0 only, ending with the event and Audit
capability matrices and a bounded implementation proposal. Preserve the visa
history slice, then prioritize policy evolution and concrete-context potential
access as the next Audit capabilities. Do not promise historical replay or
exhaustive possible-access analysis before confirming retained inputs and
engine support.
