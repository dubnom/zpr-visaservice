#!/bin/sh
set -eu

profile_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(CDPATH= cd -- "$profile_dir/../.." && pwd)
workspace_dir=$(CDPATH= cd -- "$repo_dir/.." && pwd)
runtime_dir="${ZPR_OBSERVABILITY_RUNTIME_DIR:-$workspace_dir/.local-runtime/observability}"
rig=${ZPR_SIMULATION_CONTAINER:-zpr-local-linux-node}
container=${ZPR_OBSERVABILITY_CONTAINER:-zpr-observability}
address=${ZPR_OBSERVABILITY_ADDR:-fd5a:5052:adda:1::54}
publisher_tun=${ZPR_TELEMETRY_PUBLISHER_TUN:-tun6}
pid_file="$runtime_dir/collector.pid"
log_file="$runtime_dir/collector.log"
container_runtime=/work/.local-runtime
collector_script=/work/zpr-visaservice/observability/openobserve/collector.py
simulation_manifest=${SIMULATION_MANIFEST:-$workspace_dir/.local-runtime/simulation-environment.json}
case "$simulation_manifest" in
    */*) manifest_directory=${simulation_manifest%/*} ;;
    *) manifest_directory=$workspace_dir ;;
esac
active_organization_file=${SIMULATION_ACTIVE_ORGANIZATION_FILE:-$manifest_directory/active-organization.txt}
zpr_organization_id=${ZPR_ORGANIZATION_ID:-}
if [ -z "$zpr_organization_id" ]; then
    if [ -r "$active_organization_file" ]; then
        IFS= read -r zpr_organization_id <"$active_organization_file" || true
    else
        zpr_organization_id=${SIMULATION_ORGANIZATION_ID:-northstar}
    fi
fi
case "$zpr_organization_id" in
    ''|*[!a-z0-9-]*) echo "invalid ZPR organization ID for telemetry tagging" >&2; exit 2 ;;
esac

case "${1:-}" in
    start)
        [ -r "$runtime_dir/collector.env" ] || { echo "collector config is missing" >&2; exit 1; }
        [ -r "$runtime_dir/ingestion.token" ] || { echo "ingestion token is missing" >&2; exit 1; }
        docker inspect "$rig" >/dev/null
        docker inspect "$container" >/dev/null
        if [ -f "$pid_file" ] && docker exec "$rig" kill -0 "$(cat "$pid_file")" 2>/dev/null; then
            echo "telemetry collector already running (pid $(cat "$pid_file"))"
            exit 0
        fi
        docker exec "$rig" ip -n zpr-vs link show "$publisher_tun" >/dev/null
        docker exec "$rig" ip -n zpr-vs -6 route replace "$address/128" dev "$publisher_tun"
        docker exec -d -e ZPR_RUNTIME_DIR="$container_runtime" -e ZPR_ORGANIZATION_ID="$zpr_organization_id" "$rig" sh -c \
            'echo $$ > "$1"; exec ip netns exec zpr-vs python3 -u "$2" >> "$3" 2>&1' \
            zpr-telemetry-collector "$container_runtime/observability/collector.pid" "$collector_script" "$container_runtime/observability/collector.log"
        ;;
    stop)
        if [ -f "$pid_file" ]; then
            docker exec "$rig" kill -TERM "$(cat "$pid_file")" 2>/dev/null || true
            rm -f "$pid_file"
        fi
        ;;
    status)
        if [ -f "$pid_file" ] && docker exec "$rig" kill -0 "$(cat "$pid_file")" 2>/dev/null; then
            echo "running (pid $(cat "$pid_file"))"
        else
            echo "stopped"
            exit 1
        fi
        ;;
    *)
        echo "usage: $0 {start|stop|status}" >&2
        exit 2
        ;;
esac