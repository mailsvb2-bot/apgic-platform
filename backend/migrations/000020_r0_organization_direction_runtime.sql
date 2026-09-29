BEGIN;

CREATE TABLE organization_ownerships (
  organization_id uuid NOT NULL REFERENCES organizations(id),
  identity_id uuid NOT NULL REFERENCES identities(id),
  status text NOT NULL CHECK (status IN ('ACTIVE','REVOKED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz,
  PRIMARY KEY (organization_id, identity_id),
  CHECK (
    (status = 'ACTIVE' AND revoked_at IS NULL) OR
    (status = 'REVOKED' AND revoked_at IS NOT NULL)
  )
);

CREATE INDEX organization_ownerships_identity_idx
  ON organization_ownerships (identity_id, status);

CREATE OR REPLACE FUNCTION apgic_organization_owner_membership_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.status = 'ACTIVE' AND NOT EXISTS (
    SELECT 1
      FROM organization_memberships
     WHERE organization_id = NEW.organization_id
       AND identity_id = NEW.identity_id
       AND status = 'ACTIVE'
  ) THEN
    RAISE EXCEPTION 'active organization owner requires active membership';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER organization_ownerships_membership_guard
BEFORE INSERT OR UPDATE ON organization_ownerships
FOR EACH ROW EXECUTE FUNCTION apgic_organization_owner_membership_guard();

CREATE OR REPLACE FUNCTION apgic_organization_membership_owner_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  active_owner boolean;
BEGIN
  SELECT EXISTS (
    SELECT 1
      FROM organization_ownerships
     WHERE organization_id = OLD.organization_id
       AND identity_id = OLD.identity_id
       AND status = 'ACTIVE'
  ) INTO active_owner;

  IF TG_OP = 'DELETE' THEN
    IF OLD.status = 'ACTIVE' AND active_owner THEN
      RAISE EXCEPTION 'revoke organization ownership before deleting active owner membership';
    END IF;
    RETURN OLD;
  END IF;

  IF OLD.status = 'ACTIVE' AND NEW.status <> 'ACTIVE' AND active_owner THEN
    RAISE EXCEPTION 'revoke organization ownership before deactivating active owner membership';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER organization_memberships_owner_guard
BEFORE UPDATE OR DELETE ON organization_memberships
FOR EACH ROW EXECUTE FUNCTION apgic_organization_membership_owner_guard();

CREATE OR REPLACE FUNCTION apgic_organization_ownership_delete_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'organization ownership hard delete is forbidden; revoke it instead';
END;
$$;

CREATE TRIGGER organization_ownerships_no_delete
BEFORE DELETE ON organization_ownerships
FOR EACH ROW EXECUTE FUNCTION apgic_organization_ownership_delete_guard();

ALTER TABLE organization_directions
  ADD COLUMN direction_type text NOT NULL DEFAULT 'GENERAL',
  ADD CONSTRAINT organization_directions_type_nonblank
    CHECK (btrim(direction_type) <> '');

CREATE OR REPLACE FUNCTION apgic_organization_direction_delete_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'organization direction hard delete is forbidden; archive it instead';
END;
$$;

CREATE TRIGGER organization_directions_no_delete
BEFORE DELETE ON organization_directions
FOR EACH ROW EXECUTE FUNCTION apgic_organization_direction_delete_guard();

COMMIT;
