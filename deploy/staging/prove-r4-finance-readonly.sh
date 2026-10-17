#!/usr/bin/env bash
# Read-only, on-host R4 operational acceptance. Does not prove bank settlement.
# Produces no evidence on failure and does not expose credentials in output.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/assert-authorized-host.sh"
ENV_FILE="${APGIC_ENV_FILE:-/etc/apgic/staging.env}"
[[ -f "$ENV_FILE" ]] || { echo "R4 STAGING: missing approved environment" >&2; exit 1; }
set -a
source "$ENV_FILE"
set +a
: "${APGIC_DATABASE_URL:?}"
: "${APGIC_COMMIT_SHA:?}"
: "${APGIC_HTTP_ADDR:=127.0.0.1:43111}"
if [[ "${APGIC_FINANCE_WORKER_ENABLED:-0}" != "1" || "${APGIC_OUTBOX_WORKER_ENABLED:-0}" != "1" ]]; then
  echo "R4 STAGING: worker/outbox are not both enabled" >&2
  exit 1
fi
for unit in apgic-finance-worker-staging.service apgic-outbox-worker-staging.service; do
  systemctl is-active --quiet "$unit" || {
    echo "R4 STAGING: inactive service $unit" >&2
    exit 1
  }
done
bash "$SCRIPT_DIR/check-staging-runtime.sh"
# Count only real persisted ledger facts, not fixture-created rows. Counts
# intentionally cannot assert any provider-origin settlement provenance.
db_probe="$(psql "$APGIC_DATABASE_URL" -X -A -t -v ON_ERROR_STOP=1 -c "
  SELECT concat_ws(',',
    (SELECT count(*) FROM finance_settlement_instructions),
    (SELECT count(*) FROM finance_provider_evidence),
    (SELECT count(*) FROM finance_reconciliation_runs),
    (SELECT count(*) FROM finance_reconciliation_discrepancies),
    (SELECT count(*) FROM outbox_events WHERE delivery_status='PENDING'));
")"
[[ "$db_probe" =~ ^[0-9]+,[0-9]+,[0-9]+,[0-9]+,[0-9]+$ ]] || {
  echo "R4 STAGING: invalid database evidence" >&2
  exit 1
}
runtime_sha="$(curl -fsS --max-time 5 "http://${APGIC_HTTP_ADDR}/v1/meta" |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["commit_sha"])')"
[[ "$runtime_sha" == "$APGIC_COMMIT_SHA" ]] || {
  echo "R4 STAGING: candidate and runtime SHA differ" >&2
  exit 1
}
# Check that no public client can obtain a self-issued MFA assertion. Step-up
# is blocked until a trusted real IdentityProvider ceremony is integrated.
status="$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' \
  -X POST "http://${APGIC_HTTP_ADDR}/v1/auth/step-up/issue")"
if [[ "$status" != "404" && "$status" != "405" ]]; then
  echo "R4 STAGING: potentially public MFA issuance route ($status)" >&2
  exit 1
fi
IFS=, read -r instructions evidence reconciliations discrepancies pending <<<"$db_probe"
# This stdout is operational observation only. The separate reviewed
# production evidence index must still independently authenticate providers.
printf 'R4_STAGING_READ_ONLY_PASS sha=%s instructions=%s provider_evidence_rows=%s reconciliations=%s discrepancies=%s outbox_pending=%s mfa_self_issue_http=%s\n' \
  "$runtime_sha" "$instructions" "$evidence" "$reconciliations" "$discrepancies" "$pending" "$status"
