#!/usr/bin/env python3
from __future__ import annotations

import plistlib
import sys
import xml.etree.ElementTree as ET
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

if ios_present and apple.get("status") != "GENERATED_AND_VERIFIED":
    errors.append("iOS build graph requires a generated and verified Apple privacy manifest")
if android_present and google.get("status") != "GENERATED_AND_VERIFIED":
    errors.append("Android build graph requires a generated and verified Google Data Safety artifact")

if apple.get("status") not in {"NOT_YET_GENERATED", "GENERATED_AND_VERIFIED"}:
    errors.append("invalid Apple privacy manifest status")
if google.get("status") not in {"NOT_YET_GENERATED", "GENERATED_AND_VERIFIED"}:
    errors.append("invalid Google Data Safety status")


def resolve_artifact(row: dict, label: str) -> Path | None:
    raw = row.get("path")
    if not isinstance(raw, str) or not raw.strip():
        errors.append(f"{label} path is required")
        return None
    relative = Path(raw)
    if relative.is_absolute() or ".." in relative.parts:
        errors.append(f"{label} path escapes repository")
        return None
    resolved = (ROOT / relative).resolve()
    try:
        resolved.relative_to(ROOT)
    except ValueError:
        errors.append(f"{label} path escapes repository")
        return None
    if not resolved.is_file():
        errors.append(f"{label} artifact missing: {raw}")
        return None
    return resolved


if ios_present and apple.get("status") == "GENERATED_AND_VERIFIED":
    apple_path = resolve_artifact(apple, "Apple privacy manifest")
    if apple.get("production_submission") is not False:
        errors.append("repository Apple privacy manifest must not claim production submission")
    if apple_path is not None:
        with apple_path.open("rb") as handle:
            manifest = plistlib.load(handle)
        actual_reason_apis = sorted(
            item.get("NSPrivacyAccessedAPIType")
            for item in manifest.get("NSPrivacyAccessedAPITypes", [])
            if item.get("NSPrivacyAccessedAPIType")
        )
        declared_reason_apis = sorted(declaration.get("required_reason_apis") or [])
        if actual_reason_apis != declared_reason_apis:
            errors.append(
                "Apple required-reason API declaration differs from committed manifest"
            )
        if manifest.get("NSPrivacyTracking") is not declaration.get("tracking"):
            errors.append("Apple tracking declaration differs from privacy declaration")
        if declaration.get("collected_data_classes") == [] and manifest.get("NSPrivacyCollectedDataTypes") != []:
            errors.append("Apple manifest declares collected data absent from privacy declaration")

if android_present and google.get("status") == "GENERATED_AND_VERIFIED":
    google_path = resolve_artifact(google, "Google Data Safety")
    if google.get("production_submission") is not False:
        errors.append("repository Google Data Safety artifact must not claim production submission")
    if google_path is not None:
        data_safety = yaml.safe_load(google_path.read_text(encoding="utf-8")) or {}
        for key in ("tracking", "collected_data_classes", "shared_data_classes"):
            if data_safety.get(key) != declaration.get(key):
                errors.append(f"Google Data Safety {key} differs from privacy declaration")

        manifest_path = ROOT / "apps/mobile/android/app/src/main/AndroidManifest.xml"
        if not manifest_path.is_file():
            errors.append("Android manifest missing from native build graph")
        else:
            root = ET.parse(manifest_path).getroot()
            permission_key = "{http://schemas.android.com/apk/res/android}name"
            manifest_permissions = sorted(
                item.get(permission_key)
                for item in root.findall("uses-permission")
                if item.get(permission_key)
            )
            declared_android_permissions = sorted(data_safety.get("android_permissions") or [])
            if manifest_permissions != declared_android_permissions:
                errors.append("Google Data Safety permissions differ from Android manifest")

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
