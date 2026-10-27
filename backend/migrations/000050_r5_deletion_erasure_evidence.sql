-- Persistent proof of the approved policy and every locally executed erasure.
-- Existing canonical delete_account_requests remains the only lifecycle owner.
CREATE TABLE client_deletion_policy_approvals (
  request_id uuid PRIMARY KEY REFERENCES delete_account_requests(id) ON DELETE RESTRICT,
  policy_digest text NOT NULL CHECK (policy_digest ~ '^[0-9a-f]{64}$'),
  policy_snapshot jsonb NOT NULL CHECK (jsonb_typeof(policy_snapshot)='object'),
  verified_at timestamptz NOT NULL,
  approved_by text NOT NULL CHECK (btrim(approved_by)<>'')
);
CREATE TABLE client_deletion_local_erasure_receipts (
  request_id uuid NOT NULL REFERENCES delete_account_requests(id) ON DELETE RESTRICT,
  data_class text NOT NULL CHECK (data_class IN (
    'PERSONAL_PREFERENCES','HELP_INTENT_PERSONAL_DATA','PUSH_ENDPOINTS'
  )),
  affected_rows bigint NOT NULL CHECK (affected_rows>=0),
  policy_digest text NOT NULL REFERENCES client_deletion_policy_approvals(policy_digest),
  executed_at timestamptz NOT NULL,
  PRIMARY KEY(request_id,data_class)
);
CREATE UNIQUE INDEX client_deletion_policy_digest_idx
  ON client_deletion_policy_approvals(policy_digest,request_id);

CREATE TABLE client_deletion_revocations (
  identity_id uuid PRIMARY KEY REFERENCES identities(id) ON DELETE RESTRICT,
  request_id uuid UNIQUE NOT NULL REFERENCES delete_account_requests(id) ON DELETE RESTRICT,
  revoked_at timestamptz NOT NULL
);

CREATE TRIGGER client_deletion_policy_immutable
 BEFORE UPDATE OR DELETE ON client_deletion_policy_approvals
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER client_deletion_local_receipts_immutable
 BEFORE UPDATE OR DELETE ON client_deletion_local_erasure_receipts
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER client_deletion_revocations_immutable
 BEFORE UPDATE OR DELETE ON client_deletion_revocations
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE OR REPLACE FUNCTION apgic_deletion_terminal_evidence_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.state IN ('COMPLETED','PARTIALLY_RETAINED_WITH_REASON')
     AND NEW.state IS DISTINCT FROM OLD.state THEN
    IF NOT EXISTS (
      SELECT 1 FROM client_deletion_policy_approvals a
      WHERE a.request_id=OLD.id
    ) OR (
      SELECT count(*) FROM client_deletion_local_erasure_receipts e
      WHERE e.request_id=OLD.id
    ) <> 3 OR NOT EXISTS (
      SELECT 1 FROM client_deletion_revocations r
      WHERE r.request_id=OLD.id AND r.identity_id=OLD.identity_id
    ) THEN
      RAISE EXCEPTION 'deletion terminal state requires approved retention, all local erasure receipts and session revocation';
    END IF;
    IF EXISTS (
      SELECT 1 FROM provider_erasure_jobs j
      WHERE j.delete_request_id=OLD.id AND j.state<>'SUCCEEDED'
    ) THEN
      RAISE EXCEPTION 'deletion terminal state requires verified provider erasure';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER delete_account_requests_terminal_evidence_required
 BEFORE UPDATE ON delete_account_requests
 FOR EACH ROW EXECUTE FUNCTION apgic_deletion_terminal_evidence_guard();
