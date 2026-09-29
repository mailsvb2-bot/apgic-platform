BEGIN;

ALTER TABLE products
  ADD COLUMN name text,
  ADD COLUMN status text NOT NULL DEFAULT 'DRAFT',
  ADD COLUMN published_at timestamptz,
  ADD CONSTRAINT products_name_nonblank
    CHECK (name IS NULL OR btrim(name) <> ''),
  ADD CONSTRAINT products_status_check
    CHECK (status IN ('DRAFT','PUBLISHED')),
  ADD CONSTRAINT products_publication_timestamp_check
    CHECK (
      (status = 'DRAFT' AND published_at IS NULL)
      OR
      (status = 'PUBLISHED' AND published_at IS NOT NULL)
    );

CREATE OR REPLACE FUNCTION apgic_validate_product_publication()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  direction_org uuid;
  direction_status text;
BEGIN
  IF NEW.status <> 'PUBLISHED' THEN
    RETURN NEW;
  END IF;

  IF NEW.name IS NULL OR btrim(NEW.name) = '' THEN
    RAISE EXCEPTION 'published product requires name';
  END IF;

  IF NEW.commercial_owner_ref IS NULL OR btrim(NEW.commercial_owner_ref) = ''
     OR NEW.revenue_beneficiary_ref IS NULL OR btrim(NEW.revenue_beneficiary_ref) = ''
     OR NEW.author_refs IS NULL OR cardinality(NEW.author_refs) = 0 THEN
    RAISE EXCEPTION 'published product requires explicit commercial owner, authors and revenue beneficiary';
  END IF;

  IF EXISTS (
    SELECT 1 FROM unnest(NEW.author_refs) AS author(value)
     WHERE value IS NULL OR btrim(value) = ''
  ) THEN
    RAISE EXCEPTION 'published product author refs must be nonblank';
  END IF;

  IF (
    SELECT count(*) <> count(DISTINCT value)
      FROM unnest(NEW.author_refs) AS author(value)
  ) THEN
    RAISE EXCEPTION 'published product author refs must be unique';
  END IF;

  IF NEW.owner_type = 'ORGANIZATION' THEN
    IF NEW.organization_direction_id IS NULL THEN
      RAISE EXCEPTION 'published organization product requires organization direction';
    END IF;

    SELECT organization_id, status
      INTO direction_org, direction_status
      FROM organization_directions
     WHERE id = NEW.organization_direction_id;

    IF NOT FOUND OR direction_org <> NEW.owner_id OR direction_status <> 'ACTIVE' THEN
      RAISE EXCEPTION 'published organization product requires active direction owned by product organization';
    END IF;
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER products_publication_guard
BEFORE INSERT OR UPDATE OF
  name, status, published_at, owner_type, owner_id, commercial_owner_ref,
  revenue_beneficiary_ref, author_refs, organization_direction_id
ON products
FOR EACH ROW EXECUTE FUNCTION apgic_validate_product_publication();

COMMIT;
