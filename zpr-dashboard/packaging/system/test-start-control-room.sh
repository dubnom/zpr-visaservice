#!/bin/sh
set -eu

directory=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
temporary=$(mktemp -d)
trap 'rm -f "$temporary/app/start-control-room.sh" "$temporary/app/preflight.sh" "$temporary/bin/docker" "$temporary/bin/zpr-system-config" "$temporary/company.json" "$temporary/operator-oidc.json" "$temporary/compose.yaml" "$temporary/calls" "$temporary/output"; rmdir "$temporary/app" "$temporary/bin" "$temporary"' EXIT
trap 'exit 1' HUP INT TERM
mkdir "$temporary/app" "$temporary/bin"
cp "$directory/start-control-room.sh" "$temporary/app/start-control-room.sh"
cat > "$temporary/app/preflight.sh" <<'EOF'
#!/bin/sh
[ "${TEST_PREFLIGHT_FAILURE:-no}" = no ] || {
    echo "FAIL: test host preflight failed" >&2
    exit 1
}
printf 'PASS: test host preflight\n'
EOF
touch "$temporary/company.json"

cat > "$temporary/bin/zpr-system-config" <<'EOF'
#!/bin/sh
printf 'validator %s\n' "$*" >> "$TEST_CALLS"
[ "${TEST_VALIDATOR_FAILURE:-no}" = no ] || {
    echo "FAIL: synthetic validator failure" >&2
    exit 1
}
oidc=
compose=
while [ "$#" -gt 0 ]; do
    case "$1" in
        -render-operator-oidc-config) shift; oidc=$1 ;;
        -render-control-room-compose) shift; compose=$1 ;;
    esac
    shift
done
[ -n "$oidc" ] && [ -n "$compose" ] || exit 90
printf '{}\n' > "$oidc"
printf 'engineering fixture\n' > "$compose"
EOF

cat > "$temporary/bin/docker" <<'EOF'
#!/bin/sh
printf 'docker %s\n' "$*" >> "$TEST_CALLS"
case "$*" in
    "image inspect "*)
        [ "${TEST_IMAGE_PRESENT:-yes}" = yes ] || exit 1 ;;
    "compose --project-name zpr-control-room-engineering -f $TEST_COMPOSE config --quiet")
        [ "${TEST_COMPOSE_INVALID:-no}" = no ] || exit 1 ;;
    "compose --project-name zpr-control-room-engineering -f $TEST_COMPOSE config --services")
        printf '%s\n' "${TEST_SERVICES:-control-room}" ;;
    "compose --project-name zpr-control-room-engineering -f $TEST_COMPOSE config --images")
        printf '%s\n' "${TEST_IMAGES:-zpr-control-room:engineering}" ;;
    "compose --project-name zpr-control-room-engineering -f $TEST_COMPOSE ps --all --services")
        printf '%s\n' "${TEST_EXISTING:-}" ;;
    "compose --project-name zpr-control-room-engineering -f $TEST_COMPOSE up -d --no-build --no-deps control-room")
        [ "${TEST_START_FAILURE:-no}" = no ] || exit 1 ;;
    "compose --project-name zpr-control-room-engineering -f $TEST_COMPOSE ps --status running --services")
        printf '%s\n' "${TEST_RUNNING:-control-room}" ;;
    *) echo "Unexpected Docker operation" >&2; exit 90 ;;
esac
EOF
chmod 700 "$temporary/bin/docker" "$temporary/bin/zpr-system-config"

export TEST_CALLS="$temporary/calls"
export TEST_COMPOSE="$temporary/compose.yaml"
export ZPR_SYSTEM_CONFIG="$temporary/bin/zpr-system-config"
export PATH="$temporary/bin:$PATH"
export TEST_PREFLIGHT_FAILURE TEST_VALIDATOR_FAILURE TEST_IMAGE_PRESENT TEST_COMPOSE_INVALID
export TEST_SERVICES TEST_IMAGES TEST_EXISTING TEST_START_FAILURE TEST_RUNNING

reset_case() {
    TEST_PREFLIGHT_FAILURE=no
    TEST_VALIDATOR_FAILURE=no
    TEST_IMAGE_PRESENT=yes
    TEST_COMPOSE_INVALID=no
    TEST_SERVICES=control-room
    TEST_IMAGES=zpr-control-room:engineering
    TEST_EXISTING=
    TEST_START_FAILURE=no
    TEST_RUNNING=control-room
    rm -f "$temporary/operator-oidc.json" "$temporary/compose.yaml"
    : > "$temporary/calls"
}

check() {
    expected=$1
    message=$2
    shift 2
    status=0
    /bin/sh "$temporary/app/start-control-room.sh" "$@" > "$temporary/output" 2>&1 || status=$?
    if [ "$status" -ne "$expected" ] || ! grep -F "$message" "$temporary/output" >/dev/null; then
        cat "$temporary/output"
        echo "Unexpected status $status (wanted $expected) or missing diagnostic: $message" >&2
        exit 1
    fi
    printf 'PASS: %s\n' "$message"
}

reset_case
check 2 "usage:" 
[ ! -s "$temporary/calls" ]

reset_case
check 0 "OIDC login, Control-Service readiness" \
    "$temporary/company.json" "$temporary/operator-oidc.json" "$temporary/compose.yaml" "zpr-control-room:engineering"
grep -F "up -d --no-build --no-deps control-room" "$temporary/calls" >/dev/null
grep -F "ps --status running --services" "$temporary/calls" >/dev/null

reset_case
TEST_PREFLIGHT_FAILURE=yes
check 1 "test host preflight failed" \
    "$temporary/company.json" "$temporary/operator-oidc.json" "$temporary/compose.yaml" "zpr-control-room:engineering"
[ ! -s "$temporary/calls" ]

reset_case
TEST_VALIDATOR_FAILURE=yes
check 1 "synthetic validator failure" \
    "$temporary/company.json" "$temporary/operator-oidc.json" "$temporary/compose.yaml" "zpr-control-room:engineering"
! grep -F "image inspect" "$temporary/calls" >/dev/null

reset_case
TEST_IMAGE_PRESENT=no
check 1 "not available locally" \
    "$temporary/company.json" "$temporary/operator-oidc.json" "$temporary/compose.yaml" "zpr-control-room:engineering"
! grep -F " up -d " "$temporary/calls" >/dev/null

reset_case
TEST_COMPOSE_INVALID=yes
check 1 "Compose definition is invalid" \
    "$temporary/company.json" "$temporary/operator-oidc.json" "$temporary/compose.yaml" "zpr-control-room:engineering"
! grep -F " up -d " "$temporary/calls" >/dev/null

reset_case
TEST_SERVICES='control-room
policy-service'
check 1 "exactly the Control Room service" \
    "$temporary/company.json" "$temporary/operator-oidc.json" "$temporary/compose.yaml" "zpr-control-room:engineering"
! grep -F " up -d " "$temporary/calls" >/dev/null

reset_case
TEST_EXISTING=control-room
check 1 "refusing to restart or replace it" \
    "$temporary/company.json" "$temporary/operator-oidc.json" "$temporary/compose.yaml" "zpr-control-room:engineering"
! grep -F " up -d " "$temporary/calls" >/dev/null

reset_case
TEST_START_FAILURE=yes
check 1 "no cleanup or rollback" \
    "$temporary/company.json" "$temporary/operator-oidc.json" "$temporary/compose.yaml" "zpr-control-room:engineering"
! grep -F " down " "$temporary/calls" >/dev/null
! grep -F " rm " "$temporary/calls" >/dev/null

reset_case
TEST_RUNNING=stopped
check 1 "left for diagnosis" \
    "$temporary/company.json" "$temporary/operator-oidc.json" "$temporary/compose.yaml" "zpr-control-room:engineering"
! grep -F " down " "$temporary/calls" >/dev/null
! grep -F " rm " "$temporary/calls" >/dev/null

echo "All 10 Control Room startup cases passed using command stubs; no Docker daemon or service was contacted."
