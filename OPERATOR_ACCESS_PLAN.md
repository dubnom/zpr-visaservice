# Production operator access: loopback, ZPR and optional gateway

Date: 2026-10-29

Status: User-approved architectural direction; implementation and qualification
remain planned. This document does not establish current listener protection,
working ZPR access, or a qualified gateway-disable operation.

Release gate: **OPA-1**, required by the
[system release plan](System%20release%20plan.md) for a production pilot.

## Required access model

| Path | Purpose | Required lifecycle |
|---|---|---|
| Protected loopback | Local setup, administration and recovery | Remains configured and available independently of gateway enablement |
| ZPR-protected HTTPS | Normal operator access with identity and policy enforcement | Always configured for each shipped operator application; not controlled by the gateway switch |
| Operator browser gateway | Remote bootstrap and optional browser access | Explicitly enabled/disabled independently of the other paths |

Disabling the gateway removes its ingress, not the applications, protected
loopback, or ZPR access. Gateway-off must stop new access and terminate existing
gateway-mediated sessions/connections. Re-enabling requires authorized action.

"Always available through ZPR" means a supported, configured access path, not
guaranteed reachability during fabric or authorization-service outages. Local
recovery must not require a functioning fabric, gateway, protected DNS service
or Simulator.

Scope: Control Room, Audit, LDAP administration GUI and logger GUI when shipped.
Simulator uses the same access pattern where installed, but remains a separate
optional lab/engineering application, never a production dependency. Its outage
must not impair the other tools. This plan does not require installing a new
LDAP GUI or change the approved logging provider selection.

## Trust and listener boundaries

- Every path retains application authentication, organization scope and
  operation-level authorization. ZPR reachability is not administrator authority.
- Loopback is not anonymous or automatically trusted. Bind only the intended
  IPv4/IPv6 loopback addresses, reject untrusted browser-origin mutations, and
  use explicit host/origin/session protections. Do not trust caller-supplied
  forwarded identity headers.
- Record which host and network namespace each loopback listener belongs to.
  Container loopback is not host loopback; publishing container ports must not
  create wildcard underlay exposure.
- Expose application backends to their ZPR adapters and explicitly authorized
  gateway connections only. No directly reachable underlay port may bypass
  the intended ingress restrictions.
- Use verified named-user identity at the gateway and applications. A gateway
  service credential is not a shared operator identity or blanket access grant.
- Preserve separate permissions for audit evidence, logs, directory reads and
  edits, policy changes, signing and recovery.
- HTTPS origins, certificates, cookies, redirect/callback URLs and logout must
  be defined for all supported paths. Do not assume one path's browser session
  safely transfers to another origin.
- Bootstrap reachability must not permit unauthenticated first-admin takeover.
  Define company-specific initial authorization before accepting remote setup.

## Independent recovery

Keep local authentication/recovery usable when normal network identity
dependencies are down. This requires an explicit narrowly scoped local recovery
mechanism, not disabling authentication when the IdP is unavailable.

Specify protected credential storage, local authorization, audit, rotation and
recovery operations. Normal operation can use company OIDC; the independent
recovery mechanism must be separately authorized and must not silently expand
into remote access. Document console/SSH-forwarded access and its trust boundary
if supported; do not assume SSH is installed or expose a new daemon implicitly.

Select exact recovery semantics before implementation. Merely binding a GUI
to localhost does not establish authenticated outage recovery.

## Operator gateway switch

Use an explicit scope such as **Enable operator browser gateway access**.
This is not a global switch for unrelated service gateways, proxies or ZPR.

The control/status contract must include:

- Authorized organization/deployment scope and expected configuration revision.
- Desired enabled state, observed ingress state, draining/termination progress,
  failure reason and last verification time.
- Idempotent operation identity, concurrent-update protection, timeout/readback
  semantics and audit attribution.
- A persistent disable state that survives service/host restart and upgrade.
- An explicit rollback/restore rule that never silently restores remote access.
- Enforcement at the actual ingress and session owners, not just removal of
  links, a browser flag, or rejection of new login attempts.

Audit every enable/disable attempt and completion. Do not call a timeout
"disabled"; report unknown or failed until authoritative readback confirms.
Define and test a shutdown deadline for HTTP keepalive, HTTP/2, WebSockets,
CONNECT/tunnels if supported, upstream forwarding and other long-lived sessions.
Invalidate gateway access sessions so retained cookies cannot resume access on
re-enable without the selected fresh-authentication rule. Do not terminate
independent loopback or direct ZPR operator sessions as a side effect.

Warn before disabling when the intended operator's ZPR access has not been
verified. The verification must use the real ZPR path and relevant authorization,
not the gateway or a server-local health probe. Permit an explicit authorized
local override for recovery; do not require a healthy fabric to turn ingress off.
Report clearly if the caller's current gateway connection will be closed.

## Implementation phases

### A0. Inventory and contract decisions

- Identify authoritative runtime branches/configuration and preserve unrelated
  work in progress.
- Inventory every shipped GUI, API, backend port, container publication,
  gateway, login callback, health probe and startup dependency.
- Map current application identities, ZPR service registration and policy.
- Select local recovery authentication, gateway ingress scope, first-admin
  bootstrap rules, session termination and restart/restore semantics.
- Set measurable readiness, disable/termination and recovery deadlines.

Exit: reviewed access matrix and explicit configuration/control contract.
Unresolved recovery authentication blocks implementation acceptance.

### A1. Protected loopback and backend isolation

Prerequisite: A0.

- Establish authenticated local entry points and supported recovery operations.
- Remove unintended published listeners through approved deployment changes.
- Preserve browser protections and application authorization on local access.
- Verify container/host namespaces, IPv4/IPv6, and any supported local forwarding.

Exit: local setup and recovery work with ZPR and normal identity dependencies
unavailable; unauthorized local callers do not acquire administration.

### A2. Permanent ZPR access

Prerequisite: A0; can overlap A1 after contracts stabilize.

- Register operator services and adapters through production configuration.
- Apply explicit least-privilege user/device/service policies.
- Integrate HTTPS identity, callbacks and per-application permissions.
- Prove real authenticated allowed and denied access from provisioned clients.

Exit: every shipped tool works through ZPR without the browser gateway, including
API actions and supported persistent connections. Simulator remains optional.

### A3. Gateway enable/disable lifecycle

Prerequisites: A1-A2 for full acceptance.

- Implement separately scoped configuration, authorization, status and audit.
- Stop new ingress, terminate existing gateway sessions and confirm completion.
- Add ZPR-path verification/warning and explicit local override.
- Persist off-state and handle restart, concurrent commands, failures and retries.
- Distinguish remote ingress from internal-only proxy plumbing required by the
  protected application path; disabling one must not accidentally break the other.

Exit: an operator can disable and re-enable the gateway with truthful results
while loopback and direct ZPR access continue.

### A4. Packaging, recovery and release qualification

Prerequisites: A1-A3.

- Ship the access matrix, ports, policies, initial authorization and runbooks.
- Rehearse fresh bootstrap, gateway shutdown, upgrades and isolated restore.
- Run the OPA-1 matrix against exact release artifacts on the supported host.
- Attach measured results and known exclusions to the release record.

Exit: all required OPA-1 cases pass; no overall production claim from a
developer-local probe or partial application inventory.

## OPA-1: blocking production acceptance matrix

Record each case as passed, failed, blocked or not run. Missing evidence leaves
the gate open.

| Case | Required evidence |
|---|---|
| Fresh bootstrap | Company-specific first administrator can use protected loopback before ZPR is operational; no anonymous takeover |
| Remote bootstrap | Explicitly enabled gateway supports authenticated setup without widening backend exposure |
| Application coverage | Each shipped tool and required API has documented loopback/ZPR/gateway behavior and exact listener ownership |
| Direct ZPR operation | Real provisioned client reaches each tool under allowed policy with gateway off |
| Policy and application denials | Denied user/device/service and unauthorized organization/action fail through actual ingress paths |
| Gateway off | New ingress denied and pre-existing sessions/streams terminated within the agreed deadline; no UI-only shutdown |
| Path independence | Loopback and direct ZPR sessions continue when gateway ingress is disabled |
| Recovery independence | Authorized local recovery works during ZPR, gateway and IdP outage without silent auth bypass |
| Exposure | External and host/container IPv4/IPv6 probes find no unintended wildcard/published backend or bypass path |
| Browser/identity isolation | Origins, callbacks, cookies, CSRF and verified identity propagation preserve individual authority |
| Persistence | Disable state survives gateway/application/host restart and supported upgrade |
| Restore and rollback | Restored or older configuration cannot silently reopen remote ingress; recovery behavior is explicit and exercised |
| Failure semantics | Lost replies, concurrent changes and failed termination report unknown/pending/failed rather than false success |
| Audit | Named actor, configuration revision, requested state and observed completion retained without session secrets |
| Simulator independence | Production cases pass with Simulator absent and its environment configuration invalid/unset |

For an optional GUI not shipped, record not applicable with rationale. A shipped
GUI with no protected access integration is a blocker, not not-applicable.

## Estimate and remaining decisions

Re-estimate after A0. A useful preliminary allowance is 3-5 engineer-days for
inventory/contracts, 8-16 for listener/recovery/ZPR integration, 5-9 for the
gateway lifecycle, and 5-9 for packaging/qualification: **21-39 engineer-days
before contingency**. These judgment ranges assume reusable authentication and
gateway primitives; a new local recovery mechanism or third-party GUI integration
may exceed them. They exclude implementing Lite or a new identity provider.

Open decisions include recovery credentials/operations, exact gateway placement,
third-party application integration, shutdown bounds, initial gateway enablement,
cross-origin login handling and supported restore safeguards. The architectural
requirement is approved; those implementation choices are not implicitly settled.
