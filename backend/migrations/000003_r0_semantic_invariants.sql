BEGIN;

ALTER TABLE outbox_events
  ADD COLUMN idempotency_key text NOT NULL UNIQUE;

ALTER TABLE outbox_events
  ADD CONSTRAINT outbox_delivery_evidence_check
  CHECK (
    (delivery_status = 'PENDING' AND delivered_at IS NULL)
    OR
    (delivery_status = 'DELIVERED' AND delivered_at IS NOT NULL)
  );

ALTER TABLE products
  ADD COLUMN author_refs text[] NOT NULL
  CHECK (cardinality(author_refs) > 0);

CREATE OR REPLACE FUNCTION apgic_validate_product_owner()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.owner_type = 'IDENTITY' THEN
    IF NOT EXISTS (SELECT 1 FROM identities WHERE id = NEW.owner_id) THEN
      RAISE EXCEPTION 'product owner identity does not exist';
    END IF;
  ELSIF NEW.owner_type = 'ORGANIZATION' THEN
    IF NOT EXISTS (SELECT 1 FROM organizations WHERE id = NEW.owner_id) THEN
      RAISE EXCEPTION 'product owner organization does not exist';
    END IF;
  ELSE
    RAISE EXCEPTION 'unsupported product owner type';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER products_owner_exists
BEFORE INSERT OR UPDATE OF owner_type, owner_id ON products
FOR EACH ROW EXECUTE FUNCTION apgic_validate_product_owner();

CREATE OR REPLACE FUNCTION apgic_forbid_direction_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'organization direction hard delete is forbidden; archive instead';
END;
$$;

CREATE TRIGGER organization_directions_no_delete
BEFORE DELETE ON organization_directions
FOR EACH ROW EXECUTE FUNCTION apgic_forbid_direction_delete();

COMMIT;
