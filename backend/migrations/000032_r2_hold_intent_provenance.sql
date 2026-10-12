BEGIN;

-- A hold is an exclusive, immutable reservation of one specific priced offer
-- for one confirmed HelpIntent. The public live catalog omits held slots;
-- without this persisted origin an HTTP retry after API restart could not
-- distinguish the original operation from a different intent of the same user.
ALTER TABLE booking_holds
  ADD COLUMN help_intent_id uuid REFERENCES help_intents(id),
  ADD COLUMN specialist_offer_id uuid REFERENCES specialist_bookable_offers(id),
  ADD CONSTRAINT booking_holds_origin_pair CHECK (
    (help_intent_id IS NULL) = (specialist_offer_id IS NULL)
  );

CREATE INDEX booking_holds_origin_lookup
  ON booking_holds(help_intent_id, specialist_offer_id)
  WHERE help_intent_id IS NOT NULL;

-- The first assignment of the pair happens inside the SAME transaction as
-- apgic_acquire_slot_hold. Once assigned neither field may change, even when
-- the offer is paused or the hold expires. Legacy holds are read-only for
-- provenance-sensitive replay: their NULL origin cannot mint access.
CREATE OR REPLACE FUNCTION apgic_live_hold_origin_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  intent_row help_intents%ROWTYPE;
  slot_offer_id uuid;
  slot_scope text;
  offer_topic text;
BEGIN
  IF OLD.help_intent_id IS NOT NULL
    AND (
      NEW.help_intent_id IS DISTINCT FROM OLD.help_intent_id
      OR NEW.specialist_offer_id IS DISTINCT FROM OLD.specialist_offer_id
    ) THEN
    RAISE EXCEPTION 'immutable live booking hold intent/offer cannot be rewritten';
  END IF;

  IF NEW.help_intent_id IS NULL THEN
    RETURN NEW;
  END IF;

  SELECT *
    INTO intent_row
    FROM help_intents
   WHERE id=NEW.help_intent_id;
  SELECT specialist_offer_id,tenant_scope
    INTO slot_offer_id,slot_scope
    FROM booking_slots
   WHERE id=NEW.slot_id;
  SELECT topic_id INTO offer_topic
    FROM specialist_bookable_offers
   WHERE id=NEW.specialist_offer_id;

  IF NOT FOUND
    OR intent_row.id IS NULL
    OR intent_row.state <> 'CONFIRMED'
    OR intent_row.identity_id <> NEW.client_identity_id
    OR NOT offer_topic=ANY(intent_row.topics)
    OR slot_scope <> 'specialist/live'
    OR slot_offer_id IS DISTINCT FROM NEW.specialist_offer_id THEN
    RAISE EXCEPTION 'live hold intent/offer provenance or qualification mismatch';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER booking_holds_live_origin_guard
BEFORE UPDATE ON booking_holds
FOR EACH ROW EXECUTE FUNCTION apgic_live_hold_origin_guard();

COMMIT;
