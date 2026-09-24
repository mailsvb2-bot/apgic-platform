#!/usr/bin/env bash
set -euo pipefail

: "${PGHOST:=127.0.0.1}"
: "${PGPORT:=5432}"
: "${PGUSER:=apgic}"
: "${PGDATABASE:=apgic_ci}"

slot_id="00000000-0000-0000-0000-00000000c101"
specialist_id="00000000-0000-0000-0000-00000000c001"
client_a="00000000-0000-0000-0000-00000000c002"
client_b="00000000-0000-0000-0000-00000000c003"
hold_a="00000000-0000-0000-0000-00000000c201"
hold_b="00000000-0000-0000-0000-00000000c202"

psql -v ON_ERROR_STOP=1 <<SQL
INSERT INTO identities (id)
VALUES
  ('$specialist_id'),
  ('$client_a'),
  ('$client_b')
ON CONFLICT (id) DO NOTHING;

INSERT INTO booking_slots (
  id,
  specialist_identity_id,
  tenant_scope,
  starts_at,
  ends_at,
  exclusive
) VALUES (
  '$slot_id',
  '$specialist_id',
  'tenant/r2-concurrency',
  now() + interval '2 hours',
  now() + interval '3 hours',
  true
)
ON CONFLICT (id) DO NOTHING;

DELETE FROM booking_holds
WHERE slot_id = '$slot_id'
  AND state <> 'ACTIVE';
SQL

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

acquire() {
  local hold_id="$1"
  local client_id="$2"
  local output="$3"
  psql -v ON_ERROR_STOP=1 -At -F '|' -c "
    SELECT acquired, reason_code
    FROM apgic_acquire_slot_hold(
      '$hold_id',
      '$slot_id',
      '$client_id',
      now() + interval '10 minutes',
      now()
    );
  " >"$output"
}

acquire "$hold_a" "$client_a" "$tmp_dir/a.out" &
pid_a=$!
acquire "$hold_b" "$client_b" "$tmp_dir/b.out" &
pid_b=$!

wait "$pid_a"
wait "$pid_b"

cat "$tmp_dir/a.out"
cat "$tmp_dir/b.out"

acquired_count="$(
  cat "$tmp_dir/a.out" "$tmp_dir/b.out" |
    grep -c '^t|BOOK_HOLD_ACQUIRED$' || true
)"
conflict_count="$(
  cat "$tmp_dir/a.out" "$tmp_dir/b.out" |
    grep -c '^f|BOOK_SLOT_HELD$' || true
)"
active_count="$(
  psql -At -c "
    SELECT count(*)
    FROM booking_holds
    WHERE slot_id = '$slot_id'
      AND state = 'ACTIVE';
  "
)"
distinct_clients="$(
  psql -At -c "
    SELECT count(DISTINCT client_identity_id)
    FROM booking_holds
    WHERE slot_id = '$slot_id'
      AND state = 'ACTIVE';
  "
)"

if [[ "$acquired_count" != "1" ]]; then
  echo "expected exactly one BOOK_HOLD_ACQUIRED, got $acquired_count" >&2
  exit 1
fi
if [[ "$conflict_count" != "1" ]]; then
  echo "expected exactly one BOOK_SLOT_HELD, got $conflict_count" >&2
  exit 1
fi
if [[ "$active_count" != "1" || "$distinct_clients" != "1" ]]; then
  echo "exclusive slot invariant failed: active=$active_count clients=$distinct_clients" >&2
  exit 1
fi

echo "R2 BOOKING CONCURRENCY: PASS (exactly one active hold)"
