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
