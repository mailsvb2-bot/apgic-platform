-- R6/R2 mutual exclusion: canonical booking hold against active instant dispatch.
-- Preserve migration 000008 history; update only the canonical function.
CREATE OR REPLACE FUNCTION apgic_acquire_slot_hold(
  p_hold_id uuid,
  p_slot_id uuid,
  p_client_identity_id uuid,
  p_expires_at timestamptz,
  p_now timestamptz
)
RETURNS TABLE(acquired boolean, reason_code text)
LANGUAGE plpgsql
AS $$
DECLARE
  v_exclusive boolean;
  v_starts_at timestamptz;
  v_ends_at timestamptz;
  v_specialist_id uuid;
  v_duty_state text;
BEGIN
  IF p_hold_id IS NULL OR
     p_slot_id IS NULL OR
     p_client_identity_id IS NULL OR
     p_now IS NULL OR
     p_expires_at IS NULL OR
     p_expires_at <= p_now THEN
    RETURN QUERY SELECT false, 'BOOK_HOLD_INVALID';
    RETURN;
  END IF;

  SELECT exclusive, starts_at, ends_at, specialist_identity_id
  INTO v_exclusive, v_starts_at, v_ends_at, v_specialist_id
  FROM booking_slots
  WHERE id = p_slot_id
  FOR UPDATE;

  IF NOT FOUND THEN
    RETURN QUERY SELECT false, 'BOOK_SLOT_NOT_FOUND';
    RETURN;
  END IF;

  IF NOT v_exclusive THEN
    RETURN QUERY SELECT false, 'BOOK_SLOT_NOT_EXCLUSIVE';
    RETURN;
  END IF;

  IF p_expires_at >= v_starts_at THEN
    RETURN QUERY SELECT false, 'BOOK_HOLD_INVALID';
    RETURN;
  END IF;

  IF v_starts_at <= p_now THEN
    RETURN QUERY SELECT false, 'BOOK_SLOT_NOT_AVAILABLE';
    RETURN;
  END IF;

  -- Serialize canonical R2 holds with R6 dispatch's FOR UPDATE OF presence.
  -- A queued ordinary booking must not consume a specialist already promised
  -- to an active Duty/Now request. No separate booking/ledger is created.
  SELECT p.state INTO v_duty_state
    FROM instant_presence p
   WHERE p.specialist_identity_id=v_specialist_id
   FOR UPDATE;
  IF v_duty_state IN ('OFFERED','RESERVED','IN_CONSULTATION') AND
     v_starts_at < p_now + interval '70 minutes' AND v_ends_at > p_now THEN
    RETURN QUERY SELECT false, 'BOOK_SLOT_DUTY_BUSY';
    RETURN;
  END IF;

  UPDATE booking_holds
  SET state = 'EXPIRED',
      updated_at = p_now
  WHERE slot_id = p_slot_id
    AND state = 'ACTIVE'
    AND expires_at <= p_now;

  IF EXISTS (
    SELECT 1
    FROM bookings
    WHERE slot_id = p_slot_id
      AND state IN ('HELD','PENDING_PAYMENT','CONFIRMED')
  ) THEN
    RETURN QUERY SELECT false, 'BOOK_SLOT_BOOKED';
    RETURN;
  END IF;

  IF EXISTS (
    SELECT 1
    FROM booking_holds
    WHERE slot_id = p_slot_id
      AND state = 'ACTIVE'
  ) THEN
    RETURN QUERY SELECT false, 'BOOK_SLOT_HELD';
    RETURN;
  END IF;

  INSERT INTO booking_holds (
    id,
    slot_id,
    client_identity_id,
    state,
    expires_at,
    created_at,
    updated_at
  ) VALUES (
    p_hold_id,
    p_slot_id,
    p_client_identity_id,
    'ACTIVE',
    p_expires_at,
    p_now,
    p_now
  );

  RETURN QUERY SELECT true, 'BOOK_HOLD_ACQUIRED';
END;
$$;

-- Defense in depth: reject writes that bypass the canonical hold function.
-- The existing slot-insert guard runs independently; this checks the other
-- owner's live duty commitment under the same presence row lock.
CREATE FUNCTION apgic_booking_hold_duty_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE v_state text; v_starts timestamptz; v_ends timestamptz; v_specialist uuid;
BEGIN
  SELECT s.starts_at,s.ends_at,s.specialist_identity_id
    INTO v_starts,v_ends,v_specialist
    FROM booking_slots s WHERE s.id=NEW.slot_id;
  SELECT p.state INTO v_state FROM instant_presence p
   WHERE p.specialist_identity_id=v_specialist FOR UPDATE;
  IF v_state IN ('OFFERED','RESERVED','IN_CONSULTATION') AND
     v_starts < clock_timestamp() + interval '70 minutes' AND v_ends > clock_timestamp() THEN
    RAISE EXCEPTION 'booking hold conflicts with active Duty/Now offer';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER booking_holds_duty_guard
BEFORE INSERT ON booking_holds FOR EACH ROW
EXECUTE FUNCTION apgic_booking_hold_duty_guard();

-- R6 takes the presence lock before changing AVAILABLE_NOW -> OFFERED.
-- A pre-existing active R2 hold for the immediate service window must stop
-- dispatch even if its booking row has not been created yet.
CREATE FUNCTION apgic_instant_offer_hold_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.state='OFFERED' AND OLD.state IS DISTINCT FROM 'OFFERED' AND EXISTS (
    SELECT 1 FROM booking_holds h
    JOIN booking_slots s ON s.id=h.slot_id
    WHERE s.specialist_identity_id=NEW.specialist_identity_id
      AND h.state='ACTIVE' AND h.expires_at>clock_timestamp()
      AND s.starts_at < clock_timestamp() + interval '70 minutes'
      AND s.ends_at>clock_timestamp()
  ) THEN
    RAISE EXCEPTION 'Duty/Now dispatch conflicts with existing canonical booking hold';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER instant_offer_hold_guard
BEFORE UPDATE OF state ON instant_presence FOR EACH ROW
EXECUTE FUNCTION apgic_instant_offer_hold_guard();

-- Check both directions before making a specialist appear available;
-- a canonical R2 hold is a real commitment before booking persistence.
CREATE OR REPLACE FUNCTION apgic_instant_enable(p_identity uuid,p_offer uuid,p_available boolean,p_now timestamptz)
RETURNS text LANGUAGE plpgsql AS $$
DECLARE existing instant_presence%ROWTYPE;
BEGIN
  PERFORM apgic_instant_expire(p_now);
  SELECT * INTO existing FROM instant_presence WHERE specialist_identity_id=p_identity FOR UPDATE;
  IF FOUND AND existing.state IN ('OFFERED','RESERVED','IN_CONSULTATION') THEN
    RETURN 'DUTY_BUSY';
  END IF;
  IF NOT p_available THEN
    UPDATE instant_presence SET state='OFFLINE',available_until=NULL,updated_at=p_now,version=version+1
      WHERE specialist_identity_id=p_identity;
    RETURN 'DUTY_OFFLINE';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM specialist_bookable_offers o
       WHERE o.id=p_offer AND o.specialist_identity_id=p_identity AND apgic_instant_supply_eligible(o.id)) THEN
    RETURN 'DUTY_NOT_ELIGIBLE';
  END IF;
  -- Do not offer a specialist who has an overlapping scheduled booking:
  -- one owner of time, irrespective of the scheduled or instant sales route.
  IF EXISTS (
    SELECT 1 FROM bookings b
    JOIN booking_slots slot ON slot.id=b.slot_id
    WHERE slot.specialist_identity_id=p_identity
      AND b.state IN ('HELD','PENDING_PAYMENT','CONFIRMED')
      AND b.starts_at < p_now + interval '70 minutes'
      AND b.ends_at > p_now
  ) THEN
    RETURN 'DUTY_SCHEDULE_CONFLICT';
  END IF;
  -- An active R2 hold has not necessarily produced its booking row yet.
  IF EXISTS (
    SELECT 1 FROM booking_holds h JOIN booking_slots slot ON slot.id=h.slot_id
    WHERE slot.specialist_identity_id=p_identity AND h.state='ACTIVE'
      AND h.expires_at>p_now
      AND slot.starts_at < p_now + interval '70 minutes'
      AND slot.ends_at>p_now
  ) THEN
    RETURN 'DUTY_SCHEDULE_CONFLICT';
  END IF;
  -- A specialist chooses one priced offer for the current duty window.
  INSERT INTO instant_presence(specialist_identity_id,offer_id,state,available_until,updated_at)
  VALUES(p_identity,p_offer,'AVAILABLE_NOW',p_now + interval '10 minutes',p_now)
  ON CONFLICT (specialist_identity_id) DO UPDATE
    SET offer_id=EXCLUDED.offer_id,state='AVAILABLE_NOW',
        available_until=EXCLUDED.available_until,updated_at=p_now,
        version=instant_presence.version+1;
  RETURN 'DUTY_AVAILABLE_NOW';
END;
$$;
CREATE OR REPLACE FUNCTION apgic_instant_request(p_id uuid,p_key uuid,p_identity uuid,p_intent uuid,p_now timestamptz)
RETURNS text LANGUAGE plpgsql AS $$
DECLARE selected record; active_id uuid; old_id uuid;
BEGIN
  PERFORM pg_advisory_xact_lock(hashtextextended('apgic:duty:client:' || p_identity::text,0));
  PERFORM apgic_instant_expire(p_now);
  SELECT id INTO old_id FROM instant_dispatches
    WHERE client_identity_id=p_identity AND request_key=p_key;
  IF FOUND THEN
    IF NOT EXISTS (SELECT 1 FROM instant_dispatches
      WHERE id=old_id AND help_intent_id=p_intent) THEN
      RETURN 'DUTY_IDEMPOTENCY_CONFLICT';
    END IF;
    RETURN 'DUTY_IDEMPOTENT_REPLAY';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM help_intents h WHERE h.id=p_intent
    AND h.identity_id=p_identity AND h.state='CONFIRMED'
    AND cardinality(h.topics)>0
    AND h.context->>'format'='ONLINE'
    AND h.context->>'jurisdiction'='RU'
    AND h.context->>'age_group'='ADULT'
    AND h.context->>'language'='ru') THEN
    RETURN 'DUTY_INTENT_NOT_ELIGIBLE';
  END IF;
  SELECT id INTO active_id FROM instant_dispatches
    WHERE client_identity_id=p_identity AND state IN ('OFFERED','ACCEPTED');
  IF active_id IS NOT NULL THEN RETURN 'DUTY_ALREADY_ACTIVE'; END IF;

  SELECT p.specialist_identity_id,p.offer_id,o.topic_id,o.price_minor,o.currency
    INTO selected
    FROM instant_presence p
    JOIN specialist_bookable_offers o ON o.id=p.offer_id
    JOIN help_intents h ON h.id=p_intent
    JOIN specialist_profiles sp ON sp.id=o.specialist_id
    JOIN LATERAL (
      SELECT q.decision FROM qualification_evaluations q
       WHERE q.specialist_id=o.specialist_id AND q.topic_id=o.topic_id
         AND q.jurisdiction=h.context->>'jurisdiction'
         AND q.service_format=h.context->>'format'
         AND q.age_group=h.context->>'age_group'
       ORDER BY q.evaluated_at DESC,q.id DESC LIMIT 1
    ) qualification ON qualification.decision='ELIGIBLE'
    WHERE p.state='AVAILABLE_NOW' AND p.available_until>p_now + interval '90 seconds'
      AND apgic_instant_supply_eligible(o.id)
      AND o.topic_id=ANY(h.topics)
      AND p.specialist_identity_id<>p_identity
      AND NOT EXISTS (
        SELECT 1 FROM bookings b
        JOIN booking_slots scheduled_slot ON scheduled_slot.id=b.slot_id
        WHERE scheduled_slot.specialist_identity_id=p.specialist_identity_id
          AND b.state IN ('HELD','PENDING_PAYMENT','CONFIRMED')
          AND b.starts_at < p_now + interval '70 minutes'
          AND b.ends_at > p_now
      )
      AND NOT EXISTS (
        SELECT 1 FROM booking_holds h JOIN booking_slots hold_slot ON hold_slot.id=h.slot_id
        WHERE hold_slot.specialist_identity_id=p.specialist_identity_id
          AND h.state='ACTIVE' AND h.expires_at>p_now
          AND hold_slot.starts_at < p_now + interval '70 minutes'
          AND hold_slot.ends_at>p_now
      )
      -- A user who explicitly cancelled may choose to try again.
      -- Never silently re-offer a practitioner who declined, missed the
      -- deadline, or became ineligible during this HelpIntent.
      AND NOT EXISTS (SELECT 1 FROM instant_dispatches tried
        WHERE tried.help_intent_id=p_intent AND tried.specialist_identity_id=p.specialist_identity_id
          AND tried.state IN ('DECLINED','EXPIRED','OFFERED','ACCEPTED','INVALIDATED'))
    ORDER BY p.updated_at,p.specialist_identity_id
    FOR UPDATE OF p SKIP LOCKED LIMIT 1;
  IF NOT FOUND THEN
    INSERT INTO instant_dispatches(id,client_identity_id,request_key,help_intent_id,state)
    VALUES(p_id,p_identity,p_key,p_intent,'UNFILLED');
    RETURN 'DUTY_NO_SUPPLY';
  END IF;
  UPDATE instant_presence SET state='OFFERED',updated_at=p_now,version=version+1
    WHERE specialist_identity_id=selected.specialist_identity_id;
  INSERT INTO instant_dispatches
    (id,client_identity_id,request_key,help_intent_id,specialist_identity_id,
     offer_id,topic_id,price_minor,currency,state,offered_at,expires_at)
  VALUES (p_id,p_identity,p_key,p_intent,selected.specialist_identity_id,
     selected.offer_id,selected.topic_id,selected.price_minor,selected.currency,
     'OFFERED',p_now,p_now+interval '45 seconds');
  RETURN 'DUTY_OFFERED';
END;
$$;
