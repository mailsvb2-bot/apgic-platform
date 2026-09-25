#!/usr/bin/env python3
from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]

POLICIES = {
    "jurisdiction": ("jurisdiction_matrix_version", "jurisdiction_matrix_path"),
    "retention": ("retention_policy_version", "retention_policy_path"),
    "slo": ("slo_policy_version", "slo_policy_path"),
    "providers": ("provider_matrix_version", "provider_matrix_path"),
}

R1_POLICIES = {
    **POLICIES,
    "market_cell": ("market_cell_thresholds_version", "market_cell_thresholds_path"),
}

R2_POLICIES = {
    **R1_POLICIES,
    "commerce": ("commerce_policy_version", "commerce_policy_path"),
    "store_commerce": ("store_policy_version", "store_policy_path"),
}

R3_POLICIES = {
    **R2_POLICIES,
    "booking_fulfillment": (
        "booking_fulfillment_policy_version",
        "booking_fulfillment_policy_path",
    ),
}

R4_POLICIES = {
    **R3_POLICIES,
    "provider_settlement": (
        "provider_settlement_policy_version",
        "provider_settlement_policy_path",
    ),
}

R5_POLICIES = {
    **R4_POLICIES,
}

def fail(message: str) -> None:
    print(f"LAUNCH CONFIG PRECHECK: FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)

def contains_config_required(value: object) -> bool:
    if isinstance(value, str):
        return value.strip() == "CONFIG_REQUIRED"
    if isinstance(value, dict):
        return any(contains_config_required(item) for item in value.values())
    if isinstance(value, list):
        return any(contains_config_required(item) for item in value)
    return False

def positive_number(value: object, allow_zero: bool = False) -> bool:
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        return False
    return value >= 0 if allow_zero else value > 0

def bounded_number(value: object, minimum: float, maximum: float) -> bool:
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        return False
    return minimum <= value <= maximum

def load_repo_yaml(raw_path: object, label: str) -> tuple[Path, dict]:
    if not isinstance(raw_path, str) or not raw_path.strip():
        fail(f"{label} path is required")
    path = (ROOT / raw_path).resolve()
    if ROOT not in path.parents or not path.is_file():
        fail(f"{label} path is missing or outside repository")
    doc = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    if not isinstance(doc, dict):
        fail(f"{label} policy must be a mapping")
    if contains_config_required(doc):
        fail(f"{label} policy contains CONFIG_REQUIRED")
    return path, doc

def validate_scope(policy: dict, mode: str, label: str) -> None:
    if mode == "production":
        if policy.get("production_approved") is not True:
            fail(f"{label} policy is not production-approved")
        if policy.get("scope") != "PRODUCTION":
            fail(f"{label} policy must have scope=PRODUCTION")
    else:
        if policy.get("scope") != "CI_ONLY":
            fail(f"{label} policy must have scope=CI_ONLY in CI mode")

def validate_jurisdiction(policy: dict) -> None:
    if policy.get("unknown_combination") != "BLOCK":
        fail("jurisdiction unknown_combination must be BLOCK")
    rows = policy.get("jurisdictions")
    if not isinstance(rows, list) or not rows:
        fail("jurisdiction matrix must contain at least one row")
    required_roles = {
        "seller_or_service_provider",
        "commercial_owner",
        "payment_recipient",
        "fiscal_responsibility",
        "refund_responsibility",
        "payout_beneficiary",
    }
    for row in rows:
        if not isinstance(row, dict) or not row.get("code"):
            fail("jurisdiction row requires code")
        if row.get("enabled") is not True:
            continue
        if not row.get("product_types") or not row.get("currencies"):
            fail(f"{row.get('code')}: enabled jurisdiction requires product_types and currencies")
        roles = row.get("legal_financial_roles") or {}
        missing = sorted(required_roles - set(roles))
        if missing:
            fail(f"{row.get('code')}: missing legal/financial roles {missing}")

def validate_retention(policy: dict) -> None:
    rows = policy.get("data_classes")
    if not isinstance(rows, dict) or not rows:
        fail("retention matrix must contain data_classes")
    required_classes = {
        "PUBLIC", "INTERNAL", "SENSITIVE", "RAW_CONSULTATION",
        "RAW_PERSONA", "FINANCIAL_EVIDENCE", "CREDENTIAL",
    }
    missing = sorted(required_classes - set(rows))
    if missing:
        fail(f"retention matrix missing DataClass entries: {missing}")
    for name, row in rows.items():
        if not isinstance(row, dict):
            fail(f"{name}: retention row must be mapping")
        if not positive_number(row.get("retention_seconds"), allow_zero=True):
            fail(f"{name}: invalid retention_seconds")
        if not row.get("deletion") or not row.get("legal_hold"):
            fail(f"{name}: deletion/legal_hold rules are required")

def validate_slo(policy: dict) -> None:
    paths = policy.get("critical_paths")
    if not isinstance(paths, list) or not paths:
        fail("critical_paths must be non-empty")
    for item in paths:
        if not isinstance(item, dict) or not item.get("id"):
            fail("critical path entry requires id")
        availability = item.get("availability_target_percent")
        if not positive_number(availability) or availability > 100:
            fail(f"{item.get('id')}: invalid availability target")
        if "latency_p95_ms" in item and not positive_number(item.get("latency_p95_ms")):
            fail(f"{item.get('id')}: invalid latency_p95_ms")
        if "rpo_seconds" in item and not positive_number(item.get("rpo_seconds"), allow_zero=True):
            fail(f"{item.get('id')}: invalid rpo_seconds")
        if "rto_seconds" in item and not positive_number(item.get("rto_seconds")):
            fail(f"{item.get('id')}: invalid rto_seconds")

def validate_providers(policy: dict) -> None:
    capabilities = policy.get("capabilities")
    providers = policy.get("providers")
    if not isinstance(capabilities, dict) or not capabilities:
        fail("provider matrix requires capabilities")
    if not isinstance(providers, dict) or not providers:
        fail("provider matrix requires providers")
    for capability, row in capabilities.items():
        if not isinstance(row, dict):
            fail(f"{capability}: capability row must be mapping")
        if row.get("enabled") is not True:
            continue
        primary = row.get("primary_provider")
        fallback = row.get("fallback_providers")
        if not primary or primary not in providers:
            fail(f"{capability}: primary provider missing from provider registry")
        if not isinstance(fallback, list):
            fail(f"{capability}: fallback_providers must be a list")
        for provider in fallback:
            if provider not in providers:
                fail(f"{capability}: fallback provider {provider} is unknown")
        for field in (
            "degraded_behavior",
            "certification_status",
            "exit_semantics",
            "provider_neutral_contract",
        ):
            if field not in row:
                fail(f"{capability}: missing {field}")
        if row.get("provider_neutral_contract") is not True:
            fail(f"{capability}: canonical contract must remain provider-neutral")

def validate_market_cell(policy: dict) -> None:
    if policy.get("policy_kind") != "MARKET_CELL_THRESHOLDS":
        fail("market_cell policy_kind must be MARKET_CELL_THRESHOLDS")
    cells = policy.get("market_cells")
    if not isinstance(cells, list) or not cells:
        fail("market_cell policy requires at least one market cell")

    nonnegative = {
        "eligible_verified_supply_min",
        "active_specialists_min",
        "duty_supply_min",
        "time_to_available_slot_median_max_minutes",
        "time_to_available_slot_p95_max_minutes",
        "response_time_p95_max_minutes",
        "acceptance_time_p95_max_minutes",
    }
    percentages = {
        "bookable_slot_coverage_min_percent",
        "fill_conversion_min_percent",
        "booking_conversion_min_percent",
        "cancellation_max_percent",
        "no_show_max_percent",
        "unfilled_demand_max_percent",
        "complaint_rate_max_percent",
        "safety_incident_rate_max_percent",
    }
    states = {"DISCOVERY", "SUPPLY_SEEDING", "DEMAND_TEST", "SCALE_READY"}

    for cell in cells:
        if not isinstance(cell, dict) or not cell.get("id"):
            fail("market_cell row requires id")
        for field in ("jurisdiction", "topic", "format", "time_window"):
            if not isinstance(cell.get(field), str) or not cell.get(field).strip():
                fail(f"{cell.get('id')}: missing {field}")
        if cell.get("state") not in states:
            fail(f"{cell.get('id')}: invalid market cell state")

        thresholds = cell.get("thresholds")
        if not isinstance(thresholds, dict):
            fail(f"{cell.get('id')}: thresholds must be a mapping")
        missing = sorted((nonnegative | percentages | {"contribution_margin_min_percent"}) - set(thresholds))
        if missing:
            fail(f"{cell.get('id')}: missing MarketCell thresholds {missing}")
        unknown = sorted(set(thresholds) - (nonnegative | percentages | {"contribution_margin_min_percent"}))
        if unknown:
            fail(f"{cell.get('id')}: unknown MarketCell thresholds {unknown}")

        for field in nonnegative:
            if not positive_number(thresholds.get(field), allow_zero=True):
                fail(f"{cell.get('id')}: invalid {field}")
        for field in percentages:
            if not bounded_number(thresholds.get(field), 0, 100):
                fail(f"{cell.get('id')}: invalid {field}")
        if not bounded_number(thresholds.get("contribution_margin_min_percent"), -100, 100):
            fail(f"{cell.get('id')}: invalid contribution_margin_min_percent")

        evidence = cell.get("evidence_refs")
        if not isinstance(evidence, list):
            fail(f"{cell.get('id')}: evidence_refs must be a list")
        if any(not isinstance(ref, str) or not ref.strip() for ref in evidence):
            fail(f"{cell.get('id')}: evidence_refs cannot contain blank values")

        if cell.get("state") == "SCALE_READY":
            if not evidence:
                fail(f"{cell.get('id')}: SCALE_READY requires recorded evidence_refs")
            if not isinstance(cell.get("evidence_id"), str) or not cell.get("evidence_id").strip():
                fail(f"{cell.get('id')}: SCALE_READY requires evidence_id")
            if not isinstance(cell.get("observed_at"), str) or not cell.get("observed_at").strip():
                fail(f"{cell.get('id')}: SCALE_READY requires observed_at")

            observed = cell.get("observed_metrics")
            if not isinstance(observed, dict):
                fail(f"{cell.get('id')}: SCALE_READY requires observed_metrics")

            minimum_checks = {
                "eligible_verified_supply": "eligible_verified_supply_min",
                "active_specialists": "active_specialists_min",
                "bookable_slot_coverage_percent": "bookable_slot_coverage_min_percent",
                "duty_supply": "duty_supply_min",
                "fill_conversion_percent": "fill_conversion_min_percent",
                "booking_conversion_percent": "booking_conversion_min_percent",
                "contribution_margin_percent": "contribution_margin_min_percent",
            }
            maximum_checks = {
                "time_to_available_slot_median_minutes": "time_to_available_slot_median_max_minutes",
                "time_to_available_slot_p95_minutes": "time_to_available_slot_p95_max_minutes",
                "cancellation_percent": "cancellation_max_percent",
                "no_show_percent": "no_show_max_percent",
                "response_time_p95_minutes": "response_time_p95_max_minutes",
                "acceptance_time_p95_minutes": "acceptance_time_p95_max_minutes",
                "unfilled_demand_percent": "unfilled_demand_max_percent",
                "complaint_rate_percent": "complaint_rate_max_percent",
                "safety_incident_rate_percent": "safety_incident_rate_max_percent",
            }

            for metric, threshold_name in minimum_checks.items():
                value = observed.get(metric)
                threshold = thresholds[threshold_name]
                if not isinstance(value, (int, float)) or isinstance(value, bool):
                    fail(f"{cell.get('id')}: observed metric {metric} must be numeric")
                if value < threshold:
                    fail(
                        f"{cell.get('id')}: SCALE_READY blocked; "
                        f"{metric}={value} is below {threshold_name}={threshold}"
                    )

            for metric, threshold_name in maximum_checks.items():
                value = observed.get(metric)
                threshold = thresholds[threshold_name]
                if not isinstance(value, (int, float)) or isinstance(value, bool):
                    fail(f"{cell.get('id')}: observed metric {metric} must be numeric")
                if value > threshold:
                    fail(
                        f"{cell.get('id')}: SCALE_READY blocked; "
                        f"{metric}={value} exceeds {threshold_name}={threshold}"
                    )

def validate_store_commerce(policy: dict) -> None:
    if policy.get("policy_kind") != "STORE_COMMERCE":
        fail("store_commerce policy_kind must be STORE_COMMERCE")
    if policy.get("unknown_path") != "PURCHASE_DISABLED":
        fail("store_commerce unknown_path must be PURCHASE_DISABLED")

    rules = policy.get("rules")
    if not isinstance(rules, list) or not rules:
        fail("store_commerce policy requires explicit rules")

    seen: set[tuple[str, str, str, str, str]] = set()
    for rule in rules:
        if not isinstance(rule, dict):
            fail("store_commerce rule must be a mapping")
        key_fields = ("product_type", "surface", "store", "storefront", "jurisdiction")
        values: list[str] = []
        for field in key_fields:
            value = rule.get(field)
            if not isinstance(value, str) or not value.strip():
                fail(f"store_commerce rule requires {field}")
            values.append(value)
        key = tuple(values)
        if key in seen:
            fail(f"store_commerce duplicate rule for {key}")
        seen.add(key)

        if rule.get("surface") not in {"IOS", "ANDROID"}:
            fail("store_commerce surface must be IOS or ANDROID")
        enabled = rule.get("enabled")
        rail = rule.get("rail")
        reason = rule.get("reason_code")
        if not isinstance(enabled, bool):
            fail("store_commerce enabled must be boolean")
        if not isinstance(rail, str) or not rail.strip():
            fail("store_commerce rail is required")
        if not isinstance(reason, str) or not reason.strip():
            fail("store_commerce reason_code is required")
        if enabled and rail == "PURCHASE_DISABLED":
            fail("enabled store_commerce rule cannot use PURCHASE_DISABLED")
        if not enabled and rail != "PURCHASE_DISABLED":
            fail("disabled store_commerce rule must use PURCHASE_DISABLED")


def validate_booking_fulfillment(policy: dict) -> None:
    if policy.get("policy_kind") != "BOOKING_FULFILLMENT":
        fail("booking_fulfillment policy_kind must be BOOKING_FULFILLMENT")
    if policy.get("unknown_path") != "BLOCK":
        fail("booking_fulfillment unknown_path must be BLOCK")

    cancellation = policy.get("cancellation")
    no_show = policy.get("no_show")
    refund = policy.get("refund")
    reschedule = policy.get("reschedule")
    for name, row in (
        ("cancellation", cancellation),
        ("no_show", no_show),
        ("refund", refund),
        ("reschedule", reschedule),
    ):
        if not isinstance(row, dict):
            fail(f"booking_fulfillment {name} policy is required")

    numeric_fields = (
        ("cancellation.free_cancel_until_seconds_before_start", cancellation.get("free_cancel_until_seconds_before_start"), 0, None),
        ("cancellation.late_cancel_until_seconds_before_start", cancellation.get("late_cancel_until_seconds_before_start"), 0, None),
        ("cancellation.late_cancel_refund_basis_points", cancellation.get("late_cancel_refund_basis_points"), 0, 10000),
        ("no_show.participant_grace_seconds", no_show.get("participant_grace_seconds"), 0, None),
        ("refund.late_cancel_refund_basis_points", refund.get("late_cancel_refund_basis_points"), 0, 10000),
        ("refund.no_show_refund_basis_points", refund.get("no_show_refund_basis_points"), 0, 10000),
        ("reschedule.minimum_seconds_before_start", reschedule.get("minimum_seconds_before_start"), 0, None),
        ("reschedule.max_reschedules_per_booking", reschedule.get("max_reschedules_per_booking"), 0, None),
    )
    for field, value, minimum, maximum in numeric_fields:
        if not isinstance(value, int) or isinstance(value, bool) or value < minimum:
            fail(f"{field} must be an explicit non-negative integer")
        if maximum is not None and value > maximum:
            fail(f"{field} must be within 0..{maximum}")

    if cancellation["late_cancel_until_seconds_before_start"] > cancellation["free_cancel_until_seconds_before_start"]:
        fail("late cancellation window cannot exceed free cancellation window")

    required_actions = (
        ("cancellation.after_start_action", cancellation.get("after_start_action")),
        ("no_show.client_no_show_action", no_show.get("client_no_show_action")),
        ("no_show.specialist_no_show_action", no_show.get("specialist_no_show_action")),
        ("refund.technical_failure_action", refund.get("technical_failure_action")),
    )
    for field, value in required_actions:
        if not isinstance(value, str) or not value.strip() or value == "CONFIG_REQUIRED":
            fail(f"{field} must be explicit")


def validate_provider_settlement(policy: dict) -> None:
    if policy.get("policy_kind") != "PROVIDER_SETTLEMENT":
        fail("provider_settlement policy_kind must be PROVIDER_SETTLEMENT")
    if policy.get("unknown_path") != "BLOCK_POLICY_NOT_CONFIGURED":
        fail("provider_settlement unknown_path must be BLOCK_POLICY_NOT_CONFIGURED")

    rules = policy.get("commission_rules")
    if not isinstance(rules, list) or not rules:
        fail("provider_settlement requires commission_rules")

    seen: set[tuple[str, str, str]] = set()
    for rule in rules:
        if not isinstance(rule, dict):
            fail("provider_settlement commission rule must be a mapping")
        for field in (
            "id",
            "commission_policy_version",
            "demand_source",
            "product_ref",
            "jurisdiction",
            "currency",
            "rounding_mode",
            "platform_fee_recipient_ref",
        ):
            value = rule.get(field)
            if not isinstance(value, str) or not value.strip() or value == "CONFIG_REQUIRED":
                fail(f"provider_settlement rule requires explicit {field}")
        if rule["currency"] != rule["currency"].upper() or len(rule["currency"]) != 3:
            fail("provider_settlement currency must be uppercase ISO-style code")
        bps = rule.get("basis_points")
        if not isinstance(bps, int) or isinstance(bps, bool) or not 0 <= bps <= 10000:
            fail("provider_settlement basis_points must be within 0..10000")
        if rule["rounding_mode"] != "FLOOR_MINOR":
            fail("provider_settlement rounding_mode must be FLOOR_MINOR")
        key = (rule["demand_source"], rule["product_ref"], rule["jurisdiction"])
        if key in seen:
            fail(f"provider_settlement duplicate source/product/jurisdiction rule: {key}")
        seen.add(key)

    execution = policy.get("provider_execution")
    if not isinstance(execution, dict):
        fail("provider_settlement provider_execution is required")
    required_settlement = set(execution.get("required_settlement_capabilities") or [])
    if not {"MARKETPLACE_SPLIT", "SETTLEMENT_EXECUTION"}.issubset(required_settlement):
        fail("provider_settlement requires MARKETPLACE_SPLIT and SETTLEMENT_EXECUTION")
    required_payout = set(execution.get("required_payout_capabilities") or [])
    if "PAYOUT_EXECUTION" not in required_payout:
        fail("provider_settlement requires PAYOUT_EXECUTION")
    if execution.get("execution_owner") != "EXTERNAL_PROVIDER":
        fail("provider_settlement execution_owner must be EXTERNAL_PROVIDER")
    if execution.get("historical_provider_affinity") != "REQUIRED":
        fail("provider_settlement historical_provider_affinity must be REQUIRED")
    if execution.get("cross_provider_money_bridge") != "FORBIDDEN":
        fail("provider_settlement cross_provider_money_bridge must be FORBIDDEN")


def validate_commerce(policy: dict) -> None:
    if policy.get("policy_kind") != "PRICING_COMMISSION":
        fail("commerce policy_kind must be PRICING_COMMISSION")
    if policy.get("unknown_path") != "BLOCK":
        fail("commerce unknown_path must be BLOCK")

    paths = policy.get("monetized_paths")
    if not isinstance(paths, dict) or not paths:
        fail("commerce policy requires monetized_paths")

    for path_id, row in paths.items():
        if not isinstance(path_id, str) or not path_id.strip() or not isinstance(row, dict):
            fail("commerce monetized path must be a named mapping")
        for field in (
            "pricing_policy_version",
            "commission_policy_version",
            "price_source_ref",
            "currency",
        ):
            value = row.get(field)
            if not isinstance(value, str) or not value.strip() or value == "CONFIG_REQUIRED":
                fail(f"{path_id}: explicit {field} is required")
        currency = row["currency"]
        if len(currency) != 3 or currency != currency.upper():
            fail(f"{path_id}: currency must be uppercase ISO-style code")

        commission = row.get("commission")
        if not isinstance(commission, dict) or commission.get("kind") != "PERCENT_BPS":
            fail(f"{path_id}: explicit PERCENT_BPS commission is required")
        bps = commission.get("basis_points")
        if not isinstance(bps, int) or isinstance(bps, bool) or not 0 <= bps <= 10000:
            fail(f"{path_id}: commission basis_points must be within 0..10000")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("config")
    parser.add_argument("--mode", choices=("ci", "production"), required=True)
    parser.add_argument("--profile", choices=("R0", "R1", "R2", "R3", "R4", "R5"), default="R0")
    args = parser.parse_args()

    config_path = (ROOT / args.config).resolve()
    if ROOT not in config_path.parents or not config_path.is_file():
        fail("config path missing or outside repository")

    config = yaml.safe_load(config_path.read_text(encoding="utf-8")) or {}
    if contains_config_required(config):
        fail("active config contains CONFIG_REQUIRED")

    if args.mode == "production":
        if config.get("environment") != "PRODUCTION":
            fail("production preflight requires environment=PRODUCTION")
        if config.get("production_approved") is not True:
            fail("production config is not approved")
    else:
        if config.get("environment") != "CI":
            fail("CI preflight requires environment=CI")

    selected_policies = {
        "R0": POLICIES,
        "R1": R1_POLICIES,
        "R2": R2_POLICIES,
        "R3": R3_POLICIES,
        "R4": R4_POLICIES,
        "R5": R5_POLICIES,
    }[args.profile]

    loaded: dict[str, tuple[Path, dict]] = {}
    for label, (version_field, path_field) in selected_policies.items():
        version = config.get(version_field)
        if not isinstance(version, str) or not version.strip():
            fail(f"missing explicit version: {version_field}")
        path, policy = load_repo_yaml(config.get(path_field), label)
        if policy.get("version", policy.get("policy_version")) != version:
            fail(f"{label} policy version mismatch")
        validate_scope(policy, args.mode, label)
        loaded[label] = (path, policy)

    validate_jurisdiction(loaded["jurisdiction"][1])
    validate_retention(loaded["retention"][1])
    validate_slo(loaded["slo"][1])
    validate_providers(loaded["providers"][1])
    if "market_cell" in loaded:
        validate_market_cell(loaded["market_cell"][1])
    if "commerce" in loaded:
        validate_commerce(loaded["commerce"][1])
    if "store_commerce" in loaded:
        validate_store_commerce(loaded["store_commerce"][1])
    if "booking_fulfillment" in loaded:
        validate_booking_fulfillment(loaded["booking_fulfillment"][1])
    if "provider_settlement" in loaded:
        validate_provider_settlement(loaded["provider_settlement"][1])

    refs = ", ".join(
        f"{label}={path.relative_to(ROOT)}"
        for label, (path, _) in loaded.items()
    )
    print(
        "LAUNCH CONFIG PRECHECK: PASS "
        f"(mode={args.mode}, profile={args.profile}, config={config_path.relative_to(ROOT)}, {refs})"
    )

if __name__ == "__main__":
    main()
