import copy
import json
import unittest

import yaml

from tools.mobile_dependency_contract_guard import (
    PACKAGE,
    REGISTRY,
    SCHEMA,
    validate_mobile_dependency_contract,
)


class MobileDependencyContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.package = json.loads(PACKAGE.read_text(encoding="utf-8"))
        self.registry = yaml.safe_load(REGISTRY.read_text(encoding="utf-8")) or {}
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_package_and_registry(self):
        self.assertEqual(
            validate_mobile_dependency_contract(self.package, self.registry, self.schema),
            [],
        )

    def test_unregistered_dependency_is_rejected(self):
        broken = copy.deepcopy(self.package)
        broken.setdefault("dependencies", {})["unregistered-test-package"] = "1.0.0"
        errors = validate_mobile_dependency_contract(broken, self.registry, self.schema)
        self.assertTrue(any("registry/package graph differ" in error for error in errors))

    def test_missing_kill_strategy_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["dependencies"]["items"]["required"].remove("kill_strategy")
        errors = validate_mobile_dependency_contract(self.package, self.registry, broken)
        self.assertTrue(any("required fields differ" in error for error in errors))

    def test_unknown_dependency_policy_must_block(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["policy"]["properties"]["unknown_dependency"]["const"] = "ALLOW"
        errors = validate_mobile_dependency_contract(self.package, self.registry, broken)
        self.assertTrue(any("unknown dependency policy" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
