-- Growth acquisition integrity (independent of the HTTP caller).
-- Keep all business truth inside APGIC; don't let a connector bypass
-- qualification, session ownership or an immutable referral source.
CREATE FUNCTION apgic_growth_link_insert_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.acquisition_source <> 'SPECIALIST_SOURCED'
    OR NEW.state <> 'ACTIVE'
    OR NEW.revoked_at IS NOT NULL
    OR NOT EXISTS (
      SELECT 1 FROM specialist_bookable_offers o
       WHERE o.id = NEW.specialist_offer_id
         AND o.specialist_identity_id = NEW.issuer_identity_id
         AND apgic_live_offer_valid(o.id)
    )
  THEN
    RAISE EXCEPTION 'growth issuer requires owned qualified live offer';
  END IF;
  IF NEW.created_at > clock_timestamp() + interval '5 seconds'
    OR NEW.created_at < clock_timestamp() - interval '1 minute'
    OR NEW.expires_at <= NEW.created_at
    OR NEW.expires_at > NEW.created_at + interval '30 days'
  THEN
    RAISE EXCEPTION 'invalid growth source validity window';
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER growth_links_insert_guard
BEFORE INSERT ON growth_links
FOR EACH ROW EXECUTE FUNCTION apgic_growth_link_insert_guard();

CREATE FUNCTION apgic_growth_touch_insert_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  issuer uuid;
  offer uuid;
  valid_until timestamptz;
  created timestamptz;
BEGIN
  -- Lock the provenance row, so concurrent revocation cannot overtake
  -- an already verified attribution write.
  SELECT l.issuer_identity_id, l.specialist_offer_id, l.expires_at, l.created_at
    INTO issuer, offer, valid_until, created
    FROM growth_links l
   WHERE l.id = NEW.link_id AND l.state = 'ACTIVE'
   FOR SHARE OF l;
  IF issuer IS NULL
    OR issuer = NEW.client_identity_id
    OR NOT apgic_live_offer_valid(offer)
    OR NEW.recorded_at < created
    OR NEW.recorded_at >= valid_until
    OR NEW.recorded_at > clock_timestamp() + interval '5 seconds'
    OR NEW.recorded_at < clock_timestamp() - interval '1 minute'
  THEN
    RAISE EXCEPTION 'growth touch requires active source and distinct client';
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER growth_touches_insert_guard
BEFORE INSERT ON growth_touches
FOR EACH ROW EXECUTE FUNCTION apgic_growth_touch_insert_guard();

CREATE FUNCTION apgic_growth_intent_attribution_insert_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  valid_link boolean;
BEGIN
  SELECT TRUE INTO valid_link
    FROM help_intents i
    JOIN growth_touches t
      ON t.client_identity_id = i.identity_id
    JOIN growth_links l
      ON l.id = t.link_id
   WHERE i.id = NEW.intent_id
     AND t.id = NEW.touch_id
     AND l.id = NEW.link_id
     AND NEW.acquisition_source = l.acquisition_source
     AND NEW.captured_at = i.created_at
     AND t.recorded_at <= i.created_at
     AND t.recorded_at >= i.created_at - interval '30 days';
  IF valid_link IS DISTINCT FROM TRUE THEN
    RAISE EXCEPTION 'growth intent attribution must match first party identity and canonical time';
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER growth_intent_attribution_insert_guard
BEFORE INSERT ON growth_intent_attribution
FOR EACH ROW EXECUTE FUNCTION apgic_growth_intent_attribution_insert_guard();
