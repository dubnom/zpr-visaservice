#!/bin/sh
set -eu

directory=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
command -v python3 >/dev/null 2>&1 || {
    echo "FAIL: Preinstalled Python 3 is required; nothing will be installed." >&2
    exit 1
}
exec python3 -I - "$directory/verify.sh" <<'PY'
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import tempfile

verifier = sys.argv[1]
payload = (
    "README.md", "bootstrap.sh", "company.example.json",
    "manifest.json", "preflight.sh", "zpr-system-config",
)
manifest = {
    "schema_version": 1,
    "artifact": "zpr-engineering-bootstrap",
    "status": "UNSIGNED/UNQUALIFIED",
    "platform": "linux/amd64",
    "source_revision": "eb17e3cfa87365e36dcd83b76643ee9802816b93",
    "source_state": "tracked dashboard sources match HEAD; packaging files may be uncommitted",
    "purpose": "Read-only host preflight and offline management-bootstrap input validation only",
    "limitations": "No services, images, installation, signing, managed control plane or readiness certification",
}
count = 0


def seal(bundle):
    (bundle / "SHA256SUMS").write_text("".join(
        hashlib.sha256((bundle / name).read_bytes()).hexdigest() + "  " + name + "\n"
        for name in payload
    ), encoding="ascii")


def snapshot(bundle):
    return {
        path.name: (path.read_bytes(), stat.S_IMODE(path.stat().st_mode), path.stat().st_mtime_ns)
        for path in bundle.iterdir()
    }


def check(label, arguments, status, text):
    global count
    result = subprocess.run(
        ["sh", verifier, *map(str, arguments)], capture_output=True, text=True, timeout=10,
        env={**os.environ, "PYTHONPATH": str(root), "PYTHONSTARTUP": str(marker)},
    )
    assert result.returncode == status, (label, result.returncode, result.stdout, result.stderr)
    assert text in result.stdout + result.stderr, (label, result.stdout, result.stderr)
    if status:
        assert "PASS:" not in result.stdout, label
    assert not marker.exists(), "Verifier executed payload or imported a fixture module"
    count += 1
    print("PASS: " + label)


with tempfile.TemporaryDirectory(prefix="zpr-verify-tests-") as temporary:
    root = Path(temporary)
    marker = root / "payload-was-executed"
    # Neither an executable member nor a Python import override may run.
    (root / "hashlib.py").write_text(f"open({str(marker)!r}, 'w').close()\n")
    original = root / "original bundle"
    original.mkdir()
    for name in payload:
        (original / name).write_text(f"#!/bin/sh\n: > '{marker}'\nexit 99\n")
        (original / name).chmod(0o755 if name in {
            "bootstrap.sh", "preflight.sh", "zpr-system-config"} else 0o644)
    (original / "manifest.json").write_text(json.dumps(manifest))
    seal(original)
    before = snapshot(original)
    check("valid synthetic bundle at a path with spaces", [original], 0, "Integrity only")
    check("success explicitly disclaims qualification", [original], 0, "UNSIGNED/UNQUALIFIED")
    assert snapshot(original) == before, "Verifier modified the bundle"
    check("missing argument", [], 2, "usage:")
    check("extra argument", [original, original], 2, "usage:")
    check("absent directory", [root / "absent"], 1, "FAIL:")
    check("file as root", [original / "README.md"], 1, "FAIL:")
    link = root / "linked-root"
    link.symlink_to(original, target_is_directory=True)
    check("symlink root", [link], 1, "FAIL:")
    check("symlink root with trailing slash", [str(link) + "/"], 1, "FAIL:")

    def case(label, change, reseal=False):
        bundle = root / "case"
        if bundle.exists():
            shutil.rmtree(bundle)
        shutil.copytree(original, bundle)
        change(bundle)
        if reseal:
            seal(bundle)
        check(label, [bundle], 1, "FAIL:")

    case("missing member", lambda b: (b / "README.md").unlink())
    case("extra hidden member", lambda b: (b / ".extra").write_text("extra"))
    case("extra directory", lambda b: (b / "extra").mkdir())

    def replace(bundle, kind):
        member = bundle / "bootstrap.sh"
        member.unlink()
        if kind == "symlink":
            member.symlink_to(original / "bootstrap.sh")
        elif kind == "hardlink":
            os.link(original / "bootstrap.sh", member)
        elif kind == "fifo":
            os.mkfifo(member)
        else:
            member.mkdir()

    for kind in ("symlink", "hardlink", "fifo", "directory"):
        case(kind + " payload", lambda b, kind=kind: replace(b, kind))
    case("missing executable bit", lambda b: (b / "bootstrap.sh").chmod(0o644))
    case("tampered payload", lambda b: (b / "README.md").write_text("tampered"))
    case("malformed JSON with matching checksum", lambda b: (b / "manifest.json").write_text("{"), True)
    case("non-object manifest", lambda b: (b / "manifest.json").write_text("[]"), True)
    case("duplicate manifest key", lambda b: (b / "manifest.json").write_text(
        json.dumps(manifest)[:-1] + ', "status": "UNSIGNED/UNQUALIFIED"}'), True)
    case("oversized manifest", lambda b: (b / "manifest.json").write_text(" " * 65537), True)
    case("deeply nested JSON", lambda b: (b / "manifest.json").write_text("[" * 2000 + "]" * 2000), True)

    for field, value in (
        ("schema_version", True), ("schema_version", 2),
        ("artifact", "production"), ("status", "SIGNED"),
        ("platform", "linux/arm64"), ("source_revision", "short"),
        ("source_revision", 42), ("source_state", "clean"),
        ("purpose", "install"), ("limitations", ""),
        ("unexpected", "field"),
    ):
        case("invalid manifest " + field + "=" + str(value), lambda b, field=field, value=value:
             (b / "manifest.json").write_text(json.dumps({**manifest, field: value})), True)
    case("missing manifest field", lambda b: (b / "manifest.json").write_text(
        json.dumps({k: v for k, v in manifest.items() if k != "source_revision"})), True)

    def checksum_edit(bundle, edit):
        path = bundle / "SHA256SUMS"
        path.write_text(edit(path.read_text()), encoding="ascii")

    for label, edit in (
        ("duplicate checksum", lambda s: s + s.splitlines()[0] + "\n"),
        ("missing checksum", lambda s: "\n".join(s.splitlines()[1:]) + "\n"),
        ("path traversal checksum", lambda s: s.replace("  README.md", "  ../README.md")),
        ("absolute checksum path", lambda s: s.replace("  README.md", "  /README.md")),
        ("unknown checksum member", lambda s: s.replace("  README.md", "  unexpected")),
        ("checksum file self-reference", lambda s: s.replace("  README.md", "  SHA256SUMS")),
        ("malformed digest", lambda s: "z" + s[1:]),
        ("wrong digest", lambda s: ("0" if s[0] != "0" else "1") + s[1:]),
        ("empty checksum inventory", lambda s: ""),
        ("oversized checksum inventory", lambda s: "0" * 65537),
    ):
        case(label, lambda b, edit=edit: checksum_edit(b, edit))
    case("non-ASCII checksum inventory", lambda b: (b / "SHA256SUMS").write_bytes(b"\xff"))
    assert snapshot(original) == before, "Original synthetic bundle was changed"

print(f"All {count} offline verifier cases passed; temporary fixtures removed.")
print("Synthetic integrity evidence only; no payload execution, builds or service contact.")
PY
