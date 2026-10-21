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
host_port=${ZPR_SIMULATOR_PORT:-8788}
case "$host_port" in
    ""|*[!0-9]*|??????*) fail "ZPR_SIMULATOR_PORT must be a decimal port from 1024 through 65535" ;;
esac
[ "$host_port" -ge 1024 ] && [ "$host_port" -le 65535 ] ||
    fail "ZPR_SIMULATOR_PORT must be a decimal port from 1024 through 65535"
docker_network=${ZPR_SIMULATOR_DOCKER_NETWORK:-bridge}
case "$docker_network" in
    ""|*[!a-zA-Z0-9_.-]*|[!a-zA-Z0-9]*) fail "ZPR_SIMULATOR_DOCKER_NETWORK must be a Docker network name" ;;
esac
state_dir=$ZPR_SIMULATOR_STATE_DIR
enable_provisioning=${ZPR_SIMULATOR_ENABLE_PROVISIONING:-0}
case "$enable_provisioning" in
    0|1) ;;
    *) fail "ZPR_SIMULATOR_ENABLE_PROVISIONING must be 0 or 1" ;;
esac
operator_auth_dir=${ZPR_SIMULATOR_OPERATOR_AUTH_DIR:-}
operator_origin=${ZPR_SIMULATOR_OPERATOR_ORIGIN:-}
budget_authority_url=${ZPR_BUDGET_AUTHORITY_URL:-}
budget_authority_tls_server_name=${ZPR_BUDGET_AUTHORITY_TLS_SERVER_NAME:-}
simulator_operator_trusted_peer=${ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP:-}
container_port=8788
if [ -n "$operator_origin" ]; then
    container_port=$host_port
fi
operator_config_digest=none
validate_ipv4_address() {
    python3 - "$1" <<'PY'
import ipaddress
import sys

try:
    address = ipaddress.ip_address(sys.argv[1])
except ValueError as error:
    raise SystemExit(f"invalid IPv4 address: {error}")
if address.version != 4:
    raise SystemExit("address must be IPv4")
PY
}
if [ -n "$simulator_operator_trusted_peer" ]; then
    validate_ipv4_address "$simulator_operator_trusted_peer" ||
        fail "Simulator trusted proxy peer must be an IPv4 address"
fi
if [ -n "$operator_auth_dir$operator_origin$budget_authority_url" ]; then
    [ -n "$operator_auth_dir" ] && [ -n "$operator_origin" ] && [ -n "$budget_authority_url" ] ||
        fail "protected Simulator access requires OIDC auth files, an HTTPS origin, and an independent budget-authority URL"
    operator_config_digest=$(
        python3 - "$operator_auth_dir" "$operator_origin" "$budget_authority_url" \
            "$budget_authority_tls_server_name" "$simulator_operator_trusted_peer" <<'PY'
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys
from urllib.parse import urlsplit

directory, origin, authority_url, server_name, trusted_peer = sys.argv[1:]
safe_path = re.compile(r"^[A-Za-z0-9_./-]+$")

def https_origin(value):
    try:
        parsed = urlsplit(value)
        return (
            parsed.scheme == "https"
            and bool(parsed.hostname)
            and parsed.port is not None
            and parsed.path in ("", "/")
            and not parsed.username
            and not parsed.password
            and not parsed.query
            and not parsed.fragment
        )
    except ValueError:
        return False

if not https_origin(origin) or origin.endswith("/"):
    raise SystemExit("Simulator operator origin must be an HTTPS origin without a trailing slash")
if not https_origin(authority_url) or authority_url.endswith("/"):
    raise SystemExit("budget authority URL must be an HTTPS origin without a trailing slash")
path = Path(directory)
if (
    not path.is_absolute()
    or os.path.normpath(path) != str(path)
    or not safe_path.fullmatch(str(path))
    or os.path.realpath(path) != str(path)
):
    raise SystemExit("Simulator operator auth directory must be a canonical absolute path")
try:
    directory_info = os.lstat(path)
except OSError as error:
    raise SystemExit(f"cannot inspect Simulator operator auth directory: {error}")
if (
    not stat.S_ISDIR(directory_info.st_mode)
    or directory_info.st_uid != os.geteuid()
    or stat.S_IMODE(directory_info.st_mode) != 0o700
):
    raise SystemExit("Simulator operator auth directory must be owned by the invoking user with mode 0700")

required_files = {
    "simulator.crt",
    "simulator.key",
    "oidc.json",
    "client.secret",
    "oidc-ca.crt",
    "budget-client.crt",
    "budget-client.key",
    "budget-ca.crt",
}
try:
    entries = set(os.listdir(path))
except OSError as error:
    raise SystemExit(f"cannot list Simulator operator auth directory: {error}")
if entries != required_files:
    raise SystemExit("Simulator operator auth directory must contain exactly the documented OIDC and authority files")

digest = hashlib.sha256()
for name in sorted(required_files):
    file_path = path / name
    try:
        info = os.lstat(file_path)
    except OSError as error:
        raise SystemExit(f"cannot inspect Simulator operator auth file {name}: {error}")
    if (
        not stat.S_ISREG(info.st_mode)
        or stat.S_ISLNK(info.st_mode)
        or info.st_uid != os.geteuid()
        or stat.S_IMODE(info.st_mode) != 0o600
        or not safe_path.fullmatch(str(file_path))
    ):
        raise SystemExit(f"Simulator operator auth file {name} must be a regular invoking-user-owned mode-0600 file")
    data = file_path.read_bytes()
    if name == "oidc.json":
        try:
            config = json.loads(data)
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise SystemExit(f"Simulator OIDC config is invalid JSON: {error}")
        if (
            not isinstance(config, dict)
            or config.get("version") != 1
            or config.get("redirect_url") != origin + "/auth/operator/callback"
        ):
            raise SystemExit("Simulator OIDC config callback must match the exact HTTPS origin")
    digest.update(name.encode())
    digest.update(b"\0")
    digest.update(data)
    digest.update(b"\0")
digest.update(origin.encode())
digest.update(b"\0")
digest.update(authority_url.encode())
digest.update(b"\0")
digest.update(server_name.encode())
digest.update(b"\0")
digest.update(trusted_peer.encode())
print(digest.hexdigest())
PY
    ) || fail "invalid protected Simulator operator or budget-authority configuration"
fi
demo_project=
demo_image=
demo_release=
if [ -n "${ZPR_DEMO_PACKAGE_DIR:-}" ]; then
    demo_metadata=$(
        python3 - "$package_dir" "$ZPR_DEMO_PACKAGE_DIR" "$release_id" <<'PY'
import hashlib
import json
import os
from pathlib import Path
import re
import sys


def fail(message):
    raise SystemExit("simulator-demo-runtime: " + message)


simulator_path = Path(sys.argv[1])
zpr_path = Path(sys.argv[2])
release_id = sys.argv[3]
if (
    not zpr_path.is_absolute()
    or os.path.normpath(zpr_path) != str(zpr_path)
    or os.path.realpath(zpr_path) != str(zpr_path)
    or zpr_path.name != "zpr-demo"
    or zpr_path.parent != simulator_path.parent
    or not zpr_path.is_dir()
):
    fail("paired ZPR package must be the canonical sibling installed release")

def read_manifest(package, role):
    manifest_path = package / "manifest.json"
    reference_path = package / "runtime/image-reference"
    if manifest_path.is_symlink() or reference_path.is_symlink():
        fail("paired package metadata must not be symlinks")
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        reference = reference_path.read_bytes()
    except (OSError, UnicodeError, ValueError) as exc:
        fail("cannot read paired " + role + " package metadata: " + str(exc))
    return manifest, reference


simulator, simulator_reference = read_manifest(simulator_path, "Simulator")
zpr, zpr_reference = read_manifest(zpr_path, "ZPR")
simulator_component = simulator.get("component") if isinstance(simulator, dict) else None
zpr_component = zpr.get("component") if isinstance(zpr, dict) else None
image_pattern = re.compile(r"sha256:[0-9a-f]{64}")
if (
    not isinstance(zpr, dict)
    or zpr.get("schema") != "zpr.demo.runtime-package.v2"
    or zpr.get("status") != "demo-unqualified-runtime-package"
    or zpr.get("platform") != "linux/amd64"
    or zpr.get("package_id") != "zpr-demo-image"
    or zpr.get("required_peer_package_id") != "simulator-image"
    or zpr.get("release_id") != release_id
    or not isinstance(zpr_component, dict)
    or zpr_component.get("id") != "zpr-demo-image"
    or not isinstance(zpr_component.get("image_reference"), str)
    or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._/:@+-]{0,254}", zpr_component["image_reference"])
    or not isinstance(zpr_component.get("docker_image_id"), str)
    or not image_pattern.fullmatch(zpr_component["docker_image_id"])
    or not isinstance(zpr_component.get("image_id"), str)
    or not image_pattern.fullmatch(zpr_component["image_id"])
    or not isinstance(simulator_component, dict)
    or simulator.get("release_id") != release_id
    or simulator.get("peer_image_id") != zpr_component["image_id"]
    or simulator.get("peer_docker_image_id") != zpr_component["docker_image_id"]
    or zpr.get("peer_image_id") != simulator_component.get("image_id")
    or zpr.get("peer_docker_image_id") != simulator_component.get("docker_image_id")
    or not isinstance(simulator_component.get("image_reference"), str)
    or simulator_reference != (simulator_component["image_reference"] + "\n").encode("ascii")
    or zpr_reference != (zpr_component["image_reference"] + "\n").encode("ascii")
):
    fail("paired ZPR package does not match the installed Simulator release")

print(
    "zprdemo-" + hashlib.sha256(release_id.encode("ascii")).hexdigest()[:12]
    + "|" + zpr_component["docker_image_id"] + "|" + release_id
)
PY
    ) || fail "paired ZPR runtime validation failed"
    IFS='|' read -r demo_project demo_image demo_release <<EOF
$demo_metadata
EOF
fi

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
        '{{index .Config.Labels "zpr.demo.package"}}|{{index .Config.Labels "zpr.demo.release"}}|{{.Image}}|{{range $port, $bindings := .HostConfig.PortBindings}}{{range $bindings}}{{.HostIp}}:{{.HostPort}}->{{$port}}{{end}}{{end}}|{{index .Config.Labels "zpr.simulator.operator-config-sha256"}}|{{index .Config.Labels "zpr.simulator.packaged-provisioning-enabled"}}|{{index .Config.Labels "zpr.simulator.docker-network"}}|{{.State.Status}}' \
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
    expected_prefix="simulator|$release_id|$expected_image_id|127.0.0.1:$host_port->$container_port/tcp|$operator_config_digest|$enable_provisioning|$docker_network|"
    case "$metadata" in
        "$expected_prefix"*) ;;
        *) fail "container name is occupied by a different or modified container: $container_name" ;;
    esac
}

print_status() {
    metadata=$1
    verify_container "$metadata"
    container_state=${metadata##*|}
    if [ -n "$operator_origin" ]; then
        ui_origin=$operator_origin
    else
        ui_origin="http://127.0.0.1:$host_port"
    fi
    case "$container_state" in
        running) echo "Simulator container running (UI endpoint $ui_origin; application readiness not checked)" ;;
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
            verify_container "$existing_container"
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
        for subdirectory in workspace-db published-directories scenarios provisioning; do
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

        case "$enable_provisioning" in
            1)
                [ -n "$demo_project" ] ||
                    fail "provisioning requires the validated paired ZPR package"
                capability=$(docker image inspect --format '{{index .Config.Labels "zpr.simulator.packaged-provisioning"}}' "$expected_image_id") ||
                    fail "cannot inspect packaged provisioning capability"
                [ "$capability" = 1 ] ||
                    fail "this Simulator image does not support packaged machine provisioning"
                ;;
            0) ;;
        esac

        network_metadata=$(docker network inspect "$docker_network" \
            --format '{{.Driver}}|{{.Internal}}|{{.Scope}}|{{(index .IPAM.Config 0).Gateway}}') ||
            fail "Simulator Docker network is unavailable: $docker_network"
        network_driver=${network_metadata%%|*}
        network_details=${network_metadata#*|}
        network_internal=${network_details%%|*}
        network_details=${network_details#*|}
        network_scope=${network_details%%|*}
        network_gateway=${network_details#*|}
        [ "$network_driver" = bridge ] && [ "$network_internal" = false ] && [ "$network_scope" = local ] ||
            fail "Simulator requires a non-internal local bridge network"
        [ -n "$network_gateway" ] ||
            fail "Simulator Docker network has no gateway address"

        lookup_container
        if [ -n "$existing_container" ]; then
            verify_container "$existing_container"
            container_state=${existing_container##*|}
            if [ "$container_state" = running ]; then
                print_status "$existing_container"
                exit 0
            fi
            docker container start "$container_name" >/dev/null ||
                fail "Docker could not start the existing Simulator container"
        else
            if [ -n "$operator_auth_dir" ]; then
                if [ ! -e "$state_dir/operator-auth" ]; then
                    mkdir -m 700 "$state_dir/operator-auth" ||
                        fail "cannot create Simulator operator auth mountpoint"
                fi
                [ -d "$state_dir/operator-auth" ] && [ ! -L "$state_dir/operator-auth" ] ||
                    fail "Simulator operator auth mountpoint is not a private directory"
                if [ -z "$simulator_operator_trusted_peer" ]; then
                    simulator_operator_trusted_peer=$network_gateway
                fi
                validate_ipv4_address "$simulator_operator_trusted_peer" ||
                    fail "Simulator trusted proxy peer must be an IPv4 address"
            fi

            set -- container run -d \
                --name "$container_name" \
                --label zpr.demo.package=simulator \
                --label "zpr.demo.release=$release_id" \
                --label "zpr.simulator.operator-config-sha256=$operator_config_digest" \
                --label "zpr.simulator.packaged-provisioning-enabled=$enable_provisioning" \
                --label "zpr.simulator.docker-network=$docker_network" \
                --restart unless-stopped \
                --user "$user_id:$group_id" \
                --network "$docker_network"
            if [ "$enable_provisioning" = 1 ]; then
                set -- "$@" \
                    --mount "type=bind,source=$ZPR_DEMO_PACKAGE_DIR,target=$ZPR_DEMO_PACKAGE_DIR,readonly" \
                    --mount "type=bind,source=$state_dir/provisioning,target=$state_dir/provisioning" \
                    -e SIMULATION_PROVISION_PACKAGE="$ZPR_DEMO_PACKAGE_DIR" \
                    -e SIMULATION_PROVISION_ROOT="$state_dir/provisioning" \
                    -e SIMULATION_PROVISION_RELEASE_ID="$release_id"
            fi
            if [ "$socket_gid" != "$group_id" ]; then
                set -- "$@" --group-add "$socket_gid"
            fi
            if [ -n "$operator_origin" ]; then
                set -- "$@" --entrypoint /usr/local/bin/zpr-web-dashboard
            fi
            set -- "$@" \
                --read-only \
                --security-opt no-new-privileges:true \
                --tmpfs /tmp:rw,noexec,nosuid,size=16777216 \
                -p "127.0.0.1:$host_port:$container_port" \
                -v "$docker_socket:/var/run/docker.sock" \
                -v "$runtime_dir:/opt/zpr-simulator/runtime:ro" \
                -v "$state_dir:/var/lib/zpr-simulator"
            if [ -n "$operator_auth_dir" ]; then
                set -- "$@" \
                    --mount "type=bind,source=$operator_auth_dir,target=/var/lib/zpr-simulator/operator-auth,readonly"
                if [ "$docker_network" = bridge ]; then
                    set -- "$@" --add-host "zpr-id.localhost:host-gateway"
                fi
                set -- "$@" \
                    -e ZPR_SIMULATOR_OPERATOR_ORIGIN="$operator_origin" \
                    -e ZPR_SIMULATOR_OPERATOR_CERT_FILE=/var/lib/zpr-simulator/operator-auth/simulator.crt \
                    -e ZPR_SIMULATOR_OPERATOR_KEY_FILE=/var/lib/zpr-simulator/operator-auth/simulator.key \
                    -e ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE=/var/lib/zpr-simulator/operator-auth/oidc.json \
                    -e ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE=/var/lib/zpr-simulator/operator-auth/client.secret \
                    -e ZPR_SIMULATOR_OPERATOR_OIDC_CA_FILE=/var/lib/zpr-simulator/operator-auth/oidc-ca.crt \
                    -e ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP="$simulator_operator_trusted_peer" \
                    -e ZPR_BUDGET_AUTHORITY_URL="$budget_authority_url" \
                    -e ZPR_BUDGET_AUTHORITY_CLIENT_CERT_FILE=/var/lib/zpr-simulator/operator-auth/budget-client.crt \
                    -e ZPR_BUDGET_AUTHORITY_CLIENT_KEY_FILE=/var/lib/zpr-simulator/operator-auth/budget-client.key \
                    -e ZPR_BUDGET_AUTHORITY_CA_FILE=/var/lib/zpr-simulator/operator-auth/budget-ca.crt
                if [ -n "$budget_authority_tls_server_name" ]; then
                    set -- "$@" -e ZPR_BUDGET_AUTHORITY_TLS_SERVER_NAME="$budget_authority_tls_server_name"
                fi
            fi
            set -- "$@" \
                --workdir /opt/zpr-simulator/runtime \
                -e SIMULATION_MANIFEST=/opt/zpr-simulator/runtime/simulation-environment.json \
                -e SIMULATION_ORGANIZATION_ID=demo \
                -e SIMULATION_DEMO_PROJECT="$demo_project" \
                -e SIMULATION_DEMO_IMAGE="$demo_image" \
                -e SIMULATION_DEMO_RELEASE_ID="$demo_release" \
                -e SIMULATION_ORGANIZATIONS_DIR=/opt/zpr-simulator/runtime/organizations \
                -e SIMULATION_SCENARIOS_DIR=/var/lib/zpr-simulator/scenarios \
                -e SIMULATION_ACTIVE_ORGANIZATION_FILE=/var/lib/zpr-simulator/active-organization.txt \
                -e SIMULATION_WORKSPACE_DB_DIR=/var/lib/zpr-simulator/workspace-db \
                -e SIMULATION_PUBLISHED_DIRECTORY_DIR=/var/lib/zpr-simulator/published-directories \
                -e ZPR_WEB_DASHBOARD_BIN=/usr/local/bin/zpr-web-dashboard \
                "$image_reference"
            if [ -n "$operator_origin" ]; then
                set -- "$@" -mode simulator -listen "0.0.0.0:$container_port"
            fi
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
