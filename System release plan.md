# ZPR system packaging and release plan

Status: **In progress (M1; initial M2 preflight/configuration slices)**, created 2026-10-15. This document defines implementation
milestones and the recurring process for real system releases. It does not
certify the current system, authorize deployment, or claim an installer exists.

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

For production, the company supplies its existing OIDC provider, directory and DNS services.
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
binary does not meet this deliverable. Gateway/telemetry inclusion and company
directory protocol/mapping details still need explicit component contracts;
do not silently install demo services to close these gaps.

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
| Access model | **Approved:** direct HTTPS on a selected private management IP and company hostname, corporate network/VPN only; required OIDC, no reverse proxy/public exposure. Runtime opt-in implemented; firewall/routing verification and container-network/installer integration remain open |
| Initial endpoint | Complete and certify one platform; current Mac pilot priority remains unchanged unless explicitly reprioritized |
| Identity sources | **Approved:** production uses company's existing OIDC provider and directory, not bundled demo identity/directory services. The separate Simulator companion supplies lab LDAP and OIDC through the same integration contracts. Supported directory protocol/mappings, credentials and first-admin recovery remain open |
| DNS and certificates | **Approved:** existing network DNS for bootstrap in production and labs; companion-hosted bootstrap DNS is optional for isolated labs. Production uses supplied/renewed HTTPS files. The combined network must also include BIND9 behind a policy-controlled ZPR adapter, distinct from bootstrap DNS. Protected DNS does not replace company infrastructure DNS; no appliance ACME automation selected. Provider image/adapter packaging, DNS integration and certificate replacement/reload/recovery remain open |
| Connectivity | State initial download/egress requirements; offline installation is unsupported unless separately built and tested |
| Reliability and scale | Pilot capacity, acceptable downtime, backup destination, RPO/RTO and support lifetime |
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
  and policy tooling; classify DNS, directory, gateway and telemetry dependencies
  as bundled, customer-provided or optional.
- [ ] Record ports, protocols, trust relationships, storage, health contracts,
  required privileges and startup dependencies. Optional services must remain
  visibly optional; required missing services must block readiness.
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
checks, installed file/mount access, full service configuration/rendering and
production container-network integration remain open. File checks are not yet wired into an
installer; the complete installation-config milestone is not checked.

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
