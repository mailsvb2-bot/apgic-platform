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

    # Staging must not manufacture a checkout path when no verified external
    # payment provider is configured. Holding time is not payment confirmation.
    checkout_unavailable_status = None
    checkout_unavailable_code = None
    try:
        client.call(
            "GET",
            f"/v1/slot-holds/{hold['id']}/checkout-options",
        )
    except APIError as error:
        checkout_unavailable_status = error.status
        if isinstance(error.payload, dict):
            checkout_unavailable_code = error.payload.get("code")

    require(meta.get("conformance_provider_events") is False, "staging exposed synthetic payment confirmation")
    require(
        checkout_unavailable_status == 503,
        f"staging exposed synthetic checkout options: {checkout_unavailable_status}",
    )
    require(
        checkout_unavailable_code == "PAYMENT_PROVIDER_UNAVAILABLE",
        f"unexpected checkout availability boundary: {checkout_unavailable_code}",
    )

    # Customer-originated CAPTURED assertions stay forbidden independently of
    # whether a checkout instruction exists.
    denied_status = None
    denied_code = None
    try:
        client.call(
            "POST",
            "/v1/provider-events",
            {
                "provider_id": "untrusted-browser",
                "provider_event_id": f"mobile009-untrusted-{proof_id}",
                "order_id": f"untrusted-{proof_id}",
                "amount_minor": 1,
                "currency": "RUB",
                "outcome": "CAPTURED",
            },
        )
    except APIError as error:
        denied_status = error.status
        if isinstance(error.payload, dict):
            denied_code = error.payload.get("code")

    require(denied_status == 403, f"client-forged provider capture was accepted: {denied_status}")
    require(denied_code == "PROVIDER_EVIDENCE_UNVERIFIED", f"unexpected provider boundary: {denied_code}")
    require(hold.get("booking_state") == "HELD", f"unexpected held booking state: {hold!r}")

    # The authoritative booking read must stay HELD after a forged capture,
    # rather than relying on the stale result of POST /v1/slot-holds.
    status, current_hold = client.call("GET", f"/v1/slot-holds/{hold['id']}")
    require(status == 200, f"owner could not read current hold: {status}")
    require(current_hold.get("booking_id") == hold["booking_id"], "hold identity changed")
    require(current_hold.get("booking_state") == "HELD", f"forged capture changed server booking: {current_hold!r}")

    # An unrelated browser session has no right to read even a valid hold ID.
    outsider = Client(args.base_url)
    outsider_status = None
    outsider_code = None
    try:
        outsider.call("GET", f"/v1/slot-holds/{hold['id']}")
    except APIError as error:
        outsider_status = error.status
        if isinstance(error.payload, dict):
            outsider_code = error.payload.get("code")
    require(outsider_status == 401 and outsider_code == "CLIENT_SESSION_REQUIRED",
            f"unauthenticated client accessed hold: {outsider_status} {outsider_code}")

    # A forged unsigned webhook must not cross the provider trust boundary.
    unsigned_webhook_status = None
    unsigned_webhook_code = None
    try:
        client.call("POST", "/v1/provider-webhooks", {
            "connector_id": "untrusted-browser",
            "external_event_id": f"forged-{proof_id}",
            "stream_id": f"payment/forged-{proof_id}",
            "payload": {"outcome": "CAPTURED"},
        })
    except APIError as error:
        unsigned_webhook_status = error.status
        if isinstance(error.payload, dict):
            unsigned_webhook_code = error.payload.get("code")
    require(unsigned_webhook_status == 401 and unsigned_webhook_code == "PROVIDER_WEBHOOK_UNVERIFIED",
            f"unsigned payment webhook accepted: {unsigned_webhook_status} {unsigned_webhook_code}")
    _, final_hold = client.call("GET", f"/v1/slot-holds/{hold['id']}")
    require(final_hold.get("booking_state") == "HELD",
            f"untrusted webhook changed authoritative booking: {final_hold!r}")


    # No communication room is available from a mere hold.
    no_access_status = None
    try:
        client.call("POST", f"/v1/consultations/{hold['booking_id']}/presence", {})
    except APIError as error:
        no_access_status = error.status
    require(no_access_status is not None and no_access_status >= 400, "unpaid booking entered consultation")

    # A browser cannot claim communication-provider facts. Assert each real
    # deployed endpoint rejects the request before any lifecycle mutation.
    consultation_denials: dict[str, dict[str, Any]] = {}
    for action in ("presence", "failures", "recovery", "complete"):
        denied_http = None
        denied_reason = None
        try:
            client.call("POST", f"/v1/consultations/{hold['booking_id']}/{action}", {})
        except APIError as error:
            denied_http = error.status
            if isinstance(error.payload, dict):
                denied_reason = error.payload.get("code")
        require(
            denied_http == 403 and denied_reason == "CONSULT_PROVIDER_EVIDENCE_UNVERIFIED",
            f"unverified consultation {action} was not rejected by provider boundary: {denied_http}, {denied_reason}",
        )
        consultation_denials[action] = {"http_status": denied_http, "reason_code": denied_reason}
    _, unchanged_hold = client.call("GET", f"/v1/slot-holds/{hold['id']}")
    require(
        unchanged_hold.get("booking_state") == "HELD",
        f"forged communication facts changed booking state: {unchanged_hold!r}",
    )

    proof = {
        "requirement_id": "APGIC-MOBILE-009",
        "evidence_kind": "STAGING_UNTRUSTED_PAYMENT_NEGATIVE_PROOF",
        "commit_sha": meta.get("commit_sha"),
        "release_track": meta.get("release_track"),
        "proof_id": proof_id,
        "booking_id": hold["booking_id"],
        "booking_state": hold.get("booking_state"),
        "checkout_http_status": checkout_unavailable_status,
        "checkout_error_code": checkout_unavailable_code,
        "client_capture_http_status": denied_status,
        "client_capture_error_code": denied_code,
        "authoritative_booking_state": final_hold.get("booking_state"),
        "unauthenticated_hold_http_status": outsider_status,
        "unauthenticated_hold_error_code": outsider_code,
        "unsigned_webhook_http_status": unsigned_webhook_status,
        "unsigned_webhook_error_code": unsigned_webhook_code,
        "untrusted_consultation_lifecycle_denials": consultation_denials,
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