#!/bin/sh
set -eu
umask 077
if [ "$#" -ne 2 ] || [ "$(uname -s)" != Darwin ]; then
    echo "usage (macOS): sh test-macos.sh development.dmg original-macos-arm64-binary" >&2
    exit 2
fi
image=$1
binary=$2
assets=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
stage=$(mktemp -d)
mounted=no
pid=
cleanup() {
    if [ -n "$pid" ]; then
        kill "$pid"
        wait "$pid" || status=$?
    fi
    if [ "$mounted" = yes ]; then
        hdiutil detach -quiet "$stage/mount"
    fi
    rm -r -- "$stage"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
mkdir "$stage/mount" "$stage/home"
hdiutil attach -readonly -nobrowse -quiet -mountpoint "$stage/mount" "$image"
mounted=yes
app="$stage/mount/ZPR Machine Setup.app"
codesign --verify --deep --strict "$app"
plutil -lint "$app/Contents/Info.plist"
[ "$(lipo -archs "$app/Contents/MacOS/zpr-enrollment-setup")" = arm64 ]
cmp "$assets/macos/launch" "$app/Contents/MacOS/launch"
cmp "$assets/macos/launch.applescript" "$app/Contents/Resources/launch.applescript"
cmp "$assets/macos/README.txt" "$app/Contents/Resources/README.txt"
cmp "$assets/macos/setup.example.json" "$app/Contents/Resources/setup.example.json"
cp "$binary" "$stage/original"
cp "$app/Contents/MacOS/zpr-enrollment-setup" "$stage/packaged"
codesign --force --sign - --timestamp=none --identifier com.zpr.enrollment.development.wizard "$stage/original"
cmp "$stage/original" "$stage/packaged"
mkdir -p "$stage/home/Library/Application Support/ZPR/EnrollmentSetup"
config="$stage/home/Library/Application Support/ZPR/EnrollmentSetup/setup.json"
cp "$assets/macos/setup.example.json" "$config"
HOME="$stage/home" "$app/Contents/MacOS/zpr-enrollment-setup" \
    -config "$config" -user-state -require-key-protection macos-keychain > "$stage/console" 2> "$stage/errors" &
pid=$!
attempt=0
while [ "$attempt" -lt 50 ]; do
    if grep -q 'http://127.0.0.1:' "$stage/console"; then break; fi
    if ! kill -0 "$pid" 2>/dev/null; then
        cat "$stage/errors" >&2
        echo "Packaged wizard did not start" >&2
        exit 1
    fi
    attempt=$((attempt + 1))
    sleep 0.1
done
url=$(sed -n 's|^\(http://127.0.0.1:[0-9]*\)/#session=\([A-Z2-7]*\)$|\1|p' "$stage/console")
token=$(sed -n 's|^http://127.0.0.1:[0-9]*/#session=\([A-Z2-7]*\)$|\1|p' "$stage/console")
[ -n "$url" ] && [ -n "$token" ]
curl --fail --silent --show-error --max-time 5 -H "X-ZPR-Setup-Session: $token" "$url/api/session" \
    > "$stage/session.json"
grep -q "macOS Keychain" "$stage/session.json"
status=$(curl --silent --show-error --max-time 5 -o "$stage/anonymous.json" -w '%{http_code}' "$url/api/session")
[ "$status" = 401 ]
kill "$pid"
wait "$pid"
pid=
echo "Mac app signature/content and native loopback startup checks passed. No login Keychain item, browser or adapter was created."
