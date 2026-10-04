#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

import yaml
from jsonschema import Draft202012Validator

ROOT = Path(__file__).resolve().parents[1]
SCHEMA_PATH = ROOT / "canon/evidence/mobile-release-evidence-v1.schema.json"
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")


def fail(message: str) -> None:
    print(f"MOBILE RELEASE EVIDENCE GUARD: FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def load_json(path: Path) -> dict:
    doc = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(doc, dict):
        fail(f"{path.relative_to(ROOT)} must be a JSON object")
    return doc


def load_yaml(path: Path) -> dict:
    doc = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    if not isinstance(doc, dict):
        fail(f"{path.relative_to(ROOT)} must be a mapping")
    return doc


def validate_binding(evidence: dict, governance: dict, expected_sha: str, mode: str) -> list[str]:
    errors: list[str] = []

    if evidence.get("candidate_sha") != expected_sha:
        errors.append("CANDIDATE_SHA_MISMATCH")

    organization = governance.get("organization") or {}
    if evidence.get("organization_id") != organization.get("organization_id"):
        errors.append("ORGANIZATION_ID_MISMATCH")

    applications = governance.get("applications") or {}
    ios_gov = applications.get("ios") or {}
    android_gov = applications.get("android") or {}
    ios = evidence.get("ios") or {}
    android = evidence.get("android") or {}

    if ios.get("bundle_id") != ios_gov.get("bundle_id"):
        errors.append("IOS_BUNDLE_ID_MISMATCH")
    if ios.get("team_id") != ios_gov.get("team_id"):
        errors.append("IOS_TEAM_ID_MISMATCH")
    if android.get("application_id") != android_gov.get("application_id"):
        errors.append("ANDROID_APPLICATION_ID_MISMATCH")
    if android.get("developer_account_id") != android_gov.get("developer_account_id"):
        errors.append("ANDROID_DEVELOPER_ACCOUNT_ID_MISMATCH")

    for platform, row in (("IOS", ios), ("ANDROID", android)):
        digest = row.get("artifact_sha256")
        if not isinstance(digest, str) or SHA256_RE.fullmatch(digest) is None:
            errors.append(f"{platform}_ARTIFACT_SHA256_INVALID")
        if row.get("signed") is not True:
            errors.append(f"{platform}_SIGNED_REQUIRED")

    if evidence.get("secret_scan") != "PASS":
        errors.append("SECRET_SCAN_NOT_PASS")

    if mode == "production":
        if evidence.get("production_release") is not True:
            errors.append("PRODUCTION_RELEASE_REQUIRED")
        if governance.get("scope") != "PRODUCTION" or governance.get("production_approved") is not True:
            errors.append("PRODUCTION_GOVERNANCE_NOT_APPROVED")
        for field in ("store_account_evidence", "signing_audit"):
            value = evidence.get(field)
            if not isinstance(value, str) or not value.startswith("evidence://"):
                errors.append(f"{field.upper()}:PRODUCTION_EVIDENCE_REF_REQUIRED")
    else:
        if evidence.get("production_release") is not False:
            errors.append("CI_PRODUCTION_RELEASE_MUST_BE_FALSE")
        for field in ("store_account_evidence", "signing_audit"):
            value = evidence.get(field)
            if not isinstance(value, str) or not value.startswith("ci://"):
                errors.append(f"{field.upper()}:CI_REF_REQUIRED")

    return errors


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("evidence")
    parser.add_argument("governance")
    parser.add_argument("--expected-sha", required=True)
    parser.add_argument("--mode", choices=("ci", "production"), required=True)
    args = parser.parse_args()

    evidence_path = (ROOT / args.evidence).resolve()
    governance_path = (ROOT / args.governance).resolve()
    for path in (evidence_path, governance_path):
        if ROOT not in path.parents or not path.is_file():
            fail("input path missing or outside repository")

    evidence = load_json(evidence_path)
    governance = load_yaml(governance_path)
    schema = load_json(SCHEMA_PATH)

    schema_errors = sorted(Draft202012Validator(schema).iter_errors(evidence), key=lambda e: list(e.path))
    if schema_errors:
        fail("; ".join(error.message for error in schema_errors))

    errors = validate_binding(evidence, governance, args.expected_sha, args.mode)
    if errors:
        fail("; ".join(errors))

    print(
        "MOBILE RELEASE EVIDENCE GUARD: PASS "
        f"(mode={args.mode}, candidate_sha={args.expected_sha})"
    )


if __name__ == "__main__":
    main()
