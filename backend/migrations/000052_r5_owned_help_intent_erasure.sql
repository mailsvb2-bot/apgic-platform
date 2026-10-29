-- Physically erase unreferenced personal help requests. Referenced canonical
-- bookings/duty evidence is immutable: scrub its private fields and owner link
-- instead of cascading into another party's financial or appointment records.
-- The only allowed erasure caller is an approved in-flight deletion request.
CREATE OR REPLACE FUNCTION apgic_guard_help_intent_erasure()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.identity_id IS NULL OR NOT EXISTS (
    SELECT 1 FROM delete_account_requests r
    WHERE r.identity_id=OLD.identity_id AND r.state='RETENTION_CLASSIFIED'
  ) THEN
    RAISE EXCEPTION 'help intent deletion requires approved canonical erasure';
  END IF;
  RETURN OLD;
END;
$$;

DROP TRIGGER help_intents_no_delete ON help_intents;
CREATE TRIGGER help_intents_no_delete
BEFORE DELETE ON help_intents
FOR EACH ROW EXECUTE FUNCTION apgic_guard_help_intent_erasure();

CREATE FUNCTION apgic_erase_owned_help_intents(p_owner uuid)
RETURNS bigint LANGUAGE plpgsql AS $$
DECLARE
  item record;
  processed bigint := 0;
BEGIN
  FOR item IN SELECT id FROM help_intents WHERE identity_id=p_owner ORDER BY id FOR UPDATE LOOP
    BEGIN
      DELETE FROM help_intents WHERE id=item.id AND identity_id=p_owner;
      IF FOUND THEN
        processed := processed + 1;
      END IF;
    EXCEPTION WHEN foreign_key_violation THEN
      -- Referenced evidence cannot be removed without breaking canonical
      -- bookings. Clear both the owner link and every client-supplied field.
      UPDATE help_intents
        SET identity_id=NULL, free_text='[удалено по запросу владельца]',
            topics=ARRAY[]::text[], goals=ARRAY[]::text[],
            context='{}'::jsonb, updated_at=now()
        WHERE id=item.id AND identity_id=p_owner;
      IF FOUND THEN
        processed := processed + 1;
      END IF;
    END;
  END LOOP;
  IF EXISTS(SELECT 1 FROM help_intents WHERE identity_id=p_owner) THEN
    RAISE EXCEPTION 'personal help intent owner residue after erasure';
  END IF;
  RETURN processed;
END;
$$;
