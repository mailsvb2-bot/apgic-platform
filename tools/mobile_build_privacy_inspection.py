#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import plistlib
import xml.etree.ElementTree as ET
from pathlib import Path

ANDROID_NS = "http://schemas.android.com/apk/res/android"


def fail(message: str) -> None:
    raise ValueError(message)


def parse_yaml_list(path: Path, key: str) -> list[str]:
    lines = path.read_text(encoding="utf-8").splitlines()
    marker = f"{key}:"
    for index, line in enumerate(lines):
        if line.strip() != marker:
            continue
        values: list[str] = []
        base_indent = len(line) - len(line.lstrip())
        for candidate in lines[index + 1 :]:
            if not candidate.strip():
                continue
            indent = len(candidate) - len(candidate.lstrip())
            stripped = candidate.strip()
            if indent <= base_indent:
                break
            if stripped.startswith("- "):
                values.append(stripped[2:].strip().strip('"').strip("'"))
        return values
    fail(f"{path}: missing YAML key {key}")
    return []


def inspect_ios(app: Path, canonical_manifest: Path) -> dict[str, object]:
    if not app.is_dir():
        fail(f"iOS app bundle missing: {app}")
    built_manifest = app / "PrivacyInfo.xcprivacy"
    if not built_manifest.is_file():
        fail(f"built iOS app is missing PrivacyInfo.xcprivacy: {built_manifest}")
    if not canonical_manifest.is_file():
        fail(f"canonical Apple privacy manifest missing: {canonical_manifest}")

    with built_manifest.open("rb") as handle:
        built = plistlib.load(handle)
    with canonical_manifest.open("rb") as handle:
        canonical = plistlib.load(handle)
    if built != canonical:
        fail("built iOS PrivacyInfo.xcprivacy differs from canonical manifest")

    info_plist = app / "Info.plist"
    if not info_plist.is_file():
        fail("built iOS app is missing Info.plist")
    with info_plist.open("rb") as handle:
        info = plistlib.load(handle)

    return {
        "platform": "IOS",
        "artifact": str(app),
        "privacy_manifest_present": True,
        "privacy_manifest_matches_canonical": True,
        "bundle_identifier": info.get("CFBundleIdentifier", ""),
        "tracking": built.get("NSPrivacyTracking"),
        "required_reason_api_count": len(built.get("NSPrivacyAccessedAPITypes", [])),
    }


def inspect_android(merged_manifest: Path, data_safety: Path) -> dict[str, object]:
    if not merged_manifest.is_file():
        fail(f"Android merged manifest missing: {merged_manifest}")
    if not data_safety.is_file():
        fail(f"Android Data Safety declaration missing: {data_safety}")

    root = ET.parse(merged_manifest).getroot()
    permission_key = f"{{{ANDROID_NS}}}name"
    actual_permissions = sorted(
        item.get(permission_key)
        for item in root.findall("uses-permission")
        if item.get(permission_key)
    )
    declared_permissions = sorted(parse_yaml_list(data_safety, "android_permissions"))
    if actual_permissions != declared_permissions:
        fail(
            "built Android permissions differ from Data Safety declaration: "
            f"actual={actual_permissions} declared={declared_permissions}"
        )

    application = root.find("application")
    if application is None:
        fail("Android merged manifest has no application element")
    allow_backup = application.get(f"{{{ANDROID_NS}}}allowBackup")
    if allow_backup != "false":
        fail(f"built Android manifest must set allowBackup=false, got {allow_backup!r}")

    return {
        "platform": "ANDROID",
        "artifact": str(merged_manifest),
        "permissions": actual_permissions,
        "permissions_match_data_safety": True,
        "allow_backup": False,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--ios-app", type=Path)
    group.add_argument("--android-manifest", type=Path)
    parser.add_argument("--canonical-ios-manifest", type=Path)
    parser.add_argument("--android-data-safety", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    if args.ios_app is not None:
        if args.canonical_ios_manifest is None:
            parser.error("--canonical-ios-manifest is required with --ios-app")
        result = inspect_ios(args.ios_app, args.canonical_ios_manifest)
    else:
        if args.android_data_safety is None:
            parser.error("--android-data-safety is required with --android-manifest")
        result = inspect_android(args.android_manifest, args.android_data_safety)

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(f"MOBILE BUILD PRIVACY INSPECTION: PASS ({result['platform']})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
