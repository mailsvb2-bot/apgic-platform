\set ON_ERROR_STOP on

INSERT INTO connector_instances (
  id, capability_class, provider_kind, status, config_ref
) VALUES
(
  '00000000-0000-0000-0000-00000000c001',
  'COMMUNICATION_PROVIDER',
  'CI_COMMUNICATION_A',
  'ACTIVE',
  'secret://ci/communication-a'
),
(
  '00000000-0000-0000-0000-00000000c002',
  'COMMUNICATION_PROVIDER',
  'CI_COMMUNICATION_B',
  'ACTIVE',
  'secret://ci/communication-b'
);

INSERT INTO booking_slots (
  id, specialist_identity_id, tenant_scope, starts_at, ends_at, exclusive
) VALUES (
  '00000000-0000-0000-0000-00000000c010',
  '00000000-0000-0000-0000-00000000b001',
  'tenant/r3-ci',
  now() + interval '1 hour',
  now() + interval '2 hours',
  true
);

SELECT *
FROM apgic_acquire_slot_hold(
  '00000000-0000-0000-0000-00000000c011',
  '00000000-0000-0000-0000-00000000c010',
  '00000000-0000-0000-0000-00000000b002',
  now() + interval '10 minutes',
  now()
);

SELECT *
FROM apgic_create_booking_from_hold(
  '00000000-0000-0000-0000-00000000c012',
  '00000000-0000-0000-0000-00000000c011',
  now() + interval '1 minute'
);

UPDATE bookings
SET state = 'CONFIRMED',
    updated_at = now() + interval '2 minutes'
WHERE id = '00000000-0000-0000-0000-00000000c012';

INSERT INTO communication_access_policy_versions (
  version, join_early_seconds, join_late_seconds,
  max_credential_ttl_seconds, created_at
) VALUES (
  'communication-access-r3-ci-v1',
  7200,
  1800,
  300,
  now()
);

INSERT INTO booking_access_entitlement_versions (
  id, booking_id, identity_id, role, state, basis_ref, policy_version,
  supersedes_id, effective_from, expires_at, created_at
) VALUES
(
  '00000000-0000-0000-0000-00000000c101',
  '00000000-0000-0000-0000-00000000c012',
  '00000000-0000-0000-0000-00000000b002',
  'CLIENT',
  'ACTIVE',
  'booking/00000000-0000-0000-0000-00000000c012',
  'fulfillment-r3-ci-v1',
  NULL,
  now(),
  now() + interval '4 hours',
  now()
),
(
  '00000000-0000-0000-0000-00000000c102',
  '00000000-0000-0000-0000-00000000c012',
  '00000000-0000-0000-0000-00000000b001',
  'SPECIALIST',
  'ACTIVE',
  'booking/00000000-0000-0000-0000-00000000c012',
  'fulfillment-r3-ci-v1',
  NULL,
  now(),
  now() + interval '4 hours',
  now()
);

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO communication_join_authorizations (
      id, booking_id, identity_id, role, provider_instance_id,
      policy_version, entitlement_version_id, decision, reason_code,
      credential_scope, provider_credential_ref, credential_expires_at,
      idempotency_key, decided_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000c199',
      '00000000-0000-0000-0000-00000000c012',
      '00000000-0000-0000-0000-00000000b003',
      'CLIENT',
      '00000000-0000-0000-0000-00000000c001',
      'communication-access-r3-ci-v1',
      '00000000-0000-0000-0000-00000000c101',
      'ALLOW',
      'COMM_JOIN_ALLOWED',
      'ROOM_JOIN',
      'credential-ref/invalid-role',
      now() + interval '5 minutes',
      'join-invalid-role',
      now()
    );
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'communication join allowed role/identity mismatch';
  END IF;
END
$$;

INSERT INTO communication_join_authorizations (
  id, booking_id, identity_id, role, provider_instance_id,
  policy_version, entitlement_version_id, decision, reason_code,
  credential_scope, provider_credential_ref, credential_expires_at,
  idempotency_key, decided_at
) VALUES (
  '00000000-0000-0000-0000-00000000c198',
  '00000000-0000-0000-0000-00000000c012',
  '00000000-0000-0000-0000-00000000b003',
  'CLIENT',
  '00000000-0000-0000-0000-00000000c001',
  'communication-access-r3-ci-v1',
  NULL,
  'DENY',
  'COMM_ROLE_MISMATCH',
  NULL,
  NULL,
  NULL,
  'join-deny-role-mismatch',
  now()
);

INSERT INTO communication_join_authorizations (
  id, booking_id, identity_id, role, provider_instance_id,
  policy_version, entitlement_version_id, decision, reason_code,
  credential_scope, provider_credential_ref, credential_expires_at,
  idempotency_key, decided_at
) VALUES
(
  '00000000-0000-0000-0000-00000000c201',
  '00000000-0000-0000-0000-00000000c012',
  '00000000-0000-0000-0000-00000000b002',
  'CLIENT',
  '00000000-0000-0000-0000-00000000c001',
  'communication-access-r3-ci-v1',
  '00000000-0000-0000-0000-00000000c101',
  'ALLOW',
  'COMM_JOIN_ALLOWED',
  'ROOM_JOIN',
  'credential-ref/client-join',
  now() + interval '5 minutes',
  'join-client-r3-ci',
  now()
),
(
  '00000000-0000-0000-0000-00000000c202',
  '00000000-0000-0000-0000-00000000c012',
  '00000000-0000-0000-0000-00000000b001',
  'SPECIALIST',
  '00000000-0000-0000-0000-00000000c001',
  'communication-access-r3-ci-v1',
  '00000000-0000-0000-0000-00000000c102',
  'ALLOW',
  'COMM_JOIN_ALLOWED',
  'ROOM_JOIN',
  'credential-ref/specialist-join',
  now() + interval '5 minutes',
  'join-specialist-r3-ci',
  now()
);

INSERT INTO consultation_sessions (
  id, booking_id, client_identity_id, specialist_identity_id,
  provider_instance_id, state, created_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-00000000c301',
  '00000000-0000-0000-0000-00000000c012',
  '00000000-0000-0000-0000-00000000b002',
  '00000000-0000-0000-0000-00000000b001',
  '00000000-0000-0000-0000-00000000c001',
  'SCHEDULED',
  now(),
  now()
);

INSERT INTO consultation_lifecycle_facts (
  id, session_id, fact_type, role, identity_id,
  provider_instance_id, provider_reference, evidence_ref, idempotency_key, occurred_at
) VALUES
(
  '00000000-0000-0000-0000-00000000c401',
  '00000000-0000-0000-0000-00000000c301',
  'JOINED',
  'CLIENT',
  '00000000-0000-0000-0000-00000000b002',
  '00000000-0000-0000-0000-00000000c001',
  'room/r3-ci-1',
  'provider-evidence/client-joined',
  'fact-client-joined',
  now() + interval '1 minute'
),
(
  '00000000-0000-0000-0000-00000000c402',
  '00000000-0000-0000-0000-00000000c301',
  'JOINED',
  'SPECIALIST',
  '00000000-0000-0000-0000-00000000b001',
  '00000000-0000-0000-0000-00000000c001',
  'room/r3-ci-1',
  'provider-evidence/specialist-joined',
  'fact-specialist-joined',
  now() + interval '2 minutes'
);

DO $$
BEGIN
  IF (
    SELECT state FROM consultation_sessions
    WHERE id = '00000000-0000-0000-0000-00000000c301'
  ) <> 'READY' THEN
    RAISE EXCEPTION 'consultation did not become READY from participant evidence';
  END IF;
END
$$;

INSERT INTO consultation_lifecycle_facts (
  id, session_id, fact_type, role, identity_id,
  provider_instance_id, provider_reference, evidence_ref, idempotency_key, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000c403',
  '00000000-0000-0000-0000-00000000c301',
  'STARTED',
  'SYSTEM',
  NULL,
  '00000000-0000-0000-0000-00000000c001',
  'room/r3-ci-1',
  'provider-evidence/session-started',
  'fact-session-started',
  now() + interval '3 minutes'
);

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    UPDATE bookings
    SET state = 'COMPLETED',
        updated_at = now() + interval '4 hours'
    WHERE id = '00000000-0000-0000-0000-00000000c012';
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'booking completed without consultation completion evidence';
  END IF;
END
$$;

INSERT INTO consultation_lifecycle_facts (
  id, session_id, fact_type, role, identity_id,
  provider_instance_id, provider_reference, evidence_ref, idempotency_key, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000c404',
  '00000000-0000-0000-0000-00000000c301',
  'RECOVERY_STARTED',
  'SYSTEM',
  NULL,
  '00000000-0000-0000-0000-00000000c001',
  'room/r3-ci-1',
  'provider-evidence/recovery-started',
  'fact-recovery-started',
  now() + interval '4 minutes'
);

INSERT INTO consultation_recovery_decisions (
  id, session_id, source_fact_id, policy_version, action,
  target_provider_instance_id, followup_path_ref, reason_code, decided_at
) VALUES (
  '00000000-0000-0000-0000-00000000c450',
  '00000000-0000-0000-0000-00000000c301',
  '00000000-0000-0000-0000-00000000c404',
  'communication-recovery-r3-ci-v1',
  'FALLBACK_PROVIDER',
  '00000000-0000-0000-0000-00000000c002',
  NULL,
  'COMM_RECOVERY_FALLBACK',
  now() + interval '4 minutes 30 seconds'
);


INSERT INTO consultation_lifecycle_facts (
  id, session_id, fact_type, role, identity_id,
  provider_instance_id, provider_reference, evidence_ref, idempotency_key, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000c405',
  '00000000-0000-0000-0000-00000000c301',
  'RECOVERY_SUCCEEDED',
  'SYSTEM',
  NULL,
  '00000000-0000-0000-0000-00000000c002',
  'room/r3-ci-2',
  'provider-evidence/recovery-succeeded',
  'fact-recovery-succeeded',
  now() + interval '5 minutes'
);

INSERT INTO consultation_lifecycle_facts (
  id, session_id, fact_type, role, identity_id,
  provider_instance_id, provider_reference, evidence_ref, idempotency_key, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000c406',
  '00000000-0000-0000-0000-00000000c301',
  'ENDED',
  'SYSTEM',
  NULL,
  '00000000-0000-0000-0000-00000000c002',
  'room/r3-ci-2',
  'provider-evidence/session-ended',
  'fact-session-ended',
  now() + interval '6 minutes'
);

UPDATE bookings
SET state = 'COMPLETED',
    updated_at = now() + interval '7 minutes'
WHERE id = '00000000-0000-0000-0000-00000000c012';

DO $$
DECLARE mutation_blocked boolean := false;
BEGIN
  IF (
    SELECT state FROM consultation_sessions
    WHERE id = '00000000-0000-0000-0000-00000000c301'
  ) <> 'COMPLETED' THEN
    RAISE EXCEPTION 'consultation completion evidence did not drive COMPLETED state';
  END IF;

  IF (
    SELECT state FROM bookings
    WHERE id = '00000000-0000-0000-0000-00000000c012'
  ) <> 'COMPLETED' THEN
    RAISE EXCEPTION 'booking did not complete from consultation evidence';
  END IF;

  BEGIN
    UPDATE consultation_lifecycle_facts
    SET evidence_ref = 'rewritten-evidence'
    WHERE id = '00000000-0000-0000-0000-00000000c406';
  EXCEPTION WHEN raise_exception THEN
    mutation_blocked := true;
  END;

  IF NOT mutation_blocked THEN
    RAISE EXCEPTION 'consultation lifecycle evidence was mutable';
  END IF;
END
$$;
