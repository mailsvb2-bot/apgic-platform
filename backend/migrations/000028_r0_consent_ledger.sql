BEGIN;

CREATE TABLE consent_records (
  consent_id uuid PRIMARY KEY,
  subject_id uuid NOT NULL REFERENCES identities(id),
  purpose text NOT NULL,
  scope text NOT NULL,
  policy_version text NOT NULL,
  text_hash_or_version text NOT NULL,
  granted_at timestamptz NOT NULL,
  revoked_at timestamptz,
  source text NOT NULL,
  proof_metadata jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT consent_records_required_text CHECK (
    btrim(purpose) <> '' AND
    btrim(scope) <> '' AND
    btrim(policy_version) <> '' AND
    btrim(text_hash_or_version) <> '' AND
    btrim(source) <> ''
  ),
  CONSTRAINT consent_records_proof_object CHECK (jsonb_typeof(proof_metadata) = 'object'),
  CONSTRAINT consent_records_revocation_time CHECK (revoked_at IS NULL OR revoked_at >= granted_at)
);

CREATE UNIQUE INDEX consent_records_active_unique
  ON consent_records (subject_id, purpose, scope)
  WHERE revoked_at IS NULL;

CREATE INDEX consent_records_subject_history_idx
  ON consent_records (subject_id, purpose, scope, granted_at DESC);

CREATE OR REPLACE FUNCTION apgic_enforce_consent_record_transition()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF (
    NEW.consent_id,
    NEW.subject_id,
    NEW.purpose,
    NEW.scope,
    NEW.policy_version,
    NEW.text_hash_or_version,
    NEW.granted_at,
    NEW.source,
    NEW.proof_metadata,
    NEW.created_at
  ) IS DISTINCT FROM (
    OLD.consent_id,
    OLD.subject_id,
    OLD.purpose,
    OLD.scope,
    OLD.policy_version,
    OLD.text_hash_or_version,
    OLD.granted_at,
    OLD.source,
    OLD.proof_metadata,
    OLD.created_at
  ) THEN
    RAISE EXCEPTION 'consent evidence is immutable after grant';
  END IF;

  IF OLD.revoked_at IS NOT NULL AND NEW.revoked_at IS DISTINCT FROM OLD.revoked_at THEN
    RAISE EXCEPTION 'revoked consent is terminal';
  END IF;

  IF OLD.revoked_at IS NULL AND NEW.revoked_at IS NULL THEN
    RAISE EXCEPTION 'consent update must be an explicit revocation';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER consent_records_transition_guard
BEFORE UPDATE ON consent_records
FOR EACH ROW EXECUTE FUNCTION apgic_enforce_consent_record_transition();

CREATE OR REPLACE FUNCTION apgic_forbid_consent_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'consent history cannot be deleted';
END;
$$;

CREATE TRIGGER consent_records_delete_guard
BEFORE DELETE ON consent_records
FOR EACH ROW EXECUTE FUNCTION apgic_forbid_consent_delete();

COMMIT;
