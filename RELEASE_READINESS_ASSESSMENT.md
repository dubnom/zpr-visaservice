# Release-readiness assessment

Assessment date: 2026-10-25. Source baseline: `54c9121`.
This is a read-only source/documentation inventory, not a release certification,
fresh test run, host inspection or authorization to publish/deploy.
Reported acceptance below is evidence recorded in the linked runbooks, not
acceptance rerun for this assessment. No running services were inspected.

**Historical-baseline notice, 2026-10-29:** the inventory below is retained as
October 25 evidence, not current source capability. The
[evaluation runbook](zpr-dashboard/packaging/evaluation/README.md) now documents
source-independent Linux/amd64 engineering archives; descriptions here of a
source-built-only evaluation do not negate that newer work. That packaging
does not establish complete demo or production qualification. The release-plan
demo host summary has been corrected to the specific 26.04.1 demo contract;
production remains 24.04. The [documentation guide](../DOCUMENTATION_GUIDE.md)
records remaining reconciliation limits. Later OPA-1/CFG-1 addenda are requirements,
not new acceptance results for this historical baseline.

## Executive result

There are useful engineering artifacts, but they do not yet constitute a
qualified complete production installation. Keep four claims separate:

1. **Component engineering:** validators, archive/signature tooling and
   production-filtered operator assets exist.
2. **Local evaluation:** the B1-B3 launcher has documented real-service
   readiness and persistence evidence, but requires a development environment,
   uses mixed-platform images and is not a complete VS/Policy installation.
3. **Paired demo:** a limited two-member slice has workbench acceptance.
   Downloaded paired-archive clean-host/full-scenario acceptance remains open.
4. **Production pilot:** full BOM, installation, platform qualification,
   lifecycle/recovery and signed release acceptance remain open.

Source-level maturity of subject-link persistence/publication is not proof that
any package includes, configures, supervises or recovers that capability.

## 1. Product and platform boundaries

| Scope | Actual target/constraint | Evidence and implication |
|---|---|---|
| Production pilot | Dedicated Ubuntu 24.04 LTS x86-64, Docker/Compose, private management HTTPS | [Release plan](System%20release%20plan.md), [preflight](zpr-dashboard/packaging/system/preflight.sh); not HA or public-Internet qualification |
| Downloaded demo pair | Disposable Ubuntu 26.04.1 LTS x86-64 | [Demo contract](zpr-dashboard/packaging/system/DEMO-PACKAGE-CONTRACT.md#downloaded-artifact-acceptance); demo-host approval does not change production qualification |
| Evaluation | Trusted local macOS/Linux workstation, Go dependencies, local Docker, preloaded mixed amd64/arm64 images | [Evaluation prerequisites](zpr-dashboard/packaging/evaluation/README.md#host-and-current-package-requirements); not source-free customer installation |
| Operator package | Production-filtered static assets; shared multipurpose Go command remains | [Asset package](zpr-dashboard/packaging/control-room/README.md); asset exclusion is not complete executable/deployment extraction |
| Optional Simulator | Separate lifecycle and synthetic authority | Must not be a production dependency; approved Docker-socket privileges apply only to isolated demo scope |

The top-level release plan still describes Ubuntu 24.04 as the selected demo
host, while the specific demo contract and system packaging README identify
26.04.1. Reconcile the historical summary before executing qualification.
Do not silently change production preflight or claim one platform qualifies
the other. Record both host OS and container-image base independently.

## 2. Artifact inventory and release gaps

| Capability | Source evidence available | Limit / missing release evidence |
|---|---|---|
| Host prerequisites | [Preflight script](zpr-dashboard/packaging/system/preflight.sh), adjacent regression script | Checks an already provisioned local Docker host; not dependency installation or clean-host acceptance |
| Company input validation | [Validator staging](zpr-dashboard/packaging/system/README.md#linuxamd64-validator-cli-engineering-staging), [example input](zpr-dashboard/packaging/system/company.example.json) | Engineering binary; validation is not activation, installation or runtime readiness |
| Bundle tooling | Manifest validation, packing, verification, signing and runtime-input rendering commands under [cmd](zpr-dashboard/cmd) | Tools do not supply a qualified complete BOM, signed system release or customer installer |
| Linux tool archive | [Build script](zpr-dashboard/packaging/system/build-bundle-tools-linux-amd64.sh), checksums and build information | Unsigned engineering staging; records local toolchain but does not establish reproducible builds/release provenance |
| Control Room assets | [Production package](zpr-dashboard/packaging/control-room/README.md), explicit embeds and manifests | Component packaging; no complete production service startup, dependency supervision or independent binary extraction claim |
| Starter identity/logging | [B1-B3 evaluation](zpr-dashboard/packaging/evaluation/README.md) | Foreground source-built evaluation; incomplete upstream VS/Policy service set, platform and licensing qualification |
| Demo runtime data | [Synthetic fixture generation](zpr-dashboard/packaging/demo-runtime/README.md) | Synthetic package data is not production initialization or company identity |
| Paired ZPR/Simulator demo | [Package contract and blockers](zpr-dashboard/packaging/system/DEMO-PACKAGE-CONTRACT.md#known-blockers-to-producing-the-pair) | Clean downloaded-archive acceptance/full scenarios remain open; limited workbench success is narrower |
| Endpoint enrollment distribution | [Enrollment packaging](zpr-dashboard/packaging/enrollment), including build/sign/download/verification scripts | Component scripts are not proof of certified endpoint admission, renewal, revocation and upgrade across the full system |
| Protected source integration | [Integration guide](TRUSTED_SOURCE_INTEGRATION.md) and implementation acceptance | Does not qualify packaged provider transport rotation, active subject-link rollout or real-company deployment |

Do not reuse component checksums or the existence of a signing command as
evidence that a system release has been signed with approved release trust.
Likewise, an image digest pins bytes; it does not establish licensing approval,
supported architecture, provenance or compatibility.

## 3. Recovery and lifecycle inventory

| Event | Existing documented behavior | Remaining qualification |
|---|---|---|
| Evaluation stop/restart | Same protected state and flags retain identities/data; supervisor cleans only its own runtime | Full appliance supervision, unattended boot and host-failure recovery |
| Evaluation backup | Whole private state copied offline while stopped; includes sensitive CA/credential material | Encrypted/access-controlled backup procedure, complete restore rehearsal, retention and recovery objectives |
| Evaluation admin reset | Explicit stopped-state workflow; interruption marker; restore whole backup on interrupted multi-file operation | Not crash-atomic automatic rollback; restored backup restores old credentials |
| Logging administrator recovery | Evaluation Dex recovery explicitly does not reset OpenObserve credentials/sessions | Supported logging-admin credential/session recovery remains unfinished; editing environment files is not a password-reset procedure |
| Certificate lifecycle | Evaluation leaves expire after 90 days; CA after one year | Rotation, issuer replacement, expiry behavior, emergency revocation and validated recovery |
| Durable source controls | State-preserving restart retains operational overrides | Package persistence configuration, full host restore and downstream delivery evidence |
| Subject-link snapshot/publication | Offline and internal durable lifecycle work is present at this baseline | Separate delivery/activation gates and package integration; not a turnkey production capability |
| Upgrade/downgrade | Tools/plans and component behavior are not a qualified system migration | Version compatibility, state schema migration, interrupted upgrade, rollback safety and identity preservation |
| Bundled-to-company migration | B6 retirement workflow is documented as unfinished | Explicit rebinding, preview, authorization, failure recovery; no automatic provider fallback |

OpenObserve's AGPL-3.0 selection is accepted for evaluation only. Its configured
seven-day asynchronous retention is not a disk quota, secure erase guarantee or
seven-day elapsed-time acceptance result. Neither licensing nor storage
qualification should be inferred from the evaluation readiness probe.

Do not treat restoring an old backup as harmless rollback: it can restore old
credentials, grants, revocation state and data. A restore policy must distinguish
recoverable operational state from security state that must not be rolled back.
The exact anti-rollback mechanism requires design/qualification; this inventory
does not invent one.

## 4. Documentation discrepancies needing reconciliation

These are source findings, not fixes applied to maturity:

| Finding | Evidence | Required disposition |
|---|---|---|
| Historical demo host says 24.04; detailed acceptance says 26.04.1 | [Release-plan priority](System%20release%20plan.md#current-execution-priority-demo-not-production) versus demo contract | Identify the authoritative demo acceptance target and update stale summary; preserve separate production target |
| System README's B1 summary says no registrations/log ingestion; detailed B1-B3 runbook reports both | [System overview](zpr-dashboard/packaging/system/README.md#b1-local-evaluation-subset) versus [evaluation matrix](zpr-dashboard/packaging/evaluation/README.md#b2b3-verification-matrix) | Update summary to distinguish completed evaluation subset from incomplete full BOM |
| Broad plan status can lag implementation | [Release plan](System%20release%20plan.md) versus current evaluation/runbooks | Attach dated evidence to individual gates; do not mark the overall installer complete |

## 5. Next release work, without blocking maturation

### Safe preparation now

1. Resolve the documentation discrepancies in a separate documentation change.
2. Define an evidence record per candidate: source commit, artifact digests,
   platform tuple, build/toolchain identity, config schema, test commands/results,
   known exclusions and publication approval.
3. Prepare a clean-host **demo** acceptance checklist directly from the approved
   pair contract; do not broaden it into production certification.
4. Prepare a production restore/upgrade test matrix using disposable state,
   including security-state rollback cases. Do not run it against live state.
5. Establish which BOM, license, platform and artifact ownership decisions remain
   open before commissioning implementation.

### Coordinate before implementation/execution

- Package current source lifecycle/snapshot functionality only after maturity
  settles its production transport, validity and recovery contracts.
- Obtain separate authorization before starting deployment containers,
  manipulating a workbench installation, issuing release credentials, publishing
  downloads or executing destructive recovery tests.
- Keep Simulator unavailable during production operator/package acceptance.
- Run full clean-host and recovery exercises against exact release artifacts,
  not substituted source builds.

## 6. Minimum evidence to change the release claim

**Added clarification, 2026-10-29:** [CFG-1](CONFIGURATION_LIFECYCLE_STATUS.md)
records the incomplete ZPR Config edit/push/lifecycle path, distinct from policy.
Production evidence must include exact-revision application, independent
effective-state readback, drift/partial-failure visibility and recovery for
the declared supported configuration targets. Policy-only guarded apply or
saving TOML drafts does not satisfy this gate. Running enablement and the full
configuration-consumer inventory have not been verified by this record.

**Added requirement, 2026-10-29 (not assessed at the original baseline):**
[OPA-1 operator access](OPERATOR_ACCESS_PLAN.md) is a blocking production gate.
It requires protected loopback setup/recovery, policy-controlled ZPR access for
every shipped operator tool, and an independently switchable browser gateway.
Acceptance must prove gateway-off terminates its sessions while local and ZPR
access remain, persistent off-state, no backend bypass listeners, and independent
authenticated local outage recovery. This is newly recorded scope, not a
claim that the source baseline implements or fails those tests.

A complete production-pilot claim requires an approved BOM and license/platform
matrix; verified signed artifacts and trust bootstrap; source-free first install;
named operator bootstrap/recovery; actual provider/logging/DNS/data-plane
readiness; endpoint enrollment and policy enforcement; independent Simulator
absence; state-preserving restart; exercised backup/restore and upgrades;
credential lifecycle; bounded resource/storage behavior; and a truthful
operator runbook with explicit failure reporting.

Keep each gate independently recorded as passed, failed, blocked or not run.
At this assessment baseline, missing evidence is **not established**, not a
claim that an uninspected implementation cannot work.
