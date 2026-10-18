#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
mkdir -p "$temporary/bin"
cat > "$temporary/bin/docker" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >> "$DOCKER_CALL_LOG"
case "$*" in
    "image inspect --format {{.Os}}/{{.Architecture}} demo-image")
        case "${DOCKER_IMAGE_RESULT:-linux/amd64}" in
            missing) exit 1 ;;
            *) printf '%s\n' "$DOCKER_IMAGE_RESULT" ;;
        esac
        ;;
    "build -f Dockerfile.simulator -t demo-image dashboard")
        exit 0
        ;;
    *)
        echo "unexpected docker arguments: $*" >&2
        exit 3
        ;;
esac
EOF
chmod +x "$temporary/bin/docker"
PATH="$temporary/bin:$PATH"
export PATH
DOCKER_CALL_LOG="$temporary/docker.log"
export DOCKER_CALL_LOG
. "$script_dir/simulator-image.sh"

ZPR_SIMULATOR_PREBUILT_IMAGE=0
export ZPR_SIMULATOR_PREBUILT_IMAGE
ensure_simulator_image demo-image Dockerfile.simulator dashboard
grep -qx 'build -f Dockerfile.simulator -t demo-image dashboard' "$DOCKER_CALL_LOG"

: > "$DOCKER_CALL_LOG"
ZPR_SIMULATOR_PREBUILT_IMAGE=1
export ZPR_SIMULATOR_PREBUILT_IMAGE
DOCKER_IMAGE_RESULT=linux/amd64
export DOCKER_IMAGE_RESULT
ensure_simulator_image demo-image Dockerfile.simulator dashboard
grep -qx 'image inspect --format {{.Os}}/{{.Architecture}} demo-image' "$DOCKER_CALL_LOG"

: > "$DOCKER_CALL_LOG"
if DOCKER_IMAGE_RESULT=missing ensure_simulator_image demo-image Dockerfile.simulator dashboard; then
    echo "missing prebuilt image unexpectedly succeeded" >&2
    exit 1
fi

: > "$DOCKER_CALL_LOG"
if DOCKER_IMAGE_RESULT=linux/arm64 ensure_simulator_image demo-image Dockerfile.simulator dashboard; then
    echo "wrong-platform prebuilt image unexpectedly succeeded" >&2
    exit 1
fi

: > "$DOCKER_CALL_LOG"
ZPR_SIMULATOR_PREBUILT_IMAGE=yes
export ZPR_SIMULATOR_PREBUILT_IMAGE
if ensure_simulator_image demo-image Dockerfile.simulator dashboard; then
    echo "invalid prebuilt-image setting unexpectedly succeeded" >&2
    exit 1
fi
if [ -s "$DOCKER_CALL_LOG" ]; then
    echo "invalid setting invoked Docker" >&2
    exit 1
fi

echo "Simulator image selection tests passed."
