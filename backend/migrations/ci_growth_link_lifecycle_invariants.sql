\set ON_ERROR_STOP on
-- Regression contract for durable distribution lifecycle across all API
-- instances and PostgreSQL restarts; not a mocked in-memory guarantee.
DO $growth_lifecycle$
DECLARE
  index_present boolean;
  active_duplicates bigint;
BEGIN
  SELECT EXISTS (
    SELECT 1 FROM pg_indexes
     WHERE schemaname='public'
       AND tablename='growth_links'
       AND indexname='growth_links_one_active_per_owner_offer'
  ) INTO index_present;
  IF NOT index_present THEN
    RAISE EXCEPTION 'growth owned link uniqueness guarantee missing';
  END IF;
  SELECT COUNT(*) INTO active_duplicates FROM (
    SELECT issuer_identity_id,specialist_offer_id
      FROM growth_links WHERE state='ACTIVE'
     GROUP BY issuer_identity_id,specialist_offer_id
    HAVING COUNT(*) > 1
  ) duplicates;
  IF active_duplicates <> 0 THEN
    RAISE EXCEPTION 'duplicate active links survive migration: %', active_duplicates;
  END IF;
END
$growth_lifecycle$;
