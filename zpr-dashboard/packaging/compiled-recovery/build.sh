#!/bin/sh
set -eu

if [ "$#" -ne 1 ] && [ "$#" -ne 3 ]; then
    echo "Usage: sh packaging/compiled-recovery/build.sh /absolute/new-output-directory [/absolute/rust-release /absolute/release-inputs.json]" >&2
    exit 2
fi
case "$1" in
    /*) ;;
    *) echo "Output must be an absolute path to a new directory" >&2; exit 2 ;;
esac
case "$1" in
    /|*/|*/.|*/..|*/../*|*/./*)
        echo "Output must be a new directory without dot components or a trailing slash" >&2
        exit 2
        ;;
esac
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
dashboard_dir=$(CDPATH='' cd -- "$script_dir/../.." && pwd)
repo_dir=$(git -C "$dashboard_dir" rev-parse --show-toplevel)
if [ -n "$(git -C "$repo_dir" status --porcelain)" ]; then
    echo "Build requires a clean committed candidate; no files were written" >&2
    exit 1
fi
GO=${GO:-go}
for tool in "$GO" git jq tar gzip shasum; do
    command -v "$tool" >/dev/null 2>&1 || { echo "Required build tool unavailable: $tool" >&2; exit 1; }
done
if [ "$#" -eq 3 ]; then
    command -v node >/dev/null 2>&1 || { echo "Rust assembly requires Node.js" >&2; exit 1; }
    node "$script_dir/verify-rust-release.cjs" "$2" "$3" >/dev/null
fi
umask 077
mkdir -- "$1"
output=$(CDPATH='' cd -- "$1" && pwd)
stage="$output/runtime"
mkdir -p "$stage/bin" "$stage/share/examples" "$stage/licenses"
cd "$dashboard_dir"
for name in zpr-web-dashboard zpr-trusted-service zpr-lab-bootstrap zpr-system-config; do
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 "$GO" build -trimpath -ldflags '-s -w' -o "$stage/bin/$name" "./cmd/$name"
done
git ls-files cmd/zpr-web-dashboard/examples > "$output/assets.list"
while IFS= read -r asset; do
    case "$asset" in
        *.json|*.ldif|*.zpl|*.zplc) ;;
        *.md) continue ;;
        *) echo "Unreviewed asset type: $asset" >&2; exit 1 ;;
    esac
    if [ ! -f "$asset" ] || [ -L "$asset" ]; then
        echo "Runtime asset must be a regular file: $asset" >&2
        exit 1
    fi
    relative=${asset#cmd/zpr-web-dashboard/examples/}
    mkdir -p "$stage/share/examples/$(dirname -- "$relative")"
    cp "$asset" "$stage/share/examples/$relative"
done < "$output/assets.list"
cp "$repo_dir/LICENSE" "$stage/licenses/PROJECT-LICENSE"
cp cmd/zpr-web-dashboard/static/ansi_up.LICENSE "$stage/licenses/"
cp cmd/zpr-web-dashboard/static/vendor/CODEMIRROR-LICENSES "$stage/licenses/"
cp cmd/zpr-web-dashboard/static/vendor/PLURALIZE-LICENSE "$stage/licenses/"
cp "$script_dir/README.md" "$stage/README.md"
jq -n --arg revision "$(git rev-parse HEAD)" --arg toolchain "$("$GO" version)" \
    '{schema_version:1,kind:"compiled-recovery-engineering-slice",target:"linux/arm64",
      source_revision:$revision,go_toolchain:$toolchain,public_release:false,
      complete_installation:false,dependency_license_review:"pending"}' > "$stage/build.json"
"$GO" version -m "$stage/bin/zpr-web-dashboard" > "$stage/go-build-info.txt"
if [ "$#" -eq 3 ]; then
    node "$script_dir/assemble-rust-release.cjs" "$2" "$3" "$stage"
fi
find "$stage" -type d -exec chmod 755 {} +
find "$stage" -type f -exec chmod 644 {} +
chmod 755 "$stage/bin/"*
cd "$stage"
find . -type f ! -name SHA256SUMS | LC_ALL=C sort | while IFS= read -r file; do
    shasum -a 256 "$file"
done > SHA256SUMS
chmod 644 SHA256SUMS
COPYFILE_DISABLE=1 tar --numeric-owner -czf "$output/compiled-recovery-linux-arm64.tar.gz" .
cd "$output"
shasum -a 256 compiled-recovery-linux-arm64.tar.gz > compiled-recovery-linux-arm64.tar.gz.sha256
printf 'Engineering artifact: %s/compiled-recovery-linux-arm64.tar.gz\n' "$output"
printf '%s\n' 'Not a complete installer or public release. Fresh runtime identities are not included.'
