# ZPR and Simulator demo package contract

Status: the requested two-member provisioning slice has passed acceptance on the
existing Ubuntu workbench. Clean-host downloaded-artifact acceptance and the
remaining full-demo scenarios are still open. This is deliberately less strict
than the production contract. A passing demo is not a production release or a
supported customer installation.

This package is a runnable dataplane demonstration, not a complete working ZPR
network under the
[product-wide minimum](../../../System%20release%20plan.md#common-minimum-for-a-working-zpr-network).
In particular, this contract does not provide an independent Control Room and
operator service or prove durable, queryable logging. Demo-specific substitutes
may simplify implementation, but do not waive the common functional baseline;
do not describe this pair as a baseline-complete network.

## Goal

Produce two separately downloadable but version-matched Linux/amd64 archives:

1. **ZPR dataplane demo package:** the existing
   `zpr-demo/multinode-demo` scenario,
   including its runnable ZPR service/node/adapters, sample organization and
   policy, demo web workload, runtime configuration/setup, and stop/status
   instructions.
2. **Simulator companion:** the actual Visa `zpr-web-dashboard -mode simulator`
   runtime plus the exact scenario/profile inputs and helper artifacts it needs
   to exercise the matching ZPR demo package.

The package build may use the source repositories and existing build tools.
Installing and running the resulting archives on the test host must not require
those source checkouts, Rust/Go toolchains, or a rebuild. The archives carry a
shared demo release ID and exact component/source/image identities so the
companion cannot silently pair with a different ZPR demo. Publish neither
archive from this implementation task.

## Approved demo-only shortcuts

- Target one disposable Ubuntu 26.04.1 LTS x86-64 machine with local Docker Engine
  and Compose. Do not claim support for multi-host, Mac, ARM, production, or
  upgrade scenarios.
- Reuse the multinode demo's TUN, `NET_ADMIN`, and privileged-container setup
  rather than waiting for production least-privilege work. The demo README and
  installer must list the exact host requirements and warn against shared,
  exposed, or production hosts.
- The current Simulator launcher mounts `/var/run/docker.sock` into the
  Simulator container so it can manage scenario containers. The user explicitly
  approved that root-equivalent host access only for isolated disposable demo
  hosts. Do not broaden socket access to ZPR services. Make the warning
  prominent before installation.
- Use isolated demo/sample organization data and test workloads. Do not include
  a company trust root, company credentials, private signing keys, reusable
  passwords, or live company data in either archive. Any generated demo secrets
  are created on the target host and remain outside the distributable archive.
- Bundled login, policy, DNS, Gateway, or log services may use existing demo
  substitutes where needed. Describe their limits and ensure their failure
  cannot be confused with production readiness. Demo Gateway acceptance is
  limited to explicitly proxy-configured test clients; do not claim transparent
  endpoint enforcement.
- The artifacts may be unsigned and labeled `demo-unqualified`; include
  checksums and clear provenance. This does not waive ordinary safe archive
  handling or permission checks.

## Required contents

Each archive must contain:

- A manifest with the same demo release ID, Linux/amd64 target, required peer
  archive ID, component IDs, source revisions or immutable image IDs/digests,
  and SHA-256 hashes for every payload.
- Prebuilt images/binaries and all runtime files needed for its package role.
  No mutable image tag may be the sole component identity.
- Non-secret sample configuration and one command-driven start/stop/status
  path, with clear ownership of generated runtime state.
- A verification command that validates the archive, manifest, exact peer
  package, and all payload checksums before extraction or installation.
- A short demo operator guide with host requirements, download/verification,
  install, start, test steps, expected outcomes, logs, stop/restart, cleanup,
  and limitations.

The existing `zpr-system-bundle` validator/packer/verifier supplies useful
manifest and archive primitives, but its current schema is a generic
engineering-bundle format. The current tools are not installers and do not
build or supply these demo artifacts.

## Image-only candidate archive staging

The first paired archive-building slice now exists in
`zpr-demo/multinode-demo/packaging/package-demo-images.py`. It exports only
the locally available Linux/amd64 ZPR and Simulator images into separate
archives. It records one shared release ID, each payload's SHA-256, the
platform-specific image-config digest, Docker's inspect ID, and the exact peer
image identities. `verify` checks both archives, sidecars, payload digests,
archive paths and links, Docker image metadata, platform, and cross-package
pairing before `install` loads either image. `install` imports images only; it
does not start containers or install runtime support files.

Example from `zpr-demo/multinode-demo`. The output directory must already
exist; the builder refuses to overwrite same-release archive names.

```sh
python3 packaging/package-demo-images.py build \
  --release-id demo-candidate-20261018-safe-context \
  --output-dir /path/to/new-empty-output-directory \
  --zpr-image zpr-multinode:demo-candidate-20261018-safe-context \
  --simulator-image zpr-simulator:demo-candidate-20261018
python3 packaging/package-demo-images.py verify \
  /path/to/new-empty-output-directory/zpr-demo-demo-candidate-20261018-safe-context.tar \
  /path/to/new-empty-output-directory/simulator-demo-candidate-20261018-safe-context.tar
```

The builder requires Python 3, Docker CLI/daemon, and both candidate images
already loaded locally. The verifier/loader also require the Python script,
Python 3, and Docker. SHA-256 sidecars detect corruption only when compared
with a separately trusted checksum; they are not signatures or provenance.
This staging script and its generated inner image-only archives are engineering
artifacts, not complete installable packages: the verification command is
not embedded in these inner archives, and runtime files and lifecycle commands
are absent. Importing the inner archives requires the separate helper script;
the published outer downloads below include that helper.
Do not use this slice as evidence of the complete downloaded-artifact acceptance
test.

### Published image-import checkpoint (October 18, 2026)

The authenticated zpr.net portal now publishes matching outer `.tar.gz`
downloads at version `dev-2026-10-18.1`. They wrap the inner image-only pair
`demo-candidate-20261018-safe-context`, checksum sidecars, and README
instructions. The ZPR wrapper includes `package-demo-images.py`; unlike the
inner archives alone, these downloads do not require a source checkout to
verify the pair and import images.

| Outer download | SHA-256 |
| --- | --- |
| `zpr-dev-2026-10-18.1-linux-x86_64.tar.gz` | `ef64a73263f5341d27aee0122c472957f57576b05e0dae53ffd5e60d9cdfecce` |
| `simulator-dev-2026-10-18.1-linux-x86_64.tar.gz` | `40f2e07512cc7274ae6634fa92a79cc440fec58e59a45164baf1b22383ad59be` |

Publication readback confirmed both catalog versions and checksums; anonymous
HTTPS requests returned 401. Local extraction of the outer downloads and
verification with their included helper passed. Authenticated HTTPS download
was not independently captured.

The operator subsequently reported successful download, extraction, and
loading of both images into Docker on the designated Ubuntu host. This closes
only the operator-reported image-import milestone. No captured host preflight,
container-TUN validation, service startup, policy/workload test, or lifecycle
acceptance has occurred. The next implementation must generate demo identities
and configuration on the target host and supply a source-independent runtime
with start/stop/status commands. Do not rebuild images merely to repeat the
completed image-import step, and do not describe image loading as service
installation or startup.

### Bundled Audit UI package slice

The ZPR runtime archive now includes the regular static files from the reviewed
`zpr-audit/public` source directory and a package-owned `serve-audit.py`
launcher. Build requires the explicit `--audit-public-dir` input; isolated
candidate preflight requires that directory's `index.html` in the selected
`zpr-visaservice` source snapshot. Archive verification requires both the
launcher and `audit/index.html`; symlinks and private/credential-named files are
rejected, and test-prefixed assets are omitted. Package tests cover asset
inclusion, safe install, and local HTTP serving. The launcher binds to `127.0.0.1` and
serves only static files. This is not an Audit backend integration, does not
connect the UI to production audit data, and does not close clean-host,
Simulator-scenario, or complete-demo acceptance.

### Paired Simulator-world source implementation

The source package runtime now passes the installed ZPR package and persistent
state paths to the paired Simulator launcher. The Simulator launcher validates
both package manifests and their reciprocal immutable image IDs, derives the
Compose project name from the shared release ID, and passes the exact project,
ZPR image ID, and release ID to the Simulator container. The package-builder
test verifies these launcher assets are present in the installed package.

The Simulator now exposes a fixed, versioned world-state API and an operator
page for the observed Alice and OciWeb containers, adapter sessions, OciWeb
HTTP service, and demo persona. Machine start/stop acts only on pre-existing,
ownership-checked containers; machine start restores the corresponding fixed
adapter session from packaged configuration. Service actions target only the
OciWeb HTTP process and require its adapter session to remain running. The
packaged web0 entrypoint keeps the container supervisor alive if Nginx stops,
separating HTTP service stop from adapter/container stop. Adapter state means
the fixed process session is present; it is not proof of an active peer link.
The paired Compose configuration marks this supervisor capability in the
container environment, and the API verifies exactly one matching marker before
service start/stop. Containers from older packages without the marker remain
observable and machine-controllable, but service actions fail closed until the
updated paired runtime recreates web0.
Alice is reported as a bootstrap-approved demo principal, not a
human-authenticated user or production enrollment.

This is a source implementation, not fresh-host acceptance. The separate
20-machine Simulator fixture remains unprovisioned. Package/runtime, API, and
authorization tests do not prove container behavior on Ubuntu. The Simulator
image must be rebuilt from the updated Go and embedded UI sources, and the
paired runtime archives must be rebuilt to include the web0 supervisor
entrypoint. No image has been built or published from this implementation,
and the designated Ubuntu host has not been changed.

### Opt-in persistent organization machine candidate

The separate `organization-provisioning-20261019-r4` candidate adds real
provisioning for `demo/demo-machine-01`, not only lifecycle of existing paired
containers. Its verified pair was installed on the existing Ubuntu workbench,
and the installed Simulator launcher was tested with
`ZPR_SIMULATOR_ENABLE_PROVISIONING=1`. This is not a fresh-OS qualification or a
published release.

The organization profile explicitly selects the fixed isolated bootstrap driver.
The machine receives an endpoint-generated persistent key and an independent
node/Visa/OciWeb plane; it does not use the reference topology placeholders.
Real protected HTTP, attribute withdrawal with explicit denial and acknowledged
visa revocation, access recovery, identity-retaining stop/start, Simulator
replacement with persistent inventory, and cancellation followed by retry were
verified. Retry creates replacement partial containers without replacing the
endpoint key. The main Simulator container-state map recognizes the real machine,
while incompatible legacy fixture login/workload controls remain unavailable.

The existing paired runtime was not replaced: its configuration and active policy
must remain unchanged, and its Alice/OciWeb service-supervision acceptance is still
a separate outstanding item.
See the [provisioning scope and commands](../simulator/README.md#packaged-organization-machine-provisioning).

### Two-member workbench acceptance (October 21, 2026)

The unqualified paired candidate
`organization-shared-topology-20261021-r7` was installed and exercised on the
existing Ubuntu workbench, alongside the preserved baseline. Both configured
members reached `ready` with successful protected HTTP. They retained distinct
endpoint fingerprints and Docker identity volumes while reporting the same
Visa, node, and OciWeb container IDs. A same-name wrong-key attempt for
`demo-machine-02` was rejected in the Visa log; the approved actor, policy, and
traffic checks remained intact.

Each member was stopped and started independently through the Simulator UI.
The stopped member's fingerprint remained unchanged, the other endpoint and
shared services stayed online, and each restarted endpoint returned to `ready`
with protected HTTP. No duplicate endpoint containers were created. After
separate UI authorization, both members also passed policy withdrawal and
recovery: fresh protected HTTP was denied, two installed visas were revoked and
acknowledged for each member, and the same approved actor regained HTTP access
after its attribute was restored without restarting the shared service.
Host-reboot recovery, the full 20-fixture inventory, clean-OS installation, and
production enrollment remain unqualified. The downloaded pair remains
`demo-unqualified`; it was not published.

## Runtime and lifecycle boundary

- Keep the ZPR demo and Simulator companion as distinct packages with distinct
  runtime/state directories and Compose project names. Simulator may control
  its provisioned demo scenario through the approved local Docker socket, but
  ZPR services must continue running when the Simulator UI/container stops.
- Bundle the reviewed `verify-packaged-demo.sh` probe in the Simulator runtime
  package and configure it only for the demo profile. The probe must verify
  the exact installed ZPR project/image and leave the active policy unchanged;
  never point packaged-demo verification at a host-provided script.
- The packaged demo profile uses `activate-packaged-demo.sh` as a demo-only
  activation/restore gate: it verifies the exact installed ZPR pair and
  preserves identity and policy state. It rejects all other organizations.
  The development `scripts/activate-organization.sh` depends on a source
  checkout and `.local-runtime`, so it must not be mounted into the downloaded
  package. Cross-organization execution remains unqualified until each
  organization has its own package-owned activation/reset contract.
- Preserve the current multinode demo's service process boundary for this
  viability test. Do not silently turn the Simulator into a ZPR service or
  make ZPR production services depend on Simulator APIs or files.
- The installer must extract to a new, operator-selected or documented fixed
  directory, validate archives before extraction, and refuse path traversal,
  symlinks, existing target replacement, and unsafe permissions.
- First-run setup may create throwaway demo runtime identities/state under a
  private target-host directory. Re-running start/stop must preserve it. A
  separately named destructive demo reset may remove only the demo-owned
  state after an explicit confirmation; never make reset the default.
- Provide actionable failure status and leave state inspectable. The first
  demo milestone may require manual cleanup for failed setup, but must document
  each step and must not silently delete unrelated Docker resources or volumes.

## Downloaded-artifact acceptance

Run on a clean disposable Ubuntu 26.04.1 LTS x86-64 host with no source checkout:

1. Download both archives; verify hashes, manifest contents and exact pairing.
2. Install each package independently. Confirm the documented Docker socket,
   TUN, capabilities, and port requirements before startup.
3. Start ZPR from its archive; verify the demo node and adapters register and
   the sample workload responds through the ZPR path.
4. Start the Simulator companion; load the paired scenario and exercise a
   Simulator action against the running ZPR demo. Verify the expected policy
   allow and deny outcomes and retain logs/screenshots or command output.
5. Stop the Simulator companion. Confirm ZPR remains running and the workload
   still behaves as specified. Restart Simulator and confirm state is preserved.
6. Stop/restart ZPR and verify the documented persistence boundary. Exercise
   explicit reset/cleanup only against the demo's own resources.
7. Record exact archive and image IDs, host details, commands, observed results,
   known failures, and every manual step. A source build or mocked service is
   not downloaded-artifact acceptance.

The designated demo test host is confirmed as Ubuntu 26.04.1 LTS x86-64. The
new read-only `multinode-demo/packaging/preflight-demo-host.sh` records the
actual OS, Docker/Compose versions, detected disk/memory and whether
`/dev/net/tun` is accessible before installation. The user expects sufficient
memory/disk and Ubuntu TUN support; the actual host and Docker-container TUN
behavior remain to be checked. The demo image is built on Ubuntu 24.04 and
relies on Ubuntu runtime packages; the 26.04 host does not change the image's
user-space base. Verify Docker/Compose behavior, host TUN access,
privileged-container behavior, and port availability on the actual host before
calling the target qualified.
Ubuntu 24.04 is no longer the designated downloaded-artifact acceptance host;
the unrelated production installer contract retains its own 24.04 scope.

The demo target is not met until these exact downloadable archives pass on the
clean host. A successful local unit test, image build, bundle validation, or
source-tree Compose launch is only an earlier development check.

## Known blockers to producing the pair

0. **Local candidate images and image-only archives now exist, but they are
   not complete packages.** On
   2026-10-18, the Simulator image built from the current dashboard worktree as
   `zpr-simulator:demo-candidate-20261018`, Linux/amd64, manifest-list digest
   `sha256:56883f24a96ff9a153f16ce2abc36c4695cc33560a559bb5303e46038d9f4ce0`.
   Its executable starts in an isolated network-disabled container and prints
   CLI usage. The ZPR image built from the existing demo binaries as
   `zpr-multinode:demo-candidate-20261018`, Linux/amd64, manifest-list digest
   `sha256:b262e1a6163a751d06803a28f272149c6fa40467e383d410d3ed4e1aa6facdce`;
   a network-disabled image check found the expected `vs`, `ph`, `zplc`, and
   `vs-admin` binaries. These are mutable local candidate tags, built from the
   current uncommitted workspace and existing demo binaries, not a clean,
   pinned, or release-qualified build. The image-only archive pair was staged
   under the session's private artifact directory for release
   `demo-candidate-20261018`, then verified together. The archived Docker
   config digests are ZPR
   `sha256:1600c6606ced3a33eba1ebffea8ee62bd09078cbfc6c31891bb1d17ef266d6a9`
   and Simulator
   `sha256:6df1eae80746162d6ceb18fc9261a7e5d77161a213416034d284d9c13cc66e23`.
   The corresponding archive SHA-256 values are ZPR
   `b02ece64cc094013e0c233e2ba2f8b5e12d4cb9e5c629861949873868b78adda`
   and Simulator
   `cb726e50f654e50af36adadd6c9c99b644451c6a1f239b178f124041c57dbaed`.
   These archives contain image payloads and manifests only; they do not meet
   the required contents or provide a download/install/start/test path.
   A subsequent ZPR candidate was rebuilt as
   `zpr-multinode:demo-candidate-20261018-safe-context` using a temporary
   Docker context containing only the existing `Dockerfile` and nine
   top-level Linux/amd64 executables from `multinode-demo/bin`. No source
   checkout, runtime state, or key files were sent in that build context. The
   resulting image reports Linux/amd64 and passed a network-disabled check for
   the expected `vs`, `ph`, `zplc`, and `vs-admin` files. Its Docker inspect
   image ID is
   `sha256:660a7907dbb98150e49e9f99627b8410666d6205ce719334ad498861756210f5`;
   its archived image-config digest is
   `sha256:6e79922b3429cb4614f7f982bf97d2daa2388e969fef0381ede9a9861294cdbb`.
   A refreshed image-only pair for release
   `demo-candidate-20261018-safe-context` was staged in the session's private
   artifact directory and verified together. Archive SHA-256 values are ZPR
   `a755ef741074ef8a6f99855dc22b8ffd2e067240a060e8859853facc3438fff0` and
   Simulator
   `6ea5551d4380a0eb363062bc4d87e5d74b908ba78ee2f293517e1cca4430a836`.
   The executables are pre-existing artifacts with no verified source revision
   or build record, and the mutable Ubuntu package layer remains a demo-only
   candidate. This archive pair still lacks runtime support files and does
   not enable self-contained install/start/test. Network-disabled CLI help
   smoke checks passed for `vs`, `ph`, `zplc`, `vs-admin`, and
   `zpr-web-dashboard`. `zpr-dashboard` did not pass a no-config invocation:
   it reads `config.toml` before argument handling and exited with that missing
   file error. No service was started.
1. `multinode-demo/local-compute/deploy-docker.sh` derives component paths from
   the developer workspace and expects sibling repositories plus compiled
   artifacts. It is not a package-root installer. Its legacy
   `multinode-demo` path copies the checked-out `zpr-conf/include` into runtime
   containers, while identity generation and policy compilation in the
   organization path require `zpr-pki`, source-relative profiles, and
   `.local-runtime` pre-generated inputs. These are not suitable archive
   contents: private keys must not be distributed, and generated demo
   identities must be created in protected target-host state. A package-owned
   initialization path must replace these source-relative inputs before
   either deploy path can be reused.
2. The ZPR Compose file expects a locally available `zpr-multinode` image and
   developer/demo bind-mounted entrypoints and runtime directory structure.
   There is no packaged image/archive and install wrapper for a new host.
3. `scripts/dashboard-stack.sh` now supports
   `ZPR_SIMULATOR_PREBUILT_IMAGE=1`: it requires a locally loaded Linux/amd64
   Simulator image and fails instead of building from the dashboard checkout.
   The default development behavior still builds the image. The
   [multinode package-mode wrapper](../../../../zpr-demo/multinode-demo/packaging/run-simulator-package-mode.sh)
   requires the dashboard support-tree path and image reference, then forces
   prebuilt mode for package startup. Focused shell tests cover the image-mode
   selection and wrapper handoff without starting services. This avoids only
   the Simulator image build at startup: `dashboard-stack.sh` still bind-mounts
   the dashboard, core, DNS, observability, and multinode-demo source trees, so
   full startup is not independent of the source workspace.
4. The ZPR and Simulator runtime artifacts do not currently have a shared
   release ID, package manifests, exact compatible component closure, or
   pairing check tied to their actual runnable images and profiles.
5. No end-to-end demo acceptance script/report has been run using both
   downloaded archives on a fresh Ubuntu host.

## Production hardening retained as explicit follow-up

The demo shortcuts above do not close these production issues:

- replace privileged/TUN broad access and Simulator Docker-socket access with
  reviewed least-privilege service and management boundaries;
- select production service ownership, supported node/endpoint contracts,
  Enrollment lifecycle and sole-writer APIs, company identity/attribute and
  logging integrations, and credential delivery;
- lock compatible immutable production images, source revisions, toolchains,
  state backend, DNS and Gateway artifacts; finish license/SBOM review;
- complete service-identity CA custody, rotation, backup/recovery, operator
  trust, secure first-admin provisioning, and production secret handling;
- implement production DNS and Gateway activation, client routing, firewall
  contracts and protocol-level readiness;
- provide data-safe transactions, upgrades, rollback, backup/restore,
  production acceptance, signing and independent release trust.

Keep those requirements in the [production system release
plan](../../../System%20release%20plan.md). Passing the demo acceptance above
must not be described as production readiness.
