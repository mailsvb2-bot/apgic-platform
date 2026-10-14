-- R3: distinct participant readiness (not provider JOINED), plus minimal
-- client-reported outcome. Both survive API restarts without media retention.
CREATE TABLE consultation_participant_readiness (
    booking_id uuid NOT NULL REFERENCES bookings(id),
    identity_id uuid NOT NULL REFERENCES identities(id),
    role text NOT NULL CHECK (role IN ('CLIENT','SPECIALIST')),
    ready_at timestamptz NOT NULL,
    PRIMARY KEY (booking_id, identity_id)
);

CREATE OR REPLACE FUNCTION apgic_consultation_readiness_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    b bookings%ROWTYPE;
    specialist uuid;
BEGIN
    SELECT * INTO b FROM bookings WHERE id=NEW.booking_id FOR UPDATE;
    IF NOT FOUND OR b.state <> 'CONFIRMED' THEN
        RAISE EXCEPTION 'readiness requires confirmed booking';
    END IF;
    SELECT specialist_identity_id INTO specialist FROM booking_slots WHERE id=b.slot_id;
    IF NOT ((NEW.role='CLIENT' AND NEW.identity_id=b.client_identity_id)
        OR (NEW.role='SPECIALIST' AND NEW.identity_id=specialist)) THEN
        RAISE EXCEPTION 'readiness requires canonical participant identity';
    END IF;
    IF NEW.ready_at < b.starts_at - interval '15 minutes'
       OR NEW.ready_at > b.ends_at OR NEW.ready_at > statement_timestamp() + interval '1 second' THEN
        RAISE EXCEPTION 'readiness outside booking window';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER consultation_readiness_guard
BEFORE INSERT ON consultation_participant_readiness
FOR EACH ROW EXECUTE FUNCTION apgic_consultation_readiness_guard();

CREATE TRIGGER consultation_readiness_append_only
BEFORE UPDATE OR DELETE ON consultation_participant_readiness
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE consultation_client_outcomes (
    id uuid PRIMARY KEY,
    booking_id uuid NOT NULL REFERENCES bookings(id),
    client_identity_id uuid NOT NULL REFERENCES identities(id),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 128),
    progress smallint NOT NULL CHECK (progress BETWEEN -2 AND 2),
    match_satisfaction smallint NOT NULL CHECK (match_satisfaction BETWEEN 1 AND 5),
    reported_at timestamptz NOT NULL,
    UNIQUE (booking_id,idempotency_key)
);

CREATE INDEX consultation_client_outcomes_recent
    ON consultation_client_outcomes (booking_id,client_identity_id,reported_at DESC,id DESC);

CREATE OR REPLACE FUNCTION apgic_consultation_client_outcome_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    booking_client uuid;
    consultation_state text;
BEGIN
    SELECT b.client_identity_id, s.state INTO booking_client, consultation_state
      FROM bookings b JOIN consultation_sessions s ON s.booking_id=b.id
      WHERE b.id=NEW.booking_id FOR UPDATE OF b;
    IF NOT FOUND OR booking_client<>NEW.client_identity_id OR consultation_state<>'COMPLETED' THEN
        RAISE EXCEPTION 'outcome requires client and completed consultation evidence';
    END IF;
    IF NEW.reported_at > statement_timestamp() + interval '1 second' THEN
        RAISE EXCEPTION 'future client outcome is invalid';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER consultation_client_outcomes_guard
BEFORE INSERT ON consultation_client_outcomes
FOR EACH ROW EXECUTE FUNCTION apgic_consultation_client_outcome_guard();

CREATE TRIGGER consultation_client_outcomes_append_only
BEFORE UPDATE OR DELETE ON consultation_client_outcomes
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
