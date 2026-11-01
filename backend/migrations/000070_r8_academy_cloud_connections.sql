-- Encrypted credential storage is separate from author-visible cloud media.
-- Only ciphertext is persisted; no Academy learner query joins this table.
CREATE TABLE academy_cloud_connections (
 author_id uuid NOT NULL REFERENCES identities(id),
 provider text NOT NULL CHECK (provider='YANDEX_DISK'),
 encrypted_access_token text NOT NULL CHECK (length(encrypted_access_token) BETWEEN 50 AND 16384),
 connected_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (author_id,provider)
);
REVOKE ALL ON academy_cloud_connections FROM PUBLIC;
