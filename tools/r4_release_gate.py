#!/usr/bin/env python3
from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]

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


def evaluate(document: dict, gate: str, mode: str) -> tuple[bool, list[str]]:
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

    passed, blockers = evaluate(load_document(path), args.gate, args.mode)
    if not passed:
        fail("; ".join(blockers))

    outcome = "CI_ONLY_PASS" if args.mode == "ci" else "PRODUCTION_GATE_PASS"
    print(f"R4 RELEASE GATE: {outcome} (gate={args.gate})")


if __name__ == "__main__":
    main()
