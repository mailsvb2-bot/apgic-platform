#!/usr/bin/env python3
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
BACKUP = ROOT / "deploy/staging/backup-staging-postgres.sh"
VERIFY = ROOT / "deploy/staging/verify-staging-backup.sh"
BACKUP_SERVICE = ROOT / "deploy/staging/apgic-staging-backup.service"
BACKUP_TIMER = ROOT / "deploy/staging/apgic-staging-backup.timer"
VERIFY_SERVICE = ROOT / "deploy/staging/apgic-staging-restore-verify.service"
VERIFY_TIMER = ROOT / "deploy/staging/apgic-staging-restore-verify.timer"


def validate() -> list[str]:
    errors: list[str] = []
    backup = BACKUP.read_text(encoding="utf-8")
    verify = VERIFY.read_text(encoding="utf-8")
    backup_service = BACKUP_SERVICE.read_text(encoding="utf-8")
    backup_timer = BACKUP_TIMER.read_text(encoding="utf-8")
    verify_service = VERIFY_SERVICE.read_text(encoding="utf-8")
    verify_timer = VERIFY_TIMER.read_text(encoding="utf-8")

    backup_required = (
        "umask 077",
        "pg_dump --format=custom",
        "pg_restore --list",
        ".dump.tmp",
        "chmod 0600",
        'mv "$tmp_file" "$final_file"',
        "APGIC_BACKUP_RETENTION_DAYS:=7",
        "retention_minutes=",
        "-mmin",
    )
    for item in backup_required:
        if item not in backup:
            errors.append(f"backup script missing invariant: {item}")

    verify_required = (
        "pg_restore --list",
        "apgic_restore_verify_",
        "trap cleanup EXIT",
        "createdb -O",
        "pg_restore",
        "--exit-on-error",
        "--no-owner",
        '--dbname="$verify_db"',
        "source_tables=",
        "restore_tables=",
        "identities",
        "audit_records",
        "ledger_entries",
        "booking_slots",
    )
    for item in verify_required:
        if item not in verify:
            errors.append(f"restore verification missing invariant: {item}")

    if '--dbname="$APGIC_BACKUP_DATABASE"' in verify:
        errors.append("restore verification must never restore into the live staging database")

    for name, text in (("backup service", backup_service), ("verify service", verify_service)):
        for item in (
            "User=postgres",
            "NoNewPrivileges=yes",
            "PrivateTmp=yes",
            "ProtectSystem=strict",
            "RestrictAddressFamilies=AF_UNIX",
        ):
            if item not in text:
                errors.append(f"{name} missing sandbox invariant: {item}")

    if "ReadWritePaths=/var/backups/apgic" not in backup_service:
        errors.append("backup service must restrict writes to /var/backups/apgic")
    if "OnCalendar=*-*-* 02:15:00 UTC" not in backup_timer or "Persistent=true" not in backup_timer:
        errors.append("daily backup timer schedule/persistence mismatch")
    if "OnCalendar=Sun *-*-* 03:30:00 UTC" not in verify_timer or "Persistent=true" not in verify_timer:
        errors.append("weekly restore verification timer schedule/persistence mismatch")

    combined = "\n".join(
        (backup, verify, backup_service, backup_timer, verify_service, verify_timer)
    ).lower()
    for forbidden in ("production_evidence", "185.215.4.49", "147.45.146.112", "metrotherapy"):
        if forbidden in combined:
            errors.append(
                f"operational staging backup contains forbidden coupling/claim: {forbidden}"
            )

    return errors


def main() -> int:
    errors = validate()
    if errors:
        print("STAGING BACKUP GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("STAGING BACKUP GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
