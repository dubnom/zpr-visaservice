# Compiled recovery engineering slice

This is an **internal validation artifact**, not the promised complete demo
download, a supported installer, or a public release. The initial target is
Linux ARM64. Build and smoke validation must not change the working recovery
VM or use its credentials.

The project-level [zpr-compiled-release skill](../../../.github/skills/zpr-compiled-release/SKILL.md)
guides reproduction and resumption of this workflow, with safety boundaries,
gate dependencies, evidence handling and completion criteria. Open this
repository as a workspace for skill discovery. The skill delegates execution
to these scripts; it does not authorize publication or main promotion.

From a clean committed candidate, with cached Go dependencies:

```sh
GOPROXY=off GO=/absolute/path/to/go \
  sh packaging/compiled-recovery/build.sh /absolute/private/new-build
```

The builder needs Go, Git, jq, tar, gzip and shasum. It refuses dirty source
and existing output paths. Failed builds leave their explicit output directory
for diagnosis; choose a new directory after correcting the error.

To include the seven separately verified Rust release tools, supply their
release directory and independent input receipt explicitly (requires Node.js):

```sh
GOPROXY=off GO=/absolute/path/to/go \
  sh packaging/compiled-recovery/build.sh /absolute/private/new-assembled-build \
  /absolute/verified-rust-release /absolute/release-inputs.json
```

The assembler validates the complete release receipts before creating output,
then revalidates and digest-checks the copied executables. Only the seven
allowlisted tools and a selected-field public provenance manifest are included;
Cargo logs, source, cache, private runtime evidence and signing keys are not.
Existing binaries and assemblies are refused. `build.json` declares the optional
`rust-release.json` manifest. Invalid/missing evidence blocks assembly.
This remains an engineering artifact, not a service installer.

## Included

- Stripped, statically built Linux ARM64 web runtime (including embedded browser
  assets), trusted-service helper, lab trust generator and configuration validator.
- Tracked JSON, LDIF, ZPL and ZPLC example assets, including five organizations,
  seventeen scenarios and reviewed topology. ZPL/ZPLC files are required runtime
  policy inputs, not compiler source trees.
- Source revision/toolchain metadata, embedded-browser license notices,
  project license, Go web-runtime module build information and SHA-256 file and
  archive inventories.

The archive root contains `bin/`, `libexec/`, `share/examples/`, `licenses/`, `build.json`,
`go-build-info.txt`, this document and `SHA256SUMS`. It contains no source
checkout, Go/Rust toolchains, node_modules, caches, database, generated identity,
operator credential, AI key or recovered runtime snapshot. The Northstar LDIF
reader password field is only the existing `DEMO_BIND_PASSWORD_HASH` placeholder;
real passwords must be generated during provisioning.
Packaged directories/executables use mode `0755` and public assets use `0644`,
so runtime access does not require the builder's UID or extra capabilities.
The enclosing output directory and archive remain owner-only. Archive creation
excludes host filesystem extended attributes (requires tar `--no-xattrs` support).
With explicit Rust inputs it additionally includes seven dynamically linked
Linux ARM64 tools and `rust-release.json`. They require a compatible Linux
loader, glibc/GCC runtime, OpenSSL 3 and libpcap; no shared-library tree or base
image is silently copied from the recovery VM.

## Source-free runtime image engineering gate

The archive by itself deliberately omits Linux shared libraries. The separate
runtime image adds an immutable Ubuntu 24.04 ARM64 base, recorded distribution
packages and **the exact accepted compiled archive**, not a source checkout.
It is a single all-tools engineering image, not the final per-service image
layout, installer, supported minimum platform or public release.

Dependency acquisition is an explicit online step using only public Ubuntu
inputs in a temporary container. Supply a reviewed immutable base digest,
an isolated Docker context and its expected daemon ID:

```sh
node packaging/compiled-recovery/acquire-runtime-inputs.cjs \
  /absolute/private/new-runtime-inputs explicit-context expected-daemon-id \
  ubuntu@sha256:REVIEWED_ARM64_DIGEST
```

Acquisition saves the base image, all downloaded `.deb` files, package names,
versions, architectures, byte sizes and SHA-256 hashes. APT verifies repository
metadata/package hashes; the saved input receipt binds subsequent offline
assembly to those files. Updates require explicit reacquisition/review; the
first acquisition does not pretend that a moving APT repository is reproducible.
Retain the entire protected input directory, not just its receipt.
The temporary downloader has network access and only CHOWN, DAC_OVERRIDE and
FOWNER capabilities for APT's cache ownership/permissions; it has no private
source/state mount, Docker socket or published port. It is removed afterward.
These acquisition privileges are **not** runtime requirements.

The dependency set includes glibc/GCC runtime inherited from the base, OpenSSL 3,
libpcap, CA certificates, Redis, shell/core utilities, iproute2, curl, jq and
util-linux for engineering launch/readiness diagnostics. No compiler, SDK,
source checkout or recovered configuration is installed. Package copyright
notices remain under the distribution's standard `/usr/share/doc/` paths; the
complete installed-package inventory is `/opt/zpr/runtime-packages.txt`.
Distribution/package license review and a release SBOM remain separate gates.

Final assembly is offline, using the saved base plus exact package files:

```sh
node packaging/compiled-recovery/build-runtime-image.cjs \
  /absolute/accepted-compiled-build /absolute/private/runtime-inputs \
  /absolute/private/new-image-build explicit-context expected-daemon-id
```

The compiled build must have a matching eight-check `smoke-evidence/result.json`
receipt. Changed archive/base/package hashes, unsafe members, missing files,
wrong architecture and failed/missing acceptance block assembly.
An existing output is refused. Failures retain partial output for diagnosis.
The builder loads the saved base, verifies its identity and assigns a local
digest-derived tag because Dockerfile `FROM sha256:...` is interpreted as a
repository name, not a local image ID. No registry resolution is needed for
the loaded tag. Build RUN instructions have network `none`; dependency packages
are bind-mounted into the build step, not copied into final image layers.
No package manager contacts a repository during assembly. Maintainer-script
service startup is disabled explicitly.

Outputs are `runtime-image.tar.gz`, its digest/size in `image.json`, and the
reviewable generated context/recipe. The receipt binds archive, offline smoke
receipt, dependency receipt, builder script, Dockerfile and image identity.
The source archive's revision remains explicit even if packaging scripts have
advanced. This establishes repeatable offline inputs, not byte-identical builds:
package maintainer scripts and image timestamps are not normalized.
Docker/BuildKit with local Docker driver and bind-mount RUN support is required.
Nothing is pushed to a registry.

Validate the exact saved image, loading it back through Docker:

```sh
node packaging/compiled-recovery/probe-runtime-image.cjs \
  /absolute/private/image-build /absolute/private/new-evidence \
  explicit-context expected-daemon-id
```

The three disposable containers run as UID 501/GID 1000, read-only root,
`--network none`, `--cap-drop ALL`, no-new-privileges, PID limit 128, memory limit
512 MiB and CPU quota 1. These are observed engineering test limits, not a
supported whole-demo sizing claim. Each has only its own private `/state` volume;
no host/source/socket mount, TUN device or published port is present. The image
initializes `/state` ownership so no privileged volume initializer is needed.
All immutable release content lives under `/opt/zpr`; default command only
prints Visa help and never silently starts a deployment.

The contract probe verifies all eleven binary/archive digests, effective UID,
zero capabilities, no-new-privileges, loopback-only networking and absence of
build tools/source/Docker socket. The other probes execute all seven Rust CLIs,
five fresh-key signed example policies, invalid-policy rejection, two independent
Visa preparations, TLS/API-key/observer rejection checks, admin policy install,
non-overwrite and durable Redis/Visa restart. Visa and Redis need **no** elevated
capabilities for this loopback HTTPS/private-Unix-socket contract.
Reviewed public receipts are copied out; generated signing keys, service
credentials, raw service logs and databases remain only on disposable volumes.
Owned containers and volumes are removed on success or failure; cleanup errors
fail the command.

This does not qualify PH container privileges, TUN/network setup, Noise peer
handshakes, Visa/service adapters, forwarding, public TLS, production volume
backup/restore, multi-service supervision or a whole-demo installer. The separate
native `--node` gate below still has its explicit root-only namespace boundary.
Never use a privileged container as an implicit workaround for these remaining
launch-contract gates.

## Offline scratch smoke validation

```sh
node packaging/compiled-recovery/smoke.cjs \
  /absolute/private/new-build \
  explicit-isolated-docker-context \
  expected-docker-daemon-id
```

The harness verifies the archive digest, extracts the exact archive into a new
directory, verifies all file digests and membership, checks binary architecture
and reviewed asset shape, and imports it as a temporary scratch image.
It requires an explicit context and matching daemon ID. Docker context selection
is not changed. The daemon must be an operator-selected isolated Linux ARM64
test daemon; identity matching alone cannot establish isolation.

Only the extracted archive reaches Docker. Containers have no network, source
mounts, Docker socket, capabilities, compiler, shell or build cache. A new private
volume holds generated test state. The harness invokes compiled policy
composition for all five organizations and generates independent lab trust
twice; different public CA certificates prove the archive did not ship a shared
identity. It also checks that the trust generator refuses existing output.
Only public CA certificates and composed example policies are copied back.
Owned containers, volume and image are removed on success or failure; failed
cleanup is an error. Host extraction/evidence remains under the build directory.

This checks offline executable/asset portability, **not** LDAP/OIDC/DNS
provisioning, runnable ZPR nodes/adapters, policy compilation/install, UI login,
all seventeen scenarios, AI setup, or supported minimum resources. It does not
run an HTTP server or expose ports. No generated trust material is a public
release artifact.
When Rust tools are included, the scratch harness checks their membership,
architecture, file size and recorded release hashes, but does **not** execute
them without their dynamic libraries. Its receipt explicitly records
`rust_runtime_tested: false`. Native runtime acceptance below must run the
binaries and assets extracted from the exact archive, not the input staging
directory, before claiming the assembled Rust contents were exercised.

## Native Rust prerequisite and policy probe

The separate [Rust runtime probe](probe-rust-runtime.sh) runs on an explicitly
isolated Linux ARM64 host, not against a running deployment:

```sh
sh probe-rust-runtime.sh /absolute/compiled-bin /absolute/examples /absolute/new-evidence
```

It requires the seven separately built `ph`, `ph-cli`, `vs`, `vs-admin`,
`vsapikey`, `zplc`, `zpt` executables plus the compiled `zpr-web-dashboard`.
These Rust binaries are included only when the explicit Rust assembly inputs
above are supplied.
Only run reviewed locally built executables; `ldd` is not a safe scanner for
untrusted executables. ELF/CLI probes do not prove binary source equivalence.
Inputs and the evidence parent must be in operator-protected directories.

The probe checks ELF architecture and actual dynamic loader resolution, records
hashes and CLI output, creates a new private RSA signing key, composes and
signs all five organizations' policies, and checks invalid syntax is rejected
without producing a bundle. Warnings are retained in each compiler log.
No daemon, listener, network scenario, policy installation or service reset is
performed. Existing evidence paths are refused; partial evidence remains after
failure for diagnosis. The new private signing key stays in the evidence
directory and **must never be bundled or published**.

On the fresh Ubuntu 24.04 ARM64 validation VM, initial PH/PH-CLI startup failed
because `libpcap.so.0.8` was absent. Installing `libpcap0.8t64` resolved it.
Visa/Compiler/Policy Tester additionally use OpenSSL 3; all seven use the
platform dynamic loader, glibc and GCC runtime. Exact observed package versions
and CLI receipts are private validation evidence, not a supported ABI matrix.
The earlier Rust build logs identify development builds (some later stripped),
not release-profile builds. The separate release builder below has subsequently
passed source/profile/hash verification and this same native runtime probe.
Those candidate release outputs remain private and separate from the original
engineering archive; service/bootstrap qualification and dependency/license
review are still required before public packaging.

## Fresh isolated Visa bootstrap

The archive includes [prepare-visa-service.sh](prepare-visa-service.sh) under
`libexec/`. It consumes the release-built tools, not a source checkout:

```sh
sh /absolute/extracted/libexec/prepare-visa-service.sh \
  /absolute/extracted/bin /protected/parent/new-visa
```

This **engineering-only** generator creates a fresh RSA authority, loopback
HTTPS leaf certificate, node/Visa bootstrap public-key pairs, policy-signing
key, distinct read/write and read-only API keys, a fresh service identity,
minimal compiled policy and explicit Visa configuration. A completed
`visa-bootstrap.json` is written last. No service is started or trusted by the
host; no recovered keys or Simulator manifests, profiles or APIs are consulted.
It requires OpenSSL and the verified release tools. Paths must be canonical
absolute paths using ASCII letters/digits, slash, dot, underscore or hyphen,
with an existing protected nonsymlink parent. Existing state is refused,
including symlinks. Directories are owner-only and files are private through
`umask 077`. Failures retain partial private output with no completion inventory.

The generated state is tied to its output path (including its private Redis
Unix socket), not relocatable after generation. The minimal policy and
loopback-only configuration do not deploy an organization or node. They are
not production defaults. The compiler warning about no policy granting fabric
admin access remains visible; authenticated HTTPS administration is separately
tested. This state must never be added to the engineering archive.

The separate [native service probe](probe-visa-service.sh) requires Linux ARM64,
Redis server/tools, curl, OpenSSL, timeout and iproute2 on an isolated test host.
Run it as an unprivileged user inside a caller-created network namespace with
only loopback enabled; it refuses ordinary network interfaces. For example,
using explicit test UID/GID, protected paths and the namespace setup below:

```sh
sudo unshare --net sh -c 'set -eu; ip link set lo up
  exec setpriv --reuid=TEST_UID --regid=TEST_GID --clear-groups \
    sh /absolute/probe-visa-service.sh /absolute/extracted/bin \
    /absolute/extracted/libexec/prepare-visa-service.sh /protected/new-evidence'
node --test packaging/compiled-recovery/prepare-visa-service.test.cjs
```

The probe creates two independent installations, verifies owner-only permissions
and non-overwrite behavior, starts its own Redis via a private Unix socket
(no TCP listener), then starts Visa on namespace loopback. It verifies TLS
using curl and requires rejection of another installation's CA/API key,
anonymous rejection, read-only observer enforcement, successful administrator
policy installation and persisted policy/network state after both Visa and
Redis are stopped and restarted (without supplying the initial policy again).
Owned child PIDs are stopped on exit; no shared database is cleared or reused.
All generated keys, database files and full evidence remain **private**.
The enclosing harness must remove private test state after preserving only
reviewed nonsecret receipts.

The repaired `vs-admin` client requires HTTPS and verifies server trust, expiry
and endpoint identity. Its local TLS regressions cover every request method,
including credential non-disclosure on rejected handshakes. The native probe
requires foreign-CA, hostname, plaintext and foreign-API-key CLI failures to
return exit 1 with empty stdout. It also independently uses curl without
insecure flags and compares verified policy/network responses with CLI results.
Only newly release-rebuilt verified client inputs satisfy these checks; older
seven-tool inputs with the insecure client must not be published.
Node trust admission, policy-signature
enforcement, a runtime image and full service/bootstrap acceptance are not proved
by the service-only mode.

### Explicit native node-admission gate

Adding `--node` to preparation also generates a fresh X25519 Noise key and
authority-signed certificate with CN `node`, plus a PH node configuration that
uses the separately generated RSA node bootstrap key. OpenSSL 3 creates these
standard PKCS#8/X.509 files; no source tree or extra PKI executable is required.
Visa's transport endpoint becomes `[fd5a:5052::1]:5002`; HTTPS administration
remains `[::1]:8182`.
The node configuration uses `fd5a:5052::2` for its ZPR address, `zprtest0` for
its TUN and `[fd00:5a::2]:5000` for the isolated substrate. These are fixed
**engineering-test addresses**, not host setup or production defaults. Ordinary
two-argument preparation retains its previous loopback-only behavior.

The service probe's optional `--node` gate requires root **inside a new isolated
network namespace**, Linux TUN, `nsenter`, `unshare` and jq. It creates a second,
temporary node namespace and a private veth pair between the two; neither
namespace has an external interface or default route. For example:

```sh
sudo unshare --net sh -c 'set -eu; ip link set lo up
  exec sh /absolute/probe-visa-service.sh /absolute/extracted/bin \
    /absolute/extracted/libexec/prepare-visa-service.sh /protected/new-evidence --node'
```

The probe first requires explicit RSA signature rejection when a real PH node
uses another installation's bootstrap private key, with no node in Visa's
inventory. It then starts PH using the matching fresh key, requires Visa's actor
to have the exact CN/address, `in_sync: true` and zero pending installs, and
requires PH's own access-grant and VSS-registration receipts. The negative
configuration explicitly selects the foreign key: the preserved Core version
loads config-file authentication keys after CLI arguments, so the probe does
not rely on a CLI override. That inherited precedence behavior is unchanged.
Visa's source-address check remains enabled; separate namespaces provide the
real expected connection source rather than weakening the check.

Both PH attempts, the temporary namespace anchor, Visa and Redis are stopped
by exact owned PIDs on exit. The calling outer namespace's lifetime removes
the private link; no named global namespace, host route, Docker network,
image pull, listener publication or host TUN is created. The test runs as root
for namespace/TUN operations, not as the unprivileged service-only probe.
Treat all generated state/logs as private; only the selected node summary,
grant/rejection lines and inventories are suitable for reviewed receipts.
This proves direct RSA node admission and VSS synchronization. It does **not**
prove Noise peer handshakes, a Visa adapter, inter-node forwarding, workloads,
node restart/revocation or the production least-privilege/runtime-image contract.

## PH container launch and process restart

The [node image probe](probe-node-image.cjs) qualifies an additional narrow
launch contract using the **unchanged exact saved runtime image**:

```sh
node packaging/compiled-recovery/probe-node-image.cjs \
  /absolute/private/image-build /absolute/private/new-node-evidence \
  explicit-isolated-context expected-daemon-id
```

The image build must retain `context/runtime/rust-release.json` for independent
binary-hash comparisons. Inputs/checksums, daemon identity and ARM64 architecture
are verified before creating resources; existing evidence is refused.
Docker must support internal IPv6-only bridge networks and the guest must
already expose `/dev/net/tun`. Missing prerequisites fail explicitly.

The probe creates a unique internal IPv6-only Docker bridge and exactly two
runtime namespaces, Visa at `fd00:5a::3` and PH at `fd00:5a::2`. The fixed
`fd00:5a::/64` subnet is engineering-only and overlap is an error, never a reason
to alter another network. Neither namespace has an IPv4/IPv6 default route;
no port is published and no host/source/socket directory is mounted. This is
an internal Docker bridge, **not** the `--network none` service-only gate.
The Docker daemon necessarily creates the bridge in the isolated guest.

Short-lived setup containers join only those owned namespaces, run as root
with **NET_ADMIN only**, and create the private addresses/routes plus a persistent
multiqueue TUN owned by UID 501. They have read-only roots, no-new-privileges,
no private state mounts and no SYS_ADMIN/privileged mode. The PH container
receives `/dev/net/tun:rw`; Visa does not. Persistent-TUN ownership lets the
existing PH binary open its queues/set carrier without NET_ADMIN at runtime.
The exact preconfigured node address is intentional; this does not qualify an
adapter dynamically assigning addresses or changing network routes.

Both runtime containers use UID 501/GID 1000, cap-drop ALL, read-only roots,
no-new-privileges, PID limit 128, memory limit 512 MiB and CPU quota 1.
Effective capabilities are verified on the running PH, Visa and Redis processes,
not inferred solely from Docker flags. No root volume initializer is needed.
Visa/Redis state and PH state occupy separate disposable volumes. An unprivileged,
network-disabled transfer container copies only the node's authority certificate,
Noise certificate/private key, RSA node key and config; administrator/API keys,
CA private key and database are not exposed to PH. A second RSA key exists in
PH state solely for the explicit negative test.

Before admission, missing TUN device and a TUN owned by UID 502 must each cause
the release-built PH to abort with exit 134 and the exact TUN-create reason:
missing file vs operation not permitted. This inherited panic/abort behavior
is recorded, not presented as a polished setup diagnostic. No Rust source is
changed. A wrong-owned TUN is removed/recreated inside the disposable namespace,
not reassigned by weakening permissions or adding runtime capabilities.

Next, a foreign RSA key must produce Visa's explicit signature rejection and
an empty node inventory. The matching fresh PH must be granted access, register
VSS, and appear under the exact CN/address with synchronized state/zero pending
installs. PH is stopped by its exact captured PID, verified exited, restarted
with the same node state, and must again receive its own grant/VSS registration.
Visa must still show exactly one synchronized node. This tests a **PH process
restart**, not a container/VM restart, revocation or full lifecycle recovery.

Only selected public node summaries, inventories, grant/rejection lines, process
status/route receipts and binary hashes leave the volumes. Raw logs/credentials
and Redis data never leave them. Containers (including helpers), both private
volumes and the internal network are removed in dependency order on exit.
Success is recorded only after cleanup succeeds.

This proves a capability-free, preconfigured **node** launch and re-admission.
It does not prove Noise peer handshakes, adapter admission/dynamic addressing,
allowed/denied workload forwarding, revocation or production network supervision.
Those are the next gates; no privileged runtime container is justified by this
engineering probe.

### Optional adapter trust and traffic gate

Add `--adapter` to the same node image probe to exercise an additional real PH
adapter. Default node-only behavior is unchanged:

```sh
node packaging/compiled-recovery/probe-node-image.cjs \
  /absolute/private/image-build /absolute/private/new-adapter-evidence \
  explicit-isolated-context expected-daemon-id --adapter
```

The [adapter fixture](prepare-adapter-probe.sh) extends fresh private Visa state
inside its container with a new RSA bootstrap key, X25519 key/certificate signed
by the new authority, and a minimal compiled traffic policy. It is a test-only
fixture, not packaged first-run setup, and does not modify the image or binaries.
An additional private volume holds only the adapter's allowlisted configuration
and keys, including deliberately foreign CA/RSA material for negative tests.
No administrator/API key, CA private key or database is copied there.

The adapter joins the same internal IPv6-only bridge at `fd00:5a::4`, with no
default route or published port. It verifies the node's certificate and the node
verifies the adapter's certificate during Noise XX. An unrelated CA must produce
the exact certificate-signature rejection and no `client` actor. The matching
certificate with a foreign bootstrap RSA key must then produce Visa's exact
signature rejection and still no client actor. Finally the correct keys must
yield an active dock and a non-node `client` actor with a dynamically assigned
`fd5a:5052:adda:` address actually present on its TUN. The probe does not preassign
a claimed adapter ZPR address.

Unlike the statically preconfigured node, the adapter updates its TUN during
docking. The preserved Linux implementation invokes `/usr/sbin/ip`. Nonroot
zero-capability execution fails with operation-not-permitted; merely adding
Docker NET_ADMIN for the nonroot process does not preserve that capability through
its subprocess exec path. The tested engineering adapter therefore runs as
**UID 0 with NET_ADMIN only**, not privileged mode, with read-only root,
no-new-privileges and only `/dev/net/tun:rw`. Process status must confirm exactly
that effective capability, UID and no-new-privileges. Visa/Redis/node remain
nonroot/capability-free. A nonroot dynamic-address adapter design is not qualified
by this gate; no code or enforcement check was weakened to manufacture one.

The adapter's selected-file transfer helper needs root with CHOWN/DAC_OVERRIDE
only to read protected source files and initialize its private root-owned volume.
It has network `none`, read-only source volume/root, no socket/host mounts and is
removed afterward. These are explicit provisioning privileges, not additional
adapter runtime capabilities.

IPv6 packet-info reception includes the local interface index. The preserved PH
looks up adapter tethers by local scoped address; an unscoped explicit IPv6 bind
created a second tether and handshake decryption failure. The fixture resolves
the owned container's actual `eth0` index and uses that scope in the adapter bind,
without changing Core. Treat this as an explicit launch-contract requirement,
not a claim that arbitrary IPv6 binding is qualified.

For traffic, two ephemeral TCP Redis PING listeners run on the node's local ZPR
actor at ports 16379 and 16380, with persistence disabled. Both must return PONG
locally before the remote test. Policy permits `client` only on 16379; 16380's
service permits only the node. The adapter must receive PONG from 16379, while
16380 times out with no PONG **and** the node records explicit
`visa request denied: Denied { code: NoMatch, ... }`. The denied listener must
still return PONG locally afterward. A timeout alone is insufficient proof.
The namespace-only return route to the dynamic adapter prefix is installed by
the setup helper, not added to a live host.

This proves adapter-to-node Noise trust/admission and one allowed/denied TCP path
to the node's local actor. It does **not** prove node-to-node Noise, service/Visa
adapters, adapter-to-adapter or inter-node forwarding, revocation or full-demo
acceptance. Successful receipts contain only selected summaries, address/process
status, grant/rejection lines and traffic results. If an attempt fails, its raw
runtime logs are saved under owner-only `private-failure/` for diagnosis; they
are never public evidence and must be removed after review. Generated keys and
database always stay on disposable volumes. Cleanup covers all three runtime
containers, every helper/workload, private volume and internal network.

### Experimental node-peer regression gate

The separate `--peer-node` mode extends the exact-image probe with two fresh
nodes and an explicitly compiled topology:

```sh
node packaging/compiled-recovery/probe-node-image.cjs \
  /absolute/private/image-build /absolute/private/new-peer-evidence \
  explicit-isolated-context expected-daemon-id --peer-node
```

This mode is mutually exclusive with `--adapter`. The
[peer fixture](prepare-peer-probe.sh) generates a second RSA bootstrap key,
X25519 key and authority-signed certificate, declares both substrate addresses
and their policy link, and provisions each node with the other's pinned Noise
certificate. It copies only node-specific keys/configuration and public trust
material to separate private volumes. The authority private key, API credentials
and database remain only in Visa's volume. Both nodes use UID 501/GID 1000,
zero effective capabilities, read-only roots and UID-owned persistent TUNs;
only short-lived namespace setup helpers have NET_ADMIN. Both nodes have
engineering-only direct Visa transport routes and `local_vs_dock = true`.
This is not the final remote Visa-adapter topology.

First the initiating node is deliberately given an unrelated Noise pin.
`KeyMismatchError` must occur with no verified peer name. After replacing only
the selected pin, both nodes must synchronize with zero pending installs and
exactly the two expected node actors. The configured peer link must log the
expected verified CN, both Hello directions and active state at each end.
An admitted actor alone is not proof of Noise success. Live `ph-cli` queries
require exactly one active link to the expected substrate and `Type: NodeToNode`
for ten consecutive one-second samples at each end, both before and after
single-node process restart. TCP PING across the peer link must succeed on the
allowed port 16379 and fail on 16380 with explicit `NoMatch`, no PONG and a
still-healthy destination listener. Both policy checks run again after restart.
The only kernel route between node ZPR addresses is through each node's TUN,
not the shared substrate bridge.

**Earlier early-restart race:** an earlier probe restarted immediately after
initial actor synchronization, without the live-link stability qualification.
That exposed a timing-dependent failure in the unchanged accepted PH binary.
The surviving node receives
`TerminateLink(Reset)`, removes its node-peer entry and subsequently creates an
ordinary adapter tether for the returning node. That node cannot complete the
node Hello exchange or re-register VSS; Visa reports `NoRoute`. Settled-link
restart succeeding does not fix or qualify this earlier timing window.
Historical receipts explicitly record `early_restart_race_resolved: false`; this is not
general node lifecycle acceptance. Do not restart both nodes, omit a failing
assertion or weaken pinned trust to claim the race fixed. A source fix and new
verified binaries/image would require fresh affected-gate qualification.

The isolated Core fix separates remote session teardown from local node-peer
authorization: received Reset/Shutdown clears the session but retains its pinned
identity, whereas local reset/topology removal still deletes the peer.
Its received-termination dispatch regression must fail with the old behavior
and pass with the fix, including local deletion and unchanged adapter semantics.
Do not infer that a saved image contains this fix from a newer harness revision:
check its archived Core source and binary receipts.

Use `--peer-node-early` instead of `--peer-node` for immediate PH restart after
initial synchronization and mutual Hello, with no pre-restart stability interval
or TCP warmup. Before starting the returning node it requires exactly one
retained NodeToNode identity at the survivor, then requires live link stability,
both synchronized actors and allowed/denied TCP after restart. The receipt
records whether the remote Reset was actually observed. Delivery is not
guaranteed by the old shutdown path; an unobserved Reset emits an explicit scope
warning and does not qualify received-Reset handling. Native dispatch tests and
runtime restart results are separate evidence, not interchangeable proxies.
General arbitrary-time/stress lifecycle remains unqualified.

Failure receipts identify the exact image, harness/fixture hashes, failing
phase, exit status and completed resource cleanup; failures exit nonzero and
never produce a successful `result.json`. Raw logs under protected
`private-failure/` remain private diagnostic material: inspect them, preserve
only reviewed failure lines, then remove those exact owned raw logs.
This slice covers node-local actor traffic across one pinned peer link, not
adapter-to-adapter transit, remote service/Visa adapters, revocation,
container/VM restart or complete installation.

## Remaining product gates

The [native Rust release builder](build-rust-release.sh) builds only the seven
required Rust binaries from committed source archives, with locked dependencies,
offline Cargo, two build jobs, release optimization and debug-information
stripping. It records toolchain, lockfile hashes, Cargo artifact/profile JSON,
compiler warnings and output hashes. Run it inside a pinned, network-disabled
ARM64 builder with source mounted read-only and separate writable Cargo-cache
and output directories. Do not mount credentials, host sockets or live state.

```sh
sh build-rust-release.sh /input/sources /build/new-release
```

The source root must contain the preserved Core, shared Common (including its
policy/VS API gitlinks), Visa Service and Compiler repositories. A prepared
offline Cargo registry/Git cache and native build prerequisites are required;
missing dependencies fail rather than download. The calling harness must record
the exact source revisions/archive hashes and builder image digest; this script
cannot establish Git provenance from arbitrary directories. Existing output
is refused; failed builds leave their diagnostics/cache for investigation.
Successful compilation is not yet runtime or end-to-end qualification.

Before runtime testing, verify the copied output against the external input
receipt:

```sh
node packaging/compiled-recovery/verify-rust-release.cjs \
  /absolute/copied-release /absolute/release-inputs.json
node --test packaging/compiled-recovery/verify-rust-release.test.cjs
```

The verifier requires all seven named binaries, ARM64 ELF headers and matching
output hashes, successful Cargo completion, optimization level `3`, disabled
debug assertions, non-test executable profiles, expected release output paths
and lockfile hashes matching the source input receipt. It rejects missing or
altered evidence rather than inferring release mode from stripped symbols.
The input receipt must be independently generated from the exact committed
archives supplied to the build; it is engineering evidence, not signed supply
chain attestation. Runtime/library and policy-build probes remain separate.

The full compiled download still needs runtime images and service launch
integration for the now optionally assembled binaries, license/provenance
review for every dependency, a complete
fresh-identity/service bootstrap, independent operator configuration, safe
first-run setup with optional AI disclosure/key storage, and clean-install plus
full scenario/browser acceptance. Go build information is not a complete SBOM.
The included notices do not establish redistribution rights for all modules.
No public upload or main promotion is authorized by an engineering smoke pass.
