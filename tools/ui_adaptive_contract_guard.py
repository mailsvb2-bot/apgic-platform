#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PLAYWRIGHT = ROOT / "apps/web/playwright.config.ts"
E2E = ROOT / "apps/web/e2e/foundation.spec.ts"
SCHEMA = ROOT / "contracts/jsonschema/adaptive-critical-flow-v1.schema.json"
VISUAL_E2E = ROOT / "apps/web/e2e/visual-regression.spec.ts"
VISUAL_BASELINES = (
    ROOT / "apps/web/e2e/snapshots/phone/phone-critical-entry.png",
    ROOT / "apps/web/e2e/snapshots/tablet/tablet-critical-entry.png",
    ROOT / "apps/web/e2e/snapshots/desktop/desktop-critical-entry.png",
)

EXPECTED_VIEWPORTS = {
    "phone": {"width": 390, "height": 844},
    "tablet": {"width": 768, "height": 1024},
    "desktop": {"width": 1440, "height": 900},
}
EXPECTED_INVARIANTS = {
    "SAME_BUSINESS_OUTCOME",
    "NO_HORIZONTAL_OVERFLOW",
    "PRIMARY_ACTION_VISIBLE",
    "KEYBOARD_COMPLETABLE",
    "WCAG_A_AA",
}


def parse_viewports(text: str) -> dict[str, dict[str, int]]:
    out: dict[str, dict[str, int]] = {}
    pattern = re.compile(
        r'name:\s*"([^"]+)".*?viewport:\s*\{\s*width:\s*(\d+),\s*height:\s*(\d+)\s*\}',
        re.DOTALL,
    )
    for name, width, height in pattern.findall(text):
        out[name] = {"width": int(width), "height": int(height)}
    return out


def validate_ui_adaptive_contract(
    playwright_text: str,
    e2e_text: str,
    schema_doc: dict,
    visual_e2e_text: str = "",
    baseline_presence: tuple[bool, ...] = (),
) -> list[str]:
    errors: list[str] = []
    if schema_doc.get("additionalProperties") is not False:
        errors.append("adaptive-flow contract must reject undeclared fields")

    vp_schema = schema_doc.get("properties", {}).get("viewports", {})
    if set(vp_schema.get("required", [])) != set(EXPECTED_VIEWPORTS):
        errors.append("adaptive-flow viewport set drifted")

    actual = parse_viewports(playwright_text)
    if actual != EXPECTED_VIEWPORTS:
        errors.append(f"Playwright viewport matrix differs: {actual}")

    enums = set(
        schema_doc.get("properties", {})
        .get("required_invariants", {})
        .get("items", {})
        .get("enum", [])
    )
    if enums != EXPECTED_INVARIANTS:
        errors.append("adaptive-flow invariant set drifted")

    e2e_snippets = (
        'expect(layout.documentWidth).toBeLessThanOrEqual(layout.viewportWidth + 1)',
        'await expect(page.getByRole("button", { name: "Разобрать запрос" })).toBeFocused()',
        '.withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])',
        'expect(accessibility.violations).toEqual([])',
        'page.getByRole("heading", { name: "Слот удерживается" })',
    )
    for snippet in e2e_snippets:
        if snippet not in e2e_text:
            errors.append(f"adaptive critical-flow E2E invariant missing: {snippet}")

    visual_snippets = (
        'snapshotPathTemplate: "{testDir}/snapshots/{projectName}/{arg}{ext}"',
        'toHaveScreenshot',
        'fullPage: true',
        'maxDiffPixelRatio: 0.001',
    )
    combined_visual = playwright_text + "\n" + visual_e2e_text
    for snippet in visual_snippets:
        if snippet not in combined_visual:
            errors.append(f"visual regression invariant missing: {snippet}")

    if baseline_presence and len(baseline_presence) != len(VISUAL_BASELINES):
        errors.append("visual baseline presence vector has invalid length")
    elif baseline_presence and not all(baseline_presence):
        missing = [
            str(path.relative_to(ROOT))
            for path, present in zip(VISUAL_BASELINES, baseline_presence, strict=True)
            if not present
        ]
        errors.append(f"visual regression baselines missing: {missing}")
    return errors


def main() -> int:
    errors = validate_ui_adaptive_contract(
        PLAYWRIGHT.read_text(encoding="utf-8"),
        E2E.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
        VISUAL_E2E.read_text(encoding="utf-8"),
        tuple(path.is_file() and path.stat().st_size > 0 for path in VISUAL_BASELINES),
    )
    if errors:
        print("UI ADAPTIVE CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("UI ADAPTIVE CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
