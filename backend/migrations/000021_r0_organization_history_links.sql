BEGIN;

ALTER TABLE products
  ADD COLUMN organization_direction_id uuid REFERENCES organization_directions(id);

ALTER TABLE booking_slots
  ADD COLUMN product_id uuid REFERENCES products(id);

ALTER TABLE orders
  ADD COLUMN product_id uuid REFERENCES products(id),
  ADD COLUMN organization_direction_id uuid REFERENCES organization_directions(id);

CREATE OR REPLACE FUNCTION apgic_validate_product_direction()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  direction_org uuid;
BEGIN
  IF NEW.organization_direction_id IS NULL THEN
    RETURN NEW;
  END IF;

  IF NEW.owner_type <> 'ORGANIZATION' THEN
    RAISE EXCEPTION 'organization direction requires organization-owned product';
  END IF;

  SELECT organization_id
    INTO direction_org
    FROM organization_directions
   WHERE id = NEW.organization_direction_id;

  IF NOT FOUND OR direction_org <> NEW.owner_id THEN
    RAISE EXCEPTION 'product organization direction must belong to product owner organization';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER products_direction_owner_guard
BEFORE INSERT OR UPDATE OF owner_type, owner_id, organization_direction_id ON products
FOR EACH ROW EXECUTE FUNCTION apgic_validate_product_direction();


CREATE OR REPLACE FUNCTION apgic_order_snapshot_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  legal legal_transaction_snapshots%ROWTYPE;
  product_row products%ROWTYPE;
  author_ref text;
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

    IF NOT FOUND OR product_row.organization_direction_id IS DISTINCT FROM NEW.organization_direction_id THEN
      RAISE EXCEPTION 'order product/direction snapshot mismatch';
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

  IF NOT EXISTS (
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

CREATE OR REPLACE FUNCTION apgic_store_transaction_verification_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  attempt payment_attempts%ROWTYPE;
  effect payment_effects%ROWTYPE;
  route payment_routing_decisions%ROWTYPE;
  config payment_provider_config_versions%ROWTYPE;
  booking_client uuid;
  order_product uuid;
BEGIN
  SELECT *
    INTO attempt
    FROM payment_attempts
   WHERE id = NEW.payment_attempt_id;

  IF NOT FOUND OR attempt.order_id <> NEW.order_id OR attempt.state <> 'SUCCEEDED' THEN
    RAISE EXCEPTION 'store verification requires succeeded canonical payment attempt';
  END IF;

  SELECT *
    INTO effect
    FROM payment_effects
   WHERE id = NEW.payment_effect_id;

  IF NOT FOUND
    OR effect.attempt_id <> NEW.payment_attempt_id
    OR effect.effect_kind <> 'CAPTURED'
    OR effect.ledger_entry_id IS DISTINCT FROM NEW.ledger_entry_id THEN
    RAISE EXCEPTION 'store verification requires captured payment effect and ledger basis';
  END IF;

  SELECT *
    INTO route
    FROM payment_routing_decisions
   WHERE id = attempt.routing_decision_id;

  IF NOT FOUND OR route.selected_rail_code <> 'STORE_BILLING' THEN
    RAISE EXCEPTION 'store verification requires STORE_BILLING payment rail';
  END IF;

  SELECT *
    INTO config
    FROM payment_provider_config_versions
   WHERE id = route.provider_config_id;

  IF NOT FOUND OR config.provider_instance_id <> NEW.provider_instance_id THEN
    RAISE EXCEPTION 'store verification provider must match routed payment provider';
  END IF;

  SELECT booking.client_identity_id, order_row.product_id
    INTO booking_client, order_product
    FROM orders order_row
    JOIN bookings booking ON booking.id = order_row.booking_id
   WHERE order_row.id = NEW.order_id;

  IF booking_client IS DISTINCT FROM NEW.identity_id THEN
    RAISE EXCEPTION 'store verification identity must match canonical order booking client';
  END IF;

  IF order_product IS NOT NULL AND NEW.product_ref <> order_product::text THEN
    RAISE EXCEPTION 'store verification product must match immutable order product';
  END IF;

  IF NOT EXISTS (
    SELECT 1
      FROM ledger_entries
     WHERE id = NEW.ledger_entry_id
       AND provider_evidence_ref = effect.provider_evidence_ref
  ) THEN
    RAISE EXCEPTION 'store verification ledger evidence mismatch';
  END IF;

  RETURN NEW;
END;
$$;

COMMIT;
