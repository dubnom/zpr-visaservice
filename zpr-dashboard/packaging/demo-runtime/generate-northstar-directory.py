#!/usr/bin/env python3
"""Generate the portable synthetic Northstar LDAP fixture from its profiles."""

import argparse
import base64
import json
import re
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
DASHBOARD_DIR = SCRIPT_DIR.parent.parent
PACKAGE_DIR = SCRIPT_DIR / "organizations"
EXAMPLES_DIR = DASHBOARD_DIR / "cmd" / "zpr-web-dashboard" / "examples"
EXAMPLE_LDIF_DIR = EXAMPLES_DIR / "organizations" / "northstar"
PACKAGE_PROFILE = PACKAGE_DIR / "northstar.json"
EXAMPLE_PROFILE = EXAMPLES_DIR / "organizations" / "northstar.json"
SIMULATION_MANIFEST = SCRIPT_DIR / "simulation-environment.json"
STATIC_ASSETS = ("README.md", "demo-policy-catalog.json", "policy-demo.zplc")


def attribute(name, value):
    text = str(value)
    raw = text.encode("utf-8")
    if (
        not text
        or text[0] in " :<"
        or text[-1] == " "
        or any(ord(char) < 0x20 or ord(char) > 0x7E for char in text)
    ):
        return f"{name}:: {base64.b64encode(raw).decode('ascii')}"
    return f"{name}: {text}"


def rdn(value):
    escaped = []
    for index, char in enumerate(value):
        if char in ',+"\\<>;=' or (index == 0 and char in " #") or (
            index == len(value) - 1 and char == " "
        ):
            escaped.append("\\")
        escaped.append(char)
    return "".join(escaped)


def entry(dn, fields):
    return "\n".join([f"dn: {dn}", *fields, ""])


def load_inputs():
    package_profile = json.loads(PACKAGE_PROFILE.read_text(encoding="utf-8"))
    example_profile = json.loads(EXAMPLE_PROFILE.read_text(encoding="utf-8"))
    if package_profile["directory"] != example_profile["directory"]:
        raise ValueError("Northstar package and example directory profiles differ")
    profile = package_profile
    directory = profile["directory"]
    manifest = json.loads(SIMULATION_MANIFEST.read_text(encoding="utf-8"))
    if not re.fullmatch(r"dc=[a-z0-9-]+(?:,dc=[a-z0-9-]+)*", directory["base_dn"]):
        raise ValueError("Northstar base_dn is not a canonical lowercase DC name")

    people = {person["uid"]: person for person in directory["people"]}
    groups = {group["name"]: group for group in directory["groups"]}
    if len(people) != len(directory["people"]) or len(groups) != len(directory["groups"]):
        raise ValueError("Northstar directory contains duplicate people or groups")
    if any(member not in people for group in groups.values() for member in group["members"]):
        raise ValueError("Northstar group references an unknown person")
    if any(machine.get("owner") and machine["owner"] not in people for machine in manifest["machines"]):
        raise ValueError("simulation manifest references an unknown Northstar machine owner")
    return profile, directory, manifest


def render_company(profile, directory):
    base = directory["base_dn"]
    lines = [entry(base, [
        "objectClass: top",
        "objectClass: dcObject",
        "objectClass: organization",
        attribute("dc", base.split(",", 1)[0][3:]),
        attribute("o", profile["name"]),
    ])]
    for name in ("People", "Machines", "Roles", "Departments", "Service Accounts"):
        lines.append(entry(f"ou={rdn(name)},{base}", [
            "objectClass: top",
            "objectClass: organizationalUnit",
            attribute("ou", name),
        ]))

    departments = {item["name"]: item for item in directory["departments"]}
    for department in directory["departments"]:
        parent = department.get("parent")
        if parent not in (None, *departments.keys()):
            raise ValueError(f"unknown Northstar parent department: {parent}")
        parent_dn = f"ou={rdn(parent)},ou=Departments,{base}" if parent else f"ou=Departments,{base}"
        lines.append(entry(f"ou={rdn(department['name'])},{parent_dn}", [
            "objectClass: top",
            "objectClass: organizationalUnit",
            attribute("ou", department["name"]),
        ]))
    return "\n".join(lines)


def render_employees(directory):
    base = directory["base_dn"]
    lines = []
    for person in directory["people"]:
        parts = person["name"].split()
        if len(parts) < 2:
            raise ValueError(f"Northstar person must have a given and family name: {person['uid']}")
        lines.append(entry(f"uid={rdn(person['uid'])},ou=People,{base}", [
            "objectClass: top",
            "objectClass: person",
            "objectClass: organizationalPerson",
            "objectClass: inetOrgPerson",
            attribute("uid", person["uid"]),
            attribute("cn", person["name"]),
            attribute("sn", parts[-1]),
            attribute("givenName", " ".join(parts[:-1])),
            attribute("mail", person["email"]),
            attribute("ou", person["department"]),
            attribute("title", person["title"]),
        ]))
    return "\n".join(lines)


def render_roles(directory):
    base = directory["base_dn"]
    lines = []
    for group in directory["groups"]:
        members = group["members"]
        if not members:
            raise ValueError(f"Northstar group must have at least one member: {group['name']}")
        lines.append(entry(f"cn={rdn(group['name'])},ou=Roles,{base}", [
            "objectClass: top",
            "objectClass: groupOfNames",
            attribute("cn", group["name"]),
            *(attribute("member", f"uid={rdn(member)},ou=People,{base}") for member in members),
        ]))
    return "\n".join(lines)


def render_machines(directory, manifest):
    base = directory["base_dn"]
    people = {person["uid"]: person for person in directory["people"]}
    location = manifest["machines"][0].get("location", "North Campus")
    if location == "Not provisioned":
        location = "North Campus"
    lines = []
    for machine in manifest["machines"]:
        fields = [
            "objectClass: top",
            "objectClass: inetOrgPerson",
            "objectClass: zprMachine",
            attribute("uid", machine["id"]),
            attribute("cn", machine["id"]),
            "sn: Endpoint",
            attribute("zprMachineId", machine["id"]),
            attribute("zprMachineType", machine["type"]),
            attribute("zprMachineModel", machine["model"]),
            attribute("zprMachineLocation", location),
            attribute("zprMachineSecure", "TRUE" if machine["secure"] else "FALSE"),
        ]
        if machine.get("owner"):
            owner = people[machine["owner"]]
            fields.extend([
                attribute("ou", owner["department"]),
                attribute("zprMachineOwner", machine["owner"]),
            ])
        lines.append(entry(f"uid={rdn(machine['id'])},ou=Machines,{base}", fields))
    lines.append(entry(f"uid={rdn(directory['health_uid'])},ou=Service Accounts,{base}", [
        "objectClass: top",
        "objectClass: account",
        attribute("uid", directory["health_uid"]),
    ]))
    return "\n".join(lines)


def generated_assets(profile, directory, manifest):
    names = directory["ldif_files"]
    expected = {
        "demo-company.ldif": render_company(profile, directory),
        "demo-employees.ldif": render_employees(directory),
        "demo-roles.ldif": render_roles(directory),
        "demo-machines.ldif": render_machines(directory, manifest),
    }
    if set(names) != set(expected):
        raise ValueError("Northstar LDIF profile does not match the generator output")
    return expected


def sync_file(source, destination, check):
    content = source.read_bytes()
    if check:
        if not destination.is_file() or destination.read_bytes() != content:
            raise ValueError(f"Northstar packaged asset is missing or stale: {destination}")
        return
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_bytes(content)


def sync_generated(destination, assets, check):
    if check:
        for name, content in assets.items():
            path = destination / name
            if not path.is_file() or path.read_text(encoding="utf-8") != content:
                raise ValueError(f"Northstar generated LDIF is missing or stale: {path}")
        return
    destination.mkdir(parents=True, exist_ok=True)
    for name, content in assets.items():
        (destination / name).write_text(content, encoding="utf-8")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="verify generated assets without writing")
    args = parser.parse_args()
    profile, directory, manifest = load_inputs()
    assets = generated_assets(profile, directory, manifest)
    example_target = EXAMPLE_LDIF_DIR
    example_assets = EXAMPLES_DIR / "northstar"
    package_target = PACKAGE_DIR / "northstar"
    for target in (example_target, package_target):
        sync_generated(target, assets, args.check)
    for name in STATIC_ASSETS:
        sync_file(example_assets / name, package_target / name, args.check)
    print("Northstar synthetic directory assets " + ("verified" if args.check else "generated"))


if __name__ == "__main__":
    try:
        main()
    except (OSError, KeyError, TypeError, ValueError, json.JSONDecodeError) as error:
        raise SystemExit(f"generate-northstar-directory: {error}") from error
