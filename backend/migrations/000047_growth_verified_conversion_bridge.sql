-- Canonical conversion attribution requires an explicitly owned, verified
-- HelpIntent linked to the actual, same-specialist booking hold.
CREATE TABLE growth_hold_attribution (
  hold_id uuid PRIMARY KEY REFERENCES booking_holds(id),
  intent_id uuid NOT NULL REFERENCES help_intents(id),
  touch_id uuid NOT NULL REFERENCES growth_touches(id),
  link_id uuid NOT NULL REFERENCES growth_links(id),
  source text NOT NULL CHECK (source = 'SPECIALIST_SOURCED'),
  recorded_at timestamptz NOT NULL
);
CREATE INDEX growth_hold_attribution_link_idx ON growth_hold_attribution(link_id,recorded_at);

CREATE FUNCTION apgic_growth_hold_attribution_insert_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE eligible boolean;
BEGIN
  SELECT TRUE INTO eligible
    FROM booking_holds h
    JOIN booking_slots slot ON slot.id=h.slot_id
    JOIN help_intents i ON i.id=NEW.intent_id
    JOIN growth_intent_attribution a ON a.intent_id=i.id
    JOIN growth_links l ON l.id=a.link_id
   WHERE h.id=NEW.hold_id
     AND h.client_identity_id=i.identity_id
     AND i.state='CONFIRMED'
     AND a.touch_id=NEW.touch_id
     AND a.link_id=NEW.link_id
     AND l.acquisition_source=NEW.source
     AND slot.tenant_scope='specialist/live'
     AND slot.specialist_offer_id=l.specialist_offer_id
     AND NEW.recorded_at>=h.created_at
     AND NEW.recorded_at<=h.expires_at;
  IF eligible IS DISTINCT FROM TRUE THEN
    RAISE EXCEPTION 'Growth conversion requires matching verified client intent, offer and hold';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER growth_hold_attribution_insert_guard
BEFORE INSERT ON growth_hold_attribution
FOR EACH ROW EXECUTE FUNCTION apgic_growth_hold_attribution_insert_guard();
CREATE TRIGGER growth_hold_attribution_append_only
BEFORE UPDATE OR DELETE ON growth_hold_attribution
FOR EACH ROW EXECUTE FUNCTION apgic_growth_append_only();
