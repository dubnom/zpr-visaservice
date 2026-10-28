#!/bin/sh
set -eu

[ "${1:-}" = verify-packaged-identity ] && [ "$#" = 1 ] || {
    echo "usage: verify-packaged-identity.sh verify-packaged-identity" >&2
    exit 2
}
: "${SIMULATION_DEMO_PROJECT:?set the installed Compose project}"
: "${SIMULATION_DEMO_IMAGE:?set the immutable installed ZPR image ID}"
fail() { echo "packaged-identity: $*" >&2; exit 1; }
case "$SIMULATION_DEMO_PROJECT" in zprdemo-*) ;; *) fail "expected installed Compose project" ;; esac
case "$SIMULATION_DEMO_PROJECT" in *[!a-z0-9-]*) fail "invalid Compose project" ;; esac
for service in alice vs node1; do
    container="$SIMULATION_DEMO_PROJECT-$service-1"
    actual=$(docker inspect --format \
        '{{.State.Running}}|{{.Image}}|{{index .Config.Labels "com.docker.compose.project"}}|{{index .Config.Labels "com.docker.compose.service"}}' "$container")
    [ "$actual" = "true|$SIMULATION_DEMO_IMAGE|$SIMULATION_DEMO_PROJECT|$service" ] ||
        fail "container ownership, image or running-state mismatch: $container"
done
node="$SIMULATION_DEMO_PROJECT-node1-1"
visa="$SIMULATION_DEMO_PROJECT-vs-1"
client="$SIMULATION_DEMO_PROJECT-alice-1"
network="${SIMULATION_DEMO_PROJECT}_zpr-demo"
[ "$(docker network inspect --format '{{index .Labels "com.docker.compose.project"}}' "$network")" = "$SIMULATION_DEMO_PROJECT" ] ||
    fail "substrate network ownership mismatch"
node_ip=$(docker inspect --format "{{(index .NetworkSettings.Networks \"$network\").IPAddress}}" "$node")
case "$node_ip" in ""|*[!0-9.]*) fail "cannot determine node1 address on $network" ;; esac
policy_before=$(docker exec "$visa" redis-cli -e --json HGETALL policy:current)
log_offset=$(docker exec "$visa" sh -c 'wc -c < /logs/vs.log')
case "$log_offset" in *[!0-9]*|'') fail "invalid Visa log offset" ;; esac
identity="simulator-unapproved-$(docker exec "$visa" openssl rand -hex 12)"
public_ca=$(docker exec "$node" cat /conf/include/auth-ca.crt)
name="$SIMULATION_DEMO_PROJECT-scenario-identity"
candidate=
cleanup() {
    result=$?
    trap - EXIT HUP INT TERM
    if [ -n "$candidate" ]; then
        if ! docker rm -f "$candidate" >/dev/null; then
            echo "packaged-identity: failed to remove owned adapter $candidate" >&2
            result=1
        fi
    fi
    [ "$result" != 0 ] || echo "PASS: independent identity container and private key removed"
    exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM
candidate=$(docker create --pull never --name "$name" \
    --label "zpr.simulator.workload.project=$SIMULATION_DEMO_PROJECT" \
    --label zpr.simulator.workload.role=unapproved-identity \
    --label "zpr.simulator.workload.identity=$identity" \
    --network "$network" --read-only --cap-drop ALL --cap-add NET_ADMIN \
    --device /dev/net/tun --security-opt no-new-privileges:true \
    --pids-limit 32 --memory 96m --cpus 0.5 \
    --tmpfs /run/zpr:rw,noexec,nosuid,size=4m,mode=0700 \
    --env "PUBLIC_CA=$public_ca" --env "NODE_ADDR=$node_ip:5000" \
    --entrypoint /usr/bin/timeout "$SIMULATION_DEMO_IMAGE" \
    -k 2s 30s /bin/sh -ec '
        umask 077
        printf "%s\n" "$PUBLIC_CA" > /run/zpr/public-ca.crt
        openssl genrsa -out /run/zpr/bootstrap.key 2048
        openssl pkey -in /run/zpr/bootstrap.key -pubout -out /run/zpr/bootstrap.pub
        echo "Generated independent bootstrap key"
        exec /app/bin/ph adapter --logging all=INFO \
            --control-path /run/zpr/control.sock --capture-path /run/zpr/capture.sock \
            --ca-file /run/zpr/public-ca.crt --bootstrap-key /run/zpr/bootstrap.key \
            --name "$1" --node-addr "$NODE_ADDR" --tun-if tun0
    ' identity-probe "$identity") ||
    fail "cannot create adapter; an existing same-named container is never adopted or removed"
docker start "$candidate" >/dev/null
echo "PASS: Simulator provisioned independent unapproved identity $identity"
attempt=0
while :; do
    evidence=$(docker exec "$visa" sh -c 'tail -c "+$1" /logs/vs.log' sh "$((log_offset + 1))")
    if printf '%s\n' "$evidence" | grep -F "no key found in policy for cn $identity"; then
        break
    fi
    [ "$(docker inspect --format '{{.State.Running}}' "$candidate")" = true ] ||
        fail "adapter exited without explicit Visa admission rejection"
    attempt=$((attempt + 1))
    [ "$attempt" -lt 20 ] || fail "no explicit Visa rejection for generated identity"
    sleep 1
done
[ "$policy_before" = "$(docker exec "$visa" redis-cli -e --json HGETALL policy:current)" ] ||
    fail "active policy changed during identity check"
docker exec "$client" curl --noproxy '*' -g -6 -fsS --connect-timeout 5 --max-time 10 \
    'http://[fd5a:5052:8888::8]/' >/dev/null
echo "PASS: unapproved identity explicitly rejected; existing Alice HTTP works; active policy unchanged"
