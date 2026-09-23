#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
PACKAGE = ROOT / "apps/mobile/package.json"
REGISTRY = ROOT / "apps/mobile/dependency-registry.yaml"

errors: list[str] = []

package = json.loads(PACKAGE.read_text(encoding="utf-8"))
registry = yaml.safe_load(REGISTRY.read_text(encoding="utf-8")) or {}

declared: dict[str, str] = {}
for section in ("dependencies", "devDependencies"):
    for name, version in (package.get(section) or {}).items():
        declared[name] = version

entries = registry.get("dependencies") or []
by_name: dict[str, dict] = {}

required_fields = (
    "name",
    "declared_version",
    "scope",
    "purpose",
    "data_classes",
    "egress",
    "permissions",
    "provenance",
    "privacy_status",
    "security_status",
    "removal_strategy",
    "kill_strategy",
)

for entry in entries:
    name = entry.get("name")
    if not isinstance(name, str) or not name:
        errors.append("dependency registry entry missing name")
        continue
    if name in by_name:
        errors.append(f"duplicate dependency registry entry: {name}")
        continue
    by_name[name] = entry

    for field in required_fields:
        if field not in entry:
            errors.append(f"{name}: missing {field}")

    if entry.get("declared_version") != declared.get(name):
        errors.append(
            f"{name}: registry version {entry.get('declared_version')!r} "
            f"!= package.json {declared.get(name)!r}"
        )

    if entry.get("scope") not in {"RUNTIME", "BUILD_ONLY"}:
        errors.append(f"{name}: invalid scope {entry.get('scope')!r}")

    if entry.get("scope") == "RUNTIME":
        if entry.get("privacy_status") in {None, "", "UNKNOWN", "PENDING"}:
            errors.append(f"{name}: runtime dependency lacks privacy review")
        if entry.get("security_status") in {None, "", "UNKNOWN", "PENDING"}:
            errors.append(f"{name}: runtime dependency lacks security review")

    if not isinstance(entry.get("data_classes"), list):
        errors.append(f"{name}: data_classes must be a list")
    if not isinstance(entry.get("permissions"), list):
        errors.append(f"{name}: permissions must be a list")

missing = sorted(set(declared) - set(by_name))
stale = sorted(set(by_name) - set(declared))

for name in missing:
    errors.append(f"unregistered mobile dependency: {name}")
for name in stale:
    errors.append(f"stale mobile dependency registry entry: {name}")

for name, version in declared.items():
    if version.strip() in {"*", "latest", "next"}:
        errors.append(f"{name}: floating dependency version is forbidden")

if errors:
    print("MOBILE DEPENDENCY GUARD: FAIL")
    for error in errors:
        print(f"ERROR: {error}")
    sys.exit(1)

print(f"MOBILE DEPENDENCY GUARD: PASS ({len(declared)} dependencies registered)")
