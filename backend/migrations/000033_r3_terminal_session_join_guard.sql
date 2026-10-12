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

-- A booking may be cancelled before a provider session actually starts or
-- after terminal technical failure, but must never be quietly cancelled or
-- marked NO_SHOW while authentic provider media evidence says it is active.
-- The booking/payment owners remain responsible for refunds or rescheduling.
CREATE OR REPLACE FUNCTION apgic_r3_active_media_booking_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.state='CONFIRMED' AND NEW.state IN ('CANCELLED','NO_SHOW')
     AND EXISTS (
       SELECT 1 FROM consultation_sessions s
       WHERE s.booking_id=OLD.id
         AND (
           s.state IN ('IN_PROGRESS','RECOVERING','COMPLETED')
           OR (NEW.state='NO_SHOW' AND s.state='TECHNICAL_FAILURE')
         )
     ) THEN
    RAISE EXCEPTION 'booking cancellation or no-show conflicts with provider session evidence';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER booking_r3_active_media_protection
BEFORE UPDATE ON bookings
FOR EACH ROW EXECUTE FUNCTION apgic_r3_active_media_booking_guard();
