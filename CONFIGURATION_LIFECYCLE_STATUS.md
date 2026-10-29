# ZPR configuration lifecycle: status and production gap

Recorded: 2026-10-29.

Status: Source/documentation assessment and requirements record, not runtime
qualification. The user explicitly clarified that **ZPR Config, distinct from
policy**, is the primary concern.

## Summary

The configuration edit/push/lifecycle work is not complete. In particular,
editing or saving a ZPR Config record does not establish that the corresponding
network or service configuration was deployed, loaded, or made effective.

Policy deployment is a separate, narrower implementation. Progress in policy
compilation, approval and guarded installation must not be used to mark ZPR
configuration management complete.

## What Was Inspected

The 2026-10-29 inspection used the main Visa Service checkout at short revision
`250c7d2`, rather than treating older planning documents as current code.
It did not inspect running services or rerun tests.

Source references below deliberately point to the inspected main checkout;
this record lives in the production-planning worktree. The repositories can
advance independently, so revalidate against a pinned candidate before release.

- [Config editor](../../zpr-visaservice/zpr-dashboard/cmd/zpr-web-dashboard/static/config-editor.js):
  checks supplied source, saves `configuration` records/revisions, uses
  `text/vnd.zpr.zplc`, and explicitly reports "runtime unchanged".
- [Structured configuration form](../../zpr-visaservice/zpr-dashboard/cmd/zpr-web-dashboard/config_form.go):
  transforms supplied TOML drafts, never installs runtime configuration, and
  is excluded by the `production` build tag in the inspected source.
- [Policy apply receiver](../../zpr-visaservice/zpr-dashboard/cmd/zpr-web-dashboard/policy_apply_service.go)
  and [status/readback](../../zpr-visaservice/zpr-dashboard/cmd/zpr-web-dashboard/policy_apply_status.go):
  guarded policy-specific installation/reconciliation, not general configuration
  application.
- [Gateway API](../../zpr-visaservice/zpr-dashboard/cmd/zpr-web-dashboard/gateway_api.go):
  contract discovery, validation and saved draft revisions.
- [Implementation documentation](../../zpr-visaservice/zpr-dashboard/README.md):
  distinguishes guarded policy-apply fixtures from production readiness and
  explicitly describes Gateway drafts as runtime-unchanged with no Activate action.

## ZPR Config Is Not Policy

The inspected editor stores ZPLC/TOML configuration drafts under the policy
repository API. That storage/API location does not make these policy rules,
and does not prove that the editor covers every node, adapter, Visa Service
or deployment configuration file.

Before implementation, inventory and distinguish:

1. Compiler configuration: inputs used when producing a policy candidate.
2. Network/system configuration: topology, addresses, endpoint/service attachment,
   bootstrap and related configuration where actually supported.
3. Component runtime configuration: Visa Service, nodes, adapters and services.
4. Deployment configuration: listeners, process/container lifecycle and placement.

The precise ownership and supported fields require a source audit. Do not
assume one existing TOML document is an authoritative universal configuration.
Likewise, a compiler-input change may require a new policy artifact without
being a runtime reload operation itself.

## Current Capability Matrix

| Area | Source-level status | Remaining boundary |
|---|---|---|
| ZPR Config authoring | Raw TOML editing, check, saved records, revision history; structured form exists with build-profile restrictions | Qualify shipped UI/API coverage and exact schema semantics |
| ZPR Config push | No demonstrated general production deployment path in the inspected editor/workflow | Target ownership, authenticated apply, revision preconditions and durable operation status |
| ZPR Config effective state | Saving explicitly leaves runtime unchanged | Independent readback, loaded/effective revision, drift and partial-target state |
| ZPR Config lifecycle | Not established as an end-to-end production feature | Reload versus restart, dependency ordering, interruption recovery and safe rollback |
| Policy | Saved revisions, compilation/stage, candidate-bound review and explicitly configured guarded apply/readback | Deployment qualification, broader rollout and supported rollback; not evidence of Config completion |
| Gateway | Checked, organization-scoped revisioned drafts | Runtime activation and confirmed applied state; no Activate action in documented draft workflow |
| Other domain controls | Some trusted-source and identity-link lifecycle operations exist | Domain-specific capabilities, not a unified configuration lifecycle |
| System orchestration | Component/evaluation tooling exists | Qualified production-wide change/recovery path, not developer or Simulator scripts |

This is a targeted assessment, not proof that no additional implementation
exists in other branches. Running deployment enablement remains unverified.

Documentation discrepancy: the inspected
[Change Review documentation](../../zpr-visaservice/zpr-dashboard/CHANGE_REVIEW.md)
still describes application as always unavailable, while current code has an
explicitly configured policy-only apply path. Resolve this when updating the
owning implementation documentation; do not conclude that general Config apply
exists from that policy-specific correction.

## Required ZPR Config Lifecycle

```text
Edit -> Validate -> Save immutable revision -> Resolve targets/dependencies
     -> Preview impact -> Review if required -> Prepare exact change
     -> Apply/reload/restart -> Read back effective state
     -> Observe health/drift -> Recover or explicitly roll back
```

Required distinctions:

- Saved, validated, approved, dispatched, installed, loaded and effective
  configuration are different states.
- Identify each target and the schema/software version it accepts.
- Bind operations to exact configuration revision/digest, expected current
  state, named actor and current organization/action authority.
- Define which fields can reload, require restart, alter connectivity or need
  separately coordinated policy/credential changes.
- Preserve secret references rather than embedding reusable credentials in
  ordinary draft, review or audit payloads.
- Report unsupported fields and incompatible targets explicitly.
- Persist operation identities and per-target results; lost replies are
  uncertain until readback/reconciliation, not implicit failures or rollbacks.
- Record desired versus observed loaded/effective revisions and drift.
- Define staged dependency ordering and behavior on partial application.
- A prior saved revision is not automatically safe rollback: check authority,
  compatibility, credential validity and external state.
- Preserve protected local recovery and prevent gateway/access changes from
  locking out administration; comply with [OPA-1](OPERATOR_ACCESS_PLAN.md).
- Keep Workflows as a presentation/orchestration layer over domain authority,
  not a second apply engine or an interim envelope protocol.
- Operate independently of Simulator and its manifests, sessions and scripts.

## Follow-Up Work and Exit Gates

1. **Inventory:** map every Config field/document to compiler/runtime/deployment
   owner, actual consumer, target and supported build/profile. List unknowns.
2. **Contracts:** specify validation, impact preview, permissions, revisions,
   apply/reload/restart, status/readback and recovery for one selected component.
3. **Vertical slice:** deploy one exact saved ZPR Config revision through production
   contracts and confirm its effective state without manual file copying.
4. **Failure qualification:** test stale revisions, unsupported fields, denied
   authority, lost replies, crash/restart and partial application.
5. **Recovery:** explicitly approve and test rollback/last-known-good restoration
   without reviving revoked credentials or silently reopening gateway ingress.
6. **Expand and package:** extend to the declared supported component set and
   ship runbooks, status/drift views and exact artifact acceptance evidence.

Do not silently expand the first slice into an all-component configuration
framework. Select its owner and scope after the inventory. No effort estimate
is asserted here because the runtime-configuration ownership audit is missing.

## Production Gate CFG-1

**Open: required production configuration lifecycle evidence.**

For each declared supported ZPR Config target:

- Demonstrate revisioned edit/check/save and truthful runtime-unchanged status.
- Apply an exact authorized revision through the production path.
- Independently verify effective revision/content and intended behavior.
- Verify denied, stale and invalid operations leave existing state intact.
- Exercise reload/restart requirements, lost-response reconciliation and
  interrupted operation recovery.
- Demonstrate drift/partial failure visibility and explicit safe rollback or
  documented restore-required boundary.
- Confirm operator recovery remains available under OPA-1 and Simulator absent.

Policy upload tests alone cannot satisfy CFG-1. Record each target's evidence
as passed, failed, blocked or not run; unimplemented support must be explicit.
