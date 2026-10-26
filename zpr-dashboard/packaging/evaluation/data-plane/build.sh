#!/bin/sh
# EVALUATION ONLY: build the Linux data-plane image (Visa Service, Valkey, one
# node and the VS adapter) from the checked-out ZPR sources. Nothing is pushed.
#
#   sh packaging/evaluation/data-plane/build.sh [-tag zpr-evaluation-data-plane:local]
#
# Sources are mounted read-only; cargo output stays in .evaluation-build.
set -eu
umask 077
tag=zpr-evaluation-data-plane:local
while [ $# -gt 0 ]; do
	case $1 in
	-tag) tag=${2:?-tag needs a value}; shift 2 ;;
	*) echo "usage: $0 [-tag image:tag]" >&2; exit 2 ;;
	esac
done
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
dashboard=$(CDPATH='' cd -- "$here/../../.." && pwd)
visaservice=$(CDPATH='' cd -- "$dashboard/.." && pwd)
workspace=${ZPR_WORKSPACE:-$(CDPATH='' cd -- "$visaservice/../.." && pwd)}
for repository in zpr-common zpr-core zpr-compiler; do
	[ -f "$workspace/$repository/Cargo.toml" ] || {
		echo "missing $workspace/$repository; set ZPR_WORKSPACE to the directory containing the ZPR repositories" >&2
		exit 1
	}
done
build=$dashboard/.evaluation-build/data-plane
mkdir -p "$build/target" "$build/cargo-home" "$build/image"
builder=zpr-evaluation-data-plane-builder:local
docker build --platform linux/arm64 -q -t "$builder" -f "$here/Dockerfile.build" "$here" >/dev/null
docker run --rm --platform linux/arm64 \
	-e CARGO_HOME=/cargo-home -e CARGO_TARGET_DIR=/target \
	-v "$workspace/zpr-common:/src/zpr-common:ro" \
	-v "$workspace/zpr-core:/src/zpr-core:ro" \
	-v "$workspace/zpr-compiler:/src/zpr-compiler:ro" \
	-v "$visaservice:/src/zpr-visaservice:ro" \
	-v "$build/target:/target" -v "$build/cargo-home:/cargo-home" \
	"$builder" sh -eu -c '
		cargo build --locked --release --manifest-path /src/zpr-core/adapter/ph/Cargo.toml
		cargo build --locked --release --manifest-path /src/zpr-core/adapter/cli/Cargo.toml
		cargo build --locked --release --manifest-path /src/zpr-visaservice/Cargo.toml -p vs -p vs-admin
		CARGO_TARGET_DIR=/target/compiler cargo build --locked --release --manifest-path /src/zpr-compiler/Cargo.toml --bin zplc'
for binary in ph ph-cli vs vs-admin; do
	cp "$build/target/release/$binary" "$build/image/$binary"
done
cp "$build/target/compiler/release/zplc" "$build/image/zplc"
cp "$here/Dockerfile" "$here/data-plane.sh" "$build/image/"
docker build --platform linux/arm64 -q -t "$tag" "$build/image" >/dev/null
echo "built $tag"
