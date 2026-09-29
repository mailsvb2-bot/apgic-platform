-- APGIC-PROD-001: new orders may only capture a previously published product.
-- Historical orders are immutable evidence and are not rewritten by this migration.
-- Transaction boundaries are owned by deploy/staging/apply-staging-migrations.sh.

CREATE OR REPLACE FUNCTION apgic_order_snapshot_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  legal legal_transaction_snapshots%ROWTYPE;
  product_row products%ROWTYPE;
  author_ref text;
  expected_product_owner_ref text;
BEGIN
  SELECT *
    INTO legal
    FROM legal_transaction_snapshots
   WHERE id = NEW.legal_snapshot_id;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'order legal snapshot not found';
  END IF;

  IF (NEW.product_id IS NULL) <> (NEW.organization_direction_id IS NULL) THEN
    RAISE EXCEPTION 'order product and organization direction snapshots must be present together';
  END IF;

  IF NEW.product_id IS NOT NULL THEN
    SELECT *
      INTO product_row
      FROM products
     WHERE id = NEW.product_id;

    IF NOT FOUND THEN
      RAISE EXCEPTION 'order product not found';
    END IF;
    IF product_row.status <> 'PUBLISHED' OR product_row.published_at IS NULL THEN
      RAISE EXCEPTION 'order requires a published product';
    END IF;
    IF product_row.organization_direction_id IS DISTINCT FROM NEW.organization_direction_id THEN
      RAISE EXCEPTION 'order product/direction snapshot mismatch';
    END IF;

    expected_product_owner_ref :=
      lower(product_row.owner_type) || '/' || product_row.owner_id::text;

    IF NEW.product_owner_ref IS DISTINCT FROM expected_product_owner_ref
       OR NEW.commercial_owner_ref IS DISTINCT FROM product_row.commercial_owner_ref
       OR NEW.payout_beneficiary_ref IS DISTINCT FROM product_row.revenue_beneficiary_ref
       OR NEW.author_refs IS DISTINCT FROM product_row.author_refs THEN
      RAISE EXCEPTION 'order product ownership snapshot mismatch';
    END IF;
  END IF;

  IF NEW.product_owner_ref IS NULL OR btrim(NEW.product_owner_ref) = '' THEN
    RAISE EXCEPTION 'order product owner snapshot is required';
  END IF;
  IF NEW.author_refs IS NULL OR cardinality(NEW.author_refs) = 0 THEN
    RAISE EXCEPTION 'order author snapshot is required';
  END IF;
  FOREACH author_ref IN ARRAY NEW.author_refs LOOP
    IF author_ref IS NULL OR btrim(author_ref) = '' THEN
      RAISE EXCEPTION 'order author snapshot contains blank author';
    END IF;
  END LOOP;
  IF (
    SELECT count(*) <> count(DISTINCT value)
      FROM unnest(NEW.author_refs) AS author(value)
  ) THEN
    RAISE EXCEPTION 'order author snapshot contains duplicate author';
  END IF;

  IF legal.transaction_ref <> 'order/' || NEW.id::text OR
     legal.seller_or_service_provider_id <> NEW.seller_ref OR
     legal.commercial_owner_id <> NEW.commercial_owner_ref OR
     legal.payment_recipient_id <> NEW.payment_recipient_ref OR
     legal.platform_role <> NEW.platform_role OR
     legal.fiscal_responsibility_id <> NEW.fiscal_responsibility_ref OR
     legal.refund_responsibility_id <> NEW.refund_responsibility_ref OR
     legal.payout_beneficiary_id <> NEW.payout_beneficiary_ref THEN
    RAISE EXCEPTION 'order/legal snapshot role mismatch';
  END IF;

  IF NOT EXISTS (
    SELECT 1
      FROM bookings
     WHERE id = NEW.booking_id
       AND state IN ('HELD','PENDING_PAYMENT','CONFIRMED')
  ) THEN
    RAISE EXCEPTION 'order requires a live booking';
  END IF;

  IF NEW.product_id IS NOT NULL AND NOT EXISTS (
    SELECT 1
      FROM bookings booking
      JOIN booking_slots slot ON slot.id = booking.slot_id
     WHERE booking.id = NEW.booking_id
       AND slot.product_id = NEW.product_id
  ) THEN
    RAISE EXCEPTION 'order product must match booking slot product';
  END IF;

  RETURN NEW;
END;
$$;
