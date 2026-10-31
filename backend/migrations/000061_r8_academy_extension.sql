-- R8 supplementary facts, separate from canonical Commerce and Finance truth.
-- All new evidence is append-only and cross-referenced by canonical identifiers.
CREATE TABLE IF NOT EXISTS academy_media_playback_facts (
  id uuid PRIMARY KEY,
  enrollment_id uuid NOT NULL REFERENCES academy_enrollments(id),
  lesson_id uuid NOT NULL REFERENCES academy_lessons(id),
  provider_event_id text NOT NULL,
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  evidence_ref text NOT NULL,
  watched_seconds bigint NOT NULL CHECK (watched_seconds >= 0),
  required_seconds bigint NOT NULL CHECK (required_seconds > 0),
  occurred_at timestamptz NOT NULL,
  recorded_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(provider_instance_id, provider_event_id),
  CHECK (watched_seconds >= required_seconds)
);
CREATE INDEX IF NOT EXISTS academy_media_playback_enrollment_idx ON academy_media_playback_facts(enrollment_id,lesson_id);

-- The canonical cohort already exists in 000060. Extend it; do not shadow
-- its title/starts_at/ends_at/capacity columns with an incompatible table.
ALTER TABLE academy_cohorts
 ADD COLUMN enrollment_closes_at timestamptz;
ALTER TABLE academy_cohorts
 ADD CONSTRAINT academy_cohort_enrollment_closes_before_start
 CHECK (enrollment_closes_at IS NULL OR enrollment_closes_at <= starts_at);
CREATE TABLE IF NOT EXISTS academy_cohort_memberships (
 cohort_id uuid NOT NULL REFERENCES academy_cohorts(id),
 enrollment_id uuid NOT NULL REFERENCES academy_enrollments(id),
 admitted_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(cohort_id,enrollment_id),
 UNIQUE(enrollment_id)
);

CREATE TABLE IF NOT EXISTS academy_cert_revocation_decisions (
 id uuid PRIMARY KEY,
 certificate_id uuid NOT NULL REFERENCES academy_certificates(id),
 operator_id uuid NOT NULL,
 reason_code text NOT NULL CHECK (char_length(btrim(reason_code)) BETWEEN 3 AND 128),
 evidence_ref text NOT NULL CHECK (char_length(btrim(evidence_ref)) > 0),
 step_up_audit_ref text NOT NULL CHECK (char_length(btrim(step_up_audit_ref)) > 0),
 decided_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(certificate_id)
);

CREATE TABLE IF NOT EXISTS academy_rights_verification_facts (
 id uuid PRIMARY KEY,
 version_id uuid NOT NULL REFERENCES academy_course_versions(id),
 content_digest text NOT NULL CHECK (char_length(btrim(content_digest)) >= 32),
 verifier_ref text NOT NULL,
 contract_ref text NOT NULL,
 jurisdiction_code text NOT NULL,
 valid_until timestamptz,
 verified_at timestamptz NOT NULL DEFAULT now(),
 revoked_at timestamptz,
 CHECK (revoked_at IS NULL OR revoked_at >= verified_at)
);
CREATE INDEX IF NOT EXISTS academy_rights_active_version_idx ON academy_rights_verification_facts(version_id, verified_at DESC);

-- Persisted evidence is not provider verification by itself. Runtime ownership
-- must check the signed provider event and active canonical entitlement.
