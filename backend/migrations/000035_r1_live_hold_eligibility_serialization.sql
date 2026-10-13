-- Serialize fresh booking holds with the canonical qualification, profile,
-- publication, seller/Product and priced-offer rows. A concurrent revoke,
-- product pause or ownership withdrawal must either commit before the hold
-- (hold denied), or wait until an already committed hold finishes.
-- Do not rely solely on an MVCC EXISTS predicate that can read pre-revoke state.
CREATE OR REPLACE FUNCTION apgic_live_booking_hold_guard()
RETURNS trigger LANGUAGE plpgsql AS $eligible_hold$
DECLARE
  live_offer uuid;
  slot_scope text;
  slot_owner uuid;
BEGIN
  SELECT tenant_scope, specialist_offer_id, specialist_identity_id
    INTO slot_scope, live_offer, slot_owner
    FROM booking_slots WHERE id = NEW.slot_id;
  IF slot_scope = 'specialist/live' THEN
    IF live_offer IS NULL THEN
      RAISE EXCEPTION 'live booking offer no longer eligible';
    END IF;

    -- Lock all *mutable* owners before checking the complete canonical
    -- predicate. Existing rows must be locked even if currently suspended,
    -- or a concurrent state transition could escape the safeguard.
    PERFORM 1
      FROM specialist_bookable_offers offer
      JOIN specialist_profiles sp ON sp.id = offer.specialist_id
      JOIN specialist_capabilities cap ON cap.specialist_id = sp.id
        AND cap.topic_id = offer.topic_id
      JOIN specialist_publications pub ON pub.specialist_id = sp.id
        AND pub.topic_id = offer.topic_id
      JOIN products product ON product.id = offer.product_id
      JOIN organization_directions direction ON direction.id = product.organization_direction_id
        AND direction.organization_id = product.owner_id
      JOIN organizations org ON org.id = product.owner_id
      JOIN organization_memberships member ON member.organization_id = org.id
        AND member.identity_id = sp.identity_id
      JOIN organization_ownerships owner ON owner.organization_id = org.id
        AND owner.identity_id = sp.identity_id
     WHERE offer.id = live_offer
       AND offer.specialist_identity_id = slot_owner
     FOR SHARE OF offer, sp, cap, pub, product, direction, org, member, owner;

    IF NOT FOUND OR NOT apgic_live_offer_valid(live_offer) THEN
      RAISE EXCEPTION 'live booking offer no longer eligible';
    END IF;
  END IF;
  RETURN NEW;
END;
$eligible_hold$;
