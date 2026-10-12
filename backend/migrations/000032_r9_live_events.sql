-- R9 Live Events: APGIC owns business truth; media/settlement belong to providers.
CREATE TABLE live_events (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL REFERENCES organizations(id),
 organizer_identity_id uuid NOT NULL REFERENCES identities(id),
 title text NOT NULL CHECK (length(btrim(title)) BETWEEN 5 AND 160),
 description text NOT NULL DEFAULT '',
 format text NOT NULL CHECK(format IN ('WEBINAR','GROUP','WORKSHOP','QA','CONFERENCE')),
 starts_at timestamptz NOT NULL,
 ends_at timestamptz NOT NULL,
 capacity integer NOT NULL CHECK (capacity BETWEEN 2 AND 1000),
 state text NOT NULL DEFAULT 'DRAFT' CHECK (state IN ('DRAFT','PUBLISHED','LIVE','COMPLETED','CANCELLED')),
 price_minor bigint NOT NULL DEFAULT 0 CHECK(price_minor=0),
 currency char(3) NOT NULL DEFAULT 'RUB' CHECK(currency='RUB'),
 provider_instance_id uuid REFERENCES connector_instances(id),
 provider_proof_checked_at timestamptz,
 recording_available boolean NOT NULL DEFAULT false CHECK(recording_available=false),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK (ends_at>starts_at AND ends_at<=starts_at+interval '8 hours'),
 CHECK ((provider_instance_id IS NULL AND provider_proof_checked_at IS NULL) OR (provider_instance_id IS NOT NULL AND provider_proof_checked_at IS NOT NULL))
);
CREATE INDEX live_events_public_idx ON live_events(starts_at,id) WHERE state IN ('PUBLISHED','LIVE');
CREATE INDEX live_events_org_idx ON live_events(organization_id, created_at DESC);
CREATE TABLE live_event_registrations (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL REFERENCES live_events(id),
 identity_id uuid NOT NULL REFERENCES identities(id),
 state text NOT NULL DEFAULT 'REGISTERED' CHECK (state IN ('REGISTERED','CANCELLED')),
 registered_at timestamptz NOT NULL DEFAULT now(),
 cancelled_at timestamptz,
 UNIQUE(event_id,identity_id),
 CHECK((state='REGISTERED' AND cancelled_at IS NULL) OR (state='CANCELLED' AND cancelled_at IS NOT NULL))
);
CREATE INDEX live_event_registrations_state_idx ON live_event_registrations(event_id,state);
CREATE TABLE live_event_attendance_facts (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL REFERENCES live_events(id),
 registration_id uuid NOT NULL REFERENCES live_event_registrations(id),
 connector_instance_id uuid NOT NULL REFERENCES connector_instances(id),
 provider_event_id text NOT NULL CHECK (btrim(provider_event_id)<>''),
 fact_type text NOT NULL CHECK(fact_type IN ('JOINED','LEFT')),
 occurred_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(connector_instance_id,provider_event_id)
);
CREATE TRIGGER live_event_attendance_facts_append_only
BEFORE UPDATE OR DELETE ON live_event_attendance_facts
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TABLE live_event_mutations (
 actor_id uuid NOT NULL REFERENCES identities(id),
 idempotency_key text NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 160),
 action text NOT NULL CHECK (action IN ('CREATE','REGISTER','UNREGISTER','PUBLISH','START','FINISH','CANCEL')),
 event_id uuid NOT NULL REFERENCES live_events(id),
 fingerprint text NOT NULL CHECK (length(fingerprint)=64),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,idempotency_key)
);

-- Defense in depth: API checks cannot be the only canonical owner/state gate.
CREATE FUNCTION apgic_r9_event_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'DRAFT' OR NEW.provider_instance_id IS NOT NULL OR NOT EXISTS(
   SELECT 1 FROM organization_ownerships o
    JOIN organizations g ON g.id=o.organization_id AND g.status='ACTIVE'
   WHERE o.organization_id=NEW.organization_id
     AND o.identity_id=NEW.organizer_identity_id AND o.status='ACTIVE'
  ) THEN
   RAISE EXCEPTION 'R9 event requires active owner, draft state and no unverified media';
  END IF;
 ELSE
  IF NEW.id<>OLD.id OR NEW.organization_id<>OLD.organization_id
    OR NEW.organizer_identity_id<>OLD.organizer_identity_id
    OR NEW.title<>OLD.title OR NEW.description<>OLD.description
    OR NEW.format<>OLD.format OR NEW.starts_at<>OLD.starts_at
    OR NEW.ends_at<>OLD.ends_at OR NEW.capacity<>OLD.capacity
    OR NEW.price_minor<>OLD.price_minor OR NEW.currency<>OLD.currency
    OR NEW.recording_available<>OLD.recording_available THEN
   RAISE EXCEPTION 'R9 published event business identity/schedule is immutable';
  END IF;
  IF NEW.state<>OLD.state AND NOT (
    (OLD.state='DRAFT' AND NEW.state IN ('PUBLISHED','CANCELLED'))
    OR (OLD.state='PUBLISHED' AND NEW.state IN ('LIVE','CANCELLED'))
    OR (OLD.state='LIVE' AND NEW.state IN ('COMPLETED','CANCELLED'))
  ) THEN
   RAISE EXCEPTION 'R9 illegal event lifecycle transition';
  END IF;
  IF OLD.state IN ('COMPLETED','CANCELLED') AND ROW(NEW.*) IS DISTINCT FROM ROW(OLD.*) THEN
   RAISE EXCEPTION 'R9 terminal event is immutable';
  END IF;
  IF NEW.state IN ('PUBLISHED','LIVE') AND
    (NEW.provider_instance_id IS NULL OR NEW.provider_proof_checked_at IS NULL) THEN
   RAISE EXCEPTION 'R9 event must have signed provider readiness before publication';
  END IF;
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER live_events_guard BEFORE INSERT OR UPDATE ON live_events
 FOR EACH ROW EXECUTE FUNCTION apgic_r9_event_guard();
CREATE TRIGGER live_events_no_delete BEFORE DELETE ON live_events
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE FUNCTION apgic_r9_registration_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
 event_state text;
 event_starts timestamptz;
 event_capacity integer;
 occupied integer;
BEGIN
 IF TG_OP='INSERT' THEN
   SELECT state,starts_at,capacity INTO event_state,event_starts,event_capacity
     FROM live_events WHERE id=NEW.event_id FOR UPDATE;
   IF event_state IS DISTINCT FROM 'PUBLISHED' OR event_starts<=now() THEN
    RAISE EXCEPTION 'R9 registration requires published future event';
   END IF;
   SELECT count(*) INTO occupied FROM live_event_registrations
     WHERE event_id=NEW.event_id AND state='REGISTERED';
   IF occupied>=event_capacity THEN RAISE EXCEPTION 'R9 event capacity exhausted'; END IF;
   IF NEW.state<>'REGISTERED' THEN RAISE EXCEPTION 'R9 invalid initial registration'; END IF;
 ELSE
   IF NEW.id<>OLD.id OR NEW.event_id<>OLD.event_id OR NEW.identity_id<>OLD.identity_id THEN
    RAISE EXCEPTION 'R9 registration identity is immutable';
   END IF;
   IF OLD.state='REGISTERED' AND NEW.state='CANCELLED' THEN
     IF NEW.registered_at<>OLD.registered_at OR NEW.cancelled_at IS NULL THEN
      RAISE EXCEPTION 'R9 invalid cancellation evidence';
     END IF;
   ELSIF OLD.state='CANCELLED' AND NEW.state='REGISTERED' THEN
     SELECT state,starts_at,capacity INTO event_state,event_starts,event_capacity
       FROM live_events WHERE id=NEW.event_id FOR UPDATE;
     IF event_state IS DISTINCT FROM 'PUBLISHED' OR event_starts<=now()
       OR NEW.cancelled_at IS NOT NULL OR NEW.registered_at<OLD.cancelled_at THEN
       RAISE EXCEPTION 'R9 re-registration requires published future event';
     END IF;
     SELECT count(*) INTO occupied FROM live_event_registrations
       WHERE event_id=NEW.event_id AND state='REGISTERED';
     IF occupied>=event_capacity THEN RAISE EXCEPTION 'R9 re-registration capacity exhausted'; END IF;
   ELSE
     RAISE EXCEPTION 'R9 registration has illegal transition';
   END IF;
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER live_event_registrations_guard BEFORE INSERT OR UPDATE ON live_event_registrations
 FOR EACH ROW EXECUTE FUNCTION apgic_r9_registration_guard();
CREATE TRIGGER live_event_registrations_no_delete BEFORE DELETE ON live_event_registrations
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER live_event_mutations_append_only BEFORE UPDATE OR DELETE ON live_event_mutations
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE FUNCTION apgic_r9_attendance_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (
  SELECT 1 FROM live_event_registrations r
  JOIN live_events e ON e.id=r.event_id
  WHERE r.id=NEW.registration_id AND e.id=NEW.event_id
    AND e.provider_instance_id=NEW.connector_instance_id
    AND r.state='REGISTERED' AND e.state IN ('PUBLISHED','LIVE')
 ) THEN RAISE EXCEPTION 'R9 attendance requires registered participant and bound provider'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER live_event_attendance_guard BEFORE INSERT ON live_event_attendance_facts
 FOR EACH ROW EXECUTE FUNCTION apgic_r9_attendance_guard();
