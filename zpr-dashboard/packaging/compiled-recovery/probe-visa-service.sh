#!/bin/sh
set -eu

fail() { echo "probe-visa-service: $*" >&2; exit 1; }
if [ "$#" -ne 3 ] && { [ "$#" -ne 4 ] || [ "$4" != --node ]; }; then
    echo "Usage: sh probe-visa-service.sh /absolute/compiled-bin /absolute/prepare-visa-service.sh /protected/new-evidence [--node]" >&2
    exit 2
fi
[ "$(uname -s)/$(uname -m)" = Linux/aarch64 ] || fail "native Linux ARM64 required"
for tool in redis-server redis-cli curl openssl timeout ip sha256sum cmp; do
    command -v "$tool" >/dev/null 2>&1 || fail "missing test prerequisite: $tool"
done
# A caller-created network namespace must have no non-loopback interfaces.
[ "$(ip -o link show | wc -l)" -eq 1 ] || fail "run in a new isolated network namespace with only loopback"
ip link show lo | grep -q '<LOOPBACK,UP' || fail "isolated loopback must be up"
bin=$1
prepare=$2
evidence=$3
node_mode=${4:-}
if [ "$node_mode" = --node ]; then
    [ "$(id -u)" -eq 0 ] || fail "node probe requires root only inside the caller-created isolated namespace"
    [ -c /dev/net/tun ] || fail "node probe requires Linux TUN"
    command -v jq >/dev/null 2>&1 || fail "node probe requires jq"
    for tool in nsenter unshare; do command -v "$tool" >/dev/null 2>&1 || fail "node probe requires $tool"; done
fi
case "$evidence" in /*) ;; *) fail "absolute evidence path required" ;; esac
umask 077
mkdir -- "$evidence"
redis_pid=
visa_pid=
node_pid=
node_namespace_pid=
cleanup() {
    if [ -n "$node_pid" ]; then
        kill "$node_pid" 2>/dev/null || :
        wait "$node_pid" 2>/dev/null || :
    fi
    if [ -n "$visa_pid" ]; then
        kill "$visa_pid" 2>/dev/null || :
        wait "$visa_pid" 2>/dev/null || :
    fi
    if [ -n "$redis_pid" ]; then
        kill "$redis_pid" 2>/dev/null || :
        wait "$redis_pid" 2>/dev/null || :
    fi
    if [ -n "$node_namespace_pid" ]; then
        kill "$node_namespace_pid" 2>/dev/null || :
        wait "$node_namespace_pid" 2>/dev/null || :
    fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
node_net() { nsenter -t "$node_namespace_pid" -n -- "$@"; }
if [ "$node_mode" = --node ]; then
    unshare --net sleep 600 &
    node_namespace_pid=$!
    attempt=0
    while [ "$(readlink "/proc/$node_namespace_pid/ns/net")" = "$(readlink /proc/self/ns/net)" ]; do
        kill -0 "$node_namespace_pid" || fail "node namespace process exited"
        [ "$attempt" -lt 50 ] || fail "node namespace creation timed out"
        attempt=$((attempt + 1))
        sleep 0.1
    done
    ip link add zprvisa0 type veth peer name zprnode0
    ip link set zprnode0 netns "$node_namespace_pid"
    ip link set zprvisa0 up
    ip -6 addr add fd00:5a::1/126 dev zprvisa0 nodad
    ip -6 addr add fd5a:5052::1/128 dev lo nodad
    node_net ip link set lo up
    node_net ip link set zprnode0 up
    node_net ip -6 addr add fd00:5a::2/126 dev zprnode0 nodad
    node_net ip tuntap add name zprtest0 mode tun multi_queue
    node_net ip link set zprtest0 mtu 1400 up
    node_net ip -6 addr add fd5a:5052::2/128 dev zprtest0 nodad
    ip -6 route add fd5a:5052::2/128 via fd00:5a::2 dev zprvisa0 src fd5a:5052::1
    node_net ip -6 route add fd5a:5052::1/128 via fd00:5a::1 dev zprnode0 src fd5a:5052::2
    ip -6 route get fd5a:5052::2 > "$evidence/visa-route.txt"
    node_net ip -6 route get fd5a:5052::1 > "$evidence/node-route.txt"
fi
unset SIMULATION_MANIFEST SIMULATOR_URL ZPR_SIMULATOR_URL SIMULATOR_SERVICE_URL VS_API_KEY
export HTTP_PROXY= HTTPS_PROXY= ALL_PROXY= http_proxy= https_proxy= all_proxy= NO_PROXY='*'
for name in vs vs-admin vsapikey zplc; do sha256sum "$bin/$name" >> "$evidence/binaries.sha256"; done
prepare_state() {
    if [ "$node_mode" = --node ]; then sh "$prepare" "$bin" "$1" --node
    else sh "$prepare" "$bin" "$1"; fi
}
prepare_state "$evidence/first" > "$evidence/prepare.txt"
prepare_state "$evidence/second" > "$evidence/prepare-second.txt"
if cmp -s "$evidence/first/administrator.key" "$evidence/second/administrator.key" ||
   cmp -s "$evidence/first/authority.pem" "$evidence/second/authority.pem" ||
   cmp -s "$evidence/first/visa-bootstrap.json" "$evidence/second/visa-bootstrap.json"; then
    fail "independent bootstrap reused identity or trust"
fi
state="$evidence/first"
sha256sum "$state/administrator.key" "$state/vs_keys.toml" > "$evidence/before-refusal.sha256"
if sh "$prepare" "$bin" "$state" > "$evidence/refusal.txt" 2>&1; then
    fail "bootstrap overwrote existing state"
fi
grep -q 'nothing was overwritten' "$evidence/refusal.txt" || fail "unexpected refusal reason"
sha256sum -c "$evidence/before-refusal.sha256" > "$evidence/nonoverwrite.txt"
[ -z "$(find "$evidence" -type f \( -perm -004 -o -perm -040 \) -print)" ] ||
    fail "generated files are readable outside the owner"
[ -z "$(find "$evidence" -type d \( -perm -007 -o -perm -070 \) -print)" ] ||
    fail "generated directories are accessible outside the owner"
start_redis() {
    redis-server --port 0 --unixsocket "$state/database/redis.sock" --unixsocketperm 700 \
        --dir "$state/database" --save '' --appendonly yes --appendfsync always >> "$evidence/redis.log" 2>&1 &
    redis_pid=$!
    attempt=0
    until [ "$(redis-cli -s "$state/database/redis.sock" --raw PING 2>/dev/null)" = PONG ]; do
        kill -0 "$redis_pid" || fail "private Redis process exited"
        [ "$attempt" -lt 50 ] || fail "private Redis readiness timed out"
        attempt=$((attempt + 1))
        sleep 0.2
    done
}
start_visa() {
    "$bin/vs" --config "$state/vs.toml" "$@" >> "$evidence/visa.log" 2>&1 &
    visa_pid=$!
    attempt=0
    until code=$(curl --silent --show-error --noproxy '*' --connect-timeout 1 --max-time 2 \
        --cacert "$state/authority.pem" -o /dev/null -w '%{http_code}' \
        'https://[::1]:8182/admin/policies' 2>/dev/null) && [ "$code" = 401 ]; do
        kill -0 "$visa_pid" || fail "Visa exited; inspect private visa.log"
        [ "$attempt" -lt 100 ] || fail "Visa HTTPS readiness timed out"
        attempt=$((attempt + 1))
        sleep 0.2
    done
}
admin() {
    timeout 20 "$bin/vs-admin" --svc-url 'https://[::1]:8182' \
        --ca-cert "$state/authority.pem" --api-key-file "$state/administrator.key" \
        --format compact "$@"
}
request() {
    curl --silent --show-error --noproxy '*' --connect-timeout 1 --max-time 5 \
        --cacert "$state/authority.pem" "$@"
}
start_redis
start_visa "$state/bootstrap.bin2"
admin network > "$evidence/network.json"
admin actors > "$evidence/actors.json"
admin policies --curr > "$evidence/policy.json"
reject_cli() {
    label=$1
    shift
    if timeout 20 "$bin/vs-admin" "$@" --format compact policies --curr \
        > "$evidence/$label-output.txt" 2> "$evidence/$label-error.txt"; then
        fail "CLI accepted $label"
    else
        [ "$?" -eq 1 ] || fail "CLI $label did not report request failure with exit 1"
    fi
    [ ! -s "$evidence/$label-output.txt" ] || fail "CLI $label produced success-shaped output"
}
reject_cli foreign-ca --svc-url 'https://[::1]:8182' \
    --ca-cert "$evidence/second/authority.pem" --api-key-file "$state/administrator.key"
reject_cli hostname-mismatch --svc-url 'https://localhost:8182' \
    --ca-cert "$state/authority.pem" --api-key-file "$state/administrator.key"
reject_cli plaintext --svc-url 'http://[::1]:8182' \
    --ca-cert "$state/authority.pem" --api-key-file "$state/administrator.key"
reject_cli foreign-api-key --svc-url 'https://[::1]:8182' \
    --ca-cert "$state/authority.pem" --api-key-file "$evidence/second/administrator.key"
printf 'X-API-Key: %s\n' "$(cat "$state/administrator.key")" > "$evidence/administrator-header"
request --fail -H "@$evidence/administrator-header" \
    'https://[::1]:8182/admin/network' > "$evidence/verified-tls-network.json"
printf '%s\n' "$(cat "$evidence/verified-tls-network.json")" | cmp - "$evidence/network.json" ||
    fail "CLI network output differs from verified HTTPS response"
verified_policy=$(request --fail -H "@$evidence/administrator-header" \
    'https://[::1]:8182/admin/policies/curr')
printf '%s\n' "$verified_policy" > "$evidence/verified-tls-policy.json"
cmp "$evidence/verified-tls-policy.json" "$evidence/policy.json" ||
    fail "CLI current policy differs from verified HTTPS response"
if curl --silent --show-error --noproxy '*' --max-time 5 \
    --cacert "$evidence/second/authority.pem" 'https://[::1]:8182/admin/network' \
    > "$evidence/foreign-ca.txt" 2> "$evidence/foreign-ca-error.txt"; then
    fail "foreign CA incorrectly trusted"
else
    [ "$?" -eq 60 ] || fail "foreign CA test did not fail certificate verification"
fi
timeout 20 "$bin/vs-admin" --svc-url 'https://[::1]:8182' \
    --ca-cert "$state/authority.pem" --api-key-file "$state/observer.key" \
    --format compact policies --curr > "$evidence/observer-policy.json"
cmp "$evidence/policy.json" "$evidence/observer-policy.json" ||
    fail "observer could not read current policy"
printf 'X-API-Key: %s\n' "$(cat "$state/observer.key")" > "$evidence/observer-header"
code=$(request -H "@$evidence/observer-header" -H 'Content-Type: application/json' \
    -X POST --data-binary "@$evidence/policy.json" \
    -o "$evidence/observer-write.txt" -w '%{http_code}' 'https://[::1]:8182/admin/policies')
[ "$code" = 403 ] || fail "observer policy write was not forbidden (HTTP $code)"
printf 'X-API-Key: %s\n' "$(cat "$evidence/second/administrator.key")" > "$evidence/foreign-header"
code=$(request -H "@$evidence/foreign-header" -o "$evidence/foreign-read.txt" -w '%{http_code}' \
    'https://[::1]:8182/admin/policies')
[ "$code" = 401 ] || fail "other installation credential was not rejected (HTTP $code)"
admin policies --path "$state/bootstrap.bin2" > "$evidence/policy-install.json"
grep -qx '{"id":0}' "$evidence/policy-install.json" || fail "policy installation receipt missing"
request --fail -H "@$evidence/administrator-header" -H 'Content-Type: application/json' \
    --data-binary "@$evidence/policy.json" 'https://[::1]:8182/admin/policies' > "$evidence/verified-tls-install.json"
kill "$visa_pid"
wait "$visa_pid"
visa_pid=
kill "$redis_pid"
wait "$redis_pid"
redis_pid=
start_redis
start_visa
admin policies --curr > "$evidence/restarted-policy.json"
admin network > "$evidence/restarted-network.json"
cmp "$evidence/policy.json" "$evidence/restarted-policy.json" || fail "policy did not persist across restart"
cmp "$evidence/network.json" "$evidence/restarted-network.json" || fail "network changed across restart"
if [ "$node_mode" = --node ]; then
    sha256sum "$bin/ph" "$bin/ph-cli" >> "$evidence/binaries.sha256"
    sed "s|^auth_private_key = .*|auth_private_key = \"$evidence/second/node.key\"|" \
        "$state/node.toml" > "$evidence/foreign-node.toml"
    nsenter -t "$node_namespace_pid" -n -- "$bin/ph" node -c "$evidence/foreign-node.toml" \
        --logging all=INFO > "$evidence/foreign-node.log" 2>&1 &
    node_pid=$!
    attempt=0
    until grep -q 'blob signature verification failed for cn node' "$evidence/visa.log"; do
        kill -0 "$node_pid" || fail "foreign-key node exited before exercising authentication"
        [ "$attempt" -lt 100 ] || fail "foreign-key node did not receive explicit rejection"
        attempt=$((attempt + 1))
        sleep 0.2
    done
    admin actors --nodes > "$evidence/rejected-nodes.json"
    jq -e 'type == "array" and length == 0' "$evidence/rejected-nodes.json" >/dev/null ||
        fail "foreign bootstrap key was admitted"
    kill "$node_pid"
    wait "$node_pid" || :
    node_pid=
    nsenter -t "$node_namespace_pid" -n -- "$bin/ph" node -c "$state/node.toml" --logging all=INFO > "$evidence/node.log" 2>&1 &
    node_pid=$!
    attempt=0
    until admin actors --cn node > "$evidence/node.json" 2> "$evidence/node-query.log" &&
        jq -e '.cn == "node" and .node == true and .zpr_addr == "fd5a:5052::2"
            and .node_details.in_sync == true and .node_details.pending_install == 0' \
            "$evidence/node.json" >/dev/null; do
        kill -0 "$node_pid" || fail "node exited before synchronizing"
        [ "$attempt" -lt 150 ] || fail "node did not reach authenticated synchronized state"
        attempt=$((attempt + 1))
        sleep 0.2
    done
    grep -q 'node access granted to visa service' "$evidence/node.log" ||
        fail "node grant receipt missing"
    grep -q 'registered VSS' "$evidence/node.log" || fail "VSS registration receipt missing"
    admin actors --nodes > "$evidence/accepted-nodes.json"
    jq -e 'length == 1 and .[0].cn == "node"' "$evidence/accepted-nodes.json" >/dev/null ||
        fail "unexpected admitted node inventory"
    jq '{cn, node, zpr_addr, node_details: {in_sync: .node_details.in_sync,
        pending_install: .node_details.pending_install, connect_requests: .node_details.connect_requests,
        vss_port: .node_details.vss_port}}' "$evidence/node.json" > "$evidence/node-summary.json"
    grep 'blob signature verification failed for cn node' "$evidence/visa.log" > "$evidence/node-rejection.txt"
    grep -E 'node access granted to visa service|registered VSS' "$evidence/node.log" > "$evidence/node-grant.txt"
    printf '%s\n' 'PASS: foreign RSA bootstrap key rejected; fresh real PH node authenticated, registered VSS and synchronized with Visa.' \
        'Namespace-only TUN and private veth substrate; no peer, workload or service adapter tested.' > "$evidence/node-result.txt"
    cat "$evidence/node-result.txt"
fi
printf '%s\n' 'PASS: two fresh bootstraps, non-overwrite, owner-only state, verified HTTPS, anonymous/foreign rejection, observer read-only, policy installation and database restart.' \
    'CLI rejects foreign CA, hostname mismatch, plaintext and foreign API key with exit 1 and empty stdout.' \
    'Simulator unavailable; no external network, demo workload or production deployment configured. A node is exercised only with explicit --node.' \
    'Private evidence contains generated credentials and must never be published.' > "$evidence/result.txt"
cat "$evidence/result.txt"
