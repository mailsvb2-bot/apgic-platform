BEGIN;

CREATE TABLE legal_acceptances (
  id uuid PRIMARY KEY,
  identity_id uuid NOT NULL REFERENCES identities(id),
  document_id text NOT NULL CHECK (document_id <> ''),
  document_version text NOT NULL CHECK (document_version <> ''),
  evidence_hash text NOT NULL CHECK (evidence_hash <> ''),
  accepted_at timestamptz NOT NULL,
  UNIQUE (identity_id, document_id, document_version, evidence_hash)
);

CREATE TABLE client_installations (
  id uuid PRIMARY KEY,
  identity_id uuid NOT NULL REFERENCES identities(id),
  platform text NOT NULL CHECK (platform IN ('IOS', 'ANDROID')),
  push_endpoint text,
  push_generation bigint NOT NULL DEFAULT 1 CHECK (push_generation > 0),
  state text NOT NULL CHECK (state IN ('ACTIVE', 'REVOKED')),
  updated_at timestamptz NOT NULL
);

CREATE INDEX client_installations_identity_idx
  ON client_installations (identity_id, state);

CREATE UNIQUE INDEX client_installations_active_push_endpoint_idx
  ON client_installations (push_endpoint)
  WHERE state = 'ACTIVE' AND push_endpoint IS NOT NULL;

CREATE TRIGGER legal_acceptances_append_only
BEFORE UPDATE OR DELETE ON legal_acceptances
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

COMMIT;
