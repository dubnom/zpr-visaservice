#!/bin/sh
set -eu
umask 077

fail() { echo "simulator-acceptance: $*" >&2; exit 1; }
case "${1:-}" in start|stop|status) action=$1 ;; *) fail "usage: simulator-acceptance-runtime.sh start|stop|status" ;; esac
[ "$#" = 1 ] || fail "one action is required"
: "${ZPR_SIMULATOR_PACKAGE_DIR:?set the installed Simulator package directory}"
: "${ZPR_SIMULATOR_STATE_DIR:?set a separate private acceptance state directory}"
: "${ZPR_SIMULATOR_ACCEPTANCE_IMAGE:?set the immutable acceptance candidate image ID}"
: "${SIMULATION_DEMO_PROJECT:?set the installed ZPR Compose project}"
: "${SIMULATION_DEMO_IMAGE:?set the immutable installed ZPR image ID}"
python3 - "$ZPR_SIMULATOR_PACKAGE_DIR" "$ZPR_SIMULATOR_STATE_DIR" <<'PY'
import json
import os
from pathlib import Path
import stat
import sys

package, state = map(Path, sys.argv[1:])
for path in (package, state):
    if not path.is_absolute() or str(path.resolve()) != str(path) or str(path) == "/":
        raise SystemExit("package/state must be canonical absolute paths without symlinks")
if package == state or package in state.parents or state in package.parents:
    raise SystemExit("package and acceptance state must not overlap")
with (package / "manifest.json").open() as source:
    manifest = json.load(source)
if (manifest.get("schema") != "zpr.demo.runtime-package.v2"
        or manifest.get("package_id") != "simulator-image"
        or manifest.get("platform") != "linux/amd64"):
    raise SystemExit("expected the installed Linux/amd64 Simulator runtime package")
if not state.exists():
    state.mkdir(mode=0o700)
info = state.lstat()
if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o077:
    raise SystemExit("acceptance state must be private and invoking-user-owned")
for name in ("workspace-db", "published-directories"):
    path = state / name
    path.mkdir(mode=0o700, exist_ok=True)
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o077:
        raise SystemExit("unsafe acceptance state subdirectory: " + str(path))
PY
socket=${DOCKER_HOST:-unix:///var/run/docker.sock}
case "$socket" in unix:///*) socket=${socket#unix://} ;; *) fail "a local Unix Docker socket is required" ;; esac
[ -S "$socket" ] && [ ! -L "$socket" ] || fail "invalid Docker socket"
[ "$(docker image inspect --format '{{.Id}} {{.Os}}/{{.Architecture}}' "$ZPR_SIMULATOR_ACCEPTANCE_IMAGE")" = "$ZPR_SIMULATOR_ACCEPTANCE_IMAGE linux/amd64" ] ||
    fail "expected an immutable Linux/amd64 acceptance candidate"
name=zpr-simulator-acceptance
existing=$(docker ps -a --filter "name=^/$name$" --format '{{.ID}}')
upgrade=no
if [ -n "$existing" ]; then
    existing_image=$(docker inspect --format '{{.Image}}' "$name")
    if [ "$existing_image" != "$ZPR_SIMULATOR_ACCEPTANCE_IMAGE" ]; then
        case "$existing_image" in
            "${ZPR_SIMULATOR_PREVIOUS_IMAGE:-}"|"${ZPR_SIMULATOR_LEGACY_IMAGE:-}") upgrade=yes ;;
            *) fail "existing candidate uses an unreviewed image; not modified" ;;
        esac
    fi
    actual=$(docker inspect --format '{{.Image}}|{{index .Config.Labels "zpr.acceptance.state"}}|{{index .Config.Labels "zpr.acceptance.project"}}|{{index .Config.Labels "zpr.acceptance.image"}}|{{.Config.User}}' "$name")
    [ "$actual" = "$existing_image|$ZPR_SIMULATOR_STATE_DIR|$SIMULATION_DEMO_PROJECT|$SIMULATION_DEMO_IMAGE|$(id -u):$(id -g)" ] ||
        fail "existing candidate belongs to different state, images, project or user; not modified"
fi
stop_candidate() {
    if [ -n "$existing" ] && [ "$(docker inspect --format '{{.State.Running}}' "$name")" = true ]; then
        cancellation=$(curl --noproxy '*' -sS --max-time 5 -X POST \
            -H 'Origin: http://127.0.0.1:8786' \
            -o /dev/null -w '%{http_code}' \
            http://127.0.0.1:8786/api/simulator/scenarios/cancel) ||
            fail "cannot request scenario cancellation; candidate not stopped"
        case "$cancellation" in 202|409) ;; *) fail "cancellation returned HTTP $cancellation; candidate not stopped" ;; esac
        attempt=0
        until curl --noproxy '*' -fsS --max-time 3 http://127.0.0.1:8786/api/simulator/scenarios |
            python3 -c 'import json,sys; d=json.load(sys.stdin); sys.exit(1 if d["run"]["state"] in ("running","cleaning") else 0)'; do
            attempt=$((attempt + 1))
            [ "$attempt" -lt 20 ] || fail "scenario cleanup did not finish; candidate not stopped"
            sleep 1
        done
        docker stop --time 20 "$name" >/dev/null
    fi
    echo "Acceptance candidate stopped; network and persistent state retained."
}
case "$action" in
    stop)
        stop_candidate
        exit 0 ;;
    status)
        if [ "$upgrade" = yes ]; then
            echo "upgrade required; use simulator-start before simulator-test"
        elif [ -n "$existing" ]; then
            docker inspect --format '{{.State.Status}}' "$name"
        else
            echo "not created"
        fi
        exit 0 ;;
esac
if [ "$upgrade" = yes ]; then
    stop_candidate
    docker rm "$existing" >/dev/null
    existing=
    echo "Upgrading verified managed Simulator image; persistent workspace retained."
fi
if [ -n "$existing" ]; then
    docker start "$name" >/dev/null
else
    echo "WARNING: disposable host only; Simulator receives root-equivalent Docker socket access." >&2
    docker run -d --name "$name" \
        --label "zpr.acceptance.state=$ZPR_SIMULATOR_STATE_DIR" \
        --label "zpr.acceptance.project=$SIMULATION_DEMO_PROJECT" \
        --label "zpr.acceptance.image=$SIMULATION_DEMO_IMAGE" \
        --user "$(id -u):$(id -g)" --group-add "$(stat -c '%g' "$socket")" \
        --read-only --cap-drop ALL --security-opt no-new-privileges:true \
        --tmpfs /tmp:rw,noexec,nosuid,size=16777216 \
        -p 127.0.0.1:8786:8788 \
        --mount "type=bind,source=$socket,target=/var/run/docker.sock" \
        --mount "type=bind,source=$ZPR_SIMULATOR_PACKAGE_DIR/runtime,target=/opt/zpr-simulator/runtime,readonly" \
        --mount "type=bind,source=$ZPR_SIMULATOR_STATE_DIR,target=/var/lib/zpr-simulator" \
        -e SIMULATION_MANIFEST=/opt/zpr-simulator/runtime/simulation-environment.json \
        -e SIMULATION_ORGANIZATIONS_DIR=/opt/zpr-simulator/runtime/organizations \
        -e SIMULATION_ORGANIZATION_ID=demo \
        -e SIMULATION_ACTIVE_ORGANIZATION_FILE=/var/lib/zpr-simulator/active-organization.txt \
        -e SIMULATION_WORKSPACE_DB_DIR=/var/lib/zpr-simulator/workspace-db \
        -e SIMULATION_PUBLISHED_DIRECTORY_DIR=/var/lib/zpr-simulator/published-directories \
        -e SIMULATION_DEMO_PROJECT -e SIMULATION_DEMO_IMAGE \
        "$ZPR_SIMULATOR_ACCEPTANCE_IMAGE" >/dev/null
fi
attempt=0
until curl --noproxy '*' -fsS --max-time 3 http://127.0.0.1:8786/api/simulator/scenarios >/dev/null; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 20 ] || fail "candidate did not become ready; inspect docker logs $name"
    sleep 1
done
echo "Simulator acceptance candidate: http://127.0.0.1:8786/scenarios.html"
echo "Scenario available; run packaged-admission-acceptance to verify workload traffic and unapproved identity rejection."
