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
