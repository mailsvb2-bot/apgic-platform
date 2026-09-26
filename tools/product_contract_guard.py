#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GO_PRODUCT = ROOT / "backend/internal/commerce/product.go"
MIGRATION_FOUNDATION = ROOT / "backend/migrations/000001_r0_foundation.sql"
MIGRATION_SEMANTICS = ROOT / "backend/migrations/000003_r0_semantic_invariants.sql"
SCHEMA = ROOT / "contracts/jsonschema/product-ownership-v1.schema.json"

OWNER_TYPE_RE = re.compile(r'Owner[A-Za-z0-9_]+\s+OwnerType\s*=\s*"([A-Z0-9_]+)"')
GO_FIELD_RE = re.compile(
    r'^\s*(OwnerType|OwnerID|CommercialOwnerRef|AuthorRefs|RevenueBeneficiaryRef)\s+',
    re.MULTILINE,
)
GO_TO_SCHEMA = {
    "OwnerType": "owner_type",
    "OwnerID": "owner_id",
    "CommercialOwnerRef": "commercial_owner_ref",
    "AuthorRefs": "author_refs",
    "RevenueBeneficiaryRef": "revenue_beneficiary_ref",
}
REQUIRED_FIELDS = set(GO_TO_SCHEMA.values())


def validate_product_contract(go_text: str, foundation_sql: str, semantics_sql: str, schema_doc: dict) -> list[str]:
    errors: list[str] = []
    go_owner_types = set(OWNER_TYPE_RE.findall(go_text))
    schema_owner_types = set(schema_doc.get("$defs", {}).get("owner_type", {}).get("enum", []))
    if go_owner_types != schema_owner_types:
        errors.append(
            f"Go/JSON Schema owner types differ: go={sorted(go_owner_types)} schema={sorted(schema_owner_types)}"
        )

    go_fields = {GO_TO_SCHEMA[name] for name in GO_FIELD_RE.findall(go_text)}
    schema_fields = set(schema_doc.get("properties", {}))
    if go_fields != schema_fields:
        errors.append(
            f"Go/JSON Schema ownership fields differ: go={sorted(go_fields)} schema={sorted(schema_fields)}"
        )

    required = set(schema_doc.get("required", []))
    if required != REQUIRED_FIELDS:
        errors.append(
            f"product ownership required fields differ: expected={sorted(REQUIRED_FIELDS)} actual={sorted(required)}"
        )

    author_refs = schema_doc.get("properties", {}).get("author_refs", {})
    if author_refs.get("minItems") != 1:
        errors.append("product author_refs must contain at least one explicit author")
    if schema_doc.get("additionalProperties") is not False:
        errors.append("product ownership contract must reject undeclared fields")

    foundation_snippets = (
        "CREATE TABLE products",
        "owner_type text NOT NULL CHECK (owner_type IN ('IDENTITY','ORGANIZATION'))",
        "commercial_owner_ref text NOT NULL",
        "revenue_beneficiary_ref text NOT NULL",
    )
    for snippet in foundation_snippets:
        if snippet not in foundation_sql:
            errors.append(f"product SQL invariant missing: {snippet}")

    semantics_snippets = (
        "ADD COLUMN author_refs text[] NOT NULL",
        "CHECK (cardinality(author_refs) > 0)",
        "CREATE TRIGGER products_owner_exists",
    )
    for snippet in semantics_snippets:
        if snippet not in semantics_sql:
            errors.append(f"product semantic SQL invariant missing: {snippet}")
    return errors


def main() -> int:
    errors = validate_product_contract(
        GO_PRODUCT.read_text(encoding="utf-8"),
        MIGRATION_FOUNDATION.read_text(encoding="utf-8"),
        MIGRATION_SEMANTICS.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("PRODUCT CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("PRODUCT CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
