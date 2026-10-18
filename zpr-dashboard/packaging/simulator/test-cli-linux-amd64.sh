#!/bin/sh
set -eu
umask 077

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)
builder=$script_dir/build-cli-linux-amd64.sh
temporary=$(mktemp -d)
cleanup() {
    rm -r "$temporary"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM

expect_failure() {
    expected_status=$1
    expected_message=$2
    shift 2
    status=0
    "$@" > "$temporary/output" 2>&1 || status=$?
    if [ "$status" -ne "$expected_status" ] ||
        ! grep -F "$expected_message" "$temporary/output" >/dev/null; then
        cat "$temporary/output" >&2
        printf 'FAIL: expected status %s and message: %s (got %s)\n' \
            "$expected_status" "$expected_message" "$status" >&2
        exit 1
    fi
    printf 'PASS: %s\n' "$expected_message"
}

directory_is_empty() {
    [ -z "$(find "$1" -mindepth 1 -print -quit)" ]
}

run_without_go() {
    PATH=$temporary/empty-path /bin/sh "$builder" "$1"
}
run_old_go() {
    PATH="$temporary/old-go-bin:$PATH" /bin/sh "$builder" "$1"
}
run_failing_go() {
    PATH="$temporary/failing-go-bin:$PATH" /bin/sh "$builder" "$temporary/build-failure/output"
}

expect_failure 2 "usage:" /bin/sh "$builder"

mkdir "$temporary/relative"
expect_failure 2 "output path must be absolute" /bin/sh "$builder" relative/output
directory_is_empty "$temporary/relative"

inside_repo=$script_dir/../../../.cli-artifact-test-output
expect_failure 2 "output path must be outside the repository" /bin/sh "$builder" "$inside_repo"
[ ! -e "$inside_repo" ] && [ ! -L "$inside_repo" ]

expect_failure 2 "output parent directory must already exist" \
    /bin/sh "$builder" "$temporary/missing-parent/output"
[ ! -e "$temporary/missing-parent" ]

mkdir "$temporary/no-go" "$temporary/empty-path"
expect_failure 1 "Go 1.26.1 or newer is required" \
    run_without_go "$temporary/no-go/zpr-lab-bootstrap"
directory_is_empty "$temporary/no-go"

mkdir "$temporary/mismatch" "$temporary/old-go-bin"
cat > "$temporary/old-go-bin/go" <<'EOF'
#!/bin/sh
echo "go version go1.26.0 darwin/arm64"
EOF
chmod 700 "$temporary/old-go-bin/go"
expect_failure 1 "Go 1.26.1 or newer stable" run_old_go "$temporary/mismatch/output"
directory_is_empty "$temporary/mismatch"

mkdir "$temporary/existing"
printf 'keep this file unchanged\n' > "$temporary/existing/output"
cp "$temporary/existing/output" "$temporary/original"
expect_failure 2 "output already exists" /bin/sh "$builder" "$temporary/existing/output"
cmp "$temporary/original" "$temporary/existing/output"

mkdir "$temporary/build-failure" "$temporary/failing-go-bin"
cat > "$temporary/failing-go-bin/go" <<'EOF'
#!/bin/sh
case "$1" in
    version)
        echo "go version go1.27.1 darwin/arm64"
        ;;
    build)
        shift
        while [ "$#" -gt 0 ]; do
            if [ "$1" = -o ]; then
                shift
                printf 'partial test artifact\n' > "$1"
                exit 23
            fi
            shift
        done
        exit 24
        ;;
    *)
        exit 25
        ;;
esac
EOF
chmod 700 "$temporary/failing-go-bin/go"
expect_failure 1 "Go build failed; no artifact was published" run_failing_go
directory_is_empty "$temporary/build-failure"

mkdir "$temporary/success"
artifact=$temporary/success/zpr-lab-bootstrap-linux-amd64
/bin/sh "$builder" "$artifact" > "$temporary/success-output"
[ -s "$artifact" ] && [ -x "$artifact" ]
toolchain=$(go version | awk '{sub(/^go/, "", $3); print $3}')
grep -F "Go toolchain: $toolchain;" "$temporary/success-output" >/dev/null
grep -F 'not claimed byte-reproducible' "$temporary/success-output" >/dev/null
magic=$(od -An -tx1 -N4 "$artifact" | tr -d ' \n')
format=$(od -An -tx1 -j4 -N2 "$artifact" | tr -d ' \n')
machine=$(od -An -tx1 -j18 -N2 "$artifact" | tr -d ' \n')
[ "$magic" = 7f454c46 ] && [ "$format" = 0201 ] && [ "$machine" = 3e00 ]
go version -m "$artifact" > "$temporary/build-info"
grep -F ": go$toolchain" "$temporary/build-info" >/dev/null
grep -F 'GOOS=linux' "$temporary/build-info" >/dev/null
grep -F 'GOARCH=amd64' "$temporary/build-info" >/dev/null
grep -F 'GOAMD64=v1' "$temporary/build-info" >/dev/null
grep -F 'CGO_ENABLED=0' "$temporary/build-info" >/dev/null
echo "PASS: produced Linux/amd64 ELF64 and verified embedded Go target metadata without executing it"
