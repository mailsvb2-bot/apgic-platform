\set ON_ERROR_STOP on

INSERT INTO specialist_profiles (
  id, identity_id, display_name, profession_code, profile_complete, review_state
) VALUES (
  '00000000-0000-0000-0000-000000001001',
  '00000000-0000-0000-0000-000000000001',
  'R1 Invariant Specialist',
  'PSYCHOLOGIST',
  true,
  'APPROVED'
);

INSERT INTO specialist_capabilities (
  specialist_id,
  topic_id,
  evidence_state,
  verification_state,
  evidence_refs,
  verified_at
) VALUES (
  '00000000-0000-0000-0000-000000001001',
  'anxiety',
  'APGIC_VERIFIED',
  'ACTIVE',
  ARRAY['evidence:verified'],
  now()
);

INSERT INTO qualification_evaluations (
  id,
  specialist_id,
  topic_id,
  jurisdiction,
  service_format,
  age_group,
  decision,
  policy_version,
  reason_codes,
  evidence_refs,
  capability_evidence_state,
  capability_verification_state,
  evaluated_at
) VALUES (
  '00000000-0000-0000-0000-000000001101',
  '00000000-0000-0000-0000-000000001001',
  'anxiety',
  'RU',
  'ONLINE',
  'ADULT',
  'MANUAL_REVIEW',
  'qualification-r1-ci-v1',
  ARRAY['QUAL_MANUAL_REVIEW_REQUIRED'],
  ARRAY['evidence:verified'],
  'APGIC_VERIFIED',
  'ACTIVE',
  now()
);

DO $$
DECLARE
  bypass_blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO specialist_publications (
      id,
      specialist_id,
      topic_id,
      qualification_evaluation_id,
      state,
      reason_code,
      published_at
    ) VALUES (
      '00000000-0000-0000-0000-000000001201',
      '00000000-0000-0000-0000-000000001001',
      'anxiety',
      '00000000-0000-0000-0000-000000001101',
      'ACTIVE',
      'PUBLISH_ALLOWED',
      now()
    );
  EXCEPTION WHEN raise_exception THEN
    bypass_blocked := true;
  END;

  IF NOT bypass_blocked THEN
    RAISE EXCEPTION 'MANUAL_REVIEW qualification created active publication';
  END IF;
END
$$;

INSERT INTO qualification_evaluations (
  id,
  specialist_id,
  topic_id,
  jurisdiction,
  service_format,
  age_group,
  decision,
  policy_version,
  reason_codes,
  evidence_refs,
  capability_evidence_state,
  capability_verification_state,
  evaluated_at
) VALUES (
  '00000000-0000-0000-0000-000000001102',
  '00000000-0000-0000-0000-000000001001',
  'anxiety',
  'RU',
  'ONLINE',
  'ADULT',
  'ELIGIBLE',
  'qualification-r1-ci-v1',
  ARRAY['QUAL_ELIGIBLE'],
  ARRAY['evidence:verified'],
  'APGIC_VERIFIED',
  'ACTIVE',
  now()
);

INSERT INTO specialist_publications (
  id,
  specialist_id,
  topic_id,
  qualification_evaluation_id,
  state,
  reason_code,
  published_at
) VALUES (
  '00000000-0000-0000-0000-000000001202',
  '00000000-0000-0000-0000-000000001001',
  'anxiety',
  '00000000-0000-0000-0000-000000001102',
  'ACTIVE',
  'PUBLISH_ALLOWED',
  now()
);

UPDATE specialist_profiles
SET review_state = 'PENDING',
    updated_at = now()
WHERE id = '00000000-0000-0000-0000-000000001001';

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM specialist_publications
    WHERE id = '00000000-0000-0000-0000-000000001202'
      AND state = 'ACTIVE'
  ) THEN
    RAISE EXCEPTION 'review revocation left stale active publication';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM specialist_publications
    WHERE id = '00000000-0000-0000-0000-000000001202'
      AND state = 'SUSPENDED'
      AND reason_code = 'PUBLISH_PROFILE_REVALIDATION_REQUIRED'
      AND ended_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'review revocation suspension evidence missing';
  END IF;
END
$$;

UPDATE specialist_profiles
SET review_state = 'APPROVED',
    updated_at = now()
WHERE id = '00000000-0000-0000-0000-000000001001';

INSERT INTO qualification_evaluations (
  id,
  specialist_id,
  topic_id,
  jurisdiction,
  service_format,
  age_group,
  decision,
  policy_version,
  reason_codes,
  evidence_refs,
  capability_evidence_state,
  capability_verification_state,
  evaluated_at
) VALUES (
  '00000000-0000-0000-0000-000000001103',
  '00000000-0000-0000-0000-000000001001',
  'anxiety',
  'RU',
  'ONLINE',
  'ADULT',
  'ELIGIBLE',
  'qualification-r1-ci-v1',
  ARRAY['QUAL_ELIGIBLE'],
  ARRAY['evidence:verified'],
  'APGIC_VERIFIED',
  'ACTIVE',
  now()
);

INSERT INTO specialist_publications (
  id,
  specialist_id,
  topic_id,
  qualification_evaluation_id,
  state,
  reason_code,
  published_at
) VALUES (
  '00000000-0000-0000-0000-000000001203',
  '00000000-0000-0000-0000-000000001001',
  'anxiety',
  '00000000-0000-0000-0000-000000001103',
  'ACTIVE',
  'PUBLISH_ALLOWED',
  now()
);

UPDATE specialist_capabilities
SET verification_state = 'REVOKED',
    updated_at = now()
WHERE specialist_id = '00000000-0000-0000-0000-000000001001'
  AND topic_id = 'anxiety';

DO $$
DECLARE
  evaluation_rewrite_blocked boolean := false;
BEGIN
  IF EXISTS (
    SELECT 1
    FROM specialist_publications
    WHERE id = '00000000-0000-0000-0000-000000001203'
      AND state = 'ACTIVE'
  ) THEN
    RAISE EXCEPTION 'capability revocation left stale active publication';
  END IF;

  BEGIN
    UPDATE qualification_evaluations
    SET reason_codes = ARRAY['REWRITTEN']
    WHERE id = '00000000-0000-0000-0000-000000001103';
  EXCEPTION WHEN raise_exception THEN
    evaluation_rewrite_blocked := true;
  END;

  IF NOT evaluation_rewrite_blocked THEN
    RAISE EXCEPTION 'qualification evidence rewrite was accepted';
  END IF;
END
$$;

INSERT INTO help_intents (
  id,
  identity_id,
  free_text,
  topics,
  goals,
  context,
  state
) VALUES (
  '00000000-0000-0000-0000-000000001301',
  '00000000-0000-0000-0000-000000000001',
  'Нужна помощь с тревогой перед выступлениями',
  ARRAY['anxiety'],
  ARRAY['public-speaking'],
  '{"format":"online"}'::jsonb,
  'DRAFT'
);

DO $$
DECLARE
  invalid_confirmation_blocked boolean := false;
  hard_delete_blocked boolean := false;
BEGIN
  BEGIN
    UPDATE help_intents
    SET state = 'CONFIRMED',
        updated_at = now()
    WHERE id = '00000000-0000-0000-0000-000000001301';
  EXCEPTION WHEN check_violation THEN
    invalid_confirmation_blocked := true;
  END;

  IF NOT invalid_confirmation_blocked THEN
    RAISE EXCEPTION 'confirmed HelpIntent without confirmation evidence was accepted';
  END IF;

  UPDATE help_intents
  SET state = 'CONFIRMED',
      topics = ARRAY['performance-anxiety'],
      confirmed_at = now(),
      updated_at = now()
  WHERE id = '00000000-0000-0000-0000-000000001301';

  BEGIN
    DELETE FROM help_intents
    WHERE id = '00000000-0000-0000-0000-000000001301';
  EXCEPTION WHEN raise_exception THEN
    hard_delete_blocked := true;
  END;

  IF NOT hard_delete_blocked THEN
    RAISE EXCEPTION 'canonical HelpIntent hard delete was accepted';
  END IF;
END
$$;
