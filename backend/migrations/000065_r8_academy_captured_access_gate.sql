-- R8: one fail-closed Academy access gate across enrollment, protected lessons,
-- progress, grading, certificates and author/learner readbacks.
-- Canonical Commerce owns the payment facts; Academy only verifies them.
-- A bare ACTIVE store_entitlement without matching captured PSP/store evidence
-- must not unlock any learning operation.
CREATE OR REPLACE FUNCTION academy_entitlement_is_active(p_entitlement uuid)
 RETURNS boolean LANGUAGE sql STABLE AS $academy_entitlement_access$
 SELECT EXISTS (
  SELECT 1
  FROM store_entitlements t
  JOIN store_transaction_verifications sv ON sv.id=t.verification_id
    AND sv.identity_id=t.identity_id AND sv.product_ref=t.product_ref
  JOIN orders o ON o.id=sv.order_id AND o.product_id::text=t.product_ref
  JOIN payment_attempts pa ON pa.id=sv.payment_attempt_id
    AND pa.order_id=o.id AND pa.state='SUCCEEDED'
    AND pa.amount_minor=o.amount_minor AND pa.currency=o.currency
  JOIN payment_effects pe ON pe.id=sv.payment_effect_id
    AND pe.attempt_id=pa.id AND pe.effect_kind='CAPTURED'
    AND pe.ledger_entry_id=sv.ledger_entry_id
  JOIN ledger_entries le ON le.id=sv.ledger_entry_id
    AND le.provider_evidence_ref=pe.provider_evidence_ref
  WHERE t.id=p_entitlement AND t.state='ACTIVE'
    AND NOT EXISTS (
      SELECT 1 FROM store_subscription_projections projection
      WHERE projection.entitlement_id=t.id
        AND (projection.identity_id IS DISTINCT FROM t.identity_id
             OR projection.entitlement_state<>'ACTIVE')
    )
 );
 $academy_entitlement_access$;

COMMENT ON FUNCTION academy_entitlement_is_active(uuid) IS
 'R8 Academy-only read gate: canonical verified Commerce capture + effective Entitlement; never creates a payment, grant or payout.';
