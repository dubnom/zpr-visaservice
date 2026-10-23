# Compiled recovery engineering slice

This is an **internal validation artifact**, not the promised complete demo
download, a supported installer, or a public release. The initial target is
Linux ARM64. Build and smoke validation must not change the working recovery
VM or use its credentials.

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

## Offline smoke validation

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
policy installation and persisted policy/network state after Visa restart.
Owned child PIDs are stopped on exit; no shared database is cleared or reused.
All generated keys, database files and full evidence remain **private**.
The enclosing harness must remove private test state after preserving only
reviewed nonsecret receipts.

The recovered `vs-admin` client currently disables server-certificate validation;
its successful calls are **not** TLS evidence. The probe independently uses curl
without insecure flags for verified HTTPS and a wrong-CA negative check, and
compares its policy/network responses with CLI results. Correcting and rebuilding
that client remains a publication gate. Node trust admission, policy-signature
enforcement, a runtime image and full service/bootstrap acceptance are not proved
by this engineering probe.

## Remaining release gates

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
