import copy
import json
import unittest

import yaml

from tools.retention_contract_guard import (
    LAUNCH,
    MATRIX,
    SCHEMA,
    validate_retention_contract,
)


class RetentionContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.matrix = yaml.safe_load(MATRIX.read_text(encoding="utf-8")) or {}
        self.launch = yaml.safe_load(LAUNCH.read_text(encoding="utf-8")) or {}
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_launch_matrix(self):
        self.assertEqual(validate_retention_contract(self.matrix, self.launch, self.schema), [])

    def test_credential_dataclass_is_required(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["data_classes"]["required"].remove("CREDENTIAL")
        errors = validate_retention_contract(self.matrix, self.launch, broken)
        self.assertTrue(any("DataClass contract drifted" in error for error in errors))

    def test_raw_consultation_retention_must_be_zero_in_r0_ci(self):
        broken = copy.deepcopy(self.matrix)
        broken["data_classes"]["RAW_CONSULTATION"]["retention_seconds"] = 1
        errors = validate_retention_contract(broken, self.launch, self.schema)
        self.assertTrue(any("RAW_CONSULTATION" in error for error in errors))

    def test_launch_version_must_match_matrix(self):
        broken = copy.deepcopy(self.launch)
        broken["retention_policy_version"] = "other"
        errors = validate_retention_contract(self.matrix, broken, self.schema)
        self.assertTrue(any("version differs" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
