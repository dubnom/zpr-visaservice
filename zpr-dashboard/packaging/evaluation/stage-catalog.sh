#!/bin/sh
set -eu
umask 077

fail() { printf 'evaluation-catalog: %s\n' "$*" >&2; exit 1; }
[ "$#" -eq 1 ] || fail "usage: stage-catalog.sh NEW_ABSOLUTE_CATALOG_DIRECTORY"
catalog=$1
case "$catalog" in /*) ;; *) fail "catalog destination must be absolute" ;; esac
[ ! -e "$catalog" ] && [ ! -L "$catalog" ] || fail "catalog destination already exists"
root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
mkdir -m 700 "$catalog"
cp -R "$root/packaging/demo-runtime/organizations" "$root/packaging/demo-runtime/scenarios" "$catalog/"
cp "$root/packaging/demo-runtime/simulation-environment.json" "$catalog/manifest.json"
# Northstar already has a packaged compiler configuration. The other original
# profiles reference source templates that were absent from the companion.
for organization in great-lakes load-lab redwood velocity; do
    cp "$root/cmd/zpr-web-dashboard/examples/organizations/$organization/policy-demo.zplc" \
        "$catalog/organizations/$organization/policy-demo.zplc"
done
for organization in northstar great-lakes load-lab redwood velocity; do
    cp "$root/cmd/zpr-web-dashboard/examples/organizations/$organization/runtime-policy.zpl" \
        "$catalog/organizations/$organization/runtime-policy.zpl"
done
