#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
IMAGE=${ZPR_LDAP_UI_IMAGE:-zpr-ldap-ui:phpldapadmin-1.2.6.7-4}

docker build \
    --platform linux/amd64 \
    --tag "$IMAGE" \
    --file "$SCRIPT_DIR/ldap-ui.Dockerfile" \
    "$SCRIPT_DIR"

printf 'Built local-only LDAP GUI image: %s\n' "$IMAGE"
