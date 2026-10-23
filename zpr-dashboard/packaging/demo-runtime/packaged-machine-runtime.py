#!/usr/bin/env python3
"""Provision one explicitly approved organization machine in an independent demo network."""

import contextlib
import fcntl
import hashlib
import http.client
import importlib.util
import ipaddress
import json
import os
from pathlib import Path
import re
import signal
import shutil
import socket
import ssl
import stat
import subprocess
import sys
import tempfile
import time
from urllib.parse import urlsplit

spec = importlib.util.spec_from_file_location(
    "identity_runtime", Path(__file__).with_name("verify-approved-identity-lab.py"))
identity_runtime = importlib.util.module_from_spec(spec)
spec.loader.exec_module(identity_runtime)

NORTHSTAR_WORKLOAD = {
    "version": 1,
    "machine_id": "machine-01",
    "name": "finance-client",
    "interface": "tun0",
}

NORTHSTAR_MACHINE_ASSETS = (
    ("machine_cert", "SIMULATION_NORTHSTAR_MACHINE_CERT_DIR", (
        ("control-ca.crt", False),
        ("client.crt", False),
        ("client.key", True),
    )),
    ("workload", "SIMULATION_NORTHSTAR_WORKLOAD_DIR", (
        ("ca.crt", False),
        ("client-finance-rsa.key", True),
        ("machine-controller-machine-01.key", True),
    )),
)


class MachineControlHTTPSConnection(http.client.HTTPSConnection):
    def connect(self):
        self.sock = socket.create_connection(
            (self.host, self.port), self.timeout, self.source_address)
        self.sock = self._context.wrap_socket(
            self.sock, server_hostname="host.docker.internal")


def certificate_common_name(path):
    certificate = ssl._ssl._test_decode_cert(str(path))
    return [
        value
        for relative_name in certificate.get("subject", ())
        for key, value in relative_name
        if key == "commonName"
    ]


def probe_machine_control_https(settings, organization, generation):
    connection = None
    try:
        parsed = urlsplit(settings["control_url"])
        context = ssl.create_default_context(cafile=str(
            settings["machine_cert_dir"] / "control-ca.crt"))
        context.minimum_version = ssl.TLSVersion.TLSv1_3
        context.load_cert_chain(
            certfile=str(settings["machine_cert_dir"] / "client.crt"),
            keyfile=str(settings["machine_cert_dir"] / "client.key"),
        )
        connection = MachineControlHTTPSConnection(
            parsed.hostname, parsed.port, context=context, timeout=5)
        connection.request(
            "GET", "/internal/ping",
            headers={
                "X-ZPR-Organization": organization,
                "X-ZPR-Generation": generation,
            },
        )
        response = connection.getresponse()
        status = response.status
        response.close()
        if status != http.client.NO_CONTENT:
            return {
                "state": "blocked",
                "diagnostic": "mTLS ping returned HTTP " + str(status),
            }
        return {
            "state": "passed",
            "method": "GET /internal/ping",
            "tls_minimum": "TLS 1.3",
            "server_name": "host.docker.internal",
            "scope_headers_sent": True,
        }
    except (OSError, ssl.SSLError, http.client.HTTPException, ValueError) as exc:
        return {
            "state": "blocked",
            "diagnostic": type(exc).__name__ + ": " + str(exc),
        }
    finally:
        if connection is not None:
            connection.close()


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


def northstar_workload_contract(profile, organization, machine, provisioning, manifest):
    workload = profile.get("synthetic_workload")
    if workload is None:
        return None
    machines = provisioning.get("machines") if isinstance(provisioning, dict) else None
    manifest_machines = manifest.get("machines") if isinstance(manifest, dict) else None
    machine_ids = [
        item.get("id") if isinstance(item, dict) else None
        for item in manifest_machines
    ] if isinstance(manifest_machines, list) else []
    if (
        organization != "northstar"
        or not isinstance(provisioning, dict)
        or profile.get("id") != "northstar"
        or manifest.get("organization_id") != "northstar"
        or profile.get("synthetic_lab_only") is not True
        or workload != NORTHSTAR_WORKLOAD
        or machine != "machine-01"
        or provisioning.get("version") != 2
        or provisioning.get("driver") != "shared-approved-bootstrap"
        or provisioning.get("primary_machine_id") != "machine-01"
        or machines != ["machine-01"]
        or provisioning.get("service") != "OciWeb"
        or len(machine_ids) != 20
        or any(not isinstance(item, str) or not re.fullmatch(
            r"machine-[0-9]{2}", item) for item in machine_ids)
        or len(set(machine_ids)) != len(machine_ids)
        or set(machine_ids) != {f"machine-{number:02d}" for number in range(1, 21)}
        or machine_ids.count("machine-01") != 1
    ):
        raise RuntimeError("unsupported Northstar synthetic workload contract")
    return dict(workload)


def configuration(organization, machine):
    if (not re.fullmatch(r"[a-z0-9][a-z0-9-]{0,63}", organization)
            or not re.fullmatch(r"[a-z0-9][a-z0-9-]{0,63}", machine)):
        raise RuntimeError("invalid organization or machine identifier")
    profile = read_json(Path(os.environ["SIMULATION_ORGANIZATIONS_DIR"]) / (organization + ".json"))
    manifest = read_json(Path(os.environ["SIMULATION_MANIFEST"]))
    if not isinstance(profile, dict) or not isinstance(manifest, dict):
        raise RuntimeError("organization profile and simulation manifest must be JSON objects")
    provisioning = profile.get("packaged_provisioning")
    synthetic_workload = northstar_workload_contract(
        profile, organization, machine, provisioning, manifest)
    if organization == "northstar" and synthetic_workload is None:
        raise RuntimeError(
            "unsupported Northstar synthetic workload contract; provisioning is not enabled")
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
            or provisioning.get("driver") != "shared-approved-bootstrap"):
        raise RuntimeError("machine is not enabled for packaged organization provisioning")
    if provisioning.get("service") != "OciWeb":
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
    return package, root, image, {
        **provisioning,
        "release_id": release_id,
        "synthetic_workload": synthetic_workload,
    }


class PackagedMachine(identity_runtime.IdentityLab):
    def __init__(self, package, root, image, organization, machine, provisioning=None,
                 *, create_directory=True):
        self.package, self.root, self.image = package, root, image
        self.organization, self.machine = organization, machine
        provisioning = provisioning or {"machines": [machine], "primary_machine_id": machine}
        self.machine_ids = provisioning["machines"]
        self.primary_machine = provisioning["primary_machine_id"]
        self.release_id = provisioning.get("release_id", "runtime-test")
        self.synthetic_workload = provisioning.get("synthetic_workload")
        self.synthetic = self.synthetic_workload is not None
        self.token = "zpr-org-" + hashlib.sha256(
            (organization + "/" + self.primary_machine + "/" + self.release_id).encode()).hexdigest()[:20]
        self.directory = root / self.token
        if create_directory:
            self.directory.mkdir(mode=0o700, exist_ok=True)
        if self.directory.exists():
            identity_runtime.private_directory(self.directory)
        elif create_directory:
            raise RuntimeError("provisioning state directory is unavailable")
        self.inventory = self.directory / "inventory.json"
        self.identity = organization + "." + machine
        self.state = self.directory / "state"
        self.record = {
            "version": 2, "organization_id": organization, "machine_id": self.primary_machine,
            "image": image, "identity": organization + "." + self.primary_machine,
            "state": "unprovisioned", "resources": {}, "pending": [], "network": None,
            "identity_volume": None, "members": {}, "initialized": False,
            "public_key_sha256": None, "runtime_image": None, "error": "",
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
        self.record.setdefault("runtime_image", None)
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

    def synthetic_settings(self):
        if not self.synthetic or self.organization != "northstar" or self.machine != "machine-01":
            raise RuntimeError("Northstar synthetic machine runtime is not enabled")

        def private_directory(variable):
            value = os.environ.get(variable, "")
            path = Path(value)
            if not path.is_absolute() or path.resolve() != path or path == Path("/"):
                raise RuntimeError(variable + " must be a canonical absolute directory")
            try:
                info = path.lstat()
            except OSError as exc:
                raise RuntimeError(variable + " is unavailable") from exc
            if (not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid()
                    or stat.S_IMODE(info.st_mode) != 0o700):
                raise RuntimeError(variable + " must be a private invoking-user-owned directory")
            return path

        def regular_asset(directory, name, *, secret=False):
            path = directory / name
            try:
                info = path.lstat()
            except OSError as exc:
                raise RuntimeError("required synthetic runtime asset is unavailable: " + name) from exc
            mode = stat.S_IMODE(info.st_mode)
            if (not stat.S_ISREG(info.st_mode) or path.is_symlink()
                    or info.st_uid != os.geteuid()
                    or secret and mode & 0o077):
                raise RuntimeError("synthetic runtime asset has unsafe type or permissions: " + name)
            return path

        machine_image = os.environ.get("SIMULATION_NORTHSTAR_MACHINE_IMAGE", "")
        if not re.fullmatch(r"sha256:[0-9a-f]{64}", machine_image):
            raise RuntimeError("Northstar machine image must be pinned by sha256 digest")
        network = os.environ.get("SIMULATION_NORTHSTAR_DOCKER_NETWORK", "")
        if not re.fullmatch(r"zpr-northstar(?:-synthetic)?", network):
            raise RuntimeError("Northstar Docker network must be the configured synthetic network")
        node_address = os.environ.get("SIMULATION_NORTHSTAR_NODE_ADDRESS", "")
        host, separator, port = node_address.rpartition(":")
        try:
            node_ip = ipaddress.ip_address(host)
        except ValueError as exc:
            raise RuntimeError("Northstar node address must be a reachable IPv4 address and port") from exc
        if (not separator or port != "5000" or node_ip.version != 4
                or node_ip.is_loopback or node_ip.is_link_local or node_ip.is_multicast
                or node_ip.is_unspecified or node_ip.is_reserved
                or node_ip in ipaddress.ip_network("192.0.2.0/24")
                or node_ip in ipaddress.ip_network("198.51.100.0/24")
                or node_ip in ipaddress.ip_network("203.0.113.0/24")):
            raise RuntimeError("Northstar node address must not use a documentation or local-only endpoint")
        control_url = os.environ.get("SIMULATION_NORTHSTAR_CONTROL_URL", "")
        try:
            parsed_control_url = urlsplit(control_url)
            control_port = parsed_control_url.port
        except ValueError as exc:
            raise RuntimeError("Northstar control URL is invalid") from exc
        if (parsed_control_url.scheme != "https" or not parsed_control_url.hostname
                or control_port != 8792 or parsed_control_url.username
                or parsed_control_url.password or parsed_control_url.path not in ("", "/")
                or parsed_control_url.query or parsed_control_url.fragment):
            raise RuntimeError("Northstar control URL must be an HTTPS machine-control endpoint")
        generation = os.environ.get("SIMULATION_RUNTIME_GENERATION", "")
        if not re.fullmatch(r"[a-z0-9][a-z0-9-]{0,127}", generation):
            raise RuntimeError("Northstar runtime generation is invalid")

        machine_cert_dir = private_directory("SIMULATION_NORTHSTAR_MACHINE_CERT_DIR")
        workload_dir = private_directory("SIMULATION_NORTHSTAR_WORKLOAD_DIR")
        for name in ("control-ca.crt", "client.crt"):
            regular_asset(machine_cert_dir, name)
        regular_asset(machine_cert_dir, "client.key", secret=True)
        regular_asset(workload_dir, "ca.crt")
        for name in ("client-finance-rsa.key", "machine-controller-machine-01.key"):
            regular_asset(workload_dir, name, secret=True)

        return {
            "image": machine_image,
            "network": network,
            "node_address": node_address,
            "control_url": control_url,
            "generation": generation,
            "machine_cert_dir": machine_cert_dir,
            "workload_dir": workload_dir,
        }

    def synthetic_asset_report(self):
        report = {}
        for category, variable, assets in NORTHSTAR_MACHINE_ASSETS:
            value = os.environ.get(variable, "")
            directory = Path(value) if value else None
            directory_state = "ready"
            if directory is None:
                directory_state = "missing"
            else:
                try:
                    info = directory.lstat()
                except OSError:
                    directory_state = "missing"
                else:
                    if (
                        not directory.is_absolute()
                        or directory.resolve() != directory
                        or not stat.S_ISDIR(info.st_mode)
                        or info.st_uid != os.geteuid()
                        or stat.S_IMODE(info.st_mode) != 0o700
                    ):
                        directory_state = "unsafe"

            asset_report = {}
            for name, secret in assets:
                if directory_state != "ready":
                    asset_report[name] = directory_state
                    continue
                path = directory / name
                try:
                    info = path.lstat()
                except OSError:
                    asset_report[name] = "missing"
                    continue
                if (
                    not stat.S_ISREG(info.st_mode)
                    or path.is_symlink()
                    or info.st_uid != os.geteuid()
                    or secret and stat.S_IMODE(info.st_mode) & 0o077
                ):
                    asset_report[name] = "unsafe"
                else:
                    asset_report[name] = "ready"
            report[category] = {
                "directory_variable": variable,
                "directory": directory_state,
                "assets": asset_report,
            }
        return report

    def probe_synthetic_readiness(self):
        if not self.synthetic or self.organization != "northstar" or self.machine != "machine-01":
            raise RuntimeError("Northstar synthetic machine runtime is not enabled")

        assets = self.synthetic_asset_report()
        blockers = [
            category + "/" + name + " is " + state
            for category, result in assets.items()
            for name, state in result["assets"].items()
            if state != "ready"
        ]
        generation = os.environ.get("SIMULATION_RUNTIME_GENERATION", "")
        settings = None
        configuration_error = None
        try:
            settings = self.synthetic_settings()
        except (RuntimeError, OSError, ValueError) as exc:
            configuration_error = str(exc)
            blockers.append(configuration_error)

        certificate_identity = {"state": "blocked"}
        control_ping = {"state": "not_run"}
        if settings is not None:
            try:
                names = certificate_common_name(
                    settings["machine_cert_dir"] / "client.crt")
                if names != ["machine-01"]:
                    certificate_identity = {
                        "state": "blocked",
                        "diagnostic": "client certificate CN must be machine-01",
                    }
                    blockers.append("machine controller certificate identity mismatch")
                else:
                    certificate_identity = {
                        "state": "passed",
                        "common_name": "machine-01",
                    }
                    control_ping = probe_machine_control_https(
                        settings, self.organization, generation)
                    if control_ping["state"] != "passed":
                        blockers.append(control_ping["diagnostic"])
            except (OSError, ssl.SSLError, ValueError) as exc:
                certificate_identity = {
                    "state": "blocked",
                    "diagnostic": type(exc).__name__ + ": " + str(exc),
                }
                blockers.append("machine controller certificate could not be validated")

        machine = {"state": "not_checked"}
        if settings is not None:
            try:
                status = self.status()
                machine = {
                    "state": status["state"],
                    "controller_adapter_active": status["controller_link_active"],
                    "workload": status["workload"],
                }
                if status["state"] != "running" or not status["controller_link_active"]:
                    blockers.append("packaged machine controller adapter is not active")
                if status["workload"]["state"] == "unavailable":
                    blockers.append("finance-client tun0 binding is unavailable")
            except (OSError, RuntimeError, subprocess.SubprocessError, ValueError, KeyError) as exc:
                machine = {
                    "state": "unavailable",
                    "diagnostic": type(exc).__name__ + ": " + str(exc),
                }
                blockers.append("packaged machine status could not be read")

        return {
            "version": 1,
            "scope": {
                "organization_id": self.organization,
                "machine_id": self.machine,
                "generation_configured": bool(generation),
                "workload": self.synthetic_workload,
            },
            "assets": assets,
            "certificate_identity": certificate_identity,
            "control_endpoint": control_ping,
            "machine": machine,
            "controller_heartbeat": {
                "state": "not_checked",
                "owner_action": "wait_controller",
                "note": (
                    "C0 wait_controller confirms the heartbeat with organization/generation "
                    "scope; GET /internal/ping bypasses scope checks and proves no heartbeat."
                ),
            },
            "ready": False,
            "blockers": blockers,
        }

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
        if self.synthetic:
            return self.synthetic_owned(name, container)
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

    def synthetic_owned(self, name, container):
        settings = self.synthetic_settings()
        expected_name = self.member_resource_name()
        if name != expected_name:
            raise RuntimeError("unexpected Northstar synthetic resource name")
        config = self.inspect(container)
        labels = config["Config"].get("Labels") or {}
        host = config["HostConfig"]
        expected_mounts = {
            "/run/zpr-machine": settings["machine_cert_dir"],
            "/opt/zpr-workloads/ca.crt": settings["workload_dir"] / "ca.crt",
            "/opt/zpr-workloads/client-finance-rsa.key":
                settings["workload_dir"] / "client-finance-rsa.key",
            "/opt/zpr-workloads/machine-controller-machine-01.key":
                settings["workload_dir"] / "machine-controller-machine-01.key",
        }
        mounts = {
            item.get("Destination"): item
            for item in config.get("Mounts", [])
        }
        if (config["Id"] != container or config["Name"] != "/" + name
                or config["Image"] != settings["image"]
                or labels.get("zpr.simulator.identity-lab") != self.token
                or labels.get("zpr.organization.id") != "northstar"
                or labels.get("zpr.runtime.generation") != settings["generation"]
                or labels.get("zpr.machine.id") != "machine-01"
                or labels.get("zpr.simulator.packaged-workload")
                != "northstar-finance-client-v1"
                or host["NetworkMode"] != settings["network"]
                or host["Privileged"]
                or not host["ReadonlyRootfs"]
                or host["CapDrop"] != ["ALL"]
                or host["CapAdd"] != ["NET_ADMIN"]
                or host["PortBindings"]
                or not all(
                    mounts.get(target, {}).get("Type") == "bind"
                    and mounts[target].get("Source") == str(source)
                    and mounts[target].get("RW") is False
                    for target, source in expected_mounts.items()
                )):
            raise RuntimeError("Northstar synthetic machine ownership or isolation changed")
        return config

    def synthetic_container_args(self, settings):
        name = self.member_resource_name()
        workload_dir = settings["workload_dir"]
        machine_cert_dir = settings["machine_cert_dir"]
        mounts = (
            (machine_cert_dir, "/run/zpr-machine"),
            (workload_dir / "ca.crt", "/opt/zpr-workloads/ca.crt"),
            (workload_dir / "client-finance-rsa.key",
             "/opt/zpr-workloads/client-finance-rsa.key"),
            (workload_dir / "machine-controller-machine-01.key",
             "/opt/zpr-workloads/machine-controller-machine-01.key"),
        )
        args = [
            "create", "--pull", "never", "--name", name,
            "--label", "zpr.simulator.identity-lab=" + self.token,
            "--label", "zpr.organization.id=northstar",
            "--label", "zpr.runtime.generation=" + settings["generation"],
            "--label", "zpr.machine.id=machine-01",
            "--label", "zpr.simulator.packaged-workload=northstar-finance-client-v1",
            "--network", settings["network"],
            "--read-only", "--cap-drop", "ALL", "--cap-add", "NET_ADMIN",
            "--device", "/dev/net/tun",
            "--security-opt", "no-new-privileges:true",
            "--pids-limit", "64", "--memory", "256m", "--cpus", "1",
            "--tmpfs", "/run/zpr-workloads:rw,noexec,nosuid,size=4m,mode=0700",
            "--tmpfs", "/run/zpr-simulator:rw,noexec,nosuid,size=1m,mode=0700",
            "--tmpfs", "/tmp:rw,nosuid,size=16m",
        ]
        for source, target in mounts:
            args.extend(("--mount",
                         f"type=bind,source={source},target={target},readonly"))
        args.extend((
            "-e", "SIMULATION_ORGANIZATION_ID=northstar",
            "-e", "SIMULATION_RUNTIME_GENERATION=" + settings["generation"],
            "-e", "SIMULATION_NORTHSTAR_MACHINE_ID=machine-01",
            "-e", "SIMULATION_NORTHSTAR_CONTROL_URL=" + settings["control_url"],
            "-e", "SIMULATION_NORTHSTAR_NODE_ADDRESS=" + settings["node_address"],
            settings["image"],
        ))
        return args

    def recover_synthetic_pending(self, settings):
        expected_name = self.member_resource_name()
        if (self.record.get("runtime_image") not in (None, settings["image"])
                or self.record.get("network") not in (None, settings["network"])):
            raise RuntimeError("Northstar runtime image or network changed; refusing adoption")
        for name in list(self.record["resources"]):
            if name != expected_name:
                raise RuntimeError("unexpected resource in Northstar synthetic inventory")
            container = self.record["resources"][name]
            ids = super().docker(
                "ps", "-a", "--no-trunc", "--filter", "name=^/" + name + "$",
                "--format", "{{.ID}}").decode().split()
            if not ids:
                del self.record["resources"][name]
                self.save()
            elif ids != [container]:
                raise RuntimeError("Northstar machine container was replaced; refusing adoption")
            else:
                self.synthetic_owned(name, container)
        for name in list(self.record["pending"]):
            if name != expected_name:
                raise RuntimeError("unexpected pending Northstar synthetic resource")
            ids = super().docker(
                "ps", "-a", "--no-trunc", "--filter", "name=^/" + name + "$",
                "--format", "{{.ID}}").decode().split()
            if len(ids) > 1:
                raise RuntimeError("multiple containers match pending Northstar machine")
            if ids:
                self.synthetic_owned(name, ids[0])
                self.record["resources"][name] = ids[0]
            self.record["pending"].remove(name)
            self.save()
        self.network = settings["network"]
        self.record["network"] = settings["network"]
        self.record["runtime_image"] = settings["image"]
        self.save()

    def validate_synthetic_runtime(self, settings):
        image_identity = self.docker(
            "image", "inspect", "--format", "{{.Id}} {{.Os}}/{{.Architecture}}",
            settings["image"]).decode().strip()
        if image_identity != settings["image"] + " linux/amd64":
            raise RuntimeError("Northstar machine image is not the installed pinned Linux/amd64 image")
        networks = json.loads(self.docker("network", "inspect", settings["network"]))
        if len(networks) != 1 or networks[0].get("Name") != settings["network"]:
            raise RuntimeError("configured Northstar Docker network is unavailable")
        node_ip = settings["node_address"].rsplit(":", 1)[0]
        attached_node = any(
            details.get("IPv4Address", "").split("/", 1)[0] == node_ip
            for details in (networks[0].get("Containers") or {}).values()
        )
        if not attached_node:
            raise RuntimeError("Northstar node address is not attached to the configured Docker network")

    def synthetic_link_active(self, container, socket):
        present = self.execute(
            container, "sh", "-c",
            "if [ -S " + socket + " ]; then printf present; else printf absent; fi",
        ).strip()
        if present == b"absent":
            return False
        if present != b"present":
            raise RuntimeError("Northstar adapter socket probe returned invalid output")
        output = self.execute(
            container, "/app/bin/ph-cli", "-p", socket, "link", "show")
        return b"(Active)" in output

    def synthetic_controller_active(self, container):
        return self.synthetic_link_active(
            container, "/run/zpr-workloads/machine-control.sock")

    def synthetic_workload_status(self, container):
        socket = "/run/zpr-workloads/finance-client.sock"
        if not self.synthetic_link_active(container, socket):
            return {"state": "not_started", "address": ""}
        addresses = json.loads(self.execute(
            container, "ip", "-j", "-6", "addr", "show", "dev", "tun0"))
        addresses = [
            (item.get("local"), item.get("scope"))
            for interface in addresses if isinstance(interface, dict)
            for item in interface.get("addr_info", []) if isinstance(item, dict)
        ]
        for address, scope in addresses:
            if scope == "global" and isinstance(address, str) and address.startswith("fd5a:5052:"):
                return {"state": "active", "address": address}
        fallback_address = "fd00:1:4::1"
        if not any(scope == "global" and address == fallback_address
                   for address, scope in addresses):
            return {"state": "unavailable", "address": ""}
        return {"state": "active", "address": fallback_address}

    def wait_synthetic_controller(self, container):
        identity_runtime.wait_for(
            "Northstar machine-control adapter",
            lambda: self.synthetic_controller_active(container),
            attempts=90,
        )

    def create_synthetic_machine(self, settings):
        self.validate_synthetic_runtime(settings)
        self.network = settings["network"]
        self.record["network"] = settings["network"]
        self.record["runtime_image"] = settings["image"]
        self.save()
        container = self.docker(*self.synthetic_container_args(settings)).decode().strip()
        if not container:
            raise RuntimeError("Northstar machine container creation returned no identifier")
        self.docker("start", container)
        self.wait_synthetic_controller(container)
        self.set_state("running")
        self.set_error("")
        self.save()

    def provision_synthetic(self):
        if self.current_state() in ("ready", "running", "stopped"):
            raise RuntimeError("machine is already provisioned; use start or stop")
        settings = self.synthetic_settings()
        self.validate_synthetic_runtime(settings)
        self.recover_synthetic_pending(settings)
        self.set_state("provisioning")
        self.set_error("")
        self.save()
        existing = self.record["resources"].get(self.member_resource_name())
        if existing is None:
            self.create_synthetic_machine(settings)
            return
        config = self.synthetic_owned(self.member_resource_name(), existing)
        if not config["State"]["Running"]:
            self.docker("start", existing)
        self.wait_synthetic_controller(existing)
        self.set_state("running")
        self.set_error("")
        self.save()

    def start_synthetic(self):
        if self.current_state() == "unprovisioned":
            raise RuntimeError("incomplete provisioning; approve and retry provisioning")
        settings = self.synthetic_settings()
        self.validate_synthetic_runtime(settings)
        self.recover_synthetic_pending(settings)
        resources = self.resources()
        endpoint = resources.get("endpoint")
        if endpoint is None:
            raise RuntimeError("Northstar machine container is unavailable")
        if not endpoint[1]["State"]["Running"]:
            self.docker("start", endpoint[0])
        self.wait_synthetic_controller(endpoint[0])
        self.set_state("running")
        self.set_error("")
        self.save()

    def member_container_name(self, machine):
        suffix = "endpoint" if machine == self.primary_machine else "endpoint-" + machine
        return self.token + "-" + suffix

    def recover_pending(self):
        if self.synthetic:
            self.recover_synthetic_pending(self.synthetic_settings())
            return
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
        if self.synthetic:
            name = self.member_resource_name()
            container = self.record["resources"].get(name)
            return {} if container is None else {
                "endpoint": (container, self.owned(name, container)),
            }
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
        if self.synthetic:
            resources = self.resources()
            endpoint = resources.get("endpoint")
            if endpoint and endpoint[1]["State"]["Running"]:
                self.docker("stop", "--time", "5", endpoint[0])
                if self.inspect(endpoint[0])["State"]["Running"]:
                    raise RuntimeError("Northstar machine did not stop")
            self.set_state("stopped")
            self.set_error("")
            self.save()
            return
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
        if self.synthetic:
            self.provision_synthetic()
            return
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
        if self.synthetic:
            self.start_synthetic()
            return
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
        if self.synthetic:
            result = self.status()
            if result["state"] != "running" or not result["controller_link_active"]:
                raise RuntimeError(result.get("error") or "Northstar machine controller is not ready")
            return
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
        if self.synthetic:
            resources = self.resources()
            state = self.current_state()
            error = self.current_error()
            controller_link_active = False
            workload = {"state": "not_started", "address": ""}
            endpoint = resources.get("endpoint")
            if state not in ("unprovisioned", "stopped", "failed"):
                if endpoint is None or not endpoint[1]["State"]["Running"]:
                    state = "unavailable"
                    error = "Northstar machine container is not running"
                else:
                    controller_link_active = self.synthetic_controller_active(endpoint[0])
                    if not controller_link_active:
                        state = "unavailable"
                        error = "Northstar machine-control adapter is not active"
                    else:
                        workload = self.synthetic_workload_status(endpoint[0])
                        if workload["state"] == "unavailable":
                            state = "unavailable"
                            error = "finance-client tun0 does not have the configured ZPR address"
            return {
                "version": 1,
                "organization_id": self.organization,
                "machine_id": self.machine,
                "state": state,
                "identity": "finance-client",
                "container_id": endpoint[0] if endpoint else "",
                "resources": {
                    "machine": {
                        "container_id": endpoint[0],
                        "running": endpoint[1]["State"]["Running"],
                    }
                } if endpoint else {},
                "controller_link_active": controller_link_active,
                "workload": {
                    "name": "finance-client",
                    "interface": "tun0",
                    **workload,
                },
                "error": error,
                "scope": (
                    "Northstar synthetic machine-control and finance-client adapter only; "
                    "no service endpoint or OciWeb readiness is included."
                ),
            }
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
    actions = ("status", "approve-and-provision", "start", "stop", "verify", "probe")
    if len(sys.argv) != 4 or sys.argv[1] not in actions:
        raise SystemExit(
            "usage: packaged-machine-runtime.py "
            "status|approve-and-provision|start|stop|verify|probe ORGANIZATION MACHINE")
    os.umask(0o077)
    action, organization, machine = sys.argv[1:]
    package, root, image, provisioning = configuration(organization, machine)
    read_only_probe = action == "probe"
    runtime = PackagedMachine(
        package, root, image, organization, machine, provisioning,
        create_directory=not read_only_probe)
    if read_only_probe:
        runtime.load()
        result = runtime.probe_synthetic_readiness()
        result["probe_state"] = (
            "blocked" if result["blockers"]
            else "transport_verified_controller_heartbeat_pending")
        print(json.dumps(result))
        return 2 if result["blockers"] else 0
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
    return 0


if __name__ == "__main__":
    sys.exit(main())
