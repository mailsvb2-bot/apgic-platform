#!/usr/bin/env python3
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
MIGRATIONS = ROOT / "backend" / "migrations"
LEDGER_RUNNER = ROOT / "deploy" / "staging" / "apply-staging-migrations.sh"
LEGACY_TRANSACTIONAL_CUTOFF = 23


def fail(message: str) -> None:
    print(f"STAGING MIGRATION CONTRACT GUARD: FAIL\nERROR: {message}")
    raise SystemExit(1)


def main() -> int:
    runner = LEDGER_RUNNER.read_text(encoding="utf-8")
    for needle in (
        "apgic_schema_migrations",
        "checksum_sha256",
        "pg_advisory_lock",
        "pg_advisory_unlock",
        "baseline boolean",
        "BEGIN;",
        "COMMIT;",
        "ON CONFLICT (migration_name) DO NOTHING",
        "migration checksum mismatch",
    ):
        if needle not in runner:
            fail(f"ledger runner missing invariant: {needle}")

    pattern = re.compile(r"^(\d{6})_.+\.sql$")
    tx_control = re.compile(r"(?im)^\s*(BEGIN|COMMIT)\s*;")
    for path in sorted(MIGRATIONS.glob("[0-9][0-9][0-9][0-9][0-9][0-9]_*.sql")):
        match = pattern.match(path.name)
        if not match:
            continue
        number = int(match.group(1))
        if number <= LEGACY_TRANSACTIONAL_CUTOFF:
            continue
        text = path.read_text(encoding="utf-8")
        if tx_control.search(text):
            fail(
                f"{path.name} contains BEGIN/COMMIT; migrations after "
                f"{LEGACY_TRANSACTIONAL_CUTOFF:06d} must leave transaction control "
                "to apply-staging-migrations.sh so schema change + ledger write stay atomic"
            )

    print("STAGING MIGRATION CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
