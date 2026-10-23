# Compiled baseline recovery candidate

Branch: `recovery/compiled-baseline-20261023`.
Base: `b49842bded73fe581b5a67383fd18734bad55b58`
(`checkpoint-2026-10-16-pre-simulator-admin`).

This candidate restores the checkpoint direction, not the later experimental
Simulator/admin/budget implementation. It does not change the running VM,
shared main, remote branches, or the original dirty worktree.

## Imported changes

The dashboard overlay is content-identical to the isolated recovered dashboard
source as of October 22. Only 21 files differ from the checkpoint:

- Owned-process lifecycle and cleanup for machine workloads, test services and
  stress fleets; do not terminate unrelated processes by executable name.
- Simulator mutation requests use the existing CSRF helper, with deterministic
  script initialization and idempotent initialization across navigation.
- Login UI hides verbose scope details while retaining session-state behavior.
- Direct policy/assertion AI edits validate exact unique original-source anchors,
  reject overlaps/no-ops, and allow one correction within the original output
  budget. Edit requests authorize immediate undoable unsaved changes.
- Regression tests for those changes.

The browser editor bundle and dependency manifests are unchanged. No credential,
private snapshot, executable image, runtime database, or recovered directory is
added to Git.

## What the source does not reproduce yet

The working environment also depends on guest-local recovery provisioning,
organization lifecycle wrappers, generated state, compiler inputs, telemetry
collection and the private DNS statistics relay. Those are preserved with the
cold VM and recovery archives, not hidden as dependencies satisfied by this
source tree.

Some historical companion executable/source mappings remain unproven. A source
build is not evidence that it recreates the exact historical binaries. The
public distribution still requires reviewed seeds, fresh identities, relocatable
configuration, independent setup, and clean-machine installation acceptance.

## Validation

On macOS ARM64 with cached dependencies:

- `GOPROXY=off make test GO=/opt/homebrew/bin/go` was run for both this candidate
  and an untouched checkpoint extracted into a separate directory.
- Candidate has 15 failing top-level tests; all 15 also fail on the checkpoint.
  The checkpoint additionally failed `TestObservabilityWaitsForAssignedAddress`.
  This is baseline comparison evidence, not a passing full-suite claim.
- Failures include absent external organization seeds/manifests/compiler inputs,
  inherited assistant/enrollment contract mismatches, launcher fixture mismatch,
  container-state assumptions, and a Linux ELF artifact expectation.
- Focused recovery/direct-editor/operator-boundary tests pass with `-race`.
- Complete `internal/assistant`, `internal/controlservice/inventory` and
  `internal/dnsstats` tests pass.
- `node --test scripts/operator-request.test.mjs` passes.
- `go build ./...` passes; the web runtime cross-compiles for Linux ARM64.
- `git diff --check` passes.

No tests were weakened or rewritten to hide the inherited failures. All test
failures must be dispositioned before promoting this candidate. The candidate
binary has not replaced any live executable and has not passed the full
seventeen-scenario/browser suite in a fresh environment.

### Self-contained policy assets

Restored the credential-free Great Lakes and Load Lab example compiler inputs
from the original host's corresponding example files. The Great Lakes content
is unchanged; Load Lab differs only by adding the final newline.
The broad dashboard `*.zplc` ignore rule had omitted both files; explicit
exceptions now include these reviewed paths without allowing arbitrary local
compiler configuration into Git.

`TestBundledPolicyAssetsAreSelfContained` reads all five profile references
through the existing bounded, no-symlink, credential-checking definition reader.
It checks compiler configuration TOML, policy catalog JSON and runtime-policy
availability. This is an asset completeness/format gate, not proof of compiler
acceptance or a runtime deployment.

After restoring these files, the complete Go suite has nine failing top-level
tests, down from fifteen. Six organization duplication/profile/archive/import
test groups now pass. Focused asset and duplicate/import tests also pass.

Remaining blockers, with no test suppression:

| Area | Evidence / next action |
|---|---|
| Scenario AI prompt | Embedded skill lacks the test's expected instruction `proposal to null`; reconcile intended structured-output contract. |
| Northstar directory | Profile references pre-generated LDIF outside Git; review seeds and generation provenance before bundling sanitized demo data. Also blocks design-assistant test. |
| Enrollment | Verified-TLS test expects invitation creation without the readiness preflight now required by the implementation; preserve the production readiness gate and reconcile test intent separately. |
| Policy installation fixture | Extracted shell function omits platform initialization provided by its normal caller; standalone fixture lacks `ZPR_MULTINODE_PLATFORM`. |
| Organization catalog | Test reads an absent external `simulation-environment.json`; define a reviewed distributable manifest/bootstrap contract. |
| Placement fixture | Launcher helper moved to sourced `machine-runtime.sh`; test extracts only `dashboard-stack.sh`. Do not duplicate production logic to satisfy an outdated extractor. |
| Simulator container state | Container-state fixture expects running but receives exited; investigate environment-independent state contract. |
| Trusted service build | Artifact validation expects Linux amd64 ELF; fixture/environment currently produces an incompatible artifact. |

Private full-suite logs are preserved outside Git. The production recovery and
original working trees remain unchanged.

## Adapter retention and main promotion

Core is a separate repository. Its adapter development is preserved at
`archive/adapter-work-before-baseline-20261023` pointing to
`4e82d14f767a` (resolve the full revision in that repository).
Retain and review independently:

- macOS TUN configuration/readiness and installer work;
- reusable cancellable adapter lifecycle;
- Windows portability and Wintun work;
- iOS packet-flow/FFI groundwork;
- related peer/Visa reliability and tests.

Preservation is not approval of every change or proof of platform readiness.
Do not merge unrelated newer Simulator work to obtain adapter improvements.

Promotion gates:

1. Establish a self-contained build/runtime manifest and seed provenance.
2. Reconcile missing fixtures and inherited failures without copying private state.
3. Validate a compiled candidate in a new isolated environment.
4. Run all five organizations/seventeen scenarios and operator acceptance.
5. Review the candidate against main, then choose a forward restoration commit
   or an explicitly approved history rewrite. Preserve newer refs and dirty work.

No remote/main cutover is part of this candidate checkpoint.
