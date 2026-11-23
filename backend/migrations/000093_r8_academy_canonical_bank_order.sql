-- R8: booking-free Academy purchases remain canonical Commerce orders.
-- No side order table, no new entitlement issuer and no synthetic booking.
-- This schema change alone does not enable bank collection or academy VIDEO.
ALTER TABLE orders ALTER COLUMN booking_id DROP NOT NULL;
ALTER TABLE orders ADD COLUMN academy_buyer_identity_id uuid REFERENCES identities(id);
ALTER TABLE orders ADD CONSTRAINT orders_single_acquisition_basis
 CHECK ((booking_id IS NOT NULL AND academy_buyer_identity_id IS NULL)
     OR (booking_id IS NULL AND academy_buyer_identity_id IS NOT NULL));
CREATE INDEX orders_academy_buyer_id_idx ON orders(academy_buyer_identity_id)
 WHERE academy_buyer_identity_id IS NOT NULL;

-- Extend, do not replace, the existing legally snapshotted order guard.
CREATE OR REPLACE FUNCTION apgic_order_snapshot_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE
 legal legal_transaction_snapshots%ROWTYPE;
 product_row products%ROWTYPE;
 author_ref text;
 expected_product_owner_ref text;
 academy_price bigint;
 academy_currency text;
BEGIN
 SELECT * INTO legal FROM legal_transaction_snapshots
 WHERE id=NEW.legal_snapshot_id;
 IF NOT FOUND THEN
  RAISE EXCEPTION 'order legal snapshot not found';
 END IF;
 IF (NEW.product_id IS NULL) <> (NEW.organization_direction_id IS NULL) THEN
  RAISE EXCEPTION 'order product and organization direction snapshots must be present together';
 END IF;
 IF NEW.product_id IS NOT NULL THEN
  SELECT * INTO product_row FROM products WHERE id=NEW.product_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'order product not found'; END IF;
  IF product_row.status<>'PUBLISHED' OR product_row.published_at IS NULL THEN
   RAISE EXCEPTION 'order requires a published product';
  END IF;
  IF product_row.organization_direction_id IS DISTINCT FROM NEW.organization_direction_id THEN
   RAISE EXCEPTION 'order product/direction snapshot mismatch';
  END IF;
  expected_product_owner_ref:=lower(product_row.owner_type)||'/'||product_row.owner_id::text;
  IF NEW.product_owner_ref IS DISTINCT FROM expected_product_owner_ref
     OR NEW.commercial_owner_ref IS DISTINCT FROM product_row.commercial_owner_ref
     OR NEW.payout_beneficiary_ref IS DISTINCT FROM product_row.revenue_beneficiary_ref
     OR NEW.author_refs IS DISTINCT FROM product_row.author_refs THEN
   RAISE EXCEPTION 'order product ownership snapshot mismatch';
  END IF;
 END IF;
 IF NEW.product_owner_ref IS NULL OR btrim(NEW.product_owner_ref)='' THEN
  RAISE EXCEPTION 'order product owner snapshot is required';
 END IF;
 IF NEW.author_refs IS NULL OR cardinality(NEW.author_refs)=0 THEN
  RAISE EXCEPTION 'order author snapshot is required';
 END IF;
 FOREACH author_ref IN ARRAY NEW.author_refs LOOP
  IF author_ref IS NULL OR btrim(author_ref)='' THEN
   RAISE EXCEPTION 'order author snapshot contains blank author';
  END IF;
 END LOOP;
 IF (SELECT count(*)<>count(DISTINCT value)
     FROM unnest(NEW.author_refs) AS author(value)) THEN
  RAISE EXCEPTION 'order author snapshot contains duplicate author';
 END IF;
 IF legal.transaction_ref<>'order/'||NEW.id::text OR
    legal.seller_or_service_provider_id<>NEW.seller_ref OR
    legal.commercial_owner_id<>NEW.commercial_owner_ref OR
    legal.payment_recipient_id<>NEW.payment_recipient_ref OR
    legal.platform_role<>NEW.platform_role OR
    legal.fiscal_responsibility_id<>NEW.fiscal_responsibility_ref OR
    legal.refund_responsibility_id<>NEW.refund_responsibility_ref OR
    legal.payout_beneficiary_id<>NEW.payout_beneficiary_ref THEN
  RAISE EXCEPTION 'order/legal snapshot role mismatch';
 END IF;

 IF NEW.academy_buyer_identity_id IS NOT NULL THEN
  IF NEW.booking_id IS NOT NULL OR NEW.product_id IS NULL OR
     NEW.currency<>'RUB' OR NEW.amount_minor<=0 THEN
   RAISE EXCEPTION 'academy checkout must use a RUB product and one buyer, without booking';
  END IF;
  -- No purchase of a draft or undeliverable course, including after
  -- an author changed the price between the quote and order commit.
  SELECT terms.price_minor,terms.currency INTO academy_price,academy_currency
    FROM academy_courses course
    JOIN commerce_product_access_terms terms ON terms.product_id=course.product_id
    WHERE course.product_id=NEW.product_id AND course.state='PUBLISHED'
      AND terms.access_mode='PAID'
      AND EXISTS (SELECT 1 FROM academy_course_versions v
                  WHERE v.course_id=course.id AND v.state='PUBLISHED'
                    AND v.version_no=(SELECT max(latest.version_no)
                                      FROM academy_course_versions latest
                                      WHERE latest.course_id=course.id AND latest.state='PUBLISHED')
                    AND academy_video_learner_delivery_ready(v.id))
    FOR SHARE OF terms;
  IF NOT FOUND OR academy_price IS DISTINCT FROM NEW.amount_minor
     OR academy_currency IS DISTINCT FROM NEW.currency THEN
   RAISE EXCEPTION 'academy checkout requires current published Commerce quote and deliverable media';
  END IF;
 ELSE
  IF NEW.booking_id IS NULL OR NOT EXISTS(
    SELECT 1 FROM bookings WHERE id=NEW.booking_id
      AND state IN ('HELD','PENDING_PAYMENT','CONFIRMED')
  ) THEN
   RAISE EXCEPTION 'order requires a live booking';
  END IF;
  IF NEW.product_id IS NOT NULL AND NOT EXISTS(
    SELECT 1 FROM bookings booking JOIN booking_slots slot ON slot.id=booking.slot_id
    WHERE booking.id=NEW.booking_id AND slot.product_id=NEW.product_id
  ) THEN
   RAISE EXCEPTION 'order product must match booking slot product';
  END IF;
 END IF;
 RETURN NEW;
END;
$fn$;

-- The old store-only verification must still reject a non-store rail.
-- Bank-acquired Academy orders use the same canonical captured payment,
-- signature-verified webhook receipt, provider routing and ledger facts.
CREATE OR REPLACE FUNCTION apgic_store_transaction_verification_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE
 attempt payment_attempts%ROWTYPE;
 effect payment_effects%ROWTYPE;
 route payment_routing_decisions%ROWTYPE;
 config payment_provider_config_versions%ROWTYPE;
 acquisition_order orders%ROWTYPE;
 buyer uuid;
BEGIN
 SELECT * INTO attempt FROM payment_attempts WHERE id=NEW.payment_attempt_id;
 IF NOT FOUND OR attempt.order_id<>NEW.order_id OR attempt.state<>'SUCCEEDED' THEN
  RAISE EXCEPTION 'purchase verification requires succeeded canonical attempt';
 END IF;
 SELECT * INTO effect FROM payment_effects WHERE id=NEW.payment_effect_id;
 IF NOT FOUND OR effect.attempt_id<>NEW.payment_attempt_id
    OR effect.effect_kind<>'CAPTURED'
    OR effect.ledger_entry_id IS DISTINCT FROM NEW.ledger_entry_id THEN
  RAISE EXCEPTION 'purchase verification requires captured payment effect and ledger';
 END IF;
 SELECT * INTO route FROM payment_routing_decisions WHERE id=attempt.routing_decision_id;
 SELECT * INTO config FROM payment_provider_config_versions WHERE id=route.provider_config_id;
 IF NOT FOUND OR config.provider_instance_id<>NEW.provider_instance_id THEN
  RAISE EXCEPTION 'purchase verification provider does not match payment routing';
 END IF;
 SELECT * INTO acquisition_order FROM orders WHERE id=NEW.order_id;
 IF NOT FOUND THEN RAISE EXCEPTION 'purchase verification order not found'; END IF;
 IF acquisition_order.academy_buyer_identity_id IS NOT NULL THEN
  IF acquisition_order.booking_id IS NOT NULL OR
     route.selected_rail_code<>'APGIC_PAYMENT_PROVIDER' OR
     route.selected_method_code<>'BANK_CARD' OR
     acquisition_order.product_id::text IS DISTINCT FROM NEW.product_ref OR
     NEW.entitlement_kind<>'ONE_TIME' THEN
   RAISE EXCEPTION 'Academy bank purchase requires card rail and matching product';
  END IF;
  buyer:=acquisition_order.academy_buyer_identity_id;
 ELSE
  IF route.selected_rail_code<>'STORE_BILLING' THEN
   RAISE EXCEPTION 'store verification requires STORE_BILLING rail';
  END IF;
  SELECT booking.client_identity_id INTO buyer
  FROM bookings booking WHERE booking.id=acquisition_order.booking_id;
 END IF;
 IF buyer IS DISTINCT FROM NEW.identity_id THEN
  RAISE EXCEPTION 'purchase verification buyer does not match order';
 END IF;
 IF NOT EXISTS (
  SELECT 1 FROM ledger_entries
  WHERE id=NEW.ledger_entry_id AND provider_evidence_ref=effect.provider_evidence_ref
 ) THEN
  RAISE EXCEPTION 'purchase verification ledger evidence mismatch';
 END IF;
 RETURN NEW;
END;
$fn$;
