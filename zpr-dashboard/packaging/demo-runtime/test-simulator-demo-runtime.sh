#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)
source_runtime_script=$script_dir/simulator-demo-runtime.sh
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/zpr-simulator-runtime-test.XXXXXX")
temp_dir=$(CDPATH='' cd -- "$temp_dir" && pwd -P)
socket_pid=

cleanup() {
    if [ -n "$socket_pid" ]; then
        kill "$socket_pid" 2>/dev/null || true
        wait "$socket_pid" 2>/dev/null || true
    fi
    rm -rf "$temp_dir"
}
trap cleanup EXIT HUP INT TERM

package_dir=$temp_dir/install/test-release/simulator
zpr_package_dir=$temp_dir/install/test-release/zpr-demo
state_dir=$temp_dir/state
stub_dir=$temp_dir/bin
mkdir -p "$package_dir/runtime/organizations" "$stub_dir"
mkdir -p "$zpr_package_dir/runtime"
cp "$source_runtime_script" "$script_dir/config.toml" "$script_dir/simulation-environment.json" "$package_dir/runtime/"
cp "$script_dir/organizations/demo.json" "$package_dir/runtime/organizations/"
runtime_script=$package_dir/runtime/simulator-demo-runtime.sh
printf '%s\n' 'zpr-simulator:test-release' >"$package_dir/runtime/image-reference"
printf '%s\n' 'zpr-demo:test-release' >"$zpr_package_dir/runtime/image-reference"

image_id=sha256:0000000000000000000000000000000000000000000000000000000000000000
zpr_image_id=sha256:1111111111111111111111111111111111111111111111111111111111111111
cat >"$package_dir/manifest.json" <<EOF
{
  "schema":"zpr.demo.runtime-package.v2",
  "status":"demo-unqualified-runtime-package",
  "release_id":"test-release",
  "platform":"linux/amd64",
  "package_id":"simulator-image",
  "required_peer_package_id":"zpr-demo-image",
  "peer_image_id":"$zpr_image_id",
  "peer_docker_image_id":"$zpr_image_id",
  "component":{
    "id":"simulator-image",
    "image_reference":"zpr-simulator:test-release",
    "image_id":"$image_id",
    "docker_image_id":"$image_id"
  }
}
EOF
cat >"$zpr_package_dir/manifest.json" <<EOF
{
  "schema":"zpr.demo.runtime-package.v2",
  "status":"demo-unqualified-runtime-package",
  "release_id":"test-release",
  "platform":"linux/amd64",
  "package_id":"zpr-demo-image",
  "required_peer_package_id":"simulator-image",
  "peer_image_id":"$image_id",
  "peer_docker_image_id":"$image_id",
  "component":{
    "id":"zpr-demo-image",
    "image_reference":"zpr-demo:test-release",
    "image_id":"$zpr_image_id",
    "docker_image_id":"$zpr_image_id"
  }
}
EOF

docker_socket=$temp_dir/docker.sock
python3 -c 'import socket,sys,time; s=socket.socket(socket.AF_UNIX); s.bind(sys.argv[1]); time.sleep(300)' "$docker_socket" &
socket_pid=$!
attempt=0
while [ ! -S "$docker_socket" ] && [ "$attempt" -lt 50 ]; do
    sleep 0.1
    attempt=$((attempt + 1))
done
[ -S "$docker_socket" ] || {
    echo "test-simulator-demo-runtime: could not create fake Docker socket" >&2
    exit 1
}

cat >"$stub_dir/docker" <<'SH'
#!/bin/sh
set -eu
printf '%s\n' "$*" >>"$FAKE_DOCKER_LOG"
case "$1" in
    context)
        case "$2" in
            show) printf '%s\n' fake-local ;;
            inspect) printf 'unix://%s\n' "$FAKE_DOCKER_SOCKET" ;;
            *) exit 2 ;;
        esac
        ;;
    image)
        [ "$2" = inspect ] || exit 2
        case "$*" in
            *zpr.simulator.packaged-provisioning*) printf '1\n' ;;
            *) printf '%s\n' "$FAKE_DOCKER_IMAGE_METADATA" ;;
        esac
        ;;
    container)
        case "$2" in
            ls)
                if [ -f "$FAKE_DOCKER_CONTAINER_STATE" ]; then
                    printf '%s\n' zpr-simulator-demo-test-release
                fi
                ;;
            inspect)
                if [ ! -f "$FAKE_DOCKER_CONTAINER_STATE" ]; then
                    exit 1
                fi
                IFS='|' read -r package release image published state <"$FAKE_DOCKER_CONTAINER_STATE"
                printf '%s|%s|%s|%s|%s\n' "$package" "$release" "$image" "$published" "$state"
                ;;
            run)
                [ "$3" = -d ] || exit 2
                mounts=0
                published=
                socket_mount=no
                runtime_mount=no
                state_mount=no
                privileged=no
                shift 3
                while [ "$#" -gt 0 ]; do
                    case "$1" in
                        -v)
                            shift
                            mounts=$((mounts + 1))
                            case "$1" in
                                "$FAKE_DOCKER_SOCKET":/var/run/docker.sock) socket_mount=yes ;;
                                *:/opt/zpr-simulator/runtime:ro) runtime_mount=yes ;;
                                *:/var/lib/zpr-simulator) state_mount=yes ;;
                                *) echo "unexpected mount: $1" >&2; exit 1 ;;
                            esac
                            ;;
                        -p)
                            shift
                            published=$1
                            ;;
                        --privileged) privileged=yes ;;
                    esac
                    shift
                done
                [ "$mounts" -eq 3 ] && [ "$socket_mount" = yes ] &&
                    [ "$runtime_mount" = yes ] && [ "$state_mount" = yes ] &&
                    [ "$published" = "127.0.0.1:$FAKE_DOCKER_PORT:8788" ] &&
                    [ "$privileged" = no ] || {
                        echo "Simulator container request violated the fake-Docker test contract" >&2
                        exit 1
                    }
                printf 'simulator|test-release|%s|127.0.0.1:%s|running\n' \
                    "$FAKE_DOCKER_IMAGE_ID" "$FAKE_DOCKER_PORT" >"$FAKE_DOCKER_CONTAINER_STATE"
                printf '%s\n' fake-container-id
                ;;
            stop)
                [ -f "$FAKE_DOCKER_CONTAINER_STATE" ] || exit 1
                IFS='|' read -r package release image published state <"$FAKE_DOCKER_CONTAINER_STATE"
                printf '%s|%s|%s|%s|exited\n' "$package" "$release" "$image" "$published" >"$FAKE_DOCKER_CONTAINER_STATE"
                printf '%s\n' "$4"
                ;;
            start)
                [ -f "$FAKE_DOCKER_CONTAINER_STATE" ] || exit 1
                IFS='|' read -r package release image published state <"$FAKE_DOCKER_CONTAINER_STATE"
                printf '%s|%s|%s|%s|running\n' "$package" "$release" "$image" "$published" >"$FAKE_DOCKER_CONTAINER_STATE"
                printf '%s\n' "$3"
                ;;
            *) exit 2 ;;
        esac
        ;;
    *) exit 2 ;;
esac
SH
chmod +x "$stub_dir/docker"

cat >"$stub_dir/stat" <<'SH'
#!/bin/sh
set -eu
[ "$1" = -c ] && [ "$2" = %g ] || exit 2
printf '%s\n' "$FAKE_DOCKER_SOCKET_GID"
SH
chmod +x "$stub_dir/stat"

export PATH="$stub_dir:$PATH"
unset ZPR_SIMULATOR_PACKAGE_DIR DOCKER_CONTEXT DOCKER_HOST
export ZPR_SIMULATOR_STATE_DIR="$state_dir"
export ZPR_DEMO_PACKAGE_DIR="$zpr_package_dir"
export FAKE_DOCKER_LOG="$temp_dir/docker.log"
export FAKE_DOCKER_SOCKET="$docker_socket"
export FAKE_DOCKER_IMAGE_ID=$image_id
export FAKE_DOCKER_PORT=8788
export FAKE_DOCKER_IMAGE_METADATA="$image_id|linux|amd64"
fake_docker_socket_gid=$(id -g)
export FAKE_DOCKER_SOCKET_GID="$fake_docker_socket_gid"
export FAKE_DOCKER_CONTAINER_STATE="$temp_dir/container-state"
: >"$FAKE_DOCKER_LOG"

output=$("$runtime_script" status)
case "$output" in
    "Simulator stopped (no container for release test-release)") ;;
    *) echo "unexpected initial status: $output" >&2; exit 1 ;;
esac

if ZPR_SIMULATOR_PORT=abc "$runtime_script" start >"$temp_dir/invalid-port.out" 2>&1; then
    echo "invalid Simulator host port unexpectedly accepted" >&2
    exit 1
fi
grep -F "ZPR_SIMULATOR_PORT must be a decimal port" "$temp_dir/invalid-port.out" >/dev/null

output=$("$runtime_script" start 2>"$temp_dir/start.err")
grep -F "root-equivalent host access" "$temp_dir/start.err" >/dev/null
case "$output" in
    "Simulator container running (UI endpoint http://127.0.0.1:8788; application readiness not checked)") ;;
    *) echo "unexpected start result: $output" >&2; exit 1 ;;
esac
grep -F "container run" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-p 127.0.0.1:8788:8788" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-v $docker_socket:/var/run/docker.sock" "$FAKE_DOCKER_LOG" >/dev/null
demo_project="zprdemo-$(printf '%s' test-release | shasum -a 256 | cut -c1-12)"
grep -F -- "-e SIMULATION_DEMO_PROJECT=$demo_project" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATION_DEMO_IMAGE=$zpr_image_id" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATION_DEMO_RELEASE_ID=test-release" "$FAKE_DOCKER_LOG" >/dev/null
[ -d "$state_dir/workspace-db" ] && [ -d "$state_dir/published-directories" ] &&
    [ -d "$state_dir/scenarios" ] || {
        echo "Simulator state directories were not created" >&2
        exit 1
    }

run_count=$(grep -c "container run" "$FAKE_DOCKER_LOG")
"$runtime_script" start >/dev/null 2>&1
[ "$(grep -c "container run" "$FAKE_DOCKER_LOG")" -eq "$run_count" ] || {
    echo "start did not reuse the running container" >&2
    exit 1
}

cp "$zpr_package_dir/manifest.json" "$temp_dir/zpr-manifest.json"
python3 - "$zpr_package_dir/manifest.json" <<'PY'
import json
from pathlib import Path
import sys
path = Path(sys.argv[1])
manifest = json.loads(path.read_text())
manifest["component"]["docker_image_id"] = "sha256:" + "2" * 64
path.write_text(json.dumps(manifest))
PY
if "$runtime_script" status >"$temp_dir/mismatch.out" 2>&1; then
    echo "Simulator accepted a mismatched paired ZPR image" >&2
    exit 1
fi
grep -F "paired ZPR package does not match the installed Simulator release" "$temp_dir/mismatch.out" >/dev/null
mv "$temp_dir/zpr-manifest.json" "$zpr_package_dir/manifest.json"

"$runtime_script" stop >/dev/null
case "$("$runtime_script" status)" in
    "Simulator stopped (container zpr-simulator-demo-test-release; state exited)") ;;
    *) echo "stopped container state was not reported" >&2; exit 1 ;;
esac
test -d "$state_dir" || {
    echo "stop removed persistent Simulator state" >&2
    exit 1
}
"$runtime_script" start >/dev/null 2>&1
[ "$(grep -c "container run" "$FAKE_DOCKER_LOG")" -eq "$run_count" ] || {
    echo "start recreated rather than restarted the stopped container" >&2
    exit 1
}

if DOCKER_HOST=tcp://remote.example:2376 "$runtime_script" start >"$temp_dir/remote.out" 2>"$temp_dir/remote.err"; then
    echo "remote Docker endpoint unexpectedly accepted" >&2
    exit 1
fi
grep -F "local Unix-socket Docker endpoint" "$temp_dir/remote.err" >/dev/null

export FAKE_DOCKER_IMAGE_METADATA="sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff|linux|amd64"
if "$runtime_script" start >"$temp_dir/image.out" 2>"$temp_dir/image.err"; then
    echo "mismatched imported image unexpectedly accepted" >&2
    exit 1
fi
grep -F "does not match the package" "$temp_dir/image.err" >/dev/null

printf '%s\n' unrelated >"$FAKE_DOCKER_CONTAINER_STATE"
if "$runtime_script" status >"$temp_dir/collision.out" 2>"$temp_dir/collision.err"; then
    echo "unowned container name unexpectedly accepted" >&2
    exit 1
fi
grep -F "occupied by a different or modified container" "$temp_dir/collision.err" >/dev/null

rm -f "$FAKE_DOCKER_CONTAINER_STATE"
export FAKE_DOCKER_PORT=8789
export FAKE_DOCKER_IMAGE_METADATA="$image_id|linux|amd64"
output=$(ZPR_SIMULATOR_PORT=8789 ZPR_SIMULATOR_ENABLE_PROVISIONING=1 "$runtime_script" start)
case "$output" in
    "Simulator container running (UI endpoint http://127.0.0.1:8789; application readiness not checked)") ;;
    *) echo "custom Simulator host port was not honored: $output" >&2; exit 1 ;;
esac
grep -F -- "-p 127.0.0.1:8789:8788" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATION_PROVISION_RELEASE_ID=test-release" "$FAKE_DOCKER_LOG" >/dev/null
case "$(ZPR_SIMULATOR_PORT=8789 "$runtime_script" status)" in
    "Simulator container running (UI endpoint http://127.0.0.1:8789; application readiness not checked)") ;;
    *) echo "custom Simulator host port was not verified: $output" >&2; exit 1 ;;
esac
if ZPR_SIMULATOR_PORT=8790 ZPR_SIMULATOR_ENABLE_PROVISIONING=1 "$runtime_script" status >"$temp_dir/port-mismatch.out" 2>&1; then
    echo "existing container with a different port binding unexpectedly accepted" >&2
    exit 1
fi
grep -F "occupied by a different or modified container" "$temp_dir/port-mismatch.out" >/dev/null

echo "Simulator demo runtime shell tests passed (Docker fully stubbed; no services started)."
