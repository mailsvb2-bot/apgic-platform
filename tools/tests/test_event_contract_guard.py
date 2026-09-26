import copy
import json
import unittest

from tools.event_contract_guard import GO_OUTBOX, SCHEMA, validate_event_contract


class EventContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.go = GO_OUTBOX.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_go(self):
        self.assertEqual(validate_event_contract(self.go, self.schema), [])

    def test_missing_idempotency_key_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["required"].remove("idempotency_key")
        broken["properties"].pop("idempotency_key")
        errors = validate_event_contract(self.go, broken)
        self.assertTrue(any("idempotency_key" in error for error in errors))

    def test_open_ended_schema_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["additionalProperties"] = True
        errors = validate_event_contract(self.go, broken)
        self.assertTrue(any("undeclared fields" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
