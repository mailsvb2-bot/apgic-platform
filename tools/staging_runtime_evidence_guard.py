#!/usr/bin/env python3
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
SCHEMA = ROOT / "canon/evidence/staging-runtime-evidence-v1.schema.json"
EVIDENCE = ROOT / "canon/evidence/staging-runtime-20260926T215500Z.json"
REGISTRY = ROOT / "canon/requirements/registry.yaml"
EVIDENCE_REF = "canon/evidence/staging-runtime-20260926T215500Z.json"

EXPECTED_SHA = "08d06917f7575ad0aa7d129dfb6b6ed4370199f2"
EXPECTED_REQUIREMENTS = {
    "APGIC-EXEC-001",
    "APGIC-NFR-001",
    "APGIC-TEST-001",
    "APGIC-RELEASE-001",
    "APGIC-UI-001",
    "APGIC-DEMAND-001",
    "APGIC-MATCH-001",
    "APGIC-SEARCH-001",
    "APGIC-BOOK-002",
    "APGIC-PAY-001",
    "APGIC-NOTIF-001",
    "APGIC-CONSULT-001",
    "APGIC-COMM-002",
    "APGIC-CONSULT-002",
    "APGIC-PRIV-001",
}


def _require(condition: bool, message: str, errors: list[str]) -> None:
    if not condition:
        errors.append(message)


def _registry_blocks(text: str) -> dict[str, str]:
    blocks: dict[str, str] = {}
    current_id: str | None = None
    current_lines: list[str] = []

    def flush() -> None:
        if current_id is not None:
            blocks[current_id] = "\n".join(current_lines)

    for line in text.splitlines():
        if line.startswith("- requirement_id: "):
            flush()
            current_id = line.split(": ", 1)[1].strip()
            current_lines = [line]
        elif current_id is not None:
            current_lines.append(line)
    flush()
    return blocks


def validate() -> list[str]:
    errors: list[str] = []
    _require(SCHEMA.is_file(), "staging runtime evidence schema is missing", errors)
    _require(EVIDENCE.is_file(), "staging runtime evidence artifact is missing", errors)
    if errors:
        return errors

    schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
    evidence = json.loads(EVIDENCE.read_text(encoding="utf-8"))
    registry = REGISTRY.read_text(encoding="utf-8")

    _require(schema.get("properties", {}).get("production_release", {}).get("const") is False,
             "schema must fail closed with production_release=false", errors)
    _require(evidence.get("schema_version") == "staging-runtime-evidence-v1",
             "unexpected staging evidence schema version", errors)
    _require(evidence.get("evidence_class") == "STAGING_RUNTIME",
             "unexpected staging evidence class", errors)
    _require(evidence.get("production_release") is False,
             "staging evidence must never claim production release", errors)
    _require(evidence.get("environment") == "staging",
             "staging evidence environment mismatch", errors)
    _require(evidence.get("candidate_sha") == EXPECTED_SHA,
             "candidate SHA mismatch", errors)
    _require(evidence.get("deployment_identity") == "staging-08d06917-20260926T2154Z",
             "deployment identity mismatch", errors)
    _require(evidence.get("runtime", {}).get("meta_commit_sha") == EXPECTED_SHA,
             "runtime meta SHA mismatch", errors)
    _require(evidence.get("live_e2e", {}).get("tested_sha") == EXPECTED_SHA,
             "live E2E tested SHA mismatch", errors)
    _require(evidence.get("runtime", {}).get("healthz") == "ok",
             "healthz evidence is not ok", errors)
    _require(evidence.get("runtime", {}).get("readyz") == "ready",
             "readyz evidence is not ready", errors)
    _require(evidence.get("runtime", {}).get("watchdog_result") == "success",
             "runtime watchdog evidence is not success", errors)
    _require(evidence.get("database", {}).get("backup_result") == "success",
             "staging backup evidence is not success", errors)
    _require(evidence.get("database", {}).get("restore_verification_result") == "success",
             "staging restore verification evidence is not success", errors)
    _require(evidence.get("live_e2e", {}).get("tests_total") == 12,
             "live E2E total must remain 12 for this immutable evidence artifact", errors)
    _require(evidence.get("live_e2e", {}).get("tests_passed") == 12,
             "live E2E pass count mismatch", errors)
    _require(evidence.get("live_e2e", {}).get("tests_failed") == 0,
             "live E2E failure count must be zero", errors)

    supported = set(evidence.get("supported_requirements", []))
    _require(supported == EXPECTED_REQUIREMENTS,
             "supported requirement set drifted from reviewed evidence scope", errors)
    _require("APGIC-DR-001" not in supported,
             "staging evidence must not claim APGIC-DR-001 production restore proof", errors)

    limitations = "\n".join(evidence.get("limitations", []))
    _require("not production release approval" in limitations,
             "production limitation must be explicit", errors)
    _require("APGIC-DR-001 remains unproven" in limitations,
             "production restore limitation must be explicit", errors)
    _require("issue #3" in limitations,
             "native production evidence blocker must remain explicit", errors)

    blocks = _registry_blocks(registry)
    for requirement_id in EXPECTED_REQUIREMENTS:
        block = blocks.get(requirement_id)
        _require(block is not None, f"registry requirement missing: {requirement_id}", errors)
        if block is not None:
            _require(EVIDENCE_REF in block,
                     f"registry requirement missing staging evidence ref: {requirement_id}", errors)

    for requirement_id, block in blocks.items():
        if EVIDENCE_REF in block and requirement_id not in EXPECTED_REQUIREMENTS:
            errors.append(f"staging evidence over-claimed by unsupported requirement: {requirement_id}")

    return errors


def main() -> int:
    errors = validate()
    if errors:
        print("STAGING RUNTIME EVIDENCE GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("STAGING RUNTIME EVIDENCE GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
