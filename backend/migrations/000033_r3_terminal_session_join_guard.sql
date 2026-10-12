-- A confirmed economic booking can remain unresolved after provider-evidenced
-- TECHNICAL_FAILURE; never issue fresh room grants for its terminal session.
-- Rescheduling creates a new booking, while the failed original is immutable.
CREATE OR REPLACE FUNCTION apgic_r3_terminal_session_join_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.decision = 'ALLOW'
     AND EXISTS (
       SELECT 1 FROM consultation_sessions s
       WHERE s.booking_id = NEW.booking_id
         AND s.state IN ('COMPLETED','TECHNICAL_FAILURE')
     ) THEN
    RAISE EXCEPTION 'terminal consultation cannot issue a new join credential';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER communication_join_authorizations_terminal_session_guard
BEFORE INSERT ON communication_join_authorizations
FOR EACH ROW EXECUTE FUNCTION apgic_r3_terminal_session_join_guard();
