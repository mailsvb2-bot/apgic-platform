#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="${APGIC_REPO_ROOT:-/opt/apgic/current}"
: "${APGIC_DATABASE_URL:?APGIC_DATABASE_URL is required}"

CURRENT_SHA="${1:?current deployed SHA is required}"
shift
TARGET_SHA="${1:?target SHA is required}"
shift
new_migrations=("$@")

sql_file="$(mktemp)"
trap 'rm -f "$sql_file"' EXIT

sql_quote() {
  printf "%s" "$1" | sed "s/'/''/g"
}

{
  cat <<'SQL'
\set ON_ERROR_STOP on
SELECT pg_advisory_lock(hashtextextended('apgic-staging-migrations-v1', 0));
CREATE TABLE IF NOT EXISTS apgic_schema_migrations (
  migration_name text PRIMARY KEY,
  checksum_sha256 text NOT NULL CHECK (checksum_sha256 ~ '^[0-9a-f]{64}$'),
  applied_at timestamptz NOT NULL DEFAULT now(),
  baseline boolean NOT NULL DEFAULT false
);
SQL

  while IFS= read -r migration; do
    [[ -n "$migration" ]] || continue
    checksum="$(git -C "$REPO_ROOT" show "$CURRENT_SHA:$migration" | sha256sum | awk '{print $1}')"
    q_name="$(sql_quote "$migration")"
    q_checksum="$(sql_quote "$checksum")"
    cat <<SQL
DO \$apgic\$
BEGIN
  IF EXISTS (
    SELECT 1 FROM apgic_schema_migrations
     WHERE migration_name = '$q_name'
       AND checksum_sha256 <> '$q_checksum'
  ) THEN
    RAISE EXCEPTION 'migration checksum mismatch for $q_name';
  END IF;
  INSERT INTO apgic_schema_migrations(migration_name, checksum_sha256, baseline)
  VALUES ('$q_name', '$q_checksum', true)
  ON CONFLICT (migration_name) DO NOTHING;
END
\$apgic\$;
SQL
  done < <(
    git -C "$REPO_ROOT" ls-tree -r --name-only "$CURRENT_SHA" -- backend/migrations |
      grep -E '^backend/migrations/[0-9]{6}_.+\.sql$' |
      LC_ALL=C sort
  )

  for migration in "${new_migrations[@]}"; do
    checksum="$(sha256sum "$REPO_ROOT/$migration" | awk '{print $1}')"
    q_name="$(sql_quote "$migration")"
    q_checksum="$(sql_quote "$checksum")"
    cat <<SQL
DO \$apgic\$
BEGIN
  IF EXISTS (
    SELECT 1 FROM apgic_schema_migrations
     WHERE migration_name = '$q_name'
       AND checksum_sha256 <> '$q_checksum'
  ) THEN
    RAISE EXCEPTION 'migration checksum mismatch for $q_name';
  END IF;
END
\$apgic\$;
SELECT EXISTS (
  SELECT 1 FROM apgic_schema_migrations
   WHERE migration_name = '$q_name'
     AND checksum_sha256 = '$q_checksum'
) AS apgic_already_applied \gset
\if :apgic_already_applied
\echo 'Skipping already applied migration $q_name'
\else
\echo 'Applying migration $q_name'
BEGIN;
\i '$REPO_ROOT/$migration'
INSERT INTO apgic_schema_migrations(migration_name, checksum_sha256, baseline)
VALUES ('$q_name', '$q_checksum', false);
COMMIT;
\endif
SQL
  done

  cat <<'SQL'
SELECT pg_advisory_unlock(hashtextextended('apgic-staging-migrations-v1', 0));
SQL
} > "$sql_file"

psql "$APGIC_DATABASE_URL" -v ON_ERROR_STOP=1 -f "$sql_file"

echo "APGIC staging migration ledger: PASS current=$CURRENT_SHA target=$TARGET_SHA"
