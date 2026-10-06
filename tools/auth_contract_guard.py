#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
GO_AUTH = ROOT / "backend/internal/authz/authz.go"
TS_FOUNDATION = ROOT / "packages/contracts/src/foundation.ts"
AUTH_SCHEMA = ROOT / "contracts/jsonschema/authorization-v1.schema.json"
OPENAPI = ROOT / "contracts/openapi/apgic-v1.yaml"
AUTH001_WEB_E2E = ROOT / "tools/auth001_web_tenant_isolation_e2e.sh"
CI_WORKFLOW = ROOT / ".github/workflows/ci.yml"

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



CLIENT_SESSION_OPERATIONS = {
    ("/v1/help-intents/{id}/confirm", "post"),
    ("/v1/help-intents/{id}/matches", "get"),
    ("/v1/slot-holds", "post"),
    ("/v1/slot-holds/{id}/checkout-options", "get"),
    ("/v1/checkout-instructions", "post"),
    ("/v1/legal-acceptances", "post"),
    ("/v1/legal-acceptances/{documentID}/{documentVersion}", "get"),
    ("/v1/account-deletions", "post"),
    ("/v1/bookings/{id}/fulfillment", "get"),
}

CLIENT_ID_BODY_COMPAT = {
    "AcquireSlotHoldRequest": "client_identity_id",
    "CreateCheckoutInstructionRequest": "client_identity_id",
    "AccountDeletionRequest": "identity_id",
}

CLIENT_ID_QUERY_COMPAT = {
    ("/v1/slot-holds/{id}/checkout-options", "get"): "client_identity_id",
    ("/v1/bookings/{id}/fulfillment", "get"): "identity_id",
}


def validate_client_session_contract(document: dict) -> list[str]:
    errors: list[str] = []
    scheme = (
        document.get("components", {})
        .get("securitySchemes", {})
        .get("ClientSession", {})
    )
    if scheme.get("type") != "apiKey" or scheme.get("in") != "cookie" or scheme.get("name") != "__Host-apgic_session":
        errors.append("OpenAPI ClientSession must be the __Host-apgic_session cookie apiKey")

    paths = document.get("paths", {})
    for path, method in sorted(CLIENT_SESSION_OPERATIONS):
        operation = (paths.get(path) or {}).get(method) or {}
        security = operation.get("security") or []
        if not any(isinstance(item, dict) and item.get("ClientSession") == [] for item in security):
            errors.append(f"{method.upper()} {path} must require ClientSession")

    schemas = document.get("components", {}).get("schemas", {})
    for schema_name, property_name in CLIENT_ID_BODY_COMPAT.items():
        schema = schemas.get(schema_name) or {}
        required = set(schema.get("required") or [])
        prop = (schema.get("properties") or {}).get(property_name) or {}
        if property_name in required:
            errors.append(f"{schema_name}.{property_name} must be optional compatibility input")
        if prop.get("deprecated") is not True:
            errors.append(f"{schema_name}.{property_name} must be deprecated")

    for (path, method), parameter_name in CLIENT_ID_QUERY_COMPAT.items():
        operation = (paths.get(path) or {}).get(method) or {}
        parameters = operation.get("parameters") or []
        parameter = next(
            (
                item
                for item in parameters
                if isinstance(item, dict)
                and item.get("name") == parameter_name
                and item.get("in") == "query"
            ),
            None,
        )
        if parameter is None:
            errors.append(f"{method.upper()} {path} missing compatibility query {parameter_name}")
            continue
        if parameter.get("required") is True:
            errors.append(f"{method.upper()} {path} {parameter_name} must not be required")
        if parameter.get("deprecated") is not True:
            errors.append(f"{method.upper()} {path} {parameter_name} must be deprecated")
    return errors


def validate_auth001_web_surface_proof(web_e2e_text: str, workflow_text: str) -> list[str]:
    errors: list[str] = []
    required_script = (
        'APGIC_API_ORIGIN="$API_ORIGIN"',
        'npm run start -- --hostname 127.0.0.1',
        '/v1/organizations/${org_b}/private-profile',
        'X-Organization-Context: $org_a',
        'X-Organization-Context: $org_b',
        'AUTH_CROSS_TENANT_DENY',
        'AUTH_TENANT_CONTEXT_DENIED',
        'TOP SECRET ORGANIZATION B',
        'FROM audit_records',
        '"web_proxy_exercised": true',
        '"postgres_persistence_exercised": true',
        '"private_resource_disclosed": false',
        '"audit_evidence_persisted": true',
    )
    for snippet in required_script:
        if snippet not in web_e2e_text:
            errors.append(f"AUTH-001 WEB E2E invariant missing: {snippet}")

    required_workflow = (
        "Prove AUTH-001 tenant isolation through WEB proxy",
        "bash tools/auth001_web_tenant_isolation_e2e.sh",
        "auth001-web-tenant-isolation",
        "evidence/auth001-web-tenant-isolation.json",
    )
    for snippet in required_workflow:
        if snippet not in workflow_text:
            errors.append(f"AUTH-001 WEB CI proof missing: {snippet}")
    return errors


def main() -> int:
    errors = validate_authorization_contract(
        GO_AUTH.read_text(encoding="utf-8"),
        TS_FOUNDATION.read_text(encoding="utf-8"),
        json.loads(AUTH_SCHEMA.read_text(encoding="utf-8")),
    )
    errors.extend(
        validate_client_session_contract(
            yaml.safe_load(OPENAPI.read_text(encoding="utf-8"))
        )
    )
    errors.extend(
        validate_auth001_web_surface_proof(
            AUTH001_WEB_E2E.read_text(encoding="utf-8"),
            CI_WORKFLOW.read_text(encoding="utf-8"),
        )
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
