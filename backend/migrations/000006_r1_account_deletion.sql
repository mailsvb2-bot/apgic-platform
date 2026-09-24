BEGIN;

CREATE TABLE delete_account_requests (
  id uuid PRIMARY KEY,
  identity_id uuid NOT NULL REFERENCES identities(id),
  source_surface text NOT NULL CHECK (source_surface IN ('WEB','IOS','ANDROID')),
  state text NOT NULL CHECK (
    state IN (
      'REQUESTED',
      'IDENTITY_RECONFIRMED',
      'RETENTION_CLASSIFIED',
      'PROVIDER_ERASURE_PENDING',
      'WAITING_FOR_LEGAL_HOLD_EXPIRY',
      'PARTIALLY_RETAINED_WITH_REASON',
      'COMPLETED'
    )
  ),
  retention_snapshot jsonb,
  retained_reason text,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  completed_at timestamptz,
  CHECK (
    state NOT IN (
      'RETENTION_CLASSIFIED',
      'PROVIDER_ERASURE_PENDING',
      'WAITING_FOR_LEGAL_HOLD_EXPIRY',
      'PARTIALLY_RETAINED_WITH_REASON',
      'COMPLETED'
    )
    OR retention_snapshot IS NOT NULL
  ),
  CHECK (
    state <> 'PARTIALLY_RETAINED_WITH_REASON'
    OR nullif(btrim(retained_reason), '') IS NOT NULL
  ),
  CHECK (
    state NOT IN ('PARTIALLY_RETAINED_WITH_REASON','COMPLETED')
    OR completed_at IS NOT NULL
  )
);

CREATE INDEX delete_account_requests_identity_idx
  ON delete_account_requests (identity_id, state, created_at);

CREATE TABLE provider_erasure_jobs (
  id uuid PRIMARY KEY,
  delete_request_id uuid NOT NULL REFERENCES delete_account_requests(id),
  provider_ref text NOT NULL CHECK (nullif(btrim(provider_ref), '') IS NOT NULL),
  state text NOT NULL CHECK (state IN ('PENDING','SUCCEEDED')),
  evidence_ref text,
  updated_at timestamptz NOT NULL,
  UNIQUE (delete_request_id, provider_ref),
  CHECK (state <> 'SUCCEEDED' OR nullif(btrim(evidence_ref), '') IS NOT NULL)
);

CREATE OR REPLACE FUNCTION apgic_delete_request_transition_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.identity_id <> OLD.identity_id OR NEW.source_surface <> OLD.source_surface THEN
    RAISE EXCEPTION 'delete account request identity/source are immutable';
  END IF;

  IF NEW.state <> OLD.state THEN
    IF OLD.state = 'REQUESTED' AND NEW.state = 'IDENTITY_RECONFIRMED' THEN
      NULL;
    ELSIF OLD.state = 'IDENTITY_RECONFIRMED' AND NEW.state = 'RETENTION_CLASSIFIED' THEN
      NULL;
    ELSIF OLD.state = 'RETENTION_CLASSIFIED'
      AND NEW.state = 'PROVIDER_ERASURE_PENDING' THEN
      NULL;
    ELSIF OLD.state = 'PROVIDER_ERASURE_PENDING'
      AND NEW.state IN ('WAITING_FOR_LEGAL_HOLD_EXPIRY','PARTIALLY_RETAINED_WITH_REASON','COMPLETED') THEN
      NULL;
    ELSIF OLD.state = 'WAITING_FOR_LEGAL_HOLD_EXPIRY'
      AND NEW.state IN ('PROVIDER_ERASURE_PENDING','PARTIALLY_RETAINED_WITH_REASON','COMPLETED') THEN
      NULL;
    ELSE
      RAISE EXCEPTION 'invalid delete account state transition: % -> %', OLD.state, NEW.state;
    END IF;
  END IF;

  IF NEW.state IN ('WAITING_FOR_LEGAL_HOLD_EXPIRY','PARTIALLY_RETAINED_WITH_REASON','COMPLETED')
    AND EXISTS (
      SELECT 1
      FROM provider_erasure_jobs
      WHERE delete_request_id = OLD.id
        AND state <> 'SUCCEEDED'
    ) THEN
    RAISE EXCEPTION 'delete account cannot reach terminal/retained state before provider erasure evidence';
  END IF;

  IF NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'delete account updated_at cannot move backwards';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER delete_account_requests_transition_guard
BEFORE UPDATE ON delete_account_requests
FOR EACH ROW EXECUTE FUNCTION apgic_delete_request_transition_guard();

CREATE OR REPLACE FUNCTION apgic_reject_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'canonical APGIC lifecycle evidence cannot be hard deleted';
END;
$$;

CREATE TRIGGER delete_account_requests_no_delete
BEFORE DELETE ON delete_account_requests
FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

CREATE OR REPLACE FUNCTION apgic_provider_erasure_transition_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.delete_request_id <> OLD.delete_request_id OR NEW.provider_ref <> OLD.provider_ref THEN
    RAISE EXCEPTION 'provider erasure job identity is immutable';
  END IF;
  IF OLD.state = 'SUCCEEDED' AND NEW.state <> OLD.state THEN
    RAISE EXCEPTION 'provider erasure evidence is terminal';
  END IF;
  IF OLD.state = 'PENDING' AND NEW.state NOT IN ('PENDING','SUCCEEDED') THEN
    RAISE EXCEPTION 'invalid provider erasure transition';
  END IF;
  IF OLD.state = 'SUCCEEDED' AND NEW.evidence_ref IS DISTINCT FROM OLD.evidence_ref THEN
    RAISE EXCEPTION 'provider erasure evidence cannot be rewritten';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER provider_erasure_jobs_transition_guard
BEFORE UPDATE ON provider_erasure_jobs
FOR EACH ROW EXECUTE FUNCTION apgic_provider_erasure_transition_guard();

CREATE TRIGGER provider_erasure_jobs_no_delete
BEFORE DELETE ON provider_erasure_jobs
FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

COMMIT;
