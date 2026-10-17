#!/bin/sh
set -eu

fail() {
    printf 'FAIL: %s\n' "$1" >&2
    exit 1
}

if [ "$#" -ne 4 ]; then
    echo "usage: sh start-control-room.sh <company.json> <operator-oidc.json> <compose.yaml> <local-image>" >&2
    exit 2
fi

config_file=$1
operator_oidc_file=$2
compose_file=$3
image=$4
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
system_config=${ZPR_SYSTEM_CONFIG:-zpr-system-config}
project=zpr-control-room-engineering

command -v "$system_config" >/dev/null 2>&1 ||
    fail "zpr-system-config is missing; build the engineering validator before retrying."

sh "$script_dir/preflight.sh"
"$system_config" -config "$config_file" \
    -render-operator-oidc-config "$operator_oidc_file" \
    -render-control-room-compose "$compose_file" \
    -control-room-image "$image"

docker image inspect "$image" >/dev/null 2>&1 ||
    fail "Control Room image is not available locally; build it first. No image pull was attempted."

if ! docker compose --project-name "$project" -f "$compose_file" config --quiet; then
    fail "Control Room Compose definition is invalid."
fi

services=$(docker compose --project-name "$project" -f "$compose_file" config --services) ||
    fail "Cannot inspect Control Room Compose services."
[ "$services" = control-room ] ||
    fail "Engineering descriptor must contain exactly the Control Room service."

images=$(docker compose --project-name "$project" -f "$compose_file" config --images) ||
    fail "Cannot inspect Control Room Compose image."
[ "$images" = "$image" ] ||
    fail "Compose image differs from the explicitly selected local image."

existing=$(docker compose --project-name "$project" -f "$compose_file" ps --all --services) ||
    fail "Cannot inspect the isolated engineering Compose project."
[ -z "$existing" ] ||
    fail "The isolated engineering project already has a container; refusing to restart or replace it."

docker compose --project-name "$project" -f "$compose_file" \
    up -d --no-build --no-deps control-room ||
    fail "Control Room container start failed; no cleanup or rollback was attempted."

running=$(docker compose --project-name "$project" -f "$compose_file" ps --status running --services) ||
    fail "Cannot inspect Control Room process state; container was left for diagnosis."
[ "$running" = control-room ] ||
    fail "Control Room process is not running; container was left for diagnosis."

echo "Control Room process is running in the isolated engineering project."
echo "OIDC login, Control-Service readiness, change review and policy/packet behavior were not verified."
