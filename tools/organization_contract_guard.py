#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GO_ORGANIZATION = ROOT / "backend/internal/organization/organization.go"
SCHEMA = ROOT / "contracts/jsonschema/organization-v1.schema.json"

STATUS_RE = re.compile(r'Direction[A-Za-z0-9_]+\s+DirectionStatus\s*=\s*"([A-Z0-9_]+)"')


def validate_organization_contract(go_text: str, schema_doc: dict) -> list[str]:
    go_statuses = set(STATUS_RE.findall(go_text))
    schema_statuses = set(
        schema_doc.get("$defs", {})
        .get("direction_status", {})
        .get("enum", [])
    )
    errors: list[str] = []
    if not go_statuses:
        errors.append("canonical Go direction status set is empty")
    if go_statuses != schema_statuses:
        errors.append(
            f"Go/JSON Schema direction statuses differ: go={sorted(go_statuses)} schema={sorted(schema_statuses)}"
        )

    direction = schema_doc.get("$defs", {}).get("direction", {})
    required = set(direction.get("required", []))
    expected = {"id", "name", "status", "has_dependent_truth"}
    if required != expected:
        errors.append(
            f"organization direction required fields differ: expected={sorted(expected)} actual={sorted(required)}"
        )
    if direction.get("additionalProperties") is not False:
        errors.append("organization direction contract must reject undeclared fields")
    if schema_doc.get("additionalProperties") is not False:
        errors.append("organization contract must reject undeclared fields")
    return errors


def main() -> int:
    errors = validate_organization_contract(
        GO_ORGANIZATION.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("ORGANIZATION CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("ORGANIZATION CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
