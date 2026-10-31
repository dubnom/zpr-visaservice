# Workbench environments

The two active environments run independently on `dubnom@workbench.local`
(`10.0.1.160`). Updated 2026-10-30 for the Velocity-first delivery gate and
original Velocity action/benchmark diagnostic qualification. Main dev/stable
releases are unchanged.

| Environment | Control Room | Simulator | Package source |
|---|---|---|---|
| Dev | <https://10.0.1.160:18787/> | <https://10.0.1.160:18788/scenarios.html#simulate> | production `dev-20261029-r7`; Simulator `dev-20261030-r8` |
| Stable | <https://10.0.1.160:38787/> | <https://10.0.1.160:38788/scenarios.html#simulate> | tested package `scenario-20261029-r6` |

Stable means the retained, tested **engineering demo baseline**, not a
production-qualified release. Both provide independent production-tagged
Control Room/Control-Service, Visa Service, protected DNS, LDAP, OIDC, logging
and a separately supervised Simulator. Policy-Service is not configured.
Each has one provisioned bounded endpoint-policy scenario, not full onboarding.
Dev additionally installs Northstar, Great Lakes, Load Lab, Redwood, and Velocity
profiles with their directory/policy assets and scenario catalogs. Catalog
installation does not provision independent live organization installs:
activation and scenario execution still require those runtimes and authorization.

The prior dev and stable state directories and packages remain retained.
Simulator r8 deploys the GUI cleanup: obsolete Demo World/manual provisioning
navigation is removed, operational details and read-only policy status live on
Info, and debug details are hidden. The Simulator launch uses its checksum-matched
r8 bootstrap package against the retained r4 installation and workspace; running
production services remain on r7. Their supervisor was not restarted. Frozen
inventory, CA, administrator secret and catalog manifest hashes were unchanged.
Real OIDC login and issuer consent with the installation CA verified the session,
all five original scenario catalog counts and the served cleanup pages/assets.
Live credential-free Chromium checks at 1440px and 834px verified navigation,
Info separation, legacy redirects, no horizontal overflow and no uncaught errors.
Only those credential-free layout checks bypassed the private CA warning;
authenticated API acceptance verified the installation CA.
The source browser selection now passes all 78 desktop/tablet checks. With
explicit user permission, stale removed-asset, old wording and hidden-debug
expectations were updated while preserving approval/run payload, stale-digest,
cross-run isolation, keyboard and production-boundary coverage. Removed assets
must return 404, legacy URLs must redirect, and retained run identifiers must
remain hidden. This rollout is not full original-runtime acceptance.

Dev r7 is a runtime-binary upgrade with unchanged frozen configuration: it
reuses the r4 installation identity, state, Simulator workspace, and credentials.
Existing state is not migrated, reset, or deleted. Dev r7 passed bootstrap
readiness with Simulator stopped, Control Room and Simulator HTTP smoke checks,
LDAP GUI HTTP checks, and served-copy checks for the updated Simulator wording.
Named-user OIDC login with the installation CA verified all five profiles and
their nonempty organization-scoped scenario catalogs through the live API.
The same checks passed after a Simulator-only restart, while Control Room
continued returning HTTP 200. Scenario counts are Northstar 5, Great Lakes 5,
Load Lab 4, Redwood 2, and Velocity 1.
The installed Simulator also retains its original evaluation organization.
The previous pair also passed LDAP administrator sign-in; that sign-in and
full approved-scenario acceptance were not repeated for r7.

[dev.json](dev.json) and [stable.json](stable.json) record the exact non-secret
deployment identity, state paths, ports, source cleanliness, and the checksums
of both release archives. [The dashboard](index.html) provides browser links;
it is static inventory, not a live health monitor.

## Lifecycle

### Staged original-organization operator installations

Independent installs are staged under `/home/dubnom/zpr-dev-organizations-r9`,
separate from the running dev r7/r8 pair. They are **not a full dev r9 release**
and are not yet selectable live runtimes in Simulator.

| Organization | Install ID | People / groups | Nodes | Control Room | Policy-Service |
|---|---|---|---:|---:|---:|
| Northstar | `476d3e3a2e0a8e3e220ad034` | 22 / 17 | 1 | 20787 | 20989 |
| Great Lakes | `ebffca233ad67e24e1789ed4` | 20 / 8 | 3 | 21787 | 21989 |
| Load Lab | `d97e7dee11a747e028fa1b08` | 3 / 1 | 1 | 22787 | 22989 |
| Redwood | `936a18882d3ec7ecb81bc0ac` | 54 / 3 | 2 | 23787 | 23989 |
| Velocity | `0422ecb887a5106caea70975` | 3 / 1 | 1 | 24787 | 24989 |

For prefix 20/21/22/23/24 respectively, Control-Service uses `PREFIX790`,
LDAP `PREFIX636`, phpLDAPadmin `PREFIX789`, logging `PREFIX080`,
OIDC `PREFIX556`, VS admin `PREFIX182`, snapshot delivery `PREFIX793`,
and DNS transfer `PREFIX353`. Policy-Service is loopback-only.
Each exact `zpr-evaluation-dev-ORGANIZATION.service` user unit requires its
independently supervised `zpr-policy-dev-ORGANIZATION.service` and
`zpr-enrollment-dev-ORGANIZATION.service`; stopping the organization stops both
sidecars. Enrollment device HTTPS uses `PREFIX991` and private administration
mTLS uses loopback `PREFIX994`. Velocity's three original units and its new
`zpr-bas-adapter-dev-velocity.service` are currently active; the other twelve
organization units are stopped. None are enabled at boot. Velocity additionally
has enrollment-owned BAS adapter/token listeners at `24996`/`24997`.

Original LDIF inputs were copied once into operator ownership. Runtime
launchers, protected service bindings, directories, credentials, repositories
and state are independent of Simulator. Qualification used no organization
Simulator, checked the **current** systemd invocation (not retained readiness
logs), original people/groups, all 1/3/1/2/1 nodes, protected DNS, logging and
retained identity hashes. Real CA-verified named OIDC login checked independent
policy context and guarded review capabilities. Policy-Service returned exactly
the authenticated organization/target/digest/version read from VS; cross-org
readback was denied. Both units stopped cleanly for every organization.

Evidence is retained on workbench in `qualification-base.json`,
`qualification-policy-services.json` and `qualification-enrollment-services.json`
beneath the staged root. These explicitly
record no original scenario acceptance. The runtime image
`sha256:9f2cb68c4bbf0424888369270f1e503df03bf35e2be69e5a226772e55daa7795`
adds only runtime scripts to the previously qualified image: protected DNS docks
at the actual selected multi-node substrate, and explicit guarded VS startup
configuration is supported. PH/VS/compiler binaries were retained.

Policy-Service initially seeded the clearly labeled bounded installation policy,
not the original workload policies. Velocity now has a separately approved and
applied authentication-root policy, not original workload grants.
Velocity additionally has a separately approved exact benchmark/callback
workload policy (version 4); the other original workload policies/topology,
Simulator lifecycle wiring and all 17 scenario acceptance runs remain open.
Do not treat these qualified production bases as restored original scenarios.

#### Identity-driven approval gate and Velocity-first qualification

The next acceptance target is the original `velocity-single-node` scenario:
two original machines, real named logins, finance-client to echo-service,
100 warmed HTTP round trips and a 16 MiB transfer, followed by cleanup and a
retained-state restart. No further organization rollout is justified by base
readiness alone. Updated user requirement (2026-10-30): turn-key means sign in
and select an organization/scenario, with authorized automatic self-approval
and no further prompts, shell commands or manual state repair in all Simulator
installations; this target is **not met yet**. Audit role-risk warnings are
planned, not deployed. See the
[owning delivery plan](../SIMULATOR_AS_ZPR_CLIENT_PLAN.md#active-turn-key-delivery-plan-2026-10-30).

The currently implemented explicit self-approval gate is available for policy/gateway review,
subject-link review, individual enrollment and atomic inventory batches.
`approval.self` supplements (never replaces) each domain's approval permission.
Issuer plus subject and exact organization grants are authoritative. Submission,
approval and application remain separate actions. Delegation carries the current
session capability and intersects it with the independent backend grant.

At the user's explicit selection, only the existing named administrator
`admin@evaluation.invalid` in staged Velocity received `approval.self`. The
independent reviewer and all other staged organization grants are unchanged.
Protected overlays preserve frozen identity, audience, keys, certificate pins
and prior permissions. The coordinated staged binaries and prior configuration
backups remain under the r9 root; no main dev/stable release was replaced.

Live CA-verified OIDC and backend catalog checks passed, including cross-org
denial. A clean stop/start retained the same four step-referenced endpoint
identities and exact pending inventory digest
`ae3ebfd1bc550183bba5861642d3b272498a87405e74280dce4352a9b0cdfdf5`.
Evidence is `qualification-self-approval-velocity.json` and
`enrollment/velocity/reviewed-inventory.json`. These are **not approval receipts**:
no request was approved, no runtime credentials were issued, and admission and
scenario execution were not accepted. The four claims are not a complete runtime
inventory: the original launcher additionally requires the Simulator control
service adapter. Its retained identity, BAS authentication-service trust and
controller/runtime launch integration were unresolved at that earlier gate.
The four-item evidence remains historical and unchanged; the newer five-item
machine-runtime decision below supersedes its pending count. Northstar's earlier
partial claims are retained, not replaced.

#### Approved Velocity machine-runtime identities and BAS handoff

The user explicitly approved the separate five-item machine-runtime inventory
as `admin@evaluation.invalid`, through the real named production API. It covers
the machine-03 and machine-05 controllers, finance-client, echo-service and
`velocity-simulator-control`, not a complete organization infrastructure inventory.
The exact reviewed digest is
`34166a9fefc9e920df371c135eae39532a64f8b8021dfa2c63004de6fbbe0bdd`.
The authoritative receipt is
`enrollment/velocity/approved-machine-runtime-receipt-r11.json`; the immutable
review snapshot is `enrollment/velocity/reviewed-machine-runtime-inventory-r11.json`.

A clean stop/start of Velocity's exact three units retained the named decision
and all five distinct enrollment/runtime key bindings. Fresh key-authenticated
status reported approved revision 3 for each. The upgraded enrollment-device
command prepared immutable PH authentication files using the authoritative
registry asset ID, explicit BAS CA and the existing retained runtime key.
Original device configurations and both earlier inventory snapshots were retained.

Enrollment Service now optionally owns its BAS listeners with the same Store
in the same process, rather than a second process opening its database.
Only staged Velocity configures them. A separate CA constrained to `auth.zpr`
preserves the installation CA's existing name constraints. Live TLS 1.3 checks
proved all five genuine retained-key exchanges, HS384 signatures and exact
registry claims, pinned VS redemption, nonce/code replay denial, cross-org
denial and route isolation. Evidence:
`qualification-approved-bas-velocity-r11.json`. These native exchanges do
**not** establish authentication-root discovery or through-ZPR admission.

The staged enrollment-service/device binaries were upgraded with retained
`.before-runtime-identity-r11` backups and verified checksums. PH was built
offline and packaged separately as
`sha256:97e4e22925b1351384ded47cb13e23629669472ac519780bfe2907e64818e7c4`;
its binary SHA-256 is
`2e868ed31d2211a49255d910a955d7584f47af78675e6055bb51ff19a5a25c33`.
This PH-upgraded image is deployed **only for the reviewed BAS-root adapter**;
live staged data planes and original machine adapters were not upgraded.
VS and compiler binaries were retained in the new image.
Coordinated PH deployment is required because an older PH may ignore the new
`bas_client_id` setting.

The user separately approved BAS-root proposal
`operator-inputs/velocity/bas-root-r12/proposal.json` and authorized guarded
application. It retains the existing
node/VS/DNS roots and maps registry claims only into `device.enrollment.*`.
Root public-key SHA-256:
`6f1ab5dbea523ebd66aa821c4ce79514280450394eea19847735f5fcf012befa`.
Named save/stage/review/application succeeded against target `velocity-vs`.
Independent Policy-Service-to-VS readback verified policy digest
`8e602f66c228f02feccaa42eb5b1a49e2c755efa96762aeb7c23b061c7e74331`,
version 2. Durable evidence is under
`policy/velocity/bas-root-r12-evidence/`; no direct policy-database edits or
legacy activation bypass were used. Both the retained
baseline and candidate report the same existing warnings about unenforced
VS docking and absent policy-granted VS administration; they were not suppressed
or fixed as unrelated work.

The independent BAS root docks at
`fd5a:5052:adda:1:ffff:ffff:ffff:fffd`. The VS API confirms its connected
`VelocityEnrollmentAuth` service is `Authentication` on TCP 24996. The
TLS-transparent relays retain encryption to Enrollment Service and the existing
VS peer pin. Initial qualification failed because its required `validation/2`
service was missing; the approved r13 correction below supersedes that failure.

Guarded staging first exposed an incompatible compiler. A clean existing
source-qualified compiler worktree was tested offline (310 tests) and installed
only for Velocity, preserving the original shared compiler. The subsequent
live check exposed a compiler dependency bug: the validation companion was
pruned when no returned attribute was referenced. A new serialized-policy
regression reproduced it. The dependency fix passes all 311 compiler tests and
formatting; no pre-existing tests were changed. Its candidate binary SHA-256 is
`f9057cc9f3d2e8ceee00c60dae62ee08621243244813ec2d20b58df3f2890905`.
It is deployed only for Velocity's independent Policy-Service.

The separately user-approved `operator-inputs/velocity/bas-root-r13/` proposal keeps
the exact source, keys, CA, peer and ports, adding only explicit
`expiration_seconds = 300` to the trusted-service configuration. VS otherwise
treats the default zero lifetime as immediately expired. Its configuration
SHA-256 is
`4e81dd843873f02fd217dc99eae2e846ba14b4ef213edc1fc74cef053aa33029`.
The fixed compiler's actual binary-policy dump verifies both ports, signed
`validation/2` metadata and the 300-second bound. Named guarded application
inspected the actual staged binary before approval. Independent readback verified
digest `651da92b2ca098c3a9cb4cdf96d6fd3bf37fa62052c1eb07057ec7884260925d`,
version 3, with evidence under `policy/velocity/bas-root-r13-evidence/`.
The VS confirms both `Authentication` TCP 24996 and `Trusted("validation/2")`
TCP 24997 from the exact reviewed root.

Real endpoint admission then exposed an issuer encoding bug in PH: a socket
address became an unspecified IP at the node. PH now emits a canonical issuer
IP; the consumer accepts IP and legacy socket forms but rejects malformed input.
An actual TLS regression reproduced the producer bug. The full PH suite passed
326 tests with two ignored. The r14 probe image is
`sha256:f69fdc3345758e273433ebd3b008025811d37d9e5ce9edcef99cef7e1a54e170`.
The effective enrollment-layer operator binding was also corrected to use the
already-reviewed VS HTTPS configuration and mounts, preserving OIDC and grants.

The interrupted r14 admission attempt had no success evidence and left an
owned retrying container. On reconciliation it was stopped explicitly and its
private logs retained. Those logs isolated failed VS-to-BAS token transport.
The VS namespace had no route to the BAS address; a live route lookup returned
`Network is unreachable`. The launcher now routes `fd5a:5052::/32` through the
VS adapter for both topologies, matching the protected DNS namespace pattern.
The new regression failed before the fix for single- and multi-node setups;
focused checks and full dashboard `make test` passed after it.

The persistent script-only image
`sha256:322efc741b7ac10d02d2c63de8260b7aa0bacb282a54074a43d5b9576db99d3e`
is installed **Velocity-only**. Its parent is the original staged data-plane
image; PH, VS and compiler binaries are unchanged. Original launcher and checksum
backups have suffix `.before-routing-r15`. After clean stop/start, the new image,
route and both live BAS services were verified. All five retained identities
passed real through-ZPR admission again with exact CN, signed organization/asset
claims, approved runtime-key fingerprint and bounded authentication expiry.
Per-asset evidence is `endpoints/velocity/ASSET/qualification-admission-r14.json`;
the filename identifies the PH probe image, including when run after r15 restart.
The verifier uses the documented numeric Unix `auth_exp` representation rather
than assuming a serialized Rust `SystemTime`. Probes have a 90-second
in-container lifetime and were all cleaned up.

At the r15 routing milestone no original workload policy had been approved/applied.
The separately approved workload milestone below supersedes that policy status;
no original scenario has passed.
Main dev r7/r8 and stable r6 remain unchanged on their fixed ports.

#### Exact Velocity workload authorization

The user explicitly approved complete candidate source SHA-256
`2b28dc776fc2b3987990c0226f12eb24aa0884456eba4c1dd8a77f2ac3813648`
and compiler configuration SHA-256
`f1fa520478d1661928bdd6c9908cbfea5810300068339277a0350c7ba4962da7`.
It composes the unchanged r13 roots with the maintained
[retained workload fragment](../zpr-dashboard/cmd/zpr-web-dashboard/examples/organizations/velocity/retained-runtime-access.zpl).
The only new flows are finance-client to echo on TCP 8080, and the two
controllers to the Simulator callback on TCP 8792. Every new role matches the
exact approved registry asset CN and source-qualified organization, asset ID and
type from `velocity-enrollment-bas`. No fabricated display-name CN, new human
identity trust, other organization grant or bootstrap fallback is added.

Actual binary-policy inspection and evaluation caught a BAS integration gap:
authenticated claims carried source provenance but only raw keys, so they could
not satisfy qualified policy conditions. A new regression failed before the
fix. BAS now reuses the existing trusted-service qualifier, preserving raw
aliases, values, exact source and bounded expiry. Full VS validation passed
848 tests with 24 ignored; the additional opt-in actual-candidate evaluator was
explicitly run and passed all 15 intended/wrong-source/wrong-organization/
wrong-type/wrong-port cases. No pre-existing test was changed.

The earlier VS-only r17 image was
`sha256:b45a254b729236d30af26b2c68f5e8e7bf438c5ef096cbfa4f1e2966c6d8c347`,
based on the r15 routing image. VS binary SHA-256:
`261e39e4d10bd93dad52edf846c848bccb88131c723f9692046a9d3493b50d64`.
PH/compiler/CLI binaries remain unchanged. After scoped restart, all five
approved retained identities passed admission with exact raw and qualified
organization/asset claims. This historical image was Velocity-only; current r20 details follow below.

With the user's separate explicit authorization, a real CA-verified named OIDC
session saved/staged/reviewed/approved/applied the exact workload candidate.
The native staged bytes, scopes, source-qualified identities, validation
metadata and lifetime were inspected before approval. Independent Policy-Service
readback verified `velocity-vs` version 4, digest
`f45f1637b9a8de6024f1ae8ee67d114fcd8d5678c9526bd4d8910cf64fac0765`.
Evidence is under `policy/velocity/workload-access-r16-evidence/`.
Policy-Service uses a separate exact-hash-pinned compiler context; historical
r13 source/configuration and root approval evidence remain unchanged.
All five identities passed admission with qualified claims again under version 4;
temporary probes were cleaned up. This proves neither original traffic nor
scenario acceptance.

#### Velocity release gate and delivery discipline

Velocity remains a diagnostic installation, **not a release candidate**. Freeze
further organization rollout until the original `velocity-single-node` scenario
passes from the Simulator UI. The coordinator chat alone may mutate live runtime;
parallel chats must have bounded, non-overlapping scopes and may not deploy or
edit shared state. The source launcher now wires controller, workload and
callback BAS authentication through the retained `ASSET/identity/` layout.
Only the exact runtime configuration/key/CA files are mounted; retained mode
does not stage shared bootstrap keys. Ordinary test-service and workload cleanup
uses controller-owned child processes. The user approved
explicitly binding the original logical single node to the independent retained
installation, preserving the original people, machines, workloads and benchmark.
Registry/plan/UI source now carries that binding in the approval scope and
displays the physical installation explicitly. Its actual read-only verifier
passed live ownership/image/network/address checks and rejected an incorrect
image. A separate retained-machine consumer now passes the isolated live original
action sequence using the actual bound node. It validates exact files and
controller/image pins, stages protected regular files in private tmpfs, uses
separate Simulator-only mTLS and cleans up only owned container IDs.
The legacy shell launcher still targets the old node layout; it is not used as
a fallback. Canonical Simulator UI/registry rollout remains open. See the
[source-only handoff contract](../zpr-dashboard/packaging/evaluation/README.md#original-velocity-retained-machine-bas-handoff-source-only).

**Current diagnostic milestone (2026-10-30):** Velocity-only base r20 is
`sha256:c820f183c0fb3e05523e925dcebe9b975d045a6f03487e7b8c1da2d9531f6062`.
It preserves signed BAS claims for their authenticated session lifetime instead
of pruning them as a removed directory provider; expired evidence remains denied.
Its data-plane shutdown now signals/reaps owned service and log-reader PIDs.
Endpoint r21 is
`sha256:b6c34997f1e1e542254038b81d84ea2260f47162145bf8961d064ead23348c0c`;
its opt-in source routing keeps each temporary/granted identity on its own TUN.
The retained consumer verifies VS-owned DNS publication without writing the
legacy DNS container. No policy/root change was made.

All 13 original actions and seven original cleanup actions pass in the sequential
diagnostic, including the two original named machine owners, real workloads and
HTTP 200. Two clean repeats completed in 7.89/7.90 seconds; exact benchmark
assertions verified 100 samples and 16,777,216 bytes in 418.90/417.61 ms.
Normal independent base stop/restart restored retained file ownership without
repair; all actions then passed in 23.62 seconds with a 421.96 ms benchmark.
All diagnostic machine/callback containers are gone and the four independent
Velocity services remain active with the diagnostic listener absent.
The one-time old-image transition restored one generated DNS snapshot owner
before qualification; no successful run needed shell repair or bootstrap fallback.
Full VS, PH macOS/Linux and dashboard suites pass, including the exact-child
shutdown fixture. These results do **not** qualify canonical Simulator UI/resource
approval, parallel scheduling, failure-path lifecycle or a matched package rollout.

The next UI dependency is now explicit, not another workload repair: the running
Velocity issuer registers only `zpr-control-room` with the Room callback, and
Control-Service trusts only its existing `evaluation-room` delegation key.
An isolated Simulator requires its own ordinary OIDC client and separately
reviewed delegation signer, using the existing named administrator and scoped
only to Velocity. Do not copy the Room signing key, bypass login, infer approval,
change the approved workload policy/BAS root, or replace main dev to avoid this
registration. This client/trust change awaits review before implementation.

The canonical current acceptance record is
`staged_organization_runtimes.velocity_release_gate` in [dev.json](dev.json).
It records exact runtime images, evidence paths, thresholds and outstanding checks.

| Acceptance check | Current result |
|---|---|
| Approved root policy and both services | Verified |
| Five approved retained-key admissions, repeated after clean restart | Verified |
| Temporary probe cleanup | Verified |
| Original controllers, workloads and callback adapter use retained BAS | Verified in isolated original-action diagnostic; UI rollout pending |
| Original workload policy explicitly reviewed/applied | Verified (exact version-4 benchmark/callback grants) |
| Select and run original Velocity from Simulator UI with real named logins | Pending |
| 100 warmed HTTP requests and 16 MiB transfer within 75 seconds | Verified diagnostic; UI acceptance pending |
| Scenario cleanup, repeat run and restart recovery | Verified diagnostic; UI scheduling/failure-path acceptance pending |
| Packaged production services work with Simulator unavailable | Independent base restart/readiness passes without diagnostic listener; full packaged acceptance pending |

No shell repairs, copied bootstrap identities or inferred approvals are allowed
during acceptance. Component tests and standalone admission probes do not satisfy
the scenario gate. At each 60-90 minute engineering checkpoint record the exact
step reached, reproduced blocker/test, packaged change and rerun result, and next
dependency or decision. If work only adds configuration, stop and reassess rather
than expanding prerequisites. Once this gate passes, reuse the proven mechanism
for the remaining organizations; do not rebuild their lifecycle independently.

Validation passed `make test` (all Go packages and 11 operator-stack tests),
affected-package vet, focused approval race tests, 24 desktop/tablet change and
subject-link browser checks and six real-TLS enrollment-review browser checks.
These browser checks do not claim all other onboarding browser tests pass.

The verified package directories remain under
`/home/dubnom/zpr-demo-refresh-20261029-r1`; their checksum-verified compressed
archives are retained in `/home/dubnom/zpr-environments/releases`. Dev uses
`state-dev-20261029-r4`; Stable uses `state-scenario-r6`. Their previous state,
passwords, CAs and package directories remain retained. Frozen installation
configuration changes require the new release/state pair; do not edit the
protected inventory or reuse an earlier state path.

Dev r7 records source revision `d23511ff5c5db874d50262938179d6b403b0195b`;
stable r6 records `78fb89536ed69335b9bda28dfd9fa3ef18b9b396`.
Both record `source_dirty=true`: these are engineering packages built from the
working tree, not clean-source releases. Dev retains the updated Simulator-facing
authorization/resource wording without changing authorization enforcement.
The dev launcher opts into the packaged catalog with `--full-catalog`; stable
retains evaluation-only behavior. Production OIDC grants are unchanged;
additional organization scopes exist only in the Simulator's local login
configuration. The public website is separate and is not published by this package.
The current dev supervisors are `zpr-dev-production-r7` and
`zpr-dev-simulator-r8`; stable remains under its existing r6 supervisors.
To roll back only Simulator, set dev's `simulator_release` in the launcher to
`dev-20261029-r7` and restart only its exact Simulator supervisor. Retain the r8
archives and do not reset state or change ports.

### Fixed ports across releases

Port assignments belong to the environment, not the release number. Keep every
binding in `dev.json`, `stable.json`, and `workbench.sh` unchanged during normal
updates, even when package or state paths change. Dev Control Room/Simulator
remain `18787`/`18788`; stable remains `38787`/`38788`. A port change requires an
explicit operator decision and corresponding OIDC callback/configuration update.

Copies of the inventories and [workbench.sh](workbench.sh) are installed in
`/home/dubnom/zpr-environments`. On workbench, launch each role in a separately
supervised foreground terminal/process:

```sh
sh "$HOME/zpr-environments/workbench.sh" dev production
# After production reports ready, in another terminal:
sh "$HOME/zpr-environments/workbench.sh" dev simulator
```

Replace `dev` with `stable` for the stable pair. Never start a second copy of an
already-running role. Ctrl-C its owning foreground supervisor to stop a role.
Stop Simulator before stopping production. Production does not launch or
require Simulator; stopping Simulator leaves production services operational.
phpLDAPadmin starts as a bundled production-stack component and stops with its
ZPR production role. It listens only on workbench loopback ports `18789` (dev)
and `38789` (stable). From the browser computer, forward whichever environment
you need:

```sh
ssh -N \
  -L 18789:127.0.0.1:18789 \
  -L 38789:127.0.0.1:38789 \
  workbench.local
```

Then open `http://127.0.0.1:18789/` or `http://127.0.0.1:38789/`. The page
prefills only the administrator DN; enter the matching environment's LDAP
administrator password. The GUI is intentionally HTTP on loopback because the
SSH tunnel protects the browser connection. Do not bind it to a LAN address.
These are supervised processes, not boot-enabled system services; host reboot
requires explicit restart. No reboot/clean-host qualification is claimed.

## Sign-in and secrets

Both use `admin@evaluation.invalid`, but have separate credentials and trust.
Refresh the local environments page from the browser computer after a new
release or any credential rotation:

```sh
cd "ZPR Docs/zpr-visaservice/environments"
python3 update-local-credentials.py
```

The updater retrieves only the four demo password files for each state over
SSH and writes `credentials.local.js` with mode `0600`. If you run the copy
stored on workbench itself, use `ZPR_ENV_REMOTE=local` to read the private
state files directly. That file is ignored by Git and is loaded by the local
[dashboard](index.html). Keep each generated copy mode `0600`; never transfer,
commit, publish, or share it. The page shows operator, logger, LDAP
administrator, and LDAP reader credentials with reveal/copy controls. The LDAP
GUI uses the independent `ldap-admin.secret`; its read-only bind account is
`ldap-reader.secret`. Logger access uses its own `logging-admin.secret`.
These are demo credentials only, never production secrets.

Copy/trust only the relevant public `ca.pem` in the browser computer; never
copy CA keys or whole state directories.

OpenObserve is host-loopback only: dev port `15080`, stable port `35080`.
Use an SSH tunnel to the chosen port and the matching CA for browser access.
LDAPS and production mTLS APIs are likewise not public browser endpoints.

## Keeping the page and environments in sync

When stable or dev changes, update the matching inventory JSON, this runbook,
the [dashboard](index.html), and `workbench.sh` together. Record the exact
release, source revision, install ID, state paths, ports, launch contract, and
acceptance evidence; do not copy credentials into those tracked files.
If state paths change, update `update-local-credentials.py` at the same time.
If phpLDAPadmin or its Ubuntu package changes, update the pinned version in
`ldap-ui.Dockerfile`, rebuild the image with `build-ldap-ui.sh`, and include
that image in the matching offline package. Re-run the credential updater after
the new installations are ready.

For the scenario: select the evaluation organization and **Verify endpoint
access policy**, preview, authorize the fifteen-minute evaluation if needed, review
and approve the exact plan, then run. Each run verifies fifteen fresh allowed
and denied peer flows across three real endpoints, with fresh transport-policy
denial evidence. It does not claim Visa-Service ICMP reachability.

## Promotion

Develop and validate changes on dev first. Promotion means selecting the exact
tested matching package pair and recording its source/checksums, not silently
rebinding stable to mutable `main` or copying dev identity/state over stable.
Frozen configuration changes require an explicit new-installation decision.
Update inventories, launch paths and acceptance evidence together.

## Retired deployments

The previous source-built workbench dev (`~/zpr-dev/state-dev`), older packaged
Room on port `28787`, and r5 preview on port `49388` are stopped. Their state,
package/source directories and retained containers are not deleted. Earlier
Mac/Colima inventories are historical and are no longer the active dev/stable
contract; this workbench consolidation does not manipulate Mac runtimes.

Create temporary environments only for bounded tests with distinct ports and
explicit ownership. Stop them after testing; do not delete retained state,
shared volumes, package directories or unrelated containers as cleanup.
