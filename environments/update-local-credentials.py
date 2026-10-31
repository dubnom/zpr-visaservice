#!/usr/bin/env python3
import json
import os
import subprocess
import tempfile
from pathlib import Path


REMOTE = os.environ.get("ZPR_ENV_REMOTE", "workbench.local")
STATE_ROOT = "/home/dubnom/zpr-demo-refresh-20261029-r1"
SECRETS = {
    "dev": {
        "state": "state-dev-20261029-r4",
        "operator": "operator-admin.secret",
        "logger": "logging-admin.secret",
        "ldapAdmin": "ldap-admin.secret",
        "ldapReader": "ldap-reader.secret",
    },
    "stable": {
        "state": "state-scenario-r6",
        "operator": "operator-admin.secret",
        "logger": "logging-admin.secret",
        "ldapAdmin": "ldap-admin.secret",
        "ldapReader": "ldap-reader.secret",
    },
}


def read_remote_secret(path: str) -> str:
    if REMOTE == "local":
        secret_path = Path(path)
        if secret_path.is_symlink() or not secret_path.is_file():
            raise ValueError(f"Refusing an unsafe secret file: {path}")
        if (secret_path.stat().st_mode & 0o777) != 0o600:
            raise ValueError(f"Refusing a secret without mode 0600: {path}")
        value = secret_path.read_text(encoding="utf-8").strip()
    else:
        result = subprocess.run(
            ["ssh", "-o", "BatchMode=yes", "-T", REMOTE, "cat", "--", path],
            check=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        value = result.stdout.decode("utf-8").strip()
    if not value or len(value) > 4096 or any(ord(char) < 32 or ord(char) == 127 for char in value):
        raise ValueError(f"Refusing an invalid secret file: {path}")
    return value


def main() -> None:
    credentials = {}
    for environment, files in SECRETS.items():
        state = files["state"]
        credentials[environment] = {
            key: read_remote_secret(f"{STATE_ROOT}/{state}/{filename}")
            for key, filename in files.items()
            if key != "state"
        }

    output = Path(__file__).with_name("credentials.local.js")
    payload = (
        "window.ZPR_ENV_CREDENTIALS = "
        + json.dumps(credentials, ensure_ascii=True, separators=(",", ":"))
        + ";\n"
    ).encode("utf-8")

    descriptor, temporary_name = tempfile.mkstemp(
        prefix=".credentials.local.", dir=output.parent
    )
    try:
        os.fchmod(descriptor, 0o600)
        with os.fdopen(descriptor, "wb") as temporary:
            temporary.write(payload)
            temporary.flush()
            os.fsync(temporary.fileno())
        os.replace(temporary_name, output)
        os.chmod(output, 0o600)
    except Exception:
        try:
            os.unlink(temporary_name)
        except FileNotFoundError:
            pass
        raise

    print(f"Updated private local credential data at {output} (mode 0600).")


if __name__ == "__main__":
    main()
