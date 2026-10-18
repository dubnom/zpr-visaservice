#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)
builder=$script_dir/build-bundle-tools-linux-amd64.sh
temporary=$(mktemp -d)
cleanup() {
    rm -r "$temporary"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM
mkdir "$temporary/empty-path"

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
    PATH="$temporary/empty-path" /bin/sh "$builder" "$1"
}

run_old_go() {
    PATH="$temporary/old-go-bin:$PATH" /bin/sh "$builder" "$1"
}

run_failing_go() {
    PATH="$temporary/failing-go-bin:$PATH" /bin/sh "$builder" "$temporary/build-failure/output.tar.gz"
}

expect_failure 2 "usage:" /bin/sh "$builder"
mkdir "$temporary/relative"
expect_failure 2 "output path must be absolute" /bin/sh "$builder" relative/tools.tar.gz
directory_is_empty "$temporary/relative"

inside_repo=$script_dir/../../../.bundle-tool-test-output.tar.gz
expect_failure 2 "output path must be outside the repository" /bin/sh "$builder" "$inside_repo"
[ ! -e "$inside_repo" ] && [ ! -L "$inside_repo" ]

expect_failure 2 "output filename must end in .tar.gz" /bin/sh "$builder" "$temporary/output.bin"
expect_failure 2 "output parent directory must already exist" \
    /bin/sh "$builder" "$temporary/missing/tools.tar.gz"
[ ! -e "$temporary/missing" ]

mkdir "$temporary/no-go"
expect_failure 1 "Go 1.26.1 or newer is required" \
    run_without_go "$temporary/no-go/tools.tar.gz"
directory_is_empty "$temporary/no-go"

mkdir "$temporary/old-go-bin" "$temporary/old-go-output"
cat > "$temporary/old-go-bin/go" <<'EOF'
#!/bin/sh
echo "go version go1.26.0 darwin/arm64"
EOF
chmod 700 "$temporary/old-go-bin/go"
expect_failure 1 "Go 1.26.1 or newer stable" run_old_go "$temporary/old-go-output/tools.tar.gz"
directory_is_empty "$temporary/old-go-output"

mkdir "$temporary/existing-output"
printf 'preserve existing archive\n' > "$temporary/existing-output/tools.tar.gz"
cp "$temporary/existing-output/tools.tar.gz" "$temporary/original"
expect_failure 2 "output already exists" \
    /bin/sh "$builder" "$temporary/existing-output/tools.tar.gz"
cmp "$temporary/original" "$temporary/existing-output/tools.tar.gz"

mkdir "$temporary/build-failure" "$temporary/failing-go-bin"
cat > "$temporary/failing-go-bin/go" <<'EOF'
#!/bin/sh
case "$1" in
    version)
        echo "go version go1.27.1 darwin/arm64"
        ;;
    build)
        exit 23
        ;;
    *)
        exit 24
        ;;
esac
EOF
chmod 700 "$temporary/failing-go-bin/go"
expect_failure 1 "Go build failed for zpr-system-bundle" run_failing_go
directory_is_empty "$temporary/build-failure"

mkdir "$temporary/success"
archive=$temporary/success/bundle-tools.tar.gz
/bin/sh "$builder" "$archive" > "$temporary/success-output"
[ -s "$archive" ]
grep -F 'Engineering tooling only' "$temporary/success-output" >/dev/null
tar -tzf "$archive" > "$temporary/archive-list"
for name in zpr-system-bundle zpr-system-bundle-pack zpr-system-bundle-verify zpr-system-bundle-sign zpr-system-config-render SHA256SUMS BUILD-INFO.txt; do
    grep -F "tools/$name" "$temporary/archive-list" >/dev/null
done
tar -xOf "$archive" tools/BUILD-INFO.txt | grep -F 'not a system or companion installer' >/dev/null
mkdir "$temporary/extracted"
tar -xf "$archive" -C "$temporary/extracted"
(
    cd "$temporary/extracted/tools"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum -c SHA256SUMS > "$temporary/checksums"
    else
        shasum -a 256 -c SHA256SUMS > "$temporary/checksums"
    fi
)
for name in zpr-system-bundle zpr-system-bundle-pack zpr-system-bundle-verify zpr-system-bundle-sign zpr-system-config-render; do
    [ -x "$temporary/extracted/tools/$name" ]
    magic=$(od -An -tx1 -N4 "$temporary/extracted/tools/$name" | tr -d ' \n')
    format=$(od -An -tx1 -j4 -N2 "$temporary/extracted/tools/$name" | tr -d ' \n')
    machine=$(od -An -tx1 -j18 -N2 "$temporary/extracted/tools/$name" | tr -d ' \n')
    [ "$magic" = 7f454c46 ] && [ "$format" = 0201 ] && [ "$machine" = 3e00 ]
done
echo "PASS: produced and checksum-verified all five Linux/amd64 bundle tools; no tool was executed"
