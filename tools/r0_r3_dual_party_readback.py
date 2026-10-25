#!/usr/bin/env python3
"""Read-only, exact-SHA R0–R3 dual-party acceptance of a real consultation.

No booking or media event is created. A real completed booking, independently
authorized client/specialist sessions and an unrelated identity are mandatory.
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

# Keep all transport protections from the already accepted single-party probe.
sys.path.insert(0, str(Path(__file__).resolve().parent))
import r0_r3_live_readback as base


def validate_parties(client: dict, specialist: dict, client_result: dict,
                     specialist_result: dict, booking_id: str) -> list[str]:
    errors = []
    for role, value in (("CLIENT", client), ("SPECIALIST", specialist)):
        if value.get("booking_id") != booking_id or value.get("role") != role:
            errors.append(role + "_AUTHORIZATION_NOT_PROVEN")
        if value.get("booking_state") not in ("CONFIRMED", "COMPLETED"):
            errors.append(role + "_BOOKING_NOT_CONFIRMED")
        if not value.get("client_ready_at") or not value.get("specialist_ready_at"):
            errors.append(role + "_BOTH_PARTICIPANTS_NOT_READY")
    refs = []
    for role, result in (("CLIENT", client_result), ("SPECIALIST", specialist_result)):
        if result.get("booking_id") != booking_id or result.get("state") != "COMPLETED":
            errors.append(role + "_PROVIDER_COMPLETION_NOT_PROVEN")
        evidence = result.get("completion_evidence_ref")
        if not isinstance(evidence, str) or not evidence.strip():
            errors.append(role + "_PROVIDER_EVIDENCE_MISSING")
        provider = result.get("provider_instance_id")
        if not isinstance(provider, str) or not provider.strip():
            errors.append(role + "_PROVIDER_ID_MISSING")
        refs.append((provider, evidence))
    if refs[0] != refs[1]:
        errors.append("CROSS_ROLE_PROVIDER_RESULT_MISMATCH")
    return errors



def validate_timelines(client: dict, specialist: dict, booking_id: str) -> list[str]:
    """Demand identical durable completion evidence for both authorized roles."""
    errors = []
    for role, timeline in (("CLIENT", client), ("SPECIALIST", specialist)):
        if timeline.get("booking_id") != booking_id or timeline.get("consultation_state") != "COMPLETED":
            errors.append(role + "_TIMELINE_COMPLETION_NOT_PROVEN")
        facts = timeline.get("facts")
        if not isinstance(facts, list) or not any(
            isinstance(fact, dict) and fact.get("fact_type") == "ENDED"
            and isinstance(fact.get("evidence_ref"), str) and fact["evidence_ref"].strip()
            for fact in facts
        ):
            errors.append(role + "_TIMELINE_END_EVIDENCE_MISSING")
    if not errors:
        def end_refs(timeline):
            return {fact["evidence_ref"] for fact in timeline["facts"]
                    if isinstance(fact, dict) and fact.get("fact_type") == "ENDED"
                    and isinstance(fact.get("evidence_ref"), str) and fact["evidence_ref"].strip()}
        if not end_refs(client).intersection(end_refs(specialist)):
            errors.append("CROSS_ROLE_TIMELINE_EVIDENCE_MISMATCH")
    return errors


def denied_for_stranger(api_base: str, booking_id: str, cookie: str) -> bool:
    import urllib.error
    # An unrelated authenticated identity must not learn the appointment's
    # existence via the booking, readiness or result projections.
    for path in (f"/v1/bookings/{booking_id}",
                 f"/v1/consultations/{booking_id}/ready",
                 f"/v1/consultations/{booking_id}/result",
                 f"/v1/consultations/{booking_id}/timeline"):
        base.assert_local_api_base(api_base)
        request = base.urllib.request.Request(
            api_base.rstrip("/") + path, headers={"Cookie": cookie, "Accept": "application/json"})
        opener = base.urllib.request.build_opener(base.urllib.request.ProxyHandler({}), base.NoRedirect())
        try:
            with opener.open(request, timeout=7):
                return False
        except urllib.error.HTTPError as exc:
            if exc.code != 404:
                return False
        except (urllib.error.URLError, OSError):
            return False
    return True


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--api-base", default="http://127.0.0.1:43111")
    p.add_argument("--candidate-sha", required=True)
    p.add_argument("--booking-id", required=True)
    args = p.parse_args()
    if not base.SHA.fullmatch(args.candidate_sha) or not base.UUID.fullmatch(args.booking_id):
        print("R0-R3 DUAL PARTY: BLOCKED invalid candidate/booking ID", file=sys.stderr)
        return 1
    cookies = [os.environ.get(key, "") for key in (
        "APGIC_R0_R3_ACCEPTANCE_SESSION_COOKIE",
        "APGIC_R0_R3_SPECIALIST_SESSION_COOKIE",
        "APGIC_R0_R3_STRANGER_SESSION_COOKIE")]
    if any(not x or "\r" in x or "\n" in x for x in cookies) or len(set(cookies)) != 3:
        print("R0-R3 DUAL PARTY: BLOCKED independent client/specialist/stranger sessions required", file=sys.stderr)
        return 1
    try:
        base.assert_local_api_base(args.api_base)
        meta = base.fetch_json(args.api_base, "/v1/meta")
        directory = base.fetch_json(args.api_base, "/v1/specialists?limit=10&offset=0")
        booking = base.fetch_json(args.api_base, f"/v1/bookings/{args.booking_id}", cookies[0])
        result_path = f"/v1/consultations/{args.booking_id}/result"
        client_result = base.fetch_json(args.api_base, result_path, cookies[0])
        specialist_result = base.fetch_json(args.api_base, result_path, cookies[1])
        ready_path = f"/v1/consultations/{args.booking_id}/ready"
        client = base.fetch_json(args.api_base, ready_path, cookies[0])
        specialist = base.fetch_json(args.api_base, ready_path, cookies[1])
        timeline_path = f"/v1/consultations/{args.booking_id}/timeline"
        client_timeline = base.fetch_json(args.api_base, timeline_path, cookies[0])
        specialist_timeline = base.fetch_json(args.api_base, timeline_path, cookies[1])
        errors = base.verify_observations(meta, directory, booking, client_result,
                                          args.candidate_sha, args.booking_id)
        errors += validate_parties(client, specialist, client_result, specialist_result,
                                   args.booking_id)
        errors += validate_timelines(client_timeline, specialist_timeline, args.booking_id)
        if not denied_for_stranger(args.api_base, args.booking_id, cookies[2]):
            errors.append("R0_TENANT_PRIVACY_NOT_PROVEN")
    except base.ProbeError as exc:
        print("R0-R3 DUAL PARTY: BLOCKED " + str(exc), file=sys.stderr)
        return 1
    if errors:
        for error in errors:
            print("R0-R3 DUAL PARTY: BLOCKED " + error, file=sys.stderr)
        return 1
    print("R0-R3 DUAL PARTY: PASS independent real participant readback / foreign denial on exact SHA")
    print("LIMIT: provider signature, actual bidirectional media, native apps and human validation need separate evidence")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
