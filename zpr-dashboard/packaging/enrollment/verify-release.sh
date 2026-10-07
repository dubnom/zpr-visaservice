#!/bin/sh
set -eu
umask 077

if [ "$#" -ne 6 ]; then
    echo "usage: sh verify-release.sh package.deb manifest signature trusted-keyring full-primary-fingerprint expected-version" >&2
    exit 2
fi
assets=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=release-common.sh
. "$assets/release-common.sh"
package=$1
manifest=$2
signature=$3
keyring=$4
fingerprint=$5
version=$6
release_version "$version"
release_fingerprint "$fingerprint"
for file in "$package" "$manifest" "$signature" "$keyring"; do
    [ -f "$file" ] && [ ! -L "$file" ] || release_fail "inputs must be regular, non-symlink files"
done
for command in gpg gpgv sha256sum dpkg-deb; do
    command -v "$command" >/dev/null 2>&1 || release_fail "$command required"
done
[ "$(wc -c < "$manifest")" -le 1024 ] || release_fail "manifest too large"
[ "$(wc -c < "$signature")" -le 65536 ] || release_fail "signature too large"
temporary=$(mktemp -d)
trap 'rm -r -- "$temporary"' EXIT
trap 'exit 1' HUP INT TERM
# Use immutable-to-other-users snapshots for signature, hash, and metadata checks.
cp -- "$manifest" "$temporary/manifest"
cp -- "$signature" "$temporary/signature"
cp -- "$keyring" "$temporary/trusted.gpg"
cp -- "$package" "$temporary/package.deb"
if ! gpgv --homedir "$temporary" --keyring "$temporary/trusted.gpg" \
    --status-fd 3 "$temporary/signature" "$temporary/manifest" 3>"$temporary/status"; then
    release_fail "OpenPGP signature invalid or untrusted"
fi
now=$(date +%s)
# Require exactly one valid signature from the independently pinned primary key.
# Only SHA-256/384/512 signatures are accepted; reject explicit key/signature errors.
if ! awk -v pin="$fingerprint" -v now="$now" '
    $1 == "[GNUPG:]" && $2 ~ /^(BADSIG|ERRSIG|EXPSIG|EXPKEYSIG|REVKEYSIG|KEYEXPIRED|SIGEXPIRED|FAILURE)$/ { bad=1 }
    $1 == "[GNUPG:]" && $2 == "VALIDSIG" {
        count++
        primary=($12 == "" ? $3 : $12)
        if (primary != pin || ($10 != 8 && $10 != 9 && $10 != 10) ||
            $5 > now || ($6 != 0 && $6 <= now)) bad=1
    }
    END { exit (bad || count != 1) }
' "$temporary/status"; then
    release_fail "signature signer, algorithm, or validity does not meet policy"
fi
signer=$(awk '$1 == "[GNUPG:]" && $2 == "VALIDSIG" {print $3}' "$temporary/status")
# gpgv trusts keyring contents and does not independently enforce key expiry.
# Inspect the pinned primary and actual signing subkey at the current time.
gpg --batch --homedir "$temporary" --with-colons --show-keys "$temporary/trusted.gpg" > "$temporary/keys"
if ! awk -F: -v primary="$fingerprint" -v signer="$signer" -v now="$now" '
    $1 == "pub" || $1 == "sub" {
        usable=($2 != "r" && $2 != "e" && $2 != "d" && $6 <= now && ($7 == "" || $7 == 0 || $7 > now))
        kind=$1
    }
    $1 == "fpr" {
        if (kind == "pub" && $10 == primary && usable) primary_ok=1
        if ($10 == signer && usable) signer_ok=1
        kind=""
    }
    END { exit (!primary_ok || !signer_ok) }
' "$temporary/keys"; then
    release_fail "primary/signing key is expired, revoked, disabled, or unavailable"
fi
{
    IFS= read -r header || release_fail "missing format"
    IFS= read -r version_line || release_fail "missing version"
    IFS= read -r expiry_line || release_fail "missing expiry"
    IFS= read -r package_line || release_fail "missing package"
    IFS= read -r hash_line || release_fail "missing hash"
    if IFS= read -r extra || [ -n "$extra" ]; then
        release_fail "unexpected manifest content"
    fi
} < "$temporary/manifest"
[ "$header" = "ZPR-ENROLLMENT-RELEASE-1" ] || release_fail "unsupported manifest"
[ "$version_line" = "version=$version" ] || release_fail "unexpected release version"
[ "$package_line" = "package=zpr-enrollment-setup_${version}_amd64.deb" ] || release_fail "unexpected package filename"
expiry=${expiry_line#expires=}
case "$expiry" in
    ''|*[!0-9]*) release_fail "invalid expiry" ;;
esac
[ "${#expiry}" -le 10 ] && [ "$expiry_line" = "expires=$expiry" ] || release_fail "invalid expiry field"
[ "$expiry" -gt "$now" ] && [ "$expiry" -le "$((now + 2592000))" ] || release_fail "release expired or exceeds 30-day validity"
hash=${hash_line#sha256=}
case "$hash" in
    *[!0-9a-f]*|'') release_fail "invalid SHA-256" ;;
esac
[ "${#hash}" -eq 64 ] && [ "$hash_line" = "sha256=$hash" ] || release_fail "invalid hash field"
printf 'ZPR-ENROLLMENT-RELEASE-1\nversion=%s\nexpires=%s\npackage=%s\nsha256=%s\n' \
    "$version" "$expiry" "zpr-enrollment-setup_${version}_amd64.deb" "$hash" > "$temporary/canonical"
cmp -s "$temporary/canonical" "$temporary/manifest" || release_fail "noncanonical manifest bytes"
actual=$(sha256sum "$temporary/package.deb")
[ "${actual%% *}" = "$hash" ] || release_fail "package hash mismatch"
release_package "$temporary/package.deb" "$version"
echo "Verified release $version from primary signer $fingerprint."
echo "Verification only; nothing installed. Preserve these exact verified bytes in a trusted directory before installation."
