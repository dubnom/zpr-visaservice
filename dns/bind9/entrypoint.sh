#!/bin/sh
set -eu

adapter_name=${ZPR_DNS_ADAPTER_NAME:-adapter1}
config_file=${ZPR_DNS_NAMED_CONF:-/etc/bind/named.conf}
key_source=${ZPR_DNS_TSIG_KEY_FILE:-/run/secrets/zpr-vs-publisher.key}
key_runtime=/run/named/zpr-vs-publisher.key
viewer_key_source=${ZPR_DNS_TRANSFER_TSIG_KEY_FILE:-/run/secrets/zpr-dns-viewer.key}
viewer_key_runtime=/run/named/zpr-dns-viewer.key
zone_file=${ZPR_DNS_ZONE_FILE:-/var/lib/bind/db.svc.zpr}

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
if [ -r "$viewer_key_source" ]; then
    cp "$viewer_key_source" "$viewer_key_runtime"
    chown bind:bind "$viewer_key_runtime"
    chmod 0640 "$viewer_key_runtime"
fi
zone_directory=$(dirname "$zone_file")
chown bind:bind "$zone_directory"
chmod 0750 "$zone_directory"
if [ -e "$zone_file" ]; then
    chown bind:bind "$zone_file"
    chmod 0640 "$zone_file"
fi

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
