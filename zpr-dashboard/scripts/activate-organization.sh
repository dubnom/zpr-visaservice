#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
dashboard_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
runtime_dir=$(CDPATH= cd -- "$dashboard_dir/../../.local-runtime" && pwd)
rig=${SIMULATION_CONTAINER:-zpr-local-linux-node}
organization=${2:-}
case "${1:-}:$organization" in
    reset-organization:*) ;;
    *) echo "usage: $0 reset-organization organization-id" >&2; exit 2 ;;
esac
case "$organization" in ''|*[!a-z0-9-]*) echo "invalid organization id" >&2; exit 2 ;; esac
profile="$dashboard_dir/cmd/zpr-web-dashboard/examples/organizations/$organization.json"
[ -r "$profile" ] || { echo "organization profile missing" >&2; exit 1; }
binary="$runtime_dir/dashboard-stack/zpr-web-dashboard"
compiler=${ZPR_ZPLC_BIN:-$dashboard_dir/../../zpr-compiler/target/debug/zplc}
pregen="$runtime_dir/linux-integration/pregen"
bundle_dir="$runtime_dir/organization-policy/$organization"
request_file="$runtime_dir/organization-request.json"
umask 077
mkdir -p "$bundle_dir"
exec >>"$runtime_dir/dashboard-stack/organization-reset.log" 2>&1
printf 'Preflight organization %s\n' "$organization"
"$binary" -mode compose-policy -policy-root "$dashboard_dir/cmd/zpr-web-dashboard/examples" -policy-organization "$organization" -policy-output "$bundle_dir/runtime.zpl"
"$compiler" "$bundle_dir/runtime.zpl" -c "$pregen/v6-1node-3actor-ping.zplc" -k "$pregen/zpr-rsa-key.pem" -d "$bundle_dir" -o runtime.bin2
docker inspect "$rig" >/dev/null
[ -r "$runtime_dir/build-and-run-linux-node.sh" ] || { echo "rig launcher unavailable"; exit 1; }
generation=$(openssl rand -hex 16)
backup=$(mktemp "$runtime_dir/organization-request-backup.XXXXXX")
had_request=no
if [ -f "$request_file" ]; then cp "$request_file" "$backup"; had_request=yes; fi
changed=no
policy_changed=no
previous_organization=$(curl -fsS --max-time 5 http://127.0.0.1:8788/api/simulator/organizations | jq -er '.active_id')
rollback() {
    result=$?
    trap - EXIT HUP INT TERM
    if [ "$result" -ne 0 ] && [ "$changed" = yes ]; then
        echo "Activation failed; restoring previous rig context"
        if [ "$had_request" = yes ]; then cp "$backup" "$request_file"; else rm -f "$request_file"; fi
        docker restart "$rig" >/dev/null || true
        if [ "$policy_changed" = yes ]; then
            previous_source="$runtime_dir/organization-policy/$previous_organization/runtime.zpl"
            if [ ! -r "$previous_source" ]; then previous_source="$pregen/v4-1node-3actor-ping.zpl"; fi
            "$script_dir/dashboard-stack.sh" restart-policy-context "$previous_organization" "$previous_source" || true
        fi
    fi
    rm -f "$backup"
    exit "$result"
}
trap rollback EXIT
trap 'exit 1' HUP INT TERM
request_temp=$(mktemp "$runtime_dir/organization-request.XXXXXX")
jq -n --arg organization_id "$organization" --arg generation "$generation" --arg policy_source "/work/.local-runtime/organization-policy/$organization/runtime.zpl" '{organization_id:$organization_id,generation:$generation,policy_source:$policy_source}' > "$request_temp"
mv "$request_temp" "$request_file"
changed=yes
for machine in $(jq -r '.machines[].id' "${SIMULATION_MANIFEST:-$runtime_dir/simulation-environment.json}"); do
    state=$(docker inspect -f '{{.State.Running}}' "zpr-$machine" 2>/dev/null || true)
    if [ "$state" = true ]; then docker stop --time 5 "zpr-$machine" >/dev/null; fi
done
docker restart "$rig" >/dev/null
echo "Waiting for company LDAP and ZPR bootstrap"
attempt=0
while [ "$attempt" -lt 600 ]; do
    if [ -r "$runtime_dir/organization-runtime.json" ] && jq -e --arg id "$organization" --arg generation "$generation" '.organization_id==$id and .generation==$generation' "$runtime_dir/organization-runtime.json" >/dev/null &&
       curl -fsS --connect-timeout 1 --max-time 5 http://127.0.0.1:8787/api/snapshot | jq -e '(.errors|length)==0 and any(.actors[]; .node and .node_details.in_sync) and any(.actors[]; .cn=="adapter1")' >/dev/null; then break; fi
    attempt=$((attempt+1))
    sleep 1
done
[ "$attempt" -lt 600 ] || { echo "company bootstrap timed out"; exit 1; }
"$script_dir/dashboard-stack.sh" start-dns
"$script_dir/dashboard-stack.sh" restart-simulator-control
if [ -r "$runtime_dir/observability/restore-adapters.sh" ]; then
    rig_directory=$(docker exec "$rig" sh -c 'pid=$(pgrep -x vs | head -1); readlink "/proc/$pid/cwd"')
    docker exec "$rig" bash /work/.local-runtime/observability/restore-adapters.sh "$rig_directory"
    if docker inspect zpr-observability >/dev/null 2>&1; then docker start zpr-observability >/dev/null; fi
    sh "$dashboard_dir/../observability/openobserve/collector.sh" stop
    sh "$dashboard_dir/../observability/openobserve/collector.sh" start
fi
policy_changed=yes
"$script_dir/dashboard-stack.sh" restart-policy-context "$organization" "$bundle_dir/runtime.zpl"
curl -fsS --max-time 10 http://127.0.0.1:8787/api/policy/context | jq -e --arg id "$organization" '.organization_id==$id' >/dev/null
echo "Organization policy, directory, and ZPR context ready"