#!/usr/bin/env python3
"""Provision one explicitly approved organization machine in an independent demo network."""

import contextlib
import fcntl
import hashlib
import importlib.util
import ipaddress
import json
import os
from pathlib import Path
import re
import signal
import shutil
import subprocess
import sys
import tempfile

spec = importlib.util.spec_from_file_location(
    "identity_runtime", Path(__file__).with_name("verify-approved-identity-lab.py"))
identity_runtime = importlib.util.module_from_spec(spec)
spec.loader.exec_module(identity_runtime)


def save_json(path, value):
    fd, temporary = tempfile.mkstemp(prefix=".inventory-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as output:
            json.dump(value, output)
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def read_json(path):
    if path.is_symlink() or not path.is_file():
        raise RuntimeError("expected a regular configuration file: " + str(path))
    return json.loads(path.read_text())


def configuration(organization, machine):
    if (not re.fullmatch(r"[a-z0-9][a-z0-9-]{0,63}", organization)
            or not re.fullmatch(r"[a-z0-9][a-z0-9-]{0,63}", machine)):
        raise RuntimeError("invalid organization or machine identifier")
    profile = read_json(Path(os.environ["SIMULATION_ORGANIZATIONS_DIR"]) / (organization + ".json"))
    manifest = read_json(Path(os.environ["SIMULATION_MANIFEST"]))
    provisioning = profile.get("packaged_provisioning")
    members = provisioning.get("machines") if isinstance(provisioning, dict) else None
    if (profile.get("id") != organization or manifest.get("organization_id") != organization
            or not isinstance(members, list) or not members or members[0] != provisioning.get("primary_machine_id")
            or machine not in members
            or any(not isinstance(item, str) or not re.fullmatch(r"[a-z0-9][a-z0-9-]{0,63}", item)
                   for item in members)
            or len(members) != len(set(members))
            or any(len([entry for entry in manifest.get("machines", []) if entry.get("id") == item]) != 1
                   for item in members)
            or provisioning.get("version") != 2
            or provisioning.get("driver") != "shared-approved-bootstrap"
            or provisioning.get("service") != "OciWeb"):
        raise RuntimeError("machine is not enabled for packaged organization provisioning")
    package = Path(os.environ["SIMULATION_PROVISION_PACKAGE"])
    root = Path(os.environ["SIMULATION_PROVISION_ROOT"])
    for path in (root, package):
        if not path.is_absolute() or path.resolve() != path or path == Path("/"):
            raise RuntimeError("provisioning paths must be canonical absolute paths")
    identity_runtime.private_directory(root)
    metadata = read_json(package / "manifest.json")
    image = metadata.get("component", {}).get("docker_image_id")
    release_id = metadata.get("release_id")
    if (not isinstance(release_id, str)
            or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,79}", release_id)
            or os.environ.get("SIMULATION_PROVISION_RELEASE_ID") != release_id):
        raise RuntimeError("provisioning release does not match the installed paired package")
    if (metadata.get("schema") != "zpr.demo.runtime-package.v2"
            or metadata.get("package_id") != "zpr-demo-image"
            or metadata.get("platform") != "linux/amd64"
            or not isinstance(image, str) or not re.fullmatch(r"sha256:[0-9a-f]{64}", image)):
        raise RuntimeError("provisioning requires an installed pinned Linux/amd64 ZPR package")
    return package, root, image, {**provisioning, "release_id": release_id}


class PackagedMachine(identity_runtime.IdentityLab):
    def __init__(self, package, root, image, organization, machine, provisioning=None):
        self.package, self.root, self.image = package, root, image
        self.organization, self.machine = organization, machine
        provisioning = provisioning or {"machines": [machine], "primary_machine_id": machine}
        self.machine_ids = provisioning["machines"]
        self.primary_machine = provisioning["primary_machine_id"]
        self.release_id = provisioning.get("release_id", "runtime-test")
        self.token = "zpr-org-" + hashlib.sha256(
            (organization + "/" + self.primary_machine + "/" + self.release_id).encode()).hexdigest()[:20]
        self.directory = root / self.token
        self.directory.mkdir(mode=0o700, exist_ok=True)
        identity_runtime.private_directory(self.directory)
        self.inventory = self.directory / "inventory.json"
        self.identity = organization + "." + machine
        self.state = self.directory / "state"
        self.record = {
            "version": 2, "organization_id": organization, "machine_id": self.primary_machine,
            "image": image, "identity": organization + "." + self.primary_machine,
            "state": "unprovisioned", "resources": {}, "pending": [], "network": None,
            "identity_volume": None, "members": {}, "initialized": False,
            "public_key_sha256": None, "error": "",
        }
        self.containers, self.pending_containers = [], []
        self.network = self.identity_volume = None
        self.network_requested = self.volume_requested = False

    def load(self):
        if self.inventory.exists():
            loaded = read_json(self.inventory)
            if loaded.get("version") not in (1, 2):
                raise RuntimeError("unsupported organization inventory version")
            for key in ("organization_id", "machine_id", "image"):
                if loaded.get(key) != self.record[key]:
                    raise RuntimeError("provisioning inventory identity or package image changed")
            if loaded.get("identity") != self.record["identity"]:
                raise RuntimeError("organization's primary endpoint identity changed")
            if loaded["version"] == 1:
                loaded["version"] = 2
                loaded["members"] = {}
                loaded["members"][self.primary_machine] = {
                    "identity": loaded["identity"], "state": loaded["state"],
                    "public_key_sha256": loaded["public_key_sha256"],
                    "identity_volume": loaded["identity_volume"],
                    "container": loaded.get("resources", {}).get(self.token + "-endpoint"),
                }
            loaded.setdefault("members", {})
            volumes = [
                member.get("identity_volume")
                for member in loaded["members"].values()
                if member.get("identity_volume") is not None
            ]
            if len(volumes) != len(set(volumes)):
                raise RuntimeError("organization members must retain distinct endpoint identity volumes")
            self.record = loaded
        self.network = self.record["network"]
        member = self.record["members"].get(self.machine, {})
        if self.machine == self.primary_machine:
            self.identity_volume = member.get("identity_volume") or self.record["identity_volume"]
        else:
            self.identity_volume = member.get("identity_volume")
        self.record["members"].setdefault(self.machine, {
            "identity": self.identity, "state": "unprovisioned",
            "public_key_sha256": None, "identity_volume": self.identity_volume,
            "container": None,
        })
        self.sync_member()

    def save(self):
        self.sync_member()
        save_json(self.inventory, self.record)

    def sync_member(self):
        member = self.record["members"].setdefault(self.machine, {
            "identity": self.identity, "state": "unprovisioned",
            "public_key_sha256": None, "identity_volume": None, "container": None,
        })
        if self.machine == self.primary_machine:
            member.update(identity=self.identity, state=self.record["state"],
                          public_key_sha256=self.record["public_key_sha256"],
                          identity_volume=self.identity_volume, error=self.record["error"])
        else:
            member["identity"] = self.identity
            member.setdefault("state", "unprovisioned")
            member.setdefault("public_key_sha256", None)
            member.setdefault("identity_volume", self.identity_volume)
            member.setdefault("error", "")
        member["container"] = self.record["resources"].get(self.member_resource_name())

    def current_member(self):
        return self.record["members"][self.machine]

    def current_state(self):
        return self.record["state"] if self.machine == self.primary_machine else self.current_member()["state"]

    def set_state(self, state):
        if self.machine == self.primary_machine:
            self.record["state"] = state
        else:
            self.current_member()["state"] = state

    def current_key_hash(self):
        return (self.record["public_key_sha256"] if self.machine == self.primary_machine
                else self.current_member()["public_key_sha256"])

    def set_key_hash(self, value):
        if self.machine == self.primary_machine:
            self.record["public_key_sha256"] = value
        else:
            self.current_member()["public_key_sha256"] = value

    def current_error(self):
        return self.record["error"] if self.machine == self.primary_machine else self.current_member().get("error", "")

    def set_error(self, value):
        if self.machine == self.primary_machine:
            self.record["error"] = value
        else:
            self.current_member()["error"] = value

    def member_resource_name(self):
        suffix = "endpoint" if self.machine == self.primary_machine else "endpoint-" + self.machine
        return self.token + "-" + suffix

    def container_process(self, args):
        return ["--entrypoint", args[0], self.image, *args[1:]]

    def adapter_process(self):
        return ["/app/bin/ph", "adapter"]

    def endpoint(self, role, identity_volume=None):
        return super().endpoint(role, identity_volume=identity_volume)

    def docker(self, *args, **kwargs):
        if args[:2] == ("network", "create") and self.network:
            self.check_network()
            return self.network.encode()
        creating = args[0] == "create"
        if creating:
            name = args[args.index("--name") + 1]
            if name not in self.record["pending"]:
                self.record["pending"].append(name)
            self.save()
        result = super().docker(*args, **kwargs)
        if creating:
            self.record["resources"][name] = result.decode().strip()
            self.record["pending"].remove(name)
        elif args[:2] == ("network", "create"):
            self.network = self.record["network"] = result.decode().strip()
        elif args[:2] == ("volume", "create"):
            self.identity_volume = result.decode().strip()
            if self.machine == self.primary_machine:
                self.record["identity_volume"] = self.identity_volume
            else:
                self.current_member()["identity_volume"] = self.identity_volume
        if creating or args[:2] in (("network", "create"), ("volume", "create")):
            self.save()
        return result

    def check_network(self):
        network = json.loads(super().docker("network", "inspect", self.network))[0]
        if (network.get("Id") != self.network or network.get("Name") != self.token
                or not network.get("Internal")
                or (network.get("Labels") or {}).get("zpr.simulator.identity-lab") != self.token):
            raise RuntimeError("organization network ownership changed")
        return network

    def owned(self, name, container):
        allowed = {self.token + "-" + role for role in ("vs", "node0", "web0")}
        allowed.update(self.member_container_name(machine) for machine in self.machine_ids)
        if name not in allowed:
            raise RuntimeError("unexpected organization resource name")
        config = self.inspect(container)
        if (config["Id"] != container or config["Name"] != "/" + name
                or config["Image"] != self.image
                or config["Config"]["Labels"].get("zpr.simulator.identity-lab") != self.token
                or config["HostConfig"]["NetworkMode"] != self.network):
            raise RuntimeError("organization container ownership changed")
        member_machine = next((machine for machine in self.machine_ids
                               if name == self.member_container_name(machine)), None)
        if member_machine:
            member = self.record["members"].get(member_machine, {})
            expected_volume = member.get("identity_volume")
            if member_machine == self.primary_machine:
                expected_volume = expected_volume or self.record.get("identity_volume")
            if not expected_volume:
                raise RuntimeError("organization endpoint identity volume is missing from inventory")
            self.check_endpoint_storage(config, persistent=True, identity_volume=expected_volume)
        return config

    def member_container_name(self, machine):
        suffix = "endpoint" if machine == self.primary_machine else "endpoint-" + machine
        return self.token + "-" + suffix

    def recover_pending(self):
        for name, container in list(self.record["resources"].items()):
            ids = super().docker("ps", "-a", "--no-trunc", "--filter",
                                 "name=^/" + name + "$", "--format", "{{.ID}}").decode().split()
            if not ids:
                del self.record["resources"][name]
                self.save()
            elif ids != [container]:
                raise RuntimeError("organization resource name was replaced; refusing adoption")
        for name in list(self.record["pending"]):
            ids = super().docker("ps", "-a", "--no-trunc", "--filter",
                                 "name=^/" + name + "$", "--format", "{{.ID}}").decode().split()
            if len(ids) > 1:
                raise RuntimeError("multiple containers match pending organization resource")
            if ids:
                self.owned(name, ids[0])
                self.record["resources"][name] = ids[0]
            self.record["pending"].remove(name)
            self.save()
        if not self.network:
            ids = super().docker("network", "ls", "--filter",
                                 "label=zpr.simulator.identity-lab=" + self.token,
                                 "--format", "{{.ID}}", "--no-trunc").decode().split()
            if len(ids) > 1:
                raise RuntimeError("multiple organization networks found")
            if ids:
                self.network = ids[0]
                self.check_network()
                self.record["network"] = self.network
                self.save()

    def initialize_state(self):
        if not self.record["initialized"]:
            if self.state.exists() and not (self.state / ".zpr-demo-state-v1").is_file():
                raise RuntimeError("incomplete identity initialization retained; inspect state before recovery")
            super().initialize_state()
            self.record["initialized"] = True
            self.save()

    def resources(self):
        resources = {}
        for name, container in self.record["resources"].items():
            role = name.removeprefix(self.token + "-")
            if role.startswith("endpoint") and name != self.member_container_name(self.machine):
                continue
            resources[role] = (container, self.owned(name, container))
        member_name = self.member_container_name(self.machine).removeprefix(self.token + "-")
        if member_name in resources:
            resources["endpoint"] = resources[member_name]
            if member_name != "endpoint":
                del resources[member_name]
        return resources

    def stop(self):
        resources = self.resources()
        endpoint = resources.get("endpoint")
        if endpoint and endpoint[1]["State"]["Running"]:
            self.docker("stop", "--time", "5", endpoint[0])
            if self.inspect(endpoint[0])["State"]["Running"]:
                raise RuntimeError("organization endpoint did not stop")
        self.set_state("stopped")
        self.set_error("")
        self.save()

    def fingerprint(self, endpoint):
        public = self.execute(endpoint, "cat", "/run/zpr/public.pem")
        digest = hashlib.sha256(public).hexdigest()
        if self.current_key_hash() not in (None, digest):
            raise RuntimeError("retained endpoint identity changed; refusing reapproval")
        self.set_key_hash(digest)
        self.save()
        return public

    def node_ip(self):
        network = self.check_network()
        return str(ipaddress.ip_network(network["IPAM"]["Config"][0]["Subnet"])[13])

    def provision(self):
        if self.current_state() in ("ready", "stopped"):
            raise RuntimeError("machine is already provisioned; use start or stop")
        if self.machine != self.primary_machine:
            self.provision_member()
            return
        self.recover_pending()
        for _, (container, _) in self.resources().items():
            self.docker("rm", "-f", container)
            self.record["resources"] = {
                name: value for name, value in self.record["resources"].items() if value != container}
            self.save()
        self.set_state("provisioning")
        self.set_error("")
        self.save()
        # The retained volume is never removed by a retry; only public keys enter policy.
        _, _, _, endpoint, _, _, public = self.prepare()
        self.fingerprint(endpoint)
        if public != self.execute(endpoint, "cat", "/run/zpr/public.pem"):
            raise RuntimeError("endpoint public key changed during provisioning")
        self.set_state("ready")
        self.set_error("")
        self.save()

    def compile_endpoint_policy(self, public_key):
        public_name = "approved-public-key-" + self.machine + ".pem"
        (self.state / "include" / public_name).write_bytes(public_key)
        compiler = self.state / "admin/multinode-demo.zplc"
        source = compiler.read_text()
        identity_entry = '"' + self.identity + '" = "../include/' + public_name + '"'
        if identity_entry not in source:
            marker = "[bootstrap]"
            if marker not in source:
                raise RuntimeError("organization policy has no explicit bootstrap section")
            source = source.replace(marker, marker + "\n" + identity_entry, 1)
            compiler.write_text(source)
        attributes_path = self.state / "conf/vs/attrfile.json"
        attributes = read_json(attributes_path)
        attributes.setdefault("device.zpr.adapter.cn", {})[self.identity] = {
            "location": ["cloud"], "access": ["cloud"]}
        save_json(attributes_path, attributes)
        public_include = self.directory / "compiler-public"
        public_include.mkdir(mode=0o700, exist_ok=True)
        for path in (self.state / "include").glob("*.pem"):
            shutil.copyfile(path, public_include / path.name)
        self.docker(
            "run", "--rm", "--pull", "never", "--network", "none", "--read-only",
            "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
            "--user", f"{os.getuid()}:{os.getgid()}",
            "--mount", f"type=bind,source={self.state / 'admin'},target=/admin",
            "--mount", f"type=bind,source={public_include},target=/include,readonly",
            "--workdir", "/admin", "--entrypoint", "/app/bin/zplc", self.image,
            "--config", "multinode-demo.zplc", "multinode-demo.zpl")
        shutil.copyfile(self.state / "admin/multinode-demo.bin2",
                        self.state / "conf/vs/multinode-demo.bin2")

    def provision_member(self):
        self.recover_pending()
        if not self.record["initialized"] or not self.network or not self.record["identity_volume"]:
            raise RuntimeError("the shared organization topology must be provisioned first")
        resources = self.resources()
        if not all(role in resources for role in ("vs", "node0", "web0")):
            raise RuntimeError("shared organization node, Visa Service and OciWeb must be present")
        if any(not resources[role][1]["State"]["Running"] for role in ("vs", "node0", "web0")):
            raise RuntimeError("shared organization services must be running before adding a machine")
        self.set_state("provisioning")
        self.set_error("")
        self.save()
        volume = self.current_member().get("identity_volume")
        if volume is None:
            volume = self.docker(
                "volume", "create", "--driver", "local", "--label",
                "zpr.simulator.identity-lab=" + self.token,
                self.token + "-identity-" + self.machine).decode().strip()
            self.current_member()["identity_volume"] = volume
            self.save()
        self.identity_volume = volume
        endpoint = self.endpoint("endpoint-" + self.machine, identity_volume=volume)
        public_key = self.fingerprint(endpoint)
        self.compile_endpoint_policy(public_key)
        visa = resources["vs"][0]
        try:
            self.execute(visa, "pgrep", "-x", "vs")
        except subprocess.CalledProcessError:
            pass
        else:
            self.docker("exec", visa, "pkill", "-x", "vs")

        def visa_stopped():
            try:
                self.execute(visa, "pgrep", "-x", "vs")
            except subprocess.CalledProcessError:
                return True
            return False

        identity_runtime.wait_for("Visa Service policy reload", visa_stopped)
        self.sync_service_file(visa, "multinode-demo.bin2")
        self.sync_service_file(visa, "attrfile.json")
        self.launch(visa, "vs", ["/app/bin/vs", "multinode-demo.bin2"])
        identity_runtime.wait_for("Visa Service actor API", lambda: self._actor_available(visa))
        self.admin(visa, "services", "--id", "attrfile", "--flush", write=True)
        self.prepare_endpoint(endpoint)
        node_ip = self.node_ip()
        if not self.adapter_active(endpoint):
            self.start_adapter(endpoint, node_ip, self.identity)
        identity_runtime.wait_for("second endpoint adapter link", lambda: b"(Active)" in self.execute(
            endpoint, "sh", "-c",
            'test ! -S /run/zpr/control.sock || /app/bin/ph-cli -p /run/zpr/control.sock link show'))
        actor = self.actor(visa)
        if actor.get("cn") != self.identity or actor.get("node"):
            raise RuntimeError("Visa did not admit the explicitly approved organization endpoint")
        attributes = {entry["key"]: entry["value"] for entry in actor.get("attrs", [])}
        if attributes.get("user.access") != ["cloud"]:
            raise RuntimeError("approved organization endpoint lacks the trusted OciWeb attribute")
        self.verify_unapproved_key(visa, endpoint, node_ip, actor)
        member = self.current_member()
        member["container"] = self.record["resources"][self.member_resource_name()]
        member.update(state="ready", error="")
        self.fetch(endpoint)
        primary = self.record["members"][self.primary_machine]
        self.fetch(primary["container"])
        self.set_error("")
        self.save()

    def verify_unapproved_key(self, visa, endpoint, node_ip, approved_actor):
        policy = self.execute(visa, "redis-cli", "-e", "--json", "HGETALL", "policy:current")
        impostor = self.endpoint("wrong-key")
        try:
            if self.execute(impostor, "cat", "/run/zpr/public.pem") == self.execute(
                    endpoint, "cat", "/run/zpr/public.pem"):
                raise RuntimeError("wrong-key test did not generate an independent key")
            offset = len(self.service_log(visa, "vs.log"))
            self.start_adapter(impostor, node_ip, self.identity)

            def signature_rejected():
                return ("blob signature verification failed for cn " + self.identity).encode() in \
                    self.service_log(visa, "vs.log")[offset:]

            identity_runtime.wait_for("explicit wrong-key signature rejection", signature_rejected, attempts=30)
            self.fetch(endpoint)
            current = self.actor(visa)
            if current.get("zpr_addr") != approved_actor.get("zpr_addr"):
                raise RuntimeError("wrong-key attempt replaced the approved actor")
            if policy != self.execute(visa, "redis-cli", "-e", "--json", "HGETALL", "policy:current"):
                raise RuntimeError("organization policy changed during wrong-key check")
        finally:
            name = self.token + "-wrong-key"
            container = self.record["resources"].get(name)
            if container:
                self.docker("rm", "-f", container)
                self.record["resources"].pop(name, None)
                self.save()

    def _actor_available(self, visa):
        try:
            actor = json.loads(self.admin(
                visa, "actors", "--cn", self.organization + "." + self.primary_machine))
            return actor.get("cn") == self.organization + "." + self.primary_machine
        except (subprocess.SubprocessError, RuntimeError, ValueError):
            return False

    def start(self):
        resources = self.resources()
        if not {"vs", "node0", "web0", "endpoint"} <= set(resources):
            raise RuntimeError("incomplete provisioning; approve and retry provisioning")
        if any(not resources[role][1]["State"]["Running"] for role in ("vs", "node0", "web0")):
            raise RuntimeError("shared organization services are stopped; service recovery must be performed first")
        self.set_state("starting")
        self.save()
        if not resources["endpoint"][1]["State"]["Running"]:
            self.docker("start", resources["endpoint"][0])
        endpoint = resources["endpoint"][0]
        self.prepare_endpoint(endpoint)
        self.fingerprint(endpoint)
        if not self.adapter_active(endpoint):
            self.start_adapter(endpoint, self.node_ip(), self.identity)
        identity_runtime.wait_for("approved adapter link", lambda: b"(Active)" in self.execute(
            endpoint, "sh", "-c",
            'test ! -S /run/zpr/control.sock || /app/bin/ph-cli -p /run/zpr/control.sock link show'))
        for _ in range(6):
            self.fetch(endpoint)
        self.set_state("ready")
        self.set_error("")
        self.save()

    def verify(self):
        resources = self.resources()
        endpoint, visa, web = [resources[role][0] for role in ("endpoint", "vs", "web0")]
        self.fingerprint(endpoint)
        actor = self.actor(visa)
        self.authorization_cycle(visa, endpoint, web, actor)

    def adapter_active(self, endpoint):
        try:
            return b"(Active)" in self.execute(
                endpoint, "sh", "-c",
                'test ! -S /run/zpr/control.sock || /app/bin/ph-cli -p /run/zpr/control.sock link show')
        except subprocess.CalledProcessError:
            return False

    def status(self):
        resources = self.resources()
        state = self.current_state()
        error = self.current_error()
        if state == "ready":
            if len(resources) != 4 or not all(config["State"]["Running"] for _, config in resources.values()):
                state = "unavailable"
                error = "one or more provisioned resources are not running"
            else:
                self.fingerprint(resources["endpoint"][0])
                try:
                    self.fetch(resources["endpoint"][0])
                except (subprocess.SubprocessError, RuntimeError) as exc:
                    state, error = "unavailable", "protected HTTP readiness failed: " + str(exc)
        return {
            "version": 1, "organization_id": self.organization, "machine_id": self.machine,
            "state": state, "identity": self.identity,
            "public_key_sha256": self.current_key_hash(),
            "container_id": resources.get("endpoint", ("",))[0],
            "resources": {role: {"container_id": container, "running": config["State"]["Running"]}
                          for role, (container, config) in resources.items()},
            "error": error,
            "scope": "Shared organization node/Visa/OciWeb; each provisioned member retains an independent approved endpoint key. Demo bootstrap only, not production enrollment or human login.",
        }


def main():
    if len(sys.argv) != 4 or sys.argv[1] not in ("status", "approve-and-provision", "start", "stop", "verify"):
        raise SystemExit("usage: packaged-machine-runtime.py status|approve-and-provision|start|stop|verify ORGANIZATION MACHINE")
    os.umask(0o077)
    action, organization, machine = sys.argv[1:]
    package, root, image, provisioning = configuration(organization, machine)
    runtime = PackagedMachine(package, root, image, organization, machine, provisioning)
    lock_path = runtime.directory / "operation.lock"
    if lock_path.is_symlink():
        raise RuntimeError("operation lock must not be a symlink")
    with lock_path.open("a") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as exc:
            raise RuntimeError("another provisioning operation is in progress") from exc
        runtime.load()
        def interrupted(signum, frame):
            raise RuntimeError("provisioning interrupted by signal " + str(signum))
        signal.signal(signal.SIGTERM, interrupted)
        signal.signal(signal.SIGINT, interrupted)
        try:
            with contextlib.redirect_stdout(sys.stderr):
                if action == "approve-and-provision":
                    runtime.provision()
                elif action == "start":
                    runtime.start()
                elif action == "stop":
                    runtime.stop()
                elif action == "verify":
                    runtime.verify()
                result = runtime.status()
        except (RuntimeError, OSError, subprocess.SubprocessError, ValueError, KeyError) as exc:
            if action != "status":
                runtime.set_state("failed")
                runtime.set_error(str(exc))
                runtime.save()
            raise
        print(json.dumps(result))


if __name__ == "__main__":
    main()
