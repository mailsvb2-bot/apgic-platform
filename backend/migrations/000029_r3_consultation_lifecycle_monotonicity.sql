-- R3: provider evidence must never make a completed/failed consultation
-- bookable again, or rewind an active call to READY after a late JOINED fact.
-- The state is still owned by the existing canonical PostgreSQL lifecycle.
BEGIN;

CREATE OR REPLACE FUNCTION apgic_consultation_terminal_fact_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  current_state text;
BEGIN
  SELECT state INTO current_state
    FROM consultation_sessions
   WHERE id = NEW.session_id
   FOR UPDATE;

  IF current_state IN ('COMPLETED','TECHNICAL_FAILURE') THEN
    RAISE EXCEPTION 'terminal consultation cannot accept further lifecycle facts';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER consultation_lifecycle_facts_terminal_guard
BEFORE INSERT ON consultation_lifecycle_facts
FOR EACH ROW EXECUTE FUNCTION apgic_consultation_terminal_fact_guard();

CREATE OR REPLACE FUNCTION apgic_consultation_fact_apply_state()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  next_state text;
  client_ready boolean;
  specialist_ready boolean;
BEGIN
  next_state := NULL;

  IF NEW.fact_type IN ('READY','JOINED') THEN
    SELECT EXISTS (
      SELECT 1 FROM consultation_lifecycle_facts
      WHERE session_id = NEW.session_id
        AND role = 'CLIENT'
        AND fact_type IN ('READY','JOINED')
    ) INTO client_ready;

    SELECT EXISTS (
      SELECT 1 FROM consultation_lifecycle_facts
      WHERE session_id = NEW.session_id
        AND role = 'SPECIALIST'
        AND fact_type IN ('READY','JOINED')
    ) INTO specialist_ready;

    -- Re-joining after STARTED or during recovery is valid evidence, but\n    -- cannot roll IN_PROGRESS / RECOVERING back to READY.\n    IF client_ready AND specialist_ready AND EXISTS (
      SELECT 1 FROM consultation_sessions
      WHERE id = NEW.session_id AND state IN ('SCHEDULED','READY')
    ) THEN
      next_state := 'READY';
    END IF;
  ELSIF NEW.fact_type = 'STARTED' THEN
    next_state := 'IN_PROGRESS';
  ELSIF NEW.fact_type = 'RECOVERY_STARTED' THEN
    next_state := 'RECOVERING';
  ELSIF NEW.fact_type = 'RECOVERY_SUCCEEDED' THEN
    next_state := 'IN_PROGRESS';
  ELSIF NEW.fact_type = 'TECHNICAL_FAILURE' THEN
    next_state := 'TECHNICAL_FAILURE';
  ELSIF NEW.fact_type = 'ENDED' THEN
    next_state := 'COMPLETED';
  END IF;

  IF next_state IS NOT NULL THEN
    UPDATE consultation_sessions
    SET state = next_state, updated_at = NEW.occurred_at
    WHERE id = NEW.session_id;
  ELSE
    UPDATE consultation_sessions
    SET updated_at = NEW.occurred_at
    WHERE id = NEW.session_id;
  END IF;

  RETURN NEW;
END;
$$;

COMMIT;
