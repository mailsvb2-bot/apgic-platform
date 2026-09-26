#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
MATRIX = ROOT / "config/jurisdiction/r0-ci-matrix.yaml"
LAUNCH = ROOT / "config/launch.ci.yaml"
SCHEMA = ROOT / "contracts/jsonschema/jurisdiction-matrix-v1.schema.json"

REQUIRED_ROLES = {
    "seller_or_service_provider",
    "commercial_owner",
    "payment_recipient",
    "fiscal_responsibility",
    "refund_responsibility",
    "payout_beneficiary",
}


def validate_jurisdiction_contract(matrix: dict, launch: dict, schema: dict) -> list[str]:
    errors: list[str] = []

    if schema.get("additionalProperties") is not False:
        errors.append("jurisdiction contract must reject undeclared top-level fields")

    props = schema.get("properties", {})
    if props.get("unknown_combination", {}).get("const") != "BLOCK":
        errors.append("jurisdiction unknown_combination must fail closed with BLOCK")

    role_schema = (
        props.get("jurisdictions", {})
        .get("items", {})
        .get("properties", {})
        .get("legal_financial_roles", {})
    )
    if set(role_schema.get("required", [])) != REQUIRED_ROLES:
        errors.append("jurisdiction legal/financial role contract drifted")

    if matrix.get("unknown_combination") != "BLOCK":
        errors.append("repository jurisdiction matrix does not block unknown combinations")
    rows = matrix.get("jurisdictions") or []
    if not isinstance(rows, list) or not rows:
        errors.append("repository jurisdiction matrix has no rows")
    for row in rows:
        if not isinstance(row, dict) or not row.get("code"):
            errors.append("jurisdiction row missing code")
            continue
        if row.get("enabled") is True:
            if not row.get("product_types") or not row.get("currencies"):
                errors.append(f"{row.get('code')}: enabled jurisdiction lacks product_types/currencies")
            roles = row.get("legal_financial_roles") or {}
            missing = REQUIRED_ROLES - set(roles)
            if missing:
                errors.append(f"{row.get('code')}: missing legal/financial roles {sorted(missing)}")

    if launch.get("jurisdiction_matrix_version") != matrix.get("version"):
        errors.append("launch jurisdiction version differs from matrix version")
    if launch.get("jurisdiction_matrix_path") != "config/jurisdiction/r0-ci-matrix.yaml":
        errors.append("launch jurisdiction path differs from canonical R0 CI matrix")
    return errors


def main() -> int:
    errors = validate_jurisdiction_contract(
        yaml.safe_load(MATRIX.read_text(encoding="utf-8")) or {},
        yaml.safe_load(LAUNCH.read_text(encoding="utf-8")) or {},
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("JURISDICTION CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("JURISDICTION CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
