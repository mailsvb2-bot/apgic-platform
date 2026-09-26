#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCHEMA = ROOT / "contracts/jsonschema/accessibility-critical-flow-v1.schema.json"
E2E = ROOT / "apps/web/e2e/foundation.spec.ts"
CSS = ROOT / "apps/web/app/globals.css"
JOURNEY = ROOT / "apps/web/app/journey.tsx"

EXPECTED = {
    "ACCESSIBLE_LABELS",
    "KEYBOARD_COMPLETABLE",
    "FOCUS_VISIBLE",
    "WCAG_A_AA_AUTOMATED",
    "TEXT_SCALING_200_PERCENT",
    "NO_HORIZONTAL_OVERFLOW_AT_SCALE",
}


def validate_accessibility_contract(schema_doc: dict, e2e_text: str, css_text: str, journey_text: str) -> list[str]:
    errors: list[str] = []
    if schema_doc.get("additionalProperties") is not False:
        errors.append("accessibility contract must reject undeclared fields")

    invariants = set(
        schema_doc.get("properties", {})
        .get("required_invariants", {})
        .get("items", {})
        .get("enum", [])
    )
    if invariants != EXPECTED:
        errors.append("accessibility invariant set drifted")

    required_e2e = (
        'page.getByLabel("С чем нужна помощь")',
        'critical journey can be completed from the keyboard',
        '.withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])',
        'accessibility.violations).toEqual([])',
        'critical journey survives 200% text scaling',
        'document.documentElement.style.fontSize = "200%"',
        'expect(layout.documentWidth).toBeLessThanOrEqual(layout.viewportWidth + 1)',
    )
    for snippet in required_e2e:
        if snippet not in e2e_text:
            errors.append(f"accessibility E2E invariant missing: {snippet}")

    if "focus-visible" not in css_text or "outline:" not in css_text:
        errors.append("focus-visible outline styling is missing")
    if 'label htmlFor="request"' not in journey_text:
        errors.append("critical request control lacks explicit label association")
    return errors


def main() -> int:
    errors = validate_accessibility_contract(
        json.loads(SCHEMA.read_text(encoding="utf-8")),
        E2E.read_text(encoding="utf-8"),
        CSS.read_text(encoding="utf-8"),
        JOURNEY.read_text(encoding="utf-8"),
    )
    if errors:
        print("ACCESSIBILITY CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("ACCESSIBILITY CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
