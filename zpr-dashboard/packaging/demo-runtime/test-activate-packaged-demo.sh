#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/zpr-packaged-demo-activation-test.XXXXXX")
temp_dir=$(CDPATH='' cd -- "$temp_dir" && pwd -P)
cleanup() {
    rm -rf "$temp_dir"
}
trap cleanup EXIT HUP INT TERM

runtime_dir=$temp_dir/runtime
mkdir -m 700 "$runtime_dir"
cp "$script_dir/activate-packaged-demo.sh" "$runtime_dir/"
cat >"$runtime_dir/verify-packaged-demo.sh" <<'SH'
#!/bin/sh
set -eu
[ "$#" -eq 1 ] && [ "$1" = verify-packaged-demo ]
printf '%s\n' "$*" >>"$VERIFY_LOG"
SH

run_activation() {
    SIMULATION_ORGANIZATION_ID=demo \
        SIMULATION_PACKAGED_DEMO_SCRIPT="$runtime_dir/verify-packaged-demo.sh" \
        VERIFY_LOG="$temp_dir/verifier.log" \
        sh "$runtime_dir/activate-packaged-demo.sh" "$@"
}

run_activation reset-organization demo
run_activation restore-base demo
[ "$(cat "$temp_dir/verifier.log")" = "verify-packaged-demo
verify-packaged-demo" ] || {
    echo "activation and restoration did not verify the installed demo" >&2
    exit 1
}

if run_activation reset-organization great-lakes 2>"$temp_dir/unsupported-org.err"; then
    echo "non-demo organization activation unexpectedly succeeded" >&2
    exit 1
fi
grep -F "supports only the demo organization" "$temp_dir/unsupported-org.err" >/dev/null

if SIMULATION_ORGANIZATION_ID=demo \
    SIMULATION_PACKAGED_DEMO_SCRIPT="$temp_dir/other.sh" \
    VERIFY_LOG="$temp_dir/verifier.log" \
    sh "$runtime_dir/activate-packaged-demo.sh" restore-base demo \
    2>"$temp_dir/verifier-path.err"; then
    echo "host-selected verifier path unexpectedly succeeded" >&2
    exit 1
fi
grep -F "path does not match this runtime" "$temp_dir/verifier-path.err" >/dev/null

echo "Packaged demo activation tests passed."
