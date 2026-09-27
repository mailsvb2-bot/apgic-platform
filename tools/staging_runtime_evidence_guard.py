#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import json
import sys
from datetime import datetime
from pathlib import Path

import yaml
from jsonschema import Draft202012Validator, FormatChecker

ROOT = Path(__file__).resolve().parents[1]
EVIDENCE_DIR = ROOT / "canon/evidence"
SCHEMA = EVIDENCE_DIR / "staging-runtime-evidence-v1.schema.json"
REGISTRY = ROOT / "canon/requirements/registry.yaml"

PINNED_ARTIFACTS = {
    "staging-runtime-20260926T215500Z.json": {
        "sha256": "c01a3207d528bfac38c8f5e0b8a4bf7b4cc61d4093959519370a9386f30e5eab",
        "candidate_sha": "08d06917f7575ad0aa7d129dfb6b6ed4370199f2",
        "deployment_identity": "staging-08d06917-20260926T2154Z",
    },
    "staging-runtime-20260927T072352Z.json": {
        "sha256": "cc8e09a08a9dd832fce5f6efb480b122d0f3662f1dff02c4447ec38ec52abe8f",
        "candidate_sha": "b55fd416c2a35cb22b6a5ea0b317a42f068e94b3",
        "deployment_identity": "staging-b55fd416-20260927T0718Z",
    },
}


def discover_evidence_paths() -> list[Path]:
    return sorted(
        path
        for path in EVIDENCE_DIR.glob("staging-runtime-*.json")
        if path != SCHEMA
    )


def _is_rfc3339_datetime(value: object) -> bool:
    if not isinstance(value, str) or not value:
        return False
    try:
        datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return False
    return "T" in value and (value.endswith("Z") or "+" in value[10:] or "-" in value[10:])


def _schema_errors(document: dict, schema: dict, ref: str) -> list[str]:
    validator = Draft202012Validator(schema, format_checker=FormatChecker())
    errors: list[str] = []
    for issue in sorted(validator.iter_errors(document), key=lambda err: list(err.absolute_path)):
        location = ".".join(str(part) for part in issue.absolute_path) or "<root>"
        errors.append(f"{ref}: schema violation at {location}: {issue.message}")

    date_fields = (
        ("generated_at", document.get("generated_at")),
        ("tls.not_before", document.get("tls", {}).get("not_before") if isinstance(document.get("tls"), dict) else None),
        ("tls.not_after", document.get("tls", {}).get("not_after") if isinstance(document.get("tls"), dict) else None),
    )
    for location, value in date_fields:
        if value is not None and not _is_rfc3339_datetime(value):
            message = f"{ref}: schema violation at {location}: {value!r} is not a valid RFC3339 date-time"
            if message not in errors:
                errors.append(message)
    return errors


def _registry_evidence_refs(registry_document: dict) -> dict[str, set[str]]:
    result: dict[str, set[str]] = {}
    for requirement in registry_document.get("requirements", []):
        requirement_id = requirement.get("requirement_id")
        if not isinstance(requirement_id, str) or not requirement_id:
            continue
        refs = requirement.get("evidence_refs") or []
        result[requirement_id] = {
            ref for ref in refs
            if isinstance(ref, str) and ref
        }
    return result


def _pin_errors(
    filename: str,
    raw: bytes,
    document: dict,
) -> list[str]:
    pin = PINNED_ARTIFACTS.get(filename)
    if pin is None:
        return [f"{filename}: staging evidence artifact is not reviewed/pinned"]

    errors: list[str] = []
    digest = hashlib.sha256(raw).hexdigest()
    if digest != pin["sha256"]:
        errors.append(
            f"{filename}: immutable digest mismatch expected={pin['sha256']} actual={digest}"
        )
    if document.get("candidate_sha") != pin["candidate_sha"]:
        errors.append(
            f"{filename}: candidate SHA changed from reviewed deployment identity"
        )
    if document.get("deployment_identity") != pin["deployment_identity"]:
        errors.append(
            f"{filename}: deployment identity changed from reviewed value"
        )
    return errors


def _validate_artifact(
    evidence_path: Path,
    schema: dict,
    registry_refs: dict[str, set[str]],
) -> list[str]:
    ref = f"canon/evidence/{evidence_path.name}"
    try:
        raw = evidence_path.read_bytes()
        document = json.loads(raw)
    except (OSError, json.JSONDecodeError) as exc:
        return [f"{ref}: unreadable JSON: {exc}"]

    errors = _schema_errors(document, schema, ref)
    errors.extend(_pin_errors(evidence_path.name, raw, document))

    supported_raw = document.get("supported_requirements", [])
    supported = {
        requirement_id for requirement_id in supported_raw
        if isinstance(requirement_id, str)
    } if isinstance(supported_raw, list) else set()

    for requirement_id in supported:
        if requirement_id not in registry_refs:
            errors.append(f"{ref}: registry requirement missing: {requirement_id}")
            continue
        if ref not in registry_refs[requirement_id]:
            errors.append(
                f"{ref}: registry requirement missing exact evidence_refs entry: {requirement_id}"
            )

    for requirement_id, evidence_refs in registry_refs.items():
        if ref in evidence_refs and requirement_id not in supported:
            errors.append(
                f"{ref}: evidence_refs over-claims unsupported requirement: {requirement_id}"
            )

    return errors


def validate() -> list[str]:
    errors: list[str] = []
    if not SCHEMA.is_file():
        errors.append("staging runtime evidence schema is missing")
    if not REGISTRY.is_file():
        errors.append("requirement registry is missing")
    if errors:
        return errors

    try:
        schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        return [f"staging runtime evidence schema is invalid: {exc}"]

    try:
        registry_document = yaml.safe_load(REGISTRY.read_text(encoding="utf-8"))
    except (OSError, yaml.YAMLError) as exc:
        return [f"requirement registry YAML is invalid: {exc}"]

    if not isinstance(registry_document, dict):
        return ["requirement registry root must be an object"]

    registry_refs = _registry_evidence_refs(registry_document)
    evidence_paths = discover_evidence_paths()
    discovered = {path.name for path in evidence_paths}
    pinned = set(PINNED_ARTIFACTS)

    if discovered != pinned:
        missing_pins = sorted(discovered - pinned)
        missing_artifacts = sorted(pinned - discovered)
        if missing_pins:
            errors.append(
                f"unreviewed staging evidence artifacts require immutable pins: {missing_pins}"
            )
        if missing_artifacts:
            errors.append(
                f"pinned staging evidence artifacts are missing: {missing_artifacts}"
            )

    for evidence_path in evidence_paths:
        errors.extend(_validate_artifact(evidence_path, schema, registry_refs))

    return errors


def main() -> int:
    errors = validate()
    if errors:
        print("STAGING RUNTIME EVIDENCE GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print(f"STAGING RUNTIME EVIDENCE GUARD: PASS ({len(discover_evidence_paths())} artifacts)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
