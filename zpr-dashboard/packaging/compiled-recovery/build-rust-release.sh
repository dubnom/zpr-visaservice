#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    echo "Usage: sh build-rust-release.sh /absolute/committed-source-root /absolute/new-output" >&2
    exit 2
fi
for directory in "$1" "$2"; do
    case "$directory" in /*) ;; *) echo "Build paths must be absolute" >&2; exit 2 ;; esac
    case "$directory" in
        /|*/|*/.|*/..|*/../*|*/./*) echo "Build paths must not contain dot components or trailing slashes" >&2; exit 2 ;;
    esac
done
if [ "$(uname -s)" != Linux ] || [ "$(uname -m)" != aarch64 ]; then
    echo "Release build requires native Linux ARM64" >&2
    exit 1
fi
sources=$1
for repository in zpr-core zpr-common zpr-visaservice zpr-compiler; do
    test -f "$sources/$repository/Cargo.lock" || { echo "Missing locked source: $repository" >&2; exit 1; }
done
umask 077
mkdir -- "$2"
output=$(CDPATH='' cd -- "$2" && pwd)
mkdir "$output/bin" "$output/receipts"
export CARGO_BUILD_JOBS=2
export CARGO_PROFILE_RELEASE_STRIP=debuginfo
export CARGO_NET_OFFLINE=true
rustc -Vv > "$output/receipts/rustc.txt"
cargo -V > "$output/receipts/cargo.txt"
for repository in zpr-core zpr-common zpr-visaservice zpr-compiler; do
    sha256sum "$sources/$repository/Cargo.lock" >> "$output/receipts/lockfiles.sha256"
done
build_repository() {
    repository=$1
    shift
    export CARGO_TARGET_DIR="$output/target/$repository"
    if ! cargo build --manifest-path "$sources/$repository/Cargo.toml" \
        --locked --offline --release --message-format=json-render-diagnostics "$@" \
        > "$output/receipts/$repository-artifacts.jsonl" \
        2> "$output/receipts/$repository-build.log"; then
        cat "$output/receipts/$repository-build.log" >&2
        echo "Release build failed: $repository (partial output retained)" >&2
        exit 1
    fi
}
build_repository zpr-core --bin ph --bin ph-cli
build_repository zpr-visaservice --bin vs --bin vs-admin --bin vsapikey --bin zpt
build_repository zpr-compiler --bin zplc
for name in ph ph-cli; do cp "$output/target/zpr-core/release/$name" "$output/bin/"; done
for name in vs vs-admin vsapikey zpt; do cp "$output/target/zpr-visaservice/release/$name" "$output/bin/"; done
cp "$output/target/zpr-compiler/release/zplc" "$output/bin/"
chmod 755 "$output/bin/"*
(
    cd "$output"
    sha256sum bin/* > receipts/binaries.sha256
)
printf '%s\n' 'PASS: seven locked, offline, release-profile Linux ARM64 Rust binaries built.' \
    'Runtime CLI, policy compilation and source/archive provenance checks are separate required gates.' \
    > "$output/receipts/result.txt"
cat "$output/receipts/result.txt"
