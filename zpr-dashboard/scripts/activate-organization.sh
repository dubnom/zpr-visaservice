#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
dashboard_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
runtime_dir=$(CDPATH= cd -- "$dashboard_dir/../../.local-runtime" && pwd)
rig=${SIMULATION_CONTAINER:-zpr-local-linux-node}
action=${1:-}
organization=${2:-}
case "$action:$organization" in
    reset-organization:*|restore-base:*) ;;
    *) echo "usage: $0 {reset-organization|restore-base} organization-id" >&2; exit 2 ;;
esac
case "$organization" in ''|*[!a-z0-9-]*) echo "invalid organization id" >&2; exit 2 ;; esac
profile="$dashboard_dir/cmd/zpr-web-dashboard/examples/organizations/$organization.json"
[ -r "$profile" ] || { echo "organization profile missing" >&2; exit 1; }
organization_driver=$(jq -er '.runtime.driver' "$profile")
policy_config_relative=$(jq -er '.policy_config' "$profile")
policy_config="$dashboard_dir/cmd/zpr-web-dashboard/examples/$policy_config_relative"
runtime_policy_config="$policy_config"
organization_base_dn=$(jq -er '.directory.base_dn' "$profile")
organization_bind_dn="cn=zpr-reader,ou=Service Accounts,$organization_base_dn"
control_room_url=${SIMULATOR_OPERATOR_SERVICE_URL:-https://127.0.0.1:8790}
control_room_curl() {
    if [ "${ZPR_DASHBOARD_CONTAINER_RUNTIME:-}" = 1 ]; then
        curl --connect-to 127.0.0.1:8790:host.docker.internal:8790 \
            --cacert "$runtime_dir/service-certs/service-ca.crt" \
            --cert "$runtime_dir/service-certs/control-room-client.crt" \
            --key "$runtime_dir/service-certs/control-room-client.key" "$@"
    else
        curl --cacert "$runtime_dir/service-certs/service-ca.crt" \
            --cert "$runtime_dir/service-certs/control-room-client.crt" \
            --key "$runtime_dir/service-certs/control-room-client.key" "$@"
    fi
}
policy_service_curl() {
    if [ "${ZPR_DASHBOARD_CONTAINER_RUNTIME:-}" = 1 ]; then
        curl --connect-to 127.0.0.1:8789:host.docker.internal:8789 "$@"
    else
        curl "$@"
    fi
}
binary=${ZPR_WEB_DASHBOARD_BIN:-$runtime_dir/dashboard-stack/zpr-web-dashboard}
compiler=${ZPR_ZPLC_BIN:-$dashboard_dir/../../zpr-compiler/target/debug/zplc}
pregen="$runtime_dir/linux-integration/pregen"
multinode_dir="$dashboard_dir/../../zpr-demo/multinode-demo"
multinode_deploy="$multinode_dir/local-compute/deploy-docker.sh"
install_multinode_policy() {
    runtime_policy_config="$runtime_dir/multinode/$organization/admin/multinode-demo.zplc"
    [ -r "$runtime_policy_config" ] || { echo "multinode runtime policy config missing: $runtime_policy_config" >&2; return 1; }
    install_policy_config="$bundle_dir/install.zplc"
    "$binary" -mode merge-policy-config \
        -policy-config-base "$policy_config" \
        -policy-config-runtime "$runtime_policy_config" \
        -policy-config-bootstrap-dir "$runtime_dir/multinode/$organization/include" \
        -policy-output "$install_policy_config"
    "$compiler" "$bundle_dir/runtime.zpl" -c "$install_policy_config" \
        -k "$pregen/zpr-rsa-key.pem" -d "$bundle_dir" -o runtime.bin2
    docker run --rm \
        -v "$bundle_dir:/runtime/policy:ro" \
        -v "$multinode_dir/zpr-conf/include:/runtime/include:ro" \
        -v "$runtime_dir/multinode/$organization/bob:/runtime/operator:ro" \
        zpr-multinode /app/bin/vs-admin \
        --svc-url "https://host.docker.internal:${ZPR_ADMIN_PORT:-8185}" \
        --ca-cert /runtime/include/admin-tls-cert.pem \
        --api-key-file /runtime/operator/client.key \
        --format compact policies --path /runtime/policy/runtime.bin2
}
bundle_dir="$runtime_dir/organization-policy/$organization"
request_file="$runtime_dir/organization-request.json"
runtime_context_file="$runtime_dir/organization-runtime.json"
status_file=${SIMULATION_ACTIVATION_STATUS_FILE:-$runtime_dir/dashboard-stack/active-organization.txt.progress}
umask 077
mkdir -p "$bundle_dir"
exec >>"$runtime_dir/dashboard-stack/organization-reset.log" 2>&1
report_activation_status() {
    status_temp="$status_file.tmp.$$"
    printf '%s\n' "$1" >"$status_temp"
    mv "$status_temp" "$status_file"
}
write_runtime_context() {
    context_organization=$1
    context_base_dn=$2
    context_generation=$3
    context_temp=$(mktemp "${runtime_context_file}.XXXXXX")
    jq -n --arg organization_id "$context_organization" --arg ldap_base_dn "$context_base_dn" --arg generation "$context_generation" '{organization_id:$organization_id,ldap_base_dn:$ldap_base_dn,generation:$generation}' > "$context_temp"
    chmod 600 "$context_temp"
    mv "$context_temp" "$runtime_context_file"
}
report_activation_status "Preparing runtime policy"
printf 'Preflight organization %s\n' "$organization"
"$binary" -mode compose-policy -policy-root "$dashboard_dir/cmd/zpr-web-dashboard/examples" -policy-organization "$organization" -policy-output "$bundle_dir/runtime.zpl"
if [ "$organization_driver" = "docker-multinode" ]; then
    report_activation_status "Runtime policy source prepared"
elif [ -n "${ZPR_ZPLC_IMAGE:-}" ]; then
    docker run --rm --network none -v "$runtime_dir:/runtime" -v "$dashboard_dir:/dashboard:ro" "$ZPR_ZPLC_IMAGE" \
        "/runtime/organization-policy/$organization/runtime.zpl" \
        -c "/dashboard/cmd/zpr-web-dashboard/examples/$policy_config_relative" \
        -k /runtime/linux-integration/pregen/zpr-rsa-key.pem \
        -d "/runtime/organization-policy/$organization" -o runtime.bin2
else
    "$compiler" "$bundle_dir/runtime.zpl" -c "$runtime_policy_config" -k "$pregen/zpr-rsa-key.pem" -d "$bundle_dir" -o runtime.bin2
fi
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
if [ "$organization_driver" = docker-multinode ] || [ "$previous_driver" = docker-multinode ]; then
    report_activation_status "Preparing trusted-service helper before stopping runtime"
    sh "$script_dir/prepare-trusted-service.sh"
fi
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
restore_organization_base() {
    backup_directory="$runtime_dir/organization-backups/$organization/$(date -u +%Y%m%dT%H%M%SZ)-$generation"
    policy_database="$runtime_dir/dashboard-stack/policy-private/$organization-policy-only.db"
    staged_policy="$runtime_dir/dashboard-stack/staged-policy/$organization"
    published_directory="$runtime_dir/published-directories/$organization.ldif"
    mkdir -m 700 -p "$backup_directory"

    "$script_dir/dashboard-stack.sh" stop-policy-service
    for file_path in "$policy_database" "$policy_database-wal" "$policy_database-shm" "$published_directory"; do
        if [ -f "$file_path" ]; then
            cp -p "$file_path" "$backup_directory/$(basename "$file_path")"
        fi
    done
    if [ -d "$staged_policy" ]; then
        cp -R "$staged_policy" "$backup_directory/staged-policy"
    fi

    if [ "$organization_driver" = "docker-multinode" ]; then
        directory_root="$runtime_dir/multinode/$organization/directory"
        for file_path in "$directory_root/seed-complete" "$directory_root/company.ldif" "$directory_root/slapd.conf"; do
            if [ -f "$file_path" ]; then
                cp -p "$file_path" "$backup_directory/$(basename "$file_path")"
            fi
        done
        if [ -d "$directory_root/data" ]; then
            cp -R "$directory_root/data" "$backup_directory/directory-data"
        fi
    fi

    rm -f "$policy_database" "$policy_database-wal" "$policy_database-shm" "$published_directory"
    rm -rf "$staged_policy"
    if [ "$organization_driver" = "docker-multinode" ]; then
        rm -rf "$directory_root/data" "$directory_root/slapd.d"
        rm -f "$directory_root/seed-complete" "$directory_root/company.ldif" "$directory_root/slapd.conf"
    fi
    echo "Base-state restore backup: $backup_directory"
}
start_runtime() {
    runtime_id=$1
    runtime_driver=$(jq -er '.runtime.driver' "$dashboard_dir/cmd/zpr-web-dashboard/examples/organizations/$runtime_id.json")
    case "$runtime_driver" in
        linux-one-node)
            if [ "$(docker inspect -f '{{.State.Running}}' "$rig" 2>/dev/null || true)" = true ]; then docker restart "$rig" >/dev/null; else docker start "$rig" >/dev/null; fi
            ;;
        docker-multinode)
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
        "$script_dir/dashboard-stack.sh" stop-observability-collector || true
        stop_runtime "$organization" || true
        if [ "$had_request" = yes ]; then cp "$backup" "$request_file"; else rm -f "$request_file"; fi
        previous_generation=startup
        if [ -r "$request_file" ] && jq -e --arg id "$previous_organization" '.organization_id==$id' "$request_file" >/dev/null 2>&1; then
            previous_generation=$(jq -er '.generation // "startup"' "$request_file")
        fi
        write_runtime_context "$previous_organization" "$previous_base_dn" "$previous_generation" || true
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
        if [ "$previous_driver" = docker-multinode ]; then
            "$script_dir/dashboard-stack.sh" start-observability-collector "$previous_organization" || true
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
"$script_dir/dashboard-stack.sh" stop-observability-collector
stop_runtime "$previous_organization"
if [ "$action" = "restore-base" ]; then
    report_activation_status "Backing up and restoring organization base state"
    restore_organization_base
fi
report_activation_status "Starting $organization runtime"
start_runtime "$organization"
if [ "$organization_driver" = "docker-multinode" ]; then
    report_activation_status "Compiling and installing policy with deployed bootstrap keys"
    if ! install_multinode_policy; then
        echo "organization runtime policy installation failed" >&2
        exit 1
    fi
fi
if [ "$organization_driver" = linux-one-node ]; then
    report_activation_status "Starting DNS and platform relays"
    "$script_dir/dashboard-stack.sh" start-admin-relay
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
           control_room_curl -fsS --connect-timeout 1 --max-time 5 "$control_room_url/api/snapshot" | jq -e '(.errors|length)==0 and any(.actors[]; .node and .node_details.in_sync) and any(.actors[]; .cn=="adapter1")' >/dev/null; then break; fi
        attempt=$((attempt+1))
        sleep 1
    done
    [ "$attempt" -lt 600 ] || { echo "company bootstrap timed out"; exit 1; }
    report_activation_status "Starting DNS and UI relays"
    "$script_dir/dashboard-stack.sh" start-dns
    "$script_dir/dashboard-stack.sh" start-ui-relays
    report_activation_status "Restoring observability adapters"
    "$script_dir/dashboard-stack.sh" restart-simulator-control
    if [ -r "$runtime_dir/observability/restore-adapters.sh" ]; then
        rig_directory=$(docker exec "$rig" sh -c 'pid=$(pgrep -x vs | head -1); readlink "/proc/$pid/cwd"')
        docker exec "$rig" bash /work/.local-runtime/observability/restore-adapters.sh "$rig_directory"
        if docker inspect zpr-observability >/dev/null 2>&1; then docker start zpr-observability >/dev/null; fi
        collector_script="$dashboard_dir/../observability/openobserve/collector.sh"
        if [ -r "$collector_script" ]; then
            sh "$collector_script" stop
            ZPR_ORGANIZATION_ID="$organization" sh "$collector_script" start
        fi
    fi
else
    report_activation_status "Checking multi-node ZPR services"
    configured_node_count=$(jq -er '.runtime.nodes | length' "$dashboard_dir/cmd/zpr-web-dashboard/examples/organizations/$organization.json")
    echo "$configured_node_count-node $organization runtime passed configured peer and service readiness checks"
    report_activation_status "Starting organization ZPR DNS"
    SIMULATION_ORGANIZATION_ID="$organization" "$script_dir/dashboard-stack.sh" start-dns
fi
policy_changed=yes
report_activation_status "Applying organization runtime policy"
"$script_dir/dashboard-stack.sh" restart-policy-context "$organization" "$bundle_dir/runtime.zpl"
control_room_curl -fsS --max-time 10 "$control_room_url/api/policy/context" | jq -e --arg id "$organization" '.organization_id==$id' >/dev/null
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
    control_snapshot=
    control_attempt=0
    while [ "$control_attempt" -lt 60 ]; do
        control_snapshot=$(control_room_curl --silent --show-error --max-time 15 "$control_room_url/api/snapshot" || true)
        if printf '%s' "$control_snapshot" | jq -e '.api_status=="connected" and (.errors|length)==0' >/dev/null 2>&1; then
            break
        fi
        control_attempt=$((control_attempt + 1))
        sleep 1
    done
    if [ "$control_attempt" -ge 60 ]; then
        control_summary=$(printf '%s' "$control_snapshot" | jq -c '{api_status,errors}' 2>/dev/null || printf '%s' "$control_snapshot")
        echo "Control Service did not report a healthy snapshot for $organization: $control_summary" >&2
        exit 1
    fi
else
    SIMULATION_ORGANIZATION_ID="$organization" \
    ZPR_ASSERTION_LDAP_BASE_DN="$organization_base_dn" \
    ZPR_ASSERTION_LDAP_BIND_DN="$organization_bind_dn" \
        "$script_dir/dashboard-stack.sh" restart-control-service
    control_room_curl -fsS --max-time 15 "$control_room_url/api/assertions/source" >/dev/null
fi
if [ "$organization_driver" = docker-multinode ]; then
    report_activation_status "Starting organization observability collector"
    "$script_dir/dashboard-stack.sh" start-observability-collector "$organization"
fi
report_activation_status "Verifying policy, assertions, LDAP, DNS, and logs"
policy_settings=$(policy_service_curl --silent --show-error --max-time 10 \
    --cacert "$runtime_dir/service-certs/service-ca.crt" \
    --cert "$runtime_dir/service-certs/control-policy-client.crt" \
    --key "$runtime_dir/service-certs/control-policy-client.key" \
    https://127.0.0.1:8789/api/assertions/settings || true)
if ! printf '%s' "$policy_settings" | jq -e --arg id "$organization" --arg base_dn "$organization_base_dn" \
    '.organization_id==$id and .base_dn==$base_dn and (.settings.source|type=="string" and length>0)' >/dev/null 2>&1; then
    echo "Policy assertion settings do not match organization $organization" >&2
    exit 1
fi
ldap_source=$(control_room_curl --silent --show-error --max-time 15 "$control_room_url/api/assertions/source" || true)
if ! printf '%s' "$ldap_source" | jq -e --arg id "$organization" --arg base_dn "$organization_base_dn" \
    '.organization_id==$id and .base_dn==$base_dn and .people>0 and (.groups|length)>0' >/dev/null 2>&1; then
    ldap_summary=$(printf '%s' "$ldap_source" | jq -c '{organization_id,base_dn,people,groups,error}' 2>/dev/null || printf '%s' "$ldap_source")
    echo "Organization LDAP assertion source is not ready: $ldap_summary" >&2
    exit 1
fi
assertion_payload=$(printf '%s' "$policy_settings" | jq -c '{source:.settings.source,expected_revision:.settings.revision}')
assertion_evaluation=$(control_room_curl --silent --show-error --max-time 15 \
    -H 'Content-Type: application/json' -d "$assertion_payload" \
    "$control_room_url/api/assertions/evaluate" || true)
if ! printf '%s' "$assertion_evaluation" | jq -e '.status=="pass" and (.results|length)>0 and all(.results[];.status=="pass")' >/dev/null 2>&1; then
    assertion_summary=$(printf '%s' "$assertion_evaluation" | jq -c '{organization_id,status,results,error}' 2>/dev/null || printf '%s' "$assertion_evaluation")
    echo "Organization assertions failed against LDAP: $assertion_summary" >&2
    exit 1
fi
control_snapshot=$(control_room_curl --silent --show-error --max-time 15 "$control_room_url/api/snapshot" || true)
if ! printf '%s' "$control_snapshot" | jq -e '.api_status=="connected" and (.errors|length)==0' >/dev/null 2>&1; then
    snapshot_summary=$(printf '%s' "$control_snapshot" | jq -c '{api_status,errors}' 2>/dev/null || printf '%s' "$control_snapshot")
    echo "Control Service snapshot is not healthy: $snapshot_summary" >&2
    exit 1
fi
adapter_logs=$(control_room_curl --silent --show-error --max-time 15 "$control_room_url/api/adapter-logs" || true)
if ! printf '%s' "$adapter_logs" | jq -e '.adapters|type=="array" and length>0 and any(.[]; any(.sources[]; (.error // "")=="" and ((.lines // [])|length)>0))' >/dev/null 2>&1; then
    echo "Control Service adapter log inventory has no available log sources" >&2
    exit 1
fi
if [ "$organization_driver" = linux-one-node ]; then
    dns_stats=$(control_room_curl --silent --show-error --max-time 15 "$control_room_url/api/dns/stats/json/v1/status" || true)
    if ! printf '%s' "$dns_stats" | jq -e 'type=="object" and length>0' >/dev/null 2>&1; then
        echo "DNS statistics are unavailable for organization $organization" >&2
        exit 1
    fi
    dns_records=$(control_room_curl --silent --show-error --max-time 15 "$control_room_url/api/dns/records" || true)
    if ! printf '%s' "$dns_records" | jq -e '.zone=="svc.zpr." and (.records|type=="array" and length>0)' >/dev/null 2>&1; then
        echo "DNS zone transfer returned no records for organization $organization" >&2
        exit 1
    fi
fi
write_runtime_context "$organization" "$organization_base_dn" "$generation"
echo "Organization policy, directory, and ZPR context ready"