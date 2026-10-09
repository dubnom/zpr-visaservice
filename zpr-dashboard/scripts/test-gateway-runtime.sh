#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
dashboard_dir=$(CDPATH='' cd -- "$script_dir/.." && pwd)
temporary=$(mktemp -d "${TMPDIR:-/tmp}/zpr-gateway-check.XXXXXX")
trap 'rm -f "$temporary/zpr-web-dashboard"; rmdir "$temporary"' EXIT
cd "$dashboard_dir"
go build -trimpath -o "$temporary/zpr-web-dashboard" ./cmd/zpr-web-dashboard
ZPR_GATEWAY_TEST_BINARY="$temporary/zpr-web-dashboard" \
    go test ./cmd/zpr-web-dashboard -run 'Test.*Gateway' -count=1
