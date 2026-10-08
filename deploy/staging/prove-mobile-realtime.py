#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path
from typing import Any


class APIError(RuntimeError):
    def __init__(self, method: str, path: str, status: int, payload: Any):
        super().__init__(f"{method} {path}: HTTP {status}: {payload!r}")
        self.status = status
        self.payload = payload


class Client:
    def __init__(self, base_url: str):
        self.base_url = base_url.rstrip("/")
        self.opener = urllib.request.build_opener()
        self.session_cookie: str | None = None

    def call(self, method: str, path: str, payload: Any | None = None) -> tuple[int, Any]:
        data = None if payload is None else json.dumps(payload).encode("utf-8")
        headers = {"Content-Type": "application/json"} if data is not None else {}
        if self.session_cookie is not None:
            headers["Cookie"] = self.session_cookie
        request = urllib.request.Request(self.base_url + path, data=data, method=method, headers=headers)
        try:
            with self.opener.open(request, timeout=15) as response:
                if self.session_cookie is None:
                    for value in response.headers.get_all("Set-Cookie", []):
                        candidate = value.split(";", 1)[0].strip()
                        if candidate.startswith("__Host-apgic_session="):
                            self.session_cookie = candidate
                            break
                raw = response.read().decode("utf-8")
                return response.status, json.loads(raw) if raw else {}
        except urllib.error.HTTPError as error:
            raw = error.read().decode("utf-8")
            try:
                parsed = json.loads(raw) if raw else {}
            except json.JSONDecodeError:
                parsed = {"raw": raw}
            raise APIError(method, path, error.code, parsed) from error


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://127.0.0.1:43111")
    parser.add_argument("--output", default="evidence/mobile009-staging-realtime.json")
    args = parser.parse_args()

    client = Client(args.base_url)
    _, meta = client.call("GET", "/v1/meta")
    proof_id = str(uuid.uuid4())
    _, intent = client.call("POST", "/v1/help-intents", {"free_text": f"mobile009 staging proof {proof_id}"})
    client.call(
        "POST",
        f"/v1/help-intents/{intent['id']}/confirm",
        {"topics": ["anxiety"], "goals": [], "context": {}},
    )
    _, slots_payload = client.call("GET", "/v1/specialists/spec-sokolov/slots")

    hold = None
    hold_errors: list[str] = []
    for slot in reversed(slots_payload.get("slots", [])):
        try:
            _, hold = client.call(
                "POST",
                "/v1/slot-holds",
                {"help_intent_id": intent["id"], "slot_id": slot["id"]},
            )
            break
        except APIError as error:
            hold_errors.append(str(error))
    require(hold is not None, f"no staging slot could be held: {hold_errors}")

    _, checkout_options = client.call(
        "GET",
        f"/v1/slot-holds/{hold['id']}/checkout-options",
    )
    options = checkout_options.get("options", [])
    require(bool(options), f"checkout options are empty: {checkout_options!r}")
    method_code = options[0].get("method_code")
    require(isinstance(method_code, str) and bool(method_code), f"checkout method missing: {options!r}")
    _, checkout = client.call(
        "POST",
        "/v1/checkout-instructions",
        {"hold_id": hold["id"], "method_code": method_code},
    )
    # Staging is a real boundary check, NOT a fake payment provider.
    # No customer-supplied CAPTURED message may confirm a booking.
    denied_status = None
    denied_code = None
    try:
        client.call(
            "POST",
            "/v1/provider-events",
            {
                "provider_id": checkout["provider_id"],
                "provider_event_id": f"mobile009-untrusted-{proof_id}",
                "order_id": checkout["order_id"],
                "amount_minor": checkout["amount_minor"],
                "currency": checkout["currency"],
                "outcome": "CAPTURED",
            },
        )
    except APIError as error:
        denied_status = error.status
        if isinstance(error.payload, dict):
            denied_code = error.payload.get("code")

    require(meta.get("conformance_provider_events") is False, "staging exposed synthetic payment confirmation")
    require(denied_status == 403, f"client-forged provider capture was accepted: {denied_status}")
    require(denied_code == "PROVIDER_EVIDENCE_UNVERIFIED", f"unexpected provider boundary: {denied_code}")
    require(checkout.get("booking_state") == "PENDING_PAYMENT", f"unexpected booking state: {checkout!r}")

    # No communication room is available without trusted external confirmation.
    no_access_status = None
    try:
        client.call("POST", f"/v1/consultations/{hold['booking_id']}/presence", {})
    except APIError as error:
        no_access_status = error.status
    require(no_access_status is not None and no_access_status >= 400, "unpaid booking entered consultation")

    proof = {
        "requirement_id": "APGIC-MOBILE-009",
        "evidence_kind": "STAGING_UNTRUSTED_PAYMENT_NEGATIVE_PROOF",
        "commit_sha": meta.get("commit_sha"),
        "release_track": meta.get("release_track"),
        "proof_id": proof_id,
        "booking_id": hold["booking_id"],
        "booking_state": checkout.get("booking_state"),
        "client_capture_http_status": denied_status,
        "client_capture_error_code": denied_code,
        "unpaid_consultation_http_status": no_access_status,
        "apgic_accepts_funds": False,
        "positive_realtime_path_proven": False,
        "positive_path_test_boundary": "ISOLATED_CONFORMANCE_ONLY",
        "proved_at_unix": int(time.time()),
    }
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(proof, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(proof, ensure_ascii=False, indent=2))
    print("APGIC MOBILE-009 staging untrusted-payment boundary proof: PASS")


if __name__ == "__main__":
    main()