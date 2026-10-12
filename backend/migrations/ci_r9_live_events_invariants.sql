-- R9 comprehensive SQL invariant suite.  One transaction, no persistent test data.
-- Every denial checks its exact domain failure, never accepts an unrelated SQL error.
BEGIN;
SET LOCAL statement_timeout = '30s';

-- Temp assertion helper: a broad WHEN OTHERS without checking SQLERRM
-- can accidentally certify a broken schema as "secure".
CREATE TEMP TABLE r9_ci_scope_marker(id integer) ON COMMIT DROP;
CREATE FUNCTION pg_temp.r9_must_reject(sql_command text, expected_message text)
RETURNS void LANGUAGE plpgsql AS $r9_assert$
DECLARE
  rejected boolean := false;
  failure_message text;
BEGIN
  BEGIN
    EXECUTE sql_command;
  EXCEPTION WHEN OTHERS THEN
    rejected := true;
    GET STACKED DIAGNOSTICS failure_message = MESSAGE_TEXT;
  END;
  IF NOT rejected THEN
    RAISE EXCEPTION 'R9 required denial missing: %', expected_message;
  END IF;
  IF position(expected_message in coalesce(failure_message, '')) = 0 THEN
    RAISE EXCEPTION 'R9 denial has WRONG reason, expected [%], got [%]',
      expected_message, failure_message;
  END IF;
END;
$r9_assert$;

INSERT INTO identities(id) VALUES
 ('91919191-0000-4000-8000-000000000001'),
 ('91919191-0000-4000-8000-000000000002'),
 ('91919191-0000-4000-8000-000000000003'),
 ('91919191-0000-4000-8000-000000000004'),
 ('91919191-0000-4000-8000-000000000005');
INSERT INTO organizations(id,name,status) VALUES
 ('91919191-0000-4000-8000-000000000010','R9 CI organizer','ACTIVE');
INSERT INTO organization_memberships(organization_id,identity_id,status)
 VALUES('91919191-0000-4000-8000-000000000010','91919191-0000-4000-8000-000000000001','ACTIVE');
INSERT INTO organization_ownerships(organization_id,identity_id,status)
 VALUES('91919191-0000-4000-8000-000000000010','91919191-0000-4000-8000-000000000001','ACTIVE');
INSERT INTO connector_instances(id,capability_class,provider_kind,status,config_ref)
 VALUES('91919191-0000-4000-8000-000000000020','COMMUNICATION_PROVIDER','CI_SFU','ACTIVE','ci-remote-capability-check');
INSERT INTO connector_instances(id,capability_class,provider_kind,status,config_ref)
 VALUES('91919191-0000-4000-8000-000000000021','COMMUNICATION_PROVIDER','CI_SFU','ACTIVE','ci-other-provider');

-- Owner and draft boundaries are enforced BY DATABASE, not only handlers.
SELECT pg_temp.r9_must_reject($r9_stmt$
 INSERT INTO live_events(id,organization_id,organizer_identity_id,title,format,starts_at,ends_at,capacity)
 VALUES('91919191-0000-4000-8000-000000000031','91919191-0000-4000-8000-000000000010',
 '91919191-0000-4000-8000-000000000002','Foreign owner not accepted','WEBINAR',
 now()+interval '3 days',now()+interval '3 days 1 hour',2)
$r9_stmt$, 'R9 event requires active owner');

SELECT pg_temp.r9_must_reject($r9_stmt$
 INSERT INTO live_events(id,organization_id,organizer_identity_id,title,format,starts_at,ends_at,capacity,price_minor)
 VALUES('91919191-0000-4000-8000-000000000032','91919191-0000-4000-8000-000000000010',
 '91919191-0000-4000-8000-000000000001','No unverified payment','WEBINAR',
 now()+interval '3 days',now()+interval '3 days 1 hour',2,100)
$r9_stmt$, 'live_events_price_minor_check');

INSERT INTO live_events(id,organization_id,organizer_identity_id,title,description,format,starts_at,ends_at,capacity)
 VALUES('91919191-0000-4000-8000-000000000030',
 '91919191-0000-4000-8000-000000000010',
 '91919191-0000-4000-8000-000000000001',
 'R9 Group CI','CI registration and attendance proof','GROUP',
 now()+interval '3 days',now()+interval '3 days 1 hour',2);

SELECT pg_temp.r9_must_reject($r9_stmt$
 INSERT INTO live_event_registrations(id,event_id,identity_id)
 VALUES('91919191-0000-4000-8000-000000000041','91919191-0000-4000-8000-000000000030',
 '91919191-0000-4000-8000-000000000002')
$r9_stmt$, 'R9 registration requires published future event');

SELECT pg_temp.r9_must_reject($r9_stmt$
 UPDATE live_events SET state='PUBLISHED'
 WHERE id='91919191-0000-4000-8000-000000000030'
$r9_stmt$, 'R9 event must have signed provider readiness');

-- The media provider is a signed-evidence boundary.  This SQL test models
-- storage state only, not production SFU identity or credential issuance.
UPDATE live_events SET provider_instance_id='91919191-0000-4000-8000-000000000020',
 provider_proof_checked_at=clock_timestamp(),state='PUBLISHED'
 WHERE id='91919191-0000-4000-8000-000000000030';

SELECT pg_temp.r9_must_reject($r9_stmt$
 UPDATE live_events SET capacity=3
 WHERE id='91919191-0000-4000-8000-000000000030'
$r9_stmt$, 'R9 published event business identity/schedule is immutable');
SELECT pg_temp.r9_must_reject($r9_stmt$
 UPDATE live_events SET state='DRAFT'
 WHERE id='91919191-0000-4000-8000-000000000030'
$r9_stmt$, 'R9 illegal event lifecycle transition');

INSERT INTO live_event_registrations(id,event_id,identity_id)
 VALUES
 ('91919191-0000-4000-8000-000000000041','91919191-0000-4000-8000-000000000030','91919191-0000-4000-8000-000000000002'),
 ('91919191-0000-4000-8000-000000000042','91919191-0000-4000-8000-000000000030','91919191-0000-4000-8000-000000000003');

SELECT pg_temp.r9_must_reject($r9_stmt$
 INSERT INTO live_event_registrations(id,event_id,identity_id)
 VALUES('91919191-0000-4000-8000-000000000043','91919191-0000-4000-8000-000000000030',
 '91919191-0000-4000-8000-000000000004')
$r9_stmt$, 'R9 event capacity exhausted');

-- Cancelling returns a seat. Filling it blocks a cancelled participant
-- from silently exceeding capacity; releasing it allows re-registration.
UPDATE live_event_registrations SET state='CANCELLED',cancelled_at=clock_timestamp()
 WHERE id='91919191-0000-4000-8000-000000000042';
INSERT INTO live_event_registrations(id,event_id,identity_id)
 VALUES('91919191-0000-4000-8000-000000000043','91919191-0000-4000-8000-000000000030',
 '91919191-0000-4000-8000-000000000004');
SELECT pg_temp.r9_must_reject($r9_stmt$
 UPDATE live_event_registrations SET state='REGISTERED',cancelled_at=NULL,registered_at=clock_timestamp()
 WHERE id='91919191-0000-4000-8000-000000000042'
$r9_stmt$, 'R9 re-registration capacity exhausted');
UPDATE live_event_registrations SET state='CANCELLED',cancelled_at=clock_timestamp()
 WHERE id='91919191-0000-4000-8000-000000000043';
UPDATE live_event_registrations SET state='REGISTERED',registered_at=clock_timestamp(),cancelled_at=NULL
 WHERE id='91919191-0000-4000-8000-000000000042';

SELECT pg_temp.r9_must_reject($r9_stmt$
 UPDATE live_event_registrations SET state='REGISTERED'
 WHERE id='91919191-0000-4000-8000-000000000041'
$r9_stmt$, 'R9 registration has illegal transition');
SELECT pg_temp.r9_must_reject($r9_stmt$
 UPDATE live_event_registrations SET identity_id='91919191-0000-4000-8000-000000000005'
 WHERE id='91919191-0000-4000-8000-000000000041'
$r9_stmt$, 'R9 registration identity is immutable');

-- Negative provider evidence: wrong registration, mismatched signed provider,
-- duplicate delivery and immutable JOINED/LEFT observations.
SELECT pg_temp.r9_must_reject($r9_stmt$
 INSERT INTO live_event_attendance_facts(id,event_id,registration_id,connector_instance_id,provider_event_id,fact_type,occurred_at)
 VALUES('91919191-0000-4000-8000-000000000051','91919191-0000-4000-8000-000000000030',
 '91919191-0000-4000-8000-000000000043','91919191-0000-4000-8000-000000000020','unregistered-join','JOINED',clock_timestamp())
$r9_stmt$, 'R9 attendance requires registered participant and bound provider');
SELECT pg_temp.r9_must_reject($r9_stmt$
 INSERT INTO live_event_attendance_facts(id,event_id,registration_id,connector_instance_id,provider_event_id,fact_type,occurred_at)
 VALUES('91919191-0000-4000-8000-000000000052','91919191-0000-4000-8000-000000000030',
 '91919191-0000-4000-8000-000000000041','91919191-0000-4000-8000-000000000021','wrong-provider','JOINED',clock_timestamp())
$r9_stmt$, 'R9 attendance requires registered participant and bound provider');

INSERT INTO live_event_attendance_facts(id,event_id,registration_id,connector_instance_id,provider_event_id,fact_type,occurred_at)
 VALUES('91919191-0000-4000-8000-000000000050',
 '91919191-0000-4000-8000-000000000030',
 '91919191-0000-4000-8000-000000000041',
 '91919191-0000-4000-8000-000000000020','joined-ci-1','JOINED',clock_timestamp());

SELECT pg_temp.r9_must_reject($r9_stmt$
 INSERT INTO live_event_attendance_facts(id,event_id,registration_id,connector_instance_id,provider_event_id,fact_type,occurred_at)
 VALUES('91919191-0000-4000-8000-000000000053','91919191-0000-4000-8000-000000000030',
 '91919191-0000-4000-8000-000000000041','91919191-0000-4000-8000-000000000020','joined-ci-1','JOINED',clock_timestamp())
$r9_stmt$, 'duplicate key value violates unique constraint');

SELECT pg_temp.r9_must_reject($r9_stmt$
 UPDATE live_event_attendance_facts SET fact_type='LEFT'
 WHERE id='91919191-0000-4000-8000-000000000050'
$r9_stmt$, 'append');
SELECT pg_temp.r9_must_reject($r9_stmt$
 DELETE FROM live_event_attendance_facts
 WHERE id='91919191-0000-4000-8000-000000000050'
$r9_stmt$, 'append');

-- Reconcile distinct signalled attendees with business registrations.
DO $r9_projection$
DECLARE
  registered_count integer;
  attended_count integer;
BEGIN
  SELECT count(*) INTO registered_count FROM live_event_registrations
   WHERE event_id='91919191-0000-4000-8000-000000000030' AND state='REGISTERED';
  SELECT count(DISTINCT registration_id) INTO attended_count FROM live_event_attendance_facts
   WHERE event_id='91919191-0000-4000-8000-000000000030' AND fact_type='JOINED';
  IF registered_count<>2 OR attended_count<>1 THEN
    RAISE EXCEPTION 'R9 attendance projection mismatch registered=% attended=%',
      registered_count,attended_count;
  END IF;
END;
$r9_projection$;

UPDATE live_events SET state='LIVE',updated_at=clock_timestamp()
 WHERE id='91919191-0000-4000-8000-000000000030';
UPDATE live_events SET state='COMPLETED',updated_at=clock_timestamp()
 WHERE id='91919191-0000-4000-8000-000000000030';
SELECT pg_temp.r9_must_reject($r9_stmt$
 UPDATE live_events SET state='LIVE'
 WHERE id='91919191-0000-4000-8000-000000000030'
$r9_stmt$, 'R9 illegal event lifecycle transition');
SELECT pg_temp.r9_must_reject($r9_stmt$
 DELETE FROM live_events WHERE id='91919191-0000-4000-8000-000000000030'
$r9_stmt$, 'append');
-- No fixture or helper persists after rollback.
ROLLBACK;
SELECT 'R9 LIVE EVENT SQL INVARIANTS: PASS' AS verdict;
