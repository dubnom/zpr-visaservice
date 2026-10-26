#!/bin/sh
# EVALUATION ONLY service probe: zpr-evaluation-probe tcp|udp ZPR-ADDRESS PORT
# Sends a token to an echo listener over ZPR and prints "probe reply" when it
# comes back, otherwise "probe no-reply". The first datagram or SYN may be
# dropped while the node requests a visa, so it tries up to three times.
set -u
[ "$#" -eq 3 ] || { echo "usage: zpr-evaluation-probe tcp|udp ADDRESS PORT" >&2; exit 2; }
protocol=$1 address=$2 port=$3
case $address in
"" | *[!0-9a-f:]*) echo "invalid address" >&2; exit 2 ;;
esac
case $port in
"" | *[!0-9]*) echo "invalid port" >&2; exit 2 ;;
esac
case $protocol in
tcp) target="TCP6:[$address]:$port,connect-timeout=3" ;;
udp) target="UDP6:[$address]:$port" ;;
*) echo "invalid protocol" >&2; exit 2 ;;
esac
token="zpr-probe-$$"
for attempt in 1 2 3; do
	reply=$(printf '%s\n' "$token" | socat -T 2 - "$target" 2>/dev/null) || true
	if [ "$reply" = "$token" ]; then
		echo "probe reply"
		exit 0
	fi
done
echo "probe no-reply"
