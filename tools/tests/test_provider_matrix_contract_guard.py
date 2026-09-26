import copy
import json
import unittest

import yaml

from tools.provider_matrix_contract_guard import (
    LAUNCH,
    MATRIX,
    SCHEMA,
    validate_provider_matrix_contract,
)


class ProviderMatrixContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.matrix = yaml.safe_load(MATRIX.read_text(encoding="utf-8")) or {}
        self.launch = yaml.safe_load(LAUNCH.read_text(encoding="utf-8")) or {}
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_launch_matrix(self):
        self.assertEqual(validate_provider_matrix_contract(self.matrix, self.launch, self.schema), [])

    def test_provider_neutral_contract_is_required(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["capabilities"]["additionalProperties"]["properties"]["provider_neutral_contract"]["const"] = False
        errors = validate_provider_matrix_contract(self.matrix, self.launch, broken)
        self.assertTrue(any("provider-neutral" in error for error in errors))

    def test_unknown_fallback_provider_is_rejected(self):
        broken = copy.deepcopy(self.matrix)
        broken["capabilities"]["PAYMENT_PROVIDER"]["fallback_providers"].append("MISSING")
        errors = validate_provider_matrix_contract(broken, self.launch, self.schema)
        self.assertTrue(any("unknown fallback providers" in error for error in errors))

    def test_launch_version_must_match_matrix(self):
        broken = copy.deepcopy(self.launch)
        broken["provider_matrix_version"] = "other"
        errors = validate_provider_matrix_contract(self.matrix, broken, self.schema)
        self.assertTrue(any("version differs" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
