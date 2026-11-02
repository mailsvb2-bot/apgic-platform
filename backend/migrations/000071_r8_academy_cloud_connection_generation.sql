-- R8 VIDEO P0: bind verified cloud media to the OAuth connection that
-- actually verified the file. Legacy/unverified references remain unplayable.
ALTER TABLE academy_cloud_connections
  ADD COLUMN generation uuid NOT NULL DEFAULT gen_random_uuid();

ALTER TABLE academy_cloud_media
  ADD COLUMN connection_generation uuid NOT NULL
    DEFAULT '00000000-0000-0000-0000-000000000000'::uuid,
  ADD COLUMN resource_id text,
  ADD COLUMN sha256 text,
  ADD COLUMN file_size bigint;

ALTER TABLE academy_cloud_media
  ADD CONSTRAINT academy_cloud_verified_identity_ck CHECK (
    (connection_generation = '00000000-0000-0000-0000-000000000000'::uuid
      AND resource_id IS NULL AND sha256 IS NULL AND file_size IS NULL)
    OR
    (connection_generation <> '00000000-0000-0000-0000-000000000000'::uuid
      AND provider = 'YANDEX_DISK'
      AND resource_id IS NOT NULL AND length(resource_id) BETWEEN 1 AND 512
      AND sha256 IS NOT NULL AND sha256 ~ '^[0-9a-fA-F]{64}$'
      AND file_size IS NOT NULL AND file_size > 0)
  );

-- Reimport after OAuth reconnect must create a different immutable asset.
ALTER TABLE academy_cloud_media
  DROP CONSTRAINT academy_cloud_media_author_id_provider_provider_file_ref_key;
CREATE UNIQUE INDEX academy_cloud_media_generation_identity_idx
  ON academy_cloud_media(author_id, provider, provider_file_ref, connection_generation);

-- Preserve the existing trigger created by migration 000069, extending it
-- to reject changes to the verified resource identity, digest and generation.
CREATE OR REPLACE FUNCTION academy_cloud_media_identity_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.id IS DISTINCT FROM OLD.id
     OR NEW.author_id IS DISTINCT FROM OLD.author_id
     OR NEW.provider IS DISTINCT FROM OLD.provider
     OR NEW.provider_file_ref IS DISTINCT FROM OLD.provider_file_ref
     OR NEW.source_url IS DISTINCT FROM OLD.source_url
     OR NEW.connection_generation IS DISTINCT FROM OLD.connection_generation
     OR NEW.resource_id IS DISTINCT FROM OLD.resource_id
     OR NEW.sha256 IS DISTINCT FROM OLD.sha256
     OR NEW.file_size IS DISTINCT FROM OLD.file_size THEN
    RAISE EXCEPTION 'academy cloud source identity is immutable';
  END IF;
  RETURN NEW;
END;
$$;
