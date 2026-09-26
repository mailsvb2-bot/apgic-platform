#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REGISTRY_GO = ROOT / "backend/internal/connector/registry.go"
CAPABILITY_GO = ROOT / "backend/internal/connector/capability.go"
WEBHOOK_GO = ROOT / "backend/internal/connector/webhook.go"
SCHEMA = ROOT / "contracts/jsonschema/connector-runtime-v1.schema.json"

CAPABILITY_RE = re.compile(r'Capability[A-Za-z0-9_]+\s+CapabilityClass\s*=\s*"([A-Z0-9_]+)"')
STATUS_RE = re.compile(r'Status[A-Za-z0-9_]+\s+Status\s*=\s*"([A-Z0-9_]+)"')
OUTCOME_RE = re.compile(r'Outcome[A-Za-z0-9_]+\s+Outcome\s*=\s*"([A-Z0-9_]+)"')
DELIVERY_RE = re.compile(r'Delivery[A-Za-z0-9_]+\s+DeliveryDecision\s*=\s*"([A-Z0-9_]+)"')


def enum_set(schema_doc: dict, name: str) -> set[str]:
    return set(schema_doc.get("$defs", {}).get(name, {}).get("enum", []))


def validate_connector_contract(registry_text: str, capability_text: str, webhook_text: str, schema_doc: dict) -> list[str]:
    pairs = [
        ("capability_class", set(CAPABILITY_RE.findall(registry_text))),
        ("status", set(STATUS_RE.findall(registry_text))),
        ("outcome", set(OUTCOME_RE.findall(capability_text))),
        ("delivery_decision", set(DELIVERY_RE.findall(webhook_text))),
    ]
    errors: list[str] = []
    for schema_name, go_values in pairs:
        schema_values = enum_set(schema_doc, schema_name)
        if not go_values:
            errors.append(f"canonical Go {schema_name} set is empty")
        if go_values != schema_values:
            errors.append(
                f"Go/JSON Schema {schema_name} differ: go={sorted(go_values)} schema={sorted(schema_values)}"
            )
    execute_scope = (
        schema_doc.get("properties", {})
        .get("instance", {})
        .get("properties", {})
        .get("execute_scope", {})
        .get("pattern")
    )
    if execute_scope != r"^connector:execute:[A-Z0-9_]+$":
        errors.append("connector execute_scope schema pattern is not canonical")
    return errors


def main() -> int:
    errors = validate_connector_contract(
        REGISTRY_GO.read_text(encoding="utf-8"),
        CAPABILITY_GO.read_text(encoding="utf-8"),
        WEBHOOK_GO.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("CONNECTOR CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("CONNECTOR CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
