#!/usr/bin/env python3
import copy
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import unittest
from unittest.mock import patch

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("control_stack", HERE / "control-stack.py")
stack = importlib.util.module_from_spec(spec)
spec.loader.exec_module(stack)


class OperatorStackTests(unittest.TestCase):
    def setUp(self):
        self.root = HERE.parent / f".operator-stack-test-{os.getpid()}-{self._testMethodName}"
        self.root.mkdir()
        self.addCleanup(shutil.rmtree, self.root)
        self.secret = self.root / "operator-config"
        self.secret.write_text("operator-owned fixture")
        self.config = {"version": 1, "deployment": "company", "services": {}}
        for mode, keys in stack.REQUIRED.items():
            env = {}
            for key in keys:
                if key.endswith("_FILE"):
                    env[key] = "/operator/config"
                elif key.endswith(("_URL", "_ORIGIN")):
                    env[key] = "https://company.example:8790"
                elif key.endswith("_LISTEN"):
                    env[key] = "0.0.0.0:8790"
                else:
                    env[key] = "company"
            self.config["services"][mode] = {
                "image": "company-control@sha256:012345", "container": f"company-{mode}",
                "env": env, "mounts": [{"source": str(self.secret), "target": "/operator/config", "read_only": True}],
                "publish": ["127.0.0.1:8790:8790"],
            }
        self.config["services"]["control-room"]["listen"] = "0.0.0.0:8787"
        self.filename = self.root / "stack.json"

    def load(self, config=None):
        self.filename.write_text(json.dumps(self.config if config is None else config))
        return stack.load_config(self.filename)

    def test_missing_required_config_fails_before_docker(self):
        with patch.object(stack, "docker") as docker:
            with self.assertRaisesRegex(ValueError, "ZPR_OPERATOR_STACK_CONFIG"):
                stack.load_config("")
            for mode, keys in stack.REQUIRED.items():
                for key in keys:
                    with self.subTest(mode=mode, key=key):
                        config = copy.deepcopy(self.config)
                        del config["services"][mode]["env"][key]
                        with self.assertRaisesRegex(ValueError, key):
                            self.load(config)
            docker.assert_not_called()

    def test_poisoned_lab_environment_is_never_forwarded_or_read(self):
        poison = {"SIMULATION_ORGANIZATION_ID": "wrong", "SIMULATION_MANIFEST": str(self.secret),
                  "SIMULATION_ORGANIZATIONS_DIR": str(self.secret), "ACTIVE_ORGANIZATION_FILE": str(self.secret),
                  "SIMULATOR_URL": "://invalid", "ZPR_ADMIN_URL": "http://untrusted.example"}
        calls = []
        def docker(*args, **kwargs):
            calls.append(args)
            if args[:2] == ("container", "inspect"):
                mode = "control-room" if args[-1].endswith("control-room") else "control-service"
                # First inspect sees no container; post-launch inspect verifies ownership/running.
                launched = any(c[:1] == ("run",) and f"company-{mode}" in c for c in calls)
                return subprocess.CompletedProcess(args, 0 if launched else 1,
                    f"company/{mode} true" if launched else "", "" if launched else "No such container")
            return subprocess.CompletedProcess(args, 0, "", "")
        with patch.dict(os.environ, poison), patch.object(stack, "docker", side_effect=docker):
            config = self.load()
            stack.run("start", config)
        launches = [c for c in calls if c[0] == "run"]
        self.assertEqual([c[-1] if "-listen" not in c else c[-3] for c in launches], list(stack.SERVICES))
        self.assertEqual(len(launches), 2)
        for launch in launches:
            forwarded = [launch[i + 1] for i, arg in enumerate(launch) if arg == "--env"]
            self.assertFalse(any("SIMULAT" in value or "untrusted.example" in value or "wrong" in value for value in forwarded))
            self.assertEqual([launch[i + 1] for i, arg in enumerate(launch) if arg == "--mount"],
                             [f"type=bind,source={self.secret},target=/operator/config,readonly"])
            self.assertNotIn("/var/run/docker.sock", " ".join(launch))
            self.assertNotIn("build", launch)

    def test_configuration_rejects_invalid_or_implicit_inputs(self):
        cases = []
        def change(edit):
            config = copy.deepcopy(self.config)
            edit(config)
            cases.append(config)
        change(lambda c: c.update(version=2))
        change(lambda c: c.update(unknown="ignored"))
        change(lambda c: c["services"]["control-room"]["env"].update(ZPR_CONTROL_ROOM_ORGANIZATION_ID="other"))
        change(lambda c: c["services"]["control-service"]["env"].update(ZPR_SIMULATOR_URL="https://forbidden.example"))
        change(lambda c: c["services"]["control-service"]["env"].update(ZPR_ADMIN_URL="http://company.example"))
        change(lambda c: c["services"]["control-service"]["env"].update(ZPR_DNS_TRANSFER_ADDR="dns.company:53"))
        change(lambda c: c["services"]["control-service"].update(mounts=[]))
        change(lambda c: c["services"]["control-service"]["mounts"][0].update(source=str(self.root / "missing")))
        change(lambda c: c["services"]["control-service"]["mounts"][0].update(source="/"))
        change(lambda c: c["services"]["control-service"].update(publish=["0.0.0.0:8790:8790"]))
        change(lambda c: c["services"]["control-service"].update(image=""))
        change(lambda c: c["services"]["control-service"].update(container="company-control-room"))
        for i, config in enumerate(cases):
            with self.subTest(case=i), self.assertRaises(ValueError):
                self.load(config)

    def test_check_does_not_contact_any_services(self):
        with patch.object(stack, "docker") as docker:
            stack.run("check", self.load())
            docker.assert_not_called()

    def test_foreign_container_or_unavailable_docker_prevents_all_mutations(self):
        for result in [subprocess.CompletedProcess([], 0, "other/control-service true", ""),
                       subprocess.CompletedProcess([], 0, "<no value>/<no value> true", ""),
                       subprocess.CompletedProcess([], 1, "", "Cannot connect to daemon")]:
            with self.subTest(result=result), patch.object(stack, "docker", return_value=result) as docker:
                with self.assertRaises(ValueError):
                    stack.run("restart", self.load())
                self.assertTrue(all(c.args[:2] == ("container", "inspect") for c in docker.call_args_list))

    def test_startup_fails_if_launched_service_exits(self):
        with patch.object(stack, "existing", side_effect=[None, None, False]), patch.object(stack, "docker") as docker:
            with self.assertRaisesRegex(ValueError, "did not remain running"):
                stack.run("start", self.load())
            self.assertEqual(len(docker.call_args_list), 1)
            self.assertIn("company-control-service", docker.call_args.args)
            self.assertNotIn("company-control-room", docker.call_args.args)

    def test_restart_and_stop_manage_only_explicit_owned_targets(self):
        config = self.load()
        states = {mode: True for mode in stack.SERVICES}
        def docker(*args, **kwargs):
            mode = "control-room" if args[-1].endswith("control-room") else "control-service"
            if args[:2] == ("container", "inspect"):
                return subprocess.CompletedProcess(args, 0 if states[mode] else 1,
                    f"company/{mode} true" if states[mode] else "", "" if states[mode] else "No such container")
            if args[0] == "rm":
                states[mode] = False
            if args[0] == "run":
                states["control-room" if "company-control-room" in args else "control-service"] = True
            return subprocess.CompletedProcess(args, 0, "", "")
        with patch.object(stack, "docker", side_effect=docker) as mock:
            stack.run("restart-control-room", config)
            self.assertEqual(sum(c.args[0] == "run" for c in mock.call_args_list), 1)
            self.assertTrue(all("company-control-service" not in c.args for c in mock.call_args_list))
            mock.reset_mock()
            stack.run("stop", config)
            removals = [c.args[-1] for c in mock.call_args_list if c.args[0] == "rm"]
            self.assertEqual(removals, ["company-control-room", "company-control-service"])

    def test_dashboard_default_dispatch_fails_before_lab_path_setup(self):
        env = {k: v for k, v in os.environ.items() if k != "ZPR_OPERATOR_STACK_CONFIG"}
        env.update(SIMULATION_MANIFEST="/nonexistent/invalid", ACTIVE_ORGANIZATION_FILE="/nonexistent/invalid")
        for command in ([], ["start"], ["restart"], ["restart-control-room"], ["restart-control-service"]):
            result = subprocess.run(["sh", str(HERE / "dashboard-stack.sh"), *command], env=env, text=True, capture_output=True)
            self.assertEqual(result.returncode, 1, result.stderr)
            self.assertIn("ZPR_OPERATOR_STACK_CONFIG", result.stderr)
            self.assertNotIn(".local-runtime", result.stderr)

    def test_simulator_lifecycle_cannot_manage_operator_owned_containers(self):
        adapter = (HERE / "simulator-control-adapter.sh").read_text()
        script = (HERE / "dashboard-stack.sh").read_text()
        import re
        stops = "\n".join(re.search(r"^" + name + r"\(\) \{\n.*?^\}", script, re.M | re.S).group(0)
                            for name in ("stop_control_room", "stop_control_service"))
        for function in ("simulator_start_control_room", "simulator_start_control_service", "stop_control_room", "stop_control_service"):
            harness = r'''
set -eu
CONTROL_ROOM_DOCKER_CONTAINER=operator-room
CONTROL_CONTAINER=operator-service
docker() {
    if [ "$1" = inspect ]; then printf company; else echo "unexpected mutation" >&2; exit 99; fi
}
'''
            result = subprocess.run(["sh", "-c", harness + adapter + "\n" + stops + "\n" + function], text=True, capture_output=True)
            self.assertEqual(result.returncode, 1, result.stderr)
            self.assertIn("operator-owned container", result.stderr)
            self.assertNotIn("unexpected mutation", result.stderr)

    def test_bare_policy_lifecycle_never_selects_a_lab_profile(self):
        for operation in ("start", "restart", "stop"):
            result = subprocess.run(["sh", str(HERE / "dashboard-stack.sh"), operation + "-policy-service"], text=True, capture_output=True)
            self.assertEqual(result.returncode, 2)
            self.assertIn("independent operator deployment", result.stderr)

    def test_simulator_adapter_preserves_lab_owned_configuration(self):
        adapter = (HERE / "simulator-control-adapter.sh").read_text()
        script = (HERE / "dashboard-stack.sh").read_text()
        activation = (HERE / "activate-organization.sh").read_text()
        self.assertIn('control_gateway_organization=${ZPR_GATEWAY_ORGANIZATION_ID:-${SIMULATION_ORGANIZATION_ID:-}}', adapter)
        self.assertIn('control_organization_profile="$ORGANIZATIONS_DIR/$control_gateway_organization.json"', adapter)
        self.assertIn('simulator-start) start_stack', script)
        self.assertIn('simulator-restart-control-service)', script)
        self.assertNotIn('dashboard-stack.sh" restart-control-service', activation)
        self.assertLess(script.index('exec python3 "$SCRIPT_DIR/control-stack.py"'), script.index('. "$SCRIPT_DIR/simulator-control-adapter.sh"'))


if __name__ == "__main__":
    unittest.main()
