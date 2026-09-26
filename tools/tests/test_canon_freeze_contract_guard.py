import copy
import json
import unittest

import yaml

from tools.canon_freeze_contract_guard import (
    CANON_LINT,
    RFC,
    SCHEMA,
    TEMPLATE,
    validate_canon_freeze_contract,
)


class CanonFreezeContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.rfc = yaml.safe_load(RFC.read_text(encoding="utf-8")) or {}
        self.template = TEMPLATE.read_text(encoding="utf-8")
        self.lint = CANON_LINT.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_governance_matches_contract(self):
        self.assertEqual(
            validate_canon_freeze_contract(self.rfc, self.template, self.lint, self.schema),
            [],
        )

    def test_missing_blocker_reason_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["rfcs"]["items"]["properties"]["reason_class"]["enum"].remove("SECURITY_BLOCKER")
        errors = validate_canon_freeze_contract(self.rfc, self.template, self.lint, broken)
        self.assertTrue(any("reason classes differ" in error for error in errors))

    def test_immutable_field_set_drift_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["rfcs"]["items"]["properties"]["field"]["enum"].remove("dependencies")
        errors = validate_canon_freeze_contract(self.rfc, self.template, self.lint, broken)
        self.assertTrue(any("immutable-field contract drifted" in error for error in errors))

    def test_rfc_decision_is_required(self):
        broken_rfc = copy.deepcopy(self.rfc)
        broken_rfc["rfcs"][0]["decision"] = ""
        errors = validate_canon_freeze_contract(broken_rfc, self.template, self.lint, self.schema)
        self.assertTrue(any("decision is required" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
