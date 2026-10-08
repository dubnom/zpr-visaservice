#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
    echo "usage: sh build-windows.sh windows-amd64-binary version-four-numbers new-output.exe" >&2
    exit 2
fi
binary=$1
version=$2
output=$3
if ! printf '%s\n' "$version" | grep -Eq '^[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}$'; then
    echo "version must contain four numeric components (for example 0.1.0.0)" >&2
    exit 2
fi
old_ifs=$IFS
IFS=.
for component in $version; do
    if [ "$component" -gt 65535 ]; then
        echo "version components must be at most 65535" >&2
        exit 2
    fi
done
IFS=$old_ifs
if [ ! -f "$binary" ] || [ -L "$binary" ] || [ -e "$output" ] || [ -L "$output" ]; then
    echo "input must be a regular binary and output must not exist" >&2
    exit 2
fi
command -v makensis >/dev/null 2>&1 || { echo "NSIS makensis is required" >&2; exit 1; }
magic=$(od -An -tx1 -N2 "$binary" | tr -d ' \n')
offset=$(od -An -tu4 -j60 -N4 "$binary" | tr -d ' \n')
case "$offset" in ''|*[!0-9]*) echo "invalid PE header offset" >&2; exit 2 ;; esac
header=$(od -An -tx1 -j "$offset" -N6 "$binary" | tr -d ' \n')
if [ "$magic" != 4d5a ] || [ "$header" != 504500006486 ]; then
    echo "binary must be a Windows PE amd64 executable" >&2
    exit 2
fi
binary=$(CDPATH='' cd -- "$(dirname -- "$binary")" && printf '%s/%s' "$(pwd)" "$(basename -- "$binary")")
output=$(CDPATH='' cd -- "$(dirname -- "$output")" && printf '%s/%s' "$(pwd)" "$(basename -- "$output")")
assets=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
makensis -DPAYLOAD="$binary" -DVERSION="$version" -DOUTPUT="$output" "$assets/windows.nsi"
echo "Created UNSIGNED development installer: $output"
