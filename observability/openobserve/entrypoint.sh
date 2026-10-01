#!/bin/sh
set -eu

: "${ZPR_OBSERVABILITY_ADDR:?set ZPR_OBSERVABILITY_ADDR to the dedicated adapter ZPR address}"
: "${ZO_ROOT_USER_EMAIL:?set ZO_ROOT_USER_EMAIL in a private env file}"
: "${ZO_ROOT_USER_PASSWORD:?set ZO_ROOT_USER_PASSWORD in a private env file}"
: "${ZPR_OBSERVABILITY_ADAPTER_NAME:?set ZPR_OBSERVABILITY_ADAPTER_NAME to the dedicated adapter name}"
if [ "${ZO_HTTP_ADDR:-}" != "127.0.0.1" ] || [ "${ZO_HTTP_IPV6_ENABLED:-}" != "false" ] ||
    [ "${ZO_HTTP_PORT:-}" != "5080" ] || [ "${ZO_GRPC_ADDR:-}" != "127.0.0.1" ]; then
    echo "OpenObserve must bind HTTP and gRPC on their configured private loopback sockets" >&2
    exit 1
fi

adapter_pid=
attempt=0
while [ -z "$adapter_pid" ] && [ "$attempt" -lt 30 ]; do
    for cmdline in /proc/[0-9]*/cmdline; do
        [ -r "$cmdline" ] || continue
        pid=${cmdline#/proc/}
        pid=${pid%/cmdline}
        [ "$(cat "/proc/$pid/comm" 2>/dev/null || true)" = "ph" ] || continue
        args=$(tr '\000' ' ' < "$cmdline" 2>/dev/null || true)
        case "$args" in
            *"adapter "*"--name $ZPR_OBSERVABILITY_ADAPTER_NAME"*)
                adapter_pid=$pid
                break
                ;;
        esac
    done
    if [ -z "$adapter_pid" ]; then
        attempt=$((attempt + 1))
        sleep 2
    fi
done

if [ -z "$adapter_pid" ]; then
    echo "ZPR observability adapter not found in the shared PID namespace" >&2
    exit 1
fi
if ! nsenter --target "$adapter_pid" --net ip -6 addr show | grep -Fq "inet6 $ZPR_OBSERVABILITY_ADDR/"; then
    echo "ZPR observability address is not assigned to the adapter" >&2
    exit 1
fi

nsenter --target "$adapter_pid" --net setpriv --reuid 10001 --regid 10001 --clear-groups /usr/local/bin/openobserve &
server_pid=$!
proxy_pid=
trap 'kill "$server_pid" "$proxy_pid" 2>/dev/null || true' INT TERM EXIT
nsenter --target "$adapter_pid" --net setpriv --reuid 10001 --regid 10001 --clear-groups socat \
    "TCP6-LISTEN:5080,bind=[$ZPR_OBSERVABILITY_ADDR],ipv6only=1,reuseaddr,fork" \
    TCP4:127.0.0.1:5080 &
proxy_pid=$!
wait "$server_pid"