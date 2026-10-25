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

## 8. Decision and acceptance gates

| Gate | Decision/evidence required |
|---|---|
| Identity | Canonical encoding, integrity binding, issuers, trust anchors and replay/audience semantics |
| Realm grants | Operation vocabulary, namespace allocation, delegation, transfer, concurrency and recovery |
| Composition | Import/export rules, mandatory restrictions, conflicts, cycles, bounds and multi-parent semantics |
| Source ownership | Canonical source namespace, declaration admission, binding updates and transport rotation |
| Access proof | Authenticated VS + principal context, field visibility, subject provenance and revocation |
| Lifecycle | Dependency model, authorization-safe caches, stream gaps, expiry and existing-grant treatment |
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

Do not implement an interim envelope credential or source-visibility bypass
merely to exercise these ideas. Current source integration and maturation
continue under their existing contracts until these gates are resolved.
