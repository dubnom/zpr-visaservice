#!/bin/sh
# EVALUATION ONLY data-plane launcher (container entrypoint).
#
# Root namespace = the node: Docker substrate interface, veth to the VS
# namespace, and tun0 at fd5a:5052::2. Namespace zpr-vs = Valkey, the Visa
# Service at its fixed ZPR address fd5a:5052::1 and the vs.zpr adapter.
# The VS admin API (8182) is relayed to the root namespace for a loopback
# Docker publish; subject snapshots are fetched through a relay to the host.
#
# Inputs (set by zpr-lab-bootstrap): ZPR_DP_STATE (state dir, mounted at the
# same path; read-only dp-* identities, writable data-plane/), ZPR_DP_INSTALL_POLICY=1 to load the compiled install policy, and
# optional ZPR_DP_DELIVERY_PORT. Any process exit stops the whole data plane.
set -eu
state=${ZPR_DP_STATE:?ZPR_DP_STATE is required}
plane=$state/data-plane
run=/run/zpr
mkdir -p "$run"

node_substrate=10.0.0.1
vs_substrate=10.0.0.2
node_zpr=fd5a:5052::2
vs_zpr=fd5a:5052::1

log() { printf '%s [data-plane] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }
prefix() { sed -u "s/^/[$1] /"; }
pids=""
spawn() {
	name=$1
	shift
	( "$@" 2>&1 | prefix "$name" ) &
	pids="$pids $!"
}
stop() {
	trap - EXIT INT TERM
	for pid in $pids; do kill "$pid" 2>/dev/null || true; done
	pkill -TERM -x ph 2>/dev/null || true
	pkill -TERM -x vs 2>/dev/null || true
	pkill -TERM -x valkey-server 2>/dev/null || true
	pkill -TERM -x socat 2>/dev/null || true
	sleep 1
	ip netns delete zpr-vs 2>/dev/null || true
}
trap 'stop; exit 0' INT TERM
trap stop EXIT

wait_for() {
	tries=$1
	shift
	while ! "$@" >/dev/null 2>&1; do
		tries=$((tries - 1))
		[ "$tries" -gt 0 ] || return 1
		sleep 1
	done
}

if [ ! -s "$plane/vs-policy.bin2" ]; then
	log "compiling install policy"
	mkdir -p "$plane"
	rm -f "$plane/vs-policy.bin2.tmp"
	(cd "$state" && zplc -c dp-policy.zplc -k dp-policy-signing.key -f v2 -o "$plane/vs-policy.bin2.tmp" dp-policy.zpl) 2>&1 | prefix zplc
	[ -s "$plane/vs-policy.bin2.tmp" ] || { log "policy compilation failed"; exit 1; }
	mv "$plane/vs-policy.bin2.tmp" "$plane/vs-policy.bin2"
fi

log "creating namespaces, veth and TUN devices"
ip netns delete zpr-vs 2>/dev/null || true
ip netns add zpr-vs
ip -n zpr-vs link set lo up
ip link add veth-vs type veth peer name veth0 netns zpr-vs
ip addr add "$node_substrate" peer "$vs_substrate" dev veth-vs
ip -n zpr-vs addr add "$vs_substrate" peer "$node_substrate" dev veth0
ip link set veth-vs up
ip -n zpr-vs link set veth0 up
ip tuntap add name tun0 mode tun multi_queue
ip -n zpr-vs tuntap add name tun0 mode tun multi_queue
ip link set tun0 up
ip -n zpr-vs link set tun0 up
ip addr add "$node_zpr" peer "$vs_zpr" dev tun0
ip -n zpr-vs addr add "$vs_zpr" peer "$node_zpr" dev tun0

spawn valkey ip netns exec zpr-vs valkey-server "$state/valkey.conf"
# Connection check only: an unauthenticated PING still proves Valkey listens.
wait_for 20 ip netns exec zpr-vs valkey-cli -p 6379 ping || { log "Valkey did not start"; exit 1; }

set -- -c "$state/vs.toml"
if [ "${ZPR_DP_INSTALL_POLICY:-}" = 1 ]; then
	set -- "$@" "$plane/vs-policy.bin2"
fi
mkdir -p "$plane/vs-data"
spawn vs ip netns exec zpr-vs env XDG_DATA_HOME="$plane/vs-data" vs "$@"

spawn admin-relay socat TCP-LISTEN:8182,fork,reuseaddr "TCP:$vs_substrate:8182"
if [ -n "${ZPR_DP_DELIVERY_PORT:-}" ]; then
	port=$ZPR_DP_DELIVERY_PORT
	spawn delivery-relay-vs ip netns exec zpr-vs socat "TCP-LISTEN:$port,bind=127.0.0.1,fork,reuseaddr" "TCP:$node_substrate:$port"
	spawn delivery-relay-host socat "TCP-LISTEN:$port,bind=$node_substrate,fork,reuseaddr" "TCP:host.docker.internal:$port"
fi

sleep 2
log "starting node"
spawn node ph node \
	--logging all=INFO \
	--control-path "$run/node.sock" \
	--capture-path "$run/node_cap.sock" \
	--advertised-substrate-addr "$node_substrate:5000" \
	--ca-file "$state/dp-ca.crt" \
	--certificate-file "$state/dp-node.crt" \
	--private-key-file "$state/dp-node.key" \
	--auth-private-key "$state/dp-node-auth.key" \
	--km-impl noise \
	--tun-if tun0 \
	--zpr-addr "$node_zpr"
sleep 2
log "starting VS adapter"
spawn vs-adapter ip netns exec zpr-vs ph adapter \
	--logging all=INFO \
	--control-path "$run/vs.sock" \
	--capture-path "$run/vs_cap.sock" \
	--self-addr "$vs_substrate" \
	--ca-file "$state/dp-ca.crt" \
	--certificate-file "$state/dp-vs.zpr.crt" \
	--private-key-file "$state/dp-vs.zpr.key" \
	--bootstrap-key "$state/dp-vs-auth.key" \
	--km-impl noise \
	--tun-if tun0 \
	--io-engine auto \
	--node-addr "$node_substrate" \
	--zpr-addr "$vs_zpr"

log "running"
# Any component exit is fatal; the bootstrap supervisor reports it.
while :; do
	for pid in $pids; do
		if ! kill -0 "$pid" 2>/dev/null; then
			log "a data-plane process exited; stopping"
			exit 1
		fi
	done
	sleep 1
done
