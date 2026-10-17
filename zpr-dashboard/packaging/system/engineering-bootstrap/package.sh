#!/bin/sh
set -eu
umask 077

fail() {
    printf 'FAIL: %s\n' "$1" >&2
    exit 1
}

if [ "$#" -ne 1 ]; then
    echo "usage: sh package.sh /absolute/existing/parent/new-bundle.tar.gz" >&2
    exit 2
fi
output=$1
case "$output" in
    /*.tar.gz) ;;
    *) fail "Output must be an absolute .tar.gz path." ;;
esac
[ ! -e "$output" ] && [ ! -L "$output" ] || fail "Output already exists; refusing to overwrite."
[ -d "$(dirname -- "$output")" ] || fail "Output parent must already exist."

directory=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
dashboard=$(CDPATH='' cd -- "$directory/../../.." && pwd)
revision=$(git -C "$dashboard" rev-parse --verify HEAD)
git -C "$dashboard" diff --quiet HEAD -- . ||
    fail "Tracked dashboard sources differ from HEAD; package only a clean source revision."

export GOENV=off GOWORK=off GOFLAGS='' GOTOOLCHAIN=local
export GOPROXY=off GOSUMDB=off GONOPROXY=none GOPRIVATE=
export CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1
command -v go >/dev/null 2>&1 || fail "Go must already be installed; nothing will be restored."
if command -v sha256sum >/dev/null 2>&1; then
    hash_command=sha256sum
elif command -v shasum >/dev/null 2>&1; then
    hash_command=shasum
else
    fail "sha256sum or shasum is required."
fi

temporary=$(mktemp -d "$(dirname -- "$output")/.zpr-engineering-bootstrap.XXXXXX")
trap 'rm -rf "$temporary"' EXIT
trap 'exit 1' HUP INT TERM
bundle="$temporary/zpr-engineering-bootstrap"
mkdir "$bundle"
printf '%s\n' "Building UNSIGNED/UNQUALIFIED engineering bootstrap with cached dependencies only."
(
    cd "$dashboard"
    go build -mod=readonly -trimpath -buildvcs=false -o "$bundle/zpr-system-config" ./cmd/zpr-system-config
)
cp "$directory/bootstrap.sh" "$directory/README.md" "$bundle/"
cp "$directory/../preflight.sh" "$directory/../company.example.json" "$bundle/"
chmod 755 "$bundle/zpr-system-config" "$bundle/bootstrap.sh" "$bundle/preflight.sh"
chmod 644 "$bundle/README.md" "$bundle/company.example.json"
cat > "$bundle/manifest.json" <<EOF
{
  "schema_version": 1,
  "artifact": "zpr-engineering-bootstrap",
  "status": "UNSIGNED/UNQUALIFIED",
  "platform": "linux/amd64",
  "source_revision": "$revision",
  "source_state": "tracked dashboard sources match HEAD; packaging files may be uncommitted",
  "purpose": "Read-only host preflight and offline management-bootstrap input validation only",
  "limitations": "No services, images, installation, signing, managed control plane or readiness certification"
}
EOF
chmod 644 "$bundle/manifest.json"
(
    cd "$bundle"
    for file in README.md bootstrap.sh company.example.json manifest.json preflight.sh zpr-system-config; do
        if [ "$hash_command" = sha256sum ]; then
            sha256sum "$file"
        else
            shasum -a 256 "$file"
        fi
    done > SHA256SUMS
    chmod 644 SHA256SUMS
)
tar -czf "$temporary/bundle.tar.gz" -C "$temporary" zpr-engineering-bootstrap
# Linking within the destination filesystem publishes without replacing any existing path.
ln "$temporary/bundle.tar.gz" "$output" || fail "Cannot publish bundle; output must not exist."
printf 'Created UNSIGNED/UNQUALIFIED engineering bootstrap: %s\n' "$output"
echo "This is NOT an installer, production/companion bundle or readiness certification."
