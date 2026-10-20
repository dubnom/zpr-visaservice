import importlib.util
import json
from pathlib import Path
import signal
import stat
import subprocess
import tempfile
import unittest
from unittest.mock import Mock, patch

spec = importlib.util.spec_from_file_location(
    "identity_lab", Path(__file__).with_name("verify-approved-identity-lab.py"))
lab_module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lab_module)


class IdentityLabTests(unittest.TestCase):
    def make_lab(self, directory):
        root = Path(directory)
        package = root / "package"
        package.mkdir()
        (package / "manifest.json").write_text(json.dumps({
            "component": {"docker_image_id": "sha256:" + "a" * 64}}))
        return lab_module.IdentityLab(package, root)

    def test_cleanup_removes_only_recorded_ids_after_ownership_checks(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            lab.containers = ["first-id", "second-id"]
            lab.network = "network-id"
            lab.inspect = Mock(return_value={
                "Config": {"Labels": {"zpr.simulator.identity-lab": lab.token}}})
            lab.docker = Mock(return_value=json.dumps([{
                "Labels": {"zpr.simulator.identity-lab": lab.token}}]).encode())
            lab.cleanup()
            self.assertEqual(lab.inspect.call_args_list, [
                unittest.mock.call("second-id"), unittest.mock.call("first-id")])
            self.assertEqual(lab.docker.call_args_list, [
                unittest.mock.call("rm", "-f", "second-id"),
                unittest.mock.call("rm", "-f", "first-id"),
                unittest.mock.call("network", "inspect", "network-id"),
                unittest.mock.call("network", "rm", "network-id")])
            self.assertTrue(lab.directory.is_dir())

    def test_changed_container_ownership_is_preserved_and_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            lab.containers = ["foreign-id"]
            lab.network = "network-id"
            lab.inspect = Mock(return_value={"Config": {"Labels": {}}})
            lab.docker = Mock()
            with self.assertRaisesRegex(RuntimeError, "ownership changed"):
                lab.cleanup()
            lab.docker.assert_not_called()
            self.assertTrue(lab.directory.is_dir())

    def test_changed_network_ownership_is_not_removed(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            lab.network = "foreign-network"
            lab.docker = Mock(return_value=b'[{"Labels": {}}]')
            with self.assertRaisesRegex(RuntimeError, "network ownership changed"):
                lab.cleanup()
            lab.docker.assert_called_once_with("network", "inspect", "foreign-network")

    def test_interrupted_create_recovers_only_exact_pending_name_and_run_network(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            name = lab.token + "-endpoint"
            lab.pending_containers = [name]
            lab.network_requested = True
            lab.inspect = Mock(return_value={
                "Config": {"Labels": {"zpr.simulator.identity-lab": lab.token}}})
            lab.docker = Mock(side_effect=[
                b"created-id\n", b"network-id\n", b"",
                json.dumps([{"Labels": {"zpr.simulator.identity-lab": lab.token}}]).encode(), b""])
            lab.cleanup()
            self.assertEqual(lab.docker.call_args_list, [
                unittest.mock.call("ps", "-a", "--no-trunc", "--filter", "name=^/" + name + "$",
                                   "--format", "{{.ID}}"),
                unittest.mock.call("network", "ls", "--filter",
                                   "label=zpr.simulator.identity-lab=" + lab.token,
                                   "--format", "{{.ID}}"),
                unittest.mock.call("rm", "-f", "created-id"),
                unittest.mock.call("network", "inspect", "network-id"),
                unittest.mock.call("network", "rm", "network-id")])

    def test_container_is_recorded_before_isolation_check_can_fail(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            lab.network = "network-id"
            lab.docker = Mock(return_value=b"created-id\n")
            lab.inspect = Mock(return_value={
                "Image": lab.image, "HostConfig": {"Privileged": True}})
            with self.assertRaisesRegex(RuntimeError, "isolation contract"):
                lab.create("endpoint", "--cap-add", "NET_ADMIN")
            self.assertEqual(lab.containers, ["created-id"])
            self.assertEqual(lab.docker.call_count, 1)

    def test_plane_stages_private_configuration_in_container_tmpfs(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            config = lab.state / "conf/vs"
            (config / "include").mkdir(parents=True)
            include = lab.state / "include"
            include.mkdir()
            for name in (
                    "auth-ca.crt", "vs-noise.crt", "vs-noise.key", "vs-private-key.pem",
                    "node0-noise-pub.pem", "admin-tls-cert.pem", "admin-tls-key.pem"):
                (include / name).write_text(name)
            lab.create = Mock(return_value="container-id")
            lab.docker = Mock()
            lab.container_process = Mock(return_value=[])
            lab.plane("vs", "172.30.0.11")
            args = lab.create.call_args.args
            self.assertIn("/conf:rw,noexec,nosuid,size=8m,mode=0700", args)
            self.assertIn("/logs:rw,noexec,nosuid,size=16m,mode=0700", args)
            self.assertFalse(any(str(config) in arg for arg in args))
            self.assertEqual(lab.docker.call_count, 8)
            self.assertEqual(lab.docker.call_args_list[0].args[:3],
                             ("exec", "container-id", "mkdir"))
            for call in lab.docker.call_args_list[1:]:
                self.assertEqual(call.args[:3], ("exec", "-i", "container-id"))
                self.assertTrue(call.kwargs["data"])

    def test_private_directory_refuses_symlinks_and_unsafe_modes(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            lab_module.private_directory(path)
            link = path / "link"
            link.symlink_to(path, target_is_directory=True)
            with self.assertRaises(RuntimeError):
                lab_module.private_directory(link)
            path.chmod(0o755)
            with self.assertRaises(RuntimeError):
                lab_module.private_directory(path)
            path.chmod(0o700)

    def test_initialize_state_keeps_script_output_off_the_json_channel(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            with patch.object(lab_module.subprocess, "run") as run:
                lab.initialize_state()
            run.assert_called_once()
            self.assertIs(run.call_args.kwargs["stdout"], lab_module.sys.stderr)

    def test_success_removes_only_its_fresh_directory(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            lab.run = Mock()
            lab.cleanup = Mock()
            with patch.object(lab_module.sys, "argv", ["lab", "--disposable-host"]), \
                    patch.dict(lab_module.os.environ, {
                        "ZPR_DEMO_PACKAGE_DIR": str(lab.package.resolve()),
                        "ZPR_IDENTITY_LAB_ROOT": str(Path(directory).resolve())}), \
                    patch.object(lab_module.os, "geteuid",
                                 side_effect=[0, lab_module.os.getuid()]), \
                    patch.object(lab_module, "IdentityLab", return_value=lab):
                lab_module.main()
            lab.run.assert_called_once_with()
            lab.cleanup.assert_called_once_with()
            self.assertFalse(lab.directory.exists())
            self.assertTrue(lab.package.is_dir())

    def test_failure_cleans_resources_but_preserves_private_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            lab.run = Mock(side_effect=RuntimeError("traffic failed"))
            lab.cleanup = Mock()
            with patch.object(lab_module.sys, "argv", ["lab", "--disposable-host"]), \
                    patch.dict(lab_module.os.environ, {
                        "ZPR_DEMO_PACKAGE_DIR": str(lab.package.resolve()),
                        "ZPR_IDENTITY_LAB_ROOT": str(Path(directory).resolve())}), \
                    patch.object(lab_module.os, "geteuid",
                                 side_effect=[0, lab_module.os.getuid()]), \
                    patch.object(lab_module, "IdentityLab", return_value=lab):
                with self.assertRaisesRegex(RuntimeError, "traffic failed"):
                    lab_module.main()
            lab.cleanup.assert_called_once_with()
            self.assertEqual(stat.S_IMODE(lab.directory.stat().st_mode), 0o700)

    def test_wait_failure_is_bounded(self):
        probe = Mock(return_value=False)
        with patch.object(lab_module.time, "sleep"):
            with self.assertRaisesRegex(RuntimeError, "explicit rejection"):
                lab_module.wait_for("explicit rejection", probe, attempts=3)
        self.assertEqual(probe.call_count, 3)

    def authorization_fixture(self, directory, *, denial=True, sweep=True, blocked_status=28):
        lab = self.make_lab(directory)
        (lab.state / "conf/vs").mkdir(parents=True)
        (lab.state / "logs").mkdir()
        path = lab.state / "conf/vs/attrfile.json"
        original = json.dumps({"device.zpr.adapter.cn": {
            lab.identity: {"location": ["cloud"], "access": ["cloud"]}}}).encode()
        path.write_bytes(original)
        log = lab.state / "logs/vs.log"
        log.write_text("old unrelated evidence\n")
        actor = {"zpr_addr": "fd5a:5052::100"}
        refreshes = []

        def admin(visa, *args, write=False):
            self.assertEqual(visa, "visa")
            if args == ("services", "--id", "attrfile", "--flush"):
                self.assertTrue(write)
                refreshes.append(json.loads(path.read_bytes()))
                if len(refreshes) == 1 and sweep:
                    with log.open("a") as output:
                        output.write("visa sweep reason=AttributeChange vinst=1: total=2 allowed=0 revoked=2 skipped_stale=0\n")
                return b""
            self.assertFalse(write)
            if args == ("actors", "--cn", lab.identity, "--visas"):
                return b'[{"id": 7}]'
            if args == ("visas", "--id", "7"):
                return json.dumps({
                    "source_addr": actor["zpr_addr"], "dest_addr": "fd5a:5052:8888::8",
                    "dest_port": 80, "proto": "TCP"}).encode()
            if args == ("actors", "--cn", "node0.demo"):
                return json.dumps({"node_details": {
                    "visas": [7] if not refreshes else [], "in_sync": True,
                    "pending_install": 0, "pending_revocation": 0}}).encode()
            if args == ("actors", "--cn", lab.identity):
                return json.dumps({**actor, "attrs": [{"key": "user.access", "value":
                    [] if len(refreshes) == 1 else ["cloud"]}]}).encode()
            self.fail("unexpected admin arguments: " + repr(args))

        def fetch(endpoint):
            if endpoint == "endpoint" and len(refreshes) == 1:
                if denial:
                    with log.open("a") as output:
                        output.write("eval denied (no match) for flow " + actor["zpr_addr"]
                                     + " -> fd5a:5052:8888::8 FlowInfo { dest_port: 80 } no matching policy\n")
                if blocked_status:
                    raise subprocess.CalledProcessError(blocked_status, ["curl"])

        lab.admin = Mock(side_effect=admin)
        lab.fetch = Mock(side_effect=fetch)
        lab.sync_service_file = Mock()
        lab.service_log = Mock(side_effect=lambda container, name: log.read_bytes())
        return lab, actor, path, original, refreshes

    def test_authorization_withdraws_and_restores_only_fresh_attribute_data(self):
        with tempfile.TemporaryDirectory() as directory:
            lab, actor, path, original, refreshes = self.authorization_fixture(directory)
            lab.authorization_cycle("visa", "endpoint", "web", actor)
            self.assertEqual(path.read_bytes(), original)
            self.assertEqual(len(refreshes), 2)
            self.assertEqual(refreshes[0]["device.zpr.adapter.cn"][lab.identity]["access"], [])
            self.assertEqual(refreshes[1]["device.zpr.adapter.cn"][lab.identity]["access"], ["cloud"])
            self.assertEqual(lab.fetch.call_args_list, [
                unittest.mock.call("web"), unittest.mock.call("endpoint"), unittest.mock.call("endpoint")])

    def test_withdrawal_requires_denial_and_revocation_not_just_timeout(self):
        for options, message in [
                ({"denial": False}, "exact-flow policy denial"),
                ({"sweep": False}, "revocation sweep"),
                ({"blocked_status": 0}, "still fetched")]:
            with self.subTest(options=options), tempfile.TemporaryDirectory() as directory:
                lab, actor, _, _, _ = self.authorization_fixture(directory, **options)
                with self.assertRaisesRegex(RuntimeError, message):
                    lab.authorization_cycle("visa", "endpoint", "web", actor)

    def test_withdrawal_does_not_treat_transport_errors_as_policy_denial(self):
        with tempfile.TemporaryDirectory() as directory:
            lab, actor, _, _, _ = self.authorization_fixture(directory, blocked_status=7)
            with self.assertRaises(subprocess.CalledProcessError) as error:
                lab.authorization_cycle("visa", "endpoint", "web", actor)
            self.assertEqual(error.exception.returncode, 7)

    def test_authorization_requires_live_http_visa_before_changing_attributes(self):
        with tempfile.TemporaryDirectory() as directory:
            lab, actor, path, original, refreshes = self.authorization_fixture(directory)
            lab.admin = Mock(return_value=b"[]")
            with self.assertRaisesRegex(RuntimeError, "no matching live visa"):
                lab.authorization_cycle("visa", "endpoint", "web", actor)
            self.assertEqual(path.read_bytes(), original)
            self.assertEqual(refreshes, [])

    def test_pending_node_revocation_cannot_pass(self):
        with tempfile.TemporaryDirectory() as directory:
            lab, actor, _, _, _ = self.authorization_fixture(directory)
            underlying = lab.admin.side_effect
            def admin(visa, *args, **kwargs):
                result = underlying(visa, *args, **kwargs)
                if args == ("actors", "--cn", "node0.demo"):
                    node = json.loads(result)
                    node["node_details"]["pending_revocation"] = 1
                    return json.dumps(node).encode()
                return result
            lab.admin.side_effect = admin
            with patch.object(lab_module.time, "sleep"):
                with self.assertRaisesRegex(RuntimeError, "HTTP visas installed"):
                    lab.authorization_cycle("visa", "endpoint", "web", actor)

    def test_unrelated_denial_or_zero_revocations_cannot_pass(self):
        for old, new, message in [
                ("-> fd5a:5052:8888::8 ", "-> fd5a:5052:8888::9 ", "exact-flow"),
                ("dest_port: 80 }", "dest_port: 18080 }", "exact-flow"),
                ("revoked=2 ", "revoked=0 ", "revocation sweep")]:
            with self.subTest(new=new), tempfile.TemporaryDirectory() as directory:
                lab, actor, _, _, _ = self.authorization_fixture(directory)
                original_fetch = lab.fetch.side_effect
                def fetch(endpoint):
                    try:
                        original_fetch(endpoint)
                    finally:
                        log = lab.state / "logs/vs.log"
                        log.write_text(log.read_text().replace(old, new))
                lab.fetch.side_effect = fetch
                with self.assertRaisesRegex(RuntimeError, message):
                    lab.authorization_cycle("visa", "endpoint", "web", actor)

    def test_admin_uses_scoped_credentials_over_stdin_not_arguments(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            (lab.state / "bob").mkdir(parents=True)
            (lab.state / "bob/client.key").write_text("private-write-key\n")
            (lab.state / "bob/web-monitor.key").write_text("private-read-key\n")
            lab.docker = Mock(return_value=b"{}")
            lab.actor("visa")
            lab.admin("visa", "services", "--id", "attrfile", "--flush", write=True)
            read, write = lab.docker.call_args_list
            self.assertEqual(read.kwargs["data"], b"private-read-key\n")
            self.assertEqual(write.kwargs["data"], b"private-write-key\n")
            for call in (read, write):
                self.assertNotIn("private-read-key", repr(call.args))
                self.assertNotIn("private-write-key", repr(call.args))

    def test_signal_interrupt_is_not_swallowed_as_a_retryable_subprocess_error(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            lab.run = Mock(side_effect=lambda: signal.raise_signal(signal.SIGTERM))
            lab.cleanup = Mock()
            with patch.object(lab_module.sys, "argv", ["lab", "--disposable-host"]), \
                    patch.dict(lab_module.os.environ, {
                        "ZPR_DEMO_PACKAGE_DIR": str(lab.package.resolve()),
                        "ZPR_IDENTITY_LAB_ROOT": str(Path(directory).resolve())}), \
                    patch.object(lab_module.os, "geteuid",
                                 side_effect=[0, lab_module.os.getuid()]), \
                    patch.object(lab_module, "IdentityLab", return_value=lab):
                with self.assertRaisesRegex(RuntimeError, "lab interrupted by signal"):
                    lab_module.main()
            lab.cleanup.assert_called_once_with()
            self.assertTrue(lab.directory.is_dir())

    def endpoint_config(self, lab):
        return {
            "Image": lab.image,
            "Config": {"Labels": {"zpr.simulator.identity-lab": lab.token}},
            "HostConfig": {
                "Binds": None, "VolumesFrom": None,
                "Tmpfs": {"/run/zpr": "rw,noexec,nosuid,size=4m,mode=0700"},
                "Mounts": [{"Type": "volume", "Source": lab.identity_volume,
                            "Target": "/var/lib/zpr-identity",
                            "VolumeOptions": {"NoCopy": True}}],
            },
            "Mounts": [{"Type": "volume", "Name": lab.identity_volume,
                        "Destination": "/var/lib/zpr-identity", "RW": True}],
        }

    def test_endpoint_storage_rejects_foreign_extra_or_readonly_mounts(self):
        for change in ("foreign", "extra", "readonly", "host", "copy"):
            with self.subTest(change=change), tempfile.TemporaryDirectory() as directory:
                lab = self.make_lab(directory)
                lab.identity_volume = lab.token + "-identity"
                lab.check_identity_volume = Mock()
                config = self.endpoint_config(lab)
                if change == "foreign":
                    config["Mounts"][0]["Name"] = "foreign"
                elif change == "extra":
                    config["Mounts"].append(dict(config["Mounts"][0]))
                elif change == "readonly":
                    config["Mounts"][0]["RW"] = False
                elif change == "host":
                    config["HostConfig"]["Binds"] = ["/host:/secrets"]
                else:
                    config["HostConfig"]["Mounts"][0]["VolumeOptions"] = {}
                with self.assertRaises(RuntimeError):
                    lab.check_endpoint_storage(config, persistent=True)
                lab.check_identity_volume.assert_not_called()

    def test_restart_requires_same_public_key_and_policy_and_six_fetches(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            lab.identity_volume = lab.token + "-identity"
            lab.inspect = Mock(side_effect=[
                self.endpoint_config(lab), {"State": {"Running": False}}])
            lab.check_identity_volume = Mock()
            lab.docker = Mock()
            lab.prepare_endpoint = Mock()
            lab.start_adapter = Mock()
            lab.actor = Mock(return_value={"cn": lab.identity, "node": False})
            lab.fetch = Mock()
            lab.execute = Mock(side_effect=[b"policy", b"public", b"(Active)", b"policy"])
            with patch.object(lab_module.time, "sleep"):
                lab.restart_endpoint("endpoint", "visa", "172.19.0.13", b"public")
            self.assertEqual(lab.docker.call_args_list, [
                unittest.mock.call("stop", "--time", "2", "endpoint"),
                unittest.mock.call("start", "endpoint")])
            lab.start_adapter.assert_called_once_with("endpoint", "172.19.0.13")
            self.assertEqual(lab.fetch.call_count, 6)

    def test_empty_driver_configuration_from_docker_cli_is_accepted(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            lab.identity_volume = lab.token + "-identity"
            lab.check_identity_volume = Mock()
            config = self.endpoint_config(lab)
            config["HostConfig"]["Mounts"][0]["VolumeOptions"]["DriverConfig"] = {}
            lab.check_endpoint_storage(config, persistent=True)
            config["HostConfig"]["Mounts"][0]["VolumeOptions"]["DriverConfig"] = {"Name": "foreign"}
            with self.assertRaisesRegex(RuntimeError, "owned identity volume"):
                lab.check_endpoint_storage(config, persistent=True)

    def test_restart_identity_change_fails_before_adapter_admission(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            lab.identity_volume = lab.token + "-identity"
            lab.inspect = Mock(side_effect=[
                self.endpoint_config(lab), {"State": {"Running": False}}])
            lab.check_identity_volume = Mock()
            lab.docker = Mock()
            lab.prepare_endpoint = Mock()
            lab.start_adapter = Mock()
            lab.execute = Mock(side_effect=[b"policy", b"different-public"])
            with self.assertRaisesRegex(RuntimeError, "identity changed"):
                lab.restart_endpoint("endpoint", "visa", "172.19.0.13", b"public")
            lab.start_adapter.assert_not_called()

    def test_restart_rejects_changed_owner_before_stopping_container(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            config = self.endpoint_config(lab)
            config["Config"]["Labels"] = {}
            lab.inspect = Mock(return_value=config)
            lab.docker = Mock()
            with self.assertRaisesRegex(RuntimeError, "ownership or image changed"):
                lab.restart_endpoint("endpoint", "visa", "172.19.0.13", b"public")
            lab.docker.assert_not_called()

    def test_restart_rejects_policy_change_even_when_traffic_works(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            lab.identity_volume = lab.token + "-identity"
            lab.inspect = Mock(side_effect=[
                self.endpoint_config(lab), {"State": {"Running": False}}])
            lab.check_identity_volume = Mock()
            lab.docker = Mock()
            lab.prepare_endpoint = Mock()
            lab.start_adapter = Mock()
            lab.actor = Mock(return_value={"cn": lab.identity, "node": False})
            lab.fetch = Mock()
            lab.execute = Mock(side_effect=[b"policy", b"public", b"(Active)", b"changed"])
            with patch.object(lab_module.time, "sleep"):
                with self.assertRaisesRegex(RuntimeError, "policy changed"):
                    lab.restart_endpoint("endpoint", "visa", "172.19.0.13", b"public")
            self.assertEqual(lab.fetch.call_count, 6)

    def test_volume_cleanup_checks_owner_and_driver_before_removal(self):
        for changed in ("none", "owner", "driver", "options", "name"):
            with self.subTest(changed=changed), tempfile.TemporaryDirectory() as directory:
                lab = self.make_lab(directory)
                lab.identity_volume = lab.token + "-identity"
                volume = {"Name": lab.identity_volume, "Driver": "local", "Options": None,
                          "Labels": {"zpr.simulator.identity-lab": lab.token}}
                if changed == "owner":
                    volume["Labels"] = {}
                elif changed == "driver":
                    volume["Driver"] = "plugin"
                elif changed == "options":
                    volume["Options"] = {"device": "/host"}
                elif changed == "name":
                    volume["Name"] = "foreign"
                lab.docker = Mock(return_value=json.dumps([volume]).encode())
                if changed == "none":
                    lab.cleanup()
                    lab.docker.assert_any_call("volume", "rm", lab.identity_volume)
                else:
                    with self.assertRaisesRegex(RuntimeError, "ownership or storage"):
                        lab.cleanup()
                    self.assertEqual(lab.docker.call_count, 1)

    def test_interrupted_volume_create_recovers_only_exact_owned_name(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.make_lab(directory)
            lab.volume_requested = True
            name = lab.token + "-identity"
            lab.docker = Mock(side_effect=[
                (name + "\n").encode(),
                json.dumps([{"Name": name, "Driver": "local", "Options": None,
                             "Labels": {"zpr.simulator.identity-lab": lab.token}}]).encode(),
                b""])
            lab.cleanup()
            lab.docker.assert_any_call("volume", "rm", name)


if __name__ == "__main__":
    unittest.main()
