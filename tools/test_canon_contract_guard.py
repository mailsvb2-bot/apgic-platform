#!/usr/bin/env python3
from __future__ import annotations

import sys
from pathlib import Path
import yaml

ROOT = Path(__file__).resolve().parents[1]
CONTRACT = ROOT / "contracts/testing/state-machine-invariant-test-canon-v1.yaml"
WORKFLOW = ROOT / ".github/workflows/ci.yml"

REQUIRED_CLASSES = {
    "TRANSITIONS",
    "IDEMPOTENCY",
    "CONCURRENCY",
    "TIMEOUT_EXPIRY",
    "DEPENDENCY_FAILURE",
    "RECOVERY",
}


def validate_test_canon_contract(doc: dict, workflow_text: str) -> list[str]:
    errors: list[str] = []
    classes = doc.get("required_classes") or {}
    if set(classes) != REQUIRED_CLASSES:
        errors.append(f"test canon classes differ: {sorted(classes)}")

    for cls in sorted(REQUIRED_CLASSES):
        row = classes.get(cls) or {}
        evidence = row.get("evidence") or []
        markers = row.get("semantic_markers") or []
        if not evidence:
            errors.append(f"{cls}: executable evidence is required")
            continue
        if not markers:
            errors.append(f"{cls}: semantic markers are required")
        combined = ""
        for rel in evidence:
            path = ROOT / rel
            if not path.is_file():
                errors.append(f"{cls}: missing evidence file {rel}")
                continue
            combined += "\n" + path.read_text(encoding="utf-8")
        lowered = combined.lower()
        if markers and not any(str(marker).lower() in lowered for marker in markers):
            errors.append(f"{cls}: evidence lacks required semantic markers")

    for invocation in doc.get("ci_requirements") or []:
        if invocation not in workflow_text:
            errors.append(f"required CI invocation missing: {invocation}")

    behavior = doc.get("release_behavior") or {}
    for key in ("missing_required_class", "missing_evidence_file", "missing_ci_invocation"):
        if behavior.get(key) != "BLOCK":
            errors.append(f"{key} must BLOCK release")
    return errors


def main() -> int:
    errors = validate_test_canon_contract(
        yaml.safe_load(CONTRACT.read_text(encoding="utf-8")) or {},
        WORKFLOW.read_text(encoding="utf-8"),
    )
    if errors:
        print("TEST CANON CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("TEST CANON CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
