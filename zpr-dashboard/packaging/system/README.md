# System packaging

## B1 local evaluation subset

The [one-command local evaluation bootstrap](../evaluation/README.md) separately
starts current production-tagged Control Room/Control-Service plus unique
OpenLDAP/OpenObserve/Dex state, verified TLS and protocol readiness. It does not
start/require Simulator, register sources, ingest logs or implement the complete
first-install BOM. Its mixed-platform package set is evaluation-only, not a
qualified Ubuntu/amd64 appliance or a replacement for the contracts below.
It is a component subset, not a working ZPR network under the
[product-wide minimum](../../../System%20release%20plan.md#common-minimum-for-a-working-zpr-network):
it does not by itself prove an admitted node, active Visa policy, protected DNS
resolution, or end-to-end durable logging.

## Current objective: paired dataplane demo packages

The immediate goal is a runnable ZPR dataplane demo archive and a separate,
version-matched Simulator companion for an isolated disposable Ubuntu 26.04.1
LTS x86-64 host. The
[demo package contract](DEMO-PACKAGE-CONTRACT.md) defines the allowed demo
shortcuts, contents, downloaded-artifact acceptance and the remaining packaging
blockers. In particular, Simulator's Docker-socket access is root-equivalent;
the user approved it only for isolated disposable demo hosts. Nothing in the
demo contract permits publishing or production use. This pair is not a
baseline-complete ZPR network and must not be described as one.
For package-mode Simulator startup, the
[multinode demo wrapper](../../../../zpr-demo/multinode-demo/packaging/run-simulator-package-mode.sh)
requires an explicit dashboard support tree and locally loaded image reference,
then forces `ZPR_SIMULATOR_PREBUILT_IMAGE=1`. The image helper verifies
`linux/amd64`; default development mode still builds from source. This removes
only image-build-on-start. The dashboard continues to require bind-mounted
support trees, so the Simulator companion is not yet source-independent.

## Production packaging remains separate

This document also records production packaging work under the
[system release plan](../../../System%20release%20plan.md). Host preflight,
configuration validation, bundle-manifest validation and runtime rendering are
engineering slices, not a production installer. There is no installable
production server bundle, production Compose definition or signed system
release. The [first-install runtime contract](INSTALLER-RUNTIME-CONTRACT.md)
records production requirements; it does not make a demo or production bundle
installable. Nothing here starts or changes the development runtime.

## Linux/amd64 validator CLI engineering staging

The `zpr-system-config` validator is built as a standalone Linux/amd64 binary;
installation and use of the binary do not require Go or a source checkout.
From the `zpr-dashboard` module directory, select an absolute output file path
outside this repository whose parent exists and whose file does not:

```sh
sh packaging/system/build-cli-linux-amd64.sh /tmp/zpr-system-config-linux-amd64
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

Build the standalone manifest validator, archive packer, downloaded-archive
verifier, detached-signature tool and Control Room runtime-input renderer
together for an Ubuntu 24.04 x86-64 qualification host:

```sh
sh packaging/system/build-bundle-tools-linux-amd64.sh \
  /tmp/zpr-bundle-tools-linux-amd64.tar.gz
```

The resulting engineering-tool archive contains five Linux/amd64 binaries,
`SHA256SUMS` and `BUILD-INFO.txt`. Verify its contents and tool checksums before
use. This is a convenience archive for qualification workflows, not a signed
ZPR installation or Simulator companion package; it does not yet ship as a
customer download.

Run the isolated staging regression script with:

```sh
sh packaging/system/test-cli-linux-amd64.sh
sh packaging/system/test-bundle-tools-linux-amd64.sh
```

The scripts build only into temporary directories that are removed on exit,
exercise success and failure/no-write cases, and verify target metadata without
executing the Linux binaries. Required module dependencies must already be
cached; the checks do not download modules or toolchains.

## Approved target

- Dedicated Ubuntu 24.04 LTS x86-64 server/VM.
- Container appliance using Docker Engine and Docker Compose v2.
- Signed installer with pinned release images; not source builds at install time.
- Remote Control Room HTTPS on a company hostname, not a mandatory SSH tunnel.
  Direct TLS on an explicitly selected private management IP, corporate/VPN
  network only; no reverse proxy or public-Internet topology. Runtime opt-in is
  implemented; production container-network/installer integration is not.
- Installer choice of existing company providers or bundled production services
  for operator OIDC login, device/actor authentication and trusted attributes.
  This 2026-10-17 approval supersedes the external-provider-only restriction.
  Bundled providers require unique installation identities, explicit initial
  administrator setup, least-privilege grants and recovery; no demo identities.
- Installer choice of existing logging integration or bundled collection,
  persistent storage and authorized querying. Define retention, disk bounds,
  sensitive-data handling, backup and explicit failure reporting. Container
  stdout alone is not the bundled logging deliverable.
- Bundled providers/logging must be independently supervised, persist across
  restart/upgrade and operate with Simulator absent. No automatic fallback from
  failed external providers. Implementations/versions remain to qualify; BAS
  is not selected or required by this decision.
- Company-provided underlay/bootstrap DNS remains an input.
- Optional zpr.net assistance is for initial setup only, followed by an explicit
  authenticated handoff to installation-owned services. Keep private keys local;
  hosted setup grants must be scoped, expiring and replay-protected. Website
  accounts do not grant local admission or administrator rights. Daily operation,
  login, enrollment, renewal/revocation, logging and recovery must not require
  zpr.net. Verify these paths with zpr.net blocked after setup and setup grants
  expired. Future gateway/federation coordination is separate, undecided scope.
  See the [bootstrap boundary](../../../System%20release%20plan.md#optional-zprnet-bootstrap-assistance).
- ZPR-managed service DNS is a required part of the ZPR package: protected BIND9,
  its real ZPR adapter, Visa Service record publishing, configuration and
  persistent state. ZPR feeds the records today and will control policy-aware
  DNS behavior in future work. It is not bootstrap DNS or Simulator-owned
  infrastructure. Protocol/mapping/credential and adapter packaging integration
  remain open; per-client answer filtering is not yet implemented.
- A separate [Simulator companion package](../../../System%20release%20plan.md#simulator-companion-package)
  is also approved: Simulator plus lab LDAP, OIDC and DNS. It supplies ordinary
  external integration endpoints to this same production package, not a
  Simulator-dependent ZPR build. No companion installer/bundle exists yet.
  Its initial [local trust bootstrap](../simulator/README.md) generates unique
  protected lab TLS identities and service secrets only; no service is started.
- Next user-test deliverable is the
  [complete from-scratch installation package](../../../System%20release%20plan.md#next-operator-test-deliverable),
  including all required ZPR services, not a standalone Control Room image.
- Company-supplied HTTPS certificate/key files and company-managed renewal.
  No automatic ACME or acme-dns installation in this slice.
- Existing Mac endpoint pilot priority is unchanged. Its certification and
  credential-delivery work remain release prerequisites.

Remote operator access approval does not approve exposing service administration
or device enrollment to the public Internet. Device enrollment retains its
existing private corporate underlay/VPN boundary. Management network/firewall
rules/verification, directory integration, certificate reload/recovery integration, Docker
version qualification, resource sizing and release owners remain open.

## Planned multi-host expansion

The approved [multi-host deployment direction](../../../System%20release%20plan.md#multi-host-role-based-deployment)
keeps the single-server pilot as the first deliverable while designing for
separate management, routing-node, protected-DNS, supporting-service and endpoint
roles. A host may run multiple explicitly configured roles; support must be
qualified per role, OS, architecture and release. The current Ubuntu preflight
and partial management configuration do not implement this model.

The installation design must distinguish creating a network from joining one,
with locally generated private keys and short-lived, administrator-approved,
role-scoped join authorization. Define a versioned production host/service
inventory, secret references, authenticated remote endpoints, health states and
add/upgrade/drain/replace/revoke/remove operations. Preserve identities and
customer state; do not assume localhost or shared filesystems. Inventory
ownership/API design must respect existing sole-writer boundaries.

After single-server acceptance, qualify a second routing host, then separately
placed protected DNS, then DNS redundancy and selected mixed-platform/rolling
upgrade paths. DNS authority, update ownership, replication and failover need
explicit contracts. Multiple hosts alone are not high availability. All paths
must operate without Simulator or zpr.net; neither provides production inventory
or daily lifecycle services. Platform priorities and scale/availability targets
remain open, and no additional topology or platform is certified by this plan.

## Companion lab versus production installation

Production uses explicitly selected existing or bundled identity/attribute and
logging services, plus company-provided bootstrap DNS and TLS.
Protected ZPR service DNS ships with the real ZPR package in both company
and lab installations and must remain operational without Simulator.
For a lab, the separately installed Simulator companion can supply lab LDAP
and OIDC through explicit endpoint, identity/mapping and trust inputs to the
normal ZPR installer. Bootstrap names use existing network DNS; the companion
does not run a separate bootstrap BIND service. Protected DNS remains ZPR-owned.
Production bundled services are not owned by that companion.
This does not make Simulator a runtime dependency or let the
production package read simulation manifests, profiles or sessions.

Existing network DNS resolves bootstrap/login hostnames in company and lab
installations. The lab generator can export the required underlay records; it
does not generate or supervise a companion bootstrap DNS server. Protected ZPR
BIND configuration is independently selectable and its service lifecycle
belongs to the ZPR package. mDNS/`.local` support is not added.

Supporting services must run independently of the Simulator application.
Acceptance must demonstrate the same ZPR artifacts operating when Simulator is
unavailable but LDAP/OIDC/DNS are healthy, and explicit failures/recovery when a
required dependency is unavailable. Per-install lab secrets/trust, isolated
state, safe reset/removal, networking and compatible release IDs must be
documented and qualified. Lab TLS must be verified, not bypassed.

The current management-bootstrap example/validator below is still partial;
it does not yet implement this handoff or full directory/DNS configuration.
Paired lab acceptance does not replace independent fresh-company qualification.
Both packages together must demonstrate real node/adapter admission, an allowed
and an evidenced denied traffic flow, protected ZPR DNS, revocation and restart.
Running containers or displaying scenario data alone is not acceptance.

The companion now renders optional OpenLDAP/Dex configuration and a
[lab identity input inventory](../simulator/README.md#ldap-backed-oidc-identity-configuration).
Real LDAP-backed OIDC/PKCE and preserved-subject provider restart are verified
in isolated engineering containers. This inventory is not yet consumed by the
real installer; verified named-subject grants and trusted attribute mappings
remain explicit unfinished prerequisites, not implicit demo authorization.
The companion also generates an independent LDAP/Dex Compose stack for the
approved same-server, separate-state topology. See its
[startup and recovery instructions](../simulator/README.md#same-host-independent-identity-stack).
This does not add a dependency on Simulator or complete the production installer.

## Simulator navigation to the production Control Room

The deployment administrator may set `ZPR_SIMULATOR_CONTROL_ROOM_URL` in the
**Simulator process environment** to the approved browser-facing Control Room
origin, for example `https://control.company.example`. There is no default and
no discovery from simulation manifests, query parameters, organizations or
targets. This is a navigation-only setting, not a production service endpoint
or an addition to the management-bootstrap JSON schema.

The value must be an absolute HTTPS origin with a canonical lowercase DNS host
or IP address and an optional port from 1 to 65535. An optional trailing `/`
is accepted. Credentials, other paths, queries (including empty `?`), fragments,
whitespace and escaped URL components are rejected. For explicit local
development only, HTTP is also accepted for `localhost`, `127.0.0.1` and `[::1]`;
there is no automatic localhost fallback. Use the approved HTTPS origin for
company deployments.

Simulator exposes only the validated, non-secret destination through its
read-only, non-cacheable `GET /simulator-navigation.json` web configuration
route, under the existing Simulator web boundary. It does not contact the
destination or assert that Control Room is reachable. Missing configuration
returns 503; invalid configuration returns 422 without echoing the supplied
value. In either case navigation shows a configuration-needed state without
an actionable destination. Restart Simulator after changing its environment.

The **Control Room** navigation entry opens a separate tab with `noopener`,
`noreferrer` and no referrer. It leaves Simulator and its unsaved context in
the original tab and sends no token, source, organization or target in the
destination URL. Control Room retains its independent login, authorization,
CSRF checks and production-owned editors. Operators must verify the production
organization there. No cookie/token sharing, iframe, shared-editor embedding,
target handoff or reverse dependency is introduced: production services never
read this Simulator setting and must work with Simulator unavailable.

## Company management-bootstrap configuration

The version-2 [example](company.example.json) is a **partial installation input**
for company identity, operator access and Control Room's Control-Service mTLS
connection, not a complete server/node/adapter configuration.
Neither the current dashboard nor a Compose installer consumes
it yet. Do not pass it as `ZPR_OPERATOR_OIDC_CONFIG_FILE`: only its `oidc.config`
object has that existing format; rendering protected runtime files is future work.

Validate it from the dashboard source directory:

```sh
go run ./cmd/zpr-system-config -config packaging/system/company.example.json
```

This is an engineering invocation, not a requirement to install Go on customer
servers. The eventual signed release must ship the validator binary. The
command is read-only, performs no network access and does not invoke Docker.
Exit `0` means structural/reference validation only, `1` means unreadable/invalid
configuration or output failure, and `2` means invalid command usage.
Diagnostics never print configuration contents or source parser excerpts.
With `-check-files`, exit `0` additionally means the referenced local files pass
the offline checks below; it still does not mean the appliance is ready.

| Input | Contract (versions 1 and 2 unless noted) |
| --- | --- |
| `schema_version` | `1` preserves the original company/operator input and rejects `control_service`; `2` requires `control_service`. Other versions fail |
| `organization.id` | 1-64 lowercase ASCII letters/digits/hyphens, alphanumeric at both ends; no wildcard scopes |
| `organization.name` | Nonempty trimmed display name, at most 256 bytes, no control characters |
| `control_room.origin` | Exact `https://company-hostname[:port]`, lowercase dotted DNS hostname, no IP, localhost/`.local`, path, userinfo, query or fragment; optional canonical port 1-65535 |
| `control_room.management_ip` | Optional canonical private IPv4/IPv6 (RFC1918/ULA); the direct-management runtime requires an explicit value matching its listener. Omission leaves binding unselected, not wildcard/defaulted |
| `control_room.certificate_file`, `key_file` | Separate absolute canonical Linux file references; company supplies a current hostname-matching server certificate chain and its private key |
| `oidc.config` | Existing operator OIDC version-1 shape: HTTPS issuer, client ID, exact callback, explicit 60-3600 second session lifetime and issuer/subject grants |
| `oidc.config.redirect_url` | Exactly `control_room.origin` plus `/auth/operator/callback`; register this exact URI with the company provider |
| `oidc.config.grants` | Each grant names an exact issuer/subject and scopes only this company. Production monitor/policy/gateway/change-review and enrollment permissions are validated. `budget.approve` is accepted only when the complete optional `budget_authority` section is configured; no grants are generated |
| `oidc.client_secret_file` | Separate absolute canonical Linux file reference, never an inline client secret |
| `oidc.ca_file` | Optional separate CA file reference for company IdP trust; omission means the existing system-root behavior, not disabled TLS verification |
| `control_service.url` | Required in v2: exact HTTPS origin with a DNS host or IP and optional canonical port 1-65535; no credentials, path (including trailing `/`), query or fragment |
| `control_service.client_certificate_file` | Required in v2: distinct canonical absolute Linux file reference for Control Room's backend client certificate |
| `control_service.client_key_file` | Required in v2: distinct canonical absolute Linux file reference for that client's private key; never inline key material |
| `control_service.ca_file` | Required in v2: distinct canonical absolute Linux file reference for Control-Service CA trust input |
| `control_service.tls_server_name` | Optional in v2: host-only ASCII DNS name or unbracketed IPv4/IPv6 literal, without a port, IPv6 zone, URL components, wildcard or whitespace. DNS labels use letters/digits/hyphens, are 1-63 bytes and cannot start/end with a hyphen; the name is at most 253 bytes excluding an optional final dot. Single-label and mixed-case DNS names are accepted; international names must use ASCII A-labels. Omitted/empty retains the production proxy's URL-hostname fallback; otherwise supplies its TLS server-name override unchanged. Syntax only: this does not validate certificate SAN identity, CA trust or mTLS operation |

These v2 fields represent the existing
[`newControlServiceProxy`](../../cmd/zpr-web-dashboard/control_service.go)
inputs, in order: `ZPR_CONTROL_SERVICE_URL`, `ZPR_CONTROL_CLIENT_CERT_FILE`,
`ZPR_CONTROL_CLIENT_KEY_FILE`, `ZPR_CONTROL_SERVICE_CA_FILE` and optional
`ZPR_CONTROL_SERVICE_TLS_SERVER_NAME`. The validator itself does not render
environment or runtime files. The example contains only placeholder references;
it does not provision client credentials or establish service CA trust.

At least one named grant must include `monitor.read`, to avoid configuring no
operator capable of viewing the Control Room. This does not grant administrative
rights automatically. The example intentionally grants only monitoring, and its
subject is a placeholder that must be replaced with the provider's exact `sub`.
Email/display name never substitutes for an identity. Select additional grants
explicitly and configure independent backend enrollment delegation permissions;
this file does not bypass those controls or implement first-admin recovery.

### Control-Service budget authority inputs

The optional schema-v2 `budget_authority` object in
[`company.example.json`](company.example.json) is additive to the Control Room
connection above. It supplies independent launch inputs for the existing
`zpr-web-dashboard -mode control-service` and `-mode control-room` processes:

| Field | Rendered use |
| --- | --- |
| `database_file` | Persistent SQLite path for `ZPR_BUDGET_AUTHORITY_DB_PATH`. Its parent directory must already exist, be private, and be writable by the Control-Service identity. The existing store creates/opens the database with private permissions. |
| `delegation_trust_file` | Operator-supplied verifier trust JSON mounted read-only as `ZPR_OPERATOR_DELEGATION_TRUST_FILE`. It is validated by Control-Service at startup; this renderer checks only file metadata. |
| `service_listen_address` | Explicit private, non-wildcard IP and port for `ZPR_CONTROL_SERVICE_LISTEN`; its port must match `control_service.url`. This renderer does not choose interfaces, routes, firewall rules, or published ports. |
| `service_certificate_file`, `service_key_file`, `service_client_ca_file` | Control-Service mTLS server identity and client trust inputs, mounted read-only and rendered to their corresponding `ZPR_CONTROL_SERVICE_*` environment names. The private key must have mode `0600`-equivalent permissions. |
| `control_room_signer_file`, `control_room_signer_key_file` | Operator-supplied delegation signer config and private key, mounted read-only and rendered to `ZPR_OPERATOR_DELEGATION_SIGNER_FILE` and `ZPR_OPERATOR_DELEGATION_KEY_FILE`. The private key must have mode `0600`-equivalent permissions. |
| `control_room_signer_audience` | Must exactly equal `control_service.url`; rendered as `ZPR_OPERATOR_DELEGATION_AUDIENCE`. The signer JSON's own audience must independently match. |

When this section is present, the rendered Control-Service block includes
`ZPR_BUDGET_AUTHORITY_DB_PATH` and `ZPR_OPERATOR_DELEGATION_TRUST_FILE`;
its writable mount is the database parent directory so SQLite WAL files can
persist. The Control Room block includes the signer environment and the OIDC
configuration supplied in `oidc.config`. At least one exact-company operator
grant must explicitly contain both `monitor.read` and `budget.approve`.
Without the complete section, `budget.approve` is rejected by this company
configuration validator. This does not create or approve a grant, signer,
delegation trust, TLS identity, database, or approval record.
The budget-authority trust file does not configure device enrollment. If the
Control-Service enrollment registry and catalog are not configured separately,
enrollment endpoints remain unavailable with HTTP 503; the budget authority can
still start and use its own delegation trust.

Offline `-check-files` checks only metadata for these additional references:
regular non-symlink file types, no group/other write bits, and no group/other
permissions on private keys; the database parent must already be a private,
non-symlink directory. It does not read or parse these referenced files,
compare certificate/key pairs, validate trust contents, establish service
reachability, or create the database. Runtime service checks remain mandatory.
The emitted Control-Service and Control Room settings are independent role
blocks for an installer or operator to apply; this renderer is not a launcher,
Compose definition, or production installer.

The reader rejects unknown/miscased fields, duplicate object keys at every
level, nulls, trailing documents, oversized input (64 KiB) and nesting above
16 levels. All TLS/OIDC/Control-Service file references must be mutually distinct,
absolute Linux paths, with no `..`, redundant separators, trailing slash or shell expansion.
They describe intended installation-host paths, not paths resolved against the
developer's machine, home directory, Simulator state or config-file directory.

Without `-check-files`, referenced files are not opened: a structurally valid
example can refer to nonexistent files.

### Offline credential-file checks

On the intended installation host, with the intended service identity's file
access, opt in to inspecting the local files:

```sh
zpr-system-config -config /path/to/company.json -check-files
```

During development, use `go run ./cmd/zpr-system-config` in place of the binary.
Paths are taken literally from the configuration; there is no alternate root,
secret lookup, company key generation, network access or configuration output.
File inspection is supported on Linux and macOS; the appliance target remains
Ubuntu 24.04 x86-64. Other platforms fail explicitly when this mode is selected.

- Reuses the runtime's protected operator-file reader. Every reference must
  resolve to a regular, single-link file owned by root or the invoking effective
  user, with no group/other write permissions. Final-component symlinks,
  hard-linked files, directories and special files are rejected. Files are
  opened without following the final symlink and checked against their initial
  metadata. Parent-directory ownership/symlinks and runtime container identity/
  mount access are **not** audited; an installer must establish those separately.
- Private key (including the Control-Service client key) and OIDC secret files
  additionally require no group/other access
  (typically `0600`). Public certificate/CA files may be `0644`. The reader does
  not repair permissions or ownership.
- Certificate, key and CA inputs are bounded to 64 KiB each, and the OIDC secret
  to 4096 bytes. A secret is a nonempty, unpadded single line, optionally ending
  in LF or CRLF, matching the current operator runtime contract.
- Control Room TLS input must be a leaf-first PEM certificate chain and one
  unencrypted PEM private key. Checks include key matching, company hostname/SAN, current
  validity, HTTPS server-authentication use, TLS 1.3 digital-signature use when
  key usage is specified, and supplied issuer signatures/path constraints.
  A lone leaf or a chain omitting its root is permitted; the supplied chain tip
  is an **inspection anchor only**, never proof that an operator trusts it.
- An optional OIDC CA file must contain only currently valid CA certificates,
  with certificate-signing use when key usage is specified. This does not prove
  that the provider's certificate chains to these anchors or that login works.
- In v2, the three Control-Service references receive protected-file access
  checks only, with a 64 KiB limit each and private-file permissions on the
  client key. Their contents are not parsed: client certificate/key format or
  matching, client authentication suitability, service CA format/trust and
  service reachability are **not established**. Passing these checks is not
  runtime mTLS validation.
- Errors name the failed configuration field without disclosing local paths,
  credential contents or raw TLS parser errors. Failure emits no success output.

**Still not checked:** Control-Service runtime mTLS, browser/company root trust, revocation, company IdP
discovery/token/login, DNS, management firewall, listener reachability, or service
readiness. This is a read-only snapshot, not an install/activation gate or a
guarantee against later file changes. No configuration is emitted or applied.
The command reports these limits even on success.

### Render Control Room runtime inputs

The separate `zpr-system-config-render` engineering command maps schema-v2
company inputs into the existing Control Room process environment and emits the
operator OIDC runtime configuration. It requires offline file checks to pass,
so the invoking installer identity must be able to read every referenced file.
The output is a new JSON file with mode `0600`; existing output paths are never
overwritten.

```sh
go build -o /tmp/zpr-system-config-render ./cmd/zpr-system-config-render
/tmp/zpr-system-config-render \
  -config /path/to/company.json \
  -output /path/to/new/runtime-inputs.json
```

The JSON includes the listener address, exact existing Control Room and
Control-Service environment names, a `0600` OIDC configuration at
`/etc/zpr/config/operator-oidc.json`, and read-only file-mount references for
the company certificate, key, secret and CA paths. The future container
composition must mount those inputs at the reported destinations and install
the generated OIDC JSON at its reported path. The renderer does not copy private
files, generate identities, validate Control-Service certificate/key matching
or remote CA trust, build Compose configuration, start services or apply
settings. It is a configuration-rendering slice, not an installer or complete
runtime configuration.

On the intended Linux host, after supplying the operator-owned configuration
and referenced files, run the structural/offline validation and render into a
new private output path:

```sh
go run ./cmd/zpr-system-config -config /etc/zpr/config/company.json -check-files
go run ./cmd/zpr-system-config-render \
  -config /etc/zpr/config/company.json \
  -output /etc/zpr/config/runtime-inputs.json
```

The renderer itself creates the output with mode `0600` and refuses to
overwrite. Read its JSON and have the independently managed service launcher
apply the Control Room and, when configured, Control-Service blocks with their
declared read-only file mounts and writable SQLite parent mount. Do not source
the JSON as shell text or place private values in environment variables.
`-check-files` is an offline file check, not a live TLS trust or service
readiness test.

The clean-directory process-level acceptance fixture can be reproduced from
the module root without Simulator, Docker, network access, or customer
credentials:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local \
  go test ./cmd/zpr-system-config-render \
  -run '^TestPackagedRoleConfigurationCLIIntegration$' -count=1
```

It creates a disposable synthetic test certificate, test-only OIDC grant and
metadata fixtures beneath the test temporary directory, builds and invokes the
actual renderer executable, checks role outputs/modes and Simulator
independence, and checks explicit refusal with no output for missing or partial
files/settings and unsafe key permissions. These are test fixtures, not
deployable credentials or grants.

### Render a non-starting operator-pair Compose plan

`zpr-system-compose-render` consumes the runtime-input JSON above and emits a
Docker Compose plan for only Control-Service and Control Room. Both services
use the caller-pinned operator image and the existing
`/usr/local/bin/zpr-web-dashboard` process contracts. Because the rendered
listeners are explicit host addresses, the plan uses `network_mode: host` for
both services, preserves those addresses and the Control-Service HTTPS URL,
and publishes no Compose ports. It does not include or depend on Simulator.

The renderer also writes the embedded operator OIDC runtime JSON as a new
mode-`0600` file at the caller-selected host path. Existing output files are
refused; the OIDC path is mounted read-only at the runtime-configured
container destination. For Control-Service, all runtime file mounts are
read-only except the configured parent directory of the persistent SQLite
authority database. Compose bind mounts use `create_host_path: false`, so
applying the plan will not silently create absent mount sources.

Build the plan renderer and render from the output of
`zpr-system-config-render`:

```sh
go build -trimpath -buildvcs=false \
  -o /tmp/zpr-system-compose-render ./cmd/zpr-system-compose-render
/tmp/zpr-system-compose-render \
  -runtime-inputs /path/to/runtime-inputs.json \
  -image registry.example.com/zpr/operator@sha256:<64-lowercase-hex-digest> \
  -compose-output /new/private/path/operator-pair.compose.yaml \
  -oidc-output /new/private/path/operator-oidc.json
```

The `-image` argument accepts either a repository reference pinned to a full
lowercase SHA-256 digest or a local Docker image ID in the exact
`sha256:<64-lowercase-hex>` form. Tags alone are refused. For a local image ID,
the generated services retain that exact ID as `image` and set
`pull_policy: never`; the renderer does not convert it to a tag or pull it.
After loading an offline archive on the target, verify the installed
`runtime/image-reference` value resolves to the manifest's
`component.docker_image_id` (for example, compare
`docker image inspect <runtime-image-reference> --format '{{.Id}}'` with that
manifest field), then pass the verified `sha256:...` value to `-image`. This
local-ID option is not a RepoDigest and does not qualify an otherwise
demo-unqualified image for production use.

The output plan is JSON-formatted Compose data (valid YAML syntax), and the
renderer never invokes Docker or Compose, starts services, inspects the host,
opens ports, or changes host or network state.

Before applying a generated plan, the operator must independently verify that
the exact configured Control Room management IP and Control-Service private IP
are assigned on the target and that both configured listener ports are free.
The renderer cannot observe, infer, or change those conditions. Host networking
shares the host network namespace; this plan does not establish firewall rules,
service isolation, reachability, TLS readiness, or production installer/release
readiness. Do not change the configured listener addresses or replace them
with wildcard binds to make a plan apply.

### Separate Simulator budget-client inputs

Simulator's HTTPS/mTLS resolver uses a separate schema and renderer; neither
the company configuration nor the production Control Room/Control-Service
renderer reads Simulator files or requires Simulator availability. The
operator supplies the authority HTTPS origin, client certificate and key,
CA trust file, and optional TLS server name in
[`simulator-budget-client.example.json`](simulator-budget-client.example.json).
The certificate and key must be provisioned for the Simulator client and
trusted by the authority's Control-Service client CA. No credential is
generated, copied, or embedded in rendered output.

From the dashboard module directory, build and run the standalone renderer
from source:

```sh
go build -o /tmp/zpr-simulator-budget-config-render ./cmd/zpr-simulator-budget-config-render
/tmp/zpr-simulator-budget-config-render \
  -config packaging/system/simulator-budget-client.example.json \
  -output /path/to/new/simulator-budget-runtime-inputs.json
```

The input must reference existing files; `-output` must not already exist.
Offline checks inspect file type and permissions only and do not read
certificate/key/CA contents or contact the authority. Output is created with
mode `0600` and contains only resolver environment names/path references and
read-only file-mount descriptors. Apply that output only to the separately
launched Simulator process. This standalone engineering command is not yet in
the customer bundle, existing Simulator launcher, or production startup path.
No service is contacted or started, no grants are generated, and successful
rendering does not establish mTLS or authority availability.

### Remote HTTPS and identity implementation contract

The approved topology is direct HTTPS on a private management IP, corporate/VPN
only. The [runtime opt-in](../../README.md#direct-management-https) requires
`ZPR_CONTROL_ROOM_MANAGEMENT_IP`, an exact-IP `-listen` on the origin port,
company hostname TLS and initialized single-company OIDC. Existing loopback and
Simulator modes are unchanged. The handler checks actual TLS, exact Host and
Origin, rejects forwarded headers, and retains named authorization/CSRF for API
and DNS asset access. Enrollment delegation and change-review remain separate.
The validator still does not render or activate this runtime configuration.

Before a deployment can consume it:

1. Establish and verify corporate/VPN routing/firewall rules and the IP within
   the process/container network namespace. Runtime binding is not a firewall;
   it accepts direct unicast peers without treating peer IP as identity. Do not
   expose backend administration listeners, use public forwarding, or introduce
   wildcard listeners to work around unimplemented container-network integration.
2. Qualify the implemented direct-TLS/OIDC boundary on actual Ubuntu and with
   the company IdP/browser. Local synthetic login and boundary tests are not
   corporate-network or clean-install acceptance.
3. Wire the offline protected-file checks into installation, then verify
   installed mount access and company trust. The opt-in validator is implemented;
   installation integration is not. Fail on absent, unsafe, mismatched, expired
   or invalid inputs. Never ship the sample identities or generate replacement
   company keys silently.
4. Define company certificate replacement/reload and expiry reporting. Company
   renewal responsibility does not imply a running process reloads new files.
   Test replacement, interruption, rollback and administrator recovery.
5. Verify company IdP discovery/login, exact grants and backend delegation on
   the installed appliance with Simulator unavailable. Keep the first-admin
   binding and recovery explicit and audited.

Earlier offline-file verification (2026-10-15): 158 selected configuration/file/CLI and existing
operator-login/delegation/change-review checks pass, including
malformed/ambiguous JSON, bounds, issuer/origin/callback mismatches, grant scope,
secret references, file protection, offline TLS checks and content-safe failures.
Focused configuration/CLI/shared-reader and existing operator-runtime race tests,
vet, native CLI smoke and Linux amd64 validator/dashboard cross-builds pass.
Required dashboard `make test` retains the four recorded failures:
enrollment readiness, policy install, scenario placement fixture and trusted
service build fixture. No full-suite, live IdP or clean-Ubuntu acceptance claimed.

Management HTTPS verification (2026-10-15): **238 selected checks pass**, covering
configuration/file/CLI, management startup/request/login, existing loopback/
Simulator, route authorization, delegation and change-review behavior. Real
hostname-verified TLS 1.3, OIDC state/nonce/PKCE, secure session cookies, CSRF and
scope denials, logout, DNS asset authorization and independent mTLS API forwarding
are exercised locally with Simulator unavailable. The TLS fixture maps only the
test company hostname to loopback; it does not bind a real management interface
or verify corporate firewall/VPN policy.

Focused race tests, vet, native validator smoke and Linux amd64 validator/dashboard
builds pass in the final shared worktree. An intervening unrelated gateway-refactor
compile failure resolved before final verification. Final shared `make test`
retains exactly the same four failures listed above. Separately, the scoped
changes passed focused race/vet/build verification on source HEAD
`c3a38366f06017764829a97cd44da95d55af514b`; its isolated full suite's 13 failure groups
exactly match a clean snapshot of that HEAD, including missing external seeds/
runtime inputs. No deployment or complete release acceptance is claimed.

## Read-only host baseline preflight

On the prospective server:

```sh
sh preflight.sh
```

Run on the host, not inside an application container. No root privileges,
installation, permission changes, downloads, container launches or network scans
are performed. It reads OS identity, checks host architecture, inspects the Docker
endpoint, queries local Engine metadata and reads the Compose version.
It rejects remote Docker endpoints before querying their engines.
It requires existing authorized Docker socket access, which can confer
root-equivalent authority; it never adds users to groups or relaxes permissions.
`DOCKER_CONTEXT` takes precedence over `DOCKER_HOST`, as in the Docker CLI.
Do not publish full environment dumps or credential-bearing Docker configuration.

Exit codes: `0` means these baseline checks passed; `1` is an unsupported host
or missing/unavailable prerequisite; `2` is invalid invocation. Passing is
**not** production readiness. No Docker Engine version range, CPU/RAM/disk
minimum, filesystem behavior, TUN/capability access, firewall, DNS/TLS, OIDC,
enrollment or application health is certified by this script.

Run isolated regressions without a Docker daemon:

```sh
sh test-preflight.sh
```

The tests substitute an OS fixture and stub uname/Docker on PATH. They cover
unsupported OS/architecture, missing dependencies, remote endpoints, context
precedence, unavailable engines, wrong server architecture and Compose versions.
This is not Ubuntu clean-host acceptance.

## M1 source audit: deployable roles and gaps

Role inclusion below reflects the user's 2026-10-17 first-install direction,
including Gateway as required. It is a source inventory, not a completed or
qualified bundle or approval of every provider implementation. Ports are
observed source defaults, not a customer firewall specification. No internal
listener should be published merely because its port is listed. Provisional
source and image candidates are tracked in the
[system release plan](../../../System%20release%20plan.md#first-install-bom-scope-and-provisional-candidates).

| Role | Current source/build input | Configuration, trust and data | Packaging/readiness gap |
| --- | --- | --- | --- |
| Visa Service | [`vs`](../../../vs/), Rust workspace; `cargo build --locked --release -p vs` | Explicit VS TOML, policy, API keys, TLS and retained identity; VSAPI default 5002, Admin HTTPS 8182; Redis-protocol state backend defaults to 6379 | Pin server image, state-backend distribution/persistence and private network; readiness must verify loaded policy, node admission and actual decisions |
| Node runtime | [`libnode2`](../../../../zpr-core/libnode2/), binary `lntest` | Node identity/key, VS address, ZPR address, advertised substrate address and reverse VSS reachability | Current executable is a CLI/TUI/test-oriented entry point; certify a headless supervised node entry point and its network/privilege requirements before shipping |
| Endpoint/service adapter | [`ph`](../../../../zpr-core/adapter/ph/), `cargo build --locked --release -p ph` | Per-device identity, authentication trust, TUN/network state; credentials must come from the supported provisioning flow | Platform packaging, privileged handoff, revocation and traffic acceptance remain required; do not ship static demo bootstrap keys |
| Policy-Service | [`policy_service.go`](../../cmd/zpr-web-dashboard/policy_service.go), dashboard Go binary in `policy-service` mode | mTLS, explicit organization and SQLite repository, compiler config/source/signing inputs; default 8789 | Package ZPLC/ZPT and independent fixtures/LDAP inputs; distinguish draft/staged policy from active runtime policy; readiness must exercise compile/evaluate |
| Control-Service | [`control_service.go`](../../cmd/zpr-web-dashboard/control_service.go), same Go binary in `control-service` mode | mTLS default 8790; VS Admin URL/CA/key; Policy-Service mTLS; gateway draft store and configured trusted-service integrations | No dedicated production image yet; eliminate dependence on demo launcher paths and unnecessary host Docker access |
| Control Room | Same Go binary in `control-room` mode; embedded browser assets | HTTPS default 8787, exact origin, OIDC config/secret/CA, grants/delegation and Control-Service client identity | [`operator_login.go`](../../cmd/zpr-web-dashboard/operator_login.go) preserves loopback mode and adds opt-in direct private management-IP/company-hostname access with required scoped OIDC; container-network/installer integration and live company acceptance remain open |
| Device Enrollment Service | [`zpr-enrollment-service`](../../cmd/zpr-enrollment-service/main.go), separate Go command | Explicit JSON, private device HTTPS and opt-in separate admin mTLS listener, owned SQLite registry and optional runtime-import signer | Opt-in Control-Service relay removes its registry dependency; BAS registry access and full lifecycle/credential/bootstrap contracts remain unfinished. No cross-service shared SQLite mount in the production package |
| Authentication | [`zpr-bas`](../../../../zpr-bas/), `cargo build --locked --release` | BAS TLS identity, protected actor records and shared BAS/VS token-verification secret | BAS 0.1.0 is an approved engineering source candidate only; standalone BAS registry access must be reconciled with Enrollment sole-writer ownership before selecting it as the production provider. OIDC human login is a separate trust path |
| Compiler/evaluator | [`zplc`](../../../../zpr-compiler/), `cargo build --locked --release`; `zpt` from Visa workspace | ZPLC configuration, per-company signing material, input schema and policy binary compatibility | Pin compiler/common-library versions and native runtime dependencies; no compiler source checkout on customer host |
| Operator identity/directory | Bundled production provider path or external company OIDC/directory | Issuer/subject grants, redirect origin, TLS, protected client/bind credentials and configured attribute mappings | User requires a bundled new-user path and documented external integrations. Dex v2.44.0 is only a provisional image candidate; OpenLDAP image/version, production mappings and first-admin recovery remain open. Lab provider state/config is not a production default |
| DNS | Existing network DNS for bootstrap names; protected [`ZPR BIND9`](../../../dns/bind9/) behind an adapter | Explicit bootstrap endpoints; protected DNS address/policy, publisher and separate viewer TSIG identities | Protected BIND9 is required in the first-install BOM. Package its adapter and independent supervised namespace; Visa publisher requires `nsupdate`. Bootstrap must not depend on ZPR DNS. Config generation exists, not service deployment; BIND image/package remains unpinned, with no Ubuntu amd64 runtime qualification |
| Gateway | Existing gateway runtime and [`rollout contract`](../../scripts/gateway-runtime-rollout.md) | Independent installed identity, allowlist, destination/forwarding configuration and adapter link | Required in the first-install BOM by user direction; editor activation/rollback, fresh-host packaging and CONNECT enforcement limits remain unresolved. Existing image overlays a baseline and is not a from-scratch gateway bundle |
| Logging/telemetry | Existing [`observability`](../../../observability/) profile or a configured external logging service | Persistent backend data, retention, operator endpoints and protected credentials | Logging is a required capability with bundled or existing-provider mode. OpenObserve is a provisional pinned-image candidate, but the profile remains optional, needs license/runtime/access-control qualification, and is not a generic external-provider contract |
| Simulator | Separate companion package | Simulation profiles, workloads and manifests; drives the installed ZPR services through supported production contracts | Excluded from the production appliance and its startup dependencies; companion must pair with the exact same ZPR release artifacts and use separate lab-provider state |

The dashboard build input is currently `go build -trimpath
./cmd/zpr-web-dashboard`; the Enrollment Service is a separate command build.
These identify source targets, not final release build recipes. The Rust source
workspaces have sibling/path dependencies and native dependencies; `--locked`
alone does not capture every sibling source revision or qualify runtime ABI.

### Existing packaging that must not become the installer by accident

- [`Dockerfile.simulator`](../../scripts/Dockerfile.simulator) builds the shared
  executable but defaults to Simulator, adds Docker tooling and uses mutable
  base tags. A production image needs a separate reviewed contract.
- [`multinode-demo/Dockerfile`](../../../../zpr-demo/multinode-demo/Dockerfile)
  copies prebuilt local binaries and includes demo tools. Its build helper uses
  Debian trixie and a moving Rust `stable`, while its runtime defaults to Ubuntu
  24.04. Qualify ABI/toolchain/runtime libraries; do not assume those builds are
  a reproducible Ubuntu appliance.
- [`dashboard-stack.sh`](../../scripts/dashboard-stack.sh) contains production
  process invocations mixed with simulation activation, source mounts,
  `host.docker.internal`, host Docker sockets and broad privileges. It is
  evidence for extraction, not a production entry point.
- [`Dockerfile.gateway-runtime`](../../scripts/Dockerfile.gateway-runtime)
  overlays an already-existing reviewed baseline image; it does not establish
  a from-scratch customer gateway image.
- Existing [enrollment packaging](../enrollment/) signs/verifies a specific
  endpoint setup format. Reuse trust principles, not its manifest as if it were
  a complete system manifest.

## Startup, persistence and trust contract to finish

Proposed order (service dependencies, not only container start order):

1. Validate host/company inputs, artifact trust, time/DNS and secret permissions.
2. Establish persistent company identity and internal service trust exactly once.
3. Start/verify the persistent state backend; compile/sign a reviewed initial
   policy offline. Bring up required authentication/trusted services through an
   explicitly designed bootstrap path; do not leave a circular dependency on
   already-authenticated adapters unresolved.
4. Start VS with its configured policy/trust, then node runtime and verify both
   directions of their connection.
5. Start Policy-Service and its compiler/evaluator, Control-Service and
   Control Room with independent production configuration and named-user login.
6. Start the independent enrollment interface under its selected trust boundary;
   enroll one endpoint and verify actual allow/deny decisions.

Persist VS/state-backend identity and data, node/service private identities,
company trust, policy records/configuration/signing keys, enrollment lifecycle
and import signer, gateway drafts, operator grants/review records and any bundled
directory/DNS data. Transient sessions are not a backup strategy. Cross-store
backup/restore and sole-writer ownership must be defined before Compose mounts
are finalized. Do not generate keys or erase data on ordinary restarts.

Operator web TLS, service mTLS, VS Admin authorization, policy signing, device
enrollment proofs and BAS token verification are separate trust relationships.
Image signatures are release trust, not authorization to operate a company.
Do not mount host Docker sockets or grant `--privileged` to management services
as a shortcut; determine each runtime's minimum capabilities and test them.

## Source observation, not a release lock

Observed 2026-10-15; these are repository HEADs, not a certified compatible set.
The Visa/dashboard worktree had unrelated uncommitted changes. A real release
must select clean reviewed revisions and record artifact digests.

| Repository | Observed HEAD |
| --- | --- |
| zpr-visaservice | `c3a38366f06017764829a97cd44da95d55af514b` |
| zpr-core | `561fa7351795a723b74b5d1e57761d870722c77c` |
| zpr-common | `2756e9a01d5552965439f94314a6b0550832cf8f` |
| zpr-vsapi | `140748f926e7675f6923568f5c0f1fef00e482aa` |
| zpr-compiler | `6f0de17fd9052567394ad306552bc435efc3ed3f` |
| zpr-bas | `9de4a25192a21b61a2f92639104abf5985e223a8` |
| zpr-utils | `7dfee1cb41a13c6cbf7f350eb436009a99c22c0b` |
| zpr-demo (audit evidence only) | `64f80ca6e54e1ab0ff1216bd25ec89653c9acf91` |

The seven product/source repositories above have Apache-2.0 license files.
This is not a transitive dependency/image/font/browser-asset license audit or
an SBOM. Compiler source declares a git-tagged common-library dependency;
Visa/core use a sibling common-library path. The lockfiles and protocol/policy
compatibility must be reconciled, not replaced by this local HEAD table.
Named component/release owners and exact distro image choices are still open.

## Offline bundle-manifest validation

The M2 engineering slice adds `cmd/zpr-system-bundle`, backed by
`internal/systembundle`. It is a separate, read-only validator, not an
installer. Invoke it from the dashboard source directory:

```sh
go run ./cmd/zpr-system-bundle \
  -manifest manifest.json \
  -bundle-dir /path/to/system-bundle

go run ./cmd/zpr-system-bundle \
  -manifest manifest.json \
  -bundle-dir /path/to/companion-bundle \
  -paired-system-manifest manifest.json \
  -paired-system-dir /path/to/system-bundle
```

Manifest paths are relative to their corresponding bundle directory. Schema
version 1 records `bundle_kind`, `release_id`, `qualification_status`, the
Ubuntu 24.04 x86-64 target, and components. Each component declares an ID,
scope (`zpr`, `production-support` or `companion`), install mode, provided
capabilities, artifact platform, local payload path and SHA-256; OCI images
also require a digest-qualified image reference. An OCI image payload is a
plain OCI image-layout tar archive containing one platform image; validation
checks the referenced manifest digest, index/platform agreement, config
platform, and every declared config/layer blob digest. It does not execute or
inspect image filesystems. Source-built binaries require a repository name and
full Git revision. Do not include credentials or private configuration in the
manifest.
Every service artifact must declare the Ubuntu host platform (`linux/amd64`).
The standalone `endpoint-adapter` artifact may declare its own `os/architecture`;
that explicit value is not evidence that the platform is supported or qualified.

System manifests must include the required ZPR service capabilities, including
Gateway and protected DNS, the bootstrap configuration, and an initial policy.
They must also include bundled-default operator identity,
actor-authentication, trusted-attribute, logging and readable LDAP bootstrap
capabilities. These bundled artifacts remain available when an installation
explicitly selects external providers instead. This is a package-completeness
gate: the manifest does not prove that LDAP is readable at runtime, the initial
policy is active, or any service is healthy. Those remain installer and
end-to-end acceptance gates. A production manifest rejects Simulator, lab-only
and bootstrap-DNS capabilities. Companion manifests require Simulator and lab
LDAP/OIDC, and may include bootstrap DNS only as a selectable lab service. The
manifest does not encode or enforce runtime provider-state separation; that
remains an installer and acceptance requirement. Pair validation requires
identical ZPR component sets, artifact identities and target platforms.

The validator hashes only declared local payload files, rejects traversal and
symlink payload paths, and accepts `engineering-unqualified` manifests only.
It does not verify signatures or manifest authenticity, inspect image
contents, contact services, install/start containers, alter the host, sign,
publish or qualify a release. It checks declared host-platform compatibility,
not whether a component platform is actually supported or qualified. A
successful result is not authorization to install. This source command is not
yet included in a release bundle or a qualified build target.

### Create an engineering bundle archive

`cmd/zpr-system-bundle-pack` creates a deterministic `.tar.gz` from the
manifest and exactly its declared payload files. It rechecks the payload
hashes while writing, normalizes archive metadata, refuses symlinks and
path traversal, and will not overwrite an existing output. The output path
must be outside the source bundle directory. The command prints the archive
SHA-256; record it with any separately reviewed release metadata.
The output's parent directory must already exist, and the target filename must
not exist.

```sh
go run ./cmd/zpr-system-bundle-pack \
  -manifest manifest.json \
  -bundle-dir /path/to/system-bundle \
  -output /path/to/output/zpr-system-candidate.tar.gz

go run ./cmd/zpr-system-bundle-pack \
  -manifest manifest.json \
  -bundle-dir /path/to/companion-bundle \
  -output /path/to/output/zpr-simulator-candidate.tar.gz \
  -paired-system-manifest manifest.json \
  -paired-system-dir /path/to/system-bundle
```

Companion packaging requires a valid exact pairing with the system bundle, but
does not embed the system archive into the companion archive. Only the listed
manifest and artifacts are packaged; extra files in the input directory are
excluded. The archive remains `engineering-unqualified`: it is unsigned and
does not establish manifest authenticity, image contents, installation,
runtime health or release qualification. No actual candidate service bundle
or installer is available yet. This source command is not itself included in
a qualified release.

### Verify a downloaded bundle archive

`cmd/zpr-system-bundle-verify` validates the archive directly without
extracting files. It requires `manifest.json` as the first regular tar entry,
then requires exactly the declared regular-file payloads, validates their
SHA-256 hashes, checks the gzip stream, and prints the archive SHA-256. A
companion archive must be paired with the matching system archive:

```sh
go run ./cmd/zpr-system-bundle-verify \
  -archive /path/to/zpr-system-candidate.tar.gz

go run ./cmd/zpr-system-bundle-verify \
  -archive /path/to/zpr-simulator-candidate.tar.gz \
  -paired-system-archive /path/to/zpr-system-candidate.tar.gz
```

This detects archive corruption and mismatched declared payloads. To create
an optional detached Ed25519 signature, use a separately provisioned PKCS#8
private key PEM protected with mode `0600` or stricter:

```sh
go run ./cmd/zpr-system-bundle-sign \
  -archive /path/to/zpr-system-candidate.tar.gz \
  -private-key /protected/offline/release-key.pem \
  -signature /path/to/output/zpr-system-candidate.sig.json
```

Companion signing additionally requires `-paired-system-archive` with the
matching system archive. The signing command refuses to overwrite an existing
signature file. The signature binds the exact archive digest, bundle kind,
release ID and Ed25519 public-key fingerprint. It does not qualify the
engineering bundle. Do not put the private signing key in the tool archive,
website, source checkout or test machine.

Verify a signature with a separately obtained trusted public key PEM (PKIX,
Ed25519). The trust key must come through an independent, authenticated channel;
downloading it beside the archive does not authenticate it:

```sh
go run ./cmd/zpr-system-bundle-verify \
  -archive /path/to/zpr-system-candidate.tar.gz \
  -signature /path/to/zpr-system-candidate.sig.json \
  -trusted-key /independently-provisioned/release-key.pub.pem
```

The signature is optional for explicitly engineering-unqualified candidates,
and the verifier reports when no publisher signature was checked. Production
release policy must require a signature and establish independent key custody,
rotation/revocation and trusted-key distribution before publication. Signing
does not prove image safety, installation, runtime health or release
qualification. Verification does not extract files or install services. The
command is currently a source-tree engineering tool, not part of a downloadable
installation bundle.

## Next implementation gates

Enrollment ownership slice (2026-10-15): the
[opt-in administration listener and relay](../../README.md#enrollment-owned-administration-api)
share one Enrollment process's Store across device/admin handlers, without a
Control-Service database mount. Independent named grants, original Room
signatures, relay certificate pins and durable replay checks remain enforced.
The existing invitation API is retained; the full lifecycle contract is not
implemented. The standalone BAS command still opens its configured registry.
Full enrollment/delegation race suites, focused relay/audience and management
regressions, vet and Linux amd64 Enrollment Service/dashboard builds pass.
Required shared `make test` retains the same four recorded dashboard failures.
No deployment or from-scratch package acceptance is claimed.

1. Qualify the implemented direct-management HTTPS mode; establish corporate/VPN
   firewall and container-network contracts, directory ownership and certificate
   replacement/recovery using the approved company OIDC and supplied TLS.
2. Integrate and qualify the opt-in Enrollment-owned administration API and
   Control-Service relay; remove remaining BAS registry sharing and complete
   lifecycle/bootstrap identity/authentication ordering, with no shared
   production database between service owners.
3. Extend the validated management-bootstrap configuration to service trust,
   storage and enrollment inputs after their ownership contracts are settled;
   integrate protected-file checks and add rendering without claiming runtime readiness.
4. Build dedicated digest-pinned production images and least-privilege Compose
   definitions from clean source inputs; test actual binaries and ABI.
5. Expand preflight to approved sizing, runtime versions, storage, ports and
   capabilities, then run fresh Ubuntu VM acceptance. Mac-host fixture tests do
   not certify Ubuntu installation.
