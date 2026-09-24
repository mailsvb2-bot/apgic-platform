\set ON_ERROR_STOP on

INSERT INTO booking_slots (
  id, specialist_identity_id, tenant_scope, starts_at, ends_at, exclusive
) VALUES (
  '00000000-0000-0000-0000-00000000a101',
  '00000000-0000-0000-0000-00000000b001',
  'tenant/r2-store-ci',
  now() + interval '5 hours',
  now() + interval '6 hours',
  true
);

SELECT *
FROM apgic_acquire_slot_hold(
  '00000000-0000-0000-0000-00000000a201',
  '00000000-0000-0000-0000-00000000a101',
  '00000000-0000-0000-0000-00000000b003',
  now() + interval '10 minutes',
  now()
);

SELECT *
FROM apgic_create_booking_from_hold(
  '00000000-0000-0000-0000-00000000a301',
  '00000000-0000-0000-0000-00000000a201',
  now() + interval '1 minute'
);

UPDATE bookings
SET state = 'CONFIRMED',
    updated_at = now() + interval '2 minutes'
WHERE id = '00000000-0000-0000-0000-00000000a301';

INSERT INTO legal_transaction_snapshots (
  id, transaction_ref, seller_or_service_provider_id, commercial_owner_id,
  payment_recipient_id, platform_role, fiscal_responsibility_id,
  refund_responsibility_id, payout_beneficiary_id, policy_version, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000a401',
  'order/00000000-0000-0000-0000-00000000a501',
  'identity/specialist-store-ci',
  'identity/specialist-store-ci',
  'identity/specialist-store-ci',
  'MARKETPLACE_INTERMEDIARY',
  'identity/specialist-store-ci',
  'identity/specialist-store-ci',
  'identity/specialist-store-ci',
  'legal-r2-store-ci-v1',
  now() + interval '2 minutes'
);

INSERT INTO orders (
  id, booking_id, offer_ref, price_source_ref, amount_minor, currency,
  commission_minor, pricing_policy_version, commission_policy_version,
  legal_snapshot_id, seller_ref, commercial_owner_ref, payment_recipient_ref,
  platform_role, fiscal_responsibility_ref, refund_responsibility_ref,
  payout_beneficiary_ref, captured_at
) VALUES (
  '00000000-0000-0000-0000-00000000a501',
  '00000000-0000-0000-0000-00000000a301',
  'offer/store-subscription-ci',
  'price/store-subscription-ci',
  10000,
  'RUB',
  1000,
  'pricing-r2-ci-v1',
  'commission-r2-ci-v1',
  '00000000-0000-0000-0000-00000000a401',
  'identity/specialist-store-ci',
  'identity/specialist-store-ci',
  'identity/specialist-store-ci',
  'MARKETPLACE_INTERMEDIARY',
  'identity/specialist-store-ci',
  'identity/specialist-store-ci',
  'identity/specialist-store-ci',
  now() + interval '2 minutes'
);

INSERT INTO connector_instances (
  id, capability_class, provider_kind, status, config_ref
) VALUES (
  '00000000-0000-0000-0000-00000000a601',
  'PAYMENT_PROVIDER',
  'CI_STORE_PROVIDER',
  'ACTIVE',
  'secret://ci/store-provider'
);

INSERT INTO payment_provider_config_versions (
  id, provider_instance_id, config_version, status, priority,
  manifest_version, certification_evidence_refs, jurisdiction_codes,
  currencies, method_codes, rail_codes, execution_owner,
  routing_weight_bps, credential_version_ref, effective_from
) VALUES (
  '00000000-0000-0000-0000-00000000a602',
  '00000000-0000-0000-0000-00000000a601',
  'store-provider-ci-v1',
  'ACTIVE',
  10,
  'store-manifest-ci-v1',
  ARRAY['evidence/store-sandbox-ci'],
  ARRAY['RU'],
  ARRAY['RUB'],
  ARRAY['APPLE_IAP'],
  ARRAY['STORE_BILLING'],
  'EXTERNAL_PROVIDER',
  10000,
  'secret-version/ci-store-provider/v1',
  now()
);

INSERT INTO payment_provider_health_snapshots (
  id, provider_config_id, health, conversion_rate_bps, latency_p95_ms,
  provider_reported_fee_bps, reconciliation_pending_count,
  reconciliation_mismatch_count, guardrail_action, evidence_refs, observed_at
) VALUES (
  '00000000-0000-0000-0000-00000000a603',
  '00000000-0000-0000-0000-00000000a602',
  'HEALTHY',
  9000,
  250,
  300,
  0,
  0,
  'ALLOW_NEW_ATTEMPTS',
  ARRAY['metrics/store-provider-ci'],
  now()
);

INSERT INTO store_policy_snapshots (
  id, policy_version, effective_at
) VALUES (
  '00000000-0000-0000-0000-00000000a701',
  'store-policy-ci-v1',
  now() - interval '1 minute'
);

INSERT INTO store_policy_rules (
  id, snapshot_id, product_type, surface, store_code, storefront,
  jurisdiction_code, enabled, rail_code, reason_code
) VALUES (
  '00000000-0000-0000-0000-00000000a702',
  '00000000-0000-0000-0000-00000000a701',
  'SUBSCRIPTION',
  'IOS',
  'STORE_A',
  'RU',
  'RU',
  true,
  'STORE_BILLING',
  'STORE_BILLING_REQUIRED'
);

INSERT INTO store_commerce_decisions (
  id, snapshot_id, identity_id, product_ref, product_type, surface,
  store_code, storefront, jurisdiction_code, outcome, rail_code,
  reason_code, decided_at
) VALUES (
  '00000000-0000-0000-0000-00000000a703',
  '00000000-0000-0000-0000-00000000a701',
  '00000000-0000-0000-0000-00000000b003',
  'product/subscription-ci',
  'SUBSCRIPTION',
  'IOS',
  'STORE_A',
  'RU',
  'RU',
  'ALLOWED',
  'STORE_BILLING',
  'STORE_BILLING_REQUIRED',
  now()
);

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO store_commerce_decisions (
      id, snapshot_id, identity_id, product_ref, product_type, surface,
      store_code, storefront, jurisdiction_code, outcome, rail_code,
      reason_code, decided_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000a704',
      '00000000-0000-0000-0000-00000000a701',
      '00000000-0000-0000-0000-00000000b003',
      'product/unknown-ci',
      'UNKNOWN_PRODUCT',
      'IOS',
      'STORE_A',
      'RU',
      'RU',
      'ALLOWED',
      'STORE_BILLING',
      'STORE_BILLING_ALLOWED',
      now()
    );
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'unknown store policy path did not fail closed';
  END IF;
END
$$;

INSERT INTO store_commerce_decisions (
  id, snapshot_id, identity_id, product_ref, product_type, surface,
  store_code, storefront, jurisdiction_code, outcome, rail_code,
  reason_code, decided_at
) VALUES (
  '00000000-0000-0000-0000-00000000a705',
  '00000000-0000-0000-0000-00000000a701',
  '00000000-0000-0000-0000-00000000b003',
  'product/unknown-ci',
  'UNKNOWN_PRODUCT',
  'IOS',
  'STORE_A',
  'RU',
  'RU',
  'PURCHASE_DISABLED',
  'PURCHASE_DISABLED',
  'STORE_POLICY_NOT_CONFIGURED',
  now()
);

INSERT INTO payment_routing_decisions (
  id, order_id, policy_version, provider_config_id, jurisdiction_code,
  selected_method_code, selected_rail_code, candidate_evidence,
  health_snapshot, health_snapshot_id, decided_at
) VALUES (
  '00000000-0000-0000-0000-00000000a801',
  '00000000-0000-0000-0000-00000000a501',
  'store-routing-ci-v1',
  '00000000-0000-0000-0000-00000000a602',
  'RU',
  'APPLE_IAP',
  'STORE_BILLING',
  '[{"provider":"CI_STORE_PROVIDER","eligible":true}]'::jsonb,
  '{"status":"HEALTHY","guardrail":"ALLOW_NEW_ATTEMPTS"}'::jsonb,
  '00000000-0000-0000-0000-00000000a603',
  now() + interval '3 minutes'
);

INSERT INTO payment_attempts (
  id, order_id, routing_decision_id, idempotency_key,
  amount_minor, currency, state, created_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-00000000a802',
  '00000000-0000-0000-0000-00000000a501',
  '00000000-0000-0000-0000-00000000a801',
  'store-payment-attempt-ci',
  10000,
  'RUB',
  'CREATED',
  now() + interval '3 minutes',
  now() + interval '3 minutes'
);

UPDATE payment_attempts
SET state = 'SENT',
    provider_reference = 'store-provider/attempt-ci',
    updated_at = now() + interval '4 minutes'
WHERE id = '00000000-0000-0000-0000-00000000a802';

UPDATE payment_attempts
SET state = 'SUCCEEDED',
    provider_evidence_ref = 'store-evidence/captured-ci',
    updated_at = now() + interval '5 minutes'
WHERE id = '00000000-0000-0000-0000-00000000a802';

SELECT *
FROM apgic_record_payment_webhook(
  '00000000-0000-0000-0000-00000000a803',
  '00000000-0000-0000-0000-00000000a601',
  'store-provider-event-captured-ci',
  'sha256:store-provider-payload-ci',
  now() + interval '5 minutes'
);

INSERT INTO ledger_entries (
  id, debit_account_ref, credit_account_ref, amount_minor, currency,
  provider_evidence_ref, economic_event_ref, correlation_id, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000a804',
  'external-store-provider/payer',
  'identity/specialist-store-ci',
  10000,
  'RUB',
  'store-evidence/captured-ci',
  'payment/00000000-0000-0000-0000-00000000a802',
  'corr-r2-store-ci',
  now() + interval '5 minutes'
);

INSERT INTO payment_effects (
  id, receipt_id, attempt_id, effect_kind,
  provider_evidence_ref, ledger_entry_id, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000a805',
  '00000000-0000-0000-0000-00000000a803',
  '00000000-0000-0000-0000-00000000a802',
  'CAPTURED',
  'store-evidence/captured-ci',
  '00000000-0000-0000-0000-00000000a804',
  now() + interval '5 minutes'
);

INSERT INTO store_transaction_verifications (
  id, identity_id, order_id, payment_attempt_id, payment_effect_id,
  ledger_entry_id, provider_instance_id, external_transaction_id,
  product_ref, entitlement_kind, policy_snapshot_id, provider_evidence_ref,
  verified_at
) VALUES (
  '00000000-0000-0000-0000-00000000a901',
  '00000000-0000-0000-0000-00000000b003',
  '00000000-0000-0000-0000-00000000a501',
  '00000000-0000-0000-0000-00000000a802',
  '00000000-0000-0000-0000-00000000a805',
  '00000000-0000-0000-0000-00000000a804',
  '00000000-0000-0000-0000-00000000a601',
  'store-tx-ci-1',
  'product/subscription-ci',
  'SUBSCRIPTION',
  '00000000-0000-0000-0000-00000000a701',
  'store-evidence/server-verified-ci',
  now() + interval '6 minutes'
);

INSERT INTO store_entitlements (
  id, identity_id, product_ref, kind, state,
  verification_id, activated_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-00000000a902',
  '00000000-0000-0000-0000-00000000b003',
  'product/subscription-ci',
  'SUBSCRIPTION',
  'ACTIVE',
  '00000000-0000-0000-0000-00000000a901',
  now() + interval '6 minutes',
  now() + interval '6 minutes'
);

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO store_entitlements (
      id, identity_id, product_ref, kind, state,
      verification_id, activated_at, updated_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000a903',
      '00000000-0000-0000-0000-00000000b003',
      'product/subscription-ci',
      'SUBSCRIPTION',
      'ACTIVE',
      '00000000-0000-0000-0000-00000000a901',
      now() + interval '7 minutes',
      now() + interval '7 minutes'
    );
  EXCEPTION WHEN unique_violation THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'store transaction created duplicate entitlement transition';
  END IF;
END
$$;

INSERT INTO client_installations (
  id, identity_id, platform, state, updated_at
) VALUES (
  '00000000-0000-0000-0000-00000000a910',
  '00000000-0000-0000-0000-00000000b003',
  'IOS',
  'ACTIVE',
  now()
);

INSERT INTO delete_account_requests (
  id, identity_id, source_surface, state, created_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-00000000a920',
  '00000000-0000-0000-0000-00000000b003',
  'IOS',
  'REQUESTED',
  now(),
  now()
);

UPDATE delete_account_requests
SET state = 'IDENTITY_RECONFIRMED',
    updated_at = now() + interval '1 second'
WHERE id = '00000000-0000-0000-0000-00000000a920';

UPDATE delete_account_requests
SET state = 'RETENTION_CLASSIFIED',
    retention_snapshot = '{"data_classes":[]}'::jsonb,
    updated_at = now() + interval '2 seconds'
WHERE id = '00000000-0000-0000-0000-00000000a920';

UPDATE delete_account_requests
SET state = 'PROVIDER_ERASURE_PENDING',
    updated_at = now() + interval '3 seconds'
WHERE id = '00000000-0000-0000-0000-00000000a920';

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    UPDATE delete_account_requests
    SET state = 'COMPLETED',
        updated_at = now() + interval '4 seconds',
        completed_at = now() + interval '4 seconds'
    WHERE id = '00000000-0000-0000-0000-00000000a920';
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'account deletion hid active external store subscription';
  END IF;
END
$$;

INSERT INTO store_subscription_exit_actions (
  id, delete_request_id, entitlement_id, action, management_target,
  billing_cancelled_by_apgic, reason_code, created_at
) VALUES (
  '00000000-0000-0000-0000-00000000a921',
  '00000000-0000-0000-0000-00000000a920',
  '00000000-0000-0000-0000-00000000a902',
  'MANAGE_EXTERNALLY',
  'https://store.example/subscriptions',
  false,
  'EXTERNAL_STORE_SUBSCRIPTION_ACTIVE',
  now() + interval '4 seconds'
);

UPDATE delete_account_requests
SET state = 'COMPLETED',
    updated_at = now() + interval '5 seconds',
    completed_at = now() + interval '5 seconds'
WHERE id = '00000000-0000-0000-0000-00000000a920';

INSERT INTO device_integrity_evidence (
  id, identity_id, installation_id, provider_kind, provider_evidence_ref,
  verdict, server_verified, observed_at
) VALUES (
  '00000000-0000-0000-0000-00000000a930',
  '00000000-0000-0000-0000-00000000b003',
  '00000000-0000-0000-0000-00000000a910',
  'CI_INTEGRITY_PROVIDER',
  'integrity-evidence/negative-ci',
  'NEGATIVE',
  true,
  now()
);

INSERT INTO audit_records (
  id, actor_id, action, scope, resource_ref, new_state, reason,
  policy_version, correlation_id, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000a931',
  'system/integrity-risk',
  'device_integrity.risk_decision',
  'platform',
  'identity/00000000-0000-0000-0000-00000000b003',
  '{"action":"STEP_UP","verdict":"NEGATIVE"}'::jsonb,
  'DEVICE_INTEGRITY_NEGATIVE_STEP_UP',
  'integrity-policy-ci-v1',
  'corr-integrity-ci',
  now()
);

INSERT INTO device_integrity_risk_decisions (
  id, evidence_id, action, reason_code, policy_version,
  appeal_path, audit_record_id, decided_at
) VALUES (
  '00000000-0000-0000-0000-00000000a932',
  '00000000-0000-0000-0000-00000000a930',
  'STEP_UP',
  'DEVICE_INTEGRITY_NEGATIVE_STEP_UP',
  'integrity-policy-ci-v1',
  '/support/integrity-appeal',
  '00000000-0000-0000-0000-00000000a931',
  now()
);

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO device_integrity_risk_decisions (
      id, evidence_id, action, reason_code, policy_version,
      appeal_path, audit_record_id, decided_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000a933',
      '00000000-0000-0000-0000-00000000a930',
      'DENY',
      'DEVICE_INTEGRITY_AUTOMATIC_BAN',
      'integrity-policy-ci-v1',
      '/support/integrity-appeal',
      '00000000-0000-0000-0000-00000000a931',
      now()
    );
  EXCEPTION WHEN check_violation THEN
    blocked := true;
  END;
  IF NOT blocked THEN
    RAISE EXCEPTION 'device integrity signal allowed unexplained automatic ban';
  END IF;
END
$$;

INSERT INTO noncash_entitlement_entries (
  id, account_ref, unit_kind, event_kind, units,
  source_ref, policy_version, idempotency_key, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000a940',
  'identity/00000000-0000-0000-0000-00000000b003',
  'CREDITS',
  'GRANT',
  100,
  'campaign/ci',
  'credits-policy-ci-v1',
  'credits-grant-ci-1',
  now()
);

INSERT INTO noncash_entitlement_entries (
  id, account_ref, unit_kind, event_kind, units,
  source_ref, policy_version, idempotency_key, occurred_at
) VALUES (
  '00000000-0000-0000-0000-00000000a941',
  'identity/00000000-0000-0000-0000-00000000b003',
  'CREDITS',
  'SPEND',
  60,
  'order/00000000-0000-0000-0000-00000000a501',
  'credits-policy-ci-v1',
  'credits-spend-ci-1',
  now() + interval '1 second'
);

DO $$
DECLARE cashout_blocked boolean := false;
DECLARE overspend_blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO noncash_entitlement_entries (
      id, account_ref, unit_kind, event_kind, units,
      source_ref, policy_version, idempotency_key, occurred_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000a942',
      'identity/00000000-0000-0000-0000-00000000b003',
      'CREDITS',
      'CASH_OUT',
      10,
      'cashout/forbidden',
      'credits-policy-ci-v1',
      'credits-cashout-ci',
      now()
    );
  EXCEPTION WHEN check_violation THEN
    cashout_blocked := true;
  END;

  BEGIN
    INSERT INTO noncash_entitlement_entries (
      id, account_ref, unit_kind, event_kind, units,
      source_ref, policy_version, idempotency_key, occurred_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000a943',
      'identity/00000000-0000-0000-0000-00000000b003',
      'CREDITS',
      'SPEND',
      50,
      'order/overspend',
      'credits-policy-ci-v1',
      'credits-overspend-ci',
      now()
    );
  EXCEPTION WHEN raise_exception THEN
    overspend_blocked := true;
  END;

  IF NOT cashout_blocked THEN
    RAISE EXCEPTION 'non-cash credits allowed CASH_OUT';
  END IF;
  IF NOT overspend_blocked THEN
    RAISE EXCEPTION 'non-cash credits allowed overspend';
  END IF;
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'noncash_entitlement_entries'
      AND column_name IN ('currency','amount_minor','cash_balance','wallet_balance')
  ) THEN
    RAISE EXCEPTION 'non-cash entitlement schema contains monetary wallet field';
  END IF;
END
$$;
