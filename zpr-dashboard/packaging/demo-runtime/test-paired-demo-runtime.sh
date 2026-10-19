#!/bin/sh
set -eu
umask 077

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/zpr-paired-runtime-test.XXXXXX")
temp_dir=$(CDPATH='' cd -- "$temp_dir" && pwd -P)
trap 'rm -rf "$temp_dir"' EXIT HUP INT TERM
export ZPR_DEMO_RELEASE_ROOT="$temp_dir/install/test-release"
export ZPR_DEMO_STATE_ROOT="$temp_dir/state"
export PAIRED_TEST_LOG="$temp_dir/calls"
export PAIRED_TEST_FAIL=
fake_bin=$temp_dir/bin
mkdir -m 700 "$fake_bin"
cat >"$fake_bin/id" <<'SH'
#!/bin/sh
if [ "$1" = -u ]; then
    printf '%s\n' "${PAIRED_FAKE_UID:-0}"
else
    exec /usr/bin/id "$@"
fi
SH
chmod 755 "$fake_bin/id"
PATH=$fake_bin:$PATH
export PATH
mkdir -p "$ZPR_DEMO_RELEASE_ROOT/zpr-demo/runtime" "$ZPR_DEMO_RELEASE_ROOT/simulator/runtime"

fail() {
    printf 'test-paired-demo-runtime: %s\n' "$*" >&2
    exit 1
}

cat >"$temp_dir/entrypoint" <<'SH'
#!/bin/sh
set -eu
name=${0##*/}
case "$name" in
    init-demo-identities.sh|zpr-demo-runtime.sh)
        [ "$ZPR_DEMO_PACKAGE_DIR" = "$ZPR_DEMO_RELEASE_ROOT/zpr-demo" ]
        [ "$ZPR_DEMO_STATE_DIR" = "$ZPR_DEMO_STATE_ROOT/zpr-demo" ]
        [ "${ZPR_SIMULATOR_PACKAGE_DIR+x}" != x ]
        [ "${ZPR_SIMULATOR_STATE_DIR+x}" != x ]
        ;;
    simulator-demo-runtime.sh)
        [ "$ZPR_SIMULATOR_PACKAGE_DIR" = "$ZPR_DEMO_RELEASE_ROOT/simulator" ]
        [ "$ZPR_SIMULATOR_STATE_DIR" = "$ZPR_DEMO_STATE_ROOT/simulator" ]
        [ "${ZPR_DEMO_PACKAGE_DIR+x}" != x ]
        [ "${ZPR_DEMO_STATE_DIR+x}" != x ]
        ;;
    *) exit 99 ;;
esac
if [ "$name" = init-demo-identities.sh ]; then
    [ "$#" -eq 2 ] && [ "$1" = prepare ] && [ "$2" = "$ZPR_DEMO_STATE_DIR" ]
else
    [ "$#" -eq 1 ]
fi
printf '%s %s\n' "$name" "$1" >>"$PAIRED_TEST_LOG"
[ "$PAIRED_TEST_FAIL" != "$name:$1" ] || exit 7
printf '%s: %s command completed (stub)\n' "$name" "$1"
SH
chmod 755 "$temp_dir/entrypoint"
for entrypoint in \
    zpr-demo/runtime/init-demo-identities.sh \
    zpr-demo/runtime/zpr-demo-runtime.sh \
    simulator/runtime/simulator-demo-runtime.sh; do
    cp "$temp_dir/entrypoint" "$ZPR_DEMO_RELEASE_ROOT/$entrypoint"
done
cp "$script_dir/paired-demo-runtime.sh" "$ZPR_DEMO_RELEASE_ROOT/simulator/runtime/"
wrapper=$ZPR_DEMO_RELEASE_ROOT/simulator/runtime/paired-demo-runtime.sh

python3 - "$ZPR_DEMO_RELEASE_ROOT" <<'PY'
import json
from pathlib import Path
import sys

root = Path(sys.argv[1])
images = {"zpr-demo": "sha256:" + "1" * 64, "simulator": "sha256:" + "2" * 64}
for name, peer in (("zpr-demo", "simulator"), ("simulator", "zpr-demo")):
    manifest = {
        "schema": "zpr.demo.runtime-package.v2",
        "status": "demo-unqualified-runtime-package",
        "release_id": root.name,
        "platform": "linux/amd64",
        "package_id": name + "-image",
        "required_peer_package_id": peer + "-image",
        "peer_image_id": images[peer],
        "peer_docker_image_id": images[peer],
        "component": {
            "id": name + "-image",
            "image_reference": name + ":test-release",
            "image_id": images[name],
            "docker_image_id": images[name],
        },
    }
    package = root / name
    (package / "manifest.json").write_text(json.dumps(manifest) + "\n", encoding="ascii")
    (package / "runtime/image-reference").write_text(images[name] + "\n", encoding="ascii")
PY

# Poison inherited component overrides: the wrapper must replace or unset them.
export ZPR_DEMO_PACKAGE_DIR=/wrong ZPR_DEMO_STATE_DIR=/wrong
export ZPR_SIMULATOR_PACKAGE_DIR=/wrong ZPR_SIMULATOR_STATE_DIR=/wrong

run() {
    : >"$PAIRED_TEST_LOG"
    sh "$wrapper" "$@" >"$temp_dir/output" 2>"$temp_dir/error"
}

expect_failure() {
    if run "$@"; then
        fail "command unexpectedly succeeded: $*"
    fi
}

expect_calls() {
    printf '%s\n' "$@" >"$temp_dir/expected"
    cmp "$temp_dir/expected" "$PAIRED_TEST_LOG" || fail "incorrect command order"
}

expect_no_calls() {
    [ ! -s "$PAIRED_TEST_LOG" ] || fail "component command ran before validation"
}

run --help
grep -F "BOOTSTRAP-ONLY" "$temp_dir/output" >/dev/null
grep -F "not a runnable paired scenario" "$temp_dir/output" >/dev/null
expect_no_calls
export PAIRED_FAKE_UID=1000
for action in start stop status; do
    expect_failure "$action"
    expect_no_calls
done
[ ! -e "$ZPR_DEMO_STATE_ROOT" ] || fail "unauthorized start created persistent state"
unset PAIRED_FAKE_UID
run start
expect_calls "init-demo-identities.sh prepare" "zpr-demo-runtime.sh start" "simulator-demo-runtime.sh start"
grep -F "no scenario or application readiness is established" "$temp_dir/output" >/dev/null
[ -d "$ZPR_DEMO_STATE_ROOT" ] || fail "state root was not created"
printf '%s\n' sentinel >"$ZPR_DEMO_STATE_ROOT/retained"
run status
expect_calls "zpr-demo-runtime.sh status" "simulator-demo-runtime.sh status"
grep -F "Application readiness is not checked" "$temp_dir/output" >/dev/null
run stop
expect_calls "simulator-demo-runtime.sh stop" "zpr-demo-runtime.sh stop"
[ "$(cat "$ZPR_DEMO_STATE_ROOT/retained")" = sentinel ] || fail "stop changed persistent state"

for step in init-demo-identities.sh:prepare zpr-demo-runtime.sh:start simulator-demo-runtime.sh:start; do
    export PAIRED_TEST_FAIL=$step
    expect_failure start
    case "$step" in
        init-demo-identities.sh:prepare) expect_calls "init-demo-identities.sh prepare" ;;
        zpr-demo-runtime.sh:start) expect_calls "init-demo-identities.sh prepare" "zpr-demo-runtime.sh start" ;;
        simulator-demo-runtime.sh:start)
            expect_calls "init-demo-identities.sh prepare" "zpr-demo-runtime.sh start" "simulator-demo-runtime.sh start"
            grep -F "ZPR is not automatically stopped" "$temp_dir/error" >/dev/null
            ;;
    esac
    [ -f "$ZPR_DEMO_STATE_ROOT/retained" ] || fail "failed start removed state"
done
export PAIRED_TEST_FAIL=simulator-demo-runtime.sh:stop
expect_failure stop
expect_calls "simulator-demo-runtime.sh stop" "zpr-demo-runtime.sh stop"
export PAIRED_TEST_FAIL=zpr-demo-runtime.sh:status
expect_failure status
expect_calls "zpr-demo-runtime.sh status" "simulator-demo-runtime.sh status"
export PAIRED_TEST_FAIL=

for name in zpr-demo simulator; do
    manifest=$ZPR_DEMO_RELEASE_ROOT/$name/manifest.json
    mv "$manifest" "$temp_dir/saved-manifest"
    for action in start stop status; do
        expect_failure "$action"
        expect_no_calls
    done
    mv "$temp_dir/saved-manifest" "$manifest"
done
manifest=$ZPR_DEMO_RELEASE_ROOT/simulator/manifest.json
cp "$manifest" "$temp_dir/original-manifest"
for field in release_id required_peer_package_id peer_image_id peer_docker_image_id; do
    python3 - "$temp_dir/original-manifest" "$manifest" "$field" <<'PY'
import json
from pathlib import Path
import sys

manifest = json.loads(Path(sys.argv[1]).read_text())
manifest[sys.argv[3]] = "mismatched"
Path(sys.argv[2]).write_text(json.dumps(manifest))
PY
    for action in start stop status; do
        expect_failure "$action"
        expect_no_calls
    done
done
cp "$temp_dir/original-manifest" "$manifest"

reference=$ZPR_DEMO_RELEASE_ROOT/simulator/runtime/image-reference
cp "$reference" "$temp_dir/original-reference"
printf '%s\n' different-image >"$reference"
expect_failure start
expect_no_calls
cp "$temp_dir/original-reference" "$reference"

original_release=$ZPR_DEMO_RELEASE_ROOT
export ZPR_DEMO_RELEASE_ROOT="$original_release/."
expect_failure status
expect_no_calls
export ZPR_DEMO_RELEASE_ROOT="$original_release"
original_state=$ZPR_DEMO_STATE_ROOT
export ZPR_DEMO_STATE_ROOT=relative-state
expect_failure start
expect_no_calls
export ZPR_DEMO_STATE_ROOT="$original_state"

unset ZPR_DEMO_RELEASE_ROOT
expect_failure status
expect_no_calls
export ZPR_DEMO_RELEASE_ROOT="$original_release"
unset ZPR_DEMO_STATE_ROOT
expect_failure start
expect_no_calls
export ZPR_DEMO_STATE_ROOT="$original_state"

ln -s "$ZPR_DEMO_RELEASE_ROOT" "$temp_dir/release-link"
export ZPR_DEMO_RELEASE_ROOT="$temp_dir/release-link"
expect_failure status
expect_no_calls
export ZPR_DEMO_RELEASE_ROOT="$original_release"
export ZPR_DEMO_STATE_ROOT="$ZPR_DEMO_RELEASE_ROOT/state"
expect_failure start
expect_no_calls
[ ! -e "$ZPR_DEMO_STATE_ROOT" ] || fail "invalid state root was created"
export ZPR_DEMO_STATE_ROOT="$original_state"

for name in zpr-demo simulator; do
    ln -s "$temp_dir" "$ZPR_DEMO_STATE_ROOT/$name"
    expect_failure start
    expect_no_calls
    rm "$ZPR_DEMO_STATE_ROOT/$name"
done

chmod 755 "$ZPR_DEMO_STATE_ROOT"
expect_failure start
expect_no_calls
chmod 700 "$ZPR_DEMO_STATE_ROOT"

printf '%s\n' "Paired bootstrap-only lifecycle tests passed (stub entrypoints only; no Docker, services, or credentials)."
