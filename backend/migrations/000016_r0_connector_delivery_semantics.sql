BEGIN;

ALTER TABLE connector_instances
  ADD COLUMN execute_scope text
    GENERATED ALWAYS AS ('connector:execute:' || capability_class) STORED,
  ADD CONSTRAINT connector_instances_capability_class_check
    CHECK (
      capability_class IN (
        'GROWTH_CRM_PROVIDER',
        'COMMUNICATION_PROVIDER',
        'PERSONA_PROVIDER',
        'MANAGEMENT_INTELLIGENCE_PROVIDER',
        'NOTIFICATION_PROVIDER',
        'CALENDAR_PROVIDER',
        'PAYMENT_PROVIDER',
        'STORAGE_PROVIDER',
        'CDN_PROVIDER',
        'ANALYTICS_PROVIDER',
        'LMS_PROVIDER',
        'ERP_PROVIDER',
        'IDENTITY_PROVIDER',
        'AI_MODEL_PROVIDER'
      )
    ),
  ADD CONSTRAINT connector_instances_execute_scope_check
    CHECK (
      execute_scope = 'connector:execute:' || capability_class
      AND execute_scope NOT LIKE '%*%'
    ),
  ADD CONSTRAINT connector_instances_provider_kind_nonempty_check
    CHECK (btrim(provider_kind) <> ''),
  ADD CONSTRAINT connector_instances_config_ref_nonempty_check
    CHECK (btrim(config_ref) <> '');

CREATE TABLE connector_stream_positions (
  connector_instance_id uuid NOT NULL
    REFERENCES connector_instances(id),
  stream_id text NOT NULL
    CHECK (btrim(stream_id) <> ''),
  last_applied_sequence bigint NOT NULL DEFAULT 0
    CHECK (last_applied_sequence >= 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (connector_instance_id, stream_id)
);

CREATE TABLE connector_delivery_receipts (
  connector_instance_id uuid NOT NULL
    REFERENCES connector_instances(id),
  external_event_id text NOT NULL
    CHECK (btrim(external_event_id) <> ''),
  stream_id text NOT NULL
    CHECK (btrim(stream_id) <> ''),
  sequence bigint NOT NULL
    CHECK (sequence > 0),
  payload_sha256 text NOT NULL
    CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  signature_key_id text NOT NULL
    CHECK (btrim(signature_key_id) <> ''),
  state text NOT NULL
    CHECK (state IN ('RECEIVED', 'APPLIED', 'DEFERRED', 'STALE')),
  received_at timestamptz NOT NULL DEFAULT now(),
  applied_at timestamptz,
  PRIMARY KEY (connector_instance_id, external_event_id),
  CONSTRAINT connector_delivery_receipt_applied_at_check
    CHECK (
      (state = 'APPLIED' AND applied_at IS NOT NULL)
      OR
      (state <> 'APPLIED' AND applied_at IS NULL)
    )
);

CREATE INDEX connector_delivery_deferred_idx
  ON connector_delivery_receipts (
    connector_instance_id,
    stream_id,
    sequence,
    received_at,
    external_event_id
  )
  WHERE state = 'DEFERRED';

CREATE OR REPLACE FUNCTION apgic_enforce_connector_delivery_transition()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'connector delivery receipt deletion is forbidden';
  END IF;
  IF (
    NEW.connector_instance_id,
    NEW.external_event_id,
    NEW.stream_id,
    NEW.sequence,
    NEW.payload_sha256,
    NEW.signature_key_id,
    NEW.received_at
  ) IS DISTINCT FROM (
    OLD.connector_instance_id,
    OLD.external_event_id,
    OLD.stream_id,
    OLD.sequence,
    OLD.payload_sha256,
    OLD.signature_key_id,
    OLD.received_at
  ) THEN
    RAISE EXCEPTION 'connector delivery identity/evidence is immutable';
  END IF;

  IF OLD.state IN ('APPLIED', 'STALE') AND NEW IS DISTINCT FROM OLD THEN
    RAISE EXCEPTION 'terminal connector delivery receipt is immutable';
  END IF;

  IF OLD.state = 'DEFERRED' AND NEW.state NOT IN ('DEFERRED', 'APPLIED', 'STALE') THEN
    RAISE EXCEPTION 'invalid deferred connector delivery transition';
  END IF;

  IF OLD.state = 'RECEIVED' AND NEW.state NOT IN ('RECEIVED', 'APPLIED', 'DEFERRED', 'STALE') THEN
    RAISE EXCEPTION 'invalid connector delivery transition';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER connector_delivery_receipts_transition_guard
BEFORE UPDATE OR DELETE ON connector_delivery_receipts
FOR EACH ROW EXECUTE FUNCTION apgic_enforce_connector_delivery_transition();

CREATE OR REPLACE FUNCTION apgic_enforce_connector_stream_position()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'connector stream position deletion is forbidden';
  END IF;

  IF NEW.connector_instance_id IS DISTINCT FROM OLD.connector_instance_id
     OR NEW.stream_id IS DISTINCT FROM OLD.stream_id THEN
    RAISE EXCEPTION 'connector stream identity is immutable';
  END IF;

  IF NEW.last_applied_sequence < OLD.last_applied_sequence THEN
    RAISE EXCEPTION 'connector stream sequence cannot decrease';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER connector_stream_positions_transition_guard
BEFORE UPDATE OR DELETE ON connector_stream_positions
FOR EACH ROW EXECUTE FUNCTION apgic_enforce_connector_stream_position();

CREATE OR REPLACE FUNCTION apgic_register_connector_delivery(
  p_connector_instance_id uuid,
  p_external_event_id text,
  p_stream_id text,
  p_sequence bigint,
  p_payload_sha256 text,
  p_signature_key_id text
)
RETURNS text
LANGUAGE plpgsql
AS $$
DECLARE
  v_last_applied_sequence bigint;
  v_existing connector_delivery_receipts%ROWTYPE;
  v_inserted integer;
BEGIN
  IF p_connector_instance_id IS NULL
     OR btrim(coalesce(p_external_event_id, '')) = ''
     OR btrim(coalesce(p_stream_id, '')) = ''
     OR p_sequence IS NULL
     OR p_sequence <= 0
     OR coalesce(p_payload_sha256, '') !~ '^[0-9a-f]{64}$'
     OR btrim(coalesce(p_signature_key_id, '')) = '' THEN
    RAISE EXCEPTION 'invalid connector delivery registration';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM connector_instances
    WHERE id = p_connector_instance_id
  ) THEN
    RAISE EXCEPTION 'connector instance does not exist';
  END IF;

  INSERT INTO connector_stream_positions (
    connector_instance_id,
    stream_id,
    last_applied_sequence,
    updated_at
  ) VALUES (
    p_connector_instance_id,
    p_stream_id,
    0,
    now()
  )
  ON CONFLICT (connector_instance_id, stream_id) DO NOTHING;

  SELECT last_applied_sequence
  INTO v_last_applied_sequence
  FROM connector_stream_positions
  WHERE connector_instance_id = p_connector_instance_id
    AND stream_id = p_stream_id
  FOR UPDATE;

  INSERT INTO connector_delivery_receipts (
    connector_instance_id,
    external_event_id,
    stream_id,
    sequence,
    payload_sha256,
    signature_key_id,
    state,
    received_at
  ) VALUES (
    p_connector_instance_id,
    p_external_event_id,
    p_stream_id,
    p_sequence,
    p_payload_sha256,
    p_signature_key_id,
    'RECEIVED',
    now()
  )
  ON CONFLICT (connector_instance_id, external_event_id) DO NOTHING;

  GET DIAGNOSTICS v_inserted = ROW_COUNT;

  IF v_inserted = 0 THEN
    SELECT *
    INTO v_existing
    FROM connector_delivery_receipts
    WHERE connector_instance_id = p_connector_instance_id
      AND external_event_id = p_external_event_id;

    IF v_existing.stream_id IS DISTINCT FROM p_stream_id
       OR v_existing.sequence IS DISTINCT FROM p_sequence
       OR v_existing.payload_sha256 IS DISTINCT FROM p_payload_sha256
       OR v_existing.signature_key_id IS DISTINCT FROM p_signature_key_id THEN
      RAISE EXCEPTION 'connector delivery identity conflict for duplicate external_event_id';
    END IF;

    RETURN 'DUPLICATE';
  END IF;

  IF p_sequence <= v_last_applied_sequence THEN
    UPDATE connector_delivery_receipts
    SET state = 'STALE'
    WHERE connector_instance_id = p_connector_instance_id
      AND external_event_id = p_external_event_id;
    RETURN 'STALE';
  END IF;

  IF p_sequence = v_last_applied_sequence + 1 THEN
    UPDATE connector_delivery_receipts
    SET state = 'APPLIED',
        applied_at = now()
    WHERE connector_instance_id = p_connector_instance_id
      AND external_event_id = p_external_event_id;

    UPDATE connector_stream_positions
    SET last_applied_sequence = p_sequence,
        updated_at = now()
    WHERE connector_instance_id = p_connector_instance_id
      AND stream_id = p_stream_id;

    RETURN 'APPLY';
  END IF;

  UPDATE connector_delivery_receipts
  SET state = 'DEFERRED'
  WHERE connector_instance_id = p_connector_instance_id
    AND external_event_id = p_external_event_id;

  RETURN 'DEFER';
END;
$$;

CREATE OR REPLACE FUNCTION apgic_promote_next_connector_delivery(
  p_connector_instance_id uuid,
  p_stream_id text
)
RETURNS text
LANGUAGE plpgsql
AS $$
DECLARE
  v_last_applied_sequence bigint;
  v_external_event_id text;
  v_next_sequence bigint;
BEGIN
  IF p_connector_instance_id IS NULL
     OR btrim(coalesce(p_stream_id, '')) = '' THEN
    RAISE EXCEPTION 'invalid connector stream promotion request';
  END IF;

  SELECT last_applied_sequence
  INTO v_last_applied_sequence
  FROM connector_stream_positions
  WHERE connector_instance_id = p_connector_instance_id
    AND stream_id = p_stream_id
  FOR UPDATE;

  IF NOT FOUND THEN
    RETURN NULL;
  END IF;

  v_next_sequence := v_last_applied_sequence + 1;

  SELECT external_event_id
  INTO v_external_event_id
  FROM connector_delivery_receipts
  WHERE connector_instance_id = p_connector_instance_id
    AND stream_id = p_stream_id
    AND sequence = v_next_sequence
    AND state = 'DEFERRED'
  ORDER BY received_at, external_event_id
  LIMIT 1
  FOR UPDATE;

  IF NOT FOUND THEN
    RETURN NULL;
  END IF;

  UPDATE connector_delivery_receipts
  SET state = 'APPLIED',
      applied_at = now()
  WHERE connector_instance_id = p_connector_instance_id
    AND external_event_id = v_external_event_id;

  UPDATE connector_delivery_receipts
  SET state = 'STALE'
  WHERE connector_instance_id = p_connector_instance_id
    AND stream_id = p_stream_id
    AND sequence = v_next_sequence
    AND state = 'DEFERRED';

  UPDATE connector_stream_positions
  SET last_applied_sequence = v_next_sequence,
      updated_at = now()
  WHERE connector_instance_id = p_connector_instance_id
    AND stream_id = p_stream_id;

  RETURN v_external_event_id;
END;
$$;

COMMIT;
