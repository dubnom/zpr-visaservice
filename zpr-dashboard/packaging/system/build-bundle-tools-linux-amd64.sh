#!/bin/sh
set -eu
umask 022

fail() {
    status=$1
    shift
    printf 'ERROR: %s\n' "$*" >&2
    exit "$status"
}

if [ "$#" -ne 1 ]; then
    echo "usage: sh build-bundle-tools-linux-amd64.sh /absolute/path/to/new-archive.tar.gz" >&2
    exit 2
fi

requested=$1
case "$requested" in
    /*) ;;
    *) fail 2 "output path must be absolute" ;;
esac
filename=${requested##*/}
case "$filename" in
    ''|.|..) fail 2 "output path must name a new archive file" ;;
esac
case "$filename" in
    *.tar.gz) ;;
    *) fail 2 "output filename must end in .tar.gz" ;;
esac
if [ -e "$requested" ] || [ -L "$requested" ]; then
    fail 2 "output already exists; refusing to overwrite it"
fi

command -v go >/dev/null 2>&1 ||
    fail 1 "Go 1.26.1 or newer is required; no toolchain will be downloaded"

for tool in awk dirname ln mktemp od rm tar tr uname; do
    command -v "$tool" >/dev/null 2>&1 ||
        fail 1 "required build utility is missing: $tool"
done
if command -v sha256sum >/dev/null 2>&1; then
    checksum_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
    checksum_tool=shasum
else
    fail 1 "sha256sum or shasum is required to record tool checksums"
fi

if ! go_info=$(GOTOOLCHAIN=local GOENV=off go version 2>&1); then
    fail 1 "cannot query the local Go toolchain: $go_info"
fi
version_token=${go_info#go version }
if [ "$version_token" = "$go_info" ]; then
    fail 1 "unrecognized Go version output: $go_info"
fi
go_version=${version_token#go}
go_version=${go_version%% *}
if ! printf '%s\n' "$go_version" | awk -F. '
    NF != 3 || $1 !~ /^[0-9]+$/ || $2 !~ /^[0-9]+$/ || $3 !~ /^[0-9]+$/ {
        exit 1
    }
    {
        if ($1 + 0 > 1) exit 0
        if ($1 + 0 < 1) exit 1
        if ($2 + 0 > 26) exit 0
        if ($2 + 0 < 26) exit 1
        if ($3 + 0 >= 1) exit 0
        exit 1
    }
'; then
    fail 1 "Go 1.26.1 or newer stable is required by go.mod; found ${go_version:-unrecognized}"
fi

host=$(uname -s) || fail 1 "cannot identify the build host"
case "$host" in
    Darwin|Linux) ;;
    *) fail 1 "build host must be macOS or Linux, not $host" ;;
esac

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P) ||
    fail 1 "cannot resolve the packaging script directory"
module_root=$(CDPATH='' cd -- "$script_dir/../.." && pwd -P) ||
    fail 1 "cannot resolve the Go module directory"
repository_root=$(CDPATH='' cd -- "$module_root/.." && pwd -P) ||
    fail 1 "cannot resolve the repository directory"
output_parent=${requested%/*}
[ -n "$output_parent" ] || output_parent=/
if [ ! -d "$output_parent" ]; then
    fail 2 "output parent directory must already exist"
fi
output_parent=$(CDPATH='' cd -P -- "$output_parent" && pwd -P) ||
    fail 2 "cannot resolve output parent directory"
case "$output_parent" in
    "$repository_root"|"$repository_root"/*)
        fail 2 "output path must be outside the repository"
        ;;
esac
destination=$output_parent/$filename
if [ -e "$destination" ] || [ -L "$destination" ]; then
    fail 2 "output already exists; refusing to overwrite it"
fi

temporary=$(mktemp -d "$output_parent/.${filename}.build.XXXXXX") ||
    fail 1 "cannot create a temporary build directory beside the requested output"
cleanup() {
    if [ -n "${temporary:-}" ] && [ -d "$temporary" ]; then
        rm -r "$temporary"
    fi
}
trap cleanup 0
trap 'exit 1' HUP INT TERM
mkdir "$temporary/tools"

build_tool() {
    name=$1
    package=$2
    artifact=$temporary/tools/$name
    if ! (
        cd "$module_root"
        GOENV=off GOTOOLCHAIN=local GOPROXY=off GOWORK=off GOFLAGS= \
            GOOS=linux GOARCH=amd64 GOAMD64=v1 GOEXPERIMENT= CGO_ENABLED=0 \
            go build -mod=readonly -trimpath -buildvcs=false -o "$artifact" "$package"
    ); then
        fail 1 "Go build failed for $name; no archive was published"
    fi
    if [ ! -f "$artifact" ] || [ ! -s "$artifact" ] || [ ! -x "$artifact" ]; then
        fail 1 "Go build did not produce a non-empty executable for $name"
    fi
    magic=$(od -An -tx1 -N4 "$artifact" | tr -d ' \n')
    format=$(od -An -tx1 -j4 -N2 "$artifact" | tr -d ' \n')
    machine=$(od -An -tx1 -j18 -N2 "$artifact" | tr -d ' \n')
    if [ "$magic" != 7f454c46 ] || [ "$format" != 0201 ] || [ "$machine" != 3e00 ]; then
        fail 1 "built $name is not a little-endian Linux ELF64 amd64 executable"
    fi
}

build_tool zpr-system-bundle ./cmd/zpr-system-bundle
build_tool zpr-system-bundle-pack ./cmd/zpr-system-bundle-pack
build_tool zpr-system-bundle-verify ./cmd/zpr-system-bundle-verify
build_tool zpr-system-bundle-sign ./cmd/zpr-system-bundle-sign
build_tool zpr-system-config-render ./cmd/zpr-system-config-render

(
    cd "$temporary/tools"
    case "$checksum_tool" in
        sha256sum) sha256sum zpr-system-bundle zpr-system-bundle-pack zpr-system-bundle-verify zpr-system-bundle-sign zpr-system-config-render > SHA256SUMS ;;
        shasum) shasum -a 256 zpr-system-bundle zpr-system-bundle-pack zpr-system-bundle-verify zpr-system-bundle-sign zpr-system-config-render > SHA256SUMS ;;
    esac
)
{
    printf 'Qualification: engineering-tooling-only; not a system or companion installer.\n'
    printf 'Target: linux/amd64\n'
    printf 'Go toolchain: %s\n' "$go_version"
    printf 'Module minimum: 1.26.1\n'
    printf 'Tools: zpr-system-bundle, zpr-system-bundle-pack, zpr-system-bundle-verify, zpr-system-bundle-sign, zpr-system-config-render\n'
    printf 'Signature and publisher authenticity: not provided.\n'
} > "$temporary/tools/BUILD-INFO.txt"

if ! tar -czf "$temporary/$filename" -C "$temporary" tools; then
    fail 1 "cannot create the bundle-tool archive"
fi
if ! ln "$temporary/$filename" "$destination"; then
    fail 1 "cannot publish archive without overwriting; output may already exist"
fi

printf 'Built Linux/amd64 bundle-tool archive: %s\n' "$destination"
printf 'Go toolchain: %s; module minimum: 1.26.1\n' "$go_version"
echo "Engineering tooling only; the archive is unsigned and is not an installer or release-qualified package."
