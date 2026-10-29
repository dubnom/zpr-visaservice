# Envelopes, realms and delegated authority

**Status: deferred design requirements, not an implemented protocol.**
This document consolidates the agreed envelope/realm model and identifies
decisions required before implementation. It does not change current policy,
source registration, subject-link review or `rest/1` behavior. No JSON examples
in other documents should be interpreted as this model's final wire format.

The earlier [architecture discussion](MULTISOURCE_POLICY_SUPPORT.md#realm-authority-and-parent-envelope-composition)
records the origin of these requirements. The
[trusted-source integration guide](TRUSTED_SOURCE_INTEGRATION.md) describes
what is implemented today.

The [worked design cases](ENVELOPE_REALM_EXAMPLES.md) exercise these invariants,
expose unresolved choices and map future acceptance scenarios without choosing
a wire format.

## 1. Terms and identity

| Term | Meaning |
|---|---|
| Realm | Administrative authority scope: who may create, own, edit, issue, delegate, transfer, revoke or recover within specified resources/namespaces |
| Realm revision | An immutable statement of realm configuration/constraints; changes produce a successor, not an in-place rewrite |
| Envelope | Immutable identity-bound rights/constraints combined with policy, assertions, defines and exact references |
| Canonical envelope ID | The identity of that immutable envelope; never an overridable field, imported alias or possession-based credential |
| Grant | Authenticated evidence authorizing a subject to exercise specific operations within a bounded scope |
| Trusted-source binding | Canonical source identity and approved authority context, distinct from endpoint location and transport credentials |
| Parent import | Explicit reuse of selected immutable content; not administrative delegation or identity inheritance |
| Alias/define | An authoring/display convenience resolved to canonical references before enforcement |
| Trust anchor | Explicitly accepted root authority from which valid grants derive |

The canonical envelope ID is **the ID**. Policy content, imported parents,
aliases, display names and namespace labels cannot replace it. An edit to its
immutable authority context or content creates a new envelope identity.
Expiration, revocation and authorized succession are separate lifecycle events;
immutability does not make a revoked artifact usable.

The ID encoding, digest/signature construction, canonical serialization and
relationship between realm identity and immutable realm revisions remain open.
Do not assume an ID alone proves integrity, issuer authority or current validity.
The implementation must verify the artifact-to-ID binding independently.

## 2. Administrative authority: who grants the realm?

Every administrative action requires an explicit grant whose issuer was
authorized to grant that operation over that scope. Follow the chain until an
explicitly accepted trust anchor is reached. A self-asserted root, DNS ownership,
an authenticated identity or possession of an envelope does not terminate this
verification by itself.

The operation vocabulary must distinguish at least:

- Create a realm within an allocated scope.
- Administer/edit a realm or publish a successor configuration.
- Allocate and delegate namespaces, including DNS namespace resources.
- Issue envelopes and authorize policy publication.
- Delegate particular administrative rights, with explicit further-delegation
  limits.
- Transfer ownership/administration.
- Suspend or revoke grants, envelopes and delegated scopes.
- Recover administration or execute emergency takeover.

These operations must not imply one another silently. Namespace ancestry is
not an implicit recovery or takeover grant. Ownership must have explicit
semantics rather than functioning as an undocumented universal permission.

A grant's authenticated content must bind its issuer and subject, resource
scope, permitted operations, constraints, validity and delegation limits.
The eventual design must also identify its authority chain and revocation/
succession context. Human users, service identities and organization principals
must remain distinguishable, including issuer-qualified identities where used.

Delegation is attenuating: the recipient cannot exercise or regrant more than
the grantor may delegate. Child scopes must be contained by the accepted parent
allocation; DNS control proves only what the chosen DNS validation mechanism
actually establishes. Overlapping allocations, transfers and concurrent
administrative updates need explicit conflict/precondition rules.

Recovery requires a pre-authorized path with defined scope, evidence and audit.
Deleting or losing a parent must not manufacture a new trust anchor.

## 3. Envelope contents and immutable ceiling

An envelope must bind enough information to verify:

1. Its canonical identity and issuer/realm authority context.
2. Resource/namespace scope and maximum permitted operations.
3. All applicable rights, prohibitions and constraints.
4. Policy content and exact policy-language/version interpretation.
5. Assertions and their enforcement versus diagnostic role.
6. Defines/aliases and their canonical resolution.
7. Trusted-source references, identity provenance and binding constraints.
8. Exact parent imports and dependency provenance.
9. Integrity/authentication and validity/lifecycle dependencies.

This is a logical inventory, not a prescribed storage schema. Resource limits,
audiences, environment restrictions, delegation depth and authorized data fields
belong in constraints when applicable; which are mandatory awaits the protocol
design. Secret material and reusable user credentials should not be embedded
as ordinary authoring conveniences.

The identity-bound scope is a ceiling on every evaluation. Policy, assertions
and imports cannot expand it. Passing the ceiling check is necessary but not
sufficient: current grants, source permissions, applicable restrictions and
dependency validity must also authorize the action.

A useful invariant, **not a settled policy-combination algorithm**, is:

```text
effective authority is contained in:
  envelope ceiling
  AND valid administrative/delegation grants
  AND applicable policy and mandatory constraints
  AND source-authorized operations/data visibility
  AND valid identity and lifecycle dependencies
```

Diagnostic assertions cannot become authorization grants. Enforcement
assertions cannot disappear through importing, renaming or recompilation.
Defines simplify authoring but carry no independent security identity.

## 4. Source identity and reuse

An existing trusted-source identity cannot be shadowed or rebound by a policy,
envelope or imported definition. A display-name collision must not silently
select a different source. Equivalent canonical references may be reused;
conflicting authority bindings must fail explicitly.

Changing an endpoint or rotating credentials is not necessarily creating a
new source authority, but it requires an authorized binding-update mechanism.
The protocol must distinguish operational transport updates from source
identity replacement and preserve audit/provenance through either event.
No source-ID namespace or update protocol is chosen here.

A policy within an authorized namespace should be able to reference/declaratively
use an approved source ID without duplicating authority registration. That
reference does not grant source field visibility, choose arbitrary credentials
or override an existing binding. How declarations are admitted and source
identity ownership is established remains a design gate.

## 5. Parent-envelope composition

An envelope may explicitly import assertions and defines from one or more
parents. Potential trusted-source binding imports remain an open decision.
It does **not** import or override parent IDs, principals or credentials.
Administrative delegation and content reuse are separate relationships.

Requirements before composition is enabled:

- Pin exact immutable parent identities/revisions; floating aliases are not
  sufficient authority references.
- Select imported content explicitly and retain its origin, original role and
  transitive dependency provenance.
- Reject cycles, unresolved references and identity/content mismatches.
  Bound depth, fanout and expanded size before consuming unbounded work.
- Never union parent permissions into a broader child grant. The child's own
  ceiling and grants apply regardless of the number of imports.
- Do not discard applicable mandatory inherited restrictions by selecting only
  convenient definitions. Content export and mandatory authority constraints
  must be distinguished.
- Reject conflicting definitions, incompatible constraints and conflicting
  source bindings explicitly. Neither import order nor a more permissive
  fallback is an acceptable conflict-resolution rule.
- Resolve aliases to canonical references before compilation/enforcement.
  Renaming a define must not change identity or source authority.
- Define the effect of parent/dependency expiration, revocation and succession
  on existing compiled policy, cached results and issued grants.

An immutable child ID alone does not prevent rights expansion. Non-expansion
requires verified ceilings, grant checks and composition semantics at both
publication and enforcement.

**Still open:** exact multi-parent policy/assertion combination, export
visibility, identical-definition deduplication, source-binding inheritance and
whether/how historical imported content survives particular parent lifecycle
events. Until resolved, ambiguity must block publication/use rather than select
a permissive interpretation. This is a design requirement, not a new runtime
failure mode added by this document.

## 6. Trusted-source access has two authorization contexts

Future source access must authenticate both:

1. The calling VS service identity.
2. The policy-associated principal or delegated authority for the specific
   envelope/policy/namespace scope.

Neither substitutes for the other. The lookup subject is a third concept: the
person/device about whom information is requested. A subject identity or
policy/envelope ID is not a delegation credential.

The source owns authorization of operations and disclosed fields. A privileged
broad fetch followed solely by VS filtering does not implement delegated
source-side visibility. VS may request narrower fields and enforce additional
local restrictions, but cannot broaden source permissions. No requirement here
implies forwarding reusable user secrets.

Define issuance, audience, verification, attenuation, expiry and revocation of
the principal/delegation proof before extending the wire protocol. Bind it to
the canonical scope and authenticated caller, not to a display name or unsigned
request parameter.

## 7. Lifecycle, caching and invalidation

An evaluation can depend on multiple sources, identity links, grants, imported
envelopes and policy revisions. Track canonical provenance for positive values
**and absence/negative conditions**, including the authorized field projection.

Cache keys must include the relevant authorization context: data obtained under
a broad principal cannot be reused to disclose information to a narrower
delegate. An unchanged data value does not keep an expired grant valid.

Keep distinct bounds for data freshness, source access, identity links,
administrative delegation, policy validity and stream continuity. A decision
must not outlive a required dependency. An unrelated update cannot renew it.
Define conservative mixed-revision behavior rather than assuming cross-source
atomic snapshots.

Change events are invalidation hints, not authorization to disclose their
payload or fetch data. Subscriptions and delivered fields require source-side
authorization under the same contexts. Revocation, missed events, stream gaps,
resync and recovery must not silently retain formerly authorized access.
Specify treatment of already-issued visas and asynchronous enforcement rather
than promising instant revocation without evidence.

## 8. Workflows: shared operator inbox

**Direction agreed 2026-10-28; requirements only, runtime implementation deferred.**
Use **Workflows** as the provisional Control Room navigation name for a shared
operator inbox. Integrate this with envelope/realm authority and the existing
domain workflows; do not create a second approval, grant or deployment engine.

The initial scope includes budget approvals, subject-link review/publication
and adapter enrollment approvals. Future envelope issuance, delegated authority
and publication requests can use the same presentation once their contracts
are approved. Not every workflow item is an envelope, and existing records must
not be relabeled as implemented envelopes.

### 8.1 Ownership and integration

Workflows aggregates authorized summaries and dispatches explicit actions to
the owning production service. That service remains authoritative for state,
permissions, independent-review rules, validity, revision preconditions,
idempotency and audit. Inbox state is not authority evidence.

Keep the following separate:

| Concern | Owner / meaning |
|---|---|
| Rights and permitted administrative operations | Current domain authorization; future accepted envelope/realm grant chains |
| Review and approval | The domain's exact immutable proposal and decision lifecycle |
| Publication and delivery | Separately authorized domain operations with their own evidence |
| Installation and enforcement | Observed recipient state, not inferred from approval or receipt |
| Inbox presentation and change acknowledgment | Workflows, without changing any authority or domain record |

An eventual envelope-aware action must bind the canonical envelope identity,
exact revision/content, realm and relevant authority dependencies. Possession
of an item, inbox visibility, selection, or a previous approval cannot confer
permission. Do not add a generic envelope credential or alternative trust chain
to support this UI.

Control Room obtains workflow items through production service contracts.
Neither it nor Control-Service may query Simulator plans, profiles, sessions
or workloads to populate the inbox. A Simulator client may submit or reference
an execution-scope request through an explicitly approved production contract.

### 8.2 User involvement and change indication

Show a Workflows navigation indicator when there are unacknowledged changes
in workflows involving the signed-in user. Involvement initially means:

- The user submitted the item.
- The user took a recorded action on the item.
- The user is currently authorized and eligible to review the item.

Use verified issuer/subject identities and organization scope, not matching
display names or email addresses. Review eligibility includes independence
rules and current authority, not merely possession of a broad role. Participation
does not override current read permissions; losing visibility must remove the
item and its contribution to counts.

The indicator and inbox must distinguish **changed since acknowledgment** from
**awaiting my decision**. A completed publication can be a relevant change
without requiring a new approval. Define semantic changes from authoritative
versions/events, not polling timestamps or a newly sampled health timestamp.

Acknowledgment requirements:

- Maintain acknowledgment independently from workflow decisions.
- Opening or polling the inbox must not silently acknowledge unseen items.
- Provide explicit acknowledgment, such as "Mark viewed items as seen", bound
  to the exact versions actually displayed. A concurrent newer change stays new.
- Keep acknowledgment isolated by verified principal and organization, durable
  across reload/restart; decide cross-device behavior before implementation.
- Clear browser-held item data, selections and in-flight results on logout,
  identity or organization change. Reject stale asynchronous responses.
- Missing events, incomplete pagination or an unavailable source must produce
  an incomplete/stale indicator, not a false zero or "all caught up".
- Do not leak other organizations' activity through badge counts or events.

For first use, surface authorized existing pending items as requiring attention;
do not manufacture historical change timestamps. The exact initial-history
window and count limits require an explicit contract.

### 8.3 Inbox page and multi-selection

Provide a page of workflow items where the operator can inspect and select one
or more exact items, then choose **Approve** or **Decline** when supported.

Each row/detail should expose authorized information sufficient to identify:

- Workflow type, organization and stable item ID.
- Subject/target, requester and requested operation.
- Current stage, latest meaningful change, and the user's involvement.
- Exact proposal revision/digest, validity and relevant dependency references.
- Available actions and why an action is unavailable.
- A detail/evidence link to the owning workflow, plus recorded decision and
  publication/enforcement state where available.

Provide filters for "Needs my decision", "Changed", "Submitted by me",
"Participated", workflow type and status, within authorized organizations.
Selection must refer to explicit item IDs and displayed versions. "Select all"
means the displayed page unless an explicit bounded cross-page selection
contract is implemented; never imply unseen items are included.

Before submitting a decision, show the selected items, exact requested action,
review evidence and a required reason. A shared reason may be applied to all
selected items if each owner permits it. Require explicit confirmation; do not
treat selecting a checkbox as completed review.

Do not silently skip incompatible selections. Explain which items are not
eligible and require their removal or an explicit eligible-only selection.
Approval of an item must not implicitly sign, publish, provision credentials,
deploy or activate anything unless that exact domain action explicitly defines
and authorizes the combined operation.

### 8.4 Decision execution and failures

Bulk decisions are orchestration of individually authorized owner operations,
not an all-or-nothing transaction across services.

- Reauthorize each item at execution, including current independent-review and
  future envelope/realm authority checks.
- Bind each decision to its reviewed immutable content and expected version.
  Concurrent edits, revocations or dependency changes must conflict or be
  handled under the owner's approved semantics, never approve replacement
  content silently.
- Preserve owner-specific audit and retry semantics, with bounded batch size
  and concurrency.
- Return per-item outcomes: committed, failed, conflicted, unavailable,
  not attempted, or outcome unknown. A transport timeout is not proof of failure.
- Retain exact operation/retry references for readback after an uncertain result;
  never replay a whole batch under new identities or against newer revisions.
- Distinguish cancellation of unsent operations from actions already committed.
  No false rollback or batch-success banner after partial completion.
- A durable decline is a domain decision with a reason and audit record.
  Hiding, dismissing or acknowledging a row does not decline it.

### 8.5 Current domain gaps and implementation gates

Historical inspection for the 2026-10-29 Workflows proposal used the former
audit-app worktree (since consolidated into main). The table records that
inspection, not a fresh main capability audit. Newer narrow Approvals UI work
must be reconciled against these broader requirements before treating any gap
as closed:

| Domain | Existing behavior | Gap before uniform inbox actions |
|---|---|---|
| Budget Authority | Direct exact-scope approval and revocation | No pending-request queue; define authenticated submission, read visibility and durable decline before offering queued decisions |
| Subject-link review | Immutable proposals and independent approval | Define a durable decline operation, permission and race/retry semantics; do not equate approval rejection errors with a recorded decline |
| Subject-link publication | Promotion/revocation intent, candidate capture, signing, pull delivery and receipts | Keep stages and permissions explicit; generic approval must not become implicit signing or delivery |
| Adapter enrollment | Existing invitation approval/rejection and registry browsing | Map owner eligibility, expiry and decision evidence; approval is not credential issuance or live connectivity |

These are not a full API audit. Domain adapters must declare their supported
read/action capabilities and fail explicitly when unconfigured. A workflow with
no supported decision action may appear as informational or link to its
specialized action, but must not show a pretend Approve/Decline control.

Before runtime work:

1. Inventory authoritative domain list/read/decision/event APIs and their
   independent backend authorization, including current in-progress changes.
2. Resolve the domain gaps above and the applicable envelope authority gates;
   approve required extensions rather than inventing generic semantics.
3. Specify the normalized inbox contract, involvement rules, event/change
   cursors, pagination, acknowledgment ownership and per-item decision results.
4. Implement contract tests for organization/principal isolation, self-review,
   stale selection, duplicate decisions, partial batches, lost replies, missed
   events, acknowledgment races and logout/account switching.
5. Integrate the Workflows page and navigation indicator, then qualify every
   enabled domain with Simulator unavailable.

## 9. Decision and acceptance gates

| Gate | Decision/evidence required |
|---|---|
| Identity | Canonical encoding, integrity binding, issuers, trust anchors and replay/audience semantics |
| Realm grants | Operation vocabulary, namespace allocation, delegation, transfer, concurrency and recovery |
| Composition | Import/export rules, mandatory restrictions, conflicts, cycles, bounds and multi-parent semantics |
| Source ownership | Canonical source namespace, declaration admission, binding updates and transport rotation |
| Access proof | Authenticated VS + principal context, field visibility, subject provenance and revocation |
| Lifecycle | Dependency model, authorization-safe caches, stream gaps, expiry and existing-grant treatment |
| Workflows | Owner APIs, supported decisions, user-scoped change/acknowledgment contract and individually authorized bulk actions |
| Rollout | Versioning, compiler/runtime agreement, explicit feature gate and migration boundaries |

Acceptance must include:

- Rejection of forged/colliding IDs, unauthorized issuance and unaccepted roots.
- Grant chains that allow valid attenuation and reject excess scope,
  expired/revoked issuers and unauthorized further delegation.
- DNS/identity authentication without the required administrative grant.
- Transfer/recovery attempts with and without explicit authority.
- Parent import chains, diamonds, cycles, conflicting defines and incompatible
  restrictions; import order must not affect authority.
- A permissive imported policy under a restrictive child ceiling.
- Source rebind attempts through aliases/imports and authorized transport-only
  changes that preserve source identity.
- Valid VS with unauthorized principal, valid principal with unauthorized VS,
  wrong scope/audience, denied fields and cross-principal cache reuse.
- Parent/grant/source/link revocation, negative-result expiry, stream gaps,
  delayed responses and restart, including already-issued grants.
- Operators and services functioning with Simulator unavailable.
- Workflow visibility and change counts isolated by current principal and scope,
  with partial/unavailable sources explicitly represented.
- Bulk review preserving independence, exact content, current authority,
  per-item outcomes and separate publication/enforcement stages.
- Acknowledgment races, stale browser responses and unknown decision outcomes
  not hiding changes or inventing successful decisions.

Do not implement an interim envelope credential or source-visibility bypass
merely to exercise these ideas. Current source integration and maturation
continue under their existing contracts until these gates are resolved.
