#!/usr/bin/env python3
"""Launch v1 (R0-R4) candidate assurance.

This is a release-authorization gate, not a deployment tool. It never turns
CI/staging evidence into production evidence and never mutates the Canon.
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import yaml

import canon_execution_gap_inventory as inventory_tool
import production_preflight_evidence as preflight
import r4_release_gate as r4

ROOT = Path(__file__).resolve().parents[1]
SHA_RE = re.compile(r"^[a-f0-9]{40}$")
LAUNCH_PROFILES = ("R0", "R1", "R2", "R3", "R4")
REQUIRED_TRACE_FIELDS = ("implementation_refs", "contract_refs", "test_refs", "evidence_refs")


def load_yaml(path: Path) -> dict[str, Any]:
    data = yaml.safe_load(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise ValueError(f"{path.name} must contain a mapping")
    return data


def confined_file(raw: str | None) -> Path:
    if not isinstance(raw, str) or not raw.strip():
        raise ValueError("path required")
    path = (ROOT / raw).resolve(strict=True)
    if not path.is_file() or not path.is_relative_to(ROOT):
        raise ValueError("path must name a file inside the checked-out repository")
    return path


def assess(
    *,
    candidate_sha: str,
    coverage: dict[str, Any],
    registry: dict[str, Any],
    production_index: dict[str, Any],
    manifest: dict[str, Any] | None = None,
    preflight_evidence: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Pure decision: absence of proof is a blocker, not a successful default."""
    blockers: list[str] = []
    warnings: list[str] = []
    inventory = inventory_tool.build_inventory(coverage, registry, production_index)
    totals = inventory["totals"]
    if not SHA_RE.fullmatch(candidate_sha):
        blockers.append("CANDIDATE_SHA_NOT_EXACT")
    if coverage.get("meta") != registry.get("meta"):
        blockers.append("FROZEN_CANON_REGISTRY_METADATA_MISMATCH")
    if totals["traceability_mismatches"]:
        blockers.append("FROZEN_CANON_LINKAGE_MISMATCH")
    if totals["owner_only_sections_needing_analysis"]:
        # These include post-launch and descriptive sections; absence of an
        # explicit link is NOT evidence that functionality is missing.
        warnings.append("UNMAPPED_CANON_SECTIONS_REQUIRE_SCOPED_GOVERNANCE_REVIEW")

    launch = [row for row in inventory["all_requirements_dependency_order"]
              if row["release_profile"] in LAUNCH_PROFILES]
    status_counts = Counter(str(row["status"]) for row in launch)
    details: list[dict[str, Any]] = []
    for row in launch:
        reasons: list[str] = []
        if row["status"] != "RELEASED":
            reasons.append("REQUIREMENT_NOT_RELEASED")
        if row["unverified_dependencies"]:
            reasons.append("UPSTREAM_REQUIREMENT_NOT_VERIFIED")
        for field in REQUIRED_TRACE_FIELDS:
            refs = row.get(field)
            if not isinstance(refs, list) or not refs:
                reasons.append(f"{field.upper()}_MISSING")
        if not row["required_evidence_classes"]:
            reasons.append("ACCEPTANCE_EVIDENCE_CLASSES_MISSING")
        if reasons:
            details.append({
                "requirement_id": row["requirement_id"],
                "profile": row["release_profile"],
                "priority": row["priority"],
                "status": row["status"],
                "reasons": reasons,
                "dependency_blockers": row["unverified_dependencies"],
            })
    if details:
        blockers.append("LAUNCH_REQUIREMENTS_UNRELEASED_OR_UNPROVEN")

    if preflight_evidence is None:
        blockers.append("PRODUCTION_CONFIG_PREFLIGHT_MISSING")
    else:
        if preflight_evidence.get("candidate_sha") != candidate_sha:
            blockers.append("PRODUCTION_CONFIG_CANDIDATE_MISMATCH")
        if preflight_evidence.get("release_profile") != "R4":
            blockers.append("PRODUCTION_CONFIG_NOT_FULL_LAUNCH")
        if preflight_evidence.get("production_release") is not False:
            blockers.append("PREFLIGHT_MISCLASSIFIED_AS_RELEASE")
        if preflight_evidence.get("evidence_class") != "PRODUCTION_PREFLIGHT":
            blockers.append("PRODUCTION_PREFLIGHT_CLASS_INVALID")

    production_records = production_index.get("records")
    if not isinstance(production_records, dict) or not production_records:
        blockers.append("PRODUCTION_EVIDENCE_INDEX_EMPTY")
    if manifest is None:
        blockers.append("R4_EXTERNAL_PRODUCTION_EVIDENCE_MANIFEST_MISSING")
    else:
        if manifest.get("candidate_sha") != candidate_sha:
            blockers.append("R4_MANIFEST_CANDIDATE_MISMATCH")
        if manifest.get("synthetic") is not False or manifest.get("production_candidate") is not True:
            blockers.append("R4_MANIFEST_NOT_REAL_PRODUCTION")
        passed, reasons = r4.evaluate(manifest, "all", "production", production_index)
        if not passed:
            blockers.extend("R4_" + reason for reason in reasons)
        for reqid in (
            r4.NATIVE_RELEASE_REQUIREMENT_ID,
            r4.STORE_RELEASE_REQUIREMENT_ID,
            r4.PAYMENT_RELEASE_REQUIREMENT_ID,
        ):
            blockers.extend("R4_" + reason for reason in r4.canon_dependency_blockers(registry, reqid))

    blockers = sorted(set(blockers))
    return {
        "schema_version": "launch-readiness-assessment-v1",
        "evidence_class": "CANDIDATE_ASSESSMENT_NOT_RELEASE_EVIDENCE",
        "candidate_sha": candidate_sha,
        "launch_cut": list(LAUNCH_PROFILES),
        "launch_ready": not blockers,
        "promotion_authorized": False,  # separate human-controlled rollout decision
        "blockers": blockers,
        "warnings": sorted(set(warnings)),
        "requirements": {
            "launch_total": len(launch),
            "launch_status_counts": dict(sorted(status_counts.items())),
            "blocked": details,
        },
        "governance": {
            "all_canon_sections": totals["canon_sections"],
            "owner_only_sections_requiring_review": totals["owner_only_sections_needing_analysis"],
            "canon_linkage_mismatches": totals["traceability_mismatches"],
            "post_launch_requirements_excluded_from_launch_cut": totals["requirements"] - len(launch),
        },
        "evidence": {
            "production_index_records": len(production_records) if isinstance(production_records, dict) else 0,
            "r4_production_manifest_supplied": manifest is not None,
            "production_config_preflight_supplied": preflight_evidence is not None,
        },
    }


def render_summary(report: dict[str, Any]) -> str:
    req = report["requirements"]
    lines = [
        "## APGIC Launch v1 candidate assurance — R0/R1/R2/R3/R4",
        "",
        f"- Candidate: \`{report['candidate_sha']}\`",
        f"- Decision: **{'READY FOR SEPARATE LAUNCH AUTHORIZATION' if report['launch_ready'] else 'BLOCKED'}**",
        f"- Launch requirements: **{req['launch_total']}**",
        f"- Requirement states: {', '.join(f'{k}={v}' for k,v in req['launch_status_counts'].items())}",
        f"- Unresolved requirement rows: **{len(req['blocked'])}**",
        f"- Production evidence records: **{report['evidence']['production_index_records']}**",
        "- This is not a production rollout or authorization to take money.",
        "",
        "### Blocking reasons",
    ]
    lines.extend(f"- \`{b}\`" for b in report["blockers"])
    if not report["blockers"]:
        lines.append("- None")
    if report["warnings"]:
        lines.extend(["", "### Governance review (not proof of missing code)"])
        lines.extend(f"- \`{w}\`" for w in report["warnings"])
    return "\n".join(lines) + "\n"


def verify_strict_checkout(candidate_sha: str) -> None:
    """The claimed candidate must be the code actually under assessment."""
    if not SHA_RE.fullmatch(candidate_sha):
        raise ValueError("strict candidate must be an exact lowercase SHA")
    try:
        head = subprocess.check_output(
            ["git", "rev-parse", "HEAD"], cwd=ROOT, text=True, stderr=subprocess.DEVNULL,
        ).strip()
    except (OSError, subprocess.CalledProcessError) as exc:
        raise ValueError("strict assessment requires an auditable Git checkout") from exc
    if head != candidate_sha:
        raise ValueError("strict candidate differs from the checked-out Git HEAD")


def validate_full_production_policy(config_rel: str) -> None:
    """Reuse the Canon's complete policy validation, never just the file/hash check.

    Evidence hashing does not establish semantic approval of jurisdiction,
    retention, SLO, provider, commerce, store and settlement rules.
    """
    config = confined_file(config_rel)
    proc = subprocess.run(
        [sys.executable, str(ROOT / "tools/launch_config_guard.py"),
         config.relative_to(ROOT).as_posix(),
         "--mode", "production", "--profile", "R4"],
        cwd=ROOT, text=True, capture_output=True, check=False,
    )
    if proc.returncode != 0:
        raise ValueError(
            "Canonical R4 production LaunchConfig validation failed; "
            "no release assessment may infer policy approval from hashes alone"
        )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--candidate-sha", required=True)
    parser.add_argument("--config", help="Approved production R4 LaunchConfig path")
    parser.add_argument("--manifest", help="R4 evidence manifest path")
    parser.add_argument("--output", required=True)
    parser.add_argument("--summary")
    parser.add_argument("--strict", action="store_true",
                        help="Fail closed if anything prevents production launch (R0-R4)")
    args = parser.parse_args()
    try:
        if args.strict:
            verify_strict_checkout(args.candidate_sha)
            if args.config:
                validate_full_production_policy(args.config)
        registry = load_yaml(ROOT / "canon/requirements/registry.yaml")
        coverage = json.loads((ROOT / "canon/requirements/coverage.json").read_text(encoding="utf-8"))
        index = load_yaml(ROOT / "canon/evidence/production-evidence-index.yaml")
        manifest = load_yaml(confined_file(args.manifest)) if args.manifest else None
        configuration = preflight.build_evidence(
            args.candidate_sha, "R4", args.config
        ) if args.config else None
        report = assess(
            candidate_sha=args.candidate_sha,
            coverage=coverage, registry=registry,
            production_index=index, manifest=manifest, preflight_evidence=configuration,
        )
        output = (ROOT / args.output).resolve()
        if not output.is_relative_to(ROOT):
            raise ValueError("output path escapes repository")
        output.parent.mkdir(parents=True, exist_ok=True)
        report["generated_at"] = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        output.write_text(json.dumps(report, indent=2, ensure_ascii=False, sort_keys=True) + "\n", encoding="utf-8")
        if args.summary:
            summary = Path(args.summary)
            summary.parent.mkdir(parents=True, exist_ok=True)
            with summary.open("a", encoding="utf-8") as stream:
                stream.write(render_summary(report))
        print("LAUNCH READINESS: " + ("READY FOR SEPARATE AUTHORIZATION" if report["launch_ready"] else "BLOCKED"))
        print(f"LAUNCH READINESS REPORT: {output}")
        for reason in report["blockers"]:
            print("BLOCKER: " + reason)
        return 2 if args.strict and not report["launch_ready"] else 0
    except (OSError, ValueError, KeyError, TypeError, yaml.YAMLError,
            inventory_tool.CanonInventoryError) as exc:
        print(f"LAUNCH READINESS: INVALID ASSESSMENT: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
