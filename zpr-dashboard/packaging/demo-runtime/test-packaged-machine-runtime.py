import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch

spec = importlib.util.spec_from_file_location(
    "packaged_runtime", Path(__file__).with_name("packaged-machine-runtime.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

EXAMPLE_ORGANIZATIONS = (
    Path(__file__).resolve().parents[2]
    / "cmd/zpr-web-dashboard/examples/organizations"
)
PACKAGED_DEMO_PROFILE = (
    Path(__file__).resolve().with_name("organizations") / "demo.json"
)


class PackagedMachineTests(unittest.TestCase):
    def runtime(self, directory):
        return module.PackagedMachine(Path(directory), Path(directory), "sha256:" + "a" * 64,
                                      "demo", "demo-machine-01")

    def northstar_profile_fixture(self, root):
        organizations = root / "organizations"
        organizations.mkdir()
        profile = {
            "id": "northstar",
            "synthetic_lab_only": True,
            "synthetic_workload": {
                "version": 1,
                "machine_id": "machine-01",
                "name": "finance-client",
                "interface": "tun0",
            },
            "packaged_provisioning": {
                "version": 2,
                "driver": "shared-approved-bootstrap",
                "primary_machine_id": "machine-01",
                "machines": ["machine-01"],
                "service": "OciWeb",
            },
        }
        (organizations / "northstar.json").write_text(json.dumps(profile))
        manifest = root / "simulation.json"
        manifest.write_text(json.dumps({
            "organization_id": "northstar",
            "machines": [{"id": f"machine-{number:02d}"} for number in range(1, 21)],
        }))
        package = root / "package"
        package.mkdir()
        (package / "manifest.json").write_text(json.dumps({
            "schema": "zpr.demo.runtime-package.v2",
            "package_id": "zpr-demo-image",
            "platform": "linux/amd64",
            "release_id": "test-release",
            "component": {"docker_image_id": "sha256:" + "a" * 64},
        }))
        state = root / "state"
        state.mkdir(mode=0o700)
        environment = {
            "SIMULATION_ORGANIZATIONS_DIR": str(organizations),
            "SIMULATION_MANIFEST": str(manifest),
            "SIMULATION_PROVISION_PACKAGE": str(package),
            "SIMULATION_PROVISION_ROOT": str(state),
            "SIMULATION_PROVISION_RELEASE_ID": "test-release",
        }
        return profile, environment

    def test_northstar_configuration_accepts_only_exact_single_machine_workload(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            profile, environment = self.northstar_profile_fixture(root)
            with patch.dict(module.os.environ, environment):
                package, state, image, provisioning = module.configuration(
                    "northstar", "machine-01")
            self.assertEqual(package, root / "package")
            self.assertEqual(state, root / "state")
            self.assertEqual(image, "sha256:" + "a" * 64)
            self.assertEqual(provisioning["synthetic_workload"], module.NORTHSTAR_WORKLOAD)
            self.assertEqual(provisioning["machines"], ["machine-01"])

            profile["synthetic_workload"]["interface"] = "tun1"
            (root / "organizations/northstar.json").write_text(json.dumps(profile))
            with patch.dict(module.os.environ, environment):
                with self.assertRaisesRegex(RuntimeError, "unsupported Northstar synthetic"):
                    module.configuration("northstar", "machine-01")

    def test_northstar_configuration_rejects_secondary_or_non_synthetic_members(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            profile, environment = self.northstar_profile_fixture(root)
            with patch.dict(module.os.environ, environment):
                with self.assertRaisesRegex(RuntimeError, "unsupported Northstar synthetic"):
                    module.configuration("northstar", "machine-02")
            profile["synthetic_lab_only"] = False
            (root / "organizations/northstar.json").write_text(json.dumps(profile))
            with patch.dict(module.os.environ, environment):
                with self.assertRaisesRegex(RuntimeError, "unsupported Northstar synthetic"):
                    module.configuration("northstar", "machine-01")

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            profile, environment = self.northstar_profile_fixture(root)
            del profile["synthetic_workload"]
            (root / "organizations/northstar.json").write_text(json.dumps(profile))
            with patch.dict(module.os.environ, environment):
                with self.assertRaisesRegex(RuntimeError, "unsupported Northstar synthetic"):
                    module.configuration("northstar", "machine-01")

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            _, environment = self.northstar_profile_fixture(root)
            manifest = json.loads((root / "simulation.json").read_text())
            manifest["machines"][1]["id"] = "machine-21"
            (root / "simulation.json").write_text(json.dumps(manifest))
            with patch.dict(module.os.environ, environment):
                with self.assertRaisesRegex(RuntimeError, "unsupported Northstar synthetic"):
                    module.configuration("northstar", "machine-01")

    def test_northstar_runtime_requires_private_existing_trust_and_workload_assets(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            certs = root / "machine-01"
            workloads = root / "workloads"
            certs.mkdir(mode=0o700)
            workloads.mkdir(mode=0o700)
            for name in ("control-ca.crt", "client.crt", "client.key"):
                path = certs / name
                path.write_bytes(b"fixture")
                path.chmod(0o600)
            for name in ("ca.crt", "client-finance-rsa.key",
                         "machine-controller-machine-01.key"):
                path = workloads / name
                path.write_bytes(b"fixture")
                path.chmod(0o600)
            environment = {
                "SIMULATION_NORTHSTAR_MACHINE_IMAGE": "sha256:" + "b" * 64,
                "SIMULATION_NORTHSTAR_DOCKER_NETWORK": "zpr-northstar-synthetic",
                "SIMULATION_NORTHSTAR_NODE_ADDRESS": "10.20.0.2:5000",
                "SIMULATION_NORTHSTAR_CONTROL_URL": "https://[fd00:1::2]:8792",
                "SIMULATION_RUNTIME_GENERATION": "synthetic-generation-1",
                "SIMULATION_NORTHSTAR_MACHINE_CERT_DIR": str(certs),
                "SIMULATION_NORTHSTAR_WORKLOAD_DIR": str(workloads),
            }
            runtime = module.PackagedMachine(
                root, root, "sha256:" + "a" * 64, "northstar", "machine-01", {
                    "machines": ["machine-01"],
                    "primary_machine_id": "machine-01",
                    "synthetic_workload": module.NORTHSTAR_WORKLOAD,
                })
            with patch.dict(module.os.environ, environment):
                settings = runtime.synthetic_settings()
            self.assertEqual(settings["node_address"], "10.20.0.2:5000")
            self.assertEqual(settings["workload_dir"], workloads)

            (workloads / "client-finance-rsa.key").chmod(0o644)
            with patch.dict(module.os.environ, environment):
                with self.assertRaisesRegex(RuntimeError, "unsafe type or permissions"):
                    runtime.synthetic_settings()

    def test_northstar_machine_args_are_single_endpoint_and_never_start_web(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            runtime = module.PackagedMachine(
                root, root, "sha256:" + "a" * 64, "northstar", "machine-01", {
                    "machines": ["machine-01"],
                    "primary_machine_id": "machine-01",
                    "release_id": "test-release",
                    "synthetic_workload": module.NORTHSTAR_WORKLOAD,
                })
            settings = {
                "image": "sha256:" + "b" * 64,
                "network": "zpr-northstar-synthetic",
                "node_address": "10.20.0.2:5000",
                "control_url": "https://[fd00:1::2]:8792",
                "generation": "synthetic-generation-1",
                "machine_cert_dir": root / "machine-01",
                "workload_dir": root / "workloads",
            }
            args = runtime.synthetic_container_args(settings)
            self.assertEqual(args[0], "create")
            self.assertEqual(args.count("create"), 1)
            self.assertIn("--network", args)
            self.assertEqual(args[args.index("--network") + 1], settings["network"])
            self.assertIn("zpr.simulator.packaged-workload=northstar-finance-client-v1", args)
            self.assertNotIn("web0", args)
            self.assertNotIn("OciWeb", args)
            self.assertEqual(args[-1], settings["image"])
            self.assertEqual(sum(item.startswith("type=bind,") for item in args), 4)

    def test_northstar_status_requires_active_controller_and_never_checks_http(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = module.PackagedMachine(
                Path(directory), Path(directory), "sha256:" + "a" * 64,
                "northstar", "machine-01", {
                    "machines": ["machine-01"],
                    "primary_machine_id": "machine-01",
                    "synthetic_workload": module.NORTHSTAR_WORKLOAD,
                })
            runtime.record["state"] = "running"
            runtime.resources = Mock(return_value={
                "endpoint": ("machine-container", {"State": {"Running": True}})
            })
            runtime.synthetic_controller_active = Mock(return_value=False)
            runtime.fetch = Mock(side_effect=AssertionError("HTTP fallback is forbidden"))
            result = runtime.status()
            self.assertEqual(result["state"], "unavailable")
            self.assertFalse(result["controller_link_active"])
            self.assertIn("not active", result["error"])
            runtime.fetch.assert_not_called()

    def test_northstar_asset_report_names_missing_contract_files_without_reading_them(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            runtime = module.PackagedMachine(
                root, root, "sha256:" + "a" * 64, "northstar", "machine-01", {
                    "machines": ["machine-01"],
                    "primary_machine_id": "machine-01",
                    "synthetic_workload": module.NORTHSTAR_WORKLOAD,
                }, create_directory=False)
            environment = {
                "SIMULATION_NORTHSTAR_MACHINE_CERT_DIR": str(root / "machine-certs"),
                "SIMULATION_NORTHSTAR_WORKLOAD_DIR": str(root / "workloads"),
            }
            with patch.dict(module.os.environ, environment):
                report = runtime.synthetic_asset_report()
            self.assertEqual(report["machine_cert"]["directory"], "missing")
            self.assertEqual(report["workload"]["directory"], "missing")
            self.assertEqual(
                set(report["machine_cert"]["assets"]),
                {"control-ca.crt", "client.crt", "client.key"})
            self.assertEqual(
                set(report["workload"]["assets"]),
                {"ca.crt", "client-finance-rsa.key", "machine-controller-machine-01.key"})
            self.assertTrue(all(
                state == "missing"
                for group in report.values()
                for state in group["assets"].values()))

    def test_northstar_https_probe_uses_mtls_get_ping_and_never_posts(self):
        class TestConnection:
            def __init__(self, host, port, *, context, timeout):
                self.host, self.port = host, port
                self.context, self.timeout = context, timeout
                self.requests = []

            def request(self, method, path, *, headers):
                self.requests.append((method, path, headers))

            def getresponse(self):
                class Response:
                    status = 204

                    def close(self):
                        pass

                return Response()

            def close(self):
                pass

        class TestContext:
            minimum_version = None

            def load_cert_chain(self, *, certfile, keyfile):
                self.certfile, self.keyfile = certfile, keyfile

        connection = TestConnection("fd00:1::2", 8792, context=None, timeout=5)
        context = TestContext()
        settings = {
            "control_url": "https://[fd00:1::2]:8792",
            "machine_cert_dir": Path("/private/machine-01"),
        }
        with (
            patch.object(module.ssl, "create_default_context", return_value=context) as create_context,
            patch.object(module, "MachineControlHTTPSConnection", return_value=connection) as connect,
        ):
            result = module.probe_machine_control_https(
                settings, "northstar", "generation-123")
        self.assertEqual(result["state"], "passed")
        self.assertEqual(result["method"], "GET /internal/ping")
        self.assertEqual(context.minimum_version, module.ssl.TLSVersion.TLSv1_3)
        create_context.assert_called_once_with(
            cafile="/private/machine-01/control-ca.crt")
        self.assertEqual(context.certfile, "/private/machine-01/client.crt")
        self.assertEqual(context.keyfile, "/private/machine-01/client.key")
        connect.assert_called_once_with(
            "fd00:1::2", 8792, context=context, timeout=5)
        self.assertEqual(connection.requests, [(
            "GET", "/internal/ping", {
                "X-ZPR-Organization": "northstar",
                "X-ZPR-Generation": "generation-123",
            })])

    def test_machine_control_https_connection_uses_controller_server_identity(self):
        raw_socket = Mock()
        wrapped_socket = Mock()
        context = Mock()
        context.wrap_socket.return_value = wrapped_socket
        connection = module.MachineControlHTTPSConnection(
            "fd00:1::2", 8792, context=context, timeout=5)
        with patch.object(module.socket, "create_connection", return_value=raw_socket) as connect:
            connection.connect()
        connect.assert_called_once_with(("fd00:1::2", 8792), 5, None)
        context.wrap_socket.assert_called_once_with(
            raw_socket, server_hostname="host.docker.internal")
        self.assertIs(connection.sock, wrapped_socket)

    def test_northstar_readiness_probe_reports_identity_heartbeat_and_workload_without_writes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            certs = root / "machine-certs"
            workloads = root / "workloads"
            certs.mkdir(mode=0o700)
            workloads.mkdir(mode=0o700)
            for name in ("control-ca.crt", "client.crt"):
                path = certs / name
                path.write_text("public test certificate fixture")
                path.chmod(0o644)
            key = certs / "client.key"
            key.write_text("private test key fixture")
            key.chmod(0o600)
            for name in ("ca.crt", "client-finance-rsa.key",
                         "machine-controller-machine-01.key"):
                path = workloads / name
                path.write_text("runtime test fixture")
                path.chmod(0o644 if name == "ca.crt" else 0o600)
            runtime = module.PackagedMachine(
                root, root, "sha256:" + "a" * 64, "northstar", "machine-01", {
                    "machines": ["machine-01"],
                    "primary_machine_id": "machine-01",
                    "synthetic_workload": module.NORTHSTAR_WORKLOAD,
                }, create_directory=False)
            environment = {
                "SIMULATION_NORTHSTAR_MACHINE_IMAGE": "sha256:" + "b" * 64,
                "SIMULATION_NORTHSTAR_DOCKER_NETWORK": "zpr-northstar-synthetic",
                "SIMULATION_NORTHSTAR_NODE_ADDRESS": "10.20.0.2:5000",
                "SIMULATION_NORTHSTAR_CONTROL_URL": "https://[fd00:1::2]:8792",
                "SIMULATION_RUNTIME_GENERATION": "generation-123",
                "SIMULATION_NORTHSTAR_MACHINE_CERT_DIR": str(certs),
                "SIMULATION_NORTHSTAR_WORKLOAD_DIR": str(workloads),
            }
            runtime.synthetic_settings = Mock(return_value={
                "machine_cert_dir": certs,
                "workload_dir": workloads,
            })
            runtime.status = Mock(return_value={
                "state": "running",
                "controller_link_active": True,
                "workload": {"state": "active", "address": "fd00:1:4::1"},
            })
            with (
                patch.dict(module.os.environ, environment),
                patch.object(module, "certificate_common_name", return_value=["machine-01"]),
                patch.object(module, "probe_machine_control_https", return_value={
                    "state": "passed",
                    "method": "GET /internal/ping",
                }) as probe_https,
            ):
                result = runtime.probe_synthetic_readiness()
            self.assertEqual(result["certificate_identity"]["state"], "passed")
            self.assertEqual(result["control_endpoint"]["state"], "passed")
            self.assertEqual(result["machine"]["workload"]["address"], "fd00:1:4::1")
            self.assertEqual(result["controller_heartbeat"]["state"], "not_checked")
            self.assertEqual(result["controller_heartbeat"]["owner_action"], "wait_controller")
            self.assertFalse(result["ready"])
            self.assertEqual(result["blockers"], [])
            self.assertFalse(runtime.directory.exists())
            runtime.status.assert_called_once_with()
            probe_https.assert_called_once()
            self.assertNotIn("private test key fixture", json.dumps(result))

            with (
                patch.object(module, "certificate_common_name", return_value=["machine-02"]),
                patch.object(module, "probe_machine_control_https") as wrong_identity_probe,
            ):
                mismatch = runtime.probe_synthetic_readiness()
            self.assertEqual(mismatch["certificate_identity"]["state"], "blocked")
            self.assertIn("machine controller certificate identity mismatch", mismatch["blockers"])
            self.assertEqual(mismatch["control_endpoint"]["state"], "not_run")
            wrong_identity_probe.assert_not_called()

    def test_northstar_probe_cli_reports_asset_gaps_without_creating_runtime_state(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            _, environment = self.northstar_profile_fixture(root)
            environment.update({
                "PYTHONDONTWRITEBYTECODE": "1",
                "SIMULATION_NORTHSTAR_MACHINE_IMAGE": "sha256:" + "b" * 64,
                "SIMULATION_NORTHSTAR_DOCKER_NETWORK": "zpr-northstar-synthetic",
                "SIMULATION_NORTHSTAR_NODE_ADDRESS": "10.20.0.2:5000",
                "SIMULATION_NORTHSTAR_CONTROL_URL": "https://[fd00:1::2]:8792",
                "SIMULATION_RUNTIME_GENERATION": "generation-123",
                "SIMULATION_NORTHSTAR_MACHINE_CERT_DIR": str(root / "missing-certs"),
                "SIMULATION_NORTHSTAR_WORKLOAD_DIR": str(root / "missing-workloads"),
            })
            process_environment = dict(module.os.environ)
            process_environment.update(environment)
            result = subprocess.run(
                [
                    sys.executable,
                    str(Path(__file__).with_name("packaged-machine-runtime.py")),
                    "probe", "northstar", "machine-01",
                ],
                check=False,
                capture_output=True,
                text=True,
                env=process_environment,
            )
            self.assertEqual(result.returncode, 2, result.stderr)
            report = json.loads(result.stdout)
            self.assertEqual(report["probe_state"], "blocked")
            self.assertEqual(report["controller_heartbeat"]["owner_action"], "wait_controller")
            self.assertEqual(report["assets"]["machine_cert"]["directory"], "missing")
            self.assertEqual(report["assets"]["workload"]["directory"], "missing")
            runtime_token = "zpr-org-" + module.hashlib.sha256(
                b"northstar/machine-01/test-release").hexdigest()[:20]
            self.assertFalse((root / "state" / runtime_token).exists())

    def test_northstar_status_reports_running_only_for_live_controller(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = module.PackagedMachine(
                Path(directory), Path(directory), "sha256:" + "a" * 64,
                "northstar", "machine-01", {
                    "machines": ["machine-01"],
                    "primary_machine_id": "machine-01",
                    "synthetic_workload": module.NORTHSTAR_WORKLOAD,
                })
            runtime.record["state"] = "running"
            runtime.resources = Mock(return_value={
                "endpoint": ("machine-container", {"State": {"Running": True}})
            })
            runtime.synthetic_controller_active = Mock(return_value=True)
            runtime.synthetic_workload_status = Mock(
                return_value={"state": "active", "address": "fd00:1:4::1"})
            result = runtime.status()
            self.assertEqual(result["state"], "running")
            self.assertTrue(result["controller_link_active"])
            self.assertEqual(result["workload"]["state"], "active")
            self.assertEqual(result["workload"]["address"], "fd00:1:4::1")

    def test_northstar_workload_readiness_prefers_granted_tun_address_and_keeps_fallback(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = module.PackagedMachine(
                Path(directory), Path(directory), "sha256:" + "a" * 64,
                "northstar", "machine-01", {
                    "machines": ["machine-01"],
                    "primary_machine_id": "machine-01",
                    "synthetic_workload": module.NORTHSTAR_WORKLOAD,
                })
            runtime.execute = Mock(side_effect=(
                b"present",
                b"(Active)",
                json.dumps([{"ifname": "tun0", "addr_info": [
                    {"local": "fe80::1", "scope": "link"},
                    {"local": "fd00:1:4::1", "scope": "global"},
                    {"local": "fd5a:5052:adda:1::3", "scope": "global"},
                ]}]).encode(),
            ))
            self.assertEqual(runtime.synthetic_workload_status("machine-container"), {
                "state": "active", "address": "fd5a:5052:adda:1::3",
            })

            runtime.execute = Mock(side_effect=(
                b"present",
                b"(Active)",
                json.dumps([{"ifname": "tun0", "addr_info": [
                    {"local": "fd00:1:4::1", "scope": "global"}]}]).encode(),
            ))
            self.assertEqual(runtime.synthetic_workload_status("machine-container"), {
                "state": "active", "address": "fd00:1:4::1",
            })

            runtime.execute = Mock(side_effect=(
                b"present",
                b"(Active)",
                json.dumps([{"ifname": "tun0", "addr_info": [
                    {"local": "fd00:1:2::1", "scope": "global"},
                    {"local": "fe80::1", "scope": "link"},
                ]}]).encode(),
            ))
            self.assertEqual(runtime.synthetic_workload_status("machine-container"), {
                "state": "unavailable", "address": "",
            })

            runtime.execute = Mock(return_value=b"absent")
            self.assertEqual(runtime.synthetic_workload_status("machine-container"), {
                "state": "not_started", "address": "",
            })
            runtime.execute.assert_called_once()

    def test_northstar_dispatch_uses_synthetic_actions_without_generic_demo_path(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = module.PackagedMachine(
                Path(directory), Path(directory), "sha256:" + "a" * 64,
                "northstar", "machine-01", {
                    "machines": ["machine-01"],
                    "primary_machine_id": "machine-01",
                    "synthetic_workload": module.NORTHSTAR_WORKLOAD,
                })
            runtime.provision_synthetic = Mock()
            runtime.provision()
            runtime.provision_synthetic.assert_called_once_with()
            runtime.start_synthetic = Mock()
            runtime.start()
            runtime.start_synthetic.assert_called_once_with()
            runtime.status = Mock(return_value={
                "state": "running", "controller_link_active": True,
            })
            runtime.verify()
            runtime.status.assert_called_once_with()

    def test_inventory_survives_new_process_and_rejects_changed_image(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = self.runtime(directory)
            runtime.record.update(state="stopped", public_key_sha256="retained")
            runtime.save()
            restored = self.runtime(directory)
            restored.load()
            self.assertEqual(restored.record["public_key_sha256"], "retained")
            self.assertEqual(restored.record["state"], "stopped")
            restored.record["image"] = "sha256:" + "b" * 64
            restored.save()
            with self.assertRaisesRegex(RuntimeError, "identity or package image changed"):
                runtime.load()

    def test_secondary_member_does_not_inherit_primary_identity_volume(self):
        with tempfile.TemporaryDirectory() as directory:
            primary = self.runtime(directory)
            primary.record["identity_volume"] = "primary-identity"
            primary.save()
            secondary = module.PackagedMachine(
                Path(directory), Path(directory), "sha256:" + "a" * 64,
                "demo", "demo-machine-02", {
                    "machines": ["demo-machine-01", "demo-machine-02"],
                    "primary_machine_id": "demo-machine-01",
                })
            secondary.load()
            self.assertIsNone(secondary.identity_volume)
            self.assertIsNone(secondary.current_member()["identity_volume"])

    def test_inventory_rejects_members_sharing_endpoint_identity_volume(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = self.runtime(directory)
            runtime.record["identity_volume"] = "shared-identity"
            runtime.identity_volume = "shared-identity"
            runtime.save()
            runtime.record["members"]["demo-machine-02"] = {
                "identity": "demo.demo-machine-02", "state": "unprovisioned",
                "public_key_sha256": None, "identity_volume": "shared-identity",
                "container": None,
            }
            runtime.save()
            with self.assertRaisesRegex(RuntimeError, "distinct endpoint identity volumes"):
                runtime.load()

    def test_persistent_processes_have_no_acceptance_timeout_lease(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = self.runtime(directory)
            self.assertEqual(runtime.container_process(["/bin/sh", "-ec", "sleep infinity"]),
                             ["--entrypoint", "/bin/sh", runtime.image, "-ec", "sleep infinity"])
            self.assertEqual(runtime.adapter_process(), ["/app/bin/ph", "adapter"])

    def test_changed_identity_is_not_reapproved(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = self.runtime(directory)
            runtime.record["public_key_sha256"] = "old"
            runtime.execute = Mock(return_value=b"new-public-key")
            with self.assertRaisesRegex(RuntimeError, "identity changed"):
                runtime.fingerprint("endpoint")
            self.assertEqual(runtime.record["public_key_sha256"], "old")

    def test_status_requires_real_traffic_not_only_running_containers(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = self.runtime(directory)
            runtime.record["state"] = "ready"
            runtime.resources = Mock(return_value={
                role: (role, {"State": {"Running": True}})
                for role in ("vs", "node0", "web0", "endpoint")})
            runtime.fingerprint = Mock()
            runtime.fetch = Mock(side_effect=RuntimeError("HTTP failed"))
            result = runtime.status()
            self.assertEqual(result["state"], "unavailable")
            self.assertIn("HTTP failed", result["error"])
            self.assertIn("Shared organization node", result["scope"])
            runtime.fetch.assert_called_once_with("endpoint")

    def test_stopped_resources_are_never_reported_ready(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = self.runtime(directory)
            runtime.record["state"] = "ready"
            runtime.resources = Mock(return_value={
                "endpoint": ("endpoint", {"State": {"Running": False}})})
            self.assertEqual(runtime.status()["state"], "unavailable")

    def test_stop_preserves_resources_and_identity_volume(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = self.runtime(directory)
            runtime.record.update(identity_volume="identity-volume", public_key_sha256="retained")
            runtime.resources = Mock(return_value={"endpoint": ("id", {"State": {"Running": True}})})
            runtime.inspect = Mock(return_value={"State": {"Running": False}})
            runtime.docker = Mock()
            runtime.stop()
            runtime.docker.assert_called_once_with("stop", "--time", "5", "id")
            self.assertEqual(runtime.record["identity_volume"], "identity-volume")
            self.assertEqual(runtime.record["public_key_sha256"], "retained")
            self.assertEqual(runtime.record["state"], "stopped")

    def test_partial_initialization_is_not_automatically_rotated(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = self.runtime(directory)
            runtime.state.mkdir()
            with self.assertRaisesRegex(RuntimeError, "incomplete identity initialization"):
                runtime.initialize_state()

    def test_owned_container_checks_name_image_network_and_labels(self):
        for changed in ("name", "image", "network", "label"):
            with self.subTest(changed=changed), tempfile.TemporaryDirectory() as directory:
                runtime = self.runtime(directory)
                runtime.network = "network-id"
                name = runtime.token + "-vs"
                config = {"Id": "id", "Name": "/" + name, "Image": runtime.image,
                          "Config": {"Labels": {"zpr.simulator.identity-lab": runtime.token}},
                          "HostConfig": {"NetworkMode": "network-id"}}
                if changed == "name":
                    config["Name"] = "/foreign"
                elif changed == "image":
                    config["Image"] = "foreign"
                elif changed == "network":
                    config["HostConfig"]["NetworkMode"] = "foreign"
                else:
                    config["Config"]["Labels"] = {}
                runtime.inspect = Mock(return_value=config)
                with self.assertRaisesRegex(RuntimeError, "ownership changed"):
                    runtime.owned(name, "id")

    def test_provision_rejects_already_provisioned_machine(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = self.runtime(directory)
            runtime.record["state"] = "stopped"
            runtime.docker = Mock()
            with self.assertRaisesRegex(RuntimeError, "already provisioned"):
                runtime.provision()
            runtime.docker.assert_not_called()

    def test_retry_recovers_interruption_after_recorded_container_removal(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = self.runtime(directory)
            runtime.network = "network"
            runtime.record["resources"] = {runtime.token + "-endpoint": "removed-id"}
            with patch.object(module.identity_runtime.IdentityLab, "docker", return_value=b""):
                runtime.recover_pending()
            self.assertEqual(runtime.record["resources"], {})

    def test_retry_refuses_name_replacement_instead_of_adopting_it(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = self.runtime(directory)
            runtime.network = "network"
            runtime.record["resources"] = {runtime.token + "-endpoint": "original-id"}
            with patch.object(module.identity_runtime.IdentityLab, "docker", return_value=b"foreign-id\n"):
                with self.assertRaisesRegex(RuntimeError, "refusing adoption"):
                    runtime.recover_pending()
            self.assertEqual(list(runtime.record["resources"].values()), ["original-id"])

    def test_configuration_requires_explicit_matching_organization_machine(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            profile = {"id": "demo", "packaged_provisioning": {
                "version": 2, "driver": "shared-approved-bootstrap",
                "primary_machine_id": "demo-machine-01",
                "machines": ["demo-machine-01", "demo-machine-02"], "service": "OciWeb"}}
            (root / "demo.json").write_text(json.dumps(profile))
            (root / "simulation.json").write_text(json.dumps({
                "organization_id": "demo", "machines": [
                    {"id": "demo-machine-01"}, {"id": "demo-machine-02"}]}))
            (root / "manifest.json").write_text(json.dumps({
                "schema": "zpr.demo.runtime-package.v2", "package_id": "zpr-demo-image",
                "platform": "linux/amd64", "release_id": "test-release",
                "component": {"docker_image_id": "sha256:" + "a" * 64}}))
            with patch.dict(module.os.environ, {
                "SIMULATION_ORGANIZATIONS_DIR": str(root), "SIMULATION_MANIFEST": str(root / "simulation.json"),
                "SIMULATION_PROVISION_PACKAGE": str(root), "SIMULATION_PROVISION_ROOT": str(root),
                "SIMULATION_PROVISION_RELEASE_ID": "test-release",
            }):
                self.assertEqual(module.configuration("demo", "demo-machine-01")[0], root)
                self.assertEqual(module.configuration("demo", "demo-machine-02")[0], root)
                with self.assertRaisesRegex(RuntimeError, "release does not match"):
                    with patch.dict(module.os.environ, {"SIMULATION_PROVISION_RELEASE_ID": "other"}):
                        module.configuration("demo", "demo-machine-02")
                with self.assertRaisesRegex(RuntimeError, "not enabled"):
                    module.configuration("demo", "demo-machine-03")
                profile.pop("packaged_provisioning")
                (root / "demo.json").write_text(json.dumps(profile))
                with self.assertRaisesRegex(RuntimeError, "not enabled"):
                    module.configuration("demo", "demo-machine-01")

    def test_example_profiles_without_packaged_contract_are_not_enabled(self):
        for organization in (
            "northstar", "redwood", "velocity", "great-lakes", "load-lab",
        ):
            with self.subTest(organization=organization), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                organizations = root / "organizations"
                organizations.mkdir()
                profile = json.loads(
                    (EXAMPLE_ORGANIZATIONS / f"{organization}.json").read_text()
                )
                self.assertEqual(profile["id"], organization)
                self.assertNotIn("packaged_provisioning", profile)
                (organizations / f"{organization}.json").write_text(json.dumps(profile))
                manifest = root / "simulation.json"
                manifest.write_text(json.dumps({
                    "organization_id": organization, "machines": [],
                }))
                with patch.dict(module.os.environ, {
                    "SIMULATION_ORGANIZATIONS_DIR": str(organizations),
                    "SIMULATION_MANIFEST": str(manifest),
                }):
                    with self.assertRaisesRegex(RuntimeError, "not enabled"):
                        module.configuration(organization, "machine-01")

    def packaged_demo_fixture(self, root, *, manifest_organization="demo"):
        root = Path(root).resolve()
        organizations = root / "organizations"
        organizations.mkdir()
        profile = json.loads(PACKAGED_DEMO_PROFILE.read_text())
        (organizations / "demo.json").write_text(json.dumps(profile))
        machine_ids = profile["packaged_provisioning"]["machines"]
        simulation_manifest = root / "simulation.json"
        simulation_manifest.write_text(json.dumps({
            "organization_id": manifest_organization,
            "machines": [{"id": machine_id} for machine_id in machine_ids],
        }))
        package = root / "package"
        package.mkdir()
        (package / "manifest.json").write_text(json.dumps({
            "schema": "zpr.demo.runtime-package.v2",
            "package_id": "zpr-demo-image",
            "platform": "linux/amd64",
            "release_id": "test-release",
            "component": {"docker_image_id": "sha256:" + "a" * 64},
        }))
        state = root / "state"
        state.mkdir(mode=0o700)
        environment = {
            "SIMULATION_ORGANIZATIONS_DIR": str(organizations),
            "SIMULATION_MANIFEST": str(simulation_manifest),
            "SIMULATION_PROVISION_PACKAGE": str(package),
            "SIMULATION_PROVISION_ROOT": str(state),
            "SIMULATION_PROVISION_RELEASE_ID": "test-release",
        }
        return profile, simulation_manifest, environment

    def test_checked_in_demo_profile_requires_v2_contract_and_matching_machine_ids(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            profile, _, environment = self.packaged_demo_fixture(root)
            contract = profile["packaged_provisioning"]
            with patch.dict(module.os.environ, environment):
                package, state, image, provisioning = module.configuration(
                    "demo", contract["machines"][1]
                )
            self.assertEqual(contract["version"], 2)
            self.assertEqual(contract["driver"], "shared-approved-bootstrap")
            self.assertEqual(contract["service"], "OciWeb")
            self.assertEqual(contract["primary_machine_id"], contract["machines"][0])
            self.assertEqual(package, root.resolve() / "package")
            self.assertEqual(state, root.resolve() / "state")
            self.assertEqual(image, "sha256:" + "a" * 64)
            self.assertEqual(provisioning["machines"], contract["machines"])

    def test_checked_in_demo_profile_rejects_contract_or_organization_mismatch(self):
        mutations = (
            ("version", 1),
            ("driver", "docker-multinode"),
            ("service", "OtherService"),
        )
        for field, value in mutations:
            with self.subTest(field=field), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                profile, _, environment = self.packaged_demo_fixture(root)
                profile["packaged_provisioning"][field] = value
                (root / "organizations/demo.json").write_text(json.dumps(profile))
                with patch.dict(module.os.environ, environment):
                    with self.assertRaisesRegex(RuntimeError, "not enabled"):
                        module.configuration("demo", "demo-machine-01")

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            profile, _, environment = self.packaged_demo_fixture(root)
            profile["id"] = "another-org"
            (root / "organizations/demo.json").write_text(json.dumps(profile))
            with patch.dict(module.os.environ, environment):
                with self.assertRaisesRegex(RuntimeError, "not enabled"):
                    module.configuration("demo", "demo-machine-01")

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            _, _, environment = self.packaged_demo_fixture(
                root, manifest_organization="another-org"
            )
            with patch.dict(module.os.environ, environment):
                with self.assertRaisesRegex(RuntimeError, "not enabled"):
                    module.configuration("demo", "demo-machine-01")

    def test_checked_in_demo_profile_rejects_missing_or_duplicate_manifest_machine_ids(self):
        for machine_ids in (
            ["demo-machine-01"],
            ["demo-machine-01", "demo-machine-02", "demo-machine-02"],
        ):
            with self.subTest(machine_ids=machine_ids), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                _, simulation_manifest, environment = self.packaged_demo_fixture(root)
                simulation_manifest.write_text(json.dumps({
                    "organization_id": "demo",
                    "machines": [{"id": machine_id} for machine_id in machine_ids],
                }))
                with patch.dict(module.os.environ, environment):
                    with self.assertRaisesRegex(RuntimeError, "not enabled"):
                        module.configuration("demo", "demo-machine-01")

    def test_second_member_uses_shared_inventory_without_overwriting_primary_identity(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            provisioning = {"machines": ["demo-machine-01", "demo-machine-02"],
                            "primary_machine_id": "demo-machine-01"}
            primary = module.PackagedMachine(
                root, root, "sha256:" + "a" * 64, "demo", "demo-machine-01", provisioning)
            primary.record.update(state="ready", public_key_sha256="primary-key")
            primary.record["members"]["demo-machine-01"] = {
                "identity": "demo.demo-machine-01", "state": "ready",
                "public_key_sha256": "primary-key", "identity_volume": "primary-volume",
                "container": "primary-endpoint", "error": ""}
            primary.save()
            secondary = module.PackagedMachine(
                root, root, "sha256:" + "a" * 64, "demo", "demo-machine-02", provisioning)
            secondary.load()
            self.assertEqual(primary.token, secondary.token)
            self.assertEqual(secondary.current_state(), "unprovisioned")
            secondary.set_state("provisioning")
            secondary.set_key_hash("secondary-key")
            secondary.current_member()["identity_volume"] = "secondary-volume"
            secondary.save()
            restored = module.PackagedMachine(
                root, root, "sha256:" + "a" * 64, "demo", "demo-machine-01", provisioning)
            restored.load()
            self.assertEqual(restored.record["state"], "ready")
            self.assertEqual(restored.record["public_key_sha256"], "primary-key")
            self.assertEqual(restored.record["members"]["demo-machine-02"]["state"], "provisioning")
            self.assertEqual(restored.record["members"]["demo-machine-02"]["public_key_sha256"], "secondary-key")

    def test_shared_policy_compilation_preserves_primary_bootstrap_and_attributes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            provisioning = {"machines": ["demo-machine-01", "demo-machine-02"],
                            "primary_machine_id": "demo-machine-01"}
            runtime = module.PackagedMachine(
                root, root, "sha256:" + "a" * 64, "demo", "demo-machine-02", provisioning)
            (runtime.state / "admin").mkdir(parents=True)
            (runtime.state / "include").mkdir()
            (runtime.state / "conf/vs").mkdir(parents=True)
            source = runtime.state / "admin/multinode-demo.zplc"
            source.write_text('[bootstrap]\\n"demo.demo-machine-01" = "../include/approved-public-key.pem"\\n')
            (runtime.state / "include/approved-public-key.pem").write_bytes(b"primary-public")
            (runtime.state / "conf/vs/attrfile.json").write_text(json.dumps({
                "device.zpr.adapter.cn": {"demo.demo-machine-01": {
                    "location": ["cloud"], "access": ["cloud"]}}}))
            compiled = runtime.state / "admin/multinode-demo.bin2"
            compiled.write_bytes(b"compiled-shared-policy")
            runtime.docker = Mock(return_value=b"")
            runtime.compile_endpoint_policy(b"secondary-public")
            self.assertIn('"demo.demo-machine-01"', source.read_text())
            self.assertIn('"demo.demo-machine-02" = "../include/approved-public-key-demo-machine-02.pem"',
                          source.read_text())
            self.assertEqual((runtime.directory / "compiler-public/approved-public-key.pem").read_bytes(),
                             b"primary-public")
            self.assertEqual((runtime.directory / "compiler-public/approved-public-key-demo-machine-02.pem")
                             .read_bytes(), b"secondary-public")
            attributes = json.loads((runtime.state / "conf/vs/attrfile.json").read_text())
            self.assertIn("demo.demo-machine-01", attributes["device.zpr.adapter.cn"])
            self.assertIn("demo.demo-machine-02", attributes["device.zpr.adapter.cn"])
            self.assertEqual((runtime.state / "conf/vs/multinode-demo.bin2").read_bytes(),
                             b"compiled-shared-policy")
            args = runtime.docker.call_args.args
            self.assertEqual(args[args.index("--user") + 1],
                             f"{module.os.getuid()}:{module.os.getgid()}")


if __name__ == "__main__":
    unittest.main()
