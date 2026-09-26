#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
MATRIX = ROOT / "config/retention/r0-ci-matrix.yaml"
LAUNCH = ROOT / "config/launch.ci.yaml"
SCHEMA = ROOT / "contracts/jsonschema/retention-matrix-v1.schema.json"

REQUIRED_CLASSES = {
    "PUBLIC","INTERNAL","SENSITIVE","RAW_CONSULTATION",
    "RAW_PERSONA","FINANCIAL_EVIDENCE","CREDENTIAL",
}


def validate_retention_contract(matrix: dict, launch: dict, schema: dict) -> list[str]:
    errors: list[str] = []
    if schema.get("additionalProperties") is not False:
        errors.append("retention contract must reject undeclared top-level fields")
    classes_schema = schema.get("properties", {}).get("data_classes", {})
    if set(classes_schema.get("required", [])) != REQUIRED_CLASSES:
        errors.append("retention DataClass contract drifted")

    rows = matrix.get("data_classes")
    if not isinstance(rows, dict):
        errors.append("repository retention matrix lacks data_classes")
        rows = {}
    missing = REQUIRED_CLASSES - set(rows)
    if missing:
        errors.append(f"repository retention matrix missing DataClass entries: {sorted(missing)}")
    for name, row in rows.items():
        if not isinstance(row, dict):
            errors.append(f"{name}: retention row must be mapping")
            continue
        value = row.get("retention_seconds")
        if not isinstance(value, int) or isinstance(value, bool) or value < 0:
            errors.append(f"{name}: retention_seconds must be non-negative integer")
        if not row.get("deletion") or not row.get("legal_hold"):
            errors.append(f"{name}: deletion/legal_hold are required")

    for raw in ("RAW_CONSULTATION","RAW_PERSONA","CREDENTIAL"):
        if rows.get(raw, {}).get("retention_seconds") != 0:
            errors.append(f"{raw}: R0 CI retention must remain zero")

    if launch.get("retention_policy_version") != matrix.get("version"):
        errors.append("launch retention version differs from matrix version")
    if launch.get("retention_policy_path") != "config/retention/r0-ci-matrix.yaml":
        errors.append("launch retention path differs from canonical R0 CI matrix")
    return errors


def main() -> int:
    errors = validate_retention_contract(
        yaml.safe_load(MATRIX.read_text(encoding="utf-8")) or {},
        yaml.safe_load(LAUNCH.read_text(encoding="utf-8")) or {},
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("RETENTION CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("RETENTION CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
