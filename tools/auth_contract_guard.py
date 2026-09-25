#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GO_AUTH = ROOT / "backend/internal/authz/authz.go"
TS_FOUNDATION = ROOT / "packages/contracts/src/foundation.ts"
AUTH_SCHEMA = ROOT / "contracts/jsonschema/authorization-v1.schema.json"

GO_DECISION_RE = re.compile(r'\b[A-Za-z0-9_]+\s+Decision\s*=\s*"([A-Z0-9_]+)"')
GO_RISK_RE = re.compile(r'\b[A-Za-z0-9_]+\s+Risk\s*=\s*"([A-Z0-9_]+)"')
TS_DECISION_BLOCK_RE = re.compile(
    r"export type AuthorizationDecision\s*=\s*(.*?);",
    re.DOTALL,
)
TS_VALUE_RE = re.compile(r'"([A-Z0-9_]+)"')


def extract_sets(go_text: str, ts_text: str, schema_doc: dict) -> tuple[set[str], set[str], set[str], set[str]]:
    go_decisions = set(GO_DECISION_RE.findall(go_text))
    go_risks = set(GO_RISK_RE.findall(go_text))
    match = TS_DECISION_BLOCK_RE.search(ts_text)
    ts_decisions = set(TS_VALUE_RE.findall(match.group(1))) if match else set()
    defs = schema_doc.get("$defs", {})
    schema_decisions = set(defs.get("AuthorizationDecision", {}).get("enum", []))
    schema_risks = set(defs.get("Risk", {}).get("enum", []))
    return go_decisions, go_risks, ts_decisions, schema_decisions | {f"__RISK__{x}" for x in schema_risks}


def validate_authorization_contract(go_text: str, ts_text: str, schema_doc: dict) -> list[str]:
    go_decisions, go_risks, ts_decisions, packed_schema = extract_sets(go_text, ts_text, schema_doc)
    schema_decisions = {x for x in packed_schema if not x.startswith("__RISK__")}
    schema_risks = {x.removeprefix("__RISK__") for x in packed_schema if x.startswith("__RISK__")}
    errors: list[str] = []
    if not go_decisions:
        errors.append("canonical Go authorization decision set is empty")
    if not go_risks:
        errors.append("canonical Go authorization risk set is empty")
    if go_decisions != ts_decisions:
        errors.append(f"Go/TypeScript authorization decisions differ: go={sorted(go_decisions)} ts={sorted(ts_decisions)}")
    if go_decisions != schema_decisions:
        errors.append(f"Go/JSON Schema authorization decisions differ: go={sorted(go_decisions)} schema={sorted(schema_decisions)}")
    if go_risks != schema_risks:
        errors.append(f"Go/JSON Schema authorization risks differ: go={sorted(go_risks)} schema={sorted(schema_risks)}")
    return errors


def main() -> int:
    errors = validate_authorization_contract(
        GO_AUTH.read_text(encoding="utf-8"),
        TS_FOUNDATION.read_text(encoding="utf-8"),
        json.loads(AUTH_SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("AUTH CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("AUTH CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
