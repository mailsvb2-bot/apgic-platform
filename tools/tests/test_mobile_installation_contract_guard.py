import copy
import json
import unittest

from tools.mobile_installation_contract_guard import (
    GO_INSTALLATION,
    MIGRATION,
    SCHEMA,
    validate_mobile_installation_contract,
)


class MobileInstallationContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.go = GO_INSTALLATION.read_text(encoding="utf-8")
        self.migration = MIGRATION.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_go_and_sql(self):
        self.assertEqual(validate_mobile_installation_contract(self.go, self.migration, self.schema), [])

    def test_revoked_state_is_required(self):
        broken = copy.deepcopy(self.schema)
        broken["$defs"]["state"]["enum"].remove("REVOKED")
        errors = validate_mobile_installation_contract(self.go, self.migration, broken)
        self.assertTrue(any("installation states differ" in error for error in errors))

    def test_push_generation_must_be_positive(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["push_generation"]["minimum"] = 0
        errors = validate_mobile_installation_contract(self.go, self.migration, broken)
        self.assertTrue(any("push_generation" in error for error in errors))

    def test_active_push_endpoint_uniqueness_is_required(self):
        broken_sql = self.migration.replace(
            "CREATE UNIQUE INDEX client_installations_active_push_endpoint_idx",
            "CREATE INDEX client_installations_active_push_endpoint_idx",
        )
        errors = validate_mobile_installation_contract(self.go, broken_sql, self.schema)
        self.assertTrue(any("SQL invariant missing" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
