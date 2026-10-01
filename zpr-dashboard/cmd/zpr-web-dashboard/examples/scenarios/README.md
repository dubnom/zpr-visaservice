# Simulator Scenarios

Each `.json` file in this directory is one selectable simulator scenario. The
file's `id` must match its filename. Scenarios are loaded from
`SIMULATION_SCENARIOS_DIR`; the dashboard stack points that setting at this
directory.

Supported step actions are `start_machine`, `wait_controller`, `login`,
`select_workloads`, `start_workload`, `traffic`, `stop_workload`, `logout`,
`stop_machine`, and `delay`. Steps are run in order. `traffic` probes the
component's manifest namespace and target, or an explicit IPv6 `target`, and
requires `expected` to be `allow` or `deny`.

An optional `cleanup` list runs after success, failure, or cancellation. Keep it
to `stop_workload`, `logout`, and `stop_machine` actions so partial scenarios do
not leave simulated sessions or workloads behind. Scenario files cannot run
arbitrary shell commands. Step timeouts are optional and capped by the runner.

See `machine-policy-deny.json` for a machine/login/workload flow and
`default-deny.json` for a policy-denial probe.

`active-team-cycle.json` keeps four user sessions and four client/service
workloads active together, probes two policy-gated flows, and restarts one
machine. Scenario files may reference at most ten distinct machines, and the
runner also refuses to start a machine if that would put the live fleet above
ten.