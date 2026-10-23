# Exact-ID browser stability investigation

## Result

**No deterministic maturing-only regression was proven among the ten requested exact test/project identities.** Every identity fails on the pinned beta commit too. The one-shot “eight new failures / two unmodified failures now passing” delta is not a reliable regression/fix classification.

- **400 matrix executions:** ten identities × ten repetitions × two commits × two worker counts. **34 passed, 366 failed/timed out**, with zero skips/retries or runner-level errors. These are measured failures, not a passing acceptance run.
- **Eight identities demonstrate shared intermittency/timing dependence** across the repetitions or supplemental diagnostics. World Map Fit and tablet programmatic completion fail all 40 matrix executions: **shared reproducible baseline failures**, not maturing-only regressions. Do not mislabel their 100% focused failure rate as demonstrated within-matrix intermittency.
- **Zero contention-only failures established.** Every identity fails with one worker on both commits. Two workers changes some race outcomes but does not introduce a unique failure signature. This is not evidence that increasing timeouts, workers, or suppressing CSP assertions is a fix.
- **120 supplemental observer-only CSP executions** establish the native editing/style-copy mechanism and demonstrate timing-sensitive CSP outcomes. They are separate from, and not counted toward, the 400 matrix executions.

## Scope and method

Tested integration: `315c10650b510911ab1b1516b699ab5a72df6adf` on `maturing/w5-browser-stability`.

Tested beta: `4627e4cd99f15e1ee227f788574ab5055ee7a353` in the disposable detached sibling worktree `maturing-w5-stability-beta-4627e4c`, removed after evidence collection. No other worktree was changed.

Matrix ran sequentially from 2026-10-23 21:10:33 UTC to approximately 22:00:40 UTC, avoiding intentional cross-commit browser overlap. Each project/worker batch uses the selected file(s), an escaped exact-title suffix, `--repeat-each=10 --workers=1` or `--workers=2`, and `--retries=0 --trace=off --reporter=json,list`. Screenshots retain the unchanged configuration default. Both projects use Chromium as configured, not a substitute tablet browser. Locked dependencies were restored with `npm ci --no-audit --no-fund` only after the runner reported `playwright: command not found`.

`CI`, Simulator variables, and `ZPR_LIVE*` opt-ins were unset; `TMPDIR` was worktree-local `.browser-runtime`. The geography asset override was absent. The selected tests use local fixture servers and intercepted synthetic API responses; no live service, authenticated browser state, push, deploy, or live mutation was used. No tests, production files, dependency manifests, browser configuration, assertions, skips, or timeouts were edited. The report/scripts/logs are the only tracked changes.

Playwright 1.63.0 generates a different reporter ID for every repeat and does not include `repeatEachIndex` in this JSON format. `analyze.py` therefore requires the original exact ID **and ten distinct generated IDs with the same exact file/title/project**, rejecting extra identities, retries, skips, incomplete runs, and runner errors. `summary.json` preserves every generated repeat ID. Playwright's `flaky: 0` statistic refers to retry classification, not absence of variation across repeats.

Environment: macOS 26.7.1 arm64, Node v26.10.0, npm 11.19.1, Playwright 1.63.0, Chromium **153.0.8010.12**. See `environment.json`.

## Exact-identity outcomes

Each cell is **passes/failures out of ten**; “failure” includes a normal assertion failure or the unchanged 30-second test timeout. † identifies the two unmodified formerly failing IDs that passed the previous one-shot run.

| Exact original ID | Project | Title | Maturing w1 P/F | Maturing w2 P/F | Beta w1 P/F | Beta w2 P/F | Classification |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `cc3070f185481daa6f24-f9d4bda33a2de34a9120` | desktop | Scenario JSON transport keeps failed creation unsaved and preserves explicit creation payloads † | 1/9 | 2/8 | 3/7 | 1/9 | Shared intermittent fixture/load race |
| `cc3070f185481daa6f24-f779cd8e5f8d23340312` | desktop | Policy runtime cancellation cannot release a newer pre-save fixtures request | 1/9 | 0/10 | 0/10 | 2/8 | Shared intermittent fixture/load race |
| `cc3070f185481daa6f24-5486e71cd297f6cb868d` | tablet | GUI shared programmatic edits keep policy completion and Scenario indentation undoable | 0/10 | 0/10 | 0/10 | 0/10 | Shared reproducible completion-timing failure |
| `cc3070f185481daa6f24-823cc6ef42d23c7a4742` | tablet | Scenario JSON transport keeps failed creation unsaved and preserves explicit creation payloads | 6/4 | 3/7 | 5/5 | 5/5 | Shared intermittent fixture/load race |
| `cc3070f185481daa6f24-b0952dd568f4f7e00c45` | tablet | GUI raw scenario analysis ignores delayed errors for an edited source | 0/10 | 0/10 | 0/10 | 0/10 | Shared intermittent native-editor CSP failure |
| `cc3070f185481daa6f24-8d7dcd12e1769238f863` | tablet | Gateway analysis scope rejects delayed success after editing back to the original source | 1/9 | 0/10 | 0/10 | 0/10 | Shared intermittent native-editor CSP failure |
| `cc3070f185481daa6f24-086595e0f8ea7af946fc` | tablet | Gateway analysis scope rejects delayed error after editing back to the original source | 0/10 | 0/10 | 0/10 | 0/10 | Shared intermittent native-editor CSP failure |
| `cc3070f185481daa6f24-3466b56273f4f71cdf83` | tablet | Policy runtime cancellation cannot release a newer pre-save fixtures request | 0/10 | 1/9 | 0/10 | 1/9 | Shared intermittent fixture/load race |
| `cc3070f185481daa6f24-10bc69e8c681940787d4` | tablet | Policy runtime cancellation cannot release a newer pre-save evaluation request † | 0/10 | 1/9 | 1/9 | 0/10 | Shared intermittent fixture/load race |
| `ba07e228bcb3aecf4d27-59cb9cddaa1f81692883` | desktop | World Map Fit reframes the current viewport after manual zoom and resize | 0/10 | 0/10 | 0/10 | 0/10 | Shared reproducible baseline geometry failure |

The two † IDs are not stable fixes: desktop Scenario transport fails 17/20 maturing and 16/20 beta repetitions; tablet pre-save evaluation fails 19/20 on **each** commit. The eight “additional” IDs are now independently baseline-reproduced at the same project/file/title identity, not inferred from an opposite-project title.

## Root causes and limitations

### Strict `style-src-attr`: native Chromium editing copies focused bracket styling

The three requested CSP identities are tablet raw-scenario delayed-error, Gateway delayed-success, and Gateway delayed-error. Their semantic stale-response assertions complete; the failure is the shared fixture's final `Unexpected CSP violations` assertion (`dashboard.spec.mjs:995` on integration), not proof that an obsolete analysis response won.

`csp-observer.mjs` observes events, shadow-root mutations, and CDP `Audits.issueAdded`; it does not weaken headers, clear recorded violations, prevent input, or modify editor state/styles. Separate input/Gateway probes additionally observe `beforeinput` and old style-attribute values. Retained evidence establishes:

1. A trusted native `beforeinput` event has `inputType: "insertText"`, with the replacement selection anchored/focused in `<span class="cm-matchingBracket">{</span>` / `}</span>`.
2. The focused matching bracket has computed punctuation color `rgb(186, 201, 191)` and background `rgba(50, 140, 130, 0.32)`. The latter is CodeMirror language's default focused matching-bracket `#328c8252` (`@codemirror/language/dist/index.js:1848`). The custom theme's weaker `.cm-matchingBracket` transparent rule in `browser/code-editor.js:27` does not win the focused-state selector; before the focused style applies the observed background is transparent.
3. During the native replacement Chromium materializes this combined formatting on a `.cm-line` DIV as:
   ```text
   color: rgb(186, 201, 191); background-color: rgba(50, 140, 130, 0.32);
   ```
   The raw-scenario CDP audit identifies the violating DIV and these exact attributes. Gateway's editor reconciles/removes the transient attribute before CDP's asynchronous describe-node completes; the old-value mutation probe captures the **same exact combined style being added and removed** on both commits. It also captures color-only native writes, which are allowed by the existing palette hashes.
4. The unchanged `style-src-attr 'unsafe-hashes' ...` policy allows specific **color-only** serialized declarations, not that combined color-plus-background declaration. Chromium correctly emits an enforced inline `style-src-attr` violation. The fixture records the event even if CodeMirror immediately rebuilds the DOM and the final source text is correct.

This is a shared native-contenteditable/editor-theme interaction, not an HTTP source-registration/analysis-scope regression or an unavailable Simulator dependency. Relevant shipped JS, editor source, CSP helper, package lock, and browser configuration are byte-identical across commits; `source-identity.json` records thirteen cross-commit SHA-256 comparisons.

**Intermittency evidence with unchanged default trace configuration plus the read-only observer:**

| Exact tablet flow | Maturing P/F (10) | Beta P/F (10) |
| --- | --- | --- |
| Raw scenario delayed error | 4/6 | 0/10 |
| Gateway delayed success | 7/3 | 8/2 |
| Gateway delayed error | 6/4 | 8/2 |

The matrix without tracing fails 119/120 CSP executions; the default-trace observer run fails 27/60. Additional trace-off input/Gateway probes fail 60/60 and capture the mechanism. This strongly demonstrates timing/instrumentation sensitivity; because tracing and observation differ, it is **not** a controlled proof that tracing alone is causal, nor evidence that worker contention alone is causal.

**Proposed follow-up, not implemented or claimed verified here:** make the CSP-safe matching-bracket override cover the focused selector as well (including inherited color and transparent background), or handle native replacement via a CodeMirror transaction without leaking rich-text formatting. Validate the chosen fix with strict unchanged CSP and real native replacement/typing coverage on both projects. Do **not** add `unsafe-inline`, blanket-hash rich-text style combinations, ignore violation events, or turn off bracket/input coverage to make the suite green.

### Other shared failure mechanisms

- **Scenario transport (desktop and tablet):** the case's endpoint handler pushes `route.request().postDataJSON()` for every method. After save, `saveScenarioDraft` awaits revisions and refreshes the scenario catalog; its GET can race the immediate `writes` equality assertion and contributes `null` as a third “write”. The retained failures show that exact extra `null`. A method-aware fixture and an explicit catalog-completion boundary are the appropriate follow-up; no production write/publish failure was established.
- **Policy pre-save fixtures/evaluation ownership:** most failures time out clicking the second `#policy-save`, which is disabled. The test clicks a new record and immediately fills the editor without awaiting that record's asynchronous load. `loadPolicyRecord` can then replace the just-filled source with saved content and reset/disable save, leaving the draft no longer dirty. That code-level race is consistent with these failures; this packet did not isolate a counterfactual policy fix. `app.js:2560` and the save ownership code are identical across commits. Synchronize on selected record/content readiness before editing in follow-up; the existing test still needs to exercise old/new completion ordering. The current shared failures do not prove the ownership mechanism itself regressed.
- **Programmatic completion (tablet):** all 40 focused executions insert a newline instead of accepting `user.department:`. The helper's 100 ms delay starts after requesting completion, not after the popup actually opens. CodeMirror rejects acceptance within its 75 ms **popup-open** interaction guard (`@codemirror/autocomplete/dist/index.js:1095`), so a visible newly opened menu can still route Enter to newline. This is a reproducible shared completion-timing failure in this setup, not an integration-only source-contract regression. A readiness-aware completion interaction is a follow-up, not a timeout increase/removed assertion.
- **World Map Fit (desktop):** all 40 executions reproduce the shared geometry assertion failure (`coverage` about 1.125, required <0.95). The geography test and relevant shipped map/app code are identical. This packet establishes baseline reproduction, not a newly introduced maturing map defect; exact geometry repair remains separate work.

These code-path explanations do not substitute for future fix validation. In particular no isolated patched experiment was used to claim a proven repair, and no deterministic maturing-only defect requiring a maturing-specific proposed patch was established.

## Evidence, verification, and reproduction

- `targets.json`: all ten original IDs, exact file/title/project identity, and previous delta category.
- `{maturing,beta}-w{1,2}-{desktop,tablet}.json` / `.log`: eight raw matrix reports and exact invocation logs; `runs.json` records commits, commands, start/end times, and process exit codes.
- `summary.json` / `analysis.log`: verified 400-run coverage, statuses, error fingerprints, repeat IDs, and raw-report hashes.
- `*-csp-{diagnostic,input-probe,gateway-probe}.json` / `.log`: 120 supplementary executions, including browser version, native inputs, violations, CDP nodes, and transient line-style mutations. Corresponding `*-runs.json` manifests retain command/status information. `csp-findings.json` / `csp-analysis.log` summarize observations.
- `source-identity.json`, `environment.json`, and `SHA256SUMS`: source identity, environment, and evidence checksums.
- Raw JSON attachment paths refer to disposable local screenshots/traces, which are not retained in this evidence commit. The raw reporter errors, diagnostic stdout, and summaries are retained; nothing relies on those attachments. Log trailing whitespace is normalized only for repository hygiene; raw JSON is unchanged.

Verification from this evidence checkout:

```sh
python3 browser-triage/stability/analyze.py
python3 browser-triage/stability/analyze-csp.py
(cd browser-triage/stability && shasum -a 256 -c SHA256SUMS)
```

For a new measurement, create **new disposable detached** worktrees pinned to the two recorded commits (do not repurpose another user's worktree), restore their locked browser dependencies only if missing, and run:

```sh
python3 browser-triage/stability/run-stability.py   --maturing-worktree ../YOUR-DETACHED-315c106   --beta-worktree ../YOUR-DETACHED-4627e4c
```

The evidence-only commit advances this branch beyond the tested integration commit, so the reusable runner explicitly accepts a separately pinned integration worktree and refuses the wrong HEAD. It writes a fresh matrix into this evidence directory; preserve the committed evidence first. Additional CSP observations can be reproduced with the retained diagnostic script against the pinned worktrees; its current default integration location is this evidence worktree, whose tests/production files are unchanged. All browser helpers are bounded and exit after use.

**Disposition:** evidence/report only. No push/deploy/live mutation. Browser acceptance remains red. The observed delta is explained by shared baseline defects and timing-sensitive fixtures/editor behavior, not a proven deterministic maturing regression.

Local cleanup note: the disposable beta worktree was successfully removed. Removal of the integration-local `.browser-runtime`, ignored `zpr-dashboard/test-results`, and evidence-script `__pycache__` directories was denied by the execution approval layer; those untracked/ignored runtime files are not evidence and are excluded from the commit. No browser process remains.
