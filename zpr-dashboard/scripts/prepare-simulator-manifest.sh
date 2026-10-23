#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
    echo "Usage: sh prepare-simulator-manifest.sh /absolute/new-directory" >&2
    exit 2
fi
case "$1" in
    /*) ;;
    *) echo "Output must be an absolute path to a new directory" >&2; exit 2 ;;
esac
case "$1" in
    */|*/.|*/..|*/../*|*/./*)
        echo "Output must not contain dot components or a trailing slash" >&2
        exit 2
        ;;
esac
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
SOURCE="$SCRIPT_DIR/../cmd/zpr-web-dashboard/examples/simulation-environment.json"
if [ ! -f "$SOURCE" ] || [ -L "$SOURCE" ]; then
    echo "Bundled Simulator manifest is missing or is a symlink" >&2
    exit 1
fi
umask 077
mkdir -- "$1"
cp -- "$SOURCE" "$1/simulation-environment.json"
printf 'Simulator topology prepared: %s/simulation-environment.json\n' "$1"
printf '%s\n' 'Set SIMULATION_MANIFEST to that file for Simulator only.'
printf '%s\n' 'No identities, services, containers, AI keys or Control Room configuration were created.'
