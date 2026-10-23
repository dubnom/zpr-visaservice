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
private snapshot, executable image or runtime database is added to Git.

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

### Portable Northstar directory

Bundled the four original fictional Northstar LDIF files and changed the profile
from external `pregen` to the existing `ldif` mode. Identity DNs, role memberships,
machine attributes and file contents are unchanged. The reader's
`DEMO_BIND_PASSWORD_HASH` is a provisioning placeholder, not a password or hash;
the existing LDAP launcher replaces it with a freshly generated password hash.
It must remain for the `simpleSecurityObject` schema and authenticated reader.
The existing dashboard definition reader strips it from definition bundles;
the directory seed loader retains it for provisioning.

The new regression checks run with nonexistent external manifest/pregen paths.
They verify that all profile people, twenty machines and key roles are present,
all DNs use the fictional test base, no credential attributes exist other than
the one provisioning placeholder, and sanitization removes that placeholder.
The existing seed-location and directory-assistant tests now pass.

The scenario assistant now explicitly returns `proposal: null` for questions
or missing essential prerequisites, rather than inventing executable changes.
Its embedded skill contract test passes. No existing tests were modified.

After these changes the complete Go suite has six failing top-level tests, down
from nine. Focused seed/asset/design-assistant/skill tests pass with `-race`;
all Go commands build and the web runtime cross-compiles for Linux ARM64.
These are source-level checks, not a fresh LDAP deployment or scenario run.

Remaining blockers, with no test suppression:

| Area | Evidence / next action |
|---|---|
| Enrollment | Verified-TLS test expects invitation creation without the readiness preflight now required by the implementation; preserve the production readiness gate and reconcile test intent separately. |
| Policy installation fixture | Extracted shell function omits platform initialization provided by its normal caller; standalone fixture lacks `ZPR_MULTINODE_PLATFORM`. |
| Organization catalog | Portable topology and five-organization catalog checks now pass; inherited test still hardcodes an absent external `.local-runtime/simulation-environment.json`. Requires an explicit fixture update, not private-state restoration. |
| Placement fixture | Launcher helper moved to sourced `machine-runtime.sh`; test extracts only `dashboard-stack.sh`. Do not duplicate production logic to satisfy an outdated extractor. |
| Simulator container state | Test invokes real Docker against `zpr-simulator` and expects it to be running. Current context reports exited. Mock Docker deterministically rather than altering shared containers. |
| Trusted service build | Artifact validation expects Linux amd64 ELF; fake Go compiler writes the text `candidate`. Replace the fixture with a valid representative artifact while retaining invalid-artifact rejection coverage. |

### Explicit topology preparation

The reviewed `examples/simulation-environment.json` retains the original
twenty-machine, six-component, seven-service, three-agent topology and explicitly
selects Northstar. It excludes only historical `bootstrap` source-build inputs
and the `extends` descriptor, neither of which the runtime resolves.
It contains descriptors, fictional identities, private topology addresses and
the public `https://example.com/` gateway target, not generated identity files,
live state or service secrets.

`scripts/prepare-simulator-manifest.sh` copies this topology into an explicit new
owner-only directory. It refuses existing paths and output symlinks; missing
parents and copy failures fail explicitly. It never starts services or changes
production configuration. Runtime path defaults and legacy commands are
unchanged; `SIMULATION_MANIFEST` is the explicit Simulator-only input.

New tests invoke preparation from an unrelated working directory, verify
owner-only permissions and non-overwrite behavior, check the manifest through
the existing credential-safe definition reader and runtime validator, and
exercise the actual catalog handler for all five selected organizations with
temporary state. These tests pass with `-race`.

The full suite still has the same six inherited failures above. In particular,
no hidden runtime manifest was created to satisfy the historical hardcoded
fixture. No existing tests were changed. The compiled distribution still needs
fresh identity/service provisioning and clean-install/scenario acceptance.

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
