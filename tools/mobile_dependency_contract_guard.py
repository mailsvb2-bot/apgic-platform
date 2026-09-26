#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
PACKAGE = ROOT / "apps/mobile/package.json"
REGISTRY = ROOT / "apps/mobile/dependency-registry.yaml"
SCHEMA = ROOT / "contracts/jsonschema/mobile-dependency-registry-v1.schema.json"

REQUIRED_ENTRY_FIELDS = {
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
}


def validate_mobile_dependency_contract(package_doc: dict, registry_doc: dict, schema_doc: dict) -> list[str]:
    errors: list[str] = []
    declared: dict[str, str] = {}
    for section in ("dependencies", "devDependencies"):
        declared.update(package_doc.get(section) or {})

    item = schema_doc.get("properties", {}).get("dependencies", {}).get("items", {})
    schema_required = set(item.get("required", []))
    if schema_required != REQUIRED_ENTRY_FIELDS:
        errors.append(f"dependency contract required fields differ: {sorted(schema_required)}")
    if item.get("additionalProperties") is not False or schema_doc.get("additionalProperties") is not False:
        errors.append("dependency registry contract must reject undeclared fields")

    policy = schema_doc.get("properties", {}).get("policy", {}).get("properties", {})
    if policy.get("unknown_dependency", {}).get("const") != "BLOCK":
        errors.append("unknown dependency policy must BLOCK")
    if policy.get("wildcard_dependency", {}).get("const") != "BLOCK":
        errors.append("wildcard dependency policy must BLOCK")
    if policy.get("runtime_dependency_requires_privacy_review", {}).get("const") is not True:
        errors.append("runtime dependencies must require privacy review")

    entries = registry_doc.get("dependencies") or []
    by_name = {entry.get("name"): entry for entry in entries if isinstance(entry, dict) and entry.get("name")}
    if set(by_name) != set(declared):
        errors.append(
            f"dependency registry/package graph differ: registry={sorted(by_name)} package={sorted(declared)}"
        )
    for name, version in declared.items():
        entry = by_name.get(name) or {}
        if entry.get("declared_version") != version:
            errors.append(f"{name}: registry/package version mismatch")
        missing = REQUIRED_ENTRY_FIELDS - set(entry)
        if missing:
            errors.append(f"{name}: dependency registry fields missing: {sorted(missing)}")
        if version.strip() in {"*", "latest", "next"}:
            errors.append(f"{name}: floating dependency version is forbidden")
        if entry.get("scope") == "RUNTIME":
            if entry.get("privacy_status") in {None, "", "UNKNOWN", "PENDING"}:
                errors.append(f"{name}: runtime dependency lacks privacy review")
            if entry.get("security_status") in {None, "", "UNKNOWN", "PENDING"}:
                errors.append(f"{name}: runtime dependency lacks security review")
    return errors


def main() -> int:
    errors = validate_mobile_dependency_contract(
        json.loads(PACKAGE.read_text(encoding="utf-8")),
        yaml.safe_load(REGISTRY.read_text(encoding="utf-8")) or {},
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("MOBILE DEPENDENCY CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("MOBILE DEPENDENCY CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
