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

## Included

- Stripped, statically built Linux ARM64 web runtime (including embedded browser
  assets), trusted-service helper, lab trust generator and configuration validator.
- Tracked JSON, LDIF, ZPL and ZPLC example assets, including five organizations,
  seventeen scenarios and reviewed topology. ZPL/ZPLC files are required runtime
  policy inputs, not compiler source trees.
- Source revision/toolchain metadata, embedded-browser license notices,
  project license, Go web-runtime module build information and SHA-256 file and
  archive inventories.

The archive root contains `bin/`, `share/examples/`, `licenses/`, `build.json`,
`go-build-info.txt`, this document and `SHA256SUMS`. It contains no source
checkout, Go/Rust tools, node_modules, caches, database, generated identity,
operator credential, AI key or recovered runtime snapshot. The Northstar LDIF
reader password field is only the existing `DEMO_BIND_PASSWORD_HASH` placeholder;
real passwords must be generated during provisioning.

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

## Remaining release gates

The full compiled download still needs node/adapter/Visa/compiler binaries and
runtime images, license/provenance review for every dependency, a complete
fresh-identity/service bootstrap, independent operator configuration, safe
first-run setup with optional AI disclosure/key storage, and clean-install plus
full scenario/browser acceptance. Go build information is not a complete SBOM.
The included notices do not establish redistribution rights for all modules.
No public upload or main promotion is authorized by an engineering smoke pass.
