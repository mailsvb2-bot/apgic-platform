#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import json
import re
import sys
from pathlib import Path

import yaml

from registry_ref_guard import validate_repository_refs

ROOT = Path(__file__).resolve().parents[1]
WORKING_REGISTRY = ROOT / "canon/requirements/registry.yaml"
WORKING_COVERAGE = ROOT / "canon/requirements/coverage.json"
BASELINE_REGISTRY = ROOT / "canon/baseline/APGIC_Requirement_Registry_v7_FINAL.yaml"
BASELINE_COVERAGE = ROOT / "canon/baseline/APGIC_Canon_Coverage_v7_FINAL.json"
EXCEPTIONS = ROOT / "canon/requirements/governance_exceptions.yaml"
CANON = ROOT / "canon/APGIC_Единое_каноническое_ТЗ_исполняемый_канон_v7_FINAL.docx"

ALLOWED_STATUS = {
    "PROPOSED", "APPROVED", "IN_PROGRESS", "IMPLEMENTED", "VERIFIED",
    "RELEASED", "BLOCKED", "DEFERRED", "SUPERSEDED",
}
ID_RE = re.compile(r"^APGIC-[A-Z0-9_]+-\d{3}$")
LAUNCH_PROFILES = {"R0", "R1", "R2", "R3", "R4"}
IMMUTABLE_REQUIREMENT_FIELDS = (
    "requirement_id", "title", "statement", "owner_domain", "source_sections",
    "release_profile", "risk", "priority", "dependencies", "acceptance",
)

errors: list[str] = []
warnings: list[str] = []

working = yaml.safe_load(WORKING_REGISTRY.read_text(encoding="utf-8"))
baseline = yaml.safe_load(BASELINE_REGISTRY.read_text(encoding="utf-8"))
coverage = json.loads(WORKING_COVERAGE.read_text(encoding="utf-8"))
baseline_coverage = json.loads(BASELINE_COVERAGE.read_text(encoding="utf-8"))
exceptions_doc = yaml.safe_load(EXCEPTIONS.read_text(encoding="utf-8")) or {}

requirements = working.get("requirements") or []
baseline_requirements = baseline.get("requirements") or []
by_id: dict[str, dict] = {}
baseline_by_id = {item.get("requirement_id"): item for item in baseline_requirements}

errors.extend(validate_repository_refs(ROOT, requirements))

if working.get("meta") != baseline.get("meta"):
    errors.append("working registry meta differs from immutable v7 FINAL baseline")

for req in requirements:
    rid = req.get("requirement_id")
    if not isinstance(rid, str) or not ID_RE.match(rid):
        errors.append(f"invalid requirement_id: {rid!r}")
        continue
    if rid in by_id:
        errors.append(f"duplicate requirement_id: {rid}")
    by_id[rid] = req

if set(by_id) != set(baseline_by_id):
    errors.append("working registry requirement set differs from immutable baseline")

for rid, req in by_id.items():
    base = baseline_by_id.get(rid)
    if base:
        for field in IMMUTABLE_REQUIREMENT_FIELDS:
            if req.get(field) != base.get(field):
                errors.append(f"{rid}: semantic field changed without Canon/RFC: {field}")

    if req.get("status") not in ALLOWED_STATUS:
        errors.append(f"{rid}: invalid status {req.get('status')!r}")
    if not req.get("owner_domain"):
        errors.append(f"{rid}: missing owner_domain")
    if not req.get("release_profile"):
        errors.append(f"{rid}: missing release_profile")
    if not req.get("acceptance"):
        errors.append(f"{rid}: missing acceptance")
    for dep in req.get("dependencies") or []:
        if dep not in by_id:
            errors.append(f"{rid}: unknown dependency {dep}")
    if req.get("release_profile") in LAUNCH_PROFILES:
        required = (req.get("acceptance") or {}).get("required_evidence") or []
        if not required:
            errors.append(f"{rid}: launch-critical requirement has no evidence expectation")
    if req.get("status") in {"VERIFIED", "RELEASED"}:
        for key in ("implementation_refs", "test_refs", "evidence_refs"):
            if not req.get(key):
                errors.append(f"{rid}: {req.get('status')} with empty {key}")

if coverage != baseline_coverage:
    errors.append("coverage map differs from immutable v7 FINAL baseline")

sections = coverage.get("sections") or []
section_ids = [s.get("section") for s in sections]
if len(section_ids) != len(set(section_ids)):
    errors.append("coverage contains duplicate section numbers")
expected = (working.get("canon_coverage_summary") or {}).get("sections_expected")
if expected and len(sections) != expected:
    errors.append(f"coverage section count {len(sections)} != expected {expected}")
if section_ids:
    missing = sorted(set(range(1, max(section_ids) + 1)) - set(section_ids))
    if missing:
        errors.append(f"coverage missing sections: {missing[:20]}")

for section in sections:
    for rid in section.get("linked_requirement_ids") or []:
        if rid not in by_id:
            errors.append(f"coverage section {section.get('section')} links unknown {rid}")

if CANON.exists():
    actual = hashlib.sha256(CANON.read_bytes()).hexdigest()
    expected_hash = (working.get("meta") or {}).get("canonical_docx_sha256")
    if actual != expected_hash:
        errors.append(f"canonical DOCX hash mismatch: {actual} != {expected_hash}")
else:
    warnings.append("canonical DOCX bytes are not stored in repository; baseline SHA remains registry-controlled")

release_rank = {f"R{i}": i for i in range(10)}
tracked = {
    (item.get("subject_requirement"), item.get("dependency_requirement"))
    for item in exceptions_doc.get("exceptions", [])
    if item.get("status") == "OPEN" and item.get("type") == "RELEASE_PROFILE_DEPENDENCY_ORDER"
}
observed_conflicts: set[tuple[str, str]] = set()

for rid, req in by_id.items():
    here = release_rank.get(req.get("release_profile"), 999)
    for dep in req.get("dependencies") or []:
        there = release_rank.get(by_id[dep].get("release_profile"), 999)
        if there > here:
            pair = (rid, dep)
            observed_conflicts.add(pair)
            if pair not in tracked:
                errors.append(
                    f"{rid} ({req.get('release_profile')}) depends on later {dep} "
                    f"({by_id[dep].get('release_profile')}) without governed exception"
                )
            else:
                warnings.append(
                    f"tracked governance exception: {rid} ({req.get('release_profile')}) -> "
                    f"{dep} ({by_id[dep].get('release_profile')})"
                )

for pair in tracked - observed_conflicts:
    errors.append(f"stale governance exception: {pair[0]} -> {pair[1]}")

if errors:
    print("CANON LINT: FAIL")
    for error in errors:
        print(f"ERROR: {error}")
    for warning in warnings:
        print(f"WARNING: {warning}")
    sys.exit(1)

print(f"CANON LINT: PASS ({len(by_id)} requirements, {len(sections)} covered sections)")
for warning in warnings:
    print(f"WARNING: {warning}")
