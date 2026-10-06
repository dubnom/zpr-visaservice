#!/bin/sh
set -eu

config=${1:?Operator node exporter configuration is required}
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
jq -e 'type == "array" and length > 0 and length <= 100 and all(.[];
    (.container | type == "string") and (.cli | startswith("/")) and
    (.socket | startswith("/")) and (.output | startswith("/")) and
    (.service_name | type == "string") and (.instance_id | type == "string"))' "$config" >/dev/null

while :; do
    jq -r '.[] | [.container, .cli, .socket, .service_name, .instance_id, .output] | @tsv' "$config" |
    while IFS="$(printf '\t')" read -r container cli socket service instance output; do
        temp="$output.tmp"
        if ZPR_NODE_CONTAINER="$container" sh "$script_dir/node-denial-metrics.sh" "$cli" "$socket" "$service" "$instance" >"$temp"; then
            mv "$temp" "$output"
        else
            echo "Node denial export failed for $instance; last sample will expire." >&2
            rm -f "$temp"
        fi
    done
    sleep 1
done
