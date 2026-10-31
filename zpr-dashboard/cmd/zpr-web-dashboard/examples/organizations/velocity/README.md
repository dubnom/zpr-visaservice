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

For the separately enrolled runtime, [retained-runtime-access.zpl](retained-runtime-access.zpl)
is an opt-in fragment, not an automatically active policy or replacement for
the legacy catalog. Compose it with the independently approved BAS-root policy
and exact compiler configuration. It preserves `EchoWeb` and
`SimulatorControlService`, with only finance-client to echo on TCP 8080 and the
two controllers to the Simulator callback on TCP 8792. It matches authoritative
registry asset IDs and source-qualified organization, asset and type claims;
display names such as `finance-client` are not authentication identities.
It adds no human identity claims, directory trust or self-approval permissions.
Real machine-owner login remains a separate controller requirement.

The matching compiler configuration must define `EchoWeb` as TCP 8080 and
`SimulatorControlService` as TCP 8792 and retain the BAS claim mappings. Visa
Service must expose qualified BAS claims; a successful compile alone does not
prove this. The opt-in `retained_velocity_policy_evaluates_verified_bas_claims`
Rust test accepts `ZPR_TEST_VELOCITY_POLICY` pointing to the compiled full
candidate and checks intended access plus wrong-source, organization, type and
port denial. Run it explicitly with `cargo test -p vs
retained_velocity_policy_evaluates_verified_bas_claims -- --ignored`.
Any complete candidate still requires separate named policy review and guarded
application. The original single-node topology and scenario are unchanged;
this fragment does not resolve a mismatched runtime topology or grant a
bootstrap fallback.

The client has a 60-second overall budget and a 10-second per-request timeout.
Failure produces a failed scenario step, not a successful measurement. Ordered
cleanup stops both workloads and machines. Other company profiles are unchanged.