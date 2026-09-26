import copy
import json
import unittest

import yaml

from tools.jurisdiction_contract_guard import (
    LAUNCH,
    MATRIX,
    SCHEMA,
    validate_jurisdiction_contract,
)


class JurisdictionContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.matrix = yaml.safe_load(MATRIX.read_text(encoding="utf-8")) or {}
        self.launch = yaml.safe_load(LAUNCH.read_text(encoding="utf-8")) or {}
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_launch_matrix(self):
        self.assertEqual(
            validate_jurisdiction_contract(self.matrix, self.launch, self.schema),
            [],
        )

    def test_unknown_combination_must_block(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["unknown_combination"]["const"] = "ALLOW"
        errors = validate_jurisdiction_contract(self.matrix, self.launch, broken)
        self.assertTrue(any("unknown_combination" in error for error in errors))

    def test_all_legal_financial_roles_are_required(self):
        broken = copy.deepcopy(self.schema)
        roles = broken["properties"]["jurisdictions"]["items"]["properties"]["legal_financial_roles"]["required"]
        roles.remove("refund_responsibility")
        errors = validate_jurisdiction_contract(self.matrix, self.launch, broken)
        self.assertTrue(any("role contract drifted" in error for error in errors))

    def test_launch_version_must_match_matrix(self):
        broken_launch = copy.deepcopy(self.launch)
        broken_launch["jurisdiction_matrix_version"] = "other"
        errors = validate_jurisdiction_contract(self.matrix, broken_launch, self.schema)
        self.assertTrue(any("version differs" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
