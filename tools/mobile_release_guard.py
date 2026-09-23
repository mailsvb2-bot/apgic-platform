#!/usr/bin/env python3
from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
FORBIDDEN_SECRET_KEYS = {"password", "private_key", "private_key_pem", "token", "secret_value"}

def fail(message: str) -> None:
    print(f"MOBILE RELEASE GUARD: FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)

def contains_config_required(value: object) -> bool:
    if isinstance(value, str):
        return value.strip() == "CONFIG_REQUIRED"
    if isinstance(value, dict):
        return any(contains_config_required(item) for item in value.values())
    if isinstance(value, list):
        return any(contains_config_required(item) for item in value)
    return False

def scan_secret_keys(value: object, path: str = "") -> None:
    if isinstance(value, dict):
        for key, item in value.items():
            if key.lower() in FORBIDDEN_SECRET_KEYS:
                fail(f"secret material key is forbidden in repository config: {path}{key}")
            scan_secret_keys(item, f"{path}{key}.")
    elif isinstance(value, list):
        for index, item in enumerate(value):
            scan_secret_keys(item, f"{path}{index}.")

def require_ref(value: object, scheme: str, field: str) -> None:
    if not isinstance(value, str) or not value.startswith(scheme):
        fail(f"{field} must use external reference scheme {scheme}")

def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("config")
    parser.add_argument("--mode", choices=("ci", "production"), required=True)
    args = parser.parse_args()

    path = (ROOT / args.config).resolve()
    if ROOT not in path.parents or not path.is_file():
        fail("config path missing or outside repository")

    config = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    if contains_config_required(config):
        fail("release governance contains CONFIG_REQUIRED")
    scan_secret_keys(config)

    organization = config.get("organization") or {}
    if organization.get("organization_owned") is not True:
        fail("organization_owned must be true")
    if organization.get("personal_owner_allowed") is not False:
        fail("personal_owner_allowed must be false")
    if not organization.get("organization_id"):
        fail("organization_id is required")

    applications = config.get("applications") or {}
    ios = applications.get("ios") or {}
    android = applications.get("android") or {}
    for field in ("bundle_id", "team_id", "store_account_id", "signing_identity_ref"):
        if not ios.get(field):
            fail(f"iOS {field} is required")
    for field in ("application_id", "developer_account_id", "signing_identity_ref"):
        if not android.get(field):
            fail(f"Android {field} is required")

    required_evidence = set(config.get("required_evidence") or [])
    expected = {
        "IOS_BUILD_PROOF",
        "ANDROID_BUILD_PROOF",
        "SECRET_SCAN",
        "STORE_ACCOUNT_EVIDENCE",
        "SIGNING_AUDIT",
        "RELEASE_EVIDENCE",
    }
    if not expected.issubset(required_evidence):
        fail(f"required_evidence missing {sorted(expected - required_evidence)}")

    if args.mode == "production":
        if config.get("scope") != "PRODUCTION" or config.get("production_approved") is not True:
            fail("production release governance is not approved")
        require_ref(ios.get("signing_identity_ref"), "secret://", "iOS signing_identity_ref")
        require_ref(android.get("signing_identity_ref"), "secret://", "Android signing_identity_ref")
        if not (ROOT / "apps/mobile/ios").is_dir():
            fail("production iOS build graph is absent")
        if not (ROOT / "apps/mobile/android").is_dir():
            fail("production Android build graph is absent")
    else:
        if config.get("scope") != "CI_ONLY":
            fail("CI release governance must have scope=CI_ONLY")
        require_ref(ios.get("signing_identity_ref"), "ci://", "iOS signing_identity_ref")
        require_ref(android.get("signing_identity_ref"), "ci://", "Android signing_identity_ref")

    print(f"MOBILE RELEASE GUARD: PASS (mode={args.mode}, config={path.relative_to(ROOT)})")

if __name__ == "__main__":
    main()
