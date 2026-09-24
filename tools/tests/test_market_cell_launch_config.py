from __future__ import annotations

import unittest

from tools import launch_config_guard as guard


def valid_policy() -> dict:
    return {
        "policy_kind": "MARKET_CELL_THRESHOLDS",
        "market_cells": [
            {
                "id": "cell-1",
                "jurisdiction": "RU",
                "topic": "anxiety",
                "format": "ONLINE",
                "time_window": "ALL",
                "state": "DEMAND_TEST",
                "thresholds": {
                    "eligible_verified_supply_min": 3,
                    "active_specialists_min": 3,
                    "bookable_slot_coverage_min_percent": 50,
                    "duty_supply_min": 0,
                    "time_to_available_slot_median_max_minutes": 1440,
                    "time_to_available_slot_p95_max_minutes": 4320,
                    "fill_conversion_min_percent": 10,
                    "booking_conversion_min_percent": 5,
                    "cancellation_max_percent": 30,
                    "no_show_max_percent": 20,
                    "response_time_p95_max_minutes": 120,
                    "acceptance_time_p95_max_minutes": 240,
                    "unfilled_demand_max_percent": 80,
                    "complaint_rate_max_percent": 20,
                    "safety_incident_rate_max_percent": 5,
                    "contribution_margin_min_percent": -20,
                },
                "evidence_refs": [],
            }
        ],
    }


class MarketCellLaunchConfigTests(unittest.TestCase):
    def test_explicit_threshold_set_passes_before_scale(self) -> None:
        guard.validate_market_cell(valid_policy())

    def test_missing_threshold_fails_closed(self) -> None:
        policy = valid_policy()
        del policy["market_cells"][0]["thresholds"]["eligible_verified_supply_min"]
        with self.assertRaises(SystemExit):
            guard.validate_market_cell(policy)

    def test_scale_ready_requires_recorded_measured_evidence(self) -> None:
        policy = valid_policy()
        cell = policy["market_cells"][0]
        cell["state"] = "SCALE_READY"
        with self.assertRaises(SystemExit):
            guard.validate_market_cell(policy)

        cell["evidence_refs"] = ["evidence://measured-market-cell"]
        with self.assertRaises(SystemExit):
            guard.validate_market_cell(policy)

        cell["evidence_id"] = "market-cell-evidence-1"
        cell["observed_at"] = "2026-09-24T12:00:00Z"
        cell["observed_metrics"] = {
            "eligible_verified_supply": 4,
            "active_specialists": 4,
            "bookable_slot_coverage_percent": 70,
            "duty_supply": 1,
            "time_to_available_slot_median_minutes": 120,
            "time_to_available_slot_p95_minutes": 480,
            "fill_conversion_percent": 35,
            "booking_conversion_percent": 20,
            "cancellation_percent": 10,
            "no_show_percent": 5,
            "response_time_p95_minutes": 30,
            "acceptance_time_p95_minutes": 45,
            "unfilled_demand_percent": 15,
            "complaint_rate_percent": 2,
            "safety_incident_rate_percent": 0,
            "contribution_margin_percent": 5,
        }
        guard.validate_market_cell(policy)

    def test_scale_ready_fails_when_observed_metric_misses_threshold(self) -> None:
        policy = valid_policy()
        cell = policy["market_cells"][0]
        cell["state"] = "SCALE_READY"
        cell["evidence_refs"] = ["evidence://measured-market-cell"]
        cell["evidence_id"] = "market-cell-evidence-1"
        cell["observed_at"] = "2026-09-24T12:00:00Z"
        cell["observed_metrics"] = {
            "eligible_verified_supply": 2,
            "active_specialists": 4,
            "bookable_slot_coverage_percent": 70,
            "duty_supply": 1,
            "time_to_available_slot_median_minutes": 120,
            "time_to_available_slot_p95_minutes": 480,
            "fill_conversion_percent": 35,
            "booking_conversion_percent": 20,
            "cancellation_percent": 10,
            "no_show_percent": 5,
            "response_time_p95_minutes": 30,
            "acceptance_time_p95_minutes": 45,
            "unfilled_demand_percent": 15,
            "complaint_rate_percent": 2,
            "safety_incident_rate_percent": 0,
            "contribution_margin_percent": 5,
        }
        with self.assertRaises(SystemExit):
            guard.validate_market_cell(policy)

    def test_unknown_threshold_is_rejected(self) -> None:
        policy = valid_policy()
        policy["market_cells"][0]["thresholds"]["mystery_default"] = 1
        with self.assertRaises(SystemExit):
            guard.validate_market_cell(policy)


if __name__ == "__main__":
    unittest.main()
