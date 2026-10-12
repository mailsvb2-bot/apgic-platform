#!/usr/bin/env python3
"""Fail-closed release check for actual correlated R0-R4 production journeys.

An indexed and hashed self-report is not provider attestation. Independently
check external proof authenticity before human approval; this tool never deploys.
"""
from __future__ import annotations

import hashlib
import json
import re
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[1]
SHA = re.compile(r"^[a-f0-9]{40}$")
DIGEST = re.compile(r"^[a-f0-9]{64}$")
IDENTIFIER = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/-]*$")
EVIDENCE_DIR = "canon/evidence/production/journeys/"
JOURNEYS = {
    "r0-tenant-denial": ("R0", ("REQUEST", "POLICY_DENY", "AUDIT")),
    "r0-outbox-recovery": ("R0", ("BUSINESS_COMMIT", "PROCESS_RECOVERY", "SINGLE_DELIVERY")),
    "r0-isolated-restore": ("R0", ("BACKUP", "ISOLATED_RESTORE", "BUSINESS_PROBE")),
    "r1-eligible-discovery": ("R1", ("HELP_INTENT", "QUALIFICATION", "ELIGIBLE_RESULT")),
    "r2-booking-payment-entitlement": ("R2", ("SLOT_HOLD", "PROVIDER_CALLBACK", "ENTITLEMENT")),
    "r2-refund-reversal": ("R2", ("CANCELLATION", "PROVIDER_REFUND", "LEDGER_REVERSAL")),
    "r3-consultation-outcome": ("R3", ("ROOM_AUTH", "REALTIME_SESSION", "OUTCOME")),
    "r4-provider-settlement": ("R4", ("PROVIDER_SETTLEMENT", "LEDGER_RECONCILIATION", "PAYOUT_EVIDENCE")),
    "r4-native-store": ("R4", ("SIGNED_CLIENT", "STORE_PURCHASE", "SERVER_ENTITLEMENT")),
}


def utc(value: object) -> datetime | None:
    if not isinstance(value, str) or not value.endswith("Z"):
        return None
    try:
        result = datetime.fromisoformat(value[:-1] + "+00:00")
        return result if result.tzinfo is not None else None
    except ValueError:
        return None


def external_source(value: object) -> bool:
    if not isinstance(value, str):
        return False
    try:
        p = urlsplit(value)
        return (p.scheme == "https" and bool(p.hostname) and bool(p.path.strip("/"))
                and p.hostname not in {"localhost", "127.0.0.1", "::1"}
                and p.username is None and p.password is None and not p.fragment)
    except ValueError:
        return False


def observation_blockers(journey: str, obs: object, record: dict,
                         candidate_sha: str, now: datetime) -> list[str]:
    if not isinstance(obs, dict):
        return ["OBSERVATION_INVALID"]
    expected = {
        "schema_version": "launch-journey-observation-v1",
        "journey_id": journey, "candidate_sha": candidate_sha,
        "environment": "PRODUCTION", "status": "PASS", "synthetic": False,
        "source_ref": record.get("source_ref"),
        "verified_at": record.get("verified_at"),
        "correlation_id": record.get("correlation_id"),
    }
    result = [f"{key.upper()}_MISMATCH" for key, val in expected.items()
              if obs.get(key) != val]
    verifier = obs.get("verified_by")
    if not isinstance(verifier, str) or not verifier.strip() or verifier.lower().startswith("ci-"):
        result.append("VERIFIER_INVALID")
    if not external_source(obs.get("source_ref")):
        result.append("EXTERNAL_SOURCE_REQUIRED")
    chain = obs.get("observed_steps")
    expected_steps = JOURNEYS[journey][1]
    if not isinstance(chain, list) or len(chain) != len(expected_steps):
        result.append("EVENT_CHAIN_INCOMPLETE")
    else:
        ids: set[str] = set()
        last: datetime | None = None
        for n, (step, phase) in enumerate(zip(chain, expected_steps)):
            if not isinstance(step, dict) or step.get("phase") != phase:
                result.append(f"STEP_{n}_PHASE_MISMATCH")
                continue
            if step.get("correlation_id") != obs.get("correlation_id"):
                result.append(f"STEP_{n}_CORRELATION_MISMATCH")
            eid = step.get("event_id")
            if not isinstance(eid, str) or not eid.strip() or eid in ids:
                result.append(f"STEP_{n}_EVENT_ID_INVALID")
            else:
                ids.add(eid)
            owner = step.get("canonical_owner")
            if not isinstance(owner, str) or not owner.strip() or owner.startswith("external:"):
                result.append(f"STEP_{n}_OWNER_INVALID")
            stamp = utc(step.get("observed_at"))
            if stamp is None or stamp > now or (last is not None and stamp < last):
                result.append(f"STEP_{n}_TIME_INVALID")
            if stamp is not None:
                last = stamp
        verified = utc(obs.get("verified_at"))
        if last is not None and (verified is None or verified < last):
            result.append("VERIFIED_BEFORE_OUTCOME")
    outcome = obs.get("business_outcome")
    if not isinstance(outcome, dict) or outcome.get("status") != "CONFIRMED" or not isinstance(outcome.get("canonical_result_id"), str) or not outcome["canonical_result_id"].strip():
        result.append("BUSINESS_OUTCOME_MISSING")
    if isinstance(outcome, dict) and outcome.get("correlation_id") != obs.get("correlation_id"):
        result.append("OUTCOME_CORRELATION_MISMATCH")
    probe = obs.get("failure_or_denial_probe")
    if isinstance(probe, dict) and probe.get("correlation_id") != obs.get("correlation_id"):
        result.append("FAILURE_PROBE_CORRELATION_MISMATCH")
    if not isinstance(probe, dict) or probe.get("status") != "PASS" or not isinstance(probe.get("observed_result"), str) or not probe["observed_result"].strip() or not external_source(probe.get("source_ref")):
        result.append("FAILURE_PROBE_MISSING")
    return result


def evaluate(manifest: dict | None, production_index: dict,
             candidate_sha: str, *, root: Path | None = None,
             now: datetime | None = None) -> tuple[bool, list[str]]:
    root = (root or ROOT).resolve()
    now = now or datetime.now(timezone.utc)
    if manifest is None:
        return False, ["MANIFEST_MISSING"]
    if not isinstance(manifest, dict) or manifest.get("schema_version") != "launch-critical-journeys-v1":
        return False, ["MANIFEST_SCHEMA_INVALID"]
    issues: list[str] = []
    if not SHA.fullmatch(candidate_sha) or manifest.get("candidate_sha") != candidate_sha:
        issues.append("CANDIDATE_SHA_MISMATCH")
    if manifest.get("synthetic") is not False or manifest.get("environment") != "PRODUCTION":
        issues.append("PRODUCTION_PROOF_REQUIRED")
    if not isinstance(production_index, dict) or production_index.get("schema_version") != "production-evidence-index-v1" or not isinstance(production_index.get("records"), dict):
        return False, issues + ["INDEX_INVALID"]
    entries = manifest.get("journeys")
    if not isinstance(entries, dict):
        return False, issues + ["JOURNEYS_INVALID"]
    if set(entries) != set(JOURNEYS):
        issues.append("JOURNEY_SET_INCOMPLETE_OR_UNKNOWN")
    consumed: set[str] = set()
    for name in JOURNEYS:
        row = entries.get(name)
        if not isinstance(row, dict) or not isinstance(row.get("evidence_ref"), str):
            issues.append(f"{name}:PROOF_MISSING")
            continue
        ref = row["evidence_ref"]
        if not ref.startswith("evidence://"):
            issues.append(f"{name}:PRODUCTION_REF_REQUIRED")
            continue
        evidence_id = ref.removeprefix("evidence://")
        if not IDENTIFIER.fullmatch(evidence_id):
            issues.append(f"{name}:EVIDENCE_ID_INVALID")
            continue
        if evidence_id in consumed:
            issues.append(f"{name}:PROOF_REUSED")
            continue
        consumed.add(evidence_id)
        record = production_index["records"].get(evidence_id)
        if not isinstance(record, dict):
            issues.append(f"{name}:NOT_INDEXED")
            continue
        for key, expected in (("evidence_type", "LAUNCH_JOURNEY_E2E"), ("journey_id", name),
                              ("candidate_sha", candidate_sha), ("status", "PASS"), ("synthetic", False)):
            if record.get(key) != expected:
                issues.append(f"{name}:{key.upper()}_MISMATCH")
        if not external_source(record.get("source_ref")):
            issues.append(f"{name}:EXTERNAL_SOURCE_REQUIRED")
        verified = utc(record.get("verified_at"))
        if verified is None or verified > now:
            issues.append(f"{name}:VERIFICATION_TIME_INVALID")
        if not isinstance(record.get("correlation_id"), str) or not record["correlation_id"].strip():
            issues.append(f"{name}:CORRELATION_ID_MISSING")
        rel = record.get("artifact_path")
        if not isinstance(rel, str) or not rel.startswith(EVIDENCE_DIR) or ".." in Path(rel).parts:
            issues.append(f"{name}:PATH_INVALID")
            continue
        path = (root / rel).resolve()
        if not path.is_relative_to(root) or not path.is_file():
            issues.append(f"{name}:ARTIFACT_MISSING")
            continue
        digest = record.get("artifact_sha256")
        if not isinstance(digest, str) or not DIGEST.fullmatch(digest) or hashlib.sha256(path.read_bytes()).hexdigest() != digest:
            issues.append(f"{name}:DIGEST_MISMATCH")
            continue
        try:
            obs = json.loads(path.read_text(encoding="utf-8"))
        except (ValueError, OSError, UnicodeError):
            issues.append(f"{name}:OBSERVATION_INVALID_JSON")
            continue
        issues.extend(f"{name}:{reason}" for reason in observation_blockers(name, obs, record, candidate_sha, now))
    return not issues, sorted(set(issues))
