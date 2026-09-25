#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]


def validate(root: Path, config: dict) -> list[str]:
    errors: list[str] = []
    applications = config.get("applications") or {}
    ios = applications.get("ios") or {}
    android = applications.get("android") or {}

    required = {
        "package": root / "apps/mobile/package.json",
        "android_gradle": root / "apps/mobile/android/app/build.gradle",
        "android_activity": root / "apps/mobile/android/app/src/main/java/com/apgic/ci/MainActivity.kt",
        "android_application": root / "apps/mobile/android/app/src/main/java/com/apgic/ci/MainApplication.kt",
        "android_wrapper": root / "apps/mobile/android/gradle/wrapper/gradle-wrapper.jar",
        "ios_project": root / "apps/mobile/ios/APGIC.xcodeproj/project.pbxproj",
        "ios_delegate": root / "apps/mobile/ios/APGIC/AppDelegate.swift",
        "ios_podfile": root / "apps/mobile/ios/Podfile",
        "entry": root / "apps/mobile/index.js",
    }
    for label, path in required.items():
        if not path.is_file():
            errors.append(f"{label} missing: {path.relative_to(root)}")
    if errors:
        return errors

    package = json.loads(required["package"].read_text(encoding="utf-8"))
    if (package.get("dependencies") or {}).get("react-native") != "0.87.1":
        errors.append("native build graph must remain pinned to react-native 0.87.1")

    gradle = required["android_gradle"].read_text(encoding="utf-8")
    expected_android = android.get("application_id")
    application_ids = re.findall(r'applicationId\s+"([^"]+)"', gradle)
    namespaces = re.findall(r'namespace\s+"([^"]+)"', gradle)
    if application_ids != [expected_android]:
        errors.append(f"Android applicationId {application_ids!r} != governance {expected_android!r}")
    if namespaces != [expected_android]:
        errors.append(f"Android namespace {namespaces!r} != governance {expected_android!r}")
    for forbidden in (
        "storePassword",
        "keyPassword",
        "keyAlias",
        "storeFile",
        "signingConfig signingConfigs.debug",
    ):
        if forbidden in gradle:
            errors.append(f"Android build graph contains forbidden repository signing material: {forbidden}")

    activity = required["android_activity"].read_text(encoding="utf-8")
    application = required["android_application"].read_text(encoding="utf-8")
    if "package com.apgic.ci" not in activity or 'getMainComponentName(): String = "APGIC"' not in activity:
        errors.append("Android activity identity/component mismatch")
    if "package com.apgic.ci" not in application:
        errors.append("Android application package mismatch")

    project = required["ios_project"].read_text(encoding="utf-8")
    expected_ios = ios.get("bundle_id")
    bundle_ids = sorted(set(value.strip().strip('"') for value in re.findall(r"PRODUCT_BUNDLE_IDENTIFIER = ([^;]+);", project)))
    if bundle_ids != [expected_ios]:
        errors.append(f"iOS bundle identifiers {bundle_ids!r} != governance {expected_ios!r}")
    for forbidden in ("DEVELOPMENT_TEAM =", "PROVISIONING_PROFILE", "CODE_SIGN_IDENTITY[sdk=iphoneos"):
        if forbidden in project:
            errors.append(f"iOS build graph contains repository signing assignment: {forbidden}")

    delegate = required["ios_delegate"].read_text(encoding="utf-8")
    podfile = required["ios_podfile"].read_text(encoding="utf-8")
    if 'withModuleName: "APGIC"' not in delegate:
        errors.append("iOS React Native module name is not APGIC")
    if "target 'APGIC' do" not in podfile:
        errors.append("iOS Podfile target is not APGIC")

    return errors


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("config")
    args = parser.parse_args()
    path = (ROOT / args.config).resolve()
    if ROOT not in path.parents or not path.is_file():
        print("MOBILE NATIVE BUILD GUARD: FAIL: config missing or outside repository", file=sys.stderr)
        raise SystemExit(1)
    config = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    errors = validate(ROOT, config)
    if errors:
        print("MOBILE NATIVE BUILD GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        raise SystemExit(1)
    print("MOBILE NATIVE BUILD GUARD: PASS")


if __name__ == "__main__":
    main()
