#!/usr/bin/env python3
from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]

REQUIRED_A11Y = {
    "voice_over",
    "talk_back",
    "text_scaling",
    "reduced_motion",
    "focus_order",
}
REQUIRED_DEVICE_CLASSES = {"PHONE_COMPACT", "PHONE_LARGE", "TABLET"}
REQUIRED_PHYSICAL_PATHS = {
    "CAMERA_MICROPHONE",
    "PUSH_BACKGROUND",
    "UNIVERSAL_APP_LINKS",
    "AUDIO_ROUTE",
}


def fail(message: str) -> None:
    print(f"MOBILE SUPPORT PRECHECK: FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def validate_policy(policy: dict, mode: str) -> None:
    version = policy.get("policy_version")
    if not isinstance(version, str) or not version.strip() or version == "CONFIG_REQUIRED":
        fail("policy_version must be explicit")

    if "device_model_allowlist" in policy:
        fail("device model allowlists are forbidden product logic")

    if mode == "production":
        if policy.get("scope") != "PRODUCTION":
            fail("production policy must have scope=PRODUCTION")
        if policy.get("production_approved") is not True:
            fail("production policy must be approved")
        if policy.get("production_evidence") is not True:
            fail("production policy requires production evidence")
    elif policy.get("scope") != "CI_ONLY":
        fail("CI policy must have scope=CI_ONLY")

    ios = policy.get("minimum_ios_major")
    android = policy.get("minimum_android_api")
    if not isinstance(ios, int) or isinstance(ios, bool) or ios <= 0:
        fail("minimum_ios_major must be a positive integer")
    if not isinstance(android, int) or isinstance(android, bool) or android <= 0:
        fail("minimum_android_api must be a positive integer")

    accessibility = policy.get("accessibility")
    if not isinstance(accessibility, dict):
        fail("accessibility policy is required")
    for key in REQUIRED_A11Y:
        if accessibility.get(key) is not True:
            fail(f"accessibility.{key} must be enabled")
    touch = accessibility.get("touch_target_min_dp")
    if not isinstance(touch, (int, float)) or isinstance(touch, bool) or touch <= 0:
        fail("accessibility.touch_target_min_dp must be positive")

    device_classes = set(policy.get("device_test_classes") or [])
    if not REQUIRED_DEVICE_CLASSES.issubset(device_classes):
        fail("representative device classes are incomplete")

    physical_paths = set(policy.get("targeted_physical_capability_paths") or [])
    if not REQUIRED_PHYSICAL_PATHS.issubset(physical_paths):
        fail("targeted physical capability paths are incomplete")

    evidence_refs = policy.get("evidence_refs")
    if not isinstance(evidence_refs, list):
        fail("evidence_refs must be a list")
    if mode == "production":
        prefixes = ("surface://IOS/", "surface://ANDROID/", "device-matrix://")
        missing = [
            prefix
            for prefix in prefixes
            if not any(isinstance(ref, str) and ref.startswith(prefix) for ref in evidence_refs)
        ]
        if missing:
            fail(f"production evidence refs missing prefixes: {missing}")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("policy")
    parser.add_argument("--mode", choices=("ci", "production"), required=True)
    args = parser.parse_args()

    path = (ROOT / args.policy).resolve()
    if ROOT not in path.parents or not path.is_file():
        fail("policy path missing or outside repository")
    document = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    if not isinstance(document, dict):
        fail("policy must be a mapping")
    validate_policy(document, args.mode)
    print(
        "MOBILE SUPPORT PRECHECK: PASS "
        f"(mode={args.mode}, policy={path.relative_to(ROOT)}, version={document['policy_version']})"
    )


if __name__ == "__main__":
    main()
