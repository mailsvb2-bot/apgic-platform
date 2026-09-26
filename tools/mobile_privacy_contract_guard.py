#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
DEPENDENCIES = ROOT / "apps/mobile/dependency-registry.yaml"
DECLARATION = ROOT / "apps/mobile/privacy-declaration.yaml"
SCHEMA = ROOT / "contracts/jsonschema/mobile-privacy-declaration-v1.schema.json"

FORBIDDEN_RAW = {"RAW_CONSULTATION", "RAW_PERSONA", "FINANCIAL_EVIDENCE"}
REQUIRED_TOP = {
    "version","build_scope","tracking","collected_data_classes","shared_data_classes",
    "required_permissions","required_reason_apis","runtime_dependency_assertions","store_artifacts",
}


def validate_mobile_privacy_contract(dependencies_doc: dict, declaration_doc: dict, schema_doc: dict) -> list[str]:
    errors: list[str] = []
    if set(schema_doc.get("required", [])) != REQUIRED_TOP:
        errors.append("mobile privacy declaration required fields differ")
    if schema_doc.get("additionalProperties") is not False:
        errors.append("mobile privacy declaration contract must reject undeclared fields")

    runtime = {
        entry.get("name"): entry
        for entry in dependencies_doc.get("dependencies", [])
        if isinstance(entry, dict) and entry.get("scope") == "RUNTIME"
    }
    assertions = declaration_doc.get("runtime_dependency_assertions") or {}
    if set(runtime) != set(assertions):
        errors.append(
            f"runtime privacy assertions differ from dependency registry: runtime={sorted(runtime)} assertions={sorted(assertions)}"
        )
    for name, entry in runtime.items():
        assertion = assertions.get(name) or {}
        for field in ("egress", "data_classes", "permissions"):
            if assertion.get(field) != entry.get(field):
                errors.append(f"{name}: privacy {field} differs from dependency registry")

    if declaration_doc.get("tracking") not in {True, False}:
        errors.append("tracking must be explicit boolean")
    collected = set(declaration_doc.get("collected_data_classes") or [])
    shared = set(declaration_doc.get("shared_data_classes") or [])
    leaked = sorted(FORBIDDEN_RAW & (collected | shared))
    if leaked:
        errors.append(f"forbidden raw data declared collected/shared: {leaked}")

    artifacts = declaration_doc.get("store_artifacts") or {}
    for key in ("apple_privacy_manifest", "google_data_safety"):
        row = artifacts.get(key) or {}
        if row.get("status") == "GENERATED_AND_VERIFIED" and row.get("production_submission") is not False:
            errors.append(f"{key}: repository evidence must not claim production submission")
        path = row.get("path")
        if isinstance(path, str) and path.strip():
            candidate = (ROOT / path).resolve()
            try:
                candidate.relative_to(ROOT)
            except ValueError:
                errors.append(f"{key}: path escapes repository")
            else:
                if not candidate.is_file():
                    errors.append(f"{key}: declared artifact path is missing")
    return errors


def main() -> int:
    errors = validate_mobile_privacy_contract(
        yaml.safe_load(DEPENDENCIES.read_text(encoding="utf-8")) or {},
        yaml.safe_load(DECLARATION.read_text(encoding="utf-8")) or {},
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("MOBILE PRIVACY CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("MOBILE PRIVACY CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
