-- R2 -> R3 production grant policy. No test-only ledger entry or
-- browser-claimed entitlement can manufacture access to a live consultation.
-- A provider capture must agree with canonical order, signed receipt,
-- attempt, ledger and actual booking. Any refund blocks new grants.
CREATE FUNCTION apgic_live_consultation_payment_authorized(
  p_booking uuid, p_entitlement_basis text, p_now timestamptz
) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS (
  SELECT 1 FROM bookings b
   JOIN orders o ON o.booking_id=b.id
   JOIN payment_attempts attempt ON attempt.order_id=o.id
     AND attempt.state='SUCCEEDED'
     AND attempt.amount_minor=o.amount_minor
     AND attempt.currency=o.currency
   JOIN payment_effects effect ON effect.attempt_id=attempt.id
     AND effect.effect_kind='CAPTURED' AND effect.ledger_entry_id IS NOT NULL
   JOIN payment_webhook_receipts receipt ON receipt.id=effect.receipt_id
     AND receipt.signature_verified=true
   JOIN ledger_entries ledger ON ledger.id=effect.ledger_entry_id
     AND ledger.economic_event_ref=o.id::text
     AND ledger.amount_minor=o.amount_minor
     AND ledger.currency=o.currency
     AND btrim(ledger.provider_evidence_ref)<>''
   WHERE b.id=p_booking AND b.state='CONFIRMED'
     AND b.ends_at>p_now
     AND p_entitlement_basis='ledger/' || ledger.id::text
     AND NOT EXISTS (
       SELECT 1 FROM payment_effects reversal
       JOIN payment_attempts refund_attempt ON refund_attempt.id=reversal.attempt_id
       WHERE refund_attempt.order_id=o.id AND reversal.effect_kind='REFUNDED'
     )
     AND NOT EXISTS (
       SELECT 1 FROM payment_attempts uncertain
       WHERE uncertain.order_id=o.id AND uncertain.state='AMBIGUOUS'
     )
     AND (
       NOT EXISTS (SELECT 1 FROM instant_booking_links link
                   WHERE link.booking_id=b.id)
       OR apgic_instant_booking_payment_verified(b.id)
     )
 )
$$;
