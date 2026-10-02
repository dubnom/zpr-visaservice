#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
DASHBOARD_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
RUNTIME_DIR=$(CDPATH= cd -- "$DASHBOARD_DIR/../../.local-runtime" && pwd)
SERVICE_CERTS="$RUNTIME_DIR/service-certs"
SIMULATION_MANIFEST="${SIMULATION_MANIFEST:-$RUNTIME_DIR/simulation-environment.json}"
STATE_DIR="$RUNTIME_DIR/dashboard-stack"
BIN="$STATE_DIR/zpr-web-dashboard"
CONTROL_DIR="$STATE_DIR/machine-control"
MACHINE_CERT_DIR="$CONTROL_DIR/machine-certs"
CONTROL_CA="$CONTROL_DIR/control-ca.crt"
CONTROL_CA_KEY="$CONTROL_DIR/control-ca.key"
CONTROL_SERVER_CERT="$CONTROL_DIR/simulator.crt"
CONTROL_SERVER_KEY="$CONTROL_DIR/simulator.key"
MACHINE_CONTROLLER_BIN="$STATE_DIR/zpr-machine-controller-linux-arm64"
MACHINE_CONTROL_PROXY_BIN="$STATE_DIR/zpr-machine-control-proxy-linux-arm64"
MACHINE_WORKLOAD_DIR="$STATE_DIR/machine-workloads"
MACHINE_IMAGE="${SIMULATOR_MACHINE_IMAGE:-zpr-sim-machine:local}"
SIMULATION_CONTAINER="${SIMULATION_CONTAINER:-zpr-local-linux-node}"
ORGANIZATIONS_DIR="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/organizations"
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
CONTROL_PID="$STATE_DIR/control-service.pid"
ROOM_PID="$STATE_DIR/control-room.pid"
SIMULATOR_PID="$STATE_DIR/simulator.pid"
ADMIN_RELAY_PID="$STATE_DIR/admin-relay.pid"
ADMIN_RELAY_PORT=8184
LDAP_UI_RELAY_PID="$STATE_DIR/ldap-ui-relay.pid"
LDAP_UI_RELAY_PORT="${ZPR_LDAP_UI_RELAY_PORT:-8797}"
OBSERVABILITY_UI_RELAY_PID="$STATE_DIR/observability-ui-relay.pid"
OBSERVABILITY_UI_RELAY_PORT="${ZPR_OBSERVABILITY_UI_RELAY_PORT:-8798}"
OBSERVABILITY_ADDRESS="${ZPR_OBSERVABILITY_ADDR:-fd5a:5052:adda:1::54}"

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
    while [ "$attempts" -lt 50 ]; do
        if curl --silent --show-error --connect-timeout 1 --max-time 2 "$@" "$url" >/dev/null 2>&1; then
            return 0
        fi
        attempts=$((attempts + 1))
        sleep 0.2
    done
    echo "$label did not respond at $url" >&2
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
    for number in $(seq -w 1 20); do
        docker rm -f "zpr-machine-$number" >/dev/null 2>&1 || true
    done
    stop_service "$SIMULATOR_PID"
    stop_service "$ROOM_PID"
    stop_service "$CONTROL_PID"
    stop_service "$ADMIN_RELAY_PID"
    stop_ui_relays
    stop_service "$POLICY_PID"
    stop_dns_service
}

start_service() {
    name=$1
    pid_file=$2
    log_file="$STATE_DIR/$name.log"
    shift 2
    "$@" >"$log_file" 2>&1 < /dev/null &
    echo $! >"$pid_file"
}

start_admin_relay() {
    start_service admin-relay "$ADMIN_RELAY_PID" socat \
        "TCP-LISTEN:$ADMIN_RELAY_PORT,bind=127.0.0.1,reuseaddr,fork" \
        "SYSTEM:\"/usr/local/bin/docker exec -i $SIMULATION_CONTAINER ip netns exec zpr-vs socat STDIO TCP:[fd5a:5052::1]:8182\""
    wait_for_response "https://127.0.0.1:$ADMIN_RELAY_PORT/admin/stats" admin-relay \
        --cacert "$RUNTIME_DIR/local-admin-cert.pem"
}

start_dns_service() {
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
        "TCP-LISTEN:$DNS_STATS_RELAY_PORT,bind=127.0.0.1,reuseaddr,fork" \
        "SYSTEM:\"docker exec -i $SIMULATION_CONTAINER ip netns exec zpr-a socat STDIO TCP:127.0.0.1:8053\""
    wait_for_url "http://127.0.0.1:$DNS_STATS_RELAY_PORT/json/v1/status" dns-stats-relay

    start_service dns-records-relay "$DNS_RECORDS_RELAY_PID" socat \
        "TCP-LISTEN:$DNS_RECORDS_RELAY_PORT,bind=127.0.0.1,reuseaddr,fork" \
        "SYSTEM:\"docker exec -i $SIMULATION_CONTAINER ip netns exec zpr-a socat STDIO TCP:[$DNS_SERVICE_ADDRESS]:53\""
}

stop_dns_service() {
    stop_service "$DNS_RECORDS_RELAY_PID"
    stop_service "$DNS_STATS_RELAY_PID"
    docker rm -f "$DNS_CONTAINER" >/dev/null 2>&1 || true
}

start_ui_relays() {
    start_service ldap-ui-relay "$LDAP_UI_RELAY_PID" socat \
        "TCP-LISTEN:$LDAP_UI_RELAY_PORT,bind=127.0.0.1,reuseaddr,fork" \
        "SYSTEM:\"docker exec -i $SIMULATION_CONTAINER ip netns exec zpr-vs socat STDIO TCP:127.0.0.1:8080\""
    start_service observability-ui-relay "$OBSERVABILITY_UI_RELAY_PID" socat \
        "TCP-LISTEN:$OBSERVABILITY_UI_RELAY_PORT,bind=127.0.0.1,reuseaddr,fork" \
        "SYSTEM:\"docker exec -i $SIMULATION_CONTAINER ip netns exec zpr-vs socat STDIO TCP:[$OBSERVABILITY_ADDRESS]:5080\""
}

stop_ui_relays() {
    stop_service "$OBSERVABILITY_UI_RELAY_PID"
    stop_service "$LDAP_UI_RELAY_PID"
}

start_control_service() {
    start_service control-service "$CONTROL_PID" env \
        ZPR_CONTROL_SERVICE_LISTEN=127.0.0.1:8790 \
        ZPR_CONTROL_SERVICE_CERT_FILE="$SERVICE_CERTS/control-service.crt" \
        ZPR_CONTROL_SERVICE_KEY_FILE="$SERVICE_CERTS/control-service.key" \
        ZPR_CONTROL_SERVICE_CLIENT_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        ZPR_ADMIN_URL="https://127.0.0.1:$ADMIN_RELAY_PORT" \
        ZPR_ADMIN_CA_FILE="$RUNTIME_DIR/local-admin-cert.pem" \
        ZPR_ADMIN_KEY_FILE="$RUNTIME_DIR/admin-read.key" \
        ZPR_DNS_STATS_URL="${ZPR_DNS_STATS_URL:-http://127.0.0.1:$DNS_STATS_RELAY_PORT}" \
        ZPR_DNS_TRANSFER_ADDR="127.0.0.1:$DNS_RECORDS_RELAY_PORT" \
        ZPR_DNS_TRANSFER_TSIG_KEY_FILE="$DNS_VIEWER_KEY_FILE" \
        ZPR_DEMO_LDAP_EDITOR_URL="http://127.0.0.1:$LDAP_UI_RELAY_PORT/" \
        ZPR_POLICY_SERVICE_URL=https://127.0.0.1:8789 \
        ZPR_POLICY_SERVICE_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        ZPR_POLICY_CLIENT_CERT_FILE="$SERVICE_CERTS/control-policy-client.crt" \
        ZPR_POLICY_CLIENT_KEY_FILE="$SERVICE_CERTS/control-policy-client.key" \
        "$BIN" -mode control-service
    wait_for_url https://127.0.0.1:8790/api/snapshot control-service \
        --cacert "$SERVICE_CERTS/service-ca.crt" \
        --cert "$SERVICE_CERTS/control-room-client.crt" \
        --key "$SERVICE_CERTS/control-room-client.key"
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

start_machine_controllers() {
    docker build -f "$SCRIPT_DIR/Dockerfile.machine" -t "$MACHINE_IMAGE" "$SCRIPT_DIR"
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go -C "$DASHBOARD_DIR" build -trimpath -o "$MACHINE_CONTROLLER_BIN" ./cmd/zpr-web-dashboard
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go -C "$DASHBOARD_DIR" build -trimpath -o "$MACHINE_CONTROL_PROXY_BIN" ./cmd/zpr-web-dashboard
    prepare_machine_workloads
    stop_legacy_named_workloads
    start_zpr_machine_control_service
    start_machine_container machine-01
}

start_machine_container() {
    machine=$1
    case "$machine" in
        machine-[0-9][0-9]) ;;
        *) echo "invalid machine id: $machine" >&2; return 2 ;;
    esac
    number=${machine#machine-}
    container="zpr-$machine"
    existing=$(docker inspect -f '{{.State.Status}}' "$container" 2>/dev/null || true)
    if [ "$existing" = running ]; then
        return 0
    fi
    if [ "$existing" = exited ] || [ "$existing" = created ]; then
        docker rm "$container" >/dev/null
    fi
    if [ -z "${RIG_IP:-}" ]; then
        RIG_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$SIMULATION_CONTAINER")
    fi
    docker run -d --name "$container" \
        --privileged --device /dev/net/tun \
        --label zpr.simulator=true \
        --label "zpr.machine.id=$machine" \
        --label "zpr.machine.type=$(jq -r --arg id "$machine" '.machines[] | select(.id == $id) | .type' "$SIMULATION_MANIFEST")" \
        --label "zpr.machine.location=$(jq -r --arg id "$machine" '.machines[] | select(.id == $id) | .location' "$SIMULATION_MANIFEST")" \
        --label "zpr.machine.model=$(jq -r --arg id "$machine" '.machines[] | select(.id == $id) | .model' "$SIMULATION_MANIFEST")" \
        --label "zpr.machine.owner=$(jq -r --arg id "$machine" '.machines[] | select(.id == $id) | .owner' "$SIMULATION_MANIFEST")" \
        --label "zpr.machine.secure=$(jq -r --arg id "$machine" '.machines[] | select(.id == $id) | .secure' "$SIMULATION_MANIFEST")" \
        -v "$MACHINE_CONTROLLER_BIN:/usr/local/bin/zpr-machine-controller:ro" \
        -v "$MACHINE_CERT_DIR/$machine:/run/zpr-machine:ro" \
        -v "$MACHINE_WORKLOAD_DIR:/opt/zpr-workloads:ro" \
        "$MACHINE_IMAGE" \
        /bin/sh -c 'mkdir -p /run/zpr-workloads && exec /usr/local/bin/zpr-machine-controller "$@"' zpr-machine \
        -mode machine-controller -machine-id "$machine" \
        -control-url "https://[fd5a:5052:adda:1:ffff:ffff:ffff:fffe]:8792" \
        -control-ca /run/zpr-machine/control-ca.crt \
        -client-cert /run/zpr-machine/client.crt \
        -client-key /run/zpr-machine/client.key \
        -zpr-ph /opt/zpr-workloads/ph \
        -zpr-bootstrap-key "/opt/zpr-workloads/machine-controller-$machine.key" \
        -zpr-node-addr 10.0.0.1:5000 >/dev/null
    docker exec "$container" ip route replace 10.0.0.0/8 via "$RIG_IP"
}

start_zpr_machine_control_service() {
    proxy_in_rig=/tmp/zpr-machine-control-proxy-linux-arm64
    service_dir=/tmp/zpr-machine-control
    service_socket="$service_dir/control.sock"
    service_log="$service_dir/adapter.log"
    proxy_log="$service_dir/proxy.log"
    key_dir=/work/.local-runtime/linux-integration/pregen/machine-control
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
    docker exec "$SIMULATION_CONTAINER" ip netns exec zpr-vs ip route replace 10.254.0.0/30 via 10.0.0.1 dev veth0
    docker exec -d "$SIMULATION_CONTAINER" sh -c \
        'exec "$1" --mode machine-control-proxy --proxy-listen 10.254.0.1:8793 --proxy-upstream host.docker.internal:8791 >>"$2" 2>&1' \
        machine-control-root-proxy "$proxy_in_rig" "$service_dir/root-proxy.log"
    docker exec -d "$SIMULATION_CONTAINER" sh -c \
        'exec ip netns exec zpr-vs "$1" --mode machine-control-proxy --proxy-listen "[fd5a:5052:adda:1:ffff:ffff:ffff:fffe]:8792" --proxy-upstream 10.254.0.1:8793 >>"$2" 2>&1' \
        machine-control-proxy "$proxy_in_rig" "$proxy_log"
}

stop_legacy_named_workloads() {
    for agent in finance-client operations-client telemetry-client echo-service metrics-service; do
        socket=$(docker exec "$SIMULATION_CONTAINER" sh -c "find /tmp -maxdepth 3 -type s -name '$agent.sock' -print -quit" 2>/dev/null || true)
        if [ -n "$socket" ]; then
            link_id=$(docker exec "$SIMULATION_CONTAINER" /tmp/zpr-core-target/debug/ph-cli -p "$socket" link show 2>/dev/null | awk '/^[[:space:]]*[0-9]+:/ { sub(":", "", $1); print $1; exit }' || true)
            if [ -n "$link_id" ]; then
                docker exec "$SIMULATION_CONTAINER" /tmp/zpr-core-target/debug/ph-cli -p "$socket" link stop "$link_id" >/dev/null 2>&1 || true
            fi
        fi
        docker exec "$SIMULATION_CONTAINER" pkill -TERM -f "[p]h adapter.*--name $agent" 2>/dev/null || true
    done
}

prepare_machine_workloads() {
    mkdir -p "$MACHINE_WORKLOAD_DIR"
    chmod 700 "$MACHINE_WORKLOAD_DIR"
    rm -f "$MACHINE_WORKLOAD_DIR"/*
    rig_dir=$(docker exec "$SIMULATION_CONTAINER" sh -c 'find /tmp -maxdepth 3 -type f -name client-finance-rsa.key -print -quit | xargs -r dirname')
    if [ -z "$rig_dir" ]; then
        echo "active ZPR rig credentials not found in $SIMULATION_CONTAINER" >&2
        return 1
    fi
    RIG_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$SIMULATION_CONTAINER")
    DOCKER_SUBNET=$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Subnet}}')
    docker cp "$SIMULATION_CONTAINER:/tmp/zpr-core-target/debug/ph" "$MACHINE_WORKLOAD_DIR/ph"
    docker cp "$SIMULATION_CONTAINER:/tmp/zpr-core-target/debug/ph-cli" "$MACHINE_WORKLOAD_DIR/ph-cli"
    for file in ca.crt client-finance-rsa.key client-operations-rsa.key client-telemetry-rsa.key service-echo-rsa.key service-metrics-rsa.key; do
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
}

start_stack() {
    mkdir -p "$STATE_DIR"
    stop_stack
    create_machine_control_pki
    if [ ! -f "$SIMULATION_MANIFEST" ]; then
        echo "simulation manifest not found: $SIMULATION_MANIFEST" >&2
        return 1
    fi
    organization_id=${SIMULATION_ORGANIZATION_ID:-$(jq -r '.organization_id // "northstar"' "$SIMULATION_MANIFEST")}
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
    policy_config="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/$policy_config_relative"
    policy_catalog="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/$policy_catalog_relative"
    if [ ! -r "$policy_config" ] || [ ! -r "$policy_catalog" ]; then
        echo "policy assets for organization $organization_id are missing" >&2
        return 1
    fi
    go -C "$DASHBOARD_DIR" build -trimpath -o "$BIN" ./cmd/zpr-web-dashboard

    start_service policy-service "$POLICY_PID" env \
        ZPR_POLICY_SERVICE_LISTEN=127.0.0.1:8789 \
        ZPR_POLICY_DB_FILE="$STATE_DIR/policy-private/$organization_id-policy-only.db" \
        ZPR_POLICY_SERVICE_CERT_FILE="$SERVICE_CERTS/policy-service.crt" \
        ZPR_POLICY_SERVICE_KEY_FILE="$SERVICE_CERTS/policy-service.key" \
        ZPR_POLICY_SERVICE_CLIENT_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        ZPR_POLICY_CONFIG_FILE="$policy_config" \
        ZPR_POLICY_DEMO_CATALOG_FILE="$policy_catalog" \
        ZPR_ZPLC_BIN="$DASHBOARD_DIR/../../zpr-compiler/target/debug/zplc" \
        "$BIN" -mode policy-service
    wait_for_url https://127.0.0.1:8789/api/policy policy-service \
        --cacert "$SERVICE_CERTS/service-ca.crt" \
        --cert "$SERVICE_CERTS/control-policy-client.crt" \
        --key "$SERVICE_CERTS/control-policy-client.key"

    start_admin_relay
    start_dns_service
    start_ui_relays
    start_control_service

    start_service control-room "$ROOM_PID" env \
        ZPR_CONTROL_SERVICE_URL=https://127.0.0.1:8790 \
        ZPR_CONTROL_SERVICE_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        ZPR_CONTROL_CLIENT_CERT_FILE="$SERVICE_CERTS/control-room-client.crt" \
        ZPR_CONTROL_CLIENT_KEY_FILE="$SERVICE_CERTS/control-room-client.key" \
        "$BIN" -mode control-room -listen 127.0.0.1:8787
    wait_for_url http://127.0.0.1:8787/ control-room
    start_service simulator "$SIMULATOR_PID" env \
        SIMULATION_MANIFEST="$SIMULATION_MANIFEST" \
        SIMULATION_ORGANIZATION_ID="$organization_id" \
        SIMULATION_ORGANIZATIONS_DIR="$ORGANIZATIONS_DIR" \
        SIMULATION_WORKSPACE_DB_DIR="$STATE_DIR/policy-private" \
        SIMULATION_PREGEN_DIR="$RUNTIME_DIR/linux-integration/pregen" \
        SIMULATION_PUBLISHED_DIRECTORY_DIR="$RUNTIME_DIR/published-directories" \
        SIMULATION_SCENARIOS_DIR="$DASHBOARD_DIR/cmd/zpr-web-dashboard/examples/scenarios" \
        SIMULATION_STACK_SCRIPT="$SCRIPT_DIR/dashboard-stack.sh" \
        SIMULATION_AGENT_SCRIPT="$SCRIPT_DIR/simulation-agent.sh" \
        SIMULATOR_CONTROL_TLS_CERT="$CONTROL_SERVER_CERT" \
        SIMULATOR_CONTROL_TLS_KEY="$CONTROL_SERVER_KEY" \
        SIMULATOR_CONTROL_CLIENT_CA="$CONTROL_CA" \
        SIMULATOR_CONTROL_LISTEN=0.0.0.0:8791 \
        "$BIN" -mode simulator -listen 127.0.0.1:8788
    wait_for_url http://127.0.0.1:8788/ simulator
    wait_for_url https://127.0.0.1:8791/internal/ping 'machine-control listener' \
		--cacert "$CONTROL_CA" \
		--cert "$CONTROL_DIR/machine-01.crt" \
		--key "$CONTROL_DIR/machine-01.key"
	start_machine_controllers
    echo "Control Room ready at http://127.0.0.1:8787"
    echo "OpenObserve GUI relay at http://127.0.0.1:$OBSERVABILITY_UI_RELAY_PORT"
    echo "LDAP editor relay at http://127.0.0.1:$LDAP_UI_RELAY_PORT/"
    echo "Simulator ready at http://127.0.0.1:8788"
    echo "Active organization: $organization_id"
    echo "Machine control mTLS listener ready at https://127.0.0.1:8791"
}

status_stack() {
    for entry in "policy-service:$POLICY_PID:8789" "admin-relay:$ADMIN_RELAY_PID:$ADMIN_RELAY_PORT" "ldap-ui-relay:$LDAP_UI_RELAY_PID:$LDAP_UI_RELAY_PORT" "observability-ui-relay:$OBSERVABILITY_UI_RELAY_PID:$OBSERVABILITY_UI_RELAY_PORT" "control-service:$CONTROL_PID:8790" "control-room:$ROOM_PID:8787" "simulator:$SIMULATOR_PID:8788"; do
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
    start-dns) start_dns_service ;;
    stop-dns) stop_dns_service ;;
    start-ui-relays) start_ui_relays ;;
    stop-ui-relays) stop_ui_relays ;;
    restart-control-service)
        stop_service "$CONTROL_PID"
        start_control_service
        ;;
    *) echo "usage: $0 {start|stop|restart|status|start-dns|stop-dns|start-ui-relays|stop-ui-relays|restart-control-service}" >&2; exit 2 ;;
esac