#!/usr/bin/env python3
"""Exercise bootstrap approval and attribute authorization in an isolated ZPR lab."""

import ipaddress
import json
import os
from pathlib import Path
import re
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time


def command(*args, data=None, timeout=60):
    return subprocess.run(args, input=data, stdout=subprocess.PIPE, check=True,
                          timeout=timeout).stdout


def wait_for(description, probe, attempts=60):
    for _ in range(attempts):
        result = probe()
        if result:
            return result
        time.sleep(1)
    raise RuntimeError("timed out waiting for " + description)


def private_directory(path):
    info = path.lstat()
    if (not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid()
            or stat.S_IMODE(info.st_mode) != 0o700):
        raise RuntimeError("expected private invoking-user-owned directory: " + str(path))


class IdentityLab:
    def __init__(self, package, root):
        self.package = package
        self.root = root
        self.directory = Path(tempfile.mkdtemp(prefix="approved-identity-", dir=root))
        self.token = self.directory.name
        self.network = None
        self.network_requested = False
        self.containers = []
        self.pending_containers = []
        self.identity_volume = None
        self.volume_requested = False
        self.image = json.loads((package / "manifest.json").read_text())["component"]["docker_image_id"]
        self.identity = "approved-" + self.token
        self.state = self.directory / "state"

    def docker(self, *args, **kwargs):
        return command("docker", *args, **kwargs)

    def inspect(self, container):
        return json.loads(self.docker("inspect", container))[0]

    def create(self, role, *args):
        name = self.token + "-" + role
        self.pending_containers.append(name)
        container = self.docker(
            "create", "--pull", "never", "--name", name,
            "--label", "zpr.simulator.identity-lab=" + self.token,
            "--network", self.network, "--read-only", "--cap-drop", "ALL",
            "--security-opt", "no-new-privileges:true", "--pids-limit", "64",
            "--memory", "256m", "--cpus", "1", *args).decode().strip()
        self.containers.append(container)
        self.pending_containers.remove(name)
        config = self.inspect(container)
        host = config["HostConfig"]
        if (config["Image"] != self.image or host["Privileged"] or not host["ReadonlyRootfs"]
                or host["NetworkMode"] != self.network or host["PortBindings"]
                or host["CapDrop"] != ["ALL"]
                or [cap.removeprefix("CAP_") for cap in host["CapAdd"]] != ["NET_ADMIN"]
                or "no-new-privileges:true" not in host["SecurityOpt"]):
            raise RuntimeError("created lab container violates isolation contract")
        if role == "wrong-key" or role.startswith("endpoint"):
            self.check_endpoint_storage(config, persistent=role.startswith("endpoint"))
        self.docker("start", container)
        return container

    def check_endpoint_storage(self, config, *, persistent, identity_volume=None):
        host = config["HostConfig"]
        mounts = config.get("Mounts", [])
        expected_mounts = host.get("Mounts") or []
        if (host["Binds"] or host["VolumesFrom"]
                or host["Tmpfs"] != {"/run/zpr": "rw,noexec,nosuid,size=4m,mode=0700"}):
            raise RuntimeError("endpoint has unexpected host mounts or tmpfs")
        if persistent:
            identity_volume = identity_volume or self.identity_volume
            volume_options = expected_mounts[0].get("VolumeOptions", {}) if len(expected_mounts) == 1 else {}
            if (len(expected_mounts) != 1
                    or expected_mounts[0].get("Type") != "volume"
                    or expected_mounts[0].get("Source") != identity_volume
                    or expected_mounts[0].get("Target") != "/var/lib/zpr-identity"
                    or expected_mounts[0].get("ReadOnly", False)
                    or volume_options.get("NoCopy") is not True
                    or set(volume_options) - {"NoCopy", "DriverConfig"}
                    or volume_options.get("DriverConfig")
                    or len(mounts) != 1
                    or mounts[0].get("Type") != "volume"
                    or mounts[0].get("Name") != identity_volume
                    or mounts[0].get("Destination") != "/var/lib/zpr-identity"
                    or mounts[0].get("RW") is not True):
                raise RuntimeError("endpoint must mount only its owned identity volume")
            self.check_identity_volume(identity_volume)
        elif expected_mounts or mounts:
            raise RuntimeError("wrong-key endpoint must have only tmpfs, no volumes")

    def check_identity_volume(self, identity_volume=None):
        identity_volume = identity_volume or self.identity_volume
        volume = json.loads(self.docker("volume", "inspect", identity_volume))[0]
        if (not re.fullmatch(re.escape(self.token) + r"-identity(?:-[a-z0-9-]+)?",
                             volume.get("Name", ""))
                or volume.get("Driver") != "local" or volume.get("Options")
                or (volume.get("Labels") or {}).get("zpr.simulator.identity-lab") != self.token):
            raise RuntimeError("lab identity volume ownership or storage configuration changed")

    def execute(self, container, *args, **kwargs):
        return self.docker("exec", container, *args, **kwargs)

    def admin(self, visa, *args, write=False):
        key = (self.state / ("bob/client.key" if write else "bob/web-monitor.key")).read_bytes()
        return self.docker(
            "exec", "-i", visa, "sh", "-ec",
            'IFS= read -r VS_API_KEY; export VS_API_KEY; exec /app/bin/vs-admin '
            '--svc-url "https://[fd5a:5052::1]:8182" '
            '--ca-cert /conf/include/admin-tls-cert.pem --format compact "$@"',
            "lab-admin", *args, data=key.rstrip(b"\n") + b"\n")

    def actor(self, visa):
        return json.loads(self.admin(visa, "actors", "--cn", self.identity))

    def plane(self, service, address):
        config = self.state / "conf" / service
        (config / "include").mkdir(mode=0o700, exist_ok=True)
        keys = {
            "node0": ["node0-noise.crt", "node0-noise.key", "node0-private-key.pem"],
            "vs": ["vs-noise.crt", "vs-noise.key", "vs-private-key.pem",
                   "node0-noise-pub.pem", "admin-tls-cert.pem", "admin-tls-key.pem"],
            "web0": ["web0-private-key.pem", "node0-noise-pub.pem"],
        }
        for name in ["auth-ca.crt", *keys[service]]:
            shutil.copyfile(self.state / "include" / name, config / "include" / name)
        if service == "web0":
            entrypoint = [
                "/bin/sh", "-ec", "ip tuntap add name tun9 mode tun multi_queue; "
                "ip link set tun9 mtu 1400; ip addr add fd5a:5052:8888::8/32 dev tun9; "
                "ip link set tun9 up; exec sleep infinity"]
            mounts = []
        else:
            entrypoint = ["/bin/sh", "/entrypoint.sh"]
            mounts = ["--mount", f"type=bind,source={self.package / 'runtime/assets/entrypoints' / ('entrypoint-' + service + '.sh')},target=/entrypoint.sh,readonly"]
        container = self.create(
            service, "--ip", address, "--cap-add", "NET_ADMIN", "--device", "/dev/net/tun",
            "--tmpfs", "/var/run/zpr:rw,noexec,nosuid,size=4m,mode=0700",
            "--tmpfs", "/tmp:rw,nosuid,size=16m",
            "--tmpfs", "/root:rw,noexec,nosuid,size=4m,mode=0700",
            "--tmpfs", "/state:rw,nosuid,size=16m",
            "--tmpfs", "/conf:rw,noexec,nosuid,size=8m,mode=0700",
            "--tmpfs", "/logs:rw,noexec,nosuid,size=16m,mode=0700",
            *mounts, *self.container_process(entrypoint))
        for path in sorted(config.rglob("*")):
            if path.is_symlink():
                raise RuntimeError("service configuration must not contain symlinks")
            relative = path.relative_to(config).as_posix()
            destination = "/conf/" + relative
            if path.is_dir():
                self.execute(container, "mkdir", "-p", destination)
            elif path.is_file():
                self.write_container_file(container, destination, path.read_bytes())
            else:
                raise RuntimeError("service configuration contains a non-file entry: " + str(path))
        return container

    def container_process(self, args):
        return ["--entrypoint", "/usr/bin/timeout", self.image,
                "-k", "2s", "600s", *args]

    def launch(self, container, name, args):
        self.docker("exec", "-d", "-w", "/conf", container, "sh", "-ec",
                    'exec "$@" > /logs/' + name + ".log 2>&1", "launch", *args)

    def sync_service_file(self, container, name):
        source = self.state / "conf/vs" / name
        pending = "/conf/" + name + ".pending"
        target = "/conf/" + name
        self.write_container_file(container, pending, source.read_bytes())
        self.execute(container, "mv", pending, target)

    def write_container_file(self, container, destination, contents):
        self.docker(
            "exec", "-i", container, "sh", "-ec",
            'umask 077; mkdir -p "$(dirname "$1")"; cat > "$1"',
            "stage", destination, data=contents)

    def service_log(self, container, name):
        return self.execute(container, "cat", "/logs/" + name)

    def endpoint(self, role, identity_volume=None):
        persistent = role.startswith("endpoint")
        mounts = []
        key_directory = "/run/zpr/identity"
        if persistent:
            if identity_volume is None:
                self.volume_requested = True
                identity_volume = self.docker(
                    "volume", "create", "--driver", "local", "--label",
                    "zpr.simulator.identity-lab=" + self.token,
                    self.token + "-identity").decode().strip()
            self.identity_volume = identity_volume
            self.check_identity_volume()
            mounts = ["--mount",
                      "type=volume,source=" + identity_volume +
                      ",target=/var/lib/zpr-identity,volume-nocopy"]
            key_directory = "/var/lib/zpr-identity"
        endpoint = self.create(
            role, "--cap-add", "NET_ADMIN", "--device", "/dev/net/tun",
            "--tmpfs", "/run/zpr:rw,noexec,nosuid,size=4m,mode=0700",
            *mounts,
            *self.container_process([
            "/bin/sh", "-ec", "umask 077; mkdir -p " + key_directory + "; "
            "chmod 700 " + key_directory + "; "
            "test ! -L " + key_directory + "/key.pem; "
            "if [ ! -e " + key_directory + "/key.pem ]; then "
            "openssl genrsa -out " + key_directory + "/key.new 2048; "
            "chmod 600 " + key_directory + "/key.new; "
            "mv " + key_directory + "/key.new " + key_directory + "/key.pem; fi; "
            "test -f " + key_directory + "/key.pem; "
            "ln -s " + key_directory + "/key.pem /run/zpr/key.pem; "
            "openssl pkey -in /run/zpr/key.pem -pubout -out /run/zpr/public.pem; "
            "touch /run/zpr/ready; exec sleep infinity"]))
        self.prepare_endpoint(endpoint)
        return endpoint

    def prepare_endpoint(self, endpoint):
        wait_for("fresh endpoint public key", lambda: self.execute(
            endpoint, "sh", "-c", "test ! -e /run/zpr/ready || echo ready").strip())
        self.docker("exec", "-i", endpoint, "sh", "-ec",
                    "umask 077; cat > /run/zpr/ca.crt",
                    data=(self.state / "include/auth-ca.crt").read_bytes())

    def restart_endpoint(self, endpoint, visa, node_ip, public_key):
        config = self.inspect(endpoint)
        if (config["Config"]["Labels"].get("zpr.simulator.identity-lab") != self.token
                or config["Image"] != self.image):
            raise RuntimeError("endpoint ownership or image changed before restart")
        self.check_endpoint_storage(config, persistent=True)
        policy = self.execute(visa, "redis-cli", "-e", "--json", "HGETALL", "policy:current")
        self.docker("stop", "--time", "2", endpoint)
        if self.inspect(endpoint)["State"]["Running"]:
            raise RuntimeError("endpoint did not stop")
        self.docker("start", endpoint)
        self.prepare_endpoint(endpoint)
        if self.execute(endpoint, "cat", "/run/zpr/public.pem") != public_key:
            raise RuntimeError("endpoint identity changed across container restart")
        self.start_adapter(endpoint, node_ip)
        wait_for("restarted adapter link", lambda: b"(Active)" in self.execute(
            endpoint, "sh", "-c",
            'test ! -S /run/zpr/control.sock || /app/bin/ph-cli -p /run/zpr/control.sock link show'))
        actor = self.actor(visa)
        if actor.get("cn") != self.identity or actor.get("node"):
            raise RuntimeError("restarted endpoint was not admitted as its retained identity")
        for _ in range(6):
            self.fetch(endpoint)
            time.sleep(1)
        if policy != self.execute(visa, "redis-cli", "-e", "--json", "HGETALL", "policy:current"):
            raise RuntimeError("lab policy changed across endpoint restart")
        print("PASS: endpoint restart retained its endpoint-owned key; six HTTP samples passed without policy replacement or reapproval",
              flush=True)

    def start_adapter(self, endpoint, node_ip, identity=None):
        self.docker("exec", "-d", endpoint, *self.adapter_process(),
                    "--logging", "all=INFO",
                    "--control-path", "/run/zpr/control.sock", "--capture-path", "/run/zpr/capture.sock",
                    "--ca-file", "/run/zpr/ca.crt", "--bootstrap-key", "/run/zpr/key.pem",
                    "--name", identity or self.identity, "--node-addr", node_ip + ":5000", "--tun-if", "tun0")

    def adapter_process(self):
        return ["/usr/bin/timeout", "-k", "2s", "180s", "/app/bin/ph", "adapter"]

    def fetch(self, endpoint):
        response = self.execute(
            endpoint, "curl", "--noproxy", "*", "-g", "-6", "-fsS",
            "--connect-timeout", "5", "--max-time", "10", "http://[fd5a:5052:8888::8]/")
        if response.strip() != b"approved-lab":
            raise RuntimeError("approved endpoint received unexpected lab HTTP response")

    def authorization_cycle(self, visa, endpoint, web, actor):
        entries = json.loads(self.admin(visa, "actors", "--cn", self.identity, "--visas"))
        flow_ids = set()
        for entry in entries:
            descriptor = json.loads(self.admin(visa, "visas", "--id", str(entry["id"])))
            if (descriptor["source_addr"] == actor["zpr_addr"]
                    and descriptor["dest_addr"] == "fd5a:5052:8888::8"
                    and descriptor["dest_port"] == 80 and descriptor["proto"] == "TCP"):
                flow_ids.add(entry["id"])
        if not flow_ids:
            raise RuntimeError("approved HTTP has no matching live visa to revoke")

        def node_settled(revoked):
            node = json.loads(self.admin(visa, "actors", "--cn", "node0.demo"))
            details = node["node_details"]
            installed = set(details["visas"])
            return (details["in_sync"] and details["pending_install"] == 0
                    and details["pending_revocation"] == 0
                    and (flow_ids.isdisjoint(installed) if revoked else flow_ids <= installed))

        wait_for("approved HTTP visas installed on lab node", lambda: node_settled(False))
        path = self.state / "conf/vs/attrfile.json"
        original = path.read_bytes()
        attributes = json.loads(original)
        attributes["device.zpr.adapter.cn"][self.identity]["access"] = []
        updated = path.with_suffix(".pending")
        updated.write_text(json.dumps(attributes))
        updated.replace(path)
        self.sync_service_file(visa, "attrfile.json")
        offset = len(self.service_log(visa, "vs.log"))
        self.admin(visa, "services", "--id", "attrfile", "--flush", write=True)
        wait_for("HTTP visa revocations acknowledged by lab node", lambda: node_settled(True))
        current = self.actor(visa)
        values = {entry["key"]: entry["value"] for entry in current["attrs"]}
        if current["zpr_addr"] != actor["zpr_addr"] or "cloud" in values.get("user.access", []):
            raise RuntimeError("trusted access withdrawal did not preserve actor and remove cloud access")
        self.fetch(web)
        try:
            self.fetch(endpoint)
        except subprocess.CalledProcessError as exc:
            if exc.returncode != 28:
                raise
        else:
            raise RuntimeError("withdrawn endpoint still fetched protected HTTP")
        evidence = self.service_log(visa, "vs.log")[offset:].decode()
        prefix = "eval denied (no match) for flow " + actor["zpr_addr"] + " -> fd5a:5052:8888::8 "
        if not any(prefix in line and "dest_port: 80 }" in line
                   and "no matching policy" in line for line in evidence.splitlines()):
            raise RuntimeError("withdrawn HTTP has no fresh exact-flow policy denial")
        if not re.search(r"visa sweep reason=AttributeChange[^\n]* revoked=[1-9][0-9]* ", evidence):
            raise RuntimeError("attribute withdrawal has no fresh visa-revocation sweep evidence")
        print("PASS: trusted access withdrawn; exact HTTP visas revoked and acknowledged; fresh HTTP explicitly denied while server remains healthy", flush=True)

        updated.write_bytes(original)
        updated.replace(path)
        self.sync_service_file(visa, "attrfile.json")
        self.admin(visa, "services", "--id", "attrfile", "--flush", write=True)

        def restored():
            try:
                self.fetch(endpoint)
            except subprocess.CalledProcessError as exc:
                if exc.returncode != 28:
                    raise
                return False
            return True

        wait_for("restored trusted access HTTP", restored, attempts=6)
        current = self.actor(visa)
        values = {entry["key"]: entry["value"] for entry in current["attrs"]}
        if current["zpr_addr"] != actor["zpr_addr"] or values.get("user.access") != ["cloud"]:
            raise RuntimeError("restored access changed actor address or failed to restore trusted attribute")
        print("PASS: trusted access restored; same approved actor fetched protected HTTP without policy replacement or service restart", flush=True)

    def prepare(self):
        if not re.fullmatch(r"sha256:[0-9a-f]{64}", self.image):
            raise RuntimeError("invalid pinned ZPR image")
        actual = self.docker("image", "inspect", "--format", "{{.Id}} {{.Os}}/{{.Architecture}}",
                             self.image).decode().strip()
        if actual != self.image + " linux/amd64":
            raise RuntimeError("expected pinned Linux/amd64 ZPR image")
        self.network_requested = True
        self.network = self.docker(
            "network", "create", "--internal", "--label",
            "zpr.simulator.identity-lab=" + self.token, self.token).decode().strip()
        network = json.loads(self.docker("network", "inspect", self.network))[0]
        subnet = ipaddress.ip_network(network["IPAM"]["Config"][0]["Subnet"])
        if subnet.version != 4 or subnet.num_addresses < 32 or not network["Internal"]:
            raise RuntimeError("lab requires an automatically allocated IPv4 subnet")
        node_ip, visa_ip, web_ip = str(subnet[13]), str(subnet[11]), str(subnet[14])
        self.initialize_state()
        node_config = self.state / "conf/node0/node0-conf.toml"
        node_source = node_config.read_text()
        if "[node.peer_noise_certificates]" in node_source:
            node_source = node_source[:node_source.index("[node.peer_noise_certificates]")]
        node_config.write_text(node_source.replace("172.30.0.13", node_ip))
        adapter_config = self.state / "conf/vs/adapter-vs-conf.toml"
        adapter_config.write_text(adapter_config.read_text().replace("172.30.0.13", node_ip))
        web_config = self.state / "conf/web0/adapter-web0-conf.toml"
        web_config.write_text(web_config.read_text().replace("172.30.0.13", node_ip))
        (web_config.parent / "http-response.sh").write_text(
            'while IFS= read -r line; do\n'
            '  case "$line" in "$(printf \'\\r\')"|\'\') break ;; esac\n'
            "done\nprintf 'HTTP/1.0 200 OK\\r\\nConnection: close\\r\\n\\r\\napproved-lab\\n'\n")

        endpoint = self.endpoint("endpoint")
        public_key = self.execute(endpoint, "cat", "/run/zpr/public.pem")
        (self.state / "include/approved-public-key.pem").write_bytes(public_key)
        compiler = self.state / "admin/multinode-demo.zplc"
        source = compiler.read_text()
        if "[nodes.n1]" in source:
            start = source.index("[nodes.n1]")
            end = source.index("[trusted_services.default]")
            source = source[:start] + source[end:]
        source = source.replace("172.30.0.13", node_ip)
        if '"' + self.identity + '"' not in source:
            source = source.replace(
                "[bootstrap]", '[bootstrap]\n"' + self.identity + '" = "../include/approved-public-key.pem"')
        compiler.write_text(source)
        attributes_path = self.state / "conf/vs/attrfile.json"
        attributes = json.loads(attributes_path.read_text())
        attributes["device.zpr.adapter.cn"][self.identity] = {"location": ["cloud"], "access": ["cloud"]}
        attributes_path.write_text(json.dumps(attributes))
        public_include = self.directory / "compiler-public"
        public_include.mkdir(mode=0o700, exist_ok=True)
        for path in (self.state / "include").glob("*-public-key.pem"):
            shutil.copyfile(path, public_include / path.name)
        self.docker(
            "run", "--rm", "--pull", "never", "--network", "none", "--read-only",
            "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
            "--user", f"{os.getuid()}:{os.getgid()}",
            "--mount", f"type=bind,source={self.state / 'admin'},target=/admin",
            "--mount", f"type=bind,source={public_include},target=/include,readonly",
            "--workdir", "/admin", "--entrypoint", "/app/bin/zplc", self.image,
            "--config", "multinode-demo.zplc", "multinode-demo.zpl")
        shutil.copyfile(self.state / "admin/multinode-demo.bin2", self.state / "conf/vs/multinode-demo.bin2")
        visa = self.plane("vs", visa_ip)
        node = self.plane("node0", node_ip)
        web = self.plane("web0", web_ip)
        actor = self.start_runtime(visa, node, web, endpoint, node_ip)
        return visa, node, web, endpoint, node_ip, actor, public_key

    def initialize_state(self):
        environment = dict(os.environ, ZPR_DEMO_PACKAGE_DIR=str(self.package))
        subprocess.run(["bash", str(self.package / "runtime/init-demo-identities.sh"),
                        "prepare", str(self.state)], env=environment, check=True, timeout=180,
                       stdout=sys.stderr)

    def start_runtime(self, visa, node, web, endpoint, node_ip):
        wait_for("lab Redis", lambda: self.execute(
            visa, "sh", "-c", 'test "$(redis-cli --raw PING 2>/dev/null)" != PONG || echo ready').strip())
        self.launch(visa, "vs", ["/app/bin/vs", "multinode-demo.bin2"])
        wait_for("lab policy", lambda: self.execute(
            visa, "sh", "-c", 'test "$(redis-cli --raw TYPE policy:current)" != hash || echo ready').strip())
        self.launch(node, "node0", ["/app/bin/ph", "node", "-c", "node0-conf.toml"])
        self.launch(visa, "vs-adapter", ["/app/bin/ph", "adapter", "-c", "adapter-vs-conf.toml"])
        self.start_adapter(endpoint, node_ip)
        wait_for("approved adapter link", lambda: b"(Active)" in self.execute(
            endpoint, "sh", "-c",
            'test ! -S /run/zpr/control.sock || /app/bin/ph-cli -p /run/zpr/control.sock link show'))
        actor = self.actor(visa)
        if actor.get("cn") != self.identity or actor.get("node"):
            raise RuntimeError("Visa did not report the exact approved adapter: " + json.dumps(actor))
        print("PASS: separate Visa admitted fresh approved bootstrap identity " + self.identity, flush=True)
        print("PASS: endpoint private key generated in its own private volume; only public key supplied to lab compiler", flush=True)
        attributes = {entry["key"]: entry["value"] for entry in actor["attrs"]}
        if attributes.get("user.access") != ["cloud"]:
            raise RuntimeError("approved adapter lacks expected trusted access attribute")
        self.launch(web, "web-adapter", ["env", "ZPR_ADAPTER_SERVICES=OciWeb",
                    "/app/bin/ph", "adapter", "-c", "adapter-web0-conf.toml"])
        self.launch(web, "http", ["/usr/bin/socat",
                    "TCP6-LISTEN:80,bind=[fd5a:5052:8888::8],ipv6only=1,reuseaddr,fork",
                    "EXEC:/bin/sh /conf/http-response.sh"])
        wait_for("lab web adapter", lambda: b"(Active)" in self.execute(
            web, "sh", "-c",
            'test ! -S /var/run/zpr/control.sock || /app/bin/ph-cli -p /var/run/zpr/control.sock link show'))
        self.fetch(endpoint)
        print("PASS: independently approved adapter fetched lab OciWeb through attribute-based policy", flush=True)
        return actor

    def run(self):
        visa, node, web, endpoint, node_ip, actor, public_key = self.prepare()
        policy = self.execute(visa, "redis-cli", "-e", "--json", "HGETALL", "policy:current")
        impostor = self.endpoint("wrong-key")
        if self.execute(impostor, "cat", "/run/zpr/public.pem") == public_key:
            raise RuntimeError("wrong-key test did not generate an independent key")
        offset = len(self.service_log(visa, "vs.log"))
        self.start_adapter(impostor, node_ip)

        def signature_rejected():
            return ("blob signature verification failed for cn " + self.identity).encode() in \
                self.service_log(visa, "vs.log")[offset:]

        wait_for("explicit wrong-key signature rejection", signature_rejected, attempts=30)
        self.fetch(endpoint)
        if self.actor(visa)["zpr_addr"] != actor["zpr_addr"]:
            raise RuntimeError("wrong-key attempt replaced the approved actor")
        if policy != self.execute(visa, "redis-cli", "-e", "--json", "HGETALL", "policy:current"):
            raise RuntimeError("lab policy changed during wrong-key check")
        print("PASS: same identity with a different private key explicitly rejected; approved traffic and actor preserved", flush=True)
        self.authorization_cycle(visa, endpoint, web, actor)
        if policy != self.execute(visa, "redis-cli", "-e", "--json", "HGETALL", "policy:current"):
            raise RuntimeError("lab policy changed during attribute authorization lifecycle")
        self.restart_endpoint(endpoint, visa, node_ip, public_key)

    def cleanup(self):
        failures = []
        containers = list(self.containers)
        for name in self.pending_containers:
            ids = self.docker("ps", "-a", "--no-trunc", "--filter", "name=^/" + name + "$",
                              "--format", "{{.ID}}").decode().split()
            containers.extend(container for container in ids if container not in containers)
        if self.network is None and self.network_requested:
            ids = self.docker("network", "ls", "--filter",
                              "label=zpr.simulator.identity-lab=" + self.token,
                              "--format", "{{.ID}}").decode().split()
            if len(ids) > 1:
                raise RuntimeError("multiple networks claim lab ownership; refusing cleanup")
            self.network = ids[0] if ids else None
        for container in reversed(containers):
            try:
                config = self.inspect(container)
                if config["Config"]["Labels"].get("zpr.simulator.identity-lab") != self.token:
                    raise RuntimeError("lab ownership changed; refusing removal: " + container)
                self.docker("rm", "-f", container)
            except (subprocess.SubprocessError, RuntimeError) as exc:
                failures.append(str(exc))
        if self.identity_volume is None and self.volume_requested:
            ids = self.docker("volume", "ls", "--filter",
                              "label=zpr.simulator.identity-lab=" + self.token,
                              "--format", "{{.Name}}").decode().split()
            if ids != [] and ids != [self.token + "-identity"]:
                raise RuntimeError("unexpected identity volumes claim lab ownership; refusing cleanup")
            self.identity_volume = ids[0] if ids else None
        if self.identity_volume is not None and not failures:
            try:
                self.check_identity_volume()
                self.docker("volume", "rm", self.identity_volume)
            except (subprocess.SubprocessError, RuntimeError) as exc:
                failures.append(str(exc))
        if self.network is not None and not failures:
            try:
                network = json.loads(self.docker("network", "inspect", self.network))[0]
                if network["Labels"].get("zpr.simulator.identity-lab") != self.token:
                    raise RuntimeError("lab network ownership changed; refusing removal")
                self.docker("network", "rm", self.network)
            except (subprocess.SubprocessError, RuntimeError) as exc:
                failures.append(str(exc))
        if failures:
            raise RuntimeError("lab cleanup failed; state retained at " + str(self.directory) + ": " + "; ".join(failures))


def main():
    if sys.argv[1:] != ["--disposable-host"]:
        raise SystemExit("usage: verify-approved-identity-lab.py --disposable-host")
    if os.geteuid() != 0:
        raise SystemExit("requires root on a disposable Linux/amd64 Docker host")
    os.umask(0o077)
    package = Path(os.environ["ZPR_DEMO_PACKAGE_DIR"])
    root = Path(os.environ["ZPR_IDENTITY_LAB_ROOT"])
    for path in (package, root):
        if not path.is_absolute() or path.resolve() != path or path == Path("/"):
            raise SystemExit("package/lab root must be canonical absolute paths without symlinks")
    private_directory(root)
    lab = IdentityLab(package, root)
    def interrupted(signum, frame):
        raise RuntimeError("lab interrupted by signal " + str(signum))
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    completed = False
    try:
        lab.run()
        completed = True
    finally:
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        signal.signal(signal.SIGINT, signal.SIG_IGN)
        lab.cleanup()
        if not completed:
            print("Failed lab evidence retained privately: " + str(lab.directory), flush=True)
    shutil.rmtree(lab.directory)
    print("PASS: isolated lab containers, network, identity volume and generated identity state removed")


if __name__ == "__main__":
    main()
