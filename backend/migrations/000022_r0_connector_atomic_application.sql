BEGIN;

ALTER TABLE connector_delivery_receipts
  ADD COLUMN payload jsonb;

-- Rows created by the pre-000022 state machine cannot be replayed safely:
-- the old schema never persisted the verified webhook payload. Fail closed by
-- terminalizing only non-applied legacy work rather than pretending it can be
-- promoted into a business effect.
UPDATE connector_delivery_receipts
SET state = 'STALE'
WHERE payload IS NULL
  AND state IN ('RECEIVED', 'DEFERRED');

ALTER TABLE connector_delivery_receipts
  DROP CONSTRAINT IF EXISTS connector_delivery_receipts_state_check;

ALTER TABLE connector_delivery_receipts
  ADD CONSTRAINT connector_delivery_receipts_state_check
  CHECK (state IN ('RECEIVED', 'READY', 'APPLIED', 'DEFERRED', 'STALE'));

ALTER TABLE connector_delivery_receipts
  DROP CONSTRAINT IF EXISTS connector_delivery_receipt_applied_at_check;

ALTER TABLE connector_delivery_receipts
  ADD CONSTRAINT connector_delivery_receipt_applied_at_check
  CHECK (
    (state = 'APPLIED' AND applied_at IS NOT NULL)
    OR
    (state <> 'APPLIED' AND applied_at IS NULL)
  );

DROP FUNCTION IF EXISTS apgic_register_connector_delivery(uuid, text, text, bigint, text, text);

CREATE OR REPLACE FUNCTION apgic_register_connector_delivery(
  p_connector_instance_id uuid,
  p_external_event_id text,
  p_stream_id text,
  p_sequence bigint,
  p_payload jsonb,
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
     OR p_payload IS NULL
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
    payload,
    payload_sha256,
    signature_key_id,
    state,
    received_at
  ) VALUES (
    p_connector_instance_id,
    p_external_event_id,
    p_stream_id,
    p_sequence,
    p_payload,
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
      AND external_event_id = p_external_event_id
    FOR UPDATE;

    IF v_existing.stream_id IS DISTINCT FROM p_stream_id
       OR v_existing.sequence IS DISTINCT FROM p_sequence
       OR v_existing.payload IS DISTINCT FROM p_payload
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
    SET state = 'READY'
    WHERE connector_instance_id = p_connector_instance_id
      AND external_event_id = p_external_event_id;
    RETURN 'APPLY';
  END IF;

  UPDATE connector_delivery_receipts
  SET state = 'DEFERRED'
  WHERE connector_instance_id = p_connector_instance_id
    AND external_event_id = p_external_event_id;

  RETURN 'DEFER';
END;
$$;

CREATE OR REPLACE FUNCTION apgic_finalize_connector_delivery(
  p_connector_instance_id uuid,
  p_external_event_id text
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
  v_receipt connector_delivery_receipts%ROWTYPE;
  v_last_applied_sequence bigint;
BEGIN
  SELECT *
  INTO v_receipt
  FROM connector_delivery_receipts
  WHERE connector_instance_id = p_connector_instance_id
    AND external_event_id = p_external_event_id
  FOR UPDATE;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'connector delivery receipt does not exist';
  END IF;

  IF v_receipt.state <> 'READY' THEN
    RAISE EXCEPTION 'connector delivery receipt is not ready for application';
  END IF;

  SELECT last_applied_sequence
  INTO v_last_applied_sequence
  FROM connector_stream_positions
  WHERE connector_instance_id = p_connector_instance_id
    AND stream_id = v_receipt.stream_id
  FOR UPDATE;

  IF NOT FOUND OR v_receipt.sequence <> v_last_applied_sequence + 1 THEN
    RAISE EXCEPTION 'connector delivery sequence is not next for application';
  END IF;

  UPDATE connector_delivery_receipts
  SET state = 'APPLIED',
      applied_at = now()
  WHERE connector_instance_id = p_connector_instance_id
    AND external_event_id = p_external_event_id;

  UPDATE connector_delivery_receipts
  SET state = 'STALE'
  WHERE connector_instance_id = p_connector_instance_id
    AND stream_id = v_receipt.stream_id
    AND sequence = v_receipt.sequence
    AND external_event_id <> p_external_event_id
    AND state IN ('RECEIVED', 'READY', 'DEFERRED');

  UPDATE connector_stream_positions
  SET last_applied_sequence = v_receipt.sequence,
      updated_at = now()
  WHERE connector_instance_id = p_connector_instance_id
    AND stream_id = v_receipt.stream_id;
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
    AND payload IS NOT NULL
  ORDER BY received_at, external_event_id
  LIMIT 1
  FOR UPDATE;

  IF NOT FOUND THEN
    RETURN NULL;
  END IF;

  UPDATE connector_delivery_receipts
  SET state = 'READY'
  WHERE connector_instance_id = p_connector_instance_id
    AND external_event_id = v_external_event_id;

  RETURN v_external_event_id;
END;
$$;

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
    NEW.payload,
    NEW.payload_sha256,
    NEW.signature_key_id,
    NEW.received_at
  ) IS DISTINCT FROM (
    OLD.connector_instance_id,
    OLD.external_event_id,
    OLD.stream_id,
    OLD.sequence,
    OLD.payload,
    OLD.payload_sha256,
    OLD.signature_key_id,
    OLD.received_at
  ) THEN
    RAISE EXCEPTION 'connector delivery identity/evidence is immutable';
  END IF;

  IF OLD.state IN ('APPLIED', 'STALE') AND NEW IS DISTINCT FROM OLD THEN
    RAISE EXCEPTION 'terminal connector delivery receipt is immutable';
  END IF;

  IF OLD.state = 'DEFERRED' AND NEW.state NOT IN ('DEFERRED', 'READY', 'STALE') THEN
    RAISE EXCEPTION 'invalid deferred connector delivery transition';
  END IF;

  IF OLD.state = 'READY' AND NEW.state NOT IN ('READY', 'APPLIED', 'STALE') THEN
    RAISE EXCEPTION 'invalid ready connector delivery transition';
  END IF;

  IF OLD.state = 'RECEIVED' AND NEW.state NOT IN ('RECEIVED', 'READY', 'DEFERRED', 'STALE') THEN
    RAISE EXCEPTION 'invalid connector delivery transition';
  END IF;

  RETURN NEW;
END;
$$;

COMMIT;
