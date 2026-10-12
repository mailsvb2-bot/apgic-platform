-- R9 live domain invariants, isolated under a rollback.
BEGIN;
INSERT INTO identities(id) VALUES
 ('91919191-0000-4000-8000-000000000001'),
 ('91919191-0000-4000-8000-000000000002'),
 ('91919191-0000-4000-8000-000000000003'),
 ('91919191-0000-4000-8000-000000000004');
INSERT INTO organizations(id,name,status) VALUES
 ('91919191-0000-4000-8000-000000000010','R9 CI organizer','ACTIVE');
INSERT INTO organization_memberships(organization_id,identity_id,status)
 VALUES('91919191-0000-4000-8000-000000000010','91919191-0000-4000-8000-000000000001','ACTIVE');
INSERT INTO organization_ownerships(organization_id,identity_id,status)
 VALUES('91919191-0000-4000-8000-000000000010','91919191-0000-4000-8000-000000000001','ACTIVE');
INSERT INTO connector_instances(id,capability_class,provider_kind,status,config_ref)
 VALUES('91919191-0000-4000-8000-000000000020','COMMUNICATION_PROVIDER','CI_SFU','ACTIVE','ci-remote-capability-check');
DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
   INSERT INTO live_events(id,organization_id,organizer_identity_id,title,format,starts_at,ends_at,capacity)
   VALUES('91919191-0000-4000-8000-000000000031','91919191-0000-4000-8000-000000000010',
   '91919191-0000-4000-8000-000000000002','Foreign owner not accepted','WEBINAR',
   now()+interval '3 days',now()+interval '3 days 1 hour',2);
  EXCEPTION WHEN OTHERS THEN blocked:=true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'R9 unauthorized owner was accepted'; END IF;
END;
$$;
INSERT INTO live_events(id,organization_id,organizer_identity_id,title,description,format,starts_at,ends_at,capacity)
 VALUES('91919191-0000-4000-8000-000000000030',
 '91919191-0000-4000-8000-000000000010',
 '91919191-0000-4000-8000-000000000001',
 'R9 Group CI','CI migration invariant group','GROUP',
 now()+interval '3 days',now()+interval '3 days 1 hour',2);
DO $$
DECLARE blocked boolean := false;
BEGIN
 BEGIN
  INSERT INTO live_event_registrations(id,event_id,identity_id)
  VALUES('91919191-0000-4000-8000-000000000041',
   '91919191-0000-4000-8000-000000000030','91919191-0000-4000-8000-000000000002');
 EXCEPTION WHEN OTHERS THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'R9 draft allowed registration'; END IF;
END;
$$;
UPDATE live_events SET provider_instance_id='91919191-0000-4000-8000-000000000020',
 provider_proof_checked_at=now(),state='PUBLISHED'
 WHERE id='91919191-0000-4000-8000-000000000030';
INSERT INTO live_event_registrations(id,event_id,identity_id)
 VALUES
 ('91919191-0000-4000-8000-000000000041','91919191-0000-4000-8000-000000000030','91919191-0000-4000-8000-000000000002'),
 ('91919191-0000-4000-8000-000000000042','91919191-0000-4000-8000-000000000030','91919191-0000-4000-8000-000000000003');
-- Cancellation releases a seat, and the same participant can re-register.
UPDATE live_event_registrations SET state='CANCELLED', cancelled_at=now()
 WHERE id='91919191-0000-4000-8000-000000000042';
UPDATE live_event_registrations SET state='REGISTERED',registered_at=now(),cancelled_at=NULL
 WHERE id='91919191-0000-4000-8000-000000000042';
DO $
DECLARE blocked boolean := false;
BEGIN
 BEGIN
  INSERT INTO live_event_registrations(id,event_id,identity_id)
  VALUES('91919191-0000-4000-8000-000000000043',
  '91919191-0000-4000-8000-000000000030','91919191-0000-4000-8000-000000000004');
 EXCEPTION WHEN OTHERS THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'R9 event exceeded capacity'; END IF;
END;
$$;
INSERT INTO live_event_attendance_facts(id,event_id,registration_id,connector_instance_id,provider_event_id,fact_type,occurred_at)
 VALUES('91919191-0000-4000-8000-000000000050',
 '91919191-0000-4000-8000-000000000030',
 '91919191-0000-4000-8000-000000000041',
 '91919191-0000-4000-8000-000000000020','joined-ci-1','JOINED',now());
DO $
DECLARE registered integer; attended integer;
BEGIN
 SELECT count(*) INTO registered FROM live_event_registrations
 WHERE event_id='91919191-0000-4000-8000-000000000030' AND state='REGISTERED';
 SELECT count(DISTINCT registration_id) INTO attended FROM live_event_attendance_facts
 WHERE event_id='91919191-0000-4000-8000-000000000030' AND fact_type='JOINED';
 IF registered <> 2 OR attended <> 1 THEN
  RAISE EXCEPTION 'R9 signed attendance projection incorrect registered=% attended=%', registered, attended;
 END IF;
END;
$;
DO $
DECLARE blocked boolean := false;
BEGIN
 BEGIN
  UPDATE live_events SET state='DRAFT'
   WHERE id='91919191-0000-4000-8000-000000000030';
 EXCEPTION WHEN OTHERS THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'R9 illegal state rollback accepted'; END IF;
 blocked:=false;
 BEGIN
  UPDATE live_event_attendance_facts SET fact_type='LEFT'
   WHERE id='91919191-0000-4000-8000-000000000050';
 EXCEPTION WHEN OTHERS THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'R9 signed attendance is mutable'; END IF;
 blocked:=false;
 BEGIN
  UPDATE live_event_registrations SET state='REGISTERED'
   WHERE id='91919191-0000-4000-8000-000000000041';
 EXCEPTION WHEN OTHERS THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'R9 registration same-state overwrite accepted'; END IF;
END;
$$;
ROLLBACK;
SELECT 'R9 LIVE EVENT SQL INVARIANTS: PASS' AS verdict;
