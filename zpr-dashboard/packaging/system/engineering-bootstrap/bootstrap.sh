#!/bin/sh
set -eu

fail() {
    printf 'FAIL: %s\n' "$1" >&2
    exit 1
}

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
    echo "usage: bootstrap.sh /path/to/company.json" >&2
    exit 2
fi

directory=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
validator="$directory/zpr-system-config"
[ -x "$validator" ] || fail "The bundled zpr-system-config validator is missing."
[ -r "$1" ] || fail "The supplied company configuration is not readable."

printf '%s\n' "Engineering bootstrap: UNSIGNED/UNQUALIFIED; validation only."
printf '%s\n' "This does NOT install/start services or certify readiness."
"$directory/preflight.sh"
"$validator" -config "$1" -check-files
printf '%s\n' "Engineering checks passed. No configuration was applied; no ZPR services were installed or started."
