BEGIN;

CREATE TABLE specialist_profiles (
  id uuid PRIMARY KEY,
  identity_id uuid NOT NULL UNIQUE REFERENCES identities(id),
  display_name text NOT NULL CHECK (nullif(btrim(display_name), '') IS NOT NULL),
  profession_code text NOT NULL CHECK (nullif(btrim(profession_code), '') IS NOT NULL),
  profile_complete boolean NOT NULL DEFAULT false,
  review_state text NOT NULL DEFAULT 'PENDING' CHECK (
    review_state IN ('PENDING','MANUAL_REVIEW','APPROVED','REJECTED')
  ),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE specialist_capabilities (
  specialist_id uuid NOT NULL REFERENCES specialist_profiles(id),
  topic_id text NOT NULL CHECK (nullif(btrim(topic_id), '') IS NOT NULL),
  evidence_state text NOT NULL CHECK (
    evidence_state IN ('SELF_DECLARED','DOCUMENT_SUPPORTED','APGIC_VERIFIED')
  ),
  verification_state text NOT NULL CHECK (
    verification_state IN ('NOT_APPLICABLE','PENDING','ACTIVE','REVIEW_REQUIRED','EXPIRED','REVOKED')
  ),
  evidence_refs text[] NOT NULL DEFAULT ARRAY[]::text[],
  verified_at timestamptz,
  expires_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (specialist_id, topic_id),
  CHECK (
    (
      evidence_state = 'APGIC_VERIFIED'
      AND verification_state IN ('ACTIVE','REVIEW_REQUIRED','EXPIRED','REVOKED')
      AND verified_at IS NOT NULL
    )
    OR
    (
      evidence_state <> 'APGIC_VERIFIED'
      AND verification_state IN ('NOT_APPLICABLE','PENDING')
      AND verified_at IS NULL
    )
  ),
  CHECK (expires_at IS NULL OR verified_at IS NOT NULL),
  CHECK (expires_at IS NULL OR expires_at > verified_at)
);

CREATE TABLE help_intents (
  id uuid PRIMARY KEY,
  identity_id uuid REFERENCES identities(id),
  free_text text NOT NULL CHECK (nullif(btrim(free_text), '') IS NOT NULL),
  topics text[] NOT NULL DEFAULT ARRAY[]::text[],
  goals text[] NOT NULL DEFAULT ARRAY[]::text[],
  context jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(context) = 'object'),
  state text NOT NULL DEFAULT 'DRAFT' CHECK (state IN ('DRAFT','CONFIRMED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  confirmed_at timestamptz,
  CHECK (
    (state = 'DRAFT' AND confirmed_at IS NULL)
    OR (state = 'CONFIRMED' AND confirmed_at IS NOT NULL)
  )
);

CREATE TABLE qualification_evaluations (
  id uuid PRIMARY KEY,
  specialist_id uuid NOT NULL,
  topic_id text NOT NULL,
  jurisdiction text NOT NULL CHECK (nullif(btrim(jurisdiction), '') IS NOT NULL),
  service_format text NOT NULL CHECK (nullif(btrim(service_format), '') IS NOT NULL),
  age_group text NOT NULL CHECK (nullif(btrim(age_group), '') IS NOT NULL),
  requested_activity text NOT NULL DEFAULT 'DISCOVERY' CHECK (
    nullif(btrim(requested_activity), '') IS NOT NULL
  ),
  decision text NOT NULL CHECK (
    decision IN ('ELIGIBLE','ELIGIBLE_WITH_CONDITIONS','MANUAL_REVIEW','INELIGIBLE')
  ),
  policy_version text NOT NULL CHECK (nullif(btrim(policy_version), '') IS NOT NULL),
  reason_codes text[] NOT NULL CHECK (cardinality(reason_codes) > 0),
  evidence_refs text[] NOT NULL DEFAULT ARRAY[]::text[],
  capability_evidence_state text NOT NULL CHECK (
    capability_evidence_state IN ('SELF_DECLARED','DOCUMENT_SUPPORTED','APGIC_VERIFIED')
  ),
  capability_verification_state text NOT NULL CHECK (
    capability_verification_state IN ('NOT_APPLICABLE','PENDING','ACTIVE','REVIEW_REQUIRED','EXPIRED','REVOKED')
  ),
  evaluated_at timestamptz NOT NULL,
  FOREIGN KEY (specialist_id, topic_id)
    REFERENCES specialist_capabilities(specialist_id, topic_id)
);

CREATE INDEX qualification_evaluations_subject_idx
  ON qualification_evaluations (specialist_id, topic_id, evaluated_at DESC);

CREATE TABLE specialist_publications (
  id uuid PRIMARY KEY,
  specialist_id uuid NOT NULL,
  topic_id text NOT NULL,
  qualification_evaluation_id uuid NOT NULL UNIQUE REFERENCES qualification_evaluations(id),
  state text NOT NULL DEFAULT 'ACTIVE' CHECK (state IN ('ACTIVE','UNPUBLISHED','SUSPENDED')),
  reason_code text NOT NULL CHECK (nullif(btrim(reason_code), '') IS NOT NULL),
  published_at timestamptz NOT NULL,
  ended_at timestamptz,
  FOREIGN KEY (specialist_id, topic_id)
    REFERENCES specialist_capabilities(specialist_id, topic_id),
  CHECK (
    (state = 'ACTIVE' AND ended_at IS NULL)
    OR (state IN ('UNPUBLISHED','SUSPENDED') AND ended_at IS NOT NULL)
  )
);

CREATE UNIQUE INDEX specialist_publications_one_active_topic_idx
  ON specialist_publications (specialist_id, topic_id)
  WHERE state = 'ACTIVE';

CREATE OR REPLACE FUNCTION apgic_specialist_publication_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  evaluation qualification_evaluations%ROWTYPE;
  profile specialist_profiles%ROWTYPE;
BEGIN
  IF TG_OP = 'UPDATE' THEN
    IF NEW.specialist_id <> OLD.specialist_id
      OR NEW.topic_id <> OLD.topic_id
      OR NEW.qualification_evaluation_id <> OLD.qualification_evaluation_id
      OR NEW.published_at <> OLD.published_at THEN
      RAISE EXCEPTION 'specialist publication identity/evidence is immutable';
    END IF;

    IF OLD.state <> 'ACTIVE' AND NEW.state = 'ACTIVE' THEN
      RAISE EXCEPTION 'ended specialist publication cannot be reactivated';
    END IF;
  END IF;

  IF NEW.state = 'ACTIVE' THEN
    SELECT * INTO evaluation
    FROM qualification_evaluations
    WHERE id = NEW.qualification_evaluation_id;

    IF NOT FOUND THEN
      RAISE EXCEPTION 'specialist publication requires qualification evaluation';
    END IF;

    IF evaluation.specialist_id <> NEW.specialist_id
      OR evaluation.topic_id <> NEW.topic_id THEN
      RAISE EXCEPTION 'publication qualification subject mismatch';
    END IF;

    IF evaluation.decision <> 'ELIGIBLE' THEN
      RAISE EXCEPTION 'only ELIGIBLE qualification may create active publication';
    END IF;

    SELECT * INTO profile
    FROM specialist_profiles
    WHERE id = NEW.specialist_id;

    IF NOT FOUND OR NOT profile.profile_complete OR profile.review_state <> 'APPROVED' THEN
      RAISE EXCEPTION 'active publication requires complete approved specialist profile';
    END IF;
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER specialist_publications_guard
BEFORE INSERT OR UPDATE ON specialist_publications
FOR EACH ROW EXECUTE FUNCTION apgic_specialist_publication_guard();

CREATE OR REPLACE FUNCTION apgic_suspend_publication_on_profile_change()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NOT NEW.profile_complete OR NEW.review_state <> 'APPROVED' THEN
    UPDATE specialist_publications
    SET state = 'SUSPENDED',
        reason_code = 'PUBLISH_PROFILE_REVALIDATION_REQUIRED',
        ended_at = COALESCE(ended_at, now())
    WHERE specialist_id = NEW.id
      AND state = 'ACTIVE';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER specialist_profile_revalidation_guard
AFTER UPDATE OF profile_complete, review_state ON specialist_profiles
FOR EACH ROW EXECUTE FUNCTION apgic_suspend_publication_on_profile_change();

CREATE OR REPLACE FUNCTION apgic_suspend_publication_on_capability_change()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.evidence_state IS DISTINCT FROM OLD.evidence_state
    OR NEW.verification_state IS DISTINCT FROM OLD.verification_state
    OR NEW.evidence_refs IS DISTINCT FROM OLD.evidence_refs
    OR NEW.expires_at IS DISTINCT FROM OLD.expires_at THEN
    UPDATE specialist_publications
    SET state = 'SUSPENDED',
        reason_code = 'PUBLISH_CAPABILITY_REVALIDATION_REQUIRED',
        ended_at = COALESCE(ended_at, now())
    WHERE specialist_id = NEW.specialist_id
      AND topic_id = NEW.topic_id
      AND state = 'ACTIVE';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER specialist_capability_revalidation_guard
AFTER UPDATE OF evidence_state, verification_state, evidence_refs, expires_at
ON specialist_capabilities
FOR EACH ROW EXECUTE FUNCTION apgic_suspend_publication_on_capability_change();

CREATE TRIGGER qualification_evaluations_append_only
BEFORE UPDATE OR DELETE ON qualification_evaluations
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TRIGGER specialist_profiles_no_delete
BEFORE DELETE ON specialist_profiles
FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

CREATE TRIGGER specialist_capabilities_no_delete
BEFORE DELETE ON specialist_capabilities
FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

CREATE TRIGGER help_intents_no_delete
BEFORE DELETE ON help_intents
FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

COMMIT;
