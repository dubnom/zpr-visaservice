# Workbench environments

The two active environments run independently on `dubnom@workbench.local`
(`10.0.1.160`). Updated 2026-10-29.

| Environment | Control Room | Simulator | Package source |
|---|---|---|---|
| Dev | <https://10.0.1.160:18787/> | <https://10.0.1.160:18788/scenarios.html#simulate> | `55dfbf1`, current-main build `dev-20261029-r1` |
| Stable | <https://10.0.1.160:38787/> | <https://10.0.1.160:38788/scenarios.html#simulate> | `f5341d3`, tested package `scenario-20261029-r3` |

Stable means the retained, tested **engineering demo baseline**, not a
production-qualified release. Both provide independent production-tagged
Control Room/Control-Service, Visa Service, protected DNS, LDAP, OIDC, logging
and a separately supervised Simulator. Policy-Service is not configured.
Each has one bounded endpoint-policy scenario, not full onboarding or the
complete scenario catalog.

Dev's build metadata records `source_dirty=true`: concurrent documentation
edits were present while packaging the pinned `55dfbf1` runtime. Both pairs
passed real sign-in and approved fifteen-probe scenario execution on 2026-10-29.
A clean-source dev rebuild is still pending command-execution approval; do not
describe this dev artifact as a clean-source release.

[dev.json](dev.json) and [stable.json](stable.json) record the exact non-secret
deployment identity, state paths and ports. [The dashboard](index.html) provides
browser links; it is static inventory, not a live health monitor.

## Lifecycle

The verified package directories remain under
`/home/dubnom/zpr-demo-refresh-20261029-r1`. Stable retains its existing state,
password, CA and ports unchanged. Dev uses new state because browser/Simulator
settings are frozen at installation; the old dev state is retained.

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
These are supervised processes, not boot-enabled system services; host reboot
requires explicit restart. No reboot/clean-host qualification is claimed.

## Sign-in and secrets

Both use `admin@evaluation.invalid`, but have separate credentials and trust:

- Dev state: `~/zpr-demo-refresh-20261029-r1/state-dev-20261029-r1`.
- Stable state: `~/zpr-demo-refresh-20261029-r1/state-scenario-r1`.

Read `operator-admin.secret` privately on workbench for the operator password.
The logger uses its own `logging-admin.secret`, not the operator password.
Copy/trust only the relevant public `ca.pem` in the browser computer; never
copy CA keys or whole state directories. The refreshed dev CA/password differ
from the retired dev installation. Stable's are unchanged.

OpenObserve is host-loopback only: dev port `15080`, stable port `35080`.
Use an SSH tunnel to the chosen port and the matching CA for browser access.
LDAPS and production mTLS APIs are likewise not public browser endpoints.
No LDAP administration GUI is included in these pairs.

For the scenario: select the evaluation organization and **Verify endpoint
access policy**, preview, approve the fifteen-minute budget if needed, review
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
