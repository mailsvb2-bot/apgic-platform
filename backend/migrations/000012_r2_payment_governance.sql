BEGIN;

ALTER TABLE payment_provider_config_versions
  ADD COLUMN routing_weight_bps integer NOT NULL DEFAULT 10000
    CHECK (routing_weight_bps BETWEEN 0 AND 10000),
  ADD COLUMN credential_version_ref text,
  ADD COLUMN supersedes_config_id uuid REFERENCES payment_provider_config_versions(id),
  ADD COLUMN change_kind text CHECK (
    change_kind IS NULL OR change_kind IN (
      'ACTIVATE','PRIORITIZE','SCOPE','PAUSE','ROTATE_CREDENTIAL','DISABLE','ROLLBACK'
    )
  ),
  ADD COLUMN actor_id text,
  ADD COLUMN reason text,
  ADD COLUMN authorization_audit_id uuid REFERENCES audit_records(id),
  ADD COLUMN change_audit_id uuid REFERENCES audit_records(id);

CREATE TABLE payment_provider_control_events (
  id uuid PRIMARY KEY,
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  action text NOT NULL CHECK (
    action IN (
      'CONNECT','TEST','ACTIVATE','PRIORITIZE','SCOPE',
      'PAUSE','ROTATE_CREDENTIAL','DISABLE','ROLLBACK'
    )
  ),
  actor_id text NOT NULL CHECK (btrim(actor_id) <> ''),
  reason text NOT NULL CHECK (btrim(reason) <> ''),
  authorization_audit_id uuid NOT NULL REFERENCES audit_records(id),
  result text NOT NULL CHECK (result IN ('SUCCEEDED','FAILED')),
  evidence_refs text[] NOT NULL CHECK (cardinality(evidence_refs) > 0),
  occurred_at timestamptz NOT NULL
);

CREATE TRIGGER payment_provider_control_events_append_only
BEFORE UPDATE OR DELETE ON payment_provider_control_events
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE OR REPLACE FUNCTION apgic_payment_provider_control_event_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  connector_capability text;
  auth_actor text;
  auth_action text;
  auth_state jsonb;
BEGIN
  SELECT capability_class
  INTO connector_capability
  FROM connector_instances
  WHERE id = NEW.provider_instance_id;

  IF NOT FOUND OR connector_capability <> 'PAYMENT_PROVIDER' THEN
    RAISE EXCEPTION 'payment provider control event requires PAYMENT_PROVIDER connector';
  END IF;

  SELECT actor_id, action, new_state
  INTO auth_actor, auth_action, auth_state
  FROM audit_records
  WHERE id = NEW.authorization_audit_id;

  IF NOT FOUND OR
     auth_actor <> NEW.actor_id OR
     auth_action <> 'authorization.decision' OR
     auth_state->>'decision' <> 'ALLOW' OR
     auth_state->>'risk' <> 'HIGH_RISK' OR
     auth_state->>'action' <> 'payment.provider.manage' THEN
    RAISE EXCEPTION 'payment provider control event requires high-risk ALLOW authorization evidence';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM unnest(NEW.evidence_refs) AS evidence_ref
    WHERE btrim(evidence_ref) = ''
  ) THEN
    RAISE EXCEPTION 'payment provider control evidence refs cannot be blank';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER payment_provider_control_events_insert_guard
BEFORE INSERT ON payment_provider_control_events
FOR EACH ROW EXECUTE FUNCTION apgic_payment_provider_control_event_guard();

CREATE OR REPLACE FUNCTION apgic_payment_provider_config_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  connector_capability text;
  connector_status text;
  previous_config payment_provider_config_versions%ROWTYPE;
  auth_actor text;
  auth_action text;
  auth_state jsonb;
  change_actor text;
  change_action text;
BEGIN
  SELECT capability_class, status
  INTO connector_capability, connector_status
  FROM connector_instances
  WHERE id = NEW.provider_instance_id;

  IF NOT FOUND OR connector_capability <> 'PAYMENT_PROVIDER' THEN
    RAISE EXCEPTION 'payment provider config requires PAYMENT_PROVIDER connector';
  END IF;

  IF NEW.status = 'ACTIVE' AND connector_status NOT IN ('ACTIVE','DEGRADED') THEN
    RAISE EXCEPTION 'ACTIVE payment provider config requires routable connector status';
  END IF;

  IF NEW.routing_weight_bps < 0 OR NEW.routing_weight_bps > 10000 THEN
    RAISE EXCEPTION 'payment provider routing weight must be within 0..10000';
  END IF;

  IF btrim(coalesce(NEW.credential_version_ref, '')) = '' THEN
    RAISE EXCEPTION 'payment provider config requires credential version reference';
  END IF;

  IF EXISTS (
    SELECT 1 FROM unnest(NEW.currencies) AS currency
    WHERE currency !~ '^[A-Z]{3}$'
  ) THEN
    RAISE EXCEPTION 'payment provider currency must be uppercase ISO-style code';
  END IF;

  IF EXISTS (
    SELECT 1 FROM unnest(
      NEW.certification_evidence_refs ||
      NEW.jurisdiction_codes ||
      NEW.method_codes ||
      NEW.rail_codes
    ) AS value
    WHERE btrim(value) = ''
  ) THEN
    RAISE EXCEPTION 'payment provider config arrays cannot contain blank values';
  END IF;

  SELECT *
  INTO previous_config
  FROM payment_provider_config_versions
  WHERE provider_instance_id = NEW.provider_instance_id
  ORDER BY effective_from DESC, created_at DESC, id DESC
  LIMIT 1;

  IF FOUND THEN
    IF NEW.supersedes_config_id IS NULL OR NEW.supersedes_config_id <> previous_config.id THEN
      RAISE EXCEPTION 'payment provider revision must supersede latest config';
    END IF;
    IF NEW.effective_from <= previous_config.effective_from THEN
      RAISE EXCEPTION 'payment provider revision effective time must advance';
    END IF;
    IF btrim(coalesce(NEW.actor_id, '')) = '' OR
       btrim(coalesce(NEW.reason, '')) = '' OR
       NEW.change_kind IS NULL OR
       NEW.authorization_audit_id IS NULL OR
       NEW.change_audit_id IS NULL THEN
      RAISE EXCEPTION 'payment provider revision requires actor/reason/change/audit evidence';
    END IF;

    SELECT actor_id, action, new_state
    INTO auth_actor, auth_action, auth_state
    FROM audit_records
    WHERE id = NEW.authorization_audit_id;

    IF NOT FOUND OR
       auth_actor <> NEW.actor_id OR
       auth_action <> 'authorization.decision' OR
       auth_state->>'decision' <> 'ALLOW' OR
       auth_state->>'risk' <> 'HIGH_RISK' OR
       auth_state->>'action' <> 'payment.provider.manage' THEN
      RAISE EXCEPTION 'payment provider revision requires high-risk ALLOW authorization audit';
    END IF;

    SELECT actor_id, action
    INTO change_actor, change_action
    FROM audit_records
    WHERE id = NEW.change_audit_id;

    IF NOT FOUND OR
       change_actor <> NEW.actor_id OR
       change_action <> 'payment.provider.config_change' THEN
      RAISE EXCEPTION 'payment provider revision requires config change audit';
    END IF;
  END IF;

  RETURN NEW;
END;
$$;

CREATE TABLE payment_provider_health_snapshots (
  id uuid PRIMARY KEY,
  provider_config_id uuid NOT NULL REFERENCES payment_provider_config_versions(id),
  health text NOT NULL CHECK (health IN ('HEALTHY','DEGRADED','UNAVAILABLE')),
  conversion_rate_bps integer NOT NULL CHECK (conversion_rate_bps BETWEEN 0 AND 10000),
  latency_p95_ms integer NOT NULL CHECK (latency_p95_ms >= 0),
  provider_reported_fee_bps integer NOT NULL CHECK (provider_reported_fee_bps BETWEEN 0 AND 10000),
  reconciliation_pending_count integer NOT NULL CHECK (reconciliation_pending_count >= 0),
  reconciliation_mismatch_count integer NOT NULL CHECK (reconciliation_mismatch_count >= 0),
  guardrail_action text NOT NULL CHECK (
    guardrail_action IN ('ALLOW_NEW_ATTEMPTS','BLOCK_NEW_ATTEMPTS')
  ),
  evidence_refs text[] NOT NULL CHECK (cardinality(evidence_refs) > 0),
  observed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX payment_provider_health_latest_idx
  ON payment_provider_health_snapshots (provider_config_id, observed_at DESC);

CREATE OR REPLACE FUNCTION apgic_payment_provider_health_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM unnest(NEW.evidence_refs) AS evidence_ref
    WHERE btrim(evidence_ref) = ''
  ) THEN
    RAISE EXCEPTION 'payment health evidence refs cannot be blank';
  END IF;

  IF NEW.health = 'UNAVAILABLE' AND NEW.guardrail_action <> 'BLOCK_NEW_ATTEMPTS' THEN
    RAISE EXCEPTION 'unavailable provider must block new attempts';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER payment_provider_health_snapshots_insert_guard
BEFORE INSERT ON payment_provider_health_snapshots
FOR EACH ROW EXECUTE FUNCTION apgic_payment_provider_health_guard();

CREATE TRIGGER payment_provider_health_snapshots_append_only
BEFORE UPDATE OR DELETE ON payment_provider_health_snapshots
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

ALTER TABLE payment_routing_decisions
  ADD COLUMN health_snapshot_id uuid REFERENCES payment_provider_health_snapshots(id);

CREATE OR REPLACE FUNCTION apgic_payment_routing_decision_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  config payment_provider_config_versions%ROWTYPE;
  latest_config_id uuid;
  order_row orders%ROWTYPE;
  health payment_provider_health_snapshots%ROWTYPE;
  latest_health_id uuid;
BEGIN
  SELECT *
  INTO config
  FROM payment_provider_config_versions
  WHERE id = NEW.provider_config_id;

  IF NOT FOUND OR config.status <> 'ACTIVE' OR config.effective_from > NEW.decided_at THEN
    RAISE EXCEPTION 'routing decision requires effective ACTIVE certified provider config';
  END IF;

  SELECT id
  INTO latest_config_id
  FROM payment_provider_config_versions
  WHERE provider_instance_id = config.provider_instance_id
    AND effective_from <= NEW.decided_at
  ORDER BY effective_from DESC, created_at DESC, id DESC
  LIMIT 1;

  IF latest_config_id IS DISTINCT FROM config.id THEN
    RAISE EXCEPTION 'routing decision cannot use superseded provider config';
  END IF;

  IF NEW.health_snapshot_id IS NULL THEN
    RAISE EXCEPTION 'routing decision requires provider health snapshot';
  END IF;

  SELECT *
  INTO health
  FROM payment_provider_health_snapshots
  WHERE id = NEW.health_snapshot_id;

  IF NOT FOUND OR
     health.provider_config_id <> config.id OR
     health.observed_at > NEW.decided_at OR
     health.health = 'UNAVAILABLE' OR
     health.guardrail_action <> 'ALLOW_NEW_ATTEMPTS' THEN
    RAISE EXCEPTION 'routing decision provider health guardrail blocks new attempts';
  END IF;

  SELECT id
  INTO latest_health_id
  FROM payment_provider_health_snapshots
  WHERE provider_config_id = config.id
    AND observed_at <= NEW.decided_at
  ORDER BY observed_at DESC, created_at DESC, id DESC
  LIMIT 1;

  IF latest_health_id IS DISTINCT FROM health.id THEN
    RAISE EXCEPTION 'routing decision requires latest provider health evidence';
  END IF;

  SELECT *
  INTO order_row
  FROM orders
  WHERE id = NEW.order_id;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'routing decision order not found';
  END IF;

  IF NOT (order_row.currency = ANY(config.currencies)) OR
     NOT (NEW.jurisdiction_code = ANY(config.jurisdiction_codes)) OR
     NOT (NEW.selected_method_code = ANY(config.method_codes)) OR
     NOT (NEW.selected_rail_code = ANY(config.rail_codes)) THEN
    RAISE EXCEPTION 'routing decision selected ineligible provider/method/rail scope';
  END IF;

  IF jsonb_typeof(NEW.candidate_evidence) <> 'array' OR
     jsonb_array_length(NEW.candidate_evidence) = 0 OR
     jsonb_typeof(NEW.health_snapshot) <> 'object' THEN
    RAISE EXCEPTION 'routing decision requires reproducible candidate and health evidence';
  END IF;

  RETURN NEW;
END;
$$;

COMMIT;
