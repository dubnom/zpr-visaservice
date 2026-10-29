# ZPR system packaging and release plan

Status: **In progress (M1 inventory; M2 host preflight, configuration and
bundle-manifest validation, Control Room input renderer, and approved runtime
contract)**, created 2026-10-15, updated 2026-10-17 and clarified 2026-10-23.
The Wave 2 evaluation-only starter is a design target; installation
implementation remains open. This document defines implementation milestones
and the recurring process for real system releases. It does not certify the
current system, authorize deployment, or claim an installer exists.

## Current execution priority: demo, not production

See the [forward-work portfolio](../../ZPR_PORTFOLIO.md) for a summarized,
cross-track priority/dependency index. This release plan retains ownership of
production acceptance gates; the specific demo contract owns limited-demo
acceptance. Neither track's completion implies the other's.

The dated [release-readiness assessment](RELEASE_READINESS_ASSESSMENT.md)
separates available engineering/evaluation/demo evidence from outstanding
production qualification and flags conflicting historical platform summaries.
It is an inventory, not new deployment or release acceptance.

On 2026-10-17 the user clarified that the immediate objective is working ZPR
and Simulator demo packages. Demo-only shortcuts are acceptable if their limits
and required production fixes are recorded. The selected target is a pair of
version-matched archives adapted from the existing `zpr-demo/multinode-demo`,
tested on an isolated disposable Ubuntu 24.04 x86-64 host. The user explicitly
allows the existing TUN/`NET_ADMIN`/privileged-container path and Simulator's
root-equivalent Docker-socket access only on that isolated demo host. The
first Gateway claim is limited to explicitly proxy-configured demo clients.

The [demo package contract](zpr-dashboard/packaging/system/DEMO-PACKAGE-CONTRACT.md)
is the immediate implementation and acceptance target. A source-tree launch,
unit test, or validator pass is not a package test; both downloaded archives
must pass their paired clean-host acceptance before the demo milestone is
complete. Do not publish or deploy the candidate without separate authorization.
The company-install and production-release requirements below remain valid
follow-up work, not blockers to the limited demo package unless the contract
calls them out as demo prerequisites.

## Objective and first-release boundary

A new company must be able to install ZPR using published, verified artifacts
and its own configuration, without developer assistance, a source checkout,
demo credentials, or Simulator. The installation must survive restart and have
a tested recovery and upgrade path.

First deliverable: a **single-server production pilot**. On 2026-10-15 the user
approved Ubuntu 24.04 LTS on x86-64, a dedicated server/VM, a signed container
appliance using Docker Engine/Compose v2, and remote Control Room HTTPS on a
company hostname. The user also approved the company's existing OIDC provider
and company-supplied TLS certificate/key files with company-managed renewal.
Remote access uses direct TLS on a selected private management IP, reachable only
through the corporate network/VPN; reverse proxy and public-Internet deployment
are outside the initial topology.
One endpoint platform must be certified; existing Mac pilot
priority is unchanged. Capacity, availability and general-availability claims
are not approved by those choices. Multi-site deployment,
high availability and additional endpoint platforms are later release scopes.

Every pilot, general-availability, maintenance and hotfix system release uses
the process below. Local development deployments and source publication are
not substitutes for a system release.

### Next operator test deliverable

On 2026-10-15 the user selected a **complete from-scratch company installation
package**, not a Control Room-only image connected to an existing installation.
Testing should require release artifacts and company inputs, not a source
checkout, Go/Rust toolchain, or manual assembly of individual services.

#### Fresh-install experience clarified on 2026-10-18

The initial installation must be a fully working ZPR environment, not a
simulated or mocked success and not a set of services that the operator must
assemble or start individually. It starts independently of the organization's
existing LDAP, monitoring, and other platform services so a new operator can
begin without first integrating their company infrastructure. Where those
capabilities are required at first run, the installation must provide real,
operational bundled services and generated per-install state; bundled services
must not be demo substitutes.

The installer installs the required runtime dependencies, configures and
supervises the complete initial ZPR service set, and starts it. The Control Room
is the operator's path to configure company-specific identity, monitoring, and
other platform integrations and then migrate the deployment toward the
organization's environment. This migration path must be explicit, validated,
and reversible; it must not silently replace providers or lose local state.
Operators should not need to choose or launch individual ZPR components during
a new installation.

Simulator is a separate learning tool for understanding ZPR concepts and
behavior. It is not required to install or operate ZPR, is not part of the
production service set, and must not be a source of production state or
service contracts. The download page should explain the install-to-Control-Room
journey without exposing a dependency checklist. Until a complete installer
passes its acceptance gates, current component snapshots must remain clearly
identified as development downloads rather than presented as turnkey packages.

The first-run flow must install required runtime dependencies and start the
service set without asking the operator to assemble components. It must not
weaken host privilege boundaries to make startup convenient. The initial
operator must receive a secure, per-install administrative bootstrap path with
no reusable default credential; Control Room access and recovery are explicit
acceptance criteria.

The installer offers existing company services or bundled production services
for operator login, device/actor authentication and trusted attributes, and
logging. Company underlay/bootstrap DNS remains an external input.
**ZPR-managed service DNS is part of the ZPR package**,
not an external company prerequisite or a Simulator-owned service. ZPR already
feeds its service records and is intended to control its policy-aware behavior.
The package must include the protected DNS service, its adapter, publisher
integration, configuration and persistent state. Future per-client DNS policy
filtering is planned work, not an implemented capability claim.
The package must include all required ZPR services and their runtime dependencies;
external company integrations must be explicit, validated configuration, never
demo replacements. No existing Visa Service, Control-Service, production
database, or Simulator installation may be assumed on the test host.

Required handoff:

- A single versioned downloadable bundle containing loadable, pinned images,
  the installer/validator, release manifest, checksums and verification material.
  Engineering candidates must state their unsigned/unqualified status explicitly;
  they cannot masquerade as signed production releases.
- One operator guide for company inputs, preflight, installation, verification,
  stop/restart, failed-install recovery and explicit data-preserving rollback/
  removal. Expected results and actionable failure output accompany each step.
- Locally generated unique company trust/service identities and protected
  persistent state; retries must preserve identities and existing customer data.
- An acceptance report from a fresh supported Ubuntu host: named operator login,
  first endpoint enrollment/admission, loaded policy and actual allowed/denied
  traffic, plus restart/recovery with Simulator unavailable.

**Not available yet.** Current blockers include the Enrollment Service's
sole-writer/API boundary, supervised node entry point, company trust/bootstrap
ordering, production images/configuration/container networking and endpoint
credential delivery. Passing Control Room tests or shipping its engineering
binary does not meet this deliverable. Gateway is now required in the first
install BOM by the 2026-10-17 user direction; its runtime activation/rollback
and enforcement contract remains a release blocker. Logging is a required
capability with bundled or existing-provider modes, but backend qualification
and company directory/authentication/attribute protocols and mappings remain
open. Do not silently install demo services to close these gaps.

Enrollment ownership progress: an opt-in separate mTLS administration listener
now serves existing delegated invitation routes in the Enrollment process,
sharing that process's device registry. Control-Service can relay original
signed named-user assertions over its own mTLS client connection without opening
the registry; mixed remote/local registry configuration is rejected. Enrollment
pins the relay transport identity and independently verifies the Room signing
key, service audience, request/body, named grants and durable replay nonce.
This is a compatibility bridge, not the proposed full `/v1` lifecycle API.
The standalone BAS command still opens its configured registry and must be
separated before claiming complete sole-writer ownership or shipping the
production package. No services were started or reconfigured.
Local verification passes full enrollment/delegation race suites, focused
Control-Service relay/audience, existing delegation and management HTTPS race
tests, vet, and Linux amd64 Enrollment Service/dashboard builds. Required
`make test` retains the same four recorded dashboard failures. None of this is
fresh-Ubuntu, complete lifecycle or package acceptance evidence.

Related plans:

- [Provisioning plan](../Provisioning%20plan.md): enrollment, credential delivery,
  endpoint installation and platform certification.
- [Source reorganization](REDIRECTORY.md): production ownership and dependency
  boundaries.
- [GUI backlog](GUI%20work.md): operator workflows and outstanding activation work.
- [Gateway rollout](zpr-dashboard/scripts/gateway-runtime-rollout.md): gateway
  runtime enforcement and rollout limitations.
- [Deployment ledger](GUI%20deployments.md): historical component deployments,
  not certification of a complete system release.

### Bundled-or-existing trusted services and logging

On 2026-10-17 the user approved an installer choice for both identity roles
and logging. This supersedes the earlier existing-company-provider-only
restriction; it does not approve substituting lab/demo services for production.

- Offer explicit existing-provider or bundled-provider configuration for
  operator login, device/actor authentication and trusted attributes, and logging.
  A company without its own providers must have a complete bundled installation
  path. Record the selected modes and validate every required integration.
- Bundled operator login must satisfy the production OIDC contract. Device/actor
  authentication and attribute publication are distinct contracts; operator
  login alone does not admit an adapter or authorize traffic.
- Bundled logging includes collection, persistent storage and authorized operator
  querying, not merely container stdout. Define retention, bounded disk usage,
  access controls, sensitive-data handling, backup and failure reporting.
- Generate unique per-install identities/secrets; provide explicit initial
  administrator setup, least-privilege grants and recovery. No shared passwords,
  demo accounts, implicit grants or silent fallback when an external provider
  fails. Preserve identities and persistent data across retries/upgrades.
- These are independently supervised production services with explicit contracts
  and operator configuration. They must run with Simulator absent and must not
  read its manifests, sessions, profiles or workloads. Reuse appropriate provider
  software, not Simulator-owned runtime state or lifecycle.
- Provider implementations and versions remain to qualify. This does not select
  BAS, LDAP/Dex or a logging backend as the production implementation, nor make
  BAS mandatory. If BAS is selected, its registry ownership/API gaps must close.
- Acceptance covers bundled and external-provider paths: operator login, actor
  admission, trusted attributes, allowed/denied traffic, queryable logs,
  restart/recovery and explicit dependency failures, with Simulator unavailable.

This is an approved requirement, not an implemented installer feature.

### Evaluation-only bootstrap profile (2026-10-23)

The 2026-10-17 decision above remains the production requirement: a production
company installation must offer supported company-provider or qualified bundled
identity/attribute and logging services. The 2026-10-23 direction adds a narrower
**EVALUATION-ONLY** beta starter profile using current lab packages so an operator
can boot a usable ZPR before linking company services. This is an explicit,
profile-scoped exception to production qualification, not a reversal of the
production requirement or evidence that an installer is already complete.

The evaluation starter brings up LDAP and logging with the ZPR installation and
its own persistent state. It is visibly labeled **not production supported**;
all secrets are unique per install, protected and never default/shared. Company
LDAP and logging are linked alongside the starter. Retirement happens only in a
later explicit operator action after source/policy bindings are reviewed and
rebound; there is no automatic identity merge, silent source switch or fallback.
The evaluation milestone sequence and visual acceptance are tracked in the
[Wave 2 bootstrap plan](SIMULATOR_AS_ZPR_CLIENT_PLAN.md#2-wave-2-bootstrap-evaluation-only-starter-profile-2026-10-23).

#### Current package, image and script inventory

| Capability / current artifacts | Verified capability now | Gap before the evaluation profile can claim completion |
| --- | --- | --- |
| Lab bootstrap: independent `-evaluation` B1 mode plus original `zpr-dashboard/cmd/zpr-lab-bootstrap`, including `-identity-services`, `-identity-compose`, and `-runtime-outputs`; `zpr-dashboard/packaging/simulator/README.md` | Generates protected lab configuration, synthetic LDAP data, and a separate LDAP/Dex Compose file and private state directories when explicitly requested. The command generates files; it does not initialize/start services. `identity-compose` requires explicit private bind IP and immutable LDAP/Dex image IDs/digests. LDAP is explicitly demo-only/unqualified; image digest syntax is checked but image contents/platform are not inspected. | B1 now separately renders/starts native production-tagged Control Room/Control-Service and current LDAP/OpenObserve/Dex with private unique stable state, lifecycle locking, verified protocol health and foreground cleanup. See the evaluation runbook. Service/source registration, complete installer/clean-host qualification and production pins remain pending. |
| Bundled identity candidates: `zpr-dashboard/scripts/Dockerfile.operator-idp` (Dex v2.44.0 digest), `zpr-lab-bootstrap` LDAP renderer and `identity-compose` output | Dex base image reference is digest-qualified. The caller supplies an immutable OpenLDAP image reference; generated LDAP/Dex state uses dedicated directories and restricted Compose settings. | Evaluation now selects the current packaged `osixia/openldap@sha256:0de3850635911109f0daf8f5957808ec4f4eb827fbaaa339100b5f1bab925745` (`linux/amd64`); no production OpenLDAP release image is selected or qualified; no production support claim, company-source linking/retirement path or integrated registration into an installed ZPR package. Bootstrap profile is lab/companion oriented and does not yet provide a production/evaluation installer lifecycle. |
| LDAP provider and push feed: `zpr-dashboard/cmd/zpr-trusted-service`; `scripts/ldap-syncprov.conf`, `scripts/enable-ldap-sync.sh`; `-ldap-watch` and `-ldap-changes-base` | The service has a read-only LDAPS attribute provider, an mTLS HTTP accesslog change feed returning metadata, and a separate read-only RFC 4533 syncrepl consumer emitting metadata-only JSONL with reconnect/resync behavior. The syncprov helper validates/enables one data database and the consumer tests include isolated OpenLDAP cases. | **B4 implemented:** Control-Service optionally embeds the RFC 4533 consumer from production network-LDAP source configuration, owns private durable organization/source checkpoints and reconnect/resync health, refreshes approved attributes and invokes the existing Visa Service cache invalidation/reconciliation path. Deterministic race tests cover restart resume, isolation, malformed/replayed events and source/Admin/persistence failures without Simulator. Standalone stdout mode remains diagnostic/in-memory. Remaining acceptance: actual LDAP-to-running-Visa-Service revocation, async reconciliation completion/queue-failure receipts, end-to-end bootstrap registration and operator accesslog/syncprov provisioning; see [B4 contract](zpr-dashboard/ASSERTIONS.md#production-ldap-push-watcher-b4). |
| Starter logging: `observability/openobserve/` profile, `zpr-dashboard/diagnostics/openobserve-provider.example.json`, dashboard OpenObserve provider | Existing optional OpenObserve profile uses a digest-pinned upstream image and a ZPR adapter-bound access path; configured diagnostics can query a named OpenObserve endpoint. | The profile is not a turnkey installation logging source: it requires an already provisioned adapter/identity/policy and separate setup. Operator logging role, startup, authentication/authorization, retention, bounded storage, persistence, health and registration are not integrated. OpenObserve AGPL-3.0 is accepted for this evaluation profile only, with 7-day global retention and admin-only loopback TLS access. B1 starts the backend but not collectors/source registration. Production license/runtime/data-handling qualification remains required. |
| Runtime composition and test surfaces: `zpr-dashboard/internal/systemruntimecompose`, `cmd/zpr-system-compose-render`, packaging/system preflight/config/manifest tools, `packaging/demo-runtime` and `packaging/simulator` | Read-only preflight, schema/config validation/rendering, Compose renderers, profile renderers and paired demo/simulator artifacts exist as engineering slices. | They do not currently compose/start the complete production/evaluation service set or qualify a fresh-host bootstrap. The simulator package is separate; no evaluation single-command install, starter source registration, complete logging bootstrap, company link/retirement flow, or integrated clean-host acceptance exists. |

**Push-feed scope:** bundled LDAP must enable both the existing accesslog-backed
change feed and the RFC 4533 syncrepl watcher in the independent ZPR evaluation
stack, not only when Simulator bootstrap is running. Verify refresh/add/modify/
delete, disconnect/reconnect, cookie rejection/resync, and feed health with
Simulator absent. This is metadata change observation only. The current code has
no consumer that refreshes ZPR attributes or revokes visas from these events;
that behavior is a distinct future milestone and must not be claimed as shipped.

**Simulator companion:** adding Simulator registers its own isolated synthetic
directory as a separate trusted source. It does not populate or mutate bundled
starter LDAP. The companion and starter must have separate credentials, state,
Compose lifecycle and health. Company LDAP/logging are added alongside both;
only explicit source/policy rebinding and a separately confirmed retirement can
stop the starter.

#### Remaining implementation decisions

The following user decisions are frozen for the evaluation profile only:

- **Directory:** use the current packaged immutable OpenLDAP image above; do not
  imply production support or copy Simulator's synthetic directory into it.
- **Admin recovery:** a local CLI reset must generate a unique one-time password,
  write it only to a new `0600` local file, invalidate prior sessions and force
  replacement on first use. **B1-RECOVERY is a design/task, not shipped:** static
  Dex password mode lacks forced-change/one-time semantics. Implement supported
  provider updates with ownership/lock/expiry/replay/rollback tests for the operator
  and independent OpenObserve admin; never reset by deleting/reseeding data.
- **Logging:** OpenObserve **AGPL-3.0 accepted, evaluation-only, 7-day global
  retention, administrator-only access**. B1 verifies authenticated/unauthenticated
  boundaries and TLS; B3 now supplies allowlisted lifecycle ingestion, named source identity
  and authorized Control Room querying. Resource/disk/backup and production
  retention/legal/runtime qualification remain separate gates.
- **B4:** Control-Service owns **per-source** change-watcher checkpoints, resync
  and metadata health; the watcher is not Simulator-owned. B1 does not start it.
- **B6:** explicit retirement wizard: **link company source → rebind policies →
  verify → disable bundled**. Show affected state and source/policy/logging
  references, require authorized confirmation and offer recovery. No automatic
  merge, fallback, source switch or disabling on company-source connection.

B1's supported local command, image platforms, protected restart/cleanup contract,
protocol-readiness scope, active demo shortcuts and gaps are documented in
[`packaging/evaluation/README.md`](zpr-dashboard/packaging/evaluation/README.md).
The local implementation is not a complete first-install BOM or production
installer; it does not change source-registration UI/handlers. Clean-host,
platform and browser qualification remain release acceptance work.

**BOM/companion reconciliation:** these current evaluation LDAP/logging/IdP
packages are an evaluation-only subset, not a qualification of the production
bundled-or-existing choices or a replacement for required Visa/Policy/Enrollment/
Gateway artifacts. Simulator remains a separately installable optional add-later
companion with separate state, credentials, source and lifecycle. No Control Room
or Control-Service path depends on it; stopping/omitting it must not affect B1.

### First-install BOM scope and provisional candidates

On 2026-10-17 the user directed the first-use package to bundle all service
artifacts needed for a new installation, while documenting company-provider
and logging integrations as alternatives. The recommended BOM split, approved
for planning, is:

- **Required production path:** Visa Service and its persistent state backend;
  a supervised production node and one certified endpoint adapter; Policy-
  Service, Control-Service and Control Room; Enrollment; compiler/evaluator;
  protected BIND behind the ZPR adapter with Visa's RFC2136/TSIG publisher;
  and Gateway. The service artifacts needed for a self-contained start path
  belong in the downloadable bundle. External provider/logging selection may
  omit starting the corresponding bundled instances, but must not trigger
  silent fallback.
- **Bundled-or-existing integration choices:** operator OIDC; device/actor
  authentication and trusted attributes; persistent, authorized logging.
  The bundled path must be independently supervised and Simulator-free. The
  current company config documents operator OIDC only; provider and logging
  handoffs still need explicit contracts and operator instructions.
- **Separate companion:** Simulator and its lab LDAP/Dex services, plus
  opt-in bootstrap BIND for isolated labs. The companion must pair with the
  exact same ZPR release artifacts, use separate provider state, and exercise
  production APIs. Simulator is not part of or required by the production
  bundle. Company underlay DNS and supplied HTTPS certificate/key remain
  external inputs.
- **Optionality distinction:** company underlay/bootstrap DNS is external by
  default; companion bootstrap BIND is optional. Protected ZPR DNS is
  required. Gateway is required by the new direction, although its complete
  runtime contract is not yet qualified. Identity and logging are required
  capabilities, with a bundled path and supported existing-provider option.

The user approved these source/image references for engineering qualification
planning only, not as compatible release pins or production approvals:

| Candidate | Current source reference | Known limit before it can enter a release lock |
| --- | --- | --- |
| Visa Service, dashboard modes, Enrollment and bootstrap/config tools | Visa Service `main` `29b4b13dc85dec9538f19012a7fa7e6e54ac8857`; dashboard Go module declares Go 1.26.1 | Source revision is a candidate snapshot, not a release; Go toolchain/image recipe and fresh-host qualification are not locked. Current Visa worktree has packaging/plan changes, not source artifacts to treat as a release. |
| PH adapter and core libraries | Core `work/macos-tun-address` `561fa7351795a723b74b5d1e57761d870722c77c`; workspace version 0.7.0 | Work branch, no supported image/build handoff; current `libnode2` executable is `lntest`, not a qualified supervised production node. |
| BAS device-auth candidate | BAS `main` `9de4a25192a21b61a2f92639104abf5985e223a8`; crate 0.1.0, Rust 1.98.1 | Candidate only; standalone BAS registry access conflicts with complete Enrollment sole-writer ownership until the API/ownership gap closes. |
| ZPLC and shared source set | Compiler `work/rest-trusted-services` `6f0de17fd9052567394ad306552bc435efc3ed3f`; common `main` `2756e9a01d5552965439f94314a6b0550832cf8f`; policy `main` `d2ad9d8c1ca1007ff17b9364d2734392f44f6575`; VSAPI `main` `140748f926e7675f6923568f5c0f1fef00e482aa` | ZPLC 0.16.0 currently depends on `zpr-common` tag v0.24.0, while Visa/Core path dependencies use 0.25.2. Resolve and test compatibility before treating these revisions as a lock. |
| Dex bundled OIDC candidate | `ghcr.io/dexidp/dex:v2.44.0@sha256:5d0656fce7d453c0e3b2706abf40c0d0ce5b371fb0b73b3cf714d05f35fa5f86` | Present in `Dockerfile.operator-idp`; engineering source reference, not production qualification. OpenLDAP image/version remains unselected. |
| OpenObserve bundled logging candidate | `public.ecr.aws/zinclabs/openobserve@sha256:d4a878fac1f6c56003764f7f2a1625668917388f167e222c8c810de3f54c56ba` | Existing profile is optional and AGPL-3.0. Its wrapper uses mutable `debian:trixie-slim` and unpinned apt packages; license, target, data/retention and access-control qualification remain. |
| Protected BIND | Visa `dns/bind9/Dockerfile`; isolated engineering test used BIND 9.20.29 on arm64 | No immutable digest or apt package pin and no Ubuntu amd64 runtime qualification. The user did not approve this as a release pin. |
| State backend | Visa Service Redis-protocol state, default 6379 | Redis/Valkey implementation/version/image is not selected or pinned. |

The candidate source set is not release-qualified or signed. No component
owners are assigned by this table; the user remains accountable for owner
designations before production. Exact immutable images, compatible source
revision closure, toolchains, supported platform manifests, SBOM/license
review, signing and acceptance remain open. No host/service/credential action
is authorized by this planning decision.

### M2 first implementation slice: offline bundle validation and archive creation

**State:** Implemented in the current Visa Service worktree as
`zpr-dashboard/internal/systembundle`, `cmd/zpr-system-bundle`,
`cmd/zpr-system-bundle-pack`, `cmd/zpr-system-bundle-verify` and
`cmd/zpr-system-bundle-sign`; an isolated Linux/amd64 staging archive now
gathers the tools with local checksums and build metadata. Detached Ed25519
signatures bind exact archive bytes and can be verified against an independently
provided trusted public key. Focused Go and shell regressions have been
reported; test evidence is qualified below. These remain engineering
validation/packaging tools, not an installer or qualified release artifact;
key custody, trust distribution, rotation/revocation and release policy remain
open. The tool archive is not published as a customer download.
The reported `go test ./internal/systembundle` run is excluded from acceptable
evidence because that suite includes signature/key tests. Do not repeat it.
This exclusion does not apply to the separately verified CLI-only test command
below; C0 did not rerun the internal package suite.
W5's `PKG-SYSTEM-BUNDLE-CLI-TESTS-01` adds command-level tests for valid system
validation, exact companion pairing, and checksum-mismatch failure output.
After W5 released `cmd/zpr-system-bundle/main_test.go`, C0 reran
`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -count=1 ./cmd/zpr-system-bundle`
successfully. These tests use harmless temporary payloads and create no keys or
secrets.
W5 then added fail-closed CLI coverage for a companion without its required
system pair, a system manifest supplied with companion-pair arguments, and a
companion whose valid ZPR source revision differs from the system manifest.
The new `cli_rejection_test.go` reservation is released. C0 reran the same
CLI-only command after that handoff; it passed with `-count=1`. The
`internal/systembundle` package test remains excluded and was not run.

The version-1 manifest distinguishes system and companion bundles, records
roles/capabilities, install selection, target platform, local payload hashes,
immutable image references and optional full source revisions. System
validation requires the approved production capabilities, including Gateway
and protected DNS, and bundled-default identity, actor-authentication,
trusted-attribute and logging paths. It rejects Simulator/lab-only and
bootstrap-DNS roles in the production manifest. Companion validation requires
Simulator plus lab LDAP/OIDC; bootstrap DNS, if present, is selectable. Pair
validation requires the exact same ZPR component identities and target in both
manifests.

The validator checks local manifests and payload checksums. The archive builder
rechecks payloads while creating a deterministic archive of the manifest and
declared files. The archive verifier validates that downloaded archives contain
exactly the declared regular-file payloads and prints their archive SHA-256;
companion archives are checked against the paired system manifest. Detached
Ed25519 signatures may be created with a separately supplied protected key and
verified against a separately provisioned public key. Signing authenticates
bytes only and is not production qualification. All manifests still accept
`engineering-unqualified` status only. OCI layout archives now verify the
manifest/config/layer blob digests and declared platform, but the tools do not
inspect image filesystems, live dependencies, installation, provider-state
separation, runtime health or production qualification. Non-endpoint artifacts
must declare the Ubuntu host platform; the standalone endpoint artifact
declares a separate platform that is not thereby certified. No services are
started and no host changes are made. The tools reject tag-only image
references and path traversal/symlink payloads; they do not publish.

The candidate source set is not yet a release lock: ZPLC/common versions are
skewed, and exact target-qualified BIND, OpenLDAP, state-backend and Gateway
artifacts remain unavailable. No release manifest has been generated from
these candidates. Production implementation still requires closure of those
dependencies and the remaining service contracts/owner decisions. Consequently, the archive
builder cannot yet produce a complete ZPR or Simulator package for the fresh
installation test.

### Optional zpr.net bootstrap assistance

On 2026-10-17 the user clarified that zpr.net should assist initial setup and
getting an installation running, not become part of its daily operation.
This is a scope requirement, not an implemented hosted service.

- Keep bootstrap assistance optional; existing-provider and bundled-local
  installation paths must remain available without a hosted account.
- Define an explicit authenticated handoff to installation-owned services.
  A website account or download password does not confer local administrator,
  device-admission or network-access rights.
- Generate and retain installation/device private keys locally. Any hosted
  setup authorization must be narrowly scoped, short-lived and replay-protected;
  do not install a permanent zpr.net runtime credential or shared trust secret.
- After handoff, operator login, actor authentication/admission, trusted
  attributes, policy decisions, ZPR DNS, logging, restart, credential renewal,
  revocation and recovery use the selected installation-owned or company
  services, not zpr.net. No periodic hosted token refresh, licensing heartbeat
  or remote approval dependency is implied.
- Acceptance must block access to zpr.net after setup and verify normal
  operation, restart, new local enrollment and credential lifecycle behavior,
  including after temporary bootstrap authorizations have expired.
- Future cross-network gateway discovery, introduction or trust coordination
  may use zpr.net, but is not selected or implemented by this decision.
  Define bilateral authorization, trust scope and outage/data-path behavior
  separately before adding any federation dependency.

The public download portal remains separate from privileged bootstrap
services. This does not authorize exposing private Control Room or service
administration interfaces, select BAS, or change the current deployment.

### Multi-host, role-based deployment

On 2026-10-17 the user approved planning for a larger installation: additional
ZPR nodes, protected DNS and supporting services on separate hosts, potentially
using different operating systems and CPU architectures. The single-server
pilot remains the first deliverable, but must be the simplest instance of this
deployment model, not an installer design that assumes everything is local.
This is an approved architectural direction, not implemented multi-host,
multi-site, high-availability or additional-platform support.

Deployment roles:

| Role | Responsibility |
| --- | --- |
| Management | Control Room and installation-management services |
| ZPR node | Additional packet-routing capacity or placement |
| Protected DNS | ZPR-managed DNS, its adapter and record-publishing integration |
| Supporting services | Selected identity, trusted-attribute, logging and storage services |
| Endpoint adapter | Connect an endpoint or service to the installation |

A host may run one or more explicitly selected roles. Support is qualified per
**role + OS + architecture + release**, not by a blanket platform claim.
For example, a Windows endpoint and Linux DNS host may belong to one
installation once those combinations are implemented and certified.
Successful compilation alone does not qualify a role.

Required deployment contracts:

- Distinguish **create a network** from **join an existing network**. Creation
  establishes installation identity/trust, management, initial node and DNS.
  Joining selects host roles, uses administrator-approved short-lived,
  role-scoped, replay-protected authorization, generates private keys locally,
  enrolls with installation-owned services, obtains configuration and verifies
  readiness. Routing-node authorization must not confer management-admin rights.
- Define a versioned production deployment inventory with stable host/service
  identities, roles, platforms, service addresses, installed versions and
  desired configuration. Store secrets separately through protected references.
  Ownership and APIs remain to design; do not introduce another registry writer
  or derive this inventory from Simulator state.
- Use explicit authenticated service endpoints and contracts. Do not assume
  shared filesystems, localhost addresses or shared host credentials between
  services. Define ports, routing/firewall requirements, advertised addresses,
  trust, dependencies and health checks for each placement.
- Support adding, upgrading, draining, replacing, revoking and removing hosts.
  Preserve installation identity and customer state; define partial-failure
  recovery, compatible version skew, upgrade ordering and data ownership.
  Distinguish installed, enrolled, configured and healthy states, with explicit
  failures rather than treating process startup as deployment success.
- Define protected DNS authority, record-update ownership, placement,
  replication and failover before advertising redundant DNS. Preserve the
  distinction between ZPR service DNS and company underlay/bootstrap DNS.
- Multiple hosts do not imply high availability. Qualify management state,
  identity, DNS and routing failure/recovery independently, with explicit
  capacity and availability limits.
- Creation, later joins and ongoing lifecycle operations must work without
  Simulator or zpr.net. Optional hosted setup assistance does not become a
  daily administration or trust dependency.

Phased acceptance, after the complete single-server installation:

1. Add a second routing node on a separate host through the supported join path;
   verify admission, allowed/denied traffic and revocation/removal.
2. Place protected DNS on a separate host and verify real record publication
   and protected resolution; then qualify replication and failure/recovery
   before claiming redundancy.
3. Qualify selected mixed-platform role placements and rolling upgrades,
   including incompatible versions, interrupted joins/upgrades, host replacement
   and preservation of identities/state.

Retain topology, role/platform/version matrix and failure/recovery evidence.
Test with Simulator unavailable and zpr.net blocked after setup grants expire.
Exact platform priorities, scale/HA targets, inventory/API design and service
replication implementations remain decisions for the corresponding phase;
this does not expand the initial certified deployment boundary.

### Simulator companion package

On 2026-10-15 the user requested a separate companion package and approved
including **Simulator, LDAP, an OIDC identity provider and lab DNS**. It supplies
a self-contained lab environment for testing the real installation package
without requiring an existing company directory, identity provider or DNS.
This is an additional deliverable, not a replacement for the production bundle
or permission to bundle demo infrastructure into production defaults.

The lab installs the **same pinned ZPR artifacts and installer** shipped to a
company. The companion supplies external integration endpoints through ordinary
documented configuration and protocols, not an alternate ZPR build or a hidden
Simulator-aware mode. Simulator may configure and exercise ZPR through supported
service contracts; Control Room and Control-Service never read Simulator state.

For the 2026-10-23 evaluation bootstrap, the bundled starter LDAP is a separate
ZPR-owned service, not the companion's lab directory. Simulator add-on setup
registers its own synthetic directory as another trusted source and never
populates or reseeds the bundled source. Both sources and logging remain up when
Simulator stops. The 2026-10-17 companion request remains valid as the separate
Simulator/lab-services deliverable; it does not authorize substituting companion
state for the starter or change the production/evaluation distinction above.
Company LDAP and logging can be linked alongside the starter and companion. Any
later starter retirement requires an explicit, reviewed source/policy rebind and
operator confirmation; there is no silent identity merge or fallback.

### Simulator methodology: act as a ZPR administrator

Execution and chat coordination are tracked in the
[parallel migration plan](SIMULATOR_ADMIN_MIGRATION_PLAN.md), including one
user-facing coordinator, five bounded workstreams, decision gates, exclusive
file ownership and integrated acceptance. No worker chats are launched by it.

**Direction approved 2026-10-16; full migration and release acceptance remain
incomplete.** The verified bounded source milestone is recorded in the
[migration plan](SIMULATOR_ADMIN_MIGRATION_PLAN.md#verified-bounded-native-acl--ph-readiness-batch-2026-10-16);
it does not qualify native key operations, production apply or deployment.
Simulator should set up its environment as a real ZPR administrator would: configure through
Control Room's supported operator workflows, stage a reviewed revision, compile/
validate it, and explicitly push/apply the resulting candidate through production
services. Simulator continues to own simulated machine/adapter lifecycle,
user activity, workloads, fault injection and scenario execution.

Control Room is the operator interface, not a new backend dependency: automated
setup should use the same supported, authorized Control-Service/Policy-Service
contracts behind that interface. Browser acceptance must also exercise the
actual Control Room workflow. Do not add a Simulator-specific configuration API,
read production databases directly or treat an internal package call as proof
that the operator path works.

Ownership and limits:

- Production services own configuration records/revisions, validation,
  compilation/signing, staged artifacts, authorized deployment, applied-state
  reporting and rollback. Missing production capabilities must be implemented
  there, not bypassed in a Simulator launcher.
- Simulator supplies proposed lab configuration and desired identities through
  those contracts, observes explicit results, then drives simulated endpoints.
  Adapter enrollment/admission and credentials must follow production contracts;
  controlling a simulated machine does not grant permission to mint credentials
  or mutate an enrollment registry.
- The standard installer/bootstrap establishes the initial services, trust and
  operator access before this workflow. Simulator must not need a working ZPR
  policy to create the only identity capable of installing that first policy.
  Lab LDAP/OIDC and optional bootstrap DNS remain independently supervised
  companion services. Protected ZPR service DNS belongs to the ZPR package.
- Automation must have explicit organization scope and attributable, least-
  privilege authorization. How automation receives its operator authority is
  still a design decision; do not copy browser cookies, bypass CSRF or reuse a
  service TLS identity as unrestricted administrator authority.
- Configure, stage, compile and push are distinct recorded outcomes, even where
  today's service combines compile/sign/stage in one operation. A saved draft
  or compiled candidate is not an applied policy. Record exact revision,
  candidate digest, target, operation result and applied-state readback.
- Push/activation and rollback require explicit authorization and reviewed
  targets. Timeouts/lost responses require operation reconciliation, not blind
  resubmission. Scenario reset must not erase production state or restart shared
  services implicitly.
- Dependency direction remains Simulator -> production operator services.
  Control Room/Control-Service/Policy-Service must work with Simulator absent;
  stopping Simulator must not remove an installed configuration or its services.

Migration checklist:

- [ ] Inventory current activation/Restore setup steps, mapping direct policy
  compilation/install, file writes, service restarts and registry access to
  production operator contracts or explicit installer/machine-control duties.
- [ ] Identify which Control Room configuration/stage/compile/push/readback/
  rollback contracts exist and which are missing. Current policy staging
  compiles/signs a candidate; it is not a complete production push workflow.
- [ ] Decide the scoped automation authorization and approval model before
  wiring privileged setup requests.
- [ ] Prove one vertical slice: Simulator proposes a policy/config revision,
  the operator workflow validates/stages/compiles/applies it, production reports
  the exact applied revision, and simulated adapters demonstrate real allowed/
  denied traffic. Include denied authorization and stale-revision failures.
- [ ] Replace equivalent launcher bypasses only after that path passes.
  Preserve explicit preflight and non-destructive failure/rollback; no silent
  fallback to the old direct install path.
- [ ] Migrate remaining organization setup and Restore operations, keeping
  machine/workload control Simulator-owned and separating production recovery
  from scenario reset.
- [ ] Certify browser and automated operator-path parity, lost-response
  reconciliation, audit and rollback; stop Simulator and verify production
  configuration, services and independently driven traffic remain operational.

This changes the intended setup methodology, not the runtime today. Existing
direct launcher behavior and historical deployment evidence are not certification
of the new path.

#### Control Room editors for environment authoring

**Requirement added 2026-10-16; integration design remains open.** Simulator
operators need the Control Room editing workflows to create, maintain and test
their environments, not just an automated setup client. Production policy,
assertions, ZPR configuration and Gateway configuration must use the same
domain editors, validation/test contracts and revision lifecycle as real
administration. Sharing CodeMirror alone is not workflow parity.

- Reuse production-owned editor modules and supported service APIs rather than
  fork their behavior into Simulator-specific editors. Environment selection
  must resolve to explicit production organization/repository/target context;
  a Simulator manifest is not production configuration authority.
- Preserve permissions, source-owned diagnostics, unsaved edits, History,
  revision conflicts, reviewed AI insertion and explicit save/stage/compile/
  push boundaries. Clearly distinguish local edits, saved revisions, test
  results, staged candidates and verified applied state.
- Separate draft validation and non-deploying policy tests from live scenario
  execution against an explicitly applied revision. Record which exact source/
  configuration revision each result tests; editing or testing must not
  implicitly provision endpoints, activate an environment or push policy.
- Keep machine models, simulated people/activity, workloads, fault injection
  and scenario timelines Simulator-owned. Lab directory content uses its
  documented provider/provisioning contracts; do not imply a general production
  directory-write editor already exists.
- Production editors must remain usable with Simulator unavailable. Shared
  code belongs to production or neutral UI modules, never a production import
  of Simulator controllers or environment-discovery code.

Open implementation decisions and acceptance:

- [ ] Inventory editor capabilities and missing environment-management APIs;
  map each document to its production or Simulator owner.
- [ ] Choose how operators access the production editors from Simulator:
  navigation into Control Room or reuse of production-owned UI modules.
  Neither embedding nor cross-origin session sharing is approved implicitly;
  preserve each application's login, CSRF and authorization boundary.
- [ ] Prove one environment round trip: create/edit through the production
  editor, validate/test, save a revision, explicitly stage/compile/push, read
  back the applied revision, then execute a Simulator scenario against it.
- [ ] Verify equivalent editor behavior from both entry points, wrong-target/
  unauthorized requests, revision conflicts, stale diagnostics/test results,
  unsaved-navigation cancellation and production operation with Simulator off.

This is a requirement for operator workflow reuse, not a claim that all
Control Room editors or deployment operations already support environment setup.

#### Provisioning simulated adapters

**Open design requirement added 2026-10-16.** Controlling simulated endpoints
and authorizing their admission are separate responsibilities. Simulator may
create/start an endpoint and drive its enrollment client; production services
must decide whether that endpoint is approved and issue its credentials through
the normal provisioning contracts. No Simulator-only credential issuer, copied
adapter identity, direct registry insertion or pre-admitted manifest identity.

Proposed lifecycle, pending authorization and headless-client design:

1. A scoped administrator registers/invites the intended lab asset through the
   production enrollment administration workflow, with explicit organization,
   owner and approved profile. A simulated user is not automatically an
   administrator.
2. The endpoint generates its own key and claims the invitation with proof of
   possession through the production enrollment client/protocol. Each adapter
   has a distinct identity and protected per-endpoint state. Credentials and
   one-time codes must not appear in manifests, scenario source or logs.
3. Production approval, issuance and secure runtime delivery complete before
   Simulator declares the adapter provisioned. Invitation creation, claim,
   approval, issuance, admission and actual connectivity remain distinct states.
4. Simulator starts/controls the adapter using the delivered identity and
   observes real admission and allowed/denied traffic. Restart preserves that
   endpoint's identity; cloning creates a new identity and requires enrollment.
5. Reset/rebuild explicitly distinguishes retained identity, re-enrollment and
   authorized revocation/removal. Scenario reset alone must not erase production
   enrollment records or silently create replacement identities.

Decisions and acceptance still required:

- [ ] Inventory current simulated adapter types and credential/bootstrap paths;
  identify a supported first endpoint/client rather than assuming all adapters
  use the same enrollment mechanism.
- [ ] Define supported noninteractive enrollment and secure invitation delivery.
  The current loopback setup wizard is not already a headless provisioning API.
- [ ] Choose how scoped automation requests invitations and how a named operator
  approves them; unattended approval is not implied by this methodology.
- [ ] Define endpoint key storage and secure credential handoff, including
  the limits of software-backed identities on simulated machines.
- [ ] Prove two independently enrolled adapters, denied/unapproved enrollment,
  replay/duplicate-claim rejection, restart identity preservation, expiry/
  renewal, revocation and explicit reset/re-enrollment reconciliation.
- [ ] Verify issued credentials and production admission continue to work with
  Simulator orchestration stopped and the endpoint processes still running.

Track production issuance/runtime-delivery gaps in the
[provisioning plan](../Provisioning%20plan.md); do not solve them with lab-only
shortcuts. This lifecycle is a proposed contract, not implemented provisioning.

On 2026-10-15 the user clarified the outcome: **both packages together must
demonstrate a working ZPR network**, not merely running infrastructure or
populated dashboards. OpenLDAP and Dex backed by LDAP are approved companion
providers. BIND9 has two separate roles: optional companion bootstrap DNS for setup/login
hostnames, and a protected DNS service inside ZPR behind a real adapter and
ordinary ZPR policy. Bootstrap must not depend on that protected service being
up; protected queries must not fall back to underlay DNS.

Subsequent clarification: use existing network DNS by default for bootstrap
names in both company and lab installations. Companion-hosted bootstrap BIND
is an explicit isolated-lab opt-in, not a package prerequisite. The generator
exports fully qualified underlay record requests without creating a new zone
or changing resolvers by default. Underlay record generation and protected
ZPR DNS generation are independent selections. mDNS is not a required bootstrap
mechanism; `.local` HTTPS/OIDC support remains unimplemented. Focused race tests,
isolated BIND config/zone loading, vet and Linux amd64 build pass for these
independent modes; full `make test` retains the same four recorded dashboard
failure groups. No service or resolver settings were changed.

The combined acceptance path must demonstrate:

- A fresh installation of the exact real-package artifacts with the companion's
  LDAP/OIDC/bootstrap DNS, named operator login, and a loaded initial policy.
- At least one real node, enrolled/admitted adapters with distinct identities
  and a reachable test service. Simulator supplies scenarios/workloads, not
  simulated substitutes for admission, Visa decisions or packet transport.
- A permitted request through the actual ZPR adapter/node/service path,
  retaining status code and bounded raw response; a policy-denied request with
  correlated enforcement evidence. A timeout alone does not prove policy denial.
- BIND bound only to the DNS adapter's ZPR address, permitted DNS resolution
  over the authorized ZPR TCP/53 path, denied DNS access for an ungranted
  identity, and no protected DNS socket exposed on the underlay.
- Visa Service's separately authorized TSIG record publication and a distinct
  read-only transfer identity, without giving query clients update credentials.
- Revocation and restart/state recovery; repeat login/traffic/DNS checks with
  the Simulator application stopped while supporting services remain running.

Record artifact versions, identities, applied policy revision, path and
correlated observations for each result. This is the combined-package release
acceptance target; none of these runtime claims is certified by file generation.

Required companion handoff:

- A separately versioned, verified downloadable bundle with pinned/loadable
  Simulator, LDAP, OIDC and DNS images, installation tools, manifest, checksums
  and verification material. Publish compatibility with exact ZPR release IDs.
- A guided lab setup that creates protected per-install credentials, test
  identities/groups, OIDC client registration and exact callback, lab DNS
  records and an explicit integration-settings handoff for the real installer.
  Never ship shared private keys or reusable default passwords.
- Explicit lab hostnames, addresses, ports and TLS trust setup for both services
  and operator browsers/endpoints. Lab trust remains separate from release
  signing trust and production company trust; no disabled TLS verification.
  The existing company TLS input contract is unchanged.
- Independently supervised supporting services: stopping the Simulator
  application or scenario runner must not stop LDAP, OIDC or DNS. The real
  package must continue normal operation without Simulator APIs or state.
  Loss of a required supporting service must instead produce its normal explicit
  dependency failure, not a synthetic success or local fallback.
- Clearly labeled lab-only data and network exposure, explicit installation
  targets and collision checks, and separate state/backup/removal controls.
  Resetting a scenario must not erase real-package identities or databases;
  cross-package destructive resets require explicit operator authorization.
- Instructions covering install order, integration handoff, first login,
  endpoint enrollment, actual allowed/denied traffic, restart, recovery and
  data-preserving removal. Neither bundle requires a checkout or local compiler.

Implementation sequence:

1. Define the companion service inventory, LDAP schema/group mappings, OIDC
   claims/subjects, DNS zones, endpoint reachability and protected handoff format.
   OpenLDAP/Dex/BIND9 are approved; exact provider versions/images and same-host
   versus separate-host networking remain to qualify. Do not assume current
   demo launchers meet these contracts.
2. Package and bootstrap the supporting services independently of the Simulator
   application, with unique identities, health checks and persistent state.
3. Wire the standard real-package installer to the resulting explicit inputs;
   add Simulator scenario orchestration only through supported ZPR APIs.
4. Publish and qualify the compatible pair using exact release artifacts.
   Run the lab acceptance path with Simulator active, then unavailable while
   LDAP/OIDC/DNS remain healthy; also exercise each dependency outage and recovery.

Every real release must record companion compatibility and retain this paired
lab evidence alongside the recurring gates below. Companion tests supplement,
but never replace, fresh-company production acceptance with Simulator absent
and independently configured external integrations. No companion bundle or
installer exists yet.

Initial implementation: the standalone
[`zpr-lab-bootstrap` tool](zpr-dashboard/packaging/simulator/README.md) now
generates unique per-install lab CA/server identities and LDAP/OIDC service
secrets into a new protected directory. It refuses existing output and writes
a relative-file inventory last. It does not configure providers, seed accounts,
register clients, set DNS, install browser trust or render installer inputs.
Focused race tests, vet and Linux amd64 build pass, including real-package TLS
certificate validation and CA DNS name constraints. Required `make test` retains
the same four recorded dashboard failure groups; no service was launched.

DNS rendering slice: optional explicit network inputs now generate separate
bootstrap and ZPR-only BIND configurations/zones, unique publisher/viewer TSIG
keys, a Visa DNS update fragment and a least-privilege ZPL fragment. The fragment
allows the publisher only; explicit client grants and actual adapter deployment
remain required. Bootstrap records resolve Room/Simulator/LDAP/OIDC independently
of protected `svc.zpr`. This generates files only, not a running DNS service.
Focused race tests, vet and Linux amd64 bootstrap build pass. An existing
engineering arm64 BIND 9.20.29 image loaded both configurations and zones in
isolated read-only/network-disabled validator containers; this is not Ubuntu
amd64 runtime qualification. Required `make test` retains the same four
recorded dashboard failure groups.

LDAP/OIDC rendering slice: optional explicit HTTPS origins now generate
OpenLDAP/Dex configuration, uniquely passworded lab users/groups, separate
one-time directory initialization and restart-only startup scripts, and a
relative-file identity input inventory. LDAP-backed Dex uses persistent
`entryUUID` identities, verified LDAPS and exact separate client callbacks.
The read-only bind account cannot read password hashes or mutate the directory.
An isolated arm64 engineering-container integration test passed real TLS LDAP,
authorization-code/PKCE login, signed OIDC issuer/audience/nonce/group checks,
initialization refusal on existing state and stable subjects across LDAP/Dex
restart, without Simulator. This is not real Room authorization or combined
network acceptance. Actual verified issuer/subject grants, trusted attribute
mapping, qualified images and installer/bootstrap remain unfinished;
no existing runtime stack was changed. Focused race tests including live
LDAP/Dex and isolated BIND validation, vet, formatting and Linux amd64 bootstrap
build pass. Required full `make test` retains the same four recorded dashboard
failure groups.

The approved initial deployment is the same Ubuntu server with **separate
stacks and persistent state**. Optional identity Compose generation now pins
explicit images, binds an explicit private IPv4, captures the target invoking
UID/GID, and separates one-time LDAP initialization from normal startup. Its
authenticated LDAPS health gate, real PKCE/OIDC login and preserved-subject
container recreation passed isolated arm64 engineering tests with Simulator
absent. No existing runtime stack was changed. This is a startable identity
slice, not the complete companion or a qualified Ubuntu amd64 release.
See the [same-host identity instructions](zpr-dashboard/packaging/simulator/README.md#same-host-independent-identity-stack).

## Non-negotiable boundaries

- **OPA-1 operator access (user direction 2026-10-29):** shipped operator tools
  retain protected loopback setup/recovery and policy-controlled ZPR access.
  An explicitly switchable operator browser gateway is a separate ingress;
  disabling it must terminate its access without disabling loopback or ZPR.
  See the [operator access plan and blocking acceptance matrix](OPERATOR_ACCESS_PLAN.md).
  Implementation and release qualification remain open.
- Control Room and Control-Service operate independently of Simulator. They
  never read simulation manifests, profiles, sessions or workloads. The server
  installation does not install or require Simulator.
- Production services consume explicit production configuration and public
  service contracts. Demo startup/Restore scripts are not the production
  installer. Shared implementation can be reused only with independent inputs.
- Each company receives unique trust material, identities and administrator
  grants. No shared bootstrap secrets, example private keys or fictional users.
- Save draft, apply configuration, successful process startup and verified
  traffic are distinct states. The installer must not report one as another.
- Distribution signing keys and company operational keys have separate owners
  and lifecycles. Never bundle either private key in a public artifact.
- No customer pilot is released with an incomplete credential-delivery path or
  an uncertified endpoint advertised as supported.

## Decisions required before implementation

Record the approved answers in the first release record; do not silently
convert recommendations into requirements.

| Decision | Proposed starting point / required answer |
| --- | --- |
| Host support | **Approved:** Ubuntu 24.04 LTS, x86-64 dedicated server/VM. Resources, supported virtualization and minimum privileges remain open |
| Deployment format | **Approved:** container appliance, Docker Engine/Compose v2, pinned images and signed installer. Exact runtime versions remain to qualify |
| Access model | **Updated user direction 2026-10-29:** protected loopback for bootstrap/recovery, permanently configured policy-controlled ZPR access, and independently switchable operator browser gateway. This supersedes the earlier direct-private-management-only/no-reverse-proxy restriction for operator access; it does not authorize public Internet exposure. Gateway placement, authenticated local recovery, HTTPS origins and session shutdown remain to implement/qualify under [OPA-1](OPERATOR_ACCESS_PLAN.md) |
| Initial endpoint | Complete and certify one platform; current Mac pilot priority remains unchanged unless explicitly reprioritized |
| Identity sources | **Updated approval (2026-10-17):** installer choice of existing company providers or bundled production services for operator OIDC login and device/actor authentication with trusted attributes. Bundled services are independent of Simulator, not demo identities. Provider qualification, protocols/mappings, credentials and first-admin recovery remain open |
| Logging | **Approved (2026-10-17):** installer choice of an existing logging integration or bundled collection, persistent storage and authorized querying. Backend qualification, retention/resource bounds, access controls, backup and failure behavior remain implementation work |
| DNS and certificates | **Approved:** existing network DNS for bootstrap in production and labs; companion-hosted bootstrap DNS is optional for isolated labs. Production uses supplied/renewed HTTPS files. The combined network must also include BIND9 behind a policy-controlled ZPR adapter, distinct from bootstrap DNS. Protected DNS does not replace company infrastructure DNS; no appliance ACME automation selected. Provider image/adapter packaging, DNS integration and certificate replacement/reload/recovery remain open |
| Gateway | **Required in first-install BOM (user direction 2026-10-17):** include the gateway service/runtime artifact; implementation, activation/rollback, external enforcement and package qualification remain open |
| Connectivity | State initial download/egress requirements; offline installation is unsupported unless separately built and tested |
| Reliability and scale | Pilot capacity, acceptable downtime, backup destination, RPO/RTO and support lifetime |
| Multi-host expansion | **Approved direction (2026-10-17):** role-based create/join and lifecycle model, followed by second node, separate/redundant DNS and qualified mixed-platform deployments. Exact platform priorities, inventory/API design, version skew and scale/HA targets remain open; see [multi-host deployment](#multi-host-role-based-deployment) |
| Release authority | Named owners for build, test, signing, approval, deployment and support |

## Initial implementation milestones

Use **Planned -> In progress -> Verified -> Released** for each milestone.
Verification requires retained evidence; release requires the recurring gates.
No milestone is checked complete at creation of this plan.

### M1. Define the product and production bill of materials

In progress: the [production packaging inventory](zpr-dashboard/packaging/system/README.md)
records source-grounded roles, ports, trust/data dependencies, observed
repository revisions and packaging gaps. It is not a finalized BOM, license
audit, source lock or approved service-provider selection. Milestone remains open.

- [ ] Approve the decisions above and list supported/unsupported deployments.
- [ ] Inventory Visa Service, node/adapter runtime, Policy-Service,
  Control-Service, Control Room, Enrollment/Authentication services, compiler
  and policy tooling; implement the approved split: protected DNS and Gateway
  are required; identity/attribute and logging capabilities have bundled and
  existing-provider modes; companion bootstrap DNS is optional and lab-only.
  Resolve remaining backend/provider implementations and contracts.
- [ ] Record ports, protocols, trust relationships, storage, health contracts,
  required privileges and startup dependencies. Optional services must remain
  visibly optional; required missing services must block readiness.
- [ ] Define the [multi-host role model](#multi-host-role-based-deployment),
  placement contracts and role/OS/architecture/release qualification matrix,
  without broadening the initial pilot's support claims.
- [ ] Map every shipped component to its repository, source revision, build
  recipe, license and accountable owner. Include sibling repositories and
  external dependencies, not only the dashboard repository.
- [ ] Audit defaults, environment/configuration discovery and packaging paths
  for Simulator, developer-machine and demo-data dependencies.
- [ ] Define the separate Simulator/LDAP/OIDC/DNS companion inventory and
  standard integration handoff, keeping lab and production ownership distinct.

Exit evidence: approved support matrix, service/dependency diagram, component
inventory and independent production configuration contract.

### M2. Build independent bootstrap and installation

Initial slice: [read-only host preflight](zpr-dashboard/packaging/system/preflight.sh)
and [19 isolated regression cases](zpr-dashboard/packaging/system/test-preflight.sh)
are implemented and locally verified. Checks cover only the approved host OS/
architecture and local Docker/Compose baseline. Resource/network/application
preflight and actual Ubuntu VM qualification remain open; no installer is built.

The [management-bootstrap configuration](zpr-dashboard/packaging/system/README.md#company-management-bootstrap-configuration)
now has a versioned example and read-only `zpr-system-config` validator, reusing
operator OIDC validation. Opt-in `-check-files` adds offline protected-file,
TLS key/hostname/validity/supplied-chain and OIDC secret/CA checks, reusing the
runtime's protected-file reader. A total of 158 selected configuration/file/CLI
and existing operator regressions pass; focused race tests, vet and Linux amd64
validator/dashboard builds pass, and the full suite retains four recorded failures. Browser/company trust, live IdP
checks, installed file/mount access, complete service configuration, runtime
composition and production container-network integration remain open. A
separate Control Room runtime-input renderer now exists, but file checks are
not wired into an installer and the renderer does not build or apply service
composition. The complete installation-config milestone is not checked.

On 2026-10-17, schema v2 added source-backed Control-Service connection inputs:
the HTTPS origin, mTLS client certificate/key and service-CA file references,
plus an optional TLS server-name override. Schema v1 remains unchanged. The
offline checker applies protected-file access checks to the new references but
does not validate client certificate/key matching, service-CA trust or
reachability. The `zpr-system-config-render` command maps validated v2 inputs to
the existing Control Room and Control-Service environment variables, exact
host-and-port listener, read-only source-to-destination mount records, and
operator OIDC JSON at `/etc/zpr/config/operator-oidc.json` with mode `0600`.
The CLI requires offline file checks to pass, writes only a new mode-`0600`
output file, and refuses to overwrite an existing output. The five-tool
Linux/amd64 engineering archive includes the renderer. Reported focused tests
for the renderer CLI and named key-free runtime-rendering cases passed; the
bundle-tools script verified all five ELF files and checksums without executing
them. Focused vet, formatting and `git diff --check` also passed. The full
`internal/systemconfig` suite and key-generating file-check test were not run.

This is a renderer only: it does not copy private files, provide the
Control-Service server identity, configure provider/logging/node services,
build Compose configuration, install or start services, or check service
health. W5 completed the v2-reference update to
`cmd/zpr-system-config/files_test.go`; the installation configuration remains
incomplete.

Client certificate/key matching, service-CA parsing/trust and certificate
identity verification remain deferred: safe fixture-based coverage would
require using key/certificate material, which is not authorized. Protected-file
access checks do not establish runtime mTLS.
W4 completed syntax-only validation of the optional TLS server-name override:
host-only ASCII DNS names and unbracketed IP literals are accepted, while the
empty fallback and original override value are preserved. The named
configuration tests, `go vet`, formatting and `git diff --check` passed
without file access or key/certificate material. This does not establish
certificate SAN identity, CA trust or mTLS operation.

The user-approved first-qualification profile and source-grounded requirements
are recorded in the [first-install runtime
contract](zpr-dashboard/packaging/system/INSTALLER-RUNTIME-CONTRACT.md). The
profile is one Ubuntu 24.04 x86-64 host with local Docker Compose, an explicitly
configured HTTP/HTTPS proxy Gateway (not transparent interception), and
bundled Dex/OpenLDAP identity candidates and an OpenObserve logging candidate.
For production qualification Dex's image reference is provisional and OpenLDAP
remains unselected (the separate B1 evaluation profile now uses the current
packaged immutable OpenLDAP image above); OpenObserve
still requires AGPL-3.0, image/runtime and data-handling review. Protected DNS
and Gateway are required; Simulator remains a separate companion and is not a
production dependency. Internal service mTLS uses per-install CA/service
identities, but custody, algorithms, rotation, backup/restore and recovery
remain gates. Gateway acceptance covers clients explicitly configured to use
the proxy; endpoint admission is tested separately, and no automatic endpoint
traffic routing is claimed. The contract defines roles, trust and persistent-
state boundaries, readiness and recovery requirements, but is not a
release-ready installer specification or a production artifact lock.
Component ownership remains pending.

Implementation still has seven blockers: (1) Enrollment lifecycle and
credential-delivery ownership, including one production registry writer;
(2) a supervised production node and supported, least-privilege endpoint
adapter path; (3) compatible immutable service artifacts/build recipes,
compiler compatibility, state backend and DNS package; (4) approval of the
per-install service-CA algorithms, custody, rotation, compromise response,
backup/restore and first-admin recovery; (5) protocol health probes,
DNS/Gateway activation, client routing, firewall rules and bootstrap order,
with Gateway tests limited to explicitly configured proxy clients until the
adapter supports automatic routing; (6) pinning and qualification of Dex,
OpenLDAP and OpenObserve, including the OpenObserve license review; and
(7) complete per-service configuration, transaction recovery, Compose
rendering and clean Ubuntu acceptance against the verified bundle. The
contract does not authorize credential generation or use, installation, or
deployment.

Direct management HTTPS runtime support is now opt-in with an exact private-IP
listener and company hostname. It requires initialized single-company OIDC,
valid supplied TLS and named route/company authorization, preserves the existing
loopback/Simulator boundary, and rejects foreign origins/forwarded headers.
Local tests include real hostname-verified TLS, company-callback OIDC login,
CSRF/permission denials, logout and independent mTLS API forwarding with Simulator
unavailable. No real management interface/firewall, company IdP, Ubuntu install or
production container deployment is certified by these local fixtures.
The final shared worktree passes 238 selected configuration/operator checks,
focused race tests, vet and Linux amd64 validator/dashboard builds; required
`make test` retains the same four recorded failures. An additional isolated
source-HEAD comparison has an identical baseline failure set, not a green full
suite. Nothing was deployed.

- [ ] Provide a versioned, validated company installation configuration, with
  secret references separated from ordinary editable configuration.
- [ ] Design configuration for stable host/service identities, explicit role
  placement and authenticated remote endpoints; define the production inventory
  contract and role-scoped create/join lifecycle before multi-host implementation.
- [ ] Implement non-mutating host/network preflight, explaining each failure
  before privileged changes. Do not upload discovered company data implicitly.
- [ ] Provide a guided setup workflow: company identity, service endpoints,
  DNS/TLS, administrator identity provider and integration configuration.
- [ ] Generate unique company trust material locally, protect private files,
  establish service identities and close first-administrator bootstrap access.
- [ ] Install verified pinned artifacts, persistent data and service startup
  definitions without a source checkout, local compiler or demo seed.
- [ ] Make installation resumable and idempotent: retry must not regenerate
  identities, duplicate assets, overwrite customer configuration or reset data.
- [ ] Produce a step-by-step outcome, actionable errors and a resumable failure
  state. Document rollback, uninstall and explicit data-retention behavior.
- [ ] Build the separately installable companion and its protected integration
  handoff; run the unmodified real-package installer against its lab services.

Exit evidence: fresh supported host reaches the authenticated Control Room
using only release artifacts and company inputs, with Simulator unavailable.

### M3. Complete useful production operation

- [ ] Complete **CFG-1**, the [ZPR Config lifecycle gate](CONFIGURATION_LIFECYCLE_STATUS.md):
  distinguish configuration from policy; identify supported consumers/targets;
  implement exact-revision apply/reload/restart, effective-state readback,
  drift/partial-failure status and explicit recovery/rollback. Saved Config
  drafts and guarded policy upload do not satisfy this requirement.
- [ ] Complete **OPA-1**: protect loopback setup/recovery, provide direct ZPR
  access to each shipped operator tool, and implement authorized operator-gateway
  enable/disable with confirmed session termination and independent path status.
  Include bootstrap policy, per-tool permissions and local outage recovery.
- [ ] Complete one platform's enrollment through trusted credential issuance,
  privileged runtime handoff, live adapter admission and revocation.
- [ ] Finish platform signing/distribution requirements; do not bypass OS trust
  controls to obtain a passing install.
- [ ] Review and activate a minimal company policy with real allowed and denied
  traffic tests and explicit cleanup of endpoint route/DNS/interface changes.
- [ ] Implement independently authenticated configuration apply/status/rollback
  for supported services and gateways, with exact revisions and capability
  validation. Do not claim HTTP method/path enforcement inside opaque CONNECT.
- [ ] Verify named-user authorization, organization isolation, audit attribution,
  first-admin recovery and enrollment approval separation for the supported
  operator model. Any single-operator exception must be explicit and audited.
- [ ] Present component versions, applied versus draft revisions, certificate
  status, diagnostics and readiness without requiring developer shell access.

Exit evidence: a new company can enroll its first endpoint, administer supported
configuration and demonstrate access control through the actual ZPR path.

### M4. Make installations maintainable

- [ ] Implement protected backup/export and tested restore for trust material,
  registry/data stores, configuration, policies and operator authorization.
- [ ] Define atomicity/consistency across stores and external systems, recovery
  key handling, restore isolation and prevention of duplicate live identities.
- [ ] Implement supported upgrade paths, configuration/schema migration,
  interruption recovery and rollback. A binary downgrade alone is not rollback
  after an incompatible data migration; identify the restore-required boundary.
- [ ] Test service/host restarts, lost dependencies, reauthentication and
  certificate renewal/rotation over the declared operating window.
- [ ] Add bounded logging, disk/resource failure handling, redacted support
  export and readiness diagnostics. Document support and incident procedures.

Exit evidence: restore and upgrade/rollback meet the declared RPO/RTO and
preserve company identity, policy and authorization.

### M5. Automate and rehearse the release process

- [ ] Build immutable artifacts in controlled CI from recorded clean revisions
  and pinned toolchains/dependencies; establish reproducibility checks and
  document any non-reproducible signing/packaging steps.
- [ ] Produce the system release manifest, component hashes/image digests,
  SBOM/license inventory and build/test provenance.
- [ ] Establish protected release signing and independently distributed trust
  anchors, including signer rotation/revocation and artifact withdrawal.
- [ ] Reuse existing enrollment signing/verification conventions where suitable;
  the existing Debian enrollment manifest is not a whole-system release format.
  Design and test a versioned system format rather than silently extending it.
- [ ] Automate fresh-host acceptance, supported upgrade acceptance and evidence
  collection; require a first-time operator usability rehearsal.
- [ ] Rehearse the complete recurring process below before admitting a customer.

Exit evidence: a retained signed candidate, completed release record and an
independent clean-host installation of those exact downloadable artifacts.

## Recurring process: run for every real release

These stages are ordered gates. A failed gate stops promotion. Fixes create a
new candidate identity and rerun affected gates; changing packaged bytes
invalidates earlier artifact-specific acceptance.

### R0. Open the release record and freeze scope

- [ ] Assign release ID, channel, candidate ID, accountable owners and dates.
- [ ] Record features, fixes, exclusions, support matrix and supported upgrade
  origins. Identify endpoint/server/component compatibility requirements.
- [ ] Pin exact source commits for every component, dependency locks and build
  inputs. Exclude dirty or unreviewed worktree content.
- [ ] Review open defects and existing failed tests. A failing required test
  blocks release. Any demonstrated unrelated exception requires a named
  approver, evidence, customer impact, expiry and a repair issue; a baseline
  comparison alone is not permission to ship.
- [ ] Confirm implementation milestones required by this release are verified.

### R1. Build, inspect and sign the candidate

- [ ] Build all supported artifacts and collect provenance, component versions,
  hashes/digests, SBOM, licenses and dependency/vulnerability review results.
- [ ] Check artifacts for secrets, demo identities, developer paths and mutable
  dependency references. Include installer tools needed on a clean host.
- [ ] Sign the finalized candidate and verify it using independently provisioned
  release trust. Test wrong signer, tampering, version mismatch, expiration and
  unexpected/unsupported artifacts.
- [ ] Place candidate artifacts in an immutable staging location. Never rebuild
  between acceptance and publication; platform signing/notarization must be
  complete before testing the final downloadable package.

### R2. Run engineering qualification

- [ ] Run **OPA-1** ingress/authorization and lifecycle tests, including browser
  identity protections, listener bypass checks, long-lived gateway sessions,
  concurrent commands and unknown outcomes. Retain per-application results.
- [ ] Run required Rust/Go tests, formatting/lints, protocol/integration tests
  and supported desktop/tablet operator checks for the release components.
- [ ] Test authorization, organization isolation, configuration revisions,
  secret exclusion and production operation with Simulator absent and its
  environment variables unset or deliberately invalid.
- [ ] Test compatibility across supported server/node/endpoint versions, with
  explicit handling of an unsupported peer or configuration.
- [ ] Qualify the declared Simulator companion pairing using the exact real
  package: LDAP/OIDC/DNS integration, scenarios, Simulator application outage,
  individual dependency outages/recovery and isolated reset/removal behavior.
- [ ] Retain command/environment identifiers, pass/fail counts and complete
  private logs; release notes must not turn targeted checks into suite passes.

### R3. Certify fresh-company installation

- [ ] Pass **CFG-1** for each declared supported ZPR Config target: edit/save,
  authorized production apply and independent effective-state/behavior readback,
  with Simulator unavailable. Do not substitute policy-install evidence.
- [ ] Pass **OPA-1** fresh bootstrap and gateway-off acceptance: protected local
  setup before fabric availability, real allowed/denied ZPR operator access,
  verified gateway session shutdown and continued loopback/ZPR operation.
- [ ] Start from a disposable fresh supported host outside the development
  workspace. Download and verify the exact staged signed artifacts.
- [ ] Use a new company identity, keys and administrator; no pre-existing runtime
  state, source mounts, manual developer repairs or Simulator.
- [ ] Exercise preflight, installation, first login, policy activation and
  endpoint enrollment through credential delivery and actual admission.
- [ ] Verify allowed traffic, evidenced denied traffic, DNS/gateway behavior
  where included, revocation and endpoint network cleanup.
- [ ] Reboot the host and endpoint; verify state, trust and reconnect behavior.
- [ ] Verify interrupted installation/resume, invalid configuration, unavailable
  dependencies and unauthorized access produce explicit recoverable failures.
- [ ] Have an operator follow only the shipped instructions. Record manual
  interventions as defects rather than omitting them from the acceptance record.

### R4. Certify upgrade and recovery

- [ ] Pass **CFG-1** interruption, lost-response, restart, drift/partial-failure
  and rollback/restore cases, preserving valid authority and OPA-1 recovery.
- [ ] Pass **OPA-1** restart/upgrade/restore and outage recovery: gateway-off
  persists, old state cannot silently reopen ingress, and authorized protected
  loopback recovery works without ZPR, gateway or normal IdP availability.
- [ ] Install each declared upgrade origin and create representative state,
  including policy revisions, enrolled identities and operator grants.
- [ ] Take and verify a consistent backup, upgrade using the signed candidate
  and confirm migrations and state preservation.
- [ ] Exercise upgrade interruption and the documented rollback/restore path.
  Verify real traffic and authorization after recovery, not merely service health.
- [ ] Restore a backup onto an isolated replacement host and verify the declared
  recovery objective without connecting duplicate identities to production.
- [ ] Exercise dependency outages, service restarts, certificate lifecycle and
  sustained reauthentication for the declared test duration and capacity.
  Record measured limits; do not infer HA or scale from a single-host pilot.

For the first release, upgrade-from-prior-release testing may be explicitly
not applicable with approval; failed-install recovery, backup and restore may
not be skipped. Later releases must name and test their supported origins.

### R5. Approve and publish

- [ ] Release approver reviews completed evidence, unresolved defects, support
  claims, migration notes, rollback boundaries and operational instructions.
- [ ] Hard blockers: broken trust/signature verification, incomplete advertised
  enrollment, production dependence on Simulator, failed required access-control
  checks, unsupported data loss or unverified promised recovery.
- [ ] Publish the exact accepted bytes, signed manifest, installation and
  upgrade/recovery guides, compatibility matrix, known limitations and release
  notes. Promotion changes channel metadata, not artifact content.
- [ ] Download from the customer-facing distribution location and verify
  signatures/hashes again. Confirm release trust is available through its
  independent channel. Archive the evidence and approval.

### R6. Roll out, observe and close

- [ ] Obtain customer/site authorization, change window and rollback owner;
  verify site prerequisites and backup before deployment. Publishing is not
  authorization to mutate customer systems.
- [ ] Deploy first to an agreed pilot/canary, verify installed versions,
  identities, actual allowed/denied traffic and unaffected service state.
- [ ] Observe for the release-defined window. Stop expansion on identity,
  policy-enforcement, data-integrity, admission or recovery regressions.
- [ ] Record accept/rollback/withdraw decision. Rollback uses the tested
  compatible configuration/data path, followed by health and traffic acceptance.
- [ ] Publish advisories/withdrawal metadata if needed; do not replace bytes
  under an existing release ID. A replacement is a new release/candidate.
- [ ] Close with actual deployments, limitations, support handoff and follow-up
  issues. Capture lessons as improvements to this process.

Hotfixes use the same gates. Narrow evidence only with a documented impact
analysis; do not skip signature verification, affected clean-install/upgrade
coverage, access-control checks or rollback readiness.

## Required release record and evidence bundle

Create one immutable record per candidate/release in the chosen release system;
the location and retention policy must be decided in M5. Keep public release
metadata separate from access-controlled logs and customer acceptance records.
Never include enrollment codes, private keys or unredacted customer exports.

| Record section | Required contents |
| --- | --- |
| Identity and scope | Release/channel/candidate, dates, owners, source revisions, changes, exclusions |
| Artifacts and trust | Filenames, platform, hashes/digests, signed manifest, signer identity, provenance and SBOM |
| Compatibility | OS/CPU support, component version matrix, upgrade origins, schema/config versions, Simulator companion pairing |
| Qualification | Gate results, exact artifact IDs, test environments, logs, measured capacity/duration |
| Operations | Installation, backup/restore, upgrade/rollback, certificate and first-admin recovery guides |
| Exceptions | Defect, evidence, customer impact, approver, expiry and repair issue; no hard-blocker waivers |
| Approval and distribution | Named approval, publication location/time, post-download verification |
| Rollout and closure | Authorized sites, private acceptance evidence, observation window, outcome and support handoff |

The release owner coordinates; component owners provide build/compatibility
evidence; qualification owns independent acceptance; the signing custodian
protects signing authority; the approver makes the go/no-go decision; the
deployment owner executes only authorized rollouts. Assign people explicitly.
Any role combination for a small-team pilot must be recorded, not confused with
the enrollment service's separate device-approval authorization rules.

## Definition of done

A system release is complete only when the published signed bytes are the
accepted bytes, a fresh company can install within the supported boundary,
supported upgrades and recovery pass, the evidence/approval is retained and
the authorized pilot rollout is accepted or explicitly recorded as not yet
deployed. **Built**, **Verified**, **Published**, **Deployed** and
**Customer accepted** remain separate recorded facts.


### Multiple trusted sources: authorization decision gate (wave 7)

Status: **partly supported; source-isolated permission authorization blocked on
contract decisions**, not completed by multi-source registration work.
See [MULTISOURCE_POLICY_SUPPORT.md](MULTISOURCE_POLICY_SUPPORT.md) for the complete
trace, seven requested scenarios, implemented support and demonstrated limits.

- [x] Characterize same-org two-LDAP snapshots, duplicate subject/attribute names,
  cross-source assertions, independent browsing, peer outage/recovery, display
  rename and backend rebinding with real composed tests, without Simulator.
- [x] Characterize compiled permission evaluation, last-writer source collisions,
  same-subject lookup fanout and source-specific revision invalidation.
- [x] Add desktop/tablet visual source-isolation screenshots with identical
  subjects/different values and verify production boundary/registration tests.
- [ ] Decide source-qualified condition contract versus validated disjoint
  namespaces; define handling of unqualified policies and explicit subject links.
- [ ] Decide identity-changing source edit behavior and dependency-aware outage
  semantics; keep conservative failure closure until approved.
- [ ] Decide global versus dependency-indexed reconciliation; current global
  sweeps do not meet “only source-A-based grants are re-evaluated”.
- [ ] After the contract decision, add a complete LDAP-to-visa authorization
  acceptance test for all seven guarantees. Current composed tests deliberately
  do not imply that unsupported source-qualified permission syntax exists.
