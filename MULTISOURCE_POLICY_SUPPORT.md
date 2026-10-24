# Multiple trusted sources: source-qualified policy support

## Current result

Trusted-service attributes now have a stable source-qualified key in the policy
catalog and Visa Service ingestion layer. For a source named `staff`, the key
`user.department` is exposed as `user.department__source__staff`. The marker
`__source__` uses identifier characters rather than ZPL operators or punctuation;
`AttrExpr.key` is already a text key, and the suffix remains a valid attribute-name
segment. The compiler's attribute parser is external to this worktree, so its
acceptance of this spelling remains an explicit pre-merge verification gap. Source
IDs must use ASCII letters, digits, `_` or `-`, and may not contain the reserved
marker; base mapped keys may not contain it either. This makes the suffix boundary
unambiguous while preserving the source's stable configured ID as the suffix.

`zpr-dashboard/cmd/zpr-web-dashboard/policy.go` now lists each trusted service's
source ID and qualified key, retains equal raw attribute mappings from different
sources instead of deduplicating them, and rejects mapped unqualified references
in analysis, policy tests and installation staging when the compiled config has
multiple sources. With exactly one source, the catalog provides both qualified
and legacy unqualified names. `vs/src/trusted_services/manager.rs` qualifies
provider returns before actor insertion; for multiple sources it emits qualified
keys only, so same-key claims cannot overwrite each other. For one configured
source it emits the qualified and legacy keys.

This is the key-namespacing compatibility layer requested for the existing
`AttrExpr(key, op, value)` contract. It does **not** complete the entire requested
source-aware visa model. In particular, an operator-selected default source for
multi-source policy config, compiler-native dependency metadata, approved
cross-source identity links, source-version binding/invalidation, and
source-dependency-aware availability/revalidation are not represented by the
current compiled schema. The existing whole-actor outage behavior remains
conservative/fail-closed; it can block unrelated grants. Global visa
revalidation remains the demo behavior.

## Full code trace

| Layer | Implemented behavior / remaining boundary |
| --- | --- |
| Control Room policy catalog | `zpr-dashboard/cmd/zpr-web-dashboard/policy.go:loadPolicyAttributeMappings` assigns the trusted-service section ID as `source`, shows `attribute__source__id`, and retains mappings per source. `policy_attributes.go` filters scanned provider names while preserving qualified keys. `static/app.js` renders the stable source label beside completion entries. |
| Analysis, policy test, installation | `policy.go:validateSourceQualifiedPolicy` rejects an unqualified mapped key in a multi-source config. It runs for editor analysis; `policy_test_api.go:handlePolicyTest` and `policy_staging.go:compileAndStagePolicyRecord` apply the same validation. It cannot see attribute names absent from configured mappings, and no compiler-native ambiguity diagnostics are available. |
| Policy-Service persistence/compile | Policy-Service is implemented in `zpr-dashboard/cmd/zpr-web-dashboard`; staging calls the configured external ZPLC. Source-qualified keys are present in the catalog and accepted as source text by this service, but the compiler's parser/runtime contract is not vendored here. |
| Compiled contract / compiler | Read-only external files: `/Volumes/Storage/dubnom/zipsquat/ZPR Docs/zpr-common/zpr-policy/policy.capnp` defines `AttrExpr { key, op, value }` and `TrustedService { serviceId, returnsAttrs, identityAttrs }`; `/Volumes/Storage/dubnom/zipsquat/ZPR Docs/.local-runtime/linux-tools/zplc` is the available compiler binary. The schema has no source-version/default-source/link/dependency fields. This worktree does not contain ZPLC source, and no external file was changed. |
| Rust ingestion/evaluation | `vs/src/trusted_services/manager.rs` namespaces mapped returns before they enter `Actor`; source IDs are validated in `factory.rs`. `libeval/src/actor.rs` remains a key-indexed map and `eval.rs` continues to evaluate ordinary text keys. Tests now prove same-key returns remain separate and an unqualified multi-source key does not match. |
| Assertions | `assertion_expressions.go` continues to resolve `source("id")` and `from` as before. These assertion data relationships support multiple sources but do not establish permission identity links. Same-text subject correlation in explicit cross-source assertions remains unchanged. |
| Subject/links | Runtime sources still receive the actor's common lookup-identity set and all successful claims join one actor. Namespaced keys prevent key overwrite, but they do not authorize only approved cross-source subject joins. No policy link model/config exists in the compiled contract. |
| Failure and visa lifecycle | Trusted-service admission/refresh errors remain fail-closed for the whole actor. This prevents fail-open behavior but does not permit unrelated source-B grants through a source-A outage. Event handling still invokes conservative global visa revalidation; per-grant source dependency sets are not persisted. |
| Source identity changes | Assertion source registry `assertion_source_registry.go` preserves source IDs while allowing endpoint/base-DN edits under the same ID. Compiled policy has no immutable source revision pin or dependent-grant invalidation. |
| Simulator boundary | No Simulator dependency or Simulator API/configuration was added. Existing Control Room boundary tests remain required. |

## Seven-scenario matrix

| # | Scenario | Current outcome after this change | Test / remaining work |
| --- | --- | --- | --- |
| 1 | Same organization, two sources, same names/different values; A-only policy | Qualified catalog/runtime keys distinguish A and B; unqualified mapped references are rejected by Control Room analysis/test/stage validation. | Go multi-source catalog validation plus Rust manager/evaluator tests. End-to-end compile/run against the external ZPLC binary is still required. |
| 2 | Combine A and B attributes | Both claims coexist under qualified keys; policy authors can explicitly reference each key and use existing key/op/value conditions. | Rust manager test asserts both colliding values survive. Cross-source subject link approval remains required before this can authorize the same person. |
| 3 | A unavailable, B-only policy still works | **Not implemented.** Current whole-actor failure closure remains; no fail-open source skipping was introduced. | Requires policy dependency metadata and three-valued missing/unavailable evaluation; see external contract gaps. |
| 4 | Rename/edit cannot silently rebind | Stable source ID is the suffix; display rename does not change the key. Endpoint/identity mapping edits under the same ID are still possible in the assertion registry/runtime config. | Needs source versioning and invalidation in Control Room, compiler contract, and visa/grant lifecycle. |
| 5 | Same subject in A/B not implicitly merged | Attribute-key collisions cannot overwrite. **Same-text identity fan-out and cross-source policy joins remain possible** because both sources receive shared lookup identities. | Needs approved link model plus subject-qualified identities enforced at lookup and policy evaluation. |
| 6 | Bundled evaluation + company source coexist | Configured distinct source IDs yield distinct keys; no Simulator dependency was introduced. | Needs explicit subject-link configuration before identity correlation; add installer/bootstrap lifecycle coverage. |
| 7 | A change affects only A-based grants | **Not implemented:** global conservative visa revalidation remains. | Per-grant selective revalidation is recorded as a pre-production shortcut in `SIMULATOR_AS_ZPR_CLIENT_PLAN.md` (X13). Needs persisted dependencies including absence/negation checks. |

## Test evidence in this worktree

- `zpr-dashboard/cmd/zpr-web-dashboard/multisource_policy_contract_test.go`
  verifies distinct catalog entries for duplicate provider mappings, qualified
  key completion, clear rejection of unqualified mapped policy references, and
  the single-source legacy alias. Existing composed registry/LDAPS/assertion
  checks continue to cover `from`/`source("id")`, peer outage, and source
  registration behavior; those are not visa authorization tests.
- `vs/src/trusted_services/manager.rs` verifies same-key returns from two stores
  remain as both qualified values and no unqualified key is emitted in a
  multi-source result.
- `libeval/src/eval.rs` replaces unsafe overwrite characterizations with tests
  that an explicit source-qualified condition matches only that key and two
  source-qualified same-name values remain available independent of insertion
  order.
- `tests/browser/multisource-policy-contract.spec.mjs` continues to verify
  source-isolated catalogs for identical subjects, scoped reads, no Simulator
  requests and no overflow. `tests/browser/dashboard.spec.mjs` adds a desktop and
  tablet policy-editor screenshot test for distinct source-qualified completions,
  source labels, and the ambiguity diagnostic. These browser responses are
  fixtures; external compiler integration remains a separate gate.

## Required external/compiler follow-up

1. At `/Volumes/Storage/dubnom/zipsquat/ZPR Docs/zpr-common/zpr-policy/policy.capnp`,
   add source revision/default-source, approved-link, and policy dependency
   contracts (including positive and negative/absence dependencies). Keep
   `AttrExpr.key/op/value` backwards-compatible or document a compiler version
   transition.
2. Update the ZPLC implementation (source checkout not present here; the local
   artifact is `/Volumes/Storage/dubnom/zipsquat/ZPR Docs/.local-runtime/linux-tools/zplc`)
   to validate qualified key syntax, resolve explicit defaults, and diagnose
   unqualified ambiguity during parse/lint/compile. Do not modify that binary or
   any external checkout from this worktree.
3. Update compiler-to-Rust generated policy types and VS evaluation to carry
   dependencies and approved subject links. Model an unavailable dependency as
   unknown; unknown/absence in allow and negated conditions must deny. Revalidate
   only grants dependent on changed sources after that contract is available.
4. Bind source endpoint/base-DN/identity-map versions immutably and invalidate
   dependent compiled policy/grants when a new source version is installed.
5. Add explicit operator UI for approving/revoking source-subject links and
   source defaults. No implicit same-text join should be enabled as a substitute.

## Visual plan and shortcut register

`SIMULATOR_AS_ZPR_CLIENT_PLAN.md` adds V16 for desktop/tablet source-qualified
policy completions and ambiguity diagnostics and V17 for link/outage/identity
version states. The same file records X13: the demo intentionally retains global
conservative visa revalidation; per-grant selective revalidation is required
before production. This change does not claim those pre-production gates passed.
