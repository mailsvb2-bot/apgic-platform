#!/usr/bin/env python3
"""Inventory ALL frozen v7 sections and remaining executable requirements.

The DOCX/immutable coverage/registry supply meaning. This module never guesses
a requirement, edits the canon, or equates references/green CI with completion.
It reports missing traceability separately from incomplete implementation.
"""
from __future__ import annotations

import argparse
import json
import sys
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any

import yaml

ROOT = Path(__file__).resolve().parents[1]
COVERAGE = ROOT / "canon/requirements/coverage.json"
REGISTRY = ROOT / "canon/requirements/registry.yaml"
PRODUCTION_INDEX = ROOT / "canon/evidence/production-evidence-index.yaml"
COMPLETE = {"VERIFIED", "RELEASED"}
PROFILES = ("R0", "R1", "R2", "R3", "R4", "R5")


class CanonInventoryError(ValueError):
    pass


def _index_requirements(rows: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    indexed: dict[str, dict[str, Any]] = {}
    for row in rows:
        rid = row.get("requirement_id")
        if not isinstance(rid, str) or not rid or rid in indexed:
            raise CanonInventoryError(f"invalid or duplicate requirement ID: {rid!r}")
        indexed[rid] = row
    return indexed


def _dependency_order(requirements: dict[str, dict[str, Any]]) -> list[str]:
    # Dependencies always precede dependents, then deterministic release/priority order.
    visited: dict[str, int] = {}
    ordered: list[str] = []

    def sort_key(rid: str) -> tuple[int, int, str]:
        row = requirements[rid]
        profile = row.get("release_profile", "R5")
        return (
            PROFILES.index(profile) if profile in PROFILES else len(PROFILES),
            0 if row.get("priority") == "P0" else 1,
            rid,
        )

    def visit(rid: str) -> None:
        state = visited.get(rid, 0)
        if state == 1:
            raise CanonInventoryError(f"cycle in requirement dependencies: {rid}")
        if state == 2:
            return
        visited[rid] = 1
        dependencies = requirements[rid].get("dependencies") or []
        for dependency in dependencies:
            if dependency not in requirements:
                raise CanonInventoryError(f"{rid}: missing dependency {dependency}")
        for dependency in sorted(dependencies, key=sort_key):
            visit(dependency)
        visited[rid] = 2
        ordered.append(rid)

    for rid in sorted(requirements, key=sort_key):
        visit(rid)
    return ordered


def build_inventory(coverage: dict[str, Any], registry: dict[str, Any], production_index: dict[str, Any]) -> dict[str, Any]:
    sections = coverage.get("sections")
    rows = registry.get("requirements")
    if not isinstance(sections, list) or not isinstance(rows, list):
        raise CanonInventoryError("canon sections and registry requirements must be arrays")

    requirements = _index_requirements(rows)
    section_by_number: dict[int, dict[str, Any]] = {}
    for section in sections:
        number = section.get("section")
        if not isinstance(number, int) or isinstance(number, bool) or number <= 0 or number in section_by_number:
            raise CanonInventoryError(f"invalid/duplicate canon section: {number!r}")
        section_by_number[number] = section
    if set(section_by_number) != set(range(1, len(sections) + 1)):
        raise CanonInventoryError("canon sections must form a complete 1-based sequence")

    reverse: dict[int, set[str]] = defaultdict(set)
    for rid, row in requirements.items():
        for number in row.get("source_sections") or []:
            if number not in section_by_number:
                raise CanonInventoryError(f"{rid}: unknown source canon section {number}")
            reverse[number].add(rid)

    traceability_mismatches: list[dict[str, Any]] = []
    unmapped: list[dict[str, Any]] = []
    section_inventory: list[dict[str, Any]] = []
    by_domain: Counter[str] = Counter()
    for number in sorted(section_by_number):
        row = section_by_number[number]
        explicit = row.get("linked_requirement_ids")
        if not isinstance(explicit, list):
            raise CanonInventoryError(f"section {number}: linked_requirement_ids must be an array")
        if len(explicit) != len(set(explicit)):
            raise CanonInventoryError(f"section {number}: duplicate explicit requirement link")
        for rid in explicit:
            if rid not in requirements:
                raise CanonInventoryError(f"section {number}: unknown requirement {rid!r}")
        source_ids = sorted(reverse[number])
        if sorted(explicit) != source_ids:
            # Frozen canon companions may disagree with each other. Report the
            # inconsistency as an explicit blocker for human/RFC reconciliation;
            # do not rewrite either normative source or hide the whole backlog.
            traceability_mismatches.append({
                "section": number,
                "coverage_requirement_ids": sorted(explicit),
                "registry_requirement_ids": source_ids,
            })
        has_explicit = bool(explicit)
        item = {
            "section": number,
            "title": row.get("title"),
            "owner_domain": row.get("owner_domain"),
            "coverage_status": row.get("coverage_status"),
            "linked_requirement_ids": explicit,
            # Do not assert unimplemented functionality from absent traceability.
            "tracking_class": "EXPLICIT_REQUIREMENT" if has_explicit else "OWNER_ONLY_REQUIRES_ANALYSIS",
        }
        section_inventory.append(item)
        if not has_explicit:
            unmapped.append(item)
            by_domain[str(row.get("owner_domain"))] += 1

    incomplete: list[dict[str, Any]] = []
    by_profile: dict[str, Counter[str]] = defaultdict(Counter)
    status_totals: Counter[str] = Counter()
    for rid in _dependency_order(requirements):
        row = requirements[rid]
        status = row.get("status")
        profile = str(row.get("release_profile"))
        status_totals[str(status)] += 1
        by_profile[profile][str(status)] += 1
        if status == "RELEASED":
            continue
        dependencies = row.get("dependencies") or []
        item = {
            "requirement_id": rid,
            "title": row.get("title"),
            "owner_domain": row.get("owner_domain"),
            "release_profile": profile,
            "priority": row.get("priority"),
            "status": status,
            "source_sections": row.get("source_sections") or [],
            "dependencies": dependencies,
            "unverified_dependencies": [
                dep for dep in dependencies if requirements[dep].get("status") not in COMPLETE
            ],
            "implementation_refs": row.get("implementation_refs") or [],
            "contract_refs": row.get("contract_refs") or [],
            "test_refs": row.get("test_refs") or [],
            "evidence_refs": row.get("evidence_refs") or [],
            "required_evidence_classes": (row.get("acceptance") or {}).get("required_evidence") or [],
            "next_state_requires": (
                "PRODUCTION_ACCEPTANCE_AND_RELEASE_PROOF" if status == "VERIFIED"
                else "IMPLEMENTATION_INTEGRATION_AND_REQUIRED_EVIDENCE"
            ),
        }
        incomplete.append(item)

    records = production_index.get("records")
    if not isinstance(records, dict):
        raise CanonInventoryError("production evidence index records must be a mapping")

    return {
        "schema_version": "canon-v7-execution-gap-inventory-v1",
        "canon_baseline": coverage.get("meta", {}).get("version"),
        "scope": "ALL_CANON_SECTIONS_ALL_REQUIREMENTS",
        "disclaimer": (
            "OWNER_ONLY_REQUIRES_ANALYSIS means absent explicit traceability, NOT proof of "
            "missing implementation. VERIFIED is not RELEASED. Staging/CI are not production."
        ),
        "totals": {
            "canon_sections": len(sections),
            "explicitly_tracked_sections": len(sections) - len(unmapped),
            "owner_only_sections_needing_analysis": len(unmapped),
            "requirements": len(requirements),
            "status_counts": dict(sorted(status_totals.items())),
            "non_released_requirements": len(incomplete),
            "production_evidence_records": len(records),
            "traceability_mismatches": len(traceability_mismatches),
        },
        "profile_status": {p: dict(sorted(by_profile[p].items())) for p in PROFILES},
        "unmapped_by_owner_domain": dict(sorted(by_domain.items(), key=lambda kv: (-kv[1], kv[0]))),
        "owner_only_sections": unmapped,
        "traceability_mismatches": traceability_mismatches,
        "requirements_backlog_dependency_order": incomplete,
        "section_index": section_inventory,
    }


def render_summary(inventory: dict[str, Any]) -> str:
    counts = inventory["totals"]
    lines = [
        "## APGIC v7: complete-canon execution gap inventory",
        "",
        f"- Canon sections: **{counts['canon_sections']}**",
        f"- Explicit requirement-linked sections: **{counts['explicitly_tracked_sections']}**",
        f"- Owner-only sections requiring human analysis: **{counts['owner_only_sections_needing_analysis']}**",
        f"- Requirements: **{counts['requirements']}**; not RELEASED: **{counts['non_released_requirements']}**",
        f"- Production evidence records: **{counts['production_evidence_records']}**",
        f"- Frozen-coverage / registry link inconsistencies: **{counts['traceability_mismatches']}**",
        "",
        "| Release | Status counts |",
        "|---|---|",
    ]
    for profile, statuses in inventory["profile_status"].items():
        lines.append(f"| {profile} | {', '.join(f'{k}: {v}' for k, v in statuses.items()) or '—'} |")
    lines.extend([
        "",
        "Owner-only means NOT explicitly mapped, **not** that code is absent.",
        "Use the JSON artifact for every canon section, requirement, dependency and evidence reference.",
        "Never promote status solely from this inventory or from green CI.",
        "",
    ])
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--coverage", type=Path, default=COVERAGE)
    parser.add_argument("--registry", type=Path, default=REGISTRY)
    parser.add_argument("--production-index", type=Path, default=PRODUCTION_INDEX)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--summary", type=Path)
    parser.add_argument("--strict-complete", action="store_true", help="For FINAL all-canon acceptance only")
    args = parser.parse_args()
    try:
        inventory = build_inventory(
            json.loads(args.coverage.read_text(encoding="utf-8")),
            yaml.safe_load(args.registry.read_text(encoding="utf-8")),
            yaml.safe_load(args.production_index.read_text(encoding="utf-8")),
        )
    except (ValueError, OSError, KeyError, TypeError, yaml.YAMLError) as exc:
        print(f"CANON EXECUTION INVENTORY: FAIL: {exc}", file=sys.stderr)
        return 1

    document = json.dumps(inventory, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(document, encoding="utf-8")
    else:
        print(document)
    if args.summary:
        args.summary.parent.mkdir(parents=True, exist_ok=True)
        with args.summary.open("a", encoding="utf-8") as stream:
            stream.write(render_summary(inventory))
    counts = inventory["totals"]
    print(
        f"CANON EXECUTION INVENTORY: sections={counts['canon_sections']}; "
        f"owner-only={counts['owner_only_sections_needing_analysis']}; "
        f"non-released={counts['non_released_requirements']}",
        file=sys.stderr,
    )
    if args.strict_complete and (
        counts["owner_only_sections_needing_analysis"] or counts["non_released_requirements"]
        or counts["traceability_mismatches"]
        or not counts["production_evidence_records"]
    ):
        print("CANON EXECUTION INVENTORY: final full-canon acceptance BLOCKED", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
