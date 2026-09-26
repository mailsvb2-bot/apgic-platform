import copy
import json
import unittest

from tools.audit_contract_guard import (
    GO_AUDIT,
    MIGRATION,
    SCHEMA,
    validate_audit_contract,
)


class AuditContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.go = GO_AUDIT.read_text(encoding="utf-8")
        self.migration = MIGRATION.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_go_and_sql(self):
        self.assertEqual(validate_audit_contract(self.go, self.migration, self.schema), [])

    def test_missing_policy_version_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["required"].remove("policy_version")
        errors = validate_audit_contract(self.go, self.migration, broken)
        self.assertTrue(any("required evidence fields differ" in error for error in errors))

    def test_missing_append_only_trigger_is_rejected(self):
        broken_sql = self.migration.replace("CREATE TRIGGER audit_records_append_only", "CREATE TRIGGER audit_records_mutable")
        errors = validate_audit_contract(self.go, broken_sql, self.schema)
        self.assertTrue(any("append-only SQL invariant missing" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
