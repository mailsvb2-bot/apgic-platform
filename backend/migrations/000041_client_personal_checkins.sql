-- One immutable, non-medical client report per trusted, completed consultation.
CREATE TABLE client_personal_check_ins (
  booking_id uuid PRIMARY KEY REFERENCES bookings(id) ON DELETE RESTRICT,
  client_identity_id uuid NOT NULL REFERENCES identities(id),
  progress_code text NOT NULL CHECK (progress_code IN ('IMPROVED','UNCHANGED','WORSE','UNSURE')),
  match_satisfaction smallint CHECK (match_satisfaction BETWEEN 1 AND 5),
  recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX client_personal_check_ins_owner_idx ON client_personal_check_ins(client_identity_id,recorded_at DESC);
CREATE OR REPLACE FUNCTION apgic_client_check_in_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP <> 'INSERT' THEN
    RAISE EXCEPTION 'client report is immutable';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM bookings b
    JOIN consultation_sessions cs ON cs.booking_id=b.id
    WHERE b.id=NEW.booking_id AND b.client_identity_id=NEW.client_identity_id
      AND cs.client_identity_id=NEW.client_identity_id AND cs.state='COMPLETED') THEN
    RAISE EXCEPTION 'client report requires owned, provider-completed consultation';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER client_personal_check_in_guard BEFORE INSERT OR UPDATE OR DELETE
ON client_personal_check_ins FOR EACH ROW EXECUTE FUNCTION apgic_client_check_in_guard();
