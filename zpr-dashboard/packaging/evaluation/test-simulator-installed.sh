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
printf '%s\n' "${SIMULATION_SETUP_MODE-unset}" > "$SETUP_MODE"
EOF
chmod 700 "$tmp/tools/"* "$tmp/package/bin/zpr-lab-bootstrap"
export PATH="$tmp/tools:$PATH" ARGS="$tmp/args" SETUP_MODE="$tmp/setup-mode"
unset SIMULATION_SETUP_MODE
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
grep -Fx -- 'unset' "$SETUP_MODE"
SIMULATION_SETUP_MODE=ordinary-client sh "$tmp/companion/simulator.sh" start \
    "$tmp/package" "$tmp/evaluation" "$tmp/simulator" >/dev/null
grep -Fx -- 'ordinary-client' "$SETUP_MODE"
# Paired-operation options pass through verbatim to the bootstrap.
printf 'operator-config\n' > "$tmp/oidc.json"
printf 'operator-config\n' > "$tmp/retained.json"
printf 'operator-config\n' > "$tmp/bas-ca.pem"
sh "$tmp/companion/simulator.sh" start "$tmp/package" "$tmp/evaluation" "$tmp/simulator" \
    --full-catalog \
    --simulator-oidc-file "$tmp/oidc.json" \
    --retained-machine-runtime-file "$tmp/retained.json" \
    --enrollment-device-url https://device.example:24991 \
    --enrollment-bas-url https://bas.example:24996 \
    --enrollment-bas-authority auth.zpr:24996 \
    --enrollment-bas-ca-file "$tmp/bas-ca.pem" >/dev/null
grep -Fx -- '-simulator-oidc-file' "$ARGS"
grep -Fx -- "$tmp/oidc.json" "$ARGS"
grep -Fx -- '-retained-machine-runtime-file' "$ARGS"
grep -Fx -- "$tmp/retained.json" "$ARGS"
grep -Fx -- '-enrollment-device-url' "$ARGS"
grep -Fx -- 'https://device.example:24991' "$ARGS"
grep -Fx -- '-enrollment-bas-url' "$ARGS"
grep -Fx -- '-enrollment-bas-authority' "$ARGS"
grep -Fx -- 'auth.zpr:24996' "$ARGS"
grep -Fx -- '-enrollment-bas-ca-file' "$ARGS"
grep -Fx -- "$tmp/bas-ca.pem" "$ARGS"
grep -Fx -- '-catalog' "$ARGS"
cp "$ARGS" "$tmp/original-args"
# Unsupported options, relative paths, missing files and plain-HTTP URLs are refused.
if sh "$tmp/companion/simulator.sh" start "$tmp/package" "$tmp/evaluation" "$tmp/simulator" \
    --unknown-option value >"$tmp/error" 2>&1; then
    echo "unsupported option accepted" >&2
    exit 1
fi
grep 'unsupported option' "$tmp/error"
if sh "$tmp/companion/simulator.sh" start "$tmp/package" "$tmp/evaluation" "$tmp/simulator" \
    --simulator-oidc-file relative.json >"$tmp/error" 2>&1; then
    echo "relative operator file accepted" >&2
    exit 1
fi
grep 'absolute path' "$tmp/error"
if sh "$tmp/companion/simulator.sh" start "$tmp/package" "$tmp/evaluation" "$tmp/simulator" \
    --retained-machine-runtime-file "$tmp/missing.json" >"$tmp/error" 2>&1; then
    echo "missing operator file accepted" >&2
    exit 1
fi
grep 'does not exist' "$tmp/error"
if sh "$tmp/companion/simulator.sh" start "$tmp/package" "$tmp/evaluation" "$tmp/simulator" \
    --enrollment-device-url http://device.example:24991 >"$tmp/error" 2>&1; then
    echo "plain-HTTP device URL accepted" >&2
    exit 1
fi
grep 'HTTPS URL' "$tmp/error"
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
printf 'PASS: paired build identity, exact launch arguments, setup-mode and paired-operation option forwarding, fail-closed option validation, tamper rejection\n'
