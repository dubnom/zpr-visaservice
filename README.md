# visa service

ZPR visa service implementation (under active development).

For the B1 **evaluation-only**, Simulator-independent local operator/LDAP/logging
bootstrap, see [the one-command evaluation runbook](zpr-dashboard/packaging/evaluation/README.md).
This is not the complete production appliance or source-registration milestone.

On a state-preserving restart, retained node identities are not evidence of a
live node session. Policy-authorized peer bootstrap visas remain available until
the peer authenticates in the current Visa Service process, allowing remote
nodes to reconnect without clearing the organization database.
Live-session tracking follows the RPC handle lifetime, including soft reconnects.
A failed VSS connection also invalidates its associated session, so half-open
VSAPI sockets cannot suppress restart bootstrap. Failure of an older VSS worker
or dropping an older RPC handle cannot clear a newer session. Bootstrap requests
check session liveness even when both endpoint identities remain in the database.
Bootstrap visas do not reuse
retained ephemeral A2A keys: those keys must be learned again through authentication.
Bootstrap transport retains authenticated hop encryption and the VSAPI TLS session.
A fresh node reset revokes that node's previous non-bootstrap visas on the live relays before
issuing replacements; clearing only the restarting node's references would leave
relays matching obsolete VSS visas and ephemeral keys.
Key-free bootstrap copies remain on live relays to preserve the VSAPI connection
used to authenticate the restarting node; its own copies are queued for reinstall.
Cached bootstrap visas missing any relay's installation reference are replaced,
so a relay reset cannot strand a future peer with an undeliverable visa.
Nodes may also pull visas by ID while their own delivery state is
`PendingInstall`, not just `Installed`. This lets a relay bind a VSS flow before
the reverse VSS channel exists, rather than requiring that channel to deliver
its own prerequisite. Pulled copies are actualized for the requesting node;
off-path, expired, pending-revoke and revoked visas are not returned. A successful
reply uses the existing installation-state transition, which cannot override a
concurrent revocation. The installed-visa inventory remains installed-only.

Node RPC pings request a reconnect when the session's authenticated identity
has less than one minute remaining. The node's existing ping-failure/reconnect
path performs a fresh signed challenge with retained-state semantics. An idle,
healthy RPC socket must not keep expired node authentication alive while Visa
deliveries to that node fail. Expired and near-expiry sessions receive
`AuthError`; this does not extend credentials without reauthentication.

See [Features Added Since the Upstream Fork](FEATURES_SINCE_ZPR_ORG.md) for a
high-level overview of the operator, policy, trusted-service, DNS, observability,
and simulator capabilities added in this fork.

Policy test fixture generation uses explicit operator configuration and live
LDAP, without loading Simulator manifests or profiles. Simulator provisioning
exports its fictional metadata into the same production input contract. See the
[fixture configuration contract](zpr-dashboard/cmd/zpr-web-dashboard/README.md#independent-fixture-configuration);
this incremental input boundary does not complete the application/package split.

For engineering coordination, see the workspace
[component work/status index](../COMPONENT_WORK.md) and
[agent team charter](../AGENT_TEAM_STARTER.md). The
[source reorganization plan](REDIRECTORY.md#inventory-and-target-layout-2026-10-14)
records the current inventory, verified incremental extractions and remaining
milestone gates; the full application/package split is not complete.

See [NATTY: session-scoped address indirection](NATTY.md) for a proposed,
not-yet-implemented design for adapter-local address aliases and their session,
visa, and replay-protection requirements.

See the [system packaging and release plan](System%20release%20plan.md) for
planned clean-company installation milestones and the recurring qualification,
signing, publication, rollout and recovery process for real system releases.
The plan is not a claim that a production installer is currently available.

See the [Simulator as an ordinary ZPR client plan](SIMULATOR_AS_ZPR_CLIENT_PLAN.md)
for a proposed demo-first task list, milestones, visual acceptance tests,
multi-source/organization LDAP work and tracked pre-production shortcuts.

Trusted-source cache refresh (`DELETE /admin/services/{id}/cache`) returns 202
only after reconciliation is recorded in the state database. This acknowledges
acceptance, not completion or relay revocation delivery. Pending generations
survive Visa Service worker/process restarts and are retried every five seconds;
duplicate refreshes coalesce and repeat the existing convergent reconciliation.
`GET` on the same authenticated resource reports each source's pending/failed
state and requested/completed generations. Database write failures return 503;
operators must configure durable Redis persistence for the state database.
Control-Service watchers retain their previous durable LDAP checkpoint on rejected
acceptance and replay on restart. Watcher health reports `accepted_pending`
separately from source-stream health; use the Visa Service resource for definitive
completion/failure, not the watcher's last refresh timestamp.

## Crates / Packages / Libraries

Policy evaluations use a fresh Cap'n Proto reader traversal budget for each
request. Repeated evaluation of an unchanged policy cannot exhaust a shared
reader's lifetime budget; parsing errors remain explicit.

- `vs` - The ZPR Visa Service. Aka "v2vs" -- the "v2" used to differentiate it
  from the "v1", prototype visa service.
- `libeval` - The "evaluator" library used by the `vs` and `zpt`. Includes the
  code that compares a description of network traffic to policy to determine if
  a visa should be issued.
- `zpt` - ZPR Policy Tester is a command line tool for testing how `libeval`
  evaluates policy.
- `vs-admin` - CLI based administration client for the visa service. Exercises
  the HTTPS admin api of `vs`.
- `zpr-dashboard` - CLI based "GUI" dashboard for monitoring visa
  service activity.
- `admin-api-types` - Library crate for data structures used by `vs` and
  `vs-admin`.
- `integration-test` - Shell-based integration tests. Includes evaluation tests
  of `libeval` using `zpt`.
- `tools` - Helper scripts, including `zpr-pki` for PKI operations.

Most of the visa service code depends on the
[zpr-common](https://github.com/org-zpr/zpr-common.git) repository, which
defines data structures used in the NODE-VS API and the policy binary format.
This dependency is pulled automatically via git in `Cargo.toml`, so no manual 
setup is required.


## Prerequisites

- **Rust** - Edition 2024. Install via [rustup](https://rustup.rs/).
- **Make** - The build is driven by a root Cargo workspace.
- **OpenSSL** - Via the `openssl` crate.
- **Redis/Valkey** - Required at runtime by `vs`.


## To build

Run `make` or `cargo build` in this top-level directory.

Run `make test` or `cargo test` in this top-level directory to run all
the unit tests.


## Release build (prototype vs only)

Run `make release` to produce a release tarball of the visa service tools. This
builds everything, then packages the `vs`, `vs-admin`, and `zpt` binaries into
`build-release/`.


## Admin HTTPS API

The visa service (`vs`) exposes an HTTPS admin API on port 8182 by default.
The `vs-admin` command line tool consumes this API.

See [admin-http-api.txt](admin-http-api.txt) for full endpoint documentation.

## ZPR DNS service

The BIND 9 deployment profile runs DNS on a ZPR service address, uses ZPL to
authorize authenticated query clients, and accepts service-record updates from
the Visa Service only through TSIG over ZPR. BIND is not exposed as a public or
underlay DNS service. See the [BIND 9 ZPR DNS guide](dns/bind9/README.md).

## ZPR observability service

The [OpenObserve deployment profile](observability/openobserve/README.md)
packages a third-party open-source log and metrics server behind a dedicated
ZPR adapter. It specifies OTLP/HTTP ingestion, authenticated reader access,
and the required Visa Service exporter interfaces. The profile does not
provision an adapter, install policy, or export existing metrics automatically.


## Multiple trusted sources and policy limitations

Multiple trusted-service attributes are now exposed as source-qualified policy
keys (`attribute__source__source-id`); multi-source authoring rejects mapped
unqualified references and runtime ingestion namespaces claims before evaluation.
Single-source policies retain unqualified aliases. This does not yet provide
approved cross-source subject links, dependency-aware outage handling, identity
version invalidation, or selective per-grant revalidation. See the
[evidence trace and seven-scenario support matrix](MULTISOURCE_POLICY_SUPPORT.md)
before relying on those guarantees.
