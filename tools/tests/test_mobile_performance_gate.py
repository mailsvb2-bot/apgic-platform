import unittest

from tools.mobile_performance_gate import validate_measurements


def policy() -> dict:
    return {
        "version": "mobile-observability-ci-v1",
        "metrics": {
            "startup_p95_ms": {
                "unit": "milliseconds",
                "guardrail_operator": "LTE",
                "guardrail_value": 5000,
                "breach_action": "HALT_OR_LIMIT_ROLLOUT",
            },
            "checkout_success": {
                "unit": "ratio",
                "guardrail_operator": "GTE",
                "guardrail_value": 0.80,
                "breach_action": "HALT_OR_LIMIT_ROLLOUT",
            },
        },
    }


def measurements(startup: float = 1800, checkout: float = 0.95) -> dict:
    return {
        "policy_version": "mobile-observability-ci-v1",
        "metrics": {
            "startup_p95_ms": {"unit": "milliseconds", "value": startup},
            "checkout_success": {"unit": "ratio", "value": checkout},
        },
    }


class MobilePerformanceGateTests(unittest.TestCase):
    def test_guardrails_accept_measurements_inside_thresholds(self) -> None:
        validate_measurements(policy(), measurements())

    def test_startup_regression_blocks_rollout(self) -> None:
        with self.assertRaises(SystemExit):
            validate_measurements(policy(), measurements(startup=6200))

    def test_checkout_regression_blocks_rollout(self) -> None:
        with self.assertRaises(SystemExit):
            validate_measurements(policy(), measurements(checkout=0.61))

    def test_wrong_unit_fails_closed(self) -> None:
        sample = measurements()
        sample["metrics"]["startup_p95_ms"]["unit"] = "ratio"
        with self.assertRaises(SystemExit):
            validate_measurements(policy(), sample)

    def test_policy_version_mismatch_fails_closed(self) -> None:
        sample = measurements()
        sample["policy_version"] = "stale-policy"
        with self.assertRaises(SystemExit):
            validate_measurements(policy(), sample)

    def test_missing_metric_fails_closed(self) -> None:
        sample = measurements()
        del sample["metrics"]["checkout_success"]
        with self.assertRaises(SystemExit):
            validate_measurements(policy(), sample)


if __name__ == "__main__":
    unittest.main()
