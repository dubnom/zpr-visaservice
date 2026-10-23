# Authorized scoped-source reconciliation — 24 cases repaired; full acceptance still blocked

**Branch:** `maturing/w4-source-contract-browser`. **Starting commit:** `430a46355dda8fd3df09f609b103ce86572ce038`.
**Scope:** only the 12 titles / 24 desktop+tablet IDs classified as `new_maturing_contract_drift` in the unchanged `failure-comparison.json` inventory were authorized for fixture/expectation updates.

## Contract reconciliation

- Each authorized case now installs a case-local production-source fixture. Multi-source catalogs register `staff` and `hr` through `/api/assertions`, with `organization_id` and stable `source_id`, and return only the requested source from `/api/assertions/source?source=…`; the old aggregate `assertionSource.sources` response is no longer used by those cases.
- All 24 cases assert that source reads are GETs carrying exactly the selected registered `source` query parameter, that registry entries belong to the selected organization, that each exercised source was read, and that no Simulator API was requested. Successful read fixtures bind both response identity fields to the selected organization/source.
- Insertion, replacement, undo/redo, analysis invalidation, author/save/evaluate, typed results, sorting, exact counts, change highlighting, keyboard behavior, filtering, read-only controls, last-good retention, navigation aborts, and stale-success/failure rejection remain asserted. No assertions were deleted; the source-level inventory records unchanged assertion counts in dashboard/workflow tests and three additional assertion occurrences in page-runtime tests, besides the new shared fixture checks.
- Single-source inserted rules explicitly qualify `great_lakes_ldap`. Evaluation fixtures reflect that qualification. Titles use canonical registered display metadata (`Great Lakes LDAP`); record-table selectors are scoped to `.trusted-source-results`, not the new source registry table.
- Delayed runtime reads intercept the scoped URL and preserve stable response identity. Because the title is now canonical registry metadata rather than provider response text, the stale-response discriminator is also asserted in Alice's displayed name (`Current directory`, never `Obsolete directory`), retaining both canonical-title assertions and stale-data rejection.
- No production files, dependency manifests, existing test titles, timeouts, skips, Simulator dependencies, or other test bodies were changed. The helper is invoked only from the authorized title definitions. No push, deployment, authenticated storage state, live opt-in, or live mutation was performed.

## Verified results

Pinned dependencies were restored only after `playwright: command not found`. Runs used the unchanged Playwright configuration, two workers, desktop and tablet, CI unset, and worktree-local `TMPDIR`.

| Validation | Passed | Failed | Skipped | Did not run | Exit |
| --- | ---: | ---: | ---: | ---: | ---: |
| All 12 authorized titles, desktop + tablet | 24 | 0 | 0 | 0 | 0 |
| All boundary + Trusted Sources GUI cases, desktop + tablet | 26 | 0 | 0 | 0 | 0 |
| Full suite / desktop | 731 | 50 | 32 | 3 | — |
| Full suite / tablet | 729 | 52 | 32 | 3 | — |
| Full suite / all | 1460 | 102 | 64 | 6 | 1 |

Unchanged strict audit: `python3 zpr-dashboard/scripts/check-control-room-boundary.py` **PASS**, exit 0. Boundary tests cover production providers available/unavailable and missing organization/source context, with Simulator unavailable and forbidden reverse requests.

All **24 exact authorized IDs** pass both the focused run and the full 1,632-case discovery. No product defect was exposed in the authorized cases.

### Exact-ID delta: additional outcome variation remains a blocker

**The requested “only 24 failing IDs changed” full-suite proof is NOT established.** The comparison verifier deliberately exits 1 and records `exactly_authorized_failure_delta: false`; this is not waived or described as a clean full-suite acceptance.

Compared with the original 120 maturing failures:

- 26 prior failures now pass: the 24 authorized cases and **two unmodified cases** listed below.
- 94 original failure IDs still fail.
- Eight additional IDs fail this run, producing **102 current failures** rather than the exact-delta expectation of 96.
- Four additional IDs were already recorded as exact beta-only failures. The other four share a failing title with the opposite project in the prior inventory; that is context, **not** exact-ID baseline reproduction or proof of a fix/regression. Three additional failures report `style-src-attr` CSP violations. None of these eight cases was edited, suppressed, or given a broader timeout; further investigation/fixes require separate authorization.

Unmodified previously failing IDs that pass this run:

| Project | ID | Test |
| --- | --- | --- |
| tablet | `cc3070f185481daa6f24-10bc69e8c681940787d4` | Policy runtime cancellation cannot release a newer pre-save evaluation request |
| desktop | `cc3070f185481daa6f24-f9d4bda33a2de34a9120` | Scenario JSON transport keeps failed creation unsaved and preserves explicit creation payloads |

Additional failing IDs outside the original 120-case maturing failure set:

| Project | ID | Test | Prior inventory context |
| --- | --- | --- | --- |
| desktop | `ba07e228bcb3aecf4d27-59cb9cddaa1f81692883` | World Map Fit reframes the current viewport after manual zoom and resize | Exact beta-only failure ID |
| tablet | `cc3070f185481daa6f24-086595e0f8ea7af946fc` | Gateway analysis scope rejects delayed error after editing back to the original source | Same title failed in opposite project; this exact ID not previously failing |
| tablet | `cc3070f185481daa6f24-3466b56273f4f71cdf83` | Policy runtime cancellation cannot release a newer pre-save fixtures request | Exact beta-only failure ID |
| tablet | `cc3070f185481daa6f24-5486e71cd297f6cb868d` | GUI shared programmatic edits keep policy completion and Scenario indentation undoable | Same title failed in opposite project; this exact ID not previously failing |
| tablet | `cc3070f185481daa6f24-823cc6ef42d23c7a4742` | Scenario JSON transport keeps failed creation unsaved and preserves explicit creation payloads | Exact beta-only failure ID |
| tablet | `cc3070f185481daa6f24-8d7dcd12e1769238f863` | Gateway analysis scope rejects delayed success after editing back to the original source | Exact beta-only failure ID |
| tablet | `cc3070f185481daa6f24-b0952dd568f4f7e00c45` | GUI raw scenario analysis ignores delayed errors for an edited source | Same title failed in opposite project; this exact ID not previously failing |
| desktop | `cc3070f185481daa6f24-f779cd8e5f8d23340312` | Policy runtime cancellation cannot release a newer pre-save fixtures request | Same title failed in opposite project; this exact ID not previously failing |

Remaining acceptance blockers include the 94 retained original failures, the eight additional outcomes above, the authenticated live-state gate, six serial-dependent cases that did not run, and explicit live/compiled-production-assets opt-ins not enabled in this run. Passing focused source-contract cases does not clear those gates or certify production readiness.

## Persistent evidence and reproduction

Evidence is committed in [`source-contract/`](source-contract/):

- `focused.json` / `focused.log`: exact 24-case source-contract selection, all passing.
- `boundary-trusted-source.json` / `.log`: all 26 boundary/trusted-source cases, all passing.
- `strict-audit.log`: unchanged strict boundary audit output.
- `full.json` / `full.log`: complete full-suite reporter evidence, including all remaining failure messages.
- `comparison.json`: raw-report SHA-256 values, counts by project, all authorized IDs, expected/actual remaining failure sets, every resolved/new failure ID, and prior inventory context without reclassification.
- `assertion-inventory.json`: before/after source-level assertion and test-definition counts.
- `compare-results.py`: reproducible exact-ID comparison against the immutable original inventory. From the repository root, run `python3 browser-triage/source-contract/compare-results.py`. It exits **1** for this observed non-exact full-suite delta, while independently requiring all 24 authorized IDs to pass and requiring unchanged full-suite discovery.

Browser commands are preserved verbatim in the logs; trailing line whitespace was normalized for repository hygiene. Raw JSON reporter evidence remains unmodified. Full suite: `TMPDIR="$PWD/../.browser-runtime" PLAYWRIGHT_JSON_OUTPUT_FILE="$PWD/../browser-triage/source-contract/full.json" npm run test:browser -- --workers=2 --reporter=json,list` from `zpr-dashboard`, after creating `.browser-runtime` in the worktree root. No failing case outside the explicit 24-case authorization was changed.

---

# Historical baseline triage (retained as originally recorded)

The following baseline report predates the authorized fixture repair above. Its counts, classifications, and statements that no tests were changed describe those historical runs, not this follow-up.

# Maturing browser triage — baseline established; acceptance remains blocked

**Compared HEAD:** `4fc460fa57398cb2f211f8984a58c444f82ec0c8` on `maturing/w4-browser-triage`.
**Beta:** `4627e4cd99f15e1ee227f788574ab5055ee7a353`, run in a disposable detached worktree named `maturing-w4-beta-baseline` under `zpr-visaservice.worktrees/`.

## Reproduction

Restored the pinned browser dependencies with `npm ci --no-audit --no-fund` after confirming `node_modules` was absent. Ran both unmodified suites with the same configuration, two workers, desktop and tablet, CI unset, and worktree-local `TMPDIR`:

```sh
PLAYWRIGHT_JSON_OUTPUT_FILE=WORKTREE_LOCAL_REPORT npm run test:browser -- --workers=2 --reporter=json,list
```

Both full runs exited 1. Each suite used two workers; the beta and maturing processes ran concurrently in separate worktrees. No live opt-in variables or authenticated storage state were supplied. No push, deployment, or live mutation was performed.

## Full-suite counts

| Revision / project | Passed | Failed | Skipped | Did not run | Total |
| --- | ---: | ---: | ---: | ---: | ---: |
| beta (all) | 1449 | 99 | 58 | 6 | 1612 |
| beta / desktop | 725 | 49 | 29 | 3 | 806 |
| beta / tablet | 724 | 50 | 29 | 3 | 806 |
| maturing (all) | 1442 | 120 | 64 | 6 | 1632 |
| maturing / desktop | 720 | 61 | 32 | 3 | 816 |
| maturing / tablet | 722 | 59 | 32 | 3 | 816 |

JSON reporter `stats.skipped` includes serial-dependent tests that did not run: beta 64 = 58 explicit skips + six not run; maturing 70 = 64 explicit skips + six not run. The CLI counts above keep those categories separate.

The earlier wave-three run reported 1,437 passed / 125 failed at this same maturing HEAD. This fresh run is 1,442 / 120 **without any implementation or test changes**. The five-case difference is not a fix or an acceptance claim.

Maturing discovers 20 additional project-specific cases, removes none: 14 pass and six production-assets cases explicitly skip without opt-in binaries. The separate compiled-fixture validation recorded by wave three remains separate evidence; it was not silently treated as part of these full-suite counts.

## Classification

| Classification of the 120 maturing failures | Cases | Desktop | Tablet |
| --- | ---: | ---: | ---: |
| New maturing browser-contract / fixture drift | 24 | 12 | 12 |
| Pre-existing exact-ID failures (excluding live gates) | 92 | 46 | 46 |
| Pre-existing CSP flakes reproduced on beta | 2 | 2 | 0 |
| Environment/live-only authenticated-state gate | 2 | 1 | 1 |

**Exact-ID delta:** 94 failures overlap both full runs; 26 fail only in the maturing full run; five fail only in beta. Of the 26, 24 are deliberate source-contract changes versus legacy browser fixtures/assertions, and two are existing intermittent CSP failures independently reproduced at the beta commit. Do not label all 26 as new product defects, and do not label all 120 pre-existing.

**Clear new implementation defects demonstrated: zero. No production code or tests were changed.** The 24 newly red contract-integration cases remain unresolved browser acceptance blockers, not silently waived tests. Reverting explicit source IDs, restoring aggregate reads, adding Simulator coupling, dropping assertions, or broadening timeouts would not be a justified fix. Repository guidance prohibits patching failing tests without explicit authorization; this task did not explicitly authorize changing their expected contracts. A follow-up can reconcile fixtures/expected source-qualified text with the intended contract while retaining every behavior assertion.

### New maturing contract drift: 24 cases / 12 titles

| Root cause | Cases |
| --- | ---: |
| Single-source assertion insertion now explicitly qualifies the selected stable source ID; older exact-text expectations remain unqualified (native/fallback undo-redo, author/evaluate, attribute insertion). | 8 |
| Multi-source tests populate only legacy aggregate `assertionSource.sources`, while maturing correctly selects only sources registered in `/api/assertions` and checks `organization_id` + `source_id` on source-specific reads. | 4 |
| Newly added configured-source registry uses the same table styling class as record tables; legacy strict `.trusted-source-table` locators now match two tables. Old delayed-read mocks also hard-code the previous URL without `?source=`. | 8 |
| Legacy heading expectations require `Trusted LDAP`; canonical selected-source display metadata is now `Great Lakes LDAP`. | 4 |

These are first-failing assertions, not proof that later assertions in those tests would pass after fixture reconciliation. Required new source-selection/coherency and boundary cases pass in the full maturing run.

#### Newly failing contract IDs

| Project | Test ID | File and test |
| --- | --- | --- |
| desktop | `cc3070f185481daa6f24-6b3f965b1f4c574c8191` | `dashboard.spec.mjs` — Control Room trusted source panel browses records read-only |
| desktop | `cc3070f185481daa6f24-593336b0d83a87619052` | `dashboard.spec.mjs` — GUI Trusted Source insertion participates in undo redo and clears obsolete analysis (fallback) |
| desktop | `cc3070f185481daa6f24-656050024b7bf260f03e` | `dashboard.spec.mjs` — GUI Trusted Source insertion participates in undo redo and clears obsolete analysis (native) |
| desktop | `cc3070f185481daa6f24-e4b279ab0e508a34ab4e` | `dashboard.spec.mjs` — Trusted Sources omits explanatory headings, counts, and the top table filter |
| desktop | `cc3070f185481daa6f24-ae702f7c34ac0fffcbef` | `dashboard.spec.mjs` — Trusted Sources tables sort, split group cn/objectClass and condense people rows |
| desktop | `cc3070f185481daa6f24-09a946de9ca589e430f4` | `dashboard.spec.mjs` — assertion attribute catalog inserts rules and renders typed comparisons |
| desktop | `cc3070f185481daa6f24-e22b5b1b272cf782ef5e` | `dashboard.spec.mjs` — assertion catalogs insert named sources and expression results show their text |
| desktop | `cc3070f185481daa6f24-c17167ec887b8a5a3ed7` | `dashboard.spec.mjs` — organization assertions author, save and evaluate without policy compilation |
| desktop | `cc3070f185481daa6f24-19e0300b0c17b298b8ab` | `page-runtime.mjs` — Shared page runtime rejects obsolete Trusted Sources failure after navigation and manual return |
| desktop | `cc3070f185481daa6f24-464329b99caf0dc48c9c` | `page-runtime.mjs` — Shared page runtime rejects obsolete Trusted Sources success after navigation and manual return |
| desktop | `cc3070f185481daa6f24-82d25565e53997a6a198` | `page-runtime.mjs` — Shared page runtime retains Trusted Sources directory data on a failed manual read |
| desktop | `cc3070f185481daa6f24-31f309487d00db30fc19` | `workflow-guidelines.mjs` — workflow assertion catalogs sort exact counts and retain source-qualified insertion |
| tablet | `cc3070f185481daa6f24-267af31642cc0f27df68` | `dashboard.spec.mjs` — Control Room trusted source panel browses records read-only |
| tablet | `cc3070f185481daa6f24-e6cb6736711ec8d9c586` | `dashboard.spec.mjs` — GUI Trusted Source insertion participates in undo redo and clears obsolete analysis (fallback) |
| tablet | `cc3070f185481daa6f24-0d1ba8271e47613a3420` | `dashboard.spec.mjs` — GUI Trusted Source insertion participates in undo redo and clears obsolete analysis (native) |
| tablet | `cc3070f185481daa6f24-ae8f1bf888fc8987fa16` | `dashboard.spec.mjs` — Trusted Sources omits explanatory headings, counts, and the top table filter |
| tablet | `cc3070f185481daa6f24-f6fa6d3928cf83e189b9` | `dashboard.spec.mjs` — Trusted Sources tables sort, split group cn/objectClass and condense people rows |
| tablet | `cc3070f185481daa6f24-8ae6f4c0df413ddb9fdd` | `dashboard.spec.mjs` — assertion attribute catalog inserts rules and renders typed comparisons |
| tablet | `cc3070f185481daa6f24-b47829e9da0136020166` | `dashboard.spec.mjs` — assertion catalogs insert named sources and expression results show their text |
| tablet | `cc3070f185481daa6f24-8e1c4472a8aac9b7f16e` | `dashboard.spec.mjs` — organization assertions author, save and evaluate without policy compilation |
| tablet | `cc3070f185481daa6f24-88758f3f43155b23afc3` | `page-runtime.mjs` — Shared page runtime rejects obsolete Trusted Sources failure after navigation and manual return |
| tablet | `cc3070f185481daa6f24-f61da4dd273ac070f28e` | `page-runtime.mjs` — Shared page runtime rejects obsolete Trusted Sources success after navigation and manual return |
| tablet | `cc3070f185481daa6f24-b68a524aaf81073c24fb` | `page-runtime.mjs` — Shared page runtime retains Trusted Sources directory data on a failed manual read |
| tablet | `cc3070f185481daa6f24-06f420addd63e15ac0e2` | `workflow-guidelines.mjs` — workflow assertion catalogs sort exact counts and retain source-qualified insertion |

### Pre-existing failures and root-cause evidence

- **28 Simulator reset cases:** both commits render the reset control hidden/disabled under the reset fixture; the first reset tests fail on `#scenario-reset` visibility. No Simulator dependency was added to Control Room to make these pass.
- **Two local enrollment cases + six serial dependents not run:** both commits fail the same first local TLS enrollment case. A serial rerun reproduces both failures. Capturing the Go fixture stdout through a worktree-local diagnostic wrapper identifies `browser_fixture_test.go:116: invalid enrollment input` during the approval decision, after the fixture emitted its ready response. It is **not** a missing `ZPR_LIVE_STORAGE_STATE` gate and is not proven to be a macOS socket-length problem. `go test ... -run ^TestEnrollmentBrowserFixture$ -v` with only `stop` passes startup; the approval path fails. No fixture source was edited.
- **Existing login callback cases:** the same `stable-login` IDs fail `net::ERR_CONNECTION_REFUSED` during local callback recovery. These are local fixture failures, not live named-user acceptance.
- **Existing Scenario/editor/navigation/layout cases:** exact same IDs fail on beta (missing scenario run selector, navigation order, wrap-control strictness, raw-editor expectations, font-size/layout and map framing assertions). The JSON inventory preserves each assertion error; no broad pre-existing claim is made for IDs that passed beta.
- **Two maturing-only CSP failures:** `style-src-attr` violations are nondeterministic in unchanged editor flows. The 18-case repeated focused beta run produces 17 passes / one failure for the exact desktop raw-scenario test; the corresponding maturing run produces 17 passes / one Gateway-error failure. An additional 12 repetitions of the exact desktop Gateway-success test on beta produce nine passes / three identical CSP failures. Thus both new-looking CSP IDs are independently beta-reproduced, not newly introduced maturing regressions. No CSP policy or browser-error assertion was weakened.

#### Pre-existing maturing failure IDs (94 cases)

| Project | Test ID | File and test |
| --- | --- | --- |
| desktop | `cc3070f185481daa6f24-baea28ee4f219238fff8` | `control-room-guidelines.mjs` — GUI guidelines Control Room visible text has an 11px minimum |
| desktop | `cc3070f185481daa6f24-5ad93d72b19f061b2b4c` | `dashboard.spec.mjs` — AI Scenario Apply supports exact raw Undo and Redo without saving or running |
| desktop | `cc3070f185481daa6f24-4501220f938bb506bb7f` | `dashboard.spec.mjs` — Control Room keeps Adapter Logs internal, Log Manager beneath it, and a Configuration group |
| desktop | `cc3070f185481daa6f24-b15e98cd55659ca9252b` | `dashboard.spec.mjs` — GUI Simulator navigation order survives page switching |
| desktop | `cc3070f185481daa6f24-63d67b533b6c7747814d` | `dashboard.spec.mjs` — GUI editor search controls follow the policy or assertion source and precede History |
| desktop | `cc3070f185481daa6f24-189faec3c73640bbe0d3` | `dashboard.spec.mjs` — GUI editor search reveals distant matches and prevents accidental form submission |
| desktop | `cc3070f185481daa6f24-6acb1ece38d5c9da6982` | `dashboard.spec.mjs` — GUI new scenarios display Untitled and cannot save that placeholder |
| desktop | `cc3070f185481daa6f24-b8da200424a2e27bd266` | `dashboard.spec.mjs` — GUI raw scenario analysis ignores delayed errors for an edited source |
| desktop | `cc3070f185481daa6f24-095b8d6b14555710e2fa` | `dashboard.spec.mjs` — GUI raw scenario assistant uses exact raw context and applies only unsaved proposals in either mode |
| desktop | `cc3070f185481daa6f24-f4c964ec83759fac99e2` | `dashboard.spec.mjs` — GUI raw scenario editor has safe syntax colors, aligned error gutter, and form round trips |
| desktop | `cc3070f185481daa6f24-d82304e3e477098fae38` | `dashboard.spec.mjs` — GUI raw scenario file import, download, and identity checks never save implicitly |
| desktop | `cc3070f185481daa6f24-c873ec8e035fb005a0ff` | `dashboard.spec.mjs` — GUI raw scenario source scroll tracks highlighting and preserves unavailable assistant state |
| desktop | `cc3070f185481daa6f24-1a249fb7505f44b8eed2` | `dashboard.spec.mjs` — GUI shared programmatic edits keep policy completion and Scenario indentation undoable |
| desktop | `cc3070f185481daa6f24-70b0243d0ac2c409fd21` | `dashboard.spec.mjs` — Gateway analysis scope rejects delayed success after editing back to the original source |
| desktop | `cc3070f185481daa6f24-f9d4bda33a2de34a9120` | `dashboard.spec.mjs` — Scenario JSON transport keeps failed creation unsaved and preserves explicit creation payloads |
| desktop | `cc3070f185481daa6f24-735df47c75d95d36a4b0` | `dashboard.spec.mjs` — Scenario machine log previews are compact, bounded and limited to running machine tracks |
| desktop | `cc3070f185481daa6f24-3c50430da25c0fb384df` | `dashboard.spec.mjs` — Shared editor-page maximize works for Config, Gateways, Scenarios, and Directory |
| desktop | `cc3070f185481daa6f24-b3c931dfc4af2f9db9e5` | `dashboard.spec.mjs` — Simulator Scenario polling aborts on navigation and ignores late catalog responses |
| desktop | `cc3070f185481daa6f24-0989c5d7663388055403` | `dashboard.spec.mjs` — Simulator Scenarios groups unfiled entries and keeps terminal run history |
| desktop | `cc3070f185481daa6f24-53d27d957011b2c94938` | `dashboard.spec.mjs` — Simulator catalog pollers report malformed JSON and recover without losing last-good data |
| desktop | `cc3070f185481daa6f24-03f8a9de71f2180ef749` | `dashboard.spec.mjs` — adding topology parents keeps existing nodes at the same screen position |
| desktop | `cc3070f185481daa6f24-b9f43e84c18819c62dd0` | `dashboard.spec.mjs` — login service recovery needs explicit sign-in after an error |
| desktop | `cc3070f185481daa6f24-e12bfb937b47516230b0` | `dashboard.spec.mjs` — visa refresh preserves current grants and resolves DNS labels |
| desktop | `f8e499d49f2323a0bbba-6076b0a3a7bb075088b0` | `enrollment-live.spec.mjs` — real TLS claim polls approval and resumes the same identity after restart |
| desktop | `cc3070f185481daa6f24-23ff08b8f14a50e9b3af` | `organization-browser.mjs` — Organization layout gives the graph screen height and keeps configuration accessible |
| desktop | `cc3070f185481daa6f24-8502cb515b84bce8bd28` | `organization-navigation.mjs` — Organization controls precede tabs and Simulator sidebar groups stay consistent across navigation |
| desktop | `cc3070f185481daa6f24-7217dba56022cca96b9a` | `remaining-code-editors.mjs` — CodeMirror Scenario source preserves typography, colors, wrapping, replacement and history |
| desktop | `cc3070f185481daa6f24-e37dab58d88497c2d9aa` | `scenario-browser.mjs` — Scenario browser File operations preserve draft and busy run guards |
| desktop | `cc3070f185481daa6f24-8f46baba583a42300cce` | `scenario-browser.mjs` — Scenario browser runs only the selected published file and preserves the active run when browsing |
| desktop | `cc3070f185481daa6f24-cd3aef99b53b077364f5` | `scenario-browser.mjs` — Scenario browser selects a newly saved draft and enables Run only after publishing |
| desktop | `cc3070f185481daa6f24-b643b53c3a04cb55492e` | `scenario-browser.mjs` — Scenario browser selects folder files without running and leaves full-width tracks |
| desktop | `cc3070f185481daa6f24-46818e971dcc329d6cd3` | `scenario-browser.mjs` — Scenario compact running steps disclose output but keep failures visible |
| desktop | `cc3070f185481daa6f24-7ba1207878d6e71334c3` | `scenario-browser.mjs` — Simulate separates monitoring execution from organization configuration and preserves run results |
| desktop | `82f2a698c8f2260b366b-f77bfee33331cc66bb51` | `simulator-reset-confirmation.spec.mjs` — Simulator reset completion survives the post-confirmation run refresh |
| desktop | `82f2a698c8f2260b366b-981865fc9980ac21e50c` | `simulator-reset-confirmation.spec.mjs` — Simulator reset discards outstanding intent evidence when the run changes |
| desktop | `82f2a698c8f2260b366b-0d96b126d435a681f16f` | `simulator-reset-confirmation.spec.mjs` — Simulator reset discards outstanding intent evidence when the selected organization changes |
| desktop | `82f2a698c8f2260b366b-d3928b6142e1a0bf3a77` | `simulator-reset-confirmation.spec.mjs` — Simulator reset fails closed for an unrecognized run state |
| desktop | `82f2a698c8f2260b366b-f703dc5232733dd862b3` | `simulator-reset-confirmation.spec.mjs` — Simulator reset intent expiry prevents confirmation |
| desktop | `82f2a698c8f2260b366b-adafe133ca029201e54a` | `simulator-reset-confirmation.spec.mjs` — Simulator reset invalidates confirmation when the named session is cleared |
| desktop | `82f2a698c8f2260b366b-dba197333403968b4e14` | `simulator-reset-confirmation.spec.mjs` — Simulator reset refuses stale generation at confirmation time |
| desktop | `82f2a698c8f2260b366b-49fcaf70da3a78f0de71` | `simulator-reset-confirmation.spec.mjs` — Simulator reset rejects intent target or actor mismatches before confirmation |
| desktop | `82f2a698c8f2260b366b-d5cc46a6d51670329e0f` | `simulator-reset-confirmation.spec.mjs` — Simulator reset rejects wrong configured status target and never falls back |
| desktop | `82f2a698c8f2260b366b-4a7e2c205c5b0ccead4c` | `simulator-reset-confirmation.spec.mjs` — Simulator reset requires explicit intent review and confirmation |
| desktop | `82f2a698c8f2260b366b-bf421b5efe8edddb92e6` | `simulator-reset-confirmation.spec.mjs` — Simulator reset stays unavailable when the configured reset service is unavailable |
| desktop | `82f2a698c8f2260b366b-0dd9c23bd51a7fe12f4d` | `simulator-reset-confirmation.spec.mjs` — Simulator reset stays unavailable without exact org-scoped permission |
| desktop | `82f2a698c8f2260b366b-7b72e79e1a4abc1519d6` | `simulator-reset-confirmation.spec.mjs` — ambiguous Simulator reset confirmation checks status without replaying mutation |
| desktop | `82f2a698c8f2260b366b-e26102b1069ce58d1c4e` | `simulator-reset-confirmation.spec.mjs` — expired Simulator reset intent requires a fresh explicit intent |
| desktop | `447f8cc2e85694770269-7d717d44ce76086ff621` | `stable-login.spec.mjs` — application expiry recovers once and preserves the original destination |
| tablet | `cc3070f185481daa6f24-83efd0bb79b87357c71d` | `control-room-guidelines.mjs` — GUI guidelines Control Room visible text has an 11px minimum |
| tablet | `cc3070f185481daa6f24-8d5274bcbca67f6d1128` | `dashboard.spec.mjs` — AI Scenario Apply supports exact raw Undo and Redo without saving or running |
| tablet | `cc3070f185481daa6f24-1e46f38477689080767a` | `dashboard.spec.mjs` — Control Room keeps Adapter Logs internal, Log Manager beneath it, and a Configuration group |
| tablet | `cc3070f185481daa6f24-16d1f2f288e9f9c0ee91` | `dashboard.spec.mjs` — GUI Simulator navigation order survives page switching |
| tablet | `cc3070f185481daa6f24-229a062420ff5761b58d` | `dashboard.spec.mjs` — GUI editor search controls follow the policy or assertion source and precede History |
| tablet | `cc3070f185481daa6f24-f00b88b5e42a67146f2f` | `dashboard.spec.mjs` — GUI editor search reveals distant matches and prevents accidental form submission |
| tablet | `cc3070f185481daa6f24-a72aa7244de8c55acf8b` | `dashboard.spec.mjs` — GUI new scenarios display Untitled and cannot save that placeholder |
| tablet | `cc3070f185481daa6f24-3041c8d0524ddedf9f42` | `dashboard.spec.mjs` — GUI raw scenario assistant uses exact raw context and applies only unsaved proposals in either mode |
| tablet | `cc3070f185481daa6f24-193f5ac5c5d5fe486bdd` | `dashboard.spec.mjs` — GUI raw scenario editor has safe syntax colors, aligned error gutter, and form round trips |
| tablet | `cc3070f185481daa6f24-0167c47780feca6144d3` | `dashboard.spec.mjs` — GUI raw scenario file import, download, and identity checks never save implicitly |
| tablet | `cc3070f185481daa6f24-0c717d658835fb09f542` | `dashboard.spec.mjs` — GUI raw scenario source scroll tracks highlighting and preserves unavailable assistant state |
| tablet | `cc3070f185481daa6f24-10bc69e8c681940787d4` | `dashboard.spec.mjs` — Policy runtime cancellation cannot release a newer pre-save evaluation request |
| tablet | `cc3070f185481daa6f24-139f3dac54dbac0d3e91` | `dashboard.spec.mjs` — Scenario machine log previews are compact, bounded and limited to running machine tracks |
| tablet | `cc3070f185481daa6f24-f1c01faabc6ee253b066` | `dashboard.spec.mjs` — Shared editor-page maximize works for Config, Gateways, Scenarios, and Directory |
| tablet | `cc3070f185481daa6f24-cfedbe6172efca5cb151` | `dashboard.spec.mjs` — Simulator Scenario polling aborts on navigation and ignores late catalog responses |
| tablet | `cc3070f185481daa6f24-9c84c385242f8320b654` | `dashboard.spec.mjs` — Simulator Scenarios groups unfiled entries and keeps terminal run history |
| tablet | `cc3070f185481daa6f24-5ddd42272cf620789da1` | `dashboard.spec.mjs` — Simulator catalog pollers report malformed JSON and recover without losing last-good data |
| tablet | `cc3070f185481daa6f24-5ed56058628ab212e89c` | `dashboard.spec.mjs` — adding topology parents keeps existing nodes at the same screen position |
| tablet | `cc3070f185481daa6f24-cd55db290207eb2cef70` | `dashboard.spec.mjs` — login service recovery needs explicit sign-in after an error |
| tablet | `cc3070f185481daa6f24-4bb0ac37751c6e3b8fa7` | `dashboard.spec.mjs` — visa refresh preserves current grants and resolves DNS labels |
| tablet | `f8e499d49f2323a0bbba-69b677565412a913672d` | `enrollment-live.spec.mjs` — real TLS claim polls approval and resumes the same identity after restart |
| tablet | `ba07e228bcb3aecf4d27-4e9a25c44211d19ce51c` | `geography.spec.mjs` — World Map Fit reframes the current viewport after manual zoom and resize |
| tablet | `cc3070f185481daa6f24-b18577de1474b8bd376e` | `organization-browser.mjs` — Organization layout gives the graph screen height and keeps configuration accessible |
| tablet | `cc3070f185481daa6f24-06425fecf4a56aef0815` | `organization-navigation.mjs` — Organization controls precede tabs and Simulator sidebar groups stay consistent across navigation |
| tablet | `cc3070f185481daa6f24-090a8bea743925fc8867` | `remaining-code-editors.mjs` — CodeMirror Scenario source preserves typography, colors, wrapping, replacement and history |
| tablet | `cc3070f185481daa6f24-8d78fea94242043aee75` | `scenario-browser.mjs` — Scenario browser File operations preserve draft and busy run guards |
| tablet | `cc3070f185481daa6f24-4c6d34291bef691b27ed` | `scenario-browser.mjs` — Scenario browser runs only the selected published file and preserves the active run when browsing |
| tablet | `cc3070f185481daa6f24-4af7e584a7ed6301eab4` | `scenario-browser.mjs` — Scenario browser selects a newly saved draft and enables Run only after publishing |
| tablet | `cc3070f185481daa6f24-fdc8708371888fb387ca` | `scenario-browser.mjs` — Scenario browser selects folder files without running and leaves full-width tracks |
| tablet | `cc3070f185481daa6f24-7ec0784bf90e34b2d77c` | `scenario-browser.mjs` — Scenario compact running steps disclose output but keep failures visible |
| tablet | `cc3070f185481daa6f24-d6564193377e41ba9c80` | `scenario-browser.mjs` — Simulate separates monitoring execution from organization configuration and preserves run results |
| tablet | `82f2a698c8f2260b366b-596825049d1c5dd0bacc` | `simulator-reset-confirmation.spec.mjs` — Simulator reset completion survives the post-confirmation run refresh |
| tablet | `82f2a698c8f2260b366b-3b3d1e9ca0cfc9339543` | `simulator-reset-confirmation.spec.mjs` — Simulator reset discards outstanding intent evidence when the run changes |
| tablet | `82f2a698c8f2260b366b-4f32ca44df3444e77768` | `simulator-reset-confirmation.spec.mjs` — Simulator reset discards outstanding intent evidence when the selected organization changes |
| tablet | `82f2a698c8f2260b366b-47e4f30ffa1310427dc3` | `simulator-reset-confirmation.spec.mjs` — Simulator reset fails closed for an unrecognized run state |
| tablet | `82f2a698c8f2260b366b-440d4813a252db8df559` | `simulator-reset-confirmation.spec.mjs` — Simulator reset intent expiry prevents confirmation |
| tablet | `82f2a698c8f2260b366b-b126ef2dcde1af5fa057` | `simulator-reset-confirmation.spec.mjs` — Simulator reset invalidates confirmation when the named session is cleared |
| tablet | `82f2a698c8f2260b366b-2cf4309c34f02add21ee` | `simulator-reset-confirmation.spec.mjs` — Simulator reset refuses stale generation at confirmation time |
| tablet | `82f2a698c8f2260b366b-746ec6eb4969d076d3a0` | `simulator-reset-confirmation.spec.mjs` — Simulator reset rejects intent target or actor mismatches before confirmation |
| tablet | `82f2a698c8f2260b366b-93eae1e0565885781ed7` | `simulator-reset-confirmation.spec.mjs` — Simulator reset rejects wrong configured status target and never falls back |
| tablet | `82f2a698c8f2260b366b-02cdbe86ef0db5d56f5e` | `simulator-reset-confirmation.spec.mjs` — Simulator reset requires explicit intent review and confirmation |
| tablet | `82f2a698c8f2260b366b-1a4efdc65967d0fed95d` | `simulator-reset-confirmation.spec.mjs` — Simulator reset stays unavailable when the configured reset service is unavailable |
| tablet | `82f2a698c8f2260b366b-5ae3f898bd81b97d5829` | `simulator-reset-confirmation.spec.mjs` — Simulator reset stays unavailable without exact org-scoped permission |
| tablet | `82f2a698c8f2260b366b-0b1a111f33c0b47071d4` | `simulator-reset-confirmation.spec.mjs` — ambiguous Simulator reset confirmation checks status without replaying mutation |
| tablet | `82f2a698c8f2260b366b-4151a21b26afb5320e01` | `simulator-reset-confirmation.spec.mjs` — expired Simulator reset intent requires a fresh explicit intent |
| tablet | `447f8cc2e85694770269-2545bae6a6538bec4791` | `stable-login.spec.mjs` — application expiry recovers once and preserves the original destination |

### Environment/live-only

Exactly two cases (one per project) fail the explicit `ZPR_LIVE_STORAGE_STATE` precondition in `ubuntu-live-control-room-acceptance.spec.mjs`. The error states that no network requests were made. A private authenticated state file is required; none was fabricated or obtained, and no live endpoints were contacted by those gated cases. Other opt-in live cases remain explicitly skipped, not passed.

#### Live-gated failure IDs

| Project | Test ID | File and test |
| --- | --- | --- |
| desktop | `00ee30bb1213ac54521d-82c177abbf58d5caa21b` | `ubuntu-live-control-room-acceptance.spec.mjs` — live Control Room OIDC and Simulator catalog/preflight are available without mutations |
| tablet | `00ee30bb1213ac54521d-12d04a8e635481ddc9c7` | `ubuntu-live-control-room-acceptance.spec.mjs` — live Control Room OIDC and Simulator catalog/preflight are available without mutations |

### Beta-only full-run failures

Five IDs fail beta but pass this maturing run. This is an observed run delta, **not** five proven fixes; CSP/timing and pre-existing asynchronous-fixture variability are present. The two commits were not edited during comparison.

#### Beta-only IDs

| Project | Test ID | File and test |
| --- | --- | --- |
| desktop | `cc3070f185481daa6f24-8a87d2058f12cea0a4c1` | `dashboard.spec.mjs` — Gateway analysis scope rejects delayed error after editing back to the original source |
| desktop | `ba07e228bcb3aecf4d27-59cb9cddaa1f81692883` | `geography.spec.mjs` — World Map Fit reframes the current viewport after manual zoom and resize |
| tablet | `cc3070f185481daa6f24-8d7dcd12e1769238f863` | `dashboard.spec.mjs` — Gateway analysis scope rejects delayed success after editing back to the original source |
| tablet | `cc3070f185481daa6f24-3466b56273f4f71cdf83` | `dashboard.spec.mjs` — Policy runtime cancellation cannot release a newer pre-save fixtures request |
| tablet | `cc3070f185481daa6f24-823cc6ef42d23c7a4742` | `dashboard.spec.mjs` — Scenario JSON transport keeps failed creation unsaved and preserves explicit creation payloads |

## Safety, validation and cleanup

- Strict audit: `python3 zpr-dashboard/scripts/check-control-room-boundary.py` **PASS**, exit 0, zero Simulator references in audited seams; checker and allowlist behavior unchanged.
- All ten maturing browser boundary cases pass (five production-source states × desktop/tablet), including unavailable providers and absent source configuration. No Simulator availability is required.
- No production implementation, browser fixture, assertion, skip condition, timeout, or configuration was changed. No additional full suite was necessary after the evidence-only documentation change.
- The disposable beta worktree and worktree-local browser output/scratch/diagnostic wrapper were removed after preserving JSON/log evidence. Other worktrees were not modified.
- `git diff --check` passes. Only this report and its machine-readable comparison are committed.

## Persisted evidence

Committed sibling `failure-comparison.json` contains every beta/maturing failure ID, project, status on both commits, classification, first-run error text, added/removed test-ID sets, full-run hashes and focused-run statistics.

Raw full JSON reports and logs are retained beside the requested session report under:

`/Volumes/Storage/dubnom/.copilot/session-state/47b551e4-8b6b-499f-86ce-6086e35586e3/files/`

- `maturing-browser-triage-beta.json` and `maturing-browser-triage-beta.log`
- `maturing-browser-triage-maturing.json` and `maturing-browser-triage-maturing.log`
- `maturing-browser-triage-beta-csp.json`, `maturing-browser-triage-maturing-csp.json`, and `maturing-browser-triage-beta-gateway-csp.json`
- `maturing-browser-triage-enrollment-focused.json` and `maturing-browser-triage-enrollment-diagnostic.json`

Raw JSON attachment paths refer to removed ephemeral traces/screenshots; the JSON errors and logs, not those attachments, are the retained evidence. Diagnostic fixture stdout contained disposable synthetic enrollment codes and was not retained.

| Raw full report | SHA-256 |
| --- | --- |
| beta | `bdf40d4d8af6b8889377a34e937dc30b0ac90ebee7dcf5bac437171e7a7ab3c2` |
| maturing | `9b7eda9a67fc0660d02d217f959cf2aeb08dd8a47068c4d69adec306fd187575` |

**Outcome:** baseline established and every failure classified; browser acceptance remains red. Zero clear new runtime defects were proven, 24 newly red legacy-contract cases remain outstanding, and live named-user acceptance remains unproven.
