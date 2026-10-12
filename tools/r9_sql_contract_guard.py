#!/usr/bin/env python3
"""R9 SQL executable contract: catch broken dollar blocks and coverage bypasses.

This static preflight is intentionally paired with live PostgreSQL execution.
It is not an alternative to migration/transaction/concurrency tests.
"""
from __future__ import annotations

from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
MIGRATION = ROOT / "backend/migrations/000032_r9_live_events.sql"
PROOF = ROOT / "backend/migrations/ci_r9_live_events_invariants.sql"
CONCURRENCY = ROOT / "backend/ci/r9_registration_concurrency.sh"
WORKFLOW = ROOT / ".github/workflows/ci.yml"

DOLLAR_TAG = re.compile(r"\$[A-Za-z_][A-Za-z_0-9]*\$|\$\$")
BAD_DO_TAG = re.compile(r"(?im)^\s*DO\s+\$(?!\$|[A-Za-z_][A-Za-z_0-9]*\$)")
DANGLING_CLOSE = re.compile(r"(?m)^\s*\$\s*;\s*$")


def validate_sql_text(sql: str) -> list[str]:
    errors: list[str] = []
    if BAD_DO_TAG.search(sql) or DANGLING_CLOSE.search(sql):
        errors.append("truncated dollar-quote delimiter (DO $ / $;)")
    open_tag: str | None = None
    for match in DOLLAR_TAG.finditer(sql):
        tag = match.group()
        if open_tag is None:
            open_tag = tag
        elif tag == open_tag:
            open_tag = None
        else:
            errors.append(f"unmatched dollar-quote: wanted {open_tag}, got {tag}")
            open_tag = tag
    if open_tag is not None:
        errors.append(f"unclosed dollar-quote {open_tag}")
    return errors


def run() -> list[str]:
    errors: list[str] = []
    for path in (MIGRATION, PROOF):
        if not path.is_file():
            errors.append(f"missing {path.relative_to(ROOT)}")
            continue
        errors.extend(f"{path.name}: {error}" for error in validate_sql_text(path.read_text(encoding="utf-8")))
    if PROOF.exists():
        proof = PROOF.read_text(encoding="utf-8")
        if proof.count("pg_temp.r9_must_reject(") < 10:
            errors.append("R9 must verify at least 10 expected SQL denials")
        for evidence in ("ROLLBACK;", "R9 LIVE EVENT SQL INVARIANTS: PASS", "registered_count", "R9 re-registration capacity exhausted", "R9 event capacity exhausted", "duplicate key value violates unique constraint"):
            if evidence not in proof:
                errors.append(f"R9 proof missing evidence: {evidence}")
    if not CONCURRENCY.is_file():
        errors.append("R9 concurrent registration probe missing")
    else:
        script = CONCURRENCY.read_text(encoding="utf-8")
        for evidence in ("pids=", "capacity=3", "attempts=24", "R9 REGISTRATION CONCURRENCY: PASS"):
            if evidence not in script:
                errors.append(f"R9 concurrency probe missing: {evidence}")
    if not WORKFLOW.is_file():
        errors.append("CI workflow missing")
    else:
        ci = WORKFLOW.read_text(encoding="utf-8")
        for evidence in ("ci_r9_live_events_invariants.sql", "r9_registration_concurrency.sh", "r9_sql_contract_guard.py"):
            if evidence not in ci:
                errors.append(f"CI omits R9 proof: {evidence}")
    return errors


if __name__ == "__main__":
    found = run()
    if found:
        for failure in found:
            print("R9 SQL CONTRACT GUARD: FAIL:", failure, file=sys.stderr)
        sys.exit(1)
    print("R9 SQL CONTRACT GUARD: PASS (structural + negative + concurrency gates)")
