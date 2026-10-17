# Engineering management-bootstrap kit

Status: **engineering only, unsigned and unqualified**. This kit packages the
existing company management-bootstrap validator and host preflight for Ubuntu
24.04 x86-64. It is a read-only validation step, not a ZPR installer or a
complete network.

The kit does not install or start any ZPR service, generate company identities,
render runtime configuration, configure enrollment, or validate service
readiness. It does not use Simulator. The included example is illustrative;
replace it with company-owned configuration and protected files.

## Build the engineering bundle

From this directory in a source checkout whose tracked dashboard files match
`HEAD`, build on a machine with the repository and Go toolchain/dependencies
already available. The packager refuses a modified tracked source tree and
does not fetch dependencies:

```sh
sh package.sh /tmp/zpr-management-bootstrap-linux-amd64.tar.gz
```

The script builds the real `zpr-system-config` command for Linux amd64 with
dependency downloads disabled. The bundle contains that binary, the existing
`preflight.sh`, the example input, this guide, a status manifest, and SHA-256
checksums. It contains no customer credentials or private keys.

The manifest labels the artifact `UNSIGNED/UNQUALIFIED`. Its
checksums detect accidental byte changes but do not authenticate the publisher.
There is no vendor signature or verification root in this engineering bundle.
Do not use it as a production release.
The source revision identifies the tracked dashboard sources; packaging files
may still be uncommitted. Checksums identify the shipped payload, including
the manifest and wrapper, but are not reproducible-build or provenance proof.
The checksum file cannot checksum itself. The output parent must already exist;
existing output paths, including symlinks, are never overwritten.

## Run the validation-only bootstrap

On the intended Ubuntu 24.04 x86-64 host with a local Docker Engine and Compose
v2 already installed:

```sh
tar -xzf /tmp/zpr-management-bootstrap-linux-amd64.tar.gz -C /tmp
/tmp/zpr-engineering-bootstrap/bootstrap.sh /path/to/company.json
```

The preflight checks the host and queries the existing local Docker Engine and
Compose baseline. Thus an operator bootstrap run is read-only, but is not a
no-service-contact operation. It does not pull images or start containers.
The validator
then checks the supplied company configuration and, with `-check-files`, its
referenced local TLS, OIDC secret, and optional CA files. The bootstrap is
read-only and ends with an explicit statement that it applied no configuration
and installed or started no services.

Successful checks do not establish browser trust, OIDC discovery/login, DNS,
firewall reachability, container mounts, service health, endpoint admission, or
packet enforcement. The sample configuration is not a customer identity or
credential. Follow the system release plan for the complete installation
requirements; that production bundle is not available yet.
The bootstrap preserves a failing preflight or validator exit status and never
runs the validator after failed preflight. It supplies `-check-files` on every
validator invocation. No rollback, reset or removal state is created.

## Verify the bundle

The package includes `SHA256SUMS` for its payload files. Verify them after
extraction with either `sha256sum -c SHA256SUMS` or `shasum -a 256 -c
SHA256SUMS`. These hashes are not a substitute for vendor-owned release
verification material.
The wrapper does not automatically verify these checksums; inspect and verify
the extracted files before an explicitly authorized operator run. Never add
company configuration, private keys or secrets to the distributable archive.

Run the offline engineering checks from this source directory (the test script
is not included in the bundle):

```sh
sh test.sh
```

The tests build a temporary bundle and inspect its contents and hashes. They do
not install software, contact a service, or certify a fresh Ubuntu host.
They check exact inventory, source/status, Linux amd64 ELF identity, unchanged
copied inputs, tamper detection and no-overwrite behavior. Wrapper ordering,
mandatory file-check arguments and exit statuses are tested with disposable
**synthetic stubs**, not a real Docker daemon or company services. Existing
focused validator tests use temporary synthetic credentials, and existing
host-preflight tests use fixtures. These are engineering checks, not real
service integration, paired-package or end-to-end traffic acceptance. Build
scratch and test artifacts are removed; cached dependencies only are used.
