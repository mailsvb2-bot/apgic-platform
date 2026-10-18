#!/usr/bin/env python3
"""Read-only R0–R3 staged customer journey admission probe.

This observes actual API/Web responses of the deployed build and can never
simulate an appointment or media event. It does not mutate the database.
"""
from __future__ import annotations

import argparse
import json
import re
import sys
import urllib.error
import urllib.parse
import urllib.request

UUID = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")
SHA = re.compile(r"^[a-f0-9]{40}$")


class ProbeError(Exception):
    pass


def fetch_json(base: str, path: str, cookie: str = "") -> dict:
    url = base.rstrip("/") + path
    req = urllib.request.Request(url, headers={"Accept": "application/json",
                                               **({"Cookie": cookie} if cookie else {})})
    try:
        with urllib.request.urlopen(req, timeout=7) as response:
            if response.status != 200:
                raise ProbeError(f"{path}: HTTP {response.status}")
            data = response.read(1_000_001)
            if len(data) > 1_000_000:
                raise ProbeError(f"{path}: response too large")
            payload = json.loads(data)
    except (urllib.error.URLError, TimeoutError, ValueError, OSError) as exc:
        raise ProbeError(f"{path}: unavailable or malformed JSON ({type(exc).__name__})") from exc
    if not isinstance(payload, dict):
        raise ProbeError(f"{path}: JSON object required")
    return payload


def verify_observations(meta: dict, directory: dict, booking: dict, result: dict,
                        expected_sha: str, booking_id: str) -> list[str]:
    errors = []
    if meta.get("commit_sha") != expected_sha or meta.get("service") != "apgic-api":
        errors.append("R0_RUNTIME_SHA_MISMATCH")
    profiles = directory.get("profiles")
    if not isinstance(profiles, list) or not profiles:
        errors.append("R1_NO_REAL_PUBLISHED_SPECIALISTS")
    elif not any(isinstance(p, dict) and isinstance(p.get("id"), str) and
                 p.get("display_name") for p in profiles):
        errors.append("R1_PUBLIC_PROFILE_MALFORMED")
    if booking.get("booking_id") != booking_id:
        errors.append("R2_BOOKING_ID_MISMATCH")
    if booking.get("state") not in ("CONFIRMED", "COMPLETED"):
        errors.append("R2_BOOKING_NOT_CONFIRMED")
    if result.get("booking_id") != booking_id:
        errors.append("R3_CONSULTATION_ID_MISMATCH")
    if result.get("state") != "COMPLETED":
        errors.append("R3_CONSULTATION_NOT_COMPLETED")
    if not isinstance(result.get("completion_evidence_ref"), str) or not result["completion_evidence_ref"].strip():
        errors.append("R3_PROVIDER_COMPLETION_EVIDENCE_MISSING")
    return errors


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--api-base", default="http://127.0.0.1:43111")
    p.add_argument("--candidate-sha", required=True)
    p.add_argument("--booking-id", required=True)
    p.add_argument("--session-cookie-env", default="APGIC_R0_R3_ACCEPTANCE_SESSION_COOKIE")
    args = p.parse_args()
    if not SHA.fullmatch(args.candidate_sha) or not UUID.fullmatch(args.booking_id):
        print("R0-R3 READBACK: BLOCKED (invalid SHA or booking UUID)", file=sys.stderr)
        return 1
    import os
    cookie = os.environ.get(args.session_cookie_env, "")
    if not cookie or any(c in cookie for c in "\r\n"):
        print("R0-R3 READBACK: BLOCKED (real authorized session unavailable)", file=sys.stderr)
        return 1
    try:
        meta = fetch_json(args.api_base, "/v1/meta")
        directory = fetch_json(args.api_base, "/v1/specialists?limit=10&offset=0")
        booking = fetch_json(args.api_base, "/v1/bookings/" + args.booking_id, cookie)
        result = fetch_json(args.api_base, "/v1/consultations/" + args.booking_id + "/result", cookie)
        errors = verify_observations(meta, directory, booking, result, args.candidate_sha, args.booking_id)
    except ProbeError as exc:
        print(f"R0-R3 READBACK: BLOCKED ({exc})", file=sys.stderr)
        return 1
    if errors:
        for item in errors:
            print("R0-R3 READBACK: BLOCKED (" + item + ")", file=sys.stderr)
        return 1
    print("R0-R3 READBACK: PASS actual persisted customer journey on deployed SHA")
    print("NOTE: human login, signed media/provider provenance and multi-device acceptance require independent evidence")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
