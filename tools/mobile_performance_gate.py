#!/usr/bin/env python3
from __future__ import annotations

import argparse
import math
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]


def fail(message: str) -> None:
    print(f"MOBILE PERFORMANCE GATE: FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def _load(path_arg: str) -> dict:
    path = (ROOT / path_arg).resolve()
    if ROOT not in path.parents or not path.is_file():
        fail("input path missing or outside repository")
    return yaml.safe_load(path.read_text(encoding="utf-8")) or {}


def validate_measurements(policy: dict, measurements: dict) -> None:
    metrics = policy.get("metrics") or {}
    observed = measurements.get("metrics") or {}

    if measurements.get("policy_version") != policy.get("version"):
        fail("measurement policy_version does not match observability policy")

    missing = sorted(set(metrics) - set(observed))
    extra = sorted(set(observed) - set(metrics))
    if missing:
        fail(f"missing measured metrics: {missing}")
    if extra:
        fail(f"unknown measured metrics: {extra}")

    breached: list[str] = []
    for name, rule in metrics.items():
        sample = observed.get(name) or {}
        if sample.get("unit") != rule.get("unit"):
            fail(f"{name}: measurement unit does not match policy")

        value = sample.get("value")
        if not isinstance(value, (int, float)) or isinstance(value, bool) or not math.isfinite(value):
            fail(f"{name}: finite numeric measurement required")

        operator = rule.get("guardrail_operator")
        threshold = rule.get("guardrail_value")
        if operator == "LTE":
            ok = value <= threshold
        elif operator == "GTE":
            ok = value >= threshold
        else:
            fail(f"{name}: unsupported guardrail operator")

        if not ok:
            if rule.get("breach_action") != "HALT_OR_LIMIT_ROLLOUT":
                fail(f"{name}: breach lacks fail-closed rollout action")
            breached.append(name)

    if breached:
        fail("rollout blocked by metric breaches: " + ", ".join(sorted(breached)))


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("policy")
    parser.add_argument("measurements")
    args = parser.parse_args()

    policy = _load(args.policy)
    measurements = _load(args.measurements)
    validate_measurements(policy, measurements)
    print(f"MOBILE PERFORMANCE GATE: PASS (metrics={len(policy.get('metrics') or {})})")


if __name__ == "__main__":
    main()
