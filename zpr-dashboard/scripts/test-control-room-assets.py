#!/usr/bin/env python3
"""Mutation checks for the strict production asset boundary audit (no services)."""
import importlib.util
from pathlib import Path
import shutil
import unittest
import uuid

SCRIPT = Path(__file__).with_name("check-control-room-boundary.py")
spec = importlib.util.spec_from_file_location("boundary_audit", SCRIPT)
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)


class ProductionAssetAuditTests(unittest.TestCase):
    def setUp(self):
        self.root = Path.cwd() / (".asset-audit-test-" + uuid.uuid4().hex)
        self.root.mkdir()
        self.addCleanup(shutil.rmtree, self.root)
        for name in ("cmd/zpr-web-dashboard/assets_control_room.go", "packaging/control-room/assets.txt"):
            target = self.root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text((audit.ROOT / name).read_text())
        self.write("scripts/dashboard-stack.sh", "start_control_room() {\n}\nstart_control_service() {\n}\n")
        self.write("cmd/zpr-web-dashboard/inventory.go", "package main\n")
        self.write("cmd/zpr-web-dashboard/control_service.go", 'fs.Sub(controlRoomFiles, "static")\n')

    def write(self, name, content):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)

    def test_clean_asset_boundary(self):
        self.assertEqual(audit.violations(self.root), [])

    def test_unfiltered_filesystem_is_rejected(self):
        self.write("cmd/zpr-web-dashboard/control_service.go", 'fs.Sub(staticFiles, "static")\n')
        self.assertTrue(any("independent embedded filesystem" in item for item in audit.violations(self.root)))

    def test_simulator_asset_cannot_be_admitted_by_manifest(self):
        bundle = self.root / "cmd/zpr-web-dashboard/assets_control_room.go"
        bundle.write_text(bundle.read_text() + "\n//go:embed static/scenarios.html\n")
        manifest = self.root / "packaging/control-room/assets.txt"
        manifest.write_text(manifest.read_text() + "scenarios.html\n")
        self.assertTrue(any("Simulator-owned asset: scenarios.html" in item for item in audit.violations(self.root)))

    def test_wildcard_and_manifest_drift_are_rejected(self):
        for content in ("//go:embed static/*\n", "//go:embed static/index.html\n"):
            with self.subTest(content=content):
                self.write("cmd/zpr-web-dashboard/assets_control_room.go", content)
                self.assertTrue(any("explicit packaging asset manifest" in item for item in audit.violations(self.root)))

    def test_missing_audit_scope_is_not_skipped(self):
        self.write("scripts/dashboard-stack.sh", "")
        self.assertEqual(sum("missing audited function" in item for item in audit.violations(self.root)), 2)


if __name__ == "__main__":
    unittest.main()
