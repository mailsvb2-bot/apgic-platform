#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/assert-authorized-host.sh"

ENV_FILE="${APGIC_ENV_FILE:-/etc/apgic/staging.env}"
if [[ -f "$ENV_FILE" ]]; then
  set -a
  source "$ENV_FILE"
  set +a
fi

: "${APGIC_HTTP_ADDR:=127.0.0.1:43111}"
: "${APGIC_WEB_ORIGIN:=http://127.0.0.1}"
: "${APGIC_PUBLIC_HOST:=apgic.ru}"
: "${APGIC_COMMIT_SHA:?APGIC_COMMIT_SHA is required}"

curl -fsS --max-time 5 "http://${APGIC_HTTP_ADDR}/healthz" | grep -q '"status":"ok"'
curl -fsS --max-time 5 "http://${APGIC_HTTP_ADDR}/readyz" | grep -q '"status":"ready"'

metrics="$(curl -fsS --max-time 5 "http://${APGIC_HTTP_ADDR}/metrics")"
grep -q 'apgic_http_requests_total' <<<"$metrics"
grep -q 'apgic_http_request_duration_milliseconds_bucket' <<<"$metrics"
grep -q 'apgic_readiness_checks_total{result="ready",reason="NONE"}' <<<"$metrics"
grep -q 'route="/readyz"' <<<"$metrics"

outbox_enabled="${APGIC_OUTBOX_WORKER_ENABLED:-0}"
if [[ "$outbox_enabled" == "1" ]]; then
    : "${APGIC_DATABASE_URL:?APGIC_DATABASE_URL is required when outbox worker is enabled}"
    if ! systemctl is-active --quiet apgic-outbox-worker-staging.service; then
        echo "APGIC staging runtime watchdog: outbox worker is not active" >&2
        exit 1
    fi
    max_pending_age="${APGIC_OUTBOX_MAX_PENDING_AGE_SECONDS:-300}"
    if [[ ! "$max_pending_age" =~ ^[1-9][0-9]*$ ]]; then
        echo "APGIC_OUTBOX_MAX_PENDING_AGE_SECONDS must be a positive integer" >&2
        exit 1
    fi
    stale_pending="$(psql "$APGIC_DATABASE_URL" -Atqc "SELECT count(*) FROM outbox_events WHERE delivery_status = 'PENDING' AND produced_at < now() - make_interval(secs => $max_pending_age::double precision)")"
    if [[ ! "$stale_pending" =~ ^[0-9]+$ ]]; then
        echo "APGIC staging runtime watchdog: could not read outbox backlog" >&2
        exit 1
    fi
    if (( stale_pending > 0 )); then
        echo "APGIC staging runtime watchdog: stale outbox backlog=$stale_pending max_age_seconds=$max_pending_age" >&2
        exit 1
    fi
elif [[ "$outbox_enabled" != "0" ]]; then
    echo "APGIC_OUTBOX_WORKER_ENABLED must be 0 or 1" >&2
    exit 1
fi

# R4 financial execution must not be called healthy just because the API
# responds. No PSP credentials are needed to prove the local worker, schema
# and its durable delivery path are actually running in staging.
finance_enabled="${APGIC_FINANCE_WORKER_ENABLED:-0}"
case "$finance_enabled" in
  0) ;;
  1)
    : "${APGIC_DATABASE_URL:?APGIC_DATABASE_URL is required when finance worker is enabled}"
    if [[ "$outbox_enabled" != "1" ]]; then
      echo "APGIC staging runtime watchdog: finance worker requires outbox delivery" >&2
      exit 1
    fi
    if ! systemctl is-active --quiet apgic-finance-worker-staging.service; then
      echo "APGIC staging runtime watchdog: finance worker is not active" >&2
      exit 1
    fi
    finance_schema="$(psql "$APGIC_DATABASE_URL" -Atqc "
      SELECT (
        to_regclass('public.finance_settlement_instructions') IS NOT NULL
        AND to_regclass('public.finance_provider_evidence') IS NOT NULL
        AND to_regclass('public.finance_payout_decisions') IS NOT NULL
        AND to_regclass('public.finance_reconciliation_runs') IS NOT NULL
        AND to_regclass('public.finance_reconciliation_results') IS NOT NULL
      )")" || {
      echo "APGIC staging runtime watchdog: cannot inspect financial schema" >&2
      exit 1
    }
    if [[ "$finance_schema" != "t" ]]; then
      echo "APGIC staging runtime watchdog: financial schema is incomplete" >&2
      exit 1
    fi
    # Read-only negative MFA probe: no browser cookie/factor can issue a
    # high-risk assertion. A 503 means the verifier/key is unavailable.
    mfa_http_status="$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' \\
      -H 'content-type: application/json' -H 'X-APGIC-MFA: totp-v1' \\
      --data '{"code":"000000"}' \\
      "http://${APGIC_HTTP_ADDR}/v1/finance/mfa/verify")"
    if [[ "$mfa_http_status" != "401" ]]; then
      echo "APGIC staging runtime watchdog: finance MFA challenge not enforcing signed sessions (HTTP $mfa_http_status)" >&2
      exit 1
    fi
    ;;
  *)
    echo "APGIC_FINANCE_WORKER_ENABLED must be 0 or 1" >&2
    exit 1
    ;;
esac

meta="$(curl -fsS --max-time 5 "http://${APGIC_HTTP_ADDR}/v1/meta")"
python3 - "$APGIC_COMMIT_SHA" "$meta" <<'PY'
import json
import sys

expected = sys.argv[1]
payload = json.loads(sys.argv[2])
actual = payload.get("commit_sha")
if actual != expected:
    raise SystemExit(f"runtime SHA mismatch: expected={expected} actual={actual}")
if payload.get("service") != "apgic-api":
    raise SystemExit(f"unexpected service metadata: {payload!r}")
PY

http_status="$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' -H "Host: ${APGIC_PUBLIC_HOST}" "${APGIC_WEB_ORIGIN}/")"
case "$http_status" in
    2??)
        ;;
    3??)
        curl -fsS --max-time 5 --resolve "${APGIC_PUBLIC_HOST}:443:127.0.0.1" "https://${APGIC_PUBLIC_HOST}/" >/dev/null
        ;;
    *)
        printf 'APGIC staging runtime watchdog: Nginx route failed status=%s host=%s\n' "$http_status" "$APGIC_PUBLIC_HOST" >&2
        exit 1
        ;;
esac

printf 'APGIC staging runtime watchdog: PASS sha=%s host=%s\n' "$APGIC_COMMIT_SHA" "$APGIC_PUBLIC_HOST"
