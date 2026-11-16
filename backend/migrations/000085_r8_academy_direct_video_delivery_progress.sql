-- Direct Yandex video delivery evidence lives in the existing Academy owner.
-- This proves complete server-side transfer of the verified file, NOT that
-- the learner watched the screen. The learner explicitly acknowledges study.
CREATE TABLE academy_direct_video_deliveries (
  id uuid PRIMARY KEY,
  enrollment_id uuid NOT NULL REFERENCES academy_enrollments(id),
  lesson_id uuid NOT NULL REFERENCES academy_lessons(id),
  media_id uuid NOT NULL REFERENCES academy_cloud_media(id),
  subject_id uuid NOT NULL REFERENCES identities(id),
  content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
  content_length bigint NOT NULL CHECK (content_length>0),
  start_byte bigint NOT NULL CHECK (start_byte>=0),
  end_byte bigint NOT NULL CHECK (end_byte>start_byte AND end_byte<=content_length),
  recorded_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(enrollment_id,lesson_id,media_id,content_sha256,content_length,start_byte,end_byte)
);

CREATE FUNCTION academy_direct_video_delivery_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE
  authorized boolean;
BEGIN
  SELECT true INTO authorized
  FROM academy_enrollments e
  JOIN store_entitlements t ON t.id=e.entitlement_id
  JOIN academy_course_versions v ON v.id=e.version_id AND v.state='PUBLISHED'
  JOIN academy_courses c ON c.id=v.course_id AND c.state='PUBLISHED'
  JOIN academy_modules m ON m.version_id=v.id
  JOIN academy_lessons l ON l.module_id=m.id AND l.kind='VIDEO'
  JOIN academy_lesson_media lm ON lm.lesson_id=l.id
  JOIN academy_cloud_media cm ON cm.id=lm.media_id AND cm.author_id=c.author_id
  JOIN academy_cloud_connections cc
    ON cc.author_id=cm.author_id AND cc.provider=cm.provider
    AND cc.generation=cm.connection_generation
  LEFT JOIN academy_lesson_schedules sc ON sc.lesson_id=l.id
  WHERE e.id=NEW.enrollment_id AND e.identity_id=NEW.subject_id
    AND e.state='ACTIVE' AND academy_entitlement_is_active(t.id)
    AND l.id=NEW.lesson_id AND cm.id=NEW.media_id
    AND cm.provider='YANDEX_DISK'
    AND cm.sha256=NEW.content_sha256 AND cm.file_size=NEW.content_length
    AND (sc.release_at IS NULL OR now()>=sc.release_at)
    AND (sc.deadline_at IS NULL OR now()<=sc.deadline_at)
  FOR UPDATE OF e,t;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'direct video delivery requires active entitled verified media binding';
  END IF;
  RETURN NEW;
END;
$fn$;
CREATE TRIGGER academy_direct_video_delivery_insert_guard
  BEFORE INSERT ON academy_direct_video_deliveries
  FOR EACH ROW EXECUTE FUNCTION academy_direct_video_delivery_guard();
CREATE TRIGGER academy_direct_video_delivery_immutable
  BEFORE UPDATE OR DELETE ON academy_direct_video_deliveries
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

-- All assigned files, not just any one file, must have their full verified
-- byte interval covered by completed server-side transfers.
CREATE FUNCTION academy_direct_video_delivery_complete(p_enrollment uuid,p_lesson uuid)
RETURNS boolean LANGUAGE sql STABLE AS $fn$
  SELECT EXISTS (
    SELECT 1 FROM academy_enrollments e
    JOIN academy_modules m ON m.version_id=e.version_id
    JOIN academy_lessons l ON l.module_id=m.id AND l.kind='VIDEO'
    JOIN academy_lesson_media lm ON lm.lesson_id=l.id
    WHERE e.id=p_enrollment AND l.id=p_lesson
  ) AND NOT EXISTS (
    SELECT 1
    FROM academy_enrollments e
    JOIN academy_modules m ON m.version_id=e.version_id
    JOIN academy_lessons l ON l.module_id=m.id AND l.kind='VIDEO'
    JOIN academy_lesson_media lm ON lm.lesson_id=l.id
    JOIN academy_cloud_media cm ON cm.id=lm.media_id
    JOIN academy_courses c ON c.id=e.course_id
    JOIN academy_cloud_connections cc
      ON cc.author_id=cm.author_id AND cc.provider=cm.provider
      AND cc.generation=cm.connection_generation
    WHERE e.id=p_enrollment AND l.id=p_lesson
      AND (
        cm.author_id<>c.author_id OR cm.provider<>'YANDEX_DISK'
        OR cm.sha256 IS NULL OR cm.file_size IS NULL OR cm.file_size<=0
        OR NOT (
          COALESCE((
            SELECT range_agg(int8range(d.start_byte,d.end_byte,'[)'))
            FROM academy_direct_video_deliveries d
            WHERE d.enrollment_id=e.id AND d.lesson_id=l.id
              AND d.media_id=cm.id AND d.subject_id=e.identity_id
              AND d.content_sha256=cm.sha256 AND d.content_length=cm.file_size
          ), '{}'::int8multirange) @> int8range(0,cm.file_size,'[)')
        )
      )
  );
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
  IF lesson_kind='VIDEO' AND NOT (
    EXISTS (
      SELECT 1 FROM academy_media_playback_facts f
      WHERE f.enrollment_id=NEW.enrollment_id AND f.lesson_id=NEW.lesson_id
        AND f.contract_version='cdn-playback-v2'
        AND f.evidence_ref=NEW.evidence_ref AND f.attestation_sha256 IS NOT NULL
        AND f.watched_seconds>=f.required_seconds
    )
    OR (
      NEW.evidence_ref LIKE 'academy/direct-delivery-v1/%'
      AND academy_direct_video_delivery_complete(NEW.enrollment_id,NEW.lesson_id)
    )
  ) THEN
    RAISE EXCEPTION 'academy video progress requires verified delivery or provider playback evidence';
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
