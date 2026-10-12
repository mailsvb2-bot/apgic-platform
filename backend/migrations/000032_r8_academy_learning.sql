-- R8 Academy/EdTech: PostgreSQL owns every learning truth; external storage owns bytes.
BEGIN;

CREATE TABLE academy_courses (
  id uuid PRIMARY KEY,
  product_id uuid NOT NULL UNIQUE REFERENCES products(id),
  author_id uuid NOT NULL REFERENCES identities(id),
  title text NOT NULL CHECK (length(btrim(title)) BETWEEN 3 AND 200),
  description text NOT NULL DEFAULT '',
  state text NOT NULL DEFAULT 'DRAFT' CHECK (state IN ('DRAFT','PUBLISHED','ARCHIVED')),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX academy_courses_state_idx ON academy_courses(state);

CREATE TABLE academy_course_versions (
  id uuid PRIMARY KEY,
  course_id uuid NOT NULL REFERENCES academy_courses(id),
  version_no integer NOT NULL CHECK (version_no > 0),
  state text NOT NULL DEFAULT 'DRAFT' CHECK (state IN ('DRAFT','PUBLISHED','RETIRED')),
  completion_policy_version text NOT NULL DEFAULT 'academy-completion-v1',
  published_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(course_id,version_no),
  CHECK ((state = 'DRAFT' AND published_at IS NULL) OR
         (state <> 'DRAFT' AND published_at IS NOT NULL))
);
CREATE UNIQUE INDEX academy_one_draft_per_course ON academy_course_versions(course_id) WHERE state='DRAFT';

CREATE TABLE academy_modules (
  id uuid PRIMARY KEY,
  version_id uuid NOT NULL REFERENCES academy_course_versions(id),
  ordinal integer NOT NULL CHECK (ordinal>0),
  title text NOT NULL CHECK (btrim(title)<>''),
  UNIQUE(version_id,ordinal),
  UNIQUE(id,version_id)
);
CREATE TABLE academy_lessons (
  id uuid PRIMARY KEY,
  module_id uuid NOT NULL REFERENCES academy_modules(id),
  ordinal integer NOT NULL CHECK (ordinal>0),
  title text NOT NULL CHECK (btrim(title)<>''),
  kind text NOT NULL CHECK (kind IN ('READING','VIDEO','ASSESSMENT')),
  media_asset_ref text,
  body text NOT NULL DEFAULT '',
  UNIQUE(module_id,ordinal),
  CHECK (kind<>'VIDEO' OR (media_asset_ref IS NOT NULL AND btrim(media_asset_ref)<>'')),
  CHECK (media_asset_ref IS NULL OR media_asset_ref !~* '^https?://')
);

CREATE TABLE academy_assessments (
  lesson_id uuid PRIMARY KEY REFERENCES academy_lessons(id),
  passing_score integer NOT NULL CHECK(passing_score BETWEEN 1 AND 100),
  rubric_version text NOT NULL CHECK (btrim(rubric_version)<>''),
  evaluation_mode text NOT NULL DEFAULT 'HUMAN' CHECK (evaluation_mode='HUMAN')
);

-- Content and assessment rules are frozen once a course version is published.
CREATE FUNCTION academy_require_draft_version() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE v_state text;
BEGIN
  IF TG_TABLE_NAME = 'academy_modules' THEN
    SELECT state INTO v_state FROM academy_course_versions WHERE id=COALESCE(NEW.version_id,OLD.version_id);
  ELSIF TG_TABLE_NAME = 'academy_lessons' THEN
    SELECT v.state INTO v_state FROM academy_course_versions v
      JOIN academy_modules m ON m.version_id=v.id
      WHERE m.id=COALESCE(NEW.module_id,OLD.module_id);
  ELSE
    SELECT v.state INTO v_state FROM academy_course_versions v
      JOIN academy_modules m ON m.version_id=v.id
      JOIN academy_lessons l ON l.module_id=m.id
      WHERE l.id=COALESCE(NEW.lesson_id,OLD.lesson_id);
  END IF;
  IF v_state IS DISTINCT FROM 'DRAFT' THEN
    RAISE EXCEPTION 'academy version content is immutable after publication';
  END IF;
  RETURN COALESCE(NEW,OLD);
END; $$;
CREATE TRIGGER academy_module_draft_guard BEFORE INSERT OR UPDATE OR DELETE ON academy_modules
  FOR EACH ROW EXECUTE FUNCTION academy_require_draft_version();
CREATE TRIGGER academy_lesson_draft_guard BEFORE INSERT OR UPDATE OR DELETE ON academy_lessons
  FOR EACH ROW EXECUTE FUNCTION academy_require_draft_version();
CREATE TRIGGER academy_assessment_draft_guard BEFORE INSERT OR UPDATE OR DELETE ON academy_assessments
  FOR EACH ROW EXECUTE FUNCTION academy_require_draft_version();

CREATE FUNCTION academy_version_publish_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.state = 'PUBLISHED' AND NEW.state NOT IN ('PUBLISHED','RETIRED') THEN
    RAISE EXCEPTION 'published version cannot become draft';
  END IF;
  IF OLD.state='RETIRED' AND NEW.state<>'RETIRED' THEN
    RAISE EXCEPTION 'retired version cannot be reopened';
  END IF;
  IF NEW.course_id<>OLD.course_id OR NEW.version_no<>OLD.version_no OR
    NEW.completion_policy_version<>OLD.completion_policy_version OR NEW.created_at<>OLD.created_at THEN
    RAISE EXCEPTION 'academy version identity and policy are immutable';
  END IF;
  IF OLD.state<>'DRAFT' AND NEW.published_at IS DISTINCT FROM OLD.published_at THEN
    RAISE EXCEPTION 'academy publication evidence is immutable';
  END IF;
  IF OLD.state='DRAFT' AND NEW.state='PUBLISHED' AND NOT EXISTS (
    SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
    WHERE m.version_id=NEW.id
  ) THEN
    RAISE EXCEPTION 'cannot publish course without lessons';
  END IF;
  RETURN NEW;
END; $$;
CREATE TRIGGER academy_version_publish_guard BEFORE UPDATE ON academy_course_versions
  FOR EACH ROW EXECUTE FUNCTION academy_version_publish_guard();
CREATE TRIGGER academy_version_no_delete BEFORE DELETE ON academy_course_versions
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

-- Enrollment can only be backed by a store-verified ACTIVE entitlement belonging to the learner.
CREATE TABLE academy_enrollments (
  id uuid PRIMARY KEY,
  identity_id uuid NOT NULL REFERENCES identities(id),
  version_id uuid NOT NULL REFERENCES academy_course_versions(id),
  entitlement_id uuid NOT NULL UNIQUE REFERENCES store_entitlements(id),
  state text NOT NULL DEFAULT 'ACTIVE' CHECK(state IN ('ACTIVE','COMPLETED','REVOKED')),
  enrolled_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  UNIQUE(identity_id,version_id),
  CHECK((state='COMPLETED')=(completed_at IS NOT NULL))
);
CREATE INDEX academy_enrollments_learner_idx ON academy_enrollments(identity_id,state);
CREATE FUNCTION academy_enrollment_entitlement_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE ent store_entitlements%ROWTYPE; product_ref text; version_state text;
BEGIN
  SELECT * INTO ent FROM store_entitlements WHERE id=NEW.entitlement_id;
  SELECT 'product/' || c.product_id::text, v.state INTO product_ref,version_state
    FROM academy_course_versions v JOIN academy_courses c ON c.id=v.course_id WHERE v.id=NEW.version_id;
  IF NOT FOUND OR ent.id IS NULL OR ent.identity_id<>NEW.identity_id OR
     ent.state<>'ACTIVE' OR ent.product_ref<>product_ref OR version_state<>'PUBLISHED' THEN
    RAISE EXCEPTION 'academy enrollment requires verified active entitlement for published version';
  END IF;
  RETURN NEW;
END; $$;
CREATE TRIGGER academy_enrollment_insert_guard BEFORE INSERT ON academy_enrollments
  FOR EACH ROW EXECUTE FUNCTION academy_enrollment_entitlement_guard();
CREATE FUNCTION academy_enrollment_transition_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.identity_id<>OLD.identity_id OR NEW.version_id<>OLD.version_id OR
    NEW.entitlement_id<>OLD.entitlement_id OR NEW.enrolled_at<>OLD.enrolled_at THEN
  RAISE EXCEPTION 'academy enrollment immutable basis';
 END IF;
 IF OLD.state<>'ACTIVE' AND NEW.state<>OLD.state THEN
  RAISE EXCEPTION 'academy enrollment terminal';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER academy_enrollment_update_guard BEFORE UPDATE ON academy_enrollments
 FOR EACH ROW EXECUTE FUNCTION academy_enrollment_transition_guard();
CREATE TRIGGER academy_enrollment_no_delete BEFORE DELETE ON academy_enrollments
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

CREATE TABLE academy_progress (
  enrollment_id uuid NOT NULL REFERENCES academy_enrollments(id),
  lesson_id uuid NOT NULL REFERENCES academy_lessons(id),
  evidence_ref text NOT NULL CHECK(btrim(evidence_ref)<>''),
  completed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(enrollment_id,lesson_id)
);
CREATE FUNCTION academy_progress_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (
  SELECT 1 FROM academy_enrollments e
  JOIN academy_modules m ON m.version_id=e.version_id
  JOIN academy_lessons l ON l.module_id=m.id
  JOIN store_entitlements t ON t.id=e.entitlement_id
  WHERE e.id=NEW.enrollment_id AND l.id=NEW.lesson_id
    AND e.state='ACTIVE' AND t.state='ACTIVE'
 ) THEN RAISE EXCEPTION 'academy progress requires active matching enrollment'; END IF;
 IF (SELECT kind FROM academy_lessons WHERE id=NEW.lesson_id)='ASSESSMENT'
    AND NOT EXISTS (
      SELECT 1 FROM academy_attempts a WHERE a.enrollment_id=NEW.enrollment_id
       AND a.lesson_id=NEW.lesson_id AND a.result='PASSED'
    ) THEN RAISE EXCEPTION 'assessment lesson needs evaluated passing attempt'; END IF;
 RETURN NEW;
END; $$;

CREATE TABLE academy_attempts (
  id uuid PRIMARY KEY,
  enrollment_id uuid NOT NULL REFERENCES academy_enrollments(id),
  lesson_id uuid NOT NULL REFERENCES academy_assessments(lesson_id),
  attempt_no integer NOT NULL CHECK(attempt_no>0),
  response_text text NOT NULL CHECK(length(btrim(response_text)) BETWEEN 1 AND 30000),
  result text NOT NULL DEFAULT 'PENDING' CHECK(result IN ('PENDING','PASSED','FAILED')),
  score integer CHECK(score BETWEEN 0 AND 100),
  rubric_version text NOT NULL CHECK(btrim(rubric_version)<>''),
  evaluated_by uuid REFERENCES identities(id),
  evaluation_evidence_ref text,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  evaluated_at timestamptz,
  UNIQUE(enrollment_id,lesson_id,attempt_no),
  CHECK ((result='PENDING' AND score IS NULL AND evaluated_by IS NULL AND evaluated_at IS NULL)
      OR (result<>'PENDING' AND score IS NOT NULL AND evaluated_by IS NOT NULL
          AND evaluated_at IS NOT NULL AND evaluation_evidence_ref IS NOT NULL))
);
CREATE FUNCTION academy_attempt_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF OLD.result<>'PENDING' OR NEW.id<>OLD.id OR NEW.enrollment_id<>OLD.enrollment_id
    OR NEW.lesson_id<>OLD.lesson_id OR NEW.attempt_no<>OLD.attempt_no OR
    NEW.response_text<>OLD.response_text OR NEW.rubric_version<>OLD.rubric_version
    OR NEW.submitted_at<>OLD.submitted_at THEN
   RAISE EXCEPTION 'academy attempt is immutable after evaluation';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER academy_attempt_update_guard BEFORE UPDATE ON academy_attempts
 FOR EACH ROW EXECUTE FUNCTION academy_attempt_guard();
CREATE TRIGGER academy_attempt_no_delete BEFORE DELETE ON academy_attempts
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();
CREATE TRIGGER academy_progress_insert_guard BEFORE INSERT ON academy_progress
 FOR EACH ROW EXECUTE FUNCTION academy_progress_guard();
CREATE TRIGGER academy_progress_append_only BEFORE UPDATE OR DELETE ON academy_progress
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE academy_certificates (
 id uuid PRIMARY KEY,
 enrollment_id uuid NOT NULL UNIQUE REFERENCES academy_enrollments(id),
 verification_token text NOT NULL UNIQUE CHECK(length(verification_token)>=32),
 state text NOT NULL DEFAULT 'ACTIVE' CHECK(state IN ('ACTIVE','REVOKED')),
 completion_evidence_ref text NOT NULL CHECK(btrim(completion_evidence_ref)<>''),
 issued_at timestamptz NOT NULL DEFAULT now(),
 revoked_at timestamptz,
 revocation_reason text,
 CHECK ((state='ACTIVE' AND revoked_at IS NULL AND revocation_reason IS NULL) OR
        (state='REVOKED' AND revoked_at IS NOT NULL AND btrim(revocation_reason) <> ''))
);
CREATE FUNCTION academy_certificate_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  IF NOT EXISTS (SELECT 1 FROM academy_enrollments WHERE id=NEW.enrollment_id AND state='COMPLETED') THEN
   RAISE EXCEPTION 'certificate requires completed enrollment';
  END IF;
 ELSE
  IF OLD.state='REVOKED' OR NEW.id<>OLD.id OR NEW.enrollment_id<>OLD.enrollment_id
   OR NEW.verification_token<>OLD.verification_token OR
   NEW.completion_evidence_ref<>OLD.completion_evidence_ref OR NEW.issued_at<>OLD.issued_at
   OR NEW.state<>'REVOKED' THEN
   RAISE EXCEPTION 'certificate history is immutable';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER academy_certificate_guard BEFORE INSERT OR UPDATE ON academy_certificates
 FOR EACH ROW EXECUTE FUNCTION academy_certificate_guard();
CREATE TRIGGER academy_certificate_no_delete BEFORE DELETE ON academy_certificates
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

CREATE TABLE academy_evidence_candidates (
  id uuid PRIMARY KEY,
  certificate_id uuid NOT NULL UNIQUE REFERENCES academy_certificates(id),
  candidate_identity_id uuid NOT NULL REFERENCES identities(id),
  state text NOT NULL DEFAULT 'PENDING_VERIFICATION' CHECK(state='PENDING_VERIFICATION'),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER academy_evidence_candidate_immutable BEFORE UPDATE OR DELETE ON academy_evidence_candidates
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

-- Future cohort/scheduled delivery, with privacy-safe enrollment isolation.
CREATE TABLE academy_cohorts (
 id uuid PRIMARY KEY, version_id uuid NOT NULL REFERENCES academy_course_versions(id),
 title text NOT NULL CHECK(btrim(title)<>''), starts_at timestamptz NOT NULL,
 ends_at timestamptz, capacity integer CHECK(capacity>0),
 CHECK(ends_at IS NULL OR ends_at>starts_at)
);
CREATE TABLE academy_instructor_assignments (
 cohort_id uuid NOT NULL REFERENCES academy_cohorts(id),
 instructor_identity_id uuid NOT NULL REFERENCES identities(id),
 role_code text NOT NULL CHECK(role_code IN ('OWNER','INSTRUCTOR','REVIEWER')),
 PRIMARY KEY(cohort_id,instructor_identity_id)
);
CREATE TABLE academy_prerequisites (
 version_id uuid NOT NULL REFERENCES academy_course_versions(id),
 prerequisite_version_id uuid NOT NULL REFERENCES academy_course_versions(id),
 PRIMARY KEY(version_id,prerequisite_version_id), CHECK(version_id<>prerequisite_version_id)
);
CREATE TABLE academy_drip_schedule (
 version_id uuid NOT NULL REFERENCES academy_course_versions(id),
 lesson_id uuid NOT NULL UNIQUE REFERENCES academy_lessons(id),
 unlock_after_seconds bigint NOT NULL CHECK(unlock_after_seconds>=0),
 PRIMARY KEY(version_id,lesson_id)
);
COMMIT;
