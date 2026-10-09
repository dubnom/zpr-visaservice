# Simulator Scenarios

The Scenarios page shows the active organization as text and lists only that
company's scenarios. To switch companies, select one on the Organizations page
and click **Activate organization** on its details. Finish or cancel running scenarios
and log out all machine users before switching. The choice survives simulator
restarts, using `active-organization.txt` beside the simulation manifest (or
`SIMULATION_ACTIVE_ORGANIZATION_FILE`). Activation does not deploy network
policy or reseed LDAP; provision those separately before running a new company.

Each `.json` file in this directory is one selectable simulator scenario. The
file's `id` must match its filename. Scenarios are loaded from
`SIMULATION_SCENARIOS_DIR`; the dashboard stack points that setting at this
directory.

Supported step actions are `start_machine`, `wait_controller`, `login`,
`select_workloads`, `start_workload`, `traffic`, `resolve_dns`, `stop_workload`, `logout`,
`stop_machine`, `start_test_service`, `request_test_service`,
`stop_test_service`, `start_service_fleet`, `stop_service_fleet`,
`stress_traffic`, `benchmark_test_service`, and `delay`. Steps are run in order. `traffic` probes the
component's manifest namespace and target, or an explicit IPv6 `target`, and
requires `expected` to be `allow` or `deny`.

`resolve_dns` requires an authenticated machine with the selected client workload,
a configured resolver in the simulation manifest, and a `.zpr` target name. It
queries the configured DNS server over TCP and records the returned IPv6 address.

An optional `cleanup` list runs after success, failure, or cancellation. Keep it
ordered so workload stops and logout precede machine shutdown. Each cleanup
step gets an independent share of the three-minute cleanup budget, so early
timeouts cannot starve final shutdown. Machine shutdown uses bounded best-effort
logout and still stops the container if its controller is unreachable.
The managed local rig does not boot standalone copies of machine workloads;
after recovery from older rigs, `scripts/dashboard-stack.sh stop-legacy-workloads`
withdraws those copies without stopping DNS or platform infrastructure.

Keep cleanup
to `stop_test_service`, `stop_service_fleet`, `stop_workload`, `logout`, and `stop_machine` actions so partial scenarios do
not leave simulated sessions or workloads behind. Scenario files cannot run
arbitrary shell commands. Step timeouts are optional and capped by the runner.

Scenarios run sequentially by default. Set `"parallel": true` to run each
machine's steps on its own timeline. Every run step then needs a unique `id`;
use `"after": ["earlier-step-id"]` to wait for a step on another machine (or
the Shared lane). References must point to earlier steps in the JSON file.
Steps on the same machine always retain their listed order, and steps without
a machine share one ordered lane. A failed step cancels all lanes; cleanup runs
in its listed order after the lanes stop. Cleanup steps do not use `id` or
`after`. See `client-service.json` for a client waiting on a service and shared
pauses between requests.

The Scenarios page groups planned steps and results by machine, with a Shared
lane for steps without a machine. Step numbers retain the scenario's original
order, even when parallel steps complete out of order.

See `machine-policy-deny.json` for a machine/login/workload flow and
`default-deny.json` for a policy-denial probe.

The Velocity Labs company includes `velocity-single-node.json` for a bounded
two-machine HTTP throughput and latency baseline. Measurement semantics and
runtime prerequisites are in `../organizations/velocity/README.md`. It does not
yet measure direct visa grant time.

`client-service.json` runs a real HTTP GET from the finance-client workload's
ZPR address to an echo-service process bound to its ZPR address on port 8080.
It requires the `EchoWeb` TCP 8080 grant in the runtime policy and a rebuilt
machine-controller binary. Client events appear in the request step output;
the service appends timestamped JSON lines inside its machine container at
`/tmp/zpr-echo-service.jsonl`. The service is stopped in cleanup. The Agents
page can show these events live from the View logs switch on each workload. Each selected
workload has its own loopback-only log service in the machine container:
finance-client 18081, operations-client 18082, telemetry-client 18083,
echo-service 18084, and metrics-service 18085. Events include a UTC timestamp,
sent/received direction, and a machine client identifier; workloads without
application traffic have empty feeds. The Simulator reads one selected feed
through the machine controller and stops polling when viewing is disabled.

`internet-gateway-egress.json` starts the separately keyed `internet-gateway`
adapter on machine-07 and requests `/health` from the Finance client over ZPR.
The policy class `internet-gateway` carries the trusted
`external-network-connection:public-internet` attribute, and Finance alone is
granted access to its TCP 8082 service. The handler fetches only the fixed
`https://example.com/` origin; it is not a CONNECT proxy and accepts no
client-selected hostname. The final HTTPS fetch uses the machine container's
ordinary egress path; the client-to-gateway request and reply traverse ZPR.
The runtime policy must be rebuilt with the `InternetGatewayWeb` grant and the
simulator-only `internet-gateway` bootstrap key must be installed in the rig.

Great Lakes organization deployment starts the `internet-gateway` adapter and
allowlisted web proxy as a persistent service. The
`great-lakes-finance-web-gateway.json` scenario starts only a Finance client,
resolves `internet-gateway.svc.zpr`, and probes `/health` over ZPR; it does not
start or stop the Gateway. While the organization is active, configure an HTTP
proxy client to `internet-gateway.svc.zpr:8082`. It accepts HTTP requests on
port 80 and HTTPS CONNECT tunnels on port 443 only for `google.com` and its
subdomains. The Great Lakes runtime policy grants access only to
`MilwaukeeFinance`; the gateway rejects non-public DNS results and pins each
connection to a validated public address. This is Simulator-only and does not
configure production Control Room gateways.

`active-team-cycle.json` runs for about five minutes: four users and four
client/service workloads generate recurring denial probes and real HTTP requests
separated by 30-second quiet windows. The finance client calls echo on port 8080,
and the operations client calls metrics on port 8081. Machine-06 restarts the
metrics app after its shutdown/restart midway through. The running demo policy
must include both `EchoWeb` and `MetricsWeb` TCP grants for these calls to succeed.
The runner routes client traffic through its selected PH link using the ZPR
address granted to that client, and routes service replies through the service
PH link rather than the machine-control link.
Scenario files may reference at most ten distinct machines, and the runner also
refuses to start a machine if that would put the live fleet above ten.

`load-lab-fanout.json` starts 200 service ports on machine-05 and runs 200
logical HTTP clients from machine-03 for two minutes. Each request opens a fresh
TCP connection to a randomly selected service; each logical client periodically
goes offline for a short pause and resumes. These clients share one
`finance-client` ZPR adapter identity/address, so this exercises flow and service
fan-out rather than hundreds of independent adapter identities. Load Lab's
README documents the generated runtime policy, provisioning steps, and bounds.