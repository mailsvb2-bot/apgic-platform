\set ON_ERROR_STOP on

INSERT INTO identities (id)
VALUES ('00000000-0000-0000-0000-000000000001');

INSERT INTO organizations (id, name, status)
VALUES ('00000000-0000-0000-0000-000000000010', 'Invariant Test Org', 'ACTIVE');

INSERT INTO audit_records (
  id, actor_id, action, scope, reason, policy_version
) VALUES (
  '00000000-0000-0000-0000-000000000101',
  'actor-1', 'override', 'finance', 'test', 'policy-v1'
);

DO $$
DECLARE
  blocked boolean := false;
BEGIN
  BEGIN
    UPDATE audit_records
    SET reason = 'rewritten'
    WHERE id = '00000000-0000-0000-0000-000000000101';
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'audit_records mutation was not blocked';
  END IF;
END
$$;

INSERT INTO ledger_entries (
  id, debit_account_ref, credit_account_ref, amount_minor, currency,
  provider_evidence_ref, correlation_id
) VALUES (
  '00000000-0000-0000-0000-000000000201',
  'receivable', 'provider-clearing', 10000, 'RUB',
  'provider-evidence-1', 'correlation-1'
);

DO $$
DECLARE
  blocked boolean := false;
BEGIN
  BEGIN
    DELETE FROM ledger_entries
    WHERE id = '00000000-0000-0000-0000-000000000201';
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'ledger_entries deletion was not blocked';
  END IF;
END
$$;

INSERT INTO legal_acceptances (
  id, identity_id, document_id, document_version, evidence_hash, accepted_at
) VALUES
(
  '00000000-0000-0000-0000-000000000301',
  '00000000-0000-0000-0000-000000000001',
  'terms', 'v1', 'sha256:v1', now()
),
(
  '00000000-0000-0000-0000-000000000302',
  '00000000-0000-0000-0000-000000000001',
  'terms', 'v2', 'sha256:v2', now()
);

DO $$
DECLARE
  blocked boolean := false;
  version_count integer;
BEGIN
  SELECT count(*) INTO version_count
  FROM legal_acceptances
  WHERE identity_id = '00000000-0000-0000-0000-000000000001'
    AND document_id = 'terms';

  IF version_count <> 2 THEN
    RAISE EXCEPTION 'historical legal acceptances were not preserved';
  END IF;

  BEGIN
    UPDATE legal_acceptances
    SET document_version = 'v3'
    WHERE id = '00000000-0000-0000-0000-000000000301';
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'legal acceptance mutation was not blocked';
  END IF;
END
$$;

INSERT INTO client_installations (
  id, identity_id, platform, push_endpoint, push_generation, state, updated_at
) VALUES (
  '00000000-0000-0000-0000-000000000401',
  '00000000-0000-0000-0000-000000000001',
  'IOS', 'push-token-active', 1, 'ACTIVE', now()
);

DO $$
DECLARE
  duplicate_blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO client_installations (
      id, identity_id, platform, push_endpoint, push_generation, state, updated_at
    ) VALUES (
      '00000000-0000-0000-0000-000000000402',
      '00000000-0000-0000-0000-000000000001',
      'IOS', 'push-token-active', 1, 'ACTIVE', now()
    );
  EXCEPTION WHEN unique_violation THEN
    duplicate_blocked := true;
  END;
  IF NOT duplicate_blocked THEN
    RAISE EXCEPTION 'active push endpoint uniqueness was not enforced';
  END IF;
END
$$;
