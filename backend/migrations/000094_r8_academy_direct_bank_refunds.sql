-- R8 Academy refund: reuse canonical R2 refund requests, signed PSP
-- receipts, reversal ledger and immutable effects. No synthetic Booking.
-- Existing booking refunds retain their cancellation gate.
ALTER TABLE refund_requests ALTER COLUMN booking_id DROP NOT NULL;

CREATE OR REPLACE FUNCTION apgic_refund_request_insert_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  booking_row bookings%ROWTYPE;
  order_row orders%ROWTYPE;
  payment_row payment_attempts%ROWTYPE;
  captured_effect payment_effects%ROWTYPE;
  routed_provider_config_id uuid;
  already_reserved bigint;
BEGIN
  SELECT * INTO order_row FROM orders WHERE id = NEW.order_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'refund order not found';
  END IF;

  IF NEW.booking_id IS NOT NULL THEN
    -- Preserve the consultation/booking cancellation gate unchanged.
    SELECT * INTO booking_row FROM bookings WHERE id = NEW.booking_id;
    IF NOT FOUND OR booking_row.state <> 'CANCELLED' THEN
      RAISE EXCEPTION 'refund requires a cancelled booking';
    END IF;
    IF order_row.booking_id IS DISTINCT FROM NEW.booking_id THEN
      RAISE EXCEPTION 'refund order/booking mismatch';
    END IF;
  ELSE
    -- A direct Academy order has no fictitious Booking. Its existing paid
    -- purchase and original provider must be independently verifiable.
    IF order_row.booking_id IS NOT NULL
       OR order_row.direct_buyer_identity_id IS NULL
       OR NOT EXISTS (
         SELECT 1 FROM academy_courses c
         WHERE c.product_id = order_row.product_id
       )
    THEN
      RAISE EXCEPTION 'direct refund requires canonical Academy order';
    END IF;
  END IF;

  SELECT *
  INTO payment_row
  FROM payment_attempts
  WHERE id = NEW.original_payment_attempt_id
  FOR UPDATE;

  IF NOT FOUND OR
     payment_row.order_id <> NEW.order_id OR
     payment_row.state <> 'SUCCEEDED' OR
     payment_row.currency <> NEW.currency THEN
    RAISE EXCEPTION 'refund requires the successful original payment attempt';
  END IF;

  SELECT *
  INTO captured_effect
  FROM payment_effects
  WHERE attempt_id = NEW.original_payment_attempt_id
    AND effect_kind = 'CAPTURED'
  FOR UPDATE;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'refund requires captured provider/ledger evidence';
  END IF;

  IF NEW.booking_id IS NULL THEN
    -- Direct bank refunds require an actual ONE_TIME purchase linked to the
    -- captured attempt, not merely a bank-looking provider reference.
    IF NEW.amount_minor <> payment_row.amount_minor OR
       NOT EXISTS (
         SELECT 1 FROM store_transaction_verifications v
         JOIN store_entitlements e ON e.verification_id = v.id
         WHERE v.payment_attempt_id = payment_row.id
           AND v.order_id = NEW.order_id
           AND v.identity_id = order_row.direct_buyer_identity_id
           AND v.product_ref = order_row.product_id::text
           AND v.entitlement_kind = 'ONE_TIME'
           AND e.identity_id = v.identity_id
       ) THEN
      RAISE EXCEPTION 'direct bank refund requires entire verified one-time purchase';
    END IF;
  END IF;

  SELECT decision.provider_config_id
  INTO routed_provider_config_id
  FROM payment_routing_decisions decision
  WHERE decision.id = payment_row.routing_decision_id;

  IF routed_provider_config_id IS NULL OR
     NEW.original_provider_config_id <> routed_provider_config_id THEN
    RAISE EXCEPTION 'refund must execute through the original payment provider';
  END IF;

  IF NEW.state <> 'REQUESTED' OR
     NEW.updated_at <> NEW.created_at OR
     NEW.amount_minor > payment_row.amount_minor THEN
    RAISE EXCEPTION 'invalid initial refund request';
  END IF;

  SELECT coalesce(sum(amount_minor), 0)
  INTO already_reserved
  FROM refund_requests
  WHERE original_payment_attempt_id = NEW.original_payment_attempt_id
    AND state IN ('REQUESTED','SENT','PENDING','AMBIGUOUS','SUCCEEDED');

  IF already_reserved + NEW.amount_minor > payment_row.amount_minor THEN
    RAISE EXCEPTION 'refund amount exceeds captured payment amount';
  END IF;

  RETURN NEW;
END;
$$;


CREATE OR REPLACE FUNCTION apgic_refund_request_transition_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.booking_id IS DISTINCT FROM OLD.booking_id OR
     NEW.order_id <> OLD.order_id OR
     NEW.original_payment_attempt_id <> OLD.original_payment_attempt_id OR
     NEW.original_provider_config_id <> OLD.original_provider_config_id OR
     NEW.amount_minor <> OLD.amount_minor OR
     NEW.currency <> OLD.currency OR
     NEW.policy_version <> OLD.policy_version OR
     NEW.policy_decision <> OLD.policy_decision OR
     NEW.reason_code <> OLD.reason_code OR
     NEW.idempotency_key <> OLD.idempotency_key OR
     NEW.execution_owner <> OLD.execution_owner OR
     NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'refund canonical identity/economics/policy are immutable';
  END IF;

  IF NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'refund updated_at cannot move backwards';
  END IF;

  IF OLD.provider_reference IS NOT NULL AND
     NEW.provider_reference IS DISTINCT FROM OLD.provider_reference THEN
    RAISE EXCEPTION 'refund provider reference cannot be rewritten';
  END IF;

  IF OLD.provider_evidence_ref IS NOT NULL AND
     NEW.provider_evidence_ref IS DISTINCT FROM OLD.provider_evidence_ref THEN
    RAISE EXCEPTION 'refund provider evidence cannot be rewritten';
  END IF;

  IF OLD.state = NEW.state THEN
    RETURN NEW;
  END IF;

  IF NEW.state = 'SENT' AND
     (NEW.provider_reference IS NULL OR btrim(NEW.provider_reference) = '') THEN
    RAISE EXCEPTION 'refund SENT requires provider reference';
  END IF;

  IF NEW.state IN ('SUCCEEDED','FAILED_TERMINAL') AND
     (NEW.provider_evidence_ref IS NULL OR btrim(NEW.provider_evidence_ref) = '') THEN
    RAISE EXCEPTION 'terminal refund requires provider evidence';
  END IF;

  IF OLD.state = 'REQUESTED' AND NEW.state IN ('SENT','FAILED_TERMINAL') THEN
    RETURN NEW;
  ELSIF OLD.state = 'SENT' AND NEW.state IN ('PENDING','SUCCEEDED','FAILED_TERMINAL','AMBIGUOUS') THEN
    RETURN NEW;
  ELSIF OLD.state = 'PENDING' AND NEW.state IN ('SUCCEEDED','FAILED_TERMINAL','AMBIGUOUS') THEN
    RETURN NEW;
  ELSIF OLD.state = 'AMBIGUOUS' AND NEW.state IN ('SUCCEEDED','FAILED_TERMINAL') THEN
    RETURN NEW;
  END IF;

  RAISE EXCEPTION 'invalid refund transition: % -> %', OLD.state, NEW.state;
END;
$$;


CREATE OR REPLACE FUNCTION apgic_apply_refund_success(
  p_refund_request_id uuid,
  p_receipt_id uuid,
  p_provider_evidence_ref text,
  p_ledger_entry_id uuid,
  p_refund_effect_id uuid,
  p_now timestamptz
)
RETURNS TABLE(applied boolean, reason_code text)
LANGUAGE plpgsql
AS $$
DECLARE
  current_refund refund_requests%ROWTYPE;
BEGIN
  SELECT *
  INTO current_refund
  FROM refund_requests
  WHERE id = p_refund_request_id
  FOR UPDATE;

  IF NOT FOUND THEN
    RETURN QUERY SELECT false, 'REFUND_NOT_FOUND';
    RETURN;
  END IF;

  IF current_refund.state = 'SUCCEEDED' THEN
    IF EXISTS (
      SELECT 1 FROM refund_effects WHERE refund_request_id = p_refund_request_id
    ) THEN
      RETURN QUERY SELECT false, 'REFUND_ALREADY_APPLIED';
      RETURN;
    END IF;
    RAISE EXCEPTION 'succeeded refund is missing immutable effect evidence';
  END IF;

  IF current_refund.state NOT IN ('SENT','PENDING','AMBIGUOUS') OR
     btrim(coalesce(p_provider_evidence_ref, '')) = '' OR
     p_receipt_id IS NULL OR
     p_ledger_entry_id IS NULL OR
     p_refund_effect_id IS NULL OR
     p_now IS NULL THEN
    RETURN QUERY SELECT false, 'REFUND_OUTCOME_INVALID';
    RETURN;
  END IF;

  UPDATE refund_requests
  SET state = 'SUCCEEDED',
      provider_evidence_ref = p_provider_evidence_ref,
      updated_at = p_now
  WHERE id = p_refund_request_id;

  INSERT INTO refund_effects (
    id, refund_request_id, receipt_id,
    provider_evidence_ref, ledger_entry_id, occurred_at
  ) VALUES (
    p_refund_effect_id, p_refund_request_id, p_receipt_id,
    p_provider_evidence_ref, p_ledger_entry_id, p_now
  );

  -- Refund success, signed receipt and reversal ledger are committed together.
  -- Revoke paid learner access only for direct Academy FULL refunds; partial
  -- provider outcomes cannot silently rewrite unrelated entitlements.
  IF current_refund.booking_id IS NULL THEN
    UPDATE store_entitlements entitlement
    SET state = 'REVOKED', updated_at = p_now
    FROM store_transaction_verifications verification
    WHERE entitlement.verification_id = verification.id
      AND verification.payment_attempt_id = current_refund.original_payment_attempt_id
      AND verification.order_id = current_refund.order_id
      AND entitlement.kind = 'ONE_TIME'
      AND entitlement.state = 'ACTIVE';
    IF NOT FOUND THEN
      -- A previously revoked entitlement is legitimate on a retried admin
      -- action, but the transaction must still refer to a real purchase.
      IF NOT EXISTS (
        SELECT 1 FROM store_entitlements entitlement
        JOIN store_transaction_verifications verification
          ON verification.id=entitlement.verification_id
        WHERE verification.payment_attempt_id=current_refund.original_payment_attempt_id
          AND verification.order_id=current_refund.order_id
          AND entitlement.state='REVOKED'
      ) THEN
        RAISE EXCEPTION 'direct refund missing learner entitlement for revocation';
      END IF;
    END IF;
  END IF;

  RETURN QUERY SELECT true, 'REFUND_APPLIED';
END;
$$;


