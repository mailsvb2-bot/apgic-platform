-- R8 versioned CDN playback evidence. This extends the existing Academy
-- evidence owner; it does not create a second media or provider owner.

CREATE TABLE academy_video_provider_certifications (
  id uuid PRIMARY KEY,
  connector_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  contract_version text NOT NULL CHECK (contract_version='cdn-playback-v1'),
  provider_kind text NOT NULL CHECK (length(btrim(provider_kind)) BETWEEN 1 AND 64),
  key_id text NOT NULL CHECK (length(btrim(key_id)) BETWEEN 1 AND 64),
  acceptance_sha256 text NOT NULL CHECK (acceptance_sha256 ~ '^[0-9a-f]{64}$'),
  acceptance_ref text NOT NULL CHECK (
    length(btrim(acceptance_ref)) BETWEEN 1 AND 2048
    AND position('://' in acceptance_ref)=0
  ),
  tested_source_sha text NOT NULL CHECK (tested_source_sha ~ '^[0-9a-f]{40}$'),
  accepted_by uuid NOT NULL REFERENCES identities(id),
  step_up_audit_ref uuid NOT NULL REFERENCES audit_records(id),
  accepted_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  CHECK (expires_at > accepted_at)
);
CREATE UNIQUE INDEX academy_video_provider_certification_source_unique
  ON academy_video_provider_certifications(
    connector_instance_id,contract_version,key_id,tested_source_sha
  );

CREATE FUNCTION academy_video_provider_certification_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE
  auth_actor text;
  auth_action text;
  auth_state jsonb;
  auth_reason text;
  auth_occurred_at timestamptz;
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM connector_instances ci
    WHERE ci.id=NEW.connector_instance_id
      AND ci.capability_class='CDN_PROVIDER'
      AND ci.provider_kind=NEW.provider_kind
      AND ci.status='ACTIVE'
  ) THEN
    RAISE EXCEPTION 'academy video certification requires an active matching CDN_PROVIDER instance';
  END IF;

  SELECT actor_id,action,new_state,reason,occurred_at
    INTO auth_actor,auth_action,auth_state,auth_reason,auth_occurred_at
  FROM audit_records
  WHERE id=NEW.step_up_audit_ref;
  IF NOT FOUND
    OR auth_actor IS DISTINCT FROM NEW.accepted_by::text
    OR auth_action IS DISTINCT FROM 'authorization.decision'
    OR auth_state->>'decision' IS DISTINCT FROM 'ALLOW'
    OR auth_state->>'risk' IS DISTINCT FROM 'HIGH_RISK'
    OR auth_state->>'action' IS DISTINCT FROM 'academy.video.provider.certify'
    OR btrim(COALESCE(auth_reason,''))=''
    OR NEW.accepted_at<auth_occurred_at
    OR NEW.accepted_at>auth_occurred_at+interval '5 minutes' THEN
    RAISE EXCEPTION 'academy video certification requires a fresh high-risk ALLOW audit by the accepting operator';
  END IF;

  IF NEW.expires_at <= now() OR NEW.accepted_at > now() THEN
    RAISE EXCEPTION 'academy video certification timestamps are not currently valid';
  END IF;
  RETURN NEW;
END;
$fn$;
CREATE TRIGGER academy_video_provider_certification_insert_guard
  BEFORE INSERT ON academy_video_provider_certifications
  FOR EACH ROW EXECUTE FUNCTION academy_video_provider_certification_guard();
CREATE TRIGGER academy_video_provider_certification_immutable
  BEFORE UPDATE OR DELETE ON academy_video_provider_certifications
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE FUNCTION academy_video_provider_certification_available(p_source_sha text)
RETURNS boolean LANGUAGE sql STABLE AS $fn$
  SELECT COALESCE(p_source_sha ~ '^[0-9a-f]{40}$',false) AND EXISTS (
    SELECT 1
    FROM academy_video_provider_certifications cert
    JOIN connector_instances ci ON ci.id=cert.connector_instance_id
    WHERE cert.contract_version='cdn-playback-v1'
      AND cert.tested_source_sha=p_source_sha
      AND ci.capability_class='CDN_PROVIDER'
      AND ci.provider_kind=cert.provider_kind
      AND ci.status='ACTIVE'
      AND cert.expires_at>now()
  );
$fn$;

ALTER TABLE academy_media_playback_facts
  ADD COLUMN contract_version text,
  ADD COLUMN course_id uuid REFERENCES academy_courses(id),
  ADD COLUMN course_version_id uuid REFERENCES academy_course_versions(id),
  ADD COLUMN subject_id uuid REFERENCES identities(id),
  ADD COLUMN entitlement_id uuid REFERENCES store_entitlements(id),
  ADD COLUMN media_id uuid REFERENCES academy_cloud_media(id),
  ADD COLUMN session_id uuid,
  ADD COLUMN provider_kind text,
  ADD COLUMN content_sha256 text,
  ADD COLUMN content_length bigint,
  ADD COLUMN duration_millis bigint,
  ADD COLUMN watched_ranges jsonb,
  ADD COLUMN signed_attestation jsonb,
  ADD COLUMN attestation_sha256 text,
  ADD COLUMN key_id text;

ALTER TABLE academy_media_playback_facts
  ADD CONSTRAINT academy_media_playback_typed_evidence_check CHECK (
    (
      contract_version IS NULL AND course_id IS NULL AND course_version_id IS NULL
      AND subject_id IS NULL AND entitlement_id IS NULL AND media_id IS NULL
      AND session_id IS NULL AND provider_kind IS NULL AND content_sha256 IS NULL
      AND content_length IS NULL AND duration_millis IS NULL AND watched_ranges IS NULL
      AND signed_attestation IS NULL AND attestation_sha256 IS NULL AND key_id IS NULL
    )
    OR
    (
      contract_version='cdn-playback-v1'
      AND course_id IS NOT NULL AND course_version_id IS NOT NULL
      AND subject_id IS NOT NULL AND entitlement_id IS NOT NULL AND media_id IS NOT NULL
      AND session_id IS NOT NULL
      AND provider_kind IS NOT NULL AND btrim(provider_kind)<>''
      AND content_sha256 IS NOT NULL AND content_sha256 ~ '^[0-9a-f]{64}$'
      AND content_length IS NOT NULL AND content_length>0
      AND duration_millis IS NOT NULL AND duration_millis>0
      AND watched_ranges IS NOT NULL AND jsonb_typeof(watched_ranges)='array'
      AND signed_attestation IS NOT NULL AND jsonb_typeof(signed_attestation)='object'
      AND jsonb_typeof(signed_attestation->'binding')='object'
      AND (signed_attestation ?& ARRAY[
        'contract_version','provider_kind','provider_event_id','provider_instance_id',
        'session_id','key_id','signature','completed_at','watched_ranges','binding'
      ])
      AND ((signed_attestation->'binding') ?& ARRAY[
        'subject_id','enrollment_id','entitlement_id','course_id',
        'course_version_id','lesson_id','media_id','content_sha256','content_length'
      ])
      AND btrim(COALESCE(signed_attestation->>'signature',''))<>''
      AND attestation_sha256 IS NOT NULL AND attestation_sha256 ~ '^[0-9a-f]{64}$'
      AND key_id IS NOT NULL AND btrim(key_id)<>''
      AND provider_event_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
      AND evidence_ref='sha256:'||attestation_sha256
      AND signed_attestation->>'contract_version'=contract_version
      AND signed_attestation->>'provider_kind'=provider_kind
      AND signed_attestation->>'provider_event_id'=provider_event_id
      AND signed_attestation->>'provider_instance_id'=provider_instance_id::text
      AND signed_attestation->>'session_id'=session_id::text
      AND signed_attestation->>'key_id'=key_id
      AND signed_attestation->'watched_ranges'=watched_ranges
      AND signed_attestation->'binding'->>'subject_id'=subject_id::text
      AND signed_attestation->'binding'->>'enrollment_id'=enrollment_id::text
      AND signed_attestation->'binding'->>'entitlement_id'=entitlement_id::text
      AND signed_attestation->'binding'->>'course_id'=course_id::text
      AND signed_attestation->'binding'->>'course_version_id'=course_version_id::text
      AND signed_attestation->'binding'->>'lesson_id'=lesson_id::text
      AND signed_attestation->'binding'->>'media_id'=media_id::text
      AND signed_attestation->'binding'->>'content_sha256'=content_sha256
      AND (signed_attestation->'binding'->>'content_length')::bigint=content_length
    )
  );

CREATE UNIQUE INDEX academy_media_playback_session_unique
  ON academy_media_playback_facts(provider_instance_id,session_id)
  WHERE session_id IS NOT NULL;

CREATE OR REPLACE FUNCTION academy_video_ranges_cover_duration(p_ranges jsonb,p_duration_millis bigint)
RETURNS boolean LANGUAGE plpgsql IMMUTABLE AS $fn$
DECLARE
  item jsonb;
  start_text text;
  end_text text;
  start_value numeric;
  end_value numeric;
  start_millis bigint;
  end_millis bigint;
  previous_end bigint := 0;
  item_count integer := 0;
BEGIN
  -- Keep the database guard at least as strict as the Go verifier. In
  -- particular, SQL bool_and() ignores NULL values, so aggregate-only
  -- validation could accidentally accept a valid full range plus a malformed
  -- row whose missing endpoint evaluated to NULL.
  IF p_ranges IS NULL OR jsonb_typeof(p_ranges) <> 'array'
    OR p_duration_millis IS NULL OR p_duration_millis <= 0
    OR p_duration_millis > 86400000 THEN
    RETURN false;
  END IF;
  IF jsonb_array_length(p_ranges) = 0 OR jsonb_array_length(p_ranges) > 10000 THEN
    RETURN false;
  END IF;

  FOR item IN
    SELECT value FROM jsonb_array_elements(p_ranges) AS ranges(value)
  LOOP
    item_count := item_count + 1;
    IF jsonb_typeof(item) <> 'object'
      OR NOT (item ?& ARRAY['start_millis','end_millis'])
      OR (item - ARRAY['start_millis','end_millis']) <> '{}'::jsonb
      OR jsonb_typeof(item->'start_millis') <> 'number'
      OR jsonb_typeof(item->'end_millis') <> 'number' THEN
      RETURN false;
    END IF;

    start_text := item->>'start_millis';
    end_text := item->>'end_millis';
    IF start_text IS NULL OR end_text IS NULL
      OR start_text !~ '^(0|[1-9][0-9]*)

CREATE OR REPLACE FUNCTION academy_media_fact_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE
  enrollment_version uuid;
  enrollment_identity uuid;
  enrollment_entitlement uuid;
  enrollment_state text;
  lesson_version uuid;
  lesson_kind text;
  lesson_course uuid;
  lesson_author uuid;
  media_matches boolean;
  provider_matches boolean;
BEGIN
  SELECT e.version_id,e.identity_id,e.entitlement_id,e.state
    INTO enrollment_version,enrollment_identity,enrollment_entitlement,enrollment_state
  FROM academy_enrollments e
  JOIN store_entitlements t ON t.id=e.entitlement_id
  WHERE e.id=NEW.enrollment_id AND academy_entitlement_is_active(t.id)
  FOR UPDATE OF e,t;
  IF NOT FOUND OR enrollment_state<>'ACTIVE' THEN
    RAISE EXCEPTION 'academy playback evidence requires an active verified enrollment';
  END IF;

  SELECT m.version_id,l.kind,v.course_id,c.author_id
    INTO lesson_version,lesson_kind,lesson_course,lesson_author
  FROM academy_lessons l
  JOIN academy_modules m ON m.id=l.module_id
  JOIN academy_course_versions v ON v.id=m.version_id
  JOIN academy_courses c ON c.id=v.course_id
  WHERE l.id=NEW.lesson_id;
  IF NOT FOUND OR lesson_kind<>'VIDEO'
    OR lesson_version IS DISTINCT FROM enrollment_version
    OR NEW.contract_version IS DISTINCT FROM 'cdn-playback-v1'
    OR NEW.course_id IS DISTINCT FROM lesson_course
    OR NEW.course_version_id IS DISTINCT FROM enrollment_version
    OR NEW.subject_id IS DISTINCT FROM enrollment_identity
    OR NEW.entitlement_id IS DISTINCT FROM enrollment_entitlement
    OR NEW.signed_attestation IS NULL OR NEW.attestation_sha256 IS NULL
    OR NEW.key_id IS NULL OR NEW.session_id IS NULL
    OR NEW.media_id IS NULL OR NEW.content_sha256 IS NULL
    OR NEW.content_length IS NULL OR NEW.duration_millis IS NULL
    OR NEW.watched_ranges IS NULL THEN
    RAISE EXCEPTION 'academy playback evidence does not match the active video lesson binding';
  END IF;

  IF NEW.watched_seconds<NEW.required_seconds
    OR NEW.required_seconds<>ceil(NEW.duration_millis::numeric/1000)::bigint
    OR NOT academy_video_ranges_cover_duration(NEW.watched_ranges,NEW.duration_millis)
    OR NEW.occurred_at IS DISTINCT FROM (NEW.signed_attestation->>'completed_at')::timestamptz
    OR NEW.evidence_ref IS DISTINCT FROM 'sha256:'||NEW.attestation_sha256 THEN
    RAISE EXCEPTION 'academy playback evidence is not a complete signed timeline';
  END IF;

  SELECT EXISTS (
    SELECT 1 FROM academy_lesson_media lm
    JOIN academy_cloud_media cm ON cm.id=lm.media_id
    JOIN academy_cloud_connections cc
      ON cc.author_id=cm.author_id AND cc.provider=cm.provider
      AND cc.generation=cm.connection_generation
    WHERE lm.lesson_id=NEW.lesson_id AND lm.media_id=NEW.media_id
      AND cm.author_id=lesson_author AND cm.provider='YANDEX_DISK'
      AND cm.resource_id IS NOT NULL AND cm.sha256=NEW.content_sha256
      AND cm.file_size=NEW.content_length
  ) INTO media_matches;
  IF NOT media_matches THEN
    RAISE EXCEPTION 'academy playback media fingerprint or author assignment mismatch';
  END IF;

  SELECT EXISTS (
    SELECT 1 FROM connector_instances ci
    JOIN academy_video_provider_certifications cert
      ON cert.connector_instance_id=ci.id
    WHERE ci.id=NEW.provider_instance_id
      AND ci.capability_class='CDN_PROVIDER'
      AND ci.provider_kind=NEW.provider_kind
      AND ci.status='ACTIVE'
      AND cert.contract_version=NEW.contract_version
      AND cert.provider_kind=NEW.provider_kind
      AND cert.key_id=NEW.key_id
      AND cert.tested_source_sha=NULLIF(current_setting('apgic.release_source_sha',true),'')
      AND cert.expires_at>now()
  ) INTO provider_matches;
  IF NOT provider_matches THEN
    RAISE EXCEPTION 'academy playback requires the certified active CDN provider instance';
  END IF;
  IF btrim(NEW.provider_event_id)='' OR btrim(NEW.evidence_ref)='' THEN
    RAISE EXCEPTION 'academy playback requires provider event and signed evidence reference';
  END IF;
  RETURN NEW;
END;
$fn$;

CREATE OR REPLACE FUNCTION academy_progress_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE lesson_kind text;
BEGIN
  SELECT l.kind INTO lesson_kind
  FROM academy_enrollments e
  JOIN academy_modules m ON m.version_id=e.version_id
  JOIN academy_lessons l ON l.module_id=m.id
  JOIN store_entitlements t ON t.id=e.entitlement_id
  WHERE e.id=NEW.enrollment_id AND l.id=NEW.lesson_id
    AND e.state='ACTIVE' AND academy_entitlement_is_active(t.id);
  IF NOT FOUND THEN
    RAISE EXCEPTION 'academy progress requires active matching enrollment and entitlement';
  END IF;
  IF lesson_kind='VIDEO' AND NOT EXISTS (
    SELECT 1 FROM academy_media_playback_facts f
    WHERE f.enrollment_id=NEW.enrollment_id AND f.lesson_id=NEW.lesson_id
      AND f.contract_version='cdn-playback-v1'
      AND f.evidence_ref=NEW.evidence_ref
      AND f.attestation_sha256 IS NOT NULL
      AND f.watched_seconds>=f.required_seconds
  ) THEN
    RAISE EXCEPTION 'academy video progress requires persisted complete provider attestation';
  END IF;
  IF lesson_kind='ASSESSMENT' AND NOT EXISTS (
    SELECT 1 FROM academy_attempts a WHERE a.enrollment_id=NEW.enrollment_id
      AND a.lesson_id=NEW.lesson_id AND a.result='PASSED'
  ) THEN
    RAISE EXCEPTION 'academy assessment requires a human-evaluated passing attempt';
  END IF;
  RETURN NEW;
END;
$fn$;

CREATE OR REPLACE FUNCTION academy_version_publish_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
BEGIN
  IF OLD.state='PUBLISHED' AND NEW.state NOT IN ('PUBLISHED','RETIRED') THEN
    RAISE EXCEPTION 'published version cannot become draft';
  END IF;
  IF OLD.state='RETIRED' AND NEW.state<>'RETIRED' THEN
    RAISE EXCEPTION 'retired version cannot be reopened';
  END IF;
  IF NEW.course_id<>OLD.course_id OR NEW.version_no<>OLD.version_no OR
    NEW.completion_policy_version<>OLD.completion_policy_version OR NEW.created_at<>OLD.created_at THEN
    RAISE EXCEPTION 'academy version identity and policy are immutable';
  END IF;
  IF OLD.state<>'DRAFT' AND (NEW.title_snapshot IS DISTINCT FROM OLD.title_snapshot OR
    NEW.description_snapshot IS DISTINCT FROM OLD.description_snapshot) THEN
    RAISE EXCEPTION 'academy version published content provenance is immutable';
  END IF;
  IF OLD.state<>'DRAFT' AND NEW.published_at IS DISTINCT FROM OLD.published_at THEN
    RAISE EXCEPTION 'academy publication evidence is immutable';
  END IF;
  IF OLD.state='DRAFT' AND NEW.state='PUBLISHED' AND NOT EXISTS (
    SELECT 1 FROM academy_original_rights r
    JOIN academy_courses c ON c.id=NEW.course_id
    WHERE r.version_id=NEW.id AND r.author_id=c.author_id
      AND r.curriculum_sha256=academy_curriculum_fingerprint(NEW.id)
  ) THEN
    RAISE EXCEPTION 'academy author ownership declaration required for the exact curriculum';
  END IF;
  IF OLD.state='DRAFT' AND NEW.state='PUBLISHED' AND EXISTS (
    SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
    WHERE m.version_id=NEW.id AND l.kind='VIDEO'
  ) THEN
    IF NOT academy_video_provider_certification_available(
      NULLIF(current_setting('apgic.release_source_sha',true),'')
    ) THEN
      RAISE EXCEPTION 'academy video publication blocked without durable CDN provider acceptance for this source SHA';
    END IF;
    IF EXISTS (
      SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
      WHERE m.version_id=NEW.id AND l.kind='VIDEO' AND NOT EXISTS (
        SELECT 1 FROM academy_lesson_media lm
        JOIN academy_cloud_media cm ON cm.id=lm.media_id
        JOIN academy_cloud_connections cc
          ON cc.author_id=cm.author_id AND cc.provider=cm.provider
          AND cc.generation=cm.connection_generation
        JOIN academy_courses c ON c.id=NEW.course_id
        WHERE lm.lesson_id=l.id AND cm.author_id=c.author_id
          AND cm.provider='YANDEX_DISK' AND cm.resource_id IS NOT NULL
          AND cm.sha256 IS NOT NULL AND cm.file_size>0
      )
    ) THEN
      RAISE EXCEPTION 'academy video publication requires a verified current author media assignment for every video lesson';
    END IF;
    IF EXISTS (
      SELECT 1 FROM academy_lesson_media lm
      JOIN academy_lessons l ON l.id=lm.lesson_id
      JOIN academy_modules m ON m.id=l.module_id
      JOIN academy_cloud_media cm ON cm.id=lm.media_id
      LEFT JOIN academy_cloud_connections cc
        ON cc.author_id=cm.author_id AND cc.provider=cm.provider
        AND cc.generation=cm.connection_generation
      WHERE m.version_id=NEW.id AND l.kind='VIDEO'
        AND (cm.provider<>'YANDEX_DISK' OR cm.resource_id IS NULL
          OR cm.sha256 IS NULL OR cm.file_size IS NULL OR cm.file_size<=0
          OR cc.generation IS NULL)
    ) THEN
      RAISE EXCEPTION 'academy video publication found a stale or unverified media assignment';
    END IF;
  END IF;
  IF OLD.state='DRAFT' AND NEW.state='PUBLISHED' AND NOT EXISTS (
    SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
    WHERE m.version_id=NEW.id
  ) THEN
    RAISE EXCEPTION 'cannot publish course without lessons';
  END IF;
  RETURN NEW;
END;
$fn$;

      OR end_text !~ '^(0|[1-9][0-9]*)

CREATE OR REPLACE FUNCTION academy_media_fact_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE
  enrollment_version uuid;
  enrollment_identity uuid;
  enrollment_entitlement uuid;
  enrollment_state text;
  lesson_version uuid;
  lesson_kind text;
  lesson_course uuid;
  lesson_author uuid;
  media_matches boolean;
  provider_matches boolean;
BEGIN
  SELECT e.version_id,e.identity_id,e.entitlement_id,e.state
    INTO enrollment_version,enrollment_identity,enrollment_entitlement,enrollment_state
  FROM academy_enrollments e
  JOIN store_entitlements t ON t.id=e.entitlement_id
  WHERE e.id=NEW.enrollment_id AND academy_entitlement_is_active(t.id)
  FOR UPDATE OF e,t;
  IF NOT FOUND OR enrollment_state<>'ACTIVE' THEN
    RAISE EXCEPTION 'academy playback evidence requires an active verified enrollment';
  END IF;

  SELECT m.version_id,l.kind,v.course_id,c.author_id
    INTO lesson_version,lesson_kind,lesson_course,lesson_author
  FROM academy_lessons l
  JOIN academy_modules m ON m.id=l.module_id
  JOIN academy_course_versions v ON v.id=m.version_id
  JOIN academy_courses c ON c.id=v.course_id
  WHERE l.id=NEW.lesson_id;
  IF NOT FOUND OR lesson_kind<>'VIDEO'
    OR lesson_version IS DISTINCT FROM enrollment_version
    OR NEW.contract_version IS DISTINCT FROM 'cdn-playback-v1'
    OR NEW.course_id IS DISTINCT FROM lesson_course
    OR NEW.course_version_id IS DISTINCT FROM enrollment_version
    OR NEW.subject_id IS DISTINCT FROM enrollment_identity
    OR NEW.entitlement_id IS DISTINCT FROM enrollment_entitlement
    OR NEW.signed_attestation IS NULL OR NEW.attestation_sha256 IS NULL
    OR NEW.key_id IS NULL OR NEW.session_id IS NULL
    OR NEW.media_id IS NULL OR NEW.content_sha256 IS NULL
    OR NEW.content_length IS NULL OR NEW.duration_millis IS NULL
    OR NEW.watched_ranges IS NULL THEN
    RAISE EXCEPTION 'academy playback evidence does not match the active video lesson binding';
  END IF;

  IF NEW.watched_seconds<NEW.required_seconds
    OR NEW.required_seconds<>ceil(NEW.duration_millis::numeric/1000)::bigint
    OR NOT academy_video_ranges_cover_duration(NEW.watched_ranges,NEW.duration_millis)
    OR NEW.occurred_at IS DISTINCT FROM (NEW.signed_attestation->>'completed_at')::timestamptz
    OR NEW.evidence_ref IS DISTINCT FROM 'sha256:'||NEW.attestation_sha256 THEN
    RAISE EXCEPTION 'academy playback evidence is not a complete signed timeline';
  END IF;

  SELECT EXISTS (
    SELECT 1 FROM academy_lesson_media lm
    JOIN academy_cloud_media cm ON cm.id=lm.media_id
    JOIN academy_cloud_connections cc
      ON cc.author_id=cm.author_id AND cc.provider=cm.provider
      AND cc.generation=cm.connection_generation
    WHERE lm.lesson_id=NEW.lesson_id AND lm.media_id=NEW.media_id
      AND cm.author_id=lesson_author AND cm.provider='YANDEX_DISK'
      AND cm.resource_id IS NOT NULL AND cm.sha256=NEW.content_sha256
      AND cm.file_size=NEW.content_length
  ) INTO media_matches;
  IF NOT media_matches THEN
    RAISE EXCEPTION 'academy playback media fingerprint or author assignment mismatch';
  END IF;

  SELECT EXISTS (
    SELECT 1 FROM connector_instances ci
    JOIN academy_video_provider_certifications cert
      ON cert.connector_instance_id=ci.id
    WHERE ci.id=NEW.provider_instance_id
      AND ci.capability_class='CDN_PROVIDER'
      AND ci.provider_kind=NEW.provider_kind
      AND ci.status='ACTIVE'
      AND cert.contract_version=NEW.contract_version
      AND cert.provider_kind=NEW.provider_kind
      AND cert.key_id=NEW.key_id
      AND cert.tested_source_sha=NULLIF(current_setting('apgic.release_source_sha',true),'')
      AND cert.expires_at>now()
  ) INTO provider_matches;
  IF NOT provider_matches THEN
    RAISE EXCEPTION 'academy playback requires the certified active CDN provider instance';
  END IF;
  IF btrim(NEW.provider_event_id)='' OR btrim(NEW.evidence_ref)='' THEN
    RAISE EXCEPTION 'academy playback requires provider event and signed evidence reference';
  END IF;
  RETURN NEW;
END;
$fn$;

CREATE OR REPLACE FUNCTION academy_progress_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE lesson_kind text;
BEGIN
  SELECT l.kind INTO lesson_kind
  FROM academy_enrollments e
  JOIN academy_modules m ON m.version_id=e.version_id
  JOIN academy_lessons l ON l.module_id=m.id
  JOIN store_entitlements t ON t.id=e.entitlement_id
  WHERE e.id=NEW.enrollment_id AND l.id=NEW.lesson_id
    AND e.state='ACTIVE' AND academy_entitlement_is_active(t.id);
  IF NOT FOUND THEN
    RAISE EXCEPTION 'academy progress requires active matching enrollment and entitlement';
  END IF;
  IF lesson_kind='VIDEO' AND NOT EXISTS (
    SELECT 1 FROM academy_media_playback_facts f
    WHERE f.enrollment_id=NEW.enrollment_id AND f.lesson_id=NEW.lesson_id
      AND f.contract_version='cdn-playback-v1'
      AND f.evidence_ref=NEW.evidence_ref
      AND f.attestation_sha256 IS NOT NULL
      AND f.watched_seconds>=f.required_seconds
  ) THEN
    RAISE EXCEPTION 'academy video progress requires persisted complete provider attestation';
  END IF;
  IF lesson_kind='ASSESSMENT' AND NOT EXISTS (
    SELECT 1 FROM academy_attempts a WHERE a.enrollment_id=NEW.enrollment_id
      AND a.lesson_id=NEW.lesson_id AND a.result='PASSED'
  ) THEN
    RAISE EXCEPTION 'academy assessment requires a human-evaluated passing attempt';
  END IF;
  RETURN NEW;
END;
$fn$;

CREATE OR REPLACE FUNCTION academy_version_publish_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
BEGIN
  IF OLD.state='PUBLISHED' AND NEW.state NOT IN ('PUBLISHED','RETIRED') THEN
    RAISE EXCEPTION 'published version cannot become draft';
  END IF;
  IF OLD.state='RETIRED' AND NEW.state<>'RETIRED' THEN
    RAISE EXCEPTION 'retired version cannot be reopened';
  END IF;
  IF NEW.course_id<>OLD.course_id OR NEW.version_no<>OLD.version_no OR
    NEW.completion_policy_version<>OLD.completion_policy_version OR NEW.created_at<>OLD.created_at THEN
    RAISE EXCEPTION 'academy version identity and policy are immutable';
  END IF;
  IF OLD.state<>'DRAFT' AND (NEW.title_snapshot IS DISTINCT FROM OLD.title_snapshot OR
    NEW.description_snapshot IS DISTINCT FROM OLD.description_snapshot) THEN
    RAISE EXCEPTION 'academy version published content provenance is immutable';
  END IF;
  IF OLD.state<>'DRAFT' AND NEW.published_at IS DISTINCT FROM OLD.published_at THEN
    RAISE EXCEPTION 'academy publication evidence is immutable';
  END IF;
  IF OLD.state='DRAFT' AND NEW.state='PUBLISHED' AND NOT EXISTS (
    SELECT 1 FROM academy_original_rights r
    JOIN academy_courses c ON c.id=NEW.course_id
    WHERE r.version_id=NEW.id AND r.author_id=c.author_id
      AND r.curriculum_sha256=academy_curriculum_fingerprint(NEW.id)
  ) THEN
    RAISE EXCEPTION 'academy author ownership declaration required for the exact curriculum';
  END IF;
  IF OLD.state='DRAFT' AND NEW.state='PUBLISHED' AND EXISTS (
    SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
    WHERE m.version_id=NEW.id AND l.kind='VIDEO'
  ) THEN
    IF NOT academy_video_provider_certification_available(
      NULLIF(current_setting('apgic.release_source_sha',true),'')
    ) THEN
      RAISE EXCEPTION 'academy video publication blocked without durable CDN provider acceptance for this source SHA';
    END IF;
    IF EXISTS (
      SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
      WHERE m.version_id=NEW.id AND l.kind='VIDEO' AND NOT EXISTS (
        SELECT 1 FROM academy_lesson_media lm
        JOIN academy_cloud_media cm ON cm.id=lm.media_id
        JOIN academy_cloud_connections cc
          ON cc.author_id=cm.author_id AND cc.provider=cm.provider
          AND cc.generation=cm.connection_generation
        JOIN academy_courses c ON c.id=NEW.course_id
        WHERE lm.lesson_id=l.id AND cm.author_id=c.author_id
          AND cm.provider='YANDEX_DISK' AND cm.resource_id IS NOT NULL
          AND cm.sha256 IS NOT NULL AND cm.file_size>0
      )
    ) THEN
      RAISE EXCEPTION 'academy video publication requires a verified current author media assignment for every video lesson';
    END IF;
    IF EXISTS (
      SELECT 1 FROM academy_lesson_media lm
      JOIN academy_lessons l ON l.id=lm.lesson_id
      JOIN academy_modules m ON m.id=l.module_id
      JOIN academy_cloud_media cm ON cm.id=lm.media_id
      LEFT JOIN academy_cloud_connections cc
        ON cc.author_id=cm.author_id AND cc.provider=cm.provider
        AND cc.generation=cm.connection_generation
      WHERE m.version_id=NEW.id AND l.kind='VIDEO'
        AND (cm.provider<>'YANDEX_DISK' OR cm.resource_id IS NULL
          OR cm.sha256 IS NULL OR cm.file_size IS NULL OR cm.file_size<=0
          OR cc.generation IS NULL)
    ) THEN
      RAISE EXCEPTION 'academy video publication found a stale or unverified media assignment';
    END IF;
  END IF;
  IF OLD.state='DRAFT' AND NEW.state='PUBLISHED' AND NOT EXISTS (
    SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
    WHERE m.version_id=NEW.id
  ) THEN
    RAISE EXCEPTION 'cannot publish course without lessons';
  END IF;
  RETURN NEW;
END;
$fn$;

      OR length(start_text) > 19 OR length(end_text) > 19 THEN
      RETURN false;
    END IF;

    start_value := start_text::numeric;
    end_value := end_text::numeric;
    IF start_value > 9223372036854775807::numeric
      OR end_value > 9223372036854775807::numeric THEN
      RETURN false;
    END IF;
    start_millis := start_value::bigint;
    end_millis := end_value::bigint;

    IF start_millis <> previous_end
      OR end_millis <= start_millis
      OR end_millis > p_duration_millis THEN
      RETURN false;
    END IF;
    previous_end := end_millis;
  END LOOP;

  RETURN item_count > 0 AND previous_end = p_duration_millis;
END;
$fn$;

CREATE OR REPLACE FUNCTION academy_media_fact_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE
  enrollment_version uuid;
  enrollment_identity uuid;
  enrollment_entitlement uuid;
  enrollment_state text;
  lesson_version uuid;
  lesson_kind text;
  lesson_course uuid;
  lesson_author uuid;
  media_matches boolean;
  provider_matches boolean;
BEGIN
  SELECT e.version_id,e.identity_id,e.entitlement_id,e.state
    INTO enrollment_version,enrollment_identity,enrollment_entitlement,enrollment_state
  FROM academy_enrollments e
  JOIN store_entitlements t ON t.id=e.entitlement_id
  WHERE e.id=NEW.enrollment_id AND academy_entitlement_is_active(t.id)
  FOR UPDATE OF e,t;
  IF NOT FOUND OR enrollment_state<>'ACTIVE' THEN
    RAISE EXCEPTION 'academy playback evidence requires an active verified enrollment';
  END IF;

  SELECT m.version_id,l.kind,v.course_id,c.author_id
    INTO lesson_version,lesson_kind,lesson_course,lesson_author
  FROM academy_lessons l
  JOIN academy_modules m ON m.id=l.module_id
  JOIN academy_course_versions v ON v.id=m.version_id
  JOIN academy_courses c ON c.id=v.course_id
  WHERE l.id=NEW.lesson_id;
  IF NOT FOUND OR lesson_kind<>'VIDEO'
    OR lesson_version IS DISTINCT FROM enrollment_version
    OR NEW.contract_version IS DISTINCT FROM 'cdn-playback-v1'
    OR NEW.course_id IS DISTINCT FROM lesson_course
    OR NEW.course_version_id IS DISTINCT FROM enrollment_version
    OR NEW.subject_id IS DISTINCT FROM enrollment_identity
    OR NEW.entitlement_id IS DISTINCT FROM enrollment_entitlement
    OR NEW.signed_attestation IS NULL OR NEW.attestation_sha256 IS NULL
    OR NEW.key_id IS NULL OR NEW.session_id IS NULL
    OR NEW.media_id IS NULL OR NEW.content_sha256 IS NULL
    OR NEW.content_length IS NULL OR NEW.duration_millis IS NULL
    OR NEW.watched_ranges IS NULL THEN
    RAISE EXCEPTION 'academy playback evidence does not match the active video lesson binding';
  END IF;

  IF NEW.watched_seconds<NEW.required_seconds
    OR NEW.required_seconds<>ceil(NEW.duration_millis::numeric/1000)::bigint
    OR NOT academy_video_ranges_cover_duration(NEW.watched_ranges,NEW.duration_millis)
    OR NEW.occurred_at IS DISTINCT FROM (NEW.signed_attestation->>'completed_at')::timestamptz
    OR NEW.evidence_ref IS DISTINCT FROM 'sha256:'||NEW.attestation_sha256 THEN
    RAISE EXCEPTION 'academy playback evidence is not a complete signed timeline';
  END IF;

  SELECT EXISTS (
    SELECT 1 FROM academy_lesson_media lm
    JOIN academy_cloud_media cm ON cm.id=lm.media_id
    JOIN academy_cloud_connections cc
      ON cc.author_id=cm.author_id AND cc.provider=cm.provider
      AND cc.generation=cm.connection_generation
    WHERE lm.lesson_id=NEW.lesson_id AND lm.media_id=NEW.media_id
      AND cm.author_id=lesson_author AND cm.provider='YANDEX_DISK'
      AND cm.resource_id IS NOT NULL AND cm.sha256=NEW.content_sha256
      AND cm.file_size=NEW.content_length
  ) INTO media_matches;
  IF NOT media_matches THEN
    RAISE EXCEPTION 'academy playback media fingerprint or author assignment mismatch';
  END IF;

  SELECT EXISTS (
    SELECT 1 FROM connector_instances ci
    JOIN academy_video_provider_certifications cert
      ON cert.connector_instance_id=ci.id
    WHERE ci.id=NEW.provider_instance_id
      AND ci.capability_class='CDN_PROVIDER'
      AND ci.provider_kind=NEW.provider_kind
      AND ci.status='ACTIVE'
      AND cert.contract_version=NEW.contract_version
      AND cert.provider_kind=NEW.provider_kind
      AND cert.key_id=NEW.key_id
      AND cert.tested_source_sha=NULLIF(current_setting('apgic.release_source_sha',true),'')
      AND cert.expires_at>now()
  ) INTO provider_matches;
  IF NOT provider_matches THEN
    RAISE EXCEPTION 'academy playback requires the certified active CDN provider instance';
  END IF;
  IF btrim(NEW.provider_event_id)='' OR btrim(NEW.evidence_ref)='' THEN
    RAISE EXCEPTION 'academy playback requires provider event and signed evidence reference';
  END IF;
  RETURN NEW;
END;
$fn$;

CREATE OR REPLACE FUNCTION academy_progress_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE lesson_kind text;
BEGIN
  SELECT l.kind INTO lesson_kind
  FROM academy_enrollments e
  JOIN academy_modules m ON m.version_id=e.version_id
  JOIN academy_lessons l ON l.module_id=m.id
  JOIN store_entitlements t ON t.id=e.entitlement_id
  WHERE e.id=NEW.enrollment_id AND l.id=NEW.lesson_id
    AND e.state='ACTIVE' AND academy_entitlement_is_active(t.id);
  IF NOT FOUND THEN
    RAISE EXCEPTION 'academy progress requires active matching enrollment and entitlement';
  END IF;
  IF lesson_kind='VIDEO' AND NOT EXISTS (
    SELECT 1 FROM academy_media_playback_facts f
    WHERE f.enrollment_id=NEW.enrollment_id AND f.lesson_id=NEW.lesson_id
      AND f.contract_version='cdn-playback-v1'
      AND f.evidence_ref=NEW.evidence_ref
      AND f.attestation_sha256 IS NOT NULL
      AND f.watched_seconds>=f.required_seconds
  ) THEN
    RAISE EXCEPTION 'academy video progress requires persisted complete provider attestation';
  END IF;
  IF lesson_kind='ASSESSMENT' AND NOT EXISTS (
    SELECT 1 FROM academy_attempts a WHERE a.enrollment_id=NEW.enrollment_id
      AND a.lesson_id=NEW.lesson_id AND a.result='PASSED'
  ) THEN
    RAISE EXCEPTION 'academy assessment requires a human-evaluated passing attempt';
  END IF;
  RETURN NEW;
END;
$fn$;

CREATE OR REPLACE FUNCTION academy_version_publish_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
BEGIN
  IF OLD.state='PUBLISHED' AND NEW.state NOT IN ('PUBLISHED','RETIRED') THEN
    RAISE EXCEPTION 'published version cannot become draft';
  END IF;
  IF OLD.state='RETIRED' AND NEW.state<>'RETIRED' THEN
    RAISE EXCEPTION 'retired version cannot be reopened';
  END IF;
  IF NEW.course_id<>OLD.course_id OR NEW.version_no<>OLD.version_no OR
    NEW.completion_policy_version<>OLD.completion_policy_version OR NEW.created_at<>OLD.created_at THEN
    RAISE EXCEPTION 'academy version identity and policy are immutable';
  END IF;
  IF OLD.state<>'DRAFT' AND (NEW.title_snapshot IS DISTINCT FROM OLD.title_snapshot OR
    NEW.description_snapshot IS DISTINCT FROM OLD.description_snapshot) THEN
    RAISE EXCEPTION 'academy version published content provenance is immutable';
  END IF;
  IF OLD.state<>'DRAFT' AND NEW.published_at IS DISTINCT FROM OLD.published_at THEN
    RAISE EXCEPTION 'academy publication evidence is immutable';
  END IF;
  IF OLD.state='DRAFT' AND NEW.state='PUBLISHED' AND NOT EXISTS (
    SELECT 1 FROM academy_original_rights r
    JOIN academy_courses c ON c.id=NEW.course_id
    WHERE r.version_id=NEW.id AND r.author_id=c.author_id
      AND r.curriculum_sha256=academy_curriculum_fingerprint(NEW.id)
  ) THEN
    RAISE EXCEPTION 'academy author ownership declaration required for the exact curriculum';
  END IF;
  IF OLD.state='DRAFT' AND NEW.state='PUBLISHED' AND EXISTS (
    SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
    WHERE m.version_id=NEW.id AND l.kind='VIDEO'
  ) THEN
    IF NOT academy_video_provider_certification_available(
      NULLIF(current_setting('apgic.release_source_sha',true),'')
    ) THEN
      RAISE EXCEPTION 'academy video publication blocked without durable CDN provider acceptance for this source SHA';
    END IF;
    IF EXISTS (
      SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
      WHERE m.version_id=NEW.id AND l.kind='VIDEO' AND NOT EXISTS (
        SELECT 1 FROM academy_lesson_media lm
        JOIN academy_cloud_media cm ON cm.id=lm.media_id
        JOIN academy_cloud_connections cc
          ON cc.author_id=cm.author_id AND cc.provider=cm.provider
          AND cc.generation=cm.connection_generation
        JOIN academy_courses c ON c.id=NEW.course_id
        WHERE lm.lesson_id=l.id AND cm.author_id=c.author_id
          AND cm.provider='YANDEX_DISK' AND cm.resource_id IS NOT NULL
          AND cm.sha256 IS NOT NULL AND cm.file_size>0
      )
    ) THEN
      RAISE EXCEPTION 'academy video publication requires a verified current author media assignment for every video lesson';
    END IF;
    IF EXISTS (
      SELECT 1 FROM academy_lesson_media lm
      JOIN academy_lessons l ON l.id=lm.lesson_id
      JOIN academy_modules m ON m.id=l.module_id
      JOIN academy_cloud_media cm ON cm.id=lm.media_id
      LEFT JOIN academy_cloud_connections cc
        ON cc.author_id=cm.author_id AND cc.provider=cm.provider
        AND cc.generation=cm.connection_generation
      WHERE m.version_id=NEW.id AND l.kind='VIDEO'
        AND (cm.provider<>'YANDEX_DISK' OR cm.resource_id IS NULL
          OR cm.sha256 IS NULL OR cm.file_size IS NULL OR cm.file_size<=0
          OR cc.generation IS NULL)
    ) THEN
      RAISE EXCEPTION 'academy video publication found a stale or unverified media assignment';
    END IF;
  END IF;
  IF OLD.state='DRAFT' AND NEW.state='PUBLISHED' AND NOT EXISTS (
    SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
    WHERE m.version_id=NEW.id
  ) THEN
    RAISE EXCEPTION 'cannot publish course without lessons';
  END IF;
  RETURN NEW;
END;
$fn$;
