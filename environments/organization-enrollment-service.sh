#!/bin/sh
set -eu
umask 077
fail() { printf 'organization-enrollment-service: %s\n' "$*" >&2; exit 1; }
[ "$#" -eq 1 ] || fail "usage: organization-enrollment-service.sh ORGANIZATION"
organization=$1
case "$organization" in
    northstar|great-lakes|load-lab|redwood|velocity) ;;
    *) fail "unknown operator organization" ;;
esac
root=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
(cd "$root" && sha256sum --check --strict SHA256SUMS >/dev/null) ||
    fail "operator binaries or immutable inputs changed"
config=$root/enrollment/$organization/service.json
if [ -f "$root/enrollment/$organization/service-development.json" ]; then
    config=$root/enrollment/$organization/service-development.json
fi
exec env -i PATH="$PATH" HOME="$HOME" \
    "$root/bin/zpr-enrollment-service" \
    -config "$config"
