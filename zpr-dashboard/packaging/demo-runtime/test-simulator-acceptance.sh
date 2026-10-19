#!/bin/sh
set -eu
: "${SIMULATION_DEMO_PROJECT:?set the installed Compose project}"
python3 - <<'PY'
import hashlib
import json
import os
import subprocess
import time
import urllib.request

base = "http://127.0.0.1:8786"
project = os.environ["SIMULATION_DEMO_PROJECT"]
server, visa = project + "-web0-1", project + "-vs-1"
workload = project + "-scenario-http"
identity_workload = project + "-scenario-identity"
scenario_id = "packaged-admission-acceptance"

def docker(*args):
    return subprocess.check_output(["docker", *args], timeout=15)

def policy_hash():
    return hashlib.sha256(docker("exec", visa, "redis-cli", "-e", "--json", "HGETALL", "policy:current")).hexdigest()

def request(path, mutation=False):
    req = urllib.request.Request(base + path, data=b"{}" if mutation else None,
                                 headers={"Content-Type": "application/json", "Origin": base})
    with urllib.request.urlopen(req, timeout=15) as response:
        return json.load(response)

def snapshot():
    return request("/api/simulator/scenarios")["run"]

def finish():
    for _ in range(450):
        run = snapshot()
        if run["state"] not in ("running", "cleaning"):
            return run
        time.sleep(0.2)
    raise AssertionError("scenario did not finish within 90 seconds")

def verify_cleanup():
    assert docker("ps", "-a", "--filter", "name=^/" + workload + "$", "--format", "{{.ID}}").strip() == b"", "workload container leaked"
    assert docker("ps", "-a", "--filter", "name=^/" + identity_workload + "$", "--format", "{{.ID}}").strip() == b"", "identity container leaked"
    docker("exec", server, "test", "!", "-e", "/tmp/zpr-packaged-demo-probe")
    docker("exec", server, "awk",
           '$2 ~ /:46A0$/ && $4 == "0A" {found=1} END {exit found ? 1 : 0}',
           "/proc/net/tcp6", "/proc/net/tcp")

before = policy_hash()
catalog = request("/api/simulator/scenarios")
assert any(s["id"] == scenario_id and s["published_revision"] == 1
           for s in catalog["scenarios"]), "published acceptance scenario missing"
request("/api/simulator/scenarios/" + scenario_id + "/run", True)
run = finish()
assert run["state"] == "completed" and len(run["steps"]) == 1, run
output = run["steps"][0].get("output", "")
assert run["steps"][0]["status"] == "completed", run
for evidence in ("Alice -> OciWeb TCP 80", "independently serves HTTP",
                 "eval denied (no match)", "no matching policy", "active policy unchanged",
                 "Simulator provisioned isolated workload", "Simulator-provisioned workload container removed",
                 "Simulator provisioned independent unapproved identity simulator-unapproved-",
                 "no key found in policy for cn simulator-unapproved-",
                 "independent identity container and private key removed",
                 "temporary listener stopped and probe lock removed"):
    assert evidence in output, (evidence, run)
verify_cleanup()
print("PASS: Simulator completed published scenario with allowed HTTP and explicit policy denial")

request("/api/simulator/scenarios/" + scenario_id + "/run", True)
for _ in range(100):
    ids = docker("ps", "--filter", "name=^/" + workload + "$", "--format", "{{.ID}}").strip()
    if ids:
        config = json.loads(docker("inspect", ids.decode()))[0]
        assert config["Image"] == os.environ["SIMULATION_DEMO_IMAGE"], config["Image"]
        assert config["Config"]["User"] == "65534:65534"
        assert config["Mounts"] == [], config["Mounts"]
        host = config["HostConfig"]
        assert host["ReadonlyRootfs"] and host["CapDrop"] == ["ALL"] and not host["Privileged"], host
        assert "no-new-privileges:true" in host["SecurityOpt"], host["SecurityOpt"]
        assert host["NetworkMode"].startswith("container:"), host["NetworkMode"]
        network_peer = host["NetworkMode"].removeprefix("container:")
        assert docker("inspect", "--format", "{{.Id}}", network_peer) == docker("inspect", "--format", "{{.Id}}", server)
        assert not host["PortBindings"] and not host["VolumesFrom"], host
        assert host["PidsLimit"] == 16 and host["Memory"] == 32 * 1024 * 1024, host
        assert host["NanoCpus"] == 250_000_000, host["NanoCpus"]
        break
    assert snapshot()["state"] == "running", "scenario stopped before cancellation check"
    time.sleep(0.05)
else:
    raise AssertionError("probe did not acquire its lock")
request("/api/simulator/scenarios/cancel", True)
run = finish()
assert run["state"] == "cancelled", run
verify_cleanup()
assert policy_hash() == before, "active policy changed"
print("PASS: cancellation removed temporary resources; active policy preserved")

request("/api/simulator/scenarios/" + scenario_id + "/run", True)
assert finish()["state"] == "completed", snapshot()
verify_cleanup()
assert policy_hash() == before, "active policy changed on rerun"
print("PASS: successful rerun after cancellation without reprovisioning")

# An unrelated same-named container must cause explicit failure, never adoption.
foreign = docker("create", "--pull", "never", "--name", workload,
                 "--network", "none", "--entrypoint", "/bin/true",
                 os.environ["SIMULATION_DEMO_IMAGE"]).decode().strip()
try:
    request("/api/simulator/scenarios/" + scenario_id + "/run", True)
    run = finish()
    assert run["state"] == "failed", run
    assert "never adopted or removed" in run["steps"][0].get("output", ""), run
    assert json.loads(docker("inspect", foreign))[0]["State"]["Status"] == "created"
    docker("exec", server, "test", "!", "-e", "/tmp/zpr-packaged-demo-probe")
    assert policy_hash() == before, "policy changed during conflict test"
    print("PASS: unrelated same-named workload preserved; scenario failed explicitly")
finally:
    docker("rm", foreign)
request("/api/simulator/scenarios/" + scenario_id + "/run", True)
assert finish()["state"] == "completed", snapshot()
verify_cleanup()
assert policy_hash() == before, "active policy changed"
print("PASS: scenario recovered after workload-name conflict")

request("/api/simulator/scenarios/" + scenario_id + "/run", True)
for _ in range(300):
    ids = docker("ps", "--filter", "name=^/" + identity_workload + "$", "--format", "{{.ID}}").strip()
    if ids:
        config = json.loads(docker("inspect", ids.decode()))[0]
        assert config["Image"] == os.environ["SIMULATION_DEMO_IMAGE"], config["Image"]
        labels = config["Config"]["Labels"]
        assert labels["zpr.simulator.workload.project"] == project
        assert labels["zpr.simulator.workload.role"] == "unapproved-identity"
        assert labels["zpr.simulator.workload.identity"].startswith("simulator-unapproved-")
        assert not any("PRIVATE KEY" in entry for entry in config["Config"]["Env"])
        host = config["HostConfig"]
        assert host["ReadonlyRootfs"] and not host["Privileged"], host
        assert host["CapDrop"] == ["ALL"], host
        assert [cap.removeprefix("CAP_") for cap in host["CapAdd"]] == ["NET_ADMIN"], host
        assert host["NetworkMode"] == project + "_zpr-demo", host["NetworkMode"]
        assert "no-new-privileges:true" in host["SecurityOpt"], host["SecurityOpt"]
        assert host["PidsLimit"] == 32 and host["Memory"] == 96 * 1024 * 1024, host
        assert host["NanoCpus"] == 500_000_000 and not host["PortBindings"], host
        assert host["Tmpfs"] == {"/run/zpr": "rw,noexec,nosuid,size=4m,mode=0700"}, host["Tmpfs"]
        assert len(host["Devices"]) == 1 and host["Devices"][0]["PathOnHost"] == "/dev/net/tun"
        assert not host["Binds"] and not host.get("Mounts") and not host["VolumesFrom"], host
        assert all(m["Type"] == "tmpfs" and m["Destination"] == "/run/zpr" for m in config["Mounts"]), config["Mounts"]
        break
    assert snapshot()["state"] == "running", "scenario finished before independent adapter inspection"
    time.sleep(0.05)
else:
    raise AssertionError("independent adapter did not start")
request("/api/simulator/scenarios/cancel", True)
assert finish()["state"] == "cancelled", snapshot()
verify_cleanup()
assert policy_hash() == before, "active policy changed on identity cancellation"
print("PASS: separate adapter restricted to NET_ADMIN/TUN, no host keys; cancellation removed its private-key tmpfs")

foreign = docker("create", "--pull", "never", "--name", identity_workload,
                 "--network", "none", "--entrypoint", "/bin/true",
                 os.environ["SIMULATION_DEMO_IMAGE"]).decode().strip()
try:
    request("/api/simulator/scenarios/" + scenario_id + "/run", True)
    run = finish()
    assert run["state"] == "failed", run
    assert "never adopted or removed" in run["steps"][0].get("output", ""), run
    assert json.loads(docker("inspect", foreign))[0]["State"]["Status"] == "created"
    assert policy_hash() == before, "active policy changed on identity conflict"
    print("PASS: unrelated identity container preserved on conflict")
finally:
    docker("rm", foreign)
request("/api/simulator/scenarios/" + scenario_id + "/run", True)
assert finish()["state"] == "completed", snapshot()
verify_cleanup()
assert policy_hash() == before, "active policy changed on final admission check"
print("PASS: independent identity rejection and traffic checks recovered after cancellation and conflicts")
PY
