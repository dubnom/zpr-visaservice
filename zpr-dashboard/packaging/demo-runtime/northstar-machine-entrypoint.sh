#!/bin/sh
set -eu

if [ "${SIMULATION_ORGANIZATION_ID:-}" != northstar ] ||
    [ "${SIMULATION_NORTHSTAR_MACHINE_ID:-}" != machine-01 ] ||
    ! printf '%s' "${SIMULATION_RUNTIME_GENERATION:-}" |
        grep -Eq '^[a-z0-9][a-z0-9-]{0,127}$'; then
    echo "Northstar machine runtime scope is invalid" >&2
    exit 2
fi

if [ -z "${SIMULATION_NORTHSTAR_CONTROL_URL:-}" ] ||
    [ -z "${SIMULATION_NORTHSTAR_NODE_ADDRESS:-}" ]; then
    echo "Northstar machine runtime endpoints are required" >&2
    exit 2
fi

for path in \
    /run/zpr-machine/control-ca.crt \
    /run/zpr-machine/client.crt \
    /run/zpr-machine/client.key \
    /opt/zpr-workloads/ca.crt \
    /opt/zpr-workloads/client-finance-rsa.key \
    /opt/zpr-workloads/machine-controller-machine-01.key; do
    if [ ! -f "$path" ]; then
        echo "Northstar machine runtime asset is unavailable" >&2
        exit 1
    fi
done

for path in /opt/zpr-workloads/ph /opt/zpr-workloads/ph-cli; do
    if [ ! -x "$path" ]; then
        echo "Northstar machine runtime adapter is unavailable" >&2
        exit 1
    fi
done

exec /usr/local/bin/zpr-web-dashboard \
    -mode machine-controller \
    -machine-id machine-01 \
    -control-url "$SIMULATION_NORTHSTAR_CONTROL_URL" \
    -control-ca /run/zpr-machine/control-ca.crt \
    -client-cert /run/zpr-machine/client.crt \
    -client-key /run/zpr-machine/client.key \
    -zpr-ph /opt/zpr-workloads/ph \
    -zpr-bootstrap-key /opt/zpr-workloads/machine-controller-machine-01.key \
    -zpr-node-addr "$SIMULATION_NORTHSTAR_NODE_ADDRESS"
