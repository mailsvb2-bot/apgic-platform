-- R8 direct Academy purchases reuse the canonical Commerce orders, payment
-- attempts, provider routing, receipts, capture effects, ledger and verified
-- Entitlements. A course sale is NOT a booking and must never create one.
-- This migration changes no pre-existing order or store-billing evidence.
-- Production payment acceptance remains disabled until the canonical writer
-- and authenticated PSP webhook are implemented and live-evidenced.

ALTER TABLE orders ALTER COLUMN booking_id DROP NOT NULL;
ALTER TABLE orders
 ADD COLUMN direct_buyer_identity_id uuid REFERENCES identities(id),
 ADD COLUMN direct_checkout_idempotency_key text,
 ADD CONSTRAINT orders_booking_xor_direct_buyer CHECK (
   (booking_id IS NOT NULL AND direct_buyer_identity_id IS NULL
    AND direct_checkout_idempotency_key IS NULL)
   OR (booking_id IS NULL AND direct_buyer_identity_id IS NOT NULL
    AND direct_checkout_idempotency_key IS NOT NULL
    AND length(direct_checkout_idempotency_key) BETWEEN 8 AND 128
    AND direct_checkout_idempotency_key ~ '^[A-Za-z0-9._~-]{8,128}$')
 );
CREATE UNIQUE INDEX orders_direct_checkout_idempotency
 ON orders(direct_buyer_identity_id,direct_checkout_idempotency_key)
 WHERE booking_id IS NULL;

-- ONE_TIME paid Academy access must not create parallel bank charges under
-- different idempotency keys from the same buyer for the same product.
CREATE UNIQUE INDEX orders_direct_once_per_buyer_product
 ON orders(direct_buyer_identity_id,product_id)
 WHERE booking_id IS NULL;

-- Provider PaymentId must never be bound to two different bank attempts.
-- Legacy provider references retain their existing semantics.
CREATE UNIQUE INDEX payment_attempts_tbank_reference_unique
 ON payment_attempts(provider_reference)
 WHERE provider_reference LIKE 'tbank/%';

CREATE OR REPLACE FUNCTION apgic_order_snapshot_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
 legal legal_transaction_snapshots%ROWTYPE;
 product_row products%ROWTYPE;
 author_ref text;
 expected_product_owner_ref text;
 current_price bigint;
 current_revision bigint;
 course_id uuid;
BEGIN
 SELECT * INTO legal FROM legal_transaction_snapshots
 WHERE id=NEW.legal_snapshot_id;
 IF NOT FOUND THEN RAISE EXCEPTION 'order legal snapshot not found'; END IF;

 IF NEW.booking_id IS NOT NULL AND
    ((NEW.product_id IS NULL) <> (NEW.organization_direction_id IS NULL))
 THEN RAISE EXCEPTION 'order product and organization direction snapshots must be present together'; END IF;
 IF NEW.product_id IS NULL AND NEW.organization_direction_id IS NOT NULL
 THEN RAISE EXCEPTION 'order organization direction requires product'; END IF;

 IF NEW.product_id IS NOT NULL THEN
   SELECT * INTO product_row FROM products WHERE id=NEW.product_id;
   IF NOT FOUND THEN RAISE EXCEPTION 'order product not found'; END IF;
   IF product_row.status<>'PUBLISHED' OR product_row.published_at IS NULL
   THEN RAISE EXCEPTION 'order requires a published product'; END IF;
   IF product_row.organization_direction_id IS DISTINCT FROM NEW.organization_direction_id
   THEN RAISE EXCEPTION 'order product/direction snapshot mismatch'; END IF;
   expected_product_owner_ref:=lower(product_row.owner_type)||'/'||product_row.owner_id::text;
   IF NEW.product_owner_ref IS DISTINCT FROM expected_product_owner_ref
      OR NEW.commercial_owner_ref IS DISTINCT FROM product_row.commercial_owner_ref
      OR NEW.payout_beneficiary_ref IS DISTINCT FROM product_row.revenue_beneficiary_ref
      OR NEW.author_refs IS DISTINCT FROM product_row.author_refs
   THEN RAISE EXCEPTION 'order product ownership snapshot mismatch'; END IF;
 END IF;

 IF NEW.product_owner_ref IS NULL OR btrim(NEW.product_owner_ref)=''
 THEN RAISE EXCEPTION 'order product owner snapshot is required'; END IF;
 IF NEW.author_refs IS NULL OR cardinality(NEW.author_refs)=0
 THEN RAISE EXCEPTION 'order author snapshot is required'; END IF;
 FOREACH author_ref IN ARRAY NEW.author_refs LOOP
   IF author_ref IS NULL OR btrim(author_ref)=''
   THEN RAISE EXCEPTION 'order author snapshot contains blank author'; END IF;
 END LOOP;
 IF (SELECT count(*)<>count(DISTINCT value)
     FROM unnest(NEW.author_refs) AS author(value))
 THEN RAISE EXCEPTION 'order author snapshot contains duplicate author'; END IF;

 IF legal.transaction_ref<>'order/'||NEW.id::text OR
    legal.seller_or_service_provider_id<>NEW.seller_ref OR
    legal.commercial_owner_id<>NEW.commercial_owner_ref OR
    legal.payment_recipient_id<>NEW.payment_recipient_ref OR
    legal.platform_role<>NEW.platform_role OR
    legal.fiscal_responsibility_id<>NEW.fiscal_responsibility_ref OR
    legal.refund_responsibility_id<>NEW.refund_responsibility_ref OR
    legal.payout_beneficiary_id<>NEW.payout_beneficiary_ref
 THEN RAISE EXCEPTION 'order/legal snapshot role mismatch'; END IF;

 IF NEW.booking_id IS NOT NULL THEN
   IF NOT EXISTS(SELECT 1 FROM bookings
       WHERE id=NEW.booking_id AND state IN ('HELD','PENDING_PAYMENT','CONFIRMED'))
   THEN RAISE EXCEPTION 'order requires a live booking'; END IF;
   IF NEW.product_id IS NOT NULL AND NOT EXISTS(
     SELECT 1 FROM bookings booking JOIN booking_slots slot ON slot.id=booking.slot_id
     WHERE booking.id=NEW.booking_id AND slot.product_id=NEW.product_id)
   THEN RAISE EXCEPTION 'order product must match booking slot product'; END IF;
 ELSE
   -- Direct purchase must never be generated from an arbitrary product/price.
   IF NEW.product_id IS NULL OR NEW.direct_buyer_identity_id IS NULL
      OR NEW.currency<>'RUB'
   THEN RAISE EXCEPTION 'direct checkout requires published product, buyer and RUB'; END IF;
   SELECT course.id,terms.price_minor,terms.revision
     INTO course_id,current_price,current_revision
     FROM academy_courses course
     JOIN commerce_product_access_terms terms ON terms.product_id=course.product_id
     WHERE course.product_id=NEW.product_id AND course.state='PUBLISHED'
       AND terms.access_mode='PAID' AND terms.price_minor>0
       AND EXISTS(SELECT 1 FROM academy_course_versions v
         WHERE v.course_id=course.id AND v.state='PUBLISHED'
           AND v.version_no=(SELECT max(latest.version_no)
             FROM academy_course_versions latest
             WHERE latest.course_id=course.id AND latest.state='PUBLISHED')
           AND academy_video_learner_delivery_ready(v.id))
     FOR SHARE OF terms;
   IF course_id IS NULL OR NEW.amount_minor IS DISTINCT FROM current_price OR
      NEW.offer_ref IS DISTINCT FROM ('academy/course/'||course_id::text) OR
      NEW.price_source_ref IS DISTINCT FROM ('academy/offer/revision/'||current_revision::text)
   THEN RAISE EXCEPTION 'direct Academy order must preserve canonical published offer price and revision'; END IF;
 END IF;
 RETURN NEW;
END;
$$;

-- Bank-acquired verification must not fabricate an Apple/Google store policy.
-- Existing mobile STORE_BILLING verifications still require their snapshot.
ALTER TABLE store_transaction_verifications
 ALTER COLUMN policy_snapshot_id DROP NOT NULL;

-- Preserve the same verified store purchase, capture and ledger lineage.
-- Allow the canonical externally-routed provider only for direct orders,
-- while keeping STORE_BILLING strictly linked to the original booking.
CREATE OR REPLACE FUNCTION apgic_store_transaction_verification_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
 attempt payment_attempts%ROWTYPE;
 effect payment_effects%ROWTYPE;
 route payment_routing_decisions%ROWTYPE;
 config payment_provider_config_versions%ROWTYPE;
 order_row orders%ROWTYPE;
 booking_client uuid;
BEGIN
 SELECT * INTO attempt FROM payment_attempts WHERE id=NEW.payment_attempt_id;
 IF NOT FOUND OR attempt.order_id<>NEW.order_id OR attempt.state<>'SUCCEEDED'
 THEN RAISE EXCEPTION 'purchase verification requires succeeded canonical attempt'; END IF;
 SELECT * INTO effect FROM payment_effects WHERE id=NEW.payment_effect_id;
 IF NOT FOUND OR effect.attempt_id<>NEW.payment_attempt_id
   OR effect.effect_kind<>'CAPTURED'
   OR effect.ledger_entry_id IS DISTINCT FROM NEW.ledger_entry_id
 THEN RAISE EXCEPTION 'purchase verification requires captured payment effect and ledger'; END IF;
 SELECT * INTO route FROM payment_routing_decisions WHERE id=attempt.routing_decision_id;
 SELECT * INTO config FROM payment_provider_config_versions WHERE id=route.provider_config_id;
 SELECT * INTO order_row FROM orders WHERE id=NEW.order_id;
 IF order_row.id IS NULL OR config.provider_instance_id IS DISTINCT FROM NEW.provider_instance_id
 THEN RAISE EXCEPTION 'verified purchase provider/order mismatch'; END IF;
 IF order_row.booking_id IS NOT NULL THEN
   IF route.selected_rail_code<>'STORE_BILLING' OR NEW.policy_snapshot_id IS NULL
   THEN RAISE EXCEPTION 'booking purchase requires store billing rail and policy'; END IF;
   SELECT client_identity_id INTO booking_client FROM bookings WHERE id=order_row.booking_id;
   IF booking_client IS DISTINCT FROM NEW.identity_id
   THEN RAISE EXCEPTION 'store verification identity must match booking'; END IF;
 ELSE
   IF route.selected_rail_code<>'APGIC_PAYMENT_PROVIDER'
      OR NEW.policy_snapshot_id IS NOT NULL
      OR route.selected_method_code<>'BANK_CARD'
      OR NEW.entitlement_kind<>'ONE_TIME'
      OR NEW.identity_id IS DISTINCT FROM order_row.direct_buyer_identity_id
      OR order_row.product_id IS NULL
      OR NEW.product_ref IS DISTINCT FROM order_row.product_id::text
   THEN RAISE EXCEPTION 'direct purchase requires exact bank route, buyer and product'; END IF;
 END IF;
 IF order_row.product_id IS NOT NULL AND NEW.product_ref<>order_row.product_id::text
 THEN RAISE EXCEPTION 'verified product must match immutable order product'; END IF;
 IF NOT EXISTS(SELECT 1 FROM ledger_entries
   WHERE id=NEW.ledger_entry_id AND provider_evidence_ref=effect.provider_evidence_ref)
 THEN RAISE EXCEPTION 'verified purchase ledger evidence mismatch'; END IF;
 RETURN NEW;
END;
$$;
