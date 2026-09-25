#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
export APGIC_HTTP_ADDR="${APGIC_HTTP_ADDR:-:43131}"
export APGIC_API_ORIGIN="${APGIC_API_ORIGIN:-http://127.0.0.1:43131}"
cd "$ROOT/backend"
go run ./cmd/api > /tmp/apgic-api.log 2>&1 &
API_PID=$!
cleanup() { kill "$API_PID" 2>/dev/null || true; }
trap cleanup EXIT
for _ in $(seq 1 50); do
  if curl -sf "${APGIC_API_ORIGIN}/healthz" >/dev/null; then
    break
  fi
  sleep 0.2
done
curl -sf "${APGIC_API_ORIGIN}/healthz" >/dev/null
curl -sS -D - -o /dev/null "${APGIC_API_ORIGIN}/v1/slot-holds/missing/checkout-options?client_identity_id=x" | grep -qi 'content-type: application/json'
cd "$ROOT/apps/web"
exec npm run start -- --hostname 127.0.0.1 --port 43110
