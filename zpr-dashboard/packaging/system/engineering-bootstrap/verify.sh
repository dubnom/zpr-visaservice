#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
    echo "usage: sh verify.sh /path/to/extracted/zpr-engineering-bootstrap" >&2
    exit 2
fi
command -v python3 >/dev/null 2>&1 || {
    echo "FAIL: Preinstalled Python 3 is required; nothing will be installed." >&2
    exit 1
}

# Isolated Python uses only the standard library, never modules from the bundle.
exec python3 -I - "$1" <<'PY'
import hashlib
import json
import os
import re
import stat
import sys

PAYLOAD = (
    "README.md", "bootstrap.sh", "company.example.json",
    "manifest.json", "preflight.sh", "zpr-system-config",
)
EXECUTABLES = {"bootstrap.sh", "preflight.sh", "zpr-system-config"}
MANIFEST = {
    "schema_version": 1,
    "artifact": "zpr-engineering-bootstrap",
    "status": "UNSIGNED/UNQUALIFIED",
    "platform": "linux/amd64",
    "source_state": "tracked dashboard sources match HEAD; packaging files may be uncommitted",
    "purpose": "Read-only host preflight and offline management-bootstrap input validation only",
    "limitations": "No services, images, installation, signing, managed control plane or readiness certification",
}


def reject(message):
    raise ValueError(message)


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            reject("manifest contains duplicate fields")
        result[key] = value
    return result


def verify(directory):
    # normpath removes trailing slashes so O_NOFOLLOW also rejects a root symlink.
    root = os.open(os.path.normpath(directory), os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        if set(os.listdir(root)) != set(PAYLOAD) | {"SHA256SUMS"}:
            reject("bundle inventory must contain exactly the seven expected files")
        digests = {}
        documents = {}
        for name in (*PAYLOAD, "SHA256SUMS"):
            fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=root)
            with os.fdopen(fd, "rb") as source:
                metadata = os.fstat(source.fileno())
                if not stat.S_ISREG(metadata.st_mode) or metadata.st_nlink != 1:
                    reject("bundle members must be regular files without hard links")
                if name in EXECUTABLES and not metadata.st_mode & 0o111:
                    reject("required executable permission is missing")
                if name in {"manifest.json", "SHA256SUMS"}:
                    data = source.read(65537)
                    if len(data) > 65536:
                        reject("manifest or checksum inventory exceeds 64 KiB")
                    documents[name] = data
                    digests[name] = hashlib.sha256(data).hexdigest()
                else:
                    digest = hashlib.sha256()
                    for block in iter(lambda: source.read(1024 * 1024), b""):
                        digest.update(block)
                    digests[name] = digest.hexdigest()

        manifest = json.loads(documents["manifest.json"], object_pairs_hook=unique_object)
        if type(manifest) is not dict or set(manifest) != set(MANIFEST) | {"source_revision"}:
            reject("manifest fields do not match schema version 1")
        for field, expected in MANIFEST.items():
            if type(manifest[field]) is not type(expected) or manifest[field] != expected:
                reject("unsupported manifest field: " + field)
        revision = manifest["source_revision"]
        if type(revision) is not str or re.fullmatch(r"[0-9a-f]{40}", revision) is None:
            reject("source_revision must be a full lowercase Git SHA-1")

        checksums = {}
        for line in documents["SHA256SUMS"].decode("ascii").splitlines():
            match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9_.-]+)", line)
            if match is None:
                reject("invalid SHA256SUMS record")
            digest, name = match.groups()
            if name not in PAYLOAD or name in checksums:
                reject("unexpected or duplicate checksum member")
            checksums[name] = digest
        if set(checksums) != set(PAYLOAD):
            reject("SHA256SUMS must list each payload file exactly once")
        for name in PAYLOAD:
            if checksums[name] != digests[name]:
                reject("checksum mismatch: " + name)
        return revision
    finally:
        os.close(root)


try:
    revision = verify(sys.argv[1])
except (OSError, ValueError, UnicodeError, RecursionError):
    # Do not echo untrusted paths, manifest contents, or raw operating-system errors.
    print("FAIL: Bundle integrity verification rejected the input.", file=sys.stderr)
    sys.exit(1)

print("PASS: Exact inventory, manifest and SHA-256 checksums verified.")
print("UNSIGNED/UNQUALIFIED source revision: " + revision)
print("Integrity only, NOT authenticity, signing, installation or readiness certification.")
print("No payload was executed; no services were contacted or bundle files written.")
print("Use a private, quiescent extracted directory; concurrent replacement is not covered.")
PY
