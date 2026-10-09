#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
MATRIX = ROOT / "config/providers/r0-ci-matrix.yaml"
LAUNCH = ROOT / "config/launch.ci.yaml"
SCHEMA = ROOT / "contracts/jsonschema/provider-matrix-v1.schema.json"

REQUIRED_CAPABILITY_FIELDS = {
    "enabled", "primary_provider", "fallback_providers", "environment",
    "jurisdictions", "regions", "credential_owner", "credential_ref_contract",
    "scopes", "webhook_event_contract_version", "data_classes_transmitted",
    "retention_subprocessor_terms_ref", "timeout_seconds", "retry_policy",
    "idempotency_contract", "reconciliation_method", "health_policy",
    "kill_switch", "degraded_behavior", "certification_status",
    "exit_semantics", "provider_neutral_contract",
}


def validate_provider_matrix_contract(matrix: dict, launch: dict, schema: dict) -> list[str]:
    errors: list[str] = []
    if schema.get("additionalProperties") is not False:
        errors.append("provider matrix contract must reject undeclared top-level fields")

    capability_schema = schema.get("properties", {}).get("capabilities", {}).get("additionalProperties", {})
    if set(capability_schema.get("required", [])) != REQUIRED_CAPABILITY_FIELDS:
        errors.append("provider capability contract required fields drifted")
    if capability_schema.get("properties", {}).get("provider_neutral_contract", {}).get("const") is not True:
        errors.append("provider capability contract must remain provider-neutral")

    capabilities = matrix.get("capabilities") or {}
    providers = matrix.get("providers") or {}
    if not isinstance(capabilities, dict) or not capabilities:
        errors.append("repository provider matrix lacks capabilities")
    if not isinstance(providers, dict) or not providers:
        errors.append("repository provider matrix lacks providers")

    for name, row in capabilities.items():
        if not isinstance(row, dict):
            errors.append(f"{name}: capability row must be mapping")
            continue
        if row.get("enabled") is not True:
            continue
        missing = REQUIRED_CAPABILITY_FIELDS - set(row)
        if missing:
            errors.append(f"{name}: missing capability fields {sorted(missing)}")
        primary = row.get("primary_provider")
        if not primary or primary not in providers:
            errors.append(f"{name}: primary provider missing from provider registry")
        fallback = row.get("fallback_providers")
        if not isinstance(fallback, list):
            errors.append(f"{name}: fallback_providers must be list")
        else:
            unknown = [p for p in fallback if p not in providers]
            if unknown:
                errors.append(f"{name}: unknown fallback providers {unknown}")
        if row.get("provider_neutral_contract") is not True:
            errors.append(f"{name}: provider-neutral contract invariant drifted")
        if row.get("environment") != "CI":
            errors.append(f"{name}: canonical R0 CI provider row must use environment=CI")
        for field in ("jurisdictions", "regions", "scopes"):
            value = row.get(field)
            if not isinstance(value, list) or not value:
                errors.append(f"{name}: {field} must be an explicit non-empty list")
        credential_ref = row.get("credential_ref_contract")
        if not isinstance(credential_ref, str) or not credential_ref.startswith(("secretref://", "managed-secret://")):
            errors.append(f"{name}: credential reference contract is invalid")
        timeout = row.get("timeout_seconds")
        if not isinstance(timeout, int) or isinstance(timeout, bool) or timeout <= 0:
            errors.append(f"{name}: timeout_seconds must be a positive integer")
        for provider_id in [primary, *(fallback if isinstance(fallback, list) else [])]:
            provider = providers.get(provider_id)
            if not isinstance(provider, dict):
                continue
            if provider.get("certified") is not True:
                errors.append(f"{name}: selected provider {provider_id} must be certified")
            declared = provider.get("capabilities")
            if not isinstance(declared, list) or name not in declared:
                errors.append(f"{name}: selected provider {provider_id} must declare capability")

    if launch.get("provider_matrix_version") != matrix.get("version"):
        errors.append("launch provider version differs from matrix version")
    if launch.get("provider_matrix_path") != "config/providers/r0-ci-matrix.yaml":
        errors.append("launch provider path differs from canonical R0 CI matrix")
    return errors


def main() -> int:
    errors = validate_provider_matrix_contract(
        yaml.safe_load(MATRIX.read_text(encoding="utf-8")) or {},
        yaml.safe_load(LAUNCH.read_text(encoding="utf-8")) or {},
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("PROVIDER MATRIX CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("PROVIDER MATRIX CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
