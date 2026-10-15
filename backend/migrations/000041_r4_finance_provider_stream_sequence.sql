-- Persist signed provider stream ordering for R4 financial evidence.
-- Evidence remains append-only; the cursor is a mutable, monotonic projection
-- used only to serialize delivery and reject gaps/replays before financial mutation.
ALTER TABLE finance_provider_evidence
  ADD COLUMN stream_id text,
  ADD COLUMN stream_sequence bigint;

UPDATE finance_provider_evidence
   SET stream_id = 'legacy/' || id::text,
       stream_sequence = 1
 WHERE stream_id IS NULL OR stream_sequence IS NULL;

ALTER TABLE finance_provider_evidence
  ALTER COLUMN stream_id SET NOT NULL,
  ALTER COLUMN stream_sequence SET NOT NULL,
  ADD CONSTRAINT finance_provider_evidence_stream_nonblank CHECK (btrim(stream_id) <> ''),
  ADD CONSTRAINT finance_provider_evidence_stream_sequence_positive CHECK (stream_sequence > 0);

CREATE UNIQUE INDEX finance_provider_evidence_stream_sequence_unique
  ON finance_provider_evidence(provider_instance_id, stream_id, stream_sequence);

CREATE TABLE finance_provider_stream_cursors (
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  stream_id text NOT NULL CHECK (btrim(stream_id) <> ''),
  last_sequence bigint NOT NULL CHECK (last_sequence > 0),
  last_external_event_id text NOT NULL CHECK (btrim(last_external_event_id) <> ''),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (provider_instance_id, stream_id)
);

CREATE OR REPLACE FUNCTION apgic_finance_stream_cursor_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'finance provider stream cursor cannot be deleted';
  END IF;
  IF NEW.provider_instance_id <> OLD.provider_instance_id
     OR NEW.stream_id <> OLD.stream_id
     OR NEW.last_sequence <> OLD.last_sequence + 1
     OR NEW.last_external_event_id = OLD.last_external_event_id THEN
    RAISE EXCEPTION 'finance provider stream cursor must advance by exactly one event';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER finance_provider_stream_cursor_guard
  BEFORE UPDATE OR DELETE ON finance_provider_stream_cursors
  FOR EACH ROW EXECUTE FUNCTION apgic_finance_stream_cursor_guard();
