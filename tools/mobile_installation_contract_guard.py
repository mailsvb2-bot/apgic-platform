#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GO_INSTALLATION = ROOT / "backend/internal/mobile/installation.go"
MIGRATION = ROOT / "backend/migrations/000002_legal_acceptance_mobile_installation.sql"
SCHEMA = ROOT / "contracts/jsonschema/mobile-installation-v1.schema.json"

STATE_RE = re.compile(r'Installation[A-Za-z0-9_]+\s+InstallationState\s*=\s*"([A-Z0-9_]+)"')
GO_FIELD_RE = re.compile(
    r'^\s*(ID|IdentityID|Platform|PushEndpoint|PushGeneration|State|UpdatedAt)\s+',
    re.MULTILINE,
)
GO_TO_SCHEMA = {
    "ID": "id",
    "IdentityID": "identity_id",
    "Platform": "platform",
    "PushEndpoint": "push_endpoint",
    "PushGeneration": "push_generation",
    "State": "state",
    "UpdatedAt": "updated_at",
}
FIELDS = set(GO_TO_SCHEMA.values())


def validate_mobile_installation_contract(go_text: str, migration_text: str, schema_doc: dict) -> list[str]:
    errors: list[str] = []

    go_states = set(STATE_RE.findall(go_text))
    schema_states = set(schema_doc.get("$defs", {}).get("state", {}).get("enum", []))
    if go_states != schema_states:
        errors.append(f"Go/JSON Schema installation states differ: go={sorted(go_states)} schema={sorted(schema_states)}")

    go_fields = {GO_TO_SCHEMA[name] for name in GO_FIELD_RE.findall(go_text)}
    schema_fields = set(schema_doc.get("properties", {}))
    if go_fields != schema_fields:
        errors.append(f"Go/JSON Schema installation fields differ: go={sorted(go_fields)} schema={sorted(schema_fields)}")

    required = set(schema_doc.get("required", []))
    expected_required = FIELDS - {"push_endpoint"}
    if required != expected_required:
        errors.append(f"installation required fields differ: expected={sorted(expected_required)} actual={sorted(required)}")

    platforms = set(schema_doc.get("$defs", {}).get("platform", {}).get("enum", []))
    if platforms != {"IOS", "ANDROID"}:
        errors.append(f"installation platforms differ: {sorted(platforms)}")

    generation = schema_doc.get("properties", {}).get("push_generation", {})
    if generation.get("type") != "integer" or generation.get("minimum") != 1:
        errors.append("push_generation must be positive integer")

    if schema_doc.get("additionalProperties") is not False:
        errors.append("mobile installation contract must reject undeclared fields")

    go_snippets = (
        "i.PushGeneration++",
        'i.State = InstallationRevoked',
        'i.PushEndpoint = ""',
        "i.State == InstallationActive",
        "i.PushGeneration == generation",
    )
    for snippet in go_snippets:
        if snippet not in go_text:
            errors.append(f"mobile installation Go invariant missing: {snippet}")

    sql_snippets = (
        "CREATE TABLE client_installations",
        "platform text NOT NULL CHECK (platform IN ('IOS', 'ANDROID'))",
        "push_generation bigint NOT NULL DEFAULT 1 CHECK (push_generation > 0)",
        "state text NOT NULL CHECK (state IN ('ACTIVE', 'REVOKED'))",
        "CREATE UNIQUE INDEX client_installations_active_push_endpoint_idx",
        "WHERE state = 'ACTIVE' AND push_endpoint IS NOT NULL",
    )
    for snippet in sql_snippets:
        if snippet not in migration_text:
            errors.append(f"mobile installation SQL invariant missing: {snippet}")
    return errors


def main() -> int:
    errors = validate_mobile_installation_contract(
        GO_INSTALLATION.read_text(encoding="utf-8"),
        MIGRATION.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("MOBILE INSTALLATION CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("MOBILE INSTALLATION CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
