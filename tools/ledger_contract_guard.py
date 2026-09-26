#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GO_LEDGER = ROOT / "backend/internal/ledger/ledger.go"
MIGRATION = ROOT / "backend/migrations/000001_r0_foundation.sql"
SCHEMA = ROOT / "contracts/jsonschema/ledger-entry-v1.schema.json"

GO_FIELD_RE = re.compile(
    r'^\s*(ID|DebitAccountRef|CreditAccountRef|AmountMinor|Currency|ProviderEvidenceRef|EconomicEventRef|CorrelationID|OccurredAt)\s+',
    re.MULTILINE,
)
GO_TO_SCHEMA = {
    "ID": "id",
    "DebitAccountRef": "debit_account_ref",
    "CreditAccountRef": "credit_account_ref",
    "AmountMinor": "amount_minor",
    "Currency": "currency",
    "ProviderEvidenceRef": "provider_evidence_ref",
    "EconomicEventRef": "economic_event_ref",
    "CorrelationID": "correlation_id",
    "OccurredAt": "occurred_at",
}
REQUIRED_ENTRY_FIELDS = {
    "id",
    "debit_account_ref",
    "credit_account_ref",
    "amount_minor",
    "currency",
    "provider_evidence_ref",
    "correlation_id",
    "occurred_at",
}


def validate_ledger_contract(go_text: str, migration_text: str, schema_doc: dict) -> list[str]:
    errors: list[str] = []
    schema_props = set(schema_doc.get("properties", {}))
    go_fields = {GO_TO_SCHEMA[name] for name in GO_FIELD_RE.findall(go_text)}
    if go_fields != schema_props:
        errors.append(
            f"Go/JSON Schema ledger fields differ: go={sorted(go_fields)} schema={sorted(schema_props)}"
        )

    required = set(schema_doc.get("required", []))
    if required != REQUIRED_ENTRY_FIELDS:
        errors.append(
            f"ledger required fields differ: expected={sorted(REQUIRED_ENTRY_FIELDS)} actual={sorted(required)}"
        )

    props = schema_doc.get("properties", {})
    if props.get("amount_minor", {}).get("minimum") != 1:
        errors.append("ledger amount_minor must be positive minor units")
    if props.get("currency", {}).get("pattern") != "^[A-Z]{3}$":
        errors.append("ledger currency must use canonical three-letter uppercase code")
    if schema_doc.get("additionalProperties") is not False:
        errors.append("ledger entry contract must reject undeclared fields")

    append_only_snippets = (
        "CREATE TABLE ledger_entries",
        "CREATE TRIGGER ledger_entries_append_only",
        "BEFORE UPDATE OR DELETE ON ledger_entries",
        "amount_minor bigint NOT NULL CHECK (amount_minor > 0)",
        "currency char(3) NOT NULL CHECK (currency = upper(currency))",
        "provider_evidence_ref text NOT NULL",
    )
    for snippet in append_only_snippets:
        if snippet not in migration_text:
            errors.append(f"ledger SQL invariant missing: {snippet}")
    return errors


def main() -> int:
    errors = validate_ledger_contract(
        GO_LEDGER.read_text(encoding="utf-8"),
        MIGRATION.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("LEDGER CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("LEDGER CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
