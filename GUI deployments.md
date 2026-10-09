# GUI deployments

[Active backlog](GUI%20work.md) | [Completed work](GUI%20completed%20work.md)

This is a historical release ledger, not a live health report. Keep dated test,
asset, configuration, scope and rollback evidence here. Feature-local release
notes also remain in the completed-work archive to preserve their context.

Latest recorded Control Room release: `zpr-editor-ui:20261009-diagnostics-layout`
(2026-10-09 19:45 UTC). This does not imply Simulator or every source change was
deployed. Commit/push state is separate; retain each dated entry's scope.

## Deployment status

- Diagnostics body-only/raw/full-space layout and initial Loading deployed to
  Control Room on 2026-10-09, verified at 19:45 UTC
  (`zpr-editor-ui:20261009-diagnostics-layout`). Only three assets changed:
  Diagnostics JS v10, CSS v6 and the Diagnostics block/cache links in the
  captured live index. No duplicate date/time/level columns, Format JSON or
  recurring Querying/Updated line; full available width and natural-height
  content, with existing ANSI colors/errors/last-good data preserved.
  Reconstructed backend plus preceding live assets exactly reproduced the live
  executable before overlay:
  `fa26f7fa42ec33c516d4de43e6932560b95d422fce6fc0d3e96a3f0e0712f28a`.
  Isolated embedded release build and 16 desktop/tablet browser cases pass
  against the exact release assets. Current unrelated enrollment edits were
  excluded rather than repaired or deployed.
  All 66 served assets hash-verified; configuration, mounts, ports and host
  settings preserved. All 17 unrelated service starts unchanged; Simulator,
  Dex, Control-Service, Diagnostics proxy, Policy-Service and gateways untouched.
  Real Dex-authenticated desktop/tablet checks verify actual provider log bodies,
  single-child/full-width log rows, uncapped panels, no page-wide overflow,
  initial Loading and retained logs without Loading on manual refresh. Queries
  were delayed locally for Loading observation without replacing provider data.
  The first live verifier used a five-second UI timeout; it passed after waiting
  for actual provider-query completion. Shared ANSI/inert-HTML, window controls,
  healthy header/uptime, unsigned API denial and login recovery also verified.
  Retained stopped `zpr-control-room-diagnostics-layout-rollback`.
  Room sessions reset on restart; sign in again. No commit or push.

- Diagnostics ANSI colors deployed to Control Room on 2026-10-09, verified at
  19:29 UTC (`zpr-editor-ui:20261009-colored-logs`). Five frontend assets changed:
  new shared `colored-log.js` v1, Diagnostics v7, machine-logs v26 and the two
  corresponding HTML cache links. All 66 served assets hash-verified.
  Reconstructed backend with prior assets exactly reproduced the live binary
  (`ac0a65f1d6fdaacca06569661d44a734e739cac8a6eb2d12372e7fde7bbf2ec1`)
  before overlay; no backend source changes introduced. Configuration, mounts,
  ports and host settings preserved; all 17 unrelated service starts unchanged.
  Simulator, Dex, Control-Service, Diagnostics proxy and gateway remain untouched.
  Eighteen targeted desktop/tablet ANSI, inert HTML, JSON, terminal/window cases
  pass. Real authenticated desktop/tablet checks verify Diagnostics API access,
  loaded shared renderer colors/bold and inert HTML, unchanged matching window
  controls, healthy header/uptime and no JavaScript errors. ANSI live probe uses
  synthetic text locally in the page, not invented provider log events.
  Unsigned API denial/login recovery and all asset hashes verified. Retained
  stopped `zpr-control-room-colored-logs-rollback`; sessions reset on restart.
  Shared Worker renderer still awaits Simulator deployment. No commit or push.

- Window-control styling follow-up deployed to Control Room on 2026-10-09,
  verified at 19:24 UTC (`zpr-editor-ui:20261009-window-style`).
  Shared dark-on-white 32px buttons now match across editor/log Maximize and
  Restore controls. Only nine static assets changed: shared stylesheet/renderer
  and seven HTML cache links (`app.css` v93, `safe-display.js` v3). Reconstructed
  backend with preceding live assets reproduced the live executable exactly
  before overlay: `f4b190312fd8601621d6b206a8d689565a7a6ccce531e5dce68797029dab1f6e`.
  All 65 served assets hash-verified. Configuration/mounts/ports/host settings
  preserved; 17 unrelated running service starts unchanged. Simulator and all
  backend services remain untouched; Change Review/public enrollment UI excluded.
  Real Dex-authenticated desktop/tablet checks verify identical computed icon/
  button colors and 32px geometry, maximize/restore, healthy header/uptime,
  Diagnostics and no JavaScript errors. Unsigned API denial and login recovery
  verified. Stopped `zpr-control-room-window-style-rollback` retained.
  Room restart resets sessions; sign in again. No commit or push.

- Shared JavaScript runtime, concise header/secondary Visa uptime, and verified
  Maximize/Restore window icons deployed to Control Room on 2026-10-09 at
  19:13 UTC (`zpr-editor-ui:20261009-runtime-header-icons`). User approved the
  frontend-only scope and separately approved including the verified icon work.
  Reconstructed the preceding backend and proved its executable SHA-256 matches
  the live login-recovery binary byte-for-byte before overlaying static assets:
  `173d192326ed60d6ca4eb2499eae5bfed628c688223892181ef62957f1d3d496`.
  The release changes 24 static assets/additions; all 65 served assets were
  hash-verified. Runtime script ordering and current cache versions preserved.
  Public enrollment URL/email UI and Change Review source/API remain excluded.
  Forwarding/enrollment backend changes were not introduced.
  Container configuration, mounts, ports, host settings and network sets are
  unchanged except the image. All 17 unrelated running service starts unchanged:
  Simulator, Dex, Control-Service, Diagnostics proxy, Policy-Service and gateways
  were not restarted. Shared runtime/icons are still awaiting Simulator rollout.
  Pre-release combined runtime/header/icon regressions: 24 desktop/tablet cases
  pass. Real deployed Dex login, healthy header/uptime, Diagnostics, log icons,
  Policy maximize/restore and absence of JavaScript errors pass on desktop/tablet.
  Diagnostics retains 11 signaling sources and seven explicit no-signal sources,
  without provider/query credential errors. Unsigned API access is denied;
  callback recovery retains HTTP 403 and **Timed out. Try again.**
  Retained stopped `zpr-control-room-runtime-header-rollback`. One initial
  replacement automatically restored the prior image because the recovery probe
  used malformed state and expected 403 instead of its correct 400; corrected
  valid-state verification then passed. Room sessions were reset; sign in again.
  No commit or push.

- Operator login timeout recovery deployed 2026-10-09 at 18:09 UTC:
  Control Room uses `zpr-editor-ui:20261009-login-recovery`; local Dex uses the
  pinned custom `zpr-local-operator-idp:login-recovery-check` image. Callback
  rejection remains HTTP 403 with a single **Timed out. Try again.** POST action.
  Dex's expired-request/refresh error remains HTTP 400 but displays **Sign-in
  timed out** and the same retry wording; opening Control Room starts a fresh
  same-origin login. Deployed desktop/tablet refresh/click regressions pass.
  All 64 served Room assets match the preceding log-layout image byte-for-byte.
  Preserved the pre-forwarding backend plus trusted-feed/city-name fixes and
  changed only the shared callback recovery handler. Container configuration,
  host settings and network sets preserved except image replacements; Dex
  configuration adds `frontend.dir` only, retaining identities, certificates and
  persistent SQLite storage. All 16 unrelated running service starts unchanged;
  Control-Service/Diagnostics, collectors, nodes and Simulator were not restarted.
  Stopped `zpr-control-room-login-recovery-rollback` and
  `zpr-local-operator-idp-login-recovery-rollback` retained, together with private
  `dex.json.login-recovery-rollback`. An initial attempt safely restored Dex after
  a verifier-only DNS callback issue; corrected verification then succeeded.
  Room restart invalidates in-memory sessions; users may need to sign in again.
  Simulator callback backend was not deployed. No commit or push.

Current tracker reconciliation: 2026-10-09. The dated entries below are
historical verification snapshots, not fresh health checks.

- [x] Deploy the Control Room guideline update (15:37 UTC; 72 release browser cases).
- [x] Deploy Trusted Sources management buttons, source-name headings and heading-based feed selection (16:18 UTC; 26 release browser cases).
- [x] Deploy Visa Service uptime/status beside Control Room status (16:33 UTC; 28 release browser cases).
- [x] Deploy uniform dark log panels and bottom-aligned scrollports to Control Room (16:55 UTC; 38 release browser cases).
- [x] Restore live Diagnostics query access (2026-10-09): dedicated authenticated query-only gateway, isolated store/query networks and persisted operator credentials/network. The OSS provider cannot enforce Viewer roles, so its separate upstream Admin credential is private outside Control-Service mounts; no provider root credential is installed in Control-Service. Gateway translates the deployed client search path, millisecond timestamps and source-scoped queries to the native tenant search API, microsecond timestamps and 16 explicitly allowed metric streams. Unscoped smoke-test stream excluded. Live API verifies 11 available sources with real node logs/Visa Service logs and metrics; seven sources explicitly report no signals, with no credential or provider-query errors. Nineteen proxy request/translation cases, focused Diagnostics Go tests, build, shell syntax and whitespace checks pass. Live missing/wrong authentication, ingestion, account management, cross-tenant and unscoped searches are rejected; Control-Service is not on either original store or private upstream network. Only Control-Service and the new gateway changed; existing Control Room/backend images and provider/collector/node/Simulator starts were preserved. Its restart invalidated the browser session; authenticated browser re-verification awaits sign-in. Stopped `zpr-control-service-diagnostics-rollback` and initial gateway rollback retained. New source/runtime recovery is uncommitted/unpushed.
- [x] Commit the completed GUI, trusted-feed, city-label, enrollment and regression-test work (`59f0bda`). This includes source-only changes; it does not imply they were all deployed.

- Log panel styling/layout deployed to Control Room at 2026-10-09 16:55 UTC
  using `zpr-editor-ui:20261009-log-layout`. Exact release passed 38 desktop/tablet
  log and typography checks, focused trusted-feed/node-name Go tests and the
  embedded build. Ten served assets match byte-for-byte. Login remains enabled;
  signed-out session/snapshot return 401/403. Production snapshot is connected
  without errors; three city-labelled nodes each retain 146 counters.
  Container configuration and other service start times are unchanged;
  `zpr-control-room-log-layout-rollback` retains the World Map release.
  Preserved World Map, uptime, Trusted Sources and existing backend; no
  Simulator restart or unrelated enrollment/Diagnostics deployment.
  Shared Workers styling is included in the release assets, but the running
  Simulator has not been updated. Live browser log collection returned an
  authorization/CSRF error, so live populated-log rendering is not claimed;
  state/style/scrollbar behavior was verified using production-contract fixtures.

- Visa Service uptime/status deployed at 2026-10-09 16:33 UTC using
  `zpr-editor-ui:20261009-uptime-status`. Exact release passed 28 desktop/tablet
  uptime, guideline and Node Stats checks plus focused trusted-feed/node-name
  Go tests and the embedded build. Six served assets match byte-for-byte.
  Operator login remains enabled; signed-out session returns 401 and protected
  snapshot returns 403. Production snapshot is connected with no errors;
  Milwaukee, Shenzhen and Tijuana each retain 146 counters. Configuration,
  mounts, networks and every other running service start time are unchanged.
  Stopped `zpr-control-room-uptime-rollback` retains the preceding image.
  Preserved deployed backend, Trusted Sources and city labels; excluded
  unpublished World Map and enrollment changes. Broader initial selectors
  encountered the old two-method Gateway validator test and the World Map
  animation regression covered by the excluded fix; neither was modified.
  No authenticated live browser interaction is claimed.

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
