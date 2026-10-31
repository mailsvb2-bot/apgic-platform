-- Canonical Academy database enforcement for schedules and assessment rules.
CREATE FUNCTION academy_assessment_submission_guard() RETURNS trigger LANGUAGE plpgsql AS $guard$
DECLARE active_enrollment boolean; lesson_version uuid; max_allowed integer;
        cutoff timestamptz; released timestamptz; due timestamptz;
        existing_count integer; existing_block boolean;
BEGIN
 SELECT e.state='ACTIVE' AND academy_entitlement_is_active(e.entitlement_id),e.version_id
 INTO active_enrollment,lesson_version FROM academy_enrollments e
 WHERE e.id=NEW.enrollment_id FOR UPDATE;
 IF NOT FOUND OR NOT coalesce(active_enrollment,false) THEN
   RAISE EXCEPTION 'academy assessment requires active purchased enrollment';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM academy_assessments a
   JOIN academy_lessons l ON l.id=a.lesson_id JOIN academy_modules m ON m.id=l.module_id
   WHERE a.lesson_id=NEW.lesson_id AND m.version_id=lesson_version) THEN
   RAISE EXCEPTION 'academy attempt lesson/version mismatch';
 END IF;
 SELECT COALESCE(l.max_attempts,1), l.submission_deadline
 INTO max_allowed,cutoff FROM academy_assessment_limits l WHERE l.lesson_id=NEW.lesson_id;
 IF NOT FOUND THEN max_allowed := 1; cutoff := NULL; END IF;
 SELECT s.release_at,s.deadline_at INTO released,due
 FROM academy_lesson_schedules s WHERE s.lesson_id=NEW.lesson_id;
 IF (released IS NOT NULL AND now()<released) OR
    (due IS NOT NULL AND now()>=due) OR
    (cutoff IS NOT NULL AND now()>=cutoff) THEN
   RAISE EXCEPTION 'academy assessment is outside the authorized window';
 END IF;
 SELECT count(*),coalesce(bool_or(result IN ('PENDING','PASSED')),false)
 INTO existing_count,existing_block
 FROM academy_attempts WHERE enrollment_id=NEW.enrollment_id AND lesson_id=NEW.lesson_id;
 IF existing_count>=max_allowed OR existing_block OR NEW.attempt_no <> existing_count+1 THEN
   RAISE EXCEPTION 'academy assessment attempt limit or state violated';
 END IF;
 RETURN NEW;
END; $guard$;
CREATE TRIGGER academy_assessment_submission_guard BEFORE INSERT ON academy_attempts
FOR EACH ROW EXECUTE FUNCTION academy_assessment_submission_guard();

CREATE FUNCTION academy_scheduled_progress_guard() RETURNS trigger LANGUAGE plpgsql AS $guard$
DECLARE released timestamptz; due timestamptz; active_enrollment boolean;
BEGIN
 SELECT release_at,deadline_at INTO released,due FROM academy_lesson_schedules
 WHERE lesson_id=NEW.lesson_id;
 IF NOT FOUND THEN RETURN NEW; END IF;
 SELECT e.state='ACTIVE' AND academy_entitlement_is_active(e.entitlement_id)
 INTO active_enrollment FROM academy_enrollments e WHERE e.id=NEW.enrollment_id;
 IF NOT coalesce(active_enrollment,false) OR
    (released IS NOT NULL AND now()<released) OR
    (due IS NOT NULL AND now()>=due) THEN
   RAISE EXCEPTION 'academy progress requires active entitlement and released lesson';
 END IF;
 RETURN NEW;
END; $guard$;
CREATE TRIGGER academy_scheduled_progress_guard BEFORE INSERT ON academy_progress
FOR EACH ROW EXECUTE FUNCTION academy_scheduled_progress_guard();

CREATE FUNCTION academy_appeal_guard() RETURNS trigger LANGUAGE plpgsql AS $guard$
DECLARE author_id uuid; learner_id uuid; attempt_result text;
BEGIN
 SELECT c.author_id,e.identity_id,a.result INTO author_id,learner_id,attempt_result
 FROM academy_attempts a JOIN academy_enrollments e ON e.id=a.enrollment_id
 JOIN academy_course_versions v ON v.id=e.version_id
 JOIN academy_courses c ON c.id=v.course_id WHERE a.id=NEW.attempt_id FOR UPDATE OF a;
 IF NOT FOUND OR attempt_result NOT IN ('PASSED','FAILED') THEN
   RAISE EXCEPTION 'academy appeal requires a human-graded attempt';
 END IF;
 IF TG_OP='INSERT' THEN
   IF NEW.submitted_by IS DISTINCT FROM learner_id OR NEW.state<>'SUBMITTED' OR
      NEW.independent_reviewer_id IS NOT NULL OR NEW.review_evidence_ref IS NOT NULL OR
      NEW.resolved_at IS NOT NULL THEN
     RAISE EXCEPTION 'academy appeal must be created by the learner';
   END IF;
 ELSE
   IF NEW.attempt_id IS DISTINCT FROM OLD.attempt_id OR
      NEW.submitted_by IS DISTINCT FROM OLD.submitted_by OR
      NEW.reason IS DISTINCT FROM OLD.reason OR NEW.submitted_at IS DISTINCT FROM OLD.submitted_at THEN
     RAISE EXCEPTION 'academy appeal immutable facts changed';
   END IF;
   IF (OLD.state='SUBMITTED' AND NEW.state<>'UNDER_REVIEW') OR
      (OLD.state='UNDER_REVIEW' AND NEW.state NOT IN ('UPHELD','REJECTED')) OR
      OLD.state IN ('UPHELD','REJECTED') THEN
     RAISE EXCEPTION 'academy appeal invalid state transition';
   END IF;
   IF NEW.independent_reviewer_id IS NULL OR
      NEW.independent_reviewer_id=learner_id OR NEW.independent_reviewer_id=author_id OR
      (OLD.independent_reviewer_id IS NOT NULL AND
       NEW.independent_reviewer_id IS DISTINCT FROM OLD.independent_reviewer_id) THEN
     RAISE EXCEPTION 'academy appeal requires independent reviewer';
   END IF;
   IF NEW.state IN ('UPHELD','REJECTED') AND
      (coalesce(btrim(NEW.review_evidence_ref),'')='' OR NEW.resolved_at IS NULL) THEN
     RAISE EXCEPTION 'academy appeal resolution lacks review evidence';
   END IF;
 END IF;
 RETURN NEW;
END; $guard$;
CREATE TRIGGER academy_appeal_insert_guard BEFORE INSERT ON academy_assessment_appeals
FOR EACH ROW EXECUTE FUNCTION academy_appeal_guard();
CREATE TRIGGER academy_appeal_update_guard BEFORE UPDATE ON academy_assessment_appeals
FOR EACH ROW EXECUTE FUNCTION academy_appeal_guard();
CREATE TRIGGER academy_appeal_no_delete BEFORE DELETE ON academy_assessment_appeals
FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

CREATE FUNCTION academy_journey_draft_only_guard() RETURNS trigger LANGUAGE plpgsql AS $guard$
DECLARE current_version uuid; current_state text;
BEGIN
 IF TG_OP='UPDATE' AND NEW.lesson_id IS DISTINCT FROM OLD.lesson_id THEN
  RAISE EXCEPTION 'academy lesson policy cannot be rebound to another lesson';
 END IF;
 IF TG_TABLE_NAME='academy_lesson_schedules' THEN
  SELECT m.version_id INTO current_version FROM academy_lessons l
    JOIN academy_modules m ON m.id=l.module_id WHERE l.id=NEW.lesson_id;
 ELSE
  SELECT m.version_id INTO current_version FROM academy_assessments a
    JOIN academy_lessons l ON l.id=a.lesson_id
    JOIN academy_modules m ON m.id=l.module_id WHERE a.lesson_id=NEW.lesson_id;
 END IF;
 SELECT state INTO current_state FROM academy_course_versions WHERE id=current_version FOR UPDATE;
 IF current_state IS DISTINCT FROM 'DRAFT' THEN
  RAISE EXCEPTION 'academy published lesson policies are immutable';
 END IF;
 RETURN NEW;
END; $guard$;
CREATE TRIGGER academy_schedule_draft_only BEFORE INSERT OR UPDATE ON academy_lesson_schedules
FOR EACH ROW EXECUTE FUNCTION academy_journey_draft_only_guard();
CREATE TRIGGER academy_attempt_limit_draft_only BEFORE INSERT OR UPDATE ON academy_assessment_limits
FOR EACH ROW EXECUTE FUNCTION academy_journey_draft_only_guard();
CREATE TRIGGER academy_schedule_no_delete BEFORE DELETE ON academy_lesson_schedules
FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();
CREATE TRIGGER academy_attempt_limit_no_delete BEFORE DELETE ON academy_assessment_limits
FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();
