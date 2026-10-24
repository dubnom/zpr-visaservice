#!/bin/sh
set -eu
umask 077
root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$root"
mkdir -p .companion-build
export TMPDIR="$root/.companion-build"
exec go run ./cmd/zpr-simulator-companion "$@"
