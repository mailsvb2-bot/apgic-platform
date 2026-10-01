#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PSQL=(psql -v ON_ERROR_STOP=1)
SCHEMA="order_snapshot_repair_$RANDOM$RANDOM"
MIGRATION="$ROOT/backend/migrations/000027_r0_order_snapshot_baseline_repair.sql"

cleanup() {
  "${PSQL[@]}" -c "DROP SCHEMA IF EXISTS \"$SCHEMA\" CASCADE" >/dev/null 2>&1 || true
}
trap cleanup EXIT

"${PSQL[@]}" <<SQL
CREATE SCHEMA "$SCHEMA";
SET search_path TO "$SCHEMA", public;

CREATE TABLE products (
  id uuid PRIMARY KEY,
  owner_type text NOT NULL,
  owner_id uuid NOT NULL,
  author_refs text[] NOT NULL
);

CREATE TABLE orders (
  id uuid PRIMARY KEY,
  product_id uuid NULL REFERENCES products(id)
);

INSERT INTO products (id, owner_type, owner_id, author_refs)
VALUES (
  '00000000-0000-0000-0000-000000002701',
  'ORGANIZATION',
  '00000000-0000-0000-0000-000000002702',
  ARRAY['identity/author-1']::text[]
);

INSERT INTO orders (id, product_id)
VALUES
  ('00000000-0000-0000-0000-000000002703', '00000000-0000-0000-0000-000000002701'),
  ('00000000-0000-0000-0000-000000002704', NULL);
SQL

"${PSQL[@]}" -c "SET search_path TO \"$SCHEMA\", public" -f "$MIGRATION"

"${PSQL[@]}" <<SQL
SET search_path TO "$SCHEMA", public;
DO \$\$
DECLARE
  owner_ref text;
  authors text[];
  historical_owner text;
BEGIN
  SELECT product_owner_ref, author_refs
    INTO owner_ref, authors
    FROM orders
   WHERE id = '00000000-0000-0000-0000-000000002703';

  IF owner_ref <> 'organization/00000000-0000-0000-0000-000000002702' THEN
    RAISE EXCEPTION 'repair owner snapshot mismatch: %', owner_ref;
  END IF;
  IF authors IS DISTINCT FROM ARRAY['identity/author-1']::text[] THEN
    RAISE EXCEPTION 'repair author snapshot mismatch: %', authors;
  END IF;

  SELECT product_owner_ref
    INTO historical_owner
    FROM orders
   WHERE id = '00000000-0000-0000-0000-000000002704';
  IF historical_owner IS NOT NULL THEN
    RAISE EXCEPTION 'pre-product historical order was rewritten';
  END IF;
END;
\$\$;
SQL

# Re-running the repair must remain idempotent.
"${PSQL[@]}" -c "SET search_path TO \"$SCHEMA\", public" -f "$MIGRATION"

"${PSQL[@]}" <<SQL
SET search_path TO "$SCHEMA", public;
DO \$\$
BEGIN
  BEGIN
    UPDATE orders
       SET product_owner_ref = ''
     WHERE id = '00000000-0000-0000-0000-000000002703';
    RAISE EXCEPTION 'blank owner ref unexpectedly accepted';
  EXCEPTION
    WHEN check_violation THEN NULL;
  END;

  BEGIN
    UPDATE orders
       SET author_refs = ARRAY[]::text[]
     WHERE id = '00000000-0000-0000-0000-000000002703';
    RAISE EXCEPTION 'empty author refs unexpectedly accepted';
  EXCEPTION
    WHEN check_violation THEN NULL;
  END;
END;
\$\$;
SQL

echo "R0 ORDER SNAPSHOT BASELINE REPAIR: PASS"