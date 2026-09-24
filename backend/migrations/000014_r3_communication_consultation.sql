BEGIN;

CREATE TABLE communication_access_policy_versions (
  version text PRIMARY KEY CHECK (btrim(version) <> ''),
  join_early_seconds integer NOT NULL CHECK (join_early_seconds >= 0),
  join_late_seconds integer NOT NULL CHECK (join_late_seconds >= 0),
  max_credential_ttl_seconds integer NOT NULL CHECK (max_credential_ttl_seconds > 0),
  created_at timestamptz NOT NULL
);

CREATE TRIGGER communication_access_policy_versions_append_only
BEFORE UPDATE OR DELETE ON communication_access_policy_versions
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE booking_access_entitlement_versions (
  id uuid PRIMARY KEY,
  booking_id uuid NOT NULL REFERENCES bookings(id),
  identity_id uuid NOT NULL REFERENCES identities(id),
  role text NOT NULL CHECK (role IN ('CLIENT','SPECIALIST')),
  state text NOT NULL CHECK (state IN ('ACTIVE','REVOKED','EXPIRED')),
  basis_ref text NOT NULL CHECK (btrim(basis_ref) <> ''),
  policy_version text NOT NULL CHECK (btrim(policy_version) <> ''),
  supersedes_id uuid REFERENCES booking_access_entitlement_versions(id),
  effective_from timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  CHECK (expires_at > effective_from)
);

CREATE INDEX booking_access_entitlement_latest_idx
  ON booking_access_entitlement_versions (
    booking_id, identity_id, role, effective_from DESC, created_at DESC
  );

CREATE OR REPLACE FUNCTION apgic_booking_access_entitlement_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  booking_row bookings%ROWTYPE;
  specialist_id uuid;
  latest_id uuid;
BEGIN
  SELECT *
  INTO booking_row
  FROM bookings
  WHERE id = NEW.booking_id;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'booking access entitlement requires booking';
  END IF;

  SELECT specialist_identity_id
  INTO specialist_id
  FROM booking_slots
  WHERE id = booking_row.slot_id;

  IF (NEW.role = 'CLIENT' AND NEW.identity_id <> booking_row.client_identity_id)
    OR (NEW.role = 'SPECIALIST' AND NEW.identity_id <> specialist_id) THEN
    RAISE EXCEPTION 'booking access entitlement role/identity mismatch';
  END IF;

  SELECT id
  INTO latest_id
  FROM booking_access_entitlement_versions
  WHERE booking_id = NEW.booking_id
    AND identity_id = NEW.identity_id
    AND role = NEW.role
  ORDER BY effective_from DESC, created_at DESC, id DESC
  LIMIT 1;

  IF latest_id IS NULL THEN
    IF NEW.supersedes_id IS NOT NULL THEN
      RAISE EXCEPTION 'first booking access entitlement version cannot supersede history';
    END IF;
  ELSE
    IF NEW.supersedes_id IS DISTINCT FROM latest_id THEN
      RAISE EXCEPTION 'booking access entitlement revision must supersede latest version';
    END IF;
    IF NEW.effective_from <= (
      SELECT effective_from FROM booking_access_entitlement_versions WHERE id = latest_id
    ) THEN
      RAISE EXCEPTION 'booking access entitlement effective time must advance';
    END IF;
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER booking_access_entitlement_versions_insert_guard
BEFORE INSERT ON booking_access_entitlement_versions
FOR EACH ROW EXECUTE FUNCTION apgic_booking_access_entitlement_guard();

CREATE TRIGGER booking_access_entitlement_versions_append_only
BEFORE UPDATE OR DELETE ON booking_access_entitlement_versions
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE communication_join_authorizations (
  id uuid PRIMARY KEY,
  booking_id uuid NOT NULL REFERENCES bookings(id),
  identity_id uuid NOT NULL REFERENCES identities(id),
  role text NOT NULL CHECK (role IN ('CLIENT','SPECIALIST')),
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  policy_version text NOT NULL REFERENCES communication_access_policy_versions(version),
  entitlement_version_id uuid REFERENCES booking_access_entitlement_versions(id),
  decision text NOT NULL CHECK (decision IN ('ALLOW','DENY')),
  reason_code text NOT NULL CHECK (btrim(reason_code) <> ''),
  credential_scope text,
  provider_credential_ref text,
  credential_expires_at timestamptz,
  idempotency_key text NOT NULL CHECK (btrim(idempotency_key) <> ''),
  decided_at timestamptz NOT NULL,
  UNIQUE (booking_id, identity_id, idempotency_key)
);

CREATE OR REPLACE FUNCTION apgic_communication_join_authorization_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  booking_row bookings%ROWTYPE;
  specialist_id uuid;
  provider_capability text;
  provider_status text;
  policy communication_access_policy_versions%ROWTYPE;
  entitlement booking_access_entitlement_versions%ROWTYPE;
  latest_entitlement_id uuid;
  window_opens_at timestamptz;
  window_closes_at timestamptz;
BEGIN
  SELECT *
  INTO booking_row
  FROM bookings
  WHERE id = NEW.booking_id;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'communication join authorization requires booking';
  END IF;

  SELECT specialist_identity_id
  INTO specialist_id
  FROM booking_slots
  WHERE id = booking_row.slot_id;

  IF (NEW.role = 'CLIENT' AND NEW.identity_id <> booking_row.client_identity_id)
    OR (NEW.role = 'SPECIALIST' AND NEW.identity_id <> specialist_id) THEN
    RAISE EXCEPTION 'communication join role/identity mismatch';
  END IF;

  SELECT capability_class, status
  INTO provider_capability, provider_status
  FROM connector_instances
  WHERE id = NEW.provider_instance_id;

  IF NOT FOUND OR provider_capability <> 'COMMUNICATION_PROVIDER' THEN
    RAISE EXCEPTION 'communication join requires COMMUNICATION_PROVIDER';
  END IF;

  SELECT *
  INTO policy
  FROM communication_access_policy_versions
  WHERE version = NEW.policy_version;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'communication join policy not found';
  END IF;

  window_opens_at := booking_row.starts_at - make_interval(secs => policy.join_early_seconds);
  window_closes_at := booking_row.ends_at + make_interval(secs => policy.join_late_seconds);

  IF NEW.decision = 'ALLOW' THEN
    IF booking_row.state <> 'CONFIRMED' THEN
      RAISE EXCEPTION 'communication join allow requires CONFIRMED booking';
    END IF;
    IF provider_status NOT IN ('ACTIVE','DEGRADED') THEN
      RAISE EXCEPTION 'communication join allow requires routable provider';
    END IF;
    IF NEW.entitlement_version_id IS NULL THEN
      RAISE EXCEPTION 'communication join allow requires entitlement evidence';
    END IF;

    SELECT *
    INTO entitlement
    FROM booking_access_entitlement_versions
    WHERE id = NEW.entitlement_version_id;

    SELECT id
    INTO latest_entitlement_id
    FROM booking_access_entitlement_versions
    WHERE booking_id = NEW.booking_id
      AND identity_id = NEW.identity_id
      AND role = NEW.role
      AND effective_from <= NEW.decided_at
    ORDER BY effective_from DESC, created_at DESC, id DESC
    LIMIT 1;

    IF NOT FOUND
      OR entitlement.id IS DISTINCT FROM latest_entitlement_id
      OR entitlement.booking_id <> NEW.booking_id
      OR entitlement.identity_id <> NEW.identity_id
      OR entitlement.role <> NEW.role
      OR entitlement.state <> 'ACTIVE'
      OR entitlement.effective_from > NEW.decided_at
      OR entitlement.expires_at < NEW.decided_at THEN
      RAISE EXCEPTION 'communication join allow requires current active entitlement';
    END IF;

    IF NEW.decided_at < window_opens_at OR NEW.decided_at >= window_closes_at THEN
      RAISE EXCEPTION 'communication join allow outside policy window';
    END IF;

    IF NEW.credential_scope <> 'ROOM_JOIN'
      OR nullif(btrim(NEW.provider_credential_ref), '') IS NULL
      OR NEW.credential_expires_at IS NULL
      OR NEW.credential_expires_at <= NEW.decided_at
      OR NEW.credential_expires_at > window_closes_at
      OR NEW.credential_expires_at >
        NEW.decided_at + make_interval(secs => policy.max_credential_ttl_seconds) THEN
      RAISE EXCEPTION 'communication join allow requires scoped short-lived credential reference';
    END IF;
  ELSE
    IF NEW.credential_scope IS NOT NULL
      OR NEW.provider_credential_ref IS NOT NULL
      OR NEW.credential_expires_at IS NOT NULL THEN
      RAISE EXCEPTION 'communication join deny cannot carry provider credential';
    END IF;
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER communication_join_authorizations_insert_guard
BEFORE INSERT ON communication_join_authorizations
FOR EACH ROW EXECUTE FUNCTION apgic_communication_join_authorization_guard();

CREATE TRIGGER communication_join_authorizations_append_only
BEFORE UPDATE OR DELETE ON communication_join_authorizations
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE consultation_sessions (
  id uuid PRIMARY KEY,
  booking_id uuid NOT NULL UNIQUE REFERENCES bookings(id),
  client_identity_id uuid NOT NULL REFERENCES identities(id),
  specialist_identity_id uuid NOT NULL REFERENCES identities(id),
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  state text NOT NULL CHECK (
    state IN ('SCHEDULED','READY','IN_PROGRESS','RECOVERING','TECHNICAL_FAILURE','COMPLETED')
  ),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE OR REPLACE FUNCTION apgic_consultation_session_insert_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  booking_row bookings%ROWTYPE;
  specialist_id uuid;
  provider_capability text;
BEGIN
  SELECT *
  INTO booking_row
  FROM bookings
  WHERE id = NEW.booking_id;

  IF NOT FOUND OR booking_row.state <> 'CONFIRMED' THEN
    RAISE EXCEPTION 'consultation session requires CONFIRMED booking';
  END IF;

  SELECT specialist_identity_id
  INTO specialist_id
  FROM booking_slots
  WHERE id = booking_row.slot_id;

  IF NEW.client_identity_id <> booking_row.client_identity_id
    OR NEW.specialist_identity_id <> specialist_id
    OR NEW.client_identity_id = NEW.specialist_identity_id THEN
    RAISE EXCEPTION 'consultation participants must match booking';
  END IF;

  SELECT capability_class
  INTO provider_capability
  FROM connector_instances
  WHERE id = NEW.provider_instance_id;

  IF NOT FOUND OR provider_capability <> 'COMMUNICATION_PROVIDER' THEN
    RAISE EXCEPTION 'consultation session requires COMMUNICATION_PROVIDER';
  END IF;

  IF NEW.state <> 'SCHEDULED' OR NEW.updated_at <> NEW.created_at THEN
    RAISE EXCEPTION 'new consultation session must start SCHEDULED';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER consultation_sessions_insert_guard
BEFORE INSERT ON consultation_sessions
FOR EACH ROW EXECUTE FUNCTION apgic_consultation_session_insert_guard();

CREATE TABLE consultation_lifecycle_facts (
  id uuid PRIMARY KEY,
  session_id uuid NOT NULL REFERENCES consultation_sessions(id),
  fact_type text NOT NULL CHECK (
    fact_type IN (
      'READY','JOINED','LEFT','STARTED','RECOVERY_STARTED',
      'RECOVERY_SUCCEEDED','TECHNICAL_FAILURE','ENDED'
    )
  ),
  role text NOT NULL CHECK (role IN ('CLIENT','SPECIALIST','SYSTEM')),
  identity_id uuid REFERENCES identities(id),
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  provider_reference text NOT NULL CHECK (btrim(provider_reference) <> ''),
  evidence_ref text NOT NULL CHECK (btrim(evidence_ref) <> ''),
  idempotency_key text NOT NULL CHECK (btrim(idempotency_key) <> ''),
  occurred_at timestamptz NOT NULL,
  UNIQUE (session_id, idempotency_key)
);

CREATE INDEX consultation_lifecycle_facts_session_time_idx
  ON consultation_lifecycle_facts (session_id, occurred_at, id);

CREATE OR REPLACE FUNCTION apgic_consultation_fact_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  session_row consultation_sessions%ROWTYPE;
  client_joined boolean;
  specialist_joined boolean;
  provider_capability text;
BEGIN
  SELECT *
  INTO session_row
  FROM consultation_sessions
  WHERE id = NEW.session_id
  FOR UPDATE;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'consultation lifecycle fact requires session';
  END IF;

  IF NEW.occurred_at < session_row.created_at OR NEW.occurred_at < session_row.updated_at THEN
    RAISE EXCEPTION 'consultation lifecycle fact time cannot move backwards';
  END IF;

  SELECT capability_class
  INTO provider_capability
  FROM connector_instances
  WHERE id = NEW.provider_instance_id;

  IF NOT FOUND OR provider_capability <> 'COMMUNICATION_PROVIDER' THEN
    RAISE EXCEPTION 'consultation lifecycle fact requires COMMUNICATION_PROVIDER evidence';
  END IF;

  IF NEW.fact_type IN ('READY','JOINED','LEFT') THEN
    IF NEW.role = 'CLIENT' AND NEW.identity_id <> session_row.client_identity_id THEN
      RAISE EXCEPTION 'client lifecycle fact identity mismatch';
    ELSIF NEW.role = 'SPECIALIST' AND NEW.identity_id <> session_row.specialist_identity_id THEN
      RAISE EXCEPTION 'specialist lifecycle fact identity mismatch';
    ELSIF NEW.role NOT IN ('CLIENT','SPECIALIST') OR NEW.identity_id IS NULL THEN
      RAISE EXCEPTION 'participant lifecycle fact requires participant identity';
    END IF;
  ELSE
    IF NEW.role <> 'SYSTEM' OR NEW.identity_id IS NOT NULL THEN
      RAISE EXCEPTION 'system lifecycle fact cannot impersonate participant';
    END IF;
  END IF;

  SELECT EXISTS (
    SELECT 1 FROM consultation_lifecycle_facts
    WHERE session_id = NEW.session_id AND fact_type = 'JOINED' AND role = 'CLIENT'
  ) INTO client_joined;

  SELECT EXISTS (
    SELECT 1 FROM consultation_lifecycle_facts
    WHERE session_id = NEW.session_id AND fact_type = 'JOINED' AND role = 'SPECIALIST'
  ) INTO specialist_joined;

  IF NEW.fact_type = 'STARTED' AND (
    session_row.state NOT IN ('SCHEDULED','READY')
    OR NOT client_joined
    OR NOT specialist_joined
  ) THEN
    RAISE EXCEPTION 'consultation STARTED requires both participant JOINED evidence';
  ELSIF NEW.fact_type = 'LEFT' AND session_row.state NOT IN ('IN_PROGRESS','RECOVERING') THEN
    RAISE EXCEPTION 'consultation LEFT requires active/recovering session';
  ELSIF NEW.fact_type = 'RECOVERY_STARTED' AND session_row.state <> 'IN_PROGRESS' THEN
    RAISE EXCEPTION 'consultation recovery requires IN_PROGRESS session';
  ELSIF NEW.fact_type = 'RECOVERY_SUCCEEDED' AND session_row.state <> 'RECOVERING' THEN
    RAISE EXCEPTION 'consultation recovery success requires RECOVERING session';
  ELSIF NEW.fact_type = 'TECHNICAL_FAILURE'
    AND session_row.state NOT IN ('IN_PROGRESS','RECOVERING') THEN
    RAISE EXCEPTION 'consultation technical failure requires active/recovering session';
  ELSIF NEW.fact_type = 'ENDED'
    AND session_row.state NOT IN ('IN_PROGRESS','RECOVERING') THEN
    RAISE EXCEPTION 'consultation completion requires active/recovering session';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER consultation_lifecycle_facts_insert_guard
BEFORE INSERT ON consultation_lifecycle_facts
FOR EACH ROW EXECUTE FUNCTION apgic_consultation_fact_guard();

CREATE TRIGGER consultation_lifecycle_facts_append_only
BEFORE UPDATE OR DELETE ON consultation_lifecycle_facts
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE OR REPLACE FUNCTION apgic_consultation_fact_apply_state()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  next_state text;
  client_ready boolean;
  specialist_ready boolean;
BEGIN
  next_state := NULL;

  IF NEW.fact_type IN ('READY','JOINED') THEN
    SELECT EXISTS (
      SELECT 1 FROM consultation_lifecycle_facts
      WHERE session_id = NEW.session_id
        AND role = 'CLIENT'
        AND fact_type IN ('READY','JOINED')
    ) INTO client_ready;

    SELECT EXISTS (
      SELECT 1 FROM consultation_lifecycle_facts
      WHERE session_id = NEW.session_id
        AND role = 'SPECIALIST'
        AND fact_type IN ('READY','JOINED')
    ) INTO specialist_ready;

    IF client_ready AND specialist_ready THEN
      next_state := 'READY';
    END IF;
  ELSIF NEW.fact_type = 'STARTED' THEN
    next_state := 'IN_PROGRESS';
  ELSIF NEW.fact_type = 'RECOVERY_STARTED' THEN
    next_state := 'RECOVERING';
  ELSIF NEW.fact_type = 'RECOVERY_SUCCEEDED' THEN
    next_state := 'IN_PROGRESS';
  ELSIF NEW.fact_type = 'TECHNICAL_FAILURE' THEN
    next_state := 'TECHNICAL_FAILURE';
  ELSIF NEW.fact_type = 'ENDED' THEN
    next_state := 'COMPLETED';
  END IF;

  IF next_state IS NOT NULL THEN
    UPDATE consultation_sessions
    SET state = next_state, updated_at = NEW.occurred_at
    WHERE id = NEW.session_id;
  ELSE
    UPDATE consultation_sessions
    SET updated_at = NEW.occurred_at
    WHERE id = NEW.session_id;
  END IF;

  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION apgic_consultation_session_transition_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  supporting_fact_type text;
BEGIN
  IF NEW.booking_id <> OLD.booking_id
    OR NEW.client_identity_id <> OLD.client_identity_id
    OR NEW.specialist_identity_id <> OLD.specialist_identity_id
    OR NEW.provider_instance_id <> OLD.provider_instance_id
    OR NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'consultation session identity is immutable';
  END IF;

  IF NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'consultation session updated_at cannot move backwards';
  END IF;

  IF NEW.state = OLD.state THEN
    RETURN NEW;
  END IF;

  supporting_fact_type := CASE
    WHEN NEW.state = 'READY' THEN NULL
    WHEN NEW.state = 'IN_PROGRESS' AND OLD.state = 'RECOVERING' THEN 'RECOVERY_SUCCEEDED'
    WHEN NEW.state = 'IN_PROGRESS' THEN 'STARTED'
    WHEN NEW.state = 'RECOVERING' THEN 'RECOVERY_STARTED'
    WHEN NEW.state = 'TECHNICAL_FAILURE' THEN 'TECHNICAL_FAILURE'
    WHEN NEW.state = 'COMPLETED' THEN 'ENDED'
    ELSE '__INVALID__'
  END;

  IF NEW.state = 'READY' THEN
    IF NOT EXISTS (
      SELECT 1 FROM consultation_lifecycle_facts
      WHERE session_id = NEW.id AND role = 'CLIENT' AND fact_type IN ('READY','JOINED')
    ) OR NOT EXISTS (
      SELECT 1 FROM consultation_lifecycle_facts
      WHERE session_id = NEW.id AND role = 'SPECIALIST' AND fact_type IN ('READY','JOINED')
    ) THEN
      RAISE EXCEPTION 'consultation READY requires participant readiness evidence';
    END IF;
  ELSIF supporting_fact_type = '__INVALID__'
    OR NOT EXISTS (
      SELECT 1 FROM consultation_lifecycle_facts
      WHERE session_id = NEW.id
        AND fact_type = supporting_fact_type
        AND occurred_at = NEW.updated_at
    ) THEN
    RAISE EXCEPTION 'consultation state transition requires matching lifecycle fact';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER consultation_sessions_transition_guard
BEFORE UPDATE ON consultation_sessions
FOR EACH ROW EXECUTE FUNCTION apgic_consultation_session_transition_guard();

CREATE TRIGGER consultation_lifecycle_facts_apply_state
AFTER INSERT ON consultation_lifecycle_facts
FOR EACH ROW EXECUTE FUNCTION apgic_consultation_fact_apply_state();

CREATE TABLE consultation_recovery_decisions (
  id uuid PRIMARY KEY,
  session_id uuid NOT NULL REFERENCES consultation_sessions(id),
  source_fact_id uuid NOT NULL REFERENCES consultation_lifecycle_facts(id),
  policy_version text NOT NULL CHECK (btrim(policy_version) <> ''),
  action text NOT NULL CHECK (
    action IN ('RETRY_SAME_PROVIDER','FALLBACK_PROVIDER','RESCHEDULE','REFUND')
  ),
  target_provider_instance_id uuid REFERENCES connector_instances(id),
  followup_path_ref text,
  reason_code text NOT NULL CHECK (btrim(reason_code) <> ''),
  decided_at timestamptz NOT NULL,
  UNIQUE (session_id, source_fact_id, policy_version)
);

CREATE OR REPLACE FUNCTION apgic_consultation_recovery_decision_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $
DECLARE
  source_fact consultation_lifecycle_facts%ROWTYPE;
  session_row consultation_sessions%ROWTYPE;
  target_capability text;
  target_status text;
BEGIN
  SELECT *
  INTO source_fact
  FROM consultation_lifecycle_facts
  WHERE id = NEW.source_fact_id;

  IF NOT FOUND OR source_fact.session_id <> NEW.session_id THEN
    RAISE EXCEPTION 'consultation recovery decision requires matching lifecycle fact';
  END IF;

  SELECT *
  INTO session_row
  FROM consultation_sessions
  WHERE id = NEW.session_id;

  IF NOT FOUND OR NEW.decided_at < source_fact.occurred_at THEN
    RAISE EXCEPTION 'consultation recovery decision time/session invalid';
  END IF;

  IF NEW.action IN ('RETRY_SAME_PROVIDER','FALLBACK_PROVIDER') THEN
    IF source_fact.fact_type <> 'RECOVERY_STARTED'
      OR NEW.target_provider_instance_id IS NULL
      OR NEW.followup_path_ref IS NOT NULL THEN
      RAISE EXCEPTION 'provider recovery decision requires RECOVERY_STARTED and target provider';
    END IF;

    SELECT capability_class, status
    INTO target_capability, target_status
    FROM connector_instances
    WHERE id = NEW.target_provider_instance_id;

    IF NOT FOUND
      OR target_capability <> 'COMMUNICATION_PROVIDER'
      OR target_status NOT IN ('ACTIVE','DEGRADED') THEN
      RAISE EXCEPTION 'recovery target must be routable COMMUNICATION_PROVIDER';
    END IF;

    IF NEW.action = 'RETRY_SAME_PROVIDER'
      AND NEW.target_provider_instance_id <> source_fact.provider_instance_id THEN
      RAISE EXCEPTION 'retry-same recovery must retain provider identity';
    END IF;

    IF NEW.action = 'FALLBACK_PROVIDER'
      AND NEW.target_provider_instance_id = source_fact.provider_instance_id THEN
      RAISE EXCEPTION 'fallback recovery must select a different provider';
    END IF;
  ELSE
    IF source_fact.fact_type <> 'TECHNICAL_FAILURE'
      OR NEW.target_provider_instance_id IS NOT NULL
      OR nullif(btrim(NEW.followup_path_ref), '') IS NULL THEN
      RAISE EXCEPTION 'terminal technical recovery requires reschedule/refund follow-up path';
    END IF;
  END IF;

  RETURN NEW;
END;
$;

CREATE TRIGGER consultation_recovery_decisions_insert_guard
BEFORE INSERT ON consultation_recovery_decisions
FOR EACH ROW EXECUTE FUNCTION apgic_consultation_recovery_decision_guard();

CREATE TRIGGER consultation_recovery_decisions_append_only
BEFORE UPDATE OR DELETE ON consultation_recovery_decisions
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE OR REPLACE FUNCTION apgic_booking_transition_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.slot_id <> OLD.slot_id OR
     NEW.hold_id <> OLD.hold_id OR
     NEW.client_identity_id <> OLD.client_identity_id OR
     NEW.hold_expires_at <> OLD.hold_expires_at OR
     NEW.starts_at <> OLD.starts_at OR
     NEW.ends_at <> OLD.ends_at OR
     NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'booking canonical identity/timing is immutable';
  END IF;

  IF NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'booking updated_at cannot move backwards';
  END IF;

  IF OLD.state = NEW.state THEN
    RETURN NEW;
  END IF;

  IF OLD.state IN ('HELD','PENDING_PAYMENT')
    AND NEW.state <> 'EXPIRED'
    AND NEW.updated_at >= OLD.hold_expires_at THEN
    RAISE EXCEPTION 'booking hold expired before transition';
  END IF;

  IF NEW.state = 'EXPIRED'
    AND (
      OLD.state NOT IN ('HELD','PENDING_PAYMENT')
      OR NEW.updated_at < OLD.hold_expires_at
    ) THEN
    RAISE EXCEPTION 'booking cannot expire before hold expiry';
  END IF;

  IF NEW.state = 'NO_SHOW'
    AND (
      OLD.state <> 'CONFIRMED'
      OR NEW.updated_at < OLD.starts_at
    ) THEN
    RAISE EXCEPTION 'booking cannot become NO_SHOW before start';
  END IF;

  IF NEW.state = 'COMPLETED' THEN
    IF OLD.state <> 'CONFIRMED' OR NOT EXISTS (
      SELECT 1
      FROM consultation_sessions
      WHERE booking_id = NEW.id
        AND state = 'COMPLETED'
        AND updated_at <= NEW.updated_at
    ) THEN
      RAISE EXCEPTION 'booking completion requires consultation completion evidence';
    END IF;
  END IF;

  IF OLD.state = 'HELD'
    AND NEW.state IN ('PENDING_PAYMENT','CONFIRMED','CANCELLED','EXPIRED') THEN
    RETURN NEW;
  ELSIF OLD.state = 'PENDING_PAYMENT'
    AND NEW.state IN ('CONFIRMED','CANCELLED','EXPIRED') THEN
    RETURN NEW;
  ELSIF OLD.state = 'CONFIRMED'
    AND NEW.state IN ('CANCELLED','COMPLETED','NO_SHOW') THEN
    RETURN NEW;
  END IF;

  RAISE EXCEPTION 'invalid booking transition: % -> %', OLD.state, NEW.state;
END;
$$;

COMMIT;
