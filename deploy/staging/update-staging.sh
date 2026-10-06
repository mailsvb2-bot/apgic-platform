#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/assert-authorized-host.sh"

REPO_ROOT="${APGIC_REPO_ROOT:-/opt/apgic/current}"
ENV_FILE="${APGIC_ENV_FILE:-/etc/apgic/staging.env}"
TARGET_REF="${1:-origin/main}"
DEPLOY_LOCK_FILE="${APGIC_DEPLOY_LOCK_FILE:-/run/lock/apgic-staging-update.lock}"

mkdir -p "$(dirname "$DEPLOY_LOCK_FILE")"
exec 9>"$DEPLOY_LOCK_FILE"
if ! flock -n 9; then
  echo "another APGIC staging deployment is already running: $DEPLOY_LOCK_FILE" >&2
  exit 1
fi

cd "$REPO_ROOT"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing APGIC environment file: $ENV_FILE" >&2
  exit 1
fi

set -a
source "$ENV_FILE"
set +a

: "${APGIC_DATABASE_URL:?APGIC_DATABASE_URL is required}"
: "${APGIC_COMMIT_SHA:?APGIC_COMMIT_SHA is required}"
: "${APGIC_MOBILE_POLICY_VERSION:?APGIC_MOBILE_POLICY_VERSION is required}"
: "${APGIC_MOBILE_CONTRACT_VERSION:?APGIC_MOBILE_CONTRACT_VERSION is required}"
: "${APGIC_MOBILE_SUPPORTED_CONTRACTS:?APGIC_MOBILE_SUPPORTED_CONTRACTS is required}"
: "${APGIC_MOBILE_FORCED_UPDATE_REASON:?APGIC_MOBILE_FORCED_UPDATE_REASON is required}"
: "${APGIC_IOS_MIN_VERSION:?APGIC_IOS_MIN_VERSION is required}"
: "${APGIC_IOS_RECOMMENDED_VERSION:?APGIC_IOS_RECOMMENDED_VERSION is required}"
: "${APGIC_IOS_MIN_BUILD:?APGIC_IOS_MIN_BUILD is required}"
: "${APGIC_IOS_UPDATE_URL:?APGIC_IOS_UPDATE_URL is required}"
: "${APGIC_ANDROID_MIN_VERSION:?APGIC_ANDROID_MIN_VERSION is required}"
: "${APGIC_ANDROID_RECOMMENDED_VERSION:?APGIC_ANDROID_RECOMMENDED_VERSION is required}"
: "${APGIC_ANDROID_MIN_BUILD:?APGIC_ANDROID_MIN_BUILD is required}"
: "${APGIC_ANDROID_UPDATE_URL:?APGIC_ANDROID_UPDATE_URL is required}"
: "${APGIC_REMOTE_CONFIG_KEY_ID:?APGIC_REMOTE_CONFIG_KEY_ID is required}"
: "${APGIC_REMOTE_CONFIG_PRIVATE_KEY_BASE64:?APGIC_REMOTE_CONFIG_PRIVATE_KEY_BASE64 is required}"
: "${APGIC_REMOTE_CONFIG_VERSION:?APGIC_REMOTE_CONFIG_VERSION is required}"
: "${APGIC_REMOTE_CONFIG_POLICY_ID:?APGIC_REMOTE_CONFIG_POLICY_ID is required}"
: "${APGIC_REMOTE_CONFIG_TTL_SECONDS:?APGIC_REMOTE_CONFIG_TTL_SECONDS is required}"

validate_semver() {
  [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
}
validate_positive_integer() {
  [[ "$1" =~ ^[1-9][0-9]*$ ]]
}
validate_https_url() {
  [[ "$1" =~ ^https://[^[:space:]]+$ ]]
}

case "$APGIC_MOBILE_FORCED_UPDATE_REASON" in
  SECURITY_CRITICAL|LEGAL_CRITICAL|INCOMPATIBLE_CRITICAL) ;;
  *)
    echo "invalid APGIC_MOBILE_FORCED_UPDATE_REASON: $APGIC_MOBILE_FORCED_UPDATE_REASON" >&2
    exit 1
    ;;
esac

for value in "$APGIC_IOS_MIN_VERSION" "$APGIC_IOS_RECOMMENDED_VERSION" "$APGIC_ANDROID_MIN_VERSION" "$APGIC_ANDROID_RECOMMENDED_VERSION"; do
  validate_semver "$value" || { echo "invalid mobile semantic version: $value" >&2; exit 1; }
done
validate_positive_integer "$APGIC_IOS_MIN_BUILD" || { echo "invalid APGIC_IOS_MIN_BUILD" >&2; exit 1; }
validate_positive_integer "$APGIC_ANDROID_MIN_BUILD" || { echo "invalid APGIC_ANDROID_MIN_BUILD" >&2; exit 1; }
validate_https_url "$APGIC_IOS_UPDATE_URL" || { echo "invalid APGIC_IOS_UPDATE_URL" >&2; exit 1; }
validate_https_url "$APGIC_ANDROID_UPDATE_URL" || { echo "invalid APGIC_ANDROID_UPDATE_URL" >&2; exit 1; }
validate_positive_integer "$APGIC_REMOTE_CONFIG_VERSION" || { echo "invalid APGIC_REMOTE_CONFIG_VERSION" >&2; exit 1; }
validate_positive_integer "$APGIC_REMOTE_CONFIG_TTL_SECONDS" || { echo "invalid APGIC_REMOTE_CONFIG_TTL_SECONDS" >&2; exit 1; }
if (( APGIC_REMOTE_CONFIG_TTL_SECONDS < 60 || APGIC_REMOTE_CONFIG_TTL_SECONDS > 604800 )); then
  echo "APGIC_REMOTE_CONFIG_TTL_SECONDS must be between 60 and 604800" >&2
  exit 1
fi
python3 - "$APGIC_REMOTE_CONFIG_PRIVATE_KEY_BASE64" <<'PY'
import base64
import binascii
import sys
try:
    raw = base64.b64decode(sys.argv[1], validate=True)
except (binascii.Error, ValueError):
    raise SystemExit("APGIC_REMOTE_CONFIG_PRIVATE_KEY_BASE64 is not valid base64")
if len(raw) not in (32, 64):
    raise SystemExit("APGIC_REMOTE_CONFIG_PRIVATE_KEY_BASE64 must decode to 32-byte Ed25519 seed or 64-byte private key")
PY

case ",$APGIC_MOBILE_SUPPORTED_CONTRACTS," in
  *,"$APGIC_MOBILE_CONTRACT_VERSION",*) ;;
  *)
    echo "current mobile contract is not in APGIC_MOBILE_SUPPORTED_CONTRACTS" >&2
    exit 1
    ;;
esac

CURRENT_SHA="$APGIC_COMMIT_SHA"

git fetch origin main
TARGET_SHA="$(git rev-parse "$TARGET_REF^{commit}")"

if ! git cat-file -e "$CURRENT_SHA^{commit}" 2>/dev/null; then
  echo "deployed commit is not available locally: $CURRENT_SHA" >&2
  exit 1
fi

if ! git merge-base --is-ancestor "$CURRENT_SHA" "$TARGET_SHA"; then
  echo "refusing non-forward deployment: current=$CURRENT_SHA target=$TARGET_SHA" >&2
  exit 1
fi

while IFS= read -r target_key; do
  [[ -z "$target_key" ]] && continue
  if ! grep -q "^${target_key}=" "$ENV_FILE"; then
    echo "target deployment requires missing environment key: $target_key" >&2
    exit 1
  fi
done < <(
  git show "$TARGET_SHA:deploy/staging/staging.env.example" |
    sed -n 's/^\([A-Z0-9_][A-Z0-9_]*\)=.*/\1/p'
)

if [[ "$CURRENT_SHA" == "$TARGET_SHA" ]]; then
  echo "APGIC staging already runs target commit $TARGET_SHA"
  exit 0
fi

mapfile -t migration_changes < <(
  git diff --name-status "$CURRENT_SHA" "$TARGET_SHA" -- 'backend/migrations/[0-9][0-9][0-9][0-9][0-9][0-9]_*.sql'
)

new_migrations=()
for change in "${migration_changes[@]}"; do
  status="${change%%$'\t'*}"
  path="${change#*$'\t'}"
  case "$status" in
    A)
      new_migrations+=("$path")
      ;;
    *)
      echo "refusing deployment: existing migration changed between deployed and target commits: $change" >&2
      exit 1
      ;;
  esac
done

if (("${#new_migrations[@]}" > 0)); then
  mapfile -t new_migrations < <(printf '%s\n' "${new_migrations[@]}" | LC_ALL=C sort)
fi

echo "APGIC staging update"
echo "  current: $CURRENT_SHA"
echo "  target:  $TARGET_SHA"
if (("${#new_migrations[@]}" > 0)); then
  printf '  migrations:\n'
  printf '    %s\n' "${new_migrations[@]}"
else
  echo "  migrations: none"
fi

git reset --hard "$TARGET_SHA"

if [[ "${APGIC_UPDATE_REEXEC:-0}" != "1" ]]; then
  echo "=== Re-exec target updater ==="
  APGIC_UPDATE_REEXEC=1 exec bash "$REPO_ROOT/deploy/staging/update-staging.sh" "$TARGET_SHA"
fi

echo "=== Build API ==="
cd "$REPO_ROOT/backend"
mkdir -p bin
go build -o bin/apgic-api ./cmd/api

echo "=== Build Web ==="
cd "$REPO_ROOT/apps/web"
npm install --ignore-scripts --no-audit --no-fund --package-lock=false
npm run build

# Next.js may rewrite the tracked next-env.d.ts during build. Deployment
# artifacts must never make the canonical checkout diverge from GitHub.
git restore -- next-env.d.ts

cd "$REPO_ROOT"

echo "=== Reconcile staging maintenance units ==="
maintenance_units=(
  apgic-staging-backup.service
  apgic-staging-backup.timer
  apgic-staging-restore-verify.service
  apgic-staging-restore-verify.timer
)
units_changed=false
for unit in "${maintenance_units[@]}"; do
  source_unit="$REPO_ROOT/deploy/staging/$unit"
  target_unit="/etc/systemd/system/$unit"
  if [[ ! -f "$target_unit" ]] || ! cmp -s "$source_unit" "$target_unit"; then
    install -m 0644 "$source_unit" "$target_unit"
    units_changed=true
  fi
done
if [[ "$units_changed" == "true" ]]; then
  systemctl daemon-reload
fi
systemctl enable --now apgic-staging-backup.timer apgic-staging-restore-verify.timer

if [[ -n "$(git status --porcelain)" ]]; then
  echo "deployment left the canonical checkout dirty:" >&2
  git status --short >&2
  exit 1
fi

ledger_exists="$(
  psql "$APGIC_DATABASE_URL" -Atqc     "SELECT to_regclass('public.apgic_schema_migrations') IS NOT NULL"
)"
case "$ledger_exists" in
  t|f) ;;
  *)
    echo "could not determine migration ledger state: $ledger_exists" >&2
    exit 1
    ;;
esac

backup_required=false
if [[ "$ledger_exists" != "t" ]] || (("${#new_migrations[@]}" > 0)); then
  backup_required=true
fi

if (("${#new_migrations[@]}" > 0)); then
  for migration in "${new_migrations[@]}"; do
    if grep -Eiq '^[[:space:]]*(BEGIN|COMMIT)[[:space:]]*;' "$REPO_ROOT/$migration"; then
      echo "refusing deployment: managed migration contains transaction control: $migration" >&2
      exit 1
    fi
  done
fi

if [[ "$backup_required" == "true" ]]; then
  if [[ "$ledger_exists" != "t" ]]; then
    echo "=== Backup PostgreSQL before migration ledger bootstrap ==="
  else
    echo "=== Backup PostgreSQL before new migrations ==="
  fi
  systemctl start apgic-staging-backup.service
  backup_result="$(systemctl show -p Result --value apgic-staging-backup.service)"
  if [[ "$backup_result" != "success" ]]; then
    echo "staging backup failed: Result=$backup_result" >&2
    exit 1
  fi
fi

echo "=== Reconcile migration ledger ==="
APGIC_REPO_ROOT="$REPO_ROOT" \
APGIC_DATABASE_URL="$APGIC_DATABASE_URL" \
bash "$REPO_ROOT/deploy/staging/apply-staging-migrations.sh" \
  "$CURRENT_SHA" "$TARGET_SHA" "${new_migrations[@]}"

echo "=== Pin deployed SHA ==="
if grep -q '^APGIC_COMMIT_SHA=' "$ENV_FILE"; then
  sed -i "s/^APGIC_COMMIT_SHA=.*/APGIC_COMMIT_SHA=$TARGET_SHA/" "$ENV_FILE"
else
  printf '\nAPGIC_COMMIT_SHA=%s\n' "$TARGET_SHA" >> "$ENV_FILE"
fi

set -a
source "$ENV_FILE"
set +a

echo "=== Restart runtime ==="
systemctl restart apgic-api-staging.service
systemctl restart apgic-web-staging.service

wait_for_http() {
  local name="$1"
  local url="$2"
  local attempts="${3:-30}"
  local delay="${4:-1}"

  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if curl -fsS --max-time 2 "$url" >/dev/null; then
      printf '%s ready after %d attempt(s): %s\n' "$name" "$attempt" "$url"
      return 0
    fi
    sleep "$delay"
  done

  printf '%s did not become ready after %d attempts: %s\n' "$name" "$attempts" "$url" >&2
  return 1
}

echo "=== Wait for runtime readiness ==="
if ! wait_for_http "API" "http://127.0.0.1:43111/readyz" 30 1; then
  systemctl --no-pager --full status apgic-api-staging.service >&2 || true
  journalctl -u apgic-api-staging.service -n 80 --no-pager >&2 || true
  exit 1
fi

if ! wait_for_http "Web" "http://127.0.0.1:43112/" 30 1; then
  systemctl --no-pager --full status apgic-web-staging.service >&2 || true
  journalctl -u apgic-web-staging.service -n 80 --no-pager >&2 || true
  exit 1
fi

echo "=== Verify runtime ==="
bash "$REPO_ROOT/deploy/staging/check-staging-runtime.sh"

actual_meta="$(curl -fsS --max-time 5 http://127.0.0.1:43111/v1/meta)"
python3 - "$TARGET_SHA" "$actual_meta" <<'PY'
import json
import sys

expected = sys.argv[1]
payload = json.loads(sys.argv[2])
actual = payload.get("commit_sha")
if actual != expected:
    raise SystemExit(f"runtime SHA mismatch: expected={expected} actual={actual}")
print(f"APGIC staging update: PASS sha={actual}")
PY
