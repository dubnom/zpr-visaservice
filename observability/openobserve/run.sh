#!/bin/sh
set -eu

profile_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
image=${ZPR_OBSERVABILITY_IMAGE:-zpr-observability:local}
container=${ZPR_OBSERVABILITY_CONTAINER:-zpr-observability}
rig=${ZPR_SIMULATION_CONTAINER:-zpr-local-linux-node}
volume=${ZPR_OBSERVABILITY_VOLUME:-zpr-observability-data}

case "${1:-}" in
    build)
        docker build -t "$image" "$profile_dir"
        ;;
    start)
        : "${ZPR_OBSERVABILITY_ENV_FILE:?set ZPR_OBSERVABILITY_ENV_FILE to a private env file}"
        : "${ZPR_OBSERVABILITY_ADAPTER_NAME:?set the dedicated adapter name}"
        : "${ZPR_OBSERVABILITY_ADDR:?set the dedicated adapter ZPR address}"
        [ -r "$ZPR_OBSERVABILITY_ENV_FILE" ] || { echo "observability env file is unreadable" >&2; exit 1; }
        docker inspect "$rig" >/dev/null
        if docker container inspect "$container" >/dev/null 2>&1; then
            echo "observability container already exists: $container" >&2
            exit 1
        fi
        docker volume create "$volume" >/dev/null
        docker run --rm -v "$volume:/data" --entrypoint chown "$image" 10001:10001 /data
        docker run -d --name "$container" --restart unless-stopped --network none --pid "container:$rig" --privileged \
            --env-file "$ZPR_OBSERVABILITY_ENV_FILE" \
            -e ZPR_OBSERVABILITY_ADAPTER_NAME="$ZPR_OBSERVABILITY_ADAPTER_NAME" \
            -e ZPR_OBSERVABILITY_ADDR="$ZPR_OBSERVABILITY_ADDR" \
            -v "$volume:/data" "$image"
        sh "$profile_dir/collector.sh" start
        ;;
    stop)
        sh "$profile_dir/collector.sh" stop
        docker rm -f "$container"
        ;;
    status)
        docker inspect -f '{{.State.Status}}' "$container"
        sh "$profile_dir/collector.sh" status || true
        ;;
    *)
        echo "usage: $0 {build|start|stop|status}" >&2
        exit 2
        ;;
esac