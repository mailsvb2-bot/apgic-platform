#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/assert-authorized-host.sh"

: "${APGIC_BACKUP_DATABASE:=apgic_staging}"
: "${APGIC_BACKUP_DIR:=/var/backups/apgic}"
: "${APGIC_BACKUP_RETENTION_DAYS:=7}"

umask 077

if [[ ! -d "$APGIC_BACKUP_DIR" || ! -w "$APGIC_BACKUP_DIR" ]]; then
  echo "backup directory is missing or not writable: $APGIC_BACKUP_DIR" >&2
  exit 1
fi

if ! [[ "$APGIC_BACKUP_RETENTION_DAYS" =~ ^[0-9]+$ ]] || (( APGIC_BACKUP_RETENTION_DAYS < 1 )); then
  echo "APGIC_BACKUP_RETENTION_DAYS must be a positive integer" >&2
  exit 1
fi

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
tmp_file="$APGIC_BACKUP_DIR/.${APGIC_BACKUP_DATABASE}_${timestamp}.dump.tmp"
final_file="$APGIC_BACKUP_DIR/${APGIC_BACKUP_DATABASE}_${timestamp}.dump"

cleanup() {
  rm -f "$tmp_file"
}
trap cleanup EXIT

pg_dump --format=custom --file="$tmp_file" "$APGIC_BACKUP_DATABASE"
pg_restore --list "$tmp_file" >/dev/null
chmod 0600 "$tmp_file"
mv "$tmp_file" "$final_file"

retention_minutes=$((APGIC_BACKUP_RETENTION_DAYS * 1440))
find "$APGIC_BACKUP_DIR" -maxdepth 1 -type f   -name "${APGIC_BACKUP_DATABASE}_*.dump"   -mmin "+${retention_minutes}"   -delete

trap - EXIT
printf 'APGIC staging backup: PASS %s\n' "$final_file"
