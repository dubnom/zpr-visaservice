#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
    echo "Usage: sh probe-rust-runtime.sh /absolute/binaries /absolute/examples /absolute/new-evidence" >&2
    exit 2
fi
for directory in "$1" "$2" "$3"; do
    case "$directory" in
        /*) ;;
        *) echo "All paths must be absolute" >&2; exit 2 ;;
    esac
    case "$directory" in
        /|*/|*/.|*/..|*/../*|*/./*)
            echo "Paths must not contain dot components or trailing slashes" >&2
            exit 2
            ;;
    esac
done
if [ "$(uname -s)" != Linux ] || [ "$(uname -m)" != aarch64 ]; then
    echo "This engineering probe requires native Linux ARM64" >&2
    exit 1
fi
for tool in ldd sha256sum timeout openssl od awk grep; do
    command -v "$tool" >/dev/null 2>&1 || { echo "Missing probe prerequisite: $tool" >&2; exit 1; }
done
bin=$1
examples=$2
umask 077
mkdir -- "$3"
evidence=$(CDPATH='' cd -- "$3" && pwd)
for name in ph ph-cli vs vs-admin vsapikey zplc zpt zpr-web-dashboard; do
    binary="$bin/$name"
    if [ ! -f "$binary" ] || [ -L "$binary" ] || [ ! -x "$binary" ]; then
        echo "Missing regular executable: $binary" >&2
        exit 1
    fi
    header=$(od -An -tu1 -N6 "$binary" | awk '{$1=$1; print}')
    machine=$(od -An -tu1 -j18 -N2 "$binary" | awk '{$1=$1; print}')
    if [ "$header" != "127 69 76 70 2 1" ] || [ "$machine" != "183 0" ]; then
        echo "Executable is not Linux ARM64: $name" >&2
        exit 1
    fi
    sha256sum "$binary" >> "$evidence/binaries.sha256"
    if [ "$name" = zpr-web-dashboard ]; then continue; fi
    if ! ldd "$binary" > "$evidence/$name-libraries.txt" 2>&1 ||
       grep -q 'not found' "$evidence/$name-libraries.txt"; then
        cat "$evidence/$name-libraries.txt" >&2
        echo "Runtime loader prerequisites are not satisfied: $name" >&2
        exit 1
    fi
    argument=--version
    if [ "$name" = vsapikey ]; then argument=--help; fi
    if ! timeout 20 "$binary" "$argument" > "$evidence/$name-cli.txt" 2>&1; then
        cat "$evidence/$name-cli.txt" >&2
        echo "CLI probe failed: $name" >&2
        exit 1
    fi
done
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 \
    -out "$evidence/policy-signing.key" 2> "$evidence/key-generation.log"
openssl pkey -in "$evidence/policy-signing.key" -pubout \
    -out "$evidence/policy-signing-public.pem"
for organization in northstar great-lakes redwood load-lab velocity; do
    directory="$evidence/$organization"
    mkdir "$directory"
    "$bin/zpr-web-dashboard" -mode compose-policy -policy-root "$examples" \
        -policy-organization "$organization" -policy-output "$directory/runtime.zpl"
    config="$examples/organizations/$organization/policy-demo.zplc"
    if [ "$organization" = northstar ]; then config="$examples/northstar/policy-demo.zplc"; fi
    if ! timeout 60 "$bin/zplc" "$directory/runtime.zpl" -c "$config" \
        -k "$evidence/policy-signing.key" -d "$directory" -o runtime.bin2 \
        > "$directory/compiler.log" 2>&1; then
        cat "$directory/compiler.log" >&2
        echo "Signed policy build failed: $organization" >&2
        exit 1
    fi
    if [ ! -s "$directory/runtime.bin2" ]; then
        echo "Compiler produced no signed policy bundle: $organization" >&2
        exit 1
    fi
    sha256sum "$directory/runtime.bin2" >> "$evidence/policies.sha256"
done
printf '%% this is not valid ZPL syntax\n' > "$evidence/invalid.zpl"
if timeout 20 "$bin/zplc" "$evidence/invalid.zpl" \
    -c "$examples/northstar/policy-demo.zplc" -d "$evidence" -o invalid.bin2 \
    > "$evidence/invalid-compiler.log" 2>&1; then
    echo "Compiler accepted invalid policy syntax" >&2
    exit 1
else
    result=$?
    if [ "$result" -ne 1 ] || [ -e "$evidence/invalid.bin2" ] ||
       ! grep -qi 'error' "$evidence/invalid-compiler.log"; then
        echo "Invalid policy did not produce the expected compiler rejection" >&2
        exit 1
    fi
fi
printf '%s\n' 'PASS: seven Rust CLIs, five fresh-key signed policy builds and invalid-policy rejection.' \
    'Compiler warnings are retained, not suppressed. No policy was installed.' \
    'Evidence contains a newly generated private signing key: keep it private and exclude from release artifacts.' \
    > "$evidence/result.txt"
cat "$evidence/result.txt"
