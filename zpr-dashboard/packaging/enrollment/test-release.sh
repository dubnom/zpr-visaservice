#!/bin/sh
set -eu
umask 077

if [ "$#" -ne 1 ] || [ ! -f /.dockerenv ]; then
    echo "Run in a disposable Debian/Ubuntu Docker container: sh test-release.sh development-package.deb" >&2
    exit 2
fi
assets=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
temporary=$(mktemp -d)
trap 'rm -r -- "$temporary"' EXIT
trap 'exit 1' HUP INT TERM
export GNUPGHOME="$temporary/gnupg"
mkdir -m 0700 "$GNUPGHOME"
# Disposable test keys only; never generate a release/production identity.
gpg --batch --pinentry-mode loopback --passphrase '' --quick-generate-key \
    'Disposable ZPR Release Test' rsa2048 cert 1d
fingerprint=$(gpg --batch --with-colons --list-keys | awk -F: '$1 == "fpr" {print $10; exit}')
gpg --batch --pinentry-mode loopback --passphrase '' --quick-add-key "$fingerprint" rsa2048 sign 1d
gpg --batch --export "$fingerprint" > "$temporary/trusted.gpg"
version=$(dpkg-deb -f "$1" Version)
expiry=$(($(date +%s) + 3600))
sh "$assets/sign-release.sh" "$1" "$version" "$fingerprint" "$expiry" "$temporary/release"
package="$temporary/release/zpr-enrollment-setup_${version}_amd64.deb"
manifest="$temporary/release/release.manifest"
signature="$temporary/release/release.manifest.asc"

verify() {
    sh "$assets/verify-release.sh" "$package" "$manifest" "$signature" \
        "$temporary/trusted.gpg" "$fingerprint" "$version"
}

reject() {
    if verify; then
        echo "FAIL: accepted $1" >&2
        exit 1
    fi
    echo "PASS: rejected $1"
}

sign_manifest() {
    gpg --batch --yes --local-user "$fingerprint" --digest-algo SHA256 \
        --armor --detach-sign --output "$signature" "$manifest"
}

verify
if [ "${ZPR_TEST_DOWNLOAD:-0}" = 1 ]; then
    node "$assets/test-download.mjs" "$temporary/release" "$temporary/trusted.gpg" "$fingerprint" "$version"
fi
cp "$manifest" "$temporary/good-manifest"
cp "$signature" "$temporary/good-signature"
cp "$package" "$temporary/good-package"
printf x >> "$package"
reject "modified package"
cp "$temporary/good-package" "$package"
printf x >> "$manifest"
reject "modified manifest"
cp "$temporary/good-manifest" "$manifest"
printf 'corrupt signature' > "$signature"
reject "corrupt signature"
cp "$temporary/good-signature" "$signature"
cat "$temporary/good-signature" >> "$signature"
reject "multiple release signatures"
cp "$temporary/good-signature" "$signature"
original_version=$version
version=0.0.0
reject "unexpected/downgraded version"
version=$original_version
original_fingerprint=$fingerprint
fingerprint=0000000000000000000000000000000000000000
reject "wrong independently pinned signer"
fingerprint=$original_fingerprint

sed "s/expires=.*/expires=$(($(date +%s) - 1))/" "$temporary/good-manifest" > "$manifest"
sign_manifest
reject "signed expired release"
sed "s/expires=.*/expires=$(($(date +%s) + 2592100))/" "$temporary/good-manifest" > "$manifest"
sign_manifest
reject "signed excessive lifetime"
sed 's/package=.*/package=..\/untrusted.deb/' "$temporary/good-manifest" > "$manifest"
sign_manifest
reject "signed unexpected package path"
cat "$temporary/good-manifest" > "$manifest"
printf 'unexpected=true\n' >> "$manifest"
sign_manifest
reject "signed extra manifest field"
cat "$temporary/good-manifest" > "$manifest"
printf '\000' >> "$manifest"
sign_manifest
reject "signed noncanonical null byte"
cp "$temporary/good-manifest" "$manifest"
gpg --batch --yes --local-user "$fingerprint" --digest-algo SHA1 \
    --armor --detach-sign --output "$signature" "$manifest"
reject "weak signature digest"
cp "$temporary/good-signature" "$signature"
mv "$package" "$temporary/package-target"
ln -s "$temporary/package-target" "$package"
reject "symlink package"
rm "$package"
mv "$temporary/package-target" "$package"
gpg --batch --pinentry-mode loopback --passphrase '' --quick-generate-key \
    'Disposable Untrusted Test' rsa2048 sign 1d
other=$(gpg --batch --with-colons --list-keys 'Disposable Untrusted Test' | awk -F: '$1 == "fpr" {print $10; exit}')
gpg --batch --export "$other" > "$temporary/trusted.gpg"
reject "keyring lacks signer"
gpg --batch --export "$fingerprint" > "$temporary/trusted.gpg"
verify
original_fingerprint=$fingerprint
past=$(($(date +%s) - 172800))
gpg --batch --faked-system-time "$past" --pinentry-mode loopback --passphrase '' \
    --quick-generate-key 'Disposable Expired Test' rsa2048 sign 1d
fingerprint=$(gpg --batch --with-colons --list-keys 'Disposable Expired Test' | awk -F: '$1 == "fpr" {print $10; exit}')
gpg --batch --export "$fingerprint" > "$temporary/trusted.gpg"
gpg --batch --yes --faked-system-time "$past" --local-user "$fingerprint" \
    --digest-algo SHA256 --armor --detach-sign --output "$signature" "$manifest"
reject "expired signing key with otherwise current manifest"
fingerprint=$original_fingerprint
cp "$temporary/good-signature" "$signature"
gpg --batch --export "$fingerprint" > "$temporary/trusted.gpg"
if sh "$assets/sign-release.sh" "$package" "$version" "$fingerprint" "$expiry" "$temporary/release"; then
    echo "FAIL: signer overwrote release directory" >&2
    exit 1
fi
sed 's/^://' "$GNUPGHOME/openpgp-revocs.d/$fingerprint.rev" > "$temporary/revocation.asc"
gpg --batch --import "$temporary/revocation.asc"
gpg --batch --export "$fingerprint" > "$temporary/trusted.gpg"
reject "revoked primary key in trusted keyring"
echo "Release signature/hash/version/lifetime/subkey/negative checks passed. No package installed."
