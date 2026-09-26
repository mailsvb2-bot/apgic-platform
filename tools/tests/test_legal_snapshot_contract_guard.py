import copy
import json
import unittest

from tools.legal_snapshot_contract_guard import (
    GO_SNAPSHOT,
    MIGRATION,
    SCHEMA,
    validate_legal_snapshot_contract,
)


class LegalSnapshotContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.go = GO_SNAPSHOT.read_text(encoding="utf-8")
        self.migration = MIGRATION.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_go_and_sql(self):
        self.assertEqual(
            validate_legal_snapshot_contract(self.go, self.migration, self.schema),
            [],
        )

    def test_missing_explicit_refund_role_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["required"].remove("refund_responsibility_id")
        errors = validate_legal_snapshot_contract(self.go, self.migration, broken)
        self.assertTrue(any("required fields differ" in error for error in errors))

    def test_missing_append_only_trigger_is_rejected(self):
        broken_sql = self.migration.replace(
            "CREATE TRIGGER legal_transaction_snapshots_append_only",
            "CREATE TRIGGER legal_transaction_snapshots_mutable",
        )
        errors = validate_legal_snapshot_contract(self.go, broken_sql, self.schema)
        self.assertTrue(any("SQL invariant missing" in error for error in errors))

    def test_transaction_ref_is_required(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"].pop("transaction_ref")
        broken["required"].remove("transaction_ref")
        errors = validate_legal_snapshot_contract(self.go, self.migration, broken)
        self.assertTrue(any("persisted legal snapshot fields differ" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
