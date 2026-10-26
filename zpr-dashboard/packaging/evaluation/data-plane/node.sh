#!/bin/sh
# EVALUATION ONLY additional node launcher (alternate entrypoint of the
# data-plane image). Runs one ZPR node on the install's Docker network; its
# dp-node<N>.toml advertises the node's fixed substrate address and pins every
# other node as a Noise peer. It reaches the Visa Service only over its links.
#
# Inputs (set by zpr-lab-bootstrap): ZPR_NODE_STATE (state dir; only this
# node's identity, the CA and peer certificates are mounted, read-only),
# ZPR_NODE_INDEX (1-7) and ZPR_NODE_ZPR_ADDR (this node's ZPR address).
set -eu
state=${ZPR_NODE_STATE:?ZPR_NODE_STATE is required}
index=${ZPR_NODE_INDEX:?ZPR_NODE_INDEX is required}
zpr_addr=${ZPR_NODE_ZPR_ADDR:?ZPR_NODE_ZPR_ADDR is required}
case $index in
[1-7]) ;;
*) echo "invalid node index" >&2; exit 2 ;;
esac

printf '%s [node%s] starting as %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$index" "$zpr_addr"
mkdir -p /run/zpr
ip tuntap add name tun0 mode tun multi_queue
ip addr add "$zpr_addr/32" dev tun0
ip link set tun0 up
exec ph node \
	--logging all=INFO \
	--control-path /run/zpr/node.sock \
	--capture-path /run/zpr/node_cap.sock \
	--config-file "$state/dp-node$index.toml" \
	--ca-file "$state/dp-ca.crt" \
	--certificate-file "$state/dp-node$index.crt" \
	--private-key-file "$state/dp-node$index.key" \
	--auth-private-key "$state/dp-node$index-auth.key" \
	--km-impl noise \
	--tun-if tun0 \
	--zpr-addr "$zpr_addr"
