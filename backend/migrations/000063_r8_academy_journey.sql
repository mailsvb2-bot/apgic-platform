-- R8 educational facts only, owned by canonical Academy.
CREATE TABLE academy_lesson_schedules (
  lesson_id uuid PRIMARY KEY REFERENCES academy_lessons(id),
  release_at timestamptz,
  deadline_at timestamptz,
  policy_version text NOT NULL DEFAULT 'academy-schedule-v1',
  CHECK (deadline_at IS NULL OR release_at IS NULL OR deadline_at > release_at)
);
CREATE TABLE academy_assessment_limits (
  lesson_id uuid PRIMARY KEY REFERENCES academy_assessments(lesson_id),
  max_attempts integer NOT NULL CHECK (max_attempts BETWEEN 1 AND 100),
  submission_deadline timestamptz,
  policy_version text NOT NULL DEFAULT 'academy-attempts-v1'
);
CREATE TABLE academy_assessment_appeals (
  id uuid PRIMARY KEY,
  attempt_id uuid NOT NULL REFERENCES academy_attempts(id),
  submitted_by uuid NOT NULL REFERENCES identities(id),
  state text NOT NULL DEFAULT 'SUBMITTED' CHECK (state IN ('SUBMITTED','UNDER_REVIEW','UPHELD','REJECTED')),
  reason text NOT NULL CHECK (char_length(btrim(reason)) BETWEEN 10 AND 2000),
  independent_reviewer_id uuid REFERENCES identities(id),
  review_evidence_ref text,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  resolved_at timestamptz,
  UNIQUE(attempt_id),
  CHECK ((state IN ('SUBMITTED','UNDER_REVIEW') AND resolved_at IS NULL) OR
         (state IN ('UPHELD','REJECTED') AND resolved_at IS NOT NULL AND btrim(coalesce(review_evidence_ref,''))<>'')),
  CHECK (independent_reviewer_id IS NULL OR independent_reviewer_id <> submitted_by)
);
CREATE INDEX academy_appeals_state_idx ON academy_assessment_appeals(state,submitted_at);
CREATE TABLE academy_guardian_consent_refs (
  learner_identity_id uuid PRIMARY KEY REFERENCES identities(id),
  guardian_identity_id uuid NOT NULL REFERENCES identities(id),
  jurisdiction text NOT NULL CHECK (char_length(btrim(jurisdiction)) BETWEEN 2 AND 16),
  canonical_consent_evidence_ref text NOT NULL CHECK (btrim(canonical_consent_evidence_ref)<>''),
  state text NOT NULL CHECK (state IN ('ACTIVE','REVOKED')),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (learner_identity_id<>guardian_identity_id)
);
CREATE TABLE academy_export_requests (
  id uuid PRIMARY KEY,
  identity_id uuid NOT NULL REFERENCES identities(id),
  request_actor_id uuid NOT NULL REFERENCES identities(id),
  state text NOT NULL DEFAULT 'QUEUED' CHECK (state IN ('QUEUED','PROCESSING','AVAILABLE','FAILED','EXPIRED')),
  authenticated_request_evidence_ref text NOT NULL CHECK (btrim(authenticated_request_evidence_ref)<>''),
  retention_until timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);
CREATE INDEX academy_export_identity_idx ON academy_export_requests(identity_id,created_at DESC);
-- Existing canonical outbox owns notification delivery.
CREATE TABLE academy_notification_preferences (
  identity_id uuid PRIMARY KEY REFERENCES identities(id),
  enabled boolean NOT NULL DEFAULT false,
  learning_updates boolean NOT NULL DEFAULT false,
  assessment_results boolean NOT NULL DEFAULT false,
  certificate_updates boolean NOT NULL DEFAULT false,
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT academy_notifications_disabled_means_no_categories
    CHECK (enabled OR NOT (learning_updates OR assessment_results OR certificate_updates))
);
