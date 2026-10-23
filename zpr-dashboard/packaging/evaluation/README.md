# Local evaluation bootstrap (B1–B3)

**EVALUATION ONLY — NOT PRODUCTION SUPPORTED.** This is a foreground,
single-host engineering evaluation launcher, not the production appliance BOM,
a complete Visa Service installation, or a Simulator launcher.

From `zpr-dashboard`, one command renders protected configuration, builds the
current production Control Room/Control-Service package, starts the packaged
starter LDAP, OpenObserve and named-operator Dex services, and waits for actual
protocol readiness:

```sh
sh packaging/evaluation/bootstrap.sh -state "$PWD/.zpr-evaluation-local"
```

To inspect generated files **without Docker, runtime-package builds, protocol probes or service
startup**, add `-render-only` to that command. File generation itself uses Go;
the wrapper sets compiler scratch to `.evaluation-build` inside this workspace.
The wrapper may compile the generator itself; preload Go dependencies for an
offline host. A render success explicitly does **not** claim running-service readiness.

## Host and current package requirements

- A local, trusted, single-administrator macOS/Linux workstation, the module's
  supported Go toolchain/dependencies, and Docker with Compose v2. Do not point
  Docker at a remote daemon: native operator services and bind-mounted private
  container state belong to the same host. On macOS, Docker file sharing must
  include the chosen state path.
- Load the following **current packaged immutable images** before starting.
  The launcher never pulls images, and never enumerates or modifies other stacks.
  These are engineering image identities, **not** qualified release pins:

  | Service | Current packaged image | Platform |
  | --- | --- | --- |
  | Starter directory | `osixia/openldap@sha256:0de3850635911109f0daf8f5957808ec4f4eb827fbaaa339100b5f1bab925745` | `linux/amd64` |
  | Logging | `public.ecr.aws/zinclabs/openobserve@sha256:d4a878fac1f6c56003764f7f2a1625668917388f167e222c8c810de3f54c56ba` | `linux/arm64` |
  | Operator login | `ghcr.io/dexidp/dex@sha256:5d0656fce7d453c0e3b2706abf40c0d0ce5b371fb0b73b3cf714d05f35fa5f86` | `linux/arm64` |

  The current mixed-architecture packages need Docker's corresponding emulation
  support (including amd64 OpenLDAP on arm64). A clean host without these images
  gets an actionable preflight failure, not an implicit download or fallback.
  `-ldap-image`, `-logging-image`/`-logging-platform` and
  `-oidc-image`/`-oidc-platform` accept explicit matching immutable replacements;
  a different package/platform is a separate acceptance exercise.
- Use an existing parent directory that is not group/other writable. The new
  state directory is `0700`; configuration, secrets, CA keys and logs are `0600`.
  Do not place state in a synchronized/shared folder or commit it. The documented
  `.zpr-evaluation*` paths are ignored; custom state paths need equivalent exclusion.
- Default **IPv4 loopback-only** ports: Control Room `18787`, Control-Service
  `18790`, LDAPS `11636`, OpenObserve `15080`, Dex `15556`. All must be free.
  Set distinct `-room-port`, `-service-port`, `-ldap-port`, `-logging-port`,
  `-oidc-port` values for parallel installations. Never expose these services
  to a LAN, add a reverse proxy, or publish the private state directory.

## Identity, health and lifecycle

Every install receives a random install/Compose-project ID, its own CA and
service/client certificates, and independent 256-bit random LDAP admin/reader,
OpenObserve admin, operator and OIDC client secrets. A fixed complexity suffix
satisfies the packaged OpenObserve password policy without reducing random
entropy. There are **no shared/default passwords**. LDAP starts with an empty People/Groups tree and one read-only service
account. No synthetic people or Simulator data are copied. A verified empty LDAP
search is valid for connection tests and selected-source browsing, but remains
an error for assertion evaluation (no vacuous policy success).

Open `https://localhost:18787`. Import **only this install's `ca.pem`** into the
evaluation browser's trust store. Never disable certificate or hostname checks.
The username is `admin@evaluation.invalid`; obtain its password locally from
`operator-admin.secret` using your protected password workflow, not a shared
terminal/log. Named-user login is served by the install's Dex. Bootstrap verifies
an actual signed ID token from that issuer before writing `operator-oidc.json`;
it does not invent an OIDC subject from an email. Grants are limited to
`monitor.read`, `logs.read`, `policy.read`, `policy.edit` and `policy.analyze`
for `evaluation-<install-id>`. The last two allow ordinary source registration
and read-only connection tests; they do not provision Policy-Service.

OpenObserve is at `https://localhost:15080`, with the same username and a
**different** password in `logging-admin.secret`. It has one administrator only;
unauthenticated query access is checked to fail. OpenObserve's **AGPL-3.0 license
is accepted for evaluation use**, not production qualification. Global retention
is explicitly **7 days**. This is asynchronous backend retention, not guaranteed
exact-time secure erasure or a disk quota; do not use real company data or modify
retention/users through the upstream admin console. Each container has a 1 GiB
memory/2 CPU cap; data-disk capacity monitoring remains an operator responsibility.

`WAIT`/`HEALTHY` output covers a verified LDAPS reader bind/base search,
OpenObserve TLS plus authenticated query/unauthenticated denial, verified OIDC
identity, Control-Service mTLS route and Control Room HTTPS. The supervisor also
checks dependent services and sends bounded lifecycle heartbeats while running.
Readiness includes **ordinary LDAP registration/read and acknowledged, queryable
lifecycle log ingestion**, not a functioning upstream Visa/Policy installation. Missing upstreams remain visibly unavailable. No service
uses a Docker socket, simulation manifest, profile, session or workload.

Keep this command in the foreground. **Ctrl-C** stops its native operator
processes and removes only the random install's containers/network, retaining
LDAP, Dex and logging state. Rerun the **same command and flags** to restart.
A per-install lifecycle lock prevents duplicate supervisors. Images/config/
secret checksums are frozen; missing, changed, symlinked or partial state fails
closed rather than reseeding or silently switching credentials. A failed start
cleans up its own runtime and retains state for inspection. If cleanup is
interrupted, run `docker compose -p zpr-evaluation-<install-id> -f compose.json down`
**from that install's state directory** (the exact command is printed on cleanup
failure; never inherit a different `COMPOSE_PROJECT_NAME`), not a shared stack directory. Back up the entire private
state directory offline while stopped; protect the CA private key.

Do not print `docker compose config`/`docker inspect` environments: OpenObserve's
upstream API takes root credentials through its environment. Upstream container
logging is disabled to avoid bootstrap-credential disclosure; native production
service diagnostics are private under `logs/` and are never echoed by bootstrap.
Process failures report the component and next action, not request bodies,
credentials or backend responses. Browser/OS trust removal and secure disposal
of the private directory are explicit operator actions after stopping.

## Ordinary source/log configuration (B2/B3)

Bootstrap uses mTLS to call `GET /api/assertions/sources`, the public read-only
connection test, `POST /api/assertions/sources` on first registration, and an
explicit selected-source read. Organization `evaluation-<install-id>` and source
`directory_<install-id>` remain stable. The source has no reserved name or
production evaluation code path: its normal `ldap` configuration uses verified
LDAPS, `ca.pem`, and a protected `ldap-reader.secret` file reference. The immutable
`source-registration.json` records the initial request template, not a password.
`ZPR_TRUSTED_SOURCE_ORGANIZATION_ID` explicitly scopes source administration/read
independently of unavailable Policy-Service; policy evaluation/settings still
fail unavailable. Conflicting Policy organization context fails closed.

The mutable **0600** `assertion-sources.json` remains the ordinary, single-writer
production source registry. Add company sources through the same API/UI. Restart
retests/reads the existing starter entry without replacing operator edits or
switching the existing default, merging identities or choosing a fallback.
Missing/invalid registries fail rather than reseed. An old B1-only inventory is
not silently migrated; use a new private state directory for this version.

The mutable **0600** `adapter-logs.json` is the existing production log inventory
(`ZPR_ADAPTER_LOG_CONFIG_FILE`), now accepting HTTPS OpenObserve sources alongside
file/container/company log sources. Target `logs_<install-id>` and stream
`lifecycle_<install-id>` carry explicit evaluation identity; company targets and
streams remain separate. Network source fields are `url` (HTTPS origin),
`ca_file`, `username`, `password_file` (absolute protected file reference),
`organization` (backend tenant) and `stream` (identifier, not SQL). TLS verification
is mandatory, redirects are rejected, backend errors are redacted, and queries
are bounded to 100 entries from the past 7 days. Inventory edits are picked up on
subsequent queries/collection; edit only while the supervisor is stopped.

Only `bootstrap_ready`/`bootstrap_heartbeat` events with timestamp, install/source/
organization IDs and an EVALUATION ONLY label are collected. Startup waits for
acknowledged ingestion and a query containing this install's ID. A failed source
has explicit unavailable/error state, never a company-source fallback. Storage
is persistent; backend global retention remains 7 days. There is no arbitrary
raw application-log collector or guaranteed-lossless delivery/outbox.

Control Room sets `ZPR_CONTROL_ROOM_LOG_PERMISSION=logs.read`; only the named
administrator receives that independent log permission. An ordinary monitor
without it cannot query logs. Other operator installations preserve their existing
`monitor.read` behavior unless they explicitly set this production access policy;
set it and grant only intended administrators when adding sensitive log sinks.
Direct Control-Service clients still require the protected administrator mTLS
identity; never distribute its private key. OpenObserve's independent admin
credential is never sent to browsers or returned through source APIs.

### B2/B3 verification matrix

| Gate | Automated evidence / remaining acceptance |
| --- | --- |
| V02/V06 | Explicit production organization with Policy/Simulator unavailable; public registration/test/read, revision and cross-organization guards; durable registry/restart; selected-source provenance. Existing registry race tests plus `TestExplicitProductionSourceScopeWithoutPolicyOrSimulator` and bootstrap contract/render tests. |
| B2 isolation/coexistence | Real two-cycle disposable bootstrap; company/starter fixture coexistence; existing default preserved by ordinary registry contract; no source-name branch in production. |
| B3 access/identity | `TestIndependentLogPermissionAdminOnly`, OpenObserve TLS/credential/unsafe-config tests, and `TestOpenObserveLogRenderingCoexistsAndFailsExplicitly` verify admin boundary, visible evaluation identity, company coexistence and independent failure. |
| B3 ingestion/persistence | Disposable smoke verifies queryable lifecycle ingestion, verified TLS/mTLS, no unauthenticated access, restart with retained identity/data and no secret output. 7-day retention is backend configuration, not a seven-day elapsed-time test. |
| Visual/platform release | Browser screenshots, multiple-company-backend live acceptance, clean-host/platform packaging and extended retention/delivery qualification remain separate. |

## Decisions and deliberately unfinished work

- **B1–B3:** named-operator bootstrap, ordinary scoped source registration/read,
  persistent logging with lifecycle ingestion and authorized Control Room queries.
  Arbitrary application/stdout collection is deliberately not enabled: upstream
  output may contain credentials. Only allowlisted launcher metadata is ingested.
- **Admin recovery design/task (not implemented):** a local CLI reset command
  must prove local state ownership and acquire this lifecycle lock, generate a
  unique one-time password into a new `0600` file (never stdout/arguments/logs),
  update the IdP's supported credential interface atomically, invalidate existing
  sessions, and require password replacement on the next successful login.
  It must cover OpenObserve's independent administrator and preserve provider
  data, with expiry/replay/rollback tests. Dex's static password mode has no
  forced-change/one-time guarantee; editing a hash/file is **not** an acceptable
  recovery implementation. Do not delete persistent state as a password reset.
- **B4:** Control-Service owns durable **per-source** change-watcher checkpoints,
  source IDs, resync/failure state and private metadata retention, independently
  of Simulator. B1 does not start syncrepl/accesslog watchers or claim cache
  refresh or visa revocation.
- **B6 explicit retirement wizard:** link company source → rebind policies →
  verify each source/policy/identity binding and logging path → explicitly disable
  the bundled service. Preview affected state, authorize confirmation and offer
  recovery; never merge identities, change defaults or fall back automatically.
- **Optional add-later hook (B5):** install the existing separate Simulator package
  only after this stack is healthy. Give it a different directory, credentials,
  ports and lifecycle, and explicitly register its synthetic source through
  production contracts (B5 lifecycle acceptance is still separate). There is intentionally no
  Simulator flag, auto-start, endpoint or Control Room dependency in this launcher.
- Certificates expire after 90 days (CA: one year); rotation/upgrade, installer
  signing, full BOM/platform qualification, HA, encrypted backup, production
  resource sizing and a browser visual-acceptance matrix are not B1 claims.

Active shortcut ownership/removal gates are recorded as **X01/X02/X06/X08/X11**
in the [plan's shortcut register](../../../SIMULATOR_AS_ZPR_CLIENT_PLAN.md#7-demo-shortcuts-and-pre-production-register).

## Focused validation

```sh
mkdir -p .evaluation-build
TMPDIR="$PWD/.evaluation-build" go test ./cmd/zpr-lab-bootstrap
TMPDIR="$PWD/.evaluation-build" go vet ./cmd/zpr-lab-bootstrap
shellcheck packaging/evaluation/bootstrap.sh
python3 -m py_compile packaging/evaluation/test-live.py
python3 scripts/check-control-room-boundary.py
```

Optional, **disposable local containers only**, random project/ports and guaranteed
cleanup attempts; validates two starts, retained identity/data, TLS/mTLS,
ordinary source registration/read, lifecycle ingestion/query, admin-only logging
and invalid inherited Simulator configuration:

```sh
python3 packaging/evaluation/test-live.py --run-disposable
```

This does not mutate any existing/company LDAP directory or start Simulator.

### Engineering evidence (2026-10-23)

Focused race/unit tests, `go vet`, shellcheck, Python syntax checks and the strict
Control Room boundary audit passed. The opt-in disposable test passed two starts
on the current macOS/arm64 engineering host using the exact mixed-platform image
set above: identity/config and LDAP/logging marker persistence, actual protocol
health, named-operator issuer/subject verification, TLS/mTLS negative checks,
admin-only logging, no secret output and complete container cleanup.

The broader `make test` was also attempted and failed outside this change:
missing Northstar/package fixtures and external Rust workspace dependencies,
existing simulator launcher/observability expectations, and macOS long Unix-socket
or protected-path checks under this worktree. It is **not** a full-suite green or
clean-host/browser/production qualification claim. No failing existing tests were
changed to obtain the focused result.
