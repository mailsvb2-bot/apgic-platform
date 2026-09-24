\set ON_ERROR_STOP on

INSERT INTO delete_account_requests (
  id, identity_id, source_surface, state, created_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-000000000901',
  '00000000-0000-0000-0000-000000000001',
  'IOS',
  'REQUESTED',
  now(),
  now()
);

DO $$
DECLARE
  invalid_jump_blocked boolean := false;
  identity_rewrite_blocked boolean := false;
BEGIN
  BEGIN
    UPDATE delete_account_requests
    SET state = 'COMPLETED',
        retention_snapshot = '{"PROFILE":"ERASE"}'::jsonb,
        completed_at = now(),
        updated_at = now()
    WHERE id = '00000000-0000-0000-0000-000000000901';
  EXCEPTION WHEN raise_exception THEN
    invalid_jump_blocked := true;
  END;

  BEGIN
    UPDATE delete_account_requests
    SET identity_id = '00000000-0000-0000-0000-000000000010',
        updated_at = now()
    WHERE id = '00000000-0000-0000-0000-000000000901';
  EXCEPTION WHEN foreign_key_violation OR raise_exception THEN
    identity_rewrite_blocked := true;
  END;

  IF NOT invalid_jump_blocked THEN
    RAISE EXCEPTION 'delete account invalid state jump was accepted';
  END IF;
  IF NOT identity_rewrite_blocked THEN
    RAISE EXCEPTION 'delete account identity rewrite was accepted';
  END IF;
END
$$;

UPDATE delete_account_requests
SET state = 'IDENTITY_RECONFIRMED',
    updated_at = now()
WHERE id = '00000000-0000-0000-0000-000000000901';

UPDATE delete_account_requests
SET state = 'RETENTION_CLASSIFIED',
    retention_snapshot = '{"PROFILE":"ERASE","FINANCIAL_EVIDENCE":"RETAIN_WITH_REASON"}'::jsonb,
    updated_at = now()
WHERE id = '00000000-0000-0000-0000-000000000901';

UPDATE delete_account_requests
SET state = 'PROVIDER_ERASURE_PENDING',
    updated_at = now()
WHERE id = '00000000-0000-0000-0000-000000000901';

INSERT INTO provider_erasure_jobs (
  id, delete_request_id, provider_ref, state, updated_at
) VALUES (
  '00000000-0000-0000-0000-000000000902',
  '00000000-0000-0000-0000-000000000901',
  'object-storage',
  'PENDING',
  now()
);

UPDATE provider_erasure_jobs
SET state = 'SUCCEEDED',
    evidence_ref = 'provider-evidence:erased',
    updated_at = now()
WHERE id = '00000000-0000-0000-0000-000000000902';

UPDATE delete_account_requests
SET state = 'PARTIALLY_RETAINED_WITH_REASON',
    retained_reason = 'jurisdiction retention policy',
    completed_at = now(),
    updated_at = now()
WHERE id = '00000000-0000-0000-0000-000000000901';

DO $$
DECLARE
  request_delete_blocked boolean := false;
  evidence_rewrite_blocked boolean := false;
BEGIN
  BEGIN
    DELETE FROM delete_account_requests
    WHERE id = '00000000-0000-0000-0000-000000000901';
  EXCEPTION WHEN raise_exception THEN
    request_delete_blocked := true;
  END;

  BEGIN
    UPDATE provider_erasure_jobs
    SET evidence_ref = 'provider-evidence:rewritten',
        updated_at = now()
    WHERE id = '00000000-0000-0000-0000-000000000902';
  EXCEPTION WHEN raise_exception THEN
    evidence_rewrite_blocked := true;
  END;

  IF NOT request_delete_blocked THEN
    RAISE EXCEPTION 'delete account canonical request hard delete was accepted';
  END IF;
  IF NOT evidence_rewrite_blocked THEN
    RAISE EXCEPTION 'provider erasure evidence rewrite was accepted';
  END IF;
END
$$;
