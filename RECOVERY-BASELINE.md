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

### Verified administration TLS repair and rebuilt archive

The candidate admin client no longer disables certificate validation. Its shared
HTTP client requires HTTPS, validates the server's trust chain, validity and
endpoint identity, and returns exit 1 for request failures rather than printing
an error with a successful process status. There is no insecure fallback.
The default platform trust roots remain enabled alongside the supplied CA.

Ten targeted Rust tests pass on both macOS and native Linux ARM64, including
every request method (GET, POST, JSON POST and DELETE) against freshly generated
trusted, foreign-authority, wrong-host and expired certificates. Rejected TLS
handshakes do not deliver the test API credential; plaintext requests are
rejected. Only the existing workspace OpenSSL dependency was added for tests;
no dependency version was upgraded. Changed Rust files pass rustfmt, and the
packaging regression tests pass.

Targeted Clippy completes with four inherited warnings: two redundant imports,
a needless return and a redundant conversion. `-D warnings` therefore fails;
all four sites were verified unchanged against the preserved checkpoint.
No lint suppression or unrelated cleanup was included.

With explicit user authorization, exact committed source archives were prepared
again, recording the repaired Visa source revision `4ebc2c0`, updated lockfile
hash, all other source/gitlink archive hashes and the unchanged builder/script
identities. All seven Rust tools were rebuilt locked/offline/release inside the
same pinned ARM64 builder, with no network, read-only source/root filesystem,
no capabilities/socket, unprivileged UID 501 and separate writable cache/output.
The strict Cargo/profile/hash verifier passes. Six executable hashes are
identical to the previous release build; only `vs-admin` changed:

| Executable | Bytes | SHA-256 |
|---|---:|---|
| `vs-admin` | 9,209,648 | `99b287a805540eedd79ad226e6f5db9c2f6493ec78c4775c2acbc481d0810ea5` |

The exact newly assembled archive passed all eight offline checks, seven native
CLI checks, five signed example-policy builds and invalid-policy rejection.
Its freshly prepared Visa service also passed the existing trust/permission
and durable Redis/Visa restart checks. Additional native CLI checks require
exit 1 and empty stdout for a foreign CA, mismatched hostname, plaintext HTTP
and another installation's API key. All pass using only the exact extracted
tools. curl's independent TLS/CA checks still pass; the fixed client now also
validates TLS itself.

| Final engineering archive | Result |
|---|---|
| Source revision | `4ebc2c00d0506f897f7f461fe8c1bcdc0c5681fc` |
| Compressed bytes | 33,536,199 |
| Extracted regular-file bytes | 87,931,660 |
| SHA-256 | `1f48287d36f0c68a2545a798265c69a8587bfac39de4a2254473fb813ca49fd4` |

Independent source receipts and exact release outputs remain private under
`validation/rust-tls-inputs-20261023` and `rust-tls-release-20261023`; native test
and build logs are retained separately. Exact archive acceptance receipts live
under `validation/compiled-tls-fixed-20261023/service-receipts` and
`policy-receipts`. The archived preparation helper and exercised executables
were digest-matched before deleting generated test keys, API headers, private
databases and temporary policies. No container remains running in validation;
the VM is stopped. Build/cache inputs are retained under guest `/var/tmp`
for future offline work.

The admin TLS publication blocker is fixed in this candidate and its new
verified archive; older archives remain historical engineering evidence only.
This does not qualify the complete installation. Runtime images, node/service
trust admission, policy-signature enforcement, complete organization bootstrap,
independent operator configuration, licenses, setup/AI disclosure and all
seventeen scenarios/browser acceptance remain open. Nothing was published or
deployed, and main, dirty worktrees and both running demos remain unchanged.

### Fresh real-node admission without Simulator

The packaged preparation helper now accepts explicit `--node` for an isolated
engineering node configuration. Default service-only behavior is unchanged.
It adds a fresh X25519 Noise key and CA-signed certificate using OpenSSL 3,
preserving the separate RSA bootstrap identity already bound by public key in
the minimal policy. No extra PKI executable or source checkout is needed.
It writes explicit fixed test addresses/configuration, but starts no processes
and changes no network or host trust. State still remains owner-only,
non-overwriting and excluded from archives.

The extended native probe starts a real release-built PH node in a temporary
second network namespace, joined to the isolated Visa namespace by a private
veth pair. Only namespace-local IPv6 routes and a TUN are created; no default
route, external interface, published port or globally named namespace is used.
This optional namespace/TUN gate runs as root; the previous service-only gate
still passes as unprivileged UID 501. Simulator is unavailable throughout.
Redis and Visa continue to use fresh private state only.

The test first runs PH with a different installation's RSA private key while
claiming the same node CN/address. It requires Visa's exact
`blob signature verification failed for cn node` receipt and an empty node
inventory, not merely a failed process or timeout. PH is then started using the
matching fresh key. Both PH's grant/VSS-registration messages and Visa's
authenticated actor are required: CN `node`, address `fd5a:5052::2`,
`node: true`, `in_sync: true`, zero pending installs and VSS port 8183.
Exactly one node is present afterward.

Initial experiments sharing a namespace were correctly rejected by Visa's
source-address check; separate namespaces fixed the harness, without changing
or bypassing that check. The preserved Core also loads config authentication
keys after CLI arguments, so a CLI override did not select the intended
negative-test key. The final probe explicitly writes a separate foreign-key
configuration and verifies actual RSA rejection. Core was not edited; that
inherited CLI-precedence issue remains documented.

The exact final archive was built from clean committed candidate `a531bd4`:

| Measurement | Result |
|---|---|
| Compressed bytes | 33,536,541 |
| Extracted regular-file bytes | 87,936,044 |
| SHA-256 | `810ff05d27f35c9f92c27a110f1384c4a0dd60399b68f7f28b6b7f4f1e900c69` |

The extracted preparation helper and six exercised node/service executables
were digest-matched to the archive. Both the root node gate and unprivileged
service-only gate pass, as do all eight scratch checks, seven Rust CLI checks,
five signed example policies, invalid-policy rejection, admin TLS/permission
negatives and durable Visa/Redis restart. Focused preparation/release-verifier
regressions and shell syntax checks pass. No Go or Rust source changed.

Only reviewed nonsecret receipts were retained under
`validation/compiled-node-admission-20261023/node-receipts`, `service-receipts`
and `policy-receipts` in the private recovery workspace. The node summary
excludes identity tokens/attributes; raw private logs/state were not exported.
Generated state and logs from every attempt were removed after receipt
verification. Owned processes and temporary namespace anchor were stopped;
the guest has no remaining test links/named namespaces or running containers.
The validation VM is stopped. Main, live demos and dirty worktrees are unchanged.

This proves direct RSA node admission and VSS synchronization, **not** Noise
peer handshakes, a Visa/service adapter, inter-node forwarding, node
restart/revocation, workloads or a production least-privilege launch contract.
Runtime images, full service trust/bootstrap, policy-signature enforcement,
operator configuration, licensing, setup/AI disclosure and all seventeen
scenarios/browser acceptance remain release gates. No public upload occurred.

### Offline runtime image and unprivileged Visa launch

Packaging commit `6e13d2a` adds explicit public dependency acquisition,
offline runtime assembly and exact-export acceptance. The new source-free
all-tools image contains the previously accepted archive from `a531bd4`,
an immutable Ubuntu 24.04 ARM64 base and 37 individually hash/size/version
recorded distribution packages. The saved base and package files are retained
for repeatable offline assembly; a future online acquisition may select newer
packages and requires new review. This is not a byte-identical-build claim.
There is no source, compiler, recovered credential or generated runtime state.
Copyright notices remain in the distribution's standard package paths.
Licensing/SBOM qualification remains open.

| Measurement | Result |
|---|---|
| Runtime gzip download bytes | 71,815,670 |
| Pristine rootfs regular-file payload bytes | 211,950,942 |
| Pristine exported rootfs tar bytes | 215,080,960 |
| Runtime download SHA-256 | `f4630f498754704362d5f611f6a07d773d056bec9294e8726338572d10233612` |
| Image ID | `sha256:4bdafbaeaf1bfe7d8a3aedce0c940b3b05d560605b615b9e2a821c4053829396` |
| Base image | `ubuntu@sha256:68ee22004a9109b44973313b02ff7b1de250f1de0f5ecb628d73f579aaff33ac` |
| Offline dependency receipt SHA-256 | `3aa324c2cca59f71ac8d4552a3209c9d940a8c98ef03108b34d6ca054e76931e` |

Rootfs measurement uses an unstarted container export before any state is
generated; regular-file payload excludes hardlinks/filesystem overhead. Docker's
engine-reported image size uses different layer accounting and is not presented
as extracted installation size. Whole-demo peak installation disk and a final
size budget are still unqualified.

The build loads the saved base, verifies its identity, assigns a local
digest-derived tag and installs the exact recorded `.deb` files with build
network `none`. No repository access is needed for assembly. Bind-mounted
package inputs do not enter image layers, and maintainer-script service startup
is disabled. The image defaults to printing Visa help, not silently starting
services. It exposes no port and uses UID 501/GID 1000 with initialized private
`/state` ownership.

The built image was removed and confirmed absent before loading the exact
saved gzip export. All three contract/policy/service checks then passed:

- UID 501/GID 1000, effective capabilities zero, no-new-privileges enabled.
- Read-only root, network `none`/loopback only, no published port, no host
  source/socket mount, no TUN and no privileged volume initializer.
- One private state volume per probe, PID limit 128, memory limit 512 MiB
  and CPU quota 1. These test limits are not whole-demo resource promises.
- All eleven executable hashes match the compiled archive. All seven Rust CLIs,
  five fresh-key signed policies and invalid-policy rejection pass.
- Two fresh Visa identities, verified TLS, foreign CA/hostname/plaintext/API-key
  rejection, observer read-only enforcement, authorized policy installation,
  non-overwrite and durable Redis/Visa restart all pass.

Acquisition initially exposed APT cache permission needs with all capabilities
dropped. The public-only temporary downloader now explicitly receives CHOWN,
DAC_OVERRIDE and FOWNER, without private mounts/socket/published ports. No such
capability is needed by the tested runtime. A stopped-container metadata read
was fixed by gathering package fields before exit. Dockerfile `FROM sha256:...`
was incorrectly interpreted as a repository name; the final build uses the
identity-checked local tag instead. None of these failures changed live demos.

Seventeen focused preparation/provenance/runtime rejection tests pass, including
wrong daemon, mutable base, changed base/package/archive, wrong architecture,
unsafe/symlink inputs and missing/failed archive acceptance. No Go/Rust source
changed. Builder, recipe, archive, dependency/smoke receipts and probe scripts
are digest-bound in the retained evidence.

The private recovery workspace retains `validation/runtime-inputs-final-20261023`,
`runtime-image-final-20261023` and `runtime-image-final-acceptance-20261023`.
Only allowlisted receipts were exported; all test credentials/databases lived
on disposable volumes. All owned containers/volumes were removed and their
absence verified. The validation VM is stopped; original/fallback VMs remain
running, main and original worktrees unchanged, default context `desktop-linux`.

This qualifies the loopback Visa/Redis engineering launch contract without
elevated runtime capabilities. It does **not** qualify PH container permissions,
TUN/network setup, Noise peers, Visa/service adapters, forwarding, node lifecycle,
public TLS, service supervision or production backup/restore. The earlier native
root-only node gate remains separate. This is one all-tools image, not final
service decomposition, full installation or public distribution. Next gates
are PH/adapter launch permissions, peer trust and forwarding, then remaining
setup/licensing/full-demo acceptance. No registry push or zpr.net upload occurred.

### Preconfigured PH container permissions and process restart

Packaging commit `2553f3d` qualifies the next engineering slice with the unchanged
saved runtime image (download SHA-256
`f4630f498754704362d5f611f6a07d773d056bec9294e8726338572d10233612`,
image ID `sha256:4bdafbaeaf1bfe7d8a3aedce0c940b3b05d560605b615b9e2a821c4053829396`).
The committed probe hash is
`d88dbdbd7d904a509f9924785784ad13b3899f18f7ab3fd186ed6465b53c393f`.
No binary, runtime image, Go or Rust source changed.

The separate Visa and PH containers use a unique internal IPv6-only Docker
bridge with engineering-only subnet `fd00:5a::/64`. No default route, published
port, host/source/socket mount, external network or Simulator is configured.
Short-lived root helpers receive only NET_ADMIN, join only owned namespaces,
and install private routes/addresses plus a multiqueue TUN owned by UID 501.
They do not receive credentials/state mounts or SYS_ADMIN. PH receives
`/dev/net/tun:rw`; Visa does not.

The running PH, Visa and Redis processes all verify as UID 501/GID 1000,
effective capabilities zero and no-new-privileges. Runtime containers have
read-only roots, one private state volume each, PID limit 128, memory limit
512 MiB and CPU quota 1. The node volume contains only node-specific credentials
and configuration; administrator/API keys, CA private key and database remain
in the separate Visa volume. A network-disabled unprivileged transfer helper
copies an explicit node file allowlist. A foreign RSA key is present only for
the negative test.

Required checks passed against the exact saved image:

- Missing TUN device: explicit TUN-create failure, no-such-file reason, exit 134.
- TUN owned by UID 502: explicit TUN-create failure, operation-not-permitted
  reason, exit 134. No runtime capability or world-writable ownership workaround.
- Foreign RSA bootstrap key: Visa's exact signature rejection and empty node
  inventory, not a generic network/authentication failure.
- Matching fresh key: PH grant and VSS registration; exact CN `node`, address
  `fd5a:5052::2`, synchronized state and zero pending installs; exactly one node.
- PH process stopped by exact captured PID and verified exited, then restarted
  in the same namespace/state: new grant/VSS receipts, identical selected
  synchronized summary and still exactly one accepted node.
- Exercised PH/Visa/admin hashes match the archived Rust manifest. All 23 focused
  preparation/provenance/runtime rejection tests pass, including wrong daemon,
  changed image/sidecar, invalid identity/target and existing-evidence refusal.

The negative harness initially assumed Rust's unwind exit 101; these preserved
release binaries abort with exit 134, which the final probe checks explicitly.
Attempting to change an existing persistent TUN's owner through `ip tuntap add`
returned busy; the final harness deletes/recreates only its own wrong-owned TUN
inside the disposable node namespace. No production behavior was weakened.
The inherited PH panic on unusable TUN remains a future setup diagnostic concern,
not a polished user-facing prerequisite check.

Reviewed selected receipts persist in
`validation/node-image-final-20261023` in the private recovery workspace.
Raw logs, generated keys and databases never left disposable volumes. Every
attempt's containers, private volumes and internal network were removed; zero
remaining containers/volumes/labeled test networks was independently verified.
The validation VM is stopped. Original/fallback VMs remain running, main and
original worktrees unchanged, default Docker context still `desktop-linux`.

This proves a **preconfigured node** can run without elevated runtime capabilities,
plus PH process re-admission. It does not prove adapter dynamic-address permission
requirements, Noise peer trust, allowed/denied forwarding, revocation, container/VM
restart or full-demo acceptance. Those networking/adapter tests remain next;
no claim is made that all networking gates are complete. The release skill now
includes this reusable probe. No public upload, deployment or main promotion.

### Adapter-to-node Noise trust and policy-controlled TCP

Commit `b271fad` adds explicit `--adapter` to the saved-image node probe, with a
test-only private fixture. It exercises the unchanged accepted runtime image
(`f4630f498754704362d5f611f6a07d773d056bec9294e8726338572d10233612`),
not another build or recovered deployment. Harness SHA-256:
`f1cee89e6c6fc80a19a0f38db9187ab7b1ad6442118c853b30153f83153d4a5a`;
fixture SHA-256:
`e48dda1ca65c4c5f29554d262d8f8312b2eb92c302343c0336caf0e50fe3d6aa`.
No Go/Rust source or executable changed.

A third owned container/volume joins the same internal IPv6-only bridge.
The fixture creates fresh RSA and X25519 material and a CA-signed `client`
certificate, extends the bootstrap policy and copies only adapter-specific
keys/configuration into that volume. Administrator/API keys, CA private key,
database and node keys are not exposed to the adapter.

The exact-image committed-harness gate passes:

- Unrelated CA rejects the node's Noise certificate with explicit signature
  verification failure and no `client` actor.
- Valid Noise certificate with foreign RSA bootstrap key produces Visa's exact
  signature rejection for CN `client`, still without an admitted client.
- Matching fresh material verifies CN `node` at the adapter and CN `client`
  at the node, reaches active dock, and registers a non-node actor.
- Visa's dynamic `fd5a:5052:adda:` address is actually present on the adapter TUN.
  No static claimed address is used.
- Two live ephemeral Redis TCP listeners on the node local actor both return
  PONG locally. Policy permits client only on 16379; that path returns PONG
  through the Noise adapter/node link. Port 16380 returns no PONG within five
  seconds, while the node records explicit policy `NoMatch` and the listener
  still returns PONG locally. A timeout alone is not treated as denial proof.
- Original node-only admission/TUN-negative/process-restart mode passes again.
  All 24 focused packaging/provenance/argument regressions pass.

The privilege boundary is deliberately different from a static node. The adapter
dynamically updates its TUN using `/usr/sbin/ip`; zero-capability execution fails.
Adding Docker NET_ADMIN to UID 501 did not retain permission across that exec
path; a nonroot ambient-capability attempt was also explicitly denied. The
qualified engineering adapter uses **UID 0 with NET_ADMIN only**, read-only root,
no-new-privileges, TUN device access, no default route/host mount/published port
and no privileged mode. Process-level UID/effective-capability checks pass.
Visa, Redis and node remain UID 501 with zero capabilities.
The separate network-disabled selected-file provisioning helper uses root
CHOWN/DAC_OVERRIDE only to populate its protected root-owned adapter volume.
A nonroot dynamic-address adapter solution is not claimed.

The first explicit IPv6 adapter bind omitted its interface scope, causing
incoming packet-info lookup to create a duplicate tether and Noise decryption
failure. The final fixture resolves its container's actual `eth0` interface
index and scopes the bind accordingly, without changing Core or trust checks.
Preassigning a claimed static address also left Linux choosing that rejected
claim as traffic source. The final fixture requests no static address and uses
the actual Visa grant; node source-identity enforcement remains intact.
An explicit private return route to the dynamic prefix completes local-actor
traffic. The Admin API omits the node flag on ordinary adapters; the probe checks
non-node semantics and normalizes only its selected summary.

Selected trust/traffic/process receipts persist in
`validation/adapter-image-final-20261023`; unchanged-mode regression receipts
are in `validation/node-image-regression-20261023`. Raw failed-attempt logs
were reviewed privately, never used as distributable evidence, and removed
with superseded outputs. Generated keys/databases stayed on disposable volumes.
All attempts' containers/helpers/workloads, private volumes and networks were
removed; zero remaining test resources independently verified. Validation VM
is stopped; both live demos, main, original worktrees and default context are
unchanged. The repeatable release skill and packaging README include this gate.

This proves **adapter-to-node** Noise trust, RSA admission, dynamic addressing
and one allowed/denied TCP path to the node's local actor. It does not qualify
node-to-node Noise, Visa/service adapters, adapter-to-adapter/inter-node traffic,
revocation, full container/VM lifecycle, production setup or whole-demo acceptance.
These remain release gates, not implied successes. No upload/deployment/promotion.

### Settled node peers and policy-controlled inter-node TCP

Commit `19c964b` adds mutually exclusive `--peer-node` mode to the exact-image
probe and a fresh private peer fixture. No Rust/Go source, executable, archive
or runtime image changed. Accepted runtime download remains SHA-256
`f4630f498754704362d5f611f6a07d773d056bec9294e8726338572d10233612`,
image ID `sha256:4bdafbaeaf1bfe7d8a3aedce0c940b3b05d560605b615b9e2a821c4053829396`.
Committed harness SHA-256:
`8d678c5171886f4b5845f9970fa0cb6eef4e44b64ece214a98975f92ec553994`;
peer fixture SHA-256:
`2aa3e536911dbf7b2cf7d37f1e0de93cb4e5021e31903954bb9023de7073e02f`.

The fixture generates a separate RSA bootstrap key, X25519 key and CA-signed
`peer` certificate, declares both nodes/substrate addresses and the policy link,
and pins the other node's Noise certificate at each end. A network-disabled,
unprivileged helper copies only node-specific keys/configuration and public
trust material into the peer volume; API credentials, authority private key and
database remain in Visa's volume. All three runtime containers run UID 501/
GID 1000, zero effective capabilities, read-only root and no-new-privileges.
The existing isolated internal IPv6 network and root NET_ADMIN-only namespace
setup contract remain unchanged.

The committed exact-image probe passed:

- Deliberately wrong peer Noise pin: exact certificate-exchange
  `KeyMismatchError`, without a verified peer name.
- Correct pins: verified CN `peer` at `node` and CN `node` at `peer`; both
  Hello directions complete and the same configured link becomes active.
- Exactly the two expected node actors at `fd5a:5052::2` and `fd5a:5052::3`,
  both synchronized with zero pending installs.
- Live `ph-cli` queries require exactly one active peer at the expected substrate
  and `Type: NodeToNode`, ten consecutive one-second samples at each end.
  Historical handshake logs or actor inventory alone are insufficient.
- Allowed TCP Redis PING on peer port 16379 returns PONG across the node link.
  Port 16380 times out with no PONG and explicit policy `NoMatch`; the denied
  listener still responds locally. Inter-node ZPR routes point only at TUN,
  not the substrate bridge.
- Restart of only the first PH process after settled-link qualification:
  VSS registration and synchronization return, both live node links pass the
  same stability checks, and both allowed/denied TCP checks pass again.
- Default node-only and adapter modes both pass with the same committed harness,
  including the shared TCP helper's stricter exact `NoMatch` assertion.
  All 26 focused bootstrap/runtime rejection/failure-receipt tests pass.

**Unresolved lifecycle limitation:** an earlier development probe restarted
immediately after initial actor synchronization, without live-link stability
qualification. It exposed a timing-dependent failure: the surviving peer receives
`TerminateLink(Reset)`, removes its node-peer entry and creates an ordinary
adapter tether for the returning node. Node Hello handling then fails and Visa
reports `NoRoute` during VSS initialization; restart timed out with exit 124.
The settled-link result is not a source fix for that earlier timing window.
Acceptance explicitly records `early_restart_race_resolved: false`.
General node lifecycle remains open; never interpret this as arbitrary-time
restart, container/VM restart or revocation qualification.

Selected diagnostics persist in
`validation/peer-early-restart-failure-20261023`. Its receipt explicitly identifies
the uncommitted development-harness provenance limitation rather than inventing
an exact harness hash or presenting it as accepted release evidence.
Exact committed-harness acceptance is separately retained in
`validation/peer-image-final-20261023`, with regressions in
`adapter-image-peer-regression-20261023` and `node-image-peer-regression-20261023`.
Image/harness/fixture digests, route receipts, mutual trust and before/after
traffic results were independently verified. Raw failed-attempt logs and all
superseded probe outputs were reviewed and removed; generated private keys and
databases stayed on disposable volumes. No containers, volumes or labeled test
networks remained before stopping validation. The validation VM is stopped;
both live recovery VMs remain running, main and original worktrees unchanged,
default context still `desktop-linux`.

Scope is **node local actor to peer node local actor over one pinned Noise
link**, with engineering-only direct Visa transport routes and
`local_vs_dock = true` on both nodes. This does not qualify remote Visa/service
adapters, adapter-to-adapter transit, early-restart behavior, revocation,
production setup or full-demo acceptance. No upload, deployment, registry push
or main promotion occurred. The packaging README and repeatable skill document
both the qualified slice and unresolved timing window.

### Received-Reset source fix and rebuilt image follow-up

Core source now preserves configured node identity on remote Reset/Shutdown,
waits up to the existing one-second terminate deadline for local reset
acknowledgement, and keeps local removal authoritative if a remote terminate
arrives while local reset cleanup is in progress. This shipped in isolated Core
revision `bdfc1f98745ba758dbf03646f2551ffe0c250b3e` after host PH library and
state-machine tests passed (`273 passed`, `2 ignored`, plus `9` focused link
tests in both lib and binary targets on native Linux ARM64).

The candidate probe now supports explicit `--peer-node-reset` in addition to
settled `--peer-node` and immediate `--peer-node-early`. Reset mode discovers
the configured live peer link IDs (no hardcoded link number), requires an
authenticated `Received terminate ... Reset` log while the sender is still
alive, then proves retained survivor identity, post-restart synchronization,
live NodeToNode stability and allowed/denied TCP rechecks.

Rebuilt exact artifacts:

- Rust release inputs: `validation/rust-reset-final-inputs-20261023`
  (`zpr-core` archive SHA-256
  `4522a3e6f16ebce87125c4ec73be537f99d4c6cdc32231784a2e3a23957cae3c`).
- Verified Rust release: `validation/rust-reset-final-release-20261023`;
  verifier receipt in `validation/rust-reset-final-release-verified.json`.
  Core binaries changed to `ph` SHA-256
  `e0fb74d842d761cb94831ac22c6a5d041dcb15c12b02af2b0083ba44a693c5a2`
  and `ph-cli` SHA-256
  `eb0ddedc434946ef857ed1849bcf68eece844195d7c5976830fee371678d5c71`.
- Compiled archive: `validation/compiled-reset-final-20261023/compiled-recovery-linux-arm64.tar.gz`,
  SHA-256 `4b98f53618e1c5534b6e5bff45b19d2cec8febb8fec2c7c83b6d5ac874df816a`,
  `33,543,789` compressed bytes and `87,955,600` extracted bytes.
- Runtime image: `validation/runtime-image-reset-final-20261023/image.json`,
  image ID `sha256:fed06250805e8a1c30734f07471d9f521c61db2df276d364645ae7aa00627658`,
  download SHA-256
  `a247c5b855907f32303d82981a8fd4ec7700942dcc185797d75b97647aa519d5`.

Final exact-image gates all passed with committed harness SHA-256
`0f89a8d646c973baf66993e34e9317857b5349a62c5446b5b63ae4a3e1dc8d8e`:

- Runtime contract/policy/service/durable restart:
  `validation/runtime-reset-final-attempt1-20261023`.
- Default node and adapter regressions:
  `validation/node-reset-final-attempt1-20261023`,
  `validation/adapter-reset-final-attempt1-20261023`.
- Settled peer restart: `validation/peer-settled-reset-final-attempt1-20261023`.
- Immediate restart with observed remote Reset:
  `validation/peer-early-reset-final-attempt1-20261023`
  (`remote_reset_delivery_observed: true`).
- Explicit received-Reset reconnect proof:
  `validation/peer-reset-reset-final-attempt1-20261023`
  (`peer_explicit_reset_tested: true`,
  `received_remote_reset_reconnect_verified: true`,
  retained survivor link ID `3`).

`early_restart_race_resolved` intentionally remains `false`; this engineering
slice still does not claim arbitrary-time lifecycle, revocation, remote
Visa/service adapters, adapter-to-adapter transit, container/VM restart
behavior, production setup or complete demo acceptance.

Superseded raw failure outputs were removed after preserving selected JSON
receipts in `validation/peer-reset-development-20261023`.

### Limited preview goal and rights-review status

The next product goal is an invite-only, self-hosted Linux ARM64 download for
only explicitly tested workflows. Claude assistance may be an opt-in using
each tester's own key only after the optional-AI acceptance gate passes; core
workflows must remain usable when AI is skipped or unconfigured. This does not
assert five-organization/seventeen-scenario completion. No participant
distribution, public download, website change, or promotion has been authorized
or performed. The exact staged gates are in the external
`COMPILED-DOWNLOAD-PLAN.md`.

Initial component inventory for the current archive
(`4b98f53618e1c5534b6e5bff45b19d2cec8febb8fec2c7c83b6d5ac874df816a`) is
retained under `validation/limited-preview-license-review-20261023` and is
**not** redistribution clearance:

- 526 Rust ARM64 normal/build dependency package versions; 31 declared license
  expressions; four crates have no Cargo license field, though their exact
  source roots have Apache-2.0 license files, staged and mapped by source
  revision pending package-level review. License-text discovery mapped all
  526 package versions: 522 from local exact source roots and four from
  repository root files at the exact commits recorded in the crate archives.
  The 294 unique texts and 918 package/file references are hash-indexed in the
  private evidence directory; this source-file inventory is not legal
  clearance. MPL-2.0 packages are present and their notice/source obligations
  need review. Exact-source ARM64 package-level Cargo trees confirm `colored`
  3.0.0 in `zplc`, `colored` 3.1.1 in `ph`, `vs`, `vs-admin`, and `zpt`, and
  `option-ext` in `ph-cli`. `vsapikey` is a binary target in package `vs`; its
  target-specific linker closure is not established. An exact Linux ARM64
  `cargo check --locked --offline --bin vsapikey` attempt stopped because
  `aws-lc-sys` could not find `aarch64-linux-gnu-gcc`; this is not closure
  evidence and must be repeated in a suitable Linux ARM64 build environment.
  The archive still lacks required participant-facing notices and any
  required source materials. A separate private source candidate has been
  assembled from the exact lockfile-checksum-verified `.crate` archives for
  `colored` 3.0.0/3.1.1 and `option-ext` 0.2.0, preserving their packaged
  sources and MPL-2.0 notices. It is not part of the runtime archive or
  participant-available; source-delivery instructions, complete notices,
  covered-scope review, and legal review remain open.
- 20 Go module versions linked into the four release binaries. A recursive
  scan of those exact module source roots found 37 conventional license/notice
  files (30 unique content hashes); all 37 are staged and hash-verified in the
  private inventory, nine beyond the earlier top-level collection. The
  additional files include the pinned SQLite module's detailed third-party
  scope notice and additional memory/libc notices. Its scope table separates
  linked, test-only, module-graph-only, and repository-only components, and
  places MPL-2.0 `hashicorp/golang-lru` in its module-graph-only section. The
  dashboard is the only release binary with `modernc.org/sqlite`; source
  imports include the driver and `lib`, but no optional `vec` or `vfs` imports
  were found. A Linux ARM64 `go list -deps` query resolves only the driver's
  `sqlite`, `lib`, and `vtab` packages; `hashicorp/golang-lru`, `vec`, and
  `vfs` are absent. This is package dependency evidence, not a linker map or
  legal clearance; the filename scan is not proof of complete notice coverage.
  None of the Go notices are in the compiled archive. Follow-up resolved the
  82-module graph offline in a disposable copy, with normal checksum verification and no
  source `go.mod`/`go.sum` changes; the graph receipt is retained in the
  license-review evidence directory. Hash-verified source indexes help
  navigate the staged files; they are not a license classification.
- 129 installed OS packages (37 retained `.deb` inputs); each maps to a
  `/usr/share/doc/<package>/copyright` path in the exact image. The private
  staged set has 129 package mappings and 92 distinct notice contents, including
  one symlinked shared notice. Exact parsing of nonempty literal Debian
  `License:` field values yields 230 distinct identifiers across 117
  machine-readable package copyright files, with 2,442 license stanzas
  indexed. Twelve package copyright files have no parseable `License:` field
  and remain on the manual-review list. The corrected token-boundary triage
  flags 97 package records with GPL/LGPL/MPL/AGPL/GFDL or content-license
  labels.
  These labels are unnormalized triage data, not legal conclusions. The exact
  text of the eight distinct legacy notices covering the 12 records has now
  been reviewed and summarized privately, including GCC runtime-exception,
  glibc library-versus-utility, and library-versus-tool/documentation
  distinctions for several packages. Exact installed-file scope and the
  remaining Ubuntu base/legal review stay open. The exact
  Ubuntu base layer has 92 installed packages; all 92 resolve to copyright
  content matching the staged runtime notice set, including three shared-doc
  symlink paths. Ubuntu's `/etc/legal` directs readers to per-package copyright
  files; base-image redistribution terms and participant notice bundling still
  need review. The exact base layer's 17 `/usr/share/common-licenses` entries
  (14 files, three symlinks) are now staged with hashes. Of 15 literal
  common-license references in package notices, 13 resolve exactly; `gpgv`
  names absent `GPL-2.0` while `GPL-2` exists, and `gzip` names absent `GFDL-3`
  while `GFDL-1.3` exists. No alias equivalence is assumed; those references
  and base redistribution terms remain open for review.
- The archive currently includes only four project/frontend notice files, not
  the full Go/Rust/OS third-party notice set. All 53 bundled demo assets now
  match files in the hash-verified `zpr-visaservice` source archive at commit
  `4ebc2c00d0506f897f7f461fe8c1bcdc0c5681fc`, and its root Apache license
  matches the archive's `PROJECT-LICENSE`. The five relevant source READMEs
  are not included in the archive and do not establish provenance/ownership
  for every named organization, person, and directory record; that review
  remains open.

The rights-review task remains active. No release rights are inferred from
build success, package metadata, or the presence of a license file alone.

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
