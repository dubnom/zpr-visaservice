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

Services selector and standard log display are **Verified; awaiting deployment**.
The main Services page selects one non-node service/trusted telemetry source
and shows its health, sortable metrics and shared log viewer. Aggregate service
health/counts and filtering remain; the Status service inventory is unchanged.
Pause/Resume, paused manual Refresh, Wrap, standard maximize/Escape and
per-service scroll retention reuse the existing viewer/runtime contracts.
No backend or Simulator change, deployment, commit or push. See
[verification evidence](GUI%20completed%20work.md#services-selector-and-standard-log-display).

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

Selected-node logs on Nodes and the shared log-panel component are
**Deployed to Control Room/Control-Service; Simulator awaiting deployment**
(2026-10-09 21:04 UTC). Nodes, Adapter/Controller
Logs and Workers share rendering, scrolling and window controls. Nodes queries
an inventory-validated production Diagnostics source rather than Simulator or
the all-source log budget. The deployed merger above moves node details to
Nodes while keeping
aggregate health and non-node details. 96 desktop/tablet browser cases, focused
production Diagnostics/log-boundary race tests and the embedded build pass.
Control Room assets and the Control-Service source filter are deployed;
shared Worker-panel rollout remains pending. No commit or push. See
[completed work](GUI%20completed%20work.md#selected-node-logs-and-shared-viewer).

Shared JavaScript page-runtime reuse is **Deployed to Control Room; Simulator awaiting deployment**.
See the [implementation and test evidence](GUI%20completed%20work.md#javascript-reuse-refactoring).
Control Room rollout: 2026-10-09 19:13 UTC. This work has not been committed or pushed.

Control Room header guideline cleanup is **Deployed** (2026-10-09 19:13 UTC).
See the [completed guideline work](GUI%20completed%20work.md#control-room-guideline-follow-up).

Consistent Maximize/Restore window icons across Control Room and Simulator are
**Deployed to Control Room; Simulator awaiting deployment**. See the
[completed guideline work](GUI%20completed%20work.md#control-room-guideline-follow-up).

Window-control color/style consistency is **Deployed to Control Room; Simulator awaiting deployment**:
editor and log controls share dark icons on white 32px buttons and identical
hover, active, disabled and keyboard-focus styles (`app.css` v93,
`safe-display.js` v3). All six editors and adapter/controller/Worker logs are
covered by 24 passing desktop/tablet runtime/header/window cases; embedded build
and editor diagnostics pass. Control Room rollout verified 2026-10-09 19:24 UTC;
no commit or push.

Diagnostics ANSI log colors are **Deployed to Control Room** (2026-10-09 19:29 UTC).
Diagnostics and adapter/controller/Worker logs share the safe colored-text
renderer; 48 desktop/tablet checks and the embedded build pass. See the
[completed guideline work](GUI%20completed%20work.md#control-room-guideline-follow-up).

Shared Worker-log renderer rollout to Simulator remains pending; no Simulator
restart, commit or push was performed.

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
The Config form editor is the next larger GUI item.
No new priority or implementation approval is implied for backend rollouts.

### Config
- [ ] Create a form editor.

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
  credential issuance, non-exporting Keychain-to-runtime handoff, adapter
  admission, allowed/denied traffic checks, and route/DNS rollback verification.
  Do not consume the invitation until issuance and delivery can complete; no
  enrollment code belongs in this tracker. See the
  [Mac pilot blockers](../Provisioning%20plan.md#first-release-decisions-and-progress).
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

- [ ] Organization creation/editing: Duplicate -> edit draft -> validate -> review activation first. The [detailed plan](Organization%20editor%20plan.md) owns the individual requirements; production administration remains independent of Simulator.
