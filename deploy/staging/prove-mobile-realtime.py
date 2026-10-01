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
        {"topics": ["sleep"], "goals": [], "context": {}},
    )
    _, slots_payload = client.call("GET", "/v1/specialists/spec-lebedeva/slots")

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
    _, capture = client.call(
        "POST",
        "/v1/provider-events",
        {
            "provider_id": checkout["provider_id"],
            "provider_event_id": f"mobile009-capture-{proof_id}",
            "order_id": checkout["order_id"],
            "amount_minor": checkout["amount_minor"],
            "currency": checkout["currency"],
            "outcome": "CAPTURED",
        },
    )
    booking_id = capture["booking_id"]

    _, presence = client.call("POST", f"/v1/consultations/{booking_id}/presence", {})
    _, failure = client.call(
        "POST",
        f"/v1/consultations/{booking_id}/failures",
        {
            "kind": "NETWORK_LOSS",
            "evidence_ref": f"mobile009/network-loss/{proof_id}",
            "recoverable": True,
        },
    )
    _, recovery = client.call(
        "POST",
        f"/v1/consultations/{booking_id}/recovery",
        {"evidence_ref": f"mobile009/recovered/{proof_id}"},
    )

    empty_completion_status = None
    empty_completion_code = None
    try:
        client.call(
            "POST",
            f"/v1/consultations/{booking_id}/complete",
            {"evidence_ref": ""},
        )
    except APIError as error:
        empty_completion_status = error.status
        if isinstance(error.payload, dict):
            empty_completion_code = error.payload.get("code")

    require(presence.get("state") == "IN_PROGRESS", f"unexpected presence state: {presence!r}")
    require(failure.get("state") == "RECOVERING", f"unexpected failure state: {failure!r}")
    require(recovery.get("state") == "IN_PROGRESS", f"unexpected recovery state: {recovery!r}")
    for label, view in (("presence", presence), ("failure", failure), ("recovery", recovery)):
        require(view.get("charged_again") is False, f"{label} charged again: {view!r}")
        require(view.get("apgic_owns_room") is False, f"{label} claimed room ownership: {view!r}")
    require(empty_completion_status == 409, f"empty completion evidence was not rejected: {empty_completion_status}")
    require(empty_completion_code == "CONSULT_EVIDENCE_REQUIRED", f"unexpected completion rejection: {empty_completion_code}")

    explicit_evidence = f"mobile009/provider-end/{proof_id}"
    _, completed = client.call(
        "POST",
        f"/v1/consultations/{booking_id}/complete",
        {"evidence_ref": explicit_evidence},
    )
    require(completed.get("state") == "COMPLETED", f"explicit provider completion failed: {completed!r}")
    require(completed.get("evidence_ref") == explicit_evidence, f"completion evidence drift: {completed!r}")
    require(completed.get("charged_again") is False, f"completion charged again: {completed!r}")

    proof = {
        "requirement_id": "APGIC-MOBILE-009",
        "evidence_kind": "STAGING_PROOF",
        "commit_sha": meta.get("commit_sha"),
        "release_track": meta.get("release_track"),
        "proof_id": proof_id,
        "booking_id": booking_id,
        "presence_state": presence.get("state"),
        "failure_state": failure.get("state"),
        "failure_action": failure.get("recovery_action"),
        "recovery_state": recovery.get("state"),
        "empty_completion_status": empty_completion_status,
        "empty_completion_code": empty_completion_code,
        "explicit_completion_state": completed.get("state"),
        "explicit_completion_evidence_ref": completed.get("evidence_ref"),
        "charged_again": False,
        "apgic_owns_room": False,
        "proved_at_unix": int(time.time()),
    }
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(proof, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(proof, ensure_ascii=False, indent=2))
    print("APGIC MOBILE-009 staging realtime proof: PASS")


if __name__ == "__main__":
    main()