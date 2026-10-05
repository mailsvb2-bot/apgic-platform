#!/usr/bin/env python3
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
REGISTRY_PATH = ROOT / "canon/requirements/registry.yaml"
PRODUCTION_EVIDENCE_INDEX_PATH = ROOT / "canon/evidence/production-evidence-index.yaml"
EVIDENCE_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._/-]*$")
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
NATIVE_RELEASE_REQUIREMENT_ID = "APGIC-MOBILE-015"
STORE_RELEASE_REQUIREMENT_ID = "APGIC-MOBILE-030"

NATIVE_REQUIRED = {
    "SERVER_CRITICAL_PATH_E2E",
    "WEB_CRITICAL_PATH_E2E",
    "IOS_NATIVE_E2E",
    "ANDROID_NATIVE_E2E",
    "DEEP_LINK_E2E",
    "PUSH_E2E",
    "OFFLINE_SYNC_E2E",
    "REALTIME_E2E",
    "IOS_SIGNED_BUILD",
    "ANDROID_SIGNED_BUILD",
    "COMPATIBILITY_EVIDENCE",
    "STAGING_PROOF",
    "RELEASE_EVIDENCE",
}

STORE_REQUIRED = {
    "STORE_ARCHITECTURE_REVIEW",
    "STORE_COMMERCE_E2E",
    "ACCOUNT_DELETION_E2E",
    "PRIVACY_MANIFEST_CHECK",
    "DEPENDENCY_REGISTRY_LINT",
    "DEVICE_INTEGRITY_E2E",
    "MOBILE_SLO_EVIDENCE",
    "COMPATIBILITY_EVIDENCE",
    "ACCESSIBILITY_EVIDENCE",
    "STORE_REVIEW_EVIDENCE",
    "IOS_SIGNED_BUILD",
    "ANDROID_SIGNED_BUILD",
    "STAGED_ROLLOUT_PROOF",
}

PAYMENT_REQUIRED = {
    "TWO_PROVIDER_CERTIFICATION",
    "MULTI_PROVIDER_CHECKOUT_E2E",
    "FAILOVER_CHAOS_PROOF",
    "PROVIDER_EXECUTED_SPLIT_PAYOUT_PROOF",
    "NO_APGIC_CUSTODY_PROOF",
    "LEDGER_RECONCILIATION",
    "ADMIN_AUDIT_EVIDENCE",
}

GATE_REQUIRED = {
    "native": NATIVE_REQUIRED,
    "store": STORE_REQUIRED,
    "payments": PAYMENT_REQUIRED,
    "all": NATIVE_REQUIRED | STORE_REQUIRED | PAYMENT_REQUIRED,
}


def fail(message: str) -> None:
    print(f"R4 RELEASE GATE: FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def load_document(path: Path) -> dict:
    document = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    if not isinstance(document, dict):
        fail("evidence manifest must be a mapping")
    return document


def requirement_index(registry: dict) -> dict[str, dict]:
    rows = registry.get("requirements")
    if not isinstance(rows, list):
        fail("Canon registry requirements must be a list")
    index: dict[str, dict] = {}
    for row in rows:
        if not isinstance(row, dict):
            continue
        requirement_id = row.get("requirement_id")
        if isinstance(requirement_id, str) and requirement_id:
            index[requirement_id] = row
    return index


def canon_dependency_blockers(
    registry: dict,
    requirement_id: str,
) -> list[str]:
    index = requirement_index(registry)
    requirement = index.get(requirement_id)
    if not isinstance(requirement, dict):
        return [f"{requirement_id}:CANON_REQUIREMENT_MISSING"]

    dependencies = requirement.get("dependencies")
    if not isinstance(dependencies, list):
        return [f"{requirement_id}:DEPENDENCIES_INVALID"]

    blockers: list[str] = []
    for dependency_id in dependencies:
        if not isinstance(dependency_id, str) or not dependency_id:
            blockers.append(f"{requirement_id}:DEPENDENCY_ID_INVALID")
            continue
        dependency = index.get(dependency_id)
        if not isinstance(dependency, dict):
            blockers.append(f"{dependency_id}:CANON_DEPENDENCY_MISSING")
            continue
        if dependency.get("status") != "VERIFIED":
            blockers.append(
                f"{dependency_id}:CANON_STATUS_{dependency.get('status', 'MISSING')}"
            )
    return blockers


def production_evidence_blockers(
    document: dict,
    evidence_type: str,
    ref: str,
    production_index: dict | None,
) -> list[str]:
    if production_index is None:
        return [f"{evidence_type}:PRODUCTION_EVIDENCE_INDEX_MISSING"]
    if production_index.get("schema_version") != "production-evidence-index-v1":
        return [f"{evidence_type}:PRODUCTION_EVIDENCE_INDEX_INVALID"]
    records = production_index.get("records")
    if not isinstance(records, dict):
        return [f"{evidence_type}:PRODUCTION_EVIDENCE_INDEX_INVALID"]
    evidence_id = ref.removeprefix("evidence://")
    if not EVIDENCE_ID_RE.fullmatch(evidence_id):
        return [f"{evidence_type}:PRODUCTION_EVIDENCE_ID_INVALID"]
    record = records.get(evidence_id)
    if not isinstance(record, dict):
        return [f"{evidence_type}:PRODUCTION_EVIDENCE_NOT_FOUND"]
    blockers: list[str] = []
    if record.get("evidence_type") != evidence_type:
        blockers.append(f"{evidence_type}:PRODUCTION_EVIDENCE_TYPE_MISMATCH")
    if record.get("status") != "PASS":
        blockers.append(f"{evidence_type}:PRODUCTION_EVIDENCE_NOT_PASS")
    if record.get("synthetic") is not False:
        blockers.append(f"{evidence_type}:PRODUCTION_EVIDENCE_SYNTHETIC")
    if record.get("candidate_sha") != document.get("candidate_sha"):
        blockers.append(f"{evidence_type}:PRODUCTION_EVIDENCE_CANDIDATE_MISMATCH")
    source_ref = record.get("source_ref")
    if not isinstance(source_ref, str) or not source_ref.strip():
        blockers.append(f"{evidence_type}:PRODUCTION_EVIDENCE_SOURCE_MISSING")
    digest = record.get("artifact_sha256")
    if not isinstance(digest, str) or not SHA256_RE.fullmatch(digest):
        blockers.append(f"{evidence_type}:PRODUCTION_EVIDENCE_DIGEST_INVALID")
    verified_at = record.get("verified_at")
    if not isinstance(verified_at, str) or not verified_at.strip():
        blockers.append(f"{evidence_type}:PRODUCTION_EVIDENCE_VERIFIED_AT_MISSING")
    return blockers


def evaluate(document: dict, gate: str, mode: str, production_index: dict | None = None) -> tuple[bool, list[str]]:
    if document.get("schema_version") != "r4-release-gate-v1":
        return False, ["SCHEMA_VERSION_INVALID"]
    if not isinstance(document.get("candidate_sha"), str) or len(document["candidate_sha"].strip()) < 7:
        return False, ["CANDIDATE_SHA_INVALID"]

    synthetic = document.get("synthetic")
    production_candidate = document.get("production_candidate")
    if not isinstance(synthetic, bool) or not isinstance(production_candidate, bool):
        return False, ["EVIDENCE_CLASSIFICATION_INVALID"]

    evidence = document.get("evidence")
    if not isinstance(evidence, dict):
        return False, ["EVIDENCE_MAP_MISSING"]

    blockers: list[str] = []
    for evidence_type in sorted(GATE_REQUIRED[gate]):
        row = evidence.get(evidence_type)
        if not isinstance(row, dict):
            blockers.append(f"{evidence_type}:MISSING")
            continue
        if row.get("status") != "PASS":
            blockers.append(f"{evidence_type}:NOT_PASS")
            continue
        ref = row.get("ref")
        if not isinstance(ref, str) or not ref.strip():
            blockers.append(f"{evidence_type}:REF_MISSING")
            continue
        if mode == "ci":
            if not ref.startswith("ci://"):
                blockers.append(f"{evidence_type}:CI_REF_REQUIRED")
        else:
            if synthetic:
                blockers.append("SYNTHETIC_EVIDENCE_FORBIDDEN")
                break
            if not production_candidate:
                blockers.append("PRODUCTION_CANDIDATE_REQUIRED")
                break
            if not ref.startswith("evidence://"):
                blockers.append(f"{evidence_type}:PRODUCTION_EVIDENCE_REF_REQUIRED")
                continue
            blockers.extend(
                production_evidence_blockers(
                    document,
                    evidence_type,
                    ref,
                    production_index,
                )
            )

    return len(blockers) == 0, blockers


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("manifest")
    parser.add_argument("--gate", choices=tuple(GATE_REQUIRED), required=True)
    parser.add_argument("--mode", choices=("ci", "production"), required=True)
    args = parser.parse_args()

    path = (ROOT / args.manifest).resolve()
    if ROOT not in path.parents or not path.is_file():
        fail("evidence manifest path missing or outside repository")

    document = load_document(path)
    production_index = None
    if args.mode == "production":
        if not PRODUCTION_EVIDENCE_INDEX_PATH.is_file():
            fail("production evidence index is missing")
        production_index = load_document(PRODUCTION_EVIDENCE_INDEX_PATH)
    passed, blockers = evaluate(document, args.gate, args.mode, production_index)

    if args.mode == "production":
        if not REGISTRY_PATH.is_file():
            fail("Canon registry is missing")
        registry = load_document(REGISTRY_PATH)
        if args.gate in {"native", "all"}:
            blockers.extend(
                canon_dependency_blockers(
                    registry,
                    NATIVE_RELEASE_REQUIREMENT_ID,
                )
            )
        if args.gate in {"store", "all"}:
            blockers.extend(
                canon_dependency_blockers(
                    registry,
                    STORE_RELEASE_REQUIREMENT_ID,
                )
            )
        passed = len(blockers) == 0

    if not passed:
        fail("; ".join(blockers))

    outcome = "CI_ONLY_PASS" if args.mode == "ci" else "PRODUCTION_GATE_PASS"
    print(f"R4 RELEASE GATE: {outcome} (gate={args.gate})")


if __name__ == "__main__":
    main()
