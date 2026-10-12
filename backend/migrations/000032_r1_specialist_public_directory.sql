-- Reviewed public biography is a projection of the existing Specialist owner,
-- never a second specialist account, credential store or qualification source.
BEGIN;
CREATE TABLE specialist_public_details (
 specialist_id uuid PRIMARY KEY REFERENCES specialist_profiles(id) ON DELETE RESTRICT,
 headline text NOT NULL DEFAULT '' CHECK (char_length(headline)<=120),
 about text NOT NULL DEFAULT '' CHECK (char_length(about)<=3000),
 experience_years smallint NOT NULL DEFAULT 0 CHECK (experience_years BETWEEN 0 AND 75),
 languages text[] NOT NULL DEFAULT ARRAY['ru']::text[] CHECK (cardinality(languages) BETWEEN 1 AND 8),
 limitations text[] NOT NULL DEFAULT ARRAY[]::text[] CHECK (cardinality(limitations)<=12),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX specialist_public_details_updated_idx ON specialist_public_details(updated_at);
CREATE TRIGGER specialist_public_details_no_delete
BEFORE DELETE ON specialist_public_details
FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();
COMMIT;
