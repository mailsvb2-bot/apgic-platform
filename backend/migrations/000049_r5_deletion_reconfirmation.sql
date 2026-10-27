-- R5: independent, immutable evidence for identity re-confirmation.
-- A signed session alone does not establish fresh reauthentication.
CREATE TABLE client_deletion_reconfirmations (
 request_id uuid PRIMARY KEY REFERENCES delete_account_requests(id) ON DELETE RESTRICT,
 identity_id uuid NOT NULL REFERENCES identities(id) ON DELETE RESTRICT,
 method text NOT NULL CHECK (method IN ('WEBAUTHN','TOTP')),
 session_ref_hash text NOT NULL CHECK (length(session_ref_hash) BETWEEN 32 AND 256),
 verified_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER client_deletion_reconfirmations_immutable
 BEFORE UPDATE OR DELETE ON client_deletion_reconfirmations
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

-- A deletion request must not transition to reconfirmed simply because a
-- privileged backend updated its state without durable verification evidence.
CREATE OR REPLACE FUNCTION apgic_deletion_requires_reconfirmation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.state='IDENTITY_RECONFIRMED' AND OLD.state='REQUESTED'
  AND NOT EXISTS (
   SELECT 1 FROM client_deletion_reconfirmations e
   WHERE e.request_id=OLD.id AND e.identity_id=OLD.identity_id
  ) THEN
  RAISE EXCEPTION 'deletion requires independently verified step-up evidence';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER delete_account_requests_reconfirmation_required
 BEFORE UPDATE ON delete_account_requests
 FOR EACH ROW EXECUTE FUNCTION apgic_deletion_requires_reconfirmation();
