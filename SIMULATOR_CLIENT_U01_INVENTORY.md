# Packet C — Simulator production-state inventory (U01)

Date: 2026-10-23. Source baseline: local beta
`4627e4cd99f15e1ee227f788574ab5055ee7a353`; branch
`maturing/simulator-client`. Source inspection and offline fixtures only: no live
Simulator, LDAP, endpoint, production API, database or service was accessed.

**U01 is not complete.** This packet inventories reachable legacy behavior and
adds one Simulator-only, opt-in fail-closed launcher guard. Existing setup paths
remain available; none is deleted or represented as an ordinary-client workflow.
No production handler, trusted-source backend, credential or runtime file changed.

## Boundary and classification

Read together with [ordinary-client M0–M4 and U01–U06](SIMULATOR_AS_ZPR_CLIENT_PLAN.md)
and [administrator migration §2, §5 and W4](SIMULATOR_ADMIN_MIGRATION_PLAN.md).
The earlier D1/D2/D3/D4 approvals and named-approval/TLS requirements remain in
force. A publicly reachable `/api/simulator/...` route is **not** a production
administration contract. An authenticated admin transport does not make its
upstream file injection, signing or service restart permissible.

- **Public API:** an existing production-owned API or supported admin client;
  its permissions, explicit target and fresh readback still matter.
- **Companion:** independently owned disposable lab LDAP/OIDC/DNS. Current
  launchers coupling these to production must migrate separately.
- **Simulator-owned:** scenarios, lab drafts, machines, workloads and their own
  execution/evidence storage, never company service state.
- **Prohibited bypass:** current code crosses the ordinary-client boundary;
  this is a migration classification, not permission to execute or delete it.

Paths below are relative to `zpr-dashboard/`. Function names are stable search
anchors; transitive launchers outside this repository are referenced, not
executed or claimed fully audited.

## Bypass / ownership table

| ID | Observed production-state establishment/use and evidence | Classification of current path | Replacement contract and dependency |
| --- | --- | --- | --- |
| C01 | Organization catalog, drafts and scenario revisions: `cmd/zpr-web-dashboard/organizations.go`, `organization_create.go`, `simulator_workspace.go`. | Simulator-owned | Retain lab definitions, but reference already permitted production organization/source IDs; no production discovery from these files. M0 B03, M3 U03. |
| C02 | Activate POST / Restore POST in `organizations.go:handleSimulatorOrganizationOperation` starts `organization_activation.go`; `SIMULATION_ORGANIZATION_RESET_SCRIPT` runs `activate-organization.sh` and commits selection only after success. | Prohibited bypass | Optional setup client calls independently configured production save/test/stage/apply/enrollment contracts with scoped setup identity; scenario selection stays local. M1/M2 before M3 U01/U02. Guard `U01-ACTIVATION`. |
| C03 | `activate-organization.sh:restore_organization_base` stops Policy-Service; backs up then removes policy SQLite/WAL/SHM, staged bundles, published LDIF and multinode directory seed/data/config. Deploy receives `ZPR_RESTORE_BASE`; Redis state backup is taken, not an API restore receipt. | Prohibited bypass | Scenario reset must not invoke this. Separate authorized production backup/restore/deletion and independent disposable-companion cleanup, with receipts and reconciliation. M3 U06, M4 P03, M6 R03. Guard `U01-RESTORE`. |
| C04 | `scripts/dashboard-stack.sh:restart_policy_context` exports `.assertions_source` into `policy-assertion-seeds`, and profile + manifest fixtures via `export-policy-fixtures.jq`; injects `ZPR_POLICY_ASSERTIONS_SOURCE_FILE` / `ZPR_POLICY_TEST_FIXTURES_FILE`. `main.go` also exposes `seed-assertions -assertions-db`; `policy_assertions.go:populateOrganizationAssertions` opens SQLite directly. The CLI is an available bypass, not proof activation currently calls it. | Prohibited bypass | Use production assertion settings/record revision APIs (`GET/PUT /api/assertions/settings`), preserving expected revision; production-owned test inputs must not be a runtime mount of scenario definitions. M2 C03/C04, M3 U01, W1 configuration owner. Guard `U01-ASSERTIONS`. |
| C05 | `trusted_source.go:simulatorTrustedSourceHandler` reads manifest/active organization, derives approved attributes, container/base DN/bind DN and invokes an LDAP reader. Activation configures trusted feeds from profiles and restarts Control-Service with that context. | Prohibited bypass when used as production provider | Production-owned organization-scoped source registry/read/test contracts with stable source identity, provenance and explicit unavailable state; never silently reuse this handler. M0 B02/B03, M1 S01–S06, M2 C01/C02; **trusted-source packet owns backend**. Guard `U01-SOURCES`. |
| C06 | Activation sets `ZPR_ASSERTION_LDAP_CONTAINER`, base/bind DN and feeds on Control-Service restarts; `dashboard-stack.sh` likewise supplies Policy-Service LDAP inputs. One-node/multinode deployment derives LDAP lifecycle from organization profiles. | Prohibited bypass | Production source config/secret references supplied by provider owner; external read-only verified LDAP independent of Docker/Simulator. Await M1 S01/S02 and M4 P03, no source-backend edits here. Guard `U01-SOURCES`. |
| C07 | LDAP/OIDC/DNS lab capabilities and trust bootstrap under `packaging/simulator/`; these may legitimately serve disposable lab clients. | Companion service | Retain behind independent supervision/configuration/storage; Simulator shutdown must not stop them. Real organization LDAP requires separate read-only consent, not a lab substitute. M0 B04, M4 P03/P05, M5 D02. |
| C08 | `directory_workspace.go` stores draft LDIF/revisions in Simulator workspace. | Simulator-owned for disposable lab drafts | Retain drafts and expected-revision checks; allow editing only explicitly owned synthetic lab data. Existing real organization/source selection must be read-only. M3 U03. |
| C09 | `directory_workspace.go:handleWorkspaceDirectoryPublish` writes `SIMULATION_PUBLISHED_DIRECTORY_DIR/<organization>.ldif` after revision checks; Restore deletes it. This is launcher-consumed publication, not a production source registration receipt. | Prohibited bypass as production setup | Independently owned companion import contract scoped to disposable data; company directory writes excluded. Production source registration references that provider rather than reading Simulator LDIF. M1 S01/S02, M3 U03, M4 P03. Guards `U01-SOURCES` / `U01-CONFIG`. |
| C10 | Policy-Service exposes records/revisions, test, check and `POST /api/policy/records/{id}/stage`; `GET /api/policy/operations/candidate` and `POST /api/policy/operations` provide guarded candidate/apply under explicit configuration and named delegation. See `policy_service.go`, `policy_apply_service.go`, `policy_apply_control.go`. | Public API | Reuse agreed W1 candidate identity, approver, operation ID, expected current digest/version and receipt contract; scoped setup identity must not become workload authority. Stage is not applied. M2 C04, M3 U01/U02/U06; no new Simulator-specific production API. |
| C11 | Activation composes source, merges runtime/bootstrap config, invokes local/container compiler with lab signing key, and writes `runtime.bin2`; `install_multinode_policy` loads deployed bootstrap include/key paths. | Prohibited bypass | Production-owned compile/sign/stage, then authorized guarded apply of the exact candidate; preserve compile-failure/no-stale-upload behavior until replaced. M2 C04, M3 U01, W1. Guard `U01-POLICY`. |
| C12 | `install_multinode_policy` invokes `vs-admin policies --path` over Visa admin TLS/API-key transport; gateway checks use `vs-admin services`. | Public API transport, **not** ordinary-client setup end to end | Service/admin APIs exist, but local signing/bootstrap mounts and gateway restarts remain C11/C18. Consume production guarded apply/status instead of giving Simulator broad VS/operator key access. M0 B03, M2 C04, M3 U02/U06. |
| C13 | Activation reads `/api/policy/context`, assertion settings/source/evaluate and snapshots; health/`in_sync` alone do not identify the applied candidate. Existing `policy_status_consumer.go` consume production `/api/policy/operations/current/{org}/{target}` and `/status/{org}/{target}/{operation}` with scoped mTLS, no install fallback; synthetic scenario preflight compares exact digest/target. | Public API | Reuse those existing read-only clients and distinguish recorded receipt from fresh exact version/digest. General activation still needs operation-specific reconciliation; no health-only success. M3 U06, W1 §5 applied-state contract. Guard `U01-RECONCILE`. |
| C14 | `simulator_provisioning.go` delegates `approve-and-provision` to configured packaged tooling. `packaging/demo-runtime/packaged-machine-runtime.py:compile_endpoint_policy` injects endpoint public key into bootstrap compiler config and attributes into `attrfile.json`, compiles/copies bundle; `provision_member` reloads VS and flushes attributes. Human approval/unique keys do not make this ordinary enrollment. | Prohibited bypass | Pre-enroll a small fixed set through supported enrollment admin/authentication tools; claim, named approval, issuance, delivery and observed admission remain distinct. No policy-file fallback. W2 producer/consumer agreement, M3 U04 (X04), U01; native helper qualification is not a demo prerequisite. Guard `U01-ADMISSION`. |
| C15 | Packaged provisioning retains separate identity volumes/fingerprints per endpoint and checks response bindings; `simulator_provisioning.go` separates start/stop/verify from approval. | Simulator-owned orchestration; keys remain endpoint-owned | Retain unique identity persistence and lifecycle checks, consume ordinary enrollment/admission observations, never export/clone private keys. M3 U04, W2; keeping this does not qualify C14. |
| C16 | Activation writes `organization-request.json` / `organization-runtime.json`, runtime policy/config files and marker flags; `dashboard-stack.sh` generates production service env, mounts runtime/config/signing paths, chooses per-org DB and restarts context. Transitive multinode `deploy-docker.sh` is outside this repository. Pure `simulator_runtime_plan.go` scenario planning is not itself production configuration. | Prohibited bypass for production runtime generation | Installer/production services own config, paths, credentials and deployment; Simulator sends explicit target/revision through APIs and keeps only its plans/evidence locally. M0 B01/B03, M3 U01, M4 P01/P02; production packaging owner. Guard `U01-CONFIG`. |
| C17 | Direct SQLite seed path (C04), Restore file deletion (C03), and `verify-approved-identity-lab.py` direct `redis-cli HGETALL policy:current` for policy observations. These are distinct from legitimate Simulator workspace/reset SQLite stores. | Prohibited bypass for production DB access, including readback | Production revision APIs and fresh protected current-state/readback endpoints; installer/storage owners handle recovery. No direct SQLite/Redis fallback. M3 U01/U06, W1/W2, M6 R03. Guards `U01-ASSERTIONS` / `U01-RESTORE` / `U01-RECONCILE`. |
| C18 | Activation stops/deploys node/VS runtime, Policy-Service, Control-Service, DNS, relays and observability; `multinode-admin.sh:refresh_multinode_gateway` restarts gateway. Packaged C14 independently reloads VS. | Prohibited bypass | Installer/service supervisor owns production; companion supervisor owns LDAP/OIDC/DNS. Simulator lifecycle targets endpoints/workloads only. M3 U01, M4 P01/P03/P05. Guard `U01-ACTIVATION`. |
| C19 | `simulator_reset.go` requires named identity, organization, intent/confirmation, epoch/generation and durable operation binding. `scenario.go:resetLocked` rejects active/cross-org scenarios and resets only local run state; uncertain outcomes cannot replay. | Simulator-owned | Retain this reset, its authorization and retry semantics. Do not connect it to C03 or use it to revoke/delete policy, sources or enrollment. M3 U06; production destructive operations remain separately owned. |
| C20 | Machine containers, user sessions, workload commands, fault/action plans and execution evidence in `simulator.go`, `scenario.go`, `simulator_workspace.go`. | Simulator-owned | Retain; scenario intent is not production observed activity. Endpoint/workload permissions separate from setup, ordinary allow/deny enforced by production. M3 U02/U05; independent clients under V09/V10/V14. |
| C21 | `simulator.go:simulatorMachineControlReturnRouteCommand` and machine start flow inject routes into the rig/node network namespace through Docker exec; substrate setup also uses launcher machine-runtime helpers. | Prohibited bypass when modifying production node networking | Operator/installer network configuration plus an ordinary admitted endpoint path; Simulator can configure its own disposable machine networking only. M0 B03, M3 U01/U04, M4 P01/P05. Guard `U01-CONFIG`. |
| C22 | `activate-organization.sh:rollback` restores request/context files, starts prior runtime and restarts prior Policy/Control contexts; progress file and manager state are not durable production mutation receipts. | Prohibited bypass | Operation IDs, exact candidate/target/revision, named approval, authoritative fresh readback and explicit rollback/recovery contracts; uncertain result must reconcile, not repeat destructive setup. M2 C04, M3 U06, W1 common handoff §5. Guard `U01-RECONCILE`. |

## One implemented increment: fail-closed ordinary-client launcher guard

`activate-organization.sh` now checks **before helper sourcing, runtime path
resolution, lock acquisition, profile reads, compilation or logging**:

```sh
# Read-only compatibility report; expected exit 1 while legacy bypasses remain.
sh zpr-dashboard/scripts/activate-organization.sh preflight-ordinary-client

# Opt-in protection for invocations of this legacy launcher (also expected 1).
SIMULATION_SETUP_MODE=ordinary-client \
  sh zpr-dashboard/scripts/activate-organization.sh reset-organization redwood
```

Both report the eight stable `U01-*` blockers mapped above and refuse setup.
Malformed preflight arguments or unknown mode return 2. No URLs, credentials,
profiles or runtime availability are needed, and no source probe is attempted.
The report describes known source-level incompatibilities, not dynamic readiness
or proof any external service is available.

Absent/empty `SIMULATION_SETUP_MODE` retains existing `legacy-launcher` behavior;
explicit `legacy-launcher` also retains it. This default is compatibility only,
**not ordinary-client acceptance**. The guard is scoped to this launcher, not all
Simulator entrypoints, packaged provisioning, or production APIs. It does not
claim to disable every C-row, authorize a mutation, or enforce least privilege
outside its own entrypoint. No new production endpoint or trusted-source path is
introduced. Callers adopting this guard must explicitly select `ordinary-client`;
existing web activation is not relabeled or migrated by this change.

## Verification (offline)

Passed:

- `sh -n` separately on `zpr-dashboard/scripts/activate-organization.sh` and
  `zpr-dashboard/scripts/test-ordinary-client-preflight.sh`.
- `sh zpr-dashboard/scripts/test-ordinary-client-preflight.sh`: **17 cases**.
  Empty `PATH` makes external commands unavailable: default/explicit mode
  preflight, both guarded mutations, invalid organization, missing/unknown
  action, bad mode and extra preflight arguments all fail closed before helper
  sourcing. Tests create no runtime/files and call no network/service commands.
- From `zpr-dashboard`, with `TMPDIR` set to a worktree-local validation directory
  and `CGO_ENABLED=0`, focused `go test ./cmd/zpr-web-dashboard -count=1` selecting:
  `TestOrganizationActivationCommitsOnlyAfterReset`,
  `TestOrganizationActivationPublishesResetProgress`,
  `TestOrganizationActivationRequiresEveryConfiguredNodeInSync`,
  `TestOrganizationActivationPreparesTrustedServiceBeforeStoppingRuntime`,
  `TestOrganizationPolicyInstallNeverUploadsStaleBundle`,
  `TestSimulatorActivationUsesIndependentServiceNotOperatorSession`,
  `TestGatewayActivationRegistersAfterEffectivePolicy`.
  **All seven top-level tests passed**, including fake compile/install failures;
  no live compilation, Docker/service control or deployment was performed.

No Go/Rust source changed. Full/browser/live acceptance was not run; these tests
are not evidence of V01–V15, real organization LDAP, connected admission,
functional demo acceptance or production qualification.

## Next packets / replacement order

1. **M0 contract/production-boundary owners:** finish B01–B03 (stable organization,
   source and target identity, supported operations, scoped permissions). Define
   ordinary setup identity separately from workloads; forbid reverse dependencies.
2. **Trusted-source packet (M1 S01–S06):** supply durable revisioned source
   registration/read/test, two-source provenance and external verified read-only
   LDAP. Obtain explicit consent/account before any real directory test. C05/C06
   cannot migrate to the Simulator handler; C07/C09 need companion ownership.
3. **W1 / M2 operator contracts:** prove save/test/stage/guarded apply/status with
   Simulator unavailable, including operation retry/partial failure and fresh
   target/version/digest readback. Then replace C04/C11/C12/C22 one path at a time;
   do not remove failure-safe compilation or old paths before equivalence tests.
4. **W2 / M3 U04:** select supported pre-enrollment tools for a fixed endpoint set
   and an ordinary non-Simulator comparison client. Retain unique endpoint keys,
   explicit approval and persisted admission evidence. Remove C14 only once that
   equivalent path works; native protected-key production qualification stays gated.
5. **M3 U03/U05/U06 and M4 packaging:** consume independent references; scope
   Simulator reset to C19; move production file/service/network ownership out of
   C02/C03/C16–C18/C21. Test scenario reset and Simulator shutdown preserving
   production registrations, policy, LDAP, enrollment and operator access.
6. **M5/M6:** execute V01–V15 with agreed outage/refresh budgets and actual source
   consent; record any active X01–X07 lab shortcuts individually. This inventory
   neither activates nor approves those shortcuts. No push/deploy was requested.
