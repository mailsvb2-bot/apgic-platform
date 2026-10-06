#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
: "${APGIC_AUTH001_DATABASE_URL:=postgres://apgic:apgic@127.0.0.1:5432/apgic_ci?sslmode=disable}"
: "${APGIC_CANDIDATE_SHA:?APGIC_CANDIDATE_SHA is required}"
: "${APGIC_AUTH001_API_PORT:=43121}"
: "${APGIC_AUTH001_WEB_PORT:=43122}"

API_ORIGIN="http://127.0.0.1:${APGIC_AUTH001_API_PORT}"
WEB_ORIGIN="http://127.0.0.1:${APGIC_AUTH001_WEB_PORT}"
TMP="$(mktemp -d)"
API_PID=""
WEB_PID=""

cleanup() {
  if [[ -n "$WEB_PID" ]]; then kill "$WEB_PID" 2>/dev/null || true; fi
  if [[ -n "$API_PID" ]]; then kill "$API_PID" 2>/dev/null || true; fi
  if [[ -n "$WEB_PID" ]]; then wait "$WEB_PID" 2>/dev/null || true; fi
  if [[ -n "$API_PID" ]]; then wait "$API_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT INT TERM

remote_key="$(python3 - <<'PY'
import base64
print(base64.b64encode(b"r" * 32).decode())
PY
)"

cd "$ROOT/backend"
go build -o "$TMP/apgic-api" ./cmd/api

APGIC_ENVIRONMENT=STAGING APGIC_RELEASE_TRACK=R0 APGIC_HTTP_ADDR="127.0.0.1:${APGIC_AUTH001_API_PORT}" APGIC_DATABASE_URL="$APGIC_AUTH001_DATABASE_URL" APGIC_JURISDICTION_MATRIX_VERSION=jurisdiction-ci-v1 APGIC_RETENTION_POLICY_VERSION=retention-ci-v1 APGIC_SLO_POLICY_VERSION=slo-r0-ci-v1 APGIC_PROVIDER_MATRIX_VERSION=providers-ci-v1 APGIC_COMMIT_SHA="$APGIC_CANDIDATE_SHA" APGIC_CLIENT_SESSION_KEY=ci-only-client-session-key-000000000000 APGIC_MOBILE_POLICY_VERSION=mobile013-ci-v1 APGIC_MOBILE_CONTRACT_VERSION=0.10.0-r0-remote-config APGIC_MOBILE_SUPPORTED_CONTRACTS=0.8.0-r2-offline-sync,0.9.0-r0-mobile-compatibility,0.10.0-r0-remote-config APGIC_REMOTE_CONFIG_KEY_ID=mobile027-ci-v1 APGIC_REMOTE_CONFIG_PRIVATE_KEY_BASE64="$remote_key" APGIC_REMOTE_CONFIG_VERSION=1 APGIC_REMOTE_CONFIG_POLICY_ID=mobile027-ci-v1 APGIC_REMOTE_CONFIG_TTL_SECONDS=3600 APGIC_REMOTE_CONFIG_DISABLED_CAPABILITIES= APGIC_MOBILE_FORCED_UPDATE_REASON=INCOMPATIBLE_CRITICAL APGIC_IOS_MIN_VERSION=1.4.0 APGIC_IOS_RECOMMENDED_VERSION=1.6.0 APGIC_IOS_MIN_BUILD=1 APGIC_IOS_UPDATE_URL=https://apgic.ru/update APGIC_ANDROID_MIN_VERSION=1.4.0 APGIC_ANDROID_RECOMMENDED_VERSION=1.6.0 APGIC_ANDROID_MIN_BUILD=1 APGIC_ANDROID_UPDATE_URL=https://apgic.ru/update   "$TMP/apgic-api" >"$TMP/api.log" 2>&1 &
API_PID=$!

for _ in $(seq 1 40); do
  if curl -fsS "$API_ORIGIN/readyz" | grep -q '"status":"ready"'; then break; fi
  if ! kill -0 "$API_PID" 2>/dev/null; then
    cat "$TMP/api.log" >&2
    exit 1
  fi
  sleep 0.25
done
curl -fsS "$API_ORIGIN/readyz" | grep -q '"status":"ready"'

cd "$ROOT/apps/web"
APGIC_API_ORIGIN="$API_ORIGIN" NEXT_TELEMETRY_DISABLED=1   npm run start -- --hostname 127.0.0.1 --port "$APGIC_AUTH001_WEB_PORT" >"$TMP/web.log" 2>&1 &
WEB_PID=$!

for _ in $(seq 1 40); do
  if curl -fsS "$WEB_ORIGIN/" >/dev/null; then break; fi
  if ! kill -0 "$WEB_PID" 2>/dev/null; then
    cat "$TMP/web.log" >&2
    exit 1
  fi
  sleep 0.25
done
curl -fsS "$WEB_ORIGIN/" >/dev/null

create_org() {
  local name="$1"
  local prefix="$2"
  curl -sS -D "$TMP/${prefix}.headers" -o "$TMP/${prefix}.json"     -X POST "$WEB_ORIGIN/v1/organizations"     -H 'content-type: application/json'     --data "$(python3 - "$name" <<'PY'
import json, sys
print(json.dumps({"name": sys.argv[1]}, ensure_ascii=False))
PY
)"
  local status
  status="$(awk '/^HTTP\// {code=$2} END {print code}' "$TMP/${prefix}.headers")"
  [[ "$status" == "201" ]] || {
    cat "$TMP/${prefix}.json" >&2
    echo "organization create failed: status=$status" >&2
    exit 1
  }
}

create_org "Organization A private" org_a
create_org "TOP SECRET ORGANIZATION B" org_b

org_a="$(python3 - "$TMP/org_a.json" <<'PY'
import json, sys
print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])
PY
)"
org_b="$(python3 - "$TMP/org_b.json" <<'PY'
import json, sys
print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])
PY
)"
cookie_a="$(sed -n 's/^[Ss]et-[Cc]ookie: \(__Host-apgic_session=[^;]*\).*/\1/p' "$TMP/org_a.headers" | tr -d '\r' | tail -n1)"
[[ -n "$cookie_a" ]] || { echo "organization A did not receive trusted session cookie" >&2; exit 1; }

suffix="$(printf '%s' "$APGIC_CANDIDATE_SHA" | cut -c1-12)"
cross_corr="auth001-web-cross-${suffix}"
forged_corr="auth001-web-forged-${suffix}"
own_corr="auth001-web-own-${suffix}"

cross_status="$(curl -sS -o "$TMP/cross.json" -w '%{http_code}'   "$WEB_ORIGIN/v1/organizations/${org_b}/private-profile"   -H "Cookie: $cookie_a"   -H "X-Organization-Context: $org_a"   -H "X-Correlation-Id: $cross_corr")"
[[ "$cross_status" == "403" ]] || {
  cat "$TMP/cross.json" >&2
  echo "cross-tenant request status=$cross_status expected=403" >&2
  exit 1
}
python3 - "$TMP/cross.json" <<'PY'
import json, sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
if payload.get("code") != "AUTH_CROSS_TENANT_DENY":
    raise SystemExit(f"unexpected cross-tenant envelope: {payload!r}")
if "AUTH_CROSS_TENANT_DENY" not in (payload.get("policy_reason_codes") or []):
    raise SystemExit(f"cross-tenant reason missing: {payload!r}")
PY
if grep -q 'TOP SECRET ORGANIZATION B' "$TMP/cross.json"; then
  echo "cross-tenant response leaked private organization data" >&2
  exit 1
fi

own_status="$(curl -sS -o "$TMP/own.json" -w '%{http_code}'   "$WEB_ORIGIN/v1/organizations/${org_a}/private-profile"   -H "Cookie: $cookie_a"   -H "X-Organization-Context: $org_a"   -H "X-Correlation-Id: $own_corr")"
[[ "$own_status" == "200" ]] || {
  cat "$TMP/own.json" >&2
  echo "same-tenant request status=$own_status expected=200" >&2
  exit 1
}
python3 - "$TMP/own.json" "$org_a" <<'PY'
import json, sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
if payload.get("id") != sys.argv[2] or payload.get("name") != "Organization A private":
    raise SystemExit(f"unexpected same-tenant profile: {payload!r}")
PY

forged_status="$(curl -sS -o "$TMP/forged.json" -w '%{http_code}'   "$WEB_ORIGIN/v1/organizations/${org_b}/private-profile"   -H "Cookie: $cookie_a"   -H "X-Organization-Context: $org_b"   -H "X-Correlation-Id: $forged_corr")"
[[ "$forged_status" == "403" ]] || {
  cat "$TMP/forged.json" >&2
  echo "forged-context request status=$forged_status expected=403" >&2
  exit 1
}
python3 - "$TMP/forged.json" <<'PY'
import json, sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
if payload.get("code") != "AUTH_TENANT_CONTEXT_DENIED":
    raise SystemExit(f"unexpected forged-context envelope: {payload!r}")
PY
if grep -q 'TOP SECRET ORGANIZATION B' "$TMP/forged.json"; then
  echo "forged-context response leaked private organization data" >&2
  exit 1
fi

cross_audit="$(psql "$APGIC_AUTH001_DATABASE_URL" -Atqc   "SELECT reason || '|' || scope || '|' || resource_ref FROM audit_records WHERE correlation_id = '$cross_corr' ORDER BY occurred_at DESC LIMIT 1")"
[[ "$cross_audit" == "AUTH_CROSS_TENANT_DENY|${org_a}|tenant/${org_b}/resource/private-profile" ]] || {
  echo "cross-tenant audit mismatch: $cross_audit" >&2
  exit 1
}
forged_audit="$(psql "$APGIC_AUTH001_DATABASE_URL" -Atqc   "SELECT reason || '|' || scope FROM audit_records WHERE correlation_id = '$forged_corr' ORDER BY occurred_at DESC LIMIT 1")"
[[ "$forged_audit" == "AUTH_TENANT_CONTEXT_DENIED|${org_b}" ]] || {
  echo "forged-context audit mismatch: $forged_audit" >&2
  exit 1
}

mkdir -p "$ROOT/evidence"
observed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
cat >"$ROOT/evidence/auth001-web-tenant-isolation.json" <<JSON
{
  "schema_version": "auth001-web-tenant-isolation-v1",
  "candidate_sha": "$APGIC_CANDIDATE_SHA",
  "observed_at": "$observed_at",
  "surface": "WEB",
  "web_proxy_exercised": true,
  "postgres_persistence_exercised": true,
  "same_tenant_allowed": true,
  "cross_tenant_denied": true,
  "forged_tenant_context_denied": true,
  "private_resource_disclosed": false,
  "audit_evidence_persisted": true
}
JSON

echo "AUTH-001 WEB tenant isolation E2E: PASS"
