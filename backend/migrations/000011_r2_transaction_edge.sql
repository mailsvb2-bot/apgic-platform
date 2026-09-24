BEGIN;

CREATE TABLE notification_intents (
  id uuid PRIMARY KEY,
  booking_id uuid NOT NULL REFERENCES bookings(id),
  purpose text NOT NULL CHECK (btrim(purpose) <> ''),
  idempotency_key text NOT NULL UNIQUE CHECK (btrim(idempotency_key) <> ''),
  transactional boolean NOT NULL CHECK (transactional = true),
  data_class text NOT NULL CHECK (btrim(data_class) <> ''),
  channel_policy_version text NOT NULL CHECK (btrim(channel_policy_version) <> ''),
  created_at timestamptz NOT NULL
);

CREATE TRIGGER notification_intents_append_only
BEFORE UPDATE OR DELETE ON notification_intents
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE OR REPLACE FUNCTION apgic_create_notification_intent(
  p_id uuid,
  p_booking_id uuid,
  p_purpose text,
  p_idempotency_key text,
  p_data_class text,
  p_channel_policy_version text,
  p_created_at timestamptz
)
RETURNS TABLE(created boolean, reason_code text, intent_id uuid)
LANGUAGE plpgsql
AS $$
DECLARE
  existing notification_intents%ROWTYPE;
BEGIN
  IF p_id IS NULL OR
     p_booking_id IS NULL OR
     btrim(coalesce(p_purpose, '')) = '' OR
     btrim(coalesce(p_idempotency_key, '')) = '' OR
     btrim(coalesce(p_data_class, '')) = '' OR
     btrim(coalesce(p_channel_policy_version, '')) = '' OR
     p_created_at IS NULL OR
     NOT EXISTS (SELECT 1 FROM bookings WHERE id = p_booking_id) THEN
    RETURN QUERY SELECT false, 'NOTIF_INTENT_INVALID', NULL::uuid;
    RETURN;
  END IF;

  INSERT INTO notification_intents (
    id, booking_id, purpose, idempotency_key,
    transactional, data_class, channel_policy_version, created_at
  ) VALUES (
    p_id, p_booking_id, p_purpose, p_idempotency_key,
    true, p_data_class, p_channel_policy_version, p_created_at
  )
  ON CONFLICT (idempotency_key) DO NOTHING;

  IF FOUND THEN
    RETURN QUERY SELECT true, 'NOTIF_INTENT_CREATED', p_id;
    RETURN;
  END IF;

  SELECT *
  INTO existing
  FROM notification_intents
  WHERE idempotency_key = p_idempotency_key;

  IF existing.booking_id = p_booking_id AND
     existing.purpose = p_purpose AND
     existing.data_class = p_data_class AND
     existing.channel_policy_version = p_channel_policy_version THEN
    RETURN QUERY SELECT false, 'NOTIF_INTENT_DUPLICATE', existing.id;
  ELSE
    RETURN QUERY SELECT false, 'NOTIF_INTENT_IDEMPOTENCY_CONFLICT', existing.id;
  END IF;
END;
$$;

CREATE TABLE notification_deliveries (
  id uuid PRIMARY KEY,
  intent_id uuid NOT NULL REFERENCES notification_intents(id),
  channel text NOT NULL CHECK (channel IN ('PUSH','EMAIL','SMS')),
  endpoint_ref text NOT NULL CHECK (btrim(endpoint_ref) <> ''),
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  delivery_idempotency_key text NOT NULL UNIQUE CHECK (btrim(delivery_idempotency_key) <> ''),
  state text NOT NULL CHECK (
    state IN ('PENDING','SENT','DELIVERED','FAILED_RETRYABLE','SUPPRESSED')
  ),
  provider_reference text,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE OR REPLACE FUNCTION apgic_notification_delivery_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  capability text;
BEGIN
  SELECT capability_class
  INTO capability
  FROM connector_instances
  WHERE id = NEW.provider_instance_id;

  IF NOT FOUND OR capability <> 'NOTIFICATION_PROVIDER' THEN
    RAISE EXCEPTION 'notification delivery requires NOTIFICATION_PROVIDER connector';
  END IF;

  IF TG_OP = 'INSERT' THEN
    IF NEW.state <> 'PENDING' OR NEW.updated_at <> NEW.created_at THEN
      RAISE EXCEPTION 'new notification delivery must start PENDING';
    END IF;
    RETURN NEW;
  END IF;

  IF NEW.intent_id <> OLD.intent_id OR
     NEW.channel <> OLD.channel OR
     NEW.endpoint_ref <> OLD.endpoint_ref OR
     NEW.provider_instance_id <> OLD.provider_instance_id OR
     NEW.delivery_idempotency_key <> OLD.delivery_idempotency_key OR
     NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'notification transport identity is immutable';
  END IF;

  IF NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'notification delivery updated_at cannot move backwards';
  END IF;

  IF OLD.provider_reference IS NOT NULL AND
     NEW.provider_reference IS DISTINCT FROM OLD.provider_reference THEN
    RAISE EXCEPTION 'notification provider reference cannot be rewritten';
  END IF;

  IF OLD.state = NEW.state THEN
    RETURN NEW;
  END IF;

  IF OLD.state = 'PENDING' AND NEW.state IN ('SENT','FAILED_RETRYABLE','SUPPRESSED') THEN
    RETURN NEW;
  ELSIF OLD.state = 'SENT' AND NEW.state IN ('DELIVERED','FAILED_RETRYABLE') THEN
    RETURN NEW;
  ELSIF OLD.state = 'FAILED_RETRYABLE' AND NEW.state = 'SENT' THEN
    RETURN NEW;
  END IF;

  RAISE EXCEPTION 'invalid notification delivery transition: % -> %', OLD.state, NEW.state;
END;
$$;

CREATE TRIGGER notification_deliveries_insert_guard
BEFORE INSERT ON notification_deliveries
FOR EACH ROW EXECUTE FUNCTION apgic_notification_delivery_guard();

CREATE TRIGGER notification_deliveries_transition_guard
BEFORE UPDATE ON notification_deliveries
FOR EACH ROW EXECUTE FUNCTION apgic_notification_delivery_guard();

CREATE TRIGGER notification_deliveries_no_delete
BEFORE DELETE ON notification_deliveries
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE calendar_sync_jobs (
  id uuid PRIMARY KEY,
  booking_id uuid NOT NULL REFERENCES bookings(id),
  booking_version bigint NOT NULL CHECK (booking_version > 0),
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  target_ref text NOT NULL CHECK (btrim(target_ref) <> ''),
  idempotency_key text NOT NULL UNIQUE CHECK (btrim(idempotency_key) <> ''),
  state text NOT NULL CHECK (state IN ('PENDING','SYNCED','CONFLICT','FAILED_RETRYABLE')),
  provider_event_ref text,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE OR REPLACE FUNCTION apgic_calendar_sync_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  capability text;
BEGIN
  SELECT capability_class
  INTO capability
  FROM connector_instances
  WHERE id = NEW.provider_instance_id;

  IF NOT FOUND OR capability <> 'CALENDAR_PROVIDER' THEN
    RAISE EXCEPTION 'calendar sync requires CALENDAR_PROVIDER connector';
  END IF;

  IF TG_OP = 'INSERT' THEN
    IF NEW.state <> 'PENDING' OR NEW.updated_at <> NEW.created_at THEN
      RAISE EXCEPTION 'new calendar sync must start PENDING';
    END IF;
    RETURN NEW;
  END IF;

  IF NEW.booking_id <> OLD.booking_id OR
     NEW.booking_version <> OLD.booking_version OR
     NEW.provider_instance_id <> OLD.provider_instance_id OR
     NEW.target_ref <> OLD.target_ref OR
     NEW.idempotency_key <> OLD.idempotency_key OR
     NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'calendar projection identity is immutable';
  END IF;

  IF NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'calendar sync updated_at cannot move backwards';
  END IF;

  IF OLD.provider_event_ref IS NOT NULL AND
     NEW.provider_event_ref IS DISTINCT FROM OLD.provider_event_ref THEN
    RAISE EXCEPTION 'calendar provider event reference cannot be rewritten';
  END IF;

  IF OLD.state = NEW.state THEN
    RETURN NEW;
  END IF;

  IF NEW.state = 'SYNCED' AND
     (NEW.provider_event_ref IS NULL OR btrim(NEW.provider_event_ref) = '') THEN
    RAISE EXCEPTION 'SYNCED calendar projection requires provider event evidence';
  END IF;

  IF OLD.state IN ('PENDING','FAILED_RETRYABLE') AND
     NEW.state IN ('SYNCED','CONFLICT','FAILED_RETRYABLE') THEN
    RETURN NEW;
  END IF;

  RAISE EXCEPTION 'invalid calendar sync transition: % -> %', OLD.state, NEW.state;
END;
$$;

CREATE TRIGGER calendar_sync_jobs_insert_guard
BEFORE INSERT ON calendar_sync_jobs
FOR EACH ROW EXECUTE FUNCTION apgic_calendar_sync_guard();

CREATE TRIGGER calendar_sync_jobs_transition_guard
BEFORE UPDATE ON calendar_sync_jobs
FOR EACH ROW EXECUTE FUNCTION apgic_calendar_sync_guard();

CREATE TRIGGER calendar_sync_jobs_no_delete
BEFORE DELETE ON calendar_sync_jobs
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE client_mutation_records (
  id uuid PRIMARY KEY,
  identity_id uuid NOT NULL REFERENCES identities(id),
  operation text NOT NULL CHECK (btrim(operation) <> ''),
  idempotency_key text NOT NULL CHECK (btrim(idempotency_key) <> ''),
  request_digest text NOT NULL CHECK (btrim(request_digest) <> ''),
  state text NOT NULL CHECK (state IN ('CLAIMED','APPLIED')),
  side_effect_ref text,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (identity_id, operation, idempotency_key)
);

CREATE OR REPLACE FUNCTION apgic_client_mutation_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    IF NEW.state <> 'CLAIMED' OR
       NEW.side_effect_ref IS NOT NULL OR
       NEW.updated_at <> NEW.created_at THEN
      RAISE EXCEPTION 'new client mutation must start CLAIMED without side effect';
    END IF;
    RETURN NEW;
  END IF;

  IF NEW.identity_id <> OLD.identity_id OR
     NEW.operation <> OLD.operation OR
     NEW.idempotency_key <> OLD.idempotency_key OR
     NEW.request_digest <> OLD.request_digest OR
     NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'client mutation identity/payload digest are immutable';
  END IF;

  IF NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'client mutation updated_at cannot move backwards';
  END IF;

  IF OLD.state = NEW.state THEN
    IF NEW.side_effect_ref IS DISTINCT FROM OLD.side_effect_ref THEN
      RAISE EXCEPTION 'client mutation side effect cannot change without transition';
    END IF;
    RETURN NEW;
  END IF;

  IF OLD.state = 'CLAIMED' AND NEW.state = 'APPLIED' AND
     NEW.side_effect_ref IS NOT NULL AND btrim(NEW.side_effect_ref) <> '' THEN
    RETURN NEW;
  END IF;

  RAISE EXCEPTION 'invalid client mutation transition: % -> %', OLD.state, NEW.state;
END;
$$;

CREATE TRIGGER client_mutation_records_insert_guard
BEFORE INSERT ON client_mutation_records
FOR EACH ROW EXECUTE FUNCTION apgic_client_mutation_guard();

CREATE TRIGGER client_mutation_records_transition_guard
BEFORE UPDATE ON client_mutation_records
FOR EACH ROW EXECUTE FUNCTION apgic_client_mutation_guard();

CREATE TRIGGER client_mutation_records_no_delete
BEFORE DELETE ON client_mutation_records
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE OR REPLACE FUNCTION apgic_claim_client_mutation(
  p_id uuid,
  p_identity_id uuid,
  p_operation text,
  p_idempotency_key text,
  p_request_digest text,
  p_now timestamptz
)
RETURNS TABLE(outcome text, mutation_id uuid, mutation_state text, side_effect_ref text)
LANGUAGE plpgsql
AS $$
DECLARE
  existing client_mutation_records%ROWTYPE;
BEGIN
  IF p_id IS NULL OR p_identity_id IS NULL OR
     btrim(coalesce(p_operation, '')) = '' OR
     btrim(coalesce(p_idempotency_key, '')) = '' OR
     btrim(coalesce(p_request_digest, '')) = '' OR
     p_now IS NULL THEN
    RETURN QUERY SELECT 'INVALID', NULL::uuid, NULL::text, NULL::text;
    RETURN;
  END IF;

  INSERT INTO client_mutation_records (
    id, identity_id, operation, idempotency_key,
    request_digest, state, created_at, updated_at
  ) VALUES (
    p_id, p_identity_id, p_operation, p_idempotency_key,
    p_request_digest, 'CLAIMED', p_now, p_now
  )
  ON CONFLICT (identity_id, operation, idempotency_key) DO NOTHING;

  IF FOUND THEN
    RETURN QUERY SELECT 'CLAIMED', p_id, 'CLAIMED', NULL::text;
    RETURN;
  END IF;

  SELECT *
  INTO existing
  FROM client_mutation_records
  WHERE identity_id = p_identity_id
    AND operation = p_operation
    AND idempotency_key = p_idempotency_key;

  IF existing.request_digest <> p_request_digest THEN
    RETURN QUERY SELECT 'CONFLICT', existing.id, existing.state, existing.side_effect_ref;
  ELSE
    RETURN QUERY SELECT
      CASE WHEN existing.state = 'APPLIED' THEN 'DUPLICATE_APPLIED' ELSE 'DUPLICATE' END,
      existing.id,
      existing.state,
      existing.side_effect_ref;
  END IF;
END;
$$;

CREATE OR REPLACE FUNCTION apgic_finalize_client_mutation(
  p_mutation_id uuid,
  p_side_effect_ref text,
  p_now timestamptz
)
RETURNS TABLE(applied boolean, reason_code text)
LANGUAGE plpgsql
AS $$
DECLARE
  current_record client_mutation_records%ROWTYPE;
BEGIN
  SELECT *
  INTO current_record
  FROM client_mutation_records
  WHERE id = p_mutation_id
  FOR UPDATE;

  IF NOT FOUND OR btrim(coalesce(p_side_effect_ref, '')) = '' OR p_now IS NULL THEN
    RETURN QUERY SELECT false, 'MUTATION_FINALIZE_INVALID';
    RETURN;
  END IF;

  IF current_record.state = 'APPLIED' THEN
    IF current_record.side_effect_ref = p_side_effect_ref THEN
      RETURN QUERY SELECT false, 'MUTATION_ALREADY_APPLIED';
    ELSE
      RETURN QUERY SELECT false, 'MUTATION_SIDE_EFFECT_CONFLICT';
    END IF;
    RETURN;
  END IF;

  UPDATE client_mutation_records
  SET state = 'APPLIED',
      side_effect_ref = p_side_effect_ref,
      updated_at = p_now
  WHERE id = p_mutation_id;

  RETURN QUERY SELECT true, 'MUTATION_APPLIED';
END;
$$;

COMMIT;
