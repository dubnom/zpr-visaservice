#!/bin/sh
set -eu

profile_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
image=${ZPR_OBSERVABILITY_IMAGE:-zpr-observability:local}
container=${ZPR_OBSERVABILITY_CONTAINER:-zpr-observability}
rig=${ZPR_SIMULATION_CONTAINER:-zpr-local-linux-node}
volume=${ZPR_OBSERVABILITY_VOLUME:-zpr-observability-data}
local_network=${ZPR_OBSERVABILITY_LOCAL_NETWORK:-zpr-observability-local}
local_container=${ZPR_OBSERVABILITY_LOCAL_CONTAINER:-zpr-observability-local}
local_port=${ZPR_OBSERVABILITY_LOCAL_PORT:-8800}

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
    start-local)
        : "${ZPR_OBSERVABILITY_ENV_FILE:?set ZPR_OBSERVABILITY_ENV_FILE to a private env file}"
        [ -r "$ZPR_OBSERVABILITY_ENV_FILE" ] || { echo "observability env file is unreadable" >&2; exit 1; }
        image_runtime=$(docker image inspect --format '{{index .Config.Labels "zpr.observability.runtime"}}' "$image" 2>/dev/null || true)
        [ "$image_runtime" = local-v1 ] || docker build -t "$image" "$profile_dir"
        docker network inspect "$local_network" >/dev/null 2>&1 || docker network create "$local_network" >/dev/null
        if docker container inspect "$local_container" >/dev/null 2>&1; then
            [ "$(docker inspect -f '{{.State.Running}}' "$local_container")" = true ] || docker start "$local_container" >/dev/null
            exit 0
        fi
        docker volume create "$volume" >/dev/null
        docker run --rm -v "$volume:/data" --entrypoint chown "$image" 10001:10001 /data
        docker run -d --name "$local_container" --restart unless-stopped --network "$local_network" \
            -p "127.0.0.1:$local_port:5080" --env-file "$ZPR_OBSERVABILITY_ENV_FILE" \
            -e ZO_DATA_DIR=/data -e ZO_HTTP_PORT=5080 -e ZO_HTTP_ADDR=0.0.0.0 \
            -e ZO_HTTP_IPV6_ENABLED=false -e ZO_GRPC_ADDR=127.0.0.1 -e ZO_TELEMETRY=false \
            -v "$volume:/data" --user 10001:10001 --entrypoint /usr/local/bin/openobserve "$image"
        ;;
    stop-local)
        docker stop "$local_container" >/dev/null 2>&1 || true
        ;;
    status-local)
        docker inspect -f '{{.State.Status}}' "$local_container"
        ;;
    *)
        echo "usage: $0 {build|start|stop|status|start-local|stop-local|status-local}" >&2
        exit 2
        ;;
esac