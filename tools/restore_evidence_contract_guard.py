#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "backend/ci/restore_drill.sh"
SCHEMA = ROOT / "contracts/jsonschema/restore-drill-evidence-v1.schema.json"
WORKFLOW = ROOT / ".github/workflows/ci.yml"


def emitted_keys(script_text: str) -> list[str]:
    marker = 'cat >"$evidence_dir/restore-drill.json" <<JSON'
    if marker not in script_text:
        return []
    block = script_text.split(marker, 1)[1].split("\nJSON\n", 1)[0]
    return re.findall(r'^\s*"([^"]+)":', block, re.MULTILINE)


def validate_restore_evidence_contract(script_text: str, workflow_text: str, schema_doc: dict) -> list[str]:
    errors: list[str] = []
    keys = emitted_keys(script_text)
    required = schema_doc.get("required", [])
    properties = schema_doc.get("properties", {})

    if not keys:
        errors.append("restore drill no longer emits restore-drill.json evidence")
        return errors
    if set(keys) != set(required) or set(keys) != set(properties):
        errors.append("restore evidence JSON keys differ from versioned schema")

    if schema_doc.get("additionalProperties") is not False:
        errors.append("restore evidence contract must reject undeclared fields")
    if properties.get("evidence_type", {}).get("const") != "CI_RESTORE_DRILL":
        errors.append("restore evidence_type must remain CI_RESTORE_DRILL")
    if properties.get("production_evidence", {}).get("const") is not False:
        errors.append("CI restore evidence must not claim production evidence")
    if properties.get("observed_data_loss_records", {}).get("const") != 0:
        errors.append("CI restore drill must prove zero observed data-loss records")
    if properties.get("measured_restore_rto_ms", {}).get("minimum") != 0:
        errors.append("restore RTO must be measured as non-negative milliseconds")

    workflow_snippets = (
        "bash backend/ci/restore_drill.sh",
        "name: r0-restore-drill-evidence",
        "path: evidence/restore-drill.json",
        "if-no-files-found: error",
    )
    for snippet in workflow_snippets:
        if snippet not in workflow_text:
            errors.append(f"restore evidence CI invariant missing: {snippet}")
    return errors


def main() -> int:
    errors = validate_restore_evidence_contract(
        SCRIPT.read_text(encoding="utf-8"),
        WORKFLOW.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
    )
    if errors:
        print("RESTORE EVIDENCE CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("RESTORE EVIDENCE CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
