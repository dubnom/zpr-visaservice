#!/usr/bin/env python3
"""Opt-in, isolated B1 startup/restart check. No company LDAP or Simulator use."""
import argparse
import base64
import json
import os
from pathlib import Path
import shutil
import signal
import socket
import ssl
import subprocess
import time
import urllib.error
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[2]


def private_write(path, text):
    path.write_text(text)
    path.chmod(0o600)


def ports(count):
    sockets = [socket.socket() for _ in range(count)]
    try:
        for item in sockets:
            item.bind(("127.0.0.1", 0))
        return [item.getsockname()[1] for item in sockets]
    finally:
        for item in sockets:
            item.close()


def request(port, path, context, authorization=None):
    req = urllib.request.Request(f"https://localhost:{port}{path}")
    if authorization:
        req.add_header("Authorization", authorization)
    opener = urllib.request.build_opener(
        urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=context)
    )
    try:
        with opener.open(req, timeout=3) as response:
            return response.status
    except urllib.error.HTTPError as error:
        return error.code


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-disposable", action="store_true")
    args = parser.parse_args()
    if not args.run_disposable:
        print("SKIP: pass --run-disposable to start uniquely named disposable containers")
        return
    os.umask(0o077)
    parent = ROOT / ".evaluation-test"
    parent.mkdir(mode=0o700, exist_ok=True)
    work = parent / uuid.uuid4().hex
    work.mkdir(mode=0o700)
    state = work / "state"
    binary = work / "bootstrap"
    environment = dict(os.environ, TMPDIR=str(work))
    environment["ZPR_SIMULATOR_URL"] = "http://127.0.0.1:1/unavailable"
    environment["ZPR_CONTROL_SERVICE_URL"] = "https://invalid.example.test:1"
    environment["COMPOSE_PROJECT_NAME"] = "must-not-touch-shared-stack"
    room, service, directory, logging, oidc = ports(5)
    command = [str(binary), "-evaluation", "-state", str(state),
               "-room-port", str(room), "-service-port", str(service),
               "-ldap-port", str(directory), "-logging-port", str(logging),
               "-oidc-port", str(oidc)]
    process = None
    try:
        subprocess.run(["go", "build", "-o", str(binary), "./cmd/zpr-lab-bootstrap"],
                       cwd=ROOT, env=environment, check=True)
        inventory = None
        for cycle in range(2):
            output = work / f"bootstrap-{cycle}.log"
            private_write(output, "")
            with output.open("w") as stream:
                process = subprocess.Popen(command, cwd=ROOT, env=environment,
                                           stdout=stream, stderr=subprocess.STDOUT)
                deadline = time.monotonic() + 180
                while "READY (B1 transport only)" not in output.read_text():
                    if process.poll() is not None or time.monotonic() > deadline:
                        raise RuntimeError("startup not ready: " + output.read_text()[-1800:])
                    time.sleep(0.3)
                current = json.loads((state / "evaluation.json").read_text())
                if inventory is not None:
                    assert current == inventory, "install identity/secrets/config changed"
                    assert (state / "ldap-state/b1-persistence-marker").read_text() == "retained"
                    assert (state / "logging-state/b1-persistence-marker").read_text() == "retained"
                inventory = current
                context = ssl.create_default_context(cafile=str(state / "ca.pem"))
                assert request(room, "/", context) == 200
                assert request(logging, "/api/default/streams", context) in (401, 403)
                password = (state / "logging-admin.secret").read_text().strip()
                basic = base64.b64encode(f"admin@evaluation.invalid:{password}".encode()).decode()
                assert request(logging, "/api/default/streams", context, "Basic " + basic) == 200
                try:
                    request(service, "/api/trusted-sources/change-feeds", context)
                except (urllib.error.URLError, ssl.SSLError):
                    pass
                else:
                    raise AssertionError("Control-Service accepted a client without mTLS identity")
                try:
                    request(room, "/", ssl.create_default_context())
                except (urllib.error.URLError, ssl.SSLError):
                    pass
                else:
                    raise AssertionError("untrusted CA accepted")
                for subtree in ("ldap-state", "logging-state"):
                    private_write(state / subtree / "b1-persistence-marker", "retained")
                process.send_signal(signal.SIGINT)
                assert process.wait(timeout=45) == 0, "unclean supervisor shutdown"
                process = None
            subprocess.run(command + ["-render-only"], cwd=ROOT, env=environment,
                           check=True, stdout=subprocess.DEVNULL)
        secrets = [(state / name).read_bytes().strip() for name in inventory["files_sha256"]
                   if name.endswith(".secret")]
        for log in list(work.glob("bootstrap-*.log")) + list((state / "logs").glob("*.log")):
            content = log.read_bytes()
            assert not any(secret in content for secret in secrets), "secret appeared in logs"
        containers = subprocess.check_output([
            "docker", "ps", "-aq", "--filter",
            "label=com.docker.compose.project=zpr-evaluation-" + inventory["install_id"]
        ], text=True)
        assert not containers.strip(), "disposable stack containers were not cleaned up"
        print("PASS: two independent starts, retained identity/data, verified TLS/mTLS, "
              "admin-only logging, invalid Simulator/inherited config, no secret output, cleanup")
    finally:
        if process is not None and process.poll() is None:
            process.send_signal(signal.SIGINT)
            try:
                process.wait(timeout=45)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=10)
        if (state / "compose.json").exists():
            subprocess.run(["docker", "compose", "-p", "zpr-evaluation-" +
                            json.loads((state / "evaluation.json").read_text())["install_id"],
                            "-f", str(state / "compose.json"), "down", "--timeout", "5"], cwd=ROOT,
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=40)
        shutil.rmtree(work)


if __name__ == "__main__":
    main()
