# GUI deployments

## Source-qualified Velocity workload authorization (2026-10-30)

- Added an opt-in retained-runtime workload fragment without changing the
  legacy catalog or original scenario. Exact registry CNs and qualified BAS
  organization/asset/type claims gate finance-to-echo TCP8080 and the two
  controller-to-callback TCP8792 grants.
- Actual candidate compilation/evaluation exposed missing qualified aliases in
  BAS authentication claims. A new regression reproduced the failure. Reused
  the existing trusted-service qualification helper, preserving legacy raw
  keys and bounded expiry. Full VS suite: 848 passed, 24 ignored; the
  opt-in actual compiled-candidate evaluator was explicitly run and passed
  all 15 cases. No existing test was modified.
- Built offline and deployed a VS-only update to staged Velocity:
  `sha256:b45a254b729236d30af26b2c68f5e8e7bf438c5ef096cbfa4f1e2966c6d8c347`.
  Parent is the r15 routing image; other binaries retained. After restart all
  five approved identities passed admission with exact qualified claims.
- User explicitly approved exact complete source/configuration hashes and
  authorized named guarded application. Real CA-verified named review and
  separate apply inspected native staged bytes before approval. Independent
  readback verified `velocity-vs` version 4, digest
  `f45f1637b9a8de6024f1ae8ee67d114fcd8d5678c9526bd4d8910cf64fac0765`.
  Evidence: `policy/velocity/workload-access-r16-evidence/`.
- All five retained identities passed admission again under the new policy;
  probes cleaned up. This does not prove original machine/workload traffic,
  launcher integration or scenario acceptance. Main dev/stable unchanged.

## Velocity delivery gate and persistent admission routing (2026-10-30)

- Scope reset: original Velocity is the sole next release gate. Main dev r7/r8,
  stable r6 and all fixed ports are unchanged; no additional organization rollout.
  The coordinator owns remote mutations. A separate read-only thread mapped
  launcher handoff gaps without editing source or accessing runtime.
- The separately approved r13 root configuration was named-reviewed and
  guarded-applied, superseding r12 below. Independent readback verified
  `velocity-vs` version 3, digest
  `651da92b2ca098c3a9cb4cdf96d6fd3bf37fa62052c1eb07057ec7884260925d`.
  The fixed compiler is deployed Velocity-only. Both required services are live.
- PH issuer serialization was corrected with an actual TLS regression; full
  PH validation passed 326 tests with two ignored. The effective enrollment
  operator binding now uses the reviewed VS HTTPS configuration and mounts.
- Reconciled the interrupted admission probe: no success evidence existed.
  Stopped its exact owned container and preserved private logs. BAS
  authentication succeeded, but VS could not route to the token listener.
  A live lookup reproduced `Network is unreachable`.
- Added the VS ZPR-space route in both launcher topologies. A new regression
  failed before the fix and passed afterward; full dashboard `make test`
  passed. Packaged a runtime-script-only image:
  `sha256:322efc741b7ac10d02d2c63de8260b7aa0bacb282a54074a43d5b9576db99d3e`.
  Deployed Velocity-only with launcher/checksum rollback copies. Initial startup
  caught an installer executable-mode error; corrected before readiness checks.
  No frozen identities, credentials, trust grants or workload policy changed.
- After clean restart, verified the exact image, persistent route and both
  services. All five approved retained runtime keys passed authoritative
  through-ZPR admission again. Temporary probes were removed; a 90-second
  in-container lifetime now bounds interrupted probes.
- This is **not original scenario acceptance**. Launcher/controller/workload/
  callback handoff, original workload policy review, UI execution, traffic
  thresholds and scenario recovery remain pending. Current checks and evidence:
  [release gate](environments/README.md#velocity-release-gate-and-delivery-discipline)
  and [inventory](environments/dev.json). Source changes are uncommitted.

## Retained Velocity endpoint approval and native BAS qualification (2026-10-30)

- The user explicitly approved the exact five-item original machine-runtime
  inventory as `admin@evaluation.invalid`, including both controllers,
  finance-client, echo-service and the Simulator callback adapter. Named API
  approval and durable receipt bind digest
  `34166a9fefc9e920df371c135eae39532a64f8b8021dfa2c63004de6fbbe0bdd`.
  This is not a complete organization infrastructure inventory.
- A clean restart of Velocity's three units preserved the decision and every
  enrollment/runtime fingerprint. All five key-authenticated statuses are
  approved revision 3. Immutable PH runtime files retain the approved keys and
  authoritative registry client IDs; no identities were recreated.
- Staged enrollment-service/device binaries were upgraded with backups.
  Velocity's Enrollment Service owns BAS adapter/token listeners at
  `24996`/`24997` with its existing Store. A separate constrained BAS CA preserves
  installation CA constraints. Genuine five-key exchanges passed TLS 1.3,
  signed exact registry claims, pinned VS redemption, replay denial, cross-org
  denial and administration-route isolation.
- The PH-upgraded candidate image
  `sha256:97e4e22925b1351384ded47cb13e23629669472ac519780bfe2907e64818e7c4`
  is deployed only for the separately reviewed BAS-root adapter. Root proposal
  `bas-root-r12` was explicitly approved and applied through the named guarded
  workflow. Independent `velocity-vs` readback verified digest
  `8e602f66c228f02feccaa42eb5b1a49e2c755efa96762aeb7c23b061c7e74331`,
  version 2. Existing baseline compiler warnings remain visible.
- The root is admitted and the VS confirms its `Authentication` service on
  TCP 24996, but the required `validation/2` companion is missing. Qualification
  failed that exact check. A compiler regression reproduced pruning of the
  companion when no returned attribute is referenced; the dependency fix passes
  all 311 compiler tests and formatting, with no pre-existing test changes.
  The fix is built but not deployed.
- Unapproved root proposal `bas-root-r13` keeps the source and all trust material,
  adding only explicit 300-second trusted-service lifetime. The fixed compiler's
  actual binary dump verifies the validation metadata, both ports and lifetime.
  This changed configuration requires fresh explicit review. No original
  endpoint admission or scenario acceptance is claimed.
- Current staged state: Velocity's three original units and BAS-root adapter
  active, the other twelve organization units stopped;
  none boot-enabled. Main dev r7/r8 and stable r6 are unchanged and returned
  CA-verified HTTP 200/302 on the same ports. Updated current state and evidence
  are in [the environment inventory](environments/README.md#approved-velocity-machine-runtime-identities-and-bas-handoff).

## Identity-driven approval gate; staged Velocity only (2026-10-30)

- Added explicit organization-scoped `approval.self` permission, supplementary
  to domain approval permissions. Policy/gateway, subject-link and enrollment
  decisions enforce it server-side. Signed session capability and independent
  backend trust must both permit delegated self-approval. Submission never approves.
- Updated review controls, confirmations and audit wording. The user authorized
  updating affected fixtures to grant self-approval explicitly while preserving
  audit and revision/fingerprint assertions. Full `make test`, affected vet,
  focused race tests, 24 desktop/tablet policy/subject-link browser checks and
  six real-TLS enrollment-review browser checks passed.
- User selected `admin@evaluation.invalid` as the development self-approver.
  Enabled it in staged Velocity only, retaining the independent reviewer,
  immutable base identity, trust keys and other organization grants. CA-verified
  real login confirmed session and independent enrollment capabilities.
- Four genuine step-referenced Velocity endpoint claims remain pending. A
  clean paired-sidecar stop/start preserved their keys and exact inventory digest.
  No approval, runtime admission or original scenario acceptance is claimed.
  The original control-service adapter dependency is not covered by those four
  claims; do not request approval as though this were a complete runtime inventory.
- Main dev r7/r8 and stable r6 remained unchanged. At this earlier milestone the
  staged units were stopped; the current state is recorded above.
  Velocity's original scenario is the first end-to-end gate, not another broad
  base-service rollout. See [the environment inventory](environments/README.md#identity-driven-approval-gate-and-velocity-first-qualification).

## Staged organization operator services (2026-10-30)

- Qualified independent Northstar, Great Lakes, Load Lab, Redwood and Velocity
  production bases plus separately supervised Policy-Service processes under
  `/home/dubnom/zpr-dev-organizations-r9`. With the independent enrollment
  sidecars, all fifteen exact units are stopped.
- Current-invocation live checks passed original LDAP scope/counts, all
  1/3/1/2/1 nodes, protected DNS, logging, retained identity hashes and clean stop.
  Real CA-verified named OIDC login passed independent policy context and
  guarded review capabilities. Policy-Service readback exactly matched VS
  organization/target/digest/version; cross-organization access was denied.
  No organization Simulator was available or needed.
- Fixed multi-node DNS docking to use the selected substrate rather than the
  single-node-only address. The runtime-layer image retains PH/VS/compiler
  binaries. Explicit protected operator bindings attach guarded policy trust
  without editing frozen installation identity or core VS settings.
- The main dev r7/r8 pair and stable r6 were not restarted. This is not a full
  r9 rollout: original policies were not approved/applied, workload admission
  is unfinished, Simulator selection is not wired, and the 17 original scenarios
  have not passed. Exact ports and evidence are in
  [the environment inventory](environments/README.md#staged-original-organization-operator-installations).

## Dev Simulator GUI cleanup (2026-10-30 15:23 UTC)

- Deployed `dev-20261030-r8` Simulator only at the fixed
  <https://10.0.1.160:18788/scenarios.html#simulate>. Production remains r7;
  stable remains r6. Their supervisor PIDs were unchanged.
- Removed obsolete Demo World/manual provisioning pages and navigation.
  Operational detail and read-only policy status live on Info; debug
  details are hidden. Old page URLs redirect to Simulate.
- Both matching archives and their internal inventories were checksum verified.
  Production companion SHA-256:
  `9c43d19eefa3dac6472da56ccf03c5ccd9bc95f608bfb5dabf8a51802f512010`.
  Simulator archive SHA-256:
  `8a3a6b8784bb60dddb4278ffdf40373d2df8c744386501bdb591ccce0d8f2d0f`.
  Source `d23511ff5c5db874d50262938179d6b403b0195b`, dirty engineering build.
- Retained r4 installation/workspace, ports, identity, CA and administrator
  credential. Frozen inventory, CA, administrator secret and catalog manifest
  hashes matched before/after. No state was reset or deleted.
- Real named OIDC login/consent with the installation CA verified the session,
  all five original catalogs (Northstar 5, Great Lakes 5, Load Lab 4, Redwood 2,
  Velocity 1), and served cleanup assets. Live credential-free Chromium checks
  at 1440 and 834 pixels verified navigation/Info separation, redirects, no
  horizontal overflow and no uncaught errors. Chromium bypassed certificate
  errors only for those credential-free layout checks; authenticated API checks
  verified the real CA. The integrated browser cannot trust this private CA.
- Source browser selection now passes all 78 desktop/tablet checks. The initial
  run passed 62 and failed 16 stale removed-asset, old wording and hidden-debug
  expectations. The user explicitly authorized those expectation updates.
  Coverage still checks exact approval/run payloads, stale digest rejection,
  cross-run isolation, keyboard navigation and production asset exclusion.
  It additionally requires removed assets to return 404, compatibility pages
  to redirect and retained run identifiers to remain hidden. No deployed asset
  needed changing for this test-only correction.
  This rollout does not claim restored original scenario runtimes or acceptance.
- Rollback: stop only `zpr-dev-simulator-r8`, set dev's
  `simulator_release=dev-20261029-r7` in
  [workbench.sh](environments/workbench.sh), then relaunch dev Simulator with
  the retained r4 state. No commit/push performed.

## Sign-out also opens provider sign-in (2026-10-16 15:10 UTC)

- User clarified that sign-out must not leave the interim operator-access page
  visible either. Supersedes the previous release's explicit-logout/manual
  reentry behavior: after confirmed local session invalidation, Control Room
  opens provider sign-in once through the existing native POST. Failed attempts
  still allow manual retry to prevent loops. Simulator behavior is unchanged.
  Local logout does not terminate provider SSO; its existing policy still applies.
- All 22 focused desktop/tablet authentication browser checks passed, including
  sign-out using real Control Room markup, session invalidation, one provider
  navigation, permission rejection, expiry and recovery. Editor diagnostics,
  whitespace check, and Linux/ARM64 embedded build passed. No Go source changes.
- Control Room only restarted at `2026-10-16T15:10:33.222923503Z`.
  Executable SHA-256:
  `241384529a765428dd00a1acef6ff5e425bdd25f307b630454f567e5ef106532`.
  Live HTTPS returns 200 and served login JS byte-matches source; script cache
  version incremented to 13. No live credentials entered.
- Control-Service, identity provider and Simulator start times unchanged.
  Rollback: `/usr/local/bin/zpr-web-dashboard.before-direct-signout-20261016`
  (previous SHA-256
  `bd7fedd85fcd8a62e6a957c207840c3b30fad4e3ff681d5600c172cc08aafaaf`).
  Stop only Control Room, restore the executable and start it. No commit or push.

## Direct Control Room sign-in and session expiry (2026-10-16 15:07 UTC)

- User approved Control Room-only deployment after confirming unsaved work was
  secured. Removed the initial Operator Access/checking screen: a signed-out
  visit now submits the existing native same-origin login POST directly to the
  identity provider. Signed-in focus checks retain the current interface.
- Confirmed session loss displays "Session expired. Redirecting to sign in..."
  for 1.5 seconds before one automatic sign-in attempt. Reload retains knowledge
  of a previous session and the safe application destination. Failed/expired
  login attempts offer manual retry instead of automatic redirect loops;
  explicit logout does not immediately reenter sign-in. Simulator retains its
  explicit sign-in behavior.
- Expired/unavailable callback attempts now return a fixed HTTP 303 to
  `/?operator_login=expired`, without an interim HTML/meta-refresh page.
  Rejected callbacks still cannot exchange codes or create sessions; malformed
  requests retain their existing denial responses.
- Validation: 20 focused desktop/tablet browser tests passed, including actual
  Control Room HTML/CSS expiry visibility with other origins unavailable,
  focus/reload/API-triggered expiry, permission rejection, logout, configuration
  failures, and callback retry. Auth package race tests, related dashboard
  Operator/Login/ControlRoomHTTP tests, Linux/ARM64 build, formatting, editor
  diagnostics, and whitespace checks passed.
- `make test` was attempted but is not green: unrelated failures remain in
  assistant scenario instructions, enrollment administration, organization
  policy installation, scenario placement, and trusted-service helper builds.
  Those tests and their implementations were not changed for this request.
- Installed the embedded executable into the existing Control Room container;
  started `2026-10-16T15:07:26.211989333Z`. SHA-256:
  `bd7fedd85fcd8a62e6a957c207840c3b30fad4e3ff681d5600c172cc08aafaaf`.
  Live HTTPS returns 200; served login JS/CSS byte-match source; unauthenticated
  session returns 401; an unavailable, correctly shaped callback attempt returns
  the expected fixed 303. Shared browser reload opens the real identity
  provider's "Log in to Your Account" page directly and remains there without
  refreshing. No credentials entered or live authenticated expiry exercised.
- Only Control Room restarted; Control-Service, identity provider, Simulator,
  and production backends were not restarted. Existing operator sessions were
  invalidated by the restart. No policy/configuration changes, commit, or push.
- Rollback executable:
  `/usr/local/bin/zpr-web-dashboard.before-direct-signin-20261016`
  (SHA-256 `f873f11e9fa3e44593e60ef3acc044c7a4a8a34c814c4ea18c8636dc253f23fc`).
  Stop only Control Room, restore this executable to
  `/usr/local/bin/zpr-web-dashboard`, start it, and verify HTTPS. Recreating the
  container from its original image does not retain this writable-layer update.

## Monitoring first with separate Simulate execution (2026-10-16 03:10 UTC)

- Reordered Simulator sidebar groups: MONITORING (Simulate, Activity, Workers),
  divider, then CONFIGURATION (Organizations).
- Per user clarification, organization Scenarios retains Browse/File, selected
  scenario details, and editing. Its Simulate shortcut opens the separate
  execution view at `/scenarios.html#simulate`. Simulate contains scenario
  selection, Run/Cancel/Clear, live tracks, logs, and results, without the
  organization configuration header or catalog. No scenario starts merely by
  selecting it or navigating. Published/active-organization/busy guards remain.
- Reused the existing scenario workspace and polling lifecycle rather than
  introducing a second runner or API. Hash-aware active navigation, browser
  history, direct reload, and Configure scenarios preserve the correct view.
  Switching between configuration and Simulate retains scenario selection.
  Updated in-page help and README.
- Validation: 30 desktop/tablet browser tests passed, covering separate panels,
  run/cancel/clear fixture actions, results, draft and busy guards, foreign
  organization restrictions, configuration editing, cached navigation, direct
  reload/history, responsive widths, track scrolling, and output disclosure.
  Linux/ARM64 build, editor diagnostics, and whitespace checks passed.
- Final Simulator-only deployment: `2026-10-16T03:10:09.041146927Z`.
  Executable SHA-256:
  `1f5c092ce3a2bbeb55d2a47fcb25ba1c0c0dc1d669b86b9c647878cde9f113cf`.
  Live navigation confirmed reordered groups, hidden configuration on Simulate,
  hidden runner under configuration, working return shortcuts, and retained
  selection. No live scenario was started; runtime stayed idle.
- Rollback: `/usr/local/bin/zpr-web-dashboard.before-simulate-navigation-20261016`
  (previous SHA-256 `400af31ebbcf5b2afc43a1bb47047964a28fa1a37a1e75eea96e720686d773ed`).
  Restore the executable and restart only Simulator. Control Room unchanged.
  No commit or push.

## Simulator panel heading cleanup (2026-10-16 02:58 UTC)

- Removed repeated Policies and Services headings inside their respective
  organization tabs; accessible region labels and records remain intact.
- Simulator Trusted Sources now hides its "Organization LDAP" source title
  and aggregate people/groups/attributes summary, including before first load
  and after tab changes. Record tabs, individual tab counts, metadata, and
  records remain available. Control Room's source headings are unchanged.
- Validation: 32 focused desktop/tablet tests passed, including new panel
  cleanup coverage, records, reload, organization navigation, current Trusted
  Sources GUI tests, and stale-response/error handling. Build, diagnostics,
  and whitespace checks passed. An initial broader selection had 38 passes
  and 14 failures in older editor selectors, source-heading/feed expectations,
  and the obsolete Simulator sidebar active-link expectation; those tests
  were not rewritten. The broader selection is not reported as passing.
- Live verification confirmed no repeated Policy/Services headings, preserved
  records in both panels, hidden Trusted Sources title/summary, and a visible
  People table. Simulator assistant readiness is healthy.
- Simulator-only deployment: `2026-10-16T02:58:12.750044553Z`.
  SHA-256: `400af31ebbcf5b2afc43a1bb47047964a28fa1a37a1e75eea96e720686d773ed`.
  Rollback: `/usr/local/bin/zpr-web-dashboard.before-panel-cleanup-20261016`
  (previous SHA-256 `cd70361e8d51cd7215bd1ff4ced7a8623f1dbd032143598915fc3b43c1cf6d5f`).
  Restore the executable and restart only Simulator. No commit or push.

## Simulator sidebar divider and directory summary cleanup (2026-10-16 02:51 UTC)

- Added the shared Control Room-style `nav-separator` between CONFIGURATION
  and MONITORING. It persists without duplication through cached navigation
  and reload, and becomes a vertical divider in the narrow horizontal nav.
- Removed the redundant departments/people/groups count line above the Identity
  directory graph, retaining the directory title, graph, and editing controls.
- Validation: 14 focused desktop/tablet browser tests passed, including
  navigation, reload, responsive divider dimensions, count removal, and
  Browse/File style parity. Editor diagnostics, Linux/ARM64 build, and
  whitespace checks passed. Live verification confirmed one 1px divider
  between the correct groups and no redundant count summary.
- Simulator-only deployment: `2026-10-16T02:51:26.670833421Z`.
  Assistant ready; scenario runtime remains idle. Control Room unchanged.
  Executable SHA-256:
  `cd70361e8d51cd7215bd1ff4ced7a8623f1dbd032143598915fc3b43c1cf6d5f`.
- Rollback: `/usr/local/bin/zpr-web-dashboard.before-nav-separators-20261016`
  (previous SHA-256 `3f6ff10ab41104271b3ecaf4f0282fde74ff12619cfb294388f326c755f2d16a`).
  Restore the executable and restart only Simulator. No commit or push.

## Match live Control Room Browse/File appearance (2026-10-16 02:42 UTC)

- Corrected the preceding rollout, which made Simulator controls internally
  consistent but did not match the deployed Control Room.
- Measured Control Room policy Browse/File visible labels and chevrons:
  ink `#17211e` background, lime `#b5e34f` text, regular 16px Avenir Next /
  Segoe UI sans-serif labels, 16px chevrons, 31px minimum height, 3px radius.
  Simulator controls now use these reference values, not the earlier
  forest-green/white 12px semibold design. Decorative chevrons do not change
  accessible button names.
- Added explicit reference-value assertions in addition to cross-control
  parity assertions. All 36 related desktop/tablet browser tests passed;
  Linux/ARM64 build and whitespace validation passed. Compared live Simulator
  organization/scenario controls against live Control Room labels: colors,
  visible font, and chevron font match. Control Room was not redeployed.
- Final Simulator deployment: `2026-10-16T02:42:54.838490295Z`.
  Assistant readiness is healthy; no organization activation or scenario run.
  Executable SHA-256:
  `3f6ff10ab41104271b3ecaf4f0282fde74ff12619cfb294388f326c755f2d16a`.
- Rollback: `/usr/local/bin/zpr-web-dashboard.before-control-room-buttons-20261016`
  (SHA-256 `0b3ad8065dda2942a633b0687f36db1900bd1522a6cfebc57f690b7931534895`).
  Restore the executable and restart only Simulator. No commit or push.

## Uniform Browse and File controls (2026-10-16 02:39 UTC)

- Unified catalog and editor Browse/File button styling in shared `app.css`:
  forest-green background, white labels, 12px semibold sans-serif typography,
  36px minimum height, identical spacing, and a CSS-drawn chevron. Existing
  text chevrons are hidden so each button has exactly one consistent indicator.
  Expanded menus point upward; hover, keyboard focus, and disabled states use
  the same styling across controls. Menu actions and behavior are unchanged.
- Added computed-style parity checks covering organization and scenario
  catalogs, organization and scenario editors, and operator policy, gateway,
  and configuration controls. Checks also compare nested label typography and
  run at desktop, tablet, and 390px widths.
- Validation: 36 related desktop/tablet browser tests passed; Linux/ARM64
  build, editor diagnostics, and whitespace checks passed. Live organization
  and scenario controls report matching colors, font, height, and chevrons.
- Deployed only Simulator at `2026-10-16T02:39:05.536324092Z`; Control Room
  was not restarted. Assistant ready; organization `great-lakes` remains idle.
  No organization activation or scenario execution was performed.
- Executable SHA-256:
  `0b3ad8065dda2942a633b0687f36db1900bd1522a6cfebc57f690b7931534895`.
  Rollback: `/usr/local/bin/zpr-web-dashboard.before-uniform-file-buttons-20261016`
  (previous executable `ac1b1f003e35e99b74b214d4b6a825777a8b7f22569092a6c394fbb28644a694`).
  Restore that executable and restart only Simulator to roll back.
  No commit or push.

## Shared organization header across every tab (2026-10-16 02:31 UTC)

- Corrected the earlier Identity-only header placement. All five organization
  tabs now reuse one functional organization workspace, including direct entry
  to Scenarios or Trusted Sources. Browse, File, organization name, and
  activation controls remain above the organization description; the horizontal
  navigation follows the description.
- Removed the legacy Scenarios disclosure/summary from Identity and its
  separate catalog request. Scenarios remains the first horizontal tab.
  Organization selection, menus, dialogs, and drafts persist across tab changes.
  Organization polling follows all organization tabs and stops on Monitoring.
  Editor Find/Replace initialization is idempotent when the workspace returns.
- Validation: 46 focused desktop/tablet tests passed, including all-tab
  control/description placement, direct Scenarios and Trusted Sources visits,
  Browse/File/edit/activation-cancel behavior, cached navigation and reload,
  responsive widths, draft protections, scenario execution guards, profile
  saving/AI review, duplicate creation, and window controls. Linux/ARM64 build,
  editor diagnostics, and whitespace checks passed.
- A broader 80-test editor/browser run had 58 passes and 22 failures. The
  failures target hidden source textareas or ambiguous labels matching both
  textarea and CodeMirror editors; they are not reported as passing. Those
  unrelated editor tests were not rewritten as part of the layout change.
- Deployed only to `zpr-simulator` at `2026-10-16T02:31:37.302407801Z`.
  Executable SHA-256:
  `ac1b1f003e35e99b74b214d4b6a825777a8b7f22569092a6c394fbb28644a694`.
  Live measurements confirmed controls above description above tabs, no legacy
  scenario summary, and exactly one workspace on all five tabs. Direct
  Scenarios and Trusted Sources reloads also passed; Browse opened and closed
  on Trusted Sources. Assistant ready; organization page HTTP 200. No
  organization activation or scenario run was performed.
- Rollback executable:
  `/usr/local/bin/zpr-web-dashboard.before-shared-org-header-20261016`
  (SHA-256 `e9b733d2465be11a57f3b2ac6bed9db1b31190856074b3213492983a7f133a58`).
  Restore it over `/usr/local/bin/zpr-web-dashboard` and restart only Simulator
  to roll back. No commit or push.

## Simulator organization header order and sidebar groups (2026-10-16 02:17 UTC)

- Moved organization Browse, File, name, and activation controls above the
  horizontal navigation. Scenarios is now the first tab; Identity remains the
  default view. Grouped the Simulator sidebar using the shared navigation-group
  styles: CONFIGURATION contains Organizations; MONITORING contains Activity
  and Workers. Control Room is unchanged.
- All 36 focused desktop/tablet browser tests passed. New coverage checks DOM
  and visual control order at 1440px, 820px, and 390px, active and inactive
  organization controls, tab order, sidebar grouping through cached navigation
  and reload, and absence of unintended activation. Linux/ARM64 build and
  whitespace checks passed.
- Deployed only to `zpr-simulator`, restarted at
  `2026-10-16T02:17:55.20811792Z`. Executable SHA-256:
  `e9b733d2465be11a57f3b2ac6bed9db1b31190856074b3213492983a7f133a58`.
  Live checks confirmed all four organization controls above the tabs,
  Scenarios first, both sidebar group titles, assistant readiness, and
  organization page HTTP 200. Static assets revalidate with `Cache-Control:
  no-cache`; reload existing tabs to load the update.
- Rollback retained at
  `/usr/local/bin/zpr-web-dashboard.before-header-order-20261016`
  (SHA-256 `f39d7331efb20deb9771666ebfe814846f6a56d9559aa382efe46454fd02bd4e`).
  Restore it over `/usr/local/bin/zpr-web-dashboard` and restart only Simulator
  to roll back. No commit or push.

## Simulator organization navigation and readable icons (2026-10-16 01:48 UTC)

- Simulator only. Added horizontal organization navigation for Identity,
  Trusted Sources, Policy, Services, and Scenarios; removed the Scenarios and
  Trusted Sources sidebar entries. Existing URLs and cached in-app navigation
  remain supported. Policy and Services now select individual detail panels.
- Removed the standalone organization "Design with Claude" action and its
  dialog. Scenario design and editor-local assistants are unchanged.
- Enlarged zoom/dismiss glyphs to bold 26px symbols in 36px controls, and
  directory branch symbols to bold 20px. Preserved labels, keyboard controls,
  focus indicators, and directory graph screen space.
- Validation: 36 focused desktop/tablet browser checks passed, including
  navigation/history/deep links, narrow layouts, organization browsing and
  draft protections, scenario browsing, editor/window controls, icon sizing,
  keyboard dismissal, and organization Help. Linux/ARM64 build passed.
  An additional existing header test still fails on its stale
  `#organization-active-name` count assertion; that element was already absent
  before this change. The full browser suite was not run.
- Installed executable SHA-256:
  `f39d7331efb20deb9771666ebfe814846f6a56d9559aa382efe46454fd02bd4e`.
  Restarted only `zpr-simulator` at `2026-10-16T01:48:49.391528293Z`;
  container ID, image, configuration, and mounts were preserved.
- Live checks: assistant ready, organization page HTTP 200, all five
  organization navigation destinations render with the correct current link.
  Measured zoom and organization-dismiss controls at 26px/700 glyphs and
  36px square buttons. No scenario was applied, saved, published, or run.
- Rollback executable:
  `/usr/local/bin/zpr-web-dashboard.before-organization-tabs-20261015`
  (SHA-256 `4b8f43504d49e432273db76a96fa56df00e9d73c603beeb43de9824633ba4ecc`).
  Restore it over `/usr/local/bin/zpr-web-dashboard` and restart only
  `zpr-simulator` to roll back. No commit or push.

## Scenario assistant compact probe patch (2026-10-15 22:05 UTC)

- Simulator only. Installed a Linux/ARM64 dashboard executable in the existing
  `zpr-simulator` container and restarted only that container. Executable
  SHA-256: `4b8f43504d49e432273db76a96fa56df00e9d73c603beeb43de9824633ba4ecc`.
  Container ID `d27412fbd2adf5a4314f549c1bd7e3df2024da829cd7ecbb25493e17ec859e5e`,
  image ID `sha256:765e0dd6b5cd8056d51b7926d39574508f351e08f18692c3acdc74c7cdc0f400`,
  configuration and mounts were preserved. Started
  `2026-10-15T22:05:39.266018137Z`.
- Diagnosis: Anthropic first rejected the earlier schema for 27 union-typed
  parameters (limit 16). Once corrected, reproducing the 78-step scenario hit
  the output limit. Combining full and patch response schemas exceeded the
  grammar limit; an unbounded patch array also exhausted output. The final
  implementation uses a patch-only schema for populated scenarios and keeps
  the full-scenario schema for new ones. It preserves the base server-side and
  adds one step at a time. Existing denied business-service pairs are provided
  as grounded probe candidates.
- Focused scenario/assistant race tests and `go vet` passed. A live request with
  the exact reported prompt returned HTTP 200 at 2,400 maximum output tokens
  (15,115 input, 324 output). The validated proposal keeps the existing 78
  steps and adds one `request_business_service` denial probe after an employee
  login, branching alongside the remaining normal work. This is an
  application-level simulated service probe, not a raw network scan. The
  expected denial is not claimed as a measured result. A before/after catalog
  comparison confirmed the stored scenario was unchanged; no Apply, save,
  publish or run occurred.
- Assistant readiness is `true`; `/scenarios.html` returned HTTP 200. No other
  service was restarted. Rollback copy retained in Simulator:
  `/usr/local/bin/zpr-web-dashboard.before-compact-patch-20261015`
  (SHA-256 `5db4f1a0af1523d083eb1043af245eda979fed7d4831a56816d02f6746c949a6`).
  To roll back, restore that file over `/usr/local/bin/zpr-web-dashboard` and
  restart only `zpr-simulator`.
- No commit or push. Reload the Scenarios page before retrying.

## Scenario structured-output schema correction (2026-10-15 21:31 UTC)

- Simulator only. Built a Linux/ARM64 dashboard executable from the current
  dashboard worktree using the Simulator Dockerfile build stage. Binary
  SHA-256: `d8f50c098fabd71a1228db592be7cab8d6c0f8509bf810ae4031f6f9a0b03322`.
- Replaced the executable in the existing `zpr-simulator` container and
  restarted only that container; no image recreation or stack restart.
  Container ID `d27412fbd2adf5a4314f549c1bd7e3df2024da829cd7ecbb25493e17ec859e5e`,
  image `zpr-simulator:20261015-org-layout-fixed` and image ID
  `sha256:765e0dd6b5cd8056d51b7926d39574508f351e08f18692c3acdc74c7cdc0f400`
  stayed unchanged. Existing mounts and container configuration stayed
  unchanged. Started `2026-10-15T21:31:36.184534719Z`.
- Anthropic's returned 400 identified 27 union-typed schema parameters versus
  its limit of 16. The schema now uses required empty/default values for
  optional scenario fields and retains only the nullable proposal union.
  Focused scenario/assistant race tests pass.
- Post-deploy assistant readiness returned `ready: true`; scenario API reports
  `great-lakes`, five scenarios and an idle run. The served Scenarios page
  reconnects and displays Great Lakes Instruments. No live provider request,
  scenario run, save, publish or other service restart was performed.
- Control Room, Control-Service and Policy-Service container IDs, images and
  start times were unchanged. Rollback executable retained in the Simulator
  as `/usr/local/bin/zpr-web-dashboard.before-structured-schema-20261015`
  (SHA-256 `2a67b9e61fc98e2bd23c79db443d3c218e946866999e680bbb394a22266ba892`).
  To roll back, restore that file over `/usr/local/bin/zpr-web-dashboard` and
  restart only `zpr-simulator`.
- A temporary diagnostic build captured the provider's union-limit error in
  local Simulator logs, then was replaced by the corrected build. Its rollback
  copy is `/usr/local/bin/zpr-web-dashboard.before-provider-diagnostics-20261015`
  (SHA-256 `d8f50c098fabd71a1228db592be7cab8d6c0f8509bf810ae4031f6f9a0b03322`).
- No commit or push. Reload the Scenarios page before trying the design again.

## Scenario AI structured design (2026-10-15 21:13 UTC)

- Simulator only, existing container executable replaced without recreation.
  Image `zpr-editor-ui:20261015-scenario-ai`, based on `59b4672` plus this
  Scenario AI follow-up. Started `2026-10-15T21:13:11.26289543Z`.
  Executable SHA-256:
  `2a67b9e61fc98e2bd23c79db443d3c218e946866999e680bbb394a22266ba892`.
- Native Scenario response schema and explicit incomplete/output-limit errors;
  preserve failed-request usage and draft. No automatic retry or limit increase.
  Existing proposal validation and explicit Apply/Save remain unchanged.
- Focused Go race tests/vet and two desktop/tablet failure UI checks passed
  before rollout. Full suite retains four recorded pre-existing failures.
  Deployment verifies exact Simulator Config/HostConfig/sorted mounts/ID,
  installed executable hash and real assistant readiness. Control Room,
  Control-Service and Policy-Service identity/image/start baselines unchanged.
- Served Scenario HTML and 18 assets match sources. Actual served browser
  error/usage check uses mocked APIs: draft unchanged, no Apply, usage visible,
  no JS/CSP errors. No live provider request, save, publish or scenario run.
  Original operator provider response was not retained; its precise failure
  cause remains unconfirmed and no live provider success is claimed.
- Private artifacts: session `files/scenario-ai-release-20261015/` contains
  executable, rollback, private inspect snapshots, unchanged-service baselines
  and verification script. Do not publish private inspect files.
  Rollback: stop Simulator only, restore `simulator-before` to
  `/usr/local/bin/zpr-web-dashboard`, start and verify. In-container copy:
  `/usr/local/bin/zpr-web-dashboard.before-scenario-ai-20261015`.
- No commit/push. Reload the Scenarios page before retrying.

[Active backlog](GUI%20work.md) | [Completed work](GUI%20completed%20work.md)

This is a historical release ledger, not a live health report. Keep dated test,
asset, configuration, scope and rollback evidence here. Feature-local release
notes also remain in the completed-work archive to preserve their context.

Latest recorded dashboard executable deployments: Simulator
`zpr-simulator:20261015-scenario-browser` (2026-10-15 16:24 UTC), and Control Room
`zpr-editor-ui:20261015-color-analyze` (2026-10-15 17:59 UTC). Simulator was
recreated from its tagged image; Control Room was updated in its existing
container's writable layer. The 17:59 release recreated Policy-Service from
the same color/analyze image with independent fixture configuration, retaining
the prior container stopped for rollback. The 17:47 release updates Control-Service; earlier
Restore/rollback operations separately restarted organization runtime components.
The latest recorded combined Control Room/Control-Service release is
`zpr-editor-ui:20261015-font-ai-edits` (17:47 UTC).
Control-Service alone was subsequently updated to
`zpr-editor-ui:20261015-structured-edits` at 18:13 UTC; Control Room remains
on the 17:59 color/analyze executable in that entry. Both were subsequently
updated to `zpr-editor-ui:20261015-scroll-schema` at 18:29 UTC, replacing the
unsupported forced-tool protocol and adding caret/history fixes.
Control Room alone was subsequently updated to
`zpr-editor-ui:20261015-format-budget` at 18:38 UTC for Format history and the
assistant proxy timeout correction; Control-Service remains on the 18:29 binary.
Control-Service alone was subsequently updated to
`zpr-editor-ui:20261015-reasoning-envelope` at 18:45 UTC; Control Room stays on
the 18:38 Format/budget executable.
Both Control Room and Control-Service were subsequently updated in place to the
executable from `zpr-editor-ui:20261015-sole-editor` at 19:12 UTC. Their container
image configuration labels remain unchanged; the executable lives in each
existing writable layer.
Control Room alone was subsequently updated from
`zpr-editor-ui:20261015-ldap-tokens` at 19:36 UTC for compact assistant settings
and LDAP namespace completion. Control-Service remains on the 19:12 executable.
Both Control Room and Control-Service were subsequently updated in place from
`zpr-editor-ui:20261015-tokens-discard` at 19:55 UTC for increased token
limits/default, production-shaped LDAP completion and Discard.
Control Room alone was subsequently updated from
`zpr-editor-ui:20261015-namespace-steps` at 20:06 UTC for the clarified
hierarchical adapter LDAP completion UX. Control-Service remains at 19:55.
Retain each dated entry's precise scope and rollback evidence.

## Organizations Browse/File bundle repair (2026-10-15)

- The owning Organizations session rebuilt/restarted Simulator for the
  Browse/File UI, then repaired a live mismatch: new HTML was served with old
  JavaScript lacking the menu handlers. The matching bundle was verified
  before redeployment. Browse, File and Edit now work after navigation with
  real mouse clicks on the live page and no JavaScript errors.
- Compact heading/summary layout gives the directory graph a 600px canvas at
  the tested 900px screen height. The session reports 22 passing desktop/tablet
  checks. Browsing does not activate an organization; private drafts remain
  separate from runnable organizations.
- This is a separate operation from the 20:34 editor-only release below.
  Do not infer one combined immutable image or claim a synchronized backend
  rollout. Exact runtime identity/configuration preservation from another
  entry must not be applied to this restart.
- DNS recovery was required because the restart helper stops DNS as a side
  effect. The session reports restored BIND, DNS queries and statistics, but
  the DNS adapter cannot reattach while its node reports a disconnected Visa
  Service connection. Full network health/recovery remains open; no successful
  adapter reattachment or sustained recovery is claimed.
- This entry reconciles the completed owning-session report; it is not a new
  live inspection, deployment or authorization to reset runtime state.

## Remaining source editors (2026-10-15 20:34 UTC)

- Scope: Control Room and Simulator editing surfaces only. User explicitly
  selected isolation from concurrent uncommitted backend/Organization UI work.
  Build context is clean `d741cec` plus an explicit editor migration file
  allowlist; image `zpr-editor-ui:20261015-all-editors`, executable SHA-256
  `f873f11e9fa3e44593e60ef3acc044c7a4a8a34c814c4ea18c8636dc253f23fc`.
  Container image labels remain unchanged; executable installed in writable
  layers. Room started `2026-10-15T20:34:48.679628502Z`, Simulator
  `2026-10-15T20:34:50.04039217Z`.
- Gateway JSON, ZPR Config TOML, Directory LDIF and Scenario JSON/YAML now use
  the shared CodeMirror editor. Preserve original typography, language colors,
  native form inputs, source-owned diagnostics, permissions/limits, explicit
  save/publish semantics, Find/Replace and Wrap. Format is undoable. Scenario
  format conversion retains its source/history boundary.
- All 78 desktop/tablet editor checks pass against the exact isolated release.
  Actual served HTML and 46 Room/28 Simulator assets byte-match staged sources.
  All four served editors verify language colors, hidden legacy editing, exact
  edit Undo and Format Undo where supported under actual CSP. Actual Room
  sign-in gate and verified-mTLS compiler/tester/assistant readiness pass.
  Browser APIs mocked; no live save, analyze, publish, provider or runtime calls.
- Room exact configuration/mounts/ID preserved. Control-Service and
  Policy-Service identity/image/start baselines unchanged.
- Concurrent operation recreated Simulator between the initial snapshot and
  installation. This rollout did not recreate it. Verification stopped at that
  discrepancy; user approved testing the replacement as-is without modifying
  its configuration. Original Simulator configuration preservation is not
  claimed. Replacement ID/configuration/sorted mounts stayed unchanged through
  verification; release executable hash matches in both running containers.
- Private artifacts: `files/all-editors-release-20261015/` in session storage
  contain isolated build sources, executable, rollback binaries, private inspect
  snapshots, test results and live verification script. Do not publish inspect
  files. Rollback each executable only: stop the affected current container,
  restore `control-room-before` or `simulator-before` to
  `/usr/local/bin/zpr-web-dashboard`, start and verify. Do not restore old
  Simulator container configuration over the concurrent replacement.
  Both containers also retain the named `.before-all-editors-20261015` binary.
- Refresh/reopen editor pages. No commit or push performed.

Publication follow-up: `c3a3836` is the preceding source checkpoint for the
Restore/runtime repairs and Scenario browser. The current source checkpoint
also includes the subsequent editor, management HTTPS, enrollment ownership,
production-boundary refactoring and system-release preparation work.
"No commit or push" in dated entries records the state at release time.
Publication does not deploy additional CLI/service changes or expand prior
verification claims.

## Hierarchical adapter LDAP completion (2026-10-15 20:06 UTC)

- Approved scope: Control Room only after draft secured. Build tag
  `zpr-editor-ui:20261015-namespace-steps`; executable SHA-256
  `5c8333202acfb209828903805f01268342af0282b31179972fdf3fbe34b5686a`.
  Existing Room container executable replaced in place without recreation;
  started `2026-10-15T20:06:00.920206175Z`.
- Clarified requirement supersedes the previous full-catalog insertion behavior:
  in adapter definitions `with grea` offers only `greatlakes.`; accepting opens
  its attributes, and location completes to `greatlakes.location:`.
  No injected built-in type prefix unless explicitly typed, no leading colon,
  one namespace segment per step and separate namespace/leaf Undo.
  User/device/service qualified completions retain existing behavior.
- All 52 desktop/tablet editor checks pass. No Go changes or provider requests.
  Complete Room Config/HostConfig/mounts/ID preserved, all backend/Simulator
  identity/image/start baselines unchanged, executable hash matches and
  verified-mTLS compiler/tester/assistant readiness passes.
- Actual served HTML/46 assets byte-match source. Actual-CSP browser verification
  uses production-shaped catalog data and confirms the single namespace option,
  immediate child menu, exact leaf insertion and both Undo steps. Sign-in gate
  preserved; no browser errors/CSP violations. Browser APIs mocked; no Save,
  Analyze, provider call or backend mutation.
- Private session artifacts: `files/namespace-release-20261015/` contains the
  release/rollback executables, private inspect snapshots, unchanged service
  baselines and verification script. Never publish inspect files.
- Rollback: stop Control Room only, restore `control-room-before` from that
  directory to `/usr/local/bin/zpr-web-dashboard`, start and verify. Container
  also retains `/usr/local/bin/zpr-web-dashboard.before-namespace-steps-20261015`.
  Prior version is the 19:55 tokens/Discard release. Refresh/sign in again.
  Not committed or pushed.

## Token limits, production LDAP completion and Discard (2026-10-15 19:55 UTC)

- Approved scope: Control Room and Control-Service after draft secured.
  Policy-Service and Simulator untouched. Build tag
  `zpr-editor-ui:20261015-tokens-discard`; executable SHA-256
  `ffdfcb4156425ca9ac959f97b840d17e088fbf56e52350911f47b60c0188643f`.
  Replace `/usr/local/bin/zpr-web-dashboard` in existing containers without
  recreation or using the image's default Simulator entrypoint.
  Control-Service started `2026-10-15T19:55:57.624806882Z`;
  Control Room started `2026-10-15T19:55:58.718916674Z`.
- Triple selectable token choices to 900/1800/3600/7200, with doubled default
  2400 added. Both backend validators default to 2400 and accept the new values;
  preserve 300/600/1200 for older open clients. No automatic retries or raising
  requested limits.
- Correct prior autocomplete fixture mismatch: production advertises
  `device.greatlakes.*`, not bare `greatlakes.*`. Short namespace matching
  inserts the exact full catalog name. Fix active-statement boundaries in
  multi-statement policies, without directory-record/API/provider disclosure.
- Discard explicitly forces same-record policy reload; ordinary same-record
  selection and assertion-save behavior remain unchanged. Cancel preserves the
  draft, failed reload is explicit and non-destructive, and successful Discard
  resets source history.
- All 54 selected browser checks pass, including production-shaped catalog,
  multi-statement completion/insertion/Undo, policy/assertion Discard, failed
  reload, cancellation, choices/default and shared assistants. Focused Go race
  tests/vet pass; required full `make test` retains the four known failures.
- Production verification: exact Room/Service IDs, complete Config/HostConfig
  and mounts unchanged; Policy-Service/Simulator identity/image/start baselines
  unchanged. Both deployed hashes match. Verified-mTLS policy readiness reports
  configured with compiler/tester/assistant ready.
- Served HTML and 46 assets byte-match source. Actual-CSP browser verification
  confirms choices/default, production-shaped LDAP completion in multi-statement
  text, insertion/Undo, Discard cancellation/reload/history reset and sign-in
  gate, with no browser errors or CSP violations. Browser API calls mocked;
  no Save/Analyze/provider request or runtime mutation.
- Private artifacts: session `files/tokens-discard-release-20261015/`, with both
  prior executables, new executable, private before/after inspect snapshots,
  untouched baselines and isolated verification script. Do not publish inspect
  files, which may contain operator secrets.
- Rollback: stop only the affected Room or Service container, restore its
  matching `control-room-before` or `control-service-before` executable to
  `/usr/local/bin/zpr-web-dashboard`, start and recheck readiness.
  Each also retains
  `/usr/local/bin/zpr-web-dashboard.before-tokens-discard-20261015`.
  Previous versions: Room 19:36 LDAP/tokens UI, Service 19:12 sole-editor.
  Refresh/sign in again to load the update. Not committed or pushed.

## Assistant settings and LDAP namespace completion (2026-10-15 19:36 UTC)

- Approved scope: Control Room only, after the operator confirmed the draft
  was secured. Control-Service, Policy-Service and Simulator were not updated.
  No Go logic changed in this follow-up; embedded UI assets require a new
  shared executable, built from the current worktree.
- Build tag `zpr-editor-ui:20261015-ldap-tokens`; executable SHA-256
  `7a5e86244ba79eaec7091503bba8c13d0c7174372090cb5b948f479d1b8c51e5`.
  Replace `/usr/local/bin/zpr-web-dashboard` in the existing Room container;
  preserve its entrypoint, arguments, configuration and mounts rather than
  using the image's default Simulator entrypoint. Room started
  `2026-10-15T19:36:03.556122551Z`.
- Remove the requested direct-edit/Undo/autosave disclosure sentences; retain
  only the draft/chat transmission notice. Rename the shared section
  **Model and Tokens** and move token stats inside it. Update related help
  and asset cache versions.
- Reproduce `define great as adapter with greatlakes.`: the completion parser
  mistook non-built-in namespace dots for statement endings. Repair arbitrary
  and multi-part LDAP attribute namespaces, preserving comments/string/value
  suppression and existing attribute filtering. No new LDAP API/provider calls.
- All 44 selected desktop/tablet editor/shared-assistant checks passed, including
  automatic namespaced suggestions, insertion/Undo, short/qualified names and
  exact disclosure/stats layout. Editor problems and whitespace checks pass.
- Production verification: exact Room container ID, complete `Config`,
  `HostConfig` and mounts unchanged. All backend/Simulator container identities,
  image labels and start times unchanged. Verified-mTLS policy readiness reports
  configured Great Lakes with compiler/tester/assistant ready.
  Executable hash matches; actual served HTML and 46 assets byte-match source.
  Actual-CSP browser checks verify exact text, renamed section, hidden/expanded
  stats, automatic `greatlakes.` suggestions/insertion/Undo and sign-in gate,
  without browser errors or CSP violations. All browser APIs mocked; no policy
  saves, Analyze, AI-provider requests or backend runtime changes.
- Private session artifacts: `files/ldap-tokens-release-20261015/`, including
  release/rollback executables, before/after private inspect files, untouched
  service baselines and isolated verification script. Never publish inspection
  files, which may contain operator secrets.
- Rollback: stop only Control Room, restore `control-room-before` from that
  private directory to `/usr/local/bin/zpr-web-dashboard`, start and recheck.
  The container also retains
  `/usr/local/bin/zpr-web-dashboard.before-ldap-tokens-20261015`.
  Previous executable is the 19:12 sole-editor release. Refresh/sign in again
  to load this update. Not committed or pushed.

## Sole editor and explicit AI edit/usage results (2026-10-15 19:12 UTC)

- Approved scope: Control Room and Control-Service only, after the operator
  confirmed the draft was secured. The shared executable also includes the
  concurrent diagnostics refactor, explicitly disclosed and approved. No
  diagnostics source changes were made by this editor follow-up.
- Build tag: `zpr-editor-ui:20261015-sole-editor`.
  Executable SHA-256:
  `008d2eaa467ef644349f0619ee9a259da86477fc79e35f49e9d77dfe6f4b9c9d`.
  Replaced `/usr/local/bin/zpr-web-dashboard` inside each existing container,
  without using the image's default Simulator entrypoint or recreating services.
  Control-Service started `2026-10-15T19:12:09.132458429Z`; Control Room started
  `2026-10-15T19:12:20.096753128Z`.
- CodeMirror is the sole policy/assertion editing UI, initialized automatically.
  Hidden source/highlight/result models remain controller bridges. No legacy
  switch or interactive fallback; startup failure is explicit and AI stays
  disabled. Source changes are verified; no-op edits are rejected, no-edit
  answers are identified neutrally, and the appended success sentence is gone.
- Show per-request output/requested maximum separately from session usage.
  Preserve reported usage on structured-response failures, reject reported
  overruns, and never automatically retry. The sole approved provider probe
  used synthetic-only input: 381 input / 77 output, with a 300-output maximum,
  complete edits and `end_turn`. It did not reproduce the first-request
  overrun; no claim of guaranteed billing enforcement or provider defect fix.
- Verification: 38 editor and 12 selected other-assistant desktop/tablet checks
  passed; focused Go race tests/vet passed. Full `make test` retains the four
  known failures. Two selected Scenario browser checks fail before assistant
  interaction on an unchanged, obsolete setup selector and were not modified.
- Production: both executable hashes match the release. Complete
  `Config`, `HostConfig`, mounts and container IDs are unchanged. Independent
  Policy-Service and Simulator image/start/ID baselines are unchanged.
  Verified-mTLS policy status reports configured Great Lakes and assistant,
  compiler and tester ready.
- Actual served HTML and 46 checked assets byte-match source. Actual CSP browser
  checks pass for automatic startup with no old editor, mocked direct AI visible
  source edits, one-step Undo/Redo, usage, no appended sentence/Insert, Format
  history and the sign-in gate. API calls in that verification were intercepted;
  no real policy or history went to AI, and no Save/Analyze/Restore/runtime
  mutation was performed.
- Private artifact directory: session
  `files/sole-editor-release-20261015/`, containing the release executable,
  both rollback executables, before/after inspect snapshots, unchanged-service
  baselines, policy readiness response and the isolated verification script.
  Inspection files are private and may contain operator secrets; do not publish.
- Rollback: stop only the affected Room or Service container; copy its matching
  `control-room-before` or `control-service-before` executable back to
  `/usr/local/bin/zpr-web-dashboard`; start and repeat readiness checks.
  Each container also retains
  `/usr/local/bin/zpr-web-dashboard.before-sole-editor-20261015`.
  The prior Room/Service versions are the 18:38 Format/budget and 18:45
  reasoning-envelope executables respectively. This follow-up is not committed
  or pushed. Refresh and sign in again to load the new editor.

## Source publication checkpoint (2026-10-15 18:20 UTC)

- User requested committing and pushing all pending work. This checkpoint
  includes the implementation, regression tests, browser bundle/licenses,
  packaging preparation and documentation accumulated since `c3a3836`.
  System-release preparation remains scaffolding, not production readiness;
  all seven reorganization milestone gates remain open.
- Publication checks pass: focused command compatibility, production
  contracts, inventory, gateway, fixtures, management HTTPS, enrollment relay
  and editor race tests; race suites for the affected internal owners and
  system-config command; all 28 selected desktop/tablet CodeMirror browser
  checks; all 19 fixture-based host preflight checks; Go vet, formatting and
  whitespace checks.
- Required `make test` retains exactly four known failure groups: enrollment
  readiness, stale-bundle platform fixture, placement shell extraction and
  trusted-service ELF fixture. No existing failing test was changed to hide
  these failures.
- Private discovery scan and editor swap artifacts remain outside publication.
  The previously excluded, diverged `zpr-common` repository is unchanged.
  Refreshed remote state confirms the existing `zpr-core` checkpoint
  `561fa73` is already published.
- This is source publication only: no service deployment, restart, Restore,
  live model request or policy mutation is performed by this checkpoint.
  The dated deployment scopes above and below remain authoritative.

## Assistant reasoning-envelope repair (2026-10-15 18:45 UTC)

- User approved a synthetic-only refactor probe and then deployment to
  Control-Service only. Real provider response metadata: `end_turn`, one
  `thinking` block followed by one complete JSON text payload. The old parser
  incorrectly rejected that envelope. No operator policy, directory data or
  conversation history was sent during the probe.
- Built Linux arm64 image `zpr-editor-ui:20261015-reasoning-envelope`, image ID
  `sha256:ae990823a821852b550b29f08429c96b4987a405cb55b996a3f6dced3d67c737`.
  Installed executable SHA-256:
  `aa7ef8106b3bcfaccc3b88c232088f85d082cb8de216e062bcf8ff7ce2f890cf`.
  Control-Service started at `2026-10-15T18:45:29.677215466Z`.
- Skip only known reasoning blocks, strictly validate the single text/edit
  payload, and retain truncation/unknown-block/tool/schema rejection. Errors
  identify the failing validation stage without source, payload or reasoning.
- Captured synthetic multi-line refactor replay through the actual parser
  passes with nonempty edits and preserved usage. Permanent transport/route
  tests cover reasoning envelopes and negative cases; focused race tests/vet
  and whitespace checks pass. Required full Go tests retain four known baseline
  failures. Browser assets are unchanged; no browser rerun for this backend fix.
- Existing Control-Service container ID, Config/HostConfig and normalized
  mounts compare equal. Only its writable-layer executable was replaced.
  Recreating from its original image discards this update; shared image
  defaults require explicit existing production settings/mode for recreation.
- Production verified-mTLS status reads Great Lakes configured and assistant,
  compiler and tester ready. Control Room, Policy-Service and Simulator
  start/image baselines remain unchanged. Open editor pages need no refresh.
  No actual operator refactor, policy save, Restore, runtime mutation, commit
  or push; operator retry remains live policy acceptance.
- Rollback executable retained privately and in Control-Service at
  `/usr/local/bin/zpr-web-dashboard-before-reasoning-envelope-20261015`,
  prior SHA-256
  `a30c07e498f22bbb57ebd660326f97b4b8ae8e072eb06ada948c05031ced9858`.
  Restore it and restart only Control-Service while preserving configuration.
  Rollback reinstates the reasoning-envelope validation bug.

## Format history and assistant proxy budget (2026-10-15 18:38 UTC)

- User approved updating only Control Room after securing the draft. Built
  Linux arm64 image `zpr-editor-ui:20261015-format-budget`, image ID
  `sha256:be9bc2e466aa207022274138a35ce9b54adc4c1f3741a8523e187d83a77d3991`.
  Installed executable SHA-256:
  `aa5fd385183e57611a6962b2c347b84dd2a150a6e70fc519c987711cdf3266cc`.
  Control Room started at `2026-10-15T18:38:34.416243969Z`.
- Existing container ID, full Config/HostConfig and normalized mounts compare
  equal. Only its writable-layer executable changed; recreating from the
  original image loses this update. Shared artifact defaults are not suitable
  for production recreation without explicit existing production settings.
- Policy Format now uses the shared edit helper. Shared CodeMirror replacement
  transactions are history-isolated, so Format is one Undo/Redo step and earlier
  typing remains undoable. Assertion Format uses the same path. App cache
  version is 164; trial/bootstrap bundle versions are 5.
- At 18:34:10 the previous Room proxy timed out the assistant and canceled its
  upstream model call. Exact POST policy/gateway assistant routes now use a
  75-second response-header budget for context lookup plus the existing
  45-second provider request. Ordinary/log API budgets and mTLS settings are
  unchanged. Browser cancellation continues to reach the upstream request.
- All 34 selected desktop/tablet editor checks pass, including exact
  Format Undo/Redo and earlier edit restoration for both source types.
  Focused proxy/assistant route race tests, vet, deterministic bundle, syntax
  and whitespace checks pass. Required full Go suite retains the same four
  known failures. Proxy tests verify exact route/method/budget/TLS behavior,
  delayed response survival and cancellation with Simulator unavailable.
- Root HTML and 46 checked assets match source. Actual served assets/CSP with
  intercepted APIs verify Format history, mock AI and Undo without CSP/script
  errors. Operator login gate is intact and live verified-mTLS readiness
  reports Great Lakes configured and assistant ready. No live model request
  or actual Milwaukee refactor was sent during validation; operator retry
  remains the live end-to-end acceptance.
- Control-Service, Policy-Service and Simulator image/start baselines are
  unchanged. No policy save, Restore, runtime change, commit or push.
- Rollback executable retained privately and inside Control Room at
  `/usr/local/bin/zpr-web-dashboard-before-format-budget-20261015`, SHA-256
  `a30c07e498f22bbb57ebd660326f97b4b8ae8e072eb06ada948c05031ced9858`.
  Restore that executable and restart only Control Room, preserving settings.
  The rollback reinstates the old assistant proxy budget and Format behavior.
  Refresh/sign in again and enable CodeMirror trial to load the new UI.

## Scroll-aware caret, toolbar history and supported AI schema (2026-10-15 18:29 UTC)

- User approved updating Control Room and Control-Service after securing the
  unsaved draft. Built Linux arm64 image
  `zpr-editor-ui:20261015-scroll-schema`, image ID
  `sha256:9a24d128a1ee3ec79ded2bc2070668cc8229c14ff256c2aa3adccbd3d413fb79`.
  Installed executable SHA-256 for both:
  `a30c07e498f22bbb57ebd660326f97b4b8ae8e072eb06ada948c05031ced9858`.
- Control-Service started at `2026-10-15T18:29:51.485948712Z`, then Control
  Room at `2026-10-15T18:29:52.369860255Z`. Both retain the same container IDs,
  complete Config/HostConfig and normalized mounts; only writable-layer
  executables changed. Recreating from original images loses these updates;
  shared artifact defaults must not be used without explicit production modes
  and existing settings.
- Actual provider rejection was HTTP 400: the configured model does not
  support forced `tool_choice` values. Replaced the 18:13 forced-tool protocol
  with native JSON-schema output. Approved live synthetic-only probes confirmed
  both no-edit and exact replacement payloads. No policy/directory data or
  conversation history was sent to the provider for testing.
- CodeMirror uses its own measured caret and selection layers. Tests check
  exact click/insertion positions and caret alignment/following after scrolling.
  Toolbar Undo/Redo now targets the visible source, including focus on Analyze;
  other text fields retain their own history. Browser cache versions advanced
  to 4. Refresh/sign in again and re-enable CodeMirror trial.
- All 32 selected desktop/tablet browser checks, focused transport/route race
  tests, vet, syntax, deterministic bundle and whitespace checks pass. Required
  full Go suite retains the four known failures (enrollment readiness,
  stale-bundle platform fixture, placement shell extraction and trusted-service
  ELF fixture).
- Root HTML and 46 checked served assets match source. A browser using actual
  served assets/CSP with intercepted APIs verifies scroll-following caret,
  Undo/Redo from Analyze, mock AI application and Undo, without JavaScript/CSP
  errors. Operator login gate remains intact. Production verified-mTLS status
  reports Great Lakes configured and assistant/compiler/tester ready.
- Policy-Service and Simulator image/start baselines are unchanged. No policy
  save, Restore, organization/runtime mutation, commit or push.
- Both rollback executables are retained privately and in the affected
  containers at `/usr/local/bin/zpr-web-dashboard-before-scroll-schema-20261015`.
  Previous Control Room SHA-256:
  `eefbe4ab627dee2834645ef46eceb5d7f0063026d181fa06d4fd9e5d17af2c01`;
  previous Control-Service SHA-256:
  `5a6fd0bb5ec71904ae8d8c4757e1c0d4c0ad2e1c05feb1cbfe4fccdf4e646b9e`.
  Restore the corresponding backup executable and restart only affected
  containers. The previous Control-Service version contains the now-proven
  unsupported forced-tool protocol.

## Structured AI editor responses (2026-10-15 18:13 UTC)

- User approved deployment to Control-Service only, with Control Room kept open.
  Built image `zpr-editor-ui:20261015-structured-edits`, image ID
  `sha256:1b4033cf74c0cf55ff0f21ffcbc265e68ebc4b426da281fae66c667986d7c747`.
  Installed executable SHA-256:
  `5a6fd0bb5ec71904ae8d8c4757e1c0d4c0ad2e1c05feb1cbfe4fccdf4e646b9e`.
  Control-Service started at `2026-10-15T18:13:11.063946805Z`.
- Only the existing Control-Service container's writable-layer executable
  changed. Container ID, complete Config/HostConfig and normalized mounts
  compare equal before/after. Recreating it from its original image loses this
  executable update; the artifact's default mode is Simulator and must not be
  used for production recreation without explicit existing production settings.
- Direct-edit requests now force a structured tool response, validate it server
  side and return the existing browser answer envelope. Token truncation has
  a specific no-changes-applied error; no partial application or automatic
  retry. Browser source/record guards and atomic undoable edits are unchanged.
  Existing open pages can retry without refreshing.
- Transport/route race tests and vet pass; all 28 selected editor browser
  checks pass, including failed-request draft preservation and safe retry.
  Required full Go suite retains the same four known baseline failures.
- A live verified-mTLS GET from Control Room through Control-Service returns
  Great Lakes configured, assistant ready, compiler ready and tester ready.
  Control Room, Policy-Service and Simulator image/start baselines are
  unchanged. No live provider/model request or policy save was performed.
  Mocked provider tests verify the structured protocol; live-model acceptance
  remains for the operator's retry.
- Rollback executable retained privately and at
  `/usr/local/bin/zpr-web-dashboard-before-structured-edits-20261015` inside
  Control-Service. Prior SHA-256:
  `3bbfb29370ef734c710af0379fe9655a4a04eba6120a3700c3d3f754d67194b0`.
  Restore the backup executable and restart only Control-Service, retaining
  existing configuration. No Restore, runtime changes, commit or push.

## Editor color parity and Analyze repair (2026-10-15 17:59 UTC)

- User approved the highlighting deployment and independent-fixture repair
  including Policy-Service update/restart, without Restore or policy mutation.
  Built Linux arm64 image `zpr-editor-ui:20261015-color-analyze`, image ID
  `sha256:27adf4ce6bb85adc081ea81d465d6f6866268014d6c6216a71b00909d110714b`.
  Installed executable SHA-256 for both affected services:
  `eefbe4ab627dee2834645ef46eceb5d7f0063026d181fa06d4fd9e5d17af2c01`.
- Policy-Service started at `2026-10-15T17:59:12.10978875Z`; Control Room
  started at `2026-10-15T17:59:29.911814217Z`. Control Room retains its existing
  container/image/configuration with a writable-layer executable replacement.
  Recreating it from its old image would lose that replacement.
- Policy-Service was recreated with its exact original Config/HostConfig/mounts,
  changing only the image and adding `ZPR_POLICY_TEST_FIXTURES_FILE`. Its
  explicit `-mode policy-service` entrypoint/settings remain intact; shared
  image defaults are not used. The launcher policy-service image marker now
  points to this release. The old container remains stopped as
  `zpr-policy-service-before-color-analyze-20261015`.
- A one-time provisioning export using the existing jq adapter produced the
  version-1 Great Lakes operator fixture JSON under the runtime
  `dashboard-stack/policy-test-fixtures/` directory (mode 0600). Production
  reads that independent contract and live LDAP, not Simulator sources/APIs.
  There are no organization/runtime switches or Restore operations.
- Live fixture GET now returns HTTP 200 with 38 actors and 10 services.
  A non-saving `POST /api/policy/test` with a small definition-only source
  returned HTTP 200, API version 1 and 380 evaluated actor/service pairs.
  The actual existing Effective network policy also returned HTTP 200 with
  38 actors, 19 result rows and 380 evaluations, without saving it. It references
  none of the omitted attributes.
  Requests used the existing verified mTLS Control Room -> Control-Service
  path. Three unsupported coordinate-valued attributes remain explicitly
  omitted by the fixture format; policies referencing them remain unsupported.
- Root HTML and all 46 checked assets match source. Actual served assets/CSP
  pass legacy token/color parity, native typing, mock-AI direct edits and
  Undo/Redo without JavaScript/CSP errors. Browser API traffic was intercepted;
  no live model call, policy save or authenticated operator Analyze click was
  performed. The login gate is intact; refresh/sign in again and re-enable
  CodeMirror trial, which still defaults off.
- All 26 selected browser checks, focused CSP/fixture/Analyze/assistant race
  tests and vet pass. Required full Go tests retain the four existing failures:
  enrollment readiness, stale-bundle platform fixture, placement shell
  extraction and trusted-service ELF fixture.
- Simulator and Control-Service image/start baselines are unchanged; assertion
  records are byte-identical before/after. No commit or push.
  The isolated sample preview was restarted with the matching nine-color CSP;
  its sample-only in-memory state reset, and its health endpoint responds.
- Rollback Control Room using
  `/usr/local/bin/zpr-web-dashboard-before-color-analyze-20261015` then restart
  only that container. Rollback Policy-Service by stopping/renaming the new
  container and renaming/starting the retained old container; never run both
  against the same DB. Restore its launcher image marker to
  `zpr-simulator:20261015-assertion-seed` if rolling back. Both prior executables
  and exact configuration snapshots are retained privately.

## Control Room/Service font parity and direct AI edits (2026-10-15 17:47 UTC)

- The user approved deploying the current worktree to Control Room and
  Control-Service, including other pending backend changes. Built Linux arm64
  artifact image `zpr-editor-ui:20261015-font-ai-edits`, image ID
  `sha256:a510f6bcbd10f3b860cbe7700ac1566f5bddf32419b6a83ffd33ca60a6d22f0a`.
  Both installed executable SHA-256 values are
  `3bbfb29370ef734c710af0379fe9655a4a04eba6120a3700c3d3f754d67194b0`.
- Existing containers, original image IDs and configuration remain unchanged;
  only their writable-layer executables were replaced. Control-Service started
  at `2026-10-15T17:47:49.467275545Z`, followed by Control Room at
  `2026-10-15T17:47:50.189710045Z`. Both restart counts remain zero. Full
  Config/HostConfig and normalized mount comparisons pass, preserving
  certificates, settings, ports and entrypoints. Recreating either container
  from its original image would discard the executable update; the build
  artifact uses the shared Simulator Dockerfile and requires explicit production
  mode/configuration if used for container recreation.
- CodeMirror policy/assertion typography matches the previous editor. AI
  requests in the trial use structured exact-match edits applied directly as
  one isolated undoable unsaved change. Other editors retain Insert. No
  automatic Save/Analyze/staging/runtime action is introduced. Refresh the page,
  sign in again and enable CodeMirror trial; it still defaults off.
- Root HTML and 46 checked assets (HTML references plus the lazy editor bundle)
  match source byte-for-byte. HTTPS and enabled operator login respond
  successfully; the browser sign-in gate is preserved. A real verified mTLS
  request from Control Room to Control-Service reads Great Lakes policy context.
  No credentials were disclosed.
- A separate mocked-data browser check using actual served assets/CSP verifies
  matching font size/family/line height, direct AI line replacement/insertion
  and complete Undo/Redo with no JavaScript/CSP errors. All API traffic in that
  check is intercepted locally; no live AI request or policy save was made.
  This is not authenticated live-model acceptance.
- Pre-rollout: all 26 selected desktop/tablet checks, focused assistant/CSP
  race tests, vet, embedded build, syntax, reproducible bundle and whitespace
  checks pass. Required `make test` retains the same four known failures:
  enrollment readiness, stale-bundle platform fixture, placement shell
  extraction and trusted-service ELF fixture.
- Simulator and Policy-Service image IDs/start times are unchanged. No policy,
  runtime, directory or organization mutation, commit or push.
- Rollback executables are retained at
  `/usr/local/bin/zpr-web-dashboard-before-font-ai-20261015` in each container,
  and in private session artifacts. Prior Control Room executable:
  `27f13d9ed041ef23ed238115388abbd3e8f2a32384463b6abc2a37a1650edebc`;
  prior Control-Service executable:
  `194f60aeb656bb624c21b9040c99c38d31a69912bdb8ea05a8be36fdbd9648a8`.
  Roll back both together: copy each backup to a temporary executable path,
  atomically move over `/usr/local/bin/zpr-web-dashboard`, then restart only the
  two affected containers. Preserve executable permissions and configuration.
  Private inspect snapshots contain environment settings; do not publish them.

## Control Room CodeMirror trial (2026-10-15 17:30 UTC)

- The user selected Control Room deployment and explicitly approved the current
  worktree, including pending policy-fixture and operator-file refactoring
  changes. Built the Linux arm64 artifact image
  `zpr-editor-ui:20261015-codemirror-trial`, image ID
  `sha256:96cf509733d5d2ec2102151d30fdbf43a60813bcb6def648df0e6f787daf2523`.
  Installed executable SHA-256:
  `27f13d9ed041ef23ed238115388abbd3e8f2a32384463b6abc2a37a1650edebc`.
- Updated the existing `zpr-control-room` container's writable-layer executable;
  container ID
  `2487b772996217b8964e53c63e1ef402b49f9cd9c0a0d0fa101c3ae71aa45567`
  and original image ID are unchanged. It restarted at
  `2026-10-15T17:30:30.678490592Z`. The image tag above is the build artifact,
  not a replacement container configuration; recreating from the original image
  would discard the writable-layer update. The build Dockerfile's default
  entrypoint is Simulator mode; do not run the artifact image without explicit
  Control Room mode and the preserved production configuration.
- Full `Config`/`HostConfig` comparison and destination-normalized mount
  comparison pass: environment, entrypoint, command, ports, certificates,
  operator settings and mounts are preserved. Control-Service, Policy-Service
  and Simulator image IDs/start times are unchanged. No runtime or policy
  mutation, Simulator update, commit or push.
- **CodeMirror trial** is opt-in under Policy/Assertions and defaults off on
  each reload. Existing text survives switching implementations. Only the five
  approved exact color declarations are allowed by CSP hashes; arbitrary
  inline styles/scripts remain prohibited. Restart cleared in-memory sessions;
  operators must sign in again.
- All 47 checked assets (HTML-referenced resources plus the lazy editor bundle
  and license file), and the root HTML, match the worktree byte-for-byte. HTTPS `/` and
  `/auth/operator/config` return 200, operator login remains enabled,
  `/index.html` retains its 301 redirect, and signed-out `/api/snapshot` returns
  403. A browser confirms the operator sign-in gate. A separate no-write,
  mocked-data browser check against the served assets/actual CSP verifies
  typing, switching back and re-enabling, with no JavaScript/CSP errors.
  All API writes were blocked; none were attempted. No authenticated live-data,
  real compiler or persisted Save check is claimed.
- Pre-rollout validation: 14 desktop/tablet trial checks and focused CSP race
  test pass; Linux embedded build succeeds. Required `make test` retains the
  same four known failures: enrollment readiness, stale-bundle platform fixture,
  placement shell extraction and trusted-service ELF fixture.
- Exact prior executable SHA-256:
  `3449abfc490d535bab303482df40caa26e87dd68e558833c297d28c2ae50f21c`,
  retained in-container at
  `/usr/local/bin/zpr-web-dashboard-before-codemirror-20261015` and in the private
  session release artifacts. Roll back by copying that backup to a temporary
  executable path, atomically moving it over `/usr/local/bin/zpr-web-dashboard`,
  then restarting only `zpr-control-room`. Preserve its executable permissions.
  Configuration snapshots and verification screenshots remain private session
  artifacts; do not publish the snapshots, which contain environment settings.

## Scenario browser deployment (2026-10-15 16:24 UTC)

- Deployed Simulator only as `zpr-simulator:20261015-scenario-browser`, image
  `sha256:ff5cb4db4f1001300d14ca0d966c9fcd5d4d3939073568a32825e0adf3ec7c5e`,
  started at `2026-10-15T16:24:55.482945882Z`. The user explicitly approved
  building the current dashboard worktree, including pending backend changes,
  rather than isolating the embedded Scenario assets.
- Browse... now opens scenario folders in a dialog; File... offers New/Edit.
  Only the selected file remains on the page, with full-width Runner/Results.
  Running/cleanup output is collapsed under Details; failures remain visible.
- Confirmed no active scenario or organization operation before replacing the
  container. Preserved environment, bind mounts, ports, network, entrypoint and
  restart policy; normalized configuration comparison passes. Great Lakes
  remains active and all five catalog entries are available. Restart cleared
  the completed in-memory Regional run; its API response was saved first.
- Restored Simulator-hosted DNS statistics and records relays. Statistics
  returns valid BIND status JSON; the records relay accepts TCP connections.
  Control Room, Control-Service and Policy-Service retain their exact previous
  image IDs and start timestamps. No Restore, activation or scenario was run.
- Four served assets match source byte-for-byte: scenarios.html, scenarios.js,
  simulator.css and sidebar.js. A live Chromium check passes Browse/folders,
  file selection, File menu and full-width tracks with API writes blocked
  (none attempted). The pre-deployment 20 focused desktop/tablet browser checks
  and embedded build pass; broader regression limitations remain documented in
  the completed-work archive. Live running-step collapse was not exercised by
  starting a workload; it is covered by fixture-based browser tests.
- Rollback: retain stopped container
  `zpr-simulator-before-scenario-browser-20261015` and its previous
  `zpr-simulator:20261015-business-denials` image. To roll back, stop and rename
  the new container, restore the retained container's name to `zpr-simulator`,
  start it, then restore both hosted DNS relays. Private configuration and
  pre-release API snapshots plus live screenshot/check evidence are in session
  `files/scenario-browser-release/`.
- No commit or push was performed.

## Regional Business runtime recovery (2026-10-15 15:04–15:12 UTC)

- Preserved the user's 14:54 UTC regional failure. All seven providers started;
  machine-07's Tijuana controller then timed out connecting to SimulatorControl.
  Reproduced with only that controller, without business mutations, and stopped
  the reproduction container afterwards.
- HQ and Shenzhen node authentication had expired although RPC pings still
  succeeded and node links appeared Active. Visa pushes remained queued and VSS
  requests were denied with `DestAuthError`. The running October 12 Visa binary
  also lacked the source's pending-visa pull recovery.
- Visa RPC pings now return `AuthError` within one minute of authentication
  expiry. The existing client ping/reconnect path performs a fresh signed
  challenge without deliberately resetting node state. Regression tests cover
  expired, near-expiry and valid authentication.
- Built and deployed `zpr-multinode:20261015-node-renewal`, image
  `sha256:d8fceb90d790ee248ac4878c8567611374ae59386c4cc670e09ea8d75625b882`.
  Used normal retained-state deployment (`ZPR_RESTORE_BASE=0`) and reconnected
  nodes. This updates Visa only within the runtime image; packet-handler
  binaries remain unchanged. Existing policies, directory and business data
  were preserved; no Restore was performed.
- The retry connected across sites but exposed a second problem: Business
  denial checks required direct Visa Admin environment variables absent from
  deployed Simulator. Checks now use its configured operator snapshot transport
  (mTLS Control-Service), retaining the exact source/destination/TCP/port
  counter-increment requirement. Missing/failed denial data cannot pass.
  Explicit direct Admin remains supported when no operator connection is selected.
- Deployed Simulator `zpr-simulator:20261015-business-denials` at
  `2026-10-15T15:09:07.638307429Z`, image
  `sha256:b411fa97ad750851146d64638c7036cdc82763c6d4047d4be92c7aa5bf664ed1`.
  Preserved configuration/mounts/ports, restored DNS relays, retained
  `zpr-simulator-before-business-denials-20261015`, and persisted the new runtime
  image in both Simulator environment and `multinode-platform.json`.
  The old runtime image is retained as `zpr-multinode:before-node-renewal-20261015`;
  the original October 12 alias was restored after deployment.
- Live regional acceptance completed at **15:12:39 UTC: 106/106 steps**,
  **42/42 employee/service pairs**, including six allowed business transactions
  and 36 denials with fresh matching Visa evidence. All 28 cleanup steps passed;
  no machine containers remain running. HQ/Shenzhen Business acceptance and
  sustained operation past the normal authentication lifetime remain unverified.
- Full Rust workspace `cargo test` passes (including 497 Visa tests); focused Go
  race tests pass. Required dashboard `make test` retains exactly the four known
  failing groups. Formatting, whitespace and editor diagnostics pass.
  Source changes are not committed or pushed.
- Session evidence: `files/scenario-failure-20261015-1456.json`,
  `files/regional-after-renewal.json`, `files/regional-deployed-acceptance.json`,
  `files/regional-runtime-deploy.txt`, `files/regional-cargo-workspace.txt`,
  `files/regional-go-make-test.txt`.

## Finance Gateway activation repair (2026-10-15 UTC)

- Preserved the failed Finance scenario: DNS resolved, but the Gateway health
  request received a Visa `NoMatch` denial. The client's live trusted claims
  were correctly Finance/Milwaukee; the trusted LDAP provider was healthy.
  The Gateway had connected before effective policy installation and was
  absent from Visa Admin's registered service inventory.
- Activation now reconnects the configured Gateway after effective policy
  installation and requires its service registration before publishing DNS.
  Failed registration fails activation rather than reporting readiness.
- Reconnecting changes the Visa address. This exposed a coupled DNS restart
  defect: the launcher replaced its base zone while retaining the old dynamic
  journal, causing BIND `journal rollforward failed: not exact` and SERVFAIL.
  The launcher now discards the previous journal after stopping BIND and before
  replacing that zone.
- These launcher scripts are read-only host bind mounts in the running
  Simulator; changes are live without replacing its executable. Exercised the
  new helper both from the host and inside Simulator, republished through the
  normal scenario DNS startup, and completed the Finance scenario twice:
  all 12 steps pass, health is HTTP 200, Google HTTPS is HTTP 200, Apple HTTP
  is denied with HTTP 403, and workload/logout/machine cleanup completes.
  No machine containers remain running. No Restore was initiated and no
  Policy-Service, Control-Service or Control Room executable was replaced.
- Gateway lifecycle and DNS journal regression tests pass with the race
  detector; shell syntax and editor checks pass. Required `make test` retains
  exactly the same four known failing groups. Both Business matrices remain
  pending. Source changes are not yet committed or pushed.
- Session evidence: `files/finance-gateway-failure-latest.json`,
  `files/finance-gateway-after-lifecycle-fix.json`,
  `files/finance-gateway-deployed-context-acceptance.json` and
  `files/finance-gateway-make-test.txt`.

## Restore progress modal (2026-10-15 02:07 UTC)

- Restore now displays a native blocking dialog with its current phase,
  rollback progress and log link. Background controls are inert; Escape does
  not dismiss progress. Completion/failure closes the dialog and leaves the
  existing result above the catalog. Activation retains its inline status.
- Six focused desktop/tablet regressions pass, including real blocked background
  clicks, Escape prevention, rollback updates, completion/failure closure and
  log retention. Editor diagnostics and whitespace checks pass.
- Waited for the user's running Restore to finish before deployment. Its API
  reported `completed` / `Organization ready`; retained that response in session
  `files/restore-modal/restore-feedback-before.json`. This establishes successful
  Restore after the image-retention fix, not business-matrix acceptance.
- Deployed only Simulator as `zpr-simulator:20261015-restore-modal` at
  `2026-10-15T02:07:56.830683054Z`, image
  `sha256:884ab799270305ae26c47fe98858a2af42cbb653ca2a827695bb823c1822de7f`.
  Retained `zpr-simulator-before-restore-modal-20261015`, preserved environment/
  mounts/ports, restored both hosted DNS relays and checked statistics response.
  Served JavaScript contains the modal and HTML loads `organizations.js?v=27`.
  Restart clears in-memory completion to idle; no Restore was initiated here.
  No production services or controller prebuilds were replaced for this UI change.

## Assertion seed repair (2026-10-15 01:53 UTC)

Follow-up, 02:03 UTC: the user's next Restore exposed an incomplete image-retention
fix. `restore_organization_base` removes Policy-Service in a separate command
before context restart, so inspecting only a still-present container could not
retain its image. Verification again saw an empty revision-1 source from the old
default image; runtime rollback completed successfully. The initial live seed
evaluation below was valid but did not certify this lifecycle.

The launcher now atomically saves the image reference before container removal
and reads it on context startup when no container exists. Explicit image overrides
still take precedence. Added actual launcher-function tests covering separate
stop, repeated stop, context restart and overrides. Focused race checks pass.
Redeployed only Policy-Service at 02:03:31 UTC, then exercised real separate
`stop-policy-service` / `restart-policy-context` commands from the Simulator
container. At `2026-10-15T02:03:44.818513173Z` the service retained
`zpr-simulator:20261015-assertion-seed`; all 28 live LDAP assertions pass again.
No additional Restore or organization reset was initiated. Retained the prior
container as `zpr-policy-before-image-retention-20261015` and the failed reset log
in session artifacts. Full end-to-end Restore acceptance remains pending.

- The failed Restore recreated Policy-Service from `zpr-simulator:local`,
  built on 2026-10-13. Its saved Great Lakes assertion record remained empty.
  Current source also silently discovered defaults through Simulator profiles;
  replaced that production dependency with explicit
  `ZPR_POLICY_ASSERTIONS_SOURCE_FILE`. Provisioning exports the seed atomically,
  without passing Simulator environment variables to Policy-Service.
- Context restarts now retain the deployed Policy-Service image; explicit
  `ZPR_POLICY_SERVICE_IMAGE` overrides it. This prevents Restore/rollback from
  silently selecting the older default image again.
- Deployed only Policy-Service from `zpr-simulator:20261015-assertion-seed`
  at `2026-10-15T01:53:56.369188721Z`, image
  `sha256:37d0f73ca071bb3199ffdadcb9d618f30c95b0db147c473b2fb24bb7d4639f92`.
  Preserved existing mounts, ports, production configuration and restart policy;
  retained stopped `zpr-policy-before-assertion-seed-20261015` plus database
  backup in the session artifacts. Other service containers were not restarted.
- Live mTLS settings now return Great Lakes, matching base DN, revision 2 and
  2,337 source characters. LDAP reports twelve people/eight groups, and all
  28 assertions evaluate `pass` through Control-Service. Existing nonempty
  operator assertions are preserved by regression-tested seeding semantics.
- Focused race tests pass for seed inputs, empty-record repair, operator-rule
  retention, bundled directory consistency, service transport and launcher image/
  seed wiring. Invalid/missing/oversize/empty explicit files fail. Tests use
  unavailable Simulator configuration. Vet and shell syntax pass; full `make
  test` retains exactly the four documented baseline failures.
- No Restore retry, directory/policy reset, controller rollout or live Business
  scenario was performed. The assertion blocker is repaired; complete
  reprovisioning and both access-matrix runs remain pending reviewed execution.

## Restore feedback deployment (2026-10-15 01:03 UTC)

- Investigated the user's Restore without starting another operation. It failed
  during `Verifying policy, assertions, LDAP, DNS, and logs`, reporting
  `Policy assertion settings do not match organization great-lakes`.
  Automatic rollback finished with `previous runtime restored and verified`.
  A read-only mTLS settings probe after rollback returned Great Lakes and the
  correct base DN but zero assertion-source characters at revision 1. No
  assertion-settings repair or Restore retry was performed.
- Fixed the missing active-organization progress surface: operation phase and
  rollback progress now appear above the catalog for any selected card, followed
  by a visible completed/failed result. Added immediate submitting feedback and
  log access during execution, not just after failure. Logs remain local;
  Simulator's operation state remains in memory and resets on restart.
- Six focused desktop/tablet browser checks pass for running/failure/completion
  feedback and retained log access. A broader initial selection also exposed
  existing confirmation-test File-menu visibility assumptions and a removed
  `organization-connection` selector; these unrelated old tests were not changed.
- After confirming rollback finished and no machines were running, deployed
  `zpr-simulator:20261015-restore-feedback`, image
  `sha256:31705057ef603f84694c1b304e87af3e7afdb1c702fe54f03caec2d7a433e448`,
  at `2026-10-15T01:03:15.558744674Z`. Only Simulator was recreated; restored
  both hosted DNS relays and verified statistics responsiveness. Retained prior
  container as `zpr-simulator-before-restore-feedback-20261015`.
- Live browser loads `organizations.js?v=26` and the new accessible status
  element; backend is connected and Great Lakes remains active. Restart cleared
  the completed failure from the API to idle; exact failure evidence and
  pre-deployment state are retained in session `files/restore-failure-final.json`
  and `files/restore-feedback-before.json`. The reset log remains unchanged.
  No organization reset, service scenario or commit/push was initiated.

## Great Lakes business-service deployment (2026-10-15 00:56 UTC)

- User authorized deployment after source-only validation. Built the current
  dashboard worktree as `zpr-simulator:20261015-business-services`, image
  `sha256:0e716fce3a9828a201cc3c70a442ac896c5b90df5373d400655c9fe25d80282a`.
  This is not an isolated patch release; it retains the current worktree's
  Results and other existing source changes.
- Recreated only `zpr-simulator`, preserving its environment, bind mounts,
  published ports, labels and bridge network. Started at
  `2026-10-15T00:56:10.540347213Z`. Kept the stopped prior container as
  `zpr-simulator-before-business-20261015` for rollback.
- Installed Linux arm64/amd64 controller and proxy prebuilds and refreshed the
  current Great Lakes generation's cached controller. No machines were running.
  Arm64 controller SHA-256:
  `e1e3a310fb8f9164de6b3a1a81d65739230ca52f63200d76c82deb9d54982218`;
  amd64:
  `185b7cfcdf2277447ed9dc74fb250220c4f943e3f4da6337b59053cf0fd284a6`.
  Prior binaries and restricted container configuration are retained in this
  session's `files/business-before-*` and `files/business-simulator-before.json`.
- Restored the two Simulator-hosted DNS relays and their PID files without
  recreating DNS or its adapter. Statistics relay responds successfully.
  Control Room, Control-Service, Visa Service, DNS and Internet Gateway image
  IDs and start times remain unchanged.
- Live scenario/status APIs respond successfully; Great Lakes exposes thirteen
  components and both new Business scenarios. Browser verification shows both
  scenario cards, a connected backend, and no manifest-unavailable error.
  This resolves the preceding release's observed manifest 503 with the new
  executable; no manifest or saved organization configuration was edited.
- Source validation: focused race tests, runtime policy and all five catalog
  ZPLC checks, vet, macOS/Linux arm64/Linux amd64 builds, formatting and shell/JS
  syntax passed. Required full `make test` still has the same four known
  baseline failure groups; no full-suite pass is claimed.
- No organization Restore, network reprovision, directory/policy reset, business
  service launch or live access-matrix execution was performed. Great Lakes
  still needs Restore/reprovision to seed its new provider identities and
  directory host, followed by both Business scenarios for forwarding acceptance.
  No commit or push was made for this deployment.

## Scenario gateway Results deployment (2026-10-15 00:49 UTC)

- User selected only Results changes on the previous release source and
  explicitly approved proceeding despite the pre-existing Simulator
  `503 {"error":"simulation manifest unavailable"}`. Preserved the configured
  manifest and `SIMULATION_ORGANIZATION_ID=velocity`; did not repair or switch
  organizations. No network scenario was started.
- Built an isolated `e1b24ab` snapshot with only the HTTP observation contract,
  gateway client capture/export, scenario result storage, Results UI/styles,
  optional-expectation help, focused tests and related documentation. Excluded
  concurrent business-service, assistant, log and production-service changes.
  Source, reviewed patches, binaries, exact prior binaries, container-state
  comparisons and test logs are retained in
  `../../../.copilot/session-state/c597bffb-ead1-4da7-bae2-8b7f15caf752/files/scenario-results-release/`.
- Linux arm64 executable SHA-256:
  `96101948a8a3a8436623ec4b465b3c13e5ebd71765c43fca71dcb00eeb58339a`.
  Linux amd64 controller SHA-256:
  `120f3bd2a2053a025942920648f7fd439a2aa581395c97f8152c1b52ac71f3eb`.
  Installed arm64 into the existing Simulator writable layer and both controller
  prebuild paths under `.local-runtime/dashboard-stack`. Also updated the
  installed controller for the current persisted runtime request,
  `machine-runtimes/great-lakes/28882a1946384a40e1bdf0c7e99e864e/controller`.
  Historical inactive generations were not rewritten. No machine containers
  were running; future generations use the updated prebuilt binaries.
- Restarted only `zpr-simulator` at `2026-10-15T00:49:59.14940675Z`.
  Its container ID and original image ID are unchanged. Restored its two socat
  relays with the original quoted `SYSTEM:"docker exec ..."` arguments.
  Updated their existing PID files to container PIDs 68 and 74.
  Verified statistics relay HTTP 200 and a framed DNS-over-TCP response through
  the records relay (the diagnostic query was refused by upstream policy, not
  a claim of successful DNS resolution). Production container IDs, image IDs
  and start times match the captured pre-deployment baseline.
- Verified running executable/controller hashes and executable file modes.
  Served HTML, JS and new CSS match the isolated release byte-for-byte.
  Live browser loads the new renderer, but the existing manifest error still
  prevents the scenario catalog/run API from working. This is an installed
  release, not live end-to-end gateway acceptance.
- Isolated focused Go race checks, vet, formatting, both Linux builds and six
  desktop/tablet browser checks pass. Required `make test` has the same 13
  failing groups on both the clean base and isolated release: four previously
  recorded failures plus nine external-runtime/seed-dependent failures in this
  standalone snapshot. No full-suite pass is claimed.
- Rollback: exact prior Simulator executable is retained both in the release
  backups and `/usr/local/bin/zpr-web-dashboard-before-results-20261015` inside
  Simulator (SHA-256
  `3449abfc490d535bab303482df40caa26e87dd68e558833c297d28c2ae50f21c`).
  Restore it to `/usr/local/bin/zpr-web-dashboard`, restart only Simulator and
  restore both recorded relays. Restore `backups/controller-prebuilt-arm64`,
  `backups/controller-prebuilt-amd64` and
  `backups/controller-installed-great-lakes-28882a1946384a40e1bdf0c7e99e864e`
  to their corresponding paths with mode 0700. Old arm64 controller SHA-256:
  `426263b15ef8c34eebf727d37626326dfd6f86db4885e6d3b1ff46e04b837a4a`;
  old amd64:
  `10c2c69fb2f9df7bf06e30e44a192d1048789a74ebf8686d4f900607382ddcf1`.
  The architecture marker was backed up but not changed. As with the preceding
  releases, recreating Simulator from its original image loses the writable-layer
  executable; retain these release artifacts for reinstallation. No commit/push.

## Gateway cloud metadata recovery (2026-10-15 00:16 UTC)

- The live Admin API reported `internet-gateway.svc.zpr` on the
  `internet-gateway` actor as `Regular`. Control-Service had
  `ZPR_PLATFORM_SERVICES=[]`, so the existing map renderer could not classify
  the gateway or draw its external-network cloud. This was missing operator
  configuration after removal of the old Simulator-manifest fallback, not a
  gateway availability failure.
- Added persistent operator-owned `dashboard-stack/platform-services.json`
  configuration to the local launcher, with `ZPR_PLATFORM_SERVICES_FILE` as
  an alternate path and explicit `ZPR_PLATFORM_SERVICES` taking precedence.
  Validate a single JSON array and required descriptor identities before
  stopping Control-Service. No Simulator-manifest/profile fallback was added.
- With user approval, recreated only `zpr-control-service` at
  `2026-10-15T00:16:24.497264762Z`, retaining image
  `sha256:359cdf6d682ca5ad65fba92b3db83f25b7845f2631abcf2ca0beb613ab493abd`
  and its existing operator connection settings/network attachments.
  Installed the persistent annotation for `internet-gateway.svc.zpr`,
  actor `internet-gateway`, kind `Gateway`, external network `public-internet`.
  No binary rebuild, unrelated local extraction deployment, commit or push.
- Verified the production snapshot over existing mTLS: connected, no errors,
  gateway classified correctly, and live address/endpoints/actor retained.
  Focused Go race tests, vet, shell syntax and diff checks pass. Added and ran
  a real-Dex operator browser regression: desktop and tablet both verify the
  labeled cloud and connecting line in Topology and World Map (2/2 pass).
  The new browser test explicitly submits the current sign-in form and uses
  the current `User: ...` status; pre-existing login tests were not modified.
- Simulator (`22:35:44 UTC`), Control Room (`22:43:45 UTC`) and the persistent
  gateway (`2026-10-13 16:33:17 UTC`) were not restarted. To remove the
  annotations, set `ZPR_PLATFORM_SERVICES=[]` when recreating Control-Service,
  or remove the persistent file and recreate without an explicit override.

## Live scenario failure and launcher fix (2026-10-14 23:55 UTC)

- The `great-lakes-finance-web-gateway` run failed on step 2 of 10 while waiting
  for `machine-03`'s controller. The Simulator reported
  `machine-03 controller did not connect: context deadline exceeded`; scenario
  cleanup steps completed. The machine container exited with code 126 and its
  final output was `/bin/sh: 1: exec: /usr/local/bin/zpr-machine-controller:
  Permission denied`.
- Root cause was in `scripts/dashboard-stack.sh`: it used `-x` alone to decide
  whether `machine-workloads/controller` was an installed executable. The
  controller path was an empty directory; directories can pass `-x`, so the
  launcher skipped the copy and bind-mounted that directory at the executable
  path. The prebuilt Linux arm64 and amd64 controller binaries are present.
- Fixed the launcher to install through a temporary regular file with mode 0700,
  replacing only an empty directory (and refusing symlinks, nonempty directories
  and special files). Added a shell-function regression covering absent, empty
  directory, non-executable file, already valid and nonempty directory cases.
  `bash -n`, the new Go test, the related idle-refresh test, `go vet` and
  `git diff --check` pass.
- The script source is on a directory mounted into the running Simulator, so
  the next machine start read the corrected installer. With the user's approval,
  retried the same published scenario at 2026-10-15 00:07 UTC. Controller wait,
  login, workload selection/start, DNS resolution and test-service request all
  completed; workload stop, logout and machine stop cleanup also completed. The
  run ended `completed` at step 10/10 with no error. No Simulator/Control-Service/
  Policy-Service restart, policy mutation or organization change occurred. The
  machine container is stopped after scenario cleanup; no labeled machine
  containers remain running. This is a focused end-to-end scenario recovery,
  not a full workday or broader fault-cycle acceptance.

## Deployment status

### Control Room GUI worktree executable (2026-10-14 22:43 UTC)

- The user selected Control Room only and approved deploying the current
  dashboard worktree, including its other pending dashboard changes. Built
  `zpr-editor-ui:20261014-control-room-gui`, image ID
  `sha256:200b9484e24aa6fe90035bf4ab5b713186da9e295d6a182020ad1d4f8665420c`
  (`linux/arm64`). The embedded dashboard executable SHA-256 is
  `3449abfc490d535bab303482df40caa26e87dd68e558833c297d28c2ae50f21c`.
- Installed that executable into the existing `zpr-control-room` container's
  writable layer and restarted only Control Room. Container ID
  `2487b772996217b8964e53c63e1ef402b49f9cd9c0a0d0fa101c3ae71aa45567` and
  its original image ID `sha256:359cdf6d682ca5ad65fba92b3db83f25b7845f2631abcf2ca0beb613ab493abd`
  are unchanged; it started at `2026-10-14T22:43:45.087558591Z`. Existing
  mounts, ports, certificates and operator settings were preserved. The exact
  prior executable, SHA-256
  `bdcdc1255dc81de1fed4cde4c47ed9f9e30aa1488ef1065fe615512f6bc128cb`, is
  retained at `/usr/local/bin/zpr-web-dashboard-before-gui-20261014` and in
  the session release artifact directory.
- HTTPS `/` and `/auth/operator/config` return 200; operator login remains
  enabled. The unauthenticated snapshot endpoint returns the expected 403.
  Restart reset in-memory operator sessions, so operators must sign in again.
- All 76 directly served static assets match the dashboard worktree byte for
  byte; `/` returns 200 and `/index.html` retains its 301 redirect. The
  signed-out browser displays the operator sign-in prompt; no authenticated
  live-data or post-login browser test was performed.
- Control-Service, Simulator, Policy-Service, DNS, gateways and runtime
  containers were not restarted or replaced. The Simulator remains the
  separately deployed 2026-10-14 GUI worktree release. No policy/provisioning
  mutation was performed. No commit or push.
- Rollback in this container: copy
  `/usr/local/bin/zpr-web-dashboard-before-gui-20261014` back over
  `/usr/local/bin/zpr-web-dashboard` and restart only `zpr-control-room`.
  The previous immutable image also remains available.

### Simulator GUI worktree executable (2026-10-14 22:35 UTC)

- The user selected Simulator only and approved deploying the current dashboard
  worktree, including its other pending dashboard edits. Built
  `zpr-simulator:20261014-gui-work`, image ID
  `sha256:6b6f7442ea71802352f1c1f3806f53ac9153f2d3de8b4198544e94297efca63a`
  (`linux/arm64`). The embedded dashboard executable SHA-256 is
  `3449abfc490d535bab303482df40caa26e87dd68e558833c297d28c2ae50f21c`.
- Installed that executable into the existing `zpr-simulator` container's
  writable layer and restarted only that container. Container ID
  `8c7bbd7cc4ef4fc3ff014b4487763869d959be08237717f79ba341cbe011dcab` and
  its original image ID `sha256:846314c15587aab6be4426bbea3d6f744834c1570a7155a392926f39c10b8894`
  are unchanged; it started at `2026-10-14T22:35:44.089766049Z`. Container
  configuration and mounts were not changed. The exact prior executable,
  SHA-256 `739c44d113512ff544095b2c394c8740439d0fff8215d362ae9dcf410fd0b90d`,
  is retained at `/usr/local/bin/zpr-web-dashboard-before-gui-20261014` and
  in the session release artifact directory.
- The Great Lakes five-minute workday had completed before restart. Great Lakes
  remains active and the Simulator run state is idle afterward; in-memory run
  history reset. No scenario, activation or policy write was performed.
- Restart stopped the two Simulator-local DNS relay processes. Restored the
  same 8054 statistics and 8055 DNS-record relay commands to `great-lakes-node0`;
  the statistics endpoint responds and the records relay listens. BIND, the
  DNS adapter, the separate `zpr-dns-stats-relay`, all Great Lakes runtime
  containers, Control Room, Control-Service and Policy-Service were not
  restarted or replaced.
- All 76 directly served static assets match the dashboard worktree byte for
  byte. `/index.html` still redirects with 301 to `/`; `/agents.html` still
  redirects with 302 to `/machine-logs.html`. Live browser checks confirmed
  Organizations connected to Great Lakes, New/Import/Duplicate dialog mode
  switching, draft-only Rename/Delete availability, and Finance/Runtime/
  Workday scenario grouping. Trusted Sources displayed People/Groups/
  Attributes counts (19/8/12). Activity displayed Recent visas and Recent
  denials without the removed timestamp/stream/request/approval metrics; no
  page alerts were present. Scenario Help displayed the 12-machine limit.
  Focused organization/archive/permission and operator-auth Go tests pass.
- Rollback in this container: copy
  `/usr/local/bin/zpr-web-dashboard-before-gui-20261014` back over
  `/usr/local/bin/zpr-web-dashboard` and restart only `zpr-simulator`, then
  restore the same two relay commands if they are not running. The prior
  immutable image also remains available. Control Room GUI changes were not
  deployed. No commit or push.

### Visa Service-only visa-pull rollout (2026-10-14 21:43 UTC)

- With explicit approval, gracefully stopped only Visa Service PID 51686 and
  relaunched the unchanged `/app/bin/vs >>/logs/vs.log 2>&1` command from `/conf`
  in its existing `vs` tmux session. New PID 25226 started at approximately
  21:43:35 UTC. Shutdown released the instance lock normally; no manual lock
  deletion, Redis reset, identity reset or policy change was performed.
- Release source is clean `zpr-visaservice` commit
  `ed27bca882b1d787420543fa22311948de389a5e` plus only `vs/src/visa_mgr.rs` and
  `vs/src/vsapi_worker.rs`. It excludes concurrent dashboard/discovery edits.
  Dependency source is clean `zpr-common`
  `2756e9a01d5552965439f94314a6b0550832cf8f`, including pinned schema submodules
  `zpr-policy` `d2ad9d8c1ca1007ff17b9364d2734392f44f6575` and `zpr-vsapi`
  `a3401642d8bf6949a9a3afa73650c7d9ef6c8222`. All 496 Visa Service unit tests
  pass again against this clean Linux arm64 release snapshot before deployment.
- New VS executable SHA-256:
  `490e33a1784e4c836e422fe4b0e351a1ce017920507ba981f069de7bd8b9f410`.
  Image `zpr-visa-service:20261014-vss-pull`, ID
  `sha256:cfc7646d43eace2081201f21991631673fea921cc49567de8eb2a23cee681627`,
  extends the prior adapter-only release and replaces only `/app/bin/vs`.
  Its PH hash remains
  `e06906c77e749ef983d388276dd56e8e14a87ba656f6aa4ff9445783cc185c14`.
- No containers were recreated. Visa adapter PID 5356, Redis PID 14 and node
  PIDs 23674/13849/13794 are unchanged; so are VS/adapter configuration hashes
  and raw active-policy hash. Control Room, Control-Service, Policy-Service,
  Simulator and BIND were not replaced or restarted.
- All three original nodes reauthenticated automatically at 21:43:38–39 UTC.
  VSS completed new TCP, TLS and Cap'n Proto connect RPCs for node0/node1 at
  21:43:42 UTC and node2 at 21:43:43 UTC. Previously blocked visas 2642/2643 now
  show Installed at both hops; subsequent VSS visas and reverse-flow visas are
  also Installed along their paths. Node2 briefly received NoRoute while node0
  was still reconnecting; ordinary registration retry recovered automatically.
- Production Control-Service mTLS snapshot at 21:44:30 UTC reports three
  successful node authentications, 30 API pings, zero VSS errors, no snapshot
  errors and all three topology edges UP. Fresh VSS sockets are established to
  each node, rather than relying on the snapshot alone. DNS reads return six
  records and BIND boot time remains `2026-10-14T16:56:32.090Z`.
- **Bounded passive observation passed:** 33 production snapshots from
  21:45:20 to 22:01:46 UTC span 985 seconds (16 minutes 25 seconds), beyond the
  earlier recurrence window. All checks retained connected API, all three fresh
  VSS connections, zero VSS errors, three UP topology edges and the same
  VS/adapter/Redis/node processes. API ping count advanced from 60 to 650.
  At 22:02 UTC there were no new VS WARN/ERROR records after initial
  reconnection, packet counters continued advancing and all four adapter
  fastpaths were polling normally. Policy/config hashes and container IDs/start
  times remained unchanged. No full workday or deliberate failure-injection
  run was performed; this is not certification across expiry/fault cycles.
- The actual running container keeps the new executable in its writable layer.
  For recreation, the persisted Great Lakes `compose-nodes.json` now selects
  `zpr-visa-service:20261014-vss-pull` only for VS; merged Compose image selection
  validates. Retain this override if organization tooling regenerates the file.
  Shared multinode tags and all node images remain unchanged.
- Exact rollback VS executable remains at
  `/app/bin/vs-before-vss-pull-20261014`, SHA-256
  `bdde537495a7855efc85c835de2fd6bcd7e41d61d9528431927ea555584ed672`,
  with a private host copy and pre-rollout Compose override. Rollback is a
  graceful stop of only the current VS PID, restoring that binary and the same
  launch command; restore the previous adapter-only image selection for future
  recreation too. Do not restart nodes, adapter or Redis, or clear state.
- The by-ID defect is independently covered by the RPC regression. Live recovery
  followed a patched-VS restart; this does not isolate the prior node0 local
  handshake defect or explain earlier Redis delays.

### VSS visa-pull recovery correction: source validated, not deployed (2026-10-14)

- Continued diagnosis without restarting Visa Service, nodes, Redis or the
  patched Visa adapter. Temporarily enabled targeted node flow/visa debug logs,
  then restored those targets to INFO. A bounded node packet capture was closed
  and its capture program removed. Header-only TUN observations did not capture
  application payloads.
- At 19:56:16 UTC node0 selected visa 2643 for the VS-to-node1 VSS flow and
  requested its stream from node1. Node1 did not have that visa locally; its
  authenticated VSAPI request for the same ID returned no copy, and it rejected
  the bind. Read-only persisted metadata confirmed visa 2643 was Installed on
  node0 but PendingInstall on node1. Visa 2642 had the equivalent node2 state.
- The Visa Service by-ID handler filtered requests to Installed visas only.
  That creates a circular dependency: the destination needs the pending visa
  to establish VSS, while the VSS channel is expected to push that visa.
  Corrected the pull path to permit PendingInstall as well as Installed for
  the authenticated requesting node, and actualize each returned copy for that
  node. Off-path, unknown, expired, PendingRevoke and Revoked copies remain
  excluded. Reply delivery uses the existing PendingInstall-to-Installed CAS;
  a concurrent revocation cannot be overwritten. Installed-only inventory
  semantics are unchanged. Retrieval/actualization failures are logged and
  returned as explicit API errors instead of an empty successful response.
- Added a real Cap'n Proto handler regression with no VSS channel: a destination
  pulls its pending copy and only its own installation state advances. Manager
  regressions cover all three path roles, unrelated nodes, expiration, revoked
  states and revocation winning an installation acknowledgement.
- Validation: all 496 Visa Service binary unit tests pass on native macOS and
  native Linux arm64 (release). The offline Linux run initially lacked locked
  `linux-raw-sys` 0.12.1; fetched locked dependencies, then reran successfully
  offline. No dependency manifests or lockfiles changed. Changed Rust files
  pass rustfmt, editor diagnostics and diff whitespace checks. Crate-wide
  rustfmt still reports an unrelated existing assertion layout in
  `vs/src/connection_control.rs`; it was not changed.
- **At source-validation completion, not yet deployed.** Visa Service PID 51686
  still ran the previous binary; see the subsequently approved rollout above.
  This proves a multi-hop bootstrap defect, not complete runtime recovery.
  Node0's local VSS flow also still times out; a SYN-RECV socket was observed
  there, but its missing handshake completion has not yet been explained.
  Redis last-seen delays also remain open. The adapter's four fastpaths were
  still polling normally at 20:04 UTC, approximately 16 minutes after rollout;
  that observation is not a full-workday or recovery-convergence certification.

### Visa adapter-only UDP I/O rollout (2026-10-14 19:48 UTC)

- User selected Visa adapter only first. Gracefully stopped only adapter PID
  51769 and relaunched the same `/conf/adapter-vs-conf.toml` through its existing
  `vs-adapter` tmux session; replacement PID 5356 started at approximately
  19:48:14 UTC. No containers were recreated. Visa Service PID 51686, Redis,
  nodes, identities, active policy and configuration were preserved. Dashboard,
  Simulator and Policy-Service releases were not changed.
- Built from clean `zpr-core` commit `27854236725f279a1af8dfa37c168683292a2b30`
  with only `adapter/ph/src/batch_io.rs` overlaid; unrelated in-progress platform
  edits were excluded. Dependency source is clean `zpr-common` commit
  `2756e9a01d5552965439f94314a6b0550832cf8f`. The clean release-source PH suites
  pass 549 test executions with two ignored executions, including the
  missed-cancellation regression and concurrent sparse UDP receive stress.
  The rebuilt baseline was not byte-identical to the prior deployed executable;
  the exact deployed executable was retained for rollback.
- Deployed PH SHA-256:
  `e06906c77e749ef983d388276dd56e8e14a87ba656f6aa4ff9445783cc185c14`.
  Packaged as `zpr-visa-adapter:20261014-udp-dontwait`, image ID
  `sha256:6fac51b62e3df951416106c931bf4e54d203ee0153ce2cc03103e1e36c1461b2`,
  based on the existing arm64 image with only `/app/bin/ph` replaced.
  Shared `zpr-multinode` tags and node binaries were not changed. Visa Service
  executable remains
  `bdde537495a7855efc85c835de2fd6bcd7e41d61d9528431927ea555584ed672`.
- The current container retains the replacement in its writable layer across
  container stop/start. The persisted Great Lakes `compose-nodes.json` adds a
  VS-only image/platform override for future recreation; merged Compose image
  selection validates without starting services. The generated override must be
  retained if organization tooling regenerates that file; do not substitute the
  shared node image or imply the running container itself was recreated.
- All three existing nodes reauthenticated to Visa API at 19:48:16–17 UTC
  without node or Visa Service restarts. The Visa adapter dock became Active,
  its UDP counters resumed advancing and its fastpath workers returned to poll.
  Verified backend mTLS reads return six DNS records and BIND statistics.
- **Partial recovery only:** new reverse VSS TCP connections to all three nodes
  timed out at 19:50:35 UTC and were retried. Old established sockets and the
  dashboard's connected/UP snapshot are not proof of live VSS synchronization.
  No passing workday or sustained-recovery certification is claimed. Leave
  the narrower I/O correction deployed; broader node/VS restarts or rollout
  were not performed. Preserve the remaining failure for follow-up diagnosis.
- Rollback executable remains inside the container at
  `/app/bin/ph-before-udp-dontwait-20261014`, SHA-256
  `c5a9eade4352318b1d63d93a09e624aa299add6796bd9d5985a4eca01cbf7fed`,
  with a private host copy and original Compose override retained in session
  artifacts. To roll back, identify and gracefully stop only the current
  adapter PID, restore that executable and launch the same tmux/config command;
  restore the saved VS image override if rolling back future recreation too.
  Do not restart Visa Service, clear Redis or reset identities/policy.

### Recovery lifecycle investigation and UDP I/O correction (2026-10-14)

- Investigated the preserved failed runtime without restarting Visa Service,
  its adapter, nodes, Redis or dashboard services. No new runtime release was
  deployed. Captured the existing binary hashes, socket/lease state, bounded
  failure logs, thread kernel stacks and `io_uring` completion state.
- The shared Visa adapter packet worker was blocked in `io_uring_enter`,
  requesting 128 completions with only 127 present. The kernel poll list
  retained opcode 10 (`RECVMSG`). The submission/completion indices and UDP
  counters remained unchanged in follow-up observations at 19:09 and 19:23 UTC,
  while the separate key-management task continued handshake retries.
  This demonstrates a stalled packet worker, not merely a slow API dial.
- At 17:08:42 UTC node0 timed out the Visa adapter's dock link and sent a
  termination, followed by address/visa removal and failed API reconnects.
  All three VSS workers failed seconds later. Without the Visa adapter's shared
  packet path, bootstrap and reconnect retries cannot restore control traffic.
  The earlier Redis timeouts remain unexplained; this does not establish them
  as the trigger or prove a separate authentication/bootstrap design defect.
- Source batch I/O submitted UDP receives without `MSG_DONTWAIT` and depended
  on unlinked asynchronous cancellations before waiting for the whole batch.
  `O_NONBLOCK` alone does not prevent an `io_uring` receive from waiting.
  Corrected both receive variants in `zpr-core` to use `MSG_DONTWAIT`, removing
  their dependence on cancellation for progress. Authorization, bootstrap
  eligibility, session handling and reconnect deadlines are unchanged.
- A deterministic fault-injection test uses the production receive builders but
  omits cancellation, modeling a cancellation that failed to find its request.
  Before the change it failed the 30-second watchdog; afterward both variants
  complete with `EAGAIN` on empty sockets. All nine batch-I/O tests pass.
  The full Linux PH library/binary suites pass 555 test executions with two
  ignored executions; all eight native macOS batch-I/O tests pass. Changed-file
  rustfmt, whitespace checks and editor diagnostics pass.
  Sparse UDP arrival stress and separate investigative TUN stress did not
  reproduce the exact live cancellation race; do not confuse that with the
  deterministic dependency test. The kernel/scheduling trigger remains open.
- Acceptance requires sustained packet-worker progress and automatic recovery
  across workload/idle/fault and expiration cycles. A coordinated restart,
  established TCP socket, passing unit suite or single workday is not sufficient.
  At investigation completion, live rollout and sustained-recovery acceptance
  were not yet performed; see the subsequent adapter-only rollout above.

### Great Lakes workday outage and scoped recovery (2026-10-14 16:42–17:01 UTC)

- Deployment of the ninth/tenth dashboard extractions was held after the user
  reported a failed Five-Minute Workday. No new dashboard image was built or
  deployed during this investigation; Control Room/Control-Service retain the
  eight-extraction 16:26 UTC release and Simulator retains its 15:14 UTC image.
- Recorded failed run: `16:42:33.013979508Z`–`16:43:30.813388756Z`.
  The primary failed step was `start_machine` for `machine-05`, reporting
  `ZPR DNS adapter did not become active on great-lakes-node0`; eight other
  launch steps were canceled afterward. The DNS adapter repeatedly received
  Hello responses without an ASA and was terminated. The DNS startup script
  had already removed BIND before waiting for adapter readiness, so DNS was
  absent after this failure.
- Node0 lost its Visa API connection after ping timeouts at 16:33 UTC and
  subsequent reconnects timed out. Visa logs show VSS connection failures and
  expired destination-node authentication as early as 00:01 UTC, predating
  today's dashboard extractions/deployments. This identifies an upstream runtime
  failure path, not proof that every deployment interaction is ruled out.
  The permanent authentication/reconnect/Redis-stall cause remains unresolved.
- User explicitly approved scoped Great Lakes control-plane/DNS recovery and a
  workday rerun, preserving identities, policy and databases. Checkpointed Redis
  with SAVE and kept Redis running. Gracefully stopped only the specific VS,
  VS-adapter and three PH node processes, waited for the existing Visa lease to
  expire, then relaunched their existing binaries/configuration. No clear-state,
  organization activation, key regeneration or database reset was performed.
  The active policy hash is unchanged. Standard scoped DNS startup restored BIND
  and its relays using the existing dedicated keys. New BIND boot time is
  `2026-10-14T16:56:00.183Z`; six record-viewer records and statistics reads pass.
- Published Five-Minute Workday rerun:
  `16:56:27.762193713Z`–`17:00:56.791682671Z`, **completed 157/157**:
  123 run steps and all 34 cleanup steps, with no recorded errors.
  This includes twelve launches, twelve controller waits/logins, three DNS
  resolutions, thirty test-service requests and three traffic checks.
  No scenario-machine containers remain running. All three nodes' API/VSS
  socket pairs remain established after cleanup.
- Follow-up at approximately 17:24 UTC found that recovery was not sustained:
  all three nodes had Visa API sockets in `SYN-SENT`, while their old VSS TCP
  sockets still appeared established. Visa logged terminal VSS ping failures
  for node1/node2 at 17:08:53 UTC; therefore an established TCP socket alone is
  not evidence of a healthy application session. Node0 continued bounded API
  reconnect attempts. No further restart or deployment was performed.
  Redis answered PING, had no blocked clients, and the Visa instance lease still
  had a positive TTL during inspection. These observations do not establish the
  original cause of the Redis timeouts or the failed dataplane reconnection.
- This is a successful recovery/run, not a permanent reliability fix. During
  the successful run Visa still logged Redis lock/last-seen timeouts and node0
  logged one Visa API ping timeout; no terminal workday failure resulted.
  These warnings must not be hidden or treated as certified indefinite health.
- The requested complete Go suite ran without exclusions and retained the same
  four recorded baseline failures. The complete desktop/tablet browser suite
  finished without exclusions (1,304 tests): **1,094 passed, 194 failed,
  10 skipped and 6 did not run**, in approximately one hour. The
  `make test-browser` command exited with status 2.
  No full-suite pass is claimed, and dashboard deployment remains held.
- Browser triage while the suite was still running found fixture startup/login
  failures, old UI selectors/expectations, and editor assertions. For example,
  Diagnostics tests expect `.diagnostics-log-body`, absent from the current
  shared-log-panel implementation; provisioning fixture failures show
  `Not signed in`, not the real deployed-login smoke result. Frontend resources,
  browser tests and Playwright configuration have no diff from published
  checkpoint `ed27bca`. This is evidence for specific fixture/UI mismatches,
  not a claim that all browser failures predate the backend changes. No failing
  pre-existing tests were edited, no failures were excluded and no passing
  baseline browser run is claimed.

### Control Room and Control-Service DNS boundary extractions (2026-10-14 16:26 UTC)

- User selected both Control Room and Control-Service, preserving configuration.
  Built the current worktree as `zpr-editor-ui:20261014-dns-boundaries`, image ID
  `sha256:359cdf6d682ca5ad65fba92b3db83f25b7845f2631abcf2ca0beb613ab493abd`.
  Both executables have SHA-256
  `bdcdc1255dc81de1fed4cde4c47ed9f9e30aa1488ef1065fe615512f6bc128cb`.
  Control-Service started at `2026-10-14T16:26:00.09858484Z`; Control Room
  started at `2026-10-14T16:26:02.019433924Z`.
- Both binaries include all eight backend extractions. The seventh statistics
  proxy and eighth signed-record viewer are now active in Control-Service.
  Simulator still runs the first-three-extraction image; Policy-Service and
  separate gateway processes are unchanged. The first seven extractions are
  committed/pushed as `ed27bca`; the eighth and this release documentation
  remain uncommitted and unpushed.
- Config and HostConfig match each predecessor exactly except for image.
  Network membership, IPv4/IPv6 addresses and aliases are preserved: one network
  for Control Room and four for Control-Service. All other 19 persistent running
  containers retain their IDs, start times and running state, including
  Simulator, Policy-Service, OIDC, BIND, independent DNS relay and runtime machines.
- Verified Control Room HTTPS against its configured CA, client-certificate
  backend reads, and unchanged exact unauthenticated denial responses.
  Live DNS statistics status, server counters, zones and stylesheet reads pass.
  All six DNS records match the pre-deployment JSON; BIND's boot time remains
  `2026-10-14T14:05:37.750Z` and statistics version remains 1.8.
  No DNS, relay or organization configuration was changed.
- Real browser OIDC sign-in returned 200 for snapshot, policy context/status,
  gateway contracts, signed enrollment catalog, DNS records, statistics
  status/server/zones and stylesheet. Unknown API routes and a validation POST
  without CSRF proof return 403; logout restores snapshot 403.
  The missing-CSRF request is denied before forwarding. No policy/provisioning
  mutation, activation or scenario run was performed by verification.
- All 77 directly served non-index static files match source byte-for-byte;
  the root response also matches the embedded index (78 byte comparisons total).
  `/index.html` still redirects 301 to `./` and `/agents.html` is directly
  served with 200. No frontend implementation changes or new full browser
  fixture run are claimed; the real-login smoke test is separate evidence.
  Local signed-transfer/race/dependency tests, vet and native/Linux builds pass.
  The same four full-Go-suite baseline failures remain; all other tests pass
  with those four explicitly excluded. Simulator-unavailable evidence is from
  local tests; the live Simulator was not paused during this rollout.
- Stopped rollback containers:
  - `zpr-control-room-before-dns-boundaries-20261014`, previous image
    `sha256:6c94a5798f33f080a869d53a626a7b41df4f9b749163f6f3b41db1ba483fe67c`.
  - `zpr-control-service-before-dns-boundaries-20261014`, previous image
    `sha256:66bd2883b5da7dd7d175529a80448d19ad4f19bafbe4dc66e2b7da4a58f1cb8a`.
  Their network endpoints were disconnected to preserve the addresses on the
  replacements. Rollback must remove/disconnect the corresponding replacement,
  restore its original name, reconnect the saved networks/aliases and start it;
  do not simply start a disconnected rollback container. Original addresses:
  Control Room bridge `172.17.0.7`; Control-Service bridge `172.17.0.6`,
  `zpr-diagnostics-read-only` `172.20.0.3`, `zpr-dns-stats-read-only`
  `172.21.0.3`, and `zpr-great-lakes` `172.31.0.4`.
  A nonsensitive endpoint recipe is retained with the session deployment result.
  Operator sessions reset on replacement; sign in again.
- Configuration/credentials remained in memory. No environment dump, private
  keys, browser traces or screenshots were saved. Temporary helper removed;
  only nonsensitive verification and rollback-network metadata retained.

### Control Room authorization and local-request extraction (2026-10-14 15:57 UTC)

- User selected Control Room only, preserving configuration. Built and deployed
  the current worktree as `zpr-editor-ui:20261014-authorization-local-request`,
  image ID
  `sha256:6c94a5798f33f080a869d53a626a7b41df4f9b749163f6f3b41db1ba483fe67c`.
  Executable SHA-256:
  `621b6ba9bc59558b9ec64f0f4501aa95061fbb4ace17624a30da00db5497a7fa`.
  Container started at `2026-10-14T15:57:18.719536584Z`.
- Config and HostConfig match the previous Control Room exactly except for image.
  All 20 other persistent running containers retain their IDs/start times.
  Simulator, Control-Service, Policy-Service, separate gateways, OIDC, DNS and
  runtime machines were not replaced or restarted. Great Lakes remains idle.
  Control Room's binary now includes all six extractions; this does not update
  the other processes that use the shared executable.
- Verified HTTPS against the configured CA, operator configuration readiness,
  live client-certificate access to Control-Service snapshot, and unchanged
  unauthenticated API denial statuses/bodies. Real browser OIDC login returned
  200 for snapshot, policy context, policy status, gateway contracts and the
  signed enrollment catalog. Unmapped routes and requests missing mutation CSRF
  proof return 403; logout restores snapshot 403. The CSRF check used Analyze's
  validation endpoint and was denied before forwarding; no policy or
  provisioning mutation, scenario execution or activation was performed.
- All 77 directly served static files match source byte-for-byte. Existing
  `/index.html` -> `./` (301) and directly served `/agents.html` (200) behavior
  is preserved. No frontend source changes were part of these two increments.
- Focused race tests pass for local-request, TLS, authorization, delegation,
  Control Room and affected application contracts. No new full browser fixture
  suite is claimed for this backend-only release; the 62-case run from 15:40 UTC
  remains historical evidence. The same four full-Go-suite baseline failures
  remain documented in [migration evidence](REDIRECTORY.md).
  Simulator-unavailable checks are local test evidence; the live Simulator was
  not stopped for this Control Room-only deployment.
- Rollback retains stopped container
  `zpr-control-room-before-auth-local-request-20261014`, old image
  `sha256:b2c62308411451307fe7153b7c40c55a17d19f5fc1485d9007808ab8e37b6b7c`.
  Restore only that container if reverting. In-memory operator sessions reset
  on replacement; sign in again.
- Container configuration and login credentials stayed in memory; no secrets,
  browser trace or screenshots were saved. Temporary deployment/login helpers
  removed. Source and deployment documentation remain uncommitted and unpushed.

### Control Room current worktree and proxy extraction (2026-10-14 15:40 UTC)

- User explicitly selected Control Room only, deploying the current worktree.
  Built and deployed `zpr-editor-ui:20261014-controlroom-package`, image ID
  `sha256:b2c62308411451307fe7153b7c40c55a17d19f5fc1485d9007808ab8e37b6b7c`.
  Executable SHA-256:
  `86e92e64e2ab5eae233ea24a59f996f52b4531ad2ad0abd95317c8b7d3570c3c`.
  Final container started at `2026-10-14T15:40:21.231455752Z`.
- Config and HostConfig match the old Control Room exactly except for image:
  mode, ports, mounts, certificates, operator settings and restart policy are
  preserved. All 20 other persistent running containers retain their IDs and
  start times, including Simulator, Control-Service, Policy-Service, local OIDC,
  DNS and runtime machines. Great Lakes remains selected and idle.
- The full-worktree binary includes all four backend extractions; only Control
  Room is running this new image. Simulator still contains the first three;
  separate gateway and backend-service processes were not updated. Current
  shared editor assets are now served by Control Room as well as Simulator.
- Verified HTTPS with the configured CA, operator-configuration readiness,
  live Control Room client-certificate access to Control-Service snapshot,
  and unauthenticated API denial. A real browser OIDC login then returned HTTP
  200 from snapshot, policy context, policy status, gateway contracts and the
  signed operator-enrollment catalog. Logout restored snapshot HTTP 403.
  These checks were read-only; no policy save, provisioning mutation, activation,
  scenario execution or backend restart was performed.
- All 77 directly served static files match source byte-for-byte. The existing
  `/index.html` -> `./` (301) redirect is preserved. Unlike Simulator, Control
  Room serves `/agents.html` directly (200); that baseline behavior is preserved.
  An initial check incorrectly expected Simulator's redirect and automatically
  restored the old container. The corrected Control Room baseline check passed
  on the final replacement; other services were unchanged throughout.
- Focused Control Room race/mTLS/delegation regressions pass. All 62 selected
  desktop/tablet shared-editor and organization fixture regressions pass; the
  known definitions-history selector case remains excluded in both viewports.
  The live-login helper was updated to wait for the actual Sign out control,
  not the older "Signed in" status text. No full-suite pass is claimed: the four
  known Go baseline failures remain in [migration evidence](REDIRECTORY.md).
  Simulator-unavailable coverage is local test evidence; the live Simulator
  was not paused during this Control Room-only release.
- Rollback retains stopped container
  `zpr-control-room-before-controlroom-package-20261014`, old image
  `sha256:eeca92b7a7884cecbd7c4cf5cee1adf98012409629c255da2264c290214a9a19`.
  Restore only that container if reverting; do not restart Simulator or services.
  Restarting Control Room resets its in-memory operator sessions; sign in again.
- Replacement configuration and login credentials remained in memory; no
  credential/environment payload, browser trace or screenshot was saved.
  Temporary deployment/login helpers removed. Source and documentation remain
  uncommitted and unpushed.

### Simulator service TLS package extraction (2026-10-14 15:14 UTC)

- User selected Simulator only, preserving configuration. Built and deployed
  `zpr-simulator:20261014-servicehttp-package` from the current worktree, image ID
  `sha256:846314c15587aab6be4426bbea3d6f744834c1570a7155a392926f39c10b8894`.
  Executable SHA-256:
  `739c44d113512ff544095b2c394c8740439d0fff8215d362ae9dcf410fd0b90d`.
  Container started at `2026-10-14T15:14:21.423808628Z`.
- Config and HostConfig match the previous Simulator exactly except for image.
  All 20 other persistent running containers retain their IDs/start times.
  The temporary deployment helper was also checked and then exited; it is not
  included in that count. Control Room, Control-Service, Policy-Service, the
  separate browser gateway, runtime machines, BIND and the independent operator
  DNS relay were not updated.
- Great Lakes remains selected and idle. Both exact DNS relay commands and PID
  files were restored; tracked processes are alive and 8054/8055 listen.
  DNS statistics version remains 1.8 and BIND boot time remains
  `2026-10-14T14:05:37.750Z`.
- All 76 directly served static files match source byte-for-byte. Existing
  `/index.html` -> `./` (301) and `/agents.html` -> `/machine-logs.html` (302)
  redirects remain unchanged. Simulator API readiness was checked again after
  the deployment helper exited.
- Focused service TLS race regressions pass, including certificate accept/reject,
  production proxy and Simulator-client integration coverage. This is not a live
  production mTLS rollout or full-suite certification: the four known full-suite
  baseline failures remain documented in [migration evidence](REDIRECTORY.md).
  No new browser suite, scenario execution, activation or policy write was
  performed for this backend-only release.
- Rollback retains stopped container
  `zpr-simulator-before-servicehttp-package-20261014`, old image
  `sha256:88340bc45fb0346ddb22006093b1113a994dce870897e0aa2aa112057ea56920`.
  Restore only that container and the same relay commands/PID files if reverting.
- Replacement configuration stayed in memory; no environment/credential payload
  was saved. Temporary helper removed. Source and deployment documentation
  remain uncommitted and unpushed.

### Simulator browser-access package extraction (2026-10-14 14:51 UTC)

- User explicitly selected Simulator only, not the separate browser-access
  gateway process. Built and deployed
  `zpr-simulator:20261014-browsergateway-package`, image ID
  `sha256:88340bc45fb0346ddb22006093b1113a994dce870897e0aa2aa112057ea56920`.
  Executable SHA-256:
  `5da600c512e715dc048b30f155d39a101c6eb5b26d6c7a7f2bb53527341d0b77`.
  Container started at `2026-10-14T14:51:32.839106716Z`.
- Config and HostConfig match the previous Simulator exactly except for image.
  All 21 other running containers retain their IDs/start times. Control Room,
  Control-Service, Policy-Service, runtime machines, BIND and the independent
  operator DNS relay were not replaced. A separately running browser gateway
  is not updated by embedding its extracted implementation in Simulator's binary.
- Great Lakes remains selected and idle. Restored both exact prior DNS relay
  commands and PID files; 8054/8055 listen and tracked processes are alive.
  DNS statistics version remains 1.8; BIND boot time remains
  `2026-10-14T14:05:37.750Z`.
- All 76 directly served static files match source byte-for-byte; existing
  `/index.html` -> `./` (301) and `/agents.html` -> `/machine-logs.html` (302)
  redirects are unchanged. No static source changes occurred in this increment.
- Focused gateway/browser gateway race regressions pass after rollout, including
  real-mTLS integration, routing with Simulator unavailable, dependency checks
  and the first extraction's forwarding coverage. Deployed binary smoke check
  confirms browser-gateway mode rejects missing TLS configuration before
  opening a listener. No live browser-gateway replacement, production
  forwarding, scenario execution, activation, or policy write was performed.
  No new desktop/tablet browser suite run is claimed for this backend-only release.
- Rollback retains stopped container
  `zpr-simulator-before-browsergateway-package-20261014`, old image
  `sha256:597e94fd54b1f1672c9346273c4d5dc7dcd438e16989be11a9242b8e79ad3c8e`.
  Restore only that container and the same two relay commands/PID files if
  reverting; do not restart other services.
- Replacement configuration remained in memory; no environment/credential
  payload was saved. Temporary replacement helper removed. Source and
  deployment documentation remain uncommitted and unpushed.

### Simulator first backend package extraction (2026-10-14 14:38 UTC)

- User selected Simulator-only deployment of the entire current worktree,
  preserving runtime configuration. Built `zpr-simulator:20261014-gateway-package`;
  deployed image ID
  `sha256:597e94fd54b1f1672c9346273c4d5dc7dcd438e16989be11a9242b8e79ad3c8e`.
  Executable SHA-256:
  `ed568925654309380db1555e607779104dd234bbbaba27e7ff8359cdcf01ee42`.
  Final container started at `2026-10-14T14:38:01.220246174Z`.
- Container Config and HostConfig match the previous Simulator exactly except
  for its image. All 21 other running containers retained their IDs and start
  times, including Control Room, Control-Service, Policy-Service, the independent
  operator DNS relay, BIND, and Great Lakes runtime machines. No production
  gateway instance or other production service was replaced.
- Great Lakes remains active. The previous run was in a failed terminal state,
  not running; planned UI replacement reset in-memory run history to idle.
  Restored the exact two prior DNS relay commands and PID files. Both 8054/8055
  listeners and recorded PIDs are alive; statistics return version 1.8 and BIND
  boot time remains `2026-10-14T14:05:37.750Z`.
- All 76 directly served static files match source byte-for-byte. The other
  two source HTML files have existing route redirects: `/index.html` returns
  301 to `./`, and `/agents.html` returns 302 to `/machine-logs.html`; these
  are not asset mismatches. No static source changes were part of extraction.
- All 62 selected desktop/tablet shared-editor and organization regressions
  pass after rollout using fixture APIs and real source assets. The known
  definitions-history selector test remains excluded in both viewports.
  These are automated source/UI regressions, not a live signed-in browser or
  full-suite certification. The deployed executable's `web-gateway-service`
  dispatch preserves its invalid-workload rejection without opening a listener.
- Native/Linux builds, focused gateway race tests, vet, dependency checks, and
  the unchanged four full-Go-suite baseline failures are documented in
  [migration evidence](REDIRECTORY.md#first-backend-extraction-2026-10-14).
  No scenario execution, organization activation, policy write, or live
  Internet gateway forwarding test was performed.
- Rollback retains stopped container
  `zpr-simulator-before-gateway-package-20261014`, old image
  `sha256:3708380378f443737d271dd01919cf3d98e0e5e6ce94e2cb243f879aed2af025`.
  To revert, replace only the new Simulator with this retained container and
  restore the same two DNS relays/PID files; other services must remain untouched.
- Configuration-preserving replacement used Docker API state in memory,
  without writing container environments/credentials to disk. Temporary helper
  removed after verification. Extraction and this deployment record are not
  committed or pushed.

### Simulator organization editor and full worktree (2026-10-14 04:58 UTC)

- User selected deployment of the entire current dashboard worktree, including
  pending shared editor/gutter/undo changes, rather than an organization-editor
  overlay. Replaced only Simulator with
  `zpr-simulator:20261014-organization-editor`, image
  `sha256:3708380378f443737d271dd01919cf3d98e0e5e6ce94e2cb243f879aed2af025`;
  executable SHA-256
  `60d4b1751688b8465bdc8d8e9873707ce772654b805127b258afd2c546c8f581`.
  Started at `2026-10-14T04:58:05.272000721Z`.
- Container configuration and host configuration match the prior container
  exactly except for its image. All 21 other running containers retained their
  IDs/start times, including Control Room, Control-Service, Policy-Service,
  independent operator DNS relay, BIND and Great Lakes runtime machines.
  This does not deploy pending Control Room assets to Control Room.
- Great Lakes remains selected and the scenario state is idle. The planned
  Simulator restart resets in-memory history. Restored both prior DNS relay
  commands and PID tracking; statistics return JSON version 1.8 and BIND's
  original boot time is unchanged.
- All 74 directly served static assets match the worktree byte-for-byte,
  including exclusive Raw/Form, File.../Browse..., identity History and
  shared Wrap/search controls. All 48 selected desktop/tablet organization
  profile/definitions/duplicate/new/import tests pass after rollout. The
  existing definitions-history test with a stale Close selector remains
  excluded in both viewports; no full-suite pass is claimed.
- The integrated live browser tool timed out, so no live interactive browser
  check is claimed. Simulator APIs, served assets and relay processes were
  verified independently. No scenario, activation or production policy write
  was performed.
- Rollback retains stopped container
  `zpr-simulator-before-organization-editor-20261014` and image
  `zpr-simulator:before-organization-editor-20261014`
  (`sha256:9c16fa17bfba01cf5df8935f0c2e315ca2421799326b8b40bd399ac28558e7de`).
  Restoring it also requires restoring the two Simulator DNS relays.
  No commit or push.

### Independent Control Room DNS statistics transport (2026-10-13 23:05 UTC)

- Replaced the unreachable `http://host.docker.internal:8054` Control-Service
  origin with `http://zpr-dns-stats-relay:8054` on the internal
  `zpr-dns-stats-read-only` Docker network. No ports are published by the relay;
  BIND TCP/8053 remains loopback-only and is not exposed on the host/underlay.
- The standalone relay uses its own `zpr-dns-stats-relay:local` image
  (`sha256:ea94d0da03b5f754aabc4b2a0f52eab33a054826259fb86bdfb31048a07101dc`),
  restart policy and explicit operator configuration in
  `.local-runtime/dashboard-stack/dns-stats-relay.json`, selecting
  `great-lakes-node0` as BIND's network namespace. It reads no Simulator
  profiles, active organization, sessions or workloads. It uses Docker socket
  administrative access; this local transport is not a production ZPR adapter.
- Control-Service's final container started at
  `2026-10-13T23:05:25.256563968Z`, retaining its original immutable image
  `sha256:66bd2883b5da7dd7d175529a80448d19ad4f19bafbe4dc66e2b7da4a58f1cb8a`.
  Host configuration and all environment entries except `ZPR_DNS_STATS_URL`
  match the prior container. Existing networks are preserved and the private
  statistics network is added. A brief initial replacement resolved the
  mutable image tag to a newer image; it was immediately corrected to the
  original immutable image before verification.
- Live `/api/dns/stats/json/v1/status`, `/server` and `/zones` requests using
  Control Room's existing service client certificate return BIND 9.20.29,
  JSON statistics version 1.8, real query counters and eight zone entries.
  With the user's permission, paused Simulator, repeated all three mutual-TLS
  checks successfully (193 IPv6 requests and 36 successes at that time), then
  resumed Simulator without resetting scenario state. Relay start is idempotent
  and its readiness check also passed after the pause.
- Signed-in browser display verification was not completed: the shared Control
  Room page remained logged out. The user elected to check the DNS display
  themselves. The verified live route is the certificate-authorized
  Control-Service API, not a claimed successful browser-login check.
- Simulator, BIND, Control Room and Policy-Service retain their previous
  container IDs and start times. Only Control-Service was replaced; no DNS
  or policy configuration was changed. Focused Go relay/proxy regressions pass
  with the race detector; shell syntax and editor diagnostics pass.
- Stack startup now requires an explicit authorized `ZPR_DNS_STATS_URL` or
  independent operator relay configuration, instead of silently defaulting to
  Simulator's relay. An explicit origin takes precedence. The relay's lifecycle
  is not tied to Simulator UI replacement/shutdown.
- Rollback retains the stopped
  `zpr-control-service-before-dns-stats-20261013` container. To roll back, stop
  and remove only the current `zpr-control-service`, rename the retained
  container back to `zpr-control-service` and start it. This restores the prior
  (unreachable) origin. Remove the operator relay configuration if reverting
  future startup behavior; stop the relay separately with
  `sh scripts/control-dns-stats.sh stop`.
- Temporary container-create payloads containing existing environment values
  were permission-restricted and removed. No commit or push for this repair.

### Simulator shared top banner (2026-10-13 19:32 UTC)

- Deployed `zpr-simulator:20261013-top-banner`, immutable image
  `sha256:9c16fa17bfba01cf5df8935f0c2e315ca2421799326b8b40bd399ac28558e7de`.
  Executable SHA-256:
  `39ad0fa644a3f3f4d9680df0f12a0b53f714263839e5e3bc6f9673b501a948a9`.
  The final container started at `2026-10-13T19:30:35.345292293Z`.
- The user approved a planned Simulator UI/relay restart after the completed
  Great Lakes workday scenario. Preserved the prior container configuration
  and host configuration exactly, except for the release image. Great Lakes
  remains active. In-memory scenario history reset to idle as disclosed.
- Restored the two live DNS socat relays, including their PID tracking. The
  internal DNS statistics endpoint returned version 1.8. The original process
  listing also contained 21 defunct socat processes, not additional live relays.
  Great Lakes runtime containers, Control Room, Control-Service, Policy-Service,
  and every other previously running service retained their IDs and start times.
- Verified the served Simulator navigation, operator-login and Simulator CSS
  hashes against the worktree. Live Organizations and Scenarios browser checks
  show backend Connected, Great Lakes Instruments, no running scenario/idle,
  and User: Not signed in before Help, without page error alerts. No scenarios,
  organization activations or production policy writes were triggered.
- Live verification caught and fixed removal of hidden organization status
  elements still used by the page controller; the banner now preserves them.
  All 36 focused desktop/tablet Help/banner tests pass after this correction.
- Rollback retains the stopped
  `zpr-simulator-before-top-banner-20261013` container and image
  `zpr-simulator:before-top-banner-20261013`
  (`sha256:66bd2883b5da7dd7d175529a80448d19ad4f19bafbe4dc66e2b7da4a58f1cb8a`).
  The deployed implementation, regression tests and rollout record were
  committed and pushed after deployment verification.

### Control Room GUI and config form (2026-10-13 19:06 UTC)

- Rebuilt from the pushed `main` worktree at
  `25be3e91c67e439fe14aadf615c6e5f3a5349599`, tagged
  `zpr-editor-ui:20261013-control-room-guidelines`, image
  `sha256:eeca92b7a7884cecbd7c4cf5cee1adf98012409629c255da2264c290214a9a19`.
  The dashboard executable SHA-256 is
  `194f60aeb656bb624c21b9040c99c38d31a69912bdb8ea05a8be36fdbd9648a8`.
- Replaced only the Control Room container on HTTPS port 8787. It started at
  `2026-10-13T19:06:37.317834586Z`. HTTPS `/` returned 200 and the
  `/auth/operator/config` endpoint confirmed operator login is enabled. All
  74 embedded static assets match the source worktree byte-for-byte.
- Policy-Service was not restarted; its mutual-TLS config-form endpoint
  returned HTTP 200 for a draft-only transformation check, and the focused
  config-form Go tests pass. Control-Service and Simulator were not restarted.
  Their existing containers remained running. Focused desktop/tablet Simulator
  and config-form browser regressions previously passed 18/18 each; no full
  dashboard suite pass is claimed.
- Rollback image retained as
  `zpr-editor-ui:before-worktree-completion-20261013`
  (`sha256:c50aa7012713308d77f55af798d66aa55f39e34ddea7b14786eb6efb4aa248a6`).
  The Control Room restart resets operator sessions; reload and sign in again
  if prompted. No runtime policy writes or Simulator operations were performed.

### Simulator GUI design guidelines (2026-10-13 12:23 EDT)

- Deployed the local Simulator as `zpr-simulator:20261013-gui-guidelines`,
  image `sha256:66bd2883b5da7dd7d175529a80448d19ad4f19bafbe4dc66e2b7da4a58f1cb8a`;
  executable SHA-256
  `6ade4855208c9cc13241639a8adb550ae0ffd74d25a618241a0a92a916fd615f`.
  The user explicitly selected deployment of the entire current dashboard
  worktree rather than a GUI-only overlay; other pending edits in that worktree
  are included in this Simulator build.
- Replaced only the Simulator container using the UI-restart path. The active
  organization remains Velocity, activation is idle, and no scenario machines
  or relays were running. The Simulator restarted at
  `2026-10-13T16:23:09.515Z`; Control Room, Control-Service and Policy-Service
  were not restarted. The restart resets in-memory Simulator history.
- All 74 directly served static assets match the worktree byte-for-byte; the
  root, `/index.html` and `/agents.html` redirects were checked separately.
  Focused desktop/tablet browser regressions pass: 18/18, covering graph
  maximize/restore, Activity metric pulses and numeric alignment, Help/top-bar
  placement, and the 11px text minimum.
- The focused Simulator-page Go test passes. The full dashboard Go suite was
  attempted but did not pass: failures cover enrollment preflight,
  `ZPR_MULTINODE_PLATFORM`, scenario placement-helper and Linux-ELF tests. No
  commit or push.
- Rollback image retained as
  `zpr-simulator:before-gui-guidelines-20261013`
  (`sha256:a35607c2fb95a615f8b261165dcc4b3a4374efebd5833c02c37b55a11a341674`).

### Config form editor (2026-10-13 05:49 UTC)

- Deployed Control Room on its usual HTTPS port 8787 and Policy-Service on
  mutual-TLS port 8789. Control-Service continues proxying through its existing
  production contract; it was not replaced or restarted.
- Room image `zpr-editor-ui:20261013-config-form`, immutable image
  `sha256:c50aa7012713308d77f55af798d66aa55f39e34ddea7b14786eb6efb4aa248a6`,
  binary SHA-256
  `40e0416a92de75508f87e0486d03c5e88ee3f6d8b467e4302571abfb469b4566`.
  Policy image `zpr-policy-service:20261013-config-form`, immutable image
  `sha256:d297936154ac4442ca4141aa3a027c1daccc60305566de1ac5666d2882adec80`,
  binary SHA-256
  `64ace285a174c42e129550eb85dec1f8b844e3d16bd868ad0f25a8572fc2a7b5`.
  Verified at `2026-10-13T05:49:46.884Z`.
- Reconstructed both prior live binaries byte-for-byte before applying the
  scoped release. Room changes only four assets: Config form/editor scripts,
  Config markup/cache versions, and Config-only styles. All 78 served assets
  verified. Existing map filter fix and unrelated live UI preserved; pending
  organization-editor changes excluded.
- Policy-Service adds only the stateless `POST /api/policy/config/form`
  draft transformation and route. No runtime installation or activation,
  migrations, organization switches or Simulator API calls.
- Exact-release Go form tests pass. All 52 focused desktop/tablet browser
  cases pass against the release UI and actual endpoint, including invalid
  input, comments/unknown fields, revision ownership and stale-response
  protection. The previously documented baseline Policy bottom-margin
  failure remains excluded; no full-suite pass is claimed.
- Real OIDC-authenticated desktop/tablet live checks verify Form/Raw
  synchronization, production proxying, syntax Analyze, preserved comments
  and unfamiliar settings, and responsive layout. No persistent writes or
  Simulator calls in those checks. Saved policy database, catalog and
  organization context unchanged; container settings, mounts, TLS and
  network contracts preserved. All 15 other running service start times
  unchanged, including Simulator, Control-Service and infrastructure.
- Stopped rollback containers retained:
  `zpr-control-room-before-config-form-20261013` and
  `zpr-policy-service-before-config-form-20261013`.
  Operator sessions reset; reload Control Room and sign in again.
  No commit or push.

### Control Room map filter popup clipping fix (2026-10-13 00:44 UTC)

- Deployed only `zpr-editor-ui:20261012-filter-popup`, immutable image
  `sha256:46f247c80e559bd402e86b0980b7138faee43f6a584c80a3a004deab72eb2eb5`.
  Started `2026-10-13T00:44:50Z`; verified
  `2026-10-13T00:45:17Z` on HTTPS port 8787.
- Resolved popup clipping bug: removed `.topology-page .legend { flex: 1; max-width: 100%; justify-content: flex-start; }`
  and set `.map-legend-key .legend, .topology-page .map-legend-key .legend` to
  `flex: none`, `width: min(620px, calc(100vw - 60px))`, and `max-width: min(620px, calc(100vw - 60px))`.
  Popup now expands to full 620px width without collapsing to the `<details>` summary button width (74px).
  Bumped stylesheet cache bust to `app.css?v=101`.
- Release binary SHA-256
  `55d8b16df52b32538623e1b7256d00c77acfa02a0bbc65eecd1920fbdbf54ba9`.
- Verified popup layout, bounding box (620px × 77px), and children dimensions
  in Playwright.
- Served markup, direct TLS enforcement, preserved container configuration,
  and unchanged start times of all 16 other running services verified.
  Simulator, Control-Service and Policy-Service untouched; no runtime/scenario
  mutations, commit, or push performed by this rollout.
  Stopped `zpr-control-room-before-filter-popup` retains rollback.
  Operator sessions reset; sign in again.

### Control Room map filter popup (2026-10-13 00:13 UTC)

- Deployed only `zpr-editor-ui:20261012-filter-popup`, immutable image
  `sha256:50acaf4e7ae98e7169a8c001ca6bd8d410a6ea4ef83d04825e607569f7fc8bd3`.
  Started `2026-10-13T00:13:51.072139834Z`; verified
  `2026-10-13T00:13:55.210Z` on HTTPS port 8787.
- Embedded `#topology-search` and the component type legend within the
  `<details id="map-legend-key">` disclosure popup triggered by a button labeled
  "Filter..." positioned in the upper right toolbar near Traffic controls.
  Kept "Clear highlight" accessible outside the popup in the toolbar. Added
  responsive column wrapping on viewports below 700px.
- Release binary SHA-256
  `212bf5240612f2012f2474e0b6949ca10ec24179a65e08f930f419810b069077`.
- All 82 geography desktop/tablet tests passed; all 26 map legend and filter
  tests passed.
- Served markup, direct TLS enforcement, preserved container configuration,
  and unchanged start times of all 16 other running services verified.
  Simulator, Control-Service and Policy-Service untouched; no runtime/scenario
  mutations, commit, or push performed by this rollout.
  Stopped `zpr-control-room-before-filter-popup` retains rollback.
  Operator sessions reset; sign in again.

### Source publication validation (2026-10-12)

- User authorized committing and pushing all pending source/documentation work,
  with the older detached Simulator experiment preserved on a separate archive
  branch rather than merged over the current implementation. Runtime databases,
  credentials, generated releases and private test receipts are not included.
- Current dashboard/trusted-service build and focused benchmark, bundled
  scenario, machine-control and delegation Go regressions pass. All 56 script
  tests pass; the enrollment package suite passes. Existing unpublished Rust
  recovery commit passes `cargo test -p ph --lib`: 268 passed, 2 ignored.
  Native live acceptance remains the separately recorded 15/15 full-matrix pass.
- Broad dashboard Go validation is **not green**: failing groups are
  `TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator` (missing current
  readiness preflight in its invitation fixture),
  `TestOrganizationPolicyInstallNeverUploadsStaleBundle` (extracted shell fixture
  lacks platform initialization), `TestScenarioPlacementPreflightMatchesLauncher`
  (helper moved out of the stack script), and
  `TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact` (fake build
  output is not an architecture-valid ELF executable). These failures are
  recorded, not skipped to claim a clean broad suite or changed during
  publication. Local staged-content credential-pattern scan found no private-key
  material or recognized credential tokens; whitespace and deployment shell
  syntax checks pass.

### Control Room disconnected inventory overlays (2026-10-12 20:56 UTC)

- Deployed only `zpr-editor-ui:20261012-map-overlay`, immutable image
  `sha256:4c3a58c06ec48828368f89e34b09bebda55038da6f960a03e2204abe80c618db`.
  Started `2026-10-12T20:56:47.210514125Z`; verified
  `2026-10-12T20:56:52.673Z` on HTTPS port 8787.
- Replaced the preceding SVG trapezoids with Detached/Lost text-button
  overlays fixed in the map's bottom-right corner, styled like Fit/zoom
  controls. Counts and member inspectors remain; disconnected inventory no
  longer affects geometry or Fit. Buttons remain fixed during zooming.
- Reconstructed the preceding live executable byte-for-byte, preserving the
  backend and all but three of 77 assets: only `app.js`, `app.css` and
  `index.html` changed. Linux arm64, Go 1.26.4, CGO disabled, `go build -trimpath`;
  release binary SHA-256
  `30ce9bad311a2d3c1dbc08bb3142ab083ad7426835cf98ba78f9d99bcc42d8cf`.
- All 78 selected exact-release desktop/tablet map cases passed. The two
  previously confirmed baseline World Map Fit coverage failures remain
  excluded and unresolved. Real desktop/tablet login verifies HTML buttons
  outside the SVG, bottom-right placement, stable screen position across
  Zoom/Fit, group counts/member lists, keyboard activation and Maximize.
  No browser errors or Simulator API requests occurred.
- All 77 served asset hashes, unsigned API rejection, preserved container
  configuration/network contract and unchanged starts of 19 other running
  services verified. Simulator, Control-Service and Policy-Service untouched;
  no runtime/scenario mutations, commit or push performed by this rollout.
  Stopped `zpr-control-room-before-map-overlay` retains rollback.
  Operator sessions reset; sign in again.

### Control Room disconnected groups and map animation recovery (2026-10-12 20:50 UTC)

- Deployed only `zpr-editor-ui:20261012-map-groups`, immutable image
  `sha256:f9354e3f10c1cf0f33e1c181fc0d3b3cd8b4a0aab7108b6881619eb40769e04c`.
  Started `2026-10-12T20:50:03.018356841Z`; verified
  `2026-10-12T20:50:08.589Z` on the existing HTTPS port 8787.
- Both maps show separate brown Detached/Lost trapezoids below/right of the
  network, with aggregate adapter counts, keyboard activation and member
  inspectors. Groups have no connecting lines and participate in Fit.
  Attachment history remains browser-local and organization-scoped.
- Restored parent-relative component growth/retraction and retiring connection
  lines, including moving-parent layouts. Active grants outside the ten recent
  entries now trigger decision feedback without replaying unchanged grants.
- Captured all 77 preceding live assets and reconstructed the preceding binary
  byte-for-byte (`0a17e78f4391ac849e199bb0d9cf8816f3712047fdabdafb1c2be0ff213a1356`).
  Only `app.js`, `app.css` and `index.html` changed; existing traffic assets,
  backend and unrelated UI assets were preserved. Linux arm64, Go 1.26.4,
  CGO disabled, `go build -trimpath`; release binary SHA-256
  `c270ba790c2b1e04b1036334f15aa745a8a2d6943bfe845bd26cffe9d065a862`.
- All 78 selected exact-release desktop/tablet geography cases passed with
  Simulator APIs prohibited. The two previously confirmed baseline World Map
  Fit coverage failures remain excluded and unresolved.
- Verified all 77 served hashes, unsigned API rejection, identical container
  configuration/network contract and unchanged starts of 20 other services.
  Real desktop/tablet login verified both groups, counts/member lists, Fit,
  keyboard activation and Maximize without browser errors or Simulator API
  requests. Live group counts were zero; nonzero transitions are fixture-tested.
- Stopped `zpr-control-room-before-map-groups` retains rollback. Operator
  sessions reset; sign in again. No Simulator, Control-Service, Policy-Service,
  runtime/scenario changes, commit or push performed by this rollout.

### Simulator switching isolation and native ARM64 infrastructure (2026-10-12)

- Final post-soak native v7 acceptance passed **15/15 published scenarios**
  from `2026-10-12T22:52:03.882Z` to `2026-10-12T23:19:23.012Z`:
  Velocity 1, Load Lab 4 (200/100/150/50 clients), Great Lakes 3, Redwood 2,
  Northstar 5, including denial cleanup revision 2. Every case passed published
  steps and independent stopped-machine/logged-out/empty-workload verification.
  Original Northstar selection restoration passed. Independently checked
  `Organization ready`, no running managed machines, no authenticated sessions
  or workloads and released runtime mutation lock after completion.
  Private report: `simulator-matrix-native-v7-post-soak-full-20261012.json`.
  The earlier intermittent Great Lakes authorization/readiness failure remains
  unexplained; this full pass plus three passing switching-soak cycles is
  acceptance evidence, not a claim that every intermittent failure is fixed.
  Expanded failure diagnostics remain enabled. No commit or push performed.
- Independent v5/CA matrix recovered Great Lakes and passed all three published
  scenarios. Approved Visa-only initialization completed for Redwood and
  Velocity; approval markers consumed. Redwood passed both published scenarios.
  Velocity passed startup/readiness and health traffic but failed the benchmark;
  all cleanup and verified Northstar restoration passed.
  A targeted repeat preserved the exact error: `Client.Timeout ... while reading
  body`, caused by a 10-second HTTP body deadline inside an otherwise 60-second
  benchmark/75-second scenario budget. Workload links remained active until
  cleanup; this is distinct from the open Load Lab keepalive issue.
- Corrected benchmark client to use its existing 60-second overall context
  rather than an additional 10-second body timeout. Tests verify 100 warmed
  latency samples, exactly 16 MiB, response failures and an actual 11-second
  response body. Scoped v6 backend/controller deployed, retaining 70 assets,
  unchanged configuration/network contract and starts of 16 other services.
  Executable SHA-256
  `c17402386de38546f11e631c6e682f074f2348e9a0a77449efed7106d4a4abae`;
  image `zpr-simulator:20261012-switching-isolation-v6`,
  `sha256:98decad9899937c729d987e586792ba861c008ad6b763f8d075339e7a6c94170`.
  Previous controller/backend retained for rollback. Live Velocity still failed
  at its existing overall deadline; all cleanup and Northstar restoration passed.
  Timer correction alone is not a datapath fix.
- Live v6 TCP/kernel evidence isolates a real receive-path stall: client delivery
  stopped near 1.9 MiB; service acknowledged-byte count stayed at 1,925,267 while
  TCP retransmissions/backoff increased. The client workload substrate socket
  retained 214,784 queued bytes against a 212,992 receive-buffer limit and
  dropped 142 datagrams in the kernel. Application queue-backpressure counters
  stayed zero, so they do not detect this receive-ring failure. Client dock
  keepalive then failed before benchmark completion. Logs show the active packet
  I/O engine is `io_uring`. A reversible diagnostic selects the already-supported
  `posix_unbatched` engine only for Simulator machine adapters, retaining existing
  queues, policies and transport deadlines; original controller is restored by
  the diagnostic runner. Rust source/production services remain unchanged.
- POSIX A/B acceptance passed the full Velocity scenario and all cleanup:
  100 measured samples, exactly 16,777,216 bytes, ~994 Mbps transfer,
  p50 0.423 ms / p95 0.567 ms / p99 0.603 ms. Both workload logs confirm
  `posix_unbatched`, with no dock keepalive failures. Northstar restoration and
  restoration of the released v6 controller were independently verified.
- Promoted this Simulator-only adapter launch fix in scoped v7:
  `zpr-simulator:20261012-switching-isolation-v7`,
  image `sha256:6ab37ddf9939ef196fd0082bb3351983ea96e0795a5740a68dfce4e4814f3e0b`,
  backend/controller SHA-256
  `426263b15ef8c34eebf727d37626326dfd6f86db4885e6d3b1ff46e04b837a4a`.
  Preserved 70 served assets, configuration/network contracts and starts of all
  16 other currently running services. Focused Go regressions and 32 targeted
  runtime/matrix Node tests pass. Native full 15-case acceptance remains pending;
  the isolated Velocity pass is not proof of Load Lab or the complete matrix.
- Full v7 native execution reached all 15 scenarios; all published steps
  completed, including all four Load Lab cases (200/100/150/50 clients),
  Great Lakes 3/3, Redwood 2/2 and Northstar 5/5. The matrix nevertheless failed
  final state-leak verification: Northstar's last `machine-policy-deny` definition
  stopped its workload and logged out but omitted `stop_machine`; machine-01
  remained running. Safe restoration was correctly blocked. This report remains
  failed and is not retroactively relabeled as acceptance success.
- Recovered only that test-owned machine through Simulator's normal stop API.
  Verified all machine containers stopped/exited/missing and sessions/workloads
  empty. Added the missing cleanup to the bundled scenario and a regression
  requiring every bundled machine start to have a matching cleanup stop.
  Published revision 2 through the workspace's optimistic revision API; checked
  no unpublished draft existed, permitted only the cleanup/description delta,
  and retained revision 1. No directory/credential/policy reset was performed.
- Matrix reporting now marks completed steps with leaked runtime state as a
  serious case failure, rather than leaving the individual result as `passed`
  while the overall report fails. Bundled-scenario Go regression and all
  23 matrix Node regressions pass, including a completed-run machine-leak test.
  Targeted live denial revision-2 verification passed every step, including
  machine stop, logged-out/empty-workload/stopped-machine verification and clean
  Northstar restoration. The corrected complete 15-case acceptance is being
  repeated; only its terminal report can certify full acceptance.
- Corrected full v7 rerun passed Velocity and all four Load Lab cases again but
  failed Great Lakes activation before its scenarios started. The failure was
  machine-07's real controller readiness request returning HTTP 504; node2's
  `RegisterAA` authorization timed out and its Visa API connection closed.
  Node0 logs show repeated controller/DNS actor disconnects. This is not the
  corrected denial cleanup issue and the full matrix remains failed.
  Activation rolled back and the runner restored Northstar; independently
  checked `Organization ready`, no running managed machines and no authenticated
  sessions/workloads. A bounded Great Lakes activation retry records kernel UDP
  receive queues and actual fastpath worker wait states before any further
  packet-I/O changes; transport deadlines/readiness gates remain unchanged.
- The isolated Great Lakes activation/runtime-verification retry passed all
  three node-placement controller probes, the published readiness case and clean
  Northstar restoration. Its kernel observer measured zero UDP receive errors
  and buffer drops on nodes 0–2 and the Visa adapter, with sampled fastpath
  workers waiting normally in poll. This successful retry does not establish the
  prior failure's root cause or certify reliable switching.
- Failed readiness now emits bounded controller/adapter logs, UDP counters,
  receive queues and thread wait states before probe removal; infrastructure
  diagnostics cover every configured node plus Visa/SimulatorControl adapters.
  Source is mounted into the live Simulator, so this script change persists
  without replacing its binary or production service defaults. Shell syntax,
  whitespace and 34 runtime/matrix Node regressions pass.
- All three bounded Great Lakes↔Northstar activation soak cycles passed the
  three configured node-placement controller probes, published runtime readiness
  scenario and original-selection restoration. Independently verified Northstar
  `Organization ready`, no running managed machines and empty authenticated
  sessions/workloads afterward. This does not resolve the earlier intermittent
  failure's root cause. The complete 15-case acceptance is being repeated with
  expanded failure diagnostics and corrected denial revision 2; certification
  remains pending its terminal report and clean restoration.
- WAL-aware logical comparison after approved initializations retains every
  previous policy revision and all workspace artifacts/revisions exactly.
  Redwood and Velocity each append one `configured-source-import` Effective
  network policy revision; the existing record changes only its current-revision
  pointer and updated timestamp. Northstar tables are unchanged from the
  21:19:55 logical baseline. These refreshes are not installation acknowledgements
  and are recorded rather than concealed or reverted.
- Follow-up backend release at `2026-10-12T21:21:18.710107001Z`:
  `zpr-simulator:20261012-switching-isolation-v5`, immutable image
  `sha256:0e5dd30bf95f63cec78cc7f2c41ef7bb564324d7f3182f085790621de9fb6e57`.
  Backend SHA-256
  `30aa0630dacdae96ac7d1f06c4cc063dce6d1f31d9948bebbcf8079ec262bab9`.
  Verified-baseline overlays contain only Gateway/controller tunnel separation,
  matching workload-address discovery and explicit numeric routing tables.
  All 70 served static assets and configuration/network contracts preserved;
  starts of 19 unrelated services unchanged. No Control Room rollout.
  Machine controller remains the separately installed tunnel-isolated ARM64
  binary `70aaa36cfae5c9c4e971d09943232288866efa26fb6512a1960abb5a9d732508`.
- Gateway originally reused controller `tun5`, disrupting cleanup. It now uses
  `zgateway0`; backend discovery follows that interface. Live v4 exposed the
  derived, invalid routing table `10zgateway0`; v5 preserves table 105 explicitly.
  The v5 live case reached the Gateway but returned HTTP 502; all cleanup passed.
  An isolated standard-library HTTPS probe reproduced missing CA trust roots
  in the machine image. Adding `ca-certificates` changes that same probe to
  `200 OK` without bypassing certificate validation.
  Promoted native machine image
  `sha256:1d88b42ecacb4c935707f7adff9e960ee9a6f0a9f527f4ebf4ce8d912298b064`;
  preceding image retained under `zpr-sim-machine:before-20261012-ca`.
  The targeted Northstar Gateway retry starting at 21:30:40 UTC passed all
  19 published steps, including real upstream HTTPS traffic and every cleanup
  step. Final restoration to the original Northstar selection passed;
  activation reports Organization ready, no managed machines are running,
  no authenticated sessions/workloads remain and the mutation lock is released.
  This does not verify Great Lakes recovery or the complete organization matrix.
- The v5 case's Great Lakes restoration failed during infrastructure startup:
  Visa-adapter/node keepalive timeouts and failed reconnects occurred even
  without Load Lab. Rollback restored Northstar's machine-control path but
  failed operator verification: Control-Service tried to join the removed
  Great Lakes trusted-feed network. Activation and rollback now recalculate
  feed scope for the organization being started/restored. No automatic
  healthy-restoration claim is made for that failed run. The subsequent
  Northstar recovery and targeted Gateway run passed; Great Lakes recovery
  remains to be independently retried.
- Deployed the switching backend as
  `zpr-simulator:20261012-switching-isolation-v3`, immutable image
  `sha256:4323cba33a17c02bcb22d172ffb26e075cacb534e07870124d403d9d20a140ef`.
  Linux/ARM64, Go 1.26.4, CGO disabled; executable SHA-256
  `1c912c2433a217d5ed5b9a184a3bda29bb4affe589a254a4e820e4c576bf8703`.
  Reconstructed the verified backend baseline and retained the exact live UI,
  rather than deploying unrelated dirty-source changes.
- Fixed the composed SimulatorControlService provider to match authenticated
  adapter identity without constraining its dynamically assigned ZPR address.
  The approved one-time `dynamic-control-policy-v1` migration preserves
  directory data/editor drafts; subsequent switches preserve installed policy.
  Admin endpoints and runtime-verification service IPv4 addresses are discovered
  from actual Docker bindings/networks, not guessed ports or old subnets.
- Machine assets and controller requests are organization/generation scoped.
  Running containers with a different scope are rejected; old controllers
  cannot update the new runtime. API/script mutations are guarded during
  switching, and all managed running machines are stopped and verified.
  Activation and rollback require a disposable machine at every configured
  node to heartbeat and acknowledge a real command over ZPR/mTLS. Failure
  reports its phase and whether the previous runtime was independently verified.
- Earlier live matrix runs passed Great Lakes traffic cases but exposed an
  intermittent x86 Rosetta assertion, exit 133, and an old-subnet verifier.
  The v3 verifier corrects the subnet assumption. After explicit approval,
  built native ARM64 infrastructure to remove x86 emulation from Simulator
  nodes, Visa, adapters, gateway, directory/manager and helper processes:
  - `zpr-multinode:20261012-arm64`:
    `sha256:aa33d157f43a0ed77a675d652ae6d6a5001850dc4751bc82b9ae1b0b89eca3e7`.
  - Initial `zpr-multinode-directory:20261012-arm64`:
    `sha256:d40f91cc78ae53a6d9d7be49abe764431a65f50989c7a037f2b9ec648b98b6d3`.
  - Initial `zpr-multinode-directory-manager:20261012-arm64`:
    `sha256:fe49b337142d03e918b3201282ce98a75f15e177fb46314cbef0311178ccb478`.
  Cached rebuilds subsequently moved directory/manager tags. Observed current
  image IDs at 21:23 UTC are respectively
  `sha256:dd37c362c4f3f3487673f4d07623bf70dd81eeb03c409bae2e4ac9baba5f102d`
  and `sha256:22224a0cee5a6b2d4e7899fa4a00976786712da395b42b0c78ec9c1b86995de9`.
- Native Rust/runtime libraries share Debian trixie's ABI. The runtime retains
  Ubuntu 24.04's Redis 7 package, avoiding an incidental Redis 8/database-format
  upgrade. All eight baked executables have ARM64 ELF headers and successful
  loader/help checks; native Redis 7 startup/PING round-trip passes.
  Architecture preflight rejects mismatched images and cached helper binaries
  before activation stops the previous runtime. Existing amd64 binaries/tags
  remain available; native settings survive Simulator recreation/restart.
- Simulator native provisioning configuration persisted at
  `2026-10-12T19:56:31.943531549Z`, retaining the same v3 executable/image.
  Verified 72 served asset responses unchanged, identical mounts/ports/network
  contract and unchanged starts of 19 other services. Only five explicit native
  provisioning environment settings changed. No Docker Desktop restart.
  Control Room was untouched by this Simulator work; subsequent independent
  Control Room rollouts are recorded separately above.
- Validation: 45 targeted Node platform/scope/admin/matrix regressions, focused
  Go controller/activation/runtime-state/multi-subnet verifier tests, shell
  syntax, whitespace and editor diagnostics pass. Native Great Lakes activation
  passed all three controller-location probes and all three published scenarios.
  Load Lab activation/readiness passed, but its 200-client fanout failed when
  both ends of the service dock link timed out waiting for reliable keepalive
  acknowledgements. Cleanup and verified Great Lakes restoration passed; the
  Rust transport investigation remains open. No Rosetta assertion occurred.
- The independent remaining-organization matrix stopped before scenarios at
  Northstar's missing Visa database. Great Lakes restoration passed. Read-only
  inspection found the same missing-state condition in Redwood and Velocity.
  With explicit approval for all three, corrected the database guard to honor
  the existing Visa-only initialization approval without silently resetting an
  unapproved runtime. Existing credentials, directory data and drafts remain
  intact; runtime-only policies from missing databases cannot be recovered.
  One-time approval markers are consumed after composed-policy installation.
  The first approved recovery initialized Northstar's Visa state, then exposed
  a readiness-selector bug: it assumed explicit machine ownership mappings,
  which Northstar's supported single-node profile omits. Rollback restored
  Great Lakes. Initial main-file checks reported all 180 fingerprints unchanged,
  but later SQLite WAL checkpointing exposed a Northstar policy database change:
  a `configured-source-import` appended revision 8 of Effective network policy
  at 20:34 UTC. The other 179 main-file fingerprints were unchanged; 38 older
  policy revisions remain. Raw main-file hashes do not prove draft preservation.
  Consistent SQLite backups including WAL state and logical table receipts for
  Northstar, Redwood and Velocity were captured at 21:19:55 UTC before further
  approved recovery. No database was reverted to conceal the discrepancy.
  Readiness now reuses actual machine-launch placement,
  validates all five profiles in regression tests and checks node coverage
  before stopping the old runtime. Northstar activation and its active-team,
  client-service and default-deny scenarios passed; the subsequent targeted
  Gateway case also passed, but the fifth case is untested. Redwood/Velocity approval markers
  remain pending initialization. Remaining acceptance is being retried;
  full five-organization/15-scenario acceptance is not complete.
- Simulator runtime reservation remains held until acceptance and cleanup.
  This is switching/runtime isolation, not separate installations or independent
  trust roots: Simulator, operator services, engine and the machine TLS identity
  directory remain shared. No commit or push performed by this work.

### Great Lakes organization-switch recovery (2026-10-12 16:03 UTC)

- The Load Lab to Great Lakes switch failed its Control-Service readiness gate:
  Great Lakes published Visa admin container port `8183/tcp` on host port 8186,
  while activation hardcoded the operator endpoint to 8185. Snapshot requests
  reported connection refused. The original attempt rolled back to Load Lab.
- With explicit approval to fix and recover, added shared published-port
  discovery to Simulator provisioning scripts. Activation and rollback,
  bootstrap policy upload, stack startup, and the observability collector now
  use the running organization's actual local Visa admin binding. Invalid or
  ambiguous bindings fail explicitly. Production services still receive
  independently configured URLs, with no Simulator dependency.
- Four endpoint-discovery regression tests, shell syntax/whitespace checks and
  focused Go activation/readiness/policy-upload/independent-service tests pass.
  Existing shared provisioning-script changes were preserved. The scripts are
  mounted into the existing Simulator, so no Simulator image replacement was
  necessary.
- Retried through the existing Organizations UI and confirmation flow. Final
  API state is `completed`, active organization `great-lakes`, progress
  `Organization ready`; GUI shows Great Lakes active and Directory ready.
  Independent mTLS Control-Service snapshot is connected with no errors,
  all three nodes synchronized, 17 actors and 12 registered services.
- Control-Service now uses `https://host.docker.internal:8186`; collector uses
  `https://vs.zpr:8186`, with logs and metrics successfully exported. The
  unchanged Control Room packet-volume image remains running with its original
  `2026-10-12T15:52:20.737017587Z` start time. No restore-base/database reset,
  full scenario matrix, commit or push was performed.

### Control Room packet volume maps (2026-10-12 15:52 UTC)

- Deployed only `zpr-editor-ui:20261012-map-traffic`, image
  `sha256:a60966b9f2dccf552d2bfc5c091d951b7f943d39dd39f9abadf03de488730311`.
  Started `2026-10-12T15:52:20.737017587Z`; verified
  `2026-10-12T15:52:25.132Z` on the existing HTTPS port 8787.
- Both maps share Traffic Off / Packets, fixed-rate node halos and a Scale
  popover. Hover and inspector show substrate RX/TX, sample age and recent
  history. Substrate packet I/O includes control traffic; no adapter/link
  rates or unique application throughput are inferred. Zero differs from
  dashed waiting/unavailable/stale telemetry. Samples expire during pauses.
- Captured all 76 preceding live assets and reconstructed the actual live
  executable byte-for-byte: SHA-256
  `1dce322f959eec57902904921910d9bf9932e6ed75b43981d34f71f2bca62057`.
  Baseline image was `zpr-simulator:local` with immutable ID
  `sha256:db02a48a9d1a2348917c0ab2a550f1e9c8ac90035e0c5afbfea4185ac363bbf1`;
  its tag alone was not used to identify backend behavior.
- Only `app.js`, `app.css`, the map toolbar/cache references in `index.html`,
  and new `map-traffic.js` changed. All other served assets and backend sources
  are preserved, excluding concurrent provisioning/sidebar/organization work.
  Linux arm64, Go 1.26.4, CGO disabled, `go build -trimpath`; release binary
  SHA-256 `0a17e78f4391ac849e199bb0d9cf8816f3712047fdabdafb1c2be0ff213a1356`.
- All 70 exact-release desktop/tablet geography tests and eight packet-rate
  arithmetic tests pass, including uint64 precision, worker aggregation,
  reset/inventory/session boundaries, stale/outage handling and zero history.
  All 77 served asset hashes, unsigned API rejection, container configuration,
  network contract and unchanged starts of 25 unrelated services verified.
- Real desktop/tablet login verifies both map selectors, Scale, traffic toggle,
  unchanged camera on toggle and Maximize/Restore, without browser errors or
  Simulator API calls. Production currently reports disconnected with zero
  nodes: measured populated halos/history are verified by isolated fixtures,
  not claimed as live traffic acceptance. No backend state was changed.
- Stopped `zpr-control-room-before-map-traffic` retains rollback. Operator
  sessions reset; sign in again. No Simulator or Control-Service deployment,
  commit or push.

### Simulator organization definitions (2026-10-11 20:18 UTC)

- Deployed only `zpr-simulator:20261011-organization-definitions`, image
  `sha256:3d6060ce7ccda2daa073ff91bf2f9a6328b84c7deb04623fe0dbf8f846d46132`.
  Started `2026-10-11T20:18:56.543634Z`; verified at
  `2026-10-11T20:18:59.815Z`.
- Existing-profile drafts, New/Import/Duplicate, portable Export, coherent
  supporting-document editing/history and reviewed cross-document AI are live.
  Saving remains private and does not make a definition runnable.
- Reconstructed the preceding executable byte-for-byte:
  `e03088b433ee77218f104dac61491fb80338a419f3ada310708b5970ca5ed890`.
  Organization-only overlays retain live login/runtime/scenario code; the
  development Load Lab 50-client change and login changes were excluded.
  Linux arm64, Go 1.26.4, CGO disabled, `go build -trimpath`. New binary:
  `d9976e3409ae127f8a575b68628771be8ca87a438558f7a18566583efb000c2f`.
  Image layering preserves the exact preceding runtime image.
- All 74 served asset hashes and two static redirects verified. All 16 other
  running container start times were preserved. Config/env/network contracts
  are unchanged except explicitly approved read-only supporting definitions.
  No Control Room, production service, login, scenario execution, organization
  activation or runtime reset was deployed.
- The live isolated source lacked Great Lakes and Load Lab `policy-demo.zplc`.
  User approved supplying these existing files. Two read-only organization
  support-directory snapshots retain all preceding files unchanged and add
  only the missing compiler configurations; the shared staging is untouched.
  Live Duplicate previews validate 7 Great Lakes and 6 Load Lab documents.
  Config hashes: Great Lakes
  `ee027285c134a335121204f3a698d8c8a7e94d044e78ee56f8a7c795eb6523aa`;
  Load Lab `f3217976d9fd0aa416b08f89d0bfb5099c4d2391af66ee9cd3a6a2358c24e5d4`.
- Focused Go race tests, build/vet and 52 exact-release desktop/tablet browser
  cases pass. A bounded LDAP-editor layout correction fixes a tablet
  pointer-interception regression exposed by release testing.
  Real candidate GUI creation, atomic profile/document save and history/export
  used persistent private storage with no Docker socket or runtime-write
  capability. Final live desktop/tablet reopen verifies persistence across
  replacement. Retained fictional inactive draft:
  `editor-release-smoke-20261011`, revision 2.
  No real AI prompt was sent; fake-provider tests cover AI and live assistant
  status reports ready.
- Two guarded attempts restored the preceding Simulator before final rollout:
  nested file mounts could not create targets inside the read-only source mount;
  a GUI smoke lacked asynchronous loading waits. Directory mounts were tested
  in a networkless container; the smoke now waits for loaded revisions.
  Rollback readiness retry is explicit. Temporary preflight containers removed.
- Rollback `zpr-simulator-organization-definitions-rollback` is retained stopped,
  with the preceding image/config/mounts. Private drafts remain on the existing
  durable runtime mount. Keep the read-only release snapshots for this release;
  a generic source rebuild/recreate is not an equivalent organization-only
  deployment. Structured editors, full semantic/compiler certification and
  draft activation review remain unfinished. No commit or push.

- Map Maximize/Restore deployed to Control Room at 2026-10-11 20:09 UTC,
  verified at 20:09:06 UTC, as `zpr-editor-ui:20261011-map-maximize`.
  Both maps use the shared window-control icons; Restore/Escape and navigation
  cleanup preserve the existing resize/Fit and camera behavior.
  Reconstructed the previous live executable byte-for-byte (SHA-256
  `15b2660cc8e35a2fe9e358cc04241f115b7aab386ffb4faf83e0329ef94dc192`).
  Only `app.js`, `app.css`, and map-control/cache markup in `index.html`
  changed; concurrent login, assistant, organization and backend edits are
  excluded. All 72 served assets, configuration and network contract verified;
  all 16 unrelated running service start times unchanged.
  All 62 desktop/tablet geography tests and the build pass. Real desktop/tablet
  login verifies both map window controls, Escape, navigation cleanup, and
  World Map Fit with no browser errors or Simulator API requests.
  The production snapshot is disconnected with zero map actors and seven
  reported errors: populated-map acceptance is covered by regression fixtures,
  not a live connected network. Backend state was not changed.
  Initial live smoke assumptions about populated maps caused automatic
  rollbacks; corrected empty-map checks passed. Stopped
  `zpr-control-room-before-map-maximize` retains rollback.
  Room sessions reset; operators must sign in again. No Simulator/backend
  deployment, commit or push is included.

- Visa API TCP-connect recovery packaged and exercised 2026-10-10 UTC.
  PH SHA-256:
  `c49c9a5334069c6d5aadd45fed9d87a5e8ce073f4271e5ec360052acce8d24c9`.
  The previous PH executable was reconstructed byte-for-byte before overlaying
  only a ten-second TCP dial deadline. The existing reconnect loop now retries
  stalled dials instead of waiting for the operating system's SYN timeout;
  shutdown remains interruptible. All four libnode2 tests and 551 PH test
  executions passed (four PH tests ignored). Packaged PH and `zpr-multinode:latest`
  persist the change; `zpr-multinode:before-tcp-connect-timeout-20261010` and
  private binary backups retain rollback. Activation failure diagnostics now
  report node identities, last contact and pending visa operations without
  weakening the all-configured-nodes synchronization requirement.
  The live fail-fast matrix confirmed Great Lakes activation completed:
  Shenzhen and Tijuana each timed out their initial stalled dial and
  authenticated/registered VSS within approximately seventeen seconds of PH
  startup. All three published Great Lakes scenarios passed, including the
  157-step workday. The matrix then stopped before Load Lab scenarios because
  Load Lab had retained identity files but no recoverable Visa database;
  activation correctly refused an implicit policy reset. The runner's explicit
  Great Lakes restoration subsequently failed readiness with Shenzhen absent
  from live synchronization, despite repeated bounded dials. Its script rollback
  ultimately recovered all three API/VSS pairs and bilateral peer links.
  Thus the TCP stall is fixed and Great Lakes acceptance passed once, but
  repeatable activation/restoration is not yet certified; this is not a full
  matrix pass. Report and diagnostic artifacts are retained privately. No
  Load Lab state was erased or recreated.

- PH visa-expiration worker deployed and validated 2026-10-10 UTC.
  PH SHA-256:
  `4becee67682b81b3e0535e97115b1897eb98684365c3649e84b1d84bc1bcd5dc`.
  Root cause traced during isolated Shenzhen restart: Milwaukee repeatedly
  matched expired visa `1518` for VS->VSS flow `(fd5a:5052::1 -> fd5a:5052:90de::11, tcp 8183)`
  while Shenzhen had already dropped that visa, causing repeated bind rejects
  and persistent VS socket `SYN-SENT`. The stale entry survived because PH
  expiration cleanup ran in tests but not in the live node process.
  Fix: added a one-second PH expiration worker and guarded expiration queue
  handling so an older queue entry cannot revoke a reinstalled same-ID visa
  with a later expiry. New regressions cover stale-match replacement and
  reinstalled-ID safety. PH tests passed (267 lib + 288 bin; four ignored).
  After deploying only this PH change, two consecutive fail-fast Great Lakes
  matrix executions passed all three published scenarios with restoration
  `restored-selection`, and a direct Shenzhen restart no longer selected stale
  visa `1518` (Milwaukee matched fresh visas `1524` and `1725` instead).
  Runtime remains synchronized across all three nodes with pending install/revoke
  counts at zero. This validates the Great Lakes restart path; Load Lab remains
  blocked by missing recoverable Visa DB and was not reset.

- Detached tray rename, actor attachment details, and duplicate actor-inventory
  resolution deployed to Control Room and Control-Service at 2026-10-10
  06:12 UTC. Running image for both:
  `sha256:7033868be58dbe43b55b62d71ce3d2b30ce50595e427061277ee83ed71bf4bbd`.
  HTTPS verification confirms the Detached summary, `app.js?v=153`, and the
  Attachment detail section. The live snapshot is connected with no errors,
  17 actor identities, and no duplicate common names. Actor-inventory and
  snapshot tests passed; 14 desktop/tablet tray checks passed before rollout.
  Existing Great Lakes Admin/LDAP endpoint configuration was retained.
  Simulator and all three node start times were unchanged. Room sessions reset
  on restart. No Simulator rollout, commit, or push is included.

- Services simplification deployed to Control Room at 2026-10-10 06:06 UTC
  as `zpr-editor-ui:20261010-services-simplified`, verified at 06:06:40 UTC.
  Removed the introductory paragraph, provisional service/log filter and
  Services metrics display; retained service selection, source health and logs.
  Nodes metrics and the latest Unattached map tray remain unchanged.
  Reconstructed the live executable byte-for-byte (SHA-256
  `76fa4da10e04deadce78e7ca214b9e84ec1d4134ea7f5d0f9ab23ce485d4910f`)
  before overlaying only `diagnostics.js` and Services markup/cache reference
  in `index.html`. The concurrent actor-inventory backend change is excluded.
  All 72 served assets, container configuration and network contract were
  verified; all 19 unrelated service start times were unchanged.
  Real Dex login and Services selection/health/logs were verified on desktop
  and tablet with 20 production source entries, no filter/metrics, map/Fit and
  no Simulator API requests. All 18 focused browser cases and the build pass.
  An initial smoke assertion incorrectly used the selector's combined
  name/identity as the log title; automatic rollback restored the baseline.
  Corrected verification uses the production source name, and the retry passed.
  Stopped `zpr-control-room-before-services-simplified` retains rollback.
  Room sessions reset; operators must sign in again. No Simulator, backend,
  provisioning, commit or push changes are included.

- Unattached map tray deployed to Control Room at 2026-10-10 05:55 UTC using
  the documented `restart-control-room` command. Running image:
  `sha256:b816b6bb4d12adbdb8e52ebd47b147a571bf7f6c748f01a691147a3b55c4a928`.
  HTTPS checks confirm `app.js?v=152`, `app.css?v=94`, and the Unattached tray
  markup/function are served. Both Topology and World Map use the tray; isolated
  nodes remain visible and unattached adapters do not affect Fit.
  All 58 desktop/tablet geography tests passed before rollout. Control-Service,
  Simulator (including its launch-queue recovery), and all three Great Lakes node
  start times were unchanged. Room sessions reset on restart. No Simulator
  deployment, commit, or push is implied by this entry.

- Simulator launch-queue recovery deployed 2026-10-10 at 05:32:17 UTC as
  `zpr-simulator:20261010-launch-queue`. Executable SHA-256:
  `b7c84cc78ffc7a87a563ae2023d4830242ad75d2811e19ffea79006846bad97d`.
  Reconstructed the previous Simulator executable byte-for-byte
  (`d4d9e8a7b0908fba540f4aacb3893cd8adedbde3c6736fe0c324f5fb12b6dcad`)
  before overlaying only the scenario scheduling fix. All 62 embedded UI assets,
  container configuration, mounts, ports and organization settings are unchanged;
  all 19 unrelated running service start times were preserved.
  Stopped `zpr-simulator-before-launch-queue-20261010` retains the rollback.
  Machine launches remain serialized through capacity checks and provisioning,
  but acquire a cancellable slot before starting their execution deadline.
  Queue time remains included in recorded elapsed time; other action deadlines
  are unchanged. All 39 scenario-file tests pass, including queue-budget,
  cancellation and failed-launch slot-release regressions; targeted scenario
  race suites pass in both current source and the deployed baseline.
  The full Great Lakes workday ran from 05:32:30 to 05:37:12 UTC and completed
  all 157 steps with no failures: twelve launches, twelve controller-readiness
  checks, twelve logins, three DNS resolutions, thirty service requests returning
  HTTP 200, and three expected-denial traffic probes. All 34 cleanup steps
  passed, including twelve machine stops; no scenario machines remained running.
  The live browser displayed completed, 157 / 157. At 05:37:26 all three nodes'
  API/VSS socket pairs remained established and every node-peer link was Active
  bilaterally, without restarting VS or the nodes for this run. No VS timeout/
  VSS-failure log lines occurred during this workday. This establishes a passing
  end-to-end workday after the earlier independent Tijuana restart recovery;
  it does not explain the intermittent Redis stalls observed before this run
  or certify indefinite stability. No policy, identity or Redis durability
  settings were changed.

- Great Lakes reconnect follow-up deployed 2026-10-10 at 05:10 UTC.
  Visa Service SHA-256:
  `84bf016b9aa7797efb31c9c87f01a49a30f90638efe03ae0f158b21c65ae4d89`;
  PH SHA-256:
  `3345d72943daf74dfb110830df60f019e43cbdbcc81151cc80287599ca74b39c`.
  Packaged binaries and `zpr-multinode:latest` persist these versions.
  The three node containers and VS adapter now use matching PH versions;
  dashboard releases, organization identities, policy and Redis persistence
  configuration were not changed. Rollback images
  `zpr-multinode:before-vss-recovery-20261010` and
  `zpr-multinode:before-transit-support-20261010` and private backups remain.
  VS checks bootstrap eligibility even with both endpoint actors retained,
  invalidates a failed VSS worker's own live-session generation, and revokes
  pre-reset normal visas on live relays while preserving/reinstalling key-free
  bootstrap. Cached bootstrap visas missing path-node references are replaced.
  PH supports forwarding-only node transit without endpoint classifiers;
  malformed visas and adapter use are rejected explicitly. All 499 VS tests
  and 551 PH test executions passed (four PH tests ignored); the optional PH
  security-testing feature compiles. Changed-file rustfmt and diff checks pass.
  Tijuana-only forced process failure at approximately 05:11:55 recovered
  VSAPI authentication at 05:12:09 and VSS at 05:12:12, without restarting VS
  or the other nodes. By 05:13:15 all three nodes had live API/VSS sockets and
  all peer links were Active bilaterally. The workday starting 05:13:31 failed
  at 05:15:26 during machine creation, not remote controller readiness:
  machine-02's launch was killed and machine-05, machine-09 and machine-01
  exhausted their deadlines. Five endpoint logins completed, including
  machine-08/Tijuana, and all twelve stop-machine cleanup steps completed.
  At 05:16 UTC all three API/VSS connections remained established and all
  peer links remained Active bilaterally. Source inspection shows machine
  launches are serialized by `scenarioMachineStartMu`, while parallel step
  deadlines begin before acquiring that mutex; queued launches consume their
  90-second execution budgets waiting for earlier Docker launches. No
  Simulator scheduling change was deployed in this recovery. Full workday
  traffic certification is still incomplete. Earlier idle/runtime Redis
  timeouts also remain unexplained.

- Great Lakes bootstrap recovery deployed 2026-10-10 UTC. This changes the
  local Visa Service and three PH node binaries, not the dashboard releases.
  Existing Redis policy/state and organization identities were retained.
  Visa Service SHA-256:
  `fe0e369cfb099b779c38204860daae17e43f644532cdd059202d593728aab5fa`;
  PH SHA-256:
  `7a4489df74990f9c665b4b7ab4ed098e37e11f436a1c0b058ce0af6edfd87174`.
  The packaged binaries and `zpr-multinode:latest` contain the updates;
  existing runtime containers received the binaries in their writable layers.
  Rollback image `zpr-multinode:before-bootstrap-key-fix-20261010` retained;
  database and binary backups are private session artifacts.
  VS revoked bootstrap visas 1177-1180 with retained ephemeral keys and
  authenticated both remote nodes without clearing the database. PH now
  accepts late successful node-peer Hello responses and returns installation
  errors for unsupported forwarding-only or malformed full visas instead of
  panicking. All 495 VS tests and 547 PH test executions passed (four ignored);
  changed PH files pass rustfmt and whitespace checks. Workspace-wide VS
  rustfmt still reports a pre-existing difference in `connection_control.rs`.
  After a coordinated VS/node restart, all three peer links were Active at
  both ends and UP in the Admin snapshot. The workday run beginning
  02:17:43 UTC completed ten endpoint logins, including Shenzhen and Tijuana,
  and three HTTP requests, but failed on a later DNS lookup. During that run,
  Redis request timeouts and VS-adapter keepalive loss interrupted the control
  plane; subsequent cleanup also reported errors. Earlier runs passed expected
  denial probes but failed endpoint readiness. Full twelve-endpoint workday
  certification and independent node-only restart recovery remain incomplete.
  A synchronized snapshot alone is not sufficient: verify live PH processes,
  bilateral peer state and sustained endpoint traffic. At 02:21 UTC, all
  bilateral peer links remained Active and the Admin snapshot reported UP,
  but both remote PH logs still reported a closed VS API connection.
  This deployment does not establish sustained control-plane recovery.

- Diagnostics/Nodes ownership merger deployed to Control Room only, verified
  2026-10-10T00:50:31.511Z; started at 00:50:14.849567128Z.
  Exact live baseline executable SHA-256
  `f99c09562353f4cbabffb205342751a6970a465edc2092ae60874f371cbb2d49`
  reproduced before overlay. Only source-metrics.js v1, node-logs.js v2,
  node-stats.css v7, diagnostics.js v11 and captured-live index references
  changed. All 69 assets hash-verified; configuration, mounts, ports and
  network contracts preserved; 17 unrelated running container starts unchanged.
  All 96 desktop/tablet cases pass against exact release assets; preserved
  operator-auth race tests and embedded release build pass.
  Real Dex desktop/tablet checks verify retry/login, selected-node health and
  exact production metrics/logs, source isolation, pause/refresh/resume,
  window controls, Diagnostics aggregate health and 10 non-node detail sources,
  node selection/navigation, maps/Fit and no Simulator API requests.
  Current production inventory has two geographic anchors and zero docks;
  seven unconnected components retain exact 40-unit clearance. No claim of
  Great Lakes client connectivity is made. Initial verifier required historical
  dock presence and automatically rolled back; snapshot-grounded retry passed
  without application changes.
  Control-Service, Simulator, Great Lakes, enrollment, Change Review and Gateway
  runtime are excluded. Rollback `zpr-control-room-node-merger-rollback`
  retained stopped. No commit or push.

- Login-timeout retry correction deployed to Control Room only, verified
  2026-10-10T00:24:10Z; started at 00:23:50.411774626Z. Exact preceding binary
  SHA-256 `1f17a57c630e54758e40f7ee33707121ce67a25ef7ee891280119fe334c09010`
  reproduced before patch. Only the timeout-page form method/action changes:
  GET Control Room rather than POST login from the restrictive recovery page.
  All 68 frontend assets remain hash-identical. Exact-release desktop/tablet
  recovery tests and isolated operator-auth race tests pass. Real Dex checks
  verify expired callback retry, exact POST Origin, fresh provider login and
  successful authentication, plus maps, compact components, Nodes logs,
  Diagnostics and no Simulator API requests.
  Configuration, mounts and ports preserved; all 17 other running container
  start times unchanged. Control-Service, Simulator, enrollment, Change Review,
  Gateway runtime and remaining source changes excluded.
  Rollback `zpr-control-room-login-retry-rollback` retained stopped.
  No commit or push.

- Compact unconnected-component layout deployed to Control Room only, verified
  2026-10-10T00:15:16Z; container started at 00:15:00.242181257Z.
  Exact preceding live executable SHA-256
  `4bf9fe5cb0b68838f2de3365fbb322c157762eb246efd0d3f8d3d8de34c0ed7e`
  reproduced before overlay. Only app.js v148 and its captured live index cache
  link changed. All 68 served asset hashes verified; configuration/mounts/ports
  preserved and all 17 other running container start times unchanged.
  All 50 desktop/tablet map cases pass against exact release assets. Real Dex
  checks verify four production unconnected adapters packed at exactly 40-unit
  group clearance in both maps, three unchanged geographic anchors, viewport
  Fit, actual selected-node logs, shared window controls, existing Diagnostics
  and no Simulator API requests. Control-Service, Simulator, login recovery,
  enrollment, Change Review and Gateway runtime remain untouched.
  Rollback `zpr-control-room-compact-components-rollback` retained stopped.
  No commit or push.

- Viewport-aware Topology/World Map layout deployed to Control Room only,
  verified 2026-10-10T00:09:45Z; container started at 00:09:28.924716673Z.
  Reconstructed live Room executable matched SHA-256
  `032874934c12c10c857654c00d37705e7e0406bd8a2b74e551c1b2c91d779970`
  exactly before overlay. Only app.js v147 and its cache link in the captured
  live index changed. All 68 served asset hashes verified; configuration,
  mounts, ports and network settings preserved; all 17 other running container
  start times unchanged. This count includes Control-Service, which was not
  replaced in this rollout.
  All 44 exact-release desktop/tablet geography cases pass. Real Dex login
  checks verify landscape/portrait Fit aspect in both maps, three exact
  geographic anchors, variable docks, actual selected-node logs and shared
  window controls, aggregate Diagnostics and no Simulator API requests.
  Existing login recovery remains unchanged; pending timeout correction,
  enrollment, Change Review, Gateway runtime and Simulator excluded.
  Rollback container `zpr-control-room-viewport-layout-rollback` retained stopped.
  No commit or push.

- Selected-node logs/shared viewer and World Map variable-length docks deployed
  on 2026-10-09, verified at 21:04 UTC. Control-Service started at 21:03:57 UTC
  (`zpr-control-service:20261009-node-log-source`); Control Room at 21:04:00 UTC
  (`zpr-editor-ui:20261009-nodes-world-map`).
  Service overlay contains only the inventory-validated Diagnostics source
  filter. Room overlay changes six assets: app JS v146, machine-logs JS v27,
  node-stats CSS v6, new log-panel/node-logs JS v1, and captured live index
  additions/cache links. Aggregate Diagnostics is retained.
  Isolated builds preserve preceding backend behavior: Room reconstruction
  reproduces its entire preceding live executable exactly. Service reconstruction
  matches every ELF section, including compiled code/data, debug information,
  symbols and embedded assets, except the two build-ID note sections; its full
  executable hash therefore differs. Each service's historical Gateway validator
  error wording is preserved. Current enrollment, Change Review, Gateway runtime
  and newer login work is excluded; Simulator is not deployed.
  Exact-release expanded selection: 134 browser cases pass, two existing
  intermittent viewport Fit cases fail. Repeated checks against captured
  deployed map code reproduce the same 1.125 coverage failure (three of six);
  candidate repeats pass four of six. These tests and existing application code
  remain unchanged. Final approved Nodes/map selection passes all 66 cases,
  excluding documented baseline failures; staged Diagnostics tests pass with
  the race detector.
  Real Dex-authenticated desktop/tablet checks verify initial Loading without
  invented provider data, exact rendered bodies from two selected production
  nodes, source isolation, Pause/manual Refresh/Resume, shared standard window
  controls, Escape/focus restoration and navigation detachment. World Map has
  three exact geographic anchors and five docks with unequal lengths; zoom/Fit
  contains production components. Aggregate Diagnostics loads actual logs
  without errors. No JavaScript errors or Simulator API requests.
  All 68 Room served assets hash-verified. Both container configurations,
  mounts, ports, host settings and network sets preserved; all 16 unrelated
  running container start times unchanged. Source selection, empty/duplicate/
  unknown source rejection, unsigned API denial and existing login recovery
  verified. The first attempt hit a request-release race in the live verifier
  and automatically restored both preceding containers; corrected verifier
  retry passed. Retained stopped `zpr-control-service-node-log-source-rollback`
  and `zpr-control-room-nodes-world-map-rollback`. Room sessions reset on restart;
  sign in again. No commit or push.

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
