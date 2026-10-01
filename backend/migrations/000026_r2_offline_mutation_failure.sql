ALTER TABLE client_mutation_records
  DROP CONSTRAINT client_mutation_records_state_check;

ALTER TABLE client_mutation_records
  ADD COLUMN failure_code text,
  ADD COLUMN correlation_id text;

UPDATE client_mutation_records
SET correlation_id = 'legacy:' || id::text
WHERE correlation_id IS NULL;

ALTER TABLE client_mutation_records
  ALTER COLUMN correlation_id SET NOT NULL,
  ADD CONSTRAINT client_mutation_records_state_check
    CHECK (state IN ('CLAIMED','APPLIED','FAILED')),
  ADD CONSTRAINT client_mutation_records_correlation_nonempty
    CHECK (btrim(correlation_id) <> ''),
  ADD CONSTRAINT client_mutation_records_terminal_shape_check CHECK (
    (state = 'CLAIMED' AND side_effect_ref IS NULL AND failure_code IS NULL) OR
    (state = 'APPLIED' AND side_effect_ref IS NOT NULL AND btrim(side_effect_ref) <> '' AND failure_code IS NULL) OR
    (state = 'FAILED' AND side_effect_ref IS NULL AND failure_code IS NOT NULL AND btrim(failure_code) <> '')
  );

CREATE OR REPLACE FUNCTION apgic_client_mutation_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    IF NEW.state <> 'CLAIMED' OR
       NEW.side_effect_ref IS NOT NULL OR
       NEW.failure_code IS NOT NULL OR
       NEW.updated_at <> NEW.created_at THEN
      RAISE EXCEPTION 'new client mutation must start CLAIMED without terminal outcome';
    END IF;
    RETURN NEW;
  END IF;

  IF NEW.identity_id <> OLD.identity_id OR
     NEW.operation <> OLD.operation OR
     NEW.idempotency_key <> OLD.idempotency_key OR
     NEW.correlation_id <> OLD.correlation_id OR
     NEW.request_digest <> OLD.request_digest OR
     NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'client mutation identity/payload digest are immutable';
  END IF;

  IF NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'client mutation updated_at cannot move backwards';
  END IF;

  IF OLD.state IN ('APPLIED','FAILED') AND NEW.state <> OLD.state THEN
    RAISE EXCEPTION 'terminal client mutation cannot transition: % -> %', OLD.state, NEW.state;
  END IF;

  IF OLD.state = NEW.state THEN
    IF NEW.side_effect_ref IS DISTINCT FROM OLD.side_effect_ref OR
       NEW.failure_code IS DISTINCT FROM OLD.failure_code THEN
      RAISE EXCEPTION 'client mutation terminal evidence cannot change without transition';
    END IF;
    RETURN NEW;
  END IF;

  IF OLD.state = 'CLAIMED' AND NEW.state = 'APPLIED' AND
     NEW.side_effect_ref IS NOT NULL AND btrim(NEW.side_effect_ref) <> '' AND
     NEW.failure_code IS NULL THEN
    RETURN NEW;
  END IF;

  IF OLD.state = 'CLAIMED' AND NEW.state = 'FAILED' AND
     NEW.side_effect_ref IS NULL AND
     NEW.failure_code IS NOT NULL AND btrim(NEW.failure_code) <> '' THEN
    RETURN NEW;
  END IF;

  RAISE EXCEPTION 'invalid client mutation transition: % -> %', OLD.state, NEW.state;
END;
$$;

DROP FUNCTION apgic_claim_client_mutation(uuid, uuid, text, text, text, timestamptz);

CREATE OR REPLACE FUNCTION apgic_claim_client_mutation(
  p_id uuid,
  p_identity_id uuid,
  p_operation text,
  p_idempotency_key text,
  p_correlation_id text,
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
     btrim(coalesce(p_correlation_id, '')) = '' OR
     btrim(coalesce(p_request_digest, '')) = '' OR
     p_now IS NULL THEN
    RETURN QUERY SELECT 'INVALID', NULL::uuid, NULL::text, NULL::text;
    RETURN;
  END IF;

  INSERT INTO client_mutation_records (
    id, identity_id, operation, idempotency_key, correlation_id,
    request_digest, state, created_at, updated_at
  ) VALUES (
    p_id, p_identity_id, p_operation, p_idempotency_key, p_correlation_id,
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

  IF existing.correlation_id <> p_correlation_id OR
     existing.request_digest <> p_request_digest THEN
    RETURN QUERY SELECT 'CONFLICT', existing.id, existing.state, existing.side_effect_ref;
  ELSE
    RETURN QUERY SELECT
      CASE
        WHEN existing.state = 'APPLIED' THEN 'DUPLICATE_APPLIED'
        WHEN existing.state = 'FAILED' THEN 'FAILED'
        ELSE 'DUPLICATE'
      END,
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

  IF current_record.state = 'FAILED' THEN
    RETURN QUERY SELECT false, 'MUTATION_ALREADY_FAILED';
    RETURN;
  END IF;

  UPDATE client_mutation_records
  SET state = 'APPLIED',
      side_effect_ref = p_side_effect_ref,
      failure_code = NULL,
      updated_at = p_now
  WHERE id = p_mutation_id;

  RETURN QUERY SELECT true, 'MUTATION_APPLIED';
END;
$$;

CREATE OR REPLACE FUNCTION apgic_fail_client_mutation(
  p_mutation_id uuid,
  p_failure_code text,
  p_now timestamptz
)
RETURNS TABLE(failed boolean, reason_code text)
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

  IF NOT FOUND OR btrim(coalesce(p_failure_code, '')) = '' OR p_now IS NULL THEN
    RETURN QUERY SELECT false, 'MUTATION_FAIL_INVALID';
    RETURN;
  END IF;

  IF current_record.state = 'APPLIED' THEN
    RETURN QUERY SELECT false, 'MUTATION_ALREADY_APPLIED';
    RETURN;
  END IF;

  IF current_record.state = 'FAILED' THEN
    IF current_record.failure_code = p_failure_code THEN
      RETURN QUERY SELECT false, 'MUTATION_ALREADY_FAILED';
    ELSE
      RETURN QUERY SELECT false, 'MUTATION_FAILURE_CONFLICT';
    END IF;
    RETURN;
  END IF;

  UPDATE client_mutation_records
  SET state = 'FAILED',
      side_effect_ref = NULL,
      failure_code = p_failure_code,
      updated_at = p_now
  WHERE id = p_mutation_id;

  RETURN QUERY SELECT true, 'MUTATION_FAILED';
END;
$$;
