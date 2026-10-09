#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
SHA_RE = re.compile(r"^[0-9a-f]{40}$")
PROFILE_POLICY_FIELDS = {
    "R0": (
        "jurisdiction_matrix_path",
        "retention_policy_path",
        "slo_policy_path",
        "provider_matrix_path",
    ),
    "R1": (
        "jurisdiction_matrix_path",
        "retention_policy_path",
        "slo_policy_path",
        "provider_matrix_path",
        "market_cell_thresholds_path",
    ),
    "R2": (
        "jurisdiction_matrix_path",
        "retention_policy_path",
        "slo_policy_path",
        "provider_matrix_path",
        "market_cell_thresholds_path",
        "commerce_policy_path",
        "store_policy_path",
    ),
    "R3": (
        "jurisdiction_matrix_path",
        "retention_policy_path",
        "slo_policy_path",
        "provider_matrix_path",
        "market_cell_thresholds_path",
        "commerce_policy_path",
        "store_policy_path",
        "booking_fulfillment_policy_path",
    ),
    "R4": (
        "jurisdiction_matrix_path",
        "retention_policy_path",
        "slo_policy_path",
        "provider_matrix_path",
        "market_cell_thresholds_path",
        "commerce_policy_path",
        "store_policy_path",
        "booking_fulfillment_policy_path",
        "provider_settlement_policy_path",
    ),
}


def fail(message: str) -> None:
    print(f"PRODUCTION PREFLIGHT EVIDENCE: FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def repo_file(raw: object, label: str) -> Path:
    if not isinstance(raw, str) or not raw.strip():
        fail(f"{label} is required")
    path = (ROOT / raw).resolve()
    if ROOT not in path.parents or not path.is_file():
        fail(f"{label} must resolve to an existing repository file")
    return path


def build_evidence(candidate_sha: str, profile: str, config_rel: str) -> dict:
    if not SHA_RE.fullmatch(candidate_sha):
        fail("candidate SHA must be a lowercase 40-character commit SHA")
    if profile not in PROFILE_POLICY_FIELDS:
        fail(f"unsupported profile {profile}")

    config_path = repo_file(config_rel, "launch config")
    config = yaml.safe_load(config_path.read_text(encoding="utf-8")) or {}
    if not isinstance(config, dict):
        fail("launch config must be a mapping")
    if config.get("environment") != "PRODUCTION":
        fail("launch config environment must be PRODUCTION")
    if config.get("production_approved") is not True:
        fail("launch config must be production_approved=true")

    policies: dict[str, dict[str, str]] = {}
    for field in PROFILE_POLICY_FIELDS[profile]:
        policy_path = repo_file(config.get(field), field)
        policies[field] = {
            "path": policy_path.relative_to(ROOT).as_posix(),
            "sha256": sha256(policy_path),
        }

    return {
        "schema_version": "production-preflight-evidence-v1",
        "evidence_class": "PRODUCTION_PREFLIGHT",
        "production_release": False,
        "production_promotion_eligible": True,
        "candidate_sha": candidate_sha,
        "release_profile": profile,
        "generated_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "launch_config": {
            "path": config_path.relative_to(ROOT).as_posix(),
            "sha256": sha256(config_path),
        },
        "policy_artifacts": policies,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate-sha", required=True)
    parser.add_argument("--profile", choices=tuple(PROFILE_POLICY_FIELDS), required=True)
    parser.add_argument("--config", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    evidence = build_evidence(args.candidate_sha, args.profile, args.config)
    output = (ROOT / args.output).resolve()
    if ROOT not in output.parents:
        fail("output path escapes repository")
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(
        json.dumps(evidence, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    print(
        "PRODUCTION PREFLIGHT EVIDENCE: PASS "
        f"(profile={args.profile}, candidate={args.candidate_sha}, output={output.relative_to(ROOT)})"
    )


if __name__ == "__main__":
    main()
