# Multiple trusted sources: source-qualified policy support

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

## Explicit subject links: required follow-up

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
