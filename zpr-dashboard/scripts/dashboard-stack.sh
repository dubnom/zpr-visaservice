#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
DASHBOARD_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
RUNTIME_DIR=$(CDPATH= cd -- "$DASHBOARD_DIR/../../.local-runtime" && pwd)
SERVICE_CERTS="$RUNTIME_DIR/service-certs"
SIMULATION_MANIFEST="${SIMULATION_MANIFEST:-$RUNTIME_DIR/simulation-environment.json}"
STATE_DIR="$RUNTIME_DIR/dashboard-stack"
BIN="$STATE_DIR/zpr-web-dashboard"

POLICY_PID="$STATE_DIR/policy-service.pid"
CONTROL_PID="$STATE_DIR/control-service.pid"
ROOM_PID="$STATE_DIR/control-room.pid"

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

stop_service() {
    pid_file=$1
    if pid_running "$pid_file"; then
        kill "$(cat "$pid_file")" 2>/dev/null || true
    fi
    rm -f "$pid_file"
}

stop_stack() {
    stop_service "$ROOM_PID"
    stop_service "$CONTROL_PID"
    stop_service "$POLICY_PID"
}

start_service() {
    name=$1
    pid_file=$2
    log_file="$STATE_DIR/$name.log"
    shift 2
    "$@" >"$log_file" 2>&1 < /dev/null &
    echo $! >"$pid_file"
}

start_stack() {
    mkdir -p "$STATE_DIR"
    stop_stack
    if [ ! -f "$SIMULATION_MANIFEST" ]; then
        echo "simulation manifest not found: $SIMULATION_MANIFEST" >&2
        return 1
    fi
    go -C "$DASHBOARD_DIR" build -trimpath -o "$BIN" ./cmd/zpr-web-dashboard

    start_service policy-service "$POLICY_PID" env \
        ZPR_POLICY_SERVICE_LISTEN=127.0.0.1:8789 \
        ZPR_POLICY_SERVICE_CERT_FILE="$SERVICE_CERTS/policy-service.crt" \
        ZPR_POLICY_SERVICE_KEY_FILE="$SERVICE_CERTS/policy-service.key" \
        ZPR_POLICY_SERVICE_CLIENT_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        ZPR_POLICY_CONFIG_FILE="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/northstar/policy-demo.zplc" \
        ZPR_POLICY_DEMO_CATALOG_FILE="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/northstar/demo-policy-catalog.json" \
        ZPR_ZPLC_BIN="$DASHBOARD_DIR/../../zpr-compiler/target/debug/zplc" \
        "$BIN" -mode policy-service
    wait_for_url https://127.0.0.1:8789/api/policy policy-service \
        --cacert "$SERVICE_CERTS/service-ca.crt" \
        --cert "$SERVICE_CERTS/control-policy-client.crt" \
        --key "$SERVICE_CERTS/control-policy-client.key"

    start_service control-service "$CONTROL_PID" env \
        ZPR_CONTROL_SERVICE_LISTEN=127.0.0.1:8790 \
        ZPR_CONTROL_SERVICE_CERT_FILE="$SERVICE_CERTS/control-service.crt" \
        ZPR_CONTROL_SERVICE_KEY_FILE="$SERVICE_CERTS/control-service.key" \
        ZPR_CONTROL_SERVICE_CLIENT_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        ZPR_ADMIN_URL=https://127.0.0.1:8183 \
        ZPR_ADMIN_CA_FILE="$RUNTIME_DIR/local-admin-cert.pem" \
        ZPR_ADMIN_KEY_FILE="$RUNTIME_DIR/admin-read.key" \
        ZPR_POLICY_SERVICE_URL=https://127.0.0.1:8789 \
        ZPR_POLICY_SERVICE_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        ZPR_POLICY_CLIENT_CERT_FILE="$SERVICE_CERTS/control-policy-client.crt" \
        ZPR_POLICY_CLIENT_KEY_FILE="$SERVICE_CERTS/control-policy-client.key" \
        "$BIN" -mode control-service
    wait_for_url https://127.0.0.1:8790/api/snapshot control-service \
        --cacert "$SERVICE_CERTS/service-ca.crt" \
        --cert "$SERVICE_CERTS/control-room-client.crt" \
        --key "$SERVICE_CERTS/control-room-client.key"

    start_service control-room "$ROOM_PID" env \
        ZPR_CONTROL_SERVICE_URL=https://127.0.0.1:8790 \
        ZPR_CONTROL_SERVICE_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        ZPR_CONTROL_CLIENT_CERT_FILE="$SERVICE_CERTS/control-room-client.crt" \
        ZPR_CONTROL_CLIENT_KEY_FILE="$SERVICE_CERTS/control-room-client.key" \
        "$BIN" -mode control-room -listen 127.0.0.1:8787
    wait_for_url http://127.0.0.1:8787/ control-room
    echo "Control Room ready at http://127.0.0.1:8787"
}

status_stack() {
    for entry in "policy-service:$POLICY_PID:8789" "control-service:$CONTROL_PID:8790" "control-room:$ROOM_PID:8787"; do
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
}

case "${1:-start}" in
    start) start_stack ;;
    stop) stop_stack ;;
    restart) start_stack ;;
    status) status_stack ;;
    *) echo "usage: $0 {start|stop|restart|status}" >&2; exit 2 ;;
esac