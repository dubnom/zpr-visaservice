#!/bin/sh
set -eu
umask 077

export GOENV=off GOWORK=off GOFLAGS='' GOTOOLCHAIN=local
export GOPROXY=off GOSUMDB=off GONOPROXY=none GOPRIVATE='' CGO_ENABLED=0
unset GOOS GOARCH GOAMD64

directory=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
dashboard=$(CDPATH='' cd -- "$directory/../../.." && pwd)
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT
trap 'exit 1' HUP INT TERM
archive="$temporary/engineering-bootstrap.tar.gz"
extracted="$temporary/extracted"
mkdir "$extracted"

for script in "$directory/package.sh" "$directory/bootstrap.sh" "$directory/test.sh"; do
    sh -n "$script"
done

(
    cd "$dashboard"
    go test -mod=readonly -count=1 ./cmd/zpr-system-config ./internal/systemconfig
)

"$directory/package.sh" "$archive"
tar -tzf "$archive" > "$temporary/contents"
tar -xzf "$archive" -C "$extracted"
bundle="$extracted/zpr-engineering-bootstrap"

for file in \
    README.md \
    bootstrap.sh \
    company.example.json \
    manifest.json \
    preflight.sh \
    SHA256SUMS \
    zpr-system-config
do
    grep -Fx "zpr-engineering-bootstrap/$file" "$temporary/contents" >/dev/null || {
        printf 'Missing bundle member: %s\n' "$file" >&2
        exit 1
    }
    [ -f "$bundle/$file" ] || {
        printf 'Missing extracted bundle file: %s\n' "$file" >&2
        exit 1
    }
done

LC_ALL=C sort "$temporary/contents" > "$temporary/sorted-contents"
cat > "$temporary/expected-contents" <<'EOF'
zpr-engineering-bootstrap/
zpr-engineering-bootstrap/README.md
zpr-engineering-bootstrap/SHA256SUMS
zpr-engineering-bootstrap/bootstrap.sh
zpr-engineering-bootstrap/company.example.json
zpr-engineering-bootstrap/manifest.json
zpr-engineering-bootstrap/preflight.sh
zpr-engineering-bootstrap/zpr-system-config
EOF
diff -u "$temporary/expected-contents" "$temporary/sorted-contents"
cmp "$directory/../preflight.sh" "$bundle/preflight.sh"
cmp "$directory/../company.example.json" "$bundle/company.example.json"
cmp "$directory/bootstrap.sh" "$bundle/bootstrap.sh"
cmp "$directory/README.md" "$bundle/README.md"
revision=$(git -C "$dashboard" rev-parse HEAD)
grep -F "\"source_revision\": \"$revision\"" "$bundle/manifest.json" >/dev/null
grep -F '"platform": "linux/amd64"' "$bundle/manifest.json" >/dev/null
[ -x "$bundle/zpr-system-config" ]
# Inspect the actual cross-compiled binary without executing foreign code.
[ "$(od -An -tx1 -N6 "$bundle/zpr-system-config" | tr -d ' \n')" = 7f454c460201 ]
[ "$(od -An -tx1 -j18 -N2 "$bundle/zpr-system-config" | tr -d ' \n')" = 3e00 ]

verify_checksums() {
    if command -v sha256sum >/dev/null 2>&1; then
        (cd "$bundle" && sha256sum -c SHA256SUMS)
    else
        (cd "$bundle" && shasum -a 256 -c SHA256SUMS)
    fi
}

verify_checksums
cut -c 67- "$bundle/SHA256SUMS" > "$temporary/checksummed-files"
printf '%s\n' README.md bootstrap.sh company.example.json manifest.json preflight.sh zpr-system-config > "$temporary/expected-checksums"
diff -u "$temporary/expected-checksums" "$temporary/checksummed-files"
printf '\nTAMPER\n' >> "$bundle/company.example.json"
if verify_checksums > "$temporary/tamper.out" 2>&1; then
    echo "Checksums accepted a changed payload." >&2
    exit 1
fi
cp "$directory/../company.example.json" "$bundle/company.example.json"

grep -F '"status": "UNSIGNED/UNQUALIFIED"' "$bundle/manifest.json" >/dev/null
grep -F 'There is no vendor signature or verification root' "$bundle/README.md" >/dev/null
grep -F 'does not install or start any ZPR service' "$bundle/README.md" >/dev/null
if grep -Eiq '\.(pem|key|p12)([[:space:]]|$)' "$temporary/contents"; then
    echo "Unexpected credential or private-key file in the bundle." >&2
    exit 1
fi

if "$directory/package.sh" "$archive" >"$temporary/overwrite.out" 2>"$temporary/overwrite.err"; then
    echo "The package script overwrote an existing bundle." >&2
    exit 1
fi
grep -F 'refusing to overwrite' "$temporary/overwrite.err" >/dev/null

check_status() {
    expected=$1
    shift
    status=0
    "$@" > "$temporary/output" 2>&1 || status=$?
    if [ "$status" -ne "$expected" ]; then
        cat "$temporary/output" >&2
        printf 'Expected status %s; got %s.\n' "$expected" "$status" >&2
        exit 1
    fi
}

ln -s "$temporary/absent" "$temporary/existing.tar.gz"
check_status 1 sh "$directory/package.sh" "$temporary/existing.tar.gz"
[ -L "$temporary/existing.tar.gz" ]
check_status 2 sh "$directory/package.sh"
check_status 1 sh "$directory/package.sh" relative.tar.gz

fixture="$temporary/wrapper fixture"
mkdir "$fixture"
cp "$bundle/bootstrap.sh" "$fixture/"
export TEST_CALLS="$temporary/calls" TEST_CONFIG="$temporary/operator input.json"
export TEST_PREFLIGHT_STATUS=0 TEST_VALIDATOR_STATUS=0
printf '{}\n' > "$TEST_CONFIG"
cp "$TEST_CONFIG" "$temporary/config-before"
cat > "$fixture/preflight.sh" <<'EOF'
#!/bin/sh
set -eu
printf 'preflight\n' >> "$TEST_CALLS"
exit "$TEST_PREFLIGHT_STATUS"
EOF
cat > "$fixture/zpr-system-config" <<'EOF'
#!/bin/sh
set -eu
printf 'validator\n' >> "$TEST_CALLS"
[ "$#" -eq 3 ] && [ "$1" = -config ] && [ "$2" = "$TEST_CONFIG" ] && [ "$3" = -check-files ] || exit 90
exit "$TEST_VALIDATOR_STATUS"
EOF
chmod 700 "$fixture/preflight.sh" "$fixture/zpr-system-config"
: > "$TEST_CALLS"
check_status 0 sh "$fixture/bootstrap.sh" "$TEST_CONFIG"
grep -F 'UNSIGNED/UNQUALIFIED' "$temporary/output" >/dev/null
grep -F 'does NOT install/start services or certify readiness' "$temporary/output" >/dev/null
printf 'preflight\nvalidator\n' > "$temporary/expected-calls"
cmp "$temporary/expected-calls" "$TEST_CALLS"
cmp "$temporary/config-before" "$TEST_CONFIG"
TEST_PREFLIGHT_STATUS=17
: > "$TEST_CALLS"
check_status 17 sh "$fixture/bootstrap.sh" "$TEST_CONFIG"
printf 'preflight\n' > "$temporary/expected-calls"
cmp "$temporary/expected-calls" "$TEST_CALLS"
TEST_PREFLIGHT_STATUS=0 TEST_VALIDATOR_STATUS=19
: > "$TEST_CALLS"
check_status 19 sh "$fixture/bootstrap.sh" "$TEST_CONFIG"
printf 'preflight\nvalidator\n' > "$temporary/expected-calls"
cmp "$temporary/expected-calls" "$TEST_CALLS"
if grep -F 'Engineering checks passed' "$temporary/output" >/dev/null; then
    echo "Failed validation reported success." >&2
    exit 1
fi
: > "$TEST_CALLS"
check_status 2 sh "$fixture/bootstrap.sh"
check_status 2 sh "$fixture/bootstrap.sh" "$TEST_CONFIG" extra
check_status 1 sh "$fixture/bootstrap.sh" "$temporary/missing.json"
[ ! -s "$TEST_CALLS" ]
sh "$directory/../test-preflight.sh"

printf '%s\n' "PASS: exact engineering bundle contents, revision/status, Linux amd64 ELF, checksums, tamper rejection and no-overwrite behavior."
printf '%s\n' "PASS: synthetic wrapper ordering, mandatory -check-files, read-only input and failure/usage status propagation."
printf '%s\n' "Scope: validation only; no host installation, service startup, or fresh-host certification."
