-- R6 Duty/Now: APGIC-owned ephemeral availability and durable dispatch truth.
-- Real Payment/Booking/Communication remain canonical and are NOT fabricated here.
CREATE TABLE instant_presence (
  specialist_identity_id uuid PRIMARY KEY REFERENCES identities(id),
  offer_id uuid NOT NULL REFERENCES specialist_bookable_offers(id),
  state text NOT NULL CHECK (state IN ('OFFLINE','AVAILABLE_NOW','OFFERED','RESERVED','IN_CONSULTATION','COOLDOWN')),
  available_until timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  version bigint NOT NULL DEFAULT 1,
  CHECK ((state = 'OFFLINE' AND available_until IS NULL)
    OR (state <> 'OFFLINE' AND available_until IS NOT NULL))
);

CREATE INDEX instant_presence_available_idx ON instant_presence (updated_at, specialist_identity_id)
  WHERE state = 'AVAILABLE_NOW';

CREATE TABLE instant_dispatches (
  id uuid PRIMARY KEY,
  client_identity_id uuid NOT NULL REFERENCES identities(id),
  request_key uuid NOT NULL,
  help_intent_id uuid NOT NULL REFERENCES help_intents(id),
  specialist_identity_id uuid REFERENCES identities(id),
  offer_id uuid REFERENCES specialist_bookable_offers(id),
  topic_id text,
  price_minor bigint,
  currency text,
  state text NOT NULL CHECK (state IN ('UNFILLED','OFFERED','DECLINED','EXPIRED','ACCEPTED','CANCELLED','INVALIDATED')),
  policy_version text NOT NULL DEFAULT 'duty-dispatch-v1',
  offered_at timestamptz,
  expires_at timestamptz,
  responded_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (client_identity_id, request_key),
  CHECK ((state = 'UNFILLED' AND specialist_identity_id IS NULL AND offer_id IS NULL
    AND offered_at IS NULL AND expires_at IS NULL AND price_minor IS NULL AND currency IS NULL AND topic_id IS NULL)
  OR (state <> 'UNFILLED' AND specialist_identity_id IS NOT NULL AND offer_id IS NOT NULL
    AND offered_at IS NOT NULL AND expires_at IS NOT NULL AND price_minor > 0 AND currency = 'RUB'
    AND topic_id IS NOT NULL)),
  CHECK (expires_at IS NULL OR expires_at > offered_at)
);

CREATE UNIQUE INDEX instant_one_active_dispatch_per_client ON instant_dispatches(client_identity_id)
  WHERE state IN ('OFFERED','ACCEPTED');
CREATE UNIQUE INDEX instant_one_active_dispatch_per_specialist ON instant_dispatches(specialist_identity_id)
  WHERE state IN ('OFFERED','ACCEPTED');
CREATE INDEX instant_dispatch_specialist_inbox ON instant_dispatches(specialist_identity_id, created_at DESC)
  WHERE state IN ('OFFERED','ACCEPTED');

-- Duty/Now has a narrower real-time scope than the general marketplace:
-- online, Russian jurisdiction, adult clients. A historic ELIGIBLE verdict
-- is insufficient if a later decision is MANUAL_REVIEW or INELIGIBLE.
CREATE FUNCTION apgic_instant_supply_eligible(p_offer_id uuid)
RETURNS boolean LANGUAGE sql VOLATILE AS $$
  SELECT EXISTS (
    SELECT 1 FROM specialist_bookable_offers o
    JOIN LATERAL (
      SELECT q.decision FROM qualification_evaluations q
       WHERE q.specialist_id=o.specialist_id AND q.topic_id=o.topic_id
         AND q.jurisdiction='RU' AND q.service_format='ONLINE'
         AND q.age_group='ADULT'
       ORDER BY q.evaluated_at DESC,q.id DESC LIMIT 1
    ) qualification ON qualification.decision='ELIGIBLE'
    WHERE o.id=p_offer_id AND apgic_live_offer_valid(o.id)
  )
$$;

-- At all entrypoints, release timed-out offers; never leave a busy specialist
-- forever or expose an expired offer as available. An ACCEPTED dispatch is a
-- prepayment agreement, not a consultation, and expires if not fulfilled.
CREATE FUNCTION apgic_instant_expire(p_now timestamptz)
RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  WITH expired AS (
    UPDATE instant_dispatches
       SET state='EXPIRED'
     WHERE state IN ('OFFERED','ACCEPTED') AND expires_at <= p_now
     RETURNING specialist_identity_id, offer_id
  )
  UPDATE instant_presence p
     SET state = CASE WHEN p.available_until > p_now THEN 'AVAILABLE_NOW' ELSE 'OFFLINE' END,
         available_until = CASE WHEN p.available_until > p_now THEN p.available_until ELSE NULL END,
         updated_at=p_now,version=version+1
    FROM expired d
   WHERE p.specialist_identity_id=d.specialist_identity_id
     AND p.offer_id=d.offer_id
     AND p.state IN ('OFFERED','RESERVED');

  -- A previous successful evaluation is not a permanent entitlement.
  -- Revoked publications, Product ownership and later MANUAL_REVIEW decisions
  -- terminate even already accepted but unpaid offers on the next read.
  WITH invalidated AS (
    UPDATE instant_dispatches d SET state='INVALIDATED'
    WHERE d.state IN ('OFFERED','ACCEPTED')
      AND NOT EXISTS (
        SELECT 1 FROM specialist_bookable_offers o
        JOIN help_intents h ON h.id=d.help_intent_id
        JOIN LATERAL (
          SELECT q.decision FROM qualification_evaluations q
           WHERE q.specialist_id=o.specialist_id AND q.topic_id=o.topic_id
             AND q.jurisdiction=h.context->>'jurisdiction'
             AND q.service_format=h.context->>'format'
             AND q.age_group=h.context->>'age_group'
           ORDER BY q.evaluated_at DESC,q.id DESC LIMIT 1
        ) qualification ON qualification.decision='ELIGIBLE'
        WHERE o.id=d.offer_id AND apgic_live_offer_valid(o.id)
          AND o.topic_id=ANY(h.topics) AND h.state='CONFIRMED'
      )
    RETURNING d.specialist_identity_id,d.offer_id
  )
  UPDATE instant_presence p
     SET state='OFFLINE',available_until=NULL,updated_at=p_now,version=version+1
    FROM invalidated d
   WHERE p.specialist_identity_id=d.specialist_identity_id
     AND p.offer_id=d.offer_id
     AND p.state IN ('OFFERED','RESERVED');

  UPDATE instant_presence p
     SET state='OFFLINE',available_until=NULL,
         updated_at=p_now,version=version+1
  WHERE p.state IN ('AVAILABLE_NOW','COOLDOWN')
    AND (p.available_until <= p_now OR NOT apgic_instant_supply_eligible(p.offer_id));
END;
$$;

-- Eligibility remains tied to canonical published priced offer and
-- specialist qualification. Distinct from mere self-declared availability.
CREATE FUNCTION apgic_instant_enable(p_identity uuid,p_offer uuid,p_available boolean,p_now timestamptz)
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

-- Immutable snapshot of which offer the client accepted to be offered.
-- Qualification is evaluated for the confirmed intent's explicit context.
CREATE FUNCTION apgic_instant_request(p_id uuid,p_key uuid,p_identity uuid,p_intent uuid,p_now timestamptz)
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

CREATE FUNCTION apgic_instant_answer(p_id uuid,p_specialist uuid,p_accept boolean,p_now timestamptz)
RETURNS text LANGUAGE plpgsql AS $$
DECLARE current_dispatch instant_dispatches%ROWTYPE;
BEGIN
  PERFORM apgic_instant_expire(p_now);
  SELECT * INTO current_dispatch FROM instant_dispatches WHERE id=p_id FOR UPDATE;
  IF NOT FOUND OR current_dispatch.specialist_identity_id<>p_specialist THEN
    RETURN 'DUTY_NOT_FOUND';
  END IF;
  IF current_dispatch.state='ACCEPTED' AND p_accept THEN
    RETURN 'DUTY_ALREADY_ACCEPTED';
  END IF;
  IF current_dispatch.state<>'OFFERED' THEN RETURN 'DUTY_OFFER_NOT_ACTIVE'; END IF;
  IF current_dispatch.expires_at<=p_now THEN RETURN 'DUTY_OFFER_NOT_ACTIVE'; END IF;
  -- Recheck the current qualification decision at acceptance, not merely
  -- the old Product/publication approval. A later MANUAL_REVIEW or revocation
  -- must not turn a stale OFFERED badge into a payable agreement.
  IF p_accept AND NOT EXISTS (
    SELECT 1
      FROM specialist_bookable_offers o
      JOIN help_intents h ON h.id=current_dispatch.help_intent_id
      JOIN LATERAL (
        SELECT q.decision FROM qualification_evaluations q
         WHERE q.specialist_id=o.specialist_id AND q.topic_id=o.topic_id
           AND q.jurisdiction=h.context->>'jurisdiction'
           AND q.service_format=h.context->>'format'
           AND q.age_group=h.context->>'age_group'
         ORDER BY q.evaluated_at DESC,q.id DESC LIMIT 1
      ) qualification ON qualification.decision='ELIGIBLE'
     WHERE o.id=current_dispatch.offer_id
       AND o.specialist_identity_id=p_specialist
       AND o.topic_id=ANY(h.topics)
       AND h.state='CONFIRMED'
       AND apgic_live_offer_valid(o.id)
  ) THEN
    RETURN 'DUTY_NOT_ELIGIBLE';
  END IF;
  IF p_accept AND EXISTS (
    SELECT 1 FROM bookings b JOIN booking_slots slot ON slot.id=b.slot_id
    WHERE slot.specialist_identity_id=p_specialist
      AND b.state IN ('HELD','PENDING_PAYMENT','CONFIRMED')
      AND b.starts_at < p_now + interval '70 minutes'
      AND b.ends_at > p_now
  ) THEN
    RETURN 'DUTY_SCHEDULE_CONFLICT';
  END IF;
  UPDATE instant_dispatches
   SET state=CASE WHEN p_accept THEN 'ACCEPTED' ELSE 'DECLINED' END,
       responded_at=p_now,
       expires_at=CASE WHEN p_accept THEN p_now+interval '10 minutes' ELSE expires_at END
   WHERE id=p_id;
  UPDATE instant_presence SET state=CASE WHEN p_accept THEN 'RESERVED' ELSE 'COOLDOWN' END,
       updated_at=p_now,version=version+1
   WHERE specialist_identity_id=p_specialist AND offer_id=current_dispatch.offer_id
     AND state='OFFERED';
  IF NOT FOUND THEN RAISE EXCEPTION 'instant duty presence changed during answer'; END IF;
  RETURN CASE WHEN p_accept THEN 'DUTY_ACCEPTED_PAYMENT_REQUIRED' ELSE 'DUTY_DECLINED' END;
END;
$$;

-- A signed client's explicit cancellation is a terminal and idempotent
-- transition: release the specialist without creating booking/payment.
-- The prepayment offer cannot be resurrected by a late specialist response.
CREATE FUNCTION apgic_instant_cancel(
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
  IF selected.state='CANCELLED' THEN
    RETURN 'DUTY_ALREADY_CANCELLED';
  END IF;
  IF selected.state NOT IN ('OFFERED','ACCEPTED') THEN
    RETURN 'DUTY_NOT_CANCELLABLE';
  END IF;
  UPDATE instant_dispatches
     SET state='CANCELLED', responded_at=COALESCE(responded_at,p_now)
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
     AND p.offer_id=selected.offer_id
     AND p.state IN ('OFFERED','RESERVED');
  IF NOT FOUND THEN
    RAISE EXCEPTION 'instant duty presence changed during cancellation';
  END IF;
  RETURN 'DUTY_CANCELLED';
END;
$$;
