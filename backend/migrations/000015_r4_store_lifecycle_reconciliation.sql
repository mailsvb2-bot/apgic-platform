BEGIN;

CREATE TABLE store_subscription_lifecycle_events (
  id uuid PRIMARY KEY,
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  provider_event_id text NOT NULL CHECK (btrim(provider_event_id) <> ''),
  external_transaction_id text NOT NULL CHECK (btrim(external_transaction_id) <> ''),
  subscription_ref text NOT NULL CHECK (btrim(subscription_ref) <> ''),
  identity_id uuid NOT NULL REFERENCES identities(id),
  entitlement_id uuid NOT NULL REFERENCES store_entitlements(id),
  provider_sequence bigint NOT NULL CHECK (provider_sequence > 0),
  event_type text NOT NULL CHECK (
    event_type IN (
      'RENEWAL','REFUND','REVOCATION','CHARGEBACK',
      'GRACE_STARTED','HOLD_STARTED','EXPIRED'
    )
  ),
  amount_minor bigint,
  currency text,
  provider_evidence_ref text NOT NULL CHECK (btrim(provider_evidence_ref) <> ''),
  ledger_entry_id uuid REFERENCES ledger_entries(id),
  occurred_at timestamptz NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (provider_instance_id, provider_event_id)
);

CREATE INDEX store_subscription_lifecycle_sequence_idx
  ON store_subscription_lifecycle_events (
    provider_instance_id, subscription_ref, provider_sequence, received_at
  );

CREATE OR REPLACE FUNCTION apgic_store_lifecycle_event_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  entitlement store_entitlements%ROWTYPE;
  verification store_transaction_verifications%ROWTYPE;
  ledger ledger_entries%ROWTYPE;
BEGIN
  SELECT *
  INTO entitlement
  FROM store_entitlements
  WHERE id = NEW.entitlement_id;

  IF NOT FOUND OR entitlement.identity_id <> NEW.identity_id OR entitlement.kind <> 'SUBSCRIPTION' THEN
    RAISE EXCEPTION 'store lifecycle event requires matching subscription entitlement';
  END IF;

  SELECT *
  INTO verification
  FROM store_transaction_verifications
  WHERE id = entitlement.verification_id;

  IF NOT FOUND OR verification.provider_instance_id <> NEW.provider_instance_id THEN
    RAISE EXCEPTION 'store lifecycle event must preserve original store provider affinity';
  END IF;

  IF NEW.event_type IN ('RENEWAL','REFUND','CHARGEBACK') THEN
    IF NEW.amount_minor IS NULL OR NEW.amount_minor <= 0
      OR NEW.currency IS NULL OR NEW.currency !~ '^[A-Z]{3}$'
      OR NEW.ledger_entry_id IS NULL THEN
      RAISE EXCEPTION 'financial store lifecycle event requires amount/currency/ledger evidence';
    END IF;

    SELECT *
    INTO ledger
    FROM ledger_entries
    WHERE id = NEW.ledger_entry_id;

    IF NOT FOUND
      OR ledger.amount_minor <> NEW.amount_minor
      OR ledger.currency <> NEW.currency
      OR ledger.provider_evidence_ref <> NEW.provider_evidence_ref THEN
      RAISE EXCEPTION 'store lifecycle ledger evidence mismatch';
    END IF;
  ELSE
    IF NEW.amount_minor IS NOT NULL OR NEW.currency IS NOT NULL OR NEW.ledger_entry_id IS NOT NULL THEN
      RAISE EXCEPTION 'non-financial store lifecycle event cannot invent monetary effect';
    END IF;
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER store_subscription_lifecycle_events_insert_guard
BEFORE INSERT ON store_subscription_lifecycle_events
FOR EACH ROW EXECUTE FUNCTION apgic_store_lifecycle_event_guard();

CREATE TRIGGER store_subscription_lifecycle_events_append_only
BEFORE UPDATE OR DELETE ON store_subscription_lifecycle_events
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE store_subscription_projections (
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  subscription_ref text NOT NULL CHECK (btrim(subscription_ref) <> ''),
  identity_id uuid NOT NULL REFERENCES identities(id),
  entitlement_id uuid NOT NULL REFERENCES store_entitlements(id),
  subscription_state text NOT NULL CHECK (
    subscription_state IN ('ACTIVE','GRACE','HOLD','EXPIRED','REVOKED')
  ),
  entitlement_state text NOT NULL CHECK (
    entitlement_state IN ('ACTIVE','EXPIRED','REVOKED')
  ),
  last_provider_sequence bigint NOT NULL CHECK (last_provider_sequence > 0),
  last_event_id uuid NOT NULL REFERENCES store_subscription_lifecycle_events(id),
  applied_event_count bigint NOT NULL CHECK (applied_event_count > 0),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (provider_instance_id, subscription_ref)
);

CREATE OR REPLACE FUNCTION apgic_store_subscription_projection_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'UPDATE' THEN
    IF NEW.provider_instance_id <> OLD.provider_instance_id
      OR NEW.subscription_ref <> OLD.subscription_ref
      OR NEW.identity_id <> OLD.identity_id
      OR NEW.entitlement_id <> OLD.entitlement_id THEN
      RAISE EXCEPTION 'store subscription projection identity is immutable';
    END IF;
    IF NEW.last_provider_sequence <= OLD.last_provider_sequence
      OR NEW.applied_event_count <> OLD.applied_event_count + 1
      OR NEW.updated_at < OLD.updated_at THEN
      RAISE EXCEPTION 'store subscription projection must advance monotonically';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER store_subscription_projections_update_guard
BEFORE UPDATE ON store_subscription_projections
FOR EACH ROW EXECUTE FUNCTION apgic_store_subscription_projection_guard();

CREATE TRIGGER store_subscription_projections_no_delete
BEFORE DELETE ON store_subscription_projections
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE store_lifecycle_effects (
  id uuid PRIMARY KEY,
  lifecycle_event_id uuid NOT NULL UNIQUE REFERENCES store_subscription_lifecycle_events(id),
  entitlement_effect text NOT NULL CHECK (
    entitlement_effect IN ('ACTIVATE','KEEP','EXPIRE','REVOKE')
  ),
  ledger_effect text NOT NULL CHECK (
    ledger_effect IN ('NONE','RENEWAL_CAPTURE','REFUND','CHARGEBACK')
  ),
  ledger_entry_id uuid REFERENCES ledger_entries(id),
  applied_at timestamptz NOT NULL,
  CHECK (
    (ledger_effect = 'NONE' AND ledger_entry_id IS NULL)
    OR
    (ledger_effect <> 'NONE' AND ledger_entry_id IS NOT NULL)
  )
);

CREATE TRIGGER store_lifecycle_effects_append_only
BEFORE UPDATE OR DELETE ON store_lifecycle_effects
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE OR REPLACE FUNCTION apgic_apply_store_lifecycle_event(
  p_event_id uuid,
  p_effect_id uuid,
  p_provider_instance_id uuid,
  p_provider_event_id text,
  p_external_transaction_id text,
  p_subscription_ref text,
  p_identity_id uuid,
  p_entitlement_id uuid,
  p_provider_sequence bigint,
  p_event_type text,
  p_amount_minor bigint,
  p_currency text,
  p_provider_evidence_ref text,
  p_ledger_entry_id uuid,
  p_occurred_at timestamptz
)
RETURNS TABLE(applied boolean, reason_code text)
LANGUAGE plpgsql
AS $$
DECLARE
  inserted_event boolean := false;
  current_projection store_subscription_projections%ROWTYPE;
  next_subscription_state text;
  next_entitlement_state text;
  entitlement_effect text;
  ledger_effect text;
BEGIN
  INSERT INTO store_subscription_lifecycle_events (
    id, provider_instance_id, provider_event_id, external_transaction_id,
    subscription_ref, identity_id, entitlement_id, provider_sequence,
    event_type, amount_minor, currency, provider_evidence_ref,
    ledger_entry_id, occurred_at
  ) VALUES (
    p_event_id, p_provider_instance_id, p_provider_event_id, p_external_transaction_id,
    p_subscription_ref, p_identity_id, p_entitlement_id, p_provider_sequence,
    p_event_type, p_amount_minor, p_currency, p_provider_evidence_ref,
    p_ledger_entry_id, p_occurred_at
  )
  ON CONFLICT (provider_instance_id, provider_event_id) DO NOTHING;

  inserted_event := FOUND;
  IF NOT inserted_event THEN
    RETURN QUERY SELECT false, 'STORE_EVENT_DUPLICATE';
    RETURN;
  END IF;

  SELECT *
  INTO current_projection
  FROM store_subscription_projections
  WHERE provider_instance_id = p_provider_instance_id
    AND subscription_ref = p_subscription_ref
  FOR UPDATE;

  IF FOUND AND p_provider_sequence <= current_projection.last_provider_sequence THEN
    RETURN QUERY SELECT false, 'STORE_EVENT_STALE';
    RETURN;
  END IF;

  CASE p_event_type
    WHEN 'RENEWAL' THEN
      next_subscription_state := 'ACTIVE';
      next_entitlement_state := 'ACTIVE';
      entitlement_effect := 'ACTIVATE';
      ledger_effect := 'RENEWAL_CAPTURE';
    WHEN 'REFUND' THEN
      next_subscription_state := 'REVOKED';
      next_entitlement_state := 'REVOKED';
      entitlement_effect := 'REVOKE';
      ledger_effect := 'REFUND';
    WHEN 'CHARGEBACK' THEN
      next_subscription_state := 'REVOKED';
      next_entitlement_state := 'REVOKED';
      entitlement_effect := 'REVOKE';
      ledger_effect := 'CHARGEBACK';
    WHEN 'REVOCATION' THEN
      next_subscription_state := 'REVOKED';
      next_entitlement_state := 'REVOKED';
      entitlement_effect := 'REVOKE';
      ledger_effect := 'NONE';
    WHEN 'GRACE_STARTED' THEN
      next_subscription_state := 'GRACE';
      next_entitlement_state := 'ACTIVE';
      entitlement_effect := 'KEEP';
      ledger_effect := 'NONE';
    WHEN 'HOLD_STARTED' THEN
      next_subscription_state := 'HOLD';
      next_entitlement_state := 'ACTIVE';
      entitlement_effect := 'KEEP';
      ledger_effect := 'NONE';
    WHEN 'EXPIRED' THEN
      next_subscription_state := 'EXPIRED';
      next_entitlement_state := 'EXPIRED';
      entitlement_effect := 'EXPIRE';
      ledger_effect := 'NONE';
    ELSE
      RAISE EXCEPTION 'unsupported store lifecycle event type';
  END CASE;

  INSERT INTO store_lifecycle_effects (
    id, lifecycle_event_id, entitlement_effect, ledger_effect,
    ledger_entry_id, applied_at
  ) VALUES (
    p_effect_id, p_event_id, entitlement_effect, ledger_effect,
    CASE WHEN ledger_effect = 'NONE' THEN NULL ELSE p_ledger_entry_id END,
    p_occurred_at
  );

  IF current_projection.provider_instance_id IS NULL THEN
    INSERT INTO store_subscription_projections (
      provider_instance_id, subscription_ref, identity_id, entitlement_id,
      subscription_state, entitlement_state, last_provider_sequence,
      last_event_id, applied_event_count, updated_at
    ) VALUES (
      p_provider_instance_id, p_subscription_ref, p_identity_id, p_entitlement_id,
      next_subscription_state, next_entitlement_state, p_provider_sequence,
      p_event_id, 1, p_occurred_at
    );
  ELSE
    UPDATE store_subscription_projections
    SET subscription_state = next_subscription_state,
        entitlement_state = next_entitlement_state,
        last_provider_sequence = p_provider_sequence,
        last_event_id = p_event_id,
        applied_event_count = applied_event_count + 1,
        updated_at = p_occurred_at
    WHERE provider_instance_id = p_provider_instance_id
      AND subscription_ref = p_subscription_ref;
  END IF;

  RETURN QUERY SELECT true, 'STORE_EVENT_APPLIED';
END;
$$;

COMMIT;
