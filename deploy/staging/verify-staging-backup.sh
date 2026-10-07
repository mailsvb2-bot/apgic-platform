#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/assert-authorized-host.sh"

: "${APGIC_BACKUP_DATABASE:=apgic_staging}"
: "${APGIC_BACKUP_ROLE:=apgic_staging}"
: "${APGIC_BACKUP_DIR:=/var/backups/apgic}"
: "${APGIC_RESTORE_EVIDENCE_DIR:=$APGIC_BACKUP_DIR/evidence}"

latest="$(
  find "$APGIC_BACKUP_DIR" -maxdepth 1 -type f     -name "${APGIC_BACKUP_DATABASE}_*.dump"     -printf '%T@ %p\n' |
    sort -nr |
    head -n 1 |
    cut -d' ' -f2-
)"

if [[ -z "$latest" || ! -f "$latest" ]]; then
  echo "no APGIC staging backup found" >&2
  exit 1
fi

mode="$(stat -c '%a' "$latest")"
if [[ "$mode" != "600" ]]; then
  echo "backup file mode must be 600, got $mode" >&2
  exit 1
fi

pg_restore --list "$latest" >/dev/null

observed_epoch="$(date +%s)"
backup_epoch="$(stat -c '%Y' "$latest")"
measured_backup_rpo_seconds="$(( observed_epoch - backup_epoch ))"
if (( measured_backup_rpo_seconds < 0 )); then
  echo "backup mtime is in the future" >&2
  exit 1
fi

verify_db="apgic_restore_verify_$(date -u +%Y%m%d%H%M%S)_$$"
cleanup() {
  dropdb --if-exists "$verify_db" >/dev/null 2>&1 || true
}
trap cleanup EXIT

dropdb --if-exists "$verify_db"
createdb -O "$APGIC_BACKUP_ROLE" "$verify_db"
restore_started_ms="$(date +%s%3N)"
pg_restore \
  --exit-on-error \
  --no-owner \
  --role="$APGIC_BACKUP_ROLE" \
  --dbname="$verify_db" \
  "$latest"
restore_finished_ms="$(date +%s%3N)"
measured_restore_rto_ms="$(( restore_finished_ms - restore_started_ms ))"

source_tables="$(psql --dbname="$APGIC_BACKUP_DATABASE" -Atqc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'")"
restore_tables="$(psql --dbname="$verify_db" -Atqc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'")"

if [[ "$source_tables" != "$restore_tables" ]]; then
  echo "restore verification failed: source tables=$source_tables restored tables=$restore_tables" >&2
  exit 1
fi

business_probe_identity_source_count="$(psql --dbname="$APGIC_BACKUP_DATABASE" -Atqc "SELECT count(*) FROM identities")"
business_probe_identity_restored_count="$(psql --dbname="$verify_db" -Atqc "SELECT count(*) FROM identities")"
business_probe_organization_source_count="$(psql --dbname="$APGIC_BACKUP_DATABASE" -Atqc "SELECT count(*) FROM organizations")"
business_probe_organization_restored_count="$(psql --dbname="$verify_db" -Atqc "SELECT count(*) FROM organizations")"

if [[ "$business_probe_identity_source_count" -lt 1 ||
      "$business_probe_organization_source_count" -lt 1 ||
      "$business_probe_identity_source_count" != "$business_probe_identity_restored_count" ||
      "$business_probe_organization_source_count" != "$business_probe_organization_restored_count" ]]; then
  echo "restore verification failed: business probes mismatch identities=${business_probe_identity_source_count}/${business_probe_identity_restored_count} organizations=${business_probe_organization_source_count}/${business_probe_organization_restored_count}" >&2
  exit 1
fi

integrity_probe_audit_trigger_count="$(psql --dbname="$verify_db" -Atqc "SELECT count(*) FROM pg_trigger WHERE tgname='audit_records_append_only' AND NOT tgisinternal")"
integrity_probe_ledger_trigger_count="$(psql --dbname="$verify_db" -Atqc "SELECT count(*) FROM pg_trigger WHERE tgname='ledger_entries_append_only' AND NOT tgisinternal")"
integrity_probe_direction_trigger_count="$(psql --dbname="$verify_db" -Atqc "SELECT count(*) FROM pg_trigger WHERE tgname='organization_directions_no_delete' AND NOT tgisinternal")"
integrity_probe_product_owner_trigger_count="$(psql --dbname="$verify_db" -Atqc "SELECT count(*) FROM pg_trigger WHERE tgname='products_owner_exists' AND NOT tgisinternal")"
integrity_probe_booking_transition_trigger_count="$(psql --dbname="$verify_db" -Atqc "SELECT count(*) FROM pg_trigger WHERE tgname='bookings_transition_guard' AND NOT tgisinternal")"
integrity_probe_orders_append_only_trigger_count="$(psql --dbname="$verify_db" -Atqc "SELECT count(*) FROM pg_trigger WHERE tgname='orders_append_only' AND NOT tgisinternal")"

if [[ "$integrity_probe_audit_trigger_count" != "1" ||
      "$integrity_probe_ledger_trigger_count" != "1" ||
      "$integrity_probe_direction_trigger_count" != "1" ||
      "$integrity_probe_product_owner_trigger_count" != "1" ||
      "$integrity_probe_booking_transition_trigger_count" != "1" ||
      "$integrity_probe_orders_append_only_trigger_count" != "1" ]]; then
  echo "restore verification failed: integrity probe missing from restored database" >&2
  exit 1
fi

required_tables=(
  identities
  outbox_events
  audit_records
  ledger_entries
  booking_slots
  booking_holds
  bookings
)

for table in "${required_tables[@]}"; do
  exists="$(psql --dbname="$verify_db" -Atqc "SELECT to_regclass('public.$table') IS NOT NULL")"
  if [[ "$exists" != "t" ]]; then
    echo "restore verification failed: missing table $table" >&2
    exit 1
  fi
done

repo_root="$(cd "$SCRIPT_DIR/../.." && pwd -P)"
candidate_sha="$(git -c safe.directory="$repo_root" -C "$repo_root" rev-parse HEAD)"
observed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
backup_sha256="$(sha256sum "$latest" | awk '{print $1}')"
mkdir -p "$APGIC_RESTORE_EVIDENCE_DIR"
chmod 0700 "$APGIC_RESTORE_EVIDENCE_DIR"
evidence_tmp="$APGIC_RESTORE_EVIDENCE_DIR/latest.json.tmp"
evidence_file="$APGIC_RESTORE_EVIDENCE_DIR/latest.json"
cat >"$evidence_tmp" <<JSON
{
  "schema_version": "staging-restore-evidence-v2",
  "evidence_type": "STAGING_RESTORE_DRILL",
  "candidate_sha": "$candidate_sha",
  "observed_at": "$observed_at",
  "backup_file_sha256": "$backup_sha256",
  "measured_backup_rpo_seconds": $measured_backup_rpo_seconds,
  "measured_restore_rto_ms": $measured_restore_rto_ms,
  "source_table_count": $source_tables,
  "restored_table_count": $restore_tables,
  "required_table_count": ${#required_tables[@]},
  "business_probe_identity_source_count": $business_probe_identity_source_count,
  "business_probe_identity_restored_count": $business_probe_identity_restored_count,
  "business_probe_organization_source_count": $business_probe_organization_source_count,
  "business_probe_organization_restored_count": $business_probe_organization_restored_count,
  "integrity_probe_audit_trigger_count": $integrity_probe_audit_trigger_count,
  "integrity_probe_ledger_trigger_count": $integrity_probe_ledger_trigger_count,
  "integrity_probe_direction_trigger_count": $integrity_probe_direction_trigger_count,
  "integrity_probe_product_owner_trigger_count": $integrity_probe_product_owner_trigger_count,
  "integrity_probe_booking_transition_trigger_count": $integrity_probe_booking_transition_trigger_count,
  "integrity_probe_orders_append_only_trigger_count": $integrity_probe_orders_append_only_trigger_count,
  "business_probes_passed": true,
  "integrity_probes_passed": true,
  "production_evidence": false
}
JSON
chmod 0600 "$evidence_tmp"
mv "$evidence_tmp" "$evidence_file"

printf 'APGIC staging restore verification: PASS backup=%s tables=%s rpo_s=%s rto_ms=%s evidence=%s\n' \
  "$latest" "$restore_tables" "$measured_backup_rpo_seconds" "$measured_restore_rto_ms" "$evidence_file"
