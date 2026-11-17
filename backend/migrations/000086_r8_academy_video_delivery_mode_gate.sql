-- Keep one canonical Academy publication gate and preserve the reusable CDN
-- certification path for a future connected CDN runtime. Mode is a bounded
-- transaction-local value set by the Academy owner, never by a client.
CREATE FUNCTION academy_video_delivery_release_available(p_source_sha text)
RETURNS boolean LANGUAGE sql STABLE AS $fn$
  SELECT CASE current_setting('apgic.academy_video_delivery_mode',true)
    WHEN 'direct' THEN academy_video_direct_yandex_acceptance_available(p_source_sha)
    WHEN 'cdn' THEN academy_video_provider_certification_available(p_source_sha)
    ELSE false
  END;
$fn$;

-- Preserve all original rights, lesson assignment and version immutability
-- checks while routing only the release-acceptance predicate through the
-- selected canonical transport. An unset mode always fails closed.
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
    IF NOT academy_video_delivery_release_available(
      NULLIF(current_setting('apgic.release_source_sha',true),'')
    ) THEN
      RAISE EXCEPTION 'academy video publication blocked without audited acceptance for selected delivery mode and source SHA';
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
