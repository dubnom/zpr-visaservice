# Great Lakes Instruments Demo

Great Lakes models a three-site instrumentation manufacturer with a Milwaukee
headquarters, a Shenzhen engineering office, and a Tijuana assembly and test
plant. Its profile contains twelve employees, eleven department nodes, eight
LDAP role groups, twelve employee-owned laptops, and one engineering-managed
business-service host (`machine-13`, owned by James Wu). Three Tijuana laptops
join the workday scenario; Chen Yu's Shenzhen laptop is `machine-09`.

## Directory And Assertions

`directory.ldif` is the seed for `dc=greatlakes,dc=test`. It includes every
employee, computer, application identity, department membership, site, and
machine owner. The synthetic `finance-client` workload has a non-human ZPR
device identity with Milwaukee Finance attributes so its gateway smoke test
uses the same trusted policy claims as the Finance department; it is excluded
from the human directory roster. The profile's `assertions_source` checks
directory identity and attribute completeness, department/group alignment, and
membership boundaries.

The runtime grants use trusted **machine** department and site attributes:
`ou -> device.greatlakes.department` and
`zprMachineLocation -> device.greatlakes.location`. The eight LDAP groups model
the corresponding employee roles; they are not themselves runtime group-claim
grants. Simulator login selects an allowed machine owner, not a cryptographic
human authentication session. Do not interpret these scenarios as proof of
per-user authentication or group-claim enforcement.

## Policies And Services

The runtime policy is composed with the shared bootstrap and platform layers.
It keeps the seven business applications restricted by department and site:
Customer Support, Engineering Build, Finance, Shipping, Receiving, Assembly,
and Quality Test. Workday Echo and Workday Metrics are additional simulation
endpoints used to create observable client/server activity; Finance can reach
Echo, while Operations and Telemetry can reach Metrics. Cross-service probes
are expected to fail.

The organization-started Internet Gateway runs with the Great Lakes Compose
stack and is granted only to Milwaukee Finance. It accepts HTTP/80 and HTTPS
CONNECT/443 for `google.com` and subdomains, rejecting non-public resolved
addresses. The `Great Lakes Finance web gateway` scenario tests the persistent
service; it does not start or stop the gateway.

The three ZPR sites are named `mke.zpr`, `shenzhen.zpr`, and `tijuana.zpr`.
The shared ZPR DNS service publishes service records such as
`echo-web.svc.zpr` and `metrics-web.svc.zpr`. The workday scenario explicitly
resolves them over the authenticated client's ZPR workload link.

## Stateful Business Simulations

These are small simulated business APIs, not deployed commercial applications.
The source includes seven independently registered providers, each with its own
adapter, private key, TCP port, persistent state file, and request log:

| Workload | DNS name | Port | Record transition | Allowed employee departments/sites |
| --- | --- | --- | --- | --- |
| `support-desk-app` | `customer-support.svc.zpr` | 8443 | open -> resolved | Milwaukee Customer Support |
| `engineering-build-farm` | `engineering-build.svc.zpr` | 8444 | queued -> built | Milwaukee Engineering; Shenzhen Engineering |
| `finance-workspace` | `finance-workspace.svc.zpr` | 8445 | draft -> approved | Milwaukee Finance |
| `shipping-portal` | `shipping-portal.svc.zpr` | 8446 | queued -> dispatched | Milwaukee Shipping |
| `receiving-portal` | `receiving-portal.svc.zpr` | 8447 | expected -> received | Milwaukee Receiving |
| `assembly-mes` | `assembly-mes.svc.zpr` | 8448 | planned -> assembled | Tijuana Assembly |
| `quality-test-bench` | `quality-test.svc.zpr` | 8449 | pending -> passed | Tijuana Test and Quality; Milwaukee Engineering |

All seven run on the separate service host so a laptop request cannot bypass
ZPR through Linux local delivery. Each provider must pass its exact health check
before its DNS record is published. Employee requests use their machine's
authenticated `tun5` adapter and Visa-assigned address, without a generic client
identity or caller-supplied identity header.

The two **Business** scenarios are **Great Lakes: HQ and Shenzhen business
access** and **Great Lakes: regional business access**. Each starts six employee
laptops plus the provider host, exercises 42 employee/service pairs, and cleans
up its services, workloads, logins, and machines even after failure. Together they
cover all twelve employees against all seven providers: **13 allowed and 71
denied**. Allowed requests create, read, advance, and reread a record. Denied
requests require an increasing Visa Admin denial counter for the exact
source/destination/TCP/port tuple; DNS failure, service failure, stale counters,
and timeout alone do not pass. They do not test real human authentication.

The APIs expose `GET /health`, `GET /records`, `POST /records` with an
`id` and `summary`, `GET /records/{id}`, and `POST /records/{id}/advance`.
IDs are at most 80 lowercase letters/digits/hyphens; summaries are nonempty and
at most 256 bytes. Each service retains at most 1,000 records and returns HTTP
507 at capacity, without silently deleting earlier work. Duplicate creation and
repeated advancement return HTTP 409. Corrupt or incompatible persisted state
prevents startup. Mutations require a pre-write audit event and atomic state
replacement; a failed final event log can return HTTP 500 after the state was
committed, so inspect the record before retrying.

`business_simulation: true` requires Docker multinode provisioning and the
seven matching `workload_services` registrations. Provisioning generates private
provider keys under the organization's runtime `business-keys/` and preserves
them on repeated preparation. Machine state is a separate writable bind at
`/var/lib/zpr-business`, outside the read-only workload assets. Restarting or
scenario cleanup retains the records; a new organization runtime generation
starts a new dataset. Do not remove the state's generation to clear individual
records while it is running.

The Policy Studio example catalog now mirrors the runtime's department/site
grants, including shared Engineering/Quality access and Finance gateway access.
Catalog records remain authoring examples (`live: false`); importing them does
not replace published operator edits or automatically install runtime policy.

Software deployment is separate from organization provisioning. Existing running
containers do not gain the new binary, keys, or providers merely because this
directory changes. The business-service software/controller prebuilds were
installed on 2026-10-15 UTC, followed by the Restore-feedback UI. The user's
Restore failed policy-assertion verification and the previous runtime was
restored and verified; a post-rollback settings probe found an empty assertion
source. Neither live Business matrix has run.

Repair and verify assertion seeding before a separately reviewed Restore retry,
then run both scenarios for live forwarding acceptance. Restore backs up and
resets saved policy/directory configuration; it does not install a new executable
or automatically run the Business scenarios. The Organizations page shows
progress, rollback and outcomes with logs during execution; operation status
currently clears on Simulator restart, while local logs remain. Control Room and
Control-Service have no dependency on these simulations or their availability.

## Workday Simulation

Activate Great Lakes Instruments, then run **Great Lakes: five-minute
workday**. It starts and signs in all twelve employee laptops, brings up Echo
and Metrics, and runs finance, operations, telemetry, assembly, and quality
activity across eight work blocks. The three Tijuana laptops join during the
day and run intermittent work bursts. Expected cross-service denials remain
part of the scenario.

Client and server request events are available from the Simulator Workers page's
workload logs. DNS resolutions, HTTP requests, and expected-denial probes are
also recorded as scenario steps. Cleanup stops services and workloads, logs out
every employee, and shuts down all twelve computers, including on failure or
cancellation.

The machine image must include `dig`; the normal machine-controller startup
builds it from `scripts/Dockerfile.machine`.
