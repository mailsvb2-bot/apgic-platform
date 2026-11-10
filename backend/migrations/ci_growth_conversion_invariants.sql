\set ON_ERROR_STOP on
DO $proof$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='growth_hold_attribution_insert_guard' AND NOT tgisinternal)
  OR NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='growth_hold_attribution_append_only' AND NOT tgisinternal)
  THEN RAISE EXCEPTION 'Growth booking conversion provenance guard missing'; END IF;
END $proof$;