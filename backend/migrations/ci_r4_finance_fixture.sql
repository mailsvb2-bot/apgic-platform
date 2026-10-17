-- R4 finance fixture: one canonical paid + COMPLETED consultation chain.
-- Depends on CI fixtures loaded before it (identities b001/b002, communication
-- connector c001, payment providers d001/d002 with configs d101/d102 and
-- health snapshot d112). Used by ci_r4_finance_invariants.sql (inside a
-- rolled-back transaction) and by the runtimepostgres finance integration
-- test (inside its own rolled-back transaction), so it never contaminates
-- later fixtures.
INSERT INTO booking_slots (id, specialist_identity_id, tenant_scope, starts_at, ends_at, exclusive)
VALUES ('00000000-0000-0000-0000-0000000f4410','00000000-0000-0000-0000-00000000b001',
        'tenant/r4-finance-ci', now() + interval '1 hour', now() + interval '2 hours', true);

SELECT * FROM apgic_acquire_slot_hold(
  '00000000-0000-0000-0000-0000000f4411','00000000-0000-0000-0000-0000000f4410',
  '00000000-0000-0000-0000-00000000b002', now() + interval '10 minutes', now());
SELECT * FROM apgic_create_booking_from_hold(
  '00000000-0000-0000-0000-0000000f4412','00000000-0000-0000-0000-0000000f4411', now() + interval '1 minute');
UPDATE bookings SET state = 'CONFIRMED', updated_at = now() + interval '2 minutes'
 WHERE id = '00000000-0000-0000-0000-0000000f4412';

INSERT INTO legal_transaction_snapshots (
  id, transaction_ref, seller_or_service_provider_id, commercial_owner_id, payment_recipient_id,
  platform_role, fiscal_responsibility_id, refund_responsibility_id, payout_beneficiary_id,
  policy_version, occurred_at
) VALUES (
  '00000000-0000-0000-0000-0000000f4440','order/00000000-0000-0000-0000-0000000f4450',
  'identity/specialist-r4','identity/specialist-r4','identity/specialist-r4',
  'MARKETPLACE_INTERMEDIARY','identity/specialist-r4','identity/specialist-r4','identity/specialist-r4',
  'legal-r4-ci-v1', now() + interval '2 minutes');

INSERT INTO orders (
  id, booking_id, offer_ref, price_source_ref, amount_minor, currency, commission_minor,
  pricing_policy_version, commission_policy_version, legal_snapshot_id, seller_ref,
  commercial_owner_ref, payment_recipient_ref, platform_role, fiscal_responsibility_ref,
  refund_responsibility_ref, payout_beneficiary_ref, captured_at, product_owner_ref, author_refs
) VALUES (
  '00000000-0000-0000-0000-0000000f4450','00000000-0000-0000-0000-0000000f4412',
  'offer/r4-ci','price/r4-ci',20000,'RUB',2000,'pricing-r2-ci-v1','commission-r2-ci-v1',
  '00000000-0000-0000-0000-0000000f4440','identity/specialist-r4','identity/specialist-r4',
  'identity/specialist-r4','MARKETPLACE_INTERMEDIARY','identity/specialist-r4',
  'identity/specialist-r4','identity/specialist-r4', now() + interval '2 minutes',
  'identity/specialist-r4', ARRAY['identity/specialist-r4']);

INSERT INTO payment_routing_decisions (
  id, order_id, policy_version, provider_config_id, jurisdiction_code, selected_method_code,
  selected_rail_code, candidate_evidence, health_snapshot, health_snapshot_id, decided_at
) VALUES (
  '00000000-0000-0000-0000-0000000f4460','00000000-0000-0000-0000-0000000f4450','routing-r2-ci-v1',
  '00000000-0000-0000-0000-00000000d102','RU','SBP','APGIC_PAYMENT_PROVIDER',
  '[{"provider":"CI_PAYMENT_B","eligible":true}]'::jsonb,'{"CI_PAYMENT_B":"HEALTHY"}'::jsonb,
  '00000000-0000-0000-0000-00000000d112', now() + interval '3 minutes');

INSERT INTO payment_attempts (id, order_id, routing_decision_id, idempotency_key, amount_minor, currency, state, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-0000000f4470','00000000-0000-0000-0000-0000000f4450',
        '00000000-0000-0000-0000-0000000f4460','pay-attempt-r4',20000,'RUB','CREATED',
        now() + interval '3 minutes', now() + interval '3 minutes');
UPDATE payment_attempts SET state='SENT', provider_reference='ci-b/attempt-r4', updated_at=now()+interval '4 minutes'
 WHERE id='00000000-0000-0000-0000-0000000f4470';
UPDATE payment_attempts SET state='SUCCEEDED', provider_evidence_ref='provider-evidence/ci-b-r4-captured', updated_at=now()+interval '5 minutes'
 WHERE id='00000000-0000-0000-0000-0000000f4470';

SELECT * FROM apgic_record_payment_webhook(
  '00000000-0000-0000-0000-0000000f4480','00000000-0000-0000-0000-00000000d002',
  'provider-event-r4-captured','sha256:ci-r4-capture', now() + interval '6 minutes');

INSERT INTO ledger_entries (id, debit_account_ref, credit_account_ref, amount_minor, currency,
  provider_evidence_ref, economic_event_ref, correlation_id, occurred_at)
VALUES ('00000000-0000-0000-0000-0000000f4490','external-provider/ci-b/payer','identity/specialist-r4',
        20000,'RUB','provider-evidence/ci-b-r4-captured','payment/00000000-0000-0000-0000-0000000f4470',
        'corr-r4-finance-ci', now() + interval '6 minutes');

INSERT INTO payment_effects (id, receipt_id, attempt_id, effect_kind, provider_evidence_ref, ledger_entry_id, occurred_at)
VALUES ('00000000-0000-0000-0000-0000000f4491','00000000-0000-0000-0000-0000000f4480',
        '00000000-0000-0000-0000-0000000f4470','CAPTURED','provider-evidence/ci-b-r4-captured',
        '00000000-0000-0000-0000-0000000f4490', now() + interval '6 minutes');

INSERT INTO consultation_sessions (id, booking_id, client_identity_id, specialist_identity_id,
  provider_instance_id, state, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-0000000f4501','00000000-0000-0000-0000-0000000f4412',
        '00000000-0000-0000-0000-00000000b002','00000000-0000-0000-0000-00000000b001',
        '00000000-0000-0000-0000-00000000c001','SCHEDULED', now(), now());

INSERT INTO consultation_lifecycle_facts (id, session_id, fact_type, role, identity_id,
  provider_instance_id, provider_reference, evidence_ref, idempotency_key, occurred_at)
VALUES
('00000000-0000-0000-0000-0000000f4511','00000000-0000-0000-0000-0000000f4501','JOINED','CLIENT',
 '00000000-0000-0000-0000-00000000b002','00000000-0000-0000-0000-00000000c001','room/r4-ci',
 'provider-evidence/r4-client-joined','r4-fact-client-joined', now() + interval '7 minutes'),
('00000000-0000-0000-0000-0000000f4512','00000000-0000-0000-0000-0000000f4501','JOINED','SPECIALIST',
 '00000000-0000-0000-0000-00000000b001','00000000-0000-0000-0000-00000000c001','room/r4-ci',
 'provider-evidence/r4-specialist-joined','r4-fact-specialist-joined', now() + interval '8 minutes');
INSERT INTO consultation_lifecycle_facts (id, session_id, fact_type, role, identity_id,
  provider_instance_id, provider_reference, evidence_ref, idempotency_key, occurred_at)
VALUES ('00000000-0000-0000-0000-0000000f4513','00000000-0000-0000-0000-0000000f4501','STARTED','SYSTEM',
 NULL,'00000000-0000-0000-0000-00000000c001','room/r4-ci','provider-evidence/r4-started',
 'r4-fact-started', now() + interval '9 minutes');
INSERT INTO consultation_lifecycle_facts (id, session_id, fact_type, role, identity_id,
  provider_instance_id, provider_reference, evidence_ref, idempotency_key, occurred_at)
VALUES ('00000000-0000-0000-0000-0000000f4514','00000000-0000-0000-0000-0000000f4501','ENDED','SYSTEM',
 NULL,'00000000-0000-0000-0000-00000000c001','room/r4-ci','provider-evidence/r4-ended',
 'r4-fact-ended', now() + interval '10 minutes');
