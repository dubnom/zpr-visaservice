# Great Lakes Instruments Demo

Great Lakes models a three-site instrumentation manufacturer with a Milwaukee
headquarters, a Shenzhen engineering office, and a Tijuana assembly and test
plant. Its profile contains nine employees, eleven department nodes, eight
LDAP role groups, and nine employee-owned computers. Chen Yu's Shenzhen laptop
is `machine-09`.

## Directory And Assertions

`directory.ldif` is the seed for `dc=greatlakes,dc=test`. It includes every
employee, computer, application identity, department membership, site, and
machine owner. The profile's `assertions_source` checks directory identity and
attribute completeness, department/group alignment, and membership boundaries.

## Policies And Services

The runtime policy is composed with the shared bootstrap and platform layers.
It keeps the seven business applications restricted by department and site:
Customer Support, Engineering Build, Finance, Shipping, Receiving, Assembly,
and Quality Test. Workday Echo and Workday Metrics are additional simulation
endpoints used to create observable client/server activity; Finance can reach
Echo, while Operations and Telemetry can reach Metrics. Cross-service probes
are expected to fail.

The three ZPR sites are named `mke.zpr`, `shenzhen.zpr`, and `tijuana.zpr`.
The shared ZPR DNS service publishes service records such as
`echo-web.svc.zpr` and `metrics-web.svc.zpr`. The workday scenario explicitly
resolves them over the authenticated client's ZPR workload link.

## Workday Simulation

Activate Great Lakes Instruments, then run **Great Lakes: five-minute
workday**. It starts and signs in all nine employee computers, brings up Echo
and Metrics, and runs finance, operations, and telemetry requests across eight
work blocks separated by seven 30-second lulls. Expected cross-service denials
are included. Startup and cleanup bring the full run to about five minutes.

Client and server request events are available from the Simulator Agents page's
workload logs. DNS resolutions, HTTP requests, and expected-denial probes are
also recorded as scenario steps. Cleanup stops services and workloads, logs out
every employee, and shuts down all nine computers, including on failure or
cancellation.

The machine image must include `dig`; the normal machine-controller startup
builds it from `scripts/Dockerfile.machine`.
