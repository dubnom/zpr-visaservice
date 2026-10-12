#!/bin/sh
set -eu
umask 022

if [ "$#" -ne 3 ] && [ "$#" -ne 4 ]; then
    echo "usage: sh build-macos.sh macos-arm64-binary version new-output.dmg [Developer-ID-Application-identity]" >&2
    exit 2
fi
binary=$1
version=$2
output=$3
identity=${4:-}
if [ -n "$identity" ]; then
    case "$identity" in
        "Developer ID Application: "*) ;;
        *) echo "distribution signing requires a Developer ID Application identity" >&2; exit 2 ;;
    esac
fi
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
if [ -n "$identity" ]; then
    sed 's/This local build is ad-hoc signed, NOT Developer ID signed or notarized./This build is Developer ID signed. Notarization must be verified separately./' \
        "$assets/macos/README.txt" > "$app/Contents/Resources/README.txt"
fi
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
if [ -n "$identity" ]; then
    codesign --force --sign "$identity" --options runtime --timestamp --identifier com.zpr.enrollment.development.wizard "$app/Contents/MacOS/zpr-enrollment-setup"
    codesign --force --sign "$identity" --options runtime --timestamp "$app"
else
    codesign --force --sign - --timestamp=none --identifier com.zpr.enrollment.development.wizard "$app/Contents/MacOS/zpr-enrollment-setup"
    codesign --force --sign - "$app"
fi
codesign --verify --deep --strict "$app"
osacompile -o "$stage/launch-test.scpt" "$assets/macos/launch.applescript"
rm "$stage/launch-test.scpt"
cp "$app/Contents/Resources/README.txt" "$stage/READ-ME-FIRST.txt"
hdiutil create -quiet -format UDZO -volname "ZPR Enrollment Development" -srcfolder "$stage" "$output"
if [ -n "$identity" ]; then
    codesign --force --sign "$identity" --timestamp "$output"
    codesign --verify --strict "$output"
    echo "Created DEVELOPER-ID-SIGNED, NOT YET NOTARIZED enrollment-only image: $output"
else
    echo "Created AD-HOC-SIGNED, NOT NOTARIZED development image: $output"
fi
