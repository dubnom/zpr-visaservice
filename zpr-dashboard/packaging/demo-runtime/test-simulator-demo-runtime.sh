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
northstar_package_dir=$temp_dir/northstar-install/test-release/simulator
northstar_zpr_package_dir=$temp_dir/northstar-install/test-release/zpr-demo
northstar_package_runtime_dir=$northstar_package_dir/runtime
northstar_root=$temp_dir/northstar-synthetic
northstar_lab_root=$northstar_root/lab
northstar_runtime_dir=$northstar_lab_root/runtime
northstar_route_output=$northstar_root/node-route
state_dir=$temp_dir/state
stub_dir=$temp_dir/bin
mkdir -p "$package_dir/runtime/organizations" "$package_dir/runtime/scenarios" \
    "$northstar_package_runtime_dir" "$northstar_root" "$northstar_route_output" \
    "$northstar_zpr_package_dir/runtime" "$stub_dir"
printf '%s\n' 'owned by W2' >"$northstar_route_output/route-output.txt"
mkdir -p "$zpr_package_dir/runtime"
cp "$source_runtime_script" "$script_dir/config.toml" "$script_dir/simulation-environment.json" "$package_dir/runtime/"
cp "$script_dir/organizations/demo.json" "$package_dir/runtime/organizations/"
printf '%s\n' '{"id":"fixture-scenario"}' >"$package_dir/runtime/scenarios/fixture-scenario.json"
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
cp "$package_dir/manifest.json" "$northstar_package_dir/manifest.json"
cp "$zpr_package_dir/manifest.json" "$northstar_zpr_package_dir/manifest.json"
cp -R "$package_dir/runtime/." "$northstar_package_runtime_dir/"
cp "$package_dir/runtime/image-reference" "$northstar_package_runtime_dir/image-reference"
cp "$zpr_package_dir/runtime/image-reference" "$northstar_zpr_package_dir/runtime/image-reference"
python3 "$script_dir/prepare_northstar_synthetic_runtime.py" \
    --lab-root "$northstar_lab_root" >/dev/null

snapshot_dir=$temp_dir/package-snapshots
mkdir "$snapshot_dir"
cp "$northstar_package_dir/manifest.json" "$snapshot_dir/simulator-manifest.json"
cp "$northstar_package_runtime_dir/image-reference" "$snapshot_dir/simulator-image-reference"
cp "$northstar_package_runtime_dir/config.toml" "$snapshot_dir/simulator-config.toml"
cp "$northstar_package_runtime_dir/simulation-environment.json" "$snapshot_dir/demo-manifest.json"
cp "$northstar_package_runtime_dir/organizations/demo.json" "$snapshot_dir/demo-organization.json"

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
            inspect) printf 'unix://%s\n' "$FAKE_DOCKER_SOCKET" ;;
            show) printf '%s\n' fake-local ;;
            *) exit 2 ;;
        esac
        ;;
    network)
        [ "$2" = inspect ] || exit 2
        case "$*" in
            *zpr-northstar-synthetic*Driver*)
                printf 'bridge|%s|local|172.30.0.1\n' "${FAKE_DOCKER_NORTHSTAR_NETWORK_INTERNAL:-true}"
                ;;
            *zpr-northstar-synthetic*) printf '%s\n' 172.30.0.1 ;;
            *Driver*) printf '%s\n' 'bridge|false|local|172.17.0.1' ;;
            *) printf '%s\n' 172.17.0.1 ;;
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
                    printf '%s\n' "$FAKE_DOCKER_CONTAINER_NAME"
                fi
                ;;
            inspect)
                if [ ! -f "$FAKE_DOCKER_CONTAINER_STATE" ]; then
                    exit 1
                fi
                IFS='|' read -r package release image published operator_digest control_digest runtime_digest provisioning_digest provisioning network state <"$FAKE_DOCKER_CONTAINER_STATE"
                printf '%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s\n' \
                    "$package" "$release" "$image" "$published" "$operator_digest" "$control_digest" "$runtime_digest" "$provisioning_digest" "$provisioning" "$network" "$state"
                ;;
            run)
                [ "$3" = -d ] || exit 2
                mounts=0
                published=
                operator_digest=none
                control_digest=none
                runtime_digest=none
                provisioning_digest=none
                provisioning=0
                network=bridge
                restart=
                container_port=${FAKE_DOCKER_CONTAINER_PORT:-8788}
                socket_mount=no
                runtime_mount=no
                state_mount=no
                operator_auth_mount=no
                control_tls_mount=no
                provisioning_source_mount=no
                northstar_machine_cert_mount=no
                northstar_workload_mount=no
                privileged=no
                shift 3
                while [ "$#" -gt 0 ]; do
                    case "$1" in
                        --label)
                            shift
                            case "$1" in
                                zpr.simulator.operator-config-sha256=*)
                                    operator_digest=${1#*=}
                                    ;;
                                zpr.simulator.machine-control-config-sha256=*)
                                    control_digest=${1#*=}
                                    ;;
                                zpr.simulator.runtime-config-sha256=*)
                                    runtime_digest=${1#*=}
                                    ;;
                                zpr.simulator.northstar-provisioning-config-sha256=*)
                                    provisioning_digest=${1#*=}
                                    ;;
                                zpr.simulator.packaged-provisioning-enabled=*)
                                    provisioning=${1#*=}
                                    ;;
                                zpr.simulator.docker-network=*)
                                    network=${1#*=}
                                    ;;
                            esac
                            ;;
                        --mount)
                            shift
                            case "$1" in
                                *target=/var/lib/zpr-simulator/operator-auth,readonly)
                                    operator_auth_mount=yes
                                    ;;
                                *target=/run/zpr-simulator/machine-control,readonly)
                                    control_tls_mount=yes
                                    ;;
                                "type=bind,source=${ZPR_SIMULATOR_PROVISIONING_SOURCE_DIR:-},target=/opt/zpr-simulator/northstar-provisioning-source,readonly")
                                    provisioning_source_mount=yes
                                    ;;
                                "type=bind,source=${SIMULATION_NORTHSTAR_MACHINE_CERT_DIR:-},target=${SIMULATION_NORTHSTAR_MACHINE_CERT_DIR:-},readonly")
                                    northstar_machine_cert_mount=yes
                                    ;;
                                "type=bind,source=${SIMULATION_NORTHSTAR_WORKLOAD_DIR:-},target=${SIMULATION_NORTHSTAR_WORKLOAD_DIR:-},readonly")
                                    northstar_workload_mount=yes
                                    ;;
                                *target=/var/lib/zpr-simulator/provisioning)
                                    ;;
                                *target=/var/lib/zpr-simulator/operator-auth,*)
                                    echo "operator auth mount is not read-only" >&2
                                    exit 1
                                    ;;
                            esac
                            ;;
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
                        --network)
                            shift
                            network=$1
                            ;;
                        --restart)
                            shift
                            restart=$1
                            ;;
                        --privileged) privileged=yes ;;
                    esac
                    shift
                done
                [ "$mounts" -eq 3 ] && [ "$socket_mount" = yes ] &&
                    [ "$runtime_mount" = yes ] && [ "$state_mount" = yes ] &&
                    [ "$published" = "127.0.0.1:$FAKE_DOCKER_PORT:$container_port" ] &&
                    [ "$privileged" = no ] || {
                        echo "Simulator container request violated the fake-Docker test contract: mounts=$mounts socket=$socket_mount runtime=$runtime_mount state=$state_mount published=$published privileged=$privileged" >&2
                        exit 1
                    }
                printf 'simulator|test-release|%s|127.0.0.1:%s->%s/tcp|%s|%s|%s|%s|%s|%s|running\n' \
                    "$FAKE_DOCKER_IMAGE_ID" "$FAKE_DOCKER_PORT" "$container_port" "$operator_digest" "$control_digest" "$runtime_digest" "$provisioning_digest" "$provisioning" "$network" \
                    >"$FAKE_DOCKER_CONTAINER_STATE"
                if [ "$operator_digest" != none ]; then
                    [ "$operator_auth_mount" = yes ] || {
                        echo "protected Simulator auth mount was not read-only" >&2
                        exit 1
                    }
                fi
                if [ "$control_digest" != none ]; then
                    [ "$control_tls_mount" = yes ] && [ "$restart" = no ] || {
                        echo "Northstar machine-control TLS mount or non-persistent restart policy missing" >&2
                        exit 1
                    }
                fi
                if [ "$control_digest" != none ] && [ "$provisioning" = 1 ]; then
                    [ "$provisioning_source_mount" = yes ] &&
                        [ "$northstar_machine_cert_mount" = yes ] &&
                        [ "$northstar_workload_mount" = yes ] &&
                        [ "$provisioning_digest" != none ] || {
                            echo "Northstar provisioning sources or protected input mounts are missing" >&2
                            exit 1
                        }
                fi
                printf '%s\n' fake-container-id
                ;;
            stop)
                [ -f "$FAKE_DOCKER_CONTAINER_STATE" ] || exit 1
                IFS='|' read -r package release image published operator_digest control_digest runtime_digest provisioning_digest provisioning network state <"$FAKE_DOCKER_CONTAINER_STATE"
                printf '%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|exited\n' \
                    "$package" "$release" "$image" "$published" "$operator_digest" "$control_digest" "$runtime_digest" "$provisioning_digest" "$provisioning" "$network" \
                    >"$FAKE_DOCKER_CONTAINER_STATE"
                printf '%s\n' "$4"
                ;;
            start)
                [ -f "$FAKE_DOCKER_CONTAINER_STATE" ] || exit 1
                IFS='|' read -r package release image published operator_digest control_digest runtime_digest provisioning_digest provisioning network state <"$FAKE_DOCKER_CONTAINER_STATE"
                printf '%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|running\n' \
                    "$package" "$release" "$image" "$published" "$operator_digest" "$control_digest" "$runtime_digest" "$provisioning_digest" "$provisioning" "$network" \
                    >"$FAKE_DOCKER_CONTAINER_STATE"
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
unset ZPR_SIMULATOR_PACKAGE_DIR ZPR_SIMULATOR_PROFILE ZPR_SIMULATOR_PORT \
    ZPR_SIMULATOR_RUNTIME_DIR DOCKER_CONTEXT DOCKER_HOST
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
export FAKE_DOCKER_CONTAINER_NAME=zpr-simulator-demo-test-release
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
grep -F -- "--network bridge" "$FAKE_DOCKER_LOG" >/dev/null
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
cmp "$package_dir/runtime/scenarios/fixture-scenario.json" "$state_dir/scenarios/fixture-scenario.json"
python3 - "$state_dir/scenarios/fixture-scenario.json" <<'PY'
import os
import stat
import sys

if stat.S_IMODE(os.stat(sys.argv[1]).st_mode) != 0o600:
    raise SystemExit("seeded Simulator scenario file is not private")
PY

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
printf '%s\n' '{"id":"fixture-scenario","operator-edit":true}' >"$state_dir/scenarios/fixture-scenario.json"
"$runtime_script" start >/dev/null 2>&1
[ "$(grep -c "container run" "$FAKE_DOCKER_LOG")" -eq "$run_count" ] || {
    echo "start recreated rather than restarted the stopped container" >&2
    exit 1
}
grep -F '"operator-edit":true' "$state_dir/scenarios/fixture-scenario.json" >/dev/null || {
    echo "start overwrote an existing Simulator scenario seed" >&2
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
stop_count=$(grep -c "container stop" "$FAKE_DOCKER_LOG" || true)
if "$runtime_script" stop >"$temp_dir/collision-stop.out" 2>&1; then
    echo "stop accepted an unowned container name" >&2
    exit 1
fi
[ "$(grep -c "container stop" "$FAKE_DOCKER_LOG" || true)" -eq "$stop_count" ] || {
    echo "stop mutated an unowned container" >&2
    exit 1
}

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
case "$(ZPR_SIMULATOR_PORT=8789 ZPR_SIMULATOR_ENABLE_PROVISIONING=1 "$runtime_script" status)" in
    "Simulator container running (UI endpoint http://127.0.0.1:8789; application readiness not checked)") ;;
    *) echo "custom Simulator host port was not verified: $output" >&2; exit 1 ;;
esac
if ZPR_SIMULATOR_PORT=8790 ZPR_SIMULATOR_ENABLE_PROVISIONING=1 "$runtime_script" status >"$temp_dir/port-mismatch.out" 2>&1; then
    echo "existing container with a different port binding unexpectedly accepted" >&2
    exit 1
fi
grep -F "occupied by a different or modified container" "$temp_dir/port-mismatch.out" >/dev/null
start_count=$(grep -c "container start" "$FAKE_DOCKER_LOG" || true)
if ZPR_SIMULATOR_PORT=8790 ZPR_SIMULATOR_ENABLE_PROVISIONING=1 \
    "$runtime_script" start >"$temp_dir/stale-start.out" 2>&1; then
    echo "start accepted an existing container with a mismatched port" >&2
    exit 1
fi
grep -F "occupied by a different or modified container" "$temp_dir/stale-start.out" >/dev/null
[ "$(grep -c "container start" "$FAKE_DOCKER_LOG" || true)" -eq "$start_count" ] || {
    echo "start restarted a container before validating its ownership" >&2
    exit 1
}

auth_dir=$temp_dir/operator-auth
export FAKE_DOCKER_PORT=8790
export FAKE_DOCKER_CONTAINER_PORT=8790
mkdir -m 700 "$auth_dir"
printf '%s\n' '{"version":1,"redirect_url":"https://simulator.example.test:8790/auth/operator/callback"}' >"$auth_dir/oidc.json"
for name in simulator.crt simulator.key client.secret oidc-ca.crt budget-client.crt budget-client.key budget-ca.crt; do
    printf '%s\n' test-material >"$auth_dir/$name"
done
chmod 600 "$auth_dir"/*
rm -f "$FAKE_DOCKER_CONTAINER_STATE"
output=$(
    ZPR_SIMULATOR_PORT=8790 \
    ZPR_SIMULATOR_DOCKER_NETWORK=zpr-demo-operator \
    ZPR_SIMULATOR_OPERATOR_AUTH_DIR="$auth_dir" \
    ZPR_SIMULATOR_OPERATOR_ORIGIN=https://simulator.example.test:8790 \
    ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP=172.17.0.1 \
    ZPR_BUDGET_AUTHORITY_URL=https://control-service.example.test:8790 \
    ZPR_BUDGET_AUTHORITY_TLS_SERVER_NAME=control-service.example.test \
    "$runtime_script" start
)
case "$output" in
    "Simulator container running (UI endpoint https://simulator.example.test:8790; application readiness not checked)") ;;
    *) echo "protected Simulator origin was not reported: $output" >&2; exit 1 ;;
esac
grep -F -- "--mount type=bind,source=$auth_dir,target=/var/lib/zpr-simulator/operator-auth,readonly" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "--network zpr-demo-operator" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-p 127.0.0.1:8790:8790" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "--entrypoint /usr/local/bin/zpr-web-dashboard" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-mode simulator" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-listen 0.0.0.0:8790" "$FAKE_DOCKER_LOG" >/dev/null
if grep -F -- "zpr-id.localhost:host-gateway" "$FAKE_DOCKER_LOG" >/dev/null; then
    echo "custom Simulator network unexpectedly bypassed Docker DNS aliases" >&2
    exit 1
fi
grep -F -- "-e ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP=172.17.0.1" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e ZPR_BUDGET_AUTHORITY_URL=https://control-service.example.test:8790" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e ZPR_BUDGET_AUTHORITY_TLS_SERVER_NAME=control-service.example.test" "$FAKE_DOCKER_LOG" >/dev/null
case "$(
    ZPR_SIMULATOR_PORT=8790 \
    ZPR_SIMULATOR_DOCKER_NETWORK=zpr-demo-operator \
    ZPR_SIMULATOR_OPERATOR_AUTH_DIR="$auth_dir" \
    ZPR_SIMULATOR_OPERATOR_ORIGIN=https://simulator.example.test:8790 \
    ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP=172.17.0.1 \
    ZPR_BUDGET_AUTHORITY_URL=https://control-service.example.test:8790 \
    ZPR_BUDGET_AUTHORITY_TLS_SERVER_NAME=control-service.example.test \
    "$runtime_script" status
)" in
    "Simulator container running (UI endpoint https://simulator.example.test:8790; application readiness not checked)") ;;
    *) echo "protected Simulator container config was not verified" >&2; exit 1 ;;
esac

run_count=$(grep -c "container run" "$FAKE_DOCKER_LOG")
if ZPR_SIMULATOR_OPERATOR_AUTH_DIR="$auth_dir" "$runtime_script" start >"$temp_dir/partial-auth.out" 2>&1; then
    echo "partial protected-access configuration unexpectedly accepted" >&2
    exit 1
fi
grep -F "requires OIDC auth files, an HTTPS origin, and an independent budget-authority URL" "$temp_dir/partial-auth.out" >/dev/null
[ "$(grep -c "container run" "$FAKE_DOCKER_LOG")" -eq "$run_count" ] || {
    echo "partial protected-access configuration invoked Docker" >&2
    exit 1
}
if ZPR_SIMULATOR_OPERATOR_AUTH_DIR="$auth_dir" \
    ZPR_SIMULATOR_OPERATOR_ORIGIN=https://simulator.example.test:8790 \
    ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP=not-an-ip \
    ZPR_BUDGET_AUTHORITY_URL=https://control-service.example.test:8790 \
    "$runtime_script" start >"$temp_dir/invalid-peer.out" 2>&1; then
    echo "invalid trusted proxy peer unexpectedly accepted" >&2
    exit 1
fi
grep -F "trusted proxy peer must be an IPv4 address" "$temp_dir/invalid-peer.out" >/dev/null

run_count=$(grep -c "container run" "$FAKE_DOCKER_LOG")
if ZPR_SIMULATOR_RUNTIME_DIR="$northstar_runtime_dir" \
    "$runtime_script" start >"$temp_dir/demo-runtime-override.out" 2>&1; then
    echo "Demo profile unexpectedly accepted a Northstar runtime override" >&2
    exit 1
fi
grep -F "ZPR_SIMULATOR_RUNTIME_DIR is supported only by the northstar-synthetic profile" \
    "$temp_dir/demo-runtime-override.out" >/dev/null
[ "$(grep -c "container run" "$FAKE_DOCKER_LOG")" -eq "$run_count" ] || {
    echo "Demo runtime override invoked Docker" >&2
    exit 1
}

ln -s "$northstar_runtime_dir" "$temp_dir/northstar-runtime-link"
if ZPR_SIMULATOR_PROFILE=northstar-synthetic \
    ZPR_SIMULATOR_RUNTIME_DIR="$temp_dir/northstar-runtime-link" \
    ZPR_SIMULATOR_PACKAGE_DIR="$northstar_package_dir" \
    "$runtime_script" start >"$temp_dir/northstar-symlink-runtime.out" 2>&1; then
    echo "Northstar runtime override unexpectedly accepted a symlink" >&2
    exit 1
fi
grep -F "must be an existing non-symlink directory" "$temp_dir/northstar-symlink-runtime.out" >/dev/null
[ "$(grep -c "container run" "$FAKE_DOCKER_LOG")" -eq "$run_count" ] || {
    echo "symlinked Northstar runtime override invoked Docker" >&2
    exit 1
}
export ZPR_SIMULATOR_RUNTIME_DIR="$northstar_runtime_dir"

run_count=$(grep -c "container run" "$FAKE_DOCKER_LOG")
if ZPR_SIMULATOR_PROFILE=northstar-synthetic \
    ZPR_SIMULATOR_RUNTIME_DIR="$northstar_runtime_dir/../runtime" \
    ZPR_SIMULATOR_PACKAGE_DIR="$northstar_package_dir" \
    "$runtime_script" start >"$temp_dir/northstar-noncanonical-runtime.out" 2>&1; then
    echo "Northstar runtime override unexpectedly accepted a non-canonical path" >&2
    exit 1
fi
grep -F "must be canonical and must not traverse symlinks" \
    "$temp_dir/northstar-noncanonical-runtime.out" >/dev/null
[ "$(grep -c "container run" "$FAKE_DOCKER_LOG")" -eq "$run_count" ] || {
    echo "non-canonical Northstar runtime override invoked Docker" >&2
    exit 1
}

ln -s simulation-environment.json "$northstar_runtime_dir/untrusted-input"
if ZPR_SIMULATOR_PROFILE=northstar-synthetic \
    ZPR_SIMULATOR_RUNTIME_DIR="$northstar_runtime_dir" \
    ZPR_SIMULATOR_PACKAGE_DIR="$northstar_package_dir" \
    "$runtime_script" start >"$temp_dir/northstar-runtime-tree-symlink.out" 2>&1; then
    echo "Northstar runtime tree unexpectedly accepted a symlinked input" >&2
    exit 1
fi
grep -F "Northstar runtime tree must not contain symbolic links" \
    "$temp_dir/northstar-runtime-tree-symlink.out" >/dev/null
[ "$(grep -c "container run" "$FAKE_DOCKER_LOG")" -eq "$run_count" ] || {
    echo "symlinked Northstar runtime input invoked Docker" >&2
    exit 1
}
rm "$northstar_runtime_dir/untrusted-input"

python3 - "$northstar_runtime_dir/scenarios/machine-policy-deny.json" <<'PY'
import json
from pathlib import Path
import sys

path = Path(sys.argv[1])
scenario = json.loads(path.read_text())
scenario["topology"]["components"][1]["address"] = "fd00:1:2::99"
path.write_text(json.dumps(scenario) + "\n")
PY
northstar_auth_dir=$temp_dir/northstar-auth
mkdir -m 700 "$northstar_auth_dir"
northstar_control_dir=$temp_dir/northstar-control
mkdir -m 700 "$northstar_control_dir"
for name in control-ca.crt simulator.crt simulator.key; do
    printf '%s\n' test-material >"$northstar_control_dir/$name"
done
chmod 600 "$northstar_control_dir"/*
source_overlay_dir=$temp_dir/northstar-source-overlay
northstar_machine_cert_dir=$temp_dir/northstar-machine-control
northstar_workload_dir=$temp_dir/northstar-workload
mkdir -m 700 "$source_overlay_dir" "$northstar_machine_cert_dir" "$northstar_workload_dir"
cp "$source_runtime_script" "$source_overlay_dir/simulator-demo-runtime.sh"
cp "$script_dir/packaged-machine-runtime.py" "$script_dir/verify-approved-identity-lab.py" \
    "$source_overlay_dir/"
chmod 600 "$source_overlay_dir"/*
for name in control-ca.crt client.crt client.key; do
    printf '%s\n' test-material >"$northstar_machine_cert_dir/$name"
done
for name in ca.crt client-finance-rsa.key machine-controller-machine-01.key; do
    printf '%s\n' test-material >"$northstar_workload_dir/$name"
done
chmod 600 "$northstar_machine_cert_dir"/* "$northstar_workload_dir"/*
printf '%s\n' '{"version":1,"redirect_url":"https://northstar.example.test:8789/auth/operator/callback"}' \
    >"$northstar_auth_dir/oidc.json"
for name in simulator.crt simulator.key client.secret oidc-ca.crt budget-client.crt budget-client.key budget-ca.crt; do
    printf '%s\n' test-material >"$northstar_auth_dir/$name"
done
chmod 600 "$northstar_auth_dir"/*
run_count=$(grep -c "container run" "$FAKE_DOCKER_LOG")
if ZPR_SIMULATOR_PROFILE=northstar-synthetic \
    ZPR_SIMULATOR_PACKAGE_DIR="$northstar_package_dir" \
    ZPR_SIMULATOR_RUNTIME_DIR="$northstar_runtime_dir" \
    ZPR_DEMO_PACKAGE_DIR="$northstar_zpr_package_dir" \
    ZPR_SIMULATOR_DOCKER_NETWORK=zpr-northstar-synthetic \
    ZPR_SIMULATOR_OPERATOR_AUTH_DIR="$northstar_auth_dir" \
    ZPR_SIMULATOR_OPERATOR_ORIGIN=https://northstar.example.test:8789 \
    ZPR_BUDGET_AUTHORITY_URL=https://budget.example.test:8790 \
    ZPR_SIMULATOR_CONTROL_TLS_DIR="$northstar_control_dir" \
    "$runtime_script" start >"$temp_dir/northstar-invalid.out" 2>&1; then
    echo "Northstar synthetic package with a mismatched destination unexpectedly accepted" >&2
    exit 1
fi
grep -F "invalid Northstar synthetic Simulator configuration" "$temp_dir/northstar-invalid.out" >/dev/null || {
    cat "$temp_dir/northstar-invalid.out" >&2
    exit 1
}
[ "$(grep -c "container run" "$FAKE_DOCKER_LOG")" -eq "$run_count" ] || {
    echo "invalid Northstar synthetic package invoked Docker" >&2
    exit 1
}
python3 - "$northstar_runtime_dir/scenarios/machine-policy-deny.json" <<'PY'
import json
from pathlib import Path
import sys

path = Path(sys.argv[1])
scenario = json.loads(path.read_text())
scenario["topology"]["components"][1]["address"] = "fd00:1:2::1"
path.write_text(json.dumps(scenario) + "\n")
PY
run_count=$(grep -c "container run" "$FAKE_DOCKER_LOG")
if ZPR_SIMULATOR_PROFILE=northstar-synthetic \
    ZPR_SIMULATOR_PACKAGE_DIR="$northstar_package_dir" \
    ZPR_SIMULATOR_RUNTIME_DIR="$northstar_runtime_dir" \
    ZPR_DEMO_PACKAGE_DIR="$northstar_zpr_package_dir" \
    ZPR_SIMULATOR_DOCKER_NETWORK=zpr-northstar-synthetic \
    "$runtime_script" start >"$temp_dir/northstar-unprotected.out" 2>&1; then
    echo "Northstar synthetic profile unexpectedly allowed an auth-disabled launch" >&2
    exit 1
fi
grep -F "Northstar synthetic requires protected operator login, an independent budget-authority URL, and machine-control TLS assets" \
    "$temp_dir/northstar-unprotected.out" >/dev/null || {
    cat "$temp_dir/northstar-unprotected.out" >&2
    exit 1
}
[ "$(grep -c "container run" "$FAKE_DOCKER_LOG")" -eq "$run_count" ] || {
    echo "unprotected Northstar synthetic profile invoked Docker" >&2
    exit 1
}
run_count=$(grep -c "container run" "$FAKE_DOCKER_LOG")
if FAKE_DOCKER_NORTHSTAR_NETWORK_INTERNAL=false \
    ZPR_SIMULATOR_PROFILE=northstar-synthetic \
    ZPR_SIMULATOR_PACKAGE_DIR="$northstar_package_dir" \
    ZPR_SIMULATOR_RUNTIME_DIR="$northstar_runtime_dir" \
    ZPR_DEMO_PACKAGE_DIR="$northstar_zpr_package_dir" \
    ZPR_SIMULATOR_DOCKER_NETWORK=zpr-northstar-synthetic \
    ZPR_SIMULATOR_OPERATOR_AUTH_DIR="$northstar_auth_dir" \
    ZPR_SIMULATOR_OPERATOR_ORIGIN=https://northstar.example.test:8789 \
    ZPR_BUDGET_AUTHORITY_URL=https://budget.example.test:8790 \
    ZPR_SIMULATOR_CONTROL_TLS_DIR="$northstar_control_dir" \
    "$runtime_script" start >"$temp_dir/northstar-external-network.out" 2>&1; then
    echo "Northstar synthetic accepted a non-internal network" >&2
    exit 1
fi
grep -F "Northstar synthetic requires an internal local bridge network" \
    "$temp_dir/northstar-external-network.out" >/dev/null || {
    cat "$temp_dir/northstar-external-network.out" >&2
    exit 1
}
[ "$(grep -c "container run" "$FAKE_DOCKER_LOG")" -eq "$run_count" ] || {
    echo "Northstar synthetic with a non-internal network invoked Docker" >&2
    exit 1
}
chmod 644 "$northstar_control_dir/simulator.key"
if ZPR_SIMULATOR_PROFILE=northstar-synthetic \
    ZPR_SIMULATOR_PACKAGE_DIR="$northstar_package_dir" \
    ZPR_SIMULATOR_RUNTIME_DIR="$northstar_runtime_dir" \
    ZPR_DEMO_PACKAGE_DIR="$northstar_zpr_package_dir" \
    ZPR_SIMULATOR_DOCKER_NETWORK=zpr-northstar-synthetic \
    ZPR_SIMULATOR_OPERATOR_AUTH_DIR="$northstar_auth_dir" \
    ZPR_SIMULATOR_OPERATOR_ORIGIN=https://northstar.example.test:8789 \
    ZPR_BUDGET_AUTHORITY_URL=https://budget.example.test:8790 \
    ZPR_SIMULATOR_CONTROL_TLS_DIR="$northstar_control_dir" \
    "$runtime_script" start >"$temp_dir/northstar-invalid-control-mode.out" 2>&1; then
    echo "Northstar synthetic accepted a machine-control key with broad permissions" >&2
    exit 1
fi
grep -F "invalid Simulator machine-control TLS configuration" \
    "$temp_dir/northstar-invalid-control-mode.out" >/dev/null || {
    cat "$temp_dir/northstar-invalid-control-mode.out" >&2
    exit 1
}
[ "$(grep -c "container run" "$FAKE_DOCKER_LOG")" -eq "$run_count" ] || {
    echo "invalid machine-control TLS permissions invoked Docker" >&2
    exit 1
}
chmod 600 "$northstar_control_dir/simulator.key"
run_count=$(grep -c "container run" "$FAKE_DOCKER_LOG")
if ZPR_SIMULATOR_PROFILE=northstar-synthetic \
    ZPR_SIMULATOR_ENABLE_PROVISIONING=1 \
    ZPR_SIMULATOR_PACKAGE_DIR="$northstar_package_dir" \
    ZPR_SIMULATOR_RUNTIME_DIR="$northstar_runtime_dir" \
    ZPR_DEMO_PACKAGE_DIR="$northstar_zpr_package_dir" \
    ZPR_SIMULATOR_DOCKER_NETWORK=zpr-northstar-synthetic \
    ZPR_SIMULATOR_OPERATOR_AUTH_DIR="$northstar_auth_dir" \
    ZPR_SIMULATOR_OPERATOR_ORIGIN=https://northstar.example.test:8789 \
    ZPR_BUDGET_AUTHORITY_URL=https://budget.example.test:8790 \
    ZPR_SIMULATOR_CONTROL_TLS_DIR="$northstar_control_dir" \
    SIMULATION_NORTHSTAR_MACHINE_CERT_DIR="$northstar_machine_cert_dir" \
    SIMULATION_NORTHSTAR_WORKLOAD_DIR="$northstar_workload_dir" \
    SIMULATION_NORTHSTAR_MACHINE_IMAGE=sha256:2222222222222222222222222222222222222222222222222222222222222222 \
    SIMULATION_NORTHSTAR_NODE_ADDRESS=172.30.0.10:5000 \
    SIMULATION_NORTHSTAR_CONTROL_URL=https://host.docker.internal:8792 \
    SIMULATION_RUNTIME_GENERATION=scopefix2-test \
    "$runtime_script" start >"$temp_dir/northstar-missing-source.out" 2>&1; then
    echo "Northstar provisioning unexpectedly accepted a missing source overlay" >&2
    exit 1
fi
grep -F "Northstar packaged provisioning requires ZPR_SIMULATOR_PROVISIONING_SOURCE_DIR" \
    "$temp_dir/northstar-missing-source.out" >/dev/null || {
    cat "$temp_dir/northstar-missing-source.out" >&2
    exit 1
}
[ "$(grep -c "container run" "$FAKE_DOCKER_LOG")" -eq "$run_count" ] || {
    echo "missing Northstar source overlay invoked Docker" >&2
    exit 1
}
rm -f "$FAKE_DOCKER_CONTAINER_STATE"
export FAKE_DOCKER_PORT=8789
export FAKE_DOCKER_CONTAINER_PORT=8789
export FAKE_DOCKER_CONTAINER_NAME=zpr-simulator-northstar-synthetic-test-release
output=$(
    ZPR_SIMULATOR_PROFILE=northstar-synthetic \
    ZPR_SIMULATOR_ENABLE_PROVISIONING=1 \
    ZPR_SIMULATOR_PACKAGE_DIR="$northstar_package_dir" \
    ZPR_SIMULATOR_RUNTIME_DIR="$northstar_runtime_dir" \
    ZPR_SIMULATOR_PROVISIONING_SOURCE_DIR="$source_overlay_dir" \
    ZPR_DEMO_PACKAGE_DIR="$northstar_zpr_package_dir" \
    ZPR_SIMULATOR_DOCKER_NETWORK=zpr-northstar-synthetic \
    ZPR_SIMULATOR_OPERATOR_AUTH_DIR="$northstar_auth_dir" \
    ZPR_SIMULATOR_OPERATOR_ORIGIN=https://northstar.example.test:8789 \
    ZPR_BUDGET_AUTHORITY_URL=https://budget.example.test:8790 \
    ZPR_SIMULATOR_CONTROL_TLS_DIR="$northstar_control_dir" \
    SIMULATION_NORTHSTAR_MACHINE_CERT_DIR="$northstar_machine_cert_dir" \
    SIMULATION_NORTHSTAR_WORKLOAD_DIR="$northstar_workload_dir" \
    SIMULATION_NORTHSTAR_MACHINE_IMAGE=sha256:2222222222222222222222222222222222222222222222222222222222222222 \
    SIMULATION_NORTHSTAR_NODE_ADDRESS=172.30.0.10:5000 \
    SIMULATION_NORTHSTAR_CONTROL_URL=https://host.docker.internal:8792 \
    SIMULATION_RUNTIME_GENERATION=scopefix2-test \
    sh "$source_overlay_dir/simulator-demo-runtime.sh" start
)
case "$output" in
    "Simulator container running (UI endpoint https://northstar.example.test:8789; application readiness not checked)") ;;
    *) echo "Northstar synthetic profile did not use its isolated default port: $output" >&2; exit 1 ;;
esac
grep -F -- "--name zpr-simulator-northstar-synthetic-test-release" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATION_MANIFEST=/opt/zpr-simulator/runtime/simulation-environment.json" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-v $northstar_runtime_dir:/opt/zpr-simulator/runtime:ro" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "zpr-simulator:test-release" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATION_ORGANIZATION_ID=northstar" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATION_ORGANIZATIONS_DIR=/opt/zpr-simulator/runtime/organizations" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATION_SCENARIOS_DIR=/var/lib/zpr-simulator/scenarios" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-p 127.0.0.1:8789:8789" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "--network zpr-northstar-synthetic" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATOR_CONTROL_TLS_CERT=/run/zpr-simulator/machine-control/simulator.crt" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATOR_CONTROL_TLS_KEY=/run/zpr-simulator/machine-control/simulator.key" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATOR_CONTROL_CLIENT_CA=/run/zpr-simulator/machine-control/control-ca.crt" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATOR_CONTROL_LISTEN=0.0.0.0:8791" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "--mount type=bind,source=$source_overlay_dir,target=/opt/zpr-simulator/northstar-provisioning-source,readonly" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "--mount type=bind,source=$northstar_machine_cert_dir,target=$northstar_machine_cert_dir,readonly" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "--mount type=bind,source=$northstar_workload_dir,target=$northstar_workload_dir,readonly" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATION_PROVISION_SCRIPT=/opt/zpr-simulator/northstar-provisioning-source/packaged-machine-runtime.py" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATION_NORTHSTAR_MACHINE_IMAGE=sha256:2222222222222222222222222222222222222222222222222222222222222222" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATION_NORTHSTAR_DOCKER_NETWORK=zpr-northstar-synthetic" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATION_NORTHSTAR_NODE_ADDRESS=172.30.0.10:5000" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATION_NORTHSTAR_CONTROL_URL=https://host.docker.internal:8792" "$FAKE_DOCKER_LOG" >/dev/null
grep -F -- "-e SIMULATION_RUNTIME_GENERATION=scopefix2-test" "$FAKE_DOCKER_LOG" >/dev/null
[ ! -e "$northstar_runtime_dir/organizations/demo.json" ] || {
    echo "Northstar runtime root was merged with the Demo profile" >&2
    exit 1
}
[ -f "$state_dir/northstar-synthetic/scenarios/machine-policy-deny.json" ] || {
    echo "Northstar synthetic scenario was not seeded into isolated state" >&2
    exit 1
}
[ ! -e "$state_dir/scenarios/machine-policy-deny.json" ] || {
    echo "Northstar synthetic scenario leaked into Demo state" >&2
    exit 1
}
cmp "$snapshot_dir/simulator-manifest.json" "$northstar_package_dir/manifest.json"
cmp "$snapshot_dir/simulator-image-reference" "$northstar_package_runtime_dir/image-reference"
cmp "$snapshot_dir/simulator-config.toml" "$northstar_package_runtime_dir/config.toml"
cmp "$snapshot_dir/demo-manifest.json" "$northstar_package_runtime_dir/simulation-environment.json"
cmp "$snapshot_dir/demo-organization.json" "$northstar_package_runtime_dir/organizations/demo.json"
grep -Fx "owned by W2" "$northstar_route_output/route-output.txt" >/dev/null

echo "Simulator runtime shell tests passed (Docker fully stubbed; no services started)."
