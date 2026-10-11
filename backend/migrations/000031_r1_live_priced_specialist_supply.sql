-- R1/R2: real specialist-priced consultation supply owned by existing APGIC
-- specialist, Product, and booking_slots rows (not the conformance fixture).

CREATE TABLE specialist_bookable_offers (
  id uuid PRIMARY KEY,
  specialist_id uuid NOT NULL REFERENCES specialist_profiles(id),
  specialist_identity_id uuid NOT NULL REFERENCES identities(id),
  product_id uuid NOT NULL REFERENCES products(id),
  topic_id text NOT NULL CHECK (btrim(topic_id) <> ''),
  price_minor bigint NOT NULL CHECK (price_minor > 0),
  currency text NOT NULL CHECK (currency = 'RUB'),
  state text NOT NULL CHECK (state IN ('ACTIVE','PAUSED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  paused_at timestamptz,
  CHECK ((state = 'ACTIVE' AND paused_at IS NULL)
     OR (state = 'PAUSED' AND paused_at IS NOT NULL))
);

CREATE UNIQUE INDEX specialist_one_active_bookable_offer_per_topic
  ON specialist_bookable_offers (specialist_id, topic_id) WHERE state = 'ACTIVE';

ALTER TABLE booking_slots
  ADD COLUMN specialist_offer_id uuid REFERENCES specialist_bookable_offers(id),
  ADD CONSTRAINT booking_slots_live_offer_scope_check
  CHECK (
    (tenant_scope = 'specialist/live' AND specialist_offer_id IS NOT NULL)
    OR (tenant_scope <> 'specialist/live' AND specialist_offer_id IS NULL)
  );

CREATE UNIQUE INDEX booking_slots_live_identity_start_unique
  ON booking_slots (specialist_identity_id, starts_at)
  WHERE tenant_scope = 'specialist/live';

-- Revalidation is anchored in the existing approved specialist publication
-- and the already published Organization/Product owner; a profile alone does
-- not manufacture a paid offer or permitted sales channel.
CREATE FUNCTION apgic_live_offer_valid(p_offer_id uuid)
RETURNS boolean LANGUAGE sql VOLATILE AS $
  SELECT EXISTS (
    SELECT 1
      FROM specialist_bookable_offers offer
      JOIN specialist_profiles sp ON sp.id = offer.specialist_id
      JOIN specialist_capabilities cap
        ON cap.specialist_id = sp.id AND cap.topic_id = offer.topic_id
      JOIN specialist_publications pub
        ON pub.specialist_id = sp.id AND pub.topic_id = offer.topic_id
       AND pub.state = 'ACTIVE'
      JOIN products product ON product.id = offer.product_id
        AND product.status = 'PUBLISHED'
        AND product.owner_type = 'ORGANIZATION'
      JOIN organization_directions direction
        ON direction.id = product.organization_direction_id
       AND direction.organization_id = product.owner_id
       AND direction.status = 'ACTIVE'
      JOIN organizations org
        ON org.id = product.owner_id AND org.status = 'ACTIVE'
      JOIN organization_memberships member
        ON member.organization_id = org.id
       AND member.identity_id = sp.identity_id
       AND member.status = 'ACTIVE'
      JOIN organization_ownerships owner
        ON owner.organization_id = org.id
       AND owner.identity_id = sp.identity_id
       AND owner.status = 'ACTIVE'
     WHERE offer.id = p_offer_id
       AND offer.state = 'ACTIVE'
       AND offer.price_minor > 0
       AND offer.currency = 'RUB'
       AND offer.specialist_identity_id = sp.identity_id
       AND sp.profile_complete AND sp.review_state = 'APPROVED'
       AND cap.evidence_state = 'APGIC_VERIFIED'
       AND cap.verification_state = 'ACTIVE'
       AND (cap.expires_at IS NULL OR cap.expires_at > clock_timestamp())
  )
$$;

CREATE FUNCTION apgic_live_offer_insert_guard()
RETURNS trigger LANGUAGE plpgsql AS $
BEGIN
  IF NOT apgic_live_offer_valid(NEW.id) THEN
    RAISE EXCEPTION 'live offer requires reviewed publication and authorized Product';
  END IF;
  RETURN NEW;
END;
$;
CREATE TRIGGER specialist_live_offer_qualification_guard
AFTER INSERT ON specialist_bookable_offers
FOR EACH ROW EXECUTE FUNCTION apgic_live_offer_insert_guard();

CREATE FUNCTION apgic_live_slot_write_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  offer_identity uuid;
BEGIN
  IF TG_OP = 'UPDATE' THEN
    -- Existing conformance catalog may refresh its rolling test horizon.
    -- Never impose LIVE immutability on unrelated conformance rows.
    IF OLD.tenant_scope = 'specialist/live'
       OR NEW.tenant_scope = 'specialist/live' THEN
      IF NEW.specialist_offer_id IS DISTINCT FROM OLD.specialist_offer_id
         OR NEW.specialist_identity_id IS DISTINCT FROM OLD.specialist_identity_id
         OR NEW.tenant_scope IS DISTINCT FROM OLD.tenant_scope
         OR NEW.starts_at IS DISTINCT FROM OLD.starts_at
         OR NEW.ends_at IS DISTINCT FROM OLD.ends_at THEN
        RAISE EXCEPTION 'live booking slot ownership and timing are immutable';
      END IF;
    END IF;
    RETURN NEW;
  END IF;
  IF NEW.tenant_scope <> 'specialist/live' THEN
    RETURN NEW;
  END IF;
  SELECT specialist_identity_id INTO offer_identity
    FROM specialist_bookable_offers WHERE id = NEW.specialist_offer_id;
  IF offer_identity IS NULL OR offer_identity <> NEW.specialist_identity_id
     OR NOT apgic_live_offer_valid(NEW.specialist_offer_id) THEN
    RAISE EXCEPTION 'live slot requires active authorized priced offer';
  END IF;
  IF NOT NEW.exclusive OR NEW.starts_at <= clock_timestamp() + interval '15 minutes'
     OR NEW.ends_at <= NEW.starts_at THEN
    RAISE EXCEPTION 'live slot must be future, exclusive and have valid duration';
  END IF;
  -- Serialize overlapping offers/slots of the same specialist across API
  -- instances, not just identical start timestamps.
  PERFORM pg_advisory_xact_lock(hashtextextended('apgic:live-slot:' || offer_identity::text, 0));
  IF EXISTS (
    SELECT 1 FROM booking_slots existing
     WHERE existing.tenant_scope = 'specialist/live'
       AND existing.specialist_identity_id = offer_identity
       AND existing.starts_at < NEW.ends_at
       AND existing.ends_at > NEW.starts_at
  ) THEN
    RAISE EXCEPTION 'overlapping live specialist slot';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER booking_slots_live_supply_guard
BEFORE INSERT OR UPDATE ON booking_slots
FOR EACH ROW EXECUTE FUNCTION apgic_live_slot_write_guard();

-- Even if an older UI displayed a slot, acquisition fails closed if its
-- publication, owner, qualification or Product was revoked meanwhile.
CREATE FUNCTION apgic_live_booking_hold_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  live_offer uuid;
  slot_scope text;
BEGIN
  SELECT tenant_scope, specialist_offer_id
    INTO slot_scope, live_offer
    FROM booking_slots WHERE id = NEW.slot_id;
  IF slot_scope = 'specialist/live'
     AND (live_offer IS NULL OR NOT apgic_live_offer_valid(live_offer)) THEN
    RAISE EXCEPTION 'live booking offer no longer eligible';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER booking_holds_live_supply_guard
BEFORE INSERT ON booking_holds
FOR EACH ROW EXECUTE FUNCTION apgic_live_booking_hold_guard();

-- Never reprice an advertised offer in place. Pause and explicitly publish
-- a new version, preserving the money snapshot for already acquired holds.
CREATE FUNCTION apgic_live_offer_immutability_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.specialist_id <> OLD.specialist_id
     OR NEW.specialist_identity_id <> OLD.specialist_identity_id
     OR NEW.product_id <> OLD.product_id
     OR NEW.topic_id <> OLD.topic_id
     OR NEW.price_minor <> OLD.price_minor
     OR NEW.currency <> OLD.currency
     OR NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'live offer identity, price and owner are immutable';
  END IF;
  IF OLD.state = 'PAUSED' AND NEW.state <> 'PAUSED' THEN
    RAISE EXCEPTION 'paused live offer cannot be reactivated; publish a new one';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER specialist_live_offer_immutability_guard
BEFORE UPDATE ON specialist_bookable_offers
FOR EACH ROW EXECUTE FUNCTION apgic_live_offer_immutability_guard();

