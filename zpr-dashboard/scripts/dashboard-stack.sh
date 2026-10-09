#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
DASHBOARD_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/.." && pwd)
RUNTIME_DIR=$(CDPATH='' cd -- "$DASHBOARD_DIR/../../.local-runtime" && pwd)
PROJECT_ROOT=$(CDPATH='' cd -- "$DASHBOARD_DIR/../.." && pwd)
SERVICE_CERTS="$RUNTIME_DIR/service-certs"
SIMULATION_MANIFEST="${SIMULATION_MANIFEST:-$RUNTIME_DIR/simulation-environment.json}"
STATE_DIR="$RUNTIME_DIR/dashboard-stack"
OPERATOR_DIR="$RUNTIME_DIR/operator-login"
BIN="${ZPR_WEB_DASHBOARD_BIN:-$STATE_DIR/zpr-web-dashboard}"
POLICY_TESTER_BIN="${ZPR_ZPT_BIN:-$DASHBOARD_DIR/../target/debug/zpt}"
ZPLC_DEFAULT_BIN="$DASHBOARD_DIR/../../zpr-compiler/target/release/zplc"
ZPLC_DEBUG_BIN="$DASHBOARD_DIR/../../zpr-compiler/target/debug/zplc"
if [ -x "$ZPLC_DEBUG_BIN" ] && { [ ! -x "$ZPLC_DEFAULT_BIN" ] || [ "$ZPLC_DEBUG_BIN" -nt "$ZPLC_DEFAULT_BIN" ]; }; then
    ZPLC_DEFAULT_BIN="$ZPLC_DEBUG_BIN"
fi
ZPLC_BIN="${ZPR_ZPLC_BIN:-$ZPLC_DEFAULT_BIN}"
CONTROL_DIR="$STATE_DIR/machine-control"
MACHINE_CERT_DIR="$CONTROL_DIR/machine-certs"
CONTROL_CA="$CONTROL_DIR/control-ca.crt"
CONTROL_CA_KEY="$CONTROL_DIR/control-ca.key"
CONTROL_SERVER_CERT="$CONTROL_DIR/simulator.crt"
CONTROL_SERVER_KEY="$CONTROL_DIR/simulator.key"
MACHINE_CONTROLLER_BIN="$STATE_DIR/zpr-machine-controller-linux-arm64"
MACHINE_CONTROLLER_AMD64_BIN="$STATE_DIR/zpr-machine-controller-linux-amd64"
MACHINE_CONTROL_PROXY_BIN="$STATE_DIR/zpr-machine-control-proxy-linux-arm64"
MACHINE_CONTROL_PROXY_AMD64_BIN="$STATE_DIR/zpr-machine-control-proxy-linux-amd64"
MACHINE_RUNTIME_ARCH_FILE="$STATE_DIR/machine-runtime-arch"
MACHINE_CONTROL_ADDRESS_FILE="$STATE_DIR/multinode-machine-control-address.txt"
MACHINE_WORKLOAD_DIR="$STATE_DIR/machine-workloads"
MACHINE_IMAGE="${SIMULATOR_MACHINE_IMAGE:-zpr-sim-machine:local}"
SIMULATION_CONTAINER="${SIMULATION_CONTAINER:-zpr-local-linux-node}"
ORGANIZATIONS_DIR="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/organizations"
ACTIVE_ORGANIZATION_FILE="${SIMULATION_ACTIVE_ORGANIZATION_FILE:-$RUNTIME_DIR/active-organization.txt}"
DNS_PROFILE_DIR="$DASHBOARD_DIR/../dns/bind9"
DNS_RUNTIME_DIR="$RUNTIME_DIR/dns-bind"
DNS_CONTAINER="${ZPR_DNS_CONTAINER:-zpr-dns-bind9}"
DNS_IMAGE="${ZPR_DNS_IMAGE:-zpr-dns-bind9:local}"
DNS_STATS_RELAY_PID="$STATE_DIR/dns-stats-relay.pid"
DNS_STATS_RELAY_PORT="${ZPR_DNS_STATS_RELAY_PORT:-8054}"
DNS_RECORDS_RELAY_PID="$STATE_DIR/dns-records-relay.pid"
DNS_RECORDS_RELAY_PORT="${ZPR_DNS_RECORDS_RELAY_PORT:-8055}"
DNS_SERVICE_ADDRESS="${ZPR_DNS_SERVICE_ADDRESS:-fd00:1:1::1}"
DNS_VIEWER_KEY_FILE="${ZPR_DNS_TRANSFER_TSIG_KEY_FILE:-$DNS_RUNTIME_DIR/zpr-dns-viewer.key}"
POLICY_PID="$STATE_DIR/policy-service.pid"
POLICY_CONTAINER="${ZPR_POLICY_SERVICE_CONTAINER:-zpr-policy-service}"
POLICY_TOOLS_IMAGE="${ZPR_POLICY_TOOLS_IMAGE:-zpr-policy-tools:local}"
POLICY_TOOLS_DIR="$RUNTIME_DIR/linux-tools"
CONTROL_PID="$STATE_DIR/control-service.pid"
CONTROL_CONTAINER="${ZPR_CONTROL_SERVICE_CONTAINER:-zpr-control-service}"
ROOM_PID="$STATE_DIR/control-room.pid"
CONTROL_ROOM_DOCKER_CONTAINER="${ZPR_CONTROL_ROOM_CONTAINER:-zpr-control-room}"
SIMULATOR_PID="$STATE_DIR/simulator.pid"
SIMULATOR_DOCKER_CONTAINER="${ZPR_SIMULATOR_CONTAINER:-zpr-simulator}"
SIMULATOR_IMAGE="${ZPR_SIMULATOR_IMAGE:-zpr-simulator:local}"
ZPLC_IMAGE="${ZPR_ZPLC_IMAGE:-zpr-zplc:local}"
BROWSER_GATEWAY_PID="$STATE_DIR/browser-gateway.pid"
ADMIN_RELAY_PID="$STATE_DIR/admin-relay.pid"
ADMIN_RELAY_PORT=8184
LDAP_UI_RELAY_PID="$STATE_DIR/ldap-ui-relay.pid"
LDAP_UI_RELAY_PORT="${ZPR_LDAP_UI_RELAY_PORT:-8797}"
OBSERVABILITY_UI_RELAY_PID="$STATE_DIR/observability-ui-relay.pid"
OBSERVABILITY_UI_RELAY_PORT="${ZPR_OBSERVABILITY_UI_RELAY_PORT:-8798}"
OBSERVABILITY_ADDRESS="${ZPR_OBSERVABILITY_ADDR:-fd5a:5052:adda:1::54}"
OBSERVABILITY_PROFILE_DIR="$DASHBOARD_DIR/../observability/openobserve"
OBSERVABILITY_ENV_FILE="$RUNTIME_DIR/observability/openobserve.env"
OBSERVABILITY_COLLECTOR_CONTAINER="zpr-observability-collector"
OBSERVABILITY_LOCAL_CONTAINER="zpr-observability-local"
OBSERVABILITY_LOCAL_NETWORK="zpr-observability-local"
OBSERVABILITY_LOCAL_PORT=8800

pid_running() {
    [ -f "$1" ] && kill -0 "$(cat "$1")" 2>/dev/null
}

wait_for_url() {
    url=$1
    label=$2
    shift 2
    attempts=0
    while [ "$attempts" -lt 50 ]; do
        if curl -fsS --connect-timeout 1 --max-time 2 "$@" "$url" >/dev/null 2>&1; then
            return 0
        fi
        attempts=$((attempts + 1))
        sleep 0.2
    done
    echo "$label did not become ready at $url" >&2
    return 1
}

wait_for_response() {
    url=$1
    label=$2
    shift 2
    attempts=0
    while [ "$attempts" -lt 600 ]; do
        if curl --silent --show-error --connect-timeout 1 --max-time 2 "$@" "$url" >/dev/null 2>&1; then
            return 0
        fi
        attempts=$((attempts + 1))
        sleep 0.2
    done
    echo "$label did not respond at $url" >&2
    return 1
}

wait_for_policy_context() {
    expected_organization=$1
    attempts=0
    while [ "$attempts" -lt 150 ]; do
        if [ "${ZPR_DASHBOARD_CONTAINER_RUNTIME:-}" = 1 ]; then
            response=$(curl --silent --show-error --connect-timeout 1 --max-time 2 \
                --connect-to 127.0.0.1:8789:host.docker.internal:8789 \
                --cacert "$SERVICE_CERTS/service-ca.crt" \
                --cert "$SERVICE_CERTS/control-policy-client.crt" \
                --key "$SERVICE_CERTS/control-policy-client.key" \
                https://127.0.0.1:8789/api/policy/context 2>/dev/null || true)
        else
            response=$(curl --silent --show-error --connect-timeout 1 --max-time 2 \
                --cacert "$SERVICE_CERTS/service-ca.crt" \
                --cert "$SERVICE_CERTS/control-policy-client.crt" \
                --key "$SERVICE_CERTS/control-policy-client.key" \
                https://127.0.0.1:8789/api/policy/context 2>/dev/null || true)
        fi
        actual_organization=$(printf '%s' "$response" | jq -r '.organization_id // empty' 2>/dev/null || true)
        if [ "$actual_organization" = "$expected_organization" ]; then
            return 0
        fi
        attempts=$((attempts + 1))
        sleep 0.2
    done
    actual_organization=$(printf '%s' "$response" | jq -r '.organization_id // "unavailable"' 2>/dev/null || printf unavailable)
    echo "policy context did not become ready for $expected_organization (current: $actual_organization)" >&2
    return 1
}

ensure_policy_tools() {
    mkdir -p "$POLICY_TOOLS_DIR"
    docker build --platform linux/arm64 -f "$SCRIPT_DIR/Dockerfile.zpt" -t "$POLICY_TOOLS_IMAGE" "$PROJECT_ROOT"
    if ! docker image inspect "$ZPLC_IMAGE" >/dev/null 2>&1; then
        docker build --platform linux/arm64 -f "$SCRIPT_DIR/Dockerfile.zplc" -t "$ZPLC_IMAGE" "$DASHBOARD_DIR/../../zpr-compiler"
    fi
    docker run --rm -v "$RUNTIME_DIR:/runtime" --entrypoint /bin/sh "$POLICY_TOOLS_IMAGE" \
        -c 'cp /usr/local/bin/zpt /runtime/linux-tools/zpt'
    docker run --rm -v "$RUNTIME_DIR:/runtime" --entrypoint /bin/sh "$ZPLC_IMAGE" \
        -c 'cp /usr/local/bin/zplc /runtime/linux-tools/zplc'
    chmod 755 "$POLICY_TOOLS_DIR/zpt" "$POLICY_TOOLS_DIR/zplc"
}

docker_socket_path() {
    printf '%s\n' "${ZPR_DOCKER_SOCKET:-/var/run/docker.sock}"
}

stop_simulator() {
    if [ "${ZPR_DASHBOARD_CONTAINER_RUNTIME:-}" != 1 ]; then
        if [ "$(docker inspect -f '{{.State.Running}}' "$SIMULATOR_DOCKER_CONTAINER" 2>/dev/null || true)" = true ]; then
            for action in stop-admin-relay stop-dns stop-ui-relays; do
                docker exec "$SIMULATOR_DOCKER_CONTAINER" sh "$SCRIPT_DIR/dashboard-stack.sh" "$action" >/dev/null 2>&1 || true
            done
        else
            stop_host_relays
        fi
    fi
    stop_service "$SIMULATOR_PID"
    if docker inspect "$SIMULATOR_DOCKER_CONTAINER" >/dev/null 2>&1; then
        docker rm -f "$SIMULATOR_DOCKER_CONTAINER" >/dev/null
    fi
}

stop_control_room() {
    stop_service "$ROOM_PID"
    if docker inspect "$CONTROL_ROOM_DOCKER_CONTAINER" >/dev/null 2>&1; then
        docker rm -f "$CONTROL_ROOM_DOCKER_CONTAINER" >/dev/null
    fi
}

stop_stack() {
    stop_observability_collector
    ZPR_OBSERVABILITY_LOCAL_CONTAINER="$OBSERVABILITY_LOCAL_CONTAINER" \
        sh "$OBSERVABILITY_PROFILE_DIR/run.sh" stop-local || true
    for number in $(seq -w 1 20); do
        docker rm -f "zpr-machine-$number" >/dev/null 2>&1 || true
    done
    stop_service "$BROWSER_GATEWAY_PID"
    if [ "$(docker inspect -f '{{.State.Running}}' "$SIMULATOR_DOCKER_CONTAINER" 2>/dev/null || true)" = true ]; then
        stop_ui_relays
        stop_dns_service
    fi
    stop_simulator
    stop_control_room
    docker rm -f "$CONTROL_CONTAINER" "$POLICY_CONTAINER" >/dev/null 2>&1 || true
    stop_service "$CONTROL_PID"
    stop_service "$ADMIN_RELAY_PID"
    stop_service "$POLICY_PID"
}

stop_service() {
    pid_file=$1
    if pid_running "$pid_file"; then
        kill "$(cat "$pid_file")" 2>/dev/null || true
    fi
    rm -f "$pid_file"
}

stop_host_relays() {
    for entry in "$ADMIN_RELAY_PORT:$ADMIN_RELAY_PID" "$DNS_STATS_RELAY_PORT:$DNS_STATS_RELAY_PID" "$DNS_RECORDS_RELAY_PORT:$DNS_RECORDS_RELAY_PID" "$LDAP_UI_RELAY_PORT:$LDAP_UI_RELAY_PID" "$OBSERVABILITY_UI_RELAY_PORT:$OBSERVABILITY_UI_RELAY_PID"; do
        relay_port=${entry%%:*}
        relay_pid_file=${entry#*:}
        listener=$(lsof -tiTCP:"$relay_port" -sTCP:LISTEN 2>/dev/null | awk 'NR == 1 { print; exit }')
        if [ -n "$listener" ]; then
            process_command=$(ps -p "$listener" -o command= 2>/dev/null || true)
            case "$process_command" in
                *socat*)
                    kill "$listener" 2>/dev/null || true
                    attempts=0
                    while [ "$attempts" -lt 50 ] && lsof -tiTCP:"$relay_port" -sTCP:LISTEN >/dev/null 2>&1; do
                        attempts=$((attempts + 1))
                        sleep 0.1
                    done
                    if [ "$attempts" -lt 50 ] && lsof -tiTCP:"$relay_port" -sTCP:LISTEN >/dev/null 2>&1; then
                        echo "socat did not release relay port $relay_port" >&2
                        return 1
                    fi
                    ;;
                *) echo "refusing to stop unrelated listener $listener on relay port $relay_port" >&2; return 1 ;;
            esac
        fi
        rm -f "$relay_pid_file"
    done
}

start_service() {
    name=$1
    pid_file=$2
    log_file="$STATE_DIR/$name.log"
    shift 2
    "$@" >"$log_file" 2>&1 < /dev/null &
    echo $! >"$pid_file"
}

start_simulator() {
    if [ "$(docker inspect -f '{{.State.Running}}' "$SIMULATOR_DOCKER_CONTAINER" 2>/dev/null || true)" = true ]; then
        return 0
    fi
    docker rm -f "$SIMULATOR_DOCKER_CONTAINER" >/dev/null 2>&1 || true
    docker build -f "$SCRIPT_DIR/Dockerfile.simulator" -t "$SIMULATOR_IMAGE" "$DASHBOARD_DIR"
    simulator_socket=$(docker_socket_path)
    organization_id=${SIMULATION_ORGANIZATION_ID:-$(jq -r '.organization_id // "northstar"' "$SIMULATION_MANIFEST")}
    simulator_proxy_ip=$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}')
    simulator_operator_origin=${ZPR_SIMULATOR_OPERATOR_ORIGIN:-}
    simulator_operator_trusted_peer=${ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP:-}
    if [ -n "$simulator_operator_origin" ]; then
        for simulator_operator_file in \
            "${ZPR_SIMULATOR_OPERATOR_CERT_FILE:-}" \
            "${ZPR_SIMULATOR_OPERATOR_KEY_FILE:-}" \
            "${ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE:-}" \
            "${ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE:-}" \
            "${ZPR_SIMULATOR_OPERATOR_TLS_CA_FILE:-}"; do
            if [ -z "$simulator_operator_file" ]; then
                echo "Simulator HTTPS requires certificate, key, OIDC config/secret, and TLS CA file paths" >&2
                return 1
            fi
        done
        if [ -z "$simulator_operator_trusted_peer" ]; then
            simulator_operator_trusted_peer=$simulator_proxy_ip
        fi
        for simulator_operator_file in \
            "$ZPR_SIMULATOR_OPERATOR_CERT_FILE" \
            "$ZPR_SIMULATOR_OPERATOR_KEY_FILE" \
            "$ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE" \
            "$ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE" \
            "${ZPR_SIMULATOR_OPERATOR_OIDC_CA_FILE:-}"; do
            if [ -n "$simulator_operator_file" ]; then
                case "$simulator_operator_file" in
                    "$RUNTIME_DIR"/*) ;;
                    *) echo "Simulator operator files must be under $RUNTIME_DIR so the container can read them" >&2; return 1 ;;
                esac
            fi
        done
    fi
    docker run -d --name "$SIMULATOR_DOCKER_CONTAINER" \
        --label zpr.simulator=true \
        -v "$simulator_socket:/var/run/docker.sock" \
        -v "$DASHBOARD_DIR:$DASHBOARD_DIR:ro" \
        -v "$OBSERVABILITY_PROFILE_DIR:$OBSERVABILITY_PROFILE_DIR:ro" \
        -v "$DNS_PROFILE_DIR:$DNS_PROFILE_DIR:ro" \
        -v "$DASHBOARD_DIR/../../zpr-demo/multinode-demo:$DASHBOARD_DIR/../../zpr-demo/multinode-demo:ro" \
        -v "$DASHBOARD_DIR/../../zpr-core:$DASHBOARD_DIR/../../zpr-core:ro" \
        -v "$RUNTIME_DIR:$RUNTIME_DIR" \
        -p 127.0.0.1:8055:8055 \
        -p 127.0.0.1:8184:8184 \
        -p 127.0.0.1:8788:8788 \
        -p 0.0.0.0:8791:8791 \
        -p 127.0.0.1:8797:8797 \
        -p 127.0.0.1:8798:8798 \
        -e SIMULATION_MANIFEST="$SIMULATION_MANIFEST" \
        -e SIMULATION_ORGANIZATION_ID="$organization_id" \
        -e SIMULATION_ORGANIZATIONS_DIR="$ORGANIZATIONS_DIR" \
        -e SIMULATION_WORKSPACE_DB_DIR="$STATE_DIR/policy-private" \
        -e SIMULATION_PREGEN_DIR="$RUNTIME_DIR/linux-integration/pregen" \
        -e SIMULATION_PUBLISHED_DIRECTORY_DIR="$RUNTIME_DIR/published-directories" \
        -e SIMULATION_SCENARIOS_DIR="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/scenarios" \
        -e SIMULATION_STACK_SCRIPT="$SCRIPT_DIR/dashboard-stack.sh" \
        -e SIMULATION_ORGANIZATION_RESET_SCRIPT="$SCRIPT_DIR/activate-organization.sh" \
        -e SIMULATION_ACTIVATION_LOG_FILE="$STATE_DIR/organization-reset.log" \
        -e ZPR_WEB_DASHBOARD_BIN=/usr/local/bin/zpr-web-dashboard \
        -e ZPR_ZPT_BIN="$POLICY_TOOLS_DIR/zpt" \
        -e ZPR_ZPLC_BIN="$POLICY_TOOLS_DIR/zplc" \
        -e ZPR_ZPLC_IMAGE="$ZPLC_IMAGE" \
        -e SIMULATION_AGENT_SCRIPT="$SCRIPT_DIR/simulation-agent.sh" \
        -e SIMULATION_CONTAINER="$SIMULATION_CONTAINER" \
        -e SIMULATOR_CONTROL_TLS_CERT="$CONTROL_SERVER_CERT" \
        -e SIMULATOR_CONTROL_TLS_KEY="$CONTROL_SERVER_KEY" \
        -e SIMULATOR_CONTROL_CLIENT_CA="$CONTROL_CA" \
        -e SIMULATOR_CONTROL_LISTEN=0.0.0.0:8791 \
        -e SIMULATOR_OPERATOR_SERVICE_URL=https://host.docker.internal:8790 \
        -e SIMULATOR_OPERATOR_SERVICE_TLS_SERVER_NAME=127.0.0.1 \
        -e SIMULATOR_OPERATOR_SERVICE_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        -e SIMULATOR_OPERATOR_CLIENT_CERT_FILE="$SERVICE_CERTS/control-room-client.crt" \
        -e SIMULATOR_OPERATOR_CLIENT_KEY_FILE="$SERVICE_CERTS/control-room-client.key" \
        -e ZPR_SIMULATOR_OPERATOR_ORIGIN="$simulator_operator_origin" \
        -e ZPR_SIMULATOR_OPERATOR_CERT_FILE="${ZPR_SIMULATOR_OPERATOR_CERT_FILE:-}" \
        -e ZPR_SIMULATOR_OPERATOR_KEY_FILE="${ZPR_SIMULATOR_OPERATOR_KEY_FILE:-}" \
        -e ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE="${ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE:-}" \
        -e ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE="${ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE:-}" \
        -e ZPR_SIMULATOR_OPERATOR_OIDC_CA_FILE="${ZPR_SIMULATOR_OPERATOR_OIDC_CA_FILE:-}" \
        -e ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP="$simulator_operator_trusted_peer" \
        -e ZPR_DIAGNOSTICS_USERNAME="${ZPR_DIAGNOSTICS_USERNAME:-}" \
        -e SIMULATOR_DOCKER_CONTAINER="$SIMULATOR_DOCKER_CONTAINER" \
        -e ZPR_DASHBOARD_CONTAINER_RUNTIME=1 \
        -e ZPR_POLICY_SERVICE_CONTAINER="$POLICY_CONTAINER" \
        -e ZPR_CONTROL_SERVICE_CONTAINER="$CONTROL_CONTAINER" \
        -e ZPR_CONTROL_ROOM_CONTAINER="$CONTROL_ROOM_DOCKER_CONTAINER" \
        -e ZPR_CONTROL_ROOM_PROXY_IP="$simulator_proxy_ip" \
        -e ZPR_ANTHROPIC_API_KEY_FILE="${ZPR_ANTHROPIC_API_KEY_FILE:-$STATE_DIR/assistant/api-key}" \
        -e ANTHROPIC_API_KEY \
        "$SIMULATOR_IMAGE" >/dev/null
    if [ -n "$simulator_operator_origin" ]; then
        wait_for_url "$simulator_operator_origin/auth/operator/config" simulator --cacert "$ZPR_SIMULATOR_OPERATOR_TLS_CA_FILE"
    else
        wait_for_url http://127.0.0.1:8788/api/simulator/status simulator
    fi
    wait_for_url https://127.0.0.1:8791/internal/ping 'machine-control listener' \
        --cacert "$CONTROL_CA" \
        --cert "$MACHINE_CERT_DIR/machine-01/client.crt" \
        --key "$MACHINE_CERT_DIR/machine-01/client.key"
}

start_control_room() {
    if [ "$(docker inspect -f '{{.State.Running}}' "$CONTROL_ROOM_DOCKER_CONTAINER" 2>/dev/null || true)" = true ]; then
        return 0
    fi
    docker rm -f "$CONTROL_ROOM_DOCKER_CONTAINER" >/dev/null 2>&1 || true
    docker build -f "$SCRIPT_DIR/Dockerfile.simulator" -t "$SIMULATOR_IMAGE" "$DASHBOARD_DIR"
    control_room_proxy_ip=$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}')
    set --
    if [ -r "$OPERATOR_DIR/stack.json" ]; then
        sh "$SCRIPT_DIR/local-operator-login.sh" start
        operator_origin=$(jq -er '.origin' "$OPERATOR_DIR/stack.json")
        operator_organization=$(jq -er '.organization' "$OPERATOR_DIR/stack.json")
        set -- -v "$OPERATOR_DIR/room.crt:$OPERATOR_DIR/room.crt:ro" \
            -v "$OPERATOR_DIR/room.key:$OPERATOR_DIR/room.key:ro" \
            -v "$OPERATOR_DIR/oidc.json:$OPERATOR_DIR/oidc.json:ro" \
            -v "$OPERATOR_DIR/client.secret:$OPERATOR_DIR/client.secret:ro" \
            -v "$OPERATOR_DIR/ca.crt:$OPERATOR_DIR/ca.crt:ro" \
            -v "$OPERATOR_DIR/signer.json:$OPERATOR_DIR/signer.json:ro" \
            -v "$OPERATOR_DIR/delegation.key:$OPERATOR_DIR/delegation.key:ro" \
            --add-host zpr-id.localhost:host-gateway \
            -e ZPR_CONTROL_ROOM_ORIGIN="$operator_origin" \
            -e ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP="$control_room_proxy_ip" \
            -e ZPR_CONTROL_ROOM_ORGANIZATION_ID="$operator_organization" \
            -e ZPR_CONTROL_ROOM_CERT_FILE="$OPERATOR_DIR/room.crt" \
            -e ZPR_CONTROL_ROOM_KEY_FILE="$OPERATOR_DIR/room.key" \
            -e ZPR_OPERATOR_OIDC_CONFIG_FILE="$OPERATOR_DIR/oidc.json" \
            -e ZPR_OPERATOR_OIDC_SECRET_FILE="$OPERATOR_DIR/client.secret" \
            -e ZPR_OPERATOR_OIDC_CA_FILE="$OPERATOR_DIR/ca.crt" \
            -e ZPR_OPERATOR_DELEGATION_SIGNER_FILE="$OPERATOR_DIR/signer.json" \
            -e ZPR_OPERATOR_DELEGATION_KEY_FILE="$OPERATOR_DIR/delegation.key"
    fi
    docker run -d --name "$CONTROL_ROOM_DOCKER_CONTAINER" \
        --label zpr.control-room=true \
        --restart unless-stopped \
        -v "$SERVICE_CERTS:$SERVICE_CERTS:ro" \
        -p 127.0.0.1:8787:8787 \
        -e ZPR_CONTROL_SERVICE_URL=https://host.docker.internal:8790 \
        -e ZPR_CONTROL_SERVICE_TLS_SERVER_NAME=127.0.0.1 \
        -e ZPR_CONTROL_ROOM_PROXY_IP="$control_room_proxy_ip" \
        -e ZPR_CONTROL_SERVICE_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        -e ZPR_CONTROL_CLIENT_CERT_FILE="$SERVICE_CERTS/control-room-client.crt" \
        -e ZPR_CONTROL_CLIENT_KEY_FILE="$SERVICE_CERTS/control-room-client.key" \
        "$@" \
        --entrypoint /usr/local/bin/zpr-web-dashboard \
        "$SIMULATOR_IMAGE" -mode control-room -listen 0.0.0.0:8787 >/dev/null
    wait_for_control_room
}

wait_for_control_room() {
    if [ -r "$OPERATOR_DIR/stack.json" ]; then
        wait_for_url "$(jq -er '.origin' "$OPERATOR_DIR/stack.json")/auth/operator/config" control-room \
            --cacert "$OPERATOR_DIR/ca.crt"
    else
        wait_for_url http://127.0.0.1:8787/api/snapshot control-room
    fi
}

restart_control_room() {
    docker build -f "$SCRIPT_DIR/Dockerfile.simulator" -t "$SIMULATOR_IMAGE" "$DASHBOARD_DIR"
    if [ -r "$OPERATOR_DIR/stack.json" ]; then
        sh "$SCRIPT_DIR/local-operator-login.sh" start
    fi
    stop_control_room
    start_control_room
}

restart_simulator() {
    stop_simulator
    start_simulator
}

restart_simulator_ui() {
    if [ "$(docker inspect -f '{{.State.Running}}' "$SIMULATOR_DOCKER_CONTAINER" 2>/dev/null || true)" = true ] &&
        docker exec "$SIMULATOR_DOCKER_CONTAINER" ps -o comm | grep -qx socat; then
        echo "UI-only restart refused: Simulator hosts live relays; use a planned full UI/relay restart" >&2
        return 1
    fi
    ui_organization=${SIMULATION_ORGANIZATION_ID:-}
    if [ -z "$ui_organization" ] && [ -r "$ACTIVE_ORGANIZATION_FILE" ]; then
        ui_organization=$(tr -d '\r\n' < "$ACTIVE_ORGANIZATION_FILE")
    fi
    docker build -f "$SCRIPT_DIR/Dockerfile.simulator" -t "$SIMULATOR_IMAGE" "$DASHBOARD_DIR"
    if docker inspect "$SIMULATOR_DOCKER_CONTAINER" >/dev/null 2>&1; then
        docker stop "$SIMULATOR_DOCKER_CONTAINER" >/dev/null
        docker rm "$SIMULATOR_DOCKER_CONTAINER" >/dev/null
    fi
    SIMULATION_ORGANIZATION_ID="$ui_organization" start_simulator
}

start_browser_gateway() {
    if pid_running "$BROWSER_GATEWAY_PID"; then
        echo "browser gateway is already running (pid $(cat "$BROWSER_GATEWAY_PID"))"
        return 0
    fi
    cert_file=${ZPR_ACCESS_GATEWAY_TLS_CERT_FILE:-}
    key_file=${ZPR_ACCESS_GATEWAY_TLS_KEY_FILE:-}
    client_ca_file=${ZPR_ACCESS_GATEWAY_CLIENT_CA_FILE:-}
    if [ -z "$cert_file" ] || [ -z "$key_file" ] || [ -z "$client_ca_file" ]; then
        echo 'browser gateway requires ZPR_ACCESS_GATEWAY_TLS_CERT_FILE, ZPR_ACCESS_GATEWAY_TLS_KEY_FILE, and ZPR_ACCESS_GATEWAY_CLIENT_CA_FILE' >&2
        return 1
    fi
    if [ ! -r "$cert_file" ] || [ ! -r "$key_file" ] || [ ! -r "$client_ca_file" ]; then
        echo 'browser gateway certificate or client CA file is unreadable' >&2
        return 1
    fi
    gateway_listen=${ZPR_ACCESS_GATEWAY_LISTEN:-127.0.0.1:8443}
    go -C "$DASHBOARD_DIR" build -trimpath -o "$BIN" ./cmd/zpr-web-dashboard
    start_service browser-gateway "$BROWSER_GATEWAY_PID" env \
        ZPR_ACCESS_GATEWAY_TLS_CERT_FILE="$cert_file" \
        ZPR_ACCESS_GATEWAY_TLS_KEY_FILE="$key_file" \
        ZPR_ACCESS_GATEWAY_CLIENT_CA_FILE="$client_ca_file" \
        ZPR_ACCESS_GATEWAY_CONTROL_HOST="${ZPR_ACCESS_GATEWAY_CONTROL_HOST:-control.localhost}" \
        ZPR_ACCESS_GATEWAY_CONTROL_UPSTREAM="${ZPR_ACCESS_GATEWAY_CONTROL_UPSTREAM:-http://127.0.0.1:8787}" \
        ZPR_ACCESS_GATEWAY_SIMULATOR_HOST="${ZPR_ACCESS_GATEWAY_SIMULATOR_HOST:-simulator.localhost}" \
        ZPR_ACCESS_GATEWAY_SIMULATOR_UPSTREAM="${ZPR_ACCESS_GATEWAY_SIMULATOR_UPSTREAM:-http://127.0.0.1:8788}" \
        "$BIN" -mode browser-gateway -gateway-listen "$gateway_listen"
    echo "mTLS browser gateway started at https://$gateway_listen"
}

start_admin_relay() {
    if [ "${ZPR_DASHBOARD_CONTAINER_RUNTIME:-}" != 1 ]; then
        docker exec "$SIMULATOR_DOCKER_CONTAINER" sh "$SCRIPT_DIR/dashboard-stack.sh" start-admin-relay
        return
    fi
    if pid_running "$ADMIN_RELAY_PID"; then
        wait_for_response "https://127.0.0.1:$ADMIN_RELAY_PORT/admin/stats" admin-relay \
            --cacert "$RUNTIME_DIR/local-admin-cert.pem" && return 0
        stop_service "$ADMIN_RELAY_PID"
    fi
    start_service admin-relay "$ADMIN_RELAY_PID" socat \
        "TCP-LISTEN:$ADMIN_RELAY_PORT,bind=0.0.0.0,reuseaddr,fork" \
        "SYSTEM:\"docker exec -i $SIMULATION_CONTAINER ip netns exec zpr-vs socat STDIO TCP:[fd5a:5052::1]:8182\""
    wait_for_response "https://127.0.0.1:$ADMIN_RELAY_PORT/admin/stats" admin-relay \
        --cacert "$RUNTIME_DIR/local-admin-cert.pem"
}

stop_admin_relay() {
    if [ "${ZPR_DASHBOARD_CONTAINER_RUNTIME:-}" != 1 ]; then
        docker exec "$SIMULATOR_DOCKER_CONTAINER" sh "$SCRIPT_DIR/dashboard-stack.sh" stop-admin-relay
        return
    fi
    stop_service "$ADMIN_RELAY_PID"
}

configure_multinode_service_return_route() {
    route_node=$1
    route_tun=$2
    route_source=$3
    route_table=$4
    docker exec "$route_node" ip -6 rule del pref "$route_table" 2>/dev/null || true
    docker exec "$route_node" ip -6 route replace fd5a:5052:adda:1::/64 dev "$route_tun" table "$route_table"
    docker exec "$route_node" ip -6 rule add pref "$route_table" from "$route_source/128" table "$route_table"
}

start_multinode_dns_service() {
    dns_organization=$1
    dns_profile="$ORGANIZATIONS_DIR/$dns_organization.json"
    node_container="$dns_organization-node0"
    dns_runtime="$RUNTIME_DIR/multinode/$dns_organization/dns"
    dns_zone_dir="$dns_runtime/zone"
    dns_zone_file="$dns_zone_dir/db.svc.zpr"
    dns_config="$DNS_PROFILE_DIR/named.conf.simulator"
    publisher_key="$dns_runtime/zpr-vs-publisher.key"
    viewer_key="$dns_runtime/zpr-dns-viewer.key"
    stop_service "$DNS_RECORDS_RELAY_PID"
    stop_service "$DNS_STATS_RELAY_PID"
    echo_address=$(jq -r '[.policy_test_services[]? | select(.id == "echo-web.svc.zpr") | .zpr_address // empty][0] // empty' "$dns_profile")
    metrics_address=$(jq -r '[.policy_test_services[]? | select(.id == "metrics-web.svc.zpr") | .zpr_address // empty][0] // empty' "$dns_profile")
    gateway_enabled=no
    gateway_address=
    if jq -e '.web_gateway.allowed_hosts | type == "array" and length > 0' "$dns_profile" >/dev/null; then
        gateway_enabled=yes
        gateway_container="${dns_organization}-internet-gateway"
        gateway_address=$(docker exec "$gateway_container" ip -6 -o addr show dev tun10 scope global | awk '$4 ~ /^fd5a:5052:adda:1:/ { split($4, address, "/"); print address[1]; exit }' | tr -d '\r\n')
        [ -n "$gateway_address" ] || { echo "Visa-assigned Gateway ZPR address unavailable" >&2; return 1; }
    fi
    mkdir -p "$dns_zone_dir"
    if [ ! -r "$publisher_key" ]; then
        publisher_secret=$(openssl rand -base64 32 | tr -d '\n')
        printf 'key "zpr-vs-publisher" {\n    algorithm hmac-sha256;\n    secret "%s";\n};\n' "$publisher_secret" > "$publisher_key"
        chmod 600 "$publisher_key"
    fi
    if [ ! -r "$viewer_key" ]; then
        viewer_secret=$(openssl rand -base64 32 | tr -d '\n')
        printf 'key "zpr-dns-viewer" {\n    algorithm hmac-sha256;\n    secret "%s";\n};\n' "$viewer_secret" > "$viewer_key"
        chmod 600 "$viewer_key"
    fi
    docker stop "$DNS_CONTAINER" >/dev/null 2>&1 || true
    {
        printf '%s\n' '$TTL 30' '$ORIGIN svc.zpr.' '@ IN SOA dns.svc.zpr. hostmaster.svc.zpr. (1 60 60 86400 30)' '  IN NS dns.svc.zpr.'
        printf 'dns IN AAAA %s\n' "$DNS_SERVICE_ADDRESS"
        if [ -n "$echo_address" ]; then printf 'echo-web IN AAAA %s\n' "$echo_address"; fi
        if [ -n "$metrics_address" ]; then printf 'metrics-web IN AAAA %s\n' "$metrics_address"; fi
        if [ -n "$gateway_address" ]; then printf 'internet-gateway IN AAAA %s\n' "$gateway_address"; fi
    } > "$dns_zone_file"
    cp "$DNS_PROFILE_DIR/named.conf.simulator" "$dns_runtime/named.conf"
    docker exec "$node_container" pkill -TERM -f '[p]h adapter.*--name adapter1' 2>/dev/null || true
    docker exec "$node_container" ip link del tun6 2>/dev/null || true
    docker exec "$node_container" ip tuntap add name tun6 mode tun multi_queue
    docker exec "$node_container" ip -6 addr add "$DNS_SERVICE_ADDRESS/32" dev tun6
    docker exec "$node_container" ip link set tun6 up
    docker exec -d "$node_container" sh -c \
        'exec env ZPR_ADAPTER_SERVICES=ZprDNS,ZprDNSStatistics /app/bin/ph adapter -c /conf/adapter-dns-conf.toml --name adapter1 --node-addr "$1:5000" >>/logs/adapter1-dns.log 2>&1' \
        adapter1-dns "$(jq -er '.runtime.nodes[0].substrate_address' "$dns_profile")"
    attempts=0
    while [ "$attempts" -lt 45 ]; do
        if docker exec "$node_container" /app/bin/ph-cli -p /var/run/zpr/adapter1.sock link show 2>/dev/null | grep -q '(Active)'; then
            break
        fi
        attempts=$((attempts + 1))
        sleep 1
    done
    if [ "$attempts" -ge 45 ]; then
        docker exec "$node_container" tail -n 80 /logs/adapter1-dns.log >&2 || true
        echo "ZPR DNS adapter did not become active on $node_container" >&2
        return 1
    fi
    docker rm -f "$DNS_CONTAINER" >/dev/null 2>&1 || true
    docker build -t "$DNS_IMAGE" "$DNS_PROFILE_DIR"
    docker run -d --name "$DNS_CONTAINER" --network "container:$node_container" --pid="container:$node_container" --privileged \
        -e ZPR_DNS_ADAPTER_NAME=adapter1 \
        -e ZPR_DNS_TSIG_KEY_FILE=/run/secrets/zpr-vs-publisher.key \
        -e ZPR_DNS_TRANSFER_TSIG_KEY_FILE=/run/secrets/zpr-dns-viewer.key \
        -e ZPR_DNS_ZONE_FILE=/var/lib/bind/db.svc.zpr \
        -v "$dns_config:/etc/bind/named.conf:ro" \
        -v "$publisher_key:/run/secrets/zpr-vs-publisher.key:ro" \
        -v "$viewer_key:/run/secrets/zpr-dns-viewer.key:ro" \
        -v "$dns_zone_dir:/var/lib/bind" \
        "$DNS_IMAGE" >/dev/null
    configure_multinode_service_return_route "$node_container" tun6 "$DNS_SERVICE_ADDRESS" 106
    start_service dns-stats-relay "$DNS_STATS_RELAY_PID" socat \
        "TCP-LISTEN:$DNS_STATS_RELAY_PORT,bind=0.0.0.0,reuseaddr,fork" \
        "SYSTEM:\"docker exec -i $node_container socat STDIO TCP:127.0.0.1:8053\""
    wait_for_url "http://127.0.0.1:$DNS_STATS_RELAY_PORT/json/v1/status" dns-stats-relay
    if [ -n "$gateway_address" ]; then
        gateway_dns_update=$(printf 'server %s\nzone svc.zpr.\nupdate delete internet-gateway.svc.zpr. AAAA\nupdate add internet-gateway.svc.zpr. 30 AAAA %s\nsend\n' "$DNS_SERVICE_ADDRESS" "$gateway_address")
        printf '%s' "$gateway_dns_update" | docker exec -i "$DNS_CONTAINER" nsupdate -v -k /run/secrets/zpr-vs-publisher.key
    fi
    start_service dns-records-relay "$DNS_RECORDS_RELAY_PID" socat \
        "TCP-LISTEN:$DNS_RECORDS_RELAY_PORT,bind=0.0.0.0,reuseaddr,fork" \
        "SYSTEM:\"docker exec -i $node_container socat STDIO TCP:[$DNS_SERVICE_ADDRESS]:53\""
    attempts=0
    while [ "$attempts" -lt 50 ]; do
        if nc -z 127.0.0.1 "$DNS_RECORDS_RELAY_PORT"; then break; fi
        attempts=$((attempts + 1))
        sleep 0.2
    done
    [ "$attempts" -lt 50 ] || { echo "DNS records relay did not listen on $DNS_RECORDS_RELAY_PORT" >&2; return 1; }
    attempts=0
    while [ "$attempts" -lt 30 ]; do
        if [ -n "$echo_address" ] || [ -n "$metrics_address" ] || [ "$gateway_enabled" = yes ]; then
            dns_records_ready=yes
            if [ -n "$echo_address" ] && ! docker exec "$DNS_CONTAINER" dig +tcp +time=1 +tries=1 +short AAAA @"$DNS_SERVICE_ADDRESS" echo-web.svc.zpr 2>/dev/null | grep -Fq "$echo_address"; then
                dns_records_ready=no
            fi
            if [ -n "$metrics_address" ] && ! docker exec "$DNS_CONTAINER" dig +tcp +time=1 +tries=1 +short AAAA @"$DNS_SERVICE_ADDRESS" metrics-web.svc.zpr 2>/dev/null | grep -Fq "$metrics_address"; then
                dns_records_ready=no
            fi
            if [ -n "$gateway_address" ] && ! docker exec "$DNS_CONTAINER" dig +tcp +time=1 +tries=1 +short AAAA @"$DNS_SERVICE_ADDRESS" internet-gateway.svc.zpr 2>/dev/null | grep -Fq "$gateway_address"; then
                dns_records_ready=no
            fi
            if [ "$dns_records_ready" = yes ]; then return 0; fi
        elif docker exec "$DNS_CONTAINER" dig +tcp +time=1 +tries=1 +short SOA @"$DNS_SERVICE_ADDRESS" svc.zpr 2>/dev/null | grep -Fq 'svc.zpr'; then
            return 0
        fi
        attempts=$((attempts + 1))
        sleep 1
    done
    docker logs "$DNS_CONTAINER" >&2 || true
    docker exec "$node_container" tail -n 80 /logs/adapter1-dns.log >&2 || true
    echo "multinode ZPR DNS did not resolve the Workday services" >&2
    return 1
}

start_dns_service() {
    if [ "${ZPR_DASHBOARD_CONTAINER_RUNTIME:-}" != 1 ]; then
        if [ -n "${SIMULATION_ORGANIZATION_ID:-}" ]; then
            docker exec -e "SIMULATION_ORGANIZATION_ID=$SIMULATION_ORGANIZATION_ID" \
                "$SIMULATOR_DOCKER_CONTAINER" sh "$SCRIPT_DIR/dashboard-stack.sh" start-dns
        else
            docker exec "$SIMULATOR_DOCKER_CONTAINER" sh "$SCRIPT_DIR/dashboard-stack.sh" start-dns
        fi
        return
    fi
    dns_organization=${SIMULATION_ORGANIZATION_ID:-$(jq -r '.organization_id // "northstar"' "$SIMULATION_MANIFEST")}
    if [ -z "${SIMULATION_ORGANIZATION_ID:-}" ] && [ -r "$ACTIVE_ORGANIZATION_FILE" ]; then
        selected_organization=$(tr -d '\r\n' < "$ACTIVE_ORGANIZATION_FILE")
        [ -z "$selected_organization" ] || dns_organization=$selected_organization
    fi
    if [ "$(jq -er '.runtime.driver' "$ORGANIZATIONS_DIR/$dns_organization.json")" = docker-multinode ]; then
        start_multinode_dns_service "$dns_organization"
        return
    fi
    organization=${SIMULATION_ORGANIZATION_ID:-$(jq -r '.organization_id // "northstar"' "$SIMULATION_MANIFEST")}
    if [ -z "${SIMULATION_ORGANIZATION_ID:-}" ] && [ -r "$ACTIVE_ORGANIZATION_FILE" ]; then
        selected_organization=$(tr -d '\r\n' < "$ACTIVE_ORGANIZATION_FILE")
        [ -z "$selected_organization" ] || organization=$selected_organization
    fi
    if [ "$(jq -er '.runtime.driver' "$ORGANIZATIONS_DIR/$organization.json")" = docker-multinode ]; then
        start_multinode_dns_service
        return
    fi
    config=${ZPR_DNS_NAMED_CONF:-$DNS_PROFILE_DIR/named.conf.simulator}
    key_file=${ZPR_DNS_TSIG_KEY_FILE:-$DNS_RUNTIME_DIR/zpr-vs-publisher.key}
    zone_file=${ZPR_DNS_ZONE_FILE:-$DNS_RUNTIME_DIR/zone/db.svc.zpr}
    zone_dir=${ZPR_DNS_ZONE_DIR:-$(dirname "$zone_file")}
    legacy_zone_file="$DNS_RUNTIME_DIR/db.svc.zpr"
    mkdir -p "$zone_dir"
    if [ ! -r "$zone_file" ] && [ -r "$legacy_zone_file" ]; then
        cp "$legacy_zone_file" "$zone_file"
    fi
    if [ ! -r "$DNS_VIEWER_KEY_FILE" ]; then
        mkdir -p "$(dirname "$DNS_VIEWER_KEY_FILE")"
        (
            umask 077
            viewer_secret=$(openssl rand -base64 32 | tr -d '\n')
            printf 'key "zpr-dns-viewer" {\n    algorithm hmac-sha256;\n    secret "%s";\n};\n' "$viewer_secret" >"$DNS_VIEWER_KEY_FILE"
            chmod 600 "$DNS_VIEWER_KEY_FILE"
        )
    fi
    for file in "$config" "$key_file" "$zone_file" "$DNS_VIEWER_KEY_FILE"; do
        if [ ! -r "$file" ]; then
            echo "DNS simulator asset is missing or unreadable: $file" >&2
            return 1
        fi
    done

    stop_dns_service
    docker build -t "$DNS_IMAGE" "$DNS_PROFILE_DIR"
    docker run -d --name "$DNS_CONTAINER" --network none --privileged --pid="container:$SIMULATION_CONTAINER" \
        -v "$config:/etc/bind/named.conf:ro" \
        -v "$key_file:/run/secrets/zpr-vs-publisher.key:ro" \
        -v "$DNS_VIEWER_KEY_FILE:/run/secrets/zpr-dns-viewer.key:ro" \
        -v "$zone_dir:/var/lib/bind" \
        "$DNS_IMAGE" >/dev/null

    start_service dns-stats-relay "$DNS_STATS_RELAY_PID" socat \
        "TCP-LISTEN:$DNS_STATS_RELAY_PORT,bind=0.0.0.0,reuseaddr,fork" \
        "SYSTEM:\"docker exec -i $SIMULATION_CONTAINER ip netns exec zpr-a socat STDIO TCP:127.0.0.1:8053\""
    wait_for_url "http://127.0.0.1:$DNS_STATS_RELAY_PORT/json/v1/status" dns-stats-relay

    start_service dns-records-relay "$DNS_RECORDS_RELAY_PID" socat \
        "TCP-LISTEN:$DNS_RECORDS_RELAY_PORT,bind=0.0.0.0,reuseaddr,fork" \
        "SYSTEM:\"docker exec -i $SIMULATION_CONTAINER ip netns exec zpr-a socat STDIO TCP:[$DNS_SERVICE_ADDRESS]:53\""
}

stop_dns_service() {
    if [ "${ZPR_DASHBOARD_CONTAINER_RUNTIME:-}" != 1 ]; then
        docker exec "$SIMULATOR_DOCKER_CONTAINER" sh "$SCRIPT_DIR/dashboard-stack.sh" stop-dns
        return
    fi
    dns_organization=${SIMULATION_ORGANIZATION_ID:-$(jq -r '.organization_id // "northstar"' "$SIMULATION_MANIFEST")}
    if [ -z "${SIMULATION_ORGANIZATION_ID:-}" ] && [ -r "$ACTIVE_ORGANIZATION_FILE" ]; then
        selected_organization=$(tr -d '\r\n' < "$ACTIVE_ORGANIZATION_FILE")
        [ -z "$selected_organization" ] || dns_organization=$selected_organization
    fi
    if [ "$(jq -er '.runtime.driver' "$ORGANIZATIONS_DIR/$dns_organization.json")" = docker-multinode ]; then
        dns_node_container="$dns_organization-node0"
        stop_service "$DNS_RECORDS_RELAY_PID"
        stop_service "$DNS_STATS_RELAY_PID"
        docker rm -f "$DNS_CONTAINER" >/dev/null 2>&1 || true
        docker exec "$dns_node_container" pkill -TERM -f '[p]h adapter.*--name adapter1' 2>/dev/null || true
        docker exec "$dns_node_container" ip link del tun6 2>/dev/null || true
        return
    fi
    stop_service "$DNS_RECORDS_RELAY_PID"
    stop_service "$DNS_STATS_RELAY_PID"
    docker rm -f "$DNS_CONTAINER" >/dev/null 2>&1 || true
}

start_ui_relays() {
    if [ "${ZPR_DASHBOARD_CONTAINER_RUNTIME:-}" != 1 ]; then
        docker exec "$SIMULATOR_DOCKER_CONTAINER" sh "$SCRIPT_DIR/dashboard-stack.sh" start-ui-relays
        return
    fi
    start_service ldap-ui-relay "$LDAP_UI_RELAY_PID" socat \
        "TCP-LISTEN:$LDAP_UI_RELAY_PORT,bind=0.0.0.0,reuseaddr,fork" \
        "SYSTEM:\"docker exec -i $SIMULATION_CONTAINER ip netns exec zpr-vs socat STDIO TCP:127.0.0.1:8080\""
    start_service observability-ui-relay "$OBSERVABILITY_UI_RELAY_PID" socat \
        "TCP-LISTEN:$OBSERVABILITY_UI_RELAY_PORT,bind=0.0.0.0,reuseaddr,fork" \
        "SYSTEM:\"docker exec -i $SIMULATION_CONTAINER ip netns exec zpr-vs socat STDIO TCP:[$OBSERVABILITY_ADDRESS]:5080\""
}

stop_ui_relays() {
    if [ "${ZPR_DASHBOARD_CONTAINER_RUNTIME:-}" != 1 ]; then
        docker exec "$SIMULATOR_DOCKER_CONTAINER" sh "$SCRIPT_DIR/dashboard-stack.sh" stop-ui-relays
        return
    fi
    stop_service "$OBSERVABILITY_UI_RELAY_PID"
    stop_service "$LDAP_UI_RELAY_PID"
}

start_local_observability() {
    if [ ! -r "$OBSERVABILITY_ENV_FILE" ]; then
        echo "OpenObserve is not configured; local logger backend was not started" >&2
        return 0
    fi
    ZPR_OBSERVABILITY_ENV_FILE="$OBSERVABILITY_ENV_FILE" \
    ZPR_OBSERVABILITY_LOCAL_CONTAINER="$OBSERVABILITY_LOCAL_CONTAINER" \
    ZPR_OBSERVABILITY_LOCAL_NETWORK="$OBSERVABILITY_LOCAL_NETWORK" \
    ZPR_OBSERVABILITY_LOCAL_PORT="$OBSERVABILITY_LOCAL_PORT" \
        sh "$OBSERVABILITY_PROFILE_DIR/run.sh" start-local

    openobserve_org=$(awk -F= '$1 == "OPENOBSERVE_ORG" {sub(/^[^=]*=/, ""); print; exit}' "$RUNTIME_DIR/observability/collector.env")
    case "$openobserve_org" in ''|*[!A-Za-z0-9_-]*) echo "invalid OpenObserve organization in collector.env" >&2; return 1 ;; esac
    diagnostics_zpr_org=${SIMULATION_ORGANIZATION_ID:-}
    if [ -z "$diagnostics_zpr_org" ] && [ -r "$ACTIVE_ORGANIZATION_FILE" ]; then
        diagnostics_zpr_org=$(tr -d '\r\n' < "$ACTIVE_ORGANIZATION_FILE")
    fi
    if [ -z "$diagnostics_zpr_org" ]; then
        diagnostics_zpr_org=$(jq -er '.organization_id // "northstar"' "$SIMULATION_MANIFEST")
    fi
    diagnostics_config="${ZPR_DIAGNOSTICS_CONFIG_FILE:-$STATE_DIR/diagnostics/openobserve.json}"
    if [ "$diagnostics_config" = "$STATE_DIR/diagnostics/openobserve.json" ]; then
        mkdir -m 700 -p "$STATE_DIR/diagnostics"
        diagnostics_temp="$diagnostics_config.tmp.$$"
        jq -n --arg endpoint "http://$OBSERVABILITY_LOCAL_CONTAINER:5080" --arg organization "$openobserve_org" --arg zpr_organization_id "$diagnostics_zpr_org" \
            '{endpoint:$endpoint,organization:$organization,zpr_organization_id:$zpr_organization_id,logs_stream:"zpr_visa_service",metrics_stream:"zpr_visa_service",stale_after_seconds:300}' > "$diagnostics_temp"
        chmod 600 "$diagnostics_temp"
        mv "$diagnostics_temp" "$diagnostics_config"
    fi
}

stop_observability_collector() {
    docker rm -f "$OBSERVABILITY_COLLECTOR_CONTAINER" >/dev/null 2>&1 || true
}

start_observability_collector() {
    organization=$1
    case "$organization" in ''|*[!a-z0-9-]*) echo "invalid observability organization: $organization" >&2; return 2 ;; esac
    organization_profile="$ORGANIZATIONS_DIR/$organization.json"
    organization_runtime="$RUNTIME_DIR/multinode/$organization"
    organization_logs="$organization_runtime/logs"
    organization_driver=$(jq -er '.runtime.driver' "$organization_profile")
    if [ "$organization_driver" != docker-multinode ]; then
        echo "organization $organization does not use the supported Compose log sources" >&2
        return 1
    fi
    admin_key="$organization_runtime/bob/web-monitor.key"
    admin_ca="$DASHBOARD_DIR/../../zpr-demo/multinode-demo/zpr-conf/include/admin-tls-cert.pem"
    for source_file in "$admin_key" "$admin_ca" "$RUNTIME_DIR/observability/collector.env" "$RUNTIME_DIR/observability/ingestion.token"; do
        [ -r "$source_file" ] || { echo "required observability source is unavailable" >&2; return 1; }
    done
    [ -d "$organization_logs" ] || { echo "organization runtime logs are unavailable" >&2; return 1; }

    collector_runtime="$STATE_DIR/observability-collector/$organization"
    mkdir -m 700 -p "$collector_runtime/observability"
    install -m 600 "$admin_key" "$collector_runtime/admin-read.key"
    install -m 644 "$admin_ca" "$collector_runtime/local-admin-cert.pem"
    install -m 600 "$RUNTIME_DIR/observability/collector.env" "$collector_runtime/observability/collector.env"
    install -m 600 "$RUNTIME_DIR/observability/ingestion.token" "$collector_runtime/observability/ingestion.token"

    sources='[]'
    adapter_targets='[]'
    for source_path in "$organization_logs"/*.log; do
        [ -f "$source_path" ] || continue
        source_file=${source_path##*/}
        source_id=${source_file%.log}
        [ "$source_id" = vs ] && continue
        case "$source_id" in
            node[0-9]*) source_name=zpr-core-node; source_type=node ;;
            *-adapter) source_name=zpr-adapter; source_type=adapter ;;
            *) source_name=$source_id; source_type=service ;;
        esac
        sources=$(printf '%s' "$sources" | jq -c --arg name "$source_name" --arg instance "$organization-$source_id" \
            --arg type "$source_type" --arg path "/org-logs/$source_file" \
            '. + [{service_name:$name,service_instance_id:$instance,source_type:$type,log_file:$path}]')
        case "$source_id" in
            *-adapter) adapter_kind=adapter; adapter_title="$source_id adapter" ;;
            *) adapter_kind=controller; adapter_title="$source_id" ;;
        esac
        adapter_targets=$(printf '%s' "$adapter_targets" | jq -c --arg id "$organization-$source_id" --arg title "$organization $adapter_title" \
            --arg name "$source_id" --arg kind "$adapter_kind" --arg file "$source_path" \
            '. + [{id:$id,name:$title,sources:[{name:$name,kind:$kind,file:$file}]}]')
    done
    printf '%s\n' "$(jq -n --argjson adapters "$adapter_targets" '{adapters:$adapters}')" > "$STATE_DIR/adapter-logs.json"
    chmod 600 "$STATE_DIR/adapter-logs.json"
    jq -n --argjson sources "$sources" '{sources:$sources}' > "$collector_runtime/observability/diagnostic-sources.json"
    chmod 600 "$collector_runtime/observability/diagnostic-sources.json"

    source_map='{}'
    node_count=$(jq -er '.runtime.nodes | length' "$organization_profile")
    node_index=0
    while [ "$node_index" -lt "$node_count" ]; do
        node_instance="$organization-node$node_index"
        node_identity="node$node_index.demo"
        for source_id in "node:$node_identity" "service:/zpr/n$node_index" "service:zpr/n$node_index/vss"; do
            source_map=$(printf '%s' "$source_map" | jq -c --arg id "$source_id" --arg instance "$node_instance" \
                '. + {($id):{service_name:"zpr-core-node",instance_id:$instance}}')
        done
        node_index=$((node_index + 1))
    done
    for source_id in service:/zpr/visaservice service:/zpr/visaservice/admin; do
        source_map=$(printf '%s' "$source_map" | jq -c --arg id "$source_id" --arg instance "$organization-vs" \
            '. + {($id):{service_name:"zpr-visaservice",instance_id:$instance}}')
    done
    printf '%s\n' "$source_map" > "$STATE_DIR/diagnostics/source-map.json"
    chmod 600 "$STATE_DIR/diagnostics/source-map.json"

    stop_observability_collector
    docker run -d --name "$OBSERVABILITY_COLLECTOR_CONTAINER" --restart unless-stopped \
        --network "$OBSERVABILITY_LOCAL_NETWORK" --add-host vs.zpr:host-gateway \
        --read-only --tmpfs /tmp:rw,size=16m \
        -v "$collector_runtime:/runtime:ro" -v "$organization_logs:/org-logs:ro" \
        -v "$OBSERVABILITY_PROFILE_DIR/collector.py:/app/collector.py:ro" \
        -e ZPR_RUNTIME_DIR=/runtime -e ZPR_ORGANIZATION_ID="$organization" \
        -e ZPR_VS_INSTANCE_ID="$organization-vs" \
        -e ZPR_VS_ADMIN_URL=https://vs.zpr:8185 \
        -e ZPR_OBSERVABILITY_URL="http://$OBSERVABILITY_LOCAL_CONTAINER:5080" \
        -e ZPR_VS_LOG_FILE=/org-logs/vs.log \
        --entrypoint python3 zpr-observability:local -u /app/collector.py >/dev/null
}

start_control_service() {
    control_ldap_container=${ZPR_ASSERTION_LDAP_CONTAINER:-}
    control_gateway_organization=${ZPR_GATEWAY_ORGANIZATION_ID:-${SIMULATION_ORGANIZATION_ID:-}}
    active_organization_file=${ACTIVE_ORGANIZATION_FILE:-}
    if [ -z "$control_gateway_organization" ] && [ -n "$active_organization_file" ] && [ -r "$active_organization_file" ]; then
        control_gateway_organization=$(tr -d '\r\n' < "$active_organization_file")
    fi
    control_dns_transfer_key_file=${ZPR_DNS_TRANSFER_TSIG_KEY_FILE:-}
    if [ -z "$control_dns_transfer_key_file" ]; then
        control_dns_transfer_key_file=$DNS_VIEWER_KEY_FILE
        control_organization_profile="$ORGANIZATIONS_DIR/$control_gateway_organization.json"
        if [ -n "$control_gateway_organization" ] && [ -r "$control_organization_profile" ] &&
            [ "$(jq -er '.runtime.driver' "$control_organization_profile")" = docker-multinode ]; then
            control_dns_transfer_key_file="$RUNTIME_DIR/multinode/$control_gateway_organization/dns/zpr-dns-viewer.key"
        fi
    fi
    stop_control_service
    start_local_observability
    control_admin_url=${ZPR_ADMIN_URL:-https://127.0.0.1:$ADMIN_RELAY_PORT}
    control_admin_url=$(printf '%s' "$control_admin_url" | sed 's#://127\.0\.0\.1:#://host.docker.internal:#')
    control_dns_stats_url=${ZPR_DNS_STATS_URL:-http://127.0.0.1:$DNS_STATS_RELAY_PORT}
    control_dns_stats_url=$(printf '%s' "$control_dns_stats_url" | sed 's#://127\.0\.0\.1:#://host.docker.internal:#')
    set --
    operator_directory=${OPERATOR_DIR:-$RUNTIME_DIR/operator-login}
    if [ -r "$operator_directory/stack.json" ]; then
        set -- -e ZPR_ENROLLMENT_CONFIG_FILE="$operator_directory/enrollment.json" \
            -e ZPR_ENROLLMENT_DATABASE_FILE="$STATE_DIR/enrollment/registry.db" \
            -e ZPR_OPERATOR_DELEGATION_TRUST_FILE="$operator_directory/trust.json"
    fi
    docker run -d --name "$CONTROL_CONTAINER" \
        --label zpr.control-service=true \
        --restart unless-stopped \
        -v "$(docker_socket_path):/var/run/docker.sock" \
        -v "$RUNTIME_DIR:$RUNTIME_DIR" \
        -v "$DASHBOARD_DIR/../../zpr-demo/multinode-demo:$DASHBOARD_DIR/../../zpr-demo/multinode-demo:ro" \
        -p 127.0.0.1:8790:8790 \
        -e ANTHROPIC_API_KEY \
        -e ZPR_CONTROL_SERVICE_LISTEN=0.0.0.0:8790 \
        -e ZPR_GATEWAY_ORGANIZATION_ID="$control_gateway_organization" \
        -e ZPR_GATEWAY_CONFIG_STORE_DIR="${ZPR_GATEWAY_CONFIG_STORE_DIR:-$STATE_DIR/gateway-configs}" \
        -e ZPR_ANTHROPIC_API_KEY_FILE="${ZPR_ANTHROPIC_API_KEY_FILE:-$STATE_DIR/assistant/api-key}" \
        -e ZPR_CONTROL_SERVICE_CERT_FILE="$SERVICE_CERTS/control-service.crt" \
        -e ZPR_CONTROL_SERVICE_KEY_FILE="$SERVICE_CERTS/control-service.key" \
        -e ZPR_CONTROL_SERVICE_CLIENT_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        -e ZPR_ASSERTION_STORE_FILE="${ZPR_ASSERTION_STORE_FILE:-$STATE_DIR/assertions/global.json}" \
        -e ZPR_ASSERTION_LDAP_CONTAINER="$control_ldap_container" \
        -e ZPR_ASSERTION_LDAP_BASE_DN="${ZPR_ASSERTION_LDAP_BASE_DN:-}" \
        -e ZPR_ASSERTION_LDAP_BIND_DN="${ZPR_ASSERTION_LDAP_BIND_DN:-}" \
        -e ZPR_ASSERTION_SOURCES_FILE="${ZPR_ASSERTION_SOURCES_FILE:-}" \
        -e ZPR_TRUSTED_CHANGE_FEEDS_FILE="${ZPR_TRUSTED_CHANGE_FEEDS_FILE:-}" \
        -e ZPR_ADAPTER_LOG_CONFIG_FILE="${ZPR_ADAPTER_LOG_CONFIG_FILE:-$STATE_DIR/adapter-logs.json}" \
        -e ZPR_PROVIDER_MANAGER_URLS="${ZPR_PROVIDER_MANAGER_URLS:-}" \
        -e ZPR_DIAGNOSTICS_CONFIG_FILE="${ZPR_DIAGNOSTICS_CONFIG_FILE:-$STATE_DIR/diagnostics/openobserve.json}" \
        -e ZPR_DIAGNOSTICS_USERNAME="${ZPR_DIAGNOSTICS_USERNAME:-}" \
        -e ZPR_DIAGNOSTICS_TOKEN_FILE="${ZPR_DIAGNOSTICS_TOKEN_FILE:-$STATE_DIR/diagnostics/query.token}" \
        -e ZPR_DIAGNOSTICS_SOURCE_MAP_FILE="${ZPR_DIAGNOSTICS_SOURCE_MAP_FILE:-$STATE_DIR/diagnostics/source-map.json}" \
        -e ZPR_PLATFORM_SERVICES="${ZPR_PLATFORM_SERVICES:-[]}" \
        -e ZPR_ADMIN_URL="$control_admin_url" \
        -e ZPR_ADMIN_CA_FILE="${ZPR_ADMIN_CA_FILE:-$RUNTIME_DIR/local-admin-cert.pem}" \
        -e ZPR_ADMIN_KEY_FILE="${ZPR_ADMIN_KEY_FILE:-$RUNTIME_DIR/admin-read.key}" \
        -e ZPR_DNS_STATS_URL="$control_dns_stats_url" \
        -e ZPR_DNS_TRANSFER_ADDR="host.docker.internal:$DNS_RECORDS_RELAY_PORT" \
        -e ZPR_DNS_TRANSFER_TSIG_KEY_FILE="$control_dns_transfer_key_file" \
        -e ZPR_POLICY_SERVICE_URL=https://host.docker.internal:8789 \
        -e ZPR_POLICY_SERVICE_TLS_SERVER_NAME=127.0.0.1 \
        -e ZPR_POLICY_SERVICE_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        -e ZPR_POLICY_CLIENT_CERT_FILE="$SERVICE_CERTS/control-policy-client.crt" \
        -e ZPR_POLICY_CLIENT_KEY_FILE="$SERVICE_CERTS/control-policy-client.key" \
        "$@" \
        --entrypoint /usr/local/bin/zpr-web-dashboard \
        "$SIMULATOR_IMAGE" -mode control-service >/dev/null
    if docker inspect "$OBSERVABILITY_LOCAL_CONTAINER" >/dev/null 2>&1; then
        docker network connect "$OBSERVABILITY_LOCAL_NETWORK" "$CONTROL_CONTAINER"
    fi
    wait_for_url https://127.0.0.1:8790/api/snapshot control-service \
        ${ZPR_DASHBOARD_CONTAINER_RUNTIME:+--connect-to 127.0.0.1:8790:host.docker.internal:8790} \
        --cacert "$SERVICE_CERTS/service-ca.crt" \
        --cert "$SERVICE_CERTS/control-room-client.crt" \
        --key "$SERVICE_CERTS/control-room-client.key"
}

stop_control_service() {
    docker rm -f "$CONTROL_CONTAINER" >/dev/null 2>&1 || true
    stop_service "$CONTROL_PID"
}

reload_assistant() {
    if [ "$(docker inspect -f '{{.State.Running}}' "$CONTROL_CONTAINER")" != true ]; then
        echo "Control-Service must be running before reloading the assistant." >&2
        return 1
    fi
    configured_key_file=$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$CONTROL_CONTAINER" |
        sed -n 's/^ZPR_ANTHROPIC_API_KEY_FILE=//p')
    if [ "$configured_key_file" != "$STATE_DIR/assistant/api-key" ]; then
        echo "Control-Service uses a different assistant key path; configure its persistent key file before reloading." >&2
        return 1
    fi
    docker restart "$CONTROL_CONTAINER" >/dev/null
    wait_for_url https://127.0.0.1:8790/api/snapshot control-service \
        ${ZPR_DASHBOARD_CONTAINER_RUNTIME:+--connect-to 127.0.0.1:8790:host.docker.internal:8790} \
        --cacert "$SERVICE_CERTS/service-ca.crt" \
        --cert "$SERVICE_CERTS/control-room-client.crt" \
        --key "$SERVICE_CERTS/control-room-client.key"
}

stop_policy_service() {
    docker rm -f "$POLICY_CONTAINER" >/dev/null 2>&1 || true
    stop_service "$POLICY_PID"
}

create_control_certificate() {
    name=$1
    purpose=$2
    cert="$CONTROL_DIR/$name.crt"
    key="$CONTROL_DIR/$name.key"
    request="$CONTROL_DIR/$name.csr"
    extensions="$CONTROL_DIR/$name.ext"
    openssl req -new -newkey rsa:2048 -nodes -subj "/CN=$name" -keyout "$key" -out "$request" >/dev/null 2>&1
    if [ "$purpose" = "server" ]; then
        printf 'basicConstraints=CA:FALSE\nkeyUsage=digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\nsubjectAltName=IP:127.0.0.1,DNS:host.docker.internal\n' > "$extensions"
    else
        printf 'basicConstraints=CA:FALSE\nkeyUsage=digitalSignature,keyEncipherment\nextendedKeyUsage=clientAuth\n' > "$extensions"
    fi
    openssl x509 -req -in "$request" -CA "$CONTROL_CA" -CAkey "$CONTROL_CA_KEY" -CAcreateserial -days 30 -extfile "$extensions" -out "$cert" >/dev/null 2>&1
    chmod 600 "$key"
}

create_machine_control_pki() {
    mkdir -p "$CONTROL_DIR" "$MACHINE_CERT_DIR"
    rm -rf "$MACHINE_CERT_DIR"
    rm -f "$CONTROL_DIR"/*
    mkdir -p "$MACHINE_CERT_DIR"
    openssl req -newkey rsa:2048 -nodes -subj '/CN=ZPR Simulator Machine Control CA' \
        -keyout "$CONTROL_CA_KEY" -out "$CONTROL_DIR/control-ca.csr" >/dev/null 2>&1
    printf 'basicConstraints=critical,CA:TRUE\nkeyUsage=critical,keyCertSign,cRLSign\n' > "$CONTROL_DIR/control-ca.ext"
    openssl x509 -req -in "$CONTROL_DIR/control-ca.csr" -signkey "$CONTROL_CA_KEY" -days 30 \
        -extfile "$CONTROL_DIR/control-ca.ext" -out "$CONTROL_CA" >/dev/null 2>&1
    chmod 600 "$CONTROL_CA_KEY"
    create_control_certificate simulator server
    for number in $(seq -w 1 20); do
        machine="machine-$number"
        create_control_certificate "$machine" client
        mkdir -p "$MACHINE_CERT_DIR/$machine"
        cp "$CONTROL_CA" "$MACHINE_CERT_DIR/$machine/control-ca.crt"
        cp "$CONTROL_DIR/$machine.crt" "$MACHINE_CERT_DIR/$machine/client.crt"
        cp "$CONTROL_DIR/$machine.key" "$MACHINE_CERT_DIR/$machine/client.key"
        chmod 600 "$MACHINE_CERT_DIR/$machine/client.key"
    done
}

ensure_machine_runtime_arch() {
    machine_arch=$1
    case "$machine_arch" in arm64|amd64) ;; *) echo "unsupported machine image architecture: $machine_arch" >&2; return 1 ;; esac
    if [ "$(cat "$MACHINE_RUNTIME_ARCH_FILE" 2>/dev/null || true)" = "$machine_arch" ]; then
        return 0
    fi
    if [ "$machine_arch" = amd64 ]; then
        machine_controller_binary=$MACHINE_CONTROLLER_AMD64_BIN
        machine_proxy_binary=$MACHINE_CONTROL_PROXY_AMD64_BIN
    else
        machine_controller_binary=$MACHINE_CONTROLLER_BIN
        machine_proxy_binary=$MACHINE_CONTROL_PROXY_BIN
    fi
    if [ ! -x "$machine_controller_binary" ] || [ ! -x "$machine_proxy_binary" ]; then
        echo "prebuilt linux/$machine_arch machine controller and proxy are required in $STATE_DIR" >&2
        return 1
    fi
    if [ "$(cat "$MACHINE_RUNTIME_ARCH_FILE" 2>/dev/null || true)" = "$machine_arch" ]; then
        return 0
    fi
    docker build --platform "linux/$machine_arch" -f "$SCRIPT_DIR/Dockerfile.machine" -t "$MACHINE_IMAGE" "$SCRIPT_DIR"
    printf '%s\n' "$machine_arch" > "$MACHINE_RUNTIME_ARCH_FILE"
}

start_machine_controllers() {
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go -C "$DASHBOARD_DIR" build -trimpath -o "$MACHINE_CONTROLLER_BIN" ./cmd/zpr-web-dashboard
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go -C "$DASHBOARD_DIR" build -trimpath -o "$MACHINE_CONTROL_PROXY_BIN" ./cmd/zpr-web-dashboard
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go -C "$DASHBOARD_DIR" build -trimpath -o "$MACHINE_CONTROLLER_AMD64_BIN" ./cmd/zpr-web-dashboard
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go -C "$DASHBOARD_DIR" build -trimpath -o "$MACHINE_CONTROL_PROXY_AMD64_BIN" ./cmd/zpr-web-dashboard
    start_machine_container machine-01
}

resolve_machine_placement() {
    machine_owner=$(jq -r --arg id "$machine" '.machine_owners[$id][0] // empty' "$machine_profile")
    if [ -n "$machine_owner" ]; then
        machine_location=$(jq -er --arg owner "$machine_owner" '
            [.directory.people[] | select(.uid == $owner) | .location][0] |
            if type == "string" and length > 0 then . else error("machine owner has no organization location") end
        ' "$machine_profile")
    else
        machine_owner=$(jq -r --arg id "$machine" '.machines[] | select(.id == $id) | .owner // ""' "$SIMULATION_MANIFEST")
        machine_location=$(jq -er --arg id "$machine" '.machines[] | select(.id == $id) | .location' "$SIMULATION_MANIFEST")
        if [ "$machine_runtime_driver" = docker-multinode ]; then
            machine_location=$(jq -er '
                if .runtime.topology == "single-node" and (.runtime.nodes | length) == 1 then
                    .runtime.nodes[0].location
                else error("multi-node machine requires a configured organization owner") end
            ' "$machine_profile")

        fi
    fi
    if [ "$machine_runtime_driver" = docker-multinode ]; then
        machine_node_index=$(jq -er --arg location "$machine_location" '
            [.runtime.nodes | to_entries[] | select(.value.location == $location) | .key] |
            if length == 1 then .[0] else error("machine location must match exactly one configured node") end
        ' "$machine_profile")
        machine_node_ip=$(jq -er --argjson index "$machine_node_index" '.runtime.nodes[$index].substrate_address' "$machine_profile")
    fi
}

refresh_multinode_services_if_idle() {
    organization_id=$1
    running_machine_containers=$(docker ps --filter label=zpr.machine.id --filter status=running -q)
    [ -z "$running_machine_containers" ] || return 0
    start_multinode_dns_service "$organization_id"
    stop_multinode_control_processes "$organization_id-node0" adapter
}

start_machine_container() {
    machine=$1
    case "$machine" in
        machine-[0-9][0-9]) ;;
        *) echo "invalid machine id: $machine" >&2; return 2 ;;
    esac
    container="zpr-$machine"
    machine_organization=${SIMULATION_ORGANIZATION_ID:-$(jq -r '.organization_id // "northstar"' "$SIMULATION_MANIFEST")}
    if [ -r "$ACTIVE_ORGANIZATION_FILE" ]; then
        selected_organization=$(tr -d '\r\n' < "$ACTIVE_ORGANIZATION_FILE")
        [ -z "$selected_organization" ] || machine_organization=$selected_organization
    fi
    case "$machine_organization" in ''|*[!a-z0-9-]*) echo "invalid active organization: $machine_organization" >&2; return 1 ;; esac
    machine_profile="$ORGANIZATIONS_DIR/$machine_organization.json"
    machine_runtime_driver=$(jq -er '.runtime.driver' "$machine_profile")
    resolve_machine_placement
    case "$machine_runtime_driver" in
        linux-one-node) machine_arch=arm64 ;;
        docker-multinode)
            machine_host_arch=$(docker info --format '{{.Architecture}}')
            case "$machine_host_arch" in
                arm64|aarch64) machine_arch=arm64 ;;
                amd64|x86_64) machine_arch=amd64 ;;
                *) echo "unsupported Docker machine architecture: $machine_host_arch" >&2; return 1 ;;
            esac
            ;;
        *) echo "unsupported machine runtime driver: $machine_runtime_driver" >&2; return 1 ;;
    esac
    existing=$(docker inspect -f '{{.State.Status}}' "$container" 2>/dev/null || true)
    if [ "$existing" = running ]; then
        return 0
    fi
    if [ "$existing" = exited ] || [ "$existing" = created ]; then
        docker rm "$container" >/dev/null
    fi
    ensure_machine_runtime_arch "$machine_arch"
    machine_node_address=10.0.0.1:5000
    machine_network=
    machine_control_url="https://[fd5a:5052:adda:1:ffff:ffff:ffff:fffe]:8792"
    if [ "$machine_runtime_driver" = docker-multinode ]; then
        machine_node_address="$machine_node_ip:5000"
        machine_network="zpr-$machine_organization"
        refresh_multinode_services_if_idle "$machine_organization"
        start_zpr_machine_control_service
        machine_control_address=$(cat "$MACHINE_CONTROL_ADDRESS_FILE")
        case "$machine_control_address" in *:*) ;; *) echo "invalid multinode SimulatorControl address" >&2; return 1 ;; esac
        machine_control_url="https://[$machine_control_address]:8792"
        prepare_machine_workloads "$machine_arch"
    else
        start_zpr_machine_control_service
        prepare_machine_workloads "$machine_arch"
        stop_legacy_named_workloads
        if [ -z "${RIG_IP:-}" ]; then
            RIG_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$SIMULATION_CONTAINER")
        fi
        [ -n "$RIG_IP" ] || { echo "ZPR rig $SIMULATION_CONTAINER has no IPv4 address" >&2; return 1; }
    fi
    machine_controller_binary=$MACHINE_CONTROLLER_BIN
    if [ "$machine_arch" = amd64 ]; then machine_controller_binary=$MACHINE_CONTROLLER_AMD64_BIN; fi
    set -- docker run -d --name "$container"
    if [ -n "$machine_network" ]; then set -- "$@" --network "$machine_network"; fi
    set -- "$@" \
        --privileged --device /dev/net/tun \
        --label zpr.simulator=true \
        --label "zpr.machine.id=$machine" \
        --label "zpr.machine.type=$(jq -r --arg id "$machine" '.machines[] | select(.id == $id) | .type' "$SIMULATION_MANIFEST")" \
        --label "zpr.machine.location=$machine_location" \
        --label "zpr.machine.model=$(jq -r --arg id "$machine" '.machines[] | select(.id == $id) | .model' "$SIMULATION_MANIFEST")" \
        --label "zpr.machine.owner=$machine_owner" \
        --label "zpr.machine.secure=$(jq -r --arg id "$machine" '.machines[] | select(.id == $id) | .secure' "$SIMULATION_MANIFEST")" \
        -v "$machine_controller_binary:/usr/local/bin/zpr-machine-controller:ro" \
        -v "$MACHINE_CERT_DIR/$machine:/run/zpr-machine:ro" \
        -v "$MACHINE_WORKLOAD_DIR:/opt/zpr-workloads:ro" \
        "$MACHINE_IMAGE" \
        /bin/sh -c 'mkdir -p /run/zpr-workloads && exec /usr/local/bin/zpr-machine-controller "$@"' zpr-machine \
        -mode machine-controller -machine-id "$machine" \
        -control-url "$machine_control_url" \
        -control-ca /run/zpr-machine/control-ca.crt \
        -client-cert /run/zpr-machine/client.crt \
        -client-key /run/zpr-machine/client.key \
        -zpr-ph /opt/zpr-workloads/ph \
        -zpr-bootstrap-key "/opt/zpr-workloads/machine-controller-$machine.key" \
        -zpr-node-addr "$machine_node_address"
    "$@" >/dev/null
    if [ "$machine_runtime_driver" = linux-one-node ]; then
        docker exec "$container" ip route replace 10.0.0.0/8 via "$RIG_IP"
    fi
}

start_zpr_machine_control_service() {
    proxy_in_rig=/tmp/zpr-machine-control-proxy-linux-arm64
    service_dir=/tmp/zpr-machine-control
    service_socket="$service_dir/control.sock"
    service_log="$service_dir/adapter.log"
    proxy_log="$service_dir/proxy.log"
    key_dir=/work/.local-runtime/linux-integration/pregen/machine-control
    machine_organization=${SIMULATION_ORGANIZATION_ID:-$(jq -r '.organization_id // "northstar"' "$SIMULATION_MANIFEST")}
    if [ -r "$ACTIVE_ORGANIZATION_FILE" ]; then
        selected_organization=$(tr -d '\r\n' < "$ACTIVE_ORGANIZATION_FILE")
        [ -z "$selected_organization" ] || machine_organization=$selected_organization
    fi
    machine_profile="$ORGANIZATIONS_DIR/$machine_organization.json"
    machine_runtime_driver=$(jq -er '.runtime.driver' "$machine_profile")
    if [ "$machine_runtime_driver" = docker-multinode ]; then
        node_container="$machine_organization-node0"
        node_ip=$(jq -er '.runtime.nodes[0].substrate_address' "$machine_profile")
        proxy_in_node=/tmp/zpr-machine-control-proxy-linux-amd64
        service_dir=/tmp/zpr-machine-control
        service_socket="$service_dir/control.sock"
        key_dir="$RUNTIME_DIR/linux-integration/pregen/machine-control"
        docker exec "$node_container" mkdir -p "$service_dir"
        docker cp "$MACHINE_CONTROL_PROXY_AMD64_BIN" "$node_container:$proxy_in_node"
        if ! docker exec "$node_container" /app/bin/ph-cli -p "$service_socket" link show 2>/dev/null | grep -q '(Active)'; then
            stop_multinode_control_processes "$node_container" adapter
            stop_multinode_control_processes "$node_container" proxy
            docker cp "$key_dir/simulator-control.key" "$node_container:$service_dir/simulator-control.key"
            docker exec -d "$node_container" sh -c \
                'exec env ZPR_ADAPTER_SERVICES=SimulatorControlService /app/bin/ph adapter --logging all=INFO --control-path "$1" --capture-path "$2" --self-addr 0.0.0.0:0 --ca-file /conf/include/auth-ca.crt --bootstrap-key "$3/simulator-control.key" --name simulator-control --km-impl noise --tun-if tun5 --node-addr "$4:5000" --zpr-addr fd5a:5052:adda:1:ffff:ffff:ffff:fffe >>"$5" 2>&1' \
                machine-control-service "$service_socket" "$service_dir/control-cap.sock" "$service_dir" "$node_ip" "$service_dir/adapter.log"
            attempts=0
            while [ "$attempts" -lt 90 ]; do
                if docker exec "$node_container" /app/bin/ph-cli -p "$service_socket" link show 2>/dev/null | grep -q '(Active)'; then
                    break
                fi
                attempts=$((attempts + 1))
                sleep 1
            done
            if [ "$attempts" -ge 90 ]; then
                docker exec "$node_container" tail -n 80 "$service_dir/adapter.log" >&2 || true
                echo "multinode simulator-control adapter did not become active on $node_container" >&2
                return 1
            fi
        fi
        control_address=$(docker exec "$node_container" ip -6 -o addr show dev tun5 scope global | awk 'NR == 1 { split($4, address, "/"); print address[1]; exit }' | tr -d '\r\n')
        case "$control_address" in *:*) ;; *) echo "multinode SimulatorControl adapter has no active IPv6 address" >&2; return 1 ;; esac
        printf '%s\n' "$control_address" > "$MACHINE_CONTROL_ADDRESS_FILE"
        configure_multinode_service_return_route "$node_container" tun5 "$control_address" 105
        control_upstream_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$SIMULATOR_DOCKER_CONTAINER")
        [ -n "$control_upstream_ip" ] || { echo "Simulator container has no reachable Docker IPv4 address" >&2; return 1; }
        proxy_processes=$(docker exec "$node_container" ps -eo pid,args)
        if ! printf '%s\n' "$proxy_processes" | awk -v binary="$proxy_in_node" -v upstream="$control_upstream_ip:8791" '
            $2 == binary {
                for (i = 3; i < NF; i++) if ($i == "--proxy-upstream" && $(i+1) == upstream) found = 1
            }
            END { exit !found }
        '; then
            stop_multinode_control_processes "$node_container" proxy
            docker exec -d "$node_container" sh -c \
            'exec "$1" --mode machine-control-proxy --proxy-listen "[::]:8792" --proxy-upstream "$3" >>"$2" 2>&1' \
            machine-control-proxy "$proxy_in_node" "$service_dir/proxy.log" "$control_upstream_ip:8791"
        fi
        return 0
    fi
    [ "$machine_runtime_driver" = linux-one-node ] || { echo "unsupported machine-control runtime: $machine_runtime_driver" >&2; return 1; }
    rig_dir=$(docker exec "$SIMULATION_CONTAINER" sh -c 'for socket in $(find /tmp -maxdepth 3 -type s -name node.sock -print); do if /tmp/zpr-core-target/debug/ph-cli -p "$socket" link show >/dev/null 2>&1; then dirname "$socket"; exit 0; fi; done')
    if [ -z "$rig_dir" ]; then
        echo "active ZPR rig credentials not found in $SIMULATION_CONTAINER" >&2
        return 1
    fi
    docker cp "$MACHINE_CONTROL_PROXY_BIN" "$SIMULATION_CONTAINER:$proxy_in_rig"
    docker exec "$SIMULATION_CONTAINER" pkill -TERM -f '[p]h adapter.*--name simulator-control' 2>/dev/null || true
    docker exec "$SIMULATION_CONTAINER" pkill -TERM -f '[z]pr-machine-control-proxy-linux-arm64' 2>/dev/null || true
    docker exec -d "$SIMULATION_CONTAINER" sh -c \
        'mkdir -p "$1"; exec ip netns exec zpr-vs env ZPR_ADAPTER_SERVICES=SimulatorControlService /tmp/zpr-core-target/debug/ph adapter --logging all=INFO --control-path "$2" --capture-path "$3" --self-addr 0.0.0.0:0 --ca-file "$4/ca.crt" --bootstrap-key "$5/simulator-control.key" --name simulator-control --km-impl noise --tun-if tun5 --node-addr 10.0.0.1:5000 --zpr-addr fd5a:5052:adda:1:ffff:ffff:ffff:fffe >>"$6" 2>&1' \
        machine-control-service "$service_dir" "$service_socket" "$service_dir/control-cap.sock" "$rig_dir" "$key_dir" "$service_log"
    attempts=0
    while [ "$attempts" -lt 45 ]; do
        if docker exec "$SIMULATION_CONTAINER" /tmp/zpr-core-target/debug/ph-cli -p "$service_socket" link show 2>/dev/null | grep -q '(Active)'; then
            break
        fi
        attempts=$((attempts + 1))
        sleep 1
    done
    if [ "$attempts" -ge 45 ]; then
        echo "simulator-control PH adapter did not become active" >&2
        return 1
    fi
    docker exec "$SIMULATION_CONTAINER" ip netns exec zpr-vs sh -c '
        ip -6 route show fd5a:5052::/32 |
        while read -r prefix device_keyword device rest; do
            case "$rest" in
                *linkdown*)
                    if [ "$device_keyword" = dev ] && [ "$device" != tun5 ]; then
                        ip -6 route del "$prefix" dev "$device" 2>/dev/null || true
                    fi
                    ;;
            esac
        done
    '
    docker exec "$SIMULATION_CONTAINER" ip netns exec zpr-vs ip route replace 10.254.0.0/30 via 10.0.0.1 dev veth0
    docker exec -d "$SIMULATION_CONTAINER" sh -c \
        'exec "$1" --mode machine-control-proxy --proxy-listen 10.254.0.1:8793 --proxy-upstream host.docker.internal:8791 >>"$2" 2>&1' \
        machine-control-root-proxy "$proxy_in_rig" "$service_dir/root-proxy.log"
    docker exec -d "$SIMULATION_CONTAINER" sh -c \
        'exec ip netns exec zpr-vs "$1" --mode machine-control-proxy --proxy-listen "[fd5a:5052:adda:1:ffff:ffff:ffff:fffe]:8792" --proxy-upstream 10.254.0.1:8793 >>"$2" 2>&1' \
        machine-control-proxy "$proxy_in_rig" "$proxy_log"
}

stop_multinode_control_processes() {
    control_processes=$(docker exec "$1" ps -eo pid,args)
    control_process_ids=$(printf '%s\n' "$control_processes" | awk -v kind="$2" '
        kind == "proxy" && $2 == "/tmp/zpr-machine-control-proxy-linux-amd64" { print $1 }
        kind == "adapter" && $2 == "/app/bin/ph" && $3 == "adapter" {
            for (i = 4; i < NF; i++) if ($i == "--name" && $(i+1) == "simulator-control") print $1
        }
    ')
    for control_process_id in $control_process_ids; do
        docker exec "$1" kill "$control_process_id"
        control_stop_attempts=0
        while docker exec "$1" kill -0 "$control_process_id" 2>/dev/null; do
            control_stop_attempts=$((control_stop_attempts + 1))
            if [ "$control_stop_attempts" -ge 50 ]; then
                echo "machine-control process $control_process_id did not stop in $1" >&2
                return 1
            fi
            sleep 0.1
        done
    done
}

stop_legacy_named_workloads() {
    for agent in finance-client operations-client telemetry-client echo-service metrics-service internet-gateway; do
        socket=$(docker exec "$SIMULATION_CONTAINER" sh -c "find /tmp -maxdepth 3 -type s -name '$agent.sock' -print -quit" 2>/dev/null || true)
        if [ -n "$socket" ]; then
            link_id=$(docker exec "$SIMULATION_CONTAINER" /tmp/zpr-core-target/debug/ph-cli -p "$socket" link show 2>/dev/null | awk '/^[[:space:]]*[0-9]+:/ { sub(":", "", $1); print $1; exit }' || true)
            if [ -n "$link_id" ]; then
                docker exec "$SIMULATION_CONTAINER" /tmp/zpr-core-target/debug/ph-cli -p "$socket" link stop "$link_id" >/dev/null 2>&1 || true
            fi
        fi
        docker exec "$SIMULATION_CONTAINER" pkill -TERM -f "[p]h adapter.*--name $agent( |$)" 2>/dev/null || true
    done
}

build_arm64_machine_workload_tools() {
    arm64_builder_image=zpr-machine-workload-arm64-builder:local
    arm64_builder_dir="$PROJECT_ROOT/zpr-demo/multinode-demo"
    arm64_cargo_home="$RUNTIME_DIR/arm64-machine-workload-cargo"
    mkdir -p "$arm64_cargo_home"
    if ! docker image inspect "$arm64_builder_image" >/dev/null 2>&1; then
        docker build --platform linux/arm64 -t "$arm64_builder_image" \
            -f "$arm64_builder_dir/Dockerfile.build-linux" "$arm64_builder_dir"
    fi
    docker run --rm --platform linux/arm64 -e ZPR_ROOT=/work -e CARGO_HOME=/cargo-home \
        -v "$PROJECT_ROOT:/work" -v "$arm64_cargo_home:/cargo-home" "$arm64_builder_image" \
        /bin/bash /work/zpr-demo/multinode-demo/build-linux-arm64-machine-tools.sh
    cp "$PROJECT_ROOT/zpr-core/target/linux-arm64-machine-tools/release/ph" "$MACHINE_WORKLOAD_DIR/ph"
    cp "$PROJECT_ROOT/zpr-core/target/linux-arm64-machine-tools/release/ph-cli" "$MACHINE_WORKLOAD_DIR/ph-cli"
}

prepare_machine_workloads() {
    workload_arch=$1
    mkdir -p "$MACHINE_WORKLOAD_DIR"
    chmod 700 "$MACHINE_WORKLOAD_DIR"
    machine_organization=${SIMULATION_ORGANIZATION_ID:-$(jq -r '.organization_id // "northstar"' "$SIMULATION_MANIFEST")}
    if [ -r "$ACTIVE_ORGANIZATION_FILE" ]; then
        selected_organization=$(tr -d '\r\n' < "$ACTIVE_ORGANIZATION_FILE")
        [ -z "$selected_organization" ] || machine_organization=$selected_organization
    fi
    machine_profile="$ORGANIZATIONS_DIR/$machine_organization.json"
    machine_runtime_driver=$(jq -er '.runtime.driver' "$machine_profile")
    workload_runtime_marker="$machine_runtime_driver:$machine_organization:$workload_arch"
    if [ "$(cat "$MACHINE_WORKLOAD_DIR/.runtime" 2>/dev/null || true)" = "$workload_runtime_marker" ] && [ -x "$MACHINE_WORKLOAD_DIR/ph" ]; then
        return 0
    fi
    rm -f "$MACHINE_WORKLOAD_DIR"/*
    rm -f "$MACHINE_WORKLOAD_DIR/.runtime"
    if [ "$machine_runtime_driver" = docker-multinode ]; then
        node_container="$machine_organization-node0"
        runtime_root="$RUNTIME_DIR/multinode/$machine_organization"
        pregen="$RUNTIME_DIR/linux-integration/pregen"
        if [ "$workload_arch" = arm64 ]; then
            build_arm64_machine_workload_tools
        else
            docker cp "$node_container:/app/bin/ph" "$MACHINE_WORKLOAD_DIR/ph"
            docker cp "$node_container:/app/bin/ph-cli" "$MACHINE_WORKLOAD_DIR/ph-cli"
        fi
        cp "$runtime_root/conf/node0/include/auth-ca.crt" "$MACHINE_WORKLOAD_DIR/ca.crt"
        for file in client-finance-rsa.key client-operations-rsa.key client-telemetry-rsa.key service-echo-rsa.key service-metrics-rsa.key internet-gateway-rsa.key; do
            cp "$pregen/$file" "$MACHINE_WORKLOAD_DIR/$file"
        done
        for number in $(seq -w 1 20); do
            cp "$pregen/machine-control/machine-$number.key" "$MACHINE_WORKLOAD_DIR/machine-controller-machine-$number.key"
        done
        chmod 700 "$MACHINE_WORKLOAD_DIR/ph" "$MACHINE_WORKLOAD_DIR/ph-cli"
        chmod 600 "$MACHINE_WORKLOAD_DIR"/*.key
        printf '%s\n' "$workload_runtime_marker" > "$MACHINE_WORKLOAD_DIR/.runtime"
        return 0
    fi
    [ "$machine_runtime_driver" = linux-one-node ] || { echo "unsupported machine workload runtime: $machine_runtime_driver" >&2; return 1; }
    rig_dir=$(docker exec "$SIMULATION_CONTAINER" sh -c 'find /tmp -maxdepth 3 -type f -name client-finance-rsa.key -print -quit | xargs -r dirname')
    if [ -z "$rig_dir" ]; then
        echo "active ZPR rig credentials not found in $SIMULATION_CONTAINER" >&2
        return 1
    fi
    RIG_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$SIMULATION_CONTAINER")
    DOCKER_SUBNET=$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Subnet}}')
    if [ "$workload_arch" = arm64 ]; then
        build_arm64_machine_workload_tools
    else
        docker cp "$SIMULATION_CONTAINER:/tmp/zpr-core-target/debug/ph" "$MACHINE_WORKLOAD_DIR/ph"
        docker cp "$SIMULATION_CONTAINER:/tmp/zpr-core-target/debug/ph-cli" "$MACHINE_WORKLOAD_DIR/ph-cli"
    fi
    for file in ca.crt client-finance-rsa.key client-operations-rsa.key client-telemetry-rsa.key service-echo-rsa.key service-metrics-rsa.key internet-gateway-rsa.key; do
        docker cp "$SIMULATION_CONTAINER:$rig_dir/$file" "$MACHINE_WORKLOAD_DIR/$file"
    done
    for number in $(seq -w 1 20); do
        machine="machine-$number"
        docker cp "$SIMULATION_CONTAINER:/work/.local-runtime/linux-integration/pregen/machine-control/$machine.key" "$MACHINE_WORKLOAD_DIR/machine-controller-$machine.key"
    done
    chmod 700 "$MACHINE_WORKLOAD_DIR/ph" "$MACHINE_WORKLOAD_DIR/ph-cli"
    chmod 600 "$MACHINE_WORKLOAD_DIR"/*.key
    docker exec "$SIMULATION_CONTAINER" sh -lc "set -eu
        if ! command -v iptables >/dev/null 2>&1; then
            echo 'iptables is required for the machine underlay bridge' >&2
            exit 1
        fi
        if ! ip link show sim-host0 >/dev/null 2>&1; then
            ip link add sim-host0 type veth peer name sim-node0
            ip link set sim-node0 netns zpr-node
            ip addr add 10.254.0.1/30 dev sim-host0
            ip link set sim-host0 up
            ip -n zpr-node addr add 10.254.0.2/30 dev sim-node0
            ip -n zpr-node link set sim-node0 up
        fi
        ip -n zpr-node route replace '$DOCKER_SUBNET' via 10.254.0.1 dev sim-node0
        ip route replace 10.0.0.0/8 via 10.254.0.2 dev sim-host0
        sysctl -w net.ipv4.ip_forward=1 >/dev/null
        if command -v iptables >/dev/null 2>&1; then
            iptables -t nat -D PREROUTING -i eth0 -d '$RIG_IP' -p udp --dport 5000 -j DNAT --to-destination 10.0.0.1:5000 2>/dev/null || true
            iptables -D FORWARD -i eth0 -o sim-host0 -d 10.0.0.1 -p udp --dport 5000 -j ACCEPT 2>/dev/null || true
            iptables -D FORWARD -i sim-host0 -o eth0 -s 10.0.0.0/8 -d '$DOCKER_SUBNET' -p udp -j ACCEPT 2>/dev/null || true
            iptables -D FORWARD -i eth0 -o sim-host0 -d 10.0.0.0/8 -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
        fi"
    printf '%s\n' "$workload_runtime_marker" > "$MACHINE_WORKLOAD_DIR/.runtime"
}

restart_policy_context() {
    context_organization=$1
    context_source=$2
    case "$context_organization" in ''|*[!a-z0-9-]*) echo "invalid organization context" >&2; return 1 ;; esac
    context_profile="$ORGANIZATIONS_DIR/$context_organization.json"
    [ -r "$context_profile" ] && [ -r "$context_source" ] || { echo "policy context assets unavailable" >&2; return 1; }
    context_config="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/$(jq -er '.policy_config' "$context_profile")"
    context_catalog="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/$(jq -er '.policy_catalog' "$context_profile")"
    context_base_dn=$(jq -er '.directory.base_dn' "$context_profile")
    context_ldap_container=$(policy_ldap_container "$context_organization")
    [ -r "$context_config" ] && [ -r "$context_catalog" ] || { echo "policy profile assets unavailable" >&2; return 1; }
    prepare_policy_tester
    start_policy_context_container "$context_organization" "$context_source" \
        "Network/Effective" "Effective network policy" "$STATE_DIR/staged-policy/$context_organization"
    wait_for_policy_context "$context_organization"
}

start_policy_context_container() {
    context_organization=$1
    context_source=$2
    context_seed_category=$3
    context_seed_name=$4
    context_stage_dir=$5
    context_profile="$ORGANIZATIONS_DIR/$context_organization.json"
    context_config="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/$(jq -er '.policy_config' "$context_profile")"
    context_catalog="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/$(jq -er '.policy_catalog' "$context_profile")"
    context_base_dn=$(jq -er '.directory.base_dn' "$context_profile")
    context_ldap_container=$(policy_ldap_container "$context_organization")
    stop_policy_service
    docker run -d --name "$POLICY_CONTAINER" \
        --label zpr.policy-service=true \
        --restart unless-stopped \
        -v "$(docker_socket_path):/var/run/docker.sock" \
        -v "$DASHBOARD_DIR:$DASHBOARD_DIR:ro" \
        -v "$RUNTIME_DIR:$RUNTIME_DIR" \
        -p 127.0.0.1:8789:8789 \
        -e ZPR_POLICY_SERVICE_LISTEN=0.0.0.0:8789 \
        -e ZPR_POLICY_DB_FILE="$STATE_DIR/policy-private/$context_organization-policy-only.db" \
        -e ZPR_POLICY_SERVICE_CERT_FILE="$SERVICE_CERTS/policy-service.crt" \
        -e ZPR_POLICY_SERVICE_KEY_FILE="$SERVICE_CERTS/policy-service.key" \
        -e ZPR_POLICY_SERVICE_CLIENT_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        -e ZPR_POLICY_STAGE_CONFIG_FILE="$RUNTIME_DIR/linux-integration/pregen/v6-1node-3actor-ping.zplc" \
        -e ZPR_POLICY_ORGANIZATION_ID="$context_organization" \
        -e ZPR_POLICY_ORGANIZATION_NAME="$(jq -er '.name' "$context_profile")" \
        -e ZPR_POLICY_ORGANIZATION_BASE_DN="$context_base_dn" \
        -e ZPR_POLICY_CONFIG_FILE="$context_config" \
        -e ZPR_POLICY_SOURCE_FILE="$context_source" \
        -e ZPR_POLICY_SEED_CATEGORY="$context_seed_category" \
        -e ZPR_POLICY_SEED_NAME="$context_seed_name" \
        -e ZPR_POLICY_DEMO_CATALOG_FILE="$context_catalog" \
        -e ZPR_POLICY_STAGE_DIR="$context_stage_dir" \
        -e ZPR_POLICY_STAGE_SIGNING_KEY_FILE="$RUNTIME_DIR/linux-integration/pregen/zpr-rsa-key.pem" \
        -e ZPR_POLICY_LDAP_CONTAINER="$context_ldap_container" \
        -e ZPR_POLICY_LDAP_BASE_DN="$context_base_dn" \
        -e ZPR_POLICY_LDAP_BIND_DN="cn=zpr-reader,ou=Service Accounts,$context_base_dn" \
        -e SIMULATION_MANIFEST="$SIMULATION_MANIFEST" \
        -e SIMULATION_ORGANIZATIONS_DIR="$ORGANIZATIONS_DIR" \
        -e ZPR_ZPT_BIN="$POLICY_TOOLS_DIR/zpt" \
        -e ZPR_ZPLC_BIN="$POLICY_TOOLS_DIR/zplc" \
        --entrypoint /usr/local/bin/zpr-web-dashboard \
        "$SIMULATOR_IMAGE" -mode policy-service >/dev/null
}

policy_ldap_container() {
    organization=$1
    profile="$ORGANIZATIONS_DIR/$organization.json"
    driver=$(jq -er '.runtime.driver' "$profile")
    case "$driver" in
        docker-multinode) printf '%s-directory' "$organization" ;;
        linux-one-node) printf '%s' "$SIMULATION_CONTAINER" ;;
        *) echo "unsupported organization runtime driver: $driver" >&2; return 1 ;;
    esac
}

prepare_policy_tester() {
    if [ -n "${ZPR_ZPT_BIN:-}" ]; then
        POLICY_TESTER_BIN=$ZPR_ZPT_BIN
        if [ ! -x "$POLICY_TESTER_BIN" ]; then
            echo "ZPT evaluator is not executable: $POLICY_TESTER_BIN" >&2
            return 1
        fi
        return 0
    fi
    POLICY_TESTER_BIN="$DASHBOARD_DIR/../target/debug/zpt"
    cargo build --manifest-path "$DASHBOARD_DIR/../zpt/Cargo.toml" --bin zpt
    if [ ! -x "$POLICY_TESTER_BIN" ]; then
        echo "ZPT evaluator build did not produce $POLICY_TESTER_BIN" >&2
        return 1
    fi
}

policy_startup_organization() {
    selected=${SIMULATION_ORGANIZATION_ID:-}
    if [ -z "$selected" ] && [ -r "$ACTIVE_ORGANIZATION_FILE" ]; then
        selected=$(tr -d '\r\n' < "$ACTIVE_ORGANIZATION_FILE")
    fi
    if [ -z "$selected" ]; then
        selected=$(jq -r '.organization_id // "northstar"' "$SIMULATION_MANIFEST")
    fi
    case "$selected" in
        ''|*[!a-z0-9-]*) echo "invalid policy organization id" >&2; return 1 ;;
    esac
    printf '%s\n' "$selected"
}

start_policy_service() {
    if [ ! -f "$SIMULATION_MANIFEST" ]; then
        echo "simulation manifest not found: $SIMULATION_MANIFEST" >&2
        return 1
    fi
    organization_id=$(policy_startup_organization)
    case "$organization_id" in
        ''|*[!a-z0-9-]*) echo "invalid simulation organization id: $organization_id" >&2; return 1 ;;
    esac
    organization_file="$ORGANIZATIONS_DIR/$organization_id.json"
    if [ ! -r "$organization_file" ]; then
        echo "simulation organization profile not found: $organization_file" >&2
        return 1
    fi
    policy_config_relative=$(jq -er '.policy_config' "$organization_file")
    policy_catalog_relative=$(jq -er '.policy_catalog' "$organization_file")
    ldap_base_dn=$(jq -er '.directory.base_dn' "$organization_file")
    ldap_bind_dn="cn=zpr-reader,ou=Service Accounts,$ldap_base_dn"
    ldap_container=$(policy_ldap_container "$organization_id")
    policy_config="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/$policy_config_relative"
    policy_catalog="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/$policy_catalog_relative"
    if [ ! -r "$policy_config" ] || [ ! -r "$policy_catalog" ]; then
        echo "policy assets for organization $organization_id are missing" >&2
        return 1
    fi
    ensure_policy_tools
    docker build -f "$SCRIPT_DIR/Dockerfile.simulator" -t "$SIMULATOR_IMAGE" "$DASHBOARD_DIR"
    start_policy_context_container "$organization_id" \
        "$RUNTIME_DIR/linux-integration/pregen/v4-1node-3actor-ping.zpl" \
        "Simulator/Runtime" "Simulator runtime policy" "$STATE_DIR/staged-policy"
    wait_for_policy_context "$organization_id"
    wait_for_url https://127.0.0.1:8789/api/policy policy-service \
        --cacert "$SERVICE_CERTS/service-ca.crt" \
        --cert "$SERVICE_CERTS/control-policy-client.crt" \
        --key "$SERVICE_CERTS/control-policy-client.key"
}

restart_policy_service() {
    stop_policy_service
    start_policy_service
}

start_stack() {
    mkdir -p "$STATE_DIR"
    stop_stack
    create_machine_control_pki
    start_simulator
    start_policy_service
    startup_organization=${SIMULATION_ORGANIZATION_ID:-$(jq -r '.organization_id // "northstar"' "$SIMULATION_MANIFEST")}
    if [ -r "$ACTIVE_ORGANIZATION_FILE" ]; then
        selected_organization=$(tr -d '\r\n' < "$ACTIVE_ORGANIZATION_FILE")
        [ -z "$selected_organization" ] || startup_organization=$selected_organization
    fi
    startup_profile="$ORGANIZATIONS_DIR/$startup_organization.json"
    startup_driver=$(jq -er '.runtime.driver' "$startup_profile")
    if [ "$startup_driver" = linux-one-node ]; then
        start_admin_relay
        start_dns_service
        start_ui_relays
        ZPR_GATEWAY_ORGANIZATION_ID="$startup_organization" start_control_service
        start_machine_controllers
    else
        multinode_runtime="$RUNTIME_DIR/multinode/$startup_organization"
        SIMULATION_ORGANIZATION_ID="$startup_organization" \
        ZPR_ASSERTION_LDAP_CONTAINER="${startup_organization}-directory" \
        ZPR_ASSERTION_LDAP_BASE_DN="$(jq -er '.directory.base_dn' "$startup_profile")" \
        ZPR_ASSERTION_LDAP_BIND_DN="cn=zpr-reader,ou=Service Accounts,$(jq -er '.directory.base_dn' "$startup_profile")" \
        ZPR_ADMIN_URL=https://127.0.0.1:8185 \
        ZPR_ADMIN_CA_FILE="$DASHBOARD_DIR/../../zpr-demo/multinode-demo/zpr-conf/include/admin-tls-cert.pem" \
        ZPR_ADMIN_KEY_FILE="$multinode_runtime/bob/web-monitor.key" \
        ZPR_GATEWAY_ORGANIZATION_ID="$startup_organization" \
            start_control_service
        docker exec -e SIMULATION_ORGANIZATION_ID="$startup_organization" "$SIMULATOR_DOCKER_CONTAINER" \
            sh "$SCRIPT_DIR/dashboard-stack.sh" start-dns
    fi
    if [ "$startup_driver" = docker-multinode ]; then
        start_observability_collector "$startup_organization"
    fi
    start_control_room
    wait_for_control_room
    if [ -r "$OPERATOR_DIR/stack.json" ]; then
        echo "Control Room ready at $(jq -er '.origin' "$OPERATOR_DIR/stack.json")"
    else
        echo "Control Room ready at http://127.0.0.1:8787"
    fi
    echo "OpenObserve Logger ready at http://127.0.0.1:$OBSERVABILITY_LOCAL_PORT"
    echo "LDAP editor relay at http://127.0.0.1:$LDAP_UI_RELAY_PORT/"
    echo "Simulator ready at http://127.0.0.1:8788"
    echo "Active organization: $organization_id"
    echo "Machine control mTLS listener ready at https://127.0.0.1:8791"
}

status_stack() {
    policy_state=$(docker inspect -f '{{.State.Status}}' "$POLICY_CONTAINER" 2>/dev/null || printf stopped)
    echo "policy-service: container $policy_state"
    control_state=$(docker inspect -f '{{.State.Status}}' "$CONTROL_CONTAINER" 2>/dev/null || printf stopped)
    echo "control-service: container $control_state"
    for entry in "admin-relay:$ADMIN_RELAY_PID:$ADMIN_RELAY_PORT" "ldap-ui-relay:$LDAP_UI_RELAY_PID:$LDAP_UI_RELAY_PORT" "observability-ui-relay:$OBSERVABILITY_UI_RELAY_PID:$OBSERVABILITY_UI_RELAY_PORT" "browser-gateway:$BROWSER_GATEWAY_PID:8443"; do
        name=${entry%%:*}
        rest=${entry#*:}
        pid_file=${rest%%:*}
        port=${rest#*:}
        if pid_running "$pid_file"; then
            echo "$name: running (pid $(cat "$pid_file"), port $port)"
        else
            echo "$name: stopped"
        fi
    done
    control_room_state=$(docker inspect -f '{{.State.Status}}' "$CONTROL_ROOM_DOCKER_CONTAINER" 2>/dev/null || printf stopped)
    echo "control-room: container $control_room_state"
    observability_state=$(docker inspect -f '{{.State.Status}}' "$OBSERVABILITY_LOCAL_CONTAINER" 2>/dev/null || printf stopped)
    echo "observability: container $observability_state (127.0.0.1:$OBSERVABILITY_LOCAL_PORT)"
    collector_state=$(docker inspect -f '{{.State.Status}}' "$OBSERVABILITY_COLLECTOR_CONTAINER" 2>/dev/null || printf stopped)
    echo "observability-collector: container $collector_state"
    simulator_state=$(docker inspect -f '{{.State.Status}}' "$SIMULATOR_DOCKER_CONTAINER" 2>/dev/null || printf stopped)
    echo "simulator: container $simulator_state"
    dns_state=$(docker inspect -f '{{.State.Status}}' "$DNS_CONTAINER" 2>/dev/null || printf stopped)
    echo "dns-bind9: container $dns_state"
    if pid_running "$DNS_STATS_RELAY_PID"; then
        echo "dns-stats-relay: running (pid $(cat "$DNS_STATS_RELAY_PID"), port $DNS_STATS_RELAY_PORT)"
    else
        echo "dns-stats-relay: stopped"
    fi
    if pid_running "$DNS_RECORDS_RELAY_PID"; then
        echo "dns-records-relay: running (pid $(cat "$DNS_RECORDS_RELAY_PID"), port $DNS_RECORDS_RELAY_PORT)"
    else
        echo "dns-records-relay: stopped"
    fi
    for number in $(seq -w 1 20); do
        container="zpr-machine-$number"
        state=$(docker inspect -f '{{.State.Status}}' "$container" 2>/dev/null || printf stopped)
        echo "machine-$number: container $state"
    done
}

case "${1:-start}" in
    start) start_stack ;;
    start-machine)
        if [ "$#" -ne 2 ]; then echo "usage: $0 start-machine machine-NN" >&2; exit 2; fi
        start_machine_container "$2"
        ;;
    stop) stop_stack ;;
    restart) start_stack ;;
    status) status_stack ;;
    start-admin-relay) start_admin_relay ;;
    stop-admin-relay) stop_admin_relay ;;
    start-dns) start_dns_service ;;
    stop-dns) stop_dns_service ;;
    start-ui-relays) start_ui_relays ;;
    stop-ui-relays) stop_ui_relays ;;
    start-browser-gateway) start_browser_gateway ;;
    stop-browser-gateway) stop_service "$BROWSER_GATEWAY_PID" ;;
    restart-control-room) restart_control_room ;;
    restart-simulator) restart_simulator ;;
    restart-simulator-ui) restart_simulator_ui ;;
    restart-control-service)
        start_control_service
        ;;
    reload-assistant) reload_assistant ;;
    stop-policy-service)
        stop_policy_service
        ;;
    restart-policy-service) restart_policy_service ;;
    restart-simulator-control) start_zpr_machine_control_service ;;
    start-observability-collector) [ "$#" -eq 2 ] || { echo "usage: $0 start-observability-collector organization-id" >&2; exit 2; }; start_observability_collector "$2" ;;
    stop-observability-collector) stop_observability_collector ;;
    stop-legacy-workloads) stop_legacy_named_workloads ;;
    reset-organization) exec sh "$SCRIPT_DIR/activate-organization.sh" "$@" ;;
    restart-policy-context)
        [ "$#" -eq 3 ] || { echo "usage: $0 restart-policy-context organization source" >&2; exit 2; }
        restart_policy_context "$2" "$3"
        ;;
    *) echo "usage: $0 {start|stop|restart|status|start-admin-relay|stop-admin-relay|start-dns|stop-dns|start-ui-relays|stop-ui-relays|start-browser-gateway|stop-browser-gateway|restart-control-room|restart-simulator|restart-control-service|reload-assistant|stop-policy-service|restart-policy-service|restart-simulator-control|start-observability-collector organization-id|stop-observability-collector}" >&2; exit 2 ;;
esac