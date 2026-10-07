#!/bin/sh
set -eu
umask 022

if [ "$#" -ne 3 ]; then
    echo "usage: sh build-deb.sh /path/to/linux-amd64-binary version /path/to/new-output.deb" >&2
    exit 2
fi
binary=$1
version=$2
output=$3
case "$version" in
    ''|*[!0-9a-zA-Z.+~-]*|[!0-9]*)
        echo "version must start with a digit and use Debian version characters" >&2
        exit 2
        ;;
esac
if [ ! -f "$binary" ] || [ -e "$output" ] || [ -L "$output" ]; then
    echo "input must be a regular binary and output must not exist" >&2
    exit 2
fi
command -v dpkg-deb >/dev/null 2>&1 || {
    echo "dpkg-deb is required (Debian/Ubuntu dpkg package)" >&2
    exit 1
}
magic=$(od -An -tx1 -N4 "$binary" | tr -d ' \n')
format=$(od -An -tx1 -j4 -N2 "$binary" | tr -d ' \n')
machine=$(od -An -tx1 -j18 -N2 "$binary" | tr -d ' \n')
if [ "$magic" != "7f454c46" ] || [ "$format" != "0201" ] || [ "$machine" != "3e00" ]; then
    echo "binary must be a little-endian ELF64 amd64 executable built for Linux" >&2
    exit 2
fi
assets=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
stage=$(mktemp -d)
chmod 0755 "$stage"
trap 'rm -r -- "$stage"' EXIT
trap 'exit 1' HUP INT TERM
mkdir -p "$stage/DEBIAN" "$stage/usr/bin" "$stage/usr/share/applications" \
    "$stage/usr/share/zpr-enrollment-setup" "$stage/usr/share/doc/zpr-enrollment-setup"
install -m 0755 "$binary" "$stage/usr/bin/zpr-enrollment-setup"
install -m 0644 "$assets/zpr-enrollment-setup.desktop" "$stage/usr/share/applications/"
install -m 0644 "$assets/setup.example.json" "$stage/usr/share/zpr-enrollment-setup/"
cat > "$stage/usr/share/doc/zpr-enrollment-setup/README" <<'EOF'
DEVELOPMENT ONLY: SOFTWARE KEY, NOT HARDWARE-BACKED OR ENCRYPTED AT REST.
This package does not install an adapter, issue credentials, or connect to ZPR.
Supported development targets: Ubuntu 24.04 LTS / Debian 12 desktop, amd64.
Full graphical clean-machine certification remains required.

An administrator must provision /etc/zpr/enrollment-setup.json from the example
in /usr/share/zpr-enrollment-setup, replacing the invalid audience with the
trusted HTTPS service and optionally setting ca_file to a trusted CA PEM file.
Use root ownership and mode 0644 (or stricter). The /etc/zpr directory and CA
parents must be trusted and not writable by unprivileged users. Never put a
code, private key, or state_directory in this machine-wide configuration.

Launch "ZPR Machine Setup (Development)" as the logged-in user, NOT with sudo.
The launcher opens a terminal. Open its private URL in the same user's browser;
keep the terminal running. Ctrl-C stops the wizard. A working terminal emulator
and modern browser are required. No browser or service starts during install.

Private state is ~/.local/state/zpr-enrollment-development/identity.json.
Upgrades/removal/purge preserve per-user keys and operator-created configuration.
Do not delete state to retry; losing the key requires administrator-led recovery.
Protect backups; root/the owning user can copy and impersonate this identity.
There is no system service-account or adapter key handoff in this release.
EOF
cat > "$stage/DEBIAN/control" <<EOF
Package: zpr-enrollment-setup
Version: $version
Section: net
Priority: optional
Architecture: amd64
Maintainer: ZPR Development Team
Description: Development-only local ZPR enrollment wizard
 Loopback browser setup with a per-user software key.
 Does not issue credentials, install an adapter, or establish ZPR connectivity.
EOF
dpkg-deb --build --root-owner-group "$stage" "$output"
echo "Created UNSIGNED development package: $output"
