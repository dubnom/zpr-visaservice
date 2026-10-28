#!/bin/sh
set -eu

fail() {
    printf 'activate-packaged-demo: %s\n' "$*" >&2
    exit 1
}

[ "$#" -eq 2 ] || fail "usage: activate-packaged-demo.sh reset-organization|restore-base demo"
case "$1" in
    reset-organization|restore-base) ;;
    *) fail "unsupported organization operation" ;;
esac
[ "$2" = demo ] || fail "the packaged runtime supports only the demo organization"
[ "${SIMULATION_ORGANIZATION_ID:-}" = demo ] ||
    fail "the installed Simulator profile is not the demo organization"

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P) ||
    fail "cannot resolve the installed runtime directory"
verifier=$script_dir/verify-packaged-demo.sh
[ -f "$verifier" ] && [ ! -L "$verifier" ] ||
    fail "the packaged demo verifier is missing or unsafe"
[ "${SIMULATION_PACKAGED_DEMO_SCRIPT:-}" = "$verifier" ] ||
    fail "the packaged demo verifier path does not match this runtime"

sh "$verifier" verify-packaged-demo
