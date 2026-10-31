#!/bin/sh
set -eu

HERE=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
WORKSPACE=$(CDPATH='' cd -- "$HERE/.." && pwd)
TEST_ROOT="$WORKSPACE/.machine-auth-handoff-test-$$"
TRACE="$TEST_ROOT/docker-args"
mkdir "$TEST_ROOT"
chmod 700 "$TEST_ROOT"
trap 'rm -rf "$TEST_ROOT"' EXIT HUP INT TERM

python3 - "$TEST_ROOT" <<'PY'
import json
import os
import sys

root = sys.argv[1]
assets = (
    "velocity-machine-03-controller",
    "velocity-machine-03-finance-client",
    "velocity-machine-05-controller",
    "velocity-machine-05-echo-service",
    "velocity-simulator-control",
)
for asset_id in assets:
    asset = os.path.join(root, asset_id)
    os.mkdir(asset, 0o700)
    directory = os.path.join(asset, "identity")
    os.mkdir(directory, 0o700)
    for name in ("runtime-auth.key", "runtime-auth-ca.pem"):
        with open(os.path.join(directory, name), "w", encoding="ascii") as output:
            output.write("operator-prepared fixture\n")
        os.chmod(os.path.join(directory, name), 0o600)
    with open(os.path.join(directory, "runtime-auth.toml"), "w", encoding="utf-8") as output:
        output.write(
            "[global]\n[adapter]\n[authentication]\n"
            f'bas_key = {json.dumps(os.path.join(directory, "runtime-auth.key"))}\n'
            f'tls_ca_file = {json.dumps(os.path.join(directory, "runtime-auth-ca.pem"))}\n'
            f'bas_client_id = "{asset_id}"\n'
        )
    os.chmod(os.path.join(directory, "runtime-auth.toml"), 0o600)
with open(os.path.join(root, "manifest.json"), "w", encoding="utf-8") as output:
    json.dump({"organization_id": "velocity", "machines": [
        {"id": f"machine-{number:02}", "type": "computer", "model": "fixture", "secure": True}
        for number in (1, 3, 5)
    ]}, output)
PY

SIMULATION_MACHINE_AUTHENTICATION_DIR="$TEST_ROOT" \
SIMULATION_MANIFEST="$TEST_ROOT/manifest.json" \
SIMULATION_ORGANIZATION_ID=velocity \
ZPR_DASHBOARD_STACK_LIBRARY=1 \
. "$HERE/dashboard-stack.sh"

test_fail() {
    echo "$*" >&2
    exit 1
}

machine_profile="$WORKSPACE/cmd/zpr-web-dashboard/examples/organizations/velocity.json"
MACHINE_WORKLOAD_DIR="$TEST_ROOT/workloads"
MACHINE_CONTROL_ADDRESS_FILE="$TEST_ROOT/control-address"
MACHINE_CONTROLLER_BIN="$TEST_ROOT/controller-arm64"
MACHINE_CONTROLLER_AMD64_BIN="$TEST_ROOT/controller-amd64"
MACHINE_CONTROL_PROXY_BIN="$TEST_ROOT/proxy-arm64"
MACHINE_CONTROL_PROXY_AMD64_BIN="$TEST_ROOT/proxy-amd64"
MACHINE_CERT_DIR="$TEST_ROOT/certs"
MACHINE_IMAGE=fixture-machine
SIMULATOR_DOCKER_CONTAINER=fixture-simulator
ANTHROPIC_API_KEY=
ZPR_MULTINODE_PLATFORM=linux/amd64
mkdir -p "$MACHINE_WORKLOAD_DIR" "$MACHINE_CERT_DIR"
printf '%s\n' fd5a:5052:adda:1::10 > "$MACHINE_CONTROL_ADDRESS_FILE"
printf '%s\n' "$TEST_ROOT" > "$TEST_ROOT/auth-root"

resolve_machine_runtime() {
    machine_organization=${TEST_ORGANIZATION:-velocity}
    machine_generation=fixture
    machine_runtime_driver=docker-multinode
    machine_profile="$WORKSPACE/cmd/zpr-web-dashboard/examples/organizations/velocity.json"
    MACHINE_WORKLOAD_DIR="$TEST_ROOT/workloads"
}

resolve_machine_placement() {
    machine_owner=fixture-owner
    machine_location="Test Location"
    machine_node_index=0
    machine_node_ip=172.30.0.13
}

ensure_machine_runtime_arch() { :; }
refresh_multinode_services_if_idle() { :; }
inherit_multinode_platform() { :; }
load_multinode_platform_file() { :; }
configure_multinode_platform() {
    ZPR_MULTINODE_PLATFORM=linux/amd64
    ZPR_MULTINODE_IMAGE=fixture-multinode
    ZPR_MULTINODE_DIRECTORY_IMAGE=fixture-directory
    ZPR_MULTINODE_DIRECTORY_MANAGER_IMAGE=fixture-directory-manager
    ZPR_TRUSTED_SERVICE_BINARY=
}
ensure_simulator_image() { :; }
wait_for_url() { :; }
stop_legacy_named_workloads() { :; }
install_machine_controller() { :; }
configure_multinode_service_return_route() { :; }
sleep() { :; }
prepare_machine_workloads() {
    prepare_machine_authentication_workload_configs
}

callback_started=no
docker() {
    printf 'CALL\n' >> "$TRACE"
    for docker_arg do printf 'ARG:%s\n' "$docker_arg" >> "$TRACE"; done
    case "$1" in
        info)
            printf '%s\n' x86_64
            ;;
        inspect)
            case "$*" in
                *".State.Status"*) return 1 ;;
                *".State.Running"*) printf '%s\n' "${TEST_SIMULATOR_RUNNING:-false}" ;;
                *".Config.Env"*)
                    jq -nc --arg root "$TEST_ROOT" '["SIMULATION_MACHINE_AUTHENTICATION_DIR=" + $root]'
                    ;;
                *".Mounts"*)
                    if [ "${TEST_SIMULATOR_BROAD_MOUNT:-no}" = yes ]; then
                        jq -nc --arg root "$TEST_ROOT" '[{Source:$root,Destination:$root,RW:false}]'
                    else
                        jq -nc --arg root "$TEST_ROOT" '
                            [["velocity-machine-03-controller", "velocity-machine-03-finance-client",
                              "velocity-machine-05-controller", "velocity-machine-05-echo-service",
                              "velocity-simulator-control"][] as $asset |
                             ["runtime-auth.toml", "runtime-auth.key", "runtime-auth-ca.pem"][] |
                             {Source:($root + "/" + $asset + "/identity/" + .)} |
                             . + {Destination:.Source,RW:false}]'
                    fi
                    ;;
                *".NetworkSettings.Networks"*) printf '%s\n' 192.0.2.10 ;;
                *"fixture-simulator"*) return 1 ;;
                *) return 1 ;;
            esac
            ;;
        network)
            printf '%s\n' 192.0.2.1
            ;;
        exec)
            case "$*" in
                *"/app/bin/ph-cli"*)
                    if [ "$callback_started" = yes ]; then
                        printf '%s\n' '(Active)'
                    else
                        printf '%s\n' '(Inactive)'
                    fi
                    ;;
                *"ip -6 -o addr show"*)
                    printf '%s\n' '1: tun5 inet6 fd5a:5052:adda:1::10/64 scope global'
                    ;;
                *"ps -eo"*) ;;
            esac
            case "$*" in
                *"-d velocity-node0 sh -c exec env ZPR_ADAPTER_SERVICES=SimulatorControlService"*)
                    callback_started=yes
                    ;;
            esac
            ;;
        run|cp|network|ps|rm)
            ;;
        *)
            test_fail "unexpected Docker invocation: $*"
            ;;
    esac
}

resolve_machine_runtime
machine_authentication_preflight || test_fail "retained endpoint preflight rejected complete fixtures"

# Verify that the operator-owned endpoint files reach the Simulator process
# that owns the lifecycle command, through a read-only exact-path bind.
start_simulator
grep -Fq "ARG:$TEST_ROOT/velocity-simulator-control/identity/runtime-auth.toml:$TEST_ROOT/velocity-simulator-control/identity/runtime-auth.toml:ro" "$TRACE" ||
    test_fail "Simulator did not receive the exact runtime file mount"
if grep -Fq "ARG:$TEST_ROOT:$TEST_ROOT:ro" "$TRACE"; then
    test_fail "Simulator received a broad endpoint root mount"
fi
grep -Fq "ARG:SIMULATION_MACHINE_AUTHENTICATION_DIR=$TEST_ROOT" "$TRACE" ||
    test_fail "Simulator lifecycle process did not receive the retained BAS opt-in"
TEST_SIMULATOR_RUNNING=true
start_simulator
TEST_SIMULATOR_BROAD_MOUNT=yes
if start_simulator 2>/dev/null; then
    test_fail "running Simulator with a broad root mount was accepted"
fi
TEST_SIMULATOR_RUNNING=false
TEST_SIMULATOR_BROAD_MOUNT=no

# Exercise the real docker-run argument construction while replacing every operation
# that could mutate a live runtime, including legacy workload cleanup.
start_machine_container machine-03
start_machine_container machine-05

grep -Fq "ARG:SIMULATION_MACHINE_AUTHENTICATION_CONFIG=$TEST_ROOT/velocity-machine-03-controller/identity/runtime-auth.toml" "$TRACE" ||
    test_fail "machine-03 controller did not receive its exact BAS config path"
grep -Fq "ARG:$TEST_ROOT/velocity-machine-03-finance-client/identity/runtime-auth.key:$TEST_ROOT/velocity-machine-03-finance-client/identity/runtime-auth.key:ro" "$TRACE" ||
    test_fail "machine-03 finance endpoint files were not mounted at their canonical paths"
grep -Fq "ARG:SIMULATION_MACHINE_AUTHENTICATION_CONFIG=$TEST_ROOT/velocity-machine-05-controller/identity/runtime-auth.toml" "$TRACE" ||
    test_fail "machine-05 controller did not receive its exact BAS config path"
grep -Fq "ARG:$TEST_ROOT/velocity-machine-05-echo-service/identity/runtime-auth.key:$TEST_ROOT/velocity-machine-05-echo-service/identity/runtime-auth.key:ro" "$TRACE" ||
    test_fail "machine-05 echo endpoint files were not mounted at their canonical paths"
if grep -Fq 'ARG:-zpr-bootstrap-key' "$TRACE"; then
    test_fail "retained machine controllers received a bootstrap fallback"
fi

test -f "$TEST_ROOT/workloads/authentication/finance-client.toml" ||
    test_fail "finance workload config was not staged at its expected assets path"
test -f "$TEST_ROOT/workloads/authentication/echo-service.toml" ||
    test_fail "echo workload config was not staged at its expected assets path"

# The live callback service launcher is exercised by the machine starts against
# a Docker mock. Its process-management and filesystem operations are recorded,
# never executed.
grep -Fq 'ARG:--config-file' "$TRACE" ||
    test_fail "SimulatorControl callback did not select a BAS config"
grep -Fq "ARG:$TEST_ROOT/velocity-simulator-control/identity/runtime-auth.toml" "$TRACE" ||
    test_fail "SimulatorControl callback used the wrong endpoint configuration"
if grep -Fq 'ARG:--bootstrap-key' "$TRACE"; then
    test_fail "retained SimulatorControl callback received a bootstrap fallback"
fi

mv "$TEST_ROOT/velocity-machine-05-echo-service/identity/runtime-auth-ca.pem" \
    "$TEST_ROOT/velocity-machine-05-echo-service/identity/runtime-auth-ca.pem.saved"
if machine_authentication_preflight 2>/dev/null; then
    test_fail "missing retained CA was accepted"
fi

SIMULATION_MACHINE_AUTHENTICATION_DIR=
start_machine_container machine-01
grep -Fq 'ARG:-zpr-bootstrap-key' "$TRACE" ||
    test_fail "unset opt-in did not preserve the legacy controller bootstrap mode"

echo "machine BAS authentication handoff tests passed"
