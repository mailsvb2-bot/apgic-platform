#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="${APGIC_REPO_ROOT:-/opt/apgic/current}"
ENV_FILE="${APGIC_ENV_FILE:-/etc/apgic/staging.env}"
TARGET_REF="${1:-origin/main}"

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

echo "=== Build API ==="
cd "$REPO_ROOT/backend"
mkdir -p bin
go build -o bin/apgic-api ./cmd/api

echo "=== Build Web ==="
cd "$REPO_ROOT/apps/web"
npm install --ignore-scripts --no-audit --no-fund
npm run build

cd "$REPO_ROOT"

if (("${#new_migrations[@]}" > 0)); then
  echo "=== Backup PostgreSQL ==="
  systemctl start apgic-staging-backup.service
  backup_result="$(systemctl show -p Result --value apgic-staging-backup.service)"
  if [[ "$backup_result" != "success" ]]; then
    echo "staging backup failed: Result=$backup_result" >&2
    exit 1
  fi

  echo "=== Apply migrations ==="
  for migration in "${new_migrations[@]}"; do
    echo "Applying $migration"
    psql "$APGIC_DATABASE_URL" -v ON_ERROR_STOP=1 -f "$REPO_ROOT/$migration"
  done
fi

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

for _ in {1..20}; do
  if curl -fsS --max-time 2 "http://127.0.0.1:43111/readyz" >/dev/null; then
    break
  fi
  sleep 1
done

echo "=== Verify runtime ==="
"$REPO_ROOT/deploy/staging/check-staging-runtime.sh"

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
