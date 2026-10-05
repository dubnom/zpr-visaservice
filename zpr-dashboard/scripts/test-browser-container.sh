#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
dashboard_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)

docker run --rm \
    --user "$(id -u):$(id -g)" \
    -e HOME=/tmp \
    -e npm_config_cache=/tmp/npm-cache \
    -v "$dashboard_dir:/workspace" \
    -w /workspace \
    mcr.microsoft.com/playwright:v1.63.0-noble \
    sh -c 'npm ci --ignore-scripts && npm run test:browser -- --workers=1'