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


-- R0 semantic alignment invariants added by migration 000003.
INSERT INTO organization_directions (
  id, organization_id, name, status
) VALUES (
  '00000000-0000-0000-0000-000000000501',
  '00000000-0000-0000-0000-000000000010',
  'Archive-only direction',
  'ACTIVE'
);

DO $$
DECLARE
  blocked boolean := false;
BEGIN
  BEGIN
    DELETE FROM organization_directions
    WHERE id = '00000000-0000-0000-0000-000000000501';
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'organization direction hard delete was not blocked';
  END IF;
END
$$;

INSERT INTO products (
  id, owner_type, owner_id, commercial_owner_ref, author_refs,
  revenue_beneficiary_ref
) VALUES (
  '00000000-0000-0000-0000-000000000601',
  'ORGANIZATION',
  '00000000-0000-0000-0000-000000000010',
  'organization/00000000-0000-0000-0000-000000000010',
  ARRAY['identity/author-1'],
  'beneficiary/author-1'
);

DO $$
DECLARE
  blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO products (
      id, owner_type, owner_id, commercial_owner_ref, author_refs,
      revenue_beneficiary_ref
    ) VALUES (
      '00000000-0000-0000-0000-000000000602',
      'ORGANIZATION',
      '00000000-0000-0000-0000-000000009999',
      'organization/missing',
      ARRAY['identity/author-1'],
      'beneficiary/author-1'
    );
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'nonexistent polymorphic product owner was accepted';
  END IF;
END
$$;

INSERT INTO outbox_events (
  event_id, idempotency_key, event_type, schema_version, aggregate_ref,
  correlation_id, occurred_at, producer, payload
) VALUES (
  '00000000-0000-0000-0000-000000000701',
  'identity/person-1:role_added:1',
  'identity.role_added',
  '1',
  'identity/person-1',
  'correlation-outbox-1',
  now(),
  'identity',
  '{"role":"SPECIALIST"}'::jsonb
);

DO $$
DECLARE
  duplicate_blocked boolean := false;
  terminal_without_evidence_blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO outbox_events (
      event_id, idempotency_key, event_type, schema_version, aggregate_ref,
      correlation_id, occurred_at, producer, payload
    ) VALUES (
      '00000000-0000-0000-0000-000000000702',
      'identity/person-1:role_added:1',
      'identity.role_added',
      '1',
      'identity/person-1',
      'correlation-outbox-2',
      now(),
      'identity',
      '{"role":"SPECIALIST"}'::jsonb
    );
  EXCEPTION WHEN unique_violation THEN
    duplicate_blocked := true;
  END;

  BEGIN
    UPDATE outbox_events
    SET delivery_status = 'DELIVERED'
    WHERE event_id = '00000000-0000-0000-0000-000000000701';
  EXCEPTION WHEN check_violation THEN
    terminal_without_evidence_blocked := true;
  END;

  IF NOT duplicate_blocked THEN
    RAISE EXCEPTION 'outbox idempotency key uniqueness was not enforced';
  END IF;
  IF NOT terminal_without_evidence_blocked THEN
    RAISE EXCEPTION 'outbox terminal status without delivered_at was not blocked';
  END IF;
END
$$;


-- R0 database semantic hardening added by migration 000004.
DO $$
DECLARE
  invalid_archive_blocked boolean := false;
BEGIN
  BEGIN
    UPDATE organization_directions
    SET status = 'ARCHIVED'
    WHERE id = '00000000-0000-0000-0000-000000000501';
  EXCEPTION WHEN check_violation THEN
    invalid_archive_blocked := true;
  END;

  IF NOT invalid_archive_blocked THEN
    RAISE EXCEPTION 'ARCHIVED direction without archived_at was accepted';
  END IF;

  UPDATE organization_directions
  SET status = 'ARCHIVED',
      archived_at = now()
  WHERE id = '00000000-0000-0000-0000-000000000501';

  IF NOT EXISTS (
    SELECT 1
    FROM organization_directions
    WHERE id = '00000000-0000-0000-0000-000000000501'
      AND status = 'ARCHIVED'
      AND archived_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'valid archive transition was not persisted';
  END IF;
END
$$;

DO $$
DECLARE
  payload_rewrite_blocked boolean := false;
  attempts_decrease_blocked boolean := false;
  terminal_revert_blocked boolean := false;
  delivered_at_rewrite_blocked boolean := false;
BEGIN
  BEGIN
    UPDATE outbox_events
    SET payload = '{"role":"CLIENT"}'::jsonb
    WHERE event_id = '00000000-0000-0000-0000-000000000701';
  EXCEPTION WHEN raise_exception THEN
    payload_rewrite_blocked := true;
  END;

  UPDATE outbox_events
  SET attempts = 2
  WHERE event_id = '00000000-0000-0000-0000-000000000701';

  BEGIN
    UPDATE outbox_events
    SET attempts = 1
    WHERE event_id = '00000000-0000-0000-0000-000000000701';
  EXCEPTION WHEN raise_exception THEN
    attempts_decrease_blocked := true;
  END;

  UPDATE outbox_events
  SET delivery_status = 'DELIVERED',
      delivered_at = now(),
      attempts = 3
  WHERE event_id = '00000000-0000-0000-0000-000000000701';

  BEGIN
    UPDATE outbox_events
    SET delivery_status = 'PENDING',
        delivered_at = NULL
    WHERE event_id = '00000000-0000-0000-0000-000000000701';
  EXCEPTION WHEN raise_exception THEN
    terminal_revert_blocked := true;
  END;

  BEGIN
    UPDATE outbox_events
    SET delivered_at = delivered_at + interval '1 second'
    WHERE event_id = '00000000-0000-0000-0000-000000000701';
  EXCEPTION WHEN raise_exception THEN
    delivered_at_rewrite_blocked := true;
  END;

  IF NOT payload_rewrite_blocked THEN
    RAISE EXCEPTION 'outbox payload rewrite was not blocked';
  END IF;
  IF NOT attempts_decrease_blocked THEN
    RAISE EXCEPTION 'outbox attempts decrease was not blocked';
  END IF;
  IF NOT terminal_revert_blocked THEN
    RAISE EXCEPTION 'DELIVERED outbox record reverted to PENDING';
  END IF;
  IF NOT delivered_at_rewrite_blocked THEN
    RAISE EXCEPTION 'first delivered_at evidence was rewritten';
  END IF;
END
$$;
