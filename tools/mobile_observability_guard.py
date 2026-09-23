#!/usr/bin/env python3
from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
REQUIRED_METRICS = {
    "crash_rate",
    "anr_rate",
    "startup_p95_ms",
    "push_delivery_success",
    "deep_link_success",
    "checkout_success",
    "realtime_join_success",
    "reconnect_success",
}
FORBIDDEN_DIMENSIONS = {
    "raw_content",
    "message_text",
    "consultation_text",
    "avatar_raw",
    "token",
    "password",
    "email",
    "phone",
}

def fail(message: str) -> None:
    print(f"MOBILE OBSERVABILITY GUARD: FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)

def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("config")
    parser.add_argument("--mode", choices=("ci", "production"), required=True)
    args = parser.parse_args()

    path = (ROOT / args.config).resolve()
    if ROOT not in path.parents or not path.is_file():
        fail("config path missing or outside repository")
    config = yaml.safe_load(path.read_text(encoding="utf-8")) or {}

    if config.get("sensitive_content_telemetry") != "FORBIDDEN":
        fail("sensitive_content_telemetry must be FORBIDDEN")

    dimensions = set(config.get("allowed_dimensions") or [])
    leaked = sorted(dimensions & FORBIDDEN_DIMENSIONS)
    if leaked:
        fail(f"forbidden telemetry dimensions declared: {leaked}")

    metrics = config.get("metrics") or {}
    missing = sorted(REQUIRED_METRICS - set(metrics))
    if missing:
        fail(f"missing required mobile metrics: {missing}")

    for name in REQUIRED_METRICS:
        row = metrics.get(name) or {}
        if row.get("breach_action") != "HALT_OR_LIMIT_ROLLOUT":
            fail(f"{name}: breach_action must halt or limit rollout")
        if row.get("guardrail_operator") not in {"LTE", "GTE"}:
            fail(f"{name}: invalid guardrail_operator")
        value = row.get("guardrail_value")
        if not isinstance(value, (int, float)) or isinstance(value, bool):
            fail(f"{name}: numeric guardrail_value required")

    if args.mode == "production":
        if config.get("scope") != "PRODUCTION" or config.get("production_approved") is not True:
            fail("production mobile observability policy is not approved")
    else:
        if config.get("scope") != "CI_ONLY":
            fail("CI mobile observability policy must have scope=CI_ONLY")

    print(f"MOBILE OBSERVABILITY GUARD: PASS (mode={args.mode}, metrics={len(metrics)})")

if __name__ == "__main__":
    main()
