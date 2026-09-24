\set ON_ERROR_STOP on

INSERT INTO connector_instances (id, capability_class, provider_kind, status, config_ref) VALUES
('00000000-0000-0000-0000-00000000d001','PAYMENT_PROVIDER','CI_PAYMENT_A','ACTIVE','secret://ci/payment-a'),
('00000000-0000-0000-0000-00000000d002','PAYMENT_PROVIDER','CI_PAYMENT_B','ACTIVE','secret://ci/payment-b');

INSERT INTO payment_provider_config_versions (
  id, provider_instance_id, config_version, status, priority, manifest_version,
  certification_evidence_refs, jurisdiction_codes, currencies, method_codes,
  rail_codes, execution_owner, credential_version_ref, effective_from
) VALUES
('00000000-0000-0000-0000-00000000d101','00000000-0000-0000-0000-00000000d001','cfg-a-v1','ACTIVE',10,'manifest-v1',
 ARRAY['evidence/sandbox-a'],ARRAY['RU'],ARRAY['RUB'],ARRAY['BANK_CARD','SBP'],ARRAY['APGIC_PAYMENT_PROVIDER'],'EXTERNAL_PROVIDER','secret-version/ci-payment-a/v1',now()),
('00000000-0000-0000-0000-00000000d102','00000000-0000-0000-0000-00000000d002','cfg-b-v1','ACTIVE',20,'manifest-v1',
 ARRAY['evidence/sandbox-b'],ARRAY['RU'],ARRAY['RUB'],ARRAY['BANK_CARD','SBP'],ARRAY['APGIC_PAYMENT_PROVIDER'],'EXTERNAL_PROVIDER','secret-version/ci-payment-b/v1',now());

INSERT INTO payment_provider_health_snapshots (
  id, provider_config_id, health, conversion_rate_bps, latency_p95_ms,
  provider_reported_fee_bps, reconciliation_pending_count,
  reconciliation_mismatch_count, guardrail_action, evidence_refs, observed_at
) VALUES
('00000000-0000-0000-0000-00000000d111','00000000-0000-0000-0000-00000000d101','HEALTHY',9000,200,150,0,0,'ALLOW_NEW_ATTEMPTS',ARRAY['metrics/ci-a'],'2026-09-24T12:00:00Z'),
('00000000-0000-0000-0000-00000000d112','00000000-0000-0000-0000-00000000d102','HEALTHY',9100,190,140,0,0,'ALLOW_NEW_ATTEMPTS',ARRAY['metrics/ci-b'],'2026-09-24T12:00:00Z');

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    UPDATE payment_provider_config_versions
    SET priority = 99
    WHERE id = '00000000-0000-0000-0000-00000000d101';
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'payment provider config history was mutable';
  END IF;
END
$$;

INSERT INTO payment_routing_decisions (
  id, order_id, policy_version, provider_config_id, jurisdiction_code,
  selected_method_code, selected_rail_code, candidate_evidence, health_snapshot, health_snapshot_id, decided_at
) VALUES (
  '00000000-0000-0000-0000-00000000d201',
  '00000000-0000-0000-0000-00000000b501',
  'routing-r2-ci-v1',
  '00000000-0000-0000-0000-00000000d101',
  'RU','SBP','APGIC_PAYMENT_PROVIDER',
  '[{"provider":"CI_PAYMENT_A","eligible":true},{"provider":"CI_PAYMENT_B","eligible":true}]'::jsonb,
  '{"CI_PAYMENT_A":"HEALTHY","CI_PAYMENT_B":"HEALTHY"}'::jsonb,
  '00000000-0000-0000-0000-00000000d111',
  now()
);

INSERT INTO payment_attempts (
  id, order_id, routing_decision_id, idempotency_key,
  amount_minor, currency, state, created_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-00000000d301',
  '00000000-0000-0000-0000-00000000b501',
  '00000000-0000-0000-0000-00000000d201',
  'pay-attempt-a',10000,'RUB','CREATED',now(),now()
);

UPDATE payment_attempts
SET state='SENT', provider_reference='ci-a/attempt-1', updated_at=now()+interval '1 second'
WHERE id='00000000-0000-0000-0000-00000000d301';

UPDATE payment_attempts
SET state='AMBIGUOUS', updated_at=now()+interval '2 seconds'
WHERE id='00000000-0000-0000-0000-00000000d301';

INSERT INTO payment_routing_decisions (
  id, order_id, policy_version, provider_config_id, jurisdiction_code,
  selected_method_code, selected_rail_code, candidate_evidence, health_snapshot, decided_at
) VALUES (
  '00000000-0000-0000-0000-00000000d202',
  '00000000-0000-0000-0000-00000000b501',
  'routing-r2-ci-v1',
  '00000000-0000-0000-0000-00000000d102',
  'RU','SBP','APGIC_PAYMENT_PROVIDER',
  '[{"provider":"CI_PAYMENT_A","eligible":false,"reason":"AMBIGUOUS"},{"provider":"CI_PAYMENT_B","eligible":true}]'::jsonb,
  '{"CI_PAYMENT_A":"DEGRADED","CI_PAYMENT_B":"HEALTHY"}'::jsonb,
  '00000000-0000-0000-0000-00000000d112',
  now()+interval '3 seconds'
);

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO payment_attempts (
      id, order_id, routing_decision_id, idempotency_key,
      amount_minor, currency, state, created_at, updated_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000d302',
      '00000000-0000-0000-0000-00000000b501',
      '00000000-0000-0000-0000-00000000d202',
      'pay-attempt-b',10000,'RUB','CREATED',now()+interval '3 seconds',now()+interval '3 seconds'
    );
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'ambiguous outcome allowed blind cross-provider retry';
  END IF;
END
$$;

UPDATE payment_attempts
SET state='FAILED_TERMINAL',
    provider_evidence_ref='provider-evidence/ci-a-terminal-failure',
    updated_at=now()+interval '4 seconds'
WHERE id='00000000-0000-0000-0000-00000000d301';

INSERT INTO payment_attempts (
  id, order_id, routing_decision_id, idempotency_key,
  amount_minor, currency, state, created_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-00000000d302',
  '00000000-0000-0000-0000-00000000b501',
  '00000000-0000-0000-0000-00000000d202',
  'pay-attempt-b',10000,'RUB','CREATED',now()+interval '5 seconds',now()+interval '5 seconds'
);

UPDATE payment_attempts
SET state='SENT', provider_reference='ci-b/attempt-2', updated_at=now()+interval '6 seconds'
WHERE id='00000000-0000-0000-0000-00000000d302';

UPDATE payment_attempts
SET state='SUCCEEDED',
    provider_evidence_ref='provider-evidence/ci-b-captured',
    updated_at=now()+interval '7 seconds'
WHERE id='00000000-0000-0000-0000-00000000d302';

DO $$
DECLARE accepted_1 boolean; reason_1 text; accepted_2 boolean; reason_2 text;
BEGIN
  SELECT accepted, reason_code INTO accepted_1, reason_1
  FROM apgic_record_payment_webhook(
    '00000000-0000-0000-0000-00000000d401',
    '00000000-0000-0000-0000-00000000d002',
    'provider-event-captured-1','sha256:ci-payload-1',now()+interval '8 seconds'
  );

  SELECT accepted, reason_code INTO accepted_2, reason_2
  FROM apgic_record_payment_webhook(
    '00000000-0000-0000-0000-00000000d402',
    '00000000-0000-0000-0000-00000000d002',
    'provider-event-captured-1','sha256:ci-payload-1',now()+interval '9 seconds'
  );

  IF NOT accepted_1 OR reason_1 <> 'PAY_WEBHOOK_ACCEPTED' OR accepted_2 OR reason_2 <> 'PAY_WEBHOOK_DUPLICATE' THEN
    RAISE EXCEPTION 'payment webhook idempotency failed';
  END IF;
END
$$;

INSERT INTO ledger_entries (
  id, debit_account_ref, credit_account_ref, amount_minor, currency,
  provider_evidence_ref, economic_event_ref, correlation_id, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000d501',
  'external-provider/ci-b/payer',
  'identity/specialist-r2',
  10000,'RUB',
  'provider-evidence/ci-b-captured',
  'payment/00000000-0000-0000-0000-00000000d302',
  'corr-r2-payment-ci',
  now()+interval '10 seconds'
);

INSERT INTO payment_effects (
  id, receipt_id, attempt_id, effect_kind,
  provider_evidence_ref, ledger_entry_id, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000d601',
  '00000000-0000-0000-0000-00000000d401',
  '00000000-0000-0000-0000-00000000d302',
  'CAPTURED',
  'provider-evidence/ci-b-captured',
  '00000000-0000-0000-0000-00000000d501',
  now()+interval '10 seconds'
);

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO payment_effects (
      id, receipt_id, attempt_id, effect_kind,
      provider_evidence_ref, ledger_entry_id, occurred_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000d602',
      '00000000-0000-0000-0000-00000000d401',
      '00000000-0000-0000-0000-00000000d302',
      'CAPTURED',
      'provider-evidence/ci-b-captured',
      '00000000-0000-0000-0000-00000000d501',
      now()+interval '11 seconds'
    );
  EXCEPTION WHEN unique_violation THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'duplicate callback created a second economic effect';
  END IF;
END
$$;
