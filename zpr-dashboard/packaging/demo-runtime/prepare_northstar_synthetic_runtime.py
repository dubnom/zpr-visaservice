#!/usr/bin/env python3
"""Prepare the Northstar-only runtime tree beneath W2's LAB_ROOT."""

from __future__ import annotations

import argparse
import hashlib
import ipaddress
import json
import os
import shutil
import stat
from pathlib import Path, PurePosixPath


RUNTIME_SOURCE = Path(__file__).resolve().parent
VISA_REPOSITORY = Path(__file__).resolve().parents[3]
POLICY_SOURCE = Path("libeval/tests/zpl/northstar-machine-policy-deny-lab")
POLICY_TARGET_ID = "northstar-machine-policy-deny-lab-v4"
POLICY_DIGEST = "9a3e2715a05bd2cd57561bae2aa76b93e1f6b9115aa4e161319240ef288ee783"
POLICY_FILES = {
    ".zpl": "66bb850d27a5f1db57630973f664e2d492c85196bc014acd11a7eb1f3fa0ed01",
    ".zplc": "fd660894a390c3f567c58e9a474238c35edf8385525e83bda9f1e145e1381d26",
    ".bin2": POLICY_DIGEST,
    ".identity": "573b8cf650d94b9a0809dc7993ffe4d6eefdc082cab86f120cd2f0365eb58bff",
}
BOOTSTRAP_PUBLIC_KEY_SOURCE = Path(
    "libeval/tests/zpl/include/finance-workspace-public-key.pem"
)
BOOTSTRAP_PUBLIC_KEY_DIGEST = (
    "27146e028849905c79c647a3ceffd3a7d2ba2af5355d93cb0aa0b531c0a4998e"
)
BOOTSTRAP_PUBLIC_KEY_PATH = "include/finance-workspace-public-key.pem"
POLICY_CONFIG = "northstar/northstar-machine-policy-deny-lab.zplc"
POLICY_MACHINE_ZPR_ADDRESS = "fd00:1:4::1"
SCENARIO_MACHINE_ZPR_ADDRESS = "fd00:5a17:2026:104::1"
PROFILE_RUNTIME_NODE_ID = "northstar-node"
PROFILE_RUNTIME_NODE_ZPR_ADDRESS = "fd5a:5052:90de::10"
EXPECTED_COMPONENTS = [
    {
        "name": "finance-client",
        "kind": "client",
        "agent": "finance-client",
        "target": "fd00:1:2::1",
    }
]
EXPECTED_WORKLOAD = {
    "version": 1,
    "machine_id": "machine-01",
    "name": "finance-client",
    "interface": "tun0",
}
EXPECTED_PROVISIONING = {
    "version": 2,
    "driver": "shared-approved-bootstrap",
    "primary_machine_id": "machine-01",
    "machines": ["machine-01"],
    "service": "OciWeb",
}
TOPOLOGY = {
    "nodes": [
        {
            "id": "machine-01-finance-client",
            "zpr_address": SCENARIO_MACHINE_ZPR_ADDRESS,
        },
        {
            "id": "finance-workspace",
            "zpr_address": "fd00:1:2::1",
        },
    ],
    "links": [
        {
            "id": "finance-to-workspace",
            "from": "machine-01-finance-client",
            "to": "finance-workspace",
        }
    ],
    "components": [
        {
            "name": "finance-client",
            "kind": "client",
            "machine": "machine-01",
            "agent": "finance-client",
            "target": "fd00:1:2::1",
        },
        {
            "name": "FinanceWorkspace",
            "kind": "service",
            "address": "fd00:1:2::1",
        },
    ],
}


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def _require_regular_file(path: Path) -> None:
    if not stat.S_ISREG(path.lstat().st_mode):
        raise ValueError(f"expected a regular file: {path}")


def _copy_regular_file(source: Path, destination: Path) -> None:
    _require_regular_file(source)
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source, destination)


def _safe_relative_path(value: str) -> Path:
    relative = PurePosixPath(value)
    if relative.is_absolute() or not relative.parts or ".." in relative.parts:
        raise ValueError(f"unsafe package asset path: {value}")
    return Path(*relative.parts)


def _read_json(path: Path) -> dict:
    with path.open(encoding="utf-8") as stream:
        value = json.load(stream)
    if not isinstance(value, dict):
        raise ValueError(f"expected a JSON object: {path}")
    return value


def _write_json(path: Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="\n") as stream:
        json.dump(value, stream, indent=2)
        stream.write("\n")


def _parse_identity(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line:
            continue
        key, separator, value = line.partition("=")
        if not separator or not key or key in values:
            raise ValueError(f"invalid policy identity manifest: {path}")
        values[key] = value
    return values


def _northstar_machines(ldif_path: Path) -> list[dict]:
    entries: list[dict[str, list[str]]] = []
    current: dict[str, list[str]] = {}
    for line in ldif_path.read_text(encoding="utf-8").splitlines():
        if not line:
            if current:
                entries.append(current)
                current = {}
            continue
        if line.startswith((" ", "\t")):
            raise ValueError(f"folded LDIF attributes are unsupported: {ldif_path}")
        key, separator, value = line.partition(":")
        if not separator:
            raise ValueError(f"invalid LDIF attribute in {ldif_path}: {line}")
        current.setdefault(key, []).append(value.lstrip(" "))
    if current:
        entries.append(current)

    machines: list[dict] = []
    for entry in entries:
        if "zprMachineId" not in entry:
            continue

        def field(name: str, required: bool = True) -> str:
            values = entry.get(name, [])
            if not values and required:
                raise ValueError(f"Northstar machine LDIF entry is missing {name}")
            return values[0] if values else ""

        machine_id = field("zprMachineId")
        if field("uid") != machine_id:
            raise ValueError(f"Northstar machine uid does not match {machine_id}")
        secure_value = field("zprMachineSecure").lower()
        if secure_value not in {"true", "false"}:
            raise ValueError(f"invalid secure flag for {machine_id}")
        machines.append(
            {
                "id": machine_id,
                "type": field("zprMachineType"),
                "model": field("zprMachineModel"),
                "location": field("zprMachineLocation"),
                "owner": field("zprMachineOwner", required=False),
                "secure": secure_value == "true",
            }
        )

    expected_ids = [f"machine-{number:02d}" for number in range(1, 21)]
    machines.sort(key=lambda machine: machine["id"])
    if [machine["id"] for machine in machines] != expected_ids:
        raise ValueError("Northstar machine LDIF must contain machine-01 through machine-20 exactly once")
    return machines


def _verify_policy_sources() -> dict[str, Path]:
    sources: dict[str, Path] = {}
    for suffix, expected_digest in POLICY_FILES.items():
        source = VISA_REPOSITORY / f"{POLICY_SOURCE}{suffix}"
        _require_regular_file(source)
        actual_digest = _sha256(source)
        if actual_digest != expected_digest:
            raise ValueError(
                f"Northstar candidate {suffix} digest mismatch: "
                f"expected {expected_digest}, found {actual_digest}"
            )
        sources[suffix] = source

    identity = _parse_identity(sources[".identity"])
    expected_identity = {
        "schema_version": "1",
        "organization": "northstar",
        "target_id": POLICY_TARGET_ID,
        "synthetic_lab_only": "true",
        "production_authorized": "false",
        "policy_file": f"{POLICY_SOURCE.name}.bin2",
        "policy_sha256": POLICY_DIGEST,
        "finance_workspace_service": "FinanceWorkspace",
        "finance_workspace_target": "fd00:1:2::1",
        "finance_workspace_bootstrap_path": BOOTSTRAP_PUBLIC_KEY_PATH,
        "finance_workspace_public_key_sha256": BOOTSTRAP_PUBLIC_KEY_DIGEST,
    }
    for key, expected_value in expected_identity.items():
        if identity.get(key) != expected_value:
            raise ValueError(f"Northstar candidate identity has an unexpected {key}")
    public_key = VISA_REPOSITORY / BOOTSTRAP_PUBLIC_KEY_SOURCE
    _require_regular_file(public_key)
    if _sha256(public_key) != BOOTSTRAP_PUBLIC_KEY_DIGEST:
        raise ValueError("Northstar FinanceWorkspace public bootstrap key digest mismatch")
    policy_config = sources[".zplc"].read_text(encoding="utf-8")
    policy_node = (
        '[nodes.machine-01]\n'
        f'zpr_address = "{POLICY_MACHINE_ZPR_ADDRESS}"'
    )
    if policy_node not in policy_config:
        raise ValueError("frozen v4 policy machine address does not match its pinned source")
    sources["finance_workspace_public_key"] = public_key
    return sources


def _validate_runtime_tree(runtime_dir: Path) -> None:
    manifest = _read_json(runtime_dir / "simulation-environment.json")
    machine_ids = [
        machine.get("id")
        for machine in manifest.get("machines", [])
        if isinstance(machine, dict)
    ]
    expected_machine_ids = [f"machine-{number:02d}" for number in range(1, 21)]
    if (
        manifest.get("organization_id") != "northstar"
        or machine_ids != expected_machine_ids
        or manifest.get("components") != EXPECTED_COMPONENTS
    ):
        raise ValueError("Northstar simulation manifest does not match the synthetic v4 contract")

    profile = _read_json(runtime_dir / "organizations/northstar.json")
    runtime = profile.get("runtime")
    runtime_nodes = runtime.get("nodes") if isinstance(runtime, dict) else None
    if (
        profile.get("id") != "northstar"
        or profile.get("synthetic_lab_only") is not True
        or profile.get("active_policy_target_id") != POLICY_TARGET_ID
        or profile.get("active_policy_digest") != POLICY_DIGEST
        or profile.get("policy_config") != POLICY_CONFIG
        or profile.get("synthetic_workload") != EXPECTED_WORKLOAD
        or profile.get("packaged_provisioning") != EXPECTED_PROVISIONING
        or not isinstance(runtime, dict)
        or runtime.get("driver") != "docker-multinode"
        or not isinstance(runtime_nodes, list)
        or len(runtime_nodes) != 1
        or not isinstance(runtime_nodes[0], dict)
        or runtime_nodes[0].get("id") != PROFILE_RUNTIME_NODE_ID
        or runtime_nodes[0].get("zpr_address") != PROFILE_RUNTIME_NODE_ZPR_ADDRESS
    ):
        raise ValueError("Northstar profile does not match the v4 machine-01 synthetic scope")

    scenario = _read_json(runtime_dir / "scenarios/machine-policy-deny.json")
    traffic_steps = [
        step
        for step in scenario.get("steps", [])
        if isinstance(step, dict) and step.get("action") == "traffic"
    ]
    expected_traffic = {
        "action": "traffic",
        "machine": "machine-01",
        "component": "finance-client",
        "target": "fd00:1:2::1",
        "expected": "deny",
    }
    expected_cleanup = [
        {
            "action": "stop_workload",
            "machine": "machine-01",
            "component": "finance-client",
        },
        {"action": "logout", "machine": "machine-01"},
        {"action": "stop_machine", "machine": "machine-01"},
    ]
    if scenario.get("cleanup") != expected_cleanup:
        raise ValueError("Northstar scenario cleanup does not match the machine-01 scope")
    topology = scenario.get("topology")
    topology_nodes = topology.get("nodes") if isinstance(topology, dict) else None
    topology_links = topology.get("links") if isinstance(topology, dict) else None
    topology_components = topology.get("components") if isinstance(topology, dict) else None
    if (
        not isinstance(topology_nodes, list)
        or not isinstance(topology_links, list)
        or not isinstance(topology_components, list)
    ):
        raise ValueError("Northstar scenario must include its finance topology")
    nodes_by_id = {
        node.get("id"): node for node in topology_nodes if isinstance(node, dict)
    }
    client_node = nodes_by_id.get("machine-01-finance-client")
    workspace_node = nodes_by_id.get("finance-workspace")
    if (
        len(topology_nodes) != 2
        or len(nodes_by_id) != 2
        or not isinstance(client_node, dict)
        or not isinstance(workspace_node, dict)
        or workspace_node.get("zpr_address") != "fd00:1:2::1"
        or any("substrate" in node for node in topology_nodes if isinstance(node, dict))
    ):
        raise ValueError("Northstar scenario topology has an unexpected node identity or target")
    client_address = client_node.get("zpr_address")
    workspace_address = workspace_node.get("zpr_address")
    if not isinstance(client_address, str) or not isinstance(workspace_address, str):
        raise ValueError("Northstar scenario topology addresses must be IPv6 strings")
    try:
        ipaddress.IPv6Address(client_address)
        ipaddress.IPv6Address(workspace_address)
    except ipaddress.AddressValueError as error:
        raise ValueError("Northstar scenario topology addresses must be IPv6") from error
    expected_links = [
        {
            "id": "finance-to-workspace",
            "from": "machine-01-finance-client",
            "to": "finance-workspace",
        }
    ]
    expected_topology_components = [
        {
            "name": "finance-client",
            "kind": "client",
            "machine": "machine-01",
            "agent": "finance-client",
            "target": "fd00:1:2::1",
        },
        {
            "name": "FinanceWorkspace",
            "kind": "service",
            "address": "fd00:1:2::1",
        },
    ]
    if (
        scenario.get("id") != "machine-policy-deny"
        or scenario.get("organization_id") != "northstar"
        or topology_links != expected_links
        or topology_components != expected_topology_components
        or "protocol" in scenario
        or traffic_steps != [expected_traffic]
    ):
        raise ValueError("Northstar scenario does not match the machine-01 deny contract")

    organization_dir = runtime_dir / "organizations/northstar"
    identity_path = organization_dir / f"{POLICY_SOURCE.name}.identity"
    identity = _parse_identity(identity_path)
    expected_identity = {
        "organization": "northstar",
        "target_id": POLICY_TARGET_ID,
        "synthetic_lab_only": "true",
        "production_authorized": "false",
        "policy_file": f"{POLICY_SOURCE.name}.bin2",
        "policy_sha256": POLICY_DIGEST,
        "finance_workspace_service": "FinanceWorkspace",
        "finance_workspace_target": "fd00:1:2::1",
        "finance_workspace_public_key_sha256": BOOTSTRAP_PUBLIC_KEY_DIGEST,
    }
    if any(identity.get(key) != value for key, value in expected_identity.items()):
        raise ValueError("Northstar policy identity does not match the frozen v4 synthetic candidate")

    for suffix, expected_digest in POLICY_FILES.items():
        candidate = organization_dir / f"{POLICY_SOURCE.name}{suffix}"
        if _sha256(candidate) != expected_digest:
            raise ValueError(f"Northstar runtime policy {suffix} digest mismatch")
    public_key = organization_dir / BOOTSTRAP_PUBLIC_KEY_PATH
    if _sha256(public_key) != BOOTSTRAP_PUBLIC_KEY_DIGEST:
        raise ValueError("Northstar runtime FinanceWorkspace public key digest mismatch")


def build_runtime_root(lab_root: Path) -> Path:
    destination = lab_root.expanduser().absolute()
    repository = VISA_REPOSITORY.resolve()
    resolved_destination = destination.resolve()
    if resolved_destination != destination:
        raise ValueError("LAB_ROOT must be canonical and contain no symlinked path components")
    if resolved_destination == repository or repository in resolved_destination.parents:
        raise ValueError("LAB_ROOT must be outside the Visa source repository")
    if not destination.parent.is_dir():
        raise ValueError("LAB_ROOT parent must already exist")
    created_lab_root = False
    try:
        destination.mkdir(mode=0o700)
        destination.chmod(0o700)
        created_lab_root = True
    except FileExistsError:
        info = destination.lstat()
        if (
            not stat.S_ISDIR(info.st_mode)
            or info.st_uid != os.getuid()
            or stat.S_IMODE(info.st_mode) != 0o700
        ):
            raise ValueError("LAB_ROOT must be an invoking-user-owned mode-0700 directory")
    runtime_dir = destination / "runtime"
    runtime_dir.mkdir(mode=0o700)
    runtime_dir.chmod(0o700)
    complete = False
    try:
        profile_path = RUNTIME_SOURCE / "organizations/northstar.json"
        profile = _read_json(profile_path)
        if profile.get("id") != "northstar":
            raise ValueError("expected the canonical Northstar organization profile")
        profile_assets = profile.get("directory", {}).get("ldif_files")
        if not isinstance(profile_assets, list) or not all(
            isinstance(asset, str) for asset in profile_assets
        ):
            raise ValueError("Northstar profile has an invalid LDIF asset list")

        organization_source = RUNTIME_SOURCE / "organizations"
        organization_destination = runtime_dir / "organizations"
        for asset in profile_assets:
            relative = _safe_relative_path(asset)
            _copy_regular_file(
                organization_source / "northstar" / relative,
                organization_destination / "northstar" / relative,
            )

        original_policy_config = profile.get("policy_config")
        policy_catalog = profile.get("policy_catalog")
        if not isinstance(original_policy_config, str) or not isinstance(policy_catalog, str):
            raise ValueError("Northstar profile must declare policy_config and policy_catalog")
        for asset in (original_policy_config, policy_catalog):
            relative = _safe_relative_path(asset)
            _copy_regular_file(organization_source / relative, organization_destination / relative)

        policy_sources = _verify_policy_sources()
        candidate_directory = organization_destination / "northstar"
        candidate_directory.mkdir(parents=True, exist_ok=True)
        for suffix, source in policy_sources.items():
            if suffix == "finance_workspace_public_key":
                destination_path = candidate_directory / BOOTSTRAP_PUBLIC_KEY_PATH
            else:
                destination_path = candidate_directory / f"{POLICY_SOURCE.name}{suffix}"
            _copy_regular_file(source, destination_path)

        profile["synthetic_lab_only"] = True
        profile["synthetic_workload"] = {
            "version": 1,
            "machine_id": "machine-01",
            "name": "finance-client",
            "interface": "tun0",
        }
        profile["active_policy_target_id"] = POLICY_TARGET_ID
        profile["active_policy_digest"] = POLICY_DIGEST
        profile["policy_config"] = POLICY_CONFIG
        profile["packaged_provisioning"] = {
            "version": 2,
            "driver": "shared-approved-bootstrap",
            "primary_machine_id": "machine-01",
            "machines": ["machine-01"],
            "service": "OciWeb",
        }
        _write_json(organization_destination / "northstar.json", profile)

        manifest = {
            "name": "Northstar Synthetic Machine-Policy-Deny Lab",
            "organization_id": "northstar",
            "agents": [],
            "services": [],
            "machines": _northstar_machines(
                organization_source / "northstar" / "demo-machines.ldif"
            ),
            "components": [
                {
                    "name": "finance-client",
                    "kind": "client",
                    "agent": "finance-client",
                    "target": "fd00:1:2::1",
                }
            ],
        }
        _write_json(runtime_dir / "simulation-environment.json", manifest)

        scenario = _read_json(RUNTIME_SOURCE / "scenarios/machine-policy-deny.json")
        if scenario.get("id") != "machine-policy-deny" or scenario.get("organization_id") != "northstar":
            raise ValueError("expected the Northstar machine-policy-deny scenario")
        if "topology" in scenario:
            raise ValueError("review the source scenario topology before preparing this fixture")
        scenario["description"] = (
            "Synthetic lab-only denial scenario for machine-01 finance-client to the "
            "FinanceWorkspace target; the target is metadata, not a provisioned service."
        )
        scenario["topology"] = TOPOLOGY
        _write_json(runtime_dir / "scenarios/machine-policy-deny.json", scenario)

        launcher = RUNTIME_SOURCE / "simulator-demo-runtime.sh"
        _require_regular_file(launcher)
        if not launcher.stat().st_mode & stat.S_IXUSR:
            raise ValueError("Simulator runtime launcher must be executable")
        _copy_regular_file(launcher, runtime_dir / "simulator-demo-runtime.sh")
        _validate_runtime_tree(runtime_dir)
        complete = True
        return runtime_dir
    finally:
        if not complete:
            shutil.rmtree(runtime_dir)
            if created_lab_root:
                destination.rmdir()


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--lab-root",
        type=Path,
        required=True,
        help="W2 LAB_ROOT; runtime inputs are written only beneath LAB_ROOT/runtime",
    )
    args = parser.parse_args()
    try:
        result = build_runtime_root(args.lab_root)
    except (OSError, TypeError, ValueError, json.JSONDecodeError) as error:
        parser.error(str(error))
    print(f"Prepared Northstar synthetic runtime tree: {result}")


if __name__ == "__main__":
    main()
