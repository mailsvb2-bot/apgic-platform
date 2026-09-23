#!/usr/bin/env bash
set -euo pipefail

: "${PGHOST:=127.0.0.1}"
: "${PGPORT:=5432}"
: "${PGUSER:=apgic}"
: "${PGDATABASE:=apgic_ci}"
: "${RESTORE_DATABASE:=apgic_restore}"

evidence_dir="${GITHUB_WORKSPACE:-.}/evidence"
mkdir -p "$evidence_dir"

sentinel_identity="00000000-0000-0000-0000-00000000d001"
sentinel_org="00000000-0000-0000-0000-00000000d002"
sentinel_time="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
backup_file="$evidence_dir/apgic-ci.dump"

psql -v ON_ERROR_STOP=1 <<SQL
INSERT INTO identities (id, created_at)
VALUES ('$sentinel_identity', '$sentinel_time'::timestamptz);

INSERT INTO organizations (id, name, status, created_at, updated_at)
VALUES (
  '$sentinel_org',
  'Restore Drill Organization',
  'ACTIVE',
  '$sentinel_time'::timestamptz,
  '$sentinel_time'::timestamptz
);
SQL

backup_started_ms="$(date +%s%3N)"
pg_dump --format=custom --file="$backup_file" "$PGDATABASE"
backup_finished_ms="$(date +%s%3N)"

dropdb --if-exists "$RESTORE_DATABASE"
createdb "$RESTORE_DATABASE"

restore_started_ms="$(date +%s%3N)"
pg_restore --exit-on-error --no-owner --dbname="$RESTORE_DATABASE" "$backup_file"
restore_finished_ms="$(date +%s%3N)"

identity_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM identities WHERE id = '$sentinel_identity';"
)"
organization_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM organizations WHERE id = '$sentinel_org' AND status = 'ACTIVE';"
)"
audit_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'audit_records_append_only' AND NOT tgisinternal;"
)"
ledger_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'ledger_entries_append_only' AND NOT tgisinternal;"
)"
legal_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'legal_acceptances_append_only' AND NOT tgisinternal;"
)"
direction_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'organization_directions_no_delete' AND NOT tgisinternal;"
)"
product_owner_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'products_owner_exists' AND NOT tgisinternal;"
)"
outbox_delivery_constraint_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_constraint WHERE conname = 'outbox_delivery_evidence_check';"
)"
outbox_transition_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'outbox_events_transition_guard' AND NOT tgisinternal;"
)"
direction_archive_constraint_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_constraint WHERE conname = 'organization_directions_archive_timestamp_check';"
)"

if [[ "$identity_count" != "1" || "$organization_count" != "1" ]]; then
  echo "restore drill failed: business sentinel missing" >&2
  exit 1
fi

if [[ "$audit_trigger_count" != "1" || "$ledger_trigger_count" != "1" || "$legal_trigger_count" != "1" ]]; then
  echo "restore drill failed: append-only trigger missing" >&2
  exit 1
fi

if [[ "$direction_trigger_count" != "1" || "$product_owner_trigger_count" != "1" || "$outbox_delivery_constraint_count" != "1" || "$outbox_transition_trigger_count" != "1" || "$direction_archive_constraint_count" != "1" ]]; then
  echo "restore drill failed: semantic invariant object missing" >&2
  exit 1
fi

rto_ms=$((restore_finished_ms - restore_started_ms))
backup_ms=$((backup_finished_ms - backup_started_ms))

cat >"$evidence_dir/restore-drill.json" <<JSON
{
  "evidence_type": "CI_RESTORE_DRILL",
  "candidate_sha": "${APGIC_CANDIDATE_SHA:-${GITHUB_SHA:-unknown}}",
  "source_database": "$PGDATABASE",
  "target_database": "$RESTORE_DATABASE",
  "sentinel_created_at": "$sentinel_time",
  "backup_duration_ms": $backup_ms,
  "measured_restore_rto_ms": $rto_ms,
  "observed_data_loss_records": 0,
  "business_probe_identity_count": $identity_count,
  "business_probe_organization_count": $organization_count,
  "integrity_probe_audit_trigger_count": $audit_trigger_count,
  "integrity_probe_ledger_trigger_count": $ledger_trigger_count,
  "integrity_probe_legal_trigger_count": $legal_trigger_count,
  "integrity_probe_direction_trigger_count": $direction_trigger_count,
  "integrity_probe_product_owner_trigger_count": $product_owner_trigger_count,
  "integrity_probe_outbox_delivery_constraint_count": $outbox_delivery_constraint_count,
  "integrity_probe_outbox_transition_trigger_count": $outbox_transition_trigger_count,
  "integrity_probe_direction_archive_constraint_count": $direction_archive_constraint_count,
  "production_evidence": false
}
JSON

cat "$evidence_dir/restore-drill.json"
