import hashlib
import json
import tempfile
import unittest
from pathlib import Path

from prepare_northstar_synthetic_runtime import (
    BOOTSTRAP_PUBLIC_KEY_DIGEST,
    BOOTSTRAP_PUBLIC_KEY_PATH,
    BOOTSTRAP_PUBLIC_KEY_SOURCE,
    POLICY_DIGEST,
    POLICY_FILES,
    POLICY_MACHINE_ZPR_ADDRESS,
    POLICY_TARGET_ID,
    POLICY_SOURCE,
    PROFILE_RUNTIME_NODE_ID,
    PROFILE_RUNTIME_NODE_ZPR_ADDRESS,
    RUNTIME_SOURCE,
    SCENARIO_MACHINE_ZPR_ADDRESS,
    TOPOLOGY,
    _validate_runtime_tree,
    build_runtime_root,
)


class NorthstarSyntheticRuntimeTest(unittest.TestCase):
    def test_v4_policy_digest_is_pinned(self):
        expected_digest = (
            "9a3e2715a05bd2cd57561bae2aa76b93e1f6b9115aa4e161319240ef288ee783"
        )
        self.assertEqual(len(expected_digest), 64)
        self.assertEqual(POLICY_DIGEST, expected_digest)
        self.assertEqual(POLICY_FILES[".bin2"], expected_digest)

    def test_public_bootstrap_key_digest_is_pinned(self):
        expected_digest = (
            "27146e028849905c79c647a3ceffd3a7d2ba2af5355d93cb0aa0b531c0a4998e"
        )
        self.assertEqual(len(expected_digest), 64)
        self.assertEqual(BOOTSTRAP_PUBLIC_KEY_DIGEST, expected_digest)

        repository = RUNTIME_SOURCE.parents[2]
        public_key = repository / BOOTSTRAP_PUBLIC_KEY_SOURCE
        identity = repository / "libeval/tests/zpl/northstar-machine-policy-deny-lab.identity"
        self.assertEqual(
            hashlib.sha256(public_key.read_bytes()).hexdigest(),
            expected_digest,
        )
        self.assertIn(
            f"finance_workspace_public_key_sha256={expected_digest}",
            identity.read_text(),
        )

    def test_builds_isolated_runtime_root_from_northstar_sources(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve() / "northstar-synthetic"
            root.mkdir(mode=0o700)
            lab_root = root / "lab"
            route_output = root / "node-route"
            route_output.mkdir(parents=True, mode=0o700)
            route_marker = route_output / "route-output.txt"
            route_marker.write_text("owned by W2\n")
            source_profile = (RUNTIME_SOURCE / "organizations/northstar.json").read_bytes()
            source_scenario = (RUNTIME_SOURCE / "scenarios/machine-policy-deny.json").read_bytes()
            demo_paths = (
                RUNTIME_SOURCE / "simulation-environment.json",
                RUNTIME_SOURCE / "organizations/demo.json",
                RUNTIME_SOURCE / "config.toml",
            )
            demo_bytes = {path: path.read_bytes() for path in demo_paths}

            runtime = build_runtime_root(lab_root)

            self.assertEqual(runtime, lab_root / "runtime")
            self.assertEqual(lab_root.stat().st_mode & 0o777, 0o700)
            self.assertEqual(runtime.stat().st_mode & 0o777, 0o700)
            self.assertFalse((lab_root / "simulation-environment.json").exists())
            self.assertFalse((runtime / "runtime").exists())
            self.assertFalse((runtime / "organizations/demo.json").exists())
            self.assertTrue((runtime / "simulator-demo-runtime.sh").stat().st_mode & 0o100)
            manifest = json.loads((runtime / "simulation-environment.json").read_text())
            self.assertEqual(manifest["organization_id"], "northstar")
            self.assertEqual(manifest["agents"], [])
            self.assertEqual(manifest["services"], [])
            self.assertEqual(
                [machine["id"] for machine in manifest["machines"]],
                [f"machine-{number:02d}" for number in range(1, 21)],
            )
            self.assertEqual(manifest["machines"][0]["owner"], "elena.park")
            self.assertEqual(
                manifest["components"],
                [
                    {
                        "name": "finance-client",
                        "kind": "client",
                        "agent": "finance-client",
                        "target": "fd00:1:2::1",
                    }
                ],
            )
            self.assertNotIn("address", manifest["components"][0])
            self.assertNotIn("dns_server", manifest)

            profile = json.loads((runtime / "organizations/northstar.json").read_text())
            self.assertEqual(profile["id"], "northstar")
            self.assertTrue(profile["synthetic_lab_only"])
            self.assertEqual(profile["runtime"]["driver"], "docker-multinode")
            self.assertEqual(len(profile["runtime"]["nodes"]), 1)
            self.assertEqual(profile["runtime"]["nodes"][0]["id"], PROFILE_RUNTIME_NODE_ID)
            self.assertEqual(
                profile["runtime"]["nodes"][0]["zpr_address"],
                PROFILE_RUNTIME_NODE_ZPR_ADDRESS,
            )
            self.assertEqual(PROFILE_RUNTIME_NODE_ID, "northstar-node")
            self.assertEqual(PROFILE_RUNTIME_NODE_ZPR_ADDRESS, "fd5a:5052:90de::10")
            self.assertEqual(
                profile["synthetic_workload"],
                {
                    "version": 1,
                    "machine_id": "machine-01",
                    "name": "finance-client",
                    "interface": "tun0",
                },
            )
            self.assertEqual(profile["active_policy_target_id"], POLICY_TARGET_ID)
            self.assertEqual(profile["active_policy_digest"], POLICY_DIGEST)
            self.assertEqual(profile["policy_config"], "northstar/northstar-machine-policy-deny-lab.zplc")
            self.assertEqual(
                profile["packaged_provisioning"],
                {
                    "version": 2,
                    "driver": "shared-approved-bootstrap",
                    "primary_machine_id": "machine-01",
                    "machines": ["machine-01"],
                    "service": "OciWeb",
                },
            )

            for filename in (
                "demo-company.ldif",
                "demo-employees.ldif",
                "demo-roles.ldif",
                "demo-machines.ldif",
                "demo-policy-catalog.json",
                "policy-demo.zplc",
            ):
                self.assertTrue((runtime / "organizations/northstar" / filename).is_file())

                candidate = runtime / "organizations/northstar" / f"{POLICY_SOURCE.name}.bin2"
            digest = hashlib.sha256(candidate.read_bytes()).hexdigest()
            self.assertEqual(digest, POLICY_DIGEST)
            for suffix, expected_digest in POLICY_FILES.items():
                candidate_file = runtime / "organizations/northstar" / f"{POLICY_SOURCE.name}{suffix}"
                self.assertEqual(
                    hashlib.sha256(candidate_file.read_bytes()).hexdigest(),
                    expected_digest,
                )
            identity = (
                runtime / "organizations/northstar" / f"{POLICY_SOURCE.name}.identity"
            ).read_text()
            self.assertIn("production_authorized=false", identity)
            self.assertIn(f"target_id={POLICY_TARGET_ID}", identity)
            self.assertIn(f"policy_sha256={POLICY_DIGEST}", identity)
            self.assertIn("finance_workspace_service=FinanceWorkspace", identity)
            self.assertIn("finance_workspace_target=fd00:1:2::1", identity)
            self.assertIn(f"finance_workspace_bootstrap_path={BOOTSTRAP_PUBLIC_KEY_PATH}", identity)
            self.assertIn(
                f"finance_workspace_public_key_sha256={BOOTSTRAP_PUBLIC_KEY_DIGEST}",
                identity,
            )
            public_key = runtime / "organizations/northstar" / BOOTSTRAP_PUBLIC_KEY_PATH
            self.assertEqual(
                hashlib.sha256(public_key.read_bytes()).hexdigest(),
                BOOTSTRAP_PUBLIC_KEY_DIGEST,
            )
            self.assertFalse(
                (
                    runtime
                    / "organizations/northstar/include/finance-workspace-private-key.pem"
                ).exists()
            )

            scenario = json.loads((runtime / "scenarios/machine-policy-deny.json").read_text())
            self.assertEqual(scenario["topology"], TOPOLOGY)
            traffic_steps = [
                step for step in scenario["steps"] if step["action"] == "traffic"
            ]
            self.assertEqual(
                traffic_steps,
                [
                    {
                        "action": "traffic",
                        "machine": "machine-01",
                        "component": "finance-client",
                        "target": "fd00:1:2::1",
                        "expected": "deny",
                    }
                ],
            )
            self.assertNotIn("protocol", scenario)
            self.assertNotIn("protocol", traffic_steps[0])
            self.assertEqual(
                scenario["cleanup"],
                [
                    {
                        "action": "stop_workload",
                        "machine": "machine-01",
                        "component": "finance-client",
                    },
                    {"action": "logout", "machine": "machine-01"},
                    {"action": "stop_machine", "machine": "machine-01"},
                ],
            )
            self.assertTrue(
                all("substrate" not in node for node in scenario["topology"]["nodes"])
            )
            finance_client = next(
                component
                for component in scenario["topology"]["components"]
                if component["name"] == "finance-client"
            )
            self.assertNotIn("address", finance_client)
            self.assertEqual(finance_client["target"], "fd00:1:2::1")
            self.assertEqual(
                scenario["topology"]["nodes"],
                [
                    {
                        "id": "machine-01-finance-client",
                        "zpr_address": SCENARIO_MACHINE_ZPR_ADDRESS,
                    },
                    {
                        "id": "finance-workspace",
                        "zpr_address": "fd00:1:2::1",
                    },
                ],
            )
            self.assertEqual(
                SCENARIO_MACHINE_ZPR_ADDRESS,
                "fd00:5a17:2026:104::1",
            )
            self.assertEqual(
                POLICY_MACHINE_ZPR_ADDRESS,
                "fd00:1:4::1",
            )
            self.assertNotEqual(
                SCENARIO_MACHINE_ZPR_ADDRESS,
                POLICY_MACHINE_ZPR_ADDRESS,
            )
            frozen_policy = (
                RUNTIME_SOURCE.parents[2]
                / "libeval/tests/zpl"
                / f"{POLICY_SOURCE.name}.zplc"
            ).read_text()
            self.assertIn(
                f'[nodes.machine-01]\nzpr_address = "{POLICY_MACHINE_ZPR_ADDRESS}"',
                frozen_policy,
            )
            self.assertIn("Synthetic lab-only", scenario["description"])
            self.assertEqual(scenario["organization_id"], "northstar")
            self.assertEqual(scenario["id"], "machine-policy-deny")

            self.assertEqual(
                (runtime / "simulator-demo-runtime.sh").read_bytes(),
                (RUNTIME_SOURCE / "simulator-demo-runtime.sh").read_bytes(),
            )
            self.assertEqual(source_profile, (RUNTIME_SOURCE / "organizations/northstar.json").read_bytes())
            self.assertEqual(
                source_scenario,
                (RUNTIME_SOURCE / "scenarios/machine-policy-deny.json").read_bytes(),
            )
            for path, content in demo_bytes.items():
                self.assertEqual(path.read_bytes(), content)
            self.assertEqual(route_marker.read_text(), "owned by W2\n")
            _validate_runtime_tree(runtime)

    def test_topology_validation_checks_ipv6_without_reconciling_address(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve() / "northstar-synthetic"
            root.mkdir(mode=0o700)
            runtime = build_runtime_root(root / "lab")
            scenario_path = runtime / "scenarios/machine-policy-deny.json"
            scenario = json.loads(scenario_path.read_text())
            client_node = next(
                node
                for node in scenario["topology"]["nodes"]
                if node["id"] == "machine-01-finance-client"
            )
            client_node["zpr_address"] = "fd00:abcd::5"
            scenario_path.write_text(json.dumps(scenario))

            _validate_runtime_tree(runtime)

            client_node["zpr_address"] = "not-an-ipv6-address"
            scenario_path.write_text(json.dumps(scenario))
            with self.assertRaisesRegex(ValueError, "addresses must be IPv6"):
                _validate_runtime_tree(runtime)

    def test_refuses_to_overwrite_existing_output(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve() / "northstar-synthetic"
            root.mkdir(mode=0o700)
            lab_root = root / "lab"
            lab_root.mkdir(mode=0o700)
            output = lab_root / "runtime"
            output.mkdir(mode=0o700)
            marker = output / "keep.txt"
            marker.write_text("preserve")

            with self.assertRaises(FileExistsError):
                build_runtime_root(lab_root)

            self.assertEqual(marker.read_text(), "preserve")

    def test_refuses_lab_root_inside_source_repository(self):
        repository_output = (
            Path(__file__).resolve().parents[3]
            / "packaging"
            / "unsafe-test-output"
        )
        with self.assertRaises(ValueError):
            build_runtime_root(repository_output)
        self.assertFalse(repository_output.exists())

    def test_rejects_demo_only_profile(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve() / "northstar-synthetic"
            root.mkdir(mode=0o700)
            runtime = build_runtime_root(root / "lab")
            profile_path = runtime / "organizations/northstar.json"
            profile = json.loads(profile_path.read_text())
            profile["synthetic_lab_only"] = False
            profile_path.write_text(json.dumps(profile))

            with self.assertRaisesRegex(ValueError, "v4 machine-01 synthetic scope"):
                _validate_runtime_tree(runtime)


if __name__ == "__main__":
    unittest.main()
