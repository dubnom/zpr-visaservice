import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch

spec = importlib.util.spec_from_file_location(
    "packaged_runtime", Path(__file__).with_name("packaged-machine-runtime.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class PackagedMachineTests(unittest.TestCase):
    def runtime(self, directory):
        return module.PackagedMachine(Path(directory), Path(directory), "sha256:" + "a" * 64,
                                      "demo", "demo-machine-01")

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
