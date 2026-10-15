#!/bin/sh
set -eu

fail() {
    printf 'FAIL: %s\n' "$1" >&2
    exit 1
}

if [ "$#" -ne 0 ]; then
    echo "usage: sh preflight.sh" >&2
    exit 2
fi

[ "$(uname -s)" = Linux ] || fail "The server appliance requires Linux."
[ "$(uname -m)" = x86_64 ] || fail "The server appliance requires an x86-64 host."
[ -r /etc/os-release ] || fail "Cannot read /etc/os-release to identify the host."
os_id=
os_version=
while IFS='=' read -r name value; do
    case "$value" in
        \"*\") value=${value#\"}; value=${value%\"} ;;
        \'*\') value=${value#\'}; value=${value%\'} ;;
    esac
    case "$name" in
        ID) os_id=$value ;;
        VERSION_ID) os_version=$value ;;
    esac
done < /etc/os-release
[ "$os_id" = ubuntu ] && [ "$os_version" = 24.04 ] ||
    fail "The supported server target is Ubuntu 24.04 LTS."
echo "PASS: Ubuntu 24.04 LTS x86-64 host."

command -v docker >/dev/null 2>&1 ||
    fail "Docker CLI is missing; install the approved Docker Engine and Compose v2 before retrying."

# DOCKER_CONTEXT overrides DOCKER_HOST in the Docker CLI.
if [ -n "${DOCKER_CONTEXT:-}" ]; then
    endpoint=$(docker context inspect "$DOCKER_CONTEXT" --format '{{.Endpoints.docker.Host}}') ||
        fail "Cannot inspect the selected Docker context."
elif [ -n "${DOCKER_HOST:-}" ]; then
    endpoint=$DOCKER_HOST
else
    endpoint=$(docker context inspect --format '{{.Endpoints.docker.Host}}') ||
        fail "Cannot inspect the current Docker context."
fi
case "$endpoint" in
    unix:///*) ;;
    *) fail "Docker must use a local Unix socket, not a remote engine." ;;
esac

engine=$(docker info --format '{{.OSType}} {{.Architecture}} {{.ServerVersion}}') ||
    fail "Cannot query Docker Engine; check daemon availability and socket permissions. No permissions were changed."
engine_os=
engine_arch=
engine_version=
extra=
IFS=' ' read -r engine_os engine_arch engine_version extra <<EOF
$engine
EOF
[ "$engine_os" = linux ] && [ -n "$engine_version" ] && [ -z "$extra" ] ||
    fail "Docker Engine must report a Linux server and version."
case "$engine_arch" in
    amd64|x86_64) ;;
    *) fail "Docker Engine must run on x86-64, not an emulated or different-architecture server." ;;
esac
printf 'PASS: Local Linux x86-64 Docker Engine %s is reachable.\n' "$engine_version"

compose=$(docker compose version --short) ||
    fail "Docker Compose v2 is unavailable."
case "$compose" in
    2.*|v2.*) ;;
    *) fail "Docker Compose v2 is required." ;;
esac
printf 'PASS: Docker Compose %s.\n' "$compose"
echo "Host baseline checks passed; this is NOT production-readiness or release certification."
echo "Resource sizing, Docker version qualification, TUN/capabilities, ports, DNS/TLS, identity, storage and application health remain unverified."
