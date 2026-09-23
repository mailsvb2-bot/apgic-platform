BEGIN;

ALTER TABLE organization_directions
  ADD CONSTRAINT organization_directions_archive_timestamp_check
  CHECK (
    (status = 'ACTIVE' AND archived_at IS NULL)
    OR
    (status = 'ARCHIVED' AND archived_at IS NOT NULL)
  );

CREATE OR REPLACE FUNCTION apgic_enforce_outbox_transition()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF (
    NEW.event_id,
    NEW.idempotency_key,
    NEW.event_type,
    NEW.schema_version,
    NEW.aggregate_ref,
    NEW.aggregate_version,
    NEW.tenant_scope,
    NEW.correlation_id,
    NEW.causation_id,
    NEW.occurred_at,
    NEW.produced_at,
    NEW.producer,
    NEW.payload
  ) IS DISTINCT FROM (
    OLD.event_id,
    OLD.idempotency_key,
    OLD.event_type,
    OLD.schema_version,
    OLD.aggregate_ref,
    OLD.aggregate_version,
    OLD.tenant_scope,
    OLD.correlation_id,
    OLD.causation_id,
    OLD.occurred_at,
    OLD.produced_at,
    OLD.producer,
    OLD.payload
  ) THEN
    RAISE EXCEPTION 'outbox event identity/payload is immutable after insert';
  END IF;

  IF NEW.attempts < OLD.attempts THEN
    RAISE EXCEPTION 'outbox delivery attempts cannot decrease';
  END IF;

  IF OLD.delivery_status = 'DELIVERED' AND NEW IS DISTINCT FROM OLD THEN
    RAISE EXCEPTION 'delivered outbox record is terminal and immutable';
  END IF;

  IF OLD.delivery_status = 'PENDING'
     AND NEW.delivery_status = 'DELIVERED'
     AND NEW.delivered_at IS NULL THEN
    RAISE EXCEPTION 'delivered outbox record requires delivered_at';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER outbox_events_transition_guard
BEFORE UPDATE ON outbox_events
FOR EACH ROW EXECUTE FUNCTION apgic_enforce_outbox_transition();

COMMIT;
