#!/usr/bin/env python3
"""Offline separate operator-image checks; no Docker commands or binary execution.

Run without arguments for synthetic regressions, or use:
  python3 test-operator-image-contract.py --image-archive IMAGE.tar

The archive must be a single-image Docker save archive. This checks the supplied
scratch-image contract, not CLI mode behavior, dynamic linking, or live services.
"""

import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path, PurePosixPath
import sys
import tarfile
import tempfile
import unittest


BINARY = "usr/local/bin/zpr-web-dashboard"
ENTRYPOINT = ["/" + BINARY]
MODES = ("control-room", "control-service")


def digest(data):
    return "sha256:" + hashlib.sha256(data).hexdigest()


def archive_members(archive):
    members = {}
    for member in archive.getmembers():
        name = member.name
        while name.startswith("./"):
            name = name[2:]
        name = name.rstrip("/")
        if not name and member.isdir():
            continue
        if (not name or name.startswith("/") or "\\" in name
                or any(part in ("", ".", "..") for part in name.split("/"))):
            raise ValueError(f"unsafe archive path: {member.name!r}")
        if name in members:
            raise ValueError(f"duplicate archive path: {name}")
        if not (member.isfile() or member.isdir()):
            raise ValueError(f"unsupported link or special archive member: {name}")
        members[name] = member
    return members


def read_member(archive, members, name):
    member = members.get(name)
    if member is None or not member.isfile():
        raise ValueError(f"missing regular archive member: {name}")
    with archive.extractfile(member) as stream:
        return stream.read()


def validate_image(path):
    with tarfile.open(path, "r:*") as archive:
        members = archive_members(archive)
        manifest = json.loads(read_member(archive, members, "manifest.json"))
        if (not isinstance(manifest, list) or len(manifest) != 1
                or not isinstance(manifest[0], dict)):
            raise ValueError("operator archive must contain exactly one image")
        entry = manifest[0]
        config_path = entry.get("Config")
        if not isinstance(config_path, str):
            raise ValueError("image config path is required")
        raw_config = read_member(archive, members, config_path)
        config_id = digest(raw_config)
        if PurePosixPath(config_path).name not in (
                config_id[7:], config_id[7:] + ".json"):
            raise ValueError("image config digest does not match its path")
        config = json.loads(raw_config)
        if not isinstance(config, dict):
            raise ValueError("image config must be an object")
        if (config.get("os"), config.get("architecture")) != ("linux", "amd64"):
            raise ValueError("operator image must target linux/amd64")
        settings = config.get("config")
        if not isinstance(settings, dict):
            raise ValueError("image runtime config is required")
        if settings.get("Entrypoint") != ENTRYPOINT:
            raise ValueError("operator entrypoint must be /usr/local/bin/zpr-web-dashboard")
        if settings.get("User") != "1000:1000":
            raise ValueError("operator image user must be 1000:1000")
        if settings.get("Cmd") not in (None, []):
            raise ValueError("operator image must not inject default mode arguments")
        layers = entry.get("Layers")
        rootfs = config.get("rootfs")
        if (not isinstance(layers, list) or not layers
                or not all(isinstance(layer, str) for layer in layers)
                or not isinstance(rootfs, dict) or rootfs.get("type") != "layers"
                or not isinstance(rootfs.get("diff_ids"), list)
                or len(rootfs["diff_ids"]) != len(layers)):
            raise ValueError("image layers and rootfs digests must agree")
        files = {}
        for layer_path, expected_digest in zip(layers, rootfs["diff_ids"]):
            raw = read_member(archive, members, layer_path)
            if raw.startswith(b"\x1f\x8b"):
                raw = gzip.decompress(raw)
            if digest(raw) != expected_digest:
                raise ValueError("image layer digest mismatch")
            with tarfile.open(fileobj=io.BytesIO(raw), mode="r:") as layer:
                layer_members = archive_members(layer)
                for name, member in layer_members.items():
                    base = PurePosixPath(name).name
                    if base.startswith(".wh."):
                        raise ValueError("whiteouts are outside the scratch COPY contract")
                    if member.isfile():
                        files[name] = (member.mode, read_member(layer, layer_members, name))
                    else:
                        files.pop(name, None)
        if set(files) != {BINARY}:
            raise ValueError("scratch operator payload must contain only " + BINARY)
        mode, binary = files[BINARY]
        if mode & 0o111 != 0o111 or mode & 0o6000:
            raise ValueError("operator binary must be executable by UID 1000 without set-ID bits")
        if (len(binary) < 20 or binary[:7] != b"\x7fELF\x02\x01\x01"
                or int.from_bytes(binary[18:20], "little") != 62):
            raise ValueError("operator binary must be an ELF64 little-endian x86-64 payload")
    return {
        "image_config_digest": config_id,
        "platform": "linux/amd64",
        "entrypoint": ENTRYPOINT,
        "user": "1000:1000",
        "binary_sha256": digest(binary),
        "mode_invocations": [ENTRYPOINT + ["-mode", mode] for mode in MODES],
        "mode_execution_verified": False,
    }


def tar_bytes(entries):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w") as archive:
        for name, data, mode, kind in entries:
            member = tarfile.TarInfo(name)
            member.mode = mode
            member.type = kind
            member.size = len(data) if kind == tarfile.REGTYPE else 0
            if kind == tarfile.SYMTYPE:
                member.linkname = "/app/bin/zpr-web-dashboard"
            archive.addfile(member, io.BytesIO(data) if member.isfile() else None)
    return output.getvalue()


class OperatorImageContractTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name) / "image.tar"
        self.binary = b"\x7fELF\x02\x01\x01" + bytes(11) + b"\x3e\x00"

    def fixture(self, *, binary_path=BINARY, binary=None, mode=0o755,
                kind=tarfile.REGTYPE, entrypoint=None, user="1000:1000",
                arch="amd64", cmd=None, bad_layer=False, bad_config=False,
                count=1, extra=False, compressed=False):
        payload = [(binary_path, self.binary if binary is None else binary, mode, kind)]
        if extra:
            payload.append(("app/bin/ph", b"component", 0o755, tarfile.REGTYPE))
        layer = tar_bytes(payload)
        config = {
            "os": "linux", "architecture": arch,
            "config": {"Entrypoint": ENTRYPOINT if entrypoint is None else entrypoint,
                       "User": user, "Cmd": cmd},
            "rootfs": {"type": "layers", "diff_ids": [
                "sha256:" + "0" * 64 if bad_layer else digest(layer)]},
        }
        raw = json.dumps(config).encode()
        config_path = ("0" * 64 if bad_config else digest(raw)[7:]) + ".json"
        manifest = [{"Config": config_path, "Layers": ["layer.tar"]}] * count
        self.path.write_bytes(tar_bytes([
            ("manifest.json", json.dumps(manifest).encode(), 0o644, tarfile.REGTYPE),
            (config_path, raw, 0o644, tarfile.REGTYPE),
            ("layer.tar", gzip.compress(layer) if compressed else layer,
             0o644, tarfile.REGTYPE),
        ]))
        return self.path

    def test_separate_operator_payload_and_mode_argument_vectors(self):
        result = validate_image(self.fixture())
        self.assertEqual(result["mode_invocations"], [
            ["/usr/local/bin/zpr-web-dashboard", "-mode", "control-room"],
            ["/usr/local/bin/zpr-web-dashboard", "-mode", "control-service"],
        ])
        self.assertFalse(result["mode_execution_verified"])

    def test_compressed_layer(self):
        self.assertEqual(validate_image(self.fixture(compressed=True))["platform"],
                         "linux/amd64")

    def test_zpr_component_must_not_be_selected_as_operator_image(self):
        with self.assertRaisesRegex(ValueError, "scratch operator payload"):
            validate_image(self.fixture(binary_path="app/bin/zpr-web-dashboard"))

    def test_rejects_missing_nonexecutable_or_wrong_architecture_payload(self):
        for changes in ({"binary_path": "app/bin/ph"}, {"mode": 0o644},
                        {"mode": 0o4755}, {"binary": b"not an ELF"},
                        {"binary": self.binary[:18] + b"\xb7\x00"}):
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                validate_image(self.fixture(**changes))

    def test_rejects_wrong_entrypoint_user_platform_and_default_mode(self):
        for changes in ({"entrypoint": ["/app/bin/zpr-web-dashboard"]},
                        {"entrypoint": ["/bin/sh", "-c"]}, {"user": "0:0"},
                        {"arch": "arm64"}, {"cmd": ["-mode", "simulator"]}):
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                validate_image(self.fixture(**changes))

    def test_rejects_symlink_traversal_and_component_payload(self):
        for changes in ({"kind": tarfile.SYMTYPE},
                        {"binary_path": "../" + BINARY}, {"extra": True}):
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                validate_image(self.fixture(**changes))

    def test_rejects_tampered_or_ambiguous_image(self):
        for changes in ({"bad_layer": True}, {"bad_config": True}, {"count": 2}):
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                validate_image(self.fixture(**changes))


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "--image-archive":
        parser = argparse.ArgumentParser(description=__doc__)
        parser.add_argument("--image-archive", type=Path, required=True)
        args = parser.parse_args()
        try:
            print(json.dumps(validate_image(args.image_archive), indent=2))
        except (ValueError, OSError, tarfile.TarError, EOFError) as exc:
            parser.exit(1, f"operator image contract: {exc}\n")
    else:
        unittest.main()
