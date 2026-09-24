BEGIN;

CREATE TABLE refund_requests (
  id uuid PRIMARY KEY,
  booking_id uuid NOT NULL REFERENCES bookings(id),
  order_id uuid NOT NULL REFERENCES orders(id),
  original_payment_attempt_id uuid NOT NULL REFERENCES payment_attempts(id),
  original_provider_config_id uuid NOT NULL REFERENCES payment_provider_config_versions(id),
  amount_minor bigint NOT NULL CHECK (amount_minor > 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  policy_version text NOT NULL CHECK (btrim(policy_version) <> ''),
  policy_decision text NOT NULL CHECK (policy_decision = 'ALLOW'),
  reason_code text NOT NULL CHECK (btrim(reason_code) <> ''),
  idempotency_key text NOT NULL CHECK (btrim(idempotency_key) <> ''),
  execution_owner text NOT NULL CHECK (execution_owner = 'EXTERNAL_PROVIDER'),
  state text NOT NULL CHECK (
    state IN ('REQUESTED','SENT','PENDING','SUCCEEDED','FAILED_TERMINAL','AMBIGUOUS')
  ),
  provider_reference text,
  provider_evidence_ref text,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (original_payment_attempt_id, idempotency_key)
);

CREATE INDEX refund_requests_payment_state_idx
  ON refund_requests (original_payment_attempt_id, state, created_at);

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
  SELECT *
  INTO booking_row
  FROM bookings
  WHERE id = NEW.booking_id;

  IF NOT FOUND OR booking_row.state <> 'CANCELLED' THEN
    RAISE EXCEPTION 'refund requires a cancelled booking';
  END IF;

  SELECT *
  INTO order_row
  FROM orders
  WHERE id = NEW.order_id;

  IF NOT FOUND OR order_row.booking_id <> NEW.booking_id THEN
    RAISE EXCEPTION 'refund order/booking mismatch';
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
  IF NEW.booking_id <> OLD.booking_id OR
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

CREATE TRIGGER refund_requests_insert_guard
BEFORE INSERT ON refund_requests
FOR EACH ROW EXECUTE FUNCTION apgic_refund_request_insert_guard();

CREATE TRIGGER refund_requests_transition_guard
BEFORE UPDATE ON refund_requests
FOR EACH ROW EXECUTE FUNCTION apgic_refund_request_transition_guard();

CREATE OR REPLACE FUNCTION apgic_forbid_refund_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'refund/reversal history cannot be deleted';
END;
$$;

CREATE TRIGGER refund_requests_no_delete
BEFORE DELETE ON refund_requests
FOR EACH ROW EXECUTE FUNCTION apgic_forbid_refund_delete();

CREATE TABLE refund_effects (
  id uuid PRIMARY KEY,
  refund_request_id uuid NOT NULL UNIQUE REFERENCES refund_requests(id),
  receipt_id uuid NOT NULL UNIQUE REFERENCES payment_webhook_receipts(id),
  provider_evidence_ref text NOT NULL CHECK (btrim(provider_evidence_ref) <> ''),
  ledger_entry_id uuid NOT NULL UNIQUE REFERENCES ledger_entries(id),
  occurred_at timestamptz NOT NULL
);

CREATE OR REPLACE FUNCTION apgic_refund_effect_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  refund_row refund_requests%ROWTYPE;
  receipt_provider_instance_id uuid;
  expected_provider_instance_id uuid;
  ledger_row ledger_entries%ROWTYPE;
BEGIN
  SELECT *
  INTO refund_row
  FROM refund_requests
  WHERE id = NEW.refund_request_id;

  IF NOT FOUND OR refund_row.state <> 'SUCCEEDED' THEN
    RAISE EXCEPTION 'refund effect requires SUCCEEDED refund request';
  END IF;

  SELECT provider_instance_id
  INTO receipt_provider_instance_id
  FROM payment_webhook_receipts
  WHERE id = NEW.receipt_id
    AND signature_verified = true;

  SELECT provider_instance_id
  INTO expected_provider_instance_id
  FROM payment_provider_config_versions
  WHERE id = refund_row.original_provider_config_id;

  IF receipt_provider_instance_id IS NULL OR
     expected_provider_instance_id IS NULL OR
     receipt_provider_instance_id <> expected_provider_instance_id THEN
    RAISE EXCEPTION 'refund evidence must come from original payment provider';
  END IF;

  SELECT *
  INTO ledger_row
  FROM ledger_entries
  WHERE id = NEW.ledger_entry_id;

  IF NOT FOUND OR
     ledger_row.amount_minor <> refund_row.amount_minor OR
     ledger_row.currency <> refund_row.currency OR
     ledger_row.provider_evidence_ref <> NEW.provider_evidence_ref THEN
    RAISE EXCEPTION 'refund ledger reversal does not match provider evidence/economics';
  END IF;

  IF refund_row.provider_evidence_ref <> NEW.provider_evidence_ref THEN
    RAISE EXCEPTION 'refund effect evidence does not match terminal request evidence';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER refund_effects_insert_guard
BEFORE INSERT ON refund_effects
FOR EACH ROW EXECUTE FUNCTION apgic_refund_effect_guard();

CREATE TRIGGER refund_effects_append_only
BEFORE UPDATE OR DELETE ON refund_effects
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

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

  RETURN QUERY SELECT true, 'REFUND_APPLIED';
END;
$$;

COMMIT;
