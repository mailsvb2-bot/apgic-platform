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
HTTP_API = ROOT / "backend/internal/httpapi/mobile_installation.go"
POSTGRES_STORE = ROOT / "backend/internal/runtimepostgres/mobile_installation.go"
OPENAPI = ROOT / "contracts/openapi/apgic-v1.yaml"
NATIVE_CLIENT = ROOT / "apps/mobile/src/mobile-installation-client.ts"
ANDROID_BRIDGE = ROOT / "apps/mobile/android/app/src/main/java/com/apgic/ci/MainActivity.kt"
IOS_BRIDGE = ROOT / "apps/mobile/ios/APGIC/AppDelegate.swift"
ANDROID_E2E = ROOT / "tools/mobile_android_capability_e2e.sh"
IOS_E2E = ROOT / "tools/mobile_ios_capability_e2e.sh"

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




def validate_mobile_installation_runtime(http_text: str, store_text: str, openapi_text: str) -> list[str]:
    errors: list[str] = []
    http_snippets = (
        'POST /v1/mobile/installations',
        'GET /v1/mobile/installations',
        'PATCH /v1/mobile/installations/{installationID}/push-endpoint',
        'POST /v1/mobile/installations/{installationID}/revoke',
        'requiredClientSessionIdentity',
        'MOBILE_PUSH_ENDPOINT_CONFLICT',
    )
    for snippet in http_snippets:
        if snippet not in http_text:
            errors.append(f"mobile installation HTTP runtime missing: {snippet}")

    store_snippets = (
        "RegisterInstallation",
        "RotateInstallationPushEndpoint",
        "RevokeInstallation",
        "ListInstallations",
        "pg_advisory_xact_lock",
        "push-endpoint:",
        "FOR UPDATE",
    )
    for snippet in store_snippets:
        if snippet not in store_text:
            errors.append(f"mobile installation PostgreSQL runtime missing: {snippet}")

    openapi_snippets = (
        "/v1/mobile/installations:",
        "operationId: registerMobileInstallation",
        "operationId: listMobileInstallations",
        "operationId: rotateMobilePushEndpoint",
        "operationId: revokeMobileInstallation",
        "$ref: '#/components/schemas/ClientInstallation'",
        "ClientSession: []",
    )
    for snippet in openapi_snippets:
        if snippet not in openapi_text:
            errors.append(f"mobile installation OpenAPI contract missing: {snippet}")
    return errors

def validate_mobile_installation_native_e2e(
    client_text: str,
    android_bridge_text: str,
    ios_bridge_text: str,
    android_e2e_text: str,
    ios_e2e_text: str,
) -> list[str]:
    errors: list[str] = []
    client_snippets = (
        "registerMobileInstallation",
        "rotateMobilePushEndpoint",
        "revokeMobileInstallation",
        "listMobileInstallations",
        "runInstallationE2ELifecycle",
        "/v1/mobile/installations",
        "/push-endpoint",
        "/revoke",
        'credentials: "include"',
        "headers.Cookie = config.sessionCookie",
        "MOBILE_INSTALLATION_REGISTER_INVARIANT",
        "MOBILE_INSTALLATION_ROTATE_INVARIANT",
        "MOBILE_INSTALLATION_REVOKE_INVARIANT",
        "MOBILE_INSTALLATION_LIST_INVARIANT",
        "MOBILE_INSTALLATION_E2E_DISABLED",
    )
    for snippet in client_snippets:
        if snippet not in client_text:
            errors.append(f"mobile installation native client proof missing: {snippet}")

    e2e_body = client_text.split("export async function runInstallationE2ELifecycle", 1)
    if len(e2e_body) != 2:
        errors.append("mobile installation E2E orchestrator is missing")
    else:
        for production_call in (
            "registerMobileInstallation(",
            "rotateMobilePushEndpoint(",
            "revokeMobileInstallation(",
            "listMobileInstallations(",
        ):
            if production_call not in e2e_body[1]:
                errors.append(
                    "E2E must compose production mobile installation functions: "
                    + production_call
                )

    bridge_snippets = (
        "APGIC_E2E_INSTALLATION_BASE_URL",
        "APGIC_E2E_SESSION_COOKIE",
        "APGIC_E2E_INSTALLATION_ID",
        "APGIC_E2E_INSTALLATION_PLATFORM",
    )
    for snippet in bridge_snippets:
        if snippet not in android_bridge_text:
            errors.append(f"Android installation E2E bridge missing: {snippet}")
        if snippet not in ios_bridge_text:
            errors.append(f"iOS installation E2E bridge missing: {snippet}")
    if "BuildConfig.DEBUG" not in android_bridge_text:
        errors.append("Android installation E2E bridge must be debug-only")
    if "#if DEBUG" not in ios_bridge_text:
        errors.append("iOS installation E2E bridge must be debug-only")

    script_snippets = (
        "mobile-installation-e2e-server",
        "__Host-apgic_session=",
        "installation-e2e:PASS",
        "installation-e2e-state:REVOKED",
        "installation-e2e-generation:2",
        "/v1/mobile/installations",
    )
    for snippet in script_snippets:
        if snippet not in android_e2e_text:
            errors.append(f"Android installed-app E2E proof missing: {snippet}")
        if snippet not in ios_e2e_text:
            errors.append(f"iOS installed-app E2E proof missing: {snippet}")
    return errors


def main() -> int:
    errors = validate_mobile_installation_contract(
        GO_INSTALLATION.read_text(encoding="utf-8"),
        MIGRATION.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    errors.extend(
        validate_mobile_installation_runtime(
            HTTP_API.read_text(encoding="utf-8"),
            POSTGRES_STORE.read_text(encoding="utf-8"),
            OPENAPI.read_text(encoding="utf-8"),
        )
    )
    errors.extend(
        validate_mobile_installation_native_e2e(
            NATIVE_CLIENT.read_text(encoding="utf-8"),
            ANDROID_BRIDGE.read_text(encoding="utf-8"),
            IOS_BRIDGE.read_text(encoding="utf-8"),
            ANDROID_E2E.read_text(encoding="utf-8"),
            IOS_E2E.read_text(encoding="utf-8"),
        )
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
