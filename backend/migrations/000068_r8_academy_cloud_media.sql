-- Private author-managed cloud media references. No learner-facing URL projection.
CREATE TABLE academy_cloud_media (
 id uuid PRIMARY KEY,
 author_id uuid NOT NULL REFERENCES identities(id),
 provider text NOT NULL CHECK (provider IN ('YANDEX_DISK','EXTERNAL')),
 provider_file_ref text NOT NULL,
 source_url text,
 title text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (author_id, provider, provider_file_ref),
 CHECK (length(btrim(provider_file_ref)) BETWEEN 1 AND 2048),
 CHECK (length(btrim(title)) BETWEEN 1 AND 300)
);
CREATE TABLE academy_lesson_media (
 lesson_id uuid NOT NULL REFERENCES academy_lessons(id),
 media_id uuid NOT NULL REFERENCES academy_cloud_media(id),
 ordinal integer NOT NULL CHECK (ordinal > 0),
 PRIMARY KEY (lesson_id,media_id),
 UNIQUE (lesson_id,ordinal)
);
CREATE INDEX academy_lesson_media_media_idx ON academy_lesson_media(media_id);
