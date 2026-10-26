#!/bin/sh
# EVALUATION ONLY operator client (U05b): runs the install's client-01 adapter
# without the Simulator, under exactly an endpoint's policy, so its traffic can
# be compared with the Simulator's endpoints in Control Room.
#
#   sh client.sh start  STATE   start client-01 on the install's data plane
#   sh client.sh status STATE   show whether it runs and its ZPR address
#   sh client.sh probe  STATE ping ADDRESS | tcp|udp ADDRESS PORT
#   sh client.sh stop   STATE   remove the client container
#
# STATE is the install directory. The client reads only its own bootstrap key,
# the ZPR CA and data-plane-client.env (parsed, never executed). Install
# teardown also removes the client, like every data-plane member.
set -eu
image=${ZPR_CLIENT_IMAGE:-zpr-evaluation-data-plane:local}
usage() {
	echo "usage: $0 start|status|stop STATE | probe STATE ping ADDRESS | probe STATE tcp|udp ADDRESS PORT" >&2
	exit 2
}
[ "$#" -ge 2 ] || usage
command=$1 state=$2
shift 2
[ -d "$state" ] || { echo "install directory $state not found" >&2; exit 1; }
state=$(CDPATH='' cd -- "$state" && pwd)
description=$state/data-plane-client.env
[ -f "$description" ] && [ ! -L "$description" ] || { echo "$description not found; the install needs -data-plane-endpoints" >&2; exit 1; }

install_id='' network='' node='' name='' zpr_addr='' key='' ca='' listen=''
while IFS= read -r line || [ -n "$line" ]; do
	case $line in
	'' | '#'*) continue ;;
	esac
	value=${line#*=}
	case $line in
	ZPR_CLIENT_INSTALL_ID=*) install_id=$value ;;
	ZPR_CLIENT_NETWORK=*) network=$value ;;
	ZPR_CLIENT_NODE=*) node=$value ;;
	ZPR_CLIENT_NAME=*) name=$value ;;
	ZPR_CLIENT_ZPR_ADDR=*) zpr_addr=$value ;;
	ZPR_CLIENT_KEY=*) key=$value ;;
	ZPR_CLIENT_CA=*) ca=$value ;;
	ZPR_CLIENT_LISTEN=*) listen=$value ;;
	*) echo "unexpected line in $description" >&2; exit 1 ;;
	esac
done <"$description"
case $install_id in
"" | *[!0-9a-f]*) echo "invalid install id" >&2; exit 1 ;;
esac
[ "${#install_id}" -eq 24 ] || { echo "invalid install id" >&2; exit 1; }
expected=zpr-evaluation-$install_id-data-plane
[ "$network" = "$expected" ] && [ "$node" = "$expected" ] || { echo "client network does not belong to its install" >&2; exit 1; }
case $name in
client-[0-9][0-9]) ;;
*) echo "invalid client name" >&2; exit 1 ;;
esac
case $zpr_addr in
"" | *[!0-9a-f:]*) echo "invalid ZPR address" >&2; exit 1 ;;
esac
[ "$key" = "dp-$name-auth.key" ] && [ "$ca" = "dp-ca.crt" ] || { echo "unexpected client key or CA file" >&2; exit 1; }
case $listen in
*[!a-z0-9:\ ]*) echo "invalid listeners" >&2; exit 1 ;;
esac
container=$network-$name

case $command in
start)
	[ "$#" -eq 0 ] || usage
	for file in "$key" "$ca"; do
		[ -f "$state/$file" ] && [ ! -L "$state/$file" ] || { echo "$state/$file is not a regular file" >&2; exit 1; }
	done
	docker network inspect "$network" >/dev/null 2>&1 || { echo "install data plane is not running" >&2; exit 1; }
	docker rm -f "$container" >/dev/null 2>&1 || true
	docker run -d --rm --privileged --init --pull never \
		--network "$network" --name "$container" --hostname "$name" \
		--label "org.zpr.evaluation-install=$install_id" --label "org.zpr.evaluation-client=$name" \
		-e "ZPR_EP_NAME=$name" -e ZPR_EP_KEY=/run/zpr-endpoint/bootstrap.key -e ZPR_EP_CA=/run/zpr-endpoint/ca.crt \
		-e "ZPR_EP_NODE=$node" -e "ZPR_EP_ZPR_ADDR=$zpr_addr" -e "ZPR_EP_LISTEN=$listen" \
		--mount "type=bind,source=$state/$key,target=/run/zpr-endpoint/bootstrap.key,readonly" \
		--mount "type=bind,source=$state/$ca,target=/run/zpr-endpoint/ca.crt,readonly" \
		--entrypoint /usr/local/bin/zpr-evaluation-endpoint "$image" >/dev/null
	echo "started $name as $zpr_addr in $container"
	;;
status)
	[ "$#" -eq 0 ] || usage
	running=$(docker inspect --format '{{.State.Running}}' "$container" 2>/dev/null || echo false)
	echo "$name $zpr_addr running=$running container=$container"
	;;
probe)
	[ "$#" -ge 2 ] || usage
	case $1 in
	ping)
		[ "$#" -eq 2 ] || usage
		case $2 in
		"" | *[!0-9a-f:]*) echo "invalid address" >&2; exit 2 ;;
		esac
		if docker exec "$container" ping -6 -n -q -c 3 -i 0.5 -W 2 "$2" | grep -q ' [1-9][0-9]* received'; then
			echo "probe reply"
		else
			echo "probe no-reply"
		fi
		;;
	tcp | udp)
		[ "$#" -eq 3 ] || usage
		docker exec "$container" zpr-evaluation-probe "$1" "$2" "$3"
		;;
	*) usage ;;
	esac
	;;
stop)
	[ "$#" -eq 0 ] || usage
	docker rm -f "$container" >/dev/null 2>&1 || true
	echo "stopped $name"
	;;
*) usage ;;
esac
