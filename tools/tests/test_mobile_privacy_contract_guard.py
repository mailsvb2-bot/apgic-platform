import copy
import json
import unittest

import yaml

from tools.mobile_privacy_contract_guard import (
    DECLARATION,
    DEPENDENCIES,
    SCHEMA,
    validate_mobile_privacy_contract,
)


class MobilePrivacyContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.dependencies = yaml.safe_load(DEPENDENCIES.read_text(encoding="utf-8")) or {}
        self.declaration = yaml.safe_load(DECLARATION.read_text(encoding="utf-8")) or {}
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_privacy_declaration(self):
        self.assertEqual(
            validate_mobile_privacy_contract(self.dependencies, self.declaration, self.schema),
            [],
        )

    def test_missing_runtime_assertion_is_rejected(self):
        broken = copy.deepcopy(self.declaration)
        broken["runtime_dependency_assertions"].pop("react")
        errors = validate_mobile_privacy_contract(self.dependencies, broken, self.schema)
        self.assertTrue(any("runtime privacy assertions differ" in error for error in errors))

    def test_raw_financial_evidence_collection_is_rejected(self):
        broken = copy.deepcopy(self.declaration)
        broken["collected_data_classes"] = ["FINANCIAL_EVIDENCE"]
        errors = validate_mobile_privacy_contract(self.dependencies, broken, self.schema)
        self.assertTrue(any("forbidden raw data" in error for error in errors))

    def test_repository_artifact_cannot_claim_production_submission(self):
        broken = copy.deepcopy(self.declaration)
        broken["store_artifacts"]["apple_privacy_manifest"]["production_submission"] = True
        errors = validate_mobile_privacy_contract(self.dependencies, broken, self.schema)
        self.assertTrue(any("production submission" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
