#!/bin/sh
set -eu
umask 077
fail() { printf 'zpr-simulator: %s\n' "$*" >&2; exit 1; }
root=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
[ "${1:-}" = start ] && [ "$#" -eq 4 ] ||
    fail "usage: ./simulator.sh start MATCHING_ZPR_PACKAGE EVALUATION_STATE NEW_SIMULATOR_STATE"
package=$2
evaluation=$3
state=$4
for path in "$package" "$evaluation" "$state"; do
    case "$path" in /*) ;; *) fail "all paths must be absolute" ;; esac
done
[ "$(uname -s)" = Linux ] && [ "$(uname -m)" = x86_64 ] || fail "Linux x86-64 is required"
command -v sha256sum >/dev/null || fail "sha256sum is required"
command -v docker >/dev/null || fail "Docker CLI is required"
(cd "$root" && sha256sum --check --strict SHA256SUMS) || fail "Simulator package verification failed"
(cd "$package" && sha256sum --check --strict SHA256SUMS) || fail "ZPR package verification failed"
cmp "$root/BUILD-INFO.txt" "$package/BUILD-INFO.txt" >/dev/null ||
    fail "ZPR and Simulator package build identities do not match"
cmp "$root/images.txt" "$package/images.txt" >/dev/null || fail "paired image inventory differs"
image=$(sed -n 's/^data-plane //p' "$root/images.txt")
[ "$(docker image inspect --format '{{.Os}}/{{.Architecture}} {{.Id}}' "$image")" = "linux/amd64 $image" ] ||
    fail "load the matching ZPR package first"
printf 'EVALUATION ONLY: Docker access is root-equivalent. Use only a trusted isolated host.\n'
exec "$package/bin/zpr-lab-bootstrap" -evaluation-simulator \
    -evaluation-state "$evaluation" -state "$state" \
    -binary "$root/bin/zpr-simulator" -data-plane-image "$image"
