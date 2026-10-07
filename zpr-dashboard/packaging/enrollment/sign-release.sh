#!/bin/sh
set -eu
umask 077

if [ "$#" -ne 5 ]; then
    echo "usage: sh sign-release.sh package.deb version full-primary-fingerprint expiry-epoch new-release-directory" >&2
    exit 2
fi
assets=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=release-common.sh
. "$assets/release-common.sh"
package=$1
version=$2
fingerprint=$3
expiry=$4
output=$5
release_version "$version"
release_fingerprint "$fingerprint"
[ -f "$package" ] && [ ! -L "$package" ] || release_fail "package must be a regular non-symlink file"
case "$expiry" in
    ''|*[!0-9]*) release_fail "expiry must be an epoch timestamp" ;;
esac
[ "${#expiry}" -le 10 ] || release_fail "invalid expiry"
now=$(date +%s)
[ "$expiry" -gt "$now" ] && [ "$expiry" -le "$((now + 2592000))" ] || release_fail "expiry must be within the next 30 days"
for command in gpg gpgv sha256sum dpkg-deb; do
    command -v "$command" >/dev/null 2>&1 || release_fail "$command required"
done
temporary=$(mktemp -d)
trap 'rm -r -- "$temporary"' EXIT
trap 'exit 1' HUP INT TERM
name="zpr-enrollment-setup_${version}_amd64.deb"
cp -- "$package" "$temporary/$name"
release_package "$temporary/$name" "$version"
hash=$(sha256sum "$temporary/$name")
printf 'ZPR-ENROLLMENT-RELEASE-1\nversion=%s\nexpires=%s\npackage=%s\nsha256=%s\n' \
    "$version" "$expiry" "$name" "${hash%% *}" > "$temporary/release.manifest"
# Signing uses the operator's existing agent/hardware key; never accept a
# passphrase on the command line or generate/import a private signing key here.
gpg --batch --yes --local-user "$fingerprint" --digest-algo SHA256 \
    --armor --detach-sign --output "$temporary/release.manifest.asc" "$temporary/release.manifest"
gpg --batch --export "$fingerprint" > "$temporary/signer.gpg"
sh "$assets/verify-release.sh" "$temporary/$name" "$temporary/release.manifest" \
    "$temporary/release.manifest.asc" "$temporary/signer.gpg" "$fingerprint" "$version"
# mkdir refuses an existing output directory. Distribute only after this
# command exits successfully; a copy failure leaves an explicitly invalid bundle.
mkdir -- "$output"
if ! cp -- "$temporary/$name" "$temporary/release.manifest" "$temporary/release.manifest.asc" "$output/"; then
    release_fail "bundle copy failed; do not distribute the incomplete output directory"
fi
chmod 0644 "$output/$name" "$output/release.manifest" "$output/release.manifest.asc"
echo "Signed bundle created at $output. Recipient keyring/fingerprint must be provisioned independently."
