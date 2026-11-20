-- One canonical Academy course per signed owner/request, even when the HTTP
-- response is lost and the browser safely retries the same creation.
ALTER TABLE academy_courses
  ADD COLUMN author_request_id text;
CREATE UNIQUE INDEX academy_courses_author_request_id_unique
  ON academy_courses(author_id, author_request_id)
  WHERE author_request_id IS NOT NULL;

-- Authors may use short meaningful names such as №1.
ALTER TABLE academy_courses
  DROP CONSTRAINT academy_courses_title_check;
ALTER TABLE academy_courses
  ADD CONSTRAINT academy_courses_title_check
  CHECK (length(btrim(title)) BETWEEN 1 AND 200);

-- A request identity is assigned at create time and cannot be rebound to a
-- different course by later updates or publications.
CREATE FUNCTION academy_draft_request_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.author_request_id IS DISTINCT FROM OLD.author_request_id THEN
    RAISE EXCEPTION 'academy draft request identity is immutable';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER academy_draft_request_immutable
BEFORE UPDATE ON academy_courses
FOR EACH ROW EXECUTE FUNCTION academy_draft_request_immutable();
