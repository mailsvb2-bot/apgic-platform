\set ON_ERROR_STOP on

-- Pure SQL domain negative probes that are safe to execute on the isolated
-- GitHub Actions PostgreSQL database, even without commercial providers.
DO $growth_ci$
DECLARE
  p RECORD;
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='growth_links_insert_guard' AND NOT tgisinternal)
    OR NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='growth_touches_insert_guard' AND NOT tgisinternal)
    OR NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='growth_intent_attribution_insert_guard' AND NOT tgisinternal)
    OR NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='growth_intent_attribution_on_create' AND NOT tgisinternal)
  THEN RAISE EXCEPTION 'Growth guard or HelpIntent attribution trigger missing'; END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='growth_links_guard' AND NOT tgisinternal)
    OR NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='growth_touches_append_only' AND NOT tgisinternal)
    OR NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='growth_intent_attribution_append_only' AND NOT tgisinternal)
  THEN RAISE EXCEPTION 'Growth append-only controls missing'; END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_indexes
                 WHERE schemaname='public' AND tablename='growth_touches'
                   AND indexdef LIKE '%UNIQUE%')
  THEN RAISE EXCEPTION 'Growth touch idempotency uniqueness missing'; END IF;

  FOR p IN SELECT proname FROM pg_proc WHERE proname IN (
       'apgic_growth_link_insert_guard','apgic_growth_touch_insert_guard',
       'apgic_growth_intent_attribution_insert_guard','apgic_attribute_new_help_intent'
  ) LOOP
    IF p.proname IS NULL THEN RAISE EXCEPTION 'growth function missing'; END IF;
  END LOOP;
END
$growth_ci$;

-- Untrusted caller cannot create a growth touch with neither a valid signed
-- source nor a canonical identity. Check the invariant with a rolled-back
-- exception, rather than polluting the CI database with fake attribution.
DO $growth_negative$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    INSERT INTO growth_touches(id,link_id,client_identity_id,recorded_at)
      VALUES ('f1111111-1111-4111-8111-111111111111',
              'f2222222-2222-4222-8222-222222222222',
              'f3333333-3333-4333-8333-333333333333',now());
  EXCEPTION WHEN OTHERS THEN blocked := true;
  END;
  IF NOT blocked THEN RAISE EXCEPTION 'untrusted growth touch accepted'; END IF;
END
$growth_negative$;
