-- R9 Live Events: APGIC owns business truth; media/settlement belong to providers.
BEGIN;
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
COMMIT;
