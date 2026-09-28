BEGIN;

CREATE TABLE specialist_evidence (
  id uuid PRIMARY KEY,
  specialist_id uuid NOT NULL,
  topic_id text NOT NULL,
  kind text NOT NULL CHECK (nullif(btrim(kind), '') IS NOT NULL),
  evidence_ref text NOT NULL CHECK (nullif(btrim(evidence_ref), '') IS NOT NULL),
  state text NOT NULL DEFAULT 'SUBMITTED' CHECK (state IN ('SUBMITTED','ACCEPTED','REJECTED')),
  submitted_at timestamptz NOT NULL DEFAULT now(),
  reviewed_at timestamptz,
  reviewer_ref text,
  FOREIGN KEY (specialist_id, topic_id)
    REFERENCES specialist_capabilities(specialist_id, topic_id),
  UNIQUE (specialist_id, topic_id, evidence_ref),
  CHECK (
    (state = 'SUBMITTED' AND reviewed_at IS NULL AND reviewer_ref IS NULL)
    OR
    (state IN ('ACCEPTED','REJECTED') AND reviewed_at IS NOT NULL AND nullif(btrim(reviewer_ref), '') IS NOT NULL)
  )
);

CREATE INDEX specialist_evidence_subject_idx
  ON specialist_evidence (specialist_id, topic_id, submitted_at DESC);

CREATE OR REPLACE FUNCTION apgic_specialist_evidence_transition_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.specialist_id <> OLD.specialist_id
    OR NEW.topic_id <> OLD.topic_id
    OR NEW.kind <> OLD.kind
    OR NEW.evidence_ref <> OLD.evidence_ref
    OR NEW.submitted_at <> OLD.submitted_at THEN
    RAISE EXCEPTION 'specialist evidence identity/content is immutable';
  END IF;

  IF OLD.state <> 'SUBMITTED' AND NEW.state <> OLD.state THEN
    RAISE EXCEPTION 'reviewed specialist evidence cannot transition again';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER specialist_evidence_transition_guard
BEFORE UPDATE ON specialist_evidence
FOR EACH ROW EXECUTE FUNCTION apgic_specialist_evidence_transition_guard();

CREATE TRIGGER specialist_evidence_no_delete
BEFORE DELETE ON specialist_evidence
FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

COMMIT;
