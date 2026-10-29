-- Canonical, session-bound evidence for identity reconfirmation.
-- Legacy rows are retained rather than silently marked verified.
ALTER TABLE delete_account_requests
  ADD COLUMN reconfirmed_at timestamptz,
  ADD COLUMN reconfirmation_method text,
  ADD COLUMN reconfirmation_session_ref text;

ALTER TABLE delete_account_requests
  ADD CONSTRAINT delete_account_reconfirmation_evidence_required
  CHECK (
    state = 'REQUESTED'
    OR (
      reconfirmed_at IS NOT NULL
      AND reconfirmation_method = 'VERIFIED_TOTP'
      AND nullif(btrim(reconfirmation_session_ref),'') IS NOT NULL
    )
  ) NOT VALID;

-- New transitions must contain evidence even while older records are reviewed.
CREATE OR REPLACE FUNCTION apgic_delete_reconfirmation_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'UPDATE' THEN
    IF OLD.reconfirmed_at IS NOT NULL AND
       (NEW.reconfirmed_at IS DISTINCT FROM OLD.reconfirmed_at OR
        NEW.reconfirmation_method IS DISTINCT FROM OLD.reconfirmation_method OR
        NEW.reconfirmation_session_ref IS DISTINCT FROM OLD.reconfirmation_session_ref) THEN
      RAISE EXCEPTION 'deletion reconfirmation evidence is immutable';
    END IF;
    IF OLD.state = 'REQUESTED' AND NEW.state <> 'REQUESTED' AND
       (NEW.reconfirmed_at IS NULL OR NEW.reconfirmation_method <> 'VERIFIED_TOTP' OR
        nullif(btrim(NEW.reconfirmation_session_ref),'') IS NULL) THEN
      RAISE EXCEPTION 'deletion transition requires independently verified identity';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER delete_account_reconfirmation_guard
BEFORE UPDATE ON delete_account_requests
FOR EACH ROW EXECUTE FUNCTION apgic_delete_reconfirmation_guard();
