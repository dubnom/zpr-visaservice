#!/usr/bin/env python3
"""Strict, read-only audit of known shared-command/launcher boundary seams.

No allowlist: current violations return nonzero. This does not execute Docker
or Simulator, and supplements (not replaces) Go/browser runtime guards.
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def violations(root=ROOT):
    findings = []
    launcher = root / "scripts/dashboard-stack.sh"
    text = launcher.read_text()
    for function in ("start_control_room", "start_control_service"):
        match = re.search(r"^" + function + r"\(\) \{\n(.*?)^\}", text, re.M | re.S)
        if not match:
            findings.append(f"{launcher.relative_to(root)}: missing audited function {function}; update audit scope")
            continue
        for offset, line in enumerate(match.group(1).splitlines()):
            number = text.count("\n", 0, match.start(1)) + offset + 1
            if re.search(r"SIMULATION_|SIMULATOR_|ACTIVE_ORGANIZATION_FILE|ORGANIZATIONS_DIR|ensure_simulator_image|control_organization_profile|/zpr-demo/", line):
                findings.append(f"{launcher.relative_to(root)}:{number}: {function}: {line.strip()}")
    inventory = root / "cmd/zpr-web-dashboard/inventory.go"
    for number, line in enumerate(inventory.read_text().splitlines(), 1):
        if re.search(r'"demo_ldap"|localLDAPEditorURL\(', line):
            findings.append(f"{inventory.relative_to(root)}:{number}: production inventory has demo-source special case: {line.strip()}")
    composition = root / "cmd/zpr-web-dashboard/control_service.go"
    if 'fs.Sub(controlRoomFiles, "static")' not in composition.read_text() or 'fs.Sub(staticFiles, "static")' in composition.read_text():
        findings.append(f"{composition.relative_to(root)}: production static handler must use its independent embedded filesystem")
    bundle = root / "cmd/zpr-web-dashboard/assets_control_room.go"
    manifest = root / "packaging/control-room/assets.txt"
    if not bundle.exists() or not manifest.exists():
        findings.append("production embedded asset bundle or packaging manifest is missing")
    else:
        embedded = re.findall(r"^//go:embed (.+)$", bundle.read_text(), re.M)
        names = [name.removeprefix("static/") for line in embedded for name in line.split()]
        expected = manifest.read_text().split()
        if sorted(names) != sorted(expected) or any("*" in name for name in names):
            findings.append("production embedded files differ from the explicit packaging asset manifest")
        for name in names:
            if re.search(r"^(?:simulator|scenario|organization|activity|agents|world|workers-redirect|ldap-org-graph|design-assistant)|^(?:machine-logs|trusted-source)\.html$", name):
                findings.append(f"production bundle includes Simulator-owned asset: {name}")
    return findings


def main():
    findings = violations()
    for finding in findings:
        print(finding)
    if findings:
        print(f"FAIL: {len(findings)} reverse-dependency references; see ../CONTROL_ROOM_BOUNDARY_INVENTORY.md", file=sys.stderr)
        return 1
    print("PASS: audited launcher/inventory seams have no Simulator references")
    return 0


if __name__ == "__main__":
    sys.exit(main())
