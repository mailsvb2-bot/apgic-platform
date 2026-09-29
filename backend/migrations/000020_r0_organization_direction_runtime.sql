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
