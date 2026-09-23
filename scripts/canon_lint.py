#!/usr/bin/env python3
import json
import pathlib
import re
import sys

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]
REGISTRY = ROOT / "canon" / "APGIC_Requirement_Registry_v7_FINAL.yaml"
COVERAGE = ROOT / "canon" / "APGIC_Canon_Coverage_v7_FINAL.json"
EXCEPTIONS = ROOT / "canon" / "governance_exceptions.yaml"

EXPECTED_REQUIREMENTS = 109
EXPECTED_SECTIONS = 345
EXPECTED_LAUNCH_CUT = ["R0", "R1", "R2", "R3", "R4"]


def fail(message: str) -> None:
    print(f"CANON_LINT_FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def main() -> None:
    registry = yaml.safe_load(REGISTRY.read_text(encoding="utf-8"))
    coverage = json.loads(COVERAGE.read_text(encoding="utf-8"))
    exceptions_doc = yaml.safe_load(EXCEPTIONS.read_text(encoding="utf-8")) or {}
    exceptions = exceptions_doc.get("exceptions", [])

    requirements = registry.get("requirements", [])
    if len(requirements) != EXPECTED_REQUIREMENTS:
        fail(f"expected {EXPECTED_REQUIREMENTS} requirements, got {len(requirements)}")

    ids = [item.get("requirement_id") for item in requirements]
    if len(ids) != len(set(ids)):
        fail("duplicate requirement_id detected")
    known_ids = set(ids)

    if registry.get("meta", {}).get("launch_cut") != EXPECTED_LAUNCH_CUT:
        fail("launch_cut differs from v7 FINAL baseline")

    hash_value = registry.get("meta", {}).get("canonical_docx_sha256", "")
    if not re.fullmatch(r"[0-9a-f]{64}", hash_value):
        fail("canonical_docx_sha256 is missing or malformed")

    profiles = registry.get("release_profiles", {})
    for item in requirements:
        rid = item.get("requirement_id")
        for field in ("title", "statement", "owner_domain", "release_profile", "priority", "status", "acceptance"):
            if field not in item:
                fail(f"{rid}: missing {field}")
        if item["release_profile"] not in profiles:
            fail(f"{rid}: unknown release profile {item['release_profile']}")
        for dep in item.get("dependencies", []):
            if dep not in known_ids:
                fail(f"{rid}: unknown dependency {dep}")

    sections = coverage.get("sections", [])
    if len(sections) != EXPECTED_SECTIONS:
        fail(f"expected {EXPECTED_SECTIONS} coverage sections, got {len(sections)}")
    section_numbers = [item.get("section") for item in sections]
    if section_numbers != list(range(1, EXPECTED_SECTIONS + 1)):
        fail("coverage sections are not contiguous 1..345")
    for section in sections:
        for rid in section.get("linked_requirement_ids", []):
            if rid not in known_ids:
                fail(f"section {section.get('section')}: unknown requirement {rid}")

    rank = {name: index for index, name in enumerate(profiles)}
    allowed = {
        (item.get("subject_requirement"), item.get("dependency_requirement"))
        for item in exceptions
        if item.get("status") == "OPEN" and item.get("type") == "RELEASE_PROFILE_DEPENDENCY_ORDER"
    }
    for item in requirements:
        for dep in item.get("dependencies", []):
            dependency = next(candidate for candidate in requirements if candidate["requirement_id"] == dep)
            if rank[item["release_profile"]] < rank[dependency["release_profile"]]:
                pair = (item["requirement_id"], dep)
                if pair not in allowed:
                    fail(f"untracked release-order conflict: {pair[0]} -> {pair[1]}")
                print(f"CANON_LINT_GOVERNANCE_EXCEPTION: {pair[0]} -> {pair[1]}")

    print(f"CANON_LINT_OK requirements={len(requirements)} sections={len(sections)}")


if __name__ == "__main__":
    main()
