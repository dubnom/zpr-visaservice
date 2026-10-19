#!/bin/sh
set -eu
umask 077

usage() {
    echo "usage: simulator-demo-runtime.sh start|stop|status" >&2
}

fail() {
    echo "simulator-demo-runtime: $*" >&2
    exit 1
}

command_name=${1:-}
case "$command_name" in
    start|stop|status) ;;
    *) usage; exit 2 ;;
esac
[ "$#" -eq 1 ] || { usage; exit 2; }

if [ -n "${ZPR_SIMULATOR_PACKAGE_DIR:-}" ]; then
    package_dir=$ZPR_SIMULATOR_PACKAGE_DIR
else
    script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P) ||
        fail "cannot resolve the runtime script directory"
    package_dir=$(CDPATH='' cd -- "$script_dir/.." && pwd -P) ||
        fail "cannot resolve the Simulator package directory"
fi
[ -d "$package_dir" ] && [ ! -L "$package_dir" ] ||
    fail "package directory must be an existing non-symlink directory"
package_dir=$(CDPATH='' cd -- "$package_dir" && pwd -P) ||
    fail "cannot resolve package directory"
runtime_dir=$package_dir/runtime
[ -d "$runtime_dir" ] && [ ! -L "$runtime_dir" ] ||
    fail "package runtime directory is unavailable"

: "${ZPR_SIMULATOR_STATE_DIR:?set ZPR_SIMULATOR_STATE_DIR to a persistent private directory}"
case "$ZPR_SIMULATOR_STATE_DIR" in
    /*) ;;
    *) fail "ZPR_SIMULATOR_STATE_DIR must be an absolute path" ;;
esac
case "$ZPR_SIMULATOR_STATE_DIR" in
    *:*) fail "ZPR_SIMULATOR_STATE_DIR must not contain a colon" ;;
esac

command -v python3 >/dev/null 2>&1 || fail "python3 is required to validate package metadata"
command -v docker >/dev/null 2>&1 || fail "Docker CLI is required"

for package_file in \
    "$package_dir/manifest.json" \
    "$runtime_dir/image-reference" \
    "$runtime_dir/config.toml" \
    "$runtime_dir/simulation-environment.json" \
    "$runtime_dir/organizations/demo.json"; do
    [ -f "$package_file" ] && [ ! -L "$package_file" ] ||
        fail "required package asset is missing or not a regular file: $package_file"
done
[ -d "$runtime_dir/organizations" ] && [ ! -L "$runtime_dir/organizations" ] ||
    fail "package organization directory is unavailable"

package_metadata=$(
    python3 - "$package_dir/manifest.json" "$runtime_dir/image-reference" \
        "$(basename "$(dirname "$package_dir")")" "$(basename "$package_dir")" <<'PY'
import json
import re
import sys

manifest_path, reference_path, directory_release, package_name = sys.argv[1:]
try:
    with open(manifest_path, "rb") as source:
        manifest = json.load(source)
    with open(reference_path, "rb") as source:
        reference_bytes = source.read()
except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
    raise SystemExit(f"cannot read package identity metadata: {exc}")

if not isinstance(manifest, dict):
    raise SystemExit("package manifest must be a JSON object")
component = manifest.get("component")
release = manifest.get("release_id")
reference = component.get("image_reference") if isinstance(component, dict) else None
docker_image_id = component.get("docker_image_id") if isinstance(component, dict) else None
image_id = component.get("image_id") if isinstance(component, dict) else None
if (
    manifest.get("schema") != "zpr.demo.runtime-package.v2"
    or manifest.get("status") != "demo-unqualified-runtime-package"
    or manifest.get("platform") != "linux/amd64"
    or manifest.get("package_id") != "simulator-image"
    or manifest.get("required_peer_package_id") != "zpr-demo-image"
    or package_name != "simulator"
    or not isinstance(release, str)
    or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,79}", release)
    or release != directory_release
    or not isinstance(component, dict)
    or component.get("id") != "simulator-image"
    or not isinstance(reference, str)
    or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._/:@+-]{0,254}", reference)
    or not isinstance(docker_image_id, str)
    or not re.fullmatch(r"sha256:[0-9a-f]{64}", docker_image_id)
    or not isinstance(image_id, str)
    or not re.fullmatch(r"sha256:[0-9a-f]{64}", image_id)
):
    raise SystemExit("manifest, release directory, or image-reference does not identify this Simulator package")
try:
    expected_reference = (reference + "\n").encode("ascii")
except UnicodeEncodeError:
    raise SystemExit("package image reference must be ASCII")
if reference_bytes != expected_reference:
    raise SystemExit("image-reference does not exactly match the package manifest")
print(release)
print(reference)
print(docker_image_id)
PY
) || fail "invalid Simulator package identity"

python3 - "$runtime_dir/simulation-environment.json" "$runtime_dir/organizations/demo.json" <<'PY' ||
import json
import sys

try:
    with open(sys.argv[1], "rb") as source:
        manifest = json.load(source)
    with open(sys.argv[2], "rb") as source:
        organization = json.load(source)
except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
    raise SystemExit(f"cannot read Simulator configuration: {exc}")

if (
    not isinstance(manifest, dict)
    or manifest.get("organization_id") != "demo"
    or not isinstance(manifest.get("machines"), list)
    or len(manifest["machines"]) != 20
    or not isinstance(organization, dict)
    or organization.get("id") != "demo"
):
    raise SystemExit("Simulator package assets must define the demo organization and exactly 20 machines")
PY
    fail "invalid Simulator configuration"

release_id=$(printf '%s\n' "$package_metadata" | sed -n '1p')
image_reference=$(printf '%s\n' "$package_metadata" | sed -n '2p')
expected_image_id=$(printf '%s\n' "$package_metadata" | sed -n '3p')
container_name="zpr-simulator-demo-$release_id"
state_dir=$ZPR_SIMULATOR_STATE_DIR

validate_state_path() {
    state_dir=$(python3 - "$state_dir" "$package_dir" <<'PY'
import os
import sys

state, package = sys.argv[1:]
if state == "/" or not os.path.isabs(state) or os.path.normpath(state) != state:
    raise SystemExit("ZPR_SIMULATOR_STATE_DIR must be a normalized absolute path")
state_real = os.path.realpath(state)
package_real = os.path.realpath(package)
if (
    state_real == package_real
    or state_real.startswith(package_real + os.sep)
    or package_real.startswith(state_real + os.sep)
):
    raise SystemExit("Simulator state and installed package directories must be separate")
if os.path.exists(state):
    info = os.lstat(state)
    if not os.path.isdir(state) or os.path.islink(state):
        raise SystemExit("Simulator state path must be a non-symlink directory")
    if info.st_uid != os.geteuid():
        raise SystemExit("Simulator state directory must be owned by the invoking user")
    if info.st_mode & 0o077:
        raise SystemExit("Simulator state directory must not grant group or other access")
print(state_real)
PY
    ) || return 1
}

inspect_container() {
    docker container inspect --format \
        '{{index .Config.Labels "zpr.demo.package"}}|{{index .Config.Labels "zpr.demo.release"}}|{{.Image}}|{{.State.Status}}' \
        "$container_name"
}

lookup_container() {
    existing_container=
    container_names=$(docker container ls --all \
        --filter "name=^/$container_name$" --format '{{.Names}}') ||
        fail "Docker could not list Simulator containers"
    case "$container_names" in
        "") return ;;
        "$container_name") ;;
        *) fail "Docker returned an unexpected container name for $container_name" ;;
    esac
    existing_container=$(inspect_container) ||
        fail "Docker listed $container_name but could not inspect it"
    verify_container "$existing_container"
}

resolve_docker_socket() {
    if [ -n "${DOCKER_CONTEXT:-}" ]; then
        docker_endpoint=$(docker context inspect --format '{{.Endpoints.docker.Host}}' "$DOCKER_CONTEXT") ||
            fail "cannot inspect Docker context"
    elif [ -n "${DOCKER_HOST:-}" ]; then
        docker_endpoint=$DOCKER_HOST
    else
        docker_context=$(docker context show) ||
            fail "cannot determine active Docker context"
        docker_endpoint=$(docker context inspect --format '{{.Endpoints.docker.Host}}' "$docker_context") ||
            fail "cannot inspect active Docker context"
    fi
    case "$docker_endpoint" in
        unix:///*) docker_socket=${docker_endpoint#unix://} ;;
        *) fail "Simulator requires a local Unix-socket Docker endpoint; remote Docker contexts are not supported" ;;
    esac
    case "$docker_socket" in
        *:*) fail "Docker socket path must not contain a colon" ;;
    esac
}

verify_container() {
    metadata=$1
    expected_prefix="simulator|$release_id|$expected_image_id|"
    case "$metadata" in
        "$expected_prefix"*) ;;
        *) fail "container name is occupied by a different or modified container: $container_name" ;;
    esac
}

print_status() {
    metadata=$1
    verify_container "$metadata"
    container_state=${metadata##*|}
    case "$container_state" in
        running) echo "Simulator container running (UI endpoint http://127.0.0.1:8788; application readiness not checked)" ;;
        *) echo "Simulator stopped (container $container_name; state $container_state)" ;;
    esac
}

case "$command_name" in
    status)
        validate_state_path || fail "invalid Simulator state directory"
        resolve_docker_socket
        lookup_container
        if [ -n "$existing_container" ]; then
            print_status "$existing_container"
        else
            echo "Simulator stopped (no container for release $release_id)"
        fi
        ;;
    stop)
        validate_state_path || fail "invalid Simulator state directory"
        resolve_docker_socket
        lookup_container
        if [ -n "$existing_container" ]; then
            container_state=${existing_container##*|}
            case "$container_state" in
                running|paused|restarting)
                docker container stop --time 10 "$container_name" >/dev/null ||
                    fail "Docker could not stop $container_name"
                    ;;
                created|exited|dead) ;;
                *) fail "Docker reported unsupported container state: $container_state" ;;
            esac
            echo "Simulator stopped (container $container_name; persistent state retained at $state_dir)"
        else
            echo "Simulator stopped (no container for release $release_id)"
        fi
        ;;
    start)
        echo "WARNING: Simulator receives root-equivalent host access through the Docker socket; use only on an isolated disposable demo host." >&2
        validate_state_path || fail "invalid Simulator state directory"
        if [ ! -e "$state_dir" ]; then
            mkdir -p "$state_dir" ||
                fail "cannot create Simulator state directory: $state_dir"
        fi
        validate_state_path || fail "invalid Simulator state directory"
        for subdirectory in workspace-db published-directories scenarios; do
            if ! python3 - "$state_dir/$subdirectory" <<'PY'
import os
import sys

path = sys.argv[1]
try:
    os.mkdir(path, 0o700)
except FileExistsError:
    pass
info = os.lstat(path)
if not os.path.isdir(path) or os.path.islink(path):
    raise SystemExit("Simulator state subdirectory must not be a symlink")
if info.st_uid != os.geteuid() or info.st_mode & 0o077:
    raise SystemExit("Simulator state subdirectory must be private and owned by the invoking user")
PY
            then
                fail "cannot create or validate Simulator state subdirectory: $subdirectory"
            fi
        done

        resolve_docker_socket
        [ -S "$docker_socket" ] ||
            fail "Docker socket is unavailable or is not a Unix socket: $docker_socket"
        [ ! -L "$docker_socket" ] ||
            fail "Docker socket path must not be a symbolic link"
        socket_gid=$(stat -c '%g' "$docker_socket") ||
            fail "cannot determine Docker socket group"
        user_id=$(id -u) || fail "cannot determine invoking user ID"
        group_id=$(id -g) || fail "cannot determine invoking group ID"

        image_metadata=$(docker image inspect --format '{{.Id}}|{{.Os}}|{{.Architecture}}' "$image_reference") ||
            fail "the package Simulator image is not available in local Docker"
        [ "$image_metadata" = "$expected_image_id|linux|amd64" ] ||
            fail "loaded Simulator image does not match the package's pinned Linux/amd64 image identity"

        lookup_container
        if [ -n "$existing_container" ]; then
            container_state=${existing_container##*|}
            if [ "$container_state" = running ]; then
                print_status "$existing_container"
                exit 0
            fi
            docker container start "$container_name" >/dev/null ||
                fail "Docker could not start the existing Simulator container"
        else
            set -- container run -d \
                --name "$container_name" \
                --label zpr.demo.package=simulator \
                --label "zpr.demo.release=$release_id" \
                --restart unless-stopped \
                --user "$user_id:$group_id"
            if [ "$socket_gid" != "$group_id" ]; then
                set -- "$@" --group-add "$socket_gid"
            fi
            set -- "$@" \
                --read-only \
                --security-opt no-new-privileges:true \
                --tmpfs /tmp:rw,noexec,nosuid,size=16777216 \
                -p 127.0.0.1:8788:8788 \
                -v "$docker_socket:/var/run/docker.sock" \
                -v "$runtime_dir:/opt/zpr-simulator/runtime:ro" \
                -v "$state_dir:/var/lib/zpr-simulator" \
                --workdir /opt/zpr-simulator/runtime \
                -e SIMULATION_MANIFEST=/opt/zpr-simulator/runtime/simulation-environment.json \
                -e SIMULATION_ORGANIZATION_ID=demo \
                -e SIMULATION_ORGANIZATIONS_DIR=/opt/zpr-simulator/runtime/organizations \
                -e SIMULATION_SCENARIOS_DIR=/var/lib/zpr-simulator/scenarios \
                -e SIMULATION_ACTIVE_ORGANIZATION_FILE=/var/lib/zpr-simulator/active-organization.txt \
                -e SIMULATION_WORKSPACE_DB_DIR=/var/lib/zpr-simulator/workspace-db \
                -e SIMULATION_PUBLISHED_DIRECTORY_DIR=/var/lib/zpr-simulator/published-directories \
                -e ZPR_WEB_DASHBOARD_BIN=/usr/local/bin/zpr-web-dashboard \
                "$image_reference"
            docker "$@" >/dev/null ||
                fail "Docker could not create the Simulator container"
        fi
        metadata=$(inspect_container) ||
            fail "Docker did not retain the Simulator container after startup"
        verify_container "$metadata"
        [ "${metadata##*|}" = running ] ||
            fail "Simulator container exited during startup; inspect Docker logs for $container_name"
        print_status "$metadata"
        ;;
esac
