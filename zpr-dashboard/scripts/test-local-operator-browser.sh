#!/bin/sh
set -eu
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
dashboard_dir=$(CDPATH='' cd -- "$script_dir/.." && pwd)
runtime_dir=$(CDPATH='' cd -- "$dashboard_dir/../../.local-runtime" && pwd)
directory="$runtime_dir/operator-login"
room_pin=$(openssl x509 -in "$directory/room.crt" -pubkey -noout | openssl pkey -pubin -outform DER | openssl dgst -sha256 -binary | openssl base64 -A)
idp_pin=$(openssl x509 -in "$directory/idp.crt" -pubkey -noout | openssl pkey -pubin -outform DER | openssl dgst -sha256 -binary | openssl base64 -A)
host_ip=$(docker run --rm --entrypoint /bin/sh zpr-simulator:local -c 'getent hosts host.docker.internal' | awk 'NR==1 {print $1}')
case "$host_ip" in ''|*[!0-9.]*) echo "Expected Docker Desktop IPv4 host address" >&2; exit 1 ;; esac
chrome_args=$(jq -nc --arg pins "$room_pin,$idp_pin" --arg ip "$host_ip" \
    '[("--host-resolver-rules=MAP localhost "+$ip+",MAP zpr-id.localhost "+$ip),("--ignore-certificate-errors-spki-list="+$pins)]')
# Container Chromium has its own trust store: pin these exact development TLS keys.
# No blanket ignoreHTTPSErrors, password traces, screenshots, or private-key mounts.
docker run --rm --user "$(id -u):$(id -g)" -e HOME=/tmp \
    -e ZPR_LOCAL_OPERATOR_DIR=/operator -e ZPR_LOCAL_OPERATOR_CHROME_ARGS="$chrome_args" \
    -v "$directory/stack.json:/operator/stack.json:ro" \
    -v "$directory/oidc.json:/operator/oidc.json:ro" \
    -v "$directory/operator-password:/operator/operator-password:ro" \
    -v "$dashboard_dir:/workspace" -w /workspace mcr.microsoft.com/playwright:v1.63.0-noble \
    sh -c 'npm run test:browser -- tests/browser/local-operator.spec.mjs --workers=1 --output /tmp/zpr-deployed-login-results "$@"' sh "$@"
