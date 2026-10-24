# Multiple trusted sources: current policy support

## Verdict: partly, not source-isolated authorization

At baseline `7ad56caca5ca08435ad46fa1c7637dc1a9ed2e50`, multiple trusted
sources can be registered and browsed, and Go **assertions** can compare their
snapshots. This does **not** establish that permission policies isolate source A
from source B. Permission evaluation uses an unqualified attribute-key map;
same-key claims from different sources replace one another. Do not represent
multiple source registration, a successful assertion, or the browser's selected
source as proof that a visa is restricted to that source.

No permission-language, persisted-policy or identity-linking semantics have
been changed by this investigation. Source-qualified authorization requires a
product/contract decision before implementation.

## Evidence trace

| Layer | Actual binding and evidence |
| --- | --- |
| Control Room permission editor | `policyclient.Attribute` contains `source` and `attribute`, but `loadPolicyAttributeMappings` in `policy.go` sets `source` to the **provider attribute name** before `->`, not a trusted source ID. Identical mappings across services are deduplicated. `policy_attributes.go` scans one configured LDAP container/base DN, not the selected source registry. |
| Editor analysis | `policyWorkspace.checkUnlocked` runs configured ZPLC with `--parse-only --lint --config`. The registry's selected source is not supplied. Record/source diagnostic ownership remains unchanged. |
| Policy-Service storage and compilation | `policy_store.go` versions source content/metadata in SQLite. `policy_staging.go:compileAndStagePolicyRecord` invokes ZPLC with the configured compiler configuration and signing key. Source registry entries are not compiled into source-qualified condition identities. `policy_test_api.go` delegates compiled permission testing to `zpt`; it is not an independent Go authorization evaluator. |
| Compiled contract | `zpr-common/zpr-policy/policy.capnp:AttrExpr` has only `key`, `op`, `value`. `TrustedService` has `serviceId`, `returnsAttrs`, and `identityAttrs`; the attribute mapping target can be a manually distinct ZPR key, but conditions do not carry `(organization_id, source_id)`. These sibling sources were inspected read-only because they define this workspace's compiler/runtime contract. |
| Rust permission evaluation | `libeval/src/actor.rs:Actor.attrs` is `HashMap<String, Attribute>`. `add_attribute` inserts by key only (last writer wins). `eval.rs:match_condition_to_actor` matches key/value/expiry, not `Attribute.source`. The latter is lifecycle provenance, not a condition selector. |
| Go assertion evaluation | `assertion_expressions.go:assertionExpressionDirectory` resolves `source("id")` by the configured `Name`/source ID in the current organization. Display name is not the selector. Legacy `ldap` works only without a named-snapshot map. Unqualified simple assertions use the explicit configured default snapshot, not an arbitrary first source. Cross-source current-person expressions look up the same textual subject in the other snapshot; there is no verified identity-linking relation. |
| Source registry and failure | `assertion_source_registry.go` validates organization, source ID and revision. Display edits preserve ID. Backend URL/base-DN edits may change what an unchanged ID resolves to. `assertion_sources.go:readDirectory` reads **all** organization-applicable sources and requires the default; any read failure aborts the whole assertion run. Single-source browsing remains independent. |
| Trusted-service identity resolution | `vs/src/trusted_services/manager.rs:get_attributes_for_actor` queries every configured service with the same `(identity key, identity value)` set. `file_attribute_store.rs` distinguishes identity keys within a source but adds no cross-source subject namespace. `connection_control.rs` combines successful returned claims; a failed provider blocks connection admission. Distinct keys from two providers can coexist, but their presence is not an explicit subject-linking decision. |
| Visa issuance and refresh | `connection_control.rs` calls `EvalContext::approve_connection` over combined claims. `actor_attributes.rs` refreshes by source revisions, adds returned attributes into the same key map and derives a single `user.zpr.authority` key; that authority is not provenance for every key. Revision-stale failures are indeterminate and deny requests, including requests whose policy might not need that provider. `visa_policy.rs` evaluates stored actors and `visa_mgr.rs` creates/rechecks visas. |
| Revocation and change events | Watchers flush a named source through the Admin API. Per-actor revision tracking narrows provider refreshes. However, `event_mgr.rs:TrustedServiceChange` carries no source ID, refreshes actors behind all live visas and calls a global `revalidate_visas` sweep. There is no per-grant source-dependency set; source-A-only reevaluation is not promised. |
| Control Room UI | Registry, browser selectors and snapshot responses retain organization/source IDs, isolate delayed reads and show independent provider failures. These are operator/read-path guarantees, not permission compiler or visa guarantees. Control Room remains independent of Simulator. |

## Requested scenario support matrix

“Partial” distinguishes real implemented behavior from the stronger requested
permission guarantee. Tests characterize the current contract, not approval of
its unsafe collision behavior.

| # | Scenario | Assertion/operator path | Permission/visa guarantee |
| --- | --- | --- | --- |
| 1 | Same org, two LDAPs, same names/different values; A-only policy | Explicit `from`/`source(...)` distinguishes 50000 vs 60000 for the same `alice`. | **No source-isolated guarantee.** Unqualified condition accepts an attribute from either source; same-key values overwrite. |
| 2 | Combine A and B attributes | Supported explicit comparison of the two snapshots, with same-text subject lookup. | **Partial:** disjoint mapped ZPR keys coexist and ordinary conditions can combine them; no first-class source-qualified condition or verified identity link. |
| 3 | A unavailable, B-only policy still works | B-only browsing works. **B-only assertion evaluation does not**: reads include A. | **Not supported as an independence guarantee:** failed admission/stale refresh is fail-closed for the whole actor, not only dependent policies. |
| 4 | Rename/edit cannot silently rebind | Display rename retains ID and old assertions keep working. Display name is not syntax. **Backend edit can rebind** an unchanged source ID. | No persisted provider-binding/version pin in a permission condition. Product rule needed for identity-changing edits. |
| 5 | Same subject in A/B not implicitly merged | Snapshots remain separate; an explicit cross-source expression uses the same subject string. | **Not guaranteed:** all stores receive the same identity set; claims from different sources populate one actor, including overwriting matching keys. |
| 6 | Bundled evaluation + company source coexist | Generic independent same-org sources coexist; browser and LDAPS tests prove this without Simulator. Actual organization scopes must match; this is not proof of an installer migration/linking lifecycle. | Multiple configured trusted services work with disjoint mappings; collision/subject/failure limitations still apply. No automatic bundled/company identity link is justified. |
| 7 | A change affects only A-based grants | Named flush + per-source revision checks are supported. | **Not supported for selective reevaluation:** global visa sweep; unchanged B may survive, but it is still rechecked. Cross-source key collisions also prevent dependable grant attribution. |

## Automated and visual evidence

- `multisource_policy_contract_test.go`: real authenticated registry operations,
  durable restart, two actual local LDAPS protocol listeners, independent
  snapshots, explicit cross-source evaluation, display rename, peer outage,
  recovery and accepted backend rebinding. This is a composed operator/assertion
  end-to-end test, **not** an LDAP-to-visa integration claim.
- The same file characterizes permission authoring catalog deduplication and
  proves that source snapshots do not union same-named subjects implicitly.
- `libeval/src/eval.rs:test_multisource_*`: existing compiled policy evaluated
  against source-stamped attributes; either source can satisfy an unqualified
  policy, and insertion order changes a same-key authorization result.
- `vs/src/trusted_services/manager.rs:test_multisource_*`: real file-backed stores,
  same `user.sub` in both sources, key collision, distinct-key coexistence and
  source-specific revision invalidation. Existing actor refresh and event tests
  exercise indeterminate failures, persistence, pruning and visa sweeps.
- `tests/browser/multisource-policy-contract.spec.mjs`: new desktop/tablet
  screenshots of company/evaluation source catalogs with the same `alice` and
  distinct salary values, scoped read requests, no cross-source display values,
  no horizontal overflow, and no Simulator requests. Browser API responses are
  fixtures; real LDAP behavior is tested separately in Go.
- Existing `source-registration.spec.mjs` and `control-room-boundary.spec.mjs`
  cover editing, delayed responses, credential-reference hygiene and unavailable
  production integrations without Simulator.

## Decisions required before implementing source-isolated permissions

1. **Binding contract:** choose immutable `(organization_id, source_id)` condition
   identities (compiler/schema/runtime/editor changes), or formally support
   operator-managed disjoint ZPR attribute namespaces with compiler validation
   rejecting ambiguous mappings. Do not invent a `source(...)` permission syntax
   merely because the assertion language has it.
2. **Unqualified policies:** choose reject-on-ambiguity, an explicit source binding
   at authoring/installation, or retained documented unqualified semantics.
   Early-release database migration is not required here, but policy compatibility
   and reinstall behavior still need an explicit decision.
3. **Subject correlation:** require explicit issuer/source-qualified subjects and
   approved links for cross-source joins, or intentionally authorize same-text
   subject correlation. A single mutable `user.zpr.authority` is insufficient
   to qualify all accumulated claims.
4. **Edits and availability:** define whether endpoint/base-DN/identity mapping
   changes create a new identity/version and invalidate policy/grants. Establish
   dependency-aware unknown/negation semantics before relaxing whole-actor
   failure closure; simply skipping an unavailable source can grant access.
5. **Selective reconciliation:** retain conservative global sweeps explicitly or
   introduce persisted per-grant source dependencies, including absence checks,
   before claiming only-A grants are re-evaluated.

Stop at this decision gate. No syntax, migration, fail-open shortcut, or implicit
identity merge has been introduced as a “fix”.
