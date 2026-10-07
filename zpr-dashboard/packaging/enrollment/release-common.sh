#!/bin/sh

release_fail() {
    echo "Release verification failed: $*" >&2
    exit 1
}

release_version() {
    case "$1" in
        ''|*[!0-9a-zA-Z.+~-]*|[!0-9]*) release_fail "invalid expected version" ;;
    esac
    [ "${#1}" -le 128 ] || release_fail "version too long"
}

release_fingerprint() {
    case "$1" in
        *[!0-9A-F]*|'') release_fail "fingerprint must be full uppercase hexadecimal" ;;
    esac
    [ "${#1}" -eq 40 ] || [ "${#1}" -eq 64 ] || release_fail "full signer fingerprint required"
}

release_package() {
    [ "$(dpkg-deb -f "$1" Package)" = zpr-enrollment-setup ] || release_fail "wrong package name"
    [ "$(dpkg-deb -f "$1" Architecture)" = amd64 ] || release_fail "wrong architecture"
    [ "$(dpkg-deb -f "$1" Version)" = "$2" ] || release_fail "wrong package version"
}
