#!/bin/sh
set -eu
umask 022

if [ "$#" -ne 3 ]; then
    echo "usage: sh build-macos.sh macos-arm64-binary version new-output.dmg" >&2
    exit 2
fi
binary=$1
version=$2
output=$3
if [ "$(uname -s)" != Darwin ]; then
    echo "macOS host required for native app signing and DMG packaging" >&2
    exit 1
fi
if ! printf '%s\n' "$version" | grep -Eq '^[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}$'; then
    echo "version must be three numeric components (for example 0.1.0)" >&2
    exit 2
fi
if [ ! -f "$binary" ] || [ -L "$binary" ] || [ -e "$output" ] || [ -L "$output" ]; then
    echo "input must be a regular binary and output must not exist" >&2
    exit 2
fi
if [ "$(lipo -archs "$binary")" != arm64 ] || ! otool -L "$binary" | grep -Fq '/Security.framework/'; then
    echo "payload must be a native arm64 Mach-O with CGO Security.framework support" >&2
    exit 2
fi
minimum=$(otool -l "$binary" | awk '/LC_BUILD_VERSION/{build=1} build && $1=="minos"{print $2; exit}')
if [ "$minimum" != 13.0 ] && [ "$minimum" != 13.0.0 ]; then
    echo "payload must be built with explicit macOS 13.0 deployment target (CGO_CFLAGS/CGO_LDFLAGS)" >&2
    exit 2
fi
assets=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
stage=$(mktemp -d)
trap 'rm -r -- "$stage"' EXIT
trap 'exit 1' HUP INT TERM
app="$stage/ZPR Machine Setup.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
install -m 0755 "$binary" "$app/Contents/MacOS/zpr-enrollment-setup"
install -m 0755 "$assets/macos/launch" "$app/Contents/MacOS/launch"
install -m 0644 "$assets/macos/launch.applescript" "$app/Contents/Resources/launch.applescript"
install -m 0644 "$assets/macos/README.txt" "$app/Contents/Resources/README.txt"
install -m 0644 "$assets/macos/setup.example.json" "$app/Contents/Resources/setup.example.json"
cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>com.zpr.enrollment.development</string>
<key>CFBundleName</key><string>ZPR Machine Setup</string>
<key>CFBundleDisplayName</key><string>ZPR Machine Setup (Development)</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleExecutable</key><string>launch</string>
<key>CFBundleShortVersionString</key><string>$version</string>
<key>CFBundleVersion</key><string>$version</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>LSArchitecturePriority</key><array><string>arm64</string></array>
<key>LSUIElement</key><true/>
<key>NSAppleEventsUsageDescription</key><string>Open Terminal to run the local enrollment wizard as your desktop user.</string>
</dict></plist>
EOF
plutil -lint "$app/Contents/Info.plist"
codesign --force --sign - --timestamp=none --identifier com.zpr.enrollment.development.wizard "$app/Contents/MacOS/zpr-enrollment-setup"
codesign --force --sign - "$app"
codesign --verify --deep --strict "$app"
osacompile -o "$stage/launch-test.scpt" "$assets/macos/launch.applescript"
rm "$stage/launch-test.scpt"
cp "$assets/macos/README.txt" "$stage/READ-ME-FIRST.txt"
hdiutil create -quiet -format UDZO -volname "ZPR Enrollment Development" -srcfolder "$stage" "$output"
echo "Created AD-HOC-SIGNED, NOT NOTARIZED development image: $output"
