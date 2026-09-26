import copy
import json
import unittest

from tools.ui_adaptive_contract_guard import (
    E2E,
    PLAYWRIGHT,
    SCHEMA,
    validate_ui_adaptive_contract,
)


class UIAdaptiveContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.playwright = PLAYWRIGHT.read_text(encoding="utf-8")
        self.e2e = E2E.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_ui_matches_adaptive_contract(self):
        self.assertEqual(
            validate_ui_adaptive_contract(self.playwright, self.e2e, self.schema),
            [],
        )

    def test_missing_tablet_viewport_is_rejected(self):
        broken = self.playwright.replace(
            '{\n      name: "tablet",\n      use: { viewport: { width: 768, height: 1024 } },\n    },\n',
            "",
        )
        errors = validate_ui_adaptive_contract(broken, self.e2e, self.schema)
        self.assertTrue(any("viewport matrix differs" in error for error in errors))

    def test_overflow_assertion_is_required(self):
        broken = self.e2e.replace(
            "expect(layout.documentWidth).toBeLessThanOrEqual(layout.viewportWidth + 1);",
            "",
        )
        errors = validate_ui_adaptive_contract(self.playwright, broken, self.schema)
        self.assertTrue(any("E2E invariant missing" in error for error in errors))

    def test_wcag_invariant_is_required(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["required_invariants"]["items"]["enum"].remove("WCAG_A_AA")
        errors = validate_ui_adaptive_contract(self.playwright, self.e2e, broken)
        self.assertTrue(any("invariant set drifted" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
