#!/usr/bin/env python3
"""Validate an actual, measured R0 PostgreSQL restore-drill observation.

Read-only validation of the existing staging restore evidence v2, not
an assertion that production recovery or launch is certified.
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path

SHA_RE = re.compile(r"^[a-f0-9]{40}$")
DIGEST_RE = re.compile(r"^[a-f0-9]{64}$")
INTEGRITY_PROBES = (
    "audit", "ledger", "direction", "product_owner", "booking_transition",
    "orders_append_only",
)


def verify(doc: object, expected_sha: str, max_age_seconds: int, now: datetime) -> list[str]:
    errors: list[str] = []
    if not isinstance(doc, dict):
        return ["restore evidence must be an object"]
    if not SHA_RE.fullmatch(expected_sha):
        return ["expected deployed commit SHA is invalid"]
    if doc.get("schema_version") != "staging-restore-evidence-v2":
        errors.append("restore evidence v2 required")
    if doc.get("evidence_type") != "STAGING_RESTORE_DRILL" or doc.get("production_evidence") is not False:
        errors.append("restore evidence must be measured staging evidence, never production")
    if doc.get("candidate_sha") != expected_sha:
        errors.append("restore drill is not bound to the currently deployed commit")
    when = doc.get("observed_at")
    try:
        if not isinstance(when, str) or not when.endswith("Z"):
            raise ValueError("not UTC")
        observed = datetime.fromisoformat(when.replace("Z", "+00:00"))
        age = (now - observed).total_seconds()
        if age < 0 or age > max_age_seconds:
            errors.append("restore drill is stale or timestamp is in the future")
    except (ValueError, TypeError, OverflowError):
        errors.append("restore drill timestamp is invalid")
    if not isinstance(doc.get("backup_file_sha256"), str) or not DIGEST_RE.fullmatch(doc["backup_file_sha256"]):
        errors.append("restore backup digest is invalid")
    for name in ("measured_backup_rpo_seconds", "measured_restore_rto_ms"):
        value = doc.get(name)
        if type(value) is not int or value < 0:
            errors.append(name + " is not a measured nonnegative integer")
    counts = ("source_table_count", "restored_table_count", "required_table_count",
              "business_probe_identity_count", "business_probe_organization_count")
    for name in counts:
        value = doc.get(name)
        if type(value) is not int or value <= 0:
            errors.append(name + " must be positive")
    if type(doc.get("source_table_count")) is int and type(doc.get("restored_table_count")) is int:
        if doc["source_table_count"] != doc["restored_table_count"]:
            errors.append("restored and source table counts differ")
    for probe in INTEGRITY_PROBES:
        field = "integrity_probe_" + probe + "_trigger_count"
        if type(doc.get(field)) is not int or doc[field] != 1:
            errors.append(field + " must equal one")
    if doc.get("business_probes_passed") is not True or doc.get("integrity_probes_passed") is not True:
        errors.append("required restore business/integrity probes have not passed")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--evidence", required=True)
    parser.add_argument("--candidate-sha", required=True)
    parser.add_argument("--max-age-seconds", type=int, default=8 * 86400)
    args = parser.parse_args()
    if args.max_age_seconds <= 0:
        print("R0 RESTORE EVIDENCE: FAIL: max age must be positive", file=sys.stderr)
        return 1
    try:
        doc = json.loads(Path(args.evidence).read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        print(f"R0 RESTORE EVIDENCE: FAIL: cannot read evidence ({type(exc).__name__})", file=sys.stderr)
        return 1
    errors = verify(doc, args.candidate_sha, args.max_age_seconds, datetime.now(timezone.utc))
    if errors:
        for err in errors:
            print("R0 RESTORE EVIDENCE: FAIL: " + err, file=sys.stderr)
        return 1
    print("R0 RESTORE EVIDENCE: PASS (measured staging drill, exact deployed SHA)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
