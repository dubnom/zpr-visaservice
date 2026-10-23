#!/bin/sh
# Build only the production asset bundle; do not package the source/lab trees.
set -eu
if [ "$#" -ne 1 ]; then
    echo "usage: sh packaging/control-room/build.sh NEW_OUTPUT_DIRECTORY" >&2
    exit 2
fi
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
mkdir -p -- "$(dirname -- "$1")"
mkdir -- "$1"
output=$(CDPATH= cd -- "$1" && pwd)
cd "$root"
go build -trimpath -tags production -o "$output/zpr-control-room" ./cmd/zpr-web-dashboard
cp packaging/control-room/assets.txt "$output/assets.txt"
cp packaging/control-room/embedded-files.txt "$output/embedded-files.txt"
