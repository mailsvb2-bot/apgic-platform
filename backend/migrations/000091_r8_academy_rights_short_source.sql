-- Specialist may identify original materials with any non-empty source label,
-- including a single character or `№1`. Existing attestations remain immutable.
BEGIN;
ALTER TABLE academy_original_rights
 DROP CONSTRAINT academy_original_rights_provenance_ref_check;
ALTER TABLE academy_original_rights
 ADD CONSTRAINT academy_original_rights_provenance_ref_check
 CHECK (length(btrim(provenance_ref)) BETWEEN 1 AND 2048);
COMMIT;
