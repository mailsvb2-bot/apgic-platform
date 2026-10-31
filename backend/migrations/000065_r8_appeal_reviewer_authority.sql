-- Independent Academy appeal review is an explicitly provisioned capability.
-- Provisioning requires controlled database access; there is deliberately no
-- self-service HTTP route and no reuse of Finance administrator permissions.
CREATE TABLE academy_appeal_reviewer_grants (
  id uuid PRIMARY KEY,
  reviewer_identity_id uuid NOT NULL REFERENCES identities(id),
  approval_evidence_ref text NOT NULL CHECK (btrim(approval_evidence_ref)<>''),
  granted_by text NOT NULL CHECK (btrim(granted_by)<>''),
  granted_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz,
  CHECK (revoked_at IS NULL OR revoked_at>granted_at)
);
CREATE UNIQUE INDEX academy_active_reviewer_grant
 ON academy_appeal_reviewer_grants(reviewer_identity_id) WHERE revoked_at IS NULL;
CREATE FUNCTION academy_appeal_reviewer_guard() RETURNS trigger LANGUAGE plpgsql AS $guard$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.independent_reviewer_id IS NULL OR NOT EXISTS (
    SELECT 1 FROM academy_appeal_reviewer_grants g
    WHERE g.reviewer_identity_id=NEW.independent_reviewer_id AND g.revoked_at IS NULL
  ) THEN
   RAISE EXCEPTION 'academy appeal reviewer lacks active independent grant';
  END IF;
 END IF;
 RETURN NEW;
END; $guard$;
CREATE TRIGGER academy_appeal_independent_grant
 BEFORE UPDATE ON academy_assessment_appeals
 FOR EACH ROW EXECUTE FUNCTION academy_appeal_reviewer_guard();
