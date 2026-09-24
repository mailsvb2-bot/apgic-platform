#!/usr/bin/env python3
from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
REQUIRED_PROHIBITED_FIELDS = {
    "token",
    "password",
    "payment_secret",
    "raw_consultation",
    "raw_transcript",
    "raw_audio",
    "raw_video",
    "avatar_raw",
}


def fail(message: str) -> None:
    print(f"MOBILE REALTIME PRECHECK: FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def validate_policy(policy: dict, mode: str) -> None:
    version = policy.get("policy_version")
    if not isinstance(version, str) or not version.strip() or version == "CONFIG_REQUIRED":
        fail("policy_version must be explicit")

    if mode == "production":
        if policy.get("scope") != "PRODUCTION" or policy.get("production_approved") is not True:
            fail("production realtime policy must be approved")
    elif policy.get("scope") != "CI_ONLY":
        fail("CI realtime policy must have scope=CI_ONLY")

    reconnect = policy.get("reconnect")
    if not isinstance(reconnect, dict):
        fail("reconnect policy is required")
    max_attempts = reconnect.get("max_attempts")
    if not isinstance(max_attempts, int) or isinstance(max_attempts, bool) or max_attempts < 0:
        fail("reconnect.max_attempts must be a non-negative integer")
    backoff = reconnect.get("backoff_ms")
    if not isinstance(backoff, list) or len(backoff) != max_attempts:
        fail("reconnect.backoff_ms length must equal max_attempts")
    if any(not isinstance(item, int) or isinstance(item, bool) or item <= 0 for item in backoff):
        fail("reconnect.backoff_ms must contain positive integers")
    if reconnect.get("allow_in_background") is not True:
        fail("CI policy must explicitly exercise background reconnect")

    permissions = policy.get("permissions") or {}
    if permissions.get("microphone_required") is not True:
        fail("microphone permission must be explicit")

    network = policy.get("network") or {}
    if network.get("offline_action") != "RECONNECT_WHEN_ONLINE":
        fail("offline network action must be RECONNECT_WHEN_ONLINE")
    if network.get("degraded_action") != "KEEP_SESSION_DEGRADED":
        fail("degraded network action must preserve degraded technical state")

    audio = policy.get("audio") or {}
    if audio.get("route_change_action") != "REFRESH_ROUTE":
        fail("audio route change action must be REFRESH_ROUTE")

    interruption = policy.get("interruption") or {}
    if interruption.get("action") != "PAUSE_MEDIA_AND_RECONNECT":
        fail("interruption action must preserve session and reconnect")

    diagnostics = policy.get("diagnostics")
    if not isinstance(diagnostics, dict):
        fail("diagnostics policy is required")
    if diagnostics.get("export_requires_confirmation") is not True:
        fail("diagnostics export must require explicit confirmation")
    ttl = diagnostics.get("max_export_ttl_seconds")
    if not isinstance(ttl, int) or isinstance(ttl, bool) or ttl <= 0:
        fail("diagnostics max_export_ttl_seconds must be positive")
    prohibited = set(diagnostics.get("prohibited_export_fields") or [])
    missing = sorted(REQUIRED_PROHIBITED_FIELDS - prohibited)
    if missing:
        fail(f"diagnostics prohibited fields missing: {missing}")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("policy")
    parser.add_argument("--mode", choices=("ci", "production"), required=True)
    args = parser.parse_args()

    path = (ROOT / args.policy).resolve()
    if ROOT not in path.parents or not path.is_file():
        fail("policy path missing or outside repository")
    document = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    if not isinstance(document, dict):
        fail("policy must be a mapping")
    validate_policy(document, args.mode)
    print(
        "MOBILE REALTIME PRECHECK: PASS "
        f"(mode={args.mode}, version={document['policy_version']})"
    )


if __name__ == "__main__":
    main()
