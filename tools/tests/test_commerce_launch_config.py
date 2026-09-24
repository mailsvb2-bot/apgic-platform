import unittest

from tools import launch_config_guard as guard


def valid_policy() -> dict:
    return {
        "policy_kind": "PRICING_COMMISSION",
        "unknown_path": "BLOCK",
        "monetized_paths": {
            "consultation_booking": {
                "pricing_policy_version": "pricing-v1",
                "commission_policy_version": "commission-v1",
                "price_source_ref": "catalog/consultation-v1",
                "currency": "RUB",
                "commission": {
                    "kind": "PERCENT_BPS",
                    "basis_points": 1000,
                },
            }
        },
    }


class CommerceLaunchConfigTests(unittest.TestCase):
    def test_explicit_pricing_and_commission_pass(self) -> None:
        guard.validate_commerce(valid_policy())

    def test_unknown_path_must_fail_closed(self) -> None:
        policy = valid_policy()
        policy["unknown_path"] = "USE_DEFAULT"
        with self.assertRaises(SystemExit):
            guard.validate_commerce(policy)

    def test_missing_policy_version_is_rejected(self) -> None:
        policy = valid_policy()
        policy["monetized_paths"]["consultation_booking"]["pricing_policy_version"] = ""
        with self.assertRaises(SystemExit):
            guard.validate_commerce(policy)

    def test_config_required_is_not_a_policy_version(self) -> None:
        policy = valid_policy()
        policy["monetized_paths"]["consultation_booking"]["commission_policy_version"] = "CONFIG_REQUIRED"
        with self.assertRaises(SystemExit):
            guard.validate_commerce(policy)

    def test_invalid_commission_range_is_rejected(self) -> None:
        policy = valid_policy()
        policy["monetized_paths"]["consultation_booking"]["commission"]["basis_points"] = 10001
        with self.assertRaises(SystemExit):
            guard.validate_commerce(policy)


if __name__ == "__main__":
    unittest.main()
