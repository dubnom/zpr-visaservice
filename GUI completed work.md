# GUI completed work

[Active backlog](GUI%20work.md) | [Design guidelines](GUI%20design%20guidelines.md) |
[Deployment history](GUI%20deployments.md)

Historical completed checklists and their verification evidence, migrated on
2026-10-09 without changing completion claims. Checked means implemented as
recorded, not necessarily deployed, committed or pushed. Older requests can be
superseded by later ones (for example Wrap defaults); preserve them as history,
not competing current requirements. References such as "above", "below" and
"this deployment" retain their original context from the former combined tracker;
use the dated deployment ledger when checking release scope.

## Hierarchical adapter LDAP completion (2026-10-15 UTC)

**Deployed to Control Room at 20:06 UTC; not committed or pushed.**

- [x] Clarify expected output rather than retaining the prior full-catalog-name
  assumption: `with grea` should offer only `greatlakes.`, followed by its
  attributes; the final source should be `greatlakes.location:`.
- [x] For adapter definitions, remove a catalog built-in type prefix unless the
  operator explicitly typed it, and offer one namespace segment at a time.
  Keep user/device/service type-filtered completion behavior unchanged.
  Accepting a namespace automatically opens its children without adding a
  colon; accepting a leaf adds its single trailing colon.
- [x] Namespace/leaf acceptance are separate Undo steps. The exact clarified
  workflow is tested with the production-shaped `device.greatlakes.*` catalog,
  alongside multi-statement, explicit-prefix, multi-part namespace, existing
  short/qualified completions and editor regressions.
  All 52 selected desktop/tablet editor checks pass; editor problems and
  whitespace checks pass. Bundle rebuilt; no Go or provider calls.
- [x] Approved Room-only deployment preserves complete configuration/mounts/ID;
  all backend/Simulator image/start/ID baselines unchanged. Matching executable
  hash, actual served HTML/46 assets, exact two-stage insertion and both Undo
  steps under strict CSP, sign-in gate and verified-mTLS readiness pass.
  All browser APIs mocked; no source sent to AI or saved/analyzed.

See [rollout/rollback](GUI%20deployments.md#hierarchical-adapter-ldap-completion-2026-10-15-2006-utc).

## Token limits, production LDAP completion and Discard (2026-10-15 UTC)

**Deployed to Control Room and Control-Service at 19:55 UTC; not committed
or pushed.**

- [x] Triple the original token choices to 900/1800/3600/7200 and add the doubled
  default 2400. Update the shared browser settings, editor validator and design
  assistant validator. Retain old 300/600/1200 inputs for already-open clients;
  requested limits are forwarded unchanged, never raised or retried implicitly.
- [x] Read production's attribute definitions safely: the catalog contains
  `device.greatlakes.department` and `device.greatlakes.location`, not the bare
  `greatlakes.*` fixture used in the earlier check. Match input without the
  built-in type prefix while inserting the full catalog name and colon. Correct
  statement-boundary selection for completion after preceding definitions.
  Test production-shaped catalog entries in multi-statement policy text, with
  automatic suggestions while typing and exact insertion/Undo.
- [x] Find Discard's same-record early return. Add an explicit force-reload
  argument used only by Discard, preserving same-record selection and assertion
  save messages/history. Verify cancel, successful reload/reset history,
  failure with draft preservation, and assertion Discard.
- [x] All 54 selected desktop/tablet browser checks pass. Focused assistant
  transport/route/validation race tests and Go vet pass; required `make test`
  retains the four known Enrollment readiness, runtime-platform fixture,
  Scenario shell extraction and ELF fixture failures. Editor problem checks
  pass. No real policy/chat sent to a provider; production inspection read
  attribute definitions only, not directory membership or individual records.
- [x] With approval and draft secured, replace both service executables in place.
  Exact configuration/mounts/IDs preserved, Policy-Service and Simulator
  unchanged, hashes match and verified-mTLS readiness passes. Actual served
  HTML/46 assets match source; actual-CSP isolated browser verification passes
  for choices/default, production-shaped LDAP suggestions/insertion/Undo,
  Discard cancel/reload/history reset and sign-in gate. APIs mocked, no writes
  or provider requests.

See [rollout/rollback](GUI%20deployments.md#token-limits-production-ldap-completion-and-discard-2026-10-15-1955-utc).

## Assistant compact settings and LDAP completion (2026-10-15 UTC)

**Deployed to Control Room at 19:36 UTC; not committed or pushed.**

- [x] Remove the requested direct-edit/Undo/autosave sentences from the
  policy/assertion disclosure, keeping the draft/chat transmission notice.
  Rename the shared collapsed section **Model and Tokens** and put token usage
  inside it, hidden until expanded. Update sidebar help, documentation and
  source asset cache versions.
- [x] Reproduce the operator's synthetic example
  `define great as adapter with greatlakes.` in both desktop/tablet tests.
  The completion parser special-cased only built-in namespaces before a dot,
  incorrectly treating arbitrary LDAP namespace dots as statement endings.
  Accept dotted identifiers in definition attribute slots without hardcoding
  provider names or fetching directory data.
- [x] Verify automatic suggestions and insertion/Undo for `greatlakes.` and
  `zpr.adapter.`, short and qualified LDAP attributes, parent filtering and
  suppression in comments, strings, completed values and statement endings.
  All 44 selected editor/shared-assistant browser checks pass. Editor problem
  checks and whitespace validation pass. No Go changes; no provider requests,
  policy saves, Analyze or runtime changes.
- [x] With approval and the draft secured, update Control Room only. Verify
  matching executable and served HTML/46 assets; exact disclosure and renamed
  section, stats visible only when expanded, automatic `greatlakes.` completion
  insertion and Undo under actual CSP, no browser errors and preserved sign-in
  gate. All browser APIs mocked. Complete Room configuration/mounts/ID and
  Control-Service/Policy-Service/Simulator identity/image/start baselines match.
  Independent backend readiness remains healthy over verified mutual TLS.

See [rollout/rollback](GUI%20deployments.md#assistant-settings-and-ldap-namespace-completion-2026-10-15-1936-utc).

## Sole editor and AI edit/usage verification (2026-10-15 UTC)

**Deployed to Control Room and Control-Service at 19:12 UTC; this follow-up is
not committed or pushed.** Supersedes the opt-in
editor/fallback UX described in historical entries below.

- [x] Start CodeMirror automatically for policy/assertions. Remove the checkbox,
  public disable path and interactive legacy fallback. Rename bootstrap/style
  assets and update cache versions. Keep hidden source/highlight/result elements
  solely as controller bridges for existing Save, Analyze, diagnostics and
  classification. Other editor types retain their own editing implementations.
- [x] Disable policy/assertion assistant requests until the editor is ready;
  all requests use direct edits. Bundle-load failure is explicit and never
  exposes an old editor. Fresh record states read current permissions and Wrap
  rather than reusing startup's disabled state.
- [x] Require requested modifications in the actual edit list, not just the
  explanation. Verify the resulting CodeMirror text and backing source, reject
  nonempty no-op replacement lists, and give empty edit lists a neutral notice.
  Remove the appended "Applied as one unsaved..." success text. Preserve atomic
  validation, one-step Undo/Redo, unsaved changes and stale-response rejection.
  The original operator response was not retained, so its precise no-edit
  cause has not been established.
- [x] Forward every selected output maximum unchanged, with no automatic retry.
  Preserve provider usage on structured validation/token-limit errors. Explicitly
  reject and report responses whose provider output count exceeds the requested
  maximum; do not misrepresent this as preventing already-incurred billing.
  Show last-request output/requested maximum separately from session totals.
- [x] With explicit approval, send one synthetic-only replacement request to
  the configured provider: requested maximum 300, reported input 381/output 77,
  `end_turn`, complete nonempty edits. No real policy/chat history sent. This
  check did not reproduce the reported first-request overrun; don't claim a
  provider billing defect was reproduced or fixed.
- [x] All 38 desktop/tablet editor checks pass, including delayed startup,
  load failure, font/color parity, scrolling caret, policy/assertion application
  and Undo, Format/Analyze/Save, no-ops, usage, all four requested maxima,
  no retries, stale changes, permissions and strict CSP. Twelve selected
  Config/Gateway/directory/organization assistant checks also pass.
  Two selected Scenario checks fail in the unchanged setup helper on missing
  `#scenario-organization`, before assistant interaction; not modified.
- [x] Focused assistant transport/route race tests and `go vet` pass. Full
  `make test` retains the four known Enrollment readiness, missing runtime
  platform, Scenario shell extraction and ELF fixture failures. No unrelated
  diagnostics/inventory files or failing tests changed.

- [x] With user approval and the draft secured, deploy both services in place.
  Verify exact IDs/configuration/mounts, unchanged Policy-Service/Simulator
  baselines, matching executable hashes, verified-mTLS readiness and actual
  served HTML plus 46 matching assets. Verify automatic sole-editor startup,
  visible mocked AI source changes, Undo/Redo, no success suffix/Insert,
  per-request/session usage, Format history, strict CSP and login gate.
  All browser APIs were mocked; no real policy or chat was sent to AI.

The provider maximum includes reasoning and structured edit JSON; it is not an
application-enforced financial quota.
See [rollout/rollback](GUI%20deployments.md#sole-editor-and-explicit-ai-editusage-results-2026-10-15-1912-utc).

## Assistant reasoning-envelope repair (2026-10-15 UTC)

**Deployed to Control-Service only at 18:45 UTC; not committed or pushed.**

- [x] The 18:40:36 failure was a server response-validation rejection. With
  explicit approval, send a synthetic multi-line refactor prompt, not operator
  policy/directory/conversation data. The real provider returned `end_turn`,
  a `thinking` block and one complete JSON `text` block. The prior parser
  rejected any non-text block before inspecting the edit payload.
- [x] Skip only known `thinking`/`redacted_thinking` blocks; never expose them
  as editor text or edits. Continue rejecting unknown blocks, tools, missing or
  multiple text payloads, truncation, invalid schemas and oversized edits.
- [x] Add content-free validation check labels to server errors/logs without
  including source, payload or reasoning. Preserve `errors.Is` classification
  and the existing browser answer contract.
- [x] Replay the captured synthetic provider response through the actual parser:
  complete nonempty edits accepted, token usage preserved. Remove the temporary
  private-fixture replay test afterward. Permanent transport/route regressions
  cover both reasoning block types, no-edit/edit payloads and unsafe envelopes.
  Focused race tests/vet and whitespace checks pass; full Go suite retains the
  four known failures. No UI changes or browser test rerun in this backend fix.
- [x] Production verified-mTLS status reports configured Great Lakes and
  assistant/compiler/tester ready. Complete container configuration/mounts
  unchanged; Control Room, Policy-Service and Simulator start/image baselines
  unchanged. No refresh, policy save, Restore or runtime changes.

The captured synthetic refactor reproduces the validation bug; the original
operator response was not retained or replayed. The actual Milwaukee request
remains for operator retry.
See [rollout/rollback](GUI%20deployments.md#assistant-reasoning-envelope-repair-2026-10-15-1845-utc).

## Format history and assistant proxy budget (2026-10-15 UTC)

**Deployed to Control Room only at 18:38 UTC; not committed or pushed.**

- [x] Confirm the user's requirement: Format should be undone as one source edit.
  Policy Format previously assigned textarea value directly, resetting
  CodeMirror history. Route it through the shared edit helper. All shared
  CodeMirror replacement/insertion transactions are isolated; no-op replacements
  leave history alone. Assertion Format already uses that helper.
- [x] Verify exact pre-format text restoration, Redo, and an additional Undo of
  preceding typing for both policy/assertion Format, including shortcuts from
  the Format button. No record save occurs.
- [x] Diagnose the actual assistant failure at 18:34:10: Control Room logged a
  timeout and Control-Service's provider call was canceled. Control Room had
  applied the ordinary 10-second response-header timeout to assistant requests,
  despite the provider's existing 45-second budget.
- [x] Route exact POST policy/gateway assistant endpoints through a separate
  cloned mTLS transport with a 75-second response-header budget, allowing
  context lookup plus the provider call. Preserve all ordinary API/log
  timeouts and browser cancellation; no unlimited request or automatic retry.
- [x] New proxy tests verify exact route/method selection, budget values,
  preserved TLS settings, response survival beyond the ordinary timeout and
  upstream cancellation with Simulator unavailable. All 34 selected
  desktop/tablet editor checks pass; focused Go race tests, vet, deterministic
  bundle, syntax and whitespace checks pass. Required full Go suite retains
  the same four known failures.
- [x] Actual served assets/CSP verify policy Format as one Undo/Redo step,
  earlier typing Undo and mock AI/Undo without JavaScript/CSP errors.
  Live verified-mTLS readiness passes. No model request was sent during this
  repair; the operator's actual Milwaukee refactor remains to be retried.

Only Control Room restarted; its full configuration/mounts are unchanged.
Control-Service, Policy-Service and Simulator start/image baselines are
unchanged. No policy saves, Restore or runtime changes.
See [rollout/rollback](GUI%20deployments.md#format-history-and-assistant-proxy-budget-2026-10-15-1838-utc).

## Scroll-aware caret, toolbar history and supported AI schema (2026-10-15 UTC)

**Deployed to Control Room and Control-Service at 18:29 UTC; not committed or
pushed.** Supersedes the 18:13 forced-tool protocol.

- [x] Diagnose actual AI rejection: provider HTTP 400,
  `tool_choice: type "tool" and "any" are not supported for this model.`
  With approval, synthetic-only provider probes confirmed native
  `output_config.format` JSON-schema output works for the configured model.
  Both an empty edit list and an exact synthetic `original` -> `updated`
  replacement were returned successfully. No policy, directory data or
  conversation history was sent in those probes.
- [x] Replace forced tools with native response-schema output. Retain complete
  response, token-limit, bounded payload/schema and atomic browser validation;
  preserve the answer envelope expected by already-open editor pages.
- [x] Use CodeMirror's measured caret/selection rendering rather than native
  contenteditable caret painting. Long-document tests verify exact click/text
  positions, caret geometry within 3 pixels and scroll-following geometry in
  policy/assertion editors on desktop/tablet, including horizontal scrolling.
  The user's original native caret painting drift was not independently
  reproduced by the initial click tests; this removes that native rendering
  path and verifies the replacement rather than claiming a proven browser cause.
- [x] Clarify Undo expectation with the user: Undo the last source edit while
  focus remains on Analyze. Route Undo/Redo from editor controls to the visible
  editable CodeMirror instance, then return focus to it. Do not intercept other
  editable fields or read-only/hidden sources. Record-switch history isolation
  remains unchanged.
- [x] All 32 focused desktop/tablet editor checks pass, including toolbar
  Undo/Redo after policy/assertion Analyze, failed responses/retry and atomic AI
  history. Focused Go race tests, vet, syntax, deterministic bundle and
  whitespace checks pass. Required full Go suite retains four known failures.
- [x] Actual served assets/CSP pass measured-caret scrolling, Analyze-toolbar
  Undo/Redo, mock AI edits and Undo without CSP/JavaScript errors. All browser
  API traffic was intercepted; no live policy was saved or analyzed. Production
  verified-mTLS readiness passes. Both service configurations are unchanged;
  Policy-Service and Simulator were not restarted.

Refresh/sign in again and re-enable the opt-in trial to load the new UI.
See [rollout/rollback](GUI%20deployments.md#scroll-aware-caret-toolbar-history-and-supported-ai-schema-2026-10-15-1829-utc).

## Structured AI editor responses (2026-10-15 UTC)

**Deployed to Control-Service only at 18:13 UTC; not committed or pushed.**

- [x] Replace prompt-only JSON output in direct-edit mode with the forced
  `propose_editor_edits` tool contract. Require exactly one complete response,
  bounded edits and explicit before/after strings. Text blocks are never
  interpreted as edit payloads. The existing browser JSON contract remains
  compatible with open Control Room pages.
- [x] Reject token-limited responses before applying anything, including
  otherwise parseable partial tool results. Report the output-limit cause and
  direct operators to increase Max tokens or request smaller changes.
- [x] Preserve atomic validation, stale-record/source guards, direct unsaved
  edits and Undo. Legacy editor assistants continue using their previous text
  transport and Insert behavior.
- [x] Mock-provider transport/route tests cover structured requests, escaped
  source, usage, truncation, wrong/missing/duplicate tools and invalid schema.
  All 28 desktop/tablet browser checks pass, including preserved drafts on
  errors and a successful undoable retry. Focused Go race tests and vet pass.
  Required full Go tests retain the same four known failures.
- [x] Live verified-mTLS readiness confirms Great Lakes configured and
  assistant/compiler/tester ready. Only Control-Service restarted, with exact
  Config/HostConfig/mounts preserved. No live model request, policy save,
  Restore or runtime mutation; no Control Room refresh required.

This corrects the fragile response protocol; the user's original raw provider
response was not available, so malformed text versus truncation was not proven
for that individual failure. See
[deployment/rollback](GUI%20deployments.md#structured-ai-editor-responses-2026-10-15-1813-utc).

## Editor color parity and Analyze repair (2026-10-15 UTC)

**Deployed to Control Room and Policy-Service at 17:59 UTC; not committed or
pushed.**

- [x] CodeMirror decorations reuse the legacy policy/assertion token
  classification and exact colors, including base text. No second tokenizer.
- [x] Nine exact color-only CSP hashes support native Chromium editing without
  permitting arbitrary inline styles/scripts. Font metrics and direct undoable
  AI edits remain unchanged.
- [x] Diagnose production Analyze: compilation worked, but the fixture endpoint
  returned HTTP 503 because Policy-Service had an old executable and no
  independent fixture-file setting.
- [x] Export the existing provisioning inputs once through the existing adapter,
  then configure Policy-Service to read only the independent version-1 operator
  contract plus live LDAP. No production profile/manifest/API dependency on
  Simulator, Restore, policy save or organization runtime changes.
- [x] Verify fixture GET and a non-saving Analyze over production verified mTLS:
  HTTP 200, 38 actors, 10 services, 380 actor/service evaluations.
  The existing Effective network policy also returns HTTP 200 with 380
  evaluations; it references none of the omitted attributes.
- [x] All 26 focused desktop/tablet editor checks pass; focused race tests and vet
  pass. Required full Go suite retains the same four known baseline failures.
- [x] Actual served assets/CSP pass isolated token parity, typing, mock AI and
  Undo/Redo checks with no JavaScript/CSP errors. API calls in that browser are
  intercepted; this is not an authenticated operator Analyze click or real
  model request.

Existing fixture-format limits remain explicit: three coordinate-valued
attributes cannot be represented in ZPT and are reported as omitted. Policies
referencing them cannot be analyzed. Refresh, sign in again if prompted, then
enable the opt-in CodeMirror trial.
See [deployment/rollback](GUI%20deployments.md#editor-color-parity-and-analyze-repair-2026-10-15-1759-utc).

## CodeMirror font parity and direct AI edits (2026-10-15 UTC)

**Deployed to Control Room and Control-Service at 17:47 UTC; not committed or
pushed.** The user approved
automatic application of requested AI line changes as undoable, unsaved edits,
rather than retaining explicit Insert/Apply in the CodeMirror trial.

- CodeMirror takes font size, family and line height from each original
  textarea's computed styles, matching both policy and assertion editors.
- Policy/assertion requests in CodeMirror opt into structured exact-match
  replacement edits. Accepted edits apply together as one isolated Undo step;
  Redo restores them. Other unsaved text is preserved, and nothing is saved,
  analyzed, staged or activated automatically.
- Reject malformed/truncated responses, missing/ambiguous/overlapping anchors,
  oversize edits, stale source/record responses and mode/permission changes.
  Validation is atomic: a rejected edit leaves the entire source unchanged.
  The previous textarea editors retain explicit Insert; other assistants keep
  their existing contracts.
- All 26 selected desktop/tablet checks pass: typography parity, policy and
  assertion direct edits, Undo/Redo, rejection paths, record changes, legacy
  Insert and existing trial/Config-assistant regressions. Focused assistant/CSP
  race tests and embedded build pass. Required full Go tests retain the four
  known failures documented below; no full-suite pass is claimed.

The user approved the coordinated current-worktree rollout, including other
pending backend changes. Both service configurations are unchanged; Simulator
and Policy-Service were not restarted. Live HTML/assets match source; the real
server CSP permits matching font metrics and a mocked direct-edit/Undo/Redo
browser check without JavaScript/CSP errors. Verified mTLS read of independent
production policy context succeeds. No live AI request or policy save was
performed. See [rollout and rollback](GUI%20deployments.md#control-roomservice-font-parity-and-direct-ai-edits-2026-10-15-1747-utc).

## Policy/Assertions CodeMirror trial (2026-10-15 UTC)

**Deployed to Control Room at 2026-10-15 17:30 UTC; not committed or pushed
as part of this trial.** Simulator was not updated.

- [x] Opt-in CodeMirror 6 for policy and assertions only, with syntax colors,
  bracket matching, line numbers, policy completion, undo/redo and shared
  Find/Replace/Wrap.
- [x] Switch back without losing text/selection; default off after reload.
  Implementation changes and record loads reset local undo history.
- [x] Preserve existing Save shortcuts/confirmation, revision payloads,
  Analyze source/context guards, source-error marks and clickable result actions.
- [x] Bundle locally, load only on opt-in, retain dependency licenses and report
  load failure without disabling the current editor.
- [x] User-approved exact-color CSP hashes only; arbitrary inline styles and
  scripts remain blocked. No external editor service or Simulator dependency.

Verification: 14 trial browser checks pass across desktop/tablet, including
exact Save/Analyze text, stale-response rejection, fallback and negative CSP
checks. The broader 36-check editor selection passes 34; two legacy assertion
toolbar-spacing checks fail at 27px versus the expected 9px. Removing the trial
controls and stylesheet still yields 27px in both viewports; the existing tests
were not changed. Focused CSP race test, embedded Go build and editor diagnostics
pass. Required dashboard `make test` retains the four known failures: enrollment
readiness, stale-bundle platform fixture, placement shell extraction and
trusted-service ELF fixture. No full-suite pass is claimed.
The deployment rerun passes all 14 trial checks and the focused CSP race test;
the full Go suite retains those same four failures. The bundle-load-failure test
now uses a click rather than a checked-state assertion because the intended
failure handler immediately resets the checkbox.

The user approved deploying the current worktree, including pending
policy-fixture/operator-file refactoring changes. Control Room alone restarted.
Forty-seven served assets match source byte-for-byte, and configuration/mounts/
ports are unchanged. Live sign-in protection and actual server CSP are verified;
a separate isolated browser fixture verifies typing and editor fallback with
live assets, no unexpected JavaScript/CSP errors and all API writes blocked
(none attempted). This is not an authenticated live-data or real Save/Analyze
check. See [deployment and rollback](GUI%20deployments.md#control-room-codemirror-trial-2026-10-15-1730-utc).

At the user's request, an isolated loopback-only preview runs at
`http://127.0.0.1:8796/#policy`, with a conspicuous sample-data banner.
Its static assets are mounted read-only; sample Save revisions exist only in
memory, and Analyze emits a labeled sample diagnostic rather than invoking a
compiler. Browser verification confirms exact assertion Save text/revision
round-trip and the CodeMirror policy diagnostic marker. No live service is
connected or restarted. This preview is not a deployment and does not provide
real validation, formatting or assistant services.

## Runtime repairs and Scenario browser publication (2026-10-15 UTC)

This publication checkpoint includes independent assertion seeding and image
retention, blocking Restore progress, Gateway registration/DNS lifecycle repair,
node authentication renewal, operator-backed Business denial evidence, and the
Scenario Browse/File/compact-track changes. Dated deployment entries retain
their original publication state; source publication does not imply a new rollout
or completion of a refactoring milestone.

Pre-publication checks:

- Full Rust workspace `cargo test` passes, including 497 Visa tests.
- Focused Go race tests, `go vet ./...`, `go build ./...`, changed Go formatting
  and launcher shell syntax checks pass.
- Required dashboard `make test` retains exactly the four known failure groups:
  enrollment readiness, stale-bundle platform fixture, placement shell
  extraction and trusted-service ELF fixture.
- Twenty focused desktop/tablet browser checks pass for Scenario browsing,
  compact running steps/log previews and Restore progress/outcome/log access.
  An initial broader 26-check selection passed 20 and failed six legacy checks
  at the documented obsolete `#scenario-organization` locator. No full
  browser-suite pass is claimed, and those tests were not changed.
- Local discovery scan output and a Vim swap file are excluded from publication.
  The diverged `zpr-common` branch is explicitly outside this publication.
  The existing `zpr-core` feature-branch commit was confirmed already published.

## Scenario browsing and compact running steps (2026-10-15 UTC)

- [x] Browse... opens a dialog with collapsible folders (including Unfiled).
  Selecting a file leaves one compact scenario card on the page without
  starting a run or replacing the active run display.
- [x] File... offers New Scenario... and Edit through the shared accessible
  menu. Saving a new draft selects it; running still requires a published
  revision in the active organization and no run/cleanup already in progress.
- [x] Runner/Results use the full page width, retaining synchronized top
  scrolling when machine tracks overflow.
- [x] During running/cleanup, verbose output is collapsed under Details and
  routine Waiting/Running placeholders are omitted. Open disclosures persist
  across updates of the same run and reset for a different run. Failures remain
  explicit; terminal runs show full output.
- [x] Updated page Help and the dashboard web README.

Verification: 20 selected desktop/tablet browser checks pass (12 new checks
plus eight existing editor-window, Clear/history, failure-progress and Results
checks). Includes a 390px Browse dialog check, selected-file run requests,
save/publish selection, unpublished/busy guards, organization changes, explicit
load errors, safe output text and API-backed overflow scrolling. Embedded
dashboard build, JavaScript syntax, editor diagnostics and whitespace checks pass.

This is not a full browser-suite pass. A broader 70-check selection had 56
failures at the obsolete `#scenario-organization` locator (representative tests
also fail with unchanged Scenario assets); 14 passed. The existing late-catalog
and late-log tests also fail in both viewports with unchanged assets. One legacy
direct-render scrollbar check failed intermittently in a later run; the new
API-backed scrolling check passes in both viewports. These unrelated tests were
not weakened to obtain a green run; UI selectors for the intentional File/menu
change were adapted separately.

Deployed to Simulator at 2026-10-15 16:24 UTC. Live Browse/select/File/full-width
checks pass without API writes; no workload was started for release verification.
See the [deployment record](GUI%20deployments.md#scenario-browser-deployment-2026-10-15-1624-utc).
Not committed or pushed by this task.

## Publication reconciliation (2026-10-15 UTC)

This checkpoint publishes the pending dashboard/discovery source, regression
tests and tracker updates after `e1b24ab`. Historical uncommitted/not-deployed
notes retain their original dates; they do not override the latest deployment
ledger. Publication includes the assistant/configuration and log-boundary
extractions, business simulations, Results navigation and Restore feedback.
It does not install undeployed production-service or standalone CLI changes.

Pre-publication checks:

- Full dashboard `make test`: exactly the four previously recorded failure
  groups (enrollment readiness, stale-bundle platform fixture, placement shell
  extraction and trusted-service ELF fixture); other packages pass.
- Selected business, Great Lakes, assistant, platform services, Results,
  discovery/progress/deadline and package-boundary race checks pass.
- Sixteen selected desktop/tablet browser checks pass, covering Restore status
  and logs, gateway Results/raw evidence/navigation, and existing assertion/
  policy-result behavior. This is not a full browser-suite pass.
- No additional Restore, business forwarding scenario, gateway rollout or
  production-service restart was performed for publication.

## Great Lakes business simulations and Restore feedback

Software deployed 2026-10-15 UTC; live business acceptance remains blocked.
See [business release](GUI%20deployments.md#great-lakes-business-service-deployment-2026-10-15-0056-utc)
and [feedback release](GUI%20deployments.md#restore-feedback-deployment-2026-10-15-0103-utc).

- [x] Implement seven stateful services: support, engineering builds, finance,
  shipping, receiving, assembly and quality, all on dedicated `machine-13`.
  Real create/read/advance/read HTTP flows persist per organization/generation/
  machine, with strict identity/schema checks, atomic writes and bounded records.
- [x] Provision independent provider keys, the directory service host and group
  memberships; complete department/site runtime grants and all five standalone
  catalog records. Runtime and each catalog record compile independently.
  Catalog examples remain authoring records, not automatically published edits.
- [x] Add two 42-pair Business scenarios covering twelve employees and seven
  services: 13 allows and 71 denials. Check provider readiness before DNS;
  enforce cross-host placement and fresh tuple-specific denial telemetry.
- [x] Verify persistence/reload, strict inputs, corruption, concurrent exact
  1,000-record capacity, write/audit failure semantics, policy/directory
  consistency, placement, readiness, key preservation and state-bind isolation.
  Focused race tests, compiler checks, vet, formatting, shell/JS checks and
  macOS/Linux arm64/Linux amd64 builds pass. Full Go baseline exceptions remain.
- [x] Deploy Simulator software and controller/proxy prebuilds while preserving
  runtime configuration, production services and both hosted DNS relays.
  Verify manifest/status/scenario APIs and both Business cards.
- [x] Show organization Restore progress, rollback and completed/failed outcomes
  above the catalog, regardless of active/selected card. Show immediate submit
  feedback and logs during execution; retain failure log access on catalog errors.
  Six focused desktop/tablet feedback tests and live version/status-element
  checks pass; the embedded fix is installed.

The user's Restore failed at policy-assertion settings verification and the
previous runtime was restored and verified. A read-only post-rollback probe
found matching organization/base DN but empty assertion source at revision 1.
No repair or retry is claimed. New provider provisioning and both live access
matrices are still unverified. The operation result is in memory and was cleared
by the UI deployment restart; the local reset log remains available.

## Scenario gateway response results

Results navigation follow-up (2026-10-15 UTC): added Runner / Results tabs
beside the run heading, including an observation count, disabled empty state,
keyboard navigation and selection preservation across polling. Results replace
the tracks instead of appearing below a long runner. Clearing evidence returns
to Runner. Eight desktop/tablet browser checks pass, including a 40-step runner,
viewport visibility and existing status/action placement. This UI follow-up is
verified locally, not deployed; the release below predates the tabs.

Verified locally on 2026-10-15 UTC (2026-10-14 EDT). An isolated Results-only
release was installed in Simulator and the controller binaries at 00:49 UTC;
live acceptance was blocked by the pre-existing manifest error at that release.
The 00:56 UTC business-service executable subsequently resolved that error;
no live forwarding acceptance run is claimed. Source is included in the current
publication; Runner/Results tab rollout remains separate.

- [x] Retain a versioned structured HTTP observation from the gateway client
  through scenario execution/API snapshots, including responses that fail
  allow/deny expectations, network errors and interrupted response bodies.
  Emit observations before propagating command failure; keep verdict separate.
- [x] Show a Results section with response code/reason, URLs, elapsed time,
  decoded bytes read, content type, bounded/redacted headers and original
  bounded raw body source. Never execute fetched HTML or parse/reformat raw
  JSON; preserve non-UTF-8 bytes as labeled base64. Mark truncation, read-limit
  byte counts and missing/old-controller evidence explicitly.
- [x] Permit gateway fetches with no expectation, while retaining existing
  organization allowlist and denial-probe restrictions. Preserve explicit
  allow/deny behavior. Clear results when changing/clearing runs and avoid
  rebuilding unchanged result views on polling.

Six desktop/tablet browser checks pass, including inert malicious-looking
HTML, exact JSON/large-number retention, failed expectations versus observed
responses, empty/no-response/binary/unavailable states, and existing scenario
run controls. Focused Go race tests, vet and embedded dashboard build pass.
Go tests exercise real loopback redirects/HTTP 502, response header redaction
at both ends, partial-body/network errors, output emission on failure, byte/
preview bounds, binary preservation, duplicate/invalid/missing contracts and
observational destination validation. Required `make test` still fails in
four existing groups: enrollment readiness, stale policy-bundle installation,
scenario placement fixture extraction and trusted-service Linux executable
fixture validation. No full-suite pass is claimed.

Current-run storage only; durable result history, downloads and other result
types are not implemented. Both Simulator and machine-controller executables
have been updated; exact release assets/hashes and restored DNS relay traffic
are verified. The isolated release's full suite has the same 13 failing groups
as its clean base, including nine additional external-runtime/seed-dependent
failures outside the original workspace. No network scenario run or Control
Room/production-service dependency change was performed. See the
[deployment and rollback record](GUI%20deployments.md#scenario-gateway-results-deployment-2026-10-15-0049-utc).

## Publication status (2026-10-14)

All pending repository source, tests and documentation were committed and
pushed to `origin/main` in `e1b24ab` (`Complete GUI workflows and service
updates`). Historical "not committed or pushed" statements below describe the
state at the time of those checks, not the current publication state.

The full dashboard worktree executable was deployed to Simulator at 22:35 UTC
and Control Room at 22:43 UTC on 2026-10-14. This includes shared editor
gutter/undo changes, organization structured editors and toolbar alignment,
and the GUI polish below. It does not deploy the standalone discovery CLI,
replace Control-Service or Policy-Service, certify draft activation, or imply
an authenticated Control Room browser check. See the [release ledger](GUI%20deployments.md#deployment-status).

Final pre-publication validation: `cargo test --workspace` passes and
`git diff --check` passes. The full dashboard `go test ./...` run passes the
discovery CLI/engine, DNS-record, policy-client and operator-auth packages, but
fails four dashboard test groups:

- `TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator`: invitation
  creation lacks a current organization-scoped readiness preflight.
- `TestOrganizationPolicyInstallNeverUploadsStaleBundle`: its success fixture
  lacks `ZPR_MULTINODE_PLATFORM`.
- `TestScenarioPlacementPreflightMatchesLauncher`: its extraction cannot find
  `resolve_machine_placement`.
- `TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact`: its helper
  fixture is not a Linux amd64 ELF executable.

The earlier organization archive HTTP 404 failure recorded under discovery
verification was fixed before publication; focused archive/permission tests
pass and it is absent from the final full-suite failures. No full Go-suite
pass is claimed. The Playwright suite was not rerun for the final release
because Node/npm were unavailable; earlier selected browser results remain
historical evidence.

## Control Room and Simulator GUI polish

Verified locally on 2026-10-14. Simulator portion deployed at 2026-10-14
22:35 UTC; Control Room portion deployed at 22:43 UTC. Committed and pushed
in `e1b24ab`; see the publication status above.

- [x] Restyle the shared Help trigger consistently across Control Room and
  Simulator.
- [x] Tighten Simulator Trusted Sources spacing and show People, Groups and
  Attributes counts beside their tabs.
- [x] Simplify scenario cards: remove redundant runner/version/internal-name
  details and Delete controls, place the title first, keep Edit under File...,
  preserve the actual Finance/Runtime/Workday folders, and move the machine
  limit into Scenario Help.
- [x] Simplify Security Review table headers while retaining the selection
  checkbox; label observed time as TIME and use the shared bold monospace
  header treatment.
- [x] Remove redundant Activity timestamp, stream-state and request/approval
  metrics while keeping explicit failure status.
- [x] Keep Control Room Trusted Sources' Updates (24h) feed as live change
  metadata, separate from Simulator's record browser.
- [x] Move Simulator organization editing into the right-side viewer's File...
  actions; allow Rename/Delete only for private drafts, protect bundled and
  active organizations, confirm deletion and use revision-checked soft
  archival. Consolidate New, Import and Duplicate under New organization.

Focused Go archive/permission tests and the operator-auth package tests pass.
After the Simulator deployment, 76 directly served assets matched the
worktree; live browser checks confirmed the connected organization viewer,
draft-only File actions, New/Import/Duplicate mode switching, scenario folders
Trusted Sources counts, Activity's Recent visas/Recent denials view without the
removed metrics, and Scenario Help's machine limit. Control Room restarted
with the updated executable; its root and operator-config endpoint return 200,
the unauthenticated snapshot remains 403, and all 76 served assets match the
worktree. The signed-out browser showed the operator login prompt; no
authenticated post-login browser check was performed. The Playwright suite
could not be run because Node/npm are unavailable; no automated browser-suite
pass is claimed.

## Migration discovery CLI

The original slice was published in `e1b24ab`; subsequent progress/deadline
changes are included in the current publication. The standalone CLI is not
deployed by dashboard-only releases.

### Progress and scan lifetime follow-up (2026-10-15 UTC)

- [x] Remove the overall scan deadline and reject obsolete `duration` config/
  CLI options. Preserve bounded per-operation timeouts, worker/start-rate limits,
  explicit authorization and interrupted partial-report export.
- [x] Emit progress immediately, periodically while waiting, and on completed
  checks. Keep report JSON separate on stdout; stderr adapts to terminal width/
  resize or emits plain redirected lines. Never fabricate completion on cancel.
- [x] Distinguish pingable clients with completed/no-open-port checks from hosts
  with open TCP endpoints; count services as observed open endpoints, not
  inferred application identities. Progress-output failure cancels scanning and
  exports partial results before reporting the error.

Current CLI/engine race checks and full-suite package tests pass. No new
authorized real-subnet scan, remote scanner selection, authenticated adapter
collection or migration-policy generation is claimed.

### Unlimited target ranges and list configuration

- [x] Remove the 256-host and CIDR-size restrictions in parsing/scanning and
  the fixed imported-host count limit (retain the 2 MiB import-size limit).
  Store sorted, merged inclusive ranges and iterate hosts lazily instead of
  preallocating a record for every requested address. Reject ranges crossing
  unsupported address types rather than silently trimming them.
- [x] Require config `targets` to be a nonempty string array with one IP,
  CIDR or dash range per item. Support full inclusive IPv4/IPv6 endpoints
  and IPv4 final-octet shorthand. Preserve CLI comma-separated targets and
  whole-list CLI replacement. Reject old string-valued config targets,
  empty/invalid arrays and wrong element types.
- [x] Introduce version-2 report/preview scope contracts with compact ranges
  and exact decimal-string host/planned-check counts (including counts above
  64 bits). Keep reached/queued hosts and all imported evidence in partial
  exports; unrepresented scoped hosts mean not attempted, never absent/down.
  Keep version-1 adapter import compatibility. Existing authorization,
  rate/worker/time, output privacy and no-overwrite restrictions remain.

All 86 focused checks in the editor runner pass, as do focused engine/CLI race
tests, vet, formatting, native build and Linux/Windows cross-builds. Tests cover
508-host multi-CIDR unions, a /22 and /8, a 2^96-address IPv6 scope, inclusive
cross-subnet/IPv6/shorthand ranges, invalid/intersecting address types, overlap
deduplication, early iterator termination, bounded large-scope cancellation,
300 imported hosts retained before scanning, config-list/CLI precedence and
loopback dash-range export. The sample config preview and a no-network preview
of 1,063 targets (a /22 plus a dash range) pass. No real subnet scan was run.

Required `make test` was rerun. The same four dashboard failures described in
the original-slice section remain, plus
`TestOrganizationDraftArchiveRequiresCurrentRevisionAndHidesDraft` reports
HTTP 404 `unknown organization` while testing stale archive behavior. That
organization/archive code has concurrent changes and was not modified by this
discovery update. No full-suite pass or Linux/Windows runtime certification
is claimed.

### Ping, reverse lookup and config-file extension

- [x] Enable one ICMP echo and one system-resolver reverse lookup per explicit
  target by default, independently disableable with `-ping=false` and
  `-reverse-dns=false`. Never skip TCP checks because of host-check results.
  macOS/Linux use their installed ping utilities without a shell or privilege
  escalation; other platforms require disabling ping. Missing utilities and
  permission/resolver failures are explicit rather than silent fallbacks.
- [x] Keep worker/start-rate limits and per-operation/overall deadlines shared
  across TCP and host checks. Export separate host-check states, timestamps,
  reverse names and error/attempt counters. Cancellation includes not-attempted
  host checks; reverse names are unverified system-resolver evidence.
- [x] Add bounded JSON config input, explicit CLI precedence (including false
  switches), strict key/type/duplicate/null/trailing-data validation and a
  documented sample. Authorization remains CLI-only; dry runs do not invoke
  ping or DNS.

Focused race tests, vet, native build and Linux/Windows cross-builds pass.
New coverage includes ping utility statuses/diagnostics/deadlines, PTR lookup
results/failures, config precedence, dry-run defaults, config-driven private
export, disabled-check compatibility and TCP scanning despite failed ping.
Real macOS IPv4/IPv6 loopback verification returned ping replies and `localhost`
names, completed all 2 TCP and 4 host checks, and exported a private report.
No subnet was scanned again for this extension. `make test` was rerun and
still fails in the same four existing dashboard tests listed below; no
full-suite pass or Linux/Windows runtime certification is claimed.

### Original standalone slice

- [x] Add the standalone `zpr-discover` command and standard-library-only
  `internal/discovery` engine. No Simulator, Control Room, runtime manifest or
  live adapter dependency is introduced.
- [x] Require explicit IP/CIDR scope, TCP ports, scanner/organization labels and
  operator authorization confirmation before scanning. A dry run validates and
  prints scope without making network connections. Limit host/port counts,
  concurrency, dial-start rate, per-operation timeout and overall scan duration.
- [x] Export private, no-overwrite version-1 JSON inventories with per-target
  results, explicit closed/timeout/unreachable/error/not-attempted distinctions,
  scan provenance and cancellation/partial-result reporting.
- [x] Keep common-port service hints separate from optional bounded SSH banner,
  HTTP HEAD and certificate-verified TLS evidence. Do not follow redirects,
  attempt authentication, send application requests on TLS-only checks, or
  infer application protocols from TLS handshakes.
- [x] Validate and preserve imported adapter listener inventory separately,
  including bind addresses, TCP/UDP listeners, process/service labels and
  timestamps. Mark it `imported_unverified`; no claim of live adapter
  collection or remote reachability is made.

Focused engine/CLI race tests and vet pass. Native macOS build and Linux amd64/
Windows amd64 cross-builds pass; runtime execution on Linux/Windows is not
claimed. The documented adapter-import dry run passes. Tests cover exact scope
caps, deduplication, port states, pacing across workers, partial cancellation,
bounded checks, untrusted TLS certificates, JSON contracts and private
no-overwrite publication. Only local loopback/in-memory fixtures were used;
no real network discovery run or policy change was performed.

Required `make test` was run. It currently fails in the existing dashboard
tests `TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator` (readiness
preflight rejection), `TestOrganizationPolicyInstallNeverUploadsStaleBundle`
(unbound platform variable), `TestScenarioPlacementPreflightMatchesLauncher`
(missing extracted launcher function), and
`TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact`
(fixture is not a Linux amd64 executable). Those tests/code were not changed
by this slice; no full-suite pass is claimed.

Control Room discovery UI, remote scanner selection/control, authenticated
adapter collection, UDP scanning and migration policy drafts remain future work.
See [CLI usage and export contract](zpr-dashboard/cmd/zpr-discover/README.md).

## Editor gutter synchronization and undo

Verified locally on 2026-10-13; not deployed, committed or pushed.

- [x] Assertion and Policy gutters rebuild blank rows for the current logical
  source lines after newline insertion/deletion and Undo/Redo. Obsolete analysis
  markers are cleared rather than retained against changed source.
- [x] Trusted Source group/attribute assertion insertions use the shared
  undo-aware edit helper and dispatch normal input notifications, invalidating
  prior analysis and keeping gutters, highlights and dirty state current.
  Selection replacement, source qualification, read-only restrictions and
  character limits remain enforced.
- [x] Extract the existing native-edit/Undo/Redo fallback from the AI shell into
  the service-independent editor blueprint. AI insertion remains compatible;
  Find & Replace, policy completion, and Assertion/Policy/Scenario indentation
  reuse the same helper instead of bypassing undo with `setRangeText`.
- [x] Refresh shared asset versions and load the editor blueprint before the
  AI shell on organization pages.

Ninety focused desktop/tablet browser cases pass, including twelve new cases
covering gutter row counts, newline deletion, wrapping, native and fallback
catalog undo/redo, selected-text replacement, stale-marker clearing, input
limits, policy completion, Scenario indentation and fallback Find & Replace.
Related assertion analysis/format scope, shared scroll layout, search and AI
tests also pass. The embedded dashboard build and editor diagnostics pass.

## Services selector and standard log display

### GUI-only publication verification

The 2026-10-10 GUI-only source snapshot excludes unfinished Mac provisioning,
Great Lakes Rust/runtime changes, provider-manager launcher changes and
file-source runtime-state backend changes. It includes completed GUI changes
and the directly required Diagnostics node-source filter and login retry fix.
Historical "no commit or push" statements below describe their deployment time.

Verification in an isolated snapshot: 144 desktop/tablet browser cases pass
for Nodes, Services, maps and shared logs; targeted production Diagnostics/
log-boundary/node-name race tests, operator-auth race tests and the embedded
dashboard build pass. The full dashboard Go package run has four unrelated
organization/Simulator fixture failures (missing directory seeds/manifest and
LDAP identity expectations); it is not claimed passing. No additional deploy
is part of publication.

- [x] Replace the main Services page's multiple log lists with a service
  selector and the shared `log-panel.js` viewer. Show only the selected service's
  health and sortable metrics, preserving non-node service/trusted-source
  inventory counts, aggregate health and identity/metric/log filtering.
  Leave the Status service-descriptor tab unchanged.
- [x] Preserve exact raw JSON, inert provider HTML and independent ANSI record
  styles. Provide Pause/Resume, manual Refresh while paused, Wrap initially off,
  standard maximize/restore/Escape/focus, per-service scroll/follow retention,
  navigation cancellation and session-loss clearing.
- Verification: all 14 desktop/tablet Services cases pass, plus 18 existing
  Nodes ownership/shared-viewer and window-control regression cases.
  Focused production Diagnostics API tests and the
  embedded dashboard build pass; edited JavaScript has no editor problems.
  Empty inventory, stale/partial/unavailable samples, cached-log retention,
  duplicate IDs and malformed metrics/logs are covered.
  Legacy Diagnostics tests still describe multiple ordered log lists, uncapped
  log height and all-source detail tables; those expectations are superseded,
  not rewritten without approval or claimed passing.
- **Verified; awaiting deployment.** Frontend only: existing aggregate API caps
  still apply; no selected-service API request or backend/Simulator change.
  No deployment, commit or push.

## Diagnostics/Nodes ownership merger

- [x] Move selected-node source health and metrics into Nodes alongside its
  shared log viewer. Retain aggregate health and the source overview in
  Diagnostics, with non-node service/trusted-source details unchanged.
  Node overview rows select and navigate to Nodes; unknown inventory nodes
  have disabled navigation with an explicit explanation.
- [x] Share exact sortable metric rendering between the two views. Preserve
  large integer strings, units, refreshed change pulses and per-node sort
  choices. Keep last healthy/stale metrics independent from partial log
  samples; transport failures retain both with explicit last-known labels.
  Selection/navigation cancellation rejects obsolete health/metric/log
  responses, and session loss clears the selected-source caches.
- Verification: 96 desktop/tablet browser cases pass, covering merger ownership,
  aggregate/service retention, exact integers/sorting/pulses, malformed metrics,
  partial logs and failed reads, narrow source-panel bounds, inert provider text,
  session-loss cancellation, node switching and shared Adapter/Controller/Worker
  rendering/window regressions. Embedded dashboard build, editor problems and
  whitespace checks pass. Presentation fixtures now use non-node services
  with user approval. The previously superseded Diagnostics JSON-toggle test,
  known legacy picker lookup and topology-parent position failures are excluded;
  those tests are not rewritten to hide failures.
- **Deployed to Control Room**, verified 2026-10-10T00:50:31Z. Rollout includes
  `source-metrics.js` v1, `node-logs.js` v2, `node-stats.css` v7,
  `diagnostics.js` v11 and the corresponding index references. Reuses the
  deployed Control-Service node-source filter; no backend change, Simulator
  request or Simulator rollout is required for this merger.
- All 96 cases pass against exact release assets. Real Dex-authenticated
  desktop/tablet checks verify selected-source health, exact returned metrics
  and logs, pause/refresh/resume/window controls, aggregate health, 10 non-node
  detail sources and navigation from Diagnostics to the selected node.
  Maps/Fit and timeout retry remain working. All 69 served asset hashes,
  configuration/network contracts and 17 unrelated container starts verified.
  First attempt rolled back because the old map smoke required docks absent
  from the current production inventory; corrected snapshot-grounded checks
  verify two geographic anchors and zero expected docks. No map code change.
  Rollback retained stopped; no commit or push. See deployment ledger.

## Compact unconnected components

- [x] Bring undocked adapters next to the occupied graph area in both maps.
  Placement measures existing actors/service/cloud extents and packs a separate
  group to the right or below with 40-unit clearance. Side and column count
  minimize viewport-fit size. No fixed x=4500 offset or node-row staging gap.
  Unconnected-only World Map groups are centered on the basemap without
  inventing geographic coordinates or dock/network edges.
- Verification: 50 desktop/tablet geography cases pass, including six new
  landscape/portrait, mixed-size gateway, exact gap, component separation,
  refresh/reordering stability, Fit containment and unconnected-only checks.
  Geographic anchors and existing map tests are preserved. Embedded build,
  editor problems and whitespace checks pass.
- **Deployed to Control Room**, verified 2026-10-10 00:15 UTC. `app.js` v148.
  All 50 cases also pass against the exact release assets. Live desktop/tablet
  checks verify four real undocked components at the exact 40-unit group
  clearance in both maps, geographic anchors, Fit, Nodes logs and Diagnostics.
  Backend unchanged; no commit or push. See the deployment ledger.

## Viewport-aware map layout

- [x] Base both map layouts on the actual graph viewport, not a square grid or
  fixed vertical fan-out. Topology scores node rows/columns against aspect and
  orientation while retaining stable slots. Free-host columns are aspect-aware.
  Two-adapter clusters follow landscape/portrait orientation; World Map fan-out
  scoring balances aspect fit with component separation, inter-node corridors
  and basemap containment. Geographic coordinates remain authoritative.
- Significant aspect changes recalculate layout in both views. Manual camera
  transforms and disabled Auto-fit remain preserved; Fit/Auto-fit use the actual
  viewport aspect for both maps. Hidden/zero-size viewports are ignored.
- Verification: 44 desktop/tablet geography cases pass, including landscape to
  portrait rotation, exact geographic anchors, refresh stability, Topology dock
  orientation and manual-camera retention. Existing geography tests are
  unchanged; embedded build, editor problems and whitespace checks pass.
- **Deployed to Control Room**, verified 2026-10-10 00:09 UTC. `app.js` v147.
  All 44 geography cases also pass against the exact release assets. Live
  Dex desktop/tablet checks verify both maps' viewport-aspect Fit, geographic
  anchors, selected-node logs, Diagnostics and no Simulator API requests.
  Only app.js and its captured live index cache link changed. Backend unchanged;
  no commit or push. See the deployment ledger for rollback and hash evidence.

## Login-timeout retry correction

- [x] Fix the bare "Operator authentication unavailable or denied." response
  after the timeout retry action. Chromium reproduces `Origin: null` on the old
  direct POST under `no-referrer`; changing only that policy still leaves the
  provider redirect blocked by `form-action 'self'`. The single existing button
  now GETs Control Room, whose normal page initiates the exact same-origin login
  POST and allows the provider redirect. Timeout-page privacy/CSP, HTTP 403,
  one-time login validation and strict login origin checks remain unchanged.
- Verification: desktop/tablet browser cases use actual recovery markup and
  Control Room assets, verifying one fresh POST with the exact origin, no
  callback-parameter referrer leakage, and no CSP redirect violation. Full
  operator-auth race tests and embedded dashboard build pass. The directly
  related old recovery action expectation was updated with user approval.
- **Deployed to Control Room**, verified 2026-10-10 00:24 UTC. Exact-release
  desktop/tablet tests and isolated authentication race tests pass. Real Dex
  desktop/tablet checks verify the timeout button reaches a fresh login through
  an exact same-origin POST and successful subsequent authentication.
  Current maps, Nodes logs and Diagnostics remain verified; no commit or push.

## World Map variable dock lengths

- [x] Remove the equal-length dock constraint in World Map while retaining
  configured geographic node positions. Adapter distances now account for
  individual service rings and gateway clouds. Cosine-rule angular spacing
  separates differently sized components and retains link-corridor clearance;
  contained placements are preferred, with size-aware outside placements when
  the fan-out cannot fit. Topology spacing is unchanged. This is not a global
  collision solver for separate nodes' clusters.
- Verification: 42 desktop/tablet geography cases, including eight new cases
  for linked/unlinked mixed-size docks, exact anchors, component separation,
  unchanged/reordered refreshes, Topology preservation, missing geography and
  dense fan-outs. Existing tests remain unchanged. Current static assets build
  against the committed backend in isolation; the working-tree build was blocked
  by concurrent enrollment syntax errors, which were not modified.
- Status: **Deployed to Control Room** on 2026-10-09 at 21:04 UTC, `app.js`
  v146. Real authenticated desktop/tablet checks verify three geographic node
  anchors, five adapter docks with variable lengths and Fit on production data.
  No commit or push. See [deployment evidence](GUI%20deployments.md).

## Selected-node logs and shared viewer

- [x] Add the selected production node's logs below its statistics on Nodes.
  Use stable node identity for queries and operator display names for headings.
  Keep the aggregate Diagnostics page, including service/trusted-source logs,
  metrics and source health, until replacement views are ready.
- [x] Extract `log-panel.js` and wire Nodes, Adapter/Controller Logs and Simulator
  Workers to the same panel construction, safe ANSI rendering, scroll/follow
  behavior and Maximize/Restore controls. Preserve adapter/Worker JSON formatting,
  source selection, disconnected tails, filtering, layout and polling contracts.
- [x] Keep node bodies raw and body-only; reset ANSI state per provider record,
  leave HTML/terminal links inert and preserve exact large-integer JSON text.
  Use standard black terminal styling, Wrap initially off, local Pause/Resume,
  manual Refresh while paused and Escape/focus restoration. Restore/detach the
  viewer on navigation and remember each node's scroll/follow state.
- [x] Use the authenticated independent production Diagnostics API with an
  optional inventory-validated `source` filter. Resolve before aggregate caps
  and query only the selected source; retain the existing unfiltered behavior.
  Reject invalid/unknown IDs explicitly. No Simulator endpoint, configuration,
  manifest, workload or availability dependency is introduced.
- [x] Preserve initial Loading, explicit request/source failures, stale/partial
  signals and the selected node's last observed logs. Cancel on navigation/
  selection changes and reject obsolete responses without showing another
  node's data.
- Verification on 2026-10-09: **96 desktop/tablet browser cases pass**, including
  16 new Nodes/shared-panel cases plus existing terminal layout, icons, exact
  raw/formatted JSON, source filtering, disconnected tails, Nodes/geography and
  Diagnostics regressions. Focused production Diagnostics/log-boundary/node-name
  Go tests pass with the race detector; the complete embedded dashboard builds.
  New Go cases verify source-scoped provider queries, selection beyond aggregate
  inventory/log caps, exact metric strings, provider absence without Simulator,
  invalid/repeated/malformed IDs and rejection without provider calls.
- Baseline exceptions: two older picker tests (four desktop/tablet cases) still
  request a combobox for a multi-option listbox, and the topology-parent animation
  position test (two cases) fails its existing expectation. Both failures were
  reproduced against committed HEAD assets/tests and left unchanged by this
  task; the final passing selection excludes them. A new current-contract picker
  regression verifies per-type selection and per-source scroll retention.
- Status: **Deployed to Control Room/Control-Service** on 2026-10-09 at
  21:04 UTC. Real authenticated desktop/tablet checks compare selected-node
  rendered logs with actual provider records for two production nodes; verify
  Loading, Pause/Refresh/Resume, window controls, navigation restoration and
  aggregate Diagnostics. No Simulator API requests or Simulator restart.
  Shared Worker-panel rendering still awaits Simulator asset rollout.
  No commit or push. See [deployment evidence](GUI%20deployments.md).

## Control Room guideline follow-up

- [x] Standardize window-control button styling, not only SVG geometry:
  editor and adapter/controller/Worker log controls use dark icons on white
  32px squares, with shared hover/active, disabled and keyboard-focus treatment.
  Verified 2026-10-09: 24 desktop/tablet runtime/header/window cases and embedded
  build pass. Window cases check actual computed colors, icon stroke, button
  dimensions and all interaction states across six editors and all log types.
  Existing focus restoration and Enter/click/Escape behavior are preserved.
  `app.css` v93 and `safe-display.js` v3 are wired across all HTML consumers.
  This color/style follow-up is deployed to Control Room (verified 2026-10-09
  19:24 UTC); Simulator rollout remains pending. Not committed or pushed.

- [x] Recover from a timed-out operator sign-in instead of leaving a bare
  "Operator authentication unavailable or denied." page. Expired, missing-cookie,
  lost-state and replayed callbacks retain HTTP 403 with an accessible
  single **Timed out. Try again.** POST action, without a return link. A retry
  creates fresh browser-bound state/nonce/PKCE; the rejected callback never
  exchanges a code or creates a session. Four new recovery cases and existing
  OIDC/operator regressions pass; dashboard build and whitespace checks pass.
  Deployed 2026-10-09 at 18:09 UTC; source changes not committed or pushed.
  - Refreshing an expired Dex login page also gets **Sign-in timed out** and a
    single **Timed out. Try again.** action instead of "Bad Request". The local
    pinned-Dex image has a scoped error template; other provider errors remain
    explicit. The action opens Control Room to start a fresh same-origin login.
    Three template cases and an isolated real-Dex initial/refresh check pass.
    Deployed desktop/tablet browser regressions pass, including refresh and
    clicking the recovery action to create a fresh same-origin login POST.
    Existing IdP identities, certificates, SQLite mount and configuration
    preserved except selecting the custom frontend template directory.

Implemented, verified and deployed to Control Room at 2026-10-09 15:37 UTC.
The implementation was committed in `59f0bda`; it has not been pushed.

- [x] Use a shared two-pulse change treatment with reduced-motion fading.
- [x] Cover Status counts, service uptime, Diagnostics, Security and live inspector values.
- [x] Compare stable record/column identities after each asynchronous source update; no false pulses on sorting/selection.
- [x] Make all table headings bold and numeric headings/values right-aligned.
- [x] Enforce the new 11px minimum text size in Control Room, excluding maps, without changing Simulator typography.
- [x] Keep the top banner application-wide; separate Control Room transport from Visa Service source health.
- [x] Move permanent Nodes, Gateway, DNS and Provisioning explanations into Help; preserve operational errors and consequential warnings/confirmations.
- [x] Use sortable Diagnostics source/metric tables with expandable log details.
- [x] Remove the recurring Diagnostics Querying/Updated status line. Show "Loading" during the initial request only; hide it after completion/failure, preserve explicit errors and retain existing data without loading chatter on subsequent refreshes.
  - **Deployed to Control Room** on 2026-10-09 at 19:45 UTC, together with body-only/raw/full-space layout. Exact live-backend reproduction, isolated embedded build, 16 desktop/tablet release-asset tests and real authenticated desktop/tablet Diagnostics checks pass. Only Diagnostics JS/CSS and its live HTML block/cache links changed; all 66 assets verified, configuration preserved and 17 unrelated service starts unchanged. Simulator and concurrent enrollment/review/forwarding work excluded. No commit or push; see [deployment evidence](GUI%20deployments.md).
  - Browser-verified on 2026-10-09: 16 desktop/tablet cases pass, including delayed initial success/failure, recovery, retained content during refresh/failure, stale-response ownership, body-only/full-width layout and ANSI safety. Editor diagnostics and `git diff --check` are clean. The standard fixture runner and embedded build were blocked by concurrent enrollment edits; the final build retry reported undefined `err` in `setup.go` and an unused `bytes` import in `device.go`. Browser tests ran directly in the existing Playwright container. No unrelated enrollment code changed; no deployment, commit or push performed.
- [x] Remove Diagnostics Format JSON and always render provider body text with ANSI colors intact. Use the full available application width, remove the 720px metrics width limit and fixed 220px log/68vh source-table height caps. Retain natural page scrolling, narrow-table horizontal scrolling, filtering and bounded provider responses.
  - **Verified, awaiting deployment** on 2026-10-09: 16 desktop/tablet cases and embedded build pass. New tests measure full application/metric widths and uncapped log/source height at 2400px, 1280px, 820px and 700px, with 100 log records, no page-wide overflow and no Format JSON control. Body-only text, ANSI safety, raw exact JSON, source filtering, navigation ownership and header regressions pass. Editor diagnostics and `git diff --check` are clean.
  - The older Diagnostics JSON-toggle and sortable/pulse tests still exercise the removed control; left unchanged per repository guidance and excluded from this focused run. Current body/color tests were updated to the raw-only requirement. No deployment, commit or push performed.
- [x] Remove duplicate timestamp/date/time and severity columns from Diagnostics logs. Display each provider body at full width, retaining embedded metadata, multiline text, ANSI colors, JSON formatting and explicit missing/empty-body states.
  - **Verified, awaiting deployment** on 2026-10-09: 34 selected desktop/tablet browser cases and the embedded build pass, including new full-width/body-only checks at desktop/tablet and 700px, retained colored metadata, empty-body handling, safe ANSI rendering, JSON toggling, source tables/pulses and navigation cancellation. Editor diagnostics and `git diff --check` are clean. No deployment, commit or push performed.
- [x] Respect provider-supplied ANSI colors and emphasis in Diagnostics log bodies using the shared adapter/controller/Worker renderer. Preserve standard/RGB foreground and background colors, reset record styling independently, keep provider HTML/terminal links inert and retain Format JSON behavior.
  - Deployed to Control Room and verified 2026-10-09 19:29 UTC; shared Worker
    renderer still awaits Simulator rollout. No commit or push. See deployment
    ledger for scoped asset, backend-preservation and real-login evidence.
  - **Verified, awaiting deployment** on 2026-10-09: 48 desktop/tablet browser cases pass, covering new colored Diagnostics, foreground/background/bold/reset, malicious HTML/inert hyperlinks, exact large-integer JSON toggling, refreshed colors, shared terminal log regressions, window controls and navigation ownership. Production Diagnostics makes no Simulator requests. Embedded dashboard build, editor diagnostics and `git diff --check` pass. No dependency added; no deployment, commit or push performed.
- [x] Sort loaded Provisioning queue pages without changing server pagination or authorization.
- [x] Sort Assertion catalogs/results and inspector counters without changing source-line/evaluation ownership.
- [x] Verify desktop/narrow layouts, keyboard sorting, exact integers, pulses and existing refresh semantics without Simulator.
- [x] Visa Service uptime has an independent red/green dot and duration beside Control Room status in the top banner; remove the separate "Visa Service connected" text and lower source-summary row. Green represents a connected snapshot; partial/unavailable/unconfigured states are red and explicitly labelled. A failed snapshot transport clears the duration rather than presenting stale uptime. Deployed at 2026-10-09 16:33 UTC: 28 desktop/tablet uptime, failure/recovery, layout and pulse regressions plus the embedded build pass. This update is not yet committed.
- [x] Follow-up: remove healthy "Available" and "connected" text from the top header. Use concise Control Room and Visa Service titles, with Visa Service "Uptime ..." on the secondary line. Preserve independent red/green lamps and explicit partial/unavailable/not-configured/transport-failure states.
  - **Verified, awaiting deployment** on 2026-10-09: 42 desktop/tablet browser cases and the embedded dashboard build pass. New header coverage checks healthy/zero uptime, source failures, transport failure/recovery, hidden healthy status, secondary-line geometry, the 11px minimum, 700px layout and no Simulator requests. Related polling, pulses, Diagnostics and Trusted Sources cases pass. Editor diagnostics and `git diff --check` are clean.
  - The two older uptime/banner tests still assert the superseded "Control Room connected" / "Visa Service uptime" text. They were left unchanged and excluded from this focused selection; the new header test covers the revised requirement and failure/recovery behavior. No deployment, commit or push was performed.
- [x] Replace window Maximize/Restore text with shared standard icons across Control Room and Simulator: one outlined window for Maximize, overlapping windows for Restore. Cover Policy, Assertions, Config, Gateways, Scenario and Directory editors plus adapter/controller/Worker log panels. Preserve tooltips, accessible names, pressed state, keyboard actions and existing maximize ownership; record/base-state Restore actions remain textual.
  - **Verified, awaiting deployment** on 2026-10-09: 28 desktop/tablet cases and the embedded dashboard build pass. New cross-app coverage checks identical 16px SVG geometry, no visible button text, accessible labels/tooltips, Enter/click/Escape transitions, restored editor focus and preserved nonmodal editor dialogs. Shared runtime and header regressions pass; editor diagnostics and `git diff --check` are clean.
  - Two older editor-maximize tests assert visible "Maximize"/"Restore" text. They remain unchanged per repository guidance and are excluded from this selection; the new icon tests cover all six editor surfaces and logs. No deployment, commit or push was performed.
- [x] Extract shared neutral page polling/lifecycle helpers: start/stop on navigation, serialize refreshes, abort obsolete requests, reject late responses and retain last-good data with explicit errors.
  - [x] Add dependency-free `page-runtime.js`; retain the existing Simulator navigation poller entrypoint as a compatibility adapter, with no production dependency on Simulator.
  - [x] Migrate Control Room snapshots, DNS statistics/record caching, Diagnostics, Security scans, adapter/Worker logs, Policy context checks and manual Trusted Sources reads. Preserve background inventory alerts, disconnected tails, paused manual Refresh and independent DNS name caching.
  - [x] Cancel Trusted Sources requests on route departure/disconnection without introducing periodic LDAP reads or collapsing retained trees. Preserve per-feed cursor ownership, late-tab guards and explicit expired/unconfirmed feed errors.

Verification: 72 combined desktop/tablet browser cases pass, including measured
text sizes, page overflow, keyboard sorting, exact integer comparisons, source-local
evaluation, async change pulses and production-only Diagnostics. Focused node and
Diagnostics Go tests, the embedded Control Room build, JavaScript syntax checks
and whitespace checks pass. Three unrelated older tests (Assertion reload via a
hidden File menu item and two outdated Security inventory/markup expectations)
fail identically on committed baseline `e41af68` in both viewports; those tests
were not modified.

## Overall

### Help
- [x] Put help button in the upper-right corner of the page (when it is in the nav it is unclear that it is page specific).
- [x] Show the help text in a pop-up dialog window
- [x] Improve the help text explanation of what things are and what they do.
- [x] Show examples when applicable (mainly the editors and organization or scenario creation).
- [x] Policy and assertion help text needs much more - ZPL guide, assertion guide, explanation of the security/service/client security model, where trusted sources fit in...
- [x] Move the uptime and refresh to be next to the help button.
- [x] Help text is still not following the checked off Improve directive above.

### Page headers
- [x] Get rid of page title since the nav already indicates it.
- [x] Pages shouldn't have their own "REFRESH" buttons when they are part of the global refresh updates.
- [x] Only one line between the topmost status/refresh panel and the content below.

### All text editors
- [x] Add search and replace functionality. Put the buttons on the right side before History. Shared local controls cover policy/assertion, ZPR Config, scenario JSON, and directory LDIF source editors.
- [x] Add AI Assistant
- [x] Add task-specific embedded SKILL.md instructions for Policy, Assertions, ZPR Config, Gateways, Simulator directory LDIF, scenario design and organization design; preserve response contracts and operator-only application.
- [x] Make Search and Replace one button/ Clicking the button should show or hide the dialog. Button should change colors while in use.
- [x] Remove "Enter text..." in Search and Replace.
- [x] Add a checkbox to Search and Replace to enable/disable regular expressions
- [x] Remove line and column numbers from Search and Replace.
- [x] Replace needs to support undo in the editor.
- [x] Rename Search & Replace to Find & Replace
- [x] Find & Replace button doesn't change color.
- [x] Make the Find & Replace dialog mimic Microsoft Word but with support for regular expressions (if checked).
- [x] Word wrap should be a check box, and default to enabled
- [x] Analyze and Format should be next to File...
- [x] Analyze should use the same colors, fonts, etc. as the Policy editor.
- [x] Change "Word wrap" to "Wrap".
- [x] Use the Policy editor as the blueprint for every editor: shared `editor-page.js` core for Gateways, ZPR Config, Simulator directory and scenario editors.
- [x] Remove the centred editor kind label and idle/"Select a…" placeholder text from all editor pages.
- [x] Open the Simulator directory and scenario editors as full pages instead of modal dialogs.
- [x] Keep source-line errors in the gutter only; show service/configuration failures under the control buttons and above source in all six editors. Do not assign invented source lines.
- [x] Remove extra top spacing and the separator above editor controls.
- [x] Remove the naming placeholder and New/unsaved label from the ZPR Config editor.
- [x] Remove the ZPR Config name box above Browse; ask for a name on Save.
- [x] Match the ZPR Config editor bottom margin to Policy and keep long documents scrolling inside the editor.
- [x] Place History beside the file name in every editor, removing separate version labels.
- [x] Display Untitled for unnamed drafts and prevent saving Untitled as a name.
- [x] Wrap should start as disabled for all editors; a user's explicit saved Wrap choice is remembered.
- [x] Fix assistant key setup: persist the key securely for both Control-Service and Simulator, use it for readiness and requests, and explain the supported setup command. Reload the existing Control-Service container in place so key setup preserves configured LDAP/admin settings.
- [x] Add Rename to the Browse right-click menus for Policy, Assertion, and ZPR Config records.
- [x] AI Assistant doesn't need the 'Use assistant' checkbox. Configured assistants are ready; requests still require explicit Send.
- [x] AI Assistant Model and Max output (should be max tokens) should be collapsible, and start off collapsed. Shared Model and max tokens preferences apply to text and design assistants.
- [x] Make sure AI changes are able to be undone. Text Insert supports native/fallback undo/redo; Scenario and organization-directory Apply provide guarded Undo AI change/Redo AI change without saving or publishing.
- [x] Does the AI assistant also send attributes and groups to Anthropic? Policy/Assertions attach source/group definitions and configured attribute definitions, but never load a People catalog or memberships. Per clarification, do not expand disclosure or change Directory/Scenario payloads.
- [x] Change 'Ask about this policy' to 'How can I help?' in every assistant.
- [x] Put source-located assertion lint warnings in the gutter as clickable warning markers; keep warnings without valid source lines visible below the controls rather than assigning invented locations.
- [x] If an assertion evaluation has warnings, color Analyze yellow while keeping errors red and warning-free success green.
- [x] Add common assertion expressions to the contextual help: group cardinality, exactly-one and mutually exclusive membership, attribute presence, approved values, and exact-value matching.

### All log viewers
- [x] Word wrap checkbox defaults to disabled; users can enable it per view.
- [x] Change "Word wrap" to "Wrap"
- [x] Wrap should start as disabled for all log viewers.
- [x] Toggle between raw and formatted JSON in Adapter/Controller Logs, Workers, and Diagnostics. Keep raw as the default, preserve numeric/string tokens, and leave plain-text or malformed entries unchanged.

### Logging in and out
- [x] If there isn't a properly authenticated and permissioned user don't show anything other than a sign in box with similar styling can colors as the app. Failure to sign in should show the error and allow retries.
- [x] Skip the "you're not logged in" dialog box and go directly to the identity provider login page. Failed/denied callbacks remain retryable, and explicit sign-out does not automatically sign back in.
- [x] Recheck the operator session after protected API 401/403 responses and open sign-in when the session is expired; a still-valid session/permission denial or Visa Service outage does not trigger a login redirect.

### JavaScript reuse refactoring
- [x] Extract a shared editor controller for Policy, Assertions, ZPR Config, Gateways, Simulator Directory and Scenario, with explicit adapters for load, analyze, save and domain-specific rendering.
  - [x] Register one controller per source; share Scenario form/raw ownership and retain Policy/Assertion's shared File/History chrome without duplicate listeners. Directory has no Analyze adapter.
  - [x] Centralize command/Save-shortcut bindings, named request scopes, pending cleanup and disposable menu/History/viewport setup. Preserve independent production and Simulator contracts, domain cancellation, diagnostics, warnings and save/publish/stage confirmations.
  - [x] Share File menu interaction across all six editors: visible/enabled action focus, arrow/Home/End navigation, Escape focus restoration and outside-click dismissal.
- [x] Consolidate editor title/Untitled handling, reserved-name validation, History placement, dirty state, discard confirmation and Save keyboard shortcuts.
  - [x] Share Ctrl/Cmd+S handling across Policy, Assertions, Config, Gateways, Directory and Scenario. Use each editor's enabled Save action; ignore modified, composing, repeated and already-handled shortcuts.
  - [x] Use the shared identity renderer in all six editors, including Policy/Assertions; preserve domain-owned dirty checks and draft naming. Clear obsolete version tooltips when identities change.
  - [x] Share dirty-state discard confirmation across all six editors, preserving action-specific warnings, cancellation and reset behavior.
  - [x] Use the shared History controller and version renderer for Policy/Assertions as well as Config, Gateways, Directory and Scenario. Keep revision loading domain-owned and prevent another editor's History selection from invoking Policy revision loading.
- [x] Consolidate Analyze state, source-owned diagnostics, diagnostic clearing and stale-response rejection without mixing service/configuration errors with source-line errors.
  - [x] Share valid source-line bounds checking across source surfaces and Policy/Assertion gutter rendering. Place non-line editor errors under controls; Assertion errors without valid locations remain visible there instead of receiving line-1/clamped markers.
  - [x] Share source/context analysis guards for Gateway and raw Scenario. Reject superseded runs, edited-then-restored source, and changed record/revision/organization/format context without applying obsolete success or error results.
  - [x] Use the shared analysis-context guard for Config Analyze and validation-before-Format. Keep mutation error handling separate; stale responses cannot format replaced text or attach obsolete diagnostics.
  - [x] Use the shared analysis-context guard for Assertion evaluation, preserving AbortController cancellation and record/organization/revision ownership. Source edits, Tab insertion, catalog insertion and record changes invalidate obsolete results.
  - [x] Use the shared analysis-context guard for Assertion Format. Reject obsolete source, warnings and errors after edit-return/context changes; abort record/navigation changes without clearing a newer pending action. Apply current formatting as an undoable unsaved input edit.
  - [x] Use the shared analysis-context guard for Policy compiler checks, preserving source/record/revision/organization ownership, request-owned cancellation and pending state. Cancelled checks stop analysis and Save/Save As continuations; current failures remain explicitly saveable with errors.
  - [x] Use the shared analysis-context guard for Policy runtime fixtures/evaluation with independent request ownership. Reject late results, dimensions, warnings and failures; cancel source/record/revision/organization replacements and stop obsolete Save/Save As/Stage continuations without releasing a newer pre-save test.
  - [x] Use the shared analysis-context guard for structured Scenario Analyze. Reject superseded requests, edited-then-restored/replaced forms and changed identity/revision/organization/viewed revision/mode/open state. Raw analysis also rejects closed-editor and viewed-revision changes.
  - [x] Share explicit idle/pending/success/error button-state transitions in the five Analyze-capable editors. Preserve readiness, disabled actions, diagnostic rendering and stale-response guards; do not add an Analyze action to Directory.
- [x] Consolidate JavaScript editor-frame setup and responsive viewport sizing so individual editors do not duplicate layout wiring.
  - [x] Apply the shared source viewport contract to all six editors; reschedule on open/visibility/mode/resize changes, preserve minimum usable source height and natural small-screen scrolling, and let Maximize own full-page sizing.
  - [x] Give the existing shared Policy/Config viewport binding idempotent setup, coalesced scheduling and disposal/rebind contracts without changing sizing rules. Test hidden containers, pending-frame cancellation, restored inline height/priority and responsive geometry.
  - [x] Share source scrolling and resize observation across all six editors, preserving overlay alignment, native versus translated gutters, domain-specific gutter bounds and Policy completion positioning. Expose disposal through bindings/source surfaces; avoid rebuilding Assertion highlights on scroll/resize.

Editor-controller completion was deployed to local Control Room and Simulator
on 2026-10-09 at 00:49 EDT using `zpr-editor-ui:20261009-unified`.
The editor assets match the working-tree source on both dashboards, including
shared core v16, Control Room application v133, Scenario v37/raw v9 and
Directory v20. The build preserves the recorded 04:33 backend baseline and
seven-method draft validator, excluding the later source-only Gateway
forwarding/consolidation changes. Container configuration is unchanged.
Only the two UI containers were replaced; Control-Service, Policy-Service,
DNS and all Great Lakes runtime start times are unchanged. Simulator DNS
statistics/records relays were restored and verified. Great Lakes remains
active with activation idle and three scenarios. A live read-only Scenario
Analyze succeeded without saving, publishing or running. Operator login
readiness and signed-out protected API rejection remain correct. Stopped
rollback containers are retained. These source changes were subsequently
committed in `24fbf5f` at 00:53 EDT.
The combined desktop/tablet regression run passed 261 cases;
five trace-cleanup failures (missing shared artifact files, not editor
assertions) passed in an isolated container-local rerun of 22 cases, including
all six registrations/viewports, pending lifecycle, History, cancellation and
Maximize/Restore. Focused Go editor/API and production-boundary tests pass.
The older Assertion transparent-gutter expectation and hidden Gateway-toolbar
tests also fail against committed baseline assets; neither test was changed.

- [x] Extract a shared AI assistant shell and conversation controller from `editor-assistant.js` and `design-assistant.js`: messages, model/token controls, usage counters, pending/error states and collapse behavior. Policy/Assertion uses the same `assistant-core.js` controller.
- [x] Share assistant stale-context/proposal protection while keeping undoable text Insert and validated structured Apply as separate adapters; never save, publish, activate or run automatically. Reject edited-then-restored source, changed record/revision/organization, reset conversations and stale failures.
- [x] Consolidate HTTP/JSON error handling and cancellation; keep authentication, same-origin CSRF handling and mutation retry/uncertainty policies explicit.
  - [x] Delegate editor and assistant JSON transport to the neutral runtime. Reuse it for production telemetry, trusted directory/feeds, Policy File operations, Activity, Simulator organization operations and policy browsing.
  - [x] Preserve strict enrollment status/content-type checks and unconfirmed mutation handling; share only response decoding there. Preserve Scenario Delete's empty-success contract and specialized operator login/HTML navigation flows.
  - [x] Migrate Simulator Activity polling to the shared navigation lifecycle; abort on departure and ignore late responses while retaining the last successful activity data.
  - [x] Migrate Simulator Scenario polling to the shared navigation lifecycle; abort on departure, serialize periodic/manual refreshes and ignore late catalog/run responses.
  - [x] Migrate Simulator Organization catalog polling to the shared navigation lifecycle; keep last-good catalog data and preserve faster polling during activation.
- [x] Extract shared sortable-table helpers for column definitions, comparators, accessible sort headings and empty states, starting with Control Room tables and Simulator Activity.
  - [x] Share value comparison, accessible sortable heading setup, and empty-row rendering between Control Room tables and Simulator Activity; keep dataset-specific columns, stable tie-breaking, and row rendering owned by each view.
  - [x] Reuse shared value comparison and accessible sort headings in the Trusted Sources browser while retaining its empty-values-last ordering and stable row tie-breaks.
- [x] Consolidate safe text rendering and timestamp/protocol display helpers without changing API contracts or losing integer precision.
  - [x] Share HTML escaping across Control Room and Assertion rendering, and reuse protocol labels in Control Room and Activity; retain caller-specific fallback text for unknown protocol numbers.
  - [x] Route date/time locale formatting through shared helpers without changing caller-selected date/time style, input values, or seconds-to-milliseconds conversions.
  - [x] Replace the remaining Activity, Organization and Scenario local escaping implementations with the shared HTML-escape helper; retain Activity's em-dash fallback.
- Open requirement moved to [active GUI work](GUI%20work.md); completed substeps below remain historical evidence.
  - [x] Share injected JSON transport for Config, Assertions and Gateways. Preserve structured errors and abort identity; report malformed JSON explicitly rather than defaulting to empty success. Keep Gateway validation-result acceptance local to Analyze and reject failed saves without retrying or replacing unsaved drafts.
  - [x] Use shared injected JSON transport for Simulator Directory artifact/revision reads, Save and Publish. Retain explicit operation errors, unsaved drafts, expected-revision payloads and separate publish/reseed semantics without retries or production dependencies.
  - [x] Use shared injected JSON transport for Scenario form/raw Analyze, artifact/history/revision reads, Create, Save and Publish. Keep source-local diagnostic mapping after stale-context checks; preserve draft/revision ownership and explicit mutation payloads without retries, implicit publish/run or production dependencies.
  - [x] Use shared authenticated JSON transport for Policy compiler checks and runtime fixture/evaluation requests. Preserve non-success compiler diagnostic priority/warnings, separate runtime errors and cancellation-safe pending/Save ownership without retries or Simulator dependencies.
  - [x] Use shared authenticated JSON transport for Policy workspace, record, History and revision reads. Show load failures separately from analysis diagnostics, retain source on failed reads and clear load status on recovery/new drafts. Preserve History/dirty-confirmation behavior without retries or production/Simulator dependencies.
  - [x] Use shared authenticated JSON transport for Policy revision Save, draft creation through Save and confirmed Stage. Preserve revision payloads, error dialogs and explicit confirmation without automatic retries or treating undecodable mutation responses as success.
  - [x] Use shared JSON response/error parsing for Simulator Organization and Scenario polling reads; retain their independent service endpoints, abort signals, error surfaces and last-good data, with no mutation retries.
  - [x] Use shared JSON response/error parsing for Scenario run/cancel/clear mutations; report undecodable responses explicitly and preserve single-attempt semantics.
- [x] Reuse the existing shared `machine-logs.js` viewer rather than introducing separate Control Room and Simulator log implementations.
- [x] Add shared JavaScript component contract tests and desktop/tablet integration coverage for each migration, preserving intended behavior and accessibility.
- [x] Verify Control Room works with Simulator unavailable; shared JavaScript must receive independent production endpoints/configuration and must not introduce Simulator data, authorization or runtime dependencies.

The page-runtime/HTTP reuse completion is **Verified**, not deployed, on
2026-10-09. The final focused/expanded browser run passed **172 desktop/tablet
cases**, including 18 new runtime/ownership cases, malformed DNS response
retention, late Diagnostics/Trusted Sources success and failure after departure
and return, existing Simulator polling, editor transports, assistant context,
provisioning registry/mutation safeguards and manual-only LDAP behavior.
Production browser cases made no Simulator requests. Six targeted Go
production-boundary/API tests and the embedded dashboard build passed;
editor diagnostics and `git diff --check` were clean.
The existing `visa refresh preserves current grants and resolves DNS labels`
test failed its DOM-node-retention assertion in both layouts in the broader
run. The same two failures were reproduced using committed HEAD application
code; that test was not modified and is excluded from the final passing
selection. No dependency, deployment, commit or push was added by this task.

## Control Room

### Map
- [x] Animate retained adapters when topology bounds move, avoiding sudden jumps.
- [x] Fit centers the rendered topology bounds with padding and preserves glyph proportions.
- [x] Remove the large empty area below the Map canvas.
- [x] Adapter connections to nodes sometimes cover connections between nodes. This may be a generic problem with the map placement algorhythm.
- [x] Auto-fit (fit after/during refresh) should be a checkbox.
- [x] Use solid lines for connecting components.
- [x] Put a little number beside adapters and services showing how many active visas they have.
- [x] Make pulsing even more apparant.
- [x] not all of the connectors are starting at the edges (non circulat shapes show this issue more than others).
- [x] add a dark mode check box.
- [x] Since Nodes don't show visa count - show the count of buffered denies in a similar fashion but use red (and the same outline rules)
- [x] When the number of visas or buffered denies change, do a little pulse.
- [x] Pulse the connector in addition to the adapter.
- [x] Does the map come up blank because it is missing info, or just waiting for a refresh to fill it?
- [x] Possibly related - when I switch to the map from another page, the map is only partially drawn until a refresh.
- [x] Show management and per-fastpath packet-processing counters in node details when a node is clicked, with explicit unavailable/stale telemetry states.
- [x] Add Monitoring > Node Stats with a node selector, live state, denial telemetry, grouped management/fastpath counters, exact counter integers, sample freshness and explicit errors; share global snapshot polling without a Simulator dependency.
  - Navigation renamed Nodes; Docked adapters shows a clickable count expanding a name-sorted adapter/address table, with sortable headings, refresh retention and an explicit empty state. Sixteen focused desktop/tablet cases pass. Deployed in the Control Room-only Nodes update above; served assets match source and unsigned snapshot access remains protected.
  - City display names deployed 2026-10-09: `node0.demo` = Milwaukee, `node1.demo` = Shenzhen, `node2.demo` = Tijuana. Independent operator IPv6/name configuration labels maps, inspectors, actor tables and Nodes; the Nodes summary retains the CN as Node identity. CNs, addresses, authentication, dock/link sets and 146 counters per node remain unchanged. Control-Service and Control Room use `zpr-editor-ui:20261009-city-node-names`; preserved the pre-forwarding backend plus the deployed trusted-feed fix and only city-label changes. All 61 unrelated served assets and 15 unrelated service start times remain unchanged. Concurrent Trusted Sources/enrollment changes and the source-only map-refresh fix were excluded. Live app.js version 140 contains only naming additions to the previously deployed version 137; source version 141 reserves fresh cache invalidation for the other unpublished work. Exact release desktop/tablet naming tests and focused backend tests pass; broader geography run passed 33/34, with a tablet map-stability failure also reproduced with naming changes removed. No authenticated live browser interaction claimed; service API verifies labels and clean snapshot, and unsigned UI snapshot remains HTTP 403. Initial verification rolled back safely because adapter lists changed order; corrected verification compares sets, then deployment succeeded. Stopped `zpr-control-service-city-names-rollback` and `zpr-control-room-city-names-rollback` retained.
  - Fastpath counters use one sortable comparison table (Counter, then number-only worker columns) beside a narrower management table, without redundant cumulative-total captions. Deployed in the Control Room-only update above; tests cover exact numeric sorting, refresh preservation and responsive layout.
  - Changed summary/counter values pulse twice, including decreases/reset to zero; stable node/counter/worker keys avoid false pulses on sorting or node selection. Reduced-motion mode fades instead of scaling. All fourteen Node Stats desktop/tablet cases pass. Deployed in the Control Room-only update above.
  - Verified eight new desktop/tablet browser cases and focused Go node-telemetry tests. Deployed in the UI-only update above; live authenticated desktop/tablet checks confirm all three nodes expose 146 counters each without snapshot errors.
  - Repaired the local operator exporter resource identities to match production source mappings. Live Control-Service now reads 146 packet counters per Great Lakes node (plus denial metrics), without counter/denial telemetry errors; no node/service restarts required.
- [x] Add a right mouse click on adapters to highlight current visa allowed services and routes. Right click again on the adapter, or in blank space to remove the highlight.
- [x] Hide the dark mode checkbox for now.
- [x] The visa and buffered denies still don't pulse as far as I can tell. Are we using our expand/contract paradigm.
- [x] Make the right-click highlight stuff work for services as well.
- [x] Right-click on visa count should show the visas.
- [x] Right-click on Buffer denials should show the details.
- [x] Move the small number indicator to overlap the top right corner of the objects. For the visa service put it overlapping the center of the upper right line. For circles upper right line and overlap it. For future shapes do the same kind of thing.
- [x] For 0 visa or buffered denials, fill in the circle with white.
- [x] When the visa count is zero, show the same circle, but just as an outline with nothing in it.
- [x] When the user interacts through panning and zooming, turn off Auto-fit.
- [x] Make right-click on a component should have the same behavior as right-clicking on its number.
- [x] Clicking on the component legend items should highlight what has been clicked on.
- [x] No obvious way to clear highlighted areas or dimmed components.(or the component) Closing the info dialog on the right doesn't always remove the highlight.
- [x] This gets worse with the click-on legend.
- [x] The right-click (and legend) behavior should simply highlight, no dimming of the other stuff.
- [x] What happened to showing visa routes? Routes now draw as thick glowing blue lines with moving dashes and pulsing routed components; right-click only toggles routes (count badge click opens the visa list).
- [x] Link forward and reverse visas for display: paired on the Visas page (PAIR column) and in Map visa lists.
- [x] Make clicking away from the component info dialog close the window.
- [x] In both map views, arrange each node's docked adapters into a compact arc in the largest link-free sector. Grow the radius to account for gateway clouds and service rings rather than letting a wide arc wrap across inter-node connectors. Nodes without inter-node links retain a spaced ring. Cover gateway-heavy, multi-link layouts in desktop/tablet regression tests.
- [x] Show each gateway with a visible connector to a labeled, cloud-shaped external-network component in both Topology and World Map.

### World map
- [x] Fix viewport jerking on unchanged refreshes: compare viewport scales only after Auto-fit establishes the final viewBox, not against temporary pre-fit geometry. Follow-up 2026-10-09 also removes SVG intrinsic-size feedback into the surrounding flex/grid layout and reuses geographic content bounds when component geometry/labels are unchanged, ignoring contact/status and counter-badge widths. Explicit Fit remeasures, viewport resize still refits, and actual component changes still reframe. Deployed Control Room-only `zpr-editor-ui:20261009-world-map-stable`, app.js 143/app.css 91, preserving the uptime/status, city-name, trusted-feed and Trusted Sources baseline; public enrollment URL and unapproved forwarding remain excluded. All 34 exact-release desktop/tablet geography tests, 10 repeated unchanged-refresh cases, six shared manual-camera/Auto-fit regressions and focused release backend tests pass. All 64 served assets match the release, configuration is preserved, and all 16 unrelated service start times are unchanged. Live snapshot has no errors and three city-named nodes with 146 counters each; feed and unsigned snapshot protection (403) remain intact. No authenticated live browser interaction claimed. Stopped `zpr-control-room-world-map-stable-rollback` retained. New follow-up source changes are uncommitted/unpushed.
- [x] `worldmap.svg` is available in the `zpr-dashboard` folder.
- [x] Plan for a geographic network view using node latitude/longitude:
  - [x] Add optional validated latitude/longitude properties to production node metadata and the Visa Service Admin API; never infer coordinates from location names or read Simulator profiles in Control Room.
  - [x] Inspect the supplied SVG license/viewBox; because its projection is undocumented, use a public-domain Natural Earth basemap generated in a known equirectangular projection with explicit bounds and a clear no-coordinate state.
  - [x] Add a separate World Map view while retaining Topology as the default; both now share authoritative component, link, route and runtime-state rendering.
  - [x] Reuse node details/selection and keyboard-accessible graph components; selecting duplicate/co-located nodes in the main graph opens an explicit responsive chooser without a separate location overview.
  - [x] Test projection bounds, missing/invalid/zero coordinates, mobile sizing, replacement node sets, and Simulator-unavailable production metadata pass-through.
  - [x] Seed all eight nodes across the five bundled Simulator organizations with explicit approximate coordinates and per-node guess notes; provision them into Visa Service config under actual runtime CNs without adding a reverse Simulator dependency to Control Room.
  - [x] Deploy to authenticated Control Room on HTTPS 8787 and verify Great Lakes through real Dex login: all three actual node CNs match their profile coordinates and all reported network components render.
  - [x] Complete second-organization live certification. Repaired Redwood's shared compiler contracts and made policy installation fail closed on merge/build/missing-output errors instead of uploading stale bundles. Real Dex-authenticated desktop/tablet checks passed for Redwood's two nodes (Eureka and Sacramento), then Great Lakes was restored and passed the same live checks for its three nodes (Milwaukee, Shenzhen, and Tijuana). Activation requires every configured node synchronized; Great Lakes is active with no snapshot errors, and no Redwood containers remain.
  - [x] World Map supports the same zoom, wheel, pan, Fit, Auto-fit, inspection, counts, route focus, search and legend controls as Topology; both use the same renderer with geographically constrained nodes and independent saved cameras.
  - [x] Rename Geography to "World Map".
  - [x] Add country borders using a locally served public-domain Natural Earth country polygon basemap.
  - [x] Use light land/ocean colors so the existing network overlay stays readable.
  - [x] Switching back to Topology rebuilds connectors and route highlights immediately, without waiting for polling or a cross-layout animation.
  - [x] Use high-contrast dark text on light inactive buttons and white text on dark-blue selected buttons.
  - [x] Remove the placed/unplaced-node count, Node location overview, Nodes with locations list, and "Countries..." text. Preserve keyboard-accessible co-location choices in the main graph and explicit missing/invalid-location inspection. Deployed and verified through real Dex login on desktop and tablet.
  - [x] Auto-fit and Fit frame the network components rather than the full geographic basemap. When there are no reported nodes or adapters, keep the basemap visible and Fit falls back to its full extent.
  - [x] Autofit and fit aren't doing what I asked above. World Map Fit now reframes network bounds using the current viewport aspect after manual navigation, and Auto-fit follows viewport resizing. Empty maps retain full-basemap fitting; Topology camera behavior is unchanged. Desktop/tablet regressions pass; deployed with live desktop/tablet Fit verification.
  - [x] Another constraint is to keep the connections as short as possible. Removed World Map's fixed 340-unit dock radius; compact radii are derived from node/adapter clearance and adjacent component spacing, growing only as needed for service rings, gateway clouds, and link-free sectors. Four plain adapters use a 106-unit radius with no overlap. Geographic nodes stay at their reported coordinates and Topology retains its spacing. Desktop/tablet regressions pass; deployed in the UI-only update above.
  - [x] Make the background the same color as the map background. World Map viewport matches the ocean fill and removes the Topology dot grid, including with the retained dark-mode class; switching back preserves Topology backgrounds. Implemented and verified with desktop/tablet geographic regressions; deployed and live desktop/tablet background checks pass.

### Navigation
- [x] Keep Map as the main view and group Adapters, Actors, Services, Visas, Denials, and DNS under horizontal Status tabs with counts.
- [x] Keep summary status metrics on Map only.
- [x] Add Security, Trusted Sources, Logging, and Editor navigation entries with separators.
- [x] Add a ZPR Config menu item for managing versioned ZPLC configuration drafts. Validate and save drafts without applying them to the live runtime.
- [x] Add Trusted Sources links to provider control GUIs using server-configured provider URLs; hide links for providers without a configured manager.
- [x] Rename Logging to "Adapter Logs" and have it show the adapter logs. Remove the arrow. Log Manager remains a separate Tools link.
- [x] Move "Gateways" and "ZPR Config" under editor. Rename editor 'Policy".
- [x] If there is a subtle way of adding a title to the policy/gateways/... group, call it 'Configuration'.
- [x] Move Log Manager to below Adapter Logs. Make the arrow more visible.
- [x] Clicking on "Log Manager" should switch focus to the Log Manger.
- [x] Rename "ZPR Config" to "Config" in the navigation.

### Service logs and statistics
- [x] Add one Control Room diagnostics view for logs and current stats from every ZPR node and every configured trusted/required service used by Visa Service.
- [x] Standardize ingestion on OpenTelemetry through a ZPR observability trusted service. Put a provider-neutral catalog/query contract behind Control-Service; OpenObserve is the initial replaceable backend, not a UI/API dependency.
- [x] Show source identity/type, last update, stale/unavailable states, bounded searchable logs, and service stats. Keep credentials and provider queries server-side; do not depend on Simulator.
- [x] Test nodes and each configured service class, unavailable/stale sources, redaction/size limits, and Control Room behavior with Simulator unavailable.
- [x] Put "Monitoring" as a header for the Map, Status... area.
- [x] Add another separator below ZPR Config named "Provisioning"
- [x] Add the new provisioner to the nav and call it "Adapters"

### Security review
- [x] Remove the investigation-leads read-only heading and redundant baseline/dismissal explanations.
- [x] Label the browser-local inventory as Baseline and align its count and timestamp with Current.
- [x] Remove the redundant Security Review heading and place Reset Baseline beside the baseline summary.
- [x] Remove the Scan complete message.
- [x] Use explicitly labeled compact controls: Show dismissed controls visibility; Select visible is the bulk-selection checkbox.
- [x] Highlight the nav when a high alert is noticed. This may entail changes to the prioritization of detections (for example - Actor first observed, is not a high alert; multiple requests being blocked could be a high alert - especially if addresses aren't found, varied attempts, etc.)
- [x] Clear the security highlight in the nav once the page is visited (until another security alert)
- [x] Make the security highlight only color the little side indicator, not the main body area.
- [x] Flag sustained aggregate DNS NXDOMAIN probing in Security Review as an unattributed review finding; do not raise the high-priority nav alert from server-wide counters.

### Editor
- [x] Show one Assertion Analyze gutter tag per source line, with ERR > FAIL > WARN > PASS precedence; clicking retains every result and warning for that line.
- [x] Restore Assertion Analyze's production trusted-directory configuration and match the standard editor's white lower-left scrollbar corner on horizontal overflow.
- [x] If there is a horizontal scrollbar, color the forbidden area under the gutter white.
- [x] Keep every source-located Analyze warning in the gutter on its reported line; never render a duplicate warning list above or below the editor. Keep line-less analysis failures in the status area rather than inventing line 1.
- [x] Analyze of a define-only policy turned red with no message. Keep line-less compiler/analysis errors visible while Analyze is red, and omit directory attributes the ZPT fixture format cannot carry (e.g. Great Lakes `user.l` with commas) instead of failing every analysis; fail with a named-attribute message only when the policy references one.
- [x] Keep line-specific policy warnings in the gutter; show analysis failures without a source line in the status area instead of attaching them to line 1.
- [x] Rename "Rescan LDAP" to "Refresh Attributes", and make sure it handles all trusted attribute sources.
- [x] Put "Policy" or "Assertion" above the editor to be clear of the mode. Center between the left and right button groups.
- [x] Get rid of the extra line above the name of the policy/assertion being edited.
- [x] Add an indicator to the policy/assertion name line to indicate if the file has been modified. Don't show anything if it isn't.
- [x] Match the assertion editor gutter to the other editors.
- [x] Change Nav from "Policy" to "Policy/Assertions".
- [x] Remove Syntax from the bottom of the assertions editor.
- [x] Change "Analyze" color to blue instead of orange across Policy, Assertions, Config, Gateways and Scenario, including hover and pending states. Keep success green and errors red.
- [x] Support Maximize/Restore across Policy/Assertions, Config, Gateways, Scenario, and Directory editors through the shared editor-page foundation. Maximize fills the viewport, prevents background interaction and page scrolling, and restores focus; Scenario/Directory dialogs remain open when Escape restores their normal layout.

### Status
- [x] I noticed on the "adapters" page that even when sorted, the records moved around (though nothing changed). I think sorting only sorts on its primary field but if that field has duplicates this occurs. A solution would be to always have a secondary field (or more) that is/are automatically attached. The user has nothing to do with this but the results would be deterministic.
- [x] Status tables need vertical scroll bars when they get big.

### Adapter Logs
- [x] move the buttons to be next to "Adapter Logs".
- [x] Add a new button toggle for showing and hiding all adapters.
- [x] The Adapter picker should be a dropdown next to the adapter title. That should remove its line.
- [x] A page-wide setting for wordwrap toggling should be added next to the page buttons.
- [x] Add a button to toggle seeing only running adapters.
- [x] The adapter picker dropdown should be a pop-up dialog controlled by a place to click.
- [x] I'm confused by the new Adapter titles.
- [x] Running Only checkbox doesn't work - it shows non-running adapters.
- [x] Remove "Adapter logs" and "Controller logs" from the adapter titles.
- [x] Show the adapter name where you used to show "Adapter logs"... in the same font.
- [x] Make a chevron button next to the adapter name which should display the dropdown/dialog.
- [x] Start with Word wrap enabled.
- [x] The adapter picker doesn't need a header or anything else. It just needs the pull down to start in an open position.
- [x] If there aren't any adapters (none available) than show a message that says this/
- [x] Move the picker to be under the adapter title when opened.
- [x] The error message "Unexpected token 'C', "Client sen"... is not valid JSON" makes no sense when the real issue is the Visa service being unavailable.
- [x] Standardize adapter/controller and shared Workers log panels on the user-selected dark terminal surface, with 11px monospace text in every runtime state. Preserve ANSI colors, safe text rendering, explicit high-contrast errors and retained disconnected history.
- [x] Keep each log viewport and its horizontal scrollbar at the panel bottom. Shared column-flex layout fills space beneath uneven headers and source/detail controls; maximized panels remain bounded and independently scrollable. Deployed to Control Room at 2026-10-09 16:55 UTC; Simulator remains unchanged. Not yet committed: 38 related desktop/tablet checks pass, including 12 new state/style/geometry cases, plus the embedded build and whitespace checks. Two older picker tests fail in both viewports because they expect a combobox instead of a listbox; the same failures were reproduced on committed baseline `59f0bda` and those tests were not changed.

### Config
- [x] Create a full draft-only ZPLC form editor for resolver order/hosts, nodes/provider attributes/substrate interfaces, links/peers/attributes, Visa Service, trusted services and attribute mappings, protocols/ICMP/ports, service refinements and bootstrap public-key paths. Named entries and mapping rows can be added/removed; Form and Raw TOML share the existing revision, import, download, New, Discard, Analyze and Save workflows. Field patches preserve unrelated source, unfamiliar settings, comments and newline style; ambiguous inline/array-of-table layouts remain editable in raw mode rather than being destructively serialized. Incomplete form input blocks Analyze/Save and participates in discard guards; late responses cannot overwrite another source or record. Uses the existing Policy-Service TOML parser and independent production contracts, with Simulator deliberately unavailable in tests. Analyze remains TOML-syntax-only and Save creates a draft revision, never installation/activation. Deployed 2026-10-13; not committed or pushed.
  Validation: focused Go form/authorization/proxy tests and the dashboard build pass.
  Fifty-two desktop/tablet browser cases pass, including 18 new form cases
  against the real TOML endpoint and existing raw-editor regressions. The
  existing shared Policy bottom-margin test fails on both committed baseline
  and current assets (13-pixel deviation against a 2-pixel limit); it was
  left unchanged and excluded from the passing focused rerun. No full-suite
  pass is claimed. Deployed to Control Room and Policy-Service at
  2026-10-13 05:49 UTC; exact-release tests and real authenticated
  desktop/tablet checks pass, saved drafts/configuration preserved and
  all other services untouched. See [deployment evidence](GUI%20deployments.md#config-form-editor-2026-10-13-0549-utc).
- [x] Adopt the same look-and-feel as the policy editor. If ZPR config is limited to one file, there is no need for Browse, or some of the File commands. Refresh Attributes is also not relevant.
- [x] Keep ZPR Config toolbar roles aligned with the Policy editor: File on the left, editor mode centered, Analyze/Format in the right-side action group, and History/utilities at the right. Match the blue pending-analysis state, enabled/disabled Format styling, success/error colors, and responsive placement.
- [x] Still things to do to get the look and feel better. "Validate syntax" should act like "Analyze". There should be a gutter for errors. A Format button, a file button.
- [x] Where are the colors? Where is the gutter? Why is the "Save Draft" not part of the file pulldown? Make "File..." and Analyze use the same color and sizes of the policy editor.
- [x] Remove "New draft" at the bottom of the editor, and use that space for the editor.
- [x] Remove line numbers from the gutter.
- [x] Gutter looks different from the other editors.
- Open requirement moved to [active GUI work](GUI%20work.md); completed substeps below remain historical evidence.

### Trusted Sources
- [x] Get rid of "Read Only"
- [x] Add a typical LDAP tree view.
- [x] Don't auto refresh ldap.
- [x] All the tables should be sortable and use the same visual treatment as above.
- [x] Groups should make the cn and objectclass their own columns
- [x] People should condense each row and toggle viewing when clicked.
- [x] Add a page to view updates from the trusted source (Control Room reads the mTLS-protected trusted-service change feed through Control-Service; the Updates tab shows one day of metadata only, never attribute values).
- [x] Fix the live "No trusted-source change feeds are configured" state (2026-10-09): persist the independent `ldap` / Great Lakes LDAP feed and provider network in operator configuration, issue a dedicated Control-Service mTLS client, update the existing trusted-provider binary for lookback support, and serialize bootstrap as exactly `since=24h` rather than rejected `24h0m0s`. Deployed `zpr-control-service:20261009-trusted-feed` from the preserved `3880c6a0` backend plus seven-method validation and this feed fix; no forwarding backend or Control Room assets deployed. Focused provider/dashboard tests and exact release-source tests pass. Live authenticated service API lists the feed, returns valid empty 24-hour history and cursor continuation, and snapshot errors are empty. All 16 unrelated running service start times are unchanged; only directory and Control-Service restarted. Unsigned Control Room feed remains HTTP 403; browser reaches Dex login, so no authenticated browser interaction claimed. Retained stopped `zpr-control-service-trusted-feed-rollback` and original trusted binary `.trusted-feed-rollback`. Reload an already-open Updates page to discard its cached empty feed list.
- [x] Remove filter from the top table.
- [x] Rename lower table "Trusted source: {great_lakes_ldap}" using the source name returned by the directory endpoint.
- [x] Add a button "Manage" with an arrow for each trusted source with a known management interface (dedicated Manage column; only operator-configured URLs; reusable named window).
- [x] Change the title "Trusted source: Trusted source" and replace it with the configured source identifier, such as "great_lakes_ldap" (use the existing directory response's `default_source`, otherwise `source_name`; no invented source names).
- [x] Remove the separate "Update feed" label and pulldown. A single feed is named in the heading; with multiple feeds, the heading itself is the source selector, as confirmed by the user. Selection resets that feed's cursor and never mixes records from another feed.
- [x] Get rid of "Trusted attribute provider" under the source name (provider type remains in its own table column).

Trusted Sources follow-up deployed to Control Room at 2026-10-09 16:18 UTC using
`zpr-editor-ui:20261009-trusted-sources-gui`. Twenty-six focused
desktop/tablet browser cases pass, covering source identity, one/multiple feeds,
cursor reset/continuation, empty/error/expired states, late responses after tab
changes, manual LDAP refresh, named management-window reuse, sorting, minimum
text size and Simulator's unchanged shared browser. Focused source-contract Go
tests and the embedded Control Room build pass. No Simulator API dependency or
new automatic directory polling was added.

Exact release assets passed the same 26 browser cases before deployment.
Served index, app, sidebar, Trusted Sources JS/CSS and preserved Nodes/provisioning
assets match the release build byte-for-byte. Login is enabled; signed-out
session/feed requests return 401/403. Independent production configuration still
lists `ldap` / Great Lakes LDAP; snapshot errors are empty and all three city-named
nodes retain 146 counters. All 16 other service start times, operator settings,
mounts, network and restart policy are unchanged; Simulator remains responsive.
Stopped `zpr-control-room-trusted-sources-gui-rollback` retains the city-name image.
Preserved its backend city-name and trusted-feed fixes; unrelated World Map,
enrollment and Gateway forwarding changes remain excluded. No authenticated live
browser interaction is claimed. Implementation was committed in `59f0bda`;
it has not been pushed.

### Gateways
- [x] Gateway Save and Ctrl/Command-S automatically Analyze modified drafts, show errors without saving invalid drafts, and save a revision only after successful exact-source validation.
- [x] Support GET, HEAD, POST, PUT, PATCH, DELETE and OPTIONS in Gateway draft Form/Raw, Analyze and Save; retain GET/HEAD defaults and reject TRACE/CONNECT. This does not expand runtime handler support.
- [x] Expand fixed-upstream gateway forwarding to the seven draft methods with bounded bodies and safe headers; retain GET/HEAD health checks. Verify all seven methods and bodies through the separate HTTP/HTTPS-tunnel web proxy. Committed and tested, not deployed; draft activation remains separate.
- [x] Consolidate shared Internet-egress forwarding primitives (hostname/method validation, redirect handling, header filtering and response forwarding); remove Simulator-specific naming from the reusable web proxy while preserving both CLI modes and protocol behavior. Committed and tested, not deployed.
- Open requirement moved to [active GUI work](GUI%20work.md); completed substeps below remain historical evidence.
- Open requirement moved to [active GUI work](GUI%20work.md); completed substeps below remain historical evidence.
- [x] Simplify Gateway form rows to Destinations Allowed, Add Destination, Base URL, Paths and Remove; remove numbered titles/borders, the prefix Add button and verbose help paragraphs. Remove deletes the whole destination; Paths remains one prefix per line.
- Open requirement moved to [active GUI work](GUI%20work.md); completed substeps below remain historical evidence.
- [x] Add a Gateway form editor with draft-only HTTPS origin/path-prefix controls synchronized with the raw JSON view; keep form controls out of the raw editor and Analyze/Save separate from runtime enforcement. Include GET/HEAD, timeout/response limits, read-only installed identity, lossless unknown-field preservation and form/raw validation coverage.
- [x] Report Gateway Analyze source-line errors only through standard ERR gutter markers; clicking opens error details and selects the source line. Never put source-line diagnostics at the top of the editor; preserve exact formatting and keep live inventory/contract errors separate.
- [x] Align Gateway ERR markers at the gutter's left inset and suppress the textarea's clipped native focus outline while retaining the editor's focus-within indicator.
- [x] Add a Control Room page listing installed ZPL Gateway services by organization and editing their versioned runtime drafts.
- [x] Validate destination/path/method/timeout/response-size settings through Control-Service before saving; keep drafts organization-scoped and do not activate them.
- Open requirement moved to [active GUI work](GUI%20work.md); completed substeps below remain historical evidence.
- [x] There can be multiple gateways, so lets use the exact same editor paradigm, colors, and behavior as the polic editor.

### Header
- [x] Preserve the current application path, query and navigation fragment across operator re-login, including expired sessions and sign-in retries; restore only same-origin app locations after successful authentication.
- [x] Show the verified OIDC display name or verified email in the signed-in UI; keep the subject as an internal authorization/audit key.

## Simulator

### Navigation
- [x] Set order to: Organizations, Scenarios, Trusted Sources, Activity, Workers (merged Agents/Workload logs).

### Agents
- [x] Remove the machine count label.
- [x] Change Simulated Fleet to Devices and make the heading prominent.
- [x] Change the type filter's All Machines option to All Devices.
- [x] Merge Agents and Workload Logs as Workers, retaining passive device/runtime inspection and removing active start/stop/login controls (approved).

### Scenarios
- [x] Remove Available scenarios and the catalog-count label.
- [x] Place New Scenario above the list and label it New Scenario....
- [x] Remove the EXECUTION PLAN label from the scenario editor.
- [x] Remove the Unfiled count/header; unfiled scenarios appear without a separate count.
- [x] Show the scenario name and run state together in the run heading.
- [x] Saturate the running state; show Cancel only while running and Clear only after a terminal run.
- [x] Label the published scenario action "Run".
- [x] Pulse the running state only when scenario progress changes; respect reduced-motion preferences.
- [x] Remove the redundant Organizations list heading and profile-count text.
- [x] Keep the Activate/Active control beside the organization name and place its description below.
- [x] Make the scenario running state more visible with more saturated colors. The
Running" orange should be a non-indicated color like blue.
- [x] When a scenario fails, the step/total steps doesn't show the failure line because of the cleanup.
- [x] We need a raw version of a scenario editor (that gives the same type of experience, colors, columns, etc. as the Control Room editors)
- [x] Add AI Assistants to the dialog and raw scenario editors.
- [x] Move the raw/dialog link/button to the top right corner of the inner pages.
- [x] Implement the Analyze button with colors and behavior just like every other editor.
- [x] Switch (or just edit) scenarios in yaml.
- [x] Show shrunk-down log windows above the running simulator machines. It doesn't have to be readable. Scenario machine tracks show tiny bounded log previews above their headings while the run/cleanup is active and the machine is running, with a Workers link for full-size logs. Uses existing Simulator-only log collection and non-overlapping navigation-aware polling; unavailable/malformed responses are explicit, last-good tails remain labeled on request errors, and obsolete run/organization responses are rejected. Eighteen focused desktop/tablet regressions pass; deployed, with live log collection/idle-machine checks passing. Running-log animation awaits a live running scenario; no scenario was started for deployment verification.

### Activity
- [x] Remove the Decisions and Blocked flows headings.
- [x] Show visas and denials in sortable tables.
- [x] Place Refresh beside the Stream status in the banner.

### Organizations
 - [x] "Activate organization" and "Active" use very different styles. Make them look similar (other than the color).
 - [x] "Open reset log" should only show if there is a failure. Change the name to "See Logs".

### Workers
- [x] Add the wordwrap button like the Adapter Logs.

### Header
- [x] Put the active org name in every header, but get rid of "Active organization" and the short name.

## Provisioning

Machine/adapter enrollment is tracked in [Provisioning plan](../Provisioning%20plan.md).

- [x] Add a Control Room invitation worksheet, local review, delivery/expiry
  explanations, and enrollment help under Provisioning > Adapters.
- [x] Keep the worksheet memory-only, with explicit unsaved/unvalidated
  labels when catalogs are unavailable; local review sends nothing.
- [x] Connect approved catalogs and invitation/review actions only after HTTPS
  named-user sign-in and independently verified Control-Service delegation.
  Completed through the authorized catalog, opt-in creation and independently
  granted revision/key-bound review flows verified below; credential issuance
  and adapter admission remain separate.
- [x] Add opt-in direct HTTPS OIDC sign-in, named subject/scope display, explicit
  unavailable/expired states, and CSRF logout without unlocking enrollment.
- [x] Deploy real loopback Dex/HTTPS login for dubnom with a dedicated user-trusted
  CA and explicit all-organization admin grants; preserve default-off invitation
  creation and independent backend catalog/grants.
- [x] Preserve native login Origin under production security headers and add
  memory-only CSRF transport for policy/assertion/config/gateway editor mutations.
- [x] Verify the deployed real-provider login, wrong-password denial, authenticated
  reads, GUI config Analyze, reload and logout in desktop/tablet browsers.
- [x] Keep Simulator local HTTP; move Activity/activation reads to private mTLS
  Control-Service instead of depending on a Control Room browser session.
- [x] Add opt-in signed named-user delegation to the private enrollment API,
  independent backend grants, persistent replay protection, and named audit.
- [x] Load authorized catalogs and organization-scoped paginated registry
  list/fresh details without enabling mutations. Clear on navigation/session loss,
  discard stale responses, and distinguish unavailable from empty.
- [x] Verify read-only registry access through actual HTTPS OIDC login, mTLS
  delegation, SQLite, and logout in desktop/tablet browsers with Simulator unavailable.
- [x] Wire form selections to approved catalogs and opt-in invitation creation,
  CSRF confirmation, one-time code clearing, and uncertainty/reconciliation without
  automatic retry. Preserve existing local unauthenticated behavior and direct
  certificate administration; no email, credentials, or Simulator calls.
- [x] Verify real-service browser creation, named audit, secret-free readback,
  and committed-but-lost response recovery in desktop/tablet browsers.
- [x] Wire named-user cancellation from fresh Details without another feature
  flag: independent cancel grants, reason, revision/key binding, audit and
  uncertainty/readback acknowledgement with no automatic retry.
- [x] Verify real-service invited/pending cancellation and committed-but-lost
  response readback in desktop/tablet browsers with Simulator unavailable.
- [x] Wire revision/key-bound approval/rejection only for independently granted
  organizations; require fresh Details, trusted-channel fingerprint verification,
  a reason, explicit confirmation, named audit and uncertain-response
  reconciliation without automatic retry.
- [x] Verify approve, reject, locked-without-backend-grant and committed-but-lost
  review response through HTTPS OIDC/CSRF/mTLS/SQLite browser tests. Approval is
  only a review decision; credential issuance and adapter admission remain separate.
- [x] Add Windows 11 x64 per-user enrollment wizard support and build an unsigned
  setup EXE; use user-bound DPAPI/private ACLs and preserve keys/config on uninstall.
- Open requirement moved to [active GUI work](GUI%20work.md); completed substeps below remain historical evidence.
- [x] Build Apple Silicon per-user Mac enrollment app/DMG with explicit Keychain
  storage and no plaintext fallback; verify isolated native Keychain/restart
  tests and the packaged loopback wizard. Local build is ad-hoc signed only.
- [x] Build/test the native Mac adapter without privileged network changes;
  harden Mac tunnel/address validation with unprivileged regression tests.
- [x] Explicitly approved isolated Mac utun lifecycle smoke: fresh kernel-assigned
  interfaces, MTU 1400 then 1280, exact IPv6 /128 aliases, duplicate add/removal
  and repeated teardown. Existing interfaces/default routes/DNS match before/
  after; no running organization changed. Not packet-flow or credential certification.
- Open requirement moved to [active GUI work](GUI%20work.md); completed substeps below remain historical evidence.
