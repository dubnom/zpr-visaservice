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
    echo "usage: sh build-cli-linux-amd64.sh /absolute/path/to/new-output-binary" >&2
    exit 2
fi

output=$1
case "$output" in
    /*) ;;
    *) fail 2 "output path must be absolute" ;;
esac
output_name=${output##*/}
case "$output_name" in
    ''|.|..) fail 2 "output path must name a new file" ;;
esac
if [ -e "$output" ] || [ -L "$output" ]; then
    fail 2 "output already exists; refusing to overwrite it"
fi

command -v go >/dev/null 2>&1 ||
    fail 1 "Go 1.26.1 or newer is required; no toolchain will be downloaded"

for tool in awk dirname ln mkdir mktemp od rm tr uname; do
    command -v "$tool" >/dev/null 2>&1 ||
        fail 1 "required build utility is missing: $tool"
done

if ! go_info=$(GOTOOLCHAIN=local GOENV=off go version 2>&1); then
    fail 1 "cannot query the local Go toolchain: $go_info"
fi
version_token=${go_info#go version }
if [ "$version_token" = "$go_info" ]; then
    fail 1 "unrecognized Go version output: $go_info"
fi
version_token=${version_token%% *}
go_version=${version_token#go}
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

output_parent=${output%/*}
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
destination=$output_parent/$output_name
if [ -e "$destination" ] || [ -L "$destination" ]; then
    fail 2 "output already exists; refusing to overwrite it"
fi

if ! temporary=$(mktemp -d "$output_parent/.${output_name}.build.XXXXXX"); then
    fail 1 "cannot create a temporary build directory beside the requested output"
fi
cleanup() {
    if [ -n "$temporary" ] && [ -d "$temporary" ]; then
        rm -r "$temporary"
    fi
}
trap cleanup 0
trap 'exit 1' HUP INT TERM

artifact=$temporary/$output_name
if ! (
    cd "$module_root"
    GOENV=off GOTOOLCHAIN=local GOPROXY=off GOWORK=off GOFLAGS= \
        GOOS=linux GOARCH=amd64 GOAMD64=v1 GOEXPERIMENT= CGO_ENABLED=0 \
        go build -mod=readonly -trimpath -buildvcs=false \
        -o "$artifact" ./cmd/zpr-system-config
); then
    fail 1 "Go build failed; no artifact was published"
fi

if [ ! -f "$artifact" ] || [ ! -s "$artifact" ] || [ ! -x "$artifact" ]; then
    fail 1 "Go build did not produce a non-empty executable"
fi
magic=$(od -An -tx1 -N4 "$artifact" | tr -d ' \n')
format=$(od -An -tx1 -j4 -N2 "$artifact" | tr -d ' \n')
machine=$(od -An -tx1 -j18 -N2 "$artifact" | tr -d ' \n')
if [ "$magic" != 7f454c46 ] || [ "$format" != 0201 ] || [ "$machine" != 3e00 ]; then
    fail 1 "built output is not a little-endian Linux ELF64 amd64 executable"
fi

if ! ln "$artifact" "$destination"; then
    fail 1 "cannot publish output without overwriting; output may already exist"
fi

printf 'Staged Linux/amd64 zpr-system-config at %s\n' "$destination"
printf 'Go toolchain: %s; module minimum: 1.26.1\n' "$go_version"
echo "Engineering staging only; this output is not claimed byte-reproducible or release-qualified."
