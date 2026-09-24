from __future__ import annotations

import unittest

from tools import launch_config_guard as guard


def valid_policy() -> dict:
    return {
        "policy_kind": "PROVIDER_SETTLEMENT",
        "unknown_path": "BLOCK_POLICY_NOT_CONFIGURED",
        "commission_rules": [{
            "id": "organic-consultation-ru",
            "commission_policy_version": "commission-v1",
            "demand_source": "MARKETPLACE_ORGANIC",
            "product_ref": "consultation_booking",
            "jurisdiction": "RU",
            "currency": "RUB",
            "basis_points": 1000,
            "rounding_mode": "FLOOR_MINOR",
            "platform_fee_recipient_ref": "platform/apgic-fee",
        }],
        "provider_execution": {
            "required_settlement_capabilities": ["MARKETPLACE_SPLIT", "SETTLEMENT_EXECUTION"],
            "required_payout_capabilities": ["PAYOUT_EXECUTION"],
            "execution_owner": "EXTERNAL_PROVIDER",
            "historical_provider_affinity": "REQUIRED",
            "cross_provider_money_bridge": "FORBIDDEN",
        },
    }


class R4FinancePolicyTests(unittest.TestCase):
    def test_explicit_provider_settlement_policy_passes(self) -> None:
        guard.validate_provider_settlement(valid_policy())

    def test_missing_source_rule_fails_closed(self) -> None:
        policy = valid_policy()
        policy["commission_rules"] = []
        with self.assertRaises(SystemExit):
            guard.validate_provider_settlement(policy)

    def test_duplicate_source_product_jurisdiction_rule_is_rejected(self) -> None:
        policy = valid_policy()
        policy["commission_rules"].append(dict(policy["commission_rules"][0], id="duplicate"))
        with self.assertRaises(SystemExit):
            guard.validate_provider_settlement(policy)

    def test_apgic_execution_owner_is_rejected(self) -> None:
        policy = valid_policy()
        policy["provider_execution"]["execution_owner"] = "APGIC"
        with self.assertRaises(SystemExit):
            guard.validate_provider_settlement(policy)

    def test_cross_provider_bridge_must_be_forbidden(self) -> None:
        policy = valid_policy()
        policy["provider_execution"]["cross_provider_money_bridge"] = "ALLOWED"
        with self.assertRaises(SystemExit):
            guard.validate_provider_settlement(policy)


if __name__ == "__main__":
    unittest.main()
