# Local evaluation bootstrap (B1–B3)

Newly generated identity-provider configuration sets
`oauth2.skipApprovalScreen: true`: successful sign-in returns directly to
Control Room or Simulator without a **Grant Access** consent page. This does
not bypass credentials or application authorization. Existing installations
retain their generated configuration on restart; adopting the setting requires
an explicit identity-provider configuration rollout, preserving installation
state and its integrity records. Rebuilding the GUI alone does not change it.

## Network display name

Add `-network-name "Research Network"` to `demo.sh start STATE ...` or
`zpr-lab-bootstrap -evaluation -state STATE ...`. This optional, non-unique
label is passed only to production Control-Service. Control Room and Simulator
read it from that service; it is not an organization ID or an authorization
input. Omission leaves the network unnamed.

The option is runtime-only, not part of the frozen installation configuration:
changing it does not require a new state directory or reset trust, credentials,
policy, DNS, or enrollment state. Keep it in the foreground supervisor/launch
arguments on every start. To rename, change that argument and restart the
production role using the normal stop/start procedure; do not edit the protected
installation inventory. Refresh the interfaces afterward.

## Offline Linux/amd64 engineering package

The evaluation stack can also be built into a source-independent archive.
The builder creates matching ZPR and Simulator archives. This is **not yet
the complete sign-in-to-scenario demo**. The ZPR archive includes a prebuilt bootstrap, production-tagged
Control Room/Control-Service, the privileged data-plane image, and the exact
LDAP, phpLDAPadmin, logging and OIDC images. phpLDAPadmin is an evaluation-only
local directory editor, not a production operator UI. It shares the host
network namespace but binds its HTTP listener only to `127.0.0.1`; access it
through an SSH tunnel. The UI container receives only the public CA and its
configuration, never a directory password. It does not contain installation secrets:
credentials and identities are generated into the private target state.
The host needs Linux x86-64, Docker Engine with Compose v2, `sha256sum`, and
enough disk for the images and persistent state. No Go/Rust toolchain,
source checkout, image pull, or build is needed at installation.

On the build host, with Go dependencies cached and the four existing
Linux/amd64 service images already loaded, first build the phpLDAPadmin image
from the pinned Ubuntu package version, then create the offline pair:

```sh
sh ../environments/build-ldap-ui.sh
sh packaging/evaluation/package.sh demo-20261029 /absolute/new-output-directory
```

`ZPR_EVALUATION_PACKAGE_DATA_PLANE_IMAGE`, `ZPR_EVALUATION_PACKAGE_LDAP_IMAGE`,
`ZPR_EVALUATION_PACKAGE_LDAP_ADMIN_UI_IMAGE`,
`ZPR_EVALUATION_PACKAGE_LOGGING_IMAGE`, and
`ZPR_EVALUATION_PACKAGE_OIDC_IMAGE` may select different locally loaded images.
The builder records immutable image IDs, source revision/dirty status, toolchain
version and payload checksums. A failed build retains an `INCOMPLETE` marker;
do not distribute its output. Successful archives are unsigned engineering
artifacts, not production releases. Validate the archive SHA-256 against a
separately trusted checksum before extracting it into a new private directory.
Then, from the extracted package:

```sh
./demo.sh verify
./demo.sh load
./demo.sh start /absolute/new-private-state -simulator-port 18788
```

`start` runs in the foreground. Ctrl-C stops this installation and retains its
state. Restart with the identical state path and configuration flags; never
delete retained state to resolve startup problems. Generated credentials are
delivered through the existing evaluation workflow. Trust only `ca.pem` on
the browser computer, never copy CA keys or the whole state directory.
The default Room URL is `https://localhost:18787`. The `-browser-ip` option
below permits explicit private-network browser access on a **new** installation.
phpLDAPadmin uses `-ldap-admin-port` (default `18789`) and accepts the generated
directory administrator DN, but leaves its password blank for manual entry.
The host-network container can reach host loopback services; this is one reason
the entire evaluation package is restricted to isolated local development.
When testing alongside another installation, supply distinct ports at first
start using the flags documented below.

An independently configured evaluation may set `-organization-id northstar`
(or another lowercase alphanumeric/hyphen identifier of at most 64 characters).
The ID is frozen with the installation and scopes its operator identity,
service configuration, and subject-link resources. Omitting it preserves the
existing `evaluation-<install-id>` identity. An existing installation cannot
be rebound by changing this flag. This flag does not read a Simulator profile,
provision scenario machines, or supply an on-demand lifecycle.

### Independent operator service bindings

For an already initialized independent installation, optional
`-operator-services-file /absolute/private/operator-services.json` attaches
operator-owned Policy-Service and named policy review contracts:

```json
{
  "version": 1,
  "organization": "northstar",
  "policy_service": {
    "url": "https://localhost:20989",
    "client_certificate_file": "/private/northstar/client.pem",
    "client_key_file": "/private/northstar/client.key",
    "ca_file": "/private/northstar/ca.pem"
  },
  "change_review_config_file": "/private/northstar/change-review.json",
  "operator_oidc_config_file": "/private/northstar/operator-oidc.json",
  "visa_service_config_file": "/private/northstar/guarded-vs.toml",
  "visa_service_read_only_files": []
}
```

All inputs must be canonical absolute, regular protected files with owner-only
permissions; symlinks are rejected. The file is strict versioned JSON with the
installation's exact organization. Service credentials must form a valid mTLS
identity. Values are explicit production contracts, never inherited caller
environment or Simulator configuration.

The optional OIDC configuration preserves the retained issuer, callback,
client, session lifetime, named subjects and exact organization grants.
Only existing named policy editors may gain `policy.apply`, `change.read`,
`change.submit`, `change.review`, `gateway.read` and explicit `approval.self` for the shared review queue;
existing permissions cannot be removed. This config grants capabilities, not
approval. Policy staging, human review and authoritative receipt/readback are
still required. The frozen base OIDC configuration is not edited.

The optional `operator_delegation_trust_file` preserves the retained audience,
keys, certificate pins, named subjects, organization scopes and existing grants.
It may add only `approval.self`, only to an existing named approver. This is the
independent Control-Service half of self-approval; the current signed-in session
must also have the permission. Neither a backend grant alone nor the legacy
`allow_self_approval` review setting enables self-approval. No request is approved
by configuration or by login.

With an independent `enrollment_service` mTLS binding (the same URL/certificate/key/CA
shape as `policy_service`), existing named editors may also receive explicit
`read`, `create`, `cancel`, `approve`, and `reject` enrollment permissions.
The backend owns its registry and verifies end-to-end delegation against the
retained logical Control-Service audience; its private transport origin and
certificate remain independently verified. An explicitly configured development
catalog may pin the actual device listener's private CA for development preflight;
pilot/production ACME requirements are unchanged.

The Visa override requires data-plane mode and explicit guarded policy trust
for this organization's exact VS and Policy-Service origins. It can change only
`guarded_policy` and `trusted_service_http`: all other parsed TOML settings must
equal the frozen base configuration. Additional explicitly named regular files
are mounted read-only at identical host/container paths. Overlapping existing
mounts are rejected. The runtime image must support `ZPR_DP_VS_CONFIG`; absence
of these optional bindings preserves existing startup and mounts.

`environments/organization-policy-service.sh` supervises Policy-Service as a
separate process with a private per-organization repository, staging directory,
operation ledger and exact mTLS listener. It does not launch or read Simulator.
The current engineering service executable uses the dashboard's existing
`-mode policy-service` implementation, not a separately pruned build target.
The retained bounded installation policy is labeled as such when seeded; it
is not an original scenario policy and is not automatically applied.

### Runtime-script-only image updates

For script fixes that do not change PH, VS, compiler or CLI binaries:

```sh
sh packaging/evaluation/data-plane/build.sh \
  -tag zpr-evaluation-data-plane:operator-update \
  -runtime-base-image sha256:EXACT_LOCALLY_LOADED_IMAGE_ID
```

The builder verifies the local Linux architecture/image ID, assigns an exact
content-derived local base tag, and builds with pulling disabled. The child
copies only the four runtime scripts and records the parent ID in
`org.zpr.evaluation.runtime-base`. It does not rebuild or qualify the retained
binaries. Inspect and pin the resulting local image ID before deploying.
Omitting the flag retains the normal full source build.

The VS namespace routes the `fd5a:5052::/32` ZPR address space through its
adapter in both single- and multi-node installations. A docking-node route
alone cannot reach policy-authorized BAS or trusted providers in other ZPR
subnets. This route supplies reachability only; signed policy and provider
TLS verification still authorize the connection.

The package enables Visa Service, the data plane, protected DNS and three
bootstrap-admitted endpoints. Endpoint admission is an evaluation shortcut,
not human authentication or successful subject-link publication. Control Room
includes one Approvals navigation entry; its current queue handles subject-link
approval only, not rejection or budget requests. Simulator-driven organization
setup, complete scenario acceptance,
and clean-host qualification remain separate work. Stopping or omitting
Simulator must not stop these operator services.

### Matching Simulator companion

The matching companion includes the Northstar, Great Lakes, Load Lab, Redwood,
and Velocity profiles, including directory/policy assets and scenario files.
Add `--full-catalog` after the Simulator state argument to install them:

```sh
./simulator.sh start /absolute/matching-zpr-package /absolute/evaluation-state /absolute/simulator-state --full-catalog
```

This opt-in extends only the Simulator's local named-user organization scopes;
production grants and identity remain unchanged. The existing evaluation
organization is retained. Installation is repeatable and refuses to overwrite
changed assets. Catalog availability does not provision independent production
installs, machine runtimes, or authorize scenario execution. Live activation
still requires the organization's independently configured install.
Without the flag, the companion retains its evaluation-only behavior.
Full-catalog startup points Northstar's pre-generated directory loader at its
installed private LDIF assets; it does not require the old source runtime's
`linux-integration/pregen` directory.
The staged catalog also includes the original compiler configuration templates
and runtime policy fragments for all five organizations. They are source
authoring assets, not compiled/applied policy or proof of admission: their
addresses, trusted providers, credentials and shared runtime definitions must
be resolved against each independent install before compilation and activation.

#### Bounded scenario opt-in

For the single-node starter-directory endpoint-access scenario, create a **new** installation with
`-simulator-port PORT -scenario-demo`. This frozen opt-in enables the independent
production budget-authority database and gives the verified administrator
`budget.approve` permission. ZPR still does not start or require Simulator.
The companion captures a versioned evaluation runtime contract binding the
installation, immutable image, endpoint handoff and canonical live Visa policy
bundle. A changed policy or handoff is rejected, not silently rebound.

Open Simulator's Scenarios page, select **Verify endpoint access policy**, and
choose **Preview plan**. With no current budget evidence, choose **Approve
15-minute evaluation budget**. This sends your named, CSRF-protected delegated
approval to Control-Service; scope and digest are computed by the server.
Review and approve the resulting plan, then explicitly run it.

The runner rechecks current budget and runtime evidence, consumes the exact
plan approval once, and executes a fresh bounded probe round. It checks denied
listeners locally and requires matching fresh Visa denials, not just missing
replies. Results belong to the scenario run; cached background probes cannot
pass it. This opt-in disables background traffic probes to isolate run evidence.
With three endpoints, each run performs fifteen peer-access checks: allowed
ICMP, TCP/7007 and UDP/7008, and denied TCP/7009 and UDP/7010. The fixed ICMP
flow to the Visa Service is deliberately outside this scenario: its reusable
negative visa does not provide a fresh decision on every run. The ordinary
background workload view retains that separate observation when enabled.
It authorizes no provisioning, machine lifecycle, arbitrary targets or commands.
The existing OciWeb package gate remains mandatory for legacy machine scenarios.
This is one endpoint-policy scenario, not full user enrollment, subject-link
automation, multi-organization onboarding or policy-service qualification.

The second archive contains the prebuilt Linux/amd64 Simulator and a startup
wrapper. Verify its outer checksum before extraction. It requires the exact
matching ZPR package: startup verifies both payload inventories and compares
their build identities and immutable image inventories. No source tree or
toolchain is required. This is a native engineering client with Docker access,
which is root-equivalent; run only on a trusted isolated host.

Create the ZPR installation with `-simulator-port PORT` from the outset.
This optional setting registers an independent confidential OIDC client with
its own secret and exact HTTPS callback, but never reserves that port, launches
Simulator, or makes ZPR depend on it. The verified administrator receives
organization-scoped Simulator grants automatically. No identifiers, versions
or digests are entered in a browser. The same password works for both separate
application sessions. Existing installations without this option are not
silently migrated: create a separate private state directory.

After ZPR reports ready, from the extracted Simulator package:

```sh
./simulator.sh start \
  /absolute/zpr-evaluation-demo-20261029 \
  /absolute/new-private-state \
  /absolute/new-private-simulator-state
```

Use the actual extracted ZPR package path. With the example port above the
Simulator is at `https://localhost:18788`. For `-browser-ip`, it uses the same
explicit private address and the same public CA. Sign in normally; the verified
user is scoped to the paired evaluation organization. Simulator owns separate
workspace state and observes Control-Service over verified mTLS. Its endpoint
reconciler starts the three already-bootstrap-admitted endpoints and existing
bounded evaluation workloads exercise allowed and denied flows. These workload
observations are **not full Scenario runner acceptance** or successful
human/device enrollment. The 20 legacy scenario-machine records are explicitly
unprovisioned, not representations of the live endpoint containers.
The evaluation organization profile currently supports only the starter
directory, not custom LDIF installations.

Stopping this foreground companion stops only Simulator. ZPR remains running;
already started evaluation endpoint containers remain until ZPR teardown.
Simulator has no stop/start authority over the ZPR backbone. The companion does
hold an offline administrator-recovery lock: stop it as well as ZPR before
resetting the administrator, so old in-memory sessions cannot survive recovery.
Only one companion may run against an installation at a time.
Outside the explicit bounded-scenario opt-in above, the companion does not
automatically create budget requests. It does not create subject-link reviews,
signed snapshots or policy-service state, and it never bypasses scenario preflight.

The source launcher also accepts
`-control-room-binary /absolute/prebuilt/zpr-control-room` to skip its Go build.
That file must be a nonempty regular executable, not a symlink. The option is
runtime-only, like `-vs-binary`, and does not rotate installation identity.
Without it the existing source-build behavior is unchanged.

On graceful data-plane shutdown, writable state is returned to the owner of
the mounted `vs.toml` file. The container's parent state directory is
Docker-created and is not evidence of host ownership. Older images could
leave `data-plane/` and `valkey-state/` root-owned and prevent a non-root
restart; use an image containing the corrected shutdown script. Do not repair
unrelated installations or rotate credentials to work around that failure.

## Source launcher

**EVALUATION ONLY — NOT PRODUCTION SUPPORTED.** This is a foreground,
single-host engineering evaluation launcher, not the production appliance BOM,
a complete Visa Service installation, or a Simulator launcher.
It is a component subset, not a working ZPR network under the
[product-wide minimum](../../../System%20release%20plan.md#common-minimum-for-a-working-zpr-network):
it does not by itself prove an admitted node, active Visa policy, protected DNS
resolution (unless `-protected-dns` is enabled with the data plane), or
end-to-end durable logging.

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
  `-oidc-port` values for parallel installations. Explicit private-network
  browser access is available as described below. Never publish the private
  state directory or expose the internal directory/logging/Control-Service
  ports; do not add an unconfigured reverse proxy.

## Non-loopback browser address

Localhost is the default, not a mandatory transport. For a **new** evaluation,
opt in to browser access at the host's private IPv4 address:

```sh
sh packaging/evaluation/bootstrap.sh \
  -state "$PWD/.zpr-evaluation-network" \
  -browser-ip 192.168.194.24
```

The address must belong to the machine running this launcher. Configure the
host firewall to allow only intended clients to Room `18787` and Dex
`15556` (or their explicitly selected ports). The launcher binds that exact
address, not a wildcard; this is not public-Internet or production qualification.
Internal services remain loopback-only. The native Room process has no
Simulator dependency.

Open `https://192.168.194.24:18787`. Dex uses the single canonical issuer
`https://192.168.194.24:15556`; the callback uses the same address on the Room
port. Generated TLS certificates cover both the selected IP and `localhost`.
Securely transfer **only `ca.pem`** and trust it on the browser computer.
Passwords and grants follow the normal named-operator workflow below.

`https://localhost:18787` remains an HTTPS entrypoint on the server and redirects
to the network URL. It is not an independent login/session origin: this avoids
host-only cookie and callback confusion. Localhost on your laptop still refers
to your laptop, not this server; no tunnel is required for the network URL.
Dex also retains its local listener, but its advertised issuer stays canonical.
The browser and server must both be able to reach that issuer.

The IP is frozen with the install's configuration. Restart with identical
image/port/address flags. Changing it on an existing localhost-only install is
an **issuer migration**, not a bind-address tweak: old grants bind the issuer.
This launcher refuses that change rather than rewriting checksums, credentials,
subjects or persistent state. Do not delete state or edit its protected files
to bypass this check. Preserve the existing installation until a separately
validated identity-preserving migration is available; a new state directory
creates a distinct evaluation with new credentials, not a migration.

This launcher does not launch or relay Simulator. Simulator needs its own
listener, certificate, issuer/callback and deployment routing on its actual
host. Exposing Room never requires Simulator availability.

## Optional loopback Visa Service

Without this option, Control Room reports the Visa Service as unavailable. For
a **new** evaluation, `-visa-service` also runs a native Visa Service (VS) and a
private Valkey, so that Control Room shows live VS actors, services and visas
through Control-Service:

```sh
(cd .. && cargo build -p vs)          # once; any built vs executable works
sh packaging/evaluation/bootstrap.sh \
  -state "$PWD/.zpr-evaluation-vs" \
  -visa-service -vs-binary "$(cd .. && pwd)/target/debug/vs"
```

- **Loopback only:** VS admin `18182`, VSAPI `15002` and Valkey `16379` bind
  `127.0.0.1`. Override them with `-vs-admin-port`, `-vs-api-port` and
  `-valkey-port`. The ports and the option are frozen with the install, like the
  other ports. Adding the option to an existing install requires a new state
  directory.
- **Executables are not frozen.** `-vs-binary` (absolute path) and
  `-valkey-server` (default `valkey-server` on `PATH`) may be upgraded between
  starts. Both are checked before ports are reserved or containers start.
- **Generated, protected `0600` files:** `vs.toml`, `valkey.conf`,
  `valkey.secret`, the admin API key `vs-admin-api.key`, its hash-only
  `vs-admin-keys.toml`, and `vs-policy.bin2`. The VS admin API reuses this
  install's server certificate. Only Control-Service receives `ZPR_ADMIN_*`;
  Control Room still reaches the VS through Control-Service.
- **Policy:** the bundled policy is `integration-test/pregen/zpt-test.bin2`
  (test-signed). A test guards it against drift. It is installed into Valkey
  on the first healthy start and recorded in `vs-policy.installed`. Later starts
  keep the stored policy, including any policy applied through Control Room.
  Valkey persists with an append-only file under `valkey-state/`.
- **Not enforcing:** there are no nodes, adapters or endpoints, so no traffic is
  admitted or denied. Policy-Service is still not configured.
- **Original organization policy binding (source contract):** a nonsynthetic
  Simulator profile may declare `policy_binding` with `version: 1`,
  `target_id` and a lowercase 64-hex SHA-256 `digest`. Preflight and execution
  require authoritative Policy-Service current-state readback for that exact
  organization, target and digest; configuration alone is not evidence.
  Bindings are included in the authorization-scope digest. They cannot be
  combined with the synthetic or evaluation-runtime policy contracts and never
  remove topology, provisioning or authorization blockers. This does not
  provision Policy-Service or install an original organization's policy.
  A multi-install registry supplies `policy_service_url` per organization,
  using that install's explicit client certificate/key and service CA. It never
  falls back to another install or a global policy service when the selected
  install lacks this URL.
- **Named-reviewed machine enrollment (source components, not configured by
  the evaluation launcher):** packages include `zpr-enrollment-service`,
  `zpr-enrollment-bas` and `zpr-enrollment-device`. The device command takes a
  private `-config /absolute/device.json` followed by `initialize`, `claim`,
  `status` or `prepare-runtime`. Its version-1 JSON requires `audience`, `organization`,
  `invitation_id`, `state_directory`, `ca_file` and
  `allow_software_development: true`. State and CA paths must be canonical
  absolute paths. Initialize explicitly creates separate retained enrollment
  and BAS runtime keys; it never replaces or rebinds an existing identity.
  Claim additionally requires `-code-file /absolute/private-code.secret`;
  the one-time code is never accepted as a command argument or printed.
  Status and claim do not initialize missing identities or approve anything.
  See the [named batch review API](../../README.md#reviewing-a-claimed-device)
  for exact inventory review and durable approval recovery. Approval is not
  issuance, admission or traffic acceptance.
  After named approval, `prepare-runtime` obtains fresh key-bound status and
  requires both approved fingerprints to match this endpoint's retained keys.
  The status must supply the authoritative registry `asset_id`; the command
  never substitutes a workload display name or invitation ID. The handoff sets
  `authentication.bas_client_id` explicitly, preserving the original machine
  and workload names. Matching upgraded PH is required; older PH ignores this
  new field and is not qualified for these handoffs.
  It additionally requires `bas_ca_file`, an explicit canonical absolute BAS
  TLS CA path; enrollment TLS trust is not implicitly reused. It creates
  owner-private `runtime-auth.key`, `runtime-auth-ca.pem` and
  `runtime-auth.toml` inside the retained endpoint state directory. These files
  must remain on that endpoint: the private runtime key is never printed,
  returned through an API, or obtained from the enrollment server. Repetition
  preserves exactly matching files; changed keys, trust or symlinks are rejected.
  Pass the returned `authentication_config` path to PH or the controller;
  provisioning must mount the **same canonical paths** into its process
  namespace. The JSON reports `admission_verified: false`: preparing files is
  not successful BAS authentication or Visa Service admission.
- **Machine-controller BAS path (source only):**
  `SIMULATION_MACHINE_AUTHENTICATION_CONFIG=/absolute/machine-auth.toml`
  replaces, and is mutually exclusive with, `-zpr-bootstrap-key`. The protected TOML accepts only
  empty `[global]` and `[adapter]` tables plus `[authentication]` with `bas_key`
  and `tls_ca_file`, both canonical absolute paths, and an exact registry
  `bas_client_id`. Unknown configuration fields and missing IDs are rejected.
  In this mode each workload requires its own protected
  `/opt/zpr-workloads/authentication/WORKLOAD.toml` and endpoint-owned retained
  BAS key. Missing/invalid configuration fails rather than falling back to
  bootstrap keys. Existing explicitly configured bootstrap controllers retain
  their prior mode. This wiring does not deploy BAS, supply device credentials,
  authorize a device, or prove that the packaged PH supports the required
  AuthCode/TLS flow.
- **Machine identity requirements:** the canonical scenario plan includes
  `machine_runtime_identities` for controllers, resolved workload agents and
  the implicit `simulator-control` callback adapter. Its separate
  `machine_runtime_trust` identifies the BAS authentication root requirement.
  Both are included in the reviewed plan digest and authorized plan previews.
  A response without current resource-authorization evidence retains its minimal
  contract and does not disclose these runtime details.
  They are requirements, not enrollment approvals, root trust, a complete
  organization infrastructure inventory or proof of admission. The original
  Velocity machine path requires five enrolled identities, not just its four
  step-referenced controllers/workloads. Machine-free scenarios do not acquire
  fabricated machine requirements; evaluation endpoint scenarios keep their
  separate contract. No preflight blocker is waived by declaring identities.
- **Enrollment-owned BAS (optional source configuration):** the device-service
  `bas` object hosts adapter HTTPS and VS token mTLS on separate listeners in the
  enrollment-owning process, with its exact database path. No second BAS process
  or cross-process registry mount is needed. The standalone BAS command remains
  available for isolated fixtures; do not run it against the owning service's
  registry. Policy-authorized BAS root admission is still separately reviewed.
- **Supervision:** the Valkey and VS logs are `logs/valkey.log` and
  `logs/visa-service.log`. If either process exits, the install stops, like any
  other operator process.

### Subject-link publication cycle (with `-visa-service`)

A new `-visa-service` evaluation also configures the complete subject-link
review, signing and pinned delivery cycle to its loopback VS. It is
**evaluation only**: every key is generated for this install, and the VS
verifies and checkpoints snapshots but **never enforces** them.

- **Port:** Control-Service serves pinned snapshot delivery (TLS 1.3, mTLS) on
  loopback `18793`; override it with `-snapshot-delivery-port`. Installs created
  with `-visa-service` before this cycle existed have no delivery port. Restart
  them with `-snapshot-delivery-port 0`, or create a new state directory.
- **Second login:** `reviewer@evaluation.invalid` (password in
  `operator-reviewer.secret`) is an optional second Dex subject that may only read
  and approve. The administrator gains `subject-link.read`, `.propose`,
  `.registry.manage`, `.publish`, `.snapshot.capture`, `.snapshot.sign` and
  `.approve`. A single operator may propose and approve; the two steps stay
  separate, and a proposer's own approval is flagged in the audit as a less
  secure single-operator review.
- **Generated, protected files:**
  - `subject-link-providers.json`: binds LDAP source `evaluation-directory`
    version `1` to this install's LDAPS directory.
  - `operator-delegation-signer.json` and `operator-delegation-key.pem`: the
    Room delegation signer.
  - `subject-snapshot-signing.json` (public pins) and
    `subject-snapshot-signing-key.pem`.
  - `vs-client-chain.pem` and `vs-client-key.pem`: the VS delivery identity.
  - `subject-snapshot-recipients.json`: pins that identity's SPKI to
    `evaluation-vs`.
  - `SUBJECT-LINKS.txt`: the organization ID and identifiers below.
  - `operator-delegation-trust.json`: written on first start, once both Dex
    subjects are verified. As with `operator-oidc.json`, a later difference is
    refused, never rebound.
- **Synthetic subject:** the directory gains
  `uid=evaluation-subject,ou=People` with a generated, explicit `entryUUID`
  (listed in `SUBJECT-LINKS.txt`), because Control Room does not display LDAP
  UUIDs.
- **Explicit VS initialization:** `vs.toml` contains `[subject_snapshot_fetch]`
  (poll every 15 s). Until the checkpoint is initialized, `logs/visa-service.log`
  reports "checkpoint unavailable" on each poll. Initialization is never
  automatic.

Operator flow, using the Subject links and Publication pages:

1. As the administrator, create the BAS authority (`kind` `bas_device`) and the
   source `evaluation-directory` (`provider_id` `evaluation-ldap`,
   `selector_kind` `ldap_entry_uuid`, for example `enrichment_keys` `["mail"]`).
   Keep the source at version `1`: an edit advances its version, which the
   provider no longer binds.
2. Load the registry context, then propose a link from a device subject to the
   sample `entryUUID`.
3. Approve the review (as the administrator, or as the optional second login).
4. As the administrator, on the Publication page:
   - provision `evaluation-vs` / `evaluation`;
   - promote the approved review;
   - capture a candidate with explicit issued and expiry times;
   - verify its digest and sign it.
5. Initialize the VS checkpoint once, with the command printed at READY:

   ```sh
   "<vs-binary>" --initialize-subject-snapshot -c "<state>/vs.toml"
   ```

   The running VS then fetches each newly signed snapshot. Its receipt is the
   stream's latest receipt on the Publication page.

   With `-data-plane`, run the `docker exec … vs --initialize-subject-snapshot`
   command printed at READY instead. Its checkpoint is
   `data-plane/vs-subject-snapshot`.

### Data-plane container (with `-visa-service -data-plane`)

The native VS has no ZPR overlay, so it can never admit a node. ZPR nodes reach
the VS only at its fixed ZPR address `fd5a:5052::1` (VSAPI `5002`), through the
VS's own `vs.zpr` adapter. For a **new** evaluation, `-data-plane` therefore
runs the VS, private Valkey, one node and the `vs.zpr` adapter in a privileged
Linux container. Control Room then shows the live node and VS actors.

```sh
packaging/evaluation/data-plane/build.sh   # once; builds zpr-evaluation-data-plane:local
sh packaging/evaluation/bootstrap.sh \
  -state "$PWD/.zpr-evaluation-dp" -visa-service -data-plane
```

To include the protected DNS service, enable it when creating the install:

```sh
sh packaging/evaluation/bootstrap.sh \
  -state "$PWD/.zpr-evaluation-dns" -visa-service -data-plane -protected-dns
```

`-protected-dns` is frozen with the install and requires a new state directory
to change. It creates a distinct `zpr-dns` adapter identity and per-install
bootstrap/TSIG keys, adds the TCP/53 service and authenticated-device policy to
the initial data-plane policy, configures Visa Service RFC 2136 updates, and
runs authoritative-only BIND on `fd5a:5052::53`. BIND is not published on a
host or underlay port; recursion is disabled and zone transfers are refused
except to a read-only TSIG viewer key (`dp-dns-viewer.key`) over a loopback-only
relay (`-dns-transfer-port`, default 15353) that feeds the Control Room DNS
records page. Startup does
not report ready until Visa lists the DNS adapter and direct TCP queries from
the Visa Service's isolated ZPR namespace return both the DNS service address
and Visa Service's dynamically published adapter record over ZPR. This
exercises the protected query and Visa-to-BIND TSIG update paths, but not every
client identity or policy allow/deny case.

- **Build:** `build.sh` compiles `ph`, `ph-cli` (`zpr-core`), `vs` and
  `vs-admin` (this worktree) and `zplc` (`zpr-compiler`) for Linux in a builder
  image. The sibling checkouts are mounted read-only, and `ZPR_WORKSPACE`
  overrides their parent. `ZPR_CORE_DIR` overrides only `zpr-core`, for
  example to build a zpr-core fix branch from its own worktree; tag such
  builds separately with `-tag`. The cargo cache and target live under
  `.evaluation-build/data-plane/`. `-data-plane-image` (default
  `zpr-evaluation-data-plane:local`) is runtime-only, like `-vs-binary`. It is
  never pulled.
- **Ports:** only the VS admin API is published, to `127.0.0.1:<vs-admin-port>`.
  VSAPI and Valkey stay inside the container. `-vs-api-port`, `-valkey-port`,
  `-vs-binary` and `-valkey-server` are rejected. `-data-plane` is frozen with
  the install.
- **Unique per-install ZPR identities (protected `dp-*` files):**
  - an RSA ZPR CA (`dp-ca.crt`; its key is discarded);
  - X25519 Noise certificates for `node` and `vs.zpr`. The `vs.zpr` subject is
    the exact DER the node uses to recognize the VS;
  - RSA bootstrap keys for both;
  - an RSA policy-signing key;
  - the policy source `dp-policy.zpl` and `dp-policy.zplc`. They admit only the
    node and `vs.zpr`, plus ping between them.
- With `-protected-dns`, the install additionally creates a `zpr-dns`
  certificate/bootstrap identity, a private publisher TSIG key, and a DNS
  service declaration. The policy permits authenticated ZPR devices to query
  TCP/53; the Visa publisher uses the separate TSIG key for authorized updates.
- **Policy:** on first start, the container compiles the policy with `zplc` into
  `data-plane/vs-policy.bin2` and installs it into Valkey once, recorded in
  `vs-policy.installed`. Later starts keep the stored policy, as in
  native mode.
- **Minimal mounts:** the container sees only the files it needs, at their
  original absolute paths, so `vs.toml` stays valid. Everything is read-only
  except `valkey-state/` and `data-plane/`. It never sees the install CA key,
  operator secrets or the VS admin API key.
- **Network:** a per-install Docker network,
  `zpr-evaluation-<install-id>-data-plane`, which endpoint containers join. Inside the container, namespace `zpr-vs` holds Valkey, the VS and the
  `vs.zpr` adapter; the node runs in the root namespace. With `-protected-dns`,
  the DNS adapter and authoritative BIND run in a separate `zpr-dns` namespace
  with an underlay veth to the docking node, keeping DNS replies on that
  adapter's ZPR tunnel. Snapshot delivery is relayed to the host's loopback
  delivery port.
- **Readiness:** the VS admin API must be healthy *and* list both `node` and
  `vs.zpr` as admitted actors. With `-protected-dns`, it must also list
  `zpr-dns` and pass TCP DNS queries for the service address and the Visa
  adapter's dynamic record from the Visa Service namespace over its ZPR tunnel.
  `logs/data-plane.log` holds every component's
  log, and any component exit stops the install. Ctrl-C removes the container,
  every other container on the install network (including endpoints) and the
  network; state is retained.
- **Map position (optional):** `-node-location LAT,LON` (decimal degrees,
  requires `-data-plane`) writes `[nodes."node"] latitude/longitude` into the
  frozen `vs.toml`. The VS reports it as the node's actor details, so Control
  Room can place the node on the map. The position is operator configuration;
  it can only be set when the install is created.
- **Still not production:** endpoint admission by keys compiled into the
  policy is a recorded evaluation stopgap (X15), and enrollment is deferred.
  Nothing is enforced outside this container.

### Simulator endpoints (with `-data-plane -data-plane-endpoints N`)

`-data-plane-endpoints N` (0, or 2–16; frozen with the install) generates
`endpoint-01`…`endpoint-NN`. Each endpoint has its own RSA bootstrap key,
`dp-endpoint-NN-auth.{key,pub}`, and a fixed ZPR address `fd5a:5052:0:1::N` (hex).
The per-install policy admits each endpoint by its public key and lets every
endpoint ping the others (`EndpointNNPing`). The data-plane container mounts
only the public keys.

The install also writes the handoff `data-plane-endpoints.json`. It names the
install network, the node container, the CA and each endpoint's key file and
address. The install does not run endpoints itself. Point the Simulator at the
handoff:

```sh
SIMULATOR_DATA_PLANE_ENDPOINTS_FILE="$STATE/data-plane-endpoints.json" \
  go run ./cmd/zpr-web-dashboard -mode simulator ...
```

The Simulator validates the handoff strictly. Every 10 seconds it reconciles
one container per endpoint, `zpr-evaluation-<id>-data-plane-endpoint-NN`, on
the install network:

- It uses the same image with entrypoint `zpr-evaluation-endpoint`, overridable
  with `SIMULATOR_DATA_PLANE_IMAGE`.
- Each container mounts only that endpoint's private key and the CA.
- The Simulator adopts running endpoints and relaunches missing ones. It
  reports "install data plane is not running" while the install is down.

Endpoints outlive a Simulator restart and are removed by install teardown.
`GET /api/simulator/data-plane-endpoints` (`simulator.read`) reports their
state. Admitted endpoints appear in the VS actors and in Control Room through
Control-Service. Control Room never reads the handoff.

#### Allow/deny workloads (D4)

The Simulator probes the data plane from its own endpoints every 30 seconds by
default:

- **Allowed:** each endpoint pings the next one, which the per-install policy
  permits.
- **Denied:** each endpoint pings the Visa Service's fixed ZPR address
  `fd5a:5052::1`. The policy lets only the node ping it, so the VS denies the
  visa with `NoMatch`.
- **Services (U05):** each endpoint also runs `socat` echo listeners bound only
  to its ZPR address: `tcp/7007` and `udp/7008`, which the policy publishes to
  peer endpoints (`EndpointNNEchoTCP`, `EndpointNNEchoUDP`), plus `tcp/7009`
  and `udp/7010`, which have no policy service. Each endpoint sends a token to
  every listener of the next endpoint with `zpr-evaluation-probe`. The
  published ports must echo it and the unpublished ones must be dropped by
  ZPR, even though a process is listening.

The install lists the listeners in the handoff (`listeners`, with `in_policy`
per port), and the Simulator passes them to its endpoint containers as
`ZPR_EP_LISTEN`. Handoffs without `listeners`, from installs created before
U05, keep the ping probes only; the policy is frozen, so service workloads need
a new install.

`SIMULATOR_DATA_PLANE_WORKLOAD_INTERVAL` sets the interval as a Go duration of
at least `5s`; `0` or `off` disables the probes.

`GET /api/simulator/data-plane-workloads` (`simulator.read`) reports each
probe's expected outcome (`allow`/`deny`), the outcome the Simulator observed
(`reachable`/`unreachable`/`error`), whether they match, and totals for
`mismatches` and `errors`. A mismatch is also logged.

The expectations are Simulator scenario intent only. Control Room observes the
same traffic independently: Control-Service reads the visas, recent denies and
the approved/denied counters from the VS admin API. The node caches denials,
so a repeated denied probe does not increase the VS deny count.

#### Operator client (U05b)

With `-data-plane-endpoints N`, the install also generates one operator client,
`client-01` at `fd5a:5052:0:1::c1`. It has its own key and gets exactly an
endpoint's policy in both directions: its echo services are published to every
endpoint, and every endpoint's services are published to it. The Simulator
handoff does not name it; the operator runs it from `data-plane-client.env`:

```sh
sh packaging/evaluation/data-plane/client.sh start  "$STATE"
sh packaging/evaluation/data-plane/client.sh status "$STATE"
sh packaging/evaluation/data-plane/client.sh probe  "$STATE" ping fd5a:5052:0:1::1
sh packaging/evaluation/data-plane/client.sh probe  "$STATE" tcp  fd5a:5052:0:1::1 7007
sh packaging/evaluation/data-plane/client.sh stop   "$STATE"
```

`client.sh` parses the env file without sourcing it, and runs the same image
and listeners as the endpoints (override the image with `ZPR_CLIENT_IMAGE`).
Its probes should match a Simulator endpoint's probes to the same target: a
reply on `tcp/7007` and `udp/7008`, no reply on `tcp/7009` and `udp/7010`.
Control-Service reports the client's actor, services, visas and `NoMatch`
denials the same way as the endpoints', without the Simulator. Install teardown
removes the client container with the rest of the data-plane network. Installs
created before U05b have no client.

#### One install per organization (D5)

To switch Simulator organizations against real installs, the operator starts one
evaluation install per organization, each with its own state directory, ports
and (usually) `-node-location`. By default, the Simulator only selects among
already-running installs listed in an operator-written registry:

```json
{"version": 1, "kind": "zpr-simulator-evaluation-installs", "evaluation_only": true,
 "installs": [{
   "organization_id": "northstar",
   "control_room_url": "https://192.0.2.10:38787",
   "operator_service_url": "https://localhost:38790",
   "client_cert_file": "/abs/northstar/client-chain.pem",
   "client_key_file": "/abs/northstar/client-key.pem",
   "service_ca_file": "/abs/.zpr-evaluation-northstar/ca.pem",
   "data_plane_endpoints_file": "/abs/.zpr-evaluation-northstar/data-plane-endpoints.json"
 }]}
```

Set `SIMULATOR_EVALUATION_INSTALLS_FILE` to the registry. It replaces
`SIMULATOR_OPERATOR_SERVICE_URL`, `SIMULATOR_DATA_PLANE_ENDPOINTS_FILE`,
`SIMULATION_ORGANIZATION_RESET_SCRIPT` and `ZPR_SIMULATOR_CONTROL_ROOM_URL`;
setting any of them alongside it fails startup. Each organization must exist in
the catalog and appear once. Service URLs must be exact HTTPS origins, TLS file
paths must be distinct absolute paths, and two installs cannot share a data
plane. The Simulator client certificate for each install is issued from that
install's `ca.pem`.

With the registry:

- Activating an organization checks that its install's Control-Service answers,
  then selects it. Restore base is not available; reset or recreate the install
  independently.
- Snapshots, the Control Room link and the endpoint/workload status routes
  follow the selected organization.
- Endpoints and workloads run for **every** registered install that has a
  handoff, not only the selected one, so idle installs stay exercised.
- `GET /api/simulator/evaluation-installs` (`simulator.read`) lists the
  registered organizations, which one is selected and whether each has a data
  plane. It does not expose TLS file paths.

##### Optional on-demand supervision

An install may additionally set `"lifecycle_command"` to an argument array, for
example `["/abs/operator-supervisor", "--config", "/abs/northstar/operator.json"]`.
This is an operator-owned executable and independent production configuration,
not the legacy organization activation script. It must not read Simulator
profiles, manifests, sessions, or workloads. The package supplies a systemd user
service adapter; operator units and original-scenario machine provisioning must
be supplied independently.

- The registry must be a private regular file (for example mode `0600`). The
  executable must be a regular executable file, not a symlink, and neither file
  may be group/other writable. Use trusted parent directories. The command has
  at most 16 nonempty elements, each at most 4096 characters.
- The Simulator executes the argument array directly, **without a shell**,
  appending exactly one action: `start` or `stop`. Environment variables prefixed
  `SIMULATION_`, `SIMULATOR_`, or `ZPR_SIMULATOR_` are removed. The supervisor
  must write its own private logs; its output is not returned to browser users.
- `start` must idempotently ensure this install is supervised independently of
  the short-lived command. `stop` must idempotently await shutdown of only this
  install, including endpoint containers, without deleting persistent state.
  Both actions must handle SIGTERM and finish within two minutes. An install
  without this field is never stopped by the Simulator.
- Switching starts the requested runtime, then polls its authenticated mTLS
  operator snapshot for up to two minutes, matching the evaluation bootstrap
  startup allowance. Only the selected managed install has
  active endpoint/workload workers. Before stopping the previous managed runtime,
  the Simulator cancels and joins its workers.
- Selection is saved only after the previous runtime stops. A stop/save failure
  restarts the previous runtime and stops the newly started one. A partial-start
  or readiness failure also stops the new runtime. Same-organization failure
  does not stop that selected runtime. Cleanup/recovery errors remain explicit;
  a failed recovery must be repaired by the operator.
- At Simulator startup, the persisted selected managed runtime is started and
  checked, and other explicitly managed runtimes are stopped. This reconciles
  interrupted switches. An unavailable supervisor fails startup rather than
  pretending the organization is ready. Simulator shutdown joins its workers
  but leaves the independent selected production install running.

This lifecycle contract does not restore the five original organization
scenarios by itself. Their admitted machine runtimes, topology, workload assets,
authoritative policy evidence, and live scenario acceptance are separate gates.

###### Systemd user service adapter

On Linux hosts with an active systemd user manager, use:

```json
"lifecycle_command": ["/abs/zpr-evaluation-RELEASE/bin/zpr-evaluation-supervisor", "zpr-evaluation-northstar.service"]
```

The adapter accepts only `zpr-evaluation-NAME.service` (lowercase letters,
digits, internal hyphens; at most 64 characters in NAME) and `start`/`stop`.
It invokes `/usr/bin/systemctl --user` directly, waits for completion, and
requires the final state to be `active`/`inactive`, respectively. Missing units
cannot start. Repeated stop of an unloaded unit succeeds only after an explicit
`LoadState=not-found`, `ActiveState=inactive` readback. Failed jobs, incomplete
shutdown, and cancellation are errors, not successes.
The adapter does not install units, change user-manager settings, or enable
services. Its total command deadline is 110 seconds.

The operator must install one independent unit per organization, with:

- A fixed absolute `ExecStart` launching its package and private state with all
  frozen operator flags (including organization identity and fixed ports).
  Do not point it at `environments/workbench.sh`, Simulator launchers, or
  profile-reading legacy activation scripts.
- Foreground supervision (`Type=exec`), `Restart=on-failure`, `UMask=0077`,
  SIGTERM shutdown and enough `TimeoutStopSec` for retained-state cleanup.
  Avoid `RemainAfterExit`: an exited launcher must not appear healthy.
- Install-scoped endpoint/container cleanup as part of shutdown. A successful
  systemd stop alone does not prove that all Docker endpoints were removed.
- Private operator logs and no credentials in unit command lines. The adapter
  reports exit failures but never returns unit journal output to the browser.

Render the unit from an explicit independent production command, without
starting anything or reading Simulator input:

```sh
umask 077
/abs/zpr-evaluation-RELEASE/bin/zpr-evaluation-supervisor render-unit \
  /abs/zpr-evaluation-RELEASE/demo.sh start /abs/northstar-state \
  -organization-id northstar \
  -room-port 28787 -service-port 28790 -ldap-port 21636 \
  -ldap-admin-port 28789 -logging-port 25080 -oidc-port 25556 \
  -vs-admin-port 28182 -snapshot-delivery-port 28793 -dns-transfer-port 25353 \
  > /abs/operator-units/zpr-evaluation-northstar.service
```

These are illustrative **new-install** ports, not a change to the fixed dev or
stable bindings. Supply the organization's reviewed directory, node placement,
and any other frozen flags before first startup. The renderer accepts a trusted
regular executable launcher plus at most 127 arguments. It quotes each systemd
argument, escapes environment/specifier expansion, rejects control characters,
and emits no environment overrides or shell evaluation. The unit uses
`KillMode=mixed` so SIGTERM reaches the supervising bootstrap first and lets it
clean up its children and containers. Review the unit and use
`systemd-analyze --user verify` before installing it in the user unit directory
and reloading the manager. Rendering never installs, enables, or starts a unit.
Do not put passwords, tokens or other secrets in launch arguments.

Use the user journal to diagnose failures. If services must survive logout,
the operator must separately approve/configure user lingering; the adapter never
changes it. A user manager is available on workbench, but lingering is not
enabled there. No organization units have been installed by this source change.

### Organization directory (D6a)

By default LDAP serves the empty starter tree under `dc=evaluation,dc=zpr`. To
serve an organization's own directory instead, give its base DN and LDIF files
(absolute paths, imported in order) when the install is created:

```sh
-directory-base-dn dc=greatlakes,dc=test \
  -directory-ldif /abs/organizations/great-lakes/directory.ldif
```

- **Frozen:** the LDIF is rendered into the protected `directory.ldif` once.
  Restarts need the same flags; changing the directory needs a new install.
- **Unchanged import:** entries keep their DNs and attributes. The first entry
  must be the base entry, every entry must be under the base DN, and only plain
  content records are accepted (no change records or base64 DNs).
- **No imported credentials:** entries carrying `userPassword` (an
  organization's bind accounts) are skipped and listed in `EVALUATION.txt`;
  entries below a skipped entry are rejected. The install adds its own reader
  `cn=zpr-reader,<base>`, plus `ou=People`/`ou=Groups` if missing, and the
  subject-link sample under `ou=People`.
- **Schema:** the install also loads `zpr-evaluation.schema`, an
  EVALUATION ONLY auxiliary class `zprMachine` (`zprMachineId`, `…Type`,
  `…Model`, `…Location`, `…Secure`, `…Owner`) under a UUID OID arc.
- Source registration, the subject-link provider and the LDAP health check use
  the configured base DN. Nothing reads the Simulator; the LDIF is ordinary
  operator input.

### Additional data-plane nodes (D6b)

The data-plane container is the docking node (`node`, at `-node-location`)
with the Visa Service. Add linked nodes when the install is created:

```sh
-data-plane-node shenzhen-office@22.54,114.06 \
  -data-plane-node tijuana@32.51,-117.04
```

- **Names:** up to 6 extra nodes; lowercase letters, digits and `-`; not
  `node`, not `endpoint-*`, no duplicates. The name is the node's certificate
  CN, so the VS and Control Room show it, at its latitude/longitude.
- **Topology:** every node links to every other node (full mesh). The install
  uses its own Docker subnet `10.2xx.yy.0/24`, derived from the install ID.
  Nodes have fixed addresses from `.2` and ZPR addresses from `fd5a:5052::2`.
  The policy declares the nodes, links and bootstrap keys, and each node
  pins its peers' Noise certificates. Generated `dp-node*.toml` files are
  frozen with the install.
- **Startup:** the extra nodes start as `<network>-node-<name>` containers
  (image entrypoint `zpr-evaluation-node`) once the docking node is admitted.
  They reach the Visa Service through the docking node; readiness waits for
  every node to be admitted.
- **Endpoints** from `-data-plane-endpoints` are spread round-robin across all
  nodes. The Simulator handoff names each endpoint's `node_host`, the
  node's network alias `node-<name>` (full container names can exceed DNS
  label limits).
- Without `-data-plane-node`, the single-node layout is unchanged.

## Identity, health and lifecycle

Every install receives a random install/Compose-project ID, its own CA and
service/client certificates, and independent 256-bit random LDAP admin/reader,
OpenObserve admin, operator and OIDC client secrets. A fixed complexity suffix
satisfies the packaged OpenObserve password policy without reducing random
entropy. There are **no shared/default passwords**. Unless an organization directory is
configured (see above), LDAP starts with an empty People/Groups tree and one read-only service
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
Readiness includes **ordinary LDAP registration/read, a byte-for-byte readback
of the initial policy on first start when `-visa-service` is enabled, and
acknowledged/queryable lifecycle logging retained through an OpenObserve
container restart**. The baseline policy allow/deny traffic checks remain part
of the explicit data-plane/endpoint acceptance flow; startup does not claim
those checks passed. This is still not the complete system-install baseline,
and does not qualify production services. Later restarts validate the current
Visa policy bundle without requiring it to remain identical to the initial
policy, preserving operator-installed policy changes. Missing upstreams remain
visibly unavailable. No service
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
Recovery rotates only the administrator. A subject-link reviewer login is left
unchanged, and both `operator-oidc.json` and `operator-delegation-trust.json`
are retired so the next start rebinds verified subjects.
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

- **B1–B3:** named-operator bootstrap, persistent starter directory, persistent
  logging backend and independent operator runtime, with ordinary source
  registration and authorized log queries as described below. Bootstrap emits
  its lifecycle records; merely starting OpenObserve does not mean other
  application logs are ingested. LDAP push watchers remain independently
  operator-configured; bootstrap does not enable server overlays or watchers.
- **Bundled login recovery:** the local, stopped-host command above rotates only
  the evaluation Dex operator. X12 records the deliberately unfinished
  forced-change, expiry, single-use and upstream-token revocation guarantees.
  OpenObserve's independent administrator is not recovered by this command;
  its supported credential/session recovery remains unfinished. Never delete
  provider state or change `openobserve.env` to claim a logging password reset.

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
- **Bundled login recovery:** the evaluation-only stopped-host reset is
  implemented and tested; it rotates Dex's static password, client secret and
  subject while preserving provider data. Delivery is a one-time disclosure,
  not enforced one-time use. Dex does not enforce password replacement or expiry,
  already-issued JWTs remain valid until expiry, and OpenObserve's independent
  administrator is not reset. These limits block production recovery; editing a
  hash/file or deleting persistent state is not an acceptable recovery method.
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

### Working-network evaluation acceptance (2026-10-27)

A fresh evaluation state was started on the macOS/arm64 engineering host with
`-visa-service -data-plane -protected-dns -data-plane-endpoints 3`. Startup
verified the Visa Service, node and adapters, the protected DNS query and
dynamic Visa adapter record through the ZPR tunnel, plus B1-B3 readiness. The
separate Simulator companion registered the three evaluation endpoints.

Live endpoint-to-endpoint probes verified TCP/7007 and UDP/7008 return their
echo payloads, while the listening but unpublished TCP/7009 and UDP/7010
services return no reply. From an admitted endpoint, TCP DNS queries resolved
`dns.svc.zpr` to `fd5a:5052::53` and
`vs.zpr.adapters.svc.zpr` to `fd5a:5052::1`. The disposable B1-B3 test also
passed two starts, retained identity/data, TLS/mTLS checks, scoped source
registration/read, admin-only logging, lifecycle ingestion and cleanup.

This verifies the evaluation network path, not a released package: it ran on the
existing macOS/arm64 engineering host, not a clean supported Ubuntu host. The
endpoints use evaluation bootstrap-key admission (X15), not production
enrollment; Policy-Service is not configured. Signing, clean-host packaging,
production provider qualification and the remaining production BOM gates stay
open.

## Optional add-later synthetic companion (B5)

After bootstrap is healthy, use the separate [Simulator companion runbook](../simulator-companion/README.md)
to add an isolated synthetic directory in the **same runtime evaluation organization**
with a distinct stable source ID. Bootstrap never starts it, reads its state or
requires its availability. Companion setup/test/registration/removal use only
ordinary authenticated public source contracts. Bundled LDAP, logging, operator
configuration and default source are not replaced or reseeded. Stopping or removing
the companion leaves the evaluation install running. This is not legacy Simulator
scenario activation, multi-source policy acceptance or B6 starter retirement.

### Original Velocity retained machine BAS handoff (source only)

The original two-machine runtime has a separate, explicit opt-in:
`SIMULATION_MACHINE_AUTHENTICATION_DIR=/absolute/private/velocity-machine-auth`.
The directory must be canonical, owned by the launcher user, and private
(mode `0700`). It contains endpoint-local state directories named for their
authoritative registry asset IDs:

```text
velocity-machine-auth/
  velocity-machine-03-controller/
  velocity-machine-03-finance-client/
  velocity-machine-05-controller/
  velocity-machine-05-echo-service/
  velocity-simulator-control/
```

Each child contains the retained `identity/` directory produced by the enrollment
helper, with `runtime-auth.toml`, `runtime-auth.key` and
`runtime-auth-ca.pem`. The TOML's absolute key and CA paths must remain valid both
on the launcher host and in the machine container. The launcher checks all five
IDs, config paths, owner-private modes and required files before starting the
callback or a machine. Only the three exact runtime files per identity are
mounted read-only, not the endpoint or identity directory;
the callback receives only its three runtime files. No registry or enrollment
private key is mounted or read.

Before starting or recreating the Simulator container, export both
`SIMULATION_ORGANIZATION_ID=velocity` and
`SIMULATION_MACHINE_AUTHENTICATION_DIR=/absolute/private/velocity-machine-auth`.
The existing launcher passes only those runtime files at their exact paths and passes
the opt-in to the Simulator process, which is the lifecycle consumer that
starts machine containers. A running Simulator created without that exact
environment and mount is rejected; it is not silently upgraded. Use the same
environment when starting `machine-03` and `machine-05`. Starting or restarting
the SimulatorControl service uses the same opt-in for
`velocity-simulator-control`. The controller identities are
`velocity-machine-03-controller` and `velocity-machine-05-controller`; the
workload identities are `velocity-machine-03-finance-client` and
`velocity-machine-05-echo-service`. They are never substituted with display
labels. This mode supplies no bootstrap fallback: missing or mismatched
material fails preflight, and an already-running container in another auth
mode must be stopped before switching. With the variable unset, existing
environments retain their legacy bootstrap behavior.

This is launcher/source wiring only. It does not enroll or authorize
endpoints, change policy or topology, migrate a running scenario, deploy
services, or prove that the configured PH binaries successfully authenticate
or are admitted.

The original launcher still expects its demo node layout (`/app/bin` tools,
profile node addresses and generated configuration). This opt-in does not
make it compatible with the separate evaluation installation layout. Its
readiness and package preflight gates must not be bypassed to claim integration.
`make test` executes the isolated shell handoff fixture as well as Go tests.

The approved next direction is an explicit logical-to-physical binding of the
original Velocity single-node scenario to its independent retained installation,
not restoring the legacy demo node or rewriting the original scenario. The
Simulator install registry now accepts `retained_topology_binding` on the
Velocity install:

```json
{
  "version": 1,
  "install_id": "0422ecb887a5106caea70975",
  "logical_node_id": "velocity-node",
  "node_container": "zpr-evaluation-0422ecb887a5106caea70975-data-plane",
  "network": "zpr-evaluation-0422ecb887a5106caea70975-data-plane",
  "image_id": "sha256:c820f183c0fb3e05523e925dcebe9b975d045a6f03487e7b8c1da2d9531f6062",
  "substrate_address": "172.26.0.2",
  "zpr_address": "fd5a:5052::2"
}
```

These are observed installation values, not defaults. A changed image, address,
owner or network must fail preflight until the operator explicitly updates the
binding and requests a new plan. The Simulator checks the running container,
install ownership label, exact image, network address and node TUN address using
read-only Docker inspection. The binding is included in the authorization-scope
digest, cloned approval snapshot and preflight topology display. It does not
change the source profile, people, workload inventory, benchmark or cleanup.
The actual verifier passed against the retained node and rejected a deliberately
incorrect image. Source planner/registry, race and frontend-contract tests pass.

Without an explicit `retained_machine_runtime`, preflight still reports
`retained-topology-only` and remains blocked. With the maintained consumer
configured, the source plan reports `retained-machine-bas-v1`; this is not a
released package or permission to bypass policy/resource approval. Production
services never read either Simulator configuration.

The version-1 runtime configuration pins `image_id`, `controller_file`,
`controller_sha256`, `endpoint_root`, `data_plane_ca_file`, `control_tls_dir`
and `callback_upstream`. Files and the actual node binding are checked before
start. `scripts/prepare-retained-machine-control.sh` prepares or verifies a
separate Simulator-only mTLS CA/server/two controller identities; it neither
enrolls devices nor changes production trust. Each runtime receives only its
exact files. Protected regular-file copies in private root-owned tmpfs preserve
the TOML's absolute key/CA references without weakening host ownership checks.
The callback and machine containers have exact install/configuration/generation
ownership, and cleanup mutates inspected container IDs only.

The matched PH must support Linux `--tun-route-table`: retained controllers use
105, callback 106 and workloads their distinct configured tables. This binds
temporary BAS authentication and granted-address traffic to each identity's own
TUN; legacy runtime routing remains unchanged. The retained path reads back the
VS-managed service AAAA record from its owning production DNS namespace instead
of trying to publish into `zpr-dns-bind9`. Client traffic still performs the
actual protected DNS query through its workload adapter.

**2026-10-30 isolated diagnostic evidence:** all 13 original Velocity actions
and seven original cleanup actions pass, with both named machine owners and
all five retained BAS identities. Two repeats verified exactly 100 latency
samples and 16,777,216 bytes, with benchmark phases below 0.5 seconds and total
runs below eight seconds. After a normal independent base restart, the same
actions and threshold assertions passed in 23.62 seconds overall. No shell
repair or policy/trust change occurred during these runs. This sequential
direct-action harness is not Simulator UI/resource approval or parallel
scheduling acceptance. See the [current release gate](../../../environments/README.md#velocity-release-gate-and-delivery-discipline).

Velocity-only base r20 preserves session-owned signed BAS claims during
directory refresh, denying expired evidence rather than treating it as known
absence. Its data-plane shutdown now signals/reaps owned service and log-reader
PIDs, including bounded forced cleanup; a same-name unrelated process survives
the required shell fixture. Normal restart restores generated-state ownership.
The one-time transition off the old image completed its retired cleanup's DNS
snapshot ownership restoration before qualification; that repair was not part
of any successful scenario run.

Machine-controller workload adapters now retain their actual child process
handles. Start/stop never kill by executable name; an active adapter left by
another controller is reported explicitly and requires a machine restart.
Ordinary test-service startup/cleanup also runs through the machine's mTLS
command channel and controller-owned child handles. Stops are bounded and
reaped before replacement, and stopping a workload also stops its owned test
service. This source behavior requires a matched Simulator/controller upgrade;
it has not been deployed to the retained dev/stable pair. Business/gateway and
stress service paths retain their existing separate lifecycle behavior.
