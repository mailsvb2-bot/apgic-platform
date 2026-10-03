#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/assert-authorized-host.sh"

: "${APGIC_BACKUP_DATABASE:=apgic_staging}"
: "${APGIC_BACKUP_ROLE:=apgic_staging}"
: "${APGIC_BACKUP_DIR:=/var/backups/apgic}"

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

verify_db="apgic_restore_verify_$(date -u +%Y%m%d%H%M%S)_$$"
cleanup() {
  dropdb --if-exists "$verify_db" >/dev/null 2>&1 || true
}
trap cleanup EXIT

dropdb --if-exists "$verify_db"
createdb -O "$APGIC_BACKUP_ROLE" "$verify_db"
pg_restore \
  --exit-on-error \
  --no-owner \
  --role="$APGIC_BACKUP_ROLE" \
  --dbname="$verify_db" \
  "$latest"

source_tables="$(psql --dbname="$APGIC_BACKUP_DATABASE" -Atqc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'")"
restore_tables="$(psql --dbname="$verify_db" -Atqc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'")"

if [[ "$source_tables" != "$restore_tables" ]]; then
  echo "restore verification failed: source tables=$source_tables restored tables=$restore_tables" >&2
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

printf 'APGIC staging restore verification: PASS backup=%s tables=%s\n' "$latest" "$restore_tables"
