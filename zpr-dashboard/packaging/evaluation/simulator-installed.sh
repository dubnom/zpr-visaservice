#!/bin/sh
set -eu
umask 077
fail() { printf 'zpr-simulator: %s\n' "$*" >&2; exit 1; }
root=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
usage="usage: ./simulator.sh start MATCHING_ZPR_PACKAGE EVALUATION_STATE SIMULATOR_STATE
    [--full-catalog] [--simulator-oidc-file FILE] [--retained-machine-runtime-file FILE]
    [--enrollment-device-url HTTPS_URL] [--enrollment-bas-url HTTPS_URL]
    [--enrollment-bas-authority HOST:PORT] [--enrollment-bas-ca-file FILE]"
[ "${1:-}" = start ] && [ "$#" -ge 4 ] || fail "$usage"
package=$2
evaluation=$3
state=$4
shift 4
for path in "$package" "$evaluation" "$state"; do
    case "$path" in /*) ;; *) fail "all paths must be absolute" ;; esac
done
# Optional paired-operation configuration is operator-owned and passed
# through verbatim; the bootstrap revalidates every file and contract.
full_catalog=false
oidc_file=
retained_file=
device_url=
bas_url=
bas_authority=
bas_ca_file=
option_file() {
    [ "$#" -ge 2 ] || fail "$1 requires an absolute existing file"
    case "$2" in /*) ;; *) fail "$1 requires an absolute path" ;; esac
    [ -f "$2" ] || fail "$1 file does not exist: $2"
}
option_url() {
    [ "$#" -ge 2 ] || fail "$1 requires an HTTPS URL"
    case "$2" in https://?*) ;; *) fail "$1 requires an HTTPS URL" ;; esac
}
while [ "$#" -gt 0 ]; do
    case "$1" in
        --full-catalog) full_catalog=true; shift ;;
        --simulator-oidc-file) option_file "$1" "${2:-}"; oidc_file=$2; shift 2 ;;
        --retained-machine-runtime-file) option_file "$1" "${2:-}"; retained_file=$2; shift 2 ;;
        --enrollment-bas-ca-file) option_file "$1" "${2:-}"; bas_ca_file=$2; shift 2 ;;
        --enrollment-device-url) option_url "$1" "${2:-}"; device_url=$2; shift 2 ;;
        --enrollment-bas-url) option_url "$1" "${2:-}"; bas_url=$2; shift 2 ;;
        --enrollment-bas-authority)
            [ "$#" -ge 2 ] && [ -n "${2:-}" ] || fail "$1 requires a host:port authority"
            bas_authority=$2; shift 2 ;;
        *) fail "unsupported option $1
$usage" ;;
    esac
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
set --
if [ "$full_catalog" = true ]; then set -- "$@" -catalog "$root/catalog"; fi
if [ -n "$oidc_file" ]; then set -- "$@" -simulator-oidc-file "$oidc_file"; fi
if [ -n "$retained_file" ]; then set -- "$@" -retained-machine-runtime-file "$retained_file"; fi
if [ -n "$device_url" ]; then set -- "$@" -enrollment-device-url "$device_url"; fi
if [ -n "$bas_url" ]; then set -- "$@" -enrollment-bas-url "$bas_url"; fi
if [ -n "$bas_authority" ]; then set -- "$@" -enrollment-bas-authority "$bas_authority"; fi
if [ -n "$bas_ca_file" ]; then set -- "$@" -enrollment-bas-ca-file "$bas_ca_file"; fi
exec "$package/bin/zpr-lab-bootstrap" -evaluation-simulator \
    -evaluation-state "$evaluation" -state "$state" \
    -binary "$root/bin/zpr-simulator" -data-plane-image "$image" "$@"
