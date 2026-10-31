-- R8 hardening for evidence added in 000061. Run only after 000060 and 000061.
CREATE FUNCTION academy_cohort_membership_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE cohort_version uuid; current_capacity integer; enrolled_version uuid; enrolled_state text; current_count bigint; learner uuid; entitlement_active boolean;
BEGIN
  PERFORM pg_advisory_xact_lock(hashtextextended(NEW.cohort_id::text, 82748));
  SELECT version_id, capacity INTO cohort_version, current_capacity
    FROM academy_cohorts WHERE id=NEW.cohort_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'academy cohort not found'; END IF;
  SELECT version_id, state, identity_id, academy_entitlement_is_active(entitlement_id)
    INTO enrolled_version, enrolled_state, learner, entitlement_active
    FROM academy_enrollments WHERE id=NEW.enrollment_id FOR UPDATE;
  IF NOT FOUND OR enrolled_version IS DISTINCT FROM cohort_version OR enrolled_state <> 'ACTIVE' OR entitlement_active IS DISTINCT FROM TRUE THEN
    RAISE EXCEPTION 'academy cohort enrollment version/state mismatch';
  END IF;
  IF EXISTS (SELECT 1 FROM academy_cohort_memberships WHERE cohort_id=NEW.cohort_id AND enrollment_id=NEW.enrollment_id) THEN
    RETURN NEW; -- ON CONFLICT DO NOTHING remains idempotent after the trigger.
  END IF;
  SELECT count(*) INTO current_count FROM academy_cohort_memberships WHERE cohort_id=NEW.cohort_id;
  IF current_capacity IS NULL OR current_count >= current_capacity THEN RAISE EXCEPTION 'academy cohort capacity exceeded'; END IF;
  IF EXISTS (SELECT 1 FROM academy_cohorts
    WHERE id=NEW.cohort_id AND (enrollment_closes_at <= now() OR starts_at <= now())) THEN
    RAISE EXCEPTION 'academy cohort enrollment is closed';
  END IF;
  IF EXISTS (
    SELECT 1 FROM academy_prerequisites p
    WHERE p.version_id=cohort_version AND NOT EXISTS (
      SELECT 1 FROM academy_enrollments prior
      WHERE prior.identity_id=learner AND prior.version_id=p.prerequisite_version_id
        AND prior.state='COMPLETED'
    )
  ) THEN RAISE EXCEPTION 'academy cohort prerequisites not completed'; END IF;
  RETURN NEW;
END; $$;
CREATE TRIGGER academy_cohort_membership_insert_guard
  BEFORE INSERT ON academy_cohort_memberships FOR EACH ROW
  EXECUTE FUNCTION academy_cohort_membership_guard();
CREATE TRIGGER academy_cohort_membership_no_mutation
  BEFORE UPDATE OR DELETE ON academy_cohort_memberships FOR EACH ROW
  EXECUTE FUNCTION apgic_reject_mutation();

CREATE FUNCTION academy_media_fact_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE enrollment_version uuid; enrollment_state text; lesson_version uuid; lesson_kind text;
BEGIN
  SELECT version_id, state INTO enrollment_version, enrollment_state
    FROM academy_enrollments WHERE id=NEW.enrollment_id FOR UPDATE;
  SELECT m.version_id, l.kind INTO lesson_version, lesson_kind
    FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id WHERE l.id=NEW.lesson_id;
  IF enrollment_version IS NULL OR lesson_version IS NULL
    OR enrollment_version IS DISTINCT FROM lesson_version
    OR lesson_kind <> 'VIDEO' OR enrollment_state <> 'ACTIVE'
    OR NOT EXISTS (
      SELECT 1 FROM academy_enrollments e
      WHERE e.id=NEW.enrollment_id AND academy_entitlement_is_active(e.entitlement_id)) THEN
    RAISE EXCEPTION 'academy playback evidence does not match active enrollment and video lesson';
  END IF;
  IF btrim(NEW.provider_event_id)='' OR btrim(NEW.evidence_ref)='' THEN
    RAISE EXCEPTION 'academy playback requires provider evidence reference';
  END IF;
  RETURN NEW;
END; $$;
CREATE TRIGGER academy_media_fact_insert_guard BEFORE INSERT ON academy_media_playback_facts
  FOR EACH ROW EXECUTE FUNCTION academy_media_fact_guard();
CREATE TRIGGER academy_media_fact_no_mutation BEFORE UPDATE OR DELETE ON academy_media_playback_facts
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE FUNCTION academy_certificate_revocation_apply() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE old_state text;
BEGIN
  SELECT state INTO old_state FROM academy_certificates WHERE id=NEW.certificate_id FOR UPDATE;
  IF old_state IS DISTINCT FROM 'ACTIVE' THEN
    RAISE EXCEPTION 'academy certificate is not active';
  END IF;
  IF btrim(NEW.step_up_audit_ref)='' OR btrim(NEW.evidence_ref)='' THEN
    RAISE EXCEPTION 'academy certificate revocation requires audit evidence';
  END IF;
  UPDATE academy_certificates
    SET state='REVOKED',revoked_at=NEW.decided_at,revocation_reason=NEW.reason_code
    WHERE id=NEW.certificate_id;
  RETURN NEW;
END; $$;
CREATE TRIGGER academy_certificate_revocation_apply BEFORE INSERT ON academy_cert_revocation_decisions
  FOR EACH ROW EXECUTE FUNCTION academy_certificate_revocation_apply();
CREATE TRIGGER academy_certificate_revocation_immutable BEFORE UPDATE OR DELETE ON academy_cert_revocation_decisions
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER academy_rights_verification_immutable BEFORE UPDATE OR DELETE ON academy_rights_verification_facts
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
