#!/bin/sh
set -eu

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
    echo "usage: serve-lan.sh private-lan-ipv4 [port]" >&2
    exit 2
fi
lan_address=$1
port=${2:-8090}
python3 - "$lan_address" "$port" <<'PY'
import ipaddress
import sys

address = ipaddress.ip_address(sys.argv[1])
port = int(sys.argv[2])
private_networks = tuple(map(ipaddress.ip_network, ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16")))
if address.version != 4 or not any(address in network for network in private_networks):
    raise SystemExit("bind address must be a private LAN IPv4 address")
if not 1024 <= port <= 65535:
    raise SystemExit("port must be between 1024 and 65535")
PY
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
site_dir=${ZPR_ENROLLMENT_PUBLIC_ROOT:-$script_dir}
[ -d "$site_dir" ] || { echo "site directory does not exist: $site_dir" >&2; exit 1; }
echo "Serving static enrollment page on http://$lan_address:$port (development LAN only; no enrollment API or email sender)" >&2
exec python3 -m http.server "$port" --bind "$lan_address" --directory "$site_dir"