-- Specialist reputation is grounded in completed canonical LIVE bookings,
-- never self-declared ratings, sponsors or synthetic catalog entries.
-- Migrations from 000024 onward are wrapped atomically by the migration runner.
CREATE TABLE specialist_verified_feedback (
  id uuid PRIMARY KEY,
  booking_id uuid NOT NULL UNIQUE REFERENCES bookings(id),
  specialist_id uuid NOT NULL REFERENCES specialist_profiles(id),
  client_identity_id uuid NOT NULL REFERENCES identities(id),
  rating smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
  state text NOT NULL DEFAULT 'PENDING'
    CHECK (state IN ('PENDING','APPROVED','REJECTED','WITHDRAWN')),
  submitted_at timestamptz NOT NULL DEFAULT now(),
  reviewed_at timestamptz,
  reviewer_ref text,
  CHECK (
    (state='PENDING' AND reviewed_at IS NULL AND reviewer_ref IS NULL)
    OR (state<>'PENDING' AND reviewed_at IS NOT NULL
        AND nullif(btrim(reviewer_ref), '') IS NOT NULL)
  )
);

CREATE INDEX specialist_verified_feedback_public_idx
  ON specialist_verified_feedback(specialist_id)
  INCLUDE (rating) WHERE state='APPROVED';

CREATE OR REPLACE FUNCTION apgic_specialist_feedback_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='INSERT' THEN
    IF NEW.state <> 'PENDING' OR NOT EXISTS (
      SELECT 1 FROM bookings booking
      JOIN booking_slots slot ON slot.id=booking.slot_id
        AND slot.tenant_scope='specialist/live'
      JOIN specialist_bookable_offers offer ON offer.id=slot.specialist_offer_id
        AND offer.specialist_identity_id=slot.specialist_identity_id
      WHERE booking.id=NEW.booking_id
        AND booking.client_identity_id=NEW.client_identity_id
        AND booking.state='COMPLETED'
        AND offer.specialist_id=NEW.specialist_id
        AND slot.ends_at <= clock_timestamp()
    ) THEN
      RAISE EXCEPTION 'verified feedback requires a completed, real client-owned live booking';
    END IF;
  ELSIF TG_OP='UPDATE' THEN
    IF NEW.id<>OLD.id OR NEW.booking_id<>OLD.booking_id
      OR NEW.specialist_id<>OLD.specialist_id
      OR NEW.client_identity_id<>OLD.client_identity_id
      OR NEW.rating<>OLD.rating OR NEW.submitted_at<>OLD.submitted_at THEN
      RAISE EXCEPTION 'verified feedback identity and rating are immutable';
    END IF;
    IF OLD.state<>'PENDING' AND NEW.state=OLD.state
      AND (NEW.reviewed_at, NEW.reviewer_ref)
       IS DISTINCT FROM (OLD.reviewed_at, OLD.reviewer_ref) THEN
      RAISE EXCEPTION 'verified feedback moderation decision cannot be rewritten';
    END IF;
    IF NEW.state<>OLD.state AND NOT (
      (OLD.state='PENDING' AND NEW.state IN ('APPROVED','REJECTED'))
      OR (OLD.state='APPROVED' AND NEW.state='WITHDRAWN')
    ) THEN
      RAISE EXCEPTION 'invalid verified feedback moderation transition';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER specialist_verified_feedback_guard
BEFORE INSERT OR UPDATE ON specialist_verified_feedback
FOR EACH ROW EXECUTE FUNCTION apgic_specialist_feedback_guard();

CREATE OR REPLACE FUNCTION apgic_specialist_feedback_no_delete()
RETURNS trigger LANGUAGE plpgsql AS $feedback_no_delete$
BEGIN
  RAISE EXCEPTION 'review evidence is immutable; use explicit audited WITHDRAWN state';
END;
$feedback_no_delete$;

CREATE TRIGGER specialist_verified_feedback_no_delete
BEFORE DELETE ON specialist_verified_feedback
FOR EACH ROW EXECUTE FUNCTION apgic_specialist_feedback_no_delete();
