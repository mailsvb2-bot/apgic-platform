-- A draft VIDEO lesson is an outline node first. Its playable source is
-- attached later through the verified academy_lesson_media relation.
-- Publication remains independently blocked by the canonical R8 playback gate.
DO $migration$
DECLARE
  legacy_constraint text;
BEGIN
  SELECT c.conname INTO legacy_constraint
  FROM pg_constraint c
  WHERE c.conrelid = 'academy_lessons'::regclass
    AND c.contype = 'c'
    AND (
      SELECT count(DISTINCT a.attname)
      FROM unnest(c.conkey) AS key(attnum)
      JOIN pg_attribute a
        ON a.attrelid = c.conrelid AND a.attnum = key.attnum
      WHERE a.attname IN ('kind','media_asset_ref')
    ) = 2
    AND pg_get_constraintdef(c.oid) ILIKE '%VIDEO%'
    AND pg_get_constraintdef(c.oid) ILIKE '%media_asset_ref IS NOT NULL%';

  IF legacy_constraint IS NULL THEN
    RAISE EXCEPTION 'Expected legacy VIDEO media_asset_ref requirement was not found';
  END IF;

  EXECUTE format('ALTER TABLE academy_lessons DROP CONSTRAINT %I', legacy_constraint);
END
$migration$;

-- Keep the legacy field opaque if it is populated by an older trusted writer;
-- raw URLs remain prohibited by the independent constraint from migration 60.
