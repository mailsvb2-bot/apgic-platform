#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GO_OUTBOX = ROOT / "backend/internal/eventspine/outbox.go"
SCHEMA = ROOT / "contracts/jsonschema/event-envelope-v1.schema.json"

STATUS_RE = re.compile(r'\b(?:Pending|Delivered)\s+DeliveryStatus\s*=\s*"([A-Z0-9_]+)"')
GO_FIELD_RE = re.compile(r'^\s*(EventID|IdempotencyKey|EventType|SchemaVersion|AggregateRef|OccurredAt|ProducedAt|Producer|TenantScope|CorrelationID|CausationID|PayloadJSON)\s+', re.MULTILINE)

GO_TO_SCHEMA = {
    "EventID": "event_id",
    "IdempotencyKey": "idempotency_key",
    "EventType": "event_type",
    "SchemaVersion": "schema_version",
    "AggregateRef": "aggregate_ref",
    "OccurredAt": "occurred_at",
    "ProducedAt": "produced_at",
    "Producer": "producer",
    "TenantScope": "tenant_scope",
    "CorrelationID": "correlation_id",
    "CausationID": "causation_id",
    "PayloadJSON": "payload",
}
REQUIRED_BY_GO = {
    "event_id",
    "idempotency_key",
    "event_type",
    "schema_version",
    "aggregate_ref",
    "occurred_at",
    "producer",
    "correlation_id",
    "payload",
}


def validate_event_contract(go_text: str, schema_doc: dict) -> list[str]:
    errors: list[str] = []
    schema_props = set(schema_doc.get("properties", {}))
    go_fields = {GO_TO_SCHEMA[name] for name in GO_FIELD_RE.findall(go_text)}
    missing_props = go_fields - schema_props
    if missing_props:
        errors.append(f"event schema is missing Go envelope fields: {sorted(missing_props)}")

    required = set(schema_doc.get("required", []))
    missing_required = REQUIRED_BY_GO - required
    if missing_required:
        errors.append(f"event schema misses Go-required fields: {sorted(missing_required)}")

    if schema_doc.get("additionalProperties") is not False:
        errors.append("event envelope contract must reject undeclared fields")

    statuses = set(STATUS_RE.findall(go_text))
    if statuses != {"PENDING", "DELIVERED"}:
        errors.append(f"canonical Go delivery statuses drifted: {sorted(statuses)}")
    return errors


def main() -> int:
    errors = validate_event_contract(
        GO_OUTBOX.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("EVENT CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("EVENT CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
