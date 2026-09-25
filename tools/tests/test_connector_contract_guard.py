import copy
import json
import unittest

from tools.connector_contract_guard import (
    CAPABILITY_GO,
    REGISTRY_GO,
    SCHEMA,
    WEBHOOK_GO,
    validate_connector_contract,
)


class ConnectorContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.registry = REGISTRY_GO.read_text(encoding="utf-8")
        self.capability = CAPABILITY_GO.read_text(encoding="utf-8")
        self.webhook = WEBHOOK_GO.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_go(self):
        self.assertEqual(
            validate_connector_contract(self.registry, self.capability, self.webhook, self.schema),
            [],
        )

    def test_missing_capability_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["$defs"]["capability_class"]["enum"].remove("PAYMENT_PROVIDER")
        errors = validate_connector_contract(self.registry, self.capability, self.webhook, broken)
        self.assertTrue(any("capability_class" in error for error in errors))

    def test_missing_ambiguous_outcome_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["$defs"]["outcome"]["enum"].remove("AMBIGUOUS")
        errors = validate_connector_contract(self.registry, self.capability, self.webhook, broken)
        self.assertTrue(any("outcome" in error for error in errors))

    def test_delivery_decision_drift_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["$defs"]["delivery_decision"]["enum"].append("RETRY")
        errors = validate_connector_contract(self.registry, self.capability, self.webhook, broken)
        self.assertTrue(any("delivery_decision" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
