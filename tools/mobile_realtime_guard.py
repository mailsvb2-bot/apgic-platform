#!/usr/bin/env python3
from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]

ANDROID_MANIFEST = ROOT / "apps/mobile/android/app/src/main/AndroidManifest.xml"
REALTIME_E2E = ROOT / "apps/mobile/src/r3-realtime-e2e.ts"
MOBILE_APP = ROOT / "apps/mobile/src/App.tsx"
ANDROID_E2E = ROOT / "tools/mobile_android_capability_e2e.sh"
IOS_E2E = ROOT / "tools/mobile_ios_capability_e2e.sh"
REQUIRED_ANDROID_PERMISSIONS = {
    "android.permission.INTERNET",
    "android.permission.ACCESS_NETWORK_STATE",
    "android.permission.RECORD_AUDIO",
}
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
    if network.get("transport_change_action") != "RECONNECT_PROVIDER":
        fail("network transport changes must reconnect the provider")

    screen = policy.get("screen") or {}
    if screen.get("lock_action") != "PAUSE_MEDIA_AND_RECONNECT":
        fail("screen lock policy must pause media and reconnect after unlock")

    auth = policy.get("auth") or {}
    if auth.get("join_token_expiry_action") != "REFRESH_AND_REJOIN":
        fail("join/auth expiry must refresh credentials and rejoin")

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


def validate_android_manifest(text: str) -> None:
    missing = sorted(permission for permission in REQUIRED_ANDROID_PERMISSIONS if permission not in text)
    if missing:
        fail(f"Android realtime permissions missing: {missing}")


def validate_callback_order_evidence(
    realtime_e2e: str,
    mobile_app: str,
    android_e2e: str,
    ios_e2e: str,
) -> None:
    required_realtime = (
        "audioRoutesObserved",
        "networkTransportsObserved",
        "audioRoutesObserved.add(result.snapshot.audio_route)",
        "networkTransportsObserved.add(result.snapshot.network_transport)",
    )
    for marker in required_realtime:
        if marker not in realtime_e2e:
            fail(f"realtime E2E missing observed-state evidence marker: {marker}")

    for marker in (
        "realtime-audio-routes-observed:",
        "realtime-network-transports-observed:",
    ):
        if marker not in mobile_app:
            fail(f"mobile UI missing observed-state evidence marker: {marker}")

    required_android = (
        "realtime-audio-routes-observed:",
        "BLUETOOTH",
        "realtime-network-transports-observed:",
        "CELLULAR",
    )
    for marker in required_android:
        if marker not in android_e2e:
            fail(f"Android realtime E2E missing callback-order-safe evidence: {marker}")
    for forbidden in (
        "grep -q 'realtime-audio-route:BLUETOOTH'",
        "grep -q 'realtime-network-transport:CELLULAR'",
    ):
        if forbidden in android_e2e:
            fail(f"Android realtime E2E must not require synthetic final OS state: {forbidden}")

    required_ios = (
        "json_has_ax_label_contains",
        "realtime-audio-routes-observed:",
        "BLUETOOTH",
        "realtime-network-transports-observed:",
        "CELLULAR",
    )
    for marker in required_ios:
        if marker not in ios_e2e:
            fail(f"iOS realtime E2E missing callback-order-safe evidence: {marker}")
    for forbidden in (
        'json_has_ax_label "$target" "realtime-audio-route:BLUETOOTH"',
        'json_has_ax_label "$target" "realtime-network-transport:CELLULAR"',
    ):
        if forbidden in ios_e2e:
            fail(f"iOS realtime E2E must not require synthetic final OS state: {forbidden}")


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
    validate_android_manifest(ANDROID_MANIFEST.read_text(encoding="utf-8"))
    validate_callback_order_evidence(
        REALTIME_E2E.read_text(encoding="utf-8"),
        MOBILE_APP.read_text(encoding="utf-8"),
        ANDROID_E2E.read_text(encoding="utf-8"),
        IOS_E2E.read_text(encoding="utf-8"),
    )
    print(
        "MOBILE REALTIME PRECHECK: PASS "
        f"(mode={args.mode}, version={document['policy_version']})"
    )


if __name__ == "__main__":
    main()