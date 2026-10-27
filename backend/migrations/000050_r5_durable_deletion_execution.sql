-- R5 canonical deletion execution, durable proof and revocation.
-- Never infer erasure from a requested state or a fabricated reference.
CREATE TABLE deletion_local_erasure_receipts (
  request_id uuid PRIMARY KEY REFERENCES delete_account_requests(id),
  policy_version text NOT NULL CHECK (nullif(btrim(policy_version),'') IS NOT NULL),
  approval_sha256 text NOT NULL CHECK (approval_sha256 ~ '^[0-9a-f]{64}$'),
  preferences_erased bigint NOT NULL CHECK (preferences_erased >= 0),
  help_intents_anonymized bigint NOT NULL CHECK (help_intents_anonymized >= 0),
  credentials_erased bigint NOT NULL CHECK (credentials_erased >= 0),
  no_external_data boolean NOT NULL DEFAULT false,
  verified_at timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER deletion_local_erasure_receipts_immutable
BEFORE UPDATE OR DELETE ON deletion_local_erasure_receipts
FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

CREATE TABLE deletion_provider_verified_receipts (
  request_id uuid NOT NULL,
  provider_ref text NOT NULL,
  evidence_ref text NOT NULL CHECK (length(evidence_ref) > 15),
  signed_payload_sha256 text NOT NULL CHECK (signed_payload_sha256 ~ '^[0-9a-f]{64}$'),
  signature_base64 text NOT NULL CHECK (length(signature_base64) > 40),
  verified_at timestamptz NOT NULL,
  PRIMARY KEY (request_id, provider_ref),
  FOREIGN KEY (request_id, provider_ref)
    REFERENCES provider_erasure_jobs(delete_request_id, provider_ref)
);

CREATE TRIGGER deletion_provider_verified_receipts_immutable
BEFORE UPDATE OR DELETE ON deletion_provider_verified_receipts
FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

CREATE TABLE deletion_provider_dispatch (
  request_id uuid NOT NULL,
  provider_ref text NOT NULL,
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  lease_until timestamptz,
  last_error_code text,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (request_id,provider_ref),
  FOREIGN KEY (request_id,provider_ref)
    REFERENCES provider_erasure_jobs(delete_request_id,provider_ref)
);
CREATE INDEX deletion_provider_dispatch_ready
ON deletion_provider_dispatch(next_attempt_at) WHERE lease_until IS NULL;

CREATE TABLE deletion_revoked_identities (
  identity_id uuid PRIMARY KEY REFERENCES identities(id),
  request_id uuid NOT NULL UNIQUE REFERENCES delete_account_requests(id),
  revoked_at timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION apgic_delete_execution_evidence_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  local_receipt deletion_local_erasure_receipts%ROWTYPE;
BEGIN
  IF OLD.state = NEW.state THEN
    RETURN NEW;
  END IF;
  IF NEW.state NOT IN
    ('PROVIDER_ERASURE_PENDING','WAITING_FOR_LEGAL_HOLD_EXPIRY',
     'PARTIALLY_RETAINED_WITH_REASON','COMPLETED') THEN
    RETURN NEW;
  END IF;
  SELECT * INTO local_receipt FROM deletion_local_erasure_receipts
  WHERE request_id = NEW.id;
  IF NOT FOUND OR local_receipt.approval_sha256 IS NULL THEN
    RAISE EXCEPTION 'deletion cannot advance without approved local erasure receipt';
  END IF;
  IF NEW.state IN ('WAITING_FOR_LEGAL_HOLD_EXPIRY','PARTIALLY_RETAINED_WITH_REASON','COMPLETED') THEN
    IF EXISTS (
      SELECT 1 FROM provider_erasure_jobs p
      LEFT JOIN deletion_provider_verified_receipts v
      ON v.request_id = p.delete_request_id AND v.provider_ref = p.provider_ref
      WHERE p.delete_request_id = NEW.id
      AND (p.state <> 'SUCCEEDED' OR v.request_id IS NULL)
    ) THEN
      RAISE EXCEPTION 'deletion cannot finalize without verified provider receipts';
    END IF;
    IF NOT local_receipt.no_external_data AND NOT EXISTS (
      SELECT 1 FROM provider_erasure_jobs WHERE delete_request_id = NEW.id
    ) THEN
      RAISE EXCEPTION 'deletion cannot silently assume absence of external data';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER delete_execution_evidence_guard
BEFORE UPDATE ON delete_account_requests
FOR EACH ROW EXECUTE FUNCTION apgic_delete_execution_evidence_guard();
