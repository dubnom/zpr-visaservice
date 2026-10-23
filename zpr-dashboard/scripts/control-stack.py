#!/usr/bin/env python3
"""Launch only explicitly configured operator services; never provision providers."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import urlsplit

REQUIRED = {
    "control-service": (
        "ZPR_CONTROL_SERVICE_LISTEN", "ZPR_GATEWAY_ORGANIZATION_ID",
        "ZPR_CONTROL_SERVICE_CERT_FILE", "ZPR_CONTROL_SERVICE_KEY_FILE",
        "ZPR_CONTROL_SERVICE_CLIENT_CA_FILE", "ZPR_ADMIN_URL",
        "ZPR_ADMIN_CA_FILE", "ZPR_ADMIN_KEY_FILE", "ZPR_POLICY_SERVICE_URL",
        "ZPR_POLICY_SERVICE_CA_FILE", "ZPR_POLICY_CLIENT_CERT_FILE",
        "ZPR_POLICY_CLIENT_KEY_FILE",
    ),
    "control-room": (
        "ZPR_CONTROL_SERVICE_URL", "ZPR_CONTROL_SERVICE_CA_FILE",
        "ZPR_CONTROL_CLIENT_CERT_FILE", "ZPR_CONTROL_CLIENT_KEY_FILE",
        "ZPR_CONTROL_ROOM_ORGANIZATION_ID", "ZPR_CONTROL_ROOM_ORIGIN",
        "ZPR_CONTROL_ROOM_CERT_FILE", "ZPR_CONTROL_ROOM_KEY_FILE",
        "ZPR_OPERATOR_OIDC_CONFIG_FILE", "ZPR_OPERATOR_OIDC_SECRET_FILE",
        "ZPR_OPERATOR_DELEGATION_SIGNER_FILE", "ZPR_OPERATOR_DELEGATION_KEY_FILE",
    ),
}
SERVICES = tuple(REQUIRED)


def require(condition, message):
    if not condition:
        raise ValueError(message)


def fields(value, required, optional=()):
    require(isinstance(value, dict), "configuration sections must be objects")
    require(set(required) <= value.keys() <= set(required) | set(optional),
            "configuration has missing or unknown fields")


def text(value):
    return isinstance(value, str) and bool(value.strip()) and not any(c in value for c in "\r\n\x00")


def load_config(filename):
    require(bool(filename), "set ZPR_OPERATOR_STACK_CONFIG to an explicit operator deployment JSON file")
    with open(filename, encoding="utf-8") as source:
        config = json.load(source)
    fields(config, ("version", "deployment", "services"))
    require(type(config["version"]) is int and config["version"] == 1, "configuration version must be 1")
    require(isinstance(config["deployment"], str) and re.fullmatch(r"[a-z0-9][a-z0-9-]{0,62}", config["deployment"]),
            "deployment must be a lowercase operator deployment identifier")
    fields(config["services"], SERVICES)
    names = set()
    for mode, service in config["services"].items():
        fields(service, ("image", "container", "env", "mounts", "publish"), ("networks", "listen"))
        require(text(service["image"]) and not service["image"].startswith("-"), f"{mode}: image is required")
        name = service["container"]
        require(isinstance(name, str) and re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9_.-]+", name) and name not in names,
                f"{mode}: container names must be explicit and distinct")
        names.add(name)
        env = service["env"]
        require(isinstance(env, dict), f"{mode}: env must be an object")
        for key, value in env.items():
            require(re.fullmatch(r"ZPR_[A-Z0-9_]+", key) and not re.search(r"SIMULAT|DEMO", key) and text(value),
                    f"{mode}: invalid operator environment setting {key}")
        for key in REQUIRED[mode]:
            require(text(env.get(key)), f"{mode}: required production setting {key} is missing")
        for key in ("ZPR_ADMIN_URL", "ZPR_POLICY_SERVICE_URL", "ZPR_CONTROL_SERVICE_URL", "ZPR_CONTROL_ROOM_ORIGIN"):
            if key in env:
                url = urlsplit(env[key])
                require(url.scheme == "https" and url.hostname and not url.username and not url.password
                        and not url.path and not url.query and not url.fragment, f"{mode}: {key} must be an HTTPS origin")
        require(bool(env.get("ZPR_DNS_TRANSFER_ADDR")) == bool(env.get("ZPR_DNS_TRANSFER_TSIG_KEY_FILE")),
                f"{mode}: DNS transfer requires both an explicit address and TSIG key file")
        mounts = service["mounts"]
        require(isinstance(mounts, list), f"{mode}: mounts must be an explicit list")
        targets = set()
        for mount in mounts:
            fields(mount, ("source", "target", "read_only"))
            require(text(mount["source"]) and text(mount["target"]), f"{mode}: invalid mount path")
            source, target = Path(mount["source"]), Path(mount["target"])
            require(source.is_absolute() and source.exists() and not source.is_symlink()
                    and (source.is_file() or source.is_dir()) and str(source) != "/",
                    f"{mode}: mount source must be an existing absolute regular file or directory")
            require(target.is_absolute() and ".." not in target.parts and str(target) != "/"
                    and str(target) not in targets and "," not in str(source) + str(target), f"{mode}: invalid or duplicate mount target")
            require(type(mount["read_only"]) is bool, f"{mode}: read_only must be a boolean")
            targets.add(str(target))
        for key, value in env.items():
            if key.endswith(("_FILE", "_DIR")):
                path = Path(value)
                candidates = [m for m in mounts if path == Path(m["target"]) or Path(m["target"]) in path.parents]
                require(path.is_absolute() and candidates, f"{mode}: {key} requires an explicit mount")
                mount = max(candidates, key=lambda m: len(m["target"]))
                local = Path(mount["source"]) / path.relative_to(mount["target"])
                # Store/database files may be created inside a named writable directory.
                require(local.exists() or (not mount["read_only"] and local.parent.is_dir()),
                        f"{mode}: configured path for {key} does not exist")
        require(isinstance(service["publish"], list) and service["publish"]
                and all(text(p) and re.fullmatch(r"127\.0\.0\.1:[0-9]+:[0-9]+", p) for p in service["publish"]),
                f"{mode}: publish must explicitly bind host loopback ports")
        require(isinstance(service.get("networks", []), list)
                and all(text(n) and re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9_.-]*", n) for n in service.get("networks", [])),
                f"{mode}: invalid network list")
        if mode == "control-room":
            require(text(service.get("listen")), "control-room: explicit listen address is required")
    envs = {mode: service["env"] for mode, service in config["services"].items()}
    require(envs["control-room"]["ZPR_CONTROL_ROOM_ORGANIZATION_ID"] == envs["control-service"]["ZPR_GATEWAY_ORGANIZATION_ID"],
            "Control Room and Control-Service organization IDs must match")
    return config


def docker(*args, check=True):
    return subprocess.run(["docker", *args], check=check, text=True, capture_output=True)


def existing(config, mode):
    service = config["services"][mode]
    result = docker("container", "inspect", "--format",
                    '{{index .Config.Labels "zpr.operator-deployment"}}/{{index .Config.Labels "zpr.operator-service"}} {{.State.Running}}',
                    service["container"], check=False)
    if result.returncode:
        require("No such" in result.stderr, f"cannot inspect {mode}; check Docker availability")
        return None
    owner, _, running = result.stdout.strip().rpartition(" ")
    require(bool(owner) and running in ("true", "false"), f"cannot read {mode} ownership/running state")
    require(owner == f'{config["deployment"]}/{mode}', f"refusing to manage {mode}: container belongs to another deployment")
    return running == "true"


def start_service(config, mode, running):
    if running is True:
        return
    service = config["services"][mode]
    if running is False:
        docker("rm", service["container"])
    args = ["run", "-d", "--name", service["container"], "--restart", "unless-stopped",
            "--label", f'zpr.operator-deployment={config["deployment"]}', "--label", f"zpr.operator-service={mode}"]
    for port in service["publish"]:
        args += ["--publish", port]
    for mount in service["mounts"]:
        args += ["--mount", f'type=bind,source={mount["source"]},target={mount["target"]}' + (",readonly" if mount["read_only"] else "")]
    for key, value in sorted(service["env"].items()):
        args += ["--env", f"{key}={value}"]
    networks = service.get("networks", [])
    for network in networks:
        args += ["--network", network]
    args += ["--entrypoint", "/usr/local/bin/zpr-web-dashboard", service["image"], "-mode", mode]
    if mode == "control-room":
        args += ["-listen", service["listen"]]
    docker(*args)
    require(existing(config, mode) is True, f"{mode} did not remain running; inspect its operator service logs")


def run(command, config):
    if command == "check":
        print("Operator deployment configuration is valid (providers and Docker not contacted).")
        return
    operation, _, target = command.partition("-control-")
    modes = ["control-" + target] if target else list(SERVICES)
    require(operation in ("start", "restart", "stop", "status") and all(m in SERVICES for m in modes), "unknown operator command")
    # Preflight every target before stopping or replacing any owned container.
    states = {mode: existing(config, mode) for mode in modes}
    if operation == "status":
        for mode, state in states.items():
            print(f'{mode}: {"absent" if state is None else "running" if state else "stopped"}')
        return
    if operation in ("stop", "restart"):
        for mode in reversed(modes):
            if states[mode] is not None:
                docker("rm", "-f", config["services"][mode]["container"])
            states[mode] = None
    if operation != "stop":
        for mode in modes:
            start_service(config, mode, states[mode])


def main():
    try:
        require(len(sys.argv) == 2, "usage: control-stack.py start|restart|stop|status|check|<operation>-control-room|<operation>-control-service")
        run(sys.argv[1], load_config(os.environ.get("ZPR_OPERATOR_STACK_CONFIG", "")))
        return 0
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        # Never echo Docker arguments or stderr: explicitly configured secrets can occur there.
        message = "Docker operation failed; inspect operator service logs" if isinstance(error, subprocess.CalledProcessError) else str(error)
        print(f"operator startup: {message}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
