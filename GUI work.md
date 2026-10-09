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

No new implementation is started by this document migration. Work in another
session is not reconciled here until its outcome is recorded.

## Next

The open requirements below retain their original scope. Header cleanup is the
latest presentation request; the Config form editor is the next larger GUI item.
No new priority or implementation approval is implied for backend rollouts.

### Control Room guideline follow-up
- [ ] in top header, Visa service - Remove "Available" and move "Uptime ..." below, just like Control Coom. Remove connected from Control Room. and don't add it to Visa Service.

### JavaScript reuse refactoring
- [ ] Extract shared page polling/lifecycle helpers: start/stop on navigation, prevent overlapping requests, abort obsolete requests, reject late responses and retain last-good data with explicit errors.
- [ ] Consolidate HTTP/JSON error handling and cancellation; keep authentication, same-origin CSRF handling and mutation retry/uncertainty policies explicit.

### Config
- [ ] Create a form editor.

### Gateways
- [ ] Deploy the committed forwarding/consolidation changes through a separately reviewed rollout; preserve production settings and verify the actual gateway binary and protocol behavior, not just UI assets.
- [ ] Unify deployed Gateway runtime configuration with the editor contract and migrate fixed-upstream callers before retiring that mode. Define enforcement capabilities for opaque HTTPS tunnels; do not imply draft methods/paths are enforced there.
- [ ] Improve individual path-prefix management and review wildcard requirements against draft validation and runtime matching before adding support. The live web proxy's host patterns do not imply wildcard support in saved gateway drafts.
- [ ] Add verified external-network policy metadata, gateway runtime health, and explicit reviewed activation/rollback after the production provisioning contract is implemented.

### Provisioning
- [ ] Certify native Windows install/browser launch, DPAPI/ACL and two-user
  isolation; sign and deliver releases. No Windows adapter/driver/service or
  credential issuance is included in the enrollment installer.
- [ ] Certify Mac Finder/Terminal/browser permissions, Developer ID signing/
  notarization and supported OS versions; complete full privileged traffic/lifecycle
  tests, credential issuance and secure adapter-runtime handoff.

## Blocked / needs a decision

- Gateway forwarding rollout requires separate review and actual binary/protocol verification; saved drafts do not imply runtime enforcement.
- Gateway activation depends on the independent production provisioning contract.
- Native platform certification needs the relevant OS environments and release signing; local/ad-hoc builds are not certified releases.

## Larger planned work

- [ ] Organization creation/editing: Duplicate -> edit draft -> validate -> review activation first. The [detailed plan](Organization%20editor%20plan.md) owns the individual requirements; production administration remains independent of Simulator.
- General polling and HTTP/JSON consolidation remain open despite completed editor and Simulator migrations; see [completed substeps](GUI%20completed%20work.md#javascript-reuse-refactoring).
