# First-install runtime contract

Status: approved profile boundaries and proposed implementation contract;
not an installer specification ready for release. This contract does not select
release artifacts, create credentials, or claim that the services can currently
be composed.

The [common minimum for a working ZPR network](../../../System%20release%20plan.md#common-minimum-for-a-working-zpr-network)
applies across deployment profiles. This production first-install contract
specifies one implementation profile; passing its service/container checks
alone does not satisfy the baseline without the end-to-end evidence recorded
there.

## Approved first-qualification profile

- One fresh Ubuntu 24.04 x86-64 host.
- A local Docker Engine and Docker Compose installation on that host. The
  installer must reject a remote Docker endpoint and must not change Docker
  permissions or silently add privileges.
- A bundled identity and logging path for the first viability profile.
  Existing-company providers remain a later selectable configuration, not an
  implicit fallback.
- Dex plus OpenLDAP are the selected bundled identity candidates. Dex has a
  provisional pinned image reference; OpenLDAP image/version and the complete
  production mapping/bootstrap contract remain unselected.
- OpenObserve is the selected bundled logging candidate for qualification only.
  Its AGPL-3.0 license, image/runtime, access-control, retention, and data
  handling require review before distribution approval.
- Internal service mTLS uses a unique per-install CA and service identities.
  The CA is persisted protected and reused across installer retries. Its
  algorithms, custody, rotation, backup, restore, and recovery procedures must
  be approved before any implementation creates keys.
- An explicitly configured HTTP/HTTPS proxy Gateway. This is not transparent
  traffic interception, a VPN, or a TUN-based enforcement claim.
- The first Gateway acceptance uses a client explicitly configured for that
  proxy; endpoint enrollment/admission is tested separately. Gateway claims are
  limited to traffic actually routed through the proxy.
- Simulator is a separate companion. No production service may require it,
  read its files, or wait for its availability.
- Gateway and protected ZPR DNS are required services. A missing required
  service or failed readiness gate prevents installation success.

## Bundle contents and trust boundary

The download is one versioned system archive with:

1. A release manifest naming each component, role, target platform, immutable
   source revision or digest-qualified image, and payload checksum.
2. Loadable `linux/amd64` service images and any required adapter/runtime
   artifacts. Customer installation must not build from source.
3. The installer, offline validator, and service-specific health/acceptance
   probes.
4. A versioned operator guide for inputs, install, verification, stop/restart,
   interrupted-install recovery, backup/restore, and data-preserving removal.
5. Public verification material or an explicitly labeled
   `engineering-unqualified` status. An engineering candidate must never be
   presented as signed or production-qualified.

The existing manifest validator and packer do not yet constitute this bundle:
they validate/package declared payloads but do not supply production images,
configure the host, or install services.

## Component runtime responsibilities

Every service runs from its own immutable image or explicitly identified
platform artifact. Build-only tools and Simulator assets do not enter the
production service set.

| Service role | Runtime responsibility | Contract needed before composition |
| --- | --- | --- |
| Visa Service and state backend | Own authoritative ZPR state and expose the configured private APIs to the node and management services. | Exact image/runtime recipe, backend implementation and digest, state location/ownership, TLS bootstrap, and readiness that proves the expected policy and node are active. |
| Production node | Establish the authenticated node-to-Visa connection and report the installed node identity. | A headless supervised executable, address/identity config, reconnect/stop behavior, container privileges/devices, and an end-to-end health probe. The current `lntest` entry point is not this contract. |
| Endpoint adapter | Apply the supported endpoint traffic path using a provisioned per-device identity. | Supported target and privilege boundary, credential-delivery API, install/remove behavior, and an actual allow/deny test. No static demo key or bootstrap credential may be packaged. |
| Policy-Service and compiler/evaluator | Validate, persist, compile and evaluate the configured company policy; publish only an explicitly activated policy. | Compatible pinned compiler/common-library set, storage and signing ownership, TLS identities, a provisioned initial policy, and a probe that verifies the active policy digest plus expected allow and deny decisions. |
| Control-Service | Serve the production operator API over authenticated service TLS and relay owned Enrollment operations without opening Enrollment's database. | Final Enrollment lifecycle API and sole-writer boundary, required service configuration, dependency policy, and readiness beyond a listening socket. |
| Control Room | Serve named operator login over company-hostname HTTPS; call Control-Service over mTLS. | The existing schema-v2 inputs cover only this slice. The installer must validate installed mount access, company OIDC grants and first-admin recovery before reporting ready. |
| Enrollment | Own the device registry and credential lifecycle; expose only the approved device and administrative trust boundaries. | Complete lifecycle/bootstrap contract and proof that BAS or another process cannot independently write the production registry. No shared SQLite mount is permitted. |
| Operator identity and trusted attributes | Supply the bundled first-install operator login, a usable identity source and required actor authentication/attribute decisions. | Dex plus OpenLDAP are qualification candidates. Select/pin both production images, define identity/attribute mappings, readable initial directory data/configuration, initial administrator creation/recovery, TLS, durable state, and explicit fail-closed behavior. |
| Protected DNS | Provide ZPR-managed service DNS, with its adapter and narrowly authorized publisher integration. | Pin and qualify the DNS artifact, define service discovery/bootstrap ordering, zone/state ownership, publisher identity, and DNS readiness. It must not depend on ZPR DNS to start. |
| Gateway | Enforce configured egress rules for clients explicitly configured to use its HTTP proxy. | Final activation/rollback API, client routing/provisioning, identity, health, and tested host firewall/network rules. Draft-save is not activation. |
| Logging | Provide the bundled durable, access-controlled logging capability. | OpenObserve is the qualification candidate, not a distribution approval. Complete AGPL-3.0/license review; pin and qualify the image; specify retention/capacity and credentials; redact secrets; and verify ingestion/query health. |

The source-grounded current gaps for these roles are in the [system packaging
inventory](README.md#m1-source-audit-deployable-roles-and-gaps).

## Network and credential contract

- Services communicate on a private Compose network using stable service names.
  Compose start order alone is not readiness; a dependent service starts only
  after its required dependency passes its protocol-level readiness check.
- Only explicitly declared operator HTTPS, device-enrollment HTTPS, proxy, and
  any approved DNS client interfaces may be reachable from outside that private
  network. Exact interfaces, ports, source ranges, host firewall rules, and the
  node/adapter packet path must be in the release manifest/operator config.
  Backend, database, metrics, and mTLS service listeners are not published to
  the host by default.
- Every service-to-service TLS connection uses a named client identity and
  trust bundle with a documented audience. Credentials are files mounted
  read-only for the consuming service, not embedded in images, archives, logs,
  or environment-variable values.
- Internal service mTLS identities use the unique per-install CA selected for
  this profile; company-provided Control Room HTTPS certificates remain a
  separate trust boundary. The CA private key is protected persistent state,
  is not exported in logs or support bundles, and must survive retries and
  authorized restore. Key algorithms, CA access policy, issuance, rotation,
  compromise response, backup encryption, and restore-time identity checks
  remain security-design gates before implementation.
- Company-provided Control Room HTTPS certificates and OIDC configuration are
  validated before starting. Generated local service identities are created
  only by the installer under the separately approved identity-generation
  design; creation is atomic and first-install-only. Retries reuse existing
  identity and never overwrite it.
- No default account, public listener, wildcard management bind, plaintext
  backend access, or silent switch to a demo/external provider is allowed.

## Configuration and persistent state

The operator-edited company configuration contains provider choices and
references to protected files, never secret bytes. The current schema-v2
configuration and `zpr-system-config-render` output cover Control Room and its
Control-Service connection only. They are not the full runtime configuration.

The installer must validate all input and artifact references before any
mutation, render versioned per-service config, then mount each service's inputs
read-only at the rendered destination. Invalid, missing, unreadable, or
incompatible required inputs abort before service startup. The first profile
selects bundled Dex/OpenLDAP identity and OpenObserve logging; it does not
silently switch to an external service. Existing-provider modes require their
own explicitly validated contracts.

Each stateful component has one named writer and one separately managed durable
state directory/volume. At minimum, the release must enumerate owners and
backup/restore semantics for Visa/backend state, Enrollment registry, policy
records and activated policy, provider state, DNS zones/journal, Gateway
configuration, and logs. A service may not share another service's database
file or mutate another service's data directory. Installation, retry, upgrade,
restore, and removal preserve identities and data unless the operator explicitly
selects a documented destructive action.

## Install, readiness, and recovery protocol

1. Run host preflight and reject unsupported OS/architecture, remote Docker,
   insufficient approved capacity, or unavailable required interfaces.
2. Verify the exact archive, manifest, target platform and every declared
   payload before extraction or Docker loading. Production mode also verifies
   the signature using independently provisioned trust.
3. Validate company inputs and protected file access without starting services
   or modifying existing state.
4. Create only missing protected directories and first-install identities;
   refuse unsafe existing paths or identity/state conflicts. Record a
   recoverable transaction marker before the first mutation.
5. Render the complete service configuration and Compose definition into a
   private staging directory. Validate the rendered plan and bind mounts before
   applying it.
6. Start dependencies in explicit readiness order: persistent state and
   bundled identity/logging; Visa and node; policy/compiler; Control-Service
   and Enrollment; Control Room; protected DNS and Gateway after their trust,
   publisher, and client configuration is ready. The final ordering must be
   refined when the bootstrap API and service-health contracts are settled.
7. Run protocol-level readiness and acceptance checks. Success requires named
   operator login; a successful lookup against the configured, readable
   bootstrap identity source; one endpoint credential delivered through the
   supported Enrollment flow and successful node/endpoint admission; a loaded
   initial policy and observed allowed and denied traffic through the selected
   Gateway path; resolution of a Visa-published ZPR service name through the
   protected DNS path; and a real lifecycle event accepted, retained across
   restart, and returned by an authorized logging query. A running container,
   open port, or manifest capability declaration is not success.
8. Write a redacted install receipt with release ID, manifest digest, config
   schema, service states, and probe results. Never include secret contents.
9. On failure, stop only services started by this transaction and retain all
   pre-existing data and identities. Explain the failed baseline gate and
   recovery command; never silently reset, remove volumes, or report success.

Stop/restart must preserve state. Removal is non-destructive by default and
requires a separate explicit operator action to erase data. Upgrade and
rollback semantics must be versioned and tested; restoring a backup is
required when a schema/data migration is not downgrade-safe.

## Explicit-proxy Gateway behavior

For this profile, acceptance clients must explicitly use the configured proxy.
Endpoint enrollment/admission is verified separately; the first test does not
claim that endpoint adapter traffic is automatically sent through Gateway.
HTTP
requests are checked against the supported host/port allowlist and request
limits. HTTPS `CONNECT` is limited to the allowed destination host and port
443; encrypted paths, methods, headers, and bodies remain opaque and are not
policy-inspected. The first test therefore proves decisions only for clients
known to use this proxy. It does not claim transparent control of arbitrary
host traffic or HTTPS inspection. Unsupported hosts, addresses, ports, and
configurations fail closed. Gateway activation, client routing, rollback, and
host firewall setup still need their own implemented and tested contract.

## Blocking decisions before implementation can be treated as complete

1. Finish Enrollment lifecycle/credential-delivery ownership and retire
   independent production registry writers.
2. Supply a supervised production node and supported endpoint adapter path,
   including TUN/device and least-privilege requirements.
3. Select compatible immutable service images/source revisions, build recipes,
   compiler compatibility, state backend, DNS package, bundled identity images,
   and logging image/license.
4. Complete the approved per-install service-CA design: algorithms, key
   custody, rotation, compromise response, backup/restore, and first-admin
   recovery. The contract above does not authorize generating or using keys.
5. Specify service health probes, DNS/Gateway activation, host firewall rules,
   and the dependency/bootstrap order. The first Gateway test is explicitly
   proxy-client traffic only; automatic endpoint routing is out of scope until
   the adapter contract supports it.
6. Pin and qualify the selected Dex/OpenLDAP and OpenObserve candidates. OpenLDAP
   is not pinned; OpenObserve distribution is blocked on its license review.
7. Complete full per-service configuration rendering, installer transaction
   recovery, Compose rendering, and clean Ubuntu acceptance against exactly the
   verified bundle.

Until those gates close, this document defines the approved first-profile
boundary and implementation requirements only. It does not make a candidate
installable or qualify any artifact.
