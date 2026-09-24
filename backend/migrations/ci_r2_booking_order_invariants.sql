\set ON_ERROR_STOP on

INSERT INTO identities (id)
VALUES
  ('00000000-0000-0000-0000-00000000b001'),
  ('00000000-0000-0000-0000-00000000b002'),
  ('00000000-0000-0000-0000-00000000b003');

INSERT INTO booking_slots (
  id,
  specialist_identity_id,
  tenant_scope,
  starts_at,
  ends_at,
  exclusive
) VALUES (
  '00000000-0000-0000-0000-00000000b101',
  '00000000-0000-0000-0000-00000000b001',
  'tenant/r2-ci',
  now() + interval '2 hours',
  now() + interval '3 hours',
  true
);

DO $$
DECLARE
  invalid_hold_blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO booking_holds (
      id,
      slot_id,
      client_identity_id,
      state,
      expires_at,
      created_at,
      updated_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000b299',
      '00000000-0000-0000-0000-00000000b101',
      '00000000-0000-0000-0000-00000000b003',
      'ACTIVE',
      now() + interval '3 hours',
      now(),
      now()
    );
  EXCEPTION WHEN raise_exception THEN
    invalid_hold_blocked := true;
  END;

  IF NOT invalid_hold_blocked THEN
    RAISE EXCEPTION 'hold extending beyond slot start was accepted';
  END IF;
END
$$;

SELECT *
FROM apgic_acquire_slot_hold(
  '00000000-0000-0000-0000-00000000b201',
  '00000000-0000-0000-0000-00000000b101',
  '00000000-0000-0000-0000-00000000b002',
  now() + interval '10 minutes',
  now()
);

DO $$
DECLARE
  mismatched_booking_blocked boolean := false;
BEGIN
  BEGIN
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
      '00000000-0000-0000-0000-00000000b399',
      '00000000-0000-0000-0000-00000000b101',
      '00000000-0000-0000-0000-00000000b201',
      '00000000-0000-0000-0000-00000000b003',
      'HELD',
      now() + interval '10 minutes',
      now() + interval '2 hours',
      now() + interval '3 hours',
      now(),
      now()
    );
  EXCEPTION WHEN raise_exception THEN
    mismatched_booking_blocked := true;
  END;

  IF NOT mismatched_booking_blocked THEN
    RAISE EXCEPTION 'booking/hold client mismatch was accepted';
  END IF;
END
$$;

SELECT *
FROM apgic_create_booking_from_hold(
  '00000000-0000-0000-0000-00000000b301',
  '00000000-0000-0000-0000-00000000b201',
  now() + interval '1 minute'
);

DO $$
DECLARE
  acquired boolean;
  reason text;
BEGIN
  SELECT outcome.acquired, outcome.reason_code
  INTO acquired, reason
  FROM apgic_acquire_slot_hold(
    '00000000-0000-0000-0000-00000000b202',
    '00000000-0000-0000-0000-00000000b101',
    '00000000-0000-0000-0000-00000000b003',
    now() + interval '10 minutes',
    now() + interval '2 minutes'
  ) AS outcome;

  IF acquired OR reason <> 'BOOK_SLOT_BOOKED' THEN
    RAISE EXCEPTION 'live booking allowed a second slot hold: acquired=%, reason=%', acquired, reason;
  END IF;
END
$$;

DO $$
DECLARE
  invalid_transition_blocked boolean := false;
  identity_rewrite_blocked boolean := false;
BEGIN
  BEGIN
    UPDATE bookings
    SET state = 'COMPLETED',
        updated_at = now() + interval '2 minutes'
    WHERE id = '00000000-0000-0000-0000-00000000b301';
  EXCEPTION WHEN raise_exception THEN
    invalid_transition_blocked := true;
  END;

  BEGIN
    UPDATE bookings
    SET client_identity_id = '00000000-0000-0000-0000-00000000b003',
        updated_at = now() + interval '2 minutes'
    WHERE id = '00000000-0000-0000-0000-00000000b301';
  EXCEPTION WHEN raise_exception THEN
    identity_rewrite_blocked := true;
  END;

  IF NOT invalid_transition_blocked THEN
    RAISE EXCEPTION 'invalid booking transition was accepted';
  END IF;
  IF NOT identity_rewrite_blocked THEN
    RAISE EXCEPTION 'booking identity rewrite was accepted';
  END IF;
END
$$;

UPDATE bookings
SET state = 'CONFIRMED',
    updated_at = now() + interval '2 minutes'
WHERE id = '00000000-0000-0000-0000-00000000b301';

INSERT INTO legal_transaction_snapshots (
  id,
  transaction_ref,
  seller_or_service_provider_id,
  commercial_owner_id,
  payment_recipient_id,
  platform_role,
  fiscal_responsibility_id,
  refund_responsibility_id,
  payout_beneficiary_id,
  policy_version,
  occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000b401',
  'order/00000000-0000-0000-0000-00000000b501',
  'identity/specialist-r2',
  'identity/specialist-r2',
  'identity/specialist-r2',
  'MARKETPLACE_INTERMEDIARY',
  'identity/specialist-r2',
  'identity/specialist-r2',
  'identity/specialist-r2',
  'legal-r2-ci-v1',
  now() + interval '2 minutes'
);

DO $$
DECLARE
  legal_role_mismatch_blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO orders (
      id,
      booking_id,
      offer_ref,
      price_source_ref,
      amount_minor,
      currency,
      commission_minor,
      pricing_policy_version,
      commission_policy_version,
      legal_snapshot_id,
      seller_ref,
      commercial_owner_ref,
      payment_recipient_ref,
      platform_role,
      fiscal_responsibility_ref,
      refund_responsibility_ref,
      payout_beneficiary_ref,
      captured_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000b501',
      '00000000-0000-0000-0000-00000000b301',
      'offer/r2-ci',
      'price/r2-ci',
      10000,
      'RUB',
      1000,
      'pricing-r2-ci-v1',
      'commission-r2-ci-v1',
      '00000000-0000-0000-0000-00000000b401',
      'identity/specialist-r2',
      'identity/specialist-r2',
      'identity/specialist-r2',
      'DIRECT_SELLER',
      'identity/specialist-r2',
      'identity/specialist-r2',
      'identity/specialist-r2',
      now() + interval '2 minutes'
    );
  EXCEPTION WHEN raise_exception THEN
    legal_role_mismatch_blocked := true;
  END;

  IF NOT legal_role_mismatch_blocked THEN
    RAISE EXCEPTION 'order accepted legal snapshot role mismatch';
  END IF;
END
$$;

INSERT INTO orders (
  id,
  booking_id,
  offer_ref,
  price_source_ref,
  amount_minor,
  currency,
  commission_minor,
  pricing_policy_version,
  commission_policy_version,
  legal_snapshot_id,
  seller_ref,
  commercial_owner_ref,
  payment_recipient_ref,
  platform_role,
  fiscal_responsibility_ref,
  refund_responsibility_ref,
  payout_beneficiary_ref,
  captured_at
) VALUES (
  '00000000-0000-0000-0000-00000000b501',
  '00000000-0000-0000-0000-00000000b301',
  'offer/r2-ci',
  'price/r2-ci',
  10000,
  'RUB',
  1000,
  'pricing-r2-ci-v1',
  'commission-r2-ci-v1',
  '00000000-0000-0000-0000-00000000b401',
  'identity/specialist-r2',
  'identity/specialist-r2',
  'identity/specialist-r2',
  'MARKETPLACE_INTERMEDIARY',
  'identity/specialist-r2',
  'identity/specialist-r2',
  'identity/specialist-r2',
  now() + interval '2 minutes'
);

DO $$
DECLARE
  economic_rewrite_blocked boolean := false;
  order_delete_blocked boolean := false;
BEGIN
  BEGIN
    UPDATE orders
    SET amount_minor = 1
    WHERE id = '00000000-0000-0000-0000-00000000b501';
  EXCEPTION WHEN raise_exception THEN
    economic_rewrite_blocked := true;
  END;

  BEGIN
    DELETE FROM orders
    WHERE id = '00000000-0000-0000-0000-00000000b501';
  EXCEPTION WHEN raise_exception THEN
    order_delete_blocked := true;
  END;

  IF NOT economic_rewrite_blocked THEN
    RAISE EXCEPTION 'captured order economics were rewritten';
  END IF;
  IF NOT order_delete_blocked THEN
    RAISE EXCEPTION 'captured order history was deleted';
  END IF;
END
$$;
