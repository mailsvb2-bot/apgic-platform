-- Canonical R5 durable proof of actual local erasure, signed legal policy,
-- external provider receipts, and account access revocation.
CREATE TABLE client_deletion_policy_approvals (
  request_id uuid PRIMARY KEY REFERENCES delete_account_requests(id) ON DELETE RESTRICT,
  policy_digest text NOT NULL CHECK (policy_digest ~ '^[0-9a-f]{64}$'),
  policy_snapshot jsonb NOT NULL CHECK (jsonb_typeof(policy_snapshot)='object'),
  approval_signature bytea NOT NULL CHECK (length(approval_signature)=64),
  signing_key_fingerprint text NOT NULL CHECK (signing_key_fingerprint ~ '^[0-9a-f]{64}$'),
  verified_at timestamptz NOT NULL,
  approved_by text NOT NULL CHECK (btrim(approved_by)<>'')
);

CREATE TABLE client_deletion_local_erasure_receipts (
  request_id uuid NOT NULL REFERENCES delete_account_requests(id) ON DELETE RESTRICT,
  data_class text NOT NULL CHECK (data_class IN (
    'PERSONAL_PREFERENCES','HELP_INTENT_PERSONAL_DATA','PUSH_ENDPOINTS'
  )),
  affected_rows bigint NOT NULL CHECK (affected_rows>=0),
  policy_digest text NOT NULL CHECK (policy_digest ~ '^[0-9a-f]{64}$'),
  executed_at timestamptz NOT NULL,
  PRIMARY KEY(request_id,data_class)
);

CREATE TABLE client_deletion_provider_receipts (
  request_id uuid NOT NULL REFERENCES delete_account_requests(id) ON DELETE RESTRICT,
  provider_ref text NOT NULL CHECK (btrim(provider_ref)<>''),
  receipt_json jsonb NOT NULL CHECK (jsonb_typeof(receipt_json)='object'),
  receipt_signature bytea NOT NULL CHECK (length(receipt_signature)=64),
  signing_key_fingerprint text NOT NULL CHECK (signing_key_fingerprint ~ '^[0-9a-f]{64}$'),
  verified_at timestamptz NOT NULL,
  PRIMARY KEY(request_id,provider_ref)
);

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
CREATE TRIGGER client_deletion_provider_receipts_immutable
 BEFORE UPDATE OR DELETE ON client_deletion_provider_receipts
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER client_deletion_revocations_immutable
 BEFORE UPDATE OR DELETE ON client_deletion_revocations
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

-- No caller may mark an account deleted merely by updating its state.
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
    ) <> 3 OR EXISTS (
      SELECT 1 FROM client_deletion_local_erasure_receipts e
      JOIN client_deletion_policy_approvals a ON a.request_id=e.request_id
      WHERE e.request_id=OLD.id AND e.policy_digest<>a.policy_digest
    ) OR NOT EXISTS (
      SELECT 1 FROM client_deletion_revocations r
      WHERE r.request_id=OLD.id AND r.identity_id=OLD.identity_id
    ) THEN
      RAISE EXCEPTION 'terminal deletion requires signed policy, local erasure proof and session revocation';
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

-- Serialize all owner writes with erasure so stale requests cannot recreate
-- personal data after deletion commits. The erasure worker locks owner first.
CREATE OR REPLACE FUNCTION apgic_block_revoked_client_writes()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE subject uuid;
BEGIN
  subject := NULLIF(to_jsonb(NEW)->>TG_ARGV[0],'')::uuid;
  IF subject IS NULL THEN
    RETURN NEW;
  END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(subject::text,0));
  IF EXISTS (SELECT 1 FROM client_deletion_revocations WHERE identity_id=subject) THEN
    RAISE EXCEPTION 'identity access revoked following verified deletion';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER client_personal_preferences_revoke_writes
 BEFORE INSERT OR UPDATE ON client_personal_preferences
 FOR EACH ROW EXECUTE FUNCTION apgic_block_revoked_client_writes('identity_id');
CREATE TRIGGER help_intents_revoke_writes
 BEFORE INSERT OR UPDATE ON help_intents
 FOR EACH ROW EXECUTE FUNCTION apgic_block_revoked_client_writes('identity_id');
CREATE TRIGGER client_installations_revoke_writes
 BEFORE INSERT OR UPDATE ON client_installations
 FOR EACH ROW EXECUTE FUNCTION apgic_block_revoked_client_writes('identity_id');
CREATE TRIGGER bookings_revoke_client_writes
 BEFORE INSERT OR UPDATE ON bookings
 FOR EACH ROW EXECUTE FUNCTION apgic_block_revoked_client_writes('client_identity_id');
CREATE TRIGGER booking_holds_revoke_client_writes
 BEFORE INSERT OR UPDATE ON booking_holds
 FOR EACH ROW EXECUTE FUNCTION apgic_block_revoked_client_writes('client_identity_id');
CREATE TRIGGER client_checkins_revoke_client_writes
 BEFORE INSERT OR UPDATE ON client_personal_check_ins
 FOR EACH ROW EXECUTE FUNCTION apgic_block_revoked_client_writes('client_identity_id');
