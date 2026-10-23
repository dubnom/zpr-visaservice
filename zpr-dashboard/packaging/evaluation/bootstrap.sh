#!/bin/sh
set -eu
umask 077
root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$root"
# Keep compiler scratch inside the workspace, not a system temporary directory.
mkdir -p .evaluation-build
export TMPDIR="$root/.evaluation-build"
exec go run ./cmd/zpr-lab-bootstrap -evaluation "$@"
