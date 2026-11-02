-- R8 P0: bind every playable cloud reference to the OAuth connection generation
-- that actually verified it. Reconnecting a different Disk account cannot
-- reinterpret an old disk:/ path as another user's video.
ALTER TABLE academy_cloud_connections
  ADD COLUMN generation uuid NOT NULL DEFAULT gen_random_uuid();

-- Legacy catalog entries remain visible to their author but are unplayable
-- until reimported under a freshly verified connection generation.
ALTER TABLE academy_cloud_media
  ADD COLUMN connection_generation uuid NOT NULL
    DEFAULT '00000000-0000-0000-0000-000000000000'::uuid;

-- Reimports after reconnection create a distinct immutable catalog asset.
-- The old (author,provider,path) uniqueness would otherwise force a
-- previously verified asset to be silently rebound to a different account.
ALTER TABLE academy_cloud_media
  DROP CONSTRAINT academy_cloud_media_author_id_provider_provider_file_ref_key;
CREATE UNIQUE INDEX academy_cloud_media_generation_identity_idx
  ON academy_cloud_media(author_id,provider,provider_file_ref,connection_generation);

CREATE OR REPLACE FUNCTION academy_cloud_media_identity_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.id IS DISTINCT FROM OLD.id OR NEW.author_id IS DISTINCT FROM OLD.author_id
  OR NEW.provider IS DISTINCT FROM OLD.provider
  OR NEW.provider_file_ref IS DISTINCT FROM OLD.provider_file_ref
  OR NEW.source_url IS DISTINCT FROM OLD.source_url
  OR NEW.connection_generation IS DISTINCT FROM OLD.connection_generation THEN
   RAISE EXCEPTION 'academy cloud source identity is immutable';
 END IF;
 RETURN NEW;
END; $$;
