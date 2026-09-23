#!/usr/bin/env python3
from __future__ import annotations

import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
DEPENDENCIES = ROOT / "apps/mobile/dependency-registry.yaml"
DECLARATION = ROOT / "apps/mobile/privacy-declaration.yaml"

errors: list[str] = []

dependencies = yaml.safe_load(DEPENDENCIES.read_text(encoding="utf-8")) or {}
declaration = yaml.safe_load(DECLARATION.read_text(encoding="utf-8")) or {}

runtime_entries = {
    entry["name"]: entry
    for entry in dependencies.get("dependencies", [])
    if entry.get("scope") == "RUNTIME"
}
assertions = declaration.get("runtime_dependency_assertions") or {}

missing = sorted(set(runtime_entries) - set(assertions))
stale = sorted(set(assertions) - set(runtime_entries))

for name in missing:
    errors.append(f"runtime dependency lacks privacy assertion: {name}")
for name in stale:
    errors.append(f"privacy assertion references non-runtime dependency: {name}")

for name, entry in runtime_entries.items():
    assertion = assertions.get(name) or {}
    if assertion.get("egress") != entry.get("egress"):
        errors.append(
            f"{name}: privacy egress {assertion.get('egress')!r} "
            f"!= dependency registry {entry.get('egress')!r}"
        )
    if assertion.get("data_classes") != entry.get("data_classes"):
        errors.append(f"{name}: privacy data_classes differ from dependency registry")
    if assertion.get("permissions") != entry.get("permissions"):
        errors.append(f"{name}: privacy permissions differ from dependency registry")

declared_permissions = set(declaration.get("required_permissions") or [])
runtime_permissions = {
    permission
    for entry in runtime_entries.values()
    for permission in (entry.get("permissions") or [])
}
if declared_permissions != runtime_permissions:
    errors.append(
        f"required_permissions mismatch: declaration={sorted(declared_permissions)} "
        f"runtime={sorted(runtime_permissions)}"
    )

collected = set(declaration.get("collected_data_classes") or [])
shared = set(declaration.get("shared_data_classes") or [])
for forbidden in {"RAW_CONSULTATION", "RAW_PERSONA", "FINANCIAL_EVIDENCE"}:
    if forbidden in collected or forbidden in shared:
        errors.append(f"R0 native shell must not declare persistent collection/sharing of {forbidden}")

if declaration.get("tracking") not in {True, False}:
    errors.append("tracking must be explicit boolean")

store_artifacts = declaration.get("store_artifacts") or {}
apple = store_artifacts.get("apple_privacy_manifest") or {}
google = store_artifacts.get("google_data_safety") or {}

ios_present = (ROOT / "apps/mobile/ios").exists()
android_present = (ROOT / "apps/mobile/android").exists()

if ios_present and apple.get("status") == "NOT_YET_GENERATED":
    errors.append("iOS build graph exists but Apple privacy manifest is still NOT_YET_GENERATED")
if android_present and google.get("status") == "NOT_YET_GENERATED":
    errors.append("Android build graph exists but Google Data Safety artifact is still NOT_YET_GENERATED")

if apple.get("status") not in {"NOT_YET_GENERATED", "GENERATED_AND_VERIFIED"}:
    errors.append("invalid Apple privacy manifest status")
if google.get("status") not in {"NOT_YET_GENERATED", "GENERATED_AND_VERIFIED"}:
    errors.append("invalid Google Data Safety status")

if errors:
    print("MOBILE PRIVACY GUARD: FAIL")
    for error in errors:
        print(f"ERROR: {error}")
    sys.exit(1)

print(
    "MOBILE PRIVACY GUARD: PASS "
    f"(runtime_dependencies={len(runtime_entries)}, "
    f"ios_build_graph={ios_present}, android_build_graph={android_present})"
)
