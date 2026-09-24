BEGIN;

CREATE TABLE payment_provider_config_versions (
  id uuid PRIMARY KEY,
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  config_version text NOT NULL CHECK (btrim(config_version) <> ''),
  status text NOT NULL CHECK (status IN ('CONFIGURING','ACTIVE','PAUSED','DISABLED')),
  priority integer NOT NULL CHECK (priority >= 0),
  manifest_version text NOT NULL CHECK (btrim(manifest_version) <> ''),
  certification_evidence_refs text[] NOT NULL CHECK (cardinality(certification_evidence_refs) > 0),
  jurisdiction_codes text[] NOT NULL CHECK (cardinality(jurisdiction_codes) > 0),
  currencies text[] NOT NULL CHECK (cardinality(currencies) > 0),
  method_codes text[] NOT NULL CHECK (cardinality(method_codes) > 0),
  rail_codes text[] NOT NULL CHECK (cardinality(rail_codes) > 0),
  execution_owner text NOT NULL CHECK (execution_owner = 'EXTERNAL_PROVIDER'),
  effective_from timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (provider_instance_id, config_version)
);

CREATE OR REPLACE FUNCTION apgic_payment_provider_config_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  connector_capability text;
  connector_status text;
BEGIN
  SELECT capability_class, status
  INTO connector_capability, connector_status
  FROM connector_instances
  WHERE id = NEW.provider_instance_id;

  IF NOT FOUND OR connector_capability <> 'PAYMENT_PROVIDER' THEN
    RAISE EXCEPTION 'payment provider config requires PAYMENT_PROVIDER connector';
  END IF;

  IF NEW.status = 'ACTIVE' AND connector_status NOT IN ('ACTIVE','DEGRADED') THEN
    RAISE EXCEPTION 'ACTIVE payment provider config requires routable connector status';
  END IF;

  IF EXISTS (
    SELECT 1 FROM unnest(NEW.currencies) AS currency
    WHERE currency !~ '^[A-Z]{3}$'
  ) THEN
    RAISE EXCEPTION 'payment provider currency must be uppercase ISO-style code';
  END IF;

  IF EXISTS (
    SELECT 1 FROM unnest(
      NEW.certification_evidence_refs ||
      NEW.jurisdiction_codes ||
      NEW.method_codes ||
      NEW.rail_codes
    ) AS value
    WHERE btrim(value) = ''
  ) THEN
    RAISE EXCEPTION 'payment provider config arrays cannot contain blank values';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER payment_provider_config_versions_insert_guard
BEFORE INSERT ON payment_provider_config_versions
FOR EACH ROW EXECUTE FUNCTION apgic_payment_provider_config_guard();

CREATE TRIGGER payment_provider_config_versions_append_only
BEFORE UPDATE OR DELETE ON payment_provider_config_versions
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE payment_routing_decisions (
  id uuid PRIMARY KEY,
  order_id uuid NOT NULL REFERENCES orders(id),
  policy_version text NOT NULL CHECK (btrim(policy_version) <> ''),
  provider_config_id uuid NOT NULL REFERENCES payment_provider_config_versions(id),
  jurisdiction_code text NOT NULL CHECK (btrim(jurisdiction_code) <> ''),
  selected_method_code text NOT NULL CHECK (btrim(selected_method_code) <> ''),
  selected_rail_code text NOT NULL CHECK (btrim(selected_rail_code) <> ''),
  candidate_evidence jsonb NOT NULL,
  health_snapshot jsonb NOT NULL,
  decided_at timestamptz NOT NULL
);

CREATE OR REPLACE FUNCTION apgic_payment_routing_decision_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  config payment_provider_config_versions%ROWTYPE;
  order_row orders%ROWTYPE;
BEGIN
  SELECT *
  INTO config
  FROM payment_provider_config_versions
  WHERE id = NEW.provider_config_id;

  IF NOT FOUND OR config.status <> 'ACTIVE' THEN
    RAISE EXCEPTION 'routing decision requires ACTIVE certified provider config';
  END IF;

  SELECT *
  INTO order_row
  FROM orders
  WHERE id = NEW.order_id;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'routing decision order not found';
  END IF;

  IF NOT (order_row.currency = ANY(config.currencies)) OR
     NOT (NEW.jurisdiction_code = ANY(config.jurisdiction_codes)) OR
     NOT (NEW.selected_method_code = ANY(config.method_codes)) OR
     NOT (NEW.selected_rail_code = ANY(config.rail_codes)) THEN
    RAISE EXCEPTION 'routing decision selected ineligible provider/method/rail scope';
  END IF;

  IF jsonb_typeof(NEW.candidate_evidence) <> 'array' OR
     jsonb_array_length(NEW.candidate_evidence) = 0 OR
     jsonb_typeof(NEW.health_snapshot) <> 'object' THEN
    RAISE EXCEPTION 'routing decision requires reproducible candidate and health evidence';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER payment_routing_decisions_insert_guard
BEFORE INSERT ON payment_routing_decisions
FOR EACH ROW EXECUTE FUNCTION apgic_payment_routing_decision_guard();

CREATE TRIGGER payment_routing_decisions_append_only
BEFORE UPDATE OR DELETE ON payment_routing_decisions
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE payment_attempts (
  id uuid PRIMARY KEY,
  order_id uuid NOT NULL REFERENCES orders(id),
  routing_decision_id uuid NOT NULL UNIQUE REFERENCES payment_routing_decisions(id),
  idempotency_key text NOT NULL CHECK (btrim(idempotency_key) <> ''),
  amount_minor bigint NOT NULL CHECK (amount_minor > 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  state text NOT NULL CHECK (
    state IN ('CREATED','SENT','PENDING','SUCCEEDED','FAILED_TERMINAL','AMBIGUOUS')
  ),
  provider_reference text,
  provider_evidence_ref text,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (order_id, idempotency_key)
);

CREATE INDEX payment_attempts_order_state_idx
  ON payment_attempts (order_id, state, created_at);

CREATE OR REPLACE FUNCTION apgic_payment_attempt_insert_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  order_row orders%ROWTYPE;
  decision payment_routing_decisions%ROWTYPE;
BEGIN
  SELECT *
  INTO order_row
  FROM orders
  WHERE id = NEW.order_id
  FOR UPDATE;

  IF NOT FOUND OR
     NEW.amount_minor <> order_row.amount_minor OR
     NEW.currency <> order_row.currency THEN
    RAISE EXCEPTION 'payment attempt must preserve immutable order economics';
  END IF;

  SELECT *
  INTO decision
  FROM payment_routing_decisions
  WHERE id = NEW.routing_decision_id;

  IF NOT FOUND OR decision.order_id <> NEW.order_id THEN
    RAISE EXCEPTION 'payment attempt routing decision/order mismatch';
  END IF;

  IF NEW.state <> 'CREATED' OR NEW.updated_at <> NEW.created_at THEN
    RAISE EXCEPTION 'new payment attempt must start CREATED with aligned timestamps';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM payment_attempts
    WHERE order_id = NEW.order_id
      AND state IN ('CREATED','SENT','PENDING','AMBIGUOUS','SUCCEEDED')
  ) THEN
    RAISE EXCEPTION 'unsafe duplicate/cross-provider payment attempt blocked';
  END IF;

  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION apgic_payment_attempt_transition_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.order_id <> OLD.order_id OR
     NEW.routing_decision_id <> OLD.routing_decision_id OR
     NEW.idempotency_key <> OLD.idempotency_key OR
     NEW.amount_minor <> OLD.amount_minor OR
     NEW.currency <> OLD.currency OR
     NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'payment attempt identity/economics are immutable';
  END IF;

  IF NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'payment attempt updated_at cannot move backwards';
  END IF;

  IF OLD.provider_reference IS NOT NULL AND
     NEW.provider_reference IS DISTINCT FROM OLD.provider_reference THEN
    RAISE EXCEPTION 'provider reference cannot be rewritten';
  END IF;

  IF OLD.provider_evidence_ref IS NOT NULL AND
     NEW.provider_evidence_ref IS DISTINCT FROM OLD.provider_evidence_ref THEN
    RAISE EXCEPTION 'provider evidence cannot be rewritten';
  END IF;

  IF OLD.state = NEW.state THEN
    RETURN NEW;
  END IF;

  IF NEW.state IN ('SUCCEEDED','FAILED_TERMINAL') AND
     (NEW.provider_evidence_ref IS NULL OR btrim(NEW.provider_evidence_ref) = '') THEN
    RAISE EXCEPTION 'terminal payment state requires provider evidence';
  END IF;

  IF OLD.state = 'CREATED' AND NEW.state IN ('SENT','FAILED_TERMINAL') THEN
    RETURN NEW;
  ELSIF OLD.state = 'SENT' AND NEW.state IN ('PENDING','SUCCEEDED','FAILED_TERMINAL','AMBIGUOUS') THEN
    RETURN NEW;
  ELSIF OLD.state = 'PENDING' AND NEW.state IN ('SUCCEEDED','FAILED_TERMINAL','AMBIGUOUS') THEN
    RETURN NEW;
  ELSIF OLD.state = 'AMBIGUOUS' AND NEW.state IN ('SUCCEEDED','FAILED_TERMINAL') THEN
    RETURN NEW;
  END IF;

  RAISE EXCEPTION 'invalid payment attempt transition: % -> %', OLD.state, NEW.state;
END;
$$;

CREATE TRIGGER payment_attempts_insert_guard
BEFORE INSERT ON payment_attempts
FOR EACH ROW EXECUTE FUNCTION apgic_payment_attempt_insert_guard();

CREATE TRIGGER payment_attempts_transition_guard
BEFORE UPDATE ON payment_attempts
FOR EACH ROW EXECUTE FUNCTION apgic_payment_attempt_transition_guard();

CREATE OR REPLACE FUNCTION apgic_forbid_payment_attempt_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'payment attempt history cannot be deleted';
END;
$$;

CREATE TRIGGER payment_attempts_no_delete
BEFORE DELETE ON payment_attempts
FOR EACH ROW EXECUTE FUNCTION apgic_forbid_payment_attempt_delete();

CREATE TABLE payment_webhook_receipts (
  id uuid PRIMARY KEY,
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  provider_event_id text NOT NULL CHECK (btrim(provider_event_id) <> ''),
  payload_digest text NOT NULL CHECK (btrim(payload_digest) <> ''),
  signature_verified boolean NOT NULL,
  received_at timestamptz NOT NULL,
  UNIQUE (provider_instance_id, provider_event_id)
);

CREATE TRIGGER payment_webhook_receipts_append_only
BEFORE UPDATE OR DELETE ON payment_webhook_receipts
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE OR REPLACE FUNCTION apgic_record_payment_webhook(
  p_receipt_id uuid,
  p_provider_instance_id uuid,
  p_provider_event_id text,
  p_payload_digest text,
  p_received_at timestamptz
)
RETURNS TABLE(accepted boolean, reason_code text)
LANGUAGE plpgsql
AS $$
BEGIN
  IF p_receipt_id IS NULL OR
     p_provider_instance_id IS NULL OR
     btrim(coalesce(p_provider_event_id, '')) = '' OR
     btrim(coalesce(p_payload_digest, '')) = '' OR
     p_received_at IS NULL THEN
    RETURN QUERY SELECT false, 'PAY_WEBHOOK_INVALID';
    RETURN;
  END IF;

  INSERT INTO payment_webhook_receipts (
    id,
    provider_instance_id,
    provider_event_id,
    payload_digest,
    signature_verified,
    received_at
  ) VALUES (
    p_receipt_id,
    p_provider_instance_id,
    p_provider_event_id,
    p_payload_digest,
    true,
    p_received_at
  )
  ON CONFLICT (provider_instance_id, provider_event_id) DO NOTHING;

  IF FOUND THEN
    RETURN QUERY SELECT true, 'PAY_WEBHOOK_ACCEPTED';
  ELSE
    RETURN QUERY SELECT false, 'PAY_WEBHOOK_DUPLICATE';
  END IF;
END;
$$;

CREATE TABLE payment_effects (
  id uuid PRIMARY KEY,
  receipt_id uuid NOT NULL UNIQUE REFERENCES payment_webhook_receipts(id),
  attempt_id uuid NOT NULL REFERENCES payment_attempts(id),
  effect_kind text NOT NULL CHECK (effect_kind IN ('CAPTURED','REFUNDED','FAILED','STATUS')),
  provider_evidence_ref text NOT NULL CHECK (btrim(provider_evidence_ref) <> ''),
  ledger_entry_id uuid UNIQUE REFERENCES ledger_entries(id),
  occurred_at timestamptz NOT NULL,
  CHECK (
    effect_kind NOT IN ('CAPTURED','REFUNDED')
    OR ledger_entry_id IS NOT NULL
  )
);

CREATE UNIQUE INDEX payment_effects_one_capture_per_attempt
  ON payment_effects (attempt_id)
  WHERE effect_kind = 'CAPTURED';

CREATE OR REPLACE FUNCTION apgic_payment_effect_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  receipt_provider uuid;
  route_provider uuid;
BEGIN
  SELECT provider_instance_id
  INTO receipt_provider
  FROM payment_webhook_receipts
  WHERE id = NEW.receipt_id
    AND signature_verified = true;

  SELECT config.provider_instance_id
  INTO route_provider
  FROM payment_attempts attempt
  JOIN payment_routing_decisions decision ON decision.id = attempt.routing_decision_id
  JOIN payment_provider_config_versions config ON config.id = decision.provider_config_id
  WHERE attempt.id = NEW.attempt_id;

  IF receipt_provider IS NULL OR route_provider IS NULL OR receipt_provider <> route_provider THEN
    RAISE EXCEPTION 'payment effect provider evidence does not match routed attempt';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER payment_effects_insert_guard
BEFORE INSERT ON payment_effects
FOR EACH ROW EXECUTE FUNCTION apgic_payment_effect_guard();

CREATE TRIGGER payment_effects_append_only
BEFORE UPDATE OR DELETE ON payment_effects
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

COMMIT;
