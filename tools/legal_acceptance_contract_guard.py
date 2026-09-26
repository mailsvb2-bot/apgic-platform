#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GO_ACCEPTANCE = ROOT / "backend/internal/legal/acceptance.go"
MIGRATION = ROOT / "backend/migrations/000002_legal_acceptance_mobile_installation.sql"
SCHEMA = ROOT / "contracts/jsonschema/legal-acceptance-v1.schema.json"

GO_FIELD_RE = re.compile(
    r'^\s*(ID|IdentityID|DocumentID|DocumentVersion|EvidenceHash|AcceptedAt)\s+',
    re.MULTILINE,
)
GO_TO_SCHEMA = {
    "ID": "id",
    "IdentityID": "identity_id",
    "DocumentID": "document_id",
    "DocumentVersion": "document_version",
    "EvidenceHash": "evidence_hash",
    "AcceptedAt": "accepted_at",
}
FIELDS = set(GO_TO_SCHEMA.values())


def validate_legal_acceptance_contract(go_text: str, migration_text: str, schema_doc: dict) -> list[str]:
    errors: list[str] = []
    go_fields = {GO_TO_SCHEMA[name] for name in GO_FIELD_RE.findall(go_text)}
    schema_fields = set(schema_doc.get("properties", {}))
    if go_fields != FIELDS:
        errors.append(f"canonical Go legal acceptance fields drifted: {sorted(go_fields)}")
    if schema_fields != FIELDS:
        errors.append(
            f"Go/JSON Schema legal acceptance fields differ: expected={sorted(FIELDS)} actual={sorted(schema_fields)}"
        )

    required = set(schema_doc.get("required", []))
    if required != FIELDS:
        errors.append(
            f"legal acceptance required fields differ: expected={sorted(FIELDS)} actual={sorted(required)}"
        )

    for field in FIELDS - {"accepted_at"}:
        prop = schema_doc.get("properties", {}).get(field, {})
        if prop.get("type") != "string" or prop.get("minLength") != 1:
            errors.append(f"legal acceptance field must be non-empty string: {field}")

    accepted_at = schema_doc.get("properties", {}).get("accepted_at", {})
    if accepted_at.get("type") != "string" or accepted_at.get("format") != "date-time":
        errors.append("legal acceptance accepted_at must be date-time")

    if schema_doc.get("additionalProperties") is not False:
        errors.append("legal acceptance contract must reject undeclared fields")

    go_snippets = (
        "a.DocumentID == documentID && a.DocumentVersion == documentVersion",
        "acceptance.EvidenceHash == """,
        "acceptance.AcceptedAt.IsZero()",
    )
    for snippet in go_snippets:
        if snippet not in go_text:
            errors.append(f"legal acceptance Go invariant missing: {snippet}")

    sql_snippets = (
        "CREATE TABLE legal_acceptances",
        "document_version text NOT NULL",
        "evidence_hash text NOT NULL",
        "UNIQUE (identity_id, document_id, document_version, evidence_hash)",
        "CREATE TRIGGER legal_acceptances_append_only",
        "BEFORE UPDATE OR DELETE ON legal_acceptances",
    )
    for snippet in sql_snippets:
        if snippet not in migration_text:
            errors.append(f"legal acceptance SQL invariant missing: {snippet}")
    return errors


def main() -> int:
    errors = validate_legal_acceptance_contract(
        GO_ACCEPTANCE.read_text(encoding="utf-8"),
        MIGRATION.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("LEGAL ACCEPTANCE CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("LEGAL ACCEPTANCE CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
