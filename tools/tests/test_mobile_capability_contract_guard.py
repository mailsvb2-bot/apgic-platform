import copy
import json
import unittest

from tools.mobile_capability_contract_guard import (
    SCHEMA,
    TS,
    validate_mobile_capability_contract,
)


class MobileCapabilityContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.ts = TS.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_typescript(self):
        self.assertEqual(validate_mobile_capability_contract(self.ts, self.schema), [])

    def test_missing_microphone_capability_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["$defs"]["capability"]["enum"].remove("MICROPHONE")
        errors = validate_mobile_capability_contract(self.ts, broken)
        self.assertTrue(any("device capabilities differ" in error for error in errors))

    def test_state_drift_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["$defs"]["state"]["enum"].remove("RESTRICTED")
        errors = validate_mobile_capability_contract(self.ts, broken)
        self.assertTrue(any("capability states differ" in error for error in errors))

    def test_open_schema_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["additionalProperties"] = True
        errors = validate_mobile_capability_contract(self.ts, broken)
        self.assertTrue(any("undeclared fields" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
