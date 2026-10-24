#!/usr/bin/env bash
# Staging-only operational proof. Never creates synthetic events or starts finance.
# Run after a real, authorized booking operation that produces booking.ledger_committed.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
bash "$ROOT/deploy/staging/assert-authorized-host.sh"
ENV_FILE="${APGIC_ENV_FILE:-/etc/apgic/staging.env}"
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a
: "${APGIC_DATABASE_URL:?}"
[[ "${APGIC_OUTBOX_WORKER_ENABLED:-0}" == "1" ]] || { echo "BLOCKED: outbox must be enabled"; exit 1; }
[[ "${APGIC_FINANCE_WORKER_ENABLED:-0}" == "0" ]] || { echo "BLOCKED: finance must be disabled"; exit 1; }
[[ "${APGIC_EVENT_GATEWAY_DIAL_ADDR:-}" == "127.0.0.1:443" ]] || { echo "BLOCKED: missing pinned TLS loopback dial"; exit 1; }
[[ "${APGIC_EVENT_GATEWAY_URL:-}" == "https://apgic.ru/_internal/apgic-event-gateway" ]] || { echo "BLOCKED: wrong gateway URL"; exit 1; }
systemctl is-enabled --quiet apgic-outbox-worker-staging.service
systemctl is-active --quiet apgic-outbox-worker-staging.service
systemctl is-active --quiet apgic-event-gateway-staging.service
! systemctl is-active --quiet apgic-finance-worker-staging.service
bash "$ROOT/deploy/staging/check-staging-runtime.sh"
query() { psql "$APGIC_DATABASE_URL" -XAt -v ON_ERROR_STOP=1 -c "$1"; }
echo "=== VERIFY EXISTING DURABLE RECEIPTS ==="
query "SELECT o.event_id::text || ' ' || o.delivery_status || ' receipts=' || count(r.event_id)
FROM outbox_events o LEFT JOIN event_gateway_receipts r ON r.event_id=o.event_id
WHERE o.event_type='booking.ledger_committed'
GROUP BY o.event_id,o.delivery_status ORDER BY o.event_id;"
BAD="$(query "SELECT count(*) FROM outbox_events o
LEFT JOIN event_gateway_receipts r ON r.event_id=o.event_id
WHERE o.delivery_status='DELIVERED' AND o.event_type='booking.ledger_committed'
AND (r.event_id IS NULL OR o.delivered_at IS NULL);")"
[[ "$BAD" == "0" ]] || { echo "BLOCKED: delivered event without receipt"; exit 1; }
echo "=== RESTART SERVICE PATH ==="
systemctl restart apgic-event-gateway-staging.service
systemctl restart apgic-outbox-worker-staging.service
for n in $(seq 1 20); do
  if systemctl is-active --quiet apgic-event-gateway-staging.service &&
     systemctl is-active --quiet apgic-outbox-worker-staging.service; then break; fi
  sleep 1
done
systemctl is-active --quiet apgic-event-gateway-staging.service
systemctl is-active --quiet apgic-outbox-worker-staging.service
! systemctl is-active --quiet apgic-finance-worker-staging.service
echo "SERVICE_RESTART_PASS: gateway and outbox restarted; finance inactive"
echo "=== WAIT FOR NEW CANONICAL BOOKING EVENT ==="
# The after-restart marker is DB server time, avoiding cross-host time skew.
MARKER="$(query 'SELECT clock_timestamp()::text;')"
export MARKER
echo "Create a REAL authorized booking-ledger event now; no fixture is generated."
for i in $(seq 1 60); do
  FOUND="$(psql "$APGIC_DATABASE_URL" -XAt -v ON_ERROR_STOP=1 -v marker="$MARKER" -c "
SELECT count(*) FROM outbox_events o
JOIN event_gateway_receipts r ON r.event_id=o.event_id
WHERE o.event_type='booking.ledger_committed'
AND o.produced_at > :'marker'::timestamptz
AND o.delivery_status='DELIVERED' AND o.delivered_at IS NOT NULL
AND o.attempts >= 1;")"
  if [[ "$FOUND" =~ ^[0-9]+$ ]] && (( FOUND > 0 )); then
    echo "FRESH_EVENT_DELIVERY_PASS: $FOUND canonical new event(s) delivered after restart"
    exit 0
  fi
  sleep 2
done
echo "NOT_PROVEN: no new canonically generated booking event was delivered after restart"
exit 2
