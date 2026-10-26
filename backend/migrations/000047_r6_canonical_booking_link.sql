-- R6 -> R2 linkage is strictly a reference to canonical booking truth.
-- No payment or room credential is created here.
CREATE TABLE instant_booking_links (
  dispatch_id uuid PRIMARY KEY REFERENCES instant_dispatches(id),
  booking_id uuid NOT NULL UNIQUE REFERENCES bookings(id),
  linked_at timestamptz NOT NULL DEFAULT now()
);

CREATE FUNCTION apgic_instant_booking_link_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE d instant_dispatches%ROWTYPE;
        b bookings%ROWTYPE;
        h booking_holds%ROWTYPE;
        s booking_slots%ROWTYPE;
        o orders%ROWTYPE;
BEGIN
  SELECT * INTO d FROM instant_dispatches WHERE id=NEW.dispatch_id FOR UPDATE;
  SELECT * INTO b FROM bookings WHERE id=NEW.booking_id FOR UPDATE;
  IF d.id IS NULL OR b.id IS NULL OR d.state <> 'ACCEPTED'
     OR b.client_identity_id IS DISTINCT FROM d.client_identity_id
     OR b.state NOT IN ('HELD','PENDING_PAYMENT','CONFIRMED') THEN
    RAISE EXCEPTION 'R6 handoff requires accepted owner-matched canonical R2 booking';
  END IF;
  SELECT * INTO h FROM booking_holds WHERE id=b.hold_id;
  SELECT * INTO s FROM booking_slots WHERE id=b.slot_id;
  IF h.id IS NULL OR s.id IS NULL
    OR h.help_intent_id IS DISTINCT FROM d.help_intent_id
    OR h.specialist_offer_id IS DISTINCT FROM d.offer_id
    OR s.specialist_identity_id IS DISTINCT FROM d.specialist_identity_id
    OR s.specialist_offer_id IS DISTINCT FROM d.offer_id THEN
    RAISE EXCEPTION 'R6 handoff booking provenance differs from accepted offer';
  END IF;
  SELECT * INTO o FROM orders WHERE booking_id=b.id;
  IF o.id IS NOT NULL AND
     (o.amount_minor IS DISTINCT FROM d.price_minor OR o.currency IS DISTINCT FROM d.currency) THEN
    RAISE EXCEPTION 'R6 handoff order changed accepted economic snapshot';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER instant_booking_links_guard
BEFORE INSERT ON instant_booking_links
FOR EACH ROW EXECUTE FUNCTION apgic_instant_booking_link_guard();
CREATE TRIGGER instant_booking_links_append_only
BEFORE UPDATE OR DELETE ON instant_booking_links
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

-- Link operation MUST be invoked by the existing canonical R2 booking owner
-- after it has successfully made the real booking; arbitrary client-supplied
-- booking UUIDs are never accepted by the R6 HTTP boundary.
CREATE FUNCTION apgic_instant_link_booking(p_dispatch uuid,p_booking uuid)
RETURNS text LANGUAGE plpgsql AS $$
DECLARE d instant_dispatches%ROWTYPE;
BEGIN
  SELECT * INTO d FROM instant_dispatches WHERE id=p_dispatch FOR UPDATE;
  IF NOT FOUND OR d.state <> 'ACCEPTED' OR d.expires_at <= clock_timestamp() THEN
    RETURN 'DUTY_HANDOFF_INACTIVE';
  END IF;
  IF EXISTS (SELECT 1 FROM instant_booking_links WHERE dispatch_id=p_dispatch AND booking_id=p_booking) THEN
    RETURN 'DUTY_HANDOFF_REPLAY';
  END IF;
  IF EXISTS (SELECT 1 FROM instant_booking_links WHERE dispatch_id=p_dispatch OR booking_id=p_booking) THEN
    RETURN 'DUTY_HANDOFF_CONFLICT';
  END IF;
  INSERT INTO instant_booking_links(dispatch_id,booking_id) VALUES(p_dispatch,p_booking);
  RETURN 'DUTY_HANDOFF_LINKED';
END;
$$;

-- Canonical captured PSP evidence AND confirmed booking are both required.
-- Fail closed for missing, reversed, mismatched or unsigned evidence.
CREATE FUNCTION apgic_instant_booking_payment_verified(p_booking uuid)
RETURNS boolean LANGUAGE sql STABLE AS $$
  SELECT EXISTS (
    SELECT 1 FROM bookings b
    JOIN orders o ON o.booking_id=b.id
    JOIN payment_attempts a ON a.order_id=o.id
      AND a.amount_minor=o.amount_minor AND a.currency=o.currency
    JOIN payment_effects e ON e.attempt_id=a.id AND e.effect_kind='CAPTURED'
      AND e.ledger_entry_id IS NOT NULL
    JOIN payment_webhook_receipts r ON r.id=e.receipt_id AND r.signature_verified=true
    WHERE b.id=p_booking AND b.state='CONFIRMED'
      AND NOT EXISTS (
        SELECT 1 FROM payment_effects reversal
        WHERE reversal.attempt_id=a.id AND reversal.effect_kind='REFUNDED'
      )
  )
$$;
