#!/bin/sh
set -eu
umask 077
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
command -v sha256sum >/dev/null
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
mkdir "$tmp/package" "$tmp/package/bin" "$tmp/package/images" "$tmp/tools"
cp "$here/installed.sh" "$tmp/package/demo.sh"
cat > "$tmp/tools/uname" <<'EOF'
#!/bin/sh
case "$1" in -s) echo Linux ;; -m) echo x86_64 ;; *) exit 1 ;; esac
EOF
cat > "$tmp/tools/docker" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
case "$1 $2" in
    'compose version') exit 0 ;;
    'image inspect') printf 'linux/amd64 %s\n' "$5" ;;
    'load --input') exit 0 ;;
    *) exit 1 ;;
esac
EOF
cat > "$tmp/package/bin/zpr-lab-bootstrap" <<'EOF'
#!/bin/sh
printf '%s\n' "$@" > "$ARGS"
EOF
chmod 700 "$tmp/tools/"* "$tmp/package/bin/zpr-lab-bootstrap"
export PATH="$tmp/tools:$PATH" CALLS="$tmp/calls" ARGS="$tmp/args"
digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
for role in data-plane ldap ldap-admin-ui logging oidc; do
    printf '%s %s\n' "$role" "$digest" >> "$tmp/package/images.txt"
    printf 'test image\n' > "$tmp/package/images/$role.tar"
done
checksum() {
    (cd "$tmp/package" && sha256sum demo.sh images.txt bin/* images/*.tar > SHA256SUMS)
}
checksum
sh "$tmp/package/demo.sh" verify >/dev/null
test ! -e "$CALLS"
sh "$tmp/package/demo.sh" load >/dev/null
test "$(grep -c '^load --input' "$CALLS")" -eq 5
sh "$tmp/package/demo.sh" start "$tmp/state" -room-port 28787 >/dev/null
grep -Fx -- '-control-room-binary' "$ARGS"
grep -Fx -- "$tmp/package/bin/zpr-control-room" "$ARGS"
grep -Fx -- '-protected-dns' "$ARGS"
grep -Fx -- '-ldap-admin-image' "$ARGS"
grep -Fx -- '28787' "$ARGS"
cp "$ARGS" "$tmp/original-args"
printf 'tampered\n' >> "$tmp/package/images.txt"
if sh "$tmp/package/demo.sh" start "$tmp/state" >"$tmp/error" 2>&1; then
    echo "tampered package accepted" >&2
    exit 1
fi
cmp "$ARGS" "$tmp/original-args"
grep 'package verification failed' "$tmp/error"
printf 'PASS: offline package verification, image loading, prebuilt startup and tamper rejection\n'
