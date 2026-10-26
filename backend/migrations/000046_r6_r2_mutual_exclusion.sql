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
DECLARE v_state text; v_starts timestamptz; v_ends timestamptz;
BEGIN
  SELECT p.state, s.starts_at, s.ends_at
    INTO v_state, v_starts, v_ends
    FROM booking_slots s
    LEFT JOIN instant_presence p ON p.specialist_identity_id=s.specialist_identity_id
   WHERE s.id=NEW.slot_id;
  IF v_state IN ('OFFERED','RESERVED','IN_CONSULTATION') AND
     v_starts < NEW.created_at + interval '70 minutes' AND v_ends > NEW.created_at THEN
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
      AND h.state='ACTIVE' AND h.expires_at>NEW.updated_at
      AND s.starts_at < NEW.updated_at + interval '70 minutes'
      AND s.ends_at>NEW.updated_at
  ) THEN
    RAISE EXCEPTION 'Duty/Now dispatch conflicts with existing canonical booking hold';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER instant_offer_hold_guard
BEFORE UPDATE OF state ON instant_presence FOR EACH ROW
EXECUTE FUNCTION apgic_instant_offer_hold_guard();
