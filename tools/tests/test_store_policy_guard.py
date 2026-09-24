import unittest

from tools import launch_config_guard as guard


def valid_policy() -> dict:
    return {
        "policy_kind": "STORE_COMMERCE",
        "unknown_path": "PURCHASE_DISABLED",
        "rules": [
            {
                "product_type": "SUBSCRIPTION",
                "surface": "IOS",
                "store": "STORE_A",
                "storefront": "RU",
                "jurisdiction": "RU",
                "enabled": True,
                "rail": "STORE_BILLING",
                "reason_code": "STORE_BILLING_REQUIRED",
            }
        ],
    }


class StorePolicyGuardTests(unittest.TestCase):
    def test_explicit_store_policy_passes(self) -> None:
        guard.validate_store_commerce(valid_policy())

    def test_unknown_store_path_must_fail_closed(self) -> None:
        policy = valid_policy()
        policy["unknown_path"] = "USE_DEFAULT"
        with self.assertRaises(SystemExit):
            guard.validate_store_commerce(policy)

    def test_duplicate_store_context_is_rejected(self) -> None:
        policy = valid_policy()
        policy["rules"].append(dict(policy["rules"][0]))
        with self.assertRaises(SystemExit):
            guard.validate_store_commerce(policy)

    def test_disabled_rule_cannot_expose_purchase_rail(self) -> None:
        policy = valid_policy()
        policy["rules"][0]["enabled"] = False
        with self.assertRaises(SystemExit):
            guard.validate_store_commerce(policy)


if __name__ == "__main__":
    unittest.main()
