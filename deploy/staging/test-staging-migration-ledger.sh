#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="${APGIC_REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
: "${APGIC_DATABASE_URL:?APGIC_DATABASE_URL is required}"

cd "$REPO_ROOT"
current_sha="$(git rev-parse HEAD)"
probe_ok="backend/migrations/999998_ci_ledger_probe.sql"
probe_fail="backend/migrations/999999_ci_ledger_rollback.sql"

cleanup() {
  rm -f "$probe_ok" "$probe_fail"
  psql "$APGIC_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL' >/dev/null
DROP TABLE IF EXISTS apgic_ci_migration_probe;
DROP TABLE IF EXISTS apgic_ci_migration_rollback_probe;
DELETE FROM apgic_schema_migrations
 WHERE migration_name IN (
   'backend/migrations/999998_ci_ledger_probe.sql',
   'backend/migrations/999999_ci_ledger_rollback.sql'
 );
SQL
}
trap cleanup EXIT

expected_baseline="$(
  git ls-tree -r --name-only "$current_sha" -- backend/migrations |
    grep -E '^backend/migrations/[0-9]{6}_.+\.sql$' |
    wc -l |
    tr -d ' '
)"

APGIC_REPO_ROOT="$REPO_ROOT" \
APGIC_DATABASE_URL="$APGIC_DATABASE_URL" \
bash deploy/staging/apply-staging-migrations.sh "$current_sha" "$current_sha"

actual_baseline="$(
  psql "$APGIC_DATABASE_URL" -Atc \
    "SELECT count(*) FROM apgic_schema_migrations WHERE baseline"
)"
test "$actual_baseline" = "$expected_baseline"

cat > "$probe_ok" <<'SQL'
CREATE TABLE apgic_ci_migration_probe (
  id bigint PRIMARY KEY
);
SQL

APGIC_REPO_ROOT="$REPO_ROOT" \
APGIC_DATABASE_URL="$APGIC_DATABASE_URL" \
bash deploy/staging/apply-staging-migrations.sh \
  "$current_sha" "$current_sha" "$probe_ok"

test "$(
  psql "$APGIC_DATABASE_URL" -Atc \
    "SELECT count(*) FROM apgic_schema_migrations WHERE migration_name='$probe_ok' AND NOT baseline"
)" = "1"

test "$(
  psql "$APGIC_DATABASE_URL" -Atc \
    "SELECT to_regclass('public.apgic_ci_migration_probe') IS NOT NULL"
)" = "t"

APGIC_REPO_ROOT="$REPO_ROOT" \
APGIC_DATABASE_URL="$APGIC_DATABASE_URL" \
bash deploy/staging/apply-staging-migrations.sh \
  "$current_sha" "$current_sha" "$probe_ok"

test "$(
  psql "$APGIC_DATABASE_URL" -Atc \
    "SELECT count(*) FROM apgic_schema_migrations WHERE migration_name='$probe_ok'"
)" = "1"

cat >> "$probe_ok" <<'SQL'
-- checksum drift must fail closed
SQL

if APGIC_REPO_ROOT="$REPO_ROOT" \
   APGIC_DATABASE_URL="$APGIC_DATABASE_URL" \
   bash deploy/staging/apply-staging-migrations.sh \
     "$current_sha" "$current_sha" "$probe_ok"; then
  echo "checksum drift unexpectedly passed" >&2
  exit 1
fi

cat > "$probe_fail" <<'SQL'
CREATE TABLE apgic_ci_migration_rollback_probe (
  id bigint PRIMARY KEY
);
SELECT 1 / 0;
SQL

if APGIC_REPO_ROOT="$REPO_ROOT" \
   APGIC_DATABASE_URL="$APGIC_DATABASE_URL" \
   bash deploy/staging/apply-staging-migrations.sh \
     "$current_sha" "$current_sha" "$probe_fail"; then
  echo "failing migration unexpectedly passed" >&2
  exit 1
fi

test "$(
  psql "$APGIC_DATABASE_URL" -Atc \
    "SELECT to_regclass('public.apgic_ci_migration_rollback_probe') IS NULL"
)" = "t"

test "$(
  psql "$APGIC_DATABASE_URL" -Atc \
    "SELECT count(*) FROM apgic_schema_migrations WHERE migration_name='$probe_fail'"
)" = "0"

cat > "$probe_fail" <<'SQL'
CREATE TABLE apgic_ci_migration_rollback_probe (
  id bigint PRIMARY KEY
);
SQL

APGIC_REPO_ROOT="$REPO_ROOT" \
APGIC_DATABASE_URL="$APGIC_DATABASE_URL" \
bash deploy/staging/apply-staging-migrations.sh \
  "$current_sha" "$current_sha" "$probe_fail"

test "$(
  psql "$APGIC_DATABASE_URL" -Atc \
    "SELECT count(*) FROM apgic_schema_migrations WHERE migration_name='$probe_fail' AND NOT baseline"
)" = "1"

echo "STAGING MIGRATION LEDGER POSTGRES PROOF: PASS"
