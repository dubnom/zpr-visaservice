#!/bin/sh
set -eu

# This probe exercises an installed demo; it never starts or stops the backbone.
[ "${1:-}" = verify-packaged-demo ] && [ "$#" = 1 ] || {
    echo "usage: verify-packaged-demo.sh verify-packaged-demo" >&2
    exit 2
}
: "${SIMULATION_DEMO_PROJECT:?set the installed Compose project}"
: "${SIMULATION_DEMO_IMAGE:?set the immutable installed ZPR image ID}"
case "$SIMULATION_DEMO_PROJECT" in
    zprdemo-*) ;;
    *) echo "expected an installed zprdemo Compose project" >&2; exit 1 ;;
esac
case "$SIMULATION_DEMO_PROJECT" in
    *[!a-z0-9-]*) echo "invalid Compose project" >&2; exit 1 ;;
esac

fail() { echo "packaged-demo: $*" >&2; exit 1; }
for service in alice web0 vs node0 node1; do
    container="$SIMULATION_DEMO_PROJECT-$service-1"
    actual=$(docker inspect --format \
        '{{.State.Running}} {{.Image}} {{index .Config.Labels "com.docker.compose.project"}} {{index .Config.Labels "com.docker.compose.service"}}' "$container")
    [ "$actual" = "true $SIMULATION_DEMO_IMAGE $SIMULATION_DEMO_PROJECT $service" ] ||
        fail "container ownership, image or running-state mismatch: $container"
done
client="$SIMULATION_DEMO_PROJECT-alice-1"
server="$SIMULATION_DEMO_PROJECT-web0-1"
visa="$SIMULATION_DEMO_PROJECT-vs-1"
allowed='http://[fd5a:5052:8888::8]/'
blocked='http://[fd5a:5052:8888::8]:18080/'
[ "$(docker exec "$visa" redis-cli --raw TYPE policy:current)" = hash ] ||
    fail "active policy is not a Redis hash"
policy_before=$(docker exec "$visa" redis-cli -e --json HGETALL policy:current)
source_address=$(docker exec "$client" ip -6 route get fd5a:5052:8888::8 |
    awk '{for (i=1; i<NF; i++) if ($i == "src") print $(i+1)}')
case "$source_address" in *[!0-9a-fA-F:]*|'') fail "cannot determine Alice source address" ;; esac
log_offset=$(docker exec "$visa" sh -c 'wc -c < /logs/vs.log')
case "$log_offset" in *[!0-9]*|'') fail "invalid Visa log offset" ;; esac
docker exec "$client" curl --noproxy '*' -g -6 -fsS --connect-timeout 5 --max-time 10 "$allowed" >/dev/null
echo "PASS: Alice -> OciWeb TCP 80 returned HTTP success"

probe_dir=
workload_id=
workload_name="$SIMULATION_DEMO_PROJECT-scenario-http"
cleanup() {
    result=$?
    trap - EXIT HUP INT TERM
    if [ -n "$workload_id" ]; then
        if ! docker rm -f "$workload_id" >/dev/null; then
            echo "packaged-demo: failed to remove owned workload $workload_id" >&2
            result=1
        fi
    fi
    if [ -n "$probe_dir" ]; then
        if ! docker exec "$server" rmdir "$probe_dir"; then
            echo "packaged-demo: failed to remove probe directory $probe_dir" >&2
            result=1
        fi
    fi
    if [ "$result" = 0 ]; then
        echo "PASS: Simulator-provisioned workload container removed"
        echo "PASS: temporary listener stopped and probe lock removed"
    fi
    exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM
docker exec "$server" mkdir /tmp/zpr-packaged-demo-probe ||
    fail "another probe is active; inspect /tmp/zpr-packaged-demo-probe"
probe_dir=/tmp/zpr-packaged-demo-probe
if docker exec "$server" awk '$2 ~ /:46A0$/ && $4 == "0A" {found=1} END {exit !found}' /proc/net/tcp6 /proc/net/tcp; then
    fail "TCP 18080 is already in use; existing listener was not touched"
fi
# This workload shares only web0's network namespace, not its filesystem or keys.
# Bound its lifetime even if Simulator is killed before its cleanup trap.
workload_id=$(docker create --pull never --name "$workload_name" \
    --label "zpr.simulator.workload.project=$SIMULATION_DEMO_PROJECT" \
    --label zpr.simulator.workload.role=policy-denial-http \
    --network "container:$server" --user 65534:65534 \
    --read-only --cap-drop ALL --security-opt no-new-privileges:true \
    --pids-limit 16 --memory 32m --cpus 0.25 \
    --entrypoint /usr/bin/timeout "$SIMULATION_DEMO_IMAGE" \
    -k 2s 30s /usr/bin/socat \
    'TCP6-LISTEN:18080,bind=[fd5a:5052:8888::8],ipv6only=1,reuseaddr,fork' TCP:127.0.0.1:80) ||
    fail "cannot create workload; an existing same-named container is never adopted or removed"
docker start "$workload_id" >/dev/null
echo "PASS: Simulator provisioned isolated workload $workload_name (non-root, no mounts, no capabilities)"
attempt=0
until docker exec "$server" curl --noproxy '*' -g -6 -fsS --max-time 2 "$blocked" >/dev/null; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 10 ] || fail "temporary listener is not healthy"
    sleep 1
done
[ "$(docker inspect --format '{{.State.Running}}' "$workload_id")" = true ] ||
    fail "provisioned workload exited before its traffic check"
echo "PASS: temporary TCP 18080 listener independently serves HTTP"
set +e
docker exec "$client" curl --noproxy '*' -g -6 -fsS --connect-timeout 3 --max-time 5 "$blocked" >/dev/null
request_status=$?
set -e
[ "$request_status" = 28 ] || fail "expected timeout on blocked port, got curl exit $request_status"
# A timeout alone is not evidence of a policy denial. Require a fresh Visa decision.
docker exec "$visa" sh -c 'tail -c "+$1" /logs/vs.log' sh "$((log_offset + 1))" |
    grep -F "eval denied (no match) for flow $source_address -> fd5a:5052:8888::8 " |
    grep -F 'dest_port: 18080 }' |
    grep -F 'no matching policy' ||
    fail "blocked request has no matching fresh Visa denial"
docker exec "$client" curl --noproxy '*' -g -6 -fsS --connect-timeout 5 --max-time 10 "$allowed" >/dev/null
[ "$policy_before" = "$(docker exec "$visa" redis-cli -e --json HGETALL policy:current)" ] ||
    fail "active policy changed during probe"
echo "PASS: blocked request corroborated by Visa; allowed HTTP still works; active policy unchanged"
