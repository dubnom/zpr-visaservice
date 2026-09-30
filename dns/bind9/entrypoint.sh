#!/bin/sh
set -eu

adapter_name=${ZPR_DNS_ADAPTER_NAME:-adapter1}
config_file=${ZPR_DNS_NAMED_CONF:-/etc/bind/named.conf}
key_source=${ZPR_DNS_TSIG_KEY_FILE:-/run/secrets/zpr-vs-publisher.key}
key_runtime=/run/named/zpr-vs-publisher.key

if [ ! -r "$key_source" ]; then
    echo "ZPR DNS TSIG key is not readable: $key_source" >&2
    exit 1
fi
mkdir -p /run/named
chown bind:bind /run/named
chmod 0750 /run/named
cp "$key_source" "$key_runtime"
chown bind:bind "$key_runtime"
chmod 0640 "$key_runtime"

adapter_pid=
while [ -z "$adapter_pid" ]; do
    for cmdline in /proc/[0-9]*/cmdline; do
        [ -r "$cmdline" ] || continue
        pid=${cmdline#/proc/}
        pid=${pid%/cmdline}
        [ "$(cat "/proc/$pid/comm" 2>/dev/null || true)" = "ph" ] || continue
        args=$(tr '\000' ' ' < "$cmdline" 2>/dev/null || true)
        case "$args" in
            *"adapter "*"--name $adapter_name"*)
                adapter_pid=$pid
                break
                ;;
        esac
    done
    if [ -z "$adapter_pid" ]; then
        echo "Waiting for ZPR adapter '$adapter_name' in the shared PID namespace" >&2
        sleep 2
    fi
done

echo "Starting BIND in adapter '$adapter_name' network namespace (pid $adapter_pid)" >&2
exec nsenter --target "$adapter_pid" --net /usr/sbin/named -g -u bind -c "$config_file"
