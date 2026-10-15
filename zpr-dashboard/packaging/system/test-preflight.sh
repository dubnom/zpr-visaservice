#!/bin/sh
set -eu

directory=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
temporary=$(mktemp -d)
trap 'rm -f "$temporary/preflight.sh" "$temporary/os-release" "$temporary/output" "$temporary/calls" "$temporary/bin/uname" "$temporary/bin/docker" "$temporary/docker-disabled"; rmdir "$temporary/bin" "$temporary"' EXIT
trap 'exit 1' HUP INT TERM
mkdir "$temporary/bin"
sed "s|/etc/os-release|$temporary/os-release|g" "$directory/preflight.sh" > "$temporary/preflight.sh"

cat > "$temporary/bin/uname" <<'EOF'
#!/bin/sh
case "$1" in
    -s) printf '%s\n' "$TEST_KERNEL" ;;
    -m) printf '%s\n' "$TEST_ARCH" ;;
    *) exit 90 ;;
esac
EOF
cat > "$temporary/bin/docker" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >> "$TEST_CALLS"
case "$*" in
    "context inspect --format {{.Endpoints.docker.Host}}"|"context inspect local --format {{.Endpoints.docker.Host}}")
        [ "$TEST_CONTEXT_FAILURE" = no ] || exit 1
        printf '%s\n' "$TEST_ENDPOINT" ;;
    "info --format {{.OSType}} {{.Architecture}} {{.ServerVersion}}")
        [ "$TEST_ENGINE_FAILURE" = no ] || exit 1
        printf '%s\n' "$TEST_ENGINE" ;;
    "compose version --short")
        [ "$TEST_COMPOSE_FAILURE" = no ] || exit 1
        printf '%s\n' "$TEST_COMPOSE" ;;
    *) echo "Unexpected or mutating Docker operation" >&2; exit 90 ;;
esac
EOF
chmod 700 "$temporary/bin/uname" "$temporary/bin/docker"
export TEST_CALLS="$temporary/calls"
export TEST_KERNEL TEST_ARCH TEST_ENDPOINT TEST_ENGINE TEST_COMPOSE
export TEST_CONTEXT_FAILURE TEST_ENGINE_FAILURE TEST_COMPOSE_FAILURE
unset DOCKER_HOST DOCKER_CONTEXT

reset_case() {
    TEST_KERNEL=Linux
    TEST_ARCH=x86_64
    TEST_ENDPOINT=unix:///var/run/docker.sock
    TEST_ENGINE='linux x86_64 28.0.0'
    TEST_COMPOSE=2.35.0
    TEST_CONTEXT_FAILURE=no
    TEST_ENGINE_FAILURE=no
    TEST_COMPOSE_FAILURE=no
    printf 'ID=ubuntu\nVERSION_ID="24.04"\n' > "$temporary/os-release"
    : > "$temporary/calls"
    unset DOCKER_HOST DOCKER_CONTEXT
}

check() {
    expected=$1
    message=$2
    shift 2
    status=0
    PATH="$temporary/bin" /bin/sh "$temporary/preflight.sh" "$@" > "$temporary/output" 2>&1 || status=$?
    if [ "$status" -ne "$expected" ] || ! grep -F "$message" "$temporary/output" >/dev/null; then
        cat "$temporary/output"
        echo "Unexpected status $status (wanted $expected) or missing diagnostic: $message" >&2
        exit 1
    fi
    printf 'PASS: %s\n' "$message"
}

reset_case
check 0 "NOT production-readiness"
reset_case
TEST_KERNEL=Darwin
check 1 "requires Linux"
[ ! -s "$temporary/calls" ]
reset_case
TEST_ARCH=aarch64
check 1 "requires an x86-64 host"
reset_case
printf 'ID=debian\nVERSION_ID=12\n' > "$temporary/os-release"
check 1 "Ubuntu 24.04 LTS"
reset_case
printf 'ID=ubuntu\nVERSION_ID=22.04\n' > "$temporary/os-release"
check 1 "Ubuntu 24.04 LTS"
reset_case
rm "$temporary/os-release"
check 1 "Cannot read"
reset_case
mv "$temporary/bin/docker" "$temporary/docker-disabled"
check 1 "Docker CLI is missing"
mv "$temporary/docker-disabled" "$temporary/bin/docker"
reset_case
TEST_CONTEXT_FAILURE=yes
check 1 "Cannot inspect"
reset_case
TEST_ENDPOINT=ssh://remote
check 1 "local Unix socket"
! grep '^info ' "$temporary/calls"
reset_case
export DOCKER_HOST=tcp://remote:2375
check 1 "local Unix socket"
[ ! -s "$temporary/calls" ]
reset_case
export DOCKER_HOST=tcp://remote:2375 DOCKER_CONTEXT=local
check 0 "NOT production-readiness"
grep '^context inspect local ' "$temporary/calls" >/dev/null
reset_case
TEST_ENGINE_FAILURE=yes
check 1 "socket permissions"
reset_case
TEST_ENGINE='windows amd64 28.0.0'
check 1 "must report a Linux server"
reset_case
TEST_ENGINE='linux aarch64 28.0.0'
check 1 "Docker Engine must run on x86-64"
reset_case
TEST_ENGINE='linux amd64'
check 1 "must report a Linux server"
reset_case
TEST_COMPOSE_FAILURE=yes
check 1 "Compose v2 is unavailable"
reset_case
TEST_COMPOSE=1.29.0
check 1 "Compose v2 is required"
reset_case
TEST_ENGINE='linux amd64 28.0.0'
TEST_COMPOSE=v2.35.0
check 0 "NOT production-readiness"
reset_case
check 2 "usage:" --unknown
echo "All 19 host preflight regression cases passed using fixtures; no Docker daemon was contacted."
