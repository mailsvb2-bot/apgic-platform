import copy
import json
import unittest

from tools.organization_contract_guard import (
    GO_ORGANIZATION,
    SCHEMA,
    validate_organization_contract,
)


class OrganizationContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.go = GO_ORGANIZATION.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_go(self):
        self.assertEqual(validate_organization_contract(self.go, self.schema), [])

    def test_status_drift_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["$defs"]["direction_status"]["enum"].remove("ARCHIVED")
        errors = validate_organization_contract(self.go, broken)
        self.assertTrue(any("direction statuses differ" in error for error in errors))

    def test_direction_shape_drift_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["$defs"]["direction"]["required"].remove("has_dependent_truth")
        errors = validate_organization_contract(self.go, broken)
        self.assertTrue(any("required fields differ" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
