\set ON_ERROR_STOP on

UPDATE bookings
SET state = 'CANCELLED',
    updated_at = now() + interval '10 minutes'
WHERE id = '00000000-0000-0000-0000-00000000b301';

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO refund_requests (
      id, booking_id, order_id, original_payment_attempt_id,
      original_provider_config_id, amount_minor, currency,
      policy_version, policy_decision, reason_code, idempotency_key,
      execution_owner, state, created_at, updated_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000e099',
      '00000000-0000-0000-0000-00000000b301',
      '00000000-0000-0000-0000-00000000b501',
      '00000000-0000-0000-0000-00000000d302',
      '00000000-0000-0000-0000-00000000d101',
      10000,'RUB',
      'refund-r2-ci-v1','ALLOW','CLIENT_CANCEL_ALLOWED','refund-cross-provider',
      'EXTERNAL_PROVIDER','REQUESTED',now()+interval '11 minutes',now()+interval '11 minutes'
    );
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'refund was allowed through a provider different from original payment provider';
  END IF;
END
$$;

INSERT INTO refund_requests (
  id, booking_id, order_id, original_payment_attempt_id,
  original_provider_config_id, amount_minor, currency,
  policy_version, policy_decision, reason_code, idempotency_key,
  execution_owner, state, created_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-00000000e101',
  '00000000-0000-0000-0000-00000000b301',
  '00000000-0000-0000-0000-00000000b501',
  '00000000-0000-0000-0000-00000000d302',
  '00000000-0000-0000-0000-00000000d102',
  10000,'RUB',
  'refund-r2-ci-v1','ALLOW','CLIENT_CANCEL_ALLOWED','refund-full-1',
  'EXTERNAL_PROVIDER','REQUESTED',now()+interval '12 minutes',now()+interval '12 minutes'
);

UPDATE refund_requests
SET state='SENT',
    provider_reference='ci-b/refund-1',
    updated_at=now()+interval '13 minutes'
WHERE id='00000000-0000-0000-0000-00000000e101';

UPDATE refund_requests
SET state='AMBIGUOUS',
    updated_at=now()+interval '14 minutes'
WHERE id='00000000-0000-0000-0000-00000000e101';

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    UPDATE refund_requests
    SET state='SENT',
        provider_reference='ci-b/refund-2',
        updated_at=now()+interval '15 minutes'
    WHERE id='00000000-0000-0000-0000-00000000e101';
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'AMBIGUOUS refund was blindly re-sent';
  END IF;
END
$$;

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO refund_requests (
      id, booking_id, order_id, original_payment_attempt_id,
      original_provider_config_id, amount_minor, currency,
      policy_version, policy_decision, reason_code, idempotency_key,
      execution_owner, state, created_at, updated_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000e102',
      '00000000-0000-0000-0000-00000000b301',
      '00000000-0000-0000-0000-00000000b501',
      '00000000-0000-0000-0000-00000000d302',
      '00000000-0000-0000-0000-00000000d102',
      1,'RUB',
      'refund-r2-ci-v1','ALLOW','CLIENT_CANCEL_ALLOWED','refund-over-reserved',
      'EXTERNAL_PROVIDER','REQUESTED',now()+interval '16 minutes',now()+interval '16 minutes'
    );
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'refund reservations exceeded captured payment amount';
  END IF;
END
$$;

SELECT *
FROM apgic_record_payment_webhook(
  '00000000-0000-0000-0000-00000000e201',
  '00000000-0000-0000-0000-00000000d002',
  'provider-event-refunded-1',
  'sha256:ci-refund-payload-1',
  now()+interval '17 minutes'
);

INSERT INTO ledger_entries (
  id, debit_account_ref, credit_account_ref, amount_minor, currency,
  provider_evidence_ref, economic_event_ref, correlation_id, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000e301',
  'identity/specialist-r2',
  'external-provider/ci-b/payer',
  10000,'RUB',
  'provider-evidence/ci-b-refunded',
  'refund/00000000-0000-0000-0000-00000000e101',
  'corr-r2-refund-ci',
  now()+interval '18 minutes'
);

SELECT *
FROM apgic_apply_refund_success(
  '00000000-0000-0000-0000-00000000e101',
  '00000000-0000-0000-0000-00000000e201',
  'provider-evidence/ci-b-refunded',
  '00000000-0000-0000-0000-00000000e301',
  '00000000-0000-0000-0000-00000000e401',
  now()+interval '18 minutes'
);

DO $$
DECLARE reapplied boolean; reapply_reason text;
BEGIN
  SELECT applied, reason_code
  INTO reapplied, reapply_reason
  FROM apgic_apply_refund_success(
    '00000000-0000-0000-0000-00000000e101',
    '00000000-0000-0000-0000-00000000e201',
    'provider-evidence/ci-b-refunded',
    '00000000-0000-0000-0000-00000000e301',
    '00000000-0000-0000-0000-00000000e402',
    now()+interval '19 minutes'
  );

  IF reapplied OR reapply_reason <> 'REFUND_ALREADY_APPLIED' THEN
    RAISE EXCEPTION 'duplicate refund outcome was not idempotent';
  END IF;
END
$$;

DO $$
BEGIN
  IF (SELECT state FROM refund_requests WHERE id='00000000-0000-0000-0000-00000000e101') <> 'SUCCEEDED' THEN
    RAISE EXCEPTION 'refund did not reach SUCCEEDED with provider evidence';
  END IF;
  IF (SELECT count(*) FROM refund_effects WHERE refund_request_id='00000000-0000-0000-0000-00000000e101') <> 1 THEN
    RAISE EXCEPTION 'refund economic evidence is not exactly once';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM orders WHERE id='00000000-0000-0000-0000-00000000b501') THEN
    RAISE EXCEPTION 'refund deleted original Order history';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM payment_attempts WHERE id='00000000-0000-0000-0000-00000000d302' AND state='SUCCEEDED') THEN
    RAISE EXCEPTION 'refund rewrote/deleted original Payment history';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM payment_effects WHERE attempt_id='00000000-0000-0000-0000-00000000d302' AND effect_kind='CAPTURED') THEN
    RAISE EXCEPTION 'refund removed original capture evidence';
  END IF;
END
$$;
