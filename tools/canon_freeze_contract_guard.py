#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
RFC = ROOT / "canon/requirements/approved-rfcs.yaml"
TEMPLATE = ROOT / ".github/ISSUE_TEMPLATE/requirement-change.yml"
CANON_LINT = ROOT / "tools/canon_lint.py"
SCHEMA = ROOT / "contracts/jsonschema/requirement-change-rfc-v1.schema.json"

ALLOWED_REASONS = {
    "LEGAL_BLOCKER",
    "SECURITY_BLOCKER",
    "FINANCIAL_INTEGRITY_BLOCKER",
    "MISSING_DEPENDENCY_BLOCKER",
    "POST_LAUNCH_BACKLOG",
}
IMMUTABLE_FIELDS = {
    "requirement_id","title","statement","owner_domain","source_sections",
    "release_profile","risk","priority","dependencies","acceptance",
}


def validate_canon_freeze_contract(rfc_doc: dict, template_text: str, lint_text: str, schema: dict) -> list[str]:
    errors: list[str] = []
    item = schema.get("properties", {}).get("rfcs", {}).get("items", {})
    props = item.get("properties", {})
    if set(props.get("reason_class", {}).get("enum", [])) != ALLOWED_REASONS:
        errors.append("RFC reason classes differ from canonical governance set")
    if set(props.get("field", {}).get("enum", [])) != IMMUTABLE_FIELDS:
        errors.append("RFC immutable-field contract drifted")
    if item.get("additionalProperties") is not False or schema.get("additionalProperties") is not False:
        errors.append("RFC contract must reject undeclared fields")

    for reason in ALLOWED_REASONS:
        if reason not in template_text:
            errors.append(f"Requirement Change template missing reason class {reason}")

    lint_match = re.search(r"IMMUTABLE_REQUIREMENT_FIELDS\s*=\s*\((.*?)\)", lint_text, re.DOTALL)
    lint_fields = set(re.findall(r'"([^"]+)"', lint_match.group(1))) if lint_match else set()
    if lint_fields != IMMUTABLE_FIELDS:
        errors.append(f"canon_lint immutable fields differ: {sorted(lint_fields)}")

    seen: set[str] = set()
    for row in rfc_doc.get("rfcs") or []:
        rid = row.get("id")
        if not rid or rid in seen:
            errors.append(f"duplicate or missing RFC id: {rid!r}")
        seen.add(rid)
        if row.get("status") != "APPROVED":
            errors.append(f"{rid}: recorded RFC must be APPROVED")
        if row.get("reason_class") not in ALLOWED_REASONS:
            errors.append(f"{rid}: invalid reason_class")
        if row.get("field") not in IMMUTABLE_FIELDS:
            errors.append(f"{rid}: invalid immutable field")
        if not str(row.get("decision") or "").strip():
            errors.append(f"{rid}: decision is required")
    return errors


def main() -> int:
    errors = validate_canon_freeze_contract(
        yaml.safe_load(RFC.read_text(encoding="utf-8")) or {},
        TEMPLATE.read_text(encoding="utf-8"),
        CANON_LINT.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("CANON FREEZE CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("CANON FREEZE CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
