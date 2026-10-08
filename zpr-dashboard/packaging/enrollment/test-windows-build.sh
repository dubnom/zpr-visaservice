#!/bin/sh
set -eu
if [ "$#" -ne 1 ]; then
    echo "usage: sh test-windows-build.sh windows-amd64-binary" >&2
    exit 2
fi
command -v 7z >/dev/null 2>&1 || { echo "7z is required to verify packaged contents" >&2; exit 1; }
assets=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
stage=$(mktemp -d)
trap 'rm -r -- "$stage"' EXIT
trap 'exit 1' HUP INT TERM
binary=$1
sh "$assets/build-windows.sh" "$binary" 0.1.0.0 "$stage/zpr-enrollment-installer.exe"
7z x -y "-o$stage/extracted" "$stage/zpr-enrollment-installer.exe" >/dev/null
cmp "$binary" "$stage/extracted/zpr-enrollment-setup.exe"
cmp "$assets/setup.example.json" "$stage/extracted/setup.example.json"
cmp "$assets/launch-setup.cmd" "$stage/extracted/launch-setup.cmd"
cmp "$assets/WINDOWS-README.txt" "$stage/extracted/README.txt"
if sh "$assets/build-windows.sh" "$binary" 0.1.0.0 "$stage/zpr-enrollment-installer.exe" > "$stage/rejected" 2>&1; then
    echo "Existing installer was overwritten" >&2
    exit 1
fi
for version in bad 0.1.0 65536.1.0.0; do
    if sh "$assets/build-windows.sh" "$binary" "$version" "$stage/invalid.exe" > "$stage/rejected" 2>&1; then
        echo "Invalid version accepted" >&2
        exit 1
    fi
done
if sh "$assets/build-windows.sh" /bin/sh 0.1.0.0 "$stage/invalid.exe" > "$stage/rejected" 2>&1; then
    echo "Non-Windows payload accepted" >&2
    exit 1
fi
echo "Windows installer payload equality and invalid-input checks passed."
