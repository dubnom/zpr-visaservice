#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
dashboard_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
docs_root=$(CDPATH= cd -- "$dashboard_dir/../.." && pwd)
binary=${ZPR_TRUSTED_SERVICE_BINARY:-$docs_root/.local-runtime/dashboard-stack/zpr-trusted-service-linux-amd64}

if [ -x "$binary" ] &&
   [ -z "$(find "$dashboard_dir/cmd/zpr-trusted-service" "$dashboard_dir/go.mod" "$dashboard_dir/go.sum" -newer "$binary" -print -quit)" ]; then
    exit 0
fi
mkdir -p "$(dirname -- "$binary")"
temporary="$binary.build.$$"
trap 'rm -f -- "$temporary"' EXIT
trap 'exit 1' HUP INT TERM
if command -v go >/dev/null 2>&1; then
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go -C "$dashboard_dir" build -trimpath -o "$temporary" ./cmd/zpr-trusted-service
else
    echo "Building trusted-service helper using the Go builder container"
    docker run --rm \
        -e GOOS=linux -e GOARCH=amd64 -e CGO_ENABLED=0 \
        -v "$dashboard_dir:/src:ro" \
        -v "$(dirname -- "$binary"):/out" \
        -w /src golang:1.26-alpine3.22 \
        go build -trimpath -o "/out/$(basename -- "$temporary")" ./cmd/zpr-trusted-service
fi
chmod 755 "$temporary"
mv "$temporary" "$binary"
