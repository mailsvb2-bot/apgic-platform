#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
TS = ROOT / "apps/mobile/src/device-capability.ts"
SCHEMA = ROOT / "contracts/jsonschema/mobile-device-capability-v1.schema.json"

CAP_BLOCK = re.compile(r'export type DeviceCapability\s*=\s*(.*?);', re.DOTALL)
STATE_BLOCK = re.compile(r'export type CapabilityState\s*=\s*(.*?);', re.DOTALL)
VALUE_RE = re.compile(r'"([A-Z0-9_]+)"')

ACTIONS = {"CONTINUE","REQUEST_PERMISSION","REFRESH_STATE","FALLBACK"}
REASONS = {"PERMISSION_DENIED","OS_RESTRICTED","CAPABILITY_UNAVAILABLE"}


def values(block_re: re.Pattern[str], text: str) -> set[str]:
    m = block_re.search(text)
    return set(VALUE_RE.findall(m.group(1))) if m else set()


def validate_mobile_capability_contract(ts_text: str, schema_doc: dict) -> list[str]:
    errors: list[str] = []
    defs = schema_doc.get("$defs", {})
    ts_caps = values(CAP_BLOCK, ts_text)
    ts_states = values(STATE_BLOCK, ts_text)
    schema_caps = set(defs.get("capability", {}).get("enum", []))
    schema_states = set(defs.get("state", {}).get("enum", []))
    schema_actions = set(defs.get("action", {}).get("enum", []))
    schema_reasons = set(defs.get("fallback_reason", {}).get("enum", []))

    if ts_caps != schema_caps:
        errors.append(f"TypeScript/JSON Schema device capabilities differ: ts={sorted(ts_caps)} schema={sorted(schema_caps)}")
    if ts_states != schema_states:
        errors.append(f"TypeScript/JSON Schema capability states differ: ts={sorted(ts_states)} schema={sorted(schema_states)}")
    if schema_actions != ACTIONS:
        errors.append(f"mobile capability actions differ: {sorted(schema_actions)}")
    if schema_reasons != REASONS:
        errors.append(f"mobile fallback reasons differ: {sorted(schema_reasons)}")
    if schema_doc.get("additionalProperties") is not False:
        errors.append("mobile capability contract must reject undeclared fields")

    required = set(schema_doc.get("required", []))
    if required != {"capability","state","action"}:
        errors.append(f"mobile capability required fields differ: {sorted(required)}")
    return errors


def main() -> int:
    errors = validate_mobile_capability_contract(
        TS.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("MOBILE CAPABILITY CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("MOBILE CAPABILITY CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
