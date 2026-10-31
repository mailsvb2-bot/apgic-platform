#!/usr/bin/env bash
# Read-only APGIC event gateway activation preflight.
# DOES NOT start workers, send events, mutate PostgreSQL or print credentials.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/assert-authorized-host.sh"
ENV_FILE="${APGIC_ENV_FILE:-/etc/apgic/staging.env}"
[[ -f "$ENV_FILE" ]] || { echo "BLOCKED: staging environment missing"; exit 1; }
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a
: "${APGIC_DATABASE_URL:?}"
: "${APGIC_COMMIT_SHA:?}"
blocked=0
require() { if [[ -z "${!1:-}" ]]; then echo "BLOCKED: $1 missing"; blocked=1; else echo "$1: configured"; fi; }
echo "=== R4 gateway preflight (no delivery) ==="
echo "deployed sha: $APGIC_COMMIT_SHA"
require APGIC_EVENT_GATEWAY_URL
require APGIC_EVENT_GATEWAY_PRINCIPAL_ID
require APGIC_EVENT_GATEWAY_CREDENTIAL
if [[ -n "${APGIC_EVENT_GATEWAY_URL:-}" ]]; then
  python3 - <<'PY' || blocked=1
import os, urllib.parse
url = urllib.parse.urlsplit(os.environ["APGIC_EVENT_GATEWAY_URL"])
if url.scheme != "https" or not url.hostname or url.username or url.password or url.fragment:
    raise SystemExit("BLOCKED: event gateway URL must be a credential-free HTTPS endpoint")
print("gateway URL syntax: PASS (no network request performed)")
PY
fi
if [[ -n "${APGIC_EVENT_GATEWAY_CREDENTIAL:-}" && ${#APGIC_EVENT_GATEWAY_CREDENTIAL} -lt 32 ]]; then
  echo "BLOCKED: event gateway credential shorter than 32 characters"
  blocked=1
fi
echo "=== Pending event inventory (no payloads) ==="
psql "$APGIC_DATABASE_URL" -X -A -t -v ON_ERROR_STOP=1 -c "
SELECT event_type, delivery_status, count(*), min(produced_at), max(attempts)
FROM outbox_events
GROUP BY event_type, delivery_status ORDER BY event_type, delivery_status;"
echo "=== Worker states ==="
for unit in apgic-outbox-worker-staging.service apgic-finance-worker-staging.service; do
  printf '%s: ' "$unit"
  systemctl is-active "$unit" 2>/dev/null || true
done
if [[ "$blocked" -ne 0 ]]; then
  echo "BLOCKED: gateway configuration is incomplete; leave worker flags disabled"
  exit 1
fi
echo "CONFIGURATION_ONLY_PASS: NOT authorization to start workers"
echo "REQUIRES: independently verified receiving principal and scope, durable idempotent acceptance,"
echo "correct event-type consumer, authorization, signed/traceable delivery evidence, replay/restart proof."
