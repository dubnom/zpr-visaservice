#!/bin/sh
# EVALUATION ONLY endpoint launcher (alternate entrypoint of the data-plane
# image). Runs one ZPR adapter that docks with an evaluation install's node
# over the install's Docker network, authenticated by a per-install bootstrap
# key compiled into the install policy (shortcut X15, not enrollment).
#
# Inputs (set by the Simulator): ZPR_EP_NAME (endpoint-NN), ZPR_EP_KEY and
# ZPR_EP_CA (read-only mounted files), ZPR_EP_NODE (data-plane container name
# on the shared network) and ZPR_EP_ZPR_ADDR (requested ZPR address).
set -eu
name=${ZPR_EP_NAME:?ZPR_EP_NAME is required}
key=${ZPR_EP_KEY:?ZPR_EP_KEY is required}
ca=${ZPR_EP_CA:?ZPR_EP_CA is required}
node=${ZPR_EP_NODE:?ZPR_EP_NODE is required}
zpr_addr=${ZPR_EP_ZPR_ADDR:?ZPR_EP_ZPR_ADDR is required}
case $name in
endpoint-[0-9][0-9]) ;;
*) echo "invalid endpoint name" >&2; exit 2 ;;
esac

log() { printf '%s [%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$name" "$*"; }

tries=30
while ! node_addr=$(getent ahostsv4 "$node" | awk 'NR == 1 { print $1 }') || [ -z "$node_addr" ]; do
	tries=$((tries - 1))
	[ "$tries" -gt 0 ] || { log "data-plane node $node is not resolvable"; exit 1; }
	sleep 1
done

mkdir -p /run/zpr
ip tuntap add name tun0 mode tun multi_queue
ip link set tun0 up
log "docking with node at $node_addr as $zpr_addr"
exec ph adapter \
	--logging all=INFO \
	--control-path /run/zpr/adapter.sock \
	--capture-path /run/zpr/adapter_cap.sock \
	--self-addr 0.0.0.0:0 \
	--ca-file "$ca" \
	--bootstrap-key "$key" \
	--name "$name" \
	--km-impl noise \
	--tun-if tun0 \
	--io-engine auto \
	--node-addr "$node_addr" \
	--zpr-addr "$zpr_addr"
