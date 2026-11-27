\set ON_ERROR_STOP on

-- Preserve R2's original consultation order and never allow a non-Academy
-- published product to masquerade as a direct course sale.
DO $proof$
DECLARE
 blocked boolean:=false;
 message text;
 fixture orders%ROWTYPE;
BEGIN
 SELECT * INTO fixture FROM orders
 WHERE id='00000000-0000-0000-0000-00000000b501';
 IF fixture.id IS NULL OR fixture.booking_id IS NULL THEN
  RAISE EXCEPTION 'canonical booked R2 order disappeared';
 END IF;
 IF NOT EXISTS(
   SELECT 1 FROM pg_constraint WHERE conname='orders_booking_xor_direct_buyer'
 ) OR NOT EXISTS (
   SELECT 1 FROM pg_indexes WHERE indexname='orders_direct_checkout_idempotency'
 ) THEN
  RAISE EXCEPTION 'direct order basis or idempotency constraint missing';
 END IF;
 IF EXISTS (
   SELECT 1 FROM information_schema.columns
   WHERE table_name='store_transaction_verifications'
     AND column_name='policy_snapshot_id' AND is_nullable='NO'
 ) THEN
  RAISE EXCEPTION 'bank verification incorrectly requires store billing policy';
 END IF;
 BEGIN
  -- Reuse legally snapshotted economic roles from R2, but with a distinct
  -- snapshot for this attempted direct checkout. A synthetic booking is
  -- forbidden. A consultation product must never be sold as Academy.
  INSERT INTO legal_transaction_snapshots(
   id,transaction_ref,seller_or_service_provider_id,commercial_owner_id,
   payment_recipient_id,platform_role,fiscal_responsibility_id,
   refund_responsibility_id,payout_beneficiary_id,policy_version,occurred_at
  )
  SELECT '00000000-0000-0000-0000-00000000b491'::uuid,
   'order/00000000-0000-0000-0000-00000000b591',
   seller_or_service_provider_id,commercial_owner_id,payment_recipient_id,
   platform_role,fiscal_responsibility_id,refund_responsibility_id,
   payout_beneficiary_id,policy_version,now()
  FROM legal_transaction_snapshots
  WHERE id='00000000-0000-0000-0000-00000000b401';
  INSERT INTO orders(
   id,booking_id,direct_buyer_identity_id,direct_checkout_idempotency_key,
   offer_ref,price_source_ref,amount_minor,currency,commission_minor,
   pricing_policy_version,commission_policy_version,legal_snapshot_id,
   product_id,organization_direction_id,product_owner_ref,author_refs,
   seller_ref,commercial_owner_ref,payment_recipient_ref,platform_role,
   fiscal_responsibility_ref,refund_responsibility_ref,payout_beneficiary_ref,
   captured_at
  )
  SELECT '00000000-0000-0000-0000-00000000b591'::uuid,
   NULL,'00000000-0000-0000-0000-00000000b002'::uuid,
   'academy_noncourse_reject_001',
   offer_ref,price_source_ref,amount_minor,currency,commission_minor,
   pricing_policy_version,commission_policy_version,
   '00000000-0000-0000-0000-00000000b491'::uuid,
   product_id,organization_direction_id,product_owner_ref,author_refs,
   seller_ref,commercial_owner_ref,payment_recipient_ref,platform_role,
   fiscal_responsibility_ref,refund_responsibility_ref,payout_beneficiary_ref,
   now()
  FROM orders WHERE id=fixture.id;
 EXCEPTION WHEN raise_exception THEN
  GET STACKED DIAGNOSTICS message=MESSAGE_TEXT;
  blocked:=true;
 END;
 IF NOT blocked THEN
  RAISE EXCEPTION 'a non-Academy product entered canonical direct bank checkout';
 END IF;
 IF NOT EXISTS (SELECT 1 FROM orders WHERE id=fixture.id AND booking_id=fixture.booking_id)
 THEN RAISE EXCEPTION 'existing booked order changed during bank guard proof'; END IF;
END
$proof$;
