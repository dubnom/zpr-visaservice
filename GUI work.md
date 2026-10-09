# GUI Improvements

## Design Guidelines
- Values that are refreshed automatically should pulse (in a consistent way).
- Extra text, descriptions, etc. are frowned upon.
- If something needs clarity, put it in the help text.
- Data should generally be shown in a standardize sortable table. This isn't necessary for small data sets.
- Numeric table headers should right justify.
- Numbers in tables should right justify.
- Table headers should be bold.
- The topmost banner is only for application-wide information and control.
- No font sizes less than 11px - this doesn't apply to the maps.

## Control Room guideline follow-up

Implemented, verified and deployed to Control Room at 2026-10-09 15:37 UTC.
The implementation remains uncommitted and unpushed.

- [x] Use a shared two-pulse change treatment with reduced-motion fading.
- [x] Cover Status counts, service uptime, Diagnostics, Security and live inspector values.
- [x] Compare stable record/column identities after each asynchronous source update; no false pulses on sorting/selection.
- [x] Make all table headings bold and numeric headings/values right-aligned.
- [x] Enforce the new 11px minimum text size in Control Room, excluding maps, without changing Simulator typography.
- [x] Keep the top banner application-wide; separate Control Room transport from Visa Service source health.
- [x] Move permanent Nodes, Gateway, DNS and Provisioning explanations into Help; preserve operational errors and consequential warnings/confirmations.
- [x] Use sortable Diagnostics source/metric tables with expandable log details.
- [x] Sort loaded Provisioning queue pages without changing server pagination or authorization.
- [x] Sort Assertion catalogs/results and inspector counters without changing source-line/evaluation ownership.
- [x] Verify desktop/narrow layouts, keyboard sorting, exact integers, pulses and existing refresh semantics without Simulator.
- [ ] VISA SERVICE UPTIME" should have a red or green dot, show the duration. Move to top area, it is application wide. Get rid of "Visa service connected". Make it look like "Control Room connected" and put it next to it.

Verification: 72 combined desktop/tablet browser cases pass, including measured
text sizes, page overflow, keyboard sorting, exact integer comparisons, source-local
evaluation, async change pulses and production-only Diagnostics. Focused node and
Diagnostics Go tests, the embedded Control Room build, JavaScript syntax checks
and whitespace checks pass. Three unrelated older tests (Assertion reload via a
hidden File menu item and two outdated Security inventory/markup expectations)
fail identically on committed baseline `e41af68` in both viewports; those tests
were not modified.

## Deployment status

Current tracker reconciliation: 2026-10-09. The dated entries below are
historical verification snapshots, not fresh health checks.

- Control Room guideline update deployed at 2026-10-09 15:37 UTC using
  `zpr-editor-ui:20261009-control-room-guidelines`. Exact release assets passed
  72 combined desktop/tablet browser cases before deployment; all 20 changed
  or related served assets match the release build byte-for-byte. Operator
  login is enabled, signed-out session returns 401 and protected snapshot
  returns 403. Three production nodes each retain 146 counters without
  counter or denial errors. Container configuration, mounts, network and
  restart policy are unchanged; all 17 other running service start times
  are unchanged and Simulator remains responsive. Stopped
  `zpr-control-room-guidelines-rollback` retains the preceding image.
  Preserved the deployed pre-forwarding backend and seven-method validator.
  Concurrent enrollment email-draft additions and the World Map refresh fix
  were excluded from this guideline-only release. No authenticated live
  browser interaction is claimed.

- Control Room-only Nodes navigation/docked-adapter update deployed at
  2026-10-09 14:52 UTC using `zpr-editor-ui:20261009-nodes-adapters`.
  Preserved previously served assets except the four requested Nodes UI assets,
  and retained the pre-forwarding backend plus seven-method validator. Served
  index, node-stats JS/CSS and sidebar JS match source exactly; unsigned snapshot
  requests return 403. Sixteen desktop/tablet fixture cases passed before
  deployment; no authenticated live browser interaction is claimed. Container
  environment, command, mounts, ports, network and restart policy are unchanged.
  Simulator and all other running service start times are unchanged; Simulator
  remains responsive. Stopped `zpr-control-room-nodes-adapters-rollback` retains
  the preceding Control Room image. Gateway forwarding changes remain undeployed.

- Control Room-only Node Stats layout/pulse update deployed on 2026-10-09 at
  14:10 UTC using `zpr-editor-ui:20261009-node-stats-pulse`. Management is narrower
  beside the sortable worker comparison table; changed summary/counter values
  pulse, with reduced-motion fading. Served index/app/Node Stats assets match
  tested source byte-for-byte; fourteen desktop/tablet browser cases pass.
  Live Control-Service still supplies 146 counters per node without telemetry
  errors; login readiness and signed-out snapshot rejection are verified.
  User chose verification without signed-in browser access, so the deployed
  authenticated page was not interactively certified in this update.
  Preserved the pre-forwarding backend and Control Room configuration.
  Simulator, Control-Service, DNS, node and exporter start times are unchanged.
  Stopped `zpr-control-room-node-stats-rollback` retains the previous deployment.

- Runtime incident on 2026-10-09 at 13:53:50 UTC: Simulator exited with code 2
  during the Great Lakes five-minute workday; Scenario requests then failed with
  connection refused / Failed to fetch. No panic trace was present in container
  logs, and the underlying cause is not established. Restarted the same
  Simulator image and restored its relays; Scenario API and DNS statistics are
  healthy. In-memory run history reset to idle. Focused Scenario concurrency/
  machine-log race tests pass. User declined a diagnostic scenario rerun;
  no scenario was retried, and the crash remains unresolved.

- UI-only update deployed on 2026-10-09 at 13:44 UTC to Control Room and
  Simulator using `zpr-editor-ui:20261009-map-node-previews`: Node Stats,
  World Map viewport-aware Fit/background/compact docks, and Scenario miniature
  log previews. Reconstructed the same pre-forwarding backend baseline used for
  the previous image, retaining seven-method draft validation and excluding
  later Gateway forwarding/consolidation changes. Four live desktop/tablet
  checks passed: real operator login, source-matching UI assets, World Map
  Fit/background, three nodes with 146 counters each and no snapshot errors,
  and Simulator log collection/idle-machine preview behavior. No scenario
  machines were running, so running-log animation remains fixture-verified,
  not live-certified. UI environment/mounts/ports/commands are preserved;
  unrelated service start times are unchanged. DNS statistics and TCP relays
  are responsive. Stopped `zpr-control-room-map-node-rollback` and
  `zpr-simulator-map-node-rollback` retain the previous UI deployment.
- Control Room and Simulator editor-controller completion was deployed at
  00:49 EDT using `zpr-editor-ui:20261009-unified`, preserving the 04:33 UTC
  backend snapshot plus current editor assets. Control-Service and Great Lakes
  were not replaced; DNS relays were restored and stopped UI rollback containers
  retained. The temporary build snapshot was removed after deployment.
- Source changes were committed in `24fbf5f` at 00:53 EDT. Seven-method draft
  validation is deployed, but the later fixed-upstream forwarding fixes and
  shared forwarding-core consolidation are committed source only, not deployed.
  No runtime activation, draft method/path enforcement or gateway allowlist
  change is implied.
- Shared editor work is complete. General page polling/transport consolidation,
  ZPR Config forms, organization creation/workspaces and platform certification
  remain open. World Map viewport/background/compact-dock follow-ups, Node Stats
  and Scenario machine log previews are deployed as described above.

- Historical initial deployment: separate Gateway Form/Raw editor to Control Room on 2026-10-08.
  Initial form fields covered destination origins/path prefixes, GET/HEAD methods and
  request limits, with read-only installed identity. Drafts share one JSON
  source; no runtime activation is added. Eighteen focused desktop/tablet
  regressions, gateway Go tests and two authenticated live checks pass.
  During verification, Control-Service had reverted to the disconnected
  port-8184 Admin endpoint. With explicit approval, restored the Great Lakes
  port-8185 connection, gateway metadata and read-only trusted-directory settings.
  Snapshot is connected, nodes synchronized and Assertion sources configured.
- Deployed to local Control Room and Simulator on 2026-10-08 at 19:20 EDT:
  pending Policy, Scenario and Directory JSON-transport migrations below.
  Build succeeded; all eight shared/editor JavaScript assets match source on
  both dashboards, including app.js v121, organizations.js v15, scenarios.js
  v30 and scenario-source-editor.js v7. Simulator organization/scenario HTML
  hashes match, and Control Room serves the dedicated Policy load-error status.
  Operator login remains enabled; signed-out policy API returns 403. Only the
  two dashboards restarted with unchanged configuration; all other service
  start times are unchanged from the pre-deploy snapshot. Great Lakes is active,
  activation idle, assistant ready and both DNS relays responsive.
- Included in this deployment: Policy revision Save, draft creation through Save
  and confirmed Stage use shared authenticated JSON transport. Preserve
  expected-revision payloads, Save error dialogs and explicit Stage confirmation.
  Malformed responses leave revision/draft state intact and never retry writes;
  a failed response does not prove whether the server applied a mutation.
  Twelve focused desktop/tablet tests pass with isolated test output, covering
  malformed responses, Save shortcuts, save-with-errors and successful confirmed
  Stage. A preliminary run encountered shared Playwright trace cleanup failures;
  no existing tests were modified. Local Policy Format remains unchanged.
- Included in this deployment: Policy workspace, record, History and revision reads
  use shared authenticated JSON transport. Load errors now appear in a separate
  visible status above source instead of the hidden compiler-result element;
  they never create compiler/runtime diagnostics or invented source markers.
  Successful recovery and new drafts clear the load status. Twenty-eight
  focused desktop/tablet tests pass, covering malformed and structured errors,
  recovery, new-draft cleanup, History/dirty cancellation, completions and
  adjacent compiler/runtime transport. One preliminary run encountered a
  Playwright trace-file cleanup error; unchanged History tests passed on rerun.
  Mutation transport and request-lifecycle consolidation remain separate.
- Included in this deployment: Policy compiler checks and runtime fixture/evaluation
  requests use shared JSON transport through authenticated operator fetch.
  Preserve compiler diagnostic priority/warnings on non-success responses,
  distinct runtime error surfaces and cancellation/pending ownership.
  Malformed JSON is an explicit unlocated error; no retries or Simulator
  dependencies are added. One hundred two focused desktop/tablet tests pass,
  including all three malformed-response phases, compiler warnings, delayed
  source/context changes, newer request ownership and cancelled Save/Save As/
  Stage continuations. Policy mutation and workspace-load transport remain
  separate.
- Included in this deployment: Scenario form/raw Analyze and editor artifact/
  revision reads, Create, Save and Publish use shared injected JSON transport.
  Preserve raw source-local diagnostics after ownership checks, existing
  stale-response guards and explicit revision/mutation contracts. Malformed
  JSON reports errors without replacing drafts, advancing revisions or
  retrying writes; Save/Publish never run scenarios. Sixty-six focused
  desktop/tablet tests pass, including malformed responses for all migrated
  operations, creation/revision payloads, raw gutters/round trips and delayed
  form/raw analysis ownership. Scenario catalog polling, Delete and Run remain
  separate; no production dependency on Simulator is introduced.
- Included in this deployment: Simulator Directory uses shared injected JSON
  transport for artifact/revision reads, draft Save and Publish. Malformed
  responses are explicit errors without replacing source/revision state or
  retrying mutations. Production boundaries and LDAP reseed behavior are
  unchanged; organization polling/activation and Scenario remain separate.
  Twenty-two focused desktop/tablet tests pass, covering malformed responses
  for every migrated operation, successful Save/Publish/revision payloads,
  source preservation, search, menus, discard and explicit service errors.
- Deployed to local Control Room and Simulator on 2026-10-08 at 18:16 EDT:
  shared JSON transport, source scroll/resize and viewport lifecycle changes
  below. Build succeeded; live editor-page.js v13, app.js v117,
  assertions.js v20, config-editor.js v17 and gateways.js v13 hashes match
  source on both dashboards. Simulator organization/scenario HTML hashes
  also match. Named operator login remains enabled and signed-out protected
  APIs return 403. Only the two dashboards restarted; their configuration and
  all other service start times are unchanged. Great Lakes remains active,
  activation idle, assistant ready and both DNS relays responsive.
- Included in this deployment: Config, Assertions and Gateways share JSON transport
  decoding/error handling through their existing authenticated operator fetch.
  Preserve structured diagnostics, abort identity and single-attempt requests.
  Malformed JSON is explicitly reported; Gateway validation exceptions apply
  only to Analyze, never to rejected saves. No production/Simulator dependency
  or automatic retry is introduced. Thirty-four focused desktop/tablet tests
  pass, including malformed responses, structured/unlocated errors, abort and
  network-error identity, request-option forwarding, rejected Gateway saves,
  stale Config diagnostics and Assertion Format ownership/warnings.
- Included in this deployment: shared source scroll/resize binding across Policy,
  Assertions and Config/Gateway/Directory/raw-Scenario surfaces. Preserve
  native/translated gutter modes and domain-owned bounds/diagnostics; Assertion
  scroll/resize no longer rebuilds highlighted token nodes. Bindings expose
  explicit scroll/resize cleanup without clearing source or diagnostics.
  Twenty focused desktop/tablet tests pass across all six editors, including
  both gutter modes, disposal, scrolling/resize and Policy completion behavior.
  An additional existing Assertion file-controls spacing test reports a
  27px toolbar/source gap against its 9px limit in both projects; left unchanged.
- Included in this deployment: shared Policy/Config viewport sizing is idempotent
  per container and exposes coalesced scheduling and explicit disposal.
  Cleanup cancels queued frames, observers and window listeners and restores
  the original inline height/priority. Existing sizing rules are unchanged.
  Six focused desktop/tablet contract/layout tests pass. An additional
  cross-editor toolbar test fails on a hidden Gateway toolbar in both projects;
  that existing test remains unchanged.
- Deployed to local Simulator on 2026-10-08 at 17:47 EDT:
  structured Scenario analysis uses the shared
  context guard for exact form, identity/revision, organization, viewed
  revision and editor mode/open state. Edit-return, replacement, superseded
  requests and close/reopen reject obsolete success/errors. Raw analysis
  also guards viewed revision and editor close. Save/Publish/Run are unchanged.
  Focused desktop/tablet coverage: 46 tests pass, including delayed
  success/error, identical form replacement, context changes, close/reopen,
  superseded requests, raw round trips and AI Apply Undo/Redo.
  Build succeeded; live scenarios.js v29, scenario-source-editor.js v6 and
  scenarios.html hashes match source. Only Simulator restarted; configuration,
  Control Room sessions and all production/DNS/Great Lakes start times are
  unchanged. Great Lakes is active, activation idle, assistant ready and both
  DNS relays responsive.
- Deployed to local Control Room and Simulator on 2026-10-08 at 17:38 EDT:
  Assertion Format, Policy compiler checks and Policy runtime analysis
  refactors below. Live app.js v116 and assertions.js v18 hashes match source
  on both dashboards; Simulator organization/scenario HTML hashes also match.
  Named operator login is enabled; signed-out protected APIs reject requests
  with 403. Only the two dashboards restarted, with unchanged configuration.
  Control-Service, Policy-Service, DNS and Great Lakes start times are unchanged;
  Great Lakes remains active, activation idle, and both DNS relays respond.
  Build succeeded; focused desktop/tablet validation is recorded below.
- Included in this deployment: Policy runtime analysis uses the shared context
  guard for fixture loading and evaluation. Cancelled pre-save tests cannot
  cache old failures or release a newer Save; Save/Save As/Stage continuations
  stop on cancellation. Current test failures keep explicit save-anyway warnings.
  Final focused desktop/tablet coverage: 106 tests pass. Additional older
  Assertion organization-switch and copy/paste tests time out clicking hidden
  File-menu Discard/Save buttons without opening the menu (four project cases);
  those tests remain unchanged.
- Included in this deployment: Policy compiler checks use the shared source/context
  guard and request-owned cancellation/pending cleanup. Obsolete checks cannot
  decorate another draft, unlock newer checks, or continue Save/Save As.
  Current compiler failures retain the explicit save-with-errors workflow.
- Included in this deployment: Assertion Format uses the shared source/context
  guard, cancels on record/navigation changes, and keeps pending-operation
  cleanup owned by its request. Current formatted text is undoable and
  invalidates obsolete analysis/assistant suggestions through normal input.
- Hotfix deployed to Control-Service on 2026-10-08 at 17:05 EDT:
  production Policy/Assertion AI now reads attribute definitions
  through Control-Service's configured mutual-TLS Policy-Service connection;
  it no longer requires an in-process policy workspace. Context failures stop
  the model request with an explicit error. Config AI skips unnecessary
  attribute retrieval. No People catalog or Simulator dependency is added.
  Live Policy, Assertion and Config requests completed successfully using
  synthetic empty drafts. Focused Assistant/Claude/proxy tests pass, including
  real mutual TLS, no browser credentials/unrelated records forwarded, and
  explicit failures for unavailable, malformed, oversized or redirected
  context. Only Control-Service restarted; its configuration and all dashboard,
  Policy-Service, DNS and Great Lakes start times are unchanged. DNS relays
  remain responsive. Full `make test` retains the unrelated Redwood fixture
  mismatch described below.
- Deployed to local Control Room and Simulator on 2026-10-08 at 17:00 EDT:
  shared AI assistant shell/controller across all
  source and design assistants; no Use assistant checkbox; collapsed Model and
  max tokens settings; How can I help? prompt; source/context-owned responses
  and suggestions; native/fallback text undo and structured Apply Undo/Redo.
  Policy/Assertion context remains source/group and attribute definitions, with
  no People catalog or Simulator dependency. Directory/Scenario payloads are
  unchanged.
  Validation: 54 focused desktop/tablet tests and the Go Assistant/Claude
  regression suite pass; editor diagnostics and formatting checks are clean.
  Full `make test` remains blocked by the unrelated Redwood LDAP fixture
  mismatch (53 identities present; 53 people plus 3 machines expected).
  Live AI scripts, styles and editor HTML match source hashes. Operator login
  is enabled and signed-out sessions return 401; Simulator assistant readiness
  is true. Dashboard configuration and backend/DNS/Great Lakes start times are
  unchanged. Great Lakes remains active with activation idle; both DNS relays
  were restored and checked. Control Room operators must sign in again.
- Deployed to local Control Room and Simulator on 2026-10-08 at 14:25 EDT:
  Analyze idle/pending styling is blue in all five
  Analyze-capable editors; success remains green and errors remain red.
  All 14 focused desktop/tablet color and Analyze-state tests pass.
  Live stylesheets and editor HTML match source hashes; operator login is
  enabled and signed-out sessions return 401. Dashboard configuration and
  protected backend/Great Lakes start times are unchanged; Great Lakes is
  active with activation idle. Both DNS relays were restored and checked.
- Deployed to local Control Room and Simulator on 2026-10-08 at 14:20 EDT:
  shared Analyze-button states; non-line errors under controls in all six
  editors; valid-only gutter locations; shared source/context guards for
  Assertion evaluation, Config Analyze/Format, Gateway and raw Scenario.
  Edit-return cycles and Assertion Tab insertion invalidate obsolete results.
  Directory has no Analyze action; diagnostics remain editor-owned.
- Deployment verification: live HTML and changed JavaScript on both dashboards
  match source hashes; Control Room HTTPS login readiness and signed-out 401
  behavior are correct. Dashboard configurations are unchanged. Great Lakes
  remains active; backend, DNS and Great Lakes container start times were
  unchanged. Both Simulator DNS relays were restored and checked.
- Pre-deployment focused desktop/tablet suites passed: 40 error-location/layout
  and Analyze tests, 30 Gateway/Scenario guard tests, 22 Config regression tests,
  and 20 Assertion guard/integration tests (overlapping coverage).
- Deployed to local Control Room on 2026-10-08 at 12:56 EDT: shared History
  controller migration for Policy/Assertions and editor-specific revision
  selection isolation (`app.js?v=111`). Live HTML, application script and shared
  editor helper match source. Login readiness and signed-out session behavior
  were verified; 40 focused desktop/tablet regression tests passed before
  deployment. Only Control Room restarted; Simulator, its DNS relays, Great
  Lakes and backend service start times were unchanged.
- Deployed to local Control Room and Simulator on 2026-10-08:
  automatic identity-provider login, raw/formatted JSON log toggles, shared
  editor Save shortcuts, identity rendering, discard guards and File menus.
  The failure-only Simulator See Logs link remains deployed.
- Validation: 32 focused desktop/tablet tests passed for shared File menus,
  discard, identity and Save shortcut contracts. Live assets on both dashboards
  were hash-checked against source; Control Room HTTPS and login configuration
  responded successfully.
- Great Lakes remains active. Control-Service, Policy-Service, DNS and Great
  Lakes container start times were unchanged. Dashboard containers were
  updated in place; both Simulator DNS relays were restored and checked.
- Control Room restart clears in-memory sessions; operators must sign in again.
  Live browser inspection was unavailable because the browser connection timed
  out. Broader unchecked refactoring tasks below remain outstanding.

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
- [ ] Extract shared page polling/lifecycle helpers: start/stop on navigation, prevent overlapping requests, abort obsolete requests, reject late responses and retain last-good data with explicit errors.
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
- [ ] Consolidate HTTP/JSON error handling and cancellation; keep authentication, same-origin CSRF handling and mutation retry/uncertainty policies explicit.
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
- [x] Fix viewport jerking on unchanged refreshes: compare viewport scales only after Auto-fit establishes the final viewBox, not against temporary pre-fit geometry. Reproduced on desktop/tablet before the fix; 32 geographic and 8 shared Topology camera regressions pass after it. Source-only, not deployed.
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
- [ ] Logs aren't all showing the same style/layout.
- [ ] Logs sometimes have the horizontal scroll above the bottom (where it should be).

### Config
- [x] Adopt the same look-and-feel as the policy editor. If ZPR config is limited to one file, there is no need for Browse, or some of the File commands. Refresh Attributes is also not relevant.
- [x] Keep ZPR Config toolbar roles aligned with the Policy editor: File on the left, editor mode centered, Analyze/Format in the right-side action group, and History/utilities at the right. Match the blue pending-analysis state, enabled/disabled Format styling, success/error colors, and responsive placement.
- [x] Still things to do to get the look and feel better. "Validate syntax" should act like "Analyze". There should be a gutter for errors. A Format button, a file button.
- [x] Where are the colors? Where is the gutter? Why is the "Save Draft" not part of the file pulldown? Make "File..." and Analyze use the same color and sizes of the policy editor.
- [x] Remove "New draft" at the bottom of the editor, and use that space for the editor.
- [x] Remove line numbers from the gutter.
- [x] Gutter looks different from the other editors.
- [ ] Create a form editor.

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
browser interaction is claimed. Implementation remains uncommitted/unpushed.

### Gateways
- [x] Gateway Save and Ctrl/Command-S automatically Analyze modified drafts, show errors without saving invalid drafts, and save a revision only after successful exact-source validation.
- [x] Support GET, HEAD, POST, PUT, PATCH, DELETE and OPTIONS in Gateway draft Form/Raw, Analyze and Save; retain GET/HEAD defaults and reject TRACE/CONNECT. This does not expand runtime handler support.
- [x] Expand fixed-upstream gateway forwarding to the seven draft methods with bounded bodies and safe headers; retain GET/HEAD health checks. Verify all seven methods and bodies through the separate HTTP/HTTPS-tunnel web proxy. Committed and tested, not deployed; draft activation remains separate.
- [x] Consolidate shared Internet-egress forwarding primitives (hostname/method validation, redirect handling, header filtering and response forwarding); remove Simulator-specific naming from the reusable web proxy while preserving both CLI modes and protocol behavior. Committed and tested, not deployed.
- [ ] Deploy the committed forwarding/consolidation changes through a separately reviewed rollout; preserve production settings and verify the actual gateway binary and protocol behavior, not just UI assets.
- [ ] Unify deployed Gateway runtime configuration with the editor contract and migrate fixed-upstream callers before retiring that mode. Define enforcement capabilities for opaque HTTPS tunnels; do not imply draft methods/paths are enforced there.
- [x] Simplify Gateway form rows to Destinations Allowed, Add Destination, Base URL, Paths and Remove; remove numbered titles/borders, the prefix Add button and verbose help paragraphs. Remove deletes the whole destination; Paths remains one prefix per line.
- [ ] Improve individual path-prefix management and review wildcard requirements against draft validation and runtime matching before adding support. The live web proxy's host patterns do not imply wildcard support in saved gateway drafts.
- [x] Add a Gateway form editor with draft-only HTTPS origin/path-prefix controls synchronized with the raw JSON view; keep form controls out of the raw editor and Analyze/Save separate from runtime enforcement. Include GET/HEAD, timeout/response limits, read-only installed identity, lossless unknown-field preservation and form/raw validation coverage.
- [x] Report Gateway Analyze source-line errors only through standard ERR gutter markers; clicking opens error details and selects the source line. Never put source-line diagnostics at the top of the editor; preserve exact formatting and keep live inventory/contract errors separate.
- [x] Align Gateway ERR markers at the gutter's left inset and suppress the textarea's clipped native focus outline while retaining the editor's focus-within indicator.
- [x] Add a Control Room page listing installed ZPL Gateway services by organization and editing their versioned runtime drafts.
- [x] Validate destination/path/method/timeout/response-size settings through Control-Service before saving; keep drafts organization-scoped and do not activate them.
- [ ] Add verified external-network policy metadata, gateway runtime health, and explicit reviewed activation/rollback after the production provisioning contract is implemented.
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
  Proposed plan for approval: keep AI proposal-only; gather requirements and selected template/profile context; validate a schema-bound organization draft without writing files; show a diff for identity, directory, policy, services and runtime; require a separate `organization.create` authorization and explicit confirmation; create through an audited, idempotent backend API with revision/conflict handling; test invalid proposals, stale context, cancellation, duplicate IDs and lost responses. No organization creation has been wired.
 - [x] "Activate organization" and "Active" use very different styles. Make them look similar (other than the color).
 - [x] "Open reset log" should only show if there is a failure. Change the name to "See Logs".

#### Organization creation and editing
- [ ] Prioritize Duplicate organization -> edit draft -> validate -> review activation as the first end-to-end workflow.
- [ ] Add New, Duplicate and Import actions to the organization list.
- [ ] Add a short creation wizard with blank/template/existing-organization starting points.
- [ ] Collect display name, stable organization ID, description and directory base DN; suggest IDs and allow review before creation.
- [ ] Collect initial sites/departments and topology without requiring people, services or scenarios to be complete.
- [ ] Show a creation review with the proposed artifacts and missing configuration; finish with Create draft, never automatic activation.
- [ ] Exclude credentials, enrollment identities, sessions and runtime state from duplication and reject them from imports.
- [ ] Use an audited, idempotent creation API with explicit organization.create authorization, confirmation, revision/conflict handling and uncertain-response reconciliation.
- [ ] Add a full-page organization workspace using the shared editor styling, organization name and History.
- [ ] Add Overview fields for name/description and a readiness summary distinguishing incomplete drafts, validation failures and runtime not provisioned.
- [ ] Add structured Sites & topology tables and a topology preview.
- [ ] Add Directory tree/tables for people and groups alongside the existing LDIF source editor.
- [ ] Add Services & workloads tables with references to available machines and workloads.
- [ ] Open organization-scoped Policy, Assertions and ZPR Config through the existing editors.
- [ ] Add an organization-scoped Scenarios list opening the existing Scenario editor.
- [ ] Offer Advanced source for the organization profile as an alternative view of the same draft, not an independently editable copy; preserve unsupported fields through structured edits.
- [ ] Keep Save draft, Validate and Activate/Apply separate; saving must not change the runtime.
- [ ] Validate individual documents and cross-document references, including group members, service references, duplicate IDs and directory base-DN containment.
- [ ] Save coherent organization revisions across related documents; provide History and rollback without combining incompatible document versions.
- [ ] Require an explicit activation review showing runtime effects, prerequisites and changes before applying anything.
- [ ] Make AI assistance section-aware for overview, directory, policy/configuration and scenarios; disclose exactly which context will be sent.
- [ ] Show AI proposals as reviewable diffs across affected documents; never silently replace, save or activate an organization.
- [ ] Test invalid proposals/imports, stale context, structured/source round trips, cancellation, duplicate IDs, concurrent revisions, lost responses, coherent rollback and runtime isolation on draft saves.
- [ ] Keep production Control Room organization administration separate, using independently authorized production service contracts with no Simulator template, manifest or availability dependencies.

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
- [ ] Connect approved catalogs and invitation/review actions only after HTTPS
  named-user sign-in and independently verified Control-Service delegation.
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
- [ ] Certify native Windows install/browser launch, DPAPI/ACL and two-user
  isolation; sign and deliver releases. No Windows adapter/driver/service or
  credential issuance is included in the enrollment installer.
- [x] Build Apple Silicon per-user Mac enrollment app/DMG with explicit Keychain
  storage and no plaintext fallback; verify isolated native Keychain/restart
  tests and the packaged loopback wizard. Local build is ad-hoc signed only.
- [x] Build/test the native Mac adapter without privileged network changes;
  harden Mac tunnel/address validation with unprivileged regression tests.
- [x] Explicitly approved isolated Mac utun lifecycle smoke: fresh kernel-assigned
  interfaces, MTU 1400 then 1280, exact IPv6 /128 aliases, duplicate add/removal
  and repeated teardown. Existing interfaces/default routes/DNS match before/
  after; no running organization changed. Not packet-flow or credential certification.
- [ ] Certify Mac Finder/Terminal/browser permissions, Developer ID signing/
  notarization and supported OS versions; complete full privileged traffic/lifecycle
  tests, credential issuance and secure adapter-runtime handoff.
