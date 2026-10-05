#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
dashboard_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)

docker run --rm \
    --user "$(id -u):$(id -g)" \
    -e npm_config_cache=/tmp/npm-cache \
    -v "$dashboard_dir:/workspace" \
    -w /workspace \
    node:22-bookworm-slim \
    sh -c 'npm ci --ignore-scripts && mkdir -p cmd/zpr-web-dashboard/static/vendor && cp node_modules/pluralize/pluralize.js cmd/zpr-web-dashboard/static/vendor/pluralize.js && cp node_modules/pluralize/LICENSE cmd/zpr-web-dashboard/static/vendor/PLURALIZE-LICENSE'