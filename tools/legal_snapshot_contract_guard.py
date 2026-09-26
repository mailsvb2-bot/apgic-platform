#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GO_SNAPSHOT = ROOT / "backend/internal/legal/transaction_snapshot.go"
MIGRATION = ROOT / "backend/migrations/000005_r0_legal_transaction_snapshot.sql"
SCHEMA = ROOT / "contracts/jsonschema/legal-transaction-snapshot-v1.schema.json"

GO_FIELD_RE = re.compile(
    r'^\s*(SellerOrServiceProviderID|CommercialOwnerID|PaymentRecipientID|PlatformRole|FiscalResponsibilityID|RefundResponsibilityID|PayoutBeneficiaryID|PolicyVersion)\s+',
    re.MULTILINE,
)
GO_TO_SCHEMA = {
    "SellerOrServiceProviderID": "seller_or_service_provider_id",
    "CommercialOwnerID": "commercial_owner_id",
    "PaymentRecipientID": "payment_recipient_id",
    "PlatformRole": "platform_role",
    "FiscalResponsibilityID": "fiscal_responsibility_id",
    "RefundResponsibilityID": "refund_responsibility_id",
    "PayoutBeneficiaryID": "payout_beneficiary_id",
    "PolicyVersion": "policy_version",
}
PERSISTED_FIELDS = {
    "transaction_ref",
    *GO_TO_SCHEMA.values(),
    "occurred_at",
}


def validate_legal_snapshot_contract(go_text: str, migration_text: str, schema_doc: dict) -> list[str]:
    errors: list[str] = []
    schema_fields = set(schema_doc.get("properties", {}))
    go_fields = {GO_TO_SCHEMA[name] for name in GO_FIELD_RE.findall(go_text)}
    if go_fields != set(GO_TO_SCHEMA.values()):
        errors.append(f"canonical Go legal snapshot fields drifted: {sorted(go_fields)}")
    if schema_fields != PERSISTED_FIELDS:
        errors.append(
            f"persisted legal snapshot fields differ: expected={sorted(PERSISTED_FIELDS)} actual={sorted(schema_fields)}"
        )

    required = set(schema_doc.get("required", []))
    if required != PERSISTED_FIELDS:
        errors.append(
            f"legal snapshot required fields differ: expected={sorted(PERSISTED_FIELDS)} actual={sorted(required)}"
        )

    for field in PERSISTED_FIELDS - {"occurred_at"}:
        prop = schema_doc.get("properties", {}).get(field, {})
        if prop.get("type") != "string" or prop.get("minLength") != 1:
            errors.append(f"legal snapshot field must be non-empty string: {field}")

    occurred = schema_doc.get("properties", {}).get("occurred_at", {})
    if occurred.get("type") != "string" or occurred.get("format") != "date-time":
        errors.append("legal snapshot occurred_at must be date-time")

    if schema_doc.get("additionalProperties") is not False:
        errors.append("legal snapshot contract must reject undeclared fields")

    sql_snippets = (
        "CREATE TABLE legal_transaction_snapshots",
        "transaction_ref text NOT NULL UNIQUE",
        "seller_or_service_provider_id text NOT NULL",
        "commercial_owner_id text NOT NULL",
        "payment_recipient_id text NOT NULL",
        "platform_role text NOT NULL",
        "fiscal_responsibility_id text NOT NULL",
        "refund_responsibility_id text NOT NULL",
        "payout_beneficiary_id text NOT NULL",
        "policy_version text NOT NULL",
        "occurred_at timestamptz NOT NULL",
        "CREATE TRIGGER legal_transaction_snapshots_append_only",
        "BEFORE UPDATE OR DELETE ON legal_transaction_snapshots",
    )
    for snippet in sql_snippets:
        if snippet not in migration_text:
            errors.append(f"legal snapshot SQL invariant missing: {snippet}")
    return errors


def main() -> int:
    errors = validate_legal_snapshot_contract(
        GO_SNAPSHOT.read_text(encoding="utf-8"),
        MIGRATION.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("LEGAL SNAPSHOT CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("LEGAL SNAPSHOT CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
