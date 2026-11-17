-- Prevent partial completion of a VIDEO lesson containing multiple CDN assets.
-- This augments the existing Academy/PostgreSQL authority; no new owner.
-- Each assigned file requires an independently signed, completed provider
-- playback fact bound to the current immutable content fingerprint.
CREATE FUNCTION academy_cdn_video_playback_complete(p_enrollment uuid,p_lesson uuid)
RETURNS boolean LANGUAGE sql STABLE AS $fn$
  SELECT EXISTS (
    SELECT 1 FROM academy_enrollments e
    JOIN academy_modules mod ON mod.version_id=e.version_id
    JOIN academy_lessons l ON l.module_id=mod.id AND l.kind='VIDEO'
    JOIN academy_lesson_media lm ON lm.lesson_id=l.id
    WHERE e.id=p_enrollment AND l.id=p_lesson
  ) AND NOT EXISTS (
    SELECT 1 FROM academy_enrollments e
    JOIN academy_modules mod ON mod.version_id=e.version_id
    JOIN academy_lessons l ON l.module_id=mod.id AND l.kind='VIDEO'
    JOIN academy_lesson_media lm ON lm.lesson_id=l.id
    JOIN academy_cloud_media media ON media.id=lm.media_id
    WHERE e.id=p_enrollment AND l.id=p_lesson
      AND (
        media.sha256 IS NULL OR media.file_size IS NULL OR media.file_size<=0
        OR NOT EXISTS (
          SELECT 1 FROM academy_media_playback_facts fact
          WHERE fact.enrollment_id=e.id AND fact.lesson_id=l.id
            AND fact.media_id=lm.media_id
            AND fact.subject_id=e.identity_id
            AND fact.contract_version='cdn-playback-v2'
            AND fact.content_sha256=media.sha256
            AND fact.content_length=media.file_size
            AND fact.attestation_sha256 IS NOT NULL
            AND fact.watched_seconds>=fact.required_seconds
        )
      )
  );
$fn$;

-- Preserve the direct-mode multi-video proof and human assessment proof.
-- The CDN branch now requires *all* assigned media; a receipt for just the
-- first video can never be used to mark the whole lesson complete.
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
    (
      EXISTS (
        SELECT 1 FROM academy_media_playback_facts f
        WHERE f.enrollment_id=NEW.enrollment_id AND f.lesson_id=NEW.lesson_id
          AND f.contract_version='cdn-playback-v2'
          AND f.evidence_ref=NEW.evidence_ref AND f.attestation_sha256 IS NOT NULL
          AND f.watched_seconds>=f.required_seconds
      )
      AND academy_cdn_video_playback_complete(NEW.enrollment_id,NEW.lesson_id)
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
