\set ON_ERROR_STOP on

INSERT INTO ledger_entries (
  id, debit_account_ref, credit_account_ref, amount_minor, currency,
  provider_evidence_ref, economic_event_ref, correlation_id, occurred_at
) VALUES
(
  '00000000-0000-0000-0000-00000000aa51',
  'external-store-provider/payer',
  'identity/specialist-store-ci',
  10000,
  'RUB',
  'store-evidence/renewal-ci',
  'store-renewal/subscription-ci-1',
  'corr-r4-store-renewal-ci',
  now() + interval '20 minutes'
),
(
  '00000000-0000-0000-0000-00000000aa52',
  'identity/specialist-store-ci',
  'external-store-provider/payer',
  10000,
  'RUB',
  'store-evidence/refund-ci',
  'store-refund/subscription-ci-1',
  'corr-r4-store-refund-ci',
  now() + interval '24 minutes'
);

DO $$
DECLARE applied boolean;
DECLARE reason text;
BEGIN
  SELECT result.applied, result.reason_code
  INTO applied, reason
  FROM apgic_apply_store_lifecycle_event(
    '00000000-0000-0000-0000-00000000aa60',
    '00000000-0000-0000-0000-00000000aa70',
    '00000000-0000-0000-0000-00000000a601',
    'store-provider-event-renewal-1',
    'store-renewal-tx-1',
    'subscription-ci-1',
    '00000000-0000-0000-0000-00000000b003',
    '00000000-0000-0000-0000-00000000a902',
    1,
    'RENEWAL',
    10000,
    'RUB',
    'store-evidence/renewal-ci',
    '00000000-0000-0000-0000-00000000aa51',
    now() + interval '20 minutes'
  ) AS result;

  IF NOT applied OR reason <> 'STORE_EVENT_APPLIED' THEN
    RAISE EXCEPTION 'renewal store event was not applied';
  END IF;
END
$$;

DO $$
DECLARE applied boolean;
DECLARE reason text;
BEGIN
  SELECT result.applied, result.reason_code
  INTO applied, reason
  FROM apgic_apply_store_lifecycle_event(
    '00000000-0000-0000-0000-00000000aa61',
    '00000000-0000-0000-0000-00000000aa71',
    '00000000-0000-0000-0000-00000000a601',
    'store-provider-event-renewal-1',
    'store-renewal-tx-1',
    'subscription-ci-1',
    '00000000-0000-0000-0000-00000000b003',
    '00000000-0000-0000-0000-00000000a902',
    1,
    'RENEWAL',
    10000,
    'RUB',
    'store-evidence/renewal-ci',
    '00000000-0000-0000-0000-00000000aa51',
    now() + interval '20 minutes'
  ) AS result;

  IF applied OR reason <> 'STORE_EVENT_DUPLICATE' THEN
    RAISE EXCEPTION 'duplicate store event created a second effect';
  END IF;

  IF (
    SELECT count(*) FROM store_lifecycle_effects
    WHERE lifecycle_event_id = '00000000-0000-0000-0000-00000000aa60'
  ) <> 1 THEN
    RAISE EXCEPTION 'renewal event did not create exactly one lifecycle effect';
  END IF;
END
$$;

SELECT *
FROM apgic_apply_store_lifecycle_event(
  '00000000-0000-0000-0000-00000000aa63',
  '00000000-0000-0000-0000-00000000aa73',
  '00000000-0000-0000-0000-00000000a601',
  'store-provider-event-grace-3',
  'store-renewal-tx-1',
  'subscription-ci-1',
  '00000000-0000-0000-0000-00000000b003',
  '00000000-0000-0000-0000-00000000a902',
  3,
  'GRACE_STARTED',
  NULL,
  NULL,
  'store-evidence/grace-ci',
  NULL,
  now() + interval '22 minutes'
);

DO $$
DECLARE applied boolean;
DECLARE reason text;
BEGIN
  SELECT result.applied, result.reason_code
  INTO applied, reason
  FROM apgic_apply_store_lifecycle_event(
    '00000000-0000-0000-0000-00000000aa62',
    '00000000-0000-0000-0000-00000000aa72',
    '00000000-0000-0000-0000-00000000a601',
    'store-provider-event-hold-2',
    'store-renewal-tx-1',
    'subscription-ci-1',
    '00000000-0000-0000-0000-00000000b003',
    '00000000-0000-0000-0000-00000000a902',
    2,
    'HOLD_STARTED',
    NULL,
    NULL,
    'store-evidence/hold-stale-ci',
    NULL,
    now() + interval '21 minutes'
  ) AS result;

  IF applied OR reason <> 'STORE_EVENT_STALE' THEN
    RAISE EXCEPTION 'out-of-order store event rewrote projection';
  END IF;

  IF EXISTS (
    SELECT 1 FROM store_lifecycle_effects
    WHERE lifecycle_event_id = '00000000-0000-0000-0000-00000000aa62'
  ) THEN
    RAISE EXCEPTION 'stale store event created a canonical effect';
  END IF;
END
$$;

DO $$
DECLARE applied boolean;
DECLARE reason text;
BEGIN
  SELECT result.applied, result.reason_code
  INTO applied, reason
  FROM apgic_apply_store_lifecycle_event(
    '00000000-0000-0000-0000-00000000aa64',
    '00000000-0000-0000-0000-00000000aa74',
    '00000000-0000-0000-0000-00000000a601',
    'store-provider-event-refund-4',
    'store-refund-tx-1',
    'subscription-ci-1',
    '00000000-0000-0000-0000-00000000b003',
    '00000000-0000-0000-0000-00000000a902',
    4,
    'REFUND',
    10000,
    'RUB',
    'store-evidence/refund-ci',
    '00000000-0000-0000-0000-00000000aa52',
    now() + interval '24 minutes'
  ) AS result;

  IF NOT applied OR reason <> 'STORE_EVENT_APPLIED' THEN
    RAISE EXCEPTION 'refund store event was not applied';
  END IF;
END
$$;

DO $$
BEGIN
  IF (
    SELECT subscription_state
    FROM store_subscription_projections
    WHERE provider_instance_id = '00000000-0000-0000-0000-00000000a601'
      AND subscription_ref = 'subscription-ci-1'
  ) <> 'REVOKED' THEN
    RAISE EXCEPTION 'store subscription projection did not converge to REVOKED';
  END IF;

  IF (
    SELECT entitlement_state
    FROM store_subscription_projections
    WHERE provider_instance_id = '00000000-0000-0000-0000-00000000a601'
      AND subscription_ref = 'subscription-ci-1'
  ) <> 'REVOKED' THEN
    RAISE EXCEPTION 'store entitlement projection did not converge to REVOKED';
  END IF;

  IF (
    SELECT last_provider_sequence
    FROM store_subscription_projections
    WHERE provider_instance_id = '00000000-0000-0000-0000-00000000a601'
      AND subscription_ref = 'subscription-ci-1'
  ) <> 4 THEN
    RAISE EXCEPTION 'store projection sequence did not converge';
  END IF;

  IF (
    SELECT applied_event_count
    FROM store_subscription_projections
    WHERE provider_instance_id = '00000000-0000-0000-0000-00000000a601'
      AND subscription_ref = 'subscription-ci-1'
  ) <> 3 THEN
    RAISE EXCEPTION 'duplicate/stale event changed applied event count';
  END IF;

  IF (
    SELECT count(*) FROM store_subscription_lifecycle_events
    WHERE provider_instance_id = '00000000-0000-0000-0000-00000000a601'
      AND subscription_ref = 'subscription-ci-1'
  ) <> 4 THEN
    RAISE EXCEPTION 'store event evidence history did not preserve stale event or dedupe duplicate';
  END IF;

  IF (
    SELECT count(*) FROM store_lifecycle_effects effect
    JOIN store_subscription_lifecycle_events event ON event.id = effect.lifecycle_event_id
    WHERE event.provider_instance_id = '00000000-0000-0000-0000-00000000a601'
      AND event.subscription_ref = 'subscription-ci-1'
  ) <> 3 THEN
    RAISE EXCEPTION 'store canonical effects are not exactly-once';
  END IF;
END
$$;

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO store_subscription_lifecycle_events (
      id, provider_instance_id, provider_event_id, external_transaction_id,
      subscription_ref, identity_id, entitlement_id, provider_sequence,
      event_type, amount_minor, currency, provider_evidence_ref,
      ledger_entry_id, occurred_at
    ) VALUES (
      '00000000-0000-0000-0000-00000000aa99',
      '00000000-0000-0000-0000-00000000d001',
      'wrong-provider-event',
      'wrong-provider-tx',
      'subscription-ci-1',
      '00000000-0000-0000-0000-00000000b003',
      '00000000-0000-0000-0000-00000000a902',
      5,
      'REVOCATION',
      NULL,
      NULL,
      'store-evidence/wrong-provider',
      NULL,
      now() + interval '25 minutes'
    );
  EXCEPTION WHEN raise_exception THEN
    blocked := true;
  END;

  IF NOT blocked THEN
    RAISE EXCEPTION 'store lifecycle crossed original provider affinity';
  END IF;
END
$$;
