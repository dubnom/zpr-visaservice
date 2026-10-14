# GUI work

Active GUI backlog. Requirements and historical evidence were reorganized on
2026-10-09; this reorganization does not implement, deploy, commit or push work.

## Document map

- [Design guidelines](GUI%20design%20guidelines.md): durable presentation rules.
- [Completed work](GUI%20completed%20work.md): completed feature checklists and evidence.
- [Deployments](GUI%20deployments.md): dated release, verification and rollback records.
- [Organization editor plan](Organization%20editor%20plan.md): detailed creation/editing workflow.
- [Provisioning plan](../Provisioning%20plan.md): machine/adapter enrollment architecture.

## Tracking conventions

Use **Planned -> In progress -> Verified -> Deployed** for each deliverable.
Verified means implementation and required checks are complete, not that it is live.
Record commit/push state separately; deployments can contain uncommitted work.
Only check an implementation task when verified. Move completed requirements and
test evidence to the completed-work archive; put rollout evidence in deployments.
Keep this file focused on open deliverables rather than test logs or release history.
Do not infer current runtime health from a historical verification entry.

## In progress

Simulator organization-profile editor toolbar and mode alignment are **Deployed
to Simulator** (2026-10-14 04:58 UTC): exclusive Raw/Form views,
standard File.../Browse..., Analyze, Format
and Wrap controls, History beside the identity, and a multiline Description.
Draft saves, private-definition export, reviewed AI changes and optimistic
revision protection are preserved. The 48 selected desktop/tablet profile,
definitions, duplicate/new/import regressions and embedded dashboard build pass.
The existing definitions-history test has a stale Close-button selector and
was excluded in both viewports. All 74 served assets match the worktree; see
[deployment evidence](GUI%20deployments.md). Not committed or pushed.

Editor gutter synchronization and Trusted Source insertion undo are **Verified**
locally. Assertion/Policy gutter rows track source-line changes while clearing
obsolete analysis; catalog insertions and shared programmatic edits participate
in Undo/Redo. Ninety focused desktop/tablet regressions and the embedded
dashboard build pass. See
[verification evidence](GUI%20completed%20work.md#editor-gutter-synchronization-and-undo).
Not deployed, committed or pushed.

The independent Control Room DNS statistics transport is **Deployed**
(2026-10-13 23:05 UTC). Control-Service now reads an operator-owned relay on a
private Docker network, not Simulator's internal relay. Status, query-counter
and zone endpoints pass live mutual-TLS checks, including while Simulator is
paused. The local relay uses Docker administrative access; production still
requires an authorized ZPR-connected origin. See
[deployment evidence](GUI%20deployments.md). No commit/push for this repair.

The Simulator shared status banner is **Deployed** (2026-10-13 19:32 UTC).
It shows backend connectivity, active organization, scenario/run state and
signed-in identity across Simulator pages. All 36 focused desktop/tablet
Help/banner tests pass; live Organizations and Scenarios checks confirmed the
connected/idle state and `User` label. See
[deployment evidence](GUI%20deployments.md).

Simulator GUI design-guideline fixes are **Deployed** (2026-10-13 12:23 EDT):
graph Maximize/Restore, Activity metrics, Help/top-bar placement and the 11px
text minimum. The 18 focused desktop/tablet regressions pass. See
[deployment evidence](GUI%20deployments.md).

Map Maximize/Restore is **Deployed to Control Room and Simulator**
(Control Room 2026-10-11; Simulator 2026-10-13). Both maps share window
controls, Escape restoration and navigation cleanup. All 62 geography tests
and the embedded build pass; live desktop/tablet checks verify empty-map
controls. The production Visa snapshot was disconnected during those checks.
See [deployment evidence](GUI%20deployments.md).

Simulator organization definition/editing is **In progress**.
The draft/source-editor slices are **Deployed to Simulator** (2026-10-11 20:18 UTC) and add existing-organization profile drafts, name/description and
JSON editing, validation, immutable history and AI before/after review with
guarded local Undo/Redo. Saving does not alter source files, linked documents or
runtime. Bundled-definition **Duplicate -> review -> create private draft ->
AI edit -> save -> validate** is also deployed. Copies are stored as
unpublished coherent bundle snapshots, separately from runnable organizations.
Passwords/certificates and live/session/enrollment/run state are not copied.
Source base DN, desired addresses and identifiers remain explicit review items.
Source-read/target-create scopes, stale previews, concurrent IDs, persistent
creation receipts and lost-response reconciliation are covered.
**New/Import -> review -> create private incomplete draft -> AI edit/save ->
validate gaps**, plus portable saved-definition export, are deployed.
Missing sections do not fabricate runtime configuration or block draft saves;
malformed supplied configuration, unsafe definitions and detected credentials
are rejected. Creation receipts reconcile uncertain responses and concurrent IDs.
Deployed **Edit supporting definitions** provides profile/document selection,
source editing, add/remove and typed paths/kinds, atomic whole-bundle saves,
coherent history and reviewed cross-document AI with guarded Undo/Redo.
Private profile history opens the matching complete bundle. Conflicting or
uncertain saves retain local edits; explicit reload reconciles persisted state.
AI discloses the full unsaved fictional bundle and respects bounded context;
no creation receipts or runtime data are sent.
Structured section editors (Sites/Topology, Directory, Services/Workloads) with
bi-directional Raw/Form synchronization, live topology cards, and live
`<ldap-org-graph>` preview are **Verified** locally.
Full reference/compiler checks, section-only AI context and activation review
are next; production administration remains separate.
Live desktop/tablet checks confirm private revision persistence across Simulator
replacement. Great Lakes/Load Lab supporting-definition mounts are read-only;
no shared staging or runtime activation changed. See
[deployment evidence](GUI%20deployments.md). No commit/push.
See [the execution plan](Organization%20editor%20plan.md).
34 desktop/tablet organization/profile/definitions/duplication browser cases pass
(including newly added structured editor sync and preview tests), along with
focused Go race tests and build/vet/syntax/editor checks. Full `make test` still
fails on Redwood's LDAP identity expectation and assistant setup's unbound
`SCRIPT_DIR`; neither is changed by this slice.
No deployment, activation, commit or push.

Services selector and standard log display are **Deployed** (2026-10-10).
Services shows one selected non-node service/trusted source's health and logs.
The provisional filter, metrics display and introductory text are removed.
Nodes metrics and backend contracts are unchanged. Eighteen focused browser
cases and the build pass; live desktop/tablet login, selection and logs were
verified. See [deployment evidence](GUI%20deployments.md).

The Diagnostics/Nodes ownership merger is **Deployed to Control Room**
(2026-10-10 00:50 UTC).
Nodes owns selected-node source health, exact sortable metrics and logs.
Diagnostics retains aggregate/source overview and non-node service details,
with node rows navigating to Nodes rather than duplicating their detail panels.
96 desktop/tablet merger/shared-viewer regression cases and the embedded
dashboard build pass. The existing production source-filter backend is reused;
rollout contains the new shared metrics asset and updated Room assets only.
Real desktop/tablet checks verify node health/metrics/logs, aggregate health,
10 non-node detail sources and node navigation. No commit or push. See
[completed work](GUI%20completed%20work.md#diagnosticsnodes-ownership-merger).

Compact unconnected-component placement is **Deployed to Control Room**
(2026-10-10 00:15 UTC).
Undocked adapters sit beside occupied bounds in both maps rather than the
distant staging grid, with service/cloud clearance and viewport-aware packing.
`app.js` v148. See
[verification evidence](GUI%20completed%20work.md#compact-unconnected-components).
See [deployment evidence](GUI%20deployments.md). No commit or push.

Viewport-aware Topology/World Map layout is **Deployed to Control Room**
(2026-10-10 00:09 UTC).
Landscape/portrait node rows, free-host columns and adapter placement follow
the graph area's dimensions; geographic anchors and manual cameras remain
preserved. `app.js` v147. See
[verification evidence](GUI%20completed%20work.md#viewport-aware-map-layout).
See [deployment evidence](GUI%20deployments.md). No commit or push.

Login-timeout retry origin/CSP correction is **Deployed to Control Room**
(2026-10-10 00:24 UTC).
The single retry button returns to Control Room before a fresh login POST;
it no longer POSTs from the restrictive timeout page. Strict origin checks
remain unchanged. See [verification evidence](GUI%20completed%20work.md#login-timeout-retry-correction).
See [deployment evidence](GUI%20deployments.md). No commit or push.

World Map variable-length adapter docks are **Deployed to Control Room**
(2026-10-09 21:04 UTC).
Nodes remain geographically anchored; each attached adapter's distance accounts
for its own service ring/gateway cloud rather than a shared radius. Topology
layout is unchanged. See the [verification evidence](GUI%20completed%20work.md#world-map-variable-dock-lengths).
See [deployment evidence](GUI%20deployments.md). No commit or push for this rollout.

Selected-node logs on Nodes are **Deployed to Control Room/Control-Service**.
The shared log-panel component and Worker-panel assets are **Deployed to
Simulator** (2026-10-13 19:32 UTC). Nodes, Adapter/Controller
Logs and Workers share rendering, scrolling and window controls. Nodes queries
an inventory-validated production Diagnostics source rather than Simulator or
the all-source log budget. The deployed merger above moves node details to
Nodes while keeping
aggregate health and non-node details. 96 desktop/tablet browser cases, focused
production Diagnostics/log-boundary race tests and the embedded build pass.
Control Room assets and the Control-Service source filter are deployed; the
Simulator top-banner release also deployed the shared Worker-panel assets. See
[completed work](GUI%20completed%20work.md#selected-node-logs-and-shared-viewer).

Shared JavaScript page-runtime reuse is **Deployed to Control Room and Simulator**.
See the [implementation and test evidence](GUI%20completed%20work.md#javascript-reuse-refactoring).
Control Room rollout: 2026-10-09 19:13 UTC; Simulator rollout: 2026-10-13.

Control Room header guideline cleanup is **Deployed** (2026-10-09 19:13 UTC).
See the [completed guideline work](GUI%20completed%20work.md#control-room-guideline-follow-up).

Consistent Maximize/Restore window icons across Control Room and Simulator are
**Deployed to Control Room and Simulator**. See the
[completed guideline work](GUI%20completed%20work.md#control-room-guideline-follow-up).

Window-control color/style consistency is **Deployed to Control Room and Simulator**:
editor and log controls share dark icons on white 32px buttons and identical
hover, active, disabled and keyboard-focus styles (`app.css` v93,
`safe-display.js` v3). All six editors and adapter/controller/Worker logs are
covered by 24 passing desktop/tablet runtime/header/window cases; embedded build
and editor diagnostics pass. Control Room rollout verified 2026-10-09 19:24 UTC;
Simulator assets deployed 2026-10-13.

Diagnostics ANSI log colors are **Deployed to Control Room** (2026-10-09 19:29 UTC).
Diagnostics and adapter/controller/Worker logs share the safe colored-text
renderer; 48 desktop/tablet checks and the embedded build pass. See the
[completed guideline work](GUI%20completed%20work.md#control-room-guideline-follow-up).

The shared Worker-log renderer is now deployed to Simulator with the
2026-10-13 top-banner release; earlier pending-deployment notes below describe
the prior rollout state.

Diagnostics body-only/raw/full-space layout and initial-loading cleanup are
**Deployed to Control Room** (2026-10-09 19:45 UTC):
no duplicate timestamp/level columns, Format JSON or recurring Querying/Updated
line; full available width and natural-height content; initial Loading only.
The isolated embedded build passes using the exact preserved live backend;
16 desktop/tablet cases pass against the release assets. Real authenticated
desktop/tablet checks verify provider logs, layout and loading behavior.
Only three frontend assets changed; backend services and Simulator untouched.
No commit or push. See the [release evidence](GUI%20deployments.md) and
[completed guideline work](GUI%20completed%20work.md#control-room-guideline-follow-up).

## Next

The open requirements below retain their original scope.
No new priority or implementation approval is implied for backend rollouts.

### Config
- [x] Create a full draft-only form editor synchronized with raw TOML. Deployed to Control Room and Policy-Service at 2026-10-13 05:49 UTC; verified without runtime changes. See [completed Config work](GUI%20completed%20work.md#config) and [deployment evidence](GUI%20deployments.md#config-form-editor-2026-10-13-0549-utc).

### Logs and diagnostics
- [ ] Expose the shared viewer for Services and Trusted Sources; move their metrics/freshness/errors to suitable detail views before retiring the aggregate Diagnostics page.

### Gateways
- [ ] Deploy the committed forwarding/consolidation changes through a separately reviewed rollout; preserve production settings and verify the actual gateway binary and protocol behavior, not just UI assets.
- [ ] Unify deployed Gateway runtime configuration with the editor contract and migrate fixed-upstream callers before retiring that mode. Define enforcement capabilities for opaque HTTPS tunnels; do not imply draft methods/paths are enforced there.
- [ ] Improve individual path-prefix management and review wildcard requirements against draft validation and runtime matching before adding support. The live web proxy's host patterns do not imply wildcard support in saved gateway drafts.
- [ ] Add verified external-network policy metadata, gateway runtime health, and explicit reviewed activation/rollback after the production provisioning contract is implemented.

### Provisioning
- [ ] **Mac pilot first:** provision one Apple Silicon Mac (macOS 13+) through
  the selected Great Lakes invitation to live ZPR connectivity. The user can
  operate the laptop and authorize administrator prompts. Complete trusted
  credential issuance, enrollment-key-preserving runtime handoff, adapter
  admission, allowed/denied traffic checks, and route/DNS rollback verification.
  Do not consume the invitation until issuance and delivery can complete; no
  enrollment code belongs in this tracker. See the
  [Mac pilot blockers](../Provisioning%20plan.md#first-release-decisions-and-progress).
  The selected handoff uses a signed root helper, one-time local IPC, and
  direct System Keychain import of the separate runtime key after one admin
  authorization. Private enrollment-backed BAS implementation is local only;
  signed runtime-key import approval delivery is implemented locally with
  pinned Ed25519 verification and native-only receipt retention. Adapter
  configuration delivery, helper/runtime integration, and live rollout remain
  open. The BAS client ID/subject derives from inventory asset ID.
- [ ] After the Mac pilot passes, certify native Windows install/browser launch,
  DPAPI/ACL and two-user isolation; sign and deliver releases. No Windows
  adapter/driver/service or credential issuance is included in the enrollment
  installer.
- [ ] Certify Mac Finder/Terminal/browser permissions, Developer ID signing/
  notarization and supported OS versions; complete full privileged traffic/lifecycle
  tests. This remains broader release certification after the single-Mac pilot.
- [ ] **Deferred security issue:** prevent invitation creators from approving
  their own device requests before multi-operator production use. The initial
  single-operator setup currently allows this; named-user authorization,
  revision/key checks and audit remain required. Enforce separation in the
  Enrollment Service transaction, test it, and keep any future single-operator
  exception explicit and audited. See the [security hardening tracker](zpr-dashboard/SECURITY_HARDENING.md#p1-enrollment-approval-separation-of-duties).

## Blocked / needs a decision

- Gateway forwarding rollout requires separate review and actual binary/protocol verification; saved drafts do not imply runtime enforcement.
- Gateway activation depends on the independent production provisioning contract.
- Native platform certification needs the relevant OS environments and release signing; local/ad-hoc builds are not certified releases.
- Self-approval is a deferred risk accepted for the initial single-operator deployment; do not expand to multi-operator production use until the security-hardening item is verified.

## Larger planned work

- [ ] Multi-site BIND 9 DNS resilience for Great Lakes: plan DNS instances in
  Milwaukee, Shenzhen and Tijuana, with one primary and authoritative
  secondaries for internal ZPR zones, authenticated AXFR/IXFR transfers and
  NOTIFY, and site-local recursive resolution with multiple client resolver
  addresses. Define independent production configuration and ZPR policy for
  DNS queries and zone transfers; do not depend on Simulator. Verify zone
  update propagation, actual client failover, primary/site outages, secondary
  zone expiry and recovery. BIND configuration and recursive caches are not
  replicated automatically. Anycast is an optional separate routing/health
  design decision, not assumed in this task. Track only; no deployment yet.
- [ ] Organization creation/editing: Duplicate -> edit draft -> validate -> review activation first. The [detailed plan](Organization%20editor%20plan.md) owns the individual requirements; production administration remains independent of Simulator.

## Simulator

### Trusted sources
- [ ] Remove extra line between the top header and the source title.
- [ ] Put count of people, groups, and attributes to the right of their horizontal nav labels.

## Control room

### Trusted sources
- [ ] Make the simulator and control room more aligned. The control room is the "live" view, so it needs "Updates (24h).
- [ ] 
