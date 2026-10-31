#!/bin/sh
set -eu
umask 077
fail() { printf 'organization-policy-service: %s\n' "$*" >&2; exit 1; }
[ "$#" -eq 1 ] || fail "usage: organization-policy-service.sh ORGANIZATION"
organization=$1
case "$organization" in
    northstar) prefix=20 ;;
    great-lakes) prefix=21 ;;
    load-lab) prefix=22 ;;
    redwood) prefix=23 ;;
    velocity) prefix=24 ;;
    *) fail "unknown operator organization" ;;
esac
root=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
(cd "$root" && sha256sum --check --strict SHA256SUMS >/dev/null) ||
    fail "operator binaries or immutable inputs changed"
state=$root/state/$organization
policy=$root/policy/$organization
[ -d "$policy/tmp" ] || fail "protected operator Policy-Service configuration is not installed"
export TMPDIR="$policy/tmp"
export ZPR_POLICY_SERVICE_LISTEN="127.0.0.1:${prefix}989"
export ZPR_POLICY_SERVICE_CERT_FILE="$state/server-chain.pem"
export ZPR_POLICY_SERVICE_KEY_FILE="$state/server-key.pem"
export ZPR_POLICY_SERVICE_CLIENT_CA_FILE="$state/ca.pem"
export ZPR_POLICY_ORGANIZATION_ID="$organization"
export ZPR_POLICY_ORGANIZATION_NAME="$organization"
export ZPR_POLICY_DB_FILE="$policy/records.sqlite"
export ZPR_POLICY_CONFIG_FILE="$state/dp-policy.zplc"
export ZPR_POLICY_SOURCE_FILE="$state/dp-policy.zpl"
export ZPR_POLICY_SEED_NAME="Retained bounded installation policy (not original scenarios)"
export ZPR_POLICY_STAGE_DIR="$policy/staged"
export ZPR_POLICY_STAGE_CONFIG_FILE="$state/dp-policy.zplc"
export ZPR_POLICY_STAGE_SIGNING_KEY_FILE="$state/dp-policy-signing.key"
export ZPR_POLICY_APPLY_CONFIG_FILE="$policy/service-apply.json"
export ZPR_ZPLC_BIN="$root/bin/zplc"
# Do not inherit unrelated provider or Simulator configuration.
exec env -i PATH="$PATH" HOME="$HOME" TMPDIR="$TMPDIR" \
    ZPR_POLICY_SERVICE_LISTEN="$ZPR_POLICY_SERVICE_LISTEN" \
    ZPR_POLICY_SERVICE_CERT_FILE="$ZPR_POLICY_SERVICE_CERT_FILE" \
    ZPR_POLICY_SERVICE_KEY_FILE="$ZPR_POLICY_SERVICE_KEY_FILE" \
    ZPR_POLICY_SERVICE_CLIENT_CA_FILE="$ZPR_POLICY_SERVICE_CLIENT_CA_FILE" \
    ZPR_POLICY_ORGANIZATION_ID="$ZPR_POLICY_ORGANIZATION_ID" \
    ZPR_POLICY_ORGANIZATION_NAME="$ZPR_POLICY_ORGANIZATION_NAME" \
    ZPR_POLICY_DB_FILE="$ZPR_POLICY_DB_FILE" \
    ZPR_POLICY_CONFIG_FILE="$ZPR_POLICY_CONFIG_FILE" \
    ZPR_POLICY_SOURCE_FILE="$ZPR_POLICY_SOURCE_FILE" \
    ZPR_POLICY_SEED_NAME="$ZPR_POLICY_SEED_NAME" \
    ZPR_POLICY_STAGE_DIR="$ZPR_POLICY_STAGE_DIR" \
    ZPR_POLICY_STAGE_CONFIG_FILE="$ZPR_POLICY_STAGE_CONFIG_FILE" \
    ZPR_POLICY_STAGE_SIGNING_KEY_FILE="$ZPR_POLICY_STAGE_SIGNING_KEY_FILE" \
    ZPR_POLICY_APPLY_CONFIG_FILE="$ZPR_POLICY_APPLY_CONFIG_FILE" \
    ZPR_ZPLC_BIN="$ZPR_ZPLC_BIN" \
    "$root/bin/zpr-policy-service" -mode policy-service
