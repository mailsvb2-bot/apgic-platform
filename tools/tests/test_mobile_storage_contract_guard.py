import copy
import json
import unittest

from tools.mobile_storage_contract_guard import (
    SCHEMA,
    TS,
    validate_mobile_storage_contract,
)


class MobileStorageContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.ts = TS.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_typescript_policy(self):
        self.assertEqual(validate_mobile_storage_contract(self.ts, self.schema), [])

    def test_credential_dataclass_is_required(self):
        broken = copy.deepcopy(self.schema)
        broken["$defs"]["data_class"]["enum"].remove("CREDENTIAL")
        errors = validate_mobile_storage_contract(self.ts, broken)
        self.assertTrue(any("data classes differ" in error for error in errors))

    def test_secure_storage_target_is_required(self):
        broken = copy.deepcopy(self.schema)
        broken["$defs"]["storage_target"]["enum"].remove("SECURE_STORAGE")
        errors = validate_mobile_storage_contract(self.ts, broken)
        self.assertTrue(any("storage targets differ" in error for error in errors))

    def test_raw_sensitive_policy_branch_is_required(self):
        broken_ts = self.ts.replace(
            'return target === "MEMORY" || target === "EPHEMERAL_CACHE";',
            'return true;',
        )
        errors = validate_mobile_storage_contract(broken_ts, self.schema)
        self.assertTrue(any("raw sensitive storage invariant" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
