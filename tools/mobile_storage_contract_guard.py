#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
TS = ROOT / "apps/mobile/src/storage-policy.ts"
SCHEMA = ROOT / "contracts/jsonschema/mobile-storage-policy-v1.schema.json"

DATA_BLOCK = re.compile(r'export type LocalDataClass\s*=\s*(.*?);', re.DOTALL)
TARGET_BLOCK = re.compile(r'export type StorageTarget\s*=\s*(.*?);', re.DOTALL)
VALUE_RE = re.compile(r'"([A-Z0-9_]+)"')

EXPECTED_CREDENTIAL_RULE = 'return target === "SECURE_STORAGE";'
RAW_CLASSES = {"RAW_CONSULTATION", "RAW_PERSONA", "FINANCIAL_EVIDENCE"}
RAW_RULE_FRAGMENT = 'return target === "MEMORY" || target === "EPHEMERAL_CACHE";'


def values(block_re: re.Pattern[str], text: str) -> set[str]:
    m = block_re.search(text)
    return set(VALUE_RE.findall(m.group(1))) if m else set()


def validate_mobile_storage_contract(ts_text: str, schema_doc: dict) -> list[str]:
    errors: list[str] = []
    defs = schema_doc.get("$defs", {})
    ts_classes = values(DATA_BLOCK, ts_text)
    ts_targets = values(TARGET_BLOCK, ts_text)
    schema_classes = set(defs.get("data_class", {}).get("enum", []))
    schema_targets = set(defs.get("storage_target", {}).get("enum", []))

    if ts_classes != schema_classes:
        errors.append(f"TypeScript/JSON Schema data classes differ: ts={sorted(ts_classes)} schema={sorted(schema_classes)}")
    if ts_targets != schema_targets:
        errors.append(f"TypeScript/JSON Schema storage targets differ: ts={sorted(ts_targets)} schema={sorted(schema_targets)}")
    if schema_doc.get("additionalProperties") is not False:
        errors.append("mobile storage policy contract must reject undeclared fields")

    required = set(schema_doc.get("required", []))
    if required != {"data_class", "storage_target", "allowed"}:
        errors.append(f"mobile storage required fields differ: {sorted(required)}")

    if EXPECTED_CREDENTIAL_RULE not in ts_text:
        errors.append("credential storage invariant missing: credentials must use SECURE_STORAGE")
    if RAW_RULE_FRAGMENT not in ts_text:
        errors.append("raw sensitive storage invariant missing: raw data must remain memory/ephemeral only")
    for data_class in RAW_CLASSES:
        if f'dataClass === "{data_class}"' not in ts_text:
            errors.append(f"raw sensitive DataClass missing from policy branch: {data_class}")
    return errors


def main() -> int:
    errors = validate_mobile_storage_contract(
        TS.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("MOBILE STORAGE CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("MOBILE STORAGE CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
