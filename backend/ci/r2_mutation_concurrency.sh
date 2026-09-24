#!/usr/bin/env bash
set -euo pipefail

: "${PGHOST:=127.0.0.1}"
: "${PGPORT:=5432}"
: "${PGUSER:=apgic}"
: "${PGDATABASE:=apgic_ci}"
export PGHOST PGPORT PGUSER PGDATABASE

identity_id="00000000-0000-0000-0000-00000000fc01"
mutation_a="00000000-0000-0000-0000-00000000fc11"
mutation_b="00000000-0000-0000-0000-00000000fc12"

psql -v ON_ERROR_STOP=1 -c "
  INSERT INTO identities (id)
  VALUES ('$identity_id')
  ON CONFLICT (id) DO NOTHING;
"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

claim() {
  local mutation_id="$1"
  local output="$2"
  psql -v ON_ERROR_STOP=1 -At -F '|' -c "
    SELECT outcome, mutation_id
    FROM apgic_claim_client_mutation(
      '$mutation_id',
      '$identity_id',
      'PAYMENT_CREATE',
      'concurrent-offline-key',
      'sha256:concurrent-payload',
      now()
    );
  " >"$output"
}

claim "$mutation_a" "$tmp_dir/a.out" &
pid_a=$!
claim "$mutation_b" "$tmp_dir/b.out" &
pid_b=$!

wait "$pid_a"
wait "$pid_b"

cat "$tmp_dir/a.out"
cat "$tmp_dir/b.out"

claimed_count="$(cat "$tmp_dir/a.out" "$tmp_dir/b.out" | grep -c '^CLAIMED|' || true)"
duplicate_count="$(cat "$tmp_dir/a.out" "$tmp_dir/b.out" | grep -c '^DUPLICATE|' || true)"
record_count="$(psql -At -c "
  SELECT count(*)
  FROM client_mutation_records
  WHERE identity_id = '$identity_id'
    AND operation = 'PAYMENT_CREATE'
    AND idempotency_key = 'concurrent-offline-key';
")"

if [[ "$claimed_count" != "1" || "$duplicate_count" != "1" || "$record_count" != "1" ]]; then
  echo "offline mutation concurrency invariant failed: claimed=$claimed_count duplicate=$duplicate_count records=$record_count" >&2
  exit 1
fi

echo "R2 OFFLINE MUTATION CONCURRENCY: PASS (one canonical claim)"
