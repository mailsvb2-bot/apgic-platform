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
