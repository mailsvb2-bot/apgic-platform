BEGIN;

ALTER TABLE orders
  ADD COLUMN product_owner_ref text,
  ADD COLUMN author_refs text[];

ALTER TABLE orders
  ADD CONSTRAINT orders_product_owner_ref_nonblank
    CHECK (product_owner_ref IS NULL OR btrim(product_owner_ref) <> ''),
  ADD CONSTRAINT orders_author_refs_nonempty
    CHECK (author_refs IS NULL OR cardinality(author_refs) > 0);

CREATE OR REPLACE FUNCTION apgic_order_snapshot_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  legal legal_transaction_snapshots%ROWTYPE;
  author_ref text;
BEGIN
  SELECT *
  INTO legal
  FROM legal_transaction_snapshots
  WHERE id = NEW.legal_snapshot_id;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'order legal snapshot not found';
  END IF;

  IF NEW.product_owner_ref IS NULL OR btrim(NEW.product_owner_ref) = '' THEN
    RAISE EXCEPTION 'order product owner snapshot is required';
  END IF;
  IF NEW.author_refs IS NULL OR cardinality(NEW.author_refs) = 0 THEN
    RAISE EXCEPTION 'order author snapshot is required';
  END IF;
  FOREACH author_ref IN ARRAY NEW.author_refs LOOP
    IF btrim(author_ref) = '' THEN
      RAISE EXCEPTION 'order author snapshot contains blank author';
    END IF;
  END LOOP;

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

  RETURN NEW;
END;
$$;

COMMIT;
