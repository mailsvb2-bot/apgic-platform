#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GO_IDENTITY = ROOT / "backend/internal/identity/identity.go"
TS_FOUNDATION = ROOT / "packages/contracts/src/foundation.ts"
IDENTITY_SCHEMA = ROOT / "contracts/jsonschema/identity-v1.schema.json"

GO_ROLE_RE = re.compile(r'Role[A-Za-z0-9_]+\s+Role\s*=\s*"([A-Z0-9_]+)"')
TS_ROLE_BLOCK_RE = re.compile(
    r"export type IdentityRole\s*=\s*(.*?);",
    re.DOTALL,
)
TS_ROLE_RE = re.compile(r'"([A-Z0-9_]+)"')


def extract_role_sets(go_text: str, ts_text: str, schema_doc: dict) -> tuple[set[str], set[str], set[str]]:
    go_roles = set(GO_ROLE_RE.findall(go_text))
    match = TS_ROLE_BLOCK_RE.search(ts_text)
    ts_roles = set(TS_ROLE_RE.findall(match.group(1))) if match else set()
    schema_roles = set(
        schema_doc.get("properties", {})
        .get("roles", {})
        .get("items", {})
        .get("enum", [])
    )
    return go_roles, ts_roles, schema_roles


def validate_identity_role_parity(go_text: str, ts_text: str, schema_doc: dict) -> list[str]:
    go_roles, ts_roles, schema_roles = extract_role_sets(go_text, ts_text, schema_doc)
    errors: list[str] = []
    if not go_roles:
        errors.append("canonical Go identity role set is empty")
    if go_roles != ts_roles:
        errors.append(f"Go/TypeScript identity roles differ: go={sorted(go_roles)} ts={sorted(ts_roles)}")
    if go_roles != schema_roles:
        errors.append(f"Go/JSON Schema identity roles differ: go={sorted(go_roles)} schema={sorted(schema_roles)}")
    return errors


def main() -> int:
    errors = validate_identity_role_parity(
        GO_IDENTITY.read_text(encoding="utf-8"),
        TS_FOUNDATION.read_text(encoding="utf-8"),
        json.loads(IDENTITY_SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("IDENTITY CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("IDENTITY CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
