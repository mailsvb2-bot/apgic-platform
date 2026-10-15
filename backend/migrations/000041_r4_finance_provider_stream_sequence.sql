-- Persist signed provider stream ordering for R4 financial evidence.
-- Existing evidence is append-only and must not be rewritten during migration;
-- old rows keep NULL stream metadata and remain replay-safe by event ID + digest.
-- New webhook rows always carry a stream ID and positive sequence.
ALTER TABLE finance_provider_evidence
  ADD COLUMN stream_id text,
  ADD COLUMN stream_sequence bigint;

ALTER TABLE finance_provider_evidence
  ADD CONSTRAINT finance_provider_evidence_stream_metadata_pair
    CHECK ((stream_id IS NULL AND stream_sequence IS NULL)
        OR (stream_id IS NOT NULL AND btrim(stream_id) <> '' AND stream_sequence > 0));

CREATE UNIQUE INDEX finance_provider_evidence_stream_sequence_unique
  ON finance_provider_evidence(provider_instance_id, stream_id, stream_sequence)
  WHERE stream_id IS NOT NULL;

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
