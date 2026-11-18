-- R8: course publication represents an immutable author curriculum, not
-- approval to sell or deliver learner video. Keep the same canonical trigger.
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
  -- Academic publication is independent of external media delivery and sales.
  -- Existing ownership, content, immutability and rights checks remain.
  IF OLD.state='DRAFT' AND NEW.state='PUBLISHED' AND NOT EXISTS (
    SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
    WHERE m.version_id=NEW.id
  ) THEN
    RAISE EXCEPTION 'cannot publish course without lessons';
  END IF;
  RETURN NEW;
END;
$fn$;

-- Learner media admission remains fail-closed: release approval and every
-- video assignment must be independently verified before access is advertised.
-- Transaction-local release mode/SHA are set only by the canonical Academy
-- caller; an unset context must not enable learner video.
CREATE OR REPLACE FUNCTION academy_video_learner_delivery_ready(p_version_id uuid)
RETURNS boolean LANGUAGE plpgsql STABLE AS $fn$
DECLARE has_video boolean;
BEGIN
 SELECT EXISTS (SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
                WHERE m.version_id=p_version_id AND l.kind='VIDEO') INTO has_video;
 IF NOT has_video THEN RETURN true; END IF;
 IF NOT academy_video_delivery_release_available(
   NULLIF(current_setting('apgic.release_source_sha',true),'')
 ) THEN RETURN false; END IF;
 RETURN NOT EXISTS (
   SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
   WHERE m.version_id=p_version_id AND l.kind='VIDEO' AND NOT EXISTS (
     SELECT 1 FROM academy_lesson_media lm
     JOIN academy_cloud_media cm ON cm.id=lm.media_id
     JOIN academy_course_versions v ON v.id=m.version_id
     JOIN academy_courses c ON c.id=v.course_id
     JOIN academy_cloud_connections cc ON cc.author_id=c.author_id
       AND cc.provider=cm.provider AND cc.generation=cm.connection_generation
     WHERE lm.lesson_id=l.id AND cm.author_id=c.author_id
       AND cm.provider='YANDEX_DISK' AND cm.provider_file_ref LIKE 'disk:/%'
       AND cm.resource_id IS NOT NULL AND cm.sha256 IS NOT NULL AND cm.file_size>0
   )
 ) AND NOT EXISTS (
   SELECT 1 FROM academy_lesson_media lm
   JOIN academy_lessons l ON l.id=lm.lesson_id
   JOIN academy_modules m ON m.id=l.module_id
   JOIN academy_cloud_media cm ON cm.id=lm.media_id
   JOIN academy_course_versions v ON v.id=m.version_id
     JOIN academy_courses c ON c.id=v.course_id
   LEFT JOIN academy_cloud_connections cc ON cc.author_id=c.author_id
     AND cc.provider=cm.provider AND cc.generation=cm.connection_generation
   WHERE m.version_id=p_version_id AND l.kind='VIDEO'
     AND (cm.author_id<>c.author_id OR cm.provider<>'YANDEX_DISK'
       OR cc.generation IS NULL OR cm.resource_id IS NULL
       OR cm.sha256 IS NULL OR cm.file_size IS NULL OR cm.file_size<=0)
 );
END;
$fn$;
