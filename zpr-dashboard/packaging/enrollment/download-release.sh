#!/bin/sh
set -eu
umask 077

if [ "$#" -ne 6 ]; then
    echo "usage: sh download-release.sh https-base-url trusted-keyring primary-fingerprint expected-version new-output-directory ca-file-or-system" >&2
    exit 2
fi
assets=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=release-common.sh
. "$assets/release-common.sh"
base=$1
keyring=$2
fingerprint=$3
version=$4
output=$5
ca=$6
release_version "$version"
release_fingerprint "$fingerprint"
# Deliberately narrow URL contract: no credentials, escapes, queries, fragments,
# IPv6 literals, or redirect-dependent CDN links in the initial release.
printf '%s\n' "$base" | LC_ALL=C grep -Eq '^https://[A-Za-z0-9.-]+(:[0-9]+)?(/[A-Za-z0-9._~+/-]*)?$' ||
    release_fail "download base must be a plain HTTPS URL without credentials, query, fragment, or escapes"
case "$output" in
    /*) ;;
    *) release_fail "output must be an absolute path" ;;
esac
parent=$(dirname -- "$output")
[ "$(realpath -e -- "$parent")/$(basename -- "$output")" = "$output" ] ||
    release_fail "output path must be canonical with existing non-symlink parents"
[ "$(stat -c %u -- "$parent")" = "$(id -u)" ] &&
    [ "$(stat -c %a -- "$parent")" = 700 ] ||
    release_fail "output parent must be owned by the current user with mode 0700"
[ ! -e "$output" ] && [ ! -L "$output" ] || release_fail "output already exists"
[ -f "$keyring" ] && [ ! -L "$keyring" ] || release_fail "trusted keyring must be a regular non-symlink file"
for command in curl gpg gpgv sha256sum dpkg-deb; do
    command -v "$command" >/dev/null 2>&1 || release_fail "$command required"
done
if [ "$ca" != system ]; then
    [ -f "$ca" ] && [ ! -L "$ca" ] || release_fail "CA file must be an independently trusted regular file"
fi
stage=$(mktemp -d "$parent/.zpr-download-XXXXXXXX")
trap 'if [ -d "$stage" ]; then rm -r -- "$stage"; fi' EXIT
trap 'exit 1' HUP INT TERM
cp -- "$keyring" "$stage/trusted.gpg"
if [ "$ca" != system ]; then
    cp -- "$ca" "$stage/ca.pem"
fi

download() {
    name=$1
    limit=$2
    set --
    if [ "$ca" != system ]; then
        set -- --cacert "$stage/ca.pem"
    fi
    # OS file limits bound chunked downloads even with older curl versions
    # whose --max-filesize check only honors a declared Content-Length.
    if ! (
        ulimit -f 65536
        env -u CURL_CA_BUNDLE -u SSL_CERT_FILE -u SSL_CERT_DIR \
            curl -q --silent --show-error --fail --proxy '' --noproxy '*' \
            --proto '=https' --tlsv1.2 --connect-timeout 10 --max-time 120 \
            --max-filesize "$limit" "$@" --output "$stage/$name" \
            --write-out '%{http_code}' --url "${base%/}/$name" > "$stage/http-status"
    ); then
        release_fail "download failed; incomplete bundle discarded"
    fi
    [ "$(cat "$stage/http-status")" = 200 ] || release_fail "HTTP 200 required; redirects are not followed"
    [ "$(wc -c < "$stage/$name")" -le "$limit" ] || release_fail "download exceeds allowed size"
}

download release.manifest 1024
download release.manifest.asc 65536
name="zpr-enrollment-setup_${version}_amd64.deb"
download "$name" 33554432
sh "$assets/verify-release.sh" "$stage/$name" "$stage/release.manifest" \
    "$stage/release.manifest.asc" "$stage/trusted.gpg" "$fingerprint" "$version"
rm -- "$stage/trusted.gpg" "$stage/http-status"
if [ "$ca" != system ]; then
    rm -- "$stage/ca.pem"
fi
# GNU mv -T -n publishes the sibling directory without merging/replacing a
# concurrent destination. An existing destination leaves the source untouched.
mv -T -n -- "$stage" "$output"
[ ! -d "$stage" ] || release_fail "output appeared concurrently; verified staging discarded"
echo "Verified bundle staged at $output. Nothing installed."
echo "Keep this private directory protected; reverify before a separate administrator installation."
