#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GO_AUDIT = ROOT / "backend/internal/audit/audit.go"
MIGRATION = ROOT / "backend/migrations/000001_r0_foundation.sql"
SCHEMA = ROOT / "contracts/jsonschema/audit-evidence-v1.schema.json"

GO_FIELD_RE = re.compile(
    r'^\s*(ID|ActorID|Action|Scope|ResourceRef|OldState|NewState|Reason|PolicyVersion|OccurredAt|CorrelationID)\s+',
    re.MULTILINE,
)
GO_TO_SCHEMA = {
    "ID": "id",
    "ActorID": "actor_id",
    "Action": "action",
    "Scope": "scope",
    "ResourceRef": "resource_ref",
    "OldState": "old_state",
    "NewState": "new_state",
    "Reason": "reason",
    "PolicyVersion": "policy_version",
    "OccurredAt": "occurred_at",
    "CorrelationID": "correlation_id",
}
REQUIRED_EVIDENCE_FIELDS = {
    "id",
    "actor_id",
    "action",
    "scope",
    "reason",
    "policy_version",
    "occurred_at",
}


def validate_audit_contract(go_text: str, migration_text: str, schema_doc: dict) -> list[str]:
    errors: list[str] = []
    schema_props = set(schema_doc.get("properties", {}))
    go_fields = {GO_TO_SCHEMA[name] for name in GO_FIELD_RE.findall(go_text)}
    if go_fields != schema_props:
        errors.append(
            f"Go/JSON Schema audit fields differ: go={sorted(go_fields)} schema={sorted(schema_props)}"
        )

    required = set(schema_doc.get("required", []))
    if required != REQUIRED_EVIDENCE_FIELDS:
        errors.append(
            f"audit required evidence fields differ: expected={sorted(REQUIRED_EVIDENCE_FIELDS)} actual={sorted(required)}"
        )

    if schema_doc.get("additionalProperties") is not False:
        errors.append("audit evidence contract must reject undeclared fields")

    append_only_snippets = (
        "CREATE TABLE audit_records",
        "CREATE TRIGGER audit_records_append_only",
        "BEFORE UPDATE OR DELETE ON audit_records",
    )
    for snippet in append_only_snippets:
        if snippet not in migration_text:
            errors.append(f"audit append-only SQL invariant missing: {snippet}")
    return errors


def main() -> int:
    errors = validate_audit_contract(
        GO_AUDIT.read_text(encoding="utf-8"),
        MIGRATION.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("AUDIT CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("AUDIT CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
