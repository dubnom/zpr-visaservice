# Envelope and realm worked design cases

**Deferred design exercises, not an implemented protocol or settled wire format.**
These cases apply the [envelope/realm requirements](ENVELOPE_REALM_SPEC.md).
Names such as `E-finance-1` are explanatory labels for exact immutable identities,
not an ID encoding. "Reject" below means a required future outcome, not a claim
that current VS implements the check.

Each case separates a required invariant from a decision still needed. Do not
turn a proposed resolution into production behavior without approving the
corresponding specification gate.

## 1. Namespace delegation without administrative overreach

**Given:** accepted anchor A grants Organization O creation/allocation rights
within `example.test`. O grants Finance F envelope issuance within
`finance.example.test`, with no further delegation or recovery power.

**Attempts and required outcomes:**

- F issues an envelope for a resource inside its grant: eligible for validation,
  not automatically authorized without policy and other dependencies.
- F issues for `hr.example.test`: reject excess scope.
- F delegates its issuance right to another team: reject absent regrant authority.
- F changes DNS records and declares itself administrator of O: reject; DNS
  control does not supply realm administration.
- O takes over F through namespace ancestry alone: reject unless O's accepted
  grant chain explicitly includes that recovery power.

**Decision exposed:** define "inside scope" independently of string-prefix
matching: DNS label normalization, delegation boundaries, service identities and
resource selectors need exact semantics. Specify whether namespace ownership
and administration are separate grants.

## 2. Content reuse is not a grant

**Given:** `E-parent-1` contains reusable `Employee` and `BusinessHours` defines.
`E-finance-1` has a ceiling of TCP 443 to the finance report service.

**Attempts and required outcomes:**

- Import exact allowed definitions, then reference them within the finance
  policy: retain parent provenance and the child's ID and ceiling.
- Import a parent rule permitting TCP 22 or a different service: it cannot
  enlarge the child's authority.
- Copy the parent's ID into a child field or alias: it cannot replace the
  child's canonical identity.
- Omit a mandatory authority restriction while importing a convenient define:
  reject if the resulting child drops an applicable restriction.

**Decision exposed:** distinguish exportable content from mandatory constraints.
An intersection-style ceiling invariant does not itself define how policy
rules, diagnostic assertions and enforcing assertions combine.

## 3. Two parents with conflicting definitions

**Given:** `E-a-1` defines `Employee` using HR source S1. `E-b-1` defines
`Employee` using contractor source S2. A child imports both without qualification.

**Required outcome:** reject ambiguity; swapping import order must not change
the decision. Neither spelling nor last-writer-wins may silently select S1/S2.
An explicit, permitted qualification may disambiguate authoring, but cannot
rebind either source identity or discard mandatory restrictions.

**Decision exposed:** exact import qualification syntax, duplicate canonical
definition equivalence and export visibility. Identical display names are not
evidence of identical authority.

## 4. Diamonds, cycles and resource bounds

**Given:** parents A and B both import exact `E-common-1`. Child C imports A/B.

**Required outcomes:**

- Preserve the common dependency's exact identity and both provenance paths.
  Reaching it twice must not double its rights or turn assertions into grants.
- Reject A -> B -> A, self-import, unresolved IDs and content/ID mismatches.
- Apply bounded expansion before exhausting resources, including broad
  acyclic graphs, not just recursion depth.

**Decision exposed:** whether identical imports deduplicate, how diagnostics
report the diamond, and exact depth/fanout/expanded-size limits. No numeric limit
is chosen by this example.

## 5. Approved source reuse versus source rebind

**Given:** canonical source S-company is an approved company directory
authority. Finance and Engineering may reference it within their grants.

**Attempts and required outcomes:**

- Both reference S-company: no duplicate authority registration is needed merely
  because policies differ, but source access still requires authorization.
- Engineering creates a local `company` alias pointing at another authority:
  it cannot override S-company.
- A source operator rotates the TLS endpoint/certificate: allow only through an
  approved binding-update process preserving identity and audit.
- A policy author changes the source's endpoint to their own server: reject
  absent independent binding authority.

**Decision exposed:** source identity namespace/ownership, transport-update
authorization and cache invalidation. Whether parent envelopes may export
source bindings remains open; merely importing one cannot grant visibility.

## 6. Valid caller, unauthorized principal

**Given:** VS V is transport-authenticated. Policy principal P-finance may read
`department` for approved subjects from S-company but may not read `salary`.

**Required outcomes:**

- V + valid scoped P-finance proof + approved subject/operation: return only
  source-authorized data.
- V + no principal proof, wrong namespace, expired grant or requested salary:
  no unauthorized disclosure.
- Valid P-finance proof presented by unauthorized V2: reject.
- A subject UUID or envelope ID in place of proof: reject.
- Broad fetch by V followed only by local salary filtering: not acceptable as
  source-side delegated-access enforcement.

**Decision exposed:** proof format, audience/caller binding, source-side
operation/field authorization and rejection semantics. Current `rest/1` has no
such principal proof; do not add an unsigned request field as an interim fix.

## 7. Cross-principal cache reuse and negative evidence

**Given:** P-admin may see a field P-finance cannot. Both ask about the same UUID.

**Required outcomes:**

- A cache entry obtained for P-admin cannot disclose that field to P-finance.
- A valid cached value must not bypass expired/revoked principal or VS authority.
- A negative observation supporting "not a contractor" has its own dependency
  freshness; an update to another subject cannot renew it.
- A field-visibility change invalidates affected authorized views even when the
  underlying value remains unchanged.

**Decision exposed:** canonical authorization-context cache key, dependency
provenance and what source evidence permits safe sharing. Do not assume a key
of `(source ID, subject UUID)` is sufficient.

## 8. Parent revocation, succession and publication races

**Given:** a child pins `E-parent-1`; `E-parent-2` is later published.

**Required outcomes:**

- A floating "latest" alias cannot silently replace the pinned parent.
- Authorized succession creates new immutable content/identity, not a rewrite
  of `E-parent-1`.
- A delayed evaluation started under old authority cannot acquire new validity
  by resampling a newer dependency at publication time.
- Relevant revocation must not silently preserve authorization through cache
  reuse, process restart or old artifact replay.

**Decision exposed:** whether particular parent revocations invalidate imported
content, existing child artifacts or only new issuance; rules for already-issued
visas; enforcement delays and outage behavior. The conservative requirement is
no use beyond valid dependencies, but exact operational semantics need approval.

## 9. Authority transfer and emergency recovery

**Given:** O explicitly grants a recovery principal R a bounded emergency right.
F's ordinary administrator becomes unavailable.

**Required outcomes:**

- R may use only the granted recovery operations within scope and validity.
- An authenticated executive, DNS administrator or parent realm without the
  grant cannot manufacture recovery authority.
- Transfer must not silently transfer reusable credentials or exceed the
  transferor's scope.
- Concurrent transfer/revocation attempts require explicit preconditions and
  auditable outcomes, not silent last-writer-wins authority changes.

**Decision exposed:** recovery evidence/quorum, notification, delay, conflicts,
credential replacement and explicit revocation of prior administrative power.
These are design choices, not an automatic parent takeover mechanism.

## 10. Source update stream gap and restored state

**Given:** a decision depends on directory data, an identity link and a delegated
grant. The source stream disconnects; later, a backup predating revocation is
restored.

**Required outcomes:**

- Stream events are hints, not authority to broadcast data to every subscriber.
- Resync must preserve per-principal authorization and negative dependencies.
- Old backup/cache state cannot create fresh authority or erase the effect of
  a known revocation.
- An unchanged subject value does not renew the expired link/grant.
- Recovery must explicitly establish required current dependencies; it cannot
  report healthy merely because disk state parses.

**Decision exposed:** continuity evidence, permitted stale operation, recovery
trust/anti-rollback, dependency deadlines and offline policy. Existing offline
subject-link checkpoint/publication foundations are not a complete solution to
this broader envelope problem.

## 11. Workflows inbox, bulk decisions and concurrent change

**Given:** operator R can independently review items A and B in organization O.
R submitted item C and cannot approve it. A publication item D has changed since
R last acknowledged it, but its next operation is separately authorized signing,
not generic approval.

**Required outcomes:**

- A and B appear under "Needs my decision"; C is visible as R's submission only
  while current read authority allows it. D contributes a change indication
  without being presented as an approval request.
- R selects the displayed versions of A and B. B changes before execution:
  A may commit, B must report a conflict, and no batch-success or rollback claim
  may conceal the partial result.
- A committed decision with a lost reply is reported as unknown until owner
  readback/retry resolves it. Repeating the entire batch with fresh operation
  IDs must not create new decisions.
- Selecting C cannot bypass independent-review rules. Reading D or approving
  another item cannot sign, publish or enforce D.
- R acknowledges the displayed version of D while a newer version arrives:
  the newer version remains unacknowledged.
- Switching organization or signing in as another principal clears selections
  and rejects old in-flight responses. Badge counts do not disclose O's items
  to another organization.
- An unavailable source produces an incomplete/stale inbox indication, not
  an empty successful queue.
- A declined item retains the owner's durable reason/audit decision; merely
  marking an item seen does not decline it or revoke existing access.

**Decisions exposed:** owner-supported decline operations, current-authority
eligibility, normalized pagination/change cursors, durable acknowledgment
ownership, batch bounds and uncertain-outcome recovery. The
[Workflows requirements](ENVELOPE_REALM_SPEC.md#8-workflows-shared-operator-inbox)
do not themselves implement those APIs or resolve deferred envelope authority.

## Decision queue and future test mapping

| Priority | Decision to resolve | Cases |
|---|---|---|
| First | Canonical IDs, integrity, trust anchors and realm scope/grant vocabulary | 1, 2, 8, 9 |
| First | Mandatory constraints versus optional imported content; composition conflict rules | 2, 3, 4 |
| Next | Source identity ownership and separately authorized transport updates | 3, 5 |
| Next | Authenticated VS/principal/subject separation and source field authorization | 5, 6 |
| Next | Dependency-aware caches, expiry, revocation, stream and restore semantics | 7, 8, 10 |
| Before enabling imports | Import syntax, export permissions, graph bounds and provenance | 2, 3, 4, 8 |
| Before Workflows runtime | Owner decision APIs, user involvement, change acknowledgment and safe bulk-action semantics | 11 |

These examples can later become compiler/runtime/provider acceptance fixtures,
but must first bind to an approved protocol and precise outcomes. They do not
justify an implementation branch while the underlying decisions remain open.
