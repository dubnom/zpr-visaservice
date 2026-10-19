#!/bin/sh
set -eu
umask 077

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/zpr-paired-action-test.XXXXXX")
temp_dir=$(CDPATH='' cd -- "$temp_dir" && pwd -P)
trap 'rm -rf "$temp_dir"' EXIT HUP INT TERM
export ZPR_DEMO_RELEASE_ROOT="$temp_dir/install/test-release"
export ZPR_DEMO_STATE_ROOT="$temp_dir/state"
export ACTION_TEST_LOG="$temp_dir/calls"
export ACTION_TEST_START_RESULT=0 ACTION_TEST_SMOKE_RESULT=0
mkdir -p "$ZPR_DEMO_RELEASE_ROOT/simulator/runtime" "$ZPR_DEMO_RELEASE_ROOT/zpr-demo/runtime"

fail() {
    printf 'test-paired-demo-action: %s\n' "$*" >&2
    exit 1
}

cat >"$temp_dir/entrypoint" <<'SH'
#!/bin/sh
set -eu
[ "$#" -eq 1 ]
[ "${ZPR_SIMULATOR_PACKAGE_DIR+x}" != x ]
[ "${ZPR_SIMULATOR_STATE_DIR+x}" != x ]
case "${0##*/}" in
    paired-demo-runtime.sh)
        [ "$1" = start ]
        [ "${ZPR_DEMO_PACKAGE_DIR+x}" != x ]
        [ "${ZPR_DEMO_STATE_DIR+x}" != x ]
        printf '%s\n' start >>"$ACTION_TEST_LOG"
        exit "$ACTION_TEST_START_RESULT"
        ;;
    zpr-demo-runtime.sh)
        [ "$1" = smoke ]
        [ "$ZPR_DEMO_PACKAGE_DIR" = "$ZPR_DEMO_RELEASE_ROOT/zpr-demo" ]
        [ "$ZPR_DEMO_STATE_DIR" = "$ZPR_DEMO_STATE_ROOT/zpr-demo" ]
        printf '%s\n' smoke >>"$ACTION_TEST_LOG"
        exit "$ACTION_TEST_SMOKE_RESULT"
        ;;
    *) exit 99 ;;
esac
SH
chmod 755 "$temp_dir/entrypoint"
cp "$temp_dir/entrypoint" "$ZPR_DEMO_RELEASE_ROOT/simulator/runtime/paired-demo-runtime.sh"
cp "$temp_dir/entrypoint" "$ZPR_DEMO_RELEASE_ROOT/zpr-demo/runtime/zpr-demo-runtime.sh"
cp "$script_dir/paired-demo-action.sh" "$ZPR_DEMO_RELEASE_ROOT/simulator/runtime/"
wrapper=$ZPR_DEMO_RELEASE_ROOT/simulator/runtime/paired-demo-action.sh
export ZPR_DEMO_PACKAGE_DIR=/wrong ZPR_DEMO_STATE_DIR=/wrong
export ZPR_SIMULATOR_PACKAGE_DIR=/wrong ZPR_SIMULATOR_STATE_DIR=/wrong

run() {
    : >"$ACTION_TEST_LOG"
    sh "$wrapper" "$@" >"$temp_dir/output" 2>"$temp_dir/error"
}

expect_failure() {
    if run "$@"; then
        fail "unexpected success"
    fi
    if grep -F 'Alice-to-web0 ZPR smoke passed.' "$temp_dir/output" >/dev/null; then
        fail "failure reported smoke success"
    fi
}

expect_calls() {
    printf '%s\n' "$@" >"$temp_dir/expected"
    cmp "$temp_dir/expected" "$ACTION_TEST_LOG" || fail "incorrect call order"
}

expect_no_calls() {
    [ ! -s "$ACTION_TEST_LOG" ] || fail "entrypoint ran before validation"
}

run --help
grep -F 'UI-only/unprovisioned' "$temp_dir/output" >/dev/null
expect_no_calls
expect_failure unexpected
expect_no_calls
run
expect_calls start smoke
grep -Fx 'Alice-to-web0 ZPR smoke passed.' "$temp_dir/output" >/dev/null
grep -F 'Simulator remains UI-only/unprovisioned; no Simulator scenario or readiness is established.' "$temp_dir/output" >/dev/null
[ ! -e "$ZPR_DEMO_STATE_ROOT" ] || fail "wrapper created state instead of delegating"

export ACTION_TEST_START_RESULT=7
if run; then fail "start failure was ignored"; else result=$?; fi
[ "$result" -eq 7 ] || fail "start exit status was not propagated"
expect_calls start
grep -F 'smoke was not attempted' "$temp_dir/error" >/dev/null
export ACTION_TEST_START_RESULT=0 ACTION_TEST_SMOKE_RESULT=9
if run; then fail "smoke failure was ignored"; else result=$?; fi
[ "$result" -eq 9 ] || fail "smoke exit status was not propagated"
expect_calls start smoke
if grep -F 'Alice-to-web0 ZPR smoke passed.' "$temp_dir/output" >/dev/null; then
    fail "failed smoke reported success"
fi
export ACTION_TEST_SMOKE_RESULT=0

original_release=$ZPR_DEMO_RELEASE_ROOT
original_state=$ZPR_DEMO_STATE_ROOT
unset ZPR_DEMO_RELEASE_ROOT
expect_failure
expect_no_calls
export ZPR_DEMO_RELEASE_ROOT="$original_release"
unset ZPR_DEMO_STATE_ROOT
expect_failure
expect_no_calls
for invalid in relative-state "$original_release/state" "$original_state/."; do
    export ZPR_DEMO_STATE_ROOT="$invalid"
    expect_failure
    expect_no_calls
done
export ZPR_DEMO_STATE_ROOT="$original_state"
export ZPR_DEMO_RELEASE_ROOT="$original_release/."
expect_failure
expect_no_calls
export ZPR_DEMO_RELEASE_ROOT="$original_release"
ln -s "$temp_dir" "$original_state"
expect_failure
expect_no_calls
rm "$original_state"

for relative in simulator/runtime/paired-demo-runtime.sh zpr-demo/runtime/zpr-demo-runtime.sh; do
    entrypoint=$original_release/$relative
    mv "$entrypoint" "$temp_dir/saved-entrypoint"
    expect_failure
    expect_no_calls
    ln -s "$temp_dir/saved-entrypoint" "$entrypoint"
    expect_failure
    expect_no_calls
    rm "$entrypoint"
    mv "$temp_dir/saved-entrypoint" "$entrypoint"
done

printf '%s\n' "Paired action orchestration tests passed (stub entrypoints only; no Docker, services, or credentials)."
