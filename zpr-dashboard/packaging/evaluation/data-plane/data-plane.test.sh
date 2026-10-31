#!/bin/sh
set -eu
temporary=$(mktemp -d)
unrelated=""
cleanup() {
	if [ -n "$unrelated" ]; then
		kill "$unrelated" 2>/dev/null || true
		wait "$unrelated" 2>/dev/null || true
	fi
	for pid in ${pids:-} ${log_pids:-}; do
		kill -KILL "$pid" 2>/dev/null || true
		wait "$pid" 2>/dev/null || true
	done
	rm -f "$temporary/functions.sh" "$temporary/output.log" "$temporary/state/vs.toml" \
		"$temporary/run/log-normal" "$temporary/run/log-stubborn"
	rmdir "$temporary/run" "$temporary/state" "$temporary"
}
trap cleanup EXIT INT TERM
mkdir "$temporary/run" "$temporary/state"
: >"$temporary/state/vs.toml"
awk '/^log\(\)/ { printing = 1 } /^trap .*INT TERM$/ { exit } printing { print }' \
	"$(dirname "$0")/data-plane.sh" >"$temporary/functions.sh"
. "$temporary/functions.sh"
run=$temporary/run
state=$temporary/state
plane=$state/data-plane
ip() { :; }
chown() { :; }
stat() { printf '0:0\n'; }
sleep 60 &
unrelated=$!
spawn normal sleep 60
spawn stubborn sh -c 'trap "" TERM; echo ready; exec sleep 60' >"$temporary/output.log"
tries=50
while ! grep -q 'ready' "$temporary/output.log"; do
	tries=$((tries - 1))
	[ "$tries" -gt 0 ] || { echo "owned child did not become ready" >&2; exit 1; }
	sleep 0.02
done
owned=$pids
stop
trap cleanup EXIT INT TERM
for pid in $owned $log_pids; do
	if kill -0 "$pid" 2>/dev/null; then
		echo "owned child survived bounded shutdown: $pid" >&2
		exit 1
	fi
done
kill -0 "$unrelated"
pids=""
log_pids=""
echo "Data-plane shutdown reaps owned children; unrelated same-name process survives."
