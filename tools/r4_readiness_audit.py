#!/usr/bin/env python3
"""Read-only R4 inventory; never an authorization to release production money flows."""
from __future__ import annotations
import argparse
import json
import re
from pathlib import Path
import yaml

SHA = re.compile(r"^[0-9a-f]{40}$")
REQUIRED = (
    "APGIC-COMMISSION-001", "APGIC-SETTLE-001", "APGIC-PAYOUT-001",
    "APGIC-RECON-001", "APGIC-DISPUTE-001", "APGIC-REVENUE-001",
    "APGIC-MOBILE-015", "APGIC-MOBILE-019", "APGIC-MOBILE-030",
    "APGIC-PAY-010", "APGIC-PAY-011", "APGIC-PAY-015",
    "APGIC-PAY-018", "APGIC-PAY-020",
)
CRITICAL_GATES = (
    "HUMAN_MFA_STEP_UP_E2E", "STAGING_FINANCE_END_TO_END",
    "STAGING_FINANCE_WORKER_OUTBOX", "STAGING_RECONCILIATION_DISCREPANCY_E2E",
    "PROVIDER_EXECUTED_SPLIT_PAYOUT_PROOF", "NO_APGIC_CUSTODY_PROOF",
    "LEDGER_RECONCILIATION", "IOS_SIGNED_BUILD", "ANDROID_SIGNED_BUILD",
    "STORE_COMMERCE_E2E", "STORE_REVIEW_EVIDENCE",
)

def audit(registry: object, manifest: object, expected_sha: str) -> dict:
    errors: list[str] = []
    if not isinstance(registry, dict) or not isinstance(registry.get("requirements"), list):
        errors.append("REGISTRY_INVALID")
        rows = []
    else:
        rows = registry["requirements"]
    if not isinstance(manifest, dict):
        errors.append("MANIFEST_INVALID")
        manifest = {}
    if not isinstance(expected_sha, str) or not SHA.fullmatch(expected_sha):
        errors.append("EXPECTED_COMMIT_SHA_INVALID")
    if manifest.get("schema_version") != "r4-release-gate-v1":
        errors.append("MANIFEST_SCHEMA_INVALID")
    if manifest.get("candidate_sha") != expected_sha:
        errors.append("CANDIDATE_SHA_MISMATCH")
    if manifest.get("synthetic") is not False:
        errors.append("SYNTHETIC_OR_UNDECLARED_EVIDENCE")
    if manifest.get("production_candidate") is not True:
        errors.append("NOT_A_PRODUCTION_CANDIDATE")
    requirements: dict[str, dict] = {}
    for row in rows:
        if not isinstance(row, dict) or not isinstance(row.get("requirement_id"), str):
            errors.append("REGISTRY_ROW_INVALID")
            continue
        key = row["requirement_id"]
        if key in requirements:
            errors.append(f"{key}:DUPLICATE_REQUIREMENT")
        requirements[key] = row
    detail = {}
    for req_id in REQUIRED:
        item = requirements.get(req_id)
        status = item.get("status") if isinstance(item, dict) else None
        refs = item.get("evidence_refs") if isinstance(item, dict) else None
        has_refs = isinstance(refs, list) and bool(refs) and all(
            isinstance(v, str) and bool(v.strip()) for v in refs
        )
        complete = status in ("VERIFIED", "RELEASED") and has_refs
        detail[req_id] = {
            "status": status or "MISSING",
            "has_evidence_refs": has_refs,
            "registry_complete": complete,
        }
        if not complete:
            errors.append(f"{req_id}:NOT_VERIFIED_WITH_EVIDENCE")
    evidence = manifest.get("evidence")
    if not isinstance(evidence, dict):
        errors.append("EVIDENCE_MAP_INVALID")
        evidence = {}
    for gate in CRITICAL_GATES:
        row = evidence.get(gate)
        if (
            not isinstance(row, dict)
            or row.get("status") != "PASS"
            or not isinstance(row.get("ref"), str)
            or not row["ref"].startswith("evidence://")
            or len(row["ref"]) <= len("evidence://")
        ):
            errors.append(f"{gate}:MISSING_ATTESTED_EVIDENCE")
    # These checks inspect shape and references, NOT authenticity, provider
    # signatures, payment events, review, or provenance. Never grant release.
    errors.append("AUTHORITATIVE_PRODUCTION_RELEASE_GATE_REQUIRED")
    return {
        "schema_version": "apgic-r4-readiness-audit-v2",
        "candidate_sha": expected_sha,
        "ready": False,
        "inventory_complete": len(errors) == 1,
        "requirements": detail,
        "blockers": sorted(set(errors)),
        "warning": (
            "Inventory is not release authorization. Run tools/r4_release_gate.py "
            "--gate all --mode production with reviewed production evidence index "
            "and independent live provider/staging proofs."
        ),
    }

def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--registry", type=Path, required=True)
    parser.add_argument("--evidence-manifest", type=Path, required=True)
    parser.add_argument("--candidate-sha", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    try:
        report = audit(
            yaml.safe_load(args.registry.read_text(encoding="utf-8")),
            yaml.safe_load(args.evidence_manifest.read_text(encoding="utf-8")),
            args.candidate_sha,
        )
    except (OSError, ValueError, yaml.YAMLError, AttributeError, TypeError) as exc:
        report = {
            "ready": False, "inventory_complete": False,
            "blockers": [f"INPUT_INVALID:{type(exc).__name__}"],
        }
    encoded = json.dumps(report, indent=2, ensure_ascii=False) + "\n"
    if args.output:
        args.output.write_text(encoded, encoding="utf-8")
    print(encoded, end="")
    return 1  # No static inventory can pass as the production financial gate.

if __name__ == "__main__":
    raise SystemExit(main())
