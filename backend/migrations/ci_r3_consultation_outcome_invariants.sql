\set ON_ERROR_STOP on

-- The preceding ci_r3_communication_consultation_invariants.sql created a
-- provider-evidenced COMPLETED consultation c012. A separate booking d012
-- exercises the still-open Ready Check window without synthetic JOINED facts.

INSERT INTO booking_slots(id, specialist_identity_id,tenant_scope, starts_at, ends_at, exclusive)
VALUES('00000000-0000-0000-0000-00000000d010','00000000-0000-0000-0000-00000000b001',
       'tenant/r3-ready-ci',now()+interval '10 minutes',now()+interval '40 minutes',true);

SELECT * FROM apgic_acquire_slot_hold(
  '00000000-0000-0000-0000-00000000d011',
  '00000000-0000-0000-0000-00000000d010',
  '00000000-0000-0000-0000-00000000b002',
  now()+interval '5 minutes',now());

SELECT * FROM apgic_create_booking_from_hold(
  '00000000-0000-0000-0000-00000000d012',
  '00000000-0000-0000-0000-00000000d011',
  now()+interval '1 minute');

UPDATE bookings SET state='CONFIRMED',updated_at=now()+interval '2 minutes'
 WHERE id='00000000-0000-0000-0000-00000000d012';

-- Positive path: both authenticated canonical identities, but no provider
-- attendance or payment mutations.
INSERT INTO consultation_participant_readiness (booking_id,identity_id,role,ready_at)
VALUES
('00000000-0000-0000-0000-00000000d012','00000000-0000-0000-0000-00000000b002','CLIENT',now()),
('00000000-0000-0000-0000-00000000d012','00000000-0000-0000-0000-00000000b001','SPECIALIST',now());

INSERT INTO consultation_participant_readiness (booking_id,identity_id,role,ready_at)
VALUES ('00000000-0000-0000-0000-00000000d012','00000000-0000-0000-0000-00000000b002','CLIENT',now())
ON CONFLICT (booking_id,identity_id) DO NOTHING;

DO $$
DECLARE
  blocked boolean;
BEGIN
  IF (SELECT COUNT(*) FROM consultation_participant_readiness
      WHERE booking_id='00000000-0000-0000-0000-00000000d012')<>2 THEN
    RAISE EXCEPTION 'R3 ready check not durable/idempotent';
  END IF;
  IF EXISTS (SELECT 1 FROM consultation_sessions
             WHERE booking_id='00000000-0000-0000-0000-00000000d012') THEN
    RAISE EXCEPTION 'ready check fabricated provider lifecycle';
  END IF;
  blocked:=false;
  BEGIN
    INSERT INTO consultation_participant_readiness (booking_id,identity_id,role,ready_at)
    VALUES ('00000000-0000-0000-0000-00000000d012',
            '00000000-0000-0000-0000-00000000b003','CLIENT',now());
  EXCEPTION WHEN OTHERS THEN blocked:=true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'foreign readiness impersonation accepted'; END IF;

  blocked:=false;
  BEGIN
    UPDATE consultation_participant_readiness SET ready_at=now()
     WHERE booking_id='00000000-0000-0000-0000-00000000d012';
  EXCEPTION WHEN OTHERS THEN blocked:=true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'ready check was mutable'; END IF;

  blocked:=false;
  BEGIN
    INSERT INTO consultation_client_outcomes(id,booking_id,client_identity_id,idempotency_key,
      progress,match_satisfaction,reported_at)
    VALUES ('00000000-0000-0000-0000-00000000d020',
      '00000000-0000-0000-0000-00000000d012',
      '00000000-0000-0000-0000-00000000b002',
      'outcome-not-completed-ci',1,5,now());
  EXCEPTION WHEN OTHERS THEN blocked:=true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'client outcome without provider completion accepted'; END IF;
END
$$;

INSERT INTO consultation_client_outcomes(id,booking_id,client_identity_id,idempotency_key,
    progress,match_satisfaction,reported_at)
VALUES('00000000-0000-0000-0000-00000000d021',
  '00000000-0000-0000-0000-00000000c012',
  '00000000-0000-0000-0000-00000000b002',
  'outcome-completed-ci-key',1,5,now());

INSERT INTO consultation_client_outcomes(id,booking_id,client_identity_id,idempotency_key,
    progress,match_satisfaction,reported_at)
VALUES('00000000-0000-0000-0000-00000000d022',
  '00000000-0000-0000-0000-00000000c012',
  '00000000-0000-0000-0000-00000000b002',
  'outcome-completed-ci-key',1,5,now())
ON CONFLICT(booking_id,idempotency_key) DO NOTHING;

DO $$
DECLARE blocked boolean;
BEGIN
  IF (SELECT COUNT(*) FROM consultation_client_outcomes
      WHERE booking_id='00000000-0000-0000-0000-00000000c012')<>1 THEN
    RAISE EXCEPTION 'client outcome not idempotent';
  END IF;
  blocked:=false;
  BEGIN
    INSERT INTO consultation_client_outcomes(id,booking_id,client_identity_id,idempotency_key,
        progress,match_satisfaction,reported_at)
    VALUES('00000000-0000-0000-0000-00000000d023',
      '00000000-0000-0000-0000-00000000c012',
      '00000000-0000-0000-0000-00000000b001',
      'outcome-forged-role-ci',2,4,now());
  EXCEPTION WHEN OTHERS THEN blocked:=true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'specialist forged client outcome'; END IF;

  blocked:=false;
  BEGIN
    INSERT INTO consultation_client_outcomes(id,booking_id,client_identity_id,idempotency_key,
        progress,match_satisfaction,reported_at)
    VALUES('00000000-0000-0000-0000-00000000d024',
      '00000000-0000-0000-0000-00000000c012',
      '00000000-0000-0000-0000-00000000b002',
      'outcome-out-of-range-ci',4,4,now());
  EXCEPTION WHEN OTHERS THEN blocked:=true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'out of range progress accepted'; END IF;

  blocked:=false;
  BEGIN
    UPDATE consultation_client_outcomes SET progress=-2
     WHERE id='00000000-0000-0000-0000-00000000d021';
  EXCEPTION WHEN OTHERS THEN blocked:=true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'client outcome could be rewritten'; END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name IN ('consultation_participant_readiness','consultation_client_outcomes')
      AND column_name IN ('transcript','recording','raw_audio','raw_video','diagnosis','client_notes')
  ) THEN RAISE EXCEPTION 'raw consultation content leaked into outcome schema'; END IF;
END
$$;
