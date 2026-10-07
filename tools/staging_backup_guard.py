#!/usr/bin/env python3
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
BACKUP = ROOT / "deploy/staging/backup-staging-postgres.sh"
VERIFY = ROOT / "deploy/staging/verify-staging-backup.sh"
BACKUP_SERVICE = ROOT / "deploy/staging/apgic-staging-backup.service"
BACKUP_TIMER = ROOT / "deploy/staging/apgic-staging-backup.timer"
VERIFY_SERVICE = ROOT / "deploy/staging/apgic-staging-restore-verify.service"
VERIFY_TIMER = ROOT / "deploy/staging/apgic-staging-restore-verify.timer"
STAGING_RESTORE_SCHEMA = ROOT / "contracts/jsonschema/staging-restore-evidence-v2.schema.json"


def validate() -> list[str]:
    errors: list[str] = []
    backup = BACKUP.read_text(encoding="utf-8")
    bootstrap = (ROOT / "deploy/staging/bootstrap-dedicated-host.sh").read_text(encoding="utf-8")
    verify = VERIFY.read_text(encoding="utf-8")
    backup_service = BACKUP_SERVICE.read_text(encoding="utf-8")
    backup_timer = BACKUP_TIMER.read_text(encoding="utf-8")
    verify_service = VERIFY_SERVICE.read_text(encoding="utf-8")
    verify_timer = VERIFY_TIMER.read_text(encoding="utf-8")
    staging_restore_schema = json.loads(STAGING_RESTORE_SCHEMA.read_text(encoding="utf-8"))

    if "\\n" in bootstrap:
        errors.append("dedicated host bootstrap must not contain literal \\n escape sequences")

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
        "measured_backup_rpo_seconds",
        "measured_restore_rto_ms",
        "backup_file_sha256",
        "candidate_sha=",
        'repo_root="$(cd "$SCRIPT_DIR/../.." && pwd -P)"',
        "staging-restore-evidence-v2",
        "STAGING_RESTORE_DRILL",
        "business_probe_identity_source_count",
        "business_probe_identity_restored_count",
        "business_probe_organization_source_count",
        "business_probe_organization_restored_count",
        "integrity_probe_audit_trigger_count",
        "integrity_probe_ledger_trigger_count",
        "integrity_probe_direction_trigger_count",
        "integrity_probe_product_owner_trigger_count",
        "integrity_probe_booking_transition_trigger_count",
        "integrity_probe_orders_append_only_trigger_count",
        "business_probes_passed",
        "integrity_probes_passed",
        "production_evidence",
    )
    for item in verify_required:
        if item not in verify:
            errors.append(f"restore verification missing invariant: {item}")

    apply_blocks: list[str] = []
    verify_lines = verify.splitlines()
    for index, line in enumerate(verify_lines):
        if line.strip().startswith("pg_restore") and line.rstrip().endswith("\\"):
            block_lines = [line]
            for next_line in verify_lines[index + 1 : index + 12]:
                block_lines.append(next_line)
                if not next_line.rstrip().endswith("\\"):
                    break
            apply_blocks.append("\n".join(block_lines))

    if len(apply_blocks) != 1:
        errors.append(f"expected exactly one pg_restore apply block, got {len(apply_blocks)}")
    else:
        apply_block = apply_blocks[0]
        if '--dbname="$verify_db"' not in apply_block:
            errors.append("restore verification must target the temporary verify database")
        if "APGIC_BACKUP_DATABASE" in apply_block:
            errors.append("restore verification must never restore into the live staging database")

    for name, text in (("backup service", backup_service), ("verify service", verify_service)):
        for item in (
            "User=postgres",
            "NoNewPrivileges=yes",
            "PrivateTmp=yes",
            "ProtectSystem=strict",
            "RestrictAddressFamilies=AF_UNIX AF_NETLINK",
        ):
            if item not in text:
                errors.append(f"{name} missing sandbox invariant: {item}")

    if "ReadWritePaths=/var/backups/apgic" not in backup_service:
        errors.append("backup service must restrict writes to /var/backups/apgic")
    if "ExecStart=/usr/bin/bash /opt/apgic/current/deploy/staging/backup-staging-postgres.sh" not in backup_service:
        errors.append("backup service must invoke non-executable repository script via /usr/bin/bash")
    if "ExecStart=/usr/bin/bash /opt/apgic/current/deploy/staging/verify-staging-backup.sh" not in verify_service:
        errors.append("restore verification service must invoke non-executable repository script via /usr/bin/bash")
    if "ReadWritePaths=/var/backups/apgic" not in verify_service:
        errors.append("restore verification service must restrict evidence writes to /var/backups/apgic")
    if "OnCalendar=*-*-* 02:15:00 UTC" not in backup_timer or "Persistent=true" not in backup_timer:
        errors.append("daily backup timer schedule/persistence mismatch")
    if "OnCalendar=Sun *-*-* 03:30:00 UTC" not in verify_timer or "Persistent=true" not in verify_timer:
        errors.append("weekly restore verification timer schedule/persistence mismatch")

    properties = staging_restore_schema.get("properties") or {}
    required = set(staging_restore_schema.get("required") or [])
    expected_required = {
        "schema_version",
        "evidence_type",
        "candidate_sha",
        "observed_at",
        "backup_file_sha256",
        "measured_backup_rpo_seconds",
        "measured_restore_rto_ms",
        "source_table_count",
        "restored_table_count",
        "required_table_count",
        "business_probe_identity_source_count",
        "business_probe_identity_restored_count",
        "business_probe_organization_source_count",
        "business_probe_organization_restored_count",
        "integrity_probe_audit_trigger_count",
        "integrity_probe_ledger_trigger_count",
        "integrity_probe_direction_trigger_count",
        "integrity_probe_product_owner_trigger_count",
        "integrity_probe_booking_transition_trigger_count",
        "integrity_probe_orders_append_only_trigger_count",
        "business_probes_passed",
        "integrity_probes_passed",
        "production_evidence",
    }
    if required != expected_required:
        errors.append("staging restore evidence schema required fields mismatch")
    if properties.get("schema_version", {}).get("const") != "staging-restore-evidence-v2":
        errors.append("staging restore evidence schema version mismatch")
    if properties.get("evidence_type", {}).get("const") != "STAGING_RESTORE_DRILL":
        errors.append("staging restore evidence type mismatch")
    if properties.get("production_evidence", {}).get("const") is not False:
        errors.append("staging restore evidence must explicitly be non-production")
    if properties.get("business_probes_passed", {}).get("const") is not True:
        errors.append("staging restore evidence must require passing business probes")
    if properties.get("integrity_probes_passed", {}).get("const") is not True:
        errors.append("staging restore evidence must require passing integrity probes")
    for field in (
        "integrity_probe_audit_trigger_count",
        "integrity_probe_ledger_trigger_count",
        "integrity_probe_direction_trigger_count",
        "integrity_probe_product_owner_trigger_count",
        "integrity_probe_booking_transition_trigger_count",
        "integrity_probe_orders_append_only_trigger_count",
    ):
        if properties.get(field, {}).get("const") != 1:
            errors.append(f"{field}: exact restored invariant proof required")
    for field in ("measured_backup_rpo_seconds", "measured_restore_rto_ms"):
        if properties.get(field, {}).get("minimum") != 0:
            errors.append(f"{field}: non-negative measurement contract required")

    combined = "\n".join(
        (backup, verify, backup_service, backup_timer, verify_service, verify_timer)
    ).lower()
    if '"production_evidence": true' in verify.lower():
        errors.append("staging restore evidence must never claim production evidence")
    for forbidden in ("185.215.4.49", "147.45.146.112", "metrotherapy"):
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
