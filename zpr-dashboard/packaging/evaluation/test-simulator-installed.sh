#!/bin/sh
set -eu
umask 077
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
mkdir "$tmp/companion" "$tmp/companion/bin" "$tmp/package" "$tmp/package/bin" "$tmp/tools"
cp "$here/simulator-installed.sh" "$tmp/companion/simulator.sh"
cat > "$tmp/tools/uname" <<'EOF'
#!/bin/sh
case "$1" in -s) echo Linux ;; -m) echo x86_64 ;; *) exit 1 ;; esac
EOF
cat > "$tmp/tools/docker" <<'EOF'
#!/bin/sh
[ "$1 $2" = 'image inspect' ] || exit 1
printf 'linux/amd64 %s\n' "$5"
EOF
cat > "$tmp/package/bin/zpr-lab-bootstrap" <<'EOF'
#!/bin/sh
printf '%s\n' "$@" > "$ARGS"
EOF
chmod 700 "$tmp/tools/"* "$tmp/package/bin/zpr-lab-bootstrap"
export PATH="$tmp/tools:$PATH" ARGS="$tmp/args"
digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
printf 'paired-build\n' > "$tmp/package/BUILD-INFO.txt"
printf 'data-plane %s\n' "$digest" > "$tmp/package/images.txt"
cp "$tmp/package/BUILD-INFO.txt" "$tmp/package/images.txt" "$tmp/companion/"
printf 'prebuilt\n' > "$tmp/companion/bin/zpr-simulator"
checksum() {
    (cd "$tmp/package" && sha256sum BUILD-INFO.txt images.txt bin/* > SHA256SUMS)
    (cd "$tmp/companion" && sha256sum BUILD-INFO.txt images.txt simulator.sh bin/* > SHA256SUMS)
}
checksum
sh "$tmp/companion/simulator.sh" start "$tmp/package" "$tmp/evaluation" "$tmp/simulator" >/dev/null
grep -Fx -- '-evaluation-simulator' "$ARGS"
grep -Fx -- "$tmp/companion/bin/zpr-simulator" "$ARGS"
grep -Fx -- "$digest" "$ARGS"
cp "$ARGS" "$tmp/original-args"
printf 'different-build\n' > "$tmp/package/BUILD-INFO.txt"
checksum
if sh "$tmp/companion/simulator.sh" start "$tmp/package" "$tmp/evaluation" "$tmp/simulator" >"$tmp/error" 2>&1; then
    echo "mismatched pair accepted" >&2
    exit 1
fi
grep 'build identities do not match' "$tmp/error"
printf 'tampered\n' >> "$tmp/companion/bin/zpr-simulator"
if sh "$tmp/companion/simulator.sh" start "$tmp/package" "$tmp/evaluation" "$tmp/simulator" >"$tmp/error" 2>&1; then
    echo "tampered companion accepted" >&2
    exit 1
fi
grep 'Simulator package verification failed' "$tmp/error"
cmp "$ARGS" "$tmp/original-args"
printf 'PASS: paired build identity, exact launch arguments, tamper rejection\n'
