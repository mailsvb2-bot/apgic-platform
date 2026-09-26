#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GO_SECURITY = ROOT / "backend/internal/security/service_principal.go"
ARCH_GUARD = ROOT / "tools/architecture_guard.py"
SCHEMA = ROOT / "contracts/jsonschema/service-principal-v1.schema.json"

GO_FIELD_RE = re.compile(r'^\s*(ID|Scopes)\s+', re.MULTILINE)
GO_TO_SCHEMA = {"ID": "id", "Scopes": "scopes"}


def validate_security_contract(go_text: str, architecture_guard_text: str, schema_doc: dict) -> list[str]:
    errors: list[str] = []
    go_fields = {GO_TO_SCHEMA[name] for name in GO_FIELD_RE.findall(go_text)}
    schema_fields = set(schema_doc.get("properties", {}))
    if go_fields != schema_fields:
        errors.append(
            f"Go/JSON Schema service principal fields differ: go={sorted(go_fields)} schema={sorted(schema_fields)}"
        )

    required = set(schema_doc.get("required", []))
    if required != {"id", "scopes"}:
        errors.append(f"service principal required fields differ: {sorted(required)}")

    scopes = schema_doc.get("properties", {}).get("scopes", {})
    if scopes.get("minItems") != 1:
        errors.append("service principal must require at least one scope")
    item = scopes.get("items", {})
    if item.get("not", {}).get("const") != "*":
        errors.append("service principal wildcard scope must be forbidden")

    if schema_doc.get("additionalProperties") is not False:
        errors.append("service principal contract must reject undeclared fields")

    go_snippets = (
        'scope == "" || scope == "*"',
        'strings.TrimSpace(id) == "" || len(scopes) == 0',
    )
    for snippet in go_snippets:
        if snippet not in go_text:
            errors.append(f"service principal Go invariant missing: {snippet}")

    if 'for path in ROOT.rglob(".env*")' not in architecture_guard_text:
        errors.append("architecture guard no longer scans secret-bearing env files")
    if 'path.name != ".env.example"' not in architecture_guard_text:
        errors.append("architecture guard secret env exception semantics drifted")
    return errors


def main() -> int:
    errors = validate_security_contract(
        GO_SECURITY.read_text(encoding="utf-8"),
        ARCH_GUARD.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("SECURITY CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("SECURITY CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
