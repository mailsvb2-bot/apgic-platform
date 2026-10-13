#!/usr/bin/env bash
# R9 multi-connection admission proof against actual PostgreSQL.
# Only disposable CI PostgreSQL is permitted.
set -euo pipefail
: "${PGHOST:=127.0.0.1}"
: "${PGPORT:=5432}"
: "${PGUSER:=apgic}"
: "${PGDATABASE:=apgic_ci}"
export PGHOST PGPORT PGUSER PGDATABASE

readonly event='92929292-0000-4000-8000-000000000102'
readonly attempts=24
readonly capacity=3

psql -X -v ON_ERROR_STOP=1 -q <<'SQL'
INSERT INTO identities(id)
 SELECT ('92929292-0000-4000-8000-' || lpad(n::text,12,'0'))::uuid
 FROM generate_series(1,30) n
 ON CONFLICT (id) DO NOTHING;
INSERT INTO organizations(id,name,status)
 VALUES('92929292-0000-4000-8000-000000000100','R9 concurrent admission owner','ACTIVE');
INSERT INTO organization_memberships(organization_id,identity_id,status)
 VALUES('92929292-0000-4000-8000-000000000100','92929292-0000-4000-8000-000000000001','ACTIVE');
INSERT INTO organization_ownerships(organization_id,identity_id,status)
 VALUES('92929292-0000-4000-8000-000000000100','92929292-0000-4000-8000-000000000001','ACTIVE');
INSERT INTO connector_instances(id,capability_class,provider_kind,status,config_ref)
 VALUES('92929292-0000-4000-8000-000000000101','COMMUNICATION_PROVIDER','CI_SFU','ACTIVE','r9-concurrent-ci');
INSERT INTO live_events(id,organization_id,organizer_identity_id,title,description,format,starts_at,ends_at,capacity)
 VALUES('92929292-0000-4000-8000-000000000102',
 '92929292-0000-4000-8000-000000000100',
 '92929292-0000-4000-8000-000000000001',
 'Concurrent registration test','24 independent clients contend for 3 places','GROUP',
 now()+interval '2 days',now()+interval '2 days 1 hour',3);
UPDATE live_events SET state='PUBLISHED',
 provider_instance_id='92929292-0000-4000-8000-000000000101',
 provider_proof_checked_at=clock_timestamp(),
 ucr_conference_id='ucr-r9-concurrent-room',ucr_integration_id='ucr-r9-integration'
 WHERE id='92929292-0000-4000-8000-000000000102';
SQL

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
pids=()
for n in $(seq 2 25); do
  (
    identity="$(printf '92929292-0000-4000-8000-%012d' "$n")"
    registration="$(printf '93939393-0000-4000-8000-%012d' "$n")"
    if psql -X -q -v ON_ERROR_STOP=1 -c "
      INSERT INTO live_event_registrations(id,event_id,identity_id)
      VALUES('$registration','$event','$identity');" >"$tmp/$n.log" 2>&1; then
      echo 'ADMITTED' >"$tmp/$n.status"
    elif grep -q 'R9 event capacity exhausted' "$tmp/$n.log"; then
      echo 'CAPACITY_DENIED' >"$tmp/$n.status"
    else
      echo 'UNEXPECTED_FAILURE' >"$tmp/$n.status"
      cat "$tmp/$n.log" >&2
      exit 1
    fi
  ) &
  pids+=("$!")
done
for pid in "${pids[@]}"; do
  wait "$pid"
done

admitted="$(grep -l '^ADMITTED$' "$tmp"/*.status | wc -l | tr -d ' ')"
rejected="$(grep -l '^CAPACITY_DENIED$' "$tmp"/*.status | wc -l | tr -d ' ')"
actual="$(psql -X -At -v ON_ERROR_STOP=1 -c "
SELECT count(*) FROM live_event_registrations
WHERE event_id='$event' AND state='REGISTERED';")"
distinct="$(psql -X -At -v ON_ERROR_STOP=1 -c "
SELECT count(DISTINCT identity_id) FROM live_event_registrations
WHERE event_id='$event' AND state='REGISTERED';")"
if [[ "$admitted" != "$capacity" || "$rejected" != "$((attempts-capacity))" ||
  "$actual" != "$capacity" || "$distinct" != "$capacity" ]]; then
  echo "R9 CONCURRENCY FAIL: admitted=$admitted denied=$rejected active=$actual distinct=$distinct" >&2
  exit 1
fi

# Replayed identity with a different registration ID must never create another seat.
existing_identity="$(psql -X -At -v ON_ERROR_STOP=1 -c "
SELECT identity_id FROM live_event_registrations WHERE event_id='$event' LIMIT 1;")"
if psql -X -q -v ON_ERROR_STOP=1 -c "
INSERT INTO live_event_registrations(id,event_id,identity_id)
VALUES('93939393-0000-4000-8000-000000000099','$event','$existing_identity');" >"$tmp/replay.log" 2>&1; then
  echo "R9 CONCURRENCY FAIL: duplicate identity admitted" >&2
  exit 1
fi
grep -q 'R9 event capacity exhausted' "$tmp/replay.log"

echo "R9 REGISTRATION CONCURRENCY: PASS ($admitted admissions, $rejected capacity denials, no duplicate seats)"
