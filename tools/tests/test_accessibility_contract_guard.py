import copy
import json
import unittest

from tools.accessibility_contract_guard import (
    CSS,
    E2E,
    JOURNEY,
    SCHEMA,
    validate_accessibility_contract,
)


class AccessibilityContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
        self.e2e = E2E.read_text(encoding="utf-8")
        self.css = CSS.read_text(encoding="utf-8")
        self.journey = JOURNEY.read_text(encoding="utf-8")

    def test_repository_accessibility_matches_contract(self):
        self.assertEqual(
            validate_accessibility_contract(self.schema, self.e2e, self.css, self.journey),
            [],
        )

    def test_text_scaling_e2e_is_required(self):
        broken = self.e2e.replace('document.documentElement.style.fontSize = "200%";', "")
        errors = validate_accessibility_contract(self.schema, broken, self.css, self.journey)
        self.assertTrue(any("text scaling" in error.lower() or "E2E invariant missing" in error for error in errors))

    def test_focus_visible_styling_is_required(self):
        broken_css = self.css.replace("focus-visible", "focus-hidden")
        errors = validate_accessibility_contract(self.schema, self.e2e, broken_css, self.journey)
        self.assertTrue(any("focus-visible" in error for error in errors))

    def test_accessible_label_is_required(self):
        broken = self.journey.replace('label htmlFor="request"', 'label')
        errors = validate_accessibility_contract(self.schema, self.e2e, self.css, broken)
        self.assertTrue(any("label association" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
