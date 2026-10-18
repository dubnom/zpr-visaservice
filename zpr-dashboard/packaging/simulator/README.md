# Simulator companion packaging

Status: **Engineering slices only**. The immediate target is a
version-matched Simulator companion archive paired with the ZPR demo package,
not a production companion release. There is no installable companion archive
or clean-host acceptance yet. See the [demo package
contract](../system/DEMO-PACKAGE-CONTRACT.md) for its allowed shortcuts, exact
acceptance test and current blockers, and the
[approved production companion scope](../../../System%20release%20plan.md#simulator-companion-package)
for the separate production requirements.

The demo-only launcher mounts the host Docker socket into Simulator so it can
manage scenario containers. This grants root-equivalent host access and is
allowed only on an isolated disposable demo host. Do not use it on a shared,
exposed or production host.

The companion will supply Simulator, lab LDAP, OIDC and optional bootstrap DNS.
Protected ZPR service DNS belongs to the real ZPR package, including its adapter,
Visa Service publisher integration and persistent state; it is not owned by
Simulator. Existing bootstrap-tool generation of protected DNS configuration is
an engineering input, not evidence that this packaging handoff is complete.
The real ZPR
package will consume their explicit standard integration settings, using the
same artifacts and installer as a production company installation. Supporting
services must remain available when the Simulator application is stopped.
Production Control Room/Control-Service must not read Simulator state or call
Simulator APIs.

The two packages together must demonstrate a real ZPR network: node/adapter
admission, real allowed traffic, evidenced policy denials, protected DNS,
revocation and restart. OpenLDAP and LDAP-backed Dex are approved providers.
BIND9 has separate bootstrap and ZPR service roles; do not route setup/login
DNS through a network that has not yet been admitted. See the
[combined acceptance target](../../../System%20release%20plan.md#simulator-companion-package).

## Linux/amd64 bootstrap CLI engineering staging

The `zpr-lab-bootstrap` tool is built as a standalone Linux/amd64 binary;
installation and use of the binary do not require Go or a source checkout.
From the `zpr-dashboard` module directory, select an absolute output file path
outside this repository whose parent exists and whose file does not:

```sh
sh packaging/simulator/build-cli-linux-amd64.sh /tmp/zpr-lab-bootstrap-linux-amd64
```

The script records the exact local Go version, requires Go 1.26.1 or newer
(the module minimum), disables toolchain/module downloads, and uses read-only
module mode with `GOOS=linux`, `GOARCH=amd64`, `GOAMD64=v1` and
`CGO_ENABLED=0`. It applies `-trimpath` and `-buildvcs=false`, then verifies the
output is an ELF64 little-endian amd64 executable without running it. The exact
caller-selected path is used; an existing path or any path inside the checkout
is rejected. Build intermediates are temporary and cleaned on failure.

These flags and an observed Go version make the staging step explicit, not
byte-reproducible. The repository does not pin an exact Go toolchain, and these
engineering outputs are unsigned, have no release provenance and are not
release-qualified. No configuration, credential, key or trust material is
included.

Run the isolated staging regression script with:

```sh
sh packaging/simulator/test-cli-linux-amd64.sh
```

It builds only into a temporary directory that is removed on exit, exercises
success and failure/no-write cases, and verifies target metadata without
executing the binary. Required module dependencies must already be cached;
the checks do not download modules or toolchains.

## Local lab trust bootstrap

The `zpr-lab-bootstrap` command creates unique lab-only TLS identities and
integration secrets without starting services or contacting any network.
It reads no Simulator configuration or environment variables.

Engineering invocation from the dashboard source directory:

```sh
go run ./cmd/zpr-lab-bootstrap \
  -domain lab.example.test \
  -output /absolute/protected/parent/new-lab
```

Choose an explicit DNS suffix and create a protected parent directory first.
The parent must already exist and must not be group- or other-writable; the
output must be a new canonical absolute directory path. The operator is
responsible for ownership and protection of the parent and its ancestors.
The command does not audit their ownership or change permissions.
Release packaging must ship the binary, not require Go or a source checkout.

Generated files:

| Files | Purpose |
| --- | --- |
| `lab-ca.pem`, `lab-ca-key.pem` | Per-install ECDSA P-256 lab CA, valid for 365 days, no subordinate CA; DNS name constraints restrict issued names to subdomains of the chosen lab suffix |
| `room-chain.pem`, `room-key.pem` | Server identity for `room.<domain>` for the real Control Room in this lab |
| `simulator-chain.pem`, `simulator-key.pem` | Separate Simulator server identity for `simulator.<domain>` |
| `ldap-chain.pem`, `ldap-key.pem` | Separate LDAP server identity for `ldap.<domain>` |
| `oidc-chain.pem`, `oidc-key.pem` | Separate OIDC server identity for `oidc.<domain>` |
| `ldap-admin.secret`, `ldap-reader.secret` | Independent 256-bit random administrative and read-only bind password material; no directory account is created yet |
| `oidc-room-client.secret`, `oidc-simulator-client.secret` | Independent 256-bit random client-secret material; no OIDC client is registered yet |
| `lab-bootstrap.json` | Version-1 `simulator-lab-trust` inventory with relative file references, hostnames and timestamps, not embedded keys/passwords |

The directory is created with mode `0700`, all files with `0600` (the invoking
umask may restrict them further). Files are exclusively created and synced.
The inventory is written last. Leaves are valid for 90 days, have exact DNS
SANs (no wildcard/IP SANs), and support server authentication only. Each leaf
has its own key and its chain includes the public lab CA. Secrets are unpadded
base64url with one final newline; remove only that newline when configuring
providers. By default there are no seeded users or grants. The optional identity
renderer below adds initial directory users with unique generated passwords;
it does not install or activate them, or grant ZPR operator permissions.

Exit `0` means the files were written, not that services or an installation are
ready. Exit `1` is an explicit validation/generation/filesystem/output failure;
exit `2` is invalid command usage. An output-stream failure may occur after all
files are complete and is reported accordingly. Failures never delete files.
Existing directories (even empty ones), files and symlinks are refused.
On retry, inspect and preserve any existing output; there is no automatic
resume, rotation, overwrite or failed-install recovery in this initial tool.
Do not point an installer at incomplete output. The inventory is neither a
signed artifact manifest nor a complete installer integration handoff.

### Trust and operational limits

- Keep the CA private key protected/offline after issuance. Do not mount it
  into Simulator, LDAP, OIDC, DNS or the real ZPR services. Give each service
  only its own key/chain and required secret files, with explicitly configured
  UID/mount permissions.
- Distribute only the public CA to explicitly authorized lab browsers/endpoints
  and service trust stores. DNS constraints reduce the CA's naming scope; they
  do not make global/system-wide trust installation appropriate. Never disable
  TLS verification or use this CA for production or release signing.
- The generated Control Room identity can satisfy the real package's normal
  supplied TLS-file contract. No production company trust, internal ZPR mTLS,
  enrollment identities, token keys or release-signing keys are generated.
- Applying DNS records, addresses/routing, firewall rules, LDAP schema/users/groups,
  OIDC issuer/client/subject/grant setup and full installer inputs are still
  not part of an installer. Optional LDAP/Dex configuration generation is
  described below. A generated hostname does not prove DNS or HTTPS reachability.
- Expiry replacement/restart, backup/restore and resumable installation must be
  implemented and qualified before this becomes a supported companion package.
- Never publish generated private material in a distributable bundle, log,
  source checkout or scenario export. Lab reset/removal must remain separate
  from the real package's persistent identities and databases.

## Separate bootstrap and protected ZPR DNS

**Existing network DNS is the default**, in a company or lab. Bootstrap DNS
means name resolution available before ZPR admission, not a mandatory extra
server. Supply the companion and Room addresses to generate
`underlay-records.txt`: four fully qualified A records to give the existing
DNS administrator. It contains no SOA/NS/delegation or resolver settings and
does not replace an existing zone. Review collisions and ownership before
adding records; the tool never updates DNS itself.

The underlay address pair and protected ZPR address/prefix pair are independent.
This example generates the existing-DNS record request plus protected DNS files,
but **no bootstrap BIND server configuration or zone**:

```sh
go run ./cmd/zpr-lab-bootstrap \
  -domain lab.example.test \
  -output /absolute/protected/parent/new-lab \
  -bootstrap-ip 10.20.30.40 \
  -room-ip 10.20.30.41 \
  -zpr-dns-ip fd5a:5052:adda:1::53 \
  -zpr-client-prefix fd5a:5052:adda:1::/64
```

These are explicit example addresses, not defaults or assigned interfaces.
The bootstrap and Room addresses must be canonical private IPv4; they may be
the same host, with separate service ports/namespaces to qualify later.
Protected DNS must be canonical unscoped ULA IPv6, within the explicit canonical
ULA client network. Partial, public, wildcard, mapped or mismatched inputs fail
before output is created (each selected pair must be complete). Omit both
underlay options to generate protected DNS files only, or omit both ZPR options
to generate just the external DNS record request along with lab trust/secrets.
Omitting all DNS options preserves trust/secrets-only generation.
IPv6 bootstrap and routed-client ACL configuration
are not implemented in this slice.

For an isolated lab without existing DNS, explicitly add `-host-bootstrap-dns`
to generate the optional companion BIND server files. This requires both
underlay addresses but not the protected DNS pair. Merely supplying addresses
does not enable that server. `-host-bootstrap-dns=false` also leaves it disabled.
There is no automatic fallback, DNS discovery, mDNS daemon or `.local` support.
HTTPS/OIDC continue using stable DNS hostnames and verified TLS.

| Generated file | Role |
| --- | --- |
| `underlay-records.txt` | With the underlay address pair: fully qualified `room`, `simulator`, `ldap`, `oidc` A records for existing network DNS |
| `bootstrap-named.conf`, `bootstrap.zone` | Only with `-host-bootstrap-dns`: authoritative-only bootstrap DNS, bound to the bootstrap IPv4 only; `room.<domain>` resolves to Room IPv4, and `bootstrap`, `ldap`, `oidc`, `simulator` resolve to companion IPv4 |
| `zpr-named.conf`, `db.svc.zpr` | Authoritative-only `svc.zpr`, bound only to the DNS adapter's ZPR IPv6; initial `dns.svc.zpr` AAAA matches that address |
| `zpr-vs-publisher.key` | Unique 256-bit HMAC-SHA256 TSIG key for A/AAAA publication within `svc.zpr` |
| `zpr-dns-viewer.key` | Independent 256-bit transfer-only TSIG identity; no update grant |
| `zpr-dns-update.toml` | Visa Service DNS update fragment referencing the ZPR address and publisher key |
| `zpr-dns.zpl` | Least-privilege initial policy fragment: the `zpr-dns` service and `vs.zpr` publisher, TCP/53 only |
| `dns-files.json` | Relative file inventory for only the selected outputs; no embedded TSIG secrets |

All remain protected `0600` files under the same new-directory/no-overwrite
contract. `lab-bootstrap.json` includes the explicit DNS inputs and hosted
bootstrap selection when supplied. Protected files/TSIG keys are generated
only with the ZPR address/prefix pair, independently of hosted bootstrap DNS.
The completion inventory is still written after every generated file.

Deployment wiring still required:

- Normally add the requested records to the existing network DNS; no additional
  bootstrap listener, container, zone, resolver change or port 53 binding is
  needed. Verify resolution from the operator browser and service hosts before
  installation; file generation does not check live resolution.
- Only when explicitly selected, run an independent bootstrap BIND instance
  in addition to the protected ZPR BIND instance. Mount bootstrap files under
  `/etc/zpr/lab/`; it needs neither TSIG keys nor a ZPR adapter. Its query ACL is
  limited to directly connected networks and localhost; it is not an open
  recursive resolver. Add explicitly reviewed routed-client ACL support before
  claiming VPN/off-subnet bootstrap support. Configure clients to use it for
  this lab suffix without replacing unrelated company DNS.
- Run protected BIND in the real DNS adapter's network namespace, with its
  configured ZPR IPv6 present. Do not publish protected port 53 on the host or
  attach an underlay listener. BIND ACLs are not a substitute for ZPR policy.
  No process-scanning/demo launcher dependency is introduced by these files.
- Mount only the protected config and publisher/viewer keys under
  `/etc/zpr/lab/`. Install the initial zone at `/var/lib/bind/db.svc.zpr` on a
  writable persistent volume owned by the selected BIND UID; its journal must
  persist. Do not overwrite that zone on restart. Private CA/service keys do
  not belong in either BIND container.
- Give Visa its own protected copy of the publisher key at
  `/etc/zpr/dns-publisher.key`, the fragment's explicit target path. Merge the
  fragment into its normal config; this does not merge or activate it itself.
  The viewer key belongs only to the configured read-only transfer client.
- Compile/activate the fragment with the reviewed network policy and separately
  grant the intended query identities. The generator intentionally does not
  authorize every authenticated device. Without a client grant, normal client
  DNS is denied. Denial/allow evidence must use the actual ZPR path.
- Match the ZPL address, adapter address, BIND listener and Visa update target.
  Keep clients' TCP-only protected DNS configuration separate from underlay
  bootstrap DNS, with no underlay fallback for protected queries.

This slice does not generate reverse zones, recursive forwarding, DNSSEC
signing, statistics listeners, endpoint resolver changes, DNS adapter
credentials or container/namespace setup. `svc.zpr` serves protected service
records, not the TLS bootstrap hostnames. Ports, mounts/UIDs, images and startup
health still require installer integration and fresh-host qualification.

## LDAP-backed OIDC identity configuration

Opt in to generating OpenLDAP and Dex configuration. Supply exact HTTPS
origins matching the generated `room`, `simulator` and `oidc` hostnames:

```sh
go run ./cmd/zpr-lab-bootstrap \
  -domain lab.example.test \
  -output /absolute/protected/parent/new-lab \
  -identity-services \
  -room-origin https://room.lab.example.test:8787 \
  -simulator-origin https://simulator.lab.example.test:8788 \
  -oidc-origin https://oidc.lab.example.test:5556
```

Combine this with any of the DNS selections above; neither hosted bootstrap
DNS nor protected DNS generation is required for identity rendering. Origins
must have no path, credentials, query or fragment; their exact ports determine
callback URLs and Dex's HTTPS listener. Missing/mismatched origins fail before
output creation. Existing directories are still never overwritten.

Additional generated files:

| Files | Purpose |
| --- | --- |
| `slapd.conf` | OpenLDAP MDB configuration, fixed independent lab base `dc=lab,dc=zpr`, TLS 1.2 minimum, authenticated access, TLS-protected simple binds and password-hash ACLs |
| `directory.ldif` | Initial People/Groups/Services branches, read-only `zpr-reader`, `lab-operator`/`lab-observer` users and their distinct groups |
| `lab-operator.secret`, `lab-observer.secret` | Independent 256-bit random initial login passwords; no reusable demo passwords |
| `initialize-directory.sh` | Explicit one-time initialization of an existing empty `/var/lib/ldap`; refuses all existing/partial state, validates config, loads LDIF and writes completion marker last |
| `serve-directory.sh` | Startup validation and LDAPS-only foreground daemon on container port 1636; requires completed state, never imports seeds or resets databases |
| `dex.yaml` | LDAP-backed Dex, verified LDAPS, persistent SQLite, HTTPS only, separate confidential Room/Simulator clients and exact callbacks |
| `identity-inputs.json` | Versioned relative-file integration inventory: issuer, client IDs, callback URLs, CA/secret references and LDAP search/bind inputs; not a full company runtime configuration |

`dex.yaml` **contains the LDAP reader password and OIDC client secrets** and
must be treated as secret material. All generated files retain mode `0600`.
The LDIF and LDAP configuration contain salted SSHA password verifiers, not
plaintext passwords. SSHA here is the portable OpenLDAP format for generated
256-bit random values, not an approved format for low-entropy human passwords.
Do not publish configs, LDIF, secret files or databases in an image or archive.

Deployment contract to wire into the eventual installer:

- Use qualified Debian/OpenLDAP layout (`/etc/ldap/schema`, `/usr/lib/ldap`
  with `back_mdb`). Mount LDAP's own key/chain, public CA, configuration and
  scripts under `/etc/zpr/lab`, and a private persistent writable state volume
  at `/var/lib/ldap`. Do not mount the lab CA private key or other services' keys.
- Invoke `/bin/sh /etc/zpr/lab/initialize-directory.sh` once for a new empty
  state volume, then supervise `/bin/sh /etc/zpr/lab/serve-directory.sh`.
  Scripts are protected non-executable files, intentionally invoked via `sh`.
  Initializer failure retains partial data for inspection; do not delete,
  overwrite or retry initialization on a nonempty directory.
- Invoke `dex serve /etc/zpr/lab/dex.yaml` with only Dex's own key/chain,
  public CA and secret configuration mounted. Persist `/var/lib/dex` separately
  from LDAP state and Simulator. Do not reset these stores on scenario resets.
  Recreating LDAP identities changes their `entryUUID`, and therefore OIDC
  subjects; preserving LDAP state is part of preserving named operator access.
- Restrict published LDAP/Dex ports to the intended private lab interfaces;
  wildcard listeners in the generated configs are **container namespace**
  listeners, not permission to expose host ports publicly. Exact networking,
  runtime UIDs and protected mount ownership must be supplied. The opt-in
  Compose renderer below implements this contract with explicitly pinned images;
  qualifying release images remains unfinished.
  DNS must resolve `ldap.<domain>` from Dex and `oidc.<domain>` from services
  and operator browsers. Use the public lab CA for verified TLS; never skip it.
- Dex uses LDAP `entryUUID` for stable identities and DN/member group matching.
  The reader can search People/Groups but cannot read password verifiers or
  modify directory entries. Do not give Dex the LDAP root/admin password.
- Obtain and verify the actual issuer and `sub` from live OIDC before creating
  explicit company-scoped operator grants. The inventory deliberately has no
  invented subject, wildcard permissions or automatic group-to-ZPR grants.
  The user named `lab-operator` has **no ZPR administrative rights by default**.
  First-admin grant setup/recovery is still an installer prerequisite.

This is lab login/directory configuration, not an enrollment identity store.
Trusted attribute-service mappings, device records, policy attributes/change
feeds and the full real-package configuration handoff remain unfinished.
No Simulator API, profile, manifest or runtime state is needed by LDAP or Dex.

### Same-host independent identity stack

The approved initial topology is **one Ubuntu server, separate stacks and
persistent state** for the real package, companion identity providers and
Simulator. Identity services must survive stopping or resetting Simulator.
The generator can now produce `identity-compose.yaml`, `check-directory.sh`,
and independent mode-`0700` `ldap-state`/`dex-state` directories. Their relative
references are also recorded in `identity-inputs.json`.

Run generation **on the target host as the intended service-state owner**.
Compose captures that invoking numeric UID/GID; copying output between machines
does not remap ownership. Files remain private, not world-readable workarounds.
Select and load compatible immutable images into that host's Docker engine
first. LDAP requires the Debian layout above plus `slaptest`, `slapadd`,
`slapd`, `ldapsearch`, `/bin/sh`, `mktemp` and normal shell utilities; Dex must
supply its `dex` executable. Image tags and substitutions are refused. Compose
uses `pull_policy: never`; no download, signature verification or image
qualification is provided by this generator.

Engineering example, from the dashboard source directory, after setting
`LDAP_IMAGE` and `DEX_IMAGE` to explicit local `sha256:` image IDs or
`repository@sha256:` digests:

```sh
go run ./cmd/zpr-lab-bootstrap \
  -domain lab.example.test \
  -output /absolute/protected/parent/new-lab \
  -identity-services -identity-compose \
  -bind-ip 10.20.30.40 \
  -ldap-image "$LDAP_IMAGE" -dex-image "$DEX_IMAGE" \
  -room-origin https://room.lab.example.test:8787 \
  -simulator-origin https://simulator.lab.example.test:8788 \
  -oidc-origin https://oidc.lab.example.test:5556 \
  -bootstrap-ip 10.20.30.40 -room-ip 10.20.30.40
```

This generates files only. The default platform is `linux/amd64`;
`-identity-platform linux/arm64` is for engineering validation only. The bind
address must be an assigned private IPv4 on the host, never wildcard/loopback.
LDAP publishes TCP 1636; OIDC publishes its exact issuer port. Room, Simulator,
LDAP and OIDC require distinct same-host ports, including implicit HTTPS 443.
When generating DNS records too, the companion record address must match the
identity bind address. Apply the records through existing network DNS and
authorize firewall access explicitly; there is no host network/DNS change.
Dex resolves LDAP through its own Compose network alias. Browsers and the real
package still need reachable issuer DNS and explicit public-CA trust.

With Docker Engine and Compose v2 available on the target:

```sh
cd /absolute/protected/parent/new-lab
docker compose -f identity-compose.yaml --profile initialize config --quiet
docker compose -f identity-compose.yaml --profile initialize \
  run --rm --no-deps ldap-initialize
docker compose -f identity-compose.yaml up -d --wait --wait-timeout 90 ldap oidc
docker compose -f identity-compose.yaml ps
```

Run initialization **once on empty LDAP state**, not at every startup.
Initializer failure retains partial output for inspection. Normal startup
never seeds or resets LDAP. A per-install project name prevents accidental
state/network sharing; do not override it with a shared `-p` or copy an
installation for another lab.

LDAP health requires a successful read-only bind and base search over verified
LDAPS, not merely an open TCP port. Its password is newline-stripped into a
private temporary file, never process arguments or environment, and removed
on exit. Dex starts only after LDAP is healthy. Compose startup is **not** proof
of OIDC readiness or real Room authorization: separately verify issuer discovery
and actual browser login with the configured CA, then configure verified
issuer/subject grants through the real package's independent contract.

For an ordinary stop and restart:

```sh
docker compose -f identity-compose.yaml down --timeout 10
docker compose -f identity-compose.yaml up -d --wait --wait-timeout 90 ldap oidc
```

Both bind-mounted state directories survive `down`; do not rerun initialization.
Do not remove state or use a Simulator scenario reset to reset identity providers.
There is no destructive reset/uninstall or backup/restore workflow in this slice.
Services use individual read-only config mounts, read-only root filesystems,
dropped capabilities and no Docker socket. No real-package state, Simulator
state, lab CA private key or unrelated service keys are mounted.

## Verification

```sh
go test -race ./cmd/zpr-lab-bootstrap
go vet ./cmd/zpr-lab-bootstrap
```

Tests verify exact output inventory, file protection, unique keys/secrets,
key matching, hostname/chain/expiry/EKU checks, lab CA DNS constraints, secret
exclusion, invalid inputs, filesystem guards, no overwrite and CLI failures.
Generated leaves also pass the real package's supplied-certificate validator.
Focused race tests, vet and Linux amd64 build pass. Required `make test` retains
the four previously recorded dashboard failure groups (enrollment readiness,
policy-install platform variable, scenario launcher fixture and trusted-service
ELF artifact); this is not a green full-suite or release qualification result.
They do not certify production images or a fresh Ubuntu installation.

DNS tests parse exact zone records and Visa TOML, verify listener/ACL separation,
least-privilege policy, TSIG entropy/separation, file references, CLI output and
invalid/partial inputs. Independent underlay-only, protected-only and combined
modes are tested, with hosted bootstrap absent by default and present only on
explicit selection. Optional native `named-checkconf -z`/`named-checkzone`
tests run when the tools are installed. An opt-in container validation test
accepts only an explicitly selected local immutable BIND image ID:

```sh
ZPR_LAB_BIND_VALIDATION_IMAGE=sha256:<local-image-id> \
  go test -race -v ./cmd/zpr-lab-bootstrap -run TestLabDNS
```

The test uses no network, capabilities or daemon startup, a read-only container
and only generated DNS fixtures, then removes the container. With the existing
engineering arm64 image, BIND `9.20.29-1~deb13u1-Debian` successfully loaded both
generated configurations/zones. This validates BIND syntax/load, not running
queries, policy compilation, the actual ZPR path, or Ubuntu amd64 images.
Host-native tools were unavailable; that optional test is explicitly skipped.
Focused race tests (including isolated container BIND validation), vet,
formatting and Linux amd64 bootstrap build pass. Required full `make test`
still has the same four recorded dashboard failures; no running ZPR service
was changed.

Identity tests validate exact callbacks, matching per-install secrets/password
verifiers, stable identity/group mapping configuration, explicit no-grant
handoff, invalid-origin rejection and protected no-overwrite output.
An opt-in integration test uses explicitly selected immutable **local** LDAP/Dex
image IDs, never pulls images, and creates/removes its own containers and network:

```sh
ZPR_LAB_LDAP_VALIDATION_IMAGE=sha256:<local-ldap-image-id> \
ZPR_LAB_DEX_VALIDATION_IMAGE=sha256:<local-dex-image-id> \
ZPR_LAB_COMPOSE_VALIDATION_PLATFORM=linux/arm64 \
  go test -race -v ./cmd/zpr-lab-bootstrap -run TestLabIdentity
```

Select `linux/amd64` instead when using amd64 images. The generated Compose
test validates the original private-IP definition using Compose's actual
configuration parser, then replaces only published host IP/ports with random
loopback ports for isolated runtime testing. It verifies one-time initialization,
authenticated LDAPS health, live signed PKCE/OIDC login, and stable subjects
after `down`/`up` recreates both containers with preserved state. This passed with
local arm64 engineering images, without Simulator or any real-package service.
It does not certify deployment on the selected private address, Ubuntu amd64
images, or real Control Room authorization.

Only random loopback host ports are published. Containers drop capabilities,
use read-only filesystems and the invoking UID, and receive only their own
generated configuration/trust files. The LDAP initializer runs without network.
Test state is private and temporary; existing stacks are not used or changed.
The integration test verified the generated initialization/startup scripts,
LDAPS trust, denied reader mutation/password-hash reads, wrong-password
rejection, and live Dex authorization-code login with PKCE for both lab users.
It verified signed ID-token issuer/audience/nonce/subjects/groups and stable
subjects after restarting both providers with preserved state, without Simulator.
The existing engineering arm64 LDAP image and Dex v2.44.0 were used; this is
not qualification of a distributable Ubuntu amd64 provider image, real Control
Room login/authorization, endpoint admission or ZPR packet transport.
The combined focused race suite (including all three real provider validators),
vet, formatting and Linux amd64 bootstrap build pass. Required full `make test`
retains the same four recorded dashboard failure groups listed above.
