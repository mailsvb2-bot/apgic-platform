#!/usr/bin/env python3
from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
REQUIRED_VERSION_FIELDS = (
    "jurisdiction_matrix_version",
    "retention_policy_version",
    "slo_policy_version",
    "provider_matrix_version",
)

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

def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("config")
    parser.add_argument("--mode", choices=("ci", "production"), required=True)
    args = parser.parse_args()

    config_path = (ROOT / args.config).resolve()
    if ROOT not in config_path.parents:
        fail("config path escapes repository")
    if not config_path.is_file():
        fail(f"missing config: {args.config}")

    config = yaml.safe_load(config_path.read_text(encoding="utf-8")) or {}
    if contains_config_required(config):
        fail("active config contains CONFIG_REQUIRED")

    for field in REQUIRED_VERSION_FIELDS:
        value = config.get(field)
        if not isinstance(value, str) or not value.strip():
            fail(f"missing explicit version: {field}")

    slo_path_raw = config.get("slo_policy_path")
    if not isinstance(slo_path_raw, str) or not slo_path_raw.strip():
        fail("slo_policy_path is required")

    slo_path = (ROOT / slo_path_raw).resolve()
    if ROOT not in slo_path.parents or not slo_path.is_file():
        fail("slo_policy_path is missing or outside repository")

    slo = yaml.safe_load(slo_path.read_text(encoding="utf-8")) or {}
    if slo.get("policy_version") != config.get("slo_policy_version"):
        fail("slo policy version mismatch")

    paths = slo.get("critical_paths")
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

    if args.mode == "production":
        if config.get("environment") != "PRODUCTION":
            fail("production preflight requires environment=PRODUCTION")
        if config.get("production_approved") is not True:
            fail("production config is not approved")
        if slo.get("production_approved") is not True:
            fail("production SLO policy is not approved")
        if slo.get("scope") != "PRODUCTION":
            fail("production SLO policy must have scope=PRODUCTION")
    else:
        if config.get("environment") != "CI":
            fail("CI preflight requires environment=CI")
        if slo.get("scope") != "CI_ONLY":
            fail("CI SLO policy must have scope=CI_ONLY")

    print(
        "LAUNCH CONFIG PRECHECK: PASS "
        f"(mode={args.mode}, config={config_path.relative_to(ROOT)}, "
        f"slo={slo_path.relative_to(ROOT)})"
    )

if __name__ == "__main__":
    main()
