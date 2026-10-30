-- Explicit confirmation from an already authenticated first-party client session.
-- Preserve historical TOTP evidence and the existing canonical deletion lifecycle.
ALTER TABLE delete_account_requests DROP CONSTRAINT IF EXISTS delete_account_reconfirmation_evidence_required;
ALTER TABLE delete_account_requests ADD CONSTRAINT delete_account_reconfirmation_evidence_required
CHECK (
  state = 'REQUESTED' OR (
    reconfirmed_at IS NOT NULL
    AND reconfirmation_method IN ('VERIFIED_TOTP', 'SESSION_EXPLICIT')
    AND nullif(btrim(reconfirmation_session_ref),'') IS NOT NULL
  )
) NOT VALID;

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
       (NEW.reconfirmed_at IS NULL OR NEW.reconfirmation_method NOT IN ('VERIFIED_TOTP','SESSION_EXPLICIT')
        OR nullif(btrim(NEW.reconfirmation_session_ref),'') IS NULL) THEN
      RAISE EXCEPTION 'deletion transition requires authenticated and explicit confirmation';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;
