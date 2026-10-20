# Velocity Labs

Velocity uses machine-03 (client, Alex Rivera) and machine-05 (server, Sam Chen)
on the existing single forwarding node. Select Velocity Labs on the Simulator's
Organizations page to activate its workspace. Activation does not deploy a new
network policy or seed LDAP by itself; provision the company directory through
the existing organization-aware runtime setup before executing its scenario.

The `velocity-single-node` scenario starts the finance-client and echo-service
workloads, checks the service, and runs `benchmark_test_service`. Its JSON step
output contains:

- `latency_p50_ms`, `latency_p95_ms`, `latency_p99_ms`: 100 HTTP round trips on a
  reused connection, excluding an additional warm-up request. These include
  server handling and process logging; they are not raw network RTT.
- `throughput_mbps`: payload goodput from one 16 MiB HTTP download, including
  request/response overhead. This is a bounded baseline, not a sustained-load
  capacity test. No compression is used.
- `bytes` and `samples`: transfer size and latency sample count.
- `visa_grant_timing`: explicitly not measured. Measuring an uncached visa grant
  requires a timer around the authorization request/response and control of visa
  caching. First HTTP request time is not a substitute.

Rebuild the Linux machine-controller executable before running the scenario;
older controllers do not support `benchmark-client` or `/benchmark`. Install the
equivalent of `benchmark-access.zpl` into the runtime's complete signed policy
using the normal policy update workflow. The bundled config/catalog are Policy
Studio examples, not a replacement for the network's infrastructure grants or
trust certificate. Benchmark requests must traverse the workloads' ZPR links.

The client has a 60-second overall budget and a 10-second per-request timeout.
Failure produces a failed scenario step, not a successful measurement. Ordered
cleanup stops both workloads and machines. Other company profiles are unchanged.