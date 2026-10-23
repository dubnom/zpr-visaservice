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

Current candidate: the full `GOPROXY=off make test GO=/opt/homebrew/bin/go`
suite passes after the explicitly approved inherited-fixture repairs described
below. Targeted repaired tests pass with `-race`; Go builds, Linux ARM64
cross-compilation and the Node operator-request regression pass.
This is not a clean-install or full live-scenario acceptance claim.

### Initial checkpoint comparison

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

No tests were weakened or rewritten to hide the inherited failures. The
subsequent authorized fixture repairs are recorded below. The candidate
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

Inherited blockers at the portable-seed checkpoint (subsequently resolved by
the approved fixture repairs below, with no test suppression):

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

### Approved isolated fixture repairs

The user explicitly approved updates to the six existing failing tests after
the portable-manifest checkpoint. Production behavior was not changed:

- Verified-mTLS enrollment still rejects invitation creation without readiness.
  The test now submits a scoped readiness attestation over the authenticated
  API, then verifies invitation creation; shared/unverified certificates
  remain rejected with Simulator unavailable.
- The policy-install test sources and configures the actual platform helper,
  preserving merge/compile/missing-output failure, stale-bundle retention,
  no-upload and cleanup assertions.
- The organization catalog test reads the reviewed bundled manifest instead
  of an absent external runtime file.
- Placement tests source the actual machine-runtime helper. Expected node
  addresses distinguish Northstar (`172.30.0.13`) and Great Lakes
  (`172.31.0.13`); missing owner/location and ambiguous placement still fail.
- Container-state tests use a strict local Docker double, not the shared
  daemon. Running, exited and missing-container states are checked, along
  with the original live/missing PID cases.
- Trusted-service build tests provide a representative ELF header matching
  the shell validator, not a runnable executable. Plain text and wrong
  architecture remain rejected without replacing the old artifact, temporary
  files are cleaned, successful publication retains executable permissions,
  and a failing compiler double proves fresh artifacts are not rebuilt.

The full Go suite now passes. No tests were skipped, assertions removed or
production readiness/artifact gates relaxed to obtain this result. Source-level
validation does not establish runtime packaging completeness.

Private full-suite logs are preserved outside Git. The production recovery and
original working trees remain unchanged.

## Source-free compiled engineering validation

The [compiled recovery engineering slice](zpr-dashboard/packaging/compiled-recovery/README.md)
now builds four stripped static Linux ARM64 tools plus reviewed runtime assets,
revision/toolchain metadata and checksums from clean committed source.
It is explicitly marked incomplete and non-public. Dependency license review,
complete node/Visa/compiler packaging, first-run setup and full installation
acceptance remain open.

An exact archive built from `3ff39b7` was extracted and checksum-verified, then
imported into an empty Docker filesystem on a newly created ARM64 VM:
`zpr-compiled-validation-20261023`. This VM has no host directory mounts;
the active Docker context stayed `desktop-linux`. Neither the original nor the
older fallback VM was reused or modified.

Eight offline checks passed in read-only, capability-free, network-disabled
containers with only a new private generated-state volume:

1. Composed runtime policy for each of the five bundled organizations.
2. Ran the compiled lab trust generator twice, proving distinct public CAs.
3. Confirmed existing trust output is refused rather than overwritten.

No source checkout, compiler, shell, build cache, host bind mount or Docker socket
was available inside those containers. The first attempt exposed build-UID-only
archive permissions; normal public runtime modes (`0755` directories/binaries,
`0644` assets) fixed that without granting container capabilities or exposing the
enclosing private build directory/archive.

Measured engineering slice: 13,925,524 compressed bytes and 34,938,506 extracted
file bytes. This is **not the full demo's download size**. It excludes runtime
images and node/adapter/Visa/compiler binaries. The archive was not published.
Test containers, generated-secret volume and imported image were removed.
Public CA/policy receipts and digest evidence remain privately under validation.
The test VM is stopped after validation and retained for future isolated work.

This proves offline execution/asset portability for the included slice only.
It does not prove a complete installation, LDAP/OIDC/DNS initialization,
policy compilation/install, UI login, AI setup or seventeen-scenario acceptance.

### Rust runtime and fresh-key compilation gate

The separate [native runtime probe](zpr-dashboard/packaging/compiled-recovery/probe-rust-runtime.sh)
now exercises the recovered locally built Rust tools on the fresh Ubuntu
24.04.4 ARM64 validation VM. Only compiled executables, public example assets
and the probe were transferred; no old runtime directory, credential, source
tree or generated policy was reused.

Initial PH/PH-CLI startup correctly exposed a missing `libpcap.so.0.8`.
Installing only the missing `libpcap0.8t64` runtime package and its package-manager
dependencies fixed both. Observed runtime versions:

| Component | Observed CLI / runtime |
|---|---|
| PH and PH CLI | `0.7.0` |
| Visa Service and Admin | `0.19.0` |
| VS API-key tool | Help works; no `--version` option |
| Compiler | `zpc 0.16.0` (historical CLI spelling) |
| Policy Tester | `0.19.0` |
| glibc | `2.39-0ubuntu8.7` |
| OpenSSL runtime | `3.0.13-0ubuntu3.9` |
| libpcap runtime | `1.10.4-4.1ubuntu3.1` |
| GCC runtime | `14.2.0-4ubuntu2~24.04.1` |

The checked-in probe validates ARM64 ELF headers, dynamic loader resolution,
seven CLI invocations and binary hashes. It composes all five current example
policies using the compiled web command and builds each with the actual compiler
and a newly generated private RSA signing key. All five produced nonempty
signed policy bundles. Invalid syntax was rejected with compiler exit `1`
and no output bundle. Existing evidence and missing executables are refused.

Compiler warnings remain visible: Visa docking is set but not enforced, and
the default trusted service has no certificate. These example builds do not
contain the generated production runtime bootstrap overlay. They were not
uploaded or installed, and signature acceptance by a configured Visa Service
has not been tested. Neither warnings nor transport/bootstrap requirements
were suppressed to obtain a build result.

The preserved source candidates are Core `561fa7351795a723b74b5d1e57761d870722c77c`,
Compiler `6f0de17fd9052567394ad306552bc435efc3ed3f`, and Visa Service checkpoint
`b49842bded73fe581b5a67383fd18734bad55b58`. Their existing build logs identify
development-profile builds (some later stripped), not release-profile builds.
These source candidates are not proof of correspondence to the historical
live binaries, and source-equivalence/release rebuild gates remain open.
The Rust tools have therefore not been silently added to the compiled archive.

Only allowlisted public compiler logs, hashes and the generated public signing
key were copied back as private validation receipts. Temporary probe executables,
the private signing key and generated bundles were removed from the test VM
after digest verification; none became release inputs. The test VM was stopped.
No test listeners, ZPR network or workload containers were started.
This gate confirms executable prerequisites and fresh-key compilation, not
full service bootstrap, trust admission or end-to-end scenario behavior.

### Locked offline Rust release rebuild

The [native release builder](zpr-dashboard/packaging/compiled-recovery/build-rust-release.sh)
has now rebuilt all seven required Rust tools with `--locked --offline --release`,
two build jobs and release debug-information stripping. No Rust source,
manifest, lockfile or preserved worktree was edited.

Builds ran on the separate validation VM in the pinned local image
`sha256:c209d2289c90084a2fea03e67f2c500a2a3dca9bc392fec8a0f9ba7356d7860a`,
with networking disabled, a read-only root filesystem and source mounts,
all capabilities dropped, no Docker socket, and writable build/cache directories
separate from the source. Only committed archives and offline registry/Git
dependency caches were supplied. Source archive digests cover the four primary
repositories, sibling policy/VS API dependencies and Common's exact gitlinks.
The private input receipt records the builder identity, script digest,
source revisions/archive digests and lockfile hashes.

The [release receipt verifier](zpr-dashboard/packaging/compiled-recovery/verify-rust-release.cjs)
confirmed all seven executable digests, ARM64 ELF headers, successful Cargo build
completion, optimization level `3`, disabled debug assertions and non-test
profiles at the expected release paths. Lockfile digests match the independent
input receipt. Its regression test rejects debug/test profiles, altered binaries,
wrong architecture and mismatched lockfiles.

The exact release-built binaries then passed the native runtime probe:
seven CLI/loader checks, all five fresh-key signed policy compilations, and
invalid-policy rejection with no bundle. All eight runtime executable digests
(including the previously validated compiled Go composer) match their inputs.
The two example-policy warnings remain unchanged: unenforced Visa docking
and missing default trusted-service certificate. Compiler builds also retain
the existing Core dead-code/workspace-profile and Visa dead-code warnings.

Measured seven-tool totals:

| Inputs | File bytes |
|---|---:|
| Earlier development-profile tools | 135,224,312 |
| Verified release-profile tools | 52,973,408 |

This is a 60.8% reduction in these seven executable files only, not an archive,
image, total installation size or performance benchmark. Rebuilt outputs and
nonsecret receipts are retained privately under validation; they have not been
added to a public archive, installed into services or substituted for live tools.
The legacy Go-only engineering archive remains unchanged.

Temporary runtime inputs, signing keys and generated policy bundles were removed
from the test VM after receipt verification. The offline build/cache directory
is retained there for repeatable follow-up builds; no build container remains
running, and the VM is stopped. Existing Docker context selection is unchanged.
Clean committed source mapping is now established for these candidate builds;
it still does not prove equivalence to the historical deployment. Full generated
runtime bootstrap, dependency/license review, service trust admission and
seventeen-scenario/browser acceptance remain promotion/publication gates.

### Exact assembled eleven-tool archive

The engineering builder now optionally consumes the verified Rust release
directory and its independent input receipt. The Go-only mode remains supported.
Assembly refuses missing/mismatched release evidence, debug/test profiles and
existing executable outputs. It copies only the seven allowlisted Rust tools,
rechecks their staged hashes, and emits a selected-field public provenance
manifest. Cargo logs, caches, source trees and private runtime evidence are not
release inputs. Regression coverage includes profile/digest/lockfile rejection,
missing receipts, existing-assembly refusal and private-field exclusion.

The final archive was built from clean committed candidate `763d6e7`:

| Measurement | Result |
|---|---|
| Target | Linux ARM64 |
| Executables | Four static Go tools and seven release-profile Rust tools |
| Compressed bytes | 33,532,068 |
| Extracted regular-file bytes | 87,922,656 |
| Archive SHA-256 | `581cd17b03ea7604746a1b910979d29bd047d5131c111979e65023d882d83905` |

Exact archive member, checksum, executable/mode and asset checks passed, together
with all eight existing offline scratch checks. The Rust hashes and sizes were
also checked there, without falsely claiming their dynamic binaries execute in
scratch. Host extended attributes are now excluded from archive creation; a
final native extraction produced no host-provenance header warnings.

The same final archive and checksum were transferred to the mount-free
validation VM, verified again, extracted, and all member hashes checked.
Only these extracted executables and example assets were used for the native
probe. As UID 501 in an isolated network namespace (`unshare --net`), all seven
Rust CLI/loader checks passed, all five policies compiled using a fresh RSA
signing key, and invalid syntax returned exit 1 without a bundle. The eight
executable digests in the native receipts match the exact archive's contents.
The previously documented two compiler warnings remain visible.

Only allowlisted nonsecret receipts were copied back to
`validation/compiled-assembled-clean-20261023/runtime-receipts` in the private
recovery workspace. Both attempts' temporary private signing keys and generated
policy files/bundles were removed; the VM is stopped. No service or workload was
started, no policy installed, no live environment modified, and no public
artifact uploaded. The prior guest `/tmp` release-build cache did not survive
restart; host-side verified release inputs remain available.

This is a measured, tested compiled engineering archive, **not** a complete
installation or public release. Compatible dynamic libraries remain documented
prerequisites; service images, fresh service trust/bootstrap, independent
operator configuration, license review, setup/optional AI disclosure and all
seventeen scenarios/browser acceptance remain required.

### Fresh source-free Visa bootstrap and durable restart

The archive now includes a
[private-state preparation helper](zpr-dashboard/packaging/compiled-recovery/prepare-visa-service.sh).
It consumes only compiled tools, generates fresh RSA authority/admin TLS,
node/Visa bootstrap pairs, a signing key, service identity and separate
read/write/read-only API credentials, then compiles a minimal bootstrap policy.
No recovered state, Simulator topology/configuration or external provider is
used. It does not start a service or change host trust.

State is owner-only under a protected nonsymlink parent, never overwrites an
existing path, and writes its completion inventory last. Its intentionally
restricted path syntax and path-bound Unix socket are documented. Failed
generation leaves private diagnostics, not a success inventory. Focused
regression tests cover unsafe/noncanonical paths, insecure parents, symlink
state/parents, existing-state preservation and missing executable rejection.
The release-verifier regression suite also remains green. No Go or Rust
source/manifests changed in this gate.

The [service probe](zpr-dashboard/packaging/compiled-recovery/probe-visa-service.sh)
ran as UID 501 in a new network namespace with only loopback, no Simulator and
no host source mounts. Redis `7.0.15` was installed only on the validation VM
after confirming it was absent; its system service was masked so only
probe-owned Redis processes could start. Redis listened on a private Unix
socket, and Visa listened only on namespace loopback.

Verified against the exact final extracted archive:

- Two preparations have different service identities, TLS authorities and
  administrator credentials; generated files/directories are owner-only.
- Existing-state refusal preserves the API credentials and hashed-key database.
- Anonymous and foreign-installation API credentials are rejected.
- Read-only credentials can retrieve policy but receive HTTP 403 for a valid
  policy-write request; administrator policy installation succeeds.
- curl verifies the real server certificate and rejects the other preparation's
  CA. Verified policy/network responses match the compiled admin CLI's output.
- Both Visa and Redis stop and restart; Redis loads its AOF from disk and Visa
  restores policy without receiving the initial bundle again. Policy and
  network responses match the pre-restart state.
- The API returns an empty topology and only the self-authorized `vs.zpr`
  actor: no node, adapter or workload was implied by this startup test.
- All eight scratch checks, seven Rust CLI checks, five fresh-key example
  policy builds and invalid-policy rejection still pass.

Final artifact built from clean committed `6662c11`:

| Measurement | Result |
|---|---|
| Compressed bytes | 33,536,591 |
| Extracted regular-file bytes | 87,931,329 |
| SHA-256 | `4f4539e4dfdfecd024ad7fc578455f566de558c920749b13ee312a85f23a6ae1` |

Reviewed nonsecret receipts are retained under
`validation/compiled-visa-durable-20261023/service-receipts` and
`policy-receipts` in the private recovery workspace. Receipt assertions verify
API shapes, the distinct identities, both Redis startups/AOF reload, and hashes
of the exact archived executables/preparation helper. Generated private keys,
API header files, databases and temporary policies from every probe attempt
were removed after preserving the allowlisted receipts. Probe-owned children
were stopped, and the validation VM is stopped.

An inherited issue was discovered while tracing the native admin client:
`vs-admin` disables server-certificate validation. Its calls are **not** used
as TLS proof; curl independently validates CA trust, with a wrong-CA negative
test. Correcting and release-rebuilding the client remains an explicit
publication gate, not a claim that the client is safe for distribution.
The minimal bootstrap emits a visible warning that no fabric policy grants
VisaService admin access; HTTPS API-key administration is a different contract.
The existing example-policy warnings remain visible as well.

The archive remains private engineering output, not an installer. Node trust
admission, policy-signature enforcement, full organization/service generation,
runtime images, independent operator configuration, licensing, setup/AI
disclosure and seventeen-scenario/browser qualification remain open. Main,
preserved dirty worktrees and both live recovered environments are unchanged.

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
