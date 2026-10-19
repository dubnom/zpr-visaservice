#!/bin/sh
set -eu
umask 077

usage() {
    cat <<'EOF'
usage: paired-demo-action.sh
Requires canonical ZPR_DEMO_RELEASE_ROOT and persistent ZPR_DEMO_STATE_ROOT.
Starts the installed paired packages, then runs the Alice-to-web0 ZPR smoke.
Simulator remains UI-only/unprovisioned: no Simulator scenario or readiness
is established by this action.
EOF
}

case "${1:-}" in
    -h|--help)
        [ "$#" -eq 1 ] || { usage >&2; exit 2; }
        usage
        exit 0
        ;;
esac
[ "$#" -eq 0 ] || { usage >&2; exit 2; }

fail() {
    printf 'paired-demo-action: %s\n' "$*" >&2
    exit 1
}

[ -n "${ZPR_DEMO_RELEASE_ROOT:-}" ] || fail "set explicit ZPR_DEMO_RELEASE_ROOT"
[ -n "${ZPR_DEMO_STATE_ROOT:-}" ] || fail "set persistent ZPR_DEMO_STATE_ROOT"
command -v python3 >/dev/null 2>&1 || fail "Python 3 is required to validate installed paths"

# The installed paired lifecycle owns full sibling manifest and state validation.
python3 - "$ZPR_DEMO_RELEASE_ROOT" "$ZPR_DEMO_STATE_ROOT" <<'PY'
import os
from pathlib import Path
import re
import sys


def fail(message):
    raise SystemExit("paired-demo-action: " + message)


roots = []
for name, value in zip(("ZPR_DEMO_RELEASE_ROOT", "ZPR_DEMO_STATE_ROOT"), sys.argv[1:]):
    if (
        value == "/"
        or not re.fullmatch(r"/[A-Za-z0-9_./-]+", value)
        or os.path.normpath(value) != value
        or os.path.realpath(value) != value
    ):
        fail(name + " must be a canonical absolute path without symlinks")
    roots.append(Path(value))
release, state = roots
if release == state or release in state.parents or state in release.parents:
    fail("release and state must be separate non-overlapping trees")
for relative in (
    "simulator/runtime/paired-demo-runtime.sh",
    "zpr-demo/runtime/zpr-demo-runtime.sh",
):
    entrypoint = release / relative
    if (
        os.path.realpath(entrypoint) != str(entrypoint)
        or not entrypoint.is_file()
        or not os.access(entrypoint, os.X_OK)
    ):
        fail("missing or unsafe installed entrypoint: " + relative)
PY

export ZPR_DEMO_RELEASE_ROOT ZPR_DEMO_STATE_ROOT
unset ZPR_DEMO_PACKAGE_DIR ZPR_DEMO_STATE_DIR
unset ZPR_SIMULATOR_PACKAGE_DIR ZPR_SIMULATOR_STATE_DIR

printf '%s\n' "Starting installed paired packages before the ZPR smoke; Simulator remains UI-only/unprovisioned."
if "$ZPR_DEMO_RELEASE_ROOT/simulator/runtime/paired-demo-runtime.sh" start; then
    :
else
    result=$?
    printf '%s\n' "paired-demo-action: paired start failed; smoke was not attempted; persistent state and any started resources are retained." >&2
    exit "$result"
fi

export ZPR_DEMO_PACKAGE_DIR="$ZPR_DEMO_RELEASE_ROOT/zpr-demo"
export ZPR_DEMO_STATE_DIR="$ZPR_DEMO_STATE_ROOT/zpr-demo"
if "$ZPR_DEMO_PACKAGE_DIR/runtime/zpr-demo-runtime.sh" smoke; then
    printf '%s\n' \
        "Alice-to-web0 ZPR smoke passed." \
        "Simulator remains UI-only/unprovisioned; no Simulator scenario or readiness is established."
else
    result=$?
    printf '%s\n' "paired-demo-action: Alice-to-web0 ZPR smoke failed; no smoke success or Simulator readiness is claimed; state and resources are retained." >&2
    exit "$result"
fi
