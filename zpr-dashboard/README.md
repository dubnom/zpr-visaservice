# ZPR Dashboard TUI Application

This repository contains the Zero-Trust Packet Routing (ZPR) Terminal User Interface application. The dashboard application allows you to manage your ZPR infrastructure quickly through an easy-to-understand and easy-to-use interface.

# Development

To setup your development environment, run `go mod download` to install the local dependencies. Finally, run `make run` to start the TUI.

## Tests

`make test` runs the Go unit and component suites. Browser regressions run in
the pinned Playwright Docker image; Docker is the only additional local
prerequisite. Node/npm and Chromium stay inside the container:

```sh
make test-browser
make test-all
```

When changing browser dependencies, regenerate the lockfile and embedded
browser assets with `make vendor-browser-assets`. This runs npm in the
multi-architecture Node container. CI runs both the Go and browser suites;
failed browser tests retain screenshots and traces as artifacts.

The browser suite runs regressions on desktop and iPad-like tablet Chromium inside
the container, matching the supported web-GUI device scope. It serves the real
static assets with the dashboard's CSP and fixture APIs, without live services,
certificates, or private credentials. Coverage includes
radio log selection, per-source scroll/follow memory, errors and polling lifecycle,
ANSI/HTML safety, policy completions, map/table colors and gateway clouds, visa
refresh/DNS labels, LDAP popups, attributes, memberships, branch collapse, and
explicit organization-switch approval/cancellation and stale-approval handling.
These are deterministic UI regressions, not live network or multi-node acceptance
tests; unfinished protocol, teardown, and recovery work remains separate.

## Project layout

```
config.toml.example  Endpoint, timeout and credential file paths
cmd/zpr-dashboard/   Program entry point (thin main)
internal/
  app/               Bubble Tea model: Init/Update/View, input handling
  pages/             Top-level tab views (Dashboard, Visas, Actors)
  components/        Reusable panels, tables, charts and their data
  charts/            Braille dot chart and bar chart primitives
  styles/            Color palette and shared lipgloss styles
  config/            Reads config.toml into the environment
  dataplane/         HTTP client for the vs-admin API
  enrollment/        SQLite registry, admin API, and device HTTPS service
```

## Device enrollment foundation

`internal/enrollment` implements the first storage slice of production device
provisioning without Simulator dependencies or additional module dependencies.
It persists organization-scoped asset invitations, hashed enrollment codes,
expiry/cancellation, single-key claims, and transactional audit events.
The database is owner-readable/writable only. The caller supplies the approved
invitation expiration; this library does not select a deployment lifetime.

This package is not a device authentication or credential-issuance service.
The device service now verifies fresh proof of key possession and atomically
claims an invitation. Durable approval/rejection is implemented;
authoritative inventory integration,
issuance, device-facing rate limits, and the signed Debian/Ubuntu installer
remain pending. Named-user Control Room authorization and opt-in invitation
creation are described below.

### Certificate-authorized invitation administration

Control-Service optionally serves `/api/enrollment/v1/` on its existing private
TLS listener. No additional listener or public device enrollment endpoint is
opened. With both settings absent, these routes return `503`; supplying only
one setting or an invalid configuration prevents service startup:

```sh
ZPR_ENROLLMENT_CONFIG_FILE=/etc/zpr/enrollment-admin.json
ZPR_ENROLLMENT_DATABASE_FILE=/var/lib/zpr/enrollment.sqlite
```

The configuration must explicitly define the invitation lifetime, approved
organization/profile/type catalogs, and administrator certificate permissions.
Review permissions additionally require an explicit approval lifetime.
For example (both lifetimes are examples, not defaults):

```json
{
  "version": 1,
  "invitation_lifetime_seconds": 3600,
  "approval_lifetime_seconds": 3600,
  "organizations": {
    "example-company": {
      "profiles": ["managed-linux"],
      "types": ["laptop", "desktop", "server"]
    }
  },
  "principals": [
    {
      "name": "enrollment-operator",
      "certificate_sha256": "REPLACE_WITH_LOWERCASE_SHA256_OF_CLIENT_CERTIFICATE_DER",
      "organizations": ["example-company"],
      "permissions": ["read", "create", "cancel", "approve", "reject"]
    }
  ]
}
```

Use a dedicated administrator client certificate issued by the configured
Control-Service client CA. Its verified leaf certificate must also match the
explicit SHA-256 pin; a matching CN or request header is not sufficient.
Readers can be assigned only `read`. Unlisted certificates, organizations,
and permissions fail closed. Do not authorize the shared Control Room service
certificate as an administrator. Control Room proxy requests to enrollment
are blocked unless HTTPS named-user authorization and independently verified
backend delegation are explicitly configured.

Obtain the pin from the **public** administrator certificate:

```sh
openssl x509 -in admin-client.crt -outform DER | openssl dgst -sha256
```

Configuration is validated and loaded at startup; restart Control-Service
after changing grants, removing a certificate pin, or rotating a certificate.
Both configured lifetimes must be between 1 second and 30 days (a validation
bound, not a recommended duration). Existing invitation-only configurations
may omit `approval_lifetime_seconds` if they grant neither `approve` nor
`reject`. The pending asset
owner, recipient, and inventory reference are administrative claims, not
verified device attributes or granted policy roles.

| Method | Path | Permission | Input/result |
| --- | --- | --- | --- |
| GET | `/api/enrollment/v1/catalog` | read | Scoped catalogs/lifetimes; general GUI mutations disabled; separate opt-in creator organization capability on delegated route |
| POST | `/api/enrollment/v1/invitations` | create | JSON asset; returns invitation and one-time-visible enrollment code |
| GET | `/api/enrollment/v1/invitations?organization=...` | read | Paginated invitations, no codes or hashes |
| GET | `/api/enrollment/v1/invitations/{id}?organization=...` | read | Invitation detail, no code or hash |
| POST | `/api/enrollment/v1/invitations/{id}/cancel` | cancel | JSON `{"organization":"..."}`; cancels the invitation |
| POST | `/api/enrollment/v1/invitations/{id}/approve` | approve | Organization, reviewed revision, key fingerprint, and verification reason |
| POST | `/api/enrollment/v1/invitations/{id}/reject` | reject | Organization, reviewed revision, key fingerprint, and rejection reason |

Creation accepts `organization`, `asset_id`, `name`, `owner`, `type`, `profile`,
and `recipient`. All are required bounded strings; type/profile must be in the
configured catalog. The backend derives the audited principal from the verified
certificate and the expiration from server configuration. Creating an asset
does not create an adapter, issue a credential, or authorize network access.

JSON mutation bodies are limited to 8192 bytes and reject unknown fields,
trailing JSON values, non-JSON content types, and query parameters. Responses
use `Cache-Control: no-store`. Browser-origin requests are rejected, even if
they present an authorized certificate. Use the APIs directly with an mTLS
client, not through the browser/shared-service proxy.

Listing defaults to 50 invitations; `limit` is bounded to 1–100. Pass the
returned `next_after` as `after` to continue in stable invitation-ID order.
Cancellation is idempotent. Duplicate active asset invitations return `409`.
If a create response is lost, inspect the asset's invitation and cancel/replace
it; codes cannot be retrieved afterward. Delivery retries/idempotency for
creation are not yet implemented. Transfer the code through the approved
separate secure channel, never an email, URL, command argument, or log.

### Reviewing a claimed device

The device proof-of-possession service sets an explicit approval deadline
from its deployment configuration. The claim records `claimed_at`, `approval_expires_at`, and the
verified key fingerprint, and increments `revision`. Same-key retries preserve
that deadline and revision; they cannot keep a request alive indefinitely.
At the deadline, the request reports `approval_expired` and cannot be approved
or resumed. A fresh invitation is required. Legacy pending records without a
deadline fail closed rather than acquiring an unlimited approval window.

Read the invitation before review, verify the asset and fingerprint through
the trusted procedure, then submit:

```json
{
  "organization": "example-company",
  "revision": 2,
  "key_fingerprint": "REPLACE_WITH_THE_REVIEWED_DEVICE_KEY_SHA256",
  "reason": "Asset and device verification code checked through trusted channel"
}
```

Reasons are required and bounded to 256 bytes; never include enrollment secrets
or unnecessary personal information. A missing/mismatched fingerprint is rejected.
Stale revisions and competing decisions return `409`; reload and inspect the
record instead of blindly retrying. Approval also rechecks the current approved
profile/type catalog. Rejection remains available if a profile was removed.
Decision principal, reason, timestamp, and revision are persisted atomically
with the state and audit event. Approved and rejected decisions cannot be
reopened by these endpoints.

An `approved` state means **approved for future credential delivery**, not
enrolled, issued, or connected. Approved assets stay reserved against duplicate
invitations. Cancellation is not revocation and cannot cancel an approved
record; future issuance/revocation services must handle that lifecycle.
Rejected, cancelled, or expired requests may be replaced by a fresh invitation.
Expiration is evaluated on reads; replacing an expired invitation materializes
and audits its expired state transactionally.

These administrative APIs do not provide a way to manufacture a claimed request.
The separate device protocol below verifies key possession first. Neither its
listener nor the installer has been deployed, and the GUI remains blocked.

### Device challenge and proof protocol

The proposed production ownership split and versioned lifecycle APIs are
documented in the [Enrollment Service API contract](ENROLLMENT_SERVICE_CONTRACT.md).
That contract is not implemented; details below describe the current prototype.

`NewDeviceService` and `NewDeviceHandler` implement the device-facing protocol
independently of the administration handler. They are **not registered on
Control-Service**. The opt-in standalone `zpr-enrollment-service` command
serves them on a separate TLS listener.
The selected first-release boundary is private corporate underlay/VPN access
only; direct public-Internet exposure is not approved. Keep the device listener
separate from Control-Service, Policy-Service, Visa Service Admin, and operator
APIs. The handler requires end-to-end TLS, so a private TCP/L4 gateway can
forward the TLS connection; TLS termination and trusted-proxy identity handling
are not implemented. Do not expose the administrative listener to devices. No
service is automatically started by the dashboard stack or this change.

The invitation's asset fields are currently a snapshot, not an authoritative
inventory. The first release will use a separate Enrollment Service-owned,
organization-scoped asset registry; external ITAM/MDM integration is out of
scope. In production, the Enrollment Service must be the sole writer for
inventory and device identity lifecycle; Control-Service will use an
independently authenticated administrative API and must not share its database.
The current local prototype instead mounts the administrative handler in
Control-Service and uses the enrollment package/SQLite registry for device
proofs. The production ownership split, inventory API/storage,
revision-bound invitation contract, credential issuer, trusted-attribute
publication/revocation, and private network deployment have not been
implemented. Do not treat this protocol as production device admission.
For the initial single-operator deployment, the invitation creator may also
approve the request. The named administrator's permission, revision/key binding,
and audit checks remain required. Multi-person separation of duties is deferred.

The service requires an explicit trusted HTTPS origin (`Audience`), challenge
lifetime (1 second to 5 minutes), and approval lifetime (1 second to 30 days).
These are protocol bounds, not selected deployment defaults. Challenges are
durable in the same SQLite registry and bind version, origin/audience,
organization, invitation ID, purpose, key fingerprint, expiry, and a random nonce.

The initial proof scheme uses RSA-2048 through RSA-4096 with exponent 65537,
SHA-256, and RSASSA-PKCS1-v1_5, matching the existing adapter's RSA signing
family. This is an **enrollment proof**, not the adapter's Noise key or a
replacement for trusted-provider authentication. Hardware-backed key storage,
attestation, and link/authentication credential binding remain installer/provider
integration work. Possession of a key does not prove device posture.

1. Generate the key locally. Encode its public key as standard Base64 of DER
   SubjectPublicKeyInfo. Retain the private key on the device.
2. POST JSON to `/enrollment/v1/challenges` with `organization`,
   `invitation_id`, `purpose: "claim"`, `enrollment_code`, and `public_key`.
   The administrator must transfer the non-secret invitation ID and organization
   with the instructions as well as the code through the separate secure channel.
   The invitation must still be unclaimed and unexpired.
3. The response contains `challenge_id`, `payload` (Base64 of the exact bytes
   to sign), and `expires_at`. The installer must verify the payload's version,
   audience, purpose, invitation/organization, and public-key fingerprint against
   its trusted configuration and local request before signing. Do not sign
   arbitrary unvalidated payloads or reserialize the JSON before signing.
4. Sign SHA-256 of the decoded payload using RSASSA-PKCS1-v1_5, then POST
   `challenge_id` and Base64 `signature` to `/enrollment/v1/proofs`.
   The server verifies the signature against the stored key and payload, then
   claims the invitation, records its approval deadline/audit, and consumes
   the challenge in one transaction. Cancelled/expired invitations fail closed.
5. Persist the local key and non-secret request metadata. To check status,
   request a fresh challenge with `purpose: "status"` and the same public key,
   **without** `enrollment_code`, then sign/submit it identically. Status proofs
   require the claimed key; the invitation code cannot authenticate this operation.
   After a lost proof response, try key-authenticated status first. If the request
   remains unclaimed, obtain a fresh claim challenge; never replay the old proof.

Each proof is single-use, including status proofs. An audience change makes
previous challenges unusable. Replay state persists across service restarts.
Challenge validity ends at its deadline; claim challenges also end no later
than invitation expiry. A status proof returns only invitation ID, state,
revision, key fingerprint, approval deadline, and `credentials_issued: false`.
It does not return owner/recipient, administrative review reasons, credential
material, or network authorization. Approved/rejected/cancelled and
approval-expired requests can be inspected only by their already-bound key.

The device HTTP handler requires TLS, rejects browser-origin requests, uses
the same strict 8192-byte JSON limits, and never accepts secrets in mutation
URLs. It limits requests to 30 per source IP and 600 globally per minute,
without trusting forwarded IP headers, and bounds its source map to 1024.
An authenticated TLS reverse proxy is not yet integrated: simply forwarding
HTTP to the handler fails its TLS requirement.

The registry holds at most 1024 unexpired challenges globally and eight per
invitation, including consumed challenges until expiry. Expired challenges are
removed when another is requested. `429` responses include `Retry-After`;
the installer must back off instead of polling aggressively. These bounded
single-instance controls are not a substitute for production gateway/distributed
abuse protection or deployment-specific capacity sizing.

Tests cover signed claim/status round trips, wrong key/code/organization,
tampered payloads, expiry, cancellation races, persistent concurrent replay
rejection, audit-failure rollback, audience separation, HTTP/TLS guards, private
status shape, and measured challenge/request caps.

### Running the separate device HTTPS service

Build the standalone command from the dashboard module:

```sh
go build -o bin/zpr-enrollment-service ./cmd/zpr-enrollment-service
```

Create an operator-owned configuration file. Example for **local development
only**, using a certificate valid for `localhost` (lifetimes are examples):

```json
{
  "version": 1,
  "listen": "127.0.0.1:9443",
  "allow_non_loopback": false,
  "audience": "https://localhost:9443",
  "database_file": "/var/lib/zpr/enrollment.sqlite",
  "certificate_file": "/etc/zpr/enrollment-server.crt",
  "key_file": "/etc/zpr/enrollment-server.key",
  "challenge_lifetime_seconds": 60,
  "approval_lifetime_seconds": 3600
}
```

Start only when explicitly required:

```sh
bin/zpr-enrollment-service -config /etc/zpr/device-enrollment.json
```

There are no listener, certificate, lifetime, or database defaults.
Configuration rejects unknown fields, trailing JSON, and files over 65536 bytes.
Relative file paths resolve against the configuration file's directory, not
the process working directory. The listener must use a literal IP and explicit
port. Non-loopback addresses require `allow_non_loopback: true`; that flag is
a technical opt-in, **not approval for internet deployment**.

The server validates the certificate/key pair, certificate validity period,
server-authentication usage, and audience hostname before opening the database.
It serves TLS 1.3 or later and requires the HTTP Host to match the configured
audience authority exactly, including any explicit port. The audience is the
stable HTTPS origin trusted by the device, not necessarily the private bind
address. Clients must verify the hostname and trust chain; never use an
insecure TLS override.

Only `/enrollment/v1/challenges` and `/enrollment/v1/proofs` are exposed.
There are no administrative, static GUI, proxy, metrics, or inventory routes
on this listener. The service accepts devices without client certificates
because initial identity is established through the invitation and signed
challenge, not through a credential they do not yet possess.

The device service and private Control-Service administration process must
point to the **same enrollment database on the same host**, with owner-only
permissions under an operator-controlled parent directory. The current SQLite
design is not a network-filesystem or multi-host deployment contract.
Immediate transactions serialize claims/reviews across database connections.
Configure the same approval lifetime in both services; the device service
sets and persists the actual claim deadline. Changes apply after restart and
do not extend existing deadlines. Keep TLS private keys and writable registry
files out of package-download directories.

Read headers are limited to 16 KiB; header/read/write/idle timeouts are
5/10/15/30 seconds. Requests have a 10-second application deadline and a
32-request concurrency cap. Busy responses return `503` with `Retry-After`.
SIGINT/SIGTERM stops acceptance and allows up to 15 seconds for graceful
shutdown before closing remaining connections and the database.

An approved production gateway is still required before remote rollout.
Prefer TCP/TLS pass-through for this initial server. HTTP forwarding is not
supported, and proxy-supplied source/identity headers are not trusted. Behind
a pass-through gateway, connections may share the gateway's source-IP rate
budget; enforce remote-client abuse limits there as well. Certificate renewal,
gateway policy, service supervision, operational monitoring, and remote
deployment approval remain operator work. This change does not configure or
expose any running service.

Live TLS integration tests cover device claim, approval from a separate
registry connection, key-authenticated status, admin route isolation, exact
Host checks, certificate trust, launch readiness, and graceful shutdown with
Simulator configuration unavailable.

### Enrollment client core

`internal/enrollment.NewClient` implements the installer's claim/status
exchange. It accepts a trusted HTTPS audience, an optional CA pool (nil uses
OS trust roots), and a `crypto.Signer` exposing an approved RSA public key.
It does not itself select or persist a key store, generate credentials, or change
network settings. The separate development wizard is described below. The signer boundary allows a future
hardware-backed signer without exporting its private key; actual TPM support
is not implemented.

The client verifies server certificates and hostnames using TLS 1.3 or later.
It never follows redirects or inherits environment-controlled HTTP proxies.
Trusted deployments needing a forward proxy require a separate explicit proxy
contract; disabling TLS verification is not supported.

Before signing, it verifies challenge version, audience, organization,
invitation, purpose, local public-key fingerprint, challenge ID, nonce, and
matching expiry. Challenges must still be valid and expire within five minutes.
It signs the exact decoded payload bytes and checks the resulting RSA signature
locally before sending it. Enrollment service and device clocks must be correct;
there is no silent clock-skew override.

`Claim` takes the code in memory; `Status` never accepts a code. `Fingerprint`
supplies the verification value to show in the future wizard. Call `Close` to
close idle connections. The caller retains its signer and non-secret request
metadata to resume status after restart. This core does not persist the code,
log it, or accept it through a command-line interface.

Responses are bounded to 16 KiB and strictly decoded. Wrong identity/key,
invalid revision/state, missing pending approval deadline, and unexpected
credential issuance are rejected. An approved response still means approval
only, not successful credential installation or ZPR connectivity.

HTTP failures return a `RemoteError` with status and bounded `RetryAfter`,
without reflecting server error bodies or request secrets. No requests or
proofs are automatically replayed. After uncertain proof delivery, use a fresh
key-authenticated status request; the wizard must surface uncertainty and
honor backoff rather than silently declaring success or starting a new asset.

Client tests cover a real TLS claim/approval/resumed-status exchange, rejected
challenge bindings before signing, redirect prevention, unknown trust roots,
response limits/error redaction, retry timing, context cancellation,
environment-proxy isolation, and rejection of false issued/connected results.

### Development software enrollment identity

The initial Linux development release uses a **non-hardware-backed software
key**, as approved in the [Provisioning plan](../../Provisioning%20plan.md).
`CreateSoftwareIdentity` and `LoadSoftwareIdentity` in
[identity.go](internal/enrollment/identity.go) provide the local persistence
core. They are available on Linux and macOS (for development tests); this
does not expand the approved installer platform beyond Debian/Ubuntu.

Creation generates RSA-3072 and stores PKCS#8 key material with versioned
`software-development` metadata in `identity.json`. Metadata includes only
the trusted HTTPS audience, organization, and invitation ID. It does not
include the enrollment code, issued credentials, or cached approval status.
The returned identity implements `crypto.Signer` and exposes `Metadata()`;
pass it to `NewClient` and use a fresh `Status` request after restart.

The caller must select an absolute, clean, dedicated state directory under a
trusted parent on a local filesystem that supports hard links and file/directory
sync. Creation makes the leaf directory with mode `0700` and the file with
mode `0600`; existing state must belong to the effective user and have no
group/other permissions. Symlink directories/files, nonregular files, additional
hard links, unexpected fields, oversized/corrupt records, and incompatible
versions/protection/key formats are rejected rather than repaired silently.
The caller must separately validate installation configuration and CA trust;
stored metadata is not an authoritative source of new trust roots.

Creation syncs a complete temporary file, atomically links it to the final
name without replacement, removes the temporary name, and syncs the directory.
Concurrent creation has exactly one winner. An I/O failure after publication
can leave a complete identity even though creation returned an error: surface
the error and inspect/load the state, never assume failure means no key exists.
A crash can leave a private temporary file; there is no automatic deletion
or recovery of temporary key files. Loading missing/corrupt state never
creates a replacement. Losing the key requires administrator-led recovery,
not reuse of the invitation code to impersonate the previous identity.

This is permission-protected storage, **not encrypted-at-rest or non-exportable
storage**. Root/the owning account and backups containing this file can access
the key; copying the state can impersonate the enrolled request. Protect backup
and support bundles accordingly and never include the state file in logs.
TPM storage, attestation, system service-account handoff,
rotation, and production key-recovery policy remain unimplemented.

Tests cover persistence, signing after reload, unsafe filesystem entries,
strict bounded record validation, no-overwrite concurrent creation, and an
actual TLS claim/approval/status exchange using a key reloaded from disk
without the enrollment code. The identity store does not issue credentials
or provide live adapter admission; the development setup command below uses it.

Run the focused tests with `go test -race ./internal/enrollment`. The complete
workflow and release decisions are in the
[Provisioning plan](../../Provisioning%20plan.md).

### Local browser setup wizard (development)

The [setup command](cmd/zpr-enrollment-setup/main.go) is a runnable development
GUI, not a signed installer or production adapter. Build it explicitly:

```sh
go build -o /tmp/zpr-enrollment-setup ./cmd/zpr-enrollment-setup
/tmp/zpr-enrollment-setup -config /path/to/trusted-setup.json
```

Example operator configuration (replace all paths and the audience):

```json
{
  "version": 1,
  "audience": "https://enroll.example.com:9443",
  "ca_file": "/etc/zpr/enrollment-ca.pem",
  "state_directory": "/var/lib/zpr/enrollment-development",
  "allow_software_development": true
}
```

The state directory is absolute and must be a dedicated owner-only leaf under
an existing trusted parent. Run as its intended owner; do not launch a browser
as root or indiscriminately run the wizard with sudo. The development desktop
package uses the logged-in user as described below; system service-account
handoff is not implemented. Configuration and CA files must be regular,
non-symlink files owned by root or the effective user, not writable by group
or others, and at most 64 KiB. Their parent directories must also be trusted.
Relative `ca_file` paths resolve against the configuration directory. Omit
`ca_file` to use OS trust roots; a configured file supplies a separate CA pool.
An invalid configuration, corrupt identity, or changed stored audience fails
startup. The GUI cannot override the audience, CA roots, or saved invitation.

The command listens only on `127.0.0.1`, chooses an ephemeral port, and prints
a private URL with a one-hour session capability in the fragment. Open it
manually in this machine's normal browser and keep the command running.
Do not share/capture the URL or process output in support logs. No browser is
launched automatically. The page removes the fragment using `replaceState`;
it keeps the capability only in memory and sends it in API headers. Reloading
requires reopening the printed complete URL; after one hour restart the
command for a new session. Ctrl-C/SIGTERM shuts down the local listener.
There is no remote binding option, headless flow, or SSH/browser tunnel contract.

Every API call requires the capability; mutations require the exact local
Origin. Exact Host, Fetch Metadata, method, body size, and strict JSON checks
reject cross-origin or malformed requests. Responses are no-store, scripts
and styles are embedded local assets, and CSP disallows external resources,
inline scripts, framing, and form navigation. These controls do not protect
against root/the same local account or a compromised browser.

Workflow:

1. Enter the administrator-supplied organization and invitation ID. Preparing
   creates/persists the software key locally, without sending a claim.
2. Verify those details and enter the separately delivered code. It is cleared
   from the password field when submitted, never logged or stored, and sent
   only over the verified enrollment HTTPS exchange.
3. Share the displayed fingerprint through an authenticated administrator
   verification channel. Pending approval polls at ten-second intervals;
   longer service `Retry-After` is enforced on the server as well as in the UI.
4. After restart use **Check fresh status** with the saved key, without a code.
   A successful approval explicitly states that credentials are **not issued**
   and ZPR connectivity is **not established**.
5. On a transport/invalid response failure, the outcome remains uncertain.
   Automatic polling stops and code replay is blocked; wait and request fresh
   status. A denied status does not prove the invitation is unclaimed. Only
   explicit administrator-confirmed recovery can submit the code again with
   the same key. Rejected/cancelled/expired requests require administrator
   action, not deletion or replacement of local identity state.

Requests are serialized; concurrent operations are rejected rather than
queued. The ten-second floor also limits manual polling. Retry timing is
in-memory per process, not a distributed rate-control guarantee. Opening a
new command does not restore a cached approval decision.

Go tests cover session/host/origin isolation, strict configuration, no key
replacement, live TLS claim/approval/status after restart, administrator-confirmed
recovery, uncertain responses, server backoff, and graceful shutdown:

```sh
go test -race ./internal/enrollment
sh scripts/test-browser-container.sh \
  tests/browser/enrollment-setup.spec.mjs tests/browser/enrollment-live.spec.mjs
```

The isolated desktop/tablet browser tests start their own temporary loopback
wizard with an unavailable remote service, never Simulator. They verify key
preparation, code-field clearing, uncertain failure, and missing-session
blocking. All four desktop/tablet tests passed in the pinned Playwright
container; host Node/npm is not required. The runner cross-builds the Linux
setup fixture for the Docker server architecture and supplies it through
`ZPR_SETUP_TEST_BINARY`. Direct npm runs can instead build it with local Go.
These tests do not certify a full graphical clean-machine installation or
production credential issuance.

The [real-service browser suite](tests/browser/enrollment-live.spec.mjs) adds
eight desktop/tablet cases, bringing enrollment coverage to twelve passing
browser cases. It uses the actual wizard, TLS device handler, SQLite registry,
and an independent reviewer connection, with invalid Simulator configuration.
It verifies:

- A browser-submitted claim reaches pending approval and automatic polling
  observes a fingerprint-bound approval; only one code submission occurs.
- Wizard restart retains the identical stored key/metadata and requires a
  fresh signed status before showing approval.
- Rejection remains terminal across restart; no code form reappears.
- An unclaimed saved identity requires a status denial, administrator-confirmed
  recovery checkbox, and the server retry delay before code submission.
- A successfully delivered claim whose local browser response is lost remains
  uncertain, then recovers through signed status without another claim or key.

The [opt-in test fixture](internal/enrollment/browser_fixture_test.go) is
compiled into an enrollment Go test executable, not the shipped wizard.
Review/restart commands use the test process's stdin, never an HTTP bypass.
The container runner builds this executable for the Docker architecture and
sets `ZPR_ENROLLMENT_TEST_BINARY`. Direct npm execution can build it with
local Go. Test startup/teardown bounds process waits and closes temporary
listeners and state. Reviewer actions exercise the durable review state
machine, not certificate-authorized administration or Control Room login.
Thus these tests are device-flow acceptance, not named-user authorization,
graphical desktop package certification, credential issuance, or ZPR admission.

### Development Debian/Ubuntu package

Approved initial targets are **Ubuntu 24.04 LTS and Debian 12, amd64 desktop**.
The [package builder](packaging/enrollment/build-deb.sh) requires `dpkg-deb`
and standard Debian/Ubuntu shell utilities. Build a static Linux executable
from the module root, then package it:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -o /tmp/zpr-enrollment-setup ./cmd/zpr-enrollment-setup
sh packaging/enrollment/build-deb.sh /tmp/zpr-enrollment-setup \
  0.1.0~dev1 /tmp/zpr-enrollment-setup_0.1.0~dev1_amd64.deb
```

The output path must not exist. The builder checks version characters and ELF
amd64 format and sets package ownership to root. It creates an **unsigned
development artifact**, not an authenticated installer. A checksum is not a
signature; do not distribute it as the signed package required by the plan.
Detached manifest signing/verification is now implemented below. Production
signing authority, authenticated download hosting, revocation of old versions,
and release automation remain pending.

Contents:

- `/usr/bin/zpr-enrollment-setup`: static wizard binary.
- `/usr/share/applications/zpr-enrollment-setup.desktop`: terminal-based
  **ZPR Machine Setup (Development)** launcher.
- `/usr/share/zpr-enrollment-setup/setup.example.json`: intentionally invalid
  endpoint example; no active configuration or invitation.
- `/usr/share/doc/zpr-enrollment-setup/README`: operator/user instructions.

There are no installation hooks, system services, auto-start listeners,
private keys, codes, or adapter binaries. Installation does not create a
device identity. A modern browser, graphical desktop, and terminal emulator
are prerequisites; the package does not install a desktop environment.

For controlled development testing, install using the target distribution's
package manager. An administrator must then create a root-owned, non-writable
by others `/etc/zpr` directory and `/etc/zpr/enrollment-setup.json`, replacing
the example audience with the independently trusted HTTPS endpoint and
optionally adding `ca_file`. **Omit `state_directory`** in desktop configuration.
Keep the development software-key opt-in explicit. Do not use an invitation,
email, or browser form to select CA trust.

Launch the desktop entry as the logged-in user, or run:

```sh
zpr-enrollment-setup -config /etc/zpr/enrollment-setup.json -user-state
```

The desktop entry opens a terminal; open the printed private URL in the same
user's normal browser. It does not run a browser as root. `-user-state` refuses
root and derives state from `$HOME/.local/state/zpr-enrollment-development`
(it intentionally does not use `XDG_STATE_HOME`). The home and existing
`.local/state` parents must be owned by that user, not symlinks or group/other
writable. Missing parents are created with `0700`; the identity store creates
the private leaf. Existing configuration cannot override the per-user path.
The explicit-state command remains available for development/service testing,
not for the desktop package launcher.

Upgrades, removal, and purge preserve per-user state and administrator-created
configuration: the package owns neither. Losing/deleting the key requires
administrator-led recovery; uninstall is not server revocation. The eventual
machine adapter must not silently consume another user's key. System-service
identity handoff is deliberately not implemented.

The [package lifecycle checks](packaging/enrollment/test-deb.sh) are only for
fresh disposable Docker containers, never a real workstation. They install
the package, run setup tests as an unprivileged user, verify launch readiness,
reinstall it, and purge while checking preserved user state/configuration.
They passed on Debian 12 and Ubuntu 24.04 amd64, with network disabled.
The retained-state fixture checks file preservation, not adapter revocation.
Graphical launcher/desktop installation and end-to-end signed provisioning
still require clean-machine certification.

### Detached release signatures

The approved first distribution contract uses a detached OpenPGP signature
over a canonical text manifest, not an embedded `.deb` signature or an APT
repository. **Installing the `.deb` directly does not verify this signature.**
The retained development artifact is still unsigned; no production signing
identity has been created.

The [signer](packaging/enrollment/sign-release.sh) and
[verifier](packaging/enrollment/verify-release.sh) require GnuPG (`gpg`, `gpgv`),
`sha256sum`, `dpkg-deb`, and standard Debian/Ubuntu shell utilities.
An operator supplies an existing private key through their managed GnuPG
agent/hardware signer. Scripts never generate/import private keys or accept
a passphrase argument. Example operator workflow:

```sh
# PRIMARY_FINGERPRINT is the full uppercase fingerprint of the approved
# existing signer. EXPIRES is an explicit Unix timestamp, within 30 days.
sh packaging/enrollment/sign-release.sh /trusted/build/package.deb \
  0.1.0~dev1 "$PRIMARY_FINGERPRINT" "$EXPIRES" /trusted/releases/new-dev1
```

The new output directory must not exist. Signer input is copied into private
temporary storage; package metadata is checked before signing. The signature
is verified against the requested primary key before publishing. Distribute
only after a successful exit; an I/O failure can leave an incomplete output
directory that must not be served. The output contains exactly:

- `zpr-enrollment-setup_0.1.0~dev1_amd64.deb`
- `release.manifest`
- `release.manifest.asc`

The five-line, newline-terminated ASCII manifest binds format version 1,
release version, expiry epoch, canonical package filename, and SHA-256.
The signer chooses SHA-256 signatures; verification accepts SHA-256/384/512.
No codes, machine keys, CA trust, or device invitations belong in this bundle.
Publish the three files through operator-managed HTTPS without mutable local
write access. Hosting/email delivery is not implemented.

**Before downloading**, provision the trusted verifier scripts (including
`release-common.sh`), binary OpenPGP public keyring, full primary fingerprint,
and exact approved release version through an independent authenticated
operator channel. Do not trust a keyring, fingerprint, script, or version just
because it accompanies the download or appears in its email. The verifier
does not contact keyservers, import keys into the user's keyring, or fall back
to default trust. Example recipient check:

```sh
sh /trusted/tools/verify-release.sh \
  /trusted/download/zpr-enrollment-setup_0.1.0~dev1_amd64.deb \
  /trusted/download/release.manifest /trusted/download/release.manifest.asc \
  /trusted/keys/release-signers.gpg "$PRIMARY_FINGERPRINT" 0.1.0~dev1
```

Verification requires exactly one valid signature from the pinned primary key
or its signing subkey and rejects expired/revoked keys present in the supplied
keyring. It validates canonical manifest bytes, version, lifetime, SHA-256,
and package name/version/architecture. Expected-version pinning rejects
substitution of an older release; it does not automatically discover the
latest release or record a monotonic version history. Correct system clocks
are required. Expiry limits initial download verification, not invitation
lifetime, installed-package execution, or server revocation.

The command copies inputs into private temporary storage and never installs
anything. Preserve the exact original bytes in a trusted, non-writable-by-others
directory before and after verification, then install through the approved
package manager as a separate administrator action. Changing the file after
verification invalidates that result; the scripts are not a privileged
verification-to-install transaction. Signature authenticity is not evidence
of hardware protection, credential issuance, or live ZPR admission.

Operators must independently distribute refreshed keyrings/revocation data
and fingerprints during key rotation. Offline verification cannot discover
a revocation absent from its trusted keyring. No production keys are bundled,
and signed test keys must never become production trust roots.

[Disposable release tests](packaging/enrollment/test-release.sh) generate
short-lived test keys only inside Docker containers and leave no retained
signed release. Valid primary/subkey checks and rejection of tampering,
wrong trust/version, expired keys/releases, excessive lifetime, weak digests,
unexpected paths/fields/bytes, and symlink inputs pass on Debian 12 and an
Ubuntu 24.04-based image. They install no packages and do not certify release
operations or graphical clean-machine provisioning.

### Trusted HTTPS release staging

[download-release.sh](packaging/enrollment/download-release.sh) combines bounded
HTTPS download with the existing detached-signature verifier. It never installs
anything and does not download trust roots, signing keys, or verifier scripts.
Provision this tool and its sibling verification scripts, trusted keyring,
primary fingerprint, exact expected version, and approved download base URL
through an independent authenticated operator channel first.

On Debian 12/Ubuntu 24.04, install the prerequisite tools (`curl`, GnuPG,
`dpkg`, coreutils) through trusted OS package management. Create an existing
private output parent owned by the invoking user with **mode 0700**. Example:

```sh
mkdir -m 0700 "$HOME/zpr-release-downloads"
sh /trusted/tools/download-release.sh \
  https://downloads.example.com/zpr/dev1 \
  /trusted/keys/release-signers.gpg "$PRIMARY_FINGERPRINT" 0.1.0~dev1 \
  "$HOME/zpr-release-downloads/dev1" system
```

The final argument is `system` for OS TLS roots or the path to an independently
trusted CA PEM file. Environment CA overrides (`CURL_CA_BUNDLE`, `SSL_CERT_FILE`,
`SSL_CERT_DIR`), HTTP proxies, and user curl configuration are ignored.
Certificate and hostname verification remain enabled; TLS 1.2 or later is
required for the static download endpoint (the enrollment device service itself
still requires TLS 1.3). Initial URL support is intentionally narrow: HTTPS
DNS/IPv4 authority, optional port, and plain path; no credentials, percent
escapes, queries, fragments, IPv6 literals, or redirect-based CDN links.
Provide the final HTTPS base URL directly rather than weakening TLS checks.

The tool fetches exactly `release.manifest`, `release.manifest.asc`, and the
canonical versioned amd64 package. Only HTTP 200 is accepted; redirects are
not followed. Each transfer has a ten-second connect timeout and 120-second
total timeout. Accepted content limits are 1 KiB manifest, 64 KiB signature,
and 32 MiB package. Curl size checks plus an OS file-size limit bound chunked
transfers even on older curl: temporary data may reach the OS cap (up to
64 MiB) before rejection, but oversized files are never accepted/published.
There is no silent retry, HTTP downgrade, or automatic proxy fallback.

The existing parent must be an absolute canonical path without symlink
components, owned by the current user, mode 0700. Its ancestors, CA, keyring,
and tools must also be independently trusted; this is not protection from
root/the owning account. A private sibling temporary directory holds downloads
and copies of trust material. All signatures, hashes, version, expiry, and
package metadata must verify before publication. GNU `mv -T -n` atomically
publishes the directory without replacing/merging into an existing destination,
including a destination created during download. On failure, no completed
bundle is published and temporary files are removed. Existing output is left
unchanged. Only the package, manifest, and signature remain in successful output.

Staging is a separate unprivileged step, not a privileged verification-to-install
transaction. Preserve the private output, reverify if anything changes or the
release expires, and install the exact verified package through a separate
approved administrator action. The tools do not auto-select newer versions,
send emails, host downloads, manage production keys, or expose an installer GUI.

The [HTTPS download tests](packaging/enrollment/test-download.mjs) run through
`ZPR_TEST_DOWNLOAD=1 sh packaging/enrollment/test-release.sh package.deb` inside
a disposable Docker container with Node, OpenSSL, curl, and GnuPG installed.
They use a local HTTPS server and disposable signed releases, not an external
download host or Simulator. Tests pass with Debian 12 tooling and an Ubuntu
24.04-based image, covering valid staging, curl/proxy override isolation,
untrusted TLS, plaintext/redirect rejection, missing/truncated/oversized transfers,
signature/hash failures, unsafe/symlink parents, concurrent destination creation,
and cleanup. Test keys, TLS identities, and bundles are removed at teardown.

## Mac enrollment app and adapter validation (development)

The Apple Silicon development image contains a per-user **ZPR Machine Setup.app**,
the shared loopback browser wizard, Terminal/default-browser launcher, instructions
and configuration example. Copy the app to the intended user's `~/Applications`.
No privileged installer, launch daemon, network adapter, route/DNS changes,
automatic CA import or enrollment request is performed by copying the app.
Other accounts require their own asset/invitation and enrollment identity.

**This app is enrollment-only, not a connected Mac adapter.** It is ad-hoc signed
for local integrity, **not Developer ID signed or notarized**. Do not remove
quarantine or disable Gatekeeper to distribute it. The Mach-O deployment floor
and bundle minimum are explicitly macOS 13.0, with native arm64 only; testing
on this host does not certify every macOS release, Intel, Finder/Automation
permissions or remote clean-machine installation.

An administrator must provision
`~/Library/Application Support/ZPR/EnrollmentSetup/setup.json` from the bundled
example, supply the reachable HTTPS device service and optional PEM `ca_file`,
and retain `"key_protection":"macos-keychain"`. The invalid example cannot enroll
anything. Configuration/CA files must be regular user/root-owned files, not
shared-writable; they and their non-symlinked parents must be inside the trusted
real desktop home. Trust/ownership of the home and its external ancestors is an
operator prerequisite. Omit `state_directory`, keys and codes. Relative CA paths
are resolved beside the configuration. A remote Mac cannot use localhost on
the operator's machine.

Opening the app requests Terminal automation to run setup as the desktop user.
The wizard opens the default browser to a private one-hour loopback capability
URL. Keep Terminal running; Ctrl-C stops it. If configuration is absent, the app
offers instructions instead of sending a request. If browser launch fails, an
explicit warning retains the URL for manual opening. The launcher requires the
Keychain mode exactly; omission or unavailable native support fails rather than
falling back to plaintext. The existing key-fingerprint verification, separately
delivered enrollment code, claim/status and uncertain-response flow is unchanged.
Approval remains only an administrative decision.

The full RSA-3072 software identity and enrollment metadata are stored in the
current user's configured default **macOS Keychain** as a generic-password item
with service `com.zpr.enrollment.development` and label
`ZPR development enrollment identity`. Only an owner-private, single-link
reference file is published on disk:
`~/Library/Application Support/ZPR/EnrollmentDevelopment/identity.keychain.json`.
Keychain encryption/access control protects storage; the authorized wizard still
loads the software key in memory. This is not Secure Enclave-backed, TPM
attestation or a production adapter credential.

The native implementation uses Apple's Security/CoreFoundation frameworks via
CGO, not a private-key command-line argument or a third-party Keychain library.
It explicitly scopes lookup to the selected default Keychain and exact persistent
reference. Denial/locked Keychain, deleted item, invalid reference or corruption
fails closed and never produces a replacement key. Publication never overwrites
an existing reference; competing publishers remove only their unused new items.
Changing the default Keychain or app signature can require deliberate recovery/
authorization. Do not automatically grant other applications Keychain access.

Replacing/removing the app preserves the Keychain item, reference and operator
configuration. Removing the app does not revoke enrollment. Legacy unencrypted
development state is not automatically migrated/imported. Administrators must
avoid inviting the same asset again merely because the new Mac profile uses
a separate state path. Lost references/items, account migration and retirement
require deliberate recovery/revocation. A future root adapter must not receive
the user identity by exporting its private key to a plaintext service file.

Native build from this dashboard directory (the explicit deployment flags prevent
the host SDK from silently producing a newer minimum OS than the bundle declares):

```sh
MACOSX_DEPLOYMENT_TARGET=13.0 \
CGO_CFLAGS='-mmacosx-version-min=13.0' \
CGO_LDFLAGS='-mmacosx-version-min=13.0' \
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -trimpath \
  -o /absolute/output/zpr-enrollment-setup-macos-arm64 ./cmd/zpr-enrollment-setup
sh packaging/enrollment/build-macos.sh \
  /absolute/output/zpr-enrollment-setup-macos-arm64 0.1.0 \
  /absolute/output/zpr-enrollment-macos.dmg
sh packaging/enrollment/test-macos.sh \
  /absolute/output/zpr-enrollment-macos.dmg \
  /absolute/output/zpr-enrollment-setup-macos-arm64
```

The builder rejects existing output, wrong architecture, missing Security.framework
or a mismatched Mach-O deployment floor. It compiles the AppleScript launcher,
signs the executable/app ad-hoc, verifies the sealed bundle and creates a read-only
compressed DMG. The test mounts it read-only, checks signatures/plist/resources,
compares the packaged executable against equivalently signed source bytes and
starts the actual packaged wizard under a disposable home. Authorized session
reads succeed and anonymous reads fail; no login Keychain entry, browser, live
claim or adapter is created by that package check.

```sh
go test -race ./internal/enrollment
CGO_ENABLED=0 go test ./internal/enrollment
```

Mac-specific tests use disposable, explicitly scoped test Keychains—not the login
Keychain—to verify key persistence/proof signing, fresh-process reload, competing
publishers, deletion recovery, corrupt/unsafe references, legacy rejection,
configuration-parent checks and protection labels. The no-CGO suite proves that
Keychain configuration fails at startup without native support. Shared browser
flows and Linux/Windows builds remain separate regression gates.

### Existing Mac adapter runtime

The separate `zpr-core` packet handler already has a native `utun` backend.
On this Apple Silicon host, native `cargo test --locked -p ph` passes (246 library
and 267 binary tests, two ignored in each runner); `cargo build --locked -p ph
--bin ph` and non-networking help startup pass. Six new Mac-specific tests cover
interface-name/unit bounds, pre-kernel prefix/MTU rejection, IPv6 masks,
scoped/global address recognition and the single-queue restriction. Socket errors
are checked before constructing an owned descriptor; address add/clear operations
are serialized. Those unprivileged checks did not change networking.

The explicitly administrator-approved
[isolated Mac smoke runner](../../zpr-core/integration-test/macos-utun-smoke.sh)
now also passes on this host: kernel-assigned temporary utun creation, MTU 1400
then 1280, exact IPv6 /128 aliases, duplicate add/remove and two teardown cycles.
Cargo builds unprivileged; only the exact ignored test is elevated via cached
sudo or the macOS administrator dialog. Before/after existing interface, IPv4/
IPv6 default-route and DNS snapshots match; no running organization was changed.
This is **interface lifecycle validation**, not packet-flow certification.
Current high-level Mac address lifecycle is IPv6-only and single-queue. Full
route/DNS rollback, reconnect/sleep/wake, allowed/denied traffic,
credential issuance/Keychain-to-runtime handoff and revocation remain
required. Strict Clippy is currently blocked by unrelated existing dependency
lints and an existing capture-worker partial-write lint; these were not changed.
See the [adapter platform plan](../../zpr-core/ADAPTER_PLATFORM_SUPPORT_PLAN.md).

## Windows 11 x64 enrollment installer (development)

Windows is an additional first-stage enrollment target; the existing Linux
desktop package remains supported. The per-user NSIS setup EXE installs the
same loopback browser wizard, a console/browser launcher, instructions, a
configuration example, a current-user Start menu shortcut and an HKCU uninstall
entry. It requires a native AMD64 Windows 11 desktop (build 22000 or newer),
not ARM64, Windows 10 or Server. Run without elevation. Install does not start
a service/browser, import a CA, change networking/firewall rules or contact
the enrollment service.

**It does not install a network adapter/driver or Windows service, issue
credentials or connect traffic.** This development artifact is unsigned and
not yet certified on a real Windows machine. Authenticode/release verification,
trusted release delivery, credential issuance and adapter-runtime integration
are separate unfinished work. Do not bypass Windows security warnings as an
ordinary distribution procedure.

Installed files are under `%LOCALAPPDATA%\ZPR\EnrollmentSetup`. An administrator
must copy `setup.example.json` to `setup.json`, supply the actual reachable
HTTPS device enrollment audience and, if required, a trusted PEM `ca_file`
(relative to the config or an absolute local path). No trust is auto-discovered;
the invalid example cannot enroll a device. A remote Windows machine cannot
use the operator's localhost endpoint. Never put a code, key or `state_directory`
in this configuration. The wizard rejects unsafe writers/owners, reparse
points, hard-linked config/key files, UNC/device paths and alternate streams.

Launch **ZPR Machine Setup (Development)** from the current user's Start menu.
The console opens the default browser to a private one-hour loopback capability
URL. Keep it running; Ctrl-C stops setup. Failure to open a browser is explicit
and retains the URL for manual opening. The existing invitation/key verification,
separately transmitted code and uncertain-response recovery flow is unchanged.
Control Room review controls are available only with matching named-user and
independent Control-Service grants for that organization plus a configured
approval deadline. A reviewer must confirm that the exact key fingerprint was
verified through a trusted channel and record a reason; the backend binds the
decision to the fresh invitation revision and key. Uncertain responses require
fresh-detail reconciliation without automatic retry. This review does not issue
credentials, admit an adapter, or establish connectivity.

Windows private state is
`%LOCALAPPDATA%\ZPR\EnrollmentDevelopment\identity.dpapi`. The RSA-3072 identity
and metadata are encrypted with **current-user DPAPI**, never machine-wide DPAPI,
and stored under user/SYSTEM-only ACLs. Creation publishes complete ciphertext
without overwriting an existing identity; invalid protection, unsafe ACLs,
corruption and decryption failure never generate a replacement key. There is
no plaintext import/fallback. This is software development protection, not TPM
attestation; the logged-in user, compromised processes or privileged recovery
administrators can still impersonate the identity.

Each Windows user has separate enrollment state and needs a separate asset/
invitation. This does not yet mean multiple working network adapters. Neither
upgrade nor uninstall deletes the user's key or operator-created config/CA files;
uninstall does not revoke enrollment. Plan DPAPI recovery before account resets
or profile migration; lost state requires administrator-led replacement/revocation.
The Linux/macOS file protection and state paths remain unchanged.

Build from this dashboard directory with the existing Go toolchain and NSIS:

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -o /absolute/output/zpr-enrollment-setup.exe ./cmd/zpr-enrollment-setup
sh packaging/enrollment/build-windows.sh /absolute/output/zpr-enrollment-setup.exe \
  0.1.0.0 /absolute/output/zpr-enrollment-installer.exe
```

The builder refuses an existing output, non-AMD64 PE payload or invalid four-part
version. For macOS/Linux hosts without NSIS, the provided
`packaging/enrollment/Dockerfile.windows-builder` installs NSIS and the extraction
checker in an isolated, base-digest-pinned image:

```sh
docker build -f packaging/enrollment/Dockerfile.windows-builder \
  -t zpr-enrollment-windows-builder:local packaging/enrollment
docker run --rm -v "$PWD/packaging/enrollment:/packaging:ro" \
  -v /absolute/output:/artifacts zpr-enrollment-windows-builder:local \
  /packaging/build-windows.sh /artifacts/zpr-enrollment-setup.exe \
  0.1.0.0 /artifacts/zpr-enrollment-installer.exe
```

Cross-build, Windows-target vet/test compilation, existing enrollment race
tests and installer extraction checks pass. The extraction runner compares the
packaged EXE/config/launcher/instructions byte-for-byte and rejects invalid inputs:

```sh
docker run --rm -v "$PWD/packaging/enrollment:/packaging:ro" \
  -v /absolute/output:/artifacts zpr-enrollment-windows-builder:local \
  /packaging/test-windows-build.sh /artifacts/zpr-enrollment-setup.exe
```

**Native Windows certification remains required**, without elevation in a clean
Windows 11 x64 account. Run from the dashboard source directory:

```powershell
go test ./internal/enrollment -run Windows -count=1
powershell -NoProfile -File packaging/enrollment/test-windows.ps1 `
  -Installer C:\test\zpr-enrollment-installer.exe
```

The Go suite checks persistence/proof signing, DPAPI tampering, concurrent
publication, private ACLs, config overrides, unsafe paths, loopback authorization
and protection labels. The installer test refuses existing installation/state,
checks silent install, starts the actual wizard, prepares a test-only key without
claiming an invitation, and verifies upgrade/uninstall preservation. It removes
only its newly created test state/config. Also certify interactive Start menu/
default-browser behavior, two-user isolation and DPAPI decryption rejection
under another account, junction/symlink denial, Windows 10/ARM64/Server rejection,
and the actual reachable HTTPS claim/status/restart flow. Never present successful
cross-compilation or extraction as a substitute for those runtime checks.

## Named operator OIDC and opt-in direct HTTPS

[internal/operatorauth](internal/operatorauth/auth.go) provides the approved
OIDC login/session boundary for Control Room. Direct HTTPS and login are
explicitly opt-in; the default HTTP listener does not mount authentication.
Existing certificate-authorized administration remains unchanged. Control Room
GUI invitation/review controls remain disconnected. Versioned enrollment APIs
can now be explicitly enabled with signed named-user delegation and independently
configured Control-Service grants; sign-in alone is never sufficient.

The library uses `github.com/coreos/go-oidc/v3` and `golang.org/x/oauth2`, with
`go-jose` for signed-token integration tests. This concrete authentication
feature needs maintained protocol/crypto libraries; hand-written JWT validation
is intentionally avoided. Existing unrelated module dependencies are preserved.

`operatorauth.New(ctx, config, clientSecret)` validates typed operator
configuration and performs HTTPS discovery. The secret is supplied separately,
not included in public JSON, requests, logs, or sessions. The Control Room now
loads strict bounded configuration and a separate protected secret file.
Actual IdP registration/deployment remains the operator's responsibility. Example typed
configuration's JSON shape:

```json
{
  "version": 1,
  "issuer": "https://identity.example.com",
  "client_id": "zpr-control-room",
  "redirect_url": "https://localhost:8787/auth/operator/callback",
  "session_lifetime_seconds": 600,
  "grants": [{
    "issuer": "https://identity.example.com",
    "subject": "stable-admin-subject",
    "organizations": ["production"],
    "permissions": ["read", "create", "approve", "reject", "cancel"]
  }]
}
```

Issuer/subject pairs are exact and explicit; email, display name, groups,
arbitrary browser headers, and shared backend certificates are never used
as authorization. Grants are copied at initialization, so caller mutation
cannot expand scopes. Updates require a new instance, invalidating old sessions;
there is no hot reload or distributed session store.

When enabled on direct HTTPS, the handler supports:

- `POST /auth/operator/login`: exact same-origin initiation; creates a
  five-minute browser-bound state/nonce and redirects to authorization with
  S256 PKCE and only the `openid` scope.
- `GET /auth/operator/callback`: one-time state/browser cookie, code exchange,
  signed ID-token validation, explicit grant lookup, and opaque session cookie.
  Expired, missing-browser, lost-state and replayed login attempts remain HTTP
  403, but display a recovery page with a single **Timed out. Try again.** action
  (a same-origin POST starting fresh state/nonce/PKCE). Recovery
  never exchanges the rejected code or reflects callback parameters. The
  five-minute login timeout and all authorization checks remain unchanged.
- `GET /auth/operator/session`: same-origin identity and CSRF token only,
  `no-store`; no provider tokens or client secret.
- `POST /auth/operator/logout`: same-origin CSRF-protected local session
  invalidation. It does not log the user out of the identity provider.

All routes require actual TLS and the exact configured callback authority.
Forwarded headers cannot turn plaintext into trusted HTTPS. Cookies use
`__Host-` names, Secure, HttpOnly, Path `/`, no Domain, and SameSite=Lax.
Discovery authorization/token/JWKS endpoints must also be HTTPS.
The owned IdP HTTP client enforces timeouts, rejects redirects, and does not
inherit environment proxies. Provider token validation uses RS256/ES256,
issuer/client audience, authorized party, nonce, issued-at, signature, and expiry.

Sessions store only grants, CSRF proof, and expiration in memory. They expire
at the earlier of token expiry and an explicit 60-3600 second lifetime.
There is no sliding renewal, refresh-token storage, or offline token acceptance.
Restarts clear all sessions. Login/session capacity is bounded at 256/1024;
expired entries are removed on access. Gateway/source rate controls and
multi-process sessions remain deployment work.

`Session` returns a copied identity/CSRF token for a same-origin reader.
`Authorize` checks the active session plus explicit organization/permission;
mutations additionally require exact Origin and `X-ZPR-CSRF`. Only local logout
uses empty scope arguments. An enrollment integration must always pass the
actual organization and required permission and must never trust an identity
header or serialize a session as an unsigned backend assertion. Request denial
is surfaced without reflecting raw tokens, authorization codes, or provider errors.

Tests use a disposable HTTPS provider with real JWKS/signatures and real code
exchange/PKCE. They cover grant isolation, callback replay, initiating-browser
binding, invalid token claims/signatures, IdP failure, plaintext/cross-origin
rejection, forged headers, CSRF, logout, token-bounded session expiry, and
capacity limits without Simulator:

```sh
go test -race ./internal/operatorauth ./internal/enrollment
```

### Direct HTTPS configuration

The approved initial integration terminates TLS in Control Room itself. This
increment preserves the existing loopback-only browser/API exposure: use a literal
loopback `-listen` address and a `localhost`/loopback origin with the same fixed
port. A container may listen on `0.0.0.0` only with an explicit
`ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP`; the Docker host port must still bind
to `127.0.0.1`, and the handler requires actual TLS, the exact loopback Host,
and a loopback or exact configured peer address. Other wildcard/remote binding is rejected; remote-operator exposure needs a
separate reviewed authorization boundary for all existing APIs, not just enrollment.
Forwarded headers and the existing browser gateway cannot establish TLS or a
named-user identity for these handlers.

For `-mode control-room`, keep the existing Control-Service mTLS environment
configuration and explicitly add:

```sh
export ZPR_CONTROL_ROOM_ORIGIN=https://localhost:8787
export ZPR_CONTROL_ROOM_CERT_FILE=/etc/zpr/control-room/server.crt
export ZPR_CONTROL_ROOM_KEY_FILE=/etc/zpr/control-room/server.key
export ZPR_OPERATOR_OIDC_CONFIG_FILE=/etc/zpr/control-room/oidc.json
export ZPR_OPERATOR_OIDC_SECRET_FILE=/etc/zpr/control-room/oidc-client.secret
go run ./cmd/zpr-web-dashboard -mode control-room -listen 127.0.0.1:8787
```

Provision a current server-auth certificate covering the origin hostname; the
browser must independently trust its issuer. TLS minimum is 1.3. All three TLS
variables are required together; omitting both OIDC variables enables HTTPS
without login. A partial configuration or failed discovery aborts startup,
never falls back to HTTP. No automatic HTTP redirect/listener is started.
Register the exact callback URI in the operator-selected confidential OIDC client,
which must support S256 PKCE and the library's RS256/ES256 signed-token contract.
The identity provider is verified against system CA roots. For an independently
trusted private IdP CA, optionally set `ZPR_OPERATOR_OIDC_CA_FILE` to a protected
PEM file (at most 64 KiB); its certificates are added to system roots. The same
owned client verifies discovery, token exchange, and JWKS. No insecure TLS option,
environment proxy, or redirect bypass is used. A CA file without OIDC configuration
is a startup error.

Files must be regular, single-link, root/current-user-owned, and not writable by
group/others. Key and secret require no group/other permissions (normally 0600).
Final symlinks and special files are rejected. Operators must protect the containing
directories and their ancestors; file permission checks are not an adversary boundary
against root/the owning user or a writable ancestor. Certificate/key/config are
bounded to 64 KiB each; secret to 4 KiB, one nonempty line with optional final newline.
OIDC JSON rejects unknown fields and trailing documents. Configuration/grants,
secret, and certificate are startup snapshots; rotate them with a deliberate restart.

`GET /auth/operator/config` reveals only whether login is configured, not provider
configuration or secrets. The GUI shows Sign in, a verified subject/scope after
callback, and CSRF-protected Sign out. Session/CSRF data remain in memory, never
browser storage. Session expiry is rechecked on page load/window focus; logout is
not reported successful without HTTP 204. A login/config failure is shown explicitly.
The signing/trust configuration below covers only enrollment; it does not
delegate unrelated monitoring/editor operations or replace their separate
operator-policy and loopback requirements.

### Local development operator login

The local stack can now run a real Dex identity provider, **not** the test
login fixture or the certificate-only browser gateway. Both listener ports are
published only on host loopback. The dedicated development CA is independent of
the machine-control and backend-service CAs. Do not deploy this local static-user
profile remotely or treat it as a production IdP with MFA, account lockout or
managed password recovery.

First prepare the ordinary local stack, including its existing service/client
certificates and runtime directory. Then, for a fresh operator profile, run from
this dashboard directory:

```sh
sh scripts/local-operator-login.sh init dubnom great-lakes
sh scripts/local-operator-login.sh start
sh scripts/local-operator-login.sh trust  # macOS user login keychain; explicit trust change
```

`init` creates protected files under `../../.local-runtime/operator-login`,
refuses to overwrite an existing identity/configuration, and never prints
passwords, client secrets or private keys. `operator-password` contains the
random initial password; open it locally and keep it private. The IdP stores a
bcrypt hash and its signing/session state in persistent SQLite storage.
The local image built with `scripts/Dockerfile.operator-idp` extends the pinned
Dex 2.44.0 image and uses HTTPS at `https://zpr-id.localhost:5556`;
Docker maps that hostname explicitly for the Control Room OIDC client.
The browser resolves `.localhost` locally. Certificates last 90 days and the
development CA 365 days; arrange deliberate renewal rather than disabling TLS
verification when they expire.

The local error template replaces Dex's "Requested resource does not exist."
error for expired login pages (including refresh) with **Sign-in timed out**
and one **Timed out. Try again.** action. That action opens the fixed trusted
Control Room URL; its normal same-origin login POST creates a fresh attempt.
No stale state/code or browser-supplied return URL is reused. Other provider
errors remain visible. The recovery target is Control Room, not Simulator.
New configurations select `frontend.dir: "/srv/dex/web"` and new IdP containers
build the custom image. Existing containers/configurations are not silently
replaced by `start`: adopting this template requires an explicit IdP image/
configuration rollout with the persistent SQLite database and identities
preserved. The local IdP and Control Room recovery handlers were explicitly
deployed on 2026-10-09; subsequent source edits still require a separate rollout.

The local admin is deliberately granted `organizations:["*"]` at Control Room
and independently at Control-Service: all existing/future organizations, but
**only the explicitly listed permissions**. There is no permission wildcard.
An exact issuer/subject still identifies the user; Dex encodes the stable
`zpr-local-admin` ID plus its local connector into the subject. The GUI displays
this opaque verified subject; the login username is `dubnom`.
General Control Room organization context is explicitly `*`, requiring a global
grant. An organization-specific deployment should use an exact organization ID
and exact grant instead.

Enrollment remains limited to the separate Control-Service `enrollment.json`
organization/type/profile catalog. Initialization seeds `great-lakes`, laptop/
workstation/server and standard profile as explicit local operator configuration,
not from Simulator manifests. Add future enrollment organizations to that file
and deliberately restart Control-Service; global user grants automatically cover
configured organizations without allowing arbitrary unconfigured organizations.
Creation remains **default off** (`gui_invitation_creation_enabled:false`);
cancel capability requires the independent named cancel grant. No review grants,
credential issuance or adapter admission are enabled by setup.

The presence of `operator-login/stack.json` opts the launcher into this profile.
Restart Control-Service using its existing operator environment (retain configured
LDAP/admin/diagnostics settings), then run:

```sh
sh scripts/dashboard-stack.sh restart-control-room
```

The launcher mounts only the Room certificate/key, OIDC config/client secret,
public CA and delegation signer files into Control Room; it does not mount the
initial password, CA signing key or IdP signing database there. The IdP has
`unless-stopped` restart policy and startup checks trusted discovery before
Control Room switches. Configuration, IdP database and enrollment registry
persist across service/container restarts. Control Room's in-memory sessions do
not: sign in again after a Room restart.

When direct HTTPS operator login is configured, a signed-out user is sent
straight to the identity provider's login page through the same-origin native
POST login form. Failed or denied callbacks retain a retryable sign-in screen
instead of automatically redirecting again. Explicit sign-out leaves the user
signed out with a Sign in button, avoiding immediate single-sign-on reentry.
Protected API authorization responses trigger a session recheck; only an
expired session starts sign-in automatically. Permission denials and Visa
Service availability errors do not redirect to the identity provider.
Unconfigured local login retains its existing behavior; HTTP never starts OIDC.

Visit **https://localhost:8787**, choose **Sign in**, and enter `dubnom` in the
IdP's login field and the locally saved password. Do not use the old HTTP or
`https://127.0.0.1:8787` URL: the configured origin/callback is exactly localhost.
No HTTP redirect listener is provided. Login-bearing documents use
`Referrer-Policy:same-origin` so Chromium supplies Origin on native login POSTs;
callback/API responses retain `no-referrer`, and cross-origin referrers remain
suppressed. Editor mutations now use a shared memory-only CSRF request helper;
logout/session checks erase its token and no mutation is automatically retried.

Simulator remains at **http://127.0.0.1:8788** without named login in this local
profile. Activity and activation checks use private mTLS Control-Service access,
not Control Room browser sessions. Its new `SIMULATOR_OPERATOR_SERVICE_*` URL,
CA/server name and `SIMULATOR_OPERATOR_CLIENT_*` certificate/key are supplied by
the launcher. Explicit incomplete/non-HTTPS service settings fail rather than
falling back to unauthenticated browser access. Legacy `SIMULATOR_CONTROL_ROOM_*`
settings remain only for backward-compatible local callers.
`sh scripts/dashboard-stack.sh restart-simulator-ui` updates only the UI/container without stopping DNS/rig
containers; it refuses if the UI container hosts live socat relays that need a
planned relay restart. This does not switch the active organization.

Validate an explicitly deployed profile with:

```sh
sh scripts/test-local-operator-browser.sh
```

This checks actual Dex password rejection/login, global grants, authenticated
monitoring/catalog reads, native Origin, missing-CSRF denial, GUI configuration
Analyze, reload and logout in desktop/tablet Chromium. Container Chromium pins
the exact local TLS public keys instead of disabling certificate checking
globally; credential traces/screenshots are disabled. The user's browser uses
the installed dedicated CA. The integrated browser fixture also now exercises
the actual production security headers.

To remove user CA trust, run `security remove-trusted-cert` with the exact
`operator-login/ca.crt` path and remove that specific certificate from the login
keychain. Do not delete the identity database/keys or repeatedly rerun `init`
to reset a password; take a protected backup, deliberately replace the bcrypt
entry and protected password file, restart the IdP, then restart Room to invalidate
existing sessions. No credentials belong in source control, chat or deployment logs.

### Route-level Control Room and Simulator policy

Simulator organization activation prepares its Linux/amd64 trusted-service
helper before stopping the previous runtime. A stale or missing helper is built
atomically using local Go, or the pinned `golang:1.26-alpine3.22` Docker builder
when the Simulator runtime has no Go compiler. Builder failures leave the old
artifact and current rig intact. The fallback requires Docker access, the builder
image (or permission to pull it), and dependency download access on a cold cache.
Use `sh scripts/prepare-trusted-service.sh` to prepare it ahead of switching.

Without OIDC configured, both applications retain their existing local-stack
API behavior. No named operator session is required and
`ZPR_CONTROL_ROOM_ORGANIZATION_ID` may remain unset. Control Room still applies
its existing local/proxy origin checks and blocks public/private enrollment
routes; disabled login does not grant enrollment authority. Simulator retains
its existing handler boundaries and does not resolve organization context for
operator authorization in this mode. This is local development behavior, not
approval for unauthenticated remote exposure.

The Control Room API classifies monitoring, policy, and Gateway routes as
`monitor.read`, `policy.read`/`policy.analyze`/`policy.edit`, and
`gateway.read`/`gateway.analyze`/`gateway.edit`. With OIDC enabled, every API
route must have an explicit classification, the operator grant must include the
configured `ZPR_CONTROL_ROOM_ORGANIZATION_ID`, and mutations require the
session's same-origin CSRF proof. Unmapped routes fail closed. A missing
organization with OIDC enabled returns HTTP 503; set
`ZPR_CONTROL_ROOM_ORGANIZATION_ID` to the production organization authorized by
the operator grant rather than disabling authorization or using Simulator state.

Simulator accepts separate `ZPR_SIMULATOR_OPERATOR_*` TLS/OIDC settings and
requires its own confidential OIDC client with callback
`https://localhost:8788/auth/operator/callback`. Its cookie namespace is derived
from issuer, client ID, and callback, so signing into either app does not replace
the other app's browser session. Simulator routes use distinct organization,
scenario, directory, device-lifecycle, simulated-device-session, workload, and
simulator-control permissions. Organization IDs in route paths are checked
against grants; the organization catalog is filtered to granted IDs. Global
Simulator views are checked against the selected active organization. Simulated
device-user login is not operator login.

To enable Simulator OIDC through `scripts/dashboard-stack.sh`, set
`ZPR_SIMULATOR_OPERATOR_ORIGIN`, `ZPR_SIMULATOR_OPERATOR_CERT_FILE`,
`ZPR_SIMULATOR_OPERATOR_KEY_FILE`, `ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE`,
`ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE`, and
`ZPR_SIMULATOR_OPERATOR_TLS_CA_FILE`. The certificate, key, config, secret, and
optional `ZPR_SIMULATOR_OPERATOR_OIDC_CA_FILE` must be under the mounted
`.local-runtime` directory. The launcher defaults the trusted peer to the Docker
bridge gateway; override with `ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP` only when
the direct TLS peer differs. Readiness uses the supplied TLS CA, not an insecure
certificate bypass. Leaving the Simulator origin unset preserves the existing
loopback HTTP development mode.

These route checks are enforced at the web-app edge. General monitoring,
policy-editor, and Simulator operations do not yet carry signed user delegation
to their backend services; mTLS still identifies the service, not the named
operator. Enrollment has its separate signed delegation contract. Do not treat
the current edge policy as backend authorization for direct service callers.

Before deploying, complete operator-selected IdP registration and configure the
independent delegation keys, certificate pins, and grants below. Live GUI forms
still need registry-backed wiring and real-service browser acceptance. No running
service was restarted or identity provider deployed by this work.

### Independently verified named-user delegation

[internal/operatordelegation](internal/operatordelegation/delegation.go) defines
a deliberately narrow, non-JWT signed request format using standard-library
Ed25519. It is not a transferable session, OIDC access token, or unsigned
identity header. Keep the OIDC client secret, delegation signing key, TLS server
key, and Control Room mTLS client key separate.

On Control Room, configure a protected strict JSON signing file:

```json
{
  "version": 1,
  "audience": "https://control-service.example:8790",
  "key_id": "control-room-production-1"
}
```

Set `ZPR_OPERATOR_DELEGATION_SIGNER_FILE` to that file and
`ZPR_OPERATOR_DELEGATION_KEY_FILE` to one protected PKCS#8 Ed25519 PEM private
key. Both are required together and require direct HTTPS/OIDC already enabled.
Audience must exactly equal `ZPR_CONTROL_SERVICE_URL`, including explicit port;
no implicit origin aliases are accepted. Operators provision these keys
independently; no production signing keys are generated by this feature.

On Control-Service, set `ZPR_OPERATOR_DELEGATION_TRUST_FILE` to a protected
strict JSON trust configuration. Existing `ZPR_ENROLLMENT_CONFIG_FILE` and
`ZPR_ENROLLMENT_DATABASE_FILE` are also required:

```json
{
  "version": 1,
  "audience": "https://control-service.example:8790",
  "keys": [{
    "key_id": "control-room-production-1",
    "public_key": "REPLACE_WITH_BASE64URL_NO_PADDING_ED25519_PUBLIC_KEY",
    "certificate_sha256": "REPLACE_WITH_LOWERCASE_LEAF_CERTIFICATE_SHA256"
  }],
  "grants": [{
    "issuer": "https://identity.example.com",
    "subject": "stable-admin-subject",
    "organizations": ["production"],
    "permissions": ["read", "create", "cancel", "approve", "reject"]
  }]
}
```

The placeholder key/pin deliberately fail validation. Public key is the raw
32-byte Ed25519 key, base64url encoded without padding; the pin is the SHA-256
of the Control Room mTLS leaf certificate's DER bytes (not the TLS server
certificate or a CA). Control-Service verifies its existing mTLS chain *and*
the exact configured leaf pin for the asserted key ID. CA trust alone cannot
delegate a user. Independent exact issuer/subject grants determine backend
organizations/permissions; browser-side grants cannot expand these. Unknown
organizations and review permissions without an approval lifetime fail startup.
Issuer/subject audit encoding must fit the existing 256-byte principal limit.
Files follow the same ownership, permission, regular-file, and 64 KiB limits
as OIDC configuration. Grants/trust are startup snapshots; rotate by deliberate
restart. Operator administration remains independent of Simulator.

Browser callers use the existing versioned `/api/enrollment/v1/` contract,
an active same-origin OIDC session, and `X-ZPR-CSRF` for mutations. Control Room
checks its organization/permission grants, bounds request bodies to 8 KiB,
discards caller-supplied delegation headers, and signs only allowlisted enrollment
operations. It rewrites to a separate private
`/api/operator-enrollment/v1/` route; normal proxy access to that route is blocked.
Direct `/api/enrollment/v1/` certificate administration stays separate and never
accepts delegated headers as certificate authority.

The single canonical assertion binds version, key ID, exact audience, verified
issuer/subject, HTTP method, exact path/query, SHA-256 of exact body bytes,
issued/expiry nanoseconds, and a random nonce. Lifetime is at most 30 seconds;
issued-at may be at most two seconds ahead for clock skew. Keep both hosts'
clocks synchronized. The private service validates signature and bindings before
dispatch and consumes a hashed nonce in the registry's SQLite immediate
transaction. Replay tracking persists across restart and concurrent connections,
expires with the assertion, and is capped at 8192 live entries. Database errors,
capacity exhaustion, or duplicate consumption fail closed; there is no replay
cache fallback. A denied or failed operation still consumes its accepted assertion.

Creation/cancellation/approval/rejection reuse the existing registry/audit
transactions, attributing actions to `oidc:["exact-issuer","exact-subject"]`.
Review retains current-revision/key-fingerprint/deadline checks. Signing is not
credential issuance or adapter admission. Once signed, an in-flight assertion can
finish within its short validity even if the browser logs out; logout blocks new
delegation but does not retroactively cancel accepted requests. No automatic
mutation retries are introduced. If a response is lost, inspect registry state
before requesting another mutation; a token cannot be replayed.

The catalog retains `gui_mutations_enabled:false`: general GUI mutations/review
are not enabled. Invitation creation has a separate, explicitly configured
capability described below. A successful catalog read alone never authorizes
creation.

Tests include tampered method/path/query/body/signature, wrong certificate,
absent verification, wrong issuer/subject, expiry/future issue time, replay and
concurrent consumption, persisted replay after reopen, server-grant isolation,
direct-route separation, named audit attribution, and an actual HTTPS OIDC
login → mTLS delegated catalog/create/cancel → logout path:

```sh
go test -race ./internal/operatordelegation ./internal/enrollment ./internal/operatorauth
go test -race ./cmd/zpr-web-dashboard -run 'Operator|Delegation|ControlRoom|Enrollment'
```

### Control Room provisioning invitations and read-only registry

Provisioning > Adapters now offers a memory-only invitation worksheet and
local review dialog for name, owner, organization/type/profile, inventory
reference, and instruction recipient. **Review worksheet** submits nothing.
**Clear worksheet** and reload clear form values; no form values, CSRF proof, or
codes are saved in browser storage. Never paste an enrollment code or private key
into this form.

Without an authorized catalog, fields are explicitly unvalidated requests and
creation is locked. With configured HTTPS/OIDC login and independently verified
delegation, the form uses approved organization/type/profile selects; changing
organization clears incompatible choices. Catalog selections are administrative
claims, not verified device attributes. The page also loads organization-scoped
registry pages of up to 50 records.
**Next page** follows the service cursor; **Check again** reloads the catalog and
first page for the selected organization. **Details** reads a fresh record showing
revision, fingerprint, deadlines, and audit identity/reason; it never retrieves
an enrollment code. Navigation, session checks/loss, and logout clear registry
data and details. Stale reads are discarded. Invalid, denied, and unavailable
responses are distinguished from a legitimately empty page.

#### Opt-in invitation creation

Keep creation disabled in existing deployments (the default). To deliberately
enable it, add `"gui_invitation_creation_enabled": true` to the operator-owned
Control-Service `ZPR_ENROLLMENT_CONFIG_FILE`. Configure HTTPS/OIDC, signing and
independent backend trust as above, and give the exact issuer/subject `read` and
`create` grants for the intended organizations at **both** Control Room and
Control-Service. Apply the configuration through your normal deployment process;
no running service or configuration was changed by this implementation.

Control-Service returns `gui_create_organizations` containing only independently
authorized creator organizations when this flag is enabled. Older catalogs or
absent capabilities keep creation locked. The service also enforces this opt-in
on delegated POST creation, so changing the browser UI cannot bypass it.
Direct certificate-authorized creation remains unchanged by this flag.
The legacy general-mutation flag does not unlock anything.

1. Choose approved organization/type/profile and fill the remaining asset fields.
   Asset values must fit the backend limit of 256 UTF-8 bytes without line breaks.
2. Select **Create invitation**, review the exact asset and lifetime, and
   acknowledge separate authenticated code delivery. **Confirm creation** sends
   one same-origin CSRF-protected POST; double submission is blocked.
3. A validated HTTP 201 response shows the invitation ID, audited named principal,
   expiry, and code once in a dialog. Record the ID and deliver the code through a
   separate authenticated secure channel, never email or the installer URL.
4. Closing, navigation, session checks/loss, or logout erases the code from the
   page. Read APIs cannot recover it. No code is automatically copied, downloaded,
   emailed, or saved to browser storage. If it was not securely delivered, use
   authorized cancellation from fresh **Details**, certificate administration,
   or expiry before replacing it.
5. A timeout (20 seconds), lost response, unexpected status, or malformed/mismatched
   creation result is **uncertain**, not success or failure. No automatic retry
   occurs. Creation stays locked in this page until explicit registry
   reconciliation is acknowledged. Clearing the form, refreshing the catalog,
   or rechecking the session does not remove this uncertainty.
6. Check **all registry pages** for the organization and inventory reference.
   Resolve an existing unusable invitation by authorized GUI cancellation,
   certificate administration, or expiry before requesting a replacement. Reload loses the in-memory guard;
   it does not prove failure or make a retry safe. The registry's active-asset
   uniqueness remains the durable duplicate protection.

Approval and rejection GUI controls remain disabled. Creating an
invitation sends no email, reserves no admitted/live adapter, and issues no
credentials. No production signed package host or installer link is configured;
the retained package remains unsigned development tooling.

#### Authorized invitation cancellation

An administrator with `read` and `cancel` grants for the organization at **both**
Control Room and Control-Service can cancel an `invited` or `pending_approval`
request whenever it remains active. No additional feature flag or creation
opt-in is required. The catalog independently advertises
`gui_cancel_organizations`; missing/older capabilities keep the GUI locked.
HTTPS/OIDC, CSRF and independently verified delegation remain mandatory.

1. Open Provisioning > Adapters, select the organization, and find the invitation
   across all registry pages. Open **Details** for a fresh service read.
2. Review the exact ID, inventory reference, owner, state, revision, key
   fingerprint (empty before claim), and invitation/approval deadline.
3. Enter a reason of 1–256 UTF-8 bytes without line breaks, acknowledge the exact
   invitation/revision, and select **Confirm cancellation**. The GUI sends one
   CSRF-protected POST with organization, revision, fingerprint and reason.
4. Control-Service independently authorizes the named user and checks current
   state, deadline, revision and fingerprint in one SQLite transaction. A
   concurrent claim/decision cannot cancel an unseen revision. Successful
   cancellation increments the revision and durably records the named decision
   principal, reason, time and audit event. The code and pending submission can
   no longer be used.
5. Confirm the returned cancellation summary or reopen **Details** to read back
   its audit fields. Approved, rejected, cancelled and expired records cannot be
   cancelled through this workflow. This is not credential revocation.
6. Explicit rejection/conflict requires reopening fresh **Details** before
   another decision. A 20-second timeout, lost/malformed/mismatched response,
   navigation, dialog closure, or session loss during a request means
   **outcome uncertain**, not failure. No automatic retry occurs.
7. Reopen fresh **Details** and inspect state/revision/decision fields. Select
   **Acknowledge refreshed outcome**, then reopen again before a new cancellation
   if still active. An in-flight service operation may finish after a read; a new
   operation remains revision-bound. Refresh/session checks preserve the
   page-memory uncertainty marker. Reload loses it but does not prove failure.

Delegated cancellation now requires the checked body:
`{"organization":"…","revision":2,"key_fingerprint":"…","reason":"…"}`.
Direct certificate-authorized cancellation retains its existing
`{"organization":"…"}` body and idempotent behavior. Neither path reads or calls
Simulator. Real HTTPS/OIDC/mTLS/SQLite browser tests cover invited and pending
cancellation, named audit and committed-but-lost response readback with Simulator
unavailable.

Browser acceptance checks run without Simulator:

```sh
sh scripts/test-browser-container.sh dashboard.spec.mjs provisioning-live.spec.mjs \
  --grep 'GUI operator|GUI [Pp]rovisioning|live provisioning'
```

The real HTTPS browser fixture verifies opt-in creation through OIDC, CSRF, mTLS
delegation, and SQLite, named audit, secret-free readback, and a committed but
lost response without a retry. It does not certify device installation or
credential issuance.

# Compiling the binary

To compile the final binary, run `make all`. The binaries will be located at bin/zpr-dashboard and bin/zpr-sim.


## Running it

Copy `config.toml.example` to `config.toml` and edit it.


```sh
make run                                # Start admin panel
```

The same lifecycle starts a separate simulation operator site at
`http://127.0.0.1:8788`. It is intentionally simulation-only and provides
manifest-backed agent/service inventory, a fixed fleet of 20 machine profiles,
workload placement, and recent managed-service logs. The fleet has 12 laptops
and 8 desktops; machine type, model, location, owner, and secure posture are
seeded into the local demo LDAP directory when the disposable Linux rig starts.
Its Activity page is a live view of recent Control Room visa and denial data.
It shows request/approval/denial totals, sortable recent-visa and denial tables,
and a manual Refresh beside the stream status. It is an operational feed, not a
durable audit archive. **Workers** merges passive device/runtime inventory and
workload logs, with device-type filtering. Control Room remains the read-only network and policy monitor at
`http://127.0.0.1:8787`.

The Control Room sidebar lists Map, Status, Security, Diagnostics, Trusted
Sources, Adapter Logs, and the external Log Manager (marked with a green ↗
arrow). Policy/Assertions, Gateways, and Config follow under a small
**Configuration** label; the label is hidden in condensed and mobile layouts.

The top header uses concise **Control Room** and **Visa Service** titles.
Visa Service uptime is on its secondary line; healthy states do not repeat
"Available" or "connected". Independent green/red lamps distinguish Control Room
transport from Visa Service source health. Partial, unavailable, unconfigured
and snapshot-transport failures remain explicitly labelled; transport failure
clears the displayed uptime until a successful snapshot restores it.

Control Room groups Adapters, Actors, Services, Visas, Denials, and DNS under
counted Status tabs. The summary metrics stay on Map rather than repeating on
each status page. Map updates animate retained topology components as bounds
move; Fit centers the rendered topology with padding while preserving glyph
proportions, and reduced-motion preferences suppress movement animation.

Selecting an adapter in the Control Room map shows its current unexpired visas,
including inbound and outbound flows, protocol, expiry, node, and policy ID.
The panel refreshes with polling and queries the complete Visa Service list,
not the recent-ten snapshot. These are current grants involving the adapter's
ZPR address, not confirmations that its PH has installed each grant. Loading
and upstream failures are distinguished from an empty current-visa list.
Zero visa and buffered-denial badges are white-filled circles; `?` means the
count is unavailable. Click, right-click, or press Enter on a red node denial
badge to show the node's buffered and local denial telemetry plus recent Visa
Service denials whose source is an adapter docked on that node. The node exports
only a count, so these records are context and may not match it one-to-one.
Refreshes keep the last successful visa list visible, including after a failed
request; switching adapters still loads that adapter's own list. Visa flows in
the inspector and Visas table show matching DNS names alongside the original
addresses, including aliases from the configured `svc.zpr.` zone. The zone
records are cached and refreshed once per minute during normal polling;
unmatched addresses remain numeric, and DNS failures retain cached names.

The policy editor offers compact completions beside the cursor. Inside a
`define` statement's `with` clause, the same menu includes configured attributes
and their sources, including multiline clauses. Suggestions are suppressed
after terminating periods and inside comments, quoted strings, and attribute
values. Control-Space requests suggestions, arrow keys select, Tab accepts,
and Escape dismisses; Enter remains a newline and Shift-Tab moves focus out.

Source editors offer a **Find & Replace** button immediately before History:
policy/assertion, ZPR Config, Simulator scenario source, and directory LDIF. It
turns dark green while its dialog is open and closes the dialog when clicked
again. The dialog follows Microsoft Word's Find and Replace layout: **Find** and
**Replace** tabs, *Find what* and *Replace with* fields, and Replace, Replace
All, Find Next, and Cancel buttons. **More >>** reveals Search Options: search
direction (All wraps; Down and Up stop at the end or beginning), Match case,
and **Use regular expressions** (Word's wildcards are replaced by JavaScript
regular expressions). Control/Command-F opens the Find tab in the focused source;
Control/Command-H opens the Replace tab. Enter is Find Next, Shift-Enter
searches backward, and Escape closes the dialog.

Searches are literal unless regular expressions are enabled. Regular-expression
mode reports invalid patterns, ignores empty matches, and expands `$&`,
`$1`–`$99`, `$<name>`, and `$$` in replacements; literal mode keeps replacement
text literal, including `$`. The status shows only match counts and position
(for example, `Match 2 of 5`). Replace or Replace All is applied as one native
edit, so the editor's Undo/Redo (Control/Command-Z and Shift-Control/Command-Z)
restores it. Replacements change only the unsaved source, preserving each
editor's normal modified state and analysis invalidation. Nothing is saved,
published, or applied automatically; replacements exceeding an editor's
character limit are rejected.

ZPR Config uses the policy editor's syntax colors and toolbar styling. Its narrow
gutter contains only source-local error markers, not line numbers. Analyze shows
success/error state on its button, and File contains Save draft and local file
commands. An empty new draft has no redundant footer; actionable diagnostics and
save/load status remain visible. Saving configuration still never applies it to
the runtime.

The Simulator scenario dialog switches between its structured form and a
top-right raw-source editor toggle. Raw mode edits either JSON or YAML, using the
same dark syntax colors, error-only gutter, File/Analyze/Format controls, and
synchronized scrolling as Control Room's source editors, with a line/column
indicator and a responsive assistant column. Save and Publish store the same
canonical scenario definition regardless of the chosen raw format.
Analyze calls the read-only, organization-scoped
`POST /api/simulator/organizations/{organization}/scenario-check` endpoint. It
accepts `{format:"json"|"yaml", source}` and returns the canonical JSON/YAML
rendering when valid. It uses the normal scenario validator but never opens a
workspace, saves, publishes, or starts anything. Syntax/type errors with exact
source offsets appear in the gutter; definition errors without source locations
remain in the status area. Form and raw-source changes round-trip, and invalid
JSON/YAML blocks returning to the form rather than discarding the draft. File
import/download remains local and unsaved.

The same Claude assistant is available in both modes and uses the current form
or exact raw draft. Its disclosure explains what is sent to Anthropic; the
server still requires `ANTHROPIC_API_KEY`. Run
`scripts/configure-assistant.sh` from an interactive terminal to store it in a
0600 file under `.local-runtime/dashboard-stack/assistant/`; the existing
Control-Service container is restarted in place to load it, preserving its
LDAP, Admin API, diagnostics, enrollment and organization configuration.
Setup refuses to reload a service configured for a different key-file path.
Simulator assistant requests read the protected
file at request time. This avoids shell exports that disappear after a
terminal closes. Applying a proposal updates only the unsaved editor.
Responses/proposals for a changed draft or reset conversation cannot overwrite
newer work. Save, Publish, and Run remain separate explicit operations.

All editor and design assistants share `assistant-core.js`: ready-to-use
controls without an opt-in checkbox, initially collapsed **Model and max
tokens** preferences, the **How can I help?** prompt, bounded conversations,
usage counters, pending/errors, safe text rendering and pane collapse.
Text suggestions are undoable with Ctrl/Command+Z (with redo and a fallback
edit history when native editing is unavailable). Structured Scenario and
organization-directory Apply offer **Undo AI change**/**Redo AI change**,
restore unsaved drafts only, and reject history actions after conflicting
source, record or revision changes. Applying never saves, publishes, activates
or runs anything.

Policy/Assertion context consists of the selected source/group definitions,
configured trusted-attribute definitions and operator-written conversation.
It does not load a People catalog, individual user records, memberships or
Simulator state. A regression exercises both endpoints with unavailable
Simulator configuration. Directory and Scenario request payloads are unchanged.
In production, Control-Service obtains Policy/Assertion attribute definitions
from Policy-Service over its configured mutual-TLS connection rather than
requiring an in-process policy workspace. Only attribute definitions enter the
assistant prompt; unrelated repository records are not forwarded. Context
failures are reported explicitly and prevent the model request. ZPR Config
does not need an attribute catalog lookup.

Rescan LDAP, Format, Discard, Evaluate & Test, Save, Save As, and Compile & Stage
share a responsive control strip directly above the source editor. Evaluate &
Test runs ZPLC before the identity simulation. Save and Save As automatically
rerun the simulation; if it fails, the user must confirm Save anyway. Compiler-invalid
source can still be preserved, but cannot be staged. Compile & Stage recompiles
the selected saved revision on the server before creating a candidate.

The Control Room Services page uses distinct type colors for BuiltIn, Regular,
Visa, Gateway, trusted file, trusted REST, and services not in the current
policy. The upper-right **Type colors** dropdown shows the mapping and includes
stable colors for any additional reported types. Labels remain visible alongside
colors; the key stays open during polling and closes on outside click or Escape.
Map service badges use the same background, border, and text colors, including
while hovered or highlighted as arriving or exiting.

The IPv6 simulator policy also defines a ZPR `internet-gateway` service at
`[fd00:1:9::1]:8082`. Its identity uses a dedicated pre-generated bootstrap key,
and the trusted file service assigns its `public-internet` network label. The
Finance workload alone is allowed to use the service. The bundled
`internet-gateway-egress` scenario sends an HTTP health probe through ZPR to a
fixed `https://example.com/` upstream; it is not an arbitrary-host proxy. Rebuild
and restart the disposable Linux rig to load the updated signed policy and
gateway key before running this scenario.

In the Control Room map, each gateway is connected to a dark gray cloud
representing its external network. Hover over the cloud to see the declared
network label.

### Gateway draft API

Control-Service exposes `GET /api/gateways/contracts` to list live registered
Gateway services with a matching actor, and `POST /api/gateways/config/check`
to validate a JSON draft against one of those contracts. `GET /api/gateways/configs`
lists the active organization's saved drafts;
`GET /api/gateways/configs/{instance}` reads a draft and its revisions; and
`POST /api/gateways/configs/{instance}/revisions` saves a new revision with an
`expected_revision` compare-and-swap. Draft files are organization-partitioned,
private, and versioned. The Control Room **Gateways** page provides fields for
destinations, path prefixes, GET/HEAD methods, timeout, and response size, with
server validation before saving. All these operations leave runtime unchanged.
There is intentionally no Activate action yet.

The draft cannot set the external-network classification; that remains
policy-owned.

The Visa Service Admin API reports service kind from installed policy but does
not return the ZPL `external-network-connection` claim. Contract discovery uses
the policy-derived `Gateway` kind and a matching live actor; any response
external-network label is optional `ZPR_PLATFORM_SERVICES` display metadata,
not a verified ZPL claim. Do not use draft validation as an activation/egress
security boundary until Control-Service can verify the complete installed ZPL
contract and runtime DNS/egress checks are implemented.

## Policy Layers

Runtime policy is composed from three ordered source tiers:

1. `examples/policy-layers/bootstrap.zpl`: node, Visa Service, and essential
   bootstrap/administration grants.
2. `examples/policy-layers/platform.zpl`: shared simulator control, DNS,
   observability, and baseline fixture services/grants.
3. `examples/organizations/<id>/runtime-policy.zpl`: company application policy.

The composer always uses the same first two tiers and selects one company tier.
It rejects missing/empty tiers, duplicate sources, path traversal, symlink
escapes, and oversized bundles. This is source composition, not a new precedence
or authorization model: the normal compiler and evaluator still determine
policy semantics. The company runtime layer is separate from its Policy Studio
example catalog; Redwood's illustrative business endpoints are not deployed by
its login-only runtime scenario.

Generate a candidate without changing the running network:

```sh
go run ./cmd/zpr-web-dashboard -mode compose-policy \
  -policy-root cmd/zpr-web-dashboard/examples \
  -policy-organization velocity \
  -policy-output /tmp/velocity-runtime.zpl
```

Compile/sign the generated source with the complete runtime `.zplc` configuration
and the normal protected signing key, not the company's editor-only config.
Northstar, Redwood, and Velocity candidates have been compiler-checked. The
running monolithic rig policy is not replaced by this command. Layer-aware
activation must still coordinate actor/visa reset, company LDAP, Policy-Service
workspace selection, health checks, and DNS-data retention before live rollout.
Control Room consumes generic Policy-Service APIs; it must not call simulator
APIs to select a company or orchestrate reset.

## Simulator Policy Source

Control Room's Policy tree includes **Simulator / Runtime / Simulator runtime
policy** when the local stack configures `ZPR_POLICY_SOURCE_FILE`. This is the
simulator ZPL source, separate from the editable Northstar demonstration catalog
and from acknowledgement of a signed policy installed in Visa Service.

The configured source is imported even when the repository already has records.
Importer-owned records gain a revision when the file changes; identical reads
are idempotent. Manual edits to the imported record and unrelated policies are
never overwritten. `ZPR_POLICY_SEED_CATEGORY` and `ZPR_POLICY_SEED_NAME` control
its location. This view does not install or activate a policy.

## Log Views

Log panels retain each source's last successful bounded tail when an adapter,
controller, application, or service disconnects. The selected source remains
available, its panel turns white, and a disconnect/read-error status appears
without replacing the log text. Reconnection replaces the tail with fresh data
and restores the running style. Retained tails are browser-memory only, scoped
to the active organization, and cleared when organizations change or the page
is reloaded; they are not a durable archive.

Control Room's **Adapter Logs** page shows the manifest fleet with Controller,
Control adapter, and assigned-workload adapter logs. Its read-only API passes
through the authenticated Control Service to the simulator's adapter collector.
The **Adapter logs** and **Controller logs** buttons at the top switch log types
using the same horizontal panels. Each panel remembers its adapter/controller
selection and source scroll/follow state; changing types needs no additional
collection request. The machine/adapter selectors and add/remove panel controls
apply to the selected type.
The Simulator's **Workers** page shows passive device, controller, session and
selected workload status together with assigned application/service
event logs, not Controller or adapter logs. Running machines with no supported
workload logs have an explicit empty state.

Adapter/Controller Logs and Workers offer **Format JSON**, initially
unchecked. Enable it to indent complete JSON objects/arrays; disable it to restore
the exact raw log text. This is a local display change, including while paused,
and does not collect or modify logs. Plain text, prefixed messages, and malformed
or truncated JSON remain unchanged. Entries nested beyond 64 levels stay raw to
bound indentation growth. Formatting retains number tokens (including
large integers), duplicate keys, and escaped string contents. Wrap is independent.

Both views poll two seconds after each collection completes, without overlapping
requests. Pause/resume, manual refresh, search, running-only filtering, and
follow-tail controls operate independently of machine lifecycle actions.
Each machine window follows the newest entries by default. Scrolling back pauses
following for that window without affecting the others; scrolling to the bottom
resumes it. Re-enabling **Follow logs** resumes all windows. The Simulator's **Log**
radio controls switch between its workload sources, showing one at a time.
Selection survives polling, and
each source remembers its scroll position and follow state when switching back.
The selector also works in maximized windows; a disappeared source falls back
to the first available source.
Polling is cancelled when navigating away from the page. ANSI foreground and
background colors (standard, bright, 256-color, and RGB), bold, faint, italic,
and underline are rendered using the vendored MIT-licensed `ansi_up` 6.0.6
browser module. Log text is HTML-escaped, terminal hyperlinks stay inert, and
only approved color and text-emphasis styles are applied without relaxing CSP.
Diagnostics log bodies use the same `colored-log.js` renderer as
Adapter/Controller Logs and Workers, preserving provider-supplied ANSI colors
and emphasis instead of showing escape sequences as plain text. Each Diagnostics
record starts with fresh terminal styling, so color cannot leak into the next
record. Diagnostics displays provider text without JSON reformatting and has no
Format JSON control. Provider HTML
and terminal hyperlinks remain inert. No severity colors are invented and
plain messages retain the existing Diagnostics appearance.
Diagnostics log entries show only the full-width body, without separate
timestamp or severity columns. Dates, times and levels already included in
provider text remain intact; no prefix is synthesized or stripped.
Diagnostics uses the available application width, including wide monitors.
Metrics span their detail panel, and source tables/logs grow naturally with
their bounded response data rather than using fixed-height nested scroll panes.
The page scrolls vertically; narrow source tables can still scroll horizontally.
An initial **Loading** indicator is shown until the first request finishes.
Diagnostics does not display a recurring Querying/Updated status line;
background refresh retains existing content, with failures shown explicitly.

Each source returns at most 100 lines and 64 KiB. Collection runs for at most
12 seconds with four concurrent machines; empty and unavailable sources are
shown separately. Adapter Logs reads Docker controller output and PH log files;
Workers reads application/service JSON event files. Neither view generates
traffic or starts log-producing
workloads. Recognized credentials are redacted before sending logs to the
browser. This is a bounded live tail, not a durable audit archive.

## Trusted Data Assertions

Control Room's **Assertions** page edits a per-organization, report-only assertion
record in the Policy Repository, independent of ZPL compilation and access policy.
It evaluates live trusted LDAP membership data with
group cardinality, exact-one membership, mutual exclusion, and approved LDAP
attribute presence/value/integer checks. Manual draft
evaluation and opt-in periodic checks are available; source failures and missing
groups are errors, never successful checks. See [ASSERTIONS.md](ASSERTIONS.md)
for syntax, source configuration, API contracts, and current limits.
Source-located assertion lint warnings appear as clickable yellow `WARN`
markers on their source lines; clicking a marker opens its details. The Analyze
button turns yellow when a successful evaluation includes warnings. Warnings
without a valid source line remain visible below the editor controls rather
than being assigned an invented line.

## Organization Activation

Selecting a profile does not activate it. **Activate organization** opens a
warning naming the current and target organizations: activation resets the
simulated ZPR environment and can interrupt connections and workloads. Cancel
is focused by default; Cancel or Escape sends no activation request. Only
**Switch organization** approves the reset. If the active organization changes
while the dialog is open, cancel and review the new state before trying again.
The backend still requires scenarios to be finished/cancelled and machine users
to be logged out.

**See Logs** appears on the Simulator Organizations page only when the latest
activation or base restore reports a failure. It opens the local reset log in a
separate tab. The link disappears when a retry is resetting, completes, or returns
to idle; a catalog refresh failure retains access to the last known failed reset.

## LDAP Organization Graph

The simulator's Organizations page uses the reusable `<ldap-org-graph>` custom
element from `cmd/zpr-web-dashboard/static/ldap-org-graph.js` and its scoped
stylesheet `ldap-org-graph.css`. Load both assets and assign a directory profile:

```javascript
const graph = document.createElement("ldap-org-graph");
graph.directory = {
  base_dn: "dc=example,dc=test",
  departments: [{ name: "Engineering" }, { name: "Platform", parent: "Engineering" }],
  people: [{ uid: "alex", name: "Alex", department: "Platform", title: "Engineer" }],
  groups: [{ name: "Operators", members: ["alex"] }],
};
container.append(graph);
graph.addEventListener("ldap-node-select", (event) => console.log(event.detail));
```

Each instance owns its camera, search, and selection. Reassign `directory` to
replace the data. Department names are unique parent keys and group members use
person UIDs. Missing parents and cyclic department references attach to the
directory root. The +/- control on a directory, department, or Groups branch
collapses or expands its descendants and compacts the layout. Expand all and
Collapse all operate across branches; Left/Right arrow keys collapse or expand
the focused branch. Selection and search survive branch changes.
Clicking a node opens a component-info popup with its profile attributes,
including structured values. Close it with the close button, Escape, or a
click outside the popup; focus returns to the selected node.
People list their groups, and groups list member names and UIDs. Selecting a
person or group displays dashed membership edges to visible nodes; memberships
remain listed when related branches are collapsed. Node labels and attributes
are text, not interpreted HTML. The graph
shows organizational relationships from profile data, not a live LDAP query or
an LDIF parser, and does not imply that people are physically stored beneath
their department DN. It supports keyboard node selection, pan, zoom, and fit.

## Browser Access Gateway Prototype

The optional gateway is a loopback-only development prototype. It routes
`control.localhost` to `127.0.0.1:8787` and `simulator.localhost` to
`127.0.0.1:8788`; upstreams must remain loopback origins. LDAP and OpenObserve
remain separate local-only tools.

With the default hostnames and port, use `https://control.localhost:8443` for
Control Room and `https://simulator.localhost:8443` for the Simulator.

The prototype requires a server certificate/key and a dedicated browser-client
CA. Do not reuse the machine-control CA. Start it on loopback for controlled
development without restarting the rest of the stack:

```sh
ZPR_ACCESS_GATEWAY_TLS_CERT_FILE=/path/to/gateway.crt \
ZPR_ACCESS_GATEWAY_TLS_KEY_FILE=/path/to/gateway.key \
ZPR_ACCESS_GATEWAY_CLIENT_CA_FILE=/path/to/browser-client-ca.crt \
scripts/dashboard-stack.sh start-browser-gateway
```

The listener defaults to `127.0.0.1:8443`. Browsers must trust the server
certificate and present a client certificate issued by the configured client
CA. This authenticates a client certificate, not a named user; the prototype
has no OIDC, role authorization, user sessions, or per-certificate revocation.
Do not set `ZPR_ACCESS_GATEWAY_LISTEN` to a non-loopback address or publish an
administrative UI. Public access is deferred until the [Remote Browser Access
Contract](BROWSER_ACCESS_CONTRACT.md) is implemented and its release gates pass.
Use `scripts/dashboard-stack.sh stop-browser-gateway` to stop the local
prototype independently.

The Simulator opens on Agents. Shared ZPR actors, links, and service inventory
remain in Control Room; Agents provides per-machine controls without stack-wide
lifecycle buttons.

The local stack also starts the BIND 9 DNS container from `../dns/bind9` using
The local stack also starts the BIND 9 DNS container from `../dns/bind9` using
the adapter1 ZPR network namespace. It expects the publisher TSIG key and
`svc.zpr` zone file under `.local-runtime/dns-bind/`. BIND's HTTP statistics
channel is loopback-only in the adapter namespace; a host-loopback `socat`
relay feeds the Control Room DNS tab and is removed when the stack stops.
The standard simulator's Exercise action resolves each component's FQDN target
through the bootstrap DNS server in the manifest, then pings the returned ZPR
address. Provider aliases are reconciled into `svc.zpr` by Visa Service using
the configured TSIG publisher key.
Use `scripts/dashboard-stack.sh start-dns` or `stop-dns` to manage DNS without
restarting the simulator's machine fleet.

The simulator defines 20 Docker machine profiles but initially creates only
`zpr-machine-01`, from the `debian:trixie` image. Starting another machine in
a scenario creates it on demand. Each running container has its own
Linux/ARM64 machine controller and PH adapter with a unique bootstrapped ZPR
identity. Machine adapters use dynamic `fd5a:5052::/32` addresses; the
simulator-control provider uses a reserved `fd5a:5052:adda:1::/64` address so
it is reachable through the PH-advertised overlay route.
Controllers reach the TCP `SimulatorControlService`
(`[fd5a:5052:adda:1:ffff:ffff:ffff:fffe]:8792`) over
ZPR. The service is published by the dedicated `simulator-control` actor and
only the 20 machine identities are authorized to access it. The service
gateway forwards the mutually authenticated TLS protocol to the simulator's
local listener; the ZPR adapter link and per-machine TLS client certificate
are both required.

Simulator mode runs in the Linux `zpr-simulator` container, built from source
with a containerized Go toolchain. Its UI is published on `127.0.0.1:8788` and
its mTLS machine-control listener on `8791`; the Docker socket is mounted so it
can manage machine containers. Use
`scripts/dashboard-stack.sh restart-simulator` to rebuild and replace only
Simulator mode without resetting the active ZPR rig or organization.

On each machine start, the Simulator installs a host route for that machine's
assigned ZPR address through the control adapter. This avoids ambiguous return
routing when multiple adapters advertise the same overlay prefix. Startup
also removes stale link-down ZPR routes. The simulator-control adapter uses the
Visa Service's default four-hour adapter authentication lifetime; if its actor
expires, refresh only that adapter with
`scripts/dashboard-stack.sh restart-simulator-control`.

Machine Login/Logout writes/removes the selected simulated user in that
machine container at `/run/zpr-simulator/user`. It does not start or stop
workloads. User choices come from the active Organization's people and
machine-owner assignments; organizations without overrides retain the
manifest-owner behavior. The same rule applies to scenario logins. Sessions
are simulator state and reset when the stack restarts. This is not a
password check or ZPR user authentication: controller mTLS is a separate
machine identity, and the existing BAS auth-code flow authenticates an adapter
key rather than a human.

Workers is read-only: its expandable device details show owner, posture,
container/controller state, authenticated user and selected workload identities.
The former Agents page redirects to Workers at `/machine-logs.html`, and the
navigation has one Workers entry rather than separate Agents and Workload Logs.
Word wrap is initially disabled and can be enabled in editors and log viewers.

All six source editors register a shared `editor-page.js` controller with
explicit load/save/render adapters and Analyze adapters where supported.
Scenario form/raw modes share one controller; Assertions reuse Policy's
File/History controls without duplicating their listeners. Controllers own
identity/status, scoped analysis ownership, command and Save-shortcut bindings,
and responsive source-frame setup. Domain-specific service contracts,
cancellation, diagnostics, confirmations and mutation handling remain separate.
Disposal invalidates requests, removes owned listeners and releases layout
observers; obsolete request cleanup cannot release a newer operation.
All six source editors share Ctrl+S / Cmd+S handling through `editor-page.js`.
Their title rows also use its shared identity renderer for Untitled, safe title
text, path tooltips, and Modified visibility. Each editor retains its own dirty
calculation and draft naming; version information remains in History.
Discard, record/history switching, and editor-close checks use a shared
confirmation guard. Clean source does not prompt; cancellation retains the
current draft. Warning text and reset actions remain specific to each editor.
File menus also share one controller: opening focuses the first visible, enabled
action; arrow keys wrap through available actions and Home/End select the first
or last. Hidden and disabled actions are skipped. Escape closes the menu and
restores focus to File; selecting an action or clicking outside closes it.
Menu arrow keys do not intercept typing/navigation elsewhere in the editor.
History uses the same shared dropdown and safe version renderer in all six
editors, including singular/plural counts, empty histories, current-version
markers, outside-click dismissal and Escape focus restoration. Each editor owns
its revision-loading callback and discard checks; selecting another editor's
version never also invokes the Policy revision loader.
Policy, Assertions, Config, Gateways and Scenario also share explicit Analyze
button-state transitions (idle, pending, success and error). The helper does not
change readiness classes, enable actions, create diagnostics or interpret
service failures. Request ownership and stale-response rejection remain with
each editor. Scenario's pending state can mean edits need analysis, not a request
in flight. Directory has no Analyze action.
Non-line editor errors appear below the control buttons and above the source
surface in all six editors, not below the document. Source-local diagnostics
remain in the gutter. Shared line validation accepts only positive integer
locations within the current source; missing, fractional or out-of-range
locations are never clamped onto a real line. Assertion service failures stay
visible in the error status instead of acquiring a line-1 marker.
Config Analyze/validation-before-Format, Gateway and raw Scenario analysis use
a shared generation/context guard.
Editing and then restoring the same text still invalidates the old request.
Late successes and failures cannot update validity, status or gutter markers
after the source or domain-owned record/revision/organization/format context
changes. The guard has no service dependencies and never retries mutations.
Config Save and other mutations retain their existing error handling; analysis
guarding does not hide mutation failures or imply a failed save succeeded.
Assertion evaluation also uses this guard alongside its existing abort
controller. Source edits (including Tab and catalog insertion), record changes
and organization/revision context changes prevent late results from populating
the current gutter, result table or cached record result.
Assertion Format uses the same source/context guard and a separately owned
abort controller. Late formatted text, warnings and errors cannot replace or
decorate another draft, including after edit-then-restore. Switching records
or leaving the editor cancels Format; obsolete cleanup cannot unlock a newer
operation. Current formatting is an undoable unsaved edit and dispatches normal
source input so analysis and assistant suggestions are invalidated.
Policy compiler checks also use the shared source/context guard, covering
record kind/draft identity, current and browsing revisions, and organization.
Source edits and context replacement cancel the compiler request; only its
own controller can clear pending state. Obsolete successes, warnings and
failures never attach to the current source or start runtime testing. Save
and Save As stop on cancelled checks while retaining the existing explicit
save-with-errors flow for current compiler failures. Runtime testing uses the
same context adapter with its own shared scope and abort controller. Both
fixture loading and evaluation reject obsolete successes/failures before
starting another request or updating dimensions, results, warnings and errors.
Source/record/revision/organization changes cancel both analysis phases;
obsolete pre-save cleanup cannot unlock a newer Save test or cache cancelled
tests as current failures. Cancelled tests stop Save, Save As and Stage
continuations. Current test failures retain explicit warnings and the existing
save-anyway confirmation. Production service contracts remain unchanged.
Structured Scenario analysis uses the shared generation/context guard with
the exact serialized form, scenario identity/revision, organization, viewed
revision and editor mode/open state. Form edits and replacement rendering
invalidate earlier requests, including edit-then-restore and identical
replacement forms. Only the newest Analyze may update status; closing and
reopening the editor cannot resurrect an old response. Raw Scenario checks
also include the viewed revision and editor open state. These adapters retain
Simulator-only endpoints and do not change Save, Publish or Run contracts.
Shared viewport sizing across all six editors returns an idempotent binding per source
container. Its `schedule()` coalesces layout requests into one animation frame
and skips hidden containers; `dispose()` cancels queued work, disconnects the
resize observer, removes window listeners and restores the original inline
height and priority. A disposed container may be bound again. The existing
viewport inset and minimum-height rules are unchanged; the binding has no
service or editor-domain dependencies. Controllers reschedule when frames open,
mode changes alter visibility or the viewport resizes; maximized frames use
their full-page sizing instead. Scenario and Directory remain nonmodal pages.
The shared editor-page foundation supplies Maximize/Restore to Policy,
Assertions, Config, Gateways, Scenario, and Directory editors. It fills the
viewport, keeps focus within non-modal editor pages, and locks background
scrolling. Scenario and Directory dialogs remain open when Escape restores
their normal layout. The maximized editor exposes modal semantics and makes
surrounding content inert; restoring returns focus to the toolbar control and
preserves any pre-existing inert states.
All editor and adapter/controller/Worker log window controls use the same
dependency-free SVG renderer in `safe-display.js`: an outlined window for
Maximize and overlapping windows for Restore, without visible button text.
The renderer also marks each window button for shared styling in `app.css`:
dark icons on a white 32px square, with identical hover, active, disabled and
keyboard-focus treatment across editor and log toolbars.
Action-specific accessible names, tooltips and `aria-pressed` remain available;
keyboard and focus behavior are unchanged. This applies to window sizing only,
not restoring archived records, revisions or organization base state.
All six source editors share `bindSourceLayout` for highlight-overlay scroll,
native gutter scrolling or translated result rows, resize observation and
horizontal-overflow detection where supported. Policy and Assertion retain
their own gutter bounds and diagnostic/highlight rendering; Policy retains
completion positioning on scroll. Assertion scroll/resize no longer rebuilds
highlighted token nodes. The returned binding (also exposed through source
surfaces) can disconnect scroll/resize wiring with `dispose()`; disposal
leaves rendered content and diagnostics intact.
The dependency-free `page-runtime.js` supplies neutral JSON transport and page
polling/lifecycle helpers. It has no endpoints, authentication defaults,
organization lookup or Simulator dependencies. Load it before editor-page,
assistant and page scripts. The existing editor JSON and Simulator navigation
poller entrypoints delegate to this runtime, retaining their public contracts.

`createPoller` requires `run` and an explicit `onError` callback. It shares an
in-flight Promise between refreshes, aborts on stop/pause/pagehide, and supplies
`signal`/`isCurrent` so late successes or failures cannot update another request
generation. Only the owning operation releases pending controls. Timer-based
reads wait the configured interval after completion; paused pages still permit
manual Refresh. `dispose` removes the pagehide listener and prevents restart.
Control Room snapshots, active-page DNS statistics, Diagnostics, Security scans,
adapter/Worker logs and manual Trusted Sources reads use this lifecycle.
DNS names remain a separately cached, page-independent production read so other
operator pages can resolve addresses without depending on the DNS route.
Security retains background inventory alerts while telemetry reads remain
active-page-only. Policy context checks use a separate navigation-owned scope.
Sample-age displays and animation timers are not network pollers.

Failed snapshot, DNS, Diagnostics and log reads retain last-good data with
explicit unavailable/error states; logs retain disconnected tails. Directory
reads retain last-known records, and normal snapshot polling/navigation does
not automatically reread LDAP or collapse its expanded tree. Change-feed
cursors remain source-owned. Failed/expired feed reads hide unconfirmed history
and keep the feed identity/error visible, rather than presenting it as current.

`requestJSON` receives the caller's fetch function and request options. Production
reads inject authenticated operator fetch; Simulator callers inject their own
fetch and endpoints. It preserves headers, bodies and abort identity, reports
malformed JSON with HTTP status, retains structured service errors, and makes
one attempt only. Assistant, Activity, policy browsing and policy File operations
also use it. Enrollment keeps its stricter content-type, status and uncertainty
rules, reusing only JSON decoding after those checks. Scenario Delete retains
its potentially empty success response. Operator login/session handling and
HTML navigation fetches retain their specialized contracts. No write is retried
or implicitly saved, published, activated or run by these helpers.

Config, Assertions and Gateways share `requestJSON`, with each adapter injecting
the existing authenticated operator fetch function and its own endpoint/options.
The helper preserves request bodies, headers and abort signals, never retries,
and retains structured service-error details and source locations. Malformed
JSON reports an explicit HTTP-status error rather than becoming an empty success
response; network and abort errors retain their identity. Gateway validation
explicitly accepts `valid: false` analysis responses, but failed saves remain
errors and leave the unsaved draft intact. Authentication, CSRF and mutation
uncertainty handling remain owned by the existing transport and editor adapters.
Simulator Directory now uses the same injected JSON helper for opening artifacts,
listing/loading revisions, saving drafts and publishing. Its adapter injects
Simulator fetch and keeps Simulator directory endpoints local to that page;
Control Room never imports those contracts. Malformed responses leave the
existing editor source/revision state intact and are shown through the existing
operation-specific error surfaces. Save and Publish remain single attempts,
and publishing still only affects the next explicit LDAP reseed or rig restart.
Scenario form/raw Analyze, artifact/history/revision reads and Create/Save/Publish
also use the shared injected JSON transport through a Simulator-local adapter.
Raw analysis maps structured service errors to validated source-line markers
only after checking request ownership; malformed JSON remains a visible,
unlocated error. Existing form/raw stale-context guards remain independent of
transport decoding. Failed writes leave the draft and revision identity intact,
never retry automatically, and never publish or run implicitly. Catalog polling,
Delete and Run retain their separate response/lifecycle contracts.
Policy compiler checks and runtime fixture/evaluation requests use the shared
JSON helper through authenticated operator fetch with their existing abort
signals. Compiler failures preserve diagnostics-before-error message priority
and source-owned warnings even on non-success HTTP responses; a non-success
response cannot become valid merely because its body says `valid: true`.
Runtime fixture and evaluation failures retain distinct error surfaces.
Malformed responses create explicit unlocated errors without invented markers.
Context guards and request-owned pending cleanup still prevent obsolete results
or cancelled Save/Save As/Stage continuations from changing current state.
Policy workspace, record, History and revision reads also use this authenticated
transport. Load errors appear in a dedicated status above source, separate from
compiler/runtime results; structured load errors never receive source markers.
Failed reads do not replace source text, and successful recovery or a new draft
clears the load status. Existing History selection and dirty-confirmation rules
are unchanged. Load-request lifecycle consolidation remains separate.
Policy revision Save, draft creation through Save and confirmed Stage also use
the authenticated JSON helper. Existing expected-revision bodies, Save error
dialogs and explicit Stage confirmation remain unchanged. Undecodable mutation
responses never advance local revision/draft state or trigger automatic retries;
they do not prove that the server did not apply the operation. Other record
management mutations retain their separate transport contracts.
The shortcut invokes the editor's existing enabled Save action, including its
name/revision checks and confirmations; it never publishes, activates, or runs.
Disabled Save still suppresses the browser's Save Page action. Held-key repeats,
IME composition, already-handled keys, and Alt/Shift-modified shortcuts do not
trigger Save. The Scenario shortcut also works in its structured form.
Start/stop, login/logout and workload assignment are controlled
by scenarios rather than page buttons. Selections are simulator control state
and a workload can be selected on only one logged-in machine at a time. Login/logout
and workload start/stop commands are queued by the simulator and executed by
the machine controller over its ZPR service connection; PH workload adapters
run inside the selected machine container. The selected workload remains
stopped until a scenario starts it. Docker container Start/Stop
remains a local host operation because an offline container cannot receive a
ZPR command. Stopping a machine stops its workloads and clears its
simulated login session; starting it restores its route but does not log a user
in or restart workloads. Controller connectivity is shown separately from
Docker state.

For local LDAP schema and seed changes, the runtime consumes
`../../.local-runtime/linux-integration/pregen/zpr-machine.schema` and
`demo-machines.ldif`; the 20 profile definitions are in
`../../.local-runtime/simulation-environment.json`.
## Control Room and Policy Service

The browser Control Room lives in `cmd/zpr-web-dashboard` and calls only
same-origin `/api/*` endpoints. The Control-Room process proxies those requests
to Control-Service using mutual TLS and holds no upstream API credentials.
Its topology draws only actors returned by the live admin API; service
registrations remain service badges attached to their provider actor, and a
missing provider does not create a synthetic adapter vertex.
Control-Service aggregates Visa Service Admin data and proxies Policy
Repository requests to a separate Policy-Service process using mutual TLS.
The Policy-Service owns its SQLite journal and ZPLC configuration. It stores
generic categorized records with immutable revisions; ZPL records are
compiler-checked before a version is written, and are not automatically
installed into the running Visa Service. Per-user policy-record authorization
is not implemented yet.

In the ZPL service model, attribute providers, authentication, logging/audit,
and the Policy Repository are distinct trusted-service classes with separate
REST contracts. The trusted-source view associates providers with actors and
reports the latest real lookup outcome when available; it does not browse
provider records.

Foundational services are registered during the extended node boot through the
Visa Service adapter. The boot passes their names in `ZPR_ADAPTER_SERVICES`;
the adapter includes repeated `zpr.services` claims in its real
`authorize_connect` request. Visa Service then persists those services against
the adapter actor and the Control Room reads them from `/admin/services`.

The simulation environment declares machine and runtime components in
`.local-runtime/simulation-environment.json`. Organization profiles under
`cmd/zpr-web-dashboard/examples/organizations/` own the LDAP base DN and seed
files, people, groups, machine ownership, policy compiler config/catalog, and
service catalog. Scenarios reference an `organization_id` and may declare an
independent `topology` of nodes, links, and component overrides. The active
profile is selected at stack startup with `SIMULATION_ORGANIZATION_ID` (or the
manifest's `organization_id`); it defaults to `northstar`. The Organizations
page includes an organization-scoped LDAP seed editor. Scenario definitions,
directory LDIF drafts, their immutable revisions, and existing policy records
share the per-organization SQLite file under
`.local-runtime/dashboard-stack/policy-private/`. The scenario editor provides
form-based metadata, repeatable step and cleanup controls, optional parallel
dependencies, advanced JSON for topology-specific fields, and virtual folders
grouped in the catalog; scenarios without a folder appear under **Unfiled**.
The run panel offers **Clear** only for a terminal run, and clearing removes
that run history without deleting its scenario or changing machine logs.
Drafts are validated and versioned; only published
scenarios can run, and each run records the organization and published revision
it used. Deleting a scenario archives it from the catalog while retaining its
immutable revision history. Directory edits are drafts until explicitly
published. Publishing stages a private LDIF at
`.local-runtime/published-directories/<organization>.ldif`; the running
directory is unchanged, and the new seed applies on the next explicit LDAP
reseed or runtime restart. All bundled organization profiles now use isolated
Docker Compose projects; single-node profiles keep one node, while Great Lakes
and Redwood retain their multi-node topologies. Activate another profile
through the Organizations page so its directory, policy, and services are
reseeded together. Set `SIMULATION_MANIFEST` to use a different machine/runtime
manifest.

The Simulator's Claude design assistant can review scenario drafts and
organization profiles. It keeps `ANTHROPIC_API_KEY` server-side and only applies
validated scenario or LDIF proposals to the open draft; saving and publishing
remain explicit operator actions. `scripts/configure-assistant.sh` stores the
key in a protected runtime file readable by both assistant services. Simulator
status and requests check that file dynamically; the existing Control-Service
container is restarted in place by the setup command, not recreated from shell
defaults. The stack forwards only the key-file path to
containers, not the key as a command-line argument.

Assistant setup requires a running Control-Service using the standard persistent
key path. `sh scripts/dashboard-stack.sh reload-assistant` reloads an existing
stored key without replacing the container configuration. Keep the private
`.local-runtime/dashboard-stack/assistant/` directory when cleaning runtime
artifacts; if its key file is deleted, the GUI cannot recover it.

The local stack's Policy-Service startup/restart selects an explicit
`SIMULATION_ORGANIZATION_ID` first, then the saved active organization, then
the bootstrap manifest. This keeps policy attributes aligned with the selected
local directory rather than reverting to the bootstrap organization on restart.

The reusable base contract lives in `.local-runtime/generic-zpr-base.json` and
is validated separately by the installer.

The manifest extends the generic ZPR base and describes the complete bootstrap:
required packages, Rust toolchain, ZPR artifacts, base trusted services, and
boot agents. A scenario adds services and actors on top of that base rather
than replacing the installation contract.

The containerized simulated environment installs the ZPR binaries with
`.local-runtime/build-and-run-linux-node.sh`; it validates and passes the same
manifest into the extended boot. A generic simulation uses the default
manifest, while a scenario-specific simulation sets `SIMULATION_MANIFEST`.

For the local runtime, configure and start the complete stack with
`cmd/zpr-web-dashboard/scripts/dashboard-stack.sh`. It builds one binary,
starts the services in dependency order, records their PIDs and logs, and
supports `start`, `stop`, `restart`, and `status`.

The individual process environment is documented in
[`cmd/zpr-web-dashboard/README.md`](cmd/zpr-web-dashboard/README.md) for
non-local deployments.

Open `http://127.0.0.1:8787` for the Control Room UI.
