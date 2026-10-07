#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
dashboard_dir=$(CDPATH='' cd -- "$script_dir/.." && pwd)

# The browser image has Node/Chromium, not Go. Build the enrollment fixture
# for the Docker server architecture before starting its browser tests.
case "$(docker info --format '{{.Architecture}}')" in
    aarch64|arm64) architecture=arm64 ;;
    x86_64|amd64) architecture=amd64 ;;
    *) echo "Unsupported Docker architecture for enrollment browser tests" >&2; exit 1 ;;
esac
artifact_dir=$(mktemp -d)
trap 'rm -f -- "$artifact_dir/setup" "$artifact_dir/enrollment-tests"; rmdir -- "$artifact_dir"' EXIT
trap 'exit 1' HUP INT TERM
(cd "$dashboard_dir" && CGO_ENABLED=0 GOOS=linux GOARCH="$architecture" go build -o "$artifact_dir/setup" ./cmd/zpr-enrollment-setup)
(cd "$dashboard_dir" && CGO_ENABLED=0 GOOS=linux GOARCH="$architecture" go test -c -o "$artifact_dir/enrollment-tests" ./internal/enrollment)
chmod 0755 "$artifact_dir"

docker run --rm \
    --user "$(id -u):$(id -g)" \
    -e HOME=/tmp \
    -e npm_config_cache=/tmp/npm-cache \
    -e ZPR_SETUP_TEST_BINARY=/enrollment-fixture/setup \
    -e ZPR_ENROLLMENT_TEST_BINARY=/enrollment-fixture/enrollment-tests \
    -v "$artifact_dir:/enrollment-fixture:ro" \
    -v "$dashboard_dir:/workspace" \
    -w /workspace \
    mcr.microsoft.com/playwright:v1.63.0-noble \
    sh -c 'npm ci --ignore-scripts && npm run test:browser -- --workers=1 "$@"' sh "$@"