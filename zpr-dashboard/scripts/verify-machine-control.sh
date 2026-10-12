#!/bin/sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
DASHBOARD_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
RUNTIME_DIR=$(CDPATH= cd -- "$DASHBOARD_DIR/../../.local-runtime" && pwd)
STATE_DIR="$RUNTIME_DIR/dashboard-stack"
ORGANIZATIONS_DIR="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/organizations"
ACTIVE_ORGANIZATION_FILE="$RUNTIME_DIR/active-organization.txt"
SIMULATION_MANIFEST="${SIMULATION_MANIFEST:-$RUNTIME_DIR/simulation-environment.json}"
. "$SCRIPT_DIR/machine-runtime.sh"
SIMULATION_EXPECTED_ORGANIZATION=${SIMULATION_ORGANIZATION_ID:-}
resolve_machine_runtime
acquire_machine_runtime_lock
probe_container=
capture_readiness_io() {
    diagnostic_container=$1
    echo "Readiness I/O diagnostics: $diagnostic_container" >&2
    docker exec "$diagnostic_container" sh -c '
        grep "^Udp:" /proc/net/snmp
        ss -uamn | head -n 80
        ps -eLo pid,tid,stat,wchan:32,comm | grep -E "ph$|fastpath"
    ' >&2 || echo "readiness I/O diagnostics unavailable: $diagnostic_container" >&2
}
cleanup() {
    probe_result=$?
    trap - EXIT HUP INT TERM
    if [ -n "$probe_container" ]; then
        if [ "$probe_result" -ne 0 ]; then
            docker logs --tail 80 "$probe_container" >&2 || echo "probe controller logs unavailable" >&2
            docker exec "$probe_container" tail -n 80 /tmp/machine-control-ph.log >&2 || echo "probe adapter logs unavailable" >&2
            capture_readiness_io "$probe_container"
            if [ "$machine_runtime_driver" = docker-multinode ]; then
                diagnostic_nodes=$(jq -er '.runtime.nodes | keys | .[]' "$machine_profile") ||
                    echo "configured readiness diagnostic nodes unavailable" >&2
                for diagnostic_node in $diagnostic_nodes; do
                    capture_readiness_io "$machine_organization-node$diagnostic_node"
                done
                capture_readiness_io "$machine_organization-vs"
                docker exec "$machine_organization-node0" tail -n 80 /tmp/zpr-machine-control/adapter.log >&2 ||
                    echo "SimulatorControl adapter diagnostics unavailable" >&2
                docker exec "$machine_organization-vs" tail -n 80 /logs/vs-adapter.log >&2 ||
                    echo "Visa adapter diagnostics unavailable" >&2
            fi
        fi
        if ! docker stop --time 5 "$probe_container" >/dev/null || ! docker rm "$probe_container" >/dev/null; then
            echo "machine-control readiness probe cleanup failed: $probe_container" >&2
            probe_result=1
        fi
    fi
    release_machine_runtime_lock || probe_result=1
    exit "$probe_result"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
if [ "$machine_runtime_driver" = docker-multinode ]; then
    probe_machines=$(select_machine_readiness_probes)
else
    probe_machines=machine-01
fi
for probe_machine in $probe_machines; do
    echo "Checking $machine_organization/$machine_generation machine-control round-trip: $probe_machine"
    probe_container="zpr-$probe_machine"
    probe_existing=$(docker inspect -f '{{.State.Running}}' "$probe_container" 2>/dev/null || true)
    [ "$probe_existing" != true ] || { probe_container=; echo "readiness requires idle machines" >&2; exit 1; }
    SIMULATION_EXPECTED_ORGANIZATION="$machine_organization" sh "$SCRIPT_DIR/dashboard-stack.sh" start-machine "$probe_machine"
    curl -fsS --connect-timeout 3 --max-time 45 \
        --cacert "$STATE_DIR/machine-control/control-ca.crt" \
        --cert "$STATE_DIR/machine-control/machine-certs/$probe_machine/client.crt" \
        --key "$STATE_DIR/machine-control/machine-certs/$probe_machine/client.key" \
        -H "X-ZPR-Organization: $machine_organization" \
        -H "X-ZPR-Generation: $machine_generation" \
        -X POST https://127.0.0.1:8791/internal/readiness
    docker stop --time 5 "$probe_container" >/dev/null
    docker rm "$probe_container" >/dev/null
    probe_container=
done
