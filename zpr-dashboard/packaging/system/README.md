# Production system packaging foundation

Status: **In progress**, 2026-10-15. This is the first M1 inventory and the initial
M2 host-preflight/configuration slices of the [system release plan](../../../System%20release%20plan.md).
There is no installable server bundle, production Compose definition or signed
system release yet. Nothing here starts or changes the development runtime.

## Approved target

- Dedicated Ubuntu 24.04 LTS x86-64 server/VM.
- Container appliance using Docker Engine and Docker Compose v2.
- Signed installer with pinned release images; not source builds at install time.
- Remote Control Room HTTPS on a company hostname, not a mandatory SSH tunnel.
  Direct TLS on an explicitly selected private management IP, corporate/VPN
  network only; no reverse proxy or public-Internet topology. Runtime opt-in is
  implemented; production container-network/installer integration is not.
- Company-provided existing OIDC provider; no bundled identity provider.
- Company-provided existing directory and DNS; do not bundle demo LDAP or BIND
  as replacements for company infrastructure. A separate policy-controlled
  BIND9 service behind a real ZPR adapter is required for the combined-package
  demonstration; it is not bootstrap DNS. Protocol/mapping/credential and
  adapter packaging integration remain open.
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

## Companion lab versus production installation

Production continues to use company-provided directory, OIDC, DNS and TLS.
For a lab, the separately installed Simulator companion will supply LDAP, OIDC
and protected ZPR DNS plus explicit endpoint, identity/mapping and trust inputs to the normal
ZPR installer. This does not make Simulator a runtime dependency or let the
production package read simulation manifests, profiles or sessions.

Existing network DNS resolves bootstrap/login hostnames by default, also in
the lab. Companion-hosted bootstrap DNS is an explicit isolated-lab option,
not an installation prerequisite. The lab generator can export the required
underlay records without generating a bootstrap server; protected ZPR BIND
configuration is independently selectable. mDNS/`.local` support is not added.

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

## Company management-bootstrap configuration

The version-1 [example](company.example.json) is a **partial installation input**
for company identity and operator access, not a complete server/node/adapter
configuration. Neither the current dashboard nor a Compose installer consumes
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

| Input | Version-1 contract |
| --- | --- |
| `schema_version` | Exactly `1`; unknown versions fail |
| `organization.id` | 1-64 lowercase ASCII letters/digits/hyphens, alphanumeric at both ends; no wildcard scopes |
| `organization.name` | Nonempty trimmed display name, at most 256 bytes, no control characters |
| `control_room.origin` | Exact `https://company-hostname[:port]`, lowercase dotted DNS hostname, no IP, localhost/`.local`, path, userinfo, query or fragment; optional canonical port 1-65535 |
| `control_room.management_ip` | Optional canonical private IPv4/IPv6 (RFC1918/ULA); the direct-management runtime requires an explicit value matching its listener. Omission leaves binding unselected, not wildcard/defaulted |
| `control_room.certificate_file`, `key_file` | Separate absolute canonical Linux file references; company supplies a current hostname-matching server certificate chain and its private key |
| `oidc.config` | Existing operator OIDC version-1 shape: HTTPS issuer, client ID, exact callback, explicit 60-3600 second session lifetime and issuer/subject grants |
| `oidc.config.redirect_url` | Exactly `control_room.origin` plus `/auth/operator/callback`; register this exact URI with the company provider |
| `oidc.config.grants` | Each grant names an exact issuer/subject and scopes only this company. Production monitor/policy/gateway/change-review and enrollment permissions only, validated against existing operator permission names |
| `oidc.client_secret_file` | Separate absolute canonical Linux file reference, never an inline client secret |
| `oidc.ca_file` | Optional separate CA file reference for company IdP trust; omission means the existing system-root behavior, not disabled TLS verification |

At least one named grant must include `monitor.read`, to avoid configuring no
operator capable of viewing the Control Room. This does not grant administrative
rights automatically. The example intentionally grants only monitoring, and its
subject is a placeholder that must be replaced with the provider's exact `sub`.
Email/display name never substitutes for an identity. Select additional grants
explicitly and configure independent backend enrollment delegation permissions;
this file does not bypass those controls or implement first-admin recovery.

The reader rejects unknown/miscased fields, duplicate object keys at every
level, nulls, trailing documents, oversized input (64 KiB) and nesting above
16 levels. All TLS/OIDC file references must be distinct, absolute Linux paths,
with no `..`, redundant separators, trailing slash or shell expansion.
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
- Private key and OIDC secret files additionally require no group/other access
  (typically `0600`). Public certificate/CA files may be `0644`. The reader does
  not repair permissions or ownership.
- Certificate, key and CA inputs are bounded to 64 KiB each, and the OIDC secret
  to 4096 bytes. A secret is a nonempty, unpadded single line, optionally ending
  in LF or CRLF, matching the current operator runtime contract.
- TLS input must be a leaf-first PEM certificate chain and one unencrypted PEM
  private key. Checks include key matching, company hostname/SAN, current
  validity, HTTPS server-authentication use, TLS 1.3 digital-signature use when
  key usage is specified, and supplied issuer signatures/path constraints.
  A lone leaf or a chain omitting its root is permitted; the supplied chain tip
  is an **inspection anchor only**, never proof that an operator trusts it.
- An optional OIDC CA file must contain only currently valid CA certificates,
  with certificate-signing use when key usage is specified. This does not prove
  that the provider's certificate chains to these anchors or that login works.
- Errors name the failed configuration field without disclosing local paths,
  credential contents or raw TLS parser errors. Failure emits no success output.

**Still not checked:** browser/company root trust, revocation, company IdP
discovery/token/login, DNS, management firewall, listener reachability, or service
readiness. This is a read-only snapshot, not an install/activation gate or a
guarantee against later file changes. No configuration is emitted or applied.
The command reports these limits even on success.

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

Role inclusion below is an engineering proposal, not a completed bundle or
approval of an external-service provider. Ports are observed source defaults,
not a customer firewall specification. No internal listener should be published
merely because its port is listed.

| Role | Current source/build input | Configuration, trust and data | Packaging/readiness gap |
| --- | --- | --- | --- |
| Visa Service | [`vs`](../../../vs/), Rust workspace; `cargo build --locked --release -p vs` | Explicit VS TOML, policy, API keys, TLS and retained identity; VSAPI default 5002, Admin HTTPS 8182; Redis-protocol state backend defaults to 6379 | Pin server image, state-backend distribution/persistence and private network; readiness must verify loaded policy, node admission and actual decisions |
| Node runtime | [`libnode2`](../../../../zpr-core/libnode2/), binary `lntest` | Node identity/key, VS address, ZPR address, advertised substrate address and reverse VSS reachability | Current executable is a CLI/TUI/test-oriented entry point; certify a headless supervised node entry point and its network/privilege requirements before shipping |
| Endpoint/service adapter | [`ph`](../../../../zpr-core/adapter/ph/), `cargo build --locked --release -p ph` | Per-device identity, authentication trust, TUN/network state; credentials must come from the supported provisioning flow | Platform packaging, privileged handoff, revocation and traffic acceptance remain required; do not ship static demo bootstrap keys |
| Policy-Service | [`policy_service.go`](../../cmd/zpr-web-dashboard/policy_service.go), dashboard Go binary in `policy-service` mode | mTLS, explicit organization and SQLite repository, compiler config/source/signing inputs; default 8789 | Package ZPLC/ZPT and independent fixtures/LDAP inputs; distinguish draft/staged policy from active runtime policy; readiness must exercise compile/evaluate |
| Control-Service | [`control_service.go`](../../cmd/zpr-web-dashboard/control_service.go), same Go binary in `control-service` mode | mTLS default 8790; VS Admin URL/CA/key; Policy-Service mTLS; gateway draft store and configured trusted-service integrations | No dedicated production image yet; eliminate dependence on demo launcher paths and unnecessary host Docker access |
| Control Room | Same Go binary in `control-room` mode; embedded browser assets | HTTPS default 8787, exact origin, OIDC config/secret/CA, grants/delegation and Control-Service client identity | [`operator_login.go`](../../cmd/zpr-web-dashboard/operator_login.go) preserves loopback mode and adds opt-in direct private management-IP/company-hostname access with required scoped OIDC; container-network/installer integration and live company acceptance remain open |
| Device Enrollment Service | [`zpr-enrollment-service`](../../cmd/zpr-enrollment-service/main.go), separate Go command | Explicit JSON, private device HTTPS and opt-in separate admin mTLS listener, owned SQLite registry and optional runtime-import signer | Opt-in Control-Service relay removes its registry dependency; BAS registry access and full lifecycle/credential/bootstrap contracts remain unfinished. No cross-service shared SQLite mount in the production package |
| Authentication | [`zpr-bas`](../../../../zpr-bas/), `cargo build --locked --release` | BAS TLS identity, protected actor records and shared BAS/VS token-verification secret | Package supported enrollment-backed authentication and issuance, not demonstration records; OIDC human login is a different trust path |
| Compiler/evaluator | [`zplc`](../../../../zpr-compiler/), `cargo build --locked --release`; `zpt` from Visa workspace | ZPLC configuration, per-company signing material, input schema and policy binary compatibility | Pin compiler/common-library versions and native runtime dependencies; no compiler source checkout on customer host |
| Operator identity/directory | External company OIDC and directory services | Issuer/subject grants, redirect origin, TLS, protected client/bind credentials and configured attribute mappings | Company-provided OIDC and directory approved; directory protocols/mappings and first-admin recovery remain open. Do not install demo Dex/LDAP as production defaults |
| DNS | Existing network DNS (optional companion bootstrap DNS for isolated labs), distinct from [`ZPR BIND9`](../../../dns/bind9/) behind an adapter | Explicit bootstrap endpoints; protected DNS address/policy, publisher and separate viewer TSIG identities | Protected BIND9 approved for the combined demonstration; package its adapter and independent supervised namespace. Bootstrap must not depend on ZPR DNS. Config generation exists, not service deployment; no automatic acme-dns |
| Gateway | Existing gateway runtime and [`rollout contract`](../../scripts/gateway-runtime-rollout.md) | Independent installed identity, allowlist, destination/forwarding configuration and adapter link | Decide whether bundled or optional; editor activation/rollback and CONNECT enforcement limits remain unresolved |
| Telemetry | Existing [`observability`](../../../observability/) and configured logs/metrics sources | Persistent backend data, retention, operator endpoints and credentials | Decide required diagnostic baseline versus optional backend; missing optional telemetry must not masquerade as healthy/empty data |
| Simulator | Development/test tooling only | Simulation profiles, workloads and manifests | Excluded from the production appliance and its startup dependencies |

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
