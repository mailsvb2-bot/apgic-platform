from __future__ import annotations

import unittest

from tools import launch_config_guard as guard
from tools import mobile_realtime_guard as realtime_guard


def valid_booking_policy() -> dict:
    return {
        "policy_kind": "BOOKING_FULFILLMENT",
        "unknown_path": "BLOCK",
        "cancellation": {
            "free_cancel_until_seconds_before_start": 86400,
            "late_cancel_until_seconds_before_start": 3600,
            "late_cancel_refund_basis_points": 5000,
            "after_start_action": "MANUAL_REVIEW",
        },
        "no_show": {
            "participant_grace_seconds": 900,
            "client_no_show_action": "MANUAL_REVIEW",
            "specialist_no_show_action": "RESCHEDULE_OR_REFUND",
        },
        "refund": {
            "technical_failure_action": "REFUND_OR_RESCHEDULE",
            "late_cancel_refund_basis_points": 5000,
            "no_show_refund_basis_points": 0,
        },
        "reschedule": {
            "minimum_seconds_before_start": 3600,
            "max_reschedules_per_booking": 2,
        },
    }


def valid_realtime_policy() -> dict:
    return {
        "policy_version": "mobile-realtime-r3-ci-v1",
        "scope": "CI_ONLY",
        "production_approved": False,
        "reconnect": {
            "max_attempts": 3,
            "backoff_ms": [250, 1000, 3000],
            "allow_in_background": True,
        },
        "permissions": {"microphone_required": True},
        "network": {
            "offline_action": "RECONNECT_WHEN_ONLINE",
            "degraded_action": "KEEP_SESSION_DEGRADED",
        },
        "audio": {"route_change_action": "REFRESH_ROUTE"},
        "interruption": {"action": "PAUSE_MEDIA_AND_RECONNECT"},
        "diagnostics": {
            "export_requires_confirmation": True,
            "max_export_ttl_seconds": 900,
            "prohibited_export_fields": sorted(realtime_guard.REQUIRED_PROHIBITED_FIELDS),
        },
    }


class R3PolicyTests(unittest.TestCase):
    def test_explicit_booking_policy_passes(self) -> None:
        guard.validate_booking_fulfillment(valid_booking_policy())

    def test_booking_policy_missing_numeric_window_fails_closed(self) -> None:
        policy = valid_booking_policy()
        policy["no_show"]["participant_grace_seconds"] = "CONFIG_REQUIRED"
        with self.assertRaises(SystemExit):
            guard.validate_booking_fulfillment(policy)

    def test_booking_policy_rejects_implicit_unknown_default(self) -> None:
        policy = valid_booking_policy()
        policy["unknown_path"] = "USE_PROVIDER_DEFAULT"
        with self.assertRaises(SystemExit):
            guard.validate_booking_fulfillment(policy)

    def test_realtime_and_diagnostics_policy_passes(self) -> None:
        realtime_guard.validate_policy(valid_realtime_policy(), "ci")

    def test_diagnostics_policy_requires_sensitive_field_redaction(self) -> None:
        policy = valid_realtime_policy()
        policy["diagnostics"]["prohibited_export_fields"].remove("raw_consultation")
        with self.assertRaises(SystemExit):
            realtime_guard.validate_policy(policy, "ci")

    def test_reconnect_backoff_must_match_attempt_budget(self) -> None:
        policy = valid_realtime_policy()
        policy["reconnect"]["backoff_ms"] = [250]
        with self.assertRaises(SystemExit):
            realtime_guard.validate_policy(policy, "ci")


if __name__ == "__main__":
    unittest.main()
