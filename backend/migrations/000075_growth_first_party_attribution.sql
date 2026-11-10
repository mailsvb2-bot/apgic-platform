-- Growth v1: immutable first-party acquisition provenance. Never store personal
-- consultation content, cookies, advertising audience traits, or raw HelpIntent.
CREATE TABLE growth_links (
  id uuid PRIMARY KEY,
  issuer_identity_id uuid NOT NULL REFERENCES identities(id),
  specialist_offer_id uuid NOT NULL REFERENCES specialist_bookable_offers(id),
  acquisition_source text NOT NULL CHECK (acquisition_source = 'SPECIALIST_SOURCED'),
  policy_version text NOT NULL CHECK (btrim(policy_version) <> ''),
  state text NOT NULL DEFAULT 'ACTIVE' CHECK (state IN ('ACTIVE','REVOKED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  CHECK (expires_at > created_at AND expires_at <= created_at + interval '30 days 1 minute'),
  CHECK ((state = 'ACTIVE' AND revoked_at IS NULL) OR (state = 'REVOKED' AND revoked_at IS NOT NULL))
);
CREATE INDEX growth_links_owner_idx ON growth_links (issuer_identity_id,created_at DESC);
CREATE FUNCTION apgic_growth_link_immutability() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='DELETE' THEN RAISE EXCEPTION 'growth link deletion forbidden'; END IF;
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.issuer_identity_id IS DISTINCT FROM OLD.issuer_identity_id
    OR NEW.specialist_offer_id IS DISTINCT FROM OLD.specialist_offer_id
    OR NEW.acquisition_source IS DISTINCT FROM OLD.acquisition_source
    OR NEW.policy_version IS DISTINCT FROM OLD.policy_version
    OR NEW.created_at IS DISTINCT FROM OLD.created_at
    OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
    OR OLD.state='REVOKED' OR (NEW.state='REVOKED' AND NEW.revoked_at IS NULL)
    OR (NEW.state='ACTIVE' AND NEW.revoked_at IS NOT NULL)
  THEN RAISE EXCEPTION 'growth link provenance and revocation are immutable'; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER growth_links_guard BEFORE UPDATE OR DELETE ON growth_links FOR EACH ROW EXECUTE FUNCTION apgic_growth_link_immutability();

CREATE TABLE growth_touches (
  id uuid PRIMARY KEY,
  link_id uuid NOT NULL REFERENCES growth_links(id),
  client_identity_id uuid NOT NULL REFERENCES identities(id),
  recorded_at timestamptz NOT NULL,
  UNIQUE (link_id,client_identity_id)
);
CREATE INDEX growth_touches_client_idx ON growth_touches (client_identity_id,recorded_at);
CREATE FUNCTION apgic_growth_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'growth attribution evidence is append-only';
END $$;
CREATE TRIGGER growth_touches_append_only BEFORE UPDATE OR DELETE ON growth_touches
  FOR EACH ROW EXECUTE FUNCTION apgic_growth_append_only();

CREATE TABLE growth_intent_attribution (
  intent_id uuid PRIMARY KEY REFERENCES help_intents(id),
  touch_id uuid NOT NULL REFERENCES growth_touches(id),
  link_id uuid NOT NULL REFERENCES growth_links(id),
  acquisition_source text NOT NULL CHECK (acquisition_source = 'SPECIALIST_SOURCED'),
  captured_at timestamptz NOT NULL
);
CREATE INDEX growth_intent_attribution_link_idx ON growth_intent_attribution (link_id,captured_at);
CREATE TRIGGER growth_intent_attribution_append_only BEFORE UPDATE OR DELETE ON growth_intent_attribution
  FOR EACH ROW EXECUTE FUNCTION apgic_growth_append_only();

-- A canonical client HelpIntent is captured in the same transaction as
-- origin attribution. Deterministic first valid touch prevents overwrite by
-- later advertising clicks, while follow-up orders remain a different truth.
CREATE FUNCTION apgic_attribute_new_help_intent() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.identity_id IS NULL THEN RETURN NEW; END IF;
  INSERT INTO growth_intent_attribution(intent_id,touch_id,link_id,acquisition_source,captured_at)
  SELECT NEW.id,t.id,t.link_id,l.acquisition_source,NEW.created_at
    FROM growth_touches t
    JOIN growth_links l ON l.id=t.link_id
   WHERE t.client_identity_id=NEW.identity_id
     AND t.recorded_at<=NEW.created_at
     AND t.recorded_at>=NEW.created_at - interval '30 days'
   ORDER BY t.recorded_at,t.id
   LIMIT 1
   ON CONFLICT (intent_id) DO NOTHING;
  RETURN NEW;
END $$;
CREATE TRIGGER growth_intent_attribution_on_create AFTER INSERT ON help_intents
  FOR EACH ROW EXECUTE FUNCTION apgic_attribute_new_help_intent();
