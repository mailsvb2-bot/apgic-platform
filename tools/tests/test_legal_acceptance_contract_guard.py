import copy
import json
import unittest

from tools.legal_acceptance_contract_guard import (
    GO_ACCEPTANCE,
    MIGRATION,
    SCHEMA,
    validate_legal_acceptance_contract,
)


class LegalAcceptanceContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.go = GO_ACCEPTANCE.read_text(encoding="utf-8")
        self.migration = MIGRATION.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_go_and_sql(self):
        self.assertEqual(
            validate_legal_acceptance_contract(self.go, self.migration, self.schema),
            [],
        )

    def test_document_version_is_required(self):
        broken = copy.deepcopy(self.schema)
        broken["required"].remove("document_version")
        errors = validate_legal_acceptance_contract(self.go, self.migration, broken)
        self.assertTrue(any("required fields differ" in error for error in errors))

    def test_evidence_hash_is_required(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["evidence_hash"]["minLength"] = 0
        errors = validate_legal_acceptance_contract(self.go, self.migration, broken)
        self.assertTrue(any("evidence_hash" in error for error in errors))

    def test_append_only_trigger_is_required(self):
        broken_sql = self.migration.replace(
            "CREATE TRIGGER legal_acceptances_append_only",
            "CREATE TRIGGER legal_acceptances_mutable",
        )
        errors = validate_legal_acceptance_contract(self.go, broken_sql, self.schema)
        self.assertTrue(any("SQL invariant missing" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
