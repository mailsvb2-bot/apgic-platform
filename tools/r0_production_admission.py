#!/usr/bin/env python3
"""R0 production admission is separate from configuration and CI preflight.

Only reviewed, exact-candidate operational evidence may approve a launch.
No CI/staging fixture or a mere registry status can stand in for live proof.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
SHA = re.compile(r"^[0-9a-f]{40}$")
REQUIRED_PROOFS = (
    "R0_IDENTITY_MULTISURFACE_HUMAN_E2E",
    "R0_TENANT_ISOLATION_NEGATIVE_E2E",
    "R0_HUMAN_MFA_STEP_UP_AND_AUDIT",
    "R0_STAGING_RUNTIME_EXACT_SHA",
    "R0_RESTORE_DRILL_RPO_RTO",
    "R0_ROLLBACK_RESTART_READBACK",
    "R0_SIGNED_NATIVE_INSTALLATION_IOS",
    "R0_SIGNED_NATIVE_INSTALLATION_ANDROID",
)
PROHIBITED_CLASSES = {"CI_CANDIDATE", "CONFORMANCE", "SYNTHETIC", "TEST_FIXTURE"}


def inspect(registry: dict, index: dict, candidate_sha: str, root: Path = ROOT) -> list[str]:
    blockers: list[str] = []
    if not SHA.fullmatch(candidate_sha):
        return ["CANDIDATE_SHA_INVALID"]
    requirements = registry.get("requirements")
    if not isinstance(requirements, list):
        return ["CANON_REGISTRY_INVALID"]
    r0 = [item for item in requirements if isinstance(item, dict) and item.get("release_profile") == "R0"]
    if not r0:
        blockers.append("R0_REQUIREMENTS_MISSING")
    for item in r0:
        identifier, status = item.get("requirement_id"), item.get("status")
        if not isinstance(identifier, str) or status not in {"VERIFIED", "RELEASED"}:
            blockers.append(f"CANON_NOT_VERIFIED:{identifier}:{status}")
    if index.get("schema_version") != "production-evidence-index-v1":
        blockers.append("PRODUCTION_INDEX_SCHEMA_INVALID")
    records = index.get("records")
    if not isinstance(records, dict):
        return blockers + ["PRODUCTION_INDEX_RECORDS_INVALID"]
    for proof in REQUIRED_PROOFS:
        entry = records.get(proof)
        if not isinstance(entry, dict):
            blockers.append(f"EVIDENCE_MISSING:{proof}")
            continue
        if entry.get("candidate_sha") != candidate_sha:
            blockers.append(f"EVIDENCE_CANDIDATE_MISMATCH:{proof}")
        if entry.get("evidence_class") in PROHIBITED_CLASSES or entry.get("synthetic") is not False:
            blockers.append(f"EVIDENCE_NOT_LIVE:{proof}")
        if entry.get("reviewed") is not True or not isinstance(entry.get("reviewer"), str) or not entry["reviewer"].strip():
            blockers.append(f"EVIDENCE_NOT_REVIEWED:{proof}")
        path = entry.get("path")
        digest = entry.get("sha256")
        if not isinstance(path, str) or not path.startswith("canon/evidence/production/") or not isinstance(digest, str) or not re.fullmatch(r"[0-9a-f]{64}", digest):
            blockers.append(f"EVIDENCE_PATH_OR_DIGEST_INVALID:{proof}")
            continue
        target = (root / path).resolve()
        if root.resolve() not in target.parents or not target.is_file():
            blockers.append(f"EVIDENCE_ARTIFACT_MISSING:{proof}")
            continue
        actual = hashlib.sha256(target.read_bytes()).hexdigest()
        if actual != digest:
            blockers.append(f"EVIDENCE_DIGEST_MISMATCH:{proof}")
    return blockers


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate-sha", required=True)
    parser.add_argument("--registry", default="canon/requirements/registry.yaml")
    parser.add_argument("--index", default="canon/evidence/production-evidence-index.yaml")
    args = parser.parse_args()
    try:
        registry = yaml.safe_load((ROOT / args.registry).read_text(encoding="utf-8"))
        index = yaml.safe_load((ROOT / args.index).read_text(encoding="utf-8"))
        blockers = inspect(registry, index, args.candidate_sha)
    except (OSError, yaml.YAMLError, ValueError, TypeError) as exc:
        print(f"R0 PRODUCTION ADMISSION: BLOCKED (invalid input: {type(exc).__name__})")
        return 1
    if blockers:
        print(f"R0 PRODUCTION ADMISSION: BLOCKED ({len(blockers)} outstanding)")
        for reason in blockers:
            print(f"BLOCKER: {reason}")
        return 1
    print(f"R0 PRODUCTION ADMISSION: PASS ({args.candidate_sha})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
