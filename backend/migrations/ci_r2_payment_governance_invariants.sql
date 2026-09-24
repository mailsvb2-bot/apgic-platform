\set ON_ERROR_STOP on

INSERT INTO audit_records (
  id, actor_id, action, scope, resource_ref, new_state, reason,
  policy_version, correlation_id, occurred_at
) VALUES
(
  '00000000-0000-0000-0000-00000000f001',
  'admin-ci',
  'authorization.decision',
  'platform',
  'payment-provider/CI_PAYMENT_A',
  '{"decision":"ALLOW","reason_code":"AUTH_ALLOWED","action":"payment.provider.manage","risk":"HIGH_RISK"}'::jsonb,
  'AUTH_ALLOWED',
  'auth-ci-v1',
  'corr-payment-governance-ci',
  now()
),
(
  '00000000-0000-0000-0000-00000000f002',
  'admin-ci',
  'payment.provider.config_change',
  'platform',
  'payment-provider/CI_PAYMENT_A',
  '{"config_version":"cfg-a-v2","priority":5,"routing_weight_bps":7000}'::jsonb,
  'approved routing priority change',
  'payment-governance-ci-v1',
  'corr-payment-governance-ci',
  now()
),
(
  '00000000-0000-0000-0000-00000000f003',
  'admin-ci',
  'authorization.decision',
  'platform',
  'payment-provider/CI_PAYMENT_A',
  '{"decision":"ALLOW","reason_code":"AUTH_ALLOWED","action":"payment.provider.manage","risk":"NORMAL"}'::jsonb,
  'AUTH_ALLOWED',
  'auth-ci-v1',
  'corr-payment-governance-negative',
  now()
);

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO payment_provider_control_events (
      id, provider_instance_id, action, actor_id, reason,
      authorization_audit_id, result, evidence_refs, occurred_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000f099',
      '00000000-0000-0000-0000-00000000d001',
      'PRIORITIZE',
      'admin-ci',
      'invalid no-step-up event',
      '00000000-0000-0000-0000-00000000f003',
      'SUCCEEDED',
      ARRAY['ci/control/negative'],
      now()
    );
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'payment provider control event bypassed high-risk authorization';
  END IF;
END
$$;

INSERT INTO payment_provider_control_events (
  id, provider_instance_id, action, actor_id, reason,
  authorization_audit_id, result, evidence_refs, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000f101',
  '00000000-0000-0000-0000-00000000d001',
  'PRIORITIZE',
  'admin-ci',
  'approved routing priority change',
  '00000000-0000-0000-0000-00000000f001',
  'SUCCEEDED',
  ARRAY['ci/control/prioritize-a'],
  now()
);

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO payment_provider_config_versions (
      id, provider_instance_id, config_version, status, priority,
      manifest_version, certification_evidence_refs, jurisdiction_codes,
      currencies, method_codes, rail_codes, execution_owner,
      routing_weight_bps, credential_version_ref, supersedes_config_id,
      change_kind, actor_id, reason, effective_from
    ) VALUES (
      '00000000-0000-0000-0000-00000000f199',
      '00000000-0000-0000-0000-00000000d001',
      'cfg-a-invalid',
      'ACTIVE',
      5,
      'manifest-v1',
      ARRAY['evidence/sandbox-a'],
      ARRAY['RU'],
      ARRAY['RUB'],
      ARRAY['BANK_CARD','SBP'],
      ARRAY['APGIC_PAYMENT_PROVIDER'],
      'EXTERNAL_PROVIDER',
      7000,
      'secret-version/ci-payment-a/v1',
      '00000000-0000-0000-0000-00000000d101',
      'PRIORITIZE',
      'admin-ci',
      'missing audit links',
      now()+interval '1 second'
    );
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'payment provider revision bypassed audit evidence';
  END IF;
END
$$;

INSERT INTO payment_provider_config_versions (
  id, provider_instance_id, config_version, status, priority,
  manifest_version, certification_evidence_refs, jurisdiction_codes,
  currencies, method_codes, rail_codes, execution_owner,
  routing_weight_bps, credential_version_ref, supersedes_config_id,
  change_kind, actor_id, reason, authorization_audit_id, change_audit_id,
  effective_from
) VALUES (
  '00000000-0000-0000-0000-00000000f201',
  '00000000-0000-0000-0000-00000000d001',
  'cfg-a-v2',
  'ACTIVE',
  5,
  'manifest-v1',
  ARRAY['evidence/sandbox-a'],
  ARRAY['RU'],
  ARRAY['RUB'],
  ARRAY['BANK_CARD','SBP'],
  ARRAY['APGIC_PAYMENT_PROVIDER'],
  'EXTERNAL_PROVIDER',
  7000,
  'secret-version/ci-payment-a/v1',
  '00000000-0000-0000-0000-00000000d101',
  'PRIORITIZE',
  'admin-ci',
  'approved routing priority change',
  '00000000-0000-0000-0000-00000000f001',
  '00000000-0000-0000-0000-00000000f002',
  now()+interval '1 second'
);

INSERT INTO payment_provider_health_snapshots (
  id, provider_config_id, health, conversion_rate_bps, latency_p95_ms,
  provider_reported_fee_bps, reconciliation_pending_count,
  reconciliation_mismatch_count, guardrail_action, evidence_refs, observed_at
) VALUES (
  '00000000-0000-0000-0000-00000000f301',
  '00000000-0000-0000-0000-00000000f201',
  'DEGRADED',
  7400,
  2500,
  240,
  5,
  1,
  'BLOCK_NEW_ATTEMPTS',
  ARRAY['metrics/provider-a/window-block','reconciliation/provider-a/run-block'],
  now()+interval '2 seconds'
);

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO payment_routing_decisions (
      id, order_id, policy_version, provider_config_id, jurisdiction_code,
      selected_method_code, selected_rail_code, candidate_evidence,
      health_snapshot, health_snapshot_id, decided_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000f399',
      '00000000-0000-0000-0000-00000000b501',
      'routing-r2-governance-ci-v1',
      '00000000-0000-0000-0000-00000000f201',
      'RU',
      'SBP',
      'APGIC_PAYMENT_PROVIDER',
      '[{"provider":"CI_PAYMENT_A","eligible":false,"reason":"HEALTH_GUARDRAIL"}]'::jsonb,
      '{"status":"DEGRADED","guardrail":"BLOCK_NEW_ATTEMPTS"}'::jsonb,
      '00000000-0000-0000-0000-00000000f301',
      now()+interval '3 seconds'
    );
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'routing ignored payment provider health guardrail';
  END IF;
END
$$;

INSERT INTO payment_provider_health_snapshots (
  id, provider_config_id, health, conversion_rate_bps, latency_p95_ms,
  provider_reported_fee_bps, reconciliation_pending_count,
  reconciliation_mismatch_count, guardrail_action, evidence_refs, observed_at
) VALUES (
  '00000000-0000-0000-0000-00000000f302',
  '00000000-0000-0000-0000-00000000f201',
  'HEALTHY',
  9100,
  220,
  230,
  0,
  0,
  'ALLOW_NEW_ATTEMPTS',
  ARRAY['metrics/provider-a/window-allow','reconciliation/provider-a/run-clean'],
  now()+interval '4 seconds'
);

INSERT INTO payment_routing_decisions (
  id, order_id, policy_version, provider_config_id, jurisdiction_code,
  selected_method_code, selected_rail_code, candidate_evidence,
  health_snapshot, health_snapshot_id, decided_at
) VALUES (
  '00000000-0000-0000-0000-00000000f401',
  '00000000-0000-0000-0000-00000000b501',
  'routing-r2-governance-ci-v1',
  '00000000-0000-0000-0000-00000000f201',
  'RU',
  'SBP',
  'APGIC_PAYMENT_PROVIDER',
  '[{"provider":"CI_PAYMENT_A","eligible":true}]'::jsonb,
  '{"status":"HEALTHY","guardrail":"ALLOW_NEW_ATTEMPTS"}'::jsonb,
  '00000000-0000-0000-0000-00000000f302',
  now()+interval '5 seconds'
);

INSERT INTO audit_records (
  id, actor_id, action, scope, resource_ref, new_state, reason,
  policy_version, correlation_id, occurred_at
) VALUES
(
  '00000000-0000-0000-0000-00000000f004',
  'admin-ci',
  'authorization.decision',
  'platform',
  'payment-provider/CI_PAYMENT_A',
  '{"decision":"ALLOW","reason_code":"AUTH_ALLOWED","action":"payment.provider.manage","risk":"HIGH_RISK"}'::jsonb,
  'AUTH_ALLOWED',
  'auth-ci-v1',
  'corr-payment-governance-rollback',
  now()
),
(
  '00000000-0000-0000-0000-00000000f005',
  'admin-ci',
  'payment.provider.config_change',
  'platform',
  'payment-provider/CI_PAYMENT_A',
  '{"config_version":"cfg-a-v3","rollback_to":"cfg-a-v1"}'::jsonb,
  'rollback approved',
  'payment-governance-ci-v1',
  'corr-payment-governance-rollback',
  now()
);

INSERT INTO payment_provider_config_versions (
  id, provider_instance_id, config_version, status, priority,
  manifest_version, certification_evidence_refs, jurisdiction_codes,
  currencies, method_codes, rail_codes, execution_owner,
  routing_weight_bps, credential_version_ref, supersedes_config_id,
  change_kind, actor_id, reason, authorization_audit_id, change_audit_id,
  effective_from
) VALUES (
  '00000000-0000-0000-0000-00000000f202',
  '00000000-0000-0000-0000-00000000d001',
  'cfg-a-v3',
  'ACTIVE',
  10,
  'manifest-v1',
  ARRAY['evidence/sandbox-a'],
  ARRAY['RU'],
  ARRAY['RUB'],
  ARRAY['BANK_CARD','SBP'],
  ARRAY['APGIC_PAYMENT_PROVIDER'],
  'EXTERNAL_PROVIDER',
  10000,
  'secret-version/ci-payment-a/v1',
  '00000000-0000-0000-0000-00000000f201',
  'ROLLBACK',
  'admin-ci',
  'rollback approved',
  '00000000-0000-0000-0000-00000000f004',
  '00000000-0000-0000-0000-00000000f005',
  now()+interval '6 seconds'
);

DO $$
DECLARE mutable_history boolean := false;
BEGIN
  BEGIN
    UPDATE payment_provider_config_versions
    SET priority = 1
    WHERE id = '00000000-0000-0000-0000-00000000f201';
  EXCEPTION WHEN raise_exception THEN
    mutable_history := true;
  END;

  IF NOT mutable_history THEN
    RAISE EXCEPTION 'payment provider configuration history was rewritten';
  END IF;

  IF (
    SELECT priority
    FROM payment_provider_config_versions
    WHERE id = '00000000-0000-0000-0000-00000000d101'
  ) <> 10 THEN
    RAISE EXCEPTION 'rollback rewrote original payment provider config';
  END IF;

  IF (
    SELECT count(*)
    FROM payment_provider_config_versions
    WHERE provider_instance_id = '00000000-0000-0000-0000-00000000d001'
  ) <> 3 THEN
    RAISE EXCEPTION 'versioned payment provider history was not preserved';
  END IF;
END
$$;
