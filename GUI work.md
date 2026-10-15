# GUI work

Active GUI backlog, reconciled on 2026-10-15 UTC (2026-10-14 EDT) after the
Great Lakes business-service and Restore-feedback deployments.
Completed checklists and dated release evidence live in the linked records.

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

## Current status

- **Publication:** `e1b24ab` is the previous publication checkpoint. The
  this source checkpoint includes the 2026-10-15 reconciliation and pending work;
  dated "not committed or pushed" notes remain historical evidence. Publication
  does not deploy the discovery CLI, new Results navigation or backend services.
- **Deployed:** Simulator `zpr-simulator:20261015-restore-feedback` at
  2026-10-15 01:03 UTC, following the business-service release at 00:56 UTC;
  Control Room remains at its 2026-10-14 22:43 UTC GUI release. The UI-only
  feedback deployment did not restart production services. The user's separate
  Restore attempt did reprovision runtime components before rolling back.
  See [deployment scope and verification](GUI%20deployments.md).
- **Restore blocked:** Great Lakes Restore failed its assertion-settings
  verification and the previous runtime was restored and verified. A read-only
  probe after rollback found the correct organization/base DN but empty assertion
  source. Feedback is fixed and deployed; assertion seeding is not repaired,
  Restore was not retried, and business forwarding is not live-certified.
- **Completed GUI polish:** shared Help styling, Trusted Sources spacing/counts,
  scenario cards/folders, organization viewer File actions and consolidated
  New/Import/Duplicate, Activity cleanup, and Security Review headers.
  Control Room retains its live Updates (24h) feed and independent service
  contracts. See the [completed checklist](GUI%20completed%20work.md#control-room-and-simulator-gui-polish).
- **Verification limits:** Current native core Rust tests and shared socket
  utility tests pass. The full dashboard `make test` still has four known
  failing groups. Focused race checks and 16 selected desktop/tablet browser
  checks pass using the existing container runner; no full browser-suite pass
  is claimed. Control Room's
  post-restart browser check was signed out, not an authenticated live-data
  check. See the [publication verification](GUI%20completed%20work.md#publication-reconciliation-2026-10-15-utc).
  These records do not assert current runtime health.

## In progress

Scenario gateway response capture is **Deployed (live acceptance pending)**:
a Results section
shows HTTP status, request/final URL, elapsed time, bytes read, redacted bounded
headers and inert raw HTML/JSON/text (base64 for non-UTF-8). Observations survive
failed expectations and partial-body errors; empty expectations allow
observation-only fetches within existing destination restrictions. Six
desktop/tablet browser checks, focused Go race tests, vet and the embedded build
pass; the required full Go suite retains four unrelated failures. Results
currently belong to the current run only; durable archives and other result
types remain future work. Simulator and controller binaries were updated from an
isolated Results-only release on 2026-10-15 00:49 UTC; production services were
not restarted. The later business-service executable resolved the observed
manifest-unavailable error; live forwarding acceptance has not been run.
The isolated full suite has the same 13 failures as its clean base, including
external-runtime/seed-dependent failures. The subsequent Runner/Results tabs are
verified locally, not separately deployed. See the
[deployment record](GUI%20deployments.md#scenario-gateway-results-deployment-2026-10-15-0049-utc) and
[verification evidence](GUI%20completed%20work.md#scenario-gateway-response-results).

Migration network discovery is **In progress**; its standalone CLI slice is
**Verified** locally. It has explicit authorized TCP scope, optional conservative
protocol checks, versioned JSON export and imported adapter-listener enrichment.
Ping and reverse names now default on without gating TCP checks; JSON config
files supply options with explicit CLI overrides and command-line-only
authorization. Ping requires the macOS/Linux utility (disable it elsewhere).
Config targets are now a string list of IPs, CIDRs or inclusive dash ranges
(including IPv4 final-octet shorthand). Host and CIDR-size caps are removed:
compact scopes are deduplicated and iterated lazily under the existing
rate/worker/time limits. Version-2 reports/previews store exact planned counts
and full compact scopes, with host records only for reached/imported targets.
Focused race tests, vet, native build, Linux/Windows cross-builds and the
documented no-network preview pass. The follow-up removes the overall deadline
(former `duration` options are rejected) while preserving per-check timeouts and
Ctrl+C partial export, and adds stderr progress with terminal-width-aware
rendering and distinct client/host/open-endpoint counts. Current engine/CLI race
checks pass. This extension was verified against
IPv4/IPv6 loopback only; it does not repeat the earlier authorized subnet scan.
Control Room scanner selection, authenticated live adapter inventory and
reviewed migration drafts remain future work. Discovery must remain independent
of Simulator, and remote observations must not be conflated with host-reported
listeners or port-number service guesses. See the
[CLI contract](zpr-dashboard/cmd/zpr-discover/README.md) and
[verification evidence](GUI%20completed%20work.md#migration-discovery-cli).
The original slice was published in `e1b24ab`; follow-up progress/deadline changes
are included in the current publication. This standalone CLI was not deployed by
the dashboard-only releases.

Great Lakes business simulations are **Implemented, verified and deployed;
live acceptance blocked**. Seven stateful providers are configured for dedicated
`machine-13`, with complete department/site policy grants, directory groups,
provider identities and two 42-pair scenarios (84 total: 13 allow, 71 deny).
Denials require fresh tuple-specific Visa Service evidence, not timeouts.
Writable state is isolated by organization/generation/machine. Source tests,
race checks, runtime plus five standalone catalog compiler checks and native/
Linux builds pass. The software and controller prebuilds are installed, but the
failed Restore did not complete provisioning; neither matrix has run live.
See [completed implementation](GUI%20completed%20work.md#great-lakes-business-simulations-and-restore-feedback).

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
The 2026-10-14 22:35 UTC Simulator worktree deployment includes the structured
editors and toolbar/mode alignment; source is committed and pushed. Earlier
targeted browser evidence remains recorded, but no full browser-suite rerun or
runtime activation is claimed for this release. See
[deployment evidence](GUI%20deployments.md) and
[the execution plan](Organization%20editor%20plan.md).

## Next

The open requirements below retain their original scope.
No new priority or implementation approval is implied for backend rollouts.

### Logs and diagnostics
- [ ] Repair Great Lakes assertion seeding/verification through an independent
  production policy configuration contract; identify why source is empty before
  any reviewed Restore retry. Preserve rollback and record exact failure details.
- [ ] After a successful reviewed Restore, run both Great Lakes Business
  scenarios and verify all 84 expected decisions with actual forwarding and fresh
  denial evidence. Do not mark software deployment as live policy acceptance.
- [ ] Retain organization operation outcomes across Simulator restart; current
  progress/result feedback is in memory and local logs survive independently.
- [ ] Complete the remaining Trusted Sources viewer and metrics/freshness/error detail placement before retiring the aggregate Diagnostics page. The Services selector and shared log viewer are already deployed; see [completed Services work](GUI%20completed%20work.md#services-selector-and-standard-log-display).

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
