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
DECLARE
  applied boolean;
  reason text;
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
