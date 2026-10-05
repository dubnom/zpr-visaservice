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
organization_driver=$(jq -er '.runtime.driver' "$profile")
organization_base_dn=$(jq -er '.directory.base_dn' "$profile")
organization_bind_dn="cn=zpr-reader,ou=Service Accounts,$organization_base_dn"
binary="$runtime_dir/dashboard-stack/zpr-web-dashboard"
compiler=${ZPR_ZPLC_BIN:-$dashboard_dir/../../zpr-compiler/target/debug/zplc}
pregen="$runtime_dir/linux-integration/pregen"
multinode_dir="$dashboard_dir/../../zpr-demo/multinode-demo"
multinode_deploy="$multinode_dir/local-compute/deploy-docker.sh"
bundle_dir="$runtime_dir/organization-policy/$organization"
request_file="$runtime_dir/organization-request.json"
status_file=${SIMULATION_ACTIVATION_STATUS_FILE:-$runtime_dir/dashboard-stack/active-organization.txt.progress}
umask 077
mkdir -p "$bundle_dir"
exec >>"$runtime_dir/dashboard-stack/organization-reset.log" 2>&1
report_activation_status() {
    status_temp="$status_file.tmp.$$"
    printf '%s\n' "$1" >"$status_temp"
    mv "$status_temp" "$status_file"
}
report_activation_status "Preparing runtime policy"
printf 'Preflight organization %s\n' "$organization"
"$binary" -mode compose-policy -policy-root "$dashboard_dir/cmd/zpr-web-dashboard/examples" -policy-organization "$organization" -policy-output "$bundle_dir/runtime.zpl"
"$compiler" "$bundle_dir/runtime.zpl" -c "$pregen/v6-1node-3actor-ping.zplc" -k "$pregen/zpr-rsa-key.pem" -d "$bundle_dir" -o runtime.bin2
report_activation_status "Runtime policy compiled"
docker inspect "$rig" >/dev/null
[ -r "$runtime_dir/build-and-run-linux-node.sh" ] || { echo "rig launcher unavailable"; exit 1; }
generation=$(openssl rand -hex 16)
backup=$(mktemp "$runtime_dir/organization-request-backup.XXXXXX")
had_request=no
if [ -f "$request_file" ]; then cp "$request_file" "$backup"; had_request=yes; fi
changed=no
policy_changed=no
control_changed=no
previous_organization=$(curl -fsS --max-time 5 http://127.0.0.1:8788/api/simulator/organizations | jq -er '.active_id')
previous_profile="$dashboard_dir/cmd/zpr-web-dashboard/examples/organizations/$previous_organization.json"
previous_driver=$(jq -er '.runtime.driver' "$previous_profile")
previous_base_dn=$(jq -er '.directory.base_dn' "$previous_profile")
previous_bind_dn="cn=zpr-reader,ou=Service Accounts,$previous_base_dn"
multinode_runtime_dir() { printf '%s/multinode/%s' "$runtime_dir" "$1"; }
stop_linux_one_node_runtime() {
    for machine in $(jq -r '.machines[].id' "${SIMULATION_MANIFEST:-$runtime_dir/simulation-environment.json}"); do
        if [ "$(docker inspect -f '{{.State.Running}}' "zpr-$machine" 2>/dev/null || true)" = true ]; then docker stop --time 5 "zpr-$machine" >/dev/null; fi
    done
    "$script_dir/dashboard-stack.sh" stop-admin-relay || true
    "$script_dir/dashboard-stack.sh" stop-dns || true
    "$script_dir/dashboard-stack.sh" stop-ui-relays || true
    if [ "$(docker inspect -f '{{.State.Running}}' zpr-observability 2>/dev/null || true)" = true ]; then docker stop zpr-observability >/dev/null; fi
    if [ "$(docker inspect -f '{{.State.Running}}' "$rig" 2>/dev/null || true)" = true ]; then docker stop "$rig" >/dev/null; fi
}
stop_multinode_runtimes() {
    for candidate_profile in "$dashboard_dir/cmd/zpr-web-dashboard/examples/organizations/"*.json; do
        [ -r "$candidate_profile" ] || continue
        multinode_profile_id=${candidate_profile##*/}
        multinode_profile_id=${multinode_profile_id%.json}
        if [ "$(jq -er '.runtime.driver' "$candidate_profile")" = docker-multinode ]; then
            ZPR_ORGANIZATION_ID="$multinode_profile_id" ZPR_RUNTIME_ROOT="$(multinode_runtime_dir "$multinode_profile_id")" bash "$multinode_deploy" stop
        fi
    done
    ZPR_ORGANIZATION_ID=multinode-demo bash "$multinode_deploy" stop
}
stop_runtime() {
    runtime_id=$1
    runtime_profile="$dashboard_dir/cmd/zpr-web-dashboard/examples/organizations/$runtime_id.json"
    runtime_kind=$(jq -er '.runtime.driver' "$runtime_profile")
    case "$runtime_kind" in
        linux-one-node)
            stop_linux_one_node_runtime
            ;;
        docker-multinode)
            ZPR_ORGANIZATION_ID="$runtime_id" ZPR_RUNTIME_ROOT="$(multinode_runtime_dir "$runtime_id")" bash "$multinode_deploy" stop
            ;;
        multinode-demo)
            ZPR_ORGANIZATION_ID=multinode-demo bash "$multinode_deploy" stop
            ;;
        *) echo "unsupported runtime driver: $runtime_kind" >&2; return 1 ;;
    esac
}
start_runtime() {
    runtime_id=$1
    runtime_driver=$(jq -er '.runtime.driver' "$dashboard_dir/cmd/zpr-web-dashboard/examples/organizations/$runtime_id.json")
    case "$runtime_driver" in
        linux-one-node)
            stop_multinode_runtimes
            if [ "$(docker inspect -f '{{.State.Running}}' "$rig" 2>/dev/null || true)" = true ]; then docker restart "$rig" >/dev/null; else docker start "$rig" >/dev/null; fi
            ;;
        docker-multinode)
            stop_multinode_runtimes
            stop_linux_one_node_runtime
            ZPR_ORGANIZATION_ID="$runtime_id" ZPR_RUNTIME_ROOT="$(multinode_runtime_dir "$runtime_id")" bash "$multinode_deploy" deploy
            ;;
        *) echo "unsupported runtime driver: $runtime_driver" >&2; return 1 ;;
    esac
}
rollback() {
    result=$?
    trap - EXIT HUP INT TERM
    if [ "$result" -ne 0 ] && [ "$changed" = yes ]; then
        echo "Activation failed; restoring previous rig context"
        stop_runtime "$organization" || true
        if [ "$had_request" = yes ]; then cp "$backup" "$request_file"; else rm -f "$request_file"; fi
        start_runtime "$previous_organization" || true
        if [ "$policy_changed" = yes ]; then
            previous_source="$runtime_dir/organization-policy/$previous_organization/runtime.zpl"
            if [ ! -r "$previous_source" ]; then previous_source="$pregen/v4-1node-3actor-ping.zpl"; fi
            "$script_dir/dashboard-stack.sh" restart-policy-context "$previous_organization" "$previous_source" || true
        fi
        if [ "$control_changed" = yes ]; then
            if [ "$previous_driver" = docker-multinode ]; then
                previous_runtime=$(multinode_runtime_dir "$previous_organization")
                SIMULATION_ORGANIZATION_ID="$previous_organization" \
                ZPR_ASSERTION_LDAP_CONTAINER="${previous_organization}-directory" \
                ZPR_ASSERTION_LDAP_BASE_DN="$previous_base_dn" \
                ZPR_ASSERTION_LDAP_BIND_DN="$previous_bind_dn" \
                ZPR_ADMIN_URL="https://127.0.0.1:8185" \
                ZPR_ADMIN_CA_FILE="$multinode_dir/zpr-conf/include/admin-tls-cert.pem" \
                ZPR_ADMIN_KEY_FILE="$previous_runtime/bob/web-monitor.key" \
                    "$script_dir/dashboard-stack.sh" restart-control-service || true
            else
                SIMULATION_ORGANIZATION_ID="$previous_organization" \
                ZPR_ASSERTION_LDAP_BASE_DN="$previous_base_dn" \
                ZPR_ASSERTION_LDAP_BIND_DN="$previous_bind_dn" \
                    "$script_dir/dashboard-stack.sh" restart-control-service || true
            fi
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
report_activation_status "Stopping previous runtime"
stop_runtime "$previous_organization"
report_activation_status "Starting $organization runtime"
start_runtime "$organization"
if [ "$organization_driver" = linux-one-node ]; then
    report_activation_status "Starting DNS and platform relays"
    "$script_dir/dashboard-stack.sh" start-admin-relay
    "$script_dir/dashboard-stack.sh" start-dns
    "$script_dir/dashboard-stack.sh" start-ui-relays
    control_changed=yes
    SIMULATION_ORGANIZATION_ID="$organization" \
    ZPR_ASSERTION_LDAP_BASE_DN="$organization_base_dn" \
    ZPR_ASSERTION_LDAP_BIND_DN="$organization_bind_dn" \
        "$script_dir/dashboard-stack.sh" restart-control-service
    report_activation_status "Waiting for ZPR node and company LDAP"
    echo "Waiting for company LDAP and one-node ZPR bootstrap"
    attempt=0
    while [ "$attempt" -lt 600 ]; do
        if [ -r "$runtime_dir/organization-runtime.json" ] && jq -e --arg id "$organization" --arg generation "$generation" '.organization_id==$id and .generation==$generation' "$runtime_dir/organization-runtime.json" >/dev/null &&
           curl -fsS --connect-timeout 1 --max-time 5 http://127.0.0.1:8787/api/snapshot | jq -e '(.errors|length)==0 and any(.actors[]; .node and .node_details.in_sync) and any(.actors[]; .cn=="adapter1")' >/dev/null; then break; fi
        attempt=$((attempt+1))
        sleep 1
    done
    [ "$attempt" -lt 600 ] || { echo "company bootstrap timed out"; exit 1; }
    report_activation_status "Restoring observability adapters"
    "$script_dir/dashboard-stack.sh" restart-simulator-control
    if [ -r "$runtime_dir/observability/restore-adapters.sh" ]; then
        rig_directory=$(docker exec "$rig" sh -c 'pid=$(pgrep -x vs | head -1); readlink "/proc/$pid/cwd"')
        docker exec "$rig" bash /work/.local-runtime/observability/restore-adapters.sh "$rig_directory"
        if docker inspect zpr-observability >/dev/null 2>&1; then docker start zpr-observability >/dev/null; fi
        sh "$dashboard_dir/../observability/openobserve/collector.sh" stop
        ZPR_ORGANIZATION_ID="$organization" sh "$dashboard_dir/../observability/openobserve/collector.sh" start
    fi
else
    report_activation_status "Checking multi-node ZPR services"
    echo "Two-node Redwood runtime passed peer and service readiness checks"
fi
policy_changed=yes
report_activation_status "Applying organization runtime policy"
"$script_dir/dashboard-stack.sh" restart-policy-context "$organization" "$bundle_dir/runtime.zpl"
curl -fsS --max-time 10 http://127.0.0.1:8787/api/policy/context | jq -e --arg id "$organization" '.organization_id==$id' >/dev/null
control_changed=yes
report_activation_status "Checking Control Service readiness"
if [ "$organization_driver" = docker-multinode ]; then
    multinode_runtime=$(multinode_runtime_dir "$organization")
    SIMULATION_ORGANIZATION_ID="$organization" \
    ZPR_ASSERTION_LDAP_CONTAINER="${organization}-directory" \
    ZPR_ASSERTION_LDAP_BASE_DN="$organization_base_dn" \
    ZPR_ASSERTION_LDAP_BIND_DN="$organization_bind_dn" \
    ZPR_ADMIN_URL="https://127.0.0.1:8185" \
    ZPR_ADMIN_CA_FILE="$multinode_dir/zpr-conf/include/admin-tls-cert.pem" \
    ZPR_ADMIN_KEY_FILE="$multinode_runtime/bob/web-monitor.key" \
        "$script_dir/dashboard-stack.sh" restart-control-service
    curl -fsS --max-time 15 http://127.0.0.1:8787/api/snapshot | jq -e '.api_status=="connected" and any(.actors[]; .node)' >/dev/null
else
    SIMULATION_ORGANIZATION_ID="$organization" \
    ZPR_ASSERTION_LDAP_BASE_DN="$organization_base_dn" \
    ZPR_ASSERTION_LDAP_BIND_DN="$organization_bind_dn" \
        "$script_dir/dashboard-stack.sh" restart-control-service
    curl -fsS --max-time 15 http://127.0.0.1:8787/api/assertions/source >/dev/null
fi
echo "Organization policy, directory, and ZPR context ready"