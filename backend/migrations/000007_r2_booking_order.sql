BEGIN;

CREATE TABLE booking_slots (
  id uuid PRIMARY KEY,
  specialist_identity_id uuid NOT NULL REFERENCES identities(id),
  tenant_scope text NOT NULL CHECK (btrim(tenant_scope) <> ''),
  starts_at timestamptz NOT NULL,
  ends_at timestamptz NOT NULL,
  exclusive boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (ends_at > starts_at)
);

CREATE TABLE booking_holds (
  id uuid PRIMARY KEY,
  slot_id uuid NOT NULL REFERENCES booking_slots(id),
  client_identity_id uuid NOT NULL REFERENCES identities(id),
  state text NOT NULL CHECK (state IN ('ACTIVE','CONSUMED','EXPIRED','RELEASED')),
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CHECK (expires_at > created_at)
);

CREATE UNIQUE INDEX booking_holds_one_active_per_slot
  ON booking_holds (slot_id)
  WHERE state = 'ACTIVE';

CREATE INDEX booking_holds_slot_state_idx
  ON booking_holds (slot_id, state, expires_at);

CREATE OR REPLACE FUNCTION apgic_booking_hold_transition_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.slot_id <> OLD.slot_id OR NEW.client_identity_id <> OLD.client_identity_id THEN
    RAISE EXCEPTION 'booking hold ownership is immutable';
  END IF;
  IF NEW.expires_at <> OLD.expires_at OR NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'booking hold timing is immutable';
  END IF;
  IF NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'booking hold updated_at cannot move backwards';
  END IF;
  IF OLD.state <> NEW.state THEN
    IF OLD.state = 'ACTIVE' AND NEW.state IN ('CONSUMED','EXPIRED','RELEASED') THEN
      NULL;
    ELSE
      RAISE EXCEPTION 'invalid booking hold transition: % -> %', OLD.state, NEW.state;
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION apgic_booking_hold_insert_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  slot_start timestamptz;
  slot_exclusive boolean;
BEGIN
  SELECT starts_at, exclusive
  INTO slot_start, slot_exclusive
  FROM booking_slots
  WHERE id = NEW.slot_id;

  IF NOT FOUND OR NOT slot_exclusive THEN
    RAISE EXCEPTION 'booking hold requires an exclusive canonical slot';
  END IF;
  IF NEW.expires_at <= NEW.created_at OR NEW.expires_at >= slot_start THEN
    RAISE EXCEPTION 'booking hold expiry must be before slot start';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER booking_holds_insert_guard
BEFORE INSERT ON booking_holds
FOR EACH ROW EXECUTE FUNCTION apgic_booking_hold_insert_guard();

CREATE TRIGGER booking_holds_transition_guard
BEFORE UPDATE ON booking_holds
FOR EACH ROW EXECUTE FUNCTION apgic_booking_hold_transition_guard();

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

  SELECT exclusive, starts_at
  INTO v_exclusive, v_starts_at
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

CREATE TABLE bookings (
  id uuid PRIMARY KEY,
  slot_id uuid NOT NULL REFERENCES booking_slots(id),
  hold_id uuid NOT NULL UNIQUE REFERENCES booking_holds(id),
  client_identity_id uuid NOT NULL REFERENCES identities(id),
  state text NOT NULL CHECK (
    state IN (
      'HELD',
      'PENDING_PAYMENT',
      'CONFIRMED',
      'CANCELLED',
      'EXPIRED',
      'COMPLETED',
      'NO_SHOW'
    )
  ),
  hold_expires_at timestamptz NOT NULL,
  starts_at timestamptz NOT NULL,
  ends_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CHECK (ends_at > starts_at),
  CHECK (hold_expires_at > created_at)
);

CREATE UNIQUE INDEX bookings_one_live_per_slot
  ON bookings (slot_id)
  WHERE state IN ('HELD','PENDING_PAYMENT','CONFIRMED');

CREATE INDEX bookings_client_state_idx
  ON bookings (client_identity_id, state, starts_at);

CREATE OR REPLACE FUNCTION apgic_booking_transition_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.slot_id <> OLD.slot_id OR
     NEW.hold_id <> OLD.hold_id OR
     NEW.client_identity_id <> OLD.client_identity_id OR
     NEW.hold_expires_at <> OLD.hold_expires_at OR
     NEW.starts_at <> OLD.starts_at OR
     NEW.ends_at <> OLD.ends_at OR
     NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'booking canonical identity/timing is immutable';
  END IF;

  IF NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'booking updated_at cannot move backwards';
  END IF;

  IF OLD.state = NEW.state THEN
    RETURN NEW;
  END IF;

  IF OLD.state IN ('HELD','PENDING_PAYMENT')
    AND NEW.state <> 'EXPIRED'
    AND NEW.updated_at >= OLD.hold_expires_at THEN
    RAISE EXCEPTION 'booking hold expired before transition';
  END IF;

  IF NEW.state = 'EXPIRED'
    AND (
      OLD.state NOT IN ('HELD','PENDING_PAYMENT')
      OR NEW.updated_at < OLD.hold_expires_at
    ) THEN
    RAISE EXCEPTION 'booking cannot expire before hold expiry';
  END IF;

  IF NEW.state = 'NO_SHOW'
    AND (
      OLD.state <> 'CONFIRMED'
      OR NEW.updated_at < OLD.starts_at
    ) THEN
    RAISE EXCEPTION 'booking cannot become NO_SHOW before start';
  END IF;

  IF NEW.state = 'COMPLETED'
    AND (
      OLD.state <> 'CONFIRMED'
      OR NEW.updated_at < OLD.ends_at
    ) THEN
    RAISE EXCEPTION 'booking cannot complete before end';
  END IF;

  IF OLD.state = 'HELD'
    AND NEW.state IN ('PENDING_PAYMENT','CONFIRMED','CANCELLED','EXPIRED') THEN
    RETURN NEW;
  ELSIF OLD.state = 'PENDING_PAYMENT'
    AND NEW.state IN ('CONFIRMED','CANCELLED','EXPIRED') THEN
    RETURN NEW;
  ELSIF OLD.state = 'CONFIRMED'
    AND NEW.state IN ('CANCELLED','COMPLETED','NO_SHOW') THEN
    RETURN NEW;
  END IF;

  RAISE EXCEPTION 'invalid booking transition: % -> %', OLD.state, NEW.state;
END;
$$;

CREATE OR REPLACE FUNCTION apgic_booking_insert_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  hold booking_holds%ROWTYPE;
  slot booking_slots%ROWTYPE;
BEGIN
  SELECT *
  INTO hold
  FROM booking_holds
  WHERE id = NEW.hold_id;

  IF NOT FOUND OR hold.state <> 'ACTIVE' THEN
    RAISE EXCEPTION 'booking requires an active hold';
  END IF;

  SELECT *
  INTO slot
  FROM booking_slots
  WHERE id = NEW.slot_id;

  IF NOT FOUND OR
     hold.slot_id <> NEW.slot_id OR
     hold.client_identity_id <> NEW.client_identity_id OR
     hold.expires_at <> NEW.hold_expires_at OR
     slot.starts_at <> NEW.starts_at OR
     slot.ends_at <> NEW.ends_at THEN
    RAISE EXCEPTION 'booking does not match canonical hold/slot truth';
  END IF;

  IF NEW.state <> 'HELD' THEN
    RAISE EXCEPTION 'new booking must start in HELD state';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER bookings_insert_guard
BEFORE INSERT ON bookings
FOR EACH ROW EXECUTE FUNCTION apgic_booking_insert_guard();

CREATE TRIGGER bookings_transition_guard
BEFORE UPDATE ON bookings
FOR EACH ROW EXECUTE FUNCTION apgic_booking_transition_guard();

CREATE OR REPLACE FUNCTION apgic_create_booking_from_hold(
  p_booking_id uuid,
  p_hold_id uuid,
  p_now timestamptz
)
RETURNS TABLE(created boolean, reason_code text)
LANGUAGE plpgsql
AS $$
DECLARE
  v_hold booking_holds%ROWTYPE;
  v_slot booking_slots%ROWTYPE;
BEGIN
  IF p_booking_id IS NULL OR p_hold_id IS NULL OR p_now IS NULL THEN
    RETURN QUERY SELECT false, 'BOOK_CREATE_INVALID';
    RETURN;
  END IF;

  SELECT *
  INTO v_hold
  FROM booking_holds
  WHERE id = p_hold_id
  FOR UPDATE;

  IF NOT FOUND THEN
    RETURN QUERY SELECT false, 'BOOK_HOLD_NOT_FOUND';
    RETURN;
  END IF;

  IF v_hold.state <> 'ACTIVE' THEN
    RETURN QUERY SELECT false, 'BOOK_HOLD_NOT_ACTIVE';
    RETURN;
  END IF;

  IF v_hold.expires_at <= p_now THEN
    UPDATE booking_holds
    SET state = 'EXPIRED',
        updated_at = p_now
    WHERE id = p_hold_id;
    RETURN QUERY SELECT false, 'BOOK_HOLD_EXPIRED';
    RETURN;
  END IF;

  SELECT *
  INTO v_slot
  FROM booking_slots
  WHERE id = v_hold.slot_id
  FOR UPDATE;

  IF NOT FOUND OR v_slot.starts_at <= p_now THEN
    RETURN QUERY SELECT false, 'BOOK_SLOT_NOT_AVAILABLE';
    RETURN;
  END IF;

  IF EXISTS (
    SELECT 1
    FROM bookings
    WHERE slot_id = v_hold.slot_id
      AND state IN ('HELD','PENDING_PAYMENT','CONFIRMED')
  ) THEN
    RETURN QUERY SELECT false, 'BOOK_SLOT_BOOKED';
    RETURN;
  END IF;

  INSERT INTO bookings (
    id,
    slot_id,
    hold_id,
    client_identity_id,
    state,
    hold_expires_at,
    starts_at,
    ends_at,
    created_at,
    updated_at
  ) VALUES (
    p_booking_id,
    v_hold.slot_id,
    v_hold.id,
    v_hold.client_identity_id,
    'HELD',
    v_hold.expires_at,
    v_slot.starts_at,
    v_slot.ends_at,
    p_now,
    p_now
  );

  UPDATE booking_holds
  SET state = 'CONSUMED',
      updated_at = p_now
  WHERE id = p_hold_id;

  RETURN QUERY SELECT true, 'BOOK_CREATED';
END;
$$;

CREATE TABLE orders (
  id uuid PRIMARY KEY,
  booking_id uuid NOT NULL UNIQUE REFERENCES bookings(id),
  offer_ref text NOT NULL CHECK (btrim(offer_ref) <> ''),
  price_source_ref text NOT NULL CHECK (btrim(price_source_ref) <> ''),
  amount_minor bigint NOT NULL CHECK (amount_minor > 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  commission_minor bigint NOT NULL CHECK (
    commission_minor >= 0 AND commission_minor <= amount_minor
  ),
  pricing_policy_version text NOT NULL CHECK (btrim(pricing_policy_version) <> ''),
  commission_policy_version text NOT NULL CHECK (btrim(commission_policy_version) <> ''),
  legal_snapshot_id uuid NOT NULL REFERENCES legal_transaction_snapshots(id),
  seller_ref text NOT NULL CHECK (btrim(seller_ref) <> ''),
  commercial_owner_ref text NOT NULL CHECK (btrim(commercial_owner_ref) <> ''),
  payment_recipient_ref text NOT NULL CHECK (btrim(payment_recipient_ref) <> ''),
  platform_role text NOT NULL CHECK (btrim(platform_role) <> ''),
  fiscal_responsibility_ref text NOT NULL CHECK (btrim(fiscal_responsibility_ref) <> ''),
  refund_responsibility_ref text NOT NULL CHECK (btrim(refund_responsibility_ref) <> ''),
  payout_beneficiary_ref text NOT NULL CHECK (btrim(payout_beneficiary_ref) <> ''),
  captured_at timestamptz NOT NULL
);

CREATE OR REPLACE FUNCTION apgic_order_snapshot_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  legal legal_transaction_snapshots%ROWTYPE;
BEGIN
  SELECT *
  INTO legal
  FROM legal_transaction_snapshots
  WHERE id = NEW.legal_snapshot_id;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'order legal snapshot not found';
  END IF;

  IF legal.transaction_ref <> 'order/' || NEW.id::text OR
     legal.seller_or_service_provider_id <> NEW.seller_ref OR
     legal.commercial_owner_id <> NEW.commercial_owner_ref OR
     legal.payment_recipient_id <> NEW.payment_recipient_ref OR
     legal.platform_role <> NEW.platform_role OR
     legal.fiscal_responsibility_id <> NEW.fiscal_responsibility_ref OR
     legal.refund_responsibility_id <> NEW.refund_responsibility_ref OR
     legal.payout_beneficiary_id <> NEW.payout_beneficiary_ref THEN
    RAISE EXCEPTION 'order/legal snapshot role mismatch';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM bookings
    WHERE id = NEW.booking_id
      AND state IN ('HELD','PENDING_PAYMENT','CONFIRMED')
  ) THEN
    RAISE EXCEPTION 'order requires a live booking';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER orders_snapshot_guard
BEFORE INSERT ON orders
FOR EACH ROW EXECUTE FUNCTION apgic_order_snapshot_guard();

CREATE TRIGGER orders_append_only
BEFORE UPDATE OR DELETE ON orders
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

COMMIT;
