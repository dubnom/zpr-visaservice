#!/bin/sh
set -eu

dashboard=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
workspace=$(CDPATH= cd -- "$dashboard/.." && pwd)
: "${ZPR_TEST_ZPLC:?Set ZPR_TEST_ZPLC to the source-built source-qualified compiler}"
ZPR_TEST_ZPLC=$(python3 -c 'import os,sys; print(os.path.abspath(sys.argv[1]))' "$ZPR_TEST_ZPLC")
export ZPR_TEST_ZPLC
test -x "$ZPR_TEST_ZPLC" || { echo "ZPR_TEST_ZPLC must be executable" >&2; exit 1; }

if [ -z "${ZPR_TEST_VS_BINARY:-}" ]; then
    build_log=$(mktemp "${TMPDIR:-/tmp}/zpr-source-build.XXXXXX")
    trap 'rm -f "$build_log"' 0
    trap 'exit 1' HUP INT TERM
    cd "$workspace"
    # A normal checkout needs its configured zpr-common sibling. Worktree users
    # may supply an already built test executable; never rewrite manifests here.
    if ! cargo test -p vs --bin vs --no-run --message-format=json > "$build_log"; then
        python3 - "$build_log" <<'PY'
import json
import sys

with open(sys.argv[1]) as messages:
    for row in map(json.loads, messages):
        if row.get("reason") == "compiler-message":
            print(row["message"].get("rendered") or row["message"]["message"], file=sys.stderr)
PY
        exit 1
    fi
    ZPR_TEST_VS_BINARY=$(python3 - "$build_log" <<'PY'
import json
import sys

binaries = [
    row["executable"]
    for row in map(json.loads, open(sys.argv[1]))
    if row.get("reason") == "compiler-artifact"
    and row.get("target", {}).get("name") == "vs"
    and row.get("profile", {}).get("test")
    and row.get("executable")
]
if len(binaries) != 1:
    raise SystemExit("Expected exactly one VS test executable")
print(binaries[0])
PY
    )
fi
ZPR_TEST_VS_BINARY=$(python3 -c 'import os,sys; print(os.path.abspath(sys.argv[1]))' "$ZPR_TEST_VS_BINARY")
export ZPR_TEST_VS_BINARY
test -x "$ZPR_TEST_VS_BINARY" || { echo "ZPR_TEST_VS_BINARY must be a current VS test executable" >&2; exit 1; }
cd "$dashboard"
go test ./cmd/zpr-web-dashboard -run '^TestPolicyDocumentsSourceLifecycleInterop$' -count=1 -v -timeout 4m
