-- Make cloud-media references immutable after a course version is published,
-- including writes performed outside the HTTP API.
CREATE FUNCTION academy_lesson_media_draft_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  target_lesson uuid;
  version_state text;
  lesson_author uuid;
  asset_author uuid;
BEGIN
  IF TG_OP='UPDATE' AND (OLD.lesson_id IS DISTINCT FROM NEW.lesson_id OR OLD.media_id IS DISTINCT FROM NEW.media_id) THEN
    RAISE EXCEPTION 'academy media attachment identity is immutable';
  END IF;
  target_lesson := CASE WHEN TG_OP='DELETE' THEN OLD.lesson_id ELSE NEW.lesson_id END;
  SELECT v.state,c.author_id INTO version_state,lesson_author
  FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
  JOIN academy_course_versions v ON v.id=m.version_id
  JOIN academy_courses c ON c.id=v.course_id
  WHERE l.id=target_lesson FOR SHARE OF v;
  IF version_state IS DISTINCT FROM 'DRAFT' THEN
    RAISE EXCEPTION 'academy media assignments are immutable outside draft';
  END IF;
  IF TG_OP<>'DELETE' THEN
    SELECT author_id INTO asset_author FROM academy_cloud_media WHERE id=NEW.media_id;
    IF asset_author IS DISTINCT FROM lesson_author THEN
      RAISE EXCEPTION 'academy media author mismatch';
    END IF;
  END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END; $$;

CREATE TRIGGER academy_lesson_media_write_guard
BEFORE INSERT OR UPDATE OR DELETE ON academy_lesson_media
FOR EACH ROW EXECUTE FUNCTION academy_lesson_media_draft_guard();

-- Never rewrite the cloud file identity after its initial import; changing the
-- display name is permitted, but must not silently switch lesson content.
CREATE FUNCTION academy_cloud_media_identity_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.id IS DISTINCT FROM OLD.id OR NEW.author_id IS DISTINCT FROM OLD.author_id
  OR NEW.provider IS DISTINCT FROM OLD.provider
  OR NEW.provider_file_ref IS DISTINCT FROM OLD.provider_file_ref
  OR NEW.source_url IS DISTINCT FROM OLD.source_url THEN
   RAISE EXCEPTION 'academy cloud source identity is immutable';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER academy_cloud_media_identity_guard BEFORE UPDATE ON academy_cloud_media
FOR EACH ROW EXECUTE FUNCTION academy_cloud_media_identity_guard();
