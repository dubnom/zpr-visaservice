#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
RELAY_CONTAINER=zpr-dns-stats-relay
RELAY_NETWORK=zpr-dns-stats-read-only
RELAY_IMAGE=zpr-dns-stats-relay:local

case "${1:-}" in
    start)
        config=${2:?Usage: control-dns-stats.sh start CONFIG.json}
        if ! source_container=$(jq -er '.source_container | select(type == "string" and length > 0)' "$config"); then
            echo "DNS statistics relay configuration requires a nonempty source_container string" >&2
            exit 1
        fi
        case "$source_container" in
            *[!a-zA-Z0-9_.-]*|-*)
                echo "DNS statistics source_container must be a Docker container name" >&2
                exit 1
                ;;
        esac
        docker exec "$source_container" sh -c 'command -v socat >/dev/null'
        if docker network inspect "$RELAY_NETWORK" >/dev/null 2>&1; then
            [ "$(docker network inspect --format '{{.Internal}}' "$RELAY_NETWORK")" = true ] || {
                echo "DNS statistics relay network must be internal" >&2
                exit 1
            }
        else
            docker network create --internal "$RELAY_NETWORK" >/dev/null
        fi
        if docker inspect "$RELAY_CONTAINER" >/dev/null 2>&1; then
            [ "$(docker inspect --format '{{index .Config.Labels "zpr.dns-stats-relay"}}' "$RELAY_CONTAINER")" = true ] || {
                echo "Refusing to start an unowned DNS statistics relay container" >&2
                exit 1
            }
            configured_source=$(docker inspect --format '{{index .Config.Labels "zpr.dns-stats-source"}}' "$RELAY_CONTAINER")
            [ "$configured_source" = "$source_container" ] || {
                echo "DNS statistics relay source changed; stop the existing relay before reconfiguring" >&2
                exit 1
            }
            [ "$(docker inspect --format '{{.HostConfig.NetworkMode}}' "$RELAY_CONTAINER")" = "$RELAY_NETWORK" ] || {
                echo "DNS statistics relay is not attached to its private network" >&2
                exit 1
            }
            docker start "$RELAY_CONTAINER" >/dev/null
        else
            if ! docker image inspect "$RELAY_IMAGE" >/dev/null 2>&1; then
                docker build -t "$RELAY_IMAGE" -f "$SCRIPT_DIR/Dockerfile.dns-stats-relay" "$SCRIPT_DIR"
            fi
            docker run -d --name "$RELAY_CONTAINER" \
                --label zpr.dns-stats-relay=true \
                --label "zpr.dns-stats-source=$source_container" \
                --restart unless-stopped \
                --network "$RELAY_NETWORK" \
                --read-only --cap-drop ALL --security-opt no-new-privileges \
                -v "${ZPR_DOCKER_SOCKET:-/var/run/docker.sock}:/var/run/docker.sock" \
                "$RELAY_IMAGE" \
                TCP-LISTEN:8054,bind=0.0.0.0,reuseaddr,fork \
                "EXEC:\"docker exec -i $source_container socat STDIO TCP:127.0.0.1:8053\"" >/dev/null
        fi
        attempts=0
        until docker exec "$RELAY_CONTAINER" curl -fsS --connect-timeout 1 --max-time 2 \
            http://127.0.0.1:8054/json/v1/status >/dev/null 2>&1; do
            attempts=$((attempts + 1))
            if [ "$attempts" -ge 30 ]; then
                docker logs --tail 30 "$RELAY_CONTAINER" >&2
                echo "DNS statistics relay did not become ready" >&2
                exit 1
            fi
            sleep 0.2
        done
        ;;
    stop)
        if docker inspect "$RELAY_CONTAINER" >/dev/null 2>&1; then
            [ "$(docker inspect --format '{{index .Config.Labels "zpr.dns-stats-relay"}}' "$RELAY_CONTAINER")" = true ] || {
                echo "Refusing to remove an unowned DNS statistics relay container" >&2
                exit 1
            }
            docker rm -f "$RELAY_CONTAINER" >/dev/null
        fi
        ;;
    *)
        echo "Usage: control-dns-stats.sh start CONFIG.json | stop" >&2
        exit 1
        ;;
esac
