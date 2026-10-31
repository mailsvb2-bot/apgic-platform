-- R6 -> canonical R2 prepayment handoff. No competing reservation table,
-- synthetic checkout, PSP receipt or room token. Existing R2 owners persist
-- the slot hold and booking; the R6 quote is linked one-to-one for readback.
--
-- This is a deliberately honest near-term slot fallback. It is not proof
-- of an immediately joinable paid consultation.
CREATE FUNCTION apgic_instant_prepare_canonical_booking(
  p_dispatch uuid, p_client uuid, p_slot uuid,
  p_hold uuid, p_booking uuid, p_now timestamptz
) RETURNS text LANGUAGE plpgsql AS $$
DECLARE
  d instant_dispatches%ROWTYPE;
  s booking_slots%ROWTYPE;
  present instant_presence%ROWTYPE;
  got boolean;
  reason text;
BEGIN
  SELECT * INTO d FROM instant_dispatches WHERE id=p_dispatch FOR UPDATE;
  IF NOT FOUND OR d.client_identity_id IS DISTINCT FROM p_client THEN
    RETURN 'DUTY_NOT_FOUND';
  END IF;
  IF EXISTS (SELECT 1 FROM instant_booking_links WHERE dispatch_id=p_dispatch) THEN
    RETURN 'DUTY_BOOKING_ALREADY_LINKED';
  END IF;
  IF d.state<>'ACCEPTED' OR d.expires_at<=p_now THEN
    RETURN 'DUTY_HANDOFF_INACTIVE';
  END IF;
  IF NOT apgic_live_offer_valid(d.offer_id) THEN
    RETURN 'DUTY_HANDOFF_NOT_ELIGIBLE';
  END IF;
  SELECT * INTO s FROM booking_slots WHERE id=p_slot FOR UPDATE;
  IF NOT FOUND OR s.specialist_identity_id IS DISTINCT FROM d.specialist_identity_id
    OR s.specialist_offer_id IS DISTINCT FROM d.offer_id
    OR s.tenant_scope <> 'specialist/live'
    OR s.exclusive IS DISTINCT FROM true
    OR s.starts_at <= p_now + interval '5 minutes'
    OR s.starts_at > p_now + interval '30 minutes'
    OR s.ends_at <= s.starts_at THEN
    RETURN 'DUTY_HANDOFF_SLOT_UNAVAILABLE';
  END IF;
  SELECT * INTO present FROM instant_presence
    WHERE specialist_identity_id=d.specialist_identity_id FOR UPDATE;
  IF NOT FOUND OR present.state<>'RESERVED' OR present.offer_id IS DISTINCT FROM d.offer_id THEN
    RETURN 'DUTY_HANDOFF_INACTIVE';
  END IF;
  -- Nested transaction rolls back the entire R2 acquisition + Duty release
  -- if any canonical constraint fails, including concurrent hold and write
  -- guard conflicts. No half-booked specialist can remain.
  BEGIN
    UPDATE instant_presence
       SET state='OFFLINE',available_until=NULL,updated_at=p_now,version=version+1
     WHERE specialist_identity_id=d.specialist_identity_id AND state='RESERVED';
    SELECT acquired,reason_code INTO got,reason
      FROM apgic_acquire_slot_hold(
        p_hold,p_slot,p_client,p_now+interval '5 minutes',p_now);
    IF got IS DISTINCT FROM true THEN
      RAISE EXCEPTION 'canonical R2 hold rejected: %',reason;
    END IF;
    UPDATE booking_holds
       SET help_intent_id=d.help_intent_id,specialist_offer_id=d.offer_id
     WHERE id=p_hold AND client_identity_id=p_client AND state='ACTIVE';
    IF NOT FOUND THEN
      RAISE EXCEPTION 'canonical R2 hold lost provenance';
    END IF;
    SELECT created,reason_code INTO got,reason
      FROM apgic_create_booking_from_hold(p_booking,p_hold,p_now);
    IF got IS DISTINCT FROM true THEN
      RAISE EXCEPTION 'canonical R2 booking rejected: %',reason;
    END IF;
    INSERT INTO instant_booking_links(dispatch_id,booking_id)
    VALUES(p_dispatch,p_booking);
  EXCEPTION WHEN OTHERS THEN
    -- This handler is intentionally scoped to the nested block so that
    -- PostgreSQL undoes both the duty-release and any partially acquired R2
    -- rows. The caller sees a retryable conflict, never a false booking.
    RETURN 'DUTY_HANDOFF_CONFLICT';
  END;
  RETURN 'DUTY_BOOKING_LINKED';
END;
$$;

-- Once R2 owns a real booking, R6 cancellation must not silently cancel
-- that booking or imply a refund. The existing R2 owner handles those acts.
CREATE OR REPLACE FUNCTION apgic_instant_cancel(
  p_id uuid, p_client uuid, p_now timestamptz
) RETURNS text LANGUAGE plpgsql AS $$
DECLARE
  selected instant_dispatches%ROWTYPE;
BEGIN
  PERFORM apgic_instant_expire(p_now);
  SELECT * INTO selected FROM instant_dispatches WHERE id=p_id FOR UPDATE;
  IF NOT FOUND OR selected.client_identity_id<>p_client THEN
    RETURN 'DUTY_NOT_FOUND';
  END IF;
  IF EXISTS (SELECT 1 FROM instant_booking_links WHERE dispatch_id=p_id) THEN
    RETURN 'DUTY_ALREADY_BOOKED';
  END IF;
  IF selected.state='CANCELLED' THEN
    RETURN 'DUTY_ALREADY_CANCELLED';
  END IF;
  IF selected.state NOT IN ('OFFERED','ACCEPTED') THEN
    RETURN 'DUTY_NOT_CANCELLABLE';
  END IF;
  UPDATE instant_dispatches
     SET state='CANCELLED',responded_at=COALESCE(responded_at,p_now)
   WHERE id=p_id;
  UPDATE instant_presence p
     SET state=CASE WHEN p.available_until>p_now+interval '90 seconds'
                           AND apgic_instant_supply_eligible(p.offer_id)
                    THEN 'AVAILABLE_NOW' ELSE 'OFFLINE' END,
         available_until=CASE WHEN p.available_until>p_now+interval '90 seconds'
                           AND apgic_instant_supply_eligible(p.offer_id)
                         THEN p.available_until ELSE NULL END,
         updated_at=p_now, version=version+1
   WHERE p.specialist_identity_id=selected.specialist_identity_id
     AND p.offer_id=selected.offer_id AND p.state IN ('OFFERED','RESERVED');
  IF NOT FOUND THEN
    RAISE EXCEPTION 'instant duty presence changed during cancellation';
  END IF;
  RETURN 'DUTY_CANCELLED';
END;
$$;
