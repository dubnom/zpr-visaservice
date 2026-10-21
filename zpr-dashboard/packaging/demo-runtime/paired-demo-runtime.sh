#!/bin/sh
set -eu
umask 077

usage() {
    cat <<'EOF'
usage: paired-demo-runtime.sh start|stop|status
Requires explicit canonical ZPR_DEMO_RELEASE_ROOT pointing to the installed
<install-dir>/<release-id>, and absolute persistent ZPR_DEMO_STATE_ROOT.

Starts the ZPR demo and the paired Simulator companion. Runtime identity
is limited to the installed Alice and OciWeb demo resources; the separate
20-machine Simulator fixture remains unprovisioned. This is not production
enrollment or an application-readiness check.
All lifecycle commands require sudo on an isolated disposable demo host because
the persistent state and container workloads are root-owned. start prepares
identities, starts ZPR, then starts Simulator.
stop stops Simulator, then ZPR, preserving persistent state.
status reports both component commands without asserting application readiness.
EOF
}

fail() {
    printf 'paired-demo-runtime: %s\n' "$*" >&2
    exit 1
}

case "${1:-}" in
    -h|--help)
        [ "$#" -eq 1 ] || { usage >&2; exit 2; }
        usage
        exit 0
        ;;
    start|stop|status)
        [ "$#" -eq 1 ] || { usage >&2; exit 2; }
        action=$1
        ;;
    *) usage >&2; exit 2 ;;
esac

[ -n "${ZPR_DEMO_RELEASE_ROOT:-}" ] || fail "set explicit ZPR_DEMO_RELEASE_ROOT"
[ -n "${ZPR_DEMO_STATE_ROOT:-}" ] || fail "set persistent ZPR_DEMO_STATE_ROOT"
command -v id >/dev/null 2>&1 || fail "cannot determine invoking user ID"
[ "$(id -u)" = 0 ] ||
    fail "all lifecycle commands require sudo on the disposable demo host; no component command was run"
command -v python3 >/dev/null 2>&1 || fail "Python 3 is required to validate the installed pair"

if ! python3 - "$ZPR_DEMO_RELEASE_ROOT" "$ZPR_DEMO_STATE_ROOT" <<'PY'
import json
import os
from pathlib import Path
import re
import stat
import sys


def fail(message):
    raise SystemExit("paired-demo-runtime: " + message)


def canonical_path(value, name):
    if (
        value == "/"
        or not re.fullmatch(r"/[A-Za-z0-9_./-]+", value)
        or os.path.normpath(value) != value
        or os.path.realpath(value) != value
    ):
        fail(name + " must be a canonical absolute path without symlinks")
    return Path(value)


def regular_file(path, executable=False):
    if path.is_symlink() or not path.is_file():
        fail("missing or unsafe package file: " + str(path))
    if executable and not os.access(path, os.X_OK):
        fail("package entrypoint is not executable: " + str(path))


def state_owner_uids():
    effective_uid = os.geteuid()
    owners = {0, effective_uid}
    sudo_uid = os.environ.get("SUDO_UID")
    if effective_uid == 0 and sudo_uid is not None:
        if not re.fullmatch(r"(?:0|[1-9][0-9]*)", sudo_uid):
            fail("SUDO_UID must be a canonical non-negative user ID")
        owners.add(int(sudo_uid))
    return owners


release_root = canonical_path(sys.argv[1], "ZPR_DEMO_RELEASE_ROOT")
state_root = canonical_path(sys.argv[2], "ZPR_DEMO_STATE_ROOT")
if not release_root.is_dir():
    fail("installed release root does not exist")
if release_root == state_root or release_root in state_root.parents or state_root in release_root.parents:
    fail("release and state directories must be separate non-overlapping trees")
if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,79}", release_root.name):
    fail("invalid installed release directory name")

allowed_state_owners = state_owner_uids()
for directory in [state_root, *state_root.parents]:
    if not directory.exists():
        if directory == state_root:
            continue
        fail("state root parent must already exist")
    info = directory.lstat()
    if not stat.S_ISDIR(info.st_mode) or directory.is_symlink():
        fail("state path contains a non-directory or symlink")
    if info.st_uid not in allowed_state_owners:
        fail("state ancestors must be owned by root, the invoking user, or SUDO_UID")
    if info.st_mode & 0o022 and not (info.st_uid == 0 and info.st_mode & stat.S_ISVTX):
        fail("state ancestors must not be writable by other users")
    if directory == state_root and (info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) != 0o700):
        fail("existing state root must be invoking-user-owned with mode 0700")

packages = {}
for name, package_id, peer_id, scripts in (
    ("zpr-demo", "zpr-demo-image", "simulator-image",
     ("init-demo-identities.sh", "zpr-demo-runtime.sh")),
    ("simulator", "simulator-image", "zpr-demo-image",
     ("simulator-demo-runtime.sh",)),
):
    package = release_root / name
    runtime = package / "runtime"
    if package.is_symlink() or not package.is_dir() or runtime.is_symlink() or not runtime.is_dir():
        fail("missing or unsafe sibling package: " + name)
    for script in scripts:
        regular_file(runtime / script, executable=True)
    manifest_path = package / "manifest.json"
    reference_path = runtime / "image-reference"
    regular_file(manifest_path)
    regular_file(reference_path)
    try:
        if manifest_path.stat().st_size > 65536 or reference_path.stat().st_size > 256:
            fail("package metadata is too large: " + name)
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        reference = reference_path.read_bytes()
    except (OSError, UnicodeError, ValueError):
        fail("cannot read package metadata: " + name)
    if not isinstance(manifest, dict):
        fail("package manifest must be an object: " + name)
    component = manifest.get("component")
    if (
        manifest.get("schema") != "zpr.demo.runtime-package.v2"
        or manifest.get("status") != "demo-unqualified-runtime-package"
        or manifest.get("platform") != "linux/amd64"
        or manifest.get("release_id") != release_root.name
        or manifest.get("package_id") != package_id
        or manifest.get("required_peer_package_id") != peer_id
        or not isinstance(component, dict)
        or component.get("id") != package_id
    ):
        fail("package does not match the installed release/role: " + name)
    for field in ("image_id", "docker_image_id"):
        value = component.get(field)
        if not isinstance(value, str) or not re.fullmatch(r"sha256:[0-9a-f]{64}", value):
            fail("invalid immutable image identity: " + name)
    image_reference = component.get("image_reference")
    if not isinstance(image_reference, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._/:@+-]{0,254}", image_reference):
        fail("invalid image reference: " + name)
    allowed_references = (image_reference, component["image_id"], component["docker_image_id"])
    if reference not in [(value + "\n").encode("ascii") for value in allowed_references]:
        fail("installed image-reference does not match the package: " + name)
    component_state = state_root / name
    if component_state.is_symlink():
        fail("component state must not be a symlink: " + name)
    if component_state.exists():
        info = component_state.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) != 0o700:
            fail("component state must be an invoking-user-owned mode-0700 directory: " + name)
    packages[name] = manifest

for name, peer in (("zpr-demo", "simulator"), ("simulator", "zpr-demo")):
    for field in ("image_id", "docker_image_id"):
        if packages[name].get("peer_" + field) != packages[peer]["component"][field]:
            fail("sibling package image pairing mismatch: " + name)
PY
then
    fail "installed pair validation failed; no component command was run"
fi

zpr_package=$ZPR_DEMO_RELEASE_ROOT/zpr-demo
simulator_package=$ZPR_DEMO_RELEASE_ROOT/simulator
zpr_state=$ZPR_DEMO_STATE_ROOT/zpr-demo
simulator_state=$ZPR_DEMO_STATE_ROOT/simulator

zpr_command() (
    unset ZPR_SIMULATOR_PACKAGE_DIR ZPR_SIMULATOR_STATE_DIR
    export ZPR_DEMO_PACKAGE_DIR="$zpr_package"
    export ZPR_DEMO_STATE_DIR="$zpr_state"
    "$@"
)

simulator_command() (
    unset ZPR_SIMULATOR_PACKAGE_DIR ZPR_SIMULATOR_STATE_DIR
    export ZPR_SIMULATOR_PACKAGE_DIR="$simulator_package"
    export ZPR_SIMULATOR_STATE_DIR="$simulator_state"
    export ZPR_DEMO_PACKAGE_DIR="$zpr_package"
    export ZPR_DEMO_STATE_DIR="$zpr_state"
    "$simulator_package/runtime/simulator-demo-runtime.sh" "$1"
)

printf '%s\n' \
    "Paired demo runtime: installed ZPR package and Simulator; Simulator world actions are limited to verified Alice and OciWeb demo resources."

case "$action" in
    start)
        if [ ! -d "$ZPR_DEMO_STATE_ROOT" ]; then
            mkdir -m 700 "$ZPR_DEMO_STATE_ROOT" || fail "cannot create private persistent state root"
        fi
        if ! zpr_command "$zpr_package/runtime/init-demo-identities.sh" prepare "$zpr_state"; then
            fail "identity preparation failed; no runtime start was attempted; state is retained for inspection"
        fi
        if ! zpr_command "$zpr_package/runtime/zpr-demo-runtime.sh" start; then
            fail "ZPR start failed; Simulator start was not attempted; state and any started ZPR resources are retained"
        fi
        if ! simulator_command start; then
            fail "Simulator start failed; ZPR is not automatically stopped; state is retained; use status/stop to inspect or stop components"
        fi
        printf '%s\n' "Paired start commands completed; use the packaged scenario checks to observe runtime readiness."
        ;;
    stop|status)
        result=0
        if [ "$action" = stop ]; then
            printf '%s\n' "Simulator component ($action):"
            if ! simulator_command "$action"; then
                printf 'paired-demo-runtime: Simulator %s failed; continuing with ZPR\n' "$action" >&2
                result=1
            fi
        fi
        printf '%s\n' "ZPR component ($action):"
        if ! zpr_command "$zpr_package/runtime/zpr-demo-runtime.sh" "$action"; then
            printf 'paired-demo-runtime: ZPR %s failed\n' "$action" >&2
            result=1
        fi
        if [ "$action" = status ]; then
            printf '%s\n' "Simulator component ($action):"
            if ! simulator_command "$action"; then
                printf 'paired-demo-runtime: Simulator %s failed\n' "$action" >&2
                result=1
            fi
        else
            printf '%s\n' "Persistent state retained; no reset or cleanup was performed."
        fi
        exit "$result"
        ;;
esac
