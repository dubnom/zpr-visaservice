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
# Protected DNS uses its own namespace so BIND replies leave through its adapter.
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
	pkill -TERM -x named 2>/dev/null || true
	pkill -TERM -x valkey-server 2>/dev/null || true
	pkill -TERM -x socat 2>/dev/null || true
	sleep 1
	ip netns delete zpr-vs 2>/dev/null || true
	ip netns delete zpr-dns 2>/dev/null || true
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
# Multi-node installs (ZPR_DP_NODE_SUBSTRATE set) add dp-node.toml: the node
# binds and advertises its fixed Docker address and pins its Noise peers; the
# VS adapter then reaches it through the veth. Command-line options override
# the file, so the D1 veth advertisement is passed only for a single node.
dock_addr=$node_substrate
if [ -n "${ZPR_DP_NODE_SUBSTRATE:-}" ]; then
	dock_addr=$ZPR_DP_NODE_SUBSTRATE
	ip -n zpr-vs route add "$dock_addr/32" via "$node_substrate" dev veth0 2>/dev/null ||
		ip -n zpr-vs route add "$dock_addr/32" dev veth0
	# tun0 is point-to-point to the docking node only; the Visa Service must
	# also answer the linked nodes' VSAPI sessions and reach their VSS.
	ip -n zpr-vs route add fd5a:5052::/64 dev tun0
	set -- --config-file "$state/dp-node.toml"
else
	set -- --advertised-substrate-addr "$node_substrate:5000"
fi
spawn node ph node \
	--logging all=INFO \
	--control-path "$run/node.sock" \
	--capture-path "$run/node_cap.sock" \
	"$@" \
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
	--node-addr "$dock_addr" \
	--zpr-addr "$vs_zpr"

if [ "${ZPR_DP_PROTECTED_DNS:-}" = 1 ]; then
	dns_zpr=fd5a:5052::53
	dns_zone=/var/lib/bind
	ip -n zpr-vs -6 route add "$dns_zpr/128" dev tun0
	# Keep BIND and its adapter together, separate from the co-located node tunnel.
	ip netns delete zpr-dns 2>/dev/null || true
	ip netns add zpr-dns
	ip -n zpr-dns link set lo up
	ip link add veth-dns type veth peer name veth0 netns zpr-dns
	ip addr add 10.0.0.3 peer 10.0.0.4 dev veth-dns
	ip -n zpr-dns addr add 10.0.0.4 peer 10.0.0.3 dev veth0
	ip link set veth-dns up
	ip -n zpr-dns link set veth0 up
	ip -n zpr-dns route add 10.0.0.1/32 via 10.0.0.3 dev veth0
	ip -n zpr-dns tuntap add name tun0 mode tun multi_queue
	ip -n zpr-dns -6 addr add "$dns_zpr/128" dev tun0
	ip -n zpr-dns link set tun0 up
	# Replies to ZPR clients must leave through this adapter's TUN.
	ip -n zpr-dns -6 route add fd5a:5052::/32 dev tun0
	mkdir -p /run/named
	chown bind:bind /run/named
	chmod 0750 /run/named
	mkdir -p "$dns_zone"
	if [ ! -s "$dns_zone/db.svc.zpr" ]; then
		cat > "$dns_zone/db.svc.zpr" <<EOF
\$ORIGIN svc.zpr.
\$TTL 30
@ IN SOA dns.svc.zpr. hostmaster.svc.zpr. (1 60 60 86400 30)
  IN NS dns.svc.zpr.
dns IN AAAA $dns_zpr
EOF
	fi
	spawn dns-adapter ip netns exec zpr-dns env ZPR_ADAPTER_SERVICES=ZprDNS ph adapter \
		--logging all=INFO \
		--control-path "$run/dns.sock" \
		--capture-path "$run/dns_cap.sock" \
		--self-addr 10.0.0.4:0 \
		--ca-file "$state/dp-ca.crt" \
		--certificate-file "$state/dp-dns.crt" \
		--private-key-file "$state/dp-dns.key" \
		--bootstrap-key "$state/dp-dns-auth.key" \
		--name zpr-dns \
		--km-impl noise \
		--tun-if tun0 \
		--node-addr "$node_substrate:5000" \
		--zpr-addr "$dns_zpr"
	cp "$state/dp-dns-publisher.key" "$run/zpr-vs-publisher.key"
	chown bind:bind "$run/zpr-vs-publisher.key"
	chmod 0640 "$run/zpr-vs-publisher.key"
	chown bind:bind "$dns_zone"
	chmod 0750 "$dns_zone"
	chown bind:bind "$dns_zone/db.svc.zpr"
	chmod 0640 "$dns_zone/db.svc.zpr"
	named-checkconf "$state/dp-dns-named.conf"
	named-checkzone svc.zpr "$dns_zone/db.svc.zpr" >/dev/null
	spawn protected-dns ip netns exec zpr-dns /usr/sbin/named -g -u bind -c "$state/dp-dns-named.conf"
	log "started protected DNS adapter and authoritative BIND"
fi

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
