# Local evaluation bootstrap (B1 only)

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
entropy. There are **no shared/default passwords**. LDAP starts with an empty
People/Groups tree and one read-only service account; no synthetic users or
Simulator directory are copied into it.

Open `https://localhost:18787`. Import **only this install's `ca.pem`** into the
evaluation browser's trust store. Never disable certificate or hostname checks.
The username is `admin@evaluation.invalid`; obtain its password locally from
`operator-admin.secret` using your protected password workflow, not a shared
terminal/log. Named-user login is served by the install's Dex. Bootstrap verifies
an actual signed ID token from that issuer before writing `operator-oidc.json`;
it does not invent an OIDC subject from an email. Grants are limited to
`monitor.read` and `policy.read` for `evaluation-<install-id>`.

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
checks dependent services while running. Readiness means **B1 transport/auth
readiness only**, not a functioning upstream Visa/Policy installation or source
registration/ingestion. Missing upstreams remain visibly unavailable. No service
uses a Docker socket, simulation manifest, profile, session or workload.

Keep this command in the foreground. **Ctrl-C** stops its native operator
processes and removes only the random install's containers/network, retaining
LDAP, Dex and logging state. Rerun the **same command and flags** to restart.
A per-install lifecycle lock prevents duplicate supervisors. Images/config/
secret checksums are frozen except for the explicit stopped-host admin reset below; missing, changed, symlinked or partial state fails
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

## Local bundled-login admin recovery (evaluation only)

Run on the **same evaluation host**, as the OS account that owns the protected
install directory. This is not a remote HTTP endpoint, Control Room button,
production break-glass mechanism, or a reset for company identity providers.
Stop the foreground bootstrap first and verify its containers/processes stopped.
Make a **complete offline backup** of the private state before resetting; keep
that backup private and outside the install directory. Do not use a recorded or
shared terminal session.

For a single terminal disclosure (redirection/pipes are refused):

```sh
sh packaging/evaluation/bootstrap.sh -state "$PWD/.zpr-evaluation-local" -reset-admin
```

Alternatively, deliver only to a **new** `0600` file under a real, owned `0700`
directory outside the install state (existing files/symlinks are never replaced):

```sh
mkdir -m 700 .zpr-evaluation-recovery-delivery
sh packaging/evaluation/bootstrap.sh -state "$PWD/.zpr-evaluation-local" \
  -reset-admin -password-file "$PWD/.zpr-evaluation-recovery-delivery/admin-once.secret"
```

Use a protected password manager/workflow to collect this credential. No password
is accepted as an argument or environment variable. Never `cat` it into a log,
commit it, or pass it to another CLI as an argument. Securely dispose of the
delivery file after handoff; filesystem deletion is not a secure-erasure promise.
The normal private `operator-admin.secret` remains necessary for bootstrap's
signed-token health check; “once” refers to **command disclosure**, not removal
of bootstrap's protected credential storage.

The reset validates ownership, canonical non-symlink paths, private top-level
state/config/audit/lock permissions, checksums, and no hard links, special files
or group/other-writable state. Private top-level provider directories may contain
ordinary upstream-created readable files; their `0700` ancestor still prevents
other-user traversal. It acquires the **same exclusive lifecycle lock** as normal
startup and reserves all install ports, refusing a running/orphaned stack rather
than resetting files beneath services. It uses the stored install configuration,
not default ports/image flags, and never creates a missing install or contacts
Docker, Simulator or a network service.

A fresh 256-bit random password replaces the bundled Dex static credential
through Dex's supported static configuration (not a database edit). The OIDC
client secret and administrator subject also rotate; the install ID, certificates,
read-only permissions, LDAP, logging and Dex provider data remain unchanged.
Old operator grant configuration is removed. Restart with the **original launch
command/flags**: Dex loads the new static password/client configuration and
bootstrap verifies the new signed subject before creating its grants. Native
Control Room sessions are in memory and were discarded when its process stopped;
old subjects are not granted access after restart. This is intentionally a new
subject for the same local evaluation administrator, not identity merging.

`admin-recovery.audit.jsonl` records started/committed events, UTC time, install ID,
local UID, account, evaluation scope and capability gaps, **never the credential,
client secret or password hash**. File replacements and the inventory are synced;
`admin-recovery.pending` makes interrupted/failed multi-file changes fail closed.
There is no crash-atomic multi-file transaction or automatic rollback: if that
marker remains, preserve the private non-secret audit, keep the stack stopped,
restore the **entire** offline backup (not individual hashes), and perform a fresh
reset before starting. Restoring a backup also restores its old credential; guard
backup access accordingly. Do not remove the marker to bypass validation. A
successful reset with failed terminal/file delivery can simply be reset again
while stopped, with a new delivery file; the previous recovery password is then
invalidated. No command reprints a previously delivered secret.

**Capability gaps (active X12):** Dex static-password login has no forced-change,
expiry or enforced single-use interface. The disclosed recovery password remains
valid until another local reset; it is **not an enforced one-time login**. Replace
it through another local reset after secure handoff. Rotating the confidential
client secret blocks use of that previous client credential, but does not erase
Dex's existing token records or cryptographically revoke already-issued signed
JWTs; such JWTs retain their upstream expiry. Old subjects have no new Control
Room grants, and the old Control Room cookies do not survive the required process
restart. OpenObserve's separate password/sessions are unaffected. Full upstream
token/session revocation, forced change, expiry/replay enforcement and independent
logging-admin recovery need a qualified credential-management interface before
production use; none is claimed here.

## Decisions and deliberately unfinished work

- **B1 only:** named-operator bootstrap, persistent starter directory, persistent
  logging backend and independent operator runtime. No trusted-source UI or
  registration code is changed. B2 registers the starter LDAP; B3 configures
  collection/ingestion, a named logging source and the authorized operator query
  path. Merely starting OpenObserve does not mean any application logs are ingested.
- **Bundled login recovery:** the local, stopped-host command below rotates only
  the evaluation Dex operator. X12 records the deliberately unfinished
  forced-change, expiry, single-use and upstream-token revocation guarantees.
  OpenObserve's independent administrator is not recovered by this command;
  its supported credential/session recovery remains unfinished. Never delete
  provider state or change `openobserve.env` to claim a logging password reset.
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
  production contracts when B2/B5 are available. There is intentionally no
  Simulator flag, auto-start, endpoint or Control Room dependency in this launcher.
- Certificates expire after 90 days (CA: one year); rotation/upgrade, installer
  signing, full BOM/platform qualification, HA, encrypted backup, production
  resource sizing and a browser visual-acceptance matrix are not B1 claims.

Active shortcut ownership/removal gates are recorded as **X01/X02/X06/X08/X11/X12**
in the [plan's shortcut register](../../../SIMULATOR_AS_ZPR_CLIENT_PLAN.md#7-demo-shortcuts-and-pre-production-register).

## Focused validation

```sh
mkdir -p .evaluation-build
TMPDIR="$PWD/.evaluation-build" go test -race ./cmd/zpr-lab-bootstrap ./internal/operatorauth
TMPDIR="$PWD/.evaluation-build" go vet ./cmd/zpr-lab-bootstrap
shellcheck packaging/evaluation/bootstrap.sh
python3 -m py_compile packaging/evaluation/test-live.py
python3 scripts/check-control-room-boundary.py
```

Optional, **disposable local containers only**, random project/ports and guaranteed
cleanup attempts; validates two starts, retained identity/data, TLS/mTLS,
admin-only logging and invalid inherited Simulator configuration:

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

### Admin recovery evidence (2026-10-23)

Focused recovery tests cover 256-bit generation, old-password rejection after
rotation, distinct subsequent recovery credentials, subject/client rotation,
private file-only delivery and redirected-output refusal, non-secret auditing,
provider-data retention, unsafe permissions/links/tampering, missing-state
refusal, lifecycle concurrency/orphan-port refusal and interruption fail-closed
behavior. These are local configuration/unit/race checks, not a live bundled-IdP
or browser recovery qualification. Unsupported forced-change/expiry/single-use
and issued-token revocation remain X12 gaps, not passed acceptance cases.

Validation for this change: `go test -race ./cmd/zpr-lab-bootstrap
./internal/operatorauth`, targeted `go vet`, gofmt/diff checks, wrapper shellcheck,
the strict boundary audit and its five mutation checks passed. `make test` was
attempted: existing unrelated Simulator organization/launcher tests, missing
Northstar/Rust dependency fixtures and macOS enrollment/reset protected-path
checks failed; no existing failing tests were altered. No live stack was
started or deployed for this recovery change.

Recovery test fixtures resolve the OS temporary parent through `EvalSymlinks`
and explicitly protect it with `0700`; production symlink rejection is unchanged.
Both `go test -race ./cmd/zpr-lab-bootstrap -run Recovery -count=1` and the full
bootstrap package race suite pass with the default macOS `TMPDIR`. The partial
replacement test proves Dex configuration changed and the pending marker blocks
startup, rather than confusing an earlier path-validation refusal with a reset.
