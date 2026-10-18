#!/usr/bin/env bash
# R0 foundation staging runtime verification. This is a READ-ONLY
# operational observation, never an Identity/MFA or production attestation.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/assert-authorized-host.sh"

ENV_FILE="${APGIC_ENV_FILE:-/etc/apgic/staging.env}"
if [[ ! -f "$ENV_FILE" ]]; then
  echo "R0 staging: approved environment file missing" >&2
  exit 1
fi
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a
: "${APGIC_COMMIT_SHA:?APGIC_COMMIT_SHA is required}"
: "${APGIC_DATABASE_URL:?APGIC_DATABASE_URL is required}"
[[ "$APGIC_COMMIT_SHA" =~ ^[0-9a-f]{40}$ ]] || {
  echo "R0 staging: invalid deployed candidate SHA" >&2
  exit 1
}
# Use the existing watchdog as the single owner of runtime/observability
# and server SHA checks, rather than implementing a parallel health model.
bash "$SCRIPT_DIR/check-staging-runtime.sh"

for unit in apgic-api-staging.service apgic-web-staging.service; do
  systemctl is-active --quiet "$unit" || {
    echo "R0 staging: inactive service $unit" >&2
    exit 1
  }
done
for timer in apgic-staging-backup.timer apgic-staging-restore-verify.timer; do
  systemctl is-active --quiet "$timer" || {
    echo "R0 staging: backup/restore timer not active $timer" >&2
    exit 1
  }
done

# Schema checks are read-only. These identify absent database guarantees,
# not proof that any particular human user journey has been exercised.
schema_checks="$(psql "$APGIC_DATABASE_URL" -X -A -t -v ON_ERROR_STOP=1 -c "
SELECT (
  to_regclass('public.identities') IS NOT NULL
  AND to_regclass('public.organizations') IS NOT NULL
  AND to_regclass('public.audit_records') IS NOT NULL
  AND to_regclass('public.outbox_events') IS NOT NULL
  AND to_regclass('public.ledger_entries') IS NOT NULL
)::text;
")"
[[ "$schema_checks" == "true" || "$schema_checks" == "t" ]] || {
  echo "R0 staging: canonical schema incomplete" >&2
  exit 1
}

# Latest timestamp from already-completed restore drill, no new database,
# mutation or backup side effect is performed by this read-only probe.
RESTORE_DIR="${APGIC_RESTORE_EVIDENCE_DIR:-/var/backups/apgic/evidence}"
[[ -d "$RESTORE_DIR" ]] || {
  echo "R0 staging: operational restore evidence directory missing" >&2
  exit 1
}
latest="$(find "$RESTORE_DIR" -maxdepth 1 -type f -name '*.json' -printf '%T@ %p\n' | sort -nr | head -n 1 | cut -d' ' -f2-)"
[[ -n "$latest" && -f "$latest" ]] || {
  echo "R0 staging: restore evidence absent" >&2
  exit 1
}
mtime="$(stat -c '%Y' "$latest")"
now="$(date +%s)"
age="$(( now - mtime ))"
# Old evidence may be useful historically, but cannot pass live R0 readiness.
if (( age < 0 || age > 8 * 86400 )); then
  echo "R0 staging: no fresh restore evidence (age_seconds=$age)" >&2
  exit 1
fi
# Data classification and full consent, tenant-isolation, MFA, signed-device
# installation, and rollback remain separate real-world release proofs.
printf 'R0_STAGING_READ_ONLY_PASS candidate_sha=%s restore_evidence_age_seconds=%s\n' "$APGIC_COMMIT_SHA" "$age"
