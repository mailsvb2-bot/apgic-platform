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

# Require the canonical completed restore drill, not merely a recently touched
# JSON file. The verifier validates measured RPO/RTO, database integrity
# and business probes and binds the actual observation to deployed SHA.
RESTORE_DIR="${APGIC_RESTORE_EVIDENCE_DIR:-/var/backups/apgic/evidence}"
restore_file="$RESTORE_DIR/latest.json"
[[ -f "$restore_file" ]] || {
  echo "R0 staging: canonical latest restore evidence is missing" >&2
  exit 1
}
python3 "$SCRIPT_DIR/../../tools/r0_restore_evidence_verify.py" \
  --evidence "$restore_file" \
  --candidate-sha "$APGIC_COMMIT_SHA" \
  --max-age-seconds 691200

# Data classification and full consent, tenant-isolation, MFA, signed-device
# installation, and rollback remain separate real-world release proofs.
printf 'R0_STAGING_READ_ONLY_PASS candidate_sha=%s measured_restore_proof=true\n' "$APGIC_COMMIT_SHA"
